package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// This fallback belongs only to the detail document. Other users of the timed
// detail fence (offers, chapters, files and analysis) keep their original gate.
func (d Dependencies) detailContentFenceFor(r *http.Request, p identity.Principal) (string, string, error) {
	library, fence, err := d.detailFenceFor(r, p)
	if !errors.Is(err, sql.ErrNoRows) {
		return library, fence, err
	}
	read, err := dbwork.BeginSnapshot(r.Context(), d.DB)
	if err != nil {
		return "", "", err
	}
	defer read.Rollback()
	item := r.PathValue("id")
	if err = d.savedReadingItemAuthorityTx(r.Context(), read.Tx(), p, item); err != nil {
		return "", "", err
	}
	if err = read.Tx().QueryRowContext(r.Context(), `SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.public_id=pid_blob(?) AND e.retired=0 AND cl.retired=0`, item).Scan(&library); err != nil {
		return "", "", err
	}
	// Close this snapshot before the established viewer-fence reader opens its
	// own; the accepting composition repeats the Reading admission afterward.
	if err = read.Rollback(); err != nil {
		return "", "", err
	}
	return d.detailLibraryFenceFor(r, p, library)
}

func (d Dependencies) detailContentAccess(r *http.Request) (identity.Principal, string, string, error) {
	p, err := d.principal(r)
	if err != nil {
		return p, "", "", err
	}
	if err = d.restrictedItem(r, p, r.PathValue("id")); err != nil {
		return p, "", "", err
	}
	library, fence, err := d.detailContentFenceFor(r, p)
	return p, library, fence, err
}
