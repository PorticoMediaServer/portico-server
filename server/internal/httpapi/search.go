package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (d Dependencies) searchScope(r *http.Request) (identity.Principal, bool, []string, bool, bool, string, error) {
	p, e := d.principal(r)
	if e != nil {
		return p, false, nil, false, false, "", e
	}
	all, ids, policy, restriction, e := d.Hosted.SearchAccess(p)
	if e != nil {
		return p, false, nil, false, false, "", e
	}
	sort.Strings(ids)
	raw, _ := json.Marshal(ids)
	_, contentFence, e := d.viewerRestrictions(r, p)
	if e != nil {
		return p, false, nil, false, false, "", e
	}
	// Service presence is not permission. Use the same current profile authority
	// as Guide, and fence this decision so revocation during composition also
	// withholds the response, even if a content-policy cache is still warm.
	liveTV := false
	if d.LiveChannels != nil {
		e = d.featureAllowed(r.Context(), p, "live_tv")
		if e != nil && !errors.Is(e, errFeatureRestricted) {
			return p, false, nil, false, false, "", e
		}
		liveTV = e == nil
	}
	sourceFence := ""
	liveConfigured := false
	if liveTV && d.DB != nil {
		tx, err := d.DB.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return p, false, nil, false, false, "", err
		}
		scope, scopeErr := d.guideDirectoryAuthority(p, contentFence)(r.Context(), tx)
		err = scopeErr
		sourceFence = scope.Fence
		liveConfigured = len(scope.AllowedLiveSources) > 0 || len(scope.LibraryRows) > 0
		tx.Rollback()
		if err != nil {
			return p, false, nil, false, false, "", err
		}
	}
	fence := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%d:%t:%s:%s:%t", presentationIdentity(p), p.Epoch, policy, restriction, all, raw, contentFence+":"+sourceFence, liveTV))))
	return p, all, ids, liveTV, liveConfigured, fence, nil
}

// commaList bounds a comma separated request parameter. An empty element is a
// request error rather than a silently dropped filter.
func commaList(raw string, max int) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > max {
		return nil, catalog.ErrSearchQuery
	}
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || len(part) > 256 {
			return nil, catalog.ErrSearchQuery
		}
		out = append(out, part)
	}
	return out, nil
}

func (d Dependencies) searchRoutes(mux *http.ServeMux) {
	if d.registry == nil {
		mux.HandleFunc("GET /v1/search", d.searchHandler)
		mux.HandleFunc("GET /v1/search/discovery", d.searchDiscoveryHandler)
	}
	d.httpHandle(mux, "GET /v1/search/history", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		if len(r.URL.Query()) != 0 {
			failure(w, catalog.ErrSearchQuery)
			return
		}
		out, e := d.Catalog.WithContext(r.Context()).SearchHistory(p.Viewer)
		if e != nil {
			failure(w, e)
			return
		}
		out.ServerID = d.Identity.ID()
		out.ViewerFence = presentationIdentity(p)
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, out)
	})
	d.httpHandle(mux, "DELETE /v1/search/history", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Catalog.WithContext(r.Context()).ClearSearchHistory(p.Viewer)
		if e != nil {
			failure(w, e)
			return
		}
		out.ServerID = d.Identity.ID()
		out.ViewerFence = presentationIdentity(p)
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, out)
	})
}

func (d Dependencies) searchHandler(w http.ResponseWriter, r *http.Request) {
	// One request budget above the per-group budget, so a whole-response
	// timeout only happens when every group is slow at once.
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	q := r.URL.Query()
	allowed := map[string]bool{"q": true, "group": true, "groups": true, "cursor": true, "limit": true, "sort": true, "direction": true, "libraryIds": true, "record": true}
	for key, values := range q {
		if len(values) != 1 || !allowed[key] {
			failure(w, catalog.ErrSearchQuery)
			return
		}
	}
	limit := 40
	var e error
	if q.Has("limit") {
		limit, e = strconv.Atoi(q.Get("limit"))
		if e != nil || limit < 1 {
			failure(w, catalog.ErrSearchQuery)
			return
		}
	}
	limit = min(limit, 40)
	if q.Has("record") && q.Get("record") != "1" && q.Get("record") != "0" {
		failure(w, catalog.ErrSearchQuery)
		return
	}
	groups, e := commaList(q.Get("groups"), 16)
	if e != nil {
		failure(w, e)
		return
	}
	libraryIDs, e := commaList(q.Get("libraryIds"), 64)
	if e != nil {
		failure(w, e)
		return
	}
	p, all, ids, liveTV, liveConfigured, fence, e := d.searchScope(r)
	if e != nil {
		failure(w, e)
		return
	}
	if all {
		ids, _, e = d.homeScopeFor(r, p)
		if e != nil {
			failure(w, e)
			return
		}
	}
	restrictions, _, e := d.viewerRestrictions(r, p)
	if e != nil {
		failure(w, e)
		return
	}
	// Search is the most expensive read the server serves, and a client that
	// types fast will fire several in a second. Cap it per viewer as well as
	// globally, and say so plainly rather than letting the queue grow.
	if d.admission != nil {
		done, ok := d.admission.acquireSearch(identity.PersonalKey(p.Viewer))
		if !ok {
			w.Header().Set("Retry-After", "1")
			policyError(w, "search_busy")
			return
		}
		defer done()
	}
	var out catalog.SearchEnvelope
	e = d.compose(r.WithContext(ctx), restrictions, func(cat *catalog.Service) error {
		var composeErr error
		viewer := catalog.Viewer{Profile: identity.PersonalKey(p.Viewer), Fence: fence, Libraries: ids, Restrictions: restrictions}
		queryText, _, normalizeErr := catalog.NormalizeSearch(q.Get("q"))
		if normalizeErr != nil {
			return normalizeErr
		}
		out, composeErr = cat.Search(cat.Context(), catalog.SearchRequest{Viewer: viewer, ServerID: d.Identity.ID(), Profile: identity.PersonalKey(p.Viewer), ViewerFence: fence, Q: queryText, Group: q.Get("group"), Cursor: q.Get("cursor"), Sort: q.Get("sort"), Direction: q.Get("direction"), Limit: limit, AllLibraries: all, Libraries: ids, LibraryIDs: libraryIDs, Groups: groups, LiveTV: liveTV, LiveConfigured: liveConfigured, LiveLookup: d.searchLiveLookup(p, fence, restrictions, queryText, func() string {
			if q.Get("direction") != "" {
				return q.Get("direction")
			}
			if q.Get("sort") == "title" {
				return "asc"
			}
			return "desc"
		}()), Record: q.Get("record") == "1", Restrictions: restrictions})
		return composeErr
	})
	if e != nil {
		if errors.Is(e, context.DeadlineExceeded) || errors.Is(e, context.Canceled) {
			policyError(w, "search_unavailable")
			return
		}
		failure(w, e)
		return
	}
	_, _, _, _, _, after, e := d.searchScope(r)
	if e != nil {
		failure(w, e)
		return
	}
	if after != fence {
		failure(w, catalog.ErrStaleContinuation)
		return
	}
	// History records only a committed search (`record=1`, sent when the viewer
	// submits or opens a result), never typeahead pages or continuations.
	out.Query.Recorded = false
	if q.Get("record") == "1" && q.Get("cursor") == "" {
		if e = d.Catalog.WithContext(r.Context()).RecordSearch(p.Viewer, q.Get("q")); e != nil {
			failure(w, e)
			return
		}
		out.Query.Recorded, _ = d.Catalog.WithContext(r.Context()).RemembersSearchHistory(p.Viewer)
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}

func (d Dependencies) searchDiscoveryHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	for k, values := range r.URL.Query() {
		if len(values) != 1 || (k != "cursor" && k != "limit") {
			failure(w, catalog.ErrSearchQuery)
			return
		}
	}
	limit := 12
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 24 {
			failure(w, catalog.ErrSearchQuery)
			return
		}
		limit = n
	}
	p, all, ids, live, _, fence, err := d.searchScope(r)
	if err != nil {
		failure(w, err)
		return
	}
	if all {
		ids, _, err = d.homeScopeFor(r, p)
		if err != nil {
			failure(w, err)
			return
		}
	}
	restrictions, _, err := d.viewerRestrictions(r, p)
	if err != nil {
		failure(w, err)
		return
	}
	if d.admission != nil {
		done, ok := d.admission.acquireSearch(identity.PersonalKey(p.Viewer))
		if !ok {
			w.Header().Set("Retry-After", "1")
			policyError(w, "search_busy")
			return
		}
		defer done()
	}
	var out catalog.SearchDiscovery
	err = d.compose(r, restrictions, func(cat *catalog.Service) error {
		var e error
		out, e = cat.SearchDiscovery(catalog.Viewer{Profile: identity.PersonalKey(p.Viewer), Fence: fence, Libraries: ids, Restrictions: restrictions}, d.Identity.ID(), r.URL.Query().Get("cursor"), limit)
		return e
	})
	if err != nil {
		failure(w, err)
		return
	}
	if live && r.URL.Query().Get("cursor") == "" {
		out.OnNow, err = d.searchOnNow(r.Context(), p, fence, restrictions)
		if err != nil {
			failure(w, err)
			return
		}
	}
	_, _, _, _, _, after, err := d.searchScope(r)
	if err != nil {
		failure(w, err)
		return
	}
	if after != fence {
		failure(w, catalog.ErrStaleContinuation)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	write(w, 200, out)
}
