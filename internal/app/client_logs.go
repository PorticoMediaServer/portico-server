package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/redaction"
)

var (
	clientLogBearerPattern     = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[a-z0-9._~+/=-]{8,}`)
	clientLogJWTLikePattern    = regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{8,}\b`)
	clientLogSecretPairPattern = regexp.MustCompile(`(?i)\b(access_token|refresh_token|id_token|media_grant|download_grant|api[_-]?key|password|secret|token)=([^&\s]+)`)
)

func (s *Server) handleClientLogs(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if !s.clientLogUploadsEnabled() {
		writeError(w, http.StatusForbidden, "client_logs_disabled", "Client log upload is disabled in Troubleshooting settings.")
		return
	}
	if r.ContentLength > 512<<10 {
		writeError(w, http.StatusRequestEntityTooLarge, "client_logs_too_large", "Client diagnostics exceeded the upload size limit.")
		return
	}
	var req ClientLogUploadRequest
	if !decodeJSONLimit(w, r, &req, 512<<10) {
		return
	}
	if err := validateClientLogUpload(req); err != nil {
		writeError(w, http.StatusBadRequest, "client_logs_invalid", err.Error())
		return
	}
	origin := clientIPFromRequest(r) + "|" + truncateLogField(r.Header.Get("Origin"), 160)
	accepted := s.recordClientLogUploadForOrigin(user, req, origin)
	writeJSON(w, http.StatusCreated, ClientLogUploadResponse{OK: true, Accepted: accepted})
}

func (s *Server) clientLogUploadsEnabled() bool {
	settings, err := s.loadSettings()
	if err != nil {
		return false
	}
	group, _ := settings["troubleshooting"].(map[string]any)
	return settingBool(group, "clientLogUploads", false)
}

func (s *Server) recordClientLogUpload(user User, req ClientLogUploadRequest) int {
	return s.recordClientLogUploadForOrigin(user, req, "internal")
}

func validateClientLogUpload(req ClientLogUploadRequest) error {
	if len(req.Entries) == 0 {
		return fmt.Errorf("at least one diagnostic entry is required")
	}
	if len(req.Entries) > 50 {
		return fmt.Errorf("at most 50 diagnostic entries are accepted")
	}
	if len(req.Device) > 160 || len(req.App) > 160 {
		return fmt.Errorf("client and app identifiers are too long")
	}
	for _, entry := range req.Entries {
		if len(entry.Message) > 4000 {
			return fmt.Errorf("diagnostic messages are limited to 4000 bytes")
		}
		if len(entry.Context) > 20 {
			return fmt.Errorf("diagnostic context is limited to 20 fields")
		}
		if len(entry.Timestamp) > 64 {
			return fmt.Errorf("diagnostic timestamps are too long")
		}
		if strings.TrimSpace(entry.Timestamp) != "" {
			if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(entry.Timestamp)); err != nil {
				return fmt.Errorf("diagnostic timestamps must use RFC 3339")
			}
		}
		for key, value := range entry.Context {
			if len(key) > 80 || len(value) > 1000 {
				return fmt.Errorf("diagnostic context fields are too large")
			}
		}
	}
	return nil
}

func (s *Server) recordClientLogUploadForOrigin(user User, req ClientLogUploadRequest, origin string) int {
	device := truncateLogField(s.sanitizeDiagnosticText(req.Device, 80), 80)
	app := truncateLogField(s.sanitizeDiagnosticText(req.App, 80), 80)
	origin = truncateLogField(s.sanitizeDiagnosticText(origin, 200), 200)
	if device == "" {
		device = "unknown"
	}
	if app == "" {
		app = "unknown"
	}
	entries := make([]clientDiagnosticRecord, 0, len(req.Entries))
	totalBytes := 0
	for _, entry := range req.Entries {
		clientAt := truncateLogField(s.sanitizeDiagnosticText(entry.Timestamp, 64), 64)
		message := truncateLogField(s.sanitizeDiagnosticText(redactClientLogText(entry.Message), 500), 500)
		if message == "" {
			continue
		}
		level := normalizeClientLogLevel(entry.Level)
		fields := map[string]string{
			"device": device,
			"app":    app,
			"source": "client",
		}
		if clientAt != "" {
			fields["clientTime"] = clientAt
		}
		for key, value := range sanitizeClientLogContext(entry.Context) {
			fields["client."+key] = value
		}
		fields = s.sanitizeDiagnosticFields(fields)
		record := clientDiagnosticRecord{
			id:        randomID("client-log"),
			accountID: firstNonEmpty(accountIDForUser(user), user.ID),
			device:    device,
			app:       app,
			origin:    origin,
			level:     level,
			message:   "Client log: " + message,
			fields:    fields,
			clientAt:  clientAt,
		}
		record.size = clientDiagnosticRecordSize(record)
		totalBytes += int(record.size)
		entries = append(entries, record)
	}
	if len(entries) == 0 {
		return 0
	}
	// Device and Origin are uploader-controlled labels. Rate limiting by either
	// lets one authenticated account rotate labels and evade the write ceiling.
	rateKey := firstNonEmpty(accountIDForUser(user), user.ID)
	if !s.allowClientDiagnostic(rateKey, len(entries), totalBytes) {
		s.diagnosticClientDropped.Add(uint64(len(entries)))
		return 0
	}
	accepted := 0
	for _, record := range entries {
		if s.enqueueClientDiagnostic(record) {
			accepted++
		}
	}
	return accepted
}

func clientDiagnosticRecordSize(record clientDiagnosticRecord) int64 {
	raw, err := json.Marshal(struct {
		ID        string            `json:"id"`
		AccountID string            `json:"accountId"`
		Device    string            `json:"device"`
		App       string            `json:"app"`
		Origin    string            `json:"origin"`
		Level     string            `json:"level"`
		Message   string            `json:"message"`
		Fields    map[string]string `json:"fields,omitempty"`
		ClientAt  string            `json:"clientAt,omitempty"`
	}{
		ID: record.id, AccountID: record.accountID, Device: record.device, App: record.app,
		Origin: record.origin, Level: record.level, Message: record.message,
		Fields: record.fields, ClientAt: record.clientAt,
	})
	if err != nil {
		return 0
	}
	return int64(len(raw))
}

func normalizeClientLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug", "info", "warn", "error":
		return strings.ToLower(strings.TrimSpace(level))
	default:
		return "info"
	}
}

func sanitizeClientLogContext(context map[string]string) map[string]string {
	clean := map[string]string{}
	for key, value := range context {
		if len(clean) >= 20 {
			break
		}
		key = strings.TrimSpace(key)
		lower := strings.ToLower(key)
		if key == "" {
			continue
		}
		if strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "authorization") || strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") {
			clean[truncateLogField(key, 40)] = "[redacted]"
			continue
		}
		clean[truncateLogField(key, 40)] = truncateLogField(redactClientLogText(value), 200)
	}
	return clean
}

func redactClientLogText(value string) string {
	value = redaction.RedactPorticoCredentials(value)
	value = clientLogBearerPattern.ReplaceAllString(value, "$1 [redacted]")
	value = clientLogJWTLikePattern.ReplaceAllString(value, "[redacted-jwt]")
	value = clientLogSecretPairPattern.ReplaceAllString(value, "$1=[redacted]")
	return value
}

func truncateLogField(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
