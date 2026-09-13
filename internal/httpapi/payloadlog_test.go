package httpapi

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/provider"
)

func payloadTestLogger(t *testing.T, maxBody int) (*payloadlog.Logger, string) {
	t.Helper()
	dir := t.TempDir()
	l, err := payloadlog.New(config.PayloadLog{Enabled: true, Dir: dir, Rotation: config.PayloadLogRotationDaily, MaxBodyBytes: maxBody})
	if err != nil {
		t.Fatalf("payloadlog.New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l, dir
}

func readPayloadEntries(t *testing.T, dir string) []payloadlog.Entry {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "payload-*.jsonl"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("payload files = %v, want exactly 1", files)
	}
	fh, err := os.Open(files[0])
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer fh.Close()
	var out []payloadlog.Entry
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		var e payloadlog.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}

func TestPayloadLogUnary(t *testing.T) {
	rt := newRT()
	pl, dir := payloadTestLogger(t, 1<<20)
	h := NewHandler(Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    &stubAdapter{},
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		PayloadLog: pl,
	})
	body := `{"model":"openai/gpt-4o-mini","messages":[]}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer client-secret")
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	entries := readPayloadEntries(t, dir)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Method != http.MethodPost || e.Path != "/v1/chat/completions" {
		t.Fatalf("entry = %+v", e)
	}
	if e.PublicModel != "openai/gpt-4o-mini" || e.Provider != "openai" || e.Status != http.StatusOK {
		t.Fatalf("entry = %+v", e)
	}
	if e.Request.Body.Data != body {
		t.Fatalf("request body = %q", e.Request.Body.Data)
	}
	if got := e.Request.Headers["Authorization"]; len(got) != 1 || got[0] != "[REDACTED]" {
		t.Fatalf("request auth header = %v", got)
	}
	if !strings.Contains(e.Response.Body.Data, "chatcmpl-stub") {
		t.Fatalf("response body = %q", e.Response.Body.Data)
	}
	if e.Streaming {
		t.Fatalf("streaming should be false")
	}
}

func TestPayloadLogStreaming(t *testing.T) {
	rt := newRT()
	pl, dir := payloadTestLogger(t, 1<<20)
	streamBody := "data: {\"choices\":[]}\n\ndata: [DONE]\n\n"
	adapter := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		StreamBody: io.NopCloser(strings.NewReader(streamBody)),
		Streaming:  true,
		Stream:     provider.NewStreamCompletion(),
	}}
	h := NewHandler(Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    adapter,
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		PayloadLog: pl,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[],"stream":true}`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != streamBody {
		t.Fatalf("downstream body = %q", w.Body.String())
	}
	entries := readPayloadEntries(t, dir)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if !e.Streaming {
		t.Fatalf("streaming should be true: %+v", e)
	}
	if e.Response.Body.Data != streamBody || e.Response.Body.Bytes != len(streamBody) {
		t.Fatalf("response body = %+v", e.Response.Body)
	}
}

func TestPayloadLogDisabledByDefault(t *testing.T) {
	rt := newRT()
	h := NewHandler(Dependencies{
		Resolver: modelresolver.New(rt),
		Adapter:  &stubAdapter{},
		Auth:     auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:  rt.Catalog,
		Metrics:  observability.NewMetrics(),
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[]}`))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestPayloadLogTruncatesBodies(t *testing.T) {
	rt := newRT()
	pl, dir := payloadTestLogger(t, 4)
	h := NewHandler(Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    &stubAdapter{},
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		PayloadLog: pl,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[]}`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	entries := readPayloadEntries(t, dir)
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	if !entries[0].Request.Body.Truncated || !entries[0].Response.Body.Truncated {
		t.Fatalf("bodies should be truncated: %+v / %+v", entries[0].Request.Body, entries[0].Response.Body)
	}
}
