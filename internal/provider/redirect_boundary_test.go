package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/upstreamhttp"
)

func TestProviderAlternateClientRedirectIsolation(t *testing.T) {
	for _, kind := range []config.ProviderType{config.ProviderTypeAnthropic, config.ProviderTypeGemini, config.ProviderTypeOpenAI} {
		for _, injected := range []bool{false, true} {
			for _, status := range []int{302, 307, 308} {
				t.Run(fmt.Sprintf("%s/injected=%t/%d", kind, injected, status), func(t *testing.T) {
					var calls atomic.Int32
					destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
					defer destination.Close()
					origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, status) }))
					defer origin.Close()
					var client *http.Client
					if injected {
						client = origin.Client()
					}
					_, err := New().Do(context.Background(), Request{ProviderType: kind, BaseURL: origin.URL, APIKey: "synthetic", UpstreamModel: "test", Client: client, Inbound: httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil), Body: []byte(`{"messages":[{"role":"user","content":"synthetic prompt"}]}`)})
					if !errors.Is(err, upstreamhttp.ErrRedirectOrigin) || calls.Load() != 0 {
						t.Fatalf("calls=%d err=%v", calls.Load(), err)
					}
					if client != nil && client.CheckRedirect != nil {
						t.Fatal("injected client mutated")
					}
				})
			}
		}
	}
}
