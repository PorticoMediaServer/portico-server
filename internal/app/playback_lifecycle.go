package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/database"
	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

// PlaybackTerminationCause records why the server is retiring playback. The
// cause intentionally does not collapse completion semantics: an ordered
// terminal event, when present, remains the authority for completed vs stopped
// progress.
type PlaybackTerminationCause string

const (
	playbackTerminationExplicit        PlaybackTerminationCause = "explicit"
	playbackTerminationCommand         PlaybackTerminationCause = "command"
	playbackTerminationStale           PlaybackTerminationCause = "stale"
	playbackTerminationReplacement     PlaybackTerminationCause = "replacement"
	playbackTerminationHandoff         PlaybackTerminationCause = "handoff"
	playbackTerminationFailedStart     PlaybackTerminationCause = "failed_start"
	playbackTerminationProducerFailure PlaybackTerminationCause = "producer_failure"
	playbackTerminationAuthorization   PlaybackTerminationCause = "authorization"
	playbackTerminationReceiver        PlaybackTerminationCause = "receiver"
)

type playbackTerminalEvent = PlaybackTerminalEvent

var errPlaybackTerminalEvidenceInvalid = errors.New("playback terminal evidence is invalid")
var errPlaybackTerminalDurationMismatch = errors.New("playback terminal duration does not match authoritative duration")

// Completion duration admits ordinary container/probe rounding while rejecting
// client-invented short durations. One percent scales for long-form media; the
// cap prevents that tolerance from becoming a material watched-position jump.
func playbackCompletionDurationTolerance(authoritativeSeconds float64) float64 {
	return math.Min(30, math.Max(2, authoritativeSeconds*0.01))
}

type playbackTerminationRequest struct {
	SessionID string
	UserID    string
	ProfileID string
	Cause     PlaybackTerminationCause
	Event     *playbackTerminalEvent

	// RequireActive preserves the public terminal endpoint's not-found response
	// for an already closed session. Internal cleanup is otherwise idempotent and
	// also reconciles authority left by an interrupted historical terminal path.
	RequireActive bool
	RemoveSession bool

	// StaleBefore is an optional compare-and-terminate fence evaluated inside the
	// same transaction as terminalization. AudioStaleBefore supplies the longer
	// background-audio lease without creating another terminal implementation.
	StaleBefore      *time.Time
	AudioStaleBefore *time.Time
	TunerStaleBefore *time.Time

	Now                 time.Time
	ProgressPreferences PlaybackProgressPreferences
	TerminalReceipt     *playbackTerminalReceipt
}

type playbackTerminationResult struct {
	SessionID         string
	Cause             PlaybackTerminationCause
	UserID            string
	ProfileID         string
	MediaID           string
	Changed           bool
	AlreadyTerminated bool
	Removed           bool
	ProgressWritten   bool
	AuthorityChanged  bool
}

var errPlaybackTerminationNotEligible = errors.New("playback session is not eligible for termination")

// PlaybackLifecycle is the single owner of playback terminal state. Callers
// may add their own ownership mutation to the surrounding transaction through
// terminateTx, but they do not duplicate session/grant/continuation/tuner SQL.
type PlaybackLifecycle struct {
	server *Server
}

func (s *Server) playbackLifecycle() PlaybackLifecycle {
	return PlaybackLifecycle{server: s}
}

func (l PlaybackLifecycle) Terminate(ctx context.Context, req playbackTerminationRequest) (playbackTerminationResult, error) {
	if l.server == nil {
		return playbackTerminationResult{}, errors.New("playback lifecycle server is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.UserID = strings.TrimSpace(req.UserID)
	req.ProfileID = strings.TrimSpace(req.ProfileID)
	if req.SessionID == "" {
		return playbackTerminationResult{}, sql.ErrNoRows
	}
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	} else {
		req.Now = req.Now.UTC()
	}

	// Profile preferences are immutable input to the retried transaction. Resolve
	// identity only when a system caller did not already load it.
	if req.ProfileID == "" || req.UserID == "" {
		var userID, profileID string
		if err := l.server.queryUserRow(ctx, `SELECT user_id, profile_id FROM playback_sessions WHERE id = ?`, req.SessionID).Scan(&userID, &profileID); err != nil {
			return playbackTerminationResult{}, err
		}
		if req.UserID == "" {
			req.UserID = userID
		}
		if req.ProfileID == "" {
			req.ProfileID = profileID
		}
	}
	if req.ProgressPreferences == (PlaybackProgressPreferences{}) {
		req.ProgressPreferences = l.server.playbackProgressPreferencesForUserContext(ctx, req.ProfileID)
	}

	var result playbackTerminationResult
	err := l.server.withWorkClassTxTaggedForViewer(ctx, foundationcontract.WorkClassSecurityFence, "playback_terminal_tx", database.UserWriteRetry,
		req.UserID, req.ProfileID, []string{"playback", "playback-progress", "media-state", "library-items", "home", "live-tv"}, func(tx *sql.Tx) error {
			var err error
			result, err = l.terminateTx(ctx, tx, req)
			return err
		})
	if err != nil {
		return playbackTerminationResult{}, err
	}
	l.afterCommit(ctx, result)
	return result, nil
}

type playbackTerminationSession struct {
	UserID              string
	ProfileID           string
	MediaID             string
	MediaType           string
	LastSeenAt          string
	EndedAt             string
	State               string
	IsLive              int
	HistoryPaused       int
	Progress            int
	PositionSeconds     int
	DurationSeconds     int
	ProgressGeneration  int64
	LastEventSequence   int64
	LastEventRecordedAt string
}

func (l PlaybackLifecycle) terminateTx(ctx context.Context, tx *sql.Tx, req playbackTerminationRequest) (playbackTerminationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	}
	if req.Event != nil && validatePlaybackTerminalEvent(*req.Event) != nil {
		return playbackTerminationResult{}, errPlaybackTerminalEvidenceInvalid
	}
	var session playbackTerminationSession
	err := tx.QueryRowContext(ctx, `
		SELECT ps.user_id, ps.profile_id, ps.media_id, ps.media_type, ps.last_seen_at,
			ps.ended_at, ps.state, ps.is_live, ps.history_paused, ps.progress,
			ps.position_seconds, COALESCE(m.duration_seconds, 0),
			ps.progress_generation, ps.last_event_sequence, ps.last_event_recorded_at
		FROM playback_sessions ps
		LEFT JOIN media_items m ON m.id = ps.media_id
		WHERE ps.id = ?`, strings.TrimSpace(req.SessionID)).Scan(
		&session.UserID, &session.ProfileID, &session.MediaID, &session.MediaType, &session.LastSeenAt,
		&session.EndedAt, &session.State, &session.IsLive, &session.HistoryPaused, &session.Progress,
		&session.PositionSeconds, &session.DurationSeconds,
		&session.ProgressGeneration, &session.LastEventSequence, &session.LastEventRecordedAt)
	if err != nil {
		return playbackTerminationResult{}, err
	}
	if req.UserID != "" && session.UserID != req.UserID {
		return playbackTerminationResult{}, sql.ErrNoRows
	}
	if req.ProfileID != "" && session.ProfileID != req.ProfileID {
		return playbackTerminationResult{}, sql.ErrNoRows
	}

	result := playbackTerminationResult{
		SessionID: req.SessionID, Cause: req.Cause, UserID: session.UserID, ProfileID: session.ProfileID, MediaID: session.MediaID,
	}
	active := strings.TrimSpace(session.EndedAt) == "" && !strings.EqualFold(strings.TrimSpace(session.State), "stopped")
	if !active {
		if req.RequireActive {
			return playbackTerminationResult{}, sql.ErrNoRows
		}
		result.AlreadyTerminated = true
	}
	if active && req.StaleBefore != nil {
		cutoff := req.StaleBefore.UTC()
		if (session.MediaType == "track" || session.MediaType == "audiobook") && req.AudioStaleBefore != nil {
			cutoff = req.AudioStaleBefore.UTC()
		}
		lastSeen, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(session.LastSeenAt))
		if parseErr != nil {
			return playbackTerminationResult{}, parseErr
		}
		if !lastSeen.Before(cutoff) {
			return playbackTerminationResult{}, errPlaybackTerminationNotEligible
		}
	}
	if active && req.TunerStaleBefore != nil {
		var staleAllocation int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM live_tv_tuner_allocations
			WHERE allocation_kind = 'live_session' AND consumer_id = ? AND heartbeat_at < ?`,
			req.SessionID, req.TunerStaleBefore.UTC().Format(time.RFC3339)).Scan(&staleAllocation); err != nil {
			return playbackTerminationResult{}, err
		}
		if staleAllocation != 1 {
			return playbackTerminationResult{}, errPlaybackTerminationNotEligible
		}
	}

	positionSeconds := max(0, session.PositionSeconds)
	durationSeconds := max(0, session.DurationSeconds)
	completed := false
	recordedAt := strings.TrimSpace(session.LastEventRecordedAt)
	if recordedAt == "" {
		recordedAt = strings.TrimSpace(session.LastSeenAt)
	}
	if recordedAt == "" {
		recordedAt = req.Now.Format(time.RFC3339Nano)
	}
	if req.Event != nil {
		if !active {
			return playbackTerminationResult{}, sql.ErrNoRows
		}
		if req.Event.Generation != session.ProgressGeneration {
			return playbackTerminationResult{}, errPlaybackGenerationStale
		}
		if req.Event.EventSequence <= session.LastEventSequence {
			return playbackTerminationResult{}, errPlaybackEventSequenceStale
		}
		completed = strings.EqualFold(strings.TrimSpace(req.Event.Disposition), "completed")
		if completed && (session.IsLive == 1 || session.DurationSeconds <= 0) {
			return playbackTerminationResult{}, errPlaybackTerminalDurationMismatch
		}
		positionSeconds = max(0, int(math.Round(req.Event.PositionSeconds)))
		clientDurationSeconds := max(0, int(math.Round(req.Event.DurationSeconds)))
		if completed && session.DurationSeconds > 0 {
			if math.Abs(req.Event.DurationSeconds-float64(session.DurationSeconds)) > playbackCompletionDurationTolerance(float64(session.DurationSeconds)) {
				return playbackTerminationResult{}, errPlaybackTerminalDurationMismatch
			}
			// Watched normalization uses the server's measured finite duration;
			// the exact client event remains preserved in its receipt/ack.
			durationSeconds = session.DurationSeconds
			positionSeconds = session.DurationSeconds
		} else {
			durationSeconds = clientDurationSeconds
		}
		if value := strings.TrimSpace(req.Event.RecordedAt); value != "" {
			recordedAt = value
		}
	} else if positionSeconds <= 0 && session.Progress > 0 && durationSeconds > 0 {
		positionSeconds = max(1, int(math.Round(float64(durationSeconds)*float64(session.Progress)/100)))
	}

	endedAt := req.Now.Format(time.RFC3339Nano)
	if active {
		progress := sessionProgressPercent(positionSeconds, durationSeconds, session.IsLive == 1)
		terminalState := "stopped"
		if req.Cause == playbackTerminationProducerFailure {
			terminalState = "failed"
		}
		var updateResult sql.Result
		if req.Event != nil {
			updateResult, err = tx.ExecContext(ctx, `
				UPDATE playback_sessions
				SET last_seen_at = ?, ended_at = ?, state = ?, progress = ?,
					position_seconds = ?, last_event_sequence = ?,
					last_event_recorded_at = ?, last_event_received_at = ?
				WHERE id = ? AND ended_at = '' AND state <> 'stopped'
					AND progress_generation = ? AND last_event_sequence < ?`,
				endedAt, endedAt, terminalState, progress, positionSeconds, req.Event.EventSequence,
				recordedAt, endedAt, req.SessionID, req.Event.Generation, req.Event.EventSequence)
		} else {
			updateResult, err = tx.ExecContext(ctx, `
				UPDATE playback_sessions
				SET last_seen_at = ?, ended_at = ?, state = ?, progress = ?, position_seconds = ?
				WHERE id = ? AND ended_at = '' AND state <> 'stopped'`,
				endedAt, endedAt, terminalState, progress, positionSeconds, req.SessionID)
		}
		if err != nil {
			return playbackTerminationResult{}, err
		}
		if rowsAffected(updateResult) != 1 {
			if req.Event != nil {
				return playbackTerminationResult{}, errPlaybackEventSequenceStale
			}
			return playbackTerminationResult{}, sql.ErrNoRows
		}
		result.Changed = true

		if req.TerminalReceipt != nil {
			receipt := req.TerminalReceipt
			if strings.TrimSpace(receipt.RequestID) == "" || strings.TrimSpace(receipt.Fingerprint) == "" || strings.TrimSpace(receipt.AuthorizationRevision) == "" || !json.Valid([]byte(receipt.ResponseJSON)) {
				return playbackTerminationResult{}, errors.New("playback terminal receipt is invalid")
			}
			currentAuthorizationRevision, authorizationErr := authorizationRevisionForUserRow(
				User{ID: session.UserID, AccountID: session.UserID, ProfileID: session.ProfileID},
				tx.QueryRowContext(ctx, authorizationRevisionQuery, session.UserID, session.ProfileID),
			)
			if authorizationErr != nil || currentAuthorizationRevision != receipt.AuthorizationRevision {
				return playbackTerminationResult{}, errPlaybackTerminalAuthorizationChanged
			}
			if _, receiptErr := tx.ExecContext(ctx, `
				INSERT INTO playback_session_terminal_receipts (
					playback_session_id, user_id, profile_id, authorization_revision, request_id, request_fingerprint, response_json,
					credential_token_hash, credential_origin, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				req.SessionID, session.UserID, session.ProfileID, receipt.AuthorizationRevision, receipt.RequestID, receipt.Fingerprint, receipt.ResponseJSON,
				receipt.CredentialTokenHash, receipt.CredentialOrigin, endedAt); receiptErr != nil {
				return playbackTerminationResult{}, receiptErr
			}
		}

		if session.IsLive == 0 && session.HistoryPaused == 0 && strings.TrimSpace(session.MediaID) != "" && positionSeconds > 0 {
			normalized := normalizePlaybackProgressState(positionSeconds, completed, durationSeconds, session.MediaType, req.ProgressPreferences)
			lastPlayed := ""
			if session.MediaType == "track" || normalized.Started || normalized.Watched {
				lastPlayed = req.Now.Format(time.RFC3339)
			}
			progressQuery := `
				INSERT INTO user_media_state (
					profile_id, user_id, media_id, watched, progress_seconds, last_played_at,
					updated_at, progress_session_id, progress_recorded_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(profile_id, media_id) DO UPDATE SET
					watched = excluded.watched,
					progress_seconds = excluded.progress_seconds,
					last_played_at = CASE WHEN excluded.last_played_at <> '' THEN excluded.last_played_at ELSE user_media_state.last_played_at END,
					updated_at = excluded.updated_at,
					progress_session_id = excluded.progress_session_id,
					progress_recorded_at = excluded.progress_recorded_at`
			progressArgs := []any{
				session.ProfileID, session.UserID, session.MediaID, boolInt(normalized.Watched), normalized.ProgressSeconds,
				lastPlayed, req.Now.Format(time.RFC3339), req.SessionID, recordedAt,
			}
			if req.Event == nil {
				// An implicit lease/command cleanup has no new observation. Do not
				// overwrite progress already persisted by a later handoff or event.
				progressQuery += `
					WHERE user_media_state.progress_recorded_at = ''
						OR excluded.progress_recorded_at = ''
						OR user_media_state.progress_recorded_at <= excluded.progress_recorded_at`
			}
			progressResult, progressErr := tx.ExecContext(ctx, progressQuery, progressArgs...)
			err = progressErr
			if err != nil {
				return playbackTerminationResult{}, err
			}
			result.ProgressWritten = rowsAffected(progressResult) > 0
		}
	}

	// All bearer and overlap authority is fenced in the same transaction as the
	// session row. No terminal caller performs these mutations independently.
	authorityStatements := []struct {
		query string
		args  []any
	}{
		{`UPDATE playback_media_grants SET revoked_at = ? WHERE playback_session_id = ? AND revoked_at = ''`, []any{endedAt, req.SessionID}},
		{`UPDATE playback_session_continuation_credentials SET revoked_at = ?, previous_valid_until = '' WHERE playback_session_id = ? AND revoked_at = ''`, []any{endedAt, req.SessionID}},
		{`DELETE FROM live_tv_tuner_allocations WHERE allocation_kind = 'live_session' AND consumer_id = ?`, []any{req.SessionID}},
		{`DELETE FROM library_channel_playback_policies WHERE playback_session_id = ?`, []any{req.SessionID}},
		{`DELETE FROM playback_prepared_handoffs WHERE source_session_id = ? AND state = 'prepared'`, []any{req.SessionID}},
		{`UPDATE cast_receiver_sessions SET status = 'stopped', stopped_at = ?, last_seen_at = ? WHERE playback_session_id = ? AND status = 'active' AND pending_playback_session_id = ''`, []any{endedAt, endedAt, req.SessionID}},
	}
	for _, statement := range authorityStatements {
		mutation, mutationErr := tx.ExecContext(ctx, statement.query, statement.args...)
		if mutationErr != nil {
			return playbackTerminationResult{}, mutationErr
		}
		if rowsAffected(mutation) > 0 {
			result.AuthorityChanged = true
		}
	}

	if req.RemoveSession {
		removed, removeErr := tx.ExecContext(ctx, `DELETE FROM playback_sessions WHERE id = ?`, req.SessionID)
		if removeErr != nil {
			return playbackTerminationResult{}, removeErr
		}
		result.Removed = rowsAffected(removed) == 1
	}
	return result, nil
}

func (l PlaybackLifecycle) afterCommit(_ context.Context, result playbackTerminationResult) {
	if l.server == nil || result.SessionID == "" {
		return
	}
	l.server.forgetMediaGrantsForPlaybackSession(result.SessionID)
	if result.ProgressWritten {
		l.server.invalidateHomeCacheForProfile(result.ProfileID)
	}
	if !result.Removed {
		if err := l.server.completeEndedPlaybackSessionHistoryContext(context.Background(), result.SessionID); err != nil {
			l.server.recordLog("warn", "Playback terminal history projection failed", map[string]string{"session": result.SessionID, "error": err.Error()})
		}
	}
	l.server.stopTranscodeSessionForPlaybackSession(result.SessionID)
	if strings.TrimSpace(result.MediaID) != "" && !l.server.hasActivePlaybackForMedia(result.MediaID) {
		l.server.stopTranscodeSessionForMedia(result.MediaID)
	}
	if result.Changed || result.AuthorityChanged {
		l.server.notifyPlaybackCommand(result.SessionID)
	}
	if result.Changed {
		l.server.recordLog("info", "Playback session terminated", map[string]string{"session": result.SessionID, "cause": string(result.Cause)})
	}
}
