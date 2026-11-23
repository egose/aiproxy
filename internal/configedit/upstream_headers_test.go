package configedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpstreamHeadersOnlyAppearInFileRendering(t *testing.T) {
	const providerExpr = `{ X-Provider = "provider-fixture-secret" }`
	const modelExpr = `{
      X-Model = env("FIXTURE_HEADER") # model-fixture-secret
    }`
	const aliasExpr = `{ X-Alias = "alias-fixture-secret" }`
	provider := ProviderInput{
		ProviderType: "openai", Name: "derived", Extends: "base", UpstreamHeadersExpr: providerExpr,
		Models: []ProviderModelInput{{Name: "header-only", UpstreamHeadersExpr: modelExpr}, {Name: "explicit-empty", UpstreamHeadersExpr: "{}"}, {Name: "not-declared"}},
	}
	alias := AliasInput{Name: "pool", Algorithm: "round_robin", UpstreamHeadersExpr: aliasExpr, Targets: []AliasTargetInput{{Provider: "derived", Model: "header-only"}}}
	for _, tc := range []struct {
		file, preview string
		expressions   []string
	}{
		{RenderProviderBlock(provider, ""), RenderProviderPreview(provider, ""), []string{providerExpr, modelExpr, "{}"}},
		{RenderAliasBlock(alias), RenderAliasPreview(alias), []string{aliasExpr}},
	} {
		for _, expr := range tc.expressions {
			if !strings.Contains(tc.file, "upstream_headers = "+expr) {
				t.Fatal("file rendering omitted a local declaration or rewrote its expression")
			}
		}
		if strings.Count(tc.preview, "# upstream_headers preserved (values hidden)") != len(tc.expressions) ||
			strings.Contains(tc.preview, "fixture-secret") || strings.Contains(tc.preview, "FIXTURE_HEADER") || strings.Contains(tc.preview, "upstream_headers =") {
			t.Fatal("review rendering exposed a fixed header expression or its value")
		}
	}
}

func TestUpstreamHeaderWritePreflightRejectsInvalidDeclarations(t *testing.T) {
	const original = `upstream_headers = { X-Valid = "fixture" }` + "\n"
	dir := t.TempDir()
	path, secrets := filepath.Join(dir, "config.hcl"), filepath.Join(dir, "keys.json")
	if err := WriteConfigFile(path, original); err != nil {
		t.Fatal(err)
	}
	const invalid = `upstream_headers = { X-Test = "first", X-Test = "fixture-not-for-diagnostics" }` + "\n"
	for _, err := range []error{
		WriteConfigFile(path, invalid),
		WriteProviderFiles(path, invalid, SecretsUpdate{Path: secrets, Key: "new", Value: "fixture"}),
	} {
		if err == nil || !strings.Contains(err.Error(), "duplicate header name") || strings.Contains(err.Error(), "fixture-not-for-diagnostics") {
			t.Fatalf("expected safe duplicate rejection before writing, got %v", err)
		}
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != original {
		t.Fatalf("invalid declaration changed the config: %v", err)
	}
	if _, err := os.Stat(secrets); !os.IsNotExist(err) {
		t.Fatalf("invalid declaration created a staged secret: %v", err)
	}
}
