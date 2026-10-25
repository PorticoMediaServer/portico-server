package metadata

import (
	"context"
	"database/sql"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/localonly"
	"portico.local/server/internal/metadataprovider"
)

type lookupServiceKey struct{}

func (s *Service) lookupContext(ctx context.Context) context.Context {
	if ctx.Value(lookupServiceKey{}) == s {
		return ctx
	}
	return context.WithValue(metadataprovider.WithNetworkAdmission(ctx, func(ctx context.Context) (func() error, error) {
		allowed, epoch, err := localonly.Online(ctx, dbwork.Read(ctx, s.db))
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, localonly.Err
		}
		return func() error {
			current, revision, err := localonly.Online(ctx, dbwork.Read(ctx, s.db))
			if err != nil {
				return err
			}
			if !current || revision != epoch {
				return localonly.Err
			}
			return nil
		}, nil
	}), lookupServiceKey{}, s)
}
func onlineLookupPolicyTx(ctx context.Context, tx *sql.Tx) (bool, int64, error) {
	return localonly.Online(ctx, tx)
}
func invalidateLookupWorkTx(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range []string{
		`UPDATE screen_metadata_work SET status='pending',generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error=''`,
		`UPDATE mb_jobs SET status='pending',generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error=''`,
		`UPDATE tvdb_jobs SET status='pending_search',generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error=''`,
		`INSERT INTO artwork_dirty(kind,entity_id) SELECT kind,entity_id FROM artwork_entity_heads ON CONFLICT(kind,entity_id) DO NOTHING`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
