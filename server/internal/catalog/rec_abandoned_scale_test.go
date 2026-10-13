package catalog

import (
	"context"
	"portico.local/server/internal/dbwork"
	"testing"
	"time"
)

func TestAbandonedTasteLoadsTheWindowInTwoStatements(t *testing.T) {
	w := newRecWorld(t)
	old := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339Nano)
	for _, film := range w.others {
		w.c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,playback_id) VALUES('p',?,60000,0,'pb')`, film.ID)
		w.c.Exec(`INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) VALUES('p','m',?,?,'paused')`, film.ID, old)
	}
	w.c.Drain()
	ctx, cost := dbwork.Measure(context.Background())
	values := map[string]float64{}
	if err := w.s.WithContext(ctx).recAbandoned("p", time.Now(), values); err != nil {
		t.Fatal(err)
	}
	if len(values) == 0 {
		t.Fatal("missing abandonment contributions")
	}
	if cost().Statements != 2 {
		t.Fatalf("window needed %d statements", cost().Statements)
	}
}
