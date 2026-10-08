package httpapi

import "net/http"

// Queue commit returns the existing delivery session, so its observations enter
// these same endpoints. Live/Library occurrences use their separate v2 channel
// observation endpoint and never enter personal state or finite queue completion.
func (d Dependencies) playbackSessionRoutes(mux *http.ServeMux) {
	if d.Playback == nil {
		return
	}
	d.configurePlaybackEvidence()
	mux.HandleFunc("POST /v1/playback/sessions", d.v1OrLegacy(d.isV1Start, func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			ItemID    string `json:"itemId"`
			Quality   string `json:"quality"`
			RequestID string `json:"requestId"`
		}
		if e = decode(w, r, &body); e == nil {
			e = d.itemAccess(r.Context(), p, body.ItemID)
		}
		if e == nil {
			// Playback is refused with the same code a list read uses, so a
			// restricted title is one explanation everywhere it is reachable.
			e = d.restrictedItem(r, p, body.ItemID)
		}
		if e != nil {
			failure(w, e)
			return
		}
		// Per-member limits (streams, schedule, rating, labels, device trust)
		// are applied before a lease exists. See access_admission.go.
		if _, e = d.admitPlayback(r, p, body.ItemID); e != nil {
			accessFailure(w, e)
			return
		}
		out, e := d.Playback.CreateContext(r.Context(), p, body.ItemID, body.Quality, body.RequestID)
		if e == nil {
			e = d.Playback.Ready(r.Context(), out)
		}
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 201, out)
	}))
	mux.HandleFunc("GET /v1/playback/sessions/{id}", d.v1OrLegacy(d.isV1Session, func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Playback.Get(p, r.PathValue("id"))
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
	mux.HandleFunc("POST /v1/playback/sessions/{id}/progress", func(w http.ResponseWriter, r *http.Request) {
		markDeprecated(w)
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			Generation      int     `json:"generation"`
			Sequence        int64   `json:"sequence"`
			PositionSeconds float64 `json:"positionSeconds"`
			State           string  `json:"state"`
		}
		if e = decode(w, r, &body); e == nil {
			e = d.Playback.Progress(p, r.PathValue("id"), body.Generation, body.Sequence, body.PositionSeconds, body.State)
		}
		if e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /v1/playback/sessions/{id}", d.v1OrLegacy(d.isV1Session, func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e == nil {
			e = d.Playback.Stop(p, r.PathValue("id"))
		}
		if e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	}))
}
