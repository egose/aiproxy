package modelresolver

import (
	"fmt"
	"strings"

	"github.com/egose/aiproxy/internal/alias"
	"github.com/egose/aiproxy/internal/config"
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
	Kind     Kind
	Provider config.Provider
	Model    config.Model
	Alias    config.Alias
	Selector alias.Selector
}

type Kind int

const (
	KindDirect Kind = iota
	KindAlias
)

type Resolver struct {
	providers map[string]config.Provider
	aliases   map[string]config.Alias
	selectors map[string]alias.Selector
}

func New(rt *config.Runtime) *Resolver {
	selectors := make(map[string]alias.Selector, len(rt.Aliases))
	for name, a := range rt.AliasByName {
		selectors[name] = alias.NewSelector(a)
	}
	for _, a := range rt.Aliases {
		if _, ok := selectors[a.Name]; !ok {
			selectors[a.Name] = alias.NewSelector(a)
		}
	}
	return &Resolver{
		providers: rt.ProviderByName,
		aliases:   rt.AliasByName,
		selectors: selectors,
	}
}

// Provider returns a provider config by name, or ok=false if not registered.
// Used by the dispatcher to look up alias target credentials and base URLs.
func (r *Resolver) Provider(name string) (config.Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Alias returns an alias config by name.
func (r *Resolver) Alias(name string) (config.Alias, bool) {
	a, ok := r.aliases[name]
	return a, ok
}

func (r *Resolver) Resolve(publicModel string) (ResolveResult, error) {
	if publicModel == "" {
		return ResolveResult{}, ErrUnknownModel{Model: publicModel}
	}
	if strings.HasPrefix(publicModel, "alias/") {
		name := strings.TrimPrefix(publicModel, "alias/")
		a, ok := r.aliases[name]
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
	prov, ok := r.providers[provName]
	if !ok {
		return ResolveResult{}, ErrUnknownProvider{Provider: provName}
	}
	model, ok := prov.ModelByName[modelName]
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
