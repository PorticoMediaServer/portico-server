package catalogkind

import "testing"

func TestPublicSeparatesStorageAndEntityKinds(t *testing.T) {
	tests := map[string]EntityKind{
		"movie": Movie, "anime": Show, "audiobook": Book,
		"live_channel": LiveChannel, "live_program": LiveProgram,
		" audiobook ": Book, "server": Unsupported, "": Unsupported,
	}
	for storage, want := range tests {
		if got := Public(storage); got != want {
			t.Errorf("Public(%q) = %q, want %q", storage, got, want)
		}
	}
}

func TestPublicVocabularyIsExplicitAndUnique(t *testing.T) {
	want := []string{"movie", "show", "season", "episode", "special", "artist", "album", "track", "author", "audiobook-series", "book", "chapter", "recording", "live-channel", "live-program", "person", "collection", "playlist", "category", "extra", "unsupported"}
	got := PublicKinds()
	if len(got) != len(want) {
		t.Fatalf("PublicKinds() count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("PublicKinds()[%d] = %q, want %q", i, got[i], want[i])
		}
		if !IsPublic(got[i]) {
			t.Errorf("published kind %q is not recognized", got[i])
		}
	}
}
