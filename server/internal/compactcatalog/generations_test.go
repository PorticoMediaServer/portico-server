package compactcatalog

import (
	"context"
	"database/sql"
	"testing"
)

func TestBrowseGenerationRetainsRowsUntilSwitch(t *testing.T) {
	db, show := failingFixture(t, "generations.sqlite")
	w := NewWorker(db)
	if err := w.Rebuild(context.Background(), DomainBrowseRows); err != nil {
		t.Fatal(err)
	}
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE catalog_browse_rows SET title='New title' WHERE entity_id=?`, show)
		return err
	})
	var title string
	if err := db.QueryRow(`SELECT title FROM catalog_browse_rows_serving WHERE entity_id=?`, show).Scan(&title); err != nil || title != "Show" {
		t.Fatalf("old generation: %q %v", title, err)
	}
	if CheckReadiness(context.Background(), db, DomainBrowseRows) != nil {
		t.Fatal("browse unavailable during rebuild")
	}
	cc1Drain(t, db)
	if n := cc1Scalar(t, db, `SELECT retaining FROM catalog_generations`); n != 0 {
		t.Fatalf("generation never switched: %d", n)
	}
}

func TestSearchGenerationsSwitchAndRebuildAgain(t *testing.T) {
	db, show := failingFixture(t, "search-generation.sqlite")
	w := NewWorker(db)
	if err := w.Rebuild(context.Background(), DomainSearch); err != nil {
		t.Fatal(err)
	}
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE catalog_entities SET title='Different' WHERE id=?`, show)
		return err
	})
	for i := 0; i < 100; i++ {
		n, err := w.Step(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	if slot := cc1Scalar(t, db, `SELECT active_slot FROM catalog_derivations WHERE domain=17`); slot != 1 {
		t.Fatalf("slot %d", slot)
	}
	if n := cc1Scalar(t, db, `SELECT count(*) FROM catalog_search_titles_next WHERE catalog_search_titles_next MATCH 'Different'`); n != 1 {
		t.Fatalf("next index has %d matches", n)
	}
	if err := w.Rebuild(context.Background(), DomainSearch); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		n, err := w.Step(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	if slot := cc1Scalar(t, db, `SELECT active_slot FROM catalog_derivations WHERE domain=17`); slot != 0 {
		t.Fatalf("second slot %d", slot)
	}
	if n := cc1Scalar(t, db, `SELECT count(*) FROM catalog_search_titles WHERE catalog_search_titles MATCH 'Different'`); n != 1 {
		t.Fatalf("rebuilt index has %d matches", n)
	}
}

func TestRetentionKeepsReferencedTerms(t *testing.T) {
	db, _ := failingFixture(t, "retention.sqlite")
	cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO catalog_terms(vocab,key,label) VALUES(1,'unused','Unused')`)
		return err
	})
	for i := 0; i < 50; i++ {
		more := false
		cc1Write(t, db, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			more, err = PruneTx(ctx, tx, 4)
			return err
		})
		if !more {
			break
		}
	}
	if n := cc1Scalar(t, db, `SELECT count(*) FROM catalog_terms WHERE key='unused'`); n != 0 {
		t.Fatal("unused vocabulary retained")
	}
}
