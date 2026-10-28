package metadata

import (
	"context"
	"database/sql"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/localonly"
	"portico.local/server/internal/metadataprovider"
)

type lookupServiceKey struct{}
type lookupCapture struct {
	service *Service
	epoch   int64
}

func (s *Service) lookupContext(ctx context.Context) context.Context {
	if captured, ok := ctx.Value(lookupServiceKey{}).(lookupCapture); ok && captured.service == s {
		return ctx
	}
	// ReadHandle returns a *sql.DB pool, not the caller's Snapshot transaction.
	// Calling its QueryRowContext directly keeps these live IO fences outside any
	// caller-held read snapshot; dbwork.QueryRow would deliberately join it.
	allowed, epoch, capturedErr := localonly.Online(ctx, dbwork.ReadHandle(ctx, s.db))
	check := func() error {
		if capturedErr != nil {
			return capturedErr
		}
		if !allowed {
			return localonly.Err
		}
		current, revision, err := localonly.Online(ctx, dbwork.ReadHandle(ctx, s.db))
		if err != nil {
			return err
		}
		if !current || revision != epoch {
			return localonly.Err
		}
		return nil
	}
	return context.WithValue(metadataprovider.WithNetworkAdmission(ctx, func(context.Context) (func() error, error) {
		if err := check(); err != nil {
			return nil, err
		}
		return check, nil
	}), lookupServiceKey{}, lookupCapture{service: s, epoch: epoch})
}
func onlineLookupPolicyTx(ctx context.Context, tx *sql.Tx) (bool, int64, error) {
	return localonly.Online(ctx, tx)
}
func invalidateLookupWorkTx(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range []string{
		`DELETE FROM subtitle_provider_candidates`,
		`UPDATE screen_metadata_work SET status=CASE WHEN status='provider_disabled' THEN 'pending' ELSE status END,generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error=''`,
		`UPDATE mb_jobs SET generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error=''`,
		`UPDATE tvdb_jobs SET generation=generation+1,revision=revision+1,lease='',lease_until='',next_attempt='',error=''`,
		`INSERT INTO artwork_dirty(kind,entity_id) SELECT kind,entity_id FROM artwork_entity_heads WHERE true ON CONFLICT(kind,entity_id) DO NOTHING`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func checkLookupContextTx(ctx context.Context, tx *sql.Tx) error {
	captured, ok := ctx.Value(lookupServiceKey{}).(lookupCapture)
	if !ok {
		return localonly.Err
	}
	return localonly.CheckOnline(ctx, tx, captured.epoch)
}
