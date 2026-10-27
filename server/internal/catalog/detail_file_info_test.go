package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
)

func TestDetailFileInfoMemberTechnicalFactsAndOwnerPaths(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("movies", "Movies", "movie", "/movies")
	movie := c.Movie(library, "/movies/Private.Title.mkv", "Title", 2020)
	c.Drain()
	s := New(c.DB)
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"movies"}}
	member, err := s.FileInfo(viewer, "server", movie.Public, "", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(member.Files) != 1 || member.Files[0].Path != "" || member.Manage != nil || member.Files[0].Duration <= 0 {
		t.Fatalf("member %+v", member)
	}
	owner, err := s.FileInfo(viewer, "server", movie.Public, "", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Files[0].Path != "/movies/Private.Title.mkv" || owner.Manage == nil || !owner.Manage.Delete {
		t.Fatalf("owner %+v", owner)
	}
	viewer.Libraries = nil
	if _, err = s.FileInfo(viewer, "server", movie.Public, "", 0, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("hidden library: %v", err)
	}
}

func TestFileInfoBatchesChildPageAndPreservesFacts(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("music", "Music", "music", "/music")
	artist := c.Artist(library, "Artist")
	album := c.Album(artist, "Album", 2026)
	items := make([]catalogtest.Item, 201)
	for i := range items {
		items[i] = c.Song(album, i+1, fmt.Sprintf("/music/%03d.flac", i), fmt.Sprintf("Track %03d", i))
	}
	// Multiple versions and one asset linked to two authorized children retain
	// the per-child version order and each child's own track arrays.
	c.File(items[0].ID, "/music/alternate.flac", 120)
	c.Exec(`INSERT INTO catalog_asset_links(entity_id,asset_id,available,part_index,edition) VALUES(?,?,1,0,'shared')`, items[1].ID, items[0].Asset)
	c.Exec(`INSERT INTO asset_stream_facts(asset_id,revision,size,modified_ns,fingerprint) SELECT token,1,size,modified_ns,'test' FROM catalog_assets`)
	c.Exec(`INSERT INTO asset_streams(asset_id,stream_index,type,codec,language,title,channels,channel_layout,is_default,is_forced,color_transfer,detail_json)
 SELECT token,0,'video','h264','','Picture',0,'',1,0,'bt709','' FROM catalog_assets
 UNION ALL SELECT token,1,'audio','flac','en','English',2,'stereo',1,0,'','' FROM catalog_assets
 UNION ALL SELECT token,2,'audio','aac','fr','French',6,'5.1',0,0,'','' FROM catalog_assets`)
	c.Exec(`INSERT INTO asset_subtitle_facts(asset_id,revision,size,modified_ns,origin_us,timing_known,status,fingerprint) SELECT token,1,size,modified_ns,0,1,'known','test' FROM catalog_assets`)
	c.Exec(`INSERT INTO asset_subtitles(id,asset_id,origin,locator,stream_index,format,language,title,is_default,is_forced,reason)
 SELECT token||'-fr',token,'embedded','stream4',4,'srt','fr','French',0,1,'' FROM catalog_assets
 UNION ALL SELECT token||'-en',token,'embedded','stream3',3,'ass','en','English',1,0,'' FROM catalog_assets`)
	c.Drain()
	s := New(c.DB)
	viewer := Viewer{Profile: "p", Fence: "f", Libraries: []string{"music"}}
	for _, owner := range []bool{false, true} {
		for _, limit := range []int{40, 200} {
			var want []TitleFile
			for _, item := range items[:limit] {
				files, err := s.referenceTitleFiles(item.Public, owner)
				if err != nil {
					t.Fatal(err)
				}
				for i := range files {
					files[i].ItemID = item.Public
				}
				want = append(want, files...)
			}
			ctx, cost := dbwork.Measure(context.Background())
			var got FileInfo
			err := dbwork.WithReadSnapshot(ctx, c.DB, func(ctx context.Context) error {
				var err error
				got, err = s.WithContext(ctx).FileInfo(viewer, "server", album.Public, "", limit, owner)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Files, want) || got.NextCursor == "" {
				t.Fatalf("owner=%t limit=%d: facts/order/continuation differ, got %d want %d", owner, limit, len(got.Files), len(want))
			}
			if statements := cost().Statements; statements > 24 {
				t.Fatalf("owner=%t limit=%d: %d statements, child/asset reads are not batched", owner, limit, statements)
			}
		}
	}
	viewer.Libraries = nil
	if got, err := s.FileInfo(viewer, "server", album.Public, "", 40, true); !errors.Is(err, sql.ErrNoRows) || len(got.Files) != 0 {
		t.Fatalf("denied library returned files: %d %v", len(got.Files), err)
	}
}

func TestFileInfoBatchedDiscFactsKeepObservationFences(t *testing.T) {
	s, db, source, job, object, policy, probe := discPublicationFixture(t)
	var token, revision string
	if err := db.QueryRow(`SELECT asset_id,revision FROM inventory_objects WHERE id=?`, object).Scan(&token, &revision); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitInventoryDiscProbe(context.Background(), job, object, revision, policy, probe); err != nil {
		t.Fatal(err)
	}
	c := catalogtest.New(t, db)
	c.Exec(`UPDATE catalog_assets SET available=1 WHERE token=?`, token)
	c.Exec(`INSERT INTO asset_stream_facts(asset_id,revision,size,modified_ns,fingerprint) SELECT token,7,size,modified_ns,'test' FROM catalog_assets WHERE token=?`, token)
	c.Exec(`INSERT INTO asset_chapter_facts(asset_id,revision,size,modified_ns,status,fingerprint) SELECT token,9,size,modified_ns,'known','test' FROM catalog_assets WHERE token=?`, token)
	c.Exec(`UPDATE inventory_disc_facts SET media_title_key=?,media_stream_revision=7,media_chapter_revision=9 WHERE object_id=?`, probe.Titles[0].Key(probe.Kind), object)
	compare := func(present bool) {
		t.Helper()
		want, err := ReadDiscMediaFacts(context.Background(), s.read(), token)
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.readDiscMediaFactsBatch([]string{token, "absent"})
		if err != nil || !reflect.DeepEqual(got[token], want) || (want != nil) != present || got["absent"] != nil {
			t.Fatalf("batch changed exact disc facts: got=%+v want=%+v present=%t err=%v", got[token], want, present, err)
		}
	}
	compare(true)
	// The owner's title list still comes from the original current observation
	// and the existing fingerprint-checked probe, not the optional display facts.
	err := dbwork.WithReadSnapshot(context.Background(), db, func(ctx context.Context) error {
		got := s.WithContext(ctx).discTitles([]TitleFile{{ID: "absent"}, {ID: token}})
		if got == nil || got.FileID != token || len(got.Titles) != len(probe.Titles) {
			t.Fatalf("owner title list changed: %+v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	c.Exec(`UPDATE library_sources SET generation=generation+1 WHERE id=?`, source.ID)
	compare(false)
	c.Exec(`UPDATE library_sources SET generation=generation-1 WHERE id=?`, source.ID)
	c.Exec(`UPDATE inventory_objects SET revision=revision||'-changed' WHERE id=?`, object)
	compare(false)
	c.Exec(`UPDATE inventory_objects SET revision=? WHERE id=?`, revision, object)
	c.Exec(`UPDATE inventory_objects SET state='unavailable' WHERE id=?`, object)
	compare(false)
	c.Exec(`UPDATE inventory_objects SET state='available' WHERE id=?`, object)
	compare(true)
}
