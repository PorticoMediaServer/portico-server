package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	playbackContinuationTTL         = 5 * time.Minute
	playbackContinuationAbsoluteCap = 12 * time.Hour
	playbackContinuationOverlap     = 30 * time.Second
	playbackContinuationRenewBefore = 2 * time.Minute
)

var (
	errPlaybackContinuationDenied           = errors.New("playback continuation credential is invalid, expired, revoked, or out of scope")
	errPlaybackContinuationRotationConflict = errors.New("playback continuation rotation request conflicts with a prior request")
	errPlaybackContinuationRotationReceipt  = errors.New("playback continuation rotation receipt cannot be recovered")
	errPlaybackGenerationStale              = errors.New("playback progress authority generation is stale")
	errPlaybackEventSequenceStale           = errors.New("playback terminal event sequence is stale")
)

type playbackContinuationRotationReceipt struct {
	Credential  PlaybackContinuationCredential `json:"credential"`
	RequestID   string                         `json:"requestId"`
	Fingerprint string                         `json:"fingerprint"`
}

func (s *Server) encodePlaybackContinuationRotationReceipt(sessionID string, receipt playbackContinuationRotationReceipt) (string, error) {
	return s.encodeContractCursor("playback-continuation-rotation:"+strings.TrimSpace(sessionID), strings.TrimSpace(receipt.RequestID), receipt, time.Now().UTC())
}

func (s *Server) decodePlaybackContinuationRotationReceipt(sessionID, requestID, encoded string) (PlaybackContinuationCredential, error) {
	var receipt playbackContinuationRotationReceipt
	if err := s.decodeContractCursor(encoded, "playback-continuation-rotation:"+strings.TrimSpace(sessionID), strings.TrimSpace(requestID), &receipt, time.Now().UTC()); err != nil {
		return PlaybackContinuationCredential{}, err
	}
	if receipt.RequestID != strings.TrimSpace(requestID) || receipt.Fingerprint == "" || receipt.Credential.Token == "" {
		return PlaybackContinuationCredential{}, errPlaybackContinuationDenied
	}
	return receipt.Credential, nil
}

func playbackContinuationOrigin(r *http.Request) string {
	if r == nil {
		return ""
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		parsed, err := url.Parse(origin)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" && parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == "" {
			return origin
		}
	}
	scheme := sRequestScheme(r)
	if strings.TrimSpace(r.Host) == "" {
		return ""
	}
	return scheme + "://" + r.Host
}
func sRequestScheme(r *http.Request) string {
	if requestUsesTLS(r) {
		return "https"
	}
	return "http"
}

func (s *Server) ensurePlaybackContinuationCredential(r *http.Request, user User, playback *PlaybackResponse) error {
	if playback == nil || strings.TrimSpace(playback.SessionID) == "" {
		return errors.New("playback session is required for continuity")
	}
	credential, err := s.issuePlaybackContinuationCredential(r, user, playback.SessionID, int64(playback.Generation))
	if err != nil {
		s.log.Warn("playback continuation issue failed", "session", playback.SessionID, "error", err)
		return err
	}
	playback.ContinuationCredential = &credential
	if playback.Generation == 0 {
		playback.Generation = int(credential.Generation)
	}
	return nil
}

func (s *Server) issuePlaybackContinuationCredential(r *http.Request, user User, sessionID string, responseGeneration int64) (PlaybackContinuationCredential, error) {
	origin := playbackContinuationOrigin(r)
	if origin == "" {
		return PlaybackContinuationCredential{}, errPlaybackContinuationDenied
	}
	now := time.Now().UTC()
	var credential PlaybackContinuationCredential
	err := s.withUserTxTaggedForViewer(r.Context(), accountIDForUser(user), viewerProfileID(user), []string{"playback"}, func(tx *sql.Tx) error {
		var err error
		credential, err = s.issuePlaybackContinuationCredentialTx(r.Context(), tx, user, sessionID, responseGeneration, origin, now, nil)
		return err
	})
	return credential, err
}

type playbackContinuationRotationRequest struct {
	RequestID   string
	Fingerprint string
}

func (s *Server) issuePlaybackContinuationCredentialTx(ctx context.Context, tx *sql.Tx, user User, sessionID string, responseGeneration int64, origin string, now time.Time, rotation *playbackContinuationRotationRequest) (PlaybackContinuationCredential, error) {
	var clientInstanceID string
	var generation int64
	var existingAbsolute, existingOrigin, existingRevoked string
	var lastRotationID, lastRotationFingerprint, lastRotationReceipt string
	if err := tx.QueryRowContext(ctx, `
		SELECT ps.client_instance_id, ps.progress_generation,
			COALESCE(c.absolute_expires_at, ''), COALESCE(c.origin, ''), COALESCE(c.revoked_at, ''),
			COALESCE(c.last_rotation_request_id, ''), COALESCE(c.last_rotation_fingerprint, ''), COALESCE(c.last_rotation_receipt, '')
		FROM playback_sessions ps
		LEFT JOIN playback_session_continuation_credentials c ON c.playback_session_id = ps.id
		WHERE ps.id = ? AND ps.profile_id = ? AND ps.ended_at = '' AND ps.state <> 'stopped'`, sessionID, viewerProfileID(user)).Scan(
		&clientInstanceID, &generation, &existingAbsolute, &existingOrigin, &existingRevoked,
		&lastRotationID, &lastRotationFingerprint, &lastRotationReceipt); err != nil {
		return PlaybackContinuationCredential{}, err
	}
	if rotation != nil && lastRotationID == rotation.RequestID {
		if lastRotationFingerprint != rotation.Fingerprint {
			return PlaybackContinuationCredential{}, errPlaybackContinuationRotationConflict
		}
		credential, err := s.decodePlaybackContinuationRotationReceipt(sessionID, rotation.RequestID, lastRotationReceipt)
		if err != nil {
			return PlaybackContinuationCredential{}, errPlaybackContinuationRotationReceipt
		}
		return credential, nil
	}
	if generation <= 0 {
		generation = max64(1, responseGeneration)
	}
	if existingOrigin != "" && existingOrigin != origin {
		return PlaybackContinuationCredential{}, errPlaybackContinuationDenied
	}
	absolute := now.Add(playbackContinuationAbsoluteCap)
	if parsed, parseErr := time.Parse(time.RFC3339Nano, existingAbsolute); parseErr == nil && parsed.After(now) {
		absolute = parsed
	}
	if existingRevoked != "" || !absolute.After(now) {
		absolute = now.Add(playbackContinuationAbsoluteCap)
	}
	token := "ptc_pb_" + randomToken()
	expires := now.Add(playbackContinuationTTL)
	if absolute.Before(expires) {
		expires = absolute
	}
	previousUntil := now.Add(playbackContinuationOverlap)
	rotationID, rotationFingerprint, rotationReceipt := "", "", ""
	if rotation != nil {
		rotationID, rotationFingerprint = rotation.RequestID, rotation.Fingerprint
		var err error
		rotationReceipt, err = s.encodePlaybackContinuationRotationReceipt(sessionID, playbackContinuationRotationReceipt{
			Credential:  PlaybackContinuationCredential{Token: token, ExpiresAt: expires.Format(time.RFC3339Nano), Origin: origin, Generation: generation},
			RequestID:   rotation.RequestID,
			Fingerprint: rotation.Fingerprint,
		})
		if err != nil {
			return PlaybackContinuationCredential{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO playback_session_continuation_credentials (
			playback_session_id, token_hash, previous_token_hash, user_id, profile_id, client_instance_id,
			generation, origin, issued_at, expires_at, absolute_expires_at, previous_valid_until,
			last_used_at, last_rotation_request_id, last_rotation_fingerprint, last_rotation_receipt, revoked_at
		) VALUES (?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, '')
		ON CONFLICT(playback_session_id) DO UPDATE SET
			previous_token_hash = CASE WHEN playback_session_continuation_credentials.revoked_at = '' THEN playback_session_continuation_credentials.token_hash ELSE '' END,
			previous_valid_until = CASE WHEN playback_session_continuation_credentials.revoked_at = '' THEN ? ELSE '' END,
			token_hash = excluded.token_hash,
			user_id = excluded.user_id, profile_id = excluded.profile_id, client_instance_id = excluded.client_instance_id,
			generation = excluded.generation, origin = excluded.origin, issued_at = excluded.issued_at,
			expires_at = excluded.expires_at, absolute_expires_at = excluded.absolute_expires_at,
			last_used_at = excluded.last_used_at, last_rotation_request_id = excluded.last_rotation_request_id,
			last_rotation_fingerprint = excluded.last_rotation_fingerprint, last_rotation_receipt = excluded.last_rotation_receipt,
			revoked_at = ''`,
		sessionID, hashToken(token), accountIDForUser(user), viewerProfileID(user), clientInstanceID,
		generation, origin, now.Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano), absolute.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano), rotationID, rotationFingerprint, rotationReceipt, previousUntil.Format(time.RFC3339Nano)); err != nil {
		return PlaybackContinuationCredential{}, err
	}
	return PlaybackContinuationCredential{Token: token, ExpiresAt: expires.Format(time.RFC3339Nano), Origin: origin, Generation: generation}, nil
}

func (s *Server) rotatePlaybackContinuationCredential(r *http.Request, user User, sessionID, requestID, fingerprint string) (PlaybackContinuationCredential, error) {
	origin := playbackContinuationOrigin(r)
	if origin == "" {
		return PlaybackContinuationCredential{}, errPlaybackContinuationDenied
	}
	now := time.Now().UTC()
	var credential PlaybackContinuationCredential
	err := s.withUserTxTaggedForViewer(r.Context(), accountIDForUser(user), viewerProfileID(user), []string{"playback"}, func(tx *sql.Tx) error {
		var err error
		credential, err = s.issuePlaybackContinuationCredentialTx(r.Context(), tx, user, sessionID, 0, origin, now, &playbackContinuationRotationRequest{RequestID: requestID, Fingerprint: fingerprint})
		return err
	})
	return credential, err
}

func max64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func playbackContinuationTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "PorticoPlayback ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "PorticoPlayback "))
}

func (s *Server) userForPlaybackContinuation(r *http.Request, sessionID string) (User, int64, error) {
	token := playbackContinuationTokenFromRequest(r)
	if !strings.HasPrefix(token, "ptc_pb_") || strings.TrimSpace(sessionID) == "" {
		return User{}, 0, errPlaybackContinuationDenied
	}
	now := time.Now().UTC()
	origin := playbackContinuationOrigin(r)
	var userID, profileID, storedOrigin, revoked, endedAt, state string
	var credentialGeneration, sessionGeneration int64
	err := s.queryUserRow(r.Context(), `
		SELECT c.user_id, c.profile_id, c.origin, c.revoked_at,
			ps.ended_at, ps.state, c.generation, ps.progress_generation
		FROM playback_session_continuation_credentials c
		JOIN playback_sessions ps ON ps.id = c.playback_session_id
		JOIN users u ON u.id = c.user_id
		JOIN profiles p ON p.id = c.profile_id AND p.account_id = c.user_id
		WHERE c.playback_session_id = ? AND COALESCE(u.disabled_at, '') = ''
			AND COALESCE(p.disabled_at, '') = ''
			AND c.client_instance_id = ps.client_instance_id
			AND c.generation = ps.progress_generation
			AND (c.token_hash = ? OR c.previous_token_hash = ?)
			AND (c.token_hash = ? AND c.expires_at > ? OR c.previous_token_hash = ? AND c.previous_valid_until > ?)
			AND c.origin = ? AND c.revoked_at = ''`,
		sessionID, hashToken(token), hashToken(token), hashToken(token), now.Format(time.RFC3339Nano), hashToken(token), now.Format(time.RFC3339Nano), origin).Scan(&userID, &profileID, &storedOrigin, &revoked, &endedAt, &state, &credentialGeneration, &sessionGeneration)
	if err != nil {
		return User{}, 0, errPlaybackContinuationDenied
	}
	if credentialGeneration <= 0 || credentialGeneration != sessionGeneration || strings.TrimSpace(revoked) != "" || strings.TrimSpace(endedAt) != "" || state == "stopped" || strings.TrimSpace(storedOrigin) != origin {
		return User{}, 0, errPlaybackContinuationDenied
	}
	user, err := s.getUser(userID)
	if err != nil {
		return User{}, 0, errPlaybackContinuationDenied
	}
	user.AccountID = userID
	user.ProfileID = profileID
	principal, err := s.resolveRequestPrincipalContext(r.Context(), userID, profileID)
	if err != nil {
		return User{}, 0, errPlaybackContinuationDenied
	}
	applyRequestPrincipal(&user, principal)
	user = s.hydratePlaybackVisibilityUserContext(r.Context(), user)
	if !user.Permissions["playMedia"] {
		return User{}, 0, errPlaybackContinuationDenied
	}
	_, _ = s.execUserWrite(r.Context(), `UPDATE playback_session_continuation_credentials SET last_used_at = ? WHERE playback_session_id = ? AND (token_hash = ? OR previous_token_hash = ?)`, now.Format(time.RFC3339Nano), sessionID, hashToken(token), hashToken(token))
	return user, credentialGeneration, nil
}

func (s *Server) extendPlaybackContinuation(r *http.Request, sessionID string) string {
	token := playbackContinuationTokenFromRequest(r)
	if token == "" {
		return ""
	}
	now := time.Now().UTC()
	var currentExpiry, absolute string
	if err := s.queryUserRow(r.Context(), `SELECT expires_at, absolute_expires_at FROM playback_session_continuation_credentials WHERE playback_session_id = ? AND (token_hash = ? OR previous_token_hash = ?) AND revoked_at = ''`, sessionID, hashToken(token), hashToken(token)).Scan(&currentExpiry, &absolute); err != nil {
		return ""
	}
	current, _ := time.Parse(time.RFC3339Nano, currentExpiry)
	capAt, _ := time.Parse(time.RFC3339Nano, absolute)
	if current.After(now.Add(playbackContinuationRenewBefore)) {
		return currentExpiry
	}
	expires := now.Add(playbackContinuationTTL)
	if capAt.Before(expires) {
		expires = capAt
	}
	if !expires.After(now) {
		return ""
	}
	result, err := s.execUserWrite(r.Context(), `
		UPDATE playback_session_continuation_credentials
		SET expires_at = ?
		WHERE playback_session_id = ? AND revoked_at = ''
			AND (token_hash = ? OR previous_token_hash = ?)
			AND expires_at = ? AND absolute_expires_at = ?`,
		expires.Format(time.RFC3339Nano), sessionID, hashToken(token), hashToken(token), currentExpiry, absolute)
	if err != nil || result == nil {
		return ""
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		return ""
	}
	return expires.Format(time.RFC3339Nano)
}

func (s *Server) revokePlaybackContinuation(sessionID string) {
	_, _ = s.execUserWrite(context.Background(), `UPDATE playback_session_continuation_credentials SET revoked_at = ?, previous_valid_until = '' WHERE playback_session_id = ? AND revoked_at = ''`, time.Now().UTC().Format(time.RFC3339Nano), sessionID)
}

func (s *Server) handlePlaybackContinuationRoute(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/playback-sessions/"), "/")
	sessionID = strings.TrimSuffix(sessionID, "/continuation")
	user, credentialGeneration, err := s.userForPlaybackContinuation(r, sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "playback_continuation_denied", "A valid PorticoPlayback credential is required.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		var state PlaybackContinuationState
		var highest, queueRevision, playbackRevision, generation int64
		if err := s.queryUserRow(r.Context(), `SELECT state, position_seconds, last_event_sequence, queue_revision, renegotiation_revision, progress_generation FROM playback_sessions WHERE id = ? AND profile_id = ? AND ended_at = ''`, sessionID, viewerProfileID(user)).Scan(&state.State, &state.PositionSeconds, &highest, &queueRevision, &playbackRevision, &generation); err != nil {
			writeError(w, http.StatusNotFound, "playback_session_not_found", "Playback session was not found.")
			return
		}
		state.SessionID, state.HighestEventSequence, state.QueueRevision, state.PlaybackRevision, state.Generation = sessionID, highest, queueRevision, playbackRevision, generation
		state.MediaGrantExpiresAt = s.playbackMediaGrantExpiry(r.Context(), user, sessionID)
		writeJSON(w, http.StatusOK, state)
	case http.MethodPatch:
		var req PlaybackProgressEvent
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.EventSequence <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_event_sequence", "eventSequence must be a positive integer.")
			return
		}
		if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(req.RecordedAt)); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_recorded_at", "recordedAt must be an RFC 3339 timestamp.")
			return
		}
		if req.Completed != nil && *req.Completed {
			writeError(w, http.StatusBadRequest, "terminal_request_required", "Complete playback with the atomic DELETE terminal request.")
			return
		}
		if req.Generation != 0 && req.Generation != credentialGeneration {
			writeError(w, http.StatusConflict, "playback_generation_stale", "Playback progress authority changed. Reload the active playback session.")
			return
		}
		req.Generation = credentialGeneration
		ack, err := s.touchPlaybackSession(user, sessionID, req)
		if err != nil {
			if errors.Is(err, errPlaybackGenerationStale) {
				writeError(w, http.StatusConflict, "playback_generation_stale", "Playback progress authority changed. Reload the active playback session.")
				return
			}
			writeError(w, http.StatusUnauthorized, "playback_continuation_denied", "Playback progress could not be accepted.")
			return
		}
		if ack.Accepted {
			_ = s.renewMediaGrantsForSession(r.Context(), user, sessionID)
			ack.MediaGrantExpiresAt = s.playbackMediaGrantExpiry(r.Context(), user, sessionID)
			if s.extendPlaybackContinuation(r, sessionID) != "" {
				ack.GrantExtended = true
				ack.GrantSemantics = "extension"
			}
		}
		writeJSON(w, http.StatusOK, ack)
	case http.MethodPost:
		var req PlaybackContinuationRotateRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		req.RequestID = strings.TrimSpace(req.RequestID)
		if req.RequestID == "" || len(req.RequestID) > 128 {
			writeError(w, http.StatusBadRequest, "invalid_continuation_rotation", "requestId is required.")
			return
		}
		body, _ := json.Marshal(req)
		fingerprint := hashToken(string(body))
		credential, err := s.rotatePlaybackContinuationCredential(r, user, sessionID, req.RequestID, fingerprint)
		if errors.Is(err, errPlaybackContinuationRotationConflict) {
			writeError(w, http.StatusConflict, "continuation_rotation_conflict", "requestId was already used for a different continuation rotation.")
			return
		}
		if errors.Is(err, errPlaybackContinuationRotationReceipt) {
			writeError(w, http.StatusConflict, "playback_continuation_denied", "Playback continuation could not be recovered safely.")
			return
		}
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errPlaybackContinuationDenied) || errors.Is(err, sql.ErrNoRows) {
				status = http.StatusConflict
			}
			writeError(w, status, "playback_continuation_denied", "Playback continuation could not be rotated.")
			return
		}
		if credential.Token == "" {
			writeError(w, http.StatusInternalServerError, "playback_continuation_failed", "Playback continuation could not be recorded safely.")
			return
		}
		writeJSON(w, http.StatusOK, credential)
	case http.MethodDelete:
		var req PlaybackSessionStopRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		req.Disposition = strings.ToLower(strings.TrimSpace(req.Disposition))
		if req.Disposition != "stopped" && req.Disposition != "completed" {
			writeError(w, http.StatusBadRequest, "invalid_playback_disposition", "disposition must be stopped or completed.")
			return
		}
		if req.Generation <= 0 || req.Generation != credentialGeneration || req.EventSequence <= 0 {
			writeError(w, http.StatusConflict, "playback_generation_stale", "Playback progress authority changed. Reload the active playback session.")
			return
		}
		if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(req.RecordedAt)); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_recorded_at", "recordedAt must be an RFC 3339 timestamp.")
			return
		}
		if math.IsNaN(req.PositionSeconds) || math.IsInf(req.PositionSeconds, 0) || req.PositionSeconds < 0 || math.IsNaN(req.DurationSeconds) || math.IsInf(req.DurationSeconds, 0) || req.DurationSeconds < 0 || (req.Disposition == "completed" && req.DurationSeconds <= 0) {
			writeError(w, http.StatusBadRequest, "invalid_playback_terminal_position", "positionSeconds and durationSeconds must be finite and non-negative; completed playback requires a positive durationSeconds.")
			return
		}
		if err := s.endPlaybackSessionAtomically(user, sessionID, req); err != nil {
			if errors.Is(err, errPlaybackGenerationStale) || errors.Is(err, errPlaybackEventSequenceStale) {
				writeError(w, http.StatusConflict, "playback_terminal_stale", "Playback terminal authority is stale.")
				return
			}
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "playback_session_not_found", "Playback session was not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "playback_session_failed", "Unable to end playback session.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET, PATCH, POST, or DELETE for this endpoint.")
	}
}
