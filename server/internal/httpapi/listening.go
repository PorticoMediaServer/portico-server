package httpapi

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"strconv"
)

func (d Dependencies) listeningRoutes(mux *http.ServeMux) {
	envelope := func(p identity.Principal, data any) any {
		return map[string]any{"scope": map[string]string{"serverId": d.Identity.ID(), "authority": p.Authority, "accountId": p.AccountID, "profileId": p.ProfileID}, "data": data}
	}
	mux.HandleFunc("GET /v1/listening/preferences", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		data, e := d.Catalog.WithContext(r.Context()).ListeningPreferences(p.Viewer, d.consoleAuthority(p, false))
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, envelope(p, data))
	})
	mux.HandleFunc("PUT /v1/listening/preferences", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			Revision       *int64   `json:"revision"`
			MusicRate      *float64 `json:"musicRate"`
			BookRate       *float64 `json:"bookRate"`
			AutoplayNext   *bool    `json:"autoplayNext"`
			PassoutMinutes *int     `json:"passoutMinutes"`
		}
		if e = strictDecode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		if body.Revision == nil || body.MusicRate == nil || body.BookRate == nil || body.AutoplayNext == nil || body.PassoutMinutes == nil {
			failure(w, errStrictJSON)
			return
		}
		data := catalog.ListeningPreferences{Revision: *body.Revision, MusicRate: *body.MusicRate, BookRate: *body.BookRate, AutoplayNext: *body.AutoplayNext, PassoutMinutes: *body.PassoutMinutes}
		data, e = d.Catalog.WithContext(r.Context()).SaveListeningPreferences(p.Viewer, data, d.consoleAuthority(p, false))
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, envelope(p, data))
	})
	mux.HandleFunc("GET /v1/libraries/{id}/listening/selection", func(w http.ResponseWriter, r *http.Request) {
		p, e := d.principal(r)
		if e == nil {
			e = d.allowedLibrary(r.Context(), p, r.PathValue("id"))
		}
		if e != nil {
			failure(w, e)
			return
		}
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		target := catalog.ListeningTarget{LibraryID: r.PathValue("id"), Kind: q.Get("kind"), ID: q.Get("entityId")}
		var policy, restriction int64
		if p.Authority != "local" {
			if e = dbwork.QueryRow(r.Context(), d.DB, `SELECT COALESCE((SELECT revision FROM policy WHERE server_id=?),0),COALESCE((SELECT revision FROM restrictions WHERE profile_id=?),0)`, d.Identity.ID(), p.ProfileID).Scan(&policy, &restriction); e != nil {
				failure(w, e)
				return
			}
		}
		fence := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%d", p.Hash, p.Epoch, policy, restriction))))
		viewer, e := d.catalogViewer(r, p, []string{target.LibraryID}, fence)
		if e != nil {
			failure(w, e)
			return
		}
		data, e := d.Catalog.WithContext(r.Context()).ListeningSelection(catalog.ContentRequest{Viewer: viewer, ServerID: d.Identity.ID(), Library: target.LibraryID, Profile: identity.PersonalKey(p.Viewer), ViewerFence: fence, Cursor: q.Get("cursor"), Limit: limit}, target, q.Get("mode"), q.Get("seed"), q.Get("resume") == "true", q.Get("startItem"))
		if e == nil {
			e = d.allowedLibrary(r.Context(), p, target.LibraryID)
		}
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, envelope(p, data))
	})
	mux.HandleFunc("GET /v1/items/{id}/listening", func(w http.ResponseWriter, r *http.Request) {
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
		data, e := d.Catalog.WithContext(r.Context()).ListeningItem(viewer, r.PathValue("id"))
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, envelope(p, data))
	})
}
