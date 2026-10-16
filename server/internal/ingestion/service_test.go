package ingestion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/storage"
	"testing"
)

func TestDurableContinuationAndCancellation(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "media")
	if e := os.Mkdir(media, 0700); e != nil {
		t.Fatal(e)
	}
	// Folders are listed and checked a batch to a step: this many take more
	// than the 32 steps of one process call and fewer than two calls' worth.
	for i := 0; i < 20*storage.MaxInventoryBatch; i++ {
		if e := os.Mkdir(filepath.Join(media, fmt.Sprint(i)), 0700); e != nil {
			t.Fatal(e)
		}
	}
	path := filepath.Join(root, "db")
	db, e := persistence.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	cat := catalog.New(db)
	lib, e := cat.Create("Movies", "movie", media)
	if e != nil {
		t.Fatal(e)
	}
	s := New(db, cat, assets.Probe{})
	job, e := s.Queue(lib.ID)
	if e != nil {
		t.Fatal(e)
	}
	s.process(context.Background(), job.ID, lib.ID)
	j, e := s.Get(job.ID)
	if e != nil || j.Status != "running" {
		t.Fatalf("unfinished work reported complete: %+v %v", j, e)
	}
	db.Close()
	db, e = persistence.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s = New(db, catalog.New(db), assets.Probe{})
	s.process(context.Background(), job.ID, lib.ID)
	j, e = s.Get(job.ID)
	if e != nil || j.Status != "complete" {
		t.Fatalf("restart continuation %+v %v", j, e)
	}
	job, e = s.Queue(lib.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Cancel(job.ID); e != nil {
		t.Fatal(e)
	}
	s.process(context.Background(), job.ID, lib.ID)
	j, e = s.Get(job.ID)
	if e != nil || j.Status != "cancelled" {
		t.Fatalf("cancelled job restarted %+v %v", j, e)
	}
}
