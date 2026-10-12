package playback

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/persistence"
)

func TestSoftwareFallbackDoesNotDisableHardwareBeforeRetrySucceeds(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err = os.WriteFile(path, []byte("probe seam; never executed"), 0700); err != nil {
		t.Fatal(err)
	}
	backend := decoder.BackendNVENC
	if runtime.GOOS == "darwin" {
		backend = decoder.BackendVideoToolbox
	}
	d := &decoder.HardwareDetector{Run: func(context.Context, *exec.Cmd) error { return nil }}
	if probe := d.Probe(context.Background(), path, backend, ""); !probe.Available {
		t.Fatal(probe)
	}
	h := &HLS{db: db}
	for range 3 {
		h.recordSoftwareFallback(context.Background(), "missing-session", string(backend), "invalid source")
	}
	if probe := d.SelectCached(path, string(backend), ""); probe.Backend != backend {
		t.Fatal("unproven source failures disabled hardware", probe)
	}
}

func TestHardwareFailureRequiresSuccessfulUninterruptedSoftwareRetry(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		err                    error
		cancelled, throttled   bool
		from, before, produced int
		want                   bool
	}{
		{"valid source", nil, false, false, 7, 6, 7, true},
		{"corrupt source", errors.New("invalid data"), false, false, 7, 6, 6, false},
		{"GPU filter also failed", errors.New("device unavailable"), false, false, 7, 6, 6, false},
		{"cancelled success", nil, true, false, 7, 6, 7, false},
		{"throttled before output", context.Canceled, false, true, 7, 6, 6, false},
		{"throttled after output", context.Canceled, false, true, 7, 6, 7, true},
		{"viewer canceled after output", context.Canceled, true, true, 7, 6, 7, false},
		{"corrupt source after output", errors.New("invalid data"), false, false, 7, 6, 7, false},
		{"clean completion before watcher tick", nil, false, false, 7, 6, 6, true},
		{"cancelled child", context.Canceled, true, false, 7, 6, 6, false},
		{"throttle on old hardware output", context.Canceled, false, true, 7, 8, 8, false},
		{"retry extends hardware output", context.Canceled, false, true, 7, 8, 9, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hardwareFailureConfirmed(tc.err, tc.cancelled, tc.throttled, tc.from, tc.before, tc.produced); got != tc.want {
				t.Fatalf("got %t, want %t", got, tc.want)
			}
		})
	}
}
