package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

func copilotReloadConfig(upstreamURL, secretsPath, name string) string {
	return `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  base_url = "` + upstreamURL + `"
  credential_ref {
    path = "` + secretsPath + `"
    name = "` + name + `"
  }
  model "gpt-4o-mini" {}
}
`
}

type copilotRecorder struct {
	mu    sync.Mutex
	auths []string
}

func (r *copilotRecorder) handler(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	r.auths = append(r.auths, req.Header.Get("Authorization")+" "+string(body))
	r.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","choices":[]}`))
}

func (r *copilotRecorder) last(t *testing.T) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.auths) == 0 {
		t.Fatal("no upstream calls recorded")
	}
	return r.auths[len(r.auths)-1]
}

func postCopilotChat(t *testing.T, a *App, model string, wantStatus int) {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[]}`))
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != wantStatus {
		t.Fatalf("chat status = %d, want %d, body=%s", w.Code, wantStatus, w.Body.String())
	}
}

func TestReloadCopilotSidecarRotationActivatesOnReload(t *testing.T) {
	rec := &copilotRecorder{}
	upstream := httptest.NewServer(http.HandlerFunc(rec.handler))
	defer upstream.Close()

	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	writeCopilotSidecar(t, secretsPath, "main", "gho_token-a")

	configPath := writeConfigFile(t, copilotReloadConfig(upstream.URL, secretsPath, "main"))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}

	postCopilotChat(t, a, "copilot/gpt-4o-mini", http.StatusOK)
	if got := rec.last(t); !strings.Contains(got, "Bearer gho_token-a") {
		t.Fatalf("initial request = %s, want token-a", got)
	}

	writeCopilotSidecar(t, secretsPath, "main", "gho_token-b")
	postCopilotChat(t, a, "copilot/gpt-4o-mini", http.StatusOK)
	if got := rec.last(t); !strings.Contains(got, "Bearer gho_token-a") {
		t.Fatalf("key-file-only change must leave runtime unchanged until reload, got %s", got)
	}

	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	postCopilotChat(t, a, "copilot/gpt-4o-mini", http.StatusOK)
	if got := rec.last(t); !strings.Contains(got, "Bearer gho_token-b") {
		t.Fatalf("successful reload must activate new credential, got %s", got)
	}
}

func TestReloadCopilotInvalidCandidateLeavesRuntimeIntact(t *testing.T) {
	rec := &copilotRecorder{}
	upstream := httptest.NewServer(http.HandlerFunc(rec.handler))
	defer upstream.Close()

	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	writeCopilotSidecar(t, secretsPath, "main", "gho_good")

	configPath := writeConfigFile(t, copilotReloadConfig(upstream.URL, secretsPath, "main"))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	postCopilotChat(t, a, "copilot/gpt-4o-mini", http.StatusOK)

	sidecar, err := copilotlogin.SidecarPath(secretsPath, "main")
	if err != nil {
		t.Fatalf("sidecar path: %v", err)
	}
	if err := os.WriteFile(sidecar, []byte(`{invalid json`), 0o600); err != nil {
		t.Fatalf("corrupt sidecar: %v", err)
	}
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "credential_ref") {
		t.Fatalf("invalid reload error = %v, want credential_ref", err)
	}
	postCopilotChat(t, a, "copilot/gpt-4o-mini", http.StatusOK)
	if got := rec.last(t); !strings.Contains(got, "Bearer gho_good") {
		t.Fatalf("failed reload must leave old runtime intact, got %s", got)
	}
	if _, ok := a.Config.Catalog.Provider("copilot"); !ok {
		t.Fatal("failed reload dropped copilot provider")
	}
}

func TestReloadCopilotExpiredCredentialRollsBack(t *testing.T) {
	rec := &copilotRecorder{}
	upstream := httptest.NewServer(http.HandlerFunc(rec.handler))
	defer upstream.Close()

	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	writeCopilotSidecar(t, secretsPath, "main", "gho_good")

	configPath := writeConfigFile(t, copilotReloadConfig(upstream.URL, secretsPath, "main"))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}

	expired, err := copilotlogin.NewCredential("Ov23testclient", "gho_old", time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	expired.ExpiresAt = time.Now().Add(-time.Hour).Unix()
	if err := copilotlogin.Save(secretsPath, "main", expired); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired reload error = %v, want expired", err)
	}
	postCopilotChat(t, a, "copilot/gpt-4o-mini", http.StatusOK)
	if got := rec.last(t); !strings.Contains(got, "Bearer gho_good") {
		t.Fatalf("expired candidate must leave old runtime intact, got %s", got)
	}
}

func TestReloadCopilotDerivedIsolation(t *testing.T) {
	rec := &copilotRecorder{}
	upstream := httptest.NewServer(http.HandlerFunc(rec.handler))
	defer upstream.Close()

	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	writeCopilotSidecar(t, secretsPath, "base", "gho_base")
	writeCopilotSidecar(t, secretsPath, "team", "gho_team-a")

	configText := `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "github-copilot" "base" {
  base_url = "` + upstream.URL + `"
  credential_ref {
    path = "` + secretsPath + `"
    name = "base"
  }
  model "gpt-4o-mini" {
    upstream_name = "gpt-4o"
  }
}
provider "github-copilot" "derived" {
  extends = "base"
  credential_ref {
    path = "` + secretsPath + `"
    name = "team"
  }
}
`
	configPath := writeConfigFile(t, configText)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}

	postCopilotChat(t, a, "base/gpt-4o-mini", http.StatusOK)
	if got := rec.last(t); !strings.Contains(got, "Bearer gho_base") {
		t.Fatalf("base initial = %s", got)
	}
	postCopilotChat(t, a, "derived/gpt-4o-mini", http.StatusOK)
	if got := rec.last(t); !strings.Contains(got, "Bearer gho_team-a") {
		t.Fatalf("derived initial = %s", got)
	}

	writeCopilotSidecar(t, secretsPath, "team", "gho_team-b")
	if err := a.Reload(); err != nil {
		t.Fatalf("reload derived rotation: %v", err)
	}
	postCopilotChat(t, a, "base/gpt-4o-mini", http.StatusOK)
	if got := rec.last(t); !strings.Contains(got, "Bearer gho_base") {
		t.Fatalf("base must be unchanged after derived rotation, got %s", got)
	}
	postCopilotChat(t, a, "derived/gpt-4o-mini", http.StatusOK)
	if got := rec.last(t); !strings.Contains(got, "Bearer gho_team-b") {
		t.Fatalf("derived must use rotated credential, got %s", got)
	}
	derived, ok := a.Config.Catalog.Provider("derived")
	if !ok {
		t.Fatal("derived missing after reload")
	}
	if derived.CopilotToken == "" || derived.CopilotToken == mustProviderToken(t, a, "base") {
		t.Fatalf("derived leaked base credential: base=%q derived=%q", mustProviderToken(t, a, "base"), derived.CopilotToken)
	}
}

func mustProviderToken(t *testing.T, a *App, name string) string {
	t.Helper()
	p, ok := a.Config.Catalog.Provider(name)
	if !ok {
		t.Fatalf("provider %q missing", name)
	}
	return p.CopilotToken
}

func TestReloadCopilotOfflineValidationWithoutSideEffects(t *testing.T) {
	dir := t.TempDir()
	missingSecrets := filepath.Join(dir, "keys.json")
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  credential_ref {
    path = "`+missingSecrets+`"
    name = "absent"
  }
  model "gpt-4o-mini" {}
}
provider "openai" "backup" {
  api_key = "sk-backup"
  model "m" {}
}
`)
	_, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err == nil || !strings.Contains(err.Error(), "credential_ref") {
		t.Fatalf("expected offline credential_ref error, got %v", err)
	}
	if _, statErr := os.Stat(missingSecrets); !os.IsNotExist(statErr) {
		t.Fatalf("validation must not create secrets files")
	}
	sidecar, _ := copilotlogin.SidecarPath(missingSecrets, "absent")
	if _, statErr := os.Stat(sidecar); !os.IsNotExist(statErr) {
		t.Fatalf("validation must not create sidecar files")
	}
}
