package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"
)

var errPlaybackTerminalReceiptConflict = errors.New("playback terminal receipt conflicts with the request")
var errPlaybackTerminalAuthorizationChanged = errors.New("playback terminal receipt authorization changed")

var playbackAuthorityRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

func validPlaybackAuthorityRequestID(requestID string) bool {
	return playbackAuthorityRequestIDPattern.MatchString(requestID)
}

type playbackTerminalReceipt struct {
	RequestID             string
	Fingerprint           string
	ResponseJSON          string
	CredentialTokenHash   string
	CredentialOrigin      string
	AuthorizationRevision string
}

func validatePlaybackTerminalEvent(event PlaybackTerminalEvent) *playbackStartHTTPError {
	if event.Disposition != "stopped" && event.Disposition != "completed" {
		return &playbackStartHTTPError{status: 400, code: "invalid_playback_disposition", message: "disposition must be stopped or completed."}
	}
	if event.Generation <= 0 || event.EventSequence <= 0 {
		return &playbackStartHTTPError{status: 400, code: "invalid_playback_terminal_authority", message: "generation and eventSequence must be positive integers."}
	}
	if _, err := time.Parse(time.RFC3339Nano, event.RecordedAt); err != nil {
		return &playbackStartHTTPError{status: 400, code: "invalid_recorded_at", message: "recordedAt must be an RFC 3339 timestamp."}
	}
	if math.IsNaN(event.PositionSeconds) || math.IsInf(event.PositionSeconds, 0) || event.PositionSeconds < 0 || math.IsNaN(event.DurationSeconds) || math.IsInf(event.DurationSeconds, 0) || event.DurationSeconds < 0 || (event.Disposition == "completed" && event.DurationSeconds <= 0) {
		return &playbackStartHTTPError{status: 400, code: "invalid_playback_terminal_position", message: "positionSeconds and durationSeconds must be finite and non-negative; completed playback requires a positive durationSeconds."}
	}
	if event.DurationSeconds > 0 && event.PositionSeconds > event.DurationSeconds {
		return &playbackStartHTTPError{status: 400, code: "invalid_playback_terminal_position", message: "positionSeconds cannot exceed durationSeconds."}
	}
	if event.Disposition == "completed" && event.PositionSeconds != event.DurationSeconds {
		return &playbackStartHTTPError{status: 400, code: "invalid_playback_terminal_position", message: "completed playback requires positionSeconds to equal durationSeconds."}
	}
	return nil
}

func normalizePlaybackSessionStopRequest(req PlaybackSessionStopRequest) (PlaybackSessionStopRequest, *playbackStartHTTPError) {
	if !validPlaybackAuthorityRequestID(req.RequestID) {
		return req, &playbackStartHTTPError{status: 400, code: "playback_terminal_request_id_invalid", message: "requestId must be 8 to 128 letters, numbers, periods, underscores, colons, or hyphens."}
	}
	req.requestFingerprint = playbackTerminalFingerprint(req.Terminal)
	if err := validatePlaybackTerminalEvent(req.Terminal); err != nil {
		return req, err
	}
	return req, nil
}

func playbackTerminalFingerprint(event PlaybackTerminalEvent) string {
	encoded, _ := json.Marshal(event)
	return hashToken(string(encoded))
}

func playbackSessionStopFingerprint(req PlaybackSessionStopRequest) string {
	if req.requestFingerprint != "" {
		return req.requestFingerprint
	}
	return playbackTerminalFingerprint(req.Terminal)
}

func playbackTerminalAcknowledgement(sessionID string, req PlaybackSessionStopRequest, duplicate bool) PlaybackSessionTerminalAcknowledgement {
	return PlaybackSessionTerminalAcknowledgement{
		RequestID: req.RequestID,
		Accepted:  true,
		Duplicate: duplicate,
		SessionID: strings.TrimSpace(sessionID),
		Terminal:  req.Terminal,
	}
}

func (s *Server) terminatePlaybackWithReceipt(ctx context.Context, user User, sessionID string, req PlaybackSessionStopRequest, credentialToken, credentialOrigin string) (PlaybackSessionTerminalAcknowledgement, error) {
	authorizationRevision, authorizationErr := s.authorizationRevisionForUserContextStrict(ctx, user)
	if authorizationErr != nil || strings.TrimSpace(authorizationRevision) == "" {
		return PlaybackSessionTerminalAcknowledgement{}, errPlaybackTerminalAuthorizationChanged
	}
	ack := playbackTerminalAcknowledgement(sessionID, req, false)
	responseJSON, err := json.Marshal(ack)
	if err != nil {
		return PlaybackSessionTerminalAcknowledgement{}, err
	}
	receipt := &playbackTerminalReceipt{
		RequestID: req.RequestID, Fingerprint: playbackSessionStopFingerprint(req), ResponseJSON: string(responseJSON),
		CredentialOrigin:      strings.TrimSpace(credentialOrigin),
		AuthorizationRevision: authorizationRevision,
	}
	if token := strings.TrimSpace(credentialToken); token != "" {
		receipt.CredentialTokenHash = hashToken(token)
	}
	_, err = s.playbackLifecycle().Terminate(ctx, playbackTerminationRequest{
		SessionID: sessionID, UserID: accountIDForUser(user), ProfileID: viewerProfileID(user),
		Cause: playbackTerminationExplicit, RequireActive: true, Event: &req.Terminal, TerminalReceipt: receipt,
	})
	if err != nil {
		return PlaybackSessionTerminalAcknowledgement{}, err
	}
	return ack, nil
}

func decodePlaybackTerminalAcknowledgement(responseJSON string) (PlaybackSessionTerminalAcknowledgement, error) {
	var ack PlaybackSessionTerminalAcknowledgement
	if err := json.Unmarshal([]byte(responseJSON), &ack); err != nil {
		return PlaybackSessionTerminalAcknowledgement{}, err
	}
	if !ack.Accepted || strings.TrimSpace(ack.RequestID) == "" || strings.TrimSpace(ack.SessionID) == "" {
		return PlaybackSessionTerminalAcknowledgement{}, errors.New("playback terminal receipt is invalid")
	}
	ack.Duplicate = true
	return ack, nil
}

func terminalReceiptMatches(req PlaybackSessionStopRequest, storedRequestID, storedFingerprint string) bool {
	return req.RequestID == storedRequestID && playbackSessionStopFingerprint(req) == storedFingerprint
}

func (s *Server) playbackTerminalReceiptForUser(ctx context.Context, user User, sessionID string, req PlaybackSessionStopRequest) (PlaybackSessionTerminalAcknowledgement, error) {
	var storedRequestID, storedFingerprint, responseJSON, authorizationRevision string
	err := s.queryUserRow(ctx, `
		SELECT request_id, request_fingerprint, response_json, authorization_revision
		FROM playback_session_terminal_receipts
		WHERE playback_session_id = ? AND user_id = ? AND profile_id = ?`,
		sessionID, accountIDForUser(user), viewerProfileID(user)).Scan(&storedRequestID, &storedFingerprint, &responseJSON, &authorizationRevision)
	if err != nil {
		return PlaybackSessionTerminalAcknowledgement{}, err
	}
	if !terminalReceiptMatches(req, storedRequestID, storedFingerprint) {
		return PlaybackSessionTerminalAcknowledgement{}, errPlaybackTerminalReceiptConflict
	}
	currentRevision, revisionErr := s.authorizationRevisionForUserContextStrict(ctx, user)
	if revisionErr != nil || currentRevision != authorizationRevision {
		return PlaybackSessionTerminalAcknowledgement{}, errPlaybackTerminalAuthorizationChanged
	}
	return decodePlaybackTerminalAcknowledgement(responseJSON)
}

func (s *Server) playbackTerminalReceiptForCredential(ctx context.Context, sessionID, token, origin string, req PlaybackSessionStopRequest) (PlaybackSessionTerminalAcknowledgement, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "ptc_pb_") {
		return PlaybackSessionTerminalAcknowledgement{}, errPlaybackContinuationDenied
	}
	var storedRequestID, storedFingerprint, responseJSON string
	err := s.queryUserRow(ctx, `
		SELECT request_id, request_fingerprint, response_json
		FROM playback_session_terminal_receipts
		WHERE playback_session_id = ? AND credential_token_hash = ? AND credential_origin = ?`,
		sessionID, hashToken(token), strings.TrimSpace(origin)).Scan(&storedRequestID, &storedFingerprint, &responseJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return PlaybackSessionTerminalAcknowledgement{}, errPlaybackContinuationDenied
	}
	if err != nil {
		return PlaybackSessionTerminalAcknowledgement{}, err
	}
	if !terminalReceiptMatches(req, storedRequestID, storedFingerprint) {
		return PlaybackSessionTerminalAcknowledgement{}, errPlaybackTerminalReceiptConflict
	}
	return decodePlaybackTerminalAcknowledgement(responseJSON)
}
