package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/egose/aiproxy/internal/upstreamhttp"
	"github.com/egose/aiproxy/internal/upstreampolicy"
	"github.com/google/uuid"
)

func TestTenantTransportPoolSeparatesTrustedAndRestrictedPolicyKeys(t *testing.T) {
	policy, _ := upstreampolicy.New([]string{"https://api.example.com"})
	other, _ := upstreampolicy.New([]string{"https://api.example.com", "https://probe.example.com"})
	trusted := config.Provider{Name: "system", Type: "openai", Workspace: identity.InWorkspace(store.SystemWorkspaceID), Enabled: true}
	tenant := config.Provider{Name: "tenant", Type: "openai", Workspace: identity.InWorkspace(uuid.New()), DatabaseManaged: true, OutboundPolicy: policy, Enabled: true}
	second := tenant
	second.Name, second.OutboundPolicy = "second", other
	created := map[string]int{}
	pool, err := prepareUpstreamClientPool(config.NewCatalog([]config.Provider{trusted, tenant, second}, nil, nil), func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
		created[options.OutboundPolicy.Identity()]++
		return upstreamhttp.NewTransport(options)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Retire()
	if len(created) != 3 || created["trusted"] != 1 || created[policy.Identity()] != 1 || created[other.Identity()] != 1 ||
		pool.ClientForProvider(trusted) == pool.ClientForProvider(tenant) || pool.ClientForProvider(tenant) == pool.ClientForProvider(second) {
		t.Fatal("trusted, tenant and differing approval snapshots shared a transport cache entry")
	}
	if err := pool.prepareProviders(config.NewCatalog([]config.Provider{tenant, second}, nil, nil)); err != nil || len(pool.clients) != 3 {
		t.Fatal("prepared restricted transports were not cached by immutable policy identity")
	}
	missing := tenant
	missing.OutboundPolicy = nil
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1/models", nil)
	if _, err := pool.ClientForProvider(missing).Do(req); err == nil || !strings.Contains(err.Error(), "not prepared") {
		t.Fatal("missing tenant restriction fell back to a trusted client")
	}
}

func TestAppTenantProviderPolicyInferenceProbesAndRejectedActivation(t *testing.T) {
	st, dsn := workspaceInferenceDB(t)
	ctx := context.Background()
	workspace := store.Workspace{Name: "tenant-policy"}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	administrator := store.User{Email: "policy-admin@example.test", IsAdmin: true}
	if err := st.CreateUser(ctx, &administrator); err != nil {
		t.Fatal(err)
	}
	access, _, err := adminauth.IssueAccess(administrator.ID.String(), administrator.Email, true)
	if err != nil {
		t.Fatal(err)
	}
	var inferenceCalls, probeCalls, copilotCalls, systemCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inferenceCalls.Add(1)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer own-api-fixture" {
			t.Error("approved inference did not use the tenant's own DB credential")
		}
		var body struct{ Stream bool }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[]}\n\ndata: [DONE]\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[]}`)
		}
	}))
	defer upstream.Close()
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeCalls.Add(1)
		if r.URL.Path != "/ready" || r.Header.Get("Authorization") != "Bearer own-api-fixture" {
			t.Error("approved absolute probe did not use the own credential")
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer probe.Close()
	copilot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		copilotCalls.Add(1)
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer own-copilot-fixture" {
			t.Error("approved Copilot inference did not use the tenant's own encrypted DB credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer copilot.Close()
	system := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		systemCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer system-fixture" {
			t.Error("trusted System credential changed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer system.Close()
	encrypted, err := store.EncryptSecret([]byte("own-api-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	own := store.DBProvider{Name: "own", Type: "openai", WorkspaceID: workspace.ID, BaseURL: "https://api.example.com/v1", Enabled: true,
		APIKeyEncrypted: encrypted, TLSClientHello: "chrome", Healthcheck: json.RawMessage(`{"path":"https://probe.example.com:8443/ready","method":"GET","expected_status":200,"expected_body":"*","interval":"1h","timeout":"1s","failure_threshold":1,"success_threshold":1,"send_authorization":true}`)}
	if err := st.CreateProviderAggregate(ctx, &own, []store.DBProviderModel{{Name: "m"}}); err != nil {
		t.Fatal(err)
	}
	cred, err := copilotlogin.NewCredential("public-client-id", "own-copilot-fixture", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	copilotEncrypted, err := store.EncryptCopilotCredential(cred, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	copilotRow := store.DBProvider{Name: "copilot", Type: "github-copilot", WorkspaceID: workspace.ID, Enabled: true, CopilotCredentialEncrypted: copilotEncrypted}
	if err := st.CreateProviderAggregate(ctx, &copilotRow, []store.DBProviderModel{{Name: "m"}}); err != nil {
		t.Fatal(err)
	}
	key := store.InboundKey{Name: "tenant", TokenHash: store.TokenHash("tenant-fixture"), TokenPrefix: "fixture", Enabled: true, WorkspaceID: workspace.ID}
	if err := st.CreateInboundKey(ctx, &key); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" {
  mode = "bearer_static"
  client "operator" { token = "system-key-fixture" }
}
provider "openai-compatible" "system" {
  base_url = %q
  api_key = "system-fixture"
  model "m" {}
}
database { url = %q }
multi_tenancy {
  allowed_upstream_origins = ["https://api.example.com", "https://probe.example.com:8443", "https://api.githubcopilot.com"]
}
`, system.URL, dsn)
	path := writeConfigFile(t, cfg)
	var factoryCalls atomic.Int32
	mockFactory := tenantMockTransportFactory(map[string]string{"api.example.com": upstream.URL, "probe.example.com": probe.URL, "api.githubcopilot.com": copilot.URL})
	options := BuildOptions{ConfigPath: path, LogOutput: io.Discard, transportFactory: func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
		factoryCalls.Add(1)
		return mockFactory(options)
	}}
	a, err := Build(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	awaitResourceCondition(t, "approved tenant absolute health probe", func() bool {
		status, ok := a.healthchecks.StatusFor("own")
		return ok && status.Checked && status.Healthy
	})
	if probeCalls.Load() != 1 || inferenceCalls.Load() != 0 || copilotCalls.Load() != 0 {
		t.Fatal("activation did not isolate the approved probe from inference")
	}
	call := func(token, model string, stream bool) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[],"stream":%t}`, model, stream)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Server.Handler.ServeHTTP(w, r)
		if w.Code != 200 || (stream && !strings.Contains(w.Body.String(), "[DONE]")) {
			t.Fatalf("approved inference %s = %d %s", model, w.Code, w.Body.String())
		}
	}
	call("tenant-fixture", "own/m", false)
	call("tenant-fixture", "own/m", true)
	call("tenant-fixture", "copilot/m", false)
	call("system-key-fixture", "system/m", false)
	if inferenceCalls.Load() != 2 || copilotCalls.Load() != 1 || systemCalls.Load() != 1 {
		t.Fatal("approved own DB credentials or trusted HCL behavior failed")
	}
	snapshot := func() [4]int32 {
		return [4]int32{inferenceCalls.Load(), probeCalls.Load(), copilotCalls.Load(), systemCalls.Load()}
	}
	admin := func(method, resource, raw string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/_internal/admin/"+resource, strings.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+access)
		r.Header.Set("X-Workspace-ID", workspace.ID.String())
		w := httptest.NewRecorder()
		a.Server.Handler.ServeHTTP(w, r)
		return w
	}
	for _, body := range []string{
		`{"base_url":"https://127.0.0.1/v1"}`, `{"base_url":"http://api.example.com/v1"}`,
		`{"forward_authorization":true}`, `{"allow_insecure_base_url":true}`,
		`{"api_key_ref":{"path":"/private/keys.json","key":"host"}}`,
		`{"healthcheck":{"path":"https://169.254.169.254/latest"}}`,
		`{"healthcheck":{"path":"https://probe.example.com/ready"}}`,
	} {
		before, beforeFactories := snapshot(), factoryCalls.Load()
		saved, err := st.GetProvider(ctx, own.Name)
		if err != nil {
			t.Fatal(err)
		}
		w := admin("PUT", "providers/own", body)
		if w.Code != 400 || w.Header().Get("X-Aiproxy-Catalog-Saved") != "" || snapshot() != before || factoryCalls.Load() != beforeFactories {
			t.Fatalf("global-author tenant edit caused outbound/preparation/persistence: %d %s", w.Code, w.Body.String())
		}
		after, err := st.GetProvider(ctx, own.Name)
		if err != nil || !reflect.DeepEqual(saved, after) {
			t.Fatal("rejected policy edit changed the database aggregate")
		}
	}
	before := snapshot()
	active := a.Config
	legacy := own
	legacy.BaseURL, legacy.APIKeyEncrypted = "https://169.254.169.254/latest", []byte("corrupt-must-not-decrypt")
	if err := st.UpdateProvider(ctx, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy, err = st.GetProvider(ctx, legacy.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "operator-approved") || strings.Contains(err.Error(), "decrypt") {
		t.Fatalf("old-row reload did not reject before decryption: %v", err)
	}
	candidate, err := Build(ctx, options)
	if candidate != nil {
		_ = candidate.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "operator-approved") || strings.Contains(err.Error(), "decrypt") ||
		a.Config != active || snapshot() != before {
		t.Fatalf("old-row startup/reload made outbound calls or replaced the active runtime: %v", err)
	}
	for _, resource := range []string{"providers/own", "providers/own/credential"} {
		w := admin("PUT", resource, `{"api_key":"replacement-never-saved"}`)
		if w.Code != 400 || w.Header().Get("X-Aiproxy-Catalog-Saved") != "" || snapshot() != before {
			t.Fatalf("credential edit of forbidden old row caused I/O: %d %s", w.Code, w.Body.String())
		}
	}
	after, err := st.GetProvider(ctx, legacy.Name)
	if err != nil || !reflect.DeepEqual(legacy, after) {
		t.Fatal("rejected activation or credential mutation modified the old row")
	}
	call("tenant-fixture", "own/m", false)
	if inferenceCalls.Load() != before[0]+1 {
		t.Fatal("failed reload did not retain the old permitted runtime")
	}
	own.UpdatedAt = legacy.UpdatedAt
	if err := st.UpdateProvider(ctx, &own); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	current, _ := a.Config.Catalog.Provider("own")
	if current.OutboundPolicy == nil || current.APIKey != "own-api-fixture" {
		t.Fatal("repair/reload lost the restricted credential snapshot")
	}
}
