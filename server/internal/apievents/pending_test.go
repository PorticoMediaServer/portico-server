package apievents

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/persistence"
	"testing"
)

func TestProfileHintsCoalesceAndKeepAudience(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "pending.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := persistence.PersonalOwnerKey("local", "account", "profile")
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, rev := range []string{"1", "2", "3"} {
		_, err = tx.Exec(`INSERT INTO api_change_pending(audience,type,resource_kind,resource_id,revision) VALUES(?,'progress.changed','library','films',?) ON CONFLICT(audience,type,resource_kind,resource_id) DO UPDATE SET revision=excluded.revision`, "personal:"+key, rev)
		if err != nil {
			t.Fatal(err)
		}
	}
	n, err := Flush(context.Background(), tx, 64)
	if err != nil || n != 1 {
		t.Fatalf("flush %d %v", n, err)
	}
	var audience, revision string
	if err = tx.QueryRow(`SELECT audience,revision FROM api_events WHERE type='progress.changed'`).Scan(&audience, &revision); err != nil || audience != ProfileAudience("local", "account", "profile") || revision != "3" {
		t.Fatalf("%q %q %v", audience, revision, err)
	}
}
