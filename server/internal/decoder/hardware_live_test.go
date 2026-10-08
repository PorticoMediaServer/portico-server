package decoder

import (
	"context"
	"portico.local/server/internal/decodertest"
	"testing"
	"time"
)

// The confined probe against a real ffmpeg with its real dependency set. This is
// the configuration production uses, and it is the only way to find out whether
// the confinement profile actually lets an encoder start.
func TestHardwareProbeConfinedWithResolvedLibraries(t *testing.T) {
	ffmpeg := decodertest.QualifiedFFmpeg(t)
	ffprobe := decodertest.QualifiedFFprobe(t)
	if !CheckConfinement(context.Background()) {
		t.Skip("decoder confinement is unavailable on this host")
	}
	libraries, err := ResolveLibraries(ffmpeg, ffprobe, nil)
	if err != nil {
		t.Skip("decoder dependencies could not be resolved: " + err.Error())
	}
	detector := &HardwareDetector{Libraries: libraries, Timeout: 30 * time.Second}
	report := detector.Report(context.Background(), ffmpeg, "")
	for _, probe := range report {
		t.Logf("%s available=%v reason=%s native=%v %dms", probe.Backend, probe.Available, probe.Reason, probe.NativeFilters, probe.DurationMS)
	}
	// Software always works; a host with no hardware is a valid outcome, but a
	// probe that claims availability must have run and must name its encoder.
	for _, probe := range report {
		if probe.Available && probe.Backend != BackendSoftware && probe.Encoder == "" {
			t.Fatal("available backend without an encoder", probe)
		}
	}
	selected := detector.Select(context.Background(), ffmpeg, "auto", "")
	if !selected.Available {
		t.Fatal("selection produced no usable backend", selected)
	}
}
