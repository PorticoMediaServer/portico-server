// Package hostlimits reads and, where the OS allows it, raises the per-process
// resource ceilings this server actually runs into.
//
// At two hundred streaming viewers the file-descriptor count per stream is a
// client socket, a media file or helper socketpair and the helper's own
// descriptors — call it three or four — on top of the SQLite pool, its WAL and
// shm, the scanner, and every stat the converter does. Docker's default soft
// NOFILE is 1024 and macOS often starts at 256. Crossing that limit does not
// crash: `accept` returns EMFILE and net/http backs off for a second, so the
// server degrades into "everything is mysteriously broken" rather than failing
// in a way anyone can read. Raising the soft limit to the hard one costs nothing
// and needs no privilege.
package hostlimits

import (
	"log"
	"runtime/debug"
)

// memoryLimitShare is how much of the cgroup's ceiling the collector is told to
// work to. The remainder is everything the Go heap does not account for — the
// binary, stacks, the SQLite page caches, the helper processes — and a limit set
// at the ceiling itself would have the collector running flat out while the
// kernel killed the process anyway.
const memoryLimitShare = 0.9

// OpenFiles reports the current soft and hard descriptor limits. On a platform
// with no such limit both are zero, which is the honest answer rather than a
// fabricated ceiling.
func OpenFiles() (soft, hard uint64) { return openFileLimits() }

// EffectiveMemoryBytes reports the smaller discoverable physical-memory and
// process cgroup ceiling. It sizes bounded transport resources, not the Go heap;
// zero means neither source could be measured.
func EffectiveMemoryBytes() int64 {
	physical, _ := physicalMemoryBytes()
	cgroup, _ := cgroupMemoryLimit()
	return effectiveMemoryBytes(physical, cgroup)
}

func effectiveMemoryBytes(physical, cgroup uint64) int64 {
	if physical == 0 || cgroup > 0 && cgroup < physical {
		physical = cgroup
	}
	// Untrusted OS text must never wrap a signed budget negative.
	if physical > 1<<63-1 {
		return 0
	}
	return int64(physical)
}

// Apply raises the descriptor limit and, where a memory ceiling is discoverable,
// gives the garbage collector one to work to. It logs what it did, because both
// numbers are things an owner reading a support bundle needs to see, and it
// never fails: a server that cannot raise its own limits still runs.
func Apply() {
	if soft, hard, raised := raiseOpenFiles(); raised {
		log.Printf("Open file limit raised to %d (hard limit %d)", soft, hard)
	} else if soft > 0 && soft < 4096 {
		// Worth saying out loud: at this ceiling a busy evening degrades into
		// mysterious failures rather than an honest refusal.
		log.Printf("Open file limit is %d, which is low for a server carrying many streams; the hard limit is %d", soft, hard)
	}
	limit, ok := cgroupMemoryLimit()
	if !ok {
		// Nothing discoverable. A guessed limit set too low is a collection death
		// spiral, which is worse than no limit at all.
		return
	}
	soft := int64(float64(limit) * memoryLimitShare)
	if soft < 64<<20 {
		// Below this the limit would be fighting the server's own working set.
		return
	}
	debug.SetMemoryLimit(soft)
	log.Printf("Heap limit set to %d MiB, from a cgroup limit of %d MiB", soft>>20, limit>>20)
}
