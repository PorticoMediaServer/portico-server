package httpapi

import (
	"context"
	"errors"
	"net/http"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
	"strconv"
	"time"
)

// Home is composed by the server: rows, their order, their policy and their
// paging shape all arrive from here so no client infers membership or ranking.
// One request renders the surface; each row then pages on its own endpoint.

func homeFailure(w http.ResponseWriter, e error) {
	if errors.Is(e, catalog.ErrHomeRowUnknown) {
		write(w, 404, map[string]any{"error": map[string]any{"code": "not_found", "message": publicErrorMessage("not_found"), "retryable": false}})
		return
	}
	if errors.Is(e, catalog.ErrHomeLayoutRow) {
		write(w, 400, map[string]any{"error": map[string]any{"code": "invalid_home_layout", "message": publicErrorMessage("invalid_home_layout"), "retryable": false}})
		return
	}
	failure(w, e)
}

// homeRequest resolves the viewer's visible libraries, their saved layout and
// the owner's community-activity policy in one place.
func (d Dependencies) homeRequest(r *http.Request) (identity.Principal, catalog.HomeRequest, error) {
	p, e := d.principal(r)
	if e != nil {
		return p, catalog.HomeRequest{}, e
	}
	restrictions, _, e := d.viewerRestrictions(r, p)
	if e != nil {
		return p, catalog.HomeRequest{}, e
	}
	request, e := d.homeRequestFor(r, p, restrictions)
	return p, request, e
}

// homeRequestFor is homeRequest with the viewer and their restriction already
// resolved, so it can run inside the caller's read snapshot.
func (d Dependencies) homeRequestFor(r *http.Request, p identity.Principal, restrictions identity.ContentRestrictions) (catalog.HomeRequest, error) {
	libraries, fence, e := d.homeScopeFor(r, p)
	if e != nil {
		return catalog.HomeRequest{}, e
	}
	viewer := catalog.Viewer{Profile: identity.PersonalKey(p.Viewer), Fence: fence, Libraries: libraries, Restrictions: restrictions}
	request := catalog.HomeRequest{Viewer: viewer, ServerID: d.Identity.ID(), Profile: identity.PersonalKey(p.Viewer), ViewerFence: fence, Libraries: libraries, Restrictions: restrictions}
	if d.Console == nil {
		return request, nil
	}
	auth := d.consoleAuthority(p, false)
	layout, e := d.Console.HomeLayout(r.Context(), p, auth)
	if e != nil {
		return request, e
	}
	request.RowOrder, request.HiddenRowIDs, request.LayoutRevision = layout.RowOrder, layout.HiddenRowIDs, layout.Revision
	settings, e := d.Console.Settings(r.Context(), auth)
	if e != nil {
		return request, e
	}
	request.CommunityActivity = settings.Effective.CommunityActivityEnabled
	return request, nil
}

func homeQueryLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}
	limit, e := strconv.Atoi(raw)
	if e != nil || limit < 1 {
		return 0, catalog.ErrCursor
	}
	return min(limit, catalog.HomeRowMaxLimit), nil
}

func homeAllowedQuery(r *http.Request, allowed ...string) error {
	for key, values := range r.URL.Query() {
		ok := false
		for _, name := range allowed {
			ok = ok || key == name
		}
		if !ok || len(values) != 1 || len(values[0]) > 4096 {
			return catalog.ErrCursor
		}
	}
	return nil
}

func (d Dependencies) homeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/home", func(w http.ResponseWriter, r *http.Request) {
		if e := homeAllowedQuery(r, "limit"); e != nil {
			homeFailure(w, e)
			return
		}
		limit, e := homeQueryLimit(r)
		if e != nil {
			homeFailure(w, e)
			return
		}
		// The viewer and their restriction come from the authority caches, so the
		// scope resolution, the validator and the whole composition can then run
		// on one read snapshot rather than on four.
		p, e := d.principal(r)
		if e != nil {
			homeFailure(w, e)
			return
		}
		restrictions, _, e := d.viewerRestrictions(r, p)
		if e != nil {
			homeFailure(w, e)
			return
		}
		var out catalog.HomeDocument
		var request catalog.HomeRequest
		notModified := false
		e = d.composeScope(r, restrictions, func(ctx context.Context, cat *catalog.Service) error {
			var scopeErr error
			if request, scopeErr = d.homeRequestFor(r.WithContext(ctx), p, restrictions); scopeErr != nil {
				return scopeErr
			}
			request.Limit = limit
			// Personal/catalog revisions cover mutations; the minute bucket also
			// expires provider feeds and daily rotation. Community composition
			// must recheck other profiles' membership and privacy on every read.
			if revision, revisionErr := cat.HomeRevision(request.Libraries, request.Profile); revisionErr == nil && !request.CommunityActivity {
				tag := responseTag("home", request.ViewerFence, number(revision.Catalog), number(revision.Viewer), number(request.LayoutRevision), number(int64(limit)), number(time.Now().UTC().Unix()/60))
				if conditional(w, r, tag) {
					notModified = true
					return nil
				}
			}
			var composeErr error
			out, composeErr = cat.HomeRows(request)
			return composeErr
		})
		if e != nil {
			homeFailure(w, e)
			return
		}
		if notModified {
			return
		}
		after, e := d.homeFenceAfter(r, p)
		if e != nil {
			homeFailure(w, e)
			return
		}
		if after != request.ViewerFence {
			homeFailure(w, catalog.ErrStaleContinuation)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/home/rows/{id}", func(w http.ResponseWriter, r *http.Request) {
		if e := homeAllowedQuery(r, "limit", "cursor", "start", "revision", "anchorId"); e != nil {
			homeFailure(w, e)
			return
		}
		limit, e := homeQueryLimit(r)
		if e != nil {
			homeFailure(w, e)
			return
		}
		page := catalog.HomeRowPage{Limit: limit, Cursor: r.URL.Query().Get("cursor"), Revision: r.URL.Query().Get("revision"), AnchorID: r.URL.Query().Get("anchorId")}
		if raw := r.URL.Query().Get("start"); raw != "" {
			start, err := strconv.Atoi(raw)
			if err != nil || start < 0 || start > 100000 {
				homeFailure(w, catalog.ErrCursor)
				return
			}
			page.Start = start
		}
		p, request, e := d.homeRequest(r)
		if e != nil {
			homeFailure(w, e)
			return
		}
		// The minute bucket expires provider feeds and daily rotation, as the
		// Home document's validator does; community rows recheck other
		// profiles' privacy on every read, so they carry no validator.
		tag := ""
		if !request.CommunityActivity {
			tag = gridTag(r, p, time.Minute, request.ViewerFence)
			if gridNotModified(w, r, tag) {
				return
			}
		}
		var out catalog.HomeRow
		e = d.compose(r, request.Restrictions, func(cat *catalog.Service) error {
			var composeErr error
			out, composeErr = cat.HomeSingleRow(request, r.PathValue("id"), page)
			return composeErr
		})
		if e != nil {
			homeFailure(w, e)
			return
		}
		after, e := d.homeFenceAfter(r, p)
		if e != nil {
			homeFailure(w, e)
			return
		}
		if after != request.ViewerFence {
			homeFailure(w, catalog.ErrStaleContinuation)
			return
		}
		if tag != "" {
			setGridTag(w, tag)
		}
		write(w, 200, out)
	})
	// CON-23: every row this viewer can arrange, empty or not, with the saved layout.
	mux.HandleFunc("GET /v1/home/layout", func(w http.ResponseWriter, r *http.Request) {
		if e := homeAllowedQuery(r); e != nil {
			homeFailure(w, e)
			return
		}
		_, request, e := d.homeRequest(r)
		if e != nil {
			homeFailure(w, e)
			return
		}
		var out catalog.HomeLayoutView
		e = d.compose(r, request.Restrictions, func(cat *catalog.Service) error {
			var composeErr error
			out, composeErr = cat.HomeLayoutRows(request)
			return composeErr
		})
		if e != nil {
			homeFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("PUT /v1/home/layout", func(w http.ResponseWriter, r *http.Request) {
		d.applyHomeLayout(w, r, false)
	})
	mux.HandleFunc("POST /v1/home/layout/reset", func(w http.ResponseWriter, r *http.Request) {
		d.applyHomeLayout(w, r, true)
	})
	mux.HandleFunc("GET /v1/suggestions", func(w http.ResponseWriter, r *http.Request) {
		if e := homeAllowedQuery(r, "limit"); e != nil {
			homeFailure(w, e)
			return
		}
		limit, e := homeQueryLimit(r)
		if e != nil {
			homeFailure(w, e)
			return
		}
		p, request, e := d.homeRequest(r)
		if e != nil {
			homeFailure(w, e)
			return
		}
		request.Limit = limit
		var out catalog.Suggestions
		e = d.compose(r, request.Restrictions, func(cat *catalog.Service) error {
			var composeErr error
			out, composeErr = cat.Suggestions(request)
			return composeErr
		})
		if e != nil {
			homeFailure(w, e)
			return
		}
		after, e := d.homeFenceAfter(r, p)
		if e != nil {
			homeFailure(w, e)
			return
		}
		if after != request.ViewerFence {
			homeFailure(w, catalog.ErrStaleContinuation)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/items/{id}/recommendations", func(w http.ResponseWriter, r *http.Request) {
		if e := homeAllowedQuery(r, "limit"); e != nil {
			homeFailure(w, e)
			return
		}
		limit, e := homeQueryLimit(r)
		if e != nil {
			homeFailure(w, e)
			return
		}
		p, library, fence, e := d.detailAccess(r)
		if e != nil {
			homeFailure(w, e)
			return
		}
		restrictions, _, e := d.viewerRestrictions(r, p)
		if e != nil {
			homeFailure(w, e)
			return
		}
		var rows []catalog.HomeRow
		var revision catalog.ContentRevision
		viewer, e := d.catalogViewer(r, p, []string{library}, fence)
		if e != nil {
			homeFailure(w, e)
			return
		}
		// Viewers also watched follows the owner's community-activity setting,
		// as Home's Popular on this server does.
		settings, e := d.Console.Settings(r.Context(), d.consoleAuthority(p, false))
		if e != nil {
			homeFailure(w, e)
			return
		}
		e = d.compose(r, restrictions, func(cat *catalog.Service) error {
			var composeErr error
			rows, revision, composeErr = cat.ItemRecommendationRows(viewer, r.PathValue("id"), limit, settings.Effective.CommunityActivityEnabled)
			return composeErr
		})
		if e != nil {
			homeFailure(w, e)
			return
		}
		_, after, e := d.detailFenceFor(r, p)
		if e != nil {
			homeFailure(w, e)
			return
		}
		if after != fence {
			homeFailure(w, catalog.ErrStaleContinuation)
			return
		}
		write(w, 200, map[string]any{"itemId": r.PathValue("id"), "rows": rows, "revision": revision})
	})
}

func (d Dependencies) applyHomeLayout(w http.ResponseWriter, r *http.Request, reset bool) {
	if d.Console == nil {
		failure(w, errors.New("home customisation is unavailable"))
		return
	}
	var body struct {
		ExpectedRevision int64    `json:"expectedRevision"`
		IdempotencyKey   string   `json:"idempotencyKey"`
		RowOrder         []string `json:"rowOrder"`
		HiddenRowIDs     []string `json:"hiddenRowIds"`
	}
	if e := decode(w, r, &body); e != nil {
		failure(w, e)
		return
	}
	p, request, e := d.homeRequest(r)
	if e != nil {
		homeFailure(w, e)
		return
	}
	if !reset {
		if e = d.Catalog.ValidateHomeLayout(request, body.RowOrder, body.HiddenRowIDs); e != nil {
			homeFailure(w, e)
			return
		}
	}
	if body.IdempotencyKey == "" {
		body.IdempotencyKey = identity.Token()
	}
	change := operations.HomeLayoutChange{ExpectedRevision: body.ExpectedRevision, IdempotencyKey: body.IdempotencyKey,
		RowOrder: body.RowOrder, HiddenRowIDs: body.HiddenRowIDs, Reset: reset}
	if reset {
		change.ExpectedRevision = request.LayoutRevision
	}
	out, e := d.Console.ApplyHomeLayout(r.Context(), p, d.consoleAuthority(p, false), change)
	if e != nil {
		consoleError(w, e)
		return
	}
	write(w, 200, out)
}
