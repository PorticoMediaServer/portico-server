package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"

	"portico.local/apikit"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
)

func registerFileInfo(registry *apikit.Registry, d Dependencies) {
	registerDiscTitles(registry, d)
	v1Route(registry, apikit.Metadata{ID: "get_items_id_file_info", Method: "GET", Path: "/v1/items/{id}/file-info", Summary: "Read authorized file information lazily", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "stale_continuation", "internal"}}, func(ctx context.Context, r *http.Request, _ struct{}) (catalog.FileInfo, error) {
		out := catalog.FileInfo{}
		if err := homeAllowedQuery(r, "cursor", "limit"); err != nil {
			return out, err
		}
		limit := 0
		if text := r.URL.Query().Get("limit"); text != "" {
			var err error
			limit, err = strconv.Atoi(text)
			if err != nil || limit < 1 || limit > 200 {
				return out, catalog.ErrCursor
			}
		}
		p, err := d.principal(r)
		if err != nil {
			return out, err
		}
		err = dbwork.WithReadSnapshot(ctx, d.DB, func(snapshot context.Context) error {
			request := r.WithContext(snapshot)
			libraries, fence, err := d.homeScopeFor(request, p)
			if err != nil {
				return err
			}
			viewer, err := d.catalogViewer(request, p, libraries, fence)
			if err != nil {
				return err
			}
			tx := dbwork.Snapshot(snapshot)
			if tx == nil {
				return sql.ErrNoRows
			}
			owner := d.ownerAuthorityTx(snapshot, tx, p) == nil
			out, err = d.Catalog.WithContext(snapshot).FileInfo(viewer, d.Identity.ID(), r.PathValue("id"), r.URL.Query().Get("cursor"), limit, owner)
			if err != nil {
				return err
			}
			return d.allowedLibraryTx(snapshot, p, out.Scope.LibraryID, tx)
		})
		if err != nil {
			return catalog.FileInfo{}, err
		}
		if _, err = d.principal(r); err != nil {
			return catalog.FileInfo{}, err
		}
		wLibraries, fence, err := d.homeScopeFor(r, p)
		_ = wLibraries
		if err != nil {
			return catalog.FileInfo{}, err
		}
		if fence != out.Scope.ViewerFence {
			return catalog.FileInfo{}, catalog.ErrStaleContinuation
		}
		return out, nil
	})
	v1Route(registry, apikit.Metadata{ID: "get_items_id_recommendations_row", Method: "GET", Path: "/v1/items/{id}/recommendations/rows/{row}", Summary: "Page a title recommendation row to exhaustion", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "stale_continuation", "internal"}}, func(ctx context.Context, r *http.Request, _ struct{}) (catalog.HomeRow, error) {
		if r.PathValue("row") != "more_like" {
			return catalog.HomeRow{}, sql.ErrNoRows
		}
		if err := homeAllowedQuery(r, "cursor", "limit"); err != nil {
			return catalog.HomeRow{}, err
		}
		limit := 37
		if raw := r.URL.Query().Get("limit"); raw != "" {
			var err error
			limit, err = strconv.Atoi(raw)
			if err != nil {
				return catalog.HomeRow{}, catalog.ErrCursor
			}
		}
		p, library, fence, err := d.detailAccess(r)
		if err != nil {
			return catalog.HomeRow{}, err
		}
		viewer, err := d.catalogViewer(r, p, []string{library}, fence)
		if err != nil {
			return catalog.HomeRow{}, err
		}
		return d.Catalog.WithContext(ctx).DetailMoreLike(viewer, r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
	})

}
