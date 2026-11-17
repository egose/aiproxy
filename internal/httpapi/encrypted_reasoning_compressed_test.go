package httpapi

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/provider"
)

func TestScopedEncryptedReasoningDirectCompressedErrors(t *testing.T) {
	plain := []byte(scopedReasoningMismatch)
	zstdBody := reasoningZstd(t, plain, false)
	large := append(bytes.Clone(plain), bytes.Repeat([]byte(" "), 2<<20)...)
	encode := func(writer func(io.Writer) io.WriteCloser, input []byte) []byte {
		t.Helper()
		var output bytes.Buffer
		w := writer(&output)
		if _, err := w.Write(input); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		return bytes.Clone(output.Bytes())
	}
	gzipEncode := func(w io.Writer) io.WriteCloser { return gzip.NewWriter(w) }
	flateEncode := func(w io.Writer) io.WriteCloser {
		out, err := flate.NewWriter(w, flate.DefaultCompression)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	cases := []struct {
		name, encoding string
		raw            []byte
		status         int
		retry, repeat  bool
	}{
		{"zstd", "zstd", zstdBody, 400, true, false},
		{"gzip", "gzip", encode(gzipEncode, plain), 400, true, false},
		{"brotli", "br", encode(func(w io.Writer) io.WriteCloser { return brotli.NewWriter(w) }, plain), 400, true, false},
		{"deflate", "deflate", encode(flateEncode, plain), 400, true, false},
		{"chain", "zstd,gzip", encode(gzipEncode, zstdBody), 400, true, false},
		{"one-recovery", "zstd", zstdBody, 400, true, true},
		{"over-budget-known", "zstd", reasoningZstd(t, large, false), 400, false, false},
		{"over-budget-unknown", "zstd", reasoningZstd(t, large, true), 400, false, false},
		{"over-budget-gzip", "gzip", encode(gzipEncode, large), 400, false, false},
		{"corrupt-literal", "zstd", plain, 400, false, false},
		{"unsupported-literal", "compress", plain, 400, false, false},
		{"excess-layers", "identity,identity,identity,identity,zstd", zstdBody, 400, false, false},
		{"excess-encoding-bytes", strings.Repeat("identity,", 40) + "zstd", zstdBody, 400, false, false},
		{"unauthorized", "zstd", zstdBody, 401, false, false},
		{"forbidden", "zstd", zstdBody, 403, false, false},
		{"server-error", "zstd", zstdBody, 503, false, false},
		{"no-opaque", "zstd", zstdBody, 400, false, false},
		{"unchanged-rewrite", "zstd", zstdBody, 400, false, false},
	}
	for _, op := range []provider.Operation{provider.OpChatCompletions, provider.OpResponses} {
		for _, scope := range []string{"root", "provider", "model"} {
			for _, tc := range cases {
				t.Run(op.String()+"/"+scope+"/"+tc.name, func(t *testing.T) {
					var mu sync.Mutex
					var upstreamBodies [][]byte
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						input, err := io.ReadAll(r.Body)
						if err != nil {
							t.Error(err)
						}
						path := "/v1/chat/completions"
						if op == provider.OpResponses {
							path = "/v1/responses"
						}
						if r.URL.Path != path || r.Header.Get("Authorization") != "Bearer credential-p1" {
							t.Errorf("compressed recovery changed upstream identity: path=%s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
						}
						mu.Lock()
						call := len(upstreamBodies)
						upstreamBodies = append(upstreamBodies, input)
						mu.Unlock()
						w.Header().Set("Content-Type", "application/json")
						if call == 0 || tc.repeat {
							w.Header()["Content-Encoding"] = []string{tc.encoding}
							w.Header()["X-Upstream-Marker"] = []string{"original", "second"}
							w.WriteHeader(tc.status)
							_, _ = w.Write(tc.raw)
							return
						}
						_, _ = io.WriteString(w, `{"id":"recovered"}`)
					}))
					defer upstream.Close()
					rt := scopedReasoningRuntime("", "strip", nil, nil)
					providers := rt.Catalog.Providers()
					providers[0].BaseURL = upstream.URL
					switch scope {
					case "root":
						rt.EncryptedReasoning = scopedReasoningPolicy("retry")
					case "provider":
						providers[0].EncryptedReasoning = scopedReasoningPolicy("retry")
					case "model":
						providers[0].Models[0].EncryptedReasoning = scopedReasoningPolicy("retry")
						providers[0].ModelByName = nil
					}
					rt.Catalog = config.NewCatalog(providers, nil, rt.Catalog.Aliases())
					adapter := provider.New()
					probe := &scopedReasoningProbe{do: func(_ int, ctx context.Context, request provider.Request) (*provider.Result, error) {
						return adapter.Do(ctx, request)
					}}
					h, _, clients := scopedReasoningHandler(t, rt, probe)
					for _, client := range clients {
						client.Transport = &http.Transport{DisableCompression: true}
						defer client.CloseIdleConnections()
					}
					ctx := context.Background()
					original, stripped := scopedReasoningBodies(op, "p1/m")
					if tc.name == "no-opaque" || tc.name == "unchanged-rewrite" {
						text := "hi"
						if tc.name == "unchanged-rewrite" {
							text = "ordinary text mentions encrypted_content"
						}
						original = []byte(`{ "model":"p1/m","messages":[{"role":"user","content":"` + text + `"}],"input":"` + text + `"}`)
					}
					w := scopedReasoningPost(h, ctx, op, original)
					expectedStatus, expectedBody, expectedCalls := tc.status, tc.raw, 1
					if tc.retry {
						expectedCalls = 2
						if !tc.repeat {
							expectedStatus, expectedBody = 200, []byte(`{"id":"recovered"}`)
						}
					}
					attempts := probe.snapshot()
					if w.Code != expectedStatus || !bytes.Equal(w.Body.Bytes(), expectedBody) || len(attempts) != expectedCalls {
						t.Fatalf("compressed response/recovery changed: status=%d calls=%d bytes=%d; want %d/%d/%d", w.Code, len(attempts), w.Body.Len(), expectedStatus, expectedCalls, len(expectedBody))
					}
					if !tc.retry || tc.repeat {
						if w.Header().Get("Content-Encoding") != tc.encoding ||
							!reflect.DeepEqual(w.Header().Values("X-Upstream-Marker"), []string{"original", "second"}) {
							t.Fatal("non-recovered compressed response headers changed")
						}
					}
					for i, attempt := range attempts {
						assertScopedReasoningAttempt(t, attempt, rt, clients, ctx, op, "p1/m", "p1")
						assertScopedReasoningBody(t, attempt.Request.Body, original, stripped, i == 1)
						if attempt.Closes.Load() != 1 {
							t.Fatalf("compressed attempt %d finalized %d times", i+1, attempt.Closes.Load())
						}
					}
					mu.Lock()
					defer mu.Unlock()
					if len(upstreamBodies) != expectedCalls {
						t.Fatal("dispatch invocation count differed from actual inference count")
					}
					upstreamOriginal := bytes.Replace(original, []byte(`"p1/m"`), []byte(`"upstream-p1"`), 1)
					upstreamStripped := bytes.Replace(stripped, []byte(`"p1/m"`), []byte(`"upstream-p1"`), 1)
					for i, body := range upstreamBodies {
						expected := upstreamOriginal
						if i == 1 {
							expected = upstreamStripped
						}
						assertScopedReasoningBody(t, body, expected, expected, true)
					}
				})
			}
		}
	}
}
