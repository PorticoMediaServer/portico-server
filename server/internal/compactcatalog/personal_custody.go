package compactcatalog

import (
	"context"
	"database/sql"
)

// RequirePersonalContainerWriteTx also checks descendants: changing a container
// watched default can change the value being retained for one of its children.
// The existing fixed ancestor indexes supply these point probes; no child list
// or profile list is materialized.
func RequirePersonalContainerWriteTx(ctx context.Context, tx *sql.Tx, entity int64) error {
	if err := RequireEntityAcquirableTx(ctx, tx, entity); err != nil {
		return err
	}
	return requireNoReverseMutationOverlapTx(ctx, tx, "", entity, 0)
}

// RequireRetainedOwnerWriteTx fences profile-wide activity resets while that
// owner's copy or input drain is in flight. The catalog owner keeps this scalar
// aligned until its inputs are drained, then selects the next owner atomically.
// Profile erasure uses its own invalidation units and must not call this guard.
func RequireRetainedOwnerWriteTx(ctx context.Context, tx *sql.Tx, owner string) error {
	var held bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_purge_jobs WHERE owner_value=?)`, owner).Scan(&held); err != nil {
		return err
	}
	if held {
		return ErrEntityMutating
	}
	return nil
}
