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

const RoutePrefix = "/dashboard"

func Matches(path string) bool {
	return path == RoutePrefix || strings.HasPrefix(path, RoutePrefix+"/")
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

func DirHandler(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r) {
			return
		}
		upath := strings.TrimPrefix(r.URL.Path, RoutePrefix)
		if upath == "" {
			http.Redirect(w, r, RoutePrefix+"/", http.StatusFound)
			return
		}
		name := resolveDiskName(http.Dir(root), strings.TrimPrefix(upath, "/"))
		if name == "" {
			http.NotFound(w, r)
			return
		}
		setCacheHeaders(w, name)
		http.ServeFile(w, r, string(http.Dir(root))+string(os.PathSeparator)+filepathFromSlash(name))
	})
}

func fileHandler(content fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r) {
			return
		}
		upath := strings.TrimPrefix(r.URL.Path, RoutePrefix)
		if upath == "" {
			http.Redirect(w, r, RoutePrefix+"/", http.StatusFound)
			return
		}
		serveFS(w, r, content, strings.TrimPrefix(upath, "/"))
	})
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
	if path.Ext(cleaned) != "" {
		return "", false
	}
	return "index.html", true
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
		if path.Ext(cleaned) != "" {
			return ""
		}
		return "index.html"
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		if path.Ext(cleaned) != "" {
			return ""
		}
		return "index.html"
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
