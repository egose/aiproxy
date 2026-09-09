package config

import (
	"strings"
	"testing"
)

const jsonTestConfig = `{
  "listener": {"http": {"public": {"address": ":8080"}}},
  "auth": {"main": {"mode": "none"}},
  "provider": {"openai": {"openai": {
    "api_key": "sk-test",
    "model": {"gpt-4o-mini": {}}
  }}}
}`

func TestLoadJSONMinimalConfig(t *testing.T) {
	rt, err := Load([]byte(jsonTestConfig), "test.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.Listener.Address != ":8080" {
		t.Errorf("address = %q, want :8080", rt.Listener.Address)
	}
	if rt.Auth.Mode != AuthModeNone {
		t.Errorf("auth mode = %q", rt.Auth.Mode)
	}
	providers := rt.Catalog.Providers()
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
	if providers[0].APIKey != "sk-test" {
		t.Errorf("api_key = %q", providers[0].APIKey)
	}
	if _, ok := providers[0].ModelByName["gpt-4o-mini"]; !ok {
		t.Errorf("models = %+v", providers[0].Models)
	}
}

func TestLoadJSONSingleLine(t *testing.T) {
	cfg := `{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{"openai":{"api_key":"sk-test","model":{"gpt-4o-mini":{}}}}}}`
	if strings.Contains(cfg, "\n") {
		t.Fatal("test config must be a single line")
	}
	rt, err := Load([]byte(cfg), "test.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.Listener.Address != ":8080" {
		t.Errorf("address = %q, want :8080", rt.Listener.Address)
	}
}

func TestLoadEnvJSONSingleLine(t *testing.T) {
	t.Setenv(ConfigEnvVar, `{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{"openai":{"api_key":"sk-test","model":{"gpt-4o-mini":{}}}}}}`)
	rt, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if rt.Listener.Address != ":8080" {
		t.Fatalf("address = %q", rt.Listener.Address)
	}
}

func TestLoadJSONDerivedProvider(t *testing.T) {
	cfg := `{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai-compatible":{
    "base":{"base_url":"https://example.com/v1","api_key":"sk-base","model":{"m":{}}},
    "derived":{"extends":"base","api_key":"sk-derived"}
  }},"alias":{"chat":{"algorithm":"round_robin","target":[{"provider":"derived","model":"m"},{"provider":"base","model":"m"}]}}}`
	rt, err := Load([]byte(cfg), "test.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	derived := testProvider(t, rt, "derived")
	if derived.APIKey != "sk-derived" || derived.BaseURL != "https://example.com/v1" {
		t.Fatalf("derived = %+v", derived)
	}
	a := testAlias(t, rt, "chat")
	if len(a.Targets) != 2 {
		t.Fatalf("targets = %+v", a.Targets)
	}
}

func TestLoadJSONDerivedRejectsForbiddenAttr(t *testing.T) {
	cfg := `{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{
    "base":{"api_key":"k","model":{"m":{}}},
    "child":{"extends":"base","base_url":"https://example.com/v1","api_key":"k2"}
  }}}`
	_, err := Load([]byte(cfg), "test.json")
	if err == nil || !strings.Contains(err.Error(), "derived provider cannot declare base_url") {
		t.Fatalf("error = %v, want derived base_url rejection", err)
	}
}

func TestLoadJSONEnvExpansion(t *testing.T) {
	t.Setenv("AIPROXY_TEST_JSON_KEY", "sk-from-env")
	cfg := `{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{"openai":{"api_key":env("AIPROXY_TEST_JSON_KEY"),"model":{"m":{}}}}}}`
	rt, err := Load([]byte(cfg), "test.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := rt.Catalog.Providers()[0].APIKey; got != "sk-from-env" {
		t.Errorf("api_key = %q", got)
	}
}

func TestLoadInvalidKeepsNativeError(t *testing.T) {
	_, err := Load([]byte(`invalid hcl >>>`), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "Invalid block definition") {
		t.Fatalf("error = %v, want native HCL diagnostic", err)
	}
}
