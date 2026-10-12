package ingestion

import (
	"context"
	"os"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/persistence"
	"strings"
	"testing"
)

func TestStoppedScanTellsTheOwnerWhereItStopped(t *testing.T) {
	root := t.TempDir()
	broken := filepath.Join(root, "Films", "Unreadable")
	if err := os.MkdirAll(broken, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(broken, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(broken, 0700) })
	if _, err := os.ReadDir(broken); err == nil {
		t.Skip("this user can read a directory without permission")
	}
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// The first account is the owner, whose inbox receives the notice.
	if _, err = db.Exec(`INSERT INTO accounts VALUES('owner','owner',x'00','owner-profile',1)`); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(db)
	lib, err := cat.Create("Movies", "movie", root)
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, cat, assets.Probe{})
	job, err := s.Queue(lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	s.process(context.Background(), job.ID, lib.ID)
	if j, err := s.Get(job.ID); err != nil || j.Status != "failed" {
		t.Fatalf("scan of an unreadable folder did not stop: %+v %v", j, err)
	}
	var body, arguments string
	if err = db.QueryRow(`SELECT body,arguments FROM notification_records WHERE category='scan.failed'`).Scan(&body, &arguments); err != nil {
		t.Fatal(err)
	}
	where := filepath.Join("Films", "Unreadable")
	if !strings.Contains(body, where) || !strings.Contains(arguments, where) {
		t.Fatalf("notice does not name the folder: %q %s", body, arguments)
	}
	if strings.Contains(body, root) || strings.Contains(arguments, root) {
		t.Fatalf("notice carries the absolute source path: %q %s", body, arguments)
	}
}
