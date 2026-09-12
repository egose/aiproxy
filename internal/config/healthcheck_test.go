package config

import (
	"strings"
	"testing"
	"time"
)

func healthcheckTestConfig(hc string) string {
	return `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai-compatible" "local" {
  base_url = "http://127.0.0.1:11434/v1"
  api_key = "k"
  model "m" {}
` + hc + `
}
`
}

func TestLoadProviderHealthcheckDefaults(t *testing.T) {
	rt, err := Load([]byte(healthcheckTestConfig(`
  healthcheck {
    path = "/health"
  }
`)), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := testProvider(t, rt, "local")
	if p.Healthcheck == nil {
		t.Fatalf("expected healthcheck block")
	}
	hc := p.Healthcheck
	if hc.Path != "/health" {
		t.Errorf("path = %q", hc.Path)
	}
	if hc.Method != "GET" {
		t.Errorf("method = %q, want GET", hc.Method)
	}
	if hc.ExpectedStatus != 200 {
		t.Errorf("expected_status = %d, want 200", hc.ExpectedStatus)
	}
	if hc.ExpectedBody != "*" {
		t.Errorf("expected_body = %q, want *", hc.ExpectedBody)
	}
	if hc.Interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", hc.Interval)
	}
	if hc.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", hc.Timeout)
	}
	if hc.FailureThreshold != 2 {
		t.Errorf("failure_threshold = %d, want 2", hc.FailureThreshold)
	}
	if hc.SuccessThreshold != 1 {
		t.Errorf("success_threshold = %d, want 1", hc.SuccessThreshold)
	}
	if hc.SendAuthorization {
		t.Errorf("send_authorization = true, want false")
	}
}

func TestLoadProviderHealthcheckFull(t *testing.T) {
	rt, err := Load([]byte(healthcheckTestConfig(`
  healthcheck {
    path = "/healthz"
    method = "HEAD"
    expected_status = 204
    expected_body = "*"
    interval = "15s"
    timeout = "3s"
    failure_threshold = 3
    success_threshold = 2
    send_authorization = true
  }
`)), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	hc := testProvider(t, rt, "local").Healthcheck
	if hc == nil {
		t.Fatalf("expected healthcheck block")
	}
	if hc.Method != "HEAD" || hc.ExpectedStatus != 204 || hc.Interval != 15*time.Second ||
		hc.Timeout != 3*time.Second || hc.FailureThreshold != 3 || hc.SuccessThreshold != 2 || !hc.SendAuthorization {
		t.Errorf("unexpected healthcheck: %+v", hc)
	}
}

func TestLoadProviderWithoutHealthcheckSkips(t *testing.T) {
	rt, err := Load([]byte(healthcheckTestConfig("")), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if hc := testProvider(t, rt, "local").Healthcheck; hc != nil {
		t.Errorf("expected nil healthcheck, got %+v", hc)
	}
}

func TestLoadProviderHealthcheckInvalid(t *testing.T) {
	for _, tc := range []struct {
		name    string
		block   string
		message string
	}{
		{"relative_path", "path = \"health\"\n", "must start with '/'"},
		{"absolute_url", "path = \"/foo://bar\"\n", "not a URL"},
		{"bad_method", "path = \"/health\"\n method = \"POST\"\n", "must be GET or HEAD"},
		{"bad_status", "path = \"/health\"\n expected_status = 99\n", "expected_status"},
		{"timeout_exceeds_interval", "path = \"/health\"\n interval = \"5s\"\n timeout = \"5s\"\n", "must be less than"},
		{"zero_failure_threshold", "path = \"/health\"\n failure_threshold = 0\n", "failure_threshold"},
		{"zero_success_threshold", "path = \"/health\"\n success_threshold = 0\n", "success_threshold"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load([]byte(healthcheckTestConfig("  healthcheck {\n    "+tc.block+"  }\n")), "test.hcl")
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected %q error, got %v", tc.message, err)
			}
		})
	}
}

func TestLoadProviderHealthcheckRejectedForCopilot(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  enabled = false
  model "gpt-5-mini" {}
  healthcheck {
    path = "/health"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "healthcheck is not supported by github-copilot") {
		t.Fatalf("expected copilot healthcheck error, got %v", err)
	}
}

func TestLoadDerivedProviderRejectsHealthcheck(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai-compatible" "base" {
  base_url = "http://127.0.0.1:11434/v1"
  api_key = "k"
  model "m" {}
}
provider "openai-compatible" "derived" {
  extends = "base"
  api_key = "k2"
  healthcheck {
    path = "/health"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "derived provider cannot declare healthcheck") {
		t.Fatalf("expected derived healthcheck error, got %v", err)
	}
}

func TestLoadDerivedProviderInheritsHealthcheck(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai-compatible" "base" {
  base_url = "http://127.0.0.1:11434/v1"
  api_key = "k"
  model "m" {}
  healthcheck {
    path = "/health"
  }
}
provider "openai-compatible" "derived" {
  extends = "base"
  api_key = "k2"
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	hc := testProvider(t, rt, "derived").Healthcheck
	if hc == nil || hc.Path != "/health" {
		t.Fatalf("expected inherited healthcheck, got %+v", hc)
	}
}

func TestConvertRoundTripPreservesHealthcheck(t *testing.T) {
	src := []byte(healthcheckTestConfig(`
  healthcheck {
    path = "/health"
    expected_body = "ok"
    interval = "15s"
    timeout = "3s"
  }
`))
	out, from, to, err := Convert(src, "test.hcl", false)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if from != FormatHCL || to != FormatJSON {
		t.Fatalf("from = %q to = %q", from, to)
	}
	back, from2, to2, err := Convert(out, "test.json", false)
	if err != nil {
		t.Fatalf("convert back: %v", err)
	}
	if from2 != FormatJSON || to2 != FormatHCL {
		t.Fatalf("from = %q to = %q", from2, to2)
	}
	rt, err := Load(back, "test.hcl")
	if err != nil {
		t.Fatalf("load converted: %v", err)
	}
	hc := testProvider(t, rt, "local").Healthcheck
	if hc == nil || hc.Path != "/health" || hc.ExpectedBody != "ok" || hc.Interval != 15*time.Second || hc.Timeout != 3*time.Second {
		t.Fatalf("healthcheck lost in round trip: %+v", hc)
	}
}
