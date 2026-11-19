package modelresolver

import (
	"fmt"
	"strings"

	"github.com/egose/aiproxy/internal/alias"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
)

// Resolve identifies a concrete provider/model pair from the public model
// string the client supplied. It returns either:
//
//   - Direct: a configured provider + model
//   - Alias: an alias name plus the alias's selector so the dispatcher can
//     rotate across its pool
//
// Unknown models return an error so the caller maps them to a 4xx response.
type ResolveResult struct {
	Kind      Kind
	Provider  config.Provider
	Model     config.Model
	Alias     config.Alias
	Selector  alias.Selector
	Workspace identity.WorkspaceScope
}

type Kind int

const (
	KindDirect Kind = iota
	KindAlias
)

type Resolver struct {
	catalog            config.Catalog
	selectors          map[string]alias.Selector
	cooldowns          *CooldownStore
	encryptedReasoning *config.EncryptedReasoning
}

func New(rt *config.Runtime) *Resolver {
	return NewWithPrevious(rt, nil)
}

func NewWithPrevious(rt *config.Runtime, previous *Resolver) *Resolver {
	aliases := rt.Catalog.Aliases()
	selectors := make(map[string]alias.Selector, len(aliases))
	for _, a := range aliases {
		selectors[a.Name] = selectorForAlias(a.Name, a, previous)
	}
	return &Resolver{
		catalog:            rt.Catalog,
		selectors:          selectors,
		cooldowns:          cooldownsForCatalog(rt.Catalog, previous),
		encryptedReasoning: config.CloneEncryptedReasoning(rt.EncryptedReasoning),
	}
}

func cooldownsForCatalog(catalog config.Catalog, previous *Resolver) *CooldownStore {
	if previous == nil || previous.cooldowns == nil {
		return NewCooldownStore()
	}
	return previous.cooldowns.cloneForCatalog(catalog)
}

func (r *Resolver) Cooldowns() *CooldownStore {
	if r == nil || r.cooldowns == nil {
		return NewCooldownStore()
	}
	return r.cooldowns
}

func selectorForAlias(name string, a config.Alias, previous *Resolver) alias.Selector {
	if previous != nil {
		if previousAlias, ok := previous.catalog.Alias(name); ok && aliasesShareSelectorState(previousAlias, a) {
			if selector := previous.selectors[name]; selector != nil {
				return selector
			}
		}
	}
	return alias.NewSelector(a)
}

func aliasesShareSelectorState(a, b config.Alias) bool {
	if a.Name != b.Name || a.Workspace != b.Workspace || a.Algorithm != b.Algorithm || len(a.Targets) != len(b.Targets) {
		return false
	}
	for i := range a.Targets {
		if a.Targets[i].Provider != b.Targets[i].Provider || a.Targets[i].Model != b.Targets[i].Model {
			return false
		}
	}
	if (a.SessionAffinity == nil) != (b.SessionAffinity == nil) {
		return false
	}
	if a.SessionAffinity != nil && b.SessionAffinity != nil {
		if len(a.SessionAffinity.Headers) != len(b.SessionAffinity.Headers) {
			return false
		}
		for i := range a.SessionAffinity.Headers {
			if a.SessionAffinity.Headers[i] != b.SessionAffinity.Headers[i] {
				return false
			}
		}
	}
	return true
}

// Provider returns a provider config by name, or ok=false if not registered.
// Used by the dispatcher to look up alias target credentials and base URLs.
func (r *Resolver) Provider(name string) (config.Provider, bool) {
	return r.catalog.Provider(name)
}

// Alias returns an alias config by name.
func (r *Resolver) Alias(name string) (config.Alias, bool) {
	return r.catalog.Alias(name)
}

func (r *Resolver) Model(providerName, modelName string) (config.Provider, config.Model, bool) {
	return r.catalog.Model(providerName, modelName)
}

func (r *Resolver) ModelVisibleInWorkspace(publicModel string, workspace identity.WorkspaceScope) bool {
	return r != nil && r.catalog.ModelVisibleInWorkspace(publicModel, workspace)
}

func (r *Resolver) ModelInWorkspace(providerName, modelName string, workspace identity.WorkspaceScope) (config.Provider, config.Model, bool) {
	if !r.ModelVisibleInWorkspace(providerName+"/"+modelName, workspace) {
		return config.Provider{}, config.Model{}, false
	}
	return r.Model(providerName, modelName)
}

func (r *Resolver) ResolveInWorkspace(publicModel string, workspace identity.WorkspaceScope) (ResolveResult, error) {
	if !r.ModelVisibleInWorkspace(publicModel, workspace) {
		return ResolveResult{}, ErrUnknownModel{Model: publicModel}
	}
	resolved, err := r.Resolve(publicModel)
	if err != nil {
		return ResolveResult{}, err
	}
	resolved.Workspace = workspace
	return resolved, nil
}

func (r *Resolver) EncryptedReasoning(provider config.Provider, model config.Model, alias *config.Alias) config.EncryptedReasoningResolution {
	return config.ResolveEncryptedReasoning(r.encryptedReasoning, provider, model, alias)
}

func (r *Resolver) Resolve(publicModel string) (ResolveResult, error) {
	if publicModel == "" {
		return ResolveResult{}, ErrUnknownModel{Model: publicModel}
	}
	if strings.HasPrefix(publicModel, "alias/") {
		name := strings.TrimPrefix(publicModel, "alias/")
		a, ok := r.catalog.Alias(name)
		if !ok {
			return ResolveResult{}, ErrUnknownAlias{Alias: name}
		}
		return ResolveResult{Kind: KindAlias, Alias: a, Selector: r.selectors[name]}, nil
	}
	parts := strings.SplitN(publicModel, "/", 2)
	if len(parts) != 2 {
		return ResolveResult{}, ErrUnknownModel{Model: publicModel}
	}
	provName, modelName := parts[0], parts[1]
	if _, ok := r.catalog.Provider(provName); !ok {
		return ResolveResult{}, ErrUnknownProvider{Provider: provName}
	}
	prov, model, ok := r.catalog.Model(provName, modelName)
	if !ok {
		return ResolveResult{}, ErrUnknownModel{Model: publicModel}
	}
	return ResolveResult{Kind: KindDirect, Provider: prov, Model: model}, nil
}

type ErrUnknownModel struct{ Model string }

func (e ErrUnknownModel) Error() string { return fmt.Sprintf("unknown model %q", e.Model) }

type ErrUnknownProvider struct{ Provider string }

func (e ErrUnknownProvider) Error() string { return fmt.Sprintf("unknown provider %q", e.Provider) }

type ErrUnknownAlias struct{ Alias string }

func (e ErrUnknownAlias) Error() string { return fmt.Sprintf("unknown alias %q", e.Alias) }
