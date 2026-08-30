package app

import "testing"

func TestPlaybackAudioSourceLabelUsesAnalyzedStream(t *testing.T) {
	item := MediaItem{Type: "movie", Streams: []Stream{
		{ID: "video", Kind: "video", Codec: "h264", DisplayTitle: "H.264 Main"},
		{ID: "audio", Kind: "audio", Codec: "aac", Channels: 2, DisplayTitle: "AAC Stereo"},
	}}
	if got := playbackAudioSourceLabel(item); got != "AAC Stereo" {
		t.Fatalf("audio source=%q", got)
	}
}

func TestPlaybackAudioSourceLabelDoesNotInventAudio(t *testing.T) {
	if got := playbackAudioSourceLabel(MediaItem{Type: "movie"}); got != "No audio" {
		t.Fatalf("silent source=%q", got)
	}
}
