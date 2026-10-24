package httpapi

import (
	"context"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/contentaccess"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// Reading adopts live authority, grants, policy and restrictions inside the
// very transaction that checks projection readiness and reads its facts.
func (d Dependencies) readingBrowseViewer(ctx context.Context, p identity.Principal, library string, before catalog.Viewer) (catalog.Viewer, error) {
	tx := dbwork.Snapshot(ctx)
	if tx == nil {
		return catalog.Viewer{}, identity.ErrForbidden
	}
	var kind string
	if err := tx.QueryRowContext(ctx, `SELECT kind FROM libraries WHERE id=?`, library).Scan(&kind); err != nil {
		return catalog.Viewer{}, err
	}
	if kind != "book" && kind != "comic" {
		return before, nil
	}
	scope, err := d.readingSeriesScopeTx(ctx, tx, p, library)
	if err != nil {
		return catalog.Viewer{}, err
	}
	restrictions, err := contentaccess.RestrictionsTx(ctx, tx, p.Viewer)
	if err != nil {
		return catalog.Viewer{}, err
	}
	return catalog.Viewer{Profile: scope.Profile, Fence: scope.Fence, Libraries: []string{library}, Restrictions: restrictions}, nil
}
