package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type deferredQuotaStore struct {
	reads        atomic.Int32
	inserts      atomic.Int32
	deadlineRead atomic.Bool
	mu           sync.Mutex
	entries      []store.SpendEntry
	deadlines    []time.Time
	budget       store.ScopeQuota
	model        store.ScopeQuota
	spent        int64
	failure      error
	insert       func(context.Context, *store.SpendEntry) error
}

func (s *deferredQuotaStore) GetScopeQuota(ctx context.Context, _ uuid.UUID, _ string, _ uuid.UUID, model string) (store.ScopeQuota, error) {
	s.reads.Add(1)
	if _, ok := ctx.Deadline(); ok {
		s.deadlineRead.Store(true)
	}
	if s.failure != nil {
		return store.ScopeQuota{}, s.failure
	}
	if model == "" && s.budget.BudgetMicros > 0 {
		return s.budget, nil
	}
	if model != "" && s.model.TPMEffective > 0 {
		return s.model, nil
	}
	return store.ScopeQuota{}, sql.ErrNoRows
}

func (s *deferredQuotaStore) SumScopeSpend(context.Context, uuid.UUID, string, uuid.UUID) (int64, error) {
	return s.spent, s.failure
}

func (s *deferredQuotaStore) InsertSpendEntry(ctx context.Context, entry *store.SpendEntry) error {
	s.inserts.Add(1)
	deadline, _ := ctx.Deadline()
	s.mu.Lock()
	s.entries = append(s.entries, *entry)
	s.deadlines = append(s.deadlines, deadline)
	s.mu.Unlock()
	if s.insert != nil {
		return s.insert(ctx, entry)
	}
	return ctx.Err()
}

func dynamicDeferredClient() auth.DynamicClient {
	return auth.DynamicClient{
		TokenHash: store.TokenHash("peer-token"), Name: "dynamic-name", Tenant: "dynamic-usage-tenant",
		KeyID: uuid.New(), Workspace: identity.InWorkspace(uuid.New()), OwnerUserID: uuid.New(),
	}
}

func configureDynamicDeferred(f *deferredHTTPFixture, clients []auth.DynamicClient, quota *deferredQuotaStore, now func() time.Time) {
	f.rt.Auth = config.Auth{Mode: config.AuthModeBearerStatic}
	deps := f.h.current()
	deps.Auth = auth.NewAuthenticatorWithClientsAndClock(f.rt.Auth, clients, now)
	deps.Authorizer = auth.NewAuthorizerWithClients(f.rt.Auth, clients)
	deps.MultiTenancy = config.MultiTenancy{Enabled: true}
	deps.Quota = NewQuotaTrackerWithStore(quota)
	f.rt.Catalog = withTestWorkspace(f.rt.Catalog, clients[0].Workspace)
	deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.NewWithPrevious(f.rt, deps.Resolver)
	f.h.UpdateDependencies(deps)
}

func withTestWorkspace(catalog config.Catalog, workspace identity.WorkspaceScope) config.Catalog {
	providers, aliases := catalog.Providers(), catalog.Aliases()
	for i := range providers {
		if _, scoped := providers[i].Workspace.ID(); !scoped {
			providers[i].Workspace = workspace
		}
	}
	for i := range aliases {
		if _, scoped := aliases[i].Workspace.ID(); !scoped {
			aliases[i].Workspace = workspace
		}
	}
	return config.NewCatalog(providers, catalog.DisabledProviders(), aliases)
}

func TestDeferredSharedAdmissionPolicyParityAndRejectedDedup(t *testing.T) {
	for _, mode := range []string{"rate", "model_denied", "unresolved", "budget", "tpm", "quota_unavailable", "capability"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
				calls.Add(1)
				return nil, fmt.Errorf("unexpected inference")
			}))
			client, quota := dynamicDeferredClient(), &deferredQuotaStore{}
			configureDynamicDeferred(f, []auth.DynamicClient{client}, quota, time.Now)
			request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m","messages":[]}`))
			expected := ""
			switch mode {
			case "rate":
				f.rate.allowed.Store(false)
				expected = "rate_limited"
			case "model_denied":
				deps := f.h.current()
				deps.Authorizer = auth.AuthorizerFunc(func(*auth.Principal, string) bool { return false })
				f.h.UpdateDependencies(deps)
				expected = "forbidden"
			case "unresolved":
				f.rt.Catalog = config.NewCatalog(nil, nil, nil)
				deps := f.h.current()
				deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
				f.h.UpdateDependencies(deps)
				expected = "model_not_found"
			case "budget":
				quota.budget.BudgetMicros, quota.spent = 100, 100
				expected = "budget_exceeded"
			case "tpm":
				quota.model.TPMEffective = 10
				workspaceID, _ := client.Workspace.ID()
				f.h.current().Quota.recordTokens(workspaceID, quotaScope{typ: quotaScopeUser, id: client.OwnerUserID}, "p/m", 10)
				expected = "tpm_exceeded"
			case "quota_unavailable":
				quota.failure = errors.New("private database detail")
				expected = "quota_unavailable"
			case "capability":
				p := f.rt.Catalog.Providers()[0]
				p.Models[0].Capabilities = []config.Capability{config.CapabilityEmbeddings}
				p.ModelByName = nil
				f.rt.Catalog = config.NewCatalog([]config.Provider{p}, nil, nil)
				deps := f.h.current()
				deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
				f.h.UpdateDependencies(deps)
				expected = "unsupported_operation"
			}
			ordinaryRequest := f.request("POST", "/v1/chat/completions", "peer-token", request.Body)
			ordinaryRequest.Header.Set("Content-Type", "application/json")
			ordinary := httptest.NewRecorder()
			f.h.ServeHTTP(ordinary, ordinaryRequest)
			first := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
			var ordinaryError, deferredError apiError
			_ = json.Unmarshal(ordinary.Body.Bytes(), &ordinaryError)
			_ = json.Unmarshal(first.Body.Bytes(), &deferredError)
			if first.Code != ordinary.Code || ordinaryError.Error.Type != expected || deferredError.Error.Type != expected {
				t.Fatalf("policy diverged: ordinary=%d %s deferred=%d %s", ordinary.Code, ordinary.Body.String(), first.Code, first.Body.String())
			}
			if bytes.Contains(first.Body.Bytes(), []byte("private database detail")) {
				t.Fatal("quota backend details leaked into an admission response")
			}
			rateCalls, quotaReads := f.rate.calls.Load(), quota.reads.Load()
			replay := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
			if replay.Code != first.Code || replay.Body.String() != first.Body.String() || f.rate.calls.Load() != rateCalls ||
				quota.reads.Load() != quotaReads || calls.Load() != 0 || quota.inserts.Load() != 0 {
				t.Fatal("rejected qualified submit reran policy or dispatched inference")
			}
		})
	}
}

func TestDeferredDynamicCanonicalOwnershipExpiryReplacementAndAnonymousControls(t *testing.T) {
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(ctx context.Context, r provider.Request) (*provider.Result, error) {
		select {
		case <-gate:
			return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`),
				Usage: provider.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))
	t.Cleanup(release)
	client, quota := dynamicDeferredClient(), &deferredQuotaStore{}
	now := time.Now()
	client.ExpiresAt = now.Add(time.Minute)
	configureDynamicDeferred(f, []auth.DynamicClient{client}, quota, func() time.Time { return now })
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`))
	receipt := f.submit(t, request)
	if !quota.deadlineRead.Load() {
		t.Fatal("deferred quota admission used an unbounded background context")
	}
	replacement := client
	replacement.KeyID = uuid.New()
	configureDynamicDeferred(f, []auth.DynamicClient{replacement}, quota, time.Now)
	w := f.call("DELETE", deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
	if w.Code != 404 || controlErrorType(t, w) != "job_unavailable" {
		t.Fatal("same display name replacement key inherited canonical ownership")
	}
	workspaceReplacement := client
	workspaceReplacement.Workspace = identity.InWorkspace(uuid.New())
	configureDynamicDeferred(f, []auth.DynamicClient{workspaceReplacement}, quota, time.Now)
	w = f.call("GET", deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
	if w.Code != 404 {
		t.Fatal("a different workspace inherited dynamic key ownership")
	}
	configureDynamicDeferred(f, []auth.DynamicClient{client}, quota, func() time.Time { return client.ExpiresAt })
	w = f.call("GET", deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
	if w.Code != 401 {
		t.Fatal("expired dynamic key continued controlling a job")
	}
	deps := f.h.current()
	deps.Auth, deps.Authorizer = auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}), auth.NewAuthorizer(config.Auth{Mode: config.AuthModeNone})
	f.h.UpdateDependencies(deps)
	for _, path := range []string{deferred.Prefix + "/capabilities?model=p/m", deferred.Prefix + "/jobs/" + receipt.ID} {
		w := f.call("GET", path, "peer-token", nil)
		if w.Code != 401 || controlErrorType(t, w) != "auth_failed" {
			t.Fatal("anonymous auth granted deferred identity/control access")
		}
	}
	w = f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
	if w.Code != 401 {
		t.Fatal("anonymous auth granted replay/admission access")
	}
	configureDynamicDeferred(f, []auth.DynamicClient{client}, quota, func() time.Time { return now })
	w = f.call("GET", deferred.Prefix+"/jobs/"+receipt.ID, "eyJhbGciOiJIUzI1NiJ9.eyJyb2xlIjoic3lzdGVtX2FkbWluIn0.invalid", nil)
	if w.Code != 401 {
		t.Fatal("dashboard/admin-shaped JWT shortcut bypassed ordinary inference auth")
	}
	release()
	f.finish(t, receipt)
	workspaceID, _ := client.Workspace.ID()
	if quota.inserts.Load() != 1 || len(f.accounting.Events()) != 1 || f.accounting.Events()[0].Client != client.Name ||
		f.accounting.Events()[0].Tenant != client.Tenant ||
		f.accounting.Events()[0].CredentialID != identity.DatabaseKeyID(client.KeyID) || f.accounting.Events()[0].WorkspaceScope != client.Workspace ||
		quota.entries[0].KeyID != client.KeyID || quota.entries[0].WorkspaceID != workspaceID ||
		quota.deadlines[0].IsZero() || quota.entries[0].Tokens != 7 {
		t.Fatal("accepted work lost its admitted execution identity or duplicated persistence")
	}
}

func TestDeferredSystemKeyAndStaticPeerHaveDistinctOwners(t *testing.T) {
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}))
	cfg := config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{
		"peer": {Token: "file-peer", Tenant: "shared-label"},
	}}
	client := auth.DynamicClient{
		TokenHash: store.TokenHash("peer-token"), Name: "peer", Tenant: "shared-label",
		KeyID: uuid.New(), Workspace: identity.InWorkspace(store.SystemWorkspaceID),
	}
	f.rt.Catalog = withTestWorkspace(f.rt.Catalog, client.Workspace)
	publish := func(client auth.DynamicClient) auth.Authenticator {
		deps := f.h.current()
		deps.MultiTenancy.Enabled = true
		deps.Auth = auth.NewAuthenticatorWithClients(cfg, []auth.DynamicClient{client})
		deps.Authorizer = auth.NewAuthorizerWithClients(cfg, []auth.DynamicClient{client})
		deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
		f.h.UpdateDependencies(deps)
		return deps.Auth
	}
	a := publish(client)
	for _, tc := range []struct {
		token string
		owner deferred.Owner
	}{
		{"peer-token", deferred.Owner{Kind: "key", ID: client.KeyID.String(), Workspace: store.SystemWorkspaceID.String()}},
		{"file-peer", deferred.Owner{Kind: "static", ID: "peer", Tenant: "shared-label"}},
	} {
		owner, err := deferredOwner(authenticatedTestPrincipal(t, a, tc.token))
		if err != nil || owner != tc.owner {
			t.Errorf("peer owner = %+v, %v; want %+v", owner, err, tc.owner)
		}
	}
	for _, p := range []*auth.Principal{nil, {Name: "peer", Tenant: "shared-label"}} {
		if _, err := deferredOwner(p); err != auth.ErrInvalidToken {
			t.Errorf("absent or label-only principal: owner error = %v", err)
		}
	}
	capabilities := deferred.Prefix + "/capabilities?model=p/m"
	if w := f.call(http.MethodGet, capabilities, "file-peer", nil); w.Code != http.StatusOK {
		t.Fatalf("static peer capability = %d %s", w.Code, w.Body.String())
	}
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`))
	receipt := f.submit(t, request)
	f.finish(t, receipt)
	job := deferred.Prefix + "/jobs/" + receipt.ID
	if w := f.call(http.MethodGet, job, "file-peer", nil); w.Code != http.StatusNotFound || controlErrorType(t, w) != "job_unavailable" {
		t.Fatalf("same-named static peer did not remain a separate owner: %d %s", w.Code, w.Body.String())
	}
	client.TokenHash, client.Name, client.Tenant = store.TokenHash("rotated-peer"), "renamed-peer", "other-label"
	publish(client)
	if w := f.call(http.MethodGet, job, "peer-token", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("old bearer still accepted: %d %s", w.Code, w.Body.String())
	}
	if w := f.call(http.MethodGet, job, "rotated-peer", nil); w.Code != http.StatusOK {
		t.Fatalf("rotated System peer lost ownership: %d %s", w.Code, w.Body.String())
	}
	w := f.call(http.MethodPost, deferred.Prefix+"/submit", "rotated-peer", submitWire(t, request))
	var replay deferred.Receipt
	if w.Code != http.StatusAccepted || deferred.DecodeControl(w.Body.Bytes(), &replay) != nil ||
		replay.ID != receipt.ID || !replay.Replayed || calls.Load() != 1 || f.rate.calls.Load() != 1 {
		t.Fatalf("rotation did not retain accepted System replay: %d %s", w.Code, w.Body.String())
	}
	client.Workspace = identity.WorkspaceScope{}
	publish(client)
	if w := f.call(http.MethodGet, capabilities, "rotated-peer", nil); w.Code != http.StatusUnauthorized || controlErrorType(t, w) != "auth_failed" {
		t.Fatalf("unscoped database credential obtained peer capability: %d %s", w.Code, w.Body.String())
	}
}

func TestDeferredGuardrailRewriteFingerprintPayloadBoundsAndRejectedBlockReplay(t *testing.T) {
	key := guardrailTestKey(t)
	received := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[],"vendor":"response"}`)
	}))
	t.Cleanup(upstream.Close)
	f := newDeferredHTTPFixture(t, provider.New())
	p := f.rt.Catalog.Providers()[0]
	p.BaseURL = upstream.URL + "/v1"
	f.rt.Catalog = config.NewCatalog([]config.Provider{p}, nil, nil)
	exceptions := exceptionTestStore(t)
	scanner := guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
	quarantine := guardrails.NewQuarantine(guardrails.QuarantinePolicy{Enabled: true})
	payload := &deferredPayloadRecorder{max: 64}
	deps := f.h.current()
	deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
	deps.Guardrails, deps.Quarantine, deps.Exceptions = scanner, quarantine, exceptions
	deps.PayloadLog, deps.RedactGuardrailBodies = payload, true
	f.h.UpdateDependencies(deps)
	original := []byte(guardrailChatBody("p/m", "deploy with "+key))
	capability := f.capability(t, "p/m")
	blocked := deferredSubmitRequest(t, capability, original)
	first := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, blocked))
	if first.Code != 400 || guardrailErrorType(t, first.Body.String()) != guardrailBlockType {
		t.Fatalf("guardrail rejection: %d %s", first.Code, first.Body.String())
	}
	blockID := blockIDOf(t, first.Body.String())
	shas := blockFindingSHAs(t, quarantine, blockID)
	if _, err := exceptions.Decide(shas, guardrails.ExceptionActionRedact, "executor-test", nil); err != nil {
		t.Fatal(err)
	}
	rejectedReplay := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, blocked))
	if rejectedReplay.Code != 400 || blockIDOf(t, rejectedReplay.Body.String()) != blockID || f.rate.calls.Load() != 1 {
		t.Fatal("cached guardrail rejection lost its block ID or reran scanning after policy change")
	}
	request := deferredSubmitRequest(t, capability, original)
	receipt := f.submit(t, request)
	f.finish(t, receipt)
	sent := <-received
	if bytes.Contains(sent, []byte(key)) || !bytes.Contains(sent, []byte(guardrails.DefaultRedactPlaceholder)) || extractModel(sent) != "actual" {
		t.Fatalf("deferred dispatch lost guardrail/model rewrites: %s", sent)
	}
	expected, _ := deferred.Fingerprint("p/m", original, request.Headers)
	if receipt.Fingerprint != expected {
		t.Fatal("redacted execution input replaced the pre-guardrail fingerprint")
	}
	entries := payload.snapshot()
	if len(entries) != 1 || entries[0].Path != "/v1/chat/completions" || !entries[0].UpstreamRequest.Body.Truncated {
		t.Fatalf("payload capture did not preserve bounded upstream snapshot metadata: %+v", entries)
	}
	encoded, _ := json.Marshal(entries[0])
	if bytes.Contains(encoded, []byte(key)) || bytes.Contains(encoded, []byte("model-secret")) || bytes.Contains(encoded, []byte(request.SubmitToken)) ||
		bytes.Contains(encoded, []byte("peer-token")) || bytes.Contains(encoded, []byte(deferred.Protocol)) {
		t.Fatal("payload log leaked private envelopes, credentials, or guardrail secrets")
	}
}

func TestDeferredCanonicalModelCannotBeOverriddenByUnknownCaseVariant(t *testing.T) {
	got := make(chan provider.Request, 1)
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(_ context.Context, request provider.Request) (*provider.Result, error) {
		got <- request
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}))
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m","Model":"forbidden/target","vendor":{"keep":true}}`))
	receipt := f.submit(t, request)
	f.finish(t, receipt)
	inference := <-got
	if inference.PublicModel != "p/m" || inference.UpstreamModel != "actual" || !bytes.Contains(inference.Body, []byte(`"Model":"forbidden/target"`)) {
		t.Fatal("an unknown case variant changed admitted model identity or was discarded")
	}
	if inference.ExecutionLimits == nil || inference.ExecutionLimits.Work == nil || inference.ExecutionLimits.SnapshotBytes != 0 ||
		inference.Inbound.URL.Path != "/v1/chat/completions" || inference.Inbound.Header.Get("Authorization") != "" ||
		strings.Contains(inference.Inbound.URL.Path, deferredPath) {
		t.Fatal("job retained private control request metadata or failed to pass frozen execution limits")
	}
}

func TestDeferredPrivateEndpointsRejectValidAdminJWTAndUntrustedOwnerHeaders(t *testing.T) {
	t.Setenv("AIPROXY_JWT_SECRET", "deferred-administrator-test-signing-secret")
	admin, _, err := adminauth.IssueAccess(uuid.NewString(), "admin@example.invalid", true)
	if err != nil {
		t.Fatal(err)
	}
	if claims, err := adminauth.VerifyAccess(admin); err != nil || !claims.IsAdmin {
		t.Fatal("administrator exclusion fixture did not contain a valid signed admin JWT")
	}
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}))
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`))
	receipt := f.submit(t, request)
	f.finish(t, receipt)
	deps := f.h.current()
	deps.MultiTenancy.Enabled, deps.AdminStore = true, &store.Store{}
	f.h.UpdateDependencies(deps)
	base := deferred.Prefix + "/jobs/" + receipt.ID
	for _, control := range []struct {
		method, path string
		body         []byte
	}{
		{"GET", deferred.Prefix + "/capabilities?model=p/m", nil},
		{"POST", deferred.Prefix + "/submit", submitWire(t, request)},
		{"GET", base, nil}, {"GET", base + "/result", nil}, {"GET", base + "/read?cursor=0", nil}, {"DELETE", base, nil},
	} {
		w := f.call(control.method, control.path, admin, control.body)
		if w.Code != 401 || controlErrorType(t, w) != "auth_failed" || bytes.Contains(w.Body.Bytes(), []byte(receipt.ID)) {
			t.Fatal("valid admin JWT granted private inference ownership or disclosed a job")
		}
	}
	for _, method := range []string{"GET", "DELETE"} {
		r := f.request(method, base, "other-token", nil)
		r.Header.Set("X-User-Id", "peer")
		r.Header.Set("X-Client-Id", "peer")
		r.Header.Set("X-Tenant", "tenant")
		w := httptest.NewRecorder()
		f.h.ServeHTTP(w, r)
		if w.Code != 404 || controlErrorType(t, w) != "job_unavailable" {
			t.Fatal("untrusted end-user/tenant metadata replaced authenticated ownership")
		}
	}
	if calls.Load() != 1 || f.rate.calls.Load() != 1 || len(f.accounting.Events()) != 1 {
		t.Fatal("excluded identity/control requests consumed inference policy")
	}
}
