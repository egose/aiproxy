package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func issuanceKey(workspace uuid.UUID) InboundKey {
	return InboundKey{Name: uuid.NewString(), TokenHash: uuid.NewString(), TokenPrefix: "fixture", WorkspaceID: workspace, Enabled: true, AllowedModels: []string{"local/model"}}
}

func assertNoIssuedKey(t *testing.T, st *Store, key InboundKey) {
	t.Helper()
	ctx := context.Background()
	if n, err := st.DB.NewSelect().Model((*InboundKey)(nil)).Where("name = ?", key.Name).Count(ctx); err != nil || n != 0 {
		t.Fatalf("rejected key persisted: count=%d err=%v", n, err)
	}
	for _, table := range []string{"key_users", "key_teams"} {
		var n int
		if err := st.DB.NewRaw("SELECT count(*) FROM "+table+" g LEFT JOIN inbound_keys k ON k.id = g.key_id WHERE k.id IS NULL").Scan(ctx, &n); err != nil || n != 0 {
			t.Fatalf("orphan grants: %s count=%d err=%v", table, n, err)
		}
	}
}

func TestKeyIssuanceRollback(t *testing.T) {
	for _, failure := range []string{"key-write", "user-write", "team-write", "nonmember", "foreign-team", "other-owner", "unowned", "not-team-admin", "disabled-actor", "deleted-actor", "two-owners"} {
		t.Run(failure, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx := context.Background()
			workspace, team, users := membershipFixture(t, st, "team")
			key := issuanceKey(workspace)
			key.OwnerTeamID = &team
			actor := users[0].ID
			boundUsers, boundTeams := []uuid.UUID{users[1].ID}, []uuid.UUID{team}
			var want error
			switch failure {
			case "key-write", "user-write", "team-write":
				table := map[string]string{"key-write": "inbound_keys", "user-write": "key_users", "team-write": "key_teams"}[failure]
				flowOK(t, execIssuanceConstraint(ctx, st, table, true))
				t.Cleanup(func() { flowOK(t, execIssuanceConstraint(ctx, st, table, false)) })
			case "nonmember":
				flowOK(t, st.DeleteMembership(ctx, users[1].ID, workspace))
				want = ErrInvalidKeyBinding
			case "foreign-team":
				foreign := WorkspaceTeam{WorkspaceID: SystemWorkspaceID, Name: uuid.NewString()}
				flowOK(t, st.CreateTeam(ctx, &foreign))
				boundTeams = []uuid.UUID{foreign.ID}
				want = ErrInvalidKeyBinding
			case "other-owner":
				key.OwnerTeamID, key.OwnerUserID = nil, &users[1].ID
				want = ErrKeyManagerRequired
			case "unowned":
				key.OwnerTeamID = nil
				want = ErrKeyManagerRequired
			case "not-team-admin":
				flowOK(t, st.SetTeamMemberRole(ctx, actor, team, "member"))
				want = ErrKeyManagerRequired
			case "disabled-actor":
				_, err := st.DB.NewUpdate().Model((*User)(nil)).Set("disabled = TRUE").Where("id = ?", actor).Exec(ctx)
				flowOK(t, err)
				want = ErrKeyManagerRequired
			case "deleted-actor":
				flowOK(t, st.DeleteUser(ctx, actor))
				want = ErrKeyManagerRequired
			case "two-owners":
				key.OwnerUserID = &actor
				want = ErrInvalidKeyBinding
			}
			before := key
			err := st.IssueInboundKey(ctx, &key, actor, boundUsers, boundTeams)
			if err == nil || (want != nil && !errors.Is(err, want)) {
				t.Fatalf("issue=%v want=%v", err, want)
			}
			if !reflect.DeepEqual(key, before) {
				t.Fatal("failed issuance published an identity")
			}
			assertNoIssuedKey(t, st, key)
		})
	}
}

func execIssuanceConstraint(ctx context.Context, st *Store, table string, add bool) error {
	query := "ALTER TABLE " + table + " DROP CONSTRAINT issuance_fault"
	if add {
		query = "ALTER TABLE " + table + " ADD CONSTRAINT issuance_fault CHECK (false) NOT VALID"
	}
	_, err := st.DB.ExecContext(ctx, query)
	return err
}

func TestKeyIssuanceExactGrants(t *testing.T) {
	for _, owner := range []string{"user", "team", "workspace", "global"} {
		t.Run(owner, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx := context.Background()
			workspace, team, users := membershipFixture(t, st, "workspace")
			key := issuanceKey(workspace)
			actor := users[0].ID
			boundUsers := []uuid.UUID{users[1].ID, users[1].ID}
			boundTeams := []uuid.UUID{team, team}
			wantUsers := []uuid.UUID{users[1].ID}
			switch owner {
			case "user":
				key.OwnerUserID = &actor
				wantUsers = uniqueKeyBindingIDs(append(wantUsers, actor))
				flowOK(t, st.SetMembershipRole(ctx, users[1].ID, actor, workspace, "member"))
			case "team":
				key.OwnerTeamID = &team
				boundTeams = nil
			case "global":
				flowOK(t, st.DeleteMembership(ctx, actor, workspace))
				_, err := st.DB.NewUpdate().Model((*User)(nil)).Set("is_admin = TRUE").Where("id = ?", actor).Exec(ctx)
				flowOK(t, err)
			}
			flowOK(t, st.IssueInboundKey(ctx, &key, actor, boundUsers, boundTeams))
			saved := readPolicyKey(t, st, key.ID)
			gotUsers, err := st.ListKeyUserIDs(ctx, key.ID)
			flowOK(t, err)
			gotTeams, err := st.ListKeyTeamIDs(ctx, key.ID)
			flowOK(t, err)
			if !reflect.DeepEqual(uniqueKeyBindingIDs(gotUsers), wantUsers) || !reflect.DeepEqual(gotTeams, []uuid.UUID{team}) || saved.WorkspaceID != workspace || !reflect.DeepEqual(saved.OwnerUserID, key.OwnerUserID) || !reflect.DeepEqual(saved.OwnerTeamID, key.OwnerTeamID) || saved.TokenHash != key.TokenHash {
				t.Fatalf("wrong aggregate: %+v users=%v teams=%v", saved, gotUsers, gotTeams)
			}
			duplicate := issuanceKey(workspace)
			duplicate.Name, duplicate.OwnerUserID, duplicate.OwnerTeamID = key.Name, key.OwnerUserID, key.OwnerTeamID
			if err := st.IssueInboundKey(ctx, &duplicate, actor, boundUsers, boundTeams); !errors.Is(err, ErrKeyNameExists) {
				t.Fatalf("duplicate=%v", err)
			}
			if !reflect.DeepEqual(saved, readPolicyKey(t, st, key.ID)) || duplicate.ID != uuid.Nil {
				t.Fatal("duplicate changed original or published identity")
			}
		})
	}
}

func TestKeyIssuanceRechecksAfterWorkspaceLock(t *testing.T) {
	for _, mutation := range []string{"recipient-offboard", "owner-offboard", "actor-offboard", "actor-demote", "team-delete", "workspace-delete"} {
		t.Run(mutation, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			workspace, team, users := membershipFixture(t, st, "team")
			key := issuanceKey(workspace)
			key.OwnerTeamID = &team
			if mutation == "owner-offboard" {
				key.OwnerTeamID, key.OwnerUserID = nil, &users[1].ID
				_, err := st.DB.NewUpdate().Model((*User)(nil)).Set("is_admin = TRUE").Where("id = ?", users[0].ID).Exec(ctx)
				flowOK(t, err)
			}
			holder, err := st.DB.BeginTx(ctx, nil)
			flowOK(t, err)
			defer holder.Rollback()
			flowOK(t, lockMembershipWorkspace(ctx, holder, workspace))
			result := make(chan error, 1)
			go func() { result <- st.IssueInboundKey(ctx, &key, users[0].ID, []uuid.UUID{users[1].ID}, nil) }()
			waitMembershipLocks(t, ctx, st, workspace, 1)
			switch mutation {
			case "recipient-offboard", "owner-offboard", "actor-offboard":
				id := users[1].ID
				if mutation == "actor-offboard" {
					id = users[0].ID
				}
				_, err = holder.NewDelete().Model((*WorkspaceMember)(nil)).Where("user_id = ? AND workspace_id = ?", id, workspace).Exec(ctx)
			case "actor-demote":
				_, err = holder.NewUpdate().Model((*TeamMember)(nil)).Set("role = 'member'").Where("user_id = ? AND team_id = ?", users[0].ID, team).Exec(ctx)
			case "team-delete":
				_, err = holder.NewDelete().Model((*WorkspaceTeam)(nil)).Where("id = ?", team).Exec(ctx)
			case "workspace-delete":
				_, err = holder.NewDelete().Model((*Workspace)(nil)).Where("id = ?", workspace).Exec(ctx)
			}
			flowOK(t, err)
			flowOK(t, holder.Commit())
			if err := <-result; err == nil {
				t.Fatal("stale issuance succeeded")
			}
			assertNoIssuedKey(t, st, key)
		})
	}
}

func TestKeyIssuanceUserDeletionLockOrder(t *testing.T) {
	for _, first := range []string{"delete", "issue"} {
		t.Run(first, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			workspace, team, users := membershipFixture(t, st, "team")
			key := issuanceKey(workspace)
			key.OwnerTeamID = &team
			holder, err := st.DB.BeginTx(ctx, nil)
			flowOK(t, err)
			defer holder.Rollback()
			flowOK(t, lockMembershipWorkspace(ctx, holder, workspace))
			deletion, issuance := make(chan error, 1), make(chan error, 1)
			deleteUser := func() { deletion <- st.DeleteUser(ctx, users[1].ID) }
			issue := func() { issuance <- st.IssueInboundKey(ctx, &key, users[0].ID, []uuid.UUID{users[1].ID}, nil) }
			if first == "delete" {
				go deleteUser()
			} else {
				go issue()
			}
			waitMembershipLocks(t, ctx, st, workspace, 1)
			if first == "delete" {
				go issue()
			} else {
				go deleteUser()
			}
			waitMembershipLocks(t, ctx, st, users[1].ID, 1)
			flowOK(t, holder.Commit())
			flowOK(t, <-deletion)
			want := error(nil)
			if first == "delete" {
				want = ErrInvalidKeyBinding
			}
			if err := <-issuance; !errors.Is(err, want) {
				t.Fatalf("issuance=%v want=%v", err, want)
			}
			if first == "delete" {
				assertNoIssuedKey(t, st, key)
			} else {
				ids, err := st.ListKeyUserIDs(ctx, key.ID)
				flowOK(t, err)
				if len(ids) != 0 {
					t.Fatal("deleted recipient grant survived")
				}
			}
		})
	}
}

func TestKeyIssuanceSerializesWithOffboarding(t *testing.T) {
	for _, first := range []string{"offboard", "issue"} {
		t.Run(first, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			workspace, team, users := membershipFixture(t, st, "team")
			key := issuanceKey(workspace)
			key.OwnerTeamID = &team
			holder, err := st.DB.BeginTx(ctx, nil)
			flowOK(t, err)
			defer holder.Rollback()
			flowOK(t, lockMembershipWorkspace(ctx, holder, workspace))
			offboarding, issuance := make(chan error, 1), make(chan error, 1)
			offboard := func() { offboarding <- st.DeleteMembership(ctx, users[1].ID, workspace) }
			issue := func() { issuance <- st.IssueInboundKey(ctx, &key, users[0].ID, []uuid.UUID{users[1].ID}, nil) }
			if first == "offboard" {
				go offboard()
			} else {
				go issue()
			}
			waitMembershipLocks(t, ctx, st, workspace, 1)
			if first == "offboard" {
				go issue()
			} else {
				go offboard()
			}
			waitMembershipLocks(t, ctx, st, workspace, 2)
			flowOK(t, holder.Commit())
			flowOK(t, <-offboarding)
			want := error(nil)
			if first == "offboard" {
				want = ErrInvalidKeyBinding
			}
			if err := <-issuance; !errors.Is(err, want) {
				t.Fatalf("issuance=%v want=%v", err, want)
			}
			if first == "offboard" {
				assertNoIssuedKey(t, st, key)
			} else {
				ids, err := st.ListKeyUserIDs(ctx, key.ID)
				flowOK(t, err)
				if len(ids) != 0 || readPolicyKey(t, st, key.ID).OwnerTeamID == nil {
					t.Fatal("offboarding left a recipient grant or changed ownership")
				}
			}
		})
	}
}
