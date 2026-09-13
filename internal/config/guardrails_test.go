package config

import (
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/guardrails"
)

func guardrailTestBase() string {
	return `
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`
}

func TestLoadIngressGuardrailsAbsent(t *testing.T) {
	rt, err := Load([]byte(guardrailTestBase()), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.IngressGuardrails.Enabled {
		t.Fatalf("guardrails should be disabled when the block is absent")
	}
}

func TestLoadIngressGuardrailsDefaults(t *testing.T) {
	rt, err := Load([]byte(guardrailTestBase()+`
ingress_guardrails {
  enabled = true
}
`), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g := rt.IngressGuardrails
	if !g.Enabled {
		t.Fatalf("guardrails should be enabled")
	}
	if g.Mode != GuardrailModeBlock {
		t.Fatalf("mode = %q, want block", g.Mode)
	}
	if g.MaxTextBytes != 0 || g.MaxStrings != 0 {
		t.Fatalf("unset bounds should stay zero for scanner defaults, got %+v", g)
	}
}

func TestLoadIngressGuardrailsExplicit(t *testing.T) {
	rt, err := Load([]byte(guardrailTestBase()+`
ingress_guardrails {
  enabled = true
  mode = "audit"
  max_text_bytes = 4096
  max_strings = 64
}
`), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g := rt.IngressGuardrails
	if !g.Enabled || g.Mode != GuardrailModeAudit || g.MaxTextBytes != 4096 || g.MaxStrings != 64 {
		t.Fatalf("guardrails = %+v", g)
	}
}

func TestLoadIngressGuardrailsJSON(t *testing.T) {
	cfg := `{
  "listener": {"http": {"public": {"address": ":8080"}}},
  "auth": {"main": {"mode": "none"}},
  "provider": {"openai": {"openai": {"api_key": "sk-test", "model": {"gpt-4o-mini": {}}}}},
  "ingress_guardrails": {"enabled": true, "mode": "audit", "max_text_bytes": 2048, "max_strings": 8}
}`
	rt, err := Load([]byte(cfg), "test.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g := rt.IngressGuardrails
	if !g.Enabled || g.Mode != GuardrailModeAudit || g.MaxTextBytes != 2048 || g.MaxStrings != 8 {
		t.Fatalf("guardrails = %+v", g)
	}
}

func TestLoadIngressGuardrailsRejectsInvalid(t *testing.T) {
	bases := []string{
		`ingress_guardrails { enabled = true
  mode = "watch"
}`,
		`ingress_guardrails { enabled = true
  max_text_bytes = 16
}`,
		`ingress_guardrails { enabled = true
  max_text_bytes = 16777216
}`,
		`ingress_guardrails { enabled = true
  max_strings = 8192
}`,
		`ingress_guardrails { enabled = true
}
ingress_guardrails { enabled = false
}`,
	}
	for i, block := range bases {
		if _, err := Load([]byte(guardrailTestBase()+block), "test.hcl"); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestConvertIngressGuardrailsRoundTrip(t *testing.T) {
	src := guardrailTestBase() + `
ingress_guardrails {
  enabled = true
  mode = "audit"
  max_text_bytes = 4096
  max_strings = 64
}
`
	out, from, to, err := Convert([]byte(src), "test.hcl", false)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if from != FormatHCL || to != FormatJSON {
		t.Fatalf("formats = %q -> %q", from, to)
	}
	if !strings.Contains(string(out), "ingress_guardrails") {
		t.Fatalf("converted output drops the block: %s", out)
	}
	back, fromJSON, toHCL, err := Convert(out, "test.json", false)
	if err != nil {
		t.Fatalf("convert back: %v", err)
	}
	if fromJSON != FormatJSON || toHCL != FormatHCL {
		t.Fatalf("formats = %q -> %q", fromJSON, toHCL)
	}
	rt, err := Load(back, "test.hcl")
	if err != nil {
		t.Fatalf("load converted: %v", err)
	}
	g := rt.IngressGuardrails
	if !g.Enabled || g.Mode != GuardrailModeAudit || g.MaxTextBytes != 4096 || g.MaxStrings != 64 {
		t.Fatalf("round-tripped guardrails = %+v", g)
	}
}

func TestGuardrailPolicyMappingDefaults(t *testing.T) {
	if guardrails.DefaultMode != guardrails.ModeBlock {
		t.Fatalf("default mode changed")
	}
}
