package naming

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

type corpusCase struct {
	Group, Path, Kind, Title string
	Year                     int
	Season                   *int
	Episodes                 []int
	Absolute                 int
	Date                     string
	Provider, ID             string
	Part                     int
	Edition, Disc            string
	Track, DiscNumber        int
}

func TestRealWorldCorpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []corpusCase
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 600 {
		t.Fatal("corpus too small", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Group+"/"+c.Path, func(t *testing.T) {
			r := Read(c.Path, c.Kind)
			title, year := r.PreferredTitle()
			if title != c.Title || year != c.Year {
				t.Errorf("title/year = %q %d; want %q %d; reading %+v", title, year, c.Title, c.Year, r)
			}
			if len(r.Diagnostics) > 0 {
				t.Errorf("skipped evidence: %v", r.Diagnostics)
			}
			if c.Season != nil && (len(r.Seasons) == 0 || r.Seasons[0].Value != *c.Season) {
				t.Errorf("season: %+v", r.Seasons)
			}
			if c.Episodes != nil && (len(r.Episodes) == 0 || !reflect.DeepEqual(r.Episodes[0].Value, c.Episodes)) {
				t.Errorf("episodes: %+v; want %v", r.Episodes, c.Episodes)
			}
			if c.Absolute != 0 && (len(r.AbsoluteNumbers) == 0 || r.AbsoluteNumbers[0].Value != c.Absolute) {
				t.Errorf("absolute: %+v", r.AbsoluteNumbers)
			}
			if c.Date != "" && (len(r.AirDates) == 0 || r.AirDates[0].Value != c.Date) {
				t.Errorf("date: %+v", r.AirDates)
			}
			if c.Provider != "" {
				found := false
				for _, e := range r.ProviderIDs {
					if e.Value == (ProviderID{c.Provider, c.ID}) {
						found = true
					}
				}
				if !found {
					t.Errorf("provider: %+v", r.ProviderIDs)
				}
			}
			if c.Part != 0 && (len(r.Parts) == 0 || r.Parts[0].Value != c.Part) {
				t.Errorf("part: %+v", r.Parts)
			}
			if c.Edition != "" && (len(r.Editions) == 0 || r.Editions[0].Value != c.Edition) {
				t.Errorf("edition: %+v", r.Editions)
			}
			if c.Disc != "" && (len(r.Discs) == 0 || r.Discs[0].Value.Format != c.Disc) {
				t.Errorf("disc: %+v", r.Discs)
			}
			if c.Track != 0 && (len(r.TrackNumbers) == 0 || r.TrackNumbers[0].Value != c.Track) {
				t.Errorf("track: %+v", r.TrackNumbers)
			}
			if c.DiscNumber != 0 && (len(r.DiscNumbers) == 0 || r.DiscNumbers[0].Value != c.DiscNumber) {
				t.Errorf("disc number: %+v", r.DiscNumbers)
			}
		})
	}
}
func TestEvidenceKeepsAlternativesAndPrecedence(t *testing.T) {
	r := Read("Folder (2000) {tvdb-123}/Season 1/File (2001) {tvdb-456} S02E01.mkv", "tv")
	if len(r.Titles) < 2 || r.Titles[0].Value != "File" || r.Titles[0].Source != FileName || r.Titles[1].Value != "Folder" || r.Titles[1].Source != GrandparentFolder {
		t.Fatalf("titles %+v", r.Titles)
	}
	if len(r.Years) < 2 || r.Years[0].Value != 2001 || r.Years[1].Value != 2000 || len(r.ProviderIDs) != 2 || r.ProviderIDs[0].Value.ID != "456" || r.ProviderIDs[1].Value.ID != "123" {
		t.Fatalf("reading %+v", r)
	}
	if len(r.Seasons) != 2 || r.Seasons[0].Value != 2 || r.Seasons[1].Value != 1 {
		t.Fatal(r.Seasons)
	}
	title, year := r.PreferredTitle()
	if title != "File" || year != 2001 {
		t.Fatal(title, year)
	}
	if len(r.Versions) > 0 {
		t.Fatal(r.Versions)
	}
	for _, e := range r.Titles {
		if e.Path == "" || e.Confidence <= 0 || e.Confidence > 1 {
			t.Fatal(e)
		}
	}
}
func TestUnsafeOrAmbiguousEpisodeEvidence(t *testing.T) {
	for _, p := range []string{"Show 1920x1080.mkv", "Show x264.mkv", "Show H.265.mkv", "Show 2048x1080.mkv", "Show - 2019 [1080p].mkv", "Show - 2020 - 01.mkv", "Show 1080p.mkv", "Show 01.mkv", "Shows1e1.mkv", "S01E12345.mkv", "01.mkv"} {
		r := Read(p, "anime")
		if len(r.Episodes)+len(r.AbsoluteNumbers)+len(r.AirDates) > 0 {
			t.Errorf("%s: %+v", p, r)
		}
	}
	for _, p := range []string{"Show S01E01-E99.mkv", "Show S01E02E01.mkv", "Show S01E01 S02E02.mkv", "News 2023-02-29.mkv"} {
		r := Read(p, "tv")
		if len(r.Diagnostics) == 0 || len(r.Episodes)+len(r.AirDates) > 0 {
			t.Errorf("%s: %+v", p, r)
		}
	}
	r := Read("Show S01E01-E08.mkv", "tv")
	if len(r.Episodes) != 1 || len(r.Episodes[0].Value) != 8 {
		t.Fatal(r)
	}
	for _, p := range []string{strings.Repeat("x", MaxPathBytes+1), strings.Repeat("a/", MaxDepth) + "Film.mkv", "Film\x00.mkv", "../Film.mkv"} {
		if len(Read(p, "movie").Diagnostics) == 0 {
			t.Error("missing bounds")
		}
	}
}
func TestWindowsPathsAndUnicode(t *testing.T) {
	r := Read(`Café (2020)\Staffel 2\Café - S02E01.mkv`, "tv")
	title, year := r.PreferredTitle()
	if title != "Café" || year != 2020 || r.Seasons[0].Value != 2 {
		t.Fatal(r)
	}
}
func FuzzRead(f *testing.F) {
	for _, p := range []string{"Film (2020)/movie.mkv", "Show/Season 2/S02E01-E04.mkv", "[Group] Anime - 012v2.mkv", "../Film.mkv"} {
		f.Add(p)
	}
	f.Fuzz(func(t *testing.T, p string) {
		r := Read(p, "anime")
		for _, e := range r.Episodes {
			if len(e.Value) > MaxEpisodeLinks {
				t.Fatal("unbounded expansion")
			}
		}
	})
}
