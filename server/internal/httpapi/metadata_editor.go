package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/metadata"
	"time"
)

// The upload body carries one image plus a small multipart envelope. The reader
// is bounded before anything is buffered, so an oversized upload never reaches
// the decoder.
const artworkUploadEnvelope = 1 << 20

func (d Dependencies) metadataEditorRoutes(mux *http.ServeMux) {
	rate := &personalLimiter{}
	mux.HandleFunc("POST /v1/metadata/bulk", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		p, err := d.manualMetadataOwner(ctx, r)
		if err != nil {
			failure(w, err)
			return
		}
		if d.Metadata == nil {
			policyError(w, "metadata_unavailable")
			return
		}
		if r.URL.RawQuery != "" {
			repairFailure(w, metadata.ErrRepairInput)
			return
		}
		if !rate.allow(viewerScope(p)) {
			write(w, 429, map[string]any{"error": map[string]any{"code": "rate_limited", "message": "Too many metadata changes.", "retryable": true}})
			return
		}
		var body metadata.BulkEdit
		if err = decodeBulk(w, r, &body); err != nil {
			repairFailure(w, metadata.ErrRepairInput)
			return
		}
		authorize := func(tx *sql.Tx) error { return d.manualMetadataAuthorize(ctx, tx, p) }
		out, err := d.Metadata.BulkEdit(ctx, body, metadata.MBActor{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID}, authorize)
		if err != nil {
			repairFailure(w, err)
			return
		}
		if _, err = d.manualMetadataOwner(ctx, r); err != nil {
			failure(w, err)
			return
		}
		// Partial failure is a reported outcome, not a request failure: the caller
		// is told exactly which targets applied and which did not.
		write(w, 200, out)
	})
	mux.HandleFunc("POST /v1/metadata/{kind}/{id}/art/{role}/upload", func(w http.ResponseWriter, r *http.Request) {
		d.artworkUpload(w, r, rate)
	})
	mux.HandleFunc("DELETE /v1/metadata/{kind}/{id}/art/{role}/upload/{candidateId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		p, err := d.manualMetadataOwner(ctx, r)
		if err != nil {
			failure(w, err)
			return
		}
		if d.Metadata == nil {
			policyError(w, "metadata_unavailable")
			return
		}
		if !rate.allow(viewerScope(p)) {
			write(w, 429, map[string]any{"error": map[string]any{"code": "rate_limited", "message": "Too many metadata changes.", "retryable": true}})
			return
		}
		target := metadata.RepairTarget{Kind: r.PathValue("kind"), ID: r.PathValue("id")}
		authorize := func(tx *sql.Tx) error { return d.manualMetadataAuthorize(ctx, tx, p) }
		out, err := d.Metadata.DeleteUploadedArtwork(ctx, target, r.PathValue("role"), r.PathValue("candidateId"), r.URL.Query().Get("expectedRevision"), metadata.MBActor{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID}, authorize)
		if err != nil {
			repairFailure(w, err)
			return
		}
		d.writeRepairState(w, r, ctx, p, out)
	})
}

func (d Dependencies) artworkUpload(w http.ResponseWriter, r *http.Request, rate *personalLimiter) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	p, err := d.manualMetadataOwner(ctx, r)
	if err != nil {
		failure(w, err)
		return
	}
	if d.Metadata == nil {
		policyError(w, "metadata_unavailable")
		return
	}
	if !rate.allow(viewerScope(p)) {
		write(w, 429, map[string]any{"error": map[string]any{"code": "rate_limited", "message": "Too many metadata changes.", "retryable": true}})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, metadata.UploadBytes+artworkUploadEnvelope)
	if err = r.ParseMultipartForm(artworkUploadEnvelope); err != nil {
		uploadFailure(w, metadata.ErrArtworkUpload)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, _, err := r.FormFile("file")
	if err != nil {
		uploadFailure(w, metadata.ErrArtworkUpload)
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, metadata.UploadBytes+1))
	if err != nil || len(raw) > metadata.UploadBytes {
		uploadFailure(w, metadata.ErrArtworkUpload)
		return
	}
	target := metadata.RepairTarget{Kind: r.PathValue("kind"), ID: r.PathValue("id")}
	authorize := func(tx *sql.Tx) error { return d.manualMetadataAuthorize(ctx, tx, p) }
	out, err := d.Metadata.UploadArtwork(ctx, target, r.PathValue("role"), r.FormValue("subject"), r.FormValue("expectedRevision"), raw, metadata.MBActor{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID}, authorize)
	if err != nil {
		uploadFailure(w, err)
		return
	}
	d.writeRepairState(w, r, ctx, p, out)
}

func (d Dependencies) writeRepairState(w http.ResponseWriter, r *http.Request, ctx context.Context, p identity.Principal, out metadata.RepairState) {
	if _, err := d.manualMetadataOwner(ctx, r); err != nil {
		failure(w, err)
		return
	}
	out.Server = d.Identity.ID()
	out.ViewerFence = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", p.Hash, p.Epoch))))
	write(w, 200, out)
}

func uploadFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, metadata.ErrArtworkUpload) {
		write(w, 415, map[string]any{"error": map[string]any{"code": "unsupported_artwork", "message": "Upload a JPEG, PNG or WebP image no larger than 10 MiB.", "retryable": false}})
		return
	}
	repairFailure(w, err)
}

// The bulk envelope is larger than a single edit: two hundred targets carry two
// hundred revision fences. It is still strictly bounded and strictly typed.
func decodeBulk(w http.ResponseWriter, r *http.Request, v *metadata.BulkEdit) error {
	r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return metadata.ErrRepairInput
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return metadata.ErrRepairInput
	}
	return nil
}
