package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strconv"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/readprep"
)

// One process-owned runtime shares the read-preparation store with other
// domains. A nil runtime answers unavailable; requests never build a store.
type FacetReadRuntime interface {
	Initial(context.Context, readprep.Caller, catalog.Viewer, catalog.FacetRequest) (catalog.SimpleFacetResult, error)
	Continue(context.Context, readprep.Caller, catalog.Viewer, catalog.FacetRequest, string) (catalog.SimpleFacetResult, error)
	Poll(context.Context, readprep.Caller, string) (catalog.SimpleFacetResult, error)
	Cancel(context.Context, readprep.Caller, string) error
}

type facetReadResponse struct{}
type facetQueryResponse struct{}

func facetSchema[T any]() map[string]any {
	s, err := apikit.Schema(reflect.TypeFor[T]())
	if err != nil {
		panic(err)
	}
	return s
}
func (facetReadResponse) JSONSchema() map[string]any {
	return map[string]any{"anyOf": []any{facetSchema[catalog.FacetPage](), facetSchema[catalog.SimpleFacetPage](), facetSchema[catalog.SimpleFacetPreparing]()}}
}
func (facetReadResponse) JSONSchemaForStatus(status int) map[string]any {
	if status == 200 {
		return map[string]any{"anyOf": []any{facetSchema[catalog.FacetPage](), facetSchema[catalog.SimpleFacetPage]()}}
	}
	if status == 202 {
		return facetSchema[catalog.SimpleFacetPreparing]()
	}
	return nil
}
func (facetQueryResponse) JSONSchema() map[string]any {
	return map[string]any{"anyOf": []any{facetSchema[catalog.SimpleFacetPage](), facetSchema[catalog.SimpleFacetPreparing]()}}
}
func (facetQueryResponse) JSONSchemaForStatus(status int) map[string]any {
	if status == 200 {
		return facetSchema[catalog.SimpleFacetPage]()
	}
	if status == 202 {
		return facetSchema[catalog.SimpleFacetPreparing]()
	}
	return nil
}

func facetPreparingHeaders() []apikit.ResponseHeader {
	return []apikit.ResponseHeader{
		{Status: 202, Name: "Location", Description: "Same-server facet preparation poll path", Required: true},
		{Status: 202, Name: "Retry-After", Description: "Minimum delay before polling", Required: true},
	}
}

// Family identity survives access-token rotation. Policy revision is private
// authority, separate from the shared effective-policy content equivalence key.
func (d Dependencies) facetCallerTx(ctx context.Context, tx *sql.Tx, r *http.Request, p identity.Principal) (readprep.Caller, error) {
	family, err := d.Identity.SessionFamilyTx(ctx, tx, p)
	if err != nil {
		return "", err
	}
	fence, err := d.browseFence(r.WithContext(ctx), p)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal([]any{p.ServerID, family.ID, presentationIdentity(p), p.Epoch, fence})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return readprep.Caller(hex.EncodeToString(sum[:])), nil
}

func (d Dependencies) facetReadContext(r *http.Request, p identity.Principal) (context.Context, readprep.Caller, error) {
	var caller readprep.Caller
	err := dbwork.WithReadSnapshot(r.Context(), d.DB, func(ctx context.Context) error {
		var err error
		caller, err = d.facetCallerTx(ctx, dbwork.Snapshot(ctx), r, p)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	guard := func(ctx context.Context, actual readprep.Caller, snapshot catalog.FacetBaseSnapshot) error {
		if actual != caller {
			return catalog.ErrStaleContinuation
		}
		return dbwork.WithReadSnapshot(ctx, d.DB, func(ctx context.Context) error {
			tx := dbwork.Snapshot(ctx)
			now, err := d.facetCallerTx(ctx, tx, r, p)
			if err != nil {
				return err
			}
			if now != caller {
				return catalog.ErrStaleContinuation
			}
			if err = d.allowedLibraryTx(ctx, p, snapshot.Scope.Library, tx); err != nil {
				return err
			}
			restrictions, _, err := d.viewerRestrictions(r.WithContext(ctx), p)
			if err != nil {
				return err
			}
			if snapshot.Scope.Policy != catalog.FacetBasePolicy(restrictions) {
				return catalog.ErrStaleContinuation
			}
			if snapshot.ID != 0 {
				authority, err := catalog.CaptureFacetAuthority(ctx, tx, snapshot.Scope)
				if err != nil {
					return err
				}
				if authority != snapshot.Authority {
					return catalog.ErrStaleContinuation
				}
			}
			return nil
		})
	}
	return catalog.WithFacetWindowAuthority(r.Context(), guard), caller, nil
}

func facetReadFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, readprep.ErrStale):
		browseFailure(w, catalog.ErrStaleContinuation)
	case errors.Is(err, readprep.ErrCapacity):
		failure(w, &apierror.Error{Code: "search_busy", RetryAfterSeconds: 1, Cause: err})
	case errors.Is(err, catalog.ErrFacetBaseUnavailable), errors.Is(err, catalog.ErrFacetBaseCapacity), errors.Is(err, readprep.ErrClosed):
		failure(w, &apierror.Error{Code: "metadata_unavailable", RetryAfterSeconds: 1, Cause: err})
	case errors.Is(err, catalog.ErrFacetBaseInput):
		browseFailure(w, catalog.ErrCursor)
	default:
		browseFailure(w, err)
	}
}

func writeFacetRead(w http.ResponseWriter, result catalog.SimpleFacetResult) {
	defer result.Release()
	if result.Preparing != nil && result.Page == nil {
		w.Header().Set("Location", "/v1/facet-queries/"+result.Preparing.QueryID)
		w.Header().Set("Retry-After", strconv.Itoa(max(1, (result.Preparing.RetryAfterMs+999)/1000)))
		write(w, http.StatusAccepted, result.Preparing)
		return
	}
	if result.Page == nil || result.Preparing != nil {
		failure(w, errors.New("invalid facet preparation result"))
		return
	}
	write(w, http.StatusOK, result.Page)
}

func (d Dependencies) simpleFacetsHandler(w http.ResponseWriter, r *http.Request, p identity.Principal, viewer catalog.Viewer, request catalog.FacetRequest) {
	if d.FacetReads == nil {
		facetReadFailure(w, catalog.ErrFacetBaseUnavailable)
		return
	}
	ctx, caller, err := d.facetReadContext(r, p)
	if err != nil {
		facetReadFailure(w, err)
		return
	}
	var result catalog.SimpleFacetResult
	token := request.After
	if request.Before != "" {
		token = request.Before
	}
	if request.At != "" {
		token = request.At
	}
	if token == "" {
		result, err = d.FacetReads.Initial(ctx, caller, viewer, request)
	} else {
		result, err = d.FacetReads.Continue(ctx, caller, viewer, request, token)
	}
	if err != nil {
		result.Release()
		facetReadFailure(w, err)
		return
	}
	writeFacetRead(w, result)
}

func validFacetQueryID(id string) bool {
	if len(id) == 0 || len(id) > 512 {
		return false
	}
	for _, c := range id {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (d Dependencies) facetQueryHandler(w http.ResponseWriter, r *http.Request) {
	p, err := d.principal(r)
	if err != nil {
		facetReadFailure(w, err)
		return
	}
	if len(r.URL.Query()) != 0 || !validFacetQueryID(r.PathValue("id")) {
		facetReadFailure(w, catalog.ErrFacetBaseInput)
		return
	}
	if d.FacetReads == nil {
		facetReadFailure(w, catalog.ErrFacetBaseUnavailable)
		return
	}
	ctx, caller, err := d.facetReadContext(r, p)
	if err != nil {
		facetReadFailure(w, err)
		return
	}
	if r.Method == http.MethodDelete {
		if err = d.FacetReads.Cancel(ctx, caller, r.PathValue("id")); err != nil {
			facetReadFailure(w, err)
			return
		}
		write(w, http.StatusNoContent, nil)
		return
	}
	result, err := d.FacetReads.Poll(ctx, caller, r.PathValue("id"))
	if err != nil {
		result.Release()
		facetReadFailure(w, err)
		return
	}
	writeFacetRead(w, result)
}
