package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/upstreamhttp"
)

func TestForwardAuthorization(t *testing.T) {
	for _, kind := range []config.ProviderType{config.ProviderTypeOpenAI, config.ProviderTypeOpenAICompatible, config.ProviderTypeOpenCodeZen, config.ProviderTypeOpenCodeGo, config.ProviderTypeZenMux, config.ProviderTypeOpenRouter} {
		for _, op := range []Operation{OpChatCompletions, OpResponses} {
			t.Run(string(kind)+"/"+op.String(), func(t *testing.T) {
				var got http.Header
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					got = r.Header.Clone()
					_, _ = w.Write([]byte(`{}`))
				}))
				defer upstream.Close()
				inbound := httptest.NewRequest(http.MethodPost, "/", nil)
				inbound.Header.Set("Authorization", "Custom client-credential")
				inbound.Header.Set("Cookie", "secret=cookie")
				inbound.Header.Set("x-api-key", "client-key")
				protocol := config.ModelProtocolChat
				if op == OpResponses {
					protocol = config.ModelProtocolResponses
				}
				res, err := New().Do(context.Background(), Request{
					ProviderType: kind, Operation: op, BaseURL: upstream.URL,
					UpstreamModel: "m", ModelProtocol: protocol, ForwardAuthorization: true,
					APIKey: "must-not-win", Inbound: inbound,
					Body: []byte(`{"messages":[{"role":"user","content":"hi"}],"input":"hi"}`),
				})
				if err != nil {
					t.Fatal(err)
				}
				if res.StreamBody != nil {
					_ = res.StreamBody.Close()
				}
				if got.Get("Authorization") != "Custom client-credential" || got.Get("Cookie") != "" || got.Get("x-api-key") != "" {
					t.Fatal("unexpected forwarded credentials")
				}
			})
		}
	}
}

func TestForwardAuthorizationRejectsInvalidHeadersWithoutIO(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	for _, values := range [][]string{nil, {""}, {"   "}, {"Bearer a", "Bearer b"}, {"Bearer bad\r\nvalue"}, {strings.Repeat("a", maxForwardedHeaderValueBytes+1)}} {
		inbound := httptest.NewRequest(http.MethodPost, "/", nil)
		inbound.Header["Authorization"] = values
		_, err := New().Do(context.Background(), Request{
			ProviderType: config.ProviderTypeOpenAI, BaseURL: upstream.URL,
			UpstreamModel: "m", ForwardAuthorization: true, APIKey: "no-fallback",
			Inbound: inbound, Body: []byte(`{"messages":[]}`),
		})
		var invalid ErrInvalidRequest
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v, want invalid request", err)
		}
	}
	_, err := New().Do(context.Background(), Request{ForwardAuthorization: true, BaseURL: upstream.URL})
	var invalid ErrInvalidRequest
	if !errors.As(err, &invalid) || calls.Load() != 0 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}

func TestForwardAuthorizationDoesNotFollowCrossOriginRedirect(t *testing.T) {
	var calls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	inbound := httptest.NewRequest(http.MethodPost, "/", nil)
	inbound.Header.Set("Authorization", "Bearer client-secret")
	_, err := New().Do(context.Background(), Request{
		ProviderType: config.ProviderTypeOpenAI, BaseURL: origin.URL,
		UpstreamModel: "m", ForwardAuthorization: true, Inbound: inbound,
		Body: []byte(`{"messages":[]}`),
	})
	if !errors.Is(err, upstreamhttp.ErrRedirectOrigin) || calls.Load() != 0 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}
