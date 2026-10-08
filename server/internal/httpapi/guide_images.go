package httpapi

import (
	"net/http"

	"portico.local/server/internal/administration"
)

// guideImageRoutes serves stored guide programme images. A programme's image
// is addressed by the digest of its own bytes, so a programme the profile may
// not see is published without its image path, and this read carries no
// programme identity of its own.
func (d Dependencies) guideImageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/guide/images/{digest}", func(w http.ResponseWriter, r *http.Request) {
		if d.Administration == nil {
			administrationFailure(w, administration.ErrUnavailable)
			return
		}
		if _, err := d.principal(r); err != nil {
			administrationFailure(w, err)
			return
		}
		digest := r.PathValue("digest")
		if !validGuideImageDigest(digest) {
			administrationFailure(w, administration.ErrNotFound)
			return
		}
		raw, mime, err := d.Administration.GuideImageBytes(r.Context(), digest)
		if err != nil {
			administrationFailure(w, err)
			return
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		w.Header().Set("ETag", `"`+digest+`"`)
		w.WriteHeader(200)
		_, _ = w.Write(raw)
	})
}

// validGuideImageDigest accepts only the 64 lowercase hex characters of a
// content digest. Anything else is a malformed address, answered 404.
func validGuideImageDigest(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for _, r := range digest {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
