package preparedmedia

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"portico.local/server/internal/audiotags"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/decodertest"
	"portico.local/server/internal/testtier"
)

func TestMusicSourceFactsDoesNotGuessWavPackModeFromCodecOrTags(t *testing.T) {
	for _, mode := range []*bool{nil, booleanMusicMode(true), booleanMusicMode(false)} {
		original := probed{Facts: Facts{AudioCodec: "wavpack"}}
		got := musicSourceFacts(original, audiotags.Tags{Values: map[string][]string{"lossless": {"true"}}, Lossless: mode})
		if got.AudioLossless != mode {
			t.Fatal("actual header mode lost", got)
		}
	}
	got := musicSourceFacts(probed{Facts: Facts{AudioCodec: "mp3"}}, audiotags.Tags{Lossless: booleanMusicMode(true)})
	if got.AudioLossless != nil {
		t.Fatal("WavPack metadata claimed another codec lossless")
	}
}
func booleanMusicMode(value bool) *bool { return &value }

func TestMusicWavPackLosslessLowBitrateUsesRequestedActualEncode(t *testing.T) {
	testtier.Media(t, "generates actual low-bitrate lossless WavPack and exercises prepared encoder")
	ffmpeg, ffprobe := decodertest.QualifiedFFmpeg(t), decodertest.QualifiedFFprobe(t)
	path := filepath.Join(t.TempDir(), "source.wv")
	cmd := exec.Command(ffmpeg, "-v", "error", "-nostdin", "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "1", "-c:a", "wavpack", path)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(data))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tags := audiotags.Read(bytes.NewReader(data), int64(len(data)))
	if tags.Lossless == nil || !*tags.Lossless {
		t.Fatal("actual WavPack lossless header missing")
	}
	probe, err := exec.Command(ffprobe, "-v", "error", "-show_format", "-show_streams", "-of", "json", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	original, err := parseProbeMode(probe, true)
	if err != nil {
		t.Fatal(err)
	}
	if original.AudioCodec != "wavpack" || original.Duration <= 0 {
		t.Fatal("actual WavPack timing missing", original.AudioCodec, original.Duration)
	}
	// WavPack does not report a stream bitrate. Download selection measures
	// file bytes over its validated duration; keep that distinct from a
	// nonexistent ffprobe stream-bit-rate assertion.
	measuredByteRate := int(float64(len(data)) * 8 / original.Duration)
	if measuredByteRate <= 0 || measuredByteRate >= 96000 {
		t.Fatal("fixture is not low-byte-rate lossless audio", measuredByteRate)
	}
	original = musicSourceFacts(original, tags)
	source := original.recipeSource()
	source.AudioBitrateBPS = measuredByteRate
	bridge := httptest.NewServer(http.FileServer(http.Dir(filepath.Dir(path))))
	defer bridge.Close()
	for _, codec := range []string{"aac", "opus"} {
		recipe := decoder.PreparedRecipe{AudioOnly: true, MusicQuality: "320", MusicCodec: codec, Source: source}
		format, err := decoder.PreparedMusicFormat(recipe)
		if err != nil || format.BitrateBPS != 320000 {
			t.Fatal("known lossless source was bitrate-clamped", codec, format, err)
		}
		// Use the real prepared recipe's unchanged HTTP input allowlist.
		args, err := decoder.PreparedArgs(bridge.URL+"/source.wv", recipe)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for i, arg := range args {
			if arg == "-b:a" && i+1 < len(args) && args[i+1] == strconv.Itoa(320000) {
				found = true
			}
		}
		if !found {
			t.Fatal("actual encoder missed requested bitrate", args)
		}
		var diagnostic bytes.Buffer
		encode := exec.Command(ffmpeg, args...)
		encode.Stderr = &diagnostic
		encoded, err := encode.Output()
		if err != nil || len(encoded) == 0 {
			t.Fatal(codec, err, diagnostic.String())
		}
		output := filepath.Join(filepath.Dir(path), codec+".media")
		if err := os.WriteFile(output, encoded, 0600); err != nil {
			t.Fatal(err)
		}
		if log, err := exec.Command(ffmpeg, "-v", "error", "-xerror", "-i", output, "-map", "0:a:0", "-f", "null", "-").CombinedOutput(); err != nil {
			t.Fatal("actual prepared output did not decode", codec, err, string(log))
		}
	}
}
