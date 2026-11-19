package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/store"
)

func TestInviteOnboardingRegistrationClosed(t *testing.T) {
	st, h, _ := openUsersTestStore(t)
	ctx := context.Background()
	deps := h.SnapshotDependencies()
	var logs bytes.Buffer
	deps.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	deps.MultiTenancy.AllowPublicRegistration = false
	h.UpdateDependencies(deps)
	workspace := &store.Workspace{Name: fmt.Sprintf("invited-tenant-%d", time.Now().UnixNano()), Kind: "organization"}
	if err := st.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteWorkspace(ctx, workspace.ID) })
	for _, kind := range []string{"invalid", "expired", "used", "valid"} {
		t.Run(kind, func(t *testing.T) {
			email := fmt.Sprintf("%s-%d@example.test", kind, time.Now().UnixNano())
			token := "fixture-token-" + email
			inv := &store.Invite{Email: email, TokenHash: store.TokenHash(token), WorkspaceID: &workspace.ID,
				WorkspaceRole: "member", ExpiresAt: time.Now().Add(time.Hour)}
			if kind == "expired" {
				inv.ExpiresAt = time.Now().Add(-time.Hour)
			}
			if kind == "used" {
				now := time.Now()
				inv.AcceptedAt = &now
			}
			if kind != "invalid" {
				if err := st.CreateInvite(ctx, inv); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = st.DeleteInvite(ctx, inv.ID) })
			}
			if code, _ := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/register", map[string]any{"email": email, "password": "invited-password"}); code != http.StatusForbidden {
				t.Fatalf("registration = %d", code)
			}
			code, _ := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/invites/accept", map[string]any{"token": token, "password": "invited-password"})
			if strings.Contains(logs.String(), token) || strings.Contains(logs.String(), "invited-password") {
				t.Fatal("invite credentials appeared in application logs")
			}
			u, err := st.GetUserByEmail(ctx, email)
			if kind != "valid" {
				if code < 400 || !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("unavailable invite created account: status=%d err=%v", code, err)
				}
				return
			}
			if code != http.StatusCreated || err != nil {
				t.Fatalf("accept = %d, %v", code, err)
			}
			t.Cleanup(func() { _ = st.DeleteUser(ctx, u.ID) })
			memberships, err := st.ListMembershipsByUser(ctx, u.ID)
			if err != nil || len(memberships) != 1 || memberships[0].WorkspaceID != workspace.ID {
				t.Fatalf("wrong workspace: %v %v", memberships, err)
			}
			code, login := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/login", map[string]any{"email": email, "password": "invited-password"})
			if code != http.StatusOK {
				t.Fatalf("login = %d", code)
			}
			code, list := callAdmin(t, h, login["access_token"].(string), http.MethodGet, "/_internal/admin/workspaces", nil)
			if code != http.StatusOK {
				t.Fatalf("workspaces = %d", code)
			}
			rows := list["workspaces"].([]any)
			if len(rows) != 1 || rows[0].(map[string]any)["id"] != workspace.ID.String() {
				t.Fatalf("wrong portal workspace: %v", rows)
			}
			if code, _ := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/invites/accept", map[string]any{"token": token, "password": "changed-password"}); code != http.StatusBadRequest {
				t.Fatalf("reuse = %d", code)
			}
			after, err := st.GetUserByEmail(ctx, email)
			if err != nil || after.ID != u.ID || after.PasswordHash != u.PasswordHash {
				t.Fatal("reuse mutated account")
			}
		})
	}
}
