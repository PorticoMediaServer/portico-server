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
	"time"
)

// The scan lists every directory before it analyses a file in it. What it saw
// there settles whether a sidecar can exist, for as long as the directory is
// as it was listed.
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
	// Each call stands for the analysis of one file.
	listed := func(job, relative string) (bool, bool) {
		return s.scanListed(context.Background(), job, source)(filepath.Join(source.ResolvedPath, relative))
	}
	if _, known := listed(job.ID, "Show/poster.jpg"); known {
		t.Fatal("a directory was called known before the scan listed it")
	}
	// Up to the point where every folder has been listed and analysis is next.
	for n := 0; ; n++ {
		var phase string
		if err = db.QueryRow(`SELECT phase FROM inventory_runs WHERE job_id=?`, job.ID).Scan(&phase); err != nil {
			t.Fatal(err)
		}
		if phase == "basic_pending" {
			break
		}
		if n == 64 {
			t.Fatal("scan never reached analysis:", phase)
		}
		s.processQuantum(context.Background(), job.ID, lib.ID)
	}
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
		present, known := listed(job.ID, c.path)
		if present != c.present || known != c.known {
			t.Errorf("%s: present=%v known=%v, want %v %v", c.path, present, known, c.present, c.known)
		}
	}
	// Another scan knows nothing of this one's listing.
	if _, known := listed("another-job", "Show/fanart.jpg"); known {
		t.Fatal("one scan's listing answered for another")
	}

	// A folder that has changed since it was listed is no longer known, so a
	// sidecar that arrived late is asked for; its unchanged neighbour still is.
	time.Sleep(50 * time.Millisecond)
	if err = os.WriteFile(filepath.Join(root, "Show/Season 1/poster.jpg"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, known := listed(job.ID, "Show/Season 1/poster.jpg"); known {
		t.Fatal("a folder that changed after it was listed was still trusted")
	}
	if _, known := listed(job.ID, "Show/Season 1/episode.nfo"); known {
		t.Fatal("a changed folder's old listing answered for another name")
	}
	if present, known := listed(job.ID, "Bare/logo.png"); present || !known {
		t.Fatal("an unchanged folder was forgotten with the changed one", present, known)
	}

	// When the scan ends, so does what it remembered.
	if err = s.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	if _, known := listed(job.ID, "Bare/logo.png"); known {
		t.Fatal("an ended scan's listing was kept")
	}
}

// A directory whose listing was not seen from its first page to its last is
// never treated as known.
func TestPartialDirectoryListingIsNotTrusted(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"Big", "Resumed"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	s := New(nil, nil, assets.Probe{})
	source := catalog.LibrarySource{ResolvedPath: root}
	page := func(directory string, complete bool, names ...string) storage.InventoryPage {
		v, err := storage.LocalInventoryDirectory(storage.InventoryRequest{Root: root, RelativePath: directory})
		if err != nil {
			t.Fatal(err)
		}
		p := storage.InventoryPage{Complete: complete, DirectoryIdentity: v.ObjectIdentity, DirectoryRevision: storage.InventoryRevision(v)}
		for _, name := range names {
			p.Entries = append(p.Entries, storage.Snapshot{Name: name})
		}
		return p
	}
	listed := func(relative string) (bool, bool) {
		return s.scanListed(context.Background(), "job", source)(filepath.Join(root, relative))
	}
	s.noteDirectoryPage("job", "Big", "", page("Big", false, "a.mkv"))
	if _, known := listed("Big/poster.jpg"); known {
		t.Fatal("a directory still being listed was called known")
	}
	s.noteDirectoryPage("job", "Big", "cursor-1", page("Big", true, "poster.jpg"))
	if present, known := listed("Big/poster.jpg"); !present || !known {
		t.Fatal("a directory listed over two pages lost a name", present, known)
	}
	// A continuation whose first page this process never saw (a restart).
	s.noteDirectoryPage("job", "Resumed", "cursor-9", page("Resumed", true, "b.mkv"))
	if _, known := listed("Resumed/poster.jpg"); known {
		t.Fatal("a listing joined part-way was called complete")
	}
	// Listing a directory again replaces what was known about it.
	s.noteDirectoryPage("job", "Big", "", page("Big", true))
	if present, known := listed("Big/poster.jpg"); present || !known {
		t.Fatal("a fresh listing did not replace the old one", present, known)
	}
	// A listing that does not say what it was a listing of is not kept.
	s.noteDirectoryPage("job", "Big", "", storage.InventoryPage{Complete: true})
	if _, known := listed("Big/poster.jpg"); known {
		t.Fatal("a listing with no directory revision was trusted")
	}
}
