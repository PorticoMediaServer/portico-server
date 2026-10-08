package decoder

import (
	"context"
	"fmt"
	"portico.local/server/internal/decodertest"
	"testing"
)

func TestToolchainListFormats(t *testing.T) {
	for _, version := range []string{"4.4", "6.1", "8.1"} {
		t.Run(version, func(t *testing.T) {
			enc := parseToolList("Encoders:\n V..... = Video\n ------\n V....D libx264 H.264\n A..... aac AAC\n", "encoders")
			filters := parseToolList("Filters:\n ... = support\n ---\n .SC zscale V->V Convert\n T.. tonemap V->V Tone map\n", "filters")
			if !enc["libx264"] || !enc["aac"] || enc["="] || !filters["zscale"] || !filters["tonemap"] {
				t.Fatal(enc, filters)
			}
		})
	}
}
func TestInstalledToolchain(t *testing.T) {
	path := decodertest.QualifiedFFmpeg(t)
	f, e := ProbeToolchain(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	if !f.Encoders["aac"] || !f.Muxers["mp4"] || !f.Decoders["h264"] {
		t.Fatalf("%+v", f)
	}
	if missing := f.MissingRequired(); len(missing) > 0 {
		decodertest.Unavailable(t, fmt.Sprintf("toolchain missing requirements: %v", missing))
	}
	t.Log(f.Version, f.MissingRequired())
}
func TestHardwareRuntimeCircuit(t *testing.T) {
	clearHardwareCircuit(BackendNVENC)
	defer clearHardwareCircuit(BackendNVENC)
	for i := 0; i < 3; i++ {
		RecordHardwareFailure(BackendNVENC)
	}
	if !hardwareCircuitOpen(BackendNVENC) {
		t.Fatal("did not open")
	}
	clearHardwareCircuit(BackendNVENC)
	if hardwareCircuitOpen(BackendNVENC) {
		t.Fatal("did not reset")
	}
}
