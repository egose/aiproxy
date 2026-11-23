package config

import (
	"maps"
	"strings"
	"testing"
)

const upstreamHeadersHCLDerived = `
upstream_headers = { X-Root = "root" }
provider "openai" "base" {
  api_key = "fixture-base"
  upstream_headers = { X-Base = "base" }
  model "m" {
    upstream_name = "upstream/m"
    upstream_headers = { X-Base-Model = "base model" }
  }
  model "n" { upstream_headers = { X-Base-Only = "base only" } }
  model "z" {}
}
provider "openai" "child" {
  extends = "base"
  api_key = "fixture-child"
  upstream_headers = { X-Child = "child" }
  model "m" {
    upstream_headers = { X-Child-Model = "child model" }
    encrypted_reasoning {}
  }
  model "z" { upstream_headers = {} }
}
provider "openai" "sibling" {
  extends = "base"
  api_key = "fixture-sibling"
}
`

const upstreamHeadersJSONDerived = `
  "upstream_headers":{"X-Root":"root"},
  "provider":{"openai":{
    "base":{"api_key":"fixture-base","upstream_headers":{"X-Base":"base"},"model":{
      "m":{"upstream_name":"upstream/m","upstream_headers":{"X-Base-Model":"base model"}},
      "n":{"upstream_headers":{"X-Base-Only":"base only"}},"z":{}}},
    "child":{"extends":"base","api_key":"fixture-child","upstream_headers":{"X-Child":"child"},"model":{
      "m":{"upstream_headers":{"X-Child-Model":"child model"},"encrypted_reasoning":{}},"z":{"upstream_headers":{}}}},
    "sibling":{"extends":"base","api_key":"fixture-sibling"}}},
`

func TestStaticDerivedUpstreamHeaderIsolation(t *testing.T) {
	for format, src := range map[string]string{
		FormatHCL:  upstreamHeadersHCLBase + upstreamHeadersHCLDerived,
		FormatJSON: "{" + upstreamHeadersJSONDerived + upstreamHeadersJSONBase + "}",
	} {
		t.Run(format, func(t *testing.T) {
			rt, err := Load([]byte(src), "fixture."+format)
			if err != nil {
				t.Fatal(err)
			}
			base := testProvider(t, rt, "base")
			child := testProvider(t, rt, "child")
			sibling := testProvider(t, rt, "sibling")
			if child.Extends != base.Name || child.APIKey != "fixture-child" || child.ModelByName["m"].UpstreamName != "upstream/m" || child.ModelByName["m"].EncryptedReasoning == nil {
				t.Fatal("header overlay changed existing inheritance behavior")
			}
			for _, tc := range []struct {
				provider Provider
				model    string
				want     map[string]string
			}{
				{base, "m", map[string]string{"X-Root": "root", "X-Base": "base", "X-Base-Model": "base model"}},
				{child, "m", map[string]string{"X-Root": "root", "X-Child": "child", "X-Child-Model": "child model"}},
				{child, "n", map[string]string{"X-Root": "root", "X-Child": "child"}},
				{child, "z", map[string]string{"X-Root": "root", "X-Child": "child"}},
				{sibling, "m", map[string]string{"X-Root": "root"}},
			} {
				if got := ResolveUpstreamHeaders(rt.UpstreamHeaders, tc.provider, tc.provider.ModelByName[tc.model], nil); !maps.Equal(got, tc.want) {
					t.Errorf("%s/%s = %q, want %q", tc.provider.Name, tc.model, got, tc.want)
				}
			}
			for _, model := range child.Models {
				indexed := child.ModelByName[model.Name]
				if model.UpstreamHeaders.declared() != indexed.UpstreamHeaders.declared() || !maps.Equal(model.UpstreamHeaders.Values(), indexed.UpstreamHeaders.Values()) {
					t.Fatalf("slice/index declarations differ for %s", model.Name)
				}
			}
			if !child.ModelByName["z"].UpstreamHeaders.declared() || child.ModelByName["n"].UpstreamHeaders.declared() || sibling.UpstreamHeaders.declared() {
				t.Fatal("derivative local intent or inheritance isolation lost")
			}
		})
	}
}

func TestStaticDerivedHeaderOverlayDoesNotAllowModelEdits(t *testing.T) {
	for _, tc := range []struct{ hclFrom, hclTo, jsonFrom, jsonTo, want string }{
		{`model "z" { upstream_headers = {} }`, `model "new" { upstream_headers = {} }`, `"z":{"upstream_headers":{}}`, `"new":{"upstream_headers":{}}`, "cannot declare new model"},
		{`model "z" { upstream_headers = {} }`, "model \"z\" {\n upstream_headers = {}\n display_name = \"\"\n }", `"z":{"upstream_headers":{}}`, `"z":{"upstream_headers":{},"display_name":null}`, "cannot declare model blocks with display_name"},
		{`model "z" { upstream_headers = {} }`, "model \"z\" { upstream_headers = {} }\n model \"z\" { upstream_headers = {} }", `"z":{"upstream_headers":{}}`, `"z":[{"upstream_headers":{}},{"upstream_headers":{}}]`, "duplicate model"},
	} {
		for format, src := range map[string]string{
			FormatHCL:  upstreamHeadersHCLBase + strings.Replace(upstreamHeadersHCLDerived, tc.hclFrom, tc.hclTo, 1),
			FormatJSON: "{" + strings.Replace(upstreamHeadersJSONDerived, tc.jsonFrom, tc.jsonTo, 1) + upstreamHeadersJSONBase + "}",
		} {
			if _, err := Load([]byte(src), "fixture."+format); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s: got %v, want error containing %q", format, err, tc.want)
			}
		}
	}
}

func TestManagedUpstreamHeaderDeclarationsAreRejected(t *testing.T) {
	declared := declaredUpstreamHeaders(map[string]string{"X-Fixture": "fixture"})
	empty := declaredUpstreamHeaders(map[string]string{})
	for _, p := range []Provider{
		{Name: "p", UpstreamHeaders: declared},
		{Name: "p", UpstreamHeaders: empty},
		{Name: "p", Models: []Model{{Name: "m", UpstreamHeaders: declared}}},
		{Name: "p", ModelByName: map[string]Model{"m": {Name: "m", UpstreamHeaders: declared}}},
	} {
		if err := ValidateDynamicProvider(p); err == nil || !strings.Contains(err.Error(), "upstream_headers is static-config-only") {
			t.Errorf("got %v, want static-only dynamic provider validation error", err)
		}
	}
	if err := ValidateDynamicAlias(Alias{Name: "a", DatabaseManaged: true, UpstreamHeaders: empty}, Catalog{}); err == nil || !strings.Contains(err.Error(), "upstream_headers is static-config-only") {
		t.Errorf("got %v, want static-only managed alias validation error", err)
	}
}
