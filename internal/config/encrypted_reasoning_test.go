package config

import (
	"strings"
	"testing"
)

func encryptedReasoningTestConfig(aliasExtra string) string {
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

func TestLoadAliasWithoutEncryptedReasoning(t *testing.T) {
	rt, err := Load([]byte(encryptedReasoningTestConfig("")), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := testAlias(t, rt, "a")
	if a.EncryptedReasoning != nil {
		t.Fatalf("EncryptedReasoning = %+v, want nil", a.EncryptedReasoning)
	}
	got := EncryptedReasoningMatchMessages(a)
	if len(got) != len(DefaultEncryptedReasoningMatchMessages) {
		t.Fatalf("match messages = %v, want defaults %v", got, DefaultEncryptedReasoningMatchMessages)
	}
}

func TestLoadAliasEncryptedReasoningFull(t *testing.T) {
	extra := `  encrypted_reasoning {
    passthrough = false
    on_caller_mismatch = "strip_and_retry"
    match_messages = ["Not Issued To This Caller", "custom-marker-text"]
  }
`
	rt, err := Load([]byte(encryptedReasoningTestConfig(extra)), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := testAlias(t, rt, "a")
	er := a.EncryptedReasoning
	if er == nil {
		t.Fatal("EncryptedReasoning is nil, want block")
	}
	if er.Passthrough {
		t.Fatalf("Passthrough = true, want false")
	}
	if er.OnCallerMismatch != EncryptedReasoningStripAndRetry {
		t.Fatalf("OnCallerMismatch = %q, want strip_and_retry", er.OnCallerMismatch)
	}
	want := []string{"not issued to this caller", "custom-marker-text"}
	if len(er.MatchMessages) != len(want) {
		t.Fatalf("match messages = %v, want %v", er.MatchMessages, want)
	}
	for i := range want {
		if er.MatchMessages[i] != want[i] {
			t.Fatalf("match messages = %v, want %v", er.MatchMessages, want)
		}
	}
}

func TestLoadAliasEncryptedReasoningDefaults(t *testing.T) {
	extra := "  encrypted_reasoning {\n  }\n"
	rt, err := Load([]byte(encryptedReasoningTestConfig(extra)), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := testAlias(t, rt, "a")
	er := a.EncryptedReasoning
	if er == nil {
		t.Fatal("EncryptedReasoning is nil, want block")
	}
	if !er.Passthrough {
		t.Fatalf("Passthrough = false, want true")
	}
	if er.OnCallerMismatch != EncryptedReasoningFail {
		t.Fatalf("OnCallerMismatch = %q, want %q", er.OnCallerMismatch, EncryptedReasoningFail)
	}
	if len(er.MatchMessages) != len(DefaultEncryptedReasoningMatchMessages) {
		t.Fatalf("match messages = %v, want defaults", er.MatchMessages)
	}
}

func TestLoadAliasEncryptedReasoningInvalid(t *testing.T) {
	cases := map[string]string{
		"bad policy":     "  encrypted_reasoning {\n    on_caller_mismatch = \"sometimes\"\n  }\n",
		"empty entry":    "  encrypted_reasoning {\n    match_messages = [\"\"]\n  }\n",
		"short entry":    "  encrypted_reasoning {\n    match_messages = [\"abc\"]\n  }\n",
		"duplicate case": "  encrypted_reasoning {\n    match_messages = [\"Custom-Marker-Text\", \"custom-marker-text\"]\n  }\n",
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load([]byte(encryptedReasoningTestConfig(extra)), "test.hcl"); err == nil {
				t.Fatal("expected load error, got nil")
			} else if !strings.Contains(err.Error(), "encrypted_reasoning") {
				t.Fatalf("error = %v, want encrypted_reasoning context", err)
			}
		})
	}
}

func TestLoadAliasEncryptedReasoningTooManyEntries(t *testing.T) {
	var b strings.Builder
	b.WriteString("  encrypted_reasoning {\n    match_messages = [")
	for i := 0; i < 17; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(`"custom-marker-`)
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString(string(rune('0' + i/26)))
		b.WriteString(`-text"`)
	}
	b.WriteString("]\n  }\n")
	if _, err := Load([]byte(encryptedReasoningTestConfig(b.String())), "test.hcl"); err == nil {
		t.Fatal("expected load error, got nil")
	}
}
