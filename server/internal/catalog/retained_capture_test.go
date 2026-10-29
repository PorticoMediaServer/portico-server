package catalog

// P2-59 / O-022 retained capture tests.
//
// These tests run against the real installed baseline. They create the job row
// and its fixed identity facts as fixtures (the caller/root does that in
// production), then drive captureRetainedOwnerStepTx through its real public
// writer path. They never install a schema stub, never forge capture progress
// (capture_arm/identity_after/reading cursors are only observed), and never
// fabricate a counter or phase row.
//
// The gate DDL is not installed yet, so this file is source-reviewed only. It
// will run once the root applies catalog_purge_jobs, catalog_purge_identities,
// the ready/capture columns and the replaced count/ready triggers.
//
// The same-writer late-write fence is NOT tested here: it is the caller/root's
// producer-fence census, and inventing a local fence in this file would claim
// custody this component does not own.

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/readingtest"
)

type captureFixture struct {
	c                      *catalogtest.Catalog
	m                      compactcatalog.EntityMutation
	item                   int64
	token                  string
	owner                  string
	account, profile       string
	lifetime, membership   string
	root                   string
}

// newCaptureFixture builds a real reading title and a real admitted removal
// mutation through the production writers.
func newCaptureFixture(t *testing.T) *captureFixture {
	t.Helper()
	c := catalogtest.Open(t)
	f := &captureFixture{c: c, account: "capture-account", profile: "capture-profile"}
	c.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id) VALUES(?,?,X'00',?)`, f.account, f.account, f.profile)
	if err := c.DB.QueryRow(`SELECT p.lifetime,m.incarnation FROM direct_profiles p
 JOIN direct_memberships m ON m.account_id=p.account_id WHERE p.account_id=? AND p.id=?`, f.account, f.profile).
		Scan(&f.lifetime, &f.membership); err != nil {
		t.Fatal(err)
	}
	f.owner = persistence.PersonalOwnerKey("local", f.account, f.profile)
	f.root = t.TempDir()
	library := c.Library("reading", "Reading", "book", f.root)
	f.item, f.token = captureAddPDF(t, c, library, 0, f.root, "original.pdf")
	var ref compactcatalog.MutationRef
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		ref, err = compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{
			Entity: f.item, OwnerKind: "administration", OwnerID: "capture-fixture", OwnerGeneration: 1, Mode: "remove",
		})
		if err != nil {
			return err
		}
		f.m, err = compactcatalog.ReadEntityMutationTx(ctx, tx, ref)
		return err
	})
	return f
}

// captureAddPDF writes one real publication file and links it to the item (or
// creates the item when item==0) through the production writers.
func captureAddPDF(t *testing.T, c *catalogtest.Catalog, library, item int64, root, name string) (int64, string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, readingtest.PDF(), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var asset int64
	var token string
	var newItem int64
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var e error
		asset, token, e = compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{
			Path: path, Size: info.Size(), ModifiedNS: info.ModTime().UnixNano(), Container: "pdf",
		})
		if e != nil {
			return e
		}
		if item == 0 {
			newItem, e = attachReadingTx(ctx, tx, "reading", "book", token, path)
			return e
		}
		newItem = item
		if e = compactcatalog.LinkAssetTx(ctx, tx, item, asset, compactcatalog.Link{}); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO reading_publications(item_id,asset_id,format) VALUES(?,?,'pdf')`, item, asset)
		return e
	})
	return newItem, token
}

func captureAssetID(t *testing.T, c *catalogtest.Catalog, token string) int64 {
	t.Helper()
	var id int64
	if err := c.DB.QueryRow(`SELECT id FROM catalog_assets WHERE token=?`, token).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// captureSeedJob inserts the real job row and its fixed <=12 identity facts.
// This is caller/root fixture setup, not capture progress.
func captureSeedJob(t *testing.T, c *catalogtest.Catalog, mutation string, generation int64, owner string, identities []string, local bool, account, profile, lifetime, membership string) {
	t.Helper()
	localAccount, localProfile, localLifetime, localMembership := "", "", "", ""
	if local {
		localAccount, localProfile, localLifetime, localMembership = account, profile, lifetime, membership
	}
	c.Exec(`INSERT INTO catalog_purge_jobs(mutation_id,generation,plan_digest,cursor,owner_after,owner_value,
 owner_started,capture_arm,source_after,state,attempt,next_attempt_ms,reason,current_state_id,
 reading_arm,reading_started,reading_asset_after,reading_state_after,reading_source_after,identity_after,
 local_account,local_profile,local_membership_incarnation,local_profile_lifetime)
 VALUES(?,?,?,'',?,?,1,0,0,'running',0,0,'',NULL,'current',0,0,0,'',0,?,?,?,?)`,
		mutation, generation, strings.Repeat("a", 64), owner, owner,
		localAccount, localProfile, localMembership, localLifetime)
	for i, identity := range identities {
		c.Exec(`INSERT INTO catalog_purge_identities(mutation_id,ordinal,identity) VALUES(?,?,?)`, mutation, i+1, identity)
	}
}

type captureChild struct {
	token, revision string
	observed       int64
	progression    float64
}

func captureOldState(t *testing.T, c *catalogtest.Catalog, owner string, identities []string, children []captureChild) int64 {
	t.Helper()
	var id int64
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `INSERT INTO retained_title_state(profile_id,kind,title,retained_ms) VALUES(?,?,?,?) RETURNING id`,
			owner, int(compactcatalog.Book), "Old retained title", 1000).Scan(&id); err != nil {
			return err
		}
		for _, identity := range identities {
			if _, err := tx.ExecContext(ctx, `INSERT INTO retained_title_identities(identity,profile_id,state_id) VALUES(?,?,?)`, identity, owner, id); err != nil {
				return err
			}
		}
		for _, child := range children {
			if _, err := tx.ExecContext(ctx, `INSERT INTO retained_reading_progress(state_id,source_id,source_revision,format,page_index,progression,revision,observed_at)
 VALUES(?,?,?,'pdf',0,?,1,?)`, id, child.token, child.revision, child.progression, child.observed); err != nil {
				return err
			}
		}
		return nil
	})
	return id
}

func captureCurrentPosition(t *testing.T, c *catalogtest.Catalog, owner string, item, asset int64, revision string, progression float64, observed int64) {
	t.Helper()
	c.Exec(`INSERT INTO reading_progress(owner_key,item_id,asset_id,source_revision,format,page_index,progression,revision,observed_at)
 VALUES(?,?,?,?,'pdf',0,?,1,?)`, owner, item, asset, revision, progression, observed)
}

// captureDrive runs the real step until publication, a terminal finish or an
// unexpected error. Sentinels are no-write outcomes and are not rolled back.
func captureDrive(t *testing.T, c *catalogtest.Catalog, m compactcatalog.EntityMutation, max int) (bool, int) {
	t.Helper()
	for i := 0; i < max; i++ {
		var published bool
		var stepErr error
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			published, stepErr = captureRetainedOwnerStepTx(ctx, tx, m)
			if stepErr == nil {
				return nil
			}
			if errors.Is(stepErr, errRetainedCaptureRoot) || errors.Is(stepErr, errRetainedCaptureState) || errors.Is(stepErr, errRetainedCaptureFinished) {
				// No write was made by a sentinel outcome.
				return nil
			}
			return stepErr
		})
		if stepErr != nil {
			switch {
			case errors.Is(stepErr, errRetainedCaptureFinished):
				return false, i
			case errors.Is(stepErr, errRetainedCaptureRoot), errors.Is(stepErr, errRetainedCaptureState):
				t.Fatalf("capture premise refused at step %d: %v", i, stepErr)
			}
			t.Fatalf("capture step %d: %v", i, stepErr)
		}
		if published {
			return true, i + 1
		}
	}
	t.Fatalf("capture did not finish within %d steps", max)
	return false, 0
}

func captureStepOnce(t *testing.T, c *catalogtest.Catalog, m compactcatalog.EntityMutation) (bool, bool, error) {
	t.Helper()
	var published, progressed bool
	var stepErr error
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		published, stepErr = captureRetainedOwnerStepTx(ctx, tx, m)
		if stepErr == nil {
			progressed = true
			return nil
		}
		if errors.Is(stepErr, errRetainedCaptureRoot) || errors.Is(stepErr, errRetainedCaptureState) || errors.Is(stepErr, errRetainedCaptureFinished) {
			// Sentinels make no write and must not roll back anything.
			return nil
		}
		return stepErr
	})
	return published, progressed, stepErr
}

func captureJob(t *testing.T, c *catalogtest.Catalog, mutation string) (arm int, ownerStarted bool, state, reason, ownerValue, lifetime string, current sql.NullInt64, identityAfter int) {
	t.Helper()
	if err := c.DB.QueryRow(`SELECT capture_arm,owner_started,state,reason,owner_value,local_profile_lifetime,current_state_id,identity_after
 FROM catalog_purge_jobs WHERE mutation_id=?`, mutation).
		Scan(&arm, &ownerStarted, &state, &reason, &ownerValue, &lifetime, &current, &identityAfter); err != nil {
		t.Fatal(err)
	}
	return
}

func captureCount(t *testing.T, c *catalogtest.Catalog, query string, args ...any) int {
	t.Helper()
	var n int
	if err := c.DB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%v: %s", err, query)
	}
	return n
}

// TestRetainedCapturePublishesOneCompleteOwner is the primary shape: a private
// ready=0 header is built, current positions win equal observed_at, prior
// retained children merge only when strictly newer, all identity mappings move
// in one publication, the count is maintained, and no old header is deleted.
func TestRetainedCapturePublishesOneCompleteOwner(t *testing.T) {
	f := newCaptureFixture(t)
	_, newerToken := captureAddPDF(t, f.c, 0, f.item, f.root, "second.pdf")
	// Two current positions: equal timestamp on the first, older than the prior
	// retained child on the second.
	captureCurrentPosition(t, f.c, f.owner, f.item, captureAssetID(t, f.c, f.token), "current-one", .2, 100)
	captureCurrentPosition(t, f.c, f.owner, f.item, captureAssetID(t, f.c, newerToken), "current-two", .8, 200)
	identities := []string{"local:/reading|book:capture", "provider:tmdb|series|77"}
	old := captureOldState(t, f.c, f.owner, identities[1:], []captureChild{
		{token: f.token, revision: "old-equal", observed: 100, progression: .9},
		{token: newerToken, revision: "old-newer", observed: 300, progression: .7},
	})
	captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, identities, true, f.account, f.profile, f.lifetime, f.membership)

	published, _ := captureDrive(t, f.c, f.m, 64)
	if !published {
		t.Fatal("owner capture did not publish")
	}
	var newID int64
	if err := f.c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identities[0], f.owner).Scan(&newID); err != nil {
		t.Fatal(err)
	}
	var ready bool
	var captureMutation sql.NullString
	var generation int64
	if err := f.c.DB.QueryRow(`SELECT ready,capture_mutation_id,capture_generation FROM retained_title_state WHERE id=?`, newID).Scan(&ready, &captureMutation, &generation); err != nil {
		t.Fatal(err)
	}
	if !ready || captureMutation.Valid || generation != 0 {
		t.Fatalf("published header ready=%v capture=%v generation=%d", ready, captureMutation, generation)
	}
	// Both identities point at the one complete state.
	for _, identity := range identities {
		var got int64
		if err := f.c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identity, f.owner).Scan(&got); err != nil || got != newID {
			t.Fatalf("identity %q mapping=%d want %d err=%v", identity, got, newID, err)
		}
	}
	// Current-first merge rules.
	var currentProgression, oldProgression float64
	if err := f.c.DB.QueryRow(`SELECT progression FROM retained_reading_progress WHERE state_id=? AND source_id=?`, newID, f.token).Scan(&currentProgression); err != nil || currentProgression != .2 {
		t.Fatalf("equal observed_at must keep the current position: %v %v", currentProgression, err)
	}
	if err := f.c.DB.QueryRow(`SELECT progression FROM retained_reading_progress WHERE state_id=? AND source_id=?`, newID, newerToken).Scan(&oldProgression); err != nil || oldProgression != .7 {
		t.Fatalf("strictly newer prior child must merge: %v %v", oldProgression, err)
	}
	// No old header deletion at publication; the count stays truthful.
	if n := captureCount(t, f.c, `SELECT count(*) FROM retained_title_state WHERE id=? AND ready=1`, old); n != 1 {
		t.Fatal("old published header was deleted at publication")
	}
	if n := captureCount(t, f.c, `SELECT title_count FROM retained_title_state_counts WHERE profile_id=?`, f.owner); n != 2 {
		t.Fatalf("maintained count=%d want 2 (old header plus published header)", n)
	}
	// The private header never carried a published mapping.
	if n := captureCount(t, f.c, `SELECT count(*) FROM retained_title_identities WHERE state_id=?`, old); n != 1 {
		t.Fatalf("old identity mapping lost: %d", n)
	}
	arm, ownerStarted, state, _, ownerValue, _, current, _ := captureJob(t, f.c, f.m.Ref.ID)
	if arm != retainedCaptureArmPublished || ownerStarted || state != "running" || ownerValue != f.owner || current.Valid {
		t.Fatalf("job advancement arm=%d started=%v state=%q owner=%q current=%v", arm, ownerStarted, state, ownerValue, current)
	}
}

// TestRetainedCaptureHasNoReadingVersionCeiling drives more than 64 real
// Reading versions through one owner; a supported title is not refused.
func TestRetainedCaptureHasNoReadingVersionCeiling(t *testing.T) {
	f := newCaptureFixture(t)
	const versions = 65
	tokens := []string{f.token}
	for i := 1; i < versions; i++ {
		_, token := captureAddPDF(t, f.c, 0, f.item, f.root, "version-"+strconv.Itoa(i)+".pdf")
		tokens = append(tokens, token)
		captureCurrentPosition(t, f.c, f.owner, f.item, captureAssetID(t, f.c, token), "revision-"+strconv.Itoa(i), float64(i)/100, int64(1000+i))
	}
	captureCurrentPosition(t, f.c, f.owner, f.item, captureAssetID(t, f.c, tokens[0]), "revision-0", .01, 1000)
	identities := []string{"local:/reading|book:many"}
	captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, identities, true, f.account, f.profile, f.lifetime, f.membership)
	published, steps := captureDrive(t, f.c, f.m, versions*3)
	if !published {
		t.Fatal("many-version owner did not publish")
	}
	if steps <= versions/2 {
		t.Fatalf("capture published after %d steps; the fixture did not exercise the per-version walk", steps)
	}
	arm, _, _, _, _, _, _, _ := captureJob(t, f.c, f.m.Ref.ID)
	if arm != retainedCaptureArmPublished {
		t.Fatalf("capture_arm=%d", arm)
	}
	var newID int64
	if err := f.c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identities[0], f.owner).Scan(&newID); err != nil {
		t.Fatal(err)
	}
	if n := captureCount(t, f.c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, newID); n != versions {
		t.Fatalf("published %d of %d real Reading versions", n, versions)
	}
}

// TestRetainedCaptureCrashMidCopyKeepsOldPublishedState stops in the middle of
// copying prior retained children, reopens the database, then resumes. The old
// published state is never modified by a partial private copy.
//
// The crash window is between committed steps: a failure inside one step is the
// caller's rollback, while a process crash after a step is the durable state a
// restart must resume from. Both leave the old published header and mapping
// untouched; this test proves the latter.
func TestRetainedCaptureCrashMidCopyKeepsOldPublishedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture-crash.sqlite")
	db, err := persistence.OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	c := catalogtest.New(t, db)
	f := &captureFixture{c: c, account: "capture-crash", profile: "capture-crash-profile"}
	c.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id) VALUES(?,?,X'00',?)`, f.account, f.account, f.profile)
	if err = c.DB.QueryRow(`SELECT p.lifetime,m.incarnation FROM direct_profiles p
 JOIN direct_memberships m ON m.account_id=p.account_id WHERE p.account_id=? AND p.id=?`, f.account, f.profile).Scan(&f.lifetime, &f.membership); err != nil {
		t.Fatal(err)
	}
	f.owner = persistence.PersonalOwnerKey("local", f.account, f.profile)
	f.root = t.TempDir()
	library := c.Library("reading", "Reading", "book", f.root)
	f.item, f.token = captureAddPDF(t, c, library, 0, f.root, "original.pdf")
	_, oldToken := captureAddPDF(t, c, 0, f.item, f.root, "old.pdf")
	captureCurrentPosition(t, c, f.owner, f.item, captureAssetID(t, c, f.token), "current", .1, 10)
	identity := "local:/reading|book:crash"
	old := captureOldState(t, c, f.owner, []string{identity}, []captureChild{{token: oldToken, revision: "old", observed: 20, progression: .9}})
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		ref, e := compactcatalog.BeginEntityMutationTx(ctx, tx, compactcatalog.MutationIntent{
			Entity: f.item, OwnerKind: "administration", OwnerID: "capture-crash", OwnerGeneration: 1, Mode: "remove",
		})
		if e != nil {
			return e
		}
		f.m, e = compactcatalog.ReadEntityMutationTx(ctx, tx, ref)
		return e
	})
	captureSeedJob(t, c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, []string{identity}, true, f.account, f.profile, f.lifetime, f.membership)

	// Walk until the private header has copied the current position and at least
	// one prior retained child, then stop without any further step.
	var privateID int64
	for i := 0; i < 32; i++ {
		published, progressed, stepErr := captureStepOnce(t, c, f.m)
		if stepErr != nil || !progressed {
			t.Fatalf("step %d: published=%v progressed=%v err=%v", i, published, progressed, stepErr)
		}
		if published {
			t.Fatal("published before the crash point")
		}
		var current sql.NullInt64
		if err = c.DB.QueryRow(`SELECT current_state_id FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if current.Valid {
			privateID = current.Int64
			if n := captureCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, privateID); n >= 2 {
				break
			}
		}
	}
	if privateID == 0 {
		t.Fatal("never reached the mid-copy point")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = persistence.OpenFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c = catalogtest.New(t, db)

	// The old published state and its mapping are intact; the private header is
	// still private and unreferenced.
	var ready bool
	if err = c.DB.QueryRow(`SELECT ready FROM retained_title_state WHERE id=?`, old).Scan(&ready); err != nil || !ready {
		t.Fatalf("old published header changed: ready=%v err=%v", ready, err)
	}
	var mapped int64
	if err = c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identity, f.owner).Scan(&mapped); err != nil || mapped != old {
		t.Fatalf("old mapping changed: %d want %d err=%v", mapped, old, err)
	}
	if n := captureCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, old); n != 1 {
		t.Fatalf("old children changed: %d", n)
	}
	if ready, _ = captureJobHeaderReady(t, c, privateID); ready {
		t.Fatal("partial private header became ready")
	}
	if n := captureCount(t, c, `SELECT count(*) FROM retained_title_identities WHERE state_id=?`, privateID); n != 0 {
		t.Fatal("partial private header got a published mapping")
	}

	published, _ := captureDrive(t, c, f.m, 64)
	if !published {
		t.Fatal("resumed capture did not publish")
	}
	if n := captureCount(t, c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, privateID); n != 2 {
		t.Fatalf("resumed merge copied %d children, want current plus prior", n)
	}
}

func captureJobHeaderReady(t *testing.T, c *catalogtest.Catalog, id int64) (bool, error) {
	t.Helper()
	var ready bool
	err := c.DB.QueryRow(`SELECT ready FROM retained_title_state WHERE id=?`, id).Scan(&ready)
	return ready, err
}

// TestRetainedCaptureMappingDriftResetsPrivateCopy replaces the prior mapping
// with another library's header mid-copy. The capture must reset, never publish
// the stale merge and never delete the new mapping.
func TestRetainedCaptureMappingDriftResetsPrivateCopy(t *testing.T) {
	f := newCaptureFixture(t)
	_, aToken := captureAddPDF(t, f.c, 0, f.item, f.root, "a.pdf")
	_, bToken := captureAddPDF(t, f.c, 0, f.item, f.root, "b.pdf")
	captureCurrentPosition(t, f.c, f.owner, f.item, captureAssetID(t, f.c, f.token), "current", .1, 10)
	identity := "local:/reading|book:drift"
	a := captureOldState(t, f.c, f.owner, []string{identity}, []captureChild{{token: aToken, revision: "a", observed: 20, progression: .2}})
	b := captureOldState(t, f.c, f.owner, nil, []captureChild{{token: bToken, revision: "b", observed: 30, progression: .3}})
	captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, []string{identity}, true, f.account, f.profile, f.lifetime, f.membership)

	// Drive until the witness captured the old mapping and at least one of its
	// children was copied, then let another title/library replace the mapping.
	var privateID int64
	for i := 0; i < 16; i++ {
		published, _, stepErr := captureStepOnce(t, f.c, f.m)
		if stepErr != nil || published {
			t.Fatalf("step %d published=%v err=%v", i, published, stepErr)
		}
		var captured int
		var current sql.NullInt64
		if err := f.c.DB.QueryRow(`SELECT mapping_captured FROM catalog_purge_identities WHERE mutation_id=? AND ordinal=1`, f.m.Ref.ID).Scan(&captured); err != nil {
			t.Fatal(err)
		}
		if err := f.c.DB.QueryRow(`SELECT current_state_id FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if current.Valid {
			privateID = current.Int64
		}
		if captured == 1 && privateID != 0 && captureCount(t, f.c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=? AND source_id=?`, privateID, aToken) == 1 {
			break
		}
	}
	f.c.Exec(`UPDATE retained_title_identities SET state_id=? WHERE identity=? AND profile_id=?`, b, identity, f.owner)
	// The replacement is a different title's header; the capture must not touch it.
	if n := captureCount(t, f.c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, b); n != 1 {
		t.Fatal("new mapping's children changed before publication")
	}

	sawReset := false
	published, _ := captureDriveObserved(t, f.c, f.m, 96, func() {
		if arm, _, _, _, _, _, _, _ := captureJob(t, f.c, f.m.Ref.ID); arm == retainedCaptureArmReset {
			sawReset = true
		}
	})
	if !sawReset {
		t.Fatal("mapping drift did not reset private construction")
	}
	if !published {
		t.Fatal("capture did not republish after the drift reset")
	}
	var newID int64
	if err := f.c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identity, f.owner).Scan(&newID); err != nil {
		t.Fatal(err)
	}
	if newID == a {
		t.Fatal("published a stale merge onto the old mapping")
	}
	if newID == b {
		t.Fatal("published directly onto another title's header")
	}
	if n := captureCount(t, f.c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=? AND source_id=?`, newID, aToken); n != 0 {
		t.Fatal("stale prior child survived publication")
	}
	if n := captureCount(t, f.c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=? AND source_id=?`, newID, bToken); n != 1 {
		t.Fatal("post-reset capture did not use the replacement mapping's header")
	}
	if n := captureCount(t, f.c, `SELECT count(*) FROM retained_title_identities WHERE identity=? AND profile_id=? AND state_id=?`, identity, f.owner, b); n != 0 {
		t.Fatal("new mapping still points at the replaced header")
	}
	// The replacement header itself is untouched (not deleted by drift).
	if n := captureCount(t, f.c, `SELECT count(*) FROM retained_title_state WHERE id=? AND ready=1`, b); n != 1 {
		t.Fatal("drift deleted the new mapping's header")
	}
}

// captureDriveObserved is captureDrive plus an observer between steps.
func captureDriveObserved(t *testing.T, c *catalogtest.Catalog, m compactcatalog.EntityMutation, max int, observe func()) (bool, int) {
	t.Helper()
	for i := 0; i < max; i++ {
		if observe != nil {
			observe()
		}
		published, _, stepErr := captureStepOnce(t, c, m)
		if stepErr != nil {
			if errors.Is(stepErr, errRetainedCaptureFinished) {
				return false, i
			}
			t.Fatalf("capture step %d: %v", i, stepErr)
		}
		if published {
			return true, i + 1
		}
	}
	t.Fatalf("capture did not finish within %d steps", max)
	return false, 0
}

// TestRetainedCaptureBindingRotationAndDestruction covers both no-carry
// outcomes: a rotated lifetime drains and recaptures against the exact current
// binding, while a destroyed profile drains and visibly refuses without ever
// falling back to a Hosted/opaque copy or a complete claim.
func TestRetainedCaptureBindingRotationAndDestruction(t *testing.T) {
	t.Run("rotation refreshes the exact current binding", func(t *testing.T) {
		f := newCaptureFixture(t)
		captureCurrentPosition(t, f.c, f.owner, f.item, captureAssetID(t, f.c, f.token), "current", .4, 40)
		identity := "local:/reading|book:rotation"
		captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, []string{identity}, true, f.account, f.profile, f.lifetime, f.membership)
		// Build the private header, then rotate the profile lifetime.
		for i := 0; i < 2; i++ {
			if _, _, err := captureStepOnce(t, f.c, f.m); err != nil {
				t.Fatal(err)
			}
		}
		f.c.Exec(`UPDATE direct_profiles SET lifetime=lower(hex(randomblob(16))) WHERE account_id=? AND id=?`, f.account, f.profile)
		var newLifetime string
		if err := f.c.DB.QueryRow(`SELECT lifetime FROM direct_profiles WHERE account_id=? AND id=?`, f.account, f.profile).Scan(&newLifetime); err != nil {
			t.Fatal(err)
		}
		sawReset := false
		published, _ := captureDriveObserved(t, f.c, f.m, 96, func() {
			if arm, _, _, _, _, _, _, _ := captureJob(t, f.c, f.m.Ref.ID); arm == retainedCaptureArmReset {
				sawReset = true
			}
		})
		if !sawReset {
			t.Fatal("rotated binding did not reset the private copy")
		}
		if !published {
			t.Fatal("rotated binding did not recapture against the current lifetime")
		}
		_, ownerStarted, state, _, ownerValue, jobLifetime, _, _ := captureJob(t, f.c, f.m.Ref.ID)
		if ownerStarted || state != "running" || ownerValue != f.owner || jobLifetime != newLifetime {
			t.Fatalf("job did not adopt the current exact binding: started=%v state=%q owner=%q lifetime=%q want %q",
				ownerStarted, state, ownerValue, jobLifetime, newLifetime)
		}
	})

	t.Run("destroyed profile refuses instead of a Hosted copy", func(t *testing.T) {
		f := newCaptureFixture(t)
		captureCurrentPosition(t, f.c, f.owner, f.item, captureAssetID(t, f.c, f.token), "current", .4, 40)
		identity := "local:/reading|book:destroyed"
		captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, []string{identity}, true, f.account, f.profile, f.lifetime, f.membership)
		for i := 0; i < 2; i++ {
			if _, _, err := captureStepOnce(t, f.c, f.m); err != nil {
				t.Fatal(err)
			}
		}
		f.c.Exec(`UPDATE direct_profiles SET deleted=1 WHERE account_id=? AND id=?`, f.account, f.profile)
		published, _ := captureDriveObserved(t, f.c, f.m, 64, nil)
		if published {
			t.Fatal("destroyed binding published a retained state")
		}
		arm, ownerStarted, state, reason, ownerValue, jobLifetime, current, _ := captureJob(t, f.c, f.m.Ref.ID)
		if arm != retainedCaptureArmFinished || ownerStarted || state != "blocked" || reason != "binding" || current.Valid {
			t.Fatalf("refusal arm=%d started=%v state=%q reason=%q current=%v", arm, ownerStarted, state, reason, current)
		}
		// No Hosted fallback and no complete claim: owner identity and exact old
		// binding are preserved, no ready header exists, and no public count moved.
		if ownerValue != f.owner || jobLifetime != f.lifetime {
			t.Fatalf("binding/owner was rewritten: owner=%q lifetime=%q", ownerValue, jobLifetime)
		}
		if n := captureCount(t, f.c, `SELECT count(*) FROM retained_title_state WHERE profile_id=? AND ready=1`, f.owner); n != 0 {
			t.Fatalf("destroyed binding left %d published rows", n)
		}
		if n := captureCount(t, f.c, `SELECT count(*) FROM retained_title_state WHERE capture_mutation_id=?`, f.m.Ref.ID); n != 0 {
			t.Fatalf("private header survived refusal: %d", n)
		}
		if n := captureCount(t, f.c, `SELECT count(*) FROM retained_title_state_counts WHERE profile_id=?`, f.owner); n != 0 {
			t.Fatal("private construction touched the maintained count")
		}
	})
}

// TestRetainedCaptureMissingHeaderIsNotCompletion erases the private header
// (profile-erasure style: ON DELETE SET NULL clears the job pointer) and proves
// the owner is revalidated and recaptured, never published as complete.
func TestRetainedCaptureMissingHeaderIsNotCompletion(t *testing.T) {
	f := newCaptureFixture(t)
	captureCurrentPosition(t, f.c, f.owner, f.item, captureAssetID(t, f.c, f.token), "current", .5, 50)
	identity := "local:/reading|book:missing"
	captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, []string{identity}, true, f.account, f.profile, f.lifetime, f.membership)
	for i := 0; i < 2; i++ {
		if _, _, err := captureStepOnce(t, f.c, f.m); err != nil {
			t.Fatal(err)
		}
	}
	_, _, _, _, _, _, current, _ := captureJob(t, f.c, f.m.Ref.ID)
	if !current.Valid {
		t.Fatal("fixture did not build a private header")
	}
	deleted := current.Int64
	// Delete the private header; the UNIQUE current_state_id FK SET NULLs the
	// job pointer exactly like profile erasure.
	f.c.Exec(`DELETE FROM retained_title_state WHERE id=?`, deleted)
	arm, _, _, _, _, _, pointer, _ := captureJob(t, f.c, f.m.Ref.ID)
	if arm == retainedCaptureArmPublished || pointer.Valid {
		t.Fatalf("fixture state after erase arm=%d pointer=%v", arm, pointer)
	}
	sawReset := false
	published, _ := captureDriveObserved(t, f.c, f.m, 64, func() {
		if a, _, _, _, _, _, _, _ := captureJob(t, f.c, f.m.Ref.ID); a == retainedCaptureArmReset {
			sawReset = true
		}
	})
	if !sawReset {
		t.Fatal("missing header was not treated as an invalidation")
	}
	if !published {
		t.Fatal("owner did not recapture after the missing header")
	}
	var newID int64
	if err := f.c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identity, f.owner).Scan(&newID); err != nil {
		t.Fatal(err)
	}
	if newID == deleted {
		t.Fatal("missing header was reused as complete")
	}
	if n := captureCount(t, f.c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, newID); n != 1 {
		t.Fatalf("recaptured header has %d children, want the current position", n)
	}
}

// TestRetainedCaptureRefusesMalformedPremises covers the caller-owned premises
// and the zero-identity and opaque-owner shapes.
func TestRetainedCaptureRefusesMalformedPremises(t *testing.T) {
	t.Run("more than twelve identity facts", func(t *testing.T) {
		f := newCaptureFixture(t)
		identities := make([]string, 0, 13)
		for i := 0; i < 13; i++ {
			identities = append(identities, "local:/reading|book:"+strconv.Itoa(i))
		}
		captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, identities, true, f.account, f.profile, f.lifetime, f.membership)
		_, _, err := captureStepOnce(t, f.c, f.m)
		if !errors.Is(err, errRetainedCaptureState) {
			t.Fatalf("13 identities: %v", err)
		}
		if n := captureCount(t, f.c, `SELECT count(*) FROM retained_title_state WHERE capture_mutation_id=?`, f.m.Ref.ID); n != 0 {
			t.Fatal("private header allocated before the identity premise was refused")
		}
	})

	t.Run("misaligned owner frontier", func(t *testing.T) {
		f := newCaptureFixture(t)
		captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, []string{"x"}, true, f.account, f.profile, f.lifetime, f.membership)
		f.c.Exec(`UPDATE catalog_purge_jobs SET owner_after='somebody-else' WHERE mutation_id=?`, f.m.Ref.ID)
		if _, _, err := captureStepOnce(t, f.c, f.m); !errors.Is(err, errRetainedCaptureState) {
			t.Fatalf("misaligned frontier: %v", err)
		}
	})

	t.Run("no identity facts finishes without a private header", func(t *testing.T) {
		f := newCaptureFixture(t)
		captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, f.owner, nil, true, f.account, f.profile, f.lifetime, f.membership)
		if published, _, err := captureStepOnce(t, f.c, f.m); err != nil || published {
			t.Fatalf("zero identities published=%v err=%v", published, err)
		}
		arm, ownerStarted, _, _, _, _, current, _ := captureJob(t, f.c, f.m.Ref.ID)
		if arm != retainedCaptureArmFinished || ownerStarted || current.Valid {
			t.Fatalf("zero identities arm=%d started=%v current=%v", arm, ownerStarted, current)
		}
		if n := captureCount(t, f.c, `SELECT count(*) FROM retained_title_state WHERE capture_mutation_id=?`, f.m.Ref.ID); n != 0 {
			t.Fatal("zero identities created a private header")
		}
	})

	t.Run("opaque owner is copied verbatim", func(t *testing.T) {
		f := newCaptureFixture(t)
		opaque := "viewer:opaque-legacy-owner"
		captureCurrentPosition(t, f.c, opaque, f.item, captureAssetID(t, f.c, f.token), "current", .6, 60)
		captureSeedJob(t, f.c, f.m.Ref.ID, f.m.Ref.Generation, opaque, []string{"local:/reading|book:opaque"}, false, "", "", "", "")
		if published, _, err := captureStepOnce(t, f.c, f.m); err != nil || published {
			// First step only builds the private header.
			if err != nil {
				t.Fatal(err)
			}
		}
		published, _ := captureDrive(t, f.c, f.m, 32)
		if !published {
			t.Fatal("opaque owner did not publish")
		}
		var newID int64
		if err := f.c.DB.QueryRow(`SELECT id FROM retained_title_state WHERE profile_id=? AND ready=1`, opaque).Scan(&newID); err != nil {
			t.Fatal(err)
		}
		if n := captureCount(t, f.c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, newID); n != 1 {
			t.Fatal("opaque owner lost the reading position")
		}
	})
}
