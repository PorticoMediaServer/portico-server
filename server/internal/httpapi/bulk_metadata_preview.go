package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadata"
)

// The preview shares the live administrator/metadata transaction guard with
// POST metadata/bulk; a preview does not grant mutation authority.
func registerBulkMetadataPreview(registry *apikit.Registry, d Dependencies) {
	v1Route(registry, apikit.Metadata{Admission: laneBrowsing, ID: "post_metadata_bulk_preview", Method: "POST", Path: "/v1/metadata/bulk/preview", Summary: "Read shared and mixed metadata for an authorized selection", Access: apikit.Admin, AdminScope: "metadata", Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Cost: apikit.PageSized, BodyLimit: 524288, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "revision_mismatch", "internal"}},
		func(ctx context.Context, r *http.Request, in catalog.BulkMetadataPreviewRequest) (out metadata.BulkMetadataPreview, err error) {
			// Selection validation can originate in either service. Translate it
			// at the HTTP boundary while retaining authentication and fault errors.
			defer func() {
				if errors.Is(err, metadata.ErrRepairInput) {
					err = &apierror.Error{Code: "invalid_request", Cause: err}
				}
				err = bulkError(err)
			}()
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			if r.URL.RawQuery != "" {
				return out, metadataAttentionError(metadata.ErrRepairInput)
			}
			if err := in.ValidateSelection(); err != nil {
				return out, err
			}
			if d.Metadata == nil || d.Catalog == nil || d.Identity == nil {
				return out, errMetadataUnavailable()
			}
			p, err := d.manualMetadataAdministrator(ctx, r)
			if err != nil {
				return out, err
			}
			var fence string
			// The preview reads the whole selection in one snapshot, so it
			// takes the single owner-long read permit (background connection,
			// at most ten minutes, ended with a retryable refusal under hard
			// WAL pressure). The request's own deadline still applies.
			long, release, err := dbwork.AcquireLongRead(ctx, dbwork.LongReadBulkMetadataPreview)
			if err != nil {
				return out, err
			}
			defer release()
			err = dbwork.WithReadSnapshot(long, d.DB, func(snapshot context.Context) error {
				request := r.WithContext(snapshot)
				libraries, bound, err := d.homeScopeFor(request, p)
				if err != nil {
					return err
				}
				fence = bound
				viewer, err := d.catalogViewer(request, p, libraries, bound)
				if err != nil {
					return err
				}
				tx := dbwork.Snapshot(snapshot)
				if tx == nil {
					return errors.New("metadata preview read snapshot unavailable")
				}
				authorize := func(tx *sql.Tx) error { return d.manualMetadataAuthorize(snapshot, tx, p) }
				if err = authorize(tx); err != nil {
					return err
				}
				selection, err := d.Catalog.BulkMetadataSelectionTx(snapshot, tx, viewer, in)
				if err != nil {
					return bulkError(err)
				}
				out, err = d.Metadata.BulkPreviewTx(snapshot, tx, selection, authorize)
				return err
			})
			if err != nil {
				return out, metadataAttentionError(err)
			}
			if _, err = d.manualMetadataAdministrator(ctx, r); err != nil {
				return metadata.BulkMetadataPreview{}, err
			}
			libraries, bound, err := d.homeScopeFor(r, p)
			_ = libraries
			if err != nil {
				return metadata.BulkMetadataPreview{}, err
			}
			if bound != fence {
				return metadata.BulkMetadataPreview{}, bulkError(catalog.ErrStaleContinuation)
			}
			out.ServerID = d.Identity.ID()
			out.ViewerFence = fence
			return out, nil
		})
}
