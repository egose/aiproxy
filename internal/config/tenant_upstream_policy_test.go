package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func tenantPolicyConfig(origins string) string {
	return `
listener "http" "public" { address = "127.0.0.1:8080" }
auth "main" { mode = "none" }
database { url = "postgres://localhost/fixture" }
multi_tenancy {
  allow_public_registration = false
  ` + origins + `
}
provider "openai-compatible" "trusted" {
  base_url = "http://10.0.0.1/v1"
  allow_insecure_base_url = true
  api_key = "trusted-fixture"
  model "m" {}
}
`
}

func TestLoadTenantUpstreamOriginsHCLJSONAndConversion(t *testing.T) {
	for _, origins := range [][]string{nil, {}, {"https://api.example.com", "https://probe.example.com:8443"}} {
		raw, err := json.Marshal(origins)
		if err != nil {
			t.Fatal(err)
		}
		declaration := ""
		if origins != nil {
			declaration = "allowed_upstream_origins = " + string(raw)
		}
		source := []byte(tenantPolicyConfig(declaration))
		rt, err := Load(source, "tenant-policy.hcl")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(rt.MultiTenancy.AllowedUpstreamOrigins, origins) && len(origins) != 0 {
			t.Fatalf("loaded approvals = %v, want %v", rt.MultiTenancy.AllowedUpstreamOrigins, origins)
		}
		trusted, ok := rt.Catalog.Provider("trusted")
		if !ok || trusted.OutboundPolicy != nil || !trusted.AllowInsecureBaseURL || trusted.APIKey != "trusted-fixture" {
			t.Fatal("tenant approval configuration changed trusted HCL behavior")
		}
		converted, from, to, err := Convert(source, "tenant-policy.hcl", false)
		if err != nil || from != FormatHCL || to != FormatJSON {
			t.Fatalf("HCL to JSON = %s/%s, %v", from, to, err)
		}
		jsonRuntime, err := Load(converted, "tenant-policy.json")
		if err != nil || !reflect.DeepEqual(rt.MultiTenancy, jsonRuntime.MultiTenancy) {
			t.Fatalf("JSON approvals differ: %v", err)
		}
		roundTrip, from, to, err := Convert(converted, "tenant-policy.json", false)
		if err != nil || from != FormatJSON || to != FormatHCL {
			t.Fatalf("JSON to HCL = %s/%s, %v", from, to, err)
		}
		roundRuntime, err := Load(roundTrip, "round-trip.hcl")
		if err != nil || !reflect.DeepEqual(rt.MultiTenancy, roundRuntime.MultiTenancy) {
			t.Fatalf("converted approvals differ: %v", err)
		}
	}
	t.Setenv("TENANT_ORIGIN", "https://api.example.com")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hcl")
	source := tenantPolicyConfig(`allowed_upstream_origins = [env("TENANT_ORIGIN")]`)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	rt, err := LoadFile(path)
	if err != nil || len(rt.MultiTenancy.AllowedUpstreamOrigins) != 1 || rt.MultiTenancy.AllowedUpstreamOrigins[0] != "https://api.example.com" {
		t.Fatalf("file env approval = %v", err)
	}
}

func TestRejectInvalidTenantUpstreamApprovalConfiguration(t *testing.T) {
	for _, declaration := range []string{
		`["http://api.example.com"]`, `["https://api.example.com/v1"]`, `["https://api.example.com?"]`,
		`["https://user@api.example.com"]`, `["https://*.example.com"]`, `["https://localhost"]`,
		`["https://127.0.0.1"]`, `["https://169.254.169.254"]`, `["https://[::1]"]`,
		`["https://[2001:db8::1]"]`, `["https://api.example.com", "https://API.example.com:443"]`,
		`"https://api.example.com"`, `[true]`, `[{}]`,
	} {
		t.Run(declaration, func(t *testing.T) {
			hcl := tenantPolicyConfig("allowed_upstream_origins = " + declaration)
			if _, err := Load([]byte(hcl), "invalid.hcl"); err == nil {
				t.Fatal("invalid HCL approvals accepted")
			}
			if _, _, _, err := Convert([]byte(hcl), "invalid.hcl", false); err == nil {
				t.Fatal("conversion accepted invalid approvals")
			}
			source := fmt.Sprintf(`{"listener":{"http":{"public":{"address":"127.0.0.1:8080"}}},"auth":{"main":{"mode":"none"}},"database":{"url":"postgres://localhost/fixture"},"multi_tenancy":{"allowed_upstream_origins":%s}}`, declaration)
			if _, err := Load([]byte(source), "invalid.json"); err == nil {
				t.Fatal("invalid JSON approvals accepted")
			}
		})
	}
	tooMany := `allowed_upstream_origins = [` + strings.Repeat(`"https://api.example.com",`, 129) + `]`
	if _, err := Load([]byte(tenantPolicyConfig(tooMany)), "unbounded.hcl"); err == nil {
		t.Fatal("unbounded approval list accepted")
	}
}
