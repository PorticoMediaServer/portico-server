package app

import (
	"net/http"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/app/productlanguage"
)

// writeProductError emits the standard RFC 9457-shaped Portico problem plus a
// stable Product Language identifier. Subsystems should use this path whenever
// their errors are part of a first-party client experience.
func writeProductError(w http.ResponseWriter, status int, code, detail string) {
	writeProductErrorWithDetails(w, status, code, detail, nil)
}

func writeProductErrorWithDetails(w http.ResponseWriter, status int, code, detail string, details map[string]any) {
	requestID := strings.TrimSpace(w.Header().Get(requestIDHeader))
	if requestID == "" {
		requestID = randomID("req")
		w.Header().Set(requestIDHeader, requestID)
	}
	payload := map[string]any{
		"type":      "https://portico.media/problems/" + strings.ReplaceAll(code, "_", "-"),
		"title":     http.StatusText(status),
		"status":    status,
		"code":      code,
		"detail":    detail,
		"requestId": requestID,
	}
	if messageID, ok := productlanguage.ProblemMessageID(code, status); ok {
		payload["messageId"] = messageID
	}
	if len(details) > 0 {
		payload["details"] = details
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	writeJSON(w, status, payload)
}
