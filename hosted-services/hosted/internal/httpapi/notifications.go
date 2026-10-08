package httpapi

import (
	"net/http"
	"net/url"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
)

func (h *Handler) notificationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/servers/{id}/events", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		if !h.securityReady(w) {
			return
		}
		var q postgres.ServerEvent
		if !decode(w, r, &q) {
			return
		}
		if e := h.Security.Store.ServerEvent(r.Context(), a, q); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, CodeResponse{Code: "server_event_applied"})
	}))
	// The account's in-app notification feed (no push, no alert email).
	mux.HandleFunc("GET /v1/account/notifications", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		if !h.securityReady(w) {
			return
		}
		out, e := h.Security.Notifications(r.Context(), bearer(r), r.URL.Query().Get("cursor"))
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
	mux.HandleFunc("GET /v1/account/notifications/{id}", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		if !h.securityReady(w) {
			return
		}
		out, e := h.Security.NotificationByID(r.Context(), bearer(r), r.PathValue("id"))
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
	mux.HandleFunc("POST /v1/account/notifications/read", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		if !h.securityReady(w) {
			return
		}
		var q NotificationReadRequest
		if !decode(w, r, &q) {
			return
		}
		if e := h.Security.MarkNotificationsRead(r.Context(), bearer(r), q.IDs, q.All); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, CodeResponse{Code: "notifications_read"})
	}))
	for _, method := range []string{"GET", "PUT"} {
		mux.HandleFunc(method+" /v1/account/notification-preferences", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
			if !h.securityReady(w) {
				return
			}
			var update *postgres.NotificationPreferences
			if r.Method == "PUT" {
				update = &postgres.NotificationPreferences{}
				if !decode(w, r, update) {
					return
				}
			}
			out, e := h.Security.NotificationPreferences(r.Context(), bearer(r), update)
			if e != nil {
				failure(w, e)
				return
			}
			write(w, 200, out)
		}))
	}
	mux.HandleFunc("POST /v1/servers/{id}/notifications/storage-nearly-full", h.server(func(w http.ResponseWriter, r *http.Request, a directory.ServerAuthority) {
		if !h.securityReady(w) {
			return
		}
		if e := h.Security.Store.ReportStorageWarning(r.Context(), a); e != nil {
			failure(w, e)
			return
		}
		write(w, 202, CodeResponse{Code: "storage_warning_accepted"})
	}))
}

// notificationQueryAllowed admits only ?cursor=<item id> on the feed.
func notificationQueryAllowed(r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Path != "/v1/account/notifications" || len(r.URL.RawQuery) > 256 {
		return false
	}
	values, e := url.ParseQuery(r.URL.RawQuery)
	return e == nil && len(values) == 1 && len(values["cursor"]) == 1 && len(values["cursor"][0]) <= 128
}
