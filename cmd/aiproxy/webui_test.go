package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunWebUIErrorsWhenBlockAbsent(t *testing.T) {
	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runWebUI(context.Background(), cfg, true, false, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when web_ui block is absent")
	}
	if !strings.Contains(stderr.String(), "not configured") {
		t.Fatalf("stderr should mention 'not configured', got: %s", stderr.String())
	}
}

func TestRunWebUIErrorsWhenConfigInvalid(t *testing.T) {
	cfg := writeDashboardConfig(t, `invalid hcl >>>`)
	var stdout, stderr bytes.Buffer
	err := runWebUI(context.Background(), cfg, true, false, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	if !strings.Contains(err.Error(), "config") {
		t.Fatalf("err should mention config, got: %v", err)
	}
}

func TestRunWebUIErrorsWhenNoServerRunning(t *testing.T) {
	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
web_ui {
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runWebUI(context.Background(), cfg, true, false, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when no server is running")
	}
	if !strings.Contains(stderr.String(), "no server running") {
		t.Fatalf("stderr should say 'no server running', got: %s", stderr.String())
	}
}

func TestRunWebUIPointsAtServerDashboard(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>stub-ui</html>"))
	}))
	defer stub.Close()
	stubAddr := strings.TrimPrefix(stub.URL, "http://")
	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = "`+stubAddr+`" }
auth "main" { mode = "none" }
web_ui {
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	if err := runWebUI(context.Background(), cfg, true, false, &stdout, &stderr); err != nil {
		t.Fatalf("runWebUI: %v (stderr=%s)", err, stderr.String())
	}
	want := "http://" + stubAddr + "/"
	if strings.TrimSpace(stdout.String()) != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunWebUIErrorsWhenServerLacksUI(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not built"}`))
	}))
	defer stub.Close()
	stubAddr := strings.TrimPrefix(stub.URL, "http://")
	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = "`+stubAddr+`" }
auth "main" { mode = "none" }
web_ui {
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runWebUI(context.Background(), cfg, true, false, &stdout, &stderr)
	if !errors.Is(err, errWebUINotAvailable) {
		t.Fatalf("expected errWebUINotAvailable, got %v (stderr=%s)", err, stderr.String())
	}
}

func TestRunWebUIErrorsOnNonHTMLResponse(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer stub.Close()
	stubAddr := strings.TrimPrefix(stub.URL, "http://")
	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = "`+stubAddr+`" }
auth "main" { mode = "none" }
web_ui {
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runWebUI(context.Background(), cfg, true, false, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unexpected content type") {
		t.Fatalf("expected content-type error, got %v (stderr=%s)", err, stderr.String())
	}
}
