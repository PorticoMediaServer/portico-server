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
	firstBytes := available - 2*mediaDiskReservationMinimum
	release, err := governor.reserveMediaDisk(filepath.Join(root, "future", "output"), firstBytes, mediaDiskReservationMinimum)
	if err != nil {
		t.Fatal(err)
	}
	second, err := governor.reserveMediaDisk(root, mediaDiskReservationMinimum, mediaDiskReservationMinimum)
	if err != nil {
		t.Fatalf("reservation that retained the safety floor failed: %v", err)
	}
	if _, err := governor.reserveMediaDisk(root, mediaDiskReservationMinimum, mediaDiskReservationMinimum); !errors.Is(err, errMediaStoragePressure) {
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
