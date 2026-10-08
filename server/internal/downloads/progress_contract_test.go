package downloads

import (
	"context"
	"encoding/json"
	"portico.local/server/internal/apispec"
	"portico.local/server/internal/identity"
	"testing"
	"time"

	"portico.local/server/internal/compactcatalog"
)

func TestOfflineProgressClampsToKnownDuration(t *testing.T) {
	h := newHarness(t)
	entries := []ProgressEntry{{ItemID: h.item, PositionSeconds: 600, ObservedAt: h.now.Format(time.RFC3339)}}
	out, err := h.service.SyncProgress(context.Background(), h.viewer, "bounded-progress", entries)
	if err != nil {
		t.Fatal(err)
	}
	if out.Applied != 1 || out.Entries[0].PositionSeconds != 60 {
		t.Fatalf("%+v", out)
	}
	var stored int64
	var unit int
	if err = h.db.QueryRow(`SELECT position,unit FROM progress WHERE profile_id=? AND item_id=?`, identity.PersonalKey(h.viewer.Viewer), h.itemID).Scan(&stored, &unit); err != nil || stored != 60_000 || unit != 0 {
		t.Fatalf("stored %v %v", stored, err)
	}
	replay, err := h.service.SyncProgress(context.Background(), h.viewer, "bounded-progress", entries)
	if err != nil || replay.Entries[0].PositionSeconds != 60 {
		t.Fatalf("replay %+v %v", replay, err)
	}
	doc, schema, err := apispec.Schema("ProgressResult")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out.Entries[0])
	if issues := doc.ValidateJSON(schema, raw); len(issues) > 0 {
		t.Fatal(issues)
	}
	updateCompactFixtureAssetByToken(t, h.db, h.asset, func(a *compactcatalog.Asset) { a.Duration = 0 })
	settleDownloadsProjection(t, h.db)
	entries[0].ObservedAt = h.now.Add(time.Second).Format(time.RFC3339)
	out, err = h.service.SyncProgress(context.Background(), h.viewer, "unknown-duration", entries)
	if err != nil || out.Entries[0].PositionSeconds != 600 {
		t.Fatalf("unknown %+v %v", out, err)
	}
}
