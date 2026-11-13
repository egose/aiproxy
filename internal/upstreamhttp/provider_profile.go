package upstreamhttp

import (
	"fmt"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/tlsprofile"
)

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
