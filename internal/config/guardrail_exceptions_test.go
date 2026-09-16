package config

import (
	"strings"
	"testing"
)

func TestLoadIngressGuardrailsExceptions(t *testing.T) {
	rt, err := Load([]byte(guardrailTestBase()+`
ingress_guardrails {
  enabled = true
  exceptions_file = "/tmp/guardrail-exceptions.json"
  redact_placeholder = "NONSECRET"
}
`), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g := rt.IngressGuardrails
	if g.ExceptionsFile != "/tmp/guardrail-exceptions.json" {
		t.Fatalf("exceptions_file = %q", g.ExceptionsFile)
	}
	if g.RedactPlaceholder != "NONSECRET" {
		t.Fatalf("redact_placeholder = %q", g.RedactPlaceholder)
	}
	if got := ResolveGuardrailExceptionsPath(g); got != "/tmp/guardrail-exceptions.json" {
		t.Fatalf("resolved path = %q", got)
	}
	if got := ResolveGuardrailPlaceholder(g); got != "NONSECRET" {
		t.Fatalf("resolved placeholder = %q", got)
	}
}

func TestLoadIngressGuardrailsExceptionsDefaults(t *testing.T) {
	rt, err := Load([]byte(guardrailTestBase()+`
ingress_guardrails {
  enabled = true
}
`), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g := rt.IngressGuardrails
	if got := ResolveGuardrailExceptionsPath(g); got == "" || !strings.HasSuffix(got, "guardrail-exceptions.json") {
		t.Fatalf("resolved path = %q", got)
	}
	if got := ResolveGuardrailPlaceholder(g); got != "REDACTED" {
		t.Fatalf("resolved placeholder = %q", got)
	}
}

func TestLoadIngressGuardrailsRejectsBadPlaceholder(t *testing.T) {
	if _, err := Load([]byte(guardrailTestBase()+`
ingress_guardrails {
  enabled = true
  redact_placeholder = "bad\nvalue"
}
`), "test.hcl"); err == nil {
		t.Fatalf("expected error for invalid placeholder")
	}
}

func TestConvertIngressGuardrailsExceptionsRoundTrip(t *testing.T) {
	src := guardrailTestBase() + `
ingress_guardrails {
  enabled = true
  exceptions_file = "/tmp/guardrail-exceptions.json"
  redact_placeholder = "NONSECRET"
}
`
	out, _, _, err := Convert([]byte(src), "test.hcl", false)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(string(out), "exceptions_file") || !strings.Contains(string(out), "redact_placeholder") {
		t.Fatalf("converted output drops exception fields: %s", out)
	}
	rt, err := Load(out, "test.json")
	if err != nil {
		t.Fatalf("load converted: %v", err)
	}
	if rt.IngressGuardrails.ExceptionsFile != "/tmp/guardrail-exceptions.json" || rt.IngressGuardrails.RedactPlaceholder != "NONSECRET" {
		t.Fatalf("round-tripped guardrails = %+v", rt.IngressGuardrails)
	}
}
