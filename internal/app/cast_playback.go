package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	castProtocolVersion       = "v1"
	castEnvelopeLabel         = "portico-cast-bootstrap-v1"
	castBootstrapTTL          = 60 * time.Second
	castReceiverSessionTTL    = 24 * time.Hour
	castReceiverAuthorization = "PorticoReceiver "
)

var (
	errCastBootstrapInvalid       = errors.New("cast bootstrap is invalid, expired, redeemed, or out of scope")
	errCastReceiverSessionInvalid = errors.New("cast receiver session is invalid, expired, stopped, or out of scope")
	errCastGenerationStale        = errors.New("cast receiver session generation is stale")
)

var castReceiverOperations = map[string]bool{
	"load": true, "control": true, "stop": true, "progress": true, "renew": true, "reconnect": true, "advance": true, "segment-skip": true,
}

type CastBootstrapRequest struct {
	VersionID               string                `json:"versionId,omitempty"`
	ClientInstanceID        string                `json:"clientInstanceId"`
	ClientProfile           PlaybackClientProfile `json:"clientProfile"`
	Intent                  PlaybackIntent        `json:"intent,omitempty"`
	SkipPreroll             bool                  `json:"skipPreroll,omitempty"`
	BurnInSubtitleID        string                `json:"burnInSubtitleId,omitempty"`
	SubtitleStreamID        string                `json:"subtitleStreamId,omitempty"`
	AudioStreamID           string                `json:"audioStreamId,omitempty"`
	StartSeconds            int                   `json:"startSeconds,omitempty"`
	QueueMediaIDs           []string              `json:"queueMediaIds,omitempty"`
	RepeatMode              string                `json:"repeatMode,omitempty"`
	SourceContext           PlaybackSourceContext `json:"sourceContext,omitempty"`
	SourceKind              string                `json:"sourceKind,omitempty"`
	SourceID                string                `json:"sourceId,omitempty"`
	SourcePlaybackSessionID string                `json:"sourcePlaybackSessionId,omitempty"`
	ReceiverID              string                `json:"receiverId"`
	ReceiverOrigin          string                `json:"receiverOrigin"`
	ReceiverPublicKey       string                `json:"receiverPublicKey"`
	ReceiverChallenge       string                `json:"receiverChallenge"`
	Capabilities            []string              `json:"capabilities,omitempty"`
}

type CastBootstrapResponse struct {
	Version           string   `json:"version"`
	BootstrapEnvelope string   `json:"bootstrapEnvelope"`
	BootstrapID       string   `json:"bootstrapId"`
	ReceiverID        string   `json:"receiverId"`
	ReceiverOrigin    string   `json:"receiverOrigin"`
	ServerOrigin      string   `json:"serverOrigin"`
	Generation        int64    `json:"generation"`
	ExpiresAt         string   `json:"expiresAt"`
	Capabilities      []string `json:"capabilities"`
}

type CastRedeemRequest struct {
	BootstrapID       string `json:"bootstrapId"`
	BootstrapSecret   string `json:"bootstrapSecret"`
	ReceiverID        string `json:"receiverId"`
	ReceiverChallenge string `json:"receiverChallenge"`
}

type CastReceiverSessionResponse struct {
	Version              string                 `json:"version"`
	ReceiverSessionToken string                 `json:"receiverSessionToken"`
	ReceiverSessionID    string                 `json:"receiverSessionId"`
	PlaybackSessionID    string                 `json:"playbackSessionId"`
	ReceiverID           string                 `json:"receiverId"`
	ServerOrigin         string                 `json:"serverOrigin"`
	Generation           int64                  `json:"generation"`
	Capabilities         []string               `json:"capabilities"`
	GrantSemantics       string                 `json:"grantSemantics"`
	MediaGrantExpiresAt  string                 `json:"mediaGrantExpiresAt"`
	Playback             PlaybackResponse       `json:"playback"`
	Automation           CastPlaybackAutomation `json:"automation"`
}

type CastPlaybackAutomation struct {
	AutoplayNext           bool   `json:"autoplayNext"`
	UpNextCountdownSeconds int    `json:"upNextCountdownSeconds"`
	PassoutProtection      bool   `json:"passoutProtection"`
	PassoutAfterEpisodes   int    `json:"passoutAfterEpisodes"`
	IntroSkip              string `json:"introSkip"`
	CreditsSkip            string `json:"creditsSkip"`
}

type CastAdvanceRequest struct {
	Generation int64  `json:"generation"`
	AdvanceID  string `json:"advanceId"`
	Automatic  bool   `json:"automatic,omitempty"`
}

type CastAdvanceResponse struct {
	Version           string                 `json:"version"`
	Status            string                 `json:"status"`
	Generation        int64                  `json:"generation"`
	AutomaticAdvances int                    `json:"automaticAdvances"`
	Automation        CastPlaybackAutomation `json:"automation"`
	Playback          *PlaybackResponse      `json:"playback,omitempty"`
}

type CastSegmentSkipRequest struct {
	Generation int64  `json:"generation"`
	SegmentID  string `json:"segmentId"`
}

type CastSegmentSkipResponse struct {
	Version         string `json:"version"`
	Generation      int64  `json:"generation"`
	Skipped         bool   `json:"skipped"`
	PositionSeconds int    `json:"positionSeconds,omitempty"`
}

type CastReceiverSessionState struct {
	Version           string                 `json:"version"`
	ReceiverSessionID string                 `json:"receiverSessionId"`
	PlaybackSessionID string                 `json:"playbackSessionId"`
	ReceiverID        string                 `json:"receiverId"`
	ServerOrigin      string                 `json:"serverOrigin"`
	Generation        int64                  `json:"generation"`
	Status            string                 `json:"status"`
	PlaybackState     string                 `json:"playbackState"`
	PositionSeconds   int                    `json:"positionSeconds"`
	MediaID           string                 `json:"mediaId"`
	Capabilities      []string               `json:"capabilities"`
	LastSeenAt        string                 `json:"lastSeenAt"`
	ExpiresAt         string                 `json:"expiresAt"`
	Automation        CastPlaybackAutomation `json:"automation"`
	AutomaticAdvances int                    `json:"automaticAdvances"`
	RepeatMode        string                 `json:"repeatMode"`
	Queue             []MediaItem            `json:"queue"`
}

type CastReconnectRequest struct {
	ReceiverSessionID string `json:"receiverSessionId,omitempty"`
	ClientInstanceID  string `json:"clientInstanceId,omitempty"`
	Generation        int64  `json:"generation,omitempty"`
}

type CastControlRequest struct {
	Generation      int64   `json:"generation"`
	Operation       string  `json:"operation"`
	PositionSeconds float64 `json:"positionSeconds,omitempty"`
	CommandID       string  `json:"commandId,omitempty"`
}

type CastProgressRequest struct {
	Generation int64 `json:"generation"`
	PlaybackProgressEvent
}

type CastRenewRequest struct {
	Generation int64  `json:"generation"`
	Mode       string `json:"mode,omitempty"`
}

type CastRenewResponse struct {
	Version             string                   `json:"version"`
	GrantSemantics      string                   `json:"grantSemantics"`
	Generation          int64                    `json:"generation"`
	MediaGrant          *MediaGrant              `json:"mediaGrant,omitempty"`
	MediaGrantExpiresAt string                   `json:"mediaGrantExpiresAt"`
	ReceiverSession     CastReceiverSessionState `json:"receiverSession"`
}

type castBootstrapRecord struct {
	ID                      string
	UserID                  string
	ProfileID               string
	ReceiverID              string
	ReceiverOrigin          string
	ReceiverPublicKey       string
	ReceiverChallenge       string
	ServerOrigin            string
	PlaybackSessionID       string
	ClientInstanceID        string
	Generation              int64
	CapabilitiesJSON        string
	AutomationJSON          string
	PlaybackEnvelope        string
	SourcePlaybackSessionID string
	ExpiresAt               string
	RedeemedAt              string
}

type castReceiverRecord struct {
	ID                      string
	UserID                  string
	ProfileID               string
	ReceiverID              string
	ReceiverOrigin          string
	ServerOrigin            string
	PlaybackSessionID       string
	ClientInstanceID        string
	Generation              int64
	CapabilitiesJSON        string
	Status                  string
	ExpiresAt               string
	LastSeenAt              string
	LastCommandID           string
	LastCommandJSON         string
	AutomationJSON          string
	AutomaticAdvances       int
	LastAdvanceID           string
	LastAdvanceJSON         string
	SourcePlaybackSessionID string
}

type castSessionAuth struct {
	record      castReceiverRecord
	user        User
	viaReceiver bool
}

type castBootstrapEnvelope struct {
	Version           string `json:"version"`
	BootstrapID       string `json:"bootstrapId"`
	ReceiverID        string `json:"receiverId"`
	ReceiverOrigin    string `json:"receiverOrigin"`
	ReceiverChallenge string `json:"receiverChallenge"`
	ServerOrigin      string `json:"serverOrigin"`
	ServerPublicKey   string `json:"serverPublicKey"`
	Nonce             string `json:"nonce"`
	Ciphertext        string `json:"ciphertext"`
}

type castBootstrapSecretPayload struct {
	Version           string `json:"version"`
	BootstrapID       string `json:"bootstrapId"`
	ReceiverID        string `json:"receiverId"`
	ReceiverOrigin    string `json:"receiverOrigin"`
	ReceiverChallenge string `json:"receiverChallenge"`
	ServerOrigin      string `json:"serverOrigin"`
	Secret            string `json:"secret"`
	ExpiresAt         string `json:"expiresAt"`
}

func (s *Server) handleCastBootstrap(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if !user.Permissions["playMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to play this media.")
		return
	}
	var req CastBootstrapRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.ReceiverID = strings.TrimSpace(req.ReceiverID)
	req.ReceiverOrigin = strings.TrimRight(strings.TrimSpace(req.ReceiverOrigin), "/")
	req.ClientInstanceID = normalizePlaybackClientInstanceID(req.ClientInstanceID)
	req.SourceKind = strings.ToLower(strings.TrimSpace(req.SourceKind))
	req.SourceID = strings.TrimSpace(req.SourceID)
	if (req.SourceKind != "media" && req.SourceKind != "live" && req.SourceKind != "dvr" && req.SourceKind != "library-channel") || req.SourceID == "" {
		writeError(w, http.StatusBadRequest, "invalid_cast_source", "Cast playback requires a supported source kind and source ID.")
		return
	}
	if strings.TrimSpace(req.SourcePlaybackSessionID) != "" {
		var sourceClientInstanceID, sourceMediaID string
		if err := s.queryUserRow(r.Context(), `SELECT client_instance_id, media_id FROM playback_sessions WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`, strings.TrimSpace(req.SourcePlaybackSessionID), accountIDForUser(user), viewerProfileID(user)).Scan(&sourceClientInstanceID, &sourceMediaID); err != nil {
			writeError(w, http.StatusConflict, "cast_source_session_invalid", "The Apple playback session is no longer eligible for Cast handoff.")
			return
		}
		expectedMediaID, expectedErr := s.castTargetSessionMediaID(r.Context(), user, req.SourceKind, req.SourceID)
		if expectedErr != nil || sourceMediaID != expectedMediaID {
			writeError(w, http.StatusConflict, "cast_source_session_invalid", "The Apple playback session does not match the selected Cast target.")
			return
		}
		sourceClientInstanceID = normalizePlaybackClientInstanceID(sourceClientInstanceID)
		if req.ClientInstanceID == "" {
			req.ClientInstanceID = sourceClientInstanceID
		} else if sourceClientInstanceID != "" && req.ClientInstanceID != sourceClientInstanceID {
			writeError(w, http.StatusConflict, "cast_source_session_invalid", "The Apple playback session belongs to a different client instance.")
			return
		}
	}
	if req.ClientInstanceID == "" || req.ReceiverID == "" || len(req.ReceiverID) > 160 || !s.castReceiverOriginAllowed(req.ReceiverOrigin) || !validCastChallenge(req.ReceiverChallenge) {
		writeError(w, http.StatusBadRequest, "invalid_cast_bootstrap", "The Cast bootstrap scope is invalid.")
		return
	}
	receiverPublicKey, err := decodeCastPublicKey(req.ReceiverPublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cast_receiver_key", "The Cast receiver key is invalid.")
		return
	}
	req.ReceiverPublicKey = base64.RawURLEncoding.EncodeToString(receiverPublicKey.Bytes())
	capabilities, ok := normalizeCastCapabilities(req.Capabilities)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_cast_capabilities", "The Cast receiver capabilities are invalid.")
		return
	}
	var activeSessionID string
	var activeGeneration int64
	_ = s.queryUserRow(r.Context(), `
		SELECT id, generation
		FROM cast_receiver_sessions
		WHERE user_id = ? AND profile_id = ? AND client_instance_id = ? AND status = 'active' AND expires_at > ?
		ORDER BY generation DESC LIMIT 1`, accountIDForUser(user), viewerProfileID(user), req.ClientInstanceID, time.Now().UTC().Format(time.RFC3339)).Scan(&activeSessionID, &activeGeneration)
	if activeSessionID != "" {
		// A lost receiver token must not strand this installation for the full
		// receiver TTL. The authenticated sender supersedes its own receiver
		// session, while the source it is handing off remains alive until the new
		// receiver reports playing.
		if err := s.supersedeCastReceiverSession(r.Context(), user, activeSessionID, activeGeneration, req.ClientInstanceID, strings.TrimSpace(req.SourcePlaybackSessionID)); err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusConflict, "cast_session_supersession_failed", "The previous Cast session could not be safely replaced. Try again.")
			return
		}
	}
	// Cast always gets a receiver-profile playback session. The sender session
	// remains authoritative until the receiver redeems, loads and reports
	// playing; commitCastReceiverHandoff retires it only at that point.
	playback, startErr := s.startCastTargetPlayback(r, user, req)
	if startErr != nil {
		if startErr.retryAfter != "" {
			w.Header().Set("Retry-After", startErr.retryAfter)
		}
		writeError(w, startErr.status, startErr.code, startErr.message)
		return
	}
	serverOrigin := s.castPublicServerOrigin(r)
	if serverOrigin == "" {
		_ = s.endPlaybackSession(user, playback.SessionID)
		writeError(w, http.StatusBadRequest, "cast_server_origin_unavailable", "The selected server does not have a usable public origin.")
		return
	}
	if err := absolutizeCastPlaybackURLs(&playback, serverOrigin); err != nil {
		_ = s.endPlaybackSession(user, playback.SessionID)
		writeError(w, http.StatusConflict, "cast_playback_source_invalid", "The selected playback source is not reachable through the verified public server origin.")
		return
	}
	playbackEnvelope, err := s.sealCastPlayback(playback)
	if err != nil {
		_ = s.endPlaybackSession(user, playback.SessionID)
		writeError(w, http.StatusInternalServerError, "cast_bootstrap_failed", "Unable to prepare Cast playback.")
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(castBootstrapTTL).Format(time.RFC3339)
	bootstrapID := randomID("cast_bootstrap")
	secret := "ptc_cb_" + randomToken()
	bootstrapEnvelope, err := makeCastBootstrapEnvelope(bootstrapID, req.ReceiverID, req.ReceiverOrigin, serverOrigin, req.ReceiverChallenge, receiverPublicKey, secret, expiresAt)
	if err != nil {
		_ = s.endPlaybackSession(user, playback.SessionID)
		writeError(w, http.StatusInternalServerError, "cast_bootstrap_failed", "Unable to prepare Cast playback.")
		return
	}
	automation := s.castPlaybackAutomation(r.Context(), user, req.ClientInstanceID)
	handoffSourceID := strings.TrimSpace(req.SourcePlaybackSessionID)
	if handoffSourceID == playback.SessionID {
		handoffSourceID = ""
	}
	err = s.withPlaybackTxTagged(r.Context(), []string{"playback"}, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(r.Context(), `UPDATE playback_sessions SET progress_authority = 'receiver', progress_generation = ? WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`, int64(1), playback.SessionID, accountIDForUser(user), viewerProfileID(user)); err != nil {
			return err
		}
		_, err := tx.ExecContext(r.Context(), `
			INSERT INTO cast_bootstraps (id, token_hash, user_id, profile_id, receiver_id, receiver_origin, receiver_public_key, receiver_challenge, server_origin, playback_session_id, source_playback_session_id, client_instance_id, generation, capabilities_json, automation_json, playback_envelope, expires_at, redeemed_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
			bootstrapID, hashToken(secret), accountIDForUser(user), viewerProfileID(user), req.ReceiverID, req.ReceiverOrigin,
			req.ReceiverPublicKey, req.ReceiverChallenge, serverOrigin, playback.SessionID, handoffSourceID, req.ClientInstanceID, 1, stringJSON(capabilities), stringJSON(automation), playbackEnvelope, expiresAt, now.Format(time.RFC3339))
		return err
	})
	if err != nil {
		_ = s.endPlaybackSession(user, playback.SessionID)
		writeError(w, http.StatusInternalServerError, "cast_bootstrap_failed", "Unable to prepare Cast playback.")
		return
	}
	writeJSON(w, http.StatusCreated, CastBootstrapResponse{Version: castProtocolVersion, BootstrapEnvelope: bootstrapEnvelope, BootstrapID: bootstrapID, ReceiverID: req.ReceiverID, ReceiverOrigin: req.ReceiverOrigin, ServerOrigin: serverOrigin, Generation: 1, ExpiresAt: expiresAt, Capabilities: capabilities})
}

func (s *Server) castTargetSessionMediaID(_ context.Context, user User, sourceKind, sourceID string) (string, error) {
	switch sourceKind {
	case "media", "live", "library-channel":
		return strings.TrimSpace(sourceID), nil
	case "dvr":
		recording, err := s.getDVRRecordingForUser(user, strings.TrimSpace(sourceID))
		if err != nil || !s.dvrRecordingChannelAllowed(user, recording) {
			return "", sql.ErrNoRows
		}
		if strings.EqualFold(strings.TrimSpace(recording.Status), "running") {
			return strings.TrimSpace(recording.ChannelID), nil
		}
		return dvrRecordingMediaID(recording.ID), nil
	default:
		return "", errors.New("unsupported cast source")
	}
}

func (s *Server) startCastTargetPlayback(r *http.Request, user User, req CastBootstrapRequest) (PlaybackResponse, *playbackStartHTTPError) {
	// Do not pass the sender instance through to the underlying constructor:
	// ordinary replacement-by-instance would stop the sender before the Cast
	// receiver proves it can load. Receiver ownership is fenced separately by
	// the Cast receiver session and generation.
	const receiverPlaybackClientInstanceID = ""
	switch req.SourceKind {
	case "media":
		return s.startPlaybackForRequest(r, user, PlaybackSessionCreateRequest{
			MediaID: req.SourceID, VersionID: req.VersionID, ClientInstanceID: receiverPlaybackClientInstanceID,
			ClientProfile: req.ClientProfile, Intent: req.Intent, SkipPreroll: req.SkipPreroll,
			BurnInSubtitleID: req.BurnInSubtitleID, SubtitleStreamID: req.SubtitleStreamID,
			AudioStreamID: req.AudioStreamID, StartSeconds: req.StartSeconds,
			QueueMediaIDs: req.QueueMediaIDs, RepeatMode: req.RepeatMode, SourceContext: req.SourceContext,
		})
	case "live":
		return s.startLiveTVPlaybackForRequest(r, user, req.SourceID, req.ClientProfile, req.Intent, receiverPlaybackClientInstanceID)
	case "dvr":
		return s.startDVRRecordingPlaybackForRequest(r, user, req.SourceID, DVRPlaybackSessionCreateRequest{
			ClientInstanceID: receiverPlaybackClientInstanceID, ClientProfile: req.ClientProfile, Intent: req.Intent,
			VersionID: req.VersionID, AudioStreamID: req.AudioStreamID, SubtitleStreamID: req.SubtitleStreamID,
			BurnInSubtitleID: req.BurnInSubtitleID, StartSeconds: req.StartSeconds,
		})
	case "library-channel":
		return s.startLibraryChannelPlaybackByID(r, user, req.SourceID, req.ClientProfile, req.Intent, receiverPlaybackClientInstanceID)
	default:
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusBadRequest, code: "invalid_cast_source", message: "Cast playback requires a supported source kind."}
	}
}

// supersedeCastReceiverSession replaces a receiver session only inside the
// authenticated account/profile/installation scope. Its receiver playback is
// retired in the same transaction as the receiver record. protectedSourceID
// is never stopped: it is the sender playback the replacement receiver still
// needs until LOAD commits.
func (s *Server) supersedeCastReceiverSession(ctx context.Context, user User, receiverSessionID string, generation int64, clientInstanceID, protectedSourceID string) error {
	receiverSessionID = strings.TrimSpace(receiverSessionID)
	clientInstanceID = normalizePlaybackClientInstanceID(clientInstanceID)
	protectedSourceID = strings.TrimSpace(protectedSourceID)
	if receiverSessionID == "" || clientInstanceID == "" {
		return sql.ErrNoRows
	}
	var candidatePlaybackID string
	if err := s.queryUserRow(ctx, `
		SELECT playback_session_id FROM cast_receiver_sessions
		WHERE id = ? AND user_id = ? AND profile_id = ? AND client_instance_id = ?
			AND generation = ? AND status = 'active'`,
		receiverSessionID, accountIDForUser(user), viewerProfileID(user), clientInstanceID, generation,
	).Scan(&candidatePlaybackID); err != nil {
		return err
	}
	candidatePlaybackID = strings.TrimSpace(candidatePlaybackID)
	mediaID := ""
	if candidatePlaybackID != "" && candidatePlaybackID != protectedSourceID {
		var err error
		mediaID, err = s.finalizePlaybackSessionProgress(user, candidatePlaybackID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}

	now := time.Now().UTC()
	endedAt := now.Format(time.RFC3339)
	revokedAt := now.Format(time.RFC3339Nano)
	retiredPlaybackID := ""
	err := s.withPlaybackTxTagged(ctx, []string{"playback", "live-tv", "dvr"}, func(tx *sql.Tx) error {
		var currentPlaybackID string
		if err := tx.QueryRowContext(ctx, `
			SELECT playback_session_id FROM cast_receiver_sessions
			WHERE id = ? AND user_id = ? AND profile_id = ? AND client_instance_id = ?
				AND generation = ? AND status = 'active'`,
			receiverSessionID, accountIDForUser(user), viewerProfileID(user), clientInstanceID, generation,
		).Scan(&currentPlaybackID); err != nil {
			return err
		}
		currentPlaybackID = strings.TrimSpace(currentPlaybackID)
		if currentPlaybackID != candidatePlaybackID {
			return errCastGenerationStale
		}
		if currentPlaybackID != "" && currentPlaybackID != protectedSourceID {
			if _, err := tx.ExecContext(ctx, `
				UPDATE playback_sessions SET state = 'stopped', ended_at = ?, last_seen_at = ?
				WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`,
				endedAt, endedAt, currentPlaybackID, accountIDForUser(user), viewerProfileID(user),
			); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE playback_media_grants SET revoked_at = ? WHERE playback_session_id = ? AND revoked_at = ''`, revokedAt, currentPlaybackID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE playback_session_continuation_credentials SET revoked_at = ?, previous_valid_until = '' WHERE playback_session_id = ? AND revoked_at = ''`, revokedAt, currentPlaybackID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM live_tv_tuner_allocations WHERE allocation_kind = 'live_session' AND consumer_id = ?`, currentPlaybackID); err != nil {
				return err
			}
			retiredPlaybackID = currentPlaybackID
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE cast_receiver_sessions
			SET status = 'stopped', stopped_at = ?, last_seen_at = ?, source_playback_session_id = ''
			WHERE id = ? AND user_id = ? AND profile_id = ? AND client_instance_id = ?
				AND generation = ? AND status = 'active' AND playback_session_id = ?`,
			endedAt, endedAt, receiverSessionID, accountIDForUser(user), viewerProfileID(user),
			clientInstanceID, generation, currentPlaybackID,
		)
		if err != nil {
			return err
		}
		if count, rowsErr := result.RowsAffected(); rowsErr != nil {
			return rowsErr
		} else if count != 1 {
			return errCastGenerationStale
		}
		return nil
	})
	if err != nil {
		return err
	}
	if retiredPlaybackID != "" {
		s.forgetMediaGrantsForPlaybackSession(retiredPlaybackID)
		_ = s.completeEndedPlaybackSessionHistoryContext(context.Background(), retiredPlaybackID)
		if mediaID != "" && !s.hasActivePlaybackForMedia(mediaID) {
			s.stopTranscodeSessionForMedia(mediaID)
		}
		s.notifyPlaybackCommand(retiredPlaybackID)
	}
	return nil
}

func (s *Server) handleCastRedeem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
	if !s.castReceiverOriginAllowed(origin) {
		writeError(w, http.StatusForbidden, "origin_not_allowed", "The Cast receiver origin is not allowed.")
		return
	}
	var req CastRedeemRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.BootstrapID = strings.TrimSpace(req.BootstrapID)
	req.BootstrapSecret = strings.TrimSpace(req.BootstrapSecret)
	req.ReceiverID = strings.TrimSpace(req.ReceiverID)
	req.ReceiverChallenge = strings.TrimSpace(req.ReceiverChallenge)
	if !strings.HasPrefix(req.BootstrapSecret, "ptc_cb_") || req.BootstrapID == "" || req.ReceiverID == "" || !validCastChallenge(req.ReceiverChallenge) {
		writeError(w, http.StatusUnauthorized, "cast_bootstrap_invalid", "The Cast bootstrap is no longer valid.")
		return
	}
	user, record, err := s.redeemCastBootstrap(r.Context(), req.BootstrapID, req.BootstrapSecret, req.ReceiverID, req.ReceiverChallenge, origin)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "cast_bootstrap_invalid", "The Cast bootstrap is no longer valid.")
		return
	}
	playback, err := s.openCastPlayback(record.PlaybackEnvelope)
	if err != nil || playback.SessionID != record.PlaybackSessionID {
		_ = s.endPlaybackSession(user, record.PlaybackSessionID)
		writeError(w, http.StatusUnauthorized, "cast_bootstrap_invalid", "The Cast bootstrap is no longer valid.")
		return
	}
	receiverToken := "ptc_cr_" + randomToken()
	now := time.Now().UTC()
	expiresAt := now.Add(castReceiverSessionTTL).Format(time.RFC3339)
	receiverSessionID := randomID("cast_receiver")
	err = s.withPlaybackTxTagged(r.Context(), []string{"playback"}, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(r.Context(), `
			INSERT INTO cast_receiver_sessions (id, token_hash, user_id, profile_id, receiver_id, receiver_origin, server_origin, playback_session_id, source_playback_session_id, client_instance_id, generation, capabilities_json, automation_json, status, expires_at, last_seen_at, created_at, stopped_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, '')`,
			receiverSessionID, hashToken(receiverToken), user.ID, viewerProfileID(user), record.ReceiverID, record.ReceiverOrigin, record.ServerOrigin,
			record.PlaybackSessionID, record.SourcePlaybackSessionID, record.ClientInstanceID, record.Generation, record.CapabilitiesJSON, record.AutomationJSON, expiresAt, now.Format(time.RFC3339), now.Format(time.RFC3339))
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE playback_sessions SET progress_authority = 'receiver', progress_generation = ? WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`, record.Generation, record.PlaybackSessionID, record.UserID, record.ProfileID)
		return err
	})
	if err != nil {
		_ = s.endPlaybackSession(user, record.PlaybackSessionID)
		writeError(w, http.StatusInternalServerError, "cast_redeem_failed", "Unable to establish the Cast receiver session.")
		return
	}
	capabilities := parseCastCapabilities(record.CapabilitiesJSON)
	var automation CastPlaybackAutomation
	_ = json.Unmarshal([]byte(record.AutomationJSON), &automation)
	writeJSON(w, http.StatusOK, CastReceiverSessionResponse{Version: castProtocolVersion, ReceiverSessionToken: receiverToken, ReceiverSessionID: receiverSessionID, PlaybackSessionID: record.PlaybackSessionID, ReceiverID: record.ReceiverID, ServerOrigin: record.ServerOrigin, Generation: record.Generation, Capabilities: capabilities, GrantSemantics: "initial", MediaGrantExpiresAt: playback.MediaGrant.ExpiresAt, Playback: playback, Automation: automation})
}

func (s *Server) handleCastReconnect(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var req CastReconnectRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	record, err := s.castReceiverRecordForUser(r.Context(), user, req.ReceiverSessionID, req.ClientInstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "cast_session_not_found", "The Cast session was not found.")
		return
	}
	if req.Generation > 0 && req.Generation != record.Generation {
		writeError(w, http.StatusConflict, "cast_generation_stale", "The Cast session generation is stale.")
		return
	}
	state, err := s.castReceiverState(r.Context(), record)
	if err != nil {
		writeError(w, http.StatusNotFound, "cast_session_not_found", "The Cast session was not found.")
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleCastSessionRoute(w http.ResponseWriter, r *http.Request) {
	pathValue := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/playback/cast/sessions/"), "/")
	parts := strings.Split(pathValue, "/")
	if len(parts) < 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "Cast receiver route was not found.")
		return
	}
	sessionID, action := parts[0], parts[1]
	auth, err := s.authenticateCastSession(r, sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "cast_session_invalid", "The Cast receiver session is no longer valid.")
		return
	}
	if r.Method == http.MethodOptions {
		return
	}
	if auth.record.Status != "active" && action != "stop" {
		writeError(w, http.StatusConflict, "cast_session_stopped", "The Cast session is no longer active.")
		return
	}
	switch action {
	case "state":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
			return
		}
		state, err := s.castReceiverState(r.Context(), auth.record)
		if err != nil {
			writeError(w, http.StatusNotFound, "cast_session_not_found", "The Cast session was not found.")
			return
		}
		writeJSON(w, http.StatusOK, state)
	case "control":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		if auth.record.Status != "active" {
			writeError(w, http.StatusConflict, "cast_session_stopped", "The Cast session is no longer active.")
			return
		}
		var req CastControlRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Generation != auth.record.Generation {
			writeError(w, http.StatusConflict, "cast_generation_stale", "The Cast session generation is stale.")
			return
		}
		if !castOperationAllowed(auth.record.CapabilitiesJSON, "control") {
			writeError(w, http.StatusForbidden, "cast_operation_denied", "This Cast session does not allow control.")
			return
		}
		commandID := strings.TrimSpace(req.CommandID)
		if len(commandID) > 128 {
			writeError(w, http.StatusBadRequest, "cast_command_invalid", "The Cast command identifier is invalid.")
			return
		}
		if commandID != "" && commandID == auth.record.LastCommandID && auth.record.LastCommandJSON != "" {
			var previous PlaybackCommand
			if json.Unmarshal([]byte(auth.record.LastCommandJSON), &previous) == nil {
				writeJSON(w, http.StatusOK, previous)
				return
			}
		}
		action := strings.ToLower(strings.TrimSpace(req.Operation))
		if action != "play" && action != "pause" && action != "seek" {
			writeError(w, http.StatusBadRequest, "cast_operation_invalid", "The Cast control operation is invalid.")
			return
		}
		command, err := s.issuePlaybackCommand(auth.user, auth.record.PlaybackSessionID, PlaybackCommandRequest{Action: action, PositionSeconds: max(0, int(req.PositionSeconds))})
		if err != nil {
			writeError(w, http.StatusConflict, "cast_control_failed", "The Cast control operation could not be applied.")
			return
		}
		if commandID != "" {
			if encoded, marshalErr := json.Marshal(command); marshalErr == nil {
				_, _ = s.execPlaybackWrite(r.Context(), `UPDATE cast_receiver_sessions SET last_command_id = ?, last_command_json = ? WHERE id = ? AND generation = ? AND status = 'active'`, commandID, string(encoded), auth.record.ID, auth.record.Generation)
			}
		}
		writeJSON(w, http.StatusOK, command)
	case "advance":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		if auth.record.Status != "active" {
			writeError(w, http.StatusConflict, "cast_session_stopped", "The Cast session is no longer active.")
			return
		}
		var req CastAdvanceRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Generation != auth.record.Generation {
			writeError(w, http.StatusConflict, "cast_generation_stale", "The Cast session generation is stale.")
			return
		}
		if strings.TrimSpace(req.AdvanceID) == "" || len(req.AdvanceID) > 128 {
			writeError(w, http.StatusBadRequest, "cast_advance_invalid", "advanceId is required.")
			return
		}
		if !castOperationAllowed(auth.record.CapabilitiesJSON, "advance") {
			writeError(w, http.StatusForbidden, "cast_operation_denied", "This Cast session does not allow queue advancement.")
			return
		}
		if req.AdvanceID == auth.record.LastAdvanceID && auth.record.LastAdvanceJSON != "" {
			var previous CastAdvanceResponse
			if json.Unmarshal([]byte(auth.record.LastAdvanceJSON), &previous) == nil {
				writeJSON(w, http.StatusOK, previous)
				return
			}
		}
		response, err := s.advanceCastReceiver(r.Context(), auth, req)
		if err != nil {
			if errors.Is(err, errCastGenerationStale) {
				writeError(w, http.StatusConflict, "cast_generation_stale", "The Cast session generation is stale.")
				return
			}
			writeError(w, http.StatusConflict, "cast_advance_failed", "The server could not advance this Cast queue.")
			return
		}
		encoded, _ := json.Marshal(response)
		_, _ = s.execPlaybackWrite(r.Context(), `UPDATE cast_receiver_sessions SET last_advance_id = ?, last_advance_json = ? WHERE id = ? AND generation = ? AND status = 'active'`, req.AdvanceID, string(encoded), auth.record.ID, response.Generation)
		writeJSON(w, http.StatusOK, response)
	case "segment-skip":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		if auth.record.Status != "active" {
			writeError(w, http.StatusConflict, "cast_session_stopped", "The Cast session is no longer active.")
			return
		}
		var req CastSegmentSkipRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Generation != auth.record.Generation {
			writeError(w, http.StatusConflict, "cast_generation_stale", "The Cast session generation is stale.")
			return
		}
		if !castOperationAllowed(auth.record.CapabilitiesJSON, "segment-skip") {
			writeError(w, http.StatusForbidden, "cast_operation_denied", "This Cast session does not allow segment automation.")
			return
		}
		response, err := s.castSegmentSkip(r.Context(), auth, req)
		if err != nil {
			writeError(w, http.StatusConflict, "cast_segment_skip_failed", "The requested segment is not eligible for automatic skipping.")
			return
		}
		writeJSON(w, http.StatusOK, response)
	case "progress":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		if auth.record.Status != "active" {
			writeError(w, http.StatusConflict, "cast_session_stopped", "The Cast session is no longer active.")
			return
		}
		var req CastProgressRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Generation != auth.record.Generation {
			writeError(w, http.StatusConflict, "cast_generation_stale", "The Cast session generation is stale.")
			return
		}
		if !castOperationAllowed(auth.record.CapabilitiesJSON, "progress") {
			writeError(w, http.StatusForbidden, "cast_operation_denied", "This Cast session does not allow progress.")
			return
		}
		req.PlaybackProgressEvent.Authority = "receiver"
		req.PlaybackProgressEvent.Generation = req.Generation
		ack, err := s.touchPlaybackSession(auth.user, auth.record.PlaybackSessionID, req.PlaybackProgressEvent)
		if err != nil {
			writeError(w, http.StatusConflict, "cast_progress_failed", "The Cast progress event could not be applied.")
			return
		}
		if strings.EqualFold(strings.TrimSpace(req.State), "playing") && auth.record.SourcePlaybackSessionID != "" {
			if commitErr := s.commitCastReceiverHandoff(r.Context(), auth); commitErr != nil {
				writeError(w, http.StatusConflict, "cast_handoff_commit_pending", "The Cast receiver has not yet committed ownership of the previous playback source.")
				return
			}
			auth.record.SourcePlaybackSessionID = ""
		}
		ack.MediaGrantExpiresAt = s.castMediaGrantExpiry(r.Context(), auth.user, auth.record.PlaybackSessionID)
		if ack.Accepted && castGrantNeedsExtension(ack.MediaGrantExpiresAt, time.Now().UTC()) {
			if renewErr := s.renewMediaGrantsForSession(r.Context(), auth.user, auth.record.PlaybackSessionID); renewErr == nil {
				ack.GrantExtended = true
				ack.GrantSemantics = "extension"
				ack.MediaGrantExpiresAt = s.castMediaGrantExpiry(r.Context(), auth.user, auth.record.PlaybackSessionID)
			}
		}
		writeJSON(w, http.StatusOK, ack)
	case "renew":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		if auth.record.Status != "active" {
			writeError(w, http.StatusConflict, "cast_session_stopped", "The Cast session is no longer active.")
			return
		}
		var req CastRenewRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Generation != auth.record.Generation {
			writeError(w, http.StatusConflict, "cast_generation_stale", "The Cast session generation is stale.")
			return
		}
		if !castOperationAllowed(auth.record.CapabilitiesJSON, "renew") {
			writeError(w, http.StatusForbidden, "cast_operation_denied", "This Cast session does not allow renewal.")
			return
		}
		response, err := s.renewCastReceiverGrant(r.Context(), auth, req.Mode)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "cast_renewal_denied", "The Cast media authorization could not be renewed.")
			return
		}
		writeJSON(w, http.StatusOK, response)
	case "stop":
		if r.Method != http.MethodPost && r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST or DELETE for this endpoint.")
			return
		}
		if !auth.viaReceiver && auth.record.Status != "active" {
			writeError(w, http.StatusConflict, "cast_session_stopped", "The Cast session is no longer active.")
			return
		}
		var generation int64
		if r.Body != nil && r.ContentLength != 0 {
			var req struct {
				Generation int64 `json:"generation"`
			}
			if !decodeJSON(w, r, &req) {
				return
			}
			generation = req.Generation
		}
		if generation != auth.record.Generation {
			writeError(w, http.StatusConflict, "cast_generation_stale", "The Cast session generation is stale.")
			return
		}
		if !castOperationAllowed(auth.record.CapabilitiesJSON, "stop") {
			writeError(w, http.StatusForbidden, "cast_operation_denied", "This Cast session does not allow stop.")
			return
		}
		if err := s.stopCastReceiverSession(r.Context(), auth); err != nil {
			writeError(w, http.StatusConflict, "cast_stop_failed", "The Cast session could not be stopped.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "generation": auth.record.Generation})
	default:
		writeError(w, http.StatusNotFound, "not_found", "Cast receiver operation was not found.")
	}
}

func castQueueNext(snapshot playbackSessionQueueSnapshot) (string, []string, bool) {
	if snapshot.RepeatMode == "one" {
		return snapshot.MediaID, snapshot.QueueIDs, snapshot.MediaID != ""
	}
	if len(snapshot.QueueIDs) > 0 {
		return snapshot.QueueIDs[0], append([]string(nil), snapshot.QueueIDs[1:]...), true
	}
	if snapshot.RepeatMode != "all" || len(snapshot.SourceContext.MediaIDs) < 2 {
		return "", nil, false
	}
	ids := append([]string(nil), snapshot.SourceContext.MediaIDs...)
	current := -1
	for index, id := range ids {
		if id == snapshot.MediaID {
			current = index
			break
		}
	}
	if current < 0 {
		return "", nil, false
	}
	nextIndex := (current + 1) % len(ids)
	next := ids[nextIndex]
	remaining := make([]string, 0, len(ids)-1)
	for offset := 1; offset < len(ids); offset++ {
		remaining = append(remaining, ids[(nextIndex+offset)%len(ids)])
	}
	return next, remaining, true
}

func (s *Server) advanceCastReceiver(ctx context.Context, auth castSessionAuth, req CastAdvanceRequest) (CastAdvanceResponse, error) {
	if req.Generation != auth.record.Generation {
		return CastAdvanceResponse{}, errCastGenerationStale
	}
	var automation CastPlaybackAutomation
	_ = json.Unmarshal([]byte(auth.record.AutomationJSON), &automation)
	count := auth.record.AutomaticAdvances
	if req.Automatic {
		limit := max(1, automation.PassoutAfterEpisodes)
		if automation.PassoutProtection && count >= limit {
			if err := s.stopCastReceiverSession(ctx, auth); err != nil {
				return CastAdvanceResponse{}, err
			}
			return CastAdvanceResponse{Version: castProtocolVersion, Status: "passout_stopped", Generation: auth.record.Generation, AutomaticAdvances: count, Automation: automation}, nil
		}
		count++
	} else {
		count = 0
	}
	snapshot, err := s.playbackSessionQueueSnapshot(ctx, auth.user, auth.record.PlaybackSessionID)
	if err != nil {
		return CastAdvanceResponse{}, err
	}
	nextID, remaining, ok := castQueueNext(snapshot)
	if !ok {
		if err := s.stopCastReceiverSession(ctx, auth); err != nil {
			return CastAdvanceResponse{}, err
		}
		return CastAdvanceResponse{Version: castProtocolVersion, Status: "exhausted", Generation: auth.record.Generation, AutomaticAdvances: count, Automation: automation}, nil
	}
	profile := PlaybackClientProfile{Device: "cast-receiver", Platform: "web", SupportsHLS: true, SupportsMSE: true}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://cast.invalid/advance", nil)
	playback, startErr := s.startPlaybackForRequest(request, auth.user, PlaybackSessionCreateRequest{
		// The current receiver generation must remain alive until the replacement
		// generation reports playing. Passing the sender instance here would invoke
		// ordinary replace-by-instance semantics and retire it before LOAD commits.
		MediaID: nextID, ClientInstanceID: "", ClientProfile: profile, SkipPreroll: true,
		QueueMediaIDs: remaining, RepeatMode: snapshot.RepeatMode, SourceContext: snapshot.SourceContext,
	})
	if startErr != nil {
		return CastAdvanceResponse{}, fmt.Errorf("cast advance playback start failed: %s", startErr.message)
	}
	newGeneration := auth.record.Generation + 1
	playback.Generation = int(newGeneration)
	encodedAutomation := stringJSON(automation)
	resultErr := s.withPlaybackTxTagged(ctx, []string{"playback"}, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE playback_sessions SET progress_authority = 'receiver', progress_generation = ? WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`, newGeneration, playback.SessionID, auth.record.UserID, auth.record.ProfileID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE cast_receiver_sessions SET playback_session_id = ?, source_playback_session_id = ?, generation = ?, automation_json = ?, automatic_advances = ?, status = 'active' WHERE id = ? AND generation = ? AND status = 'active'`, playback.SessionID, auth.record.PlaybackSessionID, newGeneration, encodedAutomation, count, auth.record.ID, auth.record.Generation)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			return errCastGenerationStale
		}
		return nil
	})
	if resultErr != nil {
		_ = s.endPlaybackSession(auth.user, playback.SessionID)
		return CastAdvanceResponse{}, resultErr
	}
	return CastAdvanceResponse{Version: castProtocolVersion, Status: "advanced", Generation: newGeneration, AutomaticAdvances: count, Automation: automation, Playback: &playback}, nil
}

func (s *Server) castSegmentSkip(ctx context.Context, auth castSessionAuth, req CastSegmentSkipRequest) (CastSegmentSkipResponse, error) {
	if req.Generation != auth.record.Generation {
		return CastSegmentSkipResponse{}, errCastGenerationStale
	}
	mediaID, isLive, _, err := s.playbackSessionState(auth.user, auth.record.PlaybackSessionID)
	if err != nil || isLive {
		return CastSegmentSkipResponse{}, errCastReceiverSessionInvalid
	}
	item, err := s.getMediaPlaybackDetailForUser(ctx, auth.user, mediaID)
	if err != nil {
		return CastSegmentSkipResponse{}, err
	}
	var automation CastPlaybackAutomation
	_ = json.Unmarshal([]byte(auth.record.AutomationJSON), &automation)
	for _, segment := range item.Segments {
		if segment.ID != strings.TrimSpace(req.SegmentID) || !segment.AutomaticSafe || (segment.Type != "intro" && segment.Type != "credits") {
			continue
		}
		behavior := automation.IntroSkip
		if segment.Type == "credits" {
			behavior = automation.CreditsSkip
		}
		if behavior != "automatic" {
			return CastSegmentSkipResponse{Version: castProtocolVersion, Generation: req.Generation, Skipped: false}, nil
		}
		return CastSegmentSkipResponse{Version: castProtocolVersion, Generation: req.Generation, Skipped: true, PositionSeconds: segment.EndSeconds}, nil
	}
	return CastSegmentSkipResponse{}, errCastReceiverSessionInvalid
}

func (s *Server) redeemCastBootstrap(ctx context.Context, bootstrapID, secret, receiverID, challenge, origin string) (User, castBootstrapRecord, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var record castBootstrapRecord
	var user User
	if err := s.queryUserRow(ctx, `SELECT id, user_id, profile_id, receiver_id, receiver_origin, receiver_public_key, receiver_challenge, server_origin, playback_session_id, source_playback_session_id, client_instance_id, generation, capabilities_json, automation_json, playback_envelope, expires_at, redeemed_at FROM cast_bootstraps WHERE id = ? AND token_hash = ?`, bootstrapID, hashToken(secret)).Scan(&record.ID, &record.UserID, &record.ProfileID, &record.ReceiverID, &record.ReceiverOrigin, &record.ReceiverPublicKey, &record.ReceiverChallenge, &record.ServerOrigin, &record.PlaybackSessionID, &record.SourcePlaybackSessionID, &record.ClientInstanceID, &record.Generation, &record.CapabilitiesJSON, &record.AutomationJSON, &record.PlaybackEnvelope, &record.ExpiresAt, &record.RedeemedAt); err != nil {
		return User{}, castBootstrapRecord{}, errCastBootstrapInvalid
	}
	if record.ReceiverID != receiverID || record.ReceiverOrigin != origin || record.ReceiverChallenge != challenge || record.RedeemedAt != "" || record.ExpiresAt <= now {
		return User{}, castBootstrapRecord{}, errCastBootstrapInvalid
	}
	user, err := s.castUserForScope(ctx, record.UserID, record.ProfileID)
	if err != nil || !user.Permissions["playMedia"] {
		return User{}, castBootstrapRecord{}, errCastBootstrapInvalid
	}
	if playback, playbackErr := s.openCastPlayback(record.PlaybackEnvelope); playbackErr != nil || playback.SessionID != record.PlaybackSessionID {
		return User{}, castBootstrapRecord{}, errCastBootstrapInvalid
	}
	err = s.withPlaybackTxTagged(ctx, []string{"playback"}, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE cast_bootstraps SET redeemed_at = ? WHERE id = ? AND token_hash = ? AND receiver_id = ? AND receiver_origin = ? AND receiver_challenge = ? AND redeemed_at = '' AND expires_at > ?`, now, bootstrapID, hashToken(secret), receiverID, origin, challenge, now)
		if err != nil {
			return err
		}
		count, _ := result.RowsAffected()
		if count != 1 {
			return errCastBootstrapInvalid
		}
		return nil
	})
	if err != nil {
		return User{}, castBootstrapRecord{}, errCastBootstrapInvalid
	}
	return user, record, nil
}

func (s *Server) authenticateCastSession(r *http.Request, sessionID string) (castSessionAuth, error) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), castReceiverAuthorization))
	if strings.HasPrefix(strings.TrimSpace(r.Header.Get("Authorization")), castReceiverAuthorization) && strings.HasPrefix(token, "ptc_cr_") {
		var record castReceiverRecord
		err := s.queryUserRow(r.Context(), `SELECT id, user_id, profile_id, receiver_id, receiver_origin, server_origin, playback_session_id, source_playback_session_id, client_instance_id, generation, capabilities_json, automation_json, status, expires_at, last_seen_at, last_command_id, last_command_json, automatic_advances, last_advance_id, last_advance_json FROM cast_receiver_sessions WHERE id = ? AND token_hash = ?`, sessionID, hashToken(token)).Scan(&record.ID, &record.UserID, &record.ProfileID, &record.ReceiverID, &record.ReceiverOrigin, &record.ServerOrigin, &record.PlaybackSessionID, &record.SourcePlaybackSessionID, &record.ClientInstanceID, &record.Generation, &record.CapabilitiesJSON, &record.AutomationJSON, &record.Status, &record.ExpiresAt, &record.LastSeenAt, &record.LastCommandID, &record.LastCommandJSON, &record.AutomaticAdvances, &record.LastAdvanceID, &record.LastAdvanceJSON)
		if err != nil || (record.Status != "active" && record.Status != "stopped") || record.ExpiresAt <= time.Now().UTC().Format(time.RFC3339) {
			return castSessionAuth{}, errCastReceiverSessionInvalid
		}
		user, err := s.castUserForScope(r.Context(), record.UserID, record.ProfileID)
		if err != nil || !user.Permissions["playMedia"] {
			return castSessionAuth{}, errCastReceiverSessionInvalid
		}
		now := time.Now().UTC()
		_, _ = s.execPlaybackWrite(r.Context(), `UPDATE cast_receiver_sessions SET last_seen_at = ?, expires_at = MAX(expires_at, ?) WHERE id = ? AND status = 'active'`, now.Format(time.RFC3339), now.Add(castReceiverSessionTTL).Format(time.RFC3339), record.ID)
		return castSessionAuth{record: record, user: user, viaReceiver: true}, nil
	}
	user, ok, err := s.currentUserWithError(castDiscardResponseWriter{}, r)
	if err != nil || !ok {
		return castSessionAuth{}, errCastReceiverSessionInvalid
	}
	record, err := s.castReceiverRecordForUser(r.Context(), user, sessionID, "")
	if err != nil {
		return castSessionAuth{}, err
	}
	return castSessionAuth{record: record, user: user, viaReceiver: false}, nil
}

func (s *Server) castReceiverRecordForUser(ctx context.Context, user User, sessionID, clientInstanceID string) (castReceiverRecord, error) {
	var record castReceiverRecord
	where := `id = ? AND user_id = ? AND profile_id = ? AND status = 'active' AND expires_at > ?`
	args := []any{strings.TrimSpace(sessionID), accountIDForUser(user), viewerProfileID(user), time.Now().UTC().Format(time.RFC3339)}
	if strings.TrimSpace(sessionID) == "" {
		where = `client_instance_id = ? AND user_id = ? AND profile_id = ? AND status = 'active' AND expires_at > ?`
		args[0] = normalizePlaybackClientInstanceID(clientInstanceID)
	}
	err := s.queryUserRow(ctx, `SELECT id, user_id, profile_id, receiver_id, receiver_origin, server_origin, playback_session_id, source_playback_session_id, client_instance_id, generation, capabilities_json, automation_json, status, expires_at, last_seen_at, last_command_id, last_command_json, automatic_advances, last_advance_id, last_advance_json FROM cast_receiver_sessions WHERE `+where+` ORDER BY generation DESC LIMIT 1`, args...).Scan(&record.ID, &record.UserID, &record.ProfileID, &record.ReceiverID, &record.ReceiverOrigin, &record.ServerOrigin, &record.PlaybackSessionID, &record.SourcePlaybackSessionID, &record.ClientInstanceID, &record.Generation, &record.CapabilitiesJSON, &record.AutomationJSON, &record.Status, &record.ExpiresAt, &record.LastSeenAt, &record.LastCommandID, &record.LastCommandJSON, &record.AutomaticAdvances, &record.LastAdvanceID, &record.LastAdvanceJSON)
	return record, err
}

func (s *Server) castUserForScope(ctx context.Context, userID, profileID string) (User, error) {
	principal, err := s.resolveRequestPrincipalContext(ctx, userID, profileID)
	if err != nil {
		return User{}, err
	}
	user := User{ID: userID, AccountID: userID, ProfileID: profileID}
	applyRequestPrincipal(&user, principal)
	return s.hydratePlaybackVisibilityUserContext(ctx, user), nil
}

func (s *Server) castReceiverState(ctx context.Context, record castReceiverRecord) (CastReceiverSessionState, error) {
	var state CastReceiverSessionState
	err := s.queryUserRow(ctx, `SELECT state, position_seconds, media_id FROM playback_sessions WHERE id = ? AND user_id = ? AND profile_id = ?`, record.PlaybackSessionID, record.UserID, record.ProfileID).Scan(&state.PlaybackState, &state.PositionSeconds, &state.MediaID)
	if err != nil {
		return CastReceiverSessionState{}, err
	}
	state.Version, state.ReceiverSessionID, state.PlaybackSessionID = castProtocolVersion, record.ID, record.PlaybackSessionID
	state.ReceiverID, state.ServerOrigin, state.Generation, state.Status = record.ReceiverID, record.ServerOrigin, record.Generation, record.Status
	state.Capabilities, state.LastSeenAt, state.ExpiresAt = parseCastCapabilities(record.CapabilitiesJSON), record.LastSeenAt, record.ExpiresAt
	_ = json.Unmarshal([]byte(record.AutomationJSON), &state.Automation)
	state.AutomaticAdvances = record.AutomaticAdvances
	var queueIDs []string
	rows, rowsErr := s.queryUserRead(ctx, `SELECT media_id FROM playback_session_queue WHERE session_id = ? ORDER BY sort_order ASC LIMIT ?`, record.PlaybackSessionID, maxPlaybackQueueItems)
	if rowsErr == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				queueIDs = append(queueIDs, id)
			}
		}
		_ = rows.Close()
		state.Queue, _ = s.mediaByOrderedIDsContext(ctx, record.ProfileID, queueIDs)
	}
	_ = s.queryUserRow(ctx, `SELECT repeat_mode FROM playback_sessions WHERE id = ?`, record.PlaybackSessionID).Scan(&state.RepeatMode)
	return state, nil
}

func (s *Server) renewCastReceiverGrant(ctx context.Context, auth castSessionAuth, requestedMode string) (CastRenewResponse, error) {
	mode := strings.ToLower(strings.TrimSpace(requestedMode))
	if mode == "" {
		mode = "extend"
	}
	if mode != "extend" && mode != "rotate" {
		return CastRenewResponse{}, errCastReceiverSessionInvalid
	}
	mediaID, isLive, _, err := s.playbackSessionState(auth.user, auth.record.PlaybackSessionID)
	if err != nil {
		return CastRenewResponse{}, errCastReceiverSessionInvalid
	}
	resourceKind := "media"
	if isLive {
		resourceKind = "live_channel"
	}
	var grant *MediaGrant
	semantics := "extension"
	if mode == "rotate" {
		rotated, err := s.rotateMediaGrantForSession(ctx, auth.user, auth.record.PlaybackSessionID, resourceKind, mediaID)
		if err != nil {
			return CastRenewResponse{}, err
		}
		grant, semantics = &rotated, "rotation"
	} else if err := s.renewMediaGrantsForSession(ctx, auth.user, auth.record.PlaybackSessionID); err != nil {
		return CastRenewResponse{}, err
	}
	var expiresAt string
	if err := s.queryUserRow(ctx, `SELECT MAX(expires_at) FROM playback_media_grants WHERE playback_session_id = ? AND principal_user_id = ? AND profile_id = ? AND revoked_at = ''`, auth.record.PlaybackSessionID, accountIDForUser(auth.user), viewerProfileID(auth.user)).Scan(&expiresAt); err != nil {
		return CastRenewResponse{}, err
	}
	state, err := s.castReceiverState(ctx, auth.record)
	if err != nil {
		return CastRenewResponse{}, err
	}
	return CastRenewResponse{Version: castProtocolVersion, GrantSemantics: semantics, Generation: auth.record.Generation, MediaGrant: grant, MediaGrantExpiresAt: expiresAt, ReceiverSession: state}, nil
}

func castGrantNeedsExtension(expiresAt string, now time.Time) bool {
	expires, err := time.Parse(time.RFC3339, expiresAt)
	return err != nil || expires.Before(now.Add(5*time.Minute))
}

func (s *Server) castMediaGrantExpiry(ctx context.Context, user User, playbackSessionID string) string {
	var expiresAt string
	_ = s.queryUserRow(ctx, `SELECT COALESCE(MAX(expires_at), '') FROM playback_media_grants WHERE playback_session_id = ? AND principal_user_id = ? AND profile_id = ? AND revoked_at = ''`, playbackSessionID, accountIDForUser(user), viewerProfileID(user)).Scan(&expiresAt)
	return expiresAt
}

func (s *Server) stopCastReceiverSession(ctx context.Context, auth castSessionAuth) error {
	now := time.Now().UTC().Format(time.RFC3339)
	err := s.withPlaybackTxTagged(ctx, []string{"playback"}, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE cast_receiver_sessions SET status = 'stopped', stopped_at = ?, last_seen_at = ? WHERE id = ? AND generation = ? AND status = 'active'`, now, now, auth.record.ID, auth.record.Generation); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE playback_sessions SET state = 'stopped', ended_at = ?, last_seen_at = ? WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`, now, now, auth.record.PlaybackSessionID, auth.record.UserID, auth.record.ProfileID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE playback_media_grants SET revoked_at = ? WHERE playback_session_id = ? AND revoked_at = ''`, now, auth.record.PlaybackSessionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE playback_session_continuation_credentials SET revoked_at = ?, previous_valid_until = '' WHERE playback_session_id = ? AND revoked_at = ''`, now, auth.record.PlaybackSessionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM live_tv_tuner_allocations WHERE allocation_kind = 'live_session' AND consumer_id = ?`, auth.record.PlaybackSessionID); err != nil {
			return err
		}
		return nil
	})
	s.forgetMediaGrantsForPlaybackSession(auth.record.PlaybackSessionID)
	return err
}

func (s *Server) commitCastReceiverHandoff(ctx context.Context, auth castSessionAuth) error {
	sourceSessionID := strings.TrimSpace(auth.record.SourcePlaybackSessionID)
	if sourceSessionID == "" || sourceSessionID == auth.record.PlaybackSessionID {
		return nil
	}
	mediaID, err := s.finalizePlaybackSessionProgress(auth.user, sourceSessionID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	endedAt := now.Format(time.RFC3339)
	revokedAt := now.Format(time.RFC3339Nano)
	err = s.withPlaybackTxTagged(ctx, []string{"playback", "live-tv", "dvr"}, func(tx *sql.Tx) error {
		var currentPlaybackSessionID, currentSourceSessionID string
		if err := tx.QueryRowContext(ctx, `
			SELECT playback_session_id, source_playback_session_id
			FROM cast_receiver_sessions
			WHERE id = ? AND user_id = ? AND profile_id = ? AND generation = ? AND status = 'active'`,
			auth.record.ID, auth.record.UserID, auth.record.ProfileID, auth.record.Generation,
		).Scan(&currentPlaybackSessionID, &currentSourceSessionID); err != nil {
			return err
		}
		if currentPlaybackSessionID != auth.record.PlaybackSessionID || currentSourceSessionID != sourceSessionID {
			return errCastGenerationStale
		}

		result, err := tx.ExecContext(ctx, `
			UPDATE playback_sessions
			SET last_seen_at = ?, ended_at = ?, state = 'stopped'
			WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`,
			endedAt, endedAt, sourceSessionID, auth.record.UserID, auth.record.ProfileID,
		)
		if err != nil {
			return err
		}
		if count, rowsErr := result.RowsAffected(); rowsErr != nil {
			return rowsErr
		} else if count != 1 {
			return errCastGenerationStale
		}
		if _, err := tx.ExecContext(ctx, `UPDATE playback_media_grants SET revoked_at = ? WHERE playback_session_id = ? AND revoked_at = ''`, revokedAt, sourceSessionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE playback_session_continuation_credentials SET revoked_at = ?, previous_valid_until = '' WHERE playback_session_id = ? AND revoked_at = ''`, revokedAt, sourceSessionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM live_tv_tuner_allocations WHERE allocation_kind = 'live_session' AND consumer_id = ?`, sourceSessionID); err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx, `
			UPDATE cast_receiver_sessions
			SET source_playback_session_id = ''
			WHERE id = ? AND user_id = ? AND profile_id = ? AND playback_session_id = ?
				AND generation = ? AND status = 'active' AND source_playback_session_id = ?`,
			auth.record.ID, auth.record.UserID, auth.record.ProfileID, auth.record.PlaybackSessionID,
			auth.record.Generation, sourceSessionID,
		)
		if err != nil {
			return err
		}
		if count, rowsErr := result.RowsAffected(); rowsErr != nil {
			return rowsErr
		} else if count != 1 {
			return errCastGenerationStale
		}
		return nil
	})
	if err != nil {
		return err
	}

	// These are process-local or derived cleanup operations. The authoritative
	// receiver/source ownership transition above is already committed and can
	// never expose an active source with a cleared handoff pointer.
	s.forgetMediaGrantsForPlaybackSession(sourceSessionID)
	_ = s.completeEndedPlaybackSessionHistoryContext(context.Background(), sourceSessionID)
	if !s.hasActivePlaybackForMedia(mediaID) {
		s.stopTranscodeSessionForMedia(mediaID)
	}
	s.notifyPlaybackCommand(sourceSessionID)
	return nil
}

func normalizeCastCapabilities(values []string) ([]string, bool) {
	if len(values) == 0 {
		values = []string{"load", "control", "stop", "progress", "renew", "reconnect"}
	}
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || !castReceiverOperations[value] || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	if !seen["load"] || !seen["progress"] || !seen["renew"] || len(result) == 0 {
		return nil, false
	}
	return result, true
}

func parseCastCapabilities(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	normalized, _ := normalizeCastCapabilities(values)
	return normalized
}

func castOperationAllowed(raw, operation string) bool {
	for _, value := range parseCastCapabilities(raw) {
		if value == operation {
			return true
		}
	}
	return false
}

func stringJSON(value any) string { raw, _ := json.Marshal(value); return string(raw) }

// castPlaybackAutomation is a server-approved projection of viewer playback
// preferences. It intentionally excludes the original preference document and
// all account credentials from Cast custom data and receiver state.
func (s *Server) castPlaybackAutomation(ctx context.Context, user User, clientInstanceID string) CastPlaybackAutomation {
	value := defaultProfileServerValues(user)
	if scope, err := s.viewerPreferenceScope(ctx, user, "television", normalizePlaybackClientInstanceID(clientInstanceID), ""); err == nil {
		if bundle, loadErr := s.loadViewerPreferenceBundle(ctx, user, scope); loadErr == nil {
			_ = json.Unmarshal(bundle.ProfileServer.Values, &value)
		}
	}
	return normalizeCastAutomation(CastPlaybackAutomation{
		AutoplayNext:           value.Playback.AutoplayNext,
		UpNextCountdownSeconds: value.Playback.UpNextCountdownSeconds,
		PassoutProtection:      value.Playback.PassoutProtection,
		PassoutAfterEpisodes:   value.Playback.PassoutAfterEpisodes,
		IntroSkip:              value.Playback.IntroSkip,
		CreditsSkip:            value.Playback.CreditsSkip,
	})
}

func normalizeCastAutomation(value CastPlaybackAutomation) CastPlaybackAutomation {
	mode := func(raw string) string {
		raw = strings.ToLower(strings.TrimSpace(raw))
		if raw != "automatic" && raw != "ask" && raw != "off" {
			return "ask"
		}
		return raw
	}
	if value.PassoutAfterEpisodes < 1 {
		value.PassoutAfterEpisodes = 1
	}
	if value.PassoutAfterEpisodes > 100 {
		value.PassoutAfterEpisodes = 100
	}
	if value.UpNextCountdownSeconds != 0 && value.UpNextCountdownSeconds != 5 && value.UpNextCountdownSeconds != 10 && value.UpNextCountdownSeconds != 15 {
		value.UpNextCountdownSeconds = 10
	}
	value.IntroSkip = mode(value.IntroSkip)
	value.CreditsSkip = mode(value.CreditsSkip)
	return value
}

func validCastChallenge(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= 16 && len(value) <= 256 && !strings.ContainsAny(value, " \t\r\n")
}

func decodeCastPublicKey(value string) (*ecdh.PublicKey, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return nil, errors.New("missing cast receiver public key")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	return ecdh.P256().NewPublicKey(raw)
}

func canonicalCastServerOrigin(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Port() != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return ""
	}
	return "https://" + strings.ToLower(parsed.Host)
}

func (s *Server) castPublicServerOrigin(_ *http.Request) string {
	return canonicalCastServerOrigin(s.cfg.PublicOrigin)
}

func absolutizeCastResourceURL(rawURL, serverOrigin string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", nil
	}
	origin := canonicalCastServerOrigin(serverOrigin)
	if origin == "" {
		return "", errors.New("invalid Cast server origin")
	}
	base, err := url.Parse(origin + "/")
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(value)
	if err != nil || strings.Contains(parsed.Path, "..") || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("invalid Cast resource URL")
	}
	for key := range parsed.Query() {
		switch strings.ToLower(key) {
		case "media_grant", "access_token", "token", "authorization":
			return "", errors.New("Cast resource URL contains a credential query")
		}
	}
	resolved := base.ResolveReference(parsed)
	if resolved.Scheme != "https" || !strings.EqualFold(resolved.Host, base.Host) || resolved.User != nil || resolved.Fragment != "" || resolved.Path == "" {
		return "", errors.New("Cast resource URL is off-origin")
	}
	return resolved.String(), nil
}

func absolutizeCastPlaybackURLs(playback *PlaybackResponse, serverOrigin string) error {
	if playback == nil {
		return errors.New("missing Cast playback")
	}
	assign := func(value *string) error {
		absolute, err := absolutizeCastResourceURL(*value, serverOrigin)
		if err != nil {
			return err
		}
		*value = absolute
		return nil
	}
	if err := assign(&playback.SourceURL); err != nil {
		return err
	}
	if err := absolutizeCastMediaItemURLs(&playback.Media, assign); err != nil {
		return err
	}
	for index := range playback.Resources {
		if err := assign(&playback.Resources[index].SourceURL); err != nil {
			return err
		}
	}
	for index := range playback.AudioStreams {
		if err := assign(&playback.AudioStreams[index].SourceURL); err != nil {
			return err
		}
	}
	for index := range playback.SubtitleStreams {
		if err := assign(&playback.SubtitleStreams[index].SourceURL); err != nil {
			return err
		}
	}
	for index := range playback.Queue {
		if err := absolutizeCastMediaItemURLs(&playback.Queue[index], assign); err != nil {
			return err
		}
	}
	return nil
}

func absolutizeCastMediaItemURLs(item *MediaItem, assign func(*string) error) error {
	if err := assign(&item.SourceURL); err != nil {
		return err
	}
	for index := range item.Streams {
		if err := assign(&item.Streams[index].SourceURL); err != nil {
			return err
		}
	}
	for index := range item.OptimizedVersions {
		if err := assign(&item.OptimizedVersions[index].DownloadURL); err != nil {
			return err
		}
		if err := assign(&item.OptimizedVersions[index].StreamURL); err != nil {
			return err
		}
	}
	for index := range item.Chapters {
		if err := assign(&item.Chapters[index].ThumbURL); err != nil {
			return err
		}
	}
	for index := range item.Children {
		if err := absolutizeCastMediaItemURLs(&item.Children[index], assign); err != nil {
			return err
		}
	}
	if item.PlaybackTarget != nil {
		if err := absolutizeCastMediaItemURLs(item.PlaybackTarget, assign); err != nil {
			return err
		}
	}
	for extraIndex := range item.Extras {
		for itemIndex := range item.Extras[extraIndex].Items {
			if err := absolutizeCastMediaItemURLs(&item.Extras[extraIndex].Items[itemIndex], assign); err != nil {
				return err
			}
		}
	}
	return nil
}

func castEnvelopeAAD(bootstrapID, receiverID, receiverOrigin, serverOrigin, receiverChallenge string) []byte {
	return []byte(castEnvelopeLabel + "|" + bootstrapID + "|" + receiverID + "|" + receiverOrigin + "|" + serverOrigin + "|" + receiverChallenge)
}

func castEnvelopeAEAD(shared []byte) (cipher.AEAD, error) {
	hash := sha256.Sum256(append(append([]byte{}, shared...), []byte(castEnvelopeLabel)...))
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func makeCastBootstrapEnvelope(bootstrapID, receiverID, receiverOrigin, serverOrigin, receiverChallenge string, receiverPublicKey *ecdh.PublicKey, secret, expiresAt string) (string, error) {
	serverPrivateKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	shared, err := serverPrivateKey.ECDH(receiverPublicKey)
	if err != nil {
		return "", err
	}
	aead, err := castEnvelopeAEAD(shared)
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(castBootstrapSecretPayload{
		Version: castProtocolVersion, BootstrapID: bootstrapID, ReceiverID: receiverID,
		ReceiverOrigin: receiverOrigin, ReceiverChallenge: receiverChallenge, ServerOrigin: serverOrigin, Secret: secret, ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, nonce, plain, castEnvelopeAAD(bootstrapID, receiverID, receiverOrigin, serverOrigin, receiverChallenge))
	envelope, err := json.Marshal(castBootstrapEnvelope{
		Version: castProtocolVersion, BootstrapID: bootstrapID, ReceiverID: receiverID,
		ReceiverOrigin: receiverOrigin, ReceiverChallenge: receiverChallenge,
		ServerOrigin:    serverOrigin,
		ServerPublicKey: base64.RawURLEncoding.EncodeToString(serverPrivateKey.PublicKey().Bytes()),
		Nonce:           base64.RawURLEncoding.EncodeToString(nonce), Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(envelope), nil
}

func (s *Server) sealCastPlayback(playback PlaybackResponse) (string, error) {
	aead, err := s.cursorAEAD()
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(playback)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, plain, []byte("portico-cast-playback-v1"))), nil
}

func (s *Server) openCastPlayback(envelope string) (PlaybackResponse, error) {
	aead, err := s.cursorAEAD()
	if err != nil {
		return PlaybackResponse{}, err
	}
	raw, err := base64.RawURLEncoding.DecodeString(envelope)
	if err != nil || len(raw) <= aead.NonceSize() {
		return PlaybackResponse{}, errCastBootstrapInvalid
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte("portico-cast-playback-v1"))
	if err != nil {
		return PlaybackResponse{}, errCastBootstrapInvalid
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(plain, &playback); err != nil {
		return PlaybackResponse{}, err
	}
	return playback, nil
}

type castDiscardResponseWriter struct{}

func (castDiscardResponseWriter) Header() http.Header            { return http.Header{} }
func (castDiscardResponseWriter) Write(body []byte) (int, error) { return len(body), nil }
func (castDiscardResponseWriter) WriteHeader(int)                {}
