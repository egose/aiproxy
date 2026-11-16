package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egose/aiproxy/internal/deferred"
)

func TestExecutionSSELimitsRequiredTranslationFailsOptionalInspectionPreservesBytes(t *testing.T) {
	for _, c := range executionProviderCases() {
		for _, kind := range []string{"line", "event", "data_lines", "depth", "tokens"} {
			t.Run(c.name+"/"+kind, func(t *testing.T) {
				var raw []byte
				switch kind {
				case "line":
					raw = []byte("data: " + strings.Repeat("x", deferred.SSELineBytes) + "\n\n")
				case "event":
					raw = []byte(strings.Repeat("data: "+strings.Repeat("x", 220<<10)+"\n", 5) + "\n")
				case "data_lines":
					raw = []byte(strings.Repeat("data: \n", deferred.SSEDataLines+1) + "\n")
				case "depth":
					raw = []byte("data: {\"x\":" + strings.Repeat("[", deferred.JSONDepth) + "0" + strings.Repeat("]", deferred.JSONDepth) + "}\n\n")
				case "tokens":
					raw = []byte("data: {\"x\":[" + strings.Repeat("0,", deferred.JSONTokens) + "0]}\n\n")
				}
				raw = append(raw, executionResponseFor(c, true)...)
				budget := &executionTestBudget{limit: 32 << 20}
				r := executionRequestFor(c, budget, true)
				r.ExecutionLimits.ResponseBytes = 4 << 20
				client, source, _, _, _ := executionClient(t, raw, true, int64(len(raw)), 200)
				r.Client = client
				result, err := New().Do(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				body, readErr := executionReadResult(result)
				if c.translation == "" {
					if readErr != nil || !bytes.Equal(body, raw) || result.Stream.Wait().Err != nil {
						t.Fatal("optional inspection changed delivered bytes/outcome", readErr)
					}
				} else {
					var limit ErrExecutionLimit
					if !errors.As(readErr, &limit) || bytes.Contains(body, []byte("[DONE]")) {
						t.Fatal("required parser overflow became a successful stream", readErr)
					}
				}
				executionCloseResult(result)
				budget.awaitReleased(t)
				if _, _, _, closes := source.stats(); closes != 1 {
					t.Fatalf("overflow closed source %d times", closes)
				}
				if _, peak, _ := budget.stats(); peak > budget.limit {
					t.Fatal("SSE overflow exceeded reserved work")
				}
			})
		}
	}
}

func TestExecutionSSEBufferLineEventAndDataLineBoundariesHaveFixedCapacity(t *testing.T) {
	for _, kind := range []string{"line", "event", "data_lines"} {
		t.Run(kind, func(t *testing.T) {
			budget := &executionTestBudget{limit: 8 << 20}
			state, err := newExecutionState(ExecutionLimits{RequestBytes: 1024, ResponseBytes: 2 << 20, Work: budget})
			if err != nil {
				t.Fatal(err)
			}
			defer state.release()
			buffer, err := newExecutionSSEBuffer(state)
			if err != nil {
				t.Fatal(err)
			}
			defer buffer.close()
			switch kind {
			case "line":
				if err := buffer.appendLine(bytes.Repeat([]byte("x"), deferred.SSELineBytes)); err != nil {
					t.Fatal(err)
				}
				if err := buffer.appendLine([]byte("x")); !errors.Is(err, deferred.ErrBudgetExceeded) {
					t.Fatal("line boundary appended an oversized byte", err)
				}
			case "event":
				for i := 0; i < 5; i++ {
					buffer.line = append(buffer.line[:0], []byte("data: ")...)
					size := min(220<<10, deferred.SSEEventBytes-len(buffer.data))
					if i > 0 && size == deferred.SSEEventBytes-len(buffer.data) {
						size--
					}
					buffer.line = append(buffer.line, bytes.Repeat([]byte("x"), size)...)
					if _, err := buffer.processLine(); err != nil {
						t.Fatal(err)
					}
				}
				if len(buffer.data) != deferred.SSEEventBytes {
					t.Fatal("maximum event was not accepted")
				}
				buffer.line = append(buffer.line[:0], []byte("data:")...)
				if _, err := buffer.processLine(); !errors.Is(err, deferred.ErrBudgetExceeded) {
					t.Fatal("event boundary appended beyond capacity", err)
				}
			case "data_lines":
				for range deferred.SSEDataLines {
					buffer.line = append(buffer.line[:0], []byte("data:")...)
					if _, err := buffer.processLine(); err != nil {
						t.Fatal(err)
					}
				}
				buffer.line = append(buffer.line[:0], []byte("data:")...)
				if _, err := buffer.processLine(); !errors.Is(err, deferred.ErrBudgetExceeded) {
					t.Fatal("data line count overflow was accepted", err)
				}
			}
			if cap(buffer.line) != deferred.SSELineBytes || cap(buffer.data) != deferred.SSEEventBytes {
				t.Fatal("parser grew an unmetered buffer")
			}
		})
	}
}

func TestExecutionSSEParserReservationPrecedesReadAndEncodedChunkPrecedesWrite(t *testing.T) {
	for _, c := range executionProviderCases() {
		if c.translation == "" {
			continue
		}
		t.Run(c.name+"/parser", func(t *testing.T) {
			budget := &executionTestBudget{limit: 8 << 20}
			denied := int64(deferred.SSELineBytes)*2 + int64(deferred.SSEEventBytes)*2 + 16<<10
			budget.deny = func(n int64) bool { return n == denied }
			r := executionRequestFor(c, budget, true)
			raw := executionResponseFor(c, true)
			client, source, _, _, _ := executionClient(t, raw, true, int64(len(raw)), 200)
			r.Client = client
			_, err := New().Do(context.Background(), r)
			if !errors.Is(err, deferred.ErrBudgetExceeded) {
				t.Fatal("required SSE parser allocated without credit", err)
			}
			if reads, _, _, closes := source.stats(); reads != 0 || closes != 1 {
				t.Fatalf("failed parser reservation read or leaked source: reads=%d closes=%d", reads, closes)
			}
			if used, _, _ := budget.stats(); used != 0 {
				t.Fatal("failed parser setup leaked ownership")
			}
		})
		t.Run(c.name+"/encoding", func(t *testing.T) {
			budget := &executionTestBudget{limit: 8 << 20}
			r := executionRequestFor(c, budget, true)
			r.ExecutionLimits.ResponseBytes = 64
			raw := executionResponseFor(c, true)
			r.Client, _, _, _, _ = executionClient(t, raw, true, int64(len(raw)), 200)
			result, err := New().Do(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			body, err := executionReadResult(result)
			if !errors.Is(err, deferred.ErrBudgetExceeded) || len(body) != 0 {
				t.Fatalf("oversized chunk committed a partial frame: bytes=%d err=%v", len(body), err)
			}
			executionCloseResult(result)
			budget.awaitReleased(t)
		})
	}
}

func TestExecutionPassthroughResultLimitIsNotAnOptionalInspectionFailure(t *testing.T) {
	budget := &executionTestBudget{limit: 4 << 20}
	r := executionRequestFor(executionProviderCases()[0], budget, true)
	r.ExecutionLimits.ResponseBytes = 128
	raw := []byte(strings.Repeat(": comment\n", 20))
	client, source, _, _, _ := executionClient(t, raw, true, int64(len(raw)), 200)
	r.Client = client
	result, err := New().Do(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	body, err := executionReadResult(result)
	if !errors.Is(err, deferred.ErrBudgetExceeded) || len(body) != 128 || !bytes.Equal(body, raw[:128]) {
		t.Fatal("result byte overflow was silently truncated or exceeded the ceiling", err)
	}
	if _, count, largest, _ := source.stats(); count > 129 || largest > 129 {
		t.Fatal("stream read past the bounded overflow probe")
	}
	executionCloseResult(result)
}

func TestExecutionPassthroughNativeErrorTextIsBoundedWithoutChangingBody(t *testing.T) {
	budget := &executionTestBudget{limit: 4 << 20}
	r := executionRequestFor(executionProviderCases()[0], budget, true)
	r.ExecutionLimits.ResponseBytes = 256 << 10
	raw := []byte("data: {\"error\":{\"type\":\"upstream_error\",\"message\":\"" + strings.Repeat("x", 64<<10) + "\"}}\n\n")
	r.Client, _, _, _, _ = executionClient(t, raw, true, int64(len(raw)), 200)
	result, err := New().Do(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	body, err := executionReadResult(result)
	if err != nil || !bytes.Equal(body, raw) {
		t.Fatal("native error observation changed passthrough bytes", err)
	}
	if outcome := result.Stream.Wait(); outcome.Err == nil || len(outcome.Err.Error()) > deferred.MaxErrorBytes {
		t.Fatal("observed native error retained an unbounded message")
	}
	executionCloseResult(result)
}

type executionBlockedSource struct {
	entered chan struct{}
	done    chan struct{}
	gate    chan struct{}
	once    sync.Once
	close   sync.Once
}

func (b *executionBlockedSource) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.gate
	close(b.done)
	return 0, io.EOF
}

func (b *executionBlockedSource) Close() error {
	b.close.Do(func() { close(b.gate) })
	return nil
}

type executionCleanupBarrierBudget struct {
	*executionTestBudget
	entered chan struct{}
	gate    chan struct{}
}

func (b *executionCleanupBarrierBudget) Reserve(n int64) (func(), error) {
	release, err := b.executionTestBudget.Reserve(n)
	if err != nil || n != 4<<10 {
		return release, err
	}
	return sync.OnceFunc(func() {
		close(b.entered)
		<-b.gate
		release()
	}), nil
}

func TestExecutionSSECloseKeepsBudgetUntilTranslatorCleanup(t *testing.T) {
	for _, c := range executionProviderCases() {
		if c.translation == "" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				budget := &executionTestBudget{limit: 4 << 20}
				barrier := &executionCleanupBarrierBudget{
					executionTestBudget: budget, entered: make(chan struct{}), gate: make(chan struct{}),
				}
				unblock := sync.OnceFunc(func() { close(barrier.gate) })
				defer unblock()
				r := executionRequestFor(c, budget, true)
				r.ExecutionLimits.Work = barrier
				raw := executionResponseFor(c, true)
				client, source, _, _, _ := executionClient(t, raw, true, int64(len(raw)), 200)
				r.Client = client
				result, err := New().Do(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				defer executionCloseResult(result)
				reader := result.StreamBody
				readDone := make(chan error, 1)
				go func() {
					_, err := io.Copy(io.Discard, reader)
					readDone <- err
				}()
				select {
				case <-barrier.entered:
				case <-time.After(2 * time.Second):
					t.Fatal("translator did not reach decoder cleanup")
				}
				beforeClose, _, _ := budget.stats()
				if beforeClose <= 4<<10 {
					t.Fatal("translator lost its retained execution ownership")
				}
				closeDone := make(chan struct{})
				go func() {
					executionCloseResult(result)
					close(closeDone)
				}()
				select {
				case <-closeDone:
				case <-time.After(2 * time.Second):
					t.Fatal("result close waited for translator cleanup")
				}
				select {
				case err := <-readDone:
					if !errors.Is(err, io.ErrClosedPipe) {
						t.Fatal("closing the result did not interrupt the pipe reader", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("result close left its pipe reader blocked")
				}
				if used, _, _ := budget.stats(); used != beforeClose {
					t.Fatalf("result close freed live translator ownership: used=%d want=%d", used, beforeClose)
				}
				if _, _, _, closes := source.stats(); closes != 1 {
					t.Fatalf("result close closed source %d times", closes)
				}
				drainDone := make(chan struct{})
				go func() {
					defer close(drainDone)
					budget.awaitReleased(t)
				}()
				synctest.Wait()
				select {
				case <-drainDone:
					t.Error("drain check finished while the translator still owned its budget")
				default:
				}
				unblock()
				synctest.Wait()
				select {
				case <-drainDone:
				default:
					t.Fatal("translator cleanup did not finish its drain check")
				}
				if used, _, _ := budget.stats(); used != 0 {
					t.Fatalf("translator cleanup retained %d reserved bytes", used)
				}
				if _, _, _, closes := source.stats(); closes != 1 {
					t.Fatalf("translator cleanup closed source %d times", closes)
				}
			})
		})
	}
}

func TestExecutionSSECloseDuringBlockedReadKeepsBudgetUntilReaderAndWorkerExit(t *testing.T) {
	for _, c := range executionProviderCases() {
		t.Run(c.name, func(t *testing.T) {
			budget := &executionTestBudget{limit: 4 << 20}
			r := executionRequestFor(c, budget, true)
			source := &executionBlockedSource{entered: make(chan struct{}), done: make(chan struct{}), gate: make(chan struct{})}
			r.Client = &http.Client{Transport: executionRoundTripper(func(req *http.Request) (*http.Response, error) {
				_ = req.Body.Close()
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: source, ContentLength: -1}, nil
			})}
			result, err := New().Do(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			reader := result.StreamBody
			readDone := make(chan error, 1)
			go func() {
				_, err := io.Copy(io.Discard, reader)
				readDone <- err
			}()
			<-source.entered
			executionCloseResult(result)
			<-source.done
			<-readDone
			budget.awaitReleased(t)
		})
	}
}

func TestExecutionSSEMinimumWorkBudgetAcceptsNormalStreams(t *testing.T) {
	for _, c := range executionProviderCases() {
		t.Run(c.name, func(t *testing.T) {
			budget := &executionTestBudget{limit: 4 << 20}
			r := executionRequestFor(c, budget, true)
			raw := executionResponseFor(c, true)
			r.Client, _, _, _, _ = executionClient(t, raw, true, int64(len(raw)), 200)
			result, err := New().Do(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := executionReadResult(result); err != nil {
				t.Fatalf("minimum supported work reservation rejected a bounded normal stream: %v", err)
			}
			executionCloseResult(result)
			budget.awaitReleased(t)
			if _, peak, _ := budget.stats(); peak > 4<<20 {
				t.Fatal(fmt.Sprintf("minimum work ceiling exceeded: %d", peak))
			}
		})
	}
}
