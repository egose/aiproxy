package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func newAdminDeps() Dependencies {
	deps := newWebUIDeps(false)
	deps.MultiTenancy = config.MultiTenancy{}
	deps.AdminStore = nil
	return deps
}

func TestAdminStatusWithoutStore(t *testing.T) {
	h := NewHandler(newAdminDeps())
	req := httptest.NewRequest(http.MethodGet, "/_internal/admin/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["multi_tenancy_enabled"] != false {
		t.Fatalf("multi_tenancy_enabled = %v, want false", body["multi_tenancy_enabled"])
	}
	if body["db_ok"] != false {
		t.Fatalf("db_ok = %v, want false", body["db_ok"])
	}
}

func TestAdminLoginWithoutStoreIsNotFound(t *testing.T) {
	h := NewHandler(newAdminDeps())
	req := httptest.NewRequest(http.MethodPost, "/_internal/admin/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when multi_tenancy disabled", w.Code)
	}
}

func TestAdminProvidersRequireAuth(t *testing.T) {
	h := NewHandler(newAdminDeps())
	req := httptest.NewRequest(http.MethodGet, "/_internal/admin/providers", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when multi_tenancy disabled", w.Code)
	}
}
