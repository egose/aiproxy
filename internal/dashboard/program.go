package dashboard

import (
	"context"
	"fmt"
	"io"
	"time"

	"charm.land/bubbletea/v2"
)

type Program struct {
	program *tea.Program
	cancel  context.CancelFunc
	done    chan struct{}
	err     error
}

type RefreshHook interface {
	Refresh(snap *RuntimeSnapshot)
}

type RunOptions struct {
	Input          io.Reader
	Output         io.Writer
	Retry          func()
	SignalsHandled bool
}

type ConnectionStatus struct {
	LastSuccess  time.Time
	RetryAt      time.Time
	Reconnecting bool
	Denied       bool
	Message      string
}

func (s ConnectionStatus) text() string {
	state := "LIVE"
	if s.Denied {
		state = "DENIED"
	} else if s.Reconnecting {
		state = "RECONNECTING"
	}
	text := fmt.Sprintf("%s · last OK %s · Ctrl+R retry", state, s.LastSuccess.Local().Format("15:04:05"))
	if !s.RetryAt.IsZero() {
		text += " · next " + s.RetryAt.Local().Format("15:04:05")
	}
	if s.Message != "" {
		text += " · " + s.Message
	}
	return text
}

func Run(ctx context.Context, snap *RuntimeSnapshot, fetcher PayloadFetcher) *Program {
	return RunWithBlockFetcher(ctx, snap, fetcher, nil)
}

func RunWithBlockFetcher(ctx context.Context, snap *RuntimeSnapshot, fetcher PayloadFetcher, blocks BlockFetcher) *Program {
	return RunWithOptions(ctx, snap, fetcher, blocks, RunOptions{})
}

func RunWithOptions(ctx context.Context, snap *RuntimeSnapshot, fetcher PayloadFetcher, blocks BlockFetcher, options RunOptions) *Program {
	m := InitialModelWithBlockFetcher(snap, fetcher, blocks).(*model)
	m.retry = options.Retry
	opts := []tea.ProgramOption{}
	if options.Input != nil {
		opts = append(opts, tea.WithInput(options.Input))
	}
	if options.Output != nil {
		opts = append(opts, tea.WithOutput(options.Output))
	}
	if options.SignalsHandled {
		opts = append(opts, tea.WithoutSignalHandler())
	}
	return startProgram(ctx, m, opts...)
}

func startProgram(parent context.Context, m *model, opts ...tea.ProgramOption) *Program {
	ctx, cancel := context.WithCancel(parent)
	m.ctx = ctx
	opts = append(opts, tea.WithContext(ctx))
	p := &Program{program: tea.NewProgram(m, opts...), cancel: cancel, done: make(chan struct{})}
	go func() {
		_, p.err = p.program.Run()
		cancel()
		close(p.done)
	}()
	return p
}

func (p *Program) Done() <-chan struct{} { return p.done }

func (p *Program) Wait() error {
	<-p.done
	return p.err
}

func (p *Program) Refresh(snap *RuntimeSnapshot) {
	p.program.Send(snapshotMsg{snapshot: snap})
}

func (p *Program) RefreshError(err error) {
	if err != nil {
		p.program.Send(pollErrorMsg{err: err.Error(), at: time.Now()})
	}
}

func (p *Program) Connection(status ConnectionStatus) {
	p.program.Send(status)
}

func (p *Program) Close() {
	p.cancel()
	<-p.done
}

func fetchContext(parents []context.Context) (context.Context, context.CancelFunc) {
	parent := context.Background()
	if len(parents) > 0 && parents[0] != nil {
		parent = parents[0]
	}
	return context.WithTimeout(parent, 10*time.Second)
}
