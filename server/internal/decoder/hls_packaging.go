package decoder

import "regexp"

var hlsInitName = regexp.MustCompile(`^(?:init|init-[0-9]{1,6}|audio-[0-9]{1,2}-init-[0-9]{1,6})\.mp4$`)

// HLSPackagingArgs selects the container and the per-window init segment. The
// producer promotes an init only after checking it matches the presentation.
func HLSPackagingArgs(packaging, initName string) ([]string, string, error) {
	switch packaging {
	case "mpegts_hls":
		return []string{"-hls_segment_type", "mpegts"}, "ts", nil
	case "fmp4_hls":
		if !hlsInitName.MatchString(initName) {
			return nil, "", ErrInvalidConfiguration
		}
		return []string{"-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", initName, "-hls_segment_options", "use_editlist=0:avoid_negative_ts=disabled", "-map_metadata", "-1", "-map_chapters", "-1", "-fflags", "+bitexact"}, "m4s", nil
	}
	return nil, "", ErrInvalidConfiguration
}
