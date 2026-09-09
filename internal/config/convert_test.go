package config

import (
	"strings"
	"testing"
)

const convertTestHCL = `
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "bearer_static"
  rate_limit {
    requests_per_minute = 120
  }
  client "ci" {
    token = "tok"
    tenant = "team-a"
    allowed_models = ["openai/gpt-4o-mini", "alias/chat"]
  }
}

provider "openai-compatible" "base" {
  base_url = "https://example.com/v1"
  api_key = "sk-base"
  model "z-ai/glm-5.2" {
    capabilities = ["chat", "responses"]
  }
}

provider "openai-compatible" "derived" {
  extends = "base"
  api_key = "sk-derived"
}

alias "chat" {
  algorithm = "round_robin"
  target {
    provider = "derived"
    model = "z-ai/glm-5.2"
  }
  target {
    provider = "base"
    model = "z-ai/glm-5.2"
  }
}
`

func checkConvertRuntime(t *testing.T, rt *Runtime) {
	t.Helper()
	if rt.Listener.Address != ":8080" {
		t.Errorf("address = %q", rt.Listener.Address)
	}
	if rt.Auth.Mode != AuthModeBearerStatic {
		t.Errorf("auth mode = %q", rt.Auth.Mode)
	}
	if rt.Auth.RateLimit == nil || rt.Auth.RateLimit.RequestsPerMinute != 120 {
		t.Errorf("rate limit = %+v", rt.Auth.RateLimit)
	}
	client, ok := rt.Auth.Clients["ci"]
	if !ok || client.Tenant != "team-a" || len(client.AllowedModels) != 2 {
		t.Errorf("client = %+v", rt.Auth.Clients)
	}
	for _, name := range []string{"base", "derived"} {
		p, ok := rt.Catalog.Provider(name)
		if !ok {
			t.Fatalf("provider %q missing", name)
		}
		if _, ok := p.ModelByName["z-ai/glm-5.2"]; !ok {
			t.Errorf("provider %q models = %+v", name, p.Models)
		}
	}
	a, ok := rt.Catalog.Alias("chat")
	if !ok || len(a.Targets) != 2 {
		t.Fatalf("alias = %+v", rt.Catalog.Aliases())
	}
}

func TestConvertHCLToJSON(t *testing.T) {
	out, from, to, err := Convert([]byte(convertTestHCL), "test.hcl", false)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if from != FormatHCL || to != FormatJSON {
		t.Fatalf("from/to = %q/%q", from, to)
	}
	rt, err := Load(out, "test.json")
	if err != nil {
		t.Fatalf("load converted: %v\n%s", err, out)
	}
	checkConvertRuntime(t, rt)
}

func TestConvertHCLToJSONCompact(t *testing.T) {
	out, _, _, err := Convert([]byte(convertTestHCL), "test.hcl", true)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if n := strings.Count(string(out), "\n"); n != 1 {
		t.Fatalf("compact output has %d newlines, want 1 trailing", n)
	}
	rt, err := Load(out, "test.json")
	if err != nil {
		t.Fatalf("load converted: %v", err)
	}
	checkConvertRuntime(t, rt)
}

func TestConvertJSONToHCL(t *testing.T) {
	jsonCfg := `{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{"openai":{"api_key":"sk-test","model":{"gpt-4o-mini":{}}}}}}`
	out, from, to, err := Convert([]byte(jsonCfg), "test.json", false)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if from != FormatJSON || to != FormatHCL {
		t.Fatalf("from/to = %q/%q", from, to)
	}
	if !strings.Contains(string(out), `provider "openai" "openai"`) {
		t.Fatalf("hcl output missing provider block:\n%s", out)
	}
	rt, err := Load(out, "test.hcl")
	if err != nil {
		t.Fatalf("load converted: %v\n%s", err, out)
	}
	if rt.Listener.Address != ":8080" || rt.Auth.Mode != AuthModeNone {
		t.Fatalf("runtime = %+v", rt)
	}
}

func TestConvertRoundTrip(t *testing.T) {
	jsonOut, _, _, err := Convert([]byte(convertTestHCL), "test.hcl", false)
	if err != nil {
		t.Fatalf("to JSON: %v", err)
	}
	hclOut, from, to, err := Convert(jsonOut, "test.json", false)
	if err != nil {
		t.Fatalf("to HCL: %v\n%s", err, jsonOut)
	}
	if from != FormatJSON || to != FormatHCL {
		t.Fatalf("from/to = %q/%q", from, to)
	}
	want, err := Load([]byte(convertTestHCL), "test.hcl")
	if err != nil {
		t.Fatalf("load original: %v", err)
	}
	got, err := Load(hclOut, "test.hcl")
	if err != nil {
		t.Fatalf("load round-tripped: %v\n%s", err, hclOut)
	}
	if got.Listener.Address != want.Listener.Address || got.Auth.Mode != want.Auth.Mode {
		t.Fatalf("listener/auth mismatch: %+v vs %+v", got.Listener, want.Listener)
	}
	for _, name := range []string{"base", "derived"} {
		wp, _ := want.Catalog.Provider(name)
		gp, ok := got.Catalog.Provider(name)
		if !ok || gp.APIKey != wp.APIKey || gp.BaseURL != wp.BaseURL {
			t.Fatalf("provider %q mismatch", name)
		}
	}
	wa, _ := want.Catalog.Alias("chat")
	ga, ok := got.Catalog.Alias("chat")
	if !ok || len(ga.Targets) != len(wa.Targets) {
		t.Fatalf("alias mismatch")
	}
	for i := range wa.Targets {
		if ga.Targets[i] != wa.Targets[i] {
			t.Fatalf("targets = %+v, want %+v", ga.Targets, wa.Targets)
		}
	}
}

func TestConvertJSONSingleTargetObject(t *testing.T) {
	jsonCfg := `{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{"openai":{"api_key":"k","model":{"m":{}}}}},"alias":{"a":{"algorithm":"round_robin","target":{"provider":"openai","model":"m"}}}}`
	out, _, _, err := Convert([]byte(jsonCfg), "test.json", false)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	rt, err := Load(out, "test.hcl")
	if err != nil {
		t.Fatalf("load converted: %v\n%s", err, out)
	}
	a, ok := rt.Catalog.Alias("a")
	if !ok || len(a.Targets) != 1 {
		t.Fatalf("alias = %+v", rt.Catalog.Aliases())
	}
}

func TestConvertResolvesEnvCalls(t *testing.T) {
	t.Setenv("AIPROXY_TEST_CONVERT_KEY", "sk-converted")
	hclCfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = env("AIPROXY_TEST_CONVERT_KEY")
  model "m" {}
}
`
	out, _, _, err := Convert([]byte(hclCfg), "test.hcl", true)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if strings.Contains(string(out), "env(") {
		t.Fatalf("output still contains env call:\n%s", out)
	}
	if !strings.Contains(string(out), "sk-converted") {
		t.Fatalf("output missing resolved secret:\n%s", out)
	}
}

func TestConvertInvalidErrors(t *testing.T) {
	_, _, _, err := Convert([]byte(`invalid hcl >>>`), "test.hcl", false)
	if err == nil || !strings.Contains(err.Error(), "Invalid block definition") {
		t.Fatalf("error = %v, want native HCL diagnostic", err)
	}
}

func TestConvertJSONUnknownBlockErrors(t *testing.T) {
	_, _, _, err := Convert([]byte(`{"bogus":{}}`), "test.json", false)
	if err == nil {
		t.Fatal("expected error for unknown block")
	}
}
