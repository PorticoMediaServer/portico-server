package livechannels

import (
	"testing"
	"time"
)

func TestLiveRewindDurationKeepsMatchingStorageBudget(t *testing.T) {
	for _, tc := range []struct {
		minutes int
		bytes   int64
	}{{10, 1_500_000_000}, {30, 4_500_000_000}, {60, 9_000_000_000}, {120, 16 << 30}} {
		v := DefaultLiveDefaults()
		v.TimeShiftMinutes = tc.minutes
		duration, bytes, err := v.TimeShiftLimits()
		if err != nil || duration != time.Duration(tc.minutes)*time.Minute || bytes != tc.bytes {
			t.Fatalf("%d minute rewind: %v/%d %v", tc.minutes, duration, bytes, err)
		}
	}
	for _, minutes := range []int{0, 1, 31, 240} {
		v := DefaultLiveDefaults()
		v.TimeShiftMinutes = minutes
		if _, _, err := v.TimeShiftLimits(); err == nil {
			t.Fatalf("unsupported rewind%d accepted", minutes)
		}
	}
}
