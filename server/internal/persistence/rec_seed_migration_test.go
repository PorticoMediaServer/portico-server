package persistence

import (
	"context"
	"path/filepath"
	"testing"

	"portico.local/server/internal/dbwork"
)

func TestPositiveRecommendationSeedIndexUpgradesExistingHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := dbwork.OpenHandle(path, dbwork.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	steps, err := schemaMigrations()
	if err != nil {
		t.Fatal(err)
	}
	older := make([]schemaMigration, 0, len(steps))
	for _, step := range steps {
		if step.version <= 10 {
			older = append(older, step)
		}
	}
	if err := migrateWith(context.Background(), db, older); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rec_profile_signals(profile_id,work_id,weight,engaged,hidden,at,long,short) VALUES('p',1,2,1,0,'2026-01-01',2,2),('p',2,-1,0,1,'2026-01-02',-1,-1),('p',3,0,1,0,'2026-01-03',0,0)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	upgraded, err := OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var rows, positive, version int
	if err := upgraded.QueryRow(`SELECT count(*) FROM rec_profile_signals WHERE profile_id='p'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.QueryRow(`SELECT count(*) FROM rec_profile_signals INDEXED BY rec_profile_signals_positive_recent WHERE profile_id='p' AND weight>0`).Scan(&positive); err != nil {
		t.Fatal(err)
	}
	version, err = storedSchemaVersion(context.Background(), upgraded)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 3 || positive != 1 || version != schemaVersion {
		t.Fatalf("upgrade altered history or omitted index: rows=%d positive=%d version=%d", rows, positive, version)
	}
}
