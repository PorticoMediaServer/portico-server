package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
)

func TestReadingLibrarySummaryUsesEligibleLeavesAndGuardedArtwork(t *testing.T) {
	for _, kind := range []string{"book", "comic"} {
		t.Run(kind, func(t *testing.T) {
			c, s, viewer, items := readingBrowseFixture(t, kind)
			libraries := []Library{{ID: "reading", Kind: kind}, {ID: "other", Kind: "book"}}
			if _, err := s.SummarizeLibraries(context.Background(), viewer, libraries); !errors.Is(err, ErrReadingProjectionChanged) {
				t.Fatalf("unready summary must refuse: %v", err)
			}
			leaf := compactcatalog.Ebook
			want := "ebook"
			if kind == "comic" {
				leaf, want = compactcatalog.Comic, "comic"
			}
			// An entity without a publication/link cannot inflate the Reading count.
			c.Entity(compactcatalog.Entity{Library: c.Handle("reading"), Kind: leaf, Key: "orphan", Title: "No publication"}, nil)
			digest := strings.Repeat("a", 64)
			c.Exec(`INSERT INTO artwork_blobs(digest,mime,width,height,bytes,created_at,status) VALUES(?,'image/png',400,600,100,'now','ready'); INSERT OR IGNORE INTO artwork_sources(digest,accepted_mime,accepted_width,accepted_height,small_blob,large_blob,ambient_blob,large_edge,small_outlier,large_outlier,encoder_revision,created_at) SELECT digest,mime,width,height,digest,digest,digest,1920,0,0,1,created_at FROM artwork_blobs`, digest)
			c.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,subject,provider,image_id,origin,attribution,source_fence,observed_at) VALUES('reading-cover','item',?,'cover','','fixture','cover','local','fixture','fence','now')`, items[0].ID)
			c.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,locked,revision,actor,observed_at) SELECT column1,column2,column3,column4,column5,column6,column8,column9,column10,column11 FROM (VALUES('item',?,'cover','','reading-cover',?,?,0,1,'fixture','now'))`, items[0].ID, digest, digest)
			c.Drain()
			summaries, err := s.SummarizeLibraries(context.Background(), viewer, libraries)
			if err != nil {
				t.Fatal(err)
			}
			if len(summaries) != 1 {
				t.Fatalf("library scope: %+v", summaries)
			}
			got := summaries["reading"]
			if len(got.Counts) != 1 || got.Counts[0].Kind != want || got.Counts[0].Count != 4 || got.Counts[0].AtLeast || got.Empty || !got.EmptyKnown {
				t.Fatalf("unavailable eligible leaves must remain counted: %+v", got)
			}
			if len(got.CoverURLs) != 1 || !strings.HasPrefix(got.CoverURLs[0], "/v1/metadata/item/"+items[0].Public+"/art/cover?v="+digest) {
				t.Fatalf("selected guarded Reading cover: %+v", got)
			}
			viewer.Restrictions = identity.ContentRestrictions{BlockUnrated: true}
			summaries, err = s.SummarizeLibraries(context.Background(), viewer, libraries)
			if err != nil {
				t.Fatal(err)
			}
			got = summaries["reading"]
			if got.Counts[0].Count != 0 || !got.Empty || !got.EmptyKnown || len(got.CoverURLs) != 0 || got.LatestArtwork != "" {
				t.Fatalf("restricted leaves or cover escaped: %+v", got)
			}
		})
	}
}
