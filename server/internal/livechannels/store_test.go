package livechannels

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/persistence"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, e := persistence.Open(filepath.Join(t.TempDir(), "guide.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	s, e := New(db)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func testAuthority(fence string, owner bool, allowed bool) Authority {
	return func(context.Context, *sql.Tx, bool) (string, func(string, string) bool, error) {
		if !owner {
			return "", nil, ErrDenied
		}
		return fence, func(string, string) bool { return allowed }, nil
	}
}
func testInput() SourceInput {
	return SourceInput{ID: strings.Repeat("ab", 24), RequestID: strings.Repeat("cd", 24), Name: "Test broadcast", TunerCount: 1, Playlist: "#EXTM3U\n#EXTINF:-1 tvg-id=\"one\" tvg-chno=\"1\" group-title=\"Local\",Channel one\nhttps://provider.invalid/stream?secret=private-source-value\n#EXTINF:-1 tvg-id=\"two\",Channel two\nhttps://provider.invalid/two\n", Guide: `<?xml version="1.0"?><tv><channel id="one"/><programme id="show-1" channel="one" start="20260905120000 +0000" stop="20260905130000 +0000"><title>Afternoon programme</title></programme><programme channel="two" start="20260905120000 +0000" stop="20260905130000 +0000"><title>Second programme</title></programme></tv>`}
}
func testQuery() GuideQuery {
	start, _ := time.Parse(time.RFC3339, "2026-09-05T12:00:00Z")
	return GuideQuery{Start: start, End: start.Add(2 * time.Hour), Timezone: "America/Halifax", Kind: LiveSource, Limit: 1}
}
func TestPreviewSavePlaintextAndConsumerBoundary(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a := testAuthority("viewer-a", true, true)
	in := testInput()
	p, e := PreviewSource(in)
	if e != nil || p.Channels != 2 || p.Programmes != 2 {
		t.Fatalf("preview %#v %v", p, e)
	}
	sources, e := s.Sources(ctx, a)
	if e != nil || len(sources) != 0 {
		t.Fatal("preview published source")
	}
	saved, e := s.Save(ctx, a, in)
	if e != nil {
		t.Fatal(e)
	}
	var stored []byte
	if e = s.db.QueryRow(`SELECT locator_sealed FROM live_channel_versions WHERE generation_id=? ORDER BY position LIMIT 1`, saved.Generation).Scan(&stored); e != nil {
		t.Fatal(e)
	}
	// Plain storage: the locator sits in the row as-is.
	if !bytes.Contains(stored, []byte("private-source-value")) {
		t.Fatal("locator not stored plainly")
	}
	locator, e := s.unseal(stored, id(saved.ID, "one")+":"+saved.Generation)
	if e != nil || !strings.Contains(locator, "private-source-value") {
		t.Fatal("locator not recoverable", e)
	}
	guide, e := s.Guide(ctx, a, testQuery())
	if e != nil || len(guide.Channels) != 1 || len(guide.Channels[0].Programmes) != 1 {
		t.Fatalf("guide %#v %v", guide, e)
	}
	body, _ := json.Marshal(guide)
	if bytes.Contains(body, []byte("provider.invalid")) || bytes.Contains(body, []byte("private-source-value")) {
		t.Fatal("consumer leaks locator")
	}
	if guide.Channels[0].TuneAvailable {
		t.Fatal("unimplemented tune advertised")
	}
}
func TestRefreshCASAndStableCursor(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a := testAuthority("viewer-a", true, true)
	in := testInput()
	saved, e := s.Save(ctx, a, in)
	if e != nil {
		t.Fatal(e)
	}
	first, e := s.Guide(ctx, a, testQuery())
	if e != nil || first.NextCursor == "" {
		t.Fatal("missing continuation", e)
	}
	q := testQuery()
	q.Cursor = first.NextCursor
	second, e := s.Guide(ctx, a, q)
	if e != nil || len(second.Channels) != 1 || second.Channels[0].ID == first.Channels[0].ID {
		t.Fatal("bad continuation", e)
	}
	if _, e = s.Guide(ctx, testAuthority("viewer-b", true, true), q); !errors.Is(e, ErrCursor) {
		t.Fatal("cross-viewer cursor accepted")
	}
	in.ID = saved.ID
	in.ExpectedRevision = saved.Revision
	in.RequestID = strings.Repeat("bb", 24)
	in.Guide = strings.Replace(in.Guide, "Afternoon programme", "Updated programme", 1)
	updated, e := s.Save(ctx, a, in)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Guide(ctx, a, q); !errors.Is(e, ErrCursor) {
		t.Fatal("old generation cursor accepted")
	}
	in.RequestID = strings.Repeat("cc", 24)
	if _, e = s.Save(ctx, a, in); !errors.Is(e, ErrConflict) {
		t.Fatal("stale write accepted")
	}
	in.ExpectedRevision = updated.Revision
	in.Guide = "<tv><programme>broken</programme></tv>"
	if _, e = s.Save(ctx, a, in); !errors.Is(e, ErrInvalid) {
		t.Fatal("invalid refresh accepted")
	}
	current, e := s.Guide(ctx, a, testQuery())
	if e != nil || current.Channels[0].Generation != updated.Generation || current.Channels[0].Programmes[0].Title != "Updated programme" {
		t.Fatal("last good guide lost", e)
	}
}
func TestDeniedCommitRollsBackAndDisableHides(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	calls := 0
	a := func(context.Context, *sql.Tx, bool) (string, func(string, string) bool, error) {
		calls++
		if calls == 2 {
			return "", nil, ErrDenied
		}
		return "viewer", func(string, string) bool { return true }, nil
	}
	if _, e := s.Save(ctx, a, testInput()); !errors.Is(e, ErrDenied) {
		t.Fatal("late denial ignored")
	}
	var n int
	s.db.QueryRow(`SELECT count(*) FROM live_sources`).Scan(&n)
	if n != 0 {
		t.Fatal("denied save committed")
	}
	owner := testAuthority("viewer", true, true)
	saved, e := s.Save(ctx, owner, testInput())
	if e != nil {
		t.Fatal(e)
	}
	guide, e := s.Guide(ctx, testAuthority("other", true, false), testQuery())
	if e != nil || len(guide.Channels) != 0 {
		t.Fatal("denied channel disclosed")
	}
	if e = s.SetEnabled(ctx, owner, saved.ID, saved.Revision, false, strings.Repeat("dd", 24)); e != nil {
		t.Fatal(e)
	}
	guide, e = s.Guide(ctx, owner, testQuery())
	if e != nil || len(guide.Channels) != 0 {
		t.Fatal("disabled source visible")
	}
}
func TestParserRejectsHostileAndAmbiguousData(t *testing.T) {
	base := testInput()
	for name, change := range map[string]func(*SourceInput){"credentials": func(v *SourceInput) {
		v.Playlist = strings.Replace(v.Playlist, "https://provider.invalid/stream", "https://user:password@provider.invalid/stream", 1)
	}, "missing-id": func(v *SourceInput) { v.Playlist = strings.Replace(v.Playlist, `tvg-id="one"`, "", 1) }, "bad-time": func(v *SourceInput) {
		v.Guide = strings.Replace(v.Guide, "20260905130000 +0000", "20260905110000 +0000", 1)
	}, "entity": func(v *SourceInput) { v.Guide = `<!DOCTYPE tv [<!ENTITY x SYSTEM "file:///etc/passwd">]><tv/>` }, "oversize": func(v *SourceInput) { v.Guide = strings.Repeat("x", MaxUploadBytes+1) }} {
		t.Run(name, func(t *testing.T) {
			in := base
			change(&in)
			if _, e := PreviewSource(in); !errors.Is(e, ErrInvalid) {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}

func TestOperationReplayDisableRefreshAndCoverage(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a := testAuthority("viewer", true, true)
	in := testInput()
	saved, e := s.Save(ctx, a, in)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Save(ctx, a, in)
	if e != nil || again.Generation != saved.Generation || again.Revision != saved.Revision {
		t.Fatal("lost-response replay changed operation", e)
	}
	in.Name = "Different input"
	if _, e = s.Save(ctx, a, in); !errors.Is(e, ErrConflict) {
		t.Fatal("reused request accepted different input")
	}
	request := strings.Repeat("11", 24)
	if e = s.SetEnabled(ctx, a, saved.ID, saved.Revision, false, request); e != nil {
		t.Fatal(e)
	}
	if e = s.SetEnabled(ctx, a, saved.ID, saved.Revision, false, request); e != nil {
		t.Fatal("enablement replay failed", e)
	}
	in = testInput()
	in.ExpectedRevision = saved.Revision + 1
	in.RequestID = strings.Repeat("22", 24)
	refresh, e := s.Save(ctx, a, in)
	if e != nil || refresh.State != "disabled" {
		t.Fatal("refresh re-enabled source", e)
	}
	if e = s.SetEnabled(ctx, a, refresh.ID, refresh.Revision, true, strings.Repeat("33", 24)); e != nil {
		t.Fatal(e)
	}
	guide, e := s.Guide(ctx, a, testQuery())
	if e != nil || len(guide.Sources) != 1 || guide.Sources[0].AvailableStart != "2026-09-05T12:00:00Z" || guide.Sources[0].AvailableEnd != "2026-09-05T13:00:00Z" {
		t.Fatalf("coverage %#v %v", guide.Sources, e)
	}
	if refresh.DeliveryValidation != "not-validated" {
		t.Fatal("source validation fabricated")
	}
}
func TestInterruptedStageResumesAndConfigurationChangeFences(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in := testInput()
	owner := testAuthority("viewer", true, true)
	var channelStage *sql.Tx
	interrupted := func(ctx context.Context, tx *sql.Tx, admin bool) (string, func(string, string) bool, error) {
		var staged int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM live_channel_versions c JOIN live_operations o ON o.generation_id=c.generation_id WHERE o.request_id=? AND o.status='staging'`, in.RequestID).Scan(&staged); err != nil {
			t.Fatal(err)
		}
		if staged == 2 {
			if channelStage == nil {
				channelStage = tx
			} else if channelStage != tx {
				// The channel batch is now committed. Interrupt the next batch,
				// independent of how many authorization checks preceded it.
				return "", nil, context.DeadlineExceeded
			}
		}
		return owner(ctx, tx, admin)
	}
	if _, e := s.Save(ctx, interrupted, in); !errors.Is(e, ErrUnavailable) {
		t.Fatal("stage interruption classification", e)
	}
	var n int
	s.db.QueryRow(`SELECT count(*) FROM live_channel_versions`).Scan(&n)
	if n != 2 {
		t.Fatal("expected durable staged channels", n)
	}
	sources, e := s.Sources(ctx, owner)
	if e != nil || len(sources) != 0 {
		t.Fatal("staging appeared published")
	}
	saved, e := s.Save(ctx, owner, in)
	if e != nil {
		t.Fatal("resume failed", e)
	}
	in.ExpectedRevision = saved.Revision
	in.RequestID = strings.Repeat("44", 24)
	channelStage = nil
	if _, e = s.Save(ctx, interrupted, in); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	if e = s.SetEnabled(ctx, owner, saved.ID, saved.Revision, false, strings.Repeat("55", 24)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Save(ctx, owner, in); !errors.Is(e, ErrConflict) {
		t.Fatal("old staging overwrote newer source", e)
	}
	sources, e = s.Sources(ctx, owner)
	if e != nil || len(sources) != 1 || sources[0].State != "disabled" || sources[0].Generation != saved.Generation {
		t.Fatal("source fence failed")
	}
}

// Rows sealed under the old source key migrate to plaintext, and the key
// file goes away.
func TestSealedPreviewMigratesToPlain(t *testing.T) {
	dir := t.TempDir()
	db, e := persistence.OpenFresh(filepath.Join(dir, "guide.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	key := make([]byte, 32)
	if _, e = rand.Read(key); e != nil {
		t.Fatal(e)
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		t.Fatal(e)
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		t.Fatal(e)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		t.Fatal(e)
	}
	sealed := aead.Seal(nonce, nonce, []byte("https://provider.invalid/guide"), []byte("preview:preview-one"))
	if _, e = db.Exec(`INSERT INTO live_remote_previews(id,source_id,expected_revision,fence,sealed,expires_ms) VALUES('preview-one','source-one',1,'',?,0)`, sealed); e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(filepath.Join(dir, "keys"), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "keys", "live-sources.v1.key"), key, 0600); e != nil {
		t.Fatal(e)
	}
	MigrateSealedToPlain(db, dir)
	var stored []byte
	if e = db.QueryRow(`SELECT sealed FROM live_remote_previews WHERE id='preview-one'`).Scan(&stored); e != nil {
		t.Fatal(e)
	}
	if string(stored) != "https://provider.invalid/guide" {
		t.Fatal("sealed preview not migrated to plaintext")
	}
	if _, e = os.Stat(filepath.Join(dir, "keys", "live-sources.v1.key")); !os.IsNotExist(e) {
		t.Fatal("key file left behind")
	}
}
