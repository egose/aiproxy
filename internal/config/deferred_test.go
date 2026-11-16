package config

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

const deferredTestAuth = `listener "http" "public" { address = ":8080" }
auth "main" {
  mode = "bearer_static"
  client "peer" { token = "peer-token" }
}
`

const deferredTestProvider = `provider "openai-compatible" "peer" {
  base_url = "https://peer.example/mount/v1"
  api_key = "upstream-key"
  model "chat" {}
}
`

func deferredRootSource(d Deferred) string {
	return fmt.Sprintf(`deferred {
  max_jobs = %d
  max_running = %d
  max_admissions = %d
  max_entries = %d
  max_memory_bytes = %d
  max_request_bytes = %d
  max_result_bytes = %d
  max_work_bytes = %d
  per_owner_jobs = %d
  per_owner_running = %d
  max_control_inflight = %d
  execution_timeout = %q
  retention = %q
  control_timeout = %q
  finalization_timeout = %q
}
`, d.MaxJobs, d.MaxRunning, d.MaxAdmissions, d.MaxEntries, d.MaxMemoryBytes,
		d.MaxRequestBytes, d.MaxResultBytes, d.MaxWorkBytes, d.PerOwnerJobs,
		d.PerOwnerRunning, d.MaxControlInflight, d.ExecutionTimeout.String(),
		d.Retention.String(), d.ControlTimeout.String(), d.FinalizationTimeout.String())
}

func TestLoadDeferredDefaultsAndCallerOnly(t *testing.T) {
	anonymous := "listener \"http\" \"public\" { address = \":8080\" }\nauth \"main\" { mode = \"none\" }\n"
	for _, opt := range []string{"", "  deferred_transport {}\n"} {
		source := anonymous + strings.Replace(deferredTestProvider, `  model "chat" {}`, opt+`  model "chat" {}`, 1)
		rt, err := Load([]byte(source), "test.hcl")
		if err != nil {
			t.Fatal(err)
		}
		if rt.Deferred != DefaultDeferred() || rt.Deferred.Enabled {
			t.Fatalf("absent root lost disabled usable defaults: %+v", rt.Deferred)
		}
		p := testProvider(t, rt, "peer")
		if (p.DeferredTransport != nil) != (opt != "") || p.DeferredReceiver != nil {
			t.Fatal("provider block presence was not preserved")
		}
		if opt != "" && *p.DeferredTransport != DefaultDeferredTransport() {
			t.Fatalf("transport defaults = %+v", p.DeferredTransport)
		}
	}
	rt, err := Load([]byte(deferredTestAuth+"deferred {}\n"+deferredTestProvider), "test.hcl")
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultDeferred()
	want.Enabled = true
	if rt.Deferred != want {
		t.Fatalf("root defaults = %+v, want %+v", rt.Deferred, want)
	}
}

func TestLoadDeferredOverridesAndReceiverDefaults(t *testing.T) {
	d := DefaultDeferred()
	d.MaxJobs, d.MaxRunning, d.MaxAdmissions, d.MaxEntries = 16, 4, 2, 32
	d.MaxMemoryBytes, d.MaxRequestBytes, d.MaxResultBytes, d.MaxWorkBytes = 128<<20, 2048, 64<<10, 8<<20
	d.PerOwnerJobs, d.PerOwnerRunning, d.MaxControlInflight = 3, 1, 4
	d.ExecutionTimeout, d.Retention, d.ControlTimeout, d.FinalizationTimeout = 30*time.Second, 10*time.Second, 100*time.Millisecond, 30*time.Second
	source := deferredTestAuth + deferredRootSource(d) + `provider "anthropic" "receiver" {
  api_key = "receiver-key"
  deferred_receiver {
    execution_timeout = "10s"
    max_result_bytes = 32768
  }
  model "chat" {}
}
provider "openai-compatible" "peer" {
  base_url = "https://peer.example/mount/v1"
  api_key = "upstream-key"
  deferred_transport {
    control_timeout = "30s"
    max_wait = "1s"
    poll_interval = "50ms"
    max_control_calls = 4
    max_control_retries = 0
  }
  model "chat" {}
  model "embedding" { capabilities = ["embeddings"] }
}
provider "gemini" "defaults" {
  api_key = "defaults-key"
  deferred_receiver {}
  model "chat" {}
}
`
	rt, err := Load([]byte(source), "test.hcl")
	if err != nil {
		t.Fatal(err)
	}
	d.Enabled = true
	if rt.Deferred != d {
		t.Fatalf("root overrides = %+v, want %+v", rt.Deferred, d)
	}
	if got := testProvider(t, rt, "receiver").DeferredReceiver; got == nil || *got != (DeferredReceiver{ExecutionTimeout: 10 * time.Second, MaxResultBytes: 32 << 10}) {
		t.Fatalf("narrowed receiver = %+v", got)
	}
	if got := testProvider(t, rt, "defaults").DeferredReceiver; got == nil || *got != (DeferredReceiver{ExecutionTimeout: d.ExecutionTimeout, MaxResultBytes: d.MaxResultBytes}) {
		t.Fatalf("inherited root receiver = %+v", got)
	}
	wantTransport := DeferredTransport{ControlTimeout: 30 * time.Second, MaxWait: time.Second, PollInterval: 50 * time.Millisecond, MaxControlCalls: 4, MaxControlRetries: 0}
	if got := testProvider(t, rt, "peer").DeferredTransport; got == nil || *got != wantTransport {
		t.Fatalf("transport overrides = %+v", got)
	}
}

func TestDeferredRootIntegerBounds(t *testing.T) {
	for _, field := range []struct {
		name string
		min  int64
		max  int64
		set  func(*Deferred, int64)
	}{
		{"max_jobs", 1, 4096, func(d *Deferred, n int64) { d.MaxJobs, d.MaxRunning = int(n), 1 }},
		{"max_running", 1, 256, func(d *Deferred, n int64) { d.MaxRunning = int(n) }},
		{"max_admissions", 1, 64, func(d *Deferred, n int64) { d.MaxAdmissions = int(n) }},
		{"max_entries", 1, 65536, func(d *Deferred, n int64) { d.MaxJobs, d.MaxRunning, d.MaxEntries = 1, 1, int(n) }},
		{"max_memory_bytes", 16 << 20, 4 << 30, func(d *Deferred, n int64) {
			d.MaxJobs, d.MaxRunning, d.MaxEntries, d.MaxMemoryBytes = 1, 1, 1, n
		}},
		{"max_request_bytes", 1 << 10, 8 << 20, func(d *Deferred, n int64) { d.MaxRequestBytes = n }},
		{"max_result_bytes", 1 << 10, 32 << 20, func(d *Deferred, n int64) { d.MaxResultBytes = n }},
		{"max_work_bytes", 4 << 20, 256 << 20, func(d *Deferred, n int64) { d.MaxWorkBytes = n }},
		{"per_owner_jobs", 1, 4096, func(d *Deferred, n int64) { d.PerOwnerJobs = int(n) }},
		{"per_owner_running", 1, 256, func(d *Deferred, n int64) { d.PerOwnerRunning = int(n) }},
		{"max_control_inflight", 4, 128, func(d *Deferred, n int64) { d.MaxControlInflight = int(n) }},
	} {
		for _, value := range []int64{field.min - 1, field.min, field.max, field.max + 1} {
			t.Run(field.name+"/"+strconv.FormatInt(value, 10), func(t *testing.T) {
				d := DefaultDeferred()
				d.MaxJobs, d.MaxRunning, d.MaxAdmissions, d.MaxEntries = 4096, 256, 1, 65536
				d.PerOwnerJobs, d.PerOwnerRunning, d.MaxControlInflight = 1, 1, 4
				d.MaxMemoryBytes, d.MaxRequestBytes, d.MaxResultBytes, d.MaxWorkBytes = 4<<30, 1024, 1024, 4<<20
				field.set(&d, value)
				rt, err := Load([]byte(deferredTestAuth+deferredRootSource(d)+deferredTestProvider), "test.hcl")
				valid := value >= field.min && value <= field.max
				if valid {
					d.Enabled = true
					if err != nil || rt.Deferred != d {
						t.Fatalf("inclusive bound did not round-trip: %v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), field.name) {
					t.Fatalf("invalid bound error = %v", err)
				}
			})
		}
	}
}

func TestDeferredRootDurationBounds(t *testing.T) {
	for _, field := range []struct {
		name string
		min  time.Duration
		max  time.Duration
		set  func(*Deferred, time.Duration)
	}{
		{"execution_timeout", time.Second, time.Hour, func(d *Deferred, n time.Duration) { d.ExecutionTimeout = n }},
		{"retention", 10 * time.Second, time.Hour, func(d *Deferred, n time.Duration) { d.Retention = n }},
		{"control_timeout", 100 * time.Millisecond, 30 * time.Second, func(d *Deferred, n time.Duration) { d.ControlTimeout = n }},
		{"finalization_timeout", 100 * time.Millisecond, 30 * time.Second, func(d *Deferred, n time.Duration) { d.FinalizationTimeout = n }},
	} {
		for _, value := range []time.Duration{field.min - 1, field.min, field.max, field.max + 1} {
			t.Run(field.name+"/"+value.String(), func(t *testing.T) {
				d := DefaultDeferred()
				field.set(&d, value)
				rt, err := Load([]byte(deferredTestAuth+deferredRootSource(d)+deferredTestProvider), "test.hcl")
				valid := value >= field.min && value <= field.max
				if valid {
					d.Enabled = true
					if err != nil || rt.Deferred != d {
						t.Fatalf("inclusive duration did not round-trip: %v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), field.name) {
					t.Fatalf("invalid duration error = %v", err)
				}
			})
		}
	}
}

func TestDeferredMemoryAndRelationships(t *testing.T) {
	d := DefaultDeferred()
	required, err := deferredMinimumMemory(d)
	if err != nil {
		t.Fatal(err)
	}
	if required >= d.MaxMemoryBytes || int64(d.MaxRunning)*(d.MaxRequestBytes+d.MaxResultBytes+d.MaxWorkBytes) <= d.MaxMemoryBytes {
		t.Fatal("fixture must allow fewer memory-resident jobs than max_running")
	}
	d.MaxMemoryBytes = required
	if err := ValidateDeferred(d); err != nil {
		t.Fatalf("exact sufficient memory rejected: %v", err)
	}
	d.MaxMemoryBytes--
	if err := ValidateDeferred(d); err == nil || !strings.Contains(err.Error(), "admission scratch") {
		t.Fatalf("insufficient reservation memory accepted: %v", err)
	}
	for _, field := range []string{
		"max_running = 65", "max_entries = 63", "per_owner_jobs = 65", "per_owner_running = 9",
		"max_jobs = 0", "max_running = -1", "max_memory_bytes = 9223372036854775808",
		"max_request_bytes = 1.5", `execution_timeout = "9223372036854775808ns"`,
		`execution_timeout = ""`, `retention = "0s"`, `control_timeout = "-1s"`,
	} {
		source := deferredTestAuth + "deferred {\n" + field + "\n}\n" + deferredTestProvider
		if _, err := Load([]byte(source), "test.hcl"); err == nil {
			t.Fatalf("invalid value accepted: %s", field)
		}
	}
}

func TestDeferredTransportBounds(t *testing.T) {
	for _, field := range []struct {
		name string
		good []string
		bad  []string
	}{
		{"control_timeout", []string{`"100ms"`, `"30s"`}, []string{`"99ms"`, `"30.001s"`, `""`, `"0s"`, `"-1s"`}},
		{"max_wait", []string{`"1s"`, `"2h"`}, []string{`"999ms"`, `"2h1ns"`, `""`, `"0s"`}},
		{"poll_interval", []string{`"50ms"`, `"5s"`}, []string{`"49ms"`, `"5s1ns"`, `""`, `"-1s"`}},
		{"max_control_calls", []string{"4", "65536"}, []string{"3", "65537", "0", "-1", "1.25", "9223372036854775808"}},
		{"max_control_retries", []string{"0", "32"}, []string{"-1", "33", "1.25", "9223372036854775808"}},
	} {
		for _, values := range []struct {
			list []string
			bad  bool
		}{{field.good, false}, {field.bad, true}} {
			for _, value := range values.list {
				t.Run(field.name+"/"+value, func(t *testing.T) {
					block := "  deferred_transport {\n    " + field.name + " = " + value + "\n  }\n"
					source := deferredTestAuth + strings.Replace(deferredTestProvider, `  model "chat" {}`, block+`  model "chat" {}`, 1)
					_, err := Load([]byte(source), "test.hcl")
					if (err != nil) != values.bad {
						t.Fatalf("bound error = %v, invalid=%t", err, values.bad)
					}
				})
			}
		}
	}
}

func TestDeferredReceiverEligibilityAndNarrowing(t *testing.T) {
	for _, tc := range []struct {
		name     string
		root     string
		provider string
		bad      bool
	}{
		{"no root", "", `provider "openai" "p" {
  api_key = "k"
  deferred_receiver {}
  model "m" {}
}`, true},
		{"root auth none", "deferred {}\n", deferredTestProvider, true},
		{"equal root", "deferred {}\n", `provider "openai" "p" {
  api_key = "k"
  deferred_receiver {
    execution_timeout = "10m"
    max_result_bytes = 8388608
  }
  model "m" {}
}`, false},
		{"narrow minimum", "deferred {}\n", `provider "openai" "p" {
  api_key = "k"
  deferred_receiver {
    execution_timeout = "1s"
    max_result_bytes = 1024
  }
  model "m" {}
}`, false},
		{"execution expands", "deferred {}\n", `provider "openai" "p" {
  api_key = "k"
  deferred_receiver { execution_timeout = "10m1ns" }
  model "m" {}
}`, true},
		{"output expands", "deferred {}\n", `provider "openai" "p" {
  api_key = "k"
  deferred_receiver { max_result_bytes = 8388609 }
  model "m" {}
}`, true},
		{"output too small", "deferred {}\n", `provider "openai" "p" {
  api_key = "k"
  deferred_receiver { max_result_bytes = 1023 }
  model "m" {}
}`, true},
		{"execution zero", "deferred {}\n", `provider "openai" "p" {
  api_key = "k"
  deferred_receiver { execution_timeout = "0s" }
  model "m" {}
}`, true},
		{"disabled valid", "deferred {}\n", `provider "openai" "p" {
  enabled = false
  deferred_receiver {}
  model "m" {}
}`, false},
		{"disabled invalid", "deferred {}\n", `provider "openai" "p" {
  enabled = false
  deferred_receiver { max_result_bytes = 0 }
  model "m" {}
}`, true},
		{"both modes", "deferred {}\n", `provider "openai-compatible" "p" {
  api_key = "k"
  base_url = "https://peer.example/v1"
  deferred_receiver {}
  deferred_transport {}
  model "m" {}
}`, true},
		{"receiver forwarding", "deferred {}\n", `provider "openai" "p" {
  forward_authorization = true
  deferred_receiver {}
  model "m" {}
}`, true},
		{"caller forwarding", "", `provider "openai-compatible" "p" {
  base_url = "https://peer.example/v1"
  forward_authorization = true
  deferred_transport {}
  model "m" {}
}`, true},
		{"caller requires key when disabled", "", `provider "openai-compatible" "p" {
  base_url = "https://peer.example/v1"
  enabled = false
  deferred_transport {}
  model "m" {}
}`, true},
		{"caller wrong type disabled", "", `provider "openai" "p" {
  api_key = "k"
  enabled = false
  deferred_transport {}
  model "m" {}
}`, true},
		{"no chat", "deferred {}\n", `provider "openai" "p" {
  api_key = "k"
  deferred_receiver {}
  model "m" { capabilities = ["embeddings"] }
}`, true},
		{"keyless zen chat", "deferred {}\n", `provider "opencode-zen" "p" {
  deferred_receiver {}
  model "m" { protocol = "chat" }
}`, false},
		{"zen responses only", "deferred {}\n", `provider "opencode-zen" "p" {
  deferred_receiver {}
  model "m" { protocol = "responses" }
}`, true},
		{"translated zen messages", "deferred {}\n", `provider "opencode-zen" "p" {
  deferred_receiver {}
  model "m" { protocol = "messages" }
}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := deferredTestAuth
			if tc.name == "root auth none" {
				auth = "listener \"http\" \"public\" { address = \":8080\" }\nauth \"main\" { mode = \"none\" }\n"
			}
			_, err := Load([]byte(auth+tc.root+tc.provider), "test.hcl")
			if (err != nil) != tc.bad {
				t.Fatalf("eligibility error = %v, invalid=%t", err, tc.bad)
			}
		})
	}
}

func TestDeferredDerivativesUseLocalOptInsOnly(t *testing.T) {
	for _, baseOpt := range []string{
		"deferred_transport { max_wait = \"7m\" }",
		"deferred_receiver {\n    execution_timeout = \"30s\"\n    max_result_bytes = 2048\n  }",
	} {
		for _, localOpt := range []string{"", "deferred_transport {}", "deferred_receiver {}"} {
			t.Run(baseOpt+"/"+localOpt, func(t *testing.T) {
				source := deferredTestAuth + "deferred {}\n" + `provider "openai-compatible" "child" {
  extends = "peer"
  api_key = "child-key"
  ` + localOpt + "\n}\n" + strings.Replace(deferredTestProvider, `  model "chat" {}`, "  "+baseOpt+"\n"+`  model "chat" {}`, 1)
				rt, err := Load([]byte(source), "test.hcl")
				if err != nil {
					t.Fatal(err)
				}
				child := testProvider(t, rt, "child")
				if child.APIKey != "child-key" || child.BaseURL != "https://peer.example/mount/v1" {
					t.Fatal("local credential or inherited endpoint changed")
				}
				if (child.DeferredTransport != nil) != strings.HasPrefix(localOpt, "deferred_transport") || (child.DeferredReceiver != nil) != strings.HasPrefix(localOpt, "deferred_receiver") {
					t.Fatal("base opt-in survived without a matching local declaration")
				}
				if child.DeferredTransport != nil && *child.DeferredTransport != DefaultDeferredTransport() {
					t.Fatal("local caller block inherited base limits")
				}
				if child.DeferredReceiver != nil && *child.DeferredReceiver != (DeferredReceiver{ExecutionTimeout: rt.Deferred.ExecutionTimeout, MaxResultBytes: rt.Deferred.MaxResultBytes}) {
					t.Fatal("local receiver block inherited base limits instead of root defaults")
				}
				jsonSource, _, _, err := Convert([]byte(source), "test.hcl", false)
				if err != nil {
					t.Fatal(err)
				}
				jsonRT, err := Load(jsonSource, "test.json")
				if err != nil || !reflect.DeepEqual(testProvider(t, jsonRT, "child"), child) {
					t.Fatalf("derived JSON local opt-in mismatch: %v", err)
				}
			})
		}
	}
}

func TestDeferredJSONPresenceAndUnsupportedAttributes(t *testing.T) {
	const head = `{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"bearer_static","client":{"peer":{"token":"peer-token"}}}},`
	const provider = `"provider":{"openai-compatible":{"peer":{"base_url":"https://peer.example/v1","api_key":"k","model":{"m":{}}`
	for _, field := range []string{"deferred", "deferred_transport", "deferred_receiver"} {
		for _, value := range []string{"null", "false", `""`, "[]", "[null]", "[{},{}]", `{"enabled":false}`} {
			t.Run(field+"/"+value, func(t *testing.T) {
				var source string
				if field == "deferred" {
					source = head + `"deferred":` + value + "," + provider + "}}}}"
				} else {
					source = head + `"deferred":{},` + provider + `,"` + field + `":` + value + "}}}}"
				}
				if _, err := Load([]byte(source), "test.json"); err == nil {
					t.Fatal("invalid declared block disappeared during loading")
				}
				if _, _, _, err := Convert([]byte(source), "test.json", false); err == nil {
					t.Fatal("invalid declared block disappeared during conversion")
				}
			})
		}
	}
	for _, declaration := range []string{
		`"deferred":{"max_jobs":null}`, `"deferred":{"execution_timeout":null}`,
		provider + `,"deferred_transport":{"max_control_retries":null}}}}`,
		provider + `,"deferred_receiver":{"max_result_bytes":null}}}}`,
	} {
		source := head + declaration
		if strings.HasPrefix(declaration, `"deferred"`) {
			source += "," + provider + "}}}"
		}
		source += "}"
		if _, err := Load([]byte(source), "test.json"); err == nil || !strings.Contains(err.Error(), "must not be null") {
			t.Fatalf("null deferred attribute was treated as omitted or fixture was malformed: %v", err)
		}
	}
	for _, declaration := range []string{
		"deferred { enabled = false }", "deferred = {}", "deferred \"named\" {}",
		"deferred {}\ndeferred {}",
	} {
		if _, err := Load([]byte(deferredTestAuth+declaration+"\n"+deferredTestProvider), "test.hcl"); err == nil {
			t.Fatalf("invalid HCL declaration accepted: %s", declaration)
		}
	}
}

func TestDeferredRoundTripInlineAndEnv(t *testing.T) {
	t.Setenv("DEFER_ROOT_EXECUTION", "1m")
	t.Setenv("DEFER_ROOT_REQUEST", "2048")
	t.Setenv("DEFER_CONTROL_TIMEOUT", "100ms")
	t.Setenv("DEFER_CALLS", "64")
	t.Setenv("DEFER_RETRIES", "0")
	source := deferredTestAuth + `deferred {
  execution_timeout = env("DEFER_ROOT_EXECUTION")
  max_request_bytes = env("DEFER_ROOT_REQUEST")
}
provider "openai-compatible" "peer" {
  api_key = "k"
  base_url = "https://peer.example/mount/v1"
  deferred_transport {
    control_timeout = env("DEFER_CONTROL_TIMEOUT")
    max_wait = "2m"
    poll_interval = "50ms"
    max_control_calls = env("DEFER_CALLS")
    max_control_retries = env("DEFER_RETRIES")
  }
  model "chat" {}
}
provider "gemini" "receiver" {
  api_key = "k"
  deferred_receiver {
    execution_timeout = "30s"
    max_result_bytes = 4096
  }
  model "chat" {}
}
`
	want, err := Load([]byte(source), "test.hcl")
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		jsonSource, _, _, err := Convert([]byte(source), "test.hcl", compact)
		if err != nil || strings.Contains(string(jsonSource), "env(") {
			t.Fatalf("HCL conversion did not resolve expressions: %v", err)
		}
		hclSource, _, _, err := Convert(jsonSource, "test.json", false)
		if err != nil {
			t.Fatal(err)
		}
		jsonEnvSource := strings.Replace(string(jsonSource), `"1m"`, `env("DEFER_ROOT_EXECUTION")`, 1)
		for _, candidate := range []string{source, string(jsonSource), string(hclSource), jsonEnvSource} {
			t.Setenv(ConfigEnvVar, candidate)
			got, err := LoadEnv()
			if err != nil || got.Deferred != want.Deferred || got.Catalog.ProviderCount() != want.Catalog.ProviderCount() {
				t.Fatalf("inline/round-trip mismatch: %v", err)
			}
			for _, provider := range want.Catalog.Providers() {
				if !reflect.DeepEqual(testProvider(t, got, provider.Name), provider) {
					t.Fatalf("inline/round-trip provider mismatch: %s", provider.Name)
				}
			}
		}
	}
}

func TestDeferredJSONSingletonBlocksAndDisabledCredentialRef(t *testing.T) {
	for _, block := range []string{"{}", "[{}]"} {
		source := `{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"bearer_static","client":{"peer":{"token":"peer-token"}}}},"deferred":` + block +
			`,"provider":{"openai-compatible":{"peer":{"base_url":"https://peer.example/v1","enabled":false,"api_key_ref":{"path":"/not-opened/keys.json","key":"peer"},"model":{"m":{}},"deferred_transport":` + block +
			`}},"openai":{"receiver":{"api_key":"k","model":{"m":{}},"deferred_receiver":` + block + `}}}}`
		rt, err := Load([]byte(source), "test.json")
		if err != nil {
			t.Fatal(err)
		}
		p := rt.Catalog.DisabledProviders()[0]
		if !rt.Deferred.Enabled || p.DeferredTransport == nil || p.APIKeyRef == nil || p.APIKeyRef.Resolved || testProvider(t, rt, "receiver").DeferredReceiver == nil {
			t.Fatal("singleton JSON blocks or disabled credential reference changed")
		}
		hclSource, _, _, err := Convert([]byte(source), "test.json", false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Load(hclSource, "test.hcl"); err != nil {
			t.Fatalf("singleton blocks did not convert: %v", err)
		}
	}
}

func TestDeferredConversionRejectsFractionalLimitsBeforeNumericRounding(t *testing.T) {
	for _, source := range []string{
		deferredTestAuth + "deferred { max_jobs = 64.0000000000000000000000001 }\n" + deferredTestProvider,
		deferredTestAuth + "deferred { max_request_bytes = 1048576.0000000000000000000000001 }\n" + deferredTestProvider,
		deferredTestAuth + strings.Replace(deferredTestProvider, `  model "chat" {}`, "  deferred_transport { max_control_calls = 4096.0000000000000000000000001 }\n"+`  model "chat" {}`, 1),
		`{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"bearer_static","client":{"peer":{"token":"peer-token"}}}},"deferred":{"max_jobs":64.0000000000000000000000001}}`,
	} {
		if _, err := Load([]byte(source), "test.hcl"); err == nil {
			t.Fatal("fractional limit unexpectedly loaded")
		}
		if _, _, _, err := Convert([]byte(source), "test.hcl", false); err == nil {
			t.Fatal("conversion rounded an invalid fractional limit into a valid integer")
		}
	}
}

func TestDeferredRootRemovalKeepsCallerOnlyConfigValid(t *testing.T) {
	caller := strings.Replace(deferredTestProvider, `  model "chat" {}`, "  deferred_transport {}\n"+`  model "chat" {}`, 1)
	receiver := `provider "openai" "receiver" {
  api_key = "k"
  deferred_receiver {}
  model "chat" {}
}
`
	before := deferredTestAuth + "deferred {}\n" + caller + receiver
	if _, err := Load([]byte(before), "test.hcl"); err != nil {
		t.Fatal(err)
	}
	after := strings.Replace(before, "deferred {}\n", "", 1)
	if _, err := Load([]byte(after), "test.hcl"); err == nil {
		t.Fatal("receiver survived removal of its required root policy")
	}
	after = strings.Replace(after, "  deferred_receiver {}\n", "", 1)
	rt, err := Load([]byte(after), "test.hcl")
	if err != nil || rt.Deferred.Enabled || rt.Deferred != DefaultDeferred() || testProvider(t, rt, "peer").DeferredTransport == nil {
		t.Fatalf("caller-only config failed after valid root removal: %v", err)
	}
}

func TestDeferredCallerBaseURLRules(t *testing.T) {
	for _, base := range []string{
		"https://peer.example/mount/v1", "https://peer.example/v1/v1",
		"https://peer.example/mount/", "https://peer.example/a%20b/v1",
		"http://127.0.0.1:8080/mount/v1",
	} {
		source := strings.Replace(deferredTestProvider, `"https://peer.example/mount/v1"`, strconv.Quote(base), 1)
		source = strings.Replace(source, `  model "chat" {}`, "  deferred_transport {}\n"+`  model "chat" {}`, 1)
		rt, err := Load([]byte(deferredTestAuth+source), "test.hcl")
		if err != nil || testProvider(t, rt, "peer").BaseURL != base {
			t.Fatalf("valid caller base URL changed/rejected: %s %v", base, err)
		}
	}
	for _, suffix := range []string{
		"/v1?query=value", "/v1?", "/v1#fragment", "/v1#",
		"/mount/./v1", "/mount/../v1", "/mount/%2e/v1", "/mount/%2E%2e/v1",
		"/mount%2fv1", "/mount%2Fv1", "/mount%5cv1", "/mount%5Cv1",
		"/mount\\name/v1", "/mount%252fv1", "/mount/%252e%252e/v1",
	} {
		t.Run(suffix, func(t *testing.T) {
			ordinary := strings.Replace(deferredTestProvider, `"https://peer.example/mount/v1"`, strconv.Quote("https://peer.example"+suffix), 1)
			if _, err := Load([]byte(deferredTestAuth+ordinary), "test.hcl"); err != nil {
				t.Fatalf("ordinary URL rules changed: %v", err)
			}
			caller := strings.Replace(ordinary, `  model "chat" {}`, "  deferred_transport {}\n"+`  model "chat" {}`, 1)
			if _, err := Load([]byte(deferredTestAuth+caller), "test.hcl"); err == nil {
				t.Fatal("ambiguous caller base URL accepted")
			}
		})
	}
}

func TestDeferredHeadersReservedWithoutOptIn(t *testing.T) {
	for _, header := range []string{"X-Aiproxy-Deferred-Version", "x-aiproxy-deferred-generation", "X-AIPROXY-DEFERRED-", "X-Aiproxy-Deferred"} {
		if !IsDeferredHeader(header) || ValidateForwardHeaders([]string{header}, "root") == nil {
			t.Fatalf("private header allowed: %s", header)
		}
		source := deferredTestAuth + "forward_headers = [" + strconv.Quote(header) + "]\n" + deferredTestProvider
		if _, err := Load([]byte(source), "test.hcl"); err == nil {
			t.Fatal("root forwarded private protocol header")
		}
		source = deferredTestAuth + strings.Replace(deferredTestProvider, `  model "chat" {}`, "  forward_headers = ["+strconv.Quote(header)+"]\n"+`  model "chat" {}`, 1)
		if _, err := Load([]byte(source), "test.hcl"); err == nil {
			t.Fatal("provider forwarded private protocol header")
		}
	}
	for _, header := range []string{"x-aiproxy-deferredx", "x-aiproxy-defer", "x-trace-id"} {
		if IsDeferredHeader(header) || ValidateForwardHeaders([]string{header}, "root") != nil {
			t.Fatalf("unrelated header rejected: %s", header)
		}
	}
}

func TestDeferredCatalogClonesOptInsAndManagedRejectsPresence(t *testing.T) {
	transport := DefaultDeferredTransport()
	p := Provider{
		Type: ProviderTypeOpenAICompatible, Name: "p", BaseURL: "https://peer.example/v1",
		APIKey: "k", Models: []Model{{Name: "m", UpstreamName: "m"}}, Enabled: true,
		DeferredTransport: &transport,
	}
	catalog := NewCatalog([]Provider{p}, nil, nil)
	p.DeferredTransport.MaxWait = time.Second
	got, _ := catalog.Provider("p")
	if got.DeferredTransport.MaxWait != 15*time.Minute {
		t.Fatal("catalog retained the caller's mutable opt-in pointer")
	}
	got.DeferredTransport.MaxWait = 2 * time.Second
	again, _ := catalog.Provider("p")
	if again.DeferredTransport.MaxWait != 15*time.Minute {
		t.Fatal("catalog getter exposed its mutable opt-in pointer")
	}
	p.DeferredTransport = nil
	p.DeferredReceiver = &DeferredReceiver{ExecutionTimeout: time.Minute, MaxResultBytes: 4096}
	catalog = NewCatalog(nil, []Provider{p}, nil)
	disabled := catalog.DisabledProviders()
	disabled[0].DeferredReceiver.MaxResultBytes = 1024
	if catalog.DisabledProviders()[0].DeferredReceiver.MaxResultBytes != 4096 {
		t.Fatal("disabled catalog receiver policies are not isolated")
	}
	for _, enabled := range []bool{false, true} {
		for _, caller := range []bool{false, true} {
			p.Enabled = enabled
			p.DeferredTransport, p.DeferredReceiver = nil, nil
			if caller {
				p.DeferredTransport = &DeferredTransport{}
			} else {
				p.DeferredReceiver = &DeferredReceiver{}
			}
			if err := ValidateDynamicProvider(p); err == nil || !strings.Contains(err.Error(), "static-config-only") {
				t.Fatalf("managed empty opt-in pointer accepted: %v", err)
			}
		}
	}
}
