package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/metadata"
	"strconv"
)

// peopleRoutes carries the person pages, bulk personal state and the library
// collection membership batch. Each of these is one request per screen action:
// no client-side fan-out, no per-row round trip.
func (d Dependencies) peopleRoutes(mux *http.ServeMux) {
	personalRate := &personalLimiter{}
	mux.HandleFunc("GET /v1/people", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, values := range q {
			if len(values) != 1 || (key != "q" && key != "limit") {
				failure(w, catalog.ErrPersonQuery)
				return
			}
		}
		limit := 25
		if q.Has("limit") {
			var e error
			limit, e = strconv.Atoi(q.Get("limit"))
			if e != nil {
				failure(w, catalog.ErrPersonQuery)
				return
			}
		}
		p, libraries, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		viewer, e := d.catalogViewer(r, p, libraries, fence)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.WithContext(r.Context()).People(viewer, d.Identity.ID(), q.Get("q"), limit)
		if e != nil {
			failure(w, e)
			return
		}
		d.savedResponse(w, r, fence, out)
	})
	mux.HandleFunc("GET /v1/people/{id}", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, values := range q {
			if len(values) != 1 || (key != "cursor" && key != "limit" && key != "role") {
				failure(w, catalog.ErrPersonQuery)
				return
			}
		}
		limit := 40
		if q.Has("limit") {
			var e error
			limit, e = strconv.Atoi(q.Get("limit"))
			if e != nil {
				failure(w, catalog.ErrPersonQuery)
				return
			}
		}
		p, libraries, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		viewer, e := d.catalogViewer(r, p, libraries, fence)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.WithContext(r.Context()).Person(viewer, d.Identity.ID(), r.PathValue("id"), q.Get("role"), q.Get("cursor"), limit)
		if e != nil {
			failure(w, e)
			return
		}
		d.savedResponse(w, r, fence, out)
	})
	mux.HandleFunc("GET /v1/people/{id}/portrait", func(w http.ResponseWriter, r *http.Request) {
		p, err := d.principal(r)
		if err != nil {
			failure(w, err)
			return
		}
		if d.Metadata == nil {
			policyError(w, "metadata_unavailable")
			return
		}
		_, libraries, fence, err := d.homeScope(r)
		if err != nil {
			failure(w, err)
			return
		}
		viewer, err := d.catalogViewer(r, p, libraries, fence)
		if err != nil {
			failure(w, err)
			return
		}
		item, subject, err := d.Catalog.WithContext(r.Context()).PersonPortrait(viewer, r.PathValue("id"))
		if err != nil {
			failure(w, err)
			return
		}
		// Re-check the owning item, so a library revoked between the page read
		// and the image read cannot still serve its artwork.
		if err = d.itemAccess(r.Context(), p, item); err != nil {
			failure(w, err)
			return
		}
		f, mime, err := d.Metadata.EntityArtworkVariant(r.Context(), metadata.RepairTarget{Kind: "item", ID: item}, "portrait", subject, artworkWidth(r), r.URL.Query().Get("v"))
		if err != nil {
			if artworkVersionFailure(w, err) {
				return
			}
			if errors.Is(err, metadata.ErrArtworkPending) {
				w.Header().Set("Retry-After", "5")
				w.Header().Set("Cache-Control", "no-store")
				policyError(w, "artwork_pending")
				return
			}
			failure(w, err)
			return
		}
		defer f.Close()
		serveArtwork(w, r, f, mime)
	})
	mux.HandleFunc("PUT /v1/items/personal-state:batch", func(w http.ResponseWriter, r *http.Request) {
		p, libraries, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		if !personalRate.allow(viewerScope(p)) {
			w.Header().Set("Retry-After", "60")
			write(w, 429, map[string]any{"error": map[string]any{"code": "rate_limited", "message": "Too many personal-state requests. Try again shortly.", "retryable": true}})
			return
		}
		var body catalog.PersonalBatchMutation
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		allowed := map[string]bool{}
		for _, id := range libraries {
			allowed[id] = true
		}
		out, e := d.Catalog.SetPersonalBatch(p.Authority+":"+p.AccountID, identity.PersonalKey(p.Viewer), body, func(tx *sql.Tx, item string) error {
			var library, kind string
			var playable bool
			if err := tx.QueryRow(`SELECT cl.library_id,k.name,k.playable FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id JOIN catalog_kinds k ON k.id=e.kind WHERE e.public_id=pid_blob(?)`, item).Scan(&library, &kind, &playable); err != nil {
				return err
			}
			// SEC-02: a title the profile may not see (another library, or over
			// its rating ceiling or with a blocked label) is refused exactly as an
			// absent one, and nothing is written for it. The library check alone
			// let a restricted profile write personal state for hidden titles.
			if !allowed[library] {
				return sql.ErrNoRows
			}
			// A show, album or book (only ever "not interested", which the
			// catalogue enforces per field) is checked as the container it is.
			if !playable {
				viewer, err := d.catalogViewer(r, p, libraries, fence)
				if err != nil {
					return err
				}
				if err = d.Catalog.VisibleEntity(r.Context(), viewer, kind, item); err != nil {
					return err
				}
				return d.resourceAuthorize(tx, p, library)
			}
			if err := d.restrictedItem(r, p, item); err != nil {
				if errors.Is(err, identity.ErrContentRestricted) || errors.Is(err, identity.ErrNotVisible) || errors.Is(err, sql.ErrNoRows) {
					return sql.ErrNoRows
				}
				return err
			}
			return d.resourceAuthorize(tx, p, library)
		})
		if e != nil {
			failure(w, e)
			return
		}
		out.ServerID = d.Identity.ID()
		out.ViewerFence = fence
		d.savedResponse(w, r, fence, out)
	})
	mux.HandleFunc("POST /v1/collections/{id}/items:batch", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.owner(r); e != nil {
			failure(w, e)
			return
		}
		var body struct {
			AddItemIDs    []string `json:"addItemIds"`
			RemoveItemIDs []string `json:"removeItemIds"`
		}
		if e := decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.SetCollectionItems(r.PathValue("id"), body.AddItemIDs, body.RemoveItemIDs)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
}
