package catalog

import (
	"context"
	"fmt"
	"portico.local/server/internal/identity"
	"testing"
	"time"
)

func TestDashboardPeopleAccountPagingAndHistorySharedFilters(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	s.Clock = func() time.Time { return now }
	item := homeNames(t, db)["m1"].ID
	for n := 0; n < 137; n++ {
		id := fmt.Sprintf("a%03d", n)
		_, e := db.Exec(`INSERT INTO play_history(playback_id,authority,account_id,profile_id,user_name,profile_name,item_id,library_id,kind,title,started_ms,updated_ms,watched_ms) VALUES(?,'local',?,'profile',?,'Kids',?,'movies',1,'Movie',?,?,?)`, id, id, id, item, now.Add(-time.Hour).UnixMilli(), now.UnixMilli(), 1000*(n+1))
		if e != nil {
			t.Fatal(e)
		}
	}
	seen := map[string]bool{}
	q := PlayHistoryQuery{Period: "7d", Limit: 12}
	for {
		page, e := s.DashboardPeople(context.Background(), q)
		if e != nil {
			t.Fatal(e)
		}
		if len(page.Items) > 12 {
			t.Fatal("unbounded page")
		}
		for _, p := range page.Items {
			if seen[p.AccountID] {
				t.Fatal("duplicate", p.AccountID)
			}
			seen[p.AccountID] = true
			if p.Plays != 1 || len(p.Profiles) != 1 || len(p.Kinds) != 1 || p.Kinds[0].Kind != "movies" {
				t.Fatal(p)
			}
		}
		if page.NextCursor == "" {
			break
		}
		q.Cursor = page.NextCursor
	}
	if len(seen) != 137 {
		t.Fatal(len(seen))
	}
	q = PlayHistoryQuery{Period: "7d", AccountID: "a002", Media: "movies", Search: "Movie", Limit: 100}
	page, e := s.PlayHistory(context.Background(), q)
	if e != nil || len(page.Entries) != 1 {
		t.Fatal(page, e)
	}
	people, e := s.DashboardPeople(context.Background(), q)
	if e != nil || len(people.Items) != 1 || people.Items[0].Plays != *page.Total {
		t.Fatal(people, e)
	}
	for _, sort := range []string{"title", "person", "player", "when", "watched", "location"} {
		total := 0
		cursor := ""
		for {
			p, e := s.PlayHistory(context.Background(), PlayHistoryQuery{Period: "7d", Limit: 17, Sort: sort, Direction: "asc", Cursor: cursor})
			if e != nil {
				t.Fatal(sort, e)
			}
			total += len(p.Entries)
			if p.NextCursor == "" {
				break
			}
			cursor = p.NextCursor
		}
		if total != 137 {
			t.Fatal(sort, total)
		}
	}
}

func TestDashboardBucketsUseMondayWeeksCalendarMonthsAndObservedTime(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	s.Clock = func() time.Time { return now }
	item := homeNames(t, db)["m1"].ID
	result, e := db.Exec(`INSERT INTO play_history(playback_id,authority,account_id,profile_id,user_name,item_id,library_id,kind,title,started_ms,updated_ms,watched_ms) VALUES('actual','local','a','p','Person',?,'movies',1,'Movie',?,?,60000)`, item, now.Add(-time.Hour).UnixMilli(), now.UnixMilli())
	if e != nil {
		t.Fatal(e)
	}
	id, _ := result.LastInsertId()
	if _, e = db.Exec(`INSERT INTO play_history_time VALUES(?,?,60000)`, id, now.Add(-time.Hour).UnixMilli()); e != nil {
		t.Fatal(e)
	}
	for _, period := range []string{"7d", "30d", "90d", "1y", "all"} {
		out, e := s.DashboardHistory(context.Background(), PlayHistoryQuery{Period: period}, "movies")
		if e != nil {
			t.Fatal(period, e)
		}
		var total int64
		for n, b := range out.Buckets {
			start, e := time.Parse(time.RFC3339Nano, b.Start)
			if e != nil {
				t.Fatal(e)
			}
			if n > 0 && (period == "30d" || period == "90d") && start.Weekday() != time.Monday {
				t.Fatal("week not Monday", b)
			}
			if n > 0 && period == "1y" && start.Day() != 1 {
				t.Fatal("month not calendar", b)
			}
			for _, k := range b.Kinds {
				total += k.WatchedMs
			}
		}
		if total != 60000 || len(out.Top) != 1 || out.Top[0].Plays != 1 || out.Top[0].People != 1 || out.Top[0].CountUnit != "movie_plays" {
			t.Fatal(period, out)
		}
		if !out.Buckets[len(out.Buckets)-1].Partial {
			t.Fatal("current bucket not partial")
		}
	}
	// Retention/Start over deletes both analytics and time facts.
	if _, e = db.Exec(`DELETE FROM play_history WHERE account_id='a'`); e != nil {
		t.Fatal(e)
	}
	var remaining int
	if e = db.QueryRow(`SELECT count(*) FROM play_history_time`).Scan(&remaining); e != nil || remaining != 0 {
		t.Fatal(remaining, e)
	}
}

func TestObservedPlayedTimeExcludesPauseSeekAndMissingReports(t *testing.T) {
	db := homeFixture(t)
	item := homeNames(t, db)["m1"].ID
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	viewer := identity.Viewer{Authority: "local", AccountID: "fixture", ProfileID: "fixture-profile"}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	// Use existing actual history insert producer, then reports at a fake clock.
	for n, report := range []struct {
		after time.Duration
		pos   int64
		state string
	}{
		{0, 30000, "playing"}, {10 * time.Second, 40000, "paused"}, {30 * time.Second, 40000, "playing"},
		{40 * time.Second, 50000, "playing"}, {50 * time.Second, 800000, "playing"}, {3 * time.Minute, 810000, "playing"},
	} {
		// Fixture account is intentionally absent: insertPlay retains only validated
		// canonical names. Add a row directly for this timing test's first report.
		if n == 0 {
			_, e = tx.Exec(`INSERT INTO play_history(playback_id,authority,account_id,profile_id,user_name,item_id,library_id,kind,title,started_ms,updated_ms,last_position_ms,last_state) VALUES('timing','local','fixture','fixture-profile','fixture',?,'movies',1,'Movie',?,?,30000,'playing')`, item, now.UnixMilli(), now.UnixMilli())
		} else {
			_, e = recordPlay(tx, viewer, "timing", item, 1, now.Add(report.after), report.pos, 1000, true, false, false, report.state)
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	var watched int64
	if e = tx.QueryRow(`SELECT watched_ms FROM play_history WHERE playback_id='timing'`).Scan(&watched); e != nil || watched != 20000 {
		t.Fatal(watched, e)
	}
}

func TestHistoryFiltersUseIndexedPersonRangeAndRejectSQLControls(t *testing.T) {
	db := homeFixture(t)
	s := New(db)
	if _, e := s.PlayHistory(context.Background(), PlayHistoryQuery{Period: "all", Sort: "title;DROP TABLE accounts", Limit: 10}); e == nil {
		t.Fatal("sort injection accepted")
	}
	rows, e := db.Query(`EXPLAIN QUERY PLAN SELECT h.id FROM play_history h WHERE h.account_id=? AND h.started_ms>=? ORDER BY h.started_ms DESC,h.id DESC LIMIT 101`, "a", 0)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if e = rows.Scan(&id, &parent, &unused, &detail); e != nil {
			t.Fatal(e)
		}
		if detail == "SEARCH h USING COVERING INDEX play_history_account (account_id=? AND started_ms>?)" {
			found = true
		}
	}
	if !found {
		t.Fatal("person/range index not used")
	}
}
