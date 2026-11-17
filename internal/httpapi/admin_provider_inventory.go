package httpapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/store"
)

type adminProviderInventory struct {
	store        *store.Store
	rows         []store.DBProvider
	rowsByName   map[string]store.DBProvider
	static       map[string]config.Provider
	materialized map[string]config.Provider
	models       map[string][]store.DBProviderModel
}

func loadAdminProviderInventory(ctx context.Context, deps Dependencies) (*adminProviderInventory, error) {
	rows, err := deps.AdminStore.ListProviders(ctx)
	if err != nil {
		return nil, catalogStorageError{err}
	}
	inventory := &adminProviderInventory{
		store: deps.AdminStore, rows: rows, rowsByName: make(map[string]store.DBProvider, len(rows)),
		static: make(map[string]config.Provider), materialized: make(map[string]config.Provider),
		models: make(map[string][]store.DBProviderModel),
	}
	for _, provider := range deps.Catalog.DisabledProviders() {
		if !provider.DatabaseManaged {
			inventory.static[provider.Name] = provider
		}
	}
	for _, provider := range deps.Catalog.Providers() {
		if !provider.DatabaseManaged {
			inventory.static[provider.Name] = provider
		}
	}
	for _, row := range rows {
		inventory.rowsByName[row.Name] = row
		delete(inventory.static, row.Name)
	}
	return inventory, nil
}

func (inventory *adminProviderInventory) providerModels(ctx context.Context, row store.DBProvider) ([]store.DBProviderModel, error) {
	if models, ok := inventory.models[row.Name]; ok {
		return models, nil
	}
	models, err := inventory.store.ListProviderModels(ctx, row.ID)
	if err != nil {
		return nil, catalogStorageError{err}
	}
	inventory.models[row.Name] = models
	return models, nil
}

func (inventory *adminProviderInventory) bases(ctx context.Context, row store.DBProvider) (map[string]config.Provider, error) {
	bases := make(map[string]config.Provider, 1)
	if row.Extends == "" {
		return bases, nil
	}
	if row.Extends == row.Name {
		return nil, fmt.Errorf("provider %q: extends %q references itself", row.Name, row.Extends)
	}
	if base, ok := inventory.rowsByName[row.Extends]; ok {
		if base.Extends != "" {
			return nil, fmt.Errorf("provider %q: extends %q must reference a concrete provider; inheritance chains are not supported", row.Name, row.Extends)
		}
		if !base.Enabled {
			return nil, fmt.Errorf("provider %q: extends %q references a disabled provider", row.Name, row.Extends)
		}
	}
	base, err := inventory.provider(ctx, row.Extends)
	if err != nil {
		return nil, err
	}
	bases[row.Extends] = base
	return bases, nil
}

func (inventory *adminProviderInventory) provider(ctx context.Context, name string) (config.Provider, error) {
	if provider, ok := inventory.materialized[name]; ok {
		return provider, nil
	}
	row, ok := inventory.rowsByName[name]
	if !ok {
		if provider, ok := inventory.static[name]; ok {
			return provider, nil
		}
		return config.Provider{}, fmt.Errorf("provider %q is not defined", name)
	}
	bases, err := inventory.bases(ctx, row)
	if err != nil {
		return config.Provider{}, err
	}
	models, err := inventory.providerModels(ctx, row)
	if err != nil {
		return config.Provider{}, err
	}
	provider, err := dbmerge.MaterializeProvider(row, models, bases)
	if err != nil {
		return config.Provider{}, err
	}
	inventory.materialized[name] = provider
	return provider, nil
}

func (h *Handler) prepareDBProvider(ctx context.Context, deps Dependencies, current *store.DBProvider, row store.DBProvider, req adminProviderUpsert) (config.Provider, []store.DBProviderModel, error) {
	inventory, err := loadAdminProviderInventory(ctx, deps)
	if err != nil {
		return config.Provider{}, nil, err
	}
	bases, err := inventory.bases(ctx, row)
	if err != nil {
		return config.Provider{}, nil, err
	}
	if row.Extends != "" {
		if _, err := dbmerge.MaterializeProvider(row, nil, bases); err != nil {
			return config.Provider{}, nil, err
		}
	}
	var models []store.DBProviderModel
	if current != nil && (req.Models == nil || (current.Extends == "") != (row.Extends == "")) {
		models, err = inventory.providerModels(ctx, *current)
		if err != nil {
			return config.Provider{}, nil, err
		}
		if err := validateProviderModelTransition(*current, row, models, req.Models); err != nil {
			return config.Provider{}, nil, err
		}
	}
	if req.Models != nil {
		models, err = h.buildDBProviderModels(row, req.Models, bases)
		if err != nil {
			return config.Provider{}, nil, err
		}
	}
	provider, err := dbmerge.BuildProvider(row, models, bases)
	if err != nil {
		return config.Provider{}, nil, err
	}
	if err := config.ValidateDynamicProvider(provider); err != nil {
		return config.Provider{}, nil, err
	}
	return provider, models, nil
}

func (h *Handler) buildDBProviderModels(row store.DBProvider, reqModels *[]adminModelUpsert, bases map[string]config.Provider) ([]store.DBProviderModel, error) {
	if row.Extends == "" || reqModels == nil {
		return h.buildDBModels(row.Name, reqModels)
	}
	base := bases[row.Extends]
	known := make(map[string]bool, len(base.Models)+len(base.ModelByName))
	for _, model := range base.Models {
		known[model.Name] = true
	}
	for name := range base.ModelByName {
		known[name] = true
	}
	seen := make(map[string]bool, len(*reqModels))
	models := make([]store.DBProviderModel, 0, len(*reqModels))
	for _, model := range *reqModels {
		if err := validateManagedDeferredDeclarations(row.Name, model.unsupportedDeferredDeclaration); err != nil {
			return nil, err
		}
		for _, field := range model.declaredFields {
			if field != "name" && field != "encrypted_reasoning" {
				return nil, fmt.Errorf("provider %q: derived model overlay cannot declare %q", row.Name, field)
			}
		}
		if model.inputJSONError != nil {
			return nil, model.inputJSONError
		}
		if model.DisplayName != "" || model.UpstreamName != "" || model.Protocol != "" || model.Capabilities != nil || model.Pricing != nil {
			return nil, fmt.Errorf("provider %q: derived model overlay cannot redefine inherited model fields", row.Name)
		}
		name := strings.TrimSpace(model.Name)
		if seen[name] {
			return nil, fmt.Errorf("provider %q: duplicate model overlay %q", row.Name, name)
		}
		seen[name] = true
		if !known[name] {
			return nil, fmt.Errorf("provider %q: model %q overlay requires an existing inherited model", row.Name, name)
		}
		policy, err := encodeAdminEncryptedReasoning(model.EncryptedReasoning, fmt.Sprintf("provider %q: model %q", row.Name, name))
		if err != nil {
			return nil, err
		}
		if policy != nil {
			models = append(models, store.DBProviderModel{Name: name, EncryptedReasoning: policy})
		}
	}
	return models, nil
}
