package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

const copilotIssuerTimeout = 25 * time.Second

const catalogSavedHeader = "X-Aiproxy-Catalog-Saved"

var errDeviceFlowNotReady = errors.New("device authorization is not ready; check flow status and retry")

type adminCopilotFlowStart struct {
	ClientID     string `json:"client_id"`
	WorkspaceID  string `json:"workspace_id"`
	ProviderName string `json:"provider_name"`
}

func (h *Handler) copilotDeviceClient(deps Dependencies) *copilotlogin.Client {
	if deps.CopilotDeviceClient != nil {
		if client := deps.CopilotDeviceClient(); client != nil {
			return client
		}
	}
	return copilotlogin.New()
}

func writeCopilotFlowStatus(w http.ResponseWriter, code int, status store.CopilotFlowStatus) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(status)
}

func writeCopilotFlowError(deps Dependencies, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrCopilotFlowNotFound):
		http.Error(w, "device flow not found", http.StatusNotFound)
	case errors.Is(err, store.ErrCopilotFlowUnauthorized):
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	case errors.Is(err, store.ErrCopilotFlowForbidden):
		http.Error(w, "workspace admin required", http.StatusForbidden)
	case errors.Is(err, store.ErrCopilotFlowLimit):
		http.Error(w, "device flow admission limit reached", http.StatusTooManyRequests)
	case errors.Is(err, store.ErrCopilotFlowUnavailable):
		http.Error(w, "device flow is no longer available", http.StatusConflict)
	case errors.Is(err, store.ErrCatalogConflict):
		http.Error(w, store.ErrCatalogConflict.Error(), http.StatusConflict)
	case errors.Is(err, errDeviceFlowNotReady):
		http.Error(w, errDeviceFlowNotReady.Error(), http.StatusConflict)
	case errors.Is(err, store.ErrCopilotFlowInput):
		http.Error(w, "invalid device flow input", http.StatusBadRequest)
	default:
		var storage catalogStorageError
		if errors.As(err, &storage) {
			if deps.Logger != nil {
				deps.Logger.Error("device flow storage failed", "error", err)
			}
			http.Error(w, "could not process device flow", http.StatusInternalServerError)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func (h *Handler) adminCopilotFlows(deps Dependencies, w http.ResponseWriter, r *http.Request, rest []string) {
	w.Header().Set("Cache-Control", "no-store")
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if deps.AdminStore == nil {
		http.Error(w, "multi_tenancy not enabled", http.StatusNotFound)
		return
	}
	if len(rest) == 0 || rest[0] == "" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.startCopilotFlow(deps, w, r, claims)
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(rest[0]))
	if err != nil {
		http.Error(w, "invalid device flow id", http.StatusBadRequest)
		return
	}
	if len(rest) == 2 && rest[1] == "poll" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.pollCopilotFlow(deps, w, r, claims, id)
		return
	}
	if len(rest) == 1 {
		switch r.Method {
		case http.MethodGet:
			h.getCopilotFlow(deps, w, r, claims, id)
			return
		case http.MethodDelete:
			h.cancelCopilotFlow(deps, w, r, claims, id)
			return
		}
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (h *Handler) startCopilotFlow(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims) {
	var req adminCopilotFlowStart
	if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		http.Error(w, "client_id is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	actorID := mustParseUUID(claims.Subject)
	var workspace store.Workspace
	var providerID *uuid.UUID
	if name := strings.TrimSpace(req.ProviderName); name != "" {
		p, err := deps.AdminStore.GetProvider(ctx, name)
		if err != nil {
			if h.isStaticProvider(deps, name) {
				http.Error(w, "config-managed: edit the HCL file", http.StatusBadRequest)
				return
			}
			http.Error(w, "provider not found", http.StatusNotFound)
			return
		}
		if p.Type != "github-copilot" {
			http.Error(w, "device authorization is only supported by github-copilot", http.StatusBadRequest)
			return
		}
		if workspaceStr := strings.TrimSpace(req.WorkspaceID); workspaceStr != "" && workspaceStr != p.WorkspaceID.String() {
			http.Error(w, "workspace does not match provider workspace", http.StatusBadRequest)
			return
		}
		if !h.requireResourceWorkspace(deps, w, r, claims, p.WorkspaceID) {
			return
		}
		workspaceID := p.WorkspaceID
		workspace = store.Workspace{ID: workspaceID}
		id := p.ID
		providerID = &id
	} else {
		resolved, ok := h.resolveWriteWorkspace(deps, w, r, claims, req.WorkspaceID)
		if !ok {
			return
		}
		workspace = resolved
	}
	_, lease, err := deps.AdminStore.StartCopilotFlow(ctx, store.CopilotFlowStart{
		ActorID: actorID, WorkspaceID: workspace.ID, ProviderID: providerID, ClientID: clientID,
	})
	if err != nil {
		writeCopilotFlowError(deps, w, err)
		return
	}
	if lease == nil {
		writeCopilotFlowError(deps, w, store.ErrCopilotFlowLimit)
		return
	}
	client := h.copilotDeviceClient(deps)
	issuerCtx, cancel := context.WithTimeout(r.Context(), copilotIssuerTimeout)
	code, reqErr := client.RequestCode(issuerCtx, clientID, copilotlogin.DefaultScope)
	cancel()
	if reqErr != nil {
		if deps.Logger != nil {
			deps.Logger.Error("device-code request failed", "error", reqErr)
		}
		final, ferr := deps.AdminStore.FinalizeCopilotFlowStart(ctx, *lease, nil)
		if ferr != nil {
			writeCopilotFlowError(deps, w, ferr)
			return
		}
		writeCopilotFlowStatus(w, http.StatusCreated, final)
		return
	}
	final, ferr := deps.AdminStore.FinalizeCopilotFlowStart(ctx, *lease, &code)
	if ferr != nil {
		writeCopilotFlowError(deps, w, ferr)
		return
	}
	writeCopilotFlowStatus(w, http.StatusCreated, final)
}

func (h *Handler) getCopilotFlow(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, id uuid.UUID) {
	status, err := deps.AdminStore.GetCopilotFlow(r.Context(), mustParseUUID(claims.Subject), id)
	if err != nil {
		writeCopilotFlowError(deps, w, err)
		return
	}
	writeCopilotFlowStatus(w, http.StatusOK, status)
}

func (h *Handler) cancelCopilotFlow(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, id uuid.UUID) {
	status, err := deps.AdminStore.CancelCopilotFlow(r.Context(), mustParseUUID(claims.Subject), id)
	if err != nil {
		writeCopilotFlowError(deps, w, err)
		return
	}
	writeCopilotFlowStatus(w, http.StatusOK, status)
}

func (h *Handler) pollCopilotFlow(deps Dependencies, w http.ResponseWriter, r *http.Request, claims *adminauth.Claims, id uuid.UUID) {
	ctx := r.Context()
	actorID := mustParseUUID(claims.Subject)
	status, lease, err := deps.AdminStore.ClaimCopilotFlowPoll(ctx, actorID, id)
	if err != nil {
		writeCopilotFlowError(deps, w, err)
		return
	}
	if lease == nil {
		writeCopilotFlowStatus(w, http.StatusOK, status)
		return
	}
	client := h.copilotDeviceClient(deps)
	issuerCtx, cancel := context.WithTimeout(r.Context(), copilotIssuerTimeout)
	step, stepErr := client.PollOneStep(issuerCtx, lease.ClientID(), lease.DeviceCode(), lease.Interval())
	cancel()
	var result store.CopilotFlowPollResult
	switch {
	case stepErr != nil && errors.Is(stepErr, copilotlogin.ErrAccessDenied):
		result.State = "denied"
	case stepErr != nil && errors.Is(stepErr, copilotlogin.ErrExpired):
		result.State = "expired"
	case stepErr != nil:
		if deps.Logger != nil {
			deps.Logger.Error("device-token poll failed", "error", stepErr)
		}
		result.State = "other"
		result.Interval = lease.Interval()
	case step.State == copilotlogin.PollStepReady:
		cred, credErr := copilotlogin.NewCredential(lease.ClientID(), step.Token.AccessToken, time.Now())
		if credErr != nil {
			if deps.Logger != nil {
				deps.Logger.Error("device-token credential invalid")
			}
			result.State = "other"
		} else {
			result.State = "ready"
			result.Interval = step.Interval
			result.Credential = cred
		}
	case step.State == copilotlogin.PollStepPending:
		result.State = "pending"
		result.Interval = step.Interval
	default:
		result.State = "other"
	}
	final, ferr := deps.AdminStore.FinalizeCopilotFlowPoll(ctx, *lease, result)
	if ferr != nil {
		if current, gerr := deps.AdminStore.GetCopilotFlow(ctx, actorID, id); gerr == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(current)
			return
		}
		writeCopilotFlowError(deps, w, ferr)
		return
	}
	writeCopilotFlowStatus(w, http.StatusOK, final)
}
