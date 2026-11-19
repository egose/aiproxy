package upstreamhttp

import (
	"fmt"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
	"github.com/egose/aiproxy/internal/tlsprofile"
)

func OptionsForProvider(provider config.Provider, timeout time.Duration) (Options, error) {
	if provider.DatabaseManaged {
		id, scoped := provider.Workspace.ID()
		if !scoped {
			return Options{}, fmt.Errorf("provider %q: managed upstream workspace is missing", provider.Name)
		}
		if id != store.SystemWorkspaceID {
			if provider.OutboundPolicy == nil || provider.APIKeyRef != nil || provider.CopilotCredentialRef != nil || // pragma: allowlist secret
				provider.ForwardAuthorization || provider.AllowInsecureBaseURL {
				return Options{}, fmt.Errorf("provider %q: tenant upstream policy was not prepared", provider.Name)
			}
		}
	}
	profile, err := ProfileForProvider(provider)
	if err != nil {
		return Options{}, err
	}
	return Options{Profile: profile, ResponseHeaderTimeout: timeout, OutboundPolicy: provider.OutboundPolicy}, nil
}

func ProfileForProvider(provider config.Provider) (tlsprofile.Profile, error) {
	if provider.TLSProfile.IsCustom() {
		if provider.TLSClientHello != "custom" || provider.TLSClientHelloSpecFile == "" {
			return tlsprofile.Profile{}, fmt.Errorf("provider %q: upstream custom TLS profile does not match its resolved declaration", provider.Name)
		}
		return provider.TLSProfile, nil
	}
	resolved, err := config.ResolveDynamicProviderTLSProfile(provider)
	if err != nil {
		return tlsprofile.Profile{}, err
	}
	return resolved.TLSProfile, nil
}
