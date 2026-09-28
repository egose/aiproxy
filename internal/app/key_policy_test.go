package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestKeyPolicyRuntimeAtomicity(t *testing.T) {
	dsn := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	t.Setenv("AIPROXY_JWT_SECRET", "life04-runtime-fixture-secret")
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "life04_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.DB.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.DB.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
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
	fixture, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.Close() })
	if _, err := fixture.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer upstream.Close()
	cfg := fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" {
  mode = "bearer_static"
  client "static" { token = "static-fixture" }
}
provider "openai-compatible" "local" {
  base_url = %q
  api_key = "fixture"
  model "old" {}
  model "new" {}
}
database { url = %q }
multi_tenancy { enabled = true }
`, upstream.URL, u.String())
	path := writeConfigFile(t, cfg)
	a, err := Build(ctx, BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	st := a.adminStore
	owner := store.User{Email: "owner@example.com", PasswordHash: "unused", IsAdmin: true}
	member := store.User{Email: "member@example.com", PasswordHash: "unused"}
	outsider := store.User{Email: "outsider@example.com", PasswordHash: "unused"}
	for _, user := range []*store.User{&owner, &member, &outsider} {
		if err := st.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.AddMembership(ctx, &store.WorkspaceMember{WorkspaceID: store.SystemWorkspaceID, UserID: member.ID}); err != nil {
		t.Fatal(err)
	}
	team := store.WorkspaceTeam{WorkspaceID: store.SystemWorkspaceID, Name: "local"}
	if err := st.CreateTeam(ctx, &team); err != nil {
		t.Fatal(err)
	}
	workspace := store.Workspace{Name: "foreign"}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	foreign := store.WorkspaceTeam{WorkspaceID: workspace.ID, Name: "foreign"}
	if err := st.CreateTeam(ctx, &foreign); err != nil {
		t.Fatal(err)
	}
	key := store.InboundKey{Name: "runtime-key", TokenHash: store.TokenHash("runtime-fixture"), TokenPrefix: "runtime", WorkspaceID: store.SystemWorkspaceID, Enabled: true, Tenant: "old", Description: "old", AllowedModels: []string{"local/old"}}
	if err := st.CreateInboundKey(ctx, &key); err != nil {
		t.Fatal(err)
	}
	if err := st.SetKeyBindings(ctx, key.ID, []uuid.UUID{member.ID}, []uuid.UUID{team.ID}); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	access, _, err := adminauth.IssueAccess(owner.ID.String(), owner.Email, true)
	if err != nil {
		t.Fatal(err)
	}
	request := func(token, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.Server.Handler.ServeHTTP(w, r)
		return w
	}
	checkLive := func(oldStatus, newStatus int) {
		t.Helper()
		for model, want := range map[string]int{"old": oldStatus, "new": newStatus} {
			w := request("runtime-fixture", http.MethodPost, "/v1/chat/completions", `{"model":"local/`+model+`","messages":[]}`)
			if w.Code != want {
				t.Fatalf("live %s = %d %s want %d", model, w.Code, w.Body.String(), want)
			}
		}
	}
	readKey := func() store.InboundKey {
		var k store.InboundKey
		if err := st.DB.NewSelect().Model(&k).Where("id = ?", key.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		return k
	}
	before := readKey()
	checkLive(200, 403)
	keyPath := "/_internal/admin/keys/" + key.ID.String()
	for _, bindings := range []string{`"user_ids":["invalid"]`, `"team_ids":["invalid"]`, `"user_ids":["` + outsider.ID.String() + `"]`, `"team_ids":["` + foreign.ID.String() + `"]`} {
		w := request(access, http.MethodPut, keyPath, `{"description":"rejected","tenant":"rejected","allowed_models":["local/new"],"expires_at":"2000-01-01T00:00:00Z",`+bindings+`}`)
		if w.Code != 400 {
			t.Fatalf("reject = %d %s", w.Code, w.Body.String())
		}
		if !reflect.DeepEqual(before, readKey()) {
			t.Fatal("rejected policy persisted")
		}
		users, err := st.ListKeyUserIDs(ctx, key.ID)
		if err != nil {
			t.Fatal(err)
		}
		teams, err := st.ListKeyTeamIDs(ctx, key.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(users, []uuid.UUID{member.ID}) || !reflect.DeepEqual(teams, []uuid.UUID{team.ID}) {
			t.Fatal("rejected bindings persisted")
		}
		checkLive(200, 403)
		if err := a.Reload(); err != nil {
			t.Fatal(err)
		}
		checkLive(200, 403)
	}
	if _, err := st.DB.ExecContext(ctx, "ALTER TABLE key_teams ADD CONSTRAINT policy_fault CHECK (false) NOT VALID"); err != nil {
		t.Fatal(err)
	}
	w := request(access, http.MethodPut, keyPath, `{"description":"fault","allowed_models":["local/new"],"user_ids":[],"team_ids":["`+team.ID.String()+`"]}`)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "could not update key") || strings.Contains(w.Body.String(), "policy_fault") {
		t.Fatalf("storage failure = %d %s", w.Code, w.Body.String())
	}
	if !reflect.DeepEqual(before, readKey()) {
		t.Fatal("binding failure changed fields")
	}
	users, err := st.ListKeyUserIDs(ctx, key.ID)
	if err != nil {
		t.Fatal(err)
	}
	teams, err := st.ListKeyTeamIDs(ctx, key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(users, []uuid.UUID{member.ID}) || !reflect.DeepEqual(teams, []uuid.UUID{team.ID}) {
		t.Fatal("binding failure changed grants")
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	checkLive(200, 403)
	if _, err := st.DB.ExecContext(ctx, "ALTER TABLE key_teams DROP CONSTRAINT policy_fault"); err != nil {
		t.Fatal(err)
	}
	w = request(access, http.MethodPut, keyPath, `{"description":"committed","tenant":"new","allowed_models":["local/new"],"expires_at":"","user_ids":[]}`)
	if w.Code != 200 {
		t.Fatalf("commit = %d %s", w.Code, w.Body.String())
	}
	checkLive(403, 200)
	users, _ = st.ListKeyUserIDs(ctx, key.ID)
	teams, _ = st.ListKeyTeamIDs(ctx, key.ID)
	if len(users)+len(teams) != 0 || readKey().Tenant != "new" {
		t.Fatal("combined policy/bindings commit incomplete")
	}
	w = request(access, http.MethodPut, keyPath, `{"expires_at":"2000-01-01T00:00:00Z"}`)
	if w.Code != 200 {
		t.Fatalf("expire = %d %s", w.Code, w.Body.String())
	}
	checkLive(401, 401)
	w = request(access, http.MethodPut, keyPath, `{"expires_at":"","allowed_models":[]}`)
	if w.Code != 200 {
		t.Fatalf("clear = %d %s", w.Code, w.Body.String())
	}
	checkLive(200, 200)
	rewriteConfigFile(t, path, "invalid hcl {")
	w = request(access, http.MethodPut, keyPath, `{"allowed_models":["local/new"]}`)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "saved but activation failed") {
		t.Fatalf("failed activation = %d %s", w.Code, w.Body.String())
	}
	if !reflect.DeepEqual(readKey().AllowedModels, []string{"local/new"}) {
		t.Fatal("valid commit lost on activation failure")
	}
	checkLive(200, 200)
	rewriteConfigFile(t, path, cfg)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	checkLive(403, 200)
}
