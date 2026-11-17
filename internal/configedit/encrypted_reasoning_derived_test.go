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

const encryptedReasoningEditDerivedBase = `provider "openai-compatible" "base" {
  api_key = "base-key"
  base_url = "https://peer.example/v1"
  encrypted_reasoning { passthrough = false }
  model "reasoning" {
    upstream_name = "inherited/reasoning"
    encrypted_reasoning { match_messages = ["Base Marker"] }
    deferred_transport {}
  }
  model "deferred" { upstream_name = "inherited/deferred" }
  model "both" {
    upstream_name = "inherited/both"
    encrypted_reasoning { passthrough = false }
  }
  model "ordinary" {
    upstream_name = "inherited/ordinary"
    encrypted_reasoning { passthrough = false }
  }
}
`

func TestEncryptedReasoningRenderStaticDerivedSparseOverlays(t *testing.T) {
	encryptedReasoningEditEnvironment(t)
	deferredEditModelEnvironment(t)
	rawPolicy, err := ParseEncryptedReasoningBlock(encryptedReasoningEditRawBlock)
	if err != nil {
		t.Fatal(err)
	}
	for _, localProvider := range []*EncryptedReasoningInput{nil, {}} {
		t.Run(map[bool]string{true: "inherit-provider", false: "reset-provider"}[localProvider == nil], func(t *testing.T) {
			input := ProviderInput{
				ProviderType: "openai-compatible", Name: "child", Extends: "base",
				Credential:         ProviderCredentialInput{Mode: "inline", APIKeyValue: "child-key"},
				EncryptedReasoning: localProvider,
				Models: []ProviderModelInput{
					{Name: "reasoning", DisplayName: "Must not emit", UpstreamName: "must/not/emit", Protocol: "responses", Capabilities: []string{"embeddings"}, Pricing: &ModelPricingInput{InputPerMillion: "1"}, EncryptedReasoning: rawPolicy},
					{Name: "deferred", UpstreamName: "must/not/emit", DeferredTransportBlock: deferredEditModelBlock},
					{Name: "both", Capabilities: []string{"embeddings"}, DeferredTransportBlock: "deferred_transport {}", EncryptedReasoning: &EncryptedReasoningInput{}},
					{Name: "ordinary", DisplayName: "Inherited", UpstreamName: "inherited/ordinary"},
				},
			}
			block := RenderProviderBlock(input, "")
			for _, forbidden := range []string{"upstream_name", "display_name", "protocol", "capabilities", "pricing", `model "ordinary"`} {
				if strings.Contains(block, forbidden) {
					t.Fatalf("derived rendering flattened or redefined inherited data: %s\n%s", forbidden, block)
				}
			}
			for _, name := range []string{"reasoning", "deferred", "both"} {
				if !strings.Contains(block, `model "`+name+`"`) {
					t.Fatalf("%s overlay disappeared", name)
				}
			}
			if !strings.Contains(block, encryptedReasoningEditRawBlock) || !strings.Contains(block, deferredEditModelBlock) {
				t.Fatal("raw reasoning/deferred source was discarded")
			}
			source := deferredEditAuth + encryptedReasoningEditDerivedBase + block
			if err := ValidateGeneratedConfig([]byte(source), "policy.hcl"); err != nil {
				t.Fatal(err)
			}
			rt, err := config.Load([]byte(source), "policy.hcl")
			if err != nil {
				t.Fatal(err)
			}
			child, _ := rt.Catalog.Provider("child")
			if child.EncryptedReasoningInherited != (localProvider == nil) || (localProvider != nil && !reflect.DeepEqual(config.DeclaredEncryptedReasoning(child.EncryptedReasoning), &config.EncryptedReasoningDeclaration{})) {
				t.Fatal("local provider intent was flattened from inherited values")
			}
			for _, name := range []string{"reasoning", "deferred", "both", "ordinary"} {
				if child.ModelByName[name].UpstreamName != "inherited/"+name {
					t.Fatalf("%s overlay changed the inherited upstream definition", name)
				}
			}
			if child.ModelByName["reasoning"].EncryptedReasoningInherited || child.ModelByName["reasoning"].DeferredTransport != nil || !child.ModelByName["ordinary"].EncryptedReasoningInherited {
				t.Fatal("local reasoning or inherited non-deferred intent changed")
			}
			if policy := child.ModelByName["both"].EncryptedReasoning; policy == nil || !policy.Passthrough || !reflect.DeepEqual(config.DeclaredEncryptedReasoning(policy), &config.EncryptedReasoningDeclaration{}) {
				t.Fatal("combined overlay reset was lost")
			}
			if child.ModelByName["deferred"].DeferredTransport == nil || child.ModelByName["both"].DeferredTransport == nil {
				t.Fatal("local deferred overlays were lost")
			}
			local, err := ReadEncryptedReasoning(source, EncryptedReasoningScope{Provider: "child"})
			if err != nil || (local == nil) != (localProvider == nil) {
				t.Fatalf("source read fabricated inherited provider declarations: %v", err)
			}
			if _, err := ReadEncryptedReasoning(source, EncryptedReasoningScope{Provider: "child", Model: "ordinary"}); err == nil {
				t.Fatal("source read fabricated an absent inherited model overlay")
			}
			for _, name := range []string{"reasoning", "both"} {
				scope := EncryptedReasoningScope{Provider: "child", Model: name}
				noOp, err := UpdateEncryptedReasoning(source, scope, EncryptedReasoningEdit{})
				if err != nil || noOp != source {
					t.Fatalf("derived no-op changed source: %v", err)
				}
				reset, err := UpdateEncryptedReasoning(source, scope, EncryptedReasoningEdit{Reset: true})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(config.DeclaredEncryptedReasoning(encryptedReasoningEditedPolicy(t, reset, scope)), &config.EncryptedReasoningDeclaration{}) {
					t.Fatal("derived reset did not remain local")
				}
				removed, err := UpdateEncryptedReasoning(source, scope, EncryptedReasoningEdit{Remove: true})
				if err != nil {
					t.Fatal(err)
				}
				rt, err := config.Load([]byte(removed), "policy.hcl")
				if err != nil {
					t.Fatalf("removing the last reasoning overlay left an invalid bare model block: %v", err)
				}
				child, _ := rt.Catalog.Provider("child")
				if !child.ModelByName[name].EncryptedReasoningInherited {
					t.Fatal("removal did not restore the base model declaration")
				}
				if name == "reasoning" {
					blocks, err := ParseTopLevelBlocks(removed)
					if err != nil {
						t.Fatal(err)
					}
					childBlock := FindBlock(blocks, func(block TopLevelBlock) bool { return block.Type == "provider" && block.Labels[1] == "child" })
					if strings.Contains(childBlock.Text, `model "reasoning"`) {
						t.Fatal("reasoning-only overlay removal retained a forbidden bare model")
					}
				} else if child.ModelByName[name].DeferredTransport == nil {
					t.Fatal("reasoning removal discarded the combined deferred overlay")
				}
			}
		})
	}
}

func TestEncryptedReasoningStaticDerivedInvalidOverlayPreflight(t *testing.T) {
	dir := t.TempDir()
	path, secrets := filepath.Join(dir, "config.hcl"), filepath.Join(dir, "keys.json")
	valid := deferredEditAuth + encryptedReasoningEditDerivedBase + `provider "openai-compatible" "child" {
  extends = "base"
  api_key = "child-key"
}
`
	if err := WriteConfigFile(path, valid); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secrets, []byte(`{"old":"preserved"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	beforeSecrets, _ := os.ReadFile(secrets)
	for _, overlay := range []string{
		`model "new" {
    encrypted_reasoning {}
  }`,
		`model "reasoning" {}`,
		`model "reasoning" {
    upstream_name = ""
    encrypted_reasoning {}
  }`,
		`model "reasoning" {
    pricing {}
    encrypted_reasoning {}
  }`,
		`model "reasoning" {
    deferred_transport {}
    encrypted_reasoning {}
    encrypted_reasoning {}
  }`,
	} {
		candidate := strings.TrimSuffix(valid, "}\n") + "  " + overlay + "\n}\n"
		if err := WriteProviderFiles(path, candidate, SecretsUpdate{Path: secrets, Key: "new", Value: "rejected-key"}); err == nil {
			t.Fatalf("invalid derived overlay passed preflight: %s", overlay)
		}
		after, _ := os.ReadFile(path)
		afterSecrets, _ := os.ReadFile(secrets)
		if !bytes.Equal(before, after) || !bytes.Equal(beforeSecrets, afterSecrets) {
			t.Fatal("invalid derived edit wrote config or credentials")
		}
	}
}

func TestEncryptedReasoningMemberPatchingInlineBlocks(t *testing.T) {
	for _, source := range []string{
		"encrypted_reasoning {}",
		"encrypted_reasoning { passthrough = true }",
		"/* keep prefix */ encrypted_reasoning {}",
	} {
		input, err := ParseEncryptedReasoningBlock(source)
		if err != nil {
			t.Fatal(err)
		}
		mode := "strip_and_retry"
		updated, err := ApplyEncryptedReasoningEdit(input, EncryptedReasoningEdit{OnCallerMismatch: &mode})
		if err != nil {
			t.Fatalf("could not patch an inline block: %v", err)
		}
		encryptedReasoningEditedPolicy(t, encryptedReasoningEditDocument("root", RenderEncryptedReasoningBlock(updated)), EncryptedReasoningScope{})
	}
	source := encryptedReasoningEditPreamble + `provider "openai" "base" {
  api_key = "base-key"
  model "m" { upstream_name = "upstream/m" }
}
alias "chat" {
  algorithm = "round_robin"
  providers = ["base"]
  model = "m"
}
`
	updated, err := UpdateEncryptedReasoning(source, EncryptedReasoningScope{Provider: "base", Model: "m"}, EncryptedReasoningEdit{Passthrough: boolPtr(false)})
	if err != nil {
		t.Fatal(err)
	}
	encryptedReasoningEditedPolicy(t, updated, EncryptedReasoningScope{Provider: "base", Model: "m"})
}

func TestEncryptedReasoningLastOverlayRemovalDoesNotGuessUnevaluableExtends(t *testing.T) {
	source := deferredEditAuth + encryptedReasoningEditDerivedBase + `provider "openai-compatible" "child" {
  extends = env("EDIT_ER_BASE")
  api_key = "child-key"
  model "reasoning" {
    encrypted_reasoning {}
  }
}
`
	t.Setenv("EDIT_ER_BASE", "base")
	scope := EncryptedReasoningScope{Provider: "child", Model: "reasoning"}
	encryptedReasoningEditedPolicy(t, source, scope)
	if updated, err := UpdateEncryptedReasoning(source, scope, EncryptedReasoningEdit{Remove: true}); err == nil || updated != "" {
		t.Fatal("last overlay removal guessed an unevaluable inheritance declaration")
	}
	if updated, err := UpdateEncryptedReasoning(source, scope, EncryptedReasoningEdit{}); err != nil || updated != source {
		t.Fatalf("unevaluable inheritance changed an otherwise valid no-op: %v", err)
	}
}
