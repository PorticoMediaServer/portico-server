package lyrics

import (
	"context"
	"errors"
	"testing"
)

func TestLibraryFilesOnlyRefusesBulkAndRevocationPreventsPublication(t *testing.T) {
	c, b, names := bulkFixture(t)
	ctx := context.Background()
	c.Exec(`UPDATE library_preferences SET lyrics_source='files' WHERE library_id='library'`)
	tx, err := c.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.QueueTx(ctx, tx, "files-only", "library")
	tx.Rollback()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal("default files-only admitted network job", err)
	}
	c.Exec(`UPDATE library_preferences SET lyrics_source='lrclib' WHERE library_id='library'`)
	tx, err = c.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.QueueTx(ctx, tx, "revoke-in-flight", "library"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var duration float64
	if err = c.DB.QueryRow(`SELECT duration FROM catalog_assets WHERE token=?`, names["song-a"].Token).Scan(&duration); err != nil {
		t.Fatal(err)
	}
	requests := 0
	b.provider = func(context.Context, string) ([]acquired, error) {
		requests++
		c.Exec(`UPDATE library_preferences SET lyrics_source='files' WHERE library_id='library'`)
		d, _ := Parse([]byte("[00:01]Provider words"), "lrc")
		return []acquired{{doc: d, title: "Alpha", artist: "Fixture Artist", duration: duration}}, nil
	}
	if _, err = b.Step(ctx); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatal("revocation test made no acquisition", requests)
	}
	var count int
	if err = c.DB.QueryRow(`SELECT count(*) FROM lyric_resources WHERE item_id=? AND scope='library'`, names["song-a"].ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("revoked result published", count, err)
	}
	// Uncached local lyrics take priority before any provider request.
	c.Exec(`UPDATE library_preferences SET lyrics_source='lrclib' WHERE library_id='library'`)
	b.Service.Local = localReaderStub{}
	b.provider = func(context.Context, string) ([]acquired, error) {
		t.Fatal("local lyrics lost priority")
		return nil, nil
	}
	outcome, err := b.fetchOne(ctx, "revoke-in-flight", names["song-b"].Public, 1)
	if err != nil || outcome != "skipped" {
		t.Fatal(outcome, err)
	}
}
