package upstreamhttp

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRedirectOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, from, to string
		allowed        bool
	}{
		{"path", "https://api.example/v1", "https://api.example/v2?next=1", true},
		{"case-and-default-port", "https://API.example/v1", "https://api.EXAMPLE:443/v2", true},
		{"http-default-port", "http://api.example:80", "http://api.example/next", true},
		{"ipv6", "https://[::1]:443/", "https://[::1]/next", true},
		{"host", "https://api.example", "https://other.example", false},
		{"subdomain", "https://api.example", "https://sub.api.example", false},
		{"port", "https://api.example", "https://api.example:444", false},
		{"downgrade", "https://api.example", "http://api.example", false},
		{"downgrade-same-port", "https://api.example:443", "http://api.example:443", false},
		{"upgrade", "http://api.example", "https://api.example", false},
		{"upgrade-same-port", "http://api.example:80", "https://api.example:80", false},
		{"unsupported-scheme", "https://api.example", "ftp://api.example", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, err := http.NewRequest(http.MethodGet, tc.from, nil)
			if err != nil {
				t.Fatal(err)
			}
			to, err := http.NewRequest(http.MethodGet, tc.to, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = CheckRedirect(to, []*http.Request{from})
			if tc.allowed && err != nil || !tc.allowed && !errors.Is(err, ErrRedirectOrigin) {
				t.Fatalf("allowed=%t error=%v", tc.allowed, err)
			}
		})
	}
}

func TestDoPreservesClientAndStricterPolicy(t *testing.T) {
	denied := errors.New("custom redirect denial")
	var calls atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Redirect(w, r, "/next", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	client := origin.Client()
	client.Timeout = time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return denied }
	transport := client.Transport
	req, _ := http.NewRequest(http.MethodGet, origin.URL, nil)
	resp, err := Do(client, req)
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, denied) || calls.Load() != 1 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
	if client.Transport != transport || client.Timeout != time.Second || !errors.Is(client.CheckRedirect(nil, nil), denied) {
		t.Fatal("client changed")
	}
}

func TestRedirectChecksOriginalOrigin(t *testing.T) {
	first, _ := http.NewRequest(http.MethodGet, "https://original.example/", nil)
	previous, _ := http.NewRequest(http.MethodGet, "https://other.example/", nil)
	next, _ := http.NewRequest(http.MethodGet, "https://other.example/next", nil)
	if err := CheckRedirect(next, []*http.Request{first, previous}); !errors.Is(err, ErrRedirectOrigin) {
		t.Fatalf("err=%v", err)
	}
}

func TestRedirectBlocksBeforeDestinationIO(t *testing.T) {
	for _, status := range []int{302, 307, 308} {
		for _, tls := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/downgrade=%t", status, tls), func(t *testing.T) {
				var calls atomic.Int32
				destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
				}))
				defer destination.Close()
				handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/start" {
						http.Redirect(w, r, "/next", status)
						return
					}
					http.Redirect(w, r, destination.URL, status)
				})
				var upstream *httptest.Server
				if tls {
					upstream = httptest.NewTLSServer(handler)
				} else {
					upstream = httptest.NewServer(handler)
				}
				defer upstream.Close()
				client := upstream.Client()
				client.CheckRedirect = CheckRedirect
				req, err := http.NewRequest(http.MethodPost, upstream.URL+"/start", strings.NewReader("synthetic-prompt"))
				if err != nil {
					t.Fatal(err)
				}
				for _, header := range []string{"x-api-key", "x-goog-api-key", "Authorization", "X-Custom-Secret"} {
					req.Header.Set(header, "synthetic-secret")
				}
				resp, err := client.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
				if !errors.Is(err, ErrRedirectOrigin) || calls.Load() != 0 {
					t.Fatalf("destination calls=%d error=%v", calls.Load(), err)
				}
			})
		}
	}
}

func TestRedirectHopBudget(t *testing.T) {
	for _, redirects := range []int{MaxRedirects, MaxRedirects + 1, -1} {
		t.Run(fmt.Sprint(redirects), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if redirects < 0 || int(n) <= redirects {
					http.Redirect(w, r, "/next", http.StatusTemporaryRedirect)
					return
				}
				_, _ = io.WriteString(w, "done")
			}))
			defer upstream.Close()
			client := &http.Client{CheckRedirect: CheckRedirect}
			resp, err := client.Get(upstream.URL)
			if resp != nil {
				resp.Body.Close()
			}
			if redirects == MaxRedirects {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrRedirectLimit) {
				t.Fatalf("error=%v, want redirect limit", err)
			}
			if calls.Load() != MaxRedirects+1 {
				t.Fatalf("requests=%d, want %d", calls.Load(), MaxRedirects+1)
			}
		})
	}
}
