//go:build linux || darwin

package main

import (
	"errors"

	"golang.org/x/sys/unix"
)

// limitProcess bounds this helper's own address space, CPU, file growth and
// core dumps before any archive is read. Every limit is checked: a helper that
// cannot enforce them refuses to parse, because the isolation depends on them.
func limitProcess() error {
	limits := []struct {
		resource int
		value    unix.Rlimit
	}{
		{unix.RLIMIT_AS, unix.Rlimit{Cur: 1 << 30, Max: 1 << 30}},
		{unix.RLIMIT_CPU, unix.Rlimit{Cur: 60, Max: 60}},
		{unix.RLIMIT_FSIZE, unix.Rlimit{Cur: maxSource, Max: maxSource}},
		// No core dumps: the spool holds private archive bytes.
		{unix.RLIMIT_CORE, unix.Rlimit{Cur: 0, Max: 0}},
	}
	for _, limit := range limits {
		if err := unix.Setrlimit(limit.resource, &limit.value); err != nil {
			return errors.New("resource limits unavailable")
		}
	}
	return nil
}
