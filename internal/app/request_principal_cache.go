package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const (
	requestPrincipalCacheLimit = 1024
	requestPrincipalCacheTTL   = 15 * time.Second
)

type verifiedRequestPrincipalSnapshot struct {
	principal RequestPrincipal
	fence     string
	expiresAt time.Time
	lastUsed  time.Time
}

type RequestPrincipalCacheMetrics struct {
	Entries   int    `json:"entries"`
	Capacity  int    `json:"capacity"`
	Hits      uint64 `json:"hits"`
	Misses    uint64 `json:"misses"`
	Evictions uint64 `json:"evictions"`
}

func requestPrincipalCacheKey(accountID, profileID string) string {
	return accountID + "\x00" + profileID
}

func (s *Server) cachedRequestPrincipal(ctx context.Context, accountID, profileID string) (RequestPrincipal, bool, error) {
	key := requestPrincipalCacheKey(accountID, profileID)
	s.requestPrincipalCacheMu.Lock()
	entry, ok := s.requestPrincipalCache[key]
	s.requestPrincipalCacheMu.Unlock()
	if !ok || !entry.expiresAt.After(time.Now().UTC()) {
		if ok {
			s.requestPrincipalCacheMu.Lock()
			delete(s.requestPrincipalCache, key)
			s.requestPrincipalCacheMu.Unlock()
		}
		s.requestPrincipalCacheMisses.Add(1)
		return RequestPrincipal{}, false, nil
	}
	fence, err := s.authorizationRevisionForUserContextStrict(ctx, User{ID: accountID, AccountID: accountID, ProfileID: profileID})
	if err != nil {
		return RequestPrincipal{}, false, err
	}
	if fence != entry.fence {
		s.requestPrincipalCacheMu.Lock()
		delete(s.requestPrincipalCache, key)
		s.requestPrincipalCacheMu.Unlock()
		s.requestPrincipalCacheMisses.Add(1)
		return RequestPrincipal{}, false, nil
	}
	entry.lastUsed = time.Now().UTC()
	s.requestPrincipalCacheMu.Lock()
	s.requestPrincipalCache[key] = entry
	s.requestPrincipalCacheMu.Unlock()
	s.requestPrincipalCacheHits.Add(1)
	return cloneRequestPrincipal(entry.principal), true, nil
}

func (s *Server) rememberRequestPrincipal(ctx context.Context, principal RequestPrincipal) {
	fence, err := s.authorizationRevisionForUserContextStrict(ctx, User{ID: principal.AccountID, AccountID: principal.AccountID, ProfileID: principal.ProfileID})
	if err != nil {
		return
	}
	now := time.Now().UTC()
	entry := verifiedRequestPrincipalSnapshot{principal: cloneRequestPrincipal(principal), fence: fence, expiresAt: now.Add(requestPrincipalCacheTTL), lastUsed: now}
	key := requestPrincipalCacheKey(principal.AccountID, principal.ProfileID)
	s.requestPrincipalCacheMu.Lock()
	defer s.requestPrincipalCacheMu.Unlock()
	if s.requestPrincipalCache == nil {
		s.requestPrincipalCache = make(map[string]verifiedRequestPrincipalSnapshot)
	}
	if _, exists := s.requestPrincipalCache[key]; !exists && len(s.requestPrincipalCache) >= requestPrincipalCacheLimit {
		oldestKey := ""
		var oldest time.Time
		for candidateKey, candidate := range s.requestPrincipalCache {
			if oldestKey == "" || candidate.lastUsed.Before(oldest) {
				oldestKey, oldest = candidateKey, candidate.lastUsed
			}
		}
		delete(s.requestPrincipalCache, oldestKey)
		s.requestPrincipalCacheEvictions.Add(1)
	}
	s.requestPrincipalCache[key] = entry
}

func cloneRequestPrincipal(principal RequestPrincipal) RequestPrincipal {
	// The snapshot is small and this keeps every nested policy slice immutable
	// without sharing backing arrays with request handlers.
	encoded, _ := json.Marshal(principal)
	var cloned RequestPrincipal
	_ = json.Unmarshal(encoded, &cloned)
	return cloned
}

func (s *Server) requestPrincipalAuthorizationFence(accountID, profileID string, now time.Time) (string, bool) {
	key := requestPrincipalCacheKey(strings.TrimSpace(accountID), strings.TrimSpace(profileID))
	s.requestPrincipalCacheMu.Lock()
	entry, ok := s.requestPrincipalCache[key]
	s.requestPrincipalCacheMu.Unlock()
	if !ok || !entry.expiresAt.After(now) || strings.TrimSpace(entry.fence) == "" {
		return "", false
	}
	return entry.fence, true
}

func (s *Server) requestPrincipalCacheMetricsSnapshot() RequestPrincipalCacheMetrics {
	s.requestPrincipalCacheMu.Lock()
	entries := len(s.requestPrincipalCache)
	s.requestPrincipalCacheMu.Unlock()
	return RequestPrincipalCacheMetrics{Entries: entries, Capacity: requestPrincipalCacheLimit, Hits: s.requestPrincipalCacheHits.Load(), Misses: s.requestPrincipalCacheMisses.Load(), Evictions: s.requestPrincipalCacheEvictions.Load()}
}

func (s *Server) invalidateAuthorizationCachesForMutation(query string, tags []string) {
	sensitive := false
	settingsChanged := false
	for _, tag := range tags {
		switch strings.ToLower(strings.TrimSpace(tag)) {
		case "settings":
			settingsChanged = true
		case "users", "account", "profiles", "devices", "sessions", "libraries", "hosted_profile_snapshot_state", "profile_account_authentications", "native_refresh_tokens":
			sensitive = true
		}
	}
	lowerQuery := strings.ToLower(query)
	if strings.Contains(lowerQuery, " settings ") || strings.Contains(lowerQuery, "into settings") {
		settingsChanged = true
	}
	if settingsChanged {
		s.settingsReadCacheMu.Lock()
		s.settingsReadCache = nil
		s.settingsReadCacheExpires = time.Time{}
		s.settingsReadCacheMu.Unlock()
	}
	for _, marker := range []string{"update users ", "delete from users", "update profiles ", "delete from profiles", "user_library_access", "hosted_profile_snapshot_state", "delete from sessions", "revoked_at", "trusted ="} {
		if strings.Contains(lowerQuery, marker) {
			sensitive = true
			break
		}
	}
	if !sensitive {
		return
	}
	s.requestPrincipalCacheMu.Lock()
	clear(s.requestPrincipalCache)
	s.requestPrincipalCacheMu.Unlock()
	s.sessionPrincipalCacheMu.Lock()
	clear(s.sessionPrincipalCache)
	s.sessionPrincipalCacheMu.Unlock()
	s.mediaGrantCacheMu.Lock()
	clear(s.mediaGrantCache)
	s.mediaGrantCacheMu.Unlock()
}
