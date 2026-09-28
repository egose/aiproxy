package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOffboardingScopedCleanupAndRollback(t *testing.T) {
	for _, failure := range []string{"none", "sole-admin", "late-delete"} {
		t.Run(failure, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx := context.Background()
			workspace, team, users := membershipFixture(t, st, "team")
			u, successor := users[0].ID, users[1].ID
			other := Workspace{Name: uuid.NewString()}
			if err := st.CreateWorkspace(ctx, &other); err != nil {
				t.Fatal(err)
			}
			if err := st.AddMembership(ctx, &WorkspaceMember{UserID: u, WorkspaceID: other.ID, Role: "admin"}); err != nil {
				t.Fatal(err)
			}
			otherTeam := WorkspaceTeam{WorkspaceID: other.ID, Name: "other"}
			if err := st.CreateTeam(ctx, &otherTeam); err != nil {
				t.Fatal(err)
			}
			if err := st.AddTeamMember(ctx, u, otherTeam.ID, "admin"); err != nil {
				t.Fatal(err)
			}
			plainTeam := WorkspaceTeam{WorkspaceID: workspace, Name: "plain"}
			if err := st.CreateTeam(ctx, &plainTeam); err != nil {
				t.Fatal(err)
			}
			if err := st.AddTeamMember(ctx, u, plainTeam.ID, "member"); err != nil {
				t.Fatal(err)
			}
			keys := []InboundKey{
				{Name: uuid.NewString(), WorkspaceID: workspace, OwnerUserID: &u},
				{Name: uuid.NewString(), WorkspaceID: workspace, OwnerTeamID: &team},
				{Name: uuid.NewString(), WorkspaceID: other.ID, OwnerUserID: &u},
			}
			for i := range keys {
				k := &keys[i]
				k.TokenHash, k.TokenPrefix, k.Enabled = uuid.NewString(), "retained", true
				if err := st.CreateInboundKey(ctx, k); err != nil {
					t.Fatal(err)
				}
				if err := st.DB.NewSelect().Model(k).Where("id = ?", k.ID).Scan(ctx); err != nil {
					t.Fatal(err)
				}
				tid, bindings := team, []uuid.UUID{u, successor}
				if k.WorkspaceID == other.ID {
					tid, bindings = otherTeam.ID, []uuid.UUID{u}
				}
				if err := st.SetKeyBindings(ctx, k.ID, bindings, []uuid.UUID{tid}); err != nil {
					t.Fatal(err)
				}
			}
			entry := SpendEntry{WorkspaceID: workspace, KeyID: keys[0].ID, UserID: &u, Tokens: 17, CostMicros: 123}
			if _, err := st.DB.NewInsert().Model(&entry).Returning("*").Exec(ctx); err != nil {
				t.Fatal(err)
			}
			quota := ScopeQuota{WorkspaceID: workspace, ScopeType: "user", ScopeID: u, BudgetMicros: 900, SpentOffsetMicros: 12}
			if err := st.UpsertScopeQuota(ctx, &quota); err != nil {
				t.Fatal(err)
			}
			if failure == "sole-admin" {
				if err := st.SetTeamMemberRole(ctx, successor, team, "member"); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "late-delete" {
				if _, err := st.DB.ExecContext(ctx, `CREATE FUNCTION fail_offboarding() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected late deletion failure'; END $$;
					CREATE TRIGGER fail_offboarding BEFORE DELETE ON workspace_members FOR EACH ROW EXECUTE FUNCTION fail_offboarding()`); err != nil {
					t.Fatal(err)
				}
			}
			if failure != "none" {
				err := st.DeleteMembership(ctx, u, workspace)
				if err == nil || (failure == "sole-admin" && !errors.Is(err, ErrLastTeamAdmin)) {
					t.Fatalf("offboarding failure = %v", err)
				}
				if _, err := st.GetMembership(ctx, u, workspace); err != nil {
					t.Fatal("partial workspace cleanup:", err)
				}
				for _, tid := range []uuid.UUID{team, plainTeam.ID} {
					if _, err := st.GetTeamMember(ctx, u, tid); err != nil {
						t.Fatal("partial team cleanup:", err)
					}
				}
				for _, k := range keys[:2] {
					ids, err := st.ListKeyUserIDs(ctx, k.ID)
					if err != nil || len(ids) != 2 {
						t.Fatalf("partial share cleanup: %v %v", ids, err)
					}
				}
				if failure == "sole-admin" {
					if err := st.SetTeamMemberRole(ctx, successor, team, "admin"); err != nil {
						t.Fatal(err)
					}
				} else if _, err := st.DB.ExecContext(ctx, `DROP TRIGGER fail_offboarding ON workspace_members; DROP FUNCTION fail_offboarding()`); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.DeleteMembership(ctx, u, workspace); err != nil {
				t.Fatal(err)
			}
			assertClean := func() {
				t.Helper()
				for _, tid := range []uuid.UUID{team, plainTeam.ID} {
					if _, err := st.GetTeamMember(ctx, u, tid); !errors.Is(err, sql.ErrNoRows) {
						t.Errorf("team grant survived: %v", err)
					}
				}
				for _, k := range keys[:2] {
					ids, err := st.ListKeyUserIDs(ctx, k.ID)
					if err != nil || !reflect.DeepEqual(ids, []uuid.UUID{successor}) {
						t.Errorf("scoped shares = %v, %v", ids, err)
					}
				}
			}
			assertClean()
			if _, err := st.GetMembership(ctx, u, workspace); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("workspace membership survived: %v", err)
			}
			if err := st.AddMembership(ctx, &WorkspaceMember{UserID: u, WorkspaceID: workspace}); err != nil {
				t.Fatal(err)
			}
			assertClean()
			m, err := st.GetMembership(ctx, u, workspace)
			if err != nil || m.Role != "member" {
				t.Fatalf("readd = %+v %v", m, err)
			}
			m, err = st.GetMembership(ctx, u, other.ID)
			if err != nil || m.Role != "admin" {
				t.Fatalf("other workspace changed: %+v %v", m, err)
			}
			tm, err := st.GetTeamMember(ctx, u, otherTeam.ID)
			if err != nil || tm.Role != "admin" {
				t.Fatalf("other team changed: %+v %v", tm, err)
			}
			ids, err := st.ListKeyUserIDs(ctx, keys[2].ID)
			if err != nil || !reflect.DeepEqual(ids, []uuid.UUID{u}) {
				t.Fatalf("other shares changed: %v %v", ids, err)
			}
			for _, k := range keys {
				var got InboundKey
				if err := st.DB.NewSelect().Model(&got).Where("id = ?", k.ID).Scan(ctx); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, k) {
					t.Errorf("key/ownership/credential changed: %+v != %+v", got, k)
				}
				ids, err := st.ListKeyTeamIDs(ctx, k.ID)
				want := team
				if k.WorkspaceID == other.ID {
					want = otherTeam.ID
				}
				if err != nil || !reflect.DeepEqual(ids, []uuid.UUID{want}) {
					t.Fatalf("team sharing changed: %v %v", ids, err)
				}
			}
			var got SpendEntry
			if err := st.DB.NewSelect().Model(&got).Where("id = ?", entry.ID).Scan(ctx); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, entry) {
				t.Fatalf("history changed: %+v != %+v", got, entry)
			}
			q, err := st.GetScopeQuota(ctx, workspace, "user", u, "")
			if err != nil || q.BudgetMicros != 900 || q.SpentOffsetMicros != 12 {
				t.Fatalf("quota history changed: %+v %v", q, err)
			}
		})
	}
}

func TestOffboardingConcurrentTeamTransitions(t *testing.T) {
	for _, op := range []string{"demote", "remove", "offboard", "promote", "add-member", "add-admin"} {
		for _, first := range []string{"offboard", "transition"} {
			t.Run(op+"/"+first+"-first", func(t *testing.T) {
				st := openInviteTestStore(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				workspace, team, users := membershipFixture(t, st, "team")
				u, other := users[0].ID, users[1].ID
				if op == "promote" {
					if err := st.SetTeamMemberRole(ctx, u, team, "member"); err != nil {
						t.Fatal(err)
					}
				}
				if op == "add-member" || op == "add-admin" {
					if err := st.RemoveTeamMember(ctx, u, team); err != nil {
						t.Fatal(err)
					}
				}
				holder, err := st.DB.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer holder.Rollback()
				if err := lockMembershipWorkspace(ctx, holder, workspace); err != nil {
					t.Fatal(err)
				}
				offboard, transition := make(chan error, 1), make(chan error, 1)
				startOffboard := func() { go func() { offboard <- st.DeleteMembership(ctx, u, workspace) }() }
				startTransition := func() {
					go func() {
						var err error
						switch op {
						case "demote":
							err = st.SetTeamMemberRole(ctx, other, team, "member")
						case "remove":
							err = st.RemoveTeamMember(ctx, other, team)
						case "offboard":
							err = st.DeleteMembership(ctx, other, workspace)
						case "promote":
							err = st.SetTeamMemberRole(ctx, u, team, "admin")
						case "add-member":
							err = st.AddTeamMember(ctx, u, team, "member")
						case "add-admin":
							err = st.AddTeamMember(ctx, u, team, "admin")
						}
						transition <- err
					}()
				}
				if first == "offboard" {
					startOffboard()
				} else {
					startTransition()
				}
				waitMembershipLocks(t, ctx, st, workspace, 1)
				if first == "offboard" {
					startTransition()
				} else {
					startOffboard()
				}
				waitMembershipLocks(t, ctx, st, workspace, 2)
				if err := holder.Commit(); err != nil {
					t.Fatal(err)
				}
				a, b := <-offboard, <-transition
				if op == "demote" || op == "remove" || op == "offboard" {
					if !((a == nil && errors.Is(b, ErrLastTeamAdmin)) || (b == nil && errors.Is(a, ErrLastTeamAdmin))) {
						t.Fatalf("offboard=%v transition=%v", a, b)
					}
					if n, err := st.CountTeamAdmins(ctx, team); err != nil || n != 1 {
						t.Fatalf("admins=%d %v", n, err)
					}
				} else {
					if a != nil {
						t.Fatal(a)
					}
					if b != nil && !errors.Is(b, sql.ErrNoRows) && !errors.Is(b, ErrNotWorkspaceMember) {
						t.Fatal(b)
					}
				}
				for _, id := range []uuid.UUID{u, other} {
					_, workspaceErr := st.GetMembership(ctx, id, workspace)
					_, teamErr := st.GetTeamMember(ctx, id, team)
					if errors.Is(workspaceErr, sql.ErrNoRows) && !errors.Is(teamErr, sql.ErrNoRows) {
						t.Fatalf("orphaned team authority for %s: %v", id, teamErr)
					}
				}
			})
		}
	}
}
