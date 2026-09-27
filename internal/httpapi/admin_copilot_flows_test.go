package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type mockTokenBehavior struct {
	mu              sync.Mutex
	codeHits        int
	tokenHits       int
	verificationURI string
	tokenQueue      []string
	tokenDefault    string
}

func (m *mockTokenBehavior) codeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.codeHits++
		uri := m.verificationURI
		m.mu.Unlock()
		if uri == "" {
			uri = copilotlogin.VerificationURL
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "dev-flow-code",
			"user_code":        "WDXA-4242",
			"verification_uri": uri,
			"expires_in":       900,
			"interval":         5,
		})
	}
}

func (m *mockTokenBehavior) tokenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.tokenHits++
		behavior := m.tokenDefault
		if len(m.tokenQueue) > 0 {
			behavior = m.tokenQueue[0]
			m.tokenQueue = m.tokenQueue[1:]
		}
		m.mu.Unlock()
		if behavior == "" {
			behavior = "pending"
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case behavior == "pending":
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		case behavior == "denied":
			_, _ = w.Write([]byte(`{"error":"access_denied"}`))
		case behavior == "expired":
			_, _ = w.Write([]byte(`{"error":"expired_token"}`))
		case behavior == "other":
			_, _ = w.Write([]byte(`{"error":"incorrect_device_code"}`))
		case behavior == "malformed":
			_, _ = w.Write([]byte(`{"unexpected":true}`))
		case behavior == "success":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "gho_flow_success_token", "token_type": "bearer", "scope": "read:user",
			})
		case strings.HasPrefix(behavior, "slow_down:"):
			_, _ = w.Write([]byte(`{"error":"slow_down","interval":` + strings.TrimPrefix(behavior, "slow_down:") + `}`))
		default:
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		}
	}
}

func (m *mockTokenBehavior) counts() (code, token int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.codeHits, m.tokenHits
}

func (m *mockTokenBehavior) enqueue(behaviors ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokenQueue = append(m.tokenQueue, behaviors...)
}

type copilotFlowHarness struct {
	st     *store.Store
	issuer *mockTokenBehavior
	srv    *httptest.Server
	h      *Handler
	seq    atomic.Int64
}

func openCopilotFlowHarness(t *testing.T) *copilotFlowHarness {
	t.Helper()
	dbURL := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Fatal("AIPROXY_TEST_DATABASE_URL is required (no skips for device-flow tests)")
	}
	t.Setenv("AIPROXY_JWT_SECRET", "test-jwt-secret-1234567890")
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	f := &copilotFlowHarness{st: st, issuer: &mockTokenBehavior{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", f.issuer.codeHandler())
	mux.HandleFunc("/login/oauth/access_token", f.issuer.tokenHandler())
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	deps := newWebUIDeps(false)
	deps.MultiTenancy.Enabled = true
	deps.AdminStore = st
	srvURL := f.srv.URL
	deps.CopilotDeviceClient = func() *copilotlogin.Client {
		c := copilotlogin.New()
		c.DeviceCodeURL = srvURL + "/login/device/code"
		c.TokenURL = srvURL + "/login/oauth/access_token"
		return c
	}
	f.h = NewHandler(deps)
	return f
}

func (f *copilotFlowHarness) user(t *testing.T, sysAdmin bool) (store.User, string) {
	t.Helper()
	n := f.seq.Add(1)
	u := store.User{Email: fmt.Sprintf("copilot-flow-%d-%d@example.com", time.Now().UnixNano(), n), PasswordHash: "unused", IsAdmin: sysAdmin}
	if err := f.st.CreateUser(context.Background(), &u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.st.DeleteUser(context.Background(), u.ID) })
	access, _, err := adminauth.IssueAccess(u.ID.String(), u.Email, sysAdmin)
	if err != nil {
		t.Fatal(err)
	}
	return u, access
}

func (f *copilotFlowHarness) workspace(t *testing.T) store.Workspace {
	t.Helper()
	workspace := store.Workspace{Name: fmt.Sprintf("copilot-flow-workspace-%d-%d", time.Now().UnixNano(), f.seq.Add(1))}
	if err := f.st.CreateWorkspace(context.Background(), &workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.st.DeleteWorkspace(context.Background(), workspace.ID) })
	return workspace
}

func (f *copilotFlowHarness) workspaceAdmin(t *testing.T, workspace store.Workspace) (store.User, string) {
	t.Helper()
	u, access := f.user(t, false)
	if err := f.st.AddMembership(context.Background(), &store.WorkspaceMember{UserID: u.ID, WorkspaceID: workspace.ID, Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	return u, access
}

func (f *copilotFlowHarness) workspaceMember(t *testing.T, workspace store.Workspace) (store.User, string) {
	t.Helper()
	u, access := f.user(t, false)
	if err := f.st.AddMembership(context.Background(), &store.WorkspaceMember{UserID: u.ID, WorkspaceID: workspace.ID, Role: "member"}); err != nil {
		t.Fatal(err)
	}
	return u, access
}

type flowResponse struct {
	code   int
	parsed map[string]interface{}
	raw    string
	header http.Header
}

func (f *copilotFlowHarness) call(t *testing.T, access, method, path string, body interface{}) flowResponse {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	parsed := map[string]interface{}{}
	if len(w.Body.Bytes()) > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	}
	out := flowResponse{code: w.Code, parsed: parsed, raw: w.Body.String(), header: w.Header()}
	assertNoFlowSecrets(t, out.raw)
	if cc := w.Header().Get("Cache-Control"); strings.Contains(path, "copilot-device-flows") && cc != "no-store" {
		t.Fatalf("device-flow response %s %s missing no-store (got %q)", method, path, cc)
	}
	return out
}

func assertNoFlowSecrets(t *testing.T, raw string) {
	t.Helper()
	for _, secret := range []string{
		"device_code", "access_token", "refresh_token", "ciphertext",
		"challenge_encrypted", "credential_encrypted", "copilot_credential_encrypted",
		"gho_", "ghu_", "dev-flow-code",
	} {
		if strings.Contains(raw, secret) {
			t.Fatalf("response leaks %q: %s", secret, raw)
		}
	}
}

func flowIDOf(t *testing.T, resp flowResponse) string {
	t.Helper()
	id, _ := resp.parsed["id"].(string)
	if id == "" {
		t.Fatalf("response missing flow id: %s", resp.raw)
	}
	return id
}

func (f *copilotFlowHarness) expeditePoll(t *testing.T, flowID string) {
	t.Helper()
	id, err := uuid.Parse(flowID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.DB.ExecContext(context.Background(), "UPDATE copilot_device_flows SET poll_at = clock_timestamp() - interval '1 second' WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}
}

func (f *copilotFlowHarness) startCreateFlow(t *testing.T, access, workspaceID string) flowResponse {
	t.Helper()
	return f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows", map[string]interface{}{
		"client_id": "Ov23flowtestclient", "workspace_id": workspaceID,
	})
}

func TestAdminCopilotFlowSuccessLifecycle(t *testing.T) {
	f := openCopilotFlowHarness(t)
	workspace := f.workspace(t)
	_, access := f.workspaceAdmin(t, workspace)

	start := f.startCreateFlow(t, access, workspace.ID.String())
	if start.code != http.StatusCreated {
		t.Fatalf("start = %d %s", start.code, start.raw)
	}
	if start.parsed["status"] != "pending" {
		t.Fatalf("start status = %v, want pending", start.parsed["status"])
	}
	if start.parsed["user_code"] != "WDXA-4242" {
		t.Fatalf("user_code = %v", start.parsed["user_code"])
	}
	if start.parsed["verification_uri"] != copilotlogin.VerificationURL {
		t.Fatalf("verification_uri = %v", start.parsed["verification_uri"])
	}
	if _, ok := start.parsed["expires_at"]; !ok {
		t.Fatalf("missing expires_at: %s", start.raw)
	}
	flowID := flowIDOf(t, start)

	codeHits, tokenHits := f.issuer.counts()
	got := f.call(t, access, http.MethodGet, "/_internal/admin/copilot-device-flows/"+flowID, nil)
	if got.code != http.StatusOK {
		t.Fatalf("status = %d %s", got.code, got.raw)
	}
	if afterCode, afterToken := f.issuer.counts(); afterCode != codeHits || afterToken != tokenHits {
		t.Fatalf("read-only status made issuer calls (code %d->%d token %d->%d)", codeHits, afterCode, tokenHits, afterToken)
	}

	f.issuer.enqueue("pending")
	f.expeditePoll(t, flowID)
	first := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows/"+flowID+"/poll", nil)
	if first.code != http.StatusOK || first.parsed["status"] != "pending" {
		t.Fatalf("first poll = %d %s", first.code, first.raw)
	}
	if _, ok := first.parsed["poll_after_ms"]; !ok {
		t.Fatalf("pending poll must advise poll_after_ms: %s", first.raw)
	}
	_, tokenAfterFirst := f.issuer.counts()
	if tokenAfterFirst != tokenHits+1 {
		t.Fatalf("first poll must make exactly one token request (token %d->%d)", tokenHits, tokenAfterFirst)
	}

	early := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows/"+flowID+"/poll", nil)
	if early.code != http.StatusOK || early.parsed["status"] != "pending" {
		t.Fatalf("early poll = %d %s", early.code, early.raw)
	}
	if _, tokenAfterEarly := f.issuer.counts(); tokenAfterEarly != tokenAfterFirst {
		t.Fatalf("early poll must make zero issuer calls (token %d->%d)", tokenAfterFirst, tokenAfterEarly)
	}

	var wg sync.WaitGroup
	results := make([]flowResponse, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows/"+flowID+"/poll", nil)
		}(i)
	}
	wg.Wait()
	for _, r := range results {
		if r.code != http.StatusOK || r.parsed["status"] != "pending" {
			t.Fatalf("concurrent poll = %d %s", r.code, r.raw)
		}
	}
	if _, tokenAfterConcurrent := f.issuer.counts(); tokenAfterConcurrent != tokenAfterFirst {
		t.Fatalf("concurrent early polls must make zero issuer calls (token %d->%d)", tokenAfterFirst, tokenAfterConcurrent)
	}

	f.expeditePoll(t, flowID)
	f.issuer.enqueue("success")
	ready := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows/"+flowID+"/poll", nil)
	if ready.code != http.StatusOK || ready.parsed["status"] != "ready" {
		t.Fatalf("ready poll = %d %s", ready.code, ready.raw)
	}
	if _, ok := ready.parsed["ready_expires_at"]; !ok {
		t.Fatalf("ready must include ready_expires_at: %s", ready.raw)
	}
	if _, hasCode := ready.parsed["user_code"]; hasCode {
		t.Fatalf("ready must not include user_code: %s", ready.raw)
	}

	save := f.call(t, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": "flow-copilot", "type": "github-copilot",
		"copilot_device_flow_id": flowID,
		"models":                 []interface{}{map[string]interface{}{"name": "m", "upstream_name": "base-model", "capabilities": []interface{}{"chat"}}},
	})
	if save.code != http.StatusCreated {
		t.Fatalf("provider save = %d %s", save.code, save.raw)
	}
	view := f.call(t, access, http.MethodGet, "/_internal/admin/providers/flow-copilot", nil)
	if view.code != http.StatusOK {
		t.Fatalf("provider view = %d %s", view.code, view.raw)
	}
	if view.parsed["copilot_credential_source"] != "database" || view.parsed["has_credential"] != true {
		t.Fatalf("source/has_credential = %v (body %s)", view.parsed, view.raw)
	}
	if _, ok := view.parsed["updated_at"]; !ok {
		t.Fatalf("provider view must expose updated_at revision: %s", view.raw)
	}
	stored, err := f.st.GetProvider(context.Background(), "flow-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.CopilotCredentialEncrypted) == 0 {
		t.Fatalf("flow save must persist encrypted DB credential")
	}
	if stored.CopilotCredentialName != "" || stored.CopilotCredentialPath != "" {
		t.Fatalf("flow save must clear sidecar refs")
	}
	t.Cleanup(func() { _ = f.st.DeleteProvider(context.Background(), stored.ID) })

	consumed := f.call(t, access, http.MethodGet, "/_internal/admin/copilot-device-flows/"+flowID, nil)
	if consumed.code != http.StatusOK || consumed.parsed["status"] != "consumed" {
		t.Fatalf("consumed status = %d %s", consumed.code, consumed.raw)
	}
	if _, ok := consumed.parsed["consumed_provider_id"]; !ok {
		t.Fatalf("consumed must reference provider: %s", consumed.raw)
	}

	replay := f.call(t, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": "flow-copilot-replay", "type": "github-copilot",
		"copilot_device_flow_id": flowID,
		"models":                 []interface{}{map[string]interface{}{"name": "m", "upstream_name": "base-model", "capabilities": []interface{}{"chat"}}},
	})
	if replay.code != http.StatusConflict {
		t.Fatalf("consumed flow replay = %d %s, want 409", replay.code, replay.raw)
	}
}

func TestAdminCopilotFlowSlowDownDenialExpiryCancel(t *testing.T) {
	f := openCopilotFlowHarness(t)
	workspace := f.workspace(t)

	_, slowAccess := f.workspaceAdmin(t, workspace)
	slow := f.startCreateFlow(t, slowAccess, workspace.ID.String())
	if slow.code != http.StatusCreated {
		t.Fatalf("start = %d %s", slow.code, slow.raw)
	}
	slowID := flowIDOf(t, slow)
	f.expeditePoll(t, slowID)
	f.issuer.enqueue("slow_down:30")
	slowed := f.call(t, slowAccess, http.MethodPost, "/_internal/admin/copilot-device-flows/"+slowID+"/poll", nil)
	if slowed.code != http.StatusOK || slowed.parsed["status"] != "pending" {
		t.Fatalf("slow_down = %d %s", slowed.code, slowed.raw)
	}
	delay, _ := slowed.parsed["poll_after_ms"].(float64)
	if delay < 20000 {
		t.Fatalf("slow_down must persist raised interval (poll_after_ms=%v): %s", delay, slowed.raw)
	}
	cancelSlow := f.call(t, slowAccess, http.MethodDelete, "/_internal/admin/copilot-device-flows/"+slowID, nil)
	if cancelSlow.code != http.StatusOK || cancelSlow.parsed["status"] != "cancelled" {
		t.Fatalf("cancel = %d %s", cancelSlow.code, cancelSlow.raw)
	}

	_, deniedAccess := f.workspaceAdmin(t, workspace)
	denied := f.startCreateFlow(t, deniedAccess, workspace.ID.String())
	if denied.code != http.StatusCreated {
		t.Fatalf("start = %d %s", denied.code, denied.raw)
	}
	deniedID := flowIDOf(t, denied)
	f.expeditePoll(t, deniedID)
	f.issuer.enqueue("denied")
	deniedPoll := f.call(t, deniedAccess, http.MethodPost, "/_internal/admin/copilot-device-flows/"+deniedID+"/poll", nil)
	if deniedPoll.code != http.StatusOK || deniedPoll.parsed["status"] != "denied" {
		t.Fatalf("denied = %d %s", deniedPoll.code, deniedPoll.raw)
	}
	if deniedPoll.parsed["error_code"] != "access_denied" {
		t.Fatalf("denied error_code = %v: %s", deniedPoll.parsed["error_code"], deniedPoll.raw)
	}

	_, expiredAccess := f.workspaceAdmin(t, workspace)
	expired := f.startCreateFlow(t, expiredAccess, workspace.ID.String())
	if expired.code != http.StatusCreated {
		t.Fatalf("start = %d %s", expired.code, expired.raw)
	}
	expiredID := flowIDOf(t, expired)
	f.expeditePoll(t, expiredID)
	f.issuer.enqueue("expired")
	expiredPoll := f.call(t, expiredAccess, http.MethodPost, "/_internal/admin/copilot-device-flows/"+expiredID+"/poll", nil)
	if expiredPoll.code != http.StatusOK || expiredPoll.parsed["status"] != "expired" {
		t.Fatalf("expired = %d %s", expiredPoll.code, expiredPoll.raw)
	}

	_, cancelAccess := f.workspaceAdmin(t, workspace)
	started := f.startCreateFlow(t, cancelAccess, workspace.ID.String())
	if started.code != http.StatusCreated {
		t.Fatalf("start = %d %s", started.code, started.raw)
	}
	cancelID := flowIDOf(t, started)
	cancelled := f.call(t, cancelAccess, http.MethodDelete, "/_internal/admin/copilot-device-flows/"+cancelID, nil)
	if cancelled.code != http.StatusOK || cancelled.parsed["status"] != "cancelled" {
		t.Fatalf("cancel = %d %s", cancelled.code, cancelled.raw)
	}
	again := f.call(t, cancelAccess, http.MethodDelete, "/_internal/admin/copilot-device-flows/"+cancelID, nil)
	if again.code != http.StatusOK || again.parsed["status"] != "cancelled" {
		t.Fatalf("idempotent cancel = %d %s", again.code, again.raw)
	}
	afterCancel := f.call(t, cancelAccess, http.MethodPost, "/_internal/admin/copilot-device-flows/"+cancelID+"/poll", nil)
	if afterCancel.code != http.StatusOK || afterCancel.parsed["status"] != "cancelled" {
		t.Fatalf("poll after cancel = %d %s, want 200 cancelled without issuer calls", afterCancel.code, afterCancel.raw)
	}
}

func TestAdminCopilotFlowAuthorityMatrix(t *testing.T) {
	f := openCopilotFlowHarness(t)
	workspace := f.workspace(t)
	_, adminAccess := f.workspaceAdmin(t, workspace)
	_, memberAccess := f.workspaceMember(t, workspace)
	otherWorkspace := f.workspace(t)
	_, otherAccess := f.workspaceAdmin(t, otherWorkspace)

	start := f.startCreateFlow(t, adminAccess, workspace.ID.String())
	if start.code != http.StatusCreated {
		t.Fatalf("start = %d %s", start.code, start.raw)
	}
	flowID := flowIDOf(t, start)
	t.Cleanup(func() {
		f.call(t, adminAccess, http.MethodDelete, "/_internal/admin/copilot-device-flows/"+flowID, nil)
	})

	if got := f.call(t, "", http.MethodGet, "/_internal/admin/copilot-device-flows/"+flowID, nil); got.code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", got.code)
	}
	if got := f.call(t, "bogus-token", http.MethodPost, "/_internal/admin/copilot-device-flows", map[string]interface{}{"client_id": "x", "workspace_id": workspace.ID.String()}); got.code != http.StatusUnauthorized {
		t.Fatalf("invalid login start = %d, want 401", got.code)
	}
	if got := f.startCreateFlow(t, memberAccess, workspace.ID.String()); got.code != http.StatusForbidden {
		t.Fatalf("member start = %d %s, want 403", got.code, got.raw)
	}
	if got := f.call(t, otherAccess, http.MethodGet, "/_internal/admin/copilot-device-flows/"+flowID, nil); got.code != http.StatusNotFound {
		t.Fatalf("cross-user status = %d %s, want 404", got.code, got.raw)
	}
	if got := f.call(t, otherAccess, http.MethodPost, "/_internal/admin/copilot-device-flows/"+flowID+"/poll", nil); got.code != http.StatusNotFound {
		t.Fatalf("cross-user poll = %d %s, want 404", got.code, got.raw)
	}
	if got := f.call(t, memberAccess, http.MethodDelete, "/_internal/admin/copilot-device-flows/"+flowID, nil); got.code != http.StatusNotFound && got.code != http.StatusForbidden {
		t.Fatalf("member cancel = %d %s, want 404/403", got.code, got.raw)
	}
}

func TestAdminCopilotFlowURLAndErrorRejection(t *testing.T) {
	f := openCopilotFlowHarness(t)
	workspace := f.workspace(t)
	_, access := f.workspaceAdmin(t, workspace)

	f.issuer.mu.Lock()
	f.issuer.verificationURI = "https://attacker.example/device"
	f.issuer.mu.Unlock()
	rejected := f.startCreateFlow(t, access, workspace.ID.String())
	if rejected.code != http.StatusCreated {
		t.Fatalf("arbitrary-URL start = %d %s", rejected.code, rejected.raw)
	}
	if rejected.parsed["status"] != "failed" || rejected.parsed["error_code"] != "authorization_failed" {
		t.Fatalf("arbitrary URL must fail safely: %s", rejected.raw)
	}
	f.issuer.mu.Lock()
	f.issuer.verificationURI = ""
	f.issuer.mu.Unlock()

	_, access2 := f.workspaceAdmin(t, workspace)
	started := f.startCreateFlow(t, access2, workspace.ID.String())
	if started.code != http.StatusCreated || started.parsed["status"] != "pending" {
		t.Fatalf("start = %d %s", started.code, started.raw)
	}
	flowID := flowIDOf(t, started)
	f.expeditePoll(t, flowID)
	f.issuer.enqueue("other", "malformed")
	other := f.call(t, access2, http.MethodPost, "/_internal/admin/copilot-device-flows/"+flowID+"/poll", nil)
	if other.code != http.StatusOK || other.parsed["status"] != "failed" {
		t.Fatalf("unknown error must fail safely = %d %s", other.code, other.raw)
	}
	if other.parsed["error_code"] != "authorization_failed" {
		t.Fatalf("error_code = %v: %s", other.parsed["error_code"], other.raw)
	}
	if strings.Contains(other.raw, "incorrect_device_code") || strings.Contains(other.raw, "unexpected") {
		t.Fatalf("raw OAuth error leaked: %s", other.raw)
	}
}

func TestAdminCopilotFlowHeldSuccessRacesAuthorityLoss(t *testing.T) {
	f := openCopilotFlowHarness(t)
	ctx := context.Background()
	workspace := f.workspace(t)
	actor, access := f.workspaceAdmin(t, workspace)

	start := f.startCreateFlow(t, access, workspace.ID.String())
	if start.code != http.StatusCreated {
		t.Fatalf("start = %d %s", start.code, start.raw)
	}
	flowID := flowIDOf(t, start)
	id, err := uuid.Parse(flowID)
	if err != nil {
		t.Fatal(err)
	}
	f.expeditePoll(t, flowID)
	status, lease, err := f.st.ClaimCopilotFlowPoll(ctx, actor.ID, id)
	if err != nil || lease == nil {
		t.Fatalf("claim = %+v %v (status %v)", lease, err, status)
	}
	successor, _ := f.workspaceAdmin(t, workspace)
	_ = successor
	if err := f.st.DeleteMembership(ctx, actor.ID, workspace.ID); err != nil {
		t.Fatal(err)
	}
	cred, credErr := copilotlogin.NewCredential(lease.ClientID(), "gho_held_success_token", time.Now())
	if credErr != nil {
		t.Fatal(credErr)
	}
	if _, err := f.st.FinalizeCopilotFlowPoll(ctx, *lease, store.CopilotFlowPollResult{State: "ready", Interval: lease.Interval(), Credential: cred}); err == nil {
		t.Fatalf("held success must not finalize after authority loss")
	}
	if _, err := f.st.ReadyCopilotFlow(ctx, actor.ID, id); err == nil {
		t.Fatalf("ready snapshot must be unavailable after authority loss")
	}
	poll := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows/"+flowID+"/poll", nil)
	if poll.code != http.StatusNotFound && poll.code != http.StatusForbidden {
		t.Fatalf("poll after authority loss = %d %s, want 404/403", poll.code, poll.raw)
	}
}

func TestAdminCopilotProviderSaveContracts(t *testing.T) {
	f := openCopilotFlowHarness(t)
	workspace := f.workspace(t)
	_, access := f.workspaceAdmin(t, workspace)

	create := f.call(t, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": "contract-copilot", "type": "github-copilot", "enabled": false,
		"models": []interface{}{map[string]interface{}{"name": "m", "upstream_name": "base-model", "capabilities": []interface{}{"chat"}}},
	})
	if create.code != http.StatusCreated {
		t.Fatalf("disabled create = %d %s", create.code, create.raw)
	}
	if create.header.Get("X-Aiproxy-Catalog-Saved") != "true" {
		t.Fatalf("create must set saved indicator")
	}
	stored, err := f.st.GetProvider(context.Background(), "contract-copilot")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.st.DeleteProvider(context.Background(), stored.ID) })

	view := f.call(t, access, http.MethodGet, "/_internal/admin/providers/contract-copilot", nil)
	if view.code != http.StatusOK {
		t.Fatalf("view = %d %s", view.code, view.raw)
	}
	if view.parsed["copilot_credential_source"] != "none" || view.parsed["has_credential"] != false {
		t.Fatalf("fresh provider source = %v: %s", view.parsed, view.raw)
	}
	revision, _ := view.parsed["updated_at"].(string)
	if revision == "" {
		t.Fatalf("view must expose updated_at: %s", view.raw)
	}

	editStart := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows", map[string]interface{}{
		"client_id": "Ov23flowtestclient", "provider_name": "contract-copilot",
	})
	if editStart.code != http.StatusCreated || editStart.parsed["status"] != "pending" {
		t.Fatalf("edit start = %d %s", editStart.code, editStart.raw)
	}
	if editStart.parsed["provider_name"] != "contract-copilot" {
		t.Fatalf("edit flow must pin provider: %s", editStart.raw)
	}
	flowID := flowIDOf(t, editStart)
	f.expeditePoll(t, flowID)
	f.issuer.enqueue("success")
	if ready := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows/"+flowID+"/poll", nil); ready.code != http.StatusOK || ready.parsed["status"] != "ready" {
		t.Fatalf("ready = %d %s", ready.code, ready.raw)
	}

	withoutRevision := f.call(t, access, http.MethodPut, "/_internal/admin/providers/contract-copilot", map[string]interface{}{
		"copilot_device_flow_id": flowID, "enabled": true,
	})
	if withoutRevision.code != http.StatusBadRequest {
		t.Fatalf("missing revision = %d %s, want 400", withoutRevision.code, withoutRevision.raw)
	}
	stale := f.call(t, access, http.MethodPut, "/_internal/admin/providers/contract-copilot", map[string]interface{}{
		"copilot_device_flow_id": flowID, "expected_updated_at": "2000-01-01T00:00:00Z", "enabled": true,
	})
	if stale.code != http.StatusConflict {
		t.Fatalf("stale revision = %d %s, want 409", stale.code, stale.raw)
	}
	if kept := f.call(t, access, http.MethodGet, "/_internal/admin/copilot-device-flows/"+flowID, nil); kept.code != http.StatusOK || kept.parsed["status"] != "ready" {
		t.Fatalf("conflict must preserve ready flow: %d %s", kept.code, kept.raw)
	}
	badModel := f.call(t, access, http.MethodPut, "/_internal/admin/providers/contract-copilot", map[string]interface{}{
		"copilot_device_flow_id": flowID, "expected_updated_at": revision, "enabled": true,
		"models": []interface{}{map[string]interface{}{"name": "m", "upstream_name": "x", "capabilities": []interface{}{"teleport"}}},
	})
	if badModel.code != http.StatusBadRequest {
		t.Fatalf("invalid model = %d %s, want 400", badModel.code, badModel.raw)
	}
	if kept := f.call(t, access, http.MethodGet, "/_internal/admin/copilot-device-flows/"+flowID, nil); kept.code != http.StatusOK || kept.parsed["status"] != "ready" {
		t.Fatalf("failed write must preserve ready flow: %d %s", kept.code, kept.raw)
	}
	ambiguous := f.call(t, access, http.MethodPut, "/_internal/admin/providers/contract-copilot", map[string]interface{}{
		"copilot_device_flow_id": flowID, "expected_updated_at": revision, "enabled": true,
		"credential_ref": map[string]interface{}{"name": "sidecar"},
	})
	if ambiguous.code != http.StatusBadRequest {
		t.Fatalf("flow+ref = %d %s, want 400", ambiguous.code, ambiguous.raw)
	}
	wrongType := f.call(t, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": "contract-openai", "type": "openai", "api_key": "secret",
		"copilot_device_flow_id": flowID,
		"models":                 []interface{}{map[string]interface{}{"name": "m", "upstream_name": "x"}},
	})
	if wrongType.code != http.StatusBadRequest {
		t.Fatalf("flow on wrong type = %d %s, want 400", wrongType.code, wrongType.raw)
	}

	applied := f.call(t, access, http.MethodPut, "/_internal/admin/providers/contract-copilot", map[string]interface{}{
		"copilot_device_flow_id": flowID, "expected_updated_at": revision, "enabled": true,
	})
	if applied.code != http.StatusOK {
		t.Fatalf("apply = %d %s", applied.code, applied.raw)
	}
	if applied.header.Get("X-Aiproxy-Catalog-Saved") != "true" {
		t.Fatalf("flow save must set saved indicator")
	}
	after := f.call(t, access, http.MethodGet, "/_internal/admin/providers/contract-copilot", nil)
	if after.code != http.StatusOK || after.parsed["copilot_credential_source"] != "database" || after.parsed["has_credential"] != true {
		t.Fatalf("saved source = %d %s", after.code, after.raw)
	}

	legacy := f.call(t, access, http.MethodPut, "/_internal/admin/providers/contract-copilot/credential", map[string]interface{}{
		"copilot_credential_name": "sidecar-main",
	})
	if legacy.code != http.StatusOK {
		t.Fatalf("legacy sidecar switch = %d %s", legacy.code, legacy.raw)
	}
	switched, err := f.st.GetProvider(context.Background(), "contract-copilot")
	if err != nil {
		t.Fatal(err)
	}
	if len(switched.CopilotCredentialEncrypted) != 0 {
		t.Fatalf("sidecar switch must clear DB ciphertext")
	}
	if switched.CopilotCredentialName != "sidecar-main" {
		t.Fatalf("sidecar switch must persist ref")
	}
	sidecarView := f.call(t, access, http.MethodGet, "/_internal/admin/providers/contract-copilot", nil)
	if sidecarView.parsed["copilot_credential_source"] != "sidecar" {
		t.Fatalf("sidecar source = %s", sidecarView.raw)
	}
	mixed := f.call(t, access, http.MethodPut, "/_internal/admin/providers/contract-copilot/credential", map[string]interface{}{
		"api_key": "nope",
	})
	if mixed.code != http.StatusBadRequest {
		t.Fatalf("api_key on copilot = %d %s, want 400", mixed.code, mixed.raw)
	}
}

func TestAdminCopilotFlowDualServiceCoherence(t *testing.T) {
	f := openCopilotFlowHarness(t)
	workspace := f.workspace(t)
	_, access := f.workspaceAdmin(t, workspace)

	deps := f.h.current()
	second := NewHandler(deps)

	startReq := httptest.NewRequest(http.MethodPost, "/_internal/admin/copilot-device-flows", strings.NewReader(`{"client_id":"Ov23flowtestclient","workspace_id":"`+workspace.ID.String()+`"}`))
	startReq.Header.Set("Authorization", "Bearer "+access)
	startW := httptest.NewRecorder()
	f.h.ServeHTTP(startW, startReq)
	if startW.Code != http.StatusCreated {
		t.Fatalf("start = %d %s", startW.Code, startW.Body.String())
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(startW.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	flowID, _ := parsed["id"].(string)
	if flowID == "" {
		t.Fatalf("missing id: %s", startW.Body.String())
	}
	t.Cleanup(func() {
		req := httptest.NewRequest(http.MethodDelete, "/_internal/admin/copilot-device-flows/"+flowID, nil)
		req.Header.Set("Authorization", "Bearer "+access)
		second.ServeHTTP(httptest.NewRecorder(), req)
	})

	getReq := httptest.NewRequest(http.MethodGet, "/_internal/admin/copilot-device-flows/"+flowID, nil)
	getReq.Header.Set("Authorization", "Bearer "+access)
	getW := httptest.NewRecorder()
	second.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("second-service status = %d %s", getW.Code, getW.Body.String())
	}
	var secondParsed map[string]interface{}
	if err := json.Unmarshal(getW.Body.Bytes(), &secondParsed); err != nil {
		t.Fatal(err)
	}
	if secondParsed["id"] != flowID || secondParsed["status"] != "pending" {
		t.Fatalf("second service sees incoherent state: %s", getW.Body.String())
	}
	assertNoFlowSecrets(t, getW.Body.String())

	delReq := httptest.NewRequest(http.MethodDelete, "/_internal/admin/copilot-device-flows/"+flowID, nil)
	delReq.Header.Set("Authorization", "Bearer "+access)
	delW := httptest.NewRecorder()
	second.ServeHTTP(delW, delReq)
	if delW.Code != http.StatusOK {
		t.Fatalf("second-service cancel = %d %s", delW.Code, delW.Body.String())
	}
	confirm := f.call(t, access, http.MethodGet, "/_internal/admin/copilot-device-flows/"+flowID, nil)
	if confirm.code != http.StatusOK || confirm.parsed["status"] != "cancelled" {
		t.Fatalf("first service must observe cancellation: %d %s", confirm.code, confirm.raw)
	}
}

func TestAdminCopilotProviderTypesDeviceCapability(t *testing.T) {
	f := openCopilotFlowHarness(t)
	_, access := f.user(t, true)
	code, body, _ := doAdmin(t, f.h, access, http.MethodGet, "/_internal/admin/provider-types", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	items, _ := body["provider_types"].([]interface{})
	byType := map[string]map[string]interface{}{}
	for _, item := range items {
		m := item.(map[string]interface{})
		byType[m["type"].(string)] = m
	}
	if byType["github-copilot"]["supports_device_authorization"] != true {
		t.Fatalf("copilot must advertise device authorization: %v", byType["github-copilot"])
	}
	if byType["openai"]["supports_device_authorization"] == true {
		t.Fatalf("openai must not advertise device authorization")
	}
}

func TestAdminCopilotReviewerDuplicateAndBinding(t *testing.T) {
	f := openCopilotFlowHarness(t)
	workspace := f.workspace(t)
	_, accessA := f.workspaceAdmin(t, workspace)
	_, accessB := f.workspaceAdmin(t, workspace)
	_, accessC := f.workspaceAdmin(t, workspace)
	ctx := context.Background()

	readyCreateFlow := func(access, workspaceID string) string {
		t.Helper()
		start := f.startCreateFlow(t, access, workspaceID)
		if start.code != http.StatusCreated {
			t.Fatalf("start = %d %s", start.code, start.raw)
		}
		id := flowIDOf(t, start)
		f.expeditePoll(t, id)
		f.issuer.enqueue("success")
		ready := f.call(t, access, http.MethodPost, "/_internal/admin/copilot-device-flows/"+id+"/poll", nil)
		if ready.code != http.StatusOK || ready.parsed["status"] != "ready" {
			t.Fatalf("ready = %d %s", ready.code, ready.raw)
		}
		return id
	}
	models := []interface{}{map[string]interface{}{"name": "m", "upstream_name": "base-model", "capabilities": []interface{}{"chat"}}}

	flowID := readyCreateFlow(accessA, workspace.ID.String())
	save := f.call(t, accessA, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": "reviewer-base", "type": "github-copilot",
		"copilot_device_flow_id": flowID, "models": models,
	})
	if save.code != http.StatusCreated {
		t.Fatalf("base save = %d %s", save.code, save.raw)
	}
	if save.header.Get("X-Aiproxy-Catalog-Saved") != "true" {
		t.Fatalf("base save must set saved indicator")
	}
	stored, err := f.st.GetProvider(ctx, "reviewer-base")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.st.DeleteProvider(ctx, stored.ID) })
	if len(stored.CopilotCredentialEncrypted) == 0 {
		t.Fatalf("base save must persist encrypted DB credential")
	}
	if bytes.Contains(stored.CopilotCredentialEncrypted, []byte("gho_flow_success_token")) {
		t.Fatalf("encrypted credential must not contain plaintext token")
	}
	cred, err := store.DecryptCopilotCredential(stored.CopilotCredentialEncrypted, time.Now())
	if err != nil {
		t.Fatalf("stored credential must decrypt: %v", err)
	}
	if cred.ClientID != "Ov23flowtestclient" {
		t.Fatalf("stored credential client = %q", cred.ClientID)
	}
	view := f.call(t, accessA, http.MethodGet, "/_internal/admin/providers/reviewer-base", nil)
	if view.code != http.StatusOK || view.parsed["copilot_credential_source"] != "database" || view.parsed["has_credential"] != true {
		t.Fatalf("base source = %d %s", view.code, view.raw)
	}

	second := NewHandler(f.h.current())
	secondGet := func(method, path string, body interface{}) flowResponse {
		t.Helper()
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(raw)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Authorization", "Bearer "+accessA)
		w := httptest.NewRecorder()
		second.ServeHTTP(w, req)
		parsed := map[string]interface{}{}
		if len(w.Body.Bytes()) > 0 {
			_ = json.Unmarshal(w.Body.Bytes(), &parsed)
		}
		out := flowResponse{code: w.Code, parsed: parsed, raw: w.Body.String(), header: w.Header()}
		assertNoFlowSecrets(t, out.raw)
		return out
	}
	if consumed := secondGet(http.MethodGet, "/_internal/admin/copilot-device-flows/"+flowID, nil); consumed.code != http.StatusOK || consumed.parsed["status"] != "consumed" {
		t.Fatalf("second instance must observe consumed flow: %d %s", consumed.code, consumed.raw)
	}
	if secondView := secondGet(http.MethodGet, "/_internal/admin/providers/reviewer-base", nil); secondView.code != http.StatusOK || secondView.parsed["copilot_credential_source"] != "database" {
		t.Fatalf("second instance must observe saved provider: %d %s", secondView.code, secondView.raw)
	}

	editStart := f.call(t, accessB, http.MethodPost, "/_internal/admin/copilot-device-flows", map[string]interface{}{
		"client_id": "Ov23flowtestclient", "provider_name": "reviewer-base",
	})
	if editStart.code != http.StatusCreated {
		t.Fatalf("edit start = %d %s", editStart.code, editStart.raw)
	}
	editID := flowIDOf(t, editStart)
	t.Cleanup(func() {
		f.call(t, accessB, http.MethodDelete, "/_internal/admin/copilot-device-flows/"+editID, nil)
	})
	f.expeditePoll(t, editID)
	f.issuer.enqueue("success")
	if ready := f.call(t, accessB, http.MethodPost, "/_internal/admin/copilot-device-flows/"+editID+"/poll", nil); ready.code != http.StatusOK || ready.parsed["status"] != "ready" {
		t.Fatalf("edit ready = %d %s", ready.code, ready.raw)
	}
	misbound := f.call(t, accessB, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": "reviewer-misbound", "type": "github-copilot",
		"copilot_device_flow_id": editID, "models": models,
	})
	if misbound.code != http.StatusBadRequest {
		t.Fatalf("edit-bound flow used for create = %d %s, want 400", misbound.code, misbound.raw)
	}
	if kept := f.call(t, accessB, http.MethodGet, "/_internal/admin/copilot-device-flows/"+editID, nil); kept.code != http.StatusOK || kept.parsed["status"] != "ready" {
		t.Fatalf("rejected binding must preserve ready flow: %d %s", kept.code, kept.raw)
	}

	raceID := readyCreateFlow(accessC, workspace.ID.String())
	raceResults := make([]flowResponse, 2)
	raceNames := []string{"reviewer-race-a", "reviewer-race-b"}
	var wg sync.WaitGroup
	for i := range raceResults {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raceResults[i] = f.call(t, accessC, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
				"name": raceNames[i], "type": "github-copilot",
				"copilot_device_flow_id": raceID, "models": models,
			})
		}(i)
	}
	wg.Wait()
	var winners, conflicts int
	var winner string
	for i, r := range raceResults {
		switch r.code {
		case http.StatusCreated:
			winners++
			winner = raceNames[i]
			if r.header.Get("X-Aiproxy-Catalog-Saved") != "true" {
				t.Fatalf("race winner must set saved indicator: %d %s", r.code, r.raw)
			}
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("race result = %d %s, want 201/409", r.code, r.raw)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("duplicate consume must yield one 201 and one 409 (winners=%d conflicts=%d)", winners, conflicts)
	}
	tombstone := f.call(t, accessC, http.MethodGet, "/_internal/admin/copilot-device-flows/"+raceID, nil)
	if tombstone.code != http.StatusOK || tombstone.parsed["status"] != "consumed" {
		t.Fatalf("race tombstone = %d %s", tombstone.code, tombstone.raw)
	}
	if _, ok := tombstone.parsed["consumed_provider_id"]; !ok {
		t.Fatalf("race tombstone must reference winning provider: %s", tombstone.raw)
	}
	for _, name := range raceNames {
		p, err := f.st.GetProvider(ctx, name)
		if name == winner {
			if err != nil {
				t.Fatalf("winning provider %q must exist: %v", name, err)
			}
			t.Cleanup(func() { _ = f.st.DeleteProvider(ctx, p.ID) })
			continue
		}
		if err == nil {
			t.Cleanup(func() { _ = f.st.DeleteProvider(ctx, p.ID) })
			t.Fatalf("losing provider %q must not exist", name)
		}
	}
}
