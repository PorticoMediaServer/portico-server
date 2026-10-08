package httpapi

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

var apiNamespace = regexp.MustCompile(`^/(?:v[0-9]+|health)(?:/|$)`)

func (d Dependencies) installationRoutes(mux *http.ServeMux) {
	// Readiness is exposed only after root composition has completed. These public
	// endpoints contain no account, build, route or dependency details.
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, map[string]bool{"alive": true})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store")
		if d.DB == nil || d.DB.PingContext(ctx) != nil {
			write(w, 503, map[string]bool{"ready": false})
			return
		}
		write(w, 200, map[string]bool{"ready": true})
	})
	if d.WebDirectory == "" {
		return
	}
	// A method-free fallback composes with protocol handlers such as DLNA,
	// which accept SUBSCRIBE as well as GET under their own path prefix.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			policyError(w, "method_not_allowed")
			return
		}
		if apiNamespace.MatchString(r.URL.Path) {
			policyError(w, "not_found")
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) || strings.ContainsAny(name, "\\\x00") {
			policyError(w, "not_found")
			return
		}
		// os.Root confines symlinks; no state/config/media directory is mounted here.
		root, e := os.OpenRoot(d.WebDirectory)
		if e != nil {
			policyError(w, "web_unavailable")
			return
		}
		defer root.Close()
		f, e := root.Open(name)
		if e != nil {
			if strings.HasPrefix(name, "assets/") || !strings.Contains(r.Header.Get("Accept"), "text/html") {
				policyError(w, "not_found")
				return
			}
			name = "index.html"
			f, e = root.Open(name)
		}
		if e != nil {
			policyError(w, "not_found")
			return
		}
		defer f.Close()
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() {
			policyError(w, "not_found")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasPrefix(name, "assets/index-") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, path.Base(name), info.ModTime(), f)
	})
}
