package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"portico.local/hosted/internal/appstore"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
)

// accountIdentityRoutes adds the workstream-H hosted account surface: the device
// list, account-level region and privacy preferences, the combined account
// document (security state, preferences and invitation status/expiry), and the
// Apple App Store Server Notifications v2 intake endpoint.
func (h *Handler) accountIdentityRoutes(mux *http.ServeMux) {
	// The notification intake is registered independently of h.Security: it is
	// gated by its own key set, and a deployment may verify notifications before
	// it has anything else configured.
	h.appStoreRoutes(mux)
	if h.Security == nil {
		return
	}
	mux.HandleFunc("GET /v1/account", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		out, e := h.Security.Document(r.Context(), bearer(r))
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
	mux.HandleFunc("GET /v1/account/devices", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		out, e := h.Security.Devices(r.Context(), bearer(r))
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
	mux.HandleFunc("GET /v1/account/preferences", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		out, e := h.Security.Preferences(r.Context(), bearer(r))
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
	mux.HandleFunc("PUT /v1/account/preferences", h.account(func(w http.ResponseWriter, r *http.Request, _ identity.Viewer) {
		var body postgres.AccountPreferenceEdit
		if !decode(w, r, &body) {
			return
		}
		out, e := h.Security.SetPreferences(r.Context(), bearer(r), body)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
}

// appStoreRoutes registers the Apple server-to-server notification intake.
//
// The endpoint is unauthenticated in the usual sense: Apple sends no bearer
// token, no Origin and no useful source address, so the JWS signature *is* the
// authentication. Everything therefore hangs on verification in
// internal/appstore, and nothing is written before it passes.
//
// The handler always answers 200 once it has decided, even for a refusal Apple
// should not retry, except for a genuine server-side failure. Apple retries a
// non-2xx for days; a notification this deployment will never accept (wrong
// bundle, wrong environment, bad signature) is not made acceptable by retrying,
// and answering 200 keeps a misconfiguration from becoming a sustained flood.
func (h *Handler) appStoreRoutes(mux *http.ServeMux) {
	if h.AppStore == nil {
		return
	}
	mux.HandleFunc("POST /v1/appstore/notifications", func(w http.ResponseWriter, r *http.Request) {
		// Apple's POST is not subject to the Origin fence (it sends none), so the
		// per-peer attempt limit is the only thing between this route and a flood.
		if !h.allowAuth(r) {
			fail(w, 429, "rate_limited", "Too many notifications from this peer.", true)
			return
		}
		body, e := io.ReadAll(io.LimitReader(r.Body, appstore.MaxPayloadBytes+1))
		if e != nil || len(body) > appstore.MaxPayloadBytes {
			h.recordAppStoreFailure(r, "malformed")
			write(w, 200, AppStoreReceipt{Accepted: false, Reason: "malformed"})
			return
		}
		var envelope AppStoreNotificationRequest
		if json.Unmarshal(body, &envelope) != nil || envelope.SignedPayload == "" {
			h.recordAppStoreFailure(r, "malformed")
			write(w, 200, AppStoreReceipt{Accepted: false, Reason: "malformed"})
			return
		}
		notification, e := h.AppStore.Verify(envelope.SignedPayload)
		if e != nil {
			// The reason is recorded; the refused bytes never are.
			reason := appstore.FailureReason(e)
			h.recordAppStoreFailure(r, reason)
			write(w, 200, AppStoreReceipt{Accepted: false, Reason: reason})
			return
		}
		claims, _, e := h.AppStore.Transaction(notification)
		if e != nil {
			reason := appstore.FailureReason(e)
			h.recordAppStoreFailure(r, reason)
			write(w, 200, AppStoreReceipt{Accepted: false, Reason: reason})
			return
		}
		if h.Security == nil {
			// Verification succeeded but there is nowhere to record it. Apple should
			// retry: the deployment is incompletely configured, not the payload wrong.
			fail(w, 503, "unavailable", "Notification storage is unavailable.", true)
			return
		}
		if e = h.Security.RecordAppStoreNotification(r.Context(), notification, claims); e != nil {
			// A storage failure is the one case Apple should retry.
			failure(w, e)
			return
		}
		write(w, 200, AppStoreReceipt{Accepted: true})
	})
}

func (h *Handler) recordAppStoreFailure(r *http.Request, reason string) {
	if h.Security == nil {
		return
	}
	// A failure record must not turn a refused notification into a failed request.
	_ = h.Security.RecordAppStoreFailure(r.Context(), reason)
}
