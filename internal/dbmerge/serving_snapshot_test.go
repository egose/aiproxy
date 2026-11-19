package dbmerge

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/uptrace/bun"
)

type snapshotMutationHook struct {
	fired  atomic.Bool
	mutate func()
}

func (h *snapshotMutationHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	return ctx
}
func (h *snapshotMutationHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	if strings.Contains(event.Query, "serving_revisions") && h.fired.CompareAndSwap(false, true) {
		h.mutate()
	}
}

func TestServingSnapshotRejectsMixedReads(t *testing.T) {
	for _, keysOnly := range []bool{false, true} {
		st := openReasoningMergeStore(t)
		ctx := context.Background()
		hook := &snapshotMutationHook{mutate: func() {
			if _, err := st.DB.ExecContext(ctx, "UPDATE inbound_keys SET enabled = FALSE"); err != nil {
				t.Fatal(err)
			}
		}}
		st.DB.AddQueryHook(hook)
		var err error
		if keysOnly {
			_, _, _, err = MergeKeys(ctx, st)
		} else {
			_, err = MergeCatalog(ctx, st, &config.Runtime{})
		}
		if !hook.fired.Load() || !errors.Is(err, ErrSnapshotChanged) {
			t.Fatalf("mixed snapshot certified: keysOnly=%v hook=%v err=%v", keysOnly, hook.fired.Load(), err)
		}
		if keysOnly {
			_, _, _, err = MergeKeys(ctx, st)
		} else {
			_, err = MergeCatalog(ctx, st, &config.Runtime{})
		}
		if err != nil {
			t.Fatal("stable snapshot did not recover", err)
		}
	}
}

func TestServingKeysDoNotCertifyMixedCatalogRevision(t *testing.T) {
	st := openReasoningMergeStore(t)
	ctx := context.Background()
	hook := &snapshotMutationHook{mutate: func() {
		if _, err := st.DB.ExecContext(ctx, "UPDATE db_providers SET enabled = FALSE"); err != nil {
			t.Fatal(err)
		}
	}}
	st.DB.AddQueryHook(hook)
	_, revisions, _, err := MergeKeys(ctx, st)
	if err != nil || revisions.Keys == 0 || revisions.Catalog != 0 {
		t.Fatal("independent keys certified a mixed catalog or failed key refresh", revisions, err)
	}
}
