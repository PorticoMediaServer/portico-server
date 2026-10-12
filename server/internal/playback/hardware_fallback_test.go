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
		name                       string
		err                        error
		cancelled, throttled, want bool
	}{
		{"valid source", nil, false, false, true},
		{"corrupt source", errors.New("invalid data"), false, false, false},
		{"GPU filter also failed", errors.New("device unavailable"), false, false, false},
		{"cancelled success", nil, true, false, false},
		{"throttled success", nil, false, true, false},
		{"cancelled child", context.Canceled, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hardwareFailureConfirmed(tc.err, tc.cancelled, tc.throttled); got != tc.want {
				t.Fatalf("got %t, want %t", got, tc.want)
			}
		})
	}
}
