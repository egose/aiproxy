package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/tlsprofile"
	utls "github.com/refraction-networking/utls"
)

func tlsTestFixture(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "tlsprofile", "testdata", "bun-1.4.4-fetch.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func tlsTestSource(t testing.TB, format string, attributes, childAttributes map[string]any) []byte {
	t.Helper()
	provider := map[string]any{"api_key": "test-key", "model": map[string]any{"m": map[string]any{}}}
	for name, value := range attributes {
		provider[name] = value
	}
	providers := map[string]any{"profiled": provider}
	if childAttributes != nil {
		child := map[string]any{"extends": "profiled", "api_key": "child-key"}
		for name, value := range childAttributes {
			child[name] = value
		}
		providers["child"] = child
	}
	root := map[string]any{
		"listener": map[string]any{"http": map[string]any{"public": map[string]any{"address": ":8080"}}},
		"auth":     map[string]any{"main": map[string]any{"mode": "none"}},
		"provider": map[string]any{"openai": providers},
	}
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if format == FormatJSON {
		return data
	}
	var out strings.Builder
	out.WriteString("listener \"http\" \"public\" { address = \":8080\" }\nauth \"main\" { mode = \"none\" }\n")
	for _, name := range []string{"profiled", "child"} {
		entry, exists := providers[name]
		if !exists {
			continue
		}
		fmt.Fprintf(&out, "provider \"openai\" %q {\n", name)
		values := entry.(map[string]any)
		for _, field := range sortedJSONKeys(values) {
			if field == "model" {
				out.WriteString("model \"m\" {}\n")
				continue
			}
			value, err := json.Marshal(values[field])
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&out, "%s = %s\n", field, value)
		}
		out.WriteString("}\n")
	}
	return []byte(out.String())
}

func tlsTestFile(t testing.TB, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadTLSProfileSelectors(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, selector := range append([]string{""}, tlsprofile.BuiltinSelectors()...) {
			t.Run(format+"/"+selector, func(t *testing.T) {
				attributes := map[string]any{}
				if selector != "" {
					attributes["tls_client_hello"] = selector
				}
				rt, err := Load(tlsTestSource(t, format, attributes, nil), "config."+format)
				if err != nil {
					t.Fatal(err)
				}
				p := testProvider(t, rt, "profiled")
				selection := selector
				if selection == "" {
					selection = "native"
				}
				want, _ := tlsprofile.ResolveBuiltin(selection)
				if p.TLSClientHello != selector || p.HasTLSClientHello != (selector != "") || p.HasTLSClientHelloSpecFile || p.TLSProfile.Identity() != want.Identity() || p.TLSClientHelloInherited {
					t.Fatal("declared and effective builtin profile metadata differ")
				}
			})
		}
	}
}

func TestLoadTLSProfileDeclarationErrors(t *testing.T) {
	cases := []struct {
		name       string
		attributes map[string]any
		want       string
	}{
		{"empty selector", map[string]any{"tls_client_hello": ""}, "tls_client_hello"},
		{"unknown selector", map[string]any{"tls_client_hello": "private-selector-value"}, "tls_client_hello"},
		{"wrong case", map[string]any{"tls_client_hello": "Chrome"}, "tls_client_hello"},
		{"deferred firefox", map[string]any{"tls_client_hello": "firefox-120"}, "tls_client_hello"},
		{"unsupported chrome version", map[string]any{"tls_client_hello": "chrome-155"}, "tls_client_hello"},
		{"null selector", map[string]any{"tls_client_hello": nil}, "tls_client_hello must be a non-null string"},
		{"numeric selector", map[string]any{"tls_client_hello": 771}, "tls_client_hello must be a non-null string"},
		{"boolean selector", map[string]any{"tls_client_hello": false}, "tls_client_hello must be a non-null string"},
		{"omitted selector with empty file", map[string]any{"tls_client_hello_spec_file": ""}, "tls_client_hello_spec_file"},
		{"native with file", map[string]any{"tls_client_hello": "native", "tls_client_hello_spec_file": "private-source-value"}, "cannot be declared"},
		{"chrome with empty file", map[string]any{"tls_client_hello": "chrome", "tls_client_hello_spec_file": ""}, "cannot be declared"},
		{"safari with file", map[string]any{"tls_client_hello": "safari", "tls_client_hello_spec_file": "private-source-value"}, "cannot be declared"},
		{"custom without file", map[string]any{"tls_client_hello": "custom"}, "tls_client_hello_spec_file"},
		{"custom empty file", map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": ""}, "tls_client_hello_spec_file"},
		{"null file", map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": nil}, "tls_client_hello_spec_file must be a non-null string"},
		{"numeric file", map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": 123}, "tls_client_hello_spec_file must be a non-null string"},
		{"controlled file", map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "private-source-value\x00"}, "control characters"},
		{"long file", map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": strings.Repeat("a", 4097)}, "4096 bytes"},
	}
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, tc := range cases {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				source := tlsTestSource(t, format, tc.attributes, nil)
				_, err := Load(source, "config."+format)
				if err == nil || !strings.Contains(err.Error(), `provider "profiled"`) || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "private-source-value") || strings.Contains(err.Error(), "private-selector-value") {
					t.Fatalf("error = %v, want provider/field %q without arbitrary data", err, tc.want)
				}
				if err := ValidateProviderTLSProfiles(source, "config."+format); err == nil {
					t.Fatal("TLS-only validation accepted invalid declarations")
				}
			})
		}
		for _, name := range []string{"tls_client_hello", "tls_client_hello_spec_file"} {
			source := tlsTestSource(t, format, nil, nil)
			if format == FormatJSON {
				var root map[string]any
				if err := json.Unmarshal(source, &root); err != nil {
					t.Fatal(err)
				}
				root[name] = "native"
				source, _ = json.Marshal(root)
			} else {
				source = append([]byte(name+" = \"native\"\n"), source...)
			}
			if _, err := Load(source, "config."+format); err == nil {
				t.Fatal("root TLS profile declaration must be rejected")
			}
		}
	}
}

func TestDerivedTLSProfilesInheritAndRejectDeclarations(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, selector := range []string{"", "native", "chrome", "safari-16.0", "custom"} {
			t.Run(format+"/"+selector, func(t *testing.T) {
				dir := t.TempDir()
				attributes := map[string]any{}
				if selector != "" {
					attributes["tls_client_hello"] = selector
				}
				if selector == "custom" {
					attributes["tls_client_hello_spec_file"] = "profiles/./bun.json"
					tlsTestFile(t, filepath.Join(dir, "profiles", "bun.json"), tlsTestFixture(t))
				}
				rt, err := Load(tlsTestSource(t, format, attributes, map[string]any{}), filepath.Join(dir, "config."+format))
				if err != nil {
					t.Fatal(err)
				}
				base, child := testProvider(t, rt, "profiled"), testProvider(t, rt, "child")
				if child.TLSClientHello != base.TLSClientHello || child.TLSClientHelloSpecFile != base.TLSClientHelloSpecFile || child.HasTLSClientHello != base.HasTLSClientHello || child.HasTLSClientHelloSpecFile != base.HasTLSClientHelloSpecFile || child.TLSProfile.Identity() != base.TLSProfile.Identity() || !child.TLSClientHelloInherited {
					t.Fatal("derived provider did not inherit the endpoint profile by value")
				}
				if !child.TLSProfile.IsNative() {
					spec, err := child.TLSProfile.Materialize()
					if err != nil {
						t.Fatal(err)
					}
					clear(spec.CipherSuites)
					baseSpec, err := testProvider(t, rt, "profiled").TLSProfile.Materialize()
					if err != nil || baseSpec.CipherSuites[0] == 0 {
						t.Fatal("derived materialization mutated base profile")
					}
				}
			})
		}
		for _, field := range []string{"tls_client_hello", "tls_client_hello_spec_file"} {
			for _, value := range []any{"", "native", "custom", "private-source-value", nil, false} {
				source := tlsTestSource(t, format, nil, map[string]any{field: value})
				_, err := Load(source, "config."+format)
				want := `provider "child": derived provider cannot declare ` + field
				if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "private-source-value") {
					t.Fatalf("derived %s = %v: %v", field, value, err)
				}
				if err := ValidateProviderTLSProfiles(source, "config."+format); err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("TLS-only derived validation = %v", err)
				}
			}
		}
	}
}

func TestCustomTLSProfileSourceResolutionAndImmutableReload(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "profiles", "bun.json")
			tlsTestFile(t, file, tlsTestFixture(t))
			filename := filepath.Join(dir, "config", "config."+format)
			reference := "../profiles/./bun.json"
			source := tlsTestSource(t, format, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": reference}, nil)
			tlsTestFile(t, filename, source)
			rt, err := LoadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			p := testProvider(t, rt, "profiled")
			if p.TLSClientHelloSpecFile != reference || !p.HasTLSClientHello || !p.HasTLSClientHelloSpecFile {
				t.Fatal("loading changed the declared spelling or presence")
			}
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			relativeFilename, err := filepath.Rel(cwd, filename)
			if err != nil {
				t.Fatal(err)
			}
			relative, err := Load(source, relativeFilename)
			if err != nil || testProvider(t, relative, "profiled").TLSProfile.Identity() != p.TLSProfile.Identity() {
				t.Fatalf("relative config filename resolution = %v", err)
			}
			changed := bytes.Replace(tlsTestFixture(t), []byte(`"http/1.1"`), []byte(`"h2", "http/1.1"`), 1)
			tlsTestFile(t, file, changed)
			candidate, err := LoadFile(filename)
			if err != nil || testProvider(t, candidate, "profiled").TLSProfile.Identity() == p.TLSProfile.Identity() {
				t.Fatalf("same-path changed content must change candidate identity: %v", err)
			}
			tlsTestFile(t, file, []byte(`{"extensions":[{"name":"private-content-value"}]}`))
			if _, err := LoadFile(filename); err == nil {
				t.Fatal("malformed replacement must reject the candidate")
			}
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			if err := Validate(rt); err != nil {
				t.Fatalf("runtime validation reopened removed profile: %v", err)
			}
			spec, err := p.TLSProfile.Materialize()
			if err != nil || !slices.Equal(spec.Extensions[6].(*utls.ALPNExtension).AlpnProtocols, []string{"http/1.1"}) || p.TLSProfile.Identity() != testProvider(t, rt, "profiled").TLSProfile.Identity() {
				t.Fatalf("old runtime lost immutable profile content: %v", err)
			}
			copied := NewCatalog(rt.Catalog.Providers(), nil, nil)
			copyProvider, _ := copied.Provider("profiled")
			if copyProvider.TLSProfile.Identity() != p.TLSProfile.Identity() {
				t.Fatal("catalog copy changed profile identity")
			}
		})
	}
}

func TestCustomTLSProfileSymlinksUseLexicalConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	realDir, linkDir := filepath.Join(dir, "real"), filepath.Join(dir, "links")
	realSpec, linkSpec := filepath.Join(realDir, "bun.json"), filepath.Join(linkDir, "bun.json")
	fixture := tlsTestFixture(t)
	changed := bytes.Replace(fixture, []byte(`"http/1.1"`), []byte(`"h2", "http/1.1"`), 1)
	tlsTestFile(t, realSpec, fixture)
	tlsTestFile(t, linkSpec, changed)
	source := tlsTestSource(t, FormatHCL, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "bun.json"}, nil)
	realConfig, linkConfig := filepath.Join(realDir, "config.hcl"), filepath.Join(linkDir, "config.hcl")
	tlsTestFile(t, realConfig, source)
	if err := os.Symlink(realConfig, linkConfig); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	rt, err := LoadFile(linkConfig)
	if err != nil {
		t.Fatal(err)
	}
	want, err := tlsprofile.ParseCustom(changed)
	if err != nil || testProvider(t, rt, "profiled").TLSProfile.Identity() != want.Identity() {
		t.Fatal("config symlink resolution used the physical target directory")
	}
	operatorLink := filepath.Join(linkDir, "operator-spec.json")
	if err := os.Symlink(realSpec, operatorLink); err != nil {
		t.Fatal(err)
	}
	rt, err = Load(tlsTestSource(t, FormatHCL, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "operator-spec.json"}, nil), linkConfig)
	if err != nil {
		t.Fatal(err)
	}
	want, _ = tlsprofile.ParseCustom(fixture)
	if testProvider(t, rt, "profiled").TLSProfile.Identity() != want.Identity() {
		t.Fatal("regular-file operator symlink was not followed")
	}
}

func TestCustomTLSProfileInlineAndSourceLess(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bun.json")
	tlsTestFile(t, file, tlsTestFixture(t))
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, filename := range []string{"", EnvConfigFilename} {
			for _, reference := range []string{"bun.json", file} {
				source := tlsTestSource(t, format, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": reference}, nil)
				_, err := Load(source, filename)
				if (err == nil) != filepath.IsAbs(reference) {
					t.Fatalf("source-less reference %s: %v", format, err)
				}
				if err != nil && !strings.Contains(err.Error(), "absolute reference") {
					t.Fatalf("wrong source-less error: %v", err)
				}
				if err := ValidateProviderTLSProfiles(source, filename); (err == nil) != filepath.IsAbs(reference) {
					t.Fatalf("TLS-only source-less validation: %v", err)
				}
			}
		}
		t.Setenv(ConfigEnvVar, string(tlsTestSource(t, format, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": file}, nil)))
		if _, err := LoadEnv(); err != nil {
			t.Fatal(err)
		}
		t.Setenv(ConfigEnvVar, string(tlsTestSource(t, format, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "bun.json"}, nil)))
		if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "absolute reference") {
			t.Fatalf("inline env relative source accepted: %v", err)
		}
	}
}

func TestTLSProfileReferenceBounds(t *testing.T) {
	for _, tc := range []struct {
		reference string
		valid     bool
	}{
		{"", false}, {"spec.json", true}, {strings.Repeat("a", 4096), true}, {strings.Repeat("a", 4097), false},
		{strings.Repeat("é", 2048), true}, {strings.Repeat("é", 2049), false}, {string([]byte{0xff}), false},
		{"a\x00b", false}, {"a\nb", false}, {"a\tb", false}, {"a\u0085b", false}, {"a\u007fb", false},
	} {
		if err := validateTLSProfileReference(tc.reference); (err == nil) != tc.valid {
			t.Fatalf("reference boundary length %d: %v", len(tc.reference), err)
		}
	}
}

func TestCustomTLSProfileFileErrorsAreBoundedAndRedacted(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "spec: invalid JSON"},
		{"truncated", []byte(`{"cipher_suites":`), "invalid JSON"},
		{"unknown field", append([]byte(`{"private-content-value":1,`), tlsTestFixture(t)[1:]...), "unknown field"},
		{"fixed hostname", bytes.Replace(tlsTestFixture(t), []byte(`{"name": "server_name"}`), []byte(`{"name":"server_name","hostname":"private-content-value"}`), 1), "extensions[0]: unknown field"},
		{"oversized", bytes.Repeat([]byte{' '}, tlsprofile.MaxSpecBytes+1), "65536 bytes"},
		{"invalid utf8", []byte{0xff}, "valid UTF-8"},
	} {
		file := filepath.Join(dir, "private-source-value-"+tc.name+".json")
		tlsTestFile(t, file, tc.data)
		_, err := Load(tlsTestSource(t, FormatHCL, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": file}, nil), filepath.Join(dir, "config.hcl"))
		if err == nil || !strings.Contains(err.Error(), `provider "profiled"`) || !strings.Contains(err.Error(), "tls_client_hello_spec_file") || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "private-source-value") || strings.Contains(err.Error(), "private-content-value") {
			t.Fatalf("redacted file error = %v, want %s", err, tc.want)
		}
	}
	for _, file := range []string{filepath.Join(dir, "private-source-value-missing"), dir} {
		_, err := Load(tlsTestSource(t, FormatHCL, map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": file}, nil), filepath.Join(dir, "config.hcl"))
		if err == nil || strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "private-source-value") {
			t.Fatalf("missing/nonregular error is not controlled: %v", err)
		}
	}
	file := filepath.Join(dir, "boundary.json")
	fixture := tlsTestFixture(t)
	tlsTestFile(t, file, append(fixture, bytes.Repeat([]byte{' '}, tlsprofile.MaxSpecBytes-len(fixture))...))
	if _, err := loadTLSProfile(file, ""); err != nil {
		t.Fatalf("exact file size bound rejected: %v", err)
	}
	if os.Geteuid() != 0 {
		if err := os.Chmod(file, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(file, 0o600) })
		if _, err := loadTLSProfile(file, ""); err == nil || !strings.Contains(err.Error(), "cannot open") || strings.Contains(err.Error(), dir) {
			t.Fatalf("unreadable file error = %v", err)
		}
	}
}

func TestCustomTLSProfileDoesNotExpandReferencesOrJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TLS_PROFILE_TEST_REFERENCE", "expanded.json")
	tlsTestFile(t, filepath.Join(dir, "expanded.json"), tlsTestFixture(t))
	for _, reference := range []string{"$TLS_PROFILE_TEST_REFERENCE", "~/expanded.json", "https://example.test/expanded.json"} {
		if _, err := loadTLSProfile(reference, filepath.Join(dir, "config.hcl")); err == nil {
			t.Fatal("reference was expanded")
		}
	}
	literal := filepath.Join(dir, "$TLS_PROFILE_TEST_REFERENCE")
	tlsTestFile(t, literal, tlsTestFixture(t))
	if _, err := loadTLSProfile("$TLS_PROFILE_TEST_REFERENCE", filepath.Join(dir, "config.hcl")); err != nil {
		t.Fatalf("literal filesystem spelling should remain usable: %v", err)
	}
	customJSON := bytes.Replace(tlsTestFixture(t), []byte(`"TLS_AES_128_GCM_SHA256"`), []byte(`env("TLS_PROFILE_TEST_CIPHER")`), 1)
	t.Setenv("TLS_PROFILE_TEST_CIPHER", "TLS_AES_128_GCM_SHA256")
	file := filepath.Join(dir, "custom-env.json")
	tlsTestFile(t, file, customJSON)
	if _, err := loadTLSProfile(file, ""); err == nil {
		t.Fatal("custom JSON env calls were expanded")
	}
}

func TestDisabledTLSProfileValidationDoesNotResolveCredentials(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bun.json")
	tlsTestFile(t, file, tlsTestFixture(t))
	for _, format := range []string{FormatHCL, FormatJSON} {
		source := tlsTestSource(t, format, map[string]any{"enabled": false, "api_key": "", "tls_client_hello": "custom", "tls_client_hello_spec_file": file}, nil)
		rt, err := Load(source, filepath.Join(dir, "config."+format))
		if err != nil {
			t.Fatal(err)
		}
		p := rt.Catalog.DisabledProviders()[0]
		if p.Enabled || p.APIKey != "" || !p.TLSProfile.IsCustom() {
			t.Fatal("disabled credential behavior or profile validation changed")
		}
		for _, selector := range []string{"", "firefox", "Chrome"} {
			if _, err := Load(tlsTestSource(t, format, map[string]any{"enabled": false, "api_key": "", "tls_client_hello": selector}, nil), "config."+format); err == nil {
				t.Fatal("disabled invalid profile was ignored")
			}
		}
	}
	missingSecrets := filepath.Join(dir, "missing-keys.json")
	source := []byte(fmt.Sprintf(`
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "profiled" {
  enabled = false
  api_key_ref {
    path = %q
    key = "missing"
  }
  tls_client_hello = "custom"
  tls_client_hello_spec_file = %q
  model "m" {}
}
`, missingSecrets, file))
	rt, err := Load(source, filepath.Join(dir, "config.hcl"))
	if err != nil || rt.Catalog.DisabledProviders()[0].APIKeyRef.Resolved {
		t.Fatalf("disabled secret reference was resolved: %v", err)
	}
}

func TestDynamicTLSProfilesResolveBuiltinsWithoutFileIO(t *testing.T) {
	for _, selector := range append([]string{""}, tlsprofile.BuiltinSelectors()...) {
		p := validDynamicProvider()
		p.TLSClientHello = selector
		p.HasTLSClientHello = selector != ""
		resolved, err := ResolveDynamicProviderTLSProfile(p)
		if err != nil || ValidateDynamicProvider(resolved) != nil {
			t.Fatalf("dynamic builtin %s: %v", selector, err)
		}
		if selector == "" {
			selector = "native"
		}
		want, _ := tlsprofile.ResolveBuiltin(selector)
		if resolved.TLSProfile.Identity() != want.Identity() {
			t.Fatal("dynamic selection did not resolve its effective profile")
		}
		rt := &Runtime{}
		if rt.ApplyProviderDefaults(p).TLSProfile.Identity() != want.Identity() {
			t.Fatal("dynamic defaults did not resolve builtin profile")
		}
		catalog := NewCatalog([]Provider{p}, nil, nil)
		copied, _ := catalog.Provider(p.Name)
		if copied.TLSProfile.Identity() != want.Identity() {
			t.Fatal("catalog did not resolve a valid builtin by value")
		}
	}
	custom, err := tlsprofile.ParseCustom(tlsTestFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Provider){
		func(p *Provider) { p.TLSClientHello = "custom" },
		func(p *Provider) { p.TLSClientHelloSpecFile = "private-source-value" },
		func(p *Provider) { p.HasTLSClientHelloSpecFile = true },
		func(p *Provider) { p.HasTLSClientHello = true },
		func(p *Provider) { p.TLSClientHello = "Chrome" },
		func(p *Provider) { p.TLSClientHello = "firefox" },
		func(p *Provider) { p.TLSProfile = custom; p.TLSClientHelloInherited = true },
		func(p *Provider) { p.TLSClientHello = "native"; p.TLSProfile = custom },
		func(p *Provider) { p.TLSClientHello = "chrome"; p.TLSProfile, _ = tlsprofile.ResolveBuiltin("safari") },
	} {
		for _, enabled := range []bool{true, false} {
			p := validDynamicProvider()
			p.Enabled = enabled
			mutate(&p)
			if _, err := ResolveDynamicProviderTLSProfile(p); err == nil || strings.Contains(err.Error(), "private-source-value") {
				t.Fatalf("invalid dynamic profile was accepted or leaked a reference: %v", err)
			}
			if err := ValidateDynamicProvider(p); err == nil {
				t.Fatal("dynamic validation bypassed builtin-only profile policy")
			}
		}
	}
}

func TestValidateRuntimeTLSProfileDoesNotResolveUnloadedCustom(t *testing.T) {
	rt, err := Load(tlsTestSource(t, FormatHCL, nil, nil), "config.hcl")
	if err != nil {
		t.Fatal(err)
	}
	p := testProvider(t, rt, "profiled")
	p.TLSClientHello = "custom"
	p.TLSClientHelloSpecFile = "private-source-value"
	rt.Catalog = NewCatalog([]Provider{p}, nil, nil)
	if err := Validate(rt); err == nil || !strings.Contains(err.Error(), "resolved during config construction") || strings.Contains(err.Error(), "private-source-value") {
		t.Fatalf("unloaded runtime custom error = %v", err)
	}
}

func TestTLSOnlyValidationSupportsStagedCredentialsAndPreservesEnvSpelling(t *testing.T) {
	dir := t.TempDir()
	tlsTestFile(t, filepath.Join(dir, "profiles", "bun.json"), tlsTestFixture(t))
	t.Setenv("TLS_PROFILE_TEST_SELECTOR", "custom")
	t.Setenv("TLS_PROFILE_TEST_SPEC", "profiles/./bun.json")
	source := []byte(`
provider "openai" "profiled" {
  api_key_ref {
    path = "not-yet-published.json"
    key = "staged"
  }
  tls_client_hello = env("TLS_PROFILE_TEST_SELECTOR")
  tls_client_hello_spec_file = env("TLS_PROFILE_TEST_SPEC")
  model "m" {}
}
`)
	before := slices.Clone(source)
	if err := ValidateProviderTLSProfiles(source, filepath.Join(dir, "config.hcl")); err != nil {
		t.Fatalf("TLS-only validation resolved staged credentials: %v", err)
	}
	if !bytes.Equal(before, source) || !bytes.Contains(source, []byte(`env("TLS_PROFILE_TEST_SPEC")`)) || !bytes.Contains(source, []byte(`env("TLS_PROFILE_TEST_SELECTOR")`)) {
		t.Fatal("TLS-only validation changed source/env spelling")
	}
	if err := ValidateProviderTLSProfiles(source, EnvConfigFilename); err == nil || !strings.Contains(err.Error(), "absolute reference") {
		t.Fatalf("TLS-only validation lost real-source semantics: %v", err)
	}
	t.Setenv("TLS_PROFILE_TEST_SELECTOR", "chrome")
	if err := ValidateProviderTLSProfiles(source, filepath.Join(dir, "config.hcl")); err == nil || !strings.Contains(err.Error(), "cannot be declared") {
		t.Fatalf("TLS-only env selector was not resolved: %v", err)
	}
}
