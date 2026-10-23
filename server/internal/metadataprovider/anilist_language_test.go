package metadataprovider

import (
	"strings"
	"testing"
)

func TestAniListOriginalLanguageUsesCountryNotTitlePreference(t *testing.T) {
	if !strings.Contains(animeFields, "countryOfOrigin") {
		t.Fatal("countryOfOrigin not requested")
	}
	for country, want := range map[string]string{"JP": "ja", "KR": "ko", "CN": "zh", "TW": "zh", "US": "", "": ""} {
		media := aniMedia{ID: 42, Type: "ANIME", CountryOfOrigin: country, Title: aniTitle{English: "Title", Native: "Native", Romaji: "Romaji"}}
		for _, locale := range []string{"en", "ja", "romaji"} {
			record, err := media.record(locale)
			if err != nil || record.Language != want {
				t.Fatalf("country %s title preference %s: %q %v", country, locale, record.Language, err)
			}
		}
	}
}
