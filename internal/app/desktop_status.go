package app

import (
	"net/http"
	"strings"
	"time"
)

type desktopStatusResponse struct {
	Server       string `json:"server"`
	RemoteAccess string `json:"remoteAccess"`
	RemoteLabel  string `json:"remoteLabel"`
	CheckedAt    string `json:"checkedAt"`
}

func (s *Server) handleDesktopStatus(w http.ResponseWriter, r *http.Request) {
	if !setupRequestFromLoopback(r) {
		writeError(w, http.StatusForbidden, "desktop_local_access_required", "Desktop status is available only on this computer.")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}

	settings, err := s.remoteAccessSettings()
	if err != nil {
		writeJSON(w, http.StatusOK, desktopStatusResponse{
			Server:       "running",
			RemoteAccess: "unknown",
			RemoteLabel:  "Status unavailable",
			CheckedAt:    time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	status, label := desktopRemoteAccessSummary(settings)
	writeJSON(w, http.StatusOK, desktopStatusResponse{
		Server:       "running",
		RemoteAccess: status,
		RemoteLabel:  label,
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
	})
}

func desktopRemoteAccessSummary(settings RemoteAccessSettings) (string, string) {
	if !settings.Enabled {
		return "off", "Off"
	}
	if settings.ServerID == "" || settings.ClaimStatus != "claimed" {
		return "account_required", "Portico Account not connected"
	}
	connectivity := remoteConnectivityStatus(settings)
	if connectivity.HostedServicesStatus == "unreachable" {
		return "hosted_unavailable", "Hosted Services unavailable"
	}
	switch strings.TrimSpace(connectivity.PublicRouteStatus) {
	case "public_reachable", "reachable", "hosted_enabled":
		return "available", "Direct access available"
	case "public_checking", "checking", "dns_synced":
		return "checking", "Checking direct access"
	case "public_unreachable", "public_http_failed", "public_tls_failed", "public_failed", "public_missing":
		return "unavailable", "Direct route unavailable"
	default:
		if strings.HasPrefix(strings.TrimSpace(connectivity.PublicRouteStatus), "repair_") {
			return "checking", "Checking direct access"
		}
		return "unknown", "Status pending"
	}
}
