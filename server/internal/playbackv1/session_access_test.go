package playbackv1

import (
	"context"
	"testing"
	"time"
)

func TestDeviceDoesNotConferAnotherProfilesSession(t *testing.T) {
	db := pausedTestDB(t)
	now := time.Now()
	s := &Service{DB: db, Now: func() time.Time { return now }}
	c := pausedCaller()
	insertPausedSession(t, db, "other-profile", "vod", "playing", now.UnixMilli(), 0)
	c.Principal.ProfileID = "child"
	if _, err := s.Get(context.Background(), c, "other-profile"); err == nil {
		t.Fatal("shared device exposed the outgoing profile's session")
	}
}

func TestRefusedMutationsKeepPreparedPlayback(t *testing.T) {
	for _, op := range []string{"unauthorized patch", "unauthorized stop", "stale patch", "invalid patch"} {
		t.Run(op, func(t *testing.T) {
			db := pausedTestDB(t)
			now := time.Now()
			s := &Service{DB: db, Now: func() time.Time { return now }}
			insertPausedSession(t, db, "ps_owner", "vod", "playing", now.UnixMilli(), 0)
			if _, err := db.Exec(`INSERT INTO queues_v1(id,owner_id,account_id,profile_id,authority,owner_kind,revision,created_ms,updated_ms) VALUES('q','dev1','owner','primary','local','device',1,1,1)`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO playback_v1_prepared(token,queue_id,device_id,create_key,digest,session_id,session_generation,session_state,queue_revision,next_position,entry_id,item_id,media_session_id,request,presentation,expires_ms,state,created_ms) VALUES('prep','q','dev1','key','digest','ps_owner',1,'playing',1,1,'entry',1,'','{}','{}',?,'prepared',1)`, now.Add(time.Minute).UnixMilli()); err != nil {
				t.Fatal(err)
			}
			c := pausedCaller()
			if op == "unauthorized patch" || op == "unauthorized stop" {
				c.DeviceID = "other"
				c.Principal.AccountID = "other"
			}
			var err error
			switch op {
			case "unauthorized stop":
				err = s.Stop(context.Background(), c, "ps_owner", nil)
			case "stale patch":
				_, err = s.Patch(context.Background(), c, "ps_owner", "0", Change{State: "paused"})
			case "invalid patch":
				_, err = s.Patch(context.Background(), c, "ps_owner", "1", Change{State: "invalid"})
			default:
				_, err = s.Patch(context.Background(), c, "ps_owner", "1", Change{State: "paused"})
			}
			if err == nil {
				t.Fatal("mutation should fail")
			}
			var state string
			if err := db.QueryRow(`SELECT state FROM playback_v1_prepared WHERE token='prep'`).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if state != "prepared" {
				t.Fatalf("refused mutation canceled preparation: %s", state)
			}
		})
	}
}
