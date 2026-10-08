package dvr

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/persistence"
)

// FEAT-08 (server data): DVR rows carry a stable show-grouping key. Two
// recordings of one series and one of another come back with the right
// seriesId values; one without series metadata has none. Paging order is
// unchanged: the client groups what it has.
func TestRecordingsCarrySeriesGrouping(t *testing.T) {
	ctx := context.Background()
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	live, err := livechannels.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{db: db, live: live, now: time.Now, captureAvailable: true, durable: func(context.Context, *sql.Tx, livechannels.Owner, string, string) error {
		return nil
	}}
	owner := livechannels.Owner{Authority: "local", AccountID: "account", ProfileID: "profile"}
	auth := func(context.Context, *sql.Tx, bool) (string, func(string, string) bool, error) {
		return "fence", func(string, string) bool { return true }, nil
	}
	src := strings.Repeat("ab", 24)
	if _, err = db.Exec(`INSERT INTO live_source_identities VALUES(?)`, src); err != nil {
		t.Fatal(err)
	}
	ch := strings.Repeat("cd", 32)
	gen := strings.Repeat("ef", 24)
	now := time.Now().UnixMilli()
	insert := func(id, series, title string, created int64) {
		t.Helper()
		prog := livechannels.Programme{ID: id, ChannelID: ch, Title: title, Start: time.UnixMilli(now).UTC().Format(time.RFC3339Nano), End: time.UnixMilli(now + 3600000).UTC().Format(time.RFC3339Nano), Lineage: "interval-only", SeriesID: series}
		pb, _ := json.Marshal(prog)
		ob, _ := json.Marshal(Options{})
		if _, err = db.Exec(`INSERT INTO dvr_recordings(id,owner_key,authority,account_id,profile_id,source_id,channel_id,programme_id,guide_generation,programme_json,rule_id,rule_revision,manual,options_json,start_ms,end_ms,priority,revision,state,created_ms,updated_ms) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,'completed',?,?)`,
			id, owner.Key(), owner.Authority, owner.AccountID, owner.ProfileID, src, ch, id, gen, string(pb), "", 0, 0, string(ob), now, now+3600000, 0, created, created); err != nil {
			t.Fatal(err)
		}
	}
	// Two of one series, one of another, one with no series metadata. Created
	// order is deliberately not grouped: the list must keep paging order.
	idA1, idA2 := strings.Repeat("a1", 32), strings.Repeat("a2", 32)
	idB, idNone := strings.Repeat("b1", 32), strings.Repeat("c1", 32)
	insert(idB, "show-b", "Show B", now+30)
	insert(idNone, "", "One-off special", now+20)
	insert(idA2, "show-a", "Show A", now+10)
	insert(idA1, "show-a", "Show A", now)
	page, err := s.List(ctx, auth, owner, Query{State: "all", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Recordings) != 4 {
		t.Fatalf("recordings = %d, want 4", len(page.Recordings))
	}
	// Paging order unchanged: created_ms DESC, not grouped.
	wantOrder := []string{idB, idNone, idA2, idA1}
	for i, want := range wantOrder {
		if page.Recordings[i].ID != want {
			t.Fatalf("order[%d] = %s, want %s", i, page.Recordings[i].ID, want)
		}
	}
	byID := map[string]Recording{}
	for _, r := range page.Recordings {
		byID[r.ID] = r
	}
	if got := byID[idA1].SeriesID; got != "show-a" {
		t.Fatalf("A1 seriesId = %q", got)
	}
	if got := byID[idA2].SeriesID; got != "show-a" {
		t.Fatalf("A2 seriesId = %q", got)
	}
	if got := byID[idB].SeriesID; got != "show-b" {
		t.Fatalf("B seriesId = %q", got)
	}
	if got := byID[idNone].SeriesID; got != "" {
		t.Fatalf("unmetadata seriesId = %q, want none", got)
	}
	if got := byID[idA1].SeriesTitle; got != "Show A" {
		t.Fatalf("A1 seriesTitle = %q", got)
	}
	if got := byID[idB].SeriesTitle; got != "Show B" {
		t.Fatalf("B seriesTitle = %q", got)
	}
	if got := byID[idNone].SeriesTitle; got != "" {
		t.Fatalf("unmetadata seriesTitle = %q, want none", got)
	}
	// Detail carries the same grouping.
	detail, err := s.Detail(ctx, auth, owner, idA1, "")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Recording.SeriesID != "show-a" || detail.Recording.SeriesTitle != "Show A" {
		t.Fatalf("detail grouping = %q %q", detail.Recording.SeriesID, detail.Recording.SeriesTitle)
	}
}
