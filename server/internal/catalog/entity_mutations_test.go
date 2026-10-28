package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"portico.local/apikit/apierror"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/mbaccounting"
)

func drainFixture(t *testing.T, n int) (*catalogtest.Catalog, int64, []int64) {
	t.Helper()
	c := catalogtest.Open(t)
	library := c.Library("m", "Music", "music", "/m")
	var album int64
	var songs []int64
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		artist, _, e := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Artist, Key: compactcatalog.ArtistKey("band"), Title: "Band"})
		if e != nil {
			return e
		}
		if e = compactcatalog.SetFactsTx(ctx, tx, artist, map[string]any{"local_key": "band"}); e != nil {
			return e
		}
		album, _, e = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Album, Parent: artist, Key: compactcatalog.AlbumKey("album"), Title: "Album"})
		if e != nil {
			return e
		}
		if e = compactcatalog.SetFactsTx(ctx, tx, album, map[string]any{"artist_id": artist, "local_key": "album", "edition_key": emptyAudioEdition}); e != nil {
			return e
		}
		for i := 0; i < n; i++ {
			song, _, err := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Track, Parent: album, Key: fmt.Sprint("song-", i), Title: fmt.Sprint("Song ", i)})
			if err != nil {
				return err
			}
			if err = compactcatalog.SetFactsTx(ctx, tx, song, map[string]any{"album_id": album, "track_number": i + 1}); err != nil {
				return err
			}
			songs = append(songs, song)
		}
		return nil
	})
	return c, album, songs
}
func drainEvidence(t *testing.T, c *catalogtest.Catalog, release string, tracks int) {
	t.Helper()
	// Fixture construction is outside the measured mutation writers. All
	// installed accounting, sealed-child, FK and derived programs remain enabled.
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if _, e := tx.Exec(`INSERT INTO mb_release_evidence(revision_id,provider_id,digest,title,artist,release_group_id,group_title,primary_type,date,country,barcode,disambiguation,observed_at,payload) VALUES(?,?,printf('%064d',0),'','','','','','','','','','now','{}')`, release, release); e != nil {
			return e
		}
		if tracks > 0 {
			if _, e := tx.Exec(`INSERT INTO mb_release_media VALUES(?,1,'','')`, release); e != nil {
				return e
			}
		}
		for i := 0; i < tracks; i++ {
			r := fmt.Sprint(release, "-", i)
			if _, e := tx.Exec(`INSERT INTO mb_recording_evidence(revision_id,provider_id,digest,title,artist,disambiguation,observed_at,payload) VALUES(?,?,printf('%064d',0),'','','','now','{}')`, r, r); e != nil {
				return e
			}
			if _, e := tx.Exec(`UPDATE mb_recording_evidence SET sealed=1 WHERE revision_id=?`, r); e != nil {
				return e
			}
			if _, e := tx.Exec(`INSERT INTO mb_release_tracks(release_revision,track_id,medium_position,position,number,title,artist,recording_revision) VALUES(?,?,1,?,'','','',?)`, release, r, i+1, r); e != nil {
				return e
			}
		}
		_, e := tx.Exec(`UPDATE mb_release_evidence SET sealed=1 WHERE revision_id=?`, release)
		return e
	})
}
func drainPin(t *testing.T, c *catalogtest.Catalog, entity int64, release string, album bool) {
	t.Helper()
	c.PinMBRelease(entity, album, release)
}
func mutationTurn(t *testing.T, c *catalogtest.Catalog, ref compactcatalog.MutationRef) compactcatalog.MutationStep {
	t.Helper()
	var step compactcatalog.MutationStep
	err := dbwork.WithWriteTx(MutationWriteContext(context.Background()), c.DB, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		var e error
		step, e = CatalogMutationStepTx(context.Background(), tx, ref)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	assertDrainAccounting(t, c)
	return step
}

// This guard follows the approved private fact ceiling and raw quantum. It
// bounds fixture pumping, not any production writer, deadline or effect plan.
const accountingMutationTurns = 2*(20002/mbaccounting.FactsPerTurn+8) + 8

func nextMutationPin(t *testing.T, c *catalogtest.Catalog, ref compactcatalog.MutationRef) compactcatalog.MutationStep {
	t.Helper()
	for turn := 0; turn < accountingMutationTurns; turn++ {
		before := rows(t, c, `SELECT count(*) FROM mb_song_links`) + rows(t, c, `SELECT count(*) FROM mb_album_links`)
		step := mutationTurn(t, c, ref)
		after := rows(t, c, `SELECT count(*) FROM mb_song_links`) + rows(t, c, `SELECT count(*) FROM mb_album_links`)
		if step.PinDrained {
			if after != before-1 || step.Complete || !step.More {
				t.Fatalf("actual pin flip %+v, pins%d→%d", step, before, after)
			}
			return step
		}
		if before != after || step.Complete {
			t.Fatalf("private accounting changed pin/finished before flip: %+v, pins%d→%d", step, before, after)
		}
	}
	t.Fatal("accounting preparation did not reach its actual pin flip")
	return compactcatalog.MutationStep{}
}

func finishMutationPinFold(t *testing.T, c *catalogtest.Catalog, ref compactcatalog.MutationRef) {
	t.Helper()
	for turn := 0; turn < accountingMutationTurns; turn++ {
		var phase string
		if e := c.DB.QueryRow(`SELECT phase FROM catalog_entity_mutations WHERE mutation_id=? AND generation=?`, ref.ID, ref.Generation).Scan(&phase); e != nil {
			t.Fatal(e)
		}
		if phase == "finalize" {
			return
		}
		if step := mutationTurn(t, c, ref); step.Complete || step.PinDrained {
			t.Fatalf("fold/finalize checkpoint %+v", step)
		}
	}
	t.Fatal("accounting fold did not reach the separate finalize phase")
}

func readyMutationPin(t *testing.T, c *catalogtest.Catalog, ref compactcatalog.MutationRef) {
	t.Helper()
	for turn := 0; turn < accountingMutationTurns; turn++ {
		var ready bool
		if e := c.DB.QueryRow(`SELECT phase='ready' AND owner_operation=? FROM mb_accounting_state WHERE singleton=1`, ref.ID).Scan(&ready); e != nil {
			t.Fatal(e)
		}
		if ready {
			return
		}
		if step := mutationTurn(t, c, ref); step.Complete || step.PinDrained {
			t.Fatalf("effects before ready checkpoint %+v", step)
		}
	}
	t.Fatal("pin preparation never became ready")
}

func TestP252bOneActualPinAndSeparateFinalize(t *testing.T) {
	c, _, songs := drainFixture(t, 1)
	song := songs[0]
	drainEvidence(t, c, "one", 1)
	drainPin(t, c, song, "one", false)
	// Deliberately inconsistent kind/link combination still drains separately.
	drainPin(t, c, song, "one", true)
	var ref compactcatalog.MutationRef
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if e := RemoveTitleTx(ctx, tx, song); !errors.Is(e, compactcatalog.ErrEntityPinned) {
			return fmt.Errorf("complete-only helper: %v", e)
		}
		var e error
		ref, e = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: song, OwnerKind: "title", OwnerID: "test", OwnerGeneration: 1, Mode: "remove"})
		return e
	})
	if rows(t, c, `SELECT count(*) FROM mb_song_links`)+rows(t, c, `SELECT count(*) FROM mb_album_links`) != 2 {
		t.Fatal("admission drained pins")
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if e := compactcatalog.RequireEntityAcquirableTx(ctx, tx, song); !errors.Is(e, compactcatalog.ErrEntityMutating) {
			return fmt.Errorf("missing acquisition fence %v", e)
		}
		return nil
	})
	if step := mutationTurn(t, c, ref); step.PinDrained {
		t.Fatal("phase transition drained")
	}
	for want := 1; want >= 0; want-- {
		step := nextMutationPin(t, c, ref)
		if !step.PinDrained || step.Complete || !step.More {
			t.Fatalf("pin step %+v", step)
		}
		if n := rows(t, c, `SELECT count(*) FROM mb_song_links`) + rows(t, c, `SELECT count(*) FROM mb_album_links`); n != want {
			t.Fatalf("got %d pins, want %d", n, want)
		}
		if rows(t, c, `SELECT count(*) FROM catalog_entities WHERE id=?`, song) != 1 {
			t.Fatal("pin writer physically deleted")
		}
	}
	finishMutationPinFold(t, c, ref)
	if step := mutationTurn(t, c, ref); !step.Complete || step.Outcome != "removed" {
		t.Fatalf("finalize %+v", step)
	}
	if step := mutationTurn(t, c, ref); !step.Complete || step.Progressed {
		t.Fatalf("terminal replay %+v", step)
	}
}
func TestP252bMembersBeyondTwelveAndPinnedAlbumRetained(t *testing.T) {
	c, album, songs := drainFixture(t, 17)
	drainEvidence(t, c, "shared", 1)
	for _, song := range songs {
		drainPin(t, c, song, "shared", false)
	}
	drainPin(t, c, album, "shared", true)
	if e := New(c.DB).RemoveTitle(context.Background(), c.Public(album)); e != nil {
		t.Fatal(e)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_songs`); n != 0 {
		t.Fatalf("skipped %d children", n)
	}
	if rows(t, c, `SELECT count(*) FROM mb_album_links WHERE album_id=?`, album) != 1 || rows(t, c, `SELECT retired FROM catalog_entities WHERE id=?`, album) != 1 {
		t.Fatal("ordinary empty pinned album not retained")
	}
}
func TestP252bSourceChangeStalesAfterDrain(t *testing.T) {
	c, _, songs := drainFixture(t, 1)
	song := songs[0]
	drainEvidence(t, c, "r", 1)
	drainPin(t, c, song, "r", false)
	asset, _ := identityFile(t, c, "m", "/m/song.flac", 1)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.LinkAssetTx(ctx, tx, song, asset, compactcatalog.Link{})
	})
	var ref compactcatalog.MutationRef
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		ref, e = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: song, OwnerKind: "title", OwnerID: "changed", OwnerGeneration: 1, Mode: "remove"})
		return e
	})
	mutationTurn(t, c, ref)
	nextMutationPin(t, c, ref)
	c.Exec(`UPDATE catalog_assets SET modified_ns=modified_ns+1 WHERE id=?`, asset)
	step := mutationTurn(t, c, ref)
	if !step.Complete || step.Outcome != "stale" {
		t.Fatalf("source change %+v", step)
	}
	if rows(t, c, `SELECT count(*) FROM catalog_entities WHERE id=?`, song) != 1 {
		t.Fatal("stale command deleted title")
	}
}
func TestP252bOnePinCompleteWriterHold(t *testing.T) {
	c, _, songs := drainFixture(t, 1)
	drainEvidence(t, c, "large", 10000)
	drainPin(t, c, songs[0], "large", false)
	dbwork.WriteGate().ResetPeak()
	var ref compactcatalog.MutationRef
	start := time.Now()
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		ref, e = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: songs[0], OwnerKind: "title", OwnerID: "hold", OwnerGeneration: 1, Mode: "remove"})
		return e
	})
	t.Logf("intent writer elapsed %s", time.Since(start))
	for i := 0; i < accountingMutationTurns; i++ {
		start = time.Now()
		step := mutationTurn(t, c, ref)
		elapsed := time.Since(start)
		if i < 2 || step.PinDrained || step.Complete {
			t.Logf("turn %d pin=%t terminal=%t elapsed=%s peak=%dms", i, step.PinDrained, step.Complete, elapsed, dbwork.WriteGate().Stats().MaxHeldMilli)
		}
		if dbwork.WriteGate().Stats().MaxHeldMilli > 250 {
			t.Fatalf("complete mutation writer exceeded 250ms: %s", elapsed)
		}
		if step.Complete {
			return
		}
	}
	t.Fatal("did not finalize")
}

func TestP252bRecursiveMergeAndAdditiveStateOnce(t *testing.T) {
	c, album, songs := drainFixture(t, 1)
	var library, artist int64
	c.DB.QueryRow(`SELECT library_id,parent_id FROM catalog_entities WHERE id=?`, album).Scan(&library, &artist)
	var otherArtist, otherAlbum, otherSong int64
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		otherArtist, _, e = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Artist, Key: "other-artist", Title: "Band"})
		if e != nil {
			return e
		}
		if e = compactcatalog.SetFactsTx(ctx, tx, otherArtist, map[string]any{"local_key": "other"}); e != nil {
			return e
		}
		otherAlbum, _, e = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Album, Parent: otherArtist, Key: "other-album", Title: "Album"})
		if e != nil {
			return e
		}
		if e = compactcatalog.SetFactsTx(ctx, tx, otherAlbum, map[string]any{"artist_id": otherArtist, "local_key": "other", "edition_key": emptyAudioEdition}); e != nil {
			return e
		}
		otherSong, _, e = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Track, Parent: otherAlbum, Key: "other-song", Title: "Song 0"})
		if e != nil {
			return e
		}
		return compactcatalog.SetFactsTx(ctx, tx, otherSong, map[string]any{"album_id": otherAlbum, "track_number": 1})
	})
	drainEvidence(t, c, "recursive", 1)
	drainPin(t, c, otherSong, "recursive", false)
	drainPin(t, c, otherAlbum, "recursive", true)
	c.Exec(`INSERT INTO personal_play_counts(profile_id,item_id,plays) VALUES('p',?,3),('p',?,7)`, songs[0], otherSong)
	from, into := c.Public(otherArtist), c.Public(artist)
	if e := New(c.DB).MergeTitles(context.Background(), from, into, func(*sql.Tx) error { return nil }); e != nil {
		t.Fatal(e)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_entities WHERE id IN(?,?,?)`, otherArtist, otherAlbum, otherSong); n != 0 {
		t.Fatalf("left %d recursive sources", n)
	}
	if plays := rows(t, c, `SELECT plays FROM personal_play_counts WHERE item_id=? AND profile_id='p'`, songs[0]); plays != 10 {
		t.Fatalf("additive plays=%d", plays)
	}
	var ref compactcatalog.MutationRef
	c.DB.QueryRow(`SELECT mutation_id,generation FROM catalog_entity_mutations WHERE owner_kind='merge' AND parent_mutation_id IS NULL`).Scan(&ref.ID, &ref.Generation)
	if step := mutationTurn(t, c, ref); !step.Complete {
		t.Fatal(step)
	}
	if plays := rows(t, c, `SELECT plays FROM personal_play_counts WHERE item_id=? AND profile_id='p'`, songs[0]); plays != 10 {
		t.Fatalf("replay added plays=%d", plays)
	}
}
func TestP252bAdoptionBeyondSixteenResumes(t *testing.T) {
	c, album, _ := drainFixture(t, 0)
	var library, artist int64
	c.DB.QueryRow(`SELECT library_id,parent_id FROM catalog_entities WHERE id=?`, album).Scan(&library, &artist)
	p := compactcatalog.ProviderIdentity{Provider: "musicbrainz", Kind: "release", ID: "common"}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if e := compactcatalog.RecordProviderIdentityTx(ctx, tx, album, p); e != nil {
			return e
		}
		for i := 0; i < 19; i++ {
			id, _, e := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Album, Parent: artist, Key: fmt.Sprint("duplicate-", i), Title: fmt.Sprint("Duplicate ", i)})
			if e != nil {
				return e
			}
			if e = compactcatalog.SetFactsTx(ctx, tx, id, map[string]any{"artist_id": artist, "local_key": fmt.Sprint(i)}); e != nil {
				return e
			}
			if e = compactcatalog.RecordProviderIdentityTx(ctx, tx, id, p); e != nil {
				return e
			}
		}
		return enqueueIdentityAdoptionTx(ctx, tx, album, p)
	})
	var root string
	c.DB.QueryRow(`SELECT mutation_id FROM catalog_entity_mutations WHERE mode='adopt'`).Scan(&root)
	for i := 0; i < 220; i++ {
		// A fresh service/adapter per turn simulates losing all process-local work.
		out, e := New(c.DB).MutationAdapter().Step(context.Background(), root)
		if e != nil {
			t.Fatal(e)
		}
		if out.State == "succeeded" {
			if n := rows(t, c, `SELECT count(*) FROM catalog_albums`); n != 1 {
				t.Fatalf("adoption skipped %d albums", n-1)
			}
			return
		}
	}
	t.Fatal("adoption never exhausted")
}
func TestP252bLibraryAllPlayableAndSixtyFourContainers(t *testing.T) {
	c, album, songs := drainFixture(t, 5)
	drainEvidence(t, c, "library", 1)
	for _, song := range songs {
		drainPin(t, c, song, "library", false)
	}
	drainPin(t, c, album, "library", true)
	var library, artist int64
	c.DB.QueryRow(`SELECT library_id,parent_id FROM catalog_entities WHERE id=?`, album).Scan(&library, &artist)
	var ids []int64
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		for i := 0; i < 66; i++ {
			id, _, e := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Album, Parent: artist, Key: fmt.Sprint("empty-", i), Title: "Empty"})
			if e != nil {
				return e
			}
			if e = compactcatalog.SetFactsTx(ctx, tx, id, map[string]any{"artist_id": artist, "local_key": fmt.Sprint(i)}); e != nil {
				return e
			}
			ids = append(ids, id)
		}
		return nil
	})
	for _, id := range ids {
		drainPin(t, c, id, "library", true)
	}
	c.Exec(`UPDATE catalog_libraries SET retired=1 WHERE library_id='m'`)
	for i := 0; i < 500; i++ {
		done := false
		e := dbwork.WithWriteTx(MutationWriteContext(context.Background()), c.DB, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
			var e error
			done, e = removeLibraryBatchTx(context.Background(), tx, "m")
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		if done {
			if n := rows(t, c, `SELECT count(*) FROM catalog_entities WHERE library_id=?`, library); n != 0 {
				t.Fatalf("skipped %d library entities", n)
			}
			return
		}
	}
	t.Fatal("library never completed")
}
func TestP252bAdministrationMembersBeyondForty(t *testing.T) {
	c, album, songs := drainFixture(t, 43)
	drainEvidence(t, c, "admin", 1)
	var plans []TitleDeletePlan
	for _, song := range songs {
		drainPin(t, c, song, "admin", false)
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			p, e := CaptureTitleDeleteTx(ctx, tx, song)
			plans = append(plans, p)
			return e
		})
	}
	for _, plan := range plans {
		for i := 0; i < 8; i++ {
			var step compactcatalog.MutationStep
			c.Write(func(ctx context.Context, tx *sql.Tx) error {
				var e error
				step, e = DeleteApprovedTitleStepTx(ctx, tx, plan, "approved-request-digest")
				return e
			})
			if step.Complete {
				if step.Outcome != "removed" {
					t.Fatal(step)
				}
				break
			}
			if i == 7 {
				t.Fatal("pending admin member skipped")
			}
		}
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_songs WHERE album_id=?`, album); n != 0 {
		t.Fatalf("skipped %d administration members", n)
	}
}
func TestP252bMissingBeyondTwentyFourAndAuthorityRevoke(t *testing.T) {
	c, _, songs := drainFixture(t, 26)
	drainEvidence(t, c, "missing", 1)
	for i, song := range songs {
		drainPin(t, c, song, "missing", false)
		asset, token := identityFile(t, c, "m", fmt.Sprintf("/m/missing-%d.flac", i), 1)
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			return compactcatalog.LinkAssetTx(ctx, tx, song, asset, compactcatalog.Link{})
		})
		markMissing(t, c, "m", token, time.Now().Add(-time.Hour))
	}
	s := New(c.DB)
	cutoff := time.Now()
	out, e := s.EmptyMissing(context.Background(), "m", cutoff, func(*sql.Tx) error { return nil })
	if e != nil || out.Titles != 0 || !out.More || !out.Advanced {
		t.Fatal(out, e)
	}
	denied := errors.New("revoked")
	if _, e = s.EmptyMissing(context.Background(), "m", cutoff, func(*sql.Tx) error { return denied }); !errors.Is(e, denied) {
		t.Fatal(e)
	}
	if n := rows(t, c, `SELECT count(*) FROM mb_song_links`); n != 26 {
		t.Fatal("revocation drained", n)
	}
	total := 0
	for i := 0; i < 150; i++ {
		out, e = s.EmptyMissing(context.Background(), "m", cutoff, func(*sql.Tx) error { return nil })
		if e != nil {
			t.Fatal(e)
		}
		total += out.Titles
		if !out.More {
			if total != 26 {
				t.Fatalf("removed %d titles", total)
			}
			return
		}
	}
	t.Fatal("missing never finished")
}

const drainAccountingOracleSQL = `WITH unaccepted_releases AS (
 SELECT r.revision_id,length(CAST(r.payload AS BLOB))*8+512*(1+
 (SELECT count(*) FROM mb_release_credits c WHERE c.revision_id=r.revision_id)+
 (SELECT count(*) FROM mb_release_group_types c WHERE c.release_revision=r.revision_id)+
 (SELECT count(*) FROM mb_release_media c WHERE c.release_revision=r.revision_id)+
 (SELECT count(*) FROM mb_release_tracks c WHERE c.release_revision=r.revision_id)+
 (SELECT count(*) FROM mb_track_credits c WHERE c.release_revision=r.revision_id)) AS charge
 FROM mb_release_evidence r WHERE NOT EXISTS(SELECT 1 FROM mb_album_links l WHERE l.release_revision=r.revision_id) AND NOT EXISTS(SELECT 1 FROM mb_song_links l WHERE l.release_revision=r.revision_id)
 ),unaccepted_recordings AS (
 SELECT r.revision_id,length(CAST(r.payload AS BLOB))*8+512*(1+(SELECT count(*) FROM mb_recording_credits c WHERE c.revision_id=r.revision_id)) AS charge
 FROM mb_recording_evidence r WHERE NOT EXISTS(SELECT 1 FROM mb_song_links l WHERE l.recording_revision=r.revision_id)
 AND NOT EXISTS(SELECT 1 FROM mb_release_tracks t WHERE t.recording_revision=r.revision_id AND (EXISTS(SELECT 1 FROM mb_album_links l WHERE l.release_revision=t.release_revision) OR EXISTS(SELECT 1 FROM mb_song_links l WHERE l.release_revision=t.release_revision)))
 ) SELECT (SELECT count(*) FROM unaccepted_releases),(SELECT count(*) FROM unaccepted_recordings),COALESCE((SELECT sum(charge) FROM unaccepted_releases),0)+COALESCE((SELECT sum(charge) FROM unaccepted_recordings),0)`

func assertDrainAccounting(t *testing.T, c *catalogtest.Catalog) {
	t.Helper()
	var got, want [3]int64
	if e := c.DB.QueryRow(`SELECT releases,recordings,charge FROM mb_evidence_totals WHERE singleton=1`).Scan(&got[0], &got[1], &got[2]); e != nil {
		t.Fatal(e)
	}
	if e := c.DB.QueryRow(drainAccountingOracleSQL).Scan(&want[0], &want[1], &want[2]); e != nil {
		t.Fatal(e)
	}
	if got != want {
		t.Fatalf("accounting got %v want original aggregate %v", got, want)
	}
}

func TestP252bRollbackAndLifetimeCustody(t *testing.T) {
	c, _, songs := drainFixture(t, 1)
	song := songs[0]
	drainEvidence(t, c, "rollback", 1)
	drainPin(t, c, song, "rollback", false)
	var ref compactcatalog.MutationRef
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		ref, e = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: song, OwnerKind: "administration", OwnerID: "command-digest", OwnerGeneration: 1, Mode: "remove"})
		return e
	})
	out, e := New(c.DB).MutationAdapter().Step(context.Background(), ref.ID)
	if e != nil || out.State != "paused" {
		t.Fatal(out, e)
	}
	if n := rows(t, c, `SELECT count(*) FROM mb_song_links WHERE item_id=?`, song); n != 1 {
		t.Fatal("background worker used owner pointer as management authority")
	}
	mutationTurn(t, c, ref)
	readyMutationPin(t, c, ref)
	rollback := errors.New("cancel after quantum")
	e = dbwork.WithWriteTx(MutationWriteContext(context.Background()), c.DB, dbwork.ClassInteractive, func(tx *sql.Tx) error {
		step, e := RemoveTitleStepTx(context.Background(), tx, ref)
		if e != nil {
			return e
		}
		if !step.PinDrained {
			return fmt.Errorf("expected pin: %+v", step)
		}
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal(e)
	}
	if n := rows(t, c, `SELECT count(*) FROM mb_song_links WHERE item_id=?`, song); n != 1 {
		t.Fatal("rollback committed drain")
	}
	if step := nextMutationPin(t, c, ref); !step.PinDrained {
		t.Fatal(step)
	}
	// Both links are now absent, but raw physical deletion still cannot break
	// the held lifetime and recreate the original integer/public identity.
	if _, e = c.DB.Exec(`DELETE FROM catalog_entities WHERE id=?`, song); e == nil {
		t.Fatal("restrictive custody FK bypassed")
	}
	finishMutationPinFold(t, c, ref)
	if step := mutationTurn(t, c, ref); !step.Complete {
		t.Fatal(step)
	}
}
func TestP252bTargetSourceChangeStalesMerge(t *testing.T) {
	c, album, songs := drainFixture(t, 1)
	source := songs[0]
	var library int64
	c.DB.QueryRow(`SELECT library_id FROM catalog_entities WHERE id=?`, album).Scan(&library)
	var target int64
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		target, _, e = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Track, Parent: album, Key: "target", Title: "Target"})
		if e != nil {
			return e
		}
		return compactcatalog.SetFactsTx(ctx, tx, target, map[string]any{"album_id": album})
	})
	asset, _ := identityFile(t, c, "m", "/m/target.flac", 1)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.LinkAssetTx(ctx, tx, target, asset, compactcatalog.Link{})
	})
	var ref compactcatalog.MutationRef
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		ref, e = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: source, Target: target, OwnerKind: "merge", OwnerID: "owner-command", OwnerGeneration: 1, Mode: "merge"})
		return e
	})
	c.Exec(`UPDATE catalog_assets SET modified_ns=modified_ns+1 WHERE id=?`, asset)
	if step := mutationTurn(t, c, ref); !step.Complete || step.Outcome != "stale" {
		t.Fatal(step)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_entities WHERE id IN(?,?)`, source, target); n != 2 {
		t.Fatal("changed target/source absorbed", n)
	}
}

func TestP252bOverlapProbesIgnoreHistoryAndUnrelatedActivePrefix(t *testing.T) {
	c, album, songs := drainFixture(t, 1)
	measure := func(owner string) int64 {
		dbwork.ResetPlans()
		e := dbwork.WithWriteTx(context.Background(), c.DB, dbwork.ClassInteractive, func(tx *sql.Tx) error {
			ref, e := compactcatalog.BeginEntityMutationTx(context.Background(), tx, compactcatalog.MutationIntent{Entity: album, OwnerKind: "title", OwnerID: owner, OwnerGeneration: 1, Mode: "members"})
			if e != nil {
				return e
			}
			_, e = compactcatalog.FinishEntityMutationTx(context.Background(), tx, ref, "stale")
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		return dbwork.Plans().Largest.VMOps
	}
	before := measure("before-history")
	var library int64
	c.DB.QueryRow(`SELECT library_id FROM catalog_entities WHERE id=?`, album).Scan(&library)
	c.Exec(`WITH RECURSIVE n(x) AS(SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<5000)
 INSERT INTO catalog_entity_mutations(mutation_id,generation,family_id,owner_kind,owner_id,owner_generation,subject_id,public_id,entity_kind,library_id,entity_incarnation,publication_incarnation,source_digest,mode,phase,status,outcome)
 SELECT 'history-'||x,1,'history-'||x,'title','history-'||x,1,?1,'history',6,?2,'','','fixture','remove','terminal','terminal','stale' FROM n`, album, library)
	c.Exec(`WITH RECURSIVE n(x) AS(SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<1200)
 INSERT INTO catalog_entities(public_id,library_id,kind,title,sort_key,sort_head) SELECT randomblob(16),?,5,'Unrelated','unrelated','u' FROM n`, library)
	c.Exec(`INSERT INTO catalog_entity_mutations(mutation_id,generation,family_id,owner_kind,owner_id,owner_generation,entity_id,subject_id,public_id,entity_kind,library_id,entity_incarnation,publication_incarnation,source_digest,mode,phase,status,outcome)
 SELECT 'active-'||id,1,'active-'||id,'title','active-'||id,1,id,id,pid(public_id),kind,library_id,'','','fixture','remove','admitted','active','pending' FROM catalog_entities WHERE title='Unrelated'`)
	after := measure("after-history")
	t.Logf("admission VM before=%d after5000terminal+1200unrelatedactive=%d", before, after)
	if after > before*2+2000 || after > 50_000 {
		t.Fatalf("admission scanned unrelated work: before=%d after=%d", before, after)
	}
	var child compactcatalog.MutationRef
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		child, e = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: songs[0], OwnerKind: "title", OwnerID: "real-child", OwnerGeneration: 1, Mode: "remove"})
		return e
	})
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		_, e := compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: album, OwnerKind: "title", OwnerID: "competing-ancestor", OwnerGeneration: 1, Mode: "members"})
		var public *apierror.Error
		if !errors.Is(e, compactcatalog.ErrEntityMutating) || !errors.As(e, &public) || public.Code != "request_in_progress" {
			return fmt.Errorf("overlap not typed conflict: %v", e)
		}
		_, e = compactcatalog.FinishEntityMutationTx(ctx, tx, child, "stale")
		return e
	})
}

func TestP252bAudioIdentityCollisionAdmitsAtPublicationTail(t *testing.T) {
	c, album, songs := drainFixture(t, 1)
	source := songs[0]
	var library int64
	c.DB.QueryRow(`SELECT library_id FROM catalog_entities WHERE id=?`, album).Scan(&library)
	var target int64
	key := compactcatalog.TrackKey("Band", "Album", 1, 1, "Song")
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		target, _, e = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Track, Parent: album, Key: key, Title: "Song"})
		if e != nil {
			return e
		}
		return compactcatalog.SetFactsTx(ctx, tx, target, map[string]any{"album_id": album, "track_number": 1})
	})
	asset, _ := identityFile(t, c, "m", "/m/source.flac", 1)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.LinkAssetTx(ctx, tx, source, asset, compactcatalog.Link{})
	})
	drainEvidence(t, c, "audio", 1)
	drainPin(t, c, source, "audio", false)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		ctx, pending := withAudioMergeAdmissions(ctx)
		returned, e := IdentifyTrackTx(ctx, tx, source, key)
		if e != nil {
			return e
		}
		if returned != source || len(pending.Pairs) != 1 {
			return fmt.Errorf("premature survivor: %d pairs=%d", returned, len(pending.Pairs))
		}
		var n int
		if e = tx.QueryRow(`SELECT count(*) FROM catalog_entity_mutations`).Scan(&n); e != nil {
			return e
		}
		if n != 0 {
			return errors.New("intent captured midway through source publication")
		}
		if e = compactcatalog.SetFieldsTx(ctx, tx, source, compactcatalog.Automatic, map[string]any{"title": "Final source title"}); e != nil {
			return e
		}
		if e = compactcatalog.SetAssetTx(ctx, tx, asset, map[string]any{"modified_ns": int64(999)}); e != nil {
			return e
		}
		return finishAudioMergeAdmissionsTx(ctx, tx, pending)
	})
	var root string
	c.DB.QueryRow(`SELECT mutation_id FROM catalog_entity_mutations WHERE owner_kind='identity' AND mode='merge'`).Scan(&root)
	for i := 0; i < 12; i++ {
		out, e := New(c.DB).MutationAdapter().Step(context.Background(), root)
		if e != nil {
			t.Fatal(e)
		}
		if out.State == "succeeded" {
			if n := rows(t, c, `SELECT count(*) FROM catalog_asset_links WHERE entity_id=? AND asset_id=?`, target, asset); n != 1 {
				t.Fatal("source file did not follow completed merge")
			}
			return
		}
	}
	t.Fatal("deferred local audio merge never finished")
}
func TestP252bManualMergeRequiresFreshAuthorityAfterDrain(t *testing.T) {
	c, album, songs := drainFixture(t, 1)
	source := songs[0]
	var library int64
	c.DB.QueryRow(`SELECT library_id FROM catalog_entities WHERE id=?`, album).Scan(&library)
	var target int64
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		target, _, e = compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Track, Parent: album, Key: "manual-target", Title: "Target"})
		if e != nil {
			return e
		}
		return compactcatalog.SetFactsTx(ctx, tx, target, map[string]any{"album_id": album})
	})
	drainEvidence(t, c, "manual", 1)
	drainPin(t, c, source, "manual", false)
	from, into := c.Public(source), c.Public(target)
	revoked := errors.New("authority revoked")
	calls := 0
	e := New(c.DB).MergeTitles(context.Background(), from, into, func(*sql.Tx) error {
		calls++
		if calls >= 5 {
			return revoked
		}
		return nil
	})
	if !errors.Is(e, revoked) || rows(t, c, `SELECT count(*) FROM mb_song_links WHERE item_id=?`, source) != 0 || rows(t, c, `SELECT count(*) FROM catalog_entities WHERE id=?`, source) != 1 {
		t.Fatal("revocation after drain", calls, e)
	}
	var root string
	c.DB.QueryRow(`SELECT mutation_id FROM catalog_entity_mutations WHERE owner_kind='merge' AND parent_mutation_id IS NULL`).Scan(&root)
	out, e := New(c.DB).MutationAdapter().Step(context.Background(), root)
	if e != nil || out.State != "paused" {
		t.Fatal("background elevated manual pointer", out, e)
	}
	if e = New(c.DB).MergeTitles(context.Background(), from, into, func(*sql.Tx) error { return nil }); e != nil {
		t.Fatal("fresh authorized resume", e)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_entities WHERE id=?`, source); n != 0 {
		t.Fatal("authorized resume did not finalize")
	}
}

func TestP252bStaleChildCancelsRootWithoutDroppingOtherMembers(t *testing.T) {
	c, album, songs := drainFixture(t, 2)
	drainEvidence(t, c, "stale-child", 1)
	for _, song := range songs {
		drainPin(t, c, song, "stale-child", false)
	}
	asset, _ := identityFile(t, c, "m", "/m/stale-child.flac", 1)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.LinkAssetTx(ctx, tx, songs[0], asset, compactcatalog.Link{})
	})
	var root compactcatalog.MutationRef
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		root, e = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{Entity: album, OwnerKind: "title", OwnerID: "stale-root", OwnerGeneration: 1, Mode: "members"})
		return e
	})
	drained := false
	for i := 0; i < 8; i++ {
		step := mutationTurn(t, c, root)
		if step.Complete {
			t.Fatal("root completed before its child", step)
		}
		if step.PinDrained {
			drained = true
			break
		}
	}
	if !drained {
		t.Fatal("first child not drained")
	}
	c.Exec(`UPDATE catalog_assets SET modified_ns=modified_ns+1 WHERE id=?`, asset)
	for i := 0; i < 8; i++ {
		step := mutationTurn(t, c, root)
		if step.Complete {
			if step.Outcome != "stale" {
				t.Fatal(step)
			}
			if n := rows(t, c, `SELECT count(*) FROM catalog_entity_mutations WHERE family_id=? AND status<>'terminal'`, root.ID); n != 0 {
				t.Fatalf("dropped %d live child intents", n)
			}
			if n := rows(t, c, `SELECT count(*) FROM catalog_songs WHERE album_id=?`, album); n != 2 {
				t.Fatalf("stale root deleted %d children", 2-n)
			}
			if n := rows(t, c, `SELECT count(*) FROM mb_song_links WHERE item_id=?`, songs[1]); n != 1 {
				t.Fatal("stale root adopted next child")
			}
			return
		}
	}
	t.Fatal("stale parent kept reselecting its terminal child")
}
