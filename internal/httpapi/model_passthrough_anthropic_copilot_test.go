package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

const (
	passthroughMessagesPath   = "/v1/messages"
	passthroughResponsesPath  = "/v1/responses"
	passthroughAnthropicReply = `{"id":"msg-fixture","type":"message","model":"vendor-claude-revision","role":"assistant","content":[{"type":"thinking","thinking":"opaque","signature":"fixture"},{"type":"text","text":"fixture-answer"},{"type":"tool_use","id":"toolu_2","name":"choose","input":{"fallbacks":["option"]}}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":6}}`
	passthroughAnthropicSSE   = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-fixture\",\"model\":\"vendor-claude-revision\",\"usage\":{\"input_tokens\":4,\"output_tokens\":0}}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"fixture-answer\"}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":6}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	passthroughCopilotReply = `{"id":"chat-fixture","object":"chat.completion","model":"vendor-copilot-revision","choices":[{"message":{"role":"assistant","content":"fixture-answer"},"finish_reason":"stop","index":0}],"usage":{"prompt_tokens":4,"completion_tokens":6,"total_tokens":10}}`
	passthroughCopilotSSE   = "data: {\"object\":\"chat.completion.chunk\",\"model\":\"vendor-copilot-revision\",\"choices\":[{\"delta\":{\"content\":\"fixture-answer\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6,\"total_tokens\":10}}\n\n" +
		"data: [DONE]\n\n"
	passthroughReasoningPart = `{"type":"reasoning","encrypted_content":"opaque-fixture"}`
	passthroughNativeFields  = `"max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":1024},"tools":[{"name":"choose","input_schema":{"type":"object","properties":{"fallbacks":{"type":"array","items":{"type":"string"}}}}}],"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"prior","signature":"signed-fixture"},{"type":"tool_use","id":"toolu_1","name":"choose","input":{"fallbacks":["option"]}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"fixture-input"}]}]`
	passthroughCopilotToken  = "stored-copilot-fixture"
)

func passthroughBodyRule(capabilities string) string {
	return fmt.Sprintf(`
  model_passthrough "body-family" {
    match = "^org/.+$"
    capabilities = [%s]
  }
`, capabilities)
}

func passthroughCopilotProvider(t *testing.T, baseURL, settings string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "keys.json")
	cred, err := copilotlogin.NewCredential("Ov23testclient", passthroughCopilotToken, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := copilotlogin.Save(path, "fixture", cred); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(`
provider "github-copilot" "copilot" {
  base_url = %q
  credential_ref {
    name = "fixture"
    path = %q
  }
  %s
}
`, baseURL, path, settings)
}

type passthroughWireCall struct {
	method, path, query, body string
	host, requestURI          string
	header                    http.Header
}

func newPassthroughWireMock(t *testing.T, respond func(http.ResponseWriter, passthroughWireCall)) (*httptest.Server, func() []passthroughWireCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []passthroughWireCall
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		call := passthroughWireCall{
			method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: string(body),
			host: r.Host, requestURI: r.RequestURI, header: r.Header.Clone(),
		}
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()
		respond(w, call)
	}))
	t.Cleanup(upstream.Close)
	return upstream, func() []passthroughWireCall {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(calls)
	}
}

type passthroughWireTransport struct {
	t        *testing.T
	origin   string
	delegate http.RoundTripper
}

func (tr passthroughWireTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme+"://"+r.URL.Host != tr.origin {
		err := fmt.Errorf("unexpected test upstream origin: %s", r.URL.Host)
		tr.t.Error(err)
		return nil, err
	}
	return tr.delegate.RoundTrip(r)
}

func newPassthroughWireProxy(t *testing.T, deps Dependencies, upstream *httptest.Server) *httptest.Server {
	t.Helper()
	deps.Client = &http.Client{Transport: passthroughWireTransport{t, upstream.URL, upstream.Client().Transport}, Timeout: 10 * time.Second}
	deps.Version = "passthrough-test"
	proxy := httptest.NewServer(NewHandler(deps))
	proxy.Client().Timeout = 10 * time.Second
	t.Cleanup(proxy.Close)
	return proxy
}

func passthroughWireRequest(t *testing.T, proxy *httptest.Server, method, path, bearer, body string) (*http.Response, string) {
	t.Helper()
	r, err := http.NewRequest(method, proxy.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	if method == http.MethodPost {
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("User-Agent", "caller-agent")
		r.Header.Set("Cookie", "fixture=inbound")
		r.Header.Set("X-Api-Key", "caller-key")
		r.Header.Set("Anthropic-Version", "caller-version")
		r.Header.Set("X-GitHub-Api-Version", "caller-version")
		r.Header.Set("Openai-Intent", "caller-intent")
		r.Header.Set("X-Initiator", "agent")
		r.Header.Set("Copilot-Vision-Request", "caller-vision")
		r.Header.Set("X-Interaction-Id", "caller-interaction")
	}
	resp, err := proxy.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(raw)
}

func assertPassthroughWireJSON(t *testing.T, got, want string) {
	t.Helper()
	var actual, expected any
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatalf("invalid fixture JSON: %v (%s)", err, want)
	}
	if err := json.Unmarshal([]byte(got), &actual); err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("upstream body = %s, want %s (decode error: %v)", got, want, err)
	}
}

func assertPassthroughWireBilling(t *testing.T, proxy *httptest.Server, bearer, providerName string, op provider.Operation) {
	t.Helper()
	resp, body := passthroughWireRequest(t, proxy, http.MethodGet, "/v1/billing/usage", bearer, "")
	var result billingUsageResponse
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(body), &result) != nil || len(result.Data) != 1 {
		t.Fatalf("unexpected billing: %d %s", resp.StatusCode, body)
	}
	row := result.Data[0]
	if row.Model != providerName+"/@passthrough/body-family" || row.Operation != op.String() ||
		row.StatusCode != http.StatusOK || row.Count != 1 || row.PromptTokens != 4 ||
		row.CompletionTokens != 6 || row.TotalTokens != 10 || row.EstimatedCostUSD != nil {
		t.Fatalf("rule billing lost grouping, tokens, or unpriced status: %s", body)
	}
}

func TestModelPassthroughAnthropicHTTP(t *testing.T) {
	const suffix = "org/claude-next.1"
	const public = "anthropic/" + suffix
	const translated = `"max_tokens":64,"messages":[{"role":"user","content":[{"type":"text","text":"fixture-input"}]}]`
	for _, tc := range []struct {
		name, path, inbound, outbound, replyObject, streamMarker string
		op                                                       provider.Operation
	}{
		{"native", passthroughMessagesPath, passthroughNativeFields, passthroughNativeFields, "", "", provider.OpMessages},
		{"chat", passthroughChatPath, `"max_tokens":64,"messages":[{"role":"user","content":[` + passthroughReasoningPart + `,{"type":"text","text":"fixture-input"}]}]`,
			translated, `"object":"chat.completion"`, `"object":"chat.completion.chunk"`, provider.OpChatCompletions},
		{"responses", passthroughResponsesPath, `"max_output_tokens":64,"input":[` + passthroughReasoningPart + `,{"type":"message","role":"user","content":"fixture-input"}]`,
			translated, `"object":"response"`, `"type":"response.completed"`, provider.OpResponses},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				responseType, mockReply, accept, streamField := "application/json", passthroughAnthropicReply, "", ""
				if stream {
					responseType, mockReply, accept, streamField = "text/event-stream", passthroughAnthropicSSE, "text/event-stream", `,"stream":true`
				}
				upstream, calls := newPassthroughWireMock(t, func(w http.ResponseWriter, _ passthroughWireCall) {
					w.Header().Set("Content-Type", responseType)
					_, _ = io.WriteString(w, mockReply)
				})
				rt := loadPassthroughRuntime(t, passthroughAnon, `encrypted_reasoning { passthrough = false }`,
					passthroughProvider("anthropic", "anthropic", upstream.URL+"/anthropic", passthroughBodyRule(`"chat","responses","messages"`)))
				deps := passthroughDeps(rt, upstream)
				enablePassthroughUsage(&deps)
				proxy := newPassthroughWireProxy(t, deps, upstream)

				resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, tc.path, "", fmt.Sprintf(`{"model":%q,%s%s}`, public, tc.inbound, streamField))
				got := calls()
				if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != responseType || len(got) != 1 {
					t.Fatalf("response = %d %s; upstream calls = %d", resp.StatusCode, reply, len(got))
				}
				call := got[0]
				if call.method != http.MethodPost || call.path != "/anthropic/v1/messages" || call.query != "" ||
					!slices.Equal(call.header.Values("X-Api-Key"), []string{"fixture-upstream"}) ||
					call.header.Get("Anthropic-Version") != "2023-06-01" || call.header.Get("Accept") != accept ||
					call.header.Get("Content-Type") != "application/json" ||
					call.header.Get("Authorization") != "" || call.header.Get("Cookie") != "" {
					t.Fatalf("unexpected Anthropic request: %+v", call)
				}
				assertPassthroughWireJSON(t, call.body, fmt.Sprintf(`{"model":%q,%s%s}`, suffix, tc.outbound, streamField))
				switch {
				case tc.op == provider.OpMessages:
					if reply != mockReply {
						t.Fatalf("native Messages response changed: %s", reply)
					}
				case stream:
					if !strings.Contains(reply, `"model":"`+public+`"`) || !strings.Contains(reply, tc.streamMarker) ||
						strings.Contains(reply, "@passthrough") || !strings.Contains(reply, "fixture-answer") ||
						!strings.HasSuffix(reply, "data: [DONE]\n\n") {
						t.Fatalf("unexpected translated stream: %s", reply)
					}
				default:
					if extractModel([]byte(reply)) != public || !strings.Contains(reply, tc.replyObject) || !strings.Contains(reply, "fixture-answer") {
						t.Fatalf("unexpected translated response: %s", reply)
					}
				}
				assertPassthroughWireBilling(t, proxy, "", "anthropic", tc.op)
			})
		}
	}
}

func TestModelPassthroughCopilotHTTP(t *testing.T) {
	const suffix = "org/copilot-next.1"
	const input = `"messages":[{"role":"assistant","content":[` + passthroughReasoningPart + `,{"type":"text","text":"fixture-input"}]}],"FaLlBaCkS":["extension-value"]`
	const stripped = `"messages":[{"role":"assistant","content":[{"type":"text","text":"fixture-input"}]}],"FaLlBaCkS":["extension-value"]`
	const imageInput = `"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]}],"stream":true`
	for _, tc := range []struct {
		name, input, reply string
		status, attempts   int
	}{
		{"json", input, passthroughCopilotReply, http.StatusOK, 1},
		{"sse-vision", imageInput, passthroughCopilotSSE, http.StatusOK, 1},
		{"unauthorized", input, `{"error":{"type":"vendor_auth","message":"stored credential revoked: encrypted_content was not issued to this caller"}}`, http.StatusUnauthorized, 1},
		{"forbidden", input, `{"error":{"type":"vendor_auth","message":"stored credential lacks access: encrypted_content was not issued to this caller"}}`, http.StatusForbidden, 1},
		{"reasoning-recovery", input, passthroughCopilotReply, http.StatusOK, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responseType, vision := "application/json", ""
			if tc.input == imageInput {
				responseType, vision = "text/event-stream", "true"
			}
			upstream, calls := newPassthroughWireMock(t, func(w http.ResponseWriter, call passthroughWireCall) {
				w.Header().Set("X-Mock-Upstream", "copilot")
				w.Header().Set("Content-Type", responseType)
				if tc.attempts == 2 && strings.Contains(call.body, `"encrypted_content"`) {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, scopedReasoningMismatch)
					return
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.reply)
			})
			rt := loadPassthroughRuntime(t, passthroughCopilotProvider(t, upstream.URL+"/copilot", passthroughBodyRule(`"chat"`)+`
  encrypted_reasoning { on_caller_mismatch = "strip_and_retry" }
  model "org/pin" { upstream_name = "pinned-copilot" }
`), passthroughProvider("openai", "backup", upstream.URL+"/backup", `model "org/pin" {}`), `
encrypted_reasoning { passthrough = false }
alias "backup" {
  algorithm = "round_robin"
  providers = ["copilot", "backup"]
  model = "org/pin"
  retry_status_codes = [401,403]
}
auth "fixture" {
  mode = "bearer_static"
  client "granted" {
    token = "caller-token"
    allowed_models = ["copilot/org/pin"]
    allowed_model_passthroughs = ["copilot/body-family"]
  }
}
`)
			deps := passthroughDeps(rt, upstream)
			enablePassthroughUsage(&deps)
			proxy := newPassthroughWireProxy(t, deps, upstream)
			resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, passthroughChatPath, "caller-token",
				fmt.Sprintf(`{"model":"copilot/%s",%s}`, suffix, tc.input))
			got := calls()
			if resp.StatusCode != tc.status || reply != tc.reply || resp.Header.Get("Content-Type") != responseType ||
				resp.Header.Get("X-Mock-Upstream") != "copilot" || len(got) != tc.attempts {
				t.Fatalf("response = %d %s; upstream calls = %+v", resp.StatusCode, reply, got)
			}
			for i, call := range got {
				if call.method != http.MethodPost || call.path != "/copilot/chat/completions" || call.query != "" ||
					!slices.Equal(call.header.Values("Authorization"), []string{"Bearer " + passthroughCopilotToken}) ||
					call.header.Get("User-Agent") != "aiproxy/passthrough-test" ||
					call.header.Get("X-GitHub-Api-Version") != copilotlogin.APIVersion ||
					call.header.Get("Openai-Intent") != copilotlogin.OpenAIIntent ||
					call.header.Get("X-Initiator") != "user" || call.header.Get("Accept") != responseType ||
					call.header.Get("Content-Type") != "application/json" ||
					call.header.Get("Copilot-Vision-Request") != vision {
					t.Fatalf("unexpected Copilot request: %+v", call)
				}
				for _, header := range []string{"Cookie", "X-Api-Key", "X-Interaction-Id", "Anthropic-Version"} {
					if call.header.Get(header) != "" {
						t.Errorf("caller header %s leaked upstream", header)
					}
				}
				fields := tc.input
				if i == 1 {
					fields = stripped
				}
				assertPassthroughWireJSON(t, call.body, fmt.Sprintf(`{"model":%q,%s}`, suffix, fields))
			}
			if tc.status == http.StatusOK {
				assertPassthroughWireBilling(t, proxy, "caller-token", "copilot", provider.OpChatCompletions)
			}
		})
	}
}

func TestModelPassthroughBodyAdapterAdmissionHTTP(t *testing.T) {
	const missing = `{"type":"error","error":{"type":"vendor_model_not_found","message":"missing pinned model"}}`
	upstream, calls := newPassthroughWireMock(t, func(w http.ResponseWriter, call passthroughWireCall) {
		w.Header().Set("Content-Type", "application/json")
		switch extractModel([]byte(call.body)) {
		case "pinned-claude":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, missing)
		case "org/mismatch":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, scopedReasoningMismatch)
		default:
			_, _ = io.WriteString(w, passthroughAnthropicReply)
		}
	})
	anthropicRule := passthroughBodyRule(`"messages","chat"`)
	rt := loadPassthroughRuntime(t, passthroughProvider("anthropic", "anthropic", upstream.URL, anthropicRule+`
  model "org/pin" {
    upstream_name = "pinned-claude"
    capabilities = ["messages"]
  }
  model_passthrough "overlap" {
    match = "^org/overlap$"
    capabilities = ["chat"]
  }
`), passthroughProvider("anthropic", "foreign", upstream.URL+"/foreign", anthropicRule),
		passthroughProvider("anthropic", "disabled", upstream.URL, "enabled = false\n"+anthropicRule),
		passthroughCopilotProvider(t, upstream.URL, passthroughBodyRule(`"chat"`)), `
encrypted_reasoning { on_caller_mismatch = "strip_and_retry" }
alias "pinned" {
  algorithm = "round_robin"
  providers = ["anthropic"]
  model = "org/pin"
}
auth "fixture" {
  mode = "bearer_static"
  client "all" {
    token = "all"
    allowed_model_passthroughs = ["anthropic/body-family","foreign/body-family","disabled/body-family","copilot/body-family"]
  }
  client "rule-only" {
    token = "rule-only"
    allowed_models = ["anthropic/other-literal"]
    allowed_model_passthroughs = ["anthropic/body-family"]
  }
  client "literal-only" {
    token = "literal-only"
    allowed_models = ["anthropic/org/new"]
  }
}
`)
	system, foreign := identity.InWorkspace(store.SystemWorkspaceID), identity.InWorkspace(uuid.New())
	providers, aliases := rt.Catalog.Providers(), rt.Catalog.Aliases()
	for i := range providers {
		providers[i].Workspace = system
		if providers[i].Name == "foreign" {
			providers[i].Workspace = foreign
		}
	}
	aliases[0].Workspace = system
	rt.Catalog = config.NewCatalog(providers, rt.Catalog.DisabledProviders(), aliases)
	key := auth.DynamicClient{TokenHash: store.TokenHash("db-key"), KeyID: uuid.New(), Name: "all", Workspace: system, AllowedModels: []string{"anthropic/org/new"}}
	deps := passthroughDeps(rt, upstream, key)
	deps.MultiTenancy.Enabled = true
	proxy := newPassthroughWireProxy(t, deps, upstream)

	for _, tc := range []struct {
		name, bearer, path, model, fields, code string
		status                                  int
	}{
		{"no-bearer", "", passthroughMessagesPath, "anthropic/org/new", "", "auth_failed", http.StatusUnauthorized},
		{"literal-allowlist-is-not-rule-grant", "literal-only", passthroughMessagesPath, "anthropic/org/new", "", "model_not_found", http.StatusNotFound},
		{"db-key", "db-key", passthroughMessagesPath, "anthropic/org/new", "", "model_not_found", http.StatusNotFound},
		{"foreign-workspace", "all", passthroughMessagesPath, "foreign/org/new", "", "model_not_found", http.StatusNotFound},
		{"disabled", "all", passthroughMessagesPath, "disabled/org/new", "", "model_not_found", http.StatusNotFound},
		{"unmatched", "all", passthroughMessagesPath, "anthropic/other/new", "", "model_not_found", http.StatusNotFound},
		{"overlap-even-without-second-grant-or-capability", "all", passthroughMessagesPath, "anthropic/org/overlap", "", "model_not_found", http.StatusNotFound},
		{"rule-capability-omitted", "all", passthroughResponsesPath, "anthropic/org/new", `,"input":"hi"`, "model_not_found", http.StatusNotFound},
		{"copilot-native-messages-excluded", "all", passthroughMessagesPath, "copilot/org/new", "", "model_not_found", http.StatusNotFound},
		{"copilot-responses-excluded", "all", passthroughResponsesPath, "copilot/org/new", `,"input":"hi"`, "model_not_found", http.StatusNotFound},
		{"denied-exact", "rule-only", passthroughMessagesPath, "anthropic/org/pin", "", "forbidden", http.StatusForbidden},
		{"unsupported-exact", "all", passthroughChatPath, "anthropic/org/pin", "", "unsupported_operation", http.StatusBadRequest},
		{"fallbacks", "all", passthroughMessagesPath, "anthropic/org/new", `,"fallbacks":["other/model"]`, "model_not_found", http.StatusNotFound},
		{"mixed-case-escaped-fallbacks", "rule-only", passthroughMessagesPath, "anthropic/org/new", `,"FaLl\u0062AcKs":null,"stream":true`, "forbidden", http.StatusForbidden},
		{"models", "all", passthroughMessagesPath, "anthropic/org/new", `,"mOdElS":["other/model"]`, "model_not_found", http.StatusNotFound},
		{"preset", "all", passthroughMessagesPath, "anthropic/org/new", `,"pre\u0053et":"other-model"`, "model_not_found", http.StatusNotFound},
		{"duplicate-model", "all", passthroughMessagesPath, "other/model", `,"model":"anthropic/org/new"`, "model_not_found", http.StatusNotFound},
		{"case-variant-model", "all", passthroughMessagesPath, "other/model", `,"mOdEl":"anthropic/org/new"`, "model_not_found", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := passthroughWireRequest(t, proxy, http.MethodPost, tc.path, tc.bearer,
				fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}]%s}`, tc.model, tc.fields))
			if resp.StatusCode != tc.status || guardrailErrorType(t, body) != tc.code || len(calls()) != 0 {
				t.Fatalf("response = %d %s; upstream calls = %+v", resp.StatusCode, body, calls())
			}
		})
	}

	for _, tc := range []struct {
		model, bearer, fields, upstreamModel, reply string
		status                                      int
	}{
		{"anthropic/org/pin", "all", passthroughNativeFields + `,"FaLlBaCkS":["keep"]`, "pinned-claude", missing, http.StatusNotFound},
		{"alias/pinned", "all", passthroughNativeFields + `,"FaLlBaCkS":["keep"]`, "pinned-claude", missing, http.StatusNotFound},
		{"anthropic/org/mismatch", "all", passthroughNativeFields, "org/mismatch", scopedReasoningMismatch, http.StatusBadRequest},
		{"anthropic/org/new", "rule-only", passthroughNativeFields, "org/new", passthroughAnthropicReply, http.StatusOK},
	} {
		t.Run(tc.model, func(t *testing.T) {
			before := len(calls())
			resp, body := passthroughWireRequest(t, proxy, http.MethodPost, passthroughMessagesPath, tc.bearer, fmt.Sprintf(`{"model":%q,%s}`, tc.model, tc.fields))
			got := calls()
			if resp.StatusCode != tc.status || body != tc.reply || len(got) != before+1 {
				t.Fatalf("response = %d %s; upstream calls = %+v", resp.StatusCode, body, got[before:])
			}
			assertPassthroughWireJSON(t, got[before].body, fmt.Sprintf(`{"model":%q,%s}`, tc.upstreamModel, tc.fields))
		})
	}
}

func TestModelPassthroughBodyAdapterGuardrailsHTTP(t *testing.T) {
	upstream, calls := newPassthroughWireMock(t, func(w http.ResponseWriter, call passthroughWireCall) {
		w.Header().Set("Content-Type", "application/json")
		if call.path == "/copilot/chat/completions" {
			_, _ = io.WriteString(w, passthroughCopilotReply)
			return
		}
		_, _ = io.WriteString(w, passthroughAnthropicReply)
	})
	rt := loadPassthroughRuntime(t, passthroughAnon,
		passthroughProvider("anthropic", "anthropic", upstream.URL, passthroughBodyRule(`"messages","chat","responses"`)),
		passthroughCopilotProvider(t, upstream.URL+"/copilot", passthroughBodyRule(`"chat"`)))
	deps := passthroughDeps(rt, upstream)
	deps.Guardrails = guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
	const replacement = "FaLlBaCkS"
	exceptions, err := guardrails.LoadExceptions("", replacement)
	if err != nil {
		t.Fatal(err)
	}
	deps.Exceptions = exceptions
	proxy := newPassthroughWireProxy(t, deps, upstream)
	key := guardrailTestKey(t)
	const suffix = "org/guardrail"
	requests := []struct{ path, model, fields string }{
		{passthroughMessagesPath, "anthropic/" + suffix, `"max_tokens":64,"messages":[{"role":"user","content":%q}]`},
		{passthroughChatPath, "anthropic/" + suffix, `"messages":[{"role":"user","content":%q}]`},
		{passthroughResponsesPath, "anthropic/" + suffix, `"input":%q`},
		{passthroughChatPath, "copilot/" + suffix, `"messages":[{"role":"user","content":%q}]`},
	}
	for _, tc := range requests {
		body := fmt.Sprintf(`{"model":%q,`+tc.fields+`}`, tc.model, key)
		resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, tc.path, "", body)
		if resp.StatusCode != http.StatusBadRequest || guardrailErrorType(t, reply) != guardrailBlockType || len(calls()) != 0 {
			t.Fatalf("%s %s: guardrail block = %d %s; calls = %+v", tc.model, tc.path, resp.StatusCode, reply, calls())
		}
	}
	if _, err := exceptions.Decide([]string{guardrails.Fingerprint(key)}, guardrails.ExceptionActionRedact, "fixture", nil); err != nil {
		t.Fatal(err)
	}
	for i, tc := range requests {
		body := fmt.Sprintf(`{"model":%q,`+tc.fields+`}`, tc.model, key)
		resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, tc.path, "", body)
		got := calls()
		if resp.StatusCode != http.StatusOK || len(got) != i+1 || extractModel([]byte(got[i].body)) != suffix ||
			strings.Contains(got[i].body, key) || !strings.Contains(got[i].body, replacement) {
			t.Fatalf("%s %s: guardrail rewrite = %d %s; calls = %+v", tc.model, tc.path, resp.StatusCode, reply, got)
		}
	}
	body := fmt.Sprintf(`{"model":"anthropic/%s","max_tokens":64,"messages":[{"role":"user","content":%q}],%q:["other/model"]}`, suffix, key, key)
	resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, passthroughMessagesPath, "", body)
	if resp.StatusCode != http.StatusBadRequest || guardrailErrorType(t, reply) != guardrailGapType || len(calls()) != len(requests) {
		t.Fatalf("guardrail rewrite introduced a top-level selector: %d %s; calls = %+v", resp.StatusCode, reply, calls())
	}
}
