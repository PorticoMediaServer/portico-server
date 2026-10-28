package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"portico.local/server/internal/dbwork"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/metadata"
)

// The preview shares the live administrator/metadata transaction guard with
// POST metadata/bulk; a preview does not grant mutation authority.
func registerBulkMetadataPreview(registry *apikit.Registry, d Dependencies) {
	v1Route(registry, apikit.Metadata{Audience: apikit.Clients, Admission: laneBrowsing, ID: "post_metadata_bulk_preview", Method: "POST", Path: "/v1/metadata/bulk/preview", Summary: "Read shared and mixed metadata for an authorized selection", Access: apikit.Admin, AdminScope: "metadata", Requires: apikit.ScopeLibrary, Lane: apikit.Browsing, Cost: apikit.PageSized, BodyLimit: 524288, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "revision_mismatch", "internal"}},
		func(ctx context.Context, r *http.Request, in catalog.BulkMetadataPreviewRequest) (out metadata.BulkMetadataPreview, err error) {
			// Selection validation can originate in either service. Translate it
			// at the HTTP boundary while retaining authentication and fault errors.
			defer func() {
				if errors.Is(err, metadata.ErrRepairInput) {
					err = &apierror.Error{Code: "invalid_request", Cause: err}
				}
				err = bulkError(err)
			}()
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
			var conversion *catalog.BrowseSelectionConversion
			var heldQuery *catalog.JobQuery
			var heldRevision string
			if in.Selector != nil && in.Selector.Query != nil && in.Selector.Query.SelectionBasis != "" {
				q := *in.Selector.Query
				var node *catalog.BrowseNode
				if err = json.Unmarshal(q.Filter, &node); err != nil {
					return out, err
				}
				if catalog.BrowseNeedsLocalSearch(node) {
					heldQuery = &q
					heldRevision = in.Expected.CatalogRevision
					err = dbwork.WithReadSnapshot(ctx, d.DB, func(snapshot context.Context) error {
						libraries, bound, e := d.homeScopeFor(r.WithContext(snapshot), p)
						if e != nil {
							return e
						}
						viewer, e := d.catalogViewer(r.WithContext(snapshot), p, libraries, bound)
						if e != nil {
							return e
						}
						read := d.Catalog.WithContext(snapshot)
						value, e := read.ResolvePreparedBrowseSelection(viewer, q, heldRevision)
						if e != nil {
							return e
						}
						conversion = &value
						if !apikit.AfterResponse(ctx, value.Release) {
							value.Release()
							return catalog.ErrStaleContinuation
						}
						if e = value.Reserve(8 << 20); e != nil {
							return e
						}
						raw, e := json.Marshal(value.IDs)
						if e != nil {
							return e
						}
						rows, e := dbwork.Snapshot(snapshot).QueryContext(snapshot, `SELECT k.name,pid(i.public_id),l.library_id FROM json_each(?) picked CROSS JOIN catalog_entities i ON i.id=picked.value JOIN catalog_kinds k ON k.id=i.kind JOIN catalog_libraries l ON l.id=i.library_id`, string(raw))
						if e != nil {
							return e
						}
						defer rows.Close()
						targets := make([]catalog.BulkMetadataTarget, 0, len(value.IDs))
						for rows.Next() {
							var t catalog.BulkMetadataTarget
							if e = rows.Scan(&t.ContentKind, &t.ID, &t.LibraryID); e != nil {
								return e
							}
							t.Kind = "item"
							targets = append(targets, t)
						}
						if e = rows.Err(); e != nil {
							return e
						}
						if len(targets) != len(value.IDs) {
							return catalog.ErrStaleContinuation
						}
						in = catalog.BulkMetadataPreviewRequest{Targets: targets}
						return nil
					})
					if err != nil {
						return out, err
					}
				}
			}
			var fence string
			// Each source call and metadata page runs in its own capped snapshot.
			// The owner request may span many pages; only the caller's deadline
			// bounds the whole operation, and cancellation cleans its spill file.
			source := func(snapshot context.Context, tx *sql.Tx, after int64) (catalog.BulkMetadataSelectionPage, error) {
				request := r.WithContext(snapshot)
				libraries, bound, err := d.homeScopeFor(request, p)
				if err != nil {
					return catalog.BulkMetadataSelectionPage{}, err
				}
				if fence == "" {
					fence = bound
				} else if bound != fence {
					return catalog.BulkMetadataSelectionPage{}, catalog.ErrStaleContinuation
				}
				viewer, err := d.catalogViewer(request, p, libraries, bound)
				if err != nil {
					return catalog.BulkMetadataSelectionPage{}, err
				}
				if heldQuery != nil {
					if err = d.Catalog.CheckBrowseSelectionBasisTx(snapshot, tx, viewer, heldQuery.LibraryID, heldQuery.SelectionBasis); err != nil {
						return catalog.BulkMetadataSelectionPage{}, err
					}
				}
				return d.Catalog.BulkMetadataSelectionPageTx(snapshot, tx, viewer, in, after)
			}
			if conversion != nil {
				out, err = d.Metadata.BulkPreviewMemory(ctx, source, func(snapshot context.Context, tx *sql.Tx) error { return d.manualMetadataAuthorize(snapshot, tx, p) }, conversion.Reserve)
				out.SelectionRevision = heldRevision
			} else {
				out, err = d.Metadata.BulkPreview(ctx, source, func(snapshot context.Context, tx *sql.Tx) error { return d.manualMetadataAuthorize(snapshot, tx, p) })
			}
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
