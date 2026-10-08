package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"portico.local/server/internal/access"
	"portico.local/server/internal/dbwork"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
)

// viewerRestrictions resolves the content restriction a viewer session carries and
// the fence fragment that makes an open cursor or a cached page expire when the
// restriction changes.
//
// Every catalog surface that can show a title calls this once, folds the fragment
// into its viewer fence and passes the restriction into the catalog request. The
// catalog then applies exactly one predicate (internal/catalog/restrictions.go);
// no surface writes a visibility rule of its own.
func (d Dependencies) viewerRestrictions(r *http.Request, p identity.Principal) (identity.ContentRestrictions, string, error) {
	// Several surfaces on one page each ask for this, and it is one row that
	// changes only through a restriction publication. The cache is fenced on the
	// authority generation, so a publication invalidates it before the next read
	// rather than within a TTL, and the security-fence lanes bypass it entirely.
	strict := strictPrincipal(dbwork.ClassFrom(r.Context(), dbwork.ClassInteractive))
	profileKey := identity.PersonalKey(p.Viewer)
	var restrictions identity.ContentRestrictions
	var found bool
	if !strict {
		restrictions, found = d.restrictions.lookup(profileKey, time.Now())
	}
	if !found {
		generation := dbwork.AuthorityGeneration()
		var e error
		restrictions, e = d.Identity.ViewerRestrictions(r.Context(), p)
		if e != nil {
			return identity.ContentRestrictions{}, "", e
		}
		if !strict {
			d.restrictions.store(profileKey, restrictions, time.Now(), generation)
		}
	}
	if d.Access.Access != nil && p.AccountID != "" {
		var revision int64
		var body string
		e := d.DB.QueryRowContext(r.Context(), `SELECT revision,body FROM access_limits WHERE account_id=?`, p.AccountID).Scan(&revision, &body)
		if e != nil && e != sql.ErrNoRows {
			return identity.ContentRestrictions{}, "", e
		}
		if e == nil {
			var limits access.Limits
			if e = json.Unmarshal([]byte(body), &limits); e != nil {
				return identity.ContentRestrictions{}, "", e
			}
			restrictions.MemberMaxRating = limits.MaxContentRating
			restrictions.MemberAllowUnrated = limits.AllowUnrated
			restrictions.MemberDeniedLabels = limits.Tags.DeniedLabels
			restrictions.MemberRevision = revision
		}
	}
	return restrictions, catalog.RestrictionFence(restrictions), nil
}

func (d Dependencies) catalogViewer(r *http.Request, p identity.Principal, libraries []string, fence string) (catalog.Viewer, error) {
	restrictions, _, err := d.viewerRestrictions(r, p)
	if err != nil {
		return catalog.Viewer{}, err
	}
	return catalog.Viewer{Profile: identity.PersonalKey(p.Viewer), Fence: fence, Libraries: libraries, Restrictions: restrictions, MemberMaxRating: restrictions.MemberMaxRating, MemberAllowUnrated: restrictions.MemberAllowUnrated, MemberDeniedLabels: restrictions.MemberDeniedLabels}, nil
}

// restrictedItem refuses a single-item read whose title the profile may not see, so
// knowing an id is not a way around a list that filtered it out.
func (d Dependencies) restrictedItem(r *http.Request, p identity.Principal, item string) error {
	// Integration merge: lane B's viewer-scoped catalogue check (profile and,
	// since 5f0be04, member limits), then lane C's member enforcer (labels and
	// the access ladder), with C's snapshot fallback when no catalogue is wired.
	if d.Catalog == nil {
		gate, err := dbwork.BeginSnapshot(r.Context(), d.DB)
		if err != nil {
			return err
		}
		defer gate.Rollback()
		return d.itemRestrictionsTx(r.Context(), gate.Tx(), p, item)
	}
	// Lane B's catalogue answers a restricted title exactly as an absent one
	// (sql.ErrNoRows); lane C's admission callers expect ErrContentRestricted,
	// which every writer maps to the same 404.
	library, e := d.Catalog.WithContext(r.Context()).LibraryForItem(item)
	if errors.Is(e, sql.ErrNoRows) {
		return identity.ErrContentRestricted
	}
	if e != nil {
		return e
	}
	viewer, e := d.catalogViewer(r, p, []string{library}, "")
	if e != nil {
		return e
	}
	if e = d.Catalog.VisibleItem(r.Context(), viewer, item); errors.Is(e, sql.ErrNoRows) {
		return identity.ErrContentRestricted
	} else if e != nil {
		return e
	}
	if d.Access.Access != nil {
		gate, err := dbwork.BeginSnapshot(r.Context(), d.DB)
		if err != nil {
			return err
		}
		defer gate.Rollback()
		return d.enforcer(false).VisibleItem(r.Context(), gate.Tx(), p, item)
	}
	return nil
}
