package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

const ServingFreshnessDeadline = 30 * time.Second

var ErrServingStale = errors.New("database serving state freshness is unconfirmed")

func servingFresh(confirmed time.Time) bool {
	return servingFreshAt(confirmed, time.Now())
}

func servingFreshAt(confirmed, now time.Time) bool {
	return !confirmed.IsZero() && !now.Before(confirmed) && now.Sub(confirmed) < ServingFreshnessDeadline
}

func databaseModel(deps Dependencies, model string) bool {
	provider, name, _ := strings.Cut(model, "/")
	if provider == "alias" {
		for _, alias := range deps.Catalog.Aliases() {
			if alias.Name != name {
				continue
			}
			if alias.DatabaseManaged {
				return true
			}
			for _, target := range alias.Targets {
				if databaseModel(deps, target.Provider+"/"+target.Model) {
					return true
				}
			}
		}
	}
	for _, p := range deps.Catalog.Providers() {
		if p.Name == provider {
			return p.DatabaseManaged
		}
	}
	return false
}

func (h *Handler) writeInferenceAuthError(deps Dependencies, w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrServingStale) {
		h.writeRequestError(deps.Metrics, w, r, http.StatusServiceUnavailable, "serving_stale", ErrServingStale.Error())
		return
	}
	h.writeRequestError(deps.Metrics, w, r, http.StatusUnauthorized, "auth_failed", err.Error())
}

func (h *Handler) denyStaleDatabaseRead(deps Dependencies, w http.ResponseWriter, r *http.Request, principal *auth.Principal) bool {
	_, _, database := principal.DatabaseKey()
	if database && deps.ServingRevisions.Catalog > 0 && !servingFresh(deps.CatalogConfirmedAt) {
		h.writeInferenceAuthError(deps, w, r, ErrServingStale)
		return true
	}
	return false
}

func (h *Handler) ApplyServingKeys(cfg config.Auth, keys []auth.DynamicClient, revisions store.ServingRevisions, confirmed time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if revisions.Keys < h.deps.ServingRevisions.Keys || confirmed.Before(h.deps.KeysConfirmedAt) {
		return
	}
	if revisions.Keys != h.deps.ServingRevisions.Keys {
		h.deps.Quota = NewQuotaTracker(h.deps.AdminStore)
	}
	h.deps.Auth = auth.NewAuthenticatorWithClients(cfg, keys)
	h.deps.Authorizer = auth.NewAuthorizerWithClients(cfg, keys)
	h.deps.ServingRevisions.Keys = revisions.Keys
	h.deps.KeysConfirmedAt = confirmed
	if revisions.Catalog == h.deps.ServingRevisions.Catalog && confirmed.After(h.deps.CatalogConfirmedAt) {
		h.deps.CatalogConfirmedAt = confirmed
	}
	h.deps.revision++
}

func (h *Handler) ServingStatus() map[string]any {
	deps := h.current()
	return map[string]any{
		"applied_revisions":          deps.ServingRevisions,
		"keys_confirmed_at":          deps.KeysConfirmedAt,
		"catalog_confirmed_at":       deps.CatalogConfirmedAt,
		"keys_fresh":                 deps.ServingRevisions.Keys == 0 || servingFresh(deps.KeysConfirmedAt),
		"catalog_fresh":              deps.ServingRevisions.Catalog == 0 || servingFresh(deps.CatalogConfirmedAt),
		"freshness_deadline_seconds": 30,
		"cluster_acknowledged":       false,
	}
}
