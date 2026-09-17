package config

import (
	"strings"
	"testing"
)

func TestModelPricingCostSplitsInputOutputCached(t *testing.T) {
	p := &ModelPricing{InputPerMillion: 3, OutputPerMillion: 15, CachedPerMillion: 0.3, CacheWritePerMillion: 3.75}
	cost, ok := p.Cost(1000000, 1000000, 500000, 100000, 400000)
	if !ok {
		t.Fatal("expected priced")
	}
	want := 0.5*3 + 1.0*15 + 0.1*3.75 + 0.4*0.3
	if cost < want-1e-9 || cost > want+1e-9 {
		t.Fatalf("cost = %v, want %v", cost, want)
	}
}

func TestModelPricingCostFallsBackToInputRate(t *testing.T) {
	p := &ModelPricing{InputPerMillion: 3, OutputPerMillion: 15}
	cost, ok := p.Cost(1000000, 0, 500000, 0, 500000)
	if !ok {
		t.Fatal("expected priced")
	}
	want := 0.5*3 + 0.5*3
	if cost < want-1e-9 || cost > want+1e-9 {
		t.Fatalf("cost = %v, want %v", cost, want)
	}
}

func TestModelPricingCostUnpriced(t *testing.T) {
	if _, ok := (*ModelPricing)(nil).Cost(1, 1, 0, 0, 0); ok {
		t.Fatal("nil pricing must not produce cost")
	}
	if _, ok := (&ModelPricing{}).Cost(1, 1, 0, 0, 0); ok {
		t.Fatal("empty pricing must not produce cost")
	}
}

func TestLoadModelPricingBlock(t *testing.T) {
	cfg, err := Load([]byte(`
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "anthropic" "anthropic" {
  api_key = "k"
  model "claude" {
    pricing {
      input_per_million = 3.00
      output_per_million = 15.00
      cached_per_million = 0.30
      cache_write_per_million = 3.75
    }
  }
}
`), "config.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	providers := cfg.Catalog.Providers()
	if len(providers) != 1 || len(providers[0].Models) != 1 {
		t.Fatalf("providers = %+v", providers)
	}
	pricing := providers[0].Models[0].Pricing
	if pricing == nil || pricing.InputPerMillion != 3 || pricing.OutputPerMillion != 15 || pricing.CachedPerMillion != 0.3 || pricing.CacheWritePerMillion != 3.75 {
		t.Fatalf("pricing = %+v", pricing)
	}
}

func TestLoadModelPricingRejectsNegative(t *testing.T) {
	_, err := Load([]byte(`
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  model "m" {
    pricing { input_per_million = -1 }
  }
}
`), "config.hcl")
	if err == nil || !strings.Contains(err.Error(), "must not be negative") {
		t.Fatalf("expected negative-rate error, got %v", err)
	}
}

func TestLoadModelPricingRejectsEmptyBlock(t *testing.T) {
	_, err := Load([]byte(`
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  model "m" {
    pricing {
    }
  }
}
`), "config.hcl")
	if err == nil || !strings.Contains(err.Error(), "at least one rate") {
		t.Fatalf("expected empty-block error, got %v", err)
	}
}
