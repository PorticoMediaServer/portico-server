package ingestion

import (
	"context"
	"os"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/storage"
	"testing"
)

// The scan lists every directory before it analyses a file in it. What it saw
// there settles whether a sidecar can exist, without asking storage again.
func TestScanRemembersWhichSidecarsADirectoryHolds(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"Show/Season 1", "Bare"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"Show/Poster.JPG", "Show/tvshow.nfo", "Show/Season 1/notes.txt", "Show/Season 1/episode.mkv"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cat := catalog.New(db)
	lib, err := cat.Create("TV", "tv", root)
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, cat, assets.Probe{})
	job, err := s.Queue(lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	source, err := cat.LibrarySource(context.Background(), lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	at := func(relative string) string { return filepath.Join(source.ResolvedPath, relative) }
	if _, known := s.sidecarListed(job.ID, source, at("Show/poster.jpg")); known {
		t.Fatal("a directory was called known before the scan listed it")
	}
	s.process(context.Background(), job.ID, lib.ID)
	for _, c := range []struct {
		path           string
		present, known bool
	}{
		// Seen, whatever the letter case on disk.
		{"Show/poster.jpg", true, true},
		{"Show/tvshow.nfo", true, true},
		// Listed completely and not there.
		{"Show/fanart.jpg", false, true},
		{"Show/Season 1/poster.jpg", false, true},
		{"Show/Season 1/episode.nfo", false, true},
		{"Bare/logo.png", false, true},
		// Not a kind of file the scan keeps track of: storage decides.
		{"Show/Season 1/notes.txt", false, false},
		{"Show/Season 1/episode.srt", false, false},
		// Not a directory this scan listed.
		{"Elsewhere/poster.jpg", false, false},
	} {
		present, known := s.sidecarListed(job.ID, source, at(c.path))
		if present != c.present || known != c.known {
			t.Errorf("%s: present=%v known=%v, want %v %v", c.path, present, known, c.present, c.known)
		}
	}
	// Another scan knows nothing of this one's listing.
	if _, known := s.sidecarListed("another-job", source, at("Show/fanart.jpg")); known {
		t.Fatal("one scan's listing answered for another")
	}
}

// A directory whose listing was not seen from its first page to its last is
// never treated as known.
func TestPartialDirectoryListingIsNotTrusted(t *testing.T) {
	s := New(nil, nil, assets.Probe{})
	source := catalog.LibrarySource{ResolvedPath: "/library"}
	page := func(complete bool, names ...string) storage.InventoryPage {
		p := storage.InventoryPage{Complete: complete}
		for _, name := range names {
			p.Entries = append(p.Entries, storage.Snapshot{Name: name})
		}
		return p
	}
	s.noteDirectoryPage("job", "Big", "", page(false, "a.mkv"))
	if _, known := s.sidecarListed("job", source, "/library/Big/poster.jpg"); known {
		t.Fatal("a directory still being listed was called known")
	}
	s.noteDirectoryPage("job", "Big", "cursor-1", page(true, "poster.jpg"))
	if present, known := s.sidecarListed("job", source, "/library/Big/poster.jpg"); !present || !known {
		t.Fatal("a directory listed over two pages lost a name", present, known)
	}
	// A continuation whose first page this process never saw (a restart).
	s.noteDirectoryPage("job", "Resumed", "cursor-9", page(true, "b.mkv"))
	if _, known := s.sidecarListed("job", source, "/library/Resumed/poster.jpg"); known {
		t.Fatal("a listing joined part-way was called complete")
	}
	// Listing a directory again replaces what was known about it.
	s.noteDirectoryPage("job", "Big", "", page(true))
	if present, known := s.sidecarListed("job", source, "/library/Big/poster.jpg"); present || !known {
		t.Fatal("a fresh listing did not replace the old one", present, known)
	}
}
