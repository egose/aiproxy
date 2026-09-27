package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type invitePrecheckHook struct {
	email string
	fired atomic.Bool
	after func()
}

func (h *invitePrecheckHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (h *invitePrecheckHook) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	if strings.HasPrefix(event.Query, "SELECT") && strings.Contains(event.Query, `"users"`) && strings.Contains(event.Query, h.email) && h.fired.CompareAndSwap(false, true) {
		h.after()
	}
}

func acceptanceHTTPFixture(t *testing.T, st *store.Store) (store.Invite, string) {
	t.Helper()
	token := uuid.NewString()
	inv := store.Invite{Email: uuid.NewString() + "@example.com", TokenHash: store.TokenHash(token), ExpiresAt: time.Now().Add(time.Hour), WorkspaceID: &store.SystemWorkspaceID, WorkspaceRole: "member"}
	if err := st.CreateInvite(context.Background(), &inv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_ = st.DeleteInvite(ctx, inv.ID)
		if u, err := st.GetUserByEmail(ctx, inv.Email); err == nil {
			_ = st.DeleteUser(ctx, u.ID)
		}
	})
	return inv, token
}

func requestInviteAcceptance(t *testing.T, h *Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"token": token, "password": "invited-password"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/_internal/admin/invites/accept", bytes.NewReader(body)))
	return w
}

func TestInviteAcceptanceRechecksAfterPrecheck(t *testing.T) {
	for _, mutation := range []string{"revoked", "expired", "accepted"} {
		t.Run(mutation, func(t *testing.T) {
			st, h, _ := openUsersTestStore(t)
			ctx := context.Background()
			inv, token := acceptanceHTTPFixture(t, st)
			hook := &invitePrecheckHook{email: inv.Email, after: func() {
				var err error
				switch mutation {
				case "revoked":
					err = st.DeleteInvite(ctx, inv.ID)
				case "expired":
					_, err = st.DB.ExecContext(ctx, `UPDATE invites SET expires_at = clock_timestamp() - interval '1 second' WHERE id = ?`, inv.ID)
				case "accepted":
					_, err = st.DB.ExecContext(ctx, `UPDATE invites SET accepted_at = clock_timestamp() WHERE id = ?`, inv.ID)
				}
				if err != nil {
					t.Fatal(err)
				}
			}}
			st.DB.AddQueryHook(hook)
			w := requestInviteAcceptance(t, h, token)
			wantCode, wantBody := http.StatusBadRequest, "invite is expired or already accepted\n"
			if mutation == "revoked" {
				wantCode, wantBody = http.StatusNotFound, "invite not found or revoked\n"
			}
			if !hook.fired.Load() {
				t.Fatal("precheck barrier not reached")
			}
			if w.Code != wantCode || w.Body.String() != wantBody {
				t.Errorf("accept = %d %q, want %d %q", w.Code, w.Body.String(), wantCode, wantBody)
			}
			if _, err := st.GetUserByEmail(ctx, inv.Email); !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("rejected invite account lookup = %v", err)
			}
		})
	}
}

func TestInviteAcceptanceMembershipFailureRollsBack(t *testing.T) {
	st, h, _ := openUsersTestStore(t)
	ctx := context.Background()
	inv, token := acceptanceHTTPFixture(t, st)
	workspace := store.Workspace{Name: "invite-" + uuid.NewString()}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteWorkspace(ctx, workspace.ID) })
	if _, err := st.DB.ExecContext(ctx, `UPDATE invites SET workspace_id = ? WHERE id = ?`, workspace.ID, inv.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `ALTER TABLE workspace_members ADD CONSTRAINT life01_http_fail CHECK (workspace_id <> ?) NOT VALID`, workspace.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := st.DB.ExecContext(ctx, `ALTER TABLE workspace_members DROP CONSTRAINT IF EXISTS life01_http_fail`); err != nil {
			t.Error(err)
		}
	})
	w := requestInviteAcceptance(t, h, token)
	if w.Code != http.StatusInternalServerError || w.Body.String() != "could not accept invite\n" {
		t.Errorf("failure = %d %q, want controlled 500", w.Code, w.Body.String())
	}
	if _, err := st.GetUserByEmail(ctx, inv.Email); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("failed acceptance account lookup = %v", err)
	}
	current, err := st.GetInviteByHash(ctx, inv.TokenHash)
	if err != nil || current.AcceptedAt != nil {
		t.Fatalf("failed acceptance consumed invitation: %+v, %v", current, err)
	}
	if members, err := st.ListMembershipsByWorkspace(ctx, workspace.ID); err != nil || len(members) != 0 {
		t.Errorf("failed acceptance memberships = %+v, %v", members, err)
	}
	if _, err := st.DB.ExecContext(ctx, `ALTER TABLE workspace_members DROP CONSTRAINT life01_http_fail`); err != nil {
		t.Fatal(err)
	}
	w = requestInviteAcceptance(t, h, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("retry = %d %q", w.Code, w.Body.String())
	}
	u, err := st.GetUserByEmail(ctx, inv.Email)
	if err != nil {
		t.Fatal(err)
	}
	m, err := st.GetMembership(ctx, u.ID, workspace.ID)
	if err != nil || m.Role != "member" {
		t.Errorf("retry membership = %+v, %v", m, err)
	}
	code, body := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/login", map[string]string{"email": inv.Email, "password": "invited-password"})
	if code != http.StatusOK || body["access_token"] == nil {
		t.Errorf("retry login = %d %+v", code, body)
	}
}

func TestInviteAcceptanceUsesCurrentAuthorityAfterPrecheck(t *testing.T) {
	st, h, _ := openUsersTestStore(t)
	ctx := context.Background()
	inv, token := acceptanceHTTPFixture(t, st)
	if _, err := st.DB.ExecContext(ctx, `UPDATE invites SET is_admin = true, workspace_role = 'admin' WHERE id = ?`, inv.ID); err != nil {
		t.Fatal(err)
	}
	workspace := store.Workspace{Name: "invite-" + uuid.NewString()}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteWorkspace(ctx, workspace.ID) })
	st.DB.AddQueryHook(&invitePrecheckHook{email: inv.Email, after: func() {
		if _, err := st.DB.ExecContext(ctx, `UPDATE invites SET is_admin = false, workspace_role = 'member', workspace_id = ? WHERE id = ?`, workspace.ID, inv.ID); err != nil {
			t.Fatal(err)
		}
	}})
	w := requestInviteAcceptance(t, h, token)
	var body adminUserView
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != http.StatusCreated || body.IsAdmin || body.Role != "user" {
		t.Fatalf("current invitation response = %d %q, %v", w.Code, w.Body.String(), err)
	}
	u, err := st.GetUserByEmail(ctx, inv.Email)
	if err != nil || u.IsAdmin {
		t.Fatalf("current account = %+v, %v", u, err)
	}
	members, err := st.ListMembershipsByUser(ctx, u.ID)
	if err != nil || len(members) != 1 || members[0].WorkspaceID != workspace.ID || members[0].Role != "member" {
		t.Errorf("current memberships = %+v, %v", members, err)
	}
}

func TestInviteAcceptanceEmailCollisionAfterPrecheck(t *testing.T) {
	st, h, _ := openUsersTestStore(t)
	ctx := context.Background()
	inv, token := acceptanceHTTPFixture(t, st)
	existing := store.User{Email: inv.Email, PasswordHash: "existing-hash"}
	st.DB.AddQueryHook(&invitePrecheckHook{email: inv.Email, after: func() {
		if err := st.CreateUser(ctx, &existing); err != nil {
			t.Fatal(err)
		}
	}})
	w := requestInviteAcceptance(t, h, token)
	if w.Code != http.StatusBadRequest || w.Body.String() != "email is already registered\n" {
		t.Errorf("email collision = %d %q", w.Code, w.Body.String())
	}
	current, err := st.GetInviteByHash(ctx, inv.TokenHash)
	if err != nil || current.AcceptedAt != nil {
		t.Errorf("email collision consumed invitation: %+v, %v", current, err)
	}
	u, err := st.GetUserByEmail(ctx, inv.Email)
	if err != nil || u.ID != existing.ID || u.PasswordHash != existing.PasswordHash {
		t.Errorf("email collision changed account: %+v, %v", u, err)
	}
	if members, err := st.ListMembershipsByUser(ctx, existing.ID); err != nil || len(members) != 0 {
		t.Errorf("email collision memberships = %+v, %v", members, err)
	}
	if err := st.DeleteUser(ctx, existing.ID); err != nil {
		t.Fatal(err)
	}
	if w := requestInviteAcceptance(t, h, token); w.Code != http.StatusCreated {
		t.Errorf("email collision retry = %d %q", w.Code, w.Body.String())
	}
}

func TestInviteAcceptanceConcurrentRequests(t *testing.T) {
	st, h, _ := openUsersTestStore(t)
	ctx := context.Background()
	inv, token := acceptanceHTTPFixture(t, st)
	ready, release := make(chan struct{}), make(chan struct{})
	st.DB.AddQueryHook(&invitePrecheckHook{email: inv.Email, after: func() {
		close(ready)
		<-release
	}})
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- requestInviteAcceptance(t, h, token) }()
	defer close(release)
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("first request did not reach precheck barrier")
	}
	second := requestInviteAcceptance(t, h, token)
	release <- struct{}{}
	var loser *httptest.ResponseRecorder
	select {
	case loser = <-first:
	case <-time.After(10 * time.Second):
		t.Fatal("competing request did not finish")
	}
	if second.Code != http.StatusCreated || loser.Code != http.StatusBadRequest || loser.Body.String() != "invite is expired or already accepted\n" {
		t.Fatalf("racing responses = %d %q, %d %q", second.Code, second.Body.String(), loser.Code, loser.Body.String())
	}
	u, err := st.GetUserByEmail(ctx, inv.Email)
	if err != nil {
		t.Fatal(err)
	}
	members, err := st.ListMembershipsByUser(ctx, u.ID)
	if err != nil || len(members) != 1 || members[0].WorkspaceID != store.SystemWorkspaceID {
		t.Errorf("winning memberships = %+v, %v", members, err)
	}
	current, err := st.GetInviteByHash(ctx, inv.TokenHash)
	if err != nil || current.AcceptedAt == nil {
		t.Errorf("winning invitation = %+v, %v", current, err)
	}
}
