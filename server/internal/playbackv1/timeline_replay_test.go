package playbackv1

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
)

func timelineLease(t *testing.T, db *sql.DB, id string) int64 {
	t.Helper()
	var lease int64
	if err := db.QueryRow(`SELECT lease_expires_ms FROM playback_v1_sessions WHERE id=?`, id).Scan(&lease); err != nil {
		t.Fatal(err)
	}
	return lease
}

func TestTimelineReplaysCoalesceLeaseWritesAndStillRenew(t *testing.T) {
	db := pausedTestDB(t)
	now := time.UnixMilli(1_750_000_000_000)
	s := &Service{DB: db, Now: func() time.Time { return now }, LeaseDuration: 120 * time.Second}
	c := pausedCaller()
	insertPausedSession(t, db, "replayed", "vod", "playing", now.UnixMilli(), 0)
	if _, err := db.Exec(`UPDATE playback_v1_sessions SET lease_expires_ms=?,last_seq=10,position_ms=7000 WHERE id='replayed'`, now.Add(s.lease()).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	initialLease := timelineLease(t, db, "replayed")
	reports := []Report{{Seq: 10, Generation: 1, State: "playing", PositionMs: 1000}, {Seq: 11, Generation: 0, State: "paused", PositionMs: 2000}}
	before := dbwork.ChangingCommits()
	for i := 0; i < 100; i++ {
		if every, err := s.Timeline(context.Background(), c, "replayed", reports[i%2]); err != nil || every != ReportPlaying {
			t.Fatalf("replay cadence %s error %v", every, err)
		}
	}
	if dbwork.ChangingCommits() != before || timelineLease(t, db, "replayed") != initialLease {
		t.Fatal("duplicate/stale reports performed unnecessary writes")
	}
	now = now.Add(6 * time.Second)
	if _, err := s.Timeline(context.Background(), c, "replayed", reports[0]); err != nil {
		t.Fatal(err)
	}
	if got := timelineLease(t, db, "replayed"); got != now.Add(s.lease()).UnixMilli() || got <= initialLease {
		t.Fatal("replay did not renew its lease before expiry")
	}
	before = dbwork.ChangingCommits()
	for i := 0; i < 10; i++ {
		if _, err := s.Timeline(context.Background(), c, "replayed", reports[0]); err != nil {
			t.Fatal(err)
		}
	}
	if dbwork.ChangingCommits() != before {
		t.Fatal("renewed lease was written again by immediate replays")
	}
	var state string
	var seq, position int64
	if err := db.QueryRow(`SELECT state,last_seq,position_ms FROM playback_v1_sessions WHERE id='replayed'`).Scan(&state, &seq, &position); err != nil {
		t.Fatal(err)
	}
	if state != "playing" || seq != 10 || position != 7000 {
		t.Fatal("duplicate or stale reports changed playback evidence")
	}
	if _, err := s.Timeline(context.Background(), c, "replayed", Report{Seq: 11, Generation: 1, State: "paused", PositionMs: 8000}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT state,last_seq,position_ms FROM playback_v1_sessions WHERE id='replayed'`).Scan(&state, &seq, &position); err != nil {
		t.Fatal(err)
	}
	if state != "paused" || seq != 11 || position != 8000 {
		t.Fatal("fresh state and progress report was not applied immediately")
	}
}

func TestReplayRenewalIntervalKeepsShortLeaseHeadroom(t *testing.T) {
	db := pausedTestDB(t)
	now := time.UnixMilli(1_750_000_000_000)
	s := &Service{DB: db, Now: func() time.Time { return now }, LeaseDuration: 200 * time.Millisecond, ReportEvery: time.Second}
	c := pausedCaller()
	insertPausedSession(t, db, "short_replay", "vod", "playing", now.UnixMilli(), 0)
	if _, err := db.Exec(`UPDATE playback_v1_sessions SET lease_expires_ms=?,last_seq=1 WHERE id='short_replay'`, now.Add(s.lease()).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		now = now.Add(60 * time.Millisecond)
		if _, err := s.Timeline(context.Background(), c, "short_replay", Report{Seq: 1, Generation: 1, State: "playing"}); err != nil {
			t.Fatal(err)
		}
		if timelineLease(t, db, "short_replay") != now.Add(s.lease()).UnixMilli() {
			t.Fatal("short lease was not renewed with sufficient headroom")
		}
	}
}
