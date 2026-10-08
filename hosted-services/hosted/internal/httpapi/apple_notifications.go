package httpapi

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"portico.local/hosted/internal/oidclogin"
)

func (h *Handler) appleNotificationRoutes(mux *http.ServeMux) {
	if h.OIDC == nil || h.OIDC.Store == nil {
		return
	}
	p := h.OIDC.Providers["apple"]
	if p == nil || p.AppleNotifications == nil {
		return
	}
	mux.HandleFunc("POST /v1/identity/apple/notifications", func(w http.ResponseWriter, r *http.Request) {
		// This server-to-server request uses Apple's signature, never a browser cookie.
		if !h.allowAuth(r) {
			fail(w, 429, "rate_limited", "Try again shortly.", true)
			return
		}
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "application/json" {
			fail(w, 415, "invalid_content_type", "JSON is required.", false)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, oidclogin.AppleNotificationMaxBytes)
		var body AppleNotificationRequest
		decoder := json.NewDecoder(r.Body)
		if decoder.Decode(&body) != nil || decoder.Decode(&struct{}{}) != io.EOF {
			fail(w, 400, "invalid_notification", "Invalid notification.", false)
			return
		}
		event, err := p.AppleNotifications.Verify(r.Context(), body.Payload)
		if err != nil {
			fail(w, 401, "invalid_notification", "Invalid notification.", false)
			return
		}
		if err = h.OIDC.ApplyAppleNotification(r.Context(), event); err != nil {
			fail(w, 503, "notification_unavailable", "Try again shortly.", true)
			return
		}
		write(w, 200, AcceptedResponse{Accepted: true})
	})
}
