package metadata

import (
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/metadataprovider"
)

func TestScreenFilenameEvidenceAndEdition(t *testing.T) {
	for _, tc := range []struct {
		raw, title string
		year       int
	}{
		{"[Group] A.Movie.2020.1080p.BluRay.x265.mkv", "A Movie", 2020},
		{"A_Show.S02E03.1080p.mkv", "A Show", 0},
		{"[Group] Anime - 014v2 [1080p].mkv", "Anime", 0},
	} {
		title, year, _ := screenFilename(tc.raw)
		if title != tc.title || year != tc.year {
			t.Fatal(tc.raw, title, year)
		}
	}
	title, year, edition := screenFilename("Film (2020) {edition-Extended Cut} 2160p.mkv")
	if title != "Film" || year != 2020 || edition != "Extended Cut" {
		t.Fatal(title, year, edition)
	}
	if normalizedScreenName("Amélie") != normalizedScreenName("Amelie") {
		t.Fatal("accent normalization")
	}
	for _, q := range []string{"/media/private/Film.mkv", "https://private.example/movie", "C:\\media\\Film", "a\nquery"} {
		if screenQuerySafe(q) {
			t.Fatal("path/query disclosure", q)
		}
	}
}
func TestScreenConfidenceRequiresVerifiedDetailsSignalsAndMargin(t *testing.T) {
	record := metadataprovider.ScreenRecord{Identity: metadataprovider.ScreenID{Provider: "tmdb", Type: "movie", ID: "1"}, Title: "Film", Year: 2020}
	top := scoreScreen(record, []string{"Film"}, 2020, false)
	if i, _ := screenWinner([]screenCandidate{top}); i != -1 {
		t.Fatal("search-only preview accepted")
	}
	top.DetailsVerified = true
	if i, _ := screenWinner([]screenCandidate{top}); i != 0 {
		t.Fatal("strong verified match rejected")
	}
	other := top
	other.Record.Identity.ID = "2"
	if i, _ := screenWinner([]screenCandidate{top, other}); i != -1 {
		t.Fatal("ambiguous remake accepted")
	}
	missing := scoreScreen(record, []string{"Film"}, 0, false)
	missing.DetailsVerified = true
	if i, _ := screenWinner([]screenCandidate{missing}); i != -1 {
		t.Fatal("one-signal match accepted")
	}
	conflict := scoreScreen(record, []string{"Film"}, 2021, false)
	conflict.DetailsVerified = true
	if i, _ := screenWinner([]screenCandidate{conflict}); i != -1 {
		t.Fatal("contradicting year accepted")
	}
	exact := scoreScreen(record, nil, 0, true)
	if i, _ := screenWinner([]screenCandidate{exact}); i != 0 {
		t.Fatal("validated exact ID rejected")
	}
}
func TestScreenLocalYearsAndNFOEpisodeCoordinates(t *testing.T) {
	if !screenLocalYearConflict(screenBase{Year: 2020, NFO: []assets.VideoNFO{{Year: 2021}}}) {
		t.Fatal("conflicting local facts hidden")
	}
	s, n := 2, 3
	b := screenBase{Numbering: "seasonal", Season: 2, Number: 3}
	if !screenNFOCoordinates(assets.VideoNFO{Season: &s, Episode: &n}, b) {
		t.Fatal("exact coordinate rejected")
	}
	s = 1
	if screenNFOCoordinates(assets.VideoNFO{Season: &s, Episode: &n}, b) {
		t.Fatal("different local season matched")
	}
}
