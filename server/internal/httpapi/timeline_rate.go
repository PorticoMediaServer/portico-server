package httpapi

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

const (
	timelineReportsPerSecond = 10
	timelineReportBurst      = 20
	timelineRateEntries      = 4096
	timelineRateTTL          = time.Minute
)

type timelineRateEntry struct {
	key         string
	tokens      float64
	at, expires time.Time
}
type timelineLimiter struct {
	mu      sync.Mutex
	order   *list.List
	entries map[string]*list.Element
}

func newTimelineLimiter() *timelineLimiter {
	return &timelineLimiter{order: list.New(), entries: make(map[string]*list.Element)}
}

// Applied only after the API's existing authentication and device binding.
// Normal progress arrives once per ten seconds; a generous burst preserves
// immediate seeks and state changes while bounding one device's writer traffic.
// Profiles, token rotation and session IDs do not mint new buckets.
func (l *timelineLimiter) allow(c v1Caller, now time.Time) bool {
	if c.DeviceID == "" {
		return false
	}
	raw, _ := json.Marshal([]string{c.ServerID, c.Authority, c.AccountID, c.DeviceID})
	sum := sha256.Sum256(raw)
	key := hex.EncodeToString(sum[:])
	l.mu.Lock()
	defer l.mu.Unlock()
	element, found := l.entries[key]
	value := timelineRateEntry{key: key, tokens: timelineReportBurst, at: now}
	if found {
		old := element.Value.(timelineRateEntry)
		if now.Before(old.expires) {
			value = old
			if now.After(value.at) {
				value.tokens += now.Sub(value.at).Seconds() * timelineReportsPerSecond
				if value.tokens > timelineReportBurst {
					value.tokens = timelineReportBurst
				}
				value.at = now
			}
		}
	}
	allowed := value.tokens >= 1
	if allowed {
		value.tokens--
	}
	value.expires = now.Add(timelineRateTTL)
	if found {
		element.Value = value
		l.order.MoveToFront(element)
	} else {
		l.entries[key] = l.order.PushFront(value)
	}
	for l.order.Len() > timelineRateEntries {
		old := l.order.Back()
		delete(l.entries, old.Value.(timelineRateEntry).key)
		l.order.Remove(old)
	}
	return allowed
}
