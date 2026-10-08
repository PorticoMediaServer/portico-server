package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"strconv"
)

func savedPageQuery(r *http.Request, extra ...string) (int, error) {
	for k, v := range r.URL.Query() {
		valid := k == "cursor" || k == "limit"
		for _, key := range extra {
			valid = valid || k == key
		}
		if !valid || len(v) != 1 {
			return 0, catalog.ErrCursor
		}
	}
	limit := 40
	if r.URL.Query().Has("limit") {
		var e error
		limit, e = strconv.Atoi(r.URL.Query().Get("limit"))
		if e != nil || limit < 1 || limit > 100 {
			return 0, catalog.ErrCursor
		}
	}
	return limit, nil
}
func (d Dependencies) savedResponse(w http.ResponseWriter, r *http.Request, fence string, out any) {
	_, _, after, e := d.homeScope(r)
	if e != nil {
		failure(w, e)
		return
	}
	if after != fence {
		failure(w, catalog.ErrStaleContinuation)
		return
	}
	write(w, 200, out)
}
func (d Dependencies) shareTargetTx(tx *sql.Tx, authority, account, profile string) error {
	if authority == "local" {
		return identity.DirectShareTarget(tx, account, profile)
	}
	if authority == "hosted" {
		return d.Hosted.AllowedTx(identity.Principal{Viewer: identity.Viewer{Authority: authority, AccountID: account, ProfileID: profile}}, "", tx)
	}
	return errors.New("invalid share authority")
}
func (d Dependencies) savedResourceRoutes(mux *http.ServeMux) {
	rate := &personalLimiter{}
	mux.HandleFunc("GET /v1/saved-resources", func(w http.ResponseWriter, r *http.Request) {
		limit, e := savedPageQuery(r, "kind", "pinned")
		if e != nil {
			failure(w, e)
			return
		}
		q := r.URL.Query()
		// Pinned listings are returned in the viewer's arrangement; both spellings
		// of the flag are accepted so a hand-written client works either way.
		pinned := q.Get("pinned")
		if q.Has("pinned") && pinned != "true" && pinned != "false" && pinned != "1" && pinned != "0" {
			failure(w, catalog.ErrCursor)
			return
		}
		p, _, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.WithContext(r.Context()).SavedResources(d.Identity.ID(), fence, resourceActor(p), q.Get("kind"), q.Get("cursor"), pinned == "true" || pinned == "1", limit)
		if e != nil {
			failure(w, e)
			return
		}
		d.savedResponse(w, r, fence, out)
	})
	mux.HandleFunc("GET /v1/saved-resources/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			failure(w, catalog.ErrCursor)
			return
		}
		p, _, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.WithContext(r.Context()).SavedResource(d.Identity.ID(), fence, r.PathValue("id"), resourceActor(p))
		if e != nil {
			failure(w, e)
			return
		}
		d.savedResponse(w, r, fence, out)
	})
	mux.HandleFunc("GET /v1/saved-resources/{id}/content", func(w http.ResponseWriter, r *http.Request) {
		limit, e := savedPageQuery(r)
		if e != nil {
			failure(w, e)
			return
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
		out, e := d.Catalog.WithContext(r.Context()).SavedResourceContent(viewer, d.Identity.ID(), fence, r.PathValue("id"), resourceActor(p), r.URL.Query().Get("cursor"), limit)
		if e != nil {
			failure(w, e)
			return
		}
		d.savedResponse(w, r, fence, out)
	})
	mux.HandleFunc("GET /v1/saved-resources/{id}/share-candidates", func(w http.ResponseWriter, r *http.Request) {
		limit, e := savedPageQuery(r)
		if e != nil {
			failure(w, e)
			return
		}
		p, _, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.WithContext(r.Context()).ResourceCandidates(d.Identity.ID(), fence, r.PathValue("id"), resourceActor(p), r.URL.Query().Get("cursor"), limit)
		if e != nil {
			failure(w, e)
			return
		}
		d.savedResponse(w, r, fence, out)
	})
	for _, route := range []struct{ pattern, action string }{{"POST /v1/saved-resources", "create"}, {"PATCH /v1/saved-resources/{id}", "update"}, {"DELETE /v1/saved-resources/{id}", "delete"}, {"PUT /v1/saved-resources/{id}/entries", "entries"}, {"PUT /v1/saved-resources/{id}/shares", "share"}, {"DELETE /v1/saved-resources/{id}/shares", "unshare"}} {
		mux.HandleFunc(route.pattern, func(w http.ResponseWriter, r *http.Request) {
			p, _, fence, e := d.homeScope(r)
			if e != nil {
				failure(w, e)
				return
			}
			if !rate.allow(viewerScope(p)) {
				w.Header().Set("Retry-After", "60")
				write(w, 429, map[string]any{"error": map[string]any{"code": "rate_limited", "message": "Too many saved resource changes. Try again shortly.", "retryable": true}})
				return
			}
			var m catalog.SavedResourceMutation
			if e = decodeSavedRevision(w, r, &m); e != nil {
				failure(w, e)
				return
			}
			// Membership reports per item. An item that is gone or in a library
			// this viewer cannot read is one outcome, not a rejected batch.
			rejected := []catalog.EntryOutcome{}
			if route.action == "entries" && len(m.AddItemIDs) > 0 {
				keep := []string{}
				for _, id := range m.AddItemIDs {
					var library string
					if err := d.DB.QueryRowContext(r.Context(), `SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?)`, id).Scan(&library); err != nil {
						rejected = append(rejected, catalog.EntryOutcome{ItemID: id, Code: "not_found"})
						continue
					}
					if err := d.allowedLibrary(r.Context(), p, library); err != nil {
						rejected = append(rejected, catalog.EntryOutcome{ItemID: id, Code: "unauthorized"})
						continue
					}
					keep = append(keep, id)
				}
				if len(keep) == 0 && len(m.RemoveEntryIDs) == 0 {
					write(w, 200, map[string]any{"serverId": d.Identity.ID(), "viewerFence": fence, "receipt": catalog.SavedResourceReceipt{OperationID: m.OperationID, ResourceID: r.PathValue("id"), Entries: &catalog.EntryOutcomes{Added: []string{}, Removed: []string{}, Unchanged: []string{}, Failed: rejected}}, "receiptLifetimeSeconds": 2592000})
					return
				}
				m.AddItemIDs = keep
			}
			out, e := d.Catalog.MutateSavedResource(resourceActor(p), r.PathValue("id"), route.action, m, func(tx *sql.Tx) error {
				if e := d.resourceAuthorize(tx, p, ""); e != nil {
					return e
				}
				if m.Definition != nil {
					if e := d.resourceAuthorize(tx, p, m.Definition.LibraryID); e != nil {
						return e
					}
				}
				for _, id := range m.AddItemIDs {
					var library string
					if e := tx.QueryRow(`SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?)`, id).Scan(&library); e != nil {
						return e
					}
					if e := d.resourceAuthorize(tx, p, library); e != nil {
						return e
					}
				}
				if route.action == "share" {
					return d.shareTargetTx(tx, m.Authority, m.AccountID, m.ProfileID)
				}
				return nil
			})
			if e != nil {
				failure(w, e)
				return
			}

			if len(rejected) > 0 {
				if out.Entries == nil {
					out.Entries = &catalog.EntryOutcomes{Added: []string{}, Removed: []string{}, Unchanged: []string{}, Failed: []catalog.EntryOutcome{}}
				}
				out.Entries.Failed = append(out.Entries.Failed, rejected...)
			}
			dead, revision, e := d.Catalog.WithContext(r.Context()).SavedResourceReceiptStatus(out.ResourceID, resourceActor(p))
			if e != nil {
				failure(w, e)
				return
			}
			d.savedResponse(w, r, fence, map[string]any{"serverId": d.Identity.ID(), "viewerFence": fence, "receipt": out, "current": map[string]any{"resourceId": out.ResourceID, "revision": revision, "deleted": dead}, "receiptLifetimeSeconds": 2592000})
		})
	}
	mux.HandleFunc("PUT /v1/saved-pins/{kind}/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, _, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		var m catalog.PinMutation
		if e = decodeSavedRevision(w, r, &m); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.MutateSavedPin(resourceActor(p), r.PathValue("kind"), r.PathValue("id"), m, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if e != nil {
			failure(w, e)
			return
		}
		d.savedResponse(w, r, fence, map[string]any{"serverId": d.Identity.ID(), "viewerFence": fence, "operationId": out.OperationID, "resourceId": out.ResourceID, "playlistId": out.PlaylistID, "revision": out.Revision, "pinRevision": out.PinRevision, "pinned": out.Pinned, "deleted": out.Deleted})
	})
	mux.HandleFunc("GET /v1/personal-history", func(w http.ResponseWriter, r *http.Request) {
		limit, e := savedPageQuery(r, "period", "libraryId")
		if e != nil {
			failure(w, e)
			return
		}
		p, libraries, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.WithContext(r.Context()).History(d.Identity.ID(), fence, identity.PersonalKey(p.Viewer), r.URL.Query().Get("cursor"), r.URL.Query().Get("period"), r.URL.Query().Get("libraryId"), libraries, limit)
		if e != nil {
			failure(w, e)
			return
		}
		d.savedResponse(w, r, fence, out)
	})
	mux.HandleFunc("POST /v1/me/recommendations:reset", func(w http.ResponseWriter, r *http.Request) {
		p, _, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		var m catalog.RecommendationsReset
		if e = decode(w, r, &m); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.ResetRecommendations(identity.PersonalKey(p.Viewer), m, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if e != nil {
			failure(w, e)
			return
		}
		d.savedResponse(w, r, fence, map[string]any{"serverId": d.Identity.ID(), "viewerFence": fence, "operationId": out.OperationID, "clearedNotInterested": out.ClearedNotInterested, "receiptLifetimeSeconds": 2592000})
	})
	mux.HandleFunc("POST /v1/personal-history/actions", func(w http.ResponseWriter, r *http.Request) {
		p, _, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		var m catalog.ActivityMutation
		if e = decodeSavedRevision(w, r, &m); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.MutateActivity(identity.PersonalKey(p.Viewer), m, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
		if e != nil {
			failure(w, e)
			return
		}
		d.savedResponse(w, r, fence, map[string]any{"serverId": d.Identity.ID(), "viewerFence": fence, "receipt": out, "receiptLifetimeSeconds": 2592000})
	})
}
