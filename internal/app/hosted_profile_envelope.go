package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var errHostedProfileSelectionExchangeUnavailable = errors.New("Hosted profile selection exchange is unavailable")

// HostedProfileSelectionEnvelope is the sole Hosted Services proof accepted
// for profile selection. It signs the selected profile and the complete active
// profile directory together so the server cannot authorize against a stale or
// partial projection.
type HostedProfileSelectionEnvelope struct {
	Version            string                  `json:"version"`
	AssertionID        string                  `json:"assertionId"`
	Audience           string                  `json:"audience"`
	AccountID          string                  `json:"accountId"`
	ProfileID          string                  `json:"profileId"`
	ServerID           string                  `json:"serverId"`
	DeviceID           string                  `json:"deviceId"`
	InstallationID     string                  `json:"installationId,omitempty"`
	AccountRevision    int64                   `json:"accountRevision"`
	PINRevision        int64                   `json:"pinRevision"`
	Profiles           []HostedProfileSnapshot `json:"profiles"`
	IssuedAt           string                  `json:"issuedAt"`
	ExpiresAt          string                  `json:"expiresAt"`
	SignatureAlgorithm string                  `json:"signatureAlgorithm"`
	SignatureKeyID     string                  `json:"signatureKeyId"`
	Signature          string                  `json:"signature"`
}

func decodeHostedProfileSelectionEnvelope(raw json.RawMessage) (HostedProfileSelectionEnvelope, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var envelope HostedProfileSelectionEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return HostedProfileSelectionEnvelope{}, fmt.Errorf("%w: %v", errInvalidHostedProfileSelectionAssertion, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return HostedProfileSelectionEnvelope{}, fmt.Errorf("%w: trailing content", errInvalidHostedProfileSelectionAssertion)
	}
	return envelope, nil
}

func (s *Server) verifyHostedProfileSelectionEnvelope(raw json.RawMessage, expectedAccountID, expectedServerID, expectedHostedDeviceID string, now time.Time) (HostedProfileSelectionEnvelope, []byte, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	envelope, err := decodeHostedProfileSelectionEnvelope(raw)
	if err != nil {
		return HostedProfileSelectionEnvelope{}, nil, err
	}
	envelope.AssertionID = strings.TrimSpace(envelope.AssertionID)
	envelope.AccountID = strings.TrimSpace(envelope.AccountID)
	envelope.ProfileID = strings.TrimSpace(envelope.ProfileID)
	envelope.ServerID = strings.TrimSpace(envelope.ServerID)
	envelope.DeviceID = strings.TrimSpace(envelope.DeviceID)
	envelope.InstallationID = strings.TrimSpace(envelope.InstallationID)
	if envelope.Version != hostedProfileSelectionAssertionVersion || envelope.Audience != hostedDocumentAudience ||
		envelope.SignatureAlgorithm != hostedSignatureAlgorithm || envelope.AssertionID == "" || envelope.AccountID == "" ||
		envelope.ProfileID == "" || envelope.ServerID == "" || envelope.DeviceID == "" ||
		envelope.AccountRevision <= 0 || envelope.PINRevision < 0 || strings.TrimSpace(envelope.Signature) == "" {
		return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: required identity, version, revision, audience, or signature fields are invalid", errInvalidHostedProfileSelectionAssertion)
	}
	for _, value := range []string{envelope.AssertionID, envelope.AccountID, envelope.ProfileID, envelope.ServerID} {
		if utf8.RuneCountInString(value) > 200 {
			return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: identity field bounds are invalid", errInvalidHostedProfileSelectionAssertion)
		}
	}
	if utf8.RuneCountInString(envelope.DeviceID) > 512 || utf8.RuneCountInString(envelope.InstallationID) > 128 {
		return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: device binding bounds are invalid", errInvalidHostedProfileSelectionAssertion)
	}
	if envelope.InstallationID != "" {
		if normalized, ok := normalizeNativeInstallationID(envelope.InstallationID); !ok || normalized != envelope.InstallationID {
			return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: installation metadata is invalid", errInvalidHostedProfileSelectionAssertion)
		}
	}
	if envelope.AccountID != strings.TrimSpace(expectedAccountID) || envelope.ServerID != strings.TrimSpace(expectedServerID) ||
		envelope.DeviceID != strings.TrimSpace(expectedHostedDeviceID) {
		return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: account, server, or hosted device binding mismatch", errInvalidHostedProfileSelectionAssertion)
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, envelope.IssuedAt)
	if err != nil {
		return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: issuedAt is invalid", errInvalidHostedProfileSelectionAssertion)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, envelope.ExpiresAt)
	if err != nil {
		return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: expiresAt is invalid", errInvalidHostedProfileSelectionAssertion)
	}
	if issuedAt.After(now.Add(hostedDocumentClockSkew)) || !expiresAt.After(now) || !expiresAt.After(issuedAt) ||
		expiresAt.Sub(issuedAt) > maximumHostedProfileSelectionAssertionLifetime {
		return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: validity window is not allowed", errInvalidHostedProfileSelectionAssertion)
	}
	selectedPINRevision, err := validateHostedProfileProjection(envelope.Profiles, envelope.AccountID, envelope.ProfileID, issuedAt)
	if err != nil || selectedPINRevision != envelope.PINRevision {
		return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: selected profile or profile projection is invalid", errInvalidHostedProfileSelectionAssertion)
	}
	encodedKey := s.trustedHostedDocumentKey(envelope.SignatureKeyID)
	if encodedKey == "" {
		return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: signing key is not trusted", errInvalidHostedProfileSelectionAssertion)
	}
	publicKey, err := decodeHostedDocumentPublicKey(encodedKey)
	if err != nil {
		return HostedProfileSelectionEnvelope{}, nil, err
	}
	payload, err := canonicalHostedDocument("profile-selection-envelope", raw)
	if err != nil {
		return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: canonicalization failed", errInvalidHostedProfileSelectionAssertion)
	}
	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(publicKey, payload, signature) {
		return HostedProfileSelectionEnvelope{}, nil, fmt.Errorf("%w: signature is invalid", errInvalidHostedProfileSelectionAssertion)
	}
	return envelope, payload, nil
}

func validateHostedProfileProjection(profiles []HostedProfileSnapshot, accountID, selectedProfileID string, issuedAt time.Time) (int64, error) {
	if err := validateHostedProfileDirectoryProjection(profiles, accountID, issuedAt); err != nil {
		return 0, err
	}
	for _, profile := range profiles {
		if profile.ExternalProfileID == selectedProfileID {
			return profile.PINRevision, nil
		}
	}
	return 0, errInvalidHostedProfileSnapshot
}

func validateHostedProfileDirectoryProjection(profiles []HostedProfileSnapshot, accountID string, issuedAt time.Time) error {
	if len(profiles) == 0 || len(profiles) > maxProfilesPerAccount {
		return errInvalidHostedProfileSnapshot
	}
	seenIDs := make(map[string]struct{}, len(profiles))
	seenSortOrders := make(map[int]struct{}, len(profiles))
	primaryCount := 0
	for index := range profiles {
		profile := &profiles[index]
		profile.ExternalProfileID = strings.TrimSpace(profile.ExternalProfileID)
		profile.AccountID = strings.TrimSpace(profile.AccountID)
		name, ok := normalizeProfileDisplayName(profile.DisplayName)
		if profile.ExternalProfileID == "" || utf8.RuneCountInString(profile.ExternalProfileID) > 200 || profile.AccountID != accountID || !ok {
			return errInvalidHostedProfileSnapshot
		}
		if profile.IsPrimary != profile.IsAccountAdmin || profile.SortOrder < 0 || profile.SortOrder >= len(profiles) || profile.PINRevision < 0 {
			return errInvalidHostedProfileSnapshot
		}
		// Hosted profiles are allowed to have no avatar. Older Hosted account
		// records can encode that absence as an empty avatar object instead of
		// omitting the field, so normalize both representations to nil. A
		// non-empty reference remains fully validated before it can enter the
		// server's trusted profile projection.
		if profile.Avatar != nil && strings.TrimSpace(profile.Avatar.Reference) == "" {
			profile.Avatar = nil
		} else if _, err := validateProfileAvatar(profile.Avatar); err != nil {
			return err
		}
		if _, exists := seenIDs[profile.ExternalProfileID]; exists {
			return errInvalidHostedProfileSnapshot
		}
		if _, exists := seenSortOrders[profile.SortOrder]; exists {
			return errInvalidHostedProfileSnapshot
		}
		seenIDs[profile.ExternalProfileID] = struct{}{}
		seenSortOrders[profile.SortOrder] = struct{}{}
		if profile.IsPrimary {
			primaryCount++
		}
		if profile.PolicyUpdatedAt.IsZero() || profile.PolicyUpdatedAt.After(issuedAt.Add(hostedDocumentClockSkew)) {
			return errInvalidHostedProfileSnapshot
		}
		profile.DisplayName = name
		var err error
		profile.Restrictions, err = validateProfileRestrictions(profile.Restrictions, false)
		if err != nil {
			return err
		}
	}
	if primaryCount != 1 {
		return errInvalidHostedProfileSnapshot
	}
	sort.SliceStable(profiles, func(i, j int) bool { return profiles[i].SortOrder < profiles[j].SortOrder })
	for index, profile := range profiles {
		if profile.SortOrder != index {
			return errInvalidHostedProfileSnapshot
		}
	}
	return nil
}

func (s *Server) exchangeHostedProfileSelectionEnvelope(ctx context.Context, settings RemoteAccessSettings, raw json.RawMessage) (json.RawMessage, error) {
	credential := strings.TrimSpace(s.secretSetting(remoteAccessCredentialKey))
	if credential == "" || strings.TrimSpace(settings.ServerID) == "" {
		return nil, errors.New("server credential is missing")
	}
	body, err := json.Marshal(map[string]json.RawMessage{"selectionEnvelope": raw})
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(settings.HostedBaseURL, "/") + "/api/servers/" + url.PathEscape(settings.ServerID) + "/profile-selection-exchanges"
	var response json.RawMessage
	if err := s.hostedJSONWithTimeout(ctx, http.MethodPost, endpoint, credential, body, &response, 8*time.Second); err != nil {
		return nil, err
	}
	if len(response) == 0 {
		return nil, errors.New("Hosted Services returned an empty profile selection envelope")
	}
	return response, nil
}

func hostedProfileProjectionDigest(envelope HostedProfileSelectionEnvelope) (string, error) {
	document := struct {
		AccountID       string                  `json:"accountId"`
		AccountRevision int64                   `json:"accountRevision"`
		Profiles        []HostedProfileSnapshot `json:"profiles"`
	}{AccountID: envelope.AccountID, AccountRevision: envelope.AccountRevision, Profiles: envelope.Profiles}
	payload, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

// reconcileHostedProfileSelectionEnvelopeContext applies only an envelope
// already returned by the authenticated Cloud exchange and independently
// verified by this server.
func (s *Server) reconcileHostedProfileSelectionEnvelopeContext(ctx context.Context, accountID string, envelope HostedProfileSelectionEnvelope, now time.Time) error {
	for attempt := 0; attempt < 3; attempt++ {
		revokedIDs, err := s.hostedProfilesAbsentFromEnvelopeContext(ctx, accountID, envelope)
		if err != nil {
			return err
		}
		fenced := make(map[string]bool, len(revokedIDs))
		handles := make([]profileErasureFenceHandle, 0, len(revokedIDs))
		retryAfterConcurrentErasure := false
		drainFailed := false
		for _, profileID := range revokedIDs {
			handle := s.profileRuntime.beginErasure(accountID, profileID)
			handles = append(handles, handle)
			if !handle.wait(ctx, profileErasureDrainTimeout) {
				if !handle.owner {
					retryAfterConcurrentErasure = true
					break
				}
				drainFailed = true
				break
			}
			if !handle.owner {
				retryAfterConcurrentErasure = true
				break
			}
			fenced[profileID] = true
		}
		if retryAfterConcurrentErasure {
			for _, handle := range handles {
				handle.finish()
			}
			continue
		}
		if drainFailed {
			for _, handle := range handles {
				handle.finish()
			}
			return errProfileErasureDrainTimeout
		}

		playbackSessionIDs := []string{}
		watchGroupIDs := []string{}
		for _, profileID := range revokedIDs {
			playbackSessionIDs = append(playbackSessionIDs, s.profilePlaybackSessionIDsContext(ctx, accountID, profileID)...)
			watchGroupIDs = append(watchGroupIDs, s.profileWatchGroupIDsContext(ctx, accountID, profileID)...)
		}
		err = s.reconcileHostedProfileSelectionEnvelopeFencedContext(ctx, accountID, envelope, now, fenced)
		for _, handle := range handles {
			handle.finish()
		}
		if errors.Is(err, errHostedProfileFenceRetry) {
			continue
		}
		if err == nil {
			for _, sessionID := range playbackSessionIDs {
				s.notifyPlaybackCommand(sessionID)
			}
			for _, groupID := range watchGroupIDs {
				s.notifyWatchWithFriendsGroup(groupID)
			}
		}
		return err
	}
	return errors.New("hosted profile directory changed repeatedly while applying revocations")
}

var errHostedProfileFenceRetry = errors.New("hosted profile revocation requires a runtime fence retry")

func (s *Server) hostedProfilesAbsentFromEnvelopeContext(ctx context.Context, accountID string, envelope HostedProfileSelectionEnvelope) ([]string, error) {
	activeExternalIDs := make(map[string]bool, len(envelope.Profiles))
	for _, profile := range envelope.Profiles {
		if !profile.IsPrimary {
			activeExternalIDs[profile.ExternalProfileID] = true
		}
	}
	rows, err := s.queryUserRead(ctx, `
		SELECT id, external_profile_id
		FROM profiles
		WHERE account_id = ? AND origin = 'hosted' AND is_primary = 0 AND disabled_at = ''
		ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	revokedIDs := []string{}
	for rows.Next() {
		var profileID, externalProfileID string
		if err := rows.Scan(&profileID, &externalProfileID); err != nil {
			return nil, err
		}
		if !activeExternalIDs[externalProfileID] {
			revokedIDs = append(revokedIDs, profileID)
		}
	}
	return revokedIDs, rows.Err()
}

func (s *Server) reconcileHostedProfileSelectionEnvelopeFencedContext(ctx context.Context, accountID string, envelope HostedProfileSelectionEnvelope, now time.Time, fenced map[string]bool) error {
	digest, err := hostedProfileProjectionDigest(envelope)
	if err != nil {
		return err
	}
	nowValue := now.UTC().Format(time.RFC3339Nano)
	return s.withUserTxTagged(ctx, []string{"profiles", "hosted_profile_snapshot_state", "sessions", "native_refresh_tokens", "profile_selection_grants", "profile_account_authentications"}, func(tx *sql.Tx) error {
		var origin, role, permissionsJSON, preferencesJSON, accountRating string
		if err := tx.QueryRow(`
			SELECT auth_origin, role, permissions_json, preferences_json, COALESCE(max_content_rating, '')
			FROM users WHERE id = ? AND COALESCE(disabled_at, '') = ''`, accountID).
			Scan(&origin, &role, &permissionsJSON, &preferencesJSON, &accountRating); err != nil {
			return err
		}
		if origin != "portico" {
			return errors.New("hosted profile envelopes require a Portico account membership")
		}
		var previousDigest, quarantinedAt string
		var previousRevision int64
		stateErr := tx.QueryRow(`SELECT revision, payload_digest, quarantined_at FROM hosted_profile_snapshot_state WHERE account_id = ?`, accountID).
			Scan(&previousRevision, &previousDigest, &quarantinedAt)
		if stateErr != nil && !errors.Is(stateErr, sql.ErrNoRows) {
			return stateErr
		}
		quarantined := stateErr == nil && quarantinedAt != ""
		if stateErr == nil && !quarantined {
			if envelope.AccountRevision < previousRevision || envelope.AccountRevision == previousRevision && digest != previousDigest {
				return errStaleHostedProfileSnapshot
			}
			if envelope.AccountRevision == previousRevision && digest == previousDigest {
				_, err := tx.Exec(`UPDATE hosted_profile_snapshot_state SET checked_at = ?, max_age_seconds = ?, stale_if_error_seconds = ?, quarantined_at = '' WHERE account_id = ?`,
					nowValue, int(hostedProfileFreshnessLease/time.Second), int(hostedProfileStaleIfError/time.Second), accountID)
				return err
			}
		}

		var existingPrimaryExternal string
		if err := tx.QueryRow(`SELECT external_profile_id FROM profiles WHERE id = ? AND account_id = ? AND is_primary = 1`, accountID, accountID).
			Scan(&existingPrimaryExternal); err != nil {
			return fmt.Errorf("hosted primary profile is missing: %w", err)
		}
		activeInternalIDs := map[string]struct{}{}
		for _, incoming := range envelope.Profiles {
			restrictionsJSON, err := encodeProfileRestrictions(incoming.Restrictions)
			if err != nil {
				return err
			}
			internalID := ""
			if incoming.IsPrimary {
				internalID = accountID
				if existingPrimaryExternal != "" && existingPrimaryExternal != incoming.ExternalProfileID {
					return errors.New("hosted primary profile id cannot be replaced")
				}
			} else {
				err := tx.QueryRow(`SELECT id FROM profiles WHERE account_id = ? AND external_profile_id = ?`, accountID, incoming.ExternalProfileID).Scan(&internalID)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				if errors.Is(err, sql.ErrNoRows) {
					internalID = randomID("prof")
				}
			}
			var existingPINRevision int64
			var existingPolicyUpdatedAt string
			existingErr := tx.QueryRow(`SELECT pin_revision, policy_updated_at FROM profiles WHERE id = ? AND account_id = ?`, internalID, accountID).
				Scan(&existingPINRevision, &existingPolicyUpdatedAt)
			if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
				return existingErr
			}
			if existingErr == nil && !quarantined {
				if incoming.PINRevision < existingPINRevision {
					return errStaleHostedProfileSnapshot
				}
				if previous, parseErr := time.Parse(time.RFC3339Nano, existingPolicyUpdatedAt); parseErr == nil && incoming.PolicyUpdatedAt.Before(previous) {
					return errStaleHostedProfileSnapshot
				}
			}
			activeInternalIDs[internalID] = struct{}{}
			_, err = tx.Exec(`
				INSERT INTO profiles (
					id, account_id, origin, external_profile_id, is_primary, sort_order, display_name, avatar_url,
					role, permissions_json, preferences_json, restrictions_json, pin_required, pin_revision,
					policy_updated_at, max_content_rating, max_active_sessions, remote_bitrate_limit_mbps,
					disabled_at, created_at, updated_at
				) VALUES (?, ?, 'hosted', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, '', ?, ?)
				ON CONFLICT(id) DO UPDATE SET
					origin = 'hosted', external_profile_id = excluded.external_profile_id,
					sort_order = excluded.sort_order, display_name = excluded.display_name,
					avatar_url = excluded.avatar_url, restrictions_json = excluded.restrictions_json,
					pin_required = excluded.pin_required, pin_revision = excluded.pin_revision,
					policy_updated_at = excluded.policy_updated_at, disabled_at = '', updated_at = excluded.updated_at`,
				internalID, accountID, incoming.ExternalProfileID, boolInt(incoming.IsPrimary), incoming.SortOrder,
				incoming.DisplayName, profileAvatarReference(incoming.Avatar), role, permissionsJSON, preferencesJSON, restrictionsJSON,
				boolInt(incoming.PINRequired), incoming.PINRevision, incoming.PolicyUpdatedAt.UTC().Format(time.RFC3339Nano),
				accountRating, nowValue, nowValue)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM local_profile_pin_credentials WHERE profile_id = ?`, internalID); err != nil {
				return err
			}
		}

		rows, err := tx.Query(`SELECT id FROM profiles WHERE account_id = ? AND origin = 'hosted' AND is_primary = 0 AND disabled_at = ''`, accountID)
		if err != nil {
			return err
		}
		var revokedIDs []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			if _, active := activeInternalIDs[id]; !active {
				revokedIDs = append(revokedIDs, id)
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, id := range revokedIDs {
			if !fenced[id] {
				return errHostedProfileFenceRetry
			}
			if _, err := s.eraseSecondaryProfileTx(ctx, tx, accountID, id, "hosted", nowValue); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`
			INSERT INTO hosted_profile_snapshot_state (account_id, snapshot_id, revision, payload_digest, issued_at, expires_at, applied_at, checked_at, max_age_seconds, stale_if_error_seconds, quarantined_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')
			ON CONFLICT(account_id) DO UPDATE SET snapshot_id = excluded.snapshot_id, revision = excluded.revision,
				payload_digest = excluded.payload_digest, issued_at = excluded.issued_at,
				expires_at = excluded.expires_at, applied_at = excluded.applied_at, checked_at = excluded.checked_at,
				max_age_seconds = excluded.max_age_seconds, stale_if_error_seconds = excluded.stale_if_error_seconds,
				quarantined_at = ''`,
			accountID, envelope.AssertionID, envelope.AccountRevision, digest, envelope.IssuedAt, envelope.ExpiresAt, nowValue, nowValue,
			int(hostedProfileFreshnessLease/time.Second), int(hostedProfileStaleIfError/time.Second))
		return err
	})
}
