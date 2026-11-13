package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/tlsprofile"
)

const tlsConfigureSeed = "listener \"http\" \"public\" { address = \":8080\" }\nauth \"main\" { mode = \"none\" }\n"

func tlsConfigureFixture(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile("../../internal/tlsprofile/testdata/bun-1.4.4-fetch.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "profile.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func tlsConfigureProvider(t *testing.T, path string, extra ...string) config.Provider {
	t.Helper()
	args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "profiled"}
	if _, _, err := executeRootCommand("", append(args, extra...)...); err != nil {
		t.Fatal(err)
	}
	rt, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := rt.Catalog.Provider("profiled")
	if !ok {
		t.Fatal("profiled provider missing")
	}
	return p
}

func TestConfigureProviderTLSClientHelloBuiltinsAndReset(t *testing.T) {
	for _, selector := range append([]string{""}, tlsprofile.BuiltinSelectors()...) {
		t.Run(selector, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.hcl")
			if err := os.WriteFile(path, []byte(tlsConfigureSeed), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--type", "openai", "--api-key", "synthetic-key", "--model", "m"}
			if selector != "" {
				args = append(args, "--tls-client-hello", selector)
			}
			p := tlsConfigureProvider(t, path, args...)
			if p.TLSClientHello != selector || p.HasTLSClientHello != (selector != "") {
				t.Fatal("creation did not retain selector declaration and alias")
			}
			before := p.TLSProfile.Identity()
			p = tlsConfigureProvider(t, path, "--display-name", "Preserved")
			if p.TLSClientHello != selector || p.TLSProfile.Identity() != before {
				t.Fatal("partial edit changed TLS profile")
			}
			p = tlsConfigureProvider(t, path, "--tls-client-hello", "native")
			if !p.TLSProfile.IsNative() || p.TLSClientHello != "native" || !p.HasTLSClientHello {
				t.Fatal("explicit native did not reset TLS profile")
			}
		})
	}
}

func TestConfigureProviderTLSCustomEnvSourceAndConversion(t *testing.T) {
	dir := t.TempDir()
	file := tlsConfigureFixture(t, dir)
	t.Setenv("CLI_TLS_SELECTOR", "custom")
	t.Setenv("CLI_TLS_SOURCE", "profile.json")
	path := filepath.Join(dir, "config.hcl")
	secrets := filepath.Join(dir, "keys.json")
	if err := os.WriteFile(path, []byte(tlsConfigureSeed), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	p := tlsConfigureProvider(t, path, "--type", "openai", "--api-key", "staged-key", "--secrets-path", secrets, "--secrets-key", "staged", "--model", "m", "--tls-client-hello", `env("CLI_TLS_SELECTOR")`, "--tls-client-hello-spec-file", `env("CLI_TLS_SOURCE")`)
	if !p.TLSProfile.IsCustom() || p.TLSClientHelloSpecFile != "profile.json" || p.APIKey != "staged-key" {
		t.Fatal("source-relative custom profile and staged secret did not round-trip")
	}
	identity := p.TLSProfile.Identity()
	p = tlsConfigureProvider(t, path, "--display-name", "Preserve expressions")
	data, _ := os.ReadFile(path)
	for _, expression := range []string{`tls_client_hello = env("CLI_TLS_SELECTOR")`, `tls_client_hello_spec_file = env("CLI_TLS_SOURCE")`} {
		if !bytes.Contains(data, []byte(expression)) {
			t.Fatal("edit lost TLS env expression spelling")
		}
	}
	if p.TLSProfile.Identity() != identity {
		t.Fatal("partial edit changed custom contents")
	}
	jsonPath := filepath.Join(t.TempDir(), "converted.json")
	if _, _, err := executeRootCommand("", "convert", jsonPath, "--config", path); err != nil {
		t.Fatal(err)
	}
	hclPath := filepath.Join(t.TempDir(), "converted.hcl")
	if _, _, err := executeRootCommand("", "convert", hclPath, "--config", jsonPath); err != nil {
		t.Fatal(err)
	}
	p = tlsConfigureProvider(t, hclPath, "--display-name", "Relocated edit")
	if p.TLSProfile.Identity() != identity || p.TLSClientHelloSpecFile != file {
		t.Fatal("converted custom reference did not survive relocation and editing")
	}
	p = tlsConfigureProvider(t, path, "--tls-client-hello", "native")
	data, _ = os.ReadFile(path)
	if !p.TLSProfile.IsNative() || p.HasTLSClientHelloSpecFile || bytes.Contains(data, []byte("tls_client_hello_spec_file")) {
		t.Fatal("native reset retained stale custom source")
	}
}

func TestConfigureProviderTLSInteractiveCustomCreationAndEditing(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "edit"}[existing], func(t *testing.T) {
			dir := t.TempDir()
			tlsConfigureFixture(t, dir)
			path := filepath.Join(dir, "config.hcl")
			seed := tlsConfigureSeed
			if existing {
				seed += "provider \"openai\" \"profiled\" {\n  api_key = \"synthetic-key\"\n  tls_client_hello = \"chrome\"\n  model \"m\" {}\n}\n"
			}
			if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
				t.Fatal(err)
			}
			answers := []string{"", "", "Interactive", "", "custom", "profile.json", "", "", "", "", "inline", "", "y"}
			if existing {
				answers = append([]string{""}, answers...)
			}
			args := []string{"configure", "provider", "--config", path, "--name", "profiled", "--type", "openai", "--api-key", "synthetic-key", "--model", "m"}
			stdout, stderr, err := executeRootCommand(strings.Join(answers, "\n")+"\n", args...)
			if err != nil {
				t.Fatalf("interactive custom configure: %v\n%s\n%s", err, stdout, stderr)
			}
			rt, err := config.LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			p, ok := rt.Catalog.Provider("profiled")
			if !ok || !p.TLSProfile.IsCustom() || p.TLSClientHelloSpecFile != "profile.json" || p.DisplayName != "Interactive" || p.APIKey != "synthetic-key" {
				t.Fatal("interactive creation/editing lost custom selection, source or unrelated fields")
			}
		})
	}
}

func TestConfigureProviderTLSRejectsInvalidBeforeWrites(t *testing.T) {
	for _, args := range [][]string{
		{"--tls-client-hello", ""}, {"--tls-client-hello", "Chrome"},
		{"--tls-client-hello", "firefox"}, {"--tls-client-hello", "bun"},
		{"--tls-client-hello", "boringssl"}, {"--tls-client-hello", "chrome-155"},
		{"--tls-client-hello", "custom"},
		{"--tls-client-hello", "custom", "--tls-client-hello-spec-file", "missing-private-profile.json"},
		{"--tls-client-hello-spec-file", ""},
		{"--tls-client-hello", "native", "--tls-client-hello-spec-file", "private-reference"},
	} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			dir := t.TempDir()
			path, secrets := filepath.Join(dir, "config.hcl"), filepath.Join(dir, "keys.json")
			if err := os.WriteFile(path, []byte(tlsConfigureSeed), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(secrets, []byte(`{"old":"preserved"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			flags := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "profiled", "--type", "openai", "--api-key", "rejected-secret", "--secrets-path", secrets, "--secrets-key", "staged", "--model", "m"}
			_, _, err := executeRootCommand("", append(flags, args...)...)
			if err == nil || strings.Contains(err.Error(), "missing-private-profile") || strings.Contains(err.Error(), "private-reference") {
				t.Fatalf("expected a controlled TLS validation error, got %v", err)
			}
			configData, _ := os.ReadFile(path)
			secretsData, _ := os.ReadFile(secrets)
			if string(configData) != tlsConfigureSeed || string(secretsData) != `{"old":"preserved"}` {
				t.Fatal("rejected TLS edit wrote config or secrets")
			}
		})
	}
}

func TestConfigureProviderTLSDerivedDeclarationsAndTransition(t *testing.T) {
	dir := t.TempDir()
	tlsConfigureFixture(t, dir)
	path := filepath.Join(dir, "config.hcl")
	seed := tlsConfigureSeed + `provider "openai" "base" {
  api_key = "base-key"
  tls_client_hello = "custom"
  tls_client_hello_spec_file = "profile.json"
  model "m" {}
}
`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	p := tlsConfigureProvider(t, path, "--type", "openai", "--api-key", "local-key", "--model", "m", "--tls-client-hello", "chrome")
	before, _ := os.ReadFile(path)
	for _, flags := range [][]string{
		{"--tls-client-hello", "native"}, {"--tls-client-hello", ""},
		{"--tls-client-hello-spec-file", ""}, {"--tls-client-hello-spec-file", "profile.json"},
	} {
		args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "profiled", "--extends", "base"}
		if _, _, err := executeRootCommand("", append(args, flags...)...); err == nil || !strings.Contains(err.Error(), "derived provider") {
			t.Fatalf("derived local flags must fail: %v", err)
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("invalid derived declaration was written")
		}
	}
	p = tlsConfigureProvider(t, path, "--extends", "base")
	data, _ := os.ReadFile(path)
	blocks, err := parseTopLevelBlocks(string(data))
	if err != nil {
		t.Fatal(err)
	}
	local := existingProviderInput(blocks, "profiled")
	if !p.TLSClientHelloInherited || !p.TLSProfile.IsCustom() || local.HasTLSClientHello || local.HasTLSClientHelloSpecFile {
		t.Fatal("static derivative did not inherit custom profile without local declarations")
	}
}

func TestPromptProviderTLSClientHelloChoicesAndCustomValidation(t *testing.T) {
	dir := t.TempDir()
	tlsConfigureFixture(t, dir)
	input := providerInput{ProviderType: "openai", Name: "profiled"}
	for _, selector := range tlsprofile.BuiltinSelectors() {
		var out bytes.Buffer
		prompts := newPromptSession(strings.NewReader("firefox\n"+selector+"\n"), &out)
		got, err := promptProviderTLSInput(&prompts, input, providerOptions{}, "")
		if err != nil || got.TLSClientHello != selector || !got.HasTLSClientHello || !strings.Contains(out.String(), "Choose one of the listed") {
			t.Fatalf("validated preset prompt: selector=%s err=%v", selector, err)
		}
	}
	var out bytes.Buffer
	prompts := newPromptSession(strings.NewReader("custom\nmissing-private-file.json\nprofile.json\n"), &out)
	got, err := promptProviderTLSInput(&prompts, input, providerOptions{ConfigFilename: filepath.Join(dir, "config.hcl")}, "")
	if err != nil || got.TLSClientHello != "custom" || got.TLSClientHelloSpecFile != "profile.json" || !got.HasTLSClientHelloSpecFile {
		t.Fatalf("source-relative custom prompt: %v", err)
	}
	if strings.Contains(out.String(), "missing-private-file") || !strings.Contains(out.String(), "cannot inspect the source file") {
		t.Fatal("interactive custom-source error leaked the source or did not re-prompt")
	}
	t.Setenv("PROMPT_TLS_SELECTOR", "custom")
	t.Setenv("PROMPT_TLS_SOURCE", "profile.json")
	input.TLSClientHello, input.TLSClientHelloSpecFile = `env("PROMPT_TLS_SELECTOR")`, `env("PROMPT_TLS_SOURCE")`
	input.HasTLSClientHello, input.HasTLSClientHelloSpecFile = true, true
	prompts = newPromptSession(strings.NewReader("\n\n"), &out)
	got, err = promptProviderTLSInput(&prompts, input, providerOptions{ConfigFilename: filepath.Join(dir, "config.hcl")}, "")
	if err != nil || got.TLSClientHello != input.TLSClientHello || got.TLSClientHelloSpecFile != input.TLSClientHelloSpecFile {
		t.Fatalf("interactive edit did not preserve TLS env expression spelling: %v", err)
	}
}
