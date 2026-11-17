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

const rawConfigureReasoningBlock = `encrypted_reasoning { # block comment
  passthrough = env("CLI_ER_PASS") == "true" # passthrough comment
  on_caller_mismatch = env("CLI_ER_MISMATCH") # mismatch comment
  match_messages = [env("CLI_ER_PATTERN")] # pattern comment
}`

func rawReasoningConfigureFixture(t *testing.T) string {
	t.Helper()
	t.Setenv("CLI_ER_TIMEOUT", "45s")
	source := `upstream_header_timeout = env("CLI_ER_TIMEOUT")
` + reasoningConfigureHeader + rawConfigureReasoningBlock + `
provider "openai" "primary" {
  api_key = env("CLI_REASONING_KEY")
  ` + rawConfigureReasoningBlock + `
  model "m" {
    upstream_name = "upstream/m"
    ` + rawConfigureReasoningBlock + `
  }
  model "n" { upstream_name = "upstream/n" }
}
alias "a" {
  algorithm = "round_robin"
  ` + rawConfigureReasoningBlock + `
  target {
    provider = "primary"
    model = "m"
  }
}
`
	return reasoningConfigureFixture(t, source)
}

func assertRawConfigureReasoning(t *testing.T, path string) {
	t.Helper()
	source := reasoningConfigureSource(t, path)
	if strings.Count(source, rawConfigureReasoningBlock) != 4 || !strings.Contains(source, `upstream_header_timeout = env("CLI_ER_TIMEOUT")`) {
		t.Fatal("unrelated edit changed reasoning comments/expressions or the root timeout expression")
	}
	for _, scope := range reasoningConfigureScopes() {
		input, err := configedit.ReadEncryptedReasoning(source, scope.scope)
		if err != nil || input == nil || input.Block != rawConfigureReasoningBlock {
			t.Fatalf("%s raw local declaration changed: %+v, %v", scope.name, input, err)
		}
	}
}

func TestConfigureEncryptedReasoningUnrelatedEditsPreserveRawPolicies(t *testing.T) {
	path := rawReasoningConfigureFixture(t)
	for _, args := range [][]string{
		{"upstream", "--user-agent", "Edited/1.0"},
		{"provider", "--name", "primary", "--display-name", "Edited"},
		{"alias", "--name", "a", "--algorithm", "least_connections"},
		{"provider", "--name", "primary", "--model", "m=updated/m", "--model", "n=upstream/n"},
		{"upstream", "--forward-user-agent=false", "--forward-headers", "x-example"},
	} {
		runReasoningConfigure(t, path, args...)
		assertRawConfigureReasoning(t, path)
	}
	for _, scope := range reasoningConfigureScopes() {
		before := reasoningConfigureSource(t, path)
		runReasoningConfigure(t, path, scope.args...)
		if reasoningConfigureSource(t, path) != before {
			t.Fatalf("%s no-op reconstructed raw policies", scope.name)
		}
		if scope.scope.Model == "" {
			args := append(append([]string{}, scope.args...), "--no-encrypted-reasoning=false", "--reset-encrypted-reasoning=false")
			runReasoningConfigure(t, path, args...)
			if reasoningConfigureSource(t, path) != before {
				t.Fatalf("%s false removal/reset controls changed the source", scope.name)
			}
		}
	}
}

func TestConfigureEncryptedReasoningOneMemberEditsPreserveOtherExpressions(t *testing.T) {
	for _, scope := range reasoningConfigureScopes() {
		for _, member := range []struct {
			name        string
			value       string
			before      string
			replacement string
		}{
			{"passthrough", "false", `env("CLI_ER_PASS") == "true"`, "false"},
			{"on-caller-mismatch", "", `env("CLI_ER_MISMATCH")`, `""`},
			{"match-message", "", `[env("CLI_ER_PATTERN")]`, "[]"},
		} {
			t.Run(scope.name+"/"+member.name, func(t *testing.T) {
				path := rawReasoningConfigureFixture(t)
				flags := reasoningScopeFlag(scope, member.name, member.value)
				runReasoningConfigure(t, path, append(append([]string{}, scope.args...), flags...)...)
				source := reasoningConfigureSource(t, path)
				input, err := configedit.ReadEncryptedReasoning(source, scope.scope)
				want := strings.Replace(rawConfigureReasoningBlock, member.before, member.replacement, 1)
				if err != nil || input == nil || input.Block != want {
					t.Fatalf("single member edit changed other raw members/comments: %v\n%s", err, source)
				}
				if strings.Count(source, rawConfigureReasoningBlock) != 3 {
					t.Fatal("a member edit changed another scope's raw block")
				}
			})
		}
	}
}

func TestConfigureEncryptedReasoningInteractiveMemberIntent(t *testing.T) {
	raw, err := configedit.ParseEncryptedReasoningBlock(rawConfigureReasoningBlock)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		existing *configedit.EncryptedReasoningInput
		answers  []string
		wantNil  bool
		wantRaw  bool
		wantPass *bool
		reset    bool
	}{
		{"inherit-preserve", nil, []string{"preserve"}, true, false, nil, false},
		{"override-with-unchanged-inputs", nil, []string{"override", "preserve", "preserve", "preserve"}, true, false, nil, false},
		{"raw-preserve", raw, []string{"preserve"}, false, true, nil, false},
		{"raw-unchanged-override", raw, []string{"override", "preserve", "preserve", "preserve"}, false, true, nil, false},
		{"raw-one-member-override", raw, []string{"override", "false", "preserve", "preserve"}, false, false, boolPtr(false), false},
		{"inherit", raw, []string{"inherit"}, true, false, nil, false},
		{"reset", raw, []string{"reset"}, false, false, nil, true},
		{"new-reset", nil, []string{"reset"}, false, false, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			prompts := newPromptSession(strings.NewReader(strings.Join(test.answers, "\n")+"\n"), &bytes.Buffer{})
			input, err := promptEncryptedReasoningInput(&prompts, "Policy", test.existing, encryptedReasoningOptions{}, false)
			if err != nil {
				t.Fatal(err)
			}
			if (input == nil) != test.wantNil {
				t.Fatal("unchanged/inherit inputs materialized a local block or reset removed it")
			}
			if test.wantRaw && input.Block != rawConfigureReasoningBlock {
				t.Fatal("unchanged prompts lost raw source")
			}
			if test.wantPass != nil && (input.Passthrough == nil || *input.Passthrough != *test.wantPass ||
				!strings.Contains(input.Block, `on_caller_mismatch = env("CLI_ER_MISMATCH")`) || !strings.Contains(input.Block, `[env("CLI_ER_PATTERN")]`)) {
				t.Fatal("interactive one-member edit lost untouched raw expressions")
			}
			if test.reset && (input.Passthrough != nil || input.HasOnCallerMismatch || input.HasMatchMessages || len(input.Expressions) != 0) {
				t.Fatal("interactive reset retained declared members")
			}
		})
	}
	if raw.Block != rawConfigureReasoningBlock {
		t.Fatal("prompts mutated original editor defaults")
	}
}

func reasoningProviderTextAnswers() []string {
	return []string{"update", "", "", "", "none", "default", "", "", "n", "", "env_expression", "", "y"}
}

func TestConfigureEncryptedReasoningInteractiveCobraWorkflows(t *testing.T) {
	for _, scope := range reasoningConfigureScopes() {
		for _, action := range []string{"preserve", "inherit", "reset", "override"} {
			t.Run(scope.name+"/"+action, func(t *testing.T) {
				path := rawReasoningConfigureFixture(t)
				var answers []string
				switch scope.name {
				case "root":
					answers = []string{"", "", "n", ""}
				case "provider":
					answers = reasoningProviderTextAnswers()
				case "model":
					answers = append(reasoningProviderTextAnswers(), "preserve", "yes", "m")
				case "alias":
					answers = []string{"update", "", "", "y", "n"}
				}
				answers = append(answers, action)
				if action == "override" {
					answers = append(answers, "false", "preserve", "preserve")
				}
				if scope.name == "provider" || scope.name == "model" {
					answers = append(answers, "no")
				}
				args := append([]string{"configure"}, scope.args...)
				args = append(args, "--config", path)
				stdout, stderr, err := executeRootCommand(strings.Join(answers, "\n")+"\n", args...)
				if err != nil {
					t.Fatalf("interactive %s: %v\n%s\n%s", scope.name, err, stdout, stderr)
				}
				rt, err := config.LoadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				local, err := configedit.ReadEncryptedReasoning(reasoningConfigureSource(t, path), scope.scope)
				if err != nil {
					t.Fatal(err)
				}
				switch action {
				case "preserve":
					if local == nil || local.Block != rawConfigureReasoningBlock {
						t.Fatal("interactive preserve changed the local block")
					}
				case "inherit":
					if local != nil {
						t.Fatal("interactive inherit retained a local block")
					}
				case "reset":
					if !reflect.DeepEqual(reasoningConfigureDeclaration(t, rt, scope.scope), &config.EncryptedReasoningDeclaration{}) {
						t.Fatal("interactive reset did not publish {}")
					}
				case "override":
					want := strings.Replace(rawConfigureReasoningBlock, `env("CLI_ER_PASS") == "true"`, "false", 1)
					if local == nil || local.Block != want {
						t.Fatal("interactive override changed unedited raw members")
					}
				}
			})
		}
	}
}

func TestConfigureEncryptedReasoningModelTUIAndTextReconstructionPreserveRawInput(t *testing.T) {
	path := rawReasoningConfigureFixture(t)
	doc, err := loadConfigDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	existing := existingProviderInput(doc.blocks, "primary")
	for _, test := range []struct {
		name    string
		prompts *promptSession
	}{
		{"text", func() *promptSession {
			p := newPromptSession(strings.NewReader("n\nm\nEdited\nupstream/m\n\nn\n"), &bytes.Buffer{})
			return &p
		}()},
		{"tui", deferredModelTUIPrompts(t, []string{"2", "1", "m", "Edited", "", "0", "4"})},
	} {
		t.Run(test.name, func(t *testing.T) {
			models, err := promptProviderModels(test.prompts, existing.ProviderType, existing.Models)
			if err != nil {
				t.Fatal(err)
			}
			model := findProviderModel(models, "m")
			if model == nil || model.EncryptedReasoning == nil || model.EncryptedReasoning.Block != rawConfigureReasoningBlock || model.DisplayName != "Edited" {
				t.Fatal("model reconstruction lost raw reasoning input")
			}
			if existing.Models[0].EncryptedReasoning.Block != rawConfigureReasoningBlock || existing.Models[0].DisplayName != "" {
				t.Fatal("model prompt mutated original defaults")
			}
			input := *existing
			input.Models = models
			updated, err := upsertBlock(doc.source, renderProviderBlock(input), func(block topLevelBlock) bool {
				return block.Type == "provider" && len(block.Labels) == 2 && block.Labels[1] == input.Name
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := config.Load([]byte(updated), path); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConfigureEncryptedReasoningInvalidEnvironmentEditsLeaveConfigAndSecretsUnchanged(t *testing.T) {
	path := rawReasoningConfigureFixture(t)
	before := reasoningConfigureSource(t, path)
	secretsPath := filepath.Join(t.TempDir(), "keys.json")
	secrets := []byte(`{"primary":"unchanged-key"}`)
	if err := os.WriteFile(secretsPath, secrets, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLI_ER_MISMATCH", "invalid")
	for _, args := range [][]string{
		{"upstream", "--encrypted-reasoning-passthrough=false"},
		{"upstream", "--user-agent", "Unrelated/1.0"},
		{"provider", "--name", "primary", "--display-name", "Unrelated", "--api-key", "staged-key", "--secrets-path", secretsPath, "--secrets-key", "primary"},
		{"alias", "--name", "a", "--algorithm", "least_connections"},
	} {
		command := append([]string{"configure"}, args...)
		command = append(command, "--config", path, "--non-interactive")
		if _, _, err := executeRootCommand("", command...); err == nil {
			t.Fatalf("invalid environment-backed policy was published: %v", args)
		}
		afterSecrets, err := os.ReadFile(secretsPath)
		if err != nil || !bytes.Equal(afterSecrets, secrets) || reasoningConfigureSource(t, path) != before {
			t.Fatalf("invalid config write changed files: %v", args)
		}
	}
}
