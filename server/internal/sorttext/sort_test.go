package sorttext

import "testing"

func TestMetadataSortKeys(t *testing.T) {
	for _, x := range []struct{ title, explicit, language, key, letter string }{
		{"The Éclair", "", "en-US", "eclair", "E"}, {"L’Été", "", "fr", "ete", "E"}, {"Die Straße", "", "de", "strasse", "S"},
		{"The Abbey", "Zulu", "en", "zulu", "Z"}, {"The Abbey", "", "", "abbey", "A"}, {"日本語", "", "ja", "日本語", "日"}, {"Москва", "", "ru", "москва", "М"}, {"42", "", "en", "42", "#"},
	} {
		if got := Key(x.title, x.explicit, x.language); got != x.key {
			t.Errorf("key %q: %q", x.title, got)
		}
		if got := Letter(x.key); got != x.letter {
			t.Errorf("letter %q: %q", x.title, got)
		}
	}
}
