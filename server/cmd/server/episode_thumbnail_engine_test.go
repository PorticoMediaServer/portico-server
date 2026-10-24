package main

import (
	"context"
	"database/sql"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/metadata"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/remotesources"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitlevideo"
)

// The optional selected-pair gate exercises the actual confined local source,
// ranged decoder, current artwork encoder and publication authority together.
// Its procedural video is temporary; supplied licensed media is never used.
func TestEpisodeActualLocalDecodeAndPublication(t *testing.T) {
	ffmpeg, ffprobe := os.Getenv("PORTICO_EPISODE_TEST_FFMPEG"), os.Getenv("PORTICO_EPISODE_TEST_FFPROBE")
	if ffmpeg == "" || ffprobe == "" {
		t.Skip("set explicit same-bundle FFmpeg/ffprobe for actual Episode engine qualification")
	}
	if !filepath.IsAbs(ffmpeg) || !filepath.IsAbs(ffprobe) || filepath.Dir(ffmpeg) != filepath.Dir(ffprobe) {
		t.Fatal("selected pair must have one explicit absolute bundle directory")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	root := t.TempDir()
	file := filepath.Join(root, "episode.mp4")
	generate := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=4", "-an", "-c:v", "libx264", "-threads", "1", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-movflags", "+faststart", file)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("procedural fixture: %v %s", err, output)
	}
	facts, err := (assets.Probe{Binary: ffprobe}).Inspect(ctx, file)
	if err != nil || facts.VideoCodec != "h264" || facts.Width != 320 || facts.Height != 180 || facts.Duration != 4 {
		t.Fatalf("actual source facts: %+v %v", facts, err)
	}
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"off", "provider", "provider_then_made"} {
		t.Run(mode, func(t *testing.T) {
			c := catalogtest.Open(t)
			local := storage.New(helper)
			managed, err := mounts.New(c.DB, t.TempDir(), "", helper, local)
			if err != nil {
				t.Fatal(err)
			}
			sources, err := remotesources.New(c.DB, t.TempDir(), managed)
			if err != nil {
				t.Fatal(err)
			}
			identities, err := identity.New(c.DB, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			c.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id,epoch) VALUES('account','owner',X'00','profile',1)`)
			grant, err := identities.Issue("account", "profile", "local", "owner", 1)
			if err != nil {
				t.Fatal(err)
			}
			owner, err := identities.Authenticate(grant.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			library := c.Library("episodes", "Episodes", "tv", root)
			show := c.Show(library, "Actual episode", 2026)
			episode := c.Episode(show, c.Season(show, 1), 1, file)
			observedRoot, err := local.InspectRoot(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			observed, err := local.Stat(ctx, "episode-engine-fixture", file)
			if err != nil {
				t.Fatal(err)
			}
			c.Exec(`UPDATE library_sources SET identity_confirmed=1,root_identity=? WHERE id='episodes'`, storage.RootIdentity(observedRoot))
			source, err := catalog.New(c.DB).LibrarySource(ctx, "episodes")
			if err != nil {
				t.Fatal(err)
			}
			revision, evidence, err := catalog.ObservationRevision(source, observed)
			if err != nil {
				t.Fatal(err)
			}
			c.Exec(`UPDATE catalog_assets SET size=?,modified_ns=?,container=?,video_codec=?,width=?,height=?,duration=? WHERE token=?`, observed.Size, observed.ModifiedNS, facts.Container, facts.VideoCodec, facts.Width, facts.Height, facts.Duration, episode.Token)
			c.Exec(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns,state) VALUES('object','episodes',?,?,'episode.mp4',?,?,?,?, 'available')`, episode.Token, source.Incarnation, revision, evidence, observed.Size, observed.ModifiedNS)
			if err := dbwork.WithWriteTx(ctx, c.DB, dbwork.ClassInteractive, func(tx *sql.Tx) error {
				if err := catalog.ApplyEpisodeThumbnailModeTx(ctx, tx, "episodes", mode, 1); err != nil {
					return err
				}
				// Explicit generation is permitted independently of this automatic tier.
				return catalog.ApplyScanPolicyTx(ctx, tx, "episodes", catalog.ScanPolicy{Tier: "file_list_only"})
			}); err != nil {
				t.Fatal(err)
			}
			c.Drain()
			meta := metadata.New(c.DB, "")
			if err := meta.SetArtworkDirectory(t.TempDir()); err != nil {
				t.Fatal(err)
			}
			inputs, err := subtitlevideo.New(subtitlevideo.Options{DB: c.DB, Storage: local, Mounts: managed, Directory: t.TempDir(), FFmpeg: ffmpeg, FFprobe: ffprobe})
			if err != nil {
				t.Fatal(err)
			}
			defer inputs.Close()
			meta.EpisodeThumbnails = episodeThumbnailRuntime(meta, sources, identities, local, inputs)
			authorize := func(tx *sql.Tx) error {
				current, err := identities.PrincipalByDigestTx(ctx, tx, owner.Hash)
				if err != nil {
					return err
				}
				if current.Viewer != owner.Viewer || current.Epoch != owner.Epoch || current.Grant != owner.Grant || current.Authority != "local" || current.Role != "owner" {
					return identity.ErrForbidden
				}
				return identity.DirectAllowed(tx, current, "episodes")
			}
			target := metadata.RepairTarget{Kind: "item", ID: episode.Public}
			before, err := meta.RepairState(ctx, target, authorize)
			if err != nil {
				t.Fatal(err)
			}
			result, err := meta.MakeEpisodeThumbnail(ctx, target, metadata.RepairCommand{Action: "make_episode_thumbnail", ExpectedRevision: before.Revision}, owner, authorize)
			if err != nil {
				t.Fatal("actual decode/publication", err)
			}
			if result.Revision == before.Revision {
				t.Fatal("decoded frame did not change publication")
			}
			picture, mime, err := meta.Artwork(ctx, episode.Public, "still")
			if err != nil {
				t.Fatal("published image bytes", err)
			}
			dimensions, err := jpeg.DecodeConfig(picture)
			picture.Close()
			if err != nil || mime != "image/jpeg" || dimensions.Width != 320 || dimensions.Height != 180 {
				t.Fatal("actual selected picture", dimensions, mime, err)
			}
			var provider, storedMode, tier string
			var locked bool
			if err := c.DB.QueryRow(`SELECT candidate.provider,selection.locked FROM artwork_selections selection JOIN artwork_candidates candidate ON candidate.id=selection.candidate_id WHERE selection.kind='item' AND selection.entity_id=? AND selection.role='still'`, episode.ID).Scan(&provider, &locked); err != nil {
				t.Fatal(err)
			}
			if err := c.DB.QueryRow(`SELECT episode_thumbnail_mode,tier FROM library_scan_policies WHERE library_id='episodes'`).Scan(&storedMode, &tier); err != nil {
				t.Fatal(err)
			}
			if provider != "generated" || !locked || storedMode != mode || tier != "file_list_only" {
				t.Fatal("publication fabricated a provider or changed automatic policy", provider, locked, storedMode, tier)
			}
		})
	}
}
