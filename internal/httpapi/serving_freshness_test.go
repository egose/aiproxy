package httpapi

import (
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func TestServingFreshnessDeadlineAndOrdering(t *testing.T) {
	confirmed := time.Now()
	if !servingFreshAt(confirmed, confirmed.Add(30*time.Second-time.Nanosecond)) || servingFreshAt(confirmed, confirmed.Add(30*time.Second)) || servingFreshAt(time.Time{}, confirmed) || servingFreshAt(confirmed, confirmed.Add(-time.Nanosecond)) {
		t.Fatal("deadline boundary is not fail closed")
	}
	h := NewHandler(Dependencies{ServingRevisions: store.ServingRevisions{Catalog: 9, Keys: 9}, KeysConfirmedAt: confirmed, CatalogConfirmedAt: confirmed})
	h.ApplyServingKeys(config.Auth{}, []auth.DynamicClient{}, store.ServingRevisions{Catalog: 8, Keys: 8}, confirmed.Add(time.Second))
	if h.current().ServingRevisions.Keys != 9 || h.current().KeysConfirmedAt != confirmed {
		t.Fatal("older revision replaced newer state")
	}
	h.ApplyServingKeys(config.Auth{}, nil, store.ServingRevisions{Catalog: 9, Keys: 10}, confirmed.Add(-time.Second))
	if h.current().ServingRevisions.Keys != 9 {
		t.Fatal("older snapshot extended freshness")
	}
	h.ApplyServingKeys(config.Auth{}, nil, store.ServingRevisions{Catalog: 10, Keys: 10}, confirmed.Add(time.Second))
	if h.current().ServingRevisions.Keys != 10 || h.current().CatalogConfirmedAt != confirmed {
		t.Fatal("key application falsely certified catalog")
	}
	if status := h.ServingStatus(); status["cluster_acknowledged"] != false {
		t.Fatal("local application claimed cluster acknowledgement")
	}
}
