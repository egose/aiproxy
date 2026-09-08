package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

// STREAM-03 release note: translated Responses streams retain at most
// maxResponsesRetainedTextBytes of generated text per stream. The delta that
// would exceed the budget is neither retained nor emitted; translation fails
// with ErrResponsesOutputOverflow (no response.completed / [DONE]), upstream
// is closed, and opaque pass-through streams are unaffected.

// withResponsesTextBudget swaps the retained-text budget for a test and
// restores it afterwards. Production code never changes the budget; only
// tests override it for deterministic small-budget coverage.
func withResponsesTextBudget(t *testing.T, limit int) {
	t.Helper()
	prev := maxResponsesRetainedTextBytes
	maxResponsesRetainedTextBytes = limit
	t.Cleanup(func() { maxResponsesRetainedTextBytes = prev })
}

// discardScanner discards all bytes while recording total size and whether
// any written chunk (spanning chunk boundaries) contained a marker. Bounded
// memory: only a tail of len(longest marker)-1 bytes is retained.
type discardScanner struct {
	markers []string
	tail    []byte
	keep    int
	n       int64
	found   map[string]bool
}

func newDiscardScanner(markers ...string) *discardScanner {
	keep := 0
	for _, m := range markers {
		if len(m)-1 > keep {
			keep = len(m) - 1
		}
	}
	return &discardScanner{markers: markers, keep: keep, found: map[string]bool{}}
}

func (w *discardScanner) Write(p []byte) (int, error) {
	w.n += int64(len(p))
	scan := p
	if len(w.tail) > 0 {
		scan = append(append([]byte(nil), w.tail...), p...)
	}
	for _, m := range w.markers {
		if strings.Contains(string(scan), m) {
			w.found[m] = true
		}
	}
	combined := scan
	if len(combined) > w.keep {
		combined = combined[len(combined)-w.keep:]
	}
	w.tail = append(w.tail[:0], combined...)
	return len(p), nil
}

func TestResponsesAccumulatorAtLimitSucceeds(t *testing.T) {
	withResponsesTextBudget(t, 64)
	state := newResponsesStreamState("test/model", "resp_test")
	if err := state.appendText(strings.Repeat("a", 64)); err != nil {
		t.Fatalf("at-limit append: %v", err)
	}
	if got := state.text(); got != strings.Repeat("a", 64) {
		t.Fatalf("retained %d bytes, want 64", len(got))
	}
}

func TestResponsesAccumulatorOverflowNotRetained(t *testing.T) {
	withResponsesTextBudget(t, 64)
	state := newResponsesStreamState("test/model", "resp_test")
	if err := state.appendText(strings.Repeat("a", 64)); err != nil {
		t.Fatalf("at-limit append: %v", err)
	}
	err := state.appendText("b")
	var overflow ErrResponsesOutputOverflow
	if !errors.As(err, &overflow) || overflow.Limit != 64 {
		t.Fatalf("expected typed overflow with limit 64, got %T: %v", err, err)
	}
	if got := state.text(); got != strings.Repeat("a", 64) {
		t.Fatalf("overflowing delta retained: %d bytes", len(got))
	}
}

func anthropicResponsesDelta(text string) string {
	return "event: content_block_delta\n" + `data: {"delta":{"type":"text_delta","text":"` + text + `"}}` + "\n\n"
}

// overflowDelta builds a deterministic 16-byte distinct delta payload.
func overflowDelta(i int, tag string) string {
	s := fmt.Sprintf("d%02d-%s-", i, tag)
	return s + strings.Repeat("x", 16-len(s))
}

func geminiResponsesPayload(text string) string {
	return `data: {"candidates":[{"content":{"parts":[{"text":"` + text + `"}]}}]}` + "\n\n"
}

func TestAnthropicResponsesRetainedTextOverflow(t *testing.T) {
	withResponsesTextBudget(t, 64)
	deltas := []string{overflowDelta(0, "ok"), overflowDelta(1, "ok"), overflowDelta(2, "ok"), overflowDelta(3, "ok"), overflowDelta(4, "OVERFLOW")}
	for _, d := range deltas {
		if len(d) != 16 {
			t.Fatalf("test delta %q must be 16 bytes", d)
		}
	}
	var upstream strings.Builder
	upstream.WriteString("event: message_start\n" + `data: {"message":{"id":"msg_1"}}` + "\n\n")
	for _, d := range deltas {
		upstream.WriteString(anthropicResponsesDelta(d))
	}
	upstream.WriteString("event: message_stop\n" + "data: {}\n\n")

	src := &observedReadCloser{Reader: strings.NewReader(upstream.String()), closed: make(chan struct{})}
	stream := translateAnthropicResponsesStream(src, "test/model", NewStreamCompletion())
	defer stream.Close()

	out := newDiscardScanner(`"type":"response.completed"`, overflowDelta(4, "OVERFLOW"), overflowDelta(0, "ok"))
	_, err := io.Copy(out, stream)
	var overflow ErrResponsesOutputOverflow
	if !errors.As(err, &overflow) || overflow.Limit != 64 {
		t.Fatalf("expected typed overflow, got body-bytes=%d err=%v", out.n, err)
	}
	if out.found[`"type":"response.completed"`] {
		t.Fatal("overflowed stream emitted response.completed")
	}
	if out.found[overflowDelta(4, "OVERFLOW")] {
		t.Fatal("overflowing delta was emitted downstream")
	}
	if !out.found[overflowDelta(0, "ok")] {
		t.Fatal("pre-overflow delta missing downstream")
	}
	select {
	case <-src.closed:
	case <-time.After(time.Second):
		t.Fatal("upstream was not closed")
	}
}

func TestAnthropicResponsesRetainedTextAtLimitSucceeds(t *testing.T) {
	withResponsesTextBudget(t, 64)
	var upstream strings.Builder
	upstream.WriteString("event: message_start\n" + `data: {"message":{"id":"msg_1"}}` + "\n\n")
	for i := 0; i < 4; i++ {
		upstream.WriteString(anthropicResponsesDelta(overflowDelta(i, "ok")))
	}
	upstream.WriteString("event: message_stop\n" + "data: {}\n\n")

	src := &observedReadCloser{Reader: strings.NewReader(upstream.String()), closed: make(chan struct{})}
	stream := translateAnthropicResponsesStream(src, "test/model", NewStreamCompletion())
	defer stream.Close()

	out := newDiscardScanner(`"type":"response.completed"`)
	if _, err := io.Copy(out, stream); err != nil {
		t.Fatalf("at-limit stream must succeed, got %v", err)
	}
	if !out.found[`"type":"response.completed"`] {
		t.Fatal("at-limit stream missing response.completed")
	}
	select {
	case <-src.closed:
	case <-time.After(time.Second):
		t.Fatal("upstream was not closed")
	}
}

func TestGeminiResponsesRetainedTextOverflow(t *testing.T) {
	withResponsesTextBudget(t, 64)
	var upstream strings.Builder
	for i := 0; i < 4; i++ {
		upstream.WriteString(geminiResponsesPayload(overflowDelta(i, "ok")))
	}
	upstream.WriteString(geminiResponsesPayload(overflowDelta(4, "OVERFLOW")))
	upstream.WriteString(`data: {"candidates":[{"content":{"parts":[]},"finishReason":"STOP"}]}` + "\n\n")

	src := &observedReadCloser{Reader: strings.NewReader(upstream.String()), closed: make(chan struct{})}
	stream := translateGeminiResponsesStream(src, "test/model", NewStreamCompletion())
	defer stream.Close()

	out := newDiscardScanner(`"type":"response.completed"`, overflowDelta(4, "OVERFLOW"))
	_, err := io.Copy(out, stream)
	var overflow ErrResponsesOutputOverflow
	if !errors.As(err, &overflow) || overflow.Limit != 64 {
		t.Fatalf("expected typed overflow, got body-bytes=%d err=%v", out.n, err)
	}
	if out.found[`"type":"response.completed"`] {
		t.Fatal("overflowed stream emitted response.completed")
	}
	if out.found[overflowDelta(4, "OVERFLOW")] {
		t.Fatal("overflowing delta was emitted downstream")
	}
	select {
	case <-src.closed:
	case <-time.After(time.Second):
		t.Fatal("upstream was not closed")
	}
}

func TestOpenCodeResponsesRetainedTextOverflowReuse(t *testing.T) {
	withResponsesTextBudget(t, 64)
	var upstreamSSE strings.Builder
	upstreamSSE.WriteString("event: message_start\n" + `data: {"message":{"id":"msg_1"}}` + "\n\n")
	for i := 0; i < 8; i++ {
		upstreamSSE.WriteString(anthropicResponsesDelta(overflowDelta(i, "ok")))
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, upstreamSSE.String())
	}))
	defer upstream.Close()

	body := `{"model":"zen/claude","stream":true,"input":"hi"}`
	inbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	res, err := New().Do(context.Background(), Request{
		Operation:     OpResponses,
		ProviderType:  config.ProviderTypeOpenCodeZen,
		PublicModel:   "zen/m",
		BaseURL:       upstream.URL,
		APIKey:        "k",
		UpstreamModel: "m",
		ModelProtocol: config.ModelProtocolMessages,
		Body:          []byte(body),
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if !res.Streaming || res.StreamBody == nil {
		t.Fatal("expected streaming result")
	}
	defer res.StreamBody.Close()

	out := newDiscardScanner(`"type":"response.completed"`)
	_, readErr := io.Copy(out, res.StreamBody)
	var overflow ErrResponsesOutputOverflow
	if !errors.As(readErr, &overflow) {
		t.Fatalf("expected typed overflow, got %v", readErr)
	}
	if out.found[`"type":"response.completed"`] {
		t.Fatal("overflowed stream emitted response.completed")
	}
}

func TestOpaquePassThroughNotCappedByResponsesBudget(t *testing.T) {
	withResponsesTextBudget(t, 64)
	big := strings.Repeat("x", 4096)
	body := "data: " + big + "\n\ndata: [DONE]\n\n"
	src := newFragmentedReadCloser([]string{body})
	stream := newOpenAIStreamUsageReadCloser(src, NewStreamCompletion())
	defer stream.Close()

	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("pass-through must not fail under responses budget: %v", err)
	}
	if string(got) != body {
		t.Fatalf("pass-through bytes changed: %d vs %d", len(got), len(body))
	}
}

func buildAnthropicResponsesUpstream(totalText int, chunkText int) string {
	var upstream strings.Builder
	upstream.WriteString("event: message_start\n" + `data: {"message":{"id":"msg_1"}}` + "\n\n")
	chunk := strings.Repeat("a", chunkText)
	for done := 0; done < totalText; done += chunkText {
		n := chunkText
		if done+n > totalText {
			n = totalText - done
		}
		upstream.WriteString(anthropicResponsesDelta(chunk[:n]))
	}
	upstream.WriteString("event: message_stop\n" + "data: {}\n\n")
	return upstream.String()
}

func BenchmarkResponsesRetainedText(b *testing.B) {
	budget := maxResponsesRetainedTextBytes
	cases := []struct {
		name      string
		totalText int
		wantErr   bool
	}{
		{"AtLimit", budget, false},
		{"Overflow2x", 2 * budget, true},
		{"Overflow4x", 4 * budget, true},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			upstream := buildAnthropicResponsesUpstream(tc.totalText, 4096)
			b.SetBytes(int64(len(upstream)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				src := io.NopCloser(strings.NewReader(upstream))
				stream := translateAnthropicResponsesStream(src, "test/model", NewStreamCompletion())
				_, err := io.Copy(io.Discard, stream)
				_ = stream.Close()
				var overflow ErrResponsesOutputOverflow
				if tc.wantErr && !errors.As(err, &overflow) {
					b.Fatalf("expected typed overflow, got %v", err)
				}
				if !tc.wantErr && err != nil {
					b.Fatalf("at-limit must succeed, got %v", err)
				}
			}
		})
	}
}
