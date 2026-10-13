package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"portico.local/server/internal/administration"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/livechannels"
)

// Images retain programme identity so knowing an old URL never substitutes for
// current source access or the programme's rating restriction.
func (d Dependencies) guideImageRoutes(mux *http.ServeMux) {
	d.httpHandle(mux, "GET /v1/guide/programme-images/{generation}/{programme}", func(w http.ResponseWriter, r *http.Request) {
		if d.Administration == nil {
			administrationFailure(w, administration.ErrUnavailable)
			return
		}
		digest, err := d.guideImageDigest(r)
		if err != nil {
			administrationFailure(w, err)
			return
		}
		raw, mime, err := d.Administration.GuideImageBytes(r.Context(), digest)
		if err != nil {
			administrationFailure(w, err)
			return
		}
		// A rule or publication may change while the file is read. Recheck before
		// returning bytes, including on conditional requests.
		current, err := d.guideImageDigest(r)
		if err != nil {
			administrationFailure(w, err)
			return
		}
		if current != digest {
			administrationFailure(w, administration.ErrNotFound)
			return
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("ETag", `"`+digest+`"`)
		w.WriteHeader(200)
		_, _ = w.Write(raw)
	})
}

func (d Dependencies) guideImageDigest(r *http.Request) (string, error) {
	p, err := d.strictPrincipalFor(r)
	if err != nil {
		return "", err
	}
	generation, programme := r.PathValue("generation"), r.PathValue("programme")
	if len(generation) > 128 || len(programme) > 128 {
		return "", administration.ErrNotFound
	}
	restrictions, _, err := d.viewerRestrictions(r.WithContext(dbwork.WithClass(r.Context(), dbwork.ClassSecurityFence)), p)
	if err != nil {
		return "", err
	}
	gate, err := dbwork.BeginSnapshot(r.Context(), d.DB)
	if err != nil {
		return "", err
	}
	defer gate.Rollback()
	_, allowed, err := d.liveAuthority(p)(r.Context(), gate.Tx(), false)
	if err != nil {
		return "", err
	}
	var source, channel, digest, facts string
	err = gate.Tx().QueryRowContext(r.Context(), `SELECT s.id,p.channel_id,img.digest,COALESCE(m.facts,'{}')
 FROM live_programmes p JOIN live_generations g ON g.id=p.generation_id
 JOIN live_sources s ON s.id=g.source_id AND s.active_generation=g.id AND s.state='active'
 JOIN live_programme_icons i ON i.generation_id=p.generation_id AND i.programme_id=p.id
 JOIN live_programme_images img ON img.url=i.url
 LEFT JOIN live_programme_metadata m ON m.generation_id=p.generation_id AND m.id=p.id
 WHERE p.generation_id=? AND p.id=?`, generation, programme).Scan(&source, &channel, &digest, &facts)
	if errors.Is(err, sql.ErrNoRows) {
		return "", administration.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if allowed == nil || !allowed(source, channel) || !validGuideImageDigest(digest) {
		return "", administration.ErrNotFound
	}
	var item livechannels.Programme
	item.ApplyFacts(facts)
	rating := ""
	if item.Rating != nil {
		rating = item.Rating.Value
	}
	if restrictions.Active() && !catalog.RatingAllowed(restrictions, rating) {
		return "", administration.ErrNotFound
	}
	return digest, nil
}

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
