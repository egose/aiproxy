package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/egose/aiproxy/internal/dashboard"
	"github.com/egose/aiproxy/internal/dashrpc"
)

type dashboardProgram interface {
	Done() <-chan struct{}
	Wait() error
	Close()
	Refresh(*dashboard.RuntimeSnapshot)
	Connection(dashboard.ConnectionStatus)
}

type dashboardStarter func(context.Context, *dashboard.RuntimeSnapshot, *dashboardPayloadFetcher, dashboard.RunOptions) dashboardProgram

func startDashboardProgram(ctx context.Context, snap *dashboard.RuntimeSnapshot, f *dashboardPayloadFetcher, options dashboard.RunOptions) dashboardProgram {
	return dashboard.RunWithOptions(ctx, snap, f, f, options)
}

func attachDashboard(parent context.Context, initial dashrpc.Snapshot, f *dashboardPayloadFetcher, options dashboard.RunOptions,
	fetch func(context.Context) (dashrpc.Snapshot, error), start dashboardStarter, interval time.Duration) error {
	ctx, cancel := context.WithCancel(parent)
	retry := make(chan struct{}, 1)
	options.Retry = func() {
		select {
		case retry <- struct{}{}:
		default:
		}
	}
	prog := start(ctx, dashboard.SnapshotFromTransport(initial), f, options)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		pollDashboard(ctx, prog, f, fetch, retry, interval)
	}()
	defer func() {
		cancel()
		prog.Close()
		<-stopped
	}()
	select {
	case <-parent.Done():
		return nil
	case <-prog.Done():
		if parent.Err() != nil {
			return nil
		}
		return prog.Wait()
	}
}

func pollDashboard(ctx context.Context, prog dashboardProgram, f *dashboardPayloadFetcher,
	fetch func(context.Context) (dashrpc.Snapshot, error), retry <-chan struct{}, interval time.Duration) {
	status := dashboard.ConnectionStatus{LastSuccess: time.Now()}
	prog.Connection(status)
	timer := time.NewTimer(interval)
	defer timer.Stop()
	delay := interval
	for {
		select {
		case <-ctx.Done():
			return
		case <-prog.Done():
			return
		case <-retry:
			timer.Stop()
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		updated, err := fetch(ctx)
		if ctx.Err() != nil {
			return
		}
		select {
		case <-retry:
		default:
		}
		if err == nil {
			f.denied.Store(false)
			status = dashboard.ConnectionStatus{LastSuccess: time.Now()}
			prog.Refresh(dashboard.SnapshotFromTransport(updated))
			delay = interval
			timer.Reset(interval)
		} else {
			status.Denied = dashboardDenial(err)
			if status.Denied {
				f.denied.Store(true)
			}
			status.Reconnecting = !status.Denied
			status.RetryAt = time.Time{}
			status.Message = dashboardRecoveryMessage(err)
			if !status.Denied {
				status.RetryAt = time.Now().Add(delay)
				timer.Reset(delay)
				delay = nextDashboardRetry(delay)
			}
		}
		prog.Connection(status)
	}
}

func nextDashboardRetry(delay time.Duration) time.Duration {
	if delay >= 15*time.Second {
		return 30 * time.Second
	}
	return delay * 2
}

type snapshotStatusError int

func (e snapshotStatusError) Error() string {
	return fmt.Sprintf("snapshot endpoint returned %d", int(e))
}

func dashboardDenial(err error) bool {
	if errors.Is(err, errDashboardUnauthorized) || errors.Is(err, errDashboardForbidden) || errors.Is(err, errDashboardUnconfigured) {
		return true
	}
	var status snapshotStatusError
	return errors.As(err, &status) && status < 500 && status != http.StatusRequestTimeout && status != http.StatusTooManyRequests
}

func dashboardRecoveryMessage(err error) string {
	switch {
	case errors.Is(err, errDashboardUnauthorized):
		return "token mismatch; re-attach to reload credentials"
	case errors.Is(err, errDashboardForbidden):
		return "access forbidden; fix server then retry"
	case errors.Is(err, errDashboardUnconfigured):
		return "dashboard not configured; fix/restart server then retry"
	case dashboardDenial(err):
		return "endpoint denied; fix server then retry"
	default:
		return "snapshot unavailable; retaining last view"
	}
}

func dashboardNoRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}
