package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
)

func writeCopilotCredential(t *testing.T, name, token string) string {
	t.Helper()
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	cred, err := copilotlogin.NewCredential("Ov23testclient", token, time.Now())
	if err != nil {
		t.Fatalf("new credential: %v", err)
	}
	if err := copilotlogin.Save(secretsPath, name, cred); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	return secretsPath
}

func copilotProviderBlock(secretsPath, name string) string {
	return `provider "github-copilot" "copilot" {
  credential_ref {
    path = "` + secretsPath + `"
    name = "` + name + `"
  }
  model "gpt-4o-mini" {}
}
`
}

func TestLoadGitHubCopilotDefaults(t *testing.T) {
	secretsPath := writeCopilotCredential(t, "main", "gho_test-token")
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
` + copilotProviderBlock(secretsPath, "main")
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := testProvider(t, rt, "copilot")
	if p.Type != ProviderTypeGitHubCopilot {
		t.Fatalf("type = %q", p.Type)
	}
	if p.CopilotToken != "gho_test-token" {
		t.Fatalf("token was not resolved")
	}
	if p.APIKey != "" {
		t.Fatalf("APIKey must stay empty for github-copilot, got %q", p.APIKey)
	}
	if !p.CopilotCredentialRef.Resolved {
		t.Fatalf("Resolved flag not set")
	}
	caps := EffectiveCapabilities(p.Type, p.Models[0])
	assertCapabilities(t, caps, []Capability{CapabilityChat})
}

func TestLoadGitHubCopilotRejectsNonChatCapabilities(t *testing.T) {
	secretsPath := writeCopilotCredential(t, "main", "gho_test-token")
	for _, cap := range []string{"responses", "embeddings", "images", "audio_transcriptions", "audio_speech"} {
		cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  credential_ref {
    path = "` + secretsPath + `"
    name = "main"
  }
  model "m" {
    capabilities = ["` + cap + `"]
  }
}
`
		_, err := Load([]byte(cfg), "test.hcl")
		if err == nil || !strings.Contains(err.Error(), "not supported by provider type") {
			t.Fatalf("capability %q: expected unsupported error, got %v", cap, err)
		}
	}
}

func TestLoadGitHubCopilotRejectsMissingCredential(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "credential_ref") {
		t.Fatalf("expected credential_ref error, got %v", err)
	}
}

func TestLoadGitHubCopilotRejectsUnknownCredential(t *testing.T) {
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  credential_ref {
    path = "` + secretsPath + `"
    name = "missing"
  }
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "credential_ref") {
		t.Fatalf("expected credential_ref error, got %v", err)
	}
}

func TestLoadGitHubCopilotRejectsExpiredCredential(t *testing.T) {
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	cred, err := copilotlogin.NewCredential("Ov23testclient", "gho_old", time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cred.ExpiresAt = time.Now().Add(-time.Hour).Unix()
	if err := copilotlogin.Save(secretsPath, "main", cred); err != nil {
		t.Fatal(err)
	}
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
` + copilotProviderBlock(secretsPath, "main")
	_, err = Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestLoadDisabledGitHubCopilotWithoutCredential(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  enabled = false
  model "m" {}
}
provider "openai" "backup" {
  api_key = "sk-backup"
  model "m" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := rt.Catalog.DisabledProviderCount(); got != 1 {
		t.Fatalf("disabled = %d", got)
	}
}

func TestLoadGitHubCopilotRejectsAPIKey(t *testing.T) {
	secretsPath := writeCopilotCredential(t, "main", "gho_test-token")
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  api_key = "sk-inline"
  credential_ref {
    path = "` + secretsPath + `"
    name = "main"
  }
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "credential_ref") {
		t.Fatalf("expected credential error, got %v", err)
	}
}

func TestLoadRejectsCredentialRefOnNonCopilot(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "sk-test"
  credential_ref {
    name = "main"
  }
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "credential_ref is only supported by github-copilot") {
		t.Fatalf("expected credential_ref error, got %v", err)
	}
}

func TestLoadGitHubCopilotRejectsProtocolAndUserAgent(t *testing.T) {
	secretsPath := writeCopilotCredential(t, "main", "gho_test-token")
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  credential_ref {
    path = "` + secretsPath + `"
    name = "main"
  }
  model "m" {
    protocol = "chat"
  }
}
`
	if _, err := Load([]byte(cfg), "test.hcl"); err == nil || !strings.Contains(err.Error(), "protocol is only supported") {
		t.Fatalf("expected protocol error, got %v", err)
	}
	cfg = `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  user_agent = "custom/1.0"
  credential_ref {
    path = "` + secretsPath + `"
    name = "main"
  }
  model "m" {}
}
`
	if _, err := Load([]byte(cfg), "test.hcl"); err == nil || !strings.Contains(err.Error(), "user_agent is only supported") {
		t.Fatalf("expected user_agent error, got %v", err)
	}
}

func TestLoadGitHubCopilotDerivedUsesLocalCredential(t *testing.T) {
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	baseCred, err := copilotlogin.NewCredential("Ov23testclient", "gho_base", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	derivedCred, err := copilotlogin.NewCredential("Ov23testclient", "gho_derived", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := copilotlogin.Save(secretsPath, "base", baseCred); err != nil {
		t.Fatal(err)
	}
	if err := copilotlogin.Save(secretsPath, "team", derivedCred); err != nil {
		t.Fatal(err)
	}
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "base" {
  credential_ref {
    path = "` + secretsPath + `"
    name = "base"
  }
  model "gpt-4o-mini" {
    upstream_name = "gpt-4o"
  }
}
provider "github-copilot" "derived" {
  extends = "base"
  credential_ref {
    path = "` + secretsPath + `"
    name = "team"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	derived := testProvider(t, rt, "derived")
	if derived.CopilotToken != "gho_derived" {
		t.Fatalf("derived token was not locally resolved")
	}
	if derived.CopilotToken == testProvider(t, rt, "base").CopilotToken {
		t.Fatalf("derived provider leaked base credential")
	}
	if derived.Models[0].UpstreamName != "gpt-4o" {
		t.Fatalf("derived models not inherited: %+v", derived.Models)
	}
}

func TestLoadGitHubCopilotDerivedRequiresLocalCredential(t *testing.T) {
	secretsPath := writeCopilotCredential(t, "base", "gho_base")
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "base" {
  credential_ref {
    path = "` + secretsPath + `"
    name = "base"
  }
  model "m" {}
}
provider "github-copilot" "derived" {
  extends = "base"
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "credential_ref") {
		t.Fatalf("expected local credential error, got %v", err)
	}
}

func TestLoadGitHubCopilotDerivedRejectsAPIKey(t *testing.T) {
	secretsPath := writeCopilotCredential(t, "base", "gho_base")
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "base" {
  credential_ref {
    path = "` + secretsPath + `"
    name = "base"
  }
  model "m" {}
}
provider "github-copilot" "derived" {
  extends = "base"
  api_key = "sk-inline"
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "credential_ref") {
		t.Fatalf("expected credential_ref error, got %v", err)
	}
}

func TestLoadGitHubCopilotBaseURLValidation(t *testing.T) {
	secretsPath := writeCopilotCredential(t, "main", "gho_test-token")
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  base_url = "http://example.com"
  credential_ref {
    path = "` + secretsPath + `"
    name = "main"
  }
  model "m" {}
}
`
	if _, err := Load([]byte(cfg), "test.hcl"); err == nil || !strings.Contains(err.Error(), "non-HTTPS base_url") {
		t.Fatalf("expected base_url error, got %v", err)
	}
	loopback := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  base_url = "http://127.0.0.1:11434"
  credential_ref {
    path = "` + secretsPath + `"
    name = "main"
  }
  model "m" {}
}
`
	if _, err := Load([]byte(loopback), "test.hcl"); err != nil {
		t.Fatalf("loopback base_url should be allowed: %v", err)
	}
}

func TestLoadGitHubCopilotCredentialNameRequired(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  credential_ref {
    path = "` + os.TempDir() + `/keys.json"
  }
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || (!strings.Contains(err.Error(), "credential_ref.name") && !strings.Contains(err.Error(), `"name" is required`)) {
		t.Fatalf("expected credential_ref.name error, got %v", err)
	}
}
