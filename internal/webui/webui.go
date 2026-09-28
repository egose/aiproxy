package webui

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

//go:embed all:dist
var embeddedDist embed.FS

var reservedPaths = []string{"/v1", "/healthz", "/readyz", "/metrics", "/_internal", "/dashboard"}

func reserved(path string) bool {
	for _, r := range reservedPaths {
		if path == r || strings.HasPrefix(path, r+"/") {
			return true
		}
	}
	return false
}

func Matches(path string) bool {
	if path == "/" {
		return true
	}
	if reserved(path) {
		return false
	}
	return true
}

func WantsHTML(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		media := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		if media == "text/html" || media == "application/xhtml+xml" {
			return true
		}
	}
	return false
}

func distSub() fs.FS {
	sub, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		return nil
	}
	return sub
}

func Built() bool {
	sub := distSub()
	if sub == nil {
		return false
	}
	f, err := sub.Open("index.html")
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func Dir() string {
	return os.Getenv("AIPROXY_WEBUI_DIR")
}

func Handler() http.Handler {
	if dir := Dir(); dir != "" {
		return DirHandler(dir)
	}
	if !Built() {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"dashboard web UI not built"}`))
		})
	}
	return fileHandler(distSub())
}

func TryServe(w http.ResponseWriter, r *http.Request) bool {
	if dir := Dir(); dir != "" {
		return serveDir(w, r, dir)
	}
	if !Built() {
		return false
	}
	return serveFSContent(w, r, distSub())
}

func DirHandler(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !serveDir(w, r, root) {
			http.NotFound(w, r)
		}
	})
}

func serveDir(w http.ResponseWriter, r *http.Request, root string) bool {
	if !allowMethod(w, r) {
		return true
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	resolved := resolveDiskName(http.Dir(root), name)
	if resolved == "" {
		if path.Ext(name) == "" && name != "" && WantsHTML(r) {
			resolved = "index.html"
		} else {
			return false
		}
	}
	setCacheHeaders(w, resolved)
	http.ServeFile(w, r, root+string(os.PathSeparator)+filepathFromSlash(resolved))
	return true
}

func fileHandler(content fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !serveFSContent(w, r, content) {
			http.NotFound(w, r)
		}
	})
}

func serveFSContent(w http.ResponseWriter, r *http.Request, content fs.FS) bool {
	if !allowMethod(w, r) {
		return true
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	resolved, found := resolveFSName(content, name)
	if !found {
		if name != "" && path.Ext(name) == "" && WantsHTML(r) {
			resolved, found = "index.html", true
		} else {
			return false
		}
	}
	serveFS(w, r, content, resolved)
	return true
}

func allowMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func serveFS(w http.ResponseWriter, r *http.Request, content fs.FS, name string) {
	resolved, found := resolveFSName(content, name)
	if !found {
		http.NotFound(w, r)
		return
	}
	f, err := content.Open(resolved)
	if err != nil {
		http.Error(w, "dashboard web UI not built", http.StatusNotFound)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "dashboard web UI not built", http.StatusNotFound)
		return
	}
	setCacheHeaders(w, resolved)
	if seeker, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, resolved, stat.ModTime(), seeker)
		return
	}
	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "dashboard web UI not built", http.StatusNotFound)
		return
	}
	http.ServeContent(w, r, resolved, stat.ModTime(), bytes.NewReader(data))
}

func resolveFSName(content fs.FS, name string) (string, bool) {
	if name == "" {
		return "index.html", true
	}
	cleaned := path.Clean("/" + name)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if isFile(content, cleaned) {
		return cleaned, true
	}
	return "", false
}

func isFile(content fs.FS, name string) bool {
	f, err := content.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		return false
	}
	return true
}

func resolveDiskName(root http.FileSystem, name string) string {
	if name == "" {
		return "index.html"
	}
	cleaned := path.Clean("/" + name)
	cleaned = strings.TrimPrefix(cleaned, "/")
	f, err := root.Open("/" + cleaned)
	if err != nil {
		return ""
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		return ""
	}
	return cleaned
}

func filepathFromSlash(name string) string {
	return strings.ReplaceAll(name, "/", string(os.PathSeparator))
}

func setCacheHeaders(w http.ResponseWriter, name string) {
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	if strings.HasSuffix(name, ".html") {
		w.Header().Set("Cache-Control", "no-cache")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
}
