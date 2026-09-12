package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func healthcheckTestConfig(baseURL string, extra string) string {
	return fmt.Sprintf(`
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai-compatible" "local" {
  base_url = %q
  api_key = "sk-test"
  model "m" {}
  healthcheck {
    path = "/health"
    interval = "1s"
    timeout = "500ms"
    failure_threshold = 1
    success_threshold = 1
%s
  }
}
`, baseURL, extra)
}

func waitForHealthcheck(t *testing.T, a *App, wantHealthy bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := a.healthchecks.StatusFor("local"); ok && st.Checked && st.Healthy == wantHealthy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for healthcheck healthy=%v", wantHealthy)
}

func TestBuildStartsHealthcheckProbes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	path := writeConfigFile(t, healthcheckTestConfig(srv.URL, ""))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() {
		if err := a.Close(); err != nil {
			t.Fatalf("close app: %v", err)
		}
	}()
	if a.healthchecks == nil {
		t.Fatalf("expected healthcheck manager")
	}
	waitForHealthcheck(t, a, false)

	w := httptest.NewRecorder()
	a.Server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503 after failed healthcheck", w.Code)
	}
}

func TestReloadReconcilesHealthcheckProviders(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusInternalServerError)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(status.Load()))
	}))
	defer srv.Close()

	path := writeConfigFile(t, healthcheckTestConfig(srv.URL, ""))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() {
		if err := a.Close(); err != nil {
			t.Fatalf("close app: %v", err)
		}
	}()
	waitForHealthcheck(t, a, false)

	status.Store(http.StatusOK)
	if err := os.WriteFile(path, []byte(healthcheckTestConfig(srv.URL, "    expected_body = \"*\"\n")), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	waitForHealthcheck(t, a, true)

	w := httptest.NewRecorder()
	a.Server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("readyz status = %d, want 200 after recovered healthcheck", w.Code)
	}
}

func TestReloadRemovesHealthcheckProbes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	path := writeConfigFile(t, healthcheckTestConfig(srv.URL, ""))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() {
		if err := a.Close(); err != nil {
			t.Fatalf("close app: %v", err)
		}
	}()
	waitForHealthcheck(t, a, true)

	plain := fmt.Sprintf(`
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai-compatible" "local" {
  base_url = %q
  api_key = "sk-test"
  model "m" {}
}
`, srv.URL)
	if err := os.WriteFile(path, []byte(plain), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := a.healthchecks.StatusFor("local"); !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected healthcheck probe to stop after reload without healthcheck block")
}
