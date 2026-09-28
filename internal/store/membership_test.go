package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func membershipFixture(t *testing.T, st *Store, scope string) (uuid.UUID, uuid.UUID, []User) {
	t.Helper()
	ctx := context.Background()
	workspace := Workspace{Name: uuid.NewString()}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	team := WorkspaceTeam{WorkspaceID: workspace.ID, Name: "team"}
	if err := st.CreateTeam(ctx, &team); err != nil {
		t.Fatal(err)
	}
	users := make([]User, 2)
	for i := range users {
		users[i] = User{Email: uuid.NewString() + "@example.com", PasswordHash: "unused"}
		if err := st.CreateUser(ctx, &users[i]); err != nil {
			t.Fatal(err)
		}
		role := "admin"
		if scope == "team" {
			role = "member"
		}
		if err := st.AddMembership(ctx, &WorkspaceMember{WorkspaceID: workspace.ID, UserID: users[i].ID, Role: role}); err != nil {
			t.Fatal(err)
		}
		if scope == "team" {
			if err := st.AddTeamMember(ctx, users[i].ID, team.ID, "admin"); err != nil {
				t.Fatal(err)
			}
		}
	}
	return workspace.ID, team.ID, users
}

func waitMembershipLocks(t *testing.T, ctx context.Context, st *Store, workspaceID uuid.UUID, count int) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		if err := st.DB.NewRaw(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()
			AND cardinality(pg_blocking_pids(pid)) > 0 AND query LIKE ?`, "%"+workspaceID.String()+"%").Scan(ctx, &waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= count {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wanted %d blocked membership transitions, got %d", count, waiting)
		case <-ticker.C:
		}
	}
}

func TestMembershipConcurrentTransitions(t *testing.T) {
	for _, scope := range []string{"workspace", "team"} {
		for _, ops := range [][2]string{{"demote", "demote"}, {"demote", "remove"}, {"remove", "remove"}, {"delete-user", "demote"}, {"delete-user", "remove"}, {"delete-user", "delete-user"}} {
			t.Run(scope+"/"+ops[0]+"+"+ops[1], func(t *testing.T) {
				st := openInviteTestStore(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				workspaceID, teamID, users := membershipFixture(t, st, scope)
				holder, err := st.DB.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer holder.Rollback()
				if err := lockMembershipWorkspace(ctx, holder, workspaceID); err != nil {
					t.Fatal(err)
				}
				results := make(chan error, 2)
				for i, op := range ops {
					go func(i int, op string) {
						id := users[i].ID
						var err error
						if op == "delete-user" {
							err = st.DeleteUser(ctx, id)
						} else if scope == "workspace" {
							if op == "demote" {
								err = st.SetMembershipRole(ctx, uuid.New(), id, workspaceID, "member")
							} else {
								err = st.DeleteMembership(ctx, id, workspaceID)
							}
						} else if op == "demote" {
							err = st.SetTeamMemberRole(ctx, id, teamID, "member")
						} else {
							err = st.RemoveTeamMember(ctx, id, teamID)
						}
						results <- err
					}(i, op)
				}
				waitMembershipLocks(t, ctx, st, workspaceID, 2)
				if err := holder.Commit(); err != nil {
					t.Fatal(err)
				}
				wantErr := ErrLastWorkspaceAdmin
				if scope == "team" {
					wantErr = ErrLastTeamAdmin
				}
				succeeded, rejected := 0, 0
				for range 2 {
					err := <-results
					if err == nil {
						succeeded++
					} else if errors.Is(err, wantErr) {
						rejected++
					} else {
						t.Errorf("unexpected transition error: %v", err)
					}
				}
				if succeeded != 1 || rejected != 1 {
					t.Fatalf("successes=%d invariant rejections=%d", succeeded, rejected)
				}
				count, err := st.CountWorkspaceAdmins(ctx, workspaceID)
				if scope == "team" {
					count, err = st.CountTeamAdmins(ctx, teamID)
				}
				if err != nil || count != 1 {
					t.Fatalf("remaining admins=%d, %v", count, err)
				}
				for _, user := range users {
					m, err := st.GetMembership(ctx, user.ID, workspaceID)
					role := m.Role
					if scope == "team" {
						tm, teamErr := st.GetTeamMember(ctx, user.ID, teamID)
						role, err = tm.Role, teamErr
					}
					if err == nil && role == "admin" {
						if _, err := st.GetUserByID(ctx, user.ID); err != nil {
							t.Fatalf("surviving admin account missing: %v", err)
						}
						if _, err := st.GetMembership(ctx, user.ID, workspaceID); err != nil {
							t.Fatalf("surviving admin workspace access missing: %v", err)
						}
					}
				}
			})
		}
	}
}

func TestMembershipInsertionAndRoleContracts(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	workspaceID, teamID, users := membershipFixture(t, st, "team")
	u := users[0].ID
	for _, role := range []string{"", "member", "admin"} {
		if err := st.AddTeamMember(ctx, u, teamID, role); !errors.Is(err, ErrMembershipExists) {
			t.Fatalf("duplicate team role %q: %v", role, err)
		}
		if err := st.AddMembership(ctx, &WorkspaceMember{UserID: u, WorkspaceID: workspaceID, Role: role}); !errors.Is(err, ErrMembershipExists) {
			t.Fatalf("duplicate workspace role %q: %v", role, err)
		}
	}
	if err := st.SetMembershipRole(ctx, users[1].ID, u, workspaceID, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMembershipRole(ctx, u, u, workspaceID, "member"); !errors.Is(err, ErrSelfDemotion) {
		t.Fatalf("self demotion: %v", err)
	}
	if err := st.SetMembershipRole(ctx, users[1].ID, u, workspaceID, "member"); !errors.Is(err, ErrLastWorkspaceAdmin) {
		t.Fatalf("last workspace admin: %v", err)
	}
	if err := st.SetTeamMemberRole(ctx, u, teamID, "member"); err != nil {
		t.Fatalf("team self demotion with successor: %v", err)
	}
	if err := st.SetTeamMemberRole(ctx, u, teamID, "admin"); err != nil {
		t.Fatalf("team promotion: %v", err)
	}
	for _, err := range []error{
		st.SetMembershipRole(ctx, u, uuid.New(), workspaceID, "admin"),
		st.SetTeamMemberRole(ctx, uuid.New(), teamID, "admin"),
		st.RemoveTeamMember(ctx, uuid.New(), teamID),
		st.DeleteMembership(ctx, uuid.New(), workspaceID),
	} {
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("missing membership: %v", err)
		}
	}
	if err := st.addSystemMembership(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := st.addSystemMembership(ctx, u); err != nil {
		t.Fatalf("bootstrap is not idempotent: %v", err)
	}
	m, err := st.GetMembership(ctx, u, workspaceID)
	if err != nil || m.Role != "admin" {
		t.Fatalf("workspace role=%q, %v", m.Role, err)
	}
	tm, err := st.GetTeamMember(ctx, u, teamID)
	if err != nil || tm.Role != "admin" {
		t.Fatalf("team role=%q, %v", tm.Role, err)
	}
}

func TestTeamAdditionRechecksMembershipAfterLockWait(t *testing.T) {
	st := openInviteTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	workspaceID, teamID, users := membershipFixture(t, st, "workspace")
	holder, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()
	if err := lockMembershipWorkspace(ctx, holder, workspaceID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- st.AddTeamMember(ctx, users[0].ID, teamID, "admin") }()
	waitMembershipLocks(t, ctx, st, workspaceID, 1)
	if _, err := holder.ExecContext(ctx, `DELETE FROM workspace_members WHERE user_id = ? AND workspace_id = ?`, users[0].ID, workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ErrNotWorkspaceMember) {
		t.Fatalf("stale team addition: %v", err)
	}
	if n, err := st.CountTeamAdmins(ctx, teamID); err != nil || n != 0 {
		t.Fatalf("team admins=%d, %v", n, err)
	}
}

func TestMembershipTransitionRollback(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	workspaceID, _, users := membershipFixture(t, st, "workspace")
	if _, err := st.DB.ExecContext(ctx, `ALTER TABLE workspace_members ADD CONSTRAINT life02_fail CHECK (role <> 'member') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMembershipRole(ctx, users[1].ID, users[0].ID, workspaceID, "member"); err == nil {
		t.Fatal("injected role failure succeeded")
	}
	if n, err := st.CountWorkspaceAdmins(ctx, workspaceID); err != nil || n != 2 {
		t.Fatalf("failed mutation changed admins=%d, %v", n, err)
	}
	if _, err := st.DB.ExecContext(ctx, `ALTER TABLE workspace_members DROP CONSTRAINT life02_fail`); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMembershipRole(ctx, users[1].ID, users[0].ID, workspaceID, "member"); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestPersonalWorkspaceRules(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	user := User{Email: uuid.NewString() + "@example.com", PasswordHash: "unused"}
	if err := st.CreateUser(ctx, &user); err != nil {
		t.Fatal(err)
	}
	personal := Workspace{Name: uuid.NewString()}
	if err := st.CreatePersonalWorkspace(ctx, user.ID, &personal); err != nil {
		t.Fatal(err)
	}
	stored, err := st.GetWorkspace(ctx, personal.ID)
	if err != nil || stored.Kind != WorkspaceKindPersonal {
		t.Fatalf("personal kind = %+v, %v", stored, err)
	}
	members, err := st.ListMembershipsByWorkspace(ctx, personal.ID)
	if err != nil || len(members) != 1 || members[0].UserID != user.ID || members[0].Role != "admin" {
		t.Fatalf("personal membership = %+v, %v", members, err)
	}
	again := Workspace{Name: uuid.NewString()}
	if err := st.CreatePersonalWorkspace(ctx, user.ID, &again); !errors.Is(err, ErrPersonalWorkspaceExists) {
		t.Fatalf("second personal workspace: %v", err)
	}
	if err := st.CreateTeam(ctx, &WorkspaceTeam{WorkspaceID: personal.ID, Name: "team"}); !errors.Is(err, ErrPersonalWorkspaceTeams) {
		t.Fatalf("personal team: %v", err)
	}
	other := User{Email: uuid.NewString() + "@example.com", PasswordHash: "unused"}
	if err := st.CreateUser(ctx, &other); err != nil {
		t.Fatal(err)
	}
	if err := st.AddMembership(ctx, &WorkspaceMember{UserID: other.ID, WorkspaceID: personal.ID, Role: "member"}); !errors.Is(err, ErrPersonalWorkspaceMembers) {
		t.Fatalf("extra personal membership: %v", err)
	}
	registered := User{Email: uuid.NewString() + "@example.com", PasswordHash: "unused"}
	provisioned := Workspace{Name: uuid.NewString(), Kind: WorkspaceKindOrganization}
	if err := st.CreateUserWithWorkspace(ctx, &registered, &provisioned, "admin"); err != nil {
		t.Fatal(err)
	}
	if provisioned.Kind != WorkspaceKindPersonal {
		t.Fatalf("registration kind = %q", provisioned.Kind)
	}
	plain := Workspace{Name: uuid.NewString()}
	if err := st.CreateWorkspace(ctx, &plain); err != nil {
		t.Fatal(err)
	}
	if plain.Kind != WorkspaceKindOrganization {
		t.Fatalf("default kind = %q", plain.Kind)
	}
	bad := Workspace{Name: uuid.NewString(), Kind: "team"}
	if err := st.CreateWorkspace(ctx, &bad); !errors.Is(err, ErrInvalidWorkspaceKind) {
		t.Fatalf("invalid kind: %v", err)
	}
}
