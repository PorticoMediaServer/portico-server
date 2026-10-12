package catalog

import "testing"

func TestFilenameTitleKeepsAYearThatBelongsToTheTitle(t *testing.T) {
	for _, c := range []struct {
		path, title string
		year        int
	}{
		// A year in brackets is the release year; an earlier number is title.
		{"Blade Runner 2049 (2017).mkv", "Blade Runner 2049", 2017},
		{"Wonder Woman 1984 (2020).mkv", "Wonder Woman 1984", 2020},
		{"Death Race 2000 [1975].mkv", "Death Race 2000", 1975},
		{"The Wolf of Wall Street 2001 (2013).mkv", "The Wolf of Wall Street 2001", 2013},
		{"1917 (2019).mkv", "1917", 2019},
		{"2001 A Space Odyssey (1968).mkv", "2001 A Space Odyssey", 1968},
		// A year directly after another is the release year.
		{"Blade.Runner.2049.2017.1080p.BluRay.mkv", "Blade Runner 2049", 2017},
		{"Death Race 2000 1975.mkv", "Death Race 2000", 1975},
		// Everything else reads as it did.
		{"A.Movie.2020.1080p.BluRay.x265.mkv", "A Movie", 2020},
		{"Film (2020) 2160p.mkv", "Film", 2020},
		{"Film.1999.Remastered.2019.mkv", "Film", 1999},
		{"Blade Runner (1982) The Final Cut 2007.mkv", "Blade Runner", 1982},
		{"Show 1999-2004.mkv", "Show", 1999},
		{"Blade Runner 2049.mkv", "Blade Runner", 2049},
		{"1917.mkv", "1917", 0},
		{"Plain Title.mkv", "Plain Title", 0},
	} {
		title, year := FilenameTitle(c.path)
		if title != c.title || year != c.year {
			t.Errorf("%s: got %q %d, want %q %d", c.path, title, year, c.title, c.year)
		}
	}
}

func TestShowFolderKeepsAYearThatBelongsToTheTitle(t *testing.T) {
	got := ParseEpisode("Space 1999 (1975)/Season 1/Space 1999 (1975) - S01E01.mkv", "tv")
	if got.Issue != "" || got.ShowTitle != "Space 1999" || got.Year != 1975 {
		t.Fatalf("unexpected parse %+v", got)
	}
}
