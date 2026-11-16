package configedit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

const deferredEditAuth = `listener "http" "public" { address = ":8080" }
auth "main" {
  mode = "bearer_static"
  client "peer" { token = "peer-token" }
}
deferred {}
`

func TestRenderProviderDeferredBlocksPreservesCompleteSource(t *testing.T) {
	t.Setenv("EDIT_DEFER_TIMEOUT", "100ms")
	t.Setenv("EDIT_DEFER_RESULT", "4096")
	for _, tc := range []struct {
		caller string
		recv   string
	}{
		{`deferred_transport {
    control_timeout = env("EDIT_DEFER_TIMEOUT")
    max_wait = "2m"
    poll_interval = "100ms"
    max_control_calls = 64
    max_control_retries = 0
  }`, ""},
		{"", `deferred_receiver {
    execution_timeout = "1m"
    max_result_bytes = env("EDIT_DEFER_RESULT")
  }`},
		{"deferred_transport {}", ""},
		{"", "deferred_receiver {}"},
	} {
		for _, derived := range []bool{false, true} {
			input := ProviderInput{
				ProviderType: "openai-compatible", Name: "edited", BaseURL: "https://peer.example/mount/v1",
				Credential:             ProviderCredentialInput{Mode: "inline", APIKeyValue: "k"},
				Models:                 []ProviderModelInput{{Name: "m"}},
				DeferredTransportBlock: tc.caller, DeferredReceiverBlock: tc.recv,
			}
			source := deferredEditAuth
			if derived {
				input.Extends, input.BaseURL, input.Models = "base", "", nil
				source += `provider "openai-compatible" "base" {
  base_url = "https://peer.example/mount/v1"
  api_key = "base-key"
  model "m" {}
}
`
			}
			block := RenderProviderBlock(input, "")
			for _, preserved := range []string{tc.caller, tc.recv} {
				if preserved != "" && !strings.Contains(block, preserved) {
					t.Fatalf("complete source block was not preserved:\n%s", block)
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
			if (p.DeferredTransport != nil) != (tc.caller != "") || (p.DeferredReceiver != nil) != (tc.recv != "") {
				t.Fatal("rendered block presence changed")
			}
		}
	}
}

func TestWriteProviderDeferredPreflightPreservesEnvAndStagedSecrets(t *testing.T) {
	dir := t.TempDir()
	path, secrets := filepath.Join(dir, "config.hcl"), filepath.Join(dir, "keys.json")
	t.Setenv("EDIT_DEFER_CONTROL", "100ms")
	t.Setenv("EDIT_DEFER_REQUEST", "2048")
	input := ProviderInput{
		ProviderType: "openai-compatible", Name: "edited", BaseURL: "https://peer.example/mount/v1",
		Credential: ProviderCredentialInput{Mode: "secrets_file", SecretsPath: secrets, SecretsKey: "staged"},
		Models:     []ProviderModelInput{{Name: "m"}},
		DeferredTransportBlock: `deferred_transport {
    control_timeout = env("EDIT_DEFER_CONTROL")
    max_wait = "3m"
    poll_interval = "250ms"
    max_control_calls = 64
    max_control_retries = 0
  }`,
	}
	source := strings.Replace(deferredEditAuth, "deferred {}", "deferred { max_request_bytes = env(\"EDIT_DEFER_REQUEST\") }", 1) + RenderProviderBlock(input, "")
	if err := ValidateGeneratedConfig([]byte(source), path); err != nil {
		t.Fatalf("preflight tried to resolve uncommitted credentials: %v", err)
	}
	if err := WriteProviderFiles(path, source, SecretsUpdate{Path: secrets, Key: "staged", Value: "staged-key"}); err != nil {
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
	for _, preserved := range []string{input.DeferredTransportBlock, `env("EDIT_DEFER_REQUEST")`} {
		if !bytes.Contains(before, []byte(preserved)) {
			t.Fatal("source/environment spelling changed during edit")
		}
	}
	rt, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := rt.Catalog.Provider("edited")
	if p.APIKey != "staged-key" || p.DeferredTransport.ControlTimeout != 100*time.Millisecond || rt.Deferred.MaxRequestBytes != 2048 {
		t.Fatal("deferred settings and staged credentials did not activate together")
	}
	t.Setenv("EDIT_DEFER_CONTROL", "99ms")
	if err := WriteProviderFiles(path, source, SecretsUpdate{Path: secrets, Key: "staged", Value: "rejected-key"}); err == nil {
		t.Fatal("invalid deferred setting passed file preflight")
	}
	after, _ := os.ReadFile(path)
	afterSecrets, _ := os.ReadFile(secrets)
	if !bytes.Equal(before, after) || !bytes.Equal(beforeSecrets, afterSecrets) {
		t.Fatal("rejected deferred edit wrote config or staged secrets")
	}
}

func TestDeferredGeneratedValidationAndDisabledCredentialPreservation(t *testing.T) {
	disabled := false
	input := ProviderInput{
		ProviderType: "openai-compatible", Name: "edited", BaseURL: "https://peer.example/v1",
		Credential: ProviderCredentialInput{Mode: "inline", APIKeyValue: "kept-key"},
		Enabled:    &disabled, Models: []ProviderModelInput{{Name: "m"}},
		DeferredTransportBlock: "deferred_transport {}",
	}
	source := deferredEditAuth + RenderProviderBlock(input, "")
	rt, err := config.Load([]byte(source), "config.hcl")
	if err != nil {
		t.Fatal(err)
	}
	p := rt.Catalog.DisabledProviders()[0]
	if p.APIKey != "kept-key" || p.DeferredTransport == nil {
		t.Fatal("disabling a deferred caller erased its required configured credential")
	}
	for _, candidate := range []string{
		strings.Replace(source, "deferred_transport {}", `deferred_transport { enabled = false }`, 1),
		strings.Replace(source, "deferred_transport {}", `deferred_transport { max_control_retries = -1 }`, 1),
		strings.Replace(source, "deferred {}", `deferred { max_memory_bytes = 16777216 }`, 1),
		strings.Replace(source, `mode = "bearer_static"`, `mode = "none"`, 1),
		`{"deferred":null}`,
		`{"deferred":[]}`,
		`{"provider":{"openai-compatible":{"p":{"deferred_transport":null}}}}`,
		`{"provider":{"openai-compatible":{"p":{"deferred_transport":{"max_control_retries":null}}}}}`,
	} {
		if err := ValidateGeneratedConfig([]byte(candidate), "config.hcl"); err == nil {
			t.Fatalf("invalid deferred candidate accepted:\n%s", candidate)
		}
	}
}

func TestDeferredRootSourceUntouchedByOtherEdits(t *testing.T) {
	t.Setenv("EDIT_DEFER_RETENTION", "1m")
	root := `deferred {
  retention = env("EDIT_DEFER_RETENTION")
  control_timeout = "100ms"
}
`
	source := strings.Replace(deferredEditAuth, "deferred {}\n", root, 1) + RenderProviderBlock(ProviderInput{
		ProviderType: "openai", Name: "receiver",
		Credential:            ProviderCredentialInput{Mode: "inline", APIKeyValue: "k"},
		Models:                []ProviderModelInput{{Name: "m"}},
		DeferredReceiverBlock: "deferred_receiver {}",
	}, "")
	updated, err := UpsertTopLevelStringAttribute(source, "upstream_header_timeout", "120s")
	if err != nil || !strings.Contains(updated, root) {
		t.Fatalf("root block/source changed during unrelated edit: %v", err)
	}
	if err := ValidateGeneratedConfig([]byte(updated), "config.hcl"); err != nil {
		t.Fatal(err)
	}
}
