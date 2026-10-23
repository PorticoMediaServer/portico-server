package metadataprovider

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestAniListTitlePreferenceUsesSameCachedProviderEvidence(t *testing.T) {
	p := NewAniList()
	calls := 0
	p.http = fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"data":{"Media":{"id":7,"type":"ANIME","title":{"english":"English title","romaji":"Romaji title","native":"日本語"},"description":"Description"}}}`)
	})
	for _, tc := range []struct{ mode, locale, want string }{{"english", "ja", "English title"}, {"romaji", "en", "Romaji title"}, {"native", "en", "日本語"}, {"language", "ja", "日本語"}} {
		r, e := p.ScreenDetails(WithScreenPreferences(context.Background(), "blank", tc.mode), "anime", "7", tc.locale, "US")
		if e != nil || r.Title != tc.want || r.OriginalTitle != "日本語" {
			t.Fatal(tc, r, e)
		}
	}
	if calls != 1 {
		t.Fatal("preference change duplicated the same provider evidence", calls)
	}
}
func TestTMDBFallbackUsesSameIdentityAndRecordsActualFieldLanguage(t *testing.T) {
	p, _ := NewTMDB("private-token")
	calls := 0
	p.http = fixtureTransport(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		locale := r.URL.Query().Get("language")
		overview := ""
		if locale == "en" {
			overview = "English description"
		}
		if locale == "ja" {
			overview = "Original description"
		}
		fmt.Fprintf(w, `{"id":42,"title":"Film","original_title":"Original","original_language":"ja","overview":%q}`, overview)
	})
	for _, tc := range []struct{ fallback, want, fieldLanguage string }{{"en", "English description", "en"}, {"original", "Original description", "ja"}, {"blank", "", ""}} {
		r, e := p.ScreenDetails(WithScreenPreferences(context.Background(), tc.fallback, "language"), "movie", "42", "fr", "CA")
		if e != nil || r.Overview != tc.want || r.Language != "ja" || r.FieldLanguages["overview"] != tc.fieldLanguage {
			t.Fatal(tc, r, e)
		}
	}
	if calls != 3 {
		t.Fatal("fallback was not bounded/cached", calls)
	}
}
