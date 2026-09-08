package app

import (
	"context"
	"testing"
)

func TestBuildFromEnv(t *testing.T) {
	t.Setenv("AIPROXY_CONFIG", `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigFromEnv: true, Version: "test"})
	if err != nil {
		t.Fatalf("build from env: %v", err)
	}
	if a.Config.Listener.Address != ":0" {
		t.Fatalf("address = %q", a.Config.Listener.Address)
	}
}

func TestBuildFromEnvUnsetErrors(t *testing.T) {
	t.Setenv("AIPROXY_CONFIG", "")
	if _, err := Build(context.Background(), BuildOptions{ConfigFromEnv: true, Version: "test"}); err == nil {
		t.Fatal("expected error when AIPROXY_CONFIG is unset")
	}
}

func TestReloadRereadsEnv(t *testing.T) {
	t.Setenv("AIPROXY_CONFIG", `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigFromEnv: true, Version: "test"})
	if err != nil {
		t.Fatalf("build from env: %v", err)
	}
	t.Setenv("AIPROXY_CONFIG", `
listener "http" "public" { address = ":0" }
auth "main" {
  mode = "bearer_static"
  client "app" { token = "tok" }
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload from env: %v", err)
	}
	if got := string(a.Config.Auth.Mode); got != "bearer_static" {
		t.Fatalf("auth mode after reload = %q, want bearer_static", got)
	}
}
