package app

import "testing"

func TestPlaybackSubtitleStreamsExposeEmbeddedTextWithoutMutatingScannedMetadata(t *testing.T) {
	mediaID := "episode_subtitles"
	original := []Stream{
		{ID: "sub_none", Kind: "subtitle", DisplayTitle: "None"},
		{ID: mediaID + "_probe_2", SourceKind: "ffprobe", Index: 2, Kind: "subtitle", Codec: "subrip", DisplayTitle: "English"},
		{ID: mediaID + "_probe_3", SourceKind: "ffprobe", Index: 3, Kind: "subtitle", Codec: "hdmv_pgs_subtitle", DisplayTitle: "English PGS"},
		{ID: "sidecar", Kind: "subtitle", Codec: "webvtt", SourceURL: "/api/media/episode_subtitles/subtitles/sidecar", DisplayTitle: "Sidecar"},
	}

	prepared := playbackSubtitleStreams(mediaID, original)
	if original[1].SourceURL != "" {
		t.Fatalf("scanned stream metadata was mutated: %#v", original[1])
	}
	if prepared[1].SourceURL != "/api/media/episode_subtitles/subtitles/episode_subtitles_probe_2" {
		t.Fatalf("embedded text subtitle URL = %q", prepared[1].SourceURL)
	}
	if prepared[2].SourceURL != "" {
		t.Fatalf("image subtitle must remain a burn-in selection: %#v", prepared[2])
	}
	if prepared[3].SourceURL != original[3].SourceURL {
		t.Fatalf("managed subtitle URL changed: %q", prepared[3].SourceURL)
	}
}
