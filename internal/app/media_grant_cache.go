package app

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/database"
)

const (
	mediaGrantCacheLimit      = 2048
	mediaGrantBusyFallbackTTL = 8 * time.Second
	mediaGrantVerifyInterval  = time.Second
)

type verifiedMediaGrantSnapshot struct {
	tokenHash             string
	resourceKind          string
	resourceID            string
	playbackSessionID     string
	user                  User
	operationClasses      []string
	deliveryMode          string
	transcodeQuality      string
	allowedQualities      []string
	expiresAt             time.Time
	authorizationRevision string
	verifiedAt            time.Time
	lastUsedAt            time.Time
}

type mediaGrantTerminalState struct {
	revokedAt          string
	expiresAt          time.Time
	playbackEndedAt    string
	playbackState      string
	userDisabledAt     string
	storedAuthRevision string
}

type MediaGrantCacheMetrics struct {
	Entries         int    `json:"entries"`
	Capacity        int    `json:"capacity"`
	Hits            uint64 `json:"hits"`
	Misses          uint64 `json:"misses"`
	Evictions       uint64 `json:"evictions"`
	BusyFallbacks   uint64 `json:"busyFallbacks"`
	TerminalDenials uint64 `json:"terminalDenials"`
}

type mediaGrantCacheCounters struct {
	hits            atomic.Uint64
	misses          atomic.Uint64
	evictions       atomic.Uint64
	busyFallbacks   atomic.Uint64
	terminalDenials atomic.Uint64
}

func (s *Server) cachedMediaGrantSnapshot(tokenHash string) (verifiedMediaGrantSnapshot, bool) {
	s.mediaGrantCacheMu.Lock()
	defer s.mediaGrantCacheMu.Unlock()
	entry, ok := s.mediaGrantCache[tokenHash]
	if !ok {
		s.mediaGrantCacheCounters.misses.Add(1)
		return verifiedMediaGrantSnapshot{}, false
	}
	entry.lastUsedAt = time.Now().UTC()
	s.mediaGrantCache[tokenHash] = entry
	s.mediaGrantCacheCounters.hits.Add(1)
	return cloneVerifiedMediaGrantSnapshot(entry), true
}

func (s *Server) rememberVerifiedMediaGrant(entry verifiedMediaGrantSnapshot) {
	if entry.tokenHash == "" || entry.expiresAt.IsZero() {
		return
	}
	s.mediaGrantCacheMu.Lock()
	defer s.mediaGrantCacheMu.Unlock()
	if s.mediaGrantCache == nil {
		s.mediaGrantCache = make(map[string]verifiedMediaGrantSnapshot)
	}
	now := time.Now().UTC()
	for key, candidate := range s.mediaGrantCache {
		if !candidate.expiresAt.After(now) {
			delete(s.mediaGrantCache, key)
		}
	}
	if _, exists := s.mediaGrantCache[entry.tokenHash]; !exists && len(s.mediaGrantCache) >= mediaGrantCacheLimit {
		oldestKey := ""
		var oldest time.Time
		for key, candidate := range s.mediaGrantCache {
			if oldestKey == "" || candidate.lastUsedAt.Before(oldest) {
				oldestKey, oldest = key, candidate.lastUsedAt
			}
		}
		delete(s.mediaGrantCache, oldestKey)
		s.mediaGrantCacheCounters.evictions.Add(1)
	}
	entry.lastUsedAt = now
	s.mediaGrantCache[entry.tokenHash] = cloneVerifiedMediaGrantSnapshot(entry)
}

func (s *Server) forgetMediaGrant(tokenHash string) {
	s.mediaGrantCacheMu.Lock()
	delete(s.mediaGrantCache, tokenHash)
	s.mediaGrantCacheMu.Unlock()
}

func (s *Server) forgetMediaGrantsForPlaybackSession(playbackSessionID string) {
	s.mediaGrantCacheMu.Lock()
	for key, entry := range s.mediaGrantCache {
		if entry.playbackSessionID == playbackSessionID {
			delete(s.mediaGrantCache, key)
		}
	}
	s.mediaGrantCacheMu.Unlock()
}

func (s *Server) forgetMediaGrantsForAPIKey(apiKeyID string) {
	s.mediaGrantCacheMu.Lock()
	for key, entry := range s.mediaGrantCache {
		if entry.user.APIKeyID == apiKeyID {
			delete(s.mediaGrantCache, key)
		}
	}
	s.mediaGrantCacheMu.Unlock()
}

func (s *Server) mediaGrantCacheMetricsSnapshot() MediaGrantCacheMetrics {
	s.mediaGrantCacheMu.Lock()
	entries := len(s.mediaGrantCache)
	s.mediaGrantCacheMu.Unlock()
	return MediaGrantCacheMetrics{
		Entries: entries, Capacity: mediaGrantCacheLimit,
		Hits: s.mediaGrantCacheCounters.hits.Load(), Misses: s.mediaGrantCacheCounters.misses.Load(),
		Evictions: s.mediaGrantCacheCounters.evictions.Load(), BusyFallbacks: s.mediaGrantCacheCounters.busyFallbacks.Load(),
		TerminalDenials: s.mediaGrantCacheCounters.terminalDenials.Load(),
	}
}

func cloneVerifiedMediaGrantSnapshot(entry verifiedMediaGrantSnapshot) verifiedMediaGrantSnapshot {
	entry.operationClasses = append([]string(nil), entry.operationClasses...)
	entry.allowedQualities = append([]string(nil), entry.allowedQualities...)
	entry.user.Permissions = clonePermissionMap(entry.user.Permissions)
	entry.user.LibraryIDs = append([]string(nil), entry.user.LibraryIDs...)
	entry.user.BlockedProfileLabels = append([]string(nil), entry.user.BlockedProfileLabels...)
	return entry
}

func (s *Server) mediaGrantTerminalProbeContext(ctx context.Context, tokenHash string, scope mediaGrantScope) (mediaGrantTerminalState, error) {
	if s.mediaGrantTerminalProbe != nil {
		return s.mediaGrantTerminalProbe(ctx, tokenHash, scope)
	}
	var state mediaGrantTerminalState
	var expiresAt string
	err := s.queryUserRow(ctx, `
		SELECT g.revoked_at, g.expires_at, ps.ended_at, ps.state, COALESCE(u.disabled_at, ''), g.authorization_revision
		FROM playback_media_grants g
		JOIN playback_sessions ps ON ps.id = g.playback_session_id
		JOIN users u ON u.id = g.principal_user_id
		WHERE g.token_hash = ? AND g.resource_kind = ? AND g.resource_id = ?
		LIMIT 1`, tokenHash, scope.ResourceKind, scope.ResourceID).Scan(
		&state.revokedAt, &expiresAt, &state.playbackEndedAt, &state.playbackState, &state.userDisabledAt, &state.storedAuthRevision)
	if err != nil {
		return mediaGrantTerminalState{}, err
	}
	state.expiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return mediaGrantTerminalState{}, err
	}
	return state, nil
}

func mediaGrantTransientDatabaseError(err error, parent context.Context) bool {
	if err == nil {
		return false
	}
	if database.IsRetryableLock(err) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) && (parent == nil || parent.Err() == nil) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "temporarily unavailable") || strings.Contains(message, "sqlite handle is not available")
}
