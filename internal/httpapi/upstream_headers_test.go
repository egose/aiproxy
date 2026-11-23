package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
)

func assertFixedHeaders(t *testing.T, headers http.Header, present map[string]string, absent ...string) {
	t.Helper()
	for name, want := range present {
		if got := headers.Values(name); !slices.Equal(got, []string{want}) {
			t.Errorf("header %s = %q, want a single %q", name, got, want)
		}
	}
	for _, name := range absent {
		if got := headers.Values(name); len(got) != 0 {
			t.Errorf("unexpected header %s: %q", name, got)
		}
	}
}

func TestScopedUpstreamHeadersDirectStreamAliasFailoverAndMultipartPassthrough(t *testing.T) {
	const firstPrivate = "fixture-private-first-provider-value"
	const stream = "data: {\"choices\":[]}\n\ndata: [DONE]\n\n"
	type attempt struct {
		provider, model string
		headers         http.Header
	}
	seen := make(chan attempt, 8)
	upstream := func(name string, failUnaryChat bool) *httptest.Server {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var model string
			var streamRequest bool
			switch r.URL.Path {
			case passthroughChatPath:
				var input struct {
					Model  string `json:"model"`
					Stream bool   `json:"stream"`
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Errorf("read chat: %v", err)
				}
				model, streamRequest = input.Model, input.Stream
			case passthroughAudio:
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Errorf("read multipart: %v", err)
				} else {
					defer r.MultipartForm.RemoveAll()
					model = r.FormValue("model")
					if r.FormValue("language") != "en" {
						t.Error("multipart field was changed")
					}
				}
			default:
				t.Errorf("unexpected upstream path %q", r.URL.Path)
			}
			select {
			case seen <- attempt{name, model, r.Header.Clone()}:
			default:
				t.Error("too many upstream requests")
			}
			if streamRequest {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, stream)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == passthroughChatPath && failUnaryChat {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			_, _ = io.WriteString(w, `{}`)
		}))
		t.Cleanup(s.Close)
		return s
	}
	first := upstream("first", true)
	backup := upstream("backup", false)
	rt := loadPassthroughRuntime(t, passthroughAnon, `
forward_headers = ["x-app-name", "x-only-caller"]
upstream_headers = { "X-App-Name" = "root", "X-Root-Only" = "root-only" }
`, passthroughProvider("openai-compatible", "first", first.URL, fmt.Sprintf(`
  upstream_headers = { "x-app-name" = "first-provider", "X-First-Only" = %q }
  model "m" {
    upstream_name = "first-model"
    upstream_headers = { "X-APP-NAME" = "first-model-header", "X-Target-Model" = "first-model-only" }
  }
  model_passthrough "transcribe" {
    match = "^org/.+$"
    capabilities = ["audio_transcriptions"]
  }
`, firstPrivate)), passthroughProvider("openai-compatible", "backup", backup.URL, `
  upstream_headers = { "X-Backup-Only" = "backup-provider-only" }
  model "m" {
    upstream_name = "backup-model"
    upstream_headers = { "X-Target-Model" = "backup-model-only" }
  }
`), `
alias "public" {
  algorithm = "round_robin"
  providers = ["first", "backup"]
  model = "m"
  upstream_headers = { "x-App-Name" = "alias-value", "X-Alias-Only" = "alias-only" }
}
`)
	deps := passthroughDeps(rt, first)
	pl, dir := payloadTestLogger(t, 4096)
	deps.PayloadLog = pl
	h := NewHandler(deps)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n := len(r.Header.Values("X-App-Name")); n != 3 {
			t.Errorf("fixture delivered %d caller values, want 3", n)
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	post := func(path, contentType, body, wantResponse string, wantAttempts int) {
		t.Helper()
		r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, proxy.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", contentType)
		r.Header["X-App-Name"] = []string{"caller-first", "caller-repeated"}
		r.Header["x-APP-name"] = []string{"caller-different-case"}
		r.Header["X-Only-Caller"] = []string{"kept", "also-kept"}
		resp, err := proxy.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil || resp.StatusCode != http.StatusOK || string(raw) != wantResponse || len(seen) != wantAttempts {
			t.Fatalf("%s response %d %q, attempts=%d, err=%v", path, resp.StatusCode, raw, len(seen), err)
		}
	}
	check := func(provider, model string, present map[string]string, absent ...string) {
		t.Helper()
		a := <-seen
		if a.provider != provider || a.model != model {
			t.Errorf("attempt targeted %s/%s, want %s/%s", a.provider, a.model, provider, model)
		}
		assertFixedHeaders(t, a.headers, present, absent...)
		if got := a.headers.Values("X-Only-Caller"); !slices.Equal(got, []string{"kept", "also-kept"}) {
			t.Errorf("allowlisted repeat headers changed: %q", got)
		}
	}
	post(passthroughChatPath, "application/json", `{"model":"first/m","messages":[],"stream":true}`, stream, 1)
	check("first", "first-model", map[string]string{
		"X-App-Name": "first-model-header", "X-Root-Only": "root-only", "X-First-Only": firstPrivate, "X-Target-Model": "first-model-only",
	}, "X-Alias-Only", "X-Backup-Only")
	post(passthroughChatPath, "application/json", `{"model":"alias/public","messages":[]}`, `{}`, 2)
	check("first", "first-model", map[string]string{
		"X-App-Name": "alias-value", "X-Root-Only": "root-only", "X-First-Only": firstPrivate, "X-Target-Model": "first-model-only", "X-Alias-Only": "alias-only",
	}, "X-Backup-Only")
	check("backup", "backup-model", map[string]string{
		"X-App-Name": "alias-value", "X-Root-Only": "root-only", "X-Backup-Only": "backup-provider-only", "X-Target-Model": "backup-model-only", "X-Alias-Only": "alias-only",
	}, "X-First-Only")
	contentType, form := passthroughForm(t, func(w *multipart.Writer) {
		if err := w.WriteField("model", "first/org/new"); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteField("language", "en"); err != nil {
			t.Fatal(err)
		}
	})
	post(passthroughAudio, contentType, form, `{}`, 1)
	check("first", "org/new", map[string]string{
		"X-App-Name": "first-provider", "X-Root-Only": "root-only", "X-First-Only": firstPrivate,
	}, "X-Target-Model", "X-Backup-Only", "X-Alias-Only")
	entries := readPayloadEntries(t, dir)
	if len(entries) != 3 || !entries[0].Streaming {
		t.Fatalf("unexpected payload capture count or stream flag: %d", len(entries))
	}
	for _, entry := range entries {
		headers := http.Header(entry.UpstreamRequest.Headers)
		assertFixedHeaders(t, headers, map[string]string{"X-App-Name": "[REDACTED]", "X-Root-Only": "[REDACTED]"})
		if got := headers.Values("X-Only-Caller"); !slices.Equal(got, []string{"kept", "also-kept"}) {
			t.Errorf("caller-only values changed in capture: %q", got)
		}
	}
	assertFixedHeaders(t, http.Header(entries[0].UpstreamRequest.Headers), map[string]string{"X-First-Only": "[REDACTED]"})
	assertFixedHeaders(t, http.Header(entries[1].UpstreamRequest.Headers), map[string]string{"X-Backup-Only": "[REDACTED]", "X-Alias-Only": "[REDACTED]"}, "X-First-Only")
	if raw, err := json.Marshal(entries); err != nil || strings.Contains(string(raw), firstPrivate) {
		t.Fatal("fixed test value escaped into a payload log")
	}
}

func TestScopedUpstreamHeadersStayOnSameTargetReasoningRecovery(t *testing.T) {
	seen := make(chan bool, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertFixedHeaders(t, r.Header, map[string]string{"X-Recovery": "recovery-value"})
		body, err := io.ReadAll(r.Body)
		if err != nil || !strings.Contains(string(body), "visible") {
			t.Errorf("read reasoning test body: %v, %q", err, body)
		}
		marker := strings.Contains(string(body), `"encrypted_content"`)
		select {
		case seen <- marker:
		default:
			t.Error("too many recovery attempts")
		}
		w.Header().Set("Content-Type", "application/json")
		if marker {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, scopedReasoningMismatch)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)
	rt := loadPassthroughRuntime(t, passthroughAnon, passthroughProvider("openai-compatible", "reasoning", upstream.URL, `
  upstream_headers = { "x-recovery" = "recovery-value" }
  encrypted_reasoning { on_caller_mismatch = "strip_and_retry" }
  model "m" {}
`), `
alias "retry" {
  algorithm = "round_robin"
  providers = ["reasoning"]
  model = "m"
}
`)
	h := NewHandler(passthroughDeps(rt, upstream))
	for _, target := range []string{"reasoning/m", "alias/retry"} {
		body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","encrypted_content":"opaque"},{"type":"text","text":"visible"}]}]}`, target)
		w := passthroughJSON(h, passthroughChatPath, "", body)
		if w.Code != http.StatusOK || len(seen) != 2 || !(<-seen) || <-seen {
			t.Fatalf("%s: expected one recovery attempt, status=%d body=%s", target, w.Code, w.Body)
		}
	}
}

func TestScopedUpstreamHeadersManagedBoundariesWithoutDatabase(t *testing.T) {
	seen := make(chan http.Header, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case seen <- r.Header.Clone():
		default:
			t.Error("too many upstream requests")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)
	rt := loadPassthroughRuntime(t, `
upstream_headers = { X-Root = "root-value" }
forward_headers = ["X-Fixture"]
auth "fixture" {
  mode = "bearer_static"
  client "system" { token = "system-fixture" }
}
`, passthroughProvider("openai-compatible", "base", upstream.URL, `
  upstream_headers = { X-Provider = "static-provider-value" }
  model "m" { upstream_headers = { X-Model = "static-model-value" } }
`), `
alias "a" {
  algorithm = "round_robin"
  providers = ["base"]
  model = "m"
  upstream_headers = { X-Alias = "static-alias-value" }
}
`)
	system := identity.InWorkspace(store.SystemWorkspaceID)
	base, _ := rt.Catalog.Provider("base")
	base.Workspace = system
	managed, err := dbmerge.MaterializeProvider(store.DBProvider{
		Type: string(base.Type), Name: "derived", Extends: base.Name, WorkspaceID: store.SystemWorkspaceID, Enabled: true,
	}, nil, map[string]config.Provider{base.Name: base})
	if err != nil {
		t.Fatal(err)
	}
	managed.APIKey = "fixture-managed"
	a, _ := rt.Catalog.Alias("a")
	a.Workspace = system
	staticToManaged := a
	staticToManaged.Name = "static-to-managed"
	staticToManaged.Targets = []config.AliasTarget{{Provider: managed.Name, Model: "m"}}
	managedToStatic := a
	managedToStatic.Name, managedToStatic.DatabaseManaged = "managed-to-static", true
	rt.Catalog = config.NewCatalog([]config.Provider{base, managed}, nil, []config.Alias{a, staticToManaged, managedToStatic})
	deps := passthroughDeps(rt, upstream)
	deps.MultiTenancy.Enabled = true
	deps.CatalogConfirmedAt = time.Now()
	deps.ServingRevisions.Catalog = 1
	h := NewHandler(deps)
	for _, tc := range []struct {
		model, key         string
		static, aliasValue bool
	}{
		{"derived/m", "fixture-managed", false, false},
		{"alias/static-to-managed", "fixture-managed", false, false},
		{"alias/managed-to-static", "fixture-upstream", true, false},
		{"alias/a", "fixture-upstream", true, true},
	} {
		w := passthroughChat(h, "system-fixture", tc.model)
		if w.Code != http.StatusOK || len(seen) != 1 {
			t.Fatalf("%s did not make one in-workspace inference request: %d %s", tc.model, w.Code, w.Body)
		}
		headers := <-seen
		assertFixedHeaders(t, headers, map[string]string{"Authorization": "Bearer " + tc.key, "X-Fixture": "preserve"})
		if tc.static {
			assertFixedHeaders(t, headers, map[string]string{"X-Root": "root-value", "X-Provider": "static-provider-value", "X-Model": "static-model-value"})
		} else {
			assertFixedHeaders(t, headers, nil, "X-Root", "X-Provider", "X-Model")
		}
		if tc.aliasValue {
			assertFixedHeaders(t, headers, map[string]string{"X-Alias": "static-alias-value"})
		} else {
			assertFixedHeaders(t, headers, nil, "X-Alias")
		}
	}
}
