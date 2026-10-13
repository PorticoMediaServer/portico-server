package catalog

import (
	"errors"
	"testing"

	"portico.local/server/internal/identity"
)

func TestContinueDismissalPublishesSemanticChange(t *testing.T) {
	s, db, v := savedFixture(t)
	item := phase34SavedFixtureItem(t, db)
	profile := identity.PersonalKey(v)
	homeExec(t, db, `INSERT INTO progress(profile_id,item_id,position,unit,playback_id) VALUES(?,?,0,0,'play')`, profile, item.ID)
	accept(t, s, db, v, item.Public, 1, 40, 100, "playing")
	state, err := s.Personal(profile, item.Public)
	if err != nil {
		t.Fatal(err)
	}
	for i, dismissed := range []bool{true, false} {
		var before int64
		if err = db.QueryRow(`SELECT revision FROM viewer_revisions WHERE profile_id=? AND library_id='library'`, profile).Scan(&before); err != nil {
			t.Fatal(err)
		}
		homeExec(t, db, `DELETE FROM api_change_pending`)
		state, err = s.SetPersonal("local:account", profile, item.Public, PersonalMutation{OperationID: []string{"dismiss", "restore"}[i], ExpectedRevision: state.Revision, ContinueDismissed: &dismissed}, nil)
		if err != nil {
			t.Fatal(err)
		}
		var after, hints int64
		if err = db.QueryRow(`SELECT revision FROM viewer_revisions WHERE profile_id=? AND library_id='library'`, profile).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRow(`SELECT count(*) FROM api_change_pending WHERE audience=? AND type='personal.changed' AND resource_id='library'`, "personal:"+profile).Scan(&hints); err != nil {
			t.Fatal(err)
		}
		if after <= before || hints != 1 {
			t.Fatalf("dismiss=%v: semantic revision %d -> %d, hints=%d", dismissed, before, after, hints)
		}
	}
}

func TestExplicitResumeAdvancesCommandRevision(t *testing.T) {
	s, db, v := savedFixture(t)
	item := phase34SavedFixtureItem(t, db)
	profile := identity.PersonalKey(v)
	no := false
	pos := 20.
	first, err := s.SetPersonal("local:account", profile, item.Public, PersonalMutation{OperationID: "resume-one", Watched: &no, ProgressSeconds: &pos}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pos = 40
	second, err := s.SetPersonal("local:account", profile, item.Public, PersonalMutation{OperationID: "resume-two", ExpectedRevision: first.Revision, Watched: &no, ProgressSeconds: &pos}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision <= first.Revision {
		t.Fatalf("manual resume kept command revision %d -> %d", first.Revision, second.Revision)
	}
	pos = 60
	_, err = s.SetPersonal("local:account", profile, item.Public, PersonalMutation{OperationID: "stale-resume", ExpectedRevision: first.Revision, Watched: &no, ProgressSeconds: &pos}, nil)
	if !errors.Is(err, ErrPersonalConflict) {
		t.Fatalf("stale resume: %v", err)
	}
	state, err := s.Personal(profile, item.Public)
	if err != nil || state.ProgressSeconds != 40 {
		t.Fatalf("stale edit changed position: %+v %v", state, err)
	}
}
