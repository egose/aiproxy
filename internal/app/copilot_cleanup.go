package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/store"
)

const copilotFlowCleanupInterval = time.Minute

const copilotFlowCleanupBatch = 100

type copilotFlowCleanup struct {
	stopCh chan struct{}
	doneCh chan struct{}
	once   sync.Once
}

func startCopilotFlowCleanup(st *store.Store, logger *slog.Logger) *copilotFlowCleanup {
	c := &copilotFlowCleanup{stopCh: make(chan struct{}), doneCh: make(chan struct{})}
	go c.run(st, logger)
	return c
}

func (c *copilotFlowCleanup) run(st *store.Store, logger *slog.Logger) {
	defer close(c.doneCh)
	ticker := time.NewTicker(copilotFlowCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err := st.CleanupCopilotFlows(ctx, copilotFlowCleanupBatch)
			cancel()
			if err != nil && logger != nil {
				logger.Error("copilot device flow cleanup failed", "error", err)
			}
		}
	}
}

func (c *copilotFlowCleanup) stop() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		close(c.stopCh)
		<-c.doneCh
	})
}

func (a *App) SetCopilotDeviceClient(factory func() *copilotlogin.Client) {
	a.mu.Lock()
	a.copilotDeviceClient = factory
	a.mu.Unlock()
	deps := a.handler.SnapshotDependencies()
	deps.CopilotDeviceClient = factory
	a.handler.UpdateDependencies(deps)
}
