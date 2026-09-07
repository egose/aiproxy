package e2e

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type openCodeCall struct {
	Path          string
	RawQuery      string
	Authorization string
	UserAgent     string
	Session       string
	APIKey        string
	Cookie        string
	Custom        string
	ContentType   string
	Body          string
}

type openCodeStub struct {
	server  *httptest.Server
	mu      sync.Mutex
	calls   []openCodeCall
	handler func(w http.ResponseWriter, r *http.Request, body string)
}

func newOpenCodeStub(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body string)) *openCodeStub {
	t.Helper()
	s := &openCodeStub{handler: handler}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read stub request body: %v", err)
		}
		s.mu.Lock()
		s.calls = append(s.calls, openCodeCall{
			Path:          r.URL.Path,
			RawQuery:      r.URL.RawQuery,
			Authorization: r.Header.Get("Authorization"),
			UserAgent:     r.Header.Get("User-Agent"),
			Session:       r.Header.Get("x-opencode-session"),
			APIKey:        r.Header.Get("x-api-key"),
			Cookie:        r.Header.Get("Cookie"),
			Custom:        r.Header.Get("X-Custom"),
			ContentType:   r.Header.Get("Content-Type"),
			Body:          string(raw),
		})
		s.mu.Unlock()
		s.handler(w, r, string(raw))
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *openCodeStub) URL() string {
	return s.server.URL
}

func (s *openCodeStub) Calls() []openCodeCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]openCodeCall, len(s.calls))
	copy(out, s.calls)
	return out
}

func writeOpenCodeJSON(w http.ResponseWriter, status int, payload string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, payload)
}

func postOpenCode(t *testing.T, server *httptest.Server, path, body string, headers map[string]string) (int, string, http.Header) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp.StatusCode, string(out), resp.Header
}

func TestE2EOpenCodeDirectChatAndResponses(t *testing.T) {
	zen := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		writeOpenCodeJSON(w, http.StatusOK, `{"id":"chatcmpl_zen","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"from-zen"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`)
	})
	goUp := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		writeOpenCodeJSON(w, http.StatusOK, `{"id":"resp_go","object":"response","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"from-go"}]}],"usage":{"input_tokens":2,"output_tokens":4,"total_tokens":6}}`)
	})
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+zen.URL()+`"
  api_key  = "sk-zen"
  model "glm-5.3" {
    protocol     = "chat"
    capabilities = ["chat"]
  }
}
provider "opencode-go" "go" {
  base_url = "`+goUp.URL()+`"
  api_key  = "sk-go"
  model "grok-4.6" {
    protocol     = "responses"
    capabilities = ["responses"]
  }
}
`)
	server := newTestServer(t, configPath)

	status, body, _ := postOpenCode(t, server, "/v1/chat/completions", `{"model":"zen/glm-5.3","messages":[{"role":"user","content":"hi"}]}`, map[string]string{
		"Authorization": "Bearer inbound-secret",
		"Cookie":        "session=evil",
		"X-Custom":      "evil",
	})
	if status != http.StatusOK {
		t.Fatalf("zen chat status = %d body=%s", status, body)
	}
	if !strings.Contains(body, "from-zen") {
		t.Fatalf("zen chat body = %s", body)
	}
	zcalls := zen.Calls()
	if len(zcalls) != 1 {
		t.Fatalf("zen calls = %d", len(zcalls))
	}
	zc := zcalls[0]
	if zc.Path != "/v1/chat/completions" {
		t.Fatalf("zen path = %q", zc.Path)
	}
	if zc.Authorization != "Bearer sk-zen" {
		t.Fatalf("zen authorization = %q", zc.Authorization)
	}
	if zc.UserAgent != "aiproxy/test" {
		t.Fatalf("zen user-agent = %q", zc.UserAgent)
	}
	if zc.Session != "" {
		t.Fatalf("zen session = %q, want empty", zc.Session)
	}
	if zc.Cookie != "" || zc.Custom != "" || zc.APIKey != "" {
		t.Fatalf("zen leaked inbound headers cookie=%q custom=%q apikey=%q", zc.Cookie, zc.Custom, zc.APIKey)
	}
	if !strings.Contains(zc.Body, `"model":"glm-5.3"`) {
		t.Fatalf("zen upstream body missing model: %s", zc.Body)
	}
	if len(goUp.Calls()) != 0 {
		t.Fatalf("go stub should not be called for direct zen request")
	}

	status, body, _ = postOpenCode(t, server, "/v1/responses", `{"model":"go/grok-4.6","input":"hi"}`, map[string]string{
		"x-opencode-session": "client-session_1",
	})
	if status != http.StatusOK {
		t.Fatalf("go responses status = %d body=%s", status, body)
	}
	if !strings.Contains(body, "from-go") {
		t.Fatalf("go responses body = %s", body)
	}
	gcalls := goUp.Calls()
	if len(gcalls) != 1 {
		t.Fatalf("go calls = %d", len(gcalls))
	}
	gc := gcalls[0]
	if gc.Path != "/v1/responses" {
		t.Fatalf("go path = %q", gc.Path)
	}
	if gc.Authorization != "Bearer sk-go" {
		t.Fatalf("go authorization = %q", gc.Authorization)
	}
	if gc.Session != "client-session_1" {
		t.Fatalf("go session = %q, want forwarded client value", gc.Session)
	}
	if gc.UserAgent != "aiproxy/test" {
		t.Fatalf("go user-agent = %q", gc.UserAgent)
	}
	if !strings.Contains(gc.Body, `"model":"grok-4.6"`) {
		t.Fatalf("go upstream body missing model: %s", gc.Body)
	}
	if len(zen.Calls()) != 1 {
		t.Fatalf("zen stub should not be called for direct go request")
	}
}

func TestE2EOpenCodeTranslatedProtocols(t *testing.T) {
	messages := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		if r.URL.Path != "/messages" {
			http.NotFound(w, r)
			return
		}
		writeOpenCodeJSON(w, http.StatusOK, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi from messages"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
	})
	gemini := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		if !strings.HasPrefix(r.URL.Path, "/models/gemini-3.8-flash:generateContent") {
			http.NotFound(w, r)
			return
		}
		writeOpenCodeJSON(w, http.StatusOK, `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi from gemini"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":4,"totalTokenCount":10}}`)
	})
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "opencode-go" "go" {
  base_url = "`+messages.URL()+`"
  api_key  = "sk-go"
  model "minimax-m3" {
    protocol = "messages"
  }
}
provider "opencode-zen" "zen" {
  base_url = "`+gemini.URL()+`"
  api_key  = "sk-zen"
  model "gemini-3.8-flash" {
    protocol = "gemini"
  }
}
`)
	server := newTestServer(t, configPath)

	status, body, _ := postOpenCode(t, server, "/v1/chat/completions", `{"model":"go/minimax-m3","messages":[{"role":"user","content":"hi"}]}`, nil)
	if status != http.StatusOK {
		t.Fatalf("go messages chat status = %d body=%s", status, body)
	}
	if !strings.Contains(body, "hi from messages") {
		t.Fatalf("go messages chat body = %s", body)
	}
	mcalls := messages.Calls()
	if len(mcalls) != 1 || mcalls[0].Path != "/messages" {
		t.Fatalf("messages calls = %+v", mcalls)
	}
	if mcalls[0].Authorization != "Bearer sk-go" {
		t.Fatalf("messages authorization = %q", mcalls[0].Authorization)
	}
	if mcalls[0].Session == "" {
		t.Fatalf("go messages request missing generated session header")
	}
	if mcalls[0].APIKey != "" {
		t.Fatalf("go messages request must not use x-api-key, got %q", mcalls[0].APIKey)
	}

	status, body, _ = postOpenCode(t, server, "/v1/chat/completions", `{"model":"zen/gemini-3.8-flash","messages":[{"role":"user","content":"hi"}]}`, nil)
	if status != http.StatusOK {
		t.Fatalf("zen gemini chat status = %d body=%s", status, body)
	}
	if !strings.Contains(body, "hi from gemini") {
		t.Fatalf("zen gemini chat body = %s", body)
	}
	gcalls := gemini.Calls()
	if len(gcalls) != 1 {
		t.Fatalf("gemini calls = %d", len(gcalls))
	}
	if !strings.HasPrefix(gcalls[0].Path, "/models/gemini-3.8-flash:generateContent") {
		t.Fatalf("gemini path = %q", gcalls[0].Path)
	}
	if gcalls[0].Authorization != "Bearer sk-zen" {
		t.Fatalf("gemini authorization = %q", gcalls[0].Authorization)
	}
	if gcalls[0].Session != "" {
		t.Fatalf("zen gemini request must not carry session header, got %q", gcalls[0].Session)
	}
}

func TestE2EOpenCodeStreamingChatSSE(t *testing.T) {
	zen := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hello-zen\"},\"index\":0}]}\n\ndata: [DONE]\n\n")
	})
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+zen.URL()+`"
  api_key  = "sk-zen"
  model "glm-5.3" {
    protocol = "chat"
  }
}
`)
	server := newTestServer(t, configPath)

	status, body, header := postOpenCode(t, server, "/v1/chat/completions", `{"model":"zen/glm-5.3","stream":true,"messages":[{"role":"user","content":"hi"}]}`, nil)
	if status != http.StatusOK {
		t.Fatalf("stream status = %d body=%s", status, body)
	}
	if !strings.Contains(header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type = %q", header.Get("Content-Type"))
	}
	if !strings.Contains(body, "hello-zen") {
		t.Fatalf("stream body = %q", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("stream body missing done marker: %q", body)
	}
	zcalls := zen.Calls()
	if len(zcalls) != 1 || zcalls[0].Path != "/v1/chat/completions" {
		t.Fatalf("zen calls = %+v", zcalls)
	}
	if zcalls[0].Authorization != "Bearer sk-zen" || zcalls[0].UserAgent != "aiproxy/test" {
		t.Fatalf("zen headers auth=%q ua=%q", zcalls[0].Authorization, zcalls[0].UserAgent)
	}
}

func TestE2EOpenCodeCapabilityGating(t *testing.T) {
	zen := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		writeOpenCodeJSON(w, http.StatusOK, `{}`)
	})
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+zen.URL()+`"
  api_key  = "sk-zen"
  model "glm-5.3" {
    protocol     = "chat"
    capabilities = ["chat"]
  }
}
`)
	server := newTestServer(t, configPath)

	status, body, _ := postOpenCode(t, server, "/v1/responses", `{"model":"zen/glm-5.3","input":"hi"}`, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("responses on chat-only model status = %d body=%s", status, body)
	}
	if !strings.Contains(body, "unsupported_operation") {
		t.Fatalf("responses body = %s", body)
	}

	status, body, _ = postOpenCode(t, server, "/v1/embeddings", `{"model":"zen/glm-5.3","input":"hi"}`, nil)
	if status == http.StatusOK {
		t.Fatalf("embeddings on chat-only model unexpectedly succeeded: %s", body)
	}
	if !strings.Contains(body, "unsupported_operation") {
		t.Fatalf("embeddings body = %s", body)
	}

	if len(zen.Calls()) != 0 {
		t.Fatalf("gated requests must make zero upstream calls, got %+v", zen.Calls())
	}
}

func TestE2EOpenCodeDirectErrorsDoNotSwitchService(t *testing.T) {
	zen := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		writeOpenCodeJSON(w, http.StatusUnauthorized, `{"error":{"message":"invalid key"}}`)
	})
	limited := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		writeOpenCodeJSON(w, http.StatusTooManyRequests, `{"error":{"message":"quota exceeded"}}`)
	})
	goUp := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		writeOpenCodeJSON(w, http.StatusOK, `{"id":"chatcmpl_go","object":"chat.completion","choices":[]}`)
	})
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+zen.URL()+`"
  api_key  = "sk-zen"
  model "glm-5.3" {
    protocol = "chat"
  }
}
provider "opencode-zen" "limited" {
  base_url = "`+limited.URL()+`"
  api_key  = "sk-limited"
  model "glm-5.3" {
    protocol = "chat"
  }
}
provider "opencode-go" "go" {
  base_url = "`+goUp.URL()+`"
  api_key  = "sk-go"
  model "glm-5.3" {
    protocol = "chat"
  }
}
`)
	server := newTestServer(t, configPath)

	status, _, _ := postOpenCode(t, server, "/v1/chat/completions", `{"model":"zen/glm-5.3","messages":[{"role":"user","content":"hi"}]}`, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("direct 401 status = %d, want 401 verbatim", status)
	}
	if len(zen.Calls()) != 1 {
		t.Fatalf("zen calls = %d", len(zen.Calls()))
	}
	if len(goUp.Calls()) != 0 {
		t.Fatalf("direct auth error must not switch to go service, go calls = %d", len(goUp.Calls()))
	}

	status, _, _ = postOpenCode(t, server, "/v1/chat/completions", `{"model":"limited/glm-5.3","messages":[{"role":"user","content":"hi"}]}`, nil)
	if status != http.StatusTooManyRequests {
		t.Fatalf("direct 429 status = %d, want 429 verbatim", status)
	}
	if len(limited.Calls()) != 1 {
		t.Fatalf("limited calls = %d", len(limited.Calls()))
	}
	if len(goUp.Calls()) != 0 {
		t.Fatalf("direct quota error must not switch to go service, go calls = %d", len(goUp.Calls()))
	}
}

func TestE2EOpenCodeExplicitAliasRetry(t *testing.T) {
	zen := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		writeOpenCodeJSON(w, http.StatusBadGateway, `{"error":{"message":"overloaded"}}`)
	})
	goUp := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		writeOpenCodeJSON(w, http.StatusOK, `{"id":"chatcmpl_go","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"from-go-failover"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	})
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+zen.URL()+`"
  api_key  = "sk-zen"
  model "glm-5.3" {
    protocol = "chat"
  }
}
provider "opencode-go" "go" {
  base_url = "`+goUp.URL()+`"
  api_key  = "sk-go"
  model "glm-5.3" {
    protocol = "chat"
  }
}
alias "fallback" {
  algorithm = "round_robin"
  target {
    provider = "zen"
    model = "glm-5.3"
  }
  target {
    provider = "go"
    model = "glm-5.3"
  }
}
`)
	server := newTestServer(t, configPath)

	status, body, _ := postOpenCode(t, server, "/v1/chat/completions", `{"model":"alias/fallback","messages":[{"role":"user","content":"hi"}]}`, nil)
	if status != http.StatusOK {
		t.Fatalf("alias failover status = %d body=%s", status, body)
	}
	if !strings.Contains(body, "from-go-failover") {
		t.Fatalf("alias failover body = %s", body)
	}
	if len(zen.Calls()) != 1 || len(goUp.Calls()) != 1 {
		t.Fatalf("calls zen=%d go=%d, want 1 each", len(zen.Calls()), len(goUp.Calls()))
	}
	gc := goUp.Calls()[0]
	if gc.Authorization != "Bearer sk-go" {
		t.Fatalf("go failover authorization = %q", gc.Authorization)
	}
	if gc.Session == "" {
		t.Fatalf("go failover request missing session header")
	}
	zc := zen.Calls()[0]
	if zc.Session != "" {
		t.Fatalf("zen request must not carry session header, got %q", zc.Session)
	}
}

func TestE2EOpenCodeExplicitAliasRetryOnConfiguredQuotaStatus(t *testing.T) {
	zen := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		writeOpenCodeJSON(w, http.StatusTooManyRequests, `{"error":{"message":"quota exceeded"}}`)
	})
	goUp := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		writeOpenCodeJSON(w, http.StatusOK, `{"id":"chatcmpl_go","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"from-go-quota-failover"},"finish_reason":"stop"}]}`)
	})
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+zen.URL()+`"
  api_key  = "sk-zen"
  model "glm-5.3" {
    protocol = "chat"
  }
}
provider "opencode-go" "go" {
  base_url = "`+goUp.URL()+`"
  api_key  = "sk-go"
  model "glm-5.3" {
    protocol = "chat"
  }
}
alias "quota_fallback" {
  algorithm = "round_robin"
  retry_status_codes = ["429"]
  target {
    provider = "zen"
    model = "glm-5.3"
  }
  target {
    provider = "go"
    model = "glm-5.3"
  }
}
`)
	server := newTestServer(t, configPath)

	status, body, _ := postOpenCode(t, server, "/v1/chat/completions", `{"model":"alias/quota_fallback","messages":[{"role":"user","content":"hi"}]}`, nil)
	if status != http.StatusOK {
		t.Fatalf("alias quota failover status = %d body=%s", status, body)
	}
	if !strings.Contains(body, "from-go-quota-failover") {
		t.Fatalf("alias quota failover body = %s", body)
	}
	if len(zen.Calls()) != 1 || len(goUp.Calls()) != 1 {
		t.Fatalf("calls zen=%d go=%d, want 1 each", len(zen.Calls()), len(goUp.Calls()))
	}
}

func TestE2EOpenCodeModelsProxyOwnedAndUsageRecorded(t *testing.T) {
	zen := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		writeOpenCodeJSON(w, http.StatusOK, `{"id":"chatcmpl_zen","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"from-zen"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`)
	})
	goUp := newOpenCodeStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		writeOpenCodeJSON(w, http.StatusOK, `{}`)
	})
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+zen.URL()+`"
  api_key  = "sk-zen"
  model "glm-5.3" {
    protocol = "chat"
  }
}
provider "opencode-go" "go" {
  base_url = "`+goUp.URL()+`"
  api_key  = "sk-go"
  model "minimax-m3" {
    protocol = "messages"
  }
}
`)
	server := newTestServer(t, configPath)

	resp, err := server.Client().Get(server.URL + "/v1/models")
	if err != nil {
		t.Fatalf("GET models: %v", err)
	}
	modelsBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read models: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("models status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(modelsBody), `"id":"zen/glm-5.3"`) || !strings.Contains(string(modelsBody), `"id":"go/minimax-m3"`) {
		t.Fatalf("models = %s", modelsBody)
	}
	if len(zen.Calls()) != 0 || len(goUp.Calls()) != 0 {
		t.Fatalf("models endpoint must make zero upstream calls")
	}

	status, _, _ := postOpenCode(t, server, "/v1/chat/completions", `{"model":"zen/glm-5.3","messages":[{"role":"user","content":"hi"}]}`, nil)
	if status != http.StatusOK {
		t.Fatalf("zen chat status = %d", status)
	}

	usageResp, err := server.Client().Get(server.URL + "/v1/billing/usage")
	if err != nil {
		t.Fatalf("GET usage: %v", err)
	}
	usageBody, err := io.ReadAll(usageResp.Body)
	usageResp.Body.Close()
	if err != nil {
		t.Fatalf("read usage: %v", err)
	}
	if usageResp.StatusCode != http.StatusOK {
		t.Fatalf("usage status = %d body=%s", usageResp.StatusCode, usageBody)
	}
	if !strings.Contains(string(usageBody), "zen/glm-5.3") {
		t.Fatalf("usage missing zen model entry: %s", usageBody)
	}
}
