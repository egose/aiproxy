package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

func TestRunModelsUpstreamOpenAIStyle(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %s, want /v1/models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4.1"}]}`))
	}))
	defer srv.Close()

	cfg := writeModelsConfig(t, `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
provider "openai-compatible" "local" {
  base_url = "`+srv.URL+`"
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runModels(ctx, cfg, true, "local", true, &stdout, &stderr); err != nil {
		t.Fatalf("runModels upstream: %v (stderr=%s)", err, stderr.String())
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth header = %q, want Bearer sk-test", gotAuth)
	}
	out := stdout.String()
	for _, want := range []string{"gpt-4o-mini", "gpt-4.1", "configured as local/gpt-4o-mini", "not in config"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunModelsUpstreamAnthropic(t *testing.T) {
	var gotKey, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet","display_name":"Claude Sonnet"}],"has_more":false}`))
	}))
	defer srv.Close()

	cfg := writeModelsConfig(t, `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
provider "anthropic" "anthropic" {
  base_url = "`+srv.URL+`"
  api_key = "sk-ant"
  model "claude-sonnet" {
    upstream_name = "claude-sonnet"
  }
}
`)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runModels(ctx, cfg, true, "anthropic", true, &stdout, &stderr); err != nil {
		t.Fatalf("runModels upstream: %v (stderr=%s)", err, stderr.String())
	}
	if gotKey != "sk-ant" || gotVersion == "" {
		t.Fatalf("anthropic headers missing: key=%q version=%q", gotKey, gotVersion)
	}
	if !strings.Contains(stdout.String(), "Claude Sonnet") {
		t.Fatalf("output missing display name:\n%s", stdout.String())
	}
}

func TestRunModelsUpstreamGemini(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-goog-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-2.0-flash","displayName":"Gemini Flash"}]}`))
	}))
	defer srv.Close()

	cfg := writeModelsConfig(t, `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
provider "gemini" "gemini" {
  base_url = "`+srv.URL+`"
  api_key = "gem-key"
  model "flash" {
    upstream_name = "gemini-2.0-flash"
  }
}
`)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runModels(ctx, cfg, true, "gemini", true, &stdout, &stderr); err != nil {
		t.Fatalf("runModels upstream: %v (stderr=%s)", err, stderr.String())
	}
	if gotKey != "gem-key" {
		t.Fatalf("gemini key header = %q", gotKey)
	}
	out := stdout.String()
	for _, want := range []string{"gemini-2.0-flash", "Gemini Flash", "configured as gemini/flash"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestListUpstreamModelsJoinsV1BaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	provider := config.Provider{
		Type:    config.ProviderTypeOpenAICompatible,
		Name:    "local",
		BaseURL: srv.URL + "/v1",
		APIKey:  "sk-test",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := listUpstreamModels(ctx, provider); err != nil {
		t.Fatalf("listUpstreamModels: %v", err)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("path = %s, want /v1/models", gotPath)
	}
}

func TestRunModelsUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := writeModelsConfig(t, `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  base_url = "`+srv.URL+`"
  api_key = "sk-bad"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runModels(ctx, cfg, true, "openai", true, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for 401 upstream")
	}
	if !strings.Contains(stderr.String(), "401") {
		t.Fatalf("stderr should mention 401, got: %s", stderr.String())
	}
}
