package identity

// O022 / P2-62 profile-adapter tests against the real installed baseline.
//
// These tests exercise the two approved phase-56 catalog capture-reset units and
// the retained-header custody unit. Every root is a real
// catalog_entity_mutations row created through
// compactcatalog.BeginEntityMutationTx; no schema stub, forged completion
// counter or claimed capture phase is installed. A private ready=0 header and
// its Reading children are legitimate domain input state, and the tests assert
// that a reset preserves that custody instead of completing or republishing it.
//
// o022SeedJob's non-default owner/progress columns are likewise domain input
// state for the erasure unit: they give the reset something to clear. They are
// not evidence that a production capture step ran or completed; the real
// capture writer and its phases are proven separately by the catalog
// retained-capture tests.

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/phasedpurge"
)

// o022ResetSet is the approved literal reset SET list. It is duplicated here on
// purpose: the exact-SQL test below pins both the source constant and the
// compiled statement, so a silent SET change cannot ride along with a predicate
// change.
const o022ResetSet = "local_account='',local_profile=''," +
	"local_membership_incarnation='',local_profile_lifetime=''," +
	"owner_after='',owner_value='',owner_started=0,current_state_id=NULL," +
	"capture_arm=0,source_after=0,reading_arm='current',reading_started=0," +
	"reading_asset_after=0,reading_state_after=0,reading_source_after=''," +
	"identity_after=0,cursor='',state='pending',attempt=0,next_attempt_ms=0,reason=''"

const o022PlanDigestFixture = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type o022Fixture struct {
	s       *Service
	db      *sql.DB
	login   DirectSignIn
	kid     DirectProfile
	preview ProfileDeletionPreview
}

func o022NewFixture(t *testing.T) *o022Fixture {
	t.Helper()
	s, db, login, kid, preview := retirementFixture(t)
	return &o022Fixture{s: s, db: db, login: login, kid: kid, preview: preview}
}

// o022Mutation creates a real catalog entity and the admitted removal root the
// catalogue capture owner uses. compactcatalog has no identity import, so this
// test can use the production writers without an import cycle.
func o022Mutation(t *testing.T, db *sql.DB, owner string) compactcatalog.EntityMutation {
	t.Helper()
	ctx := context.Background()
	var mutation compactcatalog.EntityMutation
	err := dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO libraries(id,name,kind,root) VALUES('o022','O022','movie','/fixtures/o022')`); err != nil {
			return err
		}
		library, err := compactcatalog.LibraryTx(ctx, tx, "o022")
		if err != nil {
			return err
		}
		item, _, err := compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{
			Library: library, Kind: compactcatalog.Movie, Key: "o022:" + owner, Title: "O022 " + owner,
		})
		if err != nil {
			return err
		}
		ref, err := compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{
			Entity: item, OwnerKind: "administration", OwnerID: "o022-" + owner, OwnerGeneration: 1, Mode: "remove",
		})
		if err != nil {
			return err
		}
		mutation, err = compactcatalog.ReadEntityMutationTx(ctx, tx, ref)
		return err
	})
	if err != nil {
		t.Fatalf("O022 mutation root %q: %v", owner, err)
	}
	return mutation
}

// o022JobSeed inserts the owner-shaped job row this erasure unit consumes.
// Progress columns are domain input state for the reset to clear, not verified
// capture history or a claim that a production capture phase ran.
type o022JobSeed struct {
	m          compactcatalog.EntityMutation
	ownerAfter string
	ownerValue string
	account    string
	profile    string
	membership string
	lifetime   string
	current    sql.NullInt64
	armZero    bool
	clear      bool
}

func o022SeedJob(t *testing.T, db *sql.DB, seed o022JobSeed) {
	t.Helper()
	if seed.clear {
		_, err := db.Exec(`INSERT INTO catalog_purge_jobs(
 mutation_id,generation,plan_digest,cursor,owner_after,owner_value,owner_started,
 capture_arm,source_after,state,attempt,next_attempt_ms,reason,current_state_id,
 reading_arm,reading_started,reading_asset_after,reading_state_after,reading_source_after,
 identity_after,local_account,local_profile,local_membership_incarnation,local_profile_lifetime)
 VALUES(?,?,?,'','','',0,0,0,'pending',0,0,'',NULL,'current',0,0,0,'',0,'','','','')`,
			seed.m.Ref.ID, seed.m.Ref.Generation, o022PlanDigestFixture)
		if err != nil {
			t.Fatalf("O022 cleared job fixture %q: %v", seed.m.Ref.ID, err)
		}
		return
	}
	ownerStarted, captureArm, sourceAfter := 1, 4, 9
	if seed.armZero {
		ownerStarted, captureArm, sourceAfter = 0, 0, 0
	}
	_, err := db.Exec(`INSERT INTO catalog_purge_jobs(
 mutation_id,generation,plan_digest,cursor,owner_after,owner_value,owner_started,
 capture_arm,source_after,state,attempt,next_attempt_ms,reason,current_state_id,
 reading_arm,reading_started,reading_asset_after,reading_state_after,reading_source_after,
 identity_after,local_account,local_profile,local_membership_incarnation,local_profile_lifetime)
 VALUES(?,?,?,'suppressed-o022',?,?,?, ?,?,'running',2,123,'o022 paused',?,
 'retained',1,7,8,'o022-source',3,?,?,?,?)`,
		seed.m.Ref.ID, seed.m.Ref.Generation, o022PlanDigestFixture,
		seed.ownerAfter, seed.ownerValue, ownerStarted, captureArm, sourceAfter, seed.current,
		seed.account, seed.profile, seed.membership, seed.lifetime)
	if err != nil {
		t.Fatalf("O022 job fixture %q: %v", seed.m.Ref.ID, err)
	}
}

func o022PrivateHeader(t *testing.T, db *sql.DB, profile string, m compactcatalog.EntityMutation) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`INSERT INTO retained_title_state(profile_id,kind,title,retained_ms,ready,capture_mutation_id,capture_generation)
 VALUES(?,?, 'O022 private',1000,0,?,?) RETURNING id`,
		profile, int(compactcatalog.Book), m.Ref.ID, m.Ref.Generation).Scan(&id)
	if err != nil {
		t.Fatalf("O022 private header fixture: %v", err)
	}
	return id
}

func o022PublishedHeader(t *testing.T, db *sql.DB, profile string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`INSERT INTO retained_title_state(profile_id,kind,title,retained_ms)
 VALUES(?,?, 'O022 published',1000) RETURNING id`,
		profile, int(compactcatalog.Book)).Scan(&id); err != nil {
		t.Fatalf("O022 published header fixture: %v", err)
	}
	return id
}

func o022ReadingChild(t *testing.T, db *sql.DB, state int64, source string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO retained_reading_progress(
 state_id,source_id,source_revision,format,page_index,progression,revision,observed_at)
 VALUES(?,?,'o022-revision','pdf',0,0.5,1,1)`, state, source); err != nil {
		t.Fatalf("O022 Reading child fixture: %v", err)
	}
}

func o022IdentityWitness(t *testing.T, db *sql.DB, mutation string, ordinal int, identity string, previous sql.NullInt64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO catalog_purge_identities(mutation_id,ordinal,identity,mapping_captured,previous_state_id)
 VALUES(?,?,?,0,?)`, mutation, ordinal, identity, previous); err != nil {
		t.Fatalf("O022 identity witness fixture: %v", err)
	}
}

type o022JobRow struct {
	mutation, plan, cursor, ownerAfter, ownerValue                string
	generation                                                    int64
	ownerStarted, captureArm, sourceAfter                         int
	state, reason, readingArm, readingSource                      string
	current                                                       sql.NullInt64
	readingStarted, readingAssetAfter, readingStateAfter          int
	identityAfter                                                 int
	attempt, nextAttempt                                          int64
	localAccount, localProfile, localMembership, localProfileTime string
}

func o022ReadJob(t *testing.T, db *sql.DB, mutation string) o022JobRow {
	t.Helper()
	var row o022JobRow
	err := db.QueryRow(`SELECT mutation_id,generation,plan_digest,cursor,owner_after,owner_value,
 owner_started,capture_arm,source_after,state,attempt,next_attempt_ms,reason,current_state_id,
 reading_arm,reading_started,reading_asset_after,reading_state_after,reading_source_after,
 identity_after,local_account,local_profile,local_membership_incarnation,local_profile_lifetime
 FROM catalog_purge_jobs WHERE mutation_id=?`, mutation).Scan(
		&row.mutation, &row.generation, &row.plan, &row.cursor, &row.ownerAfter, &row.ownerValue,
		&row.ownerStarted, &row.captureArm, &row.sourceAfter, &row.state, &row.attempt, &row.nextAttempt, &row.reason, &row.current,
		&row.readingArm, &row.readingStarted, &row.readingAssetAfter, &row.readingStateAfter, &row.readingSource,
		&row.identityAfter, &row.localAccount, &row.localProfile, &row.localMembership, &row.localProfileTime)
	if err != nil {
		t.Fatalf("O022 job read %q: %v", mutation, err)
	}
	return row
}

func o022AssertCleared(t *testing.T, row o022JobRow) {
	t.Helper()
	if row.localAccount != "" || row.localProfile != "" || row.localMembership != "" || row.localProfileTime != "" ||
		row.ownerAfter != "" || row.ownerValue != "" || row.ownerStarted != 0 || row.current.Valid ||
		row.captureArm != 0 || row.sourceAfter != 0 || row.readingArm != "current" || row.readingStarted != 0 ||
		row.readingAssetAfter != 0 || row.readingStateAfter != 0 || row.readingSource != "" ||
		row.identityAfter != 0 || row.cursor != "" || row.state != "pending" || row.attempt != 0 ||
		row.nextAttempt != 0 || row.reason != "" {
		t.Fatalf("O022 reset did not clear every owner/progress field: %+v", row)
	}
}

func o022Unit(t *testing.T, s *Service, job *purgeJob, name string) phasedpurge.Unit {
	t.Helper()
	for _, unit := range profilePurgeUnits(s, job) {
		if unit.Name == name {
			return unit
		}
	}
	t.Fatalf("O022 unit %q is not in the compiled plan", name)
	return phasedpurge.Unit{}
}

func o022Run(t *testing.T, db *sql.DB, unit phasedpurge.Unit) phasedpurge.Outcome {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	out, err := unit.Run(context.Background(), tx, nil)
	if err != nil {
		tx.Rollback()
		t.Fatalf("O022 unit %s: %v", unit.Name, err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatalf("O022 unit %s commit: %v", unit.Name, err)
	}
	return out
}

func o022Cleared(t *testing.T, db *sql.DB, mutation string) bool {
	t.Helper()
	var flag int
	err := db.QueryRow(`SELECT local_account='' AND local_profile='' AND owner_value='' AND owner_after=''
 AND current_state_id IS NULL AND state='pending' AND capture_arm=0 AND reading_arm='current'
 AND identity_after=0 AND cursor='' FROM catalog_purge_jobs WHERE mutation_id=?`, mutation).Scan(&flag)
	if err != nil {
		t.Fatalf("O022 cleared probe %q: %v", mutation, err)
	}
	return flag == 1
}

func o022Count(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("O022 count %q: %v", query, err)
	}
	return n
}

func o022TotalChanges(t *testing.T, tx *sql.Tx) int64 {
	t.Helper()
	var n int64
	if err := tx.QueryRow(`SELECT total_changes()`).Scan(&n); err != nil {
		t.Fatalf("O022 total_changes: %v", err)
	}
	return n
}

func o022SecurityJob(t *testing.T, db *sql.DB, profile string) *purgeJob {
	t.Helper()
	var job purgeJob
	err := db.QueryRow(`SELECT job_id,account_id,profile_id,account_incarnation,profile_incarnation,
 cursor_key,attempt,state,process_epoch,next_attempt_ms FROM security_purge_jobs WHERE profile_id=?`, profile).Scan(
		&job.id, &job.account, &job.profile, &job.accountLifetime, &job.profileLifetime,
		&job.cursor, &job.attempt, &job.state, &job.epoch, &job.due)
	if err != nil {
		t.Fatalf("O022 security job read: %v", err)
	}
	return &job
}

// TestProfilePurgeO022LocalBindingCaptureReset proves the exact account/profile
// binding unit resets one job per turn across stale incarnations and lifetimes,
// clears both aligned owner frontiers, the current pointer, all four local
// fields and every private-copy progress field, and leaves the actual mutation
// root, generation, plan digest, private header, Reading children and identity
// witnesses in custody.
func TestProfilePurgeO022LocalBindingCaptureReset(t *testing.T) {
	f := o022NewFixture(t)
	job := purgeTestJob(purgeTestAccount, f.kid.ID)
	unit := o022Unit(t, f.s, job, "catalog-retained-capture-owner")

	memberCurrent, lifetimeCurrent := strings.Repeat("a", 32), strings.Repeat("b", 32)
	memberStale, lifetimeStale := strings.Repeat("c", 32), strings.Repeat("d", 32)
	owner := persistence.PersonalOwnerKey("local", purgeTestAccount, f.kid.ID)

	mA := o022Mutation(t, f.db, "binding-a")
	mB := o022Mutation(t, f.db, "binding-b")
	mOtherAccount := o022Mutation(t, f.db, "binding-other-account")
	mOtherProfile := o022Mutation(t, f.db, "binding-other-profile")

	private := o022PrivateHeader(t, f.db, owner, mA)
	o022SeedJob(t, f.db, o022JobSeed{
		m: mA, ownerAfter: owner, ownerValue: owner,
		account: purgeTestAccount, profile: f.kid.ID, membership: memberCurrent, lifetime: lifetimeCurrent,
		current: sql.NullInt64{Int64: private, Valid: true},
	})
	o022ReadingChild(t, f.db, private, "o022-reading-a")
	o022IdentityWitness(t, f.db, mA.Ref.ID, 1, "o022-identity-a", sql.NullInt64{Int64: private, Valid: true})
	o022IdentityWitness(t, f.db, mA.Ref.ID, 2, "o022-identity-b", sql.NullInt64{})
	// Same exact account/profile, a different membership incarnation and profile
	// lifetime: no-carry restore leaves the old catalog binding stale, and this
	// unit still finds it by the two-column prefix.
	o022SeedJob(t, f.db, o022JobSeed{
		m: mB, ownerAfter: owner, ownerValue: owner,
		account: purgeTestAccount, profile: f.kid.ID, membership: memberStale, lifetime: lifetimeStale,
		armZero: true,
	})
	o022SeedJob(t, f.db, o022JobSeed{
		m: mOtherAccount, account: "other-account", profile: f.kid.ID,
		membership: memberCurrent, lifetime: lifetimeCurrent,
	})
	o022SeedJob(t, f.db, o022JobSeed{
		m: mOtherProfile, account: purgeTestAccount, profile: "other-profile",
		membership: memberCurrent, lifetime: lifetimeCurrent,
	})

	cleared := func() map[string]bool {
		return map[string]bool{
			mA.Ref.ID: o022Cleared(t, f.db, mA.Ref.ID), mB.Ref.ID: o022Cleared(t, f.db, mB.Ref.ID),
			mOtherAccount.Ref.ID: o022Cleared(t, f.db, mOtherAccount.Ref.ID),
			mOtherProfile.Ref.ID: o022Cleared(t, f.db, mOtherProfile.Ref.ID),
		}
	}
	diff := func(before, after map[string]bool) int {
		n := 0
		for id, was := range before {
			if !was && after[id] {
				n++
			}
		}
		return n
	}

	before := cleared()
	clearedCount := 0
	for _, was := range before {
		if was {
			clearedCount++
		}
	}
	if clearedCount != 0 {
		t.Fatal("O022 binding fixture already has a cleared job")
	}
	for turn := 0; turn < 2; turn++ {
		out := o022Run(t, f.db, unit)
		if !out.Changed || out.Drained || out.Rewind {
			t.Fatalf("O022 binding turn %d: %+v", turn, out)
		}
		next := cleared()
		if n := diff(before, next); n != 1 {
			t.Fatalf("O022 binding turn %d reset %d jobs, want exactly one", turn, n)
		}
		before = next
	}
	if !before[mA.Ref.ID] || !before[mB.Ref.ID] {
		t.Fatalf("O022 stale binding did not reset both lifetimes: %+v", before)
	}
	if before[mOtherAccount.Ref.ID] || before[mOtherProfile.Ref.ID] {
		t.Fatalf("O022 exact account/profile binding escaped its scope: %+v", before)
	}

	out := o022Run(t, f.db, unit)
	if out.Changed || !out.Drained || out.Rewind {
		t.Fatalf("O022 emptied binding must drain: %+v", out)
	}
	if n := diff(before, cleared()); n != 0 {
		t.Fatalf("O022 binding rewrote %d already-cleared jobs", n)
	}

	// Neither job nor root is deleted or reinterpreted.
	for _, m := range []compactcatalog.EntityMutation{mA, mB} {
		row := o022ReadJob(t, f.db, m.Ref.ID)
		if row.mutation != m.Ref.ID || row.generation != m.Ref.Generation || row.plan != o022PlanDigestFixture {
			t.Fatalf("O022 reset lost mutation/generation/plan: %+v", row)
		}
		o022AssertCleared(t, row)
		if n := o022Count(t, f.db, `SELECT count(*) FROM catalog_entity_mutations WHERE mutation_id=? AND generation=?`, m.Ref.ID, m.Ref.Generation); n != 1 {
			t.Fatalf("O022 actual mutation root %q was removed", m.Ref.ID)
		}
	}

	// Private bookkeeping is custody, not progress: the reset drops only the
	// pointer and never publishes, deletes or retargets this header.
	var ready bool
	var captureMutation string
	var captureGeneration int64
	if err := f.db.QueryRow(`SELECT ready,capture_mutation_id,capture_generation FROM retained_title_state WHERE id=?`, private).
		Scan(&ready, &captureMutation, &captureGeneration); err != nil {
		t.Fatalf("O022 private header lost: %v", err)
	}
	if ready || captureMutation != mA.Ref.ID || captureGeneration != mA.Ref.Generation {
		t.Fatalf("O022 private header changed: ready=%v capture=%q generation=%d", ready, captureMutation, captureGeneration)
	}
	if n := o022Count(t, f.db, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, private); n != 1 {
		t.Fatal("O022 reset deleted a Reading child")
	}
	if n := o022Count(t, f.db, `SELECT count(*) FROM catalog_purge_identities WHERE mutation_id=?`, mA.Ref.ID); n != 2 {
		t.Fatal("O022 reset deleted identity witness rows")
	}
	if n := o022Count(t, f.db, `SELECT count(*) FROM catalog_purge_identities WHERE mutation_id=? AND previous_state_id=?`, mA.Ref.ID, private); n != 1 {
		t.Fatal("O022 reset rewrote an identity witness mapping")
	}

	// The unrelated bindings keep every owner and progress field.
	for _, m := range []compactcatalog.EntityMutation{mOtherAccount, mOtherProfile} {
		row := o022ReadJob(t, f.db, m.Ref.ID)
		if row.ownerAfter != "" || row.ownerValue != "" || row.state != "running" || row.captureArm != 4 ||
			row.localAccount == "" || row.localProfile == "" || row.cursor != "suppressed-o022" {
			t.Fatalf("O022 reset touched an out-of-scope binding %q: %+v", m.Ref.ID, row)
		}
	}

	t.Run("one_row_effect_before_owner_save", func(t *testing.T) {
		f := o022NewFixture(t)
		if _, err := f.s.RetireDirectProfile(context.Background(), f.login.AccountToken, f.kid.ID, ProfileRetirementRequest{
			ExpectedRevision: f.preview.Revision, Lifetime: f.preview.Lifetime, DeleteSavedResources: true,
		}); err != nil {
			t.Fatal(err)
		}
		job := o022SecurityJob(t, f.db, f.kid.ID)
		mutation := o022Mutation(t, f.db, "binding-cost")
		o022SeedJob(t, f.db, o022JobSeed{
			m: mutation, ownerAfter: owner, ownerValue: owner,
			account: purgeTestAccount, profile: f.kid.ID,
			membership: strings.Repeat("e", 32), lifetime: strings.Repeat("f", 32),
		})
		unit := o022Unit(t, f.s, job, "catalog-retained-capture-owner")
		if _, err := f.db.Exec(`UPDATE security_purge_jobs SET state='blocked' WHERE job_id=?`, job.id); err != nil {
			t.Fatal(err)
		}

		ctx := context.Background()
		tx, err := f.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		start := o022TotalChanges(t, tx)
		// Owner Before: exact target proof plus the optional metered
		// blocked->running transition.
		if err = purgeTargetTx(ctx, tx, job); err != nil {
			t.Fatalf("O022 target proof: %v", err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE security_purge_jobs SET state='running' WHERE job_id=? AND profile_incarnation=?`, job.id, job.profileLifetime); err != nil {
			t.Fatal(err)
		}
		beforeReset := o022TotalChanges(t, tx)
		turn, err := unit.Run(ctx, tx, nil)
		if err != nil || !turn.Changed {
			t.Fatalf("O022 reset turn: %+v %v", turn, err)
		}
		afterReset := o022TotalChanges(t, tx)
		if reset := afterReset - beforeReset; reset < 1 || reset > 1 {
			t.Fatalf("O022 reset changed %d rows before owner Save, want exactly one", reset)
		}
		if err = purgeSaveCursorTx(ctx, tx, job, phasedpurge.Cursor{Unit: 0}); err != nil {
			t.Fatalf("O022 owner Save: %v", err)
		}
		end := o022TotalChanges(t, tx)
		// Worst approved turn: one capture reset, one normal owner Save and one
		// optional blocked->running transition = 3 row effects, never 6.
		if total := end - start; total != 3 {
			t.Fatalf("O022 full turn cost %d row effects, want the documented <=3 (measured worst case 3)", total)
		}
	})
}

// TestProfilePurgeO022CompanionOwnerValueReset proves the companion owner-value
// unit matches only the canonical LOCAL key or the raw legacy profile id, with
// no residual local-binding filter, and handles arm0/no-header jobs as well as
// bindings that a previous reset already cleared.
func TestProfilePurgeO022CompanionOwnerValueReset(t *testing.T) {
	f := o022NewFixture(t)
	job := purgeTestJob(purgeTestAccount, f.kid.ID)
	unit := o022Unit(t, f.s, job, "catalog-retained-capture-owner-value")
	canonical := persistence.PersonalOwnerKey("local", purgeTestAccount, f.kid.ID)

	mCanonical := o022Mutation(t, f.db, "owner-canonical")
	mRaw := o022Mutation(t, f.db, "owner-raw")
	mAlready := o022Mutation(t, f.db, "owner-already")

	// arm0, no header, all four local fields empty: the canonical local frontier.
	o022SeedJob(t, f.db, o022JobSeed{
		m: mCanonical, ownerAfter: canonical, ownerValue: canonical, armZero: true,
	})
	// The same shape with the raw legacy profile id.
	o022SeedJob(t, f.db, o022JobSeed{
		m: mRaw, ownerAfter: f.kid.ID, ownerValue: f.kid.ID, armZero: true,
	})
	// A job whose bindings and owner frontier were already cleared.
	o022SeedJob(t, f.db, o022JobSeed{m: mAlready, clear: true})

	if n := o022Count(t, f.db, `SELECT count(*) FROM retained_title_state`); n != 0 {
		t.Fatalf("O022 companion fixture unexpectedly has a header: %d", n)
	}
	for _, m := range []compactcatalog.EntityMutation{mCanonical, mRaw} {
		row := o022ReadJob(t, f.db, m.Ref.ID)
		if row.captureArm != 0 || row.ownerStarted != 0 || row.sourceAfter != 0 || row.current.Valid {
			t.Fatalf("O022 arm0/no-header fixture is not arm0: %+v", row)
		}
	}
	if !o022Cleared(t, f.db, mAlready.Ref.ID) {
		t.Fatal("O022 already-cleared fixture is not cleared")
	}

	for turn := 0; turn < 2; turn++ {
		out := o022Run(t, f.db, unit)
		if !out.Changed || out.Drained || out.Rewind {
			t.Fatalf("O022 companion turn %d: %+v", turn, out)
		}
		changed := 0
		for _, id := range []string{mCanonical.Ref.ID, mRaw.Ref.ID, mAlready.Ref.ID} {
			if o022Cleared(t, f.db, id) {
				changed++
			}
		}
		if changed != turn+2 /* already-cleared seed plus each turn's one reset */ {
			t.Fatalf("O022 companion turn %d left %d cleared jobs, want %d", turn, changed, turn+2)
		}
	}
	out := o022Run(t, f.db, unit)
	if out.Changed || !out.Drained || out.Rewind {
		t.Fatalf("O022 companion with no frontier must drain: %+v", out)
	}

	for _, m := range []compactcatalog.EntityMutation{mCanonical, mRaw} {
		row := o022ReadJob(t, f.db, m.Ref.ID)
		if row.mutation != m.Ref.ID || row.generation != m.Ref.Generation || row.plan != o022PlanDigestFixture {
			t.Fatalf("O022 companion reset lost mutation/generation/plan: %+v", row)
		}
		o022AssertCleared(t, row)
		if n := o022Count(t, f.db, `SELECT count(*) FROM catalog_entity_mutations WHERE mutation_id=?`, m.Ref.ID); n != 1 {
			t.Fatalf("O022 companion reset removed root %q", m.Ref.ID)
		}
		if n := o022Count(t, f.db, `SELECT count(*) FROM retained_title_state`); n != 0 {
			t.Fatalf("O022 companion reset created a header: %d", n)
		}
	}
	// The already-cleared job is untouched, including its retained root.
	row := o022ReadJob(t, f.db, mAlready.Ref.ID)
	if row.ownerValue != "" || row.localProfile != "" || row.state != "pending" || row.plan != o022PlanDigestFixture {
		t.Fatalf("O022 companion rewrote an already-cleared binding: %+v", row)
	}
	if n := o022Count(t, f.db, `SELECT count(*) FROM catalog_entity_mutations WHERE mutation_id=?`, mAlready.Ref.ID); n != 1 {
		t.Fatal("O022 companion removed an already-cleared root")
	}
}

// TestProfilePurgeO022HostedOwnerNeverMatches proves a canonical Hosted owner
// tuple with identical embedded account/profile strings is a distinct frontier:
// neither O022 unit decodes it into a local binding or grants it authority.
func TestProfilePurgeO022HostedOwnerNeverMatches(t *testing.T) {
	f := o022NewFixture(t)
	job := purgeTestJob(purgeTestAccount, f.kid.ID)
	binding := o022Unit(t, f.s, job, "catalog-retained-capture-owner")
	companion := o022Unit(t, f.s, job, "catalog-retained-capture-owner-value")

	hosted := persistence.PersonalOwnerKey("hosted", purgeTestAccount, f.kid.ID)
	otherCanonical := persistence.PersonalOwnerKey("local", purgeTestAccount, "other-profile")
	otherRaw := "other-profile"
	canonical := persistence.PersonalOwnerKey("local", purgeTestAccount, f.kid.ID)

	mHosted := o022Mutation(t, f.db, "owner-hosted")
	mOtherCanonical := o022Mutation(t, f.db, "owner-other-canonical")
	mOtherRaw := o022Mutation(t, f.db, "owner-other-raw")
	mMatch := o022Mutation(t, f.db, "owner-match")

	o022SeedJob(t, f.db, o022JobSeed{m: mHosted, ownerAfter: hosted, ownerValue: hosted, armZero: true})
	o022SeedJob(t, f.db, o022JobSeed{m: mOtherCanonical, ownerAfter: otherCanonical, ownerValue: otherCanonical, armZero: true})
	o022SeedJob(t, f.db, o022JobSeed{m: mOtherRaw, ownerAfter: otherRaw, ownerValue: otherRaw, armZero: true})
	o022SeedJob(t, f.db, o022JobSeed{m: mMatch, ownerAfter: canonical, ownerValue: canonical, armZero: true})

	// The exact account/profile binding unit has no local fields to match.
	if out := o022Run(t, f.db, binding); out.Changed || !out.Drained {
		t.Fatalf("O022 binding matched an owner frontier without a local binding: %+v", out)
	}
	// The companion finds only the canonical LOCAL key of the retiring profile.
	for turn := 0; turn < 2; turn++ {
		out := o022Run(t, f.db, companion)
		if turn == 0 && !out.Changed {
			t.Fatalf("O022 companion did not reset the canonical LOCAL frontier: %+v", out)
		}
		if turn == 1 && (out.Changed || !out.Drained) {
			t.Fatalf("O022 companion did not stop after the one matching frontier: %+v", out)
		}
	}
	if !o022Cleared(t, f.db, mMatch.Ref.ID) {
		t.Fatal("O022 companion did not clear the canonical LOCAL match")
	}
	for _, kept := range []struct {
		name string
		id   string
		key  string
	}{
		{"canonical Hosted", mHosted.Ref.ID, hosted},
		{"other canonical local profile", mOtherCanonical.Ref.ID, otherCanonical},
		{"other raw local profile", mOtherRaw.Ref.ID, otherRaw},
	} {
		row := o022ReadJob(t, f.db, kept.id)
		if row.ownerValue != kept.key || row.ownerAfter != kept.key || row.state != "running" ||
			row.captureArm != 0 || row.readingStarted != 1 || row.identityAfter != 3 || row.current.Valid {
			t.Fatalf("O022 companion changed the %s frontier: %+v", kept.name, row)
		}
		if o022Cleared(t, f.db, kept.id) {
			t.Fatalf("O022 companion classified the %s as cleared", kept.name)
		}
		if n := o022Count(t, f.db, `SELECT count(*) FROM catalog_entity_mutations WHERE mutation_id=?`, kept.id); n != 1 {
			t.Fatalf("O022 companion removed the %s root", kept.name)
		}
	}
}

// TestProfilePurgeO022RetainedHeaderCustodyAndCounts drives the real
// retained-header unit with a private ready=0 header, a Reading child and real
// current_state_id custody. It proves the child keeps the header in custody,
// that removal changes at most the one UNIQUE job pointer, that the count tracks
// ready=1 headers only and ignores private ready=0 custody, and that the actual
// mutation root is held by RESTRICT until the private header is gone.
func TestProfilePurgeO022RetainedHeaderCustodyAndCounts(t *testing.T) {
	t.Run("pointer_counts_and_restrict", func(t *testing.T) {
		f := o022NewFixture(t)
		job := purgeTestJob(purgeTestAccount, f.kid.ID)
		childUnit := o022Unit(t, f.s, job, "retained-reading-progress")
		headerUnit := o022Unit(t, f.s, job, "retained-title-state")
		owner := persistence.PersonalOwnerKey("local", purgeTestAccount, f.kid.ID)

		mPrivate := o022Mutation(t, f.db, "header-private")
		mPublished := o022Mutation(t, f.db, "header-published")
		private := o022PrivateHeader(t, f.db, owner, mPrivate)
		published := o022PublishedHeader(t, f.db, owner)
		o022ReadingChild(t, f.db, private, "o022-header-child")

		// Two real catalog jobs, each with its own root, so the UNIQUE
		// current_state_id SET NULL can be observed: only the job pointing at the
		// removed header may change.
		o022SeedJob(t, f.db, o022JobSeed{
			m: mPrivate, ownerAfter: owner, ownerValue: owner,
			account: purgeTestAccount, profile: f.kid.ID,
			membership: strings.Repeat("a", 32), lifetime: strings.Repeat("b", 32),
			current: sql.NullInt64{Int64: private, Valid: true},
		})
		o022SeedJob(t, f.db, o022JobSeed{
			m: mPublished, ownerAfter: owner, ownerValue: owner,
			account: purgeTestAccount, profile: f.kid.ID,
			membership: strings.Repeat("c", 32), lifetime: strings.Repeat("d", 32),
			current: sql.NullInt64{Int64: published, Valid: true},
		})

		if n := o022Count(t, f.db, `SELECT count(*) FROM retained_title_state WHERE profile_id=?`, owner); n != 2 {
			t.Fatalf("O022 header fixture count %d", n)
		}
		// The approved count is the number of ready=1 headers only. The private
		// ready=0 header is custody, not published count.
		if n := o022Count(t, f.db, `SELECT title_count FROM retained_title_state_counts WHERE profile_id=?`, owner); n != 1 {
			t.Fatal("O022 count fixture did not count only the published header")
		}
		// current_state_id is UNIQUE: a second job cannot acquire the same header
		// pointer, which is what bounds SET NULL to one job.
		if _, err := f.db.Exec(`UPDATE catalog_purge_jobs SET current_state_id=? WHERE mutation_id=?`, private, mPublished.Ref.ID); err == nil ||
			!strings.Contains(err.Error(), "UNIQUE") {
			t.Fatalf("O022 current_state_id is not UNIQUE: %v", err)
		}

		// A private header with a Reading child still owns the mutation root and
		// blocks its removal.
		if _, err := f.db.Exec(`DELETE FROM catalog_entity_mutations WHERE mutation_id=?`, mPrivate.Ref.ID); err == nil ||
			!strings.Contains(err.Error(), "FOREIGN KEY") {
			t.Fatalf("O022 private header/root did not hold RESTRICT: %v", err)
		}
		out := o022Run(t, f.db, headerUnit)
		if out.Changed || out.Drained || !out.Rewind {
			t.Fatalf("O022 header with a Reading child must rewind: %+v", out)
		}
		if n := o022Count(t, f.db, `SELECT count(*) FROM retained_title_state WHERE id=?`, private); n != 1 {
			t.Fatal("O022 header was removed while its Reading child was present")
		}
		if out = o022Run(t, f.db, childUnit); !out.Changed {
			t.Fatalf("O022 Reading child did not drain: %+v", out)
		}
		if n := o022Count(t, f.db, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, private); n != 0 {
			t.Fatal("O022 Reading child survived its drain")
		}

		nullBefore := o022Count(t, f.db, `SELECT count(*) FROM catalog_purge_jobs WHERE current_state_id IS NULL`)
		for turn := 0; turn < 2; turn++ {
			if out = o022Run(t, f.db, headerUnit); !out.Changed {
				t.Fatalf("O022 header turn %d did not delete a drained header: %+v", turn, out)
			}
			nullAfter := o022Count(t, f.db, `SELECT count(*) FROM catalog_purge_jobs WHERE current_state_id IS NULL`)
			if changed := nullAfter - nullBefore; changed != 1 {
				t.Fatalf("O022 header turn %d changed %d job pointers, want at most the one UNIQUE pointer", turn, changed)
			}
			nullBefore = nullAfter

			// The count table covers ready=1 headers only; a ready=0 private
			// header never contributes. Compare against ready headers, and expect
			// no count row once no ready header remains, regardless of private
			// headers.
			publishedHeaders := o022Count(t, f.db, `SELECT count(*) FROM retained_title_state WHERE profile_id=? AND ready=1`, owner)
			var count int64
			err := f.db.QueryRow(`SELECT title_count FROM retained_title_state_counts WHERE profile_id=?`, owner).Scan(&count)
			if publishedHeaders == 0 {
				if err == nil || !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("O022 count row survived %d ready headers: %v", publishedHeaders, err)
				}
			} else {
				if err != nil {
					t.Fatalf("O022 count row missing for %d ready headers: %v", publishedHeaders, err)
				}
				if count != publishedHeaders {
					t.Fatalf("O022 count %d does not match %d ready headers", count, publishedHeaders)
				}
			}
		}
		if n := o022Count(t, f.db, `SELECT count(*) FROM retained_title_state WHERE profile_id=?`, owner); n != 0 {
			t.Fatalf("O022 headers remain after the private and published drain: %d", n)
		}
		if n := o022Count(t, f.db, `SELECT count(*) FROM catalog_purge_jobs WHERE current_state_id IS NOT NULL`); n != 0 {
			t.Fatal("O022 UNIQUE current_state_id pointers remain after header removal")
		}
		// Both catalog jobs and both actual roots survive: a SET NULL is not an
		// erasure of the mutation or of the job.
		for _, m := range []compactcatalog.EntityMutation{mPrivate, mPublished} {
			if n := o022Count(t, f.db, `SELECT count(*) FROM catalog_purge_jobs WHERE mutation_id=?`, m.Ref.ID); n != 1 {
				t.Fatalf("O022 header removal deleted job %q", m.Ref.ID)
			}
			if n := o022Count(t, f.db, `SELECT count(*) FROM catalog_entity_mutations WHERE mutation_id=?`, m.Ref.ID); n != 1 {
				t.Fatalf("O022 header removal deleted actual root %q", m.Ref.ID)
			}
		}

		// With both headers gone the actual private root is finally removable.
		if _, err := f.db.Exec(`DELETE FROM catalog_purge_jobs WHERE mutation_id=?`, mPrivate.Ref.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`DELETE FROM catalog_entity_mutations WHERE mutation_id=?`, mPrivate.Ref.ID); err != nil {
			t.Fatalf("O022 actual root was still held after the private header was removed: %v", err)
		}
	})

	t.Run("restrict_held_between_pointer_and_header", func(t *testing.T) {
		f := o022NewFixture(t)
		job := purgeTestJob(purgeTestAccount, f.kid.ID)
		headerUnit := o022Unit(t, f.s, job, "retained-title-state")
		owner := persistence.PersonalOwnerKey("local", purgeTestAccount, f.kid.ID)

		mutation := o022Mutation(t, f.db, "header-restrict")
		header := o022PrivateHeader(t, f.db, owner, mutation)
		var headerReady bool
		var headerCapture string
		if err := f.db.QueryRow(`SELECT ready,capture_mutation_id FROM retained_title_state WHERE id=?`, header).
			Scan(&headerReady, &headerCapture); err != nil {
			t.Fatal(err)
		}
		if headerReady || headerCapture != mutation.Ref.ID {
			t.Fatalf("O022 RESTRICT fixture is not a ready0 header of the actual root: ready=%v capture=%q", headerReady, headerCapture)
		}
		o022SeedJob(t, f.db, o022JobSeed{
			m: mutation, ownerAfter: owner, ownerValue: owner,
			account: purgeTestAccount, profile: f.kid.ID,
			membership: strings.Repeat("a", 32), lifetime: strings.Repeat("b", 32),
			current: sql.NullInt64{Int64: header, Valid: true},
		})

		if _, err := f.db.Exec(`DELETE FROM catalog_entity_mutations WHERE mutation_id=?`, mutation.Ref.ID); err == nil {
			t.Fatal("O022 actual root was removable while the job and private header held it")
		}
		if _, err := f.db.Exec(`DELETE FROM catalog_purge_jobs WHERE mutation_id=?`, mutation.Ref.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`DELETE FROM catalog_entity_mutations WHERE mutation_id=?`, mutation.Ref.ID); err == nil ||
			!strings.Contains(err.Error(), "FOREIGN KEY") {
			t.Fatalf("O022 ready0 header did not hold RESTRICT after the job pointer was removed: %v", err)
		}
		if out := o022Run(t, f.db, headerUnit); !out.Changed {
			t.Fatalf("O022 private header did not erase: %+v", out)
		}
		if _, err := f.db.Exec(`DELETE FROM catalog_entity_mutations WHERE mutation_id=?`, mutation.Ref.ID); err != nil {
			t.Fatalf("O022 actual root stayed held after the ready0 header was removed: %v", err)
		}
	})
}

// TestProfilePurgeO022PlanIndexesAndExactSQL pins the two approved units: their
// ordinal and placement before retained-reading-progress, the exact bounded
// candidate SQL, the two-column/owner-value indexes in the real query plan, and
// the absence of any residual local binding filter, suffix sort, offset or
// growing limit.
func TestProfilePurgeO022PlanIndexesAndExactSQL(t *testing.T) {
	f := o022NewFixture(t)
	job := purgeTestJob(purgeTestAccount, f.kid.ID)
	if purgeCatalogCaptureResetSet != o022ResetSet {
		t.Fatal("O022 reset SET list changed from the approved literal")
	}
	if purgePlanVersion != "p2-62-profile-purge-v7-o022" {
		t.Fatalf("O022 plan version %q does not name this ordinal", purgePlanVersion)
	}

	bindingIndex, ownerIndex, retainedIndex := -1, -1, -1
	for i := range profilePurgeRegistry {
		switch profilePurgeRegistry[i].name {
		case "catalog-retained-capture-owner":
			bindingIndex = i
		case "catalog-retained-capture-owner-value":
			ownerIndex = i
		case "retained-reading-progress":
			retainedIndex = i
		}
	}
	if bindingIndex < 0 || ownerIndex < 0 || retainedIndex < 0 {
		t.Fatalf("O022 units missing: binding=%d owner=%d retained=%d", bindingIndex, ownerIndex, retainedIndex)
	}
	if bindingIndex+1 != ownerIndex || ownerIndex+1 != retainedIndex {
		t.Fatalf("O022 units are not immediately before retained-reading-progress: %d,%d,%d", bindingIndex, ownerIndex, retainedIndex)
	}
	// The approved adapter raises this delete's read bound for the UNIQUE
	// current_state_id FK probe. Its two child guards must stay declared.
	header := &profilePurgeRegistry[retainedIndex+2]
	if header.name != "retained-title-state" || header.readBound != 9 || len(header.guards) != 2 || !header.rewind {
		t.Fatalf("O022 retained-title-state custody descriptor changed: %+v", header)
	}

	cases := []struct {
		name      string
		index     string
		readBound int
		predicate string
		extra     string
		compare   []string
		args      []any
	}{
		{
			name: "catalog-retained-capture-owner", index: "catalog_purge_jobs_local_binding", readBound: 2,
			predicate: "p.local_account=? AND p.local_profile=?",
			extra:     "p.local_profile<>''",
			compare:   []string{"local_account", "local_profile"},
			args:      []any{purgeTestAccount, f.kid.ID},
		},
		{
			name: "catalog-retained-capture-owner-value", index: "catalog_purge_jobs_owner_value", readBound: 3,
			predicate: "p.owner_value IN(?,?)",
			extra:     "p.owner_value<>''",
			compare:   []string{"owner_value"},
			args:      []any{purgeTestPersonal(f.kid.ID), f.kid.ID},
		},
	}
	for _, c := range cases {
		st := &profilePurgeRegistry[0]
		for i := range profilePurgeRegistry {
			if profilePurgeRegistry[i].name == c.name {
				st = &profilePurgeRegistry[i]
			}
		}
		if st.name != c.name || st.phase != 56 || st.table != "catalog_purge_jobs" || st.action != purgeScrub {
			t.Fatalf("O022 unit %s descriptor: %+v", c.name, st)
		}
		if st.readBound != c.readBound || st.extra != c.extra || st.set != o022ResetSet || st.keys != "mutation_id" {
			t.Fatalf("O022 unit %s cost/extra/set/keys: %+v", c.name, st)
		}
		if len(st.keyKinds) != 1 || st.keyKinds[0] != phasedpurge.Text || len(st.guards) != 0 || len(st.of) != 0 {
			t.Fatalf("O022 unit %s key kinds/guards/hops: %+v", c.name, st)
		}
		if args := job.purgeArgs(st); !reflect.DeepEqual(args, c.args) {
			t.Fatalf("O022 unit %s args %v, want %v", c.name, args, c.args)
		}

		query := purgeStepSQL(st, "")
		want := "UPDATE catalog_purge_jobs SET " + o022ResetSet + " WHERE mutation_id IN (SELECT mutation_id FROM (SELECT p.* FROM catalog_purge_jobs AS p WHERE " +
			c.predicate + " AND " + c.extra + " LIMIT 1) AS c) RETURNING mutation_id"
		if query != want {
			t.Fatalf("O022 unit %s emitted unexpected SQL:\n got %s\nwant %s", c.name, query, want)
		}
		// The candidate prefix and bound are exact: no residual binding filter,
		// offset, sort or variable limit can grow the scan.
		marker := "FROM catalog_purge_jobs AS p WHERE "
		start := strings.Index(query, marker)
		if start < 0 {
			t.Fatalf("O022 unit %s lost its raw candidate prefix", c.name)
		}
		rest := query[start+len(marker):]
		end := strings.Index(rest, " LIMIT 1")
		if end < 0 {
			t.Fatalf("O022 unit %s lost its bounded candidate suffix", c.name)
		}
		if got := rest[:end]; got != c.predicate+" AND "+c.extra {
			t.Fatalf("O022 unit %s candidate predicate %q", c.name, got)
		}
		if strings.Count(query, "LIMIT") != 1 || strings.Contains(query, "OFFSET") ||
			strings.Contains(query, "ORDER BY") || strings.Contains(query, "SORT") {
			t.Fatalf("O022 unit %s is not a single bounded prefix: %s", c.name, query)
		}
		if len(c.compare) == 1 && strings.Contains(rest[:end], "local_account") {
			t.Fatalf("O022 unit %s retained a local binding filter: %s", c.name, rest[:end])
		}
		if !purgeHasLeadingIndex(t, f.db, "catalog_purge_jobs", c.compare) {
			t.Fatalf("O022 unit %s index leading columns changed", c.name)
		}

		stmt, err := f.db.Prepare(query)
		if err != nil {
			t.Fatalf("O022 unit %s does not prepare: %v\n%s", c.name, err, query)
		}
		stmt.Close()
		rows, err := f.db.Query("EXPLAIN QUERY PLAN "+query, job.purgeArgs(st)...)
		if err != nil {
			t.Fatalf("O022 unit %s plan: %v", c.name, err)
		}
		searched := false
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
				rows.Close()
				t.Fatalf("O022 unit %s plan scan: %v", c.name, err)
			}
			if strings.Contains(detail, c.index) {
				searched = true
			}
			if strings.Contains(detail, "TEMP B-TREE") || strings.Contains(detail, "SORT") ||
				strings.HasPrefix(detail, "SCAN catalog_purge_jobs") || strings.HasPrefix(detail, "SCAN p") {
				t.Errorf("O022 unit %s plans a scan/sort: %s", c.name, detail)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatalf("O022 unit %s plan rows: %v", c.name, err)
		}
		if !searched {
			t.Fatalf("O022 unit %s does not use %s", c.name, c.index)
		}
	}
}
