package metadataprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

func TestAniListOriginalLanguageFromActualDetailsResponse(t *testing.T) {
	p := NewAniList()
	p.http = fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		if r.Method != http.MethodPost {
			t.Error("expected provider GraphQL request")
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body.Query, "countryOfOrigin") {
			t.Error("original-language fact was not requested")
		}
		fmt.Fprint(w, `{"data":{"Media":{"id":42,"type":"ANIME","countryOfOrigin":"JP","title":{"english":"English title","romaji":"Romaji title","native":"日本語"}}}}`)
	})
	r, err := p.ScreenDetails(context.Background(), "anime", "42", "en-US", "US")
	if err != nil || r.Title != "English title" || r.Language != "ja" {
		t.Fatal("provider original language followed title preference", r, err)
	}
}
