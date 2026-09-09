package modelresolver

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

func cooldownTestRT() *config.Runtime {
	return &config.Runtime{
		Catalog: config.NewCatalog([]config.Provider{
			{
				Type:    config.ProviderTypeOpenAI,
				Name:    "p1",
				BaseURL: "https://one.example",
				APIKey:  "key1",
				Models: []config.Model{
					{Name: "m", UpstreamName: "up-m", Protocol: config.ModelProtocolChat},
				},
			},
			{
				Type:    config.ProviderTypeOpenAI,
				Name:    "p2",
				BaseURL: "https://two.example",
				APIKey:  "key2",
				Models: []config.Model{
					{Name: "m", UpstreamName: "up-m", Protocol: config.ModelProtocolChat},
				},
			},
		}, nil, []config.Alias{
			{
				Name:             "a",
				Algorithm:        config.AlgorithmRoundRobin,
				RetryStatusCodes: []int{500},
				Targets:          []config.AliasTarget{{Provider: "p1", Model: "m"}, {Provider: "p2", Model: "m"}},
			},
			{
				Name:             "b",
				Algorithm:        config.AlgorithmLeastConnections,
				RetryStatusCodes: []int{500},
				Targets:          []config.AliasTarget{{Provider: "p1", Model: "m"}},
			},
		}),
	}
}

func TestCooldownStoreActiveListsRemaining(t *testing.T) {
	rt := cooldownTestRT()
	store := NewCooldownStore()
	current := time.Now()
	store.SetNowFunc(func() time.Time { return current })
	fp := fingerprintFor(t, rt, "a", "p1", "m")
	store.Observe(fp, 30*time.Second)
	active := store.Active()
	if len(active) != 1 || active[0].Alias != "a" || active[0].Provider != "p1" || active[0].Model != "m" {
		t.Fatalf("Active = %+v", active)
	}
	if active[0].Remaining < 29*time.Second || active[0].Remaining > 30*time.Second {
		t.Fatalf("Remaining = %v", active[0].Remaining)
	}
	current = current.Add(time.Minute)
	if got := store.Active(); len(got) != 0 {
		t.Fatalf("expired Active = %+v", got)
	}
}

func fingerprintFor(t *testing.T, rt *config.Runtime, alias, provider, model string) CooldownFingerprint {
	t.Helper()
	prov, m, ok := rt.Catalog.Model(provider, model)
	if !ok {
		t.Fatalf("model %s/%s not found", provider, model)
	}
	return CooldownFingerprintFor(alias, prov, m)
}

func TestCooldownFingerprintIdentity(t *testing.T) {
	rt := cooldownTestRT()
	base := fingerprintFor(t, rt, "a", "p1", "m")
	if base.BaseURL != "https://one.example" || base.Credential != "key:key1" || base.UpstreamModel != "up-m" || base.Protocol != "chat" {
		t.Fatalf("fingerprint = %+v", base)
	}
	otherAlias := fingerprintFor(t, rt, "b", "p1", "m")
	if base == otherAlias {
		t.Fatal("alias-local identity violated")
	}
	otherTarget := fingerprintFor(t, rt, "a", "p2", "m")
	if base == otherTarget {
		t.Fatal("target identity violated")
	}
	changed := rt.Catalog.Providers()
	changed[0].APIKey = "rotated"
	rtChanged := &config.Runtime{Catalog: config.NewCatalog(changed, nil, rt.Catalog.Aliases())}
	rotated := fingerprintFor(t, rtChanged, "a", "p1", "m")
	if base == rotated {
		t.Fatal("credential change must alter fingerprint")
	}
	moved := rt.Catalog.Providers()
	moved[0].BaseURL = "https://moved.example"
	rtMoved := &config.Runtime{Catalog: config.NewCatalog(moved, nil, rt.Catalog.Aliases())}
	if base == fingerprintFor(t, rtMoved, "a", "p1", "m") {
		t.Fatal("endpoint change must alter fingerprint")
	}
}

func TestCooldownStoreMaxWinsAndExpiry(t *testing.T) {
	now := time.Now()
	current := now
	store := NewCooldownStore()
	store.SetNowFunc(func() time.Time { return current })
	fp := CooldownFingerprint{Alias: "a", Provider: "p1", Model: "m"}
	store.Observe(fp, 30*time.Second)
	store.Observe(fp, 10*time.Second)
	remaining, ok := store.Remaining(fp)
	if !ok || remaining != 30*time.Second {
		t.Fatalf("remaining = %v, %v; want 30s, true", remaining, ok)
	}
	store.Observe(fp, 60*time.Second)
	remaining, ok = store.Remaining(fp)
	if !ok || remaining != 60*time.Second {
		t.Fatalf("remaining = %v, %v; want 60s, true", remaining, ok)
	}
	current = now.Add(61 * time.Second)
	if _, ok := store.Remaining(fp); ok {
		t.Fatal("expired deadline still cooling")
	}
	if len(store.deadlines) != 0 {
		t.Fatal("expired entry not cleaned up")
	}
	store.Observe(fp, 0)
	store.Observe(fp, -time.Second)
	if _, ok := store.Remaining(fp); ok {
		t.Fatal("non-positive delay created a deadline")
	}
}

func TestCooldownStoreIsolation(t *testing.T) {
	store := NewCooldownStore()
	a := CooldownFingerprint{Alias: "a", Provider: "p1", Model: "m"}
	b := CooldownFingerprint{Alias: "b", Provider: "p1", Model: "m"}
	store.Observe(a, time.Minute)
	if _, ok := store.Remaining(b); ok {
		t.Fatal("cooldown leaked across aliases")
	}
}

func TestCooldownReloadRetainsUnchanged(t *testing.T) {
	rt := cooldownTestRT()
	now := time.Now()
	current := now
	prev := New(rt)
	prev.Cooldowns().SetNowFunc(func() time.Time { return current })
	fp1 := fingerprintFor(t, rt, "a", "p1", "m")
	fp2 := fingerprintFor(t, rt, "a", "p2", "m")
	prev.Cooldowns().Observe(fp1, 30*time.Second)
	prev.Cooldowns().Observe(fp2, 30*time.Second)
	next := NewWithPrevious(rt, prev)
	for _, fp := range []CooldownFingerprint{fp1, fp2} {
		if _, ok := next.Cooldowns().Remaining(fp); !ok {
			t.Fatalf("unchanged fingerprint %+v lost across reload", fp)
		}
	}
	current = now.Add(time.Second)
	next.Cooldowns().Observe(fp1, 5*time.Second)
	if _, ok := prev.Cooldowns().Remaining(fp1); !ok {
		t.Fatal("old in-flight store must stay independent")
	}
	remaining, _ := next.Cooldowns().Remaining(fp1)
	if remaining > 30*time.Second || remaining < 29*time.Second {
		t.Fatalf("new store polluted by old write: %v", remaining)
	}
}

func TestCooldownReloadDropsChanged(t *testing.T) {
	rt := cooldownTestRT()
	prev := New(rt)
	fp1 := fingerprintFor(t, rt, "a", "p1", "m")
	fp2 := fingerprintFor(t, rt, "a", "p2", "m")
	prev.Cooldowns().Observe(fp1, time.Minute)
	prev.Cooldowns().Observe(fp2, time.Minute)
	providers := rt.Catalog.Providers()
	for i := range providers {
		if providers[i].Name == "p1" {
			providers[i].APIKey = "rotated"
		}
	}
	changed := &config.Runtime{Catalog: config.NewCatalog(providers, nil, rt.Catalog.Aliases())}
	next := NewWithPrevious(changed, prev)
	if _, ok := next.Cooldowns().Remaining(fingerprintFor(t, changed, "a", "p1", "m")); ok {
		t.Fatal("rotated credential retained stale deadline")
	}
	if _, ok := next.Cooldowns().Remaining(fp2); !ok {
		t.Fatal("unchanged target lost deadline on unrelated rotation")
	}
}

func TestCooldownReloadDropsRemovedAndKeepsAlgorithmChanges(t *testing.T) {
	rt := cooldownTestRT()
	prev := New(rt)
	fp1 := fingerprintFor(t, rt, "a", "p1", "m")
	prev.Cooldowns().Observe(fp1, time.Minute)
	aliases := rt.Catalog.Aliases()
	aliases[0].Targets = []config.AliasTarget{{Provider: "p2", Model: "m"}}
	aliases[0].Algorithm = config.AlgorithmLeastConnections
	aliases[0].RetryStatusCodes = []int{503}
	trimmed := &config.Runtime{Catalog: config.NewCatalog(rt.Catalog.Providers(), nil, aliases)}
	next := NewWithPrevious(trimmed, prev)
	if _, ok := next.Cooldowns().Remaining(fp1); ok {
		t.Fatal("removed target retained deadline")
	}
	prev.Cooldowns().Observe(fingerprintFor(t, rt, "a", "p2", "m"), time.Minute)
	reshaped := NewWithPrevious(trimmed, prev)
	if _, ok := reshaped.Cooldowns().Remaining(fingerprintFor(t, trimmed, "a", "p2", "m")); !ok {
		t.Fatal("algorithm/retry-code change must not invalidate deadlines")
	}
}

func TestCooldownReloadDropsExpired(t *testing.T) {
	rt := cooldownTestRT()
	now := time.Now()
	current := now
	prev := New(rt)
	prev.Cooldowns().SetNowFunc(func() time.Time { return current })
	fp1 := fingerprintFor(t, rt, "a", "p1", "m")
	prev.Cooldowns().Observe(fp1, 10*time.Second)
	current = now.Add(time.Minute)
	next := NewWithPrevious(rt, prev)
	if _, ok := next.Cooldowns().Remaining(fp1); ok {
		t.Fatal("expired deadline survived reload")
	}
}

func TestCooldownReloadCarriesClock(t *testing.T) {
	rt := cooldownTestRT()
	now := time.Now()
	current := now
	prev := New(rt)
	prev.Cooldowns().SetNowFunc(func() time.Time { return current })
	next := NewWithPrevious(rt, prev)
	fp1 := fingerprintFor(t, rt, "a", "p1", "m")
	next.Cooldowns().Observe(fp1, 10*time.Second)
	current = now.Add(4 * time.Second)
	remaining, ok := next.Cooldowns().Remaining(fp1)
	if !ok || remaining != 6*time.Second {
		t.Fatalf("remaining = %v, %v; want 6s, true", remaining, ok)
	}
}

func TestCooldownStoreConcurrent(t *testing.T) {
	rt := cooldownTestRT()
	resolver := New(rt)
	fps := []CooldownFingerprint{
		fingerprintFor(t, rt, "a", "p1", "m"),
		fingerprintFor(t, rt, "a", "p2", "m"),
		fingerprintFor(t, rt, "b", "p1", "m"),
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				fp := fps[(i+j)%len(fps)]
				resolver.Cooldowns().Observe(fp, time.Duration(i+j+1)*time.Second)
				_, _ = resolver.Cooldowns().Remaining(fp)
				_ = NewWithPrevious(rt, resolver)
			}
		}(i)
	}
	wg.Wait()
	for _, fp := range fps {
		if _, ok := resolver.Cooldowns().Remaining(fp); !ok {
			t.Fatalf("fingerprint %+v lost under concurrency", fp)
		}
	}
}

func TestCooldownNilStoreSafe(t *testing.T) {
	var store *CooldownStore
	store.Observe(CooldownFingerprint{}, time.Second)
	if _, ok := store.Remaining(CooldownFingerprint{}); ok {
		t.Fatal("nil store reports cooling")
	}
	var resolver *Resolver
	if resolver.Cooldowns() == nil {
		t.Fatal("nil resolver must yield usable store")
	}
	if fmt.Sprint(resolver.Cooldowns() == nil) != "false" {
		t.Fatal("nil resolver store unusable")
	}
}
