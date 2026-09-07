package config

import (
	"strings"
	"testing"
)

func openCodeTestHeader() string {
	return `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
`
}

func TestLoadOpenCodeZenWithOmittedURL(t *testing.T) {
	cfg := openCodeTestHeader() + `
provider "opencode-zen" "zen" {
  api_key = "sk-zen"
  model "glm-5.3" {
    protocol = "chat"
    capabilities = ["chat"]
  }
  model "claude-sonnet-5" {
    protocol = "messages"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := testProvider(t, rt, "zen")
	if p.Type != ProviderTypeOpenCodeZen {
		t.Fatalf("provider type = %q", p.Type)
	}
	if p.BaseURL != "" {
		t.Fatalf("base_url should stay omitted, got %q", p.BaseURL)
	}
	chat := p.ModelByName["glm-5.3"]
	if chat.Protocol != ModelProtocolChat || chat.UpstreamName != "glm-5.3" {
		t.Fatalf("chat model = %+v", chat)
	}
	messages := p.ModelByName["claude-sonnet-5"]
	if messages.Protocol != ModelProtocolMessages {
		t.Fatalf("messages model = %+v", messages)
	}
	assertCapabilities(t, EffectiveCapabilities(p.Type, messages), []Capability{CapabilityChat, CapabilityResponses})
}

func TestLoadOpenCodeGoWithOmittedURL(t *testing.T) {
	cfg := openCodeTestHeader() + `
provider "opencode-go" "go" {
  api_key = "sk-go"
  model "minimax-m3" {
    protocol = "messages"
  }
  model "glm-5.3" {
    protocol = "chat"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := testProvider(t, rt, "go")
	if p.Type != ProviderTypeOpenCodeGo || p.BaseURL != "" {
		t.Fatalf("provider = %+v", p)
	}
	if got := p.ModelByName["minimax-m3"].Protocol; got != ModelProtocolMessages {
		t.Fatalf("minimax-m3 protocol = %q", got)
	}
	assertCapabilities(t, EffectiveCapabilities(p.Type, p.ModelByName["glm-5.3"]), []Capability{CapabilityChat})
}

func TestLoadOpenCodeGeminiProtocol(t *testing.T) {
	cfg := openCodeTestHeader() + `
provider "opencode-zen" "zen" {
  api_key = "sk-zen"
  model "gemini-3.8-flash" {
    protocol = "gemini"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := testProvider(t, rt, "zen")
	assertCapabilities(t, EffectiveCapabilities(p.Type, p.ModelByName["gemini-3.8-flash"]), []Capability{CapabilityChat, CapabilityResponses})
}

func TestLoadOpenCodeRejectsProtocols(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		want     string
	}{
		{
			name:     "omitted protocol",
			provider: "provider \"opencode-zen\" \"zen\" {\n  api_key = \"k\"\n  model \"m\" {}\n}",
			want:     `invalid protocol ""`,
		},
		{
			name:     "unknown protocol",
			provider: "provider \"opencode-zen\" \"zen\" {\n  api_key = \"k\"\n  model \"m\" {\n    protocol = \"openai\"\n  }\n}",
			want:     `invalid protocol "openai"`,
		},
		{
			name:     "gemini on go",
			provider: "provider \"opencode-go\" \"go\" {\n  api_key = \"k\"\n  model \"m\" {\n    protocol = \"gemini\"\n  }\n}",
			want:     `protocol "gemini" is not supported by provider type "opencode-go"`,
		},
		{
			name:     "embeddings capability",
			provider: "provider \"opencode-zen\" \"zen\" {\n  api_key = \"k\"\n  model \"m\" {\n    protocol = \"chat\"\n    capabilities = [\"embeddings\"]\n  }\n}",
			want:     `capability "embeddings" is not supported by provider type "opencode-zen"`,
		},
		{
			name:     "images capability",
			provider: "provider \"opencode-go\" \"go\" {\n  api_key = \"k\"\n  model \"m\" {\n    protocol = \"messages\"\n    capabilities = [\"images\"]\n  }\n}",
			want:     `capability "images" is not supported by provider type "opencode-go"`,
		},
		{
			name:     "responses outside chat protocol",
			provider: "provider \"opencode-zen\" \"zen\" {\n  api_key = \"k\"\n  model \"m\" {\n    protocol = \"chat\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n}",
			want:     `capability "responses" is not served by protocol "chat"`,
		},
		{
			name:     "chat outside responses protocol",
			provider: "provider \"opencode-zen\" \"zen\" {\n  api_key = \"k\"\n  model \"m\" {\n    protocol = \"responses\"\n    capabilities = [\"chat\"]\n  }\n}",
			want:     `capability "chat" is not served by protocol "responses"`,
		},
		{
			name:     "missing credential",
			provider: "provider \"opencode-zen\" \"zen\" {\n  api_key = \"\"\n  model \"m\" {\n    protocol = \"chat\"\n  }\n}",
			want:     `require a non-empty api_key`,
		},
		{
			name:     "generic opencode type",
			provider: "provider \"opencode\" \"o\" {\n  api_key = \"k\"\n  model \"m\" {}\n}",
			want:     `unsupported type "opencode"`,
		},
		{
			name:     "protocol on openai",
			provider: "provider \"openai\" \"o\" {\n  api_key = \"k\"\n  model \"m\" {\n    protocol = \"chat\"\n  }\n}",
			want:     `protocol is only supported by opencode-zen and opencode-go`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := openCodeTestHeader() + tc.provider + "\n"
			_, err := Load([]byte(cfg), "test.hcl")
			if err == nil {
				t.Fatal("expected load error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadOpenCodeAllowsLoopbackOverride(t *testing.T) {
	cfg := openCodeTestHeader() + `
provider "opencode-zen" "zen" {
  base_url = "http://127.0.0.1:9/custom/"
  api_key = "k"
  model "m" {
    protocol = "chat"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := testProvider(t, rt, "zen").BaseURL; got != "http://127.0.0.1:9/custom/" {
		t.Fatalf("base_url = %q", got)
	}
}

func TestLoadOpenCodeRejectsRemoteHTTPOverride(t *testing.T) {
	cfg := openCodeTestHeader() + `
provider "opencode-go" "go" {
  base_url = "http://example.com/v1"
  api_key = "k"
  model "m" {
    protocol = "chat"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "non-HTTPS base_url is allowed only for loopback hosts") {
		t.Fatalf("expected loopback error, got %v", err)
	}
}

func TestLoadOpenCodeSameTypeInheritance(t *testing.T) {
	cfg := openCodeTestHeader() + `
provider "opencode-zen" "base" {
  api_key = "sk-base"
  model "glm-5.3" {
    protocol = "chat"
  }
}
provider "opencode-zen" "child" {
  extends = "base"
  api_key = "sk-child"
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	child := testProvider(t, rt, "child")
	if child.Type != ProviderTypeOpenCodeZen {
		t.Fatalf("child type = %q", child.Type)
	}
	if got := child.ModelByName["glm-5.3"].Protocol; got != ModelProtocolChat {
		t.Fatalf("child model protocol = %q", got)
	}
}

func TestLoadOpenCodeRejectsCrossTypeInheritance(t *testing.T) {
	for _, tc := range []struct {
		name  string
		child string
		base  string
	}{
		{name: "zen extends go", child: "opencode-zen", base: "opencode-go"},
		{name: "go extends zen", child: "opencode-go", base: "opencode-zen"},
		{name: "openai extends zen", child: "openai", base: "opencode-zen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modelBlock := `model "m" {}`
			if tc.base == "opencode-zen" || tc.base == "opencode-go" {
				modelBlock = `model "m" { protocol = "chat" }`
			}
			cfg := openCodeTestHeader() + `
provider "` + tc.base + `" "base" {
  api_key = "sk-base"
  ` + modelBlock + `
}
provider "` + tc.child + `" "child" {
  extends = "base"
  api_key = "sk-child"
}
`
			_, err := Load([]byte(cfg), "test.hcl")
			if err == nil || !strings.Contains(err.Error(), "must match base provider") {
				t.Fatalf("expected type-match error, got %v", err)
			}
		})
	}
}

func TestLoadOpenCodeDerivedCannotDeclareProtocolModels(t *testing.T) {
	cfg := openCodeTestHeader() + `
provider "opencode-go" "base" {
  api_key = "sk-base"
  model "m" {
    protocol = "messages"
  }
}
provider "opencode-go" "child" {
  extends = "base"
  api_key = "sk-child"
  model "m" {
    protocol = "messages"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "derived provider cannot declare model blocks") {
		t.Fatalf("expected derived-model error, got %v", err)
	}
}

func TestLoadOpenCodeReloadPreservesProtocol(t *testing.T) {
	cfg := openCodeTestHeader() + `
provider "opencode-zen" "zen" {
  api_key = "sk-zen"
  model "minimax-m3" {
    protocol = "chat"
  }
}
provider "opencode-go" "go" {
  api_key = "sk-go"
  model "minimax-m3" {
    protocol = "messages"
  }
}
`
	first, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if got := testProvider(t, second, "zen").ModelByName["minimax-m3"].Protocol; got != ModelProtocolChat {
		t.Fatalf("zen minimax-m3 protocol = %q", got)
	}
	if got := testProvider(t, second, "go").ModelByName["minimax-m3"].Protocol; got != ModelProtocolMessages {
		t.Fatalf("go minimax-m3 protocol = %q", got)
	}
	if first.Catalog.ProviderCount() != second.Catalog.ProviderCount() {
		t.Fatalf("provider counts differ after reload")
	}
}

func TestEffectiveCapabilitiesOpenCodeProtocolAware(t *testing.T) {
	for _, tc := range []struct {
		protocol ModelProtocol
		want     []Capability
	}{
		{protocol: ModelProtocolChat, want: []Capability{CapabilityChat}},
		{protocol: ModelProtocolResponses, want: []Capability{CapabilityResponses}},
		{protocol: ModelProtocolMessages, want: []Capability{CapabilityChat, CapabilityResponses}},
		{protocol: ModelProtocolGemini, want: []Capability{CapabilityChat, CapabilityResponses}},
		{protocol: ModelProtocol(""), want: nil},
		{protocol: ModelProtocol("bogus"), want: nil},
	} {
		for _, providerType := range []ProviderType{ProviderTypeOpenCodeZen, ProviderTypeOpenCodeGo} {
			got := EffectiveCapabilities(providerType, Model{Protocol: tc.protocol})
			if len(got) != len(tc.want) {
				t.Fatalf("%s/%s capabilities = %v, want %v", providerType, tc.protocol, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("%s/%s capabilities = %v, want %v", providerType, tc.protocol, got, tc.want)
				}
			}
		}
	}
}

func TestEffectiveCapabilitiesOpenCodeExplicitCapsPreserved(t *testing.T) {
	model := Model{Protocol: ModelProtocolChat, Capabilities: []Capability{CapabilityChat}}
	assertCapabilities(t, EffectiveCapabilities(ProviderTypeOpenCodeGo, model), []Capability{CapabilityChat})
}
