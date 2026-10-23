package metadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"strings"
	"testing"
	"time"
)

func artworkFileEdge(t *testing.T, f *os.File) int {
	t.Helper()
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	return max(cfg.Width, cfg.Height)
}
func TestPictureSizeUploadServingAndCompactionFollowCurrentSetting(t *testing.T) {
	s, db, target := repairFixture(t)
	ctx := context.Background()
	large := 2560
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkMaxEdge = &large })
	before, err := s.RepairState(ctx, target, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.UploadArtwork(ctx, target, "poster", "", before.Revision, p08cPNG(t, 2700, 1350), editorActor(), allowAll)
	if err != nil {
		t.Fatal(err)
	}
	var digest string
	var width, height, size int
	if err = db.QueryRow(`SELECT a.digest,o.width,o.height,o.bytes FROM artwork_selections a JOIN artwork_objects o ON o.digest=a.digest WHERE a.kind='item' AND a.entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND a.role='poster'`, target.ID).Scan(&digest, &width, &height, &size); err != nil {
		t.Fatal(err)
	}
	if width != 2560 || height != 1280 || size > 888888 || size > artworkMaximumDisplayBytes {
		t.Fatal(width, height, size)
	}
	for requested, want := range map[int]int{400: 400, 800: 800, 1920: 1920, 2560: 2560} {
		f, _, e := s.EntityArtworkVariant(ctx, target, "poster", "", requested, digest)
		if e != nil {
			t.Fatal(requested, e)
		}
		if got := artworkFileEdge(t, f); got != want {
			t.Fatal(requested, got, want)
		}
	}
	// The setting applies to stored uploads and locked selections too.
	small := 1280
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkMaxEdge = &small })
	for pass := 0; pass < 8; pass++ {
		if err = s.CompactArtworkStep(ctx); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRow(`SELECT max(o.width,o.height) FROM artwork_selections a JOIN artwork_objects o ON o.digest=a.digest WHERE a.kind='item' AND a.entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND a.role='poster'`, target.ID).Scan(&width); err != nil {
			t.Fatal(err)
		}
		if width <= small {
			break
		}
	}
	f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", 2560, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := artworkFileEdge(t, f); got != 1280 {
		t.Fatal("compactor ignored Smaller", got, state)
	}
	standard := 1920
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkMaxEdge = &standard })
	f, _, err = s.EntityArtworkVariant(ctx, target, "poster", "", 2560, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := artworkFileEdge(t, f); got != 1280 {
		t.Fatal("stored picture enlarged", got)
	}
}
func TestPictureSizeRejectsUnapprovedEdgesAndUsesExactCeilings(t *testing.T) {
	s, _, _ := repairFixture(t)
	ctx := context.Background()
	for _, edge := range []int{0, 400, 800, 1279, 1921, 4096} {
		current, err := s.MetadataSettings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.UpdateMetadataSettings(ctx, MetadataSettingsUpdate{ExpectedRevision: current.Revision, LocalArtworkLimitBytes: current.LocalArtworkLimitBytes, IncludeAdult: current.IncludeAdult, ArtworkMaxEdge: &edge}, nil)
		if err == nil {
			t.Fatal("unapproved edge stored", edge)
		}
	}
	for edge, want := range map[int]int{1280: 500000, 1920: 500000, 2560: 888888} {
		if got := artworkByteLimit(edge); got != want {
			t.Fatal(edge, got, want)
		}
	}
}
func TestLargerPictureSweepRetainsOfflineIntentAndSelectedLock(t *testing.T) {
	s, db, target := repairFixture(t)
	ctx := context.Background()
	raw := p08cPNG(t, 2000, 1000)
	original, thumb, w, h, err := normalizeArtwork(raw)
	if err != nil {
		t.Fatal(err)
	}
	full, err := s.installArtworkContext(ctx, original, w, h)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatal(err)
	}
	small, err := s.installArtworkContext(ctx, thumb, cfg.Width, cfg.Height)
	if err != nil {
		t.Fatal(err)
	}
	// Seed through the production candidate/queue seam, then retain one selected
	// picture as a prior completed publication. No HTTP is performed by a sweep.
	var candidate string
	err = dbwork.WithWriteTx(ctx, db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		entity, e := artworkEntity(ctx, tx, target.ID)
		if e != nil {
			return e
		}
		fence, e := artworkFence(ctx, tx, target)
		if e != nil {
			return e
		}
		if e = recordArtworkObject(ctx, tx, full, "now"); e != nil {
			return e
		}
		if e = recordArtworkObject(ctx, tx, small, "now"); e != nil {
			return e
		}
		candidate, e = insertArtworkCandidate(ctx, tx, target, "poster", "", "tmdb", "prior", "https://image.tmdb.org/t/p/original/prior.jpg", "", "Provider", fence, "now", 1)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,thumbnail_digest,locked,revision,actor,observed_at) VALUES('item',?,'poster','',?,?,?,1,1,'owner:original','now')`, entity, candidate, full.digest, small.digest)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	off := false
	large := 2560
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.OnlineLookups = &off; in.ArtworkMaxEdge = &large })
	if err = s.GlobalArtworkSizeStep(ctx); err != nil {
		t.Fatal(err)
	}
	var requests, jobs int
	if err = db.QueryRow(`SELECT count(*) FROM metadata_artwork_size_requests`).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM artwork_jobs WHERE size_refresh=1`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || jobs != 0 {
		t.Fatal("offline resize intent lost or fetched", requests, jobs)
	}
	on := true
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.OnlineLookups = &on })
	if err = s.GlobalArtworkSizeStep(ctx); err != nil {
		t.Fatal(err)
	}
	var locked, refresh bool
	var got string
	if err = db.QueryRow(`SELECT candidate_id,locked,size_refresh FROM artwork_jobs WHERE size_refresh=1`).Scan(&got, &locked, &refresh); err != nil {
		t.Fatal(err)
	}
	if got != candidate || !locked || !refresh {
		t.Fatal("selected choice/lock changed", got, locked, refresh)
	}
	if _, err = db.Exec(`INSERT INTO metadata_refresh_budget(provider,span,window_start,spent) VALUES('tmdb','hour',?,?) ON CONFLICT(provider,span) DO UPDATE SET window_start=excluded.window_start,spent=excluded.spent`, s.publicationTime().Unix(), refreshBudgetHour); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.artClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("not an image"))}, nil
	})}
	if err = s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	var status, problem string
	if err = db.QueryRow(`SELECT status,error FROM artwork_jobs WHERE size_refresh=1`).Scan(&status, &problem); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || status != "retry" || problem != "refresh_budget" {
		t.Fatal("refresh escaped budget", calls, status, problem)
	}
	future := s.publicationTime().Add(2 * time.Hour)
	s.now = func() time.Time { return future }
	fresh := p08cPNG(t, 2700, 1350)
	s.artClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/t/p/original/prior.jpg" {
			t.Error("not the original selected picture", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(fresh)), ContentLength: int64(len(fresh))}, nil
	})}
	if err = s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	var edge int
	if err = db.QueryRow(`SELECT max(o.width,o.height),a.candidate_id,a.locked FROM artwork_selections a JOIN artwork_objects o ON o.digest=a.digest WHERE a.kind='item' AND a.entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND a.role='poster'`, target.ID).Scan(&edge, &got, &locked); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || edge != 2560 || got != candidate || !locked {
		t.Fatal("provider size catch-up", calls, edge, got, locked)
	}
	if err = s.GlobalArtworkSizeStep(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM metadata_artwork_size_requests`).Scan(&requests); err != nil || requests != 0 {
		t.Fatal("completed resize intent not retired", requests, err)
	}
}

func TestPictureSizeAppliesToEveryStoredDisplayRole(t *testing.T) {
	s, db := editorFixture(t)
	names := editorNames(t, db)
	ctx := context.Background()
	small := 1280
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkMaxEdge = &small })
	raw := p08cPNG(t, 1400, 700)
	for _, choice := range []struct{ kind, name, role string }{{"item", "movie", "poster"}, {"item", "movie", "backdrop"}, {"item", "movie", "thumbnail"}, {"item", "movie", "logo"}, {"item", "episode", "still"}, {"artist", "artist", "square"}, {"artist", "artist", "portrait"}, {"show", "show", "banner"}, {"book", "book", "cover"}} {
		target := editorTarget(names, choice.kind, choice.name)
		before, err := s.RepairState(ctx, target, allowAll)
		if err != nil {
			t.Fatal(choice, err)
		}
		if _, err = s.UploadArtwork(ctx, target, choice.role, "", before.Revision, raw, editorActor(), allowAll); err != nil {
			t.Fatal(choice, err)
		}
		var edge, size int
		if err = db.QueryRow(`SELECT max(o.width,o.height),o.bytes FROM artwork_selections a JOIN artwork_objects o ON o.digest=a.digest WHERE a.kind=? AND a.entity_id=? AND a.role=?`, choice.kind, names[choice.name].ID, choice.role).Scan(&edge, &size); err != nil {
			t.Fatal(choice, err)
		}
		if edge != 1280 || size > 500000 {
			t.Fatal("role ignored storage size", choice, edge, size)
		}
	}
}

func TestLargerFolderPictureReadsOriginalLocalBytesAgain(t *testing.T) {
	s, db, target := repairFixture(t)
	ctx := context.Background()
	small := 1280
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkMaxEdge = &small })
	raw := p08cPNG(t, 2700, 1350)
	path := filepath.Join(t.TempDir(), "local-original.png")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	key := hex.EncodeToString(digest[:])
	reads := 0
	s.LocalArtwork = func(requested string) (*os.File, string, error) {
		if requested != key {
			t.Error("unexpected local original key", requested)
		}
		reads++
		f, e := os.Open(path)
		return f, "image/png", e
	}
	sized, err := s.artworkSizeContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	full, thumb, w, h, err := normalizeArtworkFormatsContext(sized, raw, map[string]bool{"png": true})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.installArtworkContext(ctx, full, w, h)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.installArtworkContext(ctx, thumb, cfg.Width, cfg.Height)
	if err != nil {
		t.Fatal(err)
	}
	err = dbwork.WithWriteTx(ctx, db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
		entity, e := artworkEntity(ctx, tx, target.ID)
		if e != nil {
			return e
		}
		fence, e := artworkFence(ctx, tx, target)
		if e != nil {
			return e
		}
		if e = recordArtworkObject(ctx, tx, a, "now"); e != nil {
			return e
		}
		if e = recordArtworkObject(ctx, tx, b, "now"); e != nil {
			return e
		}
		candidate, e := insertArtworkCandidate(ctx, tx, target, "poster", "", "local", key, "local:"+key, "", "Local artwork", fence, "now", 1)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,thumbnail_digest,locked,revision,actor,observed_at) VALUES('item',?,'poster','',?,?,?,0,1,'','now')`, entity, candidate, a.digest, b.digest)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	large := 2560
	off := false
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkMaxEdge = &large; in.OnlineLookups = &off })
	if err = s.GlobalArtworkSizeStep(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	var edge int
	if err = db.QueryRow(`SELECT max(o.width,o.height) FROM artwork_selections a JOIN artwork_objects o ON o.digest=a.digest WHERE a.kind='item' AND a.entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND a.role='poster'`, target.ID).Scan(&edge); err != nil {
		t.Fatal(err)
	}
	if reads != 1 || edge != 2560 {
		t.Fatal("local bytes were not read again under Larger", reads, edge)
	}
}

func TestLegacyLocalReadQueuesBoundedPictureWithoutServingOriginal(t *testing.T) {
	s, db, target := repairFixture(t)
	ctx := context.Background()
	small := 1280
	off := false
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkMaxEdge = &small; in.OnlineLookups = &off })
	raw := p08cPNG(t, 2700, 1350)
	digest := sha256.Sum256(raw)
	key := hex.EncodeToString(digest[:])
	path := filepath.Join(t.TempDir(), "original.png")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	reads := 0
	s.LocalArtwork = func(requested string) (*os.File, string, error) {
		reads++
		if requested != key {
			t.Error("wrong local original", requested)
		}
		f, e := os.Open(path)
		return f, "image/png", e
	}
	// This is the pre-selection migration shape, not an already published image.
	if _, err := db.Exec(`UPDATE catalog_item_details SET poster_url=? WHERE entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, "local:"+key, target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM artwork_dirty`); err != nil {
		t.Fatal(err)
	}
	f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", 400, "")
	if f != nil {
		f.Close()
		t.Fatal("legacy original was served")
	}
	if !errors.Is(err, ErrArtworkPending) || reads != 0 {
		t.Fatal("cold read bypassed bounded publication", err, reads)
	}
	if err = s.RepairMissingArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	f, _, err = s.EntityArtworkVariant(ctx, target, "poster", "", 400, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := artworkFileEdge(t, f); got != 400 {
		t.Fatal("cold fallback ignored serving bucket", got)
	}
	f, _, err = s.EntityArtworkVariant(ctx, target, "poster", "", 2560, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := artworkFileEdge(t, f); got != 1280 {
		t.Fatal("cold fallback ignored stored edge", got)
	}
	if reads != 1 {
		t.Fatal("local source was reread on the serving path", reads)
	}
}

func TestPictureSizeChangeDuringReadDiscardsOldEncodingAndRetries(t *testing.T) {
	s, db, target := repairFixture(t)
	ctx := context.Background()
	off := false
	saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.OnlineLookups = &off })
	raw := p08cPNG(t, 2700, 1350)
	digest := sha256.Sum256(raw)
	key := hex.EncodeToString(digest[:])
	path := filepath.Join(t.TempDir(), "original.png")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	reads := 0
	s.LocalArtwork = func(requested string) (*os.File, string, error) {
		reads++
		if requested != key {
			t.Error("wrong local source", requested)
		}
		if reads == 1 {
			// The real setting write happens after the worker captured its
			// encoding context, while it holds no publication transaction.
			small := 1280
			saveGlobal(t, s, func(in *MetadataSettingsUpdate) { in.ArtworkMaxEdge = &small })
		}
		f, err := os.Open(path)
		return f, "image/png", err
	}
	if _, err := db.Exec(`UPDATE catalog_item_details SET poster_url=? WHERE entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, "local:"+key, target.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	var selected int
	if err := db.QueryRow(`SELECT count(*) FROM artwork_selections`).Scan(&selected); err != nil || selected != 0 || reads != 1 {
		t.Fatal("old-size bytes were published", selected, reads, err)
	}
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", 2560, "")
	if err != nil {
		t.Fatal(err)
	}
	if edge := artworkFileEdge(t, f); edge != 1280 || reads != 2 {
		t.Fatal("retry did not encode current size", edge, reads)
	}
}
