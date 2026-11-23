package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
)

const upstreamHeadersHCLBase = `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
`

const upstreamHeadersJSONBase = `"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}}`

const upstreamHeadersHCLScopes = `
upstream_headers = { x-scope = "root", X-Root = env("AIPROXY_FIXTURE_HEADER") }
forward_headers = ["X-Root"]
provider "openai" "p" {
  api_key = "fixture"
  upstream_headers = { X-Scope = "provider", x-provider = "provider" }
  model "m" { upstream_headers = { "x-SCOPE" = "model", X-Model = "model" } }
  model "blank" { upstream_headers = {} }
  model "absent" {}
  model_passthrough "family" {
    match = "id-[0-9]+"
    capabilities = ["chat"]
  }
}
provider "openai" "fallback" {
  api_key = "fixture"
  upstream_headers = { X-Fallback = "fallback" }
  model "m" {}
}
alias "pool" {
  algorithm = "round_robin"
  upstream_headers = { "X-scope" = "alias", X-Alias = "alias" }
  providers = ["p", "fallback"]
  model = "m"
}
`

const upstreamHeadersJSONScopes = `
  "upstream_headers":{"x-scope":"root","X-Root":env("AIPROXY_FIXTURE_HEADER")},
  "forward_headers":["X-Root"],
  "provider":{"openai":{
    "p":{"api_key":"fixture",
      "upstream_headers":{"X-Scope":"provider","x-provider":"provider"},
      "model":{"m":{"upstream_headers":{"x-SCOPE":"model","X-Model":"model"}},"blank":{"upstream_headers":{}},"absent":{}},
      "model_passthrough":{"family":{"match":"id-[0-9]+","capabilities":["chat"]}}},
    "fallback":{"api_key":"fixture","upstream_headers":{"X-Fallback":"fallback"},"model":{"m":{}}}}},
  "alias":{"pool":{"algorithm":"round_robin","upstream_headers":{"X-scope":"alias","X-Alias":"alias"},"providers":["p","fallback"],"model":"m"}},
`

func TestLoadScopedUpstreamHeaders(t *testing.T) {
	t.Setenv("AIPROXY_FIXTURE_HEADER", `  fixture value with "quotes" and \  `)
	const rootValue = `  fixture value with "quotes" and \  `
	for format, src := range map[string]string{
		FormatHCL:  upstreamHeadersHCLBase + upstreamHeadersHCLScopes,
		FormatJSON: "{" + upstreamHeadersJSONScopes + upstreamHeadersJSONBase + "}",
	} {
		t.Run(format, func(t *testing.T) {
			t.Setenv(ConfigEnvVar, src)
			rt, err := LoadEnv()
			if err != nil {
				t.Fatal(err)
			}
			p, m, ok := rt.Catalog.Model("p", "m")
			if !ok {
				t.Fatal("model missing")
			}
			a := testAlias(t, rt, "pool")
			fallback := testProvider(t, rt, "fallback")
			passthrough, _, match := rt.Catalog.MatchModelPassthrough("p", "id-123", nil)
			if match != ModelPassthroughMatched {
				t.Fatal("passthrough rule missing")
			}
			if len(p.ForwardHeaders) != 1 || p.ForwardHeaders[0] != "X-Root" {
				t.Fatal("fixed declarations changed caller forwarding")
			}
			if p.ModelByName["blank"].UpstreamHeaders.Values() == nil || p.ModelByName["absent"].UpstreamHeaders.Values() != nil {
				t.Fatal("empty and absent local intent were not preserved")
			}
			root := rt.UpstreamHeaders
			direct := map[string]string{"X-Scope": "model", "X-Root": rootValue, "X-Provider": "provider", "X-Model": "model"}
			for name, tc := range map[string]struct {
				got, want map[string]string
			}{
				"root intent":     {root.Values(), map[string]string{"X-Scope": "root", "X-Root": rootValue}},
				"provider intent": {p.UpstreamHeaders.Values(), map[string]string{"X-Scope": "provider", "X-Provider": "provider"}},
				"model intent":    {m.UpstreamHeaders.Values(), map[string]string{"X-Scope": "model", "X-Model": "model"}},
				"alias intent":    {a.UpstreamHeaders.Values(), map[string]string{"X-Scope": "alias", "X-Alias": "alias"}},
				"direct":          {ResolveUpstreamHeaders(root, p, m, nil), direct},
				"passthrough":     {ResolveUpstreamHeaders(root, passthrough, Model{}, nil), map[string]string{"X-Scope": "provider", "X-Root": rootValue, "X-Provider": "provider"}},
				"alias":           {ResolveUpstreamHeaders(root, p, m, &a), map[string]string{"X-Scope": "alias", "X-Root": rootValue, "X-Provider": "provider", "X-Model": "model", "X-Alias": "alias"}},
				"fallback":        {ResolveUpstreamHeaders(root, fallback, fallback.ModelByName["m"], &a), map[string]string{"X-Scope": "alias", "X-Root": rootValue, "X-Fallback": "fallback", "X-Alias": "alias"}},
			} {
				if !maps.Equal(tc.got, tc.want) {
					t.Errorf("%s = %q, want %q", name, tc.got, tc.want)
				}
			}
			managedAlias := a
			managedAlias.DatabaseManaged = true
			if got := ResolveUpstreamHeaders(root, p, m, &managedAlias); !maps.Equal(got, direct) {
				t.Fatal("managed alias affected the static target's fixed headers")
			}
			managed := p
			managed.DatabaseManaged = true
			if got := ResolveUpstreamHeaders(root, managed, m, &a); got != nil {
				t.Fatal("a managed target received fixed headers")
			}
			for _, snapshot := range []map[string]string{root.Values(), p.UpstreamHeaders.Values(), m.UpstreamHeaders.Values(), a.UpstreamHeaders.Values(), ResolveUpstreamHeaders(root, p, m, nil)} {
				snapshot["X-Scope"] = "changed"
				delete(snapshot, "X-Root")
			}
			p.UpstreamHeaders = UpstreamHeaders{}
			p.Models[0].UpstreamHeaders = UpstreamHeaders{}
			p.ModelByName["m"] = Model{}
			a.UpstreamHeaders = UpstreamHeaders{}
			rt.UpstreamHeaders = UpstreamHeaders{}
			p, m, _ = rt.Catalog.Model("p", "m")
			a = testAlias(t, rt, "pool")
			if got := ResolveUpstreamHeaders(root, p, m, nil); !maps.Equal(got, direct) || a.UpstreamHeaders.Values()["X-Scope"] != "alias" {
				t.Fatal("editing returned copies mutated the admitted snapshot")
			}
			encoded, err := json.Marshal([]any{rt, root, p, m, a})
			if err != nil || strings.Contains(string(encoded), "X-Scope") || strings.Contains(string(encoded), "UpstreamHeaders") {
				t.Fatal("fixed declarations should not serialize into views")
			}
		})
	}
}

func TestUpstreamHeaderPreflight(t *testing.T) {
	const sensitive = "fixture-not-for-diagnostics"
	for _, tc := range []struct {
		name, hcl, json, want string
	}{
		{"null-map", `null`, `null`, "map of non-null strings"},
		{"array-map", `[{ X-F = "` + sensitive + `" }]`, `[{"X-F":"` + sensitive + `"}]`, "map of non-null strings"},
		{"string-map", `"` + sensitive + `"`, `"` + sensitive + `"`, "map of non-null strings"},
		{"number", `{ X-F = 123 }`, `{"X-F":123}`, "non-null string"},
		{"bool", `{ X-F = true }`, `{"X-F":true}`, "non-null string"},
		{"null-value", `{ X-F = null }`, `{"X-F":null}`, "non-null string"},
		{"nested", `{ X-F = { content = "` + sensitive + `" } }`, `{"X-F":{"content":"` + sensitive + `"}}`, "non-null string"},
		{"empty", `{ X-F = "" }`, `{"X-F":""}`, "nonblank"},
		{"blank", `{ X-F = " \u00a0 " }`, `{"X-F":" \u00a0 "}`, "nonblank"},
		{"missing-name", `{ "" = "` + sensitive + `" }`, `{"":"` + sensitive + `"}`, "valid HTTP field names"},
		{"invalid-name", `{ "X-F:" = "` + sensitive + `" }`, `{"X-F:":"` + sensitive + `"}`, "valid HTTP field names"},
		{"unicode-name", `{ "X-Fé" = "` + sensitive + `" }`, `{"X-Fé":"` + sensitive + `"}`, "valid HTTP field names"},
		{"non-string-name", `{ (123) = "` + sensitive + `" }`, "", "HTTP field names"},
		{"crlf", `{ X-F = "` + sensitive + `\r\nmore" }`, `{"X-F":"` + sensitive + `\r\nmore"}`, "control characters"},
		{"tab", `{ X-F = "` + sensitive + `\t" }`, `{"X-F":"` + sensitive + `\t"}`, "control characters"},
		{"del", `{ X-F = "` + sensitive + `\u007f" }`, `{"X-F":"` + sensitive + `\u007f"}`, "control characters"},
		{"expression-error", `{ X-F = "${"` + sensitive + `"[0]}" }`, "", "non-null string"},
		{"malformed-template", `{ X-F = "` + sensitive + `\z" }`, "", "invalid upstream_headers syntax"},
		{"duplicate-exact", `{ X-F = "first", "X-F" = "` + sensitive + `" }`, `{"X-F":"first","X-\u0046":"` + sensitive + `"}`, "duplicate header name"},
		{"duplicate-folded", `({ "X-F" = "first", "x-f" = "` + sensitive + `" })`, `{"X-F":"first","x-f":"` + sensitive + `"}`, "duplicate header name"},
	} {
		if tc.hcl != "" {
			t.Run("hcl/"+tc.name, func(t *testing.T) {
				assertUpstreamHeaderPreflightError(t, upstreamHeadersHCLBase+"\nupstream_headers = "+tc.hcl+"\n", "fixture.hcl", tc.want, sensitive)
			})
		}
		if tc.json != "" {
			t.Run("json/"+tc.name, func(t *testing.T) {
				assertUpstreamHeaderPreflightError(t, "{"+upstreamHeadersJSONBase+`,"upstream_headers":`+tc.json+"}", "fixture.json", tc.want, sensitive)
			})
		}
	}
}

func TestUpstreamHeaderDuplicatesInAllScopes(t *testing.T) {
	const sensitive = "fixture-not-for-diagnostics"
	for scope, tc := range map[string]struct{ hcl, json string }{
		"root":     {`%s`, `%s`},
		"provider": {"provider \"openai\" \"p\" {\n enabled = false\n %s\n model \"m\" {}\n}", `"provider":{"openai":{"p":{"enabled":false,%s,"model":{"m":{}}}}}`},
		"model":    {"provider \"openai\" \"p\" {\n enabled = false\n model \"m\" {\n %s\n }\n}", `"provider":[{"openai":{"p":[{"enabled":false,"model":{"m":[{%s}]}}]}}]`},
		"alias":    {"alias \"a\" {\n algorithm = \"round_robin\"\n %s\n}", `"alias":{"a":{"algorithm":"round_robin",%s}}`},
	} {
		t.Run(scope, func(t *testing.T) {
			for _, pair := range []struct{ hcl, json, want string }{
				{`upstream_headers = { X-F = "first", X-F = "` + sensitive + `" }`, `"upstream_headers":{"X-F":"first","X-F":"` + sensitive + `"}`, "duplicate header name"},
				{"upstream_headers = { X-F = \"first\" }\nupstream_headers = { X-S = \"" + sensitive + "\" }", `"upstream_headers":{"X-F":"first"},"upstream\u005fheaders":{"X-S":"` + sensitive + `"}`, "duplicate upstream_headers declaration"},
			} {
				assertUpstreamHeaderPreflightError(t, upstreamHeadersHCLBase+fmt.Sprintf(tc.hcl, pair.hcl)+"\n", "fixture.hcl", pair.want, sensitive)
				assertUpstreamHeaderPreflightError(t, "{"+upstreamHeadersJSONBase+","+fmt.Sprintf(tc.json, pair.json)+"}", "fixture.json", pair.want, sensitive)
			}
		})
	}
	assertUpstreamHeaderPreflightError(t, upstreamHeadersHCLBase+`upstream_headers { X-F = "`+sensitive+`" }`, "fixture.hcl", "map attribute", sensitive)
}

func assertUpstreamHeaderPreflightError(t *testing.T, src, filename, want, disallowed string) {
	t.Helper()
	_, loadErr := Load([]byte(src), filename)
	for _, err := range []error{loadErr, ValidateUpstreamHeaderDeclarations([]byte(src), filename)} {
		if err == nil || !strings.Contains(err.Error(), "upstream_headers") || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), disallowed) {
			t.Fatalf("%s: got %v, want safe upstream_headers error containing %q", filename, err, want)
		}
	}
}

func TestUpstreamHeaderReservedNames(t *testing.T) {
	const sensitive = "fixture-not-for-diagnostics"
	for _, name := range append([]string{"accept-encoding", "x-aiproxy-deferred", "x-aiproxy-deferred-custom"}, slices.Collect(maps.Keys(deniedForwardHeaders))...) {
		t.Run(name, func(t *testing.T) {
			assertUpstreamHeaderPreflightError(t, upstreamHeadersHCLBase+hclUpstreamHeaders(map[string]string{strings.ToUpper(name): sensitive}), "fixture.hcl", "managed by the proxy", sensitive)
		})
	}
	for _, tc := range []struct {
		providerType ProviderType
		names        []string
	}{
		{ProviderTypeOpenRouter, []string{"http-referer", "x-title"}},
		{ProviderTypeAnthropic, []string{"anthropic-version"}},
		{ProviderTypeOpenCodeZen, []string{"x-opencode-session", "x-opencode-client"}},
		{ProviderTypeOpenCodeGo, []string{"x-opencode-session", "x-opencode-client"}},
		{ProviderTypeGitHubCopilot, []string{"x-github-api-version", "openai-intent", "x-initiator", "copilot-vision-request", "cookie2", "anthropic-beta", "x-interaction-id", "x-interaction-type"}},
	} {
		for _, name := range tc.names {
			t.Run(string(tc.providerType)+"/"+name, func(t *testing.T) {
				var protocol string
				if IsOpenCodeProviderType(tc.providerType) {
					protocol = `protocol = "chat"` + "\n"
				}
				attr := hclUpstreamHeaders(map[string]string{strings.ToUpper(name): sensitive})
				for _, scope := range []string{"root", "provider", "model"} {
					var root, local, model string
					switch scope {
					case "root":
						root = attr
					case "provider":
						local = attr
					case "model":
						model = attr
					}
					src := fmt.Sprintf("%s%sprovider %q \"p\" {\n enabled = false\n%smodel \"m\" {\n%s%s}\n}", upstreamHeadersHCLBase, root, tc.providerType, local, protocol, model)
					_, err := Load([]byte(src), "fixture.hcl")
					if err == nil || !strings.Contains(err.Error(), "managed by the proxy") || strings.Contains(err.Error(), sensitive) {
						t.Fatalf("%s: got %v, want safe reserved-header rejection on disabled provider", scope, err)
					}
				}
			})
		}
	}
	const mixedTargets = `
provider "openai" "local" {
  api_key = "fixture"
  upstream_headers = { HTTP-Referer = "ok", X-Title = "ok", X-Opencode-Session = "ok" }
  model "m" {}
}
provider "openrouter" "router" {
  api_key = "fixture"
  model "m" {}
}
provider "anthropic" "native" {
  api_key = "fixture"
  upstream_headers = { Anthropic-Beta = "ok" }
  model "m" {}
}
provider "opencode-zen" "zen" {
  upstream_headers = { Anthropic-Version = "ok", X-Session-Id = "ok" }
  model "m" { protocol = "messages" }
}
alias "pool" {
  algorithm = "round_robin"
  providers = %s
  model = "m"
  upstream_headers = { x-TITLE = "` + sensitive + `" }
}
`
	for _, tc := range []struct{ targets, want string }{
		{`["local"]`, ""},
		{`["local", "router"]`, `alias "pool": target "router/m": upstream_headers name "X-Title" is managed by the proxy`},
	} {
		_, err := Load([]byte(upstreamHeadersHCLBase+fmt.Sprintf(mixedTargets, tc.targets)), "fixture.hcl")
		if tc.want == "" && err != nil {
			t.Fatalf("non-applicable adapter header rejected: %v", err)
		}
		if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), sensitive)) {
			t.Fatalf("got %v, want safe reserved-header rejection for one alias fallback", err)
		}
	}
}

func hclUpstreamHeaders(headers map[string]string) string {
	if headers == nil {
		return ""
	}
	var src strings.Builder
	src.WriteString("upstream_headers = {\n")
	for name, value := range headers {
		fmt.Fprintf(&src, " %q = %q\n", name, value)
	}
	src.WriteString("}\n")
	return src.String()
}

func TestUpstreamHeaderDeclarationBudgets(t *testing.T) {
	const sensitive = "fixture-not-for-diagnostics"
	names := map[string]string{}
	for i := range maxUpstreamHeaderCount {
		names[fmt.Sprintf("X-%02d", i)] = sensitive
	}
	extraName := maps.Clone(names)
	extraName["X-Extra"] = sensitive
	prefix := sensitive + "v"
	value := prefix + strings.Repeat("é", (maxUpstreamHeaderValueBytes-len(prefix))/2)
	name := "X-" + strings.Repeat("n", maxUpstreamHeaderNameBytes-2)
	bytes := map[string]string{}
	for i := range 4 {
		bytes[fmt.Sprintf("X-%02d", i)] = strings.Repeat("v", 4092)
	}
	extraByte := maps.Clone(bytes)
	extraByte["X-00"] += "v"
	for _, tc := range []struct {
		name, want string
		headers    map[string]string
	}{
		{"max names", "", names},
		{"too many names", "at most 32 names", extraName},
		{"max name and value bytes", "", map[string]string{name: value}},
		{"name too long", "at most 128 bytes", map[string]string{name + "n": sensitive}},
		{"value too long", "at most 4096 bytes", map[string]string{name: value + "v"}},
		{"max total bytes", "", bytes},
		{"one byte over total", "at most 16384 name/value bytes", extraByte},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jsonHeaders, err := json.Marshal(tc.headers)
			if err != nil {
				t.Fatal(err)
			}
			for format, src := range map[string]string{
				FormatHCL:  upstreamHeadersHCLBase + hclUpstreamHeaders(tc.headers),
				FormatJSON: "{" + upstreamHeadersJSONBase + `,"upstream_headers":` + string(jsonHeaders) + "}",
			} {
				if tc.want != "" {
					assertUpstreamHeaderPreflightError(t, src, "fixture."+format, tc.want, sensitive)
				} else if _, err := Load([]byte(src), "fixture."+format); err != nil {
					t.Fatalf("%s: rejected a valid boundary: %v", format, err)
				}
			}
		})
	}
}

func TestUpstreamHeaderResolvedBudgets(t *testing.T) {
	const sensitive = "fixture-not-for-diagnostics"
	names := map[string]string{}
	for i := range maxUpstreamHeaderCount {
		names[fmt.Sprintf("X-%02d", i)] = sensitive
	}
	large := sensitive + strings.Repeat("v", 4092-len(sensitive))
	rootBytes := map[string]string{"X-R0": large, "X-R1": large}
	providerBytes := map[string]string{"X-P0": large}
	modelBytes := map[string]string{"X-M0": large}
	for _, tc := range []struct {
		name, want                   string
		root, provider, model, alias map[string]string
		disabled, passthrough        bool
	}{
		{name: "case-folded override doesn't add a name", root: names, provider: map[string]string{"x-00": "small"}},
		{name: "direct model exceeds count", root: names, model: map[string]string{"X-M0": sensitive}, want: `model "m": effective upstream_headers must contain at most 32 names`},
		{name: "passthrough exceeds count", root: names, provider: map[string]string{"X-P0": sensitive}, passthrough: true, want: `model_passthrough: effective upstream_headers must contain at most 32 names`},
		{name: "disabled model exceeds count", root: names, model: map[string]string{"X-M0": sensitive}, disabled: true, want: `model "m": effective upstream_headers must contain at most 32 names`},
		{name: "alias exceeds count", root: names, alias: map[string]string{"X-A0": sensitive}, want: `alias "pool": target "p/m": effective upstream_headers must contain at most 32 names`},
		{name: "direct exactly 16 KiB", root: rootBytes, provider: providerBytes, model: modelBytes},
		{name: "direct exceeds bytes", root: rootBytes, provider: providerBytes, model: map[string]string{"X-M0": large + "v"}, want: `model "m": effective upstream_headers must contain at most 16384 name/value bytes`},
		{name: "alias override doesn't double-count bytes", root: rootBytes, provider: providerBytes, model: modelBytes, alias: map[string]string{"x-m0": large}},
		{name: "alias exceeds bytes", root: rootBytes, provider: providerBytes, model: modelBytes, alias: map[string]string{"x-m0": large + "v"}, want: `alias "pool": target "p/m": effective upstream_headers must contain at most 16384 name/value bytes`},
		{name: "higher model shortens value", root: rootBytes, provider: map[string]string{"X-P0": large, "X-M0": large + "v"}, model: modelBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := upstreamHeadersHCLBase + hclUpstreamHeaders(tc.root) + `provider "openai" "p" {` + "\n"
			src += fmt.Sprintf(" enabled = %t\n api_key = \"fixture\"\n", !tc.disabled) + hclUpstreamHeaders(tc.provider)
			if tc.passthrough {
				src += " model_passthrough \"rule\" {\n match = \"id-[0-9]+\"\n capabilities = [\"chat\"]\n }\n"
			} else {
				src += " model \"m\" {\n" + hclUpstreamHeaders(tc.model) + " }\n"
			}
			src += "}\n"
			if tc.alias != nil {
				src += "alias \"pool\" {\n algorithm = \"round_robin\"\n providers = [\"p\"]\n model = \"m\"\n" + hclUpstreamHeaders(tc.alias) + "}\n"
			}
			_, err := Load([]byte(src), "fixture.hcl")
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), sensitive) {
				t.Fatalf("got %v, want safe upstream_headers error containing %q", err, tc.want)
			}
		})
	}
}
