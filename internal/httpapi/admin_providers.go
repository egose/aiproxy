package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/store"
)

type adminModelView struct {
	Name         string             `json:"name"`
	DisplayName  string             `json:"display_name,omitempty"`
	UpstreamName string             `json:"upstream_name,omitempty"`
	Protocol     string             `json:"protocol,omitempty"`
	Capabilities []string           `json:"capabilities,omitempty"`
	Pricing      map[string]float64 `json:"pricing,omitempty"`
}

type adminHealthcheckView struct {
	Path              string `json:"path"`
	Method            string `json:"method,omitempty"`
	ExpectedStatus    int    `json:"expected_status,omitempty"`
	ExpectedBody      string `json:"expected_body,omitempty"`
	Interval          string `json:"interval,omitempty"`
	Timeout           string `json:"timeout,omitempty"`
	FailureThreshold  int    `json:"failure_threshold,omitempty"`
	SuccessThreshold  int    `json:"success_threshold,omitempty"`
	SendAuthorization bool   `json:"send_authorization,omitempty"`
}

type adminProviderView struct {
	Name                  string                `json:"name"`
	Type                  string                `json:"type"`
	DisplayName           string                `json:"display_name,omitempty"`
	BaseURL               string                `json:"base_url,omitempty"`
	UpstreamHeaderTimeout string                `json:"upstream_header_timeout,omitempty"`
	UserAgent             string                `json:"user_agent,omitempty"`
	ForwardUserAgent      bool                  `json:"forward_user_agent"`
	ForwardHeaders        []string              `json:"forward_headers,omitempty"`
	Extends               string                `json:"extends,omitempty"`
	APIKeyRefPath         string                `json:"api_key_ref_path,omitempty"`
	APIKeyRefKey          string                `json:"api_key_ref_key,omitempty"`
	CopilotCredentialPath string                `json:"copilot_credential_path,omitempty"`
	CopilotCredentialName string                `json:"copilot_credential_name,omitempty"`
	Enabled               bool                  `json:"enabled"`
	Source                string                `json:"source"`
	OrgID                 string                `json:"org_id,omitempty"`
	OrgName               string                `json:"org_name,omitempty"`
	HasCredential         bool                  `json:"has_credential"`
	Healthcheck           *adminHealthcheckView `json:"healthcheck,omitempty"`
	Models                []adminModelView      `json:"models"`
}

func knownProviderType(t string) bool {
	for _, pt := range config.ProviderTypes() {
		if string(pt) == t {
			return true
		}
	}
	return false
}

func sortedProviderViews(deps Dependencies, views map[string]*adminProviderView) []adminProviderView {
	out := make([]adminProviderView, 0, len(views))
	for _, v := range views {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func providerModelPricingView(raw []byte) map[string]float64 {
	if !jsonBlockPresent(raw) {
		return nil
	}
	var rates map[string]float64
	if !unmarshalJSONBlock(raw, &rates) || len(rates) == 0 {
		return nil
	}
	return rates
}

func providerHealthcheckView(raw []byte) *adminHealthcheckView {
	if !jsonBlockPresent(raw) {
		return nil
	}
	var hc dbHealthcheck
	if !unmarshalJSONBlock(raw, &hc) || hc.Path == "" {
		return nil
	}
	return &adminHealthcheckView{
		Path: hc.Path, Method: hc.Method, ExpectedStatus: hc.ExpectedStatus,
		ExpectedBody: hc.ExpectedBody, Interval: hc.Interval, Timeout: hc.Timeout,
		FailureThreshold: hc.FailureThreshold, SuccessThreshold: hc.SuccessThreshold,
		SendAuthorization: hc.SendAuthorization,
	}
}

func formatTimeoutMs(ms *int64) string {
	if ms == nil {
		return ""
	}
	return (time.Duration(*ms) * time.Millisecond).String()
}

func (h *Handler) adminProviderTypes(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	if _, ok := h.adminClaims(deps, r); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	infos := config.DescribeProviderTypes()
	out := make([]map[string]interface{}, 0, len(infos))
	for _, info := range infos {
		protocols := make([]string, 0, len(info.Protocols))
		for _, protocol := range info.Protocols {
			protocols = append(protocols, string(protocol))
		}
		defaults := make([]string, 0, len(info.DefaultCapabilities))
		for _, c := range info.DefaultCapabilities {
			defaults = append(defaults, string(c))
		}
		supported := make([]string, 0, len(info.SupportedCapabilities))
		for _, c := range info.SupportedCapabilities {
			supported = append(supported, string(c))
		}
		out = append(out, map[string]interface{}{
			"type": info.Type, "credential": info.Credential,
			"requires_base_url": info.RequiresBaseURL, "supports_healthcheck": info.SupportsHealthcheck,
			"model_protocol_required": info.ModelProtocolRequired, "protocols": protocols,
			"default_capabilities": defaults, "supported_capabilities": supported,
		})
	}
	writeAdminJSON(w, http.StatusOK, map[string]interface{}{"provider_types": out})
}

func (h *Handler) adminProviders(deps Dependencies, w http.ResponseWriter, r *http.Request, rest []string) {
	claims, ok := h.adminClaims(deps, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	if len(rest) == 0 || rest[0] == "" {
		switch r.Method {
		case http.MethodGet:
			filterOrg, ok := h.resolveOrgFilter(deps, w, r, claims, r.URL.Query().Get("org_id"))
			if !ok {
				return
			}
			views, err := h.mergedProviderViews(ctx, deps, r, claims, filterOrg)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]interface{}{"providers": views})
			return
		case http.MethodPost:
			var req adminProviderUpsert
			if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			org, ok := h.resolveWriteOrg(deps, w, r, claims, req.OrgID)
			if !ok {
				return
			}
			view, err := h.createDBProvider(ctx, deps, r, claims, org, req)
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
	if len(rest) == 2 && rest[1] == "credential" {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req adminCredentialUpsert
		if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := h.setProviderCredential(ctx, deps, claims, name, req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !h.activateChange(deps, w) {
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	switch r.Method {
	case http.MethodGet:
		filterOrg, ok := h.resolveOrgFilter(deps, w, r, claims, r.URL.Query().Get("org_id"))
		if !ok {
			return
		}
		views, err := h.mergedProviderViews(ctx, deps, r, claims, filterOrg)
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
		http.Error(w, "provider not found", http.StatusNotFound)
	case http.MethodPut:
		var req adminProviderUpsert
		if err := json.NewDecoder(ioLimitReader(r)).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		req.Name = name
		view, err := h.updateDBProvider(ctx, deps, r, claims, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !h.activateChange(deps, w) {
			return
		}
		writeAdminJSON(w, http.StatusOK, view)
	case http.MethodDelete:
		p, err := deps.AdminStore.GetProvider(ctx, name)
		if err != nil {
			if h.isStaticProvider(deps, name) {
				http.Error(w, "config-managed: edit the HCL file", http.StatusForbidden)
				return
			}
			http.Error(w, "provider not found", http.StatusNotFound)
			return
		}
		if !h.requireResourceOrg(deps, w, r, claims, p.OrgID) {
			return
		}
		if err := deps.AdminStore.DeleteProvider(ctx, p.ID); err != nil {
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

type adminModelUpsert struct {
	Name         string             `json:"name"`
	DisplayName  string             `json:"display_name"`
	UpstreamName string             `json:"upstream_name"`
	Protocol     string             `json:"protocol"`
	Capabilities []string           `json:"capabilities"`
	Pricing      map[string]float64 `json:"pricing"`
}

type adminHealthcheckUpsert struct {
	Path              string  `json:"path"`
	Method            *string `json:"method"`
	ExpectedStatus    *int    `json:"expected_status"`
	ExpectedBody      *string `json:"expected_body"`
	Interval          *string `json:"interval"`
	Timeout           *string `json:"timeout"`
	FailureThreshold  *int    `json:"failure_threshold"`
	SuccessThreshold  *int    `json:"success_threshold"`
	SendAuthorization *bool   `json:"send_authorization"`
}

type adminAPIKeyRefUpsert struct {
	Path string `json:"path"`
	Key  string `json:"key"`
}

type adminCredentialRefUpsert struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type adminProviderUpsert struct {
	Name                  string                    `json:"name"`
	OrgID                 string                    `json:"org_id"`
	Type                  *string                   `json:"type"`
	DisplayName           *string                   `json:"display_name"`
	BaseURL               *string                   `json:"base_url"`
	UpstreamHeaderTimeout *string                   `json:"upstream_header_timeout"`
	UserAgent             *string                   `json:"user_agent"`
	ForwardUserAgent      *bool                     `json:"forward_user_agent"`
	ForwardHeaders        []string                  `json:"forward_headers"`
	APIKey                *string                   `json:"api_key"`
	APIKeyRef             *adminAPIKeyRefUpsert     `json:"api_key_ref"`
	CredentialRef         *adminCredentialRefUpsert `json:"credential_ref"`
	Enabled               *bool                     `json:"enabled"`
	Extends               *string                   `json:"extends"`
	Healthcheck           *adminHealthcheckUpsert   `json:"healthcheck"`
	Models                *[]adminModelUpsert       `json:"models"`
}

type adminCredentialUpsert struct {
	APIKey                *string `json:"api_key"`
	APIKeyRefPath         *string `json:"api_key_ref_path"`
	APIKeyRefKey          *string `json:"api_key_ref_key"`
	CopilotCredentialPath *string `json:"copilot_credential_path"`
	CopilotCredentialName *string `json:"copilot_credential_name"`
}

type dbHealthcheck struct {
	Path              string `json:"path"`
	Method            string `json:"method"`
	ExpectedStatus    int    `json:"expected_status"`
	ExpectedBody      string `json:"expected_body"`
	Interval          string `json:"interval"`
	Timeout           string `json:"timeout"`
	FailureThreshold  int    `json:"failure_threshold"`
	SuccessThreshold  int    `json:"success_threshold"`
	SendAuthorization bool   `json:"send_authorization"`
}

func (h *Handler) isStaticProvider(deps Dependencies, name string) bool {
	_, ok := deps.Catalog.Provider(name)
	return ok
}

func (h *Handler) mergedProviderViews(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, filterOrg string) ([]adminProviderView, error) {
	visible, _ := h.visibleOrgIDs(deps, r, claims)
	names := h.orgNameMap(deps, r, claims)
	views := map[string]*adminProviderView{}
	if h.canSeeSystemOrg(deps, r, claims, filterOrg) {
		for _, p := range deps.Catalog.Providers() {
			mv := adminProviderView{Name: p.Name, Type: string(p.Type), DisplayName: p.DisplayName, BaseURL: p.BaseURL, Enabled: true, Source: "config", HasCredential: true}
			if p.UpstreamHeaderTimeout > 0 {
				mv.UpstreamHeaderTimeout = p.UpstreamHeaderTimeout.String()
			}
			mv.UserAgent = p.UserAgent
			mv.ForwardUserAgent = p.ForwardUserAgent
			mv.ForwardHeaders = append([]string(nil), p.ForwardHeaders...)
			if p.Healthcheck != nil {
				mv.Healthcheck = &adminHealthcheckView{
					Path: p.Healthcheck.Path, Method: p.Healthcheck.Method, ExpectedStatus: p.Healthcheck.ExpectedStatus,
					ExpectedBody: p.Healthcheck.ExpectedBody, Interval: p.Healthcheck.Interval.String(), Timeout: p.Healthcheck.Timeout.String(),
					FailureThreshold: p.Healthcheck.FailureThreshold, SuccessThreshold: p.Healthcheck.SuccessThreshold,
					SendAuthorization: p.Healthcheck.SendAuthorization,
				}
			}
			for _, m := range p.Models {
				caps := make([]string, 0, len(m.Capabilities))
				for _, c := range m.Capabilities {
					caps = append(caps, string(c))
				}
				mv.Models = append(mv.Models, adminModelView{Name: m.Name, DisplayName: m.DisplayName, UpstreamName: m.UpstreamName, Protocol: string(m.Protocol), Capabilities: caps, Pricing: modelPricingView(m.Pricing)})
			}
			v := mv
			views[p.Name] = &v
		}
	}
	rows, err := deps.AdminStore.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range rows {
		if filterOrg != "" && p.OrgID.String() != filterOrg {
			continue
		}
		if visible != nil && !visible[p.OrgID.String()] {
			continue
		}
		models, err := deps.AdminStore.ListProviderModels(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		mv := adminProviderView{
			Name: p.Name, Type: p.Type, DisplayName: p.DisplayName, BaseURL: p.BaseURL,
			UpstreamHeaderTimeout: formatTimeoutMs(p.UpstreamTimeoutMs),
			UserAgent:             p.UserAgent, ForwardUserAgent: p.ForwardUserAgent,
			ForwardHeaders: append([]string(nil), p.ForwardHeaders...),
			Extends:        p.Extends, APIKeyRefPath: p.APIKeyRefPath, APIKeyRefKey: p.APIKeyRefKey,
			CopilotCredentialPath: p.CopilotCredentialPath, CopilotCredentialName: p.CopilotCredentialName,
			Enabled: p.Enabled, Source: "database",
			OrgID: p.OrgID.String(), OrgName: names[p.OrgID.String()],
			HasCredential: len(p.APIKeyEncrypted) > 0 || p.APIKeyRefKey != "" || p.CopilotCredentialName != "",
			Healthcheck:   providerHealthcheckView(p.Healthcheck),
		}
		for _, m := range models {
			mv.Models = append(mv.Models, adminModelView{Name: m.Name, DisplayName: m.DisplayName, UpstreamName: m.UpstreamName, Protocol: m.Protocol, Capabilities: append([]string(nil), m.Capabilities...), Pricing: providerModelPricingView(m.Pricing)})
		}
		v := mv
		views[p.Name] = &v
	}
	return sortedProviderViews(deps, views), nil
}

func modelPricingView(pricing *config.ModelPricing) map[string]float64 {
	if pricing == nil {
		return nil
	}
	return map[string]float64{
		"input_per_million":       pricing.InputPerMillion,
		"output_per_million":      pricing.OutputPerMillion,
		"cached_per_million":      pricing.CachedPerMillion,
		"cache_write_per_million": pricing.CacheWritePerMillion,
	}
}

func parseTimeoutUpsert(field, value string) (*int64, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", field, err)
	}
	if d <= 0 {
		return nil, fmt.Errorf("invalid %s: must be greater than zero", field)
	}
	ms := int64(d / time.Millisecond)
	return &ms, nil
}

func buildHealthcheckJSON(name string, current []byte, req *adminHealthcheckUpsert) ([]byte, error) {
	var hc dbHealthcheck
	if len(current) > 0 {
		if err := json.Unmarshal(current, &hc); err != nil {
			return nil, fmt.Errorf("provider %q: stored healthcheck is corrupt: %w", name, err)
		}
	}
	if req.Path != "" {
		hc.Path = req.Path
	}
	if req.Method != nil {
		hc.Method = *req.Method
	}
	if req.ExpectedStatus != nil {
		hc.ExpectedStatus = *req.ExpectedStatus
	}
	if req.ExpectedBody != nil {
		hc.ExpectedBody = *req.ExpectedBody
	}
	if req.Interval != nil {
		hc.Interval = *req.Interval
	}
	if req.Timeout != nil {
		hc.Timeout = *req.Timeout
	}
	if req.FailureThreshold != nil {
		hc.FailureThreshold = *req.FailureThreshold
	}
	if req.SendAuthorization != nil {
		hc.SendAuthorization = *req.SendAuthorization
	}
	if req.SuccessThreshold != nil {
		hc.SuccessThreshold = *req.SuccessThreshold
	}
	if hc.Method == "" {
		hc.Method = "GET"
	}
	if hc.ExpectedStatus == 0 {
		hc.ExpectedStatus = 200
	}
	if hc.ExpectedBody == "" {
		hc.ExpectedBody = "*"
	}
	if hc.Interval == "" {
		hc.Interval = "30s"
	}
	if hc.Timeout == "" {
		hc.Timeout = "5s"
	}
	if hc.FailureThreshold == 0 {
		hc.FailureThreshold = 2
	}
	if hc.SuccessThreshold == 0 {
		hc.SuccessThreshold = 1
	}
	interval, err := time.ParseDuration(hc.Interval)
	if err != nil || interval <= 0 {
		return nil, fmt.Errorf("provider %q: healthcheck.interval must be a positive duration", name)
	}
	timeout, err := time.ParseDuration(hc.Timeout)
	if err != nil || timeout <= 0 {
		return nil, fmt.Errorf("provider %q: healthcheck.timeout must be a positive duration", name)
	}
	out, err := json.Marshal(hc)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *Handler) validateExtends(deps Dependencies, name, extends, providerType string) error {
	if extends == "" {
		return nil
	}
	if extends == name {
		return fmt.Errorf("provider %q: extends %q references itself", name, extends)
	}
	base, ok := deps.Catalog.Provider(extends)
	if !ok {
		for _, dp := range deps.Catalog.DisabledProviders() {
			if dp.Name == extends {
				return fmt.Errorf("provider %q: extends %q references a disabled provider", name, extends)
			}
		}
		return fmt.Errorf("provider %q: extends %q is not defined", name, extends)
	}
	if string(base.Type) != providerType {
		return fmt.Errorf("provider %q: type %q must match base provider %q type %q", name, providerType, extends, base.Type)
	}
	return nil
}

func (h *Handler) buildConfigProvider(deps Dependencies, row store.DBProvider, models []store.DBProviderModel) (config.Provider, error) {
	bases := make(map[string]config.Provider)
	for _, p := range deps.Catalog.Providers() {
		bases[p.Name] = p
	}
	return dbmerge.BuildProvider(row, models, bases)
}

func (h *Handler) createDBProvider(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, org store.Organization, req adminProviderUpsert) (adminProviderView, error) {
	name := strings.TrimSpace(req.Name)
	if !config.IsLowercaseName(name) {
		return adminProviderView{}, errBad("invalid provider name: must be lowercase, no spaces, no '/', and start with [a-z0-9]")
	}
	if name == "alias" {
		return adminProviderView{}, errBad(`provider name "alias" is reserved`)
	}
	if _, err := deps.AdminStore.GetProvider(ctx, name); err == nil {
		return adminProviderView{}, errBad("provider already exists")
	}
	if h.isStaticProvider(deps, name) {
		return adminProviderView{}, errBad("provider exists in static config")
	}
	if req.Type == nil || !knownProviderType(strings.TrimSpace(*req.Type)) {
		return adminProviderView{}, errBad("unknown provider type")
	}
	providerType := strings.TrimSpace(*req.Type)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row := store.DBProvider{Name: name, Type: providerType, Enabled: enabled, OrgID: org.ID, ForwardHeaders: []string{}}
	if err := h.applyProviderUpsert(name, &row, req); err != nil {
		return adminProviderView{}, err
	}
	if err := h.validateExtends(deps, name, row.Extends, providerType); err != nil {
		return adminProviderView{}, err
	}
	models, err := h.buildDBModels(name, req.Models)
	if err != nil {
		return adminProviderView{}, err
	}
	cfgProvider, err := h.buildConfigProvider(deps, row, models)
	if err != nil {
		return adminProviderView{}, err
	}
	if err := config.ValidateDynamicProvider(cfgProvider); err != nil {
		return adminProviderView{}, err
	}
	if err := deps.AdminStore.CreateProvider(ctx, &row); err != nil {
		return adminProviderView{}, err
	}
	if err := deps.AdminStore.ReplaceProviderModels(ctx, row.ID, models); err != nil {
		return adminProviderView{}, err
	}
	return h.providerViewByName(ctx, deps, r, claims, name)
}

func (h *Handler) applyProviderUpsert(name string, row *store.DBProvider, req adminProviderUpsert) error {
	if req.DisplayName != nil {
		row.DisplayName = *req.DisplayName
	}
	if req.BaseURL != nil {
		row.BaseURL = strings.TrimSpace(*req.BaseURL)
	}
	if req.UpstreamHeaderTimeout != nil {
		if strings.TrimSpace(*req.UpstreamHeaderTimeout) == "" {
			row.UpstreamTimeoutMs = nil
		} else {
			ms, err := parseTimeoutUpsert("upstream_header_timeout", strings.TrimSpace(*req.UpstreamHeaderTimeout))
			if err != nil {
				return err
			}
			row.UpstreamTimeoutMs = ms
		}
	}
	if req.UserAgent != nil {
		row.UserAgent = *req.UserAgent
	}
	if req.ForwardUserAgent != nil {
		row.ForwardUserAgent = *req.ForwardUserAgent
	}
	if req.ForwardHeaders != nil {
		row.ForwardHeaders = append([]string(nil), req.ForwardHeaders...)
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	if req.Extends != nil {
		row.Extends = strings.TrimSpace(*req.Extends)
	}
	if req.APIKey != nil { // pragma: allowlist secret
		if *req.APIKey == "" {
			row.APIKeyEncrypted = nil // pragma: allowlist secret
		} else {
			enc, err := store.EncryptSecret([]byte(*req.APIKey))
			if err != nil {
				return err
			}
			row.APIKeyEncrypted = enc // pragma: allowlist secret
		}
	}
	if req.APIKeyRef != nil { // pragma: allowlist secret
		row.APIKeyRefPath = strings.TrimSpace(req.APIKeyRef.Path)
		row.APIKeyRefKey = strings.TrimSpace(req.APIKeyRef.Key)
	}
	if req.CredentialRef != nil {
		row.CopilotCredentialPath = strings.TrimSpace(req.CredentialRef.Path)
		row.CopilotCredentialName = strings.TrimSpace(req.CredentialRef.Name)
	}
	if req.Healthcheck != nil {
		hc, err := buildHealthcheckJSON(name, row.Healthcheck, req.Healthcheck)
		if err != nil {
			return err
		}
		row.Healthcheck = hc
	}
	return nil
}

func (h *Handler) buildDBModels(name string, reqModels *[]adminModelUpsert) ([]store.DBProviderModel, error) {
	if reqModels == nil {
		return nil, nil
	}
	models := make([]store.DBProviderModel, 0, len(*reqModels))
	for _, m := range *reqModels {
		modelName := strings.TrimSpace(m.Name)
		if modelName == "" {
			return nil, errBad("model name is required")
		}
		var pricingJSON []byte
		if m.Pricing != nil {
			if _, err := config.BuildDynamicPricing(name, modelName, m.Pricing); err != nil {
				return nil, err
			}
			raw, err := json.Marshal(m.Pricing)
			if err != nil {
				return nil, err
			}
			pricingJSON = raw
		}
		models = append(models, store.DBProviderModel{
			Name: modelName, DisplayName: m.DisplayName,
			UpstreamName: m.UpstreamName, Protocol: m.Protocol,
			Capabilities: append([]string(nil), m.Capabilities...),
			Pricing:      pricingJSON,
		})
	}
	return models, nil
}

func (h *Handler) updateDBProvider(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, req adminProviderUpsert) (adminProviderView, error) {
	name := strings.TrimSpace(req.Name)
	p, err := deps.AdminStore.GetProvider(ctx, name)
	if err != nil {
		if h.isStaticProvider(deps, name) {
			return adminProviderView{}, errBad("config-managed: edit the HCL file")
		}
		return adminProviderView{}, errBad("provider not found")
	}
	if !h.canWriteOrg(deps, ctx, claims, p.OrgID) {
		return adminProviderView{}, errBad("organization admin required")
	}
	if req.Type != nil {
		if !knownProviderType(strings.TrimSpace(*req.Type)) {
			return adminProviderView{}, errBad("unknown provider type")
		}
		p.Type = strings.TrimSpace(*req.Type)
	}
	if err := h.applyProviderUpsert(name, &p, req); err != nil {
		return adminProviderView{}, err
	}
	if err := h.validateExtends(deps, name, p.Extends, p.Type); err != nil {
		return adminProviderView{}, err
	}
	var models []store.DBProviderModel
	if req.Models != nil {
		models, err = h.buildDBModels(name, req.Models)
		if err != nil {
			return adminProviderView{}, err
		}
	} else {
		models, err = deps.AdminStore.ListProviderModels(ctx, p.ID)
		if err != nil {
			return adminProviderView{}, err
		}
	}
	cfgProvider, err := h.buildConfigProvider(deps, p, models)
	if err != nil {
		return adminProviderView{}, err
	}
	if err := config.ValidateDynamicProvider(cfgProvider); err != nil {
		return adminProviderView{}, err
	}
	if err := deps.AdminStore.UpdateProvider(ctx, &p); err != nil {
		return adminProviderView{}, err
	}
	if req.Models != nil {
		if err := deps.AdminStore.ReplaceProviderModels(ctx, p.ID, models); err != nil {
			return adminProviderView{}, err
		}
	}
	return h.providerViewByName(ctx, deps, r, claims, name)
}

func (h *Handler) providerViewByName(ctx context.Context, deps Dependencies, r *http.Request, claims *adminauth.Claims, name string) (adminProviderView, error) {
	views, err := h.mergedProviderViews(ctx, deps, r, claims, "")
	if err != nil {
		return adminProviderView{}, err
	}
	for _, v := range views {
		if v.Name == name {
			return v, nil
		}
	}
	return adminProviderView{}, errBad("provider not found after write")
}

func (h *Handler) setProviderCredential(ctx context.Context, deps Dependencies, claims *adminauth.Claims, name string, req adminCredentialUpsert) error {
	p, err := deps.AdminStore.GetProvider(ctx, name)
	if err != nil {
		if h.isStaticProvider(deps, name) {
			return errBad("config-managed: edit the HCL file")
		}
		return errBad("provider not found")
	}
	if !h.canWriteOrg(deps, ctx, claims, p.OrgID) {
		return errBad("organization admin required")
	}
	if req.APIKey != nil { // pragma: allowlist secret
		if *req.APIKey == "" {
			p.APIKeyEncrypted = nil // pragma: allowlist secret
		} else if enc, err := store.EncryptSecret([]byte(*req.APIKey)); err != nil {
			return err
		} else {
			p.APIKeyEncrypted = enc // pragma: allowlist secret
		}
	}
	if req.APIKeyRefPath != nil { // pragma: allowlist secret
		p.APIKeyRefPath = strings.TrimSpace(*req.APIKeyRefPath)
	}
	if req.APIKeyRefKey != nil { // pragma: allowlist secret
		p.APIKeyRefKey = strings.TrimSpace(*req.APIKeyRefKey)
	}
	if req.CopilotCredentialPath != nil {
		p.CopilotCredentialPath = strings.TrimSpace(*req.CopilotCredentialPath)
	}
	if req.CopilotCredentialName != nil {
		p.CopilotCredentialName = strings.TrimSpace(*req.CopilotCredentialName)
	}
	models, err := deps.AdminStore.ListProviderModels(ctx, p.ID)
	if err != nil {
		return err
	}
	cfgProvider, err := h.buildConfigProvider(deps, p, models)
	if err != nil {
		return err
	}
	if err := config.ValidateDynamicProvider(cfgProvider); err != nil {
		return err
	}
	return deps.AdminStore.UpdateProvider(ctx, &p)
}
