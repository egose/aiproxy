package configedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

func TestTopLevelStringAttributeIgnoresNested(t *testing.T) {
	source := "provider \"openai\" \"primary\" {\n" +
		"  api_key = \"k\"\n" +
		"  upstream_header_timeout = \"180s\"\n" +
		"  model \"gpt-4o-mini\" {}\n" +
		"}\n"
	got, err := TopLevelStringAttribute(source, "upstream_header_timeout")
	if err != nil {
		t.Fatalf("TopLevelStringAttribute(): %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty (nested must not read as root)", got)
	}
}

func TestUpsertTopLevelStringAttributeTable(t *testing.T) {
	cases := []struct {
		name                string
		source              string
		wantRoot            string
		wantProviderTimeout []string
		wantComments        []string
	}{
		{
			name:                "root absent no providers",
			source:              "listener \"http\" \"public\" { address = \":8080\" }\n",
			wantRoot:            "30s",
			wantProviderTimeout: nil,
		},
		{
			name: "root absent one provider override",
			source: "provider \"openai\" \"primary\" {\n" +
				"  api_key = \"k\"\n" +
				"  upstream_header_timeout = \"180s\"\n" +
				"  model \"gpt-4o-mini\" {}\n" +
				"}\n",
			wantRoot:            "30s",
			wantProviderTimeout: []string{"180s"},
		},
		{
			name: "root present one provider override",
			source: "upstream_header_timeout = \"120s\"\n" +
				"\n" +
				"provider \"openai\" \"primary\" {\n" +
				"  api_key = \"k\"\n" +
				"  upstream_header_timeout = \"180s\"\n" +
				"  model \"gpt-4o-mini\" {}\n" +
				"}\n",
			wantRoot:            "30s",
			wantProviderTimeout: []string{"180s"},
		},
		{
			name: "root present multiple provider overrides",
			source: "upstream_header_timeout = \"120s\"\n" +
				"\n" +
				"provider \"openai\" \"a\" {\n" +
				"  api_key = \"k\"\n" +
				"  upstream_header_timeout = \"180s\"\n" +
				"  model \"m\" {}\n" +
				"}\n" +
				"\n" +
				"provider \"openai\" \"b\" {\n" +
				"  api_key = \"k\"\n" +
				"  upstream_header_timeout = \"200s\"\n" +
				"  model \"m\" {}\n" +
				"}\n",
			wantRoot:            "30s",
			wantProviderTimeout: []string{"180s", "200s"},
		},
		{
			name: "provider before root order",
			source: "provider \"openai\" \"primary\" {\n" +
				"  api_key = \"k\"\n" +
				"  upstream_header_timeout = \"180s\"\n" +
				"  model \"gpt-4o-mini\" {}\n" +
				"}\n" +
				"\n" +
				"upstream_header_timeout = \"120s\"\n",
			wantRoot:            "30s",
			wantProviderTimeout: []string{"180s"},
		},
		{
			name: "commented examples preserved",
			source: "# upstream_header_timeout = \"5s\"\n" +
				"// upstream_header_timeout = \"6s\"\n" +
				"/* upstream_header_timeout = \"7s\" */\n" +
				"provider \"openai\" \"primary\" {\n" +
				"  api_key = \"k\"\n" +
				"  # upstream_header_timeout = \"8s\"\n" +
				"  upstream_header_timeout = \"180s\"\n" +
				"  model \"gpt-4o-mini\" {}\n" +
				"}\n",
			wantRoot:            "30s",
			wantProviderTimeout: []string{"180s"},
			wantComments: []string{
				"# upstream_header_timeout = \"5s\"",
				"// upstream_header_timeout = \"6s\"",
				"/* upstream_header_timeout = \"7s\" */",
				"# upstream_header_timeout = \"8s\"",
			},
		},
		{
			name: "root present with comments",
			source: "# root comment\n" +
				"upstream_header_timeout = \"120s\" # trailing comment\n" +
				"\n" +
				"provider \"openai\" \"primary\" {\n" +
				"  api_key = \"k\"\n" +
				"  upstream_header_timeout = \"180s\"\n" +
				"  model \"gpt-4o-mini\" {}\n" +
				"}\n",
			wantRoot:            "30s",
			wantProviderTimeout: []string{"180s"},
			wantComments: []string{
				"# root comment",
				"# trailing comment",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updated, err := UpsertTopLevelStringAttribute(tc.source, "upstream_header_timeout", tc.wantRoot)
			if err != nil {
				t.Fatalf("UpsertTopLevelStringAttribute(): %v", err)
			}
			got, err := TopLevelStringAttribute(updated, "upstream_header_timeout")
			if err != nil {
				t.Fatalf("TopLevelStringAttribute(): %v", err)
			}
			if got != tc.wantRoot {
				t.Fatalf("root = %q, want %q:\n%s", got, tc.wantRoot, updated)
			}
			for _, want := range tc.wantProviderTimeout {
				needle := "upstream_header_timeout = \"" + want + "\""
				if strings.Count(updated, needle) != 1 {
					t.Fatalf("expected exactly one provider %q, got %d:\n%s", needle, strings.Count(updated, needle), updated)
				}
			}
			if strings.Count(updated, "upstream_header_timeout = \""+tc.wantRoot+"\"") != 1 {
				t.Fatalf("expected exactly one root %q:\n%s", tc.wantRoot, updated)
			}
			for _, comment := range tc.wantComments {
				if !strings.Contains(updated, comment) {
					t.Fatalf("comment %q not preserved:\n%s", comment, updated)
				}
			}
			if err := ValidateGeneratedConfig([]byte(updated), "config.hcl"); err != nil {
				t.Fatalf("ValidateGeneratedConfig(): %v", err)
			}
		})
	}
}

func loadableFixture(root string, providers string) string {
	var b strings.Builder
	if root != "" {
		b.WriteString("upstream_header_timeout = \"" + root + "\"\n\n")
	}
	b.WriteString("listener \"http\" \"public\" { address = \":8080\" }\n")
	b.WriteString("auth \"main\" { mode = \"none\" }\n")
	b.WriteString(providers)
	return b.String()
}

func effectiveTimeout(t *testing.T, rt *config.Runtime, name string) time.Duration {
	t.Helper()
	if p, ok := rt.Catalog.Provider(name); ok {
		return p.UpstreamHeaderTimeout
	}
	for _, p := range rt.Catalog.DisabledProviders() {
		if p.Name == name {
			return p.UpstreamHeaderTimeout
		}
	}
	t.Fatalf("provider %q not found", name)
	return 0
}

func TestUpsertRootOnlyChangesRootEffectiveValue(t *testing.T) {
	base := "provider \"openai\" \"base\" {\n" +
		"  api_key = \"k\"\n" +
		"  upstream_header_timeout = \"180s\"\n" +
		"  model \"m\" {}\n" +
		"}\n"
	plain := "provider \"openai\" \"plain\" {\n" +
		"  api_key = \"k\"\n" +
		"  model \"m\" {}\n" +
		"}\n"
	derived := "provider \"openai\" \"child\" {\n" +
		"  extends = \"base\"\n" +
		"  api_key = \"k\"\n" +
		"}\n"
	source := loadableFixture("120s", base+"\n"+plain+"\n"+derived)
	updated, err := UpsertTopLevelStringAttribute(source, "upstream_header_timeout", "30s")
	if err != nil {
		t.Fatalf("UpsertTopLevelStringAttribute(): %v", err)
	}
	rt, err := config.Load([]byte(updated), "config.hcl")
	if err != nil {
		t.Fatalf("Load(): %v\n%s", err, updated)
	}
	if rt.UpstreamHeaderTimeout != 30*time.Second {
		t.Fatalf("root = %v, want 30s", rt.UpstreamHeaderTimeout)
	}
	if got := effectiveTimeout(t, rt, "base"); got != 180*time.Second {
		t.Fatalf("base = %v, want 180s", got)
	}
	if got := effectiveTimeout(t, rt, "child"); got != 180*time.Second {
		t.Fatalf("child (inherited) = %v, want 180s", got)
	}
	if got := effectiveTimeout(t, rt, "plain"); got != 30*time.Second {
		t.Fatalf("plain (root-inheriting) = %v, want 30s", got)
	}
}

func TestUpsertRootInsertPreservesProviderOverrides(t *testing.T) {
	providers := "provider \"openai\" \"slow\" {\n" +
		"  api_key = \"k\"\n" +
		"  upstream_header_timeout = \"180s\"\n" +
		"  model \"m\" {}\n" +
		"}\n"
	source := loadableFixture("", providers)
	updated, err := UpsertTopLevelStringAttribute(source, "upstream_header_timeout", "45s")
	if err != nil {
		t.Fatalf("UpsertTopLevelStringAttribute(): %v", err)
	}
	rt, err := config.Load([]byte(updated), "config.hcl")
	if err != nil {
		t.Fatalf("Load(): %v\n%s", err, updated)
	}
	if rt.UpstreamHeaderTimeout != 45*time.Second {
		t.Fatalf("root = %v, want 45s", rt.UpstreamHeaderTimeout)
	}
	if got := effectiveTimeout(t, rt, "slow"); got != 180*time.Second {
		t.Fatalf("slow = %v, want 180s", got)
	}
}

func TestUpsertInvalidSourceFails(t *testing.T) {
	bad := "provider \"openai\" \"primary\" {\n  api_key = \"k\"\n"
	if _, err := UpsertTopLevelStringAttribute(bad, "upstream_header_timeout", "30s"); err == nil {
		t.Fatalf("expected upsert error for invalid source")
	}
	if _, err := TopLevelStringAttribute(bad, "upstream_header_timeout"); err == nil {
		t.Fatalf("expected read error for invalid source")
	}
}

func TestWriteProviderFilesRejectsInvalidSourceWithoutPublish(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := loadableFixture("120s", "provider \"openai\" \"primary\" {\n  api_key = \"k\"\n  model \"m\" {}\n}\n")
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
	bad := "upstream_header_timeout = \"oops\"\nprovider \"openai\" {\n"
	if err := WriteProviderFiles(configPath, bad, SecretsUpdate{}); err == nil {
		t.Fatalf("expected WriteProviderFiles error for invalid source")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	if string(data) != seed {
		t.Fatalf("config was modified on failed publish:\n%s", string(data))
	}
}
