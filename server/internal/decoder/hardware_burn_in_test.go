package decoder

import (
	"strings"
	"testing"
)

// A device composite is chosen only for the case its probe proves, and the graph
// it builds keeps the picture on the device and reads the subtitle as the same
// sealed input the processor path reads.
func TestHardwareBurnInGraphBlendsOnTheDevice(t *testing.T) {
	burn := BurnIn{File: "/media/subtitle.sup", Format: "pgs", Width: 1920, Height: 1080, PositionUS: 1_000_000, OffsetUS: 0, InputStartUS: 0}
	graph, inputs, err := hardwareBurnInGraph(BackendVAAPI, burn, "format=nv12,hwupload", "scale_vaapi=w=1280:h=720,scale_vaapi=format=nv12")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(graph, "[0:v:0]format=nv12,hwupload,scale_vaapi=format=nv12[burn_picture]") {
		t.Fatalf("the picture meets the composite in its own device format: %s", graph)
	}
	if !strings.Contains(graph, "[1:s:0]settb=AVTB,setpts=PTS+(-1.000000)/TB,format=bgra,") {
		t.Fatalf("the subtitle keeps its own clock and the device's alpha format: %s", graph)
	}
	if !strings.Contains(graph, "pad=1920:1080:(ow-iw)/2:(oh-ih)/2") {
		t.Fatalf("the subtitle is fitted to the source canvas the blend happens on: %s", graph)
	}
	if !strings.Contains(graph, "hwupload[burn_cues]") || !strings.Contains(graph, "[burn_picture][burn_cues]overlay_vaapi=eof_action=pass:shortest=0[burn_mixed]") {
		t.Fatalf("the device must do the blend: %s", graph)
	}
	if !strings.Contains(graph, "[burn_mixed]scale_vaapi=w=1280:h=720,scale_vaapi=format=nv12[burn_output]") {
		t.Fatalf("the scale follows the blend, then the encoder surface: %s", graph)
	}
	if len(inputs) == 0 || inputs[len(inputs)-1] != "/media/subtitle.sup" {
		t.Fatalf("the sealed subtitle is the second input: %v", inputs)
	}
}

// Everything the probe did not prove stays on the processor.
func TestHardwareBurnInStaysOnTheProcessorWhereItIsNotProven(t *testing.T) {
	bitmap := BurnIn{File: "/media/subtitle.sup", Format: "pgs", Width: 1920, Height: 1080}
	sdr := ConversionTarget{DynamicRange: "sdr", BitDepth: 8}
	if !hardwareBurnInSupported(BackendVAAPI, bitmap, sdr) {
		t.Fatal("an 8-bit standard-range bitmap subtitle has a device composite")
	}
	for name, held := range map[string]struct {
		backend HardwareBackend
		burn    BurnIn
		target  ConversionTarget
	}{
		"text subtitle":    {BackendVAAPI, BurnIn{File: "/media/subtitle.ass", Format: "ass", Width: 1920, Height: 1080}, sdr},
		"hdr picture":      {BackendVAAPI, bitmap, ConversionTarget{DynamicRange: "hdr10", BitDepth: 10}},
		"ten bit picture":  {BackendVAAPI, bitmap, ConversionTarget{DynamicRange: "sdr", BitDepth: 10}},
		"apple backend":    {BackendVideoToolbox, bitmap, sdr},
		"amd backend":      {BackendAMF, bitmap, sdr},
		"no drawn canvas":  {BackendVAAPI, BurnIn{File: "/media/subtitle.sup", Format: "pgs"}, sdr},
	} {
		if hardwareBurnInSupported(held.backend, held.burn, held.target) {
			t.Fatalf("%s must keep the processor burn-in", name)
		}
	}
}
