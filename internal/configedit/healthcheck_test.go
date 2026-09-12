package configedit

import (
	"strings"
	"testing"
)

func TestRenderProviderBlockHealthcheck(t *testing.T) {
	sendAuth := true
	block := RenderProviderBlock(ProviderInput{
		ProviderType: "openai-compatible",
		Name:         "local",
		BaseURL:      "http://127.0.0.1:11434/v1",
		Credential:   ProviderCredentialInput{Mode: "inline", APIKeyValue: "k"},
		Healthcheck: &ProviderHealthcheckInput{
			Path:              "/health",
			Method:            "GET",
			ExpectedStatus:    "200",
			ExpectedBody:      "*",
			Interval:          "15s",
			Timeout:           "3s",
			FailureThreshold:  "3",
			SuccessThreshold:  "2",
			SendAuthorization: &sendAuth,
		},
		Models: []ProviderModelInput{{Name: "m"}},
	}, "/unused/keys.json")
	for _, want := range []string{
		"healthcheck {",
		`path = "/health"`,
		`method = "GET"`,
		"expected_status = 200",
		`expected_body = "*"`,
		`interval = "15s"`,
		`timeout = "3s"`,
		"failure_threshold = 3",
		"success_threshold = 2",
		"send_authorization = true",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("missing %q in:\n%s", want, block)
		}
	}
	if err := ValidateGeneratedConfig([]byte(block+minimalConfigTail(t)), "config.hcl"); err != nil {
		t.Fatalf("ValidateGeneratedConfig(): %v", err)
	}
}

func TestRenderProviderBlockHealthcheckMinimal(t *testing.T) {
	block := RenderProviderBlock(ProviderInput{
		ProviderType: "openai-compatible",
		Name:         "local",
		BaseURL:      "http://127.0.0.1:11434/v1",
		Credential:   ProviderCredentialInput{Mode: "inline", APIKeyValue: "k"},
		Healthcheck:  &ProviderHealthcheckInput{Path: "/health"},
		Models:       []ProviderModelInput{{Name: "m"}},
	}, "/unused/keys.json")
	if !strings.Contains(block, `path = "/health"`) {
		t.Fatalf("missing path in:\n%s", block)
	}
	for _, unwanted := range []string{"method =", "expected_status", "interval", "send_authorization"} {
		if strings.Contains(block, unwanted) {
			t.Errorf("minimal healthcheck should omit %q:\n%s", unwanted, block)
		}
	}
}

func TestRenderProviderBlockWithoutHealthcheckOmitsBlock(t *testing.T) {
	block := RenderProviderBlock(ProviderInput{
		ProviderType: "openai",
		Name:         "primary",
		Credential:   ProviderCredentialInput{Mode: "inline", APIKeyValue: "k"},
		Models:       []ProviderModelInput{{Name: "gpt-4o-mini"}},
	}, "/unused/keys.json")
	if strings.Contains(block, "healthcheck") {
		t.Fatalf("unexpected healthcheck block in:\n%s", block)
	}
}

func minimalConfigTail(t *testing.T) string {
	t.Helper()
	return `
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}
`
}
