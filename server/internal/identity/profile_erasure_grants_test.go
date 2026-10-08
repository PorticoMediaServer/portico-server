package identity

import (
	"context"
	"testing"
)

func TestDirectProfileErasureRemovesOnlyExactRecordingGrant(t *testing.T) {
	s, db, login := directFixture(t)
	ctx := context.Background()
	rows, e := s.CreateDirectProfile(ctx, login.AccountToken, "Delete me", "blue")
	if e != nil {
		t.Fatal(e)
	}
	child := childProfile(t, rows)
	for _, authority := range []string{"local", "hosted"} {
		for _, profile := range []string{"", child.ID} {
			if _, e = db.Exec(`INSERT INTO dvr_recording_grants VALUES(?,?,?,1,1,1)`, authority, "account", profile); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e = s.DeleteDirectProfile(ctx, login.AccountToken, child.ID, child.Revision, true); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = db.QueryRow(`SELECT count(*) FROM dvr_recording_grants WHERE authority='local' AND profile_id=?`, child.ID).Scan(&count); e != nil || count != 0 {
		t.Fatal("deleted profile retained grant", count, e)
	}
	if e = db.QueryRow(`SELECT count(*) FROM dvr_recording_grants`).Scan(&count); e != nil || count != 3 {
		t.Fatal("other authority or account inheritance removed", count, e)
	}
}
