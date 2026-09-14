package configedit

import (
	"strings"
	"testing"
)

func TestRenderAliasBlockEncryptedReasoning(t *testing.T) {
	out := RenderAliasBlock(AliasInput{
		Name:      "a",
		Algorithm: "round_robin",
		EncryptedReasoning: &AliasEncryptedReasoningInput{
			Passthrough:      boolPtr(false),
			OnCallerMismatch: "strip_and_retry",
			MatchMessages:    []string{"not issued to this caller"},
		},
		Targets: []AliasTargetInput{{Provider: "p1", Model: "m"}},
	})
	for _, want := range []string{"encrypted_reasoning", "passthrough = false", "strip_and_retry", "not issued to this caller"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered block missing %q:\n%s", want, out)
		}
	}
}

func TestRenderAliasBlockWithoutEncryptedReasoningOmitsBlock(t *testing.T) {
	out := RenderAliasBlock(AliasInput{
		Name:      "a",
		Algorithm: "round_robin",
		Targets:   []AliasTargetInput{{Provider: "p1", Model: "m"}},
	})
	if strings.Contains(out, "encrypted_reasoning") {
		t.Fatalf("rendered block must omit encrypted_reasoning:\n%s", out)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
