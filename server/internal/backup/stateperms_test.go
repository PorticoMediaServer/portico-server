package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrivateStateBoundaryProtectsLooseDescendants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes do not describe Windows ACLs")
	}
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(state, "artwork")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(child, 0755); err != nil {
		t.Fatal(err)
	}
	if exposed, err := CheckPermissions(state); err != nil || exposed {
		t.Fatalf("private boundary reported exposed: %t %v", exposed, err)
	}
}

func TestPermissionRepairLeavesMediaAndLinksUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes do not describe Windows ACLs")
	}
	state, external := t.TempDir(), t.TempDir()
	for _, dir := range []string{filepath.Join(state, "mounts"), filepath.Join(state, "recordings")} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "media.mkv")
		if err := os.WriteFile(path, []byte("media"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(external, "media.mkv")
	if err := os.WriteFile(outside, []byte("media"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(outside, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(state, "linked-media")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(state, "external")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(state, 0755); err != nil {
		t.Fatal(err)
	}
	if err := FixPermissions(state); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{outside, filepath.Join(state, "mounts/media.mkv"), filepath.Join(state, "recordings/media.mkv")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0644 {
			t.Fatalf("media mode changed: %s %v %v", path, info, err)
		}
	}
	info, err := os.Stat(state)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("boundary not tightened: %v %v", info, err)
	}
}

func TestPermissionRepairFollowsTheSelectedStateRootOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes do not describe Windows ACLs")
	}
	target := t.TempDir()
	if err := os.Chmod(target, 0755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(target, "media.mkv")
	if err := os.WriteFile(child, []byte("media"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(child, 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "state")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if exposed, err := CheckPermissions(link); err != nil || !exposed {
		t.Fatalf("symlinked state not assessed: %t %v", exposed, err)
	}
	if err := FixPermissions(link); err != nil {
		t.Fatal(err)
	}
	if exposed, err := CheckPermissions(link); err != nil || exposed {
		t.Fatalf("boundary not protected: %t %v", exposed, err)
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("wrong state boundary: %v %v", info, err)
	}
	if info, err := os.Stat(child); err != nil || info.Mode().Perm() != 0644 {
		t.Fatalf("descendant permissions changed: %v %v", info, err)
	}
}
