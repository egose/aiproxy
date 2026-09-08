package config

import (
	"strconv"
	"testing"
)

var (
	sinkProvider Provider
	sinkModel    Model
	sinkAlias    Alias
	sinkOK       bool
)

func buildPerfCatalog(modelsPerProvider int) Catalog {
	models := make([]Model, 0, modelsPerProvider)
	for i := 0; i < modelsPerProvider; i++ {
		name := "model-" + strconv.Itoa(i)
		models = append(models, Model{
			Name:         name,
			UpstreamName: name,
			Capabilities: []Capability{CapabilityChat, CapabilityResponses},
		})
	}
	return NewCatalog([]Provider{{
		Name:    "p0",
		Type:    ProviderTypeOpenAI,
		BaseURL: "https://example.test/v1",
		APIKey:  "sk-perf",
		Models:  models,
	}}, nil, []Alias{{
		Name:             "chat",
		Algorithm:        AlgorithmRoundRobin,
		RetryStatusCodes: []int{500},
		Targets:          []AliasTarget{{Provider: "p0", Model: "model-0"}},
	}})
}

func benchmarkSizes() []int { return []int{1, 100, 1000} }

func BenchmarkCatalogProviderDirectHit(b *testing.B) {
	for _, n := range benchmarkSizes() {
		c := buildPerfCatalog(n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p, ok := c.Provider("p0")
				sinkProvider = p
				sinkOK = ok
			}
		})
	}
}

func BenchmarkCatalogProviderUnknown(b *testing.B) {
	c := buildPerfCatalog(1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := c.Provider("missing")
		sinkOK = ok
	}
}

func BenchmarkCatalogModelDirectHit(b *testing.B) {
	for _, n := range benchmarkSizes() {
		c := buildPerfCatalog(n)
		target := "model-" + strconv.Itoa(n-1)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p, m, ok := c.Model("p0", target)
				sinkProvider = p
				sinkModel = m
				sinkOK = ok
			}
		})
	}
}

func BenchmarkCatalogModelUnknownModel(b *testing.B) {
	for _, n := range benchmarkSizes() {
		c := buildPerfCatalog(n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p, m, ok := c.Model("p0", "missing")
				sinkProvider = p
				sinkModel = m
				sinkOK = ok
			}
		})
	}
}

func BenchmarkCatalogModelUnknownProvider(b *testing.B) {
	c := buildPerfCatalog(1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, m, ok := c.Model("missing", "model-0")
		sinkProvider = p
		sinkModel = m
		sinkOK = ok
	}
}

func BenchmarkCatalogAliasTargetLookup(b *testing.B) {
	for _, n := range benchmarkSizes() {
		c := buildPerfCatalog(n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				a, ok := c.Alias("chat")
				if !ok {
					b.Fatal("alias chat missing")
				}
				p, m, ok := c.Model(a.Targets[0].Provider, a.Targets[0].Model)
				sinkAlias = a
				sinkProvider = p
				sinkModel = m
				sinkOK = ok
			}
		})
	}
}

func prototypeModelNarrow(c *Catalog, providerName, modelName string) (Provider, Model, bool) {
	i, ok := c.providerIndex[providerName]
	if !ok {
		return Provider{}, Model{}, false
	}
	stored := c.providers[i]
	m, ok := stored.ModelByName[modelName]
	if !ok {
		return Provider{}, Model{}, false
	}
	out := stored
	out.Models = nil
	out.ModelByName = map[string]Model{modelName: cloneModel(m)}
	return out, cloneModel(m), true
}

func BenchmarkCatalogModelPrototypeNarrow(b *testing.B) {
	for _, n := range benchmarkSizes() {
		c := buildPerfCatalog(n)
		target := "model-" + strconv.Itoa(n-1)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p, m, ok := prototypeModelNarrow(&c, "p0", target)
				sinkProvider = p
				sinkModel = m
				sinkOK = ok
			}
		})
	}
}
