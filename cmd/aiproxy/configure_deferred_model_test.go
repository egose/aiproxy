package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

const deferredConfigureModelBlock = `deferred_transport {
      control_timeout = env("CLI_MODEL_DEFER_CONTROL")
      max_wait = env("CLI_MODEL_DEFER_WAIT")
      poll_interval = env("CLI_MODEL_DEFER_POLL")
      max_control_calls = env("CLI_MODEL_DEFER_CALLS")
      max_control_retries = env("CLI_MODEL_DEFER_RETRIES")
    }`

const deferredConfigureModelCaller = `provider "openai-compatible" "peer" {
  base_url = "https://peer.example/mount/v1"
  api_key = env("CLI_DEFER_KEY")
  upstream_header_timeout = "90s"
  model "ordinary" { upstream_name = "executor/ordinary" }
  model "queued" {
    display_name = "Queued"
    upstream_name = "executor/queued"
    ` + deferredConfigureModelBlock + `
  }
  model "empty" {
    upstream_name = "executor/empty"
    deferred_transport {}
  }
}
`

const deferredConfigureModelReceiver = `provider "openai-compatible" "receiver" {
  base_url = "https://upstream.example/v1"
  api_key = "receiver-key"
  deferred_receiver {
    execution_timeout = env("CLI_MODEL_RECV_EXECUTION")
    max_result_bytes = env("CLI_MODEL_RECV_RESULT")
  }
  model "ordinary" {}
  model "queued" {}
}
`

func deferredModelConfigureFixture(t *testing.T, extra string) string {
	t.Helper()
	t.Setenv("CLI_DEFER_EXECUTION", "1m")
	t.Setenv("CLI_DEFER_REQUEST", "2048")
	t.Setenv("CLI_DEFER_KEY", "caller-key")
	t.Setenv("CLI_MODEL_ROTATED_KEY", "rotated-key")
	t.Setenv("CLI_MODEL_DEFER_CONTROL", "100ms")
	t.Setenv("CLI_MODEL_DEFER_WAIT", "20m")
	t.Setenv("CLI_MODEL_DEFER_POLL", "500ms")
	t.Setenv("CLI_MODEL_DEFER_CALLS", "48")
	t.Setenv("CLI_MODEL_DEFER_RETRIES", "0")
	t.Setenv("CLI_MODEL_RECV_EXECUTION", "30s")
	t.Setenv("CLI_MODEL_RECV_RESULT", "4096")
	path := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(path, []byte(deferredConfigureRoot+deferredConfigureModelCaller+deferredConfigureModelReceiver+extra), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func configuredDeferredModelProvider(t *testing.T, path, name string) config.Provider {
	t.Helper()
	rt, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := rt.Catalog.Provider(name); ok {
		return p
	}
	for _, p := range rt.Catalog.DisabledProviders() {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("configured provider %s disappeared", name)
	return config.Provider{}
}

func assertConfiguredDeferredModelsPreserved(t *testing.T, path, name string) config.Provider {
	t.Helper()
	p := configuredDeferredModelProvider(t, path, name)
	want := config.DeferredTransport{ControlTimeout: 100 * time.Millisecond, MaxWait: 20 * time.Minute, PollInterval: 500 * time.Millisecond, MaxControlCalls: 48, MaxControlRetries: 0}
	if p.DeferredTransport != nil || !reflect.DeepEqual(p.ModelByName["queued"].DeferredTransport, &want) || config.EffectiveDeferredTransport(p, p.ModelByName["ordinary"]) != nil {
		t.Fatal("edit lost or promoted a model caller block or changed the ordinary sibling")
	}
	if empty := p.ModelByName["empty"].DeferredTransport; name == "peer" && (empty == nil || *empty != config.DefaultDeferredTransport()) {
		t.Fatal("empty model caller block was stripped")
	}
	for _, model := range p.Models {
		if !reflect.DeepEqual(model.DeferredTransport, p.ModelByName[model.Name].DeferredTransport) {
			t.Fatal("edited model views disagree")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{deferredConfigureRoot, deferredConfigureModelBlock, deferredConfigureModelReceiver} {
		if !bytes.Contains(data, []byte(source)) {
			t.Fatal("edit changed root/receiver source or nested timing expressions")
		}
	}
	return p
}

func TestConfigureDeferredModelNonInteractiveCredentialSiblingAndDisablePreservation(t *testing.T) {
	path := deferredModelConfigureFixture(t, "")
	for _, edit := range []struct {
		flags []string
		key   string
	}{
		{[]string{"--display-name", "Edited"}, "caller-key"},
		{[]string{"--api-key-env", "CLI_MODEL_ROTATED_KEY"}, "rotated-key"},
		{[]string{"--model", "ordinary=executor/ordinary", "--model", "queued=executor/updated", "--model", "empty=executor/empty"}, "rotated-key"},
		{[]string{"--enabled=false"}, "rotated-key"},
		{[]string{"--display-name", "Still disabled"}, "rotated-key"},
		{[]string{"--enabled=true"}, "rotated-key"},
	} {
		args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "peer"}
		if _, _, err := executeRootCommand("", append(args, edit.flags...)...); err != nil {
			t.Fatal(err)
		}
		p := assertConfiguredDeferredModelsPreserved(t, path, "peer")
		if p.APIKey != edit.key {
			t.Fatal("model caller edit/disable discarded its configured credential")
		}
	}
	if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "peer", "--model", "ordinary", "--model", "queued"); err != nil {
		t.Fatal(err)
	}
	p := configuredDeferredModelProvider(t, path, "peer")
	if _, ok := p.ModelByName["empty"]; ok || p.ModelByName["queued"].DeferredTransport == nil {
		t.Fatal("explicit model removal stripped an opt-in from a retained model")
	}
	if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "peer", "--model", "ordinary"); err != nil {
		t.Fatal(err)
	}
	if p = configuredDeferredModelProvider(t, path, "peer"); len(p.Models) != 1 || config.HasCallerDeferredTransport(p) {
		t.Fatal("explicitly deleted caller models survived a bulk redefinition")
	}
}

func TestConfigureDeferredModelTextReconstructionPreservesRetainedOptIns(t *testing.T) {
	path := deferredModelConfigureFixture(t, "")
	answers := []string{
		"", "", "", "Text edit", "", "", "", "", "", "", "", "env_expression", "", "n",
		"ordinary", "Ordinary edited", "executor/ordinary", "", "y",
		"queued", "Queued edited", "executor/queued", "", "y",
		"empty", "", "executor/empty", "", "n",
	}
	stdout, stderr, err := executeRootCommand(strings.Join(answers, "\n")+"\n", "configure", "provider", "--config", path, "--name", "peer")
	if err != nil {
		t.Fatalf("text reconstruction: %v\n%s\n%s", err, stdout, stderr)
	}
	p := assertConfiguredDeferredModelsPreserved(t, path, "peer")
	if p.DisplayName != "Text edit" || p.ModelByName["ordinary"].DisplayName != "Ordinary edited" || p.ModelByName["queued"].DisplayName != "Queued edited" {
		t.Fatal("text path did not actually reconstruct and edit the models")
	}
}

type deferredPromptByteReader struct {
	io.Reader
}

func (r deferredPromptByteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.Reader.Read(p)
}

func deferredModelTUIPrompts(t *testing.T, answers []string) *promptSession {
	t.Helper()
	t.Setenv("TERM", "dumb")
	reader := deferredPromptByteReader{Reader: strings.NewReader(strings.Join(answers, "\n") + "\n")}
	prompts := newPromptSession(reader, &bytes.Buffer{})
	prompts.interactiveTUI = true
	return &prompts
}

func TestConfigureDeferredModelTUIReconstructionAndExplicitDeletion(t *testing.T) {
	path := deferredModelConfigureFixture(t, "")
	doc, err := loadConfigDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	existing := existingProviderInput(doc.blocks, "peer")
	prompts := deferredModelTUIPrompts(t, []string{
		"2", "2", "queued", "Queued UI edit", "", "0",
		"2", "1", "ordinary", "Ordinary UI edit", "", "0",
		"3", "3", "4",
	})
	models, err := promptProviderModels(prompts, existing.ProviderType, existing.Models)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].DisplayName != "Ordinary UI edit" || models[1].DisplayName != "Queued UI edit" || models[1].DeferredTransportBlock != deferredConfigureModelBlock || models[0].DeferredTransportBlock != "" {
		t.Fatalf("TUI reconstruction lost retained opt-ins or retained an explicitly deleted model: %+v\n%s", models, prompts.out)
	}
	if len(existing.Models) != 3 || existing.Models[1].DisplayName != "Queued" {
		t.Fatal("TUI edit mutated the original editor defaults")
	}
	existing.Models = models
	updated, err := upsertBlock(doc.source, renderProviderBlock(*existing), func(block topLevelBlock) bool {
		return block.Type == "provider" && len(block.Labels) >= 2 && block.Labels[1] == "peer"
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConfigFile(path, updated); err != nil {
		t.Fatal(err)
	}
	p := configuredDeferredModelProvider(t, path, "peer")
	if p.ModelByName["queued"].DeferredTransport == nil || config.EffectiveDeferredTransport(p, p.ModelByName["ordinary"]) != nil {
		t.Fatal("TUI-reconstructed config changed transport placement")
	}
}

func TestConfigureDeferredModelDerivativeOverlayPreservationAcrossPromptPaths(t *testing.T) {
	child := `provider "openai-compatible" "child" {
  extends = "peer"
  api_key = "child-key"
  model "queued" {
    ` + deferredConfigureModelBlock + `
  }
}
`
	path := deferredModelConfigureFixture(t, child)
	for _, flags := range [][]string{{"--display-name", "Noninteractive child"}, {"--api-key", "child-key"}} {
		args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "child"}
		if _, _, err := executeRootCommand("", append(args, flags...)...); err != nil {
			t.Fatal(err)
		}
		p := assertConfiguredDeferredModelsPreserved(t, path, "child")
		if p.ModelByName["queued"].UpstreamName != "executor/queued" || p.ModelByName["empty"].DeferredTransport != nil {
			t.Fatal("derivative inherited caller options or redefined an inherited model")
		}
	}
	answers := []string{"", "", "", "Text child", "", "inline", ""}
	stdout, stderr, err := executeRootCommand(strings.Join(answers, "\n")+"\n", "configure", "provider", "--config", path, "--name", "child")
	if err != nil {
		t.Fatalf("text derivative edit: %v\n%s\n%s", err, stdout, stderr)
	}
	assertConfiguredDeferredModelsPreserved(t, path, "child")
	doc, err := loadConfigDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	existing := existingProviderInput(doc.blocks, "child")
	prompts := deferredModelTUIPrompts(t, []string{"2", "child", "TUI child", "2", "3", "child-key"})
	input, _, err := promptProviderInput(prompts, existing, providerOptions{BaseProviderNames: []string{"peer"}, BaseProviderTypes: map[string]string{"peer": "openai-compatible"}})
	if err != nil {
		t.Fatal(err)
	}
	if input.DisplayName != "TUI child" || len(input.Models) != 1 || input.Models[0].DeferredTransportBlock != deferredConfigureModelBlock {
		t.Fatalf("TUI provider reconstruction lost its local model overlay: %+v\n%s", input, prompts.out)
	}
	for _, model := range input.Models {
		if model.UpstreamName != "" || model.DisplayName != "" || len(model.Capabilities) != 0 || model.Protocol != "" || model.Pricing != nil {
			t.Fatal("derivative model preservation enabled arbitrary inherited-field editing")
		}
	}
	updated, err := upsertBlock(doc.source, renderProviderBlock(input), func(block topLevelBlock) bool {
		return block.Type == "provider" && len(block.Labels) >= 2 && block.Labels[1] == "child"
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConfigFile(path, updated); err != nil {
		t.Fatal(err)
	}
	assertConfiguredDeferredModelsPreserved(t, path, "child")
}

func TestConfigureDeferredModelConcreteToDerivedClearsDefinitionsNotOptIns(t *testing.T) {
	base := `provider "openai-compatible" "alternate" {
  base_url = "https://alternate.example/v1"
  api_key = "base-key"
  model "ordinary" {
    upstream_name = "inherited/ordinary"
    deferred_transport {}
  }
  model "queued" { upstream_name = "inherited/queued" }
  model "empty" { upstream_name = "inherited/empty" }
}
`
	path := deferredModelConfigureFixture(t, base)
	if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "peer", "--extends", "alternate"); err != nil {
		t.Fatal(err)
	}
	p := assertConfiguredDeferredModelsPreserved(t, path, "peer")
	if p.BaseURL != "https://alternate.example/v1" || p.ModelByName["queued"].UpstreamName != "inherited/queued" {
		t.Fatal("concrete-to-derived edit retained arbitrary model or endpoint definitions")
	}
	doc, err := loadConfigDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	input := existingProviderInput(doc.blocks, "peer")
	if len(input.Models) != 2 || input.BaseURL != "" || input.UpstreamHeaderTimeout != "" {
		t.Fatal("concrete-to-derived edit failed to clear inherited fields and ordinary model declarations")
	}
	for _, model := range input.Models {
		if model.DeferredTransportBlock == "" || model.UpstreamName != "" || model.DisplayName != "" || model.Protocol != "" || len(model.Capabilities) != 0 || model.Pricing != nil {
			t.Fatal("concrete definitions became writable derivative model fields")
		}
	}
	before, _ := os.ReadFile(path)
	for _, flags := range [][]string{
		{"--model", "queued"}, {"--model-upstream", "queued=rejected"},
		{"--model-display-name", "queued=Rejected"}, {"--model-capabilities", "queued=chat"},
		{"--upstream-header-timeout", "60s"}, {"--enabled=false"},
	} {
		args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "peer"}
		if _, _, err := executeRootCommand("", append(args, flags...)...); err == nil || !strings.Contains(err.Error(), "inherited-field flags") {
			t.Fatalf("derived inherited-field flags lost their controlled rejection: %v", err)
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("unsupported derivative edit wrote the config")
		}
	}
}

func TestConfigureDeferredModelInvalidEditsDoNotWriteConfigOrCredentials(t *testing.T) {
	path := deferredModelConfigureFixture(t, "")
	before, _ := os.ReadFile(path)
	secrets := filepath.Join(t.TempDir(), "keys.json")
	if err := os.WriteFile(secrets, []byte(`{"peer":"unchanged-key"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeSecrets, _ := os.ReadFile(secrets)
	for _, flags := range [][]string{
		{"--model", "queued", "--model-capabilities", "queued=embeddings", "--api-key", "rejected-key", "--secrets-path", secrets, "--secrets-key", "peer"},
		{"--type", "openai"}, {"--base-url", "https://peer.example/v1?bad=true"},
		{"--forward-authorization"}, {"--enabled=false", "--api-key", "rejected-key"},
		{"--deferred-transport"}, {"--model-deferred-transport", "queued"},
	} {
		args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "peer"}
		if _, _, err := executeRootCommand("", append(args, flags...)...); err == nil {
			t.Fatal("invalid or unsupported model caller edit was accepted")
		}
		after, _ := os.ReadFile(path)
		afterSecrets, _ := os.ReadFile(secrets)
		if !bytes.Equal(before, after) || !bytes.Equal(beforeSecrets, afterSecrets) {
			t.Fatal("invalid nested caller edit wrote config or staged credentials")
		}
	}
	missing := `provider "openai-compatible" "incomplete" {
  base_url = "https://peer.example/v1"
  api_key = "base-key"
  model "ordinary" {}
}
`
	if err := os.WriteFile(path, append(before, []byte(missing)...), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ = os.ReadFile(path)
	if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "peer", "--extends", "incomplete"); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("conversion silently dropped a caller overlay absent from the base: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("invalid derived overlay conversion wrote the config")
	}
}
