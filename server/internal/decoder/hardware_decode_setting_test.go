package decoder

import (
	"strings"
	"testing"
)

func TestDisabledHardwareDecodeKeepsHardwareFilterAndEncoder(t *testing.T) {
	exercised := false
	for _, backend := range []HardwareBackend{BackendVAAPI, BackendQSV, BackendNVENC} {
		if !backendSpecs[backend].supported() {
			continue
		}
		exercised = true
		request := ConversionRequest{Probe: HardwareProbe{Backend: backend, Available: true, NativeFilters: true}, ConvertVideo: true, MaxHeight: 720, DisableHardwareDecode: true}
		graph, err := BuildConversion(request)
		if err != nil || argValue(graph.Input, "-hwaccel") != "" || !strings.Contains(argValue(graph.Video, "-vf"), "hwupload") || graph.Backend != backend {
			t.Fatal("decode-off did not keep software decode and hardware upload/encode", backend, graph, err)
		}
		for _, stage := range graph.Stages {
			if stage.Operation == StageDecode && stage.Execution != ExecutionSoftware {
				t.Fatal("decode-off graph advertised hardware decode", stage)
			}
		}
		request.DisableHardwareDecode = false
		enabled, err := BuildConversion(request)
		if err != nil || argValue(enabled.Input, "-hwaccel") == "" {
			t.Fatal("existing decode-enabled graph regressed", backend, enabled, err)
		}
	}
	if !exercised {
		t.Skip("this platform has no uploaded-surface backend")
	}
}
