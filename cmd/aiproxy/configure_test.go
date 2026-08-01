package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureProviderCreatesConfigAndSecrets(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	input := strings.Join([]string{
		configPath,
		"",
		"primary",
		"",
		"",
		secretsPath,
		"",
		"sk-test-primary",
		"gpt-4o-mini",
		"",
		"",
		"",
		"n",
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "provider")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"provider \"openai\" \"primary\" {",
		"api_key_ref {",
		"path = \"" + secretsPath + "\"",
		"key  = \"primary\"",
		"model \"gpt-4o-mini\" {",
		"capabilities = [\"chat\", \"responses\"]",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("config output missing %q:\n%s", check, configText)
		}
	}

	secretsData, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("ReadFile(secrets): %v", err)
	}
	if !strings.Contains(string(secretsData), `"primary": "sk-test-primary"`) {
		t.Fatalf("secrets output missing provider key:\n%s", string(secretsData))
	}
	if !strings.Contains(stdout, `updated provider "primary"`) {
		t.Fatalf("stdout missing provider summary:\n%s", stdout)
	}
}

func TestConfigureProviderRejectsInvalidProviderName(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	input := strings.Join([]string{
		configPath,
		"",
		"My Provider",    // invalid: uppercase + space -> re-prompt
		"primary/backup", // invalid: contains '/' -> re-prompt
		"primary",        // valid
		"",
		"",
		secretsPath,
		"",
		"sk-test-primary",
		"gpt-4o-mini",
		"",
		"",
		"",
		"n",
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "provider")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if !strings.Contains(strings.ToLower(stdout), "must start with [a-z0-9]") {
		t.Fatalf("stdout missing invalid-name feedback:\n%s", stdout)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	if !strings.Contains(string(configData), `provider "openai" "primary" {`) {
		t.Fatalf("config did not use the valid provider name:\n%s", string(configData))
	}
}

func TestConfigureProviderUpdatesExistingBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "primary" {
  api_key = "old-key"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	input := strings.Join([]string{
		configPath,
		"2",
		"1",
		"2",
		"primary",
		"Backup provider",
		"https://llm.internal/v1",
		"2",
		`env("LOCALAI_API_KEY")`,
		"y",
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "provider")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"provider \"openai-compatible\" \"primary\" {",
		"display_name = \"Backup provider\"",
		"base_url = \"https://llm.internal/v1\"",
		"api_key = env(\"LOCALAI_API_KEY\")",
		"model \"gpt-4o-mini\" {",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("updated config missing %q:\n%s", check, configText)
		}
	}
	if strings.Count(configText, `provider "`) != 1 {
		t.Fatalf("expected one provider block after update:\n%s", configText)
	}
	if !strings.Contains(stdout, `updated provider "primary"`) {
		t.Fatalf("stdout missing provider summary:\n%s", stdout)
	}
}

func TestConfigureProviderDeleteFlagRemovesBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "primary" {
  api_key = "sk-test"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	stdout, stderr, err := executeRootCommand("", "configure", "provider", "--config", configPath, "--delete", "--name", "primary")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	if strings.Contains(configText, `provider "openai" "primary"`) {
		t.Fatalf("provider block still present after delete:\n%s", configText)
	}
	if !strings.Contains(stdout, `deleted provider "primary"`) {
		t.Fatalf("stdout missing delete summary:\n%s", stdout)
	}
}

func TestConfigureProviderNonInteractiveFlags(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "openai-compatible",
		"--name", "backup",
		"--display-name", "Backup provider",
		"--base-url", "https://llm.internal/v1",
		"--secrets-path", secretsPath,
		"--secrets-key", "localai",
		"--api-key", "secret-value",
		"--model", "qwen3-32b=qwen/qwen3-32b",
		"--model-capabilities", "qwen3-32b=chat,responses",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"provider \"openai-compatible\" \"backup\" {",
		"display_name = \"Backup provider\"",
		"base_url = \"https://llm.internal/v1\"",
		"api_key_ref {",
		"path = \"" + secretsPath + "\"",
		"key  = \"localai\"",
		"model \"qwen3-32b\" {",
		"upstream_name = \"qwen/qwen3-32b\"",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("config output missing %q:\n%s", check, configText)
		}
	}
	secretsData, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("ReadFile(secrets): %v", err)
	}
	if !strings.Contains(string(secretsData), `"localai": "secret-value"`) {
		t.Fatalf("secrets output missing expected value:\n%s", string(secretsData))
	}
}

func TestConfigureAuthCreatesBearerStaticBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

provider "openai" "primary" {
  api_key = "sk-test"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	input := strings.Join([]string{
		configPath,
		"main",
		"2",
		"y",
		"120",
		"120",
		"internal-app",
		`env("AIPROXY_CLIENT_TOKEN")`,
		"internal",
		"alias/chat_default,openai/gpt-4o-mini",
		"n",
	}, "\n") + "\n"

	_, stderr, err := executeRootCommand(input, "configure", "auth")
	if err != nil {
		t.Fatalf("Execute(): %v\nstderr:\n%s", err, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"auth \"main\" {",
		"mode = \"bearer_static\"",
		"rate_limit {",
		"requests_per_minute = 120",
		"client \"internal-app\" {",
		"token = env(\"AIPROXY_CLIENT_TOKEN\")",
		"allowed_models = [\"alias/chat_default\", \"openai/gpt-4o-mini\"]",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("auth config missing %q:\n%s", check, configText)
		}
	}
}

func TestConfigureAliasCreatesAliasBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "primary" {
  api_key = "sk-test"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	input := strings.Join([]string{
		configPath,
		"chat_default",
		"",
		"1",
	}, "\n") + "\n"

	_, stderr, err := executeRootCommand(input, "configure", "alias")
	if err != nil {
		t.Fatalf("Execute(): %v\nstderr:\n%s", err, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"alias \"chat_default\" {",
		"algorithm = \"round_robin\"",
		"provider = \"primary\"",
		"model    = \"gpt-4o-mini\"",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("alias config missing %q:\n%s", check, configText)
		}
	}
}

func TestConfigureAliasNonInteractiveFlags(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "primary" {
  api_key = "sk-test"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "alias",
		"--config", configPath,
		"--non-interactive",
		"--name", "chat_default",
		"--algorithm", "round_robin",
		"--target", "primary/gpt-4o-mini",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	if !strings.Contains(configText, `alias "chat_default" {`) || !strings.Contains(configText, `provider = "primary"`) {
		t.Fatalf("alias config missing expected content:\n%s", configText)
	}
}

func TestConfigureAuthNonInteractiveFlags(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

provider "openai" "primary" {
  api_key = "sk-test"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "auth",
		"--config", configPath,
		"--non-interactive",
		"--name", "main",
		"--mode", "bearer_static",
		"--rate-limit-rpm", "120",
		"--rate-limit-burst", "120",
		"--client", "internal-app",
		"--client-token-env", "internal-app=AIPROXY_CLIENT_TOKEN",
		"--client-tenant", "internal-app=internal",
		"--client-allowed-models", "internal-app=alias/chat_default,openai/gpt-4o-mini",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"auth \"main\" {",
		"mode = \"bearer_static\"",
		"requests_per_minute = 120",
		"token = env(\"AIPROXY_CLIENT_TOKEN\")",
		"allowed_models = [\"alias/chat_default\", \"openai/gpt-4o-mini\"]",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("auth config missing %q:\n%s", check, configText)
		}
	}
}

func TestConfigureLoggingCreatesBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	input := strings.Join([]string{
		configPath,
		"3",
		"n",
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "logging")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"logging {",
		`level = "warn"`,
		"access_log = false",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("logging config missing %q:\n%s", check, configText)
		}
	}
	if !strings.Contains(stdout, `updated logging block`) {
		t.Fatalf("stdout missing logging summary:\n%s", stdout)
	}
}

func TestConfigureLoggingNonInteractiveFlags(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "logging",
		"--config", configPath,
		"--non-interactive",
		"--level", "error",
		"--access-log=false",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"logging {",
		`level = "error"`,
		"access_log = false",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("logging config missing %q:\n%s", check, configText)
		}
	}
}

func TestConfigureRootCommandPromptsForBlockSelection(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	input := strings.Join([]string{
		"4",
		"public",
		":8081",
		"n",
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "--config", configPath)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	if !strings.Contains(configText, `listener "http" "public" {`) || !strings.Contains(configText, `address = ":8081"`) {
		t.Fatalf("listener config missing expected content:\n%s", configText)
	}
	if !strings.Contains(stdout, `updated listener block`) {
		t.Fatalf("stdout missing listener summary:\n%s", stdout)
	}
}

func TestConfigureListenerRejectsInvalidDurationThenAcceptsValid(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	input := strings.Join([]string{
		configPath,
		"public",
		":8080",
		"y",
		"30",     // invalid duration (no unit) -> re-prompt
		"thirty", // invalid duration (non-parseable) -> re-prompt
		"-5s",    // negative duration -> re-prompt
		"30s",    // valid read_header timeout
		"",       // idle timeout blank OK
		"",       // write timeout blank OK
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "listener")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(strings.ToLower(stdout), "invalid duration") {
		t.Fatalf("stdout missing invalid-duration feedback:\n%s", stdout)
	}
	if !strings.Contains(strings.ToLower(stdout), "must not be negative") {
		t.Fatalf("stdout missing negative feedback:\n%s", stdout)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	if !strings.Contains(configText, `read_header = "30s"`) {
		t.Fatalf("config missing valid read_header timeout:\n%s", configText)
	}
	if strings.Contains(configText, "idle_timeout") {
		t.Fatalf("config should omit idle_timeout when blank:\n%s", configText)
	}
}

func TestConfigureAuthNonTUIPrintsModeDescription(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	// drive auth with mode 'none' and no rate limit, no clients (so flow ends cleanly)
	input := strings.Join([]string{
		configPath,
		"main",
		"1", // none
		"n", // no rate limit
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "auth")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, authModeDescription()) {
		t.Fatalf("stdout missing auth-mode description echo:\n%s", stdout)
	}
}

func TestConfigureProviderEnvExpressionRejectsMalformedThenAccepts(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	input := strings.Join([]string{
		configPath,
		"",                      // provider type default openai
		"primary",               // provider name
		"",                      // display name
		"2",                     // credential storage: env_expression
		`env(FOO)`,              // malformed env expression -> re-prompt
		`env("OPENAI_API_KEY")`, // valid
		"gpt-4o-mini",           // model name
		"",                      // model display name
		"",                      // upstream model name (defaults to model name)
		"",                      // capabilities default selection
		"n",                     // no more models
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "provider")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(strings.ToLower(stdout), "must match env") {
		t.Fatalf("stdout missing env-shape feedback:\n%s", stdout)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	if !strings.Contains(string(configData), `api_key = env("OPENAI_API_KEY")`) {
		t.Fatalf("config missing env expression api_key:\n%s", string(configData))
	}
	_ = secretsPath // not used in env_expression mode
}

func executeRootCommand(input string, args ...string) (string, string, error) {
	cmd := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func TestParseMultiChoiceValueAcceptsNamesAndNumbers(t *testing.T) {
	got, err := parseMultiChoiceValue("Capabilities", []string{"chat", "responses", "embeddings"}, "2,chat,2")
	if err != nil {
		t.Fatalf("parseMultiChoiceValue(): %v", err)
	}
	want := []string{"responses", "chat"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestSupportedCapabilitiesIncludeOpenAIExtendedOptions(t *testing.T) {
	got := supportedCapabilities("openai")
	want := []string{"chat", "responses", "embeddings", "images", "audio_transcriptions", "audio_speech"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestAliasTargetSpecsRoundTrip(t *testing.T) {
	input := []aliasTargetInput{{Provider: "primary", Model: "gpt-4o-mini"}, {Provider: "backup", Model: "qwen3-32b"}}
	specs := aliasTargetSpecs(input)
	got, err := buildAliasTargetsFromOptions(specs)
	if err != nil {
		t.Fatalf("buildAliasTargetsFromOptions(): %v", err)
	}
	if len(got) != len(input) {
		t.Fatalf("len(got) = %d, want %d (%v)", len(got), len(input), got)
	}
	for i := range input {
		if got[i] != input[i] {
			t.Fatalf("got[%d] = %+v, want %+v", i, got[i], input[i])
		}
	}
}

func TestAvailablePublicModelsIncludesDirectAndAliasNames(t *testing.T) {
	blocks := []topLevelBlock{
		{Type: "provider", Labels: []string{"openai", "primary"}, Text: "provider \"openai\" \"primary\" {\n  model \"gpt-4o-mini\" {}\n  model \"text-embedding-3-large\" {}\n}\n"},
		{Type: "alias", Labels: []string{"chat_default"}},
	}
	got := availablePublicModels(blocks)
	want := []string{"alias/chat_default", "primary/gpt-4o-mini", "primary/text-embedding-3-large"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestUpsertProviderModelReplacesInPlace(t *testing.T) {
	models := []providerModelInput{{Name: "first"}, {Name: "second"}}
	got, err := upsertProviderModel(models, providerModelInput{Name: "renamed"}, "second")
	if err != nil {
		t.Fatalf("upsertProviderModel(): %v", err)
	}
	if len(got) != 2 || got[0].Name != "first" || got[1].Name != "renamed" {
		t.Fatalf("got = %+v", got)
	}
}

func TestUpsertAuthClientRejectsDuplicateName(t *testing.T) {
	clients := []authClientInput{{Name: "ci"}, {Name: "internal"}}
	_, err := upsertAuthClient(clients, authClientInput{Name: "ci"}, "internal")
	if err == nil || !strings.Contains(err.Error(), `client "ci" already exists`) {
		t.Fatalf("expected duplicate-name error, got %v", err)
	}
}

func TestBuildReviewSummaryIncludesMetadataAndPreview(t *testing.T) {
	got := buildReviewSummary([]string{"Config path: /tmp/config.hcl", "Action: update listener block"}, "listener \"http\" \"public\" {}")
	checks := []string{"Config path: /tmp/config.hcl", "Action: update listener block", `listener "http" "public" {}`}
	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Fatalf("review summary missing %q:\n%s", check, got)
		}
	}
}
