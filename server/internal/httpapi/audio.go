package httpapi

import (
	"net/http"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"strconv"
)

func (d Dependencies) audioRoutes(mux *http.ServeMux) {
	d.listeningRoutes(mux)
	d.httpHandle(mux, "GET /v1/items/{id}/similar", func(w http.ResponseWriter, r *http.Request) {
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
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		data, e := d.Catalog.WithContext(r.Context()).ListeningSimilar(viewer, r.PathValue("id"), limit)
		if e == nil {
			e = d.allowedLibrary(r.Context(), p, data.LibraryID)
		}
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, map[string]any{"scope": map[string]string{"serverId": d.Identity.ID(), "authority": p.Authority, "accountId": p.AccountID, "profileId": p.ProfileID}, "data": data})
	})

	d.httpHandle(mux, "GET /v1/books/{id}/timeline", func(w http.ResponseWriter, r *http.Request) {
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
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		var position *int64
		if q.Has("positionMs") {
			n, err := strconv.ParseInt(q.Get("positionMs"), 10, 64)
			if err != nil || n < 0 {
				failure(w, errStrictJSON)
				return
			}
			position = &n
		}
		data, e := d.Catalog.WithContext(r.Context()).BookTimeline(catalog.ContentRequest{Viewer: viewer, Profile: identity.PersonalKey(p.Viewer), ViewerFence: fence, Cursor: q.Get("cursor"), Limit: limit}, r.PathValue("id"), position)
		if e == nil {
			e = d.allowedLibrary(r.Context(), p, data.LibraryID)
		}
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, map[string]any{"scope": map[string]string{"serverId": d.Identity.ID(), "authority": p.Authority, "accountId": p.AccountID, "profileId": p.ProfileID}, "data": data})
	})

	d.httpHandle(mux, "GET /v1/libraries/{id}/artists", func(w http.ResponseWriter, r *http.Request) {
		p, err := d.principal(r)
		if err == nil {
			err = d.allowedLibrary(r.Context(), p, r.PathValue("id"))
		}
		if err != nil {
			failure(w, err)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		tag := d.gridTag(r, p, gridTagBucket)
		if gridNotModified(w, r, tag) {
			return
		}
		rows, next, err := d.Catalog.WithContext(r.Context()).Artists(r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
		if err != nil {
			failure(w, err)
			return
		}
		setGridTag(w, tag)
		write(w, 200, map[string]any{"artists": rows, "nextCursor": next})
	})
	d.httpHandle(mux, "GET /v1/libraries/{id}/books", func(w http.ResponseWriter, r *http.Request) {
		p, err := d.principal(r)
		if err == nil {
			err = d.allowedLibrary(r.Context(), p, r.PathValue("id"))
		}
		if err != nil {
			failure(w, err)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		tag := d.gridTag(r, p, gridTagBucket)
		if gridNotModified(w, r, tag) {
			return
		}
		rows, next, err := d.Catalog.WithContext(r.Context()).Books(identity.PersonalKey(p.Viewer), r.PathValue("id"), r.URL.Query().Get("cursor"), limit)
		if err != nil {
			failure(w, err)
			return
		}
		setGridTag(w, tag)
		write(w, 200, map[string]any{"books": rows, "nextCursor": next})
	})
	for _, route := range []struct{ pattern, kind, collection string }{{"GET /v1/artists/{id}/albums", "artist", "albums"}, {"GET /v1/albums/{id}/songs", "album", "songs"}, {"GET /v1/books/{id}/files", "book", "files"}, {"GET /v1/books/{id}/chapters", "book", "chapters"}} {
		d.httpHandle(mux, route.pattern, func(w http.ResponseWriter, r *http.Request) {
			p, err := d.principal(r)
			if err != nil {
				failure(w, err)
				return
			}
			library, err := d.Catalog.WithContext(r.Context()).LibraryForAudioEntity(route.kind, r.PathValue("id"))
			if err == nil {
				err = d.allowedLibrary(r.Context(), p, library)
			}
			if err != nil {
				failure(w, err)
				return
			}
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			cursor := r.URL.Query().Get("cursor")
			var rows any
			var next string
			switch route.collection {
			case "albums":
				rows, next, err = d.Catalog.WithContext(r.Context()).Albums(r.PathValue("id"), cursor, limit)
			case "chapters":
				rows, next, err = d.Catalog.WithContext(r.Context()).BookChapters(r.PathValue("id"), cursor, limit)
			default:
				rows, next, err = d.Catalog.WithContext(r.Context()).AudioItems(identity.PersonalKey(p.Viewer), r.PathValue("id"), route.kind, cursor, limit)
			}
			if err != nil {
				failure(w, err)
				return
			}
			write(w, 200, map[string]any{route.collection: rows, "nextCursor": next})
		})
	}
}
