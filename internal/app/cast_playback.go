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
	RequestID         string                      `json:"requestId"`
	VersionID         string                      `json:"versionId,omitempty"`
	ClientInstanceID  string                      `json:"clientInstanceId"`
	ClientProfile     PlaybackClientProfile       `json:"clientProfile"`
	Intent            PlaybackIntent              `json:"intent,omitempty"`
	SkipPreroll       bool                        `json:"skipPreroll,omitempty"`
	BurnInSubtitleID  string                      `json:"burnInSubtitleId,omitempty"`
	SubtitleStreamID  string                      `json:"subtitleStreamId,omitempty"`
	AudioStreamID     string                      `json:"audioStreamId,omitempty"`
	StartSeconds      int                         `json:"startSeconds,omitempty"`
	QueueMediaIDs     []string                    `json:"queueMediaIds,omitempty"`
	RepeatMode        string                      `json:"repeatMode,omitempty"`
	SourceContext     PlaybackSourceContext       `json:"sourceContext,omitempty"`
	SourceKind        string                      `json:"sourceKind,omitempty"`
	SourceID          string                      `json:"sourceId,omitempty"`
	Replacement       *PlaybackReplacementRequest `json:"replacement,omitempty"`
	ReceiverID        string                      `json:"receiverId"`
	ReceiverOrigin    string                      `json:"receiverOrigin"`
	ReceiverPublicKey string                      `json:"receiverPublicKey"`
	ReceiverChallenge string                      `json:"receiverChallenge"`
	Capabilities      []string                    `json:"capabilities,omitempty"`
}

type CastBootstrapResponse struct {
	Version              string   `json:"version"`
	BootstrapEnvelope    string   `json:"bootstrapEnvelope"`
	BootstrapID          string   `json:"bootstrapId"`
	ReceiverID           string   `json:"receiverId"`
	ReceiverOrigin       string   `json:"receiverOrigin"`
	ServerOrigin         string   `json:"serverOrigin"`
	Generation           int64    `json:"generation"`
	ExpiresAt            string   `json:"expiresAt"`
	Capabilities         []string `json:"capabilities"`
	RequestID            string   `json:"requestId,omitempty"`
	SourceSessionID      string   `json:"sourceSessionId,omitempty"`
	ReplacementSessionID string   `json:"replacementSessionId,omitempty"`
	TransferStatus       string   `json:"transferStatus"`
}

type CastTransferStatusRequest struct {
	ClientInstanceID string `json:"clientInstanceId"`
	SourceSessionID  string `json:"sourceSessionId"`
	RequestID        string `json:"requestId"`
}

type CastTransferStatusResponse struct {
	Version              string                 `json:"version"`
	SourceSessionID      string                 `json:"sourceSessionId,omitempty"`
	RequestID            string                 `json:"requestId"`
	ReplacementSessionID string                 `json:"replacementSessionId"`
	Status               string                 `json:"status"`
	RequestFingerprint   string                 `json:"requestFingerprint"`
	PreviousTerminal     *PlaybackTerminalEvent `json:"previousTerminal,omitempty"`
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
	Generation       int64                 `json:"generation"`
	AdvanceID        string                `json:"advanceId"`
	Automatic        bool                  `json:"automatic,omitempty"`
	RequestID        string                `json:"requestId"`
	PreviousTerminal PlaybackTerminalEvent `json:"previousTerminal"`
}

type CastAdvanceCancelRequest struct {
	Generation int64  `json:"generation"`
	RequestID  string `json:"requestId"`
}

type CastStopRequest struct {
	Generation int64                 `json:"generation"`
	RequestID  string                `json:"requestId"`
	Terminal   PlaybackTerminalEvent `json:"terminal"`
}

type CastAdvanceResponse struct {
	Version              string                 `json:"version"`
	Status               string                 `json:"status"`
	RequestID            string                 `json:"requestId"`
	RequestFingerprint   string                 `json:"requestFingerprint"`
	PreviousTerminal     PlaybackTerminalEvent  `json:"previousTerminal"`
	SourceSessionID      string                 `json:"sourceSessionId"`
	ReplacementSessionID string                 `json:"replacementSessionId,omitempty"`
	Generation           int64                  `json:"generation"`
	AutomaticAdvances    int                    `json:"automaticAdvances"`
	Automation           CastPlaybackAutomation `json:"automation"`
	Playback             *PlaybackResponse      `json:"playback,omitempty"`
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
	Version             string                 `json:"version"`
	ReceiverSessionID   string                 `json:"receiverSessionId"`
	PlaybackSessionID   string                 `json:"playbackSessionId"`
	ReceiverID          string                 `json:"receiverId"`
	ServerOrigin        string                 `json:"serverOrigin"`
	Generation          int64                  `json:"generation"`
	Status              string                 `json:"status"`
	PlaybackState       string                 `json:"playbackState"`
	PositionSeconds     int                    `json:"positionSeconds"`
	MediaID             string                 `json:"mediaId"`
	CurrentQueueEntryID string                 `json:"currentQueueEntryId"`
	Capabilities        []string               `json:"capabilities"`
	LastSeenAt          string                 `json:"lastSeenAt"`
	ExpiresAt           string                 `json:"expiresAt"`
	Automation          CastPlaybackAutomation `json:"automation"`
	AutomaticAdvances   int                    `json:"automaticAdvances"`
	RepeatMode          string                 `json:"repeatMode"`
	Queue               []PlaybackQueueEntry   `json:"queue"`
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
	ReplacementRequestID    string
	ReplacementFingerprint  string
	ReplacementClaimID      string
	ReplacementTerminalJSON string
	AuthorizationRevision   string
	BootstrapResponseJSON   string
	RedeemResponseEnvelope  string
	TransferState           string
	PayloadExpiresAt        string
	ExpiresAt               string
	RedeemedAt              string
}

type castReceiverRecord struct {
	ID                            string
	UserID                        string
	ProfileID                     string
	ReceiverID                    string
	ReceiverOrigin                string
	ServerOrigin                  string
	PlaybackSessionID             string
	ClientInstanceID              string
	Generation                    int64
	CapabilitiesJSON              string
	Status                        string
	ExpiresAt                     string
	LastSeenAt                    string
	LastCommandID                 string
	LastCommandJSON               string
	AutomationJSON                string
	AutomaticAdvances             int
	LastAdvanceID                 string
	LastAdvanceRequestFingerprint string
	LastAdvanceJSON               string
	SourcePlaybackSessionID       string
	ReplacementRequestID          string
	ReplacementFingerprint        string
	ReplacementClaimID            string
	ReplacementTerminalJSON       string
	AuthorizationRevision         string
	TransferState                 string
	PendingPlaybackSessionID      string
	PendingGeneration             int64
	PendingRequestID              string
	PendingFingerprint            string
	PendingClaimID                string
	PendingTerminalJSON           string
	PendingAuthorizationRevision  string
	PendingAdvanceID              string
	PendingAdvanceJSON            string
	PendingPayloadExpiresAt       string
	PendingExpiresAt              string
}

type castSessionAuth struct {
	record      castReceiverRecord
	user        User
	viaReceiver bool
}

func (s *Server) discardFailedCastPlayback(ctx context.Context, user User, sessionID string) {
	_, _ = s.playbackLifecycle().Terminate(ctx, playbackTerminationRequest{
		SessionID: sessionID, UserID: accountIDForUser(user), ProfileID: viewerProfileID(user),
		Cause: playbackTerminationFailedStart, RemoveSession: true,
	})
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
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.ReceiverOrigin = strings.TrimRight(strings.TrimSpace(req.ReceiverOrigin), "/")
	req.ClientInstanceID = normalizePlaybackClientInstanceID(req.ClientInstanceID)
	req.SourceKind = strings.ToLower(strings.TrimSpace(req.SourceKind))
	req.SourceID = strings.TrimSpace(req.SourceID)
	if (req.SourceKind != "media" && req.SourceKind != "live" && req.SourceKind != "dvr" && req.SourceKind != "library-channel") || req.SourceID == "" {
		writeError(w, http.StatusBadRequest, "invalid_cast_source", "Cast playback requires a supported source kind and source ID.")
		return
	}
	if !validPlaybackAuthorityRequestID(req.RequestID) || req.Replacement != nil && strings.TrimSpace(req.Replacement.RequestID) != req.RequestID {
		writeError(w, http.StatusBadRequest, "cast_request_id_invalid", "requestId must be stable and must match replacement.requestId when transferring active playback.")
		return
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
	fingerprint := playbackReplacementTargetFingerprint("cast:"+req.SourceKind, req.SourceID, req)
	retrySourceID := ""
	if req.Replacement != nil {
		retrySourceID = strings.TrimSpace(req.Replacement.SourceSessionID)
	}
	if previous, found, retryErr := s.exactCastBootstrapRetry(r.Context(), user, req.ClientInstanceID, retrySourceID, req.RequestID, fingerprint); retryErr != nil {
		if errors.Is(retryErr, errPlaybackReplacementSourceInactive) {
			writeError(w, http.StatusConflict, "replacement_source_inactive", "The source playback authority is no longer active.")
			return
		}
		writeError(w, http.StatusConflict, "cast_bootstrap_retry_conflict", "The Cast bootstrap retry does not match its original authority transfer.")
		return
	} else if found {
		if previous.TransferStatus != "pending" {
			writeProductErrorWithDetails(w, http.StatusConflict, "cast_transfer_"+previous.TransferStatus, "The exact Cast transfer already has a definitive outcome.", map[string]any{
				"requestId": previous.RequestID, "sourceSessionId": previous.SourceSessionID,
				"replacementSessionId": previous.ReplacementSessionID, "outcome": previous.TransferStatus,
			})
			return
		}
		writeJSON(w, http.StatusCreated, previous)
		return
	}
	var replacementPlan *playbackReplacementPlan
	if req.Replacement != nil {
		sourceID := strings.TrimSpace(req.Replacement.SourceSessionID)
		var sourceMediaID string
		if err := s.queryUserRow(r.Context(), `SELECT media_id FROM playback_sessions WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`, sourceID, accountIDForUser(user), viewerProfileID(user)).Scan(&sourceMediaID); errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusConflict, "replacement_source_inactive", "The source playback authority is no longer active.")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "cast_bootstrap_failed", "Unable to inspect the source playback authority.")
			return
		}
		expectedMediaID, expectedErr := s.castTargetSessionMediaID(r.Context(), user, req.SourceKind, req.SourceID)
		if expectedErr != nil || sourceMediaID != expectedMediaID {
			writeError(w, http.StatusConflict, "cast_source_session_invalid", "The source playback session is no longer eligible for Cast transfer.")
			return
		}
	}
	plan, replacementErr := s.preparePlaybackReplacement(r.Context(), user, req.ClientInstanceID, "cast:"+req.SourceKind, req.SourceID, req, req.Replacement)
	if replacementErr != nil {
		writePlaybackStartError(w, replacementErr)
		return
	}
	if plan.Committed != nil {
		writeProductErrorWithDetails(w, http.StatusConflict, "cast_transfer_committed", "The Cast authority transfer already committed. Reconnect to the receiver session instead of creating another bootstrap.", map[string]any{"replacementSessionId": plan.Committed.SessionID, "outcome": "committed"})
		return
	}
	if plan.Active {
		replacementPlan = &plan
	}
	constructionPlan := replacementPlan
	if constructionPlan == nil {
		authorizationRevision, revisionErr := s.authorizationRevisionForUserContextStrict(r.Context(), user)
		if revisionErr != nil || authorizationRevision == "" {
			writeError(w, http.StatusConflict, "cast_scope_changed", "Playback authorization could not be pinned for this Cast bootstrap.")
			return
		}
		constructionPlan = &playbackReplacementPlan{
			RequestID: req.RequestID, Fingerprint: fingerprint,
			Claim: playbackHandoffClaim{ReplacementSessionID: randomID("castplay"), AuthorizationRevision: authorizationRevision},
		}
	}
	releaseReplacement := replacementPlan != nil
	defer func() {
		if releaseReplacement {
			s.rollbackPlaybackReplacement(plan)
		}
	}()
	var activeSessionID string
	var activeGeneration int64
	_ = s.queryUserRow(r.Context(), `
		SELECT id, generation
		FROM cast_receiver_sessions
		WHERE user_id = ? AND profile_id = ? AND client_instance_id = ? AND status = 'active' AND expires_at > ?
		ORDER BY generation DESC LIMIT 1`, accountIDForUser(user), viewerProfileID(user), req.ClientInstanceID, time.Now().UTC().Format(time.RFC3339)).Scan(&activeSessionID, &activeGeneration)
	if activeSessionID != "" {
		writeError(w, http.StatusConflict, "cast_receiver_terminal_required", "The active Cast receiver must submit its own exact terminal event before another receiver session can be created.")
		return
	}
	_ = activeGeneration
	// This is a genuinely fresh Cast start: preparePlaybackReplacement above
	// proved that this client has no active playback authority.
	playback, startErr := s.startCastTargetPlayback(r, user, req, constructionPlan)
	if startErr != nil {
		if startErr.retryAfter != "" {
			w.Header().Set("Retry-After", startErr.retryAfter)
		}
		writePlaybackStartError(w, startErr)
		return
	}
	serverOrigin := s.castPublicServerOrigin(r)
	if serverOrigin == "" {
		s.discardFailedCastPlayback(r.Context(), user, playback.SessionID)
		writeError(w, http.StatusBadRequest, "cast_server_origin_unavailable", "The selected server does not have a usable public origin.")
		return
	}
	if err := absolutizeCastPlaybackURLs(&playback, serverOrigin); err != nil {
		s.discardFailedCastPlayback(r.Context(), user, playback.SessionID)
		writeError(w, http.StatusConflict, "cast_playback_source_invalid", "The selected playback source is not reachable through the verified public server origin.")
		return
	}
	// Cast receiver routes are the only progress/terminal authority. Never hand
	// the pending receiver a second continuation bearer that could bypass the
	// first-playing commit boundary.
	playback.ContinuationCredential = nil
	playbackEnvelope, err := s.sealCastPlayback(playback)
	if err != nil {
		s.discardFailedCastPlayback(r.Context(), user, playback.SessionID)
		writeError(w, http.StatusInternalServerError, "cast_bootstrap_failed", "Unable to prepare Cast playback.")
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(castBootstrapTTL).Format(time.RFC3339)
	bootstrapID := randomID("cast_bootstrap")
	secret := "ptc_cb_" + randomToken()
	bootstrapEnvelope, err := makeCastBootstrapEnvelope(bootstrapID, req.ReceiverID, req.ReceiverOrigin, serverOrigin, req.ReceiverChallenge, receiverPublicKey, secret, expiresAt)
	if err != nil {
		s.discardFailedCastPlayback(r.Context(), user, playback.SessionID)
		writeError(w, http.StatusInternalServerError, "cast_bootstrap_failed", "Unable to prepare Cast playback.")
		return
	}
	automation := s.castPlaybackAutomation(r.Context(), user, req.ClientInstanceID)
	transferStatus := "pending"
	response := CastBootstrapResponse{
		Version: castProtocolVersion, BootstrapEnvelope: bootstrapEnvelope, BootstrapID: bootstrapID,
		ReceiverID: req.ReceiverID, ReceiverOrigin: req.ReceiverOrigin, ServerOrigin: serverOrigin,
		Generation: 1, ExpiresAt: expiresAt, Capabilities: capabilities, TransferStatus: transferStatus,
		RequestID: req.RequestID, SourceSessionID: plan.SourceSessionID, ReplacementSessionID: playback.SessionID,
	}
	responseJSON, _ := json.Marshal(response)
	terminalJSON := ""
	if replacementPlan != nil {
		terminalJSON = stringJSON(plan.Terminal)
	}
	err = s.withPlaybackTxTagged(r.Context(), []string{"playback"}, func(tx *sql.Tx) error {
		var activeCount int
		var activeID string
		if countErr := tx.QueryRowContext(r.Context(), `
			SELECT COUNT(*), COALESCE(MIN(id), '') FROM playback_sessions
			WHERE user_id = ? AND profile_id = ? AND client_instance_id = ?
				AND ended_at = '' AND state NOT IN ('stopped', 'handoff_pending')`,
			accountIDForUser(user), viewerProfileID(user), req.ClientInstanceID).Scan(&activeCount, &activeID); countErr != nil {
			return countErr
		}
		if plan.SourceSessionID == "" {
			if activeCount != 0 {
				return errPlaybackReplacementRequired
			}
		} else if activeCount == 0 {
			return errPlaybackReplacementSourceInactive
		} else if activeCount != 1 || activeID != plan.SourceSessionID {
			return errPlaybackReplacementRevisionConflict
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE playback_sessions SET progress_authority = 'receiver', progress_generation = ? WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`, int64(1), playback.SessionID, accountIDForUser(user), viewerProfileID(user)); err != nil {
			return err
		}
		_, err := tx.ExecContext(r.Context(), `
			INSERT INTO cast_bootstraps (id, token_hash, user_id, profile_id, receiver_id, receiver_origin, receiver_public_key, receiver_challenge, server_origin, playback_session_id, source_playback_session_id, client_instance_id, generation, capabilities_json, automation_json, playback_envelope, expires_at, redeemed_at, created_at, replacement_request_id, replacement_fingerprint, replacement_claim_id, replacement_terminal_json, authorization_revision, bootstrap_response_json, payload_expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?)`,
			bootstrapID, hashToken(secret), accountIDForUser(user), viewerProfileID(user), req.ReceiverID, req.ReceiverOrigin,
			req.ReceiverPublicKey, req.ReceiverChallenge, serverOrigin, playback.SessionID, plan.SourceSessionID, req.ClientInstanceID, 1, stringJSON(capabilities), stringJSON(automation), playbackEnvelope, expiresAt, now.Format(time.RFC3339),
			req.RequestID, fingerprint, constructionPlan.Claim.ID, terminalJSON, constructionPlan.Claim.AuthorizationRevision, string(responseJSON), now.Add(playbackHandoffReceiptTTL).Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		s.discardFailedCastPlayback(r.Context(), user, playback.SessionID)
		if previous, found, retryErr := s.exactCastBootstrapRetry(r.Context(), user, req.ClientInstanceID, retrySourceID, req.RequestID, fingerprint); retryErr == nil && found && previous.TransferStatus == "pending" {
			writeJSON(w, http.StatusCreated, previous)
			return
		}
		if isCastBootstrapUniqueConstraint(err) {
			writeError(w, http.StatusConflict, "cast_bootstrap_pending", "This client already has a pending Cast bootstrap. Reconcile or expire that exact request before starting another.")
			return
		}
		if errors.Is(err, errPlaybackReplacementSourceInactive) {
			writeError(w, http.StatusConflict, "replacement_source_inactive", "The source playback authority is no longer active.")
			return
		}
		if errors.Is(err, errPlaybackReplacementRequired) || errors.Is(err, errPlaybackReplacementRevisionConflict) {
			writeError(w, http.StatusConflict, "replacement_required", "Playback authority changed while the Cast bootstrap was being reserved. Retry with the active source's exact replacement envelope.")
			return
		}
		writeError(w, http.StatusInternalServerError, "cast_bootstrap_failed", "Unable to prepare Cast playback.")
		return
	}
	releaseReplacement = false
	writeJSON(w, http.StatusCreated, response)
}

func isCastBootstrapUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") && strings.Contains(message, "cast_bootstraps.")
}

func (s *Server) exactCastBootstrapRetry(ctx context.Context, user User, clientInstanceID, sourceSessionID, requestID, fingerprint string) (CastBootstrapResponse, bool, error) {
	var storedFingerprint, storedSource, encoded, expiresAt, state, playbackSessionID, claimID string
	err := s.queryUserRow(ctx, `
		SELECT replacement_fingerprint, source_playback_session_id, bootstrap_response_json,
			expires_at, transfer_state, playback_session_id, replacement_claim_id
		FROM cast_bootstraps
		WHERE user_id = ? AND profile_id = ? AND client_instance_id = ? AND replacement_request_id = ?`,
		accountIDForUser(user), viewerProfileID(user), normalizePlaybackClientInstanceID(clientInstanceID), strings.TrimSpace(requestID),
	).Scan(&storedFingerprint, &storedSource, &encoded, &expiresAt, &state, &playbackSessionID, &claimID)
	if errors.Is(err, sql.ErrNoRows) {
		return CastBootstrapResponse{}, false, nil
	}
	if err != nil {
		return CastBootstrapResponse{}, false, err
	}
	if storedFingerprint != fingerprint || storedSource != strings.TrimSpace(sourceSessionID) {
		return CastBootstrapResponse{}, false, errPreparedHandoffConflict
	}
	if state == "pending" && expiresAt <= time.Now().UTC().Format(time.RFC3339) {
		if expireErr := s.expireCastBootstrapTransfer(ctx, user, playbackSessionID, storedSource, requestID, storedFingerprint, claimID); expireErr != nil {
			return CastBootstrapResponse{}, false, expireErr
		}
		state = "expired"
	}
	if storedSource != "" && state != "committed" {
		var sourceActive int
		if activeErr := s.queryUserRow(ctx, `
			SELECT COUNT(*) FROM playback_sessions
			WHERE id = ? AND user_id = ? AND profile_id = ? AND client_instance_id = ?
				AND ended_at = '' AND state NOT IN ('stopped', 'handoff_pending')`,
			storedSource, accountIDForUser(user), viewerProfileID(user), normalizePlaybackClientInstanceID(clientInstanceID),
		).Scan(&sourceActive); activeErr != nil {
			return CastBootstrapResponse{}, false, activeErr
		}
		if sourceActive == 0 {
			if state == "pending" {
				if expireErr := s.expireCastBootstrapTransfer(ctx, user, playbackSessionID, storedSource, requestID, storedFingerprint, claimID); expireErr != nil {
					return CastBootstrapResponse{}, false, expireErr
				}
			}
			return CastBootstrapResponse{}, false, errPlaybackReplacementSourceInactive
		}
	}
	var response CastBootstrapResponse
	if encoded != "" {
		_ = json.Unmarshal([]byte(encoded), &response)
	}
	if response.RequestID == "" {
		response = CastBootstrapResponse{Version: castProtocolVersion, RequestID: requestID, SourceSessionID: storedSource, ReplacementSessionID: playbackSessionID}
	}
	response.TransferStatus = state
	if state != "pending" {
		response.BootstrapEnvelope = ""
	}
	return response, true, nil
}

func (s *Server) expireCastBootstrapTransfer(ctx context.Context, user User, playbackSessionID, sourceSessionID, requestID, fingerprint, claimID string) error {
	if strings.TrimSpace(playbackSessionID) == "" {
		return nil
	}
	lifecycle := s.playbackLifecycle()
	var termination playbackTerminationResult
	err := s.withSecurityFenceTxTagged(ctx, []string{"playback"}, func(tx *sql.Tx) error {
		updated, updateErr := tx.ExecContext(ctx, `
			UPDATE cast_bootstraps
			SET transfer_state = 'expired', bootstrap_response_json = '', playback_envelope = '', receiver_public_key = '', redeem_response_envelope = ''
			WHERE playback_session_id = ? AND replacement_request_id = ? AND replacement_fingerprint = ? AND transfer_state = 'pending'`,
			playbackSessionID, requestID, fingerprint)
		if updateErr != nil {
			return updateErr
		}
		if rowsAffected(updated) != 1 {
			return nil
		}
		var terminateErr error
		termination, terminateErr = lifecycle.terminateTx(ctx, tx, playbackTerminationRequest{
			SessionID: playbackSessionID, UserID: accountIDForUser(user), ProfileID: viewerProfileID(user),
			Cause: playbackTerminationFailedStart, RemoveSession: true,
		})
		if terminateErr != nil && !errors.Is(terminateErr, sql.ErrNoRows) {
			return terminateErr
		}
		if sourceSessionID != "" {
			rolledBack, rollbackErr := tx.ExecContext(ctx, `
				DELETE FROM playback_handoff_receipts
				WHERE source_session_id = ? AND request_id = ? AND request_fingerprint = ?
					AND state = 'committing' AND claim_id = ?`,
				sourceSessionID, requestID, fingerprint, claimID)
			if rollbackErr != nil {
				return rollbackErr
			}
			if rowsAffected(rolledBack) != 1 {
				return errPreparedHandoffConflict
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

func (s *Server) startCastTargetPlayback(r *http.Request, user User, req CastBootstrapRequest, replacementPlan *playbackReplacementPlan) (PlaybackResponse, *playbackStartHTTPError) {
	receiverPlaybackClientInstanceID := req.ClientInstanceID
	switch req.SourceKind {
	case "media":
		create := PlaybackSessionCreateRequest{
			MediaID: req.SourceID, VersionID: req.VersionID, ClientInstanceID: receiverPlaybackClientInstanceID,
			ClientProfile: req.ClientProfile, Intent: req.Intent, SkipPreroll: req.SkipPreroll,
			BurnInSubtitleID: req.BurnInSubtitleID, SubtitleStreamID: req.SubtitleStreamID,
			AudioStreamID: req.AudioStreamID, StartSeconds: req.StartSeconds,
			QueueMediaIDs: req.QueueMediaIDs, RepeatMode: req.RepeatMode, SourceContext: req.SourceContext,
			deferReplacement: replacementPlan != nil, reservedSessionID: replacementSessionID(replacementPlan),
		}
		playback, startErr := s.startPlaybackForRequest(r, user, create)
		return playback, startErr
	case "live":
		return s.startLiveTVPlaybackForRequest(r, user, req.SourceID, req.ClientProfile, req.Intent, receiverPlaybackClientInstanceID, nil, "live-tv", req.SourceID, replacementPlan)
	case "dvr":
		return s.startDVRRecordingPlaybackForRequest(r, user, req.SourceID, DVRPlaybackSessionCreateRequest{
			ClientInstanceID: receiverPlaybackClientInstanceID, ClientProfile: req.ClientProfile, Intent: req.Intent,
			VersionID: req.VersionID, AudioStreamID: req.AudioStreamID, SubtitleStreamID: req.SubtitleStreamID,
			BurnInSubtitleID: req.BurnInSubtitleID, StartSeconds: req.StartSeconds,
			externalReplacement: replacementPlan,
		})
	case "library-channel":
		return s.startLibraryChannelPlaybackByID(r, user, req.SourceID, req.ClientProfile, req.Intent, receiverPlaybackClientInstanceID, replacementPlan)
	default:
		return PlaybackResponse{}, &playbackStartHTTPError{status: http.StatusBadRequest, code: "invalid_cast_source", message: "Cast playback requires a supported source kind."}
	}
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
	if record.RedeemedAt != "" {
		response, replayErr := openCastRedeemResponse(record.RedeemResponseEnvelope, req.BootstrapSecret, record.ID, record.ReceiverID, record.ReceiverOrigin, record.ReceiverChallenge)
		if replayErr != nil {
			writeError(w, http.StatusUnauthorized, "cast_bootstrap_invalid", "The Cast bootstrap is no longer valid.")
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	playback, err := s.openCastPlayback(record.PlaybackEnvelope)
	if err != nil || playback.SessionID != record.PlaybackSessionID {
		_ = s.expireCastBootstrapTransfer(r.Context(), user, record.PlaybackSessionID, record.SourcePlaybackSessionID, record.ReplacementRequestID, record.ReplacementFingerprint, record.ReplacementClaimID)
		writeError(w, http.StatusUnauthorized, "cast_bootstrap_invalid", "The Cast bootstrap is no longer valid.")
		return
	}
	receiverToken := "ptc_cr_" + randomToken()
	now := time.Now().UTC()
	expiresAt := now.Add(castReceiverSessionTTL).Format(time.RFC3339)
	receiverSessionID := randomID("cast_receiver")
	capabilities := parseCastCapabilities(record.CapabilitiesJSON)
	var automation CastPlaybackAutomation
	_ = json.Unmarshal([]byte(record.AutomationJSON), &automation)
	response := CastReceiverSessionResponse{Version: castProtocolVersion, ReceiverSessionToken: receiverToken, ReceiverSessionID: receiverSessionID, PlaybackSessionID: record.PlaybackSessionID, ReceiverID: record.ReceiverID, ServerOrigin: record.ServerOrigin, Generation: record.Generation, Capabilities: capabilities, GrantSemantics: "initial", MediaGrantExpiresAt: playback.MediaGrant.ExpiresAt, Playback: playback, Automation: automation}
	redeemEnvelope, err := sealCastRedeemResponse(response, req.BootstrapSecret, record.ID, record.ReceiverID, record.ReceiverOrigin, record.ReceiverChallenge)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cast_redeem_failed", "Unable to establish the Cast receiver session.")
		return
	}
	err = s.withSecurityFenceTxTagged(r.Context(), []string{"playback"}, func(tx *sql.Tx) error {
		currentAuthorizationRevision, authorizationErr := authorizationRevisionForUserRow(user, tx.QueryRowContext(r.Context(), authorizationRevisionQuery, authorizationRevisionIdentity(user)...))
		if authorizationErr != nil || currentAuthorizationRevision != record.AuthorizationRevision {
			return errPlaybackReplacementAuthorizationChanged
		}
		redeemed, redeemErr := tx.ExecContext(r.Context(), `
			UPDATE cast_bootstraps SET redeemed_at = ?, redeem_response_envelope = ?
			WHERE id = ? AND token_hash = ? AND receiver_id = ? AND receiver_origin = ? AND receiver_challenge = ?
				AND redeemed_at = '' AND expires_at > ? AND transfer_state = 'pending'
				AND authorization_revision = ? AND playback_session_id = ?`,
			now.Format(time.RFC3339), redeemEnvelope, record.ID, hashToken(req.BootstrapSecret), record.ReceiverID, record.ReceiverOrigin,
			record.ReceiverChallenge, now.Format(time.RFC3339), record.AuthorizationRevision, record.PlaybackSessionID)
		if redeemErr != nil || rowsAffected(redeemed) != 1 {
			return castFirstNonNilError(redeemErr, errCastBootstrapInvalid)
		}
		_, err := tx.ExecContext(r.Context(), `
			INSERT INTO cast_receiver_sessions (id, token_hash, user_id, profile_id, receiver_id, receiver_origin, server_origin, playback_session_id, source_playback_session_id, client_instance_id, generation, capabilities_json, automation_json, status, expires_at, last_seen_at, created_at, stopped_at, replacement_request_id, replacement_fingerprint, replacement_claim_id, replacement_terminal_json, authorization_revision, transfer_state)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, '', ?, ?, ?, ?, ?, 'pending')`,
			receiverSessionID, hashToken(receiverToken), user.ID, viewerProfileID(user), record.ReceiverID, record.ReceiverOrigin, record.ServerOrigin,
			record.PlaybackSessionID, record.SourcePlaybackSessionID, record.ClientInstanceID, record.Generation, record.CapabilitiesJSON, record.AutomationJSON, expiresAt, now.Format(time.RFC3339), now.Format(time.RFC3339),
			record.ReplacementRequestID, record.ReplacementFingerprint, record.ReplacementClaimID, record.ReplacementTerminalJSON, record.AuthorizationRevision)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE playback_sessions SET progress_authority = 'receiver', progress_generation = ? WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`, record.Generation, record.PlaybackSessionID, record.UserID, record.ProfileID)
		return err
	})
	if err != nil {
		// A concurrent exact redeem may have committed after our initial read.
		// Recover only its receiver-bound encrypted outcome; no new bearer is
		// created and the bootstrap secret never becomes database-readable.
		if _, replayRecord, replayLookupErr := s.redeemCastBootstrap(r.Context(), req.BootstrapID, req.BootstrapSecret, req.ReceiverID, req.ReceiverChallenge, origin); replayLookupErr == nil && replayRecord.RedeemedAt != "" {
			if replay, replayErr := openCastRedeemResponse(replayRecord.RedeemResponseEnvelope, req.BootstrapSecret, replayRecord.ID, replayRecord.ReceiverID, replayRecord.ReceiverOrigin, replayRecord.ReceiverChallenge); replayErr == nil {
				writeJSON(w, http.StatusOK, replay)
				return
			}
		}
		writeError(w, http.StatusInternalServerError, "cast_redeem_failed", "Unable to establish the Cast receiver session.")
		return
	}
	writeJSON(w, http.StatusOK, response)
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

func (s *Server) handleCastTransferStatus(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var req CastTransferStatusRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.ClientInstanceID = normalizePlaybackClientInstanceID(req.ClientInstanceID)
	req.SourceSessionID = strings.TrimSpace(req.SourceSessionID)
	req.RequestID = strings.TrimSpace(req.RequestID)
	if req.ClientInstanceID == "" || !validPlaybackAuthorityRequestID(req.RequestID) {
		writeError(w, http.StatusBadRequest, "cast_transfer_status_invalid", "clientInstanceId and requestId are required.")
		return
	}
	var sourceSessionID, replacementSessionID, status, authorizationRevision, fingerprint, terminalJSON, claimID string
	err := s.queryUserRow(r.Context(), `
		SELECT source_playback_session_id, playback_session_id, transfer_state, authorization_revision,
			replacement_fingerprint, replacement_terminal_json, replacement_claim_id
		FROM cast_bootstraps
		WHERE user_id = ? AND profile_id = ? AND client_instance_id = ? AND replacement_request_id = ?`,
		accountIDForUser(user), viewerProfileID(user), req.ClientInstanceID, req.RequestID,
	).Scan(&sourceSessionID, &replacementSessionID, &status, &authorizationRevision, &fingerprint, &terminalJSON, &claimID)
	if err != nil || sourceSessionID != req.SourceSessionID {
		writeError(w, http.StatusNotFound, "cast_transfer_not_found", "The Cast transfer outcome was not found.")
		return
	}
	currentRevision, revisionErr := s.authorizationRevisionForUserContextStrict(r.Context(), user)
	if revisionErr != nil || currentRevision != authorizationRevision {
		writeError(w, http.StatusConflict, "cast_transfer_scope_changed", "Playback authorization changed. Reconcile the receiver without replaying transfer credentials.")
		return
	}
	if sourceSessionID != "" && status != "committed" {
		var sourceActive int
		if activeErr := s.queryUserRow(r.Context(), `
			SELECT COUNT(*) FROM playback_sessions
			WHERE id = ? AND user_id = ? AND profile_id = ? AND client_instance_id = ?
				AND ended_at = '' AND state NOT IN ('stopped', 'handoff_pending')`,
			sourceSessionID, accountIDForUser(user), viewerProfileID(user), req.ClientInstanceID,
		).Scan(&sourceActive); activeErr != nil {
			writeError(w, http.StatusInternalServerError, "cast_transfer_status_failed", "Unable to reconcile the Cast transfer outcome.")
			return
		}
		if sourceActive == 0 {
			if status == "pending" {
				if expireErr := s.expireCastBootstrapTransfer(r.Context(), user, replacementSessionID, sourceSessionID, req.RequestID, fingerprint, claimID); expireErr != nil {
					writeError(w, http.StatusInternalServerError, "cast_transfer_status_failed", "Unable to reconcile the inactive Cast transfer source.")
					return
				}
			}
			writeError(w, http.StatusConflict, "replacement_source_inactive", "The source playback authority is no longer active.")
			return
		}
	}
	var previousTerminal *PlaybackTerminalEvent
	if terminalJSON != "" {
		var terminal PlaybackTerminalEvent
		if json.Unmarshal([]byte(terminalJSON), &terminal) == nil {
			previousTerminal = &terminal
		}
	}
	writeJSON(w, http.StatusOK, CastTransferStatusResponse{
		Version: castProtocolVersion, SourceSessionID: sourceSessionID, RequestID: req.RequestID,
		ReplacementSessionID: replacementSessionID, Status: status, RequestFingerprint: fingerprint, PreviousTerminal: previousTerminal,
	})
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
	if auth.record.Status == "pending" && action != "progress" && action != "state" {
		writeError(w, http.StatusConflict, "cast_receiver_not_ready", "The receiver must prove first-playing before this operation is allowed.")
		return
	}
	if auth.record.Status != "active" && auth.record.Status != "pending" && action != "stop" && action != "advance" {
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
		var req CastAdvanceRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		requestFingerprint := castAdvanceRequestFingerprint(req)
		if req.AdvanceID == auth.record.LastAdvanceID && auth.record.LastAdvanceJSON != "" {
			var previous CastAdvanceResponse
			if json.Unmarshal([]byte(auth.record.LastAdvanceJSON), &previous) == nil && auth.record.LastAdvanceRequestFingerprint == requestFingerprint {
				writeJSON(w, http.StatusOK, previous)
				return
			}
			writeError(w, http.StatusConflict, "cast_advance_retry_conflict", "The Cast advance retry does not match the original request body.")
			return
		}
		if auth.record.Status != "active" {
			writeError(w, http.StatusConflict, "cast_session_stopped", "The Cast session is no longer active.")
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
		if !validPlaybackAuthorityRequestID(req.RequestID) || validatePlaybackTerminalEvent(req.PreviousTerminal) != nil {
			writeError(w, http.StatusBadRequest, "cast_advance_terminal_required", "Cast advance requires a stable requestId and the receiver actor's full ordered previousTerminal event.")
			return
		}
		response, err := s.advanceCastReceiver(r.Context(), auth, req)
		if err != nil {
			if errors.Is(err, errPlaybackTerminalDurationMismatch) {
				writeError(w, http.StatusConflict, "playback_terminal_duration_mismatch", "Completed playback duration does not match the server-authoritative media duration.")
				return
			}
			writeError(w, http.StatusConflict, "cast_advance_failed", "The receiver-owned atomic queue advance could not be prepared.")
			return
		}
		writeJSON(w, http.StatusOK, response)
	case "advance-cancel":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		var req CastAdvanceCancelRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Generation != auth.record.Generation || strings.TrimSpace(req.RequestID) != auth.record.PendingRequestID {
			writeError(w, http.StatusConflict, "cast_advance_cancel_conflict", "The cancellation does not match the pending receiver advance.")
			return
		}
		if err := s.cancelCastReceiverAdvance(r.Context(), auth); err != nil {
			writeError(w, http.StatusConflict, "cast_advance_cancel_failed", "The pending receiver advance could not be cancelled safely.")
			return
		}
		var previousTerminal PlaybackTerminalEvent
		_ = json.Unmarshal([]byte(auth.record.PendingTerminalJSON), &previousTerminal)
		writeJSON(w, http.StatusOK, CastTransferStatusResponse{
			Version: castProtocolVersion, SourceSessionID: auth.record.PlaybackSessionID,
			RequestID: auth.record.PendingRequestID, RequestFingerprint: auth.record.PendingFingerprint,
			ReplacementSessionID: auth.record.PendingPlaybackSessionID, PreviousTerminal: &previousTerminal, Status: "failed",
		})
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
		if auth.record.Status != "active" && auth.record.Status != "pending" {
			writeError(w, http.StatusConflict, "cast_session_stopped", "The Cast session is no longer active.")
			return
		}
		var req CastProgressRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if strings.EqualFold(strings.TrimSpace(req.State), "completed") {
			writeError(w, http.StatusConflict, "cast_terminal_required", "Completed playback must be submitted through an exact receiver terminal or atomic advance request.")
			return
		}
		pendingAdvanceReady := auth.record.PendingPlaybackSessionID != "" && req.Generation == auth.record.PendingGeneration
		if req.Generation != auth.record.Generation && !pendingAdvanceReady {
			writeError(w, http.StatusConflict, "cast_generation_stale", "The Cast session generation is stale.")
			return
		}
		if !castOperationAllowed(auth.record.CapabilitiesJSON, "progress") {
			writeError(w, http.StatusForbidden, "cast_operation_denied", "This Cast session does not allow progress.")
			return
		}
		req.PlaybackProgressEvent.Authority = "receiver"
		req.PlaybackProgressEvent.Generation = req.Generation
		if pendingAdvanceReady {
			if !strings.EqualFold(strings.TrimSpace(req.State), "playing") {
				writeError(w, http.StatusConflict, "cast_receiver_not_ready", "The first successor progress event must prove playing before queue authority can advance.")
				return
			}
			if commitErr := s.commitCastReceiverAdvanceReady(r.Context(), auth); commitErr != nil {
				writeError(w, http.StatusConflict, "cast_advance_commit_pending", "The receiver-ready queue advance did not commit; retry the exact playing event.")
				return
			}
			auth.record.PlaybackSessionID = auth.record.PendingPlaybackSessionID
			auth.record.Generation = auth.record.PendingGeneration
		} else if auth.record.TransferState == "pending" {
			if !strings.EqualFold(strings.TrimSpace(req.State), "playing") {
				writeError(w, http.StatusConflict, "cast_receiver_not_ready", "The first receiver progress event must prove playing before playback authority can be activated.")
				return
			}
			if commitErr := s.commitCastReceiverReady(r.Context(), auth); commitErr != nil {
				writeError(w, http.StatusConflict, "cast_handoff_commit_pending", "The receiver-ready authority commit did not complete; retry the exact playing event.")
				return
			}
			auth.record.TransferState = "committed"
			auth.record.SourcePlaybackSessionID = ""
		}
		ack, err := s.touchPlaybackSession(auth.user, auth.record.PlaybackSessionID, req.PlaybackProgressEvent)
		if err != nil {
			writeError(w, http.StatusConflict, "cast_progress_failed", "The Cast progress event could not be applied.")
			return
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
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
			return
		}
		if !auth.viaReceiver && auth.record.Status != "active" {
			writeError(w, http.StatusConflict, "cast_session_stopped", "The Cast session is no longer active.")
			return
		}
		var req CastStopRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if auth.record.PendingPlaybackSessionID != "" {
			writeError(w, http.StatusConflict, "cast_advance_cancel_required", "Cancel the exact pending Cast advance before stopping the source receiver.")
			return
		}
		if req.Generation != auth.record.Generation {
			writeError(w, http.StatusConflict, "cast_generation_stale", "The Cast session generation is stale.")
			return
		}
		if !castOperationAllowed(auth.record.CapabilitiesJSON, "stop") {
			writeError(w, http.StatusForbidden, "cast_operation_denied", "This Cast session does not allow stop.")
			return
		}
		terminalReq, terminalErr := normalizePlaybackSessionStopRequest(PlaybackSessionStopRequest{RequestID: strings.TrimSpace(req.RequestID), Terminal: req.Terminal})
		if terminalErr != nil {
			writePlaybackStartError(w, terminalErr)
			return
		}
		ack, err := s.stopCastReceiverSession(r.Context(), auth, terminalReq)
		if err != nil {
			if errors.Is(err, errPlaybackTerminalReceiptConflict) {
				writeError(w, http.StatusConflict, "playback_terminal_request_conflict", "The stop request does not match the accepted terminal receipt.")
				return
			}
			if errors.Is(err, errPlaybackTerminalAuthorizationChanged) {
				writeError(w, http.StatusConflict, "playback_terminal_scope_changed", "Playback authorization changed. Reconcile the receiver session before retrying stop.")
				return
			}
			if errors.Is(err, errPlaybackTerminalDurationMismatch) {
				writeError(w, http.StatusConflict, "playback_terminal_duration_mismatch", "Completed playback duration does not match the server-authoritative media duration.")
				return
			}
			writeError(w, http.StatusConflict, "cast_stop_failed", "The Cast session could not be stopped.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "generation": auth.record.Generation, "terminal": ack})
	default:
		writeError(w, http.StatusNotFound, "not_found", "Cast receiver operation was not found.")
	}
}

func castQueueNext(snapshot playbackSessionQueueSnapshot) (playbackQueueOccurrence, []playbackQueueOccurrence, bool) {
	if snapshot.RepeatMode == "one" {
		return snapshot.Current, append([]playbackQueueOccurrence(nil), snapshot.Queue...), snapshot.Current.EntryID != ""
	}
	if len(snapshot.Queue) > 0 {
		return snapshot.Queue[0], append([]playbackQueueOccurrence(nil), snapshot.Queue[1:]...), true
	}
	if snapshot.RepeatMode != "all" || len(snapshot.History) == 0 {
		return playbackQueueOccurrence{}, nil, false
	}
	ordered := make([]playbackQueueOccurrence, 0, len(snapshot.History)+1)
	seen := map[string]bool{}
	for index := len(snapshot.History) - 1; index >= 0; index-- {
		occurrence := snapshot.History[index]
		if seen[occurrence.EntryID] {
			continue
		}
		seen[occurrence.EntryID] = true
		occurrence.HistoryID = ""
		ordered = append(ordered, occurrence)
	}
	if !seen[snapshot.Current.EntryID] {
		ordered = append(ordered, snapshot.Current)
	}
	if len(ordered) == 0 {
		return playbackQueueOccurrence{}, nil, false
	}
	return ordered[0], append([]playbackQueueOccurrence(nil), ordered[1:]...), true
}

func (s *Server) advanceCastReceiver(ctx context.Context, auth castSessionAuth, req CastAdvanceRequest) (CastAdvanceResponse, error) {
	requestID := strings.TrimSpace(req.RequestID)
	requestFingerprint := castAdvanceRequestFingerprint(req)
	terminalReq, terminalErr := normalizePlaybackSessionStopRequest(PlaybackSessionStopRequest{RequestID: requestID, Terminal: req.PreviousTerminal})
	if terminalErr != nil {
		return CastAdvanceResponse{}, terminalErr
	}
	var automation CastPlaybackAutomation
	_ = json.Unmarshal([]byte(auth.record.AutomationJSON), &automation)
	count := auth.record.AutomaticAdvances
	if req.Automatic {
		limit := max(1, automation.PassoutAfterEpisodes)
		if automation.PassoutProtection && count >= limit {
			outcome := CastAdvanceResponse{Version: castProtocolVersion, Status: "passout_stopped", RequestID: requestID, RequestFingerprint: requestFingerprint, PreviousTerminal: req.PreviousTerminal, SourceSessionID: auth.record.PlaybackSessionID, Generation: auth.record.Generation, AutomaticAdvances: count, Automation: automation}
			if _, err := s.stopCastReceiverSessionWithAdvanceOutcome(ctx, auth, terminalReq, req.AdvanceID, requestFingerprint, &outcome); err != nil {
				return CastAdvanceResponse{}, err
			}
			return outcome, nil
		}
		count++
	} else {
		count = 0
	}
	snapshot, err := s.playbackSessionQueueSnapshot(ctx, auth.user, auth.record.PlaybackSessionID)
	if err != nil {
		return CastAdvanceResponse{}, err
	}
	next, remaining, ok := castQueueNext(snapshot)
	if !ok {
		outcome := CastAdvanceResponse{Version: castProtocolVersion, Status: "exhausted", RequestID: requestID, RequestFingerprint: requestFingerprint, PreviousTerminal: req.PreviousTerminal, SourceSessionID: auth.record.PlaybackSessionID, Generation: auth.record.Generation, AutomaticAdvances: count, Automation: automation}
		if _, err := s.stopCastReceiverSessionWithAdvanceOutcome(ctx, auth, terminalReq, req.AdvanceID, requestFingerprint, &outcome); err != nil {
			return CastAdvanceResponse{}, err
		}
		return outcome, nil
	}
	var playbackRevision int64
	if err := s.queryUserRow(ctx, `SELECT renegotiation_revision FROM playback_sessions WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = ''`, auth.record.PlaybackSessionID, auth.record.UserID, auth.record.ProfileID).Scan(&playbackRevision); err != nil {
		return CastAdvanceResponse{}, err
	}
	replacement := &PlaybackReplacementRequest{
		SourceSessionID: auth.record.PlaybackSessionID, RequestID: requestID, PreviousTerminal: req.PreviousTerminal,
		ExpectedQueueRevision: &snapshot.Revision, ExpectedPlaybackRevision: &playbackRevision,
	}
	targetRequest := struct {
		Advance CastAdvanceRequest      `json:"advance"`
		Next    playbackQueueOccurrence `json:"next"`
	}{Advance: req, Next: next}
	plan, startErr := s.preparePlaybackReplacement(ctx, auth.user, auth.record.ClientInstanceID, "cast-advance", next.EntryID, targetRequest, replacement)
	if startErr != nil {
		return CastAdvanceResponse{}, startErr
	}
	if plan.Committed != nil {
		return CastAdvanceResponse{Version: castProtocolVersion, Status: "committed", RequestID: requestID, RequestFingerprint: plan.Fingerprint, PreviousTerminal: req.PreviousTerminal, SourceSessionID: auth.record.PlaybackSessionID, ReplacementSessionID: plan.Committed.SessionID, Generation: auth.record.Generation + 1, AutomaticAdvances: count, Automation: automation, Playback: plan.Committed}, nil
	}
	releasePlan := plan.Active
	defer func() {
		if releasePlan {
			s.rollbackPlaybackReplacement(plan)
		}
	}()
	profile := PlaybackClientProfile{Device: "cast-receiver", Platform: "web", SupportsHLS: true, SupportsMSE: true}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://cast.invalid/advance", nil)
	playback, playbackErr := s.startPlaybackForRequest(request, auth.user, PlaybackSessionCreateRequest{
		MediaID: next.MediaID, ClientInstanceID: auth.record.ClientInstanceID, ClientProfile: profile, SkipPreroll: true,
		QueueMediaIDs: playbackQueueOccurrenceMediaIDs(remaining), RepeatMode: snapshot.RepeatMode, SourceContext: snapshot.SourceContext,
		currentEntryID: next.EntryID, queueOccurrences: remaining,
		historyOccurrences: append([]playbackQueueOccurrence{snapshot.Current}, snapshot.History...), queueOwned: true,
		deferReplacement: true, reservedSessionID: plan.Claim.ReplacementSessionID,
	})
	if playbackErr != nil {
		return CastAdvanceResponse{}, playbackErr
	}
	// The receiver bearer remains the sole target actor; a continuation bearer
	// would create a parallel terminal/progress path before first-playing.
	playback.ContinuationCredential = nil
	newGeneration := auth.record.Generation + 1
	playback.Generation = int(newGeneration)
	response := CastAdvanceResponse{Version: castProtocolVersion, Status: "prepared", RequestID: requestID, RequestFingerprint: plan.Fingerprint, PreviousTerminal: req.PreviousTerminal, SourceSessionID: auth.record.PlaybackSessionID, ReplacementSessionID: playback.SessionID, Generation: newGeneration, AutomaticAdvances: count, Automation: automation, Playback: &playback}
	encoded, _ := json.Marshal(response)
	terminalJSON := stringJSON(req.PreviousTerminal)
	err = s.withSecurityFenceTxTagged(ctx, []string{"playback"}, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE playback_sessions SET progress_authority = 'receiver', progress_generation = ? WHERE id = ? AND user_id = ? AND profile_id = ? AND ended_at = '' AND state = 'handoff_pending'`, newGeneration, playback.SessionID, auth.record.UserID, auth.record.ProfileID); err != nil {
			return err
		}
		updated, err := tx.ExecContext(ctx, `
			UPDATE cast_receiver_sessions SET
				pending_playback_session_id = ?, pending_generation = ?, pending_request_id = ?, pending_fingerprint = ?,
				pending_claim_id = ?, pending_terminal_json = ?, pending_authorization_revision = ?,
				pending_advance_id = ?, pending_advance_json = ?, pending_payload_expires_at = ?, pending_expires_at = ?,
				last_advance_id = ?, last_advance_request_fingerprint = ?, last_advance_json = ?, last_advance_payload_expires_at = ?
			WHERE id = ? AND playback_session_id = ? AND generation = ? AND status = 'active'
				AND transfer_state = 'committed' AND pending_playback_session_id = ''`,
			playback.SessionID, newGeneration, requestID, plan.Fingerprint, plan.Claim.ID, terminalJSON, plan.Claim.AuthorizationRevision,
			req.AdvanceID, string(encoded), time.Now().UTC().Add(playbackHandoffReceiptTTL).Format(time.RFC3339Nano), time.Now().UTC().Add(castBootstrapTTL).Format(time.RFC3339),
			req.AdvanceID, requestFingerprint, string(encoded), time.Now().UTC().Add(playbackHandoffReceiptTTL).Format(time.RFC3339Nano), auth.record.ID, auth.record.PlaybackSessionID, auth.record.Generation)
		if err != nil || rowsAffected(updated) != 1 {
			return castFirstNonNilError(err, errCastGenerationStale)
		}
		return nil
	})
	if err != nil {
		s.discardFailedCastPlayback(ctx, auth.user, playback.SessionID)
		return CastAdvanceResponse{}, err
	}
	releasePlan = false
	return response, nil
}

func (s *Server) cancelCastReceiverAdvance(ctx context.Context, auth castSessionAuth) error {
	if auth.record.PendingPlaybackSessionID == "" {
		return errPreparedHandoffConflict
	}
	failed := CastAdvanceResponse{
		Version: castProtocolVersion, Status: "failed", RequestID: auth.record.PendingRequestID,
		SourceSessionID: auth.record.PlaybackSessionID, ReplacementSessionID: auth.record.PendingPlaybackSessionID,
		Generation: auth.record.Generation, AutomaticAdvances: auth.record.AutomaticAdvances,
	}
	var prepared CastAdvanceResponse
	if json.Unmarshal([]byte(auth.record.PendingAdvanceJSON), &prepared) == nil {
		failed.RequestFingerprint = prepared.RequestFingerprint
		failed.PreviousTerminal = prepared.PreviousTerminal
		failed.Automation = prepared.Automation
	}
	encoded, _ := json.Marshal(failed)
	lifecycle := s.playbackLifecycle()
	var termination playbackTerminationResult
	err := s.withSecurityFenceTxTagged(ctx, []string{"playback"}, func(tx *sql.Tx) error {
		updated, err := tx.ExecContext(ctx, `
			UPDATE cast_receiver_sessions SET
				pending_playback_session_id = '', pending_generation = 0, pending_request_id = '', pending_fingerprint = '',
				pending_claim_id = '', pending_terminal_json = '', pending_authorization_revision = '', pending_advance_id = '',
				pending_advance_json = '', pending_payload_expires_at = '', pending_expires_at = '',
				last_advance_json = ?, last_advance_payload_expires_at = ?
			WHERE id = ? AND playback_session_id = ? AND generation = ? AND pending_playback_session_id = ?
				AND pending_request_id = ? AND pending_fingerprint = ? AND pending_claim_id = ?`,
			string(encoded), time.Now().UTC().Add(playbackHandoffReceiptTTL).Format(time.RFC3339Nano),
			auth.record.ID, auth.record.PlaybackSessionID, auth.record.Generation, auth.record.PendingPlaybackSessionID,
			auth.record.PendingRequestID, auth.record.PendingFingerprint, auth.record.PendingClaimID)
		if err != nil || rowsAffected(updated) != 1 {
			return castFirstNonNilError(err, errPreparedHandoffConflict)
		}
		var terminateErr error
		termination, terminateErr = lifecycle.terminateTx(ctx, tx, playbackTerminationRequest{
			SessionID: auth.record.PendingPlaybackSessionID, UserID: auth.record.UserID, ProfileID: auth.record.ProfileID,
			Cause: playbackTerminationFailedStart, RemoveSession: true,
		})
		if terminateErr != nil && !errors.Is(terminateErr, sql.ErrNoRows) {
			return terminateErr
		}
		rolledBack, rollbackErr := tx.ExecContext(ctx, `
			DELETE FROM playback_handoff_receipts
			WHERE source_session_id = ? AND request_id = ? AND request_fingerprint = ?
				AND state = 'committing' AND claim_id = ?`,
			auth.record.PlaybackSessionID, auth.record.PendingRequestID, auth.record.PendingFingerprint, auth.record.PendingClaimID)
		if rollbackErr != nil || rowsAffected(rolledBack) != 1 {
			return castFirstNonNilError(rollbackErr, errPreparedHandoffConflict)
		}
		return nil
	})
	if err != nil {
		return err
	}
	lifecycle.afterCommit(ctx, termination)
	return nil
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
	if err := s.queryUserRow(ctx, `SELECT id, user_id, profile_id, receiver_id, receiver_origin, receiver_public_key, receiver_challenge, server_origin, playback_session_id, source_playback_session_id, client_instance_id, generation, capabilities_json, automation_json, playback_envelope, expires_at, redeemed_at, replacement_request_id, replacement_fingerprint, replacement_claim_id, replacement_terminal_json, authorization_revision, bootstrap_response_json, redeem_response_envelope, transfer_state, payload_expires_at FROM cast_bootstraps WHERE id = ? AND token_hash = ?`, bootstrapID, hashToken(secret)).Scan(&record.ID, &record.UserID, &record.ProfileID, &record.ReceiverID, &record.ReceiverOrigin, &record.ReceiverPublicKey, &record.ReceiverChallenge, &record.ServerOrigin, &record.PlaybackSessionID, &record.SourcePlaybackSessionID, &record.ClientInstanceID, &record.Generation, &record.CapabilitiesJSON, &record.AutomationJSON, &record.PlaybackEnvelope, &record.ExpiresAt, &record.RedeemedAt, &record.ReplacementRequestID, &record.ReplacementFingerprint, &record.ReplacementClaimID, &record.ReplacementTerminalJSON, &record.AuthorizationRevision, &record.BootstrapResponseJSON, &record.RedeemResponseEnvelope, &record.TransferState, &record.PayloadExpiresAt); err != nil {
		return User{}, castBootstrapRecord{}, errCastBootstrapInvalid
	}
	if record.ReceiverID != receiverID || record.ReceiverOrigin != origin || record.ReceiverChallenge != challenge || record.ExpiresAt <= now || record.TransferState != "pending" || (record.RedeemedAt != "" && record.RedeemResponseEnvelope == "") {
		return User{}, castBootstrapRecord{}, errCastBootstrapInvalid
	}
	user, err := s.castUserForScope(ctx, record.UserID, record.ProfileID)
	if err != nil || !user.Permissions["playMedia"] {
		return User{}, castBootstrapRecord{}, errCastBootstrapInvalid
	}
	currentRevision, revisionErr := s.authorizationRevisionForUserContextStrict(ctx, user)
	if revisionErr != nil || currentRevision != record.AuthorizationRevision {
		return User{}, castBootstrapRecord{}, errCastBootstrapInvalid
	}
	if playback, playbackErr := s.openCastPlayback(record.PlaybackEnvelope); playbackErr != nil || playback.SessionID != record.PlaybackSessionID {
		return User{}, castBootstrapRecord{}, errCastBootstrapInvalid
	}
	return user, record, nil
}

func (s *Server) authenticateCastSession(r *http.Request, sessionID string) (castSessionAuth, error) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), castReceiverAuthorization))
	if !strings.HasPrefix(strings.TrimSpace(r.Header.Get("Authorization")), castReceiverAuthorization) || !strings.HasPrefix(token, "ptc_cr_") {
		return castSessionAuth{}, errCastReceiverSessionInvalid
	}
	var record castReceiverRecord
	err := s.queryUserRow(r.Context(), `SELECT id, user_id, profile_id, receiver_id, receiver_origin, server_origin, playback_session_id, source_playback_session_id, client_instance_id, generation, capabilities_json, automation_json, status, expires_at, last_seen_at, last_command_id, last_command_json, automatic_advances, last_advance_id, last_advance_request_fingerprint, last_advance_json, replacement_request_id, replacement_fingerprint, replacement_claim_id, replacement_terminal_json, authorization_revision, transfer_state, pending_playback_session_id, pending_generation, pending_request_id, pending_fingerprint, pending_claim_id, pending_terminal_json, pending_authorization_revision, pending_advance_id, pending_advance_json, pending_payload_expires_at, pending_expires_at FROM cast_receiver_sessions WHERE id = ? AND token_hash = ?`, sessionID, hashToken(token)).Scan(&record.ID, &record.UserID, &record.ProfileID, &record.ReceiverID, &record.ReceiverOrigin, &record.ServerOrigin, &record.PlaybackSessionID, &record.SourcePlaybackSessionID, &record.ClientInstanceID, &record.Generation, &record.CapabilitiesJSON, &record.AutomationJSON, &record.Status, &record.ExpiresAt, &record.LastSeenAt, &record.LastCommandID, &record.LastCommandJSON, &record.AutomaticAdvances, &record.LastAdvanceID, &record.LastAdvanceRequestFingerprint, &record.LastAdvanceJSON, &record.ReplacementRequestID, &record.ReplacementFingerprint, &record.ReplacementClaimID, &record.ReplacementTerminalJSON, &record.AuthorizationRevision, &record.TransferState, &record.PendingPlaybackSessionID, &record.PendingGeneration, &record.PendingRequestID, &record.PendingFingerprint, &record.PendingClaimID, &record.PendingTerminalJSON, &record.PendingAuthorizationRevision, &record.PendingAdvanceID, &record.PendingAdvanceJSON, &record.PendingPayloadExpiresAt, &record.PendingExpiresAt)
	if err != nil || (record.Status != "pending" && record.Status != "active" && record.Status != "stopped") || record.ExpiresAt <= time.Now().UTC().Format(time.RFC3339) {
		return castSessionAuth{}, errCastReceiverSessionInvalid
	}
	user, err := s.castUserForScope(r.Context(), record.UserID, record.ProfileID)
	if err != nil || !user.Permissions["playMedia"] {
		return castSessionAuth{}, errCastReceiverSessionInvalid
	}
	currentRevision, revisionErr := s.authorizationRevisionForUserContextStrict(r.Context(), user)
	if revisionErr != nil || currentRevision != record.AuthorizationRevision {
		return castSessionAuth{}, errCastReceiverSessionInvalid
	}
	now := time.Now().UTC()
	_, _ = s.execPlaybackWrite(r.Context(), `UPDATE cast_receiver_sessions SET last_seen_at = ?, expires_at = MAX(expires_at, ?) WHERE id = ? AND status = 'active'`, now.Format(time.RFC3339), now.Add(castReceiverSessionTTL).Format(time.RFC3339), record.ID)
	return castSessionAuth{record: record, user: user, viaReceiver: true}, nil
}

func (s *Server) castReceiverRecordForUser(ctx context.Context, user User, sessionID, clientInstanceID string) (castReceiverRecord, error) {
	var record castReceiverRecord
	where := `id = ? AND user_id = ? AND profile_id = ? AND status = 'active' AND expires_at > ?`
	args := []any{strings.TrimSpace(sessionID), accountIDForUser(user), viewerProfileID(user), time.Now().UTC().Format(time.RFC3339)}
	if strings.TrimSpace(sessionID) == "" {
		where = `client_instance_id = ? AND user_id = ? AND profile_id = ? AND status = 'active' AND expires_at > ?`
		args[0] = normalizePlaybackClientInstanceID(clientInstanceID)
	}
	err := s.queryUserRow(ctx, `SELECT id, user_id, profile_id, receiver_id, receiver_origin, server_origin, playback_session_id, source_playback_session_id, client_instance_id, generation, capabilities_json, automation_json, status, expires_at, last_seen_at, last_command_id, last_command_json, automatic_advances, last_advance_id, last_advance_request_fingerprint, last_advance_json, replacement_request_id, replacement_fingerprint, replacement_claim_id, replacement_terminal_json, authorization_revision, transfer_state FROM cast_receiver_sessions WHERE `+where+` ORDER BY generation DESC LIMIT 1`, args...).Scan(&record.ID, &record.UserID, &record.ProfileID, &record.ReceiverID, &record.ReceiverOrigin, &record.ServerOrigin, &record.PlaybackSessionID, &record.SourcePlaybackSessionID, &record.ClientInstanceID, &record.Generation, &record.CapabilitiesJSON, &record.AutomationJSON, &record.Status, &record.ExpiresAt, &record.LastSeenAt, &record.LastCommandID, &record.LastCommandJSON, &record.AutomaticAdvances, &record.LastAdvanceID, &record.LastAdvanceRequestFingerprint, &record.LastAdvanceJSON, &record.ReplacementRequestID, &record.ReplacementFingerprint, &record.ReplacementClaimID, &record.ReplacementTerminalJSON, &record.AuthorizationRevision, &record.TransferState)
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
	err := s.queryUserRow(ctx, `SELECT state, position_seconds, media_id, current_entry_id FROM playback_sessions WHERE id = ? AND user_id = ? AND profile_id = ?`, record.PlaybackSessionID, record.UserID, record.ProfileID).Scan(&state.PlaybackState, &state.PositionSeconds, &state.MediaID, &state.CurrentQueueEntryID)
	if err != nil {
		return CastReceiverSessionState{}, err
	}
	state.Version, state.ReceiverSessionID, state.PlaybackSessionID = castProtocolVersion, record.ID, record.PlaybackSessionID
	state.ReceiverID, state.ServerOrigin, state.Generation, state.Status = record.ReceiverID, record.ServerOrigin, record.Generation, record.Status
	state.Capabilities, state.LastSeenAt, state.ExpiresAt = parseCastCapabilities(record.CapabilitiesJSON), record.LastSeenAt, record.ExpiresAt
	_ = json.Unmarshal([]byte(record.AutomationJSON), &state.Automation)
	state.AutomaticAdvances = record.AutomaticAdvances
	state.Queue, _ = s.playbackQueueEntriesForOccurrences(ctx, record.ProfileID, s.loadPlaybackSessionQueue(record.PlaybackSessionID))
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

func (s *Server) commitCastReceiverReady(ctx context.Context, auth castSessionAuth) error {
	if auth.record.TransferState == "committed" {
		return nil
	}
	if auth.record.TransferState != "pending" || auth.record.ReplacementRequestID == "" {
		return errPreparedHandoffConflict
	}
	currentRevision, revisionErr := s.authorizationRevisionForUserContextStrict(ctx, auth.user)
	if revisionErr != nil || currentRevision != auth.record.AuthorizationRevision {
		return errPlaybackReplacementAuthorizationChanged
	}
	if auth.record.SourcePlaybackSessionID == "" {
		return s.withSecurityFenceTxTagged(ctx, []string{"playback"}, func(tx *sql.Tx) error {
			currentAuthorizationRevision, authorizationErr := authorizationRevisionForUserRow(auth.user, tx.QueryRowContext(ctx, authorizationRevisionQuery, authorizationRevisionIdentity(auth.user)...))
			if authorizationErr != nil || currentAuthorizationRevision != auth.record.AuthorizationRevision {
				return errPlaybackReplacementAuthorizationChanged
			}
			activated, err := tx.ExecContext(ctx, `
				UPDATE playback_sessions SET state = 'playing'
				WHERE id = ? AND user_id = ? AND profile_id = ? AND client_instance_id = ?
					AND ended_at = '' AND state = 'handoff_pending'`,
				auth.record.PlaybackSessionID, auth.record.UserID, auth.record.ProfileID, auth.record.ClientInstanceID)
			if err != nil || rowsAffected(activated) != 1 {
				return castFirstNonNilError(err, errPreparedHandoffConflict)
			}
			updated, err := tx.ExecContext(ctx, `
				UPDATE cast_receiver_sessions SET transfer_state = 'committed', status = 'active'
				WHERE id = ? AND playback_session_id = ? AND generation = ? AND client_instance_id = ?
					AND replacement_request_id = ? AND replacement_fingerprint = ?
					AND authorization_revision = ? AND transfer_state = 'pending' AND status = 'pending'`,
				auth.record.ID, auth.record.PlaybackSessionID, auth.record.Generation, auth.record.ClientInstanceID,
				auth.record.ReplacementRequestID, auth.record.ReplacementFingerprint, auth.record.AuthorizationRevision)
			if err != nil || rowsAffected(updated) != 1 {
				return castFirstNonNilError(err, errPreparedHandoffConflict)
			}
			updated, err = tx.ExecContext(ctx, `
				UPDATE cast_bootstraps SET transfer_state = 'committed', redeem_response_envelope = ''
				WHERE playback_session_id = ? AND user_id = ? AND profile_id = ? AND client_instance_id = ?
					AND replacement_request_id = ? AND replacement_fingerprint = ?
					AND authorization_revision = ? AND transfer_state = 'pending'`,
				auth.record.PlaybackSessionID, auth.record.UserID, auth.record.ProfileID, auth.record.ClientInstanceID,
				auth.record.ReplacementRequestID, auth.record.ReplacementFingerprint, auth.record.AuthorizationRevision)
			if err != nil || rowsAffected(updated) != 1 {
				return castFirstNonNilError(err, errPreparedHandoffConflict)
			}
			return nil
		})
	}

	var claim playbackHandoffClaim
	var receiptState string
	err := s.queryUserRow(ctx, `
		SELECT claim_id, replacement_session_id, authorization_revision, client_instance_id,
			expected_queue_revision, expected_playback_revision, state
		FROM playback_handoff_receipts
		WHERE source_session_id = ? AND user_id = ? AND profile_id = ?
			AND request_id = ? AND request_fingerprint = ?`,
		auth.record.SourcePlaybackSessionID, auth.record.UserID, auth.record.ProfileID,
		auth.record.ReplacementRequestID, auth.record.ReplacementFingerprint,
	).Scan(&claim.ID, &claim.ReplacementSessionID, &claim.AuthorizationRevision, &claim.ClientInstanceID, &claim.ExpectedQueueRevision, &claim.ExpectedPlaybackRevision, &receiptState)
	if err != nil || claim.ID != auth.record.ReplacementClaimID || claim.ReplacementSessionID != auth.record.PlaybackSessionID {
		return castFirstNonNilError(err, errPreparedHandoffConflict)
	}
	if receiptState == "committed" {
		// Receiver/bootstrap publication is part of the same transaction as the
		// receipt. A pending receiver paired with a committed receipt is corrupt,
		// not a second-stage transition that may be guessed here.
		return errPreparedHandoffConflict
	}
	if receiptState != "committed" {
		var terminal PlaybackTerminalEvent
		if json.Unmarshal([]byte(auth.record.ReplacementTerminalJSON), &terminal) != nil {
			return errPlaybackTerminalEvidenceInvalid
		}
		var envelope string
		var playback PlaybackResponse
		if err := s.queryUserRow(ctx, `SELECT playback_envelope FROM cast_bootstraps WHERE playback_session_id = ? AND replacement_request_id = ? AND transfer_state = 'pending'`, auth.record.PlaybackSessionID, auth.record.ReplacementRequestID).Scan(&envelope); err == nil {
			opened, openErr := s.openCastPlayback(envelope)
			if openErr != nil {
				return errCastBootstrapInvalid
			}
			playback = opened
		} else {
			var advance CastAdvanceResponse
			if json.Unmarshal([]byte(auth.record.LastAdvanceJSON), &advance) != nil || advance.Playback == nil {
				return errPreparedHandoffConflict
			}
			playback = *advance.Playback
		}
		if playback.SessionID != auth.record.PlaybackSessionID {
			return errCastBootstrapInvalid
		}
		if err := s.commitDirectPlaybackHandoffWithTx(ctx, auth.user, auth.record.SourcePlaybackSessionID, terminal, auth.record.ReplacementRequestID, auth.record.ReplacementFingerprint, claim, playback, func(tx *sql.Tx) error {
			updated, updateErr := tx.ExecContext(ctx, `
				UPDATE cast_receiver_sessions
				SET source_playback_session_id = '', replacement_terminal_json = '', transfer_state = 'committed', status = 'active'
				WHERE id = ? AND playback_session_id = ? AND generation = ? AND client_instance_id = ?
					AND replacement_request_id = ? AND replacement_fingerprint = ? AND replacement_claim_id = ?
					AND authorization_revision = ? AND transfer_state = 'pending' AND status = 'pending'`,
				auth.record.ID, auth.record.PlaybackSessionID, auth.record.Generation, auth.record.ClientInstanceID,
				auth.record.ReplacementRequestID, auth.record.ReplacementFingerprint, auth.record.ReplacementClaimID,
				auth.record.AuthorizationRevision)
			if updateErr != nil || rowsAffected(updated) != 1 {
				return castFirstNonNilError(updateErr, errPreparedHandoffConflict)
			}
			updated, updateErr = tx.ExecContext(ctx, `
				UPDATE cast_bootstraps SET transfer_state = 'committed', redeem_response_envelope = ''
				WHERE playback_session_id = ? AND user_id = ? AND profile_id = ? AND client_instance_id = ?
					AND replacement_request_id = ? AND replacement_fingerprint = ? AND replacement_claim_id = ?
					AND authorization_revision = ? AND transfer_state = 'pending'`,
				auth.record.PlaybackSessionID, auth.record.UserID, auth.record.ProfileID, auth.record.ClientInstanceID,
				auth.record.ReplacementRequestID, auth.record.ReplacementFingerprint, auth.record.ReplacementClaimID,
				auth.record.AuthorizationRevision)
			if updateErr != nil || rowsAffected(updated) != 1 {
				return castFirstNonNilError(updateErr, errPreparedHandoffConflict)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) commitCastReceiverAdvanceReady(ctx context.Context, auth castSessionAuth) error {
	if auth.record.PendingPlaybackSessionID == "" || auth.record.PendingRequestID == "" || auth.record.PendingGeneration <= auth.record.Generation {
		return errPreparedHandoffConflict
	}
	var claim playbackHandoffClaim
	var receiptState string
	err := s.queryUserRow(ctx, `
		SELECT claim_id, replacement_session_id, authorization_revision, client_instance_id,
			expected_queue_revision, expected_playback_revision, state
		FROM playback_handoff_receipts
		WHERE source_session_id = ? AND user_id = ? AND profile_id = ?
			AND request_id = ? AND request_fingerprint = ?`,
		auth.record.PlaybackSessionID, auth.record.UserID, auth.record.ProfileID,
		auth.record.PendingRequestID, auth.record.PendingFingerprint,
	).Scan(&claim.ID, &claim.ReplacementSessionID, &claim.AuthorizationRevision, &claim.ClientInstanceID, &claim.ExpectedQueueRevision, &claim.ExpectedPlaybackRevision, &receiptState)
	if err != nil || claim.ID != auth.record.PendingClaimID || claim.ReplacementSessionID != auth.record.PendingPlaybackSessionID {
		return castFirstNonNilError(err, errPreparedHandoffConflict)
	}
	if receiptState == "committed" {
		return errPreparedHandoffConflict
	}
	var response CastAdvanceResponse
	if json.Unmarshal([]byte(auth.record.PendingAdvanceJSON), &response) != nil || response.Playback == nil || response.Playback.SessionID != auth.record.PendingPlaybackSessionID {
		return errPreparedHandoffConflict
	}
	if claim.AuthorizationRevision != auth.record.PendingAuthorizationRevision {
		return errPlaybackReplacementAuthorizationChanged
	}
	if receiptState != "committed" {
		var terminal PlaybackTerminalEvent
		if json.Unmarshal([]byte(auth.record.PendingTerminalJSON), &terminal) != nil {
			return errPlaybackTerminalEvidenceInvalid
		}
		if err := s.commitDirectPlaybackHandoffWithTx(ctx, auth.user, auth.record.PlaybackSessionID, terminal, auth.record.PendingRequestID, auth.record.PendingFingerprint, claim, *response.Playback, func(tx *sql.Tx) error {
			updated, updateErr := tx.ExecContext(ctx, `
				UPDATE cast_receiver_sessions SET
					playback_session_id = pending_playback_session_id, generation = pending_generation,
					replacement_request_id = pending_request_id, replacement_fingerprint = pending_fingerprint,
					replacement_claim_id = pending_claim_id, replacement_terminal_json = '',
					authorization_revision = pending_authorization_revision, transfer_state = 'committed', status = 'active',
					automatic_advances = ?, source_playback_session_id = '',
					pending_playback_session_id = '', pending_generation = 0, pending_request_id = '', pending_fingerprint = '',
					pending_claim_id = '', pending_terminal_json = '', pending_authorization_revision = '',
					pending_advance_id = '', pending_advance_json = '', pending_payload_expires_at = '', pending_expires_at = ''
				WHERE id = ? AND playback_session_id = ? AND generation = ? AND client_instance_id = ?
					AND pending_playback_session_id = ? AND pending_generation = ? AND pending_request_id = ?
					AND pending_fingerprint = ? AND pending_claim_id = ? AND pending_authorization_revision = ?
					AND status = 'active' AND transfer_state = 'committed'`,
				response.AutomaticAdvances, auth.record.ID, auth.record.PlaybackSessionID, auth.record.Generation, auth.record.ClientInstanceID,
				auth.record.PendingPlaybackSessionID, auth.record.PendingGeneration, auth.record.PendingRequestID,
				auth.record.PendingFingerprint, auth.record.PendingClaimID, auth.record.PendingAuthorizationRevision)
			if updateErr != nil || rowsAffected(updated) != 1 {
				return castFirstNonNilError(updateErr, errCastGenerationStale)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func castFirstNonNilError(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}

func (s *Server) stopCastReceiverSession(ctx context.Context, auth castSessionAuth, req PlaybackSessionStopRequest) (PlaybackSessionTerminalAcknowledgement, error) {
	return s.stopCastReceiverSessionWithAdvanceOutcome(ctx, auth, req, "", "", nil)
}

func (s *Server) stopCastReceiverSessionWithAdvanceOutcome(ctx context.Context, auth castSessionAuth, req PlaybackSessionStopRequest, advanceID, advanceRequestFingerprint string, advanceOutcome *CastAdvanceResponse) (PlaybackSessionTerminalAcknowledgement, error) {
	if auth.record.Status == "stopped" {
		return s.playbackTerminalReceiptForUser(ctx, auth.user, auth.record.PlaybackSessionID, req)
	}
	authorizationRevision, authorizationErr := s.authorizationRevisionForUserContextStrict(ctx, auth.user)
	if authorizationErr != nil || authorizationRevision == "" || authorizationRevision != auth.record.AuthorizationRevision {
		return PlaybackSessionTerminalAcknowledgement{}, errPlaybackTerminalAuthorizationChanged
	}
	ack := playbackTerminalAcknowledgement(auth.record.PlaybackSessionID, req, false)
	encoded, err := json.Marshal(ack)
	if err != nil {
		return PlaybackSessionTerminalAcknowledgement{}, err
	}
	receipt := &playbackTerminalReceipt{
		RequestID: req.RequestID, Fingerprint: playbackSessionStopFingerprint(req), ResponseJSON: string(encoded),
		AuthorizationRevision: authorizationRevision,
	}
	now := time.Now().UTC()
	advanceJSON := ""
	if advanceOutcome != nil {
		encodedAdvance, marshalErr := json.Marshal(advanceOutcome)
		if marshalErr != nil {
			return PlaybackSessionTerminalAcknowledgement{}, marshalErr
		}
		advanceJSON = string(encodedAdvance)
	}
	lifecycle := s.playbackLifecycle()
	preferences := s.playbackProgressPreferencesForUserContext(ctx, auth.record.ProfileID)
	var termination playbackTerminationResult
	err = s.withSecurityFenceTxTagged(ctx, []string{"playback"}, func(tx *sql.Tx) error {
		var terminateErr error
		termination, terminateErr = lifecycle.terminateTx(ctx, tx, playbackTerminationRequest{
			SessionID: auth.record.PlaybackSessionID, UserID: auth.record.UserID, ProfileID: auth.record.ProfileID,
			Cause: playbackTerminationReceiver, RequireActive: true, Event: &req.Terminal,
			TerminalReceipt: receipt, ProgressPreferences: preferences, Now: now,
		})
		if terminateErr != nil {
			return terminateErr
		}
		updated, updateErr := tx.ExecContext(ctx, `
			UPDATE cast_receiver_sessions
			SET status = 'stopped', stopped_at = ?, last_seen_at = ?, source_playback_session_id = '', transfer_state = 'committed',
				last_advance_id = CASE WHEN ? <> '' THEN ? ELSE last_advance_id END,
				last_advance_request_fingerprint = CASE WHEN ? <> '' THEN ? ELSE last_advance_request_fingerprint END,
				last_advance_json = CASE WHEN ? <> '' THEN ? ELSE last_advance_json END,
				last_advance_payload_expires_at = CASE WHEN ? <> '' THEN ? ELSE last_advance_payload_expires_at END
			WHERE id = ? AND playback_session_id = ? AND generation = ? AND status = 'stopped'`,
			now.Format(time.RFC3339), now.Format(time.RFC3339),
			advanceID, advanceID, advanceRequestFingerprint, advanceRequestFingerprint, advanceJSON, advanceJSON, advanceJSON, now.Add(playbackHandoffReceiptTTL).Format(time.RFC3339Nano),
			auth.record.ID, auth.record.PlaybackSessionID, auth.record.Generation)
		if updateErr != nil || rowsAffected(updated) != 1 {
			return castFirstNonNilError(updateErr, errCastGenerationStale)
		}
		return nil
	})
	if err != nil {
		if duplicate, receiptErr := s.playbackTerminalReceiptForUser(ctx, auth.user, auth.record.PlaybackSessionID, req); receiptErr == nil {
			return duplicate, nil
		}
		return PlaybackSessionTerminalAcknowledgement{}, err
	}
	lifecycle.afterCommit(ctx, termination)
	return ack, nil
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
		if err := absolutizeCastMediaItemURLs(&playback.Queue[index].Media, assign); err != nil {
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

func castRedeemResponseAAD(bootstrapID, receiverID, receiverOrigin, receiverChallenge string) []byte {
	return []byte("portico-cast-redeem-response-v1|" + bootstrapID + "|" + receiverID + "|" + receiverOrigin + "|" + receiverChallenge)
}

func castRedeemResponseAEAD(secret string) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte("portico-cast-redeem-response-key-v1|" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func sealCastRedeemResponse(response CastReceiverSessionResponse, secret, bootstrapID, receiverID, receiverOrigin, receiverChallenge string) (string, error) {
	aead, err := castRedeemResponseAEAD(secret)
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(response)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, plain, castRedeemResponseAAD(bootstrapID, receiverID, receiverOrigin, receiverChallenge))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func openCastRedeemResponse(encoded, secret, bootstrapID, receiverID, receiverOrigin, receiverChallenge string) (CastReceiverSessionResponse, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return CastReceiverSessionResponse{}, err
	}
	aead, err := castRedeemResponseAEAD(secret)
	if err != nil {
		return CastReceiverSessionResponse{}, err
	}
	if len(sealed) < aead.NonceSize() {
		return CastReceiverSessionResponse{}, errCastBootstrapInvalid
	}
	nonce, ciphertext := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ciphertext, castRedeemResponseAAD(bootstrapID, receiverID, receiverOrigin, receiverChallenge))
	if err != nil {
		return CastReceiverSessionResponse{}, err
	}
	var response CastReceiverSessionResponse
	if err := json.Unmarshal(plain, &response); err != nil || response.ReceiverSessionToken == "" || response.ReceiverSessionID == "" || response.ReceiverID != receiverID {
		return CastReceiverSessionResponse{}, errCastBootstrapInvalid
	}
	return response, nil
}

func castAdvanceRequestFingerprint(req CastAdvanceRequest) string {
	return hashToken(stringJSON(req))
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
