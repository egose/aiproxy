package modelresolver

import (
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func buildRT() *config.Runtime {
	return &config.Runtime{
		Catalog: config.NewCatalog([]config.Provider{{
			Name:    "openai",
			BaseURL: "https://api.openai.com/v1",
			APIKey:  "sk",
			Models:  []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}},
		}}, nil, []config.Alias{{
			Name:      "chat_default",
			Algorithm: config.AlgorithmRoundRobin,
			Targets: []config.AliasTarget{
				{Provider: "openai", Model: "gpt-4o-mini"},
			},
		}}),
	}
}

func TestResolveDirectProviderModel(t *testing.T) {
	r := New(buildRT())
	res, err := r.Resolve("openai/gpt-4o-mini")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Kind != KindDirect {
		t.Errorf("kind = %v, want direct", res.Kind)
	}
	if res.Provider.Name != "openai" {
		t.Errorf("provider = %q", res.Provider.Name)
	}
	if res.Model.Name != "gpt-4o-mini" {
		t.Errorf("model = %q", res.Model.Name)
	}
}

func TestResolveDirectProviderModelWithSlashInModelName(t *testing.T) {
	rt := buildRT()
	providers := rt.Catalog.Providers()
	providers[0].Models = append(providers[0].Models, config.Model{Name: "vendor/model-a", UpstreamName: "vendor/model-a"})
	rt.Catalog = config.NewCatalog(providers, nil, rt.Catalog.Aliases())
	r := New(rt)
	res, err := r.Resolve("openai/vendor/model-a")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Kind != KindDirect || res.Model.Name != "vendor/model-a" {
		t.Fatalf("result = %+v", res)
	}
}

func TestResolveAlias(t *testing.T) {
	r := New(buildRT())
	res, err := r.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Kind != KindAlias {
		t.Errorf("kind = %v, want alias", res.Kind)
	}
	if res.Alias.Name != "chat_default" {
		t.Errorf("alias = %q", res.Alias.Name)
	}
	if res.Selector == nil {
		t.Errorf("selector is nil")
	}
}

func TestNewWithPreviousPreservesUnchangedRoundRobinState(t *testing.T) {
	r := New(buildAliasRT(config.AlgorithmRoundRobin, "a", "b"))
	res, err := r.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve initial alias: %v", err)
	}
	first, release := res.Selector.Acquire(nil)
	if first.Provider != "a" {
		t.Fatalf("first target = %+v, want provider a", first)
	}
	release()

	reloaded := NewWithPrevious(buildAliasRT(config.AlgorithmRoundRobin, "a", "b"), r)
	res, err = reloaded.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve reloaded alias: %v", err)
	}
	second, release := res.Selector.Acquire(nil)
	defer release()
	if second.Provider != "b" {
		t.Fatalf("second target after unchanged reload = %+v, want provider b", second)
	}
}

func TestNewWithPreviousPreservesUnchangedLeastConnectionsLease(t *testing.T) {
	r := New(buildAliasRT(config.AlgorithmLeastConnections, "a", "b"))
	res, err := r.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve initial alias: %v", err)
	}
	first, releaseFirst := res.Selector.Acquire(nil)
	if first.Provider != "a" {
		t.Fatalf("first target = %+v, want provider a", first)
	}

	reloaded := NewWithPrevious(buildAliasRT(config.AlgorithmLeastConnections, "a", "b"), r)
	res, err = reloaded.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve reloaded alias: %v", err)
	}
	second, releaseSecond := res.Selector.Acquire(nil)
	if second.Provider != "b" {
		t.Fatalf("target while pre-reload lease is held = %+v, want provider b", second)
	}
	releaseFirst()
	third, releaseThird := res.Selector.Acquire(nil)
	defer releaseThird()
	if third.Provider != "a" {
		t.Fatalf("target after pre-reload lease release = %+v, want provider a", third)
	}
	releaseSecond()
}

func TestNewWithPreviousCreatesNewStateForChangedAlias(t *testing.T) {
	r := New(buildAliasRT(config.AlgorithmLeastConnections, "a", "b"))
	res, err := r.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve initial alias: %v", err)
	}
	first, releaseFirst := res.Selector.Acquire(nil)
	if first.Provider != "a" {
		t.Fatalf("first target = %+v, want provider a", first)
	}

	reloaded := NewWithPrevious(buildAliasRT(config.AlgorithmLeastConnections, "a", "b", "c"), r)
	res, err = reloaded.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve reloaded alias: %v", err)
	}
	got, release := res.Selector.Acquire(nil)
	defer release()
	if got.Provider != "a" {
		t.Fatalf("changed alias reused old lease state: got %+v, want provider a", got)
	}
	releaseFirst()

	changedAlgorithm := NewWithPrevious(buildAliasRT(config.AlgorithmRoundRobin, "a", "b"), r)
	res, err = changedAlgorithm.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve algorithm-changed alias: %v", err)
	}
	got, release = res.Selector.Acquire(nil)
	defer release()
	if got.Provider != "a" {
		t.Fatalf("algorithm-changed alias reused old state: got %+v, want provider a", got)
	}
}

func TestNewWithPreviousDoesNotInvalidateRemovedAliasLeases(t *testing.T) {
	r := New(buildAliasRT(config.AlgorithmLeastConnections, "a", "b"))
	res, err := r.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve initial alias: %v", err)
	}
	_, release := res.Selector.Acquire(nil)

	reloaded := NewWithPrevious(buildAliasRT(config.AlgorithmLeastConnections, "b"), r)
	res, err = reloaded.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve reloaded alias: %v", err)
	}
	got, releaseReloaded := res.Selector.Acquire(nil)
	defer releaseReloaded()
	if got.Provider != "b" {
		t.Fatalf("target after removing leased target = %+v, want provider b", got)
	}
	release()
	release()
}

func TestResolveUnknownProvider(t *testing.T) {
	r := New(buildRT())
	if _, err := r.Resolve("azure/foo"); err == nil {
		t.Errorf("expected error for unknown provider")
	}
}

func TestResolveUnknownModel(t *testing.T) {
	r := New(buildRT())
	if _, err := r.Resolve("openai/unknown-model"); err == nil {
		t.Errorf("expected error for unknown model")
	}
}

func TestResolveUnknownAlias(t *testing.T) {
	r := New(buildRT())
	if _, err := r.Resolve("alias/missing"); err == nil {
		t.Errorf("expected error for unknown alias")
	}
}

func TestResolveEmptyModel(t *testing.T) {
	r := New(buildRT())
	if _, err := r.Resolve(""); err == nil {
		t.Errorf("expected error for empty model")
	}
}

func TestResolveUnqualifiedModelFails(t *testing.T) {
	r := New(buildRT())
	if _, err := r.Resolve("gpt-4o-mini"); err == nil {
		t.Errorf("expected error for unqualified model")
	}
}

func buildAliasRT(algorithm config.Algorithm, providers ...string) *config.Runtime {
	configuredProviders := make([]config.Provider, 0, len(providers))
	targets := make([]config.AliasTarget, 0, len(providers))
	for _, name := range providers {
		configuredProviders = append(configuredProviders, config.Provider{
			Name: name,
			Models: []config.Model{
				{Name: "m", UpstreamName: "m"},
			},
		})
		targets = append(targets, config.AliasTarget{Provider: name, Model: "m"})
	}
	a := config.Alias{Name: "chat_default", Algorithm: algorithm, Targets: targets}
	return &config.Runtime{Catalog: config.NewCatalog(configuredProviders, nil, []config.Alias{a})}
}
