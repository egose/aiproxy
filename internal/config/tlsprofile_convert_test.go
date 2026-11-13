package config

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertTLSProfilesResolveReferencesForRelocation(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, compact := range []bool{false, true} {
			t.Run(format+"/"+map[bool]string{false: "pretty", true: "compact"}[compact], func(t *testing.T) {
				dir := t.TempDir()
				file := filepath.Join(dir, "profiles", "bun.json")
				tlsTestFile(t, file, tlsTestFixture(t))
				filename := filepath.Join(dir, "config", "config."+format)
				reference := "../profiles/./bun.json"
				source := tlsTestSource(t, format, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": reference}, map[string]any{})
				before := bytes.Clone(source)
				original, err := Load(source, filename)
				if err != nil {
					t.Fatal(err)
				}
				out, from, to, err := Convert(source, filename, compact)
				if err != nil || from != format || to == format {
					t.Fatalf("conversion = %s -> %s, %v", from, to, err)
				}
				if !bytes.Equal(before, source) {
					t.Fatal("conversion mutated its source")
				}
				relocated := filepath.Join(t.TempDir(), "relocated."+to)
				tlsTestFile(t, relocated, out)
				converted, err := LoadFile(relocated)
				if err != nil {
					t.Fatalf("relocated conversion lost its reference: %v", err)
				}
				p, child := testProvider(t, converted, "profiled"), testProvider(t, converted, "child")
				if p.TLSClientHelloSpecFile != file || p.TLSProfile.Identity() != testProvider(t, original, "profiled").TLSProfile.Identity() || child.TLSProfile.Identity() != p.TLSProfile.Identity() {
					t.Fatal("conversion did not emit the resolved absolute reference")
				}
				back, _, backFormat, err := Convert(out, relocated, false)
				if err != nil || backFormat != format {
					t.Fatalf("reverse conversion = %v", err)
				}
				if _, err := Load(back, ""); err != nil {
					t.Fatalf("absolute reference is not source-less usable: %v", err)
				}
				if to == FormatJSON {
					var root map[string]any
					if err := json.Unmarshal(out, &root); err != nil {
						t.Fatal(err)
					}
					child := root["provider"].(map[string]any)["openai"].(map[string]any)["child"].(map[string]any)
					if _, exists := child["tls_client_hello"]; exists {
						t.Fatal("conversion emitted a prohibited local derived selector")
					}
					if _, exists := child["tls_client_hello_spec_file"]; exists {
						t.Fatal("conversion emitted a prohibited local derived reference")
					}
				}
			})
		}
	}
}

func TestConvertTLSProfilesRejectInlineRelativeAndInvalidDeclarations(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bun.json")
	tlsTestFile(t, file, tlsTestFixture(t))
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, filename := range []string{"", EnvConfigFilename} {
			source := tlsTestSource(t, format, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "bun.json"}, nil)
			if _, _, _, err := Convert(source, filename, false); err == nil || !strings.Contains(err.Error(), "absolute reference") {
				t.Fatalf("source-less conversion resolved relative reference: %v", err)
			}
			source = tlsTestSource(t, format, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": file}, nil)
			if _, _, _, err := Convert(source, filename, false); err != nil {
				t.Fatalf("absolute inline conversion rejected: %v", err)
			}
		}
		for _, attributes := range []map[string]any{
			{"tls_client_hello": ""},
			{"tls_client_hello": nil},
			{"tls_client_hello": "firefox"},
			{"tls_client_hello": "native", "tls_client_hello_spec_file": ""},
			{"tls_client_hello": "custom", "tls_client_hello_spec_file": nil},
		} {
			if _, _, _, err := Convert(tlsTestSource(t, format, attributes, nil), filepath.Join(dir, "config."+format), false); err == nil {
				t.Fatal("conversion accepted invalid TLS attributes")
			}
		}
		for _, field := range []string{"tls_client_hello", "tls_client_hello_spec_file"} {
			source := tlsTestSource(t, format, nil, map[string]any{field: ""})
			if _, _, _, err := Convert(source, "config."+format, false); err == nil || !strings.Contains(err.Error(), "derived provider cannot declare "+field) {
				t.Fatalf("conversion ignored explicit derived declaration: %v", err)
			}
		}
	}
}

func TestConvertTLSProfilesResolvesConfigEnvCalls(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "profiles", "bun.json")
	tlsTestFile(t, file, tlsTestFixture(t))
	t.Setenv("TLS_PROFILE_TEST_SELECTOR", "custom")
	t.Setenv("TLS_PROFILE_TEST_SPEC", "profiles/./bun.json")
	source := []byte(`
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "profiled" {
  api_key = "test-key"
  tls_client_hello = env("TLS_PROFILE_TEST_SELECTOR")
  tls_client_hello_spec_file = env("TLS_PROFILE_TEST_SPEC")
  model "m" {}
}
`)
	out, _, _, err := Convert(source, filepath.Join(dir, "config.hcl"), false)
	if err != nil || bytes.Contains(out, []byte("env(")) {
		t.Fatalf("conversion env expansion = %v", err)
	}
	rt, err := Load(out, "")
	if err != nil || testProvider(t, rt, "profiled").TLSClientHelloSpecFile != file {
		t.Fatalf("converted env reference was not resolved: %v", err)
	}
}

func TestConvertTLSProfilesPreservesBuiltinSelectorAndOmission(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, attributes := range []map[string]any{nil, {"tls_client_hello": "native"}, {"tls_client_hello": "chrome"}, {"tls_client_hello": "safari-16.0"}} {
			source := tlsTestSource(t, format, attributes, map[string]any{})
			out, _, to, err := Convert(source, "config."+format, false)
			if err != nil {
				t.Fatal(err)
			}
			rt, err := Load(out, "config."+to)
			if err != nil {
				t.Fatal(err)
			}
			p := testProvider(t, rt, "profiled")
			selector, present := attributes["tls_client_hello"]
			if p.HasTLSClientHello != present || present && p.TLSClientHello != selector {
				t.Fatal("conversion changed builtin declaration semantics")
			}
		}
	}
}
