package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/dashrpc"
)

func writeDashboardConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestRunDashboardErrorsWhenBlockAbsent(t *testing.T) {
	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runDashboard(context.Background(), cfg, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when dashboard block is absent")
	}
	if !strings.Contains(stderr.String(), "not configured") {
		t.Fatalf("stderr should mention 'not configured', got: %s", stderr.String())
	}
}

func TestRunDashboardErrorsWhenConfigInvalid(t *testing.T) {
	cfg := writeDashboardConfig(t, `invalid hcl >>>`)
	var stdout, stderr bytes.Buffer
	err := runDashboard(context.Background(), cfg, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	if !strings.Contains(err.Error(), "config") {
		t.Fatalf("err should mention config, got: %v", err)
	}
}

func TestRunDashboardAutoreadsPersistedTokenWhenBlockHasNoToken(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// Simulate a previously-spawned serve that minted a token to the file.
	// Use a real (non-refused) server so we can verify the dashboard command
	// actually authenticates with the persisted token. We wire up an httptest
	// server exposing the snapshot endpoint with the same token.
	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
dashboard {}
provider "openai" "openai" {
  api_key = "sk-dummy"
  model "gpt-4o-mini" {}
}
`)
	stub := newSnapshotStub(t, "stub-token")
	defer stub.Close()
	// Listen address needs to match the stub's actual port.
	// We rewrite the config with the stub's listener address.
	stubAddr := stub.Listener.Addr().String()
	cfg = writeDashboardConfig(t, `
listener "http" "public" { address = "`+stubAddr+`" }
auth "main" { mode = "none" }
dashboard {}
provider "openai" "openai" {
  api_key = "sk-dummy"
  model "gpt-4o-mini" {}
}
`)
	if err := dashrpc.PersistToken("stub-token"); err != nil {
		t.Fatalf("persist token: %v", err)
	}

	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = runDashboard(ctx, cfg, &stdout, &stderr)
	// runDashboard should have authenticated with "stub-token" and attached.
	// It returns when ctx is cancelled (TUI keeps running until then).
	if strings.Contains(stderr.String(), "no server running") {
		t.Fatalf("dashboard should reach the stub server, stderr=%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "unauthorized") {
		t.Fatalf("dashboard should authenticate with persisted token, stderr=%s", stderr.String())
	}
	// Sanity-check the stub received a snapshot request with the right token.
	if !stub.gotAuth("Bearer stub-token") {
		t.Fatal("stub never received an authenticated snapshot request")
	}
}

func TestRunDashboardErrorsWhenBlockHasNoTokenAndNoFile(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
dashboard {}
provider "openai" "openai" {
  api_key = "sk-dummy"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runDashboard(context.Background(), cfg, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when token file is missing")
	}
	if !strings.Contains(stderr.String(), "no persisted token") {
		t.Fatalf("stderr should mention 'no persisted token', got: %s", stderr.String())
	}
}

// snapshotStub is a minimal httptest server that mimics the dashboard
// snapshot endpoint with a fixed expected bearer token. It allows the
// dashboard command's persisted-token read path to be exercised end-to-end
// without running a real aiproxy serve.
type snapshotStub struct {
	*httptest.Server
	tokensSeen []string
}

func newSnapshotStub(t *testing.T, wantToken string) *snapshotStub {
	t.Helper()
	mux := http.NewServeMux()
	stub := &snapshotStub{}
	mux.HandleFunc(dashrpc.SnapshotPath, func(w http.ResponseWriter, r *http.Request) {
		stub.tokensSeen = append(stub.tokensSeen, r.Header.Get(dashrpc.AuthHeaderName))
		if r.Header.Get(dashrpc.AuthHeaderName) != "Bearer "+wantToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dashrpc.Snapshot{
			Version: "stub", Address: "stub", AuthMode: "none",
			StartTime: time.Now().Add(-time.Minute),
		})
	})
	stub.Server = httptest.NewUnstartedServer(mux)
	stub.Server.Listener, _ = net.Listen("tcp", "127.0.0.1:0")
	stub.Server.Start()
	return stub
}

func (s *snapshotStub) gotAuth(header string) bool {
	for _, h := range s.tokensSeen {
		if h == header {
			return true
		}
	}
	return false
}

func TestRunDashboardErrorsWhenNoServerRunning(t *testing.T) {
	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard {
  token = "sekret"
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runDashboard(context.Background(), cfg, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when no server is running")
	}
	if !strings.Contains(stderr.String(), "no server running") {
		t.Fatalf("stderr should say 'no server running', got: %s", stderr.String())
	}
}
