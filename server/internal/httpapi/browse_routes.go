package httpapi

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"strconv"
)

// Browse engine routes. Everything a client can express in the library — the
// vocabulary, the expression query, facet counts, pin order and pinned libraries
// — is published here, so a third-party player can drive the same surfaces the
// first-party clients do.

// browseFailure answers a validation error with the offending path so a client
// can attach the message to the control that produced it.
func browseFailure(w http.ResponseWriter, e error) {
	var issue *catalog.BrowseValidationError
	if errors.As(e, &issue) {
		write(w, 400, map[string]any{"error": map[string]any{"code": "invalid_query", "message": issue.Message, "field": issue.Path, "retryable": false}})
		return
	}
	if errors.Is(e, catalog.ErrBrowsePivot) {
		write(w, 400, map[string]any{"error": map[string]any{"code": "invalid_pivot", "message": publicErrorMessage("invalid_pivot"), "field": "pivot", "retryable": false}})
		return
	}
	if errors.Is(e, catalog.ErrPinOrderConflict) || errors.Is(e, catalog.ErrLibraryNavigationConflict) {
		write(w, 409, map[string]any{"error": map[string]any{"code": "pin_order_conflict", "message": publicErrorMessage("pin_order_conflict"), "retryable": true}})
		return
	}
	failure(w, e)
}

// browseFence folds the authorization facts into one opaque value; a permission
// change invalidates an open range exactly as a catalog change does.
func (d Dependencies) browseFence(r *http.Request, p identity.Principal) (string, error) {
	var policy, restriction int64
	if p.Authority != "local" {
		if e := dbwork.QueryRow(r.Context(), d.DB, `SELECT COALESCE((SELECT revision FROM policy WHERE server_id=?),0),COALESCE((SELECT revision FROM restrictions WHERE profile_id=?),0)`, d.Identity.ID(), p.ProfileID).Scan(&policy, &restriction); e != nil {
			return "", e
		}
	}
	_, contentFence, e := d.viewerRestrictions(r, p)
	if e != nil {
		return "", e
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%d:%s", p.Hash, p.Epoch, policy, restriction, contentFence)))), nil
}

func (d Dependencies) browseLibrary(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	p, e := d.principal(r)
	if e == nil {
		e = d.allowedLibrary(r.Context(), p, r.PathValue("id"))
	}
	if e != nil {
		failure(w, e)
		return p, false
	}
	return p, true
}

type browseRequestBody struct {
	Pivot  string                        `json:"pivot"`
	Query  json.RawMessage               `json:"query"`
	Sort   []catalog.BrowseSortSelection `json:"sort"`
	Limit  int                           `json:"limit"`
	Cursor string                        `json:"cursor"`
	Range  *catalog.BrowseRange          `json:"range"`
	Seek   *catalog.BrowseSeek           `json:"seek"`
}

func (d Dependencies) browseRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/libraries/{id}/browse-capabilities", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := d.browseLibrary(w, r); !ok {
			return
		}
		for key, values := range r.URL.Query() {
			if key != "pivot" || len(values) != 1 {
				failure(w, catalog.ErrCursor)
				return
			}
		}
		out, e := d.Catalog.WithContext(r.Context()).BrowseCapabilitiesFor(r.PathValue("id"), r.URL.Query().Get("pivot"))
		if e != nil {
			browseFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/libraries/{id}/browse", func(w http.ResponseWriter, r *http.Request) {
		p, ok := d.browseLibrary(w, r)
		if !ok {
			return
		}
		var body browseRequestBody
		if e := decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		query, e := catalog.ParseBrowseQuery(body.Query, "query")
		if e != nil {
			browseFailure(w, e)
			return
		}
		fence, e := d.browseFence(r, p)
		if e != nil {
			failure(w, e)
			return
		}
		restrictions, _, e := d.viewerRestrictions(r, p)
		if e != nil {
			failure(w, e)
			return
		}
		// One read snapshot for the whole page: the count, the letter index, the
		// anchor rank and the rows are then four views of one state rather than
		// four consecutive states, which is what the second revision read inside
		// browseSelect existed to detect.
		var out catalog.BrowseResult
		e = d.compose(r, restrictions, func(cat *catalog.Service) error {
			var composeErr error
			viewer := catalog.Viewer{Profile: identity.PersonalKey(p.Viewer), Fence: fence, Libraries: []string{r.PathValue("id")}, Restrictions: restrictions}
			out, composeErr = cat.BrowseEntities(viewer, catalog.BrowseRequest{ServerID: d.Identity.ID(), Library: r.PathValue("id"), Profile: identity.PersonalKey(p.Viewer), ViewerFence: fence, Pivot: body.Pivot, Query: query, Sort: body.Sort, Limit: min(body.Limit, catalog.BrowseMaximumLimit), Cursor: body.Cursor, Range: body.Range, Seek: body.Seek, Restrictions: restrictions})
			return composeErr
		})
		if e != nil {
			browseFailure(w, e)
			return
		}
		// Authorization is proven again after the read so a revocation that landed
		// mid-request cannot be answered with catalogue facts.
		if e = d.allowedLibrary(r.Context(), p, r.PathValue("id")); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/libraries/{id}/facets", func(w http.ResponseWriter, r *http.Request) {
		p, ok := d.browseLibrary(w, r)
		if !ok {
			return
		}
		q := r.URL.Query()
		for key, values := range q {
			if (key != "field" && key != "q" && key != "limit") || len(values) != 1 {
				failure(w, catalog.ErrCursor)
				return
			}
		}
		limit := 0
		if q.Has("limit") {
			parsed, e := strconv.Atoi(q.Get("limit"))
			if e != nil || parsed < 1 {
				failure(w, catalog.ErrCursor)
				return
			}
			limit = min(parsed, 200)
		}
		restrictions, restrictionFence, e := d.viewerRestrictions(r, p)
		if e != nil {
			failure(w, e)
			return
		}
		// Facet counts change only when the catalogue does, and they are already
		// keyed on the restriction fence inside the service. The validator says
		// the same thing to the client.
		if revision, revisionErr := d.Catalog.WithContext(r.Context()).ContentRevision(r.PathValue("id"), identity.PersonalKey(p.Viewer)); revisionErr == nil {
			tag := responseTag("facets", r.PathValue("id"), p.Hash, restrictionFence, q.Get("field"), q.Get("q"), strconv.Itoa(limit), number(revision.Catalog), number(revision.Viewer))
			if conditional(w, r, tag) {
				return
			}
		}
		var out catalog.FacetPage
		e = d.compose(r, restrictions, func(cat *catalog.Service) error {
			var composeErr error
			viewer := catalog.Viewer{Profile: identity.PersonalKey(p.Viewer), Fence: restrictionFence, Libraries: []string{r.PathValue("id")}, Restrictions: restrictions}
			out, composeErr = cat.Facets(viewer, catalog.FacetRequest{Library: r.PathValue("id"), Profile: identity.PersonalKey(p.Viewer), Field: q.Get("field"), Q: q.Get("q"), Limit: limit, Restrictions: restrictions})
			return composeErr
		})
		if e != nil {
			browseFailure(w, e)
			return
		}
		if e = d.allowedLibrary(r.Context(), p, r.PathValue("id")); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("PUT /v1/saved-pins/order", func(w http.ResponseWriter, r *http.Request) {
		p, _, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			ExpectedRevision int64                   `json:"expectedRevision"`
			Order            []catalog.PinOrderEntry `json:"order"`
		}
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.SetPinOrder(resourceActor(p), body.ExpectedRevision, body.Order, func(tx *sql.Tx) error {
			return d.resourceAuthorize(tx, p, "")
		})
		if e != nil {
			browseFailure(w, e)
			return
		}
		d.savedResponse(w, r, fence, map[string]any{"serverId": d.Identity.ID(), "viewerFence": fence, "revision": out.Revision, "order": out.Order})
	})
	mux.HandleFunc("GET /v1/saved-pins/order", func(w http.ResponseWriter, r *http.Request) {
		p, _, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.WithContext(r.Context()).PinOrder(resourceActor(p))
		if e != nil {
			browseFailure(w, e)
			return
		}
		d.savedResponse(w, r, fence, map[string]any{"serverId": d.Identity.ID(), "viewerFence": fence, "revision": out.Revision, "order": out.Order})
	})
	mux.HandleFunc("GET /v1/me/library-navigation", func(w http.ResponseWriter, r *http.Request) {
		p, libraries, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		visible := map[string]bool{}
		for _, library := range libraries {
			visible[library] = true
		}
		out, e := d.Catalog.WithContext(r.Context()).LibraryNavigation(resourceActor(p), func(id string) bool { return visible[id] })
		if e != nil {
			browseFailure(w, e)
			return
		}
		d.savedResponse(w, r, fence, out)
	})
	mux.HandleFunc("PUT /v1/me/library-navigation", func(w http.ResponseWriter, r *http.Request) {
		p, _, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			ExpectedRevision int64    `json:"expectedRevision"`
			PinnedLibraryIDs []string `json:"pinnedLibraryIds"`
		}
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.SetLibraryNavigation(resourceActor(p), body.ExpectedRevision, body.PinnedLibraryIDs, func(tx *sql.Tx, library string) error {
			return d.resourceAuthorize(tx, p, library)
		})
		if e != nil {
			browseFailure(w, e)
			return
		}
		d.savedResponse(w, r, fence, out)
	})
}
