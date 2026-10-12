package decoder

import (
	"errors"
	"strings"
	"testing"
)

// Converted audio is resampled only when the plan names an output rate.
func TestConvertedAudioTakesThePlannedSampleRate(t *testing.T) {
	graph, err := BuildConversion(ConversionRequest{ConvertAudio: true, AudioChannels: 2, AudioSampleRate: 48000})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(graph.Audio, " "); got != "-c:a aac -b:a 192k -ac 2 -ar 48000" {
		t.Fatalf("audio arguments %q", got)
	}
	graph, err = BuildConversion(ConversionRequest{ConvertAudio: true, AudioChannels: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(graph.Audio, " "); got != "-c:a aac -b:a 192k -ac 2" {
		t.Fatalf("a source rate that is kept was resampled: %q", got)
	}
	graph, err = BuildConversion(ConversionRequest{CopyAudio: true, AudioSampleRate: 48000})
	if err != nil || strings.Contains(strings.Join(graph.Audio, " "), "-ar") {
		t.Fatalf("copied audio was given a sample rate: %v %v", graph.Audio, err)
	}
	if _, err = BuildConversion(ConversionRequest{ConvertAudio: true, AudioSampleRate: 96000}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("an output rate above 48 kHz was accepted: %v", err)
	}
	// AC-3 and E-AC-3 are encoded at 32, 44.1 or 48 kHz and nothing else.
	for _, codec := range []string{"ac3", "eac3"} {
		graph, err = BuildConversion(ConversionRequest{ConvertAudio: true, AudioCodec: codec, AudioChannels: 6, AudioBitrateBPS: 640_000, AudioSampleRate: 44100})
		if err != nil || !strings.HasSuffix(strings.Join(graph.Audio, " "), "-c:a "+codec+" -b:a 640k -ac 6 -ar 44100") {
			t.Fatalf("%s at 44.1 kHz: %v %v", codec, graph.Audio, err)
		}
		if _, err = BuildConversion(ConversionRequest{ConvertAudio: true, AudioCodec: codec, AudioChannels: 6, AudioSampleRate: 24000}); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("%s was given a rate it cannot be encoded at: %v", codec, err)
		}
	}
}
