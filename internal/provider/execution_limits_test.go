package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
)

type executionTestBudget struct {
	mu      sync.Mutex
	limit   int64
	used    int64
	peak    int64
	calls   []int64
	deny    func(int64) bool
	changed chan struct{}
}

func (b *executionTestBudget) Reserve(n int64) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, n)
	if n < 0 || n > b.limit-b.used || b.deny != nil && b.deny(n) {
		return nil, deferred.ErrBudgetExceeded
	}
	b.used += n
	b.peak = max(b.peak, b.used)
	return sync.OnceFunc(func() {
		b.mu.Lock()
		b.used -= n
		if b.changed != nil {
			close(b.changed)
			b.changed = nil
		}
		b.mu.Unlock()
	}), nil
}

func (b *executionTestBudget) stats() (int64, int64, []int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, b.peak, append([]int64(nil), b.calls...)
}

func (b *executionTestBudget) awaitReleased(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.waitReleased(ctx); err != nil {
		used, _, _ := b.stats()
		t.Fatalf("execution ownership did not drain: used=%d err=%v", used, err)
	}
}

func (b *executionTestBudget) waitReleased(ctx context.Context) error {
	for {
		b.mu.Lock()
		if b.used == 0 {
			b.mu.Unlock()
			return nil
		}
		if b.changed == nil {
			b.changed = make(chan struct{})
		}
		changed := b.changed
		b.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func TestExecutionBudgetDrainWaitRequiresEveryReservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		budget := &executionTestBudget{limit: 1024}
		releaseFirst, err := budget.Reserve(32)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseFirst()
		releaseLast, err := budget.Reserve(64)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseLast()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- budget.waitReleased(ctx) }()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatal("drain check ignored outstanding reservations", err)
		default:
		}
		releaseFirst()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatal("partial release passed the drain check", err)
		default:
		}
		<-ctx.Done()
		if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("retained reservation did not fail the bounded drain check", err)
		}
		if used, _, _ := budget.stats(); used != 64 {
			t.Fatalf("drain timeout freed the outstanding reservation: used=%d", used)
		}
		releaseLast()
		releaseLast()
		budget.awaitReleased(t)
		if used, _, _ := budget.stats(); used != 0 {
			t.Fatalf("last release did not drain exactly once: used=%d", used)
		}
	})
}

type executionTestBody struct {
	reader  io.Reader
	mu      sync.Mutex
	reads   int
	bytes   int
	maxRead int
	closes  int
	onRead  func(int)
}

func (b *executionTestBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	b.reads++
	b.maxRead = max(b.maxRead, len(p))
	b.mu.Unlock()
	if b.onRead != nil {
		b.onRead(len(p))
	}
	n, err := b.reader.Read(p)
	b.mu.Lock()
	b.bytes += n
	b.mu.Unlock()
	return n, err
}

func (b *executionTestBody) Close() error {
	b.mu.Lock()
	b.closes++
	b.mu.Unlock()
	return nil
}

func (b *executionTestBody) stats() (int, int, int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reads, b.bytes, b.maxRead, b.closes
}

type executionRoundTripper func(*http.Request) (*http.Response, error)

func (f executionRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type executionProviderCase struct {
	name        string
	provider    config.ProviderType
	protocol    config.ModelProtocol
	translation string
}

func executionProviderCases() []executionProviderCase {
	return []executionProviderCase{
		{"openai", config.ProviderTypeOpenAI, "", ""},
		{"compatible", config.ProviderTypeOpenAICompatible, "", ""},
		{"anthropic", config.ProviderTypeAnthropic, "", "anthropic"},
		{"gemini", config.ProviderTypeGemini, "", "gemini"},
		{"zen-chat", config.ProviderTypeOpenCodeZen, config.ModelProtocolChat, ""},
		{"go-chat", config.ProviderTypeOpenCodeGo, config.ModelProtocolChat, ""},
		{"zen-messages", config.ProviderTypeOpenCodeZen, config.ModelProtocolMessages, "anthropic"},
		{"go-messages", config.ProviderTypeOpenCodeGo, config.ModelProtocolMessages, "anthropic"},
		{"zen-gemini", config.ProviderTypeOpenCodeZen, config.ModelProtocolGemini, "gemini"},
		{"copilot", config.ProviderTypeGitHubCopilot, "", ""},
		{"zenmux", config.ProviderTypeZenMux, "", ""},
		{"openrouter", config.ProviderTypeOpenRouter, "", ""},
	}
}

func executionRequestFor(c executionProviderCase, budget *executionTestBudget, streaming bool) Request {
	body := []byte(fmt.Sprintf(`{"model":"p/m","messages":[{"role":"user","content":"hello"}],"stream":%t}`, streaming))
	return Request{
		Operation: OpChatCompletions, ProviderType: c.provider, ModelProtocol: c.protocol,
		PublicModel: "p/m", UpstreamModel: "up-model", BaseURL: "http://127.0.0.1:1",
		APIKey: "secret", CopilotToken: "saved-token", Version: "test", Body: body,
		Inbound:         httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)),
		ExecutionLimits: &ExecutionLimits{RequestBytes: 1 << 20, ResponseBytes: 64 << 10, Work: budget},
	}
}

func executionResponseFor(c executionProviderCase, streaming bool) []byte {
	if !streaming {
		switch c.translation {
		case "anthropic":
			return []byte(`{"id":"up-id","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
		case "gemini":
			return []byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5}}`)
		default:
			return []byte(`{"choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5},"vendor":"kept"}`)
		}
	}
	switch c.translation {
	case "anthropic":
		return []byte("event: message_start\ndata: {\"message\":{\"id\":\"up-id\",\"usage\":{\"input_tokens\":2}}}\n\n" +
			"event: content_block_delta\ndata: {\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n" +
			"event: message_delta\ndata: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\n" +
			"event: message_stop\ndata: {}\n\n")
	case "gemini":
		return []byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":3,\"totalTokenCount\":5}}\n\n")
	default:
		return []byte(": untouched comment\r\nevent: vendor\ndata: {\"choices\":[],\r\ndata: \"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\r\n\r\ndata: [DONE]\n\n")
	}
}

func executionClient(t *testing.T, raw []byte, streaming bool, contentLength int64, status int) (*http.Client, *executionTestBody, *atomic.Int32, *http.Header, *[]byte) {
	t.Helper()
	body := &executionTestBody{reader: bytes.NewReader(raw)}
	var calls atomic.Int32
	var headers http.Header
	var outgoing []byte
	client := &http.Client{Transport: executionRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		headers = r.Header.Clone()
		var err error
		outgoing, err = io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			return nil, err
		}
		contentType := "application/json"
		if streaming {
			contentType = "text/event-stream"
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: body, ContentLength: contentLength}, nil
	})}
	return client, body, &calls, &headers, &outgoing
}

func executionReadResult(result *Result) ([]byte, error) {
	if result.Streaming {
		body, err := io.ReadAll(result.StreamBody)
		result.Stream.Complete(err, false)
		return body, err
	}
	return bytes.Clone(result.Body), nil
}

func executionCloseResult(result *Result) {
	if result == nil {
		return
	}
	if result.OnClose != nil {
		result.OnClose()
		result.OnClose()
	} else if result.StreamBody != nil {
		_ = result.StreamBody.Close()
	}
}

func TestExecutionLimitsAllChatAdaptersPreserveJSONSSEAndNilPaths(t *testing.T) {
	for _, c := range executionProviderCases() {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", c.name, streaming), func(t *testing.T) {
				budget := &executionTestBudget{limit: 32 << 20}
				r := executionRequestFor(c, budget, streaming)
				raw := executionResponseFor(c, streaming)
				ordinary := r
				ordinary.ExecutionLimits = nil
				ordinary.Client, _, _, _, _ = executionClient(t, raw, streaming, int64(len(raw)), 200)
				expected, err := New().Do(context.Background(), ordinary)
				if err != nil {
					t.Fatal(err)
				}
				expectedBody, err := executionReadResult(expected)
				if err != nil {
					t.Fatal(err)
				}
				defer executionCloseResult(expected)
				client, source, calls, _, _ := executionClient(t, raw, streaming, int64(len(raw)), 200)
				r.Client = client
				result, err := New().Do(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := executionReadResult(result)
				if err != nil || !bytes.Equal(actual, expectedBody) || result.StatusCode != expected.StatusCode || calls.Load() != 1 {
					t.Fatalf("adapter semantics changed: %v\nactual=%q\nexpected=%q", err, actual, expectedBody)
				}
				usage, expectedUsage := result.Usage, expected.Usage
				if streaming {
					usage, expectedUsage = result.Stream.Wait().Usage, expected.Stream.Wait().Usage
				}
				if usage != expectedUsage {
					t.Fatalf("usage changed: %+v != %+v", usage, expectedUsage)
				}
				if result.UpstreamRequestHeaders != nil || result.UpstreamRequestBody != nil || result.UpstreamRequestBodyTruncated {
					t.Fatal("logging-off execution retained a request snapshot")
				}
				if used, peak, _ := budget.stats(); used == 0 || peak > budget.limit {
					t.Fatalf("returned result lost ownership or exceeded work: used=%d peak=%d", used, peak)
				}
				executionCloseResult(result)
				budget.awaitReleased(t)
				if _, _, _, closes := source.stats(); closes != 1 {
					t.Fatalf("provider source closed %d times", closes)
				}
			})
		}
	}
}

func TestExecutionLimitsInputStructureAndWorkRejectBeforeAdapterAllocation(t *testing.T) {
	for _, c := range executionProviderCases() {
		for _, kind := range []string{"bytes", "depth", "tokens", "work", "missing_model", "duplicate_stream"} {
			t.Run(c.name+"/"+kind, func(t *testing.T) {
				budget := &executionTestBudget{limit: 4 << 20}
				r := executionRequestFor(c, budget, false)
				switch kind {
				case "bytes":
					r.ExecutionLimits.RequestBytes = int64(len(r.Body)) - 1
				case "depth":
					r.Body = []byte(`{"model":"p/m","messages":[],"x":` + strings.Repeat("[", deferred.JSONDepth) + "0" + strings.Repeat("]", deferred.JSONDepth) + "}")
				case "tokens":
					r.Body = []byte(`{"model":"p/m","messages":[],"x":[` + strings.Repeat("0,", deferred.JSONTokens) + "0]}")
				case "work":
					shape, _, err := deferred.InspectChatInput(r.Body, r.ExecutionLimits.RequestBytes, deferred.JSONDepth, deferred.JSONTokens)
					if err != nil {
						t.Fatal(err)
					}
					denied := jsonDecodeWorkBytes(len(r.Body), shape.Tokens)
					budget.deny = func(n int64) bool { return n == denied }
				case "missing_model":
					r.Body = []byte(`{"messages":[]}`)
				case "duplicate_stream":
					r.Body = []byte(`{"model":"p/m","stream":true,"stream":false}`)
				}
				client, _, calls, _, _ := executionClient(t, []byte("{}"), false, 2, 200)
				r.Client = client
				_, err := New().Do(context.Background(), r)
				if err == nil || calls.Load() != 0 {
					t.Fatal("input rejection dispatched upstream", err)
				}
				used, _, reservations := budget.stats()
				wantReservations := 1
				if kind == "work" {
					wantReservations++
				}
				if used != 0 || len(reservations) != wantReservations || reservations[0] != deferred.JobMetadataBytes {
					t.Fatalf("invalid input allocated parser/translation/snapshot ownership: used=%d calls=%v", used, reservations)
				}
				var invalid ErrInvalidRequest
				if kind == "missing_model" || kind == "duplicate_stream" {
					if !errors.As(err, &invalid) {
						t.Fatal("invalid input lost its adapter rejection type", err)
					}
				} else if !errors.Is(err, deferred.ErrBudgetExceeded) {
					t.Fatal("limit rejection lost its typed failure", err)
				}
			})
		}
	}
}

func TestExecutionLimitsBufferedBodiesBoundReadsAndCapacityBeforeAllocation(t *testing.T) {
	for _, c := range executionProviderCases() {
		for _, kind := range []string{"declared_overflow", "unknown_overflow", "boundary", "deny_buffer"} {
			t.Run(c.name+"/"+kind, func(t *testing.T) {
				budget := &executionTestBudget{limit: 8 << 20}
				r := executionRequestFor(c, budget, false)
				r.ExecutionLimits.ResponseBytes = 1024
				raw := executionResponseFor(c, false)
				raw = append(raw, bytes.Repeat([]byte(" "), 1024-len(raw))...)
				contentLength := int64(-1)
				if kind == "declared_overflow" {
					raw = append(raw, ' ')
					contentLength = int64(len(raw))
				}
				if kind == "unknown_overflow" {
					raw = append(raw, ' ')
				}
				if kind == "deny_buffer" {
					budget.deny = func(n int64) bool { return n == 1024 }
				}
				client, source, _, _, _ := executionClient(t, raw, false, contentLength, 200)
				r.Client = client
				source.onRead = func(size int) {
					used, _, _ := budget.stats()
					if used < int64(size) {
						t.Error("provider read preceded allocation ownership")
					}
				}
				result, err := New().Do(context.Background(), r)
				if kind == "boundary" {
					if err != nil || int64(len(result.Body)) > r.ExecutionLimits.ResponseBytes {
						t.Fatalf("maximum body rejected: %v", err)
					}
					executionCloseResult(result)
				} else if !errors.Is(err, deferred.ErrBudgetExceeded) {
					t.Fatal("oversized or unowned body was not rejected", err)
				}
				reads, readBytes, maxRead, closes := source.stats()
				if (kind == "declared_overflow" || kind == "deny_buffer") && reads != 0 {
					t.Fatal("known overflow or failed reservation read the body")
				}
				if readBytes > 1025 || maxRead > 1024 || closes != 1 {
					t.Fatalf("body read/close escaped bounds: bytes=%d max_read=%d closes=%d", readBytes, maxRead, closes)
				}
				if used, peak, _ := budget.stats(); used != 0 || peak > budget.limit {
					t.Fatalf("body failure leaked or overreserved: used=%d peak=%d", used, peak)
				}
			})
		}
	}
}

func TestExecutionLimitsFallbackInputReaderAndEncodedRequestHavePriorBounds(t *testing.T) {
	c := executionProviderCases()[0]
	for _, kind := range []string{"declared", "unknown", "reserve"} {
		t.Run(kind, func(t *testing.T) {
			budget := &executionTestBudget{limit: 4 << 20}
			r := executionRequestFor(c, budget, false)
			r.ExecutionLimits.RequestBytes = 1024
			input := &executionTestBody{reader: strings.NewReader(strings.Repeat(" ", 1025))}
			r.Body, r.Inbound.Body, r.Inbound.ContentLength = nil, input, -1
			if kind == "declared" {
				r.Inbound.ContentLength = 1025
			}
			if kind == "reserve" {
				budget.deny = func(n int64) bool { return n == 1024 }
			}
			client, _, calls, _, _ := executionClient(t, []byte("{}"), false, 2, 200)
			r.Client = client
			_, err := New().Do(context.Background(), r)
			if !errors.Is(err, deferred.ErrBudgetExceeded) || calls.Load() != 0 {
				t.Fatal("unbounded fallback input dispatched", err)
			}
			reads, size, largest, _ := input.stats()
			if kind != "unknown" && reads != 0 || size > 1025 || largest > 1024 {
				t.Fatalf("input overflow consumed too much: reads=%d bytes=%d largest=%d", reads, size, largest)
			}
			if used, _, _ := budget.stats(); used != 0 {
				t.Fatal("fallback input reservation leaked")
			}
		})
	}
	budget := &executionTestBudget{limit: 4 << 20}
	r := executionRequestFor(c, budget, false)
	r.ExecutionLimits.RequestBytes = int64(len(r.Body))
	r.UpstreamModel = strings.Repeat("m", 32)
	client, _, calls, _, _ := executionClient(t, []byte("{}"), false, 2, 200)
	r.Client = client
	_, err := New().Do(context.Background(), r)
	var limit ErrExecutionLimit
	if !errors.As(err, &limit) || limit.Kind != "rewritten request bytes" || calls.Load() != 0 {
		t.Fatal("rewritten output grew before its preallocation check", err)
	}
	if used, _, reservations := budget.stats(); used != 0 || len(reservations) != 2 {
		t.Fatalf("oversized rewritten output obtained ownership: used=%d calls=%v", used, reservations)
	}
}

func TestExecutionLimitsSnapshotsAreCapturePrefixesAndPrivateHeadersStayPrivate(t *testing.T) {
	for _, c := range executionProviderCases() {
		for _, capture := range []int64{0, 17} {
			t.Run(fmt.Sprintf("%s/%d", c.name, capture), func(t *testing.T) {
				budget := &executionTestBudget{limit: 32 << 20}
				r := executionRequestFor(c, budget, false)
				r.Body = []byte(`{"model":"p/m","messages":[{"role":"user","content":"` + strings.Repeat("text ", 10000) + `"}]}`)
				r.ExecutionLimits.SnapshotBytes = capture
				r.Inbound.Header.Set("X-Aiproxy-Deferred-Secret", "private")
				r.Inbound.Header.Set("X-Test", "kept")
				r.ForwardHeaders = []string{"X-Aiproxy-Deferred-Secret", "X-Test"}
				raw := executionResponseFor(c, false)
				client, _, _, headers, outgoing := executionClient(t, raw, false, int64(len(raw)), 200)
				r.Client = client
				result, err := New().Do(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				if headers.Get("X-Aiproxy-Deferred-Secret") != "" || headers.Get("X-Test") != "kept" {
					t.Fatal("private namespace reached inference or ordinary forwarding changed")
				}
				if capture == 0 {
					if result.UpstreamRequestBody != nil || result.UpstreamRequestHeaders != nil {
						t.Fatal("logging-off snapshot allocated")
					}
				} else if int64(cap(result.UpstreamRequestBody)) != capture ||
					!bytes.Equal(result.UpstreamRequestBody, (*outgoing)[:int(capture)]) || !result.UpstreamRequestBodyTruncated {
					t.Fatal("snapshot copied more than its permitted capture prefix")
				}
				executionCloseResult(result)
				if used, _, _ := budget.stats(); used != 0 {
					t.Fatalf("prefix snapshot leaked %d bytes", used)
				}
			})
		}
	}
	budget := &executionTestBudget{limit: 4 << 20, deny: func(n int64) bool { return n == 17 }}
	r := executionRequestFor(executionProviderCases()[0], budget, false)
	r.ExecutionLimits.SnapshotBytes = 17
	client, _, calls, _, _ := executionClient(t, []byte("{}"), false, 2, 200)
	r.Client = client
	if _, err := New().Do(context.Background(), r); !errors.Is(err, deferred.ErrBudgetExceeded) || calls.Load() != 0 {
		t.Fatal("snapshot allocation/read occurred before its reservation", err)
	}
}

func TestExecutionLimitsEncodedTranslationFailsBeforeOutputAllocation(t *testing.T) {
	for _, c := range executionProviderCases() {
		if c.translation == "" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			budget := &executionTestBudget{limit: 8 << 20}
			r := executionRequestFor(c, budget, false)
			raw := []byte(`{"content":[]}`)
			if c.translation == "gemini" {
				raw = []byte(`{"candidates":[]}`)
			}
			r.ExecutionLimits.ResponseBytes = 64
			r.Client, _, _, _, _ = executionClient(t, raw, false, int64(len(raw)), 200)
			if _, err := New().Do(context.Background(), r); !errors.Is(err, deferred.ErrBudgetExceeded) {
				t.Fatal("translated response encoded beyond its ceiling", err)
			}
			if used, _, _ := budget.stats(); used != 0 {
				t.Fatal("failed response encoding leaked ownership")
			}
		})
	}
	budget := &executionTestBudget{limit: 8 << 20}
	state, err := newExecutionState(ExecutionLimits{RequestBytes: 1024, ResponseBytes: 64, Work: budget})
	if err != nil {
		t.Fatal(err)
	}
	defer state.release()
	_, _, before := budget.stats()
	if _, _, err := state.marshal(struct {
		Text string `json:"text"`
	}{Text: strings.Repeat("&", 64)}, 64, false); !errors.Is(err, deferred.ErrBudgetExceeded) {
		t.Fatal("JSON escape expansion was not checked before marshal", err)
	}
	_, _, after := budget.stats()
	if len(after) != len(before) {
		t.Fatal("oversized encoding reserved/allocated a buffer")
	}
}

func TestExecutionLimitsLongUpstreamMetadataReservedBeforeTargetConstruction(t *testing.T) {
	for _, c := range executionProviderCases() {
		t.Run(c.name, func(t *testing.T) {
			budget := &executionTestBudget{limit: 4 << 20}
			r := executionRequestFor(c, budget, false)
			r.UpstreamModel = strings.Repeat("m", 64<<10)
			targetWork := int64(len(r.BaseURL)+len(r.UpstreamModel))*4 - 8<<10
			budget.deny = func(n int64) bool { return n == targetWork }
			client, _, calls, _, _ := executionClient(t, []byte("{}"), false, 2, 200)
			r.Client = client
			if _, err := New().Do(context.Background(), r); !errors.Is(err, deferred.ErrBudgetExceeded) || calls.Load() != 0 {
				t.Fatal("target construction preceded its variable metadata reservation", err)
			}
			if used, _, reservations := budget.stats(); used != 0 || len(reservations) != 3 || reservations[2] != targetWork {
				t.Fatalf("large target entered translation before ownership: used=%d calls=%v", used, reservations)
			}
		})
	}
}

func TestExecutionLimitsStructuralUnaryTranslationIsRequiredButUsageInspectionIsOptional(t *testing.T) {
	for _, c := range executionProviderCases() {
		t.Run(c.name, func(t *testing.T) {
			budget := &executionTestBudget{limit: 8 << 20}
			r := executionRequestFor(c, budget, false)
			raw := []byte(`{"ignored":` + strings.Repeat("[", deferred.JSONDepth) + "0" + strings.Repeat("]", deferred.JSONDepth) + "}")
			r.Client, _, _, _, _ = executionClient(t, raw, false, int64(len(raw)), 200)
			result, err := New().Do(context.Background(), r)
			if c.translation == "" {
				if err != nil || !bytes.Equal(result.Body, raw) || result.Usage.Has() {
					t.Fatal("optional inspection altered a passthrough result", err)
				}
				executionCloseResult(result)
			} else if !errors.Is(err, deferred.ErrBudgetExceeded) {
				t.Fatal("required translation decoded structurally oversized JSON", err)
			}
			if used, _, _ := budget.stats(); used != 0 {
				t.Fatal("structural response rejection retained allocations")
			}
		})
	}
}

func TestExecutionLimitsRequestBodyOwnershipSurvivesResultClose(t *testing.T) {
	budget := &executionTestBudget{limit: 4 << 20}
	r := executionRequestFor(executionProviderCases()[0], budget, false)
	sent := make(chan io.ReadCloser, 1)
	r.Client = &http.Client{Transport: executionRoundTripper(func(req *http.Request) (*http.Response, error) {
		sent <- req.Body
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader("{}")), ContentLength: 2}, nil
	})}
	result, err := New().Do(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	body := <-sent
	executionCloseResult(result)
	if used, _, _ := budget.stats(); used == 0 {
		t.Fatal("late-closing HTTP request body lost its work ownership")
	}
	if _, err := io.ReadAll(body); err != nil {
		t.Fatal(err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if used, _, _ := budget.stats(); used != 0 {
		t.Fatal("closing the last HTTP body did not drain work")
	}
}

func TestExecutionJSONStringUsesExactBoundedCapacity(t *testing.T) {
	for _, text := range []string{"", "<>&\x00\"\\\n\r\t\b\f", "😀\u2028\u2029\xff"} {
		expected, err := json.Marshal(text)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		out = *bytes.NewBuffer(make([]byte, 0, int(deferred.JSONStringBytes(text))))
		writeExecutionJSONString(&out, text)
		if !bytes.Equal(out.Bytes(), expected) || cap(out.Bytes()) != len(expected) {
			t.Fatalf("bounded string encoding changed bytes or grew capacity: %q != %q", out.Bytes(), expected)
		}
	}
}

func TestExecutionLimitsDoNotApplyToOtherOperationsAndRejectInvalidBudgets(t *testing.T) {
	budget := &executionTestBudget{limit: 4 << 20}
	r := executionRequestFor(executionProviderCases()[0], budget, false)
	r.Operation = OpResponses
	if _, err := New().Do(context.Background(), r); err == nil {
		t.Fatal("executor limits silently reached an unsupported operation")
	}
	for _, limits := range []ExecutionLimits{
		{}, {RequestBytes: 1024, ResponseBytes: 1024}, {RequestBytes: 1024, ResponseBytes: 1024, SnapshotBytes: -1, Work: budget},
		{RequestBytes: 1024, ResponseBytes: 1024, JSONDepth: deferred.JSONDepth + 1, Work: budget},
		{RequestBytes: 1024, ResponseBytes: 1024, SSEDataLines: deferred.SSEDataLines + 1, Work: budget},
	} {
		if _, err := newExecutionState(limits); err == nil {
			t.Fatal("invalid execution limits accepted")
		}
	}
	if _, _, calls := budget.stats(); len(calls) != 0 {
		t.Fatal("unsupported operation or invalid limits allocated work")
	}
}
