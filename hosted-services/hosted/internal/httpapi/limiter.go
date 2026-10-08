package httpapi

import (
	"hash/maphash"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// attemptBudgets counts requests per key per minute in one bounded table per
// budget class (A11). Each class is a count-min sketch: a key's count is the
// smallest of its counters in four independently seeded rows. Memory is fixed,
// nothing is ever evicted, so cycling through many keys cannot reset anyone's
// count; a collision can only overcount, and needs the same collision in all
// four rows. Unauthenticated address churn therefore cannot touch the
// account or server budgets, which live in their own tables.
type attemptBudgets struct {
	mu      sync.Mutex
	classes map[string]*attemptSketch
}

const (
	sketchRows  = 4
	sketchWidth = 8192 // 4 × 8192 × 16 bytes = 512 KiB per class
)

type sketchCell struct {
	minute int64
	count  uint32
}

type attemptSketch struct {
	seeds [sketchRows]maphash.Seed
	rows  [sketchRows][]sketchCell
}

func newAttemptSketch() *attemptSketch {
	s := &attemptSketch{}
	for i := range s.rows {
		s.seeds[i] = maphash.MakeSeed()
		s.rows[i] = make([]sketchCell, sketchWidth)
	}
	return s
}

// hit counts one request for key in the current minute and returns the key's
// estimated count so far this minute.
func (s *attemptSketch) hit(key string, minute int64) uint32 {
	estimate := ^uint32(0)
	for i := range s.rows {
		cell := &s.rows[i][maphash.String(s.seeds[i], key)%sketchWidth]
		if cell.minute != minute {
			cell.minute, cell.count = minute, 0
		}
		if cell.count < ^uint32(0) {
			cell.count++
		}
		estimate = min(estimate, cell.count)
	}
	return estimate
}

// allow counts key against its class (the text before the first colon) and
// reports whether it is still within limit for this minute.
func (b *attemptBudgets) allow(key string, limit int, now time.Time) bool {
	class, _, _ := strings.Cut(key, ":")
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.classes == nil {
		b.classes = map[string]*attemptSketch{}
	}
	s := b.classes[class]
	if s == nil {
		s = newAttemptSketch()
		b.classes[class] = s
	}
	return s.hit(key, now.Unix()/60) <= uint32(limit)
}

// budgetNetwork is the wider network an address belongs to: the /48 an ISP
// typically delegates for IPv6 and the /24 for IPv4. It bounds a client that
// cycles through the many /64s (or addresses) it controls.
func budgetNetwork(raw string) string {
	ip, e := netip.ParseAddr(raw)
	if e != nil {
		return raw
	}
	ip = ip.Unmap()
	if ip.Is6() {
		return netip.PrefixFrom(ip, 48).Masked().String()
	}
	return netip.PrefixFrom(ip, 24).Masked().String()
}
