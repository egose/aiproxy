package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

type adminAliasTargetView struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type adminAliasView struct {
	Name               string                 `json:"name"`
	Algorithm          string                 `json:"algorithm"`
	RetryStatusCodes   []int                  `json:"retry_status_codes,omitempty"`
	SessionAffinity    *adminSessionAffinity  `json:"session_affinity,omitempty"`
	EncryptedReasoning *adminEncryptedReason  `json:"encrypted_reasoning,omitempty"`
	Source             string                 `json:"source"`
	OrgID              string                 `json:"org_id,omitempty"`
	OrgName            string                 `json:"org_name,omitempty"`
	Targets            []adminAliasTargetView `json:"targets"`
}

type adminSessionAffinity struct {
	Headers []string `json:"headers"`
}

type adminEncryptedReason struct {
	Passthrough      bool     `json:"passthrough"`
	OnCallerMismatch string   `json:"on_caller_mismatch,omitempty"`
	MatchMessages    []string `json:"match_messages,omitempty"`
}

func (h *Handler) adminAliases(deps Dependencies, w http.ResponseWriter, r *http.Request, rest []string) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	static := map[string]bool{}
	for _, a := range deps.Catalog.Aliases() {
		static[a.Name] = true
	}
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			filterOrg, ok := h.resolveOrgFilter(deps, w, r, claims, r.URL.Query().Get("org_id"))
			if !ok {
				return
			}
			views, err := h.mergedAliasViews(ctx, deps, r, claims, filterOrg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"aliases": views})
			return
		case http.MethodPost:
			var req adminAliasUpsert
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			org, ok := h.resolveWriteOrg(deps, w, r, claims, req.OrgID)
			if !ok {
				return
			}
			view, err := h.createDBAlias(ctx, deps, r, claims, org, req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if !h.activateChange(deps, w) {
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
		filterOrg, ok := h.resolveOrgFilter(deps, w, r, claims, r.URL.Query().Get("org_id"))
		if !ok {
			return
		}
		views, err := h.mergedAliasViews(ctx, deps, r, claims, filterOrg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		req.Name = name
		view, err := h.updateDBAlias(ctx, deps, r, claims, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !h.activateChange(deps, w) {
			return
		}
		writeAdminJSON(w, http.StatusOK, view)
	case http.MethodDelete:
		a, err := deps.AdminStore.GetAlias(ctx, name)
		if err != nil {
			if static[name] {
				http.Error(w, "config-managed: edit the HCL file", http.StatusForbidden)
				return
			}
			http.Error(w, "alias not found", http.StatusNotFound)
			return
		}
		if !h.requireResourceOrg(deps, w, r, claims, a.OrgID) {
			return
		}
		if err := deps.AdminStore.DeleteAlias(ctx, a.ID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !h.activateChange(deps, w) {
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

type adminSessionAffinityUpsert struct {
	Headers []string `json:"headers"`
}

type adminEncryptedReasonUpsert struct {
	Passthrough      *bool    `json:"passthrough"`
	OnCallerMismatch *string  `json:"on_caller_mismatch"`
	MatchMessages    []string `json:"match_messages"`
}

type adminAliasUpsert struct {
	Name               string                      `json:"name"`
	OrgID              string                      `json:"org_id"`
	Algorithm          *string                     `json:"algorithm"`
	RetryStatusCodes   []int                       `json:"retry_status_codes"`
	Providers          []string                    `json:"providers"`
	Model              *string                     `json:"model"`
	SessionAffinity    *adminSessionAffinityUpsert `json:"session_affinity"`
	EncryptedReasoning *adminEncryptedReasonUpsert `json:"encrypted_reasoning"`
	Targets            *[]adminAliasTargetUpsert   `json:"targets"`
}

func (h *Handler) mergedAliasViews(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, filterOrg string) ([]adminAliasView, error) {
	visible, _ := h.visibleOrgIDs(deps, r, claims)
	names := h.orgNameMap(deps, r, claims)
	views := map[string]*adminAliasView{}
	if h.canSeeSystemOrg(deps, r, claims, filterOrg) {
		for _, a := range deps.Catalog.Aliases() {
			v := adminAliasView{Name: a.Name, Algorithm: string(a.Algorithm), RetryStatusCodes: append([]int(nil), a.RetryStatusCodes...), Source: "config"}
			if a.SessionAffinity != nil {
				v.SessionAffinity = &adminSessionAffinity{Headers: append([]string(nil), a.SessionAffinity.Headers...)}
			}
			if a.EncryptedReasoning != nil {
				v.EncryptedReasoning = &adminEncryptedReason{
					Passthrough: a.EncryptedReasoning.Passthrough, OnCallerMismatch: a.EncryptedReasoning.OnCallerMismatch,
					MatchMessages: append([]string(nil), a.EncryptedReasoning.MatchMessages...),
				}
			}
			for _, t := range a.Targets {
				v.Targets = append(v.Targets, adminAliasTargetView{Provider: t.Provider, Model: t.Model})
			}
			vv := v
			views[a.Name] = &vv
		}
	}
	rows, err := deps.AdminStore.ListAliases(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range rows {
		if filterOrg != "" && a.OrgID.String() != filterOrg {
			continue
		}
		if visible != nil && !visible[a.OrgID.String()] {
			continue
		}
		targets, err := deps.AdminStore.ListAliasTargets(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		v := adminAliasView{Name: a.Name, Algorithm: a.Algorithm, RetryStatusCodes: append([]int(nil), a.RetryStatusCodes...), Source: "database", OrgID: a.OrgID.String(), OrgName: names[a.OrgID.String()]}
		if jsonBlockPresent(a.SessionAffinity) {
			var affinity adminSessionAffinity
			if unmarshalJSONBlock(a.SessionAffinity, &affinity) {
				v.SessionAffinity = &affinity
			}
		}
		if jsonBlockPresent(a.EncryptedReasoning) {
			var reasoning adminEncryptedReason
			if unmarshalJSONBlock(a.EncryptedReasoning, &reasoning) {
				v.EncryptedReasoning = &reasoning
			}
		}
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

func (h *Handler) mergedAliasCatalog(ctx context.Context, deps Dependencies) (config.Catalog, error) {
	providers := append([]config.Provider(nil), deps.Catalog.Providers()...)
	disabled := append([]config.Provider(nil), deps.Catalog.DisabledProviders()...)
	rows, err := deps.AdminStore.ListProviders(ctx)
	if err != nil {
		return config.Catalog{}, err
	}
	for _, p := range rows {
		models, err := deps.AdminStore.ListProviderModels(ctx, p.ID)
		if err != nil {
			return config.Catalog{}, err
		}
		entry := config.Provider{Type: config.ProviderType(p.Type), Name: p.Name, Enabled: p.Enabled, ModelByName: make(map[string]config.Model)}
		for _, m := range models {
			model := config.Model{Name: m.Name, UpstreamName: m.UpstreamName, Protocol: config.ModelProtocol(m.Protocol)}
			for _, c := range m.Capabilities {
				model.Capabilities = append(model.Capabilities, config.Capability(c))
			}
			entry.Models = append(entry.Models, model)
			entry.ModelByName[m.Name] = model
		}
		if p.Enabled {
			providers = append(providers, entry)
		} else {
			disabled = append(disabled, entry)
		}
	}
	return config.NewCatalog(providers, disabled, deps.Catalog.Aliases()), nil
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

func buildAliasEncryptedReasoning(req *adminEncryptedReasonUpsert) *config.EncryptedReasoning {
	if req == nil {
		return nil
	}
	er := &config.EncryptedReasoning{Passthrough: true, OnCallerMismatch: config.EncryptedReasoningFail}
	if req.Passthrough != nil {
		er.Passthrough = *req.Passthrough
	}
	if req.OnCallerMismatch != nil {
		er.OnCallerMismatch = strings.TrimSpace(*req.OnCallerMismatch)
	}
	if req.MatchMessages != nil {
		er.MatchMessages = append([]string(nil), req.MatchMessages...)
	}
	return er
}

func (h *Handler) buildConfigAlias(name, algorithm string, req adminAliasUpsert) (config.Alias, []store.DBAliasTarget, error) {
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
	alias := config.Alias{Name: name, Algorithm: config.Algorithm(algorithm), RetryStatusCodes: retryCodes, SessionAffinity: affinity, EncryptedReasoning: buildAliasEncryptedReasoning(req.EncryptedReasoning)}
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

func (h *Handler) checkDisabledTargets(deps Dependencies, ctx context.Context, alias config.Alias) error {
	disabled := make(map[string]bool)
	for _, p := range deps.Catalog.DisabledProviders() {
		disabled[p.Name] = true
	}
	rows, err := deps.AdminStore.ListProviders(ctx)
	if err != nil {
		return err
	}
	for _, p := range rows {
		if !p.Enabled {
			disabled[p.Name] = true
		}
	}
	for _, t := range alias.Targets {
		if disabled[t.Provider] {
			return errBad("alias target provider " + t.Provider + " is disabled")
		}
	}
	return nil
}

func (h *Handler) createDBAlias(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, org store.Organization, req adminAliasUpsert) (adminAliasView, error) {
	name := strings.TrimSpace(req.Name)
	if !config.IsLowercaseName(name) {
		return adminAliasView{}, errBad("invalid alias name: must be lowercase, no spaces, no '/', and start with [a-z0-9]")
	}
	if _, err := deps.AdminStore.GetAlias(ctx, name); err == nil {
		return adminAliasView{}, errBad("alias already exists")
	}
	for _, a := range deps.Catalog.Aliases() {
		if a.Name == name {
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
	for _, code := range alias.RetryStatusCodes {
		if code < 400 || code > 599 {
			return adminAliasView{}, errBad("retry status codes must be between 400 and 599")
		}
	}
	if err := h.checkDisabledTargets(deps, ctx, alias); err != nil {
		return adminAliasView{}, err
	}
	catalog, err := h.mergedAliasCatalog(ctx, deps)
	if err != nil {
		return adminAliasView{}, err
	}
	if err := config.ValidateDynamicAlias(alias, catalog); err != nil {
		return adminAliasView{}, err
	}
	row := &store.DBAlias{Name: name, Algorithm: algorithm, RetryStatusCodes: append([]int(nil), alias.RetryStatusCodes...), OrgID: org.ID}
	if alias.SessionAffinity != nil {
		raw, err := json.Marshal(adminSessionAffinity{Headers: alias.SessionAffinity.Headers})
		if err != nil {
			return adminAliasView{}, err
		}
		row.SessionAffinity = raw
	}
	if alias.EncryptedReasoning != nil {
		raw, err := json.Marshal(adminEncryptedReason{
			Passthrough: alias.EncryptedReasoning.Passthrough, OnCallerMismatch: alias.EncryptedReasoning.OnCallerMismatch,
			MatchMessages: alias.EncryptedReasoning.MatchMessages,
		})
		if err != nil {
			return adminAliasView{}, err
		}
		row.EncryptedReasoning = raw
	}
	if err := deps.AdminStore.CreateAlias(ctx, row, targetRows); err != nil {
		return adminAliasView{}, err
	}
	return h.aliasViewByName(ctx, deps, r, claims, name)
}

func (h *Handler) updateDBAlias(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, req adminAliasUpsert) (adminAliasView, error) {
	name := strings.TrimSpace(req.Name)
	current, err := deps.AdminStore.GetAlias(ctx, name)
	if err != nil {
		for _, a := range deps.Catalog.Aliases() {
			if a.Name == name {
				return adminAliasView{}, errBad("config-managed: edit the HCL file")
			}
		}
		return adminAliasView{}, errBad("alias not found")
	}
	if !h.canWriteOrg(deps, ctx, claims, current.OrgID) {
		return adminAliasView{}, errBad("organization admin required")
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
	if effective.EncryptedReasoning == nil && jsonBlockPresent(current.EncryptedReasoning) {
		var reasoning adminEncryptedReason
		if !unmarshalJSONBlock(current.EncryptedReasoning, &reasoning) {
			return adminAliasView{}, errBad("stored encrypted_reasoning is corrupt")
		}
		passthrough := reasoning.Passthrough
		mismatch := reasoning.OnCallerMismatch
		effective.EncryptedReasoning = &adminEncryptedReasonUpsert{Passthrough: &passthrough, OnCallerMismatch: &mismatch, MatchMessages: reasoning.MatchMessages}
	}
	if effective.Targets == nil && len(effective.Providers) == 0 && effective.Model == nil {
		rows, err := deps.AdminStore.ListAliasTargets(ctx, current.ID)
		if err != nil {
			return adminAliasView{}, err
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
	for _, code := range alias.RetryStatusCodes {
		if code < 400 || code > 599 {
			return adminAliasView{}, errBad("retry status codes must be between 400 and 599")
		}
	}
	if err := h.checkDisabledTargets(deps, ctx, alias); err != nil {
		return adminAliasView{}, err
	}
	catalog, err := h.mergedAliasCatalog(ctx, deps)
	if err != nil {
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
	if alias.EncryptedReasoning != nil {
		raw, err := json.Marshal(adminEncryptedReason{
			Passthrough: alias.EncryptedReasoning.Passthrough, OnCallerMismatch: alias.EncryptedReasoning.OnCallerMismatch,
			MatchMessages: alias.EncryptedReasoning.MatchMessages,
		})
		if err != nil {
			return adminAliasView{}, err
		}
		current.EncryptedReasoning = raw
	} else {
		current.EncryptedReasoning = nil
	}
	if err := deps.AdminStore.UpdateAlias(ctx, &current, targetRows); err != nil {
		return adminAliasView{}, err
	}
	return h.aliasViewByName(ctx, deps, r, claims, name)
}

func (h *Handler) aliasViewByName(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, name string) (adminAliasView, error) {
	views, err := h.mergedAliasViews(ctx, deps, r, claims, "")
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
