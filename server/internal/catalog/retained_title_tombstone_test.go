package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/phasedpurge"
)

func TestRetainedTitleTombstoneCopiesPrecedenceAndPublishesOnlyOnDeletion(t *testing.T) {
	c := catalogtest.Open(t)
	root := t.TempDir()
	lib := c.Library("movies", "Movies", "movie", root)
	item := c.Movie(lib, filepath.Join(root, "one.mkv"), "One", 2026)
	owners := make([]string, 3)
	for i := range owners {
		owners[i], _, _ = titleWalkAddOwner(t, c, fmt.Sprintf("tombstone-account-%d", i), fmt.Sprintf("tombstone-profile-%d", i))
	}
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,favorite,revision,last_played_at) VALUES(?,?,1,1,'2026-01-01T00:00:00Z'),(?,?,1,1,'2026-01-03T00:00:00Z')`, owners[0], item.ID, owners[2], item.ID)
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,1000,0,0,'playing')`, owners[0], item.ID)
	c.Exec(`INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) VALUES(?,'movies',?,'2026-01-04T00:00:00Z','paused')`, owners[0], item.ID)
	for i := 0; i < 65; i++ {
		c.Exec(`INSERT INTO personal_history(id,profile_id,item_id,started_at,updated_at,sequence,position,unit,completed) VALUES(?,?,?,'start',?,1,10,0,0)`, fmt.Sprintf("history-%02d", i), owners[1], item.ID, fmt.Sprintf("history-stamp-%02d", i))
	}
	c.Exec(`INSERT INTO personal_history(id,profile_id,item_id,started_at,updated_at,sequence,position,unit,completed) VALUES('lower-priority',?,?,'start','2026-01-02T00:00:00Z',1,10,0,0)`, owners[0], item.ID)
	m := titleWalkMutation(t, c, item.ID, "tombstone-copy")
	titleWalkDrive(t, c, m, 400)

	s := New(c.DB)
	for _, owner := range owners {
		v := Viewer{Profile: owner, Fence: "current", Libraries: []string{"movies"}}
		if _, err := s.RemovedTitle(v, item.Public); !errors.Is(err, sql.ErrNoRows) {
			t.Fatal("prepared title was exposed before entity deletion", err)
		}
		source, err := s.homeRemovedSource(v)
		if err != nil {
			t.Fatal(err)
		}
		var count int
		if err = c.DB.QueryRow(`SELECT count(*) FROM (`+source.base+`)`, source.args...).Scan(&count); err != nil || count != 0 {
			t.Fatal("prepared title entered removed Continue", count, err)
		}
	}
	want := []struct {
		stamp      string
		continuing bool
	}{{"2026-01-04T00:00:00Z", true}, {"history-stamp-00", false}, {"2026-01-03T00:00:00Z", false}}
	for i, owner := range owners {
		var stamp string
		var continuing bool
		if err := c.DB.QueryRow(`SELECT activity_at,continuing FROM catalog_title_tombstone_viewers WHERE public_id=? AND profile_id=?`, item.Public, owner).Scan(&stamp, &continuing); err != nil || stamp != want[i].stamp || continuing != want[i].continuing {
			t.Fatal("viewer precedence changed", i, stamp, continuing, err)
		}
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM personal_history WHERE item_id=?`, item.ID); n != 0 {
		t.Fatal("history inputs were not drained", n)
	}

	// Storage publication boundary only: this small fixture drives the existing
	// final primitive, not a claim that the whole physical purge is qualified.
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := publishRetainedTitleTombstoneTx(ctx, tx, m); err != nil {
			return err
		}
		if _, err := compactcatalog.FinishEntityMutationTx(ctx, tx, m.Ref, "removed"); err != nil {
			return err
		}
		return compactcatalog.DeleteEntityTx(ctx, tx, item.ID)
	})
	for i, owner := range owners {
		out, err := s.RemovedTitle(Viewer{Profile: owner, Fence: "current", Libraries: []string{"movies"}}, item.Public)
		if err != nil || out.ID != item.Public || out.CanRemoveFromContinue != want[i].continuing {
			t.Fatal("completed deletion did not publish its exact viewer fact", i, out, err)
		}
	}
}

func TestRetainedTitleTombstoneAttributeUnitCountsRawRejectsAndKeepsPolicy(t *testing.T) {
	c := catalogtest.Open(t)
	root := t.TempDir()
	lib := c.Library("movies", "Movies", "movie", root)
	item := c.Movie(lib, filepath.Join(root, "restricted.mkv"), "Restricted", 2026)
	other := c.Movie(lib, filepath.Join(root, "other.mkv"), "Other", 2026)
	c.Attributes(item.ID, "contentRating", "R")
	c.Attributes(item.ID, "label", "adult")
	c.Attributes(item.ID, "genre", "Drama")
	c.Attributes(other.ID, "label", "unrelated")
	c.Exec(`INSERT INTO content_rating_ages(value_key,minimum_age) VALUES('r',17) ON CONFLICT(value_key) DO UPDATE SET minimum_age=17`)
	c.Drain()
	owner, _, _ := titleWalkAddOwner(t, c, "attribute-account", "attribute-profile")
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,favorite,revision) VALUES(?,?,1,1)`, owner, item.ID)
	m := titleWalkMutation(t, c, item.ID, "attribute-tombstone")
	titleWalkDrive(t, c, m, 100)
	before := titleWalkCount(t, c, `SELECT count(*) FROM catalog_item_attribute_edges WHERE item_id=?`, item.ID)
	otherBefore := titleWalkCount(t, c, `SELECT count(*) FROM catalog_item_attribute_edges WHERE item_id=?`, other.ID)
	plan := []phasedpurge.Unit{retainedTitleTombstoneAttributeUnit(m)}
	cursor := phasedpurge.Cursor{}
	changed, done := 0, false
	for turn := 0; turn < before+4; turn++ {
		left := titleWalkCount(t, c, `SELECT count(*) FROM catalog_item_attribute_edges WHERE item_id=?`, item.ID)
		var out phasedpurge.Result
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			var err error
			// This tests the literal compiled unit and its meter. The physical
			// registry owns durable cursor storage and source-worker fencing;
			// no production cursor or completion is forged here.
			out, err = phasedpurge.Turn(ctx, tx, plan, cursor, phasedpurge.Owner{ReadBound: 2,
				Save:     func(_ context.Context, _ *sql.Tx, next phasedpurge.Cursor) error { cursor = next; return nil },
				Finalize: func(context.Context, *sql.Tx) error { return nil },
			})
			return err
		})
		if out.Effects+1 > phasedpurge.EffectLimit {
			t.Fatal("attribute unit leaves no room for its caller's Save", out.Effects)
		}
		after := titleWalkCount(t, c, `SELECT count(*) FROM catalog_item_attribute_edges WHERE item_id=?`, item.ID)
		if left-after > 1 {
			t.Fatal("one turn deleted more than one raw attribute", left, after)
		}
		changed += left - after
		if out.Done {
			done = true
			break
		}
	}
	if !done || changed != before || before < 3 {
		t.Fatal("raw window was filtered before limiting", done, changed, before)
	}
	if got := titleWalkCount(t, c, `SELECT count(*) FROM catalog_title_tombstone_attributes WHERE public_id=?`, item.Public); got != 2 {
		t.Fatal("copied fields beyond the existing visibility set", got)
	}
	if got := titleWalkCount(t, c, `SELECT count(*) FROM catalog_item_attribute_edges WHERE item_id=?`, other.ID); got != otherBefore {
		t.Fatal("another title's attributes changed", got, otherBefore)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := publishRetainedTitleTombstoneTx(ctx, tx, m); err != nil {
			return err
		}
		if _, err := compactcatalog.FinishEntityMutationTx(ctx, tx, m.Ref, "removed"); err != nil {
			return err
		}
		return compactcatalog.DeleteEntityTx(ctx, tx, item.ID)
	})
	v := Viewer{Profile: owner, Fence: "current", Libraries: []string{"movies"}}
	s := New(c.DB)
	if _, err := s.RemovedTitle(v, item.Public); err != nil {
		t.Fatal("completed removed-title evidence missing", err)
	}
	age := 13
	v.Restrictions = identity.ContentRestrictions{MaximumAge: &age}
	if _, err := s.RemovedTitle(v, item.Public); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("copied age restriction was lost", err)
	}
	v.Restrictions = identity.ContentRestrictions{BlockedLabels: []string{"adult"}}
	if _, err := s.RemovedTitle(v, item.Public); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("copied label restriction was lost", err)
	}
}

func TestRetainedTitleTombstoneErasedOwnerIsNotReinsertedByDrain(t *testing.T) {
	c := catalogtest.Open(t)
	root := t.TempDir()
	lib := c.Library("movies", "Movies", "movie", root)
	item := c.Movie(lib, filepath.Join(root, "erased.mkv"), "Erased", 2026)
	const account, profile = "tombstone-erase-account", "tombstone-erase-profile"
	owner, lifetime, membership := titleWalkAddOwner(t, c, account, profile)
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,favorite,revision) VALUES(?,?,1,1)`, owner, item.ID)
	m := titleWalkMutation(t, c, item.ID, "erased-tombstone")
	published := false
	for turn := 0; turn < 60; turn++ {
		titleWalkStep(t, c, m)
		var arm int
		if err := c.DB.QueryRow(`SELECT capture_arm FROM catalog_purge_jobs WHERE mutation_id=?`, m.Ref.ID).Scan(&arm); err != nil {
			t.Fatal(err)
		}
		if arm == retainedCaptureArmPublished {
			published = true
			break
		}
	}
	if !published {
		t.Fatal("did not reach published retention before viewer drain")
	}
	svc, err := identity.New(c.DB, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Fixture the committed denial; the actual merged erasure engine performs
	// all physical removal and its catalog callback below.
	c.Exec(`UPDATE direct_profiles SET deleted=1,pin_hash=NULL,art='slate',allowed_libraries=NULL,position=0,revision=revision+1,pin_revision=pin_revision+1 WHERE account_id=? AND id=? AND lifetime=? AND deleted=0`, account, profile, lifetime)
	c.Exec(`INSERT INTO security_purge_jobs(job_id,authority,account_id,profile_id,account_incarnation,profile_incarnation,state) VALUES(?,'local',?,?,?,?,'pending')`, identity.Token()[:22], account, profile, membership, lifetime)
	if _, err = svc.PurgeProfileStep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM personal_items WHERE profile_id=? AND item_id=?`, owner, item.ID); n != 1 {
		t.Fatal("fixture passed the intended pre-source-erasure boundary", n)
	}
	titleWalkDrive(t, c, m, 100)
	if n := titleWalkCount(t, c, `SELECT count(*) FROM catalog_title_tombstone_viewers WHERE public_id=? AND profile_id=?`, item.Public, owner); n != 0 {
		t.Fatal("dead local binding copied a new removed-title viewer", n)
	}
	finished := false
	for turn := 0; turn < 400; turn++ {
		if _, err = svc.PurgeProfileStep(context.Background()); err != nil {
			t.Fatal(err)
		}
		if titleWalkCount(t, c, `SELECT count(*) FROM security_purge_jobs WHERE profile_id=?`, profile) == 0 {
			finished = true
			break
		}
	}
	if !finished {
		t.Fatal("actual profile erasure did not finish")
	}
	if done, _ := titleWalkStep(t, c, m); !done {
		t.Fatal("erasure callback head did not prove the erased inputs absent")
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE profile_id IN(?,?)`, owner, profile); n != 0 {
		t.Fatal("erasure left or republished retained owner state", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM catalog_title_tombstone_viewers WHERE profile_id IN(?,?)`, owner, profile); n != 0 {
		t.Fatal("erasure left or reinserted viewer evidence", n)
	}
}

func TestRetainedTitleTombstoneRebasesExpiredBuildInFinalWriterOnly(t *testing.T) {
	c := catalogtest.Open(t)
	root := t.TempDir()
	lib := c.Library("movies", "Movies", "movie", root)
	item := c.Movie(lib, filepath.Join(root, "paused.mkv"), "Paused", 2026)
	owner, _, _ := titleWalkAddOwner(t, c, "paused-account", "paused-profile")
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,favorite,revision) VALUES(?,?,1,1)`, owner, item.ID)
	m := titleWalkMutation(t, c, item.ID, "paused-tombstone")
	titleWalkDrive(t, c, m, 100)
	c.Exec(`UPDATE catalog_title_tombstones SET removed_at='2000-01-01T00:00:00Z',expires_at='2000-01-31T00:00:00Z' WHERE public_id=?`, item.Public)
	s := New(c.DB)
	if err := s.PruneRemovedTitles(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM catalog_title_tombstone_viewers WHERE public_id=?`, item.Public); n != 1 {
		t.Fatal("expiry pruned a live build's only viewer fact", n)
	}
	stop := errors.New("failed final delete")
	err := dbwork.WithWriteTx(context.Background(), c.DB, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		if err := publishRetainedTitleTombstoneTx(context.Background(), tx, m); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatal(err)
	}
	var stamp string
	if err = c.DB.QueryRow(`SELECT removed_at FROM catalog_title_tombstones WHERE public_id=?`, item.Public).Scan(&stamp); err != nil || stamp != "2000-01-01T00:00:00Z" {
		t.Fatal("failed deletion leaked the expiry rebase", stamp, err)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := publishRetainedTitleTombstoneTx(ctx, tx, m); err != nil {
			return err
		}
		if _, err := compactcatalog.FinishEntityMutationTx(ctx, tx, m.Ref, "removed"); err != nil {
			return err
		}
		return compactcatalog.DeleteEntityTx(ctx, tx, item.ID)
	})
	var expiry string
	if err = c.DB.QueryRow(`SELECT removed_at,expires_at FROM catalog_title_tombstones WHERE public_id=?`, item.Public).Scan(&stamp, &expiry); err != nil {
		t.Fatal(err)
	}
	removed, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil || time.Since(removed) > time.Minute {
		t.Fatal("publication retained the old removal time", stamp, err)
	}
	expires, err := time.Parse(time.RFC3339Nano, expiry)
	if err != nil || expires.Sub(removed) != 30*24*time.Hour {
		t.Fatal("publication did not receive the ordinary fresh lifetime", stamp, expiry, err)
	}

	legacy := c.Movie(lib, filepath.Join(root, "legacy.mkv"), "Legacy", 2026)
	old := "1999-12-01T00:00:00Z"
	c.Exec(`INSERT INTO catalog_title_tombstones(public_id,library_id,title,kind,poster_url,backdrop_url,recording,removed_at,expires_at) VALUES(?,'movies','Legacy',1,'','',0,?,'2099-01-01T00:00:00Z')`, legacy.Public, old)
	legacyMutation := titleWalkMutation(t, c, legacy.ID, "legacy-without-private-job")
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := publishRetainedTitleTombstoneTx(ctx, tx, legacyMutation); err != nil {
			return err
		}
		if _, err := compactcatalog.FinishEntityMutationTx(ctx, tx, legacyMutation.Ref, "removed"); err != nil {
			return err
		}
		return compactcatalog.DeleteEntityTx(ctx, tx, legacy.ID)
	})
	if err = c.DB.QueryRow(`SELECT removed_at FROM catalog_title_tombstones WHERE public_id=?`, legacy.Public).Scan(&stamp); err != nil || stamp != old {
		t.Fatal("legacy ON CONFLICT path was changed", stamp, err)
	}
}
