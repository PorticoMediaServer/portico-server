package listeningrules

import (
	"testing"
	"time"
)

func TestResumeRewindIsBounded(t *testing.T) {
	now := time.Unix(1800000000, 0)
	for _, v := range []struct {
		pause          time.Duration
		position, want float64
	}{{time.Minute, 100, 100}, {6 * time.Minute, 100, 85}, {2 * time.Hour, 100, 70}, {2 * time.Hour, 5, 0}} {
		if got := Resume(v.position, now.Add(-v.pause).Format(time.RFC3339Nano), now); got != v.want {
			t.Fatalf("%+v: %v", v, got)
		}
	}
}
