//go:build linux || darwin

package dbwork

import (
	"syscall"
	"time"
)

// processCPUTime reports the whole process's user+system CPU, mirroring
// operations/measurements_unix.go. RUSAGE_SELF is the process, not the calling
// goroutine, so other goroutines' CPU is included and samples say so.
func processCPUTime() (time.Duration, bool) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0, false
	}
	return time.Duration(usage.Utime.Sec+usage.Stime.Sec)*time.Second +
		time.Duration(usage.Utime.Usec+usage.Stime.Usec)*time.Microsecond, true
}
