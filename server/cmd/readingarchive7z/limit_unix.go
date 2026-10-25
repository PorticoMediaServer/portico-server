//go:build linux || darwin

package main

import "golang.org/x/sys/unix"

// limitProcess bounds this helper's own address space, CPU and file growth
// before any archive is read. The parent still sets deadlines and owns the
// process group; these are a second, in-process fence.
func limitProcess() {
	_ = unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: 1 << 30, Max: 1 << 30})
	_ = unix.Setrlimit(unix.RLIMIT_CPU, &unix.Rlimit{Cur: 60, Max: 60})
	_ = unix.Setrlimit(unix.RLIMIT_FSIZE, &unix.Rlimit{Cur: maxSource, Max: maxSource})
}
