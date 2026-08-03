package dashrpc

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
)

func TestBuildSerializesAllLiveState(t *testing.T) {
	start := time.Now().Add(-30 * time.Second)
	providers := []config.Provider{
		{Name: "openai", DisplayName: "OpenAI", Models: []config.Model{{Name: "gpt-4o-mini"}}},
	}
	disabled := []config.Provider{
		{Name: "localai"},
	}
	aliases := []config.Alias{
		{Name: "chat", Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}}},
	}
	usage := accounting.NewAggregator()
	usage.Record(accounting.Event{Model: "openai/gpt-4o-mini", Operation: "chat", StatusCode: 200, Duration: 5 * time.Millisecond})
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(map[string]config.Provider{"openai": {Name: "openai"}})
	logs := observability.NewLogBuffer(10)
	logs.Add(observability.LogEntry{Level: slog.LevelInfo, Message: "hi"})

	snap := Build("v1", ":8080", "bearer_static", start, providers, disabled, aliases, usage, health, logs, 200)
	if snap.Version != "v1" || snap.Address != ":8080" || snap.AuthMode != "bearer_static" {
		t.Fatalf("identity fields wrong: %+v", snap)
	}
	if snap.StartTime != start {
		t.Fatalf("StartTime = %v, want %v", snap.StartTime, start)
	}
	if len(snap.Providers) != 1 || snap.Providers[0].Name != "openai" || snap.Providers[0].Models[0] != "gpt-4o-mini" {
		t.Fatalf("Providers mis-serialized: %+v", snap.Providers)
	}
	if len(snap.DisabledProviders) != 1 || snap.DisabledProviders[0].Name != "localai" {
		t.Fatalf("DisabledProviders mis-serialized: %+v", snap.DisabledProviders)
	}
	if len(snap.Aliases) != 1 || snap.Aliases[0].Targets[0].Provider != "openai" {
		t.Fatalf("Aliases mis-serialized: %+v", snap.Aliases)
	}
	if !snap.Health["openai"] {
		t.Fatalf("Health = %+v, want openai=true", snap.Health)
	}
	if len(snap.Usage) != 1 || snap.Usage[0].Model != "openai/gpt-4o-mini" {
		t.Fatalf("Usage mis-serialized: %+v", snap.Usage)
	}
	if len(snap.Logs) != 1 {
		t.Fatalf("Logs = %+v, want 1 entry", snap.Logs)
	}
	if snap.LastSeq != 1 {
		t.Fatalf("LastSeq = %d, want 1", snap.LastSeq)
	}
}

func TestBuildHandlesNilLiveState(t *testing.T) {
	snap := Build("v", ":1", "none", time.Now(), nil, nil, nil, nil, nil, nil, 200)
	if len(snap.Providers) != 0 || len(snap.Health) != 0 || len(snap.Usage) != 0 || len(snap.Logs) != 0 {
		t.Fatalf("expected zero state, got %+v", snap)
	}
	if snap.LastSeq != 0 {
		t.Fatalf("LastSeq = %d, want 0", snap.LastSeq)
	}
}

func TestTokenFilePathHonorsXDG(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	want := filepath.Join(xdg, "aiproxy", "dashboard.token")
	if got := TokenFilePath(); got != want {
		t.Fatalf("TokenFilePath = %q, want %q", got, want)
	}
}

func TestTokenFilePathFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".config", "aiproxy", "dashboard.token")
	if got := TokenFilePath(); got != want {
		t.Fatalf("TokenFilePath = %q, want %q", got, want)
	}
}

func TestMintTokenProducesHexOfAtLeast32Chars(t *testing.T) {
	t1, err := MintToken()
	if err != nil {
		t.Fatalf("MintToken err = %v", err)
	}
	if len(t1) < 32 {
		t.Fatalf("minted token too short: %q", t1)
	}
	for _, c := range t1 {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Fatalf("minted token contains non-hex char %q in %q", c, t1)
		}
	}
	t2, _ := MintToken()
	if t1 == t2 {
		t.Fatal("two consecutive MintToken calls should not produce the same value")
	}
}

func TestPersistTokenCreatesParentDirAndFile(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	tokenPath := TokenFilePath()
	if _, err := os.Stat(filepath.Dir(tokenPath)); err == nil {
		t.Fatalf("expected parent dir to not exist yet, but it does")
	}
	if err := PersistToken("abcdef"); err != nil {
		t.Fatalf("PersistToken err = %v", err)
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read persisted token: %v", err)
	}
	if strings.TrimSpace(string(data)) != "abcdef" {
		t.Fatalf("persisted contents = %q, want 'abcdef'", string(data))
	}
}

func TestLoadTokenRoundTripsThroughPersist(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if err := PersistToken("roundtrip-secret"); err != nil {
		t.Fatalf("PersistToken err = %v", err)
	}
	got, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken err = %v", err)
	}
	if got != "roundtrip-secret" {
		t.Fatalf("LoadToken = %q, want 'roundtrip-secret'", got)
	}
}

func TestLoadTokenErrorsWhenMissing(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if _, err := LoadToken(); err == nil {
		t.Fatal("LoadToken should error when token file is missing")
	}
}

func TestLoadTokenErrorsWhenEmpty(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	tokenPath := TokenFilePath()
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(); err == nil {
		t.Fatal("LoadToken should error when token file contains only whitespace")
	}
}
