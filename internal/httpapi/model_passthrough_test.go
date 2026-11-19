package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

const (
	passthroughChatPath = "/v1/chat/completions"
	passthroughAudio    = "/v1/audio/transcriptions"
	passthroughLiteral  = `
  model "org/pin" {
    upstream_name = "pinned"
    capabilities = ["chat", "audio_transcriptions"]
  }
`
	passthroughFamily = `
  model_passthrough "model-family" {
    match = "^org/.+$"
    capabilities = ["chat", "embeddings", "audio_transcriptions"]
  }
`
	passthroughAnon = `
auth "fixture" {
  mode = "none"
}
`
	passthroughPeerAuth = `
auth "fixture" {
  mode = "bearer_static"
  client "peer" {
    token = "peer-token"
    allowed_model_passthroughs = ["router/model-family"]
  }
}
`
)

func passthroughProvider(typ, name, baseURL, settings string) string {
	return fmt.Sprintf(`
provider %q %q {
  base_url = %q
  api_key = "fixture-upstream"
  %s
}
`, typ, name, baseURL, settings)
}

func loadPassthroughRuntime(t *testing.T, blocks ...string) *config.Runtime {
	t.Helper()
	rt, err := config.Load([]byte(`
listener "http" "fixture" {
  address = ":8080"
}
`+strings.Join(blocks, "\n")), "passthrough.hcl")
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func passthroughDeps(rt *config.Runtime, upstream *httptest.Server, keys ...auth.DynamicClient) Dependencies {
	return Dependencies{
		Catalog: rt.Catalog, Resolver: modelresolver.New(rt), Client: upstream.Client(),
		Auth: auth.NewAuthenticatorWithClients(rt.Auth, keys), Authorizer: auth.NewAuthorizerWithClients(rt.Auth, keys),
		HasAccessLog: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func passthroughRequest(h http.Handler, method, path, token, contentType, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	r.Header.Set("X-Fixture", "preserve")
	r.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func passthroughJSON(h http.Handler, path, token, body string) *httptest.ResponseRecorder {
	return passthroughRequest(h, http.MethodPost, path, token, "application/json", body)
}

func passthroughChat(h http.Handler, token, model string) *httptest.ResponseRecorder {
	return passthroughJSON(h, passthroughChatPath, token, fmt.Sprintf(`{"model":%q,"messages":[]}`, model))
}

func passthroughForm(t *testing.T, writeParts func(*multipart.Writer)) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	writeParts(form)
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	return form.FormDataContentType(), buf.String()
}

func TestModelPassthroughOpenAIAdapterOperations(t *testing.T) {
	const public = "router/org/sub/series-43"
	const suffix = "org/sub/series-43"
	const sse = "event: vendor\r\ndata: {\"vendor\":true}\r\n\r\ndata: [DONE]\n\n"
	for _, typ := range []string{"openai", "openai-compatible", "openrouter", "zenmux"} {
		for _, tc := range []struct{ name, path, fields, responseType, response string }{
			{"chat", passthroughChatPath, `"messages":[]`, "application/json", `{"choices":[]}`},
			{"chat-sse", passthroughChatPath, `"messages":[],"stream":true`, "text/event-stream", sse},
			{"responses", "/v1/responses", `"input":"hey"`, "application/json", `{"output":[]}`},
			{"responses-sse", "/v1/responses", `"input":"hey","stream":true`, "text/event-stream", sse},
			{"embeddings", "/v1/embeddings", `"input":"hey"`, "application/json", `{"data":[]}`},
			{"images", "/v1/images/generations", `"prompt":"hey"`, "application/json", `{"data":[]}`},
			{"audio-speech", "/v1/audio/speech", `"input":"hey","voice":"alloy"`, "audio/wav", "\x00\x01synthetic audio"},
			{"audio-transcriptions", passthroughAudio, "", "application/json", `{"text":"hey"}`},
		} {
			t.Run(typ+"/"+tc.name, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Method != http.MethodPost || r.URL.Path != "/vendor"+tc.path || r.URL.RawQuery != "" ||
						r.Header.Get("Authorization") != "Bearer fixture-upstream" || r.Header.Get("User-Agent") != "fixture-agent" ||
						r.Header.Get("X-Fixture") != "preserve" || r.Header.Get("Accept") != "application/json, text/event-stream" {
						t.Errorf("unexpected upstream URL or request metadata: %s %s %#v", r.Method, r.URL, r.Header)
					}
					if typ == "openrouter" && (r.Header.Get("HTTP-Referer") != "https://opencode.ai/" || r.Header.Get("X-Title") != "opencode") {
						t.Errorf("OpenRouter headers are missing: %v", r.Header)
					}
					if tc.path == passthroughAudio {
						if err := r.ParseMultipartForm(1 << 20); err != nil {
							t.Errorf("upstream multipart parse: %v", err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						defer r.MultipartForm.RemoveAll()
						if !slices.Equal(r.MultipartForm.Value["model"], []string{suffix}) || r.FormValue("language") != "en" {
							t.Errorf("upstream values = %v", r.MultipartForm.Value)
						}
						file, header, err := r.FormFile("file")
						if err != nil {
							t.Errorf("missing upload: %v", err)
						} else {
							defer file.Close()
							audio, err := io.ReadAll(file)
							if err != nil || string(audio) != "fixture audio" || header.Filename != "audio.wav" {
								t.Errorf("unexpected upload: name=%q bytes=%q err=%v", header.Filename, audio, err)
							}
						}
					} else {
						var input struct {
							Model     string   `json:"model"`
							Fallbacks []string `json:"fallbacks"`
							Vendor    struct {
								Model  string   `json:"model"`
								Models []string `json:"models"`
								Preset string   `json:"preset"`
							} `json:"vendor"`
						}
						if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Model != suffix || input.Vendor.Model != "nested-field" ||
							!slices.Equal(input.Vendor.Models, []string{"nested-field"}) || input.Vendor.Preset != "nested-field" ||
							!slices.Equal(input.Fallbacks, []string{"top-level-extension"}) {
							t.Errorf("upstream model/opaque extension did not survive: %+v, %v", input, err)
						}
					}
					w.Header().Set("Content-Type", tc.responseType)
					w.Header().Set("X-Mock-Upstream", "kept")
					_, _ = io.WriteString(w, tc.response)
				}))
				t.Cleanup(upstream.Close)
				rt := loadPassthroughRuntime(t, passthroughAnon, passthroughProvider(typ, "router", upstream.URL+"/vendor/v1", `
  user_agent = "fixture-agent"
  forward_headers = ["X-Fixture"]
  model_passthrough "all-operations" {
    match = "^org/.+$"
    capabilities = ["chat", "responses", "embeddings", "images", "audio_transcriptions", "audio_speech"]
  }
`))
				contentType, body := "application/json", fmt.Sprintf(`{"model":%q,%s,"FaLlBaCkS":["top-level-extension"],"vendor":{"model":"nested-field","models":["nested-field"],"preset":"nested-field"}}`, public, tc.fields)
				if tc.path == passthroughAudio {
					contentType, body = passthroughForm(t, func(form *multipart.Writer) {
						if err := form.SetBoundary("quoted (boundary)"); err != nil {
							t.Fatal(err)
						}
						if err := form.WriteField("model", public); err != nil {
							t.Fatal(err)
						}
						if err := form.WriteField("language", "en"); err != nil {
							t.Fatal(err)
						}
						file, err := form.CreateFormFile("file", "audio.wav")
						if err != nil {
							t.Fatal(err)
						}
						if _, err := io.WriteString(file, "fixture audio"); err != nil {
							t.Fatal(err)
						}
					})
				}
				h := NewHandler(passthroughDeps(rt, upstream))
				w := passthroughRequest(h, http.MethodPost, tc.path, "", contentType, body)
				if w.Code != http.StatusOK || calls.Load() != 1 || w.Header().Get("Content-Type") != tc.responseType ||
					w.Header().Get("X-Mock-Upstream") != "kept" || w.Body.String() != tc.response {
					t.Fatalf("status=%d calls=%d headers=%v body=%q", w.Code, calls.Load(), w.Header(), w.Body.String())
				}
			})
		}
	}
}

func TestModelPassthroughLiteralPriorityAndUpstreamModelChanges(t *testing.T) {
	type attempt struct{ model, models, preset string }
	var attempts []attempt
	gotAttempt := make(chan attempt, 10)
	var available atomic.Bool
	const upstreamMiss = `{"error":{"message":"mock model not found","type":"model_not_found"}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.URL.Path != passthroughChatPath {
			t.Errorf("unexpected chat request: %s, %v", r.URL.Path, err)
		}
		var model string
		_ = json.Unmarshal(body["model"], &model)
		gotAttempt <- attempt{model, string(body["MoDeLs"]), string(body["preset"])}
		w.Header().Set("Content-Type", "application/json")
		if model == "pinned" || (model == "org/new" && !available.Load()) {
			w.Header().Set("X-Model-Source", "mock")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, upstreamMiss)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	t.Cleanup(upstream.Close)
	rt := loadPassthroughRuntime(t, passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", passthroughLiteral+passthroughFamily), `
alias "public-pin" {
  algorithm = "round_robin"
  providers = ["router"]
  model = "org/pin"
}
auth "fixture" {
  mode = "bearer_static"
  client "grant-only" {
    token = "grant-only"
    allowed_model_passthroughs = ["router/model-family"]
    allowed_models = ["router/some-other-model"]
  }
  client "all" {
    token = "all"
    allowed_model_passthroughs = ["router/model-family"]
  }
}
`)
	h := NewHandler(passthroughDeps(rt, upstream))
	if w := passthroughChat(h, "grant-only", "router/org/pin"); w.Code != http.StatusForbidden {
		t.Fatalf("grant overrode an exact denial: %d %s", w.Code, w.Body)
	}
	if w := passthroughJSON(h, "/v1/embeddings", "all", `{"model":"router/org/pin","input":"x"}`); w.Code != http.StatusBadRequest ||
		guardrailErrorType(t, w.Body.String()) != "unsupported_operation" {
		t.Fatalf("rule overrode unsupported literal operation: %d %s", w.Code, w.Body)
	}
	for _, model := range []string{"router/org/pin", "alias/public-pin", "router/org/new"} {
		body := fmt.Sprintf(`{"model":%q,"messages":[]}`, model)
		if model != "router/org/new" {
			body = fmt.Sprintf(`{"model":%q,"messages":[],"MoDeLs":["other/m"],"preset":"@preset/example"}`, model)
		}
		w := passthroughJSON(h, passthroughChatPath, "all", body)
		if w.Code != http.StatusNotFound || w.Body.String() != upstreamMiss || w.Header().Get("X-Model-Source") != "mock" {
			t.Fatalf("upstream miss for %q not returned verbatim: %d %s", model, w.Code, w.Body)
		}
	}
	available.Store(true)
	if w := passthroughChat(h, "grant-only", "router/org/new"); w.Code != http.StatusOK {
		t.Fatalf("new upstream model not available without reload: %d %s", w.Code, w.Body)
	}
	for len(gotAttempt) > 0 {
		attempts = append(attempts, <-gotAttempt)
	}
	if !slices.Equal(attempts, []attempt{
		{"pinned", `["other/m"]`, `"@preset/example"`}, {"pinned", `["other/m"]`, `"@preset/example"`},
		{model: "org/new"}, {model: "org/new"},
	}) {
		t.Fatalf("exact paths retried or selected the wrong model: %v", attempts)
	}
	w := passthroughRequest(h, http.MethodGet, "/v1/models", "all", "", "")
	var inventory struct{ Data []ModelCard }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &inventory) != nil ||
		len(inventory.Data) != 2 || inventory.Data[0].ID != "router/org/pin" || inventory.Data[1].ID != "alias/public-pin" {
		t.Fatalf("nonliteral target in discovery: %d %s", w.Code, w.Body)
	}
}

func TestModelPassthroughRejectsAlternateModelSelectors(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)
	const public = "router/org/new"
	for _, typ := range []string{"openai", "openai-compatible", "openrouter", "zenmux"} {
		t.Run(typ, func(t *testing.T) {
			rt := loadPassthroughRuntime(t, passthroughProvider(typ, "router", upstream.URL+"/v1", passthroughFamily), `
auth "fixture" {
  mode = "bearer_static"
  client "granted" {
    token = "granted"
    allowed_model_passthroughs = ["router/model-family"]
  }
  client "restricted" {
    token = "restricted"
    allowed_models = ["router/other"]
    allowed_model_passthroughs = ["router/model-family"]
  }
}
`)
			h := NewHandler(passthroughDeps(rt, upstream))
			before := calls.Load()
			assertDenied := func(t *testing.T, path, contentType, body string) {
				t.Helper()
				for _, tc := range []struct {
					token, code, message string
					status               int
				}{
					{"granted", "model_not_found", `unknown model "` + public + `"`, http.StatusNotFound},
					{"restricted", "forbidden", modelForbiddenMessage, http.StatusForbidden},
				} {
					w := passthroughRequest(h, http.MethodPost, path, tc.token, contentType, body)
					var got apiError
					if w.Code != tc.status || json.Unmarshal(w.Body.Bytes(), &got) != nil ||
						got.Error.Type != tc.code || got.Error.Message != tc.message || calls.Load() != before {
						t.Fatalf("%s: status=%d calls=%d body=%s", tc.token, w.Code, calls.Load()-before, w.Body)
					}
				}
			}
			for _, tc := range []struct{ name, body string }{
				{"models", `{"model":"router/org/new","models":["other/m"]}`},
				{"case-models-sse", `{"MODELS":null,"model":"router/org/new","stream":true}`},
				{"escaped-models", `{"model":"router/org/new","m\u006fdelS":["other/m"]}`},
				{"preset", `{"model":"router/org/new","preset":"@preset/example"}`},
				{"case-preset", `{"PREset":"","model":"router/org/new"}`},
				{"escaped-preset", `{"model":"router/org/new","pre\u0053et":"@preset/example"}`},
			} {
				t.Run("json/"+tc.name, func(t *testing.T) {
					assertDenied(t, passthroughChatPath, "application/json", tc.body)
				})
			}
			for _, field := range []string{"models", "MODELS", "preset", "PreSeT"} {
				t.Run("multipart/"+field, func(t *testing.T) {
					contentType, body := passthroughForm(t, func(form *multipart.Writer) {
						if err := form.WriteField("model", public); err != nil {
							t.Fatal(err)
						}
						if err := form.WriteField(field, "other/m"); err != nil {
							t.Fatal(err)
						}
					})
					assertDenied(t, passthroughAudio, contentType, body)
				})
			}
		})
	}
}

func TestModelPassthroughDenialsRetainLiteralResponses(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)
	rt := loadPassthroughRuntime(t,
		passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", passthroughFamily+`
  model_passthrough "overlap-embeddings" {
    match = "^org/overlap$"
    capabilities = ["embeddings"]
  }
`), passthroughProvider("openai", "disabled", upstream.URL+"/v1", "enabled = false\n"+passthroughFamily), `
auth "fixture" {
  mode = "bearer_static"
  client "granted" {
    token = "granted"
    allowed_model_passthroughs = ["router/model-family", "disabled/model-family"]
  }
  client "restricted" {
    token = "restricted"
    allowed_models = ["router/literal"]
    allowed_model_passthroughs = ["router/model-family"]
  }
  client "unrestricted" {
    token = "unrestricted"
    allowed_models = []
  }
  client "by-id" {
    token = "by-id"
    allowed_models = ["router/org/new"]
  }
  client "by-label" {
    token = "by-label"
    allowed_models = ["router/model-family"]
  }
}
`)
	var logs bytes.Buffer
	deps := passthroughDeps(rt, upstream)
	deps.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	h := NewHandler(deps)
	for _, tc := range []struct {
		name, token, path, model, message string
		status                            int
	}{
		{"anonymous", "", passthroughChatPath, "router/org/new", auth.ErrNoToken.Error(), http.StatusUnauthorized},
		{"unknown-token", "unknown", passthroughChatPath, "router/org/new", auth.ErrInvalidToken.Error(), http.StatusUnauthorized},
		{"unrestricted-without-grant", "unrestricted", passthroughChatPath, "router/org/new", `unknown model "router/org/new"`, http.StatusNotFound},
		{"allowlist-id", "by-id", passthroughChatPath, "router/org/new", `unknown model "router/org/new"`, http.StatusNotFound},
		{"allowlist-label", "by-label", passthroughChatPath, "router/org/new", modelForbiddenMessage, http.StatusForbidden},
		{"rule-is-not-a-model", "by-label", passthroughChatPath, "router/model-family", `unknown model "router/model-family"`, http.StatusNotFound},
		{"missing", "granted", passthroughChatPath, "router/other/new", `unknown model "router/other/new"`, http.StatusNotFound},
		{"overlap", "granted", passthroughChatPath, "router/org/overlap", `unknown model "router/org/overlap"`, http.StatusNotFound},
		{"overlap-restricted", "restricted", passthroughChatPath, "router/org/overlap", modelForbiddenMessage, http.StatusForbidden},
		{"unsupported", "granted", "/v1/responses", "router/org/new", `unknown model "router/org/new"`, http.StatusNotFound},
		{"unsupported-restricted", "restricted", "/v1/responses", "router/org/new", modelForbiddenMessage, http.StatusForbidden},
		{"native-messages", "granted", "/v1/messages", "router/org/new", `unknown model "router/org/new"`, http.StatusNotFound},
		{"disabled", "granted", passthroughChatPath, "disabled/org/new", `unknown provider "disabled"`, http.StatusNotFound},
		{"unknown-provider", "granted", passthroughChatPath, "unknown/org/new", `unknown provider "unknown"`, http.StatusNotFound},
		{"reserved-prefix", "granted", passthroughChatPath, "alias/org/new", `unknown alias "org/new"`, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := passthroughJSON(h, tc.path, tc.token, fmt.Sprintf(`{"model":%q}`, tc.model))
			var result apiError
			if w.Code != tc.status || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Error.Message != tc.message ||
				strings.Contains(w.Body.String(), "overlap-embeddings") || strings.Contains(w.Body.String(), "^org/") {
				t.Fatalf("status=%d body=%s, want %d %q", w.Code, w.Body, tc.status, tc.message)
			}
		})
	}
	for _, suffix := range []string{"org/NEW", "org//new", "org/../new", "org/new:free", "org/new ", "org/new\n", "org/" + strings.Repeat("a", config.MaxModelPassthroughSuffixBytes-3)} {
		w := passthroughChat(h, "granted", "router/"+suffix)
		if w.Code != http.StatusNotFound {
			t.Errorf("invalid suffix %q returned %d: %s", suffix, w.Code, w.Body)
		}
	}
	if calls.Load() != 0 || strings.Count(logs.String(), "ambiguous model passthrough match") != 2 || strings.Contains(logs.String(), "^org/") {
		t.Fatalf("denied requests reached upstream or missing bounded ambiguity diagnostic: calls=%d, logs=%s", calls.Load(), logs.String())
	}
	if w := passthroughChat(h, "granted", "router/org/"+strings.Repeat("a", config.MaxModelPassthroughSuffixBytes-4)); w.Code != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("suffix limit not inclusive: %d %s; calls=%d", w.Code, w.Body, calls.Load())
	}
}

func TestModelPassthroughWorkspaceAndKeyOnlyRefresh(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)
	rt := loadPassthroughRuntime(t,
		passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", passthroughLiteral+passthroughFamily),
		passthroughProvider("openai-compatible", "foreign", upstream.URL+"/v1", passthroughLiteral+passthroughFamily+`
  model_passthrough "foreign-overlap" {
    match = "^org/new$"
    capabilities = ["embeddings"]
  }
`), `
auth "fixture" {
  mode = "bearer_static"
  client "shared-name" {
    token = "static-token"
    allowed_model_passthroughs = ["router/model-family", "foreign/model-family"]
  }
}
`)
	system, foreign := identity.InWorkspace(store.SystemWorkspaceID), identity.InWorkspace(uuid.New())
	p, _ := rt.Catalog.Provider("router")
	q, _ := rt.Catalog.Provider("foreign")
	p.Workspace, q.Workspace = system, foreign
	rt.Catalog = config.NewCatalog([]config.Provider{p, q}, nil, nil)
	makeKey := func(token string, workspace identity.WorkspaceScope, allow ...string) auth.DynamicClient {
		return auth.DynamicClient{TokenHash: store.TokenHash(token), KeyID: uuid.New(), Name: "shared-name", Workspace: workspace, AllowedModels: allow}
	}
	key := makeKey("system-key", system, "router/org/pin", "router/org/new", "router/model-family")
	other := makeKey("foreign-key", foreign)
	deps := passthroughDeps(rt, upstream, key, other)

	h := NewHandler(deps)
	if w := passthroughChat(h, "system-key", "router/org/new"); w.Code != http.StatusNotFound || calls.Load() != 0 {
		t.Fatalf("DB key received file routing policy: %d %s", w.Code, w.Body)
	}

	deps.MultiTenancy.Enabled = true
	deps.ServingRevisions = store.ServingRevisions{Catalog: 1, Keys: 1}
	deps.KeysConfirmedAt, deps.CatalogConfirmedAt = time.Now(), time.Now()
	var logs bytes.Buffer
	deps.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	h.UpdateDependencies(deps)
	if w := passthroughChat(h, "static-token", "router/org/new"); w.Code != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("in-scope static grant not usable: %d %s", w.Code, w.Body)
	}
	missing := passthroughChat(h, "static-token", "absent/org/new")
	for _, tc := range []struct{ token, model string }{
		{"static-token", "foreign/org/new"}, {"static-token", "foreign/org/unique"}, {"static-token", "foreign/org/pin"},
		{"system-key", "router/org/new"}, {"foreign-key", "foreign/org/new"},
	} {
		w := passthroughChat(h, tc.token, tc.model)
		if w.Code != http.StatusNotFound || w.Body.String() != missing.Body.String() || calls.Load() != 1 {
			t.Fatalf("hosted denial differs from ordinary miss for %s %s: %d %s", tc.token, tc.model, w.Code, w.Body)
		}
	}
	if strings.Contains(logs.String(), "ambiguous model passthrough match") {
		t.Fatalf("a foreign provider's rules were examined: %s", logs.String())
	}
	if w := passthroughChat(h, "system-key", "router/org/pin"); w.Code != http.StatusOK || calls.Load() != 2 {
		t.Fatalf("ordinary DB key literal allowlist changed: %d %s", w.Code, w.Body)
	}

	replacement := makeKey("new-key", system, "router/org/pin", "router/org/new")
	h.ApplyServingKeys(rt.Auth, []auth.DynamicClient{replacement, other}, store.ServingRevisions{Catalog: 1, Keys: 2}, time.Now())
	for _, tc := range []struct {
		token, model string
		status       int
	}{
		{"static-token", "router/org/new", http.StatusOK},
		{"new-key", "router/org/pin", http.StatusOK},
		{"new-key", "router/org/new", http.StatusNotFound},
		{"system-key", "router/org/pin", http.StatusUnauthorized},
	} {
		w := passthroughChat(h, tc.token, tc.model)
		if w.Code != tc.status {
			t.Errorf("after key refresh %s %s = %d %s, want %d", tc.token, tc.model, w.Code, w.Body, tc.status)
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("unexpected upstream calls after key refresh: %d", calls.Load())
	}

	stale := h.SnapshotDependencies()
	stale.KeysConfirmedAt, stale.CatalogConfirmedAt = time.Time{}, time.Time{}
	h.UpdateDependencies(stale)
	if w := passthroughChat(h, "new-key", "router/org/new"); w.Code != http.StatusServiceUnavailable ||
		guardrailErrorType(t, w.Body.String()) != "serving_stale" || calls.Load() != 4 {
		t.Fatalf("fallback changed stale DB-key response: %d %s", w.Code, w.Body)
	}
	if w := passthroughChat(h, "static-token", "router/org/new"); w.Code != http.StatusOK || calls.Load() != 5 {
		t.Fatalf("DB freshness degraded current trusted HCL access: %d %s", w.Code, w.Body)
	}

	none := config.Auth{Mode: config.AuthModeNone}
	management := deps
	management.Auth, management.Authorizer = auth.NewAuthenticator(none), auth.NewAuthorizer(none)
	h.UpdateDependencies(management)
	for _, token := range []string{"", "system-key", "static-token"} {
		if w := passthroughChat(h, token, "router/org/new"); w.Code != http.StatusUnauthorized || calls.Load() != 5 {
			t.Fatalf("managed auth none admitted inference for %q: %d %s", token, w.Code, w.Body)
		}
	}
}

func TestModelPassthroughRequiresSingleCanonicalModel(t *testing.T) {
	type models struct {
		JSON json.RawMessage
		Form map[string][]string
	}
	attempts := make(chan models, 16)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == passthroughAudio {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("upstream form: %v", err)
			} else {
				defer r.MultipartForm.RemoveAll()
				attempts <- models{Form: r.MultipartForm.Value}
			}
		} else {
			b, _ := io.ReadAll(r.Body)
			attempts <- models{JSON: b}
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)
	rt := loadPassthroughRuntime(t, passthroughAnon, passthroughProvider("openai", "router", upstream.URL+"/v1", passthroughLiteral+passthroughFamily+`
  encrypted_reasoning {
    passthrough = false
  }
`))
	h := NewHandler(passthroughDeps(rt, upstream))
	for _, body := range []string{
		`{"Model":"router/org/new"}`,
		`{"MODEL":"foreign/m","model":"router/org/new"}`,
		`{"model":"foreign/m","Model":"router/org/new"}`,
		`{"model":"router/org/new","Model":null}`,
		`{"model":"router/org/new","model":false}`,
		`{"model":"foreign/m","mo\u0064el":"router/org/new"}`,
		`{"model":"foreign/m","model":"router/org/new","messages":[{"role":"assistant","reasoning_content":"pre-strip"}]}`,
	} {
		w := passthroughJSON(h, passthroughChatPath, "", body)
		if w.Code != http.StatusNotFound || len(attempts) != 0 {
			t.Errorf("ambiguous/noncanonical JSON admitted: %s; %d %s", body, w.Code, w.Body)
		}
	}
	for _, tc := range []struct{ name, first, second, disposition, transfer string }{
		{name: "whitespace", first: " \trouter/org/new\t "},
		{name: "duplicate", first: "router/org/new", second: "model"},
		{name: "case-variant", first: "router/org/new", second: "MoDeL"},
		{name: "extended-case-variant", first: "router/org/new", disposition: `form-data; name*=UTF-8''moDEl`},
		{name: "extended-overrides-alternate-name", first: "router/org/new", disposition: `form-data; name="MODEL"; name*=UTF-8''misc`},
		{name: "extended-preset", first: "router/org/new", disposition: `form-data; name*=UTF-8''PR%45set`},
		{name: "extended-overrides-models", first: "router/org/new", disposition: `form-data; name="models"; name*=UTF-8''misc`},
		{name: "duplicate-disposition", first: "router/org/new", disposition: `form-data; name="misc"`},
		{name: "model-transfer-encoding", first: "router/org/new", transfer: "base64"},
		{name: "decoded-model", first: "router/org/n=65w", transfer: "quoted-printable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contentType, body := passthroughForm(t, func(form *multipart.Writer) {
				header := textproto.MIMEHeader{"Content-Disposition": {`form-data; name="model"`}}
				if tc.transfer != "" {
					header.Set("Content-Transfer-Encoding", tc.transfer)
				}
				part, err := form.CreatePart(header)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(part, tc.first); err != nil {
					t.Fatal(err)
				}
				if tc.second != "" {
					if err := form.WriteField(tc.second, "foreign/m"); err != nil {
						t.Fatal(err)
					}
				}
				if tc.disposition != "" {
					header = textproto.MIMEHeader{"Content-Disposition": {tc.disposition}}
					if tc.name == "duplicate-disposition" {
						header.Add("Content-Disposition", `form-data; name="MODEL"`)
					}
					part, err := form.CreatePart(header)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := io.WriteString(part, "foreign/m"); err != nil {
						t.Fatal(err)
					}
				}
			})
			w := passthroughRequest(h, http.MethodPost, passthroughAudio, "", contentType, body)
			if w.Code != http.StatusNotFound || len(attempts) != 0 {
				t.Fatalf("competing or normalized multipart model admitted: %d %s", w.Code, w.Body)
			}
		})
	}

	if w := passthroughJSON(h, passthroughChatPath, "", `{"Model":"router/org/pin"}`); w.Code != http.StatusOK || len(attempts) != 1 {
		t.Fatalf("literal JSON alias of key changed: %d %s", w.Code, w.Body)
	}
	if got := (<-attempts).JSON; !bytes.Contains(got, []byte(`"Model":"router/org/pin"`)) || !bytes.Contains(got, []byte(`"model":"pinned"`)) {
		t.Fatalf("literal JSON rewrite changed: %s", got)
	}
	if w := passthroughJSON(h, passthroughChatPath, "", `{"mo\u0064el":"router/org/new"}`); w.Code != http.StatusOK || len(attempts) != 1 {
		t.Fatalf("escaped canonical model key rejected: %d %s", w.Code, w.Body)
	}
	if got := string((<-attempts).JSON); got != `{"model":"org/new"}` {
		t.Fatalf("escaped key changed canonical rewrite: %s", got)
	}
	if w := passthroughJSON(h, passthroughChatPath, "", `{"model":"router/org/new","messages":[{"role":"assistant","reasoning_content":"pre-strip"}]}`); w.Code != http.StatusOK || len(attempts) != 1 {
		t.Fatalf("provider reasoning policy prevented fallback: %d %s", w.Code, w.Body)
	}
	if got := (<-attempts).JSON; bytes.Contains(got, []byte(`"reasoning_content"`)) || extractModel(got) != "org/new" {
		t.Fatalf("provider reasoning policy or suffix not honored: %s", got)
	}
	contentType, body := passthroughForm(t, func(form *multipart.Writer) {
		for _, key := range []string{"model", "model", "MODEL"} {
			if err := form.WriteField(key, "  router/org/pin  "); err != nil {
				t.Fatal(err)
			}
		}
		if err := form.WriteField("models", `["other/m"]`); err != nil {
			t.Fatal(err)
		}
		if err := form.WriteField("PrEsEt", "@preset/example"); err != nil {
			t.Fatal(err)
		}
	})
	if w := passthroughRequest(h, http.MethodPost, passthroughAudio, "", contentType, body); w.Code != http.StatusOK || len(attempts) != 1 {
		t.Fatalf("literal multipart model behavior changed: %d %s", w.Code, w.Body)
	}
	if got := (<-attempts).Form; !slices.Equal(got["model"], []string{"pinned", "pinned"}) || !slices.Equal(got["MODEL"], []string{"  router/org/pin  "}) ||
		!slices.Equal(got["models"], []string{`["other/m"]`}) || !slices.Equal(got["PrEsEt"], []string{"@preset/example"}) {
		t.Fatalf("literal multipart rewrite changed: %v", got)
	}
}

func TestModelPassthroughMalformedMultipartDoesNotDispatch(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)
	rt := loadPassthroughRuntime(t, passthroughAnon, passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", passthroughFamily))
	h := NewHandler(passthroughDeps(rt, upstream))
	contentType, body := passthroughForm(t, func(form *multipart.Writer) {
		if err := form.WriteField("model", "router/org/new"); err != nil {
			t.Fatal(err)
		}
	})
	boundary := multipartContentBoundary(contentType)
	tooLong := strings.Repeat("a", 71)
	for _, tc := range []struct {
		name, contentType, body string
		status                  int
	}{
		{"wrong-case-type", strings.Replace(contentType, "multipart", "Multipart", 1), body, http.StatusBadRequest},
		{"wrong-case-boundary", strings.Replace(contentType, "boundary=", "BOUNDARY=", 1), body, http.StatusBadRequest},
		{"wrong-boundary", contentType + "wrong", body, http.StatusBadRequest},
		{"missing-boundary", "multipart/form-data;", body, http.StatusBadRequest},
		{"unclosed-quote", contentType + `"`, body, http.StatusNotFound},
		{"conflicting-boundaries", contentType + "; BOUNDARY=other", body, http.StatusNotFound},
		{"disagreeing-extended-boundary", contentType + "; boundary*=UTF-8''different", body, http.StatusNotFound},
		{"oversize-boundary", strings.ReplaceAll(contentType, boundary, tooLong), strings.ReplaceAll(body, boundary, tooLong), http.StatusNotFound},
		{"incomplete-last-part", contentType, strings.TrimSuffix(body, "--\r\n") + "\r\n", http.StatusNotFound},
		{"unclosed-last-part", contentType, strings.TrimSuffix(body, "--\r\n") + "\r\nContent-Disposition: form-data; name=\"other\"\r\n\r\nvalue", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := passthroughRequest(h, http.MethodPost, passthroughAudio, "", tc.contentType, tc.body)
			if w.Code != tc.status || calls.Load() != 0 {
				t.Fatalf("malformed multipart reached upstream or returned the wrong error: %d %s", w.Code, w.Body)
			}
		})
	}
	if w := passthroughRequest(h, http.MethodPost, passthroughAudio, "", contentType, strings.TrimSuffix(body, "\r\n")); w.Code != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("valid closing boundary at EOF did not dispatch: %d %s", w.Code, w.Body)
	}
}

func TestModelPassthroughGuardrailsAndOrdinaryReasoningRecovery(t *testing.T) {
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := attempts.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != passthroughChatPath || r.Header.Get("Authorization") != "Bearer fixture-upstream" ||
			extractModel(body) != "org/reasoning" || !bytes.Contains(body, []byte(`"hi"`)) {
			t.Errorf("changed upstream or model during reasoning retry: %s %q %s", r.URL.Path, r.Header.Get("Authorization"), body)
		}
		opaque := bytes.Contains(body, []byte(`"encrypted_content"`))
		if opaque != (call == 1) {
			t.Errorf("unexpected reasoning payload on call %d: %s", call, body)
		}
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"encrypted_content was not issued to this caller"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	t.Cleanup(upstream.Close)
	rt := loadPassthroughRuntime(t, passthroughAnon, passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", passthroughFamily), `
encrypted_reasoning {
  on_caller_mismatch = "strip_and_retry"
}
`)
	deps := passthroughDeps(rt, upstream)
	deps.Guardrails = guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
	h := NewHandler(deps)
	if w := passthroughJSON(h, passthroughChatPath, "", guardrailChatBody("router/org/reasoning", guardrailTestKey(t))); w.Code != http.StatusBadRequest ||
		guardrailErrorType(t, w.Body.String()) != "secret_blocked" || attempts.Load() != 0 {
		t.Fatalf("guardrails did not block fallback inference: %d %s", w.Code, w.Body)
	}
	w := passthroughJSON(h, passthroughChatPath, "", `{"model":"router/org/reasoning","messages":[{"role":"assistant","content":[{"type":"reasoning","encrypted_content":"opaque"},{"type":"text","text":"hi"}]}]}`)
	if w.Code != http.StatusOK || attempts.Load() != 2 {
		t.Fatalf("expected one same-target retry: %d %s, calls=%d", w.Code, w.Body, attempts.Load())
	}
}

func TestModelPassthroughGuardrailRedactionDoesNotChangeTarget(t *testing.T) {
	key := "ghp_" + strings.ReplaceAll(uuid.NewString(), "-", "") + "4u7g"
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if extractModel(body) != "org/new" || bytes.Contains(body, []byte("ghp_")) ||
			!bytes.Contains(body, []byte(guardrails.DefaultRedactPlaceholder)) {
			t.Errorf("redaction lost the selected suffix or did not mask the secret")
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)
	rt := loadPassthroughRuntime(t, passthroughAnon, passthroughProvider("openai", "router", upstream.URL+"/v1", passthroughFamily))
	deps := passthroughDeps(rt, upstream)
	deps.Guardrails = guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
	deps.Exceptions = exceptionTestStore(t)
	if _, err := deps.Exceptions.Decide([]string{guardrails.Fingerprint(key)}, guardrails.ExceptionActionRedact, "test", nil); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(deps)
	if w := passthroughJSON(h, passthroughChatPath, "", guardrailChatBody("router/org/new", "test "+key)); w.Code != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("redacted content did not proceed: %d %s", w.Code, w.Body)
	}
	if w := passthroughJSON(h, passthroughChatPath, "", guardrailChatBody("router/org/"+key, "test "+key)); w.Code != http.StatusBadRequest ||
		guardrailErrorType(t, w.Body.String()) != guardrailGapType || calls.Load() != 1 {
		t.Fatalf("redaction invalidating the selected suffix was dispatched: %d", w.Code)
	}
	for _, field := range []string{"models", "PrEsEt"} {
		exceptions, err := guardrails.LoadExceptions("", field)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := exceptions.Decide([]string{guardrails.Fingerprint(key)}, guardrails.ExceptionActionRedact, "test", nil); err != nil {
			t.Fatal(err)
		}
		deps.Exceptions = exceptions
		h.UpdateDependencies(deps)
		body := strings.TrimSuffix(guardrailChatBody("router/org/new", "test "+key), "}") + fmt.Sprintf(`,%q:"other/m"}`, key)
		if w := passthroughJSON(h, passthroughChatPath, "", body); w.Code != http.StatusBadRequest ||
			guardrailErrorType(t, w.Body.String()) != guardrailGapType || calls.Load() != 1 {
			t.Fatalf("redaction introducing an alternate model selector was dispatched: %d", w.Code)
		}
	}
}

func TestModelPassthroughDoesNotUseProviderDeferredDefault(t *testing.T) {
	var ordinary, controls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == deferred.Prefix+"/capabilities" {
			controls.Add(1)
			if r.URL.Query().Get("model") != "remote/literal" {
				t.Errorf("nonliteral model used peer transport: %s", r.URL)
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ordinary.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != passthroughChatPath || extractModel(body) != "org/new" {
			t.Errorf("unwanted ordinary request: %s %s", r.URL.Path, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	t.Cleanup(upstream.Close)
	rt := loadPassthroughRuntime(t, passthroughPeerAuth, "deferred {}", passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", `
  deferred_transport {}
`+strings.Replace(passthroughLiteral, `"pinned"`, `"remote/literal"`, 1)+passthroughFamily))
	h := NewHandler(passthroughDeps(rt, upstream))
	if w := passthroughChat(h, "peer-token", "router/org/new"); w.Code != http.StatusOK || ordinary.Load() != 1 || controls.Load() != 0 {
		t.Fatalf("fallback tried deferred transport: %d %s ordinary=%d controls=%d", w.Code, w.Body, ordinary.Load(), controls.Load())
	}
	if w := passthroughChat(h, "peer-token", "router/org/pin"); w.Code != http.StatusBadGateway || ordinary.Load() != 1 || controls.Load() != 1 {
		t.Fatalf("literal did not retain provider deferred transport: %d %s ordinary=%d controls=%d", w.Code, w.Body, ordinary.Load(), controls.Load())
	}
}

func TestModelPassthroughNotEligibleForPeerReceiver(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	t.Cleanup(upstream.Close)
	f := newDeferredHTTPFixture(t, provider.New())
	rt := loadPassthroughRuntime(t, passthroughPeerAuth, "deferred {}", passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", `
  deferred_receiver {
    execution_timeout = "1s"
    max_result_bytes = 65536
  }
`+passthroughLiteral+passthroughFamily))
	deps := passthroughDeps(rt, upstream)
	deps.DeferredStore = f.store
	f.h.UpdateDependencies(deps)
	for _, model := range []string{"router/model-family", "router/org/new"} {
		w := f.call(http.MethodGet, deferred.Prefix+"/capabilities?model="+url.QueryEscape(model), "peer-token", nil)
		if w.Code != http.StatusNotFound || controlErrorType(t, w) != "model_not_found" {
			t.Fatalf("advertised nonliteral deferred target %q: %d %s", model, w.Code, w.Body)
		}
		control, err := f.store.AcquireControl(deferred.Owner{Kind: "static", ID: "peer"}, false)
		if err != nil {
			t.Fatal(err)
		}
		capability, err := f.store.Capability(control, model, deferred.ReceiverLimits{})
		control.Release()
		if err != nil {
			t.Fatal(err)
		}
		submit := deferredSubmitRequest(t, capability, []byte(fmt.Sprintf(`{"model":%q,"messages":[]}`, model)))
		w = f.call(http.MethodPost, deferred.Prefix+"/submit", "peer-token", submitWire(t, submit))
		if w.Code != http.StatusNotFound || guardrailErrorType(t, w.Body.String()) != "model_not_found" || w.Header().Get("Location") != "" || calls.Load() != 0 {
			t.Fatalf("submitted nonliteral deferred target %q: %d %s", model, w.Code, w.Body)
		}
	}
	if capability := f.capability(t, "router/org/pin"); capability.PublicModel != "router/org/pin" {
		t.Fatalf("literal receiver no longer eligible: %+v", capability)
	}
}
