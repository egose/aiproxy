package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/app"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func newTestServer(t *testing.T, configPath string) *httptest.Server {
	t.Helper()
	a, err := app.Build(context.Background(), app.BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	server := httptest.NewServer(a.Server.Handler)
	t.Cleanup(server.Close)
	return server
}

func TestEndToEndHealthAndReady(t *testing.T) {
	upstream := newOpenAIStub(t,
		`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`,
		`{"object":"list","data":[],"model":"text-embedding-3-large"}`,
		`{"id":"resp_1","object":"response","output":[]}`,
	)
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  base_url = "`+upstream.URL()+`"
  api_key  = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	server := newTestServer(t, configPath)

	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestEndToEndDirectChatRouting(t *testing.T) {
	openai := newOpenAIStub(t,
		`{"id":"chatcmpl_direct","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"from-openai"},"finish_reason":"stop"}]}`,
		`{"object":"list","data":[],"model":"unused"}`,
		`{"id":"resp_unused","object":"response","output":[]}`,
	)
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  base_url = "`+openai.URL()+`"
  api_key  = "sk-openai"
  model "gpt-4o-mini" {
    upstream_name = "gpt-4o-2024-08-06"
  }
}
`)
	server := newTestServer(t, configPath)

	resp, err := server.Client().Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("post chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	calls := openai.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	if calls[0].Path != "/v1/chat/completions" {
		t.Fatalf("path = %q", calls[0].Path)
	}
	if !strings.Contains(calls[0].Body, `"model":"gpt-4o-2024-08-06"`) {
		t.Fatalf("upstream body missing rewritten model: %s", calls[0].Body)
	}
}

func TestEndToEndAliasRoutingToOpenAICompatible(t *testing.T) {
	openai := newOpenAIStub(t,
		`{"id":"chatcmpl_openai","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"from-openai"},"finish_reason":"stop"}]}`,
		`{"object":"list","data":[],"model":"unused"}`,
		`{"id":"resp_unused","object":"response","output":[]}`,
	)
	localai := newOpenAIStub(t,
		`{"id":"chatcmpl_alias","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"from-alias"},"finish_reason":"stop"}]}`,
		`{"object":"list","data":[],"model":"unused"}`,
		`{"id":"resp_unused","object":"response","output":[]}`,
	)
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  base_url = "`+openai.URL()+`"
  api_key  = "sk-openai"
  model "gpt-4o-mini" {}
}
provider "openai-compatible" "localai" {
  base_url = "`+localai.URL()+`/v1"
  api_key  = "sk-local"
  model "qwen3-32b" {
    upstream_name = "qwen3-32b-upstream"
  }
}
alias "chat_default" {
  algorithm = "round_robin"
  target {
    provider = "localai"
    model    = "qwen3-32b"
  }
}
`)
	server := newTestServer(t, configPath)

	resp, err := server.Client().Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"alias/chat_default","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("post alias chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(openai.Calls()) != 0 {
		t.Fatalf("direct openai stub should not be called for alias request")
	}
	calls := localai.Calls()
	if len(calls) != 1 {
		t.Fatalf("alias upstream calls = %d", len(calls))
	}
	if calls[0].Path != "/v1/chat/completions" {
		t.Fatalf("alias path = %q", calls[0].Path)
	}
	if !strings.Contains(calls[0].Body, `"model":"qwen3-32b-upstream"`) {
		t.Fatalf("alias upstream body missing rewritten model: %s", calls[0].Body)
	}
}

func TestEndToEndEmbeddingsAndResponses(t *testing.T) {
	openai := newOpenAIStub(t,
		`{"id":"chat_unused","object":"chat.completion","choices":[]}`,
		`{"object":"list","data":[{"object":"embedding","embedding":[0.1,0.2],"index":0}],"model":"text-embedding-3-large"}`,
		`{"id":"resp_123","object":"response","output":[]}`,
	)
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  base_url = "`+openai.URL()+`"
  api_key  = "sk-openai"
  model "text-embedding-3-large" {
    capabilities = ["embeddings"]
  }
  model "gpt-4.1" {
    capabilities = ["responses"]
  }
}
`)
	server := newTestServer(t, configPath)

	embedResp, err := server.Client().Post(server.URL+"/v1/embeddings", "application/json", strings.NewReader(`{"model":"openai/text-embedding-3-large","input":"hello"}`))
	if err != nil {
		t.Fatalf("post embeddings: %v", err)
	}
	defer embedResp.Body.Close()
	if embedResp.StatusCode != http.StatusOK {
		t.Fatalf("embeddings status = %d", embedResp.StatusCode)
	}
	var embedJSON struct {
		Object string `json:"object"`
	}
	if err := json.NewDecoder(embedResp.Body).Decode(&embedJSON); err != nil {
		t.Fatalf("decode embeddings: %v", err)
	}
	if embedJSON.Object != "list" {
		t.Fatalf("embeddings object = %q", embedJSON.Object)
	}

	respResp, err := server.Client().Post(server.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"openai/gpt-4.1","input":"hello"}`))
	if err != nil {
		t.Fatalf("post responses: %v", err)
	}
	defer respResp.Body.Close()
	if respResp.StatusCode != http.StatusOK {
		t.Fatalf("responses status = %d", respResp.StatusCode)
	}
	var respJSON struct {
		Object string `json:"object"`
	}
	if err := json.NewDecoder(respResp.Body).Decode(&respJSON); err != nil {
		t.Fatalf("decode responses: %v", err)
	}
	if respJSON.Object != "response" {
		t.Fatalf("responses object = %q", respJSON.Object)
	}

	calls := openai.Calls()
	if len(calls) != 2 {
		t.Fatalf("upstream call count = %d", len(calls))
	}
	if calls[0].Path != "/v1/embeddings" || calls[1].Path != "/v1/responses" {
		t.Fatalf("upstream paths = %+v", calls)
	}
}

func TestEndToEndGeminiEmbeddings(t *testing.T) {
	gemini := newGeminiStub(t,
		`{"embedding":{"values":[0.1,0.2,0.3]},"usageMetadata":{"promptTokenCount":4,"totalTokenCount":4}}`,
		``,
		``,
	)
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "gemini" "gemini" {
  base_url = "`+gemini.URL()+`"
  api_key  = "gem-key"
  model "text-embedding-004" {
    capabilities = ["embeddings"]
  }
}
`)
	server := newTestServer(t, configPath)

	resp, err := server.Client().Post(server.URL+"/v1/embeddings", "application/json", strings.NewReader(`{"model":"gemini/text-embedding-004","input":"hello","dimensions":3}`))
	if err != nil {
		t.Fatalf("post gemini embeddings: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("embeddings status = %d", resp.StatusCode)
	}
	var out struct {
		Object string `json:"object"`
		Model  string `json:"model"`
		Data   []struct {
			Index int `json:"index"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode embeddings: %v", err)
	}
	if out.Object != "list" || out.Model != "gemini/text-embedding-004" || len(out.Data) != 1 {
		t.Fatalf("unexpected embeddings response: %+v", out)
	}
	calls := gemini.Calls()
	if len(calls) != 1 {
		t.Fatalf("stub calls = %d", len(calls))
	}
	if calls[0].Path != "/v1beta/models/text-embedding-004:embedContent" {
		t.Fatalf("path = %q", calls[0].Path)
	}
	if calls[0].APIKey != "gem-key" {
		t.Fatalf("x-goog-api-key = %q", calls[0].APIKey)
	}
	if !strings.Contains(calls[0].Body, `"outputDimensionality":3`) {
		t.Fatalf("request body missing outputDimensionality: %s", calls[0].Body)
	}
}

func TestEndToEndAnthropicResponses(t *testing.T) {
	anthropic := newAnthropicStub(t,
		`{"id":"msg_resp","role":"assistant","content":[{"type":"text","text":"from-anthropic-responses"}],"stop_reason":"end_turn","usage":{"input_tokens":7,"output_tokens":5}}`,
		``,
	)
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "anthropic" "anthropic" {
  base_url = "`+anthropic.URL()+`"
  api_key  = "sk-ant"
  model "claude-sonnet" {}
}
`)
	server := newTestServer(t, configPath)

	resp, err := server.Client().Post(server.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"anthropic/claude-sonnet","instructions":"Be concise.","input":"hello"}`))
	if err != nil {
		t.Fatalf("post anthropic responses: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("responses status = %d", resp.StatusCode)
	}
	var out struct {
		Object string `json:"object"`
		Model  string `json:"model"`
		Output []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode responses: %v", err)
	}
	if out.Object != "response" || out.Model != "anthropic/claude-sonnet" || len(out.Output) != 1 {
		t.Fatalf("unexpected response payload: %+v", out)
	}
	if out.Output[0].Role != "assistant" || out.Output[0].Content[0].Text != "from-anthropic-responses" {
		t.Fatalf("unexpected output: %+v", out.Output)
	}
	calls := anthropic.Calls()
	if len(calls) != 1 || calls[0].Path != "/v1/messages" {
		t.Fatalf("anthropic calls = %+v", calls)
	}
	if calls[0].APIKey != "sk-ant" {
		t.Fatalf("anthropic x-api-key = %q", calls[0].APIKey)
	}
	if !strings.Contains(calls[0].Body, `"system":"Be concise."`) {
		t.Fatalf("anthropic request missing instructions: %s", calls[0].Body)
	}
}

func TestEndToEndGeminiResponses(t *testing.T) {
	gemini := newGeminiStub(t,
		`{"embedding":{"values":[0.1]},"usageMetadata":{"promptTokenCount":1,"totalTokenCount":1}}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"from-gemini-responses"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":6,"candidatesTokenCount":4,"totalTokenCount":10}}`,
		``,
	)
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "gemini" "gemini" {
  base_url = "`+gemini.URL()+`"
  api_key  = "gem-key"
  model "gemini-2.5-pro" {}
}
`)
	server := newTestServer(t, configPath)

	resp, err := server.Client().Post(server.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"gemini/gemini-2.5-pro","instructions":"Be concise.","input":"hello"}`))
	if err != nil {
		t.Fatalf("post gemini responses: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("responses status = %d", resp.StatusCode)
	}
	var out struct {
		Object string `json:"object"`
		Model  string `json:"model"`
		Output []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode responses: %v", err)
	}
	if out.Object != "response" || out.Model != "gemini/gemini-2.5-pro" || len(out.Output) != 1 {
		t.Fatalf("unexpected response payload: %+v", out)
	}
	if out.Output[0].Role != "assistant" || out.Output[0].Content[0].Text != "from-gemini-responses" {
		t.Fatalf("unexpected output: %+v", out.Output)
	}
	calls := gemini.Calls()
	if len(calls) != 1 || calls[0].Path != "/v1beta/models/gemini-2.5-pro:generateContent" {
		t.Fatalf("gemini calls = %+v", calls)
	}
	if !strings.Contains(calls[0].Body, `"systemInstruction"`) {
		t.Fatalf("gemini request missing instructions: %s", calls[0].Body)
	}
}

func TestEndToEndAnthropicResponsesStreaming(t *testing.T) {
	anthropic := newAnthropicStub(t,
		`{"id":"msg_unused","role":"assistant","content":[{"type":"text","text":"unused"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
		"event: message_start\n"+
			"data: {\"message\":{\"id\":\"msg_resp_stream\",\"usage\":{\"input_tokens\":7}}}\n\n"+
			"event: content_block_delta\n"+
			"data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"from-anthropic-\"}}\n\n"+
			"event: content_block_delta\n"+
			"data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"stream\"}}\n\n"+
			"event: message_delta\n"+
			"data: {\"usage\":{\"input_tokens\":7,\"output_tokens\":5}}\n\n"+
			"event: message_stop\n"+
			"data: {}\n\n",
	)
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "anthropic" "anthropic" {
  base_url = "`+anthropic.URL()+`"
  api_key  = "sk-ant"
  model "claude-sonnet" {}
}
`)
	server := newTestServer(t, configPath)

	resp, err := server.Client().Post(server.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"anthropic/claude-sonnet","stream":true,"input":"hello"}`))
	if err != nil {
		t.Fatalf("post anthropic streaming responses: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("responses status = %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type = %q", resp.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read streaming responses: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"type":"response.created"`) || !strings.Contains(text, `"type":"response.completed"`) {
		t.Fatalf("unexpected stream body: %q", text)
	}
	if !strings.Contains(text, `"text":"from-anthropic-stream"`) {
		t.Fatalf("missing translated final text: %q", text)
	}
	if !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("missing done marker: %q", text)
	}
	calls := anthropic.Calls()
	if len(calls) != 1 || calls[0].Path != "/v1/messages" {
		t.Fatalf("anthropic calls = %+v", calls)
	}
	if !strings.Contains(calls[0].Body, `"stream":true`) {
		t.Fatalf("anthropic request missing stream flag: %s", calls[0].Body)
	}
}

func TestEndToEndGeminiResponsesStreaming(t *testing.T) {
	gemini := newGeminiStub(t,
		`{"embedding":{"values":[0.1]},"usageMetadata":{"promptTokenCount":1,"totalTokenCount":1}}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"unused"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
		"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"from-gemini-\"}]}}],\"usageMetadata\":{\"promptTokenCount\":6}}\n\n"+
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"stream\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":6,\"candidatesTokenCount\":4,\"totalTokenCount\":10}}\n\n",
	)
	configPath := writeConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "gemini" "gemini" {
  base_url = "`+gemini.URL()+`"
  api_key  = "gem-key"
  model "gemini-2.5-pro" {}
}
`)
	server := newTestServer(t, configPath)

	resp, err := server.Client().Post(server.URL+"/v1/responses", "application/json", strings.NewReader(`{"model":"gemini/gemini-2.5-pro","stream":true,"input":"hello"}`))
	if err != nil {
		t.Fatalf("post gemini streaming responses: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("responses status = %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type = %q", resp.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read streaming responses: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"type":"response.created"`) || !strings.Contains(text, `"type":"response.completed"`) {
		t.Fatalf("unexpected stream body: %q", text)
	}
	if !strings.Contains(text, `"text":"from-gemini-stream"`) {
		t.Fatalf("missing translated final text: %q", text)
	}
	if !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("missing done marker: %q", text)
	}
	calls := gemini.Calls()
	if len(calls) != 1 || calls[0].Path != "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse" {
		t.Fatalf("gemini calls = %+v", calls)
	}
}
