package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

func TestExecutionBodyFailureCooldownKeepsHeaderObservationDeadline(t *testing.T) {
	gate, observed := make(chan struct{}), make(chan time.Time, 1)
	release := sync.OnceFunc(func() { close(gate) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "100")
		w.Header().Set("Retry-After-Ms", "1500")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		select {
		case <-gate:
			_, _ = io.WriteString(w, "{")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(release)
	budget := &executionTestBudget{limit: 4 << 20}
	request := executionRequestFor(executionProviderCase{provider: config.ProviderTypeOpenAICompatible}, budget, false)
	request.BaseURL = server.URL
	request.ObserveBody = func(body io.ReadCloser) io.ReadCloser {
		observed <- time.Now()
		return body
	}
	done := make(chan error, 1)
	go func() {
		result, err := New().Do(context.Background(), request)
		executionCloseResult(result)
		done <- err
	}()
	var headerTime time.Time
	select {
	case headerTime = <-observed:
	case <-time.After(time.Second):
		t.Fatal("response headers were not observed during silent body buffering")
	}
	timer := time.NewTimer(80 * time.Millisecond)
	<-timer.C
	release()
	var failure error
	select {
	case failure = <-done:
	case <-time.After(time.Second):
		t.Fatal("partial unary response did not produce a definite read failure")
	}
	var cooldown *CooldownError
	if !errors.Is(failure, io.ErrUnexpectedEOF) || !errors.As(failure, &cooldown) || cooldown.Deadline.IsZero() ||
		cooldown.Deadline.After(headerTime.Add(1500*time.Millisecond)) || cooldown.Deadline.Before(headerTime.Add(1490*time.Millisecond)) {
		t.Fatalf("buffered body failure lost its header-time retry deadline: %v", failure)
	}
	remaining, ok := CooldownDelayFromError(failure)
	if !ok || remaining >= 1470*time.Millisecond {
		t.Fatal("buffered failure restarted the original cooldown duration")
	}
	timer = time.NewTimer(20 * time.Millisecond)
	<-timer.C
	later, ok := CooldownDelayFromError(failure)
	if !ok || later >= remaining || cooldown.Deadline.IsZero() {
		t.Fatal("repeated error observation renewed model cooldown")
	}
	if used, _, _ := budget.stats(); used != 0 {
		t.Fatal("failed unary buffering retained execution work")
	}
}

func TestObservedCooldownErrorDoesNotReviveExpiredAdvice(t *testing.T) {
	failure := withCooldownObservationError(io.ErrUnexpectedEOF, time.Second, true, time.Now().Add(-2*time.Second))
	if delay, ok := CooldownDelayFromError(failure); ok || delay != 0 {
		t.Fatal("late retrieval revived already elapsed model retry advice")
	}
}
