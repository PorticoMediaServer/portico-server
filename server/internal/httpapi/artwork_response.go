package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"portico.local/server/internal/metadata"
	"strconv"
	"strings"
)

func artworkWidth(r *http.Request) int {
	if r.URL.Query().Has("w") {
		raw := r.URL.Query().Get("w")
		width, err := strconv.Atoi(raw)
		if err != nil || width < 1 {
			return 0
		}
		switch {
		case width <= 400:
			return 400
		case width <= 800:
			return 800
		default:
			return 1920
		}
	}
	switch r.URL.Query().Get("size") {
	case "thumbnail":
		return 400
	default:
		return 1920
	}
}

// Called only after authorization and a successful open of this exact version.
func serveArtwork(w http.ResponseWriter, r *http.Request, f *os.File, mime string) {
	info, err := f.Stat()
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-cache")
	digest := strings.TrimSuffix(filepath.Base(f.Name()), ".img")
	if len(digest) == 64 && strings.Trim(digest, "0123456789abcdef") == "" {
		w.Header().Set("ETag", `"`+digest+`"`)
		if r.URL.Query().Get("v") != "" && r.URL.Query().Get("candidate") == "" {
			w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		}
	}
	http.ServeContent(w, r, "artwork", info.ModTime(), f)
}

func artworkVersionFailure(w http.ResponseWriter, err error) bool {
	if err != metadata.ErrArtworkVersion {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	policyError(w, "artwork_gone")
	return true
}
