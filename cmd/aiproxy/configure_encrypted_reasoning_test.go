package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/configedit"
)

const reasoningConfigureHeader = `listener "http" "public" { address = "127.0.0.1:8080" }
auth "main" { mode = "none" }
`

const reasoningConfigureProvider = `provider "openai" "primary" {
  api_key = env("CLI_REASONING_KEY")
  model "m" { upstream_name = "upstream/m" }
  model "n" { upstream_name = "upstream/n" }
}
`

const reasoningConfigureAlias = `alias "a" {
  algorithm = "round_robin"
  target {
    provider = "primary"
    model = "m"
  }
}
`

type reasoningConfigureScope struct {
	name  string
	args  []string
	scope configedit.EncryptedReasoningScope
}

func reasoningConfigureScopes() []reasoningConfigureScope {
	return []reasoningConfigureScope{
		{"root", []string{"upstream"}, configedit.EncryptedReasoningScope{}},
		{"provider", []string{"provider", "--name", "primary"}, configedit.EncryptedReasoningScope{Provider: "primary"}},
		{"model", []string{"provider", "--name", "primary"}, configedit.EncryptedReasoningScope{Provider: "primary", Model: "m"}},
		{"alias", []string{"alias", "--name", "a"}, configedit.EncryptedReasoningScope{Alias: "a"}},
	}
}

func reasoningConfigureFixture(t *testing.T, source string) string {
	t.Helper()
	t.Setenv("CLI_REASONING_KEY", "synthetic-key")
	t.Setenv("CLI_ER_PASS", "true")
	t.Setenv("CLI_ER_MISMATCH", "strip_and_retry")
	t.Setenv("CLI_ER_PATTERN", "Raw Marker")
	path := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func reasoningConfigureSource(t *testing.T, path string) string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func runReasoningConfigure(t *testing.T, path string, args ...string) *config.Runtime {
	t.Helper()
	command := append([]string{"configure"}, args...)
	command = append(command, "--config", path, "--non-interactive")
	stdout, stderr, err := executeRootCommand("", command...)
	if err != nil {
		t.Fatalf("configure %v: %v\n%s\n%s", args, err, stdout, stderr)
	}
	rt, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("emitted config does not load: %v", err)
	}
	return rt
}

func reasoningConfigureDeclaration(t *testing.T, rt *config.Runtime, scope configedit.EncryptedReasoningScope) *config.EncryptedReasoningDeclaration {
	t.Helper()
	if scope.Alias != "" {
		alias, ok := rt.Catalog.Alias(scope.Alias)
		if !ok {
			t.Fatalf("alias %s disappeared", scope.Alias)
		}
		return config.DeclaredEncryptedReasoning(alias.EncryptedReasoning)
	}
	if scope.Provider != "" {
		p, ok := rt.Catalog.Provider(scope.Provider)
		if !ok {
			for _, disabled := range rt.Catalog.DisabledProviders() {
				if disabled.Name == scope.Provider {
					p, ok = disabled, true
				}
			}
		}
		if !ok {
			t.Fatalf("provider %s disappeared", scope.Provider)
		}
		if scope.Model != "" {
			model, ok := p.ModelByName[scope.Model]
			if !ok {
				t.Fatalf("model %s disappeared", scope.Model)
			}
			return config.DeclaredEncryptedReasoning(model.EncryptedReasoning)
		}
		return config.DeclaredEncryptedReasoning(p.EncryptedReasoning)
	}
	return config.DeclaredEncryptedReasoning(rt.EncryptedReasoning)
}

func reasoningScopeFlag(scope reasoningConfigureScope, member, value string) []string {
	if scope.scope.Model != "" {
		switch member {
		case "remove":
			return []string{"--no-model-reasoning", scope.scope.Model}
		case "reset":
			return []string{"--reset-model-reasoning", scope.scope.Model}
		default:
			return []string{"--model-reasoning-" + member, scope.scope.Model + "=" + value}
		}
	}
	switch member {
	case "remove":
		return []string{"--no-encrypted-reasoning"}
	case "reset":
		return []string{"--reset-encrypted-reasoning"}
	case "passthrough":
		return []string{"--encrypted-reasoning-passthrough=" + value}
	default:
		return []string{"--encrypted-reasoning-" + member + "=" + value}
	}
}

func TestConfigureEncryptedReasoningAllScopeIntentAndPublication(t *testing.T) {
	for _, scope := range reasoningConfigureScopes() {
		t.Run(scope.name, func(t *testing.T) {
			path := reasoningConfigureFixture(t, reasoningConfigureHeader+reasoningConfigureProvider+reasoningConfigureAlias)
			edit := func(flags ...string) *config.Runtime {
				return runReasoningConfigure(t, path, append(append([]string{}, scope.args...), flags...)...)
			}
			before := reasoningConfigureSource(t, path)
			if declaration := reasoningConfigureDeclaration(t, edit(), scope.scope); declaration != nil {
				t.Fatal("omitted flags materialized defaults")
			}
			edit(reasoningScopeFlag(scope, "remove", "")...)
			if got := reasoningConfigureSource(t, path); got != before {
				t.Fatal("no-op/removal of an absent block rewrote the source")
			}
			rt := edit(reasoningScopeFlag(scope, "passthrough", "false")...)
			want := &config.EncryptedReasoningDeclaration{Passthrough: boolPtr(false)}
			if got := reasoningConfigureDeclaration(t, rt, scope.scope); !reflect.DeepEqual(got, want) {
				t.Fatalf("false did not remain a one-member local block: %+v", got)
			}
			flags := reasoningScopeFlag(scope, "on-caller-mismatch", "")
			flags = append(flags, reasoningScopeFlag(scope, "match-message", "")...)
			rt = edit(flags...)
			empty := ""
			messages := []string{}
			want.OnCallerMismatch, want.MatchMessages = &empty, &messages
			if got := reasoningConfigureDeclaration(t, rt, scope.scope); !reflect.DeepEqual(got, want) {
				t.Fatalf("explicit empty members were omitted or replaced by normalized values: %+v", got)
			}
			before = reasoningConfigureSource(t, path)
			edit()
			if got := reasoningConfigureSource(t, path); got != before {
				t.Fatal("no-op lost explicit false/empty intent")
			}
			flags = reasoningScopeFlag(scope, "on-caller-mismatch", "strip_and_retry")
			flags = append(flags, reasoningScopeFlag(scope, "match-message", "Caller Marker")...)
			flags = append(flags, reasoningScopeFlag(scope, "match-message", "Signature Marker")...)
			rt = edit(flags...)
			mismatch := "strip_and_retry"
			messages = []string{"Caller Marker", "Signature Marker"}
			want.OnCallerMismatch, want.MatchMessages = &mismatch, &messages
			if got := reasoningConfigureDeclaration(t, rt, scope.scope); !reflect.DeepEqual(got, want) {
				t.Fatalf("member edits did not preserve the other members: %+v", got)
			}
			rt = edit(reasoningScopeFlag(scope, "reset", "")...)
			if got := reasoningConfigureDeclaration(t, rt, scope.scope); !reflect.DeepEqual(got, &config.EncryptedReasoningDeclaration{}) {
				t.Fatalf("reset is not an explicit empty block: %+v", got)
			}
			rt = edit(reasoningScopeFlag(scope, "remove", "")...)
			if got := reasoningConfigureDeclaration(t, rt, scope.scope); got != nil {
				t.Fatalf("remove did not inherit: %+v", got)
			}
			p, _ := rt.Catalog.Provider("primary")
			if p.ModelByName["m"].UpstreamName != "upstream/m" || p.ModelByName["n"].UpstreamName != "upstream/n" {
				t.Fatal("removing a policy removed or replaced a concrete model definition")
			}
		})
	}
}

func TestConfigureEncryptedReasoningWholeBlockPrecedence(t *testing.T) {
	path := reasoningConfigureFixture(t, reasoningConfigureHeader+reasoningConfigureProvider+reasoningConfigureAlias)
	rt := runReasoningConfigure(t, path, "upstream", "--encrypted-reasoning-passthrough=false", "--encrypted-reasoning-match-message", "Root Marker")
	p, _ := rt.Catalog.Provider("primary")
	if got := config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], nil); got.Source != config.EncryptedReasoningSourceRoot || got.Policy.Passthrough {
		t.Fatal("root policy did not become the fallback")
	}
	rt = runReasoningConfigure(t, path, "provider", "--name", "primary", "--encrypted-reasoning-on-caller-mismatch", "strip_and_retry")
	p, _ = rt.Catalog.Provider("primary")
	got := config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], nil)
	if got.Source != config.EncryptedReasoningSourceProvider || !got.Policy.Passthrough || !reflect.DeepEqual(got.Policy.MatchMessages, config.DefaultEncryptedReasoningMatchMessages) {
		t.Fatal("provider member edit copied lower-layer members instead of replacing the whole block")
	}
	rt = runReasoningConfigure(t, path, "provider", "--name", "primary", "--reset-model-reasoning", "m")
	p, _ = rt.Catalog.Provider("primary")
	got = config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], nil)
	if got.Source != config.EncryptedReasoningSourceModel || !reflect.DeepEqual(got.Policy, config.DefaultEncryptedReasoning()) {
		t.Fatal("model reset did not override the provider/root block")
	}
	rt = runReasoningConfigure(t, path, "alias", "--name", "a", "--encrypted-reasoning-passthrough=false")
	p, _ = rt.Catalog.Provider("primary")
	a, _ := rt.Catalog.Alias("a")
	got = config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], &a)
	if got.Source != config.EncryptedReasoningSourceAlias || got.Policy.Passthrough {
		t.Fatal("legacy alias flag did not override all lower layers")
	}
	rt = runReasoningConfigure(t, path, "alias", "--name", "a", "--reset-encrypted-reasoning")
	p, _ = rt.Catalog.Provider("primary")
	a, _ = rt.Catalog.Alias("a")
	got = config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], &a)
	if got.Source != config.EncryptedReasoningSourceAlias || !reflect.DeepEqual(got.Policy, config.DefaultEncryptedReasoning()) {
		t.Fatal("alias reset was not a whole-block safe reset")
	}
	rt = runReasoningConfigure(t, path, "alias", "--name", "a", "--no-encrypted-reasoning")
	p, _ = rt.Catalog.Provider("primary")
	a, _ = rt.Catalog.Alias("a")
	got = config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], &a)
	if got.Source != config.EncryptedReasoningSourceModel {
		t.Fatal("alias removal did not resume selected-target policy resolution")
	}
}

func TestConfigureEncryptedReasoningProviderAndAliasCreationAndModelReconciliation(t *testing.T) {
	path := reasoningConfigureFixture(t, reasoningConfigureHeader+reasoningConfigureProvider)
	rt := runReasoningConfigure(t, path, "provider", "--name", "created", "--type", "openai", "--api-key-env", "CLI_REASONING_KEY",
		"--model", "first", "--model", "second", "--encrypted-reasoning-passthrough=false",
		"--model-reasoning-on-caller-mismatch", "first=strip_and_retry", "--reset-model-reasoning", "second")
	p, _ := rt.Catalog.Provider("created")
	if p.EncryptedReasoning == nil || p.EncryptedReasoning.Passthrough || p.ModelByName["first"].EncryptedReasoning.OnCallerMismatch != config.EncryptedReasoningStripAndRetry ||
		!reflect.DeepEqual(config.DeclaredEncryptedReasoning(p.ModelByName["second"].EncryptedReasoning), &config.EncryptedReasoningDeclaration{}) {
		t.Fatal("provider/model create flags did not publish local intent")
	}
	rt = runReasoningConfigure(t, path, "provider", "--name", "created", "--model", "first=updated/first", "--model", "second",
		"--model-reasoning-passthrough", "first=false", "--no-model-reasoning", "second")
	p, _ = rt.Catalog.Provider("created")
	if p.ModelByName["first"].UpstreamName != "updated/first" || p.ModelByName["first"].EncryptedReasoning.Passthrough ||
		p.ModelByName["first"].EncryptedReasoning.OnCallerMismatch != config.EncryptedReasoningStripAndRetry || p.ModelByName["second"].EncryptedReasoning != nil {
		t.Fatal("model definitions and member edits were not reconciled by retained model name")
	}
	runReasoningConfigure(t, path, "provider", "--name", "primary", "--model", "m=upstream/m", "--model", "n=upstream/n", "--model", "first")
	rt = runReasoningConfigure(t, path, "alias", "--name", "created_alias", "--algorithm", "least_connections", "--providers", "primary,created", "--model", "first",
		"--encrypted-reasoning-passthrough=false", "--encrypted-reasoning-on-caller-mismatch", "strip_and_retry", "--encrypted-reasoning-match-message", "Custom Marker")
	a, _ := rt.Catalog.Alias("created_alias")
	if a.EncryptedReasoning == nil || a.EncryptedReasoning.Passthrough || len(a.Targets) != 2 {
		t.Fatal("alias shorthand creation lost legacy reasoning flags")
	}
	rt = runReasoningConfigure(t, path, "alias", "--name", "explicit_alias", "--algorithm", "round_robin", "--target", "created/first",
		"--encrypted-reasoning-on-caller-mismatch=", "--encrypted-reasoning-match-message=")
	a, _ = rt.Catalog.Alias("explicit_alias")
	if declaration := config.DeclaredEncryptedReasoning(a.EncryptedReasoning); declaration == nil || declaration.Passthrough != nil ||
		declaration.OnCallerMismatch == nil || *declaration.OnCallerMismatch != "" || declaration.MatchMessages == nil || len(*declaration.MatchMessages) != 0 {
		t.Fatal("explicit-target alias creation lost empty member intent or invented passthrough")
	}
	rt = runReasoningConfigure(t, path, "provider", "--name", "created", "--model", "first")
	p, _ = rt.Catalog.Provider("created")
	if len(p.Models) != 1 || p.ModelByName["first"].EncryptedReasoning == nil {
		t.Fatal("model replacement retained removed definitions or lost a retained policy")
	}
}

func TestConfigureEncryptedReasoningBareModelsDisabledProvidersAndByteBounds(t *testing.T) {
	source := reasoningConfigureHeader + `provider "openai" "primary" {
  api_key = env("CLI_REASONING_KEY")
  model "family/m" {}
}
`
	path := reasoningConfigureFixture(t, source)
	rt := runReasoningConfigure(t, path, "provider", "--name", "primary", "--model-reasoning-match-message", "family/m=éé")
	p, _ := rt.Catalog.Provider("primary")
	if p.ModelByName["family/m"].EncryptedReasoning.MatchMessages[0] != "éé" {
		t.Fatal("valid four-byte pattern or slash-segment model name was rejected")
	}
	rt = runReasoningConfigure(t, path, "provider", "--name", "primary", "--no-model-reasoning", "family/m")
	p, _ = rt.Catalog.Provider("primary")
	if len(p.Models) != 1 || p.ModelByName["family/m"].EncryptedReasoning != nil {
		t.Fatal("reasoning removal deleted a bare concrete model")
	}
	rt = runReasoningConfigure(t, path, "provider", "--name", "disabled", "--type", "openai", "--enabled=false", "--model", "m", "--encrypted-reasoning-passthrough=false")
	if declaration := reasoningConfigureDeclaration(t, rt, configedit.EncryptedReasoningScope{Provider: "disabled"}); declaration == nil || declaration.Passthrough == nil || *declaration.Passthrough {
		t.Fatal("disabled provider creation lost local reasoning intent")
	}
	rt = runReasoningConfigure(t, path, "provider", "--name", "disabled", "--display-name", "Still disabled")
	if declaration := reasoningConfigureDeclaration(t, rt, configedit.EncryptedReasoningScope{Provider: "disabled"}); declaration == nil || declaration.Passthrough == nil || *declaration.Passthrough {
		t.Fatal("unrelated disabled-provider edit lost the declared policy")
	}
	before := reasoningConfigureSource(t, path)
	for _, message := range []string{"é", strings.Repeat("é", 129), " abc "} {
		if _, _, err := executeRootCommand("", "configure", "provider", "--config", path, "--non-interactive", "--name", "primary",
			"--model-reasoning-match-message", "family/m="+message); err == nil || reasoningConfigureSource(t, path) != before {
			t.Fatal("normalized byte bounds did not reject the model pattern before publication")
		}
	}
}

func TestConfigureEncryptedReasoningHelpAndExamples(t *testing.T) {
	for _, command := range []string{"upstream", "provider", "alias"} {
		stdout, _, err := executeRootCommand("", "configure", command, "--help")
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"--encrypted-reasoning-passthrough", "--encrypted-reasoning-on-caller-mismatch", "--encrypted-reasoning-match-message",
			"--no-encrypted-reasoning", "--reset-encrypted-reasoning", "whole block", "single empty", "inherit", "{}"} {
			if !strings.Contains(stdout, text) {
				t.Fatalf("%s help is missing %q", command, text)
			}
		}
		if command == "provider" {
			for _, text := range []string{"--model-reasoning-passthrough", "--model-reasoning-on-caller-mismatch", "--model-reasoning-match-message",
				"--no-model-reasoning", "--reset-model-reasoning", "name=true|false", "name=fail|strip_and_retry", "name=substring", "policy-only overlays", "replacement semantics"} {
				if !strings.Contains(stdout, text) {
					t.Fatalf("provider help is missing %q", text)
				}
			}
		}
	}
	examples := []struct {
		command string
		example string
		args    []string
	}{
		{"upstream", "aiproxy configure upstream --non-interactive --encrypted-reasoning-passthrough=false", []string{"upstream", "--encrypted-reasoning-passthrough=false"}},
		{"upstream", "aiproxy configure upstream --non-interactive --reset-encrypted-reasoning", []string{"upstream", "--reset-encrypted-reasoning"}},
		{"upstream", "aiproxy configure upstream --non-interactive --no-encrypted-reasoning", []string{"upstream", "--no-encrypted-reasoning"}},
		{"provider", "aiproxy configure provider --non-interactive --name backup --encrypted-reasoning-on-caller-mismatch strip_and_retry", []string{"provider", "--name", "backup", "--encrypted-reasoning-on-caller-mismatch", "strip_and_retry"}},
		{"provider", "aiproxy configure provider --non-interactive --name backup --model-reasoning-passthrough qwen3-32b=false --model-reasoning-match-message 'qwen3-32b=invalid signature'", []string{"provider", "--name", "backup", "--model-reasoning-passthrough", "qwen3-32b=false", "--model-reasoning-match-message", "qwen3-32b=invalid signature"}},
		{"provider", "aiproxy configure provider --non-interactive --name backup --reset-model-reasoning qwen3-32b", []string{"provider", "--name", "backup", "--reset-model-reasoning", "qwen3-32b"}},
		{"provider", "aiproxy configure provider --non-interactive --name backup --no-model-reasoning qwen3-32b", []string{"provider", "--name", "backup", "--no-model-reasoning", "qwen3-32b"}},
		{"alias", "aiproxy configure alias --non-interactive --name chat_default --encrypted-reasoning-passthrough=false", []string{"alias", "--name", "chat_default", "--encrypted-reasoning-passthrough=false"}},
		{"alias", "aiproxy configure alias --non-interactive --name chat_default --reset-encrypted-reasoning", []string{"alias", "--name", "chat_default", "--reset-encrypted-reasoning"}},
	}
	source := strings.ReplaceAll(strings.ReplaceAll(reasoningConfigureProvider, "primary", "backup"), `"m"`, `"qwen3-32b"`)
	source += strings.ReplaceAll(strings.ReplaceAll(reasoningConfigureAlias, `"a"`, `"chat_default"`), `"primary"`, `"backup"`)
	source = strings.ReplaceAll(source, `model = "m"`, `model = "qwen3-32b"`)
	for _, example := range examples {
		t.Run(example.example, func(t *testing.T) {
			path := reasoningConfigureFixture(t, reasoningConfigureHeader+source)
			help, _, err := executeRootCommand("", "configure", example.command, "--help")
			if err != nil || !strings.Contains(help, example.example) {
				t.Fatal("executed example is not the published command example")
			}
			runReasoningConfigure(t, path, example.args...)
		})
	}
}

func TestConfigureEncryptedReasoningInvalidFlagsDoNotPublish(t *testing.T) {
	for _, scope := range reasoningConfigureScopes() {
		t.Run(scope.name, func(t *testing.T) {
			path := reasoningConfigureFixture(t, reasoningConfigureHeader+reasoningConfigureProvider+reasoningConfigureAlias)
			before := reasoningConfigureSource(t, path)
			cases := [][]string{
				append(reasoningScopeFlag(scope, "remove", ""), reasoningScopeFlag(scope, "reset", "")...),
				append(reasoningScopeFlag(scope, "remove", ""), reasoningScopeFlag(scope, "passthrough", "false")...),
				append(reasoningScopeFlag(scope, "reset", ""), reasoningScopeFlag(scope, "on-caller-mismatch", "")...),
				append(reasoningScopeFlag(scope, "reset", ""), reasoningScopeFlag(scope, "match-message", "")...),
				reasoningScopeFlag(scope, "on-caller-mismatch", "retry"),
				reasoningScopeFlag(scope, "match-message", "abc"),
				reasoningScopeFlag(scope, "match-message", strings.Repeat("a", 257)),
				append(reasoningScopeFlag(scope, "match-message", "Same"), reasoningScopeFlag(scope, "match-message", " SAME ")...),
				append(reasoningScopeFlag(scope, "match-message", ""), reasoningScopeFlag(scope, "match-message", "Valid Marker")...),
				reasoningScopeFlag(scope, "passthrough", "invalid"),
			}
			tooMany := []string{}
			for i := range 17 {
				tooMany = append(tooMany, reasoningScopeFlag(scope, "match-message", strings.Repeat("m", i+4))...)
			}
			cases = append(cases, tooMany)
			for _, flags := range cases {
				args := append([]string{"configure"}, scope.args...)
				args = append(args, "--config", path, "--non-interactive")
				args = append(args, flags...)
				if _, _, err := executeRootCommand("", args...); err == nil {
					t.Fatalf("invalid flags published: %v", flags)
				}
				if got := reasoningConfigureSource(t, path); got != before {
					t.Fatalf("invalid edit changed the config: %v", flags)
				}
			}
		})
	}
}

func TestConfigureEncryptedReasoningInvalidModelsAndStagedCredentialsDoNotPublish(t *testing.T) {
	path := reasoningConfigureFixture(t, reasoningConfigureHeader+reasoningConfigureProvider+reasoningConfigureAlias)
	secretsPath := filepath.Join(t.TempDir(), "keys.json")
	secrets := []byte(`{"primary":"unchanged-key"}`)
	if err := os.WriteFile(secretsPath, secrets, 0o600); err != nil {
		t.Fatal(err)
	}
	before := reasoningConfigureSource(t, path)
	cases := [][]string{
		{"--model-reasoning-passthrough", "missing=false"},
		{"--reset-model-reasoning", "missing"},
		{"--no-model-reasoning", "missing"},
		{"--model-reasoning-passthrough", "m=1"},
		{"--model-reasoning-passthrough", "m=false", "--model-reasoning-passthrough", "m=true"},
		{"--model-reasoning-on-caller-mismatch", "m=fail", "--model-reasoning-on-caller-mismatch", "m="},
		{"--model-reasoning-match-message", "m"},
		{"--model-reasoning-match-message", "=marker"},
		{"--model-reasoning-match-message", "Bad Name=marker"},
		{"--no-model-reasoning", "m", "--reset-model-reasoning", "m"},
		{"--reset-model-reasoning", "m", "--reset-model-reasoning", "m"},
		{"--no-model-reasoning", "m", "--model-reasoning-match-message", "m="},
		{"--model", "n", "--model-reasoning-passthrough", "m=false"},
		{"--delete", "--encrypted-reasoning-passthrough=false"},
		{"--delete", "--reset-model-reasoning", "m"},
		{"--encrypted-reasoning-on-caller-mismatch", "wrong"},
		{"--upstream-header-timeout", "-1s", "--reset-encrypted-reasoning"},
	}
	for _, flags := range cases {
		args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "primary",
			"--secrets-path", secretsPath, "--secrets-key", "primary", "--api-key", "staged-key"}
		args = append(args, flags...)
		if _, _, err := executeRootCommand("", args...); err == nil {
			t.Fatalf("invalid model/config/credential edit succeeded: %v", flags)
		}
		afterSecrets, err := os.ReadFile(secretsPath)
		if err != nil || !bytes.Equal(afterSecrets, secrets) || reasoningConfigureSource(t, path) != before {
			t.Fatalf("invalid edit published config or staged credentials: %v", flags)
		}
	}
}
