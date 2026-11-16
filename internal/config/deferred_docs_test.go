package config

import (
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

func TestDocumentedDeferredConfigurations(t *testing.T) {
	for name, value := range map[string]string{
		"AIPROXY_CLIENT_TOKEN":           "documentation-client-token",
		"AIPROXY_PEER_TOKEN":             "documentation-peer-token",
		"AIPROXY_CALLER_METRICS_TOKEN":   "documentation-caller-metrics-token",
		"AIPROXY_EXECUTOR_METRICS_TOKEN": "documentation-executor-metrics-token",
		"UPSTREAM_API_KEY":               "documentation-upstream-key",
		"SECONDARY_UPSTREAM_API_KEY":     "documentation-secondary-key",
	} {
		t.Setenv(name, value)
	}
	loaded := make(map[string]*Runtime)
	for _, name := range []string{"peer-caller.hcl", "peer-executor.hcl"} {
		t.Run(name, func(t *testing.T) {
			source := documentedDeferredConfig(t, name)
			want, err := Load(source, name)
			if err != nil {
				t.Fatalf("documented configuration does not validate: %v", err)
			}
			if want.RequiresDB() {
				t.Fatal("documented peer configuration requires a database")
			}
			loaded[name] = want
			jsonSource, _, _, err := Convert(source, name, false)
			if err != nil {
				t.Fatalf("documented HCL does not convert to JSON: %v", err)
			}
			hclSource, _, _, err := Convert(jsonSource, "peer.json", false)
			if err != nil {
				t.Fatalf("documented JSON does not convert back to HCL: %v", err)
			}
			for _, candidate := range []struct {
				name   string
				source []byte
			}{
				{name, source},
				{"peer.json", jsonSource},
				{"peer.hcl", hclSource},
			} {
				got, err := Load(candidate.source, candidate.name)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("documented configuration changed after conversion (%s): %v", candidate.name, err)
				}
				t.Setenv(ConfigEnvVar, string(candidate.source))
				got, err = LoadEnv()
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("documented inline configuration changed (%s): %v", candidate.name, err)
				}
			}
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
	if peer.Type != ProviderTypeOpenAICompatible || peer.DeferredTransport == nil ||
		*peer.DeferredTransport != DefaultDeferredTransport() || peer.DeferredReceiver != nil ||
		peer.APIKey != executor.Auth.Clients["peer-caller"].Token {
		t.Fatal("documented caller opt-in, defaults or peer credential do not match executor")
	}
	if caller.Auth.Clients["application"].Token == peer.APIKey ||
		caller.Metrics.Token == peer.APIKey || executor.Metrics.Token == peer.APIKey {
		t.Fatal("documented client, peer and metrics credentials must be separate")
	}
	_, model, ok := caller.Catalog.Model("peer", "chat")
	if !ok || model.UpstreamName != "alias/peer_chat" ||
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
		if !ok || p.DeferredReceiver == nil || p.DeferredTransport != nil || p.ForwardAuthorization ||
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
