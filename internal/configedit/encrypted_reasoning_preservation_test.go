package configedit

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

const encryptedReasoningEditRawBlock = `encrypted_reasoning { # keep block comment
  # keep passthrough comment
  passthrough = env("EDIT_ER_PASS") == "allow" # keep boolean comment
  on_caller_mismatch = env("EDIT_ER_MODE") # keep mismatch comment
  match_messages = [
    env("EDIT_ER_PATTERN"),
    "Other Marker",
  ] # keep messages comment
}`

func encryptedReasoningEditEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("EDIT_ER_PASS", "allow")
	t.Setenv("EDIT_ER_MODE", "strip_and_retry")
	t.Setenv("EDIT_ER_PATTERN", "  Env Caller Marker  ")
}

func TestEncryptedReasoningRawParseNeverCoercesUnevaluableExpressions(t *testing.T) {
	input, err := ParseEncryptedReasoningBlock(encryptedReasoningEditRawBlock)
	if err != nil {
		t.Fatal(err)
	}
	if input.Passthrough != nil || input.OnCallerMismatch != "" || input.MatchMessages != nil || !input.HasOnCallerMismatch || !input.HasMatchMessages {
		t.Fatal("unevaluable expressions were treated as literal defaults")
	}
	for name, expression := range map[string]string{
		"passthrough":        `env("EDIT_ER_PASS") == "allow"`,
		"on_caller_mismatch": `env("EDIT_ER_MODE")`,
		"match_messages":     "[\n    env(\"EDIT_ER_PATTERN\"),\n    \"Other Marker\",\n  ]",
	} {
		if input.Expressions[name] != expression {
			t.Fatalf("%s raw expression changed: %q", name, input.Expressions[name])
		}
	}
	if RenderEncryptedReasoningBlock(input) != encryptedReasoningEditRawBlock+"\n" {
		t.Fatal("raw block reconstruction changed source/comments")
	}
	clone, err := ApplyEncryptedReasoningEdit(input, EncryptedReasoningEdit{})
	if err != nil || RenderEncryptedReasoningBlock(clone) != encryptedReasoningEditRawBlock+"\n" {
		t.Fatalf("unevaluable no-op changed source: %v", err)
	}
	withoutBlock := CloneEncryptedReasoningInput(input)
	withoutBlock.Block = ""
	rendered := RenderEncryptedReasoningBlock(withoutBlock)
	for _, expression := range input.Expressions {
		if !strings.Contains(rendered, expression) {
			t.Fatal("raw-expression-only rendering coerced or dropped an expression")
		}
	}
	encryptedReasoningEditEnvironment(t)
	for _, block := range []string{RenderEncryptedReasoningBlock(input), RenderEncryptedReasoningBlock(clone), rendered} {
		encryptedReasoningEditedPolicy(t, encryptedReasoningEditDocument("root", block), EncryptedReasoningScope{})
	}
}

func TestEncryptedReasoningRawMemberUpdatesPreserveOtherExpressionsAndComments(t *testing.T) {
	encryptedReasoningEditEnvironment(t)
	empty, emptyMessages := "", []string{}
	for _, scope := range encryptedReasoningEditScopes {
		for _, tc := range []struct {
			name    string
			changed string
			edit    EncryptedReasoningEdit
		}{
			{"false", "passthrough", EncryptedReasoningEdit{Passthrough: boolPtr(false)}},
			{"empty-mismatch", "on_caller_mismatch", EncryptedReasoningEdit{OnCallerMismatch: &empty}},
			{"empty-messages", "match_messages", EncryptedReasoningEdit{MatchMessages: &emptyMessages}},
			{"unset-member", "passthrough", EncryptedReasoningEdit{Expressions: map[string]*string{"passthrough": nil}}},
		} {
			t.Run(scope.name+"/"+tc.name, func(t *testing.T) {
				source := encryptedReasoningEditDocument(scope.name, encryptedReasoningEditRawBlock)
				before, err := ReadEncryptedReasoning(source, scope.scope)
				if err != nil {
					t.Fatal(err)
				}
				updated, err := UpdateEncryptedReasoning(source, scope.scope, tc.edit)
				if err != nil {
					t.Fatal(err)
				}
				after, err := ReadEncryptedReasoning(updated, scope.scope)
				if err != nil {
					t.Fatal(err)
				}
				for name, expression := range before.Expressions {
					if name != tc.changed && after.Expressions[name] != expression {
						t.Fatalf("untouched %s expression changed", name)
					}
				}
				for _, comment := range []string{"# keep block comment", "# keep passthrough comment", "# keep boolean comment", "# keep mismatch comment", "# keep messages comment"} {
					if !strings.Contains(updated, comment) {
						t.Fatalf("comment was lost: %s", comment)
					}
				}
				declaration := config.DeclaredEncryptedReasoning(encryptedReasoningEditedPolicy(t, updated, scope.scope))
				switch tc.name {
				case "false":
					if declaration.Passthrough == nil || *declaration.Passthrough {
						t.Fatal("explicit false was lost")
					}
				case "empty-mismatch":
					if declaration.OnCallerMismatch == nil || *declaration.OnCallerMismatch != "" {
						t.Fatal("explicit empty string was lost")
					}
				case "empty-messages":
					if declaration.MatchMessages == nil || len(*declaration.MatchMessages) != 0 {
						t.Fatal("explicit empty list was lost")
					}
				case "unset-member":
					if declaration.Passthrough != nil {
						t.Fatal("member removal was converted to explicit true")
					}
				}
				noOp, err := UpdateEncryptedReasoning(updated, scope.scope, EncryptedReasoningEdit{})
				if err != nil || noOp != updated {
					t.Fatalf("partial edit no-op changed source: %v", err)
				}
			})
		}
	}
}

func TestEncryptedReasoningRawNoOpResetAndUnrelatedEdits(t *testing.T) {
	encryptedReasoningEditEnvironment(t)
	for _, scope := range encryptedReasoningEditScopes {
		t.Run(scope.name, func(t *testing.T) {
			source := encryptedReasoningEditDocument(scope.name, encryptedReasoningEditRawBlock)
			noOp, err := UpdateEncryptedReasoning(source, scope.scope, EncryptedReasoningEdit{})
			if err != nil || noOp != source {
				t.Fatalf("raw no-op changed source: %v", err)
			}
			before, err := ReadEncryptedReasoning(source, scope.scope)
			if err != nil {
				t.Fatal(err)
			}
			expression := before.Expressions["on_caller_mismatch"]
			same, err := UpdateEncryptedReasoning(source, scope.scope, EncryptedReasoningEdit{Expressions: map[string]*string{"on_caller_mismatch": &expression}})
			if err != nil || same != source {
				t.Fatalf("identical raw expression changed source: %v", err)
			}
			unrelated, err := UpsertTopLevelStringAttribute(source, "upstream_header_timeout", "120s")
			if err != nil {
				t.Fatal(err)
			}
			after, err := ReadEncryptedReasoning(unrelated, scope.scope)
			if err != nil || before.Block != after.Block {
				t.Fatalf("unrelated root edit changed local policy source: %v", err)
			}
			encryptedReasoningEditedPolicy(t, unrelated, scope.scope)
			reset, err := UpdateEncryptedReasoning(source, scope.scope, EncryptedReasoningEdit{Reset: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, comment := range []string{"# keep block comment", "# keep passthrough comment", "# keep boolean comment", "# keep mismatch comment", "# keep messages comment"} {
				if !strings.Contains(reset, comment) {
					t.Fatalf("reset lost a comment: %s", comment)
				}
			}
			if !reflect.DeepEqual(config.DeclaredEncryptedReasoning(encryptedReasoningEditedPolicy(t, reset, scope.scope)), &config.EncryptedReasoningDeclaration{}) {
				t.Fatal("reset did not retain an empty local declaration")
			}
			removed, err := UpdateEncryptedReasoning(source, scope.scope, EncryptedReasoningEdit{Remove: true})
			if err != nil || encryptedReasoningEditedPolicy(t, removed, scope.scope) != nil {
				t.Fatalf("raw block removal did not restore inheritance: %v", err)
			}
		})
	}
}

func TestEncryptedReasoningRawProviderAndAliasReconstruction(t *testing.T) {
	encryptedReasoningEditEnvironment(t)
	providerSource := encryptedReasoningEditDocument("provider", encryptedReasoningEditRawBlock)
	providerSource, err := UpdateEncryptedReasoning(providerSource, EncryptedReasoningScope{Provider: "base", Model: "m"}, EncryptedReasoningEdit{
		Expressions: map[string]*string{
			"passthrough":        stringPtr(`env("EDIT_ER_PASS") == "allow"`),
			"on_caller_mismatch": stringPtr(`env("EDIT_ER_MODE")`),
			"match_messages":     stringPtr(`[env("EDIT_ER_PATTERN")]`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	providerPolicy, err := ReadEncryptedReasoning(providerSource, EncryptedReasoningScope{Provider: "base"})
	if err != nil {
		t.Fatal(err)
	}
	modelPolicy, err := ReadEncryptedReasoning(providerSource, EncryptedReasoningScope{Provider: "base", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	replacement := RenderProviderBlock(ProviderInput{
		ProviderType: "openai-compatible", Name: "base", DisplayName: "Unrelated provider change", BaseURL: "https://upstream.example/v1",
		Credential: ProviderCredentialInput{Mode: "inline", APIKeyValue: "base-key"}, EncryptedReasoning: providerPolicy,
		Models: []ProviderModelInput{{Name: "m", UpstreamName: "upstream/m", EncryptedReasoning: modelPolicy}},
	}, "")
	reconstructed, err := UpsertBlock(providerSource, replacement, func(block TopLevelBlock) bool { return block.Type == "provider" && block.Labels[1] == "base" })
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []EncryptedReasoningScope{{Provider: "base"}, {Provider: "base", Model: "m"}} {
		before, err := ReadEncryptedReasoning(providerSource, scope)
		if err != nil {
			t.Fatal(err)
		}
		after, err := ReadEncryptedReasoning(reconstructed, scope)
		if err != nil || before.Block != after.Block {
			t.Fatalf("provider/model reconstruction changed policy source: %v", err)
		}
		encryptedReasoningEditedPolicy(t, reconstructed, scope)
	}
	aliasSource := encryptedReasoningEditDocument("alias", encryptedReasoningEditRawBlock)
	aliasPolicy, err := ReadEncryptedReasoning(aliasSource, EncryptedReasoningScope{Alias: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	replacement = RenderAliasBlock(AliasInput{
		Name: "chat", Algorithm: "round_robin", RetryStatusCodes: []string{"500", "503"}, EncryptedReasoning: aliasPolicy,
		Targets: []AliasTargetInput{{Provider: "base", Model: "m"}},
	})
	reconstructed, err = UpsertBlock(aliasSource, replacement, func(block TopLevelBlock) bool { return block.Type == "alias" })
	if err != nil {
		t.Fatal(err)
	}
	after, err := ReadEncryptedReasoning(reconstructed, EncryptedReasoningScope{Alias: "chat"})
	if err != nil || after.Block != aliasPolicy.Block {
		t.Fatalf("alias reconstruction changed policy source: %v", err)
	}
	encryptedReasoningEditedPolicy(t, reconstructed, EncryptedReasoningScope{Alias: "chat"})
}

func TestEncryptedReasoningWritePreflightRejectsBeforePublishing(t *testing.T) {
	encryptedReasoningEditEnvironment(t)
	for _, scope := range encryptedReasoningEditScopes {
		t.Run(scope.name, func(t *testing.T) {
			dir := t.TempDir()
			path, secrets := filepath.Join(dir, "config.hcl"), filepath.Join(dir, "keys.json")
			source := encryptedReasoningEditDocument(scope.name, encryptedReasoningEditRawBlock)
			if err := WriteConfigFile(path, source); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(secrets, []byte(`{"old":"preserved"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			beforeSecrets, err := os.ReadFile(secrets)
			if err != nil {
				t.Fatal(err)
			}
			for _, block := range []string{
				`encrypted_reasoning { passthrough = null }`,
				`encrypted_reasoning { on_caller_mismatch = "retry" }`,
				`encrypted_reasoning { match_messages = ["same", "SAME"] }`,
				`encrypted_reasoning { match_messages = [null] }`,
				`encrypted_reasoning { passthrough = env("EDIT_ER_PASS") }`,
			} {
				candidate := encryptedReasoningEditDocument(scope.name, block)
				if err := WriteConfigFile(path, candidate); err == nil {
					t.Fatal("invalid policy passed config write preflight")
				}
				if err := WriteProviderFiles(path, candidate, SecretsUpdate{Path: secrets, Key: "new", Value: "rejected-key"}); err == nil {
					t.Fatal("invalid policy passed staged provider write preflight")
				}
				after, _ := os.ReadFile(path)
				afterSecrets, _ := os.ReadFile(secrets)
				if !bytes.Equal(before, after) || !bytes.Equal(beforeSecrets, afterSecrets) {
					t.Fatal("rejected policy wrote config or staged credentials")
				}
			}
			t.Setenv("EDIT_ER_MODE", "invalid-mode")
			if err := WriteProviderFiles(path, source, SecretsUpdate{Path: secrets, Key: "new", Value: "rejected-key"}); err == nil {
				t.Fatal("invalid env policy passed write preflight")
			}
			after, _ := os.ReadFile(path)
			afterSecrets, _ := os.ReadFile(secrets)
			if !bytes.Equal(before, after) || !bytes.Equal(beforeSecrets, afterSecrets) {
				t.Fatal("invalid env policy wrote config or credentials")
			}
		})
	}
}

func stringPtr(value string) *string {
	return &value
}
