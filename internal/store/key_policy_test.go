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

func policyFixture(t *testing.T, st *Store) (InboundKey, []User, uuid.UUID) {
	t.Helper()
	workspace, team, users := membershipFixture(t, st, "team")
	k := InboundKey{Name: uuid.NewString(), TokenHash: uuid.NewString(), TokenPrefix: "original", WorkspaceID: workspace, Enabled: true, Description: "original", Tenant: "original", AllowedModels: []string{"old/model"}, OwnerTeamID: &team}
	if err := st.CreateInboundKey(context.Background(), &k); err != nil {
		t.Fatal(err)
	}
	if err := st.SetKeyBindings(context.Background(), k.ID, []uuid.UUID{users[0].ID}, []uuid.UUID{team}); err != nil {
		t.Fatal(err)
	}
	return k, users, team
}

func readPolicyKey(t *testing.T, st *Store, id uuid.UUID) InboundKey {
	t.Helper()
	var k InboundKey
	if err := st.DB.NewSelect().Model(&k).Where("id = ?", id).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestKeyPolicyRollbackAndReplacement(t *testing.T) {
	for _, failure := range []string{"user-write", "team-write", "nonmember", "cross-workspace"} {
		t.Run(failure, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx := context.Background()
			k, users, team := policyFixture(t, st)
			before := readPolicyKey(t, st, k.ID)
			value := "changed"
			expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
			patch := InboundKeyPolicyPatch{Description: &value, Tenant: &value, AllowedModels: []string{}, SetExpiry: true, ExpiresAt: &expiry, ReplaceBindings: true, UserIDs: []uuid.UUID{users[1].ID}, TeamIDs: []uuid.UUID{team}}
			var undo func()
			switch failure {
			case "user-write", "team-write":
				table := "key_users"
				if failure == "team-write" {
					table = "key_teams"
				}
				if _, err := st.DB.ExecContext(ctx, "ALTER TABLE "+table+" ADD CONSTRAINT policy_fault CHECK (false) NOT VALID"); err != nil {
					t.Fatal(err)
				}
				undo = func() {
					if _, err := st.DB.ExecContext(ctx, "ALTER TABLE "+table+" DROP CONSTRAINT policy_fault"); err != nil {
						t.Fatal(err)
					}
				}
			case "nonmember":
				if err := st.DeleteMembership(ctx, users[1].ID, k.WorkspaceID); err != nil {
					t.Fatal(err)
				}
			case "cross-workspace":
				workspace := Workspace{Name: uuid.NewString()}
				if err := st.CreateWorkspace(ctx, &workspace); err != nil {
					t.Fatal(err)
				}
				foreign := WorkspaceTeam{WorkspaceID: workspace.ID, Name: "foreign"}
				if err := st.CreateTeam(ctx, &foreign); err != nil {
					t.Fatal(err)
				}
				patch.TeamIDs = []uuid.UUID{foreign.ID}
			}
			if err := st.UpdateInboundKeyPolicy(ctx, k.ID, users[0].ID, patch); err == nil {
				t.Fatal("invalid update succeeded")
			}
			after := readPolicyKey(t, st, k.ID)
			boundUsers, err := st.ListKeyUserIDs(ctx, k.ID)
			if err != nil {
				t.Fatal(err)
			}
			boundTeams, err := st.ListKeyTeamIDs(ctx, k.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(boundUsers, []uuid.UUID{users[0].ID}) || !reflect.DeepEqual(boundTeams, []uuid.UUID{team}) {
				t.Fatalf("rollback failed: %+v %+v %v %v", before, after, boundUsers, boundTeams)
			}
			if undo != nil {
				undo()
				if err := st.UpdateInboundKeyPolicy(ctx, k.ID, users[0].ID, patch); err != nil {
					t.Fatal(err)
				}
				after = readPolicyKey(t, st, k.ID)
				boundUsers, _ = st.ListKeyUserIDs(ctx, k.ID)
				boundTeams, _ = st.ListKeyTeamIDs(ctx, k.ID)
				if after.Description != value || after.Tenant != value || len(after.AllowedModels) != 0 || after.ExpiresAt == nil || !after.ExpiresAt.Equal(expiry) || !reflect.DeepEqual(boundUsers, patch.UserIDs) || !reflect.DeepEqual(boundTeams, patch.TeamIDs) {
					t.Fatalf("combined commit = %+v %v %v", after, boundUsers, boundTeams)
				}
				if err := st.UpdateInboundKeyPolicy(ctx, k.ID, users[0].ID, InboundKeyPolicyPatch{SetExpiry: true}); err != nil {
					t.Fatal(err)
				}
				after = readPolicyKey(t, st, k.ID)
				boundUsers, _ = st.ListKeyUserIDs(ctx, k.ID)
				boundTeams, _ = st.ListKeyTeamIDs(ctx, k.ID)
				if after.ExpiresAt != nil || after.Description != value || !reflect.DeepEqual(boundUsers, patch.UserIDs) || !reflect.DeepEqual(boundTeams, patch.TeamIDs) || after.TokenHash != before.TokenHash || !reflect.DeepEqual(after.OwnerTeamID, before.OwnerTeamID) {
					t.Fatal("omitted fields, bindings or identity changed")
				}
			}
		})
	}
}

func TestKeyPolicyRechecksAfterWorkspaceLock(t *testing.T) {
	for _, mutation := range []string{"recipient-offboard", "actor-offboard", "actor-demote", "rotate", "revoke", "delete-key", "other-policy"} {
		t.Run(mutation, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			k, users, team := policyFixture(t, st)
			before := readPolicyKey(t, st, k.ID)
			holder, err := st.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback()
			if err := lockMembershipWorkspace(ctx, holder, k.WorkspaceID); err != nil {
				t.Fatal(err)
			}
			value := "changed"
			result := make(chan error, 1)
			go func() {
				result <- st.UpdateInboundKeyPolicy(ctx, k.ID, users[0].ID, InboundKeyPolicyPatch{Description: &value, ReplaceBindings: true, UserIDs: []uuid.UUID{users[1].ID}, TeamIDs: []uuid.UUID{team}})
			}()
			waitMembershipLocks(t, ctx, st, k.WorkspaceID, 1)
			var want error
			switch mutation {
			case "recipient-offboard", "actor-offboard":
				id := users[1].ID
				want = ErrInvalidKeyBinding
				if mutation == "actor-offboard" {
					id = users[0].ID
					want = ErrKeyManagerRequired
				}
				_, err = holder.NewDelete().Model((*WorkspaceMember)(nil)).Where("user_id = ? AND workspace_id = ?", id, k.WorkspaceID).Exec(ctx)
			case "actor-demote":
				want = ErrKeyManagerRequired
				_, err = holder.NewUpdate().Model((*TeamMember)(nil)).Set("role = 'member'").Where("user_id = ? AND team_id = ?", users[0].ID, team).Exec(ctx)
			case "rotate":
				err = st.RotateInboundKey(ctx, k.ID, "rotated", "rotated")
			case "revoke":
				err = st.SetInboundKeyEnabled(ctx, k.ID, false)
			case "delete-key":
				want = sql.ErrNoRows
				_, err = holder.NewDelete().Model((*InboundKey)(nil)).Where("id = ?", k.ID).Exec(ctx)
			case "other-policy":
				_, err = holder.NewUpdate().Model((*InboundKey)(nil)).Set("tenant = 'concurrent'").Where("id = ?", k.ID).Exec(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-result; !errors.Is(err, want) {
				t.Fatalf("update = %v want %v", err, want)
			}
			if mutation == "delete-key" {
				return
			}
			after := readPolicyKey(t, st, k.ID)
			if want != nil {
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("rejected update changed fields: %+v", after)
				}
				return
			}
			if after.Description != value {
				t.Fatal("valid update not saved")
			}
			if mutation == "rotate" && (after.TokenHash != "rotated" || after.TokenPrefix != "rotated") {
				t.Fatal("rotation overwritten")
			}
			if mutation == "revoke" && after.Enabled {
				t.Fatal("revocation overwritten")
			}
			if mutation == "other-policy" && after.Tenant != "concurrent" {
				t.Fatal("omitted policy overwritten")
			}
			if err := st.RotateInboundKey(ctx, k.ID, "later", "later"); err != nil {
				t.Fatal(err)
			}
			if err := st.SetInboundKeyEnabled(ctx, k.ID, false); err != nil {
				t.Fatal(err)
			}
			if readPolicyKey(t, st, k.ID).Description != value {
				t.Fatal("later credential mutation overwrote policy")
			}
		})
	}
}

func TestKeyBindingsSerializeWithOffboarding(t *testing.T) {
	for _, method := range []string{"policy", "bindings"} {
		t.Run(method, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			k, users, _ := policyFixture(t, st)
			holder, err := st.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback()
			if err := lockMembershipWorkspace(ctx, holder, k.WorkspaceID); err != nil {
				t.Fatal(err)
			}
			results := make(chan error, 2)
			go func() {
				if method == "bindings" {
					results <- st.SetKeyBindings(ctx, k.ID, []uuid.UUID{users[1].ID}, nil)
				} else {
					results <- st.UpdateInboundKeyPolicy(ctx, k.ID, users[0].ID, InboundKeyPolicyPatch{ReplaceBindings: true, UserIDs: []uuid.UUID{users[1].ID}})
				}
			}()
			go func() { results <- st.DeleteMembership(ctx, users[1].ID, k.WorkspaceID) }()
			waitMembershipLocks(t, ctx, st, k.WorkspaceID, 2)
			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := <-results; err != nil && !errors.Is(err, ErrInvalidKeyBinding) {
					t.Fatal(err)
				}
			}
			ids, err := st.ListKeyUserIDs(ctx, k.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range ids {
				if id == users[1].ID {
					t.Fatal("offboarded grant survived")
				}
			}
		})
	}
}

func TestKeyPolicyUserDeletionLockOrder(t *testing.T) {
	for _, first := range []string{"delete", "policy"} {
		t.Run(first, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			k, users, _ := policyFixture(t, st)
			holder, err := st.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback()
			if err := lockMembershipWorkspace(ctx, holder, k.WorkspaceID); err != nil {
				t.Fatal(err)
			}
			deletion, policy := make(chan error, 1), make(chan error, 1)
			deleteUser := func() { deletion <- st.DeleteUser(ctx, users[1].ID) }
			update := func() {
				policy <- st.UpdateInboundKeyPolicy(ctx, k.ID, users[0].ID, InboundKeyPolicyPatch{ReplaceBindings: true, UserIDs: []uuid.UUID{users[1].ID}})
			}
			if first == "delete" {
				go deleteUser()
			} else {
				go update()
			}
			waitMembershipLocks(t, ctx, st, k.WorkspaceID, 1)
			if first == "delete" {
				go update()
			} else {
				go deleteUser()
			}
			waitMembershipLocks(t, ctx, st, users[1].ID, 1)
			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-deletion; err != nil {
				t.Fatal(err)
			}
			want := error(nil)
			if first == "delete" {
				want = ErrInvalidKeyBinding
			}
			if err := <-policy; !errors.Is(err, want) {
				t.Fatalf("policy = %v want %v", err, want)
			}
			ids, err := st.ListKeyUserIDs(ctx, k.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range ids {
				if id == users[1].ID {
					t.Fatal("deleted user grant survived")
				}
			}
		})
	}
}
