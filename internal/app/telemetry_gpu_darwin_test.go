//go:build darwin

package app

import "testing"

func TestParseIOAcceleratorSample(t *testing.T) {
	raw := `+-o AGXAccelerator { "PerformanceStatistics" = {"Alloc system memory"=5482496000,"Tiler Utilization %"=18,"Renderer Utilization %"=70,"Device Utilization %"=74,"In use system memory"=1934196736} "model" = "Apple M4 Pro" }`
	sample, err := parseIOAcceleratorSample(raw)
	if err != nil {
		t.Fatalf("parse IOAccelerator sample: %v", err)
	}
	if sample.Usage.Status != telemetryStatusOK || sample.Usage.Value != 74 {
		t.Fatalf("usage = %#v, want 74%%", sample.Usage)
	}
	if sample.Memory.Status != telemetryStatusOK || sample.Memory.Value < 35 || sample.Memory.Value > 36 {
		t.Fatalf("memory = %#v, want approximately 35.3%%", sample.Memory)
	}
	if sample.Encoder.Status != telemetryStatusUnsupported {
		t.Fatalf("encoder = %#v, want explicitly unsupported", sample.Encoder)
	}
	if model := ioregStringValue(raw, "model"); model != "Apple M4 Pro" {
		t.Fatalf("model = %q", model)
	}
}

func TestParseIOAcceleratorSampleFallsBackToBusyEngine(t *testing.T) {
	sample, err := parseIOAcceleratorSample(`{"Renderer Utilization %"=21,"Tiler Utilization %"=33}`)
	if err != nil {
		t.Fatalf("parse engine fallback: %v", err)
	}
	if sample.Usage.Value != 33 {
		t.Fatalf("usage = %v, want busiest engine value", sample.Usage.Value)
	}
}
