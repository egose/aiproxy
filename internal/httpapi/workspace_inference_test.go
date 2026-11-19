package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/alias"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestHostedInferenceScopeAcrossOperations(t *testing.T) {
	a, b, empty, system := identity.InWorkspace(uuid.New()), identity.InWorkspace(uuid.New()), identity.InWorkspace(uuid.New()), identity.InWorkspace(store.SystemWorkspaceID)
	var providers []config.Provider
	var aliases []config.Alias
	for _, owner := range []struct {
		name      string
		workspace identity.WorkspaceScope
	}{{"a", a}, {"b", b}, {"system", system}} {
		for _, native := range []bool{false, true} {
			name, typ := owner.name, config.ProviderTypeOpenAI
			caps := []config.Capability{config.CapabilityChat, config.CapabilityResponses, config.CapabilityEmbeddings,
				config.CapabilityImages, config.CapabilityAudioTranscriptions, config.CapabilityAudioSpeech}
			if native {
				name, typ, caps = name+"-native", config.ProviderTypeAnthropic, []config.Capability{config.CapabilityMessages}
			}
			providers = append(providers, config.Provider{Name: name, Type: typ, Workspace: owner.workspace,
				Models: []config.Model{{Name: "m", UpstreamName: "m", Capabilities: caps}}})
			aliases = append(aliases, config.Alias{Name: name, Workspace: owner.workspace, Algorithm: config.AlgorithmRoundRobin,
				Targets: []config.AliasTarget{{Provider: name, Model: "m"}}})
		}
	}
	cfg := config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"system": {Token: "system-token"}}}
	dynamic := []auth.DynamicClient{
		{TokenHash: store.TokenHash("a-token"), KeyID: uuid.New(), Workspace: a},
		{TokenHash: store.TokenHash("empty-token"), KeyID: uuid.New(), Workspace: empty},
		{TokenHash: store.TokenHash("a-explicit"), KeyID: uuid.New(), Workspace: a, AllowedModels: []string{
			"alias/a", "alias/a-native", "b/m", "alias/b", "b-native/m", "alias/b-native",
			"system/m", "alias/system", "system-native/m", "alias/system-native", "missing/m", "alias/missing",
		}},
	}
	rt := &config.Runtime{Catalog: config.NewCatalog(providers, nil, aliases)}
	var calls atomic.Int32
	deps := Dependencies{Catalog: rt.Catalog, Resolver: modelresolver.New(rt), MultiTenancy: config.MultiTenancy{Enabled: true},
		Auth: auth.NewAuthenticatorWithClients(cfg, dynamic), Authorizer: auth.NewAuthorizerWithClients(cfg, dynamic),
		Adapter: deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
			calls.Add(1)
			return &provider.Result{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
		}), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, op := range []struct {
		name, path, body string
		native           bool
	}{
		{"chat-json", "/v1/chat/completions", `{"model":%q,"messages":[]}`, false},
		{"chat-sse", "/v1/chat/completions", `{"model":%q,"messages":[],"stream":true}`, false},
		{"messages-json", "/v1/messages", `{"model":%q,"messages":[]}`, true},
		{"messages-sse", "/v1/messages", `{"model":%q,"messages":[],"stream":true}`, true},
		{"responses-json", "/v1/responses", `{"model":%q,"input":"hi"}`, false},
		{"responses-sse", "/v1/responses", `{"model":%q,"input":"hi","stream":true}`, false},
		{"embeddings", "/v1/embeddings", `{"model":%q,"input":"hi"}`, false},
		{"images", "/v1/images/generations", `{"model":%q,"prompt":"hi"}`, false},
		{"audio-speech", "/v1/audio/speech", `{"model":%q,"input":"hi","voice":"alloy"}`, false},
		{"audio-transcriptions", "/v1/audio/transcriptions", "", false},
	} {
		t.Run(op.name, func(t *testing.T) {
			h := NewHandler(deps)
			call := func(token, model string) *httptest.ResponseRecorder {
				t.Helper()
				contentType, body := "application/json", []byte(fmt.Sprintf(op.body, model))
				if op.body == "" {
					var buffer bytes.Buffer
					form := multipart.NewWriter(&buffer)
					if err := form.WriteField("model", model); err != nil {
						t.Fatal(err)
					}
					part, err := form.CreateFormFile("file", "clip.wav")
					if err != nil {
						t.Fatal(err)
					}
					if _, err := io.WriteString(part, "fixture audio"); err != nil {
						t.Fatal(err)
					}
					if err := form.Close(); err != nil {
						t.Fatal(err)
					}
					contentType, body = form.FormDataContentType(), buffer.Bytes()
				}
				r := httptest.NewRequest(http.MethodPost, op.path, bytes.NewReader(body))
				if token != "" {
					r.Header.Set("Authorization", "Bearer "+token)
				}
				r.Header.Set("Content-Type", contentType)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				return w
			}
			names := func(owner string) []string {
				if op.native {
					owner += "-native"
				}
				return []string{owner + "/m", "alias/" + owner}
			}
			for _, owner := range []struct{ name, token string }{{"a", "a-token"}, {"system", "system-token"}} {
				for _, model := range names(owner.name) {
					before := calls.Load()
					if w := call(owner.token, model); w.Code != http.StatusOK || calls.Load() != before+1 {
						t.Fatalf("own %s = %d %s; calls=%d, want %d", model, w.Code, w.Body, calls.Load(), before+1)
					}
				}
			}
			before := calls.Load()
			for _, owner := range []string{"b", "system"} {
				for i, model := range names(owner) {
					for _, token := range []string{"a-token", "a-explicit"} {
						missing := []string{"missing/m", "alias/missing"}[i]
						w, absent := call(token, model), call(token, missing)
						if w.Code != http.StatusNotFound || w.Code != absent.Code || w.Body.String() != absent.Body.String() {
							t.Fatalf("foreign %s = %d %s, absent = %d %s", model, w.Code, w.Body, absent.Code, absent.Body)
						}
					}
				}
			}
			for _, owner := range []string{"a", "b", "system"} {
				for _, model := range names(owner) {
					if w := call("empty-token", model); w.Code != http.StatusNotFound {
						t.Fatalf("empty workspace used %s: %d %s", model, w.Code, w.Body)
					}
				}
			}
			if w := call("a-explicit", names("a")[0]); w.Code != http.StatusForbidden {
				t.Fatalf("allowlist did not narrow owned direct inference: %d %s", w.Code, w.Body)
			}
			for _, token := range []string{"", "browser-session"} {
				if w := call(token, names("a")[0]); w.Code != http.StatusUnauthorized {
					t.Fatalf("request without inference identity = %d %s", w.Code, w.Body)
				}
			}
			if calls.Load() != before {
				t.Fatal("a denied alternate-operation request reached the adapter")
			}
			if w := call("a-explicit", names("a")[1]); w.Code != http.StatusOK || calls.Load() != before+1 {
				t.Fatalf("allowing an owned alias required separate target grants: %d %s", w.Code, w.Body)
			}

			management := deps
			management.Auth, management.Authorizer = auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}), auth.NewAuthorizer(config.Auth{Mode: config.AuthModeNone})
			h.UpdateDependencies(management)
			before = calls.Load()
			for _, token := range []string{"", "a-token", "system-token", "browser-session"} {
				for _, model := range names("a") {
					if w := call(token, model); w.Code != http.StatusUnauthorized {
						t.Fatalf("management-only request = %d %s", w.Code, w.Body)
					}
				}
			}
			if calls.Load() != before {
				t.Fatal("management-only inference reached the adapter")
			}
			management.MultiTenancy.Enabled = false
			h.UpdateDependencies(management)
			for _, model := range names("a") {
				if w := call("", model); w.Code != http.StatusOK {
					t.Fatalf("file-only anonymous behavior changed: %d %s", w.Code, w.Body)
				}
			}
			if calls.Load() != before+2 {
				t.Fatal("file-only direct and alias requests did not both dispatch")
			}
		})
	}
}

func TestHostedDispatchRejectsUnscopedResultsAndForeignSelectorTargets(t *testing.T) {
	a, b := identity.InWorkspace(uuid.New()), identity.InWorkspace(uuid.New())
	rt := &config.Runtime{Catalog: config.NewCatalog([]config.Provider{
		{Name: "a", Workspace: a, Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
		{Name: "b", Workspace: b, Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
	}, nil, []config.Alias{{Name: "a", Workspace: a, Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: "a", Model: "m"}}}})}
	var calls atomic.Int32
	h := NewHandler(Dependencies{Catalog: rt.Catalog, Resolver: modelresolver.New(rt), MultiTenancy: config.MultiTenancy{Enabled: true},
		Adapter: deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
			calls.Add(1)
			return &provider.Result{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
		}), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	deps := h.current()
	dispatch := func(resolved modelresolver.ResolveResult) (*provider.Result, error) {
		if resolved.Kind == modelresolver.KindDirect {
			return h.dispatchDirect(deps, context.Background(), provider.OpChatCompletions, resolved, nil, []byte(`{}`), deps.Logger)
		}
		return h.dispatchAlias(deps, context.Background(), provider.OpChatCompletions, resolved, nil, []byte(`{}`), deps.Logger)
	}
	for _, model := range []string{"a/m", "alias/a"} {
		unscoped, err := deps.Resolver.Resolve(model)
		if err != nil {
			t.Fatal(err)
		}
		if result, err := dispatch(unscoped); err == nil || result != nil || calls.Load() != 0 {
			t.Fatalf("unscoped %s reached dispatch", model)
		}
		scoped, err := deps.Resolver.ResolveInWorkspace(model, a)
		if err != nil {
			t.Fatal(err)
		}
		scoped.Workspace = b
		if result, err := dispatch(scoped); err == nil || result != nil || calls.Load() != 0 {
			t.Fatalf("foreign scope reached %s dispatch", model)
		}
	}
	resolved, err := deps.Resolver.ResolveInWorkspace("alias/a", a)
	if err != nil {
		t.Fatal(err)
	}
	resolved.Selector = alias.NewSelector(config.Alias{Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: "b", Model: "m"}}})
	if result, err := dispatch(resolved); err == nil || result != nil || calls.Load() != 0 {
		t.Fatal("a selector dispatched a target outside the admitted alias pool")
	}
}

func TestHostedModelPolicyAndDeferredDrain(t *testing.T) {
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}))
	client := dynamicDeferredClient()
	other := auth.DynamicClient{TokenHash: store.TokenHash("workspace-b"), Name: client.Name, Tenant: client.Tenant,
		KeyID: uuid.New(), Workspace: identity.InWorkspace(uuid.New())}
	quota := &deferredQuotaStore{}
	configureDynamicDeferred(f, []auth.DynamicClient{client, other}, quota, time.Now)

	local := f.rt.Catalog.Providers()[0]
	foreign := local
	foreign.Name, foreign.Workspace = "foreign", other.Workspace
	unowned := local
	unowned.Name, unowned.Workspace = "unowned", identity.WorkspaceScope{}
	derived := local
	derived.Name, derived.Extends = "derived", foreign.Name
	target := config.AliasTarget{Provider: local.Name, Model: "m"}
	route := config.Alias{Name: "route", Workspace: client.Workspace, Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{target}}
	publish := func(route config.Alias) {
		f.rt.Catalog = config.NewCatalog([]config.Provider{local, foreign, unowned, derived}, nil, []config.Alias{route})
		deps := f.h.current()
		deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.NewWithPrevious(f.rt, deps.Resolver)
		f.h.UpdateDependencies(deps)
	}
	publish(route)
	capability := f.capability(t, "alias/route")
	body := []byte(`{"model":"alias/route","messages":[]}`)
	accepted := deferredSubmitRequest(t, capability, body)
	receipt := f.submit(t, accepted)
	f.finish(t, receipt)

	for _, invalid := range []struct {
		name   string
		target config.AliasTarget
	}{
		{"cross-workspace", config.AliasTarget{Provider: foreign.Name, Model: "m"}},
		{"missing-model", config.AliasTarget{Provider: local.Name, Model: "missing"}},
		{"missing-provider", config.AliasTarget{Provider: "missing", Model: "m"}},
		{"unowned", config.AliasTarget{Provider: unowned.Name, Model: "m"}},
		{"invalid-base", config.AliasTarget{Provider: derived.Name, Model: "m"}},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			bad := route
			bad.Targets = []config.AliasTarget{target, invalid.target}
			publish(bad)

			w := f.call(http.MethodGet, "/v1/models", "peer-token", nil)
			var inventory struct{ Data []ModelCard }
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &inventory) != nil ||
				len(inventory.Data) != 1 || inventory.Data[0].ID != "p/m" {
				t.Fatalf("invalid graph made it into discovery: %d %s", w.Code, w.Body.String())
			}
			for _, stream := range []string{"false", "true"} {
				req := func(model string) *httptest.ResponseRecorder {
					r := f.request(http.MethodPost, "/v1/chat/completions", "peer-token",
						[]byte(`{"model":"`+model+`","stream":`+stream+`,"messages":[]}`))
					r.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					f.h.ServeHTTP(w, r)
					return w
				}
				missing := req("alias/missing")
				got := req("alias/route")
				if got.Code != http.StatusNotFound || got.Body.String() != missing.Body.String() {
					t.Fatalf("invalid alias differs from a missing alias: %d %s vs %d %s", got.Code, got.Body, missing.Code, missing.Body)
				}
			}
			w = f.call(http.MethodGet, deferred.Prefix+"/capabilities?model=alias/route", "peer-token", nil)
			if w.Code != http.StatusNotFound || controlErrorType(t, w) != "model_not_found" {
				t.Fatalf("invalid alias advertised deferred eligibility: %d %s", w.Code, w.Body)
			}
			fresh := deferredSubmitRequest(t, capability, body)
			w = f.call(http.MethodPost, deferred.Prefix+"/submit", "peer-token", submitWire(t, fresh))
			if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"type":"model_not_found"`) || w.Header().Get("Location") != "" {
				t.Fatalf("previously issued capability bypassed admission: %d %s", w.Code, w.Body)
			}
		})
	}

	job := deferred.Prefix + "/jobs/" + receipt.ID
	for _, path := range []string{job, job + "/result", job + "/read?cursor=0"} {
		missing := f.call(http.MethodGet, path, "workspace-b", nil)
		if missing.Code != http.StatusNotFound || controlErrorType(t, missing) != "job_unavailable" || bytes.Contains(missing.Body.Bytes(), []byte(receipt.ID)) {
			t.Fatalf("another workspace saw the old job: %d %s", missing.Code, missing.Body)
		}
		w := f.call(http.MethodGet, path, "peer-token", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("the admitted owner could not drain after the alias changed: %d %s", w.Code, w.Body)
		}
	}
	if w := f.call(http.MethodDelete, job, "workspace-b", nil); w.Code != http.StatusNotFound || controlErrorType(t, w) != "job_unavailable" {
		t.Fatalf("another workspace could delete the job: %d %s", w.Code, w.Body)
	}
	if w := f.call(http.MethodPost, deferred.Prefix+"/submit", "workspace-b", submitWire(t, accepted)); w.Code != http.StatusBadRequest || bytes.Contains(w.Body.Bytes(), []byte(receipt.ID)) {
		t.Fatalf("another workspace could replay the admitted job: %d %s", w.Code, w.Body)
	}
	if replay := f.submit(t, accepted); !replay.Replayed || replay.ID != receipt.ID {
		t.Fatalf("accepted replay did not retain the admitted job: %+v", replay)
	}

	client.AllowedModels = []string{"p/m"}
	configureDynamicDeferred(f, []auth.DynamicClient{client, other}, quota, time.Now)
	if w := f.call(http.MethodDelete, job, "peer-token", nil); w.Code != http.StatusForbidden {
		t.Fatalf("DELETE ignored current public-name grant: %d %s", w.Code, w.Body)
	}
	if w := f.call(http.MethodPost, deferred.Prefix+"/submit", "peer-token", submitWire(t, accepted)); w.Code != http.StatusForbidden {
		t.Fatalf("accepted replay ignored current public-name grant: %d %s", w.Code, w.Body)
	}
	client.AllowedModels = nil
	configureDynamicDeferred(f, []auth.DynamicClient{client, other}, quota, time.Now)
	if w := f.call(http.MethodDelete, job, "peer-token", nil); w.Code != http.StatusNoContent || calls.Load() != 1 {
		t.Fatalf("drain issued extra inference or did not release: DELETE=%d %s, calls=%d", w.Code, w.Body, calls.Load())
	}
}
