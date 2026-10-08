package httpapi

import (
	"net/http"
	"portico.local/server/internal/identity"
	"strconv"
)

func (d Dependencies) audioRoutes(mux *http.ServeMux) {
	d.listeningRoutes(mux)
	mux.HandleFunc("GET /v1/libraries/{id}/artists", func(w http.ResponseWriter, r *http.Request) {
		p, err := d.principal(r)
		if err == nil {
			err = d.allowedLibrary(r.Context(), p, r.PathValue("id"))
		}
		if err != nil {
			failure(w, err)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		tag := gridTag(r, p, gridTagBucket)
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
	mux.HandleFunc("GET /v1/libraries/{id}/books", func(w http.ResponseWriter, r *http.Request) {
		p, err := d.principal(r)
		if err == nil {
			err = d.allowedLibrary(r.Context(), p, r.PathValue("id"))
		}
		if err != nil {
			failure(w, err)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		tag := gridTag(r, p, gridTagBucket)
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
		mux.HandleFunc(route.pattern, func(w http.ResponseWriter, r *http.Request) {
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
