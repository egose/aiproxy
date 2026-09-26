package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<html>app</html>")},
		"assets/app-abc123.js": {Data: []byte("console.log(1)")},
		"favicon.svg":          {Data: []byte("<svg></svg>")},
	}
}

func htmlRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	return req
}

func TestFileHandlerServesIndexAtRoot(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Fatalf("body = %q, want index.html", body)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", cc)
	}
}

func TestFileHandlerSPAFallbackForBrowserNavigation(t *testing.T) {
	h := fileHandler(testFS())
	req := htmlRequest(http.MethodGet, "/providers")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Fatalf("body = %q, want SPA fallback to index.html", body)
	}
}

func TestFileHandlerNoFallbackForAPIClients(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/providers", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for non-HTML client", rec.Code)
	}
}

func TestFileHandlerMissingAssetIsNotFound(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/assets/stale-bundle.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for missing asset", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, must not serve HTML fallback for assets", ct)
	}
}

func TestMatches(t *testing.T) {
	for _, path := range []string{"/", "/assets/app.js", "/providers", "/admin/keys", "/favicon.svg", "/dashboardx"} {
		if !Matches(path) {
			t.Fatalf("Matches(%q) = false, want true", path)
		}
	}
	for _, path := range []string{
		"/v1/models", "/v1/chat/completions", "/healthz", "/readyz", "/metrics",
		"/_internal/dashboard/snapshot", "/_internal/admin/status",
		"/dashboard", "/dashboard/", "/dashboard/providers",
	} {
		if Matches(path) {
			t.Fatalf("Matches(%q) = true, want false", path)
		}
	}
}

func TestWantsHTML(t *testing.T) {
	browser := htmlRequest(http.MethodGet, "/")
	if !WantsHTML(browser) {
		t.Fatalf("WantsHTML(browser) = false, want true")
	}
	plain := httptest.NewRequest(http.MethodGet, "/", nil)
	if WantsHTML(plain) {
		t.Fatalf("WantsHTML(no accept) = true, want false")
	}
	jsonReq := httptest.NewRequest(http.MethodGet, "/", nil)
	jsonReq.Header.Set("Accept", "application/json")
	if WantsHTML(jsonReq) {
		t.Fatalf("WantsHTML(json) = true, want false")
	}
}

func TestFileHandlerAssetCaching(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want immutable", cc)
	}
}

func TestFileHandlerServesFavicon(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestFileHandlerMethodNotAllowed(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestNotBuiltWithoutIndex(t *testing.T) {
	if Built() {
		t.Skip("web UI is built in this checkout; stub assertion only applies pre-build")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDirHandlerServesFromDisk(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>dev</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := DirHandler(root)
	req := htmlRequest(http.MethodGet, "/anything")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>dev</html>" {
		t.Fatalf("body = %q, want dev index.html fallback", body)
	}
}
