package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"strconv"
	"time"
)

func (d Dependencies) adminRequest(r *http.Request, kind string) (catalog.AdminRequest, error) {
	p, e := d.owner(r)
	if e != nil {
		return catalog.AdminRequest{}, e
	}
	out := catalog.AdminRequest{ServerID: d.Identity.ID(), Profile: identity.PersonalKey(p.Viewer), ViewerFence: fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", p.Hash, p.Epoch)))), Limit: 40, LibraryID: r.PathValue("id")}
	for key, values := range r.URL.Query() {
		if len(values) != 1 {
			return out, catalog.ErrAdminQuery
		}
		switch key {
		case "limit":
			if kind == "resource" {
				return out, catalog.ErrAdminQuery
			}
			out.Limit, e = strconv.Atoi(values[0])
			if e != nil || out.Limit < 1 || out.Limit > 40 {
				return out, catalog.ErrAdminQuery
			}
		case "cursor":
			if kind == "resource" {
				return out, catalog.ErrAdminQuery
			}
			out.Cursor = values[0]
		case "libraryId":
			if kind != "jobs" {
				return out, catalog.ErrAdminQuery
			}
			out.LibraryID = values[0]
		case "status":
			if kind != "jobs" {
				return out, catalog.ErrAdminQuery
			}
			out.Status = values[0]
		default:
			return out, catalog.ErrAdminQuery
		}
	}
	return out, nil
}
func (d Dependencies) adminRoutes(mux *http.ServeMux) {
	for _, kind := range []string{"libraries", "jobs", "resource"} {
		route := "GET /v1/admin/" + kind
		if kind == "resource" {
			route = "GET /v1/admin/libraries/{id}"
		}
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			q, e := d.adminRequest(r, kind)
			if e != nil {
				failure(w, e)
				return
			}
			var out any
			switch kind {
			case "libraries":
				out, e = d.Catalog.AdminLibraries(ctx, q)
			case "jobs":
				out, e = d.Catalog.AdminJobs(ctx, q)
			case "resource":
				var l catalog.AdminLibrary
				l, e = d.Catalog.AdminLibrary(ctx, q, true)
				out = map[string]any{"scope": catalog.AdminScope{ServerID: q.ServerID, ViewerFence: q.ViewerFence}, "library": l}
			}
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
	}
	// The owner's order of the libraries, as every client lists them (GET /v1/libraries).
	mux.HandleFunc("PUT /v1/admin/libraries/order", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			LibraryIDs []string `json:"libraryIds"`
		}
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		if e = d.Catalog.OrderLibraries(r.Context(), body.LibraryIDs, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") }); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, map[string]any{"libraryIds": body.LibraryIDs})
	})
	// How one show's episodes are presented (Spec — Title Pages §4): the owner's, from the show's page.
	mux.HandleFunc("PUT /v1/admin/libraries/{id}/shows/{show}/settings", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body catalog.ShowSettings
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.SetShowSettings(r.Context(), r.PathValue("id"), r.PathValue("show"), body, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, r.PathValue("id")) })
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("PATCH /v1/admin/libraries/{id}", func(w http.ResponseWriter, r *http.Request) {
		q, e := d.adminRequest(r, "resource")
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			Name             string `json:"name"`
			ExpectedRevision int64  `json:"expectedRevision"`
		}
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		if e = d.Catalog.RenameLibrary(r.Context(), q.LibraryID, body.Name, body.ExpectedRevision, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") }); e != nil {
			if errors.Is(e, catalog.ErrLibraryConfigurationConflict) {
				write(w, 409, map[string]any{"error": map[string]any{"code": "library_configuration_conflict", "message": publicErrorMessage("library_configuration_conflict"), "retryable": false}})
				return
			}
			failure(w, e)
			return
		}
		l, e := d.Catalog.AdminLibrary(r.Context(), q, false)
		if e != nil {
			failure(w, e)
			return
		}
		if _, e = d.owner(r); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, map[string]any{"scope": catalog.AdminScope{ServerID: q.ServerID, ViewerFence: q.ViewerFence}, "library": l})
	})
}
