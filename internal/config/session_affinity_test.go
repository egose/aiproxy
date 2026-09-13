package config

import (
	"strings"
	"testing"
)

func affinityTestConfig(aliasExtra string) string {
	return `
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "p1" {
  api_key = "sk-test"
  model "m" {}
}

provider "openai" "p2" {
  api_key = "sk-test"
  model "m" {}
}

alias "a" {
  algorithm = "round_robin"
` + aliasExtra + `
  target {
    provider = "p1"
    model    = "m"
  }

  target {
    provider = "p2"
    model    = "m"
  }
}
`
}

func TestLoadAliasWithoutSessionAffinity(t *testing.T) {
	rt, err := Load([]byte(affinityTestConfig("")), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := testAlias(t, rt, "a")
	if a.SessionAffinity != nil {
		t.Fatalf("SessionAffinity = %+v, want nil", a.SessionAffinity)
	}
	if got := SessionAffinityHeaders(a); got != nil {
		t.Fatalf("SessionAffinityHeaders = %v, want nil", got)
	}
}

func TestLoadAliasSessionAffinityDefaults(t *testing.T) {
	rt, err := Load([]byte(affinityTestConfig("  session_affinity {\n  }\n")), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := testAlias(t, rt, "a")
	if a.SessionAffinity == nil {
		t.Fatal("SessionAffinity is nil, want defaults")
	}
	got := SessionAffinityHeaders(a)
	if len(got) != len(DefaultSessionAffinityHeaders) {
		t.Fatalf("headers = %v, want defaults %v", got, DefaultSessionAffinityHeaders)
	}
	for i := range got {
		if got[i] != DefaultSessionAffinityHeaders[i] {
			t.Fatalf("headers = %v, want defaults %v", got, DefaultSessionAffinityHeaders)
		}
	}
}

func TestLoadAliasSessionAffinityCustomHeaders(t *testing.T) {
	rt, err := Load([]byte(affinityTestConfig("  session_affinity {\n    headers = [\"X-Custom-Session\", \" Session-Id \"]\n  }\n")), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := testAlias(t, rt, "a")
	got := SessionAffinityHeaders(a)
	want := []string{"x-custom-session", "session-id"}
	if len(got) != len(want) {
		t.Fatalf("headers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("headers = %v, want %v", got, want)
		}
	}
}

func TestLoadAliasSessionAffinityInvalid(t *testing.T) {
	cases := map[string]string{
		"empty header":      "  session_affinity {\n    headers = [\"\"]\n  }\n",
		"header with space": "  session_affinity {\n    headers = [\"bad header\"]\n  }\n",
		"header with colon": "  session_affinity {\n    headers = [\"bad:header\"]\n  }\n",
		"duplicate headers": "  session_affinity {\n    headers = [\"x-session-id\", \"X-Session-Id\"]\n  }\n",
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load([]byte(affinityTestConfig(extra)), "test.hcl"); err == nil {
				t.Fatal("expected load error, got nil")
			} else if !strings.Contains(err.Error(), "session_affinity") {
				t.Fatalf("error = %v, want session_affinity context", err)
			}
		})
	}
}

func TestLoadAliasSessionAffinityTooManyHeaders(t *testing.T) {
	var b strings.Builder
	b.WriteString("  session_affinity {\n    headers = [")
	for i := 0; i < 17; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(`"x-header-`)
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString(string(rune('0' + i/26)))
		b.WriteString(`"`)
	}
	b.WriteString("]\n  }\n")
	if _, err := Load([]byte(affinityTestConfig(b.String())), "test.hcl"); err == nil {
		t.Fatal("expected load error, got nil")
	}
}
