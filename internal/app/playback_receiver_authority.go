package app

import (
	"context"
	"crypto/ecdh"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	playbackReceiverPresenceTTL      = 2 * time.Minute
	playbackReceiverAuthorizationTTL = 15 * time.Minute
)

var (
	errPlaybackReceiverInvalid              = errors.New("playback receiver request is invalid")
	errPlaybackReceiverKeyMismatch          = errors.New("playback receiver key does not match")
	errReceiverAuthorizationConflict        = errors.New("receiver authorization request conflicts with an existing request")
	errPlaybackReceiverAuthorizationInvalid = errors.New("playback receiver authorization is invalid")
)

func normalizeNativeReceiverCommands(commands []string) []string {
	allowed := map[string]bool{"load": true, "play": true, "pause": true, "seek": true, "stop": true}
	seen := map[string]bool{}
	result := make([]string, 0, len(commands))
	for _, command := range commands {
		command = strings.ToLower(strings.TrimSpace(command))
		if !allowed[command] || seen[command] {
			continue
		}
		seen[command] = true
		result = append(result, command)
	}
	sort.Strings(result)
	return result
}

func validateReceiverX25519PublicKey(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) != 32 {
		return "", "", errPlaybackReceiverInvalid
	}
	if _, err := ecdh.X25519().NewPublicKey(raw); err != nil {
		return "", "", errPlaybackReceiverInvalid
	}
	digest := sha256.Sum256(raw)
	return value, base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func (s *Server) publicPlaybackReceiverServerID(ctx context.Context, user User) (string, error) {
	settings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return "", err
	}
	return s.publicServerIDForAuthProviderContext(ctx, settings, user.AuthProvider)
}

func (s *Server) registerPlaybackReceiver(ctx context.Context, user User, req PlaybackReceiverRequest) (PlaybackReceiver, error) {
	receiverID := limitPlaybackReceiverField(req.ReceiverID, 160)
	name := limitPlaybackReceiverField(req.Name, 80)
	app := limitPlaybackReceiverField(req.App, 80)
	platform := limitPlaybackReceiverField(req.Platform, 80)
	clientInstanceID := normalizePlaybackClientInstanceID(req.ClientInstanceID)
	publicKey, fingerprint, err := validateReceiverX25519PublicKey(req.ReceiverPublicKey)
	if receiverID == "" || name == "" || platform == "" || clientInstanceID == "" || err != nil {
		return PlaybackReceiver{}, errPlaybackReceiverInvalid
	}
	commands := normalizeNativeReceiverCommands(req.SupportedCommands)
	if !containsString(commands, "load") {
		return PlaybackReceiver{}, errPlaybackReceiverInvalid
	}
	commandsJSON, _ := json.Marshal(commands)
	serverID, err := s.publicPlaybackReceiverServerID(ctx, user)
	if err != nil || strings.TrimSpace(serverID) == "" {
		return PlaybackReceiver{}, errors.New("playback receiver server identity is unavailable")
	}
	now := time.Now().UTC()
	expiresAt := now.Add(playbackReceiverPresenceTTL)
	authorizationRevision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil {
		return PlaybackReceiver{}, err
	}
	accountID, profileID := accountIDForUser(user), viewerProfileID(user)
	err = s.withUserTxTaggedForViewer(ctx, accountID, profileID, []string{"playback-receivers", "authorization"}, func(tx *sql.Tx) error {
		currentRevision, err := authorizationRevisionForUserTx(ctx, tx, user)
		if err != nil || currentRevision != authorizationRevision {
			return errLongPollAuthorizationLost
		}
		var priorFingerprint string
		err = tx.QueryRowContext(ctx, `SELECT receiver_public_key_fingerprint FROM playback_receivers WHERE id = ? AND user_id = ? AND profile_id = ?`, receiverID, accountID, profileID).Scan(&priorFingerprint)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO playback_receivers (
				id, user_id, profile_id, name, code, app, platform, supported_commands_json,
				command_json, command_updated_at, receiver_public_key, receiver_public_key_fingerprint,
				authorization_revision, expires_at, client_instance_id, api_key_id, created_at, last_seen_at
			) VALUES (?, ?, ?, ?, '', ?, ?, ?, '{}', '', ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name, app = excluded.app, platform = excluded.platform,
				supported_commands_json = excluded.supported_commands_json,
				receiver_public_key = excluded.receiver_public_key,
				receiver_public_key_fingerprint = excluded.receiver_public_key_fingerprint,
				authorization_revision = excluded.authorization_revision,
				expires_at = excluded.expires_at, client_instance_id = excluded.client_instance_id,
				api_key_id = excluded.api_key_id,
				last_seen_at = excluded.last_seen_at
			WHERE playback_receivers.user_id = excluded.user_id AND playback_receivers.profile_id = excluded.profile_id`,
			receiverID, accountID, profileID, name, app, platform, string(commandsJSON), publicKey, fingerprint,
			authorizationRevision, expiresAt.Format(time.RFC3339Nano), clientInstanceID, strings.TrimSpace(user.APIKeyID), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		if rowsAffected(result) != 1 {
			return errPlaybackReceiverInvalid
		}
		if priorFingerprint != "" && priorFingerprint != fingerprint {
			_, err = tx.ExecContext(ctx, `UPDATE playback_receiver_authorizations SET revoked_at = ? WHERE receiver_id = ? AND revoked_at = ''`, now.Format(time.RFC3339Nano), receiverID)
		}
		return err
	})
	if err != nil {
		return PlaybackReceiver{}, err
	}
	return PlaybackReceiver{
		ID: receiverID, ServerID: serverID, Name: name, App: app, Platform: platform,
		ReceiverPublicKey: publicKey, ReceiverPublicKeyFingerprint: fingerprint,
		SupportedCommands: commands, ExpiresAt: expiresAt.Format(time.RFC3339Nano),
		CreatedAt: now.Format(time.RFC3339Nano), LastSeenAt: now.Format(time.RFC3339Nano),
		ClientInstanceID: clientInstanceID,
	}, nil
}

func (s *Server) listAuthorizedPlaybackReceivers(ctx context.Context, user User) ([]PlaybackReceiver, error) {
	serverID, err := s.publicPlaybackReceiverServerID(ctx, user)
	if err != nil {
		return nil, err
	}
	revision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT id, name, app, platform, receiver_public_key, receiver_public_key_fingerprint,
		       supported_commands_json, expires_at, created_at, last_seen_at, client_instance_id
			FROM playback_receivers
			WHERE profile_id = ? AND user_id = ? AND authorization_revision = ?
			  AND (api_key_id = '' OR EXISTS (SELECT 1 FROM api_keys k WHERE k.id = playback_receivers.api_key_id AND k.user_id = playback_receivers.user_id AND k.revoked_at = ''))
		  AND receiver_public_key <> '' AND julianday(expires_at) > julianday(?)
		ORDER BY last_seen_at DESC`, viewerProfileID(user), accountIDForUser(user), revision, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PlaybackReceiver{}
	for rows.Next() {
		var receiver PlaybackReceiver
		var commandsJSON string
		if err := rows.Scan(&receiver.ID, &receiver.Name, &receiver.App, &receiver.Platform, &receiver.ReceiverPublicKey, &receiver.ReceiverPublicKeyFingerprint, &commandsJSON, &receiver.ExpiresAt, &receiver.CreatedAt, &receiver.LastSeenAt, &receiver.ClientInstanceID); err != nil {
			return nil, err
		}
		receiver.ServerID = serverID
		receiver.SupportedCommands = decodeReceiverSupportedCommands(commandsJSON)
		result = append(result, receiver)
	}
	return result, rows.Err()
}

func (s *Server) heartbeatPlaybackReceiver(ctx context.Context, user User, receiverID, expectedFingerprint string) (PlaybackReceiverHeartbeatResponse, error) {
	receiverID = strings.TrimSpace(receiverID)
	expectedFingerprint = strings.TrimSpace(expectedFingerprint)
	if receiverID == "" || expectedFingerprint == "" {
		return PlaybackReceiverHeartbeatResponse{}, errPlaybackReceiverInvalid
	}
	revision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil {
		return PlaybackReceiverHeartbeatResponse{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(playbackReceiverPresenceTTL).Format(time.RFC3339Nano)
	result, err := s.execUserWriteTaggedForViewer(ctx, accountIDForUser(user), viewerProfileID(user), []string{"playback-receivers"}, `
		UPDATE playback_receivers SET last_seen_at = ?, expires_at = ?
		WHERE id = ? AND user_id = ? AND profile_id = ? AND authorization_revision = ?
		  AND receiver_public_key_fingerprint = ?`,
		now.Format(time.RFC3339Nano), expiresAt, receiverID, accountIDForUser(user), viewerProfileID(user), revision, expectedFingerprint)
	if err != nil {
		return PlaybackReceiverHeartbeatResponse{}, err
	}
	if rowsAffected(result) != 1 {
		return PlaybackReceiverHeartbeatResponse{}, errPlaybackReceiverKeyMismatch
	}
	receiver, err := s.playbackReceiverByID(ctx, user, receiverID)
	if err != nil {
		return PlaybackReceiverHeartbeatResponse{}, err
	}
	authorizations, err := s.receiverAuthorizationRecords(ctx, user, receiverID, expectedFingerprint)
	if err != nil {
		return PlaybackReceiverHeartbeatResponse{}, err
	}
	return PlaybackReceiverHeartbeatResponse{Receiver: receiver, Authorizations: authorizations}, nil
}

func (s *Server) playbackReceiverByID(ctx context.Context, user User, receiverID string) (PlaybackReceiver, error) {
	serverID, err := s.publicPlaybackReceiverServerID(ctx, user)
	if err != nil {
		return PlaybackReceiver{}, err
	}
	var receiver PlaybackReceiver
	var commandsJSON string
	err = s.queryUserRow(ctx, `
		SELECT id, name, app, platform, receiver_public_key, receiver_public_key_fingerprint,
		       supported_commands_json, expires_at, created_at, last_seen_at, client_instance_id
		FROM playback_receivers WHERE id = ? AND user_id = ? AND profile_id = ?`,
		receiverID, accountIDForUser(user), viewerProfileID(user)).Scan(
		&receiver.ID, &receiver.Name, &receiver.App, &receiver.Platform, &receiver.ReceiverPublicKey,
		&receiver.ReceiverPublicKeyFingerprint, &commandsJSON, &receiver.ExpiresAt, &receiver.CreatedAt, &receiver.LastSeenAt, &receiver.ClientInstanceID)
	if err != nil {
		return PlaybackReceiver{}, err
	}
	receiver.ServerID = serverID
	receiver.SupportedCommands = decodeReceiverSupportedCommands(commandsJSON)
	return receiver, nil
}

func (s *Server) issueReceiverControllerGrant(ctx context.Context, user User, receiverID string, req ReceiverAuthorizationRequest) (ReceiverControllerGrant, error) {
	requestID := limitPlaybackReceiverField(req.RequestID, 128)
	controllerID := limitPlaybackReceiverField(req.ControllerID, 160)
	controllerClientInstanceID := normalizePlaybackClientInstanceID(req.ClientInstanceID)
	controllerPublicKey, _, err := validateReceiverX25519PublicKey(req.ControllerPublicKey)
	if requestID == "" || controllerID == "" || controllerClientInstanceID == "" || err != nil {
		return ReceiverControllerGrant{}, errPlaybackReceiverInvalid
	}
	receiver, err := s.playbackReceiverByID(ctx, user, strings.TrimSpace(receiverID))
	if err != nil {
		return ReceiverControllerGrant{}, err
	}
	if expires, err := time.Parse(time.RFC3339Nano, receiver.ExpiresAt); err != nil || !expires.After(time.Now()) {
		return ReceiverControllerGrant{}, sql.ErrNoRows
	}
	requested := normalizeNativeReceiverCommands(req.AllowedCommands)
	commands := make([]string, 0, len(requested))
	for _, command := range requested {
		if containsString(receiver.SupportedCommands, command) {
			commands = append(commands, command)
		}
	}
	if !containsString(commands, "load") {
		return ReceiverControllerGrant{}, errPlaybackReceiverInvalid
	}
	revision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil {
		return ReceiverControllerGrant{}, err
	}
	fingerprintMaterial, _ := json.Marshal(struct {
		ReceiverID          string   `json:"receiverId"`
		ControllerID        string   `json:"controllerId"`
		ControllerPublicKey string   `json:"controllerPublicKey"`
		AllowedCommands     []string `json:"allowedCommands"`
		ClientInstanceID    string   `json:"clientInstanceId"`
	}{receiver.ID, controllerID, controllerPublicKey, commands, controllerClientInstanceID})
	fingerprintDigest := sha256.Sum256(fingerprintMaterial)
	requestFingerprint := hex.EncodeToString(fingerprintDigest[:])
	now := time.Now().UTC()
	expiresAt := now.Add(playbackReceiverAuthorizationTTL)
	grant := ReceiverControllerGrant{
		AuthorizationID: randomID("rcva"), ReceiverID: receiver.ID, ServerID: receiver.ServerID,
		ReceiverPublicKey: receiver.ReceiverPublicKey, ReceiverPublicKeyFingerprint: receiver.ReceiverPublicKeyFingerprint,
		AllowedCommands: commands, AuthorizationRevision: revision, ExpiresAt: expiresAt.Format(time.RFC3339Nano),
	}
	responseJSON, _ := json.Marshal(grant)
	accountID, profileID := accountIDForUser(user), viewerProfileID(user)
	err = s.withUserTxTaggedForViewer(ctx, accountID, profileID, []string{"playback-receivers", "authorization"}, func(tx *sql.Tx) error {
		var storedFingerprint, storedResponse string
		err := tx.QueryRowContext(ctx, `SELECT request_fingerprint, response_json FROM playback_receiver_authorizations WHERE user_id = ? AND profile_id = ? AND request_id = ?`, accountID, profileID, requestID).Scan(&storedFingerprint, &storedResponse)
		if err == nil {
			if storedFingerprint != requestFingerprint {
				return errReceiverAuthorizationConflict
			}
			return json.Unmarshal([]byte(storedResponse), &grant)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		currentRevision, err := authorizationRevisionForUserTx(ctx, tx, user)
		if err != nil || currentRevision != revision {
			return errLongPollAuthorizationLost
		}
		var currentFingerprint string
		err = tx.QueryRowContext(ctx, `SELECT receiver_public_key_fingerprint FROM playback_receivers WHERE id = ? AND user_id = ? AND profile_id = ? AND authorization_revision = ? AND julianday(expires_at) > julianday(?) AND (api_key_id = '' OR EXISTS (SELECT 1 FROM api_keys k WHERE k.id = playback_receivers.api_key_id AND k.user_id = playback_receivers.user_id AND k.revoked_at = ''))`, receiver.ID, accountID, profileID, revision, now.Format(time.RFC3339Nano)).Scan(&currentFingerprint)
		if err != nil || currentFingerprint != receiver.ReceiverPublicKeyFingerprint {
			return sql.ErrNoRows
		}
		commandsJSON, _ := json.Marshal(commands)
		_, err = tx.ExecContext(ctx, `
				INSERT INTO playback_receiver_authorizations (
					id, receiver_id, user_id, profile_id, controller_id, controller_public_key, controller_client_instance_id,
					receiver_public_key_fingerprint, allowed_commands_json, authorization_revision,
					request_id, request_fingerprint, response_json, created_at, expires_at, revoked_at, api_key_id
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
			grant.AuthorizationID, receiver.ID, accountID, profileID, controllerID, controllerPublicKey,
			controllerClientInstanceID, receiver.ReceiverPublicKeyFingerprint, string(commandsJSON), revision, requestID, requestFingerprint,
			string(responseJSON), now.Format(time.RFC3339Nano), grant.ExpiresAt, strings.TrimSpace(user.APIKeyID))
		return err
	})
	return grant, err
}

func (s *Server) receiverAuthorizationRecords(ctx context.Context, user User, receiverID, expectedFingerprint string) ([]ReceiverAuthorizationRecord, error) {
	revision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil {
		return nil, err
	}
	serverID, err := s.publicPlaybackReceiverServerID(ctx, user)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT a.id, a.controller_id, a.controller_public_key, a.allowed_commands_json, a.expires_at
		FROM playback_receiver_authorizations a
		JOIN playback_receivers r ON r.id = a.receiver_id
		WHERE a.receiver_id = ? AND a.user_id = ? AND a.profile_id = ?
		  AND a.authorization_revision = ? AND a.receiver_public_key_fingerprint = ?
			  AND a.revoked_at = '' AND julianday(a.expires_at) > julianday(?)
			  AND (a.api_key_id = '' OR EXISTS (SELECT 1 FROM api_keys k WHERE k.id = a.api_key_id AND k.user_id = a.user_id AND k.revoked_at = ''))
			  AND r.authorization_revision = a.authorization_revision
			  AND r.receiver_public_key_fingerprint = a.receiver_public_key_fingerprint
			  AND (r.api_key_id = '' OR EXISTS (SELECT 1 FROM api_keys rk WHERE rk.id = r.api_key_id AND rk.user_id = r.user_id AND rk.revoked_at = ''))
		ORDER BY a.created_at`, receiverID, accountIDForUser(user), viewerProfileID(user), revision, expectedFingerprint, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []ReceiverAuthorizationRecord{}
	for rows.Next() {
		var record ReceiverAuthorizationRecord
		var commandsJSON string
		if err := rows.Scan(&record.AuthorizationID, &record.ControllerID, &record.ControllerPublicKey, &commandsJSON, &record.ExpiresAt); err != nil {
			return nil, err
		}
		record.ReceiverID = receiverID
		record.ServerID = serverID
		record.AuthorizationRevision = revision
		record.ReceiverPublicKeyFingerprint = expectedFingerprint
		record.AllowedCommands = decodeReceiverSupportedCommands(commandsJSON)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Server) revokeReceiverAuthorization(ctx context.Context, user User, receiverID, authorizationID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.execSecurityFenceWriteTagged(ctx, []string{"playback-receivers", "authorization"}, `
		UPDATE playback_receiver_authorizations SET revoked_at = ?
		WHERE id = ? AND receiver_id = ? AND user_id = ? AND profile_id = ? AND revoked_at = ''`,
		now, strings.TrimSpace(authorizationID), strings.TrimSpace(receiverID), accountIDForUser(user), viewerProfileID(user))
	if err != nil {
		return err
	}
	if rowsAffected(result) != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) authorizePlaybackReceiverHandoff(ctx context.Context, user User, receiverID, authorizationID, expectedFingerprint string) (PlaybackReceiver, string, error) {
	receiverID = strings.TrimSpace(receiverID)
	authorizationID = strings.TrimSpace(authorizationID)
	expectedFingerprint = strings.TrimSpace(expectedFingerprint)
	if receiverID == "" || authorizationID == "" || expectedFingerprint == "" {
		return PlaybackReceiver{}, "", errPlaybackReceiverInvalid
	}
	revision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil {
		return PlaybackReceiver{}, "", err
	}
	var receiver PlaybackReceiver
	var sourceClientInstanceID, commandsJSON string
	err = s.queryUserRow(ctx, `
		SELECT r.id, r.receiver_public_key_fingerprint, r.client_instance_id,
		       a.controller_client_instance_id, a.allowed_commands_json
		FROM playback_receivers r
		JOIN playback_receiver_authorizations a ON a.receiver_id = r.id
		WHERE r.id = ? AND r.user_id = ? AND r.profile_id = ?
		  AND r.authorization_revision = ? AND r.receiver_public_key_fingerprint = ?
		  AND r.client_instance_id <> '' AND julianday(r.expires_at) > julianday(?)
		  AND a.id = ? AND a.user_id = r.user_id AND a.profile_id = r.profile_id
		  AND a.authorization_revision = r.authorization_revision
			  AND a.receiver_public_key_fingerprint = r.receiver_public_key_fingerprint
			  AND a.revoked_at = '' AND julianday(a.expires_at) > julianday(?)
			  AND (a.api_key_id = '' OR EXISTS (SELECT 1 FROM api_keys k WHERE k.id = a.api_key_id AND k.user_id = a.user_id AND k.revoked_at = ''))
			  AND (r.api_key_id = '' OR EXISTS (SELECT 1 FROM api_keys rk WHERE rk.id = r.api_key_id AND rk.user_id = r.user_id AND rk.revoked_at = ''))`,
		receiverID, accountIDForUser(user), viewerProfileID(user), revision, expectedFingerprint,
		time.Now().UTC().Format(time.RFC3339Nano), authorizationID, time.Now().UTC().Format(time.RFC3339Nano)).Scan(
		&receiver.ID, &receiver.ReceiverPublicKeyFingerprint, &receiver.ClientInstanceID,
		&sourceClientInstanceID, &commandsJSON)
	if err != nil {
		return PlaybackReceiver{}, "", err
	}
	if !containsString(decodeReceiverSupportedCommands(commandsJSON), "load") || normalizePlaybackClientInstanceID(sourceClientInstanceID) == "" {
		return PlaybackReceiver{}, "", errPlaybackReceiverAuthorizationInvalid
	}
	return receiver, normalizePlaybackClientInstanceID(sourceClientInstanceID), nil
}

func (s *Server) playbackReceiverHandoffStatus(ctx context.Context, user User, sourceClientInstanceID, sourceSessionID, requestID string) (PlaybackReceiverHandoffStatusResponse, error) {
	sourceSessionID = strings.TrimSpace(sourceSessionID)
	requestID = strings.TrimSpace(requestID)
	if sourceSessionID == "" || !validPlaybackAuthorityRequestID(requestID) {
		return PlaybackReceiverHandoffStatusResponse{}, errPlaybackReceiverInvalid
	}
	response := PlaybackReceiverHandoffStatusResponse{Outcome: "waiting", RequestID: requestID, SourceSessionID: sourceSessionID}
	var state, replacementSessionID string
	err := s.queryUserRow(ctx, `
		SELECT state, replacement_session_id FROM playback_handoff_receipts
		WHERE source_session_id = ? AND user_id = ? AND profile_id = ?
		  AND client_instance_id = ? AND request_id = ?`,
		sourceSessionID, accountIDForUser(user), viewerProfileID(user), normalizePlaybackClientInstanceID(sourceClientInstanceID), requestID).
		Scan(&state, &replacementSessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return response, nil
	}
	if err != nil {
		return PlaybackReceiverHandoffStatusResponse{}, err
	}
	if state == "committed" {
		response.Outcome = "accepted"
		response.ReceiverSessionID = replacementSessionID
	} else {
		response.Outcome = "pending"
	}
	return response, nil
}

type playbackReceiverPreparedHandoff struct {
	ReceiverID      string                `json:"receiverId"`
	AuthorizationID string                `json:"authorizationId"`
	RequestID       string                `json:"requestId"`
	Fingerprint     string                `json:"fingerprint"`
	SourceSessionID string                `json:"sourceSessionId"`
	Terminal        PlaybackTerminalEvent `json:"terminal"`
	Playback        PlaybackResponse      `json:"playback"`
}

func (s *Server) persistPlaybackReceiverPreparation(ctx context.Context, user User, receiverID, authorizationID string, plan playbackReplacementPlan, playback PlaybackResponse) error {
	prepared := playbackReceiverPreparedHandoff{
		ReceiverID: strings.TrimSpace(receiverID), AuthorizationID: strings.TrimSpace(authorizationID),
		RequestID: plan.RequestID, Fingerprint: plan.Fingerprint, SourceSessionID: plan.SourceSessionID,
		Terminal: plan.Terminal, Playback: playback,
	}
	encoded, err := s.encodeContractCursor("playback-receiver-prepare:"+plan.SourceSessionID, plan.RequestID, prepared, time.Now().UTC())
	if err != nil {
		return err
	}
	result, err := s.execPlaybackWrite(ctx, `
		UPDATE playback_handoff_receipts SET committed_response = ?, payload_expires_at = ?
		WHERE source_session_id = ? AND user_id = ? AND profile_id = ?
		  AND request_id = ? AND request_fingerprint = ? AND state = 'committing'
		  AND claim_id = ? AND replacement_session_id = ?`,
		encoded, time.Now().UTC().Add(playbackHandoffReceiptTTL).Format(time.RFC3339Nano),
		plan.SourceSessionID, accountIDForUser(user), viewerProfileID(user), plan.RequestID, plan.Fingerprint,
		plan.Claim.ID, playback.SessionID)
	if err != nil {
		return err
	}
	if rowsAffected(result) != 1 {
		return errPreparedHandoffConflict
	}
	return nil
}

func (s *Server) preparedPlaybackReceiverRetry(ctx context.Context, user User, receiver PlaybackReceiver, authorizationID, sourceSessionID, requestID, fingerprint string) (*PlaybackResponse, error) {
	var state, storedFingerprint, replacementSessionID, encoded string
	err := s.queryUserRow(ctx, `
		SELECT state, request_fingerprint, replacement_session_id, committed_response
		FROM playback_handoff_receipts
		WHERE source_session_id = ? AND user_id = ? AND profile_id = ? AND request_id = ?`,
		strings.TrimSpace(sourceSessionID), accountIDForUser(user), viewerProfileID(user), strings.TrimSpace(requestID)).
		Scan(&state, &storedFingerprint, &replacementSessionID, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if storedFingerprint != fingerprint {
		return nil, errPreparedHandoffConflict
	}
	if state == "committed" {
		var receipt playbackHandoffReceipt
		if err := s.decodeContractCursor(encoded, "playback-handoff:"+sourceSessionID, requestID, &receipt, time.Now().UTC()); err != nil {
			return nil, err
		}
		if receipt.Playback.SessionID != replacementSessionID || !s.playbackReceiverOwnsSession(ctx, user, receiver, replacementSessionID) {
			return nil, errPreparedHandoffConflict
		}
		playback := receipt.Playback
		return &playback, nil
	}
	if strings.TrimSpace(encoded) == "" {
		return nil, errPlaybackHandoffInProgress
	}
	var prepared playbackReceiverPreparedHandoff
	if err := s.decodeContractCursor(encoded, "playback-receiver-prepare:"+sourceSessionID, requestID, &prepared, time.Now().UTC()); err != nil {
		return nil, err
	}
	if prepared.ReceiverID != receiver.ID || prepared.AuthorizationID != strings.TrimSpace(authorizationID) || prepared.Fingerprint != fingerprint || prepared.Playback.SessionID != replacementSessionID {
		return nil, errPreparedHandoffConflict
	}
	playback := prepared.Playback
	return &playback, nil
}

func (s *Server) playbackReceiverOwnsSession(ctx context.Context, user User, receiver PlaybackReceiver, sessionID string) bool {
	var count int
	err := s.queryUserRow(ctx, `SELECT COUNT(*) FROM playback_sessions
		WHERE id = ? AND user_id = ? AND profile_id = ? AND client_instance_id = ?`,
		strings.TrimSpace(sessionID), accountIDForUser(user), viewerProfileID(user), receiver.ClientInstanceID).Scan(&count)
	return err == nil && count == 1
}

// pruneExpiredPlaybackReceiverPreparations rolls back only receipts carrying a
// receiver-preparation envelope. Ordinary direct handoff crash claims retain
// their existing exact-retry recovery semantics.
func (s *Server) pruneExpiredPlaybackReceiverPreparations(ctx context.Context, now time.Time) error {
	rows, err := s.queryUserRead(ctx, `SELECT source_session_id, user_id, profile_id, request_id,
		request_fingerprint, claim_id, replacement_session_id, committed_response
		FROM playback_handoff_receipts
		WHERE state = 'committing' AND committed_response <> '' AND claim_expires_at <= ?`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	type expiredPreparation struct {
		source, userID, profileID, requestID, fingerprint, claimID, replacementID, encoded string
	}
	expired := []expiredPreparation{}
	for rows.Next() {
		var item expiredPreparation
		if err := rows.Scan(&item.source, &item.userID, &item.profileID, &item.requestID, &item.fingerprint, &item.claimID, &item.replacementID, &item.encoded); err != nil {
			rows.Close()
			return err
		}
		var prepared playbackReceiverPreparedHandoff
		if decodeErr := s.decodeContractCursor(item.encoded, "playback-receiver-prepare:"+item.source, item.requestID, &prepared, now.UTC()); decodeErr == nil && prepared.Playback.SessionID == item.replacementID {
			expired = append(expired, item)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range expired {
		user := User{ID: item.userID, AccountID: item.userID, ProfileID: item.profileID}
		if err := s.cleanupRecoveredHandoffReplacement(ctx, user, item.replacementID); err != nil {
			return err
		}
		s.rollbackDirectPlaybackHandoff(ctx, item.source, item.requestID, item.fingerprint, item.claimID)
	}
	return nil
}

func (s *Server) commitPlaybackReceiverHandoff(ctx context.Context, user User, receiver PlaybackReceiver, sourceClientInstanceID, requestID string, req PlaybackReceiverHandoffCommitRequest) (PlaybackResponse, error) {
	readiness := strings.ToLower(strings.TrimSpace(req.Readiness))
	if readiness != "playing" {
		return PlaybackResponse{}, errPlaybackReceiverInvalid
	}
	sourceSessionID := strings.TrimSpace(req.SourceSessionID)
	receiverSessionID := strings.TrimSpace(req.ReceiverSessionID)
	requestID = strings.TrimSpace(requestID)
	if sourceSessionID == "" || receiverSessionID == "" || !validPlaybackAuthorityRequestID(requestID) {
		return PlaybackResponse{}, errPlaybackReceiverInvalid
	}
	var state, fingerprint, claimID, replacementSessionID, encoded, claimExpiresAt, authorizationRevision, clientInstanceID string
	var expectedQueueRevision, expectedPlaybackRevision int64
	err := s.queryUserRow(ctx, `
		SELECT state, request_fingerprint, claim_id, replacement_session_id, committed_response,
		       claim_expires_at, authorization_revision, client_instance_id,
		       expected_queue_revision, expected_playback_revision
		FROM playback_handoff_receipts
		WHERE source_session_id = ? AND user_id = ? AND profile_id = ? AND request_id = ?`,
		sourceSessionID, accountIDForUser(user), viewerProfileID(user), requestID).
		Scan(&state, &fingerprint, &claimID, &replacementSessionID, &encoded, &claimExpiresAt,
			&authorizationRevision, &clientInstanceID, &expectedQueueRevision, &expectedPlaybackRevision)
	if err != nil {
		return PlaybackResponse{}, err
	}
	if normalizePlaybackClientInstanceID(clientInstanceID) != normalizePlaybackClientInstanceID(sourceClientInstanceID) ||
		receiverSessionID != replacementSessionID || !s.playbackReceiverOwnsSession(ctx, user, receiver, replacementSessionID) {
		return PlaybackResponse{}, errPreparedHandoffConflict
	}
	if state == "committed" {
		var receipt playbackHandoffReceipt
		if err := s.decodeContractCursor(encoded, "playback-handoff:"+sourceSessionID, requestID, &receipt, time.Now().UTC()); err != nil {
			return PlaybackResponse{}, err
		}
		if receipt.Fingerprint != fingerprint || receipt.Playback.SessionID != receiverSessionID {
			return PlaybackResponse{}, errPreparedHandoffConflict
		}
		return receipt.Playback, nil
	}
	expiresAt, parseErr := time.Parse(time.RFC3339Nano, claimExpiresAt)
	if parseErr != nil || !expiresAt.After(time.Now().UTC()) {
		_ = s.cleanupRecoveredHandoffReplacement(ctx, user, replacementSessionID)
		s.rollbackDirectPlaybackHandoff(context.Background(), sourceSessionID, requestID, fingerprint, claimID)
		return PlaybackResponse{}, errPreparedHandoffExpired
	}
	var prepared playbackReceiverPreparedHandoff
	if strings.TrimSpace(encoded) == "" || s.decodeContractCursor(encoded, "playback-receiver-prepare:"+sourceSessionID, requestID, &prepared, time.Now().UTC()) != nil {
		return PlaybackResponse{}, errPreparedHandoffConflict
	}
	if prepared.ReceiverID != receiver.ID || prepared.AuthorizationID != strings.TrimSpace(req.AuthorizationID) ||
		prepared.RequestID != requestID || prepared.Fingerprint != fingerprint || prepared.SourceSessionID != sourceSessionID ||
		prepared.Playback.SessionID != receiverSessionID {
		return PlaybackResponse{}, errPreparedHandoffConflict
	}
	claim := playbackHandoffClaim{ID: claimID, ReplacementSessionID: replacementSessionID, AuthorizationRevision: authorizationRevision,
		ClientInstanceID: clientInstanceID, ExpectedQueueRevision: expectedQueueRevision, ExpectedPlaybackRevision: expectedPlaybackRevision}
	if err := s.commitDirectPlaybackHandoff(ctx, user, sourceSessionID, prepared.Terminal, requestID, fingerprint, claim, prepared.Playback); err != nil {
		// Another exact commit may have won after our read. Reconcile that receipt
		// before considering rollback so a successful receiver can never be torn down.
		var latestState, latestEncoded string
		if latestErr := s.queryUserRow(ctx, `SELECT state, committed_response FROM playback_handoff_receipts
			WHERE source_session_id = ? AND user_id = ? AND profile_id = ? AND request_id = ? AND request_fingerprint = ?`,
			sourceSessionID, accountIDForUser(user), viewerProfileID(user), requestID, fingerprint).Scan(&latestState, &latestEncoded); latestErr == nil && latestState == "committed" {
			var receipt playbackHandoffReceipt
			if decodeErr := s.decodeContractCursor(latestEncoded, "playback-handoff:"+sourceSessionID, requestID, &receipt, time.Now().UTC()); decodeErr == nil && receipt.Playback.SessionID == receiverSessionID {
				return receipt.Playback, nil
			}
		}
		if errors.Is(err, errPlaybackReplacementRevisionConflict) || errors.Is(err, errPlaybackReplacementSourceInactive) ||
			errors.Is(err, errPlaybackReplacementAuthorizationChanged) || errors.Is(err, errPreparedHandoffConflict) {
			_ = s.cleanupRecoveredHandoffReplacement(ctx, user, replacementSessionID)
			s.rollbackDirectPlaybackHandoff(context.Background(), sourceSessionID, requestID, fingerprint, claimID)
		}
		return PlaybackResponse{}, err
	}
	return prepared.Playback, nil
}
