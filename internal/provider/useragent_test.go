package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

const userAgentInboundBody = `{"model":"m","messages":[]}`

func userAgentUpstream(t *testing.T, gotUA *string, providerType config.ProviderType) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		switch providerType {
		case config.ProviderTypeAnthropic:
			_, _ = io.WriteString(w, `{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
		case config.ProviderTypeGemini:
			_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
}

func userAgentRequest(providerType config.ProviderType, upstream *httptest.Server, override, version string) Request {
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(userAgentInboundBody)))
	inbound.Header.Set("User-Agent", "caller/1.0 should-not-forward")
	r := Request{
		Operation:     OpChatCompletions,
		ProviderType:  providerType,
		PublicModel:   "p/m",
		BaseURL:       upstream.URL,
		UpstreamModel: "m",
		UserAgent:     override,
		Version:       version,
		Body:          []byte(userAgentInboundBody),
		Inbound:       inbound,
		Client:        upstream.Client(),
	}
	if providerType == config.ProviderTypeOpenCodeZen || providerType == config.ProviderTypeOpenCodeGo {
		r.ModelProtocol = config.ModelProtocolChat
	}
	if providerType == config.ProviderTypeGitHubCopilot {
		r.CopilotToken = "gho_tok"
	}
	if providerType != config.ProviderTypeOpenCodeZen {
		r.APIKey = "k"
	}
	return r
}

func TestUpstreamUserAgentAppliesToAllProviderTypes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		providerType config.ProviderType
	}{
		{"openai", config.ProviderTypeOpenAI},
		{"openai-compatible", config.ProviderTypeOpenAICompatible},
		{"anthropic", config.ProviderTypeAnthropic},
		{"gemini", config.ProviderTypeGemini},
		{"github-copilot", config.ProviderTypeGitHubCopilot},
		{"opencode-zen", config.ProviderTypeOpenCodeZen},
		{"opencode-go", config.ProviderTypeOpenCodeGo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, ua := range []struct {
				name     string
				override string
				want     string
			}{
				{"default", "", "aiproxy/1.2.3-test"},
				{"override", "custom/1.0", "custom/1.0"},
			} {
				t.Run(ua.name, func(t *testing.T) {
					var gotUA string
					upstream := userAgentUpstream(t, &gotUA, tc.providerType)
					defer upstream.Close()
					if _, err := New().Do(context.Background(), userAgentRequest(tc.providerType, upstream, ua.override, "1.2.3-test")); err != nil {
						t.Fatalf("do: %v", err)
					}
					if gotUA != ua.want {
						t.Fatalf("User-Agent = %q, want %q", gotUA, ua.want)
					}
				})
			}
		})
	}
}

func TestUpstreamUserAgentDefaultsToDevVersion(t *testing.T) {
	var gotUA string
	upstream := userAgentUpstream(t, &gotUA, config.ProviderTypeOpenAI)
	defer upstream.Close()
	if _, err := New().Do(context.Background(), userAgentRequest(config.ProviderTypeOpenAI, upstream, "", "")); err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotUA != "aiproxy/dev" {
		t.Fatalf("User-Agent = %q, want aiproxy/dev", gotUA)
	}
}

func TestUpstreamUserAgentForwarding(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override string
		forward  bool
		inbound  string
		want     string
	}{
		{"forwarded", "", true, "caller/9.9", "caller/9.9"},
		{"override wins over forwarded", "custom/1.0", true, "caller/9.9", "custom/1.0"},
		{"flag off ignores inbound", "", false, "caller/9.9", "aiproxy/1.2.3-test"},
		{"empty inbound falls back", "", true, "", "aiproxy/1.2.3-test"},
		{"newline inbound falls back", "", true, "bad\nua", "aiproxy/1.2.3-test"},
		{"non-ascii inbound falls back", "", true, "bad-\u00e9-ua", "aiproxy/1.2.3-test"},
		{"oversize inbound falls back", "", true, strings.Repeat("a", 257), "aiproxy/1.2.3-test"},
		{"max size inbound forwarded", "", true, strings.Repeat("a", 256), strings.Repeat("a", 256)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, providerType := range []config.ProviderType{config.ProviderTypeOpenAI, config.ProviderTypeAnthropic} {
				var gotUA string
				upstream := userAgentUpstream(t, &gotUA, providerType)
				defer upstream.Close()
				r := userAgentRequest(providerType, upstream, tc.override, "1.2.3-test")
				r.ForwardUserAgent = tc.forward
				r.Inbound.Header.Set("User-Agent", tc.inbound)
				if _, err := New().Do(context.Background(), r); err != nil {
					t.Fatalf("%s do: %v", providerType, err)
				}
				if gotUA != tc.want {
					t.Fatalf("%s User-Agent = %q, want %q", providerType, gotUA, tc.want)
				}
			}
		})
	}
}
