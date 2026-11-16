package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

const deferredConfigureRoot = `listener "http" "public" { address = ":8080" }
auth "main" {
  mode = "bearer_static"
  client "peer" { token = "peer-token" }
}
deferred {
  execution_timeout = env("CLI_DEFER_EXECUTION")
  max_request_bytes = env("CLI_DEFER_REQUEST")
  retention = "1m"
  control_timeout = "100ms"
}
`

const deferredConfigureCaller = `provider "openai-compatible" "peer" {
  base_url = "https://peer.example/mount/v1"
  api_key = env("CLI_DEFER_KEY")
  deferred_transport {
    control_timeout = env("CLI_DEFER_CONTROL")
    max_wait = "2m"
    poll_interval = "100ms"
    max_control_calls = 64
    max_control_retries = 0
  }
  model "m" {}
}
`

func deferredConfigureFixture(t *testing.T, extra string) string {
	t.Helper()
	t.Setenv("CLI_DEFER_EXECUTION", "1m")
	t.Setenv("CLI_DEFER_REQUEST", "2048")
	t.Setenv("CLI_DEFER_CONTROL", "100ms")
	t.Setenv("CLI_DEFER_KEY", "caller-key")
	path := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(path, []byte(deferredConfigureRoot+deferredConfigureCaller+extra), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigureDeferredNonInteractivePreservationAndDisable(t *testing.T) {
	path := deferredConfigureFixture(t, "")
	before, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := before.Catalog.Provider("peer")
	for _, flags := range [][]string{
		{"--display-name", "Updated"},
		{"--upstream-header-timeout", "120s"},
		{"--enabled=false"},
		{"--display-name", "Still disabled"},
		{"--enabled=true"},
	} {
		args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "peer"}
		if _, _, err := executeRootCommand("", append(args, flags...)...); err != nil {
			t.Fatal(err)
		}
		rt, err := config.LoadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		p, ok := rt.Catalog.Provider("peer")
		if !ok {
			p = rt.Catalog.DisabledProviders()[0]
		}
		if rt.Deferred != before.Deferred || !reflect.DeepEqual(p.DeferredTransport, want.DeferredTransport) || p.APIKey != "caller-key" {
			t.Fatal("partial/disabled edit changed deferred settings or configured credential")
		}
		data, _ := os.ReadFile(path)
		for _, preserved := range []string{deferredConfigureRoot, `control_timeout = env("CLI_DEFER_CONTROL")`, `api_key = env("CLI_DEFER_KEY")`, "max_control_retries = 0"} {
			if !bytes.Contains(data, []byte(preserved)) {
				t.Fatalf("edit lost complete source or environment spelling: %s", preserved)
			}
		}
	}
	if _, _, err := executeRootCommand("", "configure", "upstream", "--config", path, "--non-interactive", "--upstream-header-timeout", "90s"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte(deferredConfigureRoot)) {
		t.Fatal("unrelated root edit changed the deferred root block")
	}
}

func TestConfigureDeferredDerivativePreservesOnlyLocalBlocks(t *testing.T) {
	for _, local := range []string{"", `deferred_transport { max_wait = "3m" }`, `deferred_receiver {
    execution_timeout = "30s"
    max_result_bytes = 4096
  }`} {
		t.Run(local, func(t *testing.T) {
			extra := `provider "openai-compatible" "child" {
  extends = "peer"
  api_key = "child-key"
  ` + local + "\n}\n"
			path := deferredConfigureFixture(t, extra)
			if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "child", "--display-name", "Edited child"); err != nil {
				t.Fatal(err)
			}
			rt, err := config.LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			child, _ := rt.Catalog.Provider("child")
			if child.APIKey != "child-key" || (child.DeferredTransport != nil) != strings.HasPrefix(local, "deferred_transport") || (child.DeferredReceiver != nil) != strings.HasPrefix(local, "deferred_receiver") {
				t.Fatal("derivative edit inherited or discarded opt-ins")
			}
			data, _ := os.ReadFile(path)
			if local != "" && !bytes.Contains(data, []byte(local)) {
				t.Fatal("local derivative opt-in source changed")
			}
		})
	}
}

func TestConfigureDeferredInteractivePreservation(t *testing.T) {
	path := deferredConfigureFixture(t, "")
	answers := []string{"", "", "", "Interactive", "", "", "", "", "", "", "", "env_expression", "", "y", "y"}
	stdout, stderr, err := executeRootCommand(strings.Join(answers, "\n")+"\n", "configure", "provider", "--config", path, "--name", "peer")
	if err != nil {
		t.Fatalf("interactive edit: %v\n%s\n%s", err, stdout, stderr)
	}
	rt, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := rt.Catalog.Provider("peer")
	if p.DisplayName != "Interactive" || p.DeferredTransport == nil || p.DeferredTransport.MaxControlRetries != 0 || p.APIKey != "caller-key" {
		t.Fatal("interactive edit lost deferred settings")
	}
	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte(deferredConfigureRoot)) || !bytes.Contains(data, []byte(`control_timeout = env("CLI_DEFER_CONTROL")`)) {
		t.Fatal("interactive edit lost deferred root/provider environment source")
	}
}

func TestConfigureDeferredUnsupportedFlagsAndInvalidEditsDoNotWrite(t *testing.T) {
	path := deferredConfigureFixture(t, "")
	before, _ := os.ReadFile(path)
	for _, args := range [][]string{
		{"configure", "deferred", "--config", path},
		{"configure", "provider", "--config", path, "--deferred-transport"},
		{"configure", "provider", "--config", path, "--deferred-receiver=false"},
		{"configure", "upstream", "--config", path, "--deferred-max-jobs", "8"},
	} {
		if _, _, err := executeRootCommand("", args...); err == nil || !strings.Contains(err.Error(), "no CLI creation flags") {
			t.Fatalf("unsupported workflow needs a clear error: %v", err)
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("unsupported deferred workflow wrote the config")
		}
	}
	for _, flags := range [][]string{
		{"--base-url", "https://peer.example/v1?bad=true"},
		{"--model", "embedding", "--model-capabilities", "embedding=embeddings"},
		{"--forward-authorization"},
	} {
		args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "peer"}
		if _, _, err := executeRootCommand("", append(args, flags...)...); err == nil {
			t.Fatal("invalid deferred edit was accepted")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("invalid deferred edit wrote the config")
		}
	}
	if _, _, err := executeRootCommand("", "configure", "auth", "--config", path, "--non-interactive", "--mode", "none"); err == nil {
		t.Fatal("enabled root accepted anonymous authentication")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("auth edit invalidating receiver policy wrote the config")
	}
	jsonSource, _, _, err := config.Convert(before, path, false)
	if err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(jsonPath, jsonSource, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := executeRootCommand("", "configure", "provider", "--config", jsonPath, "--non-interactive", "--name", "peer", "--display-name", "Rejected"); err == nil || !strings.Contains(err.Error(), "convert JSON to HCL") {
		t.Fatalf("unsupported JSON editor needs a clear conversion error: %v", err)
	}
}
