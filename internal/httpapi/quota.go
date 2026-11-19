package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	store   QuotaStore
	windows map[string][]tpmSample
	spend   map[string]cachedSpend
}

type QuotaStore interface {
	GetScopeQuota(context.Context, uuid.UUID, string, uuid.UUID, string) (store.ScopeQuota, error)
	SumScopeSpend(context.Context, uuid.UUID, string, uuid.UUID) (int64, error)
	InsertSpendEntry(context.Context, *store.SpendEntry) error
}

func NewQuotaTracker(st *store.Store) *QuotaTracker {
	if st == nil {
		return NewQuotaTrackerWithStore(nil)
	}
	return NewQuotaTrackerWithStore(st)
}

func NewQuotaTrackerWithStore(st QuotaStore) *QuotaTracker {
	return &QuotaTracker{store: st, windows: map[string][]tpmSample{}, spend: map[string]cachedSpend{}}
}

func (q *QuotaTracker) HasStore() bool {
	return q != nil && q.store != nil
}

func quotaWindowKey(workspaceID uuid.UUID, scope quotaScope, model string) string {
	return workspaceID.String() + "|" + scope.typ + "|" + scope.id.String() + "|" + model
}

func (q *QuotaTracker) quotaRow(ctx context.Context, workspaceID uuid.UUID, scope quotaScope, model string) (store.ScopeQuota, bool, error) {
	if q == nil || q.store == nil {
		return store.ScopeQuota{}, false, errors.New("quota store unavailable")
	}
	row, err := q.store.GetScopeQuota(ctx, workspaceID, scope.typ, scope.id, model)
	if errors.Is(err, sql.ErrNoRows) {
		return store.ScopeQuota{}, false, nil
	}
	if err != nil {
		return store.ScopeQuota{}, false, fmt.Errorf("read quota for model %q: %w", model, err)
	}
	return row, true, nil
}

func (q *QuotaTracker) scopeSpend(ctx context.Context, workspaceID uuid.UUID, scope quotaScope) (int64, error) {
	if q == nil || q.store == nil {
		return 0, errors.New("quota store unavailable")
	}
	key := workspaceID.String() + "|" + scope.typ + "|" + scope.id.String()
	now := time.Now()
	q.mu.Lock()
	cached, ok := q.spend[key]
	q.mu.Unlock()
	if ok && now.Sub(cached.at) < quotaSpendTTL {
		return cached.sum, nil
	}
	sum, err := q.store.SumScopeSpend(ctx, workspaceID, scope.typ, scope.id)
	if err != nil {
		return 0, fmt.Errorf("read quota spend: %w", err)
	}
	q.mu.Lock()
	q.spend[key] = cachedSpend{sum: sum, at: now}
	q.mu.Unlock()
	return sum, nil
}

func (q *QuotaTracker) windowTokens(workspaceID uuid.UUID, scope quotaScope, model string) (int64, time.Duration) {
	if q == nil {
		return 0, 0
	}
	key := quotaWindowKey(workspaceID, scope, model)
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

func (q *QuotaTracker) recordTokens(workspaceID uuid.UUID, scope quotaScope, model string, tokens int64) {
	if q == nil || tokens <= 0 {
		return
	}
	key := quotaWindowKey(workspaceID, scope, model)
	q.mu.Lock()
	q.windows[key] = append(q.windows[key], tpmSample{at: time.Now(), tokens: tokens})
	q.mu.Unlock()
}

func (q *QuotaTracker) Invalidate(workspaceID uuid.UUID, scope quotaScope) {
	if q == nil {
		return
	}
	q.mu.Lock()
	delete(q.spend, workspaceID.String()+"|"+scope.typ+"|"+scope.id.String())
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
	_, workspaceID, isKey := principal.DatabaseKey()
	if !deps.Quota.HasStore() || !deps.MultiTenancy.Enabled || !isKey {
		return true
	}
	ctx := r.Context()
	for _, scope := range quotaScopes(principal) {
		row, ok, err := deps.Quota.quotaRow(ctx, workspaceID, scope, quotaBudgetModel)
		if err != nil {
			return h.rejectQuotaUnavailable(deps, w, r, workspaceID, scope, err)
		}
		if ok && row.BudgetMicros > 0 {
			spent, err := deps.Quota.scopeSpend(ctx, workspaceID, scope)
			if err != nil {
				return h.rejectQuotaUnavailable(deps, w, r, workspaceID, scope, err)
			}
			spent -= row.SpentOffsetMicros
			if spent < 0 {
				spent = 0
			}
			if spent >= row.BudgetMicros {
				h.writeRequestError(deps.Metrics, w, r, http.StatusForbidden, "budget_exceeded", "budget exhausted")
				return false
			}
		}
		row, ok, err = deps.Quota.quotaRow(ctx, workspaceID, scope, model)
		if err != nil {
			return h.rejectQuotaUnavailable(deps, w, r, workspaceID, scope, err)
		}
		if ok {
			limit := row.TPMEffective
			if limit <= 0 {
				limit = row.TPMCeiling
			}
			if limit > 0 {
				used, retry := deps.Quota.windowTokens(workspaceID, scope, model)
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

func (h *Handler) rejectQuotaUnavailable(deps Dependencies, w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID, scope quotaScope, err error) bool {
	deps.Logger.Warn("quota admission unavailable", "error", err, "workspace_id", workspaceID, "scope_type", scope.typ, "scope_id", scope.id, "request_id", w.Header().Get("X-Request-Id"))
	h.writeRequestError(deps.Metrics, w, r, http.StatusServiceUnavailable, "quota_unavailable", "quota data temporarily unavailable")
	return false
}

func quotaTokens(usage provider.Usage) int64 {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.PromptTokens + usage.CompletionTokens
}

func (h *Handler) recordQuotaUsage(deps Dependencies, principal *auth.Principal, publicModel, upstreamModel, providerName string, usage provider.Usage) {
	if err := h.persistQuotaUsageContext(context.Background(), deps, principal, publicModel, upstreamModel, providerName, usage); err != nil {
		deps.Logger.Warn("quota ledger insert failed", "error", err)
	}
	h.recordLocalQuotaUsage(deps, principal, publicModel, usage)
}

func (h *Handler) recordLocalQuotaUsage(deps Dependencies, principal *auth.Principal, publicModel string, usage provider.Usage) {
	_, workspaceID, isKey := principal.DatabaseKey()
	if !deps.Quota.HasStore() || !deps.MultiTenancy.Enabled || !isKey {
		return
	}
	tokens := quotaTokens(usage)
	if tokens <= 0 {
		return
	}
	for _, scope := range quotaScopes(principal) {
		deps.Quota.recordTokens(workspaceID, scope, publicModel, tokens)
	}
}

func (h *Handler) persistQuotaUsageContext(ctx context.Context, deps Dependencies, principal *auth.Principal, publicModel, upstreamModel, providerName string, usage provider.Usage) error {
	keyID, workspaceID, isKey := principal.DatabaseKey()
	if !deps.Quota.HasStore() || !deps.MultiTenancy.Enabled || !isKey {
		return nil
	}
	tokens := quotaTokens(usage)
	if tokens <= 0 && !usage.Has() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var costMicros int64
	if upstreamModel != "" {
		if _, model, ok := deps.Catalog.Model(providerName, upstreamModel); ok && model.Pricing != nil {
			p := model.Pricing
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
		WorkspaceID: workspaceID, KeyID: keyID,
		UserID: userID, TeamID: teamID,
		Model: publicModel, Tokens: tokens, CostMicros: costMicros,
	}
	return deps.Quota.store.InsertSpendEntry(ctx, entry)
}
