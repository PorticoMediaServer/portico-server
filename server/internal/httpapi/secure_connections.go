package httpapi

import "net/http"

func (d Dependencies) allowTransport(w http.ResponseWriter, r *http.Request) bool {
	setting, err := d.connectionSetting(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"connection_policy_unavailable"}`))
		return false
	}
	if setting != nil && setting.secureConnections == "required" && !d.secureTransport(r) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusUpgradeRequired)
		_, _ = w.Write([]byte(`{"code":"secure_connection_required"}`))
		return false
	}
	return true
}
