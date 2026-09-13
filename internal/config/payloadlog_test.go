package config

import (
	"strings"
	"testing"
	"time"
)

func payloadTestConfig(block string) string {
	return `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
logging {
  level = "info"
  access_log = true
` + block + `
}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
}

func TestLoadPayloadLogDefaults(t *testing.T) {
	rt, err := Load([]byte(payloadTestConfig("")), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := rt.Logging.PayloadLog
	if p.Enabled {
		t.Fatalf("payload log should be disabled by default: %+v", p)
	}
	if p.Rotation != DefaultPayloadLogRotation {
		t.Fatalf("rotation = %q", p.Rotation)
	}
	if p.Retention != DefaultPayloadLogRetention {
		t.Fatalf("retention = %v", p.Retention)
	}
	if p.MaxBodyBytes != DefaultPayloadLogMaxBody {
		t.Fatalf("max_body_bytes = %d", p.MaxBodyBytes)
	}
}

func TestLoadPayloadLogFull(t *testing.T) {
	rt, err := Load([]byte(payloadTestConfig(`
  payload_log {
    enabled = true
    dir = "/tmp/payloads"
    rotation = "hourly"
    retention = "72h"
    max_body_bytes = 1024
  }
`)), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := rt.Logging.PayloadLog
	if !p.Enabled || p.Dir != "/tmp/payloads" {
		t.Fatalf("payload = %+v", p)
	}
	if p.Rotation != PayloadLogRotationHourly {
		t.Fatalf("rotation = %q", p.Rotation)
	}
	if p.Retention != 72*time.Hour {
		t.Fatalf("retention = %v", p.Retention)
	}
	if p.MaxBodyBytes != 1024 {
		t.Fatalf("max_body_bytes = %d", p.MaxBodyBytes)
	}
}

func TestLoadPayloadLogValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block string
		want  string
	}{
		{name: "missing dir", block: "payload_log {\n enabled = true\n}", want: "dir is required"},
		{name: "bad rotation", block: "payload_log {\n enabled = true\n dir = \"/tmp/x\"\n rotation = \"weekly\"\n}", want: "invalid rotation"},
		{name: "negative retention", block: "payload_log {\n enabled = true\n dir = \"/tmp/x\"\n retention = \"-1h\"\n}", want: "retention must not be negative"},
		{name: "negative max", block: "payload_log {\n enabled = true\n dir = \"/tmp/x\"\n max_body_bytes = -1\n}", want: "max_body_bytes must not be negative"},
		{name: "bad duration", block: "payload_log {\n enabled = true\n dir = \"/tmp/x\"\n retention = \"later\"\n}", want: "retention"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load([]byte(payloadTestConfig(tc.block)), "test.hcl")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoadPayloadLogZeroRetentionKeepsForever(t *testing.T) {
	rt, err := Load([]byte(payloadTestConfig("payload_log {\n enabled = true\n dir = \"/tmp/x\"\n retention = \"0s\"\n}")), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.Logging.PayloadLog.Retention != 0 {
		t.Fatalf("retention = %v", rt.Logging.PayloadLog.Retention)
	}
}

func TestConvertPayloadLogRoundTrip(t *testing.T) {
	hclCfg := payloadTestConfig(`
  payload_log {
    enabled = true
    dir = "/tmp/payloads"
    rotation = "hourly"
    retention = "72h"
    max_body_bytes = 1024
  }
`)
	jsonOut, _, _, err := Convert([]byte(hclCfg), "test.hcl", false)
	if err != nil {
		t.Fatalf("to JSON: %v", err)
	}
	hclOut, _, _, err := Convert(jsonOut, "test.json", false)
	if err != nil {
		t.Fatalf("to HCL: %v\n%s", err, jsonOut)
	}
	got, err := Load(hclOut, "test.hcl")
	if err != nil {
		t.Fatalf("load round-tripped: %v\n%s", err, hclOut)
	}
	p := got.Logging.PayloadLog
	if !p.Enabled || p.Dir != "/tmp/payloads" || p.Rotation != PayloadLogRotationHourly || p.Retention != 72*time.Hour || p.MaxBodyBytes != 1024 {
		t.Fatalf("payload = %+v", p)
	}
}
