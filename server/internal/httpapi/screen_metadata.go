package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/metadata"
)

func (d Dependencies) screenMetadataRoutes(mux *http.ServeMux) {
	rate := &personalLimiter{}
	owner := func(w http.ResponseWriter, r *http.Request, mutate bool) (identity.Principal, bool) {
		p, err := d.owner(r)
		if err != nil {
			failure(w, err)
			return p, false
		}
		if mutate && !rate.allow(viewerScope(p)) {
			w.Header().Set("Retry-After", "60")
			write(w, 429, map[string]any{"error": map[string]any{"code": "rate_limited", "message": "Too many metadata changes. Try again shortly.", "retryable": true}})
			return p, false
		}
		return p, true
	}
	for _, target := range []struct{ route, kind string }{{"items", "item"}, {"shows", "show"}} {
		kind := target.kind
		base := "/v1/" + target.route + "/{id}/metadata/screen"
		mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
			if _, ok := owner(w, r, false); !ok {
				return
			}
			out, err := d.Metadata.ScreenState(r.Context(), kind, r.PathValue("id"))
			if err != nil {
				failure(w, err)
				return
			}
			if _, err = d.owner(r); err != nil {
				failure(w, err)
				return
			}
			_, _, fence, err := d.homeScope(r)
			if err != nil {
				failure(w, err)
				return
			}
			out.ServerID = d.Identity.ID()
			out.ViewerFence = fence
			write(w, 200, out)
		})
		mux.HandleFunc("PUT "+base, func(w http.ResponseWriter, r *http.Request) {
			p, ok := owner(w, r, true)
			if !ok {
				return
			}
			var body metadata.ScreenSelection
			if err := decodeLegacyRevision(w, r, &body); err != nil {
				failure(w, err)
				return
			}
			body.Actor = metadata.MBActor{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID}
			err := d.Metadata.SelectScreen(r.Context(), kind, r.PathValue("id"), body, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
			if err != nil {
				failure(w, err)
				return
			}
			write(w, 202, map[string]any{"entityId": r.PathValue("id"), "status": "pending"})
		})
		mux.HandleFunc("POST "+base+"/search", func(w http.ResponseWriter, r *http.Request) {
			p, ok := owner(w, r, true)
			if !ok {
				return
			}
			var body metadata.ScreenSearch
			if err := decodeLegacyRevision(w, r, &body); err != nil {
				failure(w, err)
				return
			}
			err := d.Metadata.SearchScreen(r.Context(), kind, r.PathValue("id"), body, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
			if err != nil {
				failure(w, err)
				return
			}
			write(w, 202, map[string]any{"entityId": r.PathValue("id"), "status": "pending"})
		})
		mux.HandleFunc("POST "+base+"/retry", func(w http.ResponseWriter, r *http.Request) {
			p, ok := owner(w, r, true)
			if !ok {
				return
			}
			var body struct {
				ExpectedRevision int64 `json:"expectedRevision"`
			}
			if err := decodeLegacyRevision(w, r, &body); err != nil {
				failure(w, err)
				return
			}
			err := d.Metadata.RetryScreen(r.Context(), kind, r.PathValue("id"), body.ExpectedRevision, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
			if err != nil {
				failure(w, err)
				return
			}
			write(w, 202, map[string]any{"entityId": r.PathValue("id"), "status": "pending"})
		})
	}
	mux.HandleFunc("PUT /v1/shows/{id}/metadata/screen/season", func(w http.ResponseWriter, r *http.Request) {
		p, ok := owner(w, r, true)
		if !ok {
			return
		}
		var body metadata.ScreenSeasonSelection
		if err := decodeLegacyRevision(w, r, &body); err != nil {
			failure(w, err)
			return
		}
		err := d.Metadata.SelectAnimeSeason(r.Context(), r.PathValue("id"), body, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 202, map[string]any{"entityId": r.PathValue("id"), "status": "pending"})
	})
	// A library's metadata source ("agent"): online providers for its media
	// kind, or local metadata only. See metadata/library_agent.go.
	mux.HandleFunc("GET /v1/libraries/{id}/metadata/agent", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := owner(w, r, false); !ok {
			return
		}
		out, err := d.Metadata.LibraryAgent(r.Context(), r.PathValue("id"))
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("PUT /v1/libraries/{id}/metadata/agent", func(w http.ResponseWriter, r *http.Request) {
		p, ok := owner(w, r, true)
		if !ok {
			return
		}
		var body struct {
			ExpectedRevision int64  `json:"expectedRevision"`
			Agent            string `json:"agent"`
		}
		if err := decodeLegacyRevision(w, r, &body); err != nil {
			failure(w, err)
			return
		}
		out, err := d.Metadata.SetLibraryAgent(r.Context(), r.PathValue("id"), body.ExpectedRevision, body.Agent, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 200, out)
	})
	// The choices before a library exists: the agents (with their languages)
	// POST /v1/libraries accepts for this kind. No database read is needed.
	mux.HandleFunc("GET /v1/library-kinds/{kind}/metadata-agents", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := owner(w, r, false); !ok {
			return
		}
		kind := r.PathValue("kind")
		if !metadata.ValidLibraryKind(kind) {
			failure(w, errors.New("invalid library kind"))
			return
		}
		write(w, 200, map[string]any{"libraryKind": kind, "agents": metadata.LibraryAgentOptions(kind)})
	})
	mux.HandleFunc("GET /v1/libraries/{id}/metadata/screen", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := owner(w, r, false); !ok {
			return
		}
		out, err := d.Metadata.ScreenPolicy(r.Context(), r.PathValue("id"))
		if err != nil {
			failure(w, err)
			return
		}
		if _, err = d.owner(r); err != nil {
			failure(w, err)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("PUT /v1/libraries/{id}/metadata/screen", func(w http.ResponseWriter, r *http.Request) {
		p, ok := owner(w, r, true)
		if !ok {
			return
		}
		var body metadata.ScreenPolicyUpdate
		if err := decodeLegacyRevision(w, r, &body); err != nil {
			failure(w, err)
			return
		}
		err := d.Metadata.UpdateScreenPolicy(r.Context(), r.PathValue("id"), body, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 202, map[string]any{"libraryId": r.PathValue("id"), "status": "pending"})
	})
}
