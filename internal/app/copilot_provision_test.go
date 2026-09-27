package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type syncedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncedBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncedBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func copilotProvisionFixture(t *testing.T) (*App, string, func(string, string, string) *httptest.ResponseRecorder, func(string, string, string, string) *httptest.ResponseRecorder, *httptest.Server, *string, *syncedBuffer) {
	t.Helper()
	dsn := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("AIPROXY_TEST_DATABASE_URL is required (no skips for device-flow tests)")
	}
	t.Setenv("AIPROXY_JWT_SECRET", "flow03-runtime-fixture-secret")
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "REDACTED")
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "flow03_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	fixture, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.MigrateUp(ctx); err != nil {
		_ = fixture.Close()
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	t.Cleanup(upstream.Close)
	tokenMode := "pending"
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/login/device/code") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code": "dev-app-code", "user_code": "WDXA-7777",
				"verification_uri": copilotlogin.VerificationURL,
				"expires_in":       900, "interval": 5,
			})
			return
		}
		if tokenMode == "success" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "gho_app_flow_token", "token_type": "bearer", "scope": "read:user",
			})
			return
		}
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	t.Cleanup(issuer.Close)
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
}
database { url = %q }
multi_tenancy { enabled = true }
`, upstream.URL, u.String())
	path := writeConfigFile(t, cfg)
	logBuf := &syncedBuffer{}
	a, err := Build(ctx, BuildOptions{ConfigPath: path, LogOutput: logBuf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if a.copilotCleanup == nil {
		t.Fatal("Build must start the device-flow cleanup worker")
	}
	a.SetCopilotDeviceClient(func() *copilotlogin.Client {
		c := copilotlogin.New()
		c.DeviceCodeURL = issuer.URL + "/login/device/code"
		c.TokenURL = issuer.URL + "/login/oauth/access_token"
		return c
	})
	owner := store.User{Email: "admin@example.com", IsAdmin: true}
	if err := a.adminStore.CreateUser(ctx, &owner); err != nil {
		t.Fatal(err)
	}
	access, _, err := adminauth.IssueAccess(owner.ID.String(), owner.Email, true)
	if err != nil {
		t.Fatal(err)
	}
	extraAccess := map[string]string{"admin": access}
	_ = extraAccess
	requestAs := func(token, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if strings.HasPrefix(path, "/v1/") {
			token = "static-fixture"
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.Server.Handler.ServeHTTP(w, r)
		return w
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		return requestAs(extraAccess["admin"], method, path, body)
	}
	return a, path, request, requestAs, upstream, &tokenMode, logBuf
}

func expediteAppFlow(t *testing.T, a *App, flowID string) {
	t.Helper()
	id, err := uuid.Parse(flowID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.adminStore.DB.ExecContext(context.Background(), "UPDATE copilot_device_flows SET poll_at = clock_timestamp() - interval '1 second' WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}
}

func parseFlowBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var parsed map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("decode %d %s: %v", w.Code, w.Body.String(), err)
	}
	return parsed
}

func TestCopilotProvisionedFlowEndToEnd(t *testing.T) {
	a, _, request, requestAs, upstream, tokenMode, logBuf := copilotProvisionFixture(t)
	ctx := context.Background()
	issueAdmin := func(email string) string {
		t.Helper()
		u := store.User{Email: email, IsAdmin: true}
		if err := a.adminStore.CreateUser(ctx, &u); err != nil {
			t.Fatal(err)
		}
		token, _, err := adminauth.IssueAccess(u.ID.String(), u.Email, true)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	editorAccess := issueAdmin("editor@example.com")
	secondAccess := issueAdmin("second@example.com")

	observedAuth := make(chan string, 8)
	upstream.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		observedAuth <- r.Header.Get("Authorization")
		if r.URL.Path != "/chat/completions" || body.Model != "base-model" || r.Header.Get("X-Initiator") != "user" {
			t.Error("incorrect Copilot dispatch")
		}
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[]}\n\ndata: [DONE]\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[]}`)
		}
	})

	start := request("POST", "/_internal/admin/copilot-device-flows", `{"client_id":"Ov23apptestclient"}`)
	if start.Code != http.StatusCreated {
		t.Fatalf("start = %d %s", start.Code, start.Body.String())
	}
	parsed := parseFlowBody(t, start)
	if parsed["status"] != "pending" || parsed["user_code"] != "WDXA-7777" {
		t.Fatalf("start body = %s", start.Body.String())
	}
	for _, secret := range []string{"device_code", "access_token", "refresh_token", "dev-app-code"} {
		if strings.Contains(start.Body.String(), secret) {
			t.Fatalf("start leaks %q", secret)
		}
	}
	flowID, _ := parsed["id"].(string)
	expediteAppFlow(t, a, flowID)
	*tokenMode = "success"
	poll := request("POST", "/_internal/admin/copilot-device-flows/"+flowID+"/poll", "")
	if poll.Code != http.StatusOK || !strings.Contains(poll.Body.String(), `"status":"ready"`) {
		t.Fatalf("poll = %d %s", poll.Code, poll.Body.String())
	}

	save := request("POST", "/_internal/admin/providers", fmt.Sprintf(`{"name":"app-copilot","type":"github-copilot","base_url":%q,"copilot_device_flow_id":%q,"models":[{"name":"m","upstream_name":"base-model","capabilities":["chat"]}]}`, upstream.URL, flowID))
	if save.Code != http.StatusCreated {
		t.Fatalf("save = %d %s", save.Code, save.Body.String())
	}
	if save.Header().Get("X-Aiproxy-Catalog-Saved") != "true" {
		t.Fatalf("save must set saved indicator")
	}
	stored, err := a.adminStore.GetProvider(ctx, "app-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.CopilotCredentialEncrypted) == 0 {
		t.Fatalf("save must persist encrypted DB credential")
	}

	for _, stream := range []bool{false, true} {
		body := fmt.Sprintf(`{"model":"app-copilot/m","messages":[],"stream":%v}`, stream)
		w := request("POST", "/v1/chat/completions", body)
		if w.Code != 200 {
			t.Fatalf("dispatch stream=%v = %d %s", stream, w.Code, w.Body.String())
		}
		if stream && !strings.Contains(w.Body.String(), "[DONE]") {
			t.Fatalf("SSE body = %s", w.Body.String())
		}
		select {
		case auth := <-observedAuth:
			if auth != "Bearer gho_app_flow_token" {
				t.Fatalf("upstream auth = %q", auth)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("upstream not reached")
		}
	}

	status := request("GET", "/_internal/admin/copilot-device-flows/"+flowID, "")
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"consumed"`) {
		t.Fatalf("consumed = %d %s", status.Code, status.Body.String())
	}

	view := request("GET", "/_internal/admin/providers/app-copilot", "")
	viewParsed := parseFlowBody(t, view)
	revision, _ := viewParsed["updated_at"].(string)
	if revision == "" {
		t.Fatalf("provider view must expose updated_at: %s", view.Body.String())
	}
	if viewParsed["copilot_credential_source"] != "database" {
		t.Fatalf("source = %s", view.Body.String())
	}

	editStart := requestAs(editorAccess, "POST", "/_internal/admin/copilot-device-flows", `{"client_id":"Ov23apptestclient","provider_name":"app-copilot"}`)
	if editStart.Code != http.StatusCreated {
		t.Fatalf("edit start = %d %s", editStart.Code, editStart.Body.String())
	}
	if editBody := editStart.Body.String(); !strings.Contains(editBody, `"status":"pending"`) {
		t.Fatalf("edit start not pending: %s\nlogs:\n%s", editBody, logBuf.String())
	}
	editID, _ := parseFlowBody(t, editStart)["id"].(string)
	expediteAppFlow(t, a, editID)
	if poll := requestAs(editorAccess, "POST", "/_internal/admin/copilot-device-flows/"+editID+"/poll", ""); poll.Code != http.StatusOK || !strings.Contains(poll.Body.String(), `"status":"ready"`) {
		t.Fatalf("edit poll = %d %s", poll.Code, poll.Body.String())
	}
	stale := requestAs(editorAccess, "PUT", "/_internal/admin/providers/app-copilot", fmt.Sprintf(`{"copilot_device_flow_id":%q,"expected_updated_at":"2000-01-01T00:00:00Z","enabled":true}`, editID))
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale revision = %d %s, want 409", stale.Code, stale.Body.String())
	}
	retry := requestAs(editorAccess, "PUT", "/_internal/admin/providers/app-copilot", fmt.Sprintf(`{"copilot_device_flow_id":%q,"expected_updated_at":%q,"enabled":true}`, editID, revision))
	if retry.Code != http.StatusOK {
		t.Fatalf("retry = %d %s", retry.Code, retry.Body.String())
	}
	if retry.Header().Get("X-Aiproxy-Catalog-Saved") != "true" {
		t.Fatalf("edit save must set saved indicator")
	}

	deps := a.handler.SnapshotDependencies()
	deps.RequestReload = func() error { return fmt.Errorf("simulated activation failure") }
	a.handler.UpdateDependencies(deps)
	secondStart := requestAs(secondAccess, "POST", "/_internal/admin/copilot-device-flows", `{"client_id":"Ov23apptestclient"}`)
	if secondStart.Code != http.StatusCreated {
		t.Fatalf("second start = %d %s", secondStart.Code, secondStart.Body.String())
	}
	secondID, _ := parseFlowBody(t, secondStart)["id"].(string)
	expediteAppFlow(t, a, secondID)
	if poll := requestAs(secondAccess, "POST", "/_internal/admin/copilot-device-flows/"+secondID+"/poll", ""); poll.Code != http.StatusOK {
		t.Fatalf("second poll = %d %s", poll.Code, poll.Body.String())
	}
	failed := requestAs(secondAccess, "POST", "/_internal/admin/providers", fmt.Sprintf(`{"name":"app-copilot-two","type":"github-copilot","base_url":%q,"copilot_device_flow_id":%q,"models":[{"name":"m","upstream_name":"base-model","capabilities":["chat"]}]}`, upstream.URL, secondID))
	if failed.Code != http.StatusInternalServerError || !strings.Contains(failed.Body.String(), "saved but activation failed") {
		t.Fatalf("failed activation = %d %s", failed.Code, failed.Body.String())
	}
	if failed.Header().Get("X-Aiproxy-Catalog-Saved") != "true" {
		t.Fatalf("failed activation must still report saved state")
	}
	if _, err := a.adminStore.GetProvider(ctx, "app-copilot-two"); err != nil {
		t.Fatalf("failed activation must leave saved provider: %v", err)
	}
	secondStatus := requestAs(secondAccess, "GET", "/_internal/admin/copilot-device-flows/"+secondID, "")
	if secondStatus.Code != http.StatusOK || !strings.Contains(secondStatus.Body.String(), `"status":"consumed"`) {
		t.Fatalf("failed activation must leave consumed flow: %d %s", secondStatus.Code, secondStatus.Body.String())
	}
	deps = a.handler.SnapshotDependencies()
	deps.RequestReload = a.Reload
	a.handler.UpdateDependencies(deps)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload recovery: %v", err)
	}
	if w := request("POST", "/v1/chat/completions", `{"model":"app-copilot-two/m","messages":[]}`); w.Code != 200 {
		t.Fatalf("recovered dispatch = %d %s", w.Code, w.Body.String())
	}

	second, err := Build(ctx, BuildOptions{ConfigPath: a.buildOpt.ConfigPath, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	second.SetCopilotDeviceClient(a.handler.SnapshotDependencies().CopilotDeviceClient)
	coherence := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/_internal/admin/copilot-device-flows/"+flowID, nil)
		owner, err := a.adminStore.GetUserByEmail(ctx, "admin@example.com")
		if err != nil {
			t.Fatal(err)
		}
		access, _, err := adminauth.IssueAccess(owner.ID.String(), owner.Email, true)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+access)
		w := httptest.NewRecorder()
		second.Server.Handler.ServeHTTP(w, r)
		return w
	}()
	if coherence.Code != http.StatusOK || !strings.Contains(coherence.Body.String(), `"status":"consumed"`) {
		t.Fatalf("second instance must share DB flow state: %d %s", coherence.Code, coherence.Body.String())
	}
	if second.copilotCleanup == nil {
		t.Fatal("second instance must own a cleanup worker")
	}
}
