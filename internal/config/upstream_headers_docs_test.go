package config

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestDocumentedFixedUpstreamHeaders(t *testing.T) {
	t.Setenv("AIPROXY_CLIENT_TOKEN", "documentation-client-token")
	t.Setenv("PRIMARY_UPSTREAM_API_KEY", "documentation-primary-key")
	t.Setenv("BACKUP_UPSTREAM_API_KEY", "documentation-backup-key")
	t.Setenv("PRIMARY_UPSTREAM_GROUP", "documentation-routing-group")
	data, err := os.ReadFile(filepath.Join("..", "..", "website", "docs", "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	for _, format := range []string{FormatHCL, FormatJSON} {
		t.Run(format, func(t *testing.T) {
			name := "fixed-upstream-headers." + format
			fence := "```" + format + ` title="` + name + "\"\n"
			if strings.Count(doc, fence) != 1 {
				t.Fatalf("expected one documented %s example", name)
			}
			_, rest, _ := strings.Cut(doc, fence)
			source, _, ok := strings.Cut(rest, "\n```")
			if !ok {
				t.Fatalf("unterminated %s example", name)
			}
			rt, err := Load([]byte(source+"\n"), name)
			if err != nil {
				t.Fatalf("documented config is invalid: %v", err)
			}
			primary, backup := testProvider(t, rt, "primary"), testProvider(t, rt, "backup")
			a := testAlias(t, rt, "public-chat")
			if backup.Extends != primary.Name || backup.BaseURL != primary.BaseURL || backup.ModelByName["chat"].UpstreamName != "chat-model" ||
				backup.UpstreamHeaders.declared() || len(a.Targets) != 2 || rt.Auth.Mode != AuthModeBearerStatic ||
				!slices.Equal(primary.ForwardHeaders, []string{"X-App-Name", "X-Request-Tag"}) {
				t.Fatal("documented routing, forwarding or derivative declarations drifted")
			}
			for _, tc := range []struct {
				provider Provider
				alias    *Alias
				want     map[string]string
			}{
				{primary, nil, map[string]string{"X-App-Name": "primary-chat", "X-Environment": "production", "X-Upstream-Group": "documentation-routing-group"}},
				{backup, nil, map[string]string{"X-App-Name": "internal", "X-Environment": "production", "X-Account-Label": "backup-chat"}},
				{primary, &a, map[string]string{"X-App-Name": "public-chat", "X-Environment": "production", "X-Upstream-Group": "documentation-routing-group"}},
				{backup, &a, map[string]string{"X-App-Name": "public-chat", "X-Environment": "production", "X-Account-Label": "backup-chat"}},
			} {
				if got := ResolveUpstreamHeaders(rt.UpstreamHeaders, tc.provider, tc.provider.ModelByName["chat"], tc.alias); !maps.Equal(got, tc.want) {
					t.Errorf("documented %s (alias=%t) resolves %q, want %q", tc.provider.Name, tc.alias != nil, got, tc.want)
				}
			}
			out, _, to, err := Convert([]byte(source+"\n"), name, false)
			if err != nil {
				t.Fatal(err)
			}
			converted, err := Load(out, "fixed-upstream-headers."+to)
			if err != nil || !reflect.DeepEqual(upstreamHeaderDeclarations(rt), upstreamHeaderDeclarations(converted)) {
				t.Fatalf("documented config lost map declarations during conversion: %v", err)
			}
		})
	}
}
