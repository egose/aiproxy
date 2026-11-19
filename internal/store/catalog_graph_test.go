package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type savedCatalogGraph struct {
	Providers []DBProvider
	Models    map[uuid.UUID][]DBProviderModel
	Aliases   []DBAlias
	Targets   map[uuid.UUID][]DBAliasTarget
}

func readSavedCatalogGraph(t *testing.T, st *Store) savedCatalogGraph {
	t.Helper()
	ctx := context.Background()
	out := savedCatalogGraph{Models: make(map[uuid.UUID][]DBProviderModel), Targets: make(map[uuid.UUID][]DBAliasTarget)}
	var err error
	out.Providers, err = st.ListProviders(ctx)
	flowOK(t, err)
	for _, p := range out.Providers {
		out.Models[p.ID], err = st.ListProviderModels(ctx, p.ID)
		flowOK(t, err)
	}
	out.Aliases, err = st.ListAliases(ctx)
	flowOK(t, err)
	for _, a := range out.Aliases {
		out.Targets[a.ID], err = st.ListAliasTargets(ctx, a.ID)
		flowOK(t, err)
	}
	return out
}

func graphTestProvider(t *testing.T, st *Store, name, typ string, workspace uuid.UUID, models ...DBProviderModel) DBProvider {
	t.Helper()
	p := DBProvider{Name: name, Type: typ, WorkspaceID: workspace, Enabled: true}
	flowOK(t, st.CreateProviderAggregate(context.Background(), &p, models))
	return p
}

func graphTestAlias(t *testing.T, st *Store, name string, workspace uuid.UUID, targets ...DBAliasTarget) DBAlias {
	t.Helper()
	a := DBAlias{Name: name, Algorithm: "round_robin", WorkspaceID: workspace}
	flowOK(t, st.CreateAlias(context.Background(), &a, targets))
	return a
}

func assertSavedGraphValid(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	flowOK(t, st.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		g, err := newCatalogGraph(ctx, tx, nil)
		if err != nil {
			return err
		}
		for name, row := range g.rows {
			if _, err := g.provider(ctx, name, row.WorkspaceID); err != nil {
				return err
			}
		}
		aliases, err := st.ListAliases(ctx)
		if err != nil {
			return err
		}
		for _, a := range aliases {
			targets, err := st.ListAliasTargets(ctx, a.ID)
			if err != nil {
				return err
			}
			if err := g.alias(ctx, a, targets); err != nil {
				return err
			}
		}
		return nil
	}))
}

func TestCatalogDependencyDestructiveEdits(t *testing.T) {
	for _, test := range []struct {
		name    string
		child   bool
		alias   bool
		overlay bool
		typ     string
		edit    string
	}{
		{name: "delete-direct-target", alias: true, edit: "delete"},
		{name: "disable-direct-target", alias: true, edit: "disable"},
		{name: "remove-direct-model-aggregate", alias: true, edit: "models"},
		{name: "remove-direct-model-standalone", alias: true, edit: "standalone-models"},
		{name: "remove-inherited-alias-model", child: true, alias: true, edit: "models"},
		{name: "remove-disabled-child-overlay-model", child: true, overlay: true, edit: "models"},
		{name: "delete-disabled-child-base", child: true, edit: "delete"},
		{name: "disable-disabled-child-base", child: true, edit: "disable"},
		{name: "change-disabled-child-base-type", child: true, edit: "type"},
		{name: "turn-required-base-into-derivative", child: true, edit: "derive"},
		{name: "derivative-loses-alias-model-on-retarget", child: true, alias: true, edit: "retarget"},
		{name: "derivative-loses-alias-model-on-exit", child: true, alias: true, edit: "concrete"},
		{name: "incompatible-direct-capabilities", alias: true, edit: "capabilities"},
		{name: "incompatible-inherited-capabilities", child: true, alias: true, edit: "capabilities"},
		{name: "incompatible-protocol", typ: "opencode-zen", alias: true, edit: "protocol"},
		{name: "invalid-protocol-with-explicit-capability", typ: "opencode-zen", alias: true, edit: "invalid-protocol"},
		{name: "invalid-disabled-inherited-protocol", typ: "opencode-zen", child: true, edit: "invalid-protocol"},
		{name: "unsupported-disabled-inherited-capability", typ: "opencode-zen", child: true, edit: "capabilities"},
	} {
		t.Run(test.name, func(t *testing.T) {
			st := openInviteTestStore(t)
			ctx := context.Background()
			typ := test.typ
			if typ == "" {
				typ = "openai"
			}
			m := DBProviderModel{Name: "m", Capabilities: []string{"chat"}}
			keep := DBProviderModel{Name: "keep", Capabilities: []string{"chat"}}
			if typ == "opencode-zen" {
				m.Protocol, keep.Protocol = "chat", "chat"
			}
			base := graphTestProvider(t, st, "base", typ, SystemWorkspaceID, m, keep)
			peer := graphTestProvider(t, st, "peer", typ, SystemWorkspaceID, keep)
			target := base
			var child DBProvider
			if test.child {
				child = DBProvider{Name: "child", Type: typ, Extends: base.Name, WorkspaceID: SystemWorkspaceID, Enabled: test.alias}
				var overlays []DBProviderModel
				if test.overlay {
					overlays = []DBProviderModel{{Name: "m", EncryptedReasoning: json.RawMessage(`{}`)}}
				}
				flowOK(t, st.CreateProviderAggregate(ctx, &child, overlays))
				target = child
			}
			if test.alias {
				graphTestAlias(t, st, "route", SystemWorkspaceID,
					DBAliasTarget{Provider: target.Name, Model: "m"}, DBAliasTarget{Provider: peer.Name, Model: "keep"})
			}
			before := readSavedCatalogGraph(t, st)
			candidate := base
			replacement := []DBProviderModel{keep}
			switch test.edit {
			case "disable":
				candidate.Enabled = false
			case "type":
				candidate.Type = "gemini"
			case "derive":
				candidate.Extends = peer.Name
				replacement = []DBProviderModel{}
			case "retarget":
				candidate = child
				candidate.Extends = peer.Name
				replacement = []DBProviderModel{}
			case "concrete":
				candidate = child
				candidate.Extends = ""
			case "capabilities":
				m.Capabilities = []string{"embeddings"}
				replacement = []DBProviderModel{m, keep}
			case "protocol":
				m.Protocol, m.Capabilities = "responses", nil
				replacement = []DBProviderModel{m, keep}
			case "invalid-protocol":
				m.Protocol = "invalid"
				replacement = []DBProviderModel{m, keep}
			}
			input := candidate
			inputModels := append([]DBProviderModel{}, replacement...)
			var err error
			switch test.edit {
			case "delete":
				err = st.DeleteProvider(ctx, base.ID)
			case "disable", "type":
				err = st.UpdateProvider(ctx, &candidate)
			case "standalone-models":
				err = st.ReplaceProviderModels(ctx, base.ID, replacement)
			default:
				err = st.UpdateProviderAggregate(ctx, &candidate, &replacement)
			}
			if !errors.Is(err, ErrCatalogDependency) {
				t.Fatalf("destructive edit = %v, want dependency rejection", err)
			}
			if !reflect.DeepEqual(before, readSavedCatalogGraph(t, st)) || !reflect.DeepEqual(input, candidate) ||
				!reflect.DeepEqual(inputModels, replacement) {
				t.Fatal("dependency rejection changed rows, credentials, revisions, models, targets, or caller outputs")
			}
			assertSavedGraphValid(t, st)
		})
	}
}

func TestCatalogDependencyCleanup(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	m := DBProviderModel{Name: "m"}
	base := graphTestProvider(t, st, "base", "openai", SystemWorkspaceID, m)
	peer := graphTestProvider(t, st, "peer", "openai", SystemWorkspaceID, m)
	child := DBProvider{Name: "child", Type: base.Type, Extends: base.Name, Enabled: true, WorkspaceID: SystemWorkspaceID}
	flowOK(t, st.CreateProvider(ctx, &child))
	direct := graphTestAlias(t, st, "direct", SystemWorkspaceID, DBAliasTarget{Provider: base.Name, Model: "m"})
	inherited := graphTestAlias(t, st, "inherited", SystemWorkspaceID, DBAliasTarget{Provider: child.Name, Model: "m"})
	if err := st.DeleteProvider(ctx, base.ID); !errors.Is(err, ErrCatalogDependency) {
		t.Fatal(err)
	}
	flowOK(t, st.UpdateAlias(ctx, &direct, []DBAliasTarget{{Provider: peer.Name, Model: "m"}}))
	child.Extends = peer.Name
	flowOK(t, st.UpdateProvider(ctx, &child))
	flowOK(t, st.DeleteProvider(ctx, base.ID))
	assertSavedGraphValid(t, st)
	flowOK(t, st.DeleteAlias(ctx, inherited.ID))
	child.Enabled = false
	flowOK(t, st.UpdateProvider(ctx, &child))
	if err := st.DeleteProvider(ctx, peer.ID); !errors.Is(err, ErrCatalogDependency) {
		t.Fatal("disabled derivative stopped requiring its base", err)
	}
	flowOK(t, st.DeleteAlias(ctx, direct.ID))
	flowOK(t, st.DeleteProvider(ctx, child.ID))
	flowOK(t, st.ReplaceProviderModels(ctx, peer.ID, []DBProviderModel{{Name: "replacement"}}))
	flowOK(t, st.DeleteProvider(ctx, peer.ID))
	assertSavedGraphValid(t, st)
}

func TestCatalogDependencyConcurrentEdits(t *testing.T) {
	for _, change := range []string{"delete", "disable", "models", "derive", "capabilities", "protocol"} {
		for _, referencer := range []string{"alias-create", "alias-update", "child-create", "child-update", "child-overlay"} {
			if (change == "capabilities" || change == "protocol") && (referencer == "child-create" || referencer == "child-update" || referencer == "child-overlay") {
				continue
			}
			if change == "derive" && referencer == "alias-create" || change == "derive" && referencer == "alias-update" {
				continue
			}
			for _, dependentFirst := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/dependent-first=%t", change, referencer, dependentFirst), func(t *testing.T) {
					st, other, workspace, _ := flowStores(t)
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					typ := "openai"
					m := DBProviderModel{Name: "m", Capabilities: []string{"chat"}}
					keep := DBProviderModel{Name: "keep", Capabilities: []string{"chat"}}
					if change == "protocol" {
						typ, m.Protocol, keep.Protocol = "opencode-zen", "chat", "chat"
					}
					base := graphTestProvider(t, st, "base", typ, workspace, m, keep)
					peer := graphTestProvider(t, st, "peer", typ, workspace, keep)
					edited := base
					replacement := []DBProviderModel{keep}
					switch change {
					case "disable":
						edited.Enabled = false
					case "derive":
						edited.Extends = peer.Name
						replacement = []DBProviderModel{}
					case "capabilities":
						m.Capabilities = []string{"embeddings"}
						replacement = []DBProviderModel{m, keep}
					case "protocol":
						m.Protocol, m.Capabilities = "responses", nil
						replacement = []DBProviderModel{m, keep}
					}
					edit := func() error {
						switch change {
						case "delete":
							return st.DeleteProvider(ctx, base.ID)
						case "disable":
							return st.UpdateProvider(ctx, &edited)
						case "models":
							return st.ReplaceProviderModels(ctx, base.ID, replacement)
						default:
							return st.UpdateProviderAggregate(ctx, &edited, &replacement)
						}
					}
					var dependent func() error
					switch referencer {
					case "alias-create", "alias-update":
						a := DBAlias{Name: "route", Algorithm: "round_robin", WorkspaceID: workspace}
						targets := []DBAliasTarget{{Provider: base.Name, Model: "m"}, {Provider: peer.Name, Model: "keep"}}
						if referencer == "alias-update" {
							flowOK(t, st.CreateAlias(ctx, &a, []DBAliasTarget{{Provider: peer.Name, Model: "keep"}}))
							dependent = func() error { return other.UpdateAlias(ctx, &a, targets) }
						} else {
							dependent = func() error { return other.CreateAlias(ctx, &a, targets) }
						}
					case "child-create", "child-update", "child-overlay":
						child := DBProvider{Name: "child", Type: typ, Extends: peer.Name, WorkspaceID: workspace}
						overlays := []DBProviderModel{{Name: "m", EncryptedReasoning: json.RawMessage(`{}`)}}
						if referencer == "child-overlay" {
							child.Extends = base.Name
							flowOK(t, st.CreateProvider(ctx, &child))
							dependent = func() error { return other.UpdateProviderAggregate(ctx, &child, &overlays) }
						} else if referencer == "child-update" {
							flowOK(t, st.CreateProvider(ctx, &child))
							child.Extends = base.Name
							dependent = func() error { return other.UpdateProviderAggregate(ctx, &child, &overlays) }
						} else {
							child.Extends = base.Name
							dependent = func() error { return other.CreateProviderAggregate(ctx, &child, overlays) }
						}
					}
					holder, err := st.DB.BeginTx(ctx, nil)
					flowOK(t, err)
					defer holder.Rollback()
					flowOK(t, lockMembershipWorkspace(ctx, holder, workspace))
					first, second := edit, dependent
					if dependentFirst {
						first, second = second, first
					}
					results := make(chan error, 2)
					go func() { results <- first() }()
					waitMembershipLocks(t, ctx, st, workspace, 1)
					go func() { results <- second() }()
					waitMembershipLocks(t, ctx, st, workspace, 2)
					flowOK(t, holder.Commit())
					succeeded, rejected := 0, 0
					for range 2 {
						err := <-results
						if err == nil {
							succeeded++
						} else if errors.Is(err, ErrCatalogDependency) {
							rejected++
						} else {
							t.Fatalf("concurrent operation = %v", err)
						}
					}
					if succeeded != 1 || rejected != 1 {
						t.Fatalf("nonserial graph outcome: successes=%d rejections=%d", succeeded, rejected)
					}
					assertSavedGraphValid(t, st)
				})
			}
		}
	}
}

func TestCatalogDependencyCopilotConsumeRollback(t *testing.T) {
	for _, purpose := range []string{"create", "edit"} {
		t.Run(purpose, func(t *testing.T) {
			st, other, workspace, users := flowStores(t)
			ctx := context.Background()
			base := graphTestProvider(t, st, "base", "github-copilot", workspace, DBProviderModel{Name: "m"}, DBProviderModel{Name: "keep"})
			var providerID *uuid.UUID
			candidate := base
			if purpose == "edit" {
				providerID = &base.ID
				graphTestAlias(t, st, "route", workspace, DBAliasTarget{Provider: base.Name, Model: "m"})
			} else {
				candidate = DBProvider{Name: "child", Type: base.Type, WorkspaceID: workspace, Extends: base.Name}
			}
			status := flowReady(t, st, users[0].ID, workspace, providerID)
			snap, err := other.ReadyCopilotFlow(ctx, users[0].ID, status.ID)
			flowOK(t, err)
			flowOK(t, snap.AttachCredential(&candidate))
			if purpose == "create" {
				flowOK(t, st.DeleteProvider(ctx, base.ID))
			}
			before := readSavedCatalogGraph(t, st)
			beforeFlow := flowRow(t, st, status.ID)
			input := candidate
			models := []DBProviderModel{{Name: "keep"}}
			if purpose == "create" {
				models = []DBProviderModel{}
			}
			err = other.ConsumeCopilotFlow(ctx, snap, &candidate, &models)
			if !errors.Is(err, ErrCatalogDependency) {
				t.Fatalf("invalid flow aggregate = %v", err)
			}
			if !reflect.DeepEqual(before, readSavedCatalogGraph(t, st)) || !reflect.DeepEqual(beforeFlow, flowRow(t, st, status.ID)) ||
				!reflect.DeepEqual(input, candidate) {
				t.Fatal("dependency rejection changed catalog, credentials, flow readiness, or caller output")
			}
			if purpose == "edit" {
				a, err := st.GetAlias(ctx, "route")
				flowOK(t, err)
				flowOK(t, st.DeleteAlias(ctx, a.ID))
			} else {
				base = graphTestProvider(t, st, "base", "github-copilot", workspace, DBProviderModel{Name: "keep"})
			}
			flowOK(t, other.ConsumeCopilotFlow(ctx, snap, &candidate, &models))
			if flowRow(t, st, status.ID).State != "consumed" {
				t.Fatal("valid cleanup did not allow consuming the same ready flow")
			}
			assertSavedGraphValid(t, st)
		})
	}
}

func TestCatalogDependencyConcurrentCleanup(t *testing.T) {
	for _, cleanup := range []string{"alias-delete", "alias-retarget", "child-delete", "child-retarget"} {
		for _, dependentFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dependent-first=%t", cleanup, dependentFirst), func(t *testing.T) {
				st, other, workspace, _ := flowStores(t)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				base := graphTestProvider(t, st, "base", "openai", workspace, DBProviderModel{Name: "m"})
				peer := graphTestProvider(t, st, "peer", "openai", workspace, DBProviderModel{Name: "m"})
				var removeReference func() error
				if strings.HasPrefix(cleanup, "alias-") {
					a := graphTestAlias(t, st, "route", workspace, DBAliasTarget{Provider: base.Name, Model: "m"})
					if cleanup == "alias-delete" {
						removeReference = func() error { return other.DeleteAlias(ctx, a.ID) }
					} else {
						removeReference = func() error { return other.UpdateAlias(ctx, &a, []DBAliasTarget{{Provider: peer.Name, Model: "m"}}) }
					}
				} else {
					child := DBProvider{Name: "child", Type: base.Type, Extends: base.Name, WorkspaceID: workspace}
					flowOK(t, st.CreateProvider(ctx, &child))
					if cleanup == "child-delete" {
						removeReference = func() error { return other.DeleteProvider(ctx, child.ID) }
					} else {
						child.Extends = peer.Name
						removeReference = func() error { return other.UpdateProvider(ctx, &child) }
					}
				}
				holder, err := st.DB.BeginTx(ctx, nil)
				flowOK(t, err)
				defer holder.Rollback()
				flowOK(t, lockMembershipWorkspace(ctx, holder, workspace))
				removeProvider := func() error { return st.DeleteProvider(ctx, base.ID) }
				first, second := removeProvider, removeReference
				if dependentFirst {
					first, second = second, first
				}
				results := make(chan error, 2)
				go func() { results <- first() }()
				waitMembershipLocks(t, ctx, st, workspace, 1)
				go func() { results <- second() }()
				waitMembershipLocks(t, ctx, st, workspace, 2)
				flowOK(t, holder.Commit())
				rejected := 0
				for range 2 {
					err := <-results
					if errors.Is(err, ErrCatalogDependency) {
						rejected++
					} else {
						flowOK(t, err)
					}
				}
				wantRejected := 1
				if dependentFirst {
					wantRejected = 0
				}
				if rejected != wantRejected {
					t.Fatalf("cleanup order rejected %d operations, want %d", rejected, wantRejected)
				}
				if rejected != 0 {
					flowOK(t, st.DeleteProvider(ctx, base.ID))
				}
				assertSavedGraphValid(t, st)
			})
		}
	}
}

func TestCatalogDependencyConcurrentCopilotConsume(t *testing.T) {
	for _, purpose := range []string{"create", "edit"} {
		for _, flowFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/flow-first=%t", purpose, flowFirst), func(t *testing.T) {
				st, other, workspace, users := flowStores(t)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				base := graphTestProvider(t, st, "base", "github-copilot", workspace, DBProviderModel{Name: "m"}, DBProviderModel{Name: "keep"})
				candidate := DBProvider{Name: "child", Type: base.Type, Extends: base.Name, WorkspaceID: workspace}
				models := []DBProviderModel{}
				var providerID *uuid.UUID
				var change func() error
				if purpose == "create" {
					change = func() error { return other.DeleteProvider(ctx, base.ID) }
				} else {
					providerID, candidate = &base.ID, base
					models = []DBProviderModel{{Name: "keep"}}
					a := DBAlias{Name: "route", WorkspaceID: workspace, Algorithm: "round_robin"}
					change = func() error { return other.CreateAlias(ctx, &a, []DBAliasTarget{{Provider: base.Name, Model: "m"}}) }
				}
				status := flowReady(t, st, users[0].ID, workspace, providerID)
				snap, err := st.ReadyCopilotFlow(ctx, users[0].ID, status.ID)
				flowOK(t, err)
				flowOK(t, snap.AttachCredential(&candidate))
				consume := func() error { return st.ConsumeCopilotFlow(ctx, snap, &candidate, &models) }
				holder, err := st.DB.BeginTx(ctx, nil)
				flowOK(t, err)
				defer holder.Rollback()
				flowOK(t, lockMembershipWorkspace(ctx, holder, workspace))
				first, second := change, consume
				if flowFirst {
					first, second = second, first
				}
				results := make(chan error, 2)
				go func() { results <- first() }()
				waitMembershipLocks(t, ctx, st, workspace, 1)
				go func() { results <- second() }()
				waitMembershipLocks(t, ctx, st, workspace, 2)
				flowOK(t, holder.Commit())
				saved, rejected := 0, 0
				for range 2 {
					err := <-results
					if err == nil {
						saved++
					} else if errors.Is(err, ErrCatalogDependency) {
						rejected++
					} else {
						t.Fatal(err)
					}
				}
				if saved != 1 || rejected != 1 {
					t.Fatalf("Copilot graph race saved=%d rejected=%d", saved, rejected)
				}
				wantState := "ready"
				if flowFirst {
					wantState = "consumed"
				}
				if flowRow(t, st, status.ID).State != wantState {
					t.Fatal("graph race changed flow consumption independently of aggregate commit")
				}
				assertSavedGraphValid(t, st)
			})
		}
	}
}

func TestCatalogDependencyEffectiveStaticAndSavedGraph(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	model := config.Model{Name: "m", UpstreamName: "m", Capabilities: []config.Capability{config.CapabilityChat}}
	static := config.Provider{Name: "static", Type: config.ProviderTypeOpenAI, Enabled: true, Models: []config.Model{model}}
	stale := config.Provider{Name: "base", Type: config.ProviderTypeOpenAI, Enabled: true, DatabaseManaged: true, Models: []config.Model{model}}
	options := CatalogEditOptions{Static: config.NewCatalog([]config.Provider{static, stale}, nil, nil)}
	child := DBProvider{Name: "child", Type: "openai", WorkspaceID: SystemWorkspaceID, Extends: static.Name}
	flowOK(t, st.CreateProviderAggregate(ctx, &child, []DBProviderModel{{Name: "m", EncryptedReasoning: json.RawMessage(`{}`)}}, options))
	base := graphTestProvider(t, st, "base", "openai", SystemWorkspaceID, DBProviderModel{Name: "keep"})
	a := DBAlias{Name: "route", Algorithm: "round_robin", WorkspaceID: SystemWorkspaceID}
	if err := st.CreateAlias(ctx, &a, []DBAliasTarget{{Provider: base.Name, Model: "m"}}, options); !errors.Is(err, ErrCatalogDependency) {
		t.Fatal("stale runtime managed inventory supplied a removed model", err)
	}
	workspace := Workspace{Name: "tenant"}
	flowOK(t, st.CreateWorkspace(ctx, &workspace))
	foreign := DBProvider{Name: "foreign", Type: "openai", WorkspaceID: workspace.ID, Extends: static.Name}
	if err := st.CreateProvider(ctx, &foreign, options); !errors.Is(err, ErrCatalogDependency) {
		t.Fatal("static base escaped System", err)
	}
	child.Enabled = true
	flowOK(t, st.UpdateProvider(ctx, &child, options))
	flowOK(t, st.CreateAlias(ctx, &a, []DBAliasTarget{{Provider: child.Name, Model: "m"}}, options))
}

func TestCatalogDependencyDeleteStaticAliasTarget(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	base := graphTestProvider(t, st, "base", "openai", SystemWorkspaceID, DBProviderModel{Name: "m", Capabilities: []string{"chat"}})
	staticAlias := config.Alias{
		Name: "static-route", Algorithm: config.AlgorithmRoundRobin,
		Targets: []config.AliasTarget{{Provider: base.Name, Model: "m"}},
	}
	options := CatalogEditOptions{Static: config.NewCatalog(nil, nil, []config.Alias{staticAlias})}
	before := readSavedCatalogGraph(t, st)
	if err := st.DeleteProvider(ctx, base.ID, options); !errors.Is(err, ErrCatalogDependency) {
		t.Fatalf("static alias target deletion = %v, want dependency rejection", err)
	}
	if !reflect.DeepEqual(before, readSavedCatalogGraph(t, st)) {
		t.Fatal("static alias dependency rejection changed saved rows")
	}
	options.Static = config.NewCatalog(nil, nil, nil)
	flowOK(t, st.DeleteProvider(ctx, base.ID, options))
	assertSavedGraphValid(t, st)
}

func TestCatalogDependencyUnrelatedCorruptionRemainsRepairable(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	bad := DBProvider{ID: uuid.New(), Name: "corrupt", Type: "openai", Extends: "absent", WorkspaceID: SystemWorkspaceID}
	flowOK(t, execInsert(t, st, &bad))
	_, err := st.DB.ExecContext(ctx, `UPDATE db_providers SET forward_headers = '{}'::jsonb WHERE id = ?`, bad.ID)
	flowOK(t, err)
	broken := DBAlias{ID: uuid.New(), Name: "broken", Algorithm: "round_robin", WorkspaceID: SystemWorkspaceID}
	flowOK(t, execInsert(t, st, &broken))
	target := DBAliasTarget{ID: uuid.New(), AliasID: broken.ID, Provider: "absent", Model: "m"}
	flowOK(t, execInsert(t, st, &target))
	valid := graphTestProvider(t, st, "valid", "openai", SystemWorkspaceID, DBProviderModel{Name: "m"})
	valid.DisplayName = "saved-despite-unrelated-corruption"
	flowOK(t, st.UpdateProvider(ctx, &valid))
	broken, err = st.GetAlias(ctx, broken.Name)
	flowOK(t, err)
	flowOK(t, st.UpdateAlias(ctx, &broken, []DBAliasTarget{{Provider: valid.Name, Model: "m"}}))
	flowOK(t, st.DeleteProvider(ctx, bad.ID))
	assertSavedGraphValid(t, st)
}

func execInsert(t *testing.T, st *Store, model any) error {
	t.Helper()
	_, err := st.DB.NewInsert().Model(model).Exec(context.Background())
	return err
}
