package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
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
			if err := h.setProviderCredential(context.Background(), Dependencies{}, nil, nil, "managed", credential); err == nil || !strings.Contains(err.Error(), "static-config-only") {
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

func assertManagedDeferredPayloadRejectedBeforeStore(t *testing.T, raw []byte) {
	t.Helper()
	h := &Handler{}
	var provider adminProviderUpsert
	var credential adminCredentialUpsert
	if err := json.Unmarshal(raw, &provider); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &credential); err != nil {
		t.Fatal(err)
	}
	row := store.DBProvider{Name: "managed", Type: "openai", DisplayName: "Unchanged", APIKeyEncrypted: []byte("unchanged-credential")}
	before := row
	if err := h.applyProviderUpsert(row.Name, &row, provider); err == nil || !strings.Contains(err.Error(), "static-config-only") || strings.Contains(err.Error(), "private-") || !reflect.DeepEqual(row, before) {
		t.Fatalf("nested declaration mutated candidate or reached credential handling: %v", err)
	}
	if _, err := h.createDBProvider(context.Background(), Dependencies{}, nil, nil, store.Workspace{}, provider); err == nil || !strings.Contains(err.Error(), "static-config-only") {
		t.Fatalf("create reached the store for a nested declaration: %v", err)
	}
	if _, err := h.updateDBProvider(context.Background(), Dependencies{}, nil, nil, provider); err == nil || !strings.Contains(err.Error(), "static-config-only") {
		t.Fatalf("update reached the store for a nested declaration: %v", err)
	}
	if err := h.setProviderCredential(context.Background(), Dependencies{}, nil, nil, "managed", credential); err == nil || !strings.Contains(err.Error(), "static-config-only") {
		t.Fatalf("credential update reached the store for an ignored nested declaration: %v", err)
	}
}

func TestManagedNestedDeferredDeclarationsRejectBeforeMutationOrStoreAccess(t *testing.T) {
	for _, parent := range []string{"models", "Models", "MODELS", "model\u017f"} {
		for _, declaration := range deferredAdminDeclarations() {
			t.Run(parent+"/"+declaration.field+"/"+stringMustMarshalDeferred(t, declaration.value), func(t *testing.T) {
				raw, err := json.Marshal(map[string]any{
					"name": "managed", "display_name": "Rejected change", "api_key": "rejected-private-token",
					parent: []any{map[string]any{"name": "m", declaration.field: declaration.value}},
				})
				if err != nil {
					t.Fatal(err)
				}
				assertManagedDeferredPayloadRejectedBeforeStore(t, raw)
			})
		}
	}
}

func stringMustMarshalDeferred(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestManagedNestedDeferredDuplicateFieldsCannotErasePresence(t *testing.T) {
	for _, fields := range []string{
		`"models":[{"name":"m","deferred_transport":{}}],"models":[]`,
		`"models":[{"name":"m","deferred_transport":null}],"models":null`,
		`"models":[{"name":"m","deferred_transport":{}}],"Models":[{"name":"ordinary"}]`,
		`"Models":[{"name":"m","DEFERRED_TRANSPORT":null}],"models":[]`,
		`"model\u017f":[{"name":"m","deferred_tran\u017fport":{}}],"MODELS":null`,
		`"models":[{"name":"m","deferred_transport":{},"deferred_transport":null}]`,
		`"models":[{"name":"m","deferred_transport":{},"Deferred_Transport":false}]`,
		`"models":[{"name":"m","deferred_transport":{},"name":"ordinary"}]`,
		`"models":[{"name":"ordinary"},{"name":"ordinary","deferred_receiver":null}]`,
		`"deferred_transport":{},"deferred_transport":null,"models":[]`,
	} {
		t.Run(fields, func(t *testing.T) {
			raw := []byte(`{"name":"managed","display_name":"Rejected change","api_key":"rejected-private-token",` + fields + `}`)
			assertManagedDeferredPayloadRejectedBeforeStore(t, raw)
		})
	}
}

func TestManagedNestedDeferredDetectionPreservesUnrelatedUnknownFields(t *testing.T) {
	for _, fields := range []string{
		`"models":[{"name":"m","x_deferred_transport":null,"unrelated_setting":{},"deferre_receiver":false}]`,
		`"models":[{"name":"m","unrelated_setting":{"deferred_transport":null}}]`,
		`"unrelated_setting":{"models":[{"deferred_transport":null}]},"models":[{"name":"m"}]`,
		`"models":[{"name":"m","display_name":"First"}],"Models":[{"name":"m","display_name":"Last"}]`,
		`"models":[{"name":"m","unknown":[[[[{"deferred_transport":null}]]]]}]`,
	} {
		t.Run(fields, func(t *testing.T) {
			var provider adminProviderUpsert
			var credential adminCredentialUpsert
			raw := []byte(`{` + fields + `}`)
			if err := json.Unmarshal(raw, &provider); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &credential); err != nil {
				t.Fatal(err)
			}
			if provider.unsupportedDeferredDeclaration || credential.unsupportedDeferredDeclaration {
				t.Fatal("unrelated unknown-field semantics changed")
			}
			models, err := (&Handler{}).buildDBModels("ordinary", provider.Models)
			if err != nil || len(models) != 1 || models[0].Name != "m" {
				t.Fatalf("ordinary managed model behavior changed: %v", err)
			}
			if strings.Contains(fields, `"Last"`) && models[0].DisplayName != "Last" {
				t.Fatal("unrelated duplicate fields no longer use normal JSON last-value behavior")
			}
		})
	}
	for _, raw := range []string{`{"models":false}`, `{"models":42}`, `{"models":[null,false,42,[]]}`} {
		var credential adminCredentialUpsert
		if err := json.Unmarshal([]byte(raw), &credential); err != nil || credential.unsupportedDeferredDeclaration {
			t.Fatalf("ignored credential fields unexpectedly became typed inputs: %v", err)
		}
	}
}

func TestManagedDeferredModelInputsRejectBeforeCandidateMutation(t *testing.T) {
	for _, declaration := range deferredAdminDeclarations() {
		raw, err := json.Marshal(map[string]any{"name": "m", declaration.field: declaration.value})
		if err != nil {
			t.Fatal(err)
		}
		var model adminModelUpsert
		if err := json.Unmarshal(raw, &model); err != nil {
			t.Fatal(err)
		}
		models := []adminModelUpsert{model}
		req := adminProviderUpsert{Name: "managed", Models: &models}
		row := store.DBProvider{Name: "managed", DisplayName: "Unchanged"}
		before := row
		h := &Handler{}
		if err := h.applyProviderUpsert(row.Name, &row, req); err == nil || !strings.Contains(err.Error(), "static-config-only") || !reflect.DeepEqual(before, row) {
			t.Fatalf("direct model input bypassed pre-mutation presence checking: %v", err)
		}
		if _, err := h.buildDBModels(row.Name, &models); err == nil || !strings.Contains(err.Error(), "static-config-only") {
			t.Fatalf("model construction discarded a prohibited declaration: %v", err)
		}
		if _, err := h.createDBProvider(context.Background(), Dependencies{}, nil, nil, store.Workspace{}, req); err == nil || !strings.Contains(err.Error(), "static-config-only") {
			t.Fatalf("direct model create input reached the store: %v", err)
		}
		if _, err := h.updateDBProvider(context.Background(), Dependencies{}, nil, nil, req); err == nil || !strings.Contains(err.Error(), "static-config-only") {
			t.Fatalf("direct model update input reached the store: %v", err)
		}
	}
}

func TestManagedNestedDeferredHTTPRejectsBeforeWorkspaceOrProviderStoreAccess(t *testing.T) {
	t.Setenv("AIPROXY_JWT_SECRET", "[REDACTED]")
	for _, isAdmin := range []bool{false, true} {
		access, _, err := adminauth.IssueAccess("11111111-1111-4111-8111-111111111111", "operator@example.test", isAdmin)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{
			`{"name":"managed","models":[{"name":"m","deferred_transport":null}]}`,
			`{"name":"managed","models":[{"deferred_transport":{}}],"Models":[]}`,
			`{"name":"managed","MODELS":[{"deferred_tran\u017fport":null}],"models":null}`,
		} {
			for _, op := range []struct {
				method string
				rest   []string
			}{
				{http.MethodPost, nil},
				{http.MethodPut, []string{"managed"}},
				{http.MethodPut, []string{"managed", "credential"}},
			} {
				req := httptest.NewRequest(op.method, "/_internal/admin/providers", strings.NewReader(raw))
				req.Header.Set("Authorization", "Bearer "+access)
				w := httptest.NewRecorder()
				activations := 0
				(&Handler{}).adminProviders(Dependencies{RequestReload: func() error { activations++; return nil }}, w, req, op.rest)
				if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" || !strings.Contains(w.Body.String(), "static-config-only") || activations != 0 {
					t.Fatalf("nested HTTP input reached workspace/store mutation or activation: %d %s", w.Code, w.Body.String())
				}
			}
		}
	}
}

func TestManagedDeferredDeclarationInspectionUsesExistingJSONBounds(t *testing.T) {
	unknown := strings.Repeat("[", 512) + `{"deferred_transport":null}` + strings.Repeat("]", 512)
	raw := []byte(`{"name":"managed","unrelated_setting":` + unknown + `,"models":[{"name":"m","deferred_transport":null}],"models":[]}`)
	assertManagedDeferredPayloadRejectedBeforeStore(t, raw)
	raw = []byte(`{"unrelated_setting":` + unknown + `,"models":[{"name":"m"}]}`)
	var provider adminProviderUpsert
	var credential adminCredentialUpsert
	if err := json.Unmarshal(raw, &provider); err != nil || provider.unsupportedDeferredDeclaration {
		t.Fatalf("inspection recursively reclassified unrelated fields: %v", err)
	}
	if err := json.Unmarshal(raw, &credential); err != nil || credential.unsupportedDeferredDeclaration {
		t.Fatalf("credential inspection recursively reclassified unrelated fields: %v", err)
	}
	for _, invalid := range []string{
		`{"models":[{"unknown":` + strings.Repeat("[", 10001) + `0` + strings.Repeat("]", 10001) + `}]}`,
		`{"unrelated_setting":"` + strings.Repeat("x", 1<<20) + `","models":[{"deferred_transport":{}}]}`,
	} {
		for _, req := range []any{&adminProviderUpsert{}, &adminCredentialUpsert{}} {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(invalid))
			if err := json.NewDecoder(ioLimitReader(request)).Decode(req); err == nil {
				t.Fatal("declaration inspection bypassed the request-byte or JSON-depth bound")
			}
		}
	}
}

func managedDeferredModelBase(view string, empty bool) config.Provider {
	model := config.Model{Name: "m", UpstreamName: "m"}
	base := config.Provider{
		Name: "base", Type: config.ProviderTypeOpenAICompatible, Enabled: true,
		BaseURL: "https://peer.example/v1", APIKey: "configured-key",
		Models: []config.Model{model}, ModelByName: map[string]config.Model{"m": model},
	}
	options := config.DefaultDeferredTransport()
	if empty {
		options = config.DeferredTransport{}
	}
	opted := model
	opted.DeferredTransport = &options
	switch view {
	case "ordered":
		base.Models[0] = opted
		base.ModelByName = nil
	case "indexed":
		base.Models = nil
		base.ModelByName["m"] = opted
	case "indexed-with-nil-ordered":
		base.ModelByName["m"] = opted
	case "ordered-with-nil-index":
		base.Models[0] = opted
	case "index-only":
		base.Models = nil
		base.ModelByName = map[string]config.Model{"index-only": {DeferredTransport: &options}}
	case "conflicting":
		base.Models[0] = opted
		other := config.DeferredTransport{}
		model.DeferredTransport = &other
		base.ModelByName["m"] = model
	case "indexed-extra":
		base.ModelByName["extra"] = opted
	}
	return base
}

func TestManagedDeferredModelInheritanceAndViewRejectOptedBases(t *testing.T) {
	h := &Handler{}
	for _, empty := range []bool{false, true} {
		for _, view := range []string{"ordered", "indexed", "indexed-with-nil-ordered", "ordered-with-nil-index", "index-only", "conflicting", "indexed-extra"} {
			t.Run(view+"/"+map[bool]string{false: "configured", true: "empty"}[empty], func(t *testing.T) {
				base := managedDeferredModelBase(view, empty)
				deps := Dependencies{Catalog: config.NewCatalog([]config.Provider{base}, nil, nil)}
				before := deps.Catalog.Providers()
				if err := h.validateExtends(deps, "managed", base.Name, string(base.Type)); err == nil || !strings.Contains(err.Error(), "static-config-only") {
					t.Fatalf("create/update inheritance accepted caller model presence: %v", err)
				}
				for _, enabled := range []bool{false, true} {
					row := store.DBProvider{Name: "managed", Type: string(base.Type), Extends: base.Name, Enabled: enabled, APIKeyEncrypted: []byte("invalid-encrypted-credential")}
					if _, err := h.buildConfigProvider(deps, row, nil); err == nil || !strings.Contains(err.Error(), "static-config-only") || strings.Contains(err.Error(), "decrypt") {
						t.Fatalf("update/credential construction reached credential handling: %v", err)
					}
					if _, err := managedProviderTLSMetadata(row, nil, deps.Catalog); err == nil || !strings.Contains(err.Error(), "static-config-only") {
						t.Fatalf("managed view hid static caller model inheritance: %v", err)
					}
					row.Extends = "middle"
					rows := map[string]store.DBProvider{"middle": {Name: "middle", Extends: base.Name}}
					if _, err := managedProviderTLSMetadata(row, rows, deps.Catalog); err == nil || !strings.Contains(err.Error(), "static-config-only") {
						t.Fatalf("indirect managed view hid static caller model inheritance: %v", err)
					}
					row.Extends = base.Name
					disabled := config.NewCatalog(nil, []config.Provider{base}, nil)
					if _, err := managedProviderTLSMetadata(row, nil, disabled); err == nil || !strings.Contains(err.Error(), "static-config-only") {
						t.Fatalf("disabled-base view hid static caller model inheritance: %v", err)
					}
				}
				if !reflect.DeepEqual(before, deps.Catalog.Providers()) {
					t.Fatal("managed inheritance rejection mutated the static catalog")
				}
			})
		}
	}
	base := managedDeferredModelBase("ordinary", false)
	deps := Dependencies{Catalog: config.NewCatalog([]config.Provider{base}, nil, nil)}
	if err := h.validateExtends(deps, "managed", base.Name, string(base.Type)); err != nil {
		t.Fatalf("ordinary managed inheritance changed: %v", err)
	}
	row := store.DBProvider{Name: "managed", Type: string(base.Type), Extends: base.Name, Enabled: false}
	if p, err := h.buildConfigProvider(deps, row, nil); err != nil || config.HasCallerDeferredTransport(p) {
		t.Fatalf("ordinary managed construction changed: %v", err)
	}
	if _, err := managedProviderTLSMetadata(row, nil, deps.Catalog); err != nil {
		t.Fatalf("ordinary managed view changed: %v", err)
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
			for _, nested := range []bool{false, true} {
				badName := uniqName(t, "deferred-rejected")
				create := validProviderBody(badName)
				create["workspace_id"] = workspace
				update := map[string]any{"api_key": "replacement-key"}
				if nested {
					models := []any{map[string]any{"name": "m", declaration.field: declaration.value}}
					create["models"], update["models"] = models, models
				} else {
					create[declaration.field], update[declaration.field] = declaration.value, declaration.value
				}
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
	if _, err := st.DB.NewUpdate().Model(&before).Column("extends").WherePK().Exec(ctx); err != nil {
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

func TestAdminDeferredModelOptedBaseRejectedBeforeDurableMutation(t *testing.T) {
	st := openValidationStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	for _, view := range []string{"ordered", "indexed-with-nil-ordered", "index-only", "conflicting"} {
		for _, empty := range []bool{false, true} {
			t.Run(view+"/"+map[bool]string{false: "configured", true: "empty"}[empty], func(t *testing.T) {
				base := managedDeferredModelBase(view, empty)
				deps := h.current()
				deps.Catalog = config.NewCatalog([]config.Provider{base}, nil, nil)
				activations := 0
				deps.RequestReload = func() error { activations++; return nil }
				h.UpdateDependencies(deps)
				name := uniqName(t, "deferred-model-inheritance")
				tlsAdminCleanupProvider(t, st, name)
				body := validProviderBody(name)
				body["type"], body["base_url"] = string(base.Type), base.BaseURL
				if w, _ := tlsAdminRequest(t, h, access, http.MethodPost, "/_internal/admin/providers", body); w.Code != http.StatusCreated {
					t.Fatal(w.Body.String())
				}
				before, _ := st.GetProvider(ctx, name)
				beforeModels, _ := st.ListProviderModels(ctx, before.ID)
				activations = 0
				badName := uniqName(t, "deferred-model-rejected")
				tlsAdminCleanupProvider(t, st, badName)
				create := validProviderBody(badName)
				create["type"], create["extends"], create["enabled"] = string(base.Type), base.Name, false
				for _, op := range []struct {
					method string
					path   string
					body   any
				}{
					{http.MethodPost, "/_internal/admin/providers", create},
					{http.MethodPut, "/_internal/admin/providers/" + name, map[string]any{"extends": base.Name, "api_key": "replacement-key"}},
				} {
					w, _ := tlsAdminRequest(t, h, access, op.method, op.path, op.body)
					if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" || !strings.Contains(w.Body.String(), "static-config-only") {
						t.Fatal("managed model-opted inheritance was accepted before commit")
					}
				}
				after, _ := st.GetProvider(ctx, name)
				afterModels, _ := st.ListProviderModels(ctx, before.ID)
				if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeModels, afterModels) || activations != 0 {
					t.Fatal("rejected model-opted inheritance mutated provider, models, credentials, or activation")
				}
				if _, err := st.GetProvider(ctx, badName); !errors.Is(err, sql.ErrNoRows) {
					t.Fatal("rejected model-opted inheritance create committed")
				}
				before.Extends = base.Name
				if _, err := st.DB.NewUpdate().Model(&before).Column("extends").WherePK().Exec(ctx); err != nil {
					t.Fatal(err)
				}
				before, _ = st.GetProvider(ctx, name)
				w, _ := tlsAdminRequest(t, h, access, http.MethodPut, "/_internal/admin/providers/"+name+"/credential", map[string]any{"api_key": "replacement-key"})
				if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" || !strings.Contains(w.Body.String(), "static-config-only") {
					t.Fatal("credential route accepted stored model-opted inheritance")
				}
				after, _ = st.GetProvider(ctx, name)
				if !reflect.DeepEqual(before, after) || activations != 0 {
					t.Fatal("rejected model-opted credential update committed or activated")
				}
			})
		}
	}
}
