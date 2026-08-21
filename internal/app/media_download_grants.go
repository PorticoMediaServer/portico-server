package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/optimized"
)

const (
	downloadGrantQueryParameter           = "download_grant"
	downloadGrantCookieName               = "portico_download_grant"
	downloadGrantTTL                      = 15 * time.Minute
	downloadGrantReplayWindow             = 2 * time.Minute
	downloadPreparationTombstoneRetention = 7 * 24 * time.Hour
	downloadPreparationTerminalRetention  = 30 * 24 * time.Hour
)

var errDownloadGrantDenied = errors.New("download grant is invalid, expired, revoked, or out of scope")

type MediaDownloadGrantRequest struct {
	Profile string `json:"profile,omitempty"`
}

type MediaDownloadGrantResponse struct {
	DownloadURL string `json:"downloadUrl"`
	GrantToken  string `json:"grantToken,omitempty"`
	ExpiresAt   string `json:"expiresAt"`
	Profile     string `json:"profile"`
}

type mediaDownloadGrantTarget struct {
	Profile            string
	VersionKind        string
	VersionID          string
	VersionFingerprint string
}

type mediaDownloadGrantRecord struct {
	ID                    string
	ServerID              string
	PrincipalUserID       string
	ProfileID             string
	MediaID               string
	VersionKind           string
	VersionID             string
	VersionFingerprint    string
	Profile               string
	ExpiresAt             string
	ConsumedAt            string
	AuthorizationRevision string
	PreparationID         string
}

func (s *Server) handleMediaDownloadGrantRoute(w http.ResponseWriter, r *http.Request, user User) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/media/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "download-grants" {
		writeError(w, http.StatusNotFound, "not_found", "Media download grant route was not found.")
		return
	}
	if !user.Permissions["downloadMedia"] {
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to download media.")
		return
	}
	var request MediaDownloadGrantRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	mediaID := parts[0]
	grant, target, item, err := s.issueMediaDownloadGrant(r.Context(), user, mediaID, request.Profile)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeError(w, http.StatusNotFound, "media_not_found", "Media item was not found.")
		case errors.Is(err, errInvalidDownloadGrantProfile):
			writeError(w, http.StatusBadRequest, "invalid_download_profile", "Choose the original source or a listed optimized quality.")
		case errors.Is(err, errUnsupportedPlaybackSource), errors.Is(err, errDownloadGrantUnavailable):
			writeError(w, http.StatusConflict, "download_unavailable", "That media version is not currently available for download.")
		default:
			writeError(w, http.StatusInternalServerError, "download_grant_failed", "Unable to prepare this browser download.")
		}
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	setMediaDownloadGrantCookie(w, r, item.ID, grant)
	s.recordAudit(r, user, "media.download_grant_issued", "media", item.ID, "info", map[string]string{
		"profile":     target.Profile,
		"versionKind": target.VersionKind,
		"expiresAt":   grant.ExpiresAt,
	})
	// Browser callers receive the capability only in an HttpOnly cookie. Native
	// offline clients use the preparation-grant endpoint, which intentionally
	// returns the token for its PorticoDownload authorization header.
	grant.GrantToken = ""
	writeJSON(w, http.StatusCreated, grant)
}

var (
	errInvalidDownloadGrantProfile = errors.New("download profile is not supported")
	errDownloadGrantUnavailable    = errors.New("download version is unavailable")
)

func (s *Server) issueMediaDownloadGrant(ctx context.Context, user User, mediaID, requestedProfile string) (MediaDownloadGrantResponse, mediaDownloadGrantTarget, MediaItem, error) {
	return s.issueMediaDownloadGrantForPreparation(ctx, user, mediaID, requestedProfile, "")
}

func (s *Server) issueMediaDownloadGrantForPreparation(ctx context.Context, user User, mediaID, requestedProfile, preparationID string) (MediaDownloadGrantResponse, mediaDownloadGrantTarget, MediaItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(user.ID) == "" || !user.Permissions["downloadMedia"] {
		return MediaDownloadGrantResponse{}, mediaDownloadGrantTarget{}, MediaItem{}, errDownloadGrantDenied
	}
	item, err := s.getMediaDownloadSeedForUser(ctx, user, strings.TrimSpace(mediaID))
	if err != nil {
		return MediaDownloadGrantResponse{}, mediaDownloadGrantTarget{}, MediaItem{}, err
	}
	target, err := s.mediaDownloadGrantTargetContext(ctx, item, requestedProfile)
	if err != nil {
		return MediaDownloadGrantResponse{}, mediaDownloadGrantTarget{}, MediaItem{}, err
	}
	identity, err := s.systemIdentityContext(ctx)
	if err != nil {
		return MediaDownloadGrantResponse{}, mediaDownloadGrantTarget{}, MediaItem{}, err
	}
	now := time.Now().UTC()
	expires := now.Add(downloadGrantTTL)
	token := "ptc_dg_" + randomToken()
	grantID := randomID("dgr")
	authorizationRevision := s.authorizationRevisionForUserContext(ctx, user)
	err = s.withUserTxTagged(ctx, nil, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM media_download_grants
			WHERE server_id = ? AND (expires_at < ? OR (consumed_at <> '' AND consumed_at < ?))`,
			identity.ServerID, now.Add(-24*time.Hour).Format(time.RFC3339), now.Add(-24*time.Hour).Format(time.RFC3339)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO media_download_grants (
				id, token_hash, server_id, principal_user_id, profile_id, media_id, version_kind,
				version_id, version_fingerprint, profile, issued_at, expires_at, consumed_at, authorization_revision, preparation_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
			grantID, hashToken(token), identity.ServerID, accountIDForUser(user), viewerProfileID(user), item.ID, target.VersionKind,
			target.VersionID, target.VersionFingerprint, target.Profile,
			now.Format(time.RFC3339), expires.Format(time.RFC3339), authorizationRevision, strings.TrimSpace(preparationID))
		return err
	})
	if err != nil {
		return MediaDownloadGrantResponse{}, mediaDownloadGrantTarget{}, MediaItem{}, err
	}
	downloadURL := "/api/media/" + url.PathEscape(item.ID) + "/download"
	query := url.Values{}
	query.Set("profile", target.Profile)
	downloadURL += "?" + query.Encode()
	return MediaDownloadGrantResponse{
		DownloadURL: downloadURL,
		GrantToken:  token,
		ExpiresAt:   expires.Format(time.RFC3339),
		Profile:     target.Profile,
	}, target, item, nil
}

func (s *Server) mediaDownloadGrantTargetContext(ctx context.Context, item MediaItem, requestedProfile string) (mediaDownloadGrantTarget, error) {
	profile, err := s.normalizeMediaDownloadGrantProfile(requestedProfile)
	if err != nil {
		return mediaDownloadGrantTarget{}, err
	}
	source, err := s.downloadSourceForRequestContext(ctx, item, profile)
	if err != nil {
		if errors.Is(err, errUnsupportedPlaybackSource) || errors.Is(err, errUnsupportedPlaybackScheme) || strings.Contains(err.Error(), "optimized version is not available") {
			return mediaDownloadGrantTarget{}, fmt.Errorf("%w: %v", errDownloadGrantUnavailable, err)
		}
		return mediaDownloadGrantTarget{}, err
	}
	if source.path != "" && source.sourceKind != "optimized" {
		info, statErr := os.Stat(filepath.Clean(source.path))
		if statErr != nil || info.IsDir() {
			return mediaDownloadGrantTarget{}, errDownloadGrantUnavailable
		}
	}
	versionKind := "source"
	versionID := ""
	if profile == "source" {
		versionID = sourceDownloadGrantVersionID(item, source)
	} else {
		versionKind = "optimized"
		versionID = strings.TrimSpace(source.versionID)
		if versionID == "" {
			return mediaDownloadGrantTarget{}, errDownloadGrantUnavailable
		}
	}
	return mediaDownloadGrantTarget{
		Profile:            profile,
		VersionKind:        versionKind,
		VersionID:          versionID,
		VersionFingerprint: downloadGrantVersionFingerprint(item.ID, profile, versionKind, versionID, source),
	}, nil
}

func sourceDownloadGrantVersionID(item MediaItem, source mediaDownloadSource) string {
	location := strings.TrimSpace(source.path)
	if location == "" {
		location = strings.TrimSpace(source.sourceURL)
	}
	for _, file := range item.MediaFiles {
		if strings.TrimSpace(file.ID) == "" {
			continue
		}
		fileLocation := strings.TrimSpace(file.Path)
		if file.Selected || fileLocation == location || (source.path != "" && filepath.Clean(fileLocation) == filepath.Clean(location)) {
			return file.ID
		}
	}
	fingerprint := hashToken(strings.Join([]string{item.ID, location}, "\x00"))
	return "source-" + fingerprint[:24]
}

func (s *Server) normalizeMediaDownloadGrantProfile(value string) (string, error) {
	raw := strings.ToLower(strings.TrimSpace(value))
	raw = strings.ReplaceAll(raw, " ", "-")
	switch raw {
	case "", "source", "original":
		return "source", nil
	case "default", "optimized":
		profile := strings.TrimSpace(s.optimizedVersionSettings().DefaultProfile)
		if _, ok := optimized.Lookup(profile); !ok {
			return "", fmt.Errorf("%w: choose the source or a listed optimized profile", errInvalidDownloadGrantProfile)
		}
		return profile, nil
	default:
		if _, ok := optimized.Lookup(raw); ok {
			return raw, nil
		}
		return "", fmt.Errorf("%w: choose the source or a listed optimized profile", errInvalidDownloadGrantProfile)
	}
}

func downloadGrantVersionFingerprint(mediaID, profile, versionKind, versionID string, source mediaDownloadSource) string {
	location := strings.TrimSpace(source.path)
	if location == "" {
		location = strings.TrimSpace(source.sourceURL)
	}
	modTime := ""
	size := source.sizeBytes
	if source.path != "" && source.sourceKind != "optimized" {
		if info, err := os.Stat(filepath.Clean(source.path)); err == nil && !info.IsDir() {
			modTime = info.ModTime().UTC().Format(time.RFC3339Nano)
			size = info.Size()
		}
	}
	material := strings.Join([]string{mediaID, profile, versionKind, versionID, location, strconv.FormatInt(size, 10), modTime}, "\x00")
	return hashToken(material)
}

func mediaDownloadGrantFromRequest(r *http.Request) string {
	if r == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return ""
	}
	if _, isDownload := mediaDownloadIDForRequest(r); !isDownload {
		return ""
	}
	token := ""
	if authorization := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(authorization, "PorticoDownload ") {
		token = strings.TrimSpace(strings.TrimPrefix(authorization, "PorticoDownload "))
	} else if cookie, err := r.Cookie(downloadGrantCookieName); err == nil {
		token = strings.TrimSpace(cookie.Value)
	}
	if token == "" {
		// A marker keeps download byte routes on the download-capability branch
		// of withMediaResourceAuth. It is intentionally not a valid token, so a
		// normal account cookie or bearer can never bypass preparation scope.
		return "missing_download_grant"
	}
	return token
}

func setMediaDownloadGrantCookie(w http.ResponseWriter, r *http.Request, mediaID string, grant MediaDownloadGrantResponse) {
	if strings.TrimSpace(grant.GrantToken) == "" || strings.TrimSpace(mediaID) == "" {
		return
	}
	expires, _ := time.Parse(time.RFC3339, grant.ExpiresAt)
	http.SetCookie(w, &http.Cookie{Name: downloadGrantCookieName, Value: grant.GrantToken, Path: "/api/media/" + url.PathEscape(mediaID) + "/download", Expires: expires, MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true, Secure: requestUsesTLS(r), SameSite: http.SameSiteStrictMode})
}

func (s *Server) consumeMediaDownloadGrant(r *http.Request) (User, error) {
	token := mediaDownloadGrantFromRequest(r)
	if !strings.HasPrefix(token, "ptc_dg_") {
		return User{}, errDownloadGrantDenied
	}
	mediaID, ok := mediaDownloadIDForRequest(r)
	if !ok {
		return User{}, errDownloadGrantDenied
	}
	identity, err := s.systemIdentityContext(r.Context())
	if err != nil {
		return User{}, errDownloadGrantDenied
	}
	var grant mediaDownloadGrantRecord
	err = s.queryUserRow(r.Context(), `
		SELECT id, server_id, principal_user_id, profile_id, media_id, version_kind, version_id,
			version_fingerprint, profile, expires_at, consumed_at, authorization_revision, preparation_id
		FROM media_download_grants
		WHERE token_hash = ? AND server_id = ?
		LIMIT 1`, hashToken(token), identity.ServerID).Scan(
		&grant.ID, &grant.ServerID, &grant.PrincipalUserID, &grant.ProfileID, &grant.MediaID, &grant.VersionKind,
		&grant.VersionID, &grant.VersionFingerprint, &grant.Profile, &grant.ExpiresAt, &grant.ConsumedAt, &grant.AuthorizationRevision, &grant.PreparationID,
	)
	if err != nil || grant.MediaID != mediaID {
		return User{}, errDownloadGrantDenied
	}
	now := time.Now().UTC()
	expiresAt, err := time.Parse(time.RFC3339, grant.ExpiresAt)
	if err != nil || !now.Before(expiresAt) {
		return User{}, errDownloadGrantDenied
	}
	if grant.ConsumedAt != "" {
		consumedAt, parseErr := time.Parse(time.RFC3339, grant.ConsumedAt)
		if parseErr != nil || now.Sub(consumedAt) > downloadGrantReplayWindow {
			return User{}, errDownloadGrantDenied
		}
	}
	requestedProfile, err := s.normalizeMediaDownloadGrantProfile(r.URL.Query().Get("profile"))
	if err != nil || requestedProfile != grant.Profile {
		return User{}, errDownloadGrantDenied
	}
	if identity.ServerID != grant.ServerID {
		return User{}, errDownloadGrantDenied
	}
	var disabledAt string
	if err := s.queryUserRow(r.Context(), `SELECT COALESCE(disabled_at, '') FROM users WHERE id = ?`, grant.PrincipalUserID).Scan(&disabledAt); err != nil || disabledAt != "" {
		return User{}, errDownloadGrantDenied
	}
	user, err := s.getUser(grant.PrincipalUserID)
	if err != nil || !user.Permissions["downloadMedia"] {
		return User{}, errDownloadGrantDenied
	}
	user.AccountID = grant.PrincipalUserID
	user.ProfileID = grant.ProfileID
	user = s.hydratePlaybackVisibilityUserContext(r.Context(), user)
	if !user.Permissions["downloadMedia"] {
		return User{}, errDownloadGrantDenied
	}
	if grant.AuthorizationRevision == "" || grant.AuthorizationRevision != s.authorizationRevisionForUserContext(r.Context(), user) {
		return User{}, errDownloadGrantDenied
	}
	if grant.PreparationID != "" {
		var preparationState string
		if err := s.queryUserRow(r.Context(), `
			SELECT state FROM download_preparations
			WHERE id = ? AND server_id = ? AND account_id = ? AND profile_id = ?
				AND media_id = ? AND quality_profile = ? AND removed_at = ''
			LIMIT 1`, grant.PreparationID, grant.ServerID, grant.PrincipalUserID, grant.ProfileID, grant.MediaID, grant.Profile).Scan(&preparationState); err != nil || preparationState != "ready" {
			return User{}, errDownloadGrantDenied
		}
	}
	item, err := s.getMediaDownloadSeedForUser(r.Context(), user, grant.MediaID)
	if err != nil {
		return User{}, errDownloadGrantDenied
	}
	target, err := s.mediaDownloadGrantTargetContext(r.Context(), item, grant.Profile)
	if err != nil || target.VersionKind != grant.VersionKind || target.VersionID != grant.VersionID || target.VersionFingerprint != grant.VersionFingerprint {
		return User{}, errDownloadGrantDenied
	}
	result, err := s.execUserWrite(r.Context(), `
		UPDATE media_download_grants
		SET consumed_at = CASE WHEN consumed_at = '' THEN ? ELSE consumed_at END
		WHERE id = ? AND token_hash = ? AND server_id = ? AND principal_user_id = ? AND profile_id = ?
			AND media_id = ? AND version_kind = ? AND version_id = ? AND version_fingerprint = ?
			AND profile = ? AND authorization_revision = ? AND expires_at > ?
			AND (consumed_at = '' OR consumed_at > ?)
			AND EXISTS (
				SELECT 1 FROM users u
				WHERE u.id = media_download_grants.principal_user_id
					AND COALESCE(u.disabled_at, '') = ''
					AND COALESCE(json_extract(u.permissions_json, '$.downloadMedia'), 0) = 1
			)
			AND EXISTS (SELECT 1 FROM profiles p WHERE p.id = media_download_grants.profile_id AND p.account_id = media_download_grants.principal_user_id AND p.disabled_at = '')`,
		now.Format(time.RFC3339), grant.ID, hashToken(token), grant.ServerID, grant.PrincipalUserID,
		grant.ProfileID, grant.MediaID, grant.VersionKind, grant.VersionID, grant.VersionFingerprint, grant.Profile, grant.AuthorizationRevision,
		now.Format(time.RFC3339), now.Add(-downloadGrantReplayWindow).Format(time.RFC3339))
	if err != nil {
		return User{}, err
	}
	consumed, err := result.RowsAffected()
	if err != nil || consumed != 1 {
		return User{}, errDownloadGrantDenied
	}
	s.recordAudit(r, user, "media.download_grant_authorized", "media", grant.MediaID, "info", map[string]string{
		"profile":     grant.Profile,
		"versionKind": grant.VersionKind,
	})
	return user, nil
}

func (s *Server) pruneDownloadStateContext(ctx context.Context, now time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if _, err := s.execBackgroundWrite(ctx, `DELETE FROM media_download_grants WHERE expires_at < ?`, now.Format(time.RFC3339)); err != nil {
		return err
	}
	if _, err := s.execBackgroundWrite(ctx, `
		DELETE FROM download_preparations
		WHERE removed_at <> '' AND updated_at < ?`, now.Add(-downloadPreparationTombstoneRetention).Format(time.RFC3339)); err != nil {
		return err
	}
	_, err := s.execBackgroundWrite(ctx, `
		DELETE FROM download_preparations
		WHERE state IN ('failed', 'unavailable', 'cancelled')
			AND updated_at < ?`, now.Add(-downloadPreparationTerminalRetention).Format(time.RFC3339))
	return err
}

func mediaDownloadIDForRequest(r *http.Request) (string, bool) {
	if r == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return "", false
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "media" || parts[2] == "" || parts[3] != "download" {
		return "", false
	}
	return parts[2], true
}
