package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

// identityFile records a file on disk as the scanner would have: a catalogue
// asset and one inventory record saying it is there.
func identityFile(t *testing.T, c *catalogtest.Catalog, library, path string, height int) (int64, string) {
	t.Helper()
	var asset int64
	var token string
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		asset, token, err = compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: path, Size: int64(1000 * height), ModifiedNS: 1, Container: "mkv", VideoCodec: "h264", AudioCodec: "aac", Width: height * 16 / 9, Height: height, Duration: 5400})
		return err
	})
	c.Exec(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns)
 SELECT ?,id,?,incarnation,?,'r','{}',?,1 FROM library_sources WHERE library_id=? ORDER BY id LIMIT 1`, "object:"+token, token, path, int64(1000*height), library)
	return asset, token
}

func attachMovie(t *testing.T, c *catalogtest.Catalog, library string, asset int64, path, title string, year int) int64 {
	t.Helper()
	var item int64
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		item, err = AttachMovieTx(ctx, tx, MovieFile{Library: library, Asset: asset, Path: path, Title: title, Year: year})
		return err
	})
	return item
}

func markMissing(t *testing.T, c *catalogtest.Catalog, library, token string, when time.Time) {
	t.Helper()
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var source string
		if err := tx.QueryRowContext(ctx, `SELECT source_id FROM inventory_objects WHERE id=?`, "object:"+token).Scan(&source); err != nil {
			return err
		}
		return MarkInventoryMissingTx(ctx, tx, source, "job-"+token, "object:"+token, when)
	})
}

func rows(t *testing.T, c *catalogtest.Catalog, query string, args ...any) int {
	t.Helper()
	var n int
	if err := c.DB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%v: %s", err, query)
	}
	return n
}

func TestTwoFilesOfOneFilmAreVersionsOfOneTitle(t *testing.T) {
	c := catalogtest.Open(t)
	c.Library("m", "Movies", "movie", "/m")
	a, _ := identityFile(t, c, "m", "/m/Alien (1979) 720p.mkv", 720)
	b, _ := identityFile(t, c, "m", "/m/4K/ALIEN.1979.2160p.mkv", 2160)
	first := attachMovie(t, c, "m", a, "/m/Alien (1979) 720p.mkv", "Alien", 1979)
	second := attachMovie(t, c, "m", b, "/m/4K/ALIEN.1979.2160p.mkv", "ALIEN", 1979)
	if first != second {
		t.Fatalf("a second file of the same film became a second title: %d %d", first, second)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_asset_links WHERE entity_id=? AND part_index=0`, first); n != 2 {
		t.Fatalf("versions: %d", n)
	}
	// Another film with the same title and a different year is another title.
	d, _ := identityFile(t, c, "m", "/m/Alien (2025).mkv", 1080)
	if other := attachMovie(t, c, "m", d, "/m/Alien (2025).mkv", "Alien", 2025); other == first {
		t.Fatal("two films of different years were merged")
	}
}

func TestAReplacementUnderANewNameKeepsTheTitleAndItsState(t *testing.T) {
	c := catalogtest.Open(t)
	c.Library("m", "Movies", "movie", "/m")
	old, oldToken := identityFile(t, c, "m", "/m/Heat.1995.WEBDL-720p.mkv", 720)
	film := attachMovie(t, c, "m", old, "/m/Heat.1995.WEBDL-720p.mkv", "Heat", 1995)
	public := c.Public(film)
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,rating,revision,watched,last_played_at,not_interested) VALUES('p',?,0,1,4.0,1,1,'2026-10-01T00:00:00Z',0)`, film)

	// The upgrade arrives under a new name, then the old file goes.
	better, _ := identityFile(t, c, "m", "/m/Heat.1995.Bluray-1080p.mkv", 1080)
	if again := attachMovie(t, c, "m", better, "/m/Heat.1995.Bluray-1080p.mkv", "Heat", 1995); again != film {
		t.Fatalf("the upgraded file became a new title: %d %d", again, film)
	}
	markMissing(t, c, "m", oldToken, time.Now().Add(-time.Hour))
	if n := rows(t, c, `SELECT count(*) FROM catalog_missing_titles WHERE entity_id=?`, film); n != 0 {
		t.Fatal("a title with a file left was marked missing")
	}
	s := New(c.DB)
	emptied, err := s.EmptyMissing(context.Background(), "", time.Now(), nil)
	if err != nil || emptied.Files != 1 || emptied.Titles != 0 {
		t.Fatalf("emptying the old file: %+v %v", emptied, err)
	}
	if c.Public(film) != public {
		t.Fatal("the title's id changed")
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_asset_links WHERE entity_id=?`, film); n != 1 {
		t.Fatalf("files left on the title: %d", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM personal_items WHERE profile_id='p' AND item_id=? AND favorite=1 AND rating=4.0`, film); n != 1 {
		t.Fatal("personal state did not stay with the title")
	}
}

func TestATitleWithNoFilesStaysMissingAndAFileBringsItBack(t *testing.T) {
	c := catalogtest.Open(t)
	c.Library("m", "Movies", "movie", "/m")
	a, token := identityFile(t, c, "m", "/m/Ran (1985).mkv", 1080)
	film := attachMovie(t, c, "m", a, "/m/Ran (1985).mkv", "Ran", 1985)
	public := c.Public(film)
	went := time.Now().Add(-2 * time.Hour)
	markMissing(t, c, "m", token, went)
	if n := rows(t, c, `SELECT count(*) FROM catalog_missing_titles WHERE entity_id=?`, film); n != 1 {
		t.Fatal("a title whose only file is gone is not marked missing")
	}
	s := New(c.DB)
	since, err := s.MissingSince(context.Background(), public)
	if err != nil || since == "" {
		t.Fatalf("missing since: %q %v", since, err)
	}
	page, err := s.MissingTitles(context.Background(), "m", "", 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != public || page.Rule != "after_days" || page.Days != 30 || page.Items[0].RemovedAt == "" {
		t.Fatalf("missing list: %+v %v", page, err)
	}
	// Under the shipped rule (thirty days) nothing is removed yet.
	if emptied, err := s.EmptyMissingDue(context.Background()); err != nil || emptied.Titles != 0 || emptied.Files != 0 {
		t.Fatalf("the default rule removed something early: %+v %v", emptied, err)
	}
	// A different file read as the same film re-attaches to the same title.
	b, _ := identityFile(t, c, "m", "/m/Ran.1985.Remastered.mkv", 2160)
	if again := attachMovie(t, c, "m", b, "/m/Ran.1985.Remastered.mkv", "Ran", 1985); again != film || c.Public(again) != public {
		t.Fatal("the replacement did not re-attach to the missing title")
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_missing_titles WHERE entity_id=?`, film); n != 0 {
		t.Fatal("a title with a file again is still marked missing")
	}
}

func TestRemovedTitleKeepsEachProfilesStateAndGetsItBack(t *testing.T) {
	c := catalogtest.Open(t)
	c.Library("m", "Movies", "movie", "/m")
	a, token := identityFile(t, c, "m", "/m/Stalker (1979).mkv", 1080)
	film := attachMovie(t, c, "m", a, "/m/Stalker (1979).mkv", "Stalker", 1979)
	c.Exec(`INSERT INTO catalog_external_ids(entity_id,provider,provider_kind,provider_id) VALUES(?,'tmdb','movie','1398')`, film)
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,rating,revision,watched,last_played_at,not_interested) VALUES('p',?,1,0,3.5,4,1,'2026-09-01T00:00:00Z',0)`, film)
	c.Exec(`INSERT INTO personal_watched_intents(profile_id,item_id,watched,authored_at) VALUES('p',?,1,'2026-09-01T00:00:00.000000000Z')`, film)
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES('q',?,600000,0,0,'play-1')`, film)
	c.Exec(`INSERT INTO personal_play_counts(profile_id,item_id,plays) VALUES('p',?,3)`, film)
	markMissing(t, c, "m", token, time.Now().Add(-time.Hour))
	s := New(c.DB)
	emptied, err := s.EmptyMissing(context.Background(), "", time.Now(), nil)
	if err != nil || emptied.Titles != 1 {
		t.Fatalf("emptying: %+v %v", emptied, err)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_entities WHERE id=?`, film); n != 0 {
		t.Fatal("the title was not removed")
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_identities`); n != 0 {
		t.Fatalf("the ledger still names a removed title: %d", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM retained_title_state`); n != 2 {
		t.Fatalf("kept state rows: %d", n)
	}
	// The same film turns up later in another library, under another name, and
	// is matched to the same provider identity.
	c.Library("k", "Classics", "movie", "/k")
	b, _ := identityFile(t, c, "k", "/k/Сталкер.mkv", 1080)
	back := attachMovie(t, c, "k", b, "/k/Сталкер.mkv", "Сталкер", 0)
	if n := rows(t, c, `SELECT count(*) FROM personal_items WHERE item_id=?`, back); n != 0 {
		t.Fatal("state came back before the title was identified")
	}
	c.Exec(`INSERT INTO catalog_external_ids(entity_id,provider,provider_kind,provider_id) VALUES(?,'tmdb','movie','1398')`, back)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		_, err := AdoptProviderIdentityTx(ctx, tx, back, compactcatalog.ProviderIdentity{Provider: "tmdb", Kind: "movie", ID: "1398"})
		return err
	})
	var watchlisted, watched bool
	var rating float64
	if err = c.DB.QueryRow(`SELECT watchlisted,watched,rating FROM personal_items WHERE profile_id='p' AND item_id=?`, back).Scan(&watchlisted, &watched, &rating); err != nil || !watchlisted || !watched || rating != 3.5 {
		t.Fatalf("state not given back: %v %v %v %v", watchlisted, watched, rating, err)
	}
	if n := rows(t, c, `SELECT count(*) FROM personal_watched_intents WHERE profile_id='p' AND item_id=? AND watched=1`, back); n != 1 {
		t.Fatal("watched mark not given back")
	}
	if n := rows(t, c, `SELECT position FROM progress WHERE profile_id='q' AND item_id=?`, back); n != 600000 {
		t.Fatalf("progress not given back: %d", n)
	}
	if n := rows(t, c, `SELECT plays FROM personal_play_counts WHERE profile_id='p' AND item_id=?`, back); n != 3 {
		t.Fatalf("plays not given back: %d", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM retained_title_state`); n != 0 {
		t.Fatalf("kept state was not cleared once given back: %d", n)
	}
}

func TestMergingTitlesCombinesFilesAndState(t *testing.T) {
	c := catalogtest.Open(t)
	c.Library("m", "Movies", "movie", "/m")
	a, _ := identityFile(t, c, "m", "/m/Alien.mkv", 1080)
	b, _ := identityFile(t, c, "m", "/m/Alien (1979).mkv", 2160)
	bare := attachMovie(t, c, "m", a, "/m/Alien.mkv", "Alien", 0)
	dated := attachMovie(t, c, "m", b, "/m/Alien (1979).mkv", "Alien", 1979)
	if bare == dated {
		t.Fatal("a film with no year and one with a year were merged by name alone")
	}
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,rating,revision,watched,last_played_at,not_interested) VALUES('p',?,0,1,NULL,1,1,'2026-01-01T00:00:00Z',0)`, bare)
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,rating,revision,watched,last_played_at,not_interested) VALUES('p',?,1,0,5.0,2,0,'2026-06-01T00:00:00Z',0)`, dated)
	c.Exec(`INSERT INTO personal_play_counts(profile_id,item_id,plays) VALUES('p',?,2)`, bare)
	c.Exec(`INSERT INTO personal_play_counts(profile_id,item_id,plays) VALUES('p',?,1)`, dated)
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES('p',?,1000,0,0,'old')`, bare)
	c.Exec(`INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) VALUES('p','m',?,'2026-01-01T00:00:00Z','paused')`, bare)
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES('p',?,9000,0,0,'new')`, dated)
	c.Exec(`INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) VALUES('p','m',?,'2026-06-01T00:00:00Z','paused')`, dated)
	// Both are matched to the same film: they are one title.
	for _, id := range []int64{bare, dated} {
		c.Exec(`INSERT INTO catalog_external_ids(entity_id,provider,provider_kind,provider_id) VALUES(?,'tmdb','movie','348')`, id)
	}
	var merged int
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		merged, err = AdoptProviderIdentityTx(ctx, tx, dated, compactcatalog.ProviderIdentity{Provider: "tmdb", Kind: "movie", ID: "348"})
		return err
	})
	if merged != 1 || rows(t, c, `SELECT count(*) FROM catalog_entities WHERE id=?`, bare) != 0 {
		t.Fatalf("merge: %d", merged)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_asset_links WHERE entity_id=?`, dated); n != 2 {
		t.Fatalf("versions after merge: %d", n)
	}
	var watchlisted, favorite, watched bool
	var rating float64
	var last string
	if err := c.DB.QueryRow(`SELECT watchlisted,favorite,watched,rating,last_played_at FROM personal_items WHERE profile_id='p' AND item_id=?`, dated).Scan(&watchlisted, &favorite, &watched, &rating, &last); err != nil || !watchlisted || !favorite || !watched || rating != 5 || last != "2026-06-01T00:00:00Z" {
		t.Fatalf("combined state: %v %v %v %v %q %v", watchlisted, favorite, watched, rating, last, err)
	}
	if n := rows(t, c, `SELECT plays FROM personal_play_counts WHERE profile_id='p' AND item_id=?`, dated); n != 3 {
		t.Fatalf("plays: %d", n)
	}
	if n := rows(t, c, `SELECT position FROM progress WHERE profile_id='p' AND item_id=?`, dated); n != 9000 {
		t.Fatalf("the later progress should win: %d", n)
	}
	// Either name finds the one title now.
	c2, _ := identityFile(t, c, "m", "/m/Alien extended.mkv", 720)
	if again := attachMovie(t, c, "m", c2, "/m/Alien extended.mkv", "Alien", 0); again != dated {
		t.Fatal("the absorbed title's name no longer finds the title")
	}
	c.Drain()
}

func episodeFile(t *testing.T, c *catalogtest.Catalog, s *Service, library, path, relative, kind string) {
	t.Helper()
	_, token := identityFile(t, c, library, path, 1080)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return s.commitEpisodes(tx, library, token, relative, ParseEpisode(relative, kind), false)
	})
}

func TestAShowIsItsFolderNotItsTitleAndYear(t *testing.T) {
	c := catalogtest.Open(t)
	c.Library("tv", "Shows", "tv", "/tv")
	c.Exec(`INSERT INTO library_sources(id,library_id,name,configured_root,root) VALUES('second','tv','More','/more','/more')`)
	s := New(c.DB)
	// One folder: its episodes are one show whatever year a file name carries.
	episodeFile(t, c, s, "tv", "/tv/Kingdom (2019)/Season 1/Kingdom (2019) - S01E01.mkv", "Kingdom (2019)/Season 1/Kingdom (2019) - S01E01.mkv", "tv")
	episodeFile(t, c, s, "tv", "/tv/Kingdom (2019)/Season 1/Kingdom (2020) - S01E02.mkv", "Kingdom (2019)/Season 1/Kingdom (2020) - S01E02.mkv", "tv")
	if n := rows(t, c, `SELECT count(*) FROM catalog_entities WHERE kind=2`); n != 1 {
		t.Fatalf("one folder became %d shows", n)
	}
	// A folder of the same name under another source is another show.
	episodeFile(t, c, s, "tv", "/more/Kingdom (2019)/Season 1/Kingdom (2019) - S01E01.mkv", "Kingdom (2019)/Season 1/Kingdom (2019) - S01E01.mkv", "tv")
	if n := rows(t, c, `SELECT count(*) FROM catalog_entities WHERE kind=2`); n != 2 {
		t.Fatalf("two folders in two sources are %d shows", n)
	}
	shows, err := scanInt64s(c.DB.Query(`SELECT id FROM catalog_entities WHERE kind=2 ORDER BY id`))
	if err != nil || len(shows) != 2 {
		t.Fatal(shows, err)
	}
	first, second := shows[0], shows[1]
	var duplicate int64
	if err = c.DB.QueryRow(`SELECT entity_id FROM catalog_episodes WHERE show_id=? AND number=1`, second).Scan(&duplicate); err != nil {
		t.Fatal(err)
	}
	c.Exec(`INSERT INTO personal_watched_intents(profile_id,item_id,watched,authored_at) VALUES('p',?,1,'2026-09-01T00:00:00.000000000Z')`, duplicate)
	// A provider says they are the same series: one show, with both files of
	// episode 1 as versions and the watched mark kept.
	for _, id := range shows {
		c.Exec(`INSERT INTO catalog_external_ids(entity_id,provider,provider_kind,provider_id) VALUES(?,'tvdb','series','355774')`, id)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		_, err := AdoptProviderIdentityTx(ctx, tx, first, compactcatalog.ProviderIdentity{Provider: "tvdb", Kind: "series", ID: "355774"})
		return err
	})
	if n := rows(t, c, `SELECT count(*) FROM catalog_entities WHERE kind=2`); n != 1 {
		t.Fatalf("shows after the provider match: %d", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_episodes WHERE show_id=?`, first); n != 2 {
		t.Fatalf("episodes after merge: %d", n)
	}
	var kept int64
	if err = c.DB.QueryRow(`SELECT entity_id FROM catalog_episodes WHERE show_id=? AND number=1`, first).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_asset_links WHERE entity_id=?`, kept); n != 2 {
		t.Fatalf("episode 1 versions: %d", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM personal_watched_intents WHERE profile_id='p' AND item_id=? AND watched=1`, kept); n != 1 {
		t.Fatal("the watched mark did not follow the episode")
	}
	// A later file in the merged-away folder still lands in the one show.
	episodeFile(t, c, s, "tv", "/more/Kingdom (2019)/Season 1/Kingdom (2019) - S01E03.mkv", "Kingdom (2019)/Season 1/Kingdom (2019) - S01E03.mkv", "tv")
	if n := rows(t, c, `SELECT count(*) FROM catalog_entities WHERE kind=2`); n != 1 {
		t.Fatal("a file in the merged folder made a new show")
	}
	c.Drain()
}

func TestAPlaceReusedByAnotherFileDoesNotJoinTheTitleThatMovedAway(t *testing.T) {
	c := catalogtest.Open(t)
	music := c.Library("mu", "Music", "music", "/mu")
	a, aToken := identityFile(t, c, "mu", "/mu/Inbox/track.mp3", 0)
	var first, second int64
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		first, err = AttachTrackTx(ctx, tx, TrackFile{Library: music, Kind: compactcatalog.Track, Asset: a, Path: "/mu/Inbox/track.mp3", Title: "track"})
		return err
	})
	// The file is moved into place; a different download lands at the old path.
	c.Exec(`UPDATE catalog_assets SET path='/mu/Artist/Album/01 Song.mp3' WHERE id=?`, a)
	c.Exec(`UPDATE inventory_objects SET relative_path='/mu/Artist/Album/01 Song.mp3' WHERE asset_id=?`, aToken)
	b, _ := identityFile(t, c, "mu", "/mu/Inbox/track.mp3", 0)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		second, err = AttachTrackTx(ctx, tx, TrackFile{Library: music, Kind: compactcatalog.Track, Asset: b, Path: "/mu/Inbox/track.mp3", Title: "track"})
		return err
	})
	if first == second {
		t.Fatal("a new file at a moved track's old path was attached to the moved track")
	}
	// Tags then say the second file is a track the library already has.
	tagged := compactcatalog.TrackKey("Artist", "Album", 1, 1, "Song")
	var survivor int64
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if _, err := IdentifyTrackTx(ctx, tx, first, tagged); err != nil {
			return err
		}
		var err error
		survivor, err = IdentifyTrackTx(ctx, tx, second, tagged)
		return err
	})
	if survivor != first || rows(t, c, `SELECT count(*) FROM catalog_asset_links WHERE entity_id=?`, first) != 2 {
		t.Fatalf("a second file with the same tags is not a version of the track: %d", survivor)
	}
}

func TestRemovingALibraryTakesItsTitlesAndKeepsState(t *testing.T) {
	c := catalogtest.Open(t)
	c.Library("m", "Movies", "movie", "/m")
	for i := 0; i < 30; i++ {
		path := fmt.Sprintf("/m/Film %02d (2001).mkv", i)
		a, _ := identityFile(t, c, "m", path, 1080)
		film := attachMovie(t, c, "m", a, path, fmt.Sprintf("Film %02d", i), 2001)
		if i == 0 {
			c.Exec(`INSERT INTO personal_watched_intents(profile_id,item_id,watched,authored_at) VALUES('p',?,1,'2026-09-01T00:00:00.000000000Z')`, film)
		}
	}
	c.Drain()
	s := New(c.DB)
	ctx := context.Background()
	allow := func(*sql.Tx) error { return nil }
	queued := ""
	enqueue := func(_ context.Context, _ *sql.Tx, kind, resource string) error {
		queued = kind + ":" + resource
		return nil
	}
	var revision int64
	if err := c.DB.QueryRow(`SELECT revision FROM library_configuration WHERE library_id='m'`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveLibrary(ctx, "m", "Wrong name", revision, allow, enqueue); err != ErrConfirmationMismatch {
		t.Fatalf("a wrong confirmation was accepted: %v", err)
	}
	if err := s.RemoveLibrary(ctx, "m", "Movies", revision, allow, enqueue); err != nil || queued != "library-removal:m" {
		t.Fatalf("remove: %v %q", err, queued)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_libraries WHERE library_id='m' AND retired=1`); n != 1 {
		t.Fatal("the library is not hidden at once")
	}
	libraries, err := s.Libraries()
	if err != nil || len(libraries) != 0 {
		t.Fatalf("a library being removed is still listed: %v %v", libraries, err)
	}
	for i := 0; ; i++ {
		done := false
		if err := dbwork.WithWriteTx(ctx, c.DB, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
			var e error
			done, e = removeLibraryBatchTx(ctx, tx, "m")
			return e
		}); err != nil {
			if err == sql.ErrNoRows {
				break
			}
			t.Fatal(err)
		}
		c.Drain()
		if done {
			break
		}
		if i > 200 {
			t.Fatal("removal did not finish")
		}
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_entities`); n != 0 {
		t.Fatalf("titles left: %d", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_assets`) + rows(t, c, `SELECT count(*) FROM inventory_objects`); n != 0 {
		t.Fatalf("file records left: %d", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM retained_title_state WHERE profile_id='p' AND watched=1`); n != 1 {
		t.Fatalf("watch state was not kept: %d", n)
	}
	// The same folder can be a library again, and the film gets its state back.
	c.Library("again", "Movies again", "movie", "/m")
	a, _ := identityFile(t, c, "again", "/m/Film 00 (2001).mkv", 1080)
	film := attachMovie(t, c, "again", a, "/m/Film 00 (2001).mkv", "Film 00", 2001)
	if n := rows(t, c, `SELECT count(*) FROM personal_watched_intents WHERE profile_id='p' AND item_id=? AND watched=1`, film); n != 1 {
		t.Fatal("the watched mark did not come back with the title")
	}
}
