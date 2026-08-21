package app

import (
	"net/http"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/app/productlanguage"
)

func (s *Server) handleProductLanguage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	locale := strings.TrimPrefix(r.URL.Path, "/api/product-language/")
	if locale == "" || strings.Contains(locale, "/") {
		writeError(w, http.StatusNotFound, "product_language_not_found", "The requested product language is not available.")
		return
	}
	payload, ok := productlanguage.Catalog(locale)
	if !ok {
		writeError(w, http.StatusNotFound, "product_language_not_found", "The requested product language is not available.")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}
