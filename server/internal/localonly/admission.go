package localonly

import (
	"context"
	"database/sql"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadataprovider"
)

// WithOnlineAdmission carries one already captured epoch through every provider
// subrequest. The live reads intentionally use the DB pool, not a caller's tx.
func WithOnlineAdmission(ctx context.Context, db *sql.DB, epoch int64) context.Context {
	return metadataprovider.WithNetworkAdmission(ctx, func(ctx context.Context) (func() error, error) {
		check := func() error { return CheckOnline(ctx, dbwork.ReadHandle(ctx, db), epoch) }
		if err := check(); err != nil {
			return nil, err
		}
		return check, nil
	})
}
