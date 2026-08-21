package app

import (
	"context"
	"strings"
	"time"
)

type mediaStatePresenceCacheEntry struct {
	hasState  bool
	expiresAt time.Time
}

// profileHasMediaStateContext keeps the cheap existence check out of the hot
// path during a fan-out burst. Media-state writes synchronously invalidate the
// affected profile before subsequent requests can reuse an empty-state scope.
func (s *Server) profileHasMediaStateContext(ctx context.Context, profileID string) (bool, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return false, nil
	}
	now := time.Now()
	s.mediaStatePresenceMu.Lock()
	if entry, ok := s.mediaStatePresenceCache[profileID]; ok && now.Before(entry.expiresAt) {
		s.mediaStatePresenceMu.Unlock()
		return entry.hasState, nil
	}
	s.mediaStatePresenceMu.Unlock()

	var hasState int
	if err := s.queryUserRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_media_state WHERE profile_id = ?
			UNION ALL
			SELECT 1 FROM live_tv_channel_profile_state WHERE profile_id = ?
			LIMIT 1
		)`, profileID, profileID).Scan(&hasState); err != nil {
		return false, err
	}
	s.mediaStatePresenceMu.Lock()
	if s.mediaStatePresenceCache == nil {
		s.mediaStatePresenceCache = map[string]mediaStatePresenceCacheEntry{}
	}
	s.mediaStatePresenceCache[profileID] = mediaStatePresenceCacheEntry{hasState: hasState != 0, expiresAt: now.Add(30 * time.Second)}
	s.mediaStatePresenceMu.Unlock()
	return hasState != 0, nil
}

func (s *Server) invalidateMediaStatePresence(profileID string) {
	profileID = strings.TrimSpace(profileID)
	s.mediaStatePresenceMu.Lock()
	if profileID == "" {
		s.mediaStatePresenceCache = map[string]mediaStatePresenceCacheEntry{}
	} else {
		delete(s.mediaStatePresenceCache, profileID)
	}
	s.mediaStatePresenceMu.Unlock()
}
