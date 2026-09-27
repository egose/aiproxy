package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

func openInviteTestStore(t *testing.T) *Store {
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
	schema := "invite_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
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

func createAcceptanceInvite(t *testing.T, st *Store) Invite {
	t.Helper()
	inv := Invite{Email: uuid.NewString() + "@example.com", TokenHash: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour), WorkspaceID: &SystemWorkspaceID, WorkspaceRole: "member"}
	if err := st.CreateInvite(context.Background(), &inv); err != nil {
		t.Fatal(err)
	}
	return inv
}

func assertNoInviteAccount(t *testing.T, st *Store, email string) {
	t.Helper()
	ctx := context.Background()
	if _, err := st.GetUserByEmail(ctx, email); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("account lookup = %v, want no account", err)
	}
	count, err := st.DB.NewSelect().Model((*WorkspaceMember)(nil)).Count(ctx)
	if err != nil || count != 0 {
		t.Errorf("membership count = %d, %v; want 0", count, err)
	}
}

func TestAcceptInviteRejectsStaleEligibility(t *testing.T) {
	for _, mutation := range []string{"revoked", "expired", "accepted", "token replaced"} {
		t.Run(mutation, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx := context.Background()
			inv := createAcceptanceInvite(t, st)
			var err error
			switch mutation {
			case "revoked":
				err = st.DeleteInvite(ctx, inv.ID)
			case "expired":
				_, err = st.DB.ExecContext(ctx, `UPDATE invites SET expires_at = clock_timestamp() - interval '1 second' WHERE id = ?`, inv.ID)
			case "accepted":
				_, err = st.DB.ExecContext(ctx, `UPDATE invites SET accepted_at = clock_timestamp() WHERE id = ?`, inv.ID)
			case "token replaced":
				_, err = st.DB.ExecContext(ctx, `UPDATE invites SET token_hash = ? WHERE id = ?`, uuid.NewString(), inv.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			u := User{Email: inv.Email, PasswordHash: "unused"}
			if err := st.AcceptInvite(ctx, &inv, &u); err == nil {
				t.Error("stale invitation accepted")
			}
			assertNoInviteAccount(t, st, inv.Email)
		})
	}
}

func TestAcceptInviteUsesCurrentAuthority(t *testing.T) {
	for _, role := range []string{"admin", "member", "", "obsolete"} {
		t.Run("workspace role "+role, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx := context.Background()
			inv := createAcceptanceInvite(t, st)
			workspace := Workspace{Name: "current"}
			if err := st.CreateWorkspace(ctx, &workspace); err != nil {
				t.Fatal(err)
			}
			currentEmail := uuid.NewString() + "@example.com"
			if _, err := st.DB.ExecContext(ctx, `UPDATE invites SET email = ?, is_admin = false, workspace_id = ?, workspace_role = ? WHERE id = ?`, currentEmail, workspace.ID, role, inv.ID); err != nil {
				t.Fatal(err)
			}
			inv.IsAdmin = true
			inv.WorkspaceRole = "admin"
			u := User{Email: inv.Email, IsAdmin: true, PasswordHash: "chosen-password-hash"}
			if err := st.AcceptInvite(ctx, &inv, &u); err != nil {
				t.Fatal(err)
			}
			stored, err := st.GetUserByID(ctx, u.ID)
			if err != nil || stored.Email != currentEmail || stored.IsAdmin || stored.PasswordHash != "chosen-password-hash" {
				t.Fatalf("account not derived from current invitation: %+v, %v", stored, err)
			}
			members, err := st.ListMembershipsByUser(ctx, u.ID)
			wantRole := "member"
			if role == "admin" {
				wantRole = "admin"
			}
			if err != nil || len(members) != 1 || members[0].WorkspaceID != workspace.ID || members[0].Role != wantRole {
				t.Fatalf("current membership = %+v, %v", members, err)
			}
			if inv.Email != currentEmail || inv.IsAdmin || inv.WorkspaceID == nil || *inv.WorkspaceID != workspace.ID || inv.AcceptedAt == nil {
				t.Errorf("returned invitation is stale: %+v", inv)
			}
		})
	}
	t.Run("system admin without workspaceanization", func(t *testing.T) {
		st := openInviteTestStore(t)
		ctx := context.Background()
		inv := createAcceptanceInvite(t, st)
		if _, err := st.DB.ExecContext(ctx, `UPDATE invites SET is_admin = true, workspace_id = NULL WHERE id = ?`, inv.ID); err != nil {
			t.Fatal(err)
		}
		u := User{Email: inv.Email, PasswordHash: "unused"}
		if err := st.AcceptInvite(ctx, &inv, &u); err != nil {
			t.Fatal(err)
		}
		stored, err := st.GetUserByID(ctx, u.ID)
		if err != nil || !stored.IsAdmin {
			t.Errorf("system authority = %+v, %v", stored, err)
		}
		members, err := st.ListMembershipsByUser(ctx, u.ID)
		if err != nil || len(members) != 0 {
			t.Errorf("system invite memberships = %+v, %v", members, err)
		}
	})
}

func TestAcceptInviteConcurrentSingleConsumption(t *testing.T) {
	st := openInviteTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	inv := createAcceptanceInvite(t, st)
	const attempts = 8
	start := make(chan struct{})
	results := make(chan error, attempts)
	for range attempts {
		go func() {
			snapshot := inv
			u := User{Email: uuid.NewString() + "@example.com", PasswordHash: "unused"}
			<-start
			results <- st.AcceptInvite(ctx, &snapshot, &u)
		}()
	}
	close(start)
	successes := 0
	for range attempts {
		if err := <-results; err == nil {
			successes++
		} else if !errors.Is(err, ErrInviteUnavailable) {
			t.Errorf("competing acceptance error = %v", err)
		}
	}
	if successes != 1 {
		t.Errorf("successful accepts = %d, want exactly 1", successes)
	}
	users, err := st.ListUsers(ctx)
	if err != nil || len(users) != 1 || users[0].Email != inv.Email {
		t.Fatalf("accounts = %+v, %v; want exactly the invited account", users, err)
	}
	members, err := st.ListMembershipsByUser(ctx, users[0].ID)
	if err != nil || len(members) != 1 || members[0].WorkspaceID != SystemWorkspaceID {
		t.Errorf("memberships = %+v, %v", members, err)
	}
	current, err := st.GetInviteByHash(ctx, inv.TokenHash)
	if err != nil || current.AcceptedAt == nil {
		t.Errorf("invitation not consumed: %+v, %v", current, err)
	}
}

func TestAcceptInviteRollbackAndRetry(t *testing.T) {
	for _, failure := range []string{"account", "membership"} {
		t.Run(failure, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx := context.Background()
			inv := createAcceptanceInvite(t, st)
			table := "users"
			if failure == "membership" {
				table = "workspace_members"
			}
			if _, err := st.DB.ExecContext(ctx, `ALTER TABLE `+table+` ADD CONSTRAINT life01_fail CHECK (false) NOT VALID`); err != nil {
				t.Fatal(err)
			}
			u := User{Email: inv.Email, PasswordHash: "unused"}
			if err := st.AcceptInvite(ctx, &inv, &u); err == nil {
				t.Error("injected write failure succeeded")
			}
			assertNoInviteAccount(t, st, inv.Email)
			current, err := st.GetInviteByHash(ctx, inv.TokenHash)
			if err != nil || current.AcceptedAt != nil {
				t.Fatalf("failed acceptance consumed invite: %+v, %v", current, err)
			}
			if inv.AcceptedAt != nil || u.ID != uuid.Nil {
				t.Error("failed acceptance published uncommitted output")
			}
			if _, err := st.DB.ExecContext(ctx, `ALTER TABLE `+table+` DROP CONSTRAINT life01_fail`); err != nil {
				t.Fatal(err)
			}
			if err := st.AcceptInvite(ctx, &inv, &u); err != nil {
				t.Fatalf("retry: %v", err)
			}
			if _, err := st.GetMembership(ctx, u.ID, SystemWorkspaceID); err != nil {
				t.Fatalf("retry membership: %v", err)
			}
		})
	}
}

func TestAcceptInviteRechecksAfterLockWait(t *testing.T) {
	for _, mutation := range []string{"revoked", "expired"} {
		t.Run(mutation, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			inv := createAcceptanceInvite(t, st)
			holder, err := st.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback()
			var pid int
			if err := holder.NewRaw(`SELECT pg_backend_pid()`).Scan(ctx, &pid); err != nil {
				t.Fatal(err)
			}
			if _, err := holder.ExecContext(ctx, `SELECT id FROM invites WHERE id = ? FOR UPDATE`, inv.ID); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				snapshot := inv
				u := User{Email: inv.Email, PasswordHash: "unused"}
				result <- st.AcceptInvite(ctx, &snapshot, &u)
			}()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				var waiting bool
				if err := st.DB.NewRaw(`SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE ? = ANY(pg_blocking_pids(pid)))`, pid).Scan(ctx, &waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("acceptance did not wait for invitation lock")
				case <-ticker.C:
				}
			}
			wantErr := ErrInviteUnavailable
			if mutation == "revoked" {
				_, err = holder.ExecContext(ctx, `DELETE FROM invites WHERE id = ?`, inv.ID)
				wantErr = sql.ErrNoRows
			} else {
				_, err = holder.ExecContext(ctx, `UPDATE invites SET expires_at = clock_timestamp() WHERE id = ?`, inv.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-result; !errors.Is(err, wantErr) {
				t.Errorf("accept after lock wait = %v, want %v", err, wantErr)
			}
			assertNoInviteAccount(t, st, inv.Email)
		})
	}
}

type inviteLockedHook struct {
	fired   atomic.Bool
	ready   chan struct{}
	release chan struct{}
}

func (h *inviteLockedHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (h *inviteLockedHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	if strings.HasPrefix(event.Query, "SELECT") && strings.Contains(event.Query, `"invites"`) && strings.Contains(event.Query, "FOR UPDATE") && h.fired.CompareAndSwap(false, true) {
		close(h.ready)
		select {
		case <-h.release:
		case <-ctx.Done():
		}
	}
}

func TestAcceptInviteWorkspaceDeletionLockOrder(t *testing.T) {
	st := openInviteTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	workspace := Workspace{Name: uuid.NewString()}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	inv := Invite{Email: uuid.NewString() + "@example.com", TokenHash: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour), WorkspaceID: &workspace.ID}
	if err := st.CreateInvite(ctx, &inv); err != nil {
		t.Fatal(err)
	}
	hook := &inviteLockedHook{ready: make(chan struct{}), release: make(chan struct{})}
	st.DB.AddQueryHook(hook)
	accepted, deleted := make(chan error, 1), make(chan error, 1)
	u := User{PasswordHash: "unused"}
	go func() { accepted <- st.AcceptInvite(ctx, &inv, &u) }()
	select {
	case <-hook.ready:
	case <-ctx.Done():
		t.Fatal("acceptance did not lock invitation")
	}
	go func() { deleted <- st.DeleteWorkspace(ctx, workspace.ID) }()
	waitMembershipLocks(t, ctx, st, workspace.ID, 1)
	close(hook.release)
	if err := <-accepted; err != nil {
		t.Errorf("acceptance: %v", err)
	}
	if err := <-deleted; err != nil {
		t.Errorf("workspaceanization deletion: %v", err)
	}
	if t.Failed() {
		return
	}
	if _, err := st.GetUserByID(ctx, u.ID); err != nil {
		t.Fatalf("committed account missing: %v", err)
	}
	if _, err := st.GetWorkspace(ctx, workspace.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("workspaceanization deletion did not commit: %v", err)
	}
	if _, err := st.GetInviteByHash(ctx, inv.TokenHash); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("invitation cascade: %v", err)
	}
	if _, err := st.GetMembership(ctx, u.ID, workspace.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("membership cascade: %v", err)
	}
}

type inviteScopeChangeHook struct {
	fired  atomic.Bool
	change func()
}

func (h *inviteScopeChangeHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	if strings.HasPrefix(event.Query, "SELECT") && strings.Contains(event.Query, `"invites"`) && strings.Contains(event.Query, "FOR UPDATE") && h.fired.CompareAndSwap(false, true) {
		h.change()
	}
	return ctx
}

func (h *inviteScopeChangeHook) AfterQuery(context.Context, *bun.QueryEvent) {}

func TestAcceptInviteScopeChangeDuringLockAcquisition(t *testing.T) {
	for _, change := range []string{"workspace-to-workspace", "workspace-to-global", "global-to-workspace"} {
		t.Run(change, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx := context.Background()
			inv := createAcceptanceInvite(t, st)
			if change == "global-to-workspace" {
				if _, err := st.DB.ExecContext(ctx, "UPDATE invites SET workspace_id = NULL WHERE id = ?", inv.ID); err != nil {
					t.Fatal(err)
				}
			}
			workspace := Workspace{Name: uuid.NewString()}
			if err := st.CreateWorkspace(ctx, &workspace); err != nil {
				t.Fatal(err)
			}
			var target *uuid.UUID
			if change != "workspace-to-global" {
				target = &workspace.ID
			}
			hook := &inviteScopeChangeHook{change: func() {
				if _, err := st.DB.ExecContext(ctx, "UPDATE invites SET workspace_id = ? WHERE id = ?", target, inv.ID); err != nil {
					t.Fatal(err)
				}
			}}
			st.DB.AddQueryHook(hook)
			u := User{PasswordHash: "unused"}
			if err := st.AcceptInvite(ctx, &inv, &u); !errors.Is(err, ErrInviteChanged) {
				t.Fatalf("scope change = %v", err)
			}
			assertNoInviteAccount(t, st, inv.Email)
			current, err := st.GetInviteByHash(ctx, inv.TokenHash)
			if err != nil || current.AcceptedAt != nil || u.ID != uuid.Nil || inv.AcceptedAt != nil {
				t.Fatalf("rejection published or consumed state: %+v %+v %v", current, u, err)
			}
			if err := st.AcceptInvite(ctx, &inv, &u); err != nil {
				t.Fatalf("retry: %v", err)
			}
			members, err := st.ListMembershipsByUser(ctx, u.ID)
			if err != nil || (target == nil && len(members) != 0) || (target != nil && (len(members) != 1 || members[0].WorkspaceID != *target)) {
				t.Fatalf("retry membership scope = %+v %v", members, err)
			}
		})
	}
}
