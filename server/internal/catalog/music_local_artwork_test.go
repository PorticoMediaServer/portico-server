package catalog

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
)

func TestMusicAlbumBackgroundUsesNormalAutomaticSourceAndPreservesOwner(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("music", "Music", "music", "/music")
	album := c.Album(c.Artist(library, "Pink Floyd"), "The Wall", 1979)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		if err := musicAlbumBackdropTx(ctx, tx, album.ID, map[string]string{"album/backdrop": "album-background"}); err != nil {
			return err
		}
		var value string
		if err := tx.QueryRowContext(ctx, `SELECT backdrop_url FROM catalog_item_details WHERE entity_id=?`, album.ID).Scan(&value); err != nil {
			return err
		}
		if value != "local:album-background" {
			t.Fatal(value)
		}
		return nil
	})
	digest := strings.Repeat("a", 64)
	c.Exec(`INSERT INTO artwork_objects(digest,mime,width,height,bytes,created_at) VALUES(?,'image/jpeg',1,1,1,'now')`, digest)
	c.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,provider,image_id,origin,attribution,source_fence,observed_at) VALUES('owned-background','album',?,'backdrop','local','owner','local:owner-background','Local file','fence','now')`, album.ID)
	c.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,candidate_id,digest,thumbnail_digest,locked,actor,observed_at) VALUES('album',?,'backdrop','owned-background',?,?,1,'owner','now')`, album.ID, digest, digest)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return musicAlbumBackdropTx(ctx, tx, album.ID, map[string]string{"album/backdrop": "artist-fallback"})
	})
	target := artworkTarget{Kind: "album", ID: album.Public}
	got, err := New(c.DB).resolveArtwork([]artworkTarget{target})
	if err != nil {
		t.Fatal(err)
	}
	want := selectedArtworkURL("album", album.Public, "backdrop", "", digest, 1920)
	if got[target].BackdropURL != want {
		t.Fatal("automatic art displaced actual owner selection", got[target], want)
	}
	var candidate string
	var locked bool
	if err := c.DB.QueryRow(`SELECT candidate_id,locked FROM artwork_selections WHERE kind='album' AND entity_id=? AND role='backdrop'`, album.ID).Scan(&candidate, &locked); err != nil {
		t.Fatal(err)
	}
	if candidate != "owned-background" || !locked {
		t.Fatal(candidate, locked)
	}
}

func TestMusicAlbumBackgroundChangeRetiresOnlyAutomaticSelectionAndLease(t *testing.T) {
	for _, owner := range []bool{false, true} {
		t.Run(map[bool]string{false: "automatic", true: "owner"}[owner], func(t *testing.T) {
			c := catalogtest.Open(t)
			library := c.Library("music", "Music", "music", "/music")
			album := c.Album(c.Artist(library, "Pink Floyd"), "The Wall", 1979)
			c.Write(func(ctx context.Context, tx *sql.Tx) error {
				return musicAlbumBackdropTx(ctx, tx, album.ID, map[string]string{"album/backdrop": "old-background"})
			})
			digest := strings.Repeat("b", 64)
			actor := ""
			if owner {
				actor = "owner"
			}
			c.Exec(`INSERT INTO artwork_objects(digest,mime,width,height,bytes,created_at) VALUES(?,'image/jpeg',1,1,1,'now')`, digest)
			c.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,provider,image_id,origin,attribution,source_fence,observed_at) VALUES('old-background','album',?,'backdrop','local','old','local:old-background','Local file','fence','now')`, album.ID)
			c.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,candidate_id,digest,thumbnail_digest,locked,actor,observed_at) VALUES('album',?,'backdrop','old-background',?,?,?,?,'now')`, album.ID, digest, digest, owner, actor)
			c.Exec(`INSERT INTO artwork_jobs(id,kind,entity_id,role,candidate_id,origin,provider,source_fence,status,lease,lease_until,actor,locked,created_at) VALUES('old-job','album',?,'backdrop','old-background','local:old-background','local','fence','running','old-lease','later',?,?,'now')`, album.ID, actor, owner)
			c.Write(func(ctx context.Context, tx *sql.Tx) error {
				return musicAlbumBackdropTx(ctx, tx, album.ID, map[string]string{"album/backdrop": "new-background"})
			})
			want := 0
			if owner {
				want = 1
			}
			if got := rows(t, c, `SELECT count(*) FROM artwork_selections WHERE kind='album' AND entity_id=? AND role='backdrop'`, album.ID); got != want {
				t.Fatalf("selection count %d want %d", got, want)
			}
			var status, lease, until string
			if err := c.DB.QueryRow(`SELECT status,lease,lease_until FROM artwork_jobs WHERE id='old-job'`).Scan(&status, &lease, &until); err != nil {
				t.Fatal(err)
			}
			if owner {
				if status != "running" || lease != "old-lease" || until != "later" {
					t.Fatal("owner job was retired", status, lease, until)
				}
			} else if status != "stale" || lease != "" || until != "" {
				t.Fatal("automatic lease survived replacement", status, lease, until)
			}
			var field string
			if err := c.DB.QueryRow(`SELECT backdrop_url FROM catalog_item_details WHERE entity_id=?`, album.ID).Scan(&field); err != nil || field != "local:new-background" {
				t.Fatal(field, err)
			}
			if rows(t, c, `SELECT count(*) FROM artwork_objects WHERE digest=?`, digest) != 1 {
				t.Fatal("replacement removed retained art bytes")
			}
		})
	}
}
