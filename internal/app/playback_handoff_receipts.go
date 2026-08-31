package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/database"
	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

var errPlaybackHandoffInProgress = errors.New("playback handoff is committing")
var errPlaybackReplacementSourceInactive = errors.New("playback replacement source is inactive")
var errPlaybackReplacementRevisionConflict = errors.New("playback replacement source revision changed")
var errPlaybackReplacementAuthorizationChanged = errors.New("playback replacement authorization revision changed")

const playbackHandoffReceiptTTL = cursorDefaultTTL

// Replacement construction can include tuner acquisition and remote media
// inspection. Five minutes exceeds every bounded start path without adding a
// second heartbeat/liveness protocol; truly abandoned claims remain recoverable.
const playbackHandoffClaimTTL = 5 * time.Minute

type playbackHandoffReceipt struct {
	RequestID   string           `json:"requestId"`
	Fingerprint string           `json:"fingerprint"`
	Playback    PlaybackResponse `json:"playback"`
}

type playbackHandoffClaim struct {
	ID                       string
	ReplacementSessionID     string
	AuthorizationRevision    string
	ClientInstanceID         string
	ExpectedQueueRevision    int64
	ExpectedPlaybackRevision int64
}

type playbackReplacementRestoreRequiredError struct {
	ReplacementSessionID string
}

func (e *playbackReplacementRestoreRequiredError) Error() string {
	return "playback replacement committed; active restore is required"
}

func playbackHandoffFingerprint(req PlaybackHandoffRequest) string {
	encoded, _ := json.Marshal(req)
	return hashToken(string(encoded))
}

func (s *Server) consumeDirectPlaybackHandoff(ctx context.Context, user User, sessionID, requestID, fingerprint string, expectedQueueRevision, expectedPlaybackRevision *int64) (*PlaybackResponse, playbackHandoffClaim, error) {
	authorizationRevision, revisionErr := s.authorizationRevisionForUserContextStrict(ctx, user)
	if revisionErr != nil || strings.TrimSpace(authorizationRevision) == "" {
		return nil, playbackHandoffClaim{}, errPlaybackReplacementAuthorizationChanged
	}
	var committed *PlaybackResponse
	var restoreRequired *playbackReplacementRestoreRequiredError
	claim := playbackHandoffClaim{}
	recoveredClaim := false
	recoveredReplacementID := ""
	err := s.withSecurityFenceTxTagged(ctx, []string{"playback"}, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		if _, pruneErr := tx.ExecContext(ctx, `
			UPDATE playback_handoff_receipts SET committed_response = ''
			WHERE state = 'committed' AND committed_response <> '' AND payload_expires_at <= ?`, now.Format(time.RFC3339Nano)); pruneErr != nil {
			return pruneErr
		}
		var storedRequestID, storedFingerprint, state, storedClaimID, encoded, replacementSessionID, claimExpiresAt, payloadExpiresAt string
		err := tx.QueryRowContext(ctx, `
			SELECT request_id, request_fingerprint, state, claim_id, committed_response,
				replacement_session_id, claim_expires_at, payload_expires_at,
				authorization_revision, client_instance_id, expected_queue_revision, expected_playback_revision
			FROM playback_handoff_receipts
			WHERE source_session_id = ? AND user_id = ? AND profile_id = ?`,
			sessionID, accountIDForUser(user), viewerProfileID(user)).
			Scan(&storedRequestID, &storedFingerprint, &state, &storedClaimID, &encoded, &replacementSessionID, &claimExpiresAt, &payloadExpiresAt,
				&claim.AuthorizationRevision, &claim.ClientInstanceID, &claim.ExpectedQueueRevision, &claim.ExpectedPlaybackRevision)
		if err == nil {
			if storedRequestID != requestID || storedFingerprint != fingerprint {
				return errPreparedHandoffConflict
			}
			claim.ReplacementSessionID = replacementSessionID
			if claim.AuthorizationRevision != authorizationRevision {
				return errPlaybackReplacementAuthorizationChanged
			}
			if state == "committing" {
				expires, parseErr := time.Parse(time.RFC3339Nano, claimExpiresAt)
				if parseErr == nil && expires.After(now) {
					return errPlaybackHandoffInProgress
				}
				claim.ID = randomID("hclaim")
				claimed, claimErr := tx.ExecContext(ctx, `
					UPDATE playback_handoff_receipts
					SET claim_id = ?, claim_expires_at = ?
					WHERE source_session_id = ? AND request_id = ? AND request_fingerprint = ?
						AND state = 'committing' AND claim_id = ? AND claim_expires_at = ?`,
					claim.ID, now.Add(playbackHandoffClaimTTL).Format(time.RFC3339Nano),
					sessionID, requestID, fingerprint, storedClaimID, claimExpiresAt)
				if claimErr != nil {
					return claimErr
				}
				if rowsAffected(claimed) != 1 {
					return errPlaybackHandoffInProgress
				}
				recoveredReplacementID = replacementSessionID
				recoveredClaim = true
				_, _ = tx.ExecContext(ctx, `
					UPDATE playback_prepared_handoffs
					SET state = 'prepared', request_id = '', request_fingerprint = ''
					WHERE source_session_id = ? AND state = 'committing'
						AND request_id = ? AND request_fingerprint = ?`,
					sessionID, requestID, fingerprint)
				return nil
			}
			payloadExpiry, expiryErr := time.Parse(time.RFC3339Nano, payloadExpiresAt)
			if encoded == "" || expiryErr != nil || !payloadExpiry.After(now) {
				restoreRequired = &playbackReplacementRestoreRequiredError{ReplacementSessionID: replacementSessionID}
				return nil
			}
			var receipt playbackHandoffReceipt
			if decodeErr := s.decodeContractCursor(encoded, "playback-handoff:"+sessionID, requestID, &receipt, time.Now().UTC()); decodeErr != nil {
				return decodeErr
			}
			if receipt.RequestID != requestID || receipt.Fingerprint != fingerprint || receipt.Playback.SessionID == "" {
				return errPreparedHandoffConflict
			}
			if !playbackResponseCredentialsValid(receipt.Playback, now) {
				restoreRequired = &playbackReplacementRestoreRequiredError{ReplacementSessionID: replacementSessionID}
				return nil
			}
			playback := receipt.Playback
			committed = &playback
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		claim.ID = randomID("hclaim")
		claim.ReplacementSessionID = randomID("play")
		claim.AuthorizationRevision = authorizationRevision
		if sourceErr := tx.QueryRowContext(ctx, `
			SELECT client_instance_id, queue_revision, renegotiation_revision
			FROM playback_sessions
			WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''
				AND state NOT IN ('stopped', 'handoff_pending')`,
			sessionID, accountIDForUser(user), viewerProfileID(user)).Scan(
			&claim.ClientInstanceID, &claim.ExpectedQueueRevision, &claim.ExpectedPlaybackRevision); sourceErr != nil {
			if errors.Is(sourceErr, sql.ErrNoRows) {
				return errPlaybackReplacementSourceInactive
			}
			return sourceErr
		}
		if strings.TrimSpace(claim.ClientInstanceID) != "" {
			var activeAuthorities int
			if countErr := tx.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM playback_sessions
				WHERE user_id = ? AND profile_id = ? AND client_instance_id = ?
					AND ended_at = '' AND state NOT IN ('stopped', 'handoff_pending')`,
				accountIDForUser(user), viewerProfileID(user), claim.ClientInstanceID).Scan(&activeAuthorities); countErr != nil {
				return countErr
			}
			if activeAuthorities != 1 {
				return errPlaybackReplacementRevisionConflict
			}
		}
		if (expectedQueueRevision != nil && *expectedQueueRevision != claim.ExpectedQueueRevision) ||
			(expectedPlaybackRevision != nil && *expectedPlaybackRevision != claim.ExpectedPlaybackRevision) {
			return errPlaybackReplacementRevisionConflict
		}
		result, insertErr := tx.ExecContext(ctx, `
			INSERT INTO playback_handoff_receipts (
				source_session_id, user_id, profile_id, authorization_revision, client_instance_id, request_id, request_fingerprint,
				expected_queue_revision, expected_playback_revision,
				state, claim_id, committed_response, replacement_session_id, created_at, claim_expires_at, payload_expires_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'committing', ?, '', ?, ?, ?, ?)`,
			sessionID, accountIDForUser(user), viewerProfileID(user), authorizationRevision, claim.ClientInstanceID, requestID, fingerprint,
			claim.ExpectedQueueRevision, claim.ExpectedPlaybackRevision, claim.ID, claim.ReplacementSessionID,
			now.Format(time.RFC3339Nano), now.Add(playbackHandoffClaimTTL).Format(time.RFC3339Nano), now.Add(playbackHandoffReceiptTTL).Format(time.RFC3339Nano))
		if insertErr != nil {
			return insertErr
		}
		if rowsAffected(result) != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err != nil || committed != nil {
		return committed, claim, err
	}
	if restoreRequired != nil {
		return nil, claim, restoreRequired
	}
	if recoveredClaim {
		if recoveredReplacementID != "" {
			if cleanupErr := s.cleanupRecoveredHandoffReplacement(ctx, user, recoveredReplacementID); cleanupErr != nil {
				return nil, playbackHandoffClaim{}, cleanupErr
			}
		}
		claim.ReplacementSessionID = randomID("play")
		reserved, reserveErr := s.execPlaybackWrite(ctx, `
			UPDATE playback_handoff_receipts SET replacement_session_id = ?
			WHERE source_session_id = ? AND request_id = ? AND request_fingerprint = ?
				AND state = 'committing' AND claim_id = ? AND replacement_session_id = ?`,
			claim.ReplacementSessionID, sessionID, requestID, fingerprint, claim.ID, recoveredReplacementID)
		if reserveErr != nil {
			return nil, playbackHandoffClaim{}, reserveErr
		}
		if rowsAffected(reserved) != 1 {
			return nil, playbackHandoffClaim{}, errPlaybackHandoffInProgress
		}
	}
	return nil, claim, nil
}

func playbackResponseCredentialsValid(playback PlaybackResponse, now time.Time) bool {
	if strings.TrimSpace(playback.MediaGrant.Token) == "" {
		return false
	}
	grantExpiry, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(playback.MediaGrant.ExpiresAt))
	if err != nil {
		grantExpiry, err = time.Parse(time.RFC3339, strings.TrimSpace(playback.MediaGrant.ExpiresAt))
	}
	if err != nil || !grantExpiry.After(now) || playback.ContinuationCredential == nil || strings.TrimSpace(playback.ContinuationCredential.Token) == "" {
		return false
	}
	continuationExpiry, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(playback.ContinuationCredential.ExpiresAt))
	return err == nil && continuationExpiry.After(now)
}

func (s *Server) prunePlaybackReplacementPayloads(ctx context.Context, now time.Time) error {
	nowText := now.UTC().Format(time.RFC3339Nano)
	if err := s.pruneCastPlaybackTransfers(ctx, now); err != nil {
		return err
	}
	if err := s.pruneExpiredPlaybackReceiverPreparations(ctx, now); err != nil {
		return err
	}
	return s.withSecurityFenceTxTagged(ctx, []string{"playback"}, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE playback_handoff_receipts SET committed_response = ''
			WHERE state = 'committed' AND committed_response <> '' AND payload_expires_at <= ?`, nowText); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE playback_prepared_handoffs SET committed_response = ''
			WHERE state = 'committed' AND committed_response <> '' AND expires_at <= ?`, nowText)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE cast_bootstraps
			SET bootstrap_response_json = '', playback_envelope = '', receiver_public_key = ''
			WHERE transfer_state = 'committed' AND payload_expires_at <> '' AND payload_expires_at <= ?`, nowText); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE cast_receiver_sessions SET last_advance_json = ''
			WHERE last_advance_json <> '' AND last_advance_payload_expires_at <> '' AND last_advance_payload_expires_at <= ?`, nowText)
		return err
	})
}

func (s *Server) pruneCastPlaybackTransfers(ctx context.Context, now time.Time) error {
	rows, err := s.queryUserRead(ctx, `
		SELECT playback_session_id, user_id, profile_id, source_playback_session_id,
			replacement_request_id, replacement_fingerprint, replacement_claim_id
		FROM cast_bootstraps
		WHERE transfer_state = 'pending' AND expires_at <= ?`, now.UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	type expiredTransfer struct{ playback, user, profile, source, request, fingerprint, claim string }
	var expired []expiredTransfer
	for rows.Next() {
		var item expiredTransfer
		if err := rows.Scan(&item.playback, &item.user, &item.profile, &item.source, &item.request, &item.fingerprint, &item.claim); err != nil {
			rows.Close()
			return err
		}
		expired = append(expired, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range expired {
		user := User{ID: item.user, AccountID: item.user, ProfileID: item.profile}
		if expireErr := s.expireCastBootstrapTransfer(ctx, user, item.playback, item.source, item.request, item.fingerprint, item.claim); expireErr != nil {
			return expireErr
		}
	}
	advanceRows, err := s.queryUserRead(ctx, `
		SELECT id, user_id, profile_id, playback_session_id, client_instance_id, generation,
			automatic_advances, pending_playback_session_id, pending_generation, pending_request_id,
			pending_fingerprint, pending_claim_id, pending_advance_id
		FROM cast_receiver_sessions
		WHERE pending_playback_session_id <> '' AND pending_expires_at <> '' AND pending_expires_at <= ?`, now.UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	var pendingAdvances []castReceiverRecord
	for advanceRows.Next() {
		var record castReceiverRecord
		if err := advanceRows.Scan(&record.ID, &record.UserID, &record.ProfileID, &record.PlaybackSessionID, &record.ClientInstanceID, &record.Generation,
			&record.AutomaticAdvances, &record.PendingPlaybackSessionID, &record.PendingGeneration, &record.PendingRequestID,
			&record.PendingFingerprint, &record.PendingClaimID, &record.PendingAdvanceID); err != nil {
			advanceRows.Close()
			return err
		}
		pendingAdvances = append(pendingAdvances, record)
	}
	if err := advanceRows.Close(); err != nil {
		return err
	}
	for _, record := range pendingAdvances {
		user, userErr := s.castUserForScope(ctx, record.UserID, record.ProfileID)
		if userErr != nil {
			return userErr
		}
		if err := s.cancelCastReceiverAdvance(ctx, castSessionAuth{record: record, user: user, viaReceiver: true}); err != nil && !errors.Is(err, errPreparedHandoffConflict) {
			return err
		}
	}
	return nil
}

func (s *Server) cleanupRecoveredHandoffReplacement(ctx context.Context, user User, replacementSessionID string) error {
	_, err := s.playbackLifecycle().Terminate(ctx, playbackTerminationRequest{
		SessionID: replacementSessionID, UserID: accountIDForUser(user), ProfileID: viewerProfileID(user),
		Cause: playbackTerminationFailedStart, RemoveSession: true,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func (s *Server) rollbackDirectPlaybackHandoff(ctx context.Context, sessionID, requestID, fingerprint, claimID string) {
	_, _ = s.execPlaybackWrite(ctx, `
		DELETE FROM playback_handoff_receipts
		WHERE source_session_id = ? AND request_id = ? AND request_fingerprint = ?
			AND state = 'committing' AND claim_id = ?`,
		sessionID, requestID, fingerprint, claimID)
}

func (s *Server) commitDirectPlaybackHandoff(ctx context.Context, user User, sourceSessionID string, terminal PlaybackTerminalEvent, requestID, fingerprint string, claim playbackHandoffClaim, playback PlaybackResponse) error {
	return s.commitDirectPlaybackHandoffWithTx(ctx, user, sourceSessionID, terminal, requestID, fingerprint, claim, playback, nil)
}

// commitDirectPlaybackHandoffWithTx is the single atomic replacement boundary.
// Callers that publish another authority record must do so through onCommit so
// no observer can see a closed source paired with an uncommitted successor, or
// an active successor without its accepted source-terminal receipt.
func (s *Server) commitDirectPlaybackHandoffWithTx(
	ctx context.Context,
	user User,
	sourceSessionID string,
	terminal PlaybackTerminalEvent,
	requestID, fingerprint string,
	claim playbackHandoffClaim,
	playback PlaybackResponse,
	onCommit func(*sql.Tx) error,
) error {
	if strings.TrimSpace(playback.SessionID) == "" || playback.SessionID != claim.ReplacementSessionID {
		return errPreparedHandoffConflict
	}
	receipt := playbackHandoffReceipt{RequestID: requestID, Fingerprint: fingerprint, Playback: playback}
	encoded, err := s.encodeContractCursor("playback-handoff:"+sourceSessionID, requestID, receipt, time.Now().UTC())
	if err != nil {
		return err
	}
	lifecycle := s.playbackLifecycle()
	preferences := s.playbackProgressPreferencesForUserContext(ctx, viewerProfileID(user))
	var termination playbackTerminationResult
	err = s.withWorkClassTxTaggedForViewer(ctx, foundationcontract.WorkClassPlaybackStart, "playback_handoff_commit_tx", database.UserWriteRetry,
		accountIDForUser(user), viewerProfileID(user), []string{"playback", "playback-progress", "media-state", "library-items", "home"}, func(tx *sql.Tx) error {
			currentAuthorizationRevision, authorizationErr := authorizationRevisionForUserRow(user, tx.QueryRowContext(ctx, authorizationRevisionQuery, authorizationRevisionIdentity(user)...))
			if authorizationErr != nil || currentAuthorizationRevision != claim.AuthorizationRevision {
				return errPlaybackReplacementAuthorizationChanged
			}
			var sourceActive int
			if sourceErr := tx.QueryRowContext(ctx, `
				SELECT 1 FROM playback_sessions
				WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''
					AND state NOT IN ('stopped', 'handoff_pending')`,
				sourceSessionID, accountIDForUser(user), viewerProfileID(user)).Scan(&sourceActive); sourceErr != nil {
				if errors.Is(sourceErr, sql.ErrNoRows) {
					return errPlaybackReplacementSourceInactive
				}
				return sourceErr
			}
			var revisionMatch int
			if revisionErr := tx.QueryRowContext(ctx, `
			SELECT 1 FROM playback_sessions
			WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''
				AND state NOT IN ('stopped', 'handoff_pending')
				AND client_instance_id = ? AND queue_revision = ? AND renegotiation_revision = ?
				AND (client_instance_id = '' OR 1 = (SELECT COUNT(*) FROM playback_sessions active
					WHERE active.user_id = ? AND active.profile_id = ? AND active.client_instance_id = ?
						AND active.ended_at = '' AND active.state NOT IN ('stopped', 'handoff_pending')))`,
				sourceSessionID, accountIDForUser(user), viewerProfileID(user),
				claim.ClientInstanceID, claim.ExpectedQueueRevision, claim.ExpectedPlaybackRevision,
				accountIDForUser(user), viewerProfileID(user), claim.ClientInstanceID).Scan(&revisionMatch); revisionErr != nil {
				if errors.Is(revisionErr, sql.ErrNoRows) {
					return errPlaybackReplacementRevisionConflict
				}
				return revisionErr
			}
			termination, err = lifecycle.terminateTx(ctx, tx, playbackTerminationRequest{
				SessionID: sourceSessionID, UserID: accountIDForUser(user), ProfileID: viewerProfileID(user),
				Cause: playbackTerminationHandoff, RequireActive: true, Event: &terminal, ProgressPreferences: preferences,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return errPlaybackReplacementSourceInactive
				}
				return err
			}
			activated, activateErr := tx.ExecContext(ctx, `
			UPDATE playback_sessions SET state = 'playing'
			WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = '' AND state = 'handoff_pending'`,
				playback.SessionID, accountIDForUser(user), viewerProfileID(user))
			if activateErr != nil {
				return activateErr
			}
			if rowsAffected(activated) != 1 {
				return errPreparedHandoffConflict
			}
			result, updateErr := tx.ExecContext(ctx, `
			UPDATE playback_handoff_receipts SET state = 'committed', committed_response = ?, payload_expires_at = ?
			WHERE source_session_id = ? AND request_id = ? AND request_fingerprint = ?
				AND state = 'committing' AND claim_id = ?`,
				encoded, time.Now().UTC().Add(playbackHandoffReceiptTTL).Format(time.RFC3339Nano),
				sourceSessionID, requestID, fingerprint, claim.ID)
			if updateErr != nil {
				return updateErr
			}
			if rowsAffected(result) != 1 {
				return errPreparedHandoffConflict
			}
			if onCommit != nil {
				if callbackErr := onCommit(tx); callbackErr != nil {
					return callbackErr
				}
			}
			return nil
		})
	if err != nil {
		return err
	}
	lifecycle.afterCommit(ctx, termination)
	return nil
}
