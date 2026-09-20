package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
)

func newWebUIDeps(enabled bool) Dependencies {
	rt := newRT()
	rt.Listener = config.Listener{Address: ":8080"}
	usage := accounting.NewAggregator()
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(rt.Catalog)
	logs := observability.NewLogBuffer(10)
	deps := newDashboardDeps(rt, time.Now(), usage, health, logs)
	deps.WebUI = config.WebUI{Enabled: enabled}
	return deps
}

func TestWebUIRequiresWebUIEnabled(t *testing.T) {
	h := NewHandler(newWebUIDeps(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when web_ui disabled", w.Code)
	}
}

func TestWebUIIndependentOfDashboardBlock(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>dev-ui</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AIPROXY_WEBUI_DIR", root)

	rt := newRT()
	rt.Listener = config.Listener{Address: ":8080"}
	usage := accounting.NewAggregator()
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(rt.Catalog)
	logs := observability.NewLogBuffer(10)
	deps := newDashboardDeps(rt, time.Now(), usage, health, logs)
	deps.Dashboard = dashrpc.NewRuntimeSource(config.Dashboard{}, "test", ":8080", "none", time.Now(), rt.Catalog, usage, health, logs)
	deps.WebUI = config.WebUI{Enabled: true}

	h := NewHandler(deps)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with web_ui enabled and dashboard disabled", w.Code)
	}
}

func TestWebUIServesFromDevDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>dev-ui</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AIPROXY_WEBUI_DIR", root)

	h := NewHandler(newWebUIDeps(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); body != "<html>dev-ui</html>" {
		t.Fatalf("body = %q, want dev index.html", body)
	}
}

func TestWebUISPAFallbackFromDevDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>dev-ui</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AIPROXY_WEBUI_DIR", root)

	h := NewHandler(newWebUIDeps(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard/providers", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); body != "<html>dev-ui</html>" {
		t.Fatalf("body = %q, want SPA fallback", body)
	}
}

func TestWebUIDevDirWithoutIndexIsNotFound(t *testing.T) {
	t.Setenv("AIPROXY_WEBUI_DIR", t.TempDir())
	h := NewHandler(newWebUIDeps(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for empty web dir", w.Code)
	}
}

func TestWebUIDoesNotShadowAPI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>dev-ui</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AIPROXY_WEBUI_DIR", root)

	h := NewHandler(newWebUIDeps(true))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/_internal/dashboard/snapshot", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 from dashboard API, not web UI", w.Code)
	}
}
