package httpapi

import (
	"net/http"
	"portico.local/hosted/internal/device"
	"portico.local/hosted/internal/identity"
)

func (h *Handler) deviceRoutes(mux *http.ServeMux) {
	if h.Devices == nil {
		return
	}
	mux.HandleFunc("POST /v1/device-authorizations", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowCredentialLookup(w, r, "device") {
			return
		}
		var q device.Start
		if !decode(w, r, &q) {
			return
		}
		if q.SessionMode == "" {
			q.SessionMode = "native"
		}
		if !h.validSessionMode(w, r, q.SessionMode) {
			return
		}
		out, e := h.Devices.Start(r.Context(), peer(r), q)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 201, out)
	})
	mux.HandleFunc("POST /v1/device-authorizations/review", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) {
		var q UserCodeRequest
		if !decode(w, r, &q) {
			return
		}
		out, e := h.Devices.Review(r.Context(), v.Account.ID, peer(r), q.UserCode)
		if e != nil {
			failure(w, e)
			return
		}
		write(w, 200, out)
	}))
	mux.HandleFunc("POST /v1/device-authorizations/decision", h.account(func(w http.ResponseWriter, r *http.Request, v identity.Viewer) {
		var q DeviceDecisionRequest
		if !decode(w, r, &q) {
			return
		}
		if e := h.Devices.Decide(r.Context(), v.Account.ID, peer(r), q.RequestID, q.UserCode, q.Decision); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	}))
	mux.HandleFunc("POST /v1/device-authorizations/token", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowCredentialLookup(w, r, "device") {
			return
		}
		var q DeviceTokenRequest
		if !decode(w, r, &q) {
			return
		}
		if q.SessionMode == "" {
			q.SessionMode = "native"
		}
		if !h.validSessionMode(w, r, q.SessionMode) {
			return
		}
		out, e := h.Devices.PollMode(r.Context(), q.DeviceCode, q.SessionMode)
		if e != nil {
			failure(w, e)
			return
		}
		h.sessionResponse(w, r, 200, out)
	})
	mux.HandleFunc("POST /v1/tv-authorizations/token", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowCredentialLookup(w, r, "device") {
			return
		}
		var q DeviceCodeRequest
		if !decode(w, r, &q) {
			return
		}
		out, e := h.Devices.PollTV(r.Context(), q.DeviceCode)
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/device-authorizations/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !h.allowCredentialLookup(w, r, "device") {
			return
		}
		var q DeviceCodeRequest
		if !decode(w, r, &q) {
			return
		}
		if e := h.Devices.Cancel(r.Context(), q.DeviceCode); e != nil {
			failure(w, e)
			return
		}
		w.WriteHeader(204)
	})
}
