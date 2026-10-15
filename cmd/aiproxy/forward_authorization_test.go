package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureProviderForwardAuthorization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.hcl")
	seed := `listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "existing" {
  api_key = "key"
  model "m" {}
}`
	if err := os.WriteFile(path, []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "forward", "--type", "openai", "--forward-authorization", "--model", "m"); err != nil {
		t.Fatal(err)
	}
	rt := loadTestConfig(t, path)
	p, ok := rt.Catalog.Provider("forward")
	if !ok || !p.ForwardAuthorization || p.APIKey != "" {
		t.Fatal("forwarding provider was not configured")
	}
	if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "forward", "--display-name", "Forwarding"); err != nil {
		t.Fatal(err)
	}
	rt = loadTestConfig(t, path)
	p, _ = rt.Catalog.Provider("forward")
	if !p.ForwardAuthorization || p.DisplayName != "Forwarding" {
		t.Fatal("update did not preserve forwarding")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "existing", "--forward-authorization"); err == nil {
		t.Fatal("forwarding with an existing key must fail")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("failed update modified the config")
	}
	if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "forward", "--forward-authorization=false", "--api-key", "configured"); err != nil {
		t.Fatal(err)
	}
	rt = loadTestConfig(t, path)
	p, _ = rt.Catalog.Provider("forward")
	if p.ForwardAuthorization || p.APIKey != "configured" {
		t.Fatal("forwarding was not disabled")
	}
}
