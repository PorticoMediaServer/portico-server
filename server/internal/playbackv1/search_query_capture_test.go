package playbackv1

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
	"testing"
)

func TestSearchCaptureRestartBindsParentalTighteningAndRelaxing(t *testing.T) {
	for _, tighten := range []bool{true, false} {
		for _, restart := range []bool{true, false} {
			t.Run(fmt.Sprintf("tighten=%v/restart=%v", tighten, restart), func(t *testing.T) {
				c := catalogtest.Open(t)
				lib := c.Library("music", "Music", "music", "/music")
				album := c.Album(c.Artist(lib, "Artist"), "Album", 2026)
				for i := 0; i < 520; i++ {
					song := c.Song(album, i+1, fmt.Sprintf("/music/%04d.flac", i), fmt.Sprintf("Harbor %04d", i))
					if i%2 == 0 {
						c.Attributes(song.ID, "label", "Adult")
					}
				}
				c.Drain()
				v := catalog.Viewer{Profile: "local:account:profile", Fence: "before", Libraries: []string{"music"}}
				if !tighten {
					v.Restrictions = identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}
				}
				s := New(c.DB, nil, nil)
				t.Cleanup(func() { releaseCaptures(s) })
				s.QueryKeys = func(ctx context.Context, tx *sql.Tx, p identity.Principal, sel Selector) (catalog.QueueSelection, error) {
					return catalog.New(c.DB).SearchQueueKeysTx(ctx, tx, v, *sel.Search)
				}
				p := identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: "account", ProfileID: "profile"}}
				in := SegmentInput{Source: Selector{Search: &catalog.SearchQueueQuery{Q: "Harbor", Group: "songs", Sort: "title", Direction: "asc"}}}
				snap, err := s.buildSnapshot(context.Background(), p, "q_test", in, "source", MaxQueueKeys, nil, scanBudget{keys: 256})
				if err != nil || !snap.partial || snap.count != 256 {
					t.Fatal(snap, err)
				}
				snap.caller = `{"viewer":{"authority":"local","accountId":"account","profileId":"profile"}}`
				c.Exec(`INSERT INTO queues_v1(id,owner_kind,owner_id,authority,account_id,profile_id,revision,created_ms,updated_ms) VALUES('q_test','device','device','local','account','profile',1,0,0)`)
				tx, err := c.DB.Begin()
				if err != nil {
					t.Fatal(err)
				}
				if err = storeSnapshot(context.Background(), tx, "q_test", &snap); err != nil {
					tx.Rollback()
					t.Fatal(err)
				}
				if _, err = tx.Exec(`INSERT INTO queue_v1_segments(id,queue_id,sort_key,snapshot_id,first_ordinal,count) VALUES('g_test','q_test','a',?,0,256)`, snap.id); err != nil {
					tx.Rollback()
					t.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
				// Only effective current authority changes: no catalogue or personal revision
				// can incidentally make this test reject a changed selection stream.
				v.Fence = "after"
				if tighten {
					v.Restrictions = identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}
				} else {
					v.Restrictions = identity.ContentRestrictions{}
				}
				if restart {
					s.releaseCapture(snap.id)
				}
				err = s.continueBuild(context.Background(), snap.id)
				if restart {
					if !errors.Is(err, errSelectionChanged) {
						t.Fatalf("mixed authority stream resumed at old seen offset: %v", err)
					}
					s.failBuild(snap.id)
				} else if err != nil {
					t.Fatal("retained source lost its pinned view", err)
				}
				var count int
				var state string
				if err = c.DB.QueryRow(`SELECT count,state FROM queue_v1_snapshots WHERE id=?`, snap.id).Scan(&count, &state); err != nil {
					t.Fatal(err)
				}
				want := 520
				if !tighten {
					want = 260
				}
				if restart {
					if count != 256 || state != "failed" {
						t.Fatal("restart persisted mixed keys", count, state)
					}
				} else if count != want || state != "ready" {
					t.Fatal("retained source changed", count, state)
				}
			})
		}
	}
}
