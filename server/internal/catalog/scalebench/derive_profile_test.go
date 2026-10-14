package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

// Only the explicitly named disposable copy is mutated. Never profile a
// concurrently running measurement's database with a second worker.
func TestMeasureDerivationStatements(t *testing.T) {
	path := os.Getenv("PORTICO_SCALE_PROFILE_DB")
	if path == "" {
		t.Skip("set an isolated profile copy")
	}
	if !strings.Contains(path, "/_program/scratch/scale-check-2/.scratch/profile-") {
		t.Fatal("refusing non-profile database")
	}
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	w := compactcatalog.NewWorker(db)
	dbwork.ProfileStatements(true)
	defer dbwork.ProfileStatements(false)
	start := time.Now()
	for i := 0; i < 200; i++ {
		if _, err = w.Step(context.Background(), compactcatalog.BatchLimit); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("200 steps in %s gate=%+v", time.Since(start), dbwork.WriteGate().Stats())
	for i, entry := range dbwork.StatementProfile() {
		if i == 20 {
			break
		}
		t.Logf("%.3f ms %d calls %s", entry.Millis(), entry.Count, entry.Shape)
	}
}
