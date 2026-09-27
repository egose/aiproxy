package store

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func openLedgerTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "ledger_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.DB.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.DB.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	st, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	return st
}

func ledgerFixture(t *testing.T, st *Store) (Workspace, User, WorkspaceTeam, InboundKey) {
	t.Helper()
	ctx := context.Background()
	workspace := Workspace{Name: "ledger-" + uuid.NewString()}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	user := User{Email: uuid.NewString() + "@example.com", PasswordHash: "unused"}
	if err := st.CreateUser(ctx, &user); err != nil {
		t.Fatal(err)
	}
	if err := st.AddMembership(ctx, &WorkspaceMember{UserID: user.ID, WorkspaceID: workspace.ID, Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	team := WorkspaceTeam{WorkspaceID: workspace.ID, Name: "ledger-team"}
	if err := st.CreateTeam(ctx, &team); err != nil {
		t.Fatal(err)
	}
	key := InboundKey{Name: uuid.NewString(), TokenHash: uuid.NewString(), TokenPrefix: "test", WorkspaceID: workspace.ID, OwnerUserID: &user.ID, Enabled: true}
	if err := st.CreateInboundKey(ctx, &key); err != nil {
		t.Fatal(err)
	}
	return workspace, user, team, key
}

func TestSpendLedgerPreservesDeletedKeySpend(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	if again, err := st.MigrateUp(ctx); err != nil || len(again) != 0 {
		t.Fatalf("repeat schema application = %v, %v", again, err)
	}
	status, err := st.Status(ctx)
	if err != nil || !reflect.DeepEqual(status.Applied, []string{"schema.sql"}) || len(status.Pending) != 0 {
		t.Fatalf("schema status = %+v, %v", status, err)
	}
	workspace, user, team, key := ledgerFixture(t, st)
	unspentKey := InboundKey{Name: uuid.NewString(), TokenHash: uuid.NewString(), TokenPrefix: "test", WorkspaceID: workspace.ID, OwnerUserID: &user.ID, Enabled: true}
	if err := st.CreateInboundKey(ctx, &unspentKey); err != nil {
		t.Fatal(err)
	}
	teamKey := InboundKey{Name: uuid.NewString(), TokenHash: uuid.NewString(), TokenPrefix: "test", WorkspaceID: workspace.ID, OwnerTeamID: &team.ID, Enabled: true}
	if err := st.CreateInboundKey(ctx, &teamKey); err != nil {
		t.Fatal(err)
	}
	entries := []SpendEntry{
		{WorkspaceID: workspace.ID, KeyID: key.ID, UserID: &user.ID, Model: "alias/user", Tokens: 11, CostMicros: 100},
		{WorkspaceID: workspace.ID, KeyID: teamKey.ID, TeamID: &team.ID, Model: "alias/team", Tokens: 22, CostMicros: 200},
	}
	for i := range entries {
		if err := st.InsertSpendEntry(ctx, &entries[i]); err != nil {
			t.Fatal(err)
		}
	}
	quota := ScopeQuota{WorkspaceID: workspace.ID, ScopeType: "user", ScopeID: user.ID, BudgetMicros: 50, SpentOffsetMicros: 25}
	if err := st.UpsertScopeQuota(ctx, &quota); err != nil {
		t.Fatal(err)
	}
	for _, keyID := range []uuid.UUID{key.ID, teamKey.ID, unspentKey.ID} {
		if err := st.DeleteInboundKey(ctx, keyID); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range entries {
		var got SpendEntry
		if err := st.DB.NewSelect().Model(&got).Where("id = ?", want.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		if got.KeyID != want.KeyID || got.WorkspaceID != want.WorkspaceID || got.Model != want.Model || got.Tokens != want.Tokens || got.CostMicros != want.CostMicros || !got.CreatedAt.Equal(want.CreatedAt.Truncate(time.Microsecond)) {
			t.Fatalf("upgrade changed entry: got %+v, want %+v", got, want)
		}
		if fmt.Sprint(got.UserID) != fmt.Sprint(want.UserID) || fmt.Sprint(got.TeamID) != fmt.Sprint(want.TeamID) {
			t.Fatalf("upgrade changed ownership: %+v", got)
		}
	}
	for scope, id := range map[string]uuid.UUID{"user": user.ID, "team": team.ID} {
		want := int64(100)
		if scope == "team" {
			want = 200
		}
		if sum, err := st.SumScopeSpend(ctx, workspace.ID, scope, id); err != nil || sum != want {
			t.Fatalf("%s spend = %d, %v; want %d", scope, sum, err, want)
		}
	}
	gotQuota, err := st.GetScopeQuota(ctx, workspace.ID, "user", user.ID, "")
	if err != nil || gotQuota.BudgetMicros != 50 || gotQuota.SpentOffsetMicros != 25 {
		t.Fatalf("upgrade changed quota/reset offset: %+v, %v", gotQuota, err)
	}
	late := SpendEntry{WorkspaceID: workspace.ID, KeyID: key.ID, UserID: &user.ID, Tokens: 3, CostMicros: 30}
	if err := st.InsertSpendEntry(ctx, &late); err != nil {
		t.Fatalf("late completion for migrated key: %v", err)
	}
	if sum, err := st.SumScopeSpend(ctx, workspace.ID, "user", user.ID); err != nil || sum != 130 {
		t.Fatalf("late migrated spend = %d, %v", sum, err)
	}
	if err := st.InsertSpendEntry(ctx, &SpendEntry{WorkspaceID: workspace.ID, KeyID: unspentKey.ID, UserID: &user.ID, CostMicros: 10}); err != nil {
		t.Fatalf("first completion for migrated, deleted key: %v", err)
	}
}

func TestSpendLedgerDeletionOrderings(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	for _, ordering := range []string{"charge-first", "delete-first", "concurrent"} {
		t.Run(ordering, func(t *testing.T) {
			workspace, user, _, key := ledgerFixture(t, st)
			entry := SpendEntry{WorkspaceID: workspace.ID, KeyID: key.ID, UserID: &user.ID, Tokens: 7, CostMicros: 70}
			charge := func() error { return st.InsertSpendEntry(ctx, &entry) }
			remove := func() error { return st.DeleteInboundKey(ctx, key.ID) }
			switch ordering {
			case "charge-first":
				if err := charge(); err != nil {
					t.Fatal(err)
				}
				if err := remove(); err != nil {
					t.Fatal(err)
				}
			case "delete-first":
				if err := remove(); err != nil {
					t.Fatal(err)
				}
				if err := charge(); err != nil {
					t.Fatal(err)
				}
			case "concurrent":
				errs := make(chan error, 2)
				start := make(chan struct{})
				go func() { <-start; errs <- charge() }()
				go func() { <-start; errs <- remove() }()
				close(start)
				for range 2 {
					if err := <-errs; err != nil {
						t.Error(err)
					}
				}
			}
			usage, err := st.KeyUsageByWorkspace(ctx, workspace.ID)
			if err != nil || len(usage) != 1 || usage[0].KeyID != key.ID || usage[0].Requests != 1 || usage[0].Tokens != 7 || usage[0].Spend != 70 {
				t.Fatalf("usage after %s = %+v, %v", ordering, usage, err)
			}
			if sum, err := st.SumScopeSpend(ctx, workspace.ID, "user", user.ID); err != nil || sum != 70 {
				t.Fatalf("spend after %s = %d, %v", ordering, sum, err)
			}
		})
	}
}

func TestSpendLedgerKeyIdentityIntegrity(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	workspace, user, _, key := ledgerFixture(t, st)
	otherWorkspace, _, _, _ := ledgerFixture(t, st)
	for _, deleted := range []bool{false, true} {
		if deleted {
			if err := st.DeleteInboundKey(ctx, key.ID); err != nil {
				t.Fatal(err)
			}
		}
		for _, entry := range []SpendEntry{
			{WorkspaceID: workspace.ID, KeyID: uuid.New(), UserID: &user.ID, CostMicros: 1},
			{WorkspaceID: otherWorkspace.ID, KeyID: key.ID, UserID: &user.ID, CostMicros: 1},
		} {
			if err := st.InsertSpendEntry(ctx, &entry); err == nil {
				t.Fatalf("invalid identity accepted (deleted=%v): %+v", deleted, entry)
			}
		}
	}
	if sum, err := st.SumScopeSpend(ctx, workspace.ID, "user", user.ID); err != nil || sum != 0 {
		t.Fatalf("invalid charge persisted: %d, %v", sum, err)
	}
	if err := st.InsertSpendEntry(ctx, &SpendEntry{WorkspaceID: workspace.ID, KeyID: key.ID, UserID: &user.ID, CostMicros: 10}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteWorkspace(ctx, workspace.ID); err != nil {
		t.Fatalf("workspaceanization deletion: %v", err)
	}
	for _, table := range []string{"spend_ledger", "spend_key_identities"} {
		count, err := st.DB.NewSelect().Table(table).Where("workspace_id = ?", workspace.ID).Count(ctx)
		if err != nil || count != 0 {
			t.Fatalf("workspaceanization deletion retained %s: %d, %v", table, count, err)
		}
	}
}
