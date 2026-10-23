package decoder

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestHardwareDeviceFailureDoesNotDisablePeer(t *testing.T) {
	backend := BackendVAAPI
	clearDeviceCircuit(backend, "/dev/dri/renderD128")
	clearDeviceCircuit(backend, "/dev/dri/renderD129")
	t.Cleanup(func() {
		clearDeviceCircuit(backend, "/dev/dri/renderD128")
		clearDeviceCircuit(backend, "/dev/dri/renderD129")
	})
	for range 3 {
		RecordHardwareDeviceFailure(backend, "/dev/dri/renderD128")
	}
	if !deviceCircuitOpen(backend, "/dev/dri/renderD128") || deviceCircuitOpen(backend, "/dev/dri/renderD129") {
		t.Fatal("device failure crossed device boundary")
	}
}
func TestHardwareDeviceSelectionFallsThroughFailedGPU(t *testing.T) {
	d := &HardwareDetector{Devices: func(context.Context) []HardwareDevice {
		return []HardwareDevice{{Backend: BackendVAAPI, Path: "/dev/dri/renderD128"}, {Backend: BackendVAAPI, Path: "/dev/dri/renderD129"}}
	}, Run: func(_ context.Context, cmd *exec.Cmd) error {
		for _, arg := range confinedArguments(t, cmd) {
			if arg == "vaapi=hw:/dev/dri/renderD128" {
				return errors.New("first GPU unavailable")
			}
		}
		return nil
	}}
	got := d.Select(context.Background(), fakeFFmpeg(t), "auto", "")
	if got.Backend != BackendVAAPI || got.Device != "/dev/dri/renderD129" {
		t.Fatal(got)
	}
}
func TestHardwareOutputRankingRequiresCodecProbe(t *testing.T) {
	probe := HardwareProbe{Backend: BackendVAAPI, Available: true, OutputEncoders: map[string]bool{"h264:8": true}}
	out := AvailableConversionOutputsForHardware(nil, probe)
	if len(out.Video) == 0 || out.Video[0].Codec != "h264" || !out.Video[0].Hardware {
		t.Fatal(out)
	}
	for _, v := range out.Video {
		if v.Codec == "av1" && v.Hardware {
			t.Fatal("unprobed AV1 became hardware")
		}
	}
}

func TestHardwareSessionLimitReleasesAndSeparatesDevices(t *testing.T) {
	p := HardwareProbe{Backend: BackendVAAPI, Device: "scheduler-test-a", Available: true, DeviceInfo: &HardwareDevice{SessionLimit: 1}}
	release, err := TryHardwareSession(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = TryHardwareSession(p); !errors.Is(err, ErrHardwareBusy) {
		t.Fatal("missing device cap", err)
	}
	q := p
	q.Device = "scheduler-test-b"
	second, err := TryHardwareSession(q)
	if err != nil {
		t.Fatal("peer device refused", err)
	}
	second()
	release()
	release()
	again, err := TryHardwareSession(p)
	if err != nil {
		t.Fatal("release did not free slot", err)
	}
	again()
}
