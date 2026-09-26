package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

const (
	quotaScopeUser   = "user"
	quotaScopeTeam   = "team"
	quotaBudgetModel = ""
	quotaWindow      = time.Minute
	quotaSpendTTL    = 30 * time.Second
)

type quotaScope struct {
	typ string
	id  uuid.UUID
}

type tpmSample struct {
	at     time.Time
	tokens int64
}

type cachedSpend struct {
	sum int64
	at  time.Time
}

type QuotaTracker struct {
	mu      sync.Mutex
	store   *store.Store
	windows map[string][]tpmSample
	spend   map[string]cachedSpend
}

func NewQuotaTracker(st *store.Store) *QuotaTracker {
	return &QuotaTracker{store: st, windows: map[string][]tpmSample{}, spend: map[string]cachedSpend{}}
}

func quotaWindowKey(orgID uuid.UUID, scope quotaScope, model string) string {
	return orgID.String() + "|" + scope.typ + "|" + scope.id.String() + "|" + model
}

func (q *QuotaTracker) quotaRow(ctx context.Context, orgID uuid.UUID, scope quotaScope, model string) (store.ScopeQuota, bool) {
	if q == nil || q.store == nil {
		return store.ScopeQuota{}, false
	}
	row, err := q.store.GetScopeQuota(ctx, orgID, scope.typ, scope.id, model)
	if err != nil {
		return store.ScopeQuota{}, false
	}
	return row, true
}

func (q *QuotaTracker) scopeSpend(ctx context.Context, orgID uuid.UUID, scope quotaScope) int64 {
	if q == nil || q.store == nil {
		return 0
	}
	key := orgID.String() + "|" + scope.typ + "|" + scope.id.String()
	now := time.Now()
	q.mu.Lock()
	cached, ok := q.spend[key]
	q.mu.Unlock()
	if ok && now.Sub(cached.at) < quotaSpendTTL {
		return cached.sum
	}
	sum, err := q.store.SumScopeSpend(ctx, orgID, scope.typ, scope.id)
	if err != nil {
		return 0
	}
	q.mu.Lock()
	q.spend[key] = cachedSpend{sum: sum, at: now}
	q.mu.Unlock()
	return sum
}

func (q *QuotaTracker) windowTokens(orgID uuid.UUID, scope quotaScope, model string) (int64, time.Duration) {
	if q == nil {
		return 0, 0
	}
	key := quotaWindowKey(orgID, scope, model)
	now := time.Now()
	q.mu.Lock()
	defer q.mu.Unlock()
	kept := q.windows[key][:0]
	var used int64
	var oldest time.Time
	for _, s := range q.windows[key] {
		if now.Sub(s.at) >= quotaWindow {
			continue
		}
		kept = append(kept, s)
		used += s.tokens
		if oldest.IsZero() || s.at.Before(oldest) {
			oldest = s.at
		}
	}
	q.windows[key] = kept
	if used <= 0 || oldest.IsZero() {
		return used, 0
	}
	retry := oldest.Add(quotaWindow).Sub(now)
	if retry < time.Second {
		retry = time.Second
	}
	return used, retry
}

func (q *QuotaTracker) recordTokens(orgID uuid.UUID, scope quotaScope, model string, tokens int64) {
	if q == nil || tokens <= 0 {
		return
	}
	key := quotaWindowKey(orgID, scope, model)
	q.mu.Lock()
	q.windows[key] = append(q.windows[key], tpmSample{at: time.Now(), tokens: tokens})
	q.mu.Unlock()
}

func (q *QuotaTracker) Invalidate(orgID uuid.UUID, scope quotaScope) {
	if q == nil {
		return
	}
	q.mu.Lock()
	delete(q.spend, orgID.String()+"|"+scope.typ+"|"+scope.id.String())
	q.mu.Unlock()
}

func quotaScopes(principal *auth.Principal) []quotaScope {
	if principal == nil {
		return nil
	}
	out := []quotaScope{}
	if principal.OwnerUserID != uuid.Nil {
		out = append(out, quotaScope{typ: quotaScopeUser, id: principal.OwnerUserID})
	}
	if principal.OwnerTeamID != uuid.Nil {
		out = append(out, quotaScope{typ: quotaScopeTeam, id: principal.OwnerTeamID})
	}
	return out
}

func (h *Handler) allowQuota(deps Dependencies, w http.ResponseWriter, r *http.Request, principal *auth.Principal, model string) bool {
	if deps.AdminStore == nil || !deps.MultiTenancy.Enabled || deps.Quota == nil || principal == nil || principal.KeyID == uuid.Nil {
		return true
	}
	ctx := r.Context()
	for _, scope := range quotaScopes(principal) {
		if row, ok := deps.Quota.quotaRow(ctx, principal.OrgID, scope, quotaBudgetModel); ok && row.BudgetMicros > 0 {
			spent := deps.Quota.scopeSpend(ctx, principal.OrgID, scope) - row.SpentOffsetMicros
			if spent < 0 {
				spent = 0
			}
			if spent >= row.BudgetMicros {
				h.writeRequestError(deps.Metrics, w, r, http.StatusForbidden, "budget_exceeded", "budget exhausted")
				return false
			}
		}
		if row, ok := deps.Quota.quotaRow(ctx, principal.OrgID, scope, model); ok {
			limit := row.TPMEffective
			if limit <= 0 {
				limit = row.TPMCeiling
			}
			if limit > 0 {
				used, retry := deps.Quota.windowTokens(principal.OrgID, scope, model)
				if used >= limit {
					seconds := int(retry / time.Second)
					if retry%time.Second != 0 {
						seconds++
					}
					if seconds < 1 {
						seconds = 1
					}
					w.Header().Set("Retry-After", strconv.Itoa(seconds))
					h.writeRequestError(deps.Metrics, w, r, http.StatusTooManyRequests, "tpm_exceeded", "token rate limit exceeded")
					return false
				}
			}
		}
	}
	return true
}

func quotaTokens(usage provider.Usage) int64 {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.PromptTokens + usage.CompletionTokens
}

func (h *Handler) recordQuotaUsage(deps Dependencies, principal *auth.Principal, publicModel, upstreamModel, providerName string, usage provider.Usage) {
	if deps.AdminStore == nil || !deps.MultiTenancy.Enabled || principal == nil || principal.KeyID == uuid.Nil {
		return
	}
	tokens := quotaTokens(usage)
	if tokens <= 0 && !usage.Has() {
		return
	}
	ctx := context.Background()
	var costMicros int64
	if prices := billingPrices(deps.Catalog); upstreamModel != "" {
		if p := prices[providerName+"/"+upstreamModel]; p != nil {
			if cost, ok := p.Cost(usage.PromptTokens, usage.CompletionTokens, usage.CachedTokens, usage.CacheCreationTokens, usage.CacheReadTokens); ok {
				costMicros = int64(cost * 1e6)
			}
		}
	}
	var userID, teamID *uuid.UUID
	if principal.OwnerUserID != uuid.Nil {
		uid := principal.OwnerUserID
		userID = &uid
	}
	if principal.OwnerTeamID != uuid.Nil {
		tid := principal.OwnerTeamID
		teamID = &tid
	}
	entry := &store.SpendEntry{
		OrgID: principal.OrgID, KeyID: principal.KeyID,
		UserID: userID, TeamID: teamID,
		Model: publicModel, Tokens: tokens, CostMicros: costMicros,
	}
	if err := deps.AdminStore.InsertSpendEntry(ctx, entry); err != nil {
		deps.Logger.Warn("quota ledger insert failed", "error", err)
	}
	if deps.Quota != nil {
		for _, scope := range quotaScopes(principal) {
			deps.Quota.recordTokens(principal.OrgID, scope, publicModel, tokens)
		}
	}
}
