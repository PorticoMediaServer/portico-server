package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/readprep"
)

func TestFileInfoOpeningPreparationHasNoDetailAndAcceptsOnlyReadyEpoch(t *testing.T) {
	f := newFileInfoLaneFixture(t)
	f.c.Exec(`UPDATE catalog_entities SET info_source_blockers=1 WHERE id=?`, f.item.ID)
	out, e := f.r.PrepareOpening(context.Background(), "family", f.item.Public, "detail", "none")
	if e != nil || out.Preparing == nil || out.Preparing.Detail != nil || out.Opening != nil {
		t.Fatal(out, e)
	}
	id := out.Preparing.QueryID
	if _, e = f.r.Poll(context.Background(), "foreign", id); !errors.Is(e, readprep.ErrStale) {
		t.Fatal("foreign preparation admitted", e)
	}
	if e = f.r.Cancel(context.Background(), "foreign", id); e != nil {
		t.Fatal(e)
	}
	out, e = f.r.Poll(context.Background(), "family", id)
	if e != nil || out.Preparing == nil || out.Preparing.Detail != nil {
		t.Fatal(out, e)
	}
	f.c.Exec(`UPDATE catalog_entities SET info_source_blockers=0 WHERE id=?`, f.item.ID)
	deadline := time.Now().Add(8 * time.Second)
	for out.Preparing != nil && time.Now().Before(deadline) {
		out.Release()
		time.Sleep(time.Millisecond)
		out, e = f.r.Poll(context.Background(), "family", id)
		if e != nil {
			t.Fatal(e)
		}
	}
	defer out.Release()
	if out.Opening == nil || out.Opening.Kind != "detail" || out.Opening.Related != "none" || out.Opening.Epoch < 1 {
		t.Fatal(out)
	}
	var opening string
	original := ContentRevision{Catalog: 71, Viewer: 8}
	e = dbwork.WithReadSnapshot(context.Background(), f.c.DB, func(ctx context.Context) error {
		var e error
		opening, e = f.r.AcceptPreparedOpening(ctx, "family", *out.Opening, original)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	proof, _, e := f.r.opening(context.Background(), "family", opening)
	if e != nil || proof.Revision != original {
		t.Fatal(proof, e)
	}
	f.c.Exec(`UPDATE catalog_entities SET info_desired_epoch=info_desired_epoch+1,info_applied_epoch=info_applied_epoch+1 WHERE id=?`, f.item.ID)
	e = dbwork.WithReadSnapshot(context.Background(), f.c.DB, func(ctx context.Context) error {
		_, e := f.r.AcceptPreparedOpening(ctx, "family", *out.Opening, original)
		return e
	})
	if !errors.Is(e, readprep.ErrStale) {
		t.Fatal("ready preparation adopted another epoch", e)
	}
}
