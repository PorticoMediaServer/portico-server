package httpapi

import (
	"encoding/json"
	"net/http"
	"portico.local/hosted/internal/security"
	"time"
)

type responseStatus struct {
	http.ResponseWriter
	status int
}

func (w *responseStatus) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *responseStatus) WriteHeader(n int) {
	if w.status == 0 {
		w.status = n
	}
	w.ResponseWriter.WriteHeader(n)
}
func (w *responseStatus) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}

// Route templates are resolved before handling. Never log URLs, query strings,
// request bodies, addresses, Authorization headers or identity values.
func (h *Handler) observe(w http.ResponseWriter, r *http.Request, mux *http.ServeMux) (http.ResponseWriter, func()) {
	start := time.Now()
	id := security.RandomID("req_")
	w.Header().Set("X-Request-ID", id)
	out := &responseStatus{ResponseWriter: w}
	_, route := mux.Handler(r)
	if route == "" {
		route = "unmatched"
	}
	return out, func() {
		status := out.status
		if status == 0 {
			status = 200
		}
		if status == 429 {
			h.rejections.Add(1)
		}
		h.requests.Add(1)
		if h.AccessLog != nil {
			raw, _ := json.Marshal(map[string]any{"requestId": id, "route": route, "status": status, "durationMs": time.Since(start).Milliseconds(), "principalClass": principalClass(r)})
			h.AccessLog(string(raw))
		}
	}
}
func principalClass(r *http.Request) string {
	if r.Header.Get("Authorization") != "" {
		return "bearer"
	}
	return "anonymous"
}
