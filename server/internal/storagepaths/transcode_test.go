package storagepaths

import (
	"runtime"
	"testing"
)

func TestTranscodeDirectoriesPlacementAndVerification(t *testing.T) {
	mounts := []Mount{{"/", "ext4"}, {"/ram", "tmpfs"}, {"/unbounded", "ramfs"}}
	for _, tc := range []struct {
		requested, fallback, disk, memory string
		fail                              bool
	}{
		{"/disk", "/fallback", "/disk", "", false},
		{"/ram/output", "/disk", "/disk", "/ram/output", false},
		{"/unbounded/output", "/disk", "/disk", "", false},
		{"/ram/output", "/ram/spill", "", "", true},
		{"/ram/output", "", "", "", true},
	} {
		disk, memory, err := TranscodeDirectories(tc.requested, tc.fallback, mounts)
		wantMemory := tc.memory
		if runtime.GOOS != "linux" {
			wantMemory = ""
		}
		if (err != nil) != tc.fail || disk != tc.disk || memory != wantMemory {
			t.Fatalf("%+v: %q %q %v", tc, disk, memory, err)
		}
	}
	if _, _, err := TranscodeDirectories("/unknown", "/disk", nil); err == nil {
		t.Fatal("unknown filesystem accepted")
	}
}
