package decoder

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHardwareDecodeUsesSourceCellInsteadOfOutput(t *testing.T) {
	backend := BackendVAAPI
	if !backendSpecs[backend].supported() {
		t.Skip("VAAPI graph requires Linux")
	}
	target := ConversionTarget{VideoCodec: "h264", BitDepth: 8, DynamicRange: "sdr"}
	request := ConversionRequest{Target: &target, ConvertVideo: true, Probe: HardwareProbe{Backend: backend, Available: true, ScaleFilters: true, DecodeCodecs: map[string]bool{"h264:8": true, "hevc:10": true}}, SourceVideoCodec: "hevc", SourceBitDepth: 10}
	graph, err := BuildConversion(request)
	if err != nil || argValue(graph.Input, "-hwaccel") != "vaapi" {
		t.Fatal("qualified source decode was not used", graph, err)
	}
	for _, source := range []struct {
		codec string
		depth int
	}{{"", 0}, {"hevc", 12}, {"vp9", 8}} {
		request.SourceVideoCodec, request.SourceBitDepth = source.codec, source.depth
		graph, err := BuildConversion(request)
		if err != nil || argValue(graph.Input, "-hwaccel") != "" || !strings.Contains(argValue(graph.Video, "-vf"), "hwupload") {
			t.Fatal("output H264 or another source depth granted unmeasured decode", source, graph, err)
		}
	}
	request.SourceVideoCodec, request.SourceBitDepth, request.MaxFrameRate = "hevc", 10, 24
	graph, err = BuildConversion(request)
	filter := argValue(graph.Video, "-vf")
	if err != nil || !strings.HasPrefix(filter, "hwdownload,format=p010le,fps=") || strings.Index(filter, "hwupload") < strings.Index(filter, "fps=") || argValue(graph.Input, "-hwaccel") != "vaapi" {
		t.Fatal("hardware decode did not cross system-frame filters with bounded transfers", graph, err)
	}
}

func TestHardwareDecodeProbeCannotPassWithSoftwareFrames(t *testing.T) {
	backend := BackendVAAPI
	if !backendSpecs[backend].supported() {
		t.Skip("VAAPI probe dialect requires Linux")
	}
	var generated []string
	attempts := 0
	d := HardwareDetector{Run: func(_ context.Context, cmd *exec.Cmd) error {
		args := confinedArguments(t, cmd)
		if argValue(args, "-i") == "testsrc2=size=160x128:rate=25" {
			file := args[len(args)-1]
			generated = append(generated, file)
			// A controlled command-construction fixture, never executed media.
			return os.WriteFile(file, []byte("controlled encoded fixture"), 0600)
		}
		attempts++
		if argValue(args, "-hwaccel_output_format") != "vaapi" || !strings.HasPrefix(argValue(args, "-vf"), "hwdownload,format=") {
			t.Fatal("decoder software fallback could satisfy probe", args)
		}
		if strings.Contains(filepath.Base(argValue(args, "-i")), "-10") {
			return errors.New("controlled failed hardware surface")
		}
		return nil
	}}
	cells := d.probeDecodeCodecs(context.Background(), fakeFFmpeg(t), backend, "/dev/dri/renderD128", time.Second)
	if attempts != 10 || !cells["h264:8"] || cells["hevc:10"] || cells["av1:10"] {
		t.Fatal("codec/depth probe outcomes were inferred", cells, attempts)
	}
	if _, measured := cells["vc1:8"]; measured {
		t.Fatal("VC1 was claimed without a licensed fixture")
	}
	for _, file := range generated {
		if _, err := os.Stat(filepath.Dir(file)); !os.IsNotExist(err) {
			t.Fatal("finished decode fixtures not removed", file, err)
		}
	}
}
