package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDocumentedTenantUpstreamPolicyHCLJSONLoadAndConversion(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "website", "docs", "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AIPROXY_OPERATOR_TOKEN", "documentation-operator-fixture")
	t.Setenv("AIPROXY_DATABASE_URL", "postgres://localhost/documentation-fixture")
	wantOrigins := []string{"https://api.openai.com", "https://api.githubcopilot.com", "https://probe.example.com:8443"}
	var expected *Runtime
	for _, format := range []string{FormatHCL, FormatJSON} {
		name := "tenant-upstream-policy." + format
		fence := "```" + format + ` title="` + name + "\"\n"
		if strings.Count(string(doc), fence) != 1 {
			t.Fatalf("expected one exact documented %s fixture", name)
		}
		_, rest, _ := strings.Cut(string(doc), fence)
		source, _, ok := strings.Cut(rest, "\n```")
		if !ok {
			t.Fatal("unterminated tenant policy example")
		}
		rt, err := Load([]byte(source), name)
		if err != nil || !rt.RequiresDB() || !reflect.DeepEqual(rt.MultiTenancy.AllowedUpstreamOrigins, wantOrigins) {
			t.Fatalf("documented %s did not load the exact approvals: %v", name, err)
		}
		if expected == nil {
			expected = rt
		} else if !reflect.DeepEqual(expected, rt) {
			t.Fatal("documented HCL and JSON configurations differ")
		}
		converted, from, to, err := Convert([]byte(source), name, false)
		if err != nil || from != format || to == format || strings.Contains(string(converted), "env(") {
			t.Fatalf("documented conversion did not validate/resolve env values: %v", err)
		}
		convertedRuntime, err := Load(converted, "converted."+to)
		if err != nil || !reflect.DeepEqual(rt, convertedRuntime) {
			t.Fatalf("documented conversion lost policy intent: %v", err)
		}
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		fileRuntime, err := LoadFile(path)
		if err != nil || !reflect.DeepEqual(rt, fileRuntime) {
			t.Fatalf("documented file policy load differs: %v", err)
		}
		t.Setenv(ConfigEnvVar, source)
		inline, err := LoadEnv()
		if err != nil || !reflect.DeepEqual(rt, inline) {
			t.Fatalf("operator inline config bypassed or lost approval validation: %v", err)
		}
	}
}
