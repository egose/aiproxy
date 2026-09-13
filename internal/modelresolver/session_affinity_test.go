package modelresolver

import (
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func affinityTestRuntime(affinity *config.SessionAffinity) *config.Runtime {
	providers := []config.Provider{
		{Type: config.ProviderTypeOpenAI, Name: "p1", APIKey: "k", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
		{Type: config.ProviderTypeOpenAI, Name: "p2", APIKey: "k", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
	}
	aliases := []config.Alias{{
		Name:             "a",
		Algorithm:        config.AlgorithmRoundRobin,
		RetryStatusCodes: []int{500},
		SessionAffinity:  affinity,
		Targets:          []config.AliasTarget{{Provider: "p1", Model: "m"}, {Provider: "p2", Model: "m"}},
	}}
	return &config.Runtime{Catalog: config.NewCatalog(providers, nil, aliases)}
}

func TestAliasSelectorPreservedAcrossIdenticalAffinityReload(t *testing.T) {
	res := New(affinityTestRuntime(&config.SessionAffinity{}))
	first, err := res.Resolve("alias/a")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if first.Selector == nil {
		t.Fatal("selector is nil")
	}
	got, release := first.Selector.Acquire(nil)
	release()
	if got.Provider != "p1" {
		t.Fatalf("first pick = %+v, want p1", got)
	}
	reloaded := NewWithPrevious(affinityTestRuntime(&config.SessionAffinity{}), res)
	second, err := reloaded.Resolve("alias/a")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	next, releaseNext := second.Selector.Acquire(nil)
	defer releaseNext()
	if next.Provider != "p2" {
		t.Fatalf("pick after identical reload = %+v, want p2 (shared cursor)", next)
	}
}

func TestAliasSelectorResetWhenAffinityChanges(t *testing.T) {
	res := New(affinityTestRuntime(nil))
	first, err := res.Resolve("alias/a")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got, release := first.Selector.Acquire(nil)
	release()
	if got.Provider != "p1" {
		t.Fatalf("first pick = %+v, want p1", got)
	}
	reloaded := NewWithPrevious(affinityTestRuntime(&config.SessionAffinity{}), res)
	second, err := reloaded.Resolve("alias/a")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	next, releaseNext := second.Selector.Acquire(nil)
	defer releaseNext()
	if next.Provider != "p1" {
		t.Fatalf("pick after affinity change = %+v, want p1 (fresh selector)", next)
	}
}
