package ffmpeggraph

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

// AudioRequest is the shared, deterministic audio ladder compiler used by
// both on-demand playback and durable optimized-version production.
type AudioRequest struct {
	InputCodec, InputLayout        string
	InputChannels, InputSampleRate int
	OutputCodec, OutputLayout      string
	OutputContainer                string
	OutputChannels                 int
	Copy                           bool
	MaxBitrate                     int64
	Gapless                        playbackplan.GaplessDecision
}

func CompileAudio(req AudioRequest) ([]string, error) {
	inputLayout, err := validateAudioLayout(req.InputLayout, req.InputChannels)
	if err != nil {
		return nil, err
	}
	outputLayout, err := validateAudioLayout(req.OutputLayout, req.OutputChannels)
	if err != nil {
		return nil, err
	}
	if req.Copy {
		if token(req.InputCodec) != token(req.OutputCodec) || inputLayout != outputLayout {
			return nil, fail(FactsMismatch, "audio copy changes codec or layout")
		}
		args := []string{"-c:a", "copy"}
		if token(req.OutputCodec) == "mp3" && token(req.OutputContainer) == "mp3" {
			args = append(args, "-write_xing", "1")
		}
		return args, nil
	}
	encoder, ok := audioEncoder(req.OutputCodec)
	if !ok {
		return nil, fail(UnsupportedGraph, "unsupported audio encoder %q", req.OutputCodec)
	}
	if req.InputSampleRate <= 0 {
		return nil, fail(UnsupportedGraph, "audio conversion requires exact sample rate")
	}
	filter, err := deterministicDownmix(inputLayout, outputLayout)
	if err != nil {
		return nil, err
	}
	args := []string{}
	// Decoders consume authoritative skip/discard side data. Resetting the
	// filtered timeline after that operation prevents a source/container start
	// offset from leaking into the encoded output. The plan deliberately keeps
	// conversion status unverified until the produced file is probed.
	if filter == "" {
		filter = "asetpts=PTS-STARTPTS"
	} else {
		filter += ",asetpts=PTS-STARTPTS"
	}
	args = append(args, "-af", filter)
	sampleRate := req.InputSampleRate
	if sampleRate > 48_000 {
		sampleRate = 48_000
	}
	args = append(args, "-c:a", encoder, "-ac", strconv.Itoa(req.OutputChannels), "-channel_layout", outputLayout,
		"-ar", strconv.Itoa(sampleRate), "-b:a", strconv.FormatInt(audioBitrateFor(req.OutputChannels, req.OutputCodec, req.MaxBitrate), 10))
	if token(req.OutputCodec) == "mp3" {
		args = append(args, "-write_xing", "1")
	}
	return args, nil
}

func audioBitrateFor(channels int, codec string, maximum int64) int64 {
	var target int64
	switch {
	case channels <= 2:
		target = 192000
	case channels <= 6:
		target = 384000
	default:
		target = 640000
	}
	switch token(codec) {
	case "ac3":
		if target > 640000 {
			target = 640000
		}
	case "eac3", "e-ac-3":
		if target > 768000 {
			target = 768000
		}
	case "opus":
		if target > 512000 {
			target = 512000
		}
	}
	if maximum > 0 && maximum < target {
		target = maximum
	}
	return target
}

var exactAudioLayouts = map[string]int{
	"mono": 1, "stereo": 2, "2.1": 3, "3.0": 3, "3.1": 4,
	"3.0(back)": 3, "4.0": 4, "quad": 4, "4.1": 5,
	"5.0": 5, "5.0(side)": 5, "5.1": 6, "5.1(side)": 6,
	"6.1": 7, "7.1": 8,
}

func canonicalLayout(layout string) (string, int, bool) {
	layout = token(layout)
	switch layout {
	case "1.0":
		layout = "mono"
	case "2.0":
		layout = "stereo"
	}
	channels, ok := exactAudioLayouts[layout]
	return layout, channels, ok
}

func validateAudioLayout(layout string, channels int) (string, error) {
	canonical, expected, ok := canonicalLayout(layout)
	if !ok || expected != channels {
		return "", fail(UnsupportedGraph, "unknown or inconsistent audio layout %q (%d channels)", layout, channels)
	}
	return canonical, nil
}

// deterministicDownmix returns an explicit, reviewable channel map.  Every
// reduction reserves headroom and ends in a true-peak limiter; FFmpeg's
// implicit -ac matrix is intentionally never used.
func deterministicDownmix(input, output string) (string, error) {
	if input == output {
		return "", nil
	}
	if output == "5.1" || output == "5.1(side)" {
		leftSurround, rightSurround := "BL", "BR"
		if output == "5.1(side)" {
			leftSurround, rightSurround = "SL", "SR"
		}
		switch input {
		case "7.1":
			return fmt.Sprintf("pan=%s|FL=0.707*FL|FR=0.707*FR|FC=0.707*FC|LFE=0.5*LFE|%s=0.5*BL+0.5*SL|%s=0.5*BR+0.5*SR,alimiter=limit=0.95:attack=5:release=50", output, leftSurround, rightSurround), nil
		case "6.1":
			return fmt.Sprintf("pan=%s|FL=0.707*FL|FR=0.707*FR|FC=0.707*FC|LFE=0.5*LFE|%s=0.5*SL+0.25*BC|%s=0.5*SR+0.25*BC,alimiter=limit=0.95:attack=5:release=50", output, leftSurround, rightSurround), nil
		}
	}
	if output == "stereo" {
		left, right := []string{}, []string{}
		add := func(dst *[]string, coefficient, channel string) { *dst = append(*dst, coefficient+"*"+channel) }
		switch input {
		case "mono":
			add(&left, "0.707", "FC")
			add(&right, "0.707", "FC")
		case "2.1":
			add(&left, "0.707", "FL")
			add(&left, "0.25", "LFE")
			add(&right, "0.707", "FR")
			add(&right, "0.25", "LFE")
		case "3.0", "3.1", "5.0", "5.1", "5.0(side)", "5.1(side)", "6.1", "7.1":
			add(&left, "0.707", "FL")
			add(&right, "0.707", "FR")
			add(&left, "0.5", "FC")
			add(&right, "0.5", "FC")
			if strings.Contains(input, ".1") {
				add(&left, "0.25", "LFE")
				add(&right, "0.25", "LFE")
			}
			switch input {
			case "5.0", "5.1", "7.1":
				add(&left, "0.5", "BL")
				add(&right, "0.5", "BR")
			case "5.0(side)", "5.1(side)":
				add(&left, "0.5", "SL")
				add(&right, "0.5", "SR")
			case "6.1":
				add(&left, "0.35", "SL")
				add(&right, "0.35", "SR")
				add(&left, "0.25", "BC")
				add(&right, "0.25", "BC")
			}
			if input == "7.1" {
				add(&left, "0.35", "SL")
				add(&right, "0.35", "SR")
			}
		case "3.0(back)":
			add(&left, "0.707", "FL")
			add(&right, "0.707", "FR")
			add(&left, "0.35", "BC")
			add(&right, "0.35", "BC")
		case "4.0", "4.1":
			add(&left, "0.707", "FL")
			add(&right, "0.707", "FR")
			add(&left, "0.5", "FC")
			add(&right, "0.5", "FC")
			add(&left, "0.35", "BC")
			add(&right, "0.35", "BC")
			if input == "4.1" {
				add(&left, "0.25", "LFE")
				add(&right, "0.25", "LFE")
			}
		case "quad":
			add(&left, "0.707", "FL")
			add(&left, "0.5", "BL")
			add(&right, "0.707", "FR")
			add(&right, "0.5", "BR")
		default:
			return "", fail(UnsupportedGraph, "unsupported audio reduction %s to stereo", input)
		}
		return fmt.Sprintf("pan=stereo|FL=%s|FR=%s,alimiter=limit=0.95:attack=5:release=50", strings.Join(left, "+"), strings.Join(right, "+")), nil
	}
	if output == "mono" {
		if input == "stereo" {
			return "pan=mono|FC=0.5*FL+0.5*FR,alimiter=limit=0.95:attack=5:release=50", nil
		}
		return "", fail(UnsupportedGraph, "unsupported audio reduction %s to mono", input)
	}
	return "", fail(UnsupportedGraph, "unsupported audio reduction %s to %s", input, output)
}
