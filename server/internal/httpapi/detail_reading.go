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
		// A Reading leaf is not timed-media playable, so the catalogue's
		// playable-kind lookup conceals it. The same request's Reading personal
		// authority decides it instead; its refusal keeps the original
		// concealed absence.
		if !errors.Is(err, identity.ErrContentRestricted) {
			return p, "", "", err
		}
		read, readErr := dbwork.BeginSnapshot(r.Context(), d.DB)
		if readErr != nil {
			return p, "", "", readErr
		}
		var kind int
		if kindErr := read.Tx().QueryRowContext(r.Context(), `SELECT kind FROM catalog_entities WHERE public_id=pid_blob(?)`, r.PathValue("id")).Scan(&kind); kindErr != nil || kind != 20 && kind != 21 {
			_ = read.Rollback()
			return p, "", "", err
		}
		readingErr := d.savedReadingItemAuthorityTx(r.Context(), read.Tx(), p, r.PathValue("id"))
		if rollbackErr := read.Rollback(); rollbackErr != nil {
			return p, "", "", rollbackErr
		}
		if readingErr != nil {
			return p, "", "", err
		}
	}
	library, fence, err := d.detailContentFenceFor(r, p)
	return p, library, fence, err
}
