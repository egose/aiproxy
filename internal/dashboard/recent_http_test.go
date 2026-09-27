package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/app"
)

func TestRecentInferenceHTTPAccountingRPCAndTUI(t *testing.T) {
	var status atomic.Int32
	status.Store(200)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model  string
			Stream bool
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "actual-upstream" {
			t.Errorf("model rewrite: %q", body.Model)
		}
		if status.Load() != 200 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(int(status.Load()))
			io.WriteString(w, `{"error":{"message":"sensitive-response-fixture"}}`)
			return
		}
		usage := `"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10,"prompt_tokens_details":{"cached_tokens":2}}`
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"sensitive-response-fixture\"}}],%s}\n\ndata: [DONE]\n\n", usage)
		} else {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"choices":[{"message":{"content":"sensitive-response-fixture"}}],%s}`, usage)
		}
	}))
	defer up.Close()
	path := filepath.Join(t.TempDir(), "config.hcl")
	text := fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" {
  mode = "bearer_static"
  client "client-a" {
    token = "inbound-secret-fixture-a"
    tenant = "tenant-a"
  }
  client "client-b" {
    token = "inbound-secret-fixture-b"
    tenant = "tenant-b"
  }
}
dashboard { token = "dashboard-secret-fixture" }
provider "openai-compatible" "origin" {
  base_url = %q
  api_key = "upstream-secret-fixture"
  model "public" { upstream_name = "actual-upstream" }
}
alias "chat" {
  algorithm = "round_robin"
  target {
    provider = "origin"
    model = "public"
  }
}
`, up.URL)
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := app.Build(t.Context(), app.BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	n := 0
	for _, public := range []string{"origin/public", "alias/chat"} {
		for _, stream := range []bool{false, true} {
			for _, code := range []int{200, 400} {
				for _, client := range []string{"a", "b"} {
					t.Run(fmt.Sprintf("%s/stream=%v/%d/%s", public, stream, code, client), func(t *testing.T) {
						status.Store(int32(code))
						id := fmt.Sprintf("request-%d", n)
						n++
						r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q,"stream":%v,"messages":[{"role":"user","content":"sensitive-prompt-fixture"}]}`, public, stream)))
						r.Header.Set("Authorization", "Bearer inbound-secret-fixture-"+client)
						r.Header.Set("X-Request-Id", id)
						w := httptest.NewRecorder()
						a.Server.Handler.ServeHTTP(w, r)
						if w.Code != code || w.Header().Get("X-Request-Id") != id {
							t.Fatalf("response %d: %s", w.Code, w.Body)
						}
						m, wire, raw := diagnosticHTTP(t, a.Server.Handler)
						if wire.PayloadEnabled {
							t.Fatal("payload logging unexpectedly enabled")
						}
						if len(wire.Recent) != n {
							t.Fatalf("not one completion per inference: %d want %d", len(wire.Recent), n)
						}
						e := wire.Recent[len(wire.Recent)-1]
						if e.RequestID != id || e.PublicModel != public || e.Model != public || e.Provider != "origin" || e.UpstreamModel != "public" || e.Operation != "chat_completions" || e.StatusCode != code || e.Client != "client-"+client || e.Tenant != "tenant-"+client || e.Duration <= 0 || e.Timestamp.IsZero() {
							t.Fatalf("completion metadata: %+v", e)
						}
						if code == 200 && (e.PromptTokens != 7 || e.CompletionTokens != 3 || e.TotalTokens != 10 || e.CachedTokens != 2) {
							t.Fatalf("tokens lost: %+v", e)
						}
						layoutApplyKey(m, "5")
						layoutApplyKey(m, "enter")
						if m.requestDetail == nil || *m.requestDetail != e {
							t.Fatal("TUI detail lost transported event")
						}
						_, lines := m.metadataLines()
						for _, want := range []string{id, public, "client-" + client, "tenant-" + client, "resolved provider: origin", "resolved model: public"} {
							if !strings.Contains(strings.Join(lines, "\n"), want) {
								t.Fatalf("detail missing %s: %v", want, lines)
							}
						}
						for _, secret := range []string{"sensitive-prompt-fixture", "sensitive-response-fixture", "inbound-secret-fixture", "upstream-secret-fixture", "dashboard-secret-fixture"} {
							if strings.Contains(raw, secret) || strings.Contains(m.View().Content, secret) {
								t.Fatalf("sensitive body/credential leaked: %s", secret)
							}
						}
						layoutApplyKey(m, "l")
						logs := m.filteredLogs(500)
						if len(logs) == 0 {
							t.Fatal("real request logs did not correlate")
						}
						for _, log := range logs {
							if log.RequestID != id {
								t.Fatal("wrong correlated ID")
							}
						}
					})
				}
			}
		}
	}
	for i := 0; i < 205; i++ {
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"missing/public","messages":[]}`))
		r.Header.Set("Authorization", "Bearer inbound-secret-fixture-a")
		w := httptest.NewRecorder()
		a.Server.Handler.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("rejected request status %d", w.Code)
		}
	}
	m, wire, _ := diagnosticHTTP(t, a.Server.Handler)
	if len(wire.Recent) != recentLimit || len(m.recentRequests()) != recentLimit {
		t.Fatalf("recent cap: %d", len(wire.Recent))
	}
	for _, e := range wire.Recent {
		if e.RequestID == "" || e.PublicModel != "missing/public" || e.Provider != "" || e.UpstreamModel != "" || e.StatusCode != 404 {
			t.Fatalf("rejection fabricated target or lost model/ID: %+v", e)
		}
	}
}
