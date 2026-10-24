package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"portico.local/apikit"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/contentaccess"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/reading"
)

func (d Dependencies) readingSeriesScopeTx(ctx context.Context, tx *sql.Tx, captured identity.Principal, library string) (reading.SeriesScope, error) {
	var out reading.SeriesScope
	if d.Identity == nil {
		return out, identity.ErrUnauthorized
	}
	live, err := d.Identity.PrincipalByDigestTx(ctx, tx, captured.Hash)
	if err != nil {
		return out, err
	}
	if live.Viewer != captured.Viewer {
		return out, identity.ErrUnauthorized
	}
	if err = live.PermitsRequest(ctx); err != nil {
		return out, err
	}
	if err = d.allowedLibraryTx(ctx, live, library, tx); err != nil {
		return out, err
	}
	restrictions, err := contentaccess.RestrictionsTx(ctx, tx, live.Viewer)
	if err != nil {
		return out, err
	}
	out.Profile = identity.PersonalKey(live.Viewer)
	out.RestrictionFence = catalog.RestrictionFence(restrictions)
	var policy, restriction int64
	if live.Authority != "local" {
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT revision FROM policy WHERE server_id=?),0),COALESCE((SELECT revision FROM restrictions WHERE profile_id=?),0)`, d.Identity.ID(), live.ProfileID).Scan(&policy, &restriction); err != nil {
			return out, err
		}
	}
	// This is the existing browse viewer fence, from the same live facts as
	// its predicate. No cached restriction or earlier authority snapshot enters.
	out.Fence = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%d:%s", presentationIdentity(live), live.Epoch, policy, restriction, out.RestrictionFence))))
	err = tx.QueryRowContext(ctx, `SELECT r.revision,COALESCE(v.revision,0) FROM library_revisions r LEFT JOIN viewer_revisions v ON v.library_id=r.library_id AND v.profile_id=? WHERE r.library_id=?`, out.Profile, library).Scan(&out.Catalog, &out.Viewer)
	return out, err
}

func (d Dependencies) readingSeries(w http.ResponseWriter, r *http.Request) {
	p, ok := d.readingPrincipal(w, r)
	if !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			readingFailure(w, reading.ErrInvalid)
			return
		}
	}
	page, err := d.readingService().Series(r.Context(), p, r.PathValue("libraryId"), r.PathValue("seriesId"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		readingFailure(w, err)
		return
	}
	out := ReadingItemsResponse{Items: make([]ReadingItem, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		out.Items = append(out.Items, readingItemProjection(item))
	}
	write(w, 200, out)
}

func registerReadingSeriesHTTPContract(registry *apikit.Registry) error {
	m := apikit.Metadata{ID: "get_reading_series_items", Method: "GET", Path: "/v1/libraries/{libraryId}/reading/series/{seriesId}", Summary: "Read one complete authorized Reading series", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "revision_mismatch", "request_in_progress", "internal"}, Query: []string{"cursor", "limit"}}
	return apikit.RegisterHTTP[struct{}, ReadingItemsResponse](registry, m)
}
