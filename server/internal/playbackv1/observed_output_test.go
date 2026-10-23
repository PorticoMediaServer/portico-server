package playbackv1

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"portico.local/server/internal/playback"
)

func TestTimelineObservedOutputRequiresCurrentDeviceGenerationAndSequence(t *testing.T) {
	db := pausedTestDB(t)
	ctx := context.Background()
	now := time.UnixMilli(1_750_000_000_000)
	s := &Service{DB: db, Now: func() time.Time { return now }}
	caller := pausedCaller()
	insertPausedSession(t, db, "observed", "vod", "playing", now.UnixMilli(), 0)
	read := func(generation int) *playback.ObservedPlayback {
		t.Helper()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		got, err := playback.ReadObservedPlayback(ctx, tx, "observed", generation, generation)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	actual := &playback.ObservedPlayback{Video: &playback.ObservedVideo{Width: 1920, Height: 1080}, Audio: &playback.ObservedAudioOutput{SampleRate: 44100, Channels: 2}}
	if _, err := s.Timeline(ctx, caller, "observed", Report{Seq: 1, Generation: 1, State: "playing", ObservedPlayback: actual}); err != nil {
		t.Fatal(err)
	}
	if got := read(1); !reflect.DeepEqual(got, actual) {
		t.Fatalf("actual sparse output lost: %+v", got)
	}
	wrong := &playback.ObservedPlayback{Screen: &playback.ObservedScreen{DynamicRange: "dolby_vision"}}
	for _, report := range []Report{{Seq: 1, Generation: 1, State: "playing", ObservedPlayback: wrong}, {Seq: 2, Generation: 0, State: "playing", ObservedPlayback: wrong}} {
		if _, err := s.Timeline(ctx, caller, "observed", report); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(read(1), actual) {
			t.Fatal("replay/stale generation replaced actual output")
		}
	}
	controller := caller
	controller.DeviceID = "other-device"
	if _, err := s.Timeline(ctx, controller, "observed", Report{Seq: 2, Generation: 1, State: "playing", ObservedPlayback: wrong}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("controller supplied output: %v", err)
	}
	if _, err := s.Timeline(ctx, caller, "observed", Report{Seq: 2, Generation: 1, State: "playing", ObservedPlayback: &playback.ObservedPlayback{}}); err != nil {
		t.Fatal(err)
	}
	if got := read(1); !reflect.DeepEqual(got, &playback.ObservedPlayback{}) {
		t.Fatalf("new snapshot inherited old facts: %+v", got)
	}
	// This unit fixture models the existing presentation replacement. The
	// observation must disappear before any new-generation report arrives.
	if _, err := db.Exec(`UPDATE playback_v1_sessions SET generation=2,last_seq=-1 WHERE id='observed'`); err != nil {
		t.Fatal(err)
	}
	if got := read(2); got != nil {
		t.Fatalf("new presentation inherited old output: %+v", got)
	}
	if got := read(1); got != nil {
		t.Fatalf("obsolete generation remained readable: %+v", got)
	}
	if _, err := s.Timeline(ctx, caller, "observed", Report{Seq: 1, Generation: 2, State: "playing", ObservedPlayback: actual}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(read(2), actual) {
		t.Fatal("current-generation report was not stored")
	}
	if _, err := s.Timeline(ctx, caller, "observed", Report{Seq: 2, Generation: 2, State: "ended"}); err != nil {
		t.Fatal(err)
	}
	if got := read(2); got != nil {
		t.Fatalf("ended session published output: %+v", got)
	}
}

func TestObservedOutputStoreCannotBypassAcceptedTimelineBinding(t *testing.T) {
	db := pausedTestDB(t)
	ctx := context.Background()
	now := time.UnixMilli(1_750_000_000_000)
	insertPausedSession(t, db, "binding", "vod", "playing", now.UnixMilli(), 0)
	if _, err := db.Exec(`UPDATE playback_v1_sessions SET last_seq=5 WHERE id='binding'`); err != nil {
		t.Fatal(err)
	}
	actual := &playback.ObservedPlayback{Audio: &playback.ObservedAudioOutput{SampleRate: 48000}}
	for _, test := range []struct {
		name, device                              string
		sessionGeneration, presentationGeneration int
		seq                                       int64
	}{
		{"not-accepted", "dev1", 1, 1, 6}, {"stale-sequence", "dev1", 1, 1, 4}, {"other-device", "dev2", 1, 1, 5}, {"stale-session", "dev1", 2, 2, 5}, {"unproved-presentation", "dev1", 1, 9, 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if err = playback.StoreObservedPlaybackTx(ctx, tx, "binding", test.device, test.sessionGeneration, test.presentationGeneration, test.seq, now.UnixMilli(), actual); err != nil {
				t.Fatal(err)
			}
			var count int
			if err = tx.QueryRow(`SELECT COUNT(*) FROM playback_observed_output WHERE session_id='binding'`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("unaccepted binding wrote observation: %d %v", count, err)
			}
		})
	}
}
