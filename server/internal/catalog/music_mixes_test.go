package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/sonic"
)

func TestMusicMixesSeparateHistoryCoverageAndExpirePrivateSelections(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("music", "Music", "music", "/music")
	album := c.Album(c.Artist(library, "Actual artist"), "Album", 2026)
	at := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	items := []catalogtest.Item{}
	for n := 0; n < 8; n++ {
		item := c.Song(album, n+1, fmt.Sprintf("/music/%d.flac", n), fmt.Sprintf("Song %d", n))
		sonicStoredFixture(t, c, item, float32(8-n), float32(n+1))
		items = append(items, item)
	}
	c.Attributes(items[7].ID, "label", "private")
	c.Drain()
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		for n, item := range items[:6] {
			_, err := tx.ExecContext(ctx, `INSERT INTO play_history(playback_id,authority,account_id,profile_id,user_name,item_id,library_id,kind,title,started_ms,updated_ms,watched_ms,completed) VALUES(?,'local','account','profile','Fixture',?,'music',7,?,?,?,100000,1)`, fmt.Sprint(n), item.ID, item.Public, at.Add(-time.Hour).UnixMilli(), at.Add(-time.Hour).UnixMilli())
			if err != nil {
				return err
			}
		}
		return nil
	})
	viewer := Viewer{Profile: identity.PersonalKey(identity.Viewer{Authority: "local", AccountID: "account", ProfileID: "profile"}), Libraries: []string{"music"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"private"}}}
	s := New(c.DB)
	got, err := s.MusicMixes(viewer, at)
	if err != nil || got.HistoryTracks != 6 || got.AnalyzedHistoryTracks != 6 || got.LibraryTracks != 7 || got.AnalyzedLibraryTracks != 7 || len(got.Mixes) != 1 {
		t.Fatal(got, err)
	}
	id := got.Mixes[0].ID
	keys, err := s.MusicMixKeys(viewer, at, id)
	if err != nil || !reflect.DeepEqual(keys, got.Mixes[0].Cluster.Tracks) {
		t.Fatal(keys, got, err)
	}
	other := viewer
	other.Profile = identity.PersonalKey(identity.Viewer{Authority: "local", AccountID: "account", ProfileID: "other"})
	if _, err := s.MusicMixKeys(other, at, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("another profile resolved private mix", err)
	}
	other = viewer
	other.Fence = "new-permission-fence"
	if _, err := s.MusicMixKeys(other, at, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("old permission fence resolved mix", err)
	}
	// Even when played tracks remain analyzed, unplayed stale source facts
	// reduce actual library coverage. Never substitute history's 100 percent.
	c.Exec(`UPDATE music_sonic_embeddings SET model_sha256='old' WHERE entity_id=?`, items[6].ID)
	got, err = s.MusicMixes(viewer, at)
	if err != nil || got.AnalyzedHistoryTracks != 6 || got.AnalyzedLibraryTracks != 6 || got.LibraryTracks != 7 {
		t.Fatal(got, err)
	}
	c.Exec(`UPDATE music_sonic_embeddings SET model_sha256='old' WHERE entity_id=?`, items[0].ID)
	if _, err := s.MusicMixKeys(viewer, at, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("changed membership retained old identity", err)
	}
	c.Exec(`UPDATE music_sonic_embeddings SET model_sha256=?`, sonic.ModelSHA256)
	c.Exec(`INSERT INTO library_sonic_settings(library_id,enabled,revision) VALUES('music',0,1)`)
	got, err = s.MusicMixes(viewer, at)
	if err != nil || got.Model != nil || len(got.Mixes) != 0 || got.LibraryTracks != 0 {
		t.Fatal("Off produced mixes", got, err)
	}
	if _, err := s.MusicMixKeys(viewer, at, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("Off resolved old selection", err)
	}
}

func TestMusicMixIDsBindOrderedMembersAndExactModel(t *testing.T) {
	v := Viewer{Profile: "profile", Fence: "fence"}
	model := sonic.Identity{ID: sonic.ModelID, Version: sonic.ModelVersion, SHA256: sonic.ModelSHA256}
	first := musicMixID(v, model, []string{"one", "two"})
	if len(first) != 47 || first != musicMixID(v, model, []string{"one", "two"}) || first == musicMixID(v, model, []string{"two", "one"}) {
		t.Fatal("unstable or order-insensitive identity", first)
	}
	model.Version = "next"
	if first == musicMixID(v, model, []string{"one", "two"}) {
		t.Fatal("model change retained mix identity")
	}
}
