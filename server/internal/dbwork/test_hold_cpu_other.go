//go:build !linux && !darwin

package dbwork

import "time"

// processCPUTime is unavailable on this platform. A sample reports that rather
// than guessing a number.
func processCPUTime() (time.Duration, bool) { return 0, false }
