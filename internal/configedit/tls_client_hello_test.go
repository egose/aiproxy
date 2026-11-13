package configedit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/tlsprofile"
)

func tlsEditFixture(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile("../tlsprofile/testdata/bun-1.4.4-fetch.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "profile.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderProviderTLSClientHello(t *testing.T) {
	for _, selector := range append([]string{""}, tlsprofile.BuiltinSelectors()...) {
		t.Run(selector, func(t *testing.T) {
			input := ProviderInput{ProviderType: "openai", Name: "profiled", TLSClientHello: selector}
			block := RenderProviderBlock(input, "")
			if strings.Contains(block, "tls_client_hello") != (selector != "") {
				t.Fatal("selector omission was not preserved")
			}
			if selector != "" && !strings.Contains(block, `tls_client_hello = "`+selector+`"`) {
				t.Fatal("selector alias was not preserved")
			}
			if err := ValidateGeneratedConfig([]byte(block), "config.hcl"); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, input := range []ProviderInput{
		{HasTLSClientHello: true},
		{TLSClientHello: "native", HasTLSClientHelloSpecFile: true},
		{TLSClientHello: "native", Extends: "base"},
		{HasTLSClientHello: true, Extends: "base"},
		{HasTLSClientHelloSpecFile: true, Extends: "base"},
	} {
		input.ProviderType, input.Name = "openai", "invalid"
		if err := ValidateGeneratedConfig([]byte(RenderProviderBlock(input, "")), "config.hcl"); err == nil {
			t.Fatalf("invalid declaration silently disappeared: %+v", input)
		}
	}
}

func TestWriteProviderTLSClientHelloUsesSourceAndStagedSecrets(t *testing.T) {
	dir := t.TempDir()
	tlsEditFixture(t, dir)
	t.Setenv("EDIT_TLS_SELECTOR", "custom")
	t.Setenv("EDIT_TLS_REFERENCE", "profile.json")
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")
	input := ProviderInput{
		ProviderType: "openai", Name: "profiled",
		TLSClientHello: `env("EDIT_TLS_SELECTOR")`, TLSClientHelloSpecFile: `env("EDIT_TLS_REFERENCE")`,
		Credential: ProviderCredentialInput{Mode: "secrets_file", SecretsPath: secretsPath, SecretsKey: "staged"},
		Models:     []ProviderModelInput{{Name: "m"}},
	}
	source := "listener \"http\" \"public\" { address = \":8080\" }\nauth \"main\" { mode = \"none\" }\n" + RenderProviderBlock(input, "")
	t.Chdir(t.TempDir())
	if err := ValidateGeneratedConfig([]byte(source), configPath); err != nil {
		t.Fatalf("TLS-only validation must not resolve uncommitted api_key_ref: %v", err)
	}
	if err := WriteProviderFiles(configPath, source, SecretsUpdate{Path: secretsPath, Key: "staged", Value: "staged-key"}); err != nil {
		t.Fatal(err)
	}
	beforeConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeSecrets, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expression := range []string{`tls_client_hello = env("EDIT_TLS_SELECTOR")`, `tls_client_hello_spec_file = env("EDIT_TLS_REFERENCE")`} {
		if !bytes.Contains(beforeConfig, []byte(expression)) {
			t.Fatal("environment expression spelling was not preserved")
		}
	}
	rt, err := config.LoadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := rt.Catalog.Provider("profiled")
	if !ok || !p.TLSProfile.IsCustom() || p.TLSClientHelloSpecFile != "profile.json" || p.APIKey != "staged-key" {
		t.Fatal("source-relative TLS and staged credential did not activate together")
	}
	t.Setenv("EDIT_TLS_REFERENCE", "missing-private-profile.json")
	if err := WriteProviderFiles(configPath, source, SecretsUpdate{Path: secretsPath, Key: "staged", Value: "rejected-key"}); err == nil || strings.Contains(err.Error(), "missing-private-profile") {
		t.Fatalf("missing custom source must fail without its path: %v", err)
	}
	afterConfig, _ := os.ReadFile(configPath)
	afterSecrets, _ := os.ReadFile(secretsPath)
	if !bytes.Equal(beforeConfig, afterConfig) || !bytes.Equal(beforeSecrets, afterSecrets) {
		t.Fatal("TLS validation failure wrote config or staged secrets")
	}
}

func TestValidateGeneratedConfigJSONTLSOnly(t *testing.T) {
	dir := t.TempDir()
	tlsEditFixture(t, dir)
	t.Setenv("EDIT_JSON_TLS", "custom")
	source := []byte(`{"provider":{"openai":{"profiled":{"tls_client_hello":env("EDIT_JSON_TLS"),"tls_client_hello_spec_file":"profile.json","api_key_ref":{"key":"not-yet-committed"}}}}}`)
	if err := ValidateGeneratedConfig(source, filepath.Join(dir, "config.json")); err != nil {
		t.Fatal(err)
	}
	for _, selector := range []string{`null`, `""`, `"firefox"`} {
		candidate := bytes.Replace(source, []byte(`env("EDIT_JSON_TLS")`), []byte(selector), 1)
		if err := ValidateGeneratedConfig(candidate, filepath.Join(dir, "config.json")); err == nil {
			t.Fatal("invalid JSON TLS declaration passed generated validation")
		}
	}
}

func TestWriteProviderTLSPreflightRejectsMalformedBeforeStaging(t *testing.T) {
	for _, source := range []string{
		`{"provider":{"openai":{"p":{"extends":"base","extends":"base","tls_client_hello":false}}}}`,
		`{"provider":{"openai":{"p":{"extends":"base","extends":"base","tls_client_hello_spec_file":{"private-content":true}}}}}`,
		`{"provider":{"openai":{"p":{"extends":42,"tls_client_hello":"native"}}}}`,
		`{"provider":{"openai":{"p":{"extends":false,"tls_client_hello_spec_file":null}}}}`,
	} {
		dir := t.TempDir()
		configPath, secretsPath := filepath.Join(dir, "config.json"), filepath.Join(dir, "keys.json")
		if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(secretsPath, []byte(`{"old":"preserved"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		err := WriteProviderFiles(configPath, source, SecretsUpdate{Path: secretsPath, Key: "staged", Value: "rejected-key"})
		if err == nil || strings.Contains(err.Error(), "private-content") {
			t.Fatalf("malformed TLS preflight must return a controlled error: %v", err)
		}
		configData, _ := os.ReadFile(configPath)
		secretsData, _ := os.ReadFile(secretsPath)
		if string(configData) != "{}\n" || string(secretsData) != `{"old":"preserved"}` {
			t.Fatal("malformed TLS candidate wrote config or staged credentials")
		}
	}
}
