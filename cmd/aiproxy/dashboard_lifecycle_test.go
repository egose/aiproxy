package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/dashboard"
	"github.com/egose/aiproxy/internal/dashrpc"
)

type lifecycleProgram struct {
	done     chan struct{}
	closed   chan struct{}
	statuses chan dashboard.ConnectionStatus
	views    chan *dashboard.RuntimeSnapshot
	err      error
}

func newLifecycleProgram() *lifecycleProgram {
	return &lifecycleProgram{done: make(chan struct{}), closed: make(chan struct{}), statuses: make(chan dashboard.ConnectionStatus, 32), views: make(chan *dashboard.RuntimeSnapshot, 32)}
}

func (p *lifecycleProgram) Done() <-chan struct{}                   { return p.done }
func (p *lifecycleProgram) Wait() error                             { return p.err }
func (p *lifecycleProgram) Close()                                  { close(p.closed) }
func (p *lifecycleProgram) Refresh(s *dashboard.RuntimeSnapshot)    { p.views <- s }
func (p *lifecycleProgram) Connection(s dashboard.ConnectionStatus) { p.statuses <- s }

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for lifecycle event")
		var zero T
		return zero
	}
}

func TestDashboardCompletionCancelsHeldPoll(t *testing.T) {
	for _, completion := range []string{"quit", "program error", "context"} {
		t.Run(completion, func(t *testing.T) {
			started, canceled := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-r.Context().Done()
				close(canceled)
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := newLifecycleProgram()
			if completion == "program error" {
				p.err = errors.New("terminal initialization failed")
			}
			result := make(chan error, 1)
			go func() {
				result <- attachDashboard(ctx, dashrpc.Snapshot{}, &dashboardPayloadFetcher{}, dashboard.RunOptions{},
					func(ctx context.Context) (dashrpc.Snapshot, error) {
						return fetchSnapshot(ctx, server.Client(), server.URL, "fixture")
					},
					func(context.Context, *dashboard.RuntimeSnapshot, *dashboardPayloadFetcher, dashboard.RunOptions) dashboardProgram {
						return p
					}, 10*time.Millisecond)
			}()
			receive(t, started)
			if completion == "context" {
				cancel()
			} else {
				close(p.done)
			}
			if err := receive(t, result); !errors.Is(err, p.err) {
				t.Fatalf("completion error = %v, want %v", err, p.err)
			}
			receive(t, canceled)
			receive(t, p.closed)
		})
	}
}

func TestDashboardReconnectBackoffManualRetryAndDenial(t *testing.T) {
	var code atomic.Int32
	code.Store(http.StatusServiceUnavailable)
	requests := make(chan time.Time, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- time.Now()
		if status := int(code.Load()); status != 200 {
			http.Error(w, "secret response must not be displayed", status)
			return
		}
		_ = json.NewEncoder(w).Encode(dashrpc.Snapshot{Version: "recovered"})
	}))
	defer server.Close()
	p := newLifecycleProgram()
	f := &dashboardPayloadFetcher{client: dashrpc.NewClient(server.URL, "fixture")}
	options := make(chan dashboard.RunOptions, 1)
	result := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		result <- attachDashboard(ctx, dashrpc.Snapshot{Version: "initial"}, f, dashboard.RunOptions{},
			func(ctx context.Context) (dashrpc.Snapshot, error) {
				return fetchSnapshot(ctx, server.Client(), server.URL, "fixture")
			},
			func(_ context.Context, s *dashboard.RuntimeSnapshot, _ *dashboardPayloadFetcher, o dashboard.RunOptions) dashboardProgram {
				p.views <- s
				options <- o
				return p
			}, 40*time.Millisecond)
	}()
	o := receive(t, options)
	if s := receive(t, p.views); s.Version != "initial" {
		t.Fatal("initial view missing")
	}
	initial := receive(t, p.statuses)
	previous := time.Time{}
	for i := 0; i < 3; i++ {
		at := receive(t, requests)
		s := receive(t, p.statuses)
		if !s.Reconnecting || s.LastSuccess != initial.LastSuccess || s.RetryAt.IsZero() || strings.Contains(s.Message, "secret") {
			t.Fatalf("invalid reconnect status: %+v", s)
		}
		if i > 0 && at.Sub(previous) < (40*time.Millisecond<<uint(i-1))-5*time.Millisecond {
			t.Fatal("backoff hammered endpoint")
		}
		previous = at
		select {
		case <-p.views:
			t.Fatal("failure replaced stale snapshot")
		default:
		}
	}
	code.Store(200)
	o.Retry()
	receive(t, requests)
	if s := receive(t, p.views); s.Version != "recovered" {
		t.Fatal("live view not restored")
	}
	if s := receive(t, p.statuses); s.Reconnecting || !s.LastSuccess.After(initial.LastSuccess) {
		t.Fatalf("recovery status: %+v", s)
	}
	code.Store(403)
	receive(t, requests)
	if s := receive(t, p.statuses); !s.Denied || !s.RetryAt.IsZero() {
		t.Fatalf("denial status: %+v", s)
	}
	if _, err := f.ListPayloads(ctx, 1, false); !errors.Is(err, errDashboardDenied) {
		t.Fatalf("payload gate: %v", err)
	}
	if _, err := f.GetPayload(ctx, "id"); !errors.Is(err, errDashboardDenied) {
		t.Fatalf("detail gate: %v", err)
	}
	if _, err := f.ListBlocks(ctx); !errors.Is(err, errDashboardDenied) {
		t.Fatalf("block gate: %v", err)
	}
	if _, err := f.GetBlock(ctx, "id"); !errors.Is(err, errDashboardDenied) {
		t.Fatalf("capture gate: %v", err)
	}
	if _, err := f.DecideBlock(ctx, "id", "deny", []string{"hash"}); !errors.Is(err, errDashboardDenied) {
		t.Fatalf("decision gate: %v", err)
	}
	select {
	case <-requests:
		t.Fatal("denied endpoints still being polled")
	case <-time.After(180 * time.Millisecond):
	}
	code.Store(200)
	o.Retry()
	receive(t, requests)
	receive(t, p.views)
	receive(t, p.statuses)
	if f.denied.Load() {
		t.Fatal("recovery did not reopen pane access")
	}
	close(p.done)
	if err := receive(t, result); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requests:
		t.Fatal("poll survived quit")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDashboardRetryBoundAndClassification(t *testing.T) {
	delay := dashPollInterval
	for _, want := range []time.Duration{4, 8, 16, 30, 30, 30} {
		delay = nextDashboardRetry(delay)
		if delay != want*time.Second {
			t.Fatalf("delay %s, want %ds", delay, want)
		}
	}
	for _, code := range []int{301, 400, 401, 403, 404, 405, 408, 429, 500, 503} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "fixture-secret", code) }))
			defer server.Close()
			_, err := fetchSnapshot(context.Background(), server.Client(), server.URL, "fixture-secret")
			wantDenied := code < 500 && code != 408 && code != 429
			if err == nil || dashboardDenial(err) != wantDenied || strings.Contains(err.Error(), "fixture-secret") {
				t.Fatalf("status %d: %v", code, err)
			}
		})
	}
}

func TestDashboardManualRetryCoalescesDuringHeldRequest(t *testing.T) {
	p := newLifecycleProgram()
	options := make(chan dashboard.RunOptions, 1)
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	result := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		result <- attachDashboard(ctx, dashrpc.Snapshot{}, &dashboardPayloadFetcher{}, dashboard.RunOptions{},
			func(ctx context.Context) (dashrpc.Snapshot, error) {
				started <- struct{}{}
				select {
				case <-release:
					return dashrpc.Snapshot{}, nil
				case <-ctx.Done():
					return dashrpc.Snapshot{}, ctx.Err()
				}
			},
			func(_ context.Context, _ *dashboard.RuntimeSnapshot, _ *dashboardPayloadFetcher, o dashboard.RunOptions) dashboardProgram {
				options <- o
				return p
			}, time.Hour)
	}()
	o := receive(t, options)
	o.Retry()
	receive(t, started)
	for range 100 {
		o.Retry()
	}
	select {
	case <-started:
		t.Fatal("parallel retry worker")
	default:
	}
	close(release)
	receive(t, p.views)
	select {
	case <-started:
		t.Fatal("queued retry burst after success")
	case <-time.After(50 * time.Millisecond):
	}
	close(p.done)
	if err := receive(t, result); err != nil {
		t.Fatal(err)
	}
}

func TestDashboardImmediateProgramFailureStopsBeforePolling(t *testing.T) {
	p := newLifecycleProgram()
	p.err = errors.New("init failed")
	close(p.done)
	err := attachDashboard(context.Background(), dashrpc.Snapshot{}, &dashboardPayloadFetcher{}, dashboard.RunOptions{},
		func(context.Context) (dashrpc.Snapshot, error) {
			t.Error("polled after init failed")
			return dashrpc.Snapshot{}, nil
		},
		func(context.Context, *dashboard.RuntimeSnapshot, *dashboardPayloadFetcher, dashboard.RunOptions) dashboardProgram {
			return p
		}, time.Hour)
	if !errors.Is(err, p.err) {
		t.Fatalf("lost init failure: %v", err)
	}
	receive(t, p.closed)
}

func TestDashboardConfiguredIOAndProgramError(t *testing.T) {
	server := newSnapshotStub(t, "fixture")
	defer server.Close()
	cfg := writeDashboardConfig(t, `listener "http" "public" { address = "`+server.Listener.Addr().String()+`" }
auth "main" { mode = "none" }
dashboard { token = "fixture" }
`)
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runDashboardWithInput(ctx, cfg, true, strings.NewReader("q"), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("CLI waited for context instead of input quit")
	}
	if out.Len() == 0 || errOut.Len() != 0 {
		t.Fatalf("configured output routing: stdout=%d stderr=%s", out.Len(), errOut.String())
	}
}

func TestSnapshotCancellationAndRedirect(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() { _, err := fetchSnapshot(ctx, server.Client(), server.URL, "fixture"); result <- err }()
		receive(t, started)
		cancel()
		if err := receive(t, result); !errors.Is(err, context.Canceled) || !errors.Is(err, errTransport) {
			t.Fatalf("lost cancellation: %v", err)
		}
	})
	t.Run("redirect", func(t *testing.T) {
		var calls atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = io.WriteString(w, "{}") }))
		defer target.Close()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		}))
		defer server.Close()
		client := &http.Client{CheckRedirect: dashboardNoRedirect}
		defer client.CloseIdleConnections()
		_, err := fetchSnapshot(context.Background(), client, server.URL, "fixture")
		if !dashboardDenial(err) || calls.Load() != 0 {
			t.Fatalf("redirect followed: calls=%d, err=%v", calls.Load(), err)
		}
	})
}
