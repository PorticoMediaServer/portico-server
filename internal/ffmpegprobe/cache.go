package ffmpegprobe

import (
	"sync"
	"time"
)

// MemoryCache is a bounded cache. Eviction chooses the least recently accessed entry.
type MemoryCache struct {
	mu       sync.Mutex
	max      int
	sequence uint64
	entries  map[string]memoryEntry
}
type memoryEntry struct {
	report Report
	access uint64
}

func NewMemoryCache(maxEntries int) *MemoryCache {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &MemoryCache{max: maxEntries, entries: map[string]memoryEntry{}}
}
func (c *MemoryCache) Get(key string, now time.Time) (Report, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return Report{}, false
	}
	if !now.Before(e.report.ExpiresAt) {
		delete(c.entries, key)
		return Report{}, false
	}
	c.sequence++
	e.access = c.sequence
	c.entries[key] = e
	return cloneReport(e.report), true
}
func (c *MemoryCache) Put(key string, report Report) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sequence++
	c.entries[key] = memoryEntry{report: cloneReport(report), access: c.sequence}
	if len(c.entries) <= c.max {
		return
	}
	var victim string
	var oldest uint64 = ^uint64(0)
	for key, e := range c.entries {
		if e.access < oldest {
			victim, oldest = key, e.access
		}
	}
	delete(c.entries, victim)
}
func cloneReport(r Report) Report { r.Results = append([]Result(nil), r.Results...); return r }
