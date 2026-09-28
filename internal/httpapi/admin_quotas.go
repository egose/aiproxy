package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type adminTPMView struct {
	Model      string `json:"model"`
	Ceiling    int64  `json:"ceiling"`
	Effective  int64  `json:"effective"`
	UsedTokens int64  `json:"used_tokens"`
}

type adminQuotaView struct {
	BudgetMicros int64          `json:"budget_micros"`
	SpendMicros  int64          `json:"spend_micros"`
	TPM          []adminTPMView `json:"tpm"`
}

type adminTPMUpsert struct {
	Model     string `json:"model"`
	Ceiling   *int64 `json:"ceiling"`
	Effective *int64 `json:"effective"`
}

type adminQuotaUpsert struct {
	BudgetMicros *int64           `json:"budget_micros"`
	ResetSpend   bool             `json:"reset_spend"`
	TPM          []adminTPMUpsert `json:"tpm"`
}

func (h *Handler) quotaView(deps Dependencies, ctx context.Context, workspaceID uuid.UUID, scope quotaScope) (adminQuotaView, error) {
	view := adminQuotaView{TPM: []adminTPMView{}}
	rows, err := deps.AdminStore.ListScopeQuotasByScope(ctx, workspaceID, scope.typ, scope.id)
	if err != nil {
		return view, err
	}
	spent, err := deps.AdminStore.SumScopeSpend(ctx, workspaceID, scope.typ, scope.id)
	if err != nil {
		return view, err
	}
	view.SpendMicros = spent
	for _, row := range rows {
		if row.Model == quotaBudgetModel {
			view.BudgetMicros = row.BudgetMicros
			view.SpendMicros = spent - row.SpentOffsetMicros
			if view.SpendMicros < 0 {
				view.SpendMicros = 0
			}
			continue
		}
		entry := adminTPMView{Model: row.Model, Ceiling: row.TPMCeiling, Effective: row.TPMEffective}
		if deps.Quota != nil {
			used, _ := deps.Quota.windowTokens(workspaceID, scope, row.Model)
			entry.UsedTokens = used
		}
		view.TPM = append(view.TPM, entry)
	}
	return view, nil
}

func errForbiddenAdmin() error {
	return errQuotaForbidden
}

var errQuotaForbidden = errors.New("workspace admin required")

type quotaInputError struct{ msg string }

func (e *quotaInputError) Error() string { return e.msg }

func quotaBad(msg string) error { return &quotaInputError{msg: msg} }

func (h *Handler) applyQuotaBudget(deps Dependencies, ctx context.Context, workspaceID uuid.UUID, scope quotaScope, budget int64, reset bool) error {
	row, err := deps.AdminStore.GetScopeQuota(ctx, workspaceID, scope.typ, scope.id, quotaBudgetModel)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		row = store.ScopeQuota{WorkspaceID: workspaceID, ScopeType: scope.typ, ScopeID: scope.id, Model: quotaBudgetModel}
	}
	if budget >= 0 {
		row.BudgetMicros = budget
	}
	if reset {
		spent, err := deps.AdminStore.SumScopeSpend(ctx, workspaceID, scope.typ, scope.id)
		if err != nil {
			return err
		}
		row.SpentOffsetMicros = spent
	}
	return deps.AdminStore.UpsertScopeQuota(ctx, &row)
}

func (h *Handler) applyQuotaTPM(deps Dependencies, ctx context.Context, workspaceID uuid.UUID, scope quotaScope, tpm []adminTPMUpsert, policy bool) error {
	for _, t := range tpm {
		model := strings.TrimSpace(t.Model)
		if model == "" {
			return quotaBad("tpm model is required")
		}
		if t.Ceiling != nil && !policy {
			return errForbiddenAdmin()
		}
		row, err := deps.AdminStore.GetScopeQuota(ctx, workspaceID, scope.typ, scope.id, model)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			row = store.ScopeQuota{WorkspaceID: workspaceID, ScopeType: scope.typ, ScopeID: scope.id, Model: model}
		}
		if t.Ceiling != nil {
			if *t.Ceiling < 0 {
				return quotaBad("tpm ceiling must be >= 0")
			}
			row.TPMCeiling = *t.Ceiling
		}
		if t.Effective != nil {
			if *t.Effective < 0 {
				return quotaBad("tpm limit must be >= 0")
			}
			if row.TPMCeiling > 0 && *t.Effective > row.TPMCeiling {
				return quotaBad("tpm limit exceeds the admin ceiling")
			}
			row.TPMEffective = *t.Effective
		}
		if err := deps.AdminStore.UpsertScopeQuota(ctx, &row); err != nil {
			return err
		}
	}
	return nil
}

func writeQuotaError(deps Dependencies, w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errQuotaForbidden) {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	var input *quotaInputError
	if errors.As(err, &input) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeQuotaStorageError(deps, w, err, "could not update quota")
}

func writeQuotaStorageError(deps Dependencies, w http.ResponseWriter, err error, message string) {
	deps.Logger.Warn(message, "error", err, "request_id", w.Header().Get("X-Request-Id"))
	http.Error(w, message, http.StatusInternalServerError)
}

func (h *Handler) adminUserQuota(deps Dependencies, w http.ResponseWriter, r *http.Request, workspaceID, userID uuid.UUID, rest []string) {
	if len(rest) != 1 || rest[0] != "quota" {
		http.Error(w, "unknown admin endpoint", http.StatusNotFound)
		return
	}
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	if _, err := deps.AdminStore.GetMembership(ctx, userID, workspaceID); err != nil {
		http.Error(w, "user is not an workspace member", http.StatusNotFound)
		return
	}
	self := mustParseUUID(claims.Subject) == userID
	workspaceAdmin := h.canWriteWorkspace(deps, ctx, claims, workspaceID)
	if !self && !workspaceAdmin {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	scope := quotaScope{typ: quotaScopeUser, id: userID}
	switch r.Method {
	case http.MethodGet:
		view, err := h.quotaView(deps, ctx, workspaceID, scope)
		if err != nil {
			writeQuotaStorageError(deps, w, err, "could not load quota")
			return
		}
		writeAdminJSON(w, http.StatusOK, view)
	case http.MethodPut:
		var req adminQuotaUpsert
		if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if (req.BudgetMicros != nil || req.ResetSpend) && !workspaceAdmin {
			http.Error(w, "workspace admin required", http.StatusForbidden)
			return
		}
		if req.BudgetMicros != nil && *req.BudgetMicros < 0 {
			http.Error(w, "budget must be >= 0", http.StatusBadRequest)
			return
		}
		if req.BudgetMicros != nil || req.ResetSpend {
			budget := int64(-1)
			if req.BudgetMicros != nil {
				budget = *req.BudgetMicros
			}
			if err := h.applyQuotaBudget(deps, ctx, workspaceID, scope, budget, req.ResetSpend); err != nil {
				writeQuotaError(deps, w, r, err)
				return
			}
		}
		if err := h.applyQuotaTPM(deps, ctx, workspaceID, scope, req.TPM, workspaceAdmin); err != nil {
			writeQuotaError(deps, w, r, err)
			return
		}
		if deps.Quota != nil {
			deps.Quota.Invalidate(workspaceID, scope)
		}
		view, err := h.quotaView(deps, ctx, workspaceID, scope)
		if err != nil {
			writeQuotaStorageError(deps, w, err, "quota saved but could not load updated quota")
			return
		}
		writeAdminJSON(w, http.StatusOK, view)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) adminTeamQuota(deps Dependencies, w http.ResponseWriter, r *http.Request, workspaceID, teamID uuid.UUID) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	if !h.canAccessWorkspace(deps, ctx, claims, workspaceID) {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}
	team, err := deps.AdminStore.GetTeam(ctx, teamID)
	if err != nil || team.WorkspaceID != workspaceID {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}
	workspaceAdmin := h.canWriteWorkspace(deps, ctx, claims, workspaceID)
	teamAdmin := h.isTeamAdmin(deps, ctx, claims, teamID)
	if !workspaceAdmin && !teamAdmin {
		if _, err := deps.AdminStore.GetTeamMember(ctx, mustParseUUID(claims.Subject), teamID); err != nil {
			http.Error(w, "team not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "team admin required", http.StatusForbidden)
			return
		}
	}
	scope := quotaScope{typ: quotaScopeTeam, id: teamID}
	switch r.Method {
	case http.MethodGet:
		view, err := h.quotaView(deps, ctx, workspaceID, scope)
		if err != nil {
			writeQuotaStorageError(deps, w, err, "could not load quota")
			return
		}
		writeAdminJSON(w, http.StatusOK, view)
	case http.MethodPut:
		var req adminQuotaUpsert
		if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if (req.BudgetMicros != nil || req.ResetSpend) && !workspaceAdmin {
			http.Error(w, "workspace admin required", http.StatusForbidden)
			return
		}
		if req.BudgetMicros != nil && *req.BudgetMicros < 0 {
			http.Error(w, "budget must be >= 0", http.StatusBadRequest)
			return
		}
		if req.BudgetMicros != nil || req.ResetSpend {
			budget := int64(-1)
			if req.BudgetMicros != nil {
				budget = *req.BudgetMicros
			}
			if err := h.applyQuotaBudget(deps, ctx, workspaceID, scope, budget, req.ResetSpend); err != nil {
				writeQuotaError(deps, w, r, err)
				return
			}
		}
		if err := h.applyQuotaTPM(deps, ctx, workspaceID, scope, req.TPM, workspaceAdmin); err != nil {
			writeQuotaError(deps, w, r, err)
			return
		}
		if deps.Quota != nil {
			deps.Quota.Invalidate(workspaceID, scope)
		}
		view, err := h.quotaView(deps, ctx, workspaceID, scope)
		if err != nil {
			writeQuotaStorageError(deps, w, err, "quota saved but could not load updated quota")
			return
		}
		writeAdminJSON(w, http.StatusOK, view)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func isGlobalAdmin(claims *adminauth.Claims) bool {
	return claims != nil && claims.IsAdmin
}
