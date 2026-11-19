package configedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestModelPassthroughConfigEditsPreserveDeclarationsAndValidateBeforeWriting(t *testing.T) {
	t.Setenv("EDIT_PASSTHROUGH_PATTERN", `(?i)^vendor/CHAT[0-9]+$`)
	t.Setenv("EDIT_PASSTHROUGH_GRANT", "router/family")
	rules := []string{`model_passthrough "family" {
    match = env("EDIT_PASSTHROUGH_PATTERN")
    capabilities = ["chat"]
  }`}
	grants := `[env("EDIT_PASSTHROUGH_GRANT")]`
	provider := RenderProviderBlock(ProviderInput{
		ProviderType: "openai", Name: "router",
		Credential:             ProviderCredentialInput{Mode: "inline", APIKeyValue: "router-key"},
		ModelPassthroughBlocks: rules,
	}, "")
	auth := RenderAuthBlock(AuthInput{Name: "main", Mode: "bearer_static", Clients: []AuthClientInput{
		{Name: "client", Token: "client-token", AllowedModelPassthroughsExpr: grants},
	}})
	source := "listener \"http\" \"public\" { address = \":8080\" }\n" + provider + auth
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hcl")
	secretPath := filepath.Join(dir, "keys.json")
	if err := WriteConfigFile(path, source); err != nil {
		t.Fatal(err)
	}
	rt, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := rt.Catalog.Provider("router")
	if rule, match := p.MatchModelPassthrough("vendor/chat7"); match != config.ModelPassthroughMatched || rule.Name() != "family" {
		t.Fatal("rendered rule did not match")
	}
	if !strings.Contains(source, rules[0]) || !strings.Contains(source, grants) {
		t.Fatal("rendering lost declared expression text")
	}
	for _, candidate := range []string{
		strings.Replace(source, `model_passthrough "family"`, `model_passthrough "renamed"`, 1),
		strings.Replace(source, `match = env("EDIT_PASSTHROUGH_PATTERN")`, `match = "*"`, 1),
	} {
		err := WriteProviderFiles(path, candidate, SecretsUpdate{Path: secretPath, Key: "router", Value: "staged-test-key"})
		if err == nil {
			t.Fatal("invalid rule or grant was written")
		}
		if data, err := os.ReadFile(path); err != nil || string(data) != source {
			t.Fatal("failed validation changed config")
		}
		if _, err := os.Stat(secretPath); !os.IsNotExist(err) {
			t.Fatal("failed validation changed staged secrets")
		}
	}
}
