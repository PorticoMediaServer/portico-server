package catalog

// P2-59 / O-022 retained-title walk lifetime tests.
//
// These tests drive the actual parent step (captureRetainedTitleStepTx) and the
// actual capture component against the real installed baseline. They add no
// schema, counter or progress phase. Fixture rows are real domain inputs:
// admitted mutation roots, catalogue entities and files created through the
// production writers, personal-state and Reading rows in the same exact tables
// the component reads, and old published retained headers built the same way a
// previous publication builds them.
//
// What is deliberately NOT faked:
//   - No capture_arm, current_state_id, identity_after, reading cursor or
//     catalog_purge_jobs plan/cursor value is invented. Every such value is
//     written by the compiled walk or the real capture component.
//   - Test 2 corrupts an already written plan/cursor to prove the refusal, then
//     puts back the exact value the component itself wrote.
//   - Test 3 drives the merged P2-62 profile-erasure engine
//     (identity.Service.PurgeProfileStep) to the approved phase-56
//     catalog-binding reset. The test only seeds the already-committed denial a
//     retirement writes (direct_profiles.deleted=1 plus the security_purge_jobs
//     row); it never runs the reset SQL itself.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/phasedpurge"
)

// titleWalkAttempt runs one real step and reports how many row effects the step
// attempted before its transaction rolled back. It never calls
// EntityMutationSourceDigestTx; the component's own point recheck is the guard.
func titleWalkAttempt(t *testing.T, c *catalogtest.Catalog, m compactcatalog.EntityMutation) (bool, int64, error) {
	t.Helper()
	var done bool
	var effects int64
	err := dbwork.WithWriteTx(context.Background(), c.DB, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		var before int64
		if err := tx.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
			return err
		}
		d, stepErr := captureRetainedTitleStepTx(context.Background(), tx, m)
		var after int64
		if err := tx.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil {
			return err
		}
		effects = after - before
		if stepErr != nil {
			return stepErr
		}
		done = d
		return nil
	})
	return done, effects, err
}

// titleWalkJobSnapshot is a comparable image of every durable scalar the parent
// or the capture component can write. A refusal must not change one byte of it.
func titleWalkJobSnapshot(t *testing.T, c *catalogtest.Catalog, mutation string) string {
	t.Helper()
	var (
		generation, attempt, nextAttempt                  int64
		sourceAfter, readingAssetAfter, readingStateAfter int64
		plan, cursor, state, reason                       string
		ownerAfter, ownerValue                            string
		readingArm, readingSource                         string
		localAccount, localProfile                        string
		localMembership, localLifetime                    string
		ownerStarted, arm, readingStarted, identityAfter  int
		current                                           sql.NullInt64
	)
	err := c.DB.QueryRow(`SELECT generation,plan_digest,cursor,state,reason,owner_started,owner_after,owner_value,
 capture_arm,current_state_id,reading_arm,reading_started,reading_asset_after,reading_state_after,reading_source_after,
 identity_after,source_after,attempt,next_attempt_ms,local_account,local_profile,local_membership_incarnation,local_profile_lifetime
 FROM catalog_purge_jobs WHERE mutation_id=?`, mutation).Scan(
		&generation, &plan, &cursor, &state, &reason, &ownerStarted, &ownerAfter, &ownerValue,
		&arm, &current, &readingArm, &readingStarted, &readingAssetAfter, &readingStateAfter, &readingSource,
		&identityAfter, &sourceAfter, &attempt, &nextAttempt,
		&localAccount, &localProfile, &localMembership, &localLifetime)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("gen=%d plan=%s cursor=%q state=%q reason=%q started=%d after=%q value=%q arm=%d current=%v "+
		"reading=%q/%d/%d/%d/%q identity=%d source=%d attempt=%d next=%d local=%q/%q/%q/%q",
		generation, plan, cursor, state, reason, ownerStarted, ownerAfter, ownerValue, arm, current,
		readingArm, readingStarted, readingAssetAfter, readingStateAfter, readingSource,
		identityAfter, sourceAfter, attempt, nextAttempt,
		localAccount, localProfile, localMembership, localLifetime)
}

// titleWalkMutation admits a real removal root for an arbitrary production
// entity. The parent owns admission; tests only ask the production writer.
func titleWalkMutation(t *testing.T, c *catalogtest.Catalog, entity int64, owner string) compactcatalog.EntityMutation {
	t.Helper()
	var m compactcatalog.EntityMutation
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		ref, err := compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{
			Entity: entity, OwnerKind: "administration", OwnerID: owner, OwnerGeneration: 1, Mode: "remove",
		})
		if err != nil {
			return err
		}
		m, err = compactcatalog.ReadEntityMutationTx(ctx, tx, ref)
		return err
	})
	return m
}

// titleWalkAddOwnerAbove creates a real local owner whose canonical key sorts
// strictly after after. The walk's owner windows seek profile_id > owner_after,
// so a fixture that wants the next owner to be reachable must choose one above
// the current frontier instead of assuming key order.
func titleWalkAddOwnerAbove(t *testing.T, c *catalogtest.Catalog, after string) (owner, account, profile, lifetime, membership string) {
	t.Helper()
	for i := 0; i < 64; i++ {
		account = fmt.Sprintf("zz-title-walk-above-%02d", i)
		profile = account + "-profile"
		if persistence.PersonalOwnerKey("local", account, profile) > after {
			owner, lifetime, membership = titleWalkAddOwner(t, c, account, profile)
			return
		}
	}
	t.Fatalf("no local owner sorts above %q", after)
	return
}

// titleWalkEpisodeInputs writes the per-title inputs the drain owns for a
// profile whose title is an episode (no Reading position).
func titleWalkEpisodeInputs(t *testing.T, c *catalogtest.Catalog, owner string, item int64) {
	t.Helper()
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,revision) VALUES(?,?,1,1)`, owner, item)
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,100,0,0,'')`, owner, item)
	c.Exec(`INSERT INTO personal_watched_intents(profile_id,item_id,watched,authored_at) VALUES(?,?,1,'2026-01-02T00:00:00Z')`, owner, item)
	c.Exec(`INSERT INTO personal_play_counts(profile_id,item_id,plays) VALUES(?,?,1)`, owner, item)
	c.Exec(`INSERT INTO personal_history(id,profile_id,item_id,started_at,updated_at,sequence,position,unit,completed)
 VALUES('title-walk-history',?,?,'2026-01-01T00:00:00Z','2026-01-02T00:00:00Z',1,100,0,0)`, owner, item)
}

// titleWalkContainerState is an exact image of one container default row so a
// test can prove the walk enumerated it without deleting or rewriting it.
func titleWalkContainerState(t *testing.T, c *catalogtest.Catalog, profile string, container int64) string {
	t.Helper()
	var kind, watermark, cleared string
	var watched, revision int
	if err := c.DB.QueryRow(`SELECT kind,watched,watermark,revision,cleared_through FROM container_personal_state
 WHERE profile_id=? AND container_id=?`, profile, container).
		Scan(&kind, &watched, &watermark, &revision, &cleared); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%s/%d/%q/%d/%q", kind, watched, watermark, revision, cleared)
}

// titleWalkJobCleared is the exact approved phase-56 catalog-binding reset shape:
// every owner/progress scalar is back at the head while mutation, generation and
// plan digest stay untouched.
func titleWalkJobCleared(t *testing.T, c *catalogtest.Catalog, mutation string) bool {
	t.Helper()
	var clear bool
	if err := c.DB.QueryRow(`SELECT local_account='' AND local_profile=''
 AND local_membership_incarnation='' AND local_profile_lifetime=''
 AND owner_after='' AND owner_value='' AND owner_started=0 AND capture_arm=0
 AND current_state_id IS NULL AND cursor='' AND identity_after=0
 AND reading_arm='current' AND reading_started=0 AND state='pending' AND reason=''
 FROM catalog_purge_jobs WHERE mutation_id=?`, mutation).Scan(&clear); err != nil {
		t.Fatal(err)
	}
	return clear
}

// TestRetainedTitleWalkLifetimeCrashMidCurrentResumesExactVersions stops in the
// middle of copying one owner's 65 current Reading positions, reopens the
// database, and resumes the actual parent. Until publication the previous
// complete header and its identity mapping stay authoritative; the resumed walk
// then completes both version streams exactly and never mutates the old header.
func TestRetainedTitleWalkLifetimeCrashMidCurrentResumesExactVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "title-walk-crash.sqlite")
	db, err := persistence.OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	c := catalogtest.New(t, db)
	f := &captureFixture{c: c, account: "title-walk-crash", profile: "title-walk-crash-profile"}
	c.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id) VALUES(?,?,X'00',?)`, f.account, f.account, f.profile)
	if err = c.DB.QueryRow(`SELECT p.lifetime,m.incarnation FROM direct_profiles p
 JOIN direct_memberships m ON m.account_id=p.account_id WHERE p.account_id=? AND p.id=?`, f.account, f.profile).
		Scan(&f.lifetime, &f.membership); err != nil {
		t.Fatal(err)
	}
	f.owner = persistence.PersonalOwnerKey("local", f.account, f.profile)
	f.root = t.TempDir()
	library := c.Library("reading", "Reading", "book", f.root)
	f.item, f.token = captureAddPDF(t, c, library, 0, f.root, "original.pdf")

	// 65 real current versions, exactly the unbounded per-version stream the
	// capture component walks one asset per turn.
	const versions = 65
	tokens := []string{f.token}
	for i := 1; i < versions; i++ {
		_, token := captureAddPDF(t, c, 0, f.item, f.root, fmt.Sprintf("current-%d.pdf", i))
		tokens = append(tokens, token)
	}
	for i, token := range tokens {
		captureCurrentPosition(t, c, f.owner, f.item, captureAssetID(t, c, token),
			fmt.Sprintf("current-%d", i), float64(i)/100, int64(1000+i))
	}

	// The authoritative prior publication: one complete header under the title's
	// own local identity, with all 65 prior versions. Every prior observed_at is
	// older, so the current arm is copied first and an older prior version can
	// never replace it.
	identity := titleWalkIdentity(t, c, f.item)
	prior := make([]captureChild, versions)
	for i, token := range tokens {
		prior[i] = captureChild{
			token: token, revision: fmt.Sprintf("prior-%d", i),
			observed: int64(500 + i), progression: float64(i) / 200,
		}
	}
	old := captureOldState(t, c, f.owner, []string{identity}, prior)

	beginCaptureFixture(t, f)

	// Stop with the private header mid-way through the current-version copy.
	var private int64
	copied := 0
	for turn := 0; turn < 200; turn++ {
		done, effects := titleWalkStep(t, c, f.m)
		if done {
			t.Fatal("walk finished before the crash point")
		}
		if effects <= 0 || effects > phasedpurge.EffectLimit {
			t.Fatalf("turn %d effects=%d (per-writer limit %d)", turn, effects, phasedpurge.EffectLimit)
		}
		var current sql.NullInt64
		var arm int
		if err = c.DB.QueryRow(`SELECT current_state_id,capture_arm FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).
			Scan(&current, &arm); err != nil {
			t.Fatal(err)
		}
		if current.Valid {
			private = current.Int64
		}
		if private != 0 && arm == retainedCaptureArmCurrent {
			if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, private); n >= 8 {
				copied = n
				break
			}
		}
		if private != 0 && arm > retainedCaptureArmCurrent {
			t.Fatal("crash point passed before it could be observed")
		}
	}
	if private == 0 || copied < 8 || copied >= versions {
		t.Fatalf("fixture did not stop mid-copy: private=%d copied=%d", private, copied)
	}

	// Before the crash the old publication is still the only authority.
	var ready bool
	if err = c.DB.QueryRow(`SELECT ready FROM retained_title_state WHERE id=?`, old).Scan(&ready); err != nil || !ready {
		t.Fatalf("old published header changed before publication: ready=%v err=%v", ready, err)
	}
	var mapped int64
	if err = c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identity, f.owner).
		Scan(&mapped); err != nil || mapped != old {
		t.Fatalf("old mapping changed before publication: %d want %d err=%v", mapped, old, err)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, old); n != versions {
		t.Fatalf("old published children changed before publication: %d", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_identities WHERE state_id=?`, private); n != 0 {
		t.Fatalf("partial private header got %d published mappings", n)
	}
	if ready, _ = captureJobHeaderReady(t, c, private); ready {
		t.Fatal("partial private header became ready")
	}

	// Reopen as a restarted process would.
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if db, err = persistence.OpenFresh(path); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c = catalogtest.New(t, db)

	// The durable old publication and mapping are unchanged across the restart.
	if err = c.DB.QueryRow(`SELECT ready FROM retained_title_state WHERE id=?`, old).Scan(&ready); err != nil || !ready {
		t.Fatalf("old published header changed across the restart: ready=%v err=%v", ready, err)
	}
	if err = c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identity, f.owner).
		Scan(&mapped); err != nil || mapped != old {
		t.Fatalf("old mapping changed across the restart: %d want %d err=%v", mapped, old, err)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, old); n != versions {
		t.Fatalf("old published children changed across the restart: %d", n)
	}
	if ready, _ = captureJobHeaderReady(t, c, private); ready {
		t.Fatal("partial private header became ready across the restart")
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, private); n != copied {
		t.Fatalf("partial private copy did not resume exactly: %d want %d", n, copied)
	}

	// Resume the actual parent to completion.
	turns, effects := titleWalkDrive(t, c, f.m, 1000)
	if turns <= versions {
		t.Fatalf("resumed walk finished in %d turns; both version streams were not walked", turns)
	}
	for i, n := range effects {
		if n > phasedpurge.EffectLimit {
			t.Fatalf("resumed turn %d effects=%d limit=%d", i, n, phasedpurge.EffectLimit)
		}
	}

	var published int64
	if err = c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identity, f.owner).
		Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published == old {
		t.Fatal("resume reused the prior header instead of publishing a fresh one")
	}
	var newReady bool
	var capture sql.NullString
	var generation int64
	if err = c.DB.QueryRow(`SELECT ready,capture_mutation_id,capture_generation FROM retained_title_state WHERE id=?`, published).
		Scan(&newReady, &capture, &generation); err != nil || !newReady || capture.Valid || generation != 0 {
		t.Fatalf("resumed publication not complete: ready=%v capture=%v generation=%d err=%v", newReady, capture, generation, err)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, published); n != versions {
		t.Fatalf("resumed header holds %d of %d current versions", n, versions)
	}
	for _, i := range []int{0, 1, versions / 2, versions - 1} {
		var progression float64
		var revision string
		if err = c.DB.QueryRow(`SELECT progression,source_revision FROM retained_reading_progress WHERE state_id=? AND source_id=?`,
			published, tokens[i]).Scan(&progression, &revision); err != nil {
			t.Fatal(err)
		}
		if revision != fmt.Sprintf("current-%d", i) || progression != float64(i)/100 {
			t.Fatalf("version %d merged the wrong stream: revision=%q progression=%v", i, revision, progression)
		}
	}
	// The old complete header and its 65 prior versions survive untouched.
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, old); n != versions {
		t.Fatalf("old complete children changed at republication: %d", n)
	}
	for _, i := range []int{0, versions / 2, versions - 1} {
		var revision string
		if err = c.DB.QueryRow(`SELECT source_revision FROM retained_reading_progress WHERE state_id=? AND source_id=?`, old, tokens[i]).
			Scan(&revision); err != nil {
			t.Fatal(err)
		}
		if revision != fmt.Sprintf("prior-%d", i) {
			t.Fatalf("old prior version %d changed: %q", i, revision)
		}
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE id=? AND ready=1`, old); n != 1 {
		t.Fatal("old complete header was deleted")
	}
	if n := titleWalkCount(t, c, `SELECT title_count FROM retained_title_state_counts WHERE profile_id=?`, f.owner); n != 2 {
		t.Fatalf("maintained count=%d want the old and resumed headers", n)
	}

	var arm int
	var ownerStarted bool
	var reason string
	if err = c.DB.QueryRow(`SELECT capture_arm,owner_started,reason FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).
		Scan(&arm, &ownerStarted, &reason); err != nil {
		t.Fatal(err)
	}
	if arm != retainedCaptureArmPublished || ownerStarted || reason != retainedTitleOwnerDrained {
		t.Fatalf("resumed walk job arm=%d started=%v reason=%q", arm, ownerStarted, reason)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE capture_mutation_id=?`, f.m.Ref.ID); n != 0 {
		t.Fatalf("private headers survived completion: %d", n)
	}
}

// TestRetainedTitleWalkLifetimeRefusesPlanAndCursorMismatch corrupts the stored
// plan digest during a private capture and, later, the shared-engine cursor in
// the middle of the owner drain. Both must be visible refusals that write
// nothing, and the parent must then finish the drain without deleting the
// title's container defaults it only enumerates as owners.
func TestRetainedTitleWalkLifetimeRefusesPlanAndCursorMismatch(t *testing.T) {
	f := newCaptureFixture(t)
	c := f.c
	tvRoot := t.TempDir()
	library := c.Library("tv", "TV", "tv", tvRoot)
	show := c.Show(library, "Walk Show", 2020)
	season := c.Season(show, 1)
	episode := c.Episode(show, season, 1, filepath.Join(tvRoot, "episode.mkv"))
	titleWalkEpisodeInputs(t, c, f.owner, episode.ID)
	m := titleWalkMutation(t, c, episode.ID, "title-walk-plan")

	if done, _ := titleWalkStep(t, c, m); done {
		t.Fatal("walk finished without a durable job")
	}
	var plan string
	if err := c.DB.QueryRow(`SELECT plan_digest FROM catalog_purge_jobs WHERE mutation_id=?`, m.Ref.ID).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if plan != retainedTitleDrainPlanDigest || len(plan) != 64 {
		t.Fatalf("job plan digest %q is not the compiled walk", plan)
	}
	titleWalkStep(t, c, m) // select the only real owner
	titleWalkStep(t, c, m) // build the private ready=0 header

	var private sql.NullInt64
	var arm int
	if err := c.DB.QueryRow(`SELECT current_state_id,capture_arm FROM catalog_purge_jobs WHERE mutation_id=?`, m.Ref.ID).
		Scan(&private, &arm); err != nil {
		t.Fatal(err)
	}
	if !private.Valid || arm != retainedCaptureArmHeader {
		t.Fatalf("fixture did not stop at the private header: pointer=%v arm=%d", private, arm)
	}

	// A plan that is not this compiled walk is refused before any write.
	c.Exec(`UPDATE catalog_purge_jobs SET plan_digest=? WHERE mutation_id=?`, strings.Repeat("c", 64), m.Ref.ID)
	planImage := titleWalkJobSnapshot(t, c, m.Ref.ID)
	headerRows := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE capture_mutation_id=?`, m.Ref.ID)
	identityRows := titleWalkCount(t, c, `SELECT count(*) FROM catalog_purge_identities WHERE mutation_id=?`, m.Ref.ID)
	done, effects, err := titleWalkAttempt(t, c, m)
	if !errors.Is(err, errRetainedTitlePlan) {
		t.Fatalf("foreign plan: done=%v effects=%d err=%v", done, effects, err)
	}
	if done || effects != 0 {
		t.Fatalf("foreign plan refusal wrote: done=%v effects=%d", done, effects)
	}
	if after := titleWalkJobSnapshot(t, c, m.Ref.ID); after != planImage {
		t.Fatalf("foreign plan refusal changed the job:\n before=%s\n after =%s", planImage, after)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE capture_mutation_id=?`, m.Ref.ID); n != headerRows {
		t.Fatalf("foreign plan refusal touched private headers: %d want %d", n, headerRows)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM catalog_purge_identities WHERE mutation_id=?`, m.Ref.ID); n != identityRows {
		t.Fatalf("foreign plan refusal touched identity witnesses: %d want %d", n, identityRows)
	}
	var ready bool
	var capture string
	if err := c.DB.QueryRow(`SELECT ready,capture_mutation_id FROM retained_title_state WHERE id=?`, private.Int64).
		Scan(&ready, &capture); err != nil {
		t.Fatal(err)
	}
	if ready || capture != m.Ref.ID {
		t.Fatalf("foreign plan refusal published or retargeted the private header: ready=%v capture=%q", ready, capture)
	}

	// Put back the exact digest the compiled walk wrote, then reach a real drain
	// cursor that the component itself saved.
	c.Exec(`UPDATE catalog_purge_jobs SET plan_digest=? WHERE mutation_id=?`, retainedTitleDrainPlanDigest, m.Ref.ID)
	var raw string
	for turn := 0; turn < 120; turn++ {
		titleWalkStep(t, c, m)
		if err := c.DB.QueryRow(`SELECT cursor FROM catalog_purge_jobs WHERE mutation_id=?`, m.Ref.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if raw != "" {
			break
		}
	}
	if raw == "" {
		t.Fatal("walk never persisted a drain cursor")
	}

	// A cursor from another plan is refused; the whole drain frontier is intact.
	c.Exec(`UPDATE catalog_purge_jobs SET cursor='not-the-compiled-drain-cursor' WHERE mutation_id=?`, m.Ref.ID)
	cursorImage := titleWalkJobSnapshot(t, c, m.Ref.ID)
	stateRows := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE profile_id=?`, f.owner)
	mappingRows := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_identities WHERE profile_id=?`, f.owner)
	done, effects, err = titleWalkAttempt(t, c, m)
	if !errors.Is(err, errRetainedTitleCursor) {
		t.Fatalf("foreign cursor: done=%v effects=%d err=%v", done, effects, err)
	}
	if done || effects != 0 {
		t.Fatalf("foreign cursor refusal wrote: done=%v effects=%d", done, effects)
	}
	if after := titleWalkJobSnapshot(t, c, m.Ref.ID); after != cursorImage {
		t.Fatalf("foreign cursor refusal changed the job:\n before=%s\n after =%s", cursorImage, after)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE profile_id=?`, f.owner); n != stateRows {
		t.Fatalf("foreign cursor refusal touched retained state: %d want %d", n, stateRows)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_identities WHERE profile_id=?`, f.owner); n != mappingRows {
		t.Fatalf("foreign cursor refusal touched retained mappings: %d want %d", n, mappingRows)
	}
	// Restore exactly the cursor the component saved.
	c.Exec(`UPDATE catalog_purge_jobs SET cursor=? WHERE mutation_id=?`, raw, m.Ref.ID)

	// A different profile whose only state is a container default is a reachable
	// owner that the walk must enumerate and never delete.
	containerOwner, _, _, _, _ := titleWalkAddOwnerAbove(t, c, f.owner)
	c.Exec(`INSERT INTO container_personal_state(profile_id,kind,container_id,watched,watermark,revision,cleared_through)
 VALUES(?, 'show', ?, 1, '2026-10-09T00:00:00Z', 3, '')`, containerOwner, show.ID)
	containerImage := titleWalkContainerState(t, c, containerOwner, show.ID)

	// The compiled owner windows are exactly six per-title sources plus the four
	// container ancestors, each one bounded LIMIT 1 seek with no offset.
	if len(retainedTitleOwnerSQL) != 10 {
		t.Fatalf("owner windows=%d want six per-title sources plus four ancestors", len(retainedTitleOwnerSQL))
	}
	for _, q := range retainedTitleOwnerSQL {
		if strings.Count(q, "LIMIT 1") != 1 || strings.Contains(q, "OFFSET") {
			t.Fatalf("owner window is not one bounded seek: %s", q)
		}
	}

	rows, err := c.DB.Query(`EXPLAIN QUERY PLAN `+retainedTitleNextShowDefaultSQL, episode.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	var planText []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		planText = append(planText, detail)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	explained := strings.Join(planText, "\n")
	if !strings.Contains(explained, "container_personal_state_retention_owner") ||
		strings.Contains(explained, "TEMP B-TREE") || strings.Contains(explained, "SCAN container_personal_state") {
		t.Fatalf("container-owner window is not a bounded index seek:\n%s", explained)
	}

	sawContainer := false
	finished := false
	for turn := 0; turn < 200; turn++ {
		done, effects := titleWalkStep(t, c, m)
		if effects > phasedpurge.EffectLimit {
			t.Fatalf("turn %d effects=%d limit=%d", turn, effects, phasedpurge.EffectLimit)
		}
		var ownerValue string
		if err := c.DB.QueryRow(`SELECT owner_value FROM catalog_purge_jobs WHERE mutation_id=?`, m.Ref.ID).Scan(&ownerValue); err != nil {
			t.Fatal(err)
		}
		if ownerValue == containerOwner {
			sawContainer = true
		}
		if done {
			finished = true
			break
		}
	}
	if !finished {
		t.Fatal("walk did not finish after the refusals")
	}
	if !sawContainer {
		t.Fatal("the container-default owner was never enumerated")
	}
	if got := titleWalkContainerState(t, c, containerOwner, show.ID); got != containerImage {
		t.Fatalf("container default changed across the drain: %s want %s", got, containerImage)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM container_personal_state WHERE profile_id=?`, containerOwner); n != 1 {
		t.Fatalf("container defaults deleted: %d", n)
	}

	for _, table := range []struct{ name, query string }{
		{"personal_items", `SELECT count(*) FROM personal_items WHERE profile_id=? AND item_id=?`},
		{"progress", `SELECT count(*) FROM progress WHERE profile_id=? AND item_id=?`},
		{"personal_watched_intents", `SELECT count(*) FROM personal_watched_intents WHERE profile_id=? AND item_id=?`},
		{"personal_play_counts", `SELECT count(*) FROM personal_play_counts WHERE profile_id=? AND item_id=?`},
		{"personal_history", `SELECT count(*) FROM personal_history WHERE profile_id=? AND item_id=?`},
	} {
		if n := titleWalkCount(t, c, table.query, f.owner, episode.ID); n != 0 {
			t.Fatalf("%s survived the verified drain: %d", table.name, n)
		}
	}
	// The walk prepared one removed-title header and published exactly one
	// viewer fact for the drained owner; the history/favourite windows merge on
	// the same (public_id,profile_id) row.
	public := c.Public(episode.ID)
	if n := titleWalkCount(t, c, `SELECT count(*) FROM catalog_title_tombstones WHERE public_id=?`, public); n != 1 {
		t.Fatalf("removed-title header count=%d", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM catalog_title_tombstone_viewers WHERE public_id=? AND profile_id=?`, public, f.owner); n != 1 {
		t.Fatalf("removed-title viewer facts=%d want one merged row", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE profile_id=? AND ready=1`, f.owner); n != 1 {
		t.Fatalf("published retained header changed: %d", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE capture_mutation_id=?`, m.Ref.ID); n != 0 {
		t.Fatalf("private headers survived completion: %d", n)
	}
	// The show mark covers this episode and must survive as retained watched
	// state, while its source container default remains unchanged above.
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE profile_id=? AND ready=1 AND watched=1`, containerOwner); n != 1 {
		t.Fatalf("container-default owner lost retained watched state: %d", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_identities WHERE profile_id=?`, containerOwner); n == 0 {
		t.Fatal("container-default retained state has no restoration mapping")
	}
}

// TestRetainedTitleWalkLifetimeProfileErasureResetResumesFromHead runs the real
// merged P2-62 profile-erasure engine while one owner's capture is in progress.
// The approved phase-56 catalog-binding unit resets the durable job to its head
// (preserving mutation, generation and plan digest). The actual parent must then
// re-enter raw discovery for the surviving owner, never republish the erased
// owner's captured scalars and never fall back to an opaque/Hosted copy.
func TestRetainedTitleWalkLifetimeProfileErasureResetResumesFromHead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "title-walk-erasure.sqlite")
	db, err := persistence.OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := catalogtest.New(t, db)
	f := &captureFixture{c: c, account: "title-walk-erase", profile: "title-walk-erase-profile"}
	c.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id) VALUES(?,?,X'00',?)`, f.account, f.account, f.profile)
	if err = c.DB.QueryRow(`SELECT p.lifetime,m.incarnation FROM direct_profiles p
 JOIN direct_memberships m ON m.account_id=p.account_id WHERE p.account_id=? AND p.id=?`, f.account, f.profile).
		Scan(&f.lifetime, &f.membership); err != nil {
		t.Fatal(err)
	}
	f.owner = persistence.PersonalOwnerKey("local", f.account, f.profile)
	f.root = t.TempDir()
	library := c.Library("reading", "Reading", "book", f.root)
	f.item, f.token = captureAddPDF(t, c, library, 0, f.root, "erase.pdf")
	asset := captureAssetID(t, c, f.token)

	// The owner being erased, and a surviving owner whose key sorts after it so
	// the erasure is driven while the first owner's capture is in progress.
	titleWalkInputs(t, c, f.owner, f.item, asset)
	survivor, survivorAccount, survivorProfile, survivorLifetime, survivorMembership := titleWalkAddOwnerAbove(t, c, f.owner)
	titleWalkInputs(t, c, survivor, f.item, asset)

	beginCaptureFixture(t, f)

	// Drive the real capture to a committed mid-copy state: a private ready=0
	// header with the erased owner's scalars and at least one current Reading
	// child, before any identity mapping exists.
	var private int64
	for turn := 0; turn < 32; turn++ {
		if done, _ := titleWalkStep(t, c, f.m); done {
			t.Fatalf("walk finished at turn %d before the reset", turn)
		}
		var current sql.NullInt64
		var arm int
		if err = c.DB.QueryRow(`SELECT current_state_id,capture_arm FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).
			Scan(&current, &arm); err != nil {
			t.Fatal(err)
		}
		if current.Valid && arm == retainedCaptureArmCurrent {
			if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, current.Int64); n >= 1 {
				private = current.Int64
				break
			}
		}
	}
	if private == 0 {
		t.Fatal("fixture never reached an in-progress private capture")
	}
	var capturedOwner string
	var watchlisted int
	if err = c.DB.QueryRow(`SELECT owner_value FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).Scan(&capturedOwner); err != nil {
		t.Fatal(err)
	}
	if capturedOwner != f.owner {
		t.Fatalf("capture was not in progress for the erased owner: %q", capturedOwner)
	}
	if err = c.DB.QueryRow(`SELECT watchlisted FROM retained_title_state WHERE id=?`, private).Scan(&watchlisted); err != nil || watchlisted != 1 {
		t.Fatalf("private header did not capture the owner's scalar state: %d %v", watchlisted, err)
	}

	// The committed denial a real retirement leaves, plus its security job. The
	// reset below is performed by the merged engine, never by this test.
	svc, err := identity.New(db, t.TempDir())
	if err != nil {
		t.Fatalf("identity service: %v", err)
	}
	c.Exec(`UPDATE direct_profiles SET deleted=1 WHERE account_id=? AND id=?`, f.account, f.profile)
	c.Exec(`INSERT INTO security_purge_jobs(job_id,authority,account_id,profile_id,account_incarnation,profile_incarnation,state)
 VALUES(?,'local',?,?,?,?,'pending')`,
		identity.Token()[:22], f.account, f.profile, f.membership, f.lifetime)
	if titleWalkJobCleared(t, c, f.m.Ref.ID) {
		t.Fatal("fixture job is already at the erase head")
	}

	resetTurn := 0
	for turn := 0; turn < 400; turn++ {
		worked, err := svc.PurgeProfileStep(context.Background())
		if err != nil {
			t.Fatalf("profile purge step %d: %v", turn, err)
		}
		if titleWalkJobCleared(t, c, f.m.Ref.ID) {
			resetTurn = turn + 1
			if !worked {
				t.Fatal("the clearing turn reported no work")
			}
			break
		}
	}
	if resetTurn == 0 {
		t.Fatal("the merged profile-erasure engine never reset the in-flight catalog job")
	}

	// The reset is the approved phase-56 binding unit: every owner/progress
	// scalar is at the head and the actual root/generation/plan are preserved.
	var phase int
	if err = c.DB.QueryRow(`SELECT phase FROM security_purge_jobs WHERE profile_id=?`, f.profile).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != 56 {
		t.Fatalf("catalog reset did not run from the approved phase-56 unit: phase=%d", phase)
	}
	var plan, ownerAfter, ownerValue, localProfile string
	var generation int64
	var started bool
	if err = c.DB.QueryRow(`SELECT plan_digest,generation,owner_after,owner_value,local_profile,owner_started
 FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).
		Scan(&plan, &generation, &ownerAfter, &ownerValue, &localProfile, &started); err != nil {
		t.Fatal(err)
	}
	if plan != retainedTitleDrainPlanDigest || generation != f.m.Ref.Generation {
		t.Fatalf("reset lost the actual root: plan=%s generation=%d", plan, generation)
	}
	if ownerAfter != "" || ownerValue != "" || localProfile != "" || started {
		t.Fatalf("reset did not return the frontier to its head: after=%q value=%q local=%q started=%v",
			ownerAfter, ownerValue, localProfile, started)
	}

	// The in-flight private header is custody, not a completion: the reset dropped
	// only its pointer and no mapping or published row was created.
	var headerReady bool
	var headerCapture string
	var headerGeneration int64
	if err = c.DB.QueryRow(`SELECT ready,capture_mutation_id,capture_generation FROM retained_title_state WHERE id=?`, private).
		Scan(&headerReady, &headerCapture, &headerGeneration); err != nil {
		t.Fatalf("private header lost by the reset: %v", err)
	}
	if headerReady || headerCapture != f.m.Ref.ID || headerGeneration != f.m.Ref.Generation {
		t.Fatalf("reset changed private header custody: ready=%v capture=%q generation=%d",
			headerReady, headerCapture, headerGeneration)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_identities WHERE state_id=?`, private); n != 0 {
		t.Fatalf("reset published %d mappings for the private header", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE profile_id=? AND ready=1`, f.owner); n != 0 {
		t.Fatalf("reset published %d rows for the erased owner", n)
	}

	// The parent resumes from the head. The only remaining real owner is the
	// survivor, and its exact local binding is re-derived rather than reusing the
	// erased owner's scalars.
	done, effects := titleWalkStep(t, c, f.m)
	if done {
		t.Fatal("parent treated the erased head as complete")
	}
	if effects > phasedpurge.EffectLimit {
		t.Fatalf("head-selection turn effects=%d limit=%d", effects, phasedpurge.EffectLimit)
	}
	var selected, account, profile, lifetime, membership string
	if err = c.DB.QueryRow(`SELECT owner_value,local_account,local_profile,local_profile_lifetime,local_membership_incarnation
 FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).
		Scan(&selected, &account, &profile, &lifetime, &membership); err != nil {
		t.Fatal(err)
	}
	if selected != survivor {
		t.Fatalf("parent did not re-enter raw discovery for the surviving owner: %q want %q", selected, survivor)
	}
	if account != survivorAccount || profile != survivorProfile ||
		lifetime != survivorLifetime || membership != survivorMembership {
		t.Fatalf("surviving owner binding was not the exact local tuple: %q/%q/%q/%q", account, profile, lifetime, membership)
	}

	for turn := 0; !done; turn++ {
		if turn > 240 {
			t.Fatal("resumed walk did not finish")
		}
		done, effects = titleWalkStep(t, c, f.m)
		if effects > phasedpurge.EffectLimit {
			t.Fatalf("resumed turn %d effects=%d limit=%d", turn, effects, phasedpurge.EffectLimit)
		}
		var ownerValue string
		if err = c.DB.QueryRow(`SELECT owner_value FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).Scan(&ownerValue); err != nil {
			t.Fatal(err)
		}
		if ownerValue != survivor {
			t.Fatalf("walk left the surviving owner for %q", ownerValue)
		}
		if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE profile_id=? AND ready=1`, f.owner); n != 0 {
			t.Fatalf("erased owner was republished: %d", n)
		}
		if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE id=? AND ready=1`, private); n != 0 {
			t.Fatalf("in-flight private header was published: %d", n)
		}
	}

	// The erased owner left no published header, no mapping and no Hosted/opaque
	// fallback; its partial private copy was structurally drained, not published.
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE id=?`, private); n != 0 {
		t.Fatalf("private header survived its structural reset: %d", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE profile_id=?`, f.owner); n != 0 {
		t.Fatalf("erased owner has retained rows: %d", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state WHERE profile_id=?`, f.profile); n != 0 {
		t.Fatalf("erased owner was retained under a raw/Hosted identity: %d", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_identities WHERE profile_id IN(?,?)`, f.owner, f.profile); n != 0 {
		t.Fatalf("erased owner has retained mappings: %d", n)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_title_state`); n != 1 {
		t.Fatalf("exactly the surviving owner's header should remain: %d", n)
	}
	var kept int64
	if err = c.DB.QueryRow(`SELECT id FROM retained_title_state WHERE profile_id=? AND ready=1`, survivor).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	var survivorMapped int64
	if err = c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`,
		titleWalkIdentity(t, c, f.item), survivor).Scan(&survivorMapped); err != nil {
		t.Fatal(err)
	}
	if survivorMapped != kept {
		t.Fatalf("surviving mapping=%d want %d", survivorMapped, kept)
	}
	if n := titleWalkCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, kept); n != 1 {
		t.Fatalf("surviving owner lost its Reading position: %d", n)
	}
	for _, table := range []struct{ name, query string }{
		{"personal_items", `SELECT count(*) FROM personal_items WHERE profile_id=? AND item_id=?`},
		{"progress", `SELECT count(*) FROM progress WHERE profile_id=? AND item_id=?`},
		{"reading_progress", `SELECT count(*) FROM reading_progress WHERE owner_key=? AND item_id=?`},
		{"personal_watched_intents", `SELECT count(*) FROM personal_watched_intents WHERE profile_id=? AND item_id=?`},
		{"personal_play_counts", `SELECT count(*) FROM personal_play_counts WHERE profile_id=? AND item_id=?`},
	} {
		if n := titleWalkCount(t, c, table.query, survivor, f.item); n != 0 {
			t.Fatalf("%s survived the surviving owner's drain: %d", table.name, n)
		}
	}
	var arm int
	var reason string
	if err = c.DB.QueryRow(`SELECT capture_arm,reason FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).
		Scan(&arm, &reason); err != nil {
		t.Fatal(err)
	}
	if arm != retainedCaptureArmPublished || reason != retainedTitleOwnerDrained {
		t.Fatalf("resumed job arm=%d reason=%q", arm, reason)
	}
}
