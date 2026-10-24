package decoder

import "testing"

func TestLinearFragmentedFiltersOnlyCopiedAAC(t *testing.T) {
	for _, tt := range []struct {
		name                   string
		fragmented, audio, aac bool
		want                   string
	}{
		{"copied AAC in MP4", true, true, true, "aac_adtstoasc"},
		{"other copied codec in MP4", true, true, false, ""},
		{"AAC in MPEG-TS", false, true, true, ""},
		{"no audio", true, false, true, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args, err := linearHLSArgs("http://127.0.0.1/input", "http://127.0.0.1/output/", LinearEncoding{Video: true, Audio: tt.audio, Fragmented: tt.fragmented, CopyAAC: tt.aac})
			if err != nil {
				t.Fatal(err)
			}
			if got := argValue(args, "-bsf:a"); got != tt.want {
				t.Fatalf("audio filter=%q want %q", got, tt.want)
			}
		})
	}
}
