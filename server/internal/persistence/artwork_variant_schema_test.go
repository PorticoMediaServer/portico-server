package persistence

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestArtworkVariantWidthsRemainClosed(t *testing.T) {
	db, err := OpenFresh(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const source = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := db.Exec(`INSERT INTO artwork_objects(digest,mime,width,height,bytes,created_at) VALUES(?,'image/jpeg',4096,4096,100000,'2026-10-23T00:00:00Z')`, source); err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{400, 800, 1920, 2560} {
		digest := fmt.Sprintf("%064x", width)
		if _, err := db.Exec(`INSERT INTO artwork_objects(digest,mime,width,height,bytes,created_at) VALUES(?,'image/jpeg',?,?,10000,'2026-10-23T00:00:00Z')`, digest, width, width); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO artwork_variants(source_digest,width,digest) VALUES(?,?,?)`, source, width, digest); err != nil {
			t.Fatalf("rejected supported variant width %d: %v", width, err)
		}
		var stored string
		if err := db.QueryRow(`SELECT digest FROM artwork_variants WHERE source_digest=? AND width=?`, source, width).Scan(&stored); err != nil || stored != digest {
			t.Fatalf("variant width %d readback: digest=%q err=%v", width, stored, err)
		}
	}
	for _, width := range []any{nil, -1, 0, 399, 401, 799, 801, 1919, 1921, 2559, 2561, 3840, 2560.5, "automatic"} {
		if _, err := db.Exec(`INSERT INTO artwork_variants(source_digest,width,digest) VALUES(?,?,?)`, source, width, fmt.Sprintf("%064x", 2560)); err == nil {
			t.Fatalf("accepted unsupported variant width %v", width)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM artwork_variants WHERE source_digest=?`, source).Scan(&count); err != nil || count != 4 {
		t.Fatalf("invalid widths changed valid variants: count=%d err=%v", count, err)
	}
}
