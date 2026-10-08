package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"time"
	"unicode/utf8"
)

const manualMetadataRequestBytes = 256 << 10

func decodeManualMetadata(w http.ResponseWriter, r *http.Request, value *catalog.ManualMetadataMutation) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, manualMetadataRequestBytes))
	if err != nil || !utf8.Valid(raw) {
		return catalog.ErrManualMetadataInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		return catalog.ErrManualMetadataInput
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return catalog.ErrManualMetadataInput
	}
	return nil
}

func (d Dependencies) manualMetadataOwner(ctx context.Context, r *http.Request) (identity.Principal, error) {
	return d.ownerContext(ctx, r)
}

// Keep the bounded direct-owner query while Hosted owners use current policy
// and token-family checks in the same metadata transaction.
func (d Dependencies) manualMetadataAuthorize(ctx context.Context, tx *sql.Tx, p identity.Principal) error {
	if p.Authority == "local" {
		return manualMetadataAuthorize(ctx, tx, p)
	}
	return d.ownerAuthorityTx(ctx, tx, p)
}

func (d Dependencies) manualMetadataRoutes(mux *http.ServeMux) {
	for _, method := range []string{"GET", "PATCH"} {
		mux.HandleFunc(method+" /v1/items/{id}/metadata/manual", d.manualMetadataHandler(method, d.Catalog.ManualMetadata))
	}
}

// This endpoint admits only local owners. The joined predicates preserve the
// resourceAuthorize session/account rules plus the owner role, with cancellable SQL.
func manualMetadataAuthorize(ctx context.Context, tx *sql.Tx, p identity.Principal) error {
	if p.Authority != "local" || p.Role != "owner" {
		return identity.ErrForbidden
	}
	var count int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM authorization_access s JOIN accounts a ON a.id=s.account_id AND a.profile_id=s.profile_id AND a.epoch=s.epoch WHERE s.hash=? AND s.authority='local' AND s.role='owner' AND s.account_id=? AND s.profile_id=? AND s.epoch=? AND s.revoked=0 AND s.expires_at>?`, p.Hash, p.AccountID, p.ProfileID, p.Epoch, time.Now().UTC().Format(time.RFC3339)).Scan(&count)
	if err != nil {
		return err
	}
	if count != 1 {
		return identity.ErrUnauthorized
	}
	return nil
}

func (d Dependencies) manualMetadataHandler(method string, read func(context.Context, string, string, string, func(*sql.Tx) error) (catalog.ManualMetadata, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		p, err := d.manualMetadataOwner(ctx, r)
		if err != nil {
			failure(w, err)
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, catalog.ErrManualMetadataInput)
			return
		}
		fence := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", p.Hash, p.Epoch))))
		authorize := func(tx *sql.Tx) error { return d.manualMetadataAuthorize(ctx, tx, p) }
		var out catalog.ManualMetadata
		if method == "GET" {
			out, err = read(ctx, d.Identity.ID(), fence, r.PathValue("id"), authorize)
		} else {
			var body catalog.ManualMetadataMutation
			if err = decodeManualMetadata(w, r, &body); err != nil {
				failure(w, err)
				return
			}
			out, err = d.Catalog.SaveManualMetadata(ctx, d.Identity.ID(), fence, r.PathValue("id"), p.AccountID, body, authorize)
		}
		if errors.Is(err, catalog.ErrManualMetadataConflict) {
			write(w, 409, map[string]any{"error": map[string]any{"code": "manual_metadata_conflict", "message": publicErrorMessage("manual_metadata_conflict"), "retryable": false}})
			return
		}
		if err != nil {
			if errors.Is(err, catalog.ErrManualMetadataProjection) {
				write(w, 503, map[string]any{"error": map[string]any{"code": "metadata_edit_unavailable", "message": "Editable metadata could not be read.", "retryable": true}})
			} else {
				failure(w, err)
			}
			return
		}
		if _, err = d.manualMetadataOwner(ctx, r); err != nil {
			failure(w, err)
			return
		}
		encoded, encodeErr := json.Marshal(out)
		if encodeErr != nil || len(encoded) > 1<<20 {
			write(w, 503, map[string]any{"error": map[string]any{"code": "metadata_edit_unavailable", "message": "Editable metadata could not be read.", "retryable": true}})
			return
		}
		if ctx.Err() != nil {
			write(w, 503, map[string]any{"error": map[string]any{"code": "metadata_edit_interrupted", "message": "The metadata response was interrupted. Check current values before trying again.", "retryable": false}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(encoded)
	}
}
