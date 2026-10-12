package dvr

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/persistence"
)

type shortStorage struct{ err error }

func (s *shortStorage) Measure(context.Context) (StorageMeasurement, error) {
	return StorageMeasurement{}, nil
}
func (s *shortStorage) CheckFloor(context.Context) error     { return s.err }
func (s *shortStorage) SetPolicySource(func() StoragePolicy) {}

// Retention and queued deletions run after the claims in the same pass. A
// recording that comes due on a full disk therefore waits while space is
// about to be freed, and fails only once nothing more is coming or its own
// window has passed.
func TestStorageShortageWaitsForQueuedDeletionsBeforeFailingARecording(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	src, gen, ch := strings.Repeat("a", 48), strings.Repeat("b", 48), strings.Repeat("c", 64)
	now := time.Now().UTC().Truncate(time.Second)
	start, end := now.Add(time.Hour), now.Add(2*time.Hour)
	programmes := []string{strings.Repeat("d", 64), strings.Repeat("e", 64), strings.Repeat("f", 64)}
	for _, q := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO live_source_identities VALUES(?)`, []any{src}},
		{`INSERT INTO live_generations VALUES(?,?,'Guide',2,1,1,?)`, []any{gen, src, now.Format(time.RFC3339)}},
		{`INSERT INTO live_sources VALUES(?,'Aerial',1,'active',2,?,?)`, []any{src, gen, now.Format(time.RFC3339)}},
		{`INSERT INTO live_channel_versions VALUES(?,?,'channel','Channel','1','',1,x'00','','')`, []any{gen, ch}},
	} {
		if _, err = db.Exec(q.q, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	for n, programme := range programmes {
		if _, err = db.Exec(`INSERT INTO live_programmes VALUES(?,?,?,?,'Episode',?,?,?)`, gen, programme, ch, "programme-"+string(rune('a'+n)), start.Format(time.RFC3339), end.Format(time.RFC3339), "lineage-"+string(rune('a'+n))); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO live_programme_metadata(generation_id,id,series_id,episode_id,new_evidence,description) VALUES(?,?,'','','','')`, gen, programme); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	clock := now
	storage := &shortStorage{err: ErrStorageFloor}
	s := &Store{db: db, live: &livechannels.Store{}, now: func() time.Time { return clock }, captureAvailable: true, storage: storage, durable: func(context.Context, *sql.Tx, livechannels.Owner, string, string) error { return nil }}
	owner := livechannels.Owner{Authority: "local", AccountID: "account", ProfileID: "profile"}
	auth := func(context.Context, *sql.Tx, bool) (string, func(string, string) bool, error) {
		return "owner", func(string, string) bool { return true }, nil
	}
	schedule := func(n int) string {
		t.Helper()
		r, err := s.Schedule(ctx, auth, owner, ScheduleInput{RequestID: strings.Repeat(string(rune('1'+n)), 48), Occurrence: Occurrence{src, ch, gen, programmes[n]}, UseDefaults: true})
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	state := func(id string) (state, reason string) {
		t.Helper()
		if err := db.QueryRow(`SELECT state,reason FROM dvr_recordings WHERE id=?`, id).Scan(&state, &reason); err != nil {
			t.Fatal(err)
		}
		return state, reason
	}
	attempt := func() {
		t.Helper()
		clock = clock.Add(4 * time.Second)
		if c, _, err := s.claimNext(ctx); err != nil || c != nil {
			t.Fatalf("claim on a full disk: %v %v", c, err)
		}
	}
	queued := schedule(1)
	if _, err = db.Exec(`UPDATE dvr_recordings SET state='pending-delete',start_ms=1,end_ms=2 WHERE id=?`, queued); err != nil {
		t.Fatal(err)
	}
	recording := schedule(0)
	clock = start.Add(time.Minute)

	// The first look at a full disk does not end the recording: this pass has
	// not yet run retention or its queued deletions.
	attempt()
	if st, reason := state(recording); st != "scheduled" || reason != "storage-floor" {
		t.Fatalf("a recording was ended before anything could free space: %s (%s)", st, reason)
	}
	// While a deletion is queued, space is on its way and the recording waits.
	if _, err = db.Exec(`INSERT INTO dvr_delete_work VALUES(?,1,?,?,'waiting',0,'')`, queued, clock.UnixMilli(), clock.Add(2*time.Minute).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	attempt()
	attempt()
	if st, reason := state(recording); st != "scheduled" || reason != "storage-floor" {
		t.Fatalf("a recording was ended while a deletion was still queued: %s (%s)", st, reason)
	}
	// Nothing more is coming and the disk is still full: now it fails, and says why.
	if _, err = db.Exec(`DELETE FROM dvr_delete_work`); err != nil {
		t.Fatal(err)
	}
	attempt()
	if st, reason := state(recording); st != "failed" || reason != "storage-floor" {
		t.Fatalf("a recording with no space and nothing queued did not fail: %s (%s)", st, reason)
	}

	// A queued deletion never holds a recording past its own window.
	late := schedule(2)
	if _, err = db.Exec(`INSERT INTO dvr_delete_work VALUES(?,1,?,?,'waiting',0,'')`, queued, clock.UnixMilli(), clock.Add(2*time.Minute).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	storage.err = ErrStorageCap
	attempt()
	attempt()
	if st, reason := state(late); st != "scheduled" || reason != "storage-cap" {
		t.Fatalf("a recording over the cap did not wait for the queued deletion: %s (%s)", st, reason)
	}
	clock = end.Add(time.Hour)
	attempt()
	if st, reason := state(late); st != "failed" || reason != "storage-cap" {
		t.Fatalf("a recording kept waiting after its window had passed: %s (%s)", st, reason)
	}
}
