package playback

import (
	"context"
	"path/filepath"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/persistence"
)

// A subtitle change (burn-in on or off) replans a session under a new
// generation while its old converter may still be winding down. The old
// producer's failure must not fail the presentation that replaced it; the
// current producer's still does.
func TestHLSFailureFencedToProducerGeneration(t *testing.T) {
	db, e := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := db.Exec(q, args...); e != nil {
			t.Fatal(e)
		}
	}
	scalar := func(q string, args ...any) string {
		t.Helper()
		var v string
		if e := db.QueryRow(q, args...).Scan(&v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	id, _, token := catalogFixture(t, db, "library", "/isolated/no-media", compactcatalog.Movie, "item", "Test", compactcatalog.Asset{Path: "/isolated/no-media/test.mp4", Size: 1, ModifiedNS: 1, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080, Duration: 100})
	exec(`INSERT INTO playback_sessions(id,session_hash,account_id,profile_id,item_id,asset_id,generation,state,grant_hash,grant_token,expires_at,request_id,duration) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"fenced", "session", "owner", "profile", id, token, 3, "playing", "grant_fenced", "token", "2100-01-01T00:00:00Z", "session_fenced", 100.0)
	h := &HLS{db: db}
	if h.failedWith(withProducerGeneration(context.Background(), 2), "fenced", FailureConverter, "stale"); scalar(`SELECT state FROM playback_sessions WHERE id='fenced'`) != "playing" {
		t.Fatal("a replaced generation's producer failed the current presentation")
	}
	if !h.failed(withProducerGeneration(context.Background(), 3), "fenced") || scalar(`SELECT state FROM playback_sessions WHERE id='fenced'`) != "failed" {
		t.Fatal("the current producer's failure was ignored")
	}
}
