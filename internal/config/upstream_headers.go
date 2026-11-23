package config

import (
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/egose/aiproxy/internal/copilotlogin"
)

const (
	maxUpstreamHeaderCount      = 32
	maxUpstreamHeaderBytes      = 16 << 10
	maxUpstreamHeaderNameBytes  = 128
	maxUpstreamHeaderValueBytes = 4096
)

type UpstreamHeaders struct {
	values map[string]string
}

func (h UpstreamHeaders) Values() map[string]string { return maps.Clone(h.values) }

func (h UpstreamHeaders) declared() bool { return h.values != nil }

func declaredUpstreamHeaders(values map[string]string) UpstreamHeaders {
	if values == nil {
		return UpstreamHeaders{}
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		out[http.CanonicalHeaderKey(name)] = value
	}
	return UpstreamHeaders{values: out}
}

func ResolveUpstreamHeaders(root UpstreamHeaders, provider Provider, model Model, alias *Alias) map[string]string {
	if provider.DatabaseManaged {
		return nil
	}
	aliasHeaders := UpstreamHeaders{}
	if alias != nil && !alias.DatabaseManaged {
		aliasHeaders = alias.UpstreamHeaders
	}
	var out map[string]string
	for _, layer := range [...]UpstreamHeaders{root, provider.UpstreamHeaders, model.UpstreamHeaders, aliasHeaders} {
		if len(layer.values) == 0 {
			continue
		}
		if out == nil {
			out = layer.Values()
		} else {
			maps.Copy(out, layer.values)
		}
	}
	return out
}

func IsReservedUpstreamHeader(providerType ProviderType, name string) bool {
	lower := strings.ToLower(name)
	if _, denied := deniedForwardHeaders[lower]; denied || lower == "accept-encoding" || IsDeferredHeader(lower) {
		return true
	}
	switch providerType {
	case ProviderTypeAnthropic:
		return lower == "anthropic-version"
	case ProviderTypeOpenRouter:
		return lower == "http-referer" || lower == "x-title"
	case ProviderTypeOpenCodeZen, ProviderTypeOpenCodeGo:
		return lower == "x-opencode-session" || lower == "x-opencode-client"
	case ProviderTypeGitHubCopilot:
		switch lower {
		case strings.ToLower(copilotlogin.APIVersionHeader), strings.ToLower(copilotlogin.IntentHeader),
			strings.ToLower(copilotlogin.InitiatorHeader), strings.ToLower(copilotlogin.VisionHeader),
			"cookie2", "anthropic-beta", "x-interaction-id", "x-interaction-type":
			return true
		}
	}
	return false
}

func validateCatalogUpstreamHeaders(root UpstreamHeaders, catalog Catalog) error {
	for _, providers := range [...][]Provider{catalog.providers, catalog.disabledProviders} {
		for _, provider := range providers {
			if provider.DatabaseManaged {
				if err := validateManagedProviderUpstreamHeaders(provider); err != nil {
					return err
				}
				continue
			}
			for _, model := range provider.Models {
				if err := validateTargetUpstreamHeaders(root, provider, model, nil); err != nil {
					return fmt.Errorf("provider %q: model %q: %w", provider.Name, model.Name, err)
				}
			}
			if len(provider.ModelPassthroughs) != 0 {
				if err := validateTargetUpstreamHeaders(root, provider, Model{}, nil); err != nil {
					return fmt.Errorf("provider %q: model_passthrough: %w", provider.Name, err)
				}
			}
		}
	}
	for _, alias := range catalog.aliases {
		if len(alias.UpstreamHeaders.values) == 0 || alias.DatabaseManaged {
			continue
		}
		for _, target := range alias.Targets {
			index, ok := catalog.providerIndex[target.Provider]
			if !ok {
				continue
			}
			provider := catalog.providers[index]
			if err := validateTargetUpstreamHeaders(root, provider, provider.ModelByName[target.Model], &alias); err != nil {
				return fmt.Errorf("alias %q: target %q: %w", alias.Name, target.Provider+"/"+target.Model, err)
			}
		}
	}
	return nil
}

func validateTargetUpstreamHeaders(root UpstreamHeaders, provider Provider, model Model, alias *Alias) error {
	resolved := ResolveUpstreamHeaders(root, provider, model, alias)
	if len(resolved) > maxUpstreamHeaderCount {
		return fmt.Errorf("effective upstream_headers must contain at most %d names", maxUpstreamHeaderCount)
	}
	byteCount := 0
	for name, value := range resolved {
		if IsReservedUpstreamHeader(provider.Type, name) {
			return fmt.Errorf("upstream_headers name %q is managed by the proxy for %s", name, provider.Type)
		}
		byteCount += len(name) + len(value)
	}
	if byteCount > maxUpstreamHeaderBytes {
		return fmt.Errorf("effective upstream_headers must contain at most %d name/value bytes", maxUpstreamHeaderBytes)
	}
	return nil
}

func validateManagedProviderUpstreamHeaders(provider Provider) error {
	if provider.UpstreamHeaders.declared() {
		return fmt.Errorf("provider %q: upstream_headers is static-config-only", provider.Name)
	}
	for _, model := range provider.Models {
		if model.UpstreamHeaders.declared() {
			return fmt.Errorf("provider %q: model %q: upstream_headers is static-config-only", provider.Name, model.Name)
		}
	}
	for name, model := range provider.ModelByName {
		if model.UpstreamHeaders.declared() {
			return fmt.Errorf("provider %q: model %q: upstream_headers is static-config-only", provider.Name, name)
		}
	}
	return nil
}
