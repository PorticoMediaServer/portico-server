package catalog

import "testing"

func TestEpisodeThumbnailModeVocabularyAndDefaults(t *testing.T) {
	for _, kind := range []string{"tv", "anime"} {
		if DefaultEpisodeThumbnailMode(kind) != "provider_then_made" {
			t.Fatalf("wrong %s default", kind)
		}
	}
	for _, kind := range []string{"movie", "music", "audiobook", "book", "comic", "", "unknown"} {
		if DefaultEpisodeThumbnailMode(kind) != "off" {
			t.Fatalf("wrong %s default", kind)
		}
	}
	for _, mode := range []string{"provider_then_made", "provider", "off"} {
		if !ValidEpisodeThumbnailMode(mode) {
			t.Fatalf("rejected %s", mode)
		}
	}
	for _, mode := range []string{"", "provider_then_generated", "provider_only", "Provider", " made", "generated"} {
		if ValidEpisodeThumbnailMode(mode) {
			t.Fatalf("accepted unknown/alias %s", mode)
		}
	}
}
