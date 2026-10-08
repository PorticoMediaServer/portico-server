package remotesources

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/storage"
)

// This executable is both the real supervised native helper and its synthetic
// rclone child. No provider, FUSE mount or credential is needed by these tests.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--portico-rclone-native" {
		if err := mounts.NativeHelper(os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Println("rclone vfixture")
		os.Exit(0)
	}
	if len(os.Args) > 2 && os.Args[1] == "lsjson" {
		for _, arg := range os.Args {
			if arg == "--stat" {
				fmt.Print(`{"Path":"","Name":"root","Size":0,"IsDir":true,"ID":"fixture-root","ModTime":"2026-01-01T00:00:00Z"}`)
				os.Exit(0)
			}
		}
		_, control, ok := strings.Cut(os.Args[2], ":")
		if !ok {
			os.Exit(2)
		}
		f, err := os.OpenFile(filepath.Join(control, "starts"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			os.Exit(2)
		}
		fmt.Fprintln(f, "start")
		f.Close()
		fmt.Print("[")
		entry := func(i int) {
			if i > 0 {
				fmt.Print(",")
			}
			raw, _ := json.Marshal(mounts.NativeEntry{Path: fmt.Sprintf("entry-%04d", i), Name: fmt.Sprintf("entry-%04d", i), Size: 1, ID: fmt.Sprint(i), ModTime: "2026-01-01T00:00:00Z"})
			os.Stdout.Write(raw)
		}
		for i := 0; i < 128; i++ {
			entry(i)
		}
		// Retain the child and its already committed first batch until the test
		// releases it, deliberately outliving each request/worker-quantum context.
		deadline := time.Now().Add(40 * time.Second)
		for {
			if _, err = os.Stat(filepath.Join(control, "release")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				os.Exit(3)
			}
			time.Sleep(10 * time.Millisecond)
		}
		for i := 128; i < 130; i++ {
			entry(i)
		}
		fmt.Print("]")
		if mode, _ := os.ReadFile(filepath.Join(control, "mode")); string(mode) == "bad-tail" {
			fmt.Print(" trailing-invalid-data")
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type nativeFixture struct {
	s                      *Service
	db                     *sql.DB
	root, id, control, job string
}

func newNativeFixture(t *testing.T) nativeFixture {
	t.Helper()
	state, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	control := filepath.Join(state, "provider")
	if err = os.Mkdir(control, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := persistence.Open(filepath.Join(state, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	binary := secureFixtureExecutable(t)
	managed, err := mounts.New(db, state, "", binary, storage.New(binary))
	if err != nil {
		t.Fatal(err)
	}
	mount, err := managed.Create(context.Background(), mounts.CreateInput{Name: "Fixture", Executable: binary, Remote: "fixture:" + control, Config: "[fixture]\ntype = s3\nprovider = AWS\naccess_key_id = fixture\nsecret_access_key = fixture\n"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(db, state, managed)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.stopNativeListings()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			s.listings.Lock()
			n := len(s.nativeListings)
			s.listings.Unlock()
			if n == 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Error("native producer did not retire")
	})
	c := catalog.New(db)
	lib, err := c.Create("Remote", "movie", mount.MountPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE library_sources SET kind='remote',root_identity='fixture-root',identity_confirmed=1 WHERE id=?`, lib.ID); err != nil {
		t.Fatal(err)
	}
	job := "native-fixture-job"
	if _, err = db.Exec(`INSERT INTO jobs(id,library_id,status,created_at) VALUES(?,?,'running',?)`, job, lib.ID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO inventory_runs(job_id,source_id,source_generation,root_incarnation,root_identity,policy_revision,phase) SELECT ?,id,generation,incarnation,root_identity,1,'inventory' FROM library_sources WHERE id=?`, job, lib.ID); err != nil {
		t.Fatal(err)
	}
	return nativeFixture{s, db, mount.MountPath, mount.ID, control, job}
}
func waitNative(t *testing.T, f nativeFixture, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("native producer did not reach expected state")
}
func (f nativeFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func (f nativeFixture) release(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.control, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
}
func (f nativeFixture) starts(t *testing.T) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.control, "starts"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(raw), "start")
}
func TestNativeListingRetainsProgressAcrossCancelledRequests(t *testing.T) {
	f := newNativeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	pending, err := f.s.ListPage(ctx, f.job, f.root, "")
	cancel()
	if err != nil || !pending.Pending || len(pending.Entries) != 0 || pending.Complete {
		t.Fatal("first response not private pending", pending, err)
	}
	waitNative(t, f, func() bool {
		return f.count(t, `SELECT count(*) FROM remote_listing_entries WHERE listing_id=?`, pending.ID) == 128
	})
	for i := 0; i < 8; i++ {
		ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
		p, e := f.s.ListPage(ctx, f.job, f.root, pending.Next)
		cancel()
		if e != nil || !p.Pending || p.ID != pending.ID || p.Complete || len(p.Entries) != 0 {
			t.Fatal("partial snapshot became visible/restarted", p, e)
		}
		if f.count(t, `SELECT count(*) FROM remote_listing_entries WHERE listing_id=?`, pending.ID) != 128 {
			t.Fatal("poll deleted retained progress")
		}
	}
	if f.starts(t) != 1 {
		t.Fatal("provider restarted for a request retry")
	}
	// A partial listing cannot be passed off as complete even inside a caller tx.
	forged := pending
	forged.Pending = false
	forged.Complete = true
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	err = f.s.CommitPage(context.Background(), tx, forged)
	tx.Rollback()
	if !errors.Is(err, storage.ErrRemoteCursor) {
		t.Fatal("partial listing accepted as absence proof", err)
	}
	f.release(t)
	waitNative(t, f, func() bool {
		return f.count(t, `SELECT ready FROM remote_listing_sessions WHERE id=?`, pending.ID) == 1
	})
	// Publication may race the commit of the last empty Pending response.
	tx, err = f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.CommitPage(context.Background(), tx, pending); err != nil {
		tx.Rollback()
		t.Fatal("pending/ready race failed scan", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	p, err := f.s.ListPage(context.Background(), f.job, f.root, pending.Next)
	if err != nil || p.Pending || p.Complete || len(p.Entries) != 128 {
		t.Fatal("first complete-snapshot page", len(p.Entries), err)
	}
	last, err := f.s.ListPage(context.Background(), f.job, f.root, p.Next)
	if err != nil || last.Pending || !last.Complete || len(last.Entries) != 2 {
		t.Fatal("snapshot EOF", last, err)
	}
	if f.starts(t) != 1 || f.count(t, `SELECT entries FROM remote_native_listings WHERE listing_id=?`, p.ID) != 130 {
		t.Fatal("producer restarted or lost rows")
	}
}
func TestNativeListingInvalidationCannotPublishPartialSnapshot(t *testing.T) {
	for _, change := range []string{"credentials", "incarnation", "cancel", "bad-tail"} {
		t.Run(change, func(t *testing.T) {
			f := newNativeFixture(t)
			pending, err := f.s.ListPage(context.Background(), f.job, f.root, "")
			if err != nil {
				t.Fatal(err)
			}
			waitNative(t, f, func() bool {
				return f.count(t, `SELECT count(*) FROM remote_listing_entries WHERE listing_id=?`, pending.ID) == 128
			})
			switch change {
			case "credentials":
				_, err = f.db.Exec(`UPDATE mount_backend_configs SET generation=generation+1 WHERE mount_id=?`, f.id)
				f.s.invalidate(f.id)
			case "incarnation":
				_, err = f.db.Exec(`UPDATE library_sources SET incarnation='replacement' WHERE id=(SELECT source_id FROM inventory_runs WHERE job_id=?)`, f.job)
			case "cancel":
				_, err = f.db.Exec(`UPDATE jobs SET status='cancelled' WHERE id=?`, f.job)
			case "bad-tail":
				err = os.WriteFile(filepath.Join(f.control, "mode"), []byte("bad-tail"), 0600)
				f.release(t)
			}
			if err != nil {
				t.Fatal(err)
			}
			waitNative(t, f, func() bool {
				return f.count(t, `SELECT count(*) FROM remote_native_listings WHERE listing_id=? AND state='failed'`, pending.ID) == 1
			})
			waitNative(t, f, func() bool {
				return f.count(t, `SELECT count(*) FROM remote_listing_entries WHERE listing_id=?`, pending.ID) == 0
			})
			if f.count(t, `SELECT ready FROM remote_listing_sessions WHERE id=?`, pending.ID) != 0 {
				t.Fatal("invalidated stream published completion")
			}
		})
	}
}
func TestNativeListingCrashAdoptsOnlyUnpublishedSnapshot(t *testing.T) {
	f := newNativeFixture(t)
	if _, err := f.db.Exec(`INSERT INTO remote_listing_sessions(id,job_id,source_id,generation,directory,created_at) VALUES('crashed',?,?,'1',?,?)`, f.job, f.id, f.root, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO remote_native_listings(listing_id,owner,state,updated_at,entries,bytes) VALUES('crashed','dead-process','running',0,1,2); INSERT INTO remote_listing_entries VALUES('crashed','stale-orphan','{}')`); err != nil {
		t.Fatal(err)
	}
	p, err := f.s.ListPage(context.Background(), f.job, f.root, "")
	if err != nil || p.ID != "crashed" {
		t.Fatal(p, err)
	}
	waitNative(t, f, func() bool {
		return f.count(t, `SELECT count(*) FROM remote_listing_entries WHERE listing_id='crashed'`) == 128
	})
	if f.count(t, `SELECT count(*) FROM remote_listing_entries WHERE listing_id='crashed' AND name='stale-orphan'`) != 0 {
		t.Fatal("mixed a crashed partial snapshot with a new one")
	}
	f.release(t)
	waitNative(t, f, func() bool { return f.count(t, `SELECT ready FROM remote_listing_sessions WHERE id='crashed'`) == 1 })
	if f.starts(t) != 1 {
		t.Fatal("crash adoption repeatedly restarted")
	}
}
func TestNativeListingLongerThanOriginalQuantum(t *testing.T) {
	if os.Getenv("PORTICO_R01_SLOW_TEST") != "1" {
		t.Skip("opt-in 27-second original-timeout regression")
	}
	f := newNativeFixture(t)
	p, err := f.s.ListPage(context.Background(), f.job, f.root, "")
	if err != nil {
		t.Fatal(err)
	}
	waitNative(t, f, func() bool {
		return f.count(t, `SELECT count(*) FROM remote_listing_entries WHERE listing_id=?`, p.ID) == 128
	})
	until := time.Now().Add(27 * time.Second)
	for time.Now().Before(until) {
		next, e := f.s.ListPage(context.Background(), f.job, f.root, p.Next)
		if e != nil || !next.Pending {
			t.Fatal(next, e)
		}
		time.Sleep(time.Second)
	}
	f.release(t)
	waitNative(t, f, func() bool { return f.count(t, `SELECT ready FROM remote_listing_sessions WHERE id=?`, p.ID) == 1 })
	if f.starts(t) != 1 {
		t.Fatal("listing beyond 25 seconds restarted")
	}
}

// secureFixtureExecutable copies the native helper with immutable permissions regardless
// of the umask used by go test. Never change the running test executable.
func secureFixtureExecutable(t *testing.T) string {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "rclone-fixture")
	if err = os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return path
}
