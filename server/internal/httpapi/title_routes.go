package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/identity"
)

// Owner routes for titles and libraries as things in their own right: the
// list of titles whose files are gone, emptying it, saying two titles are one,
// and removing a library.

type emptyMissingRequest struct {
	// LibraryID limits the emptying to one library; empty means every library.
	LibraryID string `json:"libraryId,omitempty"`
}

type mergeTitlesRequest struct {
	// FromID is merged into IntoID and removed; IntoID keeps its id.
	FromID string `json:"fromId"`
	IntoID string `json:"intoId"`
}

type mergeTitlesResponse struct {
	ItemID string `json:"itemId"`
}

type removeLibraryRequest struct {
	// Confirmation is the library's name, typed back.
	Confirmation     string `json:"confirmation"`
	ExpectedRevision int64  `json:"expectedRevision"`
}

type removeLibraryResponse struct {
	LibraryID string `json:"libraryId"`
	Removing  bool   `json:"removing"`
}

// removalEnqueue queues background removal work in the caller's transaction.
func (d Dependencies) removalEnqueue(p identity.Principal) func(context.Context, *sql.Tx, string, string) error {
	if d.Scheduler == nil {
		return nil
	}
	return func(ctx context.Context, tx *sql.Tx, kind, resource string) error {
		_, err := d.Scheduler.EnqueueDomainTx(ctx, tx, kind, resource, administrationActor(p))
		return err
	}
}

func identityRouteError(err error) error {
	var public *apierror.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &public):
		return err
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, entityid.ErrNotFound), errors.Is(err, catalog.ErrLibraryRemoving):
		return &apierror.Error{Code: "not_found", Cause: err}
	case errors.Is(err, catalog.ErrLibraryConfigurationConflict):
		return &apierror.Error{Code: "revision_mismatch", Cause: err}
	case errors.Is(err, catalog.ErrConfirmationMismatch):
		return &apierror.Error{Code: "confirmation_mismatch", Cause: err}
	case errors.Is(err, catalog.ErrAdminQuery), errors.Is(err, catalog.ErrNotMergeable):
		return &apierror.Error{Code: "invalid_request", Cause: err}
	case errors.Is(err, identity.ErrForbidden):
		return &apierror.Error{Code: "not_permitted", Cause: err}
	case errors.Is(err, identity.ErrUnauthorized):
		return &apierror.Error{Code: "unauthorized", Cause: err}
	}
	return &apierror.Error{Code: "internal", Cause: err}
}

func registerIdentityRoutes(r *apikit.Registry, d Dependencies) {
	ownerErrors := []string{"unauthorized", "not_permitted", "not_found", "invalid_request", "internal"}
	v1Route(r, apikit.Metadata{ID: "list_missing_titles", Method: "GET", Path: "/v1/admin/missing-titles", Summary: "List titles whose files are no longer on disk, oldest first, with when each will be removed",
		Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.PageSized, Status: 200, Query: []string{"libraryId", "cursor", "limit"}, Errors: ownerErrors},
		func(ctx context.Context, q *http.Request, _ noBody) (catalog.MissingTitlesPage, error) {
			limit := 50
			if raw := q.URL.Query().Get("limit"); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil || n < 1 || n > 200 {
					return catalog.MissingTitlesPage{}, &apierror.Error{Code: "invalid_request"}
				}
				limit = n
			}
			out, err := d.Catalog.MissingTitles(ctx, q.URL.Query().Get("libraryId"), q.URL.Query().Get("cursor"), limit)
			return out, identityRouteError(err)
		})
	v1Route(r, apikit.Metadata{ID: "empty_missing_titles", Method: "POST", Path: "/v1/admin/missing-titles/empty", Summary: "Remove missing titles and missing files now, whatever the owner's rule says; call again while more is true",
		Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.PageSized, BodyLimit: 4096, OptionalBody: true, Status: 200, Errors: ownerErrors},
		func(ctx context.Context, q *http.Request, in emptyMissingRequest) (catalog.MissingEmptied, error) {
			p, _ := d.foundationCaller(q)
			var total catalog.MissingEmptied
			// A few short transactions per call keeps the request brief; the
			// caller repeats while there is more.
			deadline := time.Now().Add(8 * time.Second)
			cutoff := time.Now()
			for {
				step, err := d.Catalog.EmptyMissing(ctx, in.LibraryID, cutoff, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
				if err != nil {
					return total, identityRouteError(err)
				}
				total.Titles += step.Titles
				total.Files += step.Files
				total.More = step.More && (step.Titles > 0 || step.Files > 0)
				if !total.More || time.Now().After(deadline) || ctx.Err() != nil {
					return total, nil
				}
			}
		})
	v1Route(r, apikit.Metadata{ID: "merge_titles", Method: "POST", Path: "/v1/admin/titles/merge", Summary: "Say that two titles of one library are the same title: the first becomes part of the second, with its files as versions and everyone's state combined",
		Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.PageSized, BodyLimit: 4096, Status: 200, Errors: ownerErrors},
		func(ctx context.Context, q *http.Request, in mergeTitlesRequest) (mergeTitlesResponse, error) {
			p, _ := d.foundationCaller(q)
			err := d.Catalog.MergeTitles(ctx, in.FromID, in.IntoID, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") })
			return mergeTitlesResponse{ItemID: in.IntoID}, identityRouteError(err)
		})
	v1Route(r, apikit.Metadata{ID: "remove_library", Method: "DELETE", Path: "/v1/admin/libraries/{id}", Summary: "Remove a library: it disappears at once and is taken apart in the background; files on disk are not touched and each profile's state is kept in case its titles return",
		Access: apikit.Owner, Lane: apikit.Default, Cost: apikit.Constant, BodyLimit: 4096, Status: 202, Errors: append([]string{"revision_mismatch", "confirmation_mismatch", "administration_unavailable"}, ownerErrors...)},
		func(ctx context.Context, q *http.Request, in removeLibraryRequest) (removeLibraryResponse, error) {
			p, _ := d.foundationCaller(q)
			id := q.PathValue("id")
			enqueue := d.removalEnqueue(p)
			if enqueue == nil {
				return removeLibraryResponse{}, &apierror.Error{Code: "administration_unavailable"}
			}
			err := d.Catalog.RemoveLibrary(ctx, id, in.Confirmation, in.ExpectedRevision, func(tx *sql.Tx) error { return d.resourceAuthorize(tx, p, "") }, enqueue)
			if err != nil {
				return removeLibraryResponse{}, identityRouteError(err)
			}
			if d.Ingestion != nil {
				d.Ingestion.InterruptLibrary(ctx, id)
			}
			return removeLibraryResponse{LibraryID: id, Removing: true}, nil
		})
}
