package decoder

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"portico.local/server/internal/decodertest"
	"strconv"
	"testing"
	"time"
)

func TestHLSPackagingArguments(t *testing.T) {
	args, ext, err := HLSPackagingArgs("fmp4_hls", "init-12.mp4")
	if err != nil || ext != "m4s" || argValue(args, "-hls_segment_type") != "fmp4" || argValue(args, "-hls_fmp4_init_filename") != "init-12.mp4" {
		t.Fatal(args, ext, err)
	}
	if _, _, err = HLSPackagingArgs("fmp4_hls", "../init.mp4"); err == nil {
		t.Fatal("accepted escaped init path")
	}
	_, ext, err = HLSPackagingArgs("mpegts_hls", "")
	if err != nil || ext != "ts" {
		t.Fatal(ext, err)
	}
}

func TestFragmentedConversionKeepsInitAcrossWindows(t *testing.T) {
	binary := decodertest.QualifiedFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	facts, err := ProbeToolchain(ctx, binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, codec := range []string{"h264", "hevc", "av1"} {
		t.Run(codec, func(t *testing.T) {
			if _, err := softwareVideoEncoder(codec, &facts); err != nil {
				t.Skip(err)
			}
			root := t.TempDir()
			var init []byte
			target := ConversionTarget{VideoCodec: codec, Width: 160, Height: 90, FrameRate: 24, Packaging: "fmp4_hls", RateControl: "quality"}
			graph, err := BuildConversion(ConversionRequest{Target: &target, ConvertVideo: true, SoftwarePreset: "ultrafast", Toolchain: &facts})
			if err != nil {
				t.Fatal(err)
			}
			for _, from := range []int{0, 1} {
				dir := filepath.Join(root, strconv.Itoa(from))
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				args := []string{"-v", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=s=160x90:r=24:d=7"}
				args = append(args, graph.Video...)
				packaging, ext, err := HLSPackagingArgs("fmp4_hls", fmt.Sprintf("init-%d.mp4", from))
				if err != nil {
					t.Fatal(err)
				}
				args = append(args, packaging...)
				args = append(args, "-an", "-f", "hls", "-hls_time", "6", "-hls_list_size", "0", "-hls_flags", "independent_segments+temp_file", "-output_ts_offset", strconv.Itoa(from*6), "-start_number", strconv.Itoa(from), "-hls_segment_filename", filepath.Join(dir, "segment-%06d."+ext), filepath.Join(dir, "window.m3u8"))
				runTinyMedia(t, ctx, binary, args, dir)
				data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("init-%d.mp4", from)))
				if err != nil {
					t.Fatal(err)
				}
				if from == 0 {
					init = data
				} else if !bytes.Equal(init, data) {
					t.Fatal("init changed across seek windows")
				}
				if info, err := os.Stat(filepath.Join(dir, fmt.Sprintf("segment-%06d.m4s", from))); err != nil || info.Size() == 0 {
					t.Fatal("missing fragment", err)
				}
			}
		})
	}
}
