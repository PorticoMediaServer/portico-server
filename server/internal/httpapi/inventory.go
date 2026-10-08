package httpapi

import (
	"database/sql"
	"net/http"
	"portico.local/server/internal/catalog"
	"strconv"
)

func (d Dependencies) inventoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/libraries/{id}/inventory-status", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		library := r.PathValue("id")
		if e = d.Hosted.Allowed(p, library); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.InventoryStatus(r.Context(), library)
		if e != nil {
			failure(w, e)
			return
		}
		p, e = d.principal(r)
		if e == nil {
			e = d.Hosted.Allowed(p, library)
		}
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, map[string]any{"serverId": d.Identity.ID(), "inventory": out})
	})
	mux.HandleFunc("GET /v1/admin/libraries/{id}/inventory-config", func(w http.ResponseWriter, r *http.Request) {
		q, e := d.adminRequest(r, "resource")
		if e != nil {
			failure(w, e)
			return
		}
		l, e := d.Catalog.AdminLibrary(r.Context(), q, false)
		if e != nil {
			failure(w, e)
			return
		}
		sources, e := d.Catalog.LibrarySources(r.Context(), q.LibraryID)
		if e != nil {
			failure(w, e)
			return
		}
		policy, e := d.Catalog.ScanPolicy(r.Context(), q.LibraryID)
		if e != nil {
			failure(w, e)
			return
		}
		// Detect a configuration mutation between the component reads.
		after, e := d.Catalog.AdminLibrary(r.Context(), q, false)
		if e == nil && after.Revision != l.Revision {
			e = catalog.ErrStaleContinuation
		}
		if e != nil {
			failure(w, e)
			return
		}
		if _, e = d.owner(r); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, map[string]any{"scope": catalog.AdminScope{ServerID: q.ServerID, ViewerFence: q.ViewerFence}, "libraryId": l.ID, "revision": l.Revision, "sources": sources, "policy": policy})
	})
	mux.HandleFunc("GET /v1/admin/libraries/{id}/inventory", func(w http.ResponseWriter, r *http.Request) {
		// Reuse owner/viewer fencing but parse this endpoint's exact query shape.
		clone := r.Clone(r.Context())
		u := *r.URL
		clone.URL = &u
		clone.URL.RawQuery = ""
		q, e := d.adminRequest(clone, "resource")
		if e != nil {
			failure(w, e)
			return
		}
		source, state := "", ""
		for k, v := range r.URL.Query() {
			if len(v) != 1 {
				failure(w, catalog.ErrAdminQuery)
				return
			}
			switch k {
			case "sourceId":
				source = v[0]
			case "state":
				state = v[0]
			case "cursor":
				q.Cursor = v[0]
			case "limit":
				q.Limit, e = strconv.Atoi(v[0])
			default:
				e = catalog.ErrAdminQuery
			}
			if e != nil {
				failure(w, e)
				return
			}
		}
		out, e := d.Catalog.Inventory(r.Context(), q, source, state)
		if e != nil {
			failure(w, e)
			return
		}
		if _, e = d.owner(r); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/admin/libraries/{id}/sources/{source}/changes", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.owner(r); e != nil {
			failure(w, e)
			return
		}
		for k, v := range r.URL.Query() {
			if k != "after" || len(v) != 1 {
				failure(w, catalog.ErrAdminQuery)
				return
			}
		}
		after, e := catalog.ParseInventorySequence(r.URL.Query().Get("after"))
		if e != nil {
			failure(w, e)
			return
		}
		out, next, e := d.Catalog.InventoryChanges(r.Context(), r.PathValue("id"), r.PathValue("source"), after)
		if e != nil {
			failure(w, e)
			return
		}
		if _, e = d.owner(r); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, map[string]any{"sourceId": r.PathValue("source"), "items": out, "nextSequence": next, "hasMore": len(out) == 128})
	})
	for _, pattern := range []string{"POST /v1/admin/libraries/{id}/sources", "PATCH /v1/admin/libraries/{id}/sources/{source}"} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			p, e := d.owner(r)
			if e != nil {
				failure(w, e)
				return
			}
			var body catalog.SourceSettings
			if e = decodeLegacyRevision(w, r, &body); e != nil {
				failure(w, e)
				return
			}
			out, e := d.Catalog.SaveSource(r.Context(), r.PathValue("id"), r.PathValue("source"), body, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
			if e != nil {
				failure(w, e)
				return
			}
			write(w, 200, out)
		})
	}
	mux.HandleFunc("POST /v1/admin/libraries/{id}/sources/{source}/check", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.CheckSource(r.Context(), r.PathValue("id"), r.PathValue("source"), func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("DELETE /v1/admin/libraries/{id}/sources/{source}", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			ExpectedRevision int64 `json:"expectedRevision"`
		}
		if e = decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		if e = d.Catalog.RemoveSource(r.Context(), r.PathValue("id"), r.PathValue("source"), body.ExpectedRevision, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") }); e != nil {
			failure(w, e)
			return
		}
		d.Ingestion.InterruptSource(r.Context(), r.PathValue("source"))
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /v1/admin/libraries/{id}/sources/{source}/scans", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Ingestion.QueueMode(r.Context(), r.PathValue("id"), r.PathValue("source"), r.URL.Query().Get("mode"), func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 202, out)
	})
	mux.HandleFunc("PUT /v1/admin/libraries/{id}/scan-policy", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			Tier             string   `json:"tier"`
			Operations       []string `json:"operations"`
			ExpectedRevision int64    `json:"expectedRevision"`
		}
		if e = decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.UpdateScanPolicy(r.Context(), r.PathValue("id"), catalog.ScanPolicy{Tier: body.Tier, Operations: body.Operations}, body.ExpectedRevision, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if e != nil {
			failure(w, e)
			return
		}
		if e = d.Ingestion.PolicyChanged(r.Context(), r.PathValue("id")); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/admin/ingestion/jobs/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		authorize := func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") }
		action := r.PathValue("action")
		if action == "retry" {
			out, e := d.Ingestion.Retry(r.Context(), r.PathValue("id"), authorize)
			if e != nil {
				failure(w, e)
				return
			}
			write(w, 202, out)
			return
		}
		if action != "pause" && action != "resume" && action != "cancel" {
			failure(w, catalog.ErrAdminQuery)
			return
		}
		if e = d.Ingestion.Control(r.Context(), r.PathValue("id"), action, authorize); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /v1/admin/libraries/{id}/inventory/{object}/{action}", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			Revision string `json:"revision"`
			ItemID   string `json:"itemId"`
		}
		if e = decodeLegacyRevision(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		authorize := func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") }
		if r.PathValue("action") == "associate" {
			e = d.Catalog.AssociateInventoryVersion(r.Context(), r.PathValue("id"), r.PathValue("object"), body.Revision, body.ItemID, authorize)
		} else {
			e = d.Catalog.TrashInventory(r.Context(), r.PathValue("id"), r.PathValue("object"), body.Revision, r.PathValue("action"), authorize)
		}
		if e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	})
}
