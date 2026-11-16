package upstreamhttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestControlContextForbidsEveryRedirectAndCookieJarWithoutMutatingClient(t *testing.T) {
	for _, status := range []int{302, 307, 308} {
		for _, crossOrigin := range []bool{false, true} {
			t.Run(http.StatusText(status)+"/cross="+map[bool]string{false: "false", true: "true"}[crossOrigin], func(t *testing.T) {
				var escaped atomic.Int32
				destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { escaped.Add(1) }))
				defer destination.Close()
				peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/control" {
						escaped.Add(1)
						return
					}
					if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "Bearer pinned" {
						t.Error("control received cookie jar state or lost its explicit credential")
					}
					target := "/redirected"
					if crossOrigin {
						target = destination.URL
					}
					http.Redirect(w, r, target, status)
				}))
				defer peer.Close()
				jar, _ := cookiejar.New(nil)
				u, _ := url.Parse(peer.URL)
				jar.SetCookies(u, []*http.Cookie{{Name: "private", Value: "jar-secret"}})
				client := peer.Client()
				client.Jar, client.Timeout = jar, time.Second
				policyCalls := 0
				client.CheckRedirect = func(*http.Request, []*http.Request) error {
					policyCalls++
					return nil
				}
				transport := client.Transport
				req, _ := http.NewRequestWithContext(ControlContext(context.Background()), "POST", peer.URL+"/control", nil)
				req.Header.Set("Authorization", "Bearer pinned")
				response, err := Do(client, req)
				if response != nil {
					_ = response.Body.Close()
				}
				if !errors.Is(err, ErrControlRedirect) || escaped.Load() != 0 || policyCalls != 0 ||
					client.Jar != jar || client.Transport != transport || client.Timeout != time.Second {
					t.Fatalf("control policy mutated shared client or leaked through redirect: err=%v escapes=%d policy=%d", err, escaped.Load(), policyCalls)
				}
			})
		}
	}
}

func TestControlContextTotalDeadlineIncludesResponseBody(t *testing.T) {
	canceled := make(chan struct{})
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.aiproxy.deferred+json")
		_, _ = io.WriteString(w, "{")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer peer.Close()
	ctx, cancel := context.WithTimeout(ControlContext(context.Background()), 100*time.Millisecond)
	defer cancel()
	client := peer.Client()
	req, _ := http.NewRequestWithContext(ctx, "GET", peer.URL, nil)
	started := time.Now()
	response, err := Do(client, req)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatalf("response body outlived control deadline: %v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("control deadline did not close peer response I/O")
	}
}
