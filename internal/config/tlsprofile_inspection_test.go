package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

func tlsInspectionError(t *testing.T, source []byte, filename string) error {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("TLS-only validation panicked: %v", recovered)
		}
	}()
	return ValidateProviderTLSProfiles(source, filename)
}

func TestTLSProfileInspectionDiagnosticsAreControlled(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "private-config-location.json")
	reference := filepath.Join(dir, "private-profile-reference.json")
	encodedReference, err := json.Marshal(reference)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		source string
	}{
		{"duplicate extends boolean selector", `{"provider":{"openai":{"p":{"extends":"b","extends":"b","tls_client_hello":false}}}}`},
		{"duplicate extends null selector", `{"provider":{"openai":{"p":{"extends":"b","extends":"b","tls_client_hello":null}}}}`},
		{"duplicate extends number selector", `{"provider":{"openai":{"p":{"extends":"b","extends":"b","tls_client_hello":42}}}}`},
		{"duplicate extends array selector", `{"provider":{"openai":{"p":{"extends":"b","extends":"b","tls_client_hello":[]}}}}`},
		{"duplicate extends object selector", `{"provider":{"openai":{"p":{"extends":"b","extends":"b","tls_client_hello":{"private-content-value":1}}}}}`},
		{"duplicate extends boolean reference", `{"provider":{"openai":{"p":{"extends":"b","extends":"b","tls_client_hello":"custom","tls_client_hello_spec_file":false}}}}`},
		{"duplicate extends null reference", `{"provider":{"openai":{"p":{"extends":"b","extends":"b","tls_client_hello":"custom","tls_client_hello_spec_file":null}}}}`},
		{"duplicate extends number reference", `{"provider":{"openai":{"p":{"extends":"b","extends":"b","tls_client_hello_spec_file":42}}}}`},
		{"duplicate extends array reference", `{"provider":{"openai":{"p":{"extends":"b","extends":"b","tls_client_hello_spec_file":[]}}}}`},
		{"duplicate extends object reference", `{"provider":{"openai":{"p":{"extends":"b","extends":"b","tls_client_hello_spec_file":{"private-content-value":1}}}}}`},
		{"duplicate extends valid custom reference", fmt.Sprintf(`{"provider":{"openai":{"p":{"extends":%s,"extends":%s,"tls_client_hello":"custom","tls_client_hello_spec_file":%s}}}}`, encodedReference, encodedReference, encodedReference)},
		{"duplicate root selector", `{"tls_client_hello":"native","tls_client_hello":"private-content-value"}`},
		{"duplicate root reference", fmt.Sprintf(`{"tls_client_hello_spec_file":%s,"tls_client_hello_spec_file":%s}`, encodedReference, encodedReference)},
		{"duplicate provider selector", `{"provider":{"openai":{"p":{"tls_client_hello":"native","tls_client_hello":null}}}}`},
		{"duplicate provider reference", fmt.Sprintf(`{"provider":{"openai":{"p":{"tls_client_hello":"custom","tls_client_hello_spec_file":%s,"tls_client_hello_spec_file":false}}}}`, encodedReference)},
		{"invalid provider declaration", `{"provider":"private-content-value"}`},
		{"invalid provider body", `{"provider":{"openai":{"p":false}}}`},
		{"missing provider label", "provider \"openai\" {\n  tls_client_hello = \"native\"\n}\n"},
		{"duplicate HCL root selector", "tls_client_hello = \"native\"\ntls_client_hello = \"private-content-value\"\n"},
		{"duplicate HCL root reference", fmt.Sprintf("tls_client_hello_spec_file = %q\ntls_client_hello_spec_file = %q\n", reference, reference)},
		{"invalid HCL syntax", "provider \"openai\" \"p\" {\n  tls_client_hello = \"private-content-value\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tlsInspectionError(t, []byte(tc.source), filename)
			if err == nil {
				t.Fatal("invalid inspection must return an error")
			}
			if !strings.Contains(err.Error(), "tls_client_hello") {
				t.Fatalf("missing TLS field context: %v", err)
			}
			for _, private := range []string{dir, reference, "private-config-location", "private-profile-reference", "private-content-value"} {
				if strings.Contains(err.Error(), private) {
					t.Fatalf("inspection error exposed arbitrary content or paths: %v", err)
				}
			}
		})
	}
}

func tlsExtendsInspectionSource(t *testing.T, format, baseName string, extends any, local map[string]any) []byte {
	t.Helper()
	child := map[string]any{"extends": extends}
	for name, value := range local {
		child[name] = value
	}
	source := tlsTestSource(t, format, map[string]any{"tls_client_hello": "chrome"}, child)
	base, err := json.Marshal(baseName)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.ReplaceAll(source, []byte(`"profiled"`), base)
}

func TestTLSProfileExtendsCoercionMatchesLoading(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		extends any
	}{
		{"number", "42", 42},
		{"zero", "0", 0},
		{"true", "true", true},
		{"false", "false", false},
		{"string", "42", "42"},
	}
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, tc := range cases {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				filename := filepath.Join(t.TempDir(), "private-config-location."+format)
				source := tlsExtendsInspectionSource(t, format, tc.base, tc.extends, nil)
				if err := tlsInspectionError(t, source, filename); err != nil {
					t.Fatalf("TLS-only validation rejected decoder-compatible inheritance: %v", err)
				}
				rt, err := Load(source, filename)
				if err != nil {
					t.Fatalf("full loading rejected decoder-compatible inheritance: %v", err)
				}
				base, child := testProvider(t, rt, tc.base), testProvider(t, rt, "child")
				if child.TLSProfile.Identity() != base.TLSProfile.Identity() || child.TLSProfile.CanonicalName() != "chrome-133" || !child.TLSClientHelloInherited {
					t.Fatal("coerced extends did not preserve effective TLS inheritance")
				}
				for _, field := range []string{"tls_client_hello", "tls_client_hello_spec_file"} {
					for _, value := range []any{"native", "", false, nil, "private-content-value"} {
						source := tlsExtendsInspectionSource(t, format, tc.base, tc.extends, map[string]any{field: value})
						want := `provider "child": derived provider cannot declare ` + field
						if err := tlsInspectionError(t, source, filename); err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "private-content-value") || strings.Contains(err.Error(), filename) {
							t.Fatalf("TLS-only coercion/declaration mismatch for %s: %v", field, err)
						}
						if _, err := Load(source, filename); err == nil || !strings.Contains(err.Error(), want) {
							t.Fatalf("full loading coercion/declaration mismatch for %s: %v", field, err)
						}
					}
				}
			})
		}
	}
}

func TestTLSProfileExtendsInvalidValuesAreControlled(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, value := range []any{nil, []any{}, []any{"private-content-value"}, map[string]any{"private-content-value": "private-content-value"}} {
			for _, local := range []map[string]any{nil, {"tls_client_hello": "native"}, {"tls_client_hello_spec_file": false}} {
				source := tlsExtendsInspectionSource(t, format, "base", value, local)
				filename := filepath.Join(t.TempDir(), "private-config-location."+format)
				if err := tlsInspectionError(t, source, filename); err == nil || !strings.Contains(err.Error(), "extends") || strings.Contains(err.Error(), "private-content-value") || strings.Contains(err.Error(), filename) {
					t.Fatalf("invalid extends did not produce a controlled TLS-only error: %v", err)
				}
				if _, err := Load(source, filename); err == nil || !strings.Contains(err.Error(), "extends") || strings.Contains(err.Error(), "private-content-value") || strings.Contains(err.Error(), filename) {
					t.Fatalf("invalid extends did not produce a controlled loading error: %v", err)
				}
			}
		}
	}
	for _, tc := range []struct {
		source string
		want   string
	}{
		{"provider \"openai\" \"p\" {\n  extends = private_content_value\n  tls_client_hello = \"native\"\n}\n", "extends"},
		{"provider \"openai\" \"p\" {\n  extends = unknown(\"private-content-value\")\n}\n", "extends"},
		{`{"provider":{"openai":{"p":{"extends":"${private_content_value}","tls_client_hello":"native"}}}}`, "derived provider cannot declare tls_client_hello"},
	} {
		filename := filepath.Join(t.TempDir(), "private-config-location")
		if err := tlsInspectionError(t, []byte(tc.source), filename); err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "private_content_value") || strings.Contains(err.Error(), "private-content-value") || strings.Contains(err.Error(), filename) {
			t.Fatalf("extends expression diagnostics were not controlled: %v", err)
		}
	}
}

func TestTLSProfileExtendsEmptyRemainsConcrete(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		source := tlsTestSource(t, format, nil, map[string]any{
			"extends":          "",
			"tls_client_hello": "safari",
			"model":            map[string]any{"m": map[string]any{}},
		})
		if err := tlsInspectionError(t, source, "config."+format); err != nil {
			t.Fatal(err)
		}
		rt, err := Load(source, "config."+format)
		if err != nil {
			t.Fatal(err)
		}
		p := testProvider(t, rt, "child")
		if p.TLSClientHelloInherited || p.TLSProfile.CanonicalName() != "safari-16.0" {
			t.Fatal("empty extends changed concrete provider behavior")
		}
	}
}

func TestTLSProfileAttributeStringConversionIsDefensive(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "private-config-location")
	for _, name := range []string{"tls_client_hello", "tls_client_hello_spec_file"} {
		for _, value := range []cty.Value{
			cty.False, cty.True, cty.NumberIntVal(42), cty.NullVal(cty.String), cty.NullVal(cty.Bool),
			cty.EmptyTupleVal, cty.EmptyObjectVal, cty.UnknownVal(cty.String), cty.NilVal,
		} {
			attr := &hcl.Attribute{Name: name, Expr: hcl.StaticExpr(value, hcl.Range{Filename: filename})}
			if _, err := providerTLSAttributeString(attr, "p", false); err == nil || !strings.Contains(err.Error(), name) || strings.Contains(err.Error(), filename) {
				t.Fatalf("unsafe TLS string conversion for %s: %v", name, err)
			}
		}
	}
}
