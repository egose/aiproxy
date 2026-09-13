package app

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/payloadlog"
)

func readPayloadLines(t *testing.T, dir string) []payloadlog.Entry {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "payload-*.jsonl"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no payload files in %s", dir)
	}
	var out []payloadlog.Entry
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			var e payloadlog.Entry
			if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
				fh.Close()
				t.Fatalf("unmarshal: %v", err)
			}
			out = append(out, e)
		}
		fh.Close()
		if err := sc.Err(); err != nil {
			t.Fatalf("scan: %v", err)
		}
	}
	return out
}

func TestBuildWritesPayloadLog(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","choices":[]}`))
	}))
	defer upstream.Close()
	dir := t.TempDir()
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
logging {
  access_log = false
  payload_log {
    enabled = true
    dir = "`+dir+`"
  }
}
provider "openai" "openai" {
  base_url = "`+upstream.URL+`"
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: io.Discard})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() { _ = a.Close() }()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[]}`))
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	entries := readPayloadLines(t, dir)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Provider != "openai" || entries[0].Status != http.StatusOK {
		t.Fatalf("entry = %+v", entries[0])
	}
}

func TestReloadTogglesPayloadLog(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()
	dir := t.TempDir()
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  base_url = "`+upstream.URL+`"
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: io.Discard})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() { _ = a.Close() }()
	if a.payloadLog != nil {
		t.Fatalf("payload log should be nil when not configured")
	}
	if err := os.WriteFile(configPath, []byte(`
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
logging {
  payload_log {
    enabled = true
    dir = "`+dir+`"
  }
}
provider "openai" "openai" {
  base_url = "`+upstream.URL+`"
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a.payloadLog == nil {
		t.Fatalf("payload log should be enabled after reload")
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[]}`))
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if entries := readPayloadLines(t, dir); len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
}
