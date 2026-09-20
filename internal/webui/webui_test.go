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

func TestFileHandlerServesIndexAtRoot(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
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

func TestFileHandlerSPAFallback(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/dashboard/providers", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>app</html>" {
		t.Fatalf("body = %q, want SPA fallback to index.html", body)
	}
}

func TestFileHandlerMissingAssetIsNotFound(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/dashboard/assets/stale-bundle.js", nil)
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
	for _, path := range []string{"/dashboard", "/dashboard/", "/dashboard/providers"} {
		if !Matches(path) {
			t.Fatalf("Matches(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"/", "/dashboardx", "/v1/models", "/_internal/dashboard/snapshot"} {
		if Matches(path) {
			t.Fatalf("Matches(%q) = true, want false", path)
		}
	}
}

func TestFileHandlerAssetCaching(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/dashboard/assets/app-abc123.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want immutable", cc)
	}
}

func TestFileHandlerRedirectBarePrefix(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/dashboard/" {
		t.Fatalf("Location = %q, want /dashboard/", loc)
	}
}

func TestFileHandlerMethodNotAllowed(t *testing.T) {
	h := fileHandler(testFS())
	req := httptest.NewRequest(http.MethodPost, "/dashboard/", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/dashboard/anything", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>dev</html>" {
		t.Fatalf("body = %q, want dev index.html fallback", body)
	}
}
