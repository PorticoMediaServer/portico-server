package catalog

import (
	"database/sql"
	"encoding/json"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/operations"
)

// storePreference writes one profile-server override the way ApplyPreferences
// would, so the catalog reads exactly the rows the console writes.
func storePreference(t *testing.T, db *sql.DB, v identity.Viewer, values map[string]any) {
	t.Helper()
	body, e := json.Marshal(values)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO console_documents VALUES(?,2,?,0) ON CONFLICT(scope) DO UPDATE SET revision=revision+1,body=excluded.body`, "profile:"+operations.ViewerScopeKey(v), string(body)); e != nil {
		t.Fatal(e)
	}
}

func accept(t *testing.T, s *Service, db *sql.DB, v identity.Viewer, item string, seq int64, position, duration float64, state string) {
	t.Helper()
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if e = s.AcceptPlaybackProgress(tx, v, "play", item, seq, position, duration, state, true); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
}

func TestPlaybackThresholdsFollowViewerPreferences(t *testing.T) {
	s, db, v := savedFixture(t)
	item := phase34SavedFixtureItem(t, db)
	key := identity.PersonalKey(v)
	if _, e := db.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,0,0,0,'play')`, key, item.ID); e != nil {
		t.Fatal(e)
	}
	// Default policy: 80 of 100 seconds is started, not played.
	accept(t, s, db, v, item.Public, 1, 80, 100, "playing")
	p, e := s.Personal(key, item.Public)
	if e != nil || p.Watched || p.ProgressSeconds != 80 {
		t.Fatalf("default played threshold moved: %+v %v", p, e)
	}
	// A viewer who counts 75 percent as played reaches watched at the same point.
	storePreference(t, db, v, map[string]any{"playback.playedThresholdPercent": 75})
	accept(t, s, db, v, item.Public, 2, 80, 100, "playing")
	if p, e = s.Personal(key, item.Public); e != nil || !p.Watched {
		t.Fatalf("played threshold override ignored: %+v %v", p, e)
	}
}

func TestLibraryVideoCompletionUsesApprovedCreditsOrThreshold(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		position   float64
		threshold  int
		marker     bool
		approved   int
		want       bool
	}{
		{"default ninety percent", "earliest", 90, 90, false, 0, true},
		{"owner threshold eighty percent", "threshold", 80, 80, false, 0, true},
		{"approved credits finish early", "earliest", 85, 90, true, 1, true},
		{"threshold ignores credits", "threshold", 85, 90, true, 1, false},
		{"credits mode uses marker", "credits", 85, 90, true, 1, true},
		{"credits mode ignores percentage", "credits", 95, 90, false, 0, false},
		{"unapproved guess cannot finish", "earliest", 85, 90, true, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db, v := savedFixture(t)
			item := phase34SavedFixtureItem(t, db)
			key := identity.PersonalKey(v)
			if _, err := db.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,0,0,0,'play')`, key, item.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO library_sources(id,library_id,configured_root,root) VALUES('source','library','/markers','/markers')`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns)
	 VALUES('object','source',?,'root','item.mkv','revision','{}',1,1)`, item.Token); err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(map[string]any{"continueWatching": map[string]any{"videoPlayedThreshold": tc.threshold, "videoCompletion": tc.mode}})
			if _, err := db.Exec(`INSERT INTO admin_documents(scope,revision,body,updated_ms) VALUES('library:library',2,?,0)`, string(body)); err != nil {
				t.Fatal(err)
			}
			if tc.marker {
				if _, err := db.Exec(`INSERT INTO analysis_markers(id,object_id,source_revision,source_binding,kind,start_us,end_us,confidence,provenance,result_id,approved)
 VALUES('credits','object','revision','binding','credits',80000000,100000000,1,'test','',?)`, tc.approved); err != nil {
					t.Fatal(err)
				}
			}
			catalogtest.New(t, db).Drain()
			accept(t, s, db, v, item.Public, 1, tc.position, 100, "playing")
			personal, err := s.Personal(key, item.Public)
			if err != nil || personal.Watched != tc.want {
				t.Fatalf("watched=%v want %v: %v", personal.Watched, tc.want, err)
			}
		})
	}
}

func TestStartedThresholdHonoursPercentUnderThirtySeconds(t *testing.T) {
	s, db, v := savedFixture(t)
	item := phase34SavedFixtureItem(t, db)
	key := identity.PersonalKey(v)
	if _, e := db.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,0,0,0,'play')`, key, item.ID); e != nil {
		t.Fatal(e)
	}
	// The thirty-second ceiling still applies: five percent of a long feature is
	// 360 seconds, so 40 seconds already counts as started.
	accept(t, s, db, v, item.Public, 1, 40, 7200, "playing")
	p, e := s.Personal(key, item.Public)
	if e != nil || p.ProgressSeconds != 40 {
		t.Fatalf("thirty-second ceiling lost: %+v %v", p, e)
	}
	// A one-percent viewer on a short item starts sooner than the default five.
	s2, db2, v2 := savedFixture(t)
	item2 := phase34SavedFixtureItem(t, db2)
	key2 := identity.PersonalKey(v2)
	if _, e = db2.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,0,0,0,'play')`, key2, item2.ID); e != nil {
		t.Fatal(e)
	}
	accept(t, s2, db2, v2, item2.Public, 1, 5, 200, "playing")
	if p, e = s2.Personal(key2, item2.Public); e != nil || p.ProgressSeconds != 0 {
		t.Fatalf("default started threshold moved: %+v %v", p, e)
	}
	storePreference(t, db2, v2, map[string]any{"playback.startedThresholdPercent": 1})
	accept(t, s2, db2, v2, item2.Public, 2, 5, 200, "playing")
	if p, e = s2.Personal(key2, item2.Public); e != nil || p.ProgressSeconds != 5 {
		t.Fatalf("started threshold override ignored: %+v %v", p, e)
	}
}

func TestPauseWatchHistoryStopsNewProgressWrites(t *testing.T) {
	s, db, v := savedFixture(t)
	item := phase34SavedFixtureItem(t, db)
	key := identity.PersonalKey(v)
	if _, e := db.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,0,0,0,'play')`, key, item.ID); e != nil {
		t.Fatal(e)
	}
	accept(t, s, db, v, item.Public, 1, 40, 100, "playing")
	storePreference(t, db, v, map[string]any{"privacy.pauseWatchHistory": true})
	accept(t, s, db, v, item.Public, 2, 90, 100, "playing")
	p, e := s.Personal(key, item.Public)
	if e != nil || p.Watched || p.ProgressSeconds != 40 {
		t.Fatalf("a paused history recorded new progress: %+v %v", p, e)
	}
	var rows int
	if e = db.QueryRow(`SELECT count(*) FROM personal_history WHERE profile_id=?`, key).Scan(&rows); e != nil || rows != 1 {
		t.Fatalf("a paused history appended an occurrence: %d %v", rows, e)
	}
	// Manual watched still works while history is paused.
	yes := true
	if p, e = s.SetPersonal("local:account", key, item.Public, PersonalMutation{OperationID: "manual", ExpectedRevision: p.Revision, Watched: &yes}, nil); e != nil || !p.Watched {
		t.Fatalf("a paused history blocked a manual mutation: %+v %v", p, e)
	}
	// Resuming records again from the existing state.
	storePreference(t, db, v, map[string]any{"privacy.pauseWatchHistory": false})
	accept(t, s, db, v, item.Public, 3, 60, 100, "playing")
	var rowsAfter int
	if e = db.QueryRow(`SELECT count(*) FROM personal_history WHERE profile_id=?`, key).Scan(&rowsAfter); e != nil || rowsAfter != 1 {
		t.Fatalf("resume did not reuse the occurrence: %d %v", rowsAfter, e)
	}
	var position float64
	if e = db.QueryRow(`SELECT position FROM personal_history WHERE profile_id=?`, key).Scan(&position); e != nil || position != 60000 {
		t.Fatalf("resume did not record: %v %v", position, e)
	}
}

func TestSearchHistoryRemembersOnlyWhenPreferred(t *testing.T) {
	s, db, v := savedFixture(t)
	if e := s.RecordSearch(v, "  Harbor   Lights "); e != nil {
		t.Fatal(e)
	}
	page, e := s.SearchHistory(v)
	if e != nil || !page.Remembered || len(page.Entries) != 1 || page.Entries[0].Query != "Harbor Lights" {
		t.Fatalf("history not recorded: %+v %v", page, e)
	}
	// The same query in another case updates one entry rather than adding a second.
	if e = s.RecordSearch(v, "harbor lights"); e != nil {
		t.Fatal(e)
	}
	if page, e = s.SearchHistory(v); e != nil || len(page.Entries) != 1 {
		t.Fatalf("duplicate entry: %+v %v", page, e)
	}
	storePreference(t, db, v, map[string]any{"search.rememberHistory": false})
	if e = s.RecordSearch(v, "Another Title"); e != nil {
		t.Fatal(e)
	}
	if page, e = s.SearchHistory(v); e != nil || page.Remembered || len(page.Entries) != 0 {
		t.Fatalf("history returned while remembering is off: %+v %v", page, e)
	}
	storePreference(t, db, v, map[string]any{"search.rememberHistory": true})
	if page, e = s.SearchHistory(v); e != nil || len(page.Entries) != 1 || page.Entries[0].Query != "harbor lights" {
		t.Fatalf("a paused search recorded anyway: %+v %v", page, e)
	}
	if page, e = s.ClearSearchHistory(v); e != nil || len(page.Entries) != 0 {
		t.Fatalf("clear left entries: %+v %v", page, e)
	}
}

func TestSearchHistoryIsBoundedAndPerViewer(t *testing.T) {
	s, _, v := savedFixture(t)
	for i := 0; i < SearchHistoryLimit+8; i++ {
		if e := s.RecordSearch(v, string(rune('a'+i%26))+"query"+string(rune('0'+i%10))+"-"+string(rune('A'+i))); e != nil {
			t.Fatal(e)
		}
	}
	page, e := s.SearchHistory(v)
	if e != nil || len(page.Entries) != SearchHistoryLimit {
		t.Fatalf("history is unbounded: %d %v", len(page.Entries), e)
	}
	other := v
	other.AccountID = "other"
	if page, e = s.SearchHistory(other); e != nil || len(page.Entries) != 0 {
		t.Fatalf("history leaked across viewers: %+v %v", page, e)
	}
}
