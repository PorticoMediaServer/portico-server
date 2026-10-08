//go:build linux

package hostlimits

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// With the default GOGC the heap grows to twice the live set before a
// collection. Inside a Docker memory limit, or on a 4 GiB home server, the
// kernel's OOM killer wins that race and the process dies with no log line at
// all — the worst possible failure, because the owner is left with "it restarted
// and I don't know why". debug.SetMemoryLimit turns the same pressure into more
// frequent collection and a slower server, which is a thing an owner can see and
// act on.
//
// The limit has to come from the cgroup rather than from total system memory: a
// container is told how much it may use, and that is the only number that
// matters. Where there is no such number there is nothing to derive, and a
// guessed limit set too low is a GC death spiral — so nothing is set.

// cgroupMemoryLimit reads this process's memory ceiling, in bytes.
func cgroupMemoryLimit() (uint64, bool) {
	// cgroup v2 first: the unified hierarchy is what every current distribution
	// and Docker release uses.
	if path, ok := unifiedMemoryPath(); ok {
		if value, ok := readMemoryValue(path); ok {
			return value, true
		}
	}
	// v1, for older hosts.
	for _, path := range []string{
		"/sys/fs/cgroup/memory/memory.limit_in_bytes",
		"/sys/fs/cgroup/memory.max",
	} {
		if value, ok := readMemoryValue(path); ok {
			return value, true
		}
	}
	return 0, false
}

// unifiedMemoryPath resolves this process's own cgroup v2 memory.max, which is
// where a container's limit actually lives — the root file usually says "max".
func unifiedMemoryPath() (string, bool) {
	file, err := os.Open("/proc/self/cgroup")
	if err != nil {
		return "", false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// v2 lines are "0::<path>".
		fields := strings.SplitN(scanner.Text(), ":", 3)
		if len(fields) == 3 && fields[0] == "0" {
			return filepath.Join("/sys/fs/cgroup", fields[2], "memory.max"), true
		}
	}
	return "", false
}

// readMemoryValue reads one cgroup byte value, rejecting the several spellings
// of "no limit".
func readMemoryValue(path string) (uint64, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "max" {
		return 0, false
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	// cgroup v1 writes a number near 2^63 to mean unlimited, and a limit larger
	// than any real machine is the same statement.
	if value >= 1<<62 {
		return 0, false
	}
	return value, true
}
