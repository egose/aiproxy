package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var ErrCatalogDependency = errors.New("catalog dependency conflict; retarget or remove dependent aliases and providers first")

type CatalogEditOptions struct {
	Static          config.Catalog
	PrepareProvider func(DBProvider, []DBProviderModel, map[string]config.Provider) (config.Provider, error)
}

type ProviderReference struct {
	bun.BaseModel `bun:"table:db_providers,alias:p"`
	ID            uuid.UUID
	Name          string
	Type          string
	Extends       string
	Enabled       bool
	WorkspaceID   uuid.UUID
}

type AliasReference struct {
	bun.BaseModel `bun:"table:db_aliases,alias:a"`
	ID            uuid.UUID
	Name          string
	WorkspaceID   uuid.UUID
}

func (s *Store) ListProviderReferences(ctx context.Context) ([]ProviderReference, error) {
	var rows []ProviderReference
	err := s.DB.NewSelect().Model(&rows).Order("name").Scan(ctx)
	return rows, err
}

func (s *Store) GetProviderReference(ctx context.Context, name string) (ProviderReference, error) {
	var row ProviderReference
	err := s.DB.NewSelect().Model(&row).Where("name = ?", name).Scan(ctx)
	return row, err
}

func (s *Store) GetAliasReference(ctx context.Context, name string) (AliasReference, error) {
	var row AliasReference
	err := s.DB.NewSelect().Model(&row).Where("name = ?", name).Scan(ctx)
	return row, err
}

type catalogGraph struct {
	tx        bun.Tx
	options   CatalogEditOptions
	rows      map[string]DBProvider
	static    map[string]config.Provider
	providers map[string]config.Provider
}

func newCatalogGraph(ctx context.Context, tx bun.Tx, options []CatalogEditOptions) (*catalogGraph, error) {
	g := &catalogGraph{
		tx: tx, rows: make(map[string]DBProvider),
		static: make(map[string]config.Provider), providers: make(map[string]config.Provider),
	}
	if len(options) != 0 {
		g.options = options[0]
	}
	var rows []DBProvider
	if err := tx.NewSelect().Model(&rows).Column("id", "name", "type", "extends", "enabled", "workspace_id").Scan(ctx); err != nil {
		return nil, err
	}
	for _, row := range rows {
		g.rows[row.Name] = row
	}
	for _, group := range []struct {
		providers []config.Provider
		enabled   bool
	}{
		{g.options.Static.Providers(), true},
		{g.options.Static.DisabledProviders(), false},
	} {
		for _, p := range group.providers {
			if p.DatabaseManaged {
				continue
			}
			p.Enabled = group.enabled
			p.Workspace = identity.InWorkspace(SystemWorkspaceID)
			g.static[p.Name] = p
		}
	}
	return g, nil
}

func dependencyError(err error) error {
	return fmt.Errorf("%w: %v", ErrCatalogDependency, err)
}

func (g *catalogGraph) provider(ctx context.Context, name string, workspaceID uuid.UUID) (config.Provider, error) {
	row, managed := g.rows[name]
	if managed && row.WorkspaceID != workspaceID {
		return config.Provider{}, ErrCatalogDependency
	}
	if p, ok := g.static[name]; ok {
		if managed || workspaceID != SystemWorkspaceID {
			return config.Provider{}, ErrCatalogDependency
		}
		g.providers[name] = p
		return p, nil
	}
	if !managed {
		return config.Provider{}, ErrCatalogDependency
	}
	if p, ok := g.providers[name]; ok {
		return p, nil
	}
	bases := make(map[string]config.Provider)
	if row.Extends != "" {
		baseRow, managedBase := g.rows[row.Extends]
		base, staticBase := g.static[row.Extends]
		if row.Extends == row.Name ||
			(managedBase && (baseRow.WorkspaceID != row.WorkspaceID || baseRow.Extends != "" || !baseRow.Enabled || baseRow.Type != row.Type)) ||
			(!managedBase && (!staticBase || row.WorkspaceID != SystemWorkspaceID || base.Extends != "" || !base.Enabled || string(base.Type) != row.Type)) {
			return config.Provider{}, ErrCatalogDependency
		}
		var err error
		base, err = g.provider(ctx, row.Extends, row.WorkspaceID)
		if err != nil {
			return config.Provider{}, err
		}
		base.Name = row.Name
		if err := config.ValidateManagedProviderDeferred(base); err != nil {
			return config.Provider{}, dependencyError(err)
		}
		if _, err := config.ResolveDynamicProviderTLSProfile(base); err != nil {
			return config.Provider{}, dependencyError(err)
		}
		base.Name = row.Extends
		bases[row.Extends] = base
	}
	var models []DBProviderModel
	if err := g.tx.NewSelect().Model(&models).Where("provider_id = ?", row.ID).Order("name").Scan(ctx); err != nil {
		return config.Provider{}, err
	}
	p, err := dependencyProvider(row, models, bases)
	if err != nil {
		return config.Provider{}, err
	}
	if g.options.PrepareProvider != nil {
		if err := g.tx.NewSelect().Model(&row).Where("id = ?", row.ID).Scan(ctx); err != nil {
			return config.Provider{}, err
		}
		p, err = g.options.PrepareProvider(row, models, bases)
		if err != nil {
			return config.Provider{}, dependencyError(err)
		}
	}
	g.providers[name] = p
	return p, nil
}

func dependencyProvider(row DBProvider, models []DBProviderModel, bases map[string]config.Provider) (config.Provider, error) {
	p := config.Provider{
		Name: row.Name, Type: config.ProviderType(row.Type), Enabled: row.Enabled,
		Workspace: identity.InWorkspace(row.WorkspaceID), DatabaseManaged: true,
		Extends: row.Extends, ModelByName: make(map[string]config.Model),
	}
	if row.Extends != "" {
		base := bases[row.Extends]
		p.Models, p.ModelByName = base.Models, base.ModelByName
		if len(p.Models) == 0 {
			return config.Provider{}, ErrCatalogDependency
		}
		for _, model := range models {
			policy := strings.TrimSpace(string(model.EncryptedReasoning))
			if policy == "" || strings.EqualFold(policy, "null") {
				continue
			}
			if _, ok := p.ModelByName[model.Name]; !ok {
				return config.Provider{}, ErrCatalogDependency
			}
			if model.DisplayName != "" || model.UpstreamName != "" || model.Protocol != "" || len(model.Capabilities) != 0 ||
				(strings.TrimSpace(string(model.Pricing)) != "" && !strings.EqualFold(strings.TrimSpace(string(model.Pricing)), "null")) {
				return config.Provider{}, ErrCatalogDependency
			}
		}
		for _, model := range p.Models {
			if !dependencyModelValid(p, model) {
				return config.Provider{}, ErrCatalogDependency
			}
		}
		return p, nil
	}
	for _, row := range models {
		m := config.Model{Name: row.Name, UpstreamName: row.UpstreamName, Protocol: config.ModelProtocol(row.Protocol)}
		if m.UpstreamName == "" {
			m.UpstreamName = m.Name
		}
		for _, capability := range row.Capabilities {
			m.Capabilities = append(m.Capabilities, config.Capability(capability))
		}
		p.Models = append(p.Models, m)
		p.ModelByName[m.Name] = m
	}
	return p, nil
}

func dependencyModelValid(p config.Provider, m config.Model) bool {
	return config.ValidateModelCapabilities(p.Type, p.Name, m) == nil
}

func (g *catalogGraph) alias(ctx context.Context, row DBAlias, targets []DBAliasTarget) error {
	a := config.Alias{
		Name: "dependency", Algorithm: config.AlgorithmRoundRobin,
		Workspace: identity.InWorkspace(row.WorkspaceID),
	}
	for _, target := range targets {
		p, err := g.provider(ctx, target.Provider, row.WorkspaceID)
		if err != nil {
			return err
		}
		m, ok := p.ModelByName[target.Model]
		if !ok || !p.Enabled || !dependencyModelValid(p, m) {
			return ErrCatalogDependency
		}
		a.Targets = append(a.Targets, config.AliasTarget{Provider: target.Provider, Model: target.Model})
	}
	var enabled, disabled []config.Provider
	for _, p := range g.providers {
		if p.Enabled {
			enabled = append(enabled, p)
		} else {
			disabled = append(disabled, p)
		}
	}
	if err := config.ValidateDynamicAlias(a, config.NewCatalog(enabled, disabled, nil)); err != nil {
		return dependencyError(err)
	}
	return nil
}

func validateProviderGraph(ctx context.Context, tx bun.Tx, p DBProvider, options []CatalogEditOptions) error {
	g, err := newCatalogGraph(ctx, tx, options)
	if err != nil {
		return err
	}
	affected := map[string]bool{p.Name: true}
	for changed := true; changed; {
		changed = false
		for name, row := range g.rows {
			if row.Extends != "" && affected[row.Extends] && !affected[name] {
				affected[name], changed = true, true
			}
		}
	}
	names := make([]string, 0, len(affected))
	for name := range affected {
		names = append(names, name)
		row, ok := g.rows[name]
		if !ok {
			if name != p.Name {
				return ErrCatalogDependency
			}
			continue
		}
		if _, err := g.provider(ctx, name, row.WorkspaceID); err != nil {
			return err
		}
	}
	var aliases []DBAlias
	if err := tx.NewSelect().Model(&aliases).Column("id", "name", "workspace_id").
		Where("id IN (?)", tx.NewSelect().Model((*DBAliasTarget)(nil)).Column("alias_id").Where("provider IN (?)", bun.In(names))).
		Scan(ctx); err != nil {
		return err
	}
	for _, a := range aliases {
		var targets []DBAliasTarget
		if err := tx.NewSelect().Model(&targets).Where("alias_id = ?", a.ID).Scan(ctx); err != nil {
			return err
		}
		if err := g.alias(ctx, a, targets); err != nil {
			return err
		}
	}
	for _, a := range g.options.Static.Aliases() {
		if a.DatabaseManaged {
			continue
		}
		var targets []DBAliasTarget
		touched := false
		for _, target := range a.Targets {
			touched = touched || affected[target.Provider]
			targets = append(targets, DBAliasTarget{Provider: target.Provider, Model: target.Model})
		}
		if touched {
			if err := g.alias(ctx, DBAlias{Name: a.Name, WorkspaceID: SystemWorkspaceID}, targets); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAliasGraph(ctx context.Context, tx bun.Tx, a DBAlias, targets []DBAliasTarget, options []CatalogEditOptions) error {
	g, err := newCatalogGraph(ctx, tx, options)
	if err != nil {
		return err
	}
	for _, static := range g.options.Static.Aliases() {
		if !static.DatabaseManaged && static.Name == a.Name {
			return ErrCatalogDependency
		}
	}
	return g.alias(ctx, a, targets)
}

func lockCatalogProvider(ctx context.Context, tx bun.Tx, id uuid.UUID) (DBProvider, error) {
	var p DBProvider
	if err := tx.NewSelect().Model(&p).Column("id", "workspace_id").Where("id = ?", id).Scan(ctx); err != nil {
		return DBProvider{}, err
	}
	if err := lockMembershipWorkspace(ctx, tx, p.WorkspaceID); err != nil {
		return DBProvider{}, err
	}
	workspaceID := p.WorkspaceID
	if err := tx.NewSelect().Model(&p).Column("id", "name", "type", "extends", "enabled", "workspace_id", "updated_at").
		Where("id = ? AND workspace_id = ?", id, workspaceID).For("UPDATE").Scan(ctx); err != nil {
		return DBProvider{}, err
	}
	return p, nil
}

func lockCatalogAlias(ctx context.Context, tx bun.Tx, id uuid.UUID) (DBAlias, error) {
	var a DBAlias
	if err := tx.NewSelect().Model(&a).Column("id", "workspace_id").Where("id = ?", id).Scan(ctx); err != nil {
		return DBAlias{}, err
	}
	if err := lockMembershipWorkspace(ctx, tx, a.WorkspaceID); err != nil {
		return DBAlias{}, err
	}
	workspaceID := a.WorkspaceID
	if err := tx.NewSelect().Model(&a).Column("id", "name", "workspace_id", "updated_at").
		Where("id = ? AND workspace_id = ?", id, workspaceID).For("UPDATE").Scan(ctx); err != nil {
		return DBAlias{}, err
	}
	return a, nil
}
