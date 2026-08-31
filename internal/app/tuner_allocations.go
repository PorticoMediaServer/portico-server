package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var errLiveTVTunerCapacity = errors.New("Live TV source tuner capacity is fully allocated")

const liveTVAllocationStaleAfter = 45 * time.Second

type liveTVTunerLease struct {
	Token   string
	Created bool
}

func (s *Server) reserveLiveTVTunerAllocation(ctx context.Context, sourceID, channelID, kind, consumerID string) (liveTVTunerLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sourceID = strings.TrimSpace(sourceID)
	channelID = strings.TrimSpace(channelID)
	consumerID = strings.TrimSpace(consumerID)
	if sourceID == "" || consumerID == "" || (kind != "live_session" && kind != "dvr_recording") {
		return liveTVTunerLease{}, errLiveTVTunerCapacity
	}
	now := time.Now().UTC()
	if err := s.pruneStaleLiveTVTunerAllocations(ctx); err != nil {
		return liveTVTunerLease{}, err
	}
	lease := liveTVTunerLease{Token: randomID("lease"), Created: true}
	changed := false
	err := s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		var existingToken string
		if err := tx.QueryRow(`SELECT lease_token FROM live_tv_tuner_allocations WHERE allocation_kind = ? AND consumer_id = ?`, kind, consumerID).Scan(&existingToken); err == nil {
			lease.Token, lease.Created = existingToken, false
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var capacity, allocated int
		if err := tx.QueryRow(`SELECT MAX(1, COALESCE(tuner_count, 1)) FROM live_tv_sources WHERE id = ? AND enabled = 1`, sourceID).Scan(&capacity); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM live_tv_tuner_allocations WHERE source_id = ?`, sourceID).Scan(&allocated); err != nil {
			return err
		}
		if allocated >= capacity {
			return errLiveTVTunerCapacity
		}
		// allocation_key is deliberately explicit. One playback session can make
		// many manifest/segment requests without consuming another tuner. Portico
		// does not claim cross-session upstream sharing until a provider adapter
		// supplies a real shared relay.
		allocationKey := kind + ":" + consumerID
		_, insertErr := tx.Exec(`
			INSERT INTO live_tv_tuner_allocations (
				id, source_id, channel_id, allocation_kind, consumer_id, allocation_key, lease_token, acquired_at, heartbeat_at, expires_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
			randomID("tuner"), sourceID, channelID, kind, consumerID, allocationKey, lease.Token, now.Format(time.RFC3339), now.Format(time.RFC3339))
		if insertErr == nil {
			changed = true
		}
		return insertErr
	})
	if err != nil {
		return liveTVTunerLease{}, err
	}
	if changed {
		s.publishDataChanged("data.changed", []string{"live-tv", "dvr"}, "database", "", nil)
	}
	return lease, nil
}

func (s *Server) pruneStaleLiveTVTunerAllocations(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cutoffTime := time.Now().UTC().Add(-liveTVAllocationStaleAfter)
	cutoff := cutoffTime.Format(time.RFC3339)
	rows, err := s.queryUserRead(ctx, `
		SELECT consumer_id FROM live_tv_tuner_allocations
		WHERE allocation_kind = 'live_session' AND heartbeat_at < ?`, cutoff)
	if err != nil {
		return err
	}
	sessionIDs := []string{}
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			_ = rows.Close()
			return err
		}
		sessionIDs = append(sessionIDs, sessionID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	changed := false
	lifecycle := s.playbackLifecycle()
	for _, sessionID := range sessionIDs {
		result, terminateErr := lifecycle.Terminate(ctx, playbackTerminationRequest{
			SessionID: sessionID, Cause: playbackTerminationStale, TunerStaleBefore: &cutoffTime,
		})
		if terminateErr != nil && !errors.Is(terminateErr, sql.ErrNoRows) && !errors.Is(terminateErr, errPlaybackTerminationNotEligible) {
			return terminateErr
		}
		changed = changed || result.Changed || result.AuthorityChanged
	}
	cleanup, err := s.execUserWriteTagged(ctx, []string{}, `
		DELETE FROM live_tv_tuner_allocations
		WHERE heartbeat_at < ? AND (
			allocation_kind = 'dvr_recording'
			OR (allocation_kind = 'live_session' AND NOT EXISTS (
				SELECT 1 FROM playback_sessions ps WHERE ps.id = live_tv_tuner_allocations.consumer_id
			))
		)`, cutoff)
	if err != nil {
		return err
	}
	changed = changed || rowsAffected(cleanup) > 0
	if changed {
		s.publishDataChanged("data.changed", []string{"live-tv", "dvr"}, "database", "", nil)
	}
	return nil
}

func (s *Server) releaseLiveTVTunerAllocation(ctx context.Context, kind, consumerID string) {
	s.releaseLiveTVTunerAllocationLease(ctx, kind, consumerID, "")
}

func (s *Server) releaseLiveTVTunerAllocationLease(ctx context.Context, kind, consumerID, leaseToken string) {
	if ctx == nil {
		ctx = context.Background()
	}
	query := `DELETE FROM live_tv_tuner_allocations WHERE allocation_kind = ? AND consumer_id = ?`
	args := []any{kind, strings.TrimSpace(consumerID)}
	if leaseToken = strings.TrimSpace(leaseToken); leaseToken != "" {
		query += ` AND lease_token = ?`
		args = append(args, leaseToken)
	}
	_, err := s.execUserWriteTagged(ctx, []string{"live-tv", "dvr"}, query, args...)
	if err != nil {
		s.log.Warn("Live TV tuner allocation release failed", "kind", kind, "consumer", consumerID, "error", err)
	}
}

func (s *Server) heartbeatLiveTVTunerAllocationLease(ctx context.Context, kind, consumerID, leaseToken string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := s.execUserWriteTagged(ctx, []string{}, `
		UPDATE live_tv_tuner_allocations SET heartbeat_at = ?
		WHERE allocation_kind = ? AND consumer_id = ? AND lease_token = ?`,
		time.Now().UTC().Format(time.RFC3339), kind, strings.TrimSpace(consumerID), strings.TrimSpace(leaseToken))
	if err != nil {
		return false
	}
	affected, _ := result.RowsAffected()
	return affected == 1
}

func (s *Server) heartbeatLiveTVTunerAllocation(ctx context.Context, sessionID string) {
	if ctx == nil {
		ctx = context.Background()
	}
	_, _ = s.execUserWriteTagged(ctx, []string{}, `UPDATE live_tv_tuner_allocations SET heartbeat_at = ? WHERE allocation_kind = 'live_session' AND consumer_id = ?`, time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(sessionID))
}

func (s *Server) heartbeatLiveTVTunerAllocationForGrant(ctx context.Context, grant string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	grant = strings.TrimSpace(grant)
	if !strings.HasPrefix(grant, "ptc_mg_") {
		return false
	}
	now := time.Now().UTC()
	staleBefore := now.Add(-10 * time.Second).Format(time.RFC3339)
	found := false
	err := s.withUserTxTagged(ctx, []string{}, func(tx *sql.Tx) error {
		var sessionID string
		if err := tx.QueryRow(`
			SELECT playback_session_id FROM playback_media_grants
			WHERE token_hash = ? AND resource_kind = 'live_channel' AND revoked_at = '' LIMIT 1`, hashToken(grant)).Scan(&sessionID); err != nil {
			return nil
		}
		var allocationCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM live_tv_tuner_allocations WHERE allocation_kind = 'live_session' AND consumer_id = ?`, sessionID).Scan(&allocationCount); err != nil || allocationCount != 1 {
			return errMediaGrantDenied
		}
		if _, err := tx.Exec(`
			UPDATE playback_sessions SET last_seen_at = ?
			WHERE id = ? AND is_live = 1 AND ended_at = '' AND last_seen_at < ?`, now.Format(time.RFC3339), sessionID, staleBefore); err != nil {
			return err
		}
		result, err := tx.Exec(`
			UPDATE live_tv_tuner_allocations SET heartbeat_at = ?
			WHERE allocation_kind = 'live_session' AND consumer_id = ? AND heartbeat_at < ?`, now.Format(time.RFC3339), sessionID, staleBefore)
		if err == nil {
			affected, _ := result.RowsAffected()
			found = affected == 1 || allocationCount == 1
		}
		return err
	})
	return err == nil && found
}
