package app

import (
	"context"
	"errors"
	"time"

	"github.com/egose/aiproxy/internal/dbmerge"
)

func (a *App) refreshServingKeys() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	keys, revisions, confirmed, err := dbmerge.MergeKeys(ctx, a.adminStore)
	if err != nil {
		return err
	}
	a.handler.ApplyServingKeys(a.Config.Auth, keys, revisions, confirmed)
	return nil
}

func (a *App) ReconcileServing() error {
	a.reloadMu.Lock()
	if a.closed {
		a.reloadMu.Unlock()
		return errors.New("app is closed")
	}
	if a.adminStore == nil {
		a.reloadMu.Unlock()
		return nil
	}
	err := a.refreshServingKeys()
	deps := a.handler.SnapshotDependencies()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	revisions, readErr := a.adminStore.ServingRevisions(ctx)
	cancel()
	a.reloadMu.Unlock()
	if err != nil || readErr != nil {
		return errors.Join(err, readErr)
	}
	if revisions.Catalog != deps.ServingRevisions.Catalog {
		return a.Reload()
	}
	return nil
}

func (a *App) startServingReconciliation() {
	ctx, cancel := context.WithCancel(context.Background())
	a.reconcileCancel = cancel
	a.reconcileDone = make(chan struct{})
	go func() {
		defer close(a.reconcileDone)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := a.ReconcileServing(); err != nil {
					a.logger.Warn("database serving reconciliation failed", "error", err)
				}
			}
		}
	}()
}
