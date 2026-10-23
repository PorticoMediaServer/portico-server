package persistence

import (
	"context"
	"testing"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/worker"
)

func TestPlannerStatisticsDoNotWakeCatalogWork(t *testing.T) {
	db := openPlannerStatsDB(t)
	if _, err := db.Exec(`CREATE TABLE books(id INTEGER PRIMARY KEY, title TEXT); INSERT INTO books VALUES(1,'Book'); CREATE TABLE inventory_jobs(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := dbwork.InstallWakeDependencies(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	catalog, all := worker.NewSignal(), worker.NewSignal()
	stopCatalog := dbwork.WakeOnTables(catalog, "books", "inventory_jobs")
	defer stopCatalog()
	stopAll := dbwork.WakeOnCommit(all)
	defer stopAll()
	before := dbwork.ChangingCommits()
	if err := RefreshPlannerStatistics(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if dbwork.ChangingCommits() == before || !all.Take() {
		t.Fatal("statistics fixture did not publish a real changing commit")
	}
	if catalog.Take() {
		t.Fatal("planner-only changes woke catalog or inventory workers")
	}
}
