package storagepaths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExistingUnownedDisposableScopesAreNeverAdopted(t *testing.T) {
	for _, scope := range []string{"Temp", "Cache"} {
		t.Run(scope, func(t *testing.T) {
			l := At(filepath.Join(t.TempDir(), "unrelated"))
			dir := filepath.Join(l.Data, scope)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			keep := filepath.Join(dir, "keep")
			if err := os.WriteFile(keep, []byte("unrelated contents"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := l.Prepare(); err == nil {
				t.Fatal("adopted an unowned destructive scope")
			}
			if err := l.EmptyOwned(scope); err == nil {
				t.Fatal("emptied an unowned scope")
			}
			if raw, err := os.ReadFile(keep); err != nil || string(raw) != "unrelated contents" {
				t.Fatal("unrelated contents changed", err)
			}
			entries, err := os.ReadDir(l.Data)
			if err != nil || len(entries) != 1 {
				t.Fatal("refusal modified data folder", entries, err)
			}
			if _, err := os.Stat(filepath.Join(dir, ownershipMarker)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("minted ownership over preexisting data", err)
			}
		})
	}
}

func TestOwnedScopeSurvivesReopenButMissingOrCopiedOwnershipRefusesDeletion(t *testing.T) {
	l := testLayout(t)
	if err := l.Prepare(); err != nil {
		t.Fatal("did not retain ownership", err)
	}
	if err := os.Remove(filepath.Join(l.Temp, ownershipMarker)); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(l.Temp, "keep")
	if err := os.WriteFile(keep, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := l.ResetTemp(); err == nil {
		t.Fatal("missing ownership accepted")
	}
	if err := l.Prepare(); err == nil {
		t.Fatal("ownership reminted over existing contents")
	}
	if raw, err := os.ReadFile(keep); err != nil || string(raw) != "original" {
		t.Fatal(err)
	}
	other := At(filepath.Join(t.TempDir(), "other"))
	if err := os.MkdirAll(other.Temp, 0700); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(l.Cache, ownershipMarker))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other.Temp, ownershipMarker), marker, 0600); err != nil {
		t.Fatal(err)
	}
	if err := other.ResetTemp(); err == nil {
		t.Fatal("copied ownership accepted")
	}
}

func TestCleanupRefusesUnknownMountsAndEveryDescendantVolume(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Temp")
	for _, mounts := range [][]Mount{nil, {{dir, "apfs"}}, {{filepath.Join(dir, "volume"), "apfs"}}, {{filepath.Join(dir, "same-device-bind"), "ext4"}}} {
		if err := refuseMountsWithin(dir, mounts); !errors.Is(err, ErrCleanupUnavailable) {
			t.Fatal("cleanup admitted a mount or unknown table", mounts, err)
		}
	}
	if err := refuseMountsWithin(dir, []Mount{{filepath.VolumeName(dir) + string(os.PathSeparator), "ext4"}, {dir + "-sibling", "apfs"}}); err != nil {
		t.Fatal(err)
	}
}

func TestRAMMappedTranscodeCannotBypassDiskOnlyProducer(t *testing.T) {
	mounts := []Mount{{"/", "ext4"}, {"/transcode", "tmpfs"}, {"/dev/shm", "tmpfs"}}
	l, err := Resolve(Options{Container: true, Mounts: mounts})
	if err != nil || l.Transcode != "/config/Transcode" || !l.TranscodeFallback {
		t.Fatal(l, err)
	}
	got, err := DiskTranscodeDirectory("/transcode", "/config/Transcode", mounts)
	if err != nil || got != "/config/Transcode" {
		t.Fatal(got, err)
	}
	if _, err := DiskTranscodeDirectory("/transcode", "/dev/shm/spill", mounts); err == nil {
		t.Fatal("RAM spill accepted for live FFmpeg output")
	}
}
