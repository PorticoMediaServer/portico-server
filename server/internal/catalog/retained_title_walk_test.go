package catalog

// P2-59 / O-022 retained-title walk tests.
//
// These tests use the real installed baseline and drive the actual step. They
// never seed capture_arm, identity_after, the Reading cursors or a completion
// counter. Fixture rows are real domain inputs (personal state, Reading
// positions, container defaults and admitted mutations) written through the
// production catalogue writers or the same exact personal tables the capture
// component reads.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/phasedpurge"
)

func titleWalkAddOwner(t *testing.T, c *catalogtest.Catalog, account, profile string) (string, string, string) {
	t.Helper()
	c.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id) VALUES(?,?,X'00',?)`, account, account, profile)
	var lifetime, membership string
	if err := c.DB.QueryRow(`SELECT p.lifetime,m.incarnation FROM direct_profiles p
 JOIN direct_memberships m ON m.account_id=p.account_id WHERE p.account_id=? AND p.id=?`, account, profile).
		Scan(&lifetime, &membership); err != nil {
		t.Fatal(err)
	}
	return persistence.PersonalOwnerKey("local", account, profile), lifetime, membership
}

func TestRetainedTitleWalkVerifiesInputsBeforeAdvancingOwner(t *testing.T) {
	f := newCaptureFixture(t)
	asset := captureAssetID(t, f.c, f.token)
	titleWalkInputs(t, f.c, f.owner, f.item, asset)
	beginCaptureFixture(t, f)
	verifyPending := false
	for turn := 0; turn < 100; turn++ {
		titleWalkStep(t, f.c, f.m)
		var raw, reason string
		if err := f.c.DB.QueryRow(`SELECT cursor,reason FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).Scan(&raw, &reason); err != nil {
			t.Fatal(err)
		}
		if raw == "" || reason == retainedTitleOwnerDrained {
			continue
		}
		c, err := phasedpurge.DecodeCursor(raw, retainedTitleDrainPlan(f.owner, f.item))
		if err != nil {
			t.Fatal(err)
		}
		if c.Unit == len(retainedTitleDrainUnits) && c.Verify == 0 {
			verifyPending = true
			break
		}
	}
	if !verifyPending {
		t.Fatal("did not reach the real pre-verification cursor")
	}
	// Deliberate residue injection bypasses the separately tested production
	// custody fence. The engine must still falsify a premature drained claim.
	f.c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,123,0,0,'residue')`, f.owner, f.item)
	if done, _ := titleWalkStep(t, f.c, f.m); done {
		t.Fatal("unit exhaustion was mistaken for verified completion")
	}
	var owner, reason string
	if err := f.c.DB.QueryRow(`SELECT owner_value,reason FROM catalog_purge_jobs WHERE mutation_id=?`, f.m.Ref.ID).Scan(&owner, &reason); err != nil || owner != f.owner || reason == retainedTitleOwnerDrained {
		t.Fatal("verification residue advanced the owner", owner, reason, err)
	}
	titleWalkDrive(t, f.c, f.m, 100)
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM progress WHERE profile_id=? AND item_id=?`, f.owner, f.item); n != 0 {
		t.Fatal("verified drain left progress", n)
	}
}

func TestRetainedTitleWalkAliasOracleUsesBoundedCoveringIndex(t *testing.T) {
	f := newCaptureFixture(t)
	for i := 0; i < 30; i++ {
		f.c.Exec(`INSERT INTO catalog_identities(root,source_key,public_id,kind) SELECT ?,?,public_id,kind FROM catalog_entities WHERE id=?`, fmt.Sprintf("historical-%02d", 29-i), fmt.Sprintf("alias-%02d", i), f.item)
	}
	var want, got []string
	f.c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		want, _, _, err = retentionIdentitiesTx(ctx, tx, f.item)
		if err != nil {
			return err
		}
		got, _, _, err = retainedTitleIdentitiesTx(ctx, tx, f.item)
		return err
	})
	if !sameRetainedTitleIdentities(want, got) || len(got) != retainedIdentityLimit {
		t.Fatalf("historical restoration oracle changed: want=%v got=%v", want, got)
	}
	for name, query := range map[string]string{"aliases": retainedTitleLocalSQL, "play-counts": retainedTitleNextPlaysSQL} {
		t.Run(name, func(t *testing.T) {
			args := []any{f.item, any(12)}
			if name == "play-counts" {
				args[1] = "after-owner"
			}
			rows, err := f.c.DB.Query(`EXPLAIN QUERY PLAN `+query, args...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var plan []string
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				plan = append(plan, detail)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			text := strings.Join(plan, "\n")
			if strings.Contains(text, "TEMP B-TREE") || strings.Contains(text, "SCAN ") {
				t.Fatal("oracle did not seek before limiting", text)
			}
			if name == "aliases" && !strings.Contains(text, "catalog_identities_retention (public_id=?)") {
				t.Fatal("missing retained identity covering seek", text)
			}
			if name == "play-counts" && !strings.Contains(text, "(item_id=? AND profile_id>?)") {
				t.Fatal("missing implicit WITHOUT ROWID profile seek", text)
			}
		})
	}
}

func titleWalkInputs(t *testing.T, c *catalogtest.Catalog, owner string, item, asset int64) {
	t.Helper()
	c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,revision) VALUES(?,?,1,1)`, owner, item)
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,100,0,0,'')`, owner, item)
	c.Exec(`INSERT INTO personal_watched_intents(profile_id,item_id,watched,authored_at) VALUES(?,?,1,'2026-01-02T00:00:00Z')`, owner, item)
	c.Exec(`INSERT INTO personal_play_counts(profile_id,item_id,plays) VALUES(?,?,1)`, owner, item)
	captureCurrentPosition(t, c, owner, item, asset, "title-walk-revision", .5, 100)
}

func titleWalkStep(t *testing.T, c *catalogtest.Catalog, m compactcatalog.EntityMutation) (bool, int64) {
	t.Helper()
	var done bool
	var before, after int64
	err := dbwork.WithWriteTx(context.Background(), c.DB, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		if err := tx.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
			return err
		}
		var e error
		done, e = captureRetainedTitleStepTx(context.Background(), tx, m)
		if e != nil {
			return e
		}
		return tx.QueryRow(`SELECT total_changes()`).Scan(&after)
	})
	if err != nil {
		t.Fatalf("title walk step: %v", err)
	}
	return done, after - before
}

func titleWalkDrive(t *testing.T, c *catalogtest.Catalog, m compactcatalog.EntityMutation, max int) (int, []int64) {
	t.Helper()
	var effects []int64
	for i := 0; i < max; i++ {
		done, n := titleWalkStep(t, c, m)
		effects = append(effects, n)
		if n > phasedpurge.EffectLimit {
			t.Fatalf("turn %d effects=%d limit=%d", i, n, phasedpurge.EffectLimit)
		}
		if done {
			return i + 1, effects
		}
	}
	t.Fatalf("title walk did not finish within %d turns", max)
	return 0, nil
}

func titleWalkCount(t *testing.T, c *catalogtest.Catalog, query string, args ...any) int {
	t.Helper()
	var n int
	if err := c.DB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%v: %s", err, query)
	}
	return n
}

func TestRetainedTitleWalkAdvancesOwnersAndDrainsInputs(t *testing.T) {
	f := newCaptureFixture(t)
	asset := captureAssetID(t, f.c, f.token)
	owners := []string{f.owner}
	for _, suffix := range []string{"b", "c"} {
		owner, _, _ := titleWalkAddOwner(t, f.c, "title-walk-account-"+suffix, "title-walk-profile-"+suffix)
		owners = append(owners, owner)
	}
	for _, owner := range owners {
		titleWalkInputs(t, f.c, owner, f.item, asset)
	}

	var library int64
	if err := f.c.DB.QueryRow(`SELECT library_id FROM catalog_entities WHERE id=?`, f.item).Scan(&library); err != nil {
		t.Fatal(err)
	}
	unrelated, unrelatedToken := captureAddPDF(t, f.c, library, 0, f.root, "unrelated.pdf")
	unrelatedAsset := captureAssetID(t, f.c, unrelatedToken)
	titleWalkInputs(t, f.c, owners[0], unrelated, unrelatedAsset)
	// A fourth profile has state only on the unrelated title; it must never be
	// selected by this title's walk.
	otherOwner, _, _ := titleWalkAddOwner(t, f.c, "title-walk-account-other", "title-walk-profile-other")
	titleWalkInputs(t, f.c, otherOwner, unrelated, unrelatedAsset)

	beginCaptureFixture(t, f)
	turns, effects := titleWalkDrive(t, f.c, f.m, 400)
	if turns < 3 {
		t.Fatalf("walk finished in %d turns; owners were not exercised", turns)
	}
	for i, n := range effects {
		if n > phasedpurge.EffectLimit {
			t.Fatalf("turn %d effects=%d", i, n)
		}
	}

	for _, owner := range owners {
		for _, table := range []struct{ name, query string }{
			{"personal_items", `SELECT count(*) FROM personal_items WHERE profile_id=? AND item_id=?`},
			{"progress", `SELECT count(*) FROM progress WHERE profile_id=? AND item_id=?`},
			{"reading_progress", `SELECT count(*) FROM reading_progress WHERE owner_key=? AND item_id=?`},
			{"personal_watched_intents", `SELECT count(*) FROM personal_watched_intents WHERE profile_id=? AND item_id=?`},
			{"personal_play_counts", `SELECT count(*) FROM personal_play_counts WHERE profile_id=? AND item_id=?`},
		} {
			if n := titleWalkCount(t, f.c, table.query, owner, f.item); n != 0 {
				t.Fatalf("%s for %q survived the drain: %d", table.name, owner, n)
			}
		}
		if n := titleWalkCount(t, f.c, `SELECT count(*) FROM retained_title_state WHERE profile_id=? AND ready=1`, owner); n != 1 {
			t.Fatalf("owner %q published headers=%d", owner, n)
		}
	}
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM retained_title_state WHERE capture_mutation_id=?`, f.m.Ref.ID); n != 0 {
		t.Fatalf("private headers survived completion: %d", n)
	}
	// Unrelated title and unrelated profile rows are untouched.
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM personal_items WHERE profile_id=? AND item_id=?`, owners[0], unrelated); n != 1 {
		t.Fatalf("unrelated title lost its personal row: %d", n)
	}
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM personal_items WHERE profile_id=? AND item_id=?`, otherOwner, unrelated); n != 1 {
		t.Fatalf("unrelated profile/title row was drained: %d", n)
	}
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM reading_progress WHERE owner_key=? AND item_id=?`, otherOwner, unrelated); n != 1 {
		t.Fatalf("unrelated Reading position was drained: %d", n)
	}
}

func titleWalkIdentity(t *testing.T, c *catalogtest.Catalog, entity int64) string {
	t.Helper()
	var root, key string
	if err := c.DB.QueryRow(`SELECT l.root,i.source_key FROM catalog_entities e
 JOIN catalog_libraries l ON l.id=e.library_id
 JOIN catalog_identities i ON i.public_id=e.public_id
 WHERE e.id=? ORDER BY i.source_key LIMIT 1`, entity).Scan(&root, &key); err != nil {
		t.Fatal(err)
	}
	return "local:" + root + "|" + key
}

// TestRetainedTitleWalkCurrentAndRetained65Versions proves the walk keeps the
// capture component's per-version walk for both streams: one owner with 65
// current Reading positions and another owner with a complete 65-child prior
// header. No version ceiling is introduced.
func TestRetainedTitleWalkCurrentAndRetained65Versions(t *testing.T) {
	f := newCaptureFixture(t)
	const versions = 65
	tokens := []string{f.token}
	for i := 1; i < versions; i++ {
		_, token := captureAddPDF(t, f.c, 0, f.item, f.root, fmt.Sprintf("current-%d.pdf", i))
		tokens = append(tokens, token)
	}
	for i, token := range tokens {
		captureCurrentPosition(t, f.c, f.owner, f.item, captureAssetID(t, f.c, token),
			fmt.Sprintf("current-%d", i), float64(i)/100, int64(1000+i))
	}

	ownerB, _, _ := titleWalkAddOwner(t, f.c, "title-walk-many-account", "title-walk-many-profile")
	f.c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,revision) VALUES(?,?,1,1)`, ownerB, f.item)
	identity := titleWalkIdentity(t, f.c, f.item)
	children := make([]captureChild, versions)
	for i, token := range tokens {
		children[i] = captureChild{token: token, revision: fmt.Sprintf("prior-%d", i), observed: int64(2000 + i), progression: float64(i) / 100}
	}
	old := captureOldState(t, f.c, ownerB, []string{identity}, children)

	beginCaptureFixture(t, f)
	turns, _ := titleWalkDrive(t, f.c, f.m, 1000)
	if turns < 100 {
		t.Fatalf("walk finished in %d turns; the per-version streams were not exercised", turns)
	}
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM reading_progress WHERE owner_key=? AND item_id=?`, f.owner, f.item); n != 0 {
		t.Fatalf("65 current positions drained=%d", n)
	}
	var currentID int64
	if err := f.c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identity, f.owner).Scan(&currentID); err != nil {
		t.Fatal(err)
	}
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, currentID); n != versions {
		t.Fatalf("current owner published %d of %d versions", n, versions)
	}
	var kept int64
	if err := f.c.DB.QueryRow(`SELECT state_id FROM retained_title_identities WHERE identity=? AND profile_id=?`, identity, ownerB).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept == old {
		t.Fatal("prior header was reused instead of a fresh publication")
	}
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, kept); n != versions {
		t.Fatalf("prior owner published %d of %d versions", n, versions)
	}
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM retained_reading_progress WHERE state_id=?`, old); n != versions {
		t.Fatalf("old complete header children changed: %d", n)
	}
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM retained_title_state WHERE ready=1 AND id=?`, old); n != 1 {
		t.Fatal("old complete header was deleted")
	}
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM personal_items WHERE profile_id=? AND item_id=?`, ownerB, f.item); n != 0 {
		t.Fatalf("prior owner input survived drain: %d", n)
	}
	if n := titleWalkCount(t, f.c, `SELECT count(*) FROM retained_title_state WHERE capture_mutation_id=?`, f.m.Ref.ID); n != 0 {
		t.Fatalf("private headers survived: %d", n)
	}
}
