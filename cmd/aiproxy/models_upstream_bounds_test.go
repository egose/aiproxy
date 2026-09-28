package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

func TestUpstreamDiscoveryRegression(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  config.ProviderType
		body  string
		want  string
		calls int32
	}{
		{"repeated_cursor", config.ProviderTypeGemini, `{"models":[{"name":"models/m"}],"nextPageToken":"same"}`, "cursor cycle", 2},
		{"missing_cursor", config.ProviderTypeAnthropic, `{"data":[{"id":"m"}],"has_more":true}`, "continuation", 1},
		{"oversized_valid_prefix", config.ProviderTypeOpenAI, `{"data":[{"id":"m"}]}` + strings.Repeat(" ", 8<<20), "page byte limit", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) > 3 {
					http.Error(w, "fixture stopped old unbounded traversal", http.StatusTeapot)
					return
				}
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			models, err := listUpstreamModels(context.Background(), config.Provider{Type: tc.kind, BaseURL: srv.URL, APIKey: "synthetic"})
			if err == nil || !strings.Contains(err.Error(), tc.want) || models != nil || calls.Load() != tc.calls {
				t.Fatalf("models=%v err=%v calls=%d; want nil models, %q, %d calls", models, err, calls.Load(), tc.want, tc.calls)
			}
		})
	}
}

func discoveryPage(kind config.ProviderType, count int, next string) string {
	entries := make([]map[string]string, count)
	for i := range entries {
		id := fmt.Sprintf("model-%d", i)
		if i == count-1 && next != "" {
			id = next
		}
		if kind == config.ProviderTypeGemini {
			entries[i] = map[string]string{"name": "models/" + id, "displayName": "Display " + id}
		} else {
			entries[i] = map[string]string{"id": id, "display_name": "Display " + id}
		}
	}
	page := map[string]any{"data": entries}
	if kind == config.ProviderTypeAnthropic {
		page["has_more"] = next != ""
		page["last_id"] = next
	} else if kind == config.ProviderTypeGemini {
		page = map[string]any{"models": entries, "nextPageToken": next}
	}
	body, err := json.Marshal(page)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func discoveryProvider(kind config.ProviderType, base string) config.Provider {
	return config.Provider{Type: kind, Name: "test", BaseURL: base, APIKey: "synthetic", CopilotToken: "synthetic", UserAgent: "discovery-test/1"}
}

func TestUpstreamDiscoveryPagination(t *testing.T) {
	for _, kind := range []config.ProviderType{config.ProviderTypeAnthropic, config.ProviderTypeGemini} {
		t.Run(string(kind), func(t *testing.T) {
			for _, tc := range []struct {
				name string
				next []string
				want string
			}{
				{"success", []string{"one", "two", ""}, ""},
				{"special_characters", []string{"a+b /?&limit=1#frag%=雪\r\n", ""}, ""},
				{"repeated", []string{"one", "one"}, "cursor cycle"},
				{"cycle", []string{"one", "two", "one"}, "cursor cycle"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var calls atomic.Int32
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						i := int(calls.Add(1)) - 1
						if i >= len(tc.next) {
							http.Error(w, "unexpected extra request", 500)
							return
						}
						queryKey, sizeKey, size, header := "after_id", "limit", "1000", "x-api-key"
						if kind == config.ProviderTypeGemini {
							queryKey, sizeKey, size, header = "pageToken", "pageSize", "100", "x-goog-api-key"
						} else if r.Header.Get("anthropic-version") != "2023-06-01" {
							t.Error("missing Anthropic version header")
						}
						wantQuery := map[string][]string{sizeKey: {size}}
						if i > 0 {
							wantQuery[queryKey] = []string{tc.next[i-1]}
						}
						if !reflect.DeepEqual(map[string][]string(r.URL.Query()), wantQuery) || r.URL.Fragment != "" {
							t.Errorf("query=%v fragment=%q, want %v", r.URL.Query(), r.URL.Fragment, wantQuery)
						}
						if r.Header.Get(header) != "synthetic" || r.Header.Get("User-Agent") != "discovery-test/1" {
							t.Errorf("provider headers lost on page %d", i+1)
						}
						fmt.Fprint(w, discoveryPage(kind, 1, tc.next[i]))
					}))
					defer srv.Close()
					provider := discoveryProvider(kind, srv.URL)
					provider.Models = []config.Model{{Name: "configured", UpstreamName: "model-0"}}
					var stdout, stderr bytes.Buffer
					err := runModelsUpstream(context.Background(), provider, &stdout, &stderr)
					if calls.Load() != int32(len(tc.next)) {
						t.Fatalf("calls=%d, want %d", calls.Load(), len(tc.next))
					}
					if tc.want != "" {
						if err == nil || !strings.Contains(err.Error(), tc.want) || stdout.Len() != 0 || !strings.Contains(stderr.String(), "incomplete") {
							t.Fatalf("err=%v stdout=%q stderr=%q", err, &stdout, &stderr)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					for _, want := range []string{"Display model-0", "configured as test/configured", "not in config", fmt.Sprintf("%d upstream model(s)", len(tc.next))} {
						if !strings.Contains(stdout.String(), want) {
							t.Errorf("output missing %q: %s", want, &stdout)
						}
					}
				})
			}
		})
	}
}

func TestUpstreamDiscoveryPageBudget(t *testing.T) {
	for _, kind := range []config.ProviderType{config.ProviderTypeAnthropic, config.ProviderTypeGemini} {
		for _, pages := range []int{upstreamModelMaxPages - 1, upstreamModelMaxPages, upstreamModelMaxPages + 1} {
			t.Run(fmt.Sprintf("%s/%d", kind, pages), func(t *testing.T) {
				var calls atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					i := int(calls.Add(1))
					next := fmt.Sprintf("unique-%d", i)
					if i == pages {
						next = ""
					}
					fmt.Fprint(w, discoveryPage(kind, 1, next))
				}))
				defer srv.Close()
				models, err := listUpstreamModels(context.Background(), discoveryProvider(kind, srv.URL))
				if pages > upstreamModelMaxPages {
					if err == nil || !strings.Contains(err.Error(), "page limit") || models != nil {
						t.Fatalf("models=%v err=%v", models, err)
					}
				} else if err != nil || len(models) != pages {
					t.Fatalf("models=%d err=%v", len(models), err)
				}
				if calls.Load() != int32(min(pages, upstreamModelMaxPages)) {
					t.Fatalf("calls=%d", calls.Load())
				}
			})
		}
	}
}

func TestUpstreamDiscoveryEmptyAndInvalidContinuation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   config.ProviderType
		pages  []string
		want   string
		models int
	}{
		{"anthropic_empty_terminal", config.ProviderTypeAnthropic, []string{`{"data":[],"has_more":false}`}, "", 0},
		{"anthropic_empty_continuation", config.ProviderTypeAnthropic, []string{`{"data":[],"has_more":true,"last_id":"next"}`}, "continuation", 0},
		{"anthropic_mismatched_last_id", config.ProviderTypeAnthropic, []string{`{"data":[{"id":"one"}],"has_more":true,"last_id":"two"}`}, "continuation", 0},
		{"anthropic_terminal_last_id", config.ProviderTypeAnthropic, []string{`{"data":[{"id":"one"}],"has_more":false,"last_id":"one"}`}, "", 1},
		{"gemini_empty_terminal", config.ProviderTypeGemini, []string{discoveryPage(config.ProviderTypeGemini, 0, "")}, "", 0},
		{"gemini_empty_continuation", config.ProviderTypeGemini, []string{discoveryPage(config.ProviderTypeGemini, 0, "next"), discoveryPage(config.ProviderTypeGemini, 1, "")}, "", 1},
		{"gemini_empty_cycle", config.ProviderTypeGemini, []string{discoveryPage(config.ProviderTypeGemini, 0, "next"), discoveryPage(config.ProviderTypeGemini, 0, "next")}, "cursor cycle", 0},
		{"later_decode_failure", config.ProviderTypeGemini, []string{discoveryPage(config.ProviderTypeGemini, 1, "next"), `{"models":`}, "decode upstream models", 0},
		{"invalid_token_type", config.ProviderTypeGemini, []string{`{"models":[],"nextPageToken":42}`}, "decode upstream models", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				i := int(calls.Add(1)) - 1
				if i >= len(tc.pages) {
					http.Error(w, "extra request", 500)
					return
				}
				fmt.Fprint(w, tc.pages[i])
			}))
			defer srv.Close()
			models, err := listUpstreamModels(context.Background(), discoveryProvider(tc.kind, srv.URL))
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) || models != nil {
					t.Fatalf("models=%v err=%v", models, err)
				}
			} else if err != nil || len(models) != tc.models {
				t.Fatalf("models=%v err=%v", models, err)
			}
			if calls.Load() != int32(len(tc.pages)) {
				t.Fatalf("calls=%d", calls.Load())
			}
		})
	}
}

func TestUpstreamDiscoveryEmptyUniquePages(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, discoveryPage(config.ProviderTypeGemini, 0, fmt.Sprintf("unique-%d", calls.Add(1))))
	}))
	defer srv.Close()
	models, err := listUpstreamModels(context.Background(), discoveryProvider(config.ProviderTypeGemini, srv.URL))
	if err == nil || !strings.Contains(err.Error(), "page limit") || models != nil || calls.Load() != upstreamModelMaxPages {
		t.Fatalf("models=%v err=%v calls=%d", models, err, calls.Load())
	}
}

func TestUpstreamDiscoveryModelBudget(t *testing.T) {
	for _, kind := range []config.ProviderType{config.ProviderTypeOpenAI, config.ProviderTypeGitHubCopilot, config.ProviderTypeAnthropic, config.ProviderTypeGemini} {
		for _, delta := range []int{-1, 0, 1} {
			t.Run(fmt.Sprintf("%s/%d", kind, delta), func(t *testing.T) {
				count := upstreamModelMaxEntries + delta
				pages := []string{discoveryPage(kind, count, "")}
				if kind == config.ProviderTypeAnthropic || kind == config.ProviderTypeGemini {
					pages = []string{discoveryPage(kind, 5000, "next"), discoveryPage(kind, count-5000, "")}
				}
				var calls atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					i := int(calls.Add(1)) - 1
					if i >= len(pages) {
						http.Error(w, "extra request", 500)
						return
					}
					fmt.Fprint(w, pages[i])
				}))
				defer srv.Close()
				models, err := listUpstreamModels(context.Background(), discoveryProvider(kind, srv.URL))
				if delta > 0 {
					if err == nil || !strings.Contains(err.Error(), "model entry limit") || models != nil {
						t.Fatalf("models=%d err=%v", len(models), err)
					}
				} else if err != nil || len(models) != count {
					t.Fatalf("models=%d want=%d err=%v", len(models), count, err)
				}
				if calls.Load() != int32(len(pages)) {
					t.Fatalf("calls=%d", calls.Load())
				}
			})
		}
	}
}

func TestUpstreamDiscoveryCountsBlankEntries(t *testing.T) {
	for _, kind := range []config.ProviderType{config.ProviderTypeOpenAI, config.ProviderTypeGitHubCopilot, config.ProviderTypeAnthropic, config.ProviderTypeGemini} {
		t.Run(string(kind), func(t *testing.T) {
			key := "data"
			if kind == config.ProviderTypeGemini {
				key = "models"
			}
			body := `{"` + key + `":[` + strings.Repeat(`{},`, upstreamModelMaxEntries) + `{}]}`
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer srv.Close()
			models, err := listUpstreamModels(context.Background(), discoveryProvider(kind, srv.URL))
			if err == nil || !strings.Contains(err.Error(), "model entry limit") || models != nil {
				t.Fatalf("models=%v err=%v", models, err)
			}
		})
	}
}

func TestUpstreamDiscoveryPageBytes(t *testing.T) {
	for _, kind := range []config.ProviderType{
		config.ProviderTypeOpenAI, config.ProviderTypeOpenAICompatible, config.ProviderTypeZenMux,
		config.ProviderTypeOpenCodeZen, config.ProviderTypeOpenCodeGo, config.ProviderTypeGitHubCopilot,
		config.ProviderTypeAnthropic, config.ProviderTypeGemini,
	} {
		for _, delta := range []int{-1, 0, 1} {
			t.Run(fmt.Sprintf("%s/%d", kind, delta), func(t *testing.T) {
				body := discoveryPage(kind, 1, "")
				body += strings.Repeat(" ", upstreamModelMaxPageBytes+delta-len(body))
				var calls atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if (kind == config.ProviderTypeOpenCodeZen || kind == config.ProviderTypeOpenCodeGo) && !strings.HasPrefix(r.Header.Get("x-opencode-session"), "ses_") {
						t.Error("OpenCode session header missing")
					}
					fmt.Fprint(w, body)
				}))
				defer srv.Close()
				models, err := listUpstreamModels(context.Background(), discoveryProvider(kind, srv.URL))
				if delta > 0 {
					if err == nil || !strings.Contains(err.Error(), "page byte limit") || models != nil {
						t.Fatalf("models=%v err=%v", models, err)
					}
				} else if err != nil || len(models) != 1 {
					t.Fatalf("models=%v err=%v", models, err)
				}
				if calls.Load() != 1 {
					t.Fatalf("calls=%d", calls.Load())
				}
			})
		}
	}
}

func TestUpstreamDiscoveryAggregateBytes(t *testing.T) {
	for _, kind := range []config.ProviderType{config.ProviderTypeAnthropic, config.ProviderTypeGemini} {
		for _, delta := range []int{-1, 0, 1} {
			t.Run(fmt.Sprintf("%s/%d", kind, delta), func(t *testing.T) {
				var calls atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					i := calls.Add(1)
					size, next := 7<<20, fmt.Sprintf("next-%d", i)
					if i == 5 {
						size, next = upstreamModelMaxTotalBytes-4*(7<<20)+delta, ""
					}
					body := discoveryPage(kind, 1, next)
					fmt.Fprint(w, body, strings.Repeat(" ", size-len(body)))
				}))
				defer srv.Close()
				models, err := listUpstreamModels(context.Background(), discoveryProvider(kind, srv.URL))
				if delta > 0 {
					if err == nil || !strings.Contains(err.Error(), "aggregate byte limit") || models != nil {
						t.Fatalf("models=%v err=%v", models, err)
					}
				} else if err != nil || len(models) != 5 {
					t.Fatalf("models=%v err=%v", models, err)
				}
				if calls.Load() != 5 {
					t.Fatalf("calls=%d", calls.Load())
				}
			})
		}
		t.Run(string(kind)+"/exhausted_with_continuation", func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				i := calls.Add(1)
				body := discoveryPage(kind, 1, fmt.Sprintf("next-%d", i))
				fmt.Fprint(w, body, strings.Repeat(" ", upstreamModelMaxPageBytes-len(body)))
			}))
			defer srv.Close()
			models, err := listUpstreamModels(context.Background(), discoveryProvider(kind, srv.URL))
			if err == nil || !strings.Contains(err.Error(), "aggregate byte limit") || models != nil || calls.Load() != 4 {
				t.Fatalf("models=%v err=%v calls=%d", models, err, calls.Load())
			}
		})
	}
}

type discoveryTransport func(*http.Request) (*http.Response, error)

func (f discoveryTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type discoveryBody struct {
	io.Reader
	read    int
	closed  int
	onClose func()
}

func (b *discoveryBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func (b *discoveryBody) Close() error {
	b.closed++
	if b.onClose != nil {
		b.onClose()
	}
	return nil
}

type discoveryFailReader struct{ err error }

func (r discoveryFailReader) Read([]byte) (int, error) { return 0, r.err }

func TestUpstreamDiscoveryReadBoundsAndClosure(t *testing.T) {
	failure := errors.New("injected read failure")
	for _, tc := range []struct {
		name    string
		body    string
		status  int
		used    int64
		readErr error
		want    string
		maxRead int
	}{
		{"success", `{"data":[{"id":"m"}]}`, 200, 0, nil, "", 100},
		{"decode_failure", `{"data":`, 200, 0, nil, "decode", 100},
		{"status_failure", `upstream unavailable`, 503, 0, nil, "status 503", 100},
		{"read_failure", "", 200, 0, failure, "injected read failure", 0},
		{"oversized", strings.Repeat(" ", upstreamModelMaxPageBytes+1024), 200, 0, nil, "page byte limit", upstreamModelMaxPageBytes + 1},
		{"oversized_error", strings.Repeat(" ", upstreamModelMaxPageBytes+1024), 500, 0, nil, "page byte limit", upstreamModelMaxPageBytes + 1},
		{"remaining_budget", strings.Repeat(" ", 4096), 200, upstreamModelMaxTotalBytes - 100, nil, "aggregate byte limit", 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &discoveryBody{Reader: strings.NewReader(tc.body)}
			if tc.readErr != nil {
				body.Reader = discoveryFailReader{tc.readErr}
			}
			d := &upstreamModelDiscovery{bytes: tc.used, client: &http.Client{Transport: discoveryTransport(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: body, Header: make(http.Header)}, nil
			})}}
			models, err := d.list(context.Background(), discoveryProvider(config.ProviderTypeOpenAI, "http://fixture.invalid"))
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) || models != nil {
					t.Fatalf("models=%v err=%v", models, err)
				}
			} else if err != nil || len(models) != 1 {
				t.Fatalf("models=%v err=%v", models, err)
			}
			if tc.readErr != nil && !errors.Is(err, tc.readErr) {
				t.Fatalf("read error identity lost: %v", err)
			}
			if body.closed != 1 || body.read > tc.maxRead {
				t.Fatalf("closed=%d read=%d max=%d", body.closed, body.read, tc.maxRead)
			}
			if strings.Contains(tc.want, "byte limit") && body.read != tc.maxRead {
				t.Fatalf("read=%d, want limit+one=%d", body.read, tc.maxRead)
			}
		})
	}
}

func TestUpstreamDiscoveryClosesBeforeNextPage(t *testing.T) {
	for _, kind := range []config.ProviderType{config.ProviderTypeAnthropic, config.ProviderTypeGemini} {
		t.Run(string(kind), func(t *testing.T) {
			var bodies []*discoveryBody
			d := &upstreamModelDiscovery{cursors: make(map[string]struct{}), client: &http.Client{Transport: discoveryTransport(func(req *http.Request) (*http.Response, error) {
				if len(bodies) > 0 && bodies[len(bodies)-1].closed != 1 {
					t.Error("previous body still open")
				}
				next := "next"
				if len(bodies) > 0 {
					next = ""
				}
				body := &discoveryBody{Reader: strings.NewReader(discoveryPage(kind, 1, next))}
				bodies = append(bodies, body)
				return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
			})}}
			models, err := d.list(context.Background(), discoveryProvider(kind, "http://fixture.invalid"))
			if err != nil || len(models) != 2 || len(bodies) != 2 || bodies[1].closed != 1 {
				t.Fatalf("models=%v err=%v bodies=%v", models, err, bodies)
			}
		})
	}
}

type discoveryReaderFunc func([]byte) (int, error)

func (f discoveryReaderFunc) Read(p []byte) (int, error) { return f(p) }

func TestUpstreamDiscoveryWholeListDeadline(t *testing.T) {
	for _, kind := range []config.ProviderType{config.ProviderTypeAnthropic, config.ProviderTypeGemini} {
		for _, tc := range []struct {
			name           string
			parentTimeout  time.Duration
			cancelAfter    time.Duration
			requestTimeout time.Duration
			wantDuration   time.Duration
			wantCalls      int
			wantErr        error
		}{
			{"whole_list", 0, 0, 30 * time.Second, upstreamModelListTimeout, 5, context.DeadlineExceeded},
			{"shorter_parent", 40 * time.Second, 0, 30 * time.Second, 40 * time.Second, 2, context.DeadlineExceeded},
			{"cancellation", 0, 35 * time.Second, 30 * time.Second, 35 * time.Second, 2, context.Canceled},
			{"per_request_timeout", 0, 0, 10 * time.Second, 10 * time.Second, 1, context.DeadlineExceeded},
		} {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					if tc.parentTimeout > 0 {
						var cancelTimeout context.CancelFunc
						ctx, cancelTimeout = context.WithTimeout(ctx, tc.parentTimeout)
						defer cancelTimeout()
					}
					if tc.cancelAfter > 0 {
						time.AfterFunc(tc.cancelAfter, cancel)
					}
					var bodies []*discoveryBody
					oldTransport := http.DefaultTransport
					defer func() { http.DefaultTransport = oldTransport }()
					http.DefaultTransport = discoveryTransport(func(req *http.Request) (*http.Response, error) {
						if len(bodies) > 0 && bodies[len(bodies)-1].closed != 1 {
							t.Error("previous body still open")
						}
						payload := strings.NewReader(discoveryPage(kind, 1, fmt.Sprintf("unique-%d", len(bodies))))
						waited := false
						body := &discoveryBody{Reader: discoveryReaderFunc(func(p []byte) (int, error) {
							if !waited {
								waited = true
								select {
								case <-time.After(25 * time.Second):
								case <-req.Context().Done():
									return 0, req.Context().Err()
								}
							}
							return payload.Read(p)
						})}
						bodies = append(bodies, body)
						return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
					})
					provider := discoveryProvider(kind, "http://fixture.invalid")
					provider.UpstreamHeaderTimeout = tc.requestTimeout
					start := time.Now()
					models, err := listUpstreamModels(ctx, provider)
					if !errors.Is(err, tc.wantErr) || models != nil || len(bodies) != tc.wantCalls || time.Since(start) != tc.wantDuration {
						t.Fatalf("models=%v err=%v calls=%d duration=%v", models, err, len(bodies), time.Since(start))
					}
					for i, body := range bodies {
						if body.closed != 1 {
							t.Errorf("page %d closed=%d", i+1, body.closed)
						}
					}
				})
			})
		}
	}
}

func TestUpstreamDiscoveryHTTPCancellation(t *testing.T) {
	for _, stage := range []string{"before_request", "headers", "body"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if stage == "body" {
					w.WriteHeader(200)
					fmt.Fprint(w, `{"models":[`)
					w.(http.Flusher).Flush()
				}
				cancel()
				<-r.Context().Done()
			}))
			defer srv.Close()
			if stage == "before_request" {
				cancel()
			}
			start := time.Now()
			models, err := listUpstreamModels(ctx, discoveryProvider(config.ProviderTypeGemini, srv.URL))
			wantCalls := int32(1)
			if stage == "before_request" {
				wantCalls = 0
			}
			if !errors.Is(err, context.Canceled) || models != nil || calls.Load() != wantCalls || time.Since(start) > 2*time.Second {
				t.Fatalf("models=%v err=%v calls=%d duration=%v", models, err, calls.Load(), time.Since(start))
			}
		})
	}
}

func TestUpstreamDiscoveryCancellationAfterRead(t *testing.T) {
	for _, next := range []string{"", "next"} {
		t.Run("next="+next, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			body := &discoveryBody{Reader: strings.NewReader(discoveryPage(config.ProviderTypeGemini, 1, next)), onClose: cancel}
			oldTransport := http.DefaultTransport
			defer func() { http.DefaultTransport = oldTransport }()
			http.DefaultTransport = discoveryTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
			})
			models, err := listUpstreamModels(ctx, discoveryProvider(config.ProviderTypeGemini, "http://fixture.invalid"))
			if !errors.Is(err, context.Canceled) || models != nil || calls != 1 || body.closed != 1 {
				t.Fatalf("models=%v err=%v calls=%d closed=%d", models, err, calls, body.closed)
			}
		})
	}
}

func TestUpstreamDiscoveryCompressedPageLimit(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := io.WriteString(zw, `{"data":[{"id":"m"}]}`+strings.Repeat(" ", upstreamModelMaxPageBytes)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(compressed.Bytes())
	}))
	defer srv.Close()
	models, err := listUpstreamModels(context.Background(), discoveryProvider(config.ProviderTypeOpenAI, srv.URL))
	if err == nil || !strings.Contains(err.Error(), "page byte limit") || models != nil {
		t.Fatalf("models=%v err=%v", models, err)
	}
}

func TestModelsHelpDiscoveryLimits(t *testing.T) {
	cmd := newModelsCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"100 pages", "10000 model entries", "8 MiB", "32 MiB", "2m0s", "partial results", "last_id"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("help missing %q: %s", want, &output)
		}
	}
}
