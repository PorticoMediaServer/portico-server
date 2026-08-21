package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScannerKeepsSameTitleMoviesFromDifferentYearsSeparate(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	for _, name := range []string{"The Thing (1951).mkv", "The Thing (1982).mkv"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatal(err)
	}
	rows, err := server.db.Query(`SELECT year, COUNT(*) FROM media_items WHERE library_id = ? AND type = 'movie' GROUP BY year ORDER BY year`, library.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	years := []int{}
	for rows.Next() {
		var year, count int
		if err := rows.Scan(&year, &count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("year %d merged %d movies", year, count)
		}
		years = append(years, year)
	}
	if len(years) != 2 || years[0] != 1951 || years[1] != 1982 {
		t.Fatalf("years = %#v", years)
	}
}

func TestScannerUnnumberedEpisodesDoNotCollapseIntoS01E01(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	dir := filepath.Join(root, "Documentary", "Season 01")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Arrival.mkv", "Departure.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Shows", Type: "show", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatal(err)
	}
	var count, numbered int
	if err := server.db.QueryRow(`SELECT COUNT(*), SUM(CASE WHEN episode_number = 1 THEN 1 ELSE 0 END) FROM media_items WHERE library_id = ? AND type = 'episode'`, library.ID).Scan(&count, &numbered); err != nil {
		t.Fatal(err)
	}
	if count != 2 || numbered != 0 {
		t.Fatalf("episodes=%d inferred_s01e01=%d", count, numbered)
	}
}

func TestScannerDiscoversVOBFLVAndF4V(t *testing.T) {
	for _, ext := range []string{".vob", ".flv", ".f4v"} {
		if !isMediaFileForLibrary("movie", "movie"+ext) {
			t.Fatalf("%s was not recognized as video", ext)
		}
	}
}

func TestScannerRecognizesBroadFFmpegContainerSet(t *testing.T) {
	for _, ext := range []string{".3gp", ".asf", ".divx", ".dv", ".m2v", ".mxf", ".ogm", ".rmvb", ".wtv"} {
		if !isMediaFileForLibrary("movie", "movie"+ext) {
			t.Fatalf("%s was not recognized as video", ext)
		}
	}
	for _, ext := range []string{".ape", ".caf", ".dff", ".dsf", ".mka", ".mpc", ".oga", ".ra", ".tta", ".wv"} {
		if !isMediaFileForLibrary("music", "track"+ext) {
			t.Fatalf("%s was not recognized as audio", ext)
		}
	}
}

func TestScannerReportsMediaFilesRejectedByLibraryType(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Wrong Library.mp3"), []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := server.performLibraryScan(library, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesSkipped != 1 {
		t.Fatalf("skipped = %d, expected 1", result.FilesSkipped)
	}
	var warnings string
	if err := server.db.QueryRow(`SELECT warnings_json FROM library_scan_runs WHERE library_id = ? ORDER BY started_at DESC LIMIT 1`, library.ID).Scan(&warnings); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings, ".mp3 (1)") || !strings.Contains(warnings, "not accepted") {
		t.Fatalf("warnings = %s", warnings)
	}
}

func TestScannerCataloguesLogicalDiscSourcesWithoutClaimingPlayback(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	for _, dir := range []string{filepath.Join(root, "DVD Film (2001)", "VIDEO_TS"), filepath.Join(root, "Blu Film (2002)", "BDMV")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "Image Film (2003).iso"), []byte("not-mounted"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Movies", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatal(err)
	}
	rows, err := server.db.Query(`SELECT source_type, source, version_label FROM media_files WHERE library_id = ? ORDER BY source_type`, library.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var sourceType, reason, label string
		if err := rows.Scan(&sourceType, &reason, &label); err != nil {
			t.Fatal(err)
		}
		found[sourceType] = true
		if !strings.Contains(strings.ToLower(reason), "playback requires") || !strings.Contains(label, "playback unavailable") {
			t.Fatalf("source %s was not truthful: reason=%q label=%q", sourceType, reason, label)
		}
	}
	for _, sourceType := range []string{"disc-image", "dvd-structure", "bluray-structure"} {
		if !found[sourceType] {
			t.Fatalf("missing logical source %s: %#v", sourceType, found)
		}
	}
	item := MediaItem{Type: "movie", FileCount: 1, MediaFiles: []MediaFileVersion{{SourceType: "disc-image", Available: true}}}
	if mediaItemHasPlayableFile(item) {
		t.Fatal("catalogued unsupported disc image claimed playback")
	}
}
