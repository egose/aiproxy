package dbmerge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type Merged struct {
	Revisions         store.ServingRevisions
	ConfirmedAt       time.Time
	Providers         []config.Provider
	DisabledProviders []config.Provider
	Aliases           []config.Alias
	Keys              []auth.DynamicClient
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

type dbSessionAffinity struct {
	Headers []string `json:"headers"`
}

func jsonPresent(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && !strings.EqualFold(trimmed, "null")
}

func MergeCatalog(ctx context.Context, st *store.Store, rt *config.Runtime) (Merged, error) {
	if st == nil {
		return Merged{}, nil
	}
	started := time.Now()
	before, err := st.ServingRevisions(ctx)
	if err != nil {
		return Merged{}, err
	}
	out, err := mergeCatalog(ctx, st, rt)
	if err != nil {
		return Merged{}, err
	}
	after, err := st.ServingRevisions(ctx)
	if err != nil {
		return Merged{}, err
	}
	if before != after {
		return Merged{}, ErrSnapshotChanged
	}
	out.Revisions, out.ConfirmedAt = before, started
	return out, nil
}

var ErrSnapshotChanged = errors.New("database serving snapshot changed during read")

func MergeKeys(ctx context.Context, st *store.Store) ([]auth.DynamicClient, store.ServingRevisions, time.Time, error) {
	started := time.Now()
	before, err := st.ServingRevisions(ctx)
	if err != nil {
		return nil, before, started, err
	}
	keys, err := mergeKeys(ctx, st)
	if err != nil {
		return nil, before, started, err
	}
	after, err := st.ServingRevisions(ctx)
	if err == nil && before.Keys != after.Keys {
		err = ErrSnapshotChanged
	}
	if before.Catalog != after.Catalog {
		before.Catalog = 0
	}
	return keys, before, started, err
}

func mergeCatalog(ctx context.Context, st *store.Store, rt *config.Runtime) (Merged, error) {
	var out Merged
	if st == nil {
		return out, nil
	}
	static := rt.Catalog
	staticNames := make(map[string]bool)
	for _, p := range append(static.Providers(), static.DisabledProviders()...) {
		if !p.DatabaseManaged {
			staticNames[p.Name] = true
		}
	}
	rows, err := st.ListProviders(ctx)
	if err != nil {
		return out, fmt.Errorf("database providers: %w", err)
	}
	modelsByProvider := make(map[string][]store.DBProviderModel, len(rows))
	managedByName := make(map[string]store.DBProvider, len(rows))
	for _, row := range rows {
		if staticNames[row.Name] {
			return out, fmt.Errorf("provider %q: name is already defined in static config", row.Name)
		}
		managedByName[row.Name] = row
		models, err := st.ListProviderModels(ctx, row.ID)
		if err != nil {
			return out, fmt.Errorf("database provider %q models: %w", row.Name, err)
		}
		modelsByProvider[row.Name] = models
	}
	enabled := map[string]config.Provider{}
	disabled := map[string]config.Provider{}
	system := identity.InWorkspace(store.SystemWorkspaceID)
	for _, p := range static.Providers() {
		if p.DatabaseManaged {
			continue
		}
		p.Workspace = system
		enabled[p.Name] = p
	}
	for _, p := range static.DisabledProviders() {
		if p.DatabaseManaged {
			continue
		}
		p.Workspace = system
		disabled[p.Name] = p
	}
	ordered := append([]store.DBProvider(nil), rows...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	for _, pass := range []bool{false, true} {
		for _, row := range ordered {
			if (row.Extends != "") != pass {
				continue
			}
			if base, ok := managedByName[row.Extends]; row.Extends != "" && ok {
				if base.WorkspaceID != row.WorkspaceID {
					return out, unavailableProviderBase(row)
				}
				if base.Extends != "" {
					return out, fmt.Errorf("provider %q: extends %q must reference a concrete provider; inheritance chains are not supported", row.Name, row.Extends)
				}
				if !base.Enabled {
					return out, fmt.Errorf("provider %q: extends %q references a disabled provider", row.Name, row.Extends)
				}
			}
			built, err := PrepareProvider(row, modelsByProvider[row.Name], enabled, rt.MultiTenancy.AllowedUpstreamOrigins...)
			if err != nil {
				return out, err
			}
			if row.Extends == "" {
				built = rt.ApplyProviderDefaults(built)
			}
			if built.Enabled {
				delete(disabled, built.Name)
				enabled[built.Name] = built
			} else {
				delete(enabled, built.Name)
				disabled[built.Name] = built
			}
		}
	}
	aliases, err := mergeAliases(ctx, st, static, enabled, disabled)
	if err != nil {
		return out, err
	}
	for _, row := range ordered {
		built, ok := enabled[row.Name]
		if !ok {
			built = disabled[row.Name]
		}
		if err := attachCredential(&built, row); err != nil {
			return out, err
		}
		if err := config.ValidateDynamicProvider(built); err != nil {
			return out, err
		}
		if built.Enabled {
			if err := resolveCredentials(&built); err != nil {
				return out, err
			}
			enabled[built.Name] = built
		} else {
			disabled[built.Name] = built
		}
	}
	keys, err := mergeKeys(ctx, st)
	if err != nil {
		return out, err
	}
	out.Providers = sortedProviders(enabled)
	out.DisabledProviders = sortedProviders(disabled)
	out.Aliases = aliases
	out.Keys = keys
	return out, nil
}

func sortedProviders(byName map[string]config.Provider) []config.Provider {
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]config.Provider, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out
}

func cloneProvider(base config.Provider) config.Provider {
	out := base
	out.APIKey = ""
	out.APIKeyRef = nil // pragma: allowlist secret
	out.CopilotCredentialRef = nil
	out.CopilotToken = ""
	out.UpstreamHeaders = config.UpstreamHeaders{}
	out.EncryptedReasoning = config.CloneEncryptedReasoning(base.EncryptedReasoning)
	out.Models = make([]config.Model, 0, len(base.Models)+len(base.ModelByName))
	out.ModelByName = make(map[string]config.Model, len(base.Models)+len(base.ModelByName))
	for _, model := range base.Models {
		indexed := base.ModelByName[model.Name]
		if model.EncryptedReasoning == nil {
			model.EncryptedReasoning = indexed.EncryptedReasoning
			model.EncryptedReasoningInherited = indexed.EncryptedReasoningInherited
		}
		if model.DeferredTransport == nil {
			model.DeferredTransport = indexed.DeferredTransport
		}
		out.Models = append(out.Models, cloneModel(model))
		out.ModelByName[model.Name] = cloneModel(model)
	}
	var missing []string
	for name := range base.ModelByName {
		if _, ok := out.ModelByName[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		model := base.ModelByName[name]
		out.Models = append(out.Models, cloneModel(model))
		out.ModelByName[name] = cloneModel(model)
	}
	out.ForwardHeaders = append([]string(nil), base.ForwardHeaders...)
	if base.Healthcheck != nil {
		hc := *base.Healthcheck
		out.Healthcheck = &hc
	}
	return out
}

func cloneModel(model config.Model) config.Model {
	model.UpstreamHeaders = config.UpstreamHeaders{}
	model.Capabilities = append([]config.Capability(nil), model.Capabilities...)
	model.EncryptedReasoning = config.CloneEncryptedReasoning(model.EncryptedReasoning)
	if model.Pricing != nil {
		pricing := *model.Pricing
		model.Pricing = &pricing
	}
	if model.DeferredTransport != nil {
		deferred := *model.DeferredTransport
		model.DeferredTransport = &deferred
	}
	return model
}

func BuildProvider(row store.DBProvider, models []store.DBProviderModel, bases map[string]config.Provider, allowedOrigins ...string) (config.Provider, error) {
	p, err := PrepareProvider(row, models, bases, allowedOrigins...)
	if err != nil {
		return config.Provider{}, err
	}
	if err := attachCredential(&p, row); err != nil {
		return config.Provider{}, err
	}
	return p, nil
}

func MaterializeProvider(row store.DBProvider, models []store.DBProviderModel, bases map[string]config.Provider) (config.Provider, error) {
	name := row.Name
	reasoning, err := decodeStoredEncryptedReasoning(row.EncryptedReasoning, fmt.Sprintf("provider %q", name))
	if err != nil {
		return config.Provider{}, err
	}
	if row.Extends != "" && row.TLSClientHello != "" {
		return config.Provider{}, fmt.Errorf("provider %q: derived provider cannot declare tls_client_hello", name)
	}
	localTLS, err := config.ResolveDynamicProviderTLSProfile(config.Provider{
		Name: name, TLSClientHello: row.TLSClientHello, HasTLSClientHello: row.TLSClientHello != "",
	})
	if err != nil {
		return config.Provider{}, err
	}
	if row.Extends != "" {
		if row.AllowInsecureBaseURL {
			return config.Provider{}, fmt.Errorf("provider %q: derived provider cannot declare allow_insecure_base_url", name)
		}
		if row.Extends == name {
			return config.Provider{}, fmt.Errorf("provider %q: extends %q references itself", name, row.Extends)
		}
		base, ok := bases[row.Extends]
		if !ok {
			return config.Provider{}, unavailableProviderBase(row)
		}
		if _, scoped := base.Workspace.ID(); !scoped && !base.DatabaseManaged {
			base.Workspace = identity.InWorkspace(store.SystemWorkspaceID)
		}
		if base.Workspace != identity.InWorkspace(row.WorkspaceID) {
			return config.Provider{}, unavailableProviderBase(row)
		}
		if base.Extends != "" {
			return config.Provider{}, fmt.Errorf("provider %q: extends %q must reference a concrete provider; inheritance chains are not supported", name, row.Extends)
		}
		if !base.Enabled {
			return config.Provider{}, fmt.Errorf("provider %q: extends %q references a disabled provider", name, row.Extends)
		}
		if string(base.Type) != row.Type {
			return config.Provider{}, fmt.Errorf("provider %q: type %q must match base provider %q type %q", name, row.Type, row.Extends, base.Type)
		}
		if len(base.ModelPassthroughs) != 0 && len(base.Models) == 0 && len(base.ModelByName) == 0 {
			return config.Provider{}, fmt.Errorf("provider %q: extends %q requires a base with at least one literal model", name, row.Extends)
		}
		deferredBase := base
		deferredBase.Name = name
		if err := config.ValidateManagedProviderDeferred(deferredBase); err != nil {
			return config.Provider{}, err
		}
		baseTLS := base
		baseTLS.Name = name
		baseTLS, err = config.ResolveDynamicProviderTLSProfile(baseTLS)
		if err != nil {
			return config.Provider{}, err
		}
		base.TLSProfile = baseTLS.TLSProfile
		p := cloneProvider(base)
		p.ModelPassthroughs = nil
		p.Name = name
		p.DatabaseManaged = true
		p.Workspace = identity.InWorkspace(row.WorkspaceID)
		p.Extends = row.Extends
		p.TLSClientHelloInherited = true
		p.ForwardAuthorization = row.ForwardAuthorization
		if row.DisplayName != "" {
			p.DisplayName = row.DisplayName
		}
		p.Enabled = row.Enabled
		p.EncryptedReasoning = config.NormalizeEncryptedReasoning(p.EncryptedReasoning)
		p.EncryptedReasoningInherited = p.EncryptedReasoning != nil
		if reasoning != nil {
			p.EncryptedReasoning = reasoning
			p.EncryptedReasoningInherited = false
		}
		for i := range p.Models {
			p.Models[i].EncryptedReasoning = config.NormalizeEncryptedReasoning(p.Models[i].EncryptedReasoning)
			p.Models[i].EncryptedReasoningInherited = p.Models[i].EncryptedReasoning != nil
		}
		if err := applyStoredModelOverlays(&p, models); err != nil {
			return config.Provider{}, err
		}
		if err := validateMaterializedEncryptedReasoning(p); err != nil {
			return config.Provider{}, err
		}
		return p, nil
	}
	p := config.Provider{
		Type: config.ProviderType(row.Type), Name: name, DatabaseManaged: true,
		Workspace:   identity.InWorkspace(row.WorkspaceID),
		DisplayName: row.DisplayName, BaseURL: row.BaseURL,
		TLSClientHello: row.TLSClientHello, HasTLSClientHello: row.TLSClientHello != "", TLSProfile: localTLS.TLSProfile,
		UserAgent: row.UserAgent, ForwardUserAgent: row.ForwardUserAgent,
		ForwardHeaders:       append([]string(nil), row.ForwardHeaders...),
		ForwardAuthorization: row.ForwardAuthorization,
		AllowInsecureBaseURL: row.AllowInsecureBaseURL,
		Enabled:              row.Enabled, ModelByName: make(map[string]config.Model),
		EncryptedReasoning: reasoning,
	}
	if row.UpstreamTimeoutMs != nil {
		if *row.UpstreamTimeoutMs <= 0 {
			return config.Provider{}, fmt.Errorf("provider %q: invalid upstream_header_timeout: must be greater than zero", name)
		}
		p.UpstreamHeaderTimeout = time.Duration(*row.UpstreamTimeoutMs) * time.Millisecond
	}
	if len(row.Healthcheck) > 0 && jsonPresent(row.Healthcheck) {
		var hc dbHealthcheck
		if err := json.Unmarshal(row.Healthcheck, &hc); err != nil {
			return config.Provider{}, fmt.Errorf("provider %q: stored healthcheck is corrupt: %w", name, err)
		}
		interval, err := time.ParseDuration(hc.Interval)
		if err != nil {
			interval = 0
		}
		timeout, err := time.ParseDuration(hc.Timeout)
		if err != nil {
			timeout = 0
		}
		p.Healthcheck = &config.ProviderHealthcheck{
			Path: hc.Path, Method: hc.Method, ExpectedStatus: hc.ExpectedStatus,
			ExpectedBody: hc.ExpectedBody, Interval: interval, Timeout: timeout,
			FailureThreshold: hc.FailureThreshold, SuccessThreshold: hc.SuccessThreshold,
			SendAuthorization: hc.SendAuthorization,
		}
	}
	for _, m := range models {
		if _, ok := p.ModelByName[m.Name]; ok {
			return config.Provider{}, fmt.Errorf("provider %q: duplicate model %q", name, m.Name)
		}
		reasoning, err := decodeStoredEncryptedReasoning(m.EncryptedReasoning, fmt.Sprintf("provider %q: model %q", name, m.Name))
		if err != nil {
			return config.Provider{}, err
		}
		upstream := m.UpstreamName
		if upstream == "" {
			upstream = m.Name
		}
		caps := make([]config.Capability, 0, len(m.Capabilities))
		for _, c := range m.Capabilities {
			caps = append(caps, config.Capability(c))
		}
		var pricing *config.ModelPricing
		if jsonPresent(m.Pricing) {
			var rates map[string]float64
			if err := json.Unmarshal(m.Pricing, &rates); err != nil {
				return config.Provider{}, fmt.Errorf("provider %q: model %q has corrupt pricing: %w", name, m.Name, err)
			}
			if len(rates) > 0 {
				var err error
				pricing, err = config.BuildDynamicPricing(name, m.Name, rates)
				if err != nil {
					return config.Provider{}, err
				}
			}
		}
		model := config.Model{Name: m.Name, DisplayName: m.DisplayName, UpstreamName: upstream, Protocol: config.ModelProtocol(m.Protocol), Capabilities: caps, Pricing: pricing, EncryptedReasoning: reasoning}
		p.Models = append(p.Models, cloneModel(model))
		p.ModelByName[m.Name] = cloneModel(model)
	}
	if err := validateMaterializedEncryptedReasoning(p); err != nil {
		return config.Provider{}, err
	}
	return p, nil
}

func unavailableProviderBase(row store.DBProvider) error {
	return fmt.Errorf("provider %q: inheritance base is not defined in the destination workspace", row.Name)
}

func attachCredential(p *config.Provider, row store.DBProvider) error {
	hasAPI := len(row.APIKeyEncrypted) > 0 || row.APIKeyRefKey != "" || row.APIKeyRefPath != ""
	hasCopilotRef := row.CopilotCredentialName != "" || row.CopilotCredentialPath != ""
	hasCopilotDB := row.CopilotCredentialEncrypted != nil
	if p.ForwardAuthorization && (hasAPI || hasCopilotRef || hasCopilotDB) {
		return fmt.Errorf("provider %q: forward_authorization cannot be combined with stored credentials", p.Name)
	}
	if p.Type == config.ProviderTypeGitHubCopilot && hasAPI {
		return fmt.Errorf("provider %q: api_key and api_key_ref are not supported by github-copilot; use credential_ref or a database credential", p.Name)
	}
	if p.Type != config.ProviderTypeGitHubCopilot && (hasCopilotRef || hasCopilotDB) {
		return fmt.Errorf("provider %q: Copilot credentials are only supported by github-copilot", p.Name)
	}
	if hasCopilotDB && hasCopilotRef {
		return fmt.Errorf("provider %q: only one Copilot credential source may be set", p.Name)
	}
	if row.CopilotCredentialPath != "" && row.CopilotCredentialName == "" {
		return fmt.Errorf("provider %q: credential_ref name is required", p.Name)
	}
	if hasCopilotDB {
		cred, err := store.DecryptCopilotCredential(row.CopilotCredentialEncrypted, time.Now())
		if err != nil {
			return fmt.Errorf("provider %q: %w", p.Name, err)
		}
		p.CopilotToken = cred.AccessToken
	}
	if len(row.APIKeyEncrypted) > 0 {
		plain, err := store.DecryptSecret(row.APIKeyEncrypted)
		if err != nil {
			return fmt.Errorf("provider %q: cannot decrypt api_key: %w", p.Name, err)
		}
		p.APIKey = string(plain)
	}
	if row.APIKeyRefKey != "" {
		p.APIKeyRef = &config.APIKeyRef{Path: row.APIKeyRefPath, Key: row.APIKeyRefKey}
	}
	if row.CopilotCredentialName != "" {
		p.CopilotCredentialRef = &config.CopilotCredentialRef{Path: row.CopilotCredentialPath, Name: row.CopilotCredentialName}
	}
	return nil
}

func resolveCredentials(p *config.Provider) error {
	if err := config.ResolveProviderCredential(p); err != nil {
		return err
	}
	if p.Type == config.ProviderTypeGitHubCopilot {
		if err := config.ResolveCopilotCredential(p); err != nil {
			return err
		}
	}
	return nil
}

func mergeAliases(ctx context.Context, st *store.Store, static config.Catalog, enabled, disabled map[string]config.Provider) ([]config.Alias, error) {
	rows, err := st.ListAliases(ctx)
	if err != nil {
		return nil, fmt.Errorf("database aliases: %w", err)
	}
	byName := map[string]config.Alias{}
	for _, a := range static.Aliases() {
		if a.DatabaseManaged {
			continue
		}
		a.Workspace = identity.InWorkspace(store.SystemWorkspaceID)
		byName[a.Name] = a
	}
	ordered := append([]store.DBAlias(nil), rows...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	for _, row := range ordered {
		if _, exists := byName[row.Name]; exists {
			return nil, fmt.Errorf("alias %q: name is already defined in static config", row.Name)
		}
		targets, err := st.ListAliasTargets(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("database alias %q targets: %w", row.Name, err)
		}
		alias := config.Alias{
			Name: row.Name, Workspace: identity.InWorkspace(row.WorkspaceID), DatabaseManaged: true,
			Algorithm: config.Algorithm(row.Algorithm), RetryStatusCodes: append([]int(nil), row.RetryStatusCodes...),
		}
		if jsonPresent(row.SessionAffinity) {
			var affinity dbSessionAffinity
			if err := json.Unmarshal(row.SessionAffinity, &affinity); err != nil {
				return nil, fmt.Errorf("database alias %q has corrupt session_affinity: %w", row.Name, err)
			}
			alias.SessionAffinity = &config.SessionAffinity{Headers: affinity.Headers}
		}
		alias.EncryptedReasoning, err = decodeStoredEncryptedReasoning(row.EncryptedReasoning, fmt.Sprintf("alias %q", row.Name))
		if err != nil {
			return nil, err
		}
		for _, t := range targets {
			alias.Targets = append(alias.Targets, config.AliasTarget{Provider: t.Provider, Model: t.Model})
		}
		alias = config.NormalizeAlias(alias)
		byName[row.Name] = alias
	}
	enabledList := make([]config.Provider, 0, len(enabled))
	for _, p := range enabled {
		enabledList = append(enabledList, p)
	}
	disabledList := make([]config.Provider, 0, len(disabled))
	for _, p := range disabled {
		disabledList = append(disabledList, p)
	}
	aliasList := make([]config.Alias, 0, len(byName))
	for _, a := range byName {
		aliasList = append(aliasList, a)
	}
	catalog := config.NewCatalog(enabledList, disabledList, aliasList)
	for _, a := range aliasList {
		if err := config.ValidateDynamicAlias(a, catalog); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]config.Alias, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out, nil
}

func uuidOrNil(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}

func mergeKeys(ctx context.Context, st *store.Store) ([]auth.DynamicClient, error) {
	rows, err := st.ListInboundKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("database inbound keys: %w", err)
	}
	now := time.Now()
	out := make([]auth.DynamicClient, 0, len(rows))
	for _, k := range rows {
		if !k.Enabled {
			continue
		}
		if k.ExpiresAt != nil && !k.ExpiresAt.After(now) {
			continue
		}
		var expiresAt time.Time
		if k.ExpiresAt != nil {
			expiresAt = *k.ExpiresAt
		}
		out = append(out, auth.DynamicClient{
			TokenHash: k.TokenHash, Name: k.Name, Tenant: k.Tenant,
			ExpiresAt:     expiresAt,
			AllowedModels: append([]string(nil), k.AllowedModels...),
			KeyID:         k.ID, Workspace: identity.InWorkspace(k.WorkspaceID),
			OwnerUserID: uuidOrNil(k.OwnerUserID), OwnerTeamID: uuidOrNil(k.OwnerTeamID),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
