package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func documentedReasoningHCL(t *testing.T, page, name string) []byte {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "..", "website", "docs", page))
	if err != nil {
		t.Fatal(err)
	}
	fence := "```hcl title=\"" + name + "\"\n"
	if strings.Count(string(doc), fence) != 1 {
		t.Fatalf("expected one %s configuration in %s", name, page)
	}
	_, rest, _ := strings.Cut(string(doc), fence)
	source, _, ok := strings.Cut(rest, "\n```")
	if !ok {
		t.Fatalf("unterminated %s configuration", name)
	}
	return []byte(source + "\n")
}

func documentedReasoningExample(t *testing.T, name string) []byte {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "..", "examples", name))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func verifyDocumentedReasoning(t *testing.T, source []byte, check func(*testing.T, *Runtime)) {
	t.Helper()
	for key, value := range map[string]string{
		"AIPROXY_CLIENT_TOKEN":       "documentation-client-token",
		"UPSTREAM_API_KEY":           "documentation-primary-key",
		"SECONDARY_UPSTREAM_API_KEY": "documentation-secondary-key",
	} {
		t.Setenv(key, value)
	}
	want, err := Load(source, "reasoning.hcl")
	if err != nil {
		t.Fatalf("documented HCL does not load: %v", err)
	}
	if want.RequiresDB() {
		t.Fatal("documented reasoning fixture must not require a database")
	}
	check(t, want)
	for _, compact := range []bool{false, true} {
		jsonSource, _, _, err := Convert(source, "reasoning.hcl", compact)
		if err != nil {
			t.Fatalf("documented HCL does not convert to JSON: %v", err)
		}
		if bytes.Contains(jsonSource, []byte("env(")) {
			t.Fatal("conversion must resolve environment calls")
		}
		hclSource, _, _, err := Convert(jsonSource, "reasoning.json", false)
		if err != nil {
			t.Fatalf("documented JSON does not convert to HCL: %v", err)
		}
		for _, candidate := range []struct {
			name   string
			source []byte
		}{{"reasoning.json", jsonSource}, {"reasoning.hcl", hclSource}} {
			got, err := Load(candidate.source, candidate.name)
			if err != nil {
				t.Fatalf("converted documented %s does not load: %v", candidate.name, err)
			}
			if !reflect.DeepEqual(encryptedReasoningConvertStates(got), encryptedReasoningConvertStates(want)) {
				t.Fatalf("documented declarations/inheritance changed after %s conversion", candidate.name)
			}
			if got.Catalog.AliasCount() != want.Catalog.AliasCount() {
				t.Fatal("documented alias inventory changed after conversion")
			}
			for _, alias := range want.Catalog.Aliases() {
				converted, ok := got.Catalog.Alias(alias.Name)
				if !ok || !reflect.DeepEqual(converted, alias) {
					t.Fatalf("documented %s targets/policy changed after conversion", alias.Name)
				}
			}
			check(t, got)
		}
	}
}

func assertDocumentedReasoningDeclaration(t *testing.T, policy *EncryptedReasoning, want *EncryptedReasoningDeclaration) {
	t.Helper()
	if got := DeclaredEncryptedReasoning(policy); !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		t.Fatalf("documented sparse declaration = %s, want %s", gotJSON, wantJSON)
	}
}

func assertDocumentedScopedReasoning(t *testing.T, rt *Runtime) {
	t.Helper()
	assertDocumentedReasoningDeclaration(t, rt.EncryptedReasoning, &EncryptedReasoningDeclaration{
		OnCallerMismatch: reasoningString(EncryptedReasoningStripAndRetry),
		MatchMessages:    reasoningMessages("  Caller binding mismatch  "),
	})
	primary := testProvider(t, rt, "primary")
	secondary := testProvider(t, rt, "secondary")
	inherited := testProvider(t, rt, "inherited")
	rootOnly := testProvider(t, rt, "root-only")
	if len(rt.Catalog.Providers()) != 4 || len(primary.Models) != 3 ||
		len(secondary.Models) != 3 || len(inherited.Models) != 3 ||
		secondary.Extends != primary.Name || inherited.Extends != primary.Name ||
		secondary.BaseURL != primary.BaseURL || secondary.APIKey == primary.APIKey {
		t.Fatal("documented inventory, inheritance or local credential isolation changed")
	}
	assertDocumentedReasoningDeclaration(t, primary.EncryptedReasoning, &EncryptedReasoningDeclaration{Passthrough: reasoningBool(false)})
	assertDocumentedReasoningDeclaration(t, secondary.EncryptedReasoning, &EncryptedReasoningDeclaration{})
	assertDocumentedReasoningDeclaration(t, rootOnly.EncryptedReasoning, nil)
	if !inherited.EncryptedReasoningInherited || inherited.EncryptedReasoning == primary.EncryptedReasoning {
		t.Fatal("base provider declaration must be inherited by value")
	}
	assertDocumentedReasoningDeclaration(t, inherited.EncryptedReasoning, DeclaredEncryptedReasoning(primary.EncryptedReasoning))
	assertDocumentedReasoningDeclaration(t, primary.ModelByName["chat"].EncryptedReasoning, nil)
	assertDocumentedReasoningDeclaration(t, primary.ModelByName["recover"].EncryptedReasoning, &EncryptedReasoningDeclaration{
		OnCallerMismatch: reasoningString(EncryptedReasoningStripAndRetry),
	})
	assertDocumentedReasoningDeclaration(t, primary.ModelByName["safe"].EncryptedReasoning, &EncryptedReasoningDeclaration{})
	defaultMessages := []string{}
	assertDocumentedReasoningDeclaration(t, secondary.ModelByName["chat"].EncryptedReasoning, &EncryptedReasoningDeclaration{
		OnCallerMismatch: reasoningString(EncryptedReasoningStripAndRetry),
		MatchMessages:    &defaultMessages,
	})
	assertDocumentedReasoningDeclaration(t, secondary.ModelByName["safe"].EncryptedReasoning, &EncryptedReasoningDeclaration{Passthrough: reasoningBool(false)})
	if secondary.ModelByName["recover"].EncryptedReasoning == primary.ModelByName["recover"].EncryptedReasoning ||
		secondary.ModelByName["chat"].EncryptedReasoningInherited || secondary.ModelByName["safe"].EncryptedReasoningInherited {
		t.Fatal("documented local and by-value inherited model policies were conflated")
	}
	for _, tc := range []struct {
		provider, model, winner string
		source                  EncryptedReasoningSource
		inherited               bool
	}{
		{"primary", "chat", "strip", EncryptedReasoningSourceProvider, false},
		{"primary", "recover", "retry", EncryptedReasoningSourceModel, false},
		{"primary", "safe", "reset", EncryptedReasoningSourceModel, false},
		{"secondary", "chat", "retry", EncryptedReasoningSourceModel, false},
		{"secondary", "recover", "retry", EncryptedReasoningSourceModel, true},
		{"secondary", "safe", "strip", EncryptedReasoningSourceModel, false},
		{"inherited", "chat", "strip", EncryptedReasoningSourceProvider, true},
		{"inherited", "recover", "retry", EncryptedReasoningSourceModel, true},
		{"inherited", "safe", "reset", EncryptedReasoningSourceModel, true},
	} {
		p, m, ok := rt.Catalog.Model(tc.provider, tc.model)
		if !ok {
			t.Fatalf("documented model %s/%s disappeared", tc.provider, tc.model)
		}
		assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, p, m, nil), tc.winner, tc.source, tc.inherited)
	}
	for _, p := range rt.Catalog.Providers() {
		for _, m := range p.Models {
			if !reflect.DeepEqual(p.ModelByName[m.Name], m) {
				t.Fatal("documented ordered/indexed model inventory differs")
			}
		}
	}
	wantRoot := DefaultEncryptedReasoning()
	wantRoot.OnCallerMismatch = EncryptedReasoningStripAndRetry
	wantRoot.MatchMessages = []string{"caller binding mismatch"}
	gotRoot := ResolveEncryptedReasoning(rt.EncryptedReasoning, rootOnly, rootOnly.ModelByName["chat"], nil)
	if gotRoot.Source != EncryptedReasoningSourceRoot || gotRoot.Inherited || !reflect.DeepEqual(gotRoot.Policy, wantRoot) {
		t.Fatalf("documented root fallback/normalization changed: %+v", gotRoot)
	}
	assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, secondary, Model{}, nil), "reset", EncryptedReasoningSourceProvider, false)
	for _, name := range []string{"safe_pool", "recover_pool", "target_policy"} {
		alias := testAlias(t, rt, name)
		switch name {
		case "safe_pool":
			assertDocumentedReasoningDeclaration(t, alias.EncryptedReasoning, &EncryptedReasoningDeclaration{})
		case "recover_pool":
			assertDocumentedReasoningDeclaration(t, alias.EncryptedReasoning, &EncryptedReasoningDeclaration{OnCallerMismatch: reasoningString(EncryptedReasoningStripAndRetry)})
		case "target_policy":
			assertDocumentedReasoningDeclaration(t, alias.EncryptedReasoning, nil)
		}
		for _, target := range alias.Targets {
			p, m, ok := rt.Catalog.Model(target.Provider, target.Model)
			if !ok {
				t.Fatal("documented alias target disappeared")
			}
			winner, scope := "reset", EncryptedReasoningSourceAlias
			if name == "recover_pool" {
				winner = "retry"
			} else if name == "target_policy" {
				winner, scope = "strip", EncryptedReasoningSourceProvider
				if target.Provider == "secondary" {
					winner, scope = "retry", EncryptedReasoningSourceModel
				}
			}
			assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, p, m, &alias), winner, scope, false)
		}
	}
}

func TestDocumentedEncryptedReasoningScopedConfiguration(t *testing.T) {
	source := documentedReasoningHCL(t, "configuration.md", "scoped-encrypted-reasoning.hcl")
	if !bytes.Equal(source, documentedReasoningExample(t, "scoped-encrypted-reasoning.hcl")) {
		t.Fatal("scoped example and exact documented configuration drifted")
	}
	verifyDocumentedReasoning(t, source, assertDocumentedScopedReasoning)
}

func TestDocumentedEncryptedReasoningRoutingAlias(t *testing.T) {
	source := documentedReasoningHCL(t, "configuration.md", "scoped-encrypted-reasoning.hcl")
	source = append(source, documentedReasoningHCL(t, "providers-and-routing.md", "reasoning-alias.hcl")...)
	verifyDocumentedReasoning(t, source, func(t *testing.T, rt *Runtime) {
		assertDocumentedScopedReasoning(t, rt)
		alias := testAlias(t, rt, "routing_recovery")
		if len(alias.Targets) != 2 {
			t.Fatal("documented explicit-target alias lost a target")
		}
		assertDocumentedReasoningDeclaration(t, alias.EncryptedReasoning, &EncryptedReasoningDeclaration{OnCallerMismatch: reasoningString(EncryptedReasoningStripAndRetry)})
		for _, target := range alias.Targets {
			p, m, ok := rt.Catalog.Model(target.Provider, target.Model)
			if !ok {
				t.Fatal("documented routing target does not exist")
			}
			assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, p, m, &alias), "retry", EncryptedReasoningSourceAlias, false)
		}
	})
}

func TestDocumentedEncryptedReasoningAliasExample(t *testing.T) {
	verifyDocumentedReasoning(t, documentedReasoningExample(t, "alias-encrypted-reasoning.hcl"), func(t *testing.T, rt *Runtime) {
		if rt.EncryptedReasoning != nil || len(rt.Catalog.Providers()) != 2 {
			t.Fatal("legacy alias example must retain built-in direct behavior")
		}
		for _, tc := range []struct{ alias, winner string }{{"chat_default", "retry"}, {"chat_strict", "strip"}} {
			alias := testAlias(t, rt, tc.alias)
			if len(alias.Targets) != 2 {
				t.Fatal("alias example lost a target")
			}
			want := &EncryptedReasoningDeclaration{Passthrough: reasoningBool(false)}
			if tc.winner == "retry" {
				want.Passthrough = reasoningBool(true)
				want.OnCallerMismatch = reasoningString(EncryptedReasoningStripAndRetry)
			}
			assertDocumentedReasoningDeclaration(t, alias.EncryptedReasoning, want)
			for _, target := range alias.Targets {
				p, m, ok := rt.Catalog.Model(target.Provider, target.Model)
				if !ok {
					t.Fatal("alias example target does not exist")
				}
				assertDocumentedReasoningDeclaration(t, p.EncryptedReasoning, nil)
				assertDocumentedReasoningDeclaration(t, m.EncryptedReasoning, nil)
				assertReasoningResolution(t, ResolveEncryptedReasoning(nil, p, m, nil), "reset", EncryptedReasoningSourceBuiltin, false)
				assertReasoningResolution(t, ResolveEncryptedReasoning(nil, p, m, &alias), tc.winner, EncryptedReasoningSourceAlias, false)
			}
		}
	})
}
