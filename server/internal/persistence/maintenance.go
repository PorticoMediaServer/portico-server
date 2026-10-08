package persistence

import (
	"context"
	"database/sql"

	"portico.local/server/internal/facettext"
)

// FoldFacetSQL folds a genre or person name for facet matching: case,
// separators and doubled spaces, with the sci-fi aliases. The catalogue's
// recommendation queries use the identical text.
func FoldFacetSQL(v string) string { return facettext.FoldFacetSQL(v) }

// PruneIdentityProofs deletes expired factor challenges and Top Shelf tokens.
func PruneIdentityProofs(ctx context.Context, db *sql.DB, now string) error {
	for _, statement := range []string{
		`DELETE FROM identity_factor_challenges WHERE expires_at<=?`,
		`DELETE FROM topshelf_tokens WHERE expires_at<=?`,
	} {
		if _, err := db.ExecContext(ctx, statement, now); err != nil {
			return err
		}
	}
	return nil
}
