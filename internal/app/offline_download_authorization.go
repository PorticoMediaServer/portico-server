package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
)

const (
	offlineDownloadAuthorizationPurpose = "offline-download-authorization"
	offlineDownloadAuthorizationTTL     = 30 * 24 * time.Hour
)

type OfflineDownloadAuthorizationViewerScope struct {
	ScopeKind             string `json:"scopeKind"`
	Authority             string `json:"authority"`
	AccountID             string `json:"accountId"`
	ProfileID             string `json:"profileId"`
	ServerID              string `json:"serverId"`
	AuthorizationRevision string `json:"authorizationRevision"`
}

type OfflineDownloadAuthorizationIssuer struct {
	ServerID              string `json:"serverId"`
	SigningKeyFingerprint string `json:"signingKeyFingerprint"`
}

type OfflineDownloadAuthorizationPreparation struct {
	PreparationID  string `json:"preparationId"`
	MediaID        string `json:"mediaId"`
	MediaVersionID string `json:"mediaVersionId"`
	QualityID      string `json:"qualityId"`
}

type OfflineDownloadAuthorizationArtifact struct {
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
}

type OfflineDownloadAuthorizationReceipt struct {
	Version        int                                     `json:"version"`
	Purpose        string                                  `json:"purpose"`
	ReceiptID      string                                  `json:"receiptId"`
	ViewerScope    OfflineDownloadAuthorizationViewerScope `json:"viewerScope"`
	Issuer         OfflineDownloadAuthorizationIssuer      `json:"issuer"`
	Preparation    OfflineDownloadAuthorizationPreparation `json:"preparation"`
	Artifact       OfflineDownloadAuthorizationArtifact    `json:"artifact"`
	LastVerifiedAt string                                  `json:"lastVerifiedAt"`
	VerifyBy       string                                  `json:"verifyBy"`
	Signature      string                                  `json:"signature"`
}

type OfflineDownloadAuthorizationRevalidationRequest struct {
	Receipt json.RawMessage `json:"receipt"`
}

type OfflineDownloadAuthorizationRevalidationResponse struct {
	Outcome string                               `json:"outcome"`
	Receipt *OfflineDownloadAuthorizationReceipt `json:"receipt,omitempty"`
}

func (s *Server) issueNativeDownloadPreparationGrantContext(ctx context.Context, user User, preparationID string) (MediaDownloadGrantResponse, error) {
	preparation, err := s.downloadPreparationForUserContext(ctx, user, preparationID)
	if err != nil {
		return MediaDownloadGrantResponse{}, err
	}
	if preparation.State != "ready" {
		return MediaDownloadGrantResponse{}, errDownloadPreparationNotReady
	}
	identity, err := s.systemIdentityContext(ctx)
	if err != nil {
		return MediaDownloadGrantResponse{}, err
	}
	record, err := s.downloadPreparationRecordContext(ctx, `
		WHERE dp.id = ? AND dp.server_id = ? AND dp.account_id = ? AND dp.profile_id = ? AND dp.removed_at = '' LIMIT 1`,
		preparationID, identity.ServerID, accountIDForUser(user), viewerProfileID(user))
	if err != nil {
		return MediaDownloadGrantResponse{}, err
	}
	item, err := s.getMediaDownloadSeedForUser(ctx, user, record.MediaID)
	if err != nil {
		return MediaDownloadGrantResponse{}, err
	}
	target, err := s.mediaDownloadGrantTargetContext(ctx, item, record.QualityProfile)
	if err != nil {
		return MediaDownloadGrantResponse{}, err
	}
	fingerprint, err := s.preparedDownloadArtifactFenceContext(ctx, item, target)
	if err != nil {
		return MediaDownloadGrantResponse{}, errDownloadGrantUnavailable
	}
	signingIdentity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		return MediaDownloadGrantResponse{}, err
	}
	if hook := s.offlineAuthorizationBeforeCommitHook; hook != nil {
		hook()
	}
	var response MediaDownloadGrantResponse
	err = s.withSecurityFenceTxTagged(ctx, []string{"downloads", "authorization"}, func(tx *sql.Tx) error {
		response = MediaDownloadGrantResponse{}
		scope, err := offlineDownloadAuthorizationScopeForUserTx(ctx, tx, user)
		if err != nil || !offlineDownloadPermissionActiveTx(ctx, tx, user) {
			return errDownloadGrantDenied
		}
		locked := downloadPreparationRecord{
			DownloadPreparation: DownloadPreparation{ID: record.ID, MediaTitle: record.MediaTitle},
			ServerID:            identity.ServerID, AccountID: accountIDForUser(user), ProfileID: viewerProfileID(user),
		}
		err = tx.QueryRowContext(ctx, `
			SELECT authorization_revision, media_id, quality_profile, state, media_version_id,
				version_fingerprint, artifact_sha256, size_bytes, size_kind, artifact_expires_at, removed_at
			FROM download_preparations
			WHERE id = ? AND server_id = ? AND account_id = ? AND profile_id = ? LIMIT 1`,
			record.ID, record.ServerID, record.AccountID, record.ProfileID).Scan(
			&locked.AuthorizationRevision, &locked.MediaID, &locked.QualityProfile, &locked.State,
			&locked.MediaVersionID, &locked.VersionFingerprint, &locked.ArtifactSHA256, &locked.SizeBytes,
			&locked.SizeKind, &locked.ArtifactExpiresAt, &locked.RemovedAt)
		if err != nil || locked.RemovedAt != "" || locked.State != "ready" || locked.AuthorizationRevision != scope.AuthorizationRevision {
			return errDownloadGrantDenied
		}
		if target.Profile != locked.QualityProfile || target.VersionID != locked.MediaVersionID ||
			fingerprint != locked.VersionFingerprint || !validOfflineArtifact(locked.ArtifactSHA256, locked.SizeBytes) {
			return errDownloadGrantUnavailable
		}
		bindingCurrent, err := offlineDownloadArtifactBindingCurrentTx(ctx, tx, locked, target)
		if err != nil {
			return err
		}
		if !bindingCurrent {
			return errDownloadGrantUnavailable
		}
		locked.AuthorizationRevision = scope.AuthorizationRevision
		receipt, err := signOfflineDownloadAuthorizationReceipt(locked, scope, signingIdentity, time.Now().UTC())
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		expires := now.Add(downloadGrantTTL)
		token := "ptc_dg_" + randomToken()
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM media_download_grants
			WHERE server_id = ? AND (expires_at < ? OR (consumed_at <> '' AND consumed_at < ?))`,
			identity.ServerID, now.Add(-24*time.Hour).Format(time.RFC3339), now.Add(-24*time.Hour).Format(time.RFC3339)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_download_grants (
				id, token_hash, server_id, principal_user_id, profile_id, media_id, version_kind,
				version_id, version_fingerprint, profile, issued_at, expires_at, consumed_at, authorization_revision, preparation_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
			randomID("dgr"), hashToken(token), identity.ServerID, locked.AccountID, locked.ProfileID, locked.MediaID,
			target.VersionKind, target.VersionID, target.VersionFingerprint, target.Profile, now.Format(time.RFC3339),
			expires.Format(time.RFC3339), scope.AuthorizationRevision, locked.ID); err != nil {
			return err
		}
		downloadURL := "/api/media/" + url.PathEscape(locked.MediaID) + "/download"
		query := url.Values{}
		query.Set("profile", target.Profile)
		response = MediaDownloadGrantResponse{
			DownloadURL: downloadURL + "?" + query.Encode(), GrantToken: token, ExpiresAt: expires.Format(time.RFC3339),
			Profile: target.Profile, AuthorizationReceipt: &receipt,
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errDownloadGrantDenied) {
			return MediaDownloadGrantResponse{}, errDownloadPreparationNotReady
		}
		return MediaDownloadGrantResponse{}, err
	}
	return response, nil
}

func (s *Server) handleOfflineDownloadAuthorizationRevalidation(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	var request OfflineDownloadAuthorizationRevalidationRequest
	if err := decodeStrictJSONBody(r, &request); err != nil || len(bytes.TrimSpace(request.Receipt)) == 0 {
		writeJSON(w, http.StatusOK, OfflineDownloadAuthorizationRevalidationResponse{Outcome: "invalid"})
		return
	}
	scope, err := s.offlineDownloadAuthorizationScopeForUserContext(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "offline_authorization_unavailable", "Offline download authorization could not be verified.")
		return
	}
	switch offlineReceiptViewerIdentityEnvelope(request.Receipt, scope) {
	case offlineReceiptEnvelopeInvalid:
		writeJSON(w, http.StatusOK, OfflineDownloadAuthorizationRevalidationResponse{Outcome: "invalid"})
		return
	case offlineReceiptEnvelopeForeign:
		writeJSON(w, http.StatusOK, OfflineDownloadAuthorizationRevalidationResponse{Outcome: "out-of-scope"})
		return
	}
	receipt, err := s.validateOfflineDownloadAuthorizationReceipt(r.Context(), request.Receipt)
	if err != nil {
		writeJSON(w, http.StatusOK, OfflineDownloadAuthorizationRevalidationResponse{Outcome: "invalid"})
		return
	}
	record, err := s.downloadPreparationRecordContext(r.Context(), `
		WHERE dp.id = ? AND dp.server_id = ? AND dp.account_id = ? AND dp.profile_id = ? LIMIT 1`,
		receipt.Preparation.PreparationID, s.localSystemServerIDForUserContext(r.Context(), user), accountIDForUser(user), viewerProfileID(user))
	if err != nil {
		writeJSON(w, http.StatusOK, OfflineDownloadAuthorizationRevalidationResponse{Outcome: "revoked"})
		return
	}
	if record.RemovedAt != "" || record.State == "cancelled" || record.State == "unavailable" || !user.Permissions["downloadMedia"] {
		writeJSON(w, http.StatusOK, OfflineDownloadAuthorizationRevalidationResponse{Outcome: "revoked"})
		return
	}
	if !offlineReceiptMatchesPreparation(receipt, record) {
		writeJSON(w, http.StatusOK, OfflineDownloadAuthorizationRevalidationResponse{Outcome: "invalid"})
		return
	}
	item, err := s.getMediaDownloadSeedForUser(r.Context(), user, record.MediaID)
	if err != nil {
		writeJSON(w, http.StatusOK, OfflineDownloadAuthorizationRevalidationResponse{Outcome: "revoked"})
		return
	}
	target, err := s.mediaDownloadGrantTargetContext(r.Context(), item, record.QualityProfile)
	if err != nil {
		writeJSON(w, http.StatusOK, OfflineDownloadAuthorizationRevalidationResponse{Outcome: "revoked"})
		return
	}
	fingerprint, err := s.preparedDownloadArtifactFenceContext(r.Context(), item, target)
	if err != nil || target.VersionID != record.MediaVersionID || fingerprint != record.VersionFingerprint {
		writeJSON(w, http.StatusOK, OfflineDownloadAuthorizationRevalidationResponse{Outcome: "invalid"})
		return
	}
	signingIdentity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "offline_authorization_unavailable", "Offline download authorization could not be verified.")
		return
	}
	if hook := s.offlineAuthorizationBeforeCommitHook; hook != nil {
		hook()
	}
	response := OfflineDownloadAuthorizationRevalidationResponse{}
	err = s.withSecurityFenceTxTagged(r.Context(), []string{"downloads", "authorization"}, func(tx *sql.Tx) error {
		response = OfflineDownloadAuthorizationRevalidationResponse{}
		currentScope, err := offlineDownloadAuthorizationScopeForUserTx(r.Context(), tx, user)
		if err != nil {
			return err
		}
		switch offlineReceiptViewerIdentityEnvelope(request.Receipt, currentScope) {
		case offlineReceiptEnvelopeInvalid:
			response.Outcome = "invalid"
			return nil
		case offlineReceiptEnvelopeForeign:
			response.Outcome = "out-of-scope"
			return nil
		}
		if !offlineDownloadPermissionActiveTx(r.Context(), tx, user) {
			response.Outcome = "revoked"
			return nil
		}
		locked := downloadPreparationRecord{
			DownloadPreparation: DownloadPreparation{ID: record.ID, MediaTitle: record.MediaTitle},
			ServerID:            record.ServerID, AccountID: record.AccountID, ProfileID: record.ProfileID,
		}
		err = tx.QueryRowContext(r.Context(), `
			SELECT authorization_revision, media_id, quality_profile, state, media_version_id,
				version_fingerprint, artifact_sha256, size_bytes, size_kind, artifact_expires_at, removed_at
			FROM download_preparations
			WHERE id = ? AND server_id = ? AND account_id = ? AND profile_id = ? LIMIT 1`,
			record.ID, record.ServerID, record.AccountID, record.ProfileID).Scan(
			&locked.AuthorizationRevision, &locked.MediaID, &locked.QualityProfile, &locked.State,
			&locked.MediaVersionID, &locked.VersionFingerprint, &locked.ArtifactSHA256, &locked.SizeBytes,
			&locked.SizeKind, &locked.ArtifactExpiresAt, &locked.RemovedAt)
		if errors.Is(err, sql.ErrNoRows) || locked.RemovedAt != "" || locked.State == "cancelled" || locked.State == "unavailable" {
			response.Outcome = "revoked"
			return nil
		}
		if err != nil {
			return err
		}
		if !offlineReceiptMatchesPreparation(receipt, locked) || target.Profile != locked.QualityProfile ||
			target.VersionID != locked.MediaVersionID || fingerprint != locked.VersionFingerprint ||
			!validOfflineArtifact(locked.ArtifactSHA256, locked.SizeBytes) {
			response.Outcome = "invalid"
			return nil
		}
		bindingCurrent, err := offlineDownloadArtifactBindingCurrentTx(r.Context(), tx, locked, target)
		if err != nil {
			return err
		}
		if !bindingCurrent {
			response.Outcome = "invalid"
			return nil
		}
		locked.AuthorizationRevision = currentScope.AuthorizationRevision
		replacement, err := signOfflineDownloadAuthorizationReceipt(locked, currentScope, signingIdentity, time.Now().UTC())
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(r.Context(), `
			UPDATE download_preparations SET authorization_revision = ?, updated_at = ?
			WHERE id = ? AND server_id = ? AND account_id = ? AND profile_id = ? AND state = 'ready'
				AND media_id = ? AND quality_profile = ? AND media_version_id = ? AND version_fingerprint = ?
				AND artifact_sha256 = ? AND size_bytes = ? AND removed_at = ''`,
			currentScope.AuthorizationRevision, now, locked.ID, locked.ServerID, locked.AccountID, locked.ProfileID,
			locked.MediaID, locked.QualityProfile, locked.MediaVersionID, locked.VersionFingerprint,
			locked.ArtifactSHA256, locked.SizeBytes)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return sql.ErrNoRows
		}
		response = OfflineDownloadAuthorizationRevalidationResponse{Outcome: "valid-replacement", Receipt: &replacement}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "offline_authorization_unavailable", "Offline download authorization could not be verified.")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func offlineDownloadArtifactBindingCurrentTx(ctx context.Context, tx *sql.Tx, record downloadPreparationRecord, target mediaDownloadGrantTarget) (bool, error) {
	if tx == nil || target.Profile != record.QualityProfile || target.VersionID != record.MediaVersionID ||
		!validOfflineArtifact(record.ArtifactSHA256, record.SizeBytes) {
		return false, nil
	}
	var count int
	switch target.VersionKind {
	case "optimized":
		err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM optimized_versions
			WHERE id = ? AND media_id = ? AND profile = ? AND state = 'ready'
				AND artifact_sha256 = ? AND size_bytes = ?`,
			target.VersionID, record.MediaID, record.QualityProfile, record.ArtifactSHA256, record.SizeBytes).Scan(&count)
		return count == 1, err
	case "source":
		err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM media_files
			WHERE id = ? AND media_id = ? AND available = 1`, target.VersionID, record.MediaID).Scan(&count)
		if err != nil || count == 1 {
			return count == 1, err
		}
		// HTTP and remote-storage sources need not have a media_files row. Their
		// opaque source-* version is derived from the authorized locator and is
		// still fenced by the immutable preparation fingerprint and verified
		// artifact digest.
		if !strings.HasPrefix(target.VersionID, "source-") {
			return false, nil
		}
		var sourceURL string
		err = tx.QueryRowContext(ctx, `SELECT source_url FROM media_items WHERE id = ?`, record.MediaID).Scan(&sourceURL)
		return strings.TrimSpace(sourceURL) != "", err
	default:
		return false, nil
	}
}

func decodeStrictJSONBody(r *http.Request, value any) error {
	if r == nil || r.Body == nil {
		return io.EOF
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func (s *Server) offlineDownloadAuthorizationScopeForUserContext(ctx context.Context, user User) (OfflineDownloadAuthorizationViewerScope, error) {
	settings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return OfflineDownloadAuthorizationViewerScope{}, err
	}
	serverID, err := s.publicServerIDForAuthProviderContext(ctx, settings, user.AuthProvider)
	if err != nil {
		return OfflineDownloadAuthorizationViewerScope{}, err
	}
	accountID, profileID, err := s.publicViewerIdentityForUserContext(ctx, user, user.AuthProvider)
	if err != nil {
		return OfflineDownloadAuthorizationViewerScope{}, err
	}
	revision, err := s.authorizationRevisionForUserContextStrict(ctx, user)
	if err != nil {
		return OfflineDownloadAuthorizationViewerScope{}, err
	}
	return OfflineDownloadAuthorizationViewerScope{
		ScopeKind: "server-bound", Authority: viewerAuthorityForAuthProvider(user.AuthProvider),
		AccountID: accountID, ProfileID: profileID, ServerID: serverID, AuthorizationRevision: revision,
	}, nil
}

func offlineDownloadAuthorizationScopeForUserTx(ctx context.Context, tx *sql.Tx, user User) (OfflineDownloadAuthorizationViewerScope, error) {
	provider := normalizeAuthProvider(user.AuthProvider)
	if provider == "" {
		return OfflineDownloadAuthorizationViewerScope{}, errors.New("authentication provider is unavailable")
	}
	internalAccountID, internalProfileID := accountIDForUser(user), viewerProfileID(user)
	accountID, profileID := internalAccountID, internalProfileID
	if provider == "portico" {
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(u.portico_user_id, ''), COALESCE(p.external_profile_id, '')
			FROM users u JOIN profiles p ON p.account_id = u.id
			WHERE u.id = ? AND p.id = ? AND p.origin = 'hosted' AND COALESCE(u.disabled_at, '') = '' AND COALESCE(p.disabled_at, '') = ''`,
			internalAccountID, internalProfileID).Scan(&accountID, &profileID); err != nil {
			return OfflineDownloadAuthorizationViewerScope{}, err
		}
	}
	var settingsJSON string
	settingKey := "identity"
	if provider == "portico" {
		settingKey = remoteAccessSettingsKey
	}
	if err := tx.QueryRowContext(ctx, `SELECT value_json FROM settings WHERE key = ?`, settingKey).Scan(&settingsJSON); err != nil {
		return OfflineDownloadAuthorizationViewerScope{}, err
	}
	var settings map[string]any
	if json.Unmarshal([]byte(settingsJSON), &settings) != nil {
		return OfflineDownloadAuthorizationViewerScope{}, errors.New("server identity setting is invalid")
	}
	serverID := strings.TrimSpace(settingString(settings, "serverId", ""))
	revision, err := authorizationRevisionForUserTx(ctx, tx, user)
	if err != nil || accountID == "" || profileID == "" || serverID == "" {
		return OfflineDownloadAuthorizationViewerScope{}, errors.New("server-bound viewer scope is unavailable")
	}
	return OfflineDownloadAuthorizationViewerScope{
		ScopeKind: "server-bound", Authority: viewerAuthorityForAuthProvider(provider),
		AccountID: accountID, ProfileID: profileID, ServerID: serverID, AuthorizationRevision: revision,
	}, nil
}

func offlineDownloadPermissionActiveTx(ctx context.Context, tx *sql.Tx, user User) bool {
	var allowed int
	err := tx.QueryRowContext(ctx, `
		SELECT CASE WHEN COALESCE(u.disabled_at, '') = '' AND COALESCE(p.disabled_at, '') = ''
			AND COALESCE(json_extract(u.permissions_json, '$.downloadMedia'), 0) = 1 THEN 1 ELSE 0 END
		FROM users u JOIN profiles p ON p.account_id = u.id WHERE u.id = ? AND p.id = ?`,
		accountIDForUser(user), viewerProfileID(user)).Scan(&allowed)
	return err == nil && allowed == 1 && user.Permissions["downloadMedia"]
}

func (s *Server) localSystemServerIDForUserContext(ctx context.Context, user User) string {
	identity, err := s.systemIdentityContext(ctx)
	if err != nil {
		return ""
	}
	return identity.ServerID
}

func signOfflineDownloadAuthorizationReceipt(record downloadPreparationRecord, scope OfflineDownloadAuthorizationViewerScope, identity serverIdentity, now time.Time) (OfflineDownloadAuthorizationReceipt, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	if scope.AuthorizationRevision != record.AuthorizationRevision || len(identity.PrivateKey) != ed25519.PrivateKeySize || identity.Fingerprint == "" ||
		record.MediaVersionID == "" || record.VersionFingerprint == "" || !validOfflineArtifact(record.ArtifactSHA256, record.SizeBytes) {
		return OfflineDownloadAuthorizationReceipt{}, errors.New("download preparation is not bound to a verified artifact")
	}
	receipt := OfflineDownloadAuthorizationReceipt{
		Version: 1, Purpose: offlineDownloadAuthorizationPurpose, ReceiptID: randomID("odr"), ViewerScope: scope,
		Issuer: OfflineDownloadAuthorizationIssuer{ServerID: scope.ServerID, SigningKeyFingerprint: identity.Fingerprint},
		Preparation: OfflineDownloadAuthorizationPreparation{
			PreparationID: record.ID, MediaID: record.MediaID, MediaVersionID: record.MediaVersionID, QualityID: record.QualityProfile,
		},
		Artifact:       OfflineDownloadAuthorizationArtifact{SHA256: record.ArtifactSHA256, SizeBytes: record.SizeBytes},
		LastVerifiedAt: now.Format(time.RFC3339Nano), VerifyBy: now.Add(offlineDownloadAuthorizationTTL).Format(time.RFC3339Nano),
	}
	payload, err := canonicalOfflineDownloadAuthorizationPayload(receipt)
	if err != nil {
		return OfflineDownloadAuthorizationReceipt{}, err
	}
	receipt.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(identity.PrivateKey, payload))
	return receipt, nil
}

func (s *Server) validateOfflineDownloadAuthorizationReceipt(ctx context.Context, raw json.RawMessage) (OfflineDownloadAuthorizationReceipt, error) {
	receipt, err := decodeOfflineDownloadAuthorizationReceipt(raw)
	if err != nil {
		return OfflineDownloadAuthorizationReceipt{}, err
	}
	identity, err := s.loadOrCreateServerIdentity()
	if err != nil {
		return OfflineDownloadAuthorizationReceipt{}, err
	}
	if receipt.Issuer.SigningKeyFingerprint != identity.Fingerprint {
		return OfflineDownloadAuthorizationReceipt{}, errors.New("offline receipt signing key is not current")
	}
	signature, err := decodeCanonicalBase64URL(receipt.Signature, ed25519.SignatureSize)
	if err != nil {
		return OfflineDownloadAuthorizationReceipt{}, err
	}
	payload, err := canonicalOfflineDownloadAuthorizationPayload(receipt)
	if err != nil || !ed25519.Verify(identity.PublicKey, payload, signature) {
		return OfflineDownloadAuthorizationReceipt{}, errors.New("offline receipt signature is invalid")
	}
	return receipt, nil
}

func decodeOfflineDownloadAuthorizationReceipt(raw json.RawMessage) (OfflineDownloadAuthorizationReceipt, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var receipt OfflineDownloadAuthorizationReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return OfflineDownloadAuthorizationReceipt{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return OfflineDownloadAuthorizationReceipt{}, errors.New("offline receipt must contain one JSON value")
	}
	if receipt.Version != 1 || receipt.Purpose != offlineDownloadAuthorizationPurpose ||
		receipt.ViewerScope.ScopeKind != "server-bound" || (receipt.ViewerScope.Authority != "hosted" && receipt.ViewerScope.Authority != "local") ||
		receipt.Issuer.ServerID != receipt.ViewerScope.ServerID {
		return OfflineDownloadAuthorizationReceipt{}, errors.New("offline receipt schema is invalid")
	}
	for _, value := range []string{
		receipt.ReceiptID, receipt.ViewerScope.AccountID, receipt.ViewerScope.ProfileID, receipt.ViewerScope.ServerID,
		receipt.ViewerScope.AuthorizationRevision, receipt.Issuer.ServerID, receipt.Preparation.PreparationID,
		receipt.Preparation.MediaID, receipt.Preparation.MediaVersionID, receipt.Preparation.QualityID,
	} {
		if utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 128 {
			return OfflineDownloadAuthorizationReceipt{}, errors.New("offline receipt identity is invalid")
		}
	}
	if _, err := decodeCanonicalFingerprint(receipt.Issuer.SigningKeyFingerprint); err != nil {
		return OfflineDownloadAuthorizationReceipt{}, err
	}
	if !validOfflineArtifact(receipt.Artifact.SHA256, receipt.Artifact.SizeBytes) {
		return OfflineDownloadAuthorizationReceipt{}, errors.New("offline receipt artifact is invalid")
	}
	lastVerifiedAt, err := parseCanonicalUTCTimestamp(receipt.LastVerifiedAt)
	if err != nil {
		return OfflineDownloadAuthorizationReceipt{}, err
	}
	verifyBy, err := parseCanonicalUTCTimestamp(receipt.VerifyBy)
	if err != nil || !verifyBy.Equal(lastVerifiedAt.Add(offlineDownloadAuthorizationTTL)) {
		return OfflineDownloadAuthorizationReceipt{}, errors.New("offline receipt verification window is invalid")
	}
	if _, err := decodeCanonicalBase64URL(receipt.Signature, ed25519.SignatureSize); err != nil {
		return OfflineDownloadAuthorizationReceipt{}, err
	}
	return receipt, nil
}

func canonicalOfflineDownloadAuthorizationPayload(receipt OfflineDownloadAuthorizationReceipt) ([]byte, error) {
	receipt.Signature = ""
	raw, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	var unsigned map[string]any
	if err := json.Unmarshal(raw, &unsigned); err != nil {
		return nil, err
	}
	delete(unsigned, "signature")
	raw, err = json.Marshal(unsigned)
	if err != nil {
		return nil, err
	}
	return jsoncanonicalizer.Transform(raw)
}

type offlineReceiptEnvelopeDisposition uint8

const (
	offlineReceiptEnvelopeInvalid offlineReceiptEnvelopeDisposition = iota
	offlineReceiptEnvelopeForeign
	offlineReceiptEnvelopeMatching
)

func offlineReceiptViewerIdentityEnvelope(raw json.RawMessage, scope OfflineDownloadAuthorizationViewerScope) offlineReceiptEnvelopeDisposition {
	var envelope struct {
		ViewerScope struct {
			ScopeKind             string `json:"scopeKind"`
			Authority             string `json:"authority"`
			AccountID             string `json:"accountId"`
			ProfileID             string `json:"profileId"`
			ServerID              string `json:"serverId"`
			AuthorizationRevision string `json:"authorizationRevision"`
		} `json:"viewerScope"`
		Issuer struct {
			ServerID string `json:"serverId"`
		} `json:"issuer"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return offlineReceiptEnvelopeInvalid
	}
	identityValues := []string{
		envelope.ViewerScope.AccountID,
		envelope.ViewerScope.ProfileID,
		envelope.ViewerScope.ServerID,
		envelope.ViewerScope.AuthorizationRevision,
		envelope.Issuer.ServerID,
	}
	if envelope.ViewerScope.ScopeKind != "server-bound" ||
		(envelope.ViewerScope.Authority != "hosted" && envelope.ViewerScope.Authority != "local") ||
		envelope.Issuer.ServerID != envelope.ViewerScope.ServerID {
		return offlineReceiptEnvelopeInvalid
	}
	for _, value := range identityValues {
		if utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 128 {
			return offlineReceiptEnvelopeInvalid
		}
	}
	if envelope.ViewerScope.Authority == scope.Authority &&
		envelope.ViewerScope.AccountID == scope.AccountID && envelope.ViewerScope.ProfileID == scope.ProfileID &&
		envelope.ViewerScope.ServerID == scope.ServerID && envelope.Issuer.ServerID == scope.ServerID {
		return offlineReceiptEnvelopeMatching
	}
	return offlineReceiptEnvelopeForeign
}

func offlineReceiptMatchesPreparation(receipt OfflineDownloadAuthorizationReceipt, record downloadPreparationRecord) bool {
	return receipt.Preparation.PreparationID == record.ID && receipt.Preparation.MediaID == record.MediaID &&
		receipt.Preparation.MediaVersionID == record.MediaVersionID && receipt.Preparation.QualityID == record.QualityProfile &&
		receipt.Artifact.SHA256 == record.ArtifactSHA256 && receipt.Artifact.SizeBytes == record.SizeBytes
}

func validOfflineArtifact(digest string, size int64) bool {
	if len(digest) != 64 || strings.ToLower(digest) != digest || size < 1 || size > 9007199254740991 {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == 32
}

func decodeCanonicalFingerprint(value string) ([]byte, error) {
	if !strings.HasPrefix(value, "sha256:") || strings.Count(value, ":") != 1 {
		return nil, errors.New("offline receipt key fingerprint is invalid")
	}
	return decodeCanonicalBase64URL(strings.TrimPrefix(value, "sha256:"), 32)
}

func decodeCanonicalBase64URL(value string, expected int) ([]byte, error) {
	if value == "" || strings.Contains(value, "=") || strings.ContainsAny(value, "+/") {
		return nil, errors.New("offline receipt base64url value is invalid")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != expected || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, errors.New("offline receipt base64url value is invalid")
	}
	return decoded, nil
}

func parseCanonicalUTCTimestamp(value string) (time.Time, error) {
	if !strings.HasSuffix(value, "Z") {
		return time.Time{}, errors.New("offline receipt timestamp is not UTC")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC {
		return time.Time{}, errors.New("offline receipt timestamp is invalid")
	}
	return parsed, nil
}
