package decoder

import (
	"fmt"
	"path/filepath"
	"strings"
)

type BurnIn struct {
	File, Format         string
	PositionUS, OffsetUS int64
	Width, Height        int
}

// burnFilter runs on system frames after HDR-to-SDR and before scaling/upload.
// Both subtitle formats retain the title clock across restarted HLS windows.
func burnFilter(b BurnIn, facts *ToolchainFacts) (string, []string, error) {
	if !filepath.IsAbs(b.File) || filepath.Clean(b.File) != b.File || strings.ContainsAny(b.File, "\n\r\x00") || b.PositionUS < 0 || b.PositionUS > 86400000000 || b.OffsetUS < -600000000 || b.OffsetUS > 600000000 {
		return "", nil, ErrInvalidConfiguration
	}
	switch b.Format {
	case "ass", "ssa":
		if facts != nil && !facts.Filters["ass"] {
			return "", nil, fmt.Errorf("subtitle_burn_filter_unavailable")
		}
		// Two escaping levels: AVOption's filename and the filtergraph parser.
		file := strings.NewReplacer("\\", "\\\\", "'", "\\'", ":", "\\:").Replace(b.File)
		file = strings.NewReplacer("\\", "\\\\", "'", "\\'", ",", "\\,", "[", "\\[", "]", "\\]", ";", "\\;").Replace(file)
		shift := seconds(b.PositionUS - b.OffsetUS)
		return "setpts=PTS+(" + shift + ")/TB,ass=filename=" + file + ",setpts=PTS-(" + shift + ")/TB", nil, nil
	case "pgs", "vobsub", "mks":
		if b.Width < 1 || b.Height < 1 || b.Width > 8192 || b.Height > 4320 {
			return "", nil, ErrInvalidConfiguration
		}
		if facts != nil && !facts.Filters["overlay"] {
			return "", nil, fmt.Errorf("subtitle_burn_filter_unavailable")
		}
		mux := map[string]string{"pgs": "sup", "vobsub": "vobsub", "mks": "matroska"}[b.Format]
		formats := mux
		if b.Format == "vobsub" {
			formats += " ,mpeg"
			formats = strings.ReplaceAll(formats, " ", "")
		}
		// Preserve the subtitle source clock; FFmpeg otherwise subtracts the
		// first cue timestamp. Reset only the picture after the input seek.
		inputs := []string{"-copyts", "-protocol_whitelist", "file", "-format_whitelist", formats, "-canvas_size", fmt.Sprintf("%dx%d", b.Width, b.Height), "-f", mux, "-i", b.File}
		filter := fmt.Sprintf("setpts=PTS-STARTPTS[burn_picture];[1:s:0]settb=AVTB,setpts=PTS+(%s)/TB,scale=%d:%d[burn_cues];[burn_picture][burn_cues]overlay=eof_action=pass:shortest=0", seconds(b.OffsetUS-b.PositionUS), b.Width, b.Height)
		return filter, inputs, nil
	}
	return "", nil, ErrInvalidConfiguration
}
