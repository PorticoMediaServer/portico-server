package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/playbackv1"
	"strings"
	"testing"
)

func TestSearchQueueCreateReplayReprojectsCurrentLibraryAndParentalAccess(t *testing.T) {
	for _, shuffle := range []bool{false, true} {
		for _, revoke := range []string{"library", "parental"} {
			t.Run(fmt.Sprintf("shuffle=%v/revoke=%s", shuffle, revoke), func(t *testing.T) {
				f := musicFixture(t, 2)
				member, _ := f.member()
				c := f.catalogTest
				c.Exec(`UPDATE libraries SET kind='music' WHERE id=?`, f.library)
				artist := f.names["fixture-artist"]
				album := c.Album(artist, "Private album", 2026)
				for i, song := range f.records {
					c.Fields(song.ID, map[string]any{"title": fmt.Sprintf("Harbor Private Song %d", i)})
					c.Attributes(song.ID, "label", "Adult")
					c.Write(func(ctx context.Context, tx *sql.Tx) error {
						return compactcatalog.SetFactsTx(ctx, tx, song.ID, map[string]any{"album_id": album.ID})
					})
				}
				c.Drain()
				c.Exec(`UPDATE direct_memberships SET allowed_libraries=? WHERE account_id='member'`, `["`+f.library+`"]`)
				source := map[string]any{"search": map[string]any{"q": "Harbor", "group": "songs", "sort": "title", "direction": "asc", "libraryIds": []string{f.library}}}
				body := map[string]any{"selector": source, "startPlayback": map[string]any{"state": "playing"}}
				if shuffle {
					body["shuffle"] = map[string]any{"seed": "42"}
				}
				headers := map[string]string{"Idempotency-Key": "search-replay-private-001"}
				var original playbackv1.QueueReply
				f.callAs(member, "POST", "/v1/queues", headers, body, 201, &original)
				if original.Queue.Total != 2 || len(original.Window) != 2 || original.Session == nil {
					t.Fatal("allowed Search create control", original)
				}
				var control playbackv1.QueueReply
				f.callAs(member, "POST", "/v1/queues", headers, body, 201, &control)
				if control.Queue.ID != original.Queue.ID || control.Session == nil || control.Session.ID != original.Session.ID {
					t.Fatal("replay reexecuted mutation", control)
				}
				counts := func() []int {
					out := []int{}
					for _, table := range []string{"queues_v1", "queue_v1_snapshots", "playback_v1_sessions"} {
						var n int
						if err := f.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
							t.Fatal(err)
						}
						out = append(out, n)
					}
					return out
				}
				before := fmt.Sprint(counts())
				if revoke == "library" {
					c.Exec(`UPDATE direct_memberships SET allowed_libraries='[]' WHERE account_id='member'`)
				} else {
					c.Exec(`INSERT INTO profile_restrictions(profile_id,blocked_labels) VALUES('member-profile','["Adult"]') ON CONFLICT(profile_id) DO UPDATE SET blocked_labels=excluded.blocked_labels`)
				}
				var replay playbackv1.QueueReply
				w := f.callAs(member, "POST", "/v1/queues", headers, body, 201, &replay)
				if replay.Queue.ID != original.Queue.ID || replay.Session != nil {
					t.Fatal("revoked session leaked", w.Body.String())
				}
				for _, entry := range replay.Window {
					if entry.ItemID != "" || entry.Title != "" || entry.Available {
						t.Fatal("stored private item metadata replayed", entry)
					}
				}
				for _, secret := range append([]string{"Harbor Private", "Private album", original.Session.ID}, f.items...) {
					if strings.Contains(w.Body.String(), secret) {
						t.Fatalf("revoked replay body exposes %q: %s", secret, w.Body.String())
					}
				}
				if got := fmt.Sprint(counts()); got != before {
					t.Fatal("replay reexecuted create/capture/playback", before, got)
				}
				c.Exec(`UPDATE queues_v1 SET account_id='other-account' WHERE id=?`, original.Queue.ID)
				denied := f.raw("POST", "/v1/queues", member, headers, body)
				if denied.Code != 404 || strings.Contains(denied.Body.String(), original.Queue.ID) {
					t.Fatal("receipt bypassed current queue ownership", denied.Code, denied.Body.String())
				}
				// The current reply is still a real bounded queue DTO.
				var raw map[string]any
				if json.Unmarshal(w.Body.Bytes(), &raw) != nil {
					t.Fatal("invalid reply")
				}
			})
		}
	}
}
