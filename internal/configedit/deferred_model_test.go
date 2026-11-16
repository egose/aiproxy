package configedit

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

const deferredEditModelBlock = `deferred_transport {
      control_timeout = env("EDIT_MODEL_DEFER_CONTROL")
      max_wait = env("EDIT_MODEL_DEFER_WAIT")
      poll_interval = "500ms"
      max_control_calls = env("EDIT_MODEL_DEFER_CALLS")
      max_control_retries = 0
    }`

func deferredEditModelEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("EDIT_MODEL_DEFER_CONTROL", "100ms")
	t.Setenv("EDIT_MODEL_DEFER_WAIT", "20m")
	t.Setenv("EDIT_MODEL_DEFER_CALLS", "48")
}

func TestRenderProviderDeferredModelBlocksPreservesPlacementAndSource(t *testing.T) {
	deferredEditModelEnvironment(t)
	for _, derived := range []bool{false, true} {
		for _, compatibility := range []bool{false, true} {
			t.Run(strings.Join([]string{map[bool]string{false: "concrete", true: "derived"}[derived], map[bool]string{false: "model-only", true: "compatibility"}[compatibility]}, "/"), func(t *testing.T) {
				input := ProviderInput{
					ProviderType: "openai-compatible", Name: "edited", BaseURL: "https://peer.example/v1",
					Credential: ProviderCredentialInput{Mode: "inline", APIKeyValue: "configured-key"},
					Models: []ProviderModelInput{
						{Name: "ordinary", UpstreamName: "executor/ordinary"},
						{Name: "queued", DisplayName: "Queued", UpstreamName: "executor/queued", Capabilities: []string{"chat"}, DeferredTransportBlock: deferredEditModelBlock},
						{Name: "empty", UpstreamName: "executor/empty", DeferredTransportBlock: "deferred_transport {}"},
					},
				}
				if compatibility {
					input.DeferredTransportBlock = `deferred_transport { max_wait = "30m" }`
				}
				source := deferredEditAuth
				if derived {
					input.Extends = "base"
					source += `provider "openai-compatible" "base" {
  base_url = "https://peer.example/v1"
  api_key = "base-key"
  model "ordinary" {
    upstream_name = "inherited/ordinary"
    deferred_transport {}
  }
  model "queued" { upstream_name = "inherited/queued" }
  model "empty" { upstream_name = "inherited/empty" }
}
`
				}
				block := RenderProviderBlock(input, "")
				if !strings.Contains(block, deferredEditModelBlock) {
					t.Fatal("nested block or original env expressions were discarded")
				}
				if derived {
					for _, forbidden := range []string{`model "ordinary"`, "upstream_name", "display_name", "capabilities"} {
						if strings.Contains(block, forbidden) {
							t.Fatalf("derived overlay rendered an arbitrary model definition: %s", forbidden)
						}
					}
				}
				source += block
				if err := ValidateGeneratedConfig([]byte(source), "config.hcl"); err != nil {
					t.Fatal(err)
				}
				rt, err := config.Load([]byte(source), "config.hcl")
				if err != nil {
					t.Fatal(err)
				}
				p, _ := rt.Catalog.Provider("edited")
				want := config.DeferredTransport{ControlTimeout: 100 * time.Millisecond, MaxWait: 20 * time.Minute, PollInterval: 500 * time.Millisecond, MaxControlCalls: 48, MaxControlRetries: 0}
				if (p.DeferredTransport != nil) != compatibility || !reflect.DeepEqual(p.ModelByName["queued"].DeferredTransport, &want) {
					t.Fatal("model options were promoted to a provider default or lost")
				}
				if empty := p.ModelByName["empty"].DeferredTransport; empty == nil || *empty != config.DefaultDeferredTransport() {
					t.Fatal("empty model block did not replace compatibility options with standard defaults")
				}
				if p.ModelByName["ordinary"].DeferredTransport != nil || (config.EffectiveDeferredTransport(p, p.ModelByName["ordinary"]) != nil) != compatibility {
					t.Fatal("ordinary sibling gained a local or inherited caller opt-in")
				}
				if derived && p.ModelByName["queued"].UpstreamName != "inherited/queued" {
					t.Fatal("transport-only overlay redefined the inherited model")
				}
			})
		}
	}
}

func TestRenderDisabledDeferredModelCallerRetainsCredentialSources(t *testing.T) {
	deferredEditModelEnvironment(t)
	t.Setenv("EDIT_MODEL_PEER_KEY", "configured-key")
	disabled := false
	for _, credential := range []ProviderCredentialInput{
		{Mode: "inline", APIKeyValue: "configured-key"},
		{Mode: "env_expression", APIKeyValue: `env("EDIT_MODEL_PEER_KEY")`},
		{Mode: "secrets_file", SecretsPath: filepath.Join(t.TempDir(), "unresolved.json"), SecretsKey: "peer"},
	} {
		t.Run(credential.Mode, func(t *testing.T) {
			input := ProviderInput{
				ProviderType: "openai-compatible", Name: "disabled", Enabled: &disabled, BaseURL: "https://peer.example/v1",
				Credential: credential, Models: []ProviderModelInput{{Name: "ordinary"}, {Name: "queued", DeferredTransportBlock: deferredEditModelBlock}},
			}
			source := deferredEditAuth + RenderProviderBlock(input, "")
			if !input.HasCallerDeferredTransport() || !strings.Contains(source, "api_key") {
				t.Fatal("model caller presence failed to preserve disabled credentials")
			}
			rt, err := config.Load([]byte(source), "config.hcl")
			if err != nil {
				t.Fatal(err)
			}
			p := rt.Catalog.DisabledProviders()[0]
			if !config.HasCallerDeferredTransport(p) || p.ModelByName["ordinary"].DeferredTransport != nil {
				t.Fatal("disabled model opt-ins changed")
			}
			if credential.Mode == "secrets_file" {
				if p.APIKeyRef == nil || p.APIKeyRef.Path != credential.SecretsPath || p.APIKeyRef.Key != "peer" || p.APIKeyRef.Resolved {
					t.Fatal("disabled caller resolved or discarded its credential reference")
				}
			} else if p.APIKey != "configured-key" {
				t.Fatal("disabled caller discarded its configured credential")
			}
			input.Models[1].DeferredTransportBlock = ""
			if input.HasCallerDeferredTransport() || strings.Contains(RenderProviderBlock(input, ""), "api_key") {
				t.Fatal("ordinary disabled provider credential behavior changed")
			}
		})
	}
}

func TestDeferredModelFilePreflightRejectsBeforeStagedCredentialWrites(t *testing.T) {
	deferredEditModelEnvironment(t)
	dir := t.TempDir()
	path, secrets := filepath.Join(dir, "config.hcl"), filepath.Join(dir, "keys.json")
	input := ProviderInput{
		ProviderType: "openai-compatible", Name: "edited", BaseURL: "https://peer.example/v1",
		Credential: ProviderCredentialInput{Mode: "secrets_file", SecretsPath: secrets, SecretsKey: "peer"},
		Models: []ProviderModelInput{
			{Name: "ordinary"}, {Name: "queued", DeferredTransportBlock: deferredEditModelBlock},
		},
	}
	source := deferredEditAuth + RenderProviderBlock(input, "")
	if err := WriteProviderFiles(path, source, SecretsUpdate{Path: secrets, Key: "peer", Value: "configured-key"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	beforeSecrets, _ := os.ReadFile(secrets)
	for _, invalid := range []string{
		strings.Replace(source, `"500ms"`, `"49ms"`, 1),
		strings.Replace(source, `"queued" {`, `"queued" {
    capabilities = ["embeddings"]`, 1),
		strings.Replace(source, "  model \"ordinary\"", "  deferred_receiver {}\n  model \"ordinary\"", 1),
		strings.Replace(source, "openai-compatible", "openai", 1),
	} {
		if invalid == source {
			t.Fatal("invalid preflight fixture did not change the source")
		}
		if err := WriteProviderFiles(path, invalid, SecretsUpdate{Path: secrets, Key: "peer", Value: "rejected-key"}); err == nil {
			t.Fatal("invalid nested caller passed file preflight")
		}
		after, _ := os.ReadFile(path)
		afterSecrets, _ := os.ReadFile(secrets)
		if !bytes.Equal(before, after) || !bytes.Equal(beforeSecrets, afterSecrets) {
			t.Fatal("rejected model caller edit wrote config or staged credentials")
		}
	}
	t.Setenv("EDIT_MODEL_DEFER_CONTROL", "99ms")
	if err := WriteProviderFiles(path, source, SecretsUpdate{Path: secrets, Key: "peer", Value: "rejected-key"}); err == nil {
		t.Fatal("invalid nested duration expression passed file preflight")
	}
	after, _ := os.ReadFile(path)
	afterSecrets, _ := os.ReadFile(secrets)
	if !bytes.Equal(before, after) || !bytes.Equal(beforeSecrets, afterSecrets) {
		t.Fatal("invalid env duration wrote config or credentials")
	}
}

func TestAvailableProviderModelsIncludesAllInheritedModelsWithDeferredOverlays(t *testing.T) {
	blocks, err := ParseTopLevelBlocks(`provider "openai-compatible" "base" {
  base_url = "https://peer.example/v1"
  api_key = "configured-key"
  model "ordinary" {}
  model "queued" {}
}
provider "openai-compatible" "child" {
  extends = "base"
  api_key = "child-key"
  model "queued" {
    deferred_transport {}
  }
}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := AvailableProviderModels(blocks), []string{"base/ordinary", "base/queued", "child/ordinary", "child/queued"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("overlay narrowed the inherited editor inventory: got=%v want=%v", got, want)
	}
}
