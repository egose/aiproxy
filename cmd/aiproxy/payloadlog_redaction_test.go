package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureLoggingPreservesGuardrailRedaction(t *testing.T) {
	for _, value := range []string{"", "true", "false"} {
		for _, interactive := range []bool{false, true} {
			t.Run(fmt.Sprintf("value=%s/interactive=%t", value, interactive), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "config.hcl")
				attribute := ""
				if value != "" {
					attribute = "    redact_guardrail_bodies = " + value + "\n"
				}
				seed := "logging {\n  level = \"info\"\n  access_log = true\n  payload_log {\n    enabled = true\n    dir = \"payloads\"\n" + attribute + "  }\n}\n"
				if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
					t.Fatal(err)
				}
				args := []string{"configure", "logging", "--config", path, "--level", "warn"}
				input := "\n\n\n"
				if !interactive {
					args, input = append(args, "--non-interactive"), ""
				}
				_, stderr, err := executeRootCommand(input, args...)
				if err != nil {
					t.Fatalf("configure logging: %v stderr=%s", err, stderr)
				}
				updated, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(updated), `level = "warn"`) {
					t.Fatal("log level was not updated")
				}
				if value == "" {
					if strings.Contains(string(updated), "redact_guardrail_bodies") {
						t.Fatal("an absent redaction attribute must stay absent")
					}
				} else if !strings.Contains(string(updated), "redact_guardrail_bodies = "+value) {
					t.Fatal("changing log level discarded the declared redaction setting")
				}
			})
		}
	}
}
