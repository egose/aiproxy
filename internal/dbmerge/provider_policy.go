package dbmerge

import (
	"fmt"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/store"
	"github.com/egose/aiproxy/internal/upstreampolicy"
)

func CatalogEdits(static config.Catalog, allowedOrigins ...string) store.CatalogEditOptions {
	allowedOrigins = append([]string(nil), allowedOrigins...)
	return store.CatalogEditOptions{
		Static: static,
		PrepareProvider: func(row store.DBProvider, models []store.DBProviderModel, bases map[string]config.Provider) (config.Provider, error) {
			return PrepareProvider(row, models, bases, allowedOrigins...)
		},
	}
}

func PrepareProvider(row store.DBProvider, models []store.DBProviderModel, bases map[string]config.Provider, allowedOrigins ...string) (config.Provider, error) {
	p, err := MaterializeProvider(row, models, bases)
	if err != nil {
		return config.Provider{}, err
	}
	p.OutboundPolicy = nil
	if row.WorkspaceID == store.SystemWorkspaceID {
		return p, nil
	}
	for _, field := range []struct {
		name      string
		forbidden bool
	}{
		{"api_key_ref", row.APIKeyRefPath != "" || row.APIKeyRefKey != "" || p.APIKeyRef != nil}, // pragma: allowlist secret
		{"credential_ref", row.CopilotCredentialPath != "" || row.CopilotCredentialName != "" || p.CopilotCredentialRef != nil},
		{"forward_authorization", row.ForwardAuthorization || p.ForwardAuthorization},
		{"allow_insecure_base_url", row.AllowInsecureBaseURL || p.AllowInsecureBaseURL},
	} {
		if field.forbidden {
			return config.Provider{}, fmt.Errorf("provider %q: %s is reserved for the System workspace; tenant providers must use their own database credentials and approved public HTTPS destinations", row.Name, field.name)
		}
	}
	policy, err := upstreampolicy.New(allowedOrigins)
	if err != nil {
		return config.Provider{}, fmt.Errorf("multi_tenancy: %w", err)
	}
	base := provider.EffectiveBaseURL(p.Type, p.BaseURL)
	if err := policy.Check(base); err != nil {
		return config.Provider{}, fmt.Errorf("provider %q: base_url: %w", row.Name, err)
	}
	if p.Healthcheck != nil {
		if err := policy.Check(config.ResolveHealthcheckURL(base, p.Healthcheck.Path)); err != nil {
			return config.Provider{}, fmt.Errorf("provider %q: healthcheck.path: %w", row.Name, err)
		}
	}
	p.OutboundPolicy = policy
	return p, nil
}
