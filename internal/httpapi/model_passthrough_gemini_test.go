package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

const geminiPassthroughReply = `{"candidates":[{"content":{"role":"model","parts":[{"text":"fixture-answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":6,"totalTokenCount":10}}`

func geminiPassthroughRule(pattern, capabilities string) string {
	return fmt.Sprintf(`
  model_passthrough "path-family" {
    match = %q
    capabilities = [%s]
  }
`, pattern, capabilities)
}

func TestModelPassthroughGeminiGenerationHTTP(t *testing.T) {
	const suffix = "gemini-next_2.0"
	const translated = `{"contents":[{"role":"user","parts":[{"text":"fixture-input"}]}],"generationConfig":{"maxOutputTokens":64}}`
	for _, service := range []struct {
		typ, name, prefix, modelPath, rule string
	}{
		{"gemini", "google", "/vendor/gem", "/v1beta/models/", geminiPassthroughRule(".*", `"chat","responses"`)},
		{"opencode-zen", "zen", "/vendor/zen/v1", "/models/", passthroughOpenCodeRule("path-family", ".*", config.ModelProtocolGemini, `"chat","responses"`)},
	} {
		for _, op := range []struct {
			name, path, fields, jsonObject, streamMarker string
		}{
			{"chat", passthroughChatPath, `"max_tokens":64,"messages":[{"role":"user","content":"fixture-input"}]`, "chat.completion", `"object":"chat.completion.chunk"`},
			{"responses", passthroughResponsesPath, `"max_output_tokens":64,"input":"fixture-input"`, "response", `"type":"response.completed"`},
		} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", service.name, op.name, stream), func(t *testing.T) {
					contentType, mockReply, accept := "application/json", geminiPassthroughReply, ""
					action, query, streamField := ":generateContent", "", ""
					if stream {
						contentType, mockReply, accept = "text/event-stream", "data: "+geminiPassthroughReply+"\n\n", "text/event-stream"
						action, query, streamField = ":streamGenerateContent", "alt=sse", `,"stream":true`
					}
					upstream, calls := newPassthroughWireMock(t, func(w http.ResponseWriter, _ passthroughWireCall) {
						w.Header().Set("Content-Type", contentType)
						_, _ = io.WriteString(w, mockReply)
					})
					rt := loadPassthroughRuntime(t, passthroughAnon,
						passthroughProvider(service.typ, service.name, upstream.URL+service.prefix+"/", service.rule))
					proxy := newPassthroughWireProxy(t, passthroughDeps(rt, upstream), upstream)
					public := service.name + "/" + suffix
					resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, op.path+"?alt=caller&key=ignored", "",
						fmt.Sprintf(`{"model":%q,%s%s}`, public, op.fields, streamField))
					got := calls()
					if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != contentType || len(got) != 1 {
						t.Fatalf("response = %d %s; upstream calls = %+v", resp.StatusCode, reply, got)
					}
					call := got[0]
					wantURI := service.prefix + service.modelPath + suffix + action
					if query != "" {
						wantURI += "?" + query
					}
					if call.method != http.MethodPost || call.host != strings.TrimPrefix(upstream.URL, "http://") ||
						call.requestURI != wantURI || call.query != query || call.header.Get("Accept") != accept ||
						call.header.Get("Content-Type") != "application/json" || call.header.Get("Cookie") != "" {
						t.Fatalf("unexpected upstream request, want %s: %+v", wantURI, call)
					}
					if service.name == "google" {
						if call.header.Get("X-Goog-Api-Key") != "fixture-upstream" || call.header.Get("Authorization") != "" {
							t.Fatalf("Gemini credential headers = %v", call.header)
						}
					} else if call.header.Get("Authorization") != "Bearer fixture-upstream" || call.header.Get("X-Goog-Api-Key") != "" ||
						!strings.HasPrefix(call.header.Get("X-OpenCode-Session"), "ses_") {
						t.Fatalf("Zen credential and session headers = %v", call.header)
					}
					assertPassthroughWireJSON(t, call.body, translated)
					if !strings.Contains(reply, "fixture-answer") || strings.Contains(reply, "@passthrough") {
						t.Fatalf("unexpected translated output: %s", reply)
					}
					if stream {
						if !strings.Contains(reply, `"model":"`+public+`"`) || !strings.Contains(reply, op.streamMarker) ||
							!strings.HasSuffix(reply, "data: [DONE]\n\n") {
							t.Fatalf("unexpected translated stream: %s", reply)
						}
					} else if extractModel([]byte(reply)) != public || !strings.Contains(reply, `"object":"`+op.jsonObject+`"`) {
						t.Fatalf("unexpected translated JSON: %s", reply)
					}
				})
			}
		}
	}
}

func TestModelPassthroughGeminiEmbeddingsHTTP(t *testing.T) {
	const (
		route      = "/v1/embeddings"
		basePath   = "/vendor/gem/"
		modelsPath = basePath + "v1beta/models/"
		id         = "text-embedding_next.2"
		pinnedID   = "configured/stable.embed"
		single     = `{"content":{"parts":[{"text":"first"}]},"outputDimensionality":2}`
		ruleBatch  = `{"requests":[{"model":"models/` + id + `","content":{"parts":[{"text":"first"}]},"outputDimensionality":2},{"model":"models/` + id + `","content":{"parts":[{"text":"second"}]},"outputDimensionality":2}]}`
		oldBatch   = `{"requests":[{"content":{"parts":[{"text":"first"}]},"outputDimensionality":2},{"content":{"parts":[{"text":"second"}]},"outputDimensionality":2}]}`
	)
	for _, tc := range []struct {
		name, public, input, wireModel, action, wireBody string
		items                                            int
	}{
		{"single", "google/" + id, `"first"`, id, ":embedContent", single, 1},
		{"rule-batch", "google/" + id, `["first","second"]`, id, ":batchEmbedContents", ruleBatch, 2},
		{"literal-batch", "google/pinned", `["first","second"]`, pinnedID, ":batchEmbedContents", oldBatch, 2},
		{"alias-batch", "alias/pinned", `["first","second"]`, pinnedID, ":batchEmbedContents", oldBatch, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantURI := modelsPath + tc.wireModel + tc.action
			upstream, calls := newPassthroughWireMock(t, func(w http.ResponseWriter, call passthroughWireCall) {
				if call.requestURI != wantURI || call.method != http.MethodPost || call.header.Get("X-Goog-Api-Key") != "fixture-upstream" {
					http.Error(w, "unexpected fixture request target or credentials", http.StatusBadRequest)
					return
				}
				if tc.name == "rule-batch" {
					var batch struct {
						Requests []struct {
							Model string `json:"model"`
						} `json:"requests"`
					}
					if err := json.Unmarshal([]byte(call.body), &batch); err != nil || len(batch.Requests) != tc.items {
						http.Error(w, "expected BatchEmbedContentsRequest.requests", http.StatusBadRequest)
						return
					}
					pathID := strings.TrimSuffix(strings.TrimPrefix(call.path, modelsPath), tc.action)
					for _, item := range batch.Requests {
						if item.Model != "models/"+pathID {
							http.Error(w, "each EmbedContentRequest.model must match the path resource", http.StatusBadRequest)
							return
						}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.items == 1 {
					_, _ = io.WriteString(w, `{"embedding":{"values":[0.1,0.2]}}`)
				} else {
					_, _ = io.WriteString(w, `{"embeddings":[{"values":[0.1,0.2]},{"values":[0.3,0.4]}]}`)
				}
			})
			settings := geminiPassthroughRule(".*", `"embeddings"`) + `
  model "pinned" {
    upstream_name = "` + pinnedID + `"
    capabilities = ["embeddings"]
  }
`
			rt := loadPassthroughRuntime(t, passthroughAnon,
				passthroughProvider("gemini", "google", upstream.URL+basePath, settings), `
alias "pinned" {
  algorithm = "round_robin"
  providers = ["google"]
  model = "pinned"
}
`)
			proxy := newPassthroughWireProxy(t, passthroughDeps(rt, upstream), upstream)
			resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, route+"?alt=caller", "",
				fmt.Sprintf(`{"model":%q,"input":%s,"dimensions":2}`, tc.public, tc.input))
			got := calls()
			var out struct {
				Object, Model string
				Data          []struct {
					Index     int
					Embedding []float64
				}
			}
			if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(reply), &out) != nil ||
				out.Object != "list" || out.Model != tc.public || len(out.Data) != tc.items || len(got) != 1 {
				t.Fatalf("response = %d %s; upstream calls = %+v", resp.StatusCode, reply, got)
			}
			for i, item := range out.Data {
				if item.Index != i || len(item.Embedding) != 2 {
					t.Fatalf("embedding item %d = %+v", i, item)
				}
			}
			assertPassthroughWireJSON(t, got[0].body, tc.wireBody)
			if tc.name == "rule-batch" {
				for _, fields := range []string{
					`"input":["first","second"],"requests":[{"model":"models/other"}]`,
					`"input":[{"model":"models/other"},{"model":"models/other"}]`,
				} {
					resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, route, "", fmt.Sprintf(`{"model":%q,%s}`, tc.public, fields))
					if resp.StatusCode != http.StatusBadRequest || guardrailErrorType(t, reply) != "invalid_request" || len(calls()) != 1 {
						t.Fatalf("caller batch subrequests were not rejected locally: %d %s; calls = %+v", resp.StatusCode, reply, calls())
					}
				}
			}
		})
	}
}

func TestModelPassthroughGeminiRejectsNonBareIDsHTTP(t *testing.T) {
	upstream, calls := newPassthroughWireMock(t, func(w http.ResponseWriter, _ passthroughWireCall) {
		w.WriteHeader(http.StatusBadRequest)
	})
	googleRule := geminiPassthroughRule(".*", `"chat","responses","embeddings"`)
	zenRule := passthroughOpenCodeRule("path-family", ".*", config.ModelProtocolGemini, `"chat","responses"`)
	rt := loadPassthroughRuntime(t, `
auth "fixture" {
  mode = "bearer_static"
  client "unfiltered" {
    token = "unfiltered"
    allowed_model_passthroughs = ["google/path-family", "zen/path-family", "disabled/path-family", "zen-disabled/path-family"]
  }
  client "filtered" {
    token = "filtered"
    allowed_models = ["google/another"]
    allowed_model_passthroughs = ["google/path-family", "zen/path-family"]
  }
}
`, passthroughProvider("gemini", "google", upstream.URL, googleRule),
		passthroughProvider("opencode-zen", "zen", upstream.URL, zenRule),
		passthroughProvider("gemini", "disabled", upstream.URL, "enabled = false\n"+googleRule),
		passthroughProvider("opencode-zen", "zen-disabled", upstream.URL, "enabled = false\n"+zenRule))
	proxy := newPassthroughWireProxy(t, passthroughDeps(rt, upstream), upstream)
	deny := func(path, model, fields, bearer string, status int, code string) {
		t.Helper()
		resp, reply := passthroughWireRequest(t, proxy, http.MethodPost, path, bearer, fmt.Sprintf(`{"model":%q,%s}`, model, fields))
		if resp.StatusCode != status || guardrailErrorType(t, reply) != code || len(calls()) != 0 {
			t.Fatalf("%q %s (%s): response = %d %s; upstream calls = %+v", model, path, bearer, resp.StatusCode, reply, calls())
		}
	}
	for _, name := range []string{"google", "zen"} {
		for _, suffix := range []string{
			"", "org/gemini-next", "models/gemini-next", "projects/p/locations/l/publishers/google/models/gemini-next",
			"/gemini-next", "gemini-next/", "org//gemini-next", ".", "..", "../gemini-next", "org/../gemini-next",
			"gemini%2fnext", "gemini%252fnext", "gemini%5cnext", "%2egemini-next", `gemini\next`,
			"gemini-next:embedContent", "gemini-next?alt=sse", "gemini-next#frag", "gemini-next?", "gemini-next#",
			" gemini-next", "gemini-next ", "Gemini-next", "_gemini-next", "gemini-néxt",
			strings.Repeat("x", config.MaxModelPassthroughSuffixBytes+1),
		} {
			deny(passthroughChatPath, name+"/"+suffix, `"messages":[]`, "unfiltered", http.StatusNotFound, "model_not_found")
			deny(passthroughChatPath, name+"/"+suffix, `"messages":[]`, "filtered", http.StatusForbidden, "forbidden")
		}
	}
	for _, tc := range []struct{ path, model, fields string }{
		{passthroughChatPath, "disabled/gemini-next", `"messages":[]`},
		{passthroughChatPath, "zen-disabled/gemini-next", `"messages":[]`},
		{passthroughMessagesPath, "google/gemini-next", `"messages":[]`},
		{passthroughMessagesPath, "zen/gemini-next", `"messages":[]`},
		{"/v1/embeddings", "zen/gemini-next", `"input":"fixture"`},
		{"/v1/images/generations", "google/gemini-next", `"prompt":"fixture"`},
	} {
		deny(tc.path, tc.model, tc.fields, "unfiltered", http.StatusNotFound, "model_not_found")
	}
}

func TestModelPassthroughGeminiMixedZenHTTP(t *testing.T) {
	const prefix = "/vendor/zen/v1"
	upstream, calls := newPassthroughWireMock(t, func(w http.ResponseWriter, call passthroughWireCall) {
		w.Header().Set("Content-Type", "application/json")
		switch call.requestURI {
		case prefix + "/chat/completions":
			_, _ = io.WriteString(w, passthroughOpenCodeChatReply)
		case prefix + "/models/configured/pin:generateContent":
			_, _ = io.WriteString(w, geminiPassthroughReply)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	rt := loadPassthroughRuntime(t, `
auth "fixture" {
  mode = "bearer_static"
  client "body" {
    token = "body"
    allowed_model_passthroughs = ["zen/body-family"]
  }
  client "restricted" {
    token = "restricted"
    allowed_models = ["zen/another"]
    allowed_model_passthroughs = ["zen/body-family", "zen/path-family"]
  }
}
`, passthroughProvider("opencode-zen", "zen", upstream.URL+prefix, `
  model "org/pin" {
    protocol = "gemini"
    upstream_name = "configured/pin"
  }
`+passthroughOpenCodeRule("body-family", "org/(both|body|pin)", config.ModelProtocolChat, `"chat"`)+
		passthroughOpenCodeRule("path-family", "org/(both|pin)|models/.+", config.ModelProtocolGemini, `"responses"`)))
	proxy := newPassthroughWireProxy(t, passthroughDeps(rt, upstream), upstream)
	post := func(model, bearer string) (*http.Response, string) {
		t.Helper()
		return passthroughWireRequest(t, proxy, http.MethodPost, passthroughChatPath, bearer,
			fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"fixture-input"}]}`, model))
	}
	for _, tc := range []struct {
		model, bearer, code string
		status              int
	}{
		{"zen/org/both", "body", "model_not_found", http.StatusNotFound},
		{"zen/org/both", "restricted", "forbidden", http.StatusForbidden},
		{"zen/org/pin", "restricted", "forbidden", http.StatusForbidden},
	} {
		resp, reply := post(tc.model, tc.bearer)
		if resp.StatusCode != tc.status || guardrailErrorType(t, reply) != tc.code || len(calls()) != 0 {
			t.Fatalf("%q (%s): response = %d %s; upstream calls = %+v", tc.model, tc.bearer, resp.StatusCode, reply, calls())
		}
	}
	resp, reply := post("zen/org/body", "body")
	got := calls()
	if resp.StatusCode != http.StatusOK || reply != passthroughOpenCodeChatReply || len(got) != 1 ||
		got[0].requestURI != prefix+"/chat/completions" || extractModel([]byte(got[0].body)) != "org/body" {
		t.Fatalf("body rule lost the slash-separated ID: response = %d %s; upstream calls = %+v", resp.StatusCode, reply, got)
	}
	resp, reply = post("zen/org/pin", "body")
	got = calls()
	if resp.StatusCode != http.StatusOK || extractModel([]byte(reply)) != "zen/org/pin" || len(got) != 2 ||
		got[1].requestURI != prefix+"/models/configured/pin:generateContent" {
		t.Fatalf("literal did not keep its own protocol and upstream path: response = %d %s; upstream calls = %+v", resp.StatusCode, reply, got)
	}
}
