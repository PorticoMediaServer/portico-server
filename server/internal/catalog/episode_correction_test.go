package catalog

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
)

func TestEpisodeCorrectionRoundTripRetainsIdentityStateAndMissingInvariant(t *testing.T) {
	s, db, root, library, job := episodicScanFixture(t, "Correction")
	c := catalogtest.New(t, db)
	relative := "Show/Show S01E01.mkv"
	commitEpisodeFile(t, s, db, job, root, relative, library)
	episodicDrain(t, db)
	ctx := context.Background()
	var a int64
	var public, asset string
	if err := db.QueryRow(`SELECT e.id,pid(e.public_id),a.token FROM catalog_episodes ep JOIN catalog_entities e ON e.id=ep.entity_id JOIN catalog_asset_links l ON l.entity_id=e.id JOIN catalog_assets a ON a.id=l.asset_id WHERE ep.number=1`).Scan(&a, &public, &asset); err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err := s.SetPersonal("account", "profile", public, PersonalMutation{OperationID: "favorite", Favorite: &yes}, nil); err != nil {
		t.Fatal(err)
	}
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,playback_id) VALUES('profile',?,1234,0,'history')`, a)
	c.Exec(`INSERT INTO play_history(playback_id,authority,account_id,profile_id,user_name,item_id,library_id,kind,title,started_ms,updated_ms) VALUES('history','local','account','profile','Owner',?,?,?,'Episode',1,2)`, a, library, int(compactcatalog.Episode))
	c.Exec(`INSERT INTO playback_sessions(id,session_hash,account_id,profile_id,item_id,asset_id,generation,state,grant_hash,grant_token,expires_at,request_id,duration) VALUES('playing','hash','account','profile',?,?,1,'playing','grant','token','2099-01-01','request',60)`, a, asset)
	// A separate valid episode remains genuinely missing and must still be pruned.
	commitEpisodeFile(t, s, db, job, root, "Other/Other S01E09.mkv", library)
	var missing, missingAsset int64
	if err := db.QueryRow(`SELECT ep.entity_id,l.asset_id FROM catalog_episodes ep JOIN catalog_asset_links l ON l.entity_id=ep.entity_id WHERE ep.number=9`).Scan(&missing, &missingAsset); err != nil {
		t.Fatal(err)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := compactcatalog.SetAssetAvailableTx(ctx, tx, missingAsset, false); err != nil {
			return err
		}
		return compactcatalog.SettleAssetMissingTx(ctx, tx, missingAsset)
	})
	// Establish a real matching snapshot for both a collision and a gap-fill.
	if _, err := s.SetPersonal("account", "gap-profile", public, PersonalMutation{OperationID: "gap-favorite", Favorite: &yes}, nil); err != nil {
		t.Fatal(err)
	}
	c.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,playback_id) VALUES('gap-profile',?,333,0,'gap-history')`, a)
	var identity string
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		identities, _, _, err := retentionIdentitiesTx(ctx, tx, a)
		if err != nil {
			return err
		}
		if len(identities) == 0 {
			t.Fatal("episode has no retention identity")
		}
		identity = identities[0]
		return retainStateTx(ctx, tx, a)
	})
	var retained int
	if err := db.QueryRow(`SELECT count(*) FROM retained_title_state s JOIN retained_title_identities i ON i.state_id=s.id WHERE i.identity=? AND s.favorite=1 AND s.position IN(1234,333)`, identity).Scan(&retained); err != nil || retained != 2 {
		t.Fatal("matching snapshot not established", retained, err)
	}
	c.Exec(`DELETE FROM personal_items WHERE profile_id='gap-profile' AND item_id=?`, a)
	c.Exec(`DELETE FROM progress WHERE profile_id='gap-profile' AND item_id=?`, a)
	// New live values must win over the older matching retained values.
	c.Exec(`UPDATE personal_items SET favorite=0,revision=revision+1 WHERE profile_id='profile' AND item_id=?`, a)
	c.Exec(`UPDATE progress SET position=5678 WHERE profile_id='profile' AND item_id=?`, a)
	assign := func(number int) {
		t.Helper()
		season := 1
		if err := s.AssignEpisodes(ctx, library, EpisodeAssignment{AssetID: asset, ShowTitle: "Show", Numbering: "seasonal", SeasonNumber: &season, EpisodeNumbers: []int{number}}); err != nil {
			t.Fatal(err)
		}
		episodicDrain(t, db)
	}
	state := func(id int64, wantRetired, wantMissing int) {
		t.Helper()
		var retired, marks int
		if err := db.QueryRow(`SELECT retired,(SELECT count(*) FROM catalog_missing_titles WHERE entity_id=e.id) FROM catalog_entities e WHERE id=?`, id).Scan(&retired, &marks); err != nil || retired != wantRetired || marks != wantMissing {
			t.Fatalf("entity%d retired=%d missing=%d want%d/%d: %v", id, retired, marks, wantRetired, wantMissing, err)
		}
	}
	assign(2)
	var b int64
	if err := db.QueryRow(`SELECT entity_id FROM catalog_episodes WHERE number=2`).Scan(&b); err != nil {
		t.Fatal(err)
	}
	state(a, 1, 0)
	state(b, 0, 0)
	state(missing, 0, 1)
	var stopped string
	if err := db.QueryRow(`SELECT state FROM playback_sessions WHERE id='playing'`).Scan(&stopped); err != nil || stopped != "stopped" {
		t.Fatal("corrected playback was not stopped", stopped, err)
	}
	// Rescanning the physical A filename cannot replace the owner's B mapping.
	commitEpisodeFile(t, s, db, job, root, relative, library)
	episodicDrain(t, db)
	var assetID int64
	if err := db.QueryRow(`SELECT id FROM catalog_assets WHERE token=?`, asset).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SettleAssetMissingTx(ctx, tx, assetID)
	})
	state(a, 1, 0)
	state(b, 0, 0)
	out, err := s.EmptyMissing(ctx, library, time.Now().Add(time.Hour), nil)
	if err != nil || out.Titles != 1 {
		t.Fatal("actual missing pruner", out, err)
	}
	var stillMissing int
	if err = db.QueryRow(`SELECT count(*) FROM catalog_entities WHERE id=?`, missing).Scan(&stillMissing); err != nil || stillMissing != 0 {
		t.Fatal("unrelated missing title not pruned", stillMissing, err)
	}
	state(a, 1, 0)
	assign(1)
	state(a, 0, 0)
	state(b, 1, 0)
	commitEpisodeFile(t, s, db, job, root, relative, library)
	episodicDrain(t, db)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SettleAssetMissingTx(ctx, tx, assetID)
	})
	var gotPublic string
	var favorite, revision, position, history, links int
	if err = db.QueryRow(`SELECT pid(e.public_id),p.favorite,p.revision,g.position,(SELECT count(*) FROM play_history WHERE item_id=e.id AND playback_id='history'),(SELECT count(*) FROM catalog_asset_links WHERE entity_id=e.id AND asset_id=?) FROM catalog_entities e JOIN personal_items p ON p.item_id=e.id AND p.profile_id='profile' JOIN progress g ON g.item_id=e.id AND g.profile_id='profile' WHERE e.id=?`, assetID, a).Scan(&gotPublic, &favorite, &revision, &position, &history, &links); err != nil || gotPublic != public || favorite != 0 || revision != 2 || position != 5678 || history != 1 || links != 1 {
		t.Fatal("roundtrip lost identity/live state/history", gotPublic, favorite, revision, position, history, links, err)
	}
	state(a, 0, 0)
	state(b, 1, 0)
	var gapFavorite, gapPosition int
	if err = db.QueryRow(`SELECT p.favorite,g.position FROM personal_items p JOIN progress g ON g.profile_id=p.profile_id AND g.item_id=p.item_id WHERE p.profile_id='gap-profile' AND p.item_id=?`, a).Scan(&gapFavorite, &gapPosition); err != nil || gapFavorite != 1 || gapPosition != 333 {
		t.Fatal("matching retained snapshot did not fill live gaps", gapFavorite, gapPosition, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM retained_title_identities WHERE identity=?`, identity).Scan(&retained); err != nil || retained != 0 {
		t.Fatal("matching retained snapshot was not consumed", retained, err)
	}
	visible, err := s.Get("profile", public)
	if err != nil || visible.ID != public || visible.Episode == nil || visible.Episode.Number != 1 || visible.Episode.LocalIdentityStatus != "manual" || len(visible.Sources) != 1 {
		t.Fatal("reactivated mapping not publicly visible", visible, err)
	}
	if _, err = os.Stat(filepath.Join(root, relative)); err != nil {
		t.Fatal("correction deleted physical bytes", err)
	}
}

func TestEpisodeCorrectionKeepsOtherSourcesIncludingOffline(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "available", false: "offline"}[available], func(t *testing.T) {
			s, db, root, library, job := episodicScanFixture(t, "Other sources")
			c := catalogtest.New(t, db)
			commitEpisodeFile(t, s, db, job, root, "Show/Show S01E01.mkv", library)
			commitEpisodeFile(t, s, db, job, root, "Show/Show S01E01 alternate.mkv", library)
			episodicDrain(t, db)
			var item, otherAsset int64
			var asset string
			if err := db.QueryRow(`SELECT l.entity_id,a.token FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id ORDER BY a.id LIMIT 1`).Scan(&item, &asset); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT asset_id FROM catalog_asset_links WHERE entity_id=? AND asset_id<>(SELECT id FROM catalog_assets WHERE token=?)`, item, asset).Scan(&otherAsset); err != nil {
				t.Fatal(err)
			}
			c.Write(func(ctx context.Context, tx *sql.Tx) error {
				if err := compactcatalog.SetAssetAvailableTx(ctx, tx, otherAsset, available); err != nil {
					return err
				}
				return compactcatalog.SettleAssetMissingTx(ctx, tx, otherAsset)
			})
			season := 1
			if err := s.AssignEpisodes(context.Background(), library, EpisodeAssignment{AssetID: asset, ShowTitle: "Show", Numbering: "seasonal", SeasonNumber: &season, EpisodeNumbers: []int{2}}); err != nil {
				t.Fatal(err)
			}
			episodicDrain(t, db)
			var retired, links, marks int
			if err := db.QueryRow(`SELECT retired,(SELECT count(*) FROM catalog_asset_links WHERE entity_id=e.id AND asset_id=?),(SELECT count(*) FROM catalog_missing_titles WHERE entity_id=e.id) FROM catalog_entities e WHERE id=?`, otherAsset, item).Scan(&retired, &links, &marks); err != nil || retired != 0 || links != 1 || marks != map[bool]int{true: 0, false: 1}[available] {
				t.Fatal("other source interpretation lost", retired, links, marks, err)
			}
			if _, err := os.Stat(filepath.Join(root, "Show/Show S01E01 alternate.mkv")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEpisodeCorrectionRollsBackRetirementAndLinksOnFailure(t *testing.T) {
	s, db, root, library, job := episodicScanFixture(t, "Rollback")
	c := catalogtest.New(t, db)
	commitEpisodeFile(t, s, db, job, root, "Show/Show S01E01.mkv", library)
	var item int64
	var asset string
	if err := db.QueryRow(`SELECT l.entity_id,a.token FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id`).Scan(&item, &asset); err != nil {
		t.Fatal(err)
	}
	c.Exec(`CREATE TRIGGER refuse_correction_marker BEFORE DELETE ON catalog_missing_titles BEGIN SELECT RAISE(ABORT,'fixture refuses obsolete marker deletion'); END`)
	season := 1
	if err := s.AssignEpisodes(context.Background(), library, EpisodeAssignment{AssetID: asset, ShowTitle: "Show", Numbering: "seasonal", SeasonNumber: &season, EpisodeNumbers: []int{2}}); err == nil || !strings.Contains(err.Error(), "fixture refuses obsolete marker deletion") {
		t.Fatal("writer failure swallowed or replaced", err)
	}
	var retired, links, extra int
	if err := db.QueryRow(`SELECT retired,(SELECT count(*) FROM catalog_asset_links WHERE entity_id=e.id),(SELECT count(*) FROM catalog_episodes WHERE number=2) FROM catalog_entities e WHERE id=?`, item).Scan(&retired, &links, &extra); err != nil || retired != 0 || links != 1 || extra != 0 {
		t.Fatal("partial correction escaped rollback", retired, links, extra, err)
	}
}

// Automatic reinterpretation retains the ordinary genuinely-missing semantics;
// only an explicit owner correction retires the superseded logical identity.
func TestAutomaticEpisodeReinterpretationDoesNotRetireMissingIdentity(t *testing.T) {
	s, db, root, library, job := episodicScanFixture(t, "Automatic")
	c := catalogtest.New(t, db)
	commitEpisodeFile(t, s, db, job, root, "Show/Show S01E01.mkv", library)
	var item int64
	var asset string
	if err := db.QueryRow(`SELECT l.entity_id,a.token FROM catalog_asset_links l JOIN catalog_assets a ON a.id=l.asset_id`).Scan(&item, &asset); err != nil {
		t.Fatal(err)
	}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return s.commitEpisodes(tx, library, asset, "Show S01E02.mkv", EpisodeNaming{ShowTitle: "Show", Season: 1, Numbering: "seasonal", Numbers: []int{2}}, false)
	})
	var retired, marks int
	if err := db.QueryRow(`SELECT retired,(SELECT count(*) FROM catalog_missing_titles WHERE entity_id=e.id) FROM catalog_entities e WHERE id=?`, item).Scan(&retired, &marks); err != nil || retired != 0 || marks != 1 {
		t.Fatal("automatic interpretation discarded valid missing identity", retired, marks, err)
	}
}
