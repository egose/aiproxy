package dbmerge

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type Merged struct {
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

type dbEncryptedReasoning struct {
	Passthrough      bool     `json:"passthrough"`
	OnCallerMismatch string   `json:"on_caller_mismatch"`
	MatchMessages    []string `json:"match_messages"`
}

func jsonPresent(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && !strings.EqualFold(trimmed, "null")
}

func MergeCatalog(ctx context.Context, st *store.Store, static config.Catalog) (Merged, error) {
	var out Merged
	if st == nil {
		return out, nil
	}
	rows, err := st.ListProviders(ctx)
	if err != nil {
		return out, fmt.Errorf("database providers: %w", err)
	}
	modelsByProvider := make(map[string][]store.DBProviderModel, len(rows))
	for _, row := range rows {
		models, err := st.ListProviderModels(ctx, row.ID)
		if err != nil {
			return out, fmt.Errorf("database provider %q models: %w", row.Name, err)
		}
		modelsByProvider[row.Name] = models
	}
	enabled := map[string]config.Provider{}
	disabled := map[string]config.Provider{}
	for _, p := range static.Providers() {
		enabled[p.Name] = p
	}
	for _, p := range static.DisabledProviders() {
		disabled[p.Name] = p
	}
	ordered := append([]store.DBProvider(nil), rows...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	for _, pass := range []bool{false, true} {
		for _, row := range ordered {
			if (row.Extends != "") != pass {
				continue
			}
			built, err := BuildProvider(row, modelsByProvider[row.Name], enabled)
			if err != nil {
				return out, err
			}
			if err := config.ValidateDynamicProvider(built); err != nil {
				return out, err
			}
			if built.Enabled {
				if err := resolveCredentials(&built); err != nil {
					return out, err
				}
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
	out.Models = append([]config.Model(nil), base.Models...)
	out.ModelByName = make(map[string]config.Model, len(base.ModelByName))
	for name, model := range base.ModelByName {
		out.ModelByName[name] = model
	}
	out.ForwardHeaders = append([]string(nil), base.ForwardHeaders...)
	if base.APIKeyRef != nil { // pragma: allowlist secret
		ref := *base.APIKeyRef
		out.APIKeyRef = &ref
	}
	if base.CopilotCredentialRef != nil {
		ref := *base.CopilotCredentialRef
		out.CopilotCredentialRef = &ref
	}
	if base.Healthcheck != nil {
		hc := *base.Healthcheck
		out.Healthcheck = &hc
	}
	return out
}

func BuildProvider(row store.DBProvider, models []store.DBProviderModel, bases map[string]config.Provider) (config.Provider, error) {
	name := row.Name
	if row.Extends != "" {
		if row.Extends == name {
			return config.Provider{}, fmt.Errorf("provider %q: extends %q references itself", name, row.Extends)
		}
		base, ok := bases[row.Extends]
		if !ok {
			return config.Provider{}, fmt.Errorf("provider %q: extends %q is not defined", name, row.Extends)
		}
		if string(base.Type) != row.Type {
			return config.Provider{}, fmt.Errorf("provider %q: type %q must match base provider %q type %q", name, row.Type, row.Extends, base.Type)
		}
		p := cloneProvider(base)
		p.Name = name
		if row.DisplayName != "" {
			p.DisplayName = row.DisplayName
		}
		p.Enabled = row.Enabled
		p.APIKey = ""
		p.APIKeyRef = nil // pragma: allowlist secret
		p.CopilotCredentialRef = nil
		p.CopilotToken = ""
		if err := attachCredential(&p, row); err != nil {
			return config.Provider{}, err
		}
		return p, nil
	}
	p := config.Provider{
		Type: config.ProviderType(row.Type), Name: name,
		DisplayName: row.DisplayName, BaseURL: row.BaseURL,
		UserAgent: row.UserAgent, ForwardUserAgent: row.ForwardUserAgent,
		ForwardHeaders: append([]string(nil), row.ForwardHeaders...),
		Enabled:        row.Enabled, ModelByName: make(map[string]config.Model),
	}
	if row.UpstreamTimeoutMs != nil {
		if *row.UpstreamTimeoutMs <= 0 {
			return config.Provider{}, fmt.Errorf("provider %q: invalid upstream_header_timeout: must be greater than zero", name)
		}
		p.UpstreamHeaderTimeout = time.Duration(*row.UpstreamTimeoutMs) * time.Millisecond
	}
	if err := attachCredential(&p, row); err != nil {
		return config.Provider{}, err
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
		model := config.Model{Name: m.Name, DisplayName: m.DisplayName, UpstreamName: upstream, Protocol: config.ModelProtocol(m.Protocol), Capabilities: caps, Pricing: pricing}
		p.Models = append(p.Models, model)
		p.ModelByName[m.Name] = model
	}
	return p, nil
}

func attachCredential(p *config.Provider, row store.DBProvider) error {
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
		byName[a.Name] = a
	}
	ordered := append([]store.DBAlias(nil), rows...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	for _, row := range ordered {
		targets, err := st.ListAliasTargets(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("database alias %q targets: %w", row.Name, err)
		}
		alias := config.Alias{Name: row.Name, Algorithm: config.Algorithm(row.Algorithm), RetryStatusCodes: append([]int(nil), row.RetryStatusCodes...)}
		if jsonPresent(row.SessionAffinity) {
			var affinity dbSessionAffinity
			if err := json.Unmarshal(row.SessionAffinity, &affinity); err != nil {
				return nil, fmt.Errorf("database alias %q has corrupt session_affinity: %w", row.Name, err)
			}
			alias.SessionAffinity = &config.SessionAffinity{Headers: affinity.Headers}
		}
		if jsonPresent(row.EncryptedReasoning) {
			var reasoning dbEncryptedReasoning
			if err := json.Unmarshal(row.EncryptedReasoning, &reasoning); err != nil {
				return nil, fmt.Errorf("database alias %q has corrupt encrypted_reasoning: %w", row.Name, err)
			}
			alias.EncryptedReasoning = &config.EncryptedReasoning{Passthrough: reasoning.Passthrough, OnCallerMismatch: reasoning.OnCallerMismatch, MatchMessages: reasoning.MatchMessages}
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
		out = append(out, auth.DynamicClient{
			TokenHash: k.TokenHash, Name: k.Name, Tenant: k.Tenant,
			AllowedModels: append([]string(nil), k.AllowedModels...),
			KeyID:         k.ID, OrgID: k.OrgID,
			OwnerUserID: uuidOrNil(k.OwnerUserID), OwnerTeamID: uuidOrNil(k.OwnerTeamID),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
