package playbackv1

import (
	"context"
	"testing"
	"time"
)

func TestChannelTimelineOldBufferRenewsOnlyLease(t *testing.T) {
	for _, reportedState := range []string{"paused", "ended", "playing"} {
		t.Run(reportedState, func(t *testing.T) {
			db := pausedTestDB(t)
			ctx := context.Background()
			now := time.UnixMilli(1_750_000_000_000)
			insertPausedSession(t, db, "buffer-fence", "channel", "playing", now.UnixMilli(), 0)
			if _, err := db.Exec(`UPDATE playback_v1_sessions SET position_ms=5000,last_seq=4,lease_expires_ms=? WHERE id='buffer-fence'`, now.Add(time.Minute).UnixMilli()); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO playback_channel_sessions(session_id,authority,account_id,profile_id,server_id,role,account_epoch,family_id,kind,source_id,channel_id,selection_json,transport_json,buffer_ordinal,status,stage,created_ms) VALUES('buffer-fence','local','owner','primary','unit','local',1,'unit','live-source','unit-source','unit-channel','{}','{}',8,'active','engine',?)`, now.UnixMilli()); err != nil {
				t.Fatal(err)
			}
			authority := &observedChannelAuthority{generation: "8"}
			s := &Service{DB: db, Channels: authority, Now: func() time.Time { return now }}
			before, err := loadRow(ctx, db, "buffer-fence")
			if err != nil {
				t.Fatal(err)
			}
			var eventsBefore int
			if err = db.QueryRow(`SELECT count(*) FROM api_events`).Scan(&eventsBefore); err != nil {
				t.Fatal(err)
			}
			every, err := s.Timeline(ctx, pausedCaller(), before.id, Report{Seq: 100, Generation: 7, State: reportedState, PositionMs: 99000})
			if err != nil || every != ReportPlaying {
				t.Fatalf("old buffer report: cadence=%s error=%v", every, err)
			}
			after, err := loadRow(ctx, db, before.id)
			if err != nil {
				t.Fatal(err)
			}
			if after.lease != now.Add(s.lease()).UnixMilli() || after.lease <= before.lease {
				t.Fatal("old buffer report did not renew the lease")
			}
			after.lease = before.lease
			if after != before || authority.observations != 0 {
				t.Fatalf("old buffer changed durable playback: before=%+v after=%+v observations=%d", before, after, authority.observations)
			}
			var eventsAfter int
			if err = db.QueryRow(`SELECT count(*) FROM api_events`).Scan(&eventsAfter); err != nil {
				t.Fatal(err)
			}
			if eventsAfter != eventsBefore {
				t.Fatal("old buffer emitted a session change")
			}
			var history int
			if err = db.QueryRow(`SELECT count(*) FROM play_history WHERE playback_id=?`, before.id).Scan(&history); err != nil || history != 0 {
				t.Fatalf("old buffer recorded watch history: count=%d error=%v", history, err)
			}

			// The rejected high sequence did not consume the new buffer's next
			// report. Its lower, fresh sequence still changes state and position.
			if _, err = s.Timeline(ctx, pausedCaller(), before.id, Report{Seq: 5, Generation: 8, State: "paused", PositionMs: 6000}); err != nil {
				t.Fatal(err)
			}
			current, err := loadRow(ctx, db, before.id)
			if err != nil || current.state != "paused" || current.position != 6000 || current.lastSeq != 5 || current.revision != before.revision+1 || authority.observations != 1 {
				t.Fatalf("fresh buffer report was not accepted: %+v observations=%d error=%v", current, authority.observations, err)
			}

			// A runtime callback can lag the durable buffer's replacement. It
			// cannot authorize state writes for that retired buffer either.
			if _, err = db.Exec(`UPDATE playback_channel_sessions SET buffer_ordinal=9 WHERE session_id='buffer-fence'`); err != nil {
				t.Fatal(err)
			}
			if every, err = s.Timeline(ctx, pausedCaller(), before.id, Report{Seq: 200, Generation: 8, State: "playing", PositionMs: 99000}); err != nil || every != ReportPaused {
				t.Fatalf("lagging callback report: cadence=%s error=%v", every, err)
			}
			lagged, err := loadRow(ctx, db, before.id)
			if err != nil {
				t.Fatal(err)
			}
			lagged.lease = current.lease
			if lagged != current || authority.observations != 1 {
				t.Fatalf("lagging callback bypassed durable buffer: before=%+v after=%+v", current, lagged)
			}
		})
	}
}
