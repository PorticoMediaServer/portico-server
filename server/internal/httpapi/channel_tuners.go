package httpapi

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/livechannels"
)

// liveTunerAvailable says whether a live source has a tuner for one more viewer
// (spec §11: tuner exhaustion is 409 no_tuner_available at the start). Its claims
// are the tuners held (recordings) and the live v1 channel sessions
// on the source that hold none yet, less the session being replaced (surfing
// frees its own tuner). An unknown capacity never refuses.
func liveTunerAvailable(ctx context.Context, tx *sql.Tx, sourceID, replacing string) (bool, error) {
	capacity, err := livechannels.CapacityTx(ctx, tx, sourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !capacity.Known {
		return true, nil
	}
	var claims int
	err = tx.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM live_allocations a WHERE a.source_id=? AND a.state!='released' AND a.resource_id<>?)
 +(SELECT count(*) FROM playback_channel_sessions c JOIN playback_v1_sessions v ON v.id=c.session_id
    WHERE c.kind='live-source' AND c.source_id=? AND c.ended_ms=0 AND v.ended_ms=0 AND c.session_id<>?
    AND NOT EXISTS(SELECT 1 FROM live_allocations a WHERE a.resource_id=c.session_id AND a.state!='released'))`, sourceID, replacing, sourceID, replacing).Scan(&claims)
	if err != nil {
		return false, err
	}
	return claims < capacity.Effective, nil
}
