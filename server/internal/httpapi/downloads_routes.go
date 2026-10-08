package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"portico.local/server/internal/downloads"
	"portico.local/server/internal/identity"
)

// Offline download routes. Everything a first-party client does to take media
// offline is here: preparing an artifact, reading the menu of qualities and
// their sizes, taking a short-lived transfer grant, holding a signed receipt
// that permits playback with no server in reach, handing back the progress it
// recorded while it was away, and reading what the store is holding.

// downloadFailure maps this area's errors before falling back to the shared
// mapping, so a download refusal carries a code a client can act on.
func downloadFailure(w http.ResponseWriter, e error) {
	status, code, retryable := 0, "", false
	switch {
	case errors.Is(e, downloads.ErrInput):
		status, code = 400, "invalid_request"
	case errors.Is(e, downloads.ErrNotFound):
		status, code = 404, "not_found"
	case errors.Is(e, downloads.ErrConflict):
		status, code = 409, "download_conflict"
	case errors.Is(e, downloads.ErrStorageFull):
		status, code = 409, "storage_full"
	case errors.Is(e, downloads.ErrPolicy):
		status, code = 403, "downloads_not_allowed"
	case errors.Is(e, downloads.ErrNotReady):
		status, code, retryable = 409, "download_not_ready", true
	case errors.Is(e, downloads.ErrGrant):
		status, code = 401, "invalid_download_grant"
	case errors.Is(e, downloads.ErrReceipt):
		status, code = 400, "invalid_download_receipt"
	case errors.Is(e, downloads.ErrCapacity):
		status, code, retryable = 429, "download_capacity", true
		w.Header().Set("Retry-After", "30")
	}
	if status == 0 {
		failure(w, e)
		return
	}
	write(w, status, map[string]any{"error": map[string]any{"code": code, "message": publicErrorMessage(code), "retryable": retryable}})
}

// downloadViewer authenticates and answers whether this viewer may order the
// conversions an optimized rung needs. Conversions stay owner-authorized in
// prepared media; downloads never widens that.
func (d Dependencies) downloadViewer(r *http.Request) (identity.Principal, bool, error) {
	p, e := d.principal(r)
	if e != nil {
		return p, false, e
	}
	return p, p.Role == "owner" && (p.Authority == "local" || p.Authority == "hosted"), nil
}

func (d Dependencies) downloadsReady(w http.ResponseWriter) bool {
	if d.Downloads == nil {
		write(w, 503, map[string]any{"error": map[string]any{"code": "downloads_unavailable", "message": "Offline downloads are not configured on this server.", "retryable": true}})
		return false
	}
	return true
}

func (d Dependencies) downloadRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/items/{id}/download-options", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, owner, e := d.downloadViewer(r)
		if e == nil {
			e = d.itemAccess(r.Context(), p, r.PathValue("id"))
		}
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.Options(r.Context(), p, r.PathValue("id"), owner)
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/downloads/preparations", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body downloads.Request
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		// The guard runs inside the admitting transaction, so it reads through
		// that transaction rather than opening a second one.
		out, e := d.Downloads.Submit(r.Context(), p, body, func(tx *sql.Tx, library string) error {
			return d.allowedLibraryTx(r.Context(), p, library, tx)
		})
		if e != nil {
			downloadFailure(w, e)
			return
		}
		status := 201
		if out.Duplicate {
			status = 200
		}
		write(w, status, out)
	})
	mux.HandleFunc("GET /v1/downloads/preparations", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, owner, e := d.downloadViewer(r)
		if e != nil {
			failure(w, e)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		query := downloads.ListQuery{State: r.URL.Query().Get("state"), ProfileID: r.URL.Query().Get("profileId"), Cursor: r.URL.Query().Get("cursor"), Limit: limit}
		out, e := d.Downloads.List(r.Context(), p, query, owner)
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/downloads/preparations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.Get(r.Context(), p, r.PathValue("id"))
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/downloads/preparations/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body downloads.Command
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.Act(r.Context(), p, r.PathValue("id"), body)
		if e != nil {
			// A fence failure still carries the current row, so the client can
			// re-render without a second read.
			if errors.Is(e, downloads.ErrConflict) {
				write(w, 409, map[string]any{"error": map[string]any{"code": "download_conflict", "message": publicErrorMessage("download_conflict"), "retryable": false}, "preparation": out})
				return
			}
			downloadFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/downloads/preparations/{id}/grant", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			OperationID string `json:"operationId"`
		}
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.IssueGrant(r.Context(), p, r.PathValue("id"), body.OperationID)
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 201, out)
	})
	// The transfer itself. The grant is the whole authority here, exactly as it
	// is for playback media: the token names the viewer, and the same library
	// permission is rechecked before a byte is served.
	transfer := func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		out, e := d.Downloads.OpenGrant(r.Context(), r.PathValue("grant"))
		if e != nil {
			downloadFailure(w, e)
			return
		}
		defer out.Reader.Close()
		if e = d.allowedLibrary(r.Context(), identity.Principal{Viewer: out.Viewer}, out.LibraryID); e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", out.Artifact.ContentType)
		w.Header().Set("Content-Disposition", "attachment; filename=\""+out.Artifact.FileName+"\"")
		w.Header().Set("ETag", "\"sha256-"+out.Artifact.SHA256+"\"")
		w.Header().Set("X-Portico-Artifact-Sha256", out.Artifact.SHA256)
		body := withRollingDeadline(w)
		defer body.release()
		http.ServeContent(body, r, out.Artifact.FileName, out.Modified, out.Reader)
	}
	mux.HandleFunc("GET /v1/downloads/artifacts/{grant}", transfer)
	mux.HandleFunc("HEAD /v1/downloads/artifacts/{grant}", transfer)
	mux.HandleFunc("POST /v1/downloads/receipts", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			OperationID    string   `json:"operationId"`
			PreparationIDs []string `json:"preparationIds"`
		}
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.IssueReceipts(r.Context(), p, d.Identity.ID(), body.OperationID, body.PreparationIDs)
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 201, map[string]any{"results": out})
	})
	mux.HandleFunc("POST /v1/downloads/receipts/revalidate", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			OperationID string   `json:"operationId"`
			ReceiptIDs  []string `json:"receiptIds"`
		}
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.Revalidate(r.Context(), p, d.Identity.ID(), body.OperationID, body.ReceiptIDs)
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, map[string]any{"results": out})
	})
	mux.HandleFunc("POST /v1/downloads/receipts/revoke", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, owner, e := d.downloadViewer(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			OperationID string   `json:"operationId"`
			ReceiptIDs  []string `json:"receiptIds"`
			Reason      string   `json:"reason"`
			AllProfiles bool     `json:"allProfiles"`
		}
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.Revoke(r.Context(), p, body.OperationID, body.Reason, body.ReceiptIDs, owner && body.AllProfiles)
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, map[string]any{"results": out})
	})
	mux.HandleFunc("GET /v1/downloads/receipts/revocations", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, owner, e := d.downloadViewer(r)
		if e != nil {
			failure(w, e)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		out, e := d.Downloads.Revocations(r.Context(), p, r.URL.Query().Get("cursor"), limit, owner && r.URL.Query().Get("scope") == "server")
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/downloads/receipt-keys", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		if _, e := d.principal(r); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.ReceiptKeys(r.Context())
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, map[string]any{"keys": out})
	})
	mux.HandleFunc("POST /v1/downloads/progress", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, e := d.principal(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body struct {
			OperationID string                    `json:"operationId"`
			Entries     []downloads.ProgressEntry `json:"entries"`
		}
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.SyncProgress(r.Context(), p, body.OperationID, body.Entries)
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/downloads/usage", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, owner, e := d.downloadViewer(r)
		if e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.Usage(r.Context(), p, owner && r.URL.Query().Get("scope") == "server")
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/downloads/settings", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		if _, e := d.principal(r); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.Settings(r.Context())
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("PUT /v1/downloads/settings", func(w http.ResponseWriter, r *http.Request) {
		if !d.downloadsReady(w) {
			return
		}
		p, e := d.owner(r)
		if e != nil {
			failure(w, e)
			return
		}
		var body downloads.SettingsChange
		if e = decode(w, r, &body); e != nil {
			failure(w, e)
			return
		}
		out, e := d.Downloads.ApplySettings(r.Context(), p, body)
		if e != nil {
			downloadFailure(w, e)
			return
		}
		write(w, 200, out)
	})
}
