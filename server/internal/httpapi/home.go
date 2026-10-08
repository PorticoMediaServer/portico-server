package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// homeScope resolves the viewer, the libraries they may see, and the fence that
// identifies that authority. It is the first half of every composite catalogue
// read.
//
// It used to resolve the allowed-library set by asking the policy once per
// library, each question in its own read transaction. Nine libraries meant
// thirteen transactions and forty-nine statements — and every composite handler
// ran it twice. It now asks once for the whole set.
func (d Dependencies) homeScope(r *http.Request) (identity.Principal, []string, string, error) {
	p, e := d.principal(r)
	if e != nil {
		return p, nil, "", e
	}
	ids, fence, e := d.homeScopeFor(r, p)
	return p, ids, fence, e
}

// homeScopeFor is homeScope with the principal already resolved.
func (d Dependencies) homeScopeFor(r *http.Request, p identity.Principal) ([]string, string, error) {
	var ids []string
	var fence string
	resolve := func(ctx context.Context) error {
		libraries, e := d.Catalog.WithContext(ctx).Libraries()
		if e != nil {
			return e
		}
		// The server-wide check ("") and every per-library check are one batch:
		// they are the same question asked of different arguments, and asking
		// them together is what makes the whole scope one read transaction.
		names := make([]string, 0, len(libraries)+1)
		names = append(names, "")
		for _, library := range libraries {
			names = append(names, library.ID)
		}
		allowed, e := d.allowedLibrarySet(ctx, p, names)
		if e != nil {
			return e
		}
		if !allowed[""] {
			return identity.ErrUnauthorized
		}
		ids = []string{}
		for _, library := range libraries {
			if allowed[library.ID] {
				ids = append(ids, library.ID)
			}
		}
		sort.Strings(ids)
		var policy, restriction int64
		if p.Authority != "local" {
			if e = d.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT revision FROM policy WHERE server_id=?),0),COALESCE((SELECT revision FROM restrictions WHERE profile_id=?),0)`, d.Identity.ID(), p.ProfileID).Scan(&policy, &restriction); e != nil {
				return e
			}
		}
		raw, _ := json.Marshal(ids)
		_, contentFence, e := d.viewerRestrictions(r.WithContext(ctx), p)
		if e != nil {
			return e
		}
		fence = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%d:%s:%s", p.Hash, p.Epoch, policy, restriction, raw, contentFence))))
		return nil
	}
	if d.DB == nil {
		return ids, fence, resolve(r.Context())
	}
	if e := dbwork.WithReadSnapshot(r.Context(), d.DB, resolve); e != nil {
		return nil, "", e
	}
	return ids, fence, nil
}

// homeFenceAfter is the post-read authorisation proof. The catalogue half of the
// old "resolve the whole scope again" check is satisfied by construction once
// the composition runs inside one read snapshot — a WAL snapshot cannot observe
// a write — but a revocation landing mid-request still has to be caught, so the
// authority half is re-resolved against live state and compared to the fence the
// response was composed under. Same 409, same condition, one resolution instead
// of a second full composition.
func (d Dependencies) homeFenceAfter(r *http.Request, p identity.Principal) (string, error) {
	_, fence, err := d.homeScopeFor(r, p)
	return fence, err
}

func (d Dependencies) contentHomeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/content", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("view") == "watchlist" || q.Get("view") == "favorites" || q.Get("view") == "playlists" {
			d.savedContent(w, r)
			return
		}
		if q.Get("view") != "home" {
			failure(w, errors.New("global content view must be home"))
			return
		}
		for key, values := range q {
			if len(values) != 1 || (key != "view" && key != "limit") || (key == "limit" && values[0] != "12") {
				failure(w, errors.New("Home has fixed server-defined sections and limit"))
				return
			}
		}
		p, libraries, fence, e := d.homeScope(r)
		if e != nil {
			failure(w, e)
			return
		}
		restrictions, _, e := d.viewerRestrictions(r, p)
		if e != nil {
			failure(w, e)
			return
		}
		var out catalog.ContentEnvelope
		e = d.compose(r, restrictions, func(cat *catalog.Service) error {
			var composeErr error
			viewer := catalog.Viewer{Profile: identity.PersonalKey(p.Viewer), Fence: fence, Libraries: libraries, Restrictions: restrictions}
			out, composeErr = cat.Home(catalog.HomeRequest{Viewer: viewer, ServerID: d.Identity.ID(), Profile: identity.PersonalKey(p.Viewer), ViewerFence: fence, Libraries: libraries, Restrictions: restrictions})
			return composeErr
		})
		if e != nil {
			failure(w, e)
			return
		}
		after, e := d.homeFenceAfter(r, p)
		if e != nil {
			failure(w, e)
			return
		}
		if after != fence {
			failure(w, catalog.ErrStaleContinuation)
			return
		}
		write(w, 200, out)
	})
}

// compose runs a composite handler's body on one read snapshot, and hands the
// body a catalogue bound to it. Every read inside joins that snapshot, so a page
// is built from one pooled connection and one WAL frame boundary instead of one
// per query, and the rows cannot disagree with each other about which
// publication they came from.
// composeScope is compose with the caller's own preparation folded in: the
// resolution of the request's scope and the composition of its body run on one
// read snapshot instead of two.
//
// Pending ratings are classified by a background worker. Request composition
// opens one read snapshot and treats pending spellings as unrated.
func (d Dependencies) composeScope(r *http.Request, restrictions identity.ContentRestrictions, body func(context.Context, *catalog.Service) error) error {
	if err := d.Catalog.WithContext(r.Context()).PrepareRead(r.Context(), restrictions); err != nil {
		return err
	}
	if d.DB == nil {
		return body(r.Context(), d.Catalog.WithContext(r.Context()))
	}
	return dbwork.WithReadSnapshot(r.Context(), d.DB, func(ctx context.Context) error {
		return body(ctx, d.Catalog.WithContext(ctx))
	})
}

func (d Dependencies) compose(r *http.Request, restrictions identity.ContentRestrictions, body func(*catalog.Service) error) error {
	return d.composeScope(r, restrictions, func(_ context.Context, cat *catalog.Service) error {
		return body(cat.WithRecommendationRestrictions(restrictions))
	})
}

func (d Dependencies) savedContent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for key, values := range q {
		if len(values) != 1 || (key != "view" && key != "limit" && key != "sort" && key != "direction" && key != "cursor" && key != "filter") {
			failure(w, catalog.ErrCursor)
			return
		}
	}
	limit := 40
	if q.Has("limit") {
		var e error
		limit, e = strconv.Atoi(q.Get("limit"))
		if e != nil || limit < 1 || limit > 100 {
			failure(w, catalog.ErrCursor)
			return
		}
	}
	p, libraries, fence, e := d.homeScope(r)
	if e != nil {
		failure(w, e)
		return
	}
	restrictions, _, e := d.viewerRestrictions(r, p)
	if e != nil {
		failure(w, e)
		return
	}
	viewer, e := d.catalogViewer(r, p, libraries, fence)
	if e != nil {
		failure(w, e)
		return
	}
	request := catalog.ContentRequest{Viewer: viewer, ServerID: d.Identity.ID(), Profile: identity.PersonalKey(p.Viewer), ViewerFence: fence, View: q.Get("view"), Sort: q.Get("sort"), Direction: q.Get("direction"), Filter: q.Get("filter"), Cursor: q.Get("cursor"), Limit: limit, Restrictions: restrictions}
	var out catalog.ContentEnvelope
	e = d.compose(r, restrictions, func(cat *catalog.Service) error {
		var composeErr error
		if q.Get("view") == "playlists" {
			out, composeErr = cat.PlaylistDirectory(request, resourceActor(p))
		} else {
			out, composeErr = cat.Saved(request, libraries)
		}
		return composeErr
	})
	if e != nil {
		failure(w, e)
		return
	}
	after, e := d.homeFenceAfter(r, p)
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
