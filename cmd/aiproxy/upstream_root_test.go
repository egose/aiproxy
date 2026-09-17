package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

func loadTestConfig(t *testing.T, path string) *config.Runtime {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	rt, err := config.Load(data, path)
	if err != nil {
		t.Fatalf("Load(): %v\n%s", err, string(data))
	}
	return rt
}

func providerTimeout(t *testing.T, rt *config.Runtime, name string) time.Duration {
	t.Helper()
	if p, ok := rt.Catalog.Provider(name); ok {
		return p.UpstreamHeaderTimeout
	}
	for _, p := range rt.Catalog.DisabledProviders() {
		if p.Name == name {
			return p.UpstreamHeaderTimeout
		}
	}
	t.Fatalf("provider %q not found", name)
	return 0
}

func TestConfigureUpstreamPreservesProviderOverrides(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := "upstream_header_timeout = \"120s\"\n" +
		"\n" +
		"listener \"http\" \"public\" { address = \":8080\" }\n" +
		"auth \"main\" { mode = \"none\" }\n" +
		"provider \"openai\" \"primary\" {\n" +
		"  api_key = \"sk-test\"\n" +
		"  upstream_header_timeout = \"180s\"\n" +
		"  model \"gpt-4o-mini\" {}\n" +
		"}\n" +
		"provider \"openai\" \"plain\" {\n" +
		"  api_key = \"sk-test\"\n" +
		"  model \"gpt-4o-mini\" {}\n" +
		"}\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "upstream",
		"--config", configPath,
		"--non-interactive",
		"--upstream-header-timeout", "30s",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	text := string(data)
	if strings.Count(text, "upstream_header_timeout = \"180s\"") != 1 {
		t.Fatalf("provider override not preserved exactly once:\n%s", text)
	}
	if strings.Count(text, "upstream_header_timeout = \"30s\"") != 1 {
		t.Fatalf("expected exactly one root timeout:\n%s", text)
	}
	rt := loadTestConfig(t, configPath)
	if rt.UpstreamHeaderTimeout != 30*time.Second {
		t.Fatalf("root = %v, want 30s", rt.UpstreamHeaderTimeout)
	}
	if got := providerTimeout(t, rt, "primary"); got != 180*time.Second {
		t.Fatalf("primary = %v, want 180s", got)
	}
	if got := providerTimeout(t, rt, "plain"); got != 30*time.Second {
		t.Fatalf("plain = %v, want 30s", got)
	}
}

func TestConfigureUpstreamInsertsRootWhenOnlyProviderOverrideExists(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := "listener \"http\" \"public\" { address = \":8080\" }\n" +
		"auth \"main\" { mode = \"none\" }\n" +
		"provider \"openai\" \"primary\" {\n" +
		"  api_key = \"sk-test\"\n" +
		"  upstream_header_timeout = \"180s\"\n" +
		"  model \"gpt-4o-mini\" {}\n" +
		"}\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
	if _, _, err := executeRootCommand(
		"",
		"configure", "upstream",
		"--config", configPath,
		"--non-interactive",
		"--upstream-header-timeout", "45s",
	); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	text := string(data)
	if strings.Count(text, "upstream_header_timeout = \"180s\"") != 1 {
		t.Fatalf("provider override was rewritten:\n%s", text)
	}
	if !strings.Contains(text, "upstream_header_timeout = \"45s\"") {
		t.Fatalf("root timeout missing:\n%s", text)
	}
	rt := loadTestConfig(t, configPath)
	if rt.UpstreamHeaderTimeout != 45*time.Second {
		t.Fatalf("root = %v, want 45s", rt.UpstreamHeaderTimeout)
	}
	if got := providerTimeout(t, rt, "primary"); got != 180*time.Second {
		t.Fatalf("primary = %v, want 180s", got)
	}
}

func TestConfigureUpstreamSetsRootUserAgentDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := "listener \"http\" \"public\" { address = \":8080\" }\n" +
		"auth \"main\" { mode = \"none\" }\n" +
		"provider \"openai\" \"plain\" {\n" +
		"  api_key = \"sk-test\"\n" +
		"  model \"gpt-4o-mini\" {}\n" +
		"}\n" +
		"provider \"openai\" \"custom\" {\n" +
		"  api_key = \"sk-test\"\n" +
		"  user_agent = \"provider-agent/2.0\"\n" +
		"  model \"gpt-4o-mini\" {}\n" +
		"}\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
	if _, _, err := executeRootCommand(
		"",
		"configure", "upstream",
		"--config", configPath,
		"--non-interactive",
		"--upstream-header-timeout", "30s",
		"--user-agent", "root-agent/1.0",
		"--forward-user-agent",
	); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "user_agent = \"root-agent/1.0\"") {
		t.Fatalf("root user_agent missing:\n%s", text)
	}
	if !strings.Contains(text, "forward_user_agent = true") {
		t.Fatalf("root forward_user_agent missing:\n%s", text)
	}
	if strings.Count(text, "user_agent = \"provider-agent/2.0\"") != 1 {
		t.Fatalf("provider override not preserved exactly once:\n%s", text)
	}
	rt := loadTestConfig(t, configPath)
	if rt.UserAgent != "root-agent/1.0" {
		t.Fatalf("root UserAgent = %q, want root-agent/1.0", rt.UserAgent)
	}
	plain, ok := rt.Catalog.Provider("plain")
	if !ok {
		t.Fatal("plain provider missing from catalog")
	}
	if plain.UserAgent != "root-agent/1.0" {
		t.Fatalf("plain UserAgent = %q, want inherited root-agent/1.0", plain.UserAgent)
	}
	if !plain.ForwardUserAgent {
		t.Fatal("plain ForwardUserAgent = false, want inherited true")
	}
	custom, ok := rt.Catalog.Provider("custom")
	if !ok {
		t.Fatal("custom provider missing from catalog")
	}
	if custom.UserAgent != "provider-agent/2.0" {
		t.Fatalf("custom UserAgent = %q, want provider override", custom.UserAgent)
	}
}

func TestConfigureUpstreamClearsRootUserAgentOnEmptyFlag(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := "user_agent = \"root-agent/1.0\"\n" +
		"listener \"http\" \"public\" { address = \":8080\" }\n" +
		"auth \"main\" { mode = \"none\" }\n" +
		"provider \"openai\" \"plain\" {\n" +
		"  api_key = \"sk-test\"\n" +
		"  model \"gpt-4o-mini\" {}\n" +
		"}\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
	if _, _, err := executeRootCommand(
		"",
		"configure", "upstream",
		"--config", configPath,
		"--non-interactive",
		"--upstream-header-timeout", "30s",
		"--user-agent", "",
	); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	if strings.Contains(string(data), "user_agent") {
		t.Fatalf("root user_agent not cleared:\n%s", string(data))
	}
	rt := loadTestConfig(t, configPath)
	if rt.UserAgent != "" {
		t.Fatalf("root UserAgent = %q, want empty", rt.UserAgent)
	}
	plain, ok := rt.Catalog.Provider("plain")
	if !ok {
		t.Fatal("plain provider missing from catalog")
	}
	if plain.UserAgent != "" {
		t.Fatalf("plain UserAgent = %q, want empty default", plain.UserAgent)
	}
}

func TestConfigureUpstreamFailsOnInvalidSourceWithoutPublish(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := "provider \"openai\" \"primary\" {\n  api_key = \"k\"\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
	_, _, err := executeRootCommand(
		"",
		"configure", "upstream",
		"--config", configPath,
		"--non-interactive",
		"--upstream-header-timeout", "30s",
	)
	if err == nil {
		t.Fatalf("expected error for invalid source")
	}
	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile(config): %v", readErr)
	}
	if string(data) != seed {
		t.Fatalf("config modified on failed edit:\n%s", string(data))
	}
}
