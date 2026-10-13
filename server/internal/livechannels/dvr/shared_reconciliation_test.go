package dvr

import (
	"context"
	"database/sql"
	"fmt"
	"portico.local/server/internal/mediaartifact"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/livechannels"
)

func sharedClaims(t *testing.T) (*Store, *claim, Recording) {
	t.Helper()
	s, source, p, a := sharingStore(t)
	var records []Recording
	for i := 0; i < 2; i++ {
		o := livechannels.Owner{Authority: "local", AccountID: "account", ProfileID: fmt.Sprint(i)}
		r, e := s.Schedule(context.Background(), a, o, ScheduleInput{RequestID: strings.Repeat(fmt.Sprint(i+1), 48), Occurrence: Occurrence{source.ID, p.ChannelID, source.Generation, p.ID}})
		if e != nil {
			t.Fatal(e)
		}
		records = append(records, r)
	}
	leader, found, e := s.claimNext(context.Background())
	if e != nil || !found || leader == nil {
		t.Fatalf("leader: %v %v", leader, e)
	}
	follower := records[0]
	if follower.ID == leader.request.Recording.ID {
		follower = records[1]
	}
	if c, found, e := s.claimNext(context.Background()); e != nil || !found || c != nil {
		t.Fatalf("follower: %v %v", c, e)
	}
	return s, leader, follower
}

func TestSharedFollowerSurvivesEndDuringLeaderFinalization(t *testing.T) {
	for _, leaderState := range []string{"recording", "finalizing", "completed", "incomplete-playable"} {
		t.Run(leaderState, func(t *testing.T) {
			s, c, follower := sharedClaims(t)
			end, _ := time.Parse(time.RFC3339Nano, follower.End)
			s.now = func() time.Time { return end.Add(time.Second) }
			if _, e := s.db.Exec(`UPDATE dvr_recordings SET state=? WHERE id=?`, leaderState, c.request.Recording.ID); e != nil {
				t.Fatal(e)
			}
			if e := s.ReconcileScheduled(context.Background()); e != nil {
				t.Fatal(e)
			}
			if e := s.ExpireMissed(context.Background()); e != nil {
				t.Fatal(e)
			}
			var state string
			if e := s.db.QueryRow(`SELECT state FROM dvr_recordings WHERE id=?`, follower.ID).Scan(&state); e != nil {
				t.Fatal(e)
			}
			if state != "scheduled" {
				t.Fatalf("shared follower lost during %s: %s", leaderState, state)
			}
		})
	}
}

func TestSharedFollowerStillChecksUnclaimedRule(t *testing.T) {
	for _, which := range []string{"disabled", "deleted", "edited"} {
		t.Run(which, func(t *testing.T) {
			s, c, follower := sharedClaims(t)
			// The completed leader is ready to attach; rule maintenance has not run.
			if _, e := s.db.Exec(`UPDATE dvr_recordings SET state='completed' WHERE id=?`, c.request.Recording.ID); e != nil {
				t.Fatal(e)
			}
			var profile string
			if e := s.db.QueryRow(`SELECT profile_id FROM dvr_recordings WHERE id=?`, follower.ID).Scan(&profile); e != nil {
				t.Fatal(e)
			}
			enabled, deleted, revision := 1, 0, 1
			if which == "disabled" {
				enabled = 0
			}
			if which == "deleted" {
				deleted = 1
			}
			if which == "edited" {
				revision = 2
			}
			if _, e := s.db.Exec(`INSERT INTO dvr_rules(id,owner_key,authority,account_id,profile_id,source_id,series_id,revision,config_json,enabled,deleted,updated_ms) VALUES('rule',?,'local','account',?,?,'series',?,'{}',?,?,0)`, livechannels.Owner{Authority: "local", AccountID: "account", ProfileID: profile}.Key(), profile, follower.Occurrence.SourceID, revision, enabled, deleted); e != nil {
				t.Fatal(e)
			}
			if _, e := s.db.Exec(`UPDATE dvr_recordings SET rule_id='rule',rule_revision=1,next_attempt_ms=0 WHERE id=?`, follower.ID); e != nil {
				t.Fatal(e)
			}
			attached, found, e := s.claimNext(context.Background())
			if e != nil || !found {
				t.Fatalf("claim: %v", e)
			}
			if attached != nil {
				t.Fatalf("%s rule attached shared media", which)
			}
		})
	}
}

func TestCancelledCaptureLeaderKeepsAuthorizedFollower(t *testing.T) {
	for _, action := range []string{"cancel", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, c, _ := sharedClaims(t)
			ctx := context.Background()
			var revision int64
			if e := s.db.QueryRow(`SELECT revision FROM dvr_recordings WHERE id=?`, c.request.Recording.ID).Scan(&revision); e != nil {
				t.Fatal(e)
			}
			a := func(context.Context, *sql.Tx, bool) (string, func(string, string) bool, error) {
				return "owner", func(string, string) bool { return true }, nil
			}
			mutation := Mutation{RequestID: strings.Repeat("f", 48), ExpectedRevision: revision}
			var e error
			if action == "cancel" {
				_, e = s.Cancel(ctx, a, c.request.Owner, c.request.Recording.ID, mutation)
			} else {
				_, e = s.Delete(ctx, a, c.request.Owner, c.request.Recording.ID, mutation)
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = s.renew(ctx, c, false); e != nil {
				t.Fatalf("%s cut authorized follower's capture: %v", action, e)
			}
			cp := CaptureCheckpoint{Object: mediaartifact.Object{Size: 1880}, First: time.Now().Add(-time.Minute), Last: time.Now()}
			if e = s.checkpoint(ctx, c, cp); e != nil {
				t.Fatal(e)
			}
			var state string
			if e = s.db.QueryRow(`SELECT state FROM dvr_recordings WHERE id=?`, c.request.Recording.ID).Scan(&state); e != nil {
				t.Fatal(e)
			}
			expected := "cancelled"
			if action == "delete" {
				expected = "pending-delete"
			}
			if state != expected {
				t.Fatalf("capture resurrected cancelled owner: %s", state)
			}
		})
	}
}
