package metadata

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

// Seeding runs again whenever a title's artwork is marked dirty. A slot that
// already holds this provider's image under the current state, or the owner's
// local image, is taken: it is neither downloaded again nor replaced.
func TestProviderArtworkIsNotRequeuedForAnOccupiedSlot(t *testing.T) {
	s, db, target, _ := integrationScreen(t)
	ctx := context.Background()
	requests := 0
	s.artClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(p08cPNG(t, 20, 20))), Header: http.Header{}}, nil
	})}
	if err := s.ScreenStep(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.seedArtwork(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	entity := resolveArtworkTestEntity(t, db, target)
	var selected string
	if err := db.QueryRow(`SELECT candidate_id FROM artwork_selections WHERE kind='item' AND entity_id=? AND role='poster'`, entity).Scan(&selected); err != nil || requests != 1 {
		t.Fatal("fixture poster was not downloaded once", err, requests)
	}
	pending := func() int {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM artwork_jobs WHERE kind='item' AND entity_id=? AND role='poster' AND status IN('pending','retry','running')`, entity).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	reseed := func() {
		integrationExec(t, db, `INSERT OR IGNORE INTO artwork_dirty(kind,entity_id) VALUES('item',?)`, entity)
		if err := s.seedArtwork(ctx); err != nil {
			t.Fatal(err)
		}
	}
	reseed()
	if n := pending(); n != 0 {
		t.Fatalf("a poster already held under the current state was queued again (%d jobs)", n)
	}
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("held poster downloaded again: %d requests", requests)
	}

	// The owner's local image now holds the slot, selected under an earlier state.
	integrationExec(t, db, `INSERT INTO artwork_candidates(id,kind,entity_id,role,subject,provider,image_id,origin,attribution,source_fence,observed_at) SELECT 'local-poster',kind,entity_id,role,subject,'local','sidecar','local:sidecar','Local file','earlier-state',observed_at FROM artwork_candidates WHERE id=?`, selected)
	integrationExec(t, db, `UPDATE artwork_selections SET candidate_id='local-poster' WHERE kind='item' AND entity_id=? AND role='poster'`, entity)
	integrationExec(t, db, `UPDATE artwork_candidates SET source_fence='earlier-state' WHERE id=?`, selected)
	reseed()
	if n := pending(); n != 0 {
		t.Fatalf("a provider poster was queued over the local selection (%d jobs)", n)
	}
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	var now string
	if err := db.QueryRow(`SELECT candidate_id FROM artwork_selections WHERE kind='item' AND entity_id=? AND role='poster'`, entity).Scan(&now); err != nil || now != "local-poster" || requests != 1 {
		t.Fatalf("local selection replaced: %q after %d requests (%v)", now, requests, err)
	}
}
