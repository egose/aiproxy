package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/provider"
)

const (
	passthroughOpenCodeChatReply = `{"id":"chat-fixture","object":"chat.completion","model":"upstream-chat-revision","choices":[{"index":0,"message":{"role":"assistant","content":"fixture-answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":6,"total_tokens":10}}`
	passthroughOpenCodeChatSSE   = "data: {\"object\":\"chat.completion.chunk\",\"model\":\"upstream-chat-revision\",\"choices\":[{\"delta\":{\"content\":\"fixture-answer\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6,\"total_tokens\":10}}\n\n" +
		"data: [DONE]\n\n"
	passthroughOpenCodeResponsesReply = `{"id":"resp-fixture","object":"response","model":"upstream-responses-revision","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"fixture-answer"}]}],"usage":{"input_tokens":4,"output_tokens":6,"total_tokens":10}}`
	passthroughOpenCodeResponsesSSE   = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + passthroughOpenCodeResponsesReply + "}\n\n"
)

type passthroughOpenCodeTransport struct {
	t        *testing.T
	loopback *url.URL
	delegate http.RoundTripper
}

func (tr passthroughOpenCodeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.Host != "opencode.ai" || r.Host != "opencode.ai" {
		err := fmt.Errorf("unexpected OpenCode upstream origin: %s://%s", r.URL.Scheme, r.URL.Host)
		tr.t.Error(err)
		return nil, err
	}
	req := r.Clone(r.Context())
	req.URL.Scheme, req.URL.Host = tr.loopback.Scheme, tr.loopback.Host
	return tr.delegate.RoundTrip(req)
}

func passthroughOpenCodeDeps(t *testing.T, rt *config.Runtime, upstream *httptest.Server) Dependencies {
	t.Helper()
	loopback, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	deps := passthroughDeps(rt, upstream)
	client := &http.Client{Transport: passthroughOpenCodeTransport{t, loopback, upstream.Client().Transport}, Timeout: 10 * time.Second}
	deps.ClientForProvider = func(config.Provider) *http.Client { return client }
	return deps
}

func passthroughOpenCodeRule(name, pattern string, protocol config.ModelProtocol, capabilities string) string {
	return fmt.Sprintf(`
  model_passthrough %q {
    match = %q
    protocol = %q
    capabilities = [%s]
  }
`, name, pattern, protocol, capabilities)
}

func TestModelPassthroughOpenCodeWireProtocols(t *testing.T) {
	const (
		suffix          = "org/gemini-next.1"
		chatFields      = `"messages":[{"role":"user","content":"fixture-input"}],"tools":[{"type":"function","function":{"name":"choose"}}],"tool_choice":"auto"`
		responsesFields = `"input":"fixture-input","tools":[{"type":"function","name":"choose"}]`
		translated      = `"max_tokens":64,"messages":[{"role":"user","content":[{"type":"text","text":"fixture-input"}]}]`
	)
	for _, tc := range []struct {
		protocol                                                     config.ModelProtocol
		caps, path, wirePath, inbound, outbound, jsonReply, sseReply string
		jsonObject, streamMarker                                     string
	}{
		{config.ModelProtocolChat, `"chat"`, passthroughChatPath, "/chat/completions", chatFields, chatFields,
			passthroughOpenCodeChatReply, passthroughOpenCodeChatSSE, "", ""},
		{config.ModelProtocolResponses, `"responses"`, passthroughResponsesPath, "/responses", responsesFields, responsesFields,
			passthroughOpenCodeResponsesReply, passthroughOpenCodeResponsesSSE, "", ""},
		{config.ModelProtocolMessages, `"chat","responses","messages"`, passthroughMessagesPath, "/messages", passthroughNativeFields, passthroughNativeFields,
			passthroughAnthropicReply, passthroughAnthropicSSE, "", ""},
		{config.ModelProtocolMessages, `"chat","responses","messages"`, passthroughChatPath, "/messages",
			`"max_tokens":64,"messages":[{"role":"user","content":[` + passthroughReasoningPart + `,{"type":"text","text":"fixture-input"}]}]`,
			translated, passthroughAnthropicReply, passthroughAnthropicSSE, `"object":"chat.completion"`, `"object":"chat.completion.chunk"`},
		{config.ModelProtocolMessages, `"chat","responses","messages"`, passthroughResponsesPath, "/messages",
			`"max_output_tokens":64,"input":[` + passthroughReasoningPart + `,{"type":"message","role":"user","content":"fixture-input"}]`,
			translated, passthroughAnthropicReply, passthroughAnthropicSSE, `"object":"response"`, `"type":"response.completed"`},
	} {
		for _, service := range []struct{ typ, name, basePath string }{
			{"opencode-zen", "zen", "/zen/v1"},
			{"opencode-go", "go", "/zen/go/v1"},
		} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s%s/stream=%t", service.name, tc.protocol, tc.path, stream), func(t *testing.T) {
					contentType, mockReply, streamField := "application/json", tc.jsonReply, ""
					if stream {
						contentType, mockReply, streamField = "text/event-stream", tc.sseReply, `,"stream":true`
					}
					upstream, calls := newPassthroughWireMock(t, func(w http.ResponseWriter, _ passthroughWireCall) {
						w.Header().Set("Content-Type", contentType)
						_, _ = io.WriteString(w, mockReply)
					})
					public, key := service.name+"/"+suffix, "key-"+service.name
					rt := loadPassthroughRuntime(t, `encrypted_reasoning { passthrough = false }`,
						fmt.Sprintf(`provider %q %q {
  api_key = %q
  %s
}`, service.typ, service.name, key, passthroughOpenCodeRule("body-family", "^org/.+$", tc.protocol, tc.caps)),
						fmt.Sprintf(`auth "fixture" {
  mode = "bearer_static"
  client "app" {
    token = "caller-token"
    allowed_model_passthroughs = [%q]
  }
}`, service.name+"/body-family"))
					deps := passthroughOpenCodeDeps(t, rt, upstream)
					enablePassthroughUsage(&deps)
					proxy := newPassthroughWireProxy(t, deps, upstream)
					body := fmt.Sprintf(`{"model":%q,%s%s}`, public, tc.inbound, streamField)
					r, err := http.NewRequest(http.MethodPost, proxy.URL+tc.path, strings.NewReader(body))
					if err != nil {
						t.Fatal(err)
					}
					for k, v := range map[string]string{
						"Authorization": "Bearer caller-token", "Content-Type": "application/json", "User-Agent": "caller-agent",
						"Cookie": "ignored=fixture", "X-Api-Key": "caller-key", "Anthropic-Version": "caller-version",
						"X-OpenCode-Protocol": "gemini", "X-Session-Id": "fixture-fallback",
						"X-OpenCode-Session": "fixture_session", "X-OpenCode-Client": "fixture_client",
					} {
						r.Header.Set(k, v)
					}
					wantSession, wantClient, wantAccept := "fixture_session", "fixture_client", ""
					if stream {
						r.Header.Set("X-OpenCode-Session", "invalid session")
						r.Header.Set("X-OpenCode-Client", "invalid/client")
						wantSession, wantClient = "fixture-fallback", ""
						if tc.protocol == config.ModelProtocolMessages {
							wantAccept = "text/event-stream"
						}
					}
					resp, err := proxy.Client().Do(r)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					raw, err := io.ReadAll(resp.Body)
					reply, got := string(raw), calls()
					if err != nil || resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != contentType || len(got) != 1 {
						t.Fatalf("response = %d %s (%v); upstream calls = %d", resp.StatusCode, reply, err, len(got))
					}
					call := got[0]
					if call.method != http.MethodPost || call.path != service.basePath+tc.wirePath || call.query != "" ||
						!slices.Equal(call.header.Values("Authorization"), []string{"Bearer " + key}) ||
						call.header.Get("User-Agent") != "aiproxy/passthrough-test" || call.header.Get("Content-Type") != "application/json" ||
						call.header.Get("X-OpenCode-Session") != wantSession || call.header.Get("X-OpenCode-Client") != wantClient ||
						call.header.Get("Accept") != wantAccept {
						t.Fatalf("unexpected OpenCode request: %+v", call)
					}
					for _, name := range []string{"Cookie", "X-Api-Key", "Anthropic-Version", "X-OpenCode-Protocol", "X-Session-Id"} {
						if call.header.Get(name) != "" {
							t.Errorf("unexpected upstream header %s", name)
						}
					}
					assertPassthroughWireJSON(t, call.body, fmt.Sprintf(`{"model":%q,%s%s}`, suffix, tc.outbound, streamField))
					switch {
					case tc.jsonObject == "":
						if reply != mockReply {
							t.Fatalf("native response changed: %s", reply)
						}
					case stream:
						if !strings.Contains(reply, `"model":"`+public+`"`) || !strings.Contains(reply, tc.streamMarker) ||
							!strings.Contains(reply, "fixture-answer") || !strings.HasSuffix(reply, "data: [DONE]\n\n") {
							t.Fatalf("unexpected translated stream: %s", reply)
						}
					default:
						if extractModel(raw) != public || !strings.Contains(reply, tc.jsonObject) || !strings.Contains(reply, "fixture-answer") {
							t.Fatalf("unexpected translated response: %s", reply)
						}
					}
					if strings.Contains(reply, "@passthrough") {
						t.Fatal("response leaked the synthetic accounting identity")
					}
					op, _ := provider.OperationForHTTP(http.MethodPost, tc.path)
					assertPassthroughWireBilling(t, proxy, "caller-token", service.name, op)
				})
			}
		}
	}
}

func TestModelPassthroughOpenCodeAdmissionAndDirectIdentity(t *testing.T) {
	const (
		quota   = `{"error":{"type":"quota_exhausted","message":"Go quota exhausted"}}`
		missing = `{"error":{"type":"vendor_model_not_found","message":"missing pinned model"}}`
	)
	upstream, calls := newPassthroughWireMock(t, func(w http.ResponseWriter, call passthroughWireCall) {
		w.Header().Set("Content-Type", "application/json")
		switch model := extractModel([]byte(call.body)); {
		case model == "pinned-chat":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, missing)
		case model == "org/shared" && call.path == "/zen/go/v1/chat/completions":
			w.Header().Set("Retry-After", "45")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, quota)
		case model == "org/native-mismatch" || (model == "org/recover" && strings.Contains(call.body, `"encrypted_content"`)):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, scopedReasoningMismatch)
		case model == "org/recover":
			_, _ = io.WriteString(w, passthroughOpenCodeChatReply)
		default:
			_, _ = io.WriteString(w, passthroughAnthropicReply)
		}
	})
	rt := loadPassthroughRuntime(t, `
encrypted_reasoning { on_caller_mismatch = "strip_and_retry" }
auth "fixture" {
  mode = "bearer_static"
  client "all" {
    token = "all"
    allowed_model_passthroughs = ["zen/body-family","zen/narrow","go/body-family","go/responses-family"]
  }
  client "rule-only" {
    token = "rule-only"
    allowed_models = ["zen/unrelated"]
    allowed_model_passthroughs = ["zen/body-family"]
  }
}
provider "opencode-zen" "zen" {
  model "org/pin" {
    protocol = "chat"
    upstream_name = "pinned-chat"
  }
`+passthroughOpenCodeRule("body-family", "^org/(shared|pin|overlap|native-mismatch)$", config.ModelProtocolMessages, `"chat","responses","messages"`)+
		passthroughOpenCodeRule("overlap", "^org/overlap$", config.ModelProtocolResponses, `"responses"`)+
		passthroughOpenCodeRule("narrow", "^org/narrow$", config.ModelProtocolMessages, `"messages"`)+`
}
provider "opencode-go" "go" {
  api_key = "go-key"
`+passthroughOpenCodeRule("body-family", "^org/(shared|recover)$", config.ModelProtocolChat, `"chat"`)+
		passthroughOpenCodeRule("responses-family", "^org/responses$", config.ModelProtocolResponses, `"responses"`)+`
}
`)
	deps := passthroughOpenCodeDeps(t, rt, upstream)
	deps.Guardrails = guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
	key := guardrailTestKey(t)
	exceptions, err := guardrails.LoadExceptions("", "FaLlBaCkS")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exceptions.Decide([]string{guardrails.Fingerprint(key)}, guardrails.ExceptionActionRedact, "fixture", nil); err != nil {
		t.Fatal(err)
	}
	deps.Exceptions = exceptions
	proxy := newPassthroughWireProxy(t, deps, upstream)
	for _, tc := range []struct {
		name, path, model, bearer, fields, code string
		status                                  int
	}{
		{"chat-excludes-responses", passthroughResponsesPath, "go/org/shared", "all", `,"input":"hi"`, "model_not_found", http.StatusNotFound},
		{"chat-excludes-messages", passthroughMessagesPath, "go/org/shared", "all", `,"messages":[]`, "model_not_found", http.StatusNotFound},
		{"responses-excludes-chat", passthroughChatPath, "go/org/responses", "all", `,"messages":[]`, "model_not_found", http.StatusNotFound},
		{"messages-excludes-embeddings", "/v1/embeddings", "zen/org/shared", "all", `,"input":"hi"`, "model_not_found", http.StatusNotFound},
		{"messages-narrowed-to-native", passthroughChatPath, "zen/org/narrow", "all", `,"messages":[]`, "model_not_found", http.StatusNotFound},
		{"translated-chat-still-rejects-tools", passthroughChatPath, "zen/org/shared", "all", `,"messages":[],"tools":[{"type":"function"}]`, "invalid_request", http.StatusBadRequest},
		{"overlap-before-protocol-and-grant", passthroughChatPath, "zen/org/overlap", "all", `,"messages":[]`, "model_not_found", http.StatusNotFound},
		{"shared-label-does-not-grant-go", passthroughChatPath, "go/org/shared", "rule-only", `,"messages":[]`, "forbidden", http.StatusForbidden},
		{"exact-denied", passthroughChatPath, "zen/org/pin", "rule-only", `,"messages":[]`, "forbidden", http.StatusForbidden},
		{"exact-unsupported", passthroughResponsesPath, "zen/org/pin", "all", `,"input":"hi"`, "unsupported_operation", http.StatusBadRequest},
		{"native-fallbacks", passthroughMessagesPath, "zen/org/shared", "rule-only", `,"stream":true,"FaLl\u0062AcKs":["other/model"]`, "forbidden", http.StatusForbidden},
		{"models", passthroughChatPath, "go/org/shared", "all", `,"mOdElS":["other/model"]`, "model_not_found", http.StatusNotFound},
		{"preset", passthroughResponsesPath, "go/org/responses", "all", `,"preset":"other"`, "model_not_found", http.StatusNotFound},
		{"duplicate-model", passthroughChatPath, "other/model", "all", `,"model":"go/org/shared"`, "model_not_found", http.StatusNotFound},
		{"post-guardrail-fallbacks", passthroughMessagesPath, "zen/org/shared", "all",
			fmt.Sprintf(`,"messages":[{"role":"user","content":%q}],%q:["other/model"]`, key, key), guardrailGapType, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, tc.path, tc.bearer, fmt.Sprintf(`{"model":%q%s}`, tc.model, tc.fields))
			if resp.StatusCode != tc.status || guardrailErrorType(t, reply) != tc.code || len(calls()) != 0 {
				t.Fatalf("response = %d %s; upstream calls = %+v", resp.StatusCode, reply, calls())
			}
		})
	}
	for _, tc := range []struct {
		model, bearer, path, fields, wirePath, wireModel, auth, reply, retryAfter string
		status                                                                    int
	}{
		{"go/org/shared", "all", passthroughChatPath, `"messages":[]`, "/zen/go/v1/chat/completions", "org/shared", "Bearer go-key", quota, "45", http.StatusTooManyRequests},
		{"zen/org/shared", "rule-only", passthroughChatPath, `"messages":[{"role":"user","content":"hi"}]`, "/zen/v1/messages", "org/shared", "", "", "", http.StatusOK},
		{"zen/org/pin", "all", passthroughChatPath, `"messages":[]`, "/zen/v1/chat/completions", "pinned-chat", "", missing, "", http.StatusNotFound},
		{"zen/org/native-mismatch", "all", passthroughMessagesPath, passthroughNativeFields, "/zen/v1/messages", "org/native-mismatch", "", scopedReasoningMismatch, "", http.StatusBadRequest},
	} {
		t.Run(tc.model, func(t *testing.T) {
			before := len(calls())
			resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, tc.path, tc.bearer, fmt.Sprintf(`{"model":%q,%s}`, tc.model, tc.fields))
			got := calls()
			if resp.StatusCode != tc.status || (tc.reply != "" && reply != tc.reply) || resp.Header.Get("Retry-After") != tc.retryAfter || len(got) != before+1 {
				t.Fatalf("response = %d %s; upstream calls = %+v", resp.StatusCode, reply, got[before:])
			}
			call := got[before]
			if call.path != tc.wirePath || extractModel([]byte(call.body)) != tc.wireModel || call.header.Get("Authorization") != tc.auth ||
				!strings.HasPrefix(call.header.Get("X-OpenCode-Session"), "ses_") || (tc.reply == "" && extractModel([]byte(reply)) != tc.model) {
				t.Fatalf("wrong service, protocol, or identity: %+v; reply = %s", call, reply)
			}
			if tc.path == passthroughMessagesPath {
				assertPassthroughWireJSON(t, call.body, fmt.Sprintf(`{"model":%q,%s}`, tc.wireModel, tc.fields))
			}
		})
	}
	before := len(calls())
	resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, passthroughChatPath, "all",
		`{"model":"go/org/recover","messages":[{"role":"assistant","content":[`+passthroughReasoningPart+`,{"type":"text","text":"keep"}]}]}`)
	attempts := calls()[before:]
	if resp.StatusCode != http.StatusOK || reply != passthroughOpenCodeChatReply || len(attempts) != 2 {
		t.Fatalf("expected one same-target reasoning retry: %d %s; upstream calls = %+v", resp.StatusCode, reply, attempts)
	}
	for i, call := range attempts {
		if call.path != "/zen/go/v1/chat/completions" || call.header.Get("Authorization") != "Bearer go-key" ||
			extractModel([]byte(call.body)) != "org/recover" || strings.Contains(call.body, `"encrypted_content"`) != (i == 0) ||
			!strings.Contains(call.body, `"text":"keep"`) {
			t.Fatalf("unexpected reasoning attempt %d: %+v", i, call)
		}
	}
	resp, reply = passthroughWireRequest(t, proxy, http.MethodGet, "/v1/models", "all", "")
	var inventory struct{ Data []ModelCard }
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(reply), &inventory) != nil ||
		len(inventory.Data) != 1 || inventory.Data[0].ID != "zen/org/pin" || len(calls()) != before+2 {
		t.Fatalf("rule registered a model or performed discovery: %d %s", resp.StatusCode, reply)
	}
}
