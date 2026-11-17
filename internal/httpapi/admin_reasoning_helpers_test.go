package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type adminReasoningFixture struct {
	store       *store.Store
	handler     *Handler
	access      string
	runtime     *config.Runtime
	activations int
}

func openAdminReasoningStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	t.Setenv("AIPROXY_JWT_SECRET", "reasoning-admin-fixture")
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{19}, 32)))
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "reasoning_admin_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.DB.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.DB.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	st, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureSystemWorkspace(ctx); err != nil {
		t.Fatal(err)
	}
	return st
}

func newAdminReasoningFixture(t *testing.T, rt *config.Runtime) *adminReasoningFixture {
	t.Helper()
	st := openAdminReasoningStore(t)
	h, access := validationTestHandler(t, st)
	if rt == nil {
		rt = newRT()
	}
	fixture := &adminReasoningFixture{store: st, handler: h, access: access, runtime: rt}
	deps := h.current()
	deps.Catalog = rt.Catalog
	deps.Resolver = modelresolver.New(rt)
	deps.HasAccessLog, deps.AccessLog = true, false
	deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	deps.RequestReload = fixture.reload
	h.UpdateDependencies(deps)
	return fixture
}

func (fixture *adminReasoningFixture) reload() error {
	fixture.activations++
	merged, err := dbmerge.MergeCatalog(context.Background(), fixture.store, fixture.runtime)
	if err != nil {
		return err
	}
	next := *fixture.runtime
	next.Catalog = config.NewCatalog(merged.Providers, merged.DisabledProviders, merged.Aliases)
	deps := fixture.handler.current()
	deps.Catalog = next.Catalog
	deps.Resolver = modelresolver.New(&next)
	fixture.handler.UpdateDependencies(deps)
	return nil
}

func adminReasoningPolicy(t *testing.T, raw string) *config.EncryptedReasoning {
	t.Helper()
	policy, err := config.DecodeEncryptedReasoningJSON([]byte(raw), "test policy")
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func adminReasoningRequest(t *testing.T, h *Handler, access, method, path, raw string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+access)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	view := make(map[string]any)
	_ = json.Unmarshal(w.Body.Bytes(), &view)
	return w, view
}

func requireAdminReasoningStatus(t *testing.T, fixture *adminReasoningFixture, method, path, raw string, status int) map[string]any {
	t.Helper()
	w, view := adminReasoningRequest(t, fixture.handler, fixture.access, method, path, raw)
	if w.Code != status {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, w.Code, status, w.Body.String())
	}
	if strings.Contains(path, "/providers") && method != "GET" && status < 300 && w.Header().Get(catalogSavedHeader) != "true" {
		t.Fatal("successful provider mutation did not retain the durability header")
	}
	return view
}

func adminReasoningModelView(t *testing.T, provider map[string]any, name string) map[string]any {
	t.Helper()
	models, ok := provider["models"].([]any)
	if !ok {
		t.Fatalf("models are not an inventory array: %#v", provider)
	}
	for _, entry := range models {
		model, ok := entry.(map[string]any)
		if ok && model["name"] == name {
			return model
		}
	}
	t.Fatalf("model %q absent from inventory: %#v", name, models)
	return nil
}

func assertAdminReasoningView(t *testing.T, view map[string]any, local, effective, source string, inherited bool) {
	t.Helper()
	var declaration any
	if err := json.Unmarshal([]byte(local), &declaration); err != nil {
		t.Fatal(err)
	}
	got, present := view["encrypted_reasoning"]
	if !present || !reflect.DeepEqual(got, declaration) {
		t.Fatalf("local intent: got=%#v present=%v want=%s", got, present, local)
	}
	policy := adminReasoningPolicy(t, effective)
	if policy == nil {
		defaults := config.DefaultEncryptedReasoning()
		policy = &defaults
	}
	raw, err := json.Marshal(adminReasoningEffective(*policy))
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view["encrypted_reasoning_effective"], want) || view["encrypted_reasoning_source"] != source || view["encrypted_reasoning_inherited"] != inherited {
		t.Fatalf("effective metadata: got=%#v want=%s/%s/%v", view, raw, source, inherited)
	}
}

func assertAdminReasoningStored(t *testing.T, raw json.RawMessage, expected string) {
	t.Helper()
	var actual, want any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &actual); err != nil {
			t.Fatal(err)
		}
	}
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, want) || expected == "null" && len(raw) != 0 {
		t.Fatalf("stored local intent: got=%s want=%s", raw, expected)
	}
}

func adminReasoningSavedProvider(t *testing.T, st *store.Store, name string) (store.DBProvider, []store.DBProviderModel) {
	t.Helper()
	provider, err := st.GetProvider(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	models, err := st.ListProviderModels(context.Background(), provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	return provider, models
}

func assertAdminReasoningProviderUnchanged(t *testing.T, st *store.Store, before store.DBProvider, models []store.DBProviderModel) {
	t.Helper()
	after, afterModels := adminReasoningSavedProvider(t, st, before.Name)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(models, afterModels) {
		t.Fatal("rejected mutation changed saved provider, credentials, model rows, policies, or timestamps")
	}
}
