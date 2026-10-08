package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/metadata"
)

func repairFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, metadata.ErrScreenConflict) || errors.Is(err, metadata.ErrRepairConflict) || errors.Is(err, metadata.ErrMBConflict) || errors.Is(err, metadata.ErrTVDBConflict) {
		write(w, 409, map[string]any{"error": map[string]any{"code": "metadata_conflict", "message": "Metadata changed. Reload and review your decision.", "retryable": false}})
		return
	}
	if errors.Is(err, metadata.ErrRepairInput) {
		write(w, 400, map[string]any{"error": map[string]any{"code": "invalid_metadata_repair", "message": "Check the repair fields and confirmation.", "retryable": false}})
		return
	}
	failure(w, err)
}
func (d Dependencies) metadataRepairRoutes(mux *http.ServeMux) {
	rate := &personalLimiter{}
	for _, method := range []string{"GET", "POST"} {
		mux.HandleFunc(method+" /v1/metadata/{kind}/{id}", func(w http.ResponseWriter, r *http.Request) {
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
			if r.URL.RawQuery != "" {
				repairFailure(w, metadata.ErrRepairInput)
				return
			}
			authorize := func(tx *sql.Tx) error { return d.manualMetadataAuthorize(ctx, tx, p) }
			target := metadata.RepairTarget{Kind: r.PathValue("kind"), ID: r.PathValue("id")}
			var out metadata.RepairState
			if method == "GET" {
				out, err = d.Metadata.RepairState(ctx, target, authorize)
			} else {
				if !rate.allow(viewerScope(p)) {
					write(w, 429, map[string]any{"error": map[string]any{"code": "rate_limited", "message": "Too many metadata changes.", "retryable": true}})
					return
				}
				var body metadata.RepairCommand
				if err = decode(w, r, &body); err == nil {
					out, err = d.Metadata.Repair(ctx, target, body, metadata.MBActor{Authority: p.Authority, AccountID: p.AccountID, ProfileID: p.ProfileID}, authorize)
				}
			}
			if err != nil {
				repairFailure(w, err)
				return
			}
			if _, err = d.manualMetadataOwner(ctx, r); err != nil {
				failure(w, err)
				return
			}
			out.Server = d.Identity.ID()
			out.ViewerFence = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", p.Hash, p.Epoch))))
			write(w, 200, out)
		})
	}
	mux.HandleFunc("GET /v1/metadata/{kind}/{id}/preview", func(w http.ResponseWriter, r *http.Request) {
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
		target := metadata.RepairTarget{Kind: r.PathValue("kind"), ID: r.PathValue("id")}
		var merge *metadata.RepairTarget
		if id := r.URL.Query().Get("mergeId"); id != "" {
			merge = &metadata.RepairTarget{Kind: target.Kind, ID: id}
		}
		out, err := d.Metadata.RepairPreview(ctx, target, merge, func(tx *sql.Tx) error { return d.manualMetadataAuthorize(ctx, tx, p) })
		if err != nil {
			repairFailure(w, err)
			return
		}
		write(w, 200, out)
	})
	mux.HandleFunc("GET /v1/metadata/{kind}/{id}/art/{role}", func(w http.ResponseWriter, r *http.Request) {
		p, err := d.principal(r)
		if err != nil {
			failure(w, err)
			return
		}
		if d.Metadata == nil {
			policyError(w, "metadata_unavailable")
			return
		}
		target := metadata.RepairTarget{Kind: r.PathValue("kind"), ID: r.PathValue("id")}
		var library string
		if target.Kind == "item" {
			err = d.itemAccess(r.Context(), p, target.ID)
		} else {
			// Container kinds keep their catalogue kind numbers (baseline
			// catalog_kinds): the lookup both finds the library and checks
			// the entity is of the requested kind.
			var kind int
			switch target.Kind {
			case "album":
				kind = 6
			case "show":
				kind = 2
			case "artist":
				kind = 5
			case "book":
				kind = 8
			case "season":
				kind = 3
			default:
				err = metadata.ErrRepairInput
			}
			if err == nil {
				err = d.DB.QueryRowContext(r.Context(), `SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?) AND e.kind=?`, target.ID, kind).Scan(&library)
			}
			if err == nil {
				err = d.allowedLibrary(r.Context(), p, library)
			}
			if err == nil {
				service := d.Catalog
				if service == nil {
					service = catalog.New(d.DB)
				}
				var viewer catalog.Viewer
				viewer, err = d.catalogViewer(r, p, []string{library}, "")
				if err == nil {
					err = service.VisibleEntity(r.Context(), viewer, target.Kind, target.ID)
				}
			}
		}
		if err != nil {
			// Authentication succeeded above. Inaccessible-library artwork
			// must have the same response as an unknown target.
			if errors.Is(err, identity.ErrUnauthorized) {
				err = sql.ErrNoRows
			}
			failure(w, err)
			return
		}
		var f *os.File
		var mime string
		if candidate := r.URL.Query().Get("candidate"); candidate != "" {
			if _, err = d.manualMetadataOwner(r.Context(), r); err != nil {
				failure(w, err)
				return
			}
			f, mime, err = d.Metadata.PreviewArtwork(r.Context(), target, candidate, artworkWidth(r))
		} else {
			f, mime, err = d.Metadata.EntityArtworkVariant(r.Context(), target, r.PathValue("role"), r.URL.Query().Get("subject"), artworkWidth(r), r.URL.Query().Get("v"))
		}
		if err != nil {
			if artworkVersionFailure(w, err) {
				return
			}
			if errors.Is(err, metadata.ErrArtworkPending) {
				w.Header().Set("Retry-After", "5")
				w.Header().Set("Cache-Control", "no-store")
				policyError(w, "artwork_pending")
			} else {
				repairFailure(w, err)
			}
			return
		}
		defer f.Close()
		serveArtwork(w, r, f, mime)
	})
}
