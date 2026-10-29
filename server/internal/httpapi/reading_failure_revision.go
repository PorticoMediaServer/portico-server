package httpapi

import (
	"errors"
	"net/http"

	"portico.local/apikit/apierror"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/reading"
)

// readingRevisionFailure answers a typed stale continuation with the current
// authorized content revision. Discover already resolved the library, principal
// and series scope before a continuation is compared, so this reports facts a
// successful read of the same library and viewer also publishes; an authority
// refusal never carries a revision. Other ErrChanged causes keep the existing
// revision_mismatch answer without a current document.
func readingRevisionFailure(w http.ResponseWriter, err error) bool {
	var stale *reading.RevisionError
	if !errors.As(err, &stale) {
		return false
	}
	policyErrors.Write(w, w.Header().Get("X-Request-Id"), &apierror.Error{
		Code:    "revision_mismatch",
		Current: catalog.ContentRevision{Catalog: stale.Catalog, Viewer: stale.Viewer},
		Cause:   err,
	})
	return true
}
