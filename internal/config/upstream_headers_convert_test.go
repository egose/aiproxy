package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func upstreamHeaderDeclarations(rt *Runtime) map[string]map[string]string {
	out := map[string]map[string]string{"root": rt.UpstreamHeaders.Values()}
	for _, providers := range [][]Provider{rt.Catalog.Providers(), rt.Catalog.DisabledProviders()} {
		for _, p := range providers {
			out["provider/"+p.Name] = p.UpstreamHeaders.Values()
			for _, m := range p.Models {
				out["model/"+p.Name+"/"+m.Name] = m.UpstreamHeaders.Values()
			}
		}
	}
	for _, a := range rt.Catalog.Aliases() {
		out["alias/"+a.Name] = a.UpstreamHeaders.Values()
	}
	return out
}

func TestConvertScopedUpstreamHeadersRoundTrip(t *testing.T) {
	t.Setenv("AIPROXY_FIXTURE_HEADER", `  fixture with "quotes" and \  `)
	for _, fixture := range []struct{ name, hcl, json string }{
		{"scopes", upstreamHeadersHCLScopes, upstreamHeadersJSONScopes},
		{"derived", upstreamHeadersHCLDerived, upstreamHeadersJSONDerived},
		{"literal-values", `upstream_headers = { "for" = "literal $${text} and %%{text}", "x.a!" = "field" }` + "\n", `"upstream_headers":{"for":"literal ${text} and %{text}","x.a!":"field"},`},
	} {
		hcl := []byte(upstreamHeadersHCLBase + fixture.hcl)
		json := []byte("{" + fixture.json + upstreamHeadersJSONBase + "}")
		rt, err := Load(hcl, "fixture.hcl")
		if err != nil {
			t.Fatal(err)
		}
		want := upstreamHeaderDeclarations(rt)
		for _, start := range []struct {
			format string
			source []byte
		}{{FormatHCL, hcl}, {FormatJSON, json}} {
			t.Run(fixture.name+"/"+start.format, func(t *testing.T) {
				source, format := start.source, start.format
				for range 2 {
					out, from, to, err := Convert(source, "fixture."+format, true)
					if err != nil || from != format || to == format {
						t.Fatalf("conversion from %s: %s -> %s: %v", format, from, to, err)
					}
					if strings.Contains(string(out), `env("AIPROXY_FIXTURE_HEADER")`) {
						t.Fatal("explicit conversion did not materialize env()")
					}
					loaded, err := Load(out, "fixture."+to)
					if err != nil {
						t.Fatal(err)
					}
					if got := upstreamHeaderDeclarations(loaded); !reflect.DeepEqual(got, want) {
						t.Fatalf("%s lost local header intent: got %q want %q", to, got, want)
					}
					source, format = out, to
				}
			})
		}
	}
}

func TestConvertUpstreamHeadersRejectsInvalidSource(t *testing.T) {
	const hidden = "fixture-value-not-for-diagnostics"
	for scope, template := range map[string]struct{ hcl, json string }{
		"root":     {`%s`, `%s`},
		"provider": {"provider \"openai\" \"p\" {\n enabled = false\n %s\n model \"m\" {}\n}", `"provider":{"openai":{"p":{"enabled":false,%s,"model":{"m":{}}}}}`},
		"model":    {"provider \"openai\" \"p\" {\n enabled = false\n model \"m\" {\n %s\n }\n}", `"provider":{"openai":{"p":{"enabled":false,"model":{"m":{%s}}}}}`},
		"alias":    {"alias \"pool\" {\n algorithm = \"round_robin\"\n %s\n}", `"alias":{"pool":{"algorithm":"round_robin",%s}}`},
	} {
		for _, tc := range []struct{ name, hcl, json, errorText string }{
			{"exact", `upstream_headers = { X-F = "first", X-F = "` + hidden + `" }`, `"upstream_headers":{"X-F":"first","X-F":"` + hidden + `"}`, "duplicate header name"},
			{"folded", `upstream_headers = ({ "X-F" = "first", "x-f" = "` + hidden + `" })`, `"upstream_headers":{"X-F":"first","x-f":"` + hidden + `"}`, "duplicate header name"},
			{"declaration", "upstream_headers = { X-F = \"first\" }\nupstream_headers = { X-F = \"" + hidden + "\" }", `"upstream_headers":{"X-F":"first"},"upstream\u005fheaders":{"X-F":"` + hidden + `"}`, "duplicate upstream_headers declaration"},
			{"nonstring", `upstream_headers = { X-F = "` + hidden + `", X-Other = 1 }`, `"upstream_headers":{"X-F":"` + hidden + `","X-Other":1}`, "must be a non-null string"},
		} {
			t.Run(scope+"/"+tc.name, func(t *testing.T) {
				for format, source := range map[string]string{
					FormatHCL:  upstreamHeadersHCLBase + fmt.Sprintf(template.hcl, tc.hcl) + "\n",
					FormatJSON: "{" + upstreamHeadersJSONBase + "," + fmt.Sprintf(template.json, tc.json) + "}",
				} {
					out, _, _, err := Convert([]byte(source), "fixture."+format, false)
					if out != nil || err == nil || !strings.Contains(err.Error(), tc.errorText) || strings.Contains(err.Error(), hidden) {
						t.Errorf("%s: got %v, want safe error containing %q", format, err, tc.errorText)
					}
				}
			})
		}
	}
}

func TestConvertRejectsSilentLossOfUpstreamHeadersInRepeatedJSONBlockKeys(t *testing.T) {
	source := []byte(`{` + upstreamHeadersJSONBase + `,
  "provider":{"openai":{"p":{"api_key":"fixture","model":{"m":{}}}}},
  "alias":{"a":{"algorithm":"round_robin","providers":["p"],"model":"m","upstream_headers":{}}},
  "alias":{"b":{"algorithm":"round_robin","providers":["p"],"model":"m"}}}`)
	if _, err := Load(source, "fixture.json"); err != nil {
		t.Fatalf("fixture must be readable as HCL JSON: %v", err)
	}
	if out, _, _, err := Convert(source, "fixture.json", false); out != nil || err == nil || !strings.Contains(err.Error(), "cannot preserve all upstream_headers declarations") {
		t.Fatalf("conversion silently discarded an explicit map: %v", err)
	}
}
