package httpapi

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/klauspost/compress/zstd"
)

func reasoningZstd(t *testing.T, plain []byte, streaming bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf, zstd.WithEncoderConcurrency(1), zstd.WithWindowSize(1<<20), zstd.WithSingleSegment(false))
	if err != nil {
		t.Fatal(err)
	}
	if !streaming {
		defer w.Close()
		return w.EncodeAll(plain, nil)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAliasBoundedCompressedInspection(t *testing.T) {
	plain := []byte(`{"error":{"message":"reasoning encrypted_content was not issued to this caller"}}`)
	small := reasoningZstd(t, plain, false)
	large := append(bytes.Clone(plain), bytes.Repeat([]byte(" "), 2<<20)...)
	var chained bytes.Buffer
	gw := gzip.NewWriter(&chained)
	if _, err := gw.Write(small); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"chat/completions", "responses"} {
		for _, tc := range []struct {
			name           string
			raw            []byte
			encoding       string
			status         int
			noOpaque       bool
			disabled       bool
			direct         bool
			retry          bool
			repeatMismatch bool
		}{
			{name: "small", raw: small, encoding: "zstd", status: 400, retry: true},
			{name: "chain", raw: chained.Bytes(), encoding: "zstd,gzip", status: 400, retry: true},
			{name: "retry_once", raw: small, encoding: "zstd", status: 400, retry: true, repeatMismatch: true},
			{name: "large_known", raw: reasoningZstd(t, large, false), encoding: "zstd", status: 400},
			{name: "large_unknown", raw: reasoningZstd(t, large, true), encoding: "zstd", status: 400},
			{name: "corrupt_literal", raw: plain, encoding: "zstd", status: 400},
			{name: "unsupported_literal", raw: plain, encoding: "compress", status: 400},
			{name: "excess_layers", raw: small, encoding: "identity,identity,identity,identity,zstd", status: 400},
			{name: "success", raw: small, encoding: "zstd", status: 200},
			{name: "unauthorized", raw: small, encoding: "zstd", status: 401},
			{name: "no_opaque", raw: small, encoding: "zstd", status: 400, noOpaque: true},
			{name: "disabled", raw: small, encoding: "zstd", status: 400, disabled: true},
			{name: "direct", raw: small, encoding: "zstd", status: 400, direct: true},
		} {
			t.Run(op+"/"+tc.name, func(t *testing.T) {
				var mu sync.Mutex
				var bodies [][]byte
				var keys []string
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					if r.URL.Path != "/v1/"+op {
						t.Errorf("upstream path = %s", r.URL.Path)
					}
					mu.Lock()
					bodies = append(bodies, body)
					keys = append(keys, r.Header.Get("Authorization"))
					call := len(bodies)
					mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					if call == 1 || tc.repeatMismatch {
						w.Header().Set("Content-Encoding", tc.encoding)
						w.Header().Set("X-Upstream-Marker", "original")
						w.WriteHeader(tc.status)
						_, _ = w.Write(tc.raw)
						return
					}
					_, _ = io.WriteString(w, `{"id":"ok"}`)
				}))
				defer upstream.Close()
				er := &config.EncryptedReasoning{Passthrough: true, OnCallerMismatch: config.EncryptedReasoningStripAndRetry}
				if tc.disabled {
					er.OnCallerMismatch = config.EncryptedReasoningFail
				}
				rt := reasoningTestRT(er)
				replaceProviders(rt, []config.Provider{
					{Type: config.ProviderTypeOpenAI, Name: "p1", APIKey: "key1", BaseURL: upstream.URL, Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
					{Type: config.ProviderTypeOpenAI, Name: "p2", APIKey: "key2", BaseURL: upstream.URL, Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
				})
				h := NewHandler(Dependencies{
					Resolver: modelresolver.New(rt), Adapter: provider.New(), Catalog: rt.Catalog,
					Auth: auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}), Metrics: observability.NewMetrics(),
				})
				body := `{"model":"alias/a","messages":[{"role":"assistant","content":[{"type":"reasoning","encrypted_content":"blob"},{"type":"text","text":"hi"}]}]}`
				if op == "responses" {
					body = `{"model":"alias/a","input":[{"type":"reasoning","encrypted_content":"blob"},{"type":"message","role":"user","content":"hi"}]}`
				}
				if tc.noOpaque {
					body = `{"model":"alias/a","messages":[{"role":"user","content":"hi"}],"input":"hi"}`
				}
				if tc.direct {
					body = strings.Replace(body, "alias/a", "p1/m", 1)
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/"+op, strings.NewReader(body)))
				wantStatus, wantBody, wantCalls := tc.status, tc.raw, 1
				if tc.retry {
					wantCalls = 2
					if !tc.repeatMismatch {
						wantStatus, wantBody = 200, []byte(`{"id":"ok"}`)
					}
				}
				if w.Code != wantStatus || !bytes.Equal(w.Body.Bytes(), wantBody) {
					t.Fatalf("response: status=%d want=%d bytes=%d want=%d", w.Code, wantStatus, w.Body.Len(), len(wantBody))
				}
				if !tc.retry || tc.repeatMismatch {
					if w.Header().Get("Content-Encoding") != tc.encoding || w.Header().Get("X-Upstream-Marker") != "original" {
						t.Fatal("original response headers lost")
					}
				}
				mu.Lock()
				defer mu.Unlock()
				if len(bodies) != wantCalls {
					t.Fatalf("upstream calls=%d want=%d", len(bodies), wantCalls)
				}
				if !tc.noOpaque && !bytes.Contains(bodies[0], []byte("blob")) {
					t.Fatal("first attempt lost opaque reasoning")
				}
				if tc.retry {
					if keys[0] != keys[1] || keys[0] == "" {
						t.Fatal("stripped retry changed target credential")
					}
					if bytes.Contains(bodies[1], []byte("blob")) || !bytes.Contains(bodies[1], []byte("hi")) {
						t.Fatalf("retry did not preserve text while stripping opaque data: %s", bodies[1])
					}
				}
			})
		}
	}
}

func TestEncodedCallerMismatchSkipsDecoding(t *testing.T) {
	raw := reasoningZstd(t, []byte(`{"error":"not issued to this caller"}`), false)
	patterns := []string{"not issued to this caller"}
	opaque := []byte(`{"encrypted_content":"blob"}`)
	for _, name := range []string{"nil", "streaming", "success", "other_error", "empty_patterns", "no_opaque", "empty_body"} {
		t.Run(name, func(t *testing.T) {
			result := &provider.Result{StatusCode: 400, Header: http.Header{"Content-Encoding": {"zstd"}}, Body: raw}
			body, match := opaque, patterns
			switch name {
			case "nil":
				result = nil
			case "streaming":
				result.Streaming = true
			case "success":
				result.StatusCode = 200
			case "other_error":
				result.StatusCode = 500
			case "empty_patterns":
				match = nil
			case "no_opaque":
				body = []byte(`{"messages":[]}`)
			case "empty_body":
				result.Body = nil
			}
			allocs := testing.AllocsPerRun(20, func() {
				if isEncodedCallerMismatch(result, body, match) {
					t.Fatal("inapplicable response matched")
				}
			})
			t.Logf("allocations per skipped inspection: %.0f", allocs)
			if allocs != 0 {
				t.Fatalf("inapplicable response allocated %.0f times; decoder must not run", allocs)
			}
		})
	}
}
