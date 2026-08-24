package app

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestMediaDiskReservationsAccountForConcurrentPredictedOutput(t *testing.T) {
	governor := newMediaResourceGovernor()
	root := t.TempDir()
	available, _, err := filesystemSpace(root)
	if err != nil {
		t.Fatal(err)
	}
	if available < 4*mediaDiskReservationMinimum {
		t.Skip("test filesystem has insufficient headroom")
	}
	// Leave broad headroom between checks: the filesystem is shared with other
	// tests and its reported free space can legitimately move while this test
	// runs. The reservation invariant does not depend on byte-exact exhaustion.
	firstBytes := available / 2
	release, err := governor.reserveMediaDisk(filepath.Join(root, "future", "output"), firstBytes, mediaDiskReservationMinimum)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes := available / 4
	second, err := governor.reserveMediaDisk(root, secondBytes, mediaDiskReservationMinimum)
	if err != nil {
		t.Fatalf("reservation that retained the safety floor failed: %v", err)
	}
	if _, err := governor.reserveMediaDisk(root, available/2, mediaDiskReservationMinimum); !errors.Is(err, errMediaStoragePressure) {
		t.Fatalf("concurrent overcommit error = %v", err)
	}
	second()
	release()
	release()
	secondRelease, err := governor.reserveMediaDisk(root, mediaDiskReservationMinimum, mediaDiskReservationMinimum)
	if err != nil {
		t.Fatalf("released reservation remained charged: %v", err)
	}
	secondRelease()
}
