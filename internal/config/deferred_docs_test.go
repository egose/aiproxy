package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func documentedDeferredConfig(t *testing.T, name string) []byte {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "..", "website", "docs", "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	fence := "```hcl title=\"" + name + "\"\n"
	if strings.Count(string(doc), fence) != 1 {
		t.Fatalf("expected one documented %s configuration", name)
	}
	_, rest, _ := strings.Cut(string(doc), fence)
	source, _, ok := strings.Cut(rest, "\n```")
	if !ok {
		t.Fatalf("unterminated documented %s configuration", name)
	}
	return []byte(source + "\n")
}

func loadDocumentedDeferredConfig(t *testing.T, name string, source []byte) *Runtime {
	t.Helper()
	env := map[string]string{
		"AIPROXY_CLIENT_TOKEN":           "documentation-client-token",
		"AIPROXY_PEER_TOKEN":             "documentation-peer-token",
		"AIPROXY_CALLER_METRICS_TOKEN":   "documentation-caller-metrics-token",
		"AIPROXY_EXECUTOR_METRICS_TOKEN": "documentation-executor-metrics-token",
		"UPSTREAM_API_KEY":               "documentation-upstream-key",
		"SECONDARY_UPSTREAM_API_KEY":     "documentation-secondary-key",
		"AIPROXY_DOCS_MAX_WAIT":          "20m",
		"AIPROXY_DOCS_POLL_INTERVAL":     "500ms",
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	want, err := Load(source, name)
	if err != nil {
		t.Fatalf("documented configuration does not validate: %v", err)
	}
	if want.RequiresDB() {
		t.Fatal("documented peer configuration requires a database")
	}
	for _, compact := range []bool{false, true} {
		jsonSource, _, _, err := Convert(source, name, compact)
		if err != nil {
			t.Fatalf("documented HCL does not convert to JSON: %v", err)
		}
		if bytes.Contains(jsonSource, []byte("env(")) {
			t.Fatal("conversion must resolve documented environment expressions")
		}
		hclSource, _, _, err := Convert(jsonSource, "peer.json", false)
		if err != nil {
			t.Fatalf("documented JSON does not convert back to HCL: %v", err)
		}
		jsonEnv := bytes.Clone(jsonSource)
		for key, value := range env {
			jsonEnv = bytes.ReplaceAll(jsonEnv, []byte(`"`+value+`"`), []byte(`env("`+key+`")`))
		}
		for _, candidate := range []struct {
			name   string
			source []byte
		}{
			{name, source},
			{"peer.json", jsonSource},
			{"peer.hcl", hclSource},
			{"peer-env.json", jsonEnv},
		} {
			if err := ValidateDeferredConfig(candidate.source, candidate.name); err != nil {
				t.Fatalf("documented standalone validation failed (%s): %v", candidate.name, err)
			}
			got, err := Load(candidate.source, candidate.name)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("documented configuration changed after conversion (%s): %v", candidate.name, err)
			}
			path := filepath.Join(t.TempDir(), candidate.name)
			if err := os.WriteFile(path, candidate.source, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err = LoadFile(path)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("documented file configuration changed (%s): %v", candidate.name, err)
			}
			t.Setenv(ConfigEnvVar, string(candidate.source))
			got, err = LoadEnv()
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("documented inline/env configuration changed (%s): %v", candidate.name, err)
			}
		}
	}
	return want
}

func TestDocumentedDeferredConfigurations(t *testing.T) {
	loaded := make(map[string]*Runtime)
	for _, name := range []string{"peer-caller.hcl", "peer-executor.hcl"} {
		t.Run(name, func(t *testing.T) {
			loaded[name] = loadDocumentedDeferredConfig(t, name, documentedDeferredConfig(t, name))
		})
	}
	caller, executor := loaded["peer-caller.hcl"], loaded["peer-executor.hcl"]
	if caller == nil || executor == nil {
		t.Fatal("both documented peer roles must validate")
	}
	if caller.Deferred != DefaultDeferred() || caller.Deferred.Enabled {
		t.Fatal("caller-only example must not enable receiver storage")
	}
	wantRoot := DefaultDeferred()
	wantRoot.Enabled = true
	if executor.Deferred != wantRoot {
		t.Fatalf("documented executor policy drifted from defaults: %+v", executor.Deferred)
	}
	peer := testProvider(t, caller, "peer")
	if peer.Type != ProviderTypeOpenAICompatible || peer.DeferredTransport != nil || peer.DeferredReceiver != nil ||
		peer.APIKey != executor.Auth.Clients["peer-caller"].Token {
		t.Fatal("documented caller placement or peer credential does not match executor")
	}
	if caller.Auth.Clients["application"].Token == peer.APIKey ||
		caller.Metrics.Token == peer.APIKey || executor.Metrics.Token == peer.APIKey {
		t.Fatal("documented client, peer and metrics credentials must be separate")
	}
	_, model, ok := caller.Catalog.Model("peer", "chat")
	if !ok || model.UpstreamName != "alias/peer_chat" ||
		model.DeferredTransport == nil || *model.DeferredTransport != DefaultDeferredTransport() ||
		!reflect.DeepEqual(EffectiveDeferredTransport(peer, model), model.DeferredTransport) ||
		!HasCapability(EffectiveCapabilities(peer.Type, model), CapabilityChat) ||
		!HasCapability(EffectiveCapabilities(peer.Type, model), CapabilityResponses) {
		t.Fatal("documented model must address the executor alias without losing ordinary Responses capability")
	}
	alias, ok := executor.Catalog.Alias("peer_chat")
	if !ok || len(alias.Targets) != 2 ||
		!HasCapability(executor.Catalog.AliasEffectiveCapabilities(alias), CapabilityResponses) ||
		!reflect.DeepEqual(executor.Auth.Clients["peer-caller"].AllowedModels, []string{model.UpstreamName}) {
		t.Fatal("documented executor alias capabilities or peer authorization do not match caller")
	}
	for _, target := range alias.Targets {
		p, m, ok := executor.Catalog.Model(target.Provider, target.Model)
		if !ok || p.DeferredReceiver == nil || HasCallerDeferredTransport(p) || p.ForwardAuthorization ||
			!HasCapability(EffectiveCapabilities(p.Type, m), CapabilityChat) {
			t.Fatalf("documented executor alias has an ineligible target: %+v", target)
		}
		if p.APIKey == peer.APIKey {
			t.Fatal("documented upstream and peer credentials must be separate")
		}
	}
	primary, secondary := testProvider(t, executor, "primary"), testProvider(t, executor, "secondary")
	if *primary.DeferredReceiver != (DeferredReceiver{ExecutionTimeout: wantRoot.ExecutionTimeout, MaxResultBytes: wantRoot.MaxResultBytes}) ||
		*secondary.DeferredReceiver != (DeferredReceiver{ExecutionTimeout: 5 * time.Minute, MaxResultBytes: 4 << 20}) ||
		secondary.BaseURL != primary.BaseURL || secondary.APIKey == primary.APIKey {
		t.Fatal("documented local derivative opt-in or alias 5m / 4 MiB limits drifted")
	}
}

func TestDocumentedDeferredMixedModelConfigurations(t *testing.T) {
	caller := loadDocumentedDeferredConfig(t, "peer-model-caller.hcl", documentedDeferredConfig(t, "peer-model-caller.hcl"))
	executor := loadDocumentedDeferredConfig(t, "peer-model-executor.hcl", documentedDeferredConfig(t, "peer-model-executor.hcl"))
	if len(caller.Catalog.Providers()) != 1 || len(executor.Catalog.Providers()) != 1 ||
		caller.Deferred != DefaultDeferred() || caller.Deferred.Enabled {
		t.Fatal("mixed-model pair must use one provider on each role without a caller store")
	}
	wantRoot := DefaultDeferred()
	wantRoot.Enabled = true
	if executor.Deferred != wantRoot {
		t.Fatal("mixed-model example changed root executor policy")
	}
	a, b := testProvider(t, caller, "a"), testProvider(t, executor, "b")
	if a.Type != ProviderTypeOpenAICompatible || a.DeferredTransport != nil || a.DeferredReceiver != nil ||
		!HasCallerDeferredTransport(a) || b.DeferredReceiver == nil || HasCallerDeferredTransport(b) ||
		a.APIKey != executor.Auth.Clients["peer-caller"].Token || a.APIKey == b.APIKey ||
		a.APIKey == caller.Auth.Clients["application"].Token || len(a.Models) != 2 || len(b.Models) != 2 {
		t.Fatal("mixed-model caller placement, provider-only receiver or separate credentials drifted")
	}
	if *b.DeferredReceiver != (DeferredReceiver{ExecutionTimeout: wantRoot.ExecutionTimeout, MaxResultBytes: wantRoot.MaxResultBytes}) {
		t.Fatal("mixed-model receiver must retain root defaults")
	}
	wantModel := DefaultDeferredTransport()
	wantModel.MaxWait, wantModel.PollInterval = 20*time.Minute, 500*time.Millisecond
	for _, name := range []string{"model-1", "model-2"} {
		_, model, ok := caller.Catalog.Model("a", name)
		_, upstream, upstreamOK := executor.Catalog.Model("b", name)
		if !ok || !upstreamOK || model.UpstreamName != "b/"+name || upstream.UpstreamName != "upstream-"+name ||
			upstream.DeferredTransport != nil || EffectiveDeferredTransport(b, upstream) != nil {
			t.Fatalf("mixed-model definition/receiver placement changed for %s", name)
		}
		effective := EffectiveDeferredTransport(a, model)
		if name == "model-1" {
			if model.DeferredTransport != nil || effective != nil {
				t.Fatal("ordinary sibling received caller deferral")
			}
		} else if model.DeferredTransport == nil || effective == nil || *effective != wantModel {
			t.Fatal("deferred sibling lost its partial model block or standard defaults")
		}
		for _, capability := range []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings} {
			if !HasCapability(EffectiveCapabilities(a.Type, model), capability) ||
				!HasCapability(EffectiveCapabilities(b.Type, upstream), capability) {
				t.Fatalf("mixed-model pair changed ordinary %s capability for %s", capability, name)
			}
		}
	}
	if !reflect.DeepEqual(caller.Auth.Clients["application"].AllowedModels, []string{"a/model-1", "a/model-2"}) ||
		!reflect.DeepEqual(executor.Auth.Clients["peer-caller"].AllowedModels, []string{"b/model-1", "b/model-2"}) {
		t.Fatal("mixed-model pair must authorize both ordinary and deferred models")
	}
}

func TestDocumentedDeferredCompatibilityDefaults(t *testing.T) {
	source := append(documentedDeferredConfig(t, "peer-model-caller.hcl"), documentedDeferredConfig(t, "peer-compatibility-default.hcl")...)
	rt := loadDocumentedDeferredConfig(t, "peer-compatibility-default.hcl", source)
	p := testProvider(t, rt, "legacy")
	wantDefault := DeferredTransport{
		ControlTimeout: 20 * time.Second, MaxWait: time.Hour, PollInterval: time.Second,
		MaxControlCalls: 2048, MaxControlRetries: 20,
	}
	if p.DeferredTransport == nil || *p.DeferredTransport != wantDefault {
		t.Fatal("documented provider compatibility default was ignored")
	}
	for i, name := range []string{"model-1", "model-2", "model-3"} {
		_, model, ok := rt.Catalog.Model("legacy", name)
		want := wantDefault
		if i > 0 {
			want = DefaultDeferredTransport()
			if i == 2 {
				want.MaxWait = 20 * time.Minute
			}
		}
		if !ok || (model.DeferredTransport != nil) != (i > 0) {
			t.Fatalf("documented local block presence changed for %s", name)
		}
		if effective := EffectiveDeferredTransport(p, model); effective == nil || *effective != want {
			t.Fatalf("documented empty/partial model policy merged provider values for %s: %+v", name, effective)
		}
	}
}

func TestDocumentedDeferredStaticDerivativeOverlays(t *testing.T) {
	source := append(documentedDeferredConfig(t, "peer-model-caller.hcl"), documentedDeferredConfig(t, "peer-model-derived.hcl")...)
	rt := loadDocumentedDeferredConfig(t, "peer-model-derived.hcl", source)
	base := testProvider(t, rt, "a")
	for _, name := range []string{"derived", "ordinary-derived"} {
		p := testProvider(t, rt, name)
		if p.BaseURL != base.BaseURL || p.APIKey != base.APIKey || p.DeferredTransport != nil ||
			p.DeferredReceiver != nil || len(p.Models) != len(base.Models) {
			t.Fatalf("documented derivative lost inherited definitions or local credentials: %s", name)
		}
		for _, model := range p.Models {
			inherited := base.ModelByName[model.Name]
			inherited.DeferredTransport = nil
			definition := model
			definition.DeferredTransport = nil
			if !reflect.DeepEqual(definition, inherited) {
				t.Fatal("documented transport-only overlay redefined an inherited model")
			}
			effective := EffectiveDeferredTransport(p, model)
			if name == "derived" && model.Name == "model-2" {
				if effective == nil || *effective != DefaultDeferredTransport() ||
					!reflect.DeepEqual(model.DeferredTransport, p.ModelByName[model.Name].DeferredTransport) {
					t.Fatal("documented empty derivative overlay did not reset the full policy")
				}
			} else if model.DeferredTransport != nil || effective != nil {
				t.Fatal("documented derivative inherited a base caller opt-in")
			}
		}
	}
}
