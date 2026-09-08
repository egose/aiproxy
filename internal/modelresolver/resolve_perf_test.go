package modelresolver

import (
	"strconv"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

var (
	sinkResult ResolveResult
	sinkErr    error
)

func buildPerfRT(modelsPerProvider int) *config.Runtime {
	models := make([]config.Model, 0, modelsPerProvider)
	for i := 0; i < modelsPerProvider; i++ {
		name := "model-" + strconv.Itoa(i)
		models = append(models, config.Model{
			Name:         name,
			UpstreamName: name,
			Capabilities: []config.Capability{config.CapabilityChat, config.CapabilityResponses},
		})
	}
	return &config.Runtime{
		Catalog: config.NewCatalog([]config.Provider{{
			Name:    "p0",
			BaseURL: "https://example.test/v1",
			APIKey:  "sk-perf",
			Models:  models,
		}}, nil, []config.Alias{{
			Name:      "chat",
			Algorithm: config.AlgorithmRoundRobin,
			Targets:   []config.AliasTarget{{Provider: "p0", Model: "model-0"}},
		}}),
	}
}

func BenchmarkResolveDirectHit(b *testing.B) {
	for _, n := range []int{1, 100, 1000} {
		r := New(buildPerfRT(n))
		target := "p0/model-" + strconv.Itoa(n-1)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				res, err := r.Resolve(target)
				sinkResult = res
				sinkErr = err
			}
		})
	}
}

func BenchmarkResolveUnknownProvider(b *testing.B) {
	r := New(buildPerfRT(1000))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := r.Resolve("missing/model-0")
		sinkResult = res
		sinkErr = err
	}
}

func BenchmarkResolveUnknownModel(b *testing.B) {
	for _, n := range []int{1, 100, 1000} {
		r := New(buildPerfRT(n))
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				res, err := r.Resolve("p0/missing")
				sinkResult = res
				sinkErr = err
			}
		})
	}
}

func BenchmarkResolveAlias(b *testing.B) {
	for _, n := range []int{1, 100, 1000} {
		r := New(buildPerfRT(n))
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				res, err := r.Resolve("alias/chat")
				sinkResult = res
				sinkErr = err
			}
		})
	}
}
