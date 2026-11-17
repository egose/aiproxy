package configedit

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

const encryptedReasoningEditPreamble = `listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
`

var encryptedReasoningEditScopes = []struct {
	name  string
	scope EncryptedReasoningScope
}{
	{"root", EncryptedReasoningScope{}},
	{"provider", EncryptedReasoningScope{Provider: "base"}},
	{"model", EncryptedReasoningScope{Provider: "base", Model: "m"}},
	{"alias", EncryptedReasoningScope{Alias: "chat"}},
}

func encryptedReasoningEditDocument(scope, block string) string {
	blocks := map[string]string{scope: block}
	return encryptedReasoningEditPreamble + fmt.Sprintf(`%s
provider "openai-compatible" "base" {
  base_url = "https://upstream.example/v1"
  api_key = "base-key"
  %s
  model "m" {
    upstream_name = "upstream/m"
    %s
  }
}
alias "chat" {
  algorithm = "round_robin"
  %s
  target {
    provider = "base"
    model = "m"
  }
}
`, blocks["root"], indentEncryptedReasoningTestBlock(blocks["provider"], "  "), indentEncryptedReasoningTestBlock(blocks["model"], "    "), indentEncryptedReasoningTestBlock(blocks["alias"], "  "))
}

func indentEncryptedReasoningTestBlock(block, indent string) string {
	return strings.ReplaceAll(strings.TrimSuffix(block, "\n"), "\n", "\n"+indent)
}

func encryptedReasoningEditedPolicy(t *testing.T, source string, scope EncryptedReasoningScope) *config.EncryptedReasoning {
	t.Helper()
	rt, err := config.Load([]byte(source), "policy.hcl")
	if err != nil {
		t.Fatalf("load edited config: %v\n%s", err, source)
	}
	if scope.Alias != "" {
		alias, ok := rt.Catalog.Alias(scope.Alias)
		if !ok {
			t.Fatal("edited alias disappeared")
		}
		return alias.EncryptedReasoning
	}
	if scope.Provider != "" {
		provider, ok := rt.Catalog.Provider(scope.Provider)
		if !ok {
			t.Fatal("edited provider disappeared")
		}
		if scope.Model != "" {
			model, ok := provider.ModelByName[scope.Model]
			if !ok {
				t.Fatal("edited model disappeared")
			}
			return model.EncryptedReasoning
		}
		return provider.EncryptedReasoning
	}
	return rt.EncryptedReasoning
}

func TestEncryptedReasoningSharedInputDeclarationPresence(t *testing.T) {
	empty, emptyMessages := "", []string{}
	for _, tc := range []struct {
		name        string
		declaration *config.EncryptedReasoningDeclaration
	}{
		{"inherit", nil},
		{"reset", &config.EncryptedReasoningDeclaration{}},
		{"false", &config.EncryptedReasoningDeclaration{Passthrough: boolPtr(false)}},
		{"true", &config.EncryptedReasoningDeclaration{Passthrough: boolPtr(true)}},
		{"empty-mismatch", &config.EncryptedReasoningDeclaration{OnCallerMismatch: &empty}},
		{"empty-messages", &config.EncryptedReasoningDeclaration{MatchMessages: &emptyMessages}},
		{"optional-empty-members", &config.EncryptedReasoningDeclaration{Passthrough: boolPtr(false), OnCallerMismatch: &empty, MatchMessages: &emptyMessages}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := EncryptedReasoningInputFromDeclaration(tc.declaration)
			block := RenderEncryptedReasoningBlock(input)
			for _, scope := range encryptedReasoningEditScopes {
				t.Run(scope.name, func(t *testing.T) {
					source := encryptedReasoningEditDocument(scope.name, block)
					policy := encryptedReasoningEditedPolicy(t, source, scope.scope)
					if !reflect.DeepEqual(config.DeclaredEncryptedReasoning(policy), tc.declaration) {
						t.Fatalf("declared members changed: got %+v, want %+v", config.DeclaredEncryptedReasoning(policy), tc.declaration)
					}
					parsed, err := ReadEncryptedReasoning(source, scope.scope)
					if err != nil || (parsed == nil) != (input == nil) {
						t.Fatalf("local block presence changed: %v", err)
					}
					if parsed != nil {
						reconstructed := encryptedReasoningEditDocument(scope.name, RenderEncryptedReasoningBlock(parsed))
						if !reflect.DeepEqual(config.DeclaredEncryptedReasoning(encryptedReasoningEditedPolicy(t, reconstructed, scope.scope)), tc.declaration) {
							t.Fatal("parse/render lost explicit optional-empty members")
						}
					}
				})
			}
		})
	}
}

func TestRenderProviderAndAliasSharedEncryptedReasoningInputs(t *testing.T) {
	input := &EncryptedReasoningInput{Passthrough: boolPtr(false), HasOnCallerMismatch: true, HasMatchMessages: true}
	provider := RenderProviderBlock(ProviderInput{
		ProviderType: "openai-compatible", Name: "base", BaseURL: "https://upstream.example/v1",
		Credential:         ProviderCredentialInput{Mode: "inline", APIKeyValue: "base-key"},
		EncryptedReasoning: input,
		Models:             []ProviderModelInput{{Name: "m", EncryptedReasoning: input}},
	}, "")
	alias := RenderAliasBlock(AliasInput{
		Name: "chat", Algorithm: "round_robin", EncryptedReasoning: input,
		Targets: []AliasTargetInput{{Provider: "base", Model: "m"}},
	})
	source := encryptedReasoningEditPreamble + RenderEncryptedReasoningBlock(input) + provider + alias
	for _, scope := range encryptedReasoningEditScopes {
		policy := encryptedReasoningEditedPolicy(t, source, scope.scope)
		declaration := config.DeclaredEncryptedReasoning(policy)
		if declaration == nil || declaration.Passthrough == nil || *declaration.Passthrough || declaration.OnCallerMismatch == nil || *declaration.OnCallerMismatch != "" || declaration.MatchMessages == nil || len(*declaration.MatchMessages) != 0 {
			t.Fatalf("%s lost explicit members: %+v", scope.name, declaration)
		}
	}
	disabled := RenderProviderBlock(ProviderInput{
		ProviderType: "openai", Name: "disabled", Enabled: boolPtr(false), EncryptedReasoning: input,
		Models: []ProviderModelInput{{Name: "m"}},
	}, "")
	rt, err := config.Load([]byte(source+disabled), "policy.hcl")
	if err != nil {
		t.Fatal(err)
	}
	if got := rt.Catalog.DisabledProviders(); len(got) != 1 || got[0].EncryptedReasoning == nil || got[0].EncryptedReasoning.Passthrough || strings.Contains(disabled, "api_key") {
		t.Fatal("reasoning altered disabled credential behavior or disappeared")
	}
}

func TestEncryptedReasoningSharedInputClonesDeclarationAndRawData(t *testing.T) {
	mode, messages := "strip_and_retry", []string{"Caller Marker"}
	declaration := &config.EncryptedReasoningDeclaration{Passthrough: boolPtr(false), OnCallerMismatch: &mode, MatchMessages: &messages}
	input := EncryptedReasoningInputFromDeclaration(declaration)
	*declaration.Passthrough, mode, messages[0] = true, "fail", "Other Marker"
	if *input.Passthrough || input.OnCallerMismatch != "strip_and_retry" || input.MatchMessages[0] != "Caller Marker" {
		t.Fatal("input shared declaration pointers or slices")
	}
	input.Expressions = map[string]string{"passthrough": "false"}
	clone, err := ApplyEncryptedReasoningEdit(input, EncryptedReasoningEdit{})
	if err != nil {
		t.Fatal(err)
	}
	*clone.Passthrough = true
	clone.MatchMessages[0] = "Changed Marker"
	clone.Expressions["passthrough"] = "true"
	if *input.Passthrough || input.MatchMessages[0] != "Caller Marker" || input.Expressions["passthrough"] != "false" {
		t.Fatal("no-op input clone shared mutable values")
	}
	var nilMessages []string
	input = EncryptedReasoningInputFromDeclaration(&config.EncryptedReasoningDeclaration{MatchMessages: &nilMessages})
	policy := encryptedReasoningEditedPolicy(t, encryptedReasoningEditDocument("root", RenderEncryptedReasoningBlock(input)), EncryptedReasoningScope{})
	if !input.HasMatchMessages || config.DeclaredEncryptedReasoning(policy).MatchMessages == nil {
		t.Fatal("a declared nil slice was omitted instead of rendered as an explicit empty list")
	}
}

func TestEncryptedReasoningUpdateCreateResetRemoveAllScopes(t *testing.T) {
	for _, scope := range encryptedReasoningEditScopes {
		t.Run(scope.name, func(t *testing.T) {
			source := encryptedReasoningEditDocument(scope.name, "")
			if updated, err := UpdateEncryptedReasoning(source, scope.scope, EncryptedReasoningEdit{}); err != nil || updated != source {
				t.Fatalf("absent no-op changed source: %v", err)
			}
			if updated, err := UpdateEncryptedReasoning(source, scope.scope, EncryptedReasoningEdit{Remove: true}); err != nil || updated != source {
				t.Fatalf("absent removal changed source: %v", err)
			}
			if updated, err := UpdateEncryptedReasoning(source, scope.scope, EncryptedReasoningEdit{Expressions: map[string]*string{"passthrough": nil}}); err != nil || updated != source {
				t.Fatalf("unsetting an absent member created a reset block: %v", err)
			}
			empty, messages := "", []string{}
			updated, err := UpdateEncryptedReasoning(source, scope.scope, EncryptedReasoningEdit{Passthrough: boolPtr(false), OnCallerMismatch: &empty, MatchMessages: &messages})
			if err != nil {
				t.Fatal(err)
			}
			declaration := config.DeclaredEncryptedReasoning(encryptedReasoningEditedPolicy(t, updated, scope.scope))
			if declaration.Passthrough == nil || *declaration.Passthrough || declaration.OnCallerMismatch == nil || declaration.MatchMessages == nil {
				t.Fatal("create did not retain explicit false/optional-empty members")
			}
			noOp, err := UpdateEncryptedReasoning(updated, scope.scope, EncryptedReasoningEdit{})
			if err != nil || noOp != updated {
				t.Fatalf("declared no-op changed source: %v", err)
			}
			reset, err := UpdateEncryptedReasoning(updated, scope.scope, EncryptedReasoningEdit{Reset: true})
			if err != nil {
				t.Fatal(err)
			}
			policy := encryptedReasoningEditedPolicy(t, reset, scope.scope)
			if !reflect.DeepEqual(config.DeclaredEncryptedReasoning(policy), &config.EncryptedReasoningDeclaration{}) || !policy.Passthrough || policy.OnCallerMismatch != config.EncryptedReasoningFail {
				t.Fatal("empty reset removed the local block or persisted normalized defaults")
			}
			removed, err := UpdateEncryptedReasoning(reset, scope.scope, EncryptedReasoningEdit{Remove: true})
			if err != nil {
				t.Fatal(err)
			}
			if encryptedReasoningEditedPolicy(t, removed, scope.scope) != nil {
				t.Fatal("removal did not restore inheritance")
			}
			resetAbsent, err := UpdateEncryptedReasoning(source, scope.scope, EncryptedReasoningEdit{Reset: true})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(config.DeclaredEncryptedReasoning(encryptedReasoningEditedPolicy(t, resetAbsent, scope.scope)), &config.EncryptedReasoningDeclaration{}) {
				t.Fatal("reset on an absent block was treated as inherit")
			}
		})
	}
}

func TestEncryptedReasoningEditRejectsInvalidOrAmbiguousInputs(t *testing.T) {
	invalidMode, shortMessages := "retry", []string{"bad"}
	invalidExpression := "false garbage"
	for _, edit := range []EncryptedReasoningEdit{
		{Remove: true, Reset: true}, {Remove: true, Passthrough: boolPtr(false)}, {Reset: true, MatchMessages: &shortMessages},
		{OnCallerMismatch: &invalidMode}, {MatchMessages: &shortMessages},
		{Expressions: map[string]*string{"unknown": nil}},
		{Expressions: map[string]*string{"passthrough": &invalidExpression}},
		{Passthrough: boolPtr(false), Expressions: map[string]*string{"passthrough": nil}},
	} {
		if _, err := ApplyEncryptedReasoningEdit(&EncryptedReasoningInput{}, edit); err == nil {
			t.Fatalf("invalid edit accepted: %+v", edit)
		}
	}
	for _, block := range []string{
		`encrypted_reasoning = {}`, `encrypted_reasoning "label" {}`, "encrypted_reasoning {}\nencrypted_reasoning {}",
		"encrypted_reasoning {\n nested {}\n}", `encrypted_reasoning { passthrough = null }`,
		`encrypted_reasoning { passthrough = "false" }`, `encrypted_reasoning { on_caller_mismatch = false }`,
		`encrypted_reasoning { match_messages = ["same", "SAME"] }`,
	} {
		if _, err := ParseEncryptedReasoningBlock(block); err == nil {
			t.Fatalf("invalid block accepted: %s", block)
		}
	}
	source := encryptedReasoningEditDocument("root", "")
	for _, scope := range []EncryptedReasoningScope{
		{Model: "m"}, {Provider: "base", Alias: "chat"}, {Provider: "missing"}, {Provider: "base", Model: "missing"}, {Alias: "missing"},
	} {
		if _, err := UpdateEncryptedReasoning(source, scope, EncryptedReasoningEdit{Reset: true}); err == nil {
			t.Fatalf("invalid or missing local owner accepted: %+v", scope)
		}
	}
}
