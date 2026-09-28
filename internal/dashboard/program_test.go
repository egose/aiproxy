package dashboard

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/dashrpc"
)

func waitProgram(t *testing.T, p *Program) error {
	t.Helper()
	select {
	case <-p.Done():
		return p.Wait()
	case <-time.After(3 * time.Second):
		t.Fatal("Program did not finish")
		return nil
	}
}

func TestProgramQuitCompletesAndCancelsLifetime(t *testing.T) {
	for _, key := range []string{"q", "\x1b", "\x03"} {
		t.Run(key, func(t *testing.T) {
			m := InitialModel(newSnapshot()).(*model)
			p := startProgram(context.Background(), m, tea.WithInput(strings.NewReader(key)), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
			if err := waitProgram(t, p); err != nil {
				t.Fatal(err)
			}
			if !m.quit || m.ctx.Err() != context.Canceled {
				t.Fatalf("quit=%v, lifetime=%v", m.quit, m.ctx.Err())
			}
			p.Close()
			p.Close()
			p.Refresh(newSnapshot())
			p.RefreshError(errors.New("late"))
			p.Connection(ConnectionStatus{})
		})
	}
}

func TestProgramInitializationErrorIsObservable(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "closed-input")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	m := InitialModel(newSnapshot()).(*model)
	p := startProgram(context.Background(), m, tea.WithInput(f), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	if err := waitProgram(t, p); err == nil {
		t.Fatal("expected closed input initialization error")
	}
	if m.ctx.Err() != context.Canceled {
		t.Fatal("initialization failure did not cancel lifetime")
	}
	p.Close()
}

type lifetimeFetcher struct {
	started chan context.Context
}

func (f *lifetimeFetcher) hold(ctx context.Context) error {
	f.started <- ctx
	<-ctx.Done()
	return ctx.Err()
}

func (f *lifetimeFetcher) ListPayloads(ctx context.Context, _ int, _ bool) (dashrpc.PayloadList, error) {
	return dashrpc.PayloadList{}, f.hold(ctx)
}

func (f *lifetimeFetcher) GetPayload(ctx context.Context, _ string) (string, error) {
	return "", f.hold(ctx)
}

func (f *lifetimeFetcher) ListBlocks(ctx context.Context) (dashrpc.BlockList, error) {
	return dashrpc.BlockList{}, f.hold(ctx)
}

func (f *lifetimeFetcher) GetBlock(ctx context.Context, _ string) (dashrpc.BlockCapture, error) {
	return dashrpc.BlockCapture{}, f.hold(ctx)
}

func (f *lifetimeFetcher) DecideBlock(ctx context.Context, _, _ string, _ []string) (dashrpc.BlockDecisionResponse, error) {
	return dashrpc.BlockDecisionResponse{}, f.hold(ctx)
}

func TestProgramCancellationReachesEveryPaneCommand(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &lifetimeFetcher{started: make(chan context.Context, 5)}
	m := InitialModelWithBlockFetcher(newSnapshot(), f, f).(*model)
	p := startProgram(parent, m, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	cmds := []tea.Cmd{
		fetchPayloadsCmd(f, 100, false, m.ctx), fetchPayloadDetailCmd(f, "request", m.ctx),
		fetchBlocksCmd(f, m.ctx), fetchBlockDetailCmd(f, "block", m.ctx),
		fetchBlockDecisionCmd(f, "block", "deny", []string{"hash"}, m.ctx),
	}
	finished := make(chan struct{}, len(cmds))
	for _, cmd := range cmds {
		go func() { cmd(); finished <- struct{}{} }()
	}
	for range cmds {
		select {
		case ctx := <-f.started:
			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 10*time.Second {
				t.Fatal("missing bounded fetch deadline")
			}
		case <-time.After(time.Second):
			t.Fatal("pane fetch not started")
		}
	}
	cancel()
	if err := waitProgram(t, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost context error: %v", err)
	}
	for range cmds {
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("pane work survived Program cancellation")
		}
	}
}

func TestConnectionStatusPreservesViewAndManualRetry(t *testing.T) {
	m := InitialModel(newSnapshot()).(*model)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	snap := m.snapshot
	last := time.Now().Add(-time.Minute)
	m.Update(ConnectionStatus{LastSuccess: last, Reconnecting: true, RetryAt: time.Now().Add(4 * time.Second)})
	view := m.View().Content
	if m.snapshot != snap || !strings.Contains(view, "RECONNECTING") || !strings.Contains(view, "last OK "+last.Local().Format("15:04:05")) || !strings.Contains(view, "Ctrl+R") {
		t.Fatalf("stale view/status missing: %s", view)
	}
	count := 0
	m.retry = func() { count++ }
	m.Update(tea.KeyPressMsg(tea.Key{Text: "ctrl+r"}))
	if count != 1 {
		t.Fatal("manual retry callback not invoked")
	}
	m.paused = true
	m.Update(ConnectionStatus{LastSuccess: last, Denied: true, Message: "access forbidden"})
	if view := m.View().Content; !strings.Contains(view, "DENIED/PAUSED") || !strings.Contains(view, "access forbidden") {
		t.Fatal("connection status hidden by pause")
	}
	m.Update(ConnectionStatus{LastSuccess: time.Now()})
	if strings.Contains(m.View().Content, "DENIED") {
		t.Fatal("connection did not recover")
	}
}
