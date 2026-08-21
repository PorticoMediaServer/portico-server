package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

const (
	sessionPrincipalCacheLimit = 1024
	sessionPrincipalCacheTTL   = 15 * time.Second
)

type verifiedSessionPrincipalSnapshot struct {
	user         User
	sessionID    string
	deviceID     string
	expiresAt    time.Time
	authzFence   string
	cacheExpires time.Time
	lastUsed     time.Time
}

type SessionPrincipalCacheMetrics struct {
	Entries   int    `json:"entries"`
	Capacity  int    `json:"capacity"`
	Hits      uint64 `json:"hits"`
	Misses    uint64 `json:"misses"`
	Evictions uint64 `json:"evictions"`
}

type AuthorizationCacheDiagnostics struct {
	Sessions   SessionPrincipalCacheMetrics `json:"sessions"`
	Principals RequestPrincipalCacheMetrics `json:"principals"`
	MediaGrant MediaGrantCacheMetrics       `json:"mediaGrants"`
}

func (s *Server) cachedSessionPrincipal(ctx context.Context, tokenHash string, now time.Time, strict bool) (User, time.Time, bool, error) {
	s.sessionPrincipalCacheMu.Lock()
	entry, ok := s.sessionPrincipalCache[tokenHash]
	s.sessionPrincipalCacheMu.Unlock()
	if !ok || !entry.cacheExpires.After(now) || !entry.expiresAt.After(now) {
		if ok {
			s.forgetSessionPrincipal(tokenHash)
		}
		s.sessionPrincipalCacheMisses.Add(1)
		return User{}, time.Time{}, false, nil
	}
	// Session/profile/device mutations performed by the server synchronously
	// clear this cache. Ordinary short requests can therefore use the bounded
	// verified snapshot without repeating the full authorization query graph.
	// Long-lived event transports request strict revalidation before they wait,
	// so a durable revocation cannot leave a stream authorized until TTL expiry.
	if !strict {
		entry.lastUsed = now
		s.sessionPrincipalCacheMu.Lock()
		s.sessionPrincipalCache[tokenHash] = entry
		s.sessionPrincipalCacheMu.Unlock()
		s.sessionPrincipalCacheHits.Add(1)
		return cloneSessionUser(entry.user), entry.expiresAt, true, nil
	}
	var sessionID, userID, profileID, expiresAt, deviceID, userDisabled, profileDisabled, deviceRevoked string
	var deviceTrusted int
	err := s.queryUserRow(ctx, `
		SELECT s.id, s.user_id, COALESCE(NULLIF(s.profile_id, ''), s.user_id), s.expires_at, s.device_id,
			COALESCE(u.disabled_at, ''), COALESCE(p.disabled_at, ''), COALESCE(d.revoked_at, ''), COALESCE(d.trusted, 0)
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN profiles p ON p.account_id = u.id AND p.id = COALESCE(NULLIF(s.profile_id, ''), s.user_id)
		LEFT JOIN devices d ON d.id = s.device_id
		WHERE s.token_hash = ?`, tokenHash).Scan(&sessionID, &userID, &profileID, &expiresAt, &deviceID, &userDisabled, &profileDisabled, &deviceRevoked, &deviceTrusted)
	if err != nil {
		if err == sql.ErrNoRows {
			s.forgetSessionPrincipal(tokenHash)
			return User{}, time.Time{}, false, nil
		}
		return User{}, time.Time{}, false, err
	}
	parsedExpiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || !parsedExpiry.After(now) || sessionID != entry.sessionID || userID != entry.user.ID || profileID != viewerProfileID(entry.user) || deviceID != entry.deviceID || userDisabled != "" || profileDisabled != "" || deviceRevoked != "" {
		s.forgetSessionPrincipal(tokenHash)
		return User{}, time.Time{}, false, nil
	}
	if normalizeAuthProvider(entry.user.AuthProvider) == "portico" {
		if err := s.ensureHostedProfileDirectoryFreshness(ctx, entry.user.ID, now); err != nil {
			return User{}, time.Time{}, false, err
		}
	}
	fence, err := s.authorizationRevisionForUserContextStrict(ctx, entry.user)
	if err != nil {
		return User{}, time.Time{}, false, err
	}
	if fence != entry.authzFence {
		s.forgetSessionPrincipal(tokenHash)
		return User{}, time.Time{}, false, nil
	}
	requireTrusted, err := s.requireTrustedDevicesStrict(ctx)
	if err != nil {
		return User{}, time.Time{}, false, err
	}
	if (requireTrusted && deviceTrusted != 1) || !cachedUserDevicePolicyAllows(entry.user, deviceID, deviceTrusted == 1) || !userAccessScheduleAllows(entry.user.AccessSchedule, now) {
		s.forgetSessionPrincipal(tokenHash)
		return User{}, time.Time{}, false, nil
	}
	entry.lastUsed = now
	s.sessionPrincipalCacheMu.Lock()
	s.sessionPrincipalCache[tokenHash] = entry
	s.sessionPrincipalCacheMu.Unlock()
	s.sessionPrincipalCacheHits.Add(1)
	return cloneSessionUser(entry.user), entry.expiresAt, true, nil
}

func (s *Server) rememberSessionPrincipal(ctx context.Context, tokenHash, sessionID, deviceID string, user User, expiresAt time.Time) {
	if strings.HasPrefix(sessionID, "nativesess_") || tokenHash == "" || !expiresAt.After(time.Now().UTC()) {
		return
	}
	fence, ok := s.requestPrincipalAuthorizationFence(accountIDForUser(user), viewerProfileID(user), time.Now().UTC())
	if !ok {
		var err error
		fence, err = s.authorizationRevisionForUserContextStrict(ctx, user)
		if err != nil {
			return
		}
	}
	now := time.Now().UTC()
	entry := verifiedSessionPrincipalSnapshot{user: cloneSessionUser(user), sessionID: sessionID, deviceID: deviceID, expiresAt: expiresAt, authzFence: fence, cacheExpires: now.Add(sessionPrincipalCacheTTL), lastUsed: now}
	s.sessionPrincipalCacheMu.Lock()
	defer s.sessionPrincipalCacheMu.Unlock()
	if s.sessionPrincipalCache == nil {
		s.sessionPrincipalCache = make(map[string]verifiedSessionPrincipalSnapshot)
	}
	if _, exists := s.sessionPrincipalCache[tokenHash]; !exists && len(s.sessionPrincipalCache) >= sessionPrincipalCacheLimit {
		oldestKey := ""
		var oldest time.Time
		for key, candidate := range s.sessionPrincipalCache {
			if oldestKey == "" || candidate.lastUsed.Before(oldest) {
				oldestKey, oldest = key, candidate.lastUsed
			}
		}
		delete(s.sessionPrincipalCache, oldestKey)
		s.sessionPrincipalCacheEvictions.Add(1)
	}
	s.sessionPrincipalCache[tokenHash] = entry
}

func (s *Server) forgetSessionPrincipal(tokenHash string) {
	s.sessionPrincipalCacheMu.Lock()
	delete(s.sessionPrincipalCache, tokenHash)
	s.sessionPrincipalCacheMu.Unlock()
}

func (s *Server) authorizationCacheDiagnostics() AuthorizationCacheDiagnostics {
	s.sessionPrincipalCacheMu.Lock()
	entries := len(s.sessionPrincipalCache)
	s.sessionPrincipalCacheMu.Unlock()
	return AuthorizationCacheDiagnostics{
		Sessions:   SessionPrincipalCacheMetrics{Entries: entries, Capacity: sessionPrincipalCacheLimit, Hits: s.sessionPrincipalCacheHits.Load(), Misses: s.sessionPrincipalCacheMisses.Load(), Evictions: s.sessionPrincipalCacheEvictions.Load()},
		Principals: s.requestPrincipalCacheMetricsSnapshot(),
		MediaGrant: s.mediaGrantCacheMetricsSnapshot(),
	}
}

func cloneSessionUser(user User) User {
	encoded, _ := json.Marshal(user)
	var cloned User
	_ = json.Unmarshal(encoded, &cloned)
	cloned.AccountID = user.AccountID
	cloned.AuthSessionID = user.AuthSessionID
	cloned.ProfileIsPrimary = user.ProfileIsPrimary
	cloned.DeviceID = user.DeviceID
	cloned.AccountProfilesAllowed = user.AccountProfilesAllowed
	return cloned
}

func cachedUserDevicePolicyAllows(user User, deviceID string, trusted bool) bool {
	if strings.TrimSpace(deviceID) == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(user.Role), "owner") {
		return true
	}
	switch user.DevicePolicy.Mode {
	case "trusted":
		return trusted
	case "allowlist":
		for _, allowed := range user.DevicePolicy.AllowedDeviceIDs {
			if allowed == deviceID {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func (s *Server) requireTrustedDevicesStrict(ctx context.Context) (bool, error) {
	var raw string
	err := s.queryUserRow(ctx, `SELECT value_json FROM settings WHERE key = 'devices'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var group map[string]any
	if err := json.Unmarshal([]byte(raw), &group); err != nil {
		return false, err
	}
	return settingBool(group, "requireTrustedDevices", false), nil
}
