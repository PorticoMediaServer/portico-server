package listeningrules

import (
	"math"
	"time"
)

// Resume preserves stored progress and gives the listener a little context.
func Resume(position float64, lastPlayed string, now time.Time) float64 {
	at, e := time.Parse(time.RFC3339Nano, lastPlayed)
	if e != nil {
		return position
	}
	elapsed := now.Sub(at)
	rewind := 0.
	if elapsed >= time.Hour {
		rewind = 30
	} else if elapsed >= 5*time.Minute {
		rewind = 15
	}
	return math.Max(0, position-rewind)
}
