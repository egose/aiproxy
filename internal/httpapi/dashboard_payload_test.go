package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/providerhealth"
)

func newPayloadDeps(t *testing.T, dir string, enabled bool) Dependencies {
	t.Helper()
	rt := newRT()
	rt.Listener = config.Listener{Address: ":8080"}
	usage := accounting.NewAggregator()
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(rt.Catalog)
	logs := observability.NewLogBuffer(10)
	dashboard := dashrpc.NewRuntimeSource(config.Dashboard{Token: dashboardTestToken, Enabled: true}, "test", rt.Listener.Address, string(rt.Auth.Mode), time.Now(), rt.Catalog, usage, health, logs)
	if enabled {
		dashboard.SetPayloadSource(dir, true)
	}
	return Dependencies{
		Resolver:  modelresolver.New(rt),
		Auth:      auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:   rt.Catalog,
		Metrics:   observability.NewMetrics(),
		Health:    health,
		Usage:     usage,
		Dashboard: dashboard,
	}
}

func seedDashboardPayload(t *testing.T, dir string) {
	t.Helper()
	entries := []payloadlog.Entry{
		{RequestID: "req-ok", Method: "POST", Path: "/v1/chat/completions", PublicModel: "openai/gpt-4o", Status: 200, DurationMs: 120},
		{RequestID: "req-bad", Method: "POST", Path: "/v1/chat/completions", PublicModel: "openai/gpt-4o", Status: 500, DurationMs: 340, Error: "boom"},
	}
	var data []byte
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "payload-20260913.jsonl"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDashboardPayloadsEndpoint(t *testing.T) {
	dir := t.TempDir()
	seedDashboardPayload(t, dir)
	h := NewHandler(newPayloadDeps(t, dir, true))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, dashrpc.PayloadsPath+"?limit=10", nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var list dashrpc.PayloadList
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !list.Enabled || len(list.Payloads) != 2 {
		t.Fatalf("list = %+v, want enabled with 2 entries", list)
	}
	if list.Payloads[0].RequestID != "req-bad" {
		t.Fatalf("newest first: got %q, want req-bad", list.Payloads[0].RequestID)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, dashrpc.PayloadsPath+"?errors_only=true", nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	list = dashrpc.PayloadList{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Payloads) != 1 || list.Payloads[0].RequestID != "req-bad" {
		t.Fatalf("filtered = %+v, want [req-bad]", list.Payloads)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, dashrpc.PayloadPathPrefix+"req-ok", nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body=%s", w.Code, w.Body.String())
	}
	var entry payloadlog.Entry
	if err := json.Unmarshal(w.Body.Bytes(), &entry); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if entry.RequestID != "req-ok" || entry.Status != 200 {
		t.Fatalf("detail = %+v", entry)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, dashrpc.PayloadPathPrefix+"absent", nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("absent status = %d, want 404", w.Code)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, dashrpc.PayloadsPath, nil)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d, want 401", w.Code)
	}
}

func TestDashboardPayloadsDisabled(t *testing.T) {
	h := NewHandler(newPayloadDeps(t, t.TempDir(), false))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, dashrpc.PayloadsPath, nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var list dashrpc.PayloadList
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Enabled || len(list.Payloads) != 0 {
		t.Fatalf("list = %+v, want disabled empty", list)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, dashrpc.PayloadPathPrefix+"req-ok", nil)
	r.Header.Set(dashrpc.AuthHeaderName, "Bearer "+dashboardTestToken)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("detail status = %d, want 404 when disabled", w.Code)
	}
}
