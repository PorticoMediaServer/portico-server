// The account web host serves only the built public assets. API routes belong to Hosted.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func main() {
	dir := flag.String("root", "web", "Built assets directory")
	listen := flag.String("listen", "127.0.0.1:19418", "Loopback listener")
	flag.Parse()
	host, _, e := net.SplitHostPort(*listen)
	if e != nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("loopback listener required")
	}
	root, e := filepath.Abs(*dir)
	if e != nil {
		log.Fatal(e)
	}
	h := webHandler(root, os.Getenv("PORTICO_APPLE_TEAM_ID"))
	s := &http.Server{Addr: *listen, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	log.Fatal(s.ListenAndServe())
}

var appleTeamID = regexp.MustCompile(`^[A-Z0-9]{10}$`)

func webHandler(root, teamID string) http.Handler {
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(405)
			return
		}
		if r.URL.Path == "/.well-known/apple-app-site-association" {
			if !appleTeamID.MatchString(teamID) {
				http.NotFound(w, r)
				return
			}
			app := teamID + ".tv.getportico.portico.ios"
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "public, max-age=3600")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"applinks":             map[string]any{"details": []any{map[string]any{"appIDs": []string{app}, "components": []any{map[string]string{"/": "/account"}, map[string]string{"/": "/device"}, map[string]string{"/": "/join"}, map[string]string{"/": "/together/join"}, map[string]string{"/": "/app/account-return"}}}}},
				"activitycontinuation": map[string]any{"apps": []string{app}},
				"webcredentials":       map[string]any{"apps": []string{app}},
			})
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		rel := strings.TrimPrefix(r.URL.Path, "/")
		if rel == "" {
			rel = "index.html"
		}
		if !filepath.IsLocal(rel) {
			http.NotFound(w, r)
			return
		}
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil || info.IsDir() {
			if !strings.Contains(r.Header.Get("Accept"), "text/html") || strings.HasPrefix(rel, "assets/") {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}
		if strings.HasPrefix(rel, "assets/index-") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fs.ServeHTTP(w, r)
	})
}
