package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func deferredAdminDeclarations() []struct {
	field string
	value any
} {
	var out []struct {
		field string
		value any
	}
	for _, field := range []string{
		"deferred", "deferred_transport", "deferred_receiver",
		"DeFeRrEd_TrAnSpOrT", "DEFERRED_RECEIVER", "deferred_tran\u017fport",
		"deferred_transport_config", "deferred_receiver_enabled",
	} {
		for _, value := range []any{nil, false, "", map[string]any{}, []any{}, map[string]any{"private-setting": true}} {
			out = append(out, struct {
				field string
				value any
			}{field, value})
		}
	}
	return out
}

func TestManagedDeferredDeclarationsRejectBeforeMutationOrStoreAccess(t *testing.T) {
	h := &Handler{}
	for _, declaration := range deferredAdminDeclarations() {
		raw, err := json.Marshal(map[string]any{
			"name": "managed", "display_name": "Rejected change", "api_key": "rejected-private-token",
			declaration.field: declaration.value,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Run(string(raw), func(t *testing.T) {
			var provider adminProviderUpsert
			var credential adminCredentialUpsert
			if err := json.Unmarshal(raw, &provider); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &credential); err != nil {
				t.Fatal(err)
			}
			row := store.DBProvider{Name: "managed", Type: "openai", DisplayName: "Unchanged"}
			before := row
			err := h.applyProviderUpsert(row.Name, &row, provider)
			if err == nil || !strings.Contains(err.Error(), "static-config-only") || strings.Contains(err.Error(), "private-") || !reflect.DeepEqual(row, before) {
				t.Fatalf("forbidden declaration mutated candidate or leaked data: %v", err)
			}
			if _, err := h.createDBProvider(context.Background(), Dependencies{}, nil, nil, store.Workspace{}, provider); err == nil || !strings.Contains(err.Error(), "static-config-only") {
				t.Fatalf("create did not reject before accessing the store: %v", err)
			}
			if _, err := h.updateDBProvider(context.Background(), Dependencies{}, nil, nil, provider); err == nil || !strings.Contains(err.Error(), "static-config-only") {
				t.Fatalf("update did not reject before accessing the store: %v", err)
			}
			if err := h.setProviderCredential(context.Background(), Dependencies{}, nil, "managed", credential); err == nil || !strings.Contains(err.Error(), "static-config-only") {
				t.Fatalf("credential update did not reject before accessing the store: %v", err)
			}
		})
	}
	for _, field := range []string{"x_deferred_transport", "unrelated_setting", "deferre_receiver"} {
		raw, _ := json.Marshal(map[string]any{field: false})
		var provider adminProviderUpsert
		var credential adminCredentialUpsert
		if err := json.Unmarshal(raw, &provider); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &credential); err != nil {
			t.Fatal(err)
		}
		if provider.unsupportedDeferredDeclaration || credential.unsupportedDeferredDeclaration {
			t.Fatal("unrelated unknown-field semantics changed")
		}
	}
}

func TestManagedDeferredInheritanceAndViewRejectOptedBases(t *testing.T) {
	h := &Handler{}
	for _, caller := range []bool{false, true} {
		base := config.Provider{
			Name: "base", Type: config.ProviderTypeOpenAICompatible, Enabled: true,
			BaseURL: "https://peer.example/v1", APIKey: "k",
			Models: []config.Model{{Name: "m", UpstreamName: "m"}},
		}
		if caller {
			d := config.DefaultDeferredTransport()
			base.DeferredTransport = &d
		} else {
			base.DeferredReceiver = &config.DeferredReceiver{ExecutionTimeout: time.Minute, MaxResultBytes: 4096}
		}
		deps := Dependencies{Catalog: config.NewCatalog([]config.Provider{base}, nil, nil)}
		if err := h.validateExtends(deps, "managed", "base", "openai-compatible"); err == nil || !strings.Contains(err.Error(), "static-config-only") {
			t.Fatalf("admin inheritance accepted an opted static base: %v", err)
		}
		row := store.DBProvider{Name: "managed", Type: "openai-compatible", Extends: "base"}
		if _, err := h.buildConfigProvider(deps, row, nil); err == nil || !strings.Contains(err.Error(), "static-config-only") {
			t.Fatalf("credential/update construction accepted an opted static base: %v", err)
		}
		if _, err := managedProviderTLSMetadata(row, nil, deps.Catalog); err == nil || !strings.Contains(err.Error(), "static-config-only") {
			t.Fatalf("managed view silently exposed an invalid opted-base derivative: %v", err)
		}
	}
}

func TestAdminDeferredDeclarationsRejectBeforePersistenceForAllRoles(t *testing.T) {
	st := openValidationStore(t)
	h, global := validationTestHandler(t, st)
	ctx := context.Background()
	workspace := mkWorkspace(t, st, h, global, "deferred-workspace")
	_, admin := mkWorkspaceMember(t, st, h, global, uniqName(t, "deferred-owner")+"@example.test", workspace, "admin")
	name := uniqName(t, "deferred-preserved")
	tlsAdminCleanupProvider(t, st, name)
	body := validProviderBody(name)
	body["workspace_id"] = workspace
	if w, _ := tlsAdminRequest(t, h, global, http.MethodPost, "/_internal/admin/providers", body); w.Code != http.StatusCreated {
		t.Fatal(w.Body.String())
	}
	before, err := st.GetProvider(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	beforeModels, err := st.ListProviderModels(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	activations := 0
	deps := h.current()
	deps.RequestReload = func() error { activations++; return nil }
	h.UpdateDependencies(deps)
	for _, access := range []string{global, admin} {
		for _, declaration := range deferredAdminDeclarations() {
			badName := uniqName(t, "deferred-rejected")
			create := validProviderBody(badName)
			create["workspace_id"], create[declaration.field] = workspace, declaration.value
			update := map[string]any{"api_key": "replacement-key", declaration.field: declaration.value}
			for _, op := range []struct {
				method string
				path   string
				body   any
			}{
				{http.MethodPost, "/_internal/admin/providers", create},
				{http.MethodPut, "/_internal/admin/providers/" + name, update},
				{http.MethodPut, "/_internal/admin/providers/" + name + "/credential", update},
			} {
				w, _ := tlsAdminRequest(t, h, access, op.method, op.path, op.body)
				if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" || strings.Contains(w.Body.String(), "private-") {
					t.Fatalf("managed declaration persisted or leaked data: %d %s", w.Code, w.Body.String())
				}
			}
			if _, err := st.GetProvider(ctx, badName); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("forbidden deferred create committed a row")
			}
			after, _ := st.GetProvider(ctx, name)
			afterModels, _ := st.ListProviderModels(ctx, before.ID)
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeModels, afterModels) || !bytes.Equal(before.APIKeyEncrypted, after.APIKeyEncrypted) {
				t.Fatal("forbidden deferred update committed provider, models, or credentials")
			}
		}
	}
	if activations != 0 {
		t.Fatal("forbidden declarations attempted activation")
	}
}

func TestAdminDeferredOptedBaseRejectedOnCreateUpdateAndCredential(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	deps := h.current()
	base, _ := deps.Catalog.Provider("openai")
	base.DeferredReceiver = &config.DeferredReceiver{ExecutionTimeout: time.Minute, MaxResultBytes: 4096}
	deps.Catalog = config.NewCatalog([]config.Provider{base}, nil, nil)
	activations := 0
	deps.RequestReload = func() error { activations++; return nil }
	h.UpdateDependencies(deps)
	name := uniqName(t, "deferred-inheritance")
	tlsAdminCleanupProvider(t, st, name)
	if w, _ := tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", validProviderBody(name)); w.Code != http.StatusCreated {
		t.Fatal(w.Body.String())
	}
	before, _ := st.GetProvider(ctx, name)
	activations = 0
	badName := uniqName(t, "deferred-opted-derived")
	tlsAdminCleanupProvider(t, st, badName)
	create := validProviderBody(badName)
	create["extends"], create["enabled"] = base.Name, false
	for _, op := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/_internal/admin/providers", create},
		{http.MethodPut, "/_internal/admin/providers/" + name, map[string]any{"extends": base.Name}},
	} {
		w, _ := tlsAdminRequest(t, h, access, op.method, op.path, op.body)
		if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" || !strings.Contains(w.Body.String(), "static-config-only") {
			t.Fatal("managed opted-base inheritance was accepted before commit")
		}
	}
	after, _ := st.GetProvider(ctx, name)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected inheritance update committed")
	}
	if _, err := st.GetProvider(ctx, badName); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("rejected inheritance create committed")
	}
	before.Extends = base.Name
	if err := st.UpdateProvider(ctx, &before); err != nil {
		t.Fatal(err)
	}
	before, _ = st.GetProvider(ctx, name)
	w, _ := tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name+"/credential", map[string]any{"api_key": "replacement-key"})
	if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" || !strings.Contains(w.Body.String(), "static-config-only") {
		t.Fatal("credential route accepted a stored opted-base derivative")
	}
	after, _ = st.GetProvider(ctx, name)
	if !reflect.DeepEqual(before, after) || activations != 0 {
		t.Fatal("rejected credential update committed or activated")
	}
}
