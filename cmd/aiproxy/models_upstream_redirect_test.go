package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/upstreamhttp"
)

func TestListUpstreamModelsRedirectIsolation(t *testing.T) {
	for _, provider := range []struct {
		kind                         config.ProviderType
		header, credential, response string
	}{
		{config.ProviderTypeAnthropic, "x-api-key", "synthetic-key", `{"data":[{"id":"test"}]}`},
		{config.ProviderTypeGemini, "x-goog-api-key", "synthetic-key", `{"models":[{"name":"models/test"}]}`},
		{config.ProviderTypeOpenAI, "Authorization", "Bearer synthetic-key", `{"data":[{"id":"test"}]}`},
		{config.ProviderTypeGitHubCopilot, "Authorization", "Bearer synthetic-key", `{"data":[{"id":"test"}]}`},
	} {
		for _, status := range []int{302, 307, 308} {
			for _, sameOrigin := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/same-origin=%t", provider.kind, status, sameOrigin), func(t *testing.T) {
					var calls atomic.Int32
					finish := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if !sameOrigin {
							t.Errorf("prohibited destination reached: credential=%t", r.Header.Get(provider.header) == provider.credential)
						} else if r.Header.Get(provider.header) != provider.credential {
							t.Error("same-origin redirect lost credential")
						}
						_, _ = io.WriteString(w, provider.response)
					})
					destination := httptest.NewServer(finish)
					defer destination.Close()
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/redirected" {
							finish.ServeHTTP(w, r)
							return
						}
						target := destination.URL
						if sameOrigin {
							target = "/redirected"
						}
						http.Redirect(w, r, target, status)
					}))
					defer upstream.Close()
					models, err := listUpstreamModels(context.Background(), config.Provider{Type: provider.kind, BaseURL: upstream.URL, APIKey: "synthetic-key", CopilotToken: "synthetic-key"})
					if sameOrigin {
						if err != nil || calls.Load() != 1 || len(models) != 1 || models[0].ID != "test" {
							t.Fatalf("allowed redirect: calls=%d models=%v error=%v", calls.Load(), models, err)
						}
					} else if err == nil || !strings.Contains(err.Error(), "redirect") || calls.Load() != 0 || len(models) != 0 {
						t.Fatalf("blocked redirect: calls=%d models=%v error=%v", calls.Load(), models, err)
					}
				})
			}
		}
	}
}

func TestListUpstreamModelsRedirectLoopFailure(t *testing.T) {
	for _, kind := range []config.ProviderType{config.ProviderTypeAnthropic, config.ProviderTypeGemini} {
		t.Run(string(kind), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Redirect(w, r, "/loop", http.StatusTemporaryRedirect)
			}))
			defer upstream.Close()
			models, err := listUpstreamModels(context.Background(), config.Provider{Type: kind, BaseURL: upstream.URL, APIKey: "synthetic-key"})
			if !errors.Is(err, upstreamhttp.ErrRedirectLimit) || len(models) != 0 || calls.Load() != upstreamhttp.MaxRedirects+1 {
				t.Fatalf("redirect loop: calls=%d models=%v error=%v", calls.Load(), models, err)
			}
		})
	}
}
