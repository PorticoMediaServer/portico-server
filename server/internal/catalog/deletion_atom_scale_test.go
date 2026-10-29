package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

// This opt-in diagnostic measures the existing native browse/link atoms. It
// does not qualify the surrounding entity cascade or a relational purge unit.
// Seeding and ordinary maintained publication are outside the timed samples.
func TestO022NativeDeletionAtomsOnLargeShowAndArtist(t *testing.T) {
	if os.Getenv("PORTICO_O022_ATOM_SCALE") != "1" {
		t.Skip("set PORTICO_O022_ATOM_SCALE=1 for the large deletion diagnostic")
	}
	c := catalogtest.Open(t)
	tv := c.Library("atom-tv", "Long-running TV", "tv", "/atom-tv")
	music := c.Library("atom-music", "Large catalogue", "music", "/atom-music")
	show := c.Show(tv, "Long-running show", 1960)
	artist := c.Artist(music, "Large catalogue artist")
	type leaf struct {
		id   int64
		kind compactcatalog.Kind
	}
	var leaves []leaf
	var containers []leaf
	containers = append(containers, leaf{show.ID, compactcatalog.Show}, leaf{artist.ID, compactcatalog.Artist})
	start := time.Now()
	seed := func(library, parent int64, kind compactcatalog.Kind, prefix string, count int, facts func(int) map[string]any) {
		t.Helper()
		for base := 0; base < count; base += 8 {
			c.Write(func(ctx context.Context, tx *sql.Tx) error {
				for n := base; n < min(base+8, count); n++ {
					path := fmt.Sprintf("%s/%06d", prefix, n+1)
					id, _, err := compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: kind, Parent: parent, Key: compactcatalog.ItemKey("/", path, 0), Title: fmt.Sprintf("Item %06d", n+1), Added: "2026-10-01T00:00:00.000Z"})
					if err != nil {
						return err
					}
					if err = compactcatalog.SetFactsTx(ctx, tx, id, facts(n+1)); err != nil {
						return err
					}
					for version := 0; version < 3; version++ {
						asset, _, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: fmt.Sprintf("%s-v%d.mkv", path, version), Size: int64(1000000 + version), ModifiedNS: 1, Container: "mkv", Duration: 1800 + float64(n%60), VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080})
						if err != nil {
							return err
						}
						if err = compactcatalog.LinkAssetTx(ctx, tx, id, asset, compactcatalog.Link{Edition: fmt.Sprintf("version-%d", version)}); err != nil {
							return err
						}
					}
					leaves = append(leaves, leaf{id, kind})
				}
				return nil
			})
		}
	}
	for season := 1; season <= 50; season++ {
		s := c.Season(show, season)
		containers = append(containers, leaf{s.ID, compactcatalog.Season})
		seed(tv, s.ID, compactcatalog.Episode, fmt.Sprintf("/atom-tv/season-%02d", season), 100, func(number int) map[string]any {
			return map[string]any{"show_id": show.ID, "season_id": s.ID, "numbering": "seasonal", "number": number}
		})
		if season%10 == 0 {
			t.Logf("seeded %d episodes with3versions in %s", season*100, time.Since(start))
		}
	}
	for album := 1; album <= 100; album++ {
		a := c.Album(artist, fmt.Sprintf("Album %03d", album), 1950+album%75)
		containers = append(containers, leaf{a.ID, compactcatalog.Album})
		seed(music, a.ID, compactcatalog.Track, fmt.Sprintf("/atom-music/album-%03d", album), 20, func(number int) map[string]any { return map[string]any{"album_id": a.ID, "track_number": number} })
	}
	t.Logf("seeded5000episodes+2000tracks,21000versions in %s; publishing ordinary projections", time.Since(start))
	// The generic tiny-fixture Drain guard is based on entity count alone;
	// this diagnostic has21000 version/source rows as well. Use the same real
	// worker until its actual empty milestone, with a wall deadline rather
	// than changing any production quantum or accepting a nonempty frontier.
	publishCtx, cancelPublish := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancelPublish()
	worker := compactcatalog.NewWorker(c.DB)
	for turn := 0; ; turn++ {
		n, err := worker.Step(publishCtx, 500)
		if err != nil {
			t.Fatal("ordinary projection publication", err)
		}
		if n == 0 {
			t.Logf("ordinary projection publication empty after%d turns in%s", turn+1, time.Since(start))
			break
		}
		if turn > 0 && turn%100000 == 0 {
			t.Logf("ordinary projection publication%d turns in%s", turn, time.Since(start))
		}
	}
	for _, v := range []struct {
		q    string
		want int64
	}{{`SELECT count(*) FROM catalog_episodes`, 5000}, {`SELECT count(*) FROM catalog_songs`, 2000}, {`SELECT count(*) FROM catalog_asset_links`, 21000}} {
		var n int64
		if err := c.DB.QueryRow(v.q).Scan(&n); err != nil || n != v.want {
			t.Fatalf("fixture %s got%d want%d err%v", v.q, n, v.want, err)
		}
	}
	type peak struct {
		samples int
		hold    time.Duration
		cost    dbwork.WriteCost
	}
	peaks := map[string]*peak{}
	measure := func(name, query string, args ...any) {
		t.Helper()
		dbwork.ResetPlans()
		dbwork.WriteGate().ResetPeak()
		began := time.Now()
		if err := dbwork.WithWriteTx(context.Background(), c.DB, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error { _, err := tx.ExecContext(context.Background(), query, args...); return err }); err != nil {
			t.Fatal(name, err)
		}
		elapsed := time.Since(began)
		held := time.Duration(dbwork.WriteGate().Stats().MaxHeldMilli) * time.Millisecond
		p := peaks[name]
		if p == nil {
			p = &peak{}
			peaks[name] = p
		}
		p.samples++
		p.hold = max(p.hold, held)
		cost := dbwork.Plans().Largest
		p.cost.Rows = max(p.cost.Rows, cost.Rows)
		p.cost.Pages = max(p.cost.Pages, cost.Pages)
		p.cost.VMOps = max(p.cost.VMOps, cost.VMOps)
		if held > 250*time.Millisecond {
			t.Fatalf("%s held%s (elapsed%s),cost%+v; natural season/album split required", name, held, elapsed, cost)
		}
	}
	// Link atoms run while their complete published browse/materialized facts
	// still exist. No publication worker is run during the deletion samples.
	for _, it := range leaves {
		var assets []int64
		rows, err := c.DB.Query(`SELECT asset_id FROM catalog_asset_links WHERE entity_id=? ORDER BY asset_id`, it.id)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var a int64
			if err = rows.Scan(&a); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			assets = append(assets, a)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
		for _, a := range assets {
			measure(fmt.Sprintf("link-kind-%d", it.kind), `DELETE FROM catalog_asset_links WHERE entity_id=? AND asset_id=?`, it.id, a)
		}
	}
	// The retaining branch is the wider native browse atom: production workers
	// use this singleton while maintaining their old-generation serving image.
	c.Exec(`UPDATE catalog_generations SET retaining=1 WHERE singleton=1`)
	for _, it := range append(leaves, containers...) {
		measure(fmt.Sprintf("browse-kind-%d-retaining", it.kind), `DELETE FROM catalog_browse_rows WHERE entity_id=?`, it.id)
	}
	for name, p := range peaks {
		t.Logf("%s samples%d maxhold%s maxrows%d max-cache-page-writes%d maxVM%d (observed maxima,notbounds)", name, p.samples, p.hold, p.cost.Rows, p.cost.Pages, p.cost.VMOps)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_browse_rows`); n != 0 {
		t.Fatalf("unmeasured browse rows%d", n)
	}
	t.Logf("native atoms complete in %s; entity/root/season cascades NOT measured or qualified", time.Since(start))
}
