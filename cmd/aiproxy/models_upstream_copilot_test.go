package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
)

func writeCopilotSidecar(t *testing.T, secretsPath, name, token string) {
	t.Helper()
	cred, err := copilotlogin.NewCredential("Ov23testclient", token, time.Now())
	if err != nil {
		t.Fatalf("new credential: %v", err)
	}
	if err := copilotlogin.Save(secretsPath, name, cred); err != nil {
		t.Fatalf("save credential: %v", err)
	}
}

func TestListUpstreamCopilotModelsHeadersAndOrigin(t *testing.T) {
	var gotPath, gotAuth, gotUA, gotVersion string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotVersion = r.Header.Get(copilotlogin.APIVersionHeader)
		if r.Header.Get("Cookie") != "" {
			t.Errorf("cookie header must not be sent")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4.1","display_name":"GPT 4.1"}]}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	writeCopilotSidecar(t, secretsPath, "main", "gho_test-token")

	cfg := writeModelsConfig(t, `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  base_url = "`+srv.URL+`"
  credential_ref {
    path = "`+secretsPath+`"
    name = "main"
  }
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runModels(ctx, cfg, "copilot", true, &stdout, &stderr); err != nil {
		t.Fatalf("runModels upstream: %v (stderr=%s)", err, stderr.String())
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/models" {
		t.Fatalf("path = %s, want /models", gotPath)
	}
	if gotAuth != "Bearer gho_test-token" {
		t.Fatalf("auth = %q, want Bearer gho_test-token", gotAuth)
	}
	if !strings.HasPrefix(gotUA, "aiproxy/") {
		t.Fatalf("user-agent = %q, want aiproxy/ prefix", gotUA)
	}
	if gotVersion != copilotlogin.APIVersion {
		t.Fatalf("api version = %q, want %q", gotVersion, copilotlogin.APIVersion)
	}
	out := stdout.String()
	for _, want := range []string{"gpt-4o-mini", "gpt-4.1", "configured as copilot/gpt-4o-mini", "not in config"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestListUpstreamCopilotModelsFailureSurfacesRelogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	writeCopilotSidecar(t, secretsPath, "main", "gho_revoked")

	cfg := writeModelsConfig(t, `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  base_url = "`+srv.URL+`"
  credential_ref {
    path = "`+secretsPath+`"
    name = "main"
  }
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runModels(ctx, cfg, "copilot", true, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for 401 upstream")
	}
	combined := stderr.String() + err.Error()
	if !strings.Contains(combined, "401") {
		t.Fatalf("error should mention 401, got: %s err=%v", combined, err)
	}
	if !strings.Contains(combined, "login") {
		t.Fatalf("401 should hint re-login, got: %s err=%v", combined, err)
	}
}

func TestListUpstreamCopilotModelsMissingCredentialMakesNoCall(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	provider := config.Provider{
		Type:    config.ProviderTypeGitHubCopilot,
		Name:    "copilot",
		BaseURL: srv.URL,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := listUpstreamModels(ctx, provider)
	if err == nil || !strings.Contains(err.Error(), "run login first") {
		t.Fatalf("expected missing-credential error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("missing credential must make zero upstream calls, got %d", calls)
	}
}

func TestListUpstreamCopilotModelsDefaultOrigin(t *testing.T) {
	provider := config.Provider{
		Type:         config.ProviderTypeGitHubCopilot,
		Name:         "copilot",
		CopilotToken: "gho_test",
	}
	if got := upstreamBaseURL(provider); got != copilotlogin.DefaultBaseURL {
		t.Fatalf("default origin = %q, want %q", got, copilotlogin.DefaultBaseURL)
	}
	if got := upstreamBaseURL(config.Provider{Type: config.ProviderTypeGitHubCopilot, Name: "c", BaseURL: "http://127.0.0.1:9", CopilotToken: "x"}); got != "http://127.0.0.1:9" {
		t.Fatalf("override origin not honored: %q", got)
	}
}
