package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type adminAliasTargetView struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type adminAliasView struct {
	Name                        string                                `json:"name"`
	Algorithm                   string                                `json:"algorithm"`
	RetryStatusCodes            []int                                 `json:"retry_status_codes,omitempty"`
	SessionAffinity             *adminSessionAffinity                 `json:"session_affinity,omitempty"`
	EncryptedReasoning          *config.EncryptedReasoningDeclaration `json:"encrypted_reasoning"`
	EncryptedReasoningEffective *adminEncryptedReason                 `json:"encrypted_reasoning_effective"`
	Source                      string                                `json:"source"`
	WorkspaceID                 string                                `json:"workspace_id,omitempty"`
	WorkspaceName               string                                `json:"workspace_name,omitempty"`
	Targets                     []adminAliasTargetView                `json:"targets"`
}

type adminSessionAffinity struct {
	Headers []string `json:"headers"`
}

type adminEncryptedReason struct {
	Passthrough      bool     `json:"passthrough"`
	OnCallerMismatch string   `json:"on_caller_mismatch"`
	MatchMessages    []string `json:"match_messages"`
}

func (h *Handler) adminAliases(deps Dependencies, w http.ResponseWriter, r *http.Request, rest []string) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			visibility, ok := h.resolveWorkspaceFilter(deps, w, r, claims)
			if !ok {
				return
			}
			views, err := h.mergedAliasViews(ctx, deps, visibility)
			if err != nil {
				http.Error(w, "could not list aliases", http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"aliases": views})
			return
		case http.MethodPost:
			var req adminAliasUpsert
			if err := decodeAdminCatalogBody(r, &req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			workspace, ok := h.resolveWriteWorkspace(deps, w, r, claims, trimmedOrEmpty(req.WorkspaceID.value))
			if !ok {
				return
			}
			view, err := h.createDBAlias(ctx, deps, r, claims, workspace, req)
			if err != nil {
				writeCatalogError(deps, w, err)
				return
			}
			if !activateCatalogChange(deps, w) {
				return
			}
			view, err = h.aliasViewByName(ctx, deps, claims, view.Name)
			if err != nil {
				writeCatalogPresentationError(deps, w, err)
				return
			}
			writeAdminJSON(w, http.StatusCreated, view)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := rest[0]
	switch r.Method {
	case http.MethodGet:
		visibility, ok := h.resolveWorkspaceFilter(deps, w, r, claims)
		if !ok {
			return
		}
		views, err := h.mergedAliasViews(ctx, deps, visibility)
		if err != nil {
			http.Error(w, "could not load alias", http.StatusInternalServerError)
			return
		}
		for _, v := range views {
			if v.Name == name {
				writeAdminJSON(w, http.StatusOK, v)
				return
			}
		}
		http.Error(w, "alias not found", http.StatusNotFound)
	case http.MethodPut:
		var req adminAliasUpsert
		if err := decodeAdminCatalogBody(r, &req); err != nil {
			writeCatalogWriteBodyError(w, err)
			return
		}
		req.Name = name
		view, err := h.updateDBAlias(ctx, deps, r, claims, req)
		if err != nil {
			writeCatalogError(deps, w, err)
			return
		}
		if !activateCatalogChange(deps, w) {
			return
		}
		view, err = h.aliasViewByName(ctx, deps, claims, view.Name)
		if err != nil {
			writeCatalogPresentationError(deps, w, err)
			return
		}
		writeAdminJSON(w, http.StatusOK, view)
	case http.MethodDelete:
		target, err := h.resolveCatalogWriteTarget(deps, r, claims, catalogAliasResource, name, nil)
		if err != nil {
			writeCatalogError(deps, w, err)
			return
		}
		if err := deps.AdminStore.DeleteAlias(ctx, target.id); err != nil {
			writeCatalogError(deps, w, catalogAliasResource.targetReadError(err))
			return
		}
		if !activateCatalogChange(deps, w) {
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type adminAliasTargetUpsert struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (req *adminAliasTargetUpsert) UnmarshalJSON(raw []byte) error {
	type plainUpsert adminAliasTargetUpsert
	var decoded plainUpsert
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	if err := adminJSONKeyError(raw, "provider model"); err != nil {
		return err
	}
	*req = adminAliasTargetUpsert(decoded)
	return nil
}

type adminSessionAffinityUpsert struct {
	Headers []string `json:"headers"`
}

type adminAliasUpsert struct {
	Name               string                      `json:"name"`
	WorkspaceID        catalogBodyWorkspaceID      `json:"workspace_id"`
	Algorithm          *string                     `json:"algorithm"`
	RetryStatusCodes   []int                       `json:"retry_status_codes"`
	Providers          []string                    `json:"providers"`
	Model              *string                     `json:"model"`
	SessionAffinity    *adminSessionAffinityUpsert `json:"session_affinity"`
	EncryptedReasoning json.RawMessage             `json:"encrypted_reasoning"`
	Targets            *[]adminAliasTargetUpsert   `json:"targets"`
	inputJSONError     error
}

func (req *adminAliasUpsert) UnmarshalJSON(raw []byte) error {
	type plainUpsert adminAliasUpsert
	var decoded plainUpsert
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	decoded.inputJSONError = adminJSONKeyError(raw, adminAliasWritableFields)
	*req = adminAliasUpsert(decoded)
	return nil
}

func (h *Handler) mergedAliasViews(ctx context.Context, deps Dependencies, visibility workspaceVisibility) ([]adminAliasView, error) {
	if visibility.empty() {
		return []adminAliasView{}, nil
	}
	names := workspaceNameMap(ctx, deps, visibility)
	rows, err := deps.AdminStore.ListAliases(ctx)
	if err != nil {
		return nil, err
	}
	managedNames := make(map[string]bool, len(rows))
	for _, row := range rows {
		managedNames[row.Name] = true
	}
	views := map[string]*adminAliasView{}
	for _, a := range deps.Catalog.Aliases() {
		if managedNames[a.Name] && !a.DatabaseManaged {
			return nil, fmt.Errorf("alias name conflicts with static config")
		}
	}
	if visibility.allows(store.SystemWorkspaceID) {
		for _, a := range deps.Catalog.Aliases() {
			if managedNames[a.Name] || a.DatabaseManaged {
				continue
			}
			v := adminAliasView{Name: a.Name, Algorithm: string(a.Algorithm), RetryStatusCodes: append([]int(nil), a.RetryStatusCodes...), Source: "config"}
			if a.SessionAffinity != nil {
				v.SessionAffinity = &adminSessionAffinity{Headers: append([]string(nil), a.SessionAffinity.Headers...)}
			}
			setAliasReasoningView(&v, a.EncryptedReasoning)
			for _, t := range a.Targets {
				v.Targets = append(v.Targets, adminAliasTargetView{Provider: t.Provider, Model: t.Model})
			}
			vv := v
			views[a.Name] = &vv
		}
	}
	catalogs := make(map[uuid.UUID]config.Catalog)
	for _, a := range rows {
		if !visibility.allows(a.WorkspaceID) {
			continue
		}
		targets, err := deps.AdminStore.ListAliasTargets(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		catalog, ok := catalogs[a.WorkspaceID]
		if !ok {
			catalog, err = h.mergedAliasCatalog(ctx, deps, a.WorkspaceID)
			if err != nil {
				return nil, err
			}
			catalogs[a.WorkspaceID] = catalog
		}
		for _, target := range targets {
			if !catalog.ModelVisibleInWorkspace(target.Provider+"/"+target.Model, identity.InWorkspace(a.WorkspaceID)) {
				return nil, fmt.Errorf("alias target is not defined in the destination workspace")
			}
		}
		v := adminAliasView{Name: a.Name, Algorithm: a.Algorithm, RetryStatusCodes: append([]int(nil), a.RetryStatusCodes...), Source: "database", WorkspaceID: a.WorkspaceID.String(), WorkspaceName: names[a.WorkspaceID.String()]}
		if jsonBlockPresent(a.SessionAffinity) {
			var affinity adminSessionAffinity
			if unmarshalJSONBlock(a.SessionAffinity, &affinity) {
				v.SessionAffinity = &affinity
			}
		}
		reasoning, err := config.DecodeEncryptedReasoningJSON(a.EncryptedReasoning, fmt.Sprintf("alias %q", a.Name))
		if err != nil {
			return nil, err
		}
		setAliasReasoningView(&v, reasoning)
		for _, t := range targets {
			v.Targets = append(v.Targets, adminAliasTargetView{Provider: t.Provider, Model: t.Model})
		}
		vv := v
		views[a.Name] = &vv
	}
	out := make([]adminAliasView, 0, len(views))
	for _, v := range views {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (h *Handler) mergedAliasCatalog(ctx context.Context, deps Dependencies, workspaceID uuid.UUID, targets ...config.AliasTarget) (config.Catalog, error) {
	inventory, err := loadAdminProviderInventory(ctx, deps)
	if err != nil {
		return config.Catalog{}, err
	}
	providers := make([]config.Provider, 0, len(inventory.static)+len(inventory.rows))
	disabled := make([]config.Provider, 0)
	if len(targets) != 0 {
		seen := make(map[string]bool)
		var add func(string) error
		add = func(name string) error {
			if seen[name] {
				return nil
			}
			seen[name] = true
			if row, ok := inventory.rowsByName[name]; ok {
				if row.WorkspaceID != workspaceID {
					return nil
				}
			} else if _, ok := inventory.static[name]; !ok || workspaceID != store.SystemWorkspaceID {
				return nil
			}
			p, err := inventory.provider(ctx, name)
			if err != nil {
				return err
			}
			if p.Enabled {
				providers = append(providers, p)
			} else {
				disabled = append(disabled, p)
			}
			if p.Extends != "" {
				return add(p.Extends)
			}
			return nil
		}
		for _, target := range targets {
			if err := add(target.Provider); err != nil {
				return config.Catalog{}, err
			}
		}
		return config.NewCatalog(providers, disabled, nil), nil
	}
	if workspaceID == store.SystemWorkspaceID {
		for _, provider := range inventory.static {
			if provider.Enabled {
				providers = append(providers, provider)
			} else {
				disabled = append(disabled, provider)
			}
		}
	}
	for _, row := range inventory.rows {
		if row.WorkspaceID != workspaceID {
			continue
		}
		provider, err := inventory.provider(ctx, row.Name)
		if err != nil {
			return config.Catalog{}, err
		}
		if provider.Enabled {
			providers = append(providers, provider)
		} else {
			disabled = append(disabled, provider)
		}
	}
	return config.NewCatalog(providers, disabled, nil), nil
}

func expandAliasTargets(providers []string, model string, targets *[]adminAliasTargetUpsert) ([]adminAliasTargetUpsert, error) {
	if targets != nil {
		if len(providers) > 0 || model != "" {
			return nil, errBad("alias providers/model shorthand cannot be combined with target blocks")
		}
		return *targets, nil
	}
	if len(providers) == 0 && model == "" {
		return nil, errBad("alias targets or providers/model shorthand is required")
	}
	if len(providers) == 0 || model == "" {
		return nil, errBad("alias providers and model must be set together")
	}
	seen := make(map[string]bool, len(providers))
	out := make([]adminAliasTargetUpsert, 0, len(providers))
	for _, provider := range providers {
		provider = strings.TrimSpace(provider)
		if provider == "" {
			return nil, errBad("alias providers must not contain empty values")
		}
		if seen[provider] {
			return nil, errBad("alias providers must not contain duplicates")
		}
		seen[provider] = true
		out = append(out, adminAliasTargetUpsert{Provider: provider, Model: strings.TrimSpace(model)})
	}
	return out, nil
}

func buildAliasSessionAffinity(req *adminSessionAffinityUpsert) (*config.SessionAffinity, error) {
	if req == nil {
		return nil, nil
	}
	if req.Headers == nil {
		return &config.SessionAffinity{Headers: append([]string(nil), config.DefaultSessionAffinityHeaders...)}, nil
	}
	headers := make([]string, 0, len(req.Headers))
	for _, header := range req.Headers {
		trimmed := strings.ToLower(strings.TrimSpace(header))
		if trimmed == "" {
			return nil, errBad("session_affinity.headers must not contain empty values")
		}
		headers = append(headers, trimmed)
	}
	return &config.SessionAffinity{Headers: headers}, nil
}

func (h *Handler) buildConfigAlias(name, algorithm string, req adminAliasUpsert) (config.Alias, []store.DBAliasTarget, error) {
	if req.inputJSONError != nil {
		return config.Alias{}, nil, req.inputJSONError
	}
	targets, err := expandAliasTargets(req.Providers, stringValue(req.Model), req.Targets)
	if err != nil {
		return config.Alias{}, nil, err
	}
	if len(targets) == 0 {
		return config.Alias{}, nil, errBad("at least one target is required")
	}
	retryCodes := append([]int(nil), req.RetryStatusCodes...)
	if len(retryCodes) == 0 {
		retryCodes = config.DefaultRetryStatusCodes()
	}
	affinity, err := buildAliasSessionAffinity(req.SessionAffinity)
	if err != nil {
		return config.Alias{}, nil, err
	}
	reasoning, err := config.DecodeEncryptedReasoningJSON(req.EncryptedReasoning, fmt.Sprintf("alias %q", name))
	if err != nil {
		return config.Alias{}, nil, err
	}
	alias := config.Alias{Name: name, Algorithm: config.Algorithm(algorithm), RetryStatusCodes: retryCodes, SessionAffinity: affinity, EncryptedReasoning: reasoning}
	rows := make([]store.DBAliasTarget, 0, len(targets))
	for _, t := range targets {
		provider := strings.TrimSpace(t.Provider)
		model := strings.TrimSpace(t.Model)
		if provider == "" || model == "" {
			return config.Alias{}, nil, errBad("alias targets must set provider and model")
		}
		alias.Targets = append(alias.Targets, config.AliasTarget{Provider: provider, Model: model})
		rows = append(rows, store.DBAliasTarget{Provider: provider, Model: model})
	}
	return config.NormalizeAlias(alias), rows, nil
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func checkDisabledAliasTargets(catalog config.Catalog, alias config.Alias) error {
	disabled := make(map[string]bool)
	for _, p := range catalog.DisabledProviders() {
		disabled[p.Name] = true
	}
	for _, t := range alias.Targets {
		if disabled[t.Provider] {
			return errBad("alias target provider " + t.Provider + " is disabled")
		}
	}
	return nil
}

func (h *Handler) createDBAlias(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, workspace store.Workspace, req adminAliasUpsert) (adminAliasView, error) {
	name := strings.TrimSpace(req.Name)
	if !config.IsLowercaseName(name) {
		return adminAliasView{}, errBad("invalid alias name: must be lowercase, no spaces, no '/', and start with [a-z0-9]")
	}
	if _, err := deps.AdminStore.GetAlias(ctx, name); err == nil {
		return adminAliasView{}, errBad("alias already exists")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return adminAliasView{}, catalogStorageError{err}
	}
	for _, a := range deps.Catalog.Aliases() {
		if a.Name == name && !a.DatabaseManaged {
			return adminAliasView{}, errBad("alias exists in static config")
		}
	}
	algorithm := "round_robin"
	if req.Algorithm != nil {
		algorithm = strings.TrimSpace(*req.Algorithm)
	}
	if algorithm != string(config.AlgorithmRoundRobin) && algorithm != string(config.AlgorithmLeastConnections) {
		return adminAliasView{}, errBad("invalid algorithm: must be round_robin or least_connections")
	}
	alias, targetRows, err := h.buildConfigAlias(name, algorithm, req)
	if err != nil {
		return adminAliasView{}, err
	}
	alias.Workspace = identity.InWorkspace(workspace.ID)
	for _, code := range alias.RetryStatusCodes {
		if code < 400 || code > 599 {
			return adminAliasView{}, errBad("retry status codes must be between 400 and 599")
		}
	}
	catalog, err := h.mergedAliasCatalog(ctx, deps, workspace.ID, alias.Targets...)
	if err != nil {
		return adminAliasView{}, err
	}
	if err := checkDisabledAliasTargets(catalog, alias); err != nil {
		return adminAliasView{}, err
	}
	if err := config.ValidateDynamicAlias(alias, catalog); err != nil {
		return adminAliasView{}, err
	}
	row := &store.DBAlias{Name: name, Algorithm: algorithm, RetryStatusCodes: append([]int(nil), alias.RetryStatusCodes...), WorkspaceID: workspace.ID}
	if alias.SessionAffinity != nil {
		raw, err := json.Marshal(adminSessionAffinity{Headers: alias.SessionAffinity.Headers})
		if err != nil {
			return adminAliasView{}, err
		}
		row.SessionAffinity = raw
	}
	row.EncryptedReasoning, err = config.EncodeEncryptedReasoningJSON(alias.EncryptedReasoning, fmt.Sprintf("alias %q", name))
	if err != nil {
		return adminAliasView{}, err
	}
	if err := deps.AdminStore.CreateAlias(ctx, row, targetRows, dbmerge.CatalogEdits(deps.Catalog, deps.MultiTenancy.AllowedUpstreamOrigins...)); err != nil {
		return adminAliasView{}, catalogStorageError{err}
	}
	return adminAliasView{Name: name}, nil
}

func (h *Handler) updateDBAlias(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, req adminAliasUpsert) (adminAliasView, error) {
	name := strings.TrimSpace(req.Name)
	if req.inputJSONError != nil {
		return adminAliasView{}, req.inputJSONError
	}
	target, err := h.resolveCatalogWriteTarget(deps, r, claims, catalogAliasResource, name, req.WorkspaceID.value)
	if err != nil {
		return adminAliasView{}, err
	}
	current, err := deps.AdminStore.GetAliasByIDInWorkspace(ctx, target.id, target.workspaceID)
	if err != nil {
		return adminAliasView{}, catalogAliasResource.targetReadError(err)
	}
	effective := req
	if effective.Algorithm == nil {
		algorithm := current.Algorithm
		effective.Algorithm = &algorithm
	}
	if effective.RetryStatusCodes == nil {
		effective.RetryStatusCodes = append([]int(nil), current.RetryStatusCodes...)
	}
	if effective.SessionAffinity == nil && jsonBlockPresent(current.SessionAffinity) {
		var affinity adminSessionAffinity
		if !unmarshalJSONBlock(current.SessionAffinity, &affinity) {
			return adminAliasView{}, errBad("stored session_affinity is corrupt")
		}
		effective.SessionAffinity = &adminSessionAffinityUpsert{Headers: affinity.Headers}
	}
	if len(effective.EncryptedReasoning) == 0 {
		effective.EncryptedReasoning = append(json.RawMessage(nil), current.EncryptedReasoning...)
	}
	if effective.Targets == nil && len(effective.Providers) == 0 && effective.Model == nil {
		rows, err := deps.AdminStore.ListAliasTargets(ctx, current.ID)
		if err != nil {
			return adminAliasView{}, catalogStorageError{err}
		}
		kept := make([]adminAliasTargetUpsert, 0, len(rows))
		for _, t := range rows {
			kept = append(kept, adminAliasTargetUpsert{Provider: t.Provider, Model: t.Model})
		}
		effective.Targets = &kept
	}
	algorithm := strings.TrimSpace(*effective.Algorithm)
	if algorithm != string(config.AlgorithmRoundRobin) && algorithm != string(config.AlgorithmLeastConnections) {
		return adminAliasView{}, errBad("invalid algorithm: must be round_robin or least_connections")
	}
	alias, targetRows, err := h.buildConfigAlias(name, algorithm, effective)
	if err != nil {
		return adminAliasView{}, err
	}
	alias.Workspace = identity.InWorkspace(current.WorkspaceID)
	for _, code := range alias.RetryStatusCodes {
		if code < 400 || code > 599 {
			return adminAliasView{}, errBad("retry status codes must be between 400 and 599")
		}
	}
	catalog, err := h.mergedAliasCatalog(ctx, deps, current.WorkspaceID, alias.Targets...)
	if err != nil {
		return adminAliasView{}, err
	}
	if err := checkDisabledAliasTargets(catalog, alias); err != nil {
		return adminAliasView{}, err
	}
	if err := config.ValidateDynamicAlias(alias, catalog); err != nil {
		return adminAliasView{}, err
	}
	current.Algorithm = algorithm
	current.RetryStatusCodes = append([]int(nil), alias.RetryStatusCodes...)
	if alias.SessionAffinity != nil {
		raw, err := json.Marshal(adminSessionAffinity{Headers: alias.SessionAffinity.Headers})
		if err != nil {
			return adminAliasView{}, err
		}
		current.SessionAffinity = raw
	} else {
		current.SessionAffinity = nil
	}
	if len(req.EncryptedReasoning) != 0 {
		current.EncryptedReasoning, err = config.EncodeEncryptedReasoningJSON(alias.EncryptedReasoning, fmt.Sprintf("alias %q", name))
		if err != nil {
			return adminAliasView{}, err
		}
	}
	if err := deps.AdminStore.UpdateAlias(ctx, &current, targetRows, dbmerge.CatalogEdits(deps.Catalog, deps.MultiTenancy.AllowedUpstreamOrigins...)); err != nil {
		return adminAliasView{}, catalogStorageError{err}
	}
	return adminAliasView{Name: name}, nil
}

func (h *Handler) aliasViewByName(ctx context.Context, deps Dependencies, claims *adminauth.Claims, name string) (adminAliasView, error) {
	visibility, err := resolveWorkspaceVisibility(ctx, deps, claims)
	if err != nil {
		return adminAliasView{}, err
	}
	views, err := h.mergedAliasViews(ctx, deps, visibility)
	if err != nil {
		return adminAliasView{}, err
	}
	for _, v := range views {
		if v.Name == name {
			return v, nil
		}
	}
	return adminAliasView{}, errBad("alias not found after write")
}
