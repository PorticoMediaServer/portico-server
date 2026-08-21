package app

import (
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestIrregularGOPVisualClockConformsToFullTimelineHLS exercises the real
// producer with a source whose keyframes do not align to Portico's four-second
// HLS grid. The five boxes burned into the source encode floor(PTS) as a binary
// visual clock, so the fixture remains human-inspectable even on FFmpeg builds
// that do not include the drawtext filter.
func TestIrregularGOPVisualClockConformsToFullTimelineHLS(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not available")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not available")
	}

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "irregular-gop-visual-clock.mp4")
	visualClock := strings.Join([]string{
		"drawgrid=w=40:h=40:t=1:c=white@0.25",
		"drawbox=x=10:y=10:w=32:h=32:color=white:t=fill:enable='eq(mod(floor(t),2),1)'",
		"drawbox=x=50:y=10:w=32:h=32:color=yellow:t=fill:enable='eq(mod(floor(t/2),2),1)'",
		"drawbox=x=90:y=10:w=32:h=32:color=cyan:t=fill:enable='eq(mod(floor(t/4),2),1)'",
		"drawbox=x=130:y=10:w=32:h=32:color=magenta:t=fill:enable='eq(mod(floor(t/8),2),1)'",
		"drawbox=x=170:y=10:w=32:h=32:color=lime:t=fill:enable='eq(mod(floor(t/16),2),1)'",
	}, ",")
	runCommand(t, "generate irregular-GOP visual-clock fixture", ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=13",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=13",
		"-vf", visualClock,
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-g", "312", "-keyint_min", "312", "-sc_threshold", "0",
		"-force_key_frames", "0,0.7,2.9,3.2,7.75,8.1,12.4",
		"-c:a", "aac", "-shortest",
		sourcePath,
	)

	sourceKeyframes := conformanceKeyframeTimes(t, ffprobePath, sourcePath)
	if len(sourceKeyframes) < 7 {
		t.Fatalf("source keyframes = %v, expected irregular forced keyframes", sourceKeyframes)
	}
	if keyframeIntervalsAreRegular(sourceKeyframes, 0.12) {
		t.Fatalf("source fixture accidentally has a regular GOP: %v", sourceKeyframes)
	}

	server := newScannerTestServer(t)
	server.cfg.FFmpegPath = ffmpegPath
	settings := transcodeSettings{
		Enabled:            true,
		TemporaryDirectory: filepath.Join(tempDir, "hls"),
		DirectStreamRemux:  true,
		X264Preset:         "ultrafast",
	}
	item := MediaItem{
		ID:              "movie_irregular_gop_visual_clock",
		DurationSeconds: 13,
		Streams: []Stream{
			{ID: "visual_clock_video", Kind: "video", Codec: "h264", Width: 320, Height: 180},
			{ID: "visual_clock_audio", Kind: "audio", Codec: "aac", Channels: 2},
		},
	}
	session, err := server.startTranscodeLocked("user", item, sourcePath, "original", settings, "", 0, "", "", false, false)
	if err != nil {
		t.Fatalf("start irregular-GOP conformance transcode: %v", err)
	}
	defer session.stop(0)
	waitForTranscodeDone(t, session)
	assertHLSManifestStartsNearZero(t, ffprobePath, session.manifest)

	outputKeyframes := conformanceKeyframeTimes(t, ffprobePath, session.manifest)
	for _, expected := range []float64{0, 4, 8, 12} {
		if !containsTimeNear(outputKeyframes, expected, 0.12) {
			t.Fatalf("HLS keyframes %v did not include the %.0fs timeline-grid boundary", outputKeyframes, expected)
		}
	}

	outputDuration := conformanceDuration(t, ffprobePath, session.manifest)
	if math.Abs(outputDuration-13) > 0.25 {
		t.Fatalf("HLS duration = %.3fs, expected the complete 13s timeline", outputDuration)
	}
}

func conformanceKeyframeTimes(t *testing.T, ffprobePath, input string) []float64 {
	t.Helper()
	output, err := exec.Command(ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_frames",
		"-show_entries", "frame=key_frame,best_effort_timestamp_time",
		"-of", "csv=p=0",
		input,
	).Output()
	if err != nil {
		t.Fatalf("probe conformance keyframes for %s: %v", input, err)
	}
	var result []float64
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) < 2 || strings.TrimSpace(fields[0]) != "1" {
			continue
		}
		value, parseErr := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
		if parseErr == nil {
			result = append(result, value)
		}
	}
	return result
}

func conformanceDuration(t *testing.T, ffprobePath, input string) float64 {
	t.Helper()
	output, err := exec.Command(ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1",
		input,
	).Output()
	if err != nil {
		t.Fatalf("probe conformance duration for %s: %v", input, err)
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		t.Fatalf("parse conformance duration %q: %v", strings.TrimSpace(string(output)), err)
	}
	return value
}

func keyframeIntervalsAreRegular(times []float64, tolerance float64) bool {
	if len(times) < 3 {
		return true
	}
	interval := times[1] - times[0]
	for index := 2; index < len(times); index++ {
		if math.Abs((times[index]-times[index-1])-interval) > tolerance {
			return false
		}
	}
	return true
}

func containsTimeNear(values []float64, expected, tolerance float64) bool {
	for _, value := range values {
		if math.Abs(value-expected) <= tolerance {
			return true
		}
	}
	return false
}
