package postgres

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"portico.local/hosted/internal/security"
)

// HOST03: however many names are tried, unknown-username state stays within
// the fixed set of buckets, and the bucket depends only on the stable seal key.
func TestUnknownPasswordBucketsAreBounded(t *testing.T) {
	s := &AccountSecurity{Keys: &security.Keys{SealKey: bytes.Repeat([]byte{11}, 32)}}
	network := passwordNetwork("203.0.113.9")
	seen := map[string]struct{}{}
	for n := range 4 * unknownPasswordBuckets {
		seen[string(s.unknownPasswordSubject(unknownPasswordNetwork, fmt.Sprintf("flood%d", n), network))] = struct{}{}
	}
	if len(seen) > unknownPasswordBuckets || len(seen) < unknownPasswordBuckets*9/10 {
		t.Fatalf("%d distinct buckets for %d names; capacity is %d", len(seen), 4*unknownPasswordBuckets, unknownPasswordBuckets)
	}
	replica := &AccountSecurity{Keys: &security.Keys{SealKey: bytes.Repeat([]byte{11}, 32)}}
	if !bytes.Equal(s.unknownPasswordSubject(unknownPasswordWide, "ghost", allNetworks), replica.unknownPasswordSubject(unknownPasswordWide, "ghost", allNetworks)) {
		t.Fatal("another instance with the same key chose a different bucket")
	}
	if bytes.Equal(s.unknownPasswordSubject(unknownPasswordNetwork, "ghost", network), s.unknownPasswordSubject(unknownPasswordLock, "ghost", network)) {
		t.Fatal("a bucket's network and cool-off rows share a subject")
	}
}

// passwordFailure follows admitPasswordAttempt's SQL: four free failures, a
// cool-off doubling from 30 seconds to its cap, a fresh window after
// networkWindow, and the under-attack floor.
func TestPasswordFailureMatchesAccountSchedule(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	start := now.Add(-time.Minute)
	for failures, want := range []time.Duration{0, 0, 0, 0, coolOffBase, 2 * coolOffBase, 4 * coolOffBase} {
		n, window, until := passwordFailure(failures, start, now, false)
		if n != failures+1 || !window.Equal(start) || until.Sub(now) != want {
			t.Fatalf("after %d failures: %d %v %v", failures, n, window, until.Sub(now))
		}
	}
	if _, _, until := passwordFailure(40, start, now, false); until.Sub(now) != coolOffCap {
		t.Fatalf("cap %v", until.Sub(now))
	}
	if n, window, until := passwordFailure(9, now.Add(-networkWindow-time.Second), now, true); n != 1 || !window.Equal(now) || until.Sub(now) != underAttackCoolOff {
		t.Fatalf("fresh window under attack: %d %v %v", n, window, until.Sub(now))
	}
	if _, _, until := passwordFailure(4, start, now, true); until.Sub(now) != underAttackCoolOff {
		t.Fatalf("under-attack floor %v", until.Sub(now))
	}
}
