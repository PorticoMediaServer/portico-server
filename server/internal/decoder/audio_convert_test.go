package decoder

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"portico.local/server/internal/audiofacts"
)

// Spec §18.1 converted mode is exact: the recipe cuts the source's raw decoded
// frames at the facts' trim, resamples, and pads or cuts to exactly the plan's
// frame count. The output, measured back, has exactly those frames: FLAC with
// no trim, Opus with its 312-frame pre-skip. A seek (fromFrame) is the same
// recipe from a later source frame. (Unconfined FFmpeg here; the confinement is
// the same as every analysis run's.)
func TestAudioConversionIsExact(t *testing.T) {
	ffmpeg, e1 := exec.LookPath("ffmpeg")
	ffprobe, e2 := exec.LookPath("ffprobe")
	if e1 != nil || e2 != nil {
		t.Skip("ffmpeg/ffprobe not installed")
	}
	dir := t.TempDir()
	server := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer server.Close()
	measure := func(name string) audiofacts.Facts {
		t.Helper()
		var out, stderr bytes.Buffer
		cmd := exec.Command(ffprobe, audiofacts.Args(server.URL+"/"+name)...)
		cmd.Stdout, cmd.Stderr = &out, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v %s", name, err, stderr.String())
		}
		f, err := audiofacts.Parse(&out)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return f
	}
	// Sources with priming and padding: LAME MP3 at 44.1 kHz and Opus at 48 kHz.
	for _, src := range []struct {
		name, codec string
		rate        int
	}{{"src.mp3", "libmp3lame", 44100}, {"src.opus", "libopus", 48000}} {
		frames := int64(3*src.rate + 777)
		expr := fmt.Sprintf("0.5*sin(2*PI*441*n/%d)", src.rate)
		if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", fmt.Sprintf("aevalsrc=%s|%s:s=%d:n=4096", expr, expr, src.rate), "-af", fmt.Sprintf("atrim=end_sample=%d", frames), "-c:a", src.codec, filepath.Join(dir, src.name)).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", src.name, err, out)
		}
		facts := measure(src.name)
		if facts.DurationFrames != frames {
			t.Fatalf("%s facts %s", src.name, facts)
		}
		for _, target := range []struct {
			kind          string
			rate, depth   int
			bitrate       int
			from          int64
			wantStartTrim int64
		}{{"audio_flac", src.rate, 16, 0, 0, 0}, {"audio_flac", 48000, 24, 0, 1000, 0}, {"audio_opus", 48000, 0, 96000, 0, 312}, {"audio_opus", 48000, 0, 64000, 4800, 312}} {
			out := int64(float64(frames)*float64(target.rate)/float64(src.rate) + 0.5)
			skip := int64(float64(target.from)*float64(src.rate)/float64(target.rate) + 0.5)
			spec := AnalysisSpec{Kind: target.kind, MaxDurationUS: 10_000_000, AudioStart: facts.StartFrames + skip, AudioFrames: facts.DurationFrames - skip, AudioOut: out - target.from, AudioRate: target.rate, AudioChannels: 2, AudioBitDepth: target.depth, AudioBitrate: target.bitrate}
			args, err := AnalysisArgs(server.URL+"/"+src.name, spec)
			if err != nil {
				t.Fatal(err)
			}
			name := fmt.Sprintf("%s-%s-%d-%d.out", src.name, target.kind, target.rate, target.from)
			file, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd := exec.Command(ffmpeg, args...)
			cmd.Stdout, cmd.Stderr = file, &stderr
			err = cmd.Run()
			file.Close()
			if err != nil {
				t.Fatalf("%s: %v %s", name, err, stderr.String())
			}
			got := measure(name)
			if got.DurationFrames != out-target.from || got.StartFrames != target.wantStartTrim || got.SampleRate != target.rate || got.Channels != 2 {
				t.Errorf("%s: %s, want %d frames trimmed from %d", name, got, out-target.from, target.wantStartTrim)
			}
			t.Logf("%s: %s", name, got)
		}
	}
}
