package decoder

import (
	"strings"
	"testing"
)

func TestRichPreparedRecipeKeepsHDRAndSurround(t *testing.T) {
	r := PreparedRecipe{VideoCodec: "hevc", DropChapters: true, BoundedFragments: true, Width: 3840, Height: 2160, VideoKbps: 14000, AudioKbps: 384,
		Source: PreparedSource{DynamicRange: "hdr10", BitDepth: 10, DolbyVision: true, MasterDisplay: "G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,50)", MaxCLL: "1000,400", AudioChannels: []int{8, 2, 1}}}
	args, err := preparedArgs("http://127.0.0.1/input", r, false)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(args, " ")
	for _, want := range []string{"-c:v libx265", "-pix_fmt yuv420p10le", "-profile:v main10", "-tag:v hvc1", "hdr10=1", "master-display=G(13250,34500)", "max-cll=1000,400", "-color_trc smpte2084", "filter_units=remove_types=62|63", "-ac:a:0 6 -b:a:0 384k", "-ac:a:1 2 -b:a:1 192k", "-ac:a:2 1 -b:a:2 96k"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if strings.Contains(line, "tonemap") || strings.Contains(line, " -ac 2") {
		t.Fatal("an HEVC recipe tone-mapped or downmixed", line)
	}
	// HLG keeps its transfer and carries no static metadata.
	r.Source = PreparedSource{DynamicRange: "hlg", BitDepth: 10}
	args, _ = preparedArgs("http://127.0.0.1/input", r, false)
	if line = strings.Join(args, " "); !strings.Contains(line, "-color_trc arib-std-b67") || strings.Contains(line, "hdr10=1") {
		t.Fatal(line)
	}
	// An HDR source in an H.264 recipe is tone-mapped to a standard picture.
	plain := PreparedRecipe{DropChapters: true, BoundedFragments: true, Width: 1920, Height: 1080, VideoKbps: 5000, AudioKbps: 192, Source: PreparedSource{DynamicRange: "hdr10", BitDepth: 10, AudioChannels: []int{6}}}
	args, err = preparedArgs("http://127.0.0.1/input", plain, false)
	if line = strings.Join(args, " "); err != nil || !strings.Contains(line, "tonemap=tonemap=hable") || !strings.Contains(line, "-color_trc bt709") || !strings.Contains(line, "-ac 2") {
		t.Fatal(line, err)
	}
	// Recipes stay a closed set.
	if _, err = preparedArgs("http://127.0.0.1/input", PreparedRecipe{VideoCodec: "hevc", Width: 1920, Height: 1080, VideoKbps: 5000, AudioKbps: 384}, false); err == nil {
		t.Fatal("an HEVC recipe at the H.264 rate was accepted")
	}
	if _, err = preparedArgs("http://127.0.0.1/input", PreparedRecipe{VideoCodec: "vp9", Width: 1920, Height: 1080, VideoKbps: 3500, AudioKbps: 384}, false); err == nil {
		t.Fatal("an unknown codec was accepted")
	}
	if PreparedChannels(false, 6) != 2 || PreparedChannels(true, 8) != 6 || PreparedChannels(true, 1) != 1 {
		t.Fatal("channel mapping")
	}
}
