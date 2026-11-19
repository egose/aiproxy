package provider

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestModelPassthroughGeminiAdapterPathBoundary(t *testing.T) {
	for _, adapter := range []struct {
		typ      config.ProviderType
		protocol config.ModelProtocol
		ops      []Operation
	}{
		{config.ProviderTypeGemini, "", []Operation{OpChatCompletions, OpResponses, OpEmbeddings}},
		{config.ProviderTypeOpenCodeZen, config.ModelProtocolGemini, []Operation{OpChatCompletions, OpResponses}},
	} {
		for _, op := range adapter.ops {
			t.Run(string(adapter.typ)+"/"+op.String(), func(t *testing.T) {
				const base = "https://fixture.example/prefix"
				for _, tc := range []struct{ id, base string }{
					{"models/gemini-next", base},
					{"gemini%2fnext", base},
					{"..", base},
					{"gemini-next:embedContent", base},
					{"gemini-next?alt=x", base},
					{strings.Repeat("a", config.MaxModelPassthroughSuffixBytes+1), base},
					{"gemini-next", base + "?"},
					{"gemini-next", base + "#"},
				} {
					res, err := New().Do(t.Context(), Request{
						Operation: op, ProviderType: adapter.typ, ModelProtocol: adapter.protocol,
						PublicModel: "fixture/gemini-next", UpstreamModel: tc.id, ModelPassthrough: true, BaseURL: tc.base,
						Body: []byte(`{"model":"fixture/gemini-next","messages":[]}`),
						Client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
							t.Errorf("unexpected transport call: %s", r.URL)
							return nil, errors.New("unexpected transport call")
						})},
					})
					var invalid ErrInvalidRequest
					want := "single bare model ID"
					if tc.base != base {
						want = "base_url must not include a query or fragment"
					}
					if res != nil || !errors.As(err, &invalid) || !strings.Contains(invalid.Error(), want) {
						t.Errorf("%s, model %q: result=%v, err=%v; want invalid request containing %q", tc.base, tc.id, res, err, want)
					}
				}
			})
		}
	}
}
