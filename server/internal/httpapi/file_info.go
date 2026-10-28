package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"portico.local/apikit"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
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
		limit, err := recommendationRowLimit(r)
		if err != nil {
			return catalog.HomeRow{}, err
		}
		p, library, fence, err := d.detailAccess(r)
		if err != nil {
			return catalog.HomeRow{}, err
		}
		restrictions, _, err := d.viewerRestrictions(r, p)
		if err != nil {
			return catalog.HomeRow{}, err
		}
		viewer, err := d.catalogViewer(r, p, []string{library}, fence)
		if err != nil {
			return catalog.HomeRow{}, err
		}
		community, err := d.communityActivity(r, p)
		if err != nil {
			return catalog.HomeRow{}, err
		}
		var out catalog.HomeRow
		err = d.compose(r, restrictions, func(cat *catalog.Service) error {
			var e error
			out, e = cat.ItemRecommendationRow(viewer, r.PathValue("id"), r.PathValue("row"), r.URL.Query().Get("cursor"), limit, community)
			return e
		})
		if err != nil {
			return catalog.HomeRow{}, err
		}
		if _, after, err := d.detailFenceFor(r, p); err != nil || after != fence {
			return catalog.HomeRow{}, errors.Join(err, catalog.ErrStaleContinuation)
		}
		return out, nil
	})
	v1Route(registry, apikit.Metadata{ID: "get_shows_id_recommendations_row", Method: "GET", Path: "/v1/shows/{id}/recommendations/rows/{row}", Summary: "Page a show recommendation row to exhaustion", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, Status: 200, Query: []string{"cursor", "limit"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "stale_continuation", "internal"}}, func(ctx context.Context, r *http.Request, _ struct{}) (catalog.HomeRow, error) {
		limit, err := recommendationRowLimit(r)
		if err != nil {
			return catalog.HomeRow{}, err
		}
		p, viewer, restrictions, fence, err := d.showRecommendationViewer(r)
		if err != nil {
			return catalog.HomeRow{}, err
		}
		var out catalog.HomeRow
		err = d.compose(r, restrictions, func(cat *catalog.Service) error {
			var e error
			out, e = cat.ShowRecommendationRow(viewer, r.PathValue("id"), r.PathValue("row"), r.URL.Query().Get("cursor"), limit)
			return e
		})
		if err != nil {
			return catalog.HomeRow{}, err
		}
		if _, after, err := d.detailLibraryFenceFor(r, p, viewer.Libraries[0]); err != nil || after != fence {
			return catalog.HomeRow{}, errors.Join(err, catalog.ErrStaleContinuation)
		}
		return out, nil
	})
}

// recommendationRowLimit is a row page's size: 37 when absent.
func recommendationRowLimit(r *http.Request) (int, error) {
	if err := homeAllowedQuery(r, "cursor", "limit"); err != nil {
		return 0, err
	}
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 37, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 200 {
		return 0, catalog.ErrCursor
	}
	return limit, nil
}

// showRecommendationViewer authorizes a show page's rows: the viewer of the
// show's library under the same fence its title pages carry.
func (d Dependencies) showRecommendationViewer(r *http.Request) (identity.Principal, catalog.Viewer, identity.ContentRestrictions, string, error) {
	p, err := d.principal(r)
	if err != nil {
		return p, catalog.Viewer{}, identity.ContentRestrictions{}, "", err
	}
	library, err := d.Catalog.WithContext(r.Context()).LibraryForShow(r.PathValue("id"))
	if err != nil {
		return p, catalog.Viewer{}, identity.ContentRestrictions{}, "", err
	}
	_, fence, err := d.detailLibraryFenceFor(r, p, library)
	if err != nil {
		return p, catalog.Viewer{}, identity.ContentRestrictions{}, "", err
	}
	restrictions, _, err := d.viewerRestrictions(r, p)
	if err != nil {
		return p, catalog.Viewer{}, identity.ContentRestrictions{}, "", err
	}
	viewer, err := d.catalogViewer(r, p, []string{library}, fence)
	return p, viewer, restrictions, fence, err
}
