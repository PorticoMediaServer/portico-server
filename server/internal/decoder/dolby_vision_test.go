package decoder

import (
	"strings"
	"testing"
)

// ReshapesDolbyVision agrees with the graph BuildConversion actually builds:
// true exactly when the tone mapper is libplacebo (COMPAT-04).
func TestReshapesDolbyVisionMatchesTheConversionGraph(t *testing.T) {
	filters := map[string]bool{"zscale": true, "tonemap": true, "libplacebo": true, "tonemap_vaapi": true, "tonemap_cuda": true, "vpp_qsv": true, "scale_vaapi": true, "scale_cuda": true, "scale": true}
	probes := []HardwareProbe{
		{Backend: BackendSoftware},
		{Backend: BackendVAAPI, Available: true, NativeFilters: true, Device: "/dev/dri/renderD128"},
		{Backend: BackendVAAPI, Available: true, Device: "/dev/dri/renderD128"},
		{Backend: BackendNVENC, Available: true, NativeFilters: true},
		{Backend: BackendVideoToolbox, Available: true},
	}
	compared := 0
	for _, vulkan := range []bool{false, true} {
		for _, probe := range probes {
			r := ConversionRequest{ConvertVideo: true, ToneMap: true, Probe: probe, Toolchain: &ToolchainFacts{Filters: filters, VulkanAvailable: vulkan}}
			graph, err := BuildConversion(r)
			if err != nil {
				continue // this backend isn't buildable on this platform
			}
			compared++
			uses := strings.Contains(strings.Join(graph.Video, " "), "libplacebo")
			if got := ReshapesDolbyVision(r); got != uses {
				t.Errorf("vulkan=%t %+v: ReshapesDolbyVision=%t, graph uses libplacebo=%t", vulkan, probe, got, uses)
			}
		}
	}
	if compared < 4 {
		t.Fatalf("only %d combinations built a graph", compared)
	}
	if ReshapesDolbyVision(ConversionRequest{ConvertVideo: true, ToneMap: false, Toolchain: &ToolchainFacts{Filters: filters, VulkanAvailable: true}}) {
		t.Error("no tone mapping, nothing reshaped")
	}
}
