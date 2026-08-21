package app

import (
	"encoding/hex"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestStableOpaquePublicResourceIDPersistsWithoutExposingIdentityMaterial(t *testing.T) {
	_, db, _ := newDiscoveryTestServer(t, config.Config{})
	identityMaterial := "media-id\x00file-id\x00/private/library/Movie.en.srt"
	resolve := func() string {
		tx, err := db.Begin()
		if err != nil {
			t.Fatalf("begin identity transaction: %v", err)
		}
		id, err := stableOpaquePublicResourceIDTx(tx, "media-stream", identityMaterial)
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("resolve stable public ID: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit identity transaction: %v", err)
		}
		return id
	}
	first, second := resolve(), resolve()
	if first != second {
		t.Fatalf("stable public ID changed: first=%q second=%q", first, second)
	}
	decoded, err := hex.DecodeString(first)
	if err != nil || len(decoded) != 20 {
		t.Fatalf("stable public ID %q is not opaque 160-bit hex: %v", first, err)
	}
	if first == identityMaterial {
		t.Fatal("public ID exposed identity material")
	}
}
