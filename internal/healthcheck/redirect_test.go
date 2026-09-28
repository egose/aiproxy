package healthcheck

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

func TestProbeRedirectIsolation(t *testing.T) {
	for _, status := range []int{302, 307, 308} {
		for _, same := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/same=%t", status, same), func(t *testing.T) {
				var calls atomic.Int32
				finish := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Header.Get("Authorization") != "Bearer synthetic" {
						t.Error("credential missing")
					}
				})
				destination := httptest.NewServer(finish)
				defer destination.Close()
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/finish" {
						finish.ServeHTTP(w, r)
						return
					}
					target := destination.URL
					if same {
						target = "/finish"
					}
					http.Redirect(w, r, target, status)
				}))
				defer origin.Close()
				m := New(nil, nil, "test")
				code, _, err := m.probe(context.Background(), config.Provider{BaseURL: origin.URL, APIKey: "synthetic", Healthcheck: &config.ProviderHealthcheck{Path: "/start", SendAuthorization: true}})
				if same {
					if err != nil || code != 200 || calls.Load() != 1 {
						t.Fatalf("code=%d calls=%d err=%v", code, calls.Load(), err)
					}
				} else if !errors.Is(err, upstreamhttp.ErrRedirectOrigin) || calls.Load() != 0 {
					t.Fatalf("calls=%d err=%v", calls.Load(), err)
				}
			})
		}
	}
}
