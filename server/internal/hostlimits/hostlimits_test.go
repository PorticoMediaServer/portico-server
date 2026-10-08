package hostlimits

import (
	"runtime"
	"runtime/debug"
	"testing"
)

// The descriptor limit is the one resource whose exhaustion does not crash: accept
// returns EMFILE, net/http backs off a second, and the server degrades into
// "everything is mysteriously broken". Raising it must actually work on the
// platform it runs on, and must never make it worse.
func TestApplyRaisesTheDescriptorLimitWithoutLoweringIt(t *testing.T) {
	before, hard := OpenFiles()
	if runtime.GOOS == "windows" {
		if before != 0 || hard != 0 {
			t.Fatalf("Windows reported a descriptor rlimit: %d/%d", before, hard)
		}
		return
	}
	if before == 0 {
		t.Skip("this platform reports no descriptor limit")
	}
	previousLimit := debug.SetMemoryLimit(-1)
	t.Cleanup(func() { debug.SetMemoryLimit(previousLimit) })
	Apply()
	after, _ := OpenFiles()
	if after < before {
		t.Fatalf("the soft descriptor limit fell from %d to %d", before, after)
	}
	t.Logf("descriptor limit %d -> %d (hard %d)", before, after, hard)
	// Applying twice is not a mistake an owner should be able to make matter.
	Apply()
	again, _ := OpenFiles()
	if again != after {
		t.Fatalf("a second Apply changed the limit from %d to %d", after, again)
	}
}

// A guessed heap limit set too low is a collection death spiral, which is worse
// than no limit at all, so nothing is set where nothing is discoverable.
func TestNoHeapLimitIsSetWithoutACgroupCeiling(t *testing.T) {
	if _, ok := cgroupMemoryLimit(); ok {
		t.Skip("this host has a cgroup memory ceiling")
	}
	previous := debug.SetMemoryLimit(-1)
	t.Cleanup(func() { debug.SetMemoryLimit(previous) })
	Apply()
	if debug.SetMemoryLimit(-1) != previous {
		t.Fatal("a heap limit was invented without a ceiling to derive it from")
	}
}
