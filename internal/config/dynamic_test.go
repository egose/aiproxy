package config

import (
	"strings"
	"testing"
)

func validDynamicProvider() Provider {
	return Provider{
		Type:    ProviderTypeOpenAI,
		Name:    "db-openai",
		Enabled: true,
		APIKey:  "secret",
		Models: []Model{
			{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"},
		},
	}
}

func TestValidateDynamicProviderOK(t *testing.T) {
	if err := ValidateDynamicProvider(validDynamicProvider()); err != nil {
		t.Fatalf("ValidateDynamicProvider = %v, want nil", err)
	}
}

func TestValidateDynamicProviderRules(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Provider)
		want   string
	}{
		{"bad name", func(p *Provider) { p.Name = "Bad Name" }, "lowercase"},
		{"reserved name", func(p *Provider) { p.Name = "alias" }, "reserved"},
		{"unknown type", func(p *Provider) { p.Type = "nope" }, "unsupported type"},
		{"compatible needs base url", func(p *Provider) { p.Type = ProviderTypeOpenAICompatible }, "base_url is required"},
		{"bad base url", func(p *Provider) { p.BaseURL = "http://example.com" }, "loopback"},
		{"base url userinfo", func(p *Provider) { p.BaseURL = "https://u@h.example.com" }, "userinfo"},
		{"both credentials", func(p *Provider) { p.APIKeyRef = &APIKeyRef{Key: "k"} }, "only one of api_key or api_key_ref"},
		{"ref without key", func(p *Provider) { p.APIKey = ""; p.APIKeyRef = &APIKeyRef{} }, "api_key_ref.key is required"},
		{"missing credential", func(p *Provider) { p.APIKey = "" }, "require a non-empty api_key"},
		{"copilot with api key", func(p *Provider) {
			p.Type = ProviderTypeGitHubCopilot
			p.CopilotCredentialRef = &CopilotCredentialRef{Name: "c"}
		}, "not supported by github-copilot"},
		{"copilot without ref", func(p *Provider) {
			p.Type = ProviderTypeGitHubCopilot
			p.APIKey = ""
		}, "credential_ref name"},
		{"copilot ref on openai", func(p *Provider) { p.CopilotCredentialRef = &CopilotCredentialRef{Name: "c"} }, "only supported by github-copilot"},
		{"no models", func(p *Provider) { p.Models = nil }, "at least one model"},
		{"dup models", func(p *Provider) {
			p.Models = append(p.Models, Model{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"})
		}, "duplicate model"},
		{"bad model name", func(p *Provider) { p.Models[0].Name = "Bad" }, "each '/'-separated segment"},
		{"empty upstream", func(p *Provider) { p.Models[0].UpstreamName = "" }, "empty upstream_name"},
		{"protocol on openai", func(p *Provider) { p.Models[0].Protocol = ModelProtocolChat }, "only supported by opencode-zen"},
		{"bad capability", func(p *Provider) { p.Models[0].Capabilities = []Capability{"nope"} }, "invalid capability"},
		{"unsupported capability", func(p *Provider) {
			p.Models[0].Capabilities = []Capability{CapabilityImages}
			p.Type = ProviderTypeAnthropic
		}, "not supported by provider type"},
		{"dup capability", func(p *Provider) { p.Models[0].Capabilities = []Capability{CapabilityChat, CapabilityChat} }, "duplicate capability"},
		{"negative pricing", func(p *Provider) {
			p.Models[0].Pricing = &ModelPricing{InputPerMillion: -1}
		}, "must not be negative"},
		{"healthcheck on copilot", func(p *Provider) {
			p.Type = ProviderTypeGitHubCopilot
			p.APIKey = ""
			p.CopilotCredentialRef = &CopilotCredentialRef{Name: "c"}
			p.Healthcheck = &ProviderHealthcheck{Path: "/x"}
		}, "not supported by github-copilot"},
	}
	for _, tc := range cases {
		p := validDynamicProvider()
		tc.mutate(&p)
		if err := ValidateDynamicProvider(p); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want substring %q", tc.name, err, tc.want)
		}
	}
}

func TestValidateDynamicProviderZenKeyless(t *testing.T) {
	p := validDynamicProvider()
	p.Type = ProviderTypeOpenCodeZen
	p.APIKey = ""
	p.Models[0].Protocol = ModelProtocolChat
	if err := ValidateDynamicProvider(p); err != nil {
		t.Fatalf("zen keyless = %v, want nil", err)
	}
}

func TestValidateDynamicProviderZenProtocol(t *testing.T) {
	p := validDynamicProvider()
	p.Type = ProviderTypeOpenCodeZen
	p.APIKey = ""
	if err := ValidateDynamicProvider(p); err == nil || !strings.Contains(err.Error(), "invalid protocol") {
		t.Fatalf("zen empty protocol err = %v, want invalid protocol", err)
	}
	p.Models[0].Protocol = ModelProtocolGemini
	p.Type = ProviderTypeOpenCodeGo
	p.APIKey = "secret"
	if err := ValidateDynamicProvider(p); err == nil || !strings.Contains(err.Error(), "not supported by provider type") {
		t.Fatalf("go gemini err = %v, want not supported", err)
	}
}

func TestValidateDynamicProviderDisabledSkipsCredential(t *testing.T) {
	p := validDynamicProvider()
	p.Enabled = false
	p.APIKey = ""
	if err := ValidateDynamicProvider(p); err != nil {
		t.Fatalf("disabled without credential = %v, want nil", err)
	}
}

func TestBuildDynamicPricing(t *testing.T) {
	pricing, err := BuildDynamicPricing("p", "m", map[string]float64{"input_per_million": 1.5})
	if err != nil {
		t.Fatalf("BuildDynamicPricing = %v, want nil", err)
	}
	if pricing.InputPerMillion != 1.5 || pricing.OutputPerMillion != 0 {
		t.Fatalf("pricing = %+v, want input 1.5", pricing)
	}
	if _, err := BuildDynamicPricing("p", "m", nil); err != nil {
		t.Fatalf("nil rates err = %v, want nil pricing", err)
	}
	for name, rates := range map[string]map[string]float64{
		"unknown key":  {"nope": 1},
		"negative":     {"input_per_million": -1},
		"empty object": {},
	} {
		if _, err := BuildDynamicPricing("p", "m", rates); err == nil {
			t.Errorf("%s: err = nil, want error", name)
		}
	}
}

func TestNormalizeAliasDefaults(t *testing.T) {
	a := NormalizeAlias(Alias{Name: "x", Algorithm: AlgorithmRoundRobin})
	if len(a.RetryStatusCodes) == 0 {
		t.Fatalf("retry codes not defaulted")
	}
	if a.SessionAffinity != nil {
		t.Fatalf("session affinity should stay nil when unset")
	}
	b := NormalizeAlias(Alias{
		Name:               "x",
		SessionAffinity:    &SessionAffinity{Headers: []string{}},
		EncryptedReasoning: &EncryptedReasoning{},
	})
	if len(b.SessionAffinity.Headers) == 0 {
		t.Fatalf("empty session headers not defaulted")
	}
	if b.EncryptedReasoning.OnCallerMismatch != EncryptedReasoningFail {
		t.Fatalf("on_caller_mismatch = %q, want fail", b.EncryptedReasoning.OnCallerMismatch)
	}
	if len(b.EncryptedReasoning.MatchMessages) == 0 {
		t.Fatalf("match messages not defaulted")
	}
	c := NormalizeAlias(Alias{
		Name:            "x",
		SessionAffinity: &SessionAffinity{Headers: []string{"X-Custom"}},
	})
	if c.SessionAffinity.Headers[0] != "x-custom" {
		t.Fatalf("headers not lowercased: %v", c.SessionAffinity.Headers)
	}
}

func TestDescribeProviderTypes(t *testing.T) {
	infos := DescribeProviderTypes()
	if len(infos) != 8 {
		t.Fatalf("len = %d, want 8", len(infos))
	}
	byType := map[ProviderType]ProviderTypeInfo{}
	for _, info := range infos {
		byType[info.Type] = info
	}
	if byType[ProviderTypeOpenAICompatible].RequiresBaseURL != true {
		t.Fatalf("openai-compatible must require base_url")
	}
	if byType[ProviderTypeGitHubCopilot].Credential != ProviderCredentialCopilotRef {
		t.Fatalf("copilot credential = %q", byType[ProviderTypeGitHubCopilot].Credential)
	}
	if byType[ProviderTypeOpenCodeZen].Credential != ProviderCredentialOptional {
		t.Fatalf("zen credential = %q", byType[ProviderTypeOpenCodeZen].Credential)
	}
	if !byType[ProviderTypeOpenCodeZen].ModelProtocolRequired || byType[ProviderTypeOpenAI].ModelProtocolRequired {
		t.Fatalf("model protocol requirement wrong")
	}
	if len(byType[ProviderTypeOpenCodeGo].Protocols) != 3 {
		t.Fatalf("go protocols = %v", byType[ProviderTypeOpenCodeGo].Protocols)
	}
	if len(byType[ProviderTypeAnthropic].SupportedCapabilities) != 2 {
		t.Fatalf("anthropic caps = %v", byType[ProviderTypeAnthropic].SupportedCapabilities)
	}
	if byType[ProviderTypeGitHubCopilot].SupportsHealthcheck {
		t.Fatalf("copilot must not support healthcheck")
	}
}

func TestValidateDynamicAlias(t *testing.T) {
	catalog := NewCatalog([]Provider{
		{
			Type:    ProviderTypeOpenAI,
			Name:    "openai",
			Enabled: true,
			APIKey:  "secret",
			Models:  []Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}},
		},
	}, nil, nil)
	ok := Alias{Name: "fast", Algorithm: AlgorithmRoundRobin, Targets: []AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}}}
	if err := ValidateDynamicAlias(NormalizeAlias(ok), catalog); err != nil {
		t.Fatalf("valid alias = %v, want nil", err)
	}
	bad := Alias{Name: "fast", Algorithm: AlgorithmRoundRobin, Targets: []AliasTarget{{Provider: "openai", Model: "missing"}}}
	if err := ValidateDynamicAlias(bad, catalog); err == nil {
		t.Fatalf("bad target err = nil, want error")
	}
	badAlgo := Alias{Name: "fast", Algorithm: "random", Targets: []AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}}}
	if err := ValidateDynamicAlias(badAlgo, catalog); err == nil {
		t.Fatalf("bad algorithm err = nil, want error")
	}
}
