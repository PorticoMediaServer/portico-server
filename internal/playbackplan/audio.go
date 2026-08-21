package playbackplan

import (
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
)

// audioLayoutChannels is deliberately closed.  Guessing a channel order is
// worse than declining a tuple: the same channel count does not establish the
// semantic position of the channels.
var audioLayoutChannels = map[string]int{
	"mono": 1, "stereo": 2, "2.1": 3, "3.0": 3, "3.1": 4,
	"3.0(back)": 3, "4.0": 4, "quad": 4, "4.1": 5,
	"5.0": 5, "5.0(side)": 5, "5.1": 6, "5.1(side)": 6,
	"6.1": 7, "7.1": 8,
}

func canonicalAudioLayout(layout string) (string, int, bool) {
	layout = token(layout)
	switch layout {
	case "1.0":
		layout = "mono"
	case "2.0":
		layout = "stereo"
	}
	channels, ok := audioLayoutChannels[layout]
	return layout, channels, ok
}

func audioRoute(a mediafacts.Audio, c playbackcap.Audio) (layout string, channels int, copyAudio bool, ok bool) {
	inLayout, inChannels, known := canonicalAudioLayout(a.Layout)
	if !known || a.Channels != inChannels {
		return "", 0, false, false
	}
	outLayout, outChannels, known := canonicalAudioLayout(c.Layout)
	if !known || outChannels > inChannels || (c.MaxChannels > 0 && outChannels > c.MaxChannels) || !supportedAudioReduction(inLayout, outLayout) {
		return "", 0, false, false
	}
	copyAudio = audioCodecMatches(a, c) && inLayout == outLayout
	if !copyAudio && !supportedAudioEncoder(c.Codec) {
		return "", 0, false, false
	}
	return outLayout, outChannels, copyAudio, true
}

func supportedAudioEncoder(codec string) bool {
	switch token(codec) {
	case "aac", "alac", "ac3", "eac3", "e-ac-3", "opus", "vorbis", "flac", "mp3", "pcm", "pcm_s16le", "pcm_s24le":
		return true
	default:
		return false
	}
}

func supportedAudioReduction(input, output string) bool {
	if input == output || output == "stereo" {
		return true
	}
	if output == "mono" {
		return input == "stereo"
	}
	return (output == "5.1" || output == "5.1(side)") && (input == "6.1" || input == "7.1")
}

func audioCodecMatches(a mediafacts.Audio, c playbackcap.Audio) bool {
	return same(c.Codec, a.Codec) && wild(c.Profile, a.Profile) &&
		(a.ObjectAudio == "" || c.ObjectPassthrough)
}

// audioFidelityScore makes the conversion ladder independent of capability
// tuple order. Layout and object preservation dominate codec preference.
func audioFidelityScore(source mediafacts.Audio, decision AudioDecision) int {
	score := decision.Channels * 100
	if decision.Passthrough {
		score += 10_000
	}
	if decision.ObjectsPreserved {
		score += 2_000
	}
	codec := strings.TrimSpace(strings.ToLower(decision.Codec))
	score += map[string]int{
		"truehd": 900, "mlp": 900,
		"dts-hd ma": 880, "dts-hd": 870,
		"flac": 850,
		"alac": 845,
		"pcm":  840, "pcm_s16le": 840, "pcm_s24le": 845,
		"eac3": 720, "e-ac-3": 720,
		"dts": 700, "dca": 700,
		"ac3":    650,
		"opus":   620,
		"vorbis": 580,
		"aac":    600,
		"mp3":    400,
	}[codec]
	_ = source // retained to make future source-family ladders explicit.
	return score
}
