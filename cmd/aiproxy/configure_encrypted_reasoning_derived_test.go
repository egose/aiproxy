package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/configedit"
)

func derivedReasoningConfigureFixture(t *testing.T) string {
	t.Helper()
	t.Setenv("CLI_ER_BASE", "peer")
	t.Setenv("CLI_ER_PASS", "true")
	t.Setenv("CLI_ER_MISMATCH", "strip_and_retry")
	t.Setenv("CLI_ER_PATTERN", "Raw Marker")
	t.Setenv("CLI_REASONING_KEY", "synthetic-child-key")
	child := `provider "openai-compatible" "child" {
  extends = env("CLI_ER_BASE")
  api_key = "child-key"
  ` + rawConfigureReasoningBlock + `
  model "queued" {
    ` + deferredConfigureModelBlock + `
  }
  model "empty" {
    encrypted_reasoning {}
  }
}
`
	path := deferredModelConfigureFixture(t, child)
	source := reasoningConfigureSource(t, path)
	mismatch := "strip_and_retry"
	patterns := []string{"Base Marker"}
	for _, test := range []struct {
		scope configedit.EncryptedReasoningScope
		edit  configedit.EncryptedReasoningEdit
	}{
		{configedit.EncryptedReasoningScope{Provider: "peer"}, configedit.EncryptedReasoningEdit{Passthrough: boolPtr(false)}},
		{configedit.EncryptedReasoningScope{Provider: "peer", Model: "queued"}, configedit.EncryptedReasoningEdit{OnCallerMismatch: &mismatch}},
		{configedit.EncryptedReasoningScope{Provider: "peer", Model: "ordinary"}, configedit.EncryptedReasoningEdit{MatchMessages: &patterns}},
		{configedit.EncryptedReasoningScope{Provider: "peer", Model: "empty"}, configedit.EncryptedReasoningEdit{Passthrough: boolPtr(false)}},
	} {
		var err error
		source, err = configedit.UpdateEncryptedReasoning(source, test.scope, test.edit)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := writeConfigFile(path, source); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertSparseReasoningProvider(t *testing.T, path, name string) *providerInput {
	t.Helper()
	doc, err := loadConfigDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	input := existingProviderInput(doc.blocks, name)
	if input == nil || input.Extends == "" {
		t.Fatalf("derived provider %s disappeared", name)
	}
	if input.BaseURL != "" || input.UpstreamHeaderTimeout != "" || input.Enabled != nil || input.HasTLSClientHello || input.HasTLSClientHelloSpecFile {
		t.Fatal("derived policy edit flattened inherited endpoint or provider fields")
	}
	for _, model := range input.Models {
		if model.UpstreamName != "" || model.DisplayName != "" || model.Protocol != "" || len(model.Capabilities) != 0 || model.Pricing != nil ||
			model.EncryptedReasoning == nil && model.DeferredTransportBlock == "" {
			t.Fatal("derived policy edit flattened a model definition or retained a bare overlay")
		}
	}
	return input
}

func TestConfigureEncryptedReasoningDerivedSparseOverlaysAndDeferredSiblings(t *testing.T) {
	path := derivedReasoningConfigureFixture(t)
	before := reasoningConfigureSource(t, path)
	rt := runReasoningConfigure(t, path, "provider", "--name", "child",
		"--model-reasoning-passthrough", "ordinary=false",
		"--model-reasoning-on-caller-mismatch", "ordinary=",
		"--model-reasoning-match-message", "ordinary=",
		"--reset-model-reasoning", "queued")
	input := assertSparseReasoningProvider(t, path, "child")
	if input.Extends != `env("CLI_ER_BASE")` || input.Credential.APIKeyValue != "child-key" || !strings.Contains(reasoningConfigureSource(t, path), deferredConfigureModelBlock) {
		t.Fatal("policy-only overlay creation rewrote the raw base expression, credential, or deferred sibling")
	}
	p, _ := rt.Catalog.Provider("child")
	for _, name := range []string{"ordinary", "queued", "empty"} {
		if p.ModelByName[name].UpstreamName != "executor/"+name || p.ModelByName[name].EncryptedReasoningInherited {
			t.Fatal("local model overlays redefined inherited models or lost local origin")
		}
	}
	ordinary := config.DeclaredEncryptedReasoning(p.ModelByName["ordinary"].EncryptedReasoning)
	if ordinary.Passthrough == nil || *ordinary.Passthrough || ordinary.OnCallerMismatch == nil || *ordinary.OnCallerMismatch != "" || ordinary.MatchMessages == nil || len(*ordinary.MatchMessages) != 0 {
		t.Fatal("derived overlay lost explicit false/empty members")
	}
	if p.ModelByName["queued"].DeferredTransport == nil || p.ModelByName["empty"].DeferredTransport != nil {
		t.Fatal("reasoning overlay changed deferred eligibility or inherited a base caller opt-in")
	}
	doc, err := loadConfigDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	base := findBlock(doc.blocks, func(block topLevelBlock) bool { return block.Type == "provider" && block.Labels[1] == "peer" })
	if base == nil || !strings.Contains(before, base.Text) || !strings.Contains(reasoningConfigureSource(t, path), deferredConfigureRoot) ||
		!strings.Contains(reasoningConfigureSource(t, path), deferredConfigureModelReceiver) {
		t.Fatal("targeted overlay edits reconstructed the base, root, or receiver")
	}
	rt = runReasoningConfigure(t, path, "provider", "--name", "child", "--no-model-reasoning", "queued", "--no-model-reasoning", "empty", "--no-model-reasoning", "ordinary")
	input = assertSparseReasoningProvider(t, path, "child")
	if len(input.Models) != 1 || input.Models[0].Name != "queued" || input.Models[0].EncryptedReasoning != nil || input.Models[0].DeferredTransportBlock != deferredConfigureModelBlock {
		t.Fatal("last-overlay cleanup removed a deferred sibling or retained bare reasoning-only model blocks")
	}
	p, _ = rt.Catalog.Provider("child")
	if !p.ModelByName["queued"].EncryptedReasoningInherited || !p.ModelByName["ordinary"].EncryptedReasoningInherited || !p.ModelByName["empty"].EncryptedReasoningInherited ||
		p.ModelByName["empty"].EncryptedReasoning.Passthrough || p.ModelByName["queued"].DeferredTransport == nil {
		t.Fatal("overlay removal did not restore base model declarations")
	}
	rt = runReasoningConfigure(t, path, "provider", "--name", "child", "--reset-encrypted-reasoning")
	p, _ = rt.Catalog.Provider("child")
	if p.EncryptedReasoningInherited || !reflect.DeepEqual(config.DeclaredEncryptedReasoning(p.EncryptedReasoning), &config.EncryptedReasoningDeclaration{}) {
		t.Fatal("child provider reset did not remain a local {} block")
	}
	resolution := config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["queued"], nil)
	if resolution.Source != config.EncryptedReasoningSourceModel || !resolution.Inherited || resolution.Policy.OnCallerMismatch != config.EncryptedReasoningStripAndRetry {
		t.Fatal("provider reset incorrectly reset an inherited model declaration")
	}
	rt = runReasoningConfigure(t, path, "provider", "--name", "child", "--no-encrypted-reasoning")
	p, _ = rt.Catalog.Provider("child")
	if !p.EncryptedReasoningInherited || p.EncryptedReasoning.Passthrough {
		t.Fatal("child provider removal did not restore the base provider block")
	}
}

func TestConfigureEncryptedReasoningDerivedUnrelatedAndPromptPreservation(t *testing.T) {
	path := derivedReasoningConfigureFixture(t)
	for _, flags := range [][]string{
		{"--display-name", "Edited child"},
		{"--api-key-env", "CLI_REASONING_KEY"},
		{"--display-name", "Another edit", "--model-reasoning-on-caller-mismatch", "queued=fail"},
	} {
		runReasoningConfigure(t, path, append([]string{"provider", "--name", "child"}, flags...)...)
		input := assertSparseReasoningProvider(t, path, "child")
		if input.EncryptedReasoning == nil || input.EncryptedReasoning.Block != rawConfigureReasoningBlock || input.Extends != `env("CLI_ER_BASE")` ||
			findProviderModel(input.Models, "empty").EncryptedReasoning == nil || findProviderModel(input.Models, "queued").DeferredTransportBlock != deferredConfigureModelBlock {
			t.Fatal("unrelated provider update lost local policies/deferred overlays or quoted the base expression")
		}
	}
	doc, err := loadConfigDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	existing := existingProviderInput(doc.blocks, "child")
	prompts := deferredModelTUIPrompts(t, []string{"2", "child", "TUI child", "3", "2", `env("CLI_REASONING_KEY")`})
	input, _, err := promptProviderInput(prompts, existing, providerOptions{BaseProviderNames: []string{"peer"}, BaseProviderTypes: map[string]string{"peer": "openai-compatible"}})
	if err != nil {
		t.Fatal(err)
	}
	if input.EncryptedReasoning == nil || input.EncryptedReasoning.Block != rawConfigureReasoningBlock || input.Extends != `env("CLI_ER_BASE")` ||
		len(input.Models) != 2 || findProviderModel(input.Models, "empty").EncryptedReasoning == nil || findProviderModel(input.Models, "queued").EncryptedReasoning == nil {
		t.Fatal("TUI provider reconstruction lost raw provider reasoning or local model policies")
	}
	updated, err := upsertBlock(doc.source, renderProviderBlock(input), func(block topLevelBlock) bool {
		return block.Type == "provider" && len(block.Labels) == 2 && block.Labels[1] == "child"
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load([]byte(updated), path); err != nil {
		t.Fatal(err)
	}
}

func TestConfigureEncryptedReasoningDerivedCreationAndGuardrails(t *testing.T) {
	path := derivedReasoningConfigureFixture(t)
	rt := runReasoningConfigure(t, path, "provider", "--name", "created_child", "--type", "openai-compatible", "--extends", "peer", "--api-key-env", "CLI_REASONING_KEY",
		"--encrypted-reasoning-passthrough=false", "--model-reasoning-passthrough", "queued=false", "--reset-model-reasoning", "ordinary")
	input := assertSparseReasoningProvider(t, path, "created_child")
	if len(input.Models) != 2 || input.Credential.APIKeyValue != `env("CLI_REASONING_KEY")` {
		t.Fatal("derived creation lost sparse overlays or its isolated local credential")
	}
	p, _ := rt.Catalog.Provider("created_child")
	if p.APIKey != "synthetic-child-key" || p.ModelByName["queued"].DeferredTransport != nil || p.ModelByName["queued"].EncryptedReasoningInherited ||
		!reflect.DeepEqual(config.DeclaredEncryptedReasoning(p.ModelByName["ordinary"].EncryptedReasoning), &config.EncryptedReasoningDeclaration{}) {
		t.Fatal("derived reasoning creation inherited credentials/deferred opt-ins or flattened local intent")
	}
	secretsPath := filepath.Join(t.TempDir(), "keys.json")
	secrets := []byte(`{"child":"unchanged-key"}`)
	if err := os.WriteFile(secretsPath, secrets, 0o600); err != nil {
		t.Fatal(err)
	}
	before := reasoningConfigureSource(t, path)
	cases := [][]string{
		{"--model-reasoning-passthrough", "invented=false"},
		{"--reset-model-reasoning", "invented"},
		{"--no-model-reasoning", "invented"},
		{"--model", "ordinary", "--reset-model-reasoning", "ordinary"},
		{"--model-upstream", "ordinary=redefined", "--reset-model-reasoning", "ordinary"},
		{"--model-display-name", "ordinary=Redefined", "--reset-model-reasoning", "ordinary"},
		{"--model-capabilities", "ordinary=chat", "--reset-model-reasoning", "ordinary"},
		{"--type", "anthropic", "--reset-model-reasoning", "ordinary"},
		{"--base-url", "https://redefined.example/v1", "--reset-model-reasoning", "ordinary"},
		{"--forward-authorization", "--reset-model-reasoning", "ordinary"},
	}
	for _, flags := range cases {
		args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "child",
			"--api-key", "staged-key", "--secrets-path", secretsPath, "--secrets-key", "child"}
		args = append(args, flags...)
		if _, _, err := executeRootCommand("", args...); err == nil {
			t.Fatalf("derived guardrail was bypassed: %v", flags)
		}
		afterSecrets, err := os.ReadFile(secretsPath)
		if err != nil || !bytes.Equal(afterSecrets, secrets) || reasoningConfigureSource(t, path) != before {
			t.Fatalf("rejected derivative edit published files: %v", flags)
		}
	}
	for _, flags := range [][]string{
		{"--name", "uncredentialed", "--type", "openai-compatible", "--extends", "peer", "--reset-model-reasoning", "ordinary"},
		{"--name", "chain", "--type", "openai-compatible", "--extends", "child", "--api-key", "local-key", "--reset-model-reasoning", "ordinary"},
	} {
		args := append([]string{"configure", "provider", "--config", path, "--non-interactive"}, flags...)
		if _, _, err := executeRootCommand("", args...); err == nil || reasoningConfigureSource(t, path) != before {
			t.Fatal("reasoning flags bypassed derivative credential isolation or no-chain rules")
		}
	}
	t.Setenv("CLI_ER_BASE", "missing")
	if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "child", "--no-model-reasoning", "empty"); err == nil ||
		reasoningConfigureSource(t, path) != before {
		t.Fatal("last-overlay cleanup guessed an invalid environment-backed base")
	}
}

func TestConfigureEncryptedReasoningDerivedOpenCodeOverlayPreservation(t *testing.T) {
	source := reasoningConfigureHeader + `provider "opencode-zen" "zen" {
  model "m" { protocol = "responses" }
}
provider "opencode-zen" "child" {
  extends = "zen"
  model "m" {
    encrypted_reasoning {}
  }
}
`
	path := reasoningConfigureFixture(t, source)
	rt := runReasoningConfigure(t, path, "provider", "--name", "child", "--display-name", "Edited")
	p, _ := rt.Catalog.Provider("child")
	if p.ModelByName["m"].Protocol != config.ModelProtocolResponses || p.ModelByName["m"].EncryptedReasoningInherited ||
		!reflect.DeepEqual(config.DeclaredEncryptedReasoning(p.ModelByName["m"].EncryptedReasoning), &config.EncryptedReasoningDeclaration{}) {
		t.Fatal("an unrelated OpenCode derivative edit demanded a local protocol or flattened its inherited model")
	}
	assertSparseReasoningProvider(t, path, "child")
}

func TestConfigureEncryptedReasoningDerivedPreserveDoesNotMaterializeInheritedPolicies(t *testing.T) {
	path := reasoningConfigureFixture(t, reasoningConfigureHeader+reasoningConfigureProvider+`provider "openai" "child" {
  extends = "primary"
  api_key = "child-key"
}
`)
	runReasoningConfigure(t, path, "provider", "--name", "primary", "--encrypted-reasoning-passthrough=false", "--reset-model-reasoning", "m")
	before := reasoningConfigureSource(t, path)
	runReasoningConfigure(t, path, "provider", "--name", "child")
	if reasoningConfigureSource(t, path) != before {
		t.Fatal("non-interactive preserve materialized inherited policy or model defaults")
	}
	runReasoningConfigure(t, path, "provider", "--name", "child", "--display-name", "Unrelated")
	input := assertSparseReasoningProvider(t, path, "child")
	if input.EncryptedReasoning != nil || len(input.Models) != 0 {
		t.Fatal("unrelated child edit wrote inherited declarations locally")
	}
	answers := []string{"update", "", "", "", "", "inline", "", "preserve", "yes", "m", "preserve", "no"}
	stdout, stderr, err := executeRootCommand(strings.Join(answers, "\n")+"\n", "configure", "provider", "--config", path, "--name", "child")
	if err != nil {
		t.Fatalf("interactive inherited preserve: %v\n%s\n%s", err, stdout, stderr)
	}
	input = assertSparseReasoningProvider(t, path, "child")
	if input.EncryptedReasoning != nil || len(input.Models) != 0 {
		t.Fatal("interactive preserve created an inherited/default provider or model overlay")
	}
	rt, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := rt.Catalog.Provider("child")
	if !p.EncryptedReasoningInherited || !p.ModelByName["m"].EncryptedReasoningInherited {
		t.Fatal("preserve lost inherited policy origin")
	}
}
