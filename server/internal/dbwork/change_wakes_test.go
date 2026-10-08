package dbwork

import (
	"context"
	"database/sql"
	"path/filepath"
	"portico.local/server/internal/worker"
	"testing"
	"time"
)

func TestTargetedCommitWakesFollowTriggersAndIgnoreRollback(t *testing.T) {
	db, e := OpenHandle(filepath.Join(t.TempDir(), "wake.sqlite"), DefaultPolicy())
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(`CREATE TABLE wake_parent(id INTEGER PRIMARY KEY);CREATE TABLE wake_source(id INTEGER PRIMARY KEY,parent INTEGER REFERENCES wake_parent(id) ON DELETE CASCADE);CREATE TABLE wake_jobs(id INTEGER PRIMARY KEY);CREATE TRIGGER wake_enqueue AFTER INSERT ON wake_source BEGIN INSERT INTO wake_jobs VALUES(NEW.id);END;CREATE TABLE unrelated_jobs(id INTEGER PRIMARY KEY)`)
	if e != nil {
		t.Fatal(e)
	}
	if e = InstallWakeDependencies(context.Background(), db); e != nil {
		t.Fatal(e)
	}
	listen := func(table string) chan struct{} {
		t.Helper()
		sig := worker.NewSignal()
		unregister := WakeOnTables(sig, table)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		passes := make(chan struct{}, 8)
		go func() {
			defer close(done)
			worker.RunWith(ctx, "test.targeted", sig, nil, func(context.Context) time.Duration { passes <- struct{}{}; return 0 })
		}()
		<-passes
		t.Cleanup(func() { unregister(); cancel(); <-done })
		return passes
	}
	jobs, unrelated := listen("wake_jobs"), listen("unrelated_jobs")
	commit := func(query string) {
		t.Helper()
		if e = WithWriteTx(context.Background(), db, ClassInteractive, func(tx *sql.Tx) error {
			stmt, e := tx.Prepare(query)
			if e != nil {
				return e
			}
			defer stmt.Close()
			_, e = stmt.Exec()
			return e
		}); e != nil {
			t.Fatal(e)
		}
	}
	commit(`INSERT INTO wake_source VALUES(1,NULL)`)
	select {
	case <-jobs:
	case <-time.After(time.Second):
		t.Fatal("trigger-dependent job was not woken")
	}
	select {
	case <-unrelated:
		t.Fatal("unrelated worker woken")
	case <-time.After(20 * time.Millisecond):
	}
	held, e := Begin(context.Background(), db, ClassInteractive)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = held.Tx().Exec(`INSERT INTO wake_source VALUES(2,NULL)`); e != nil {
		t.Fatal(e)
	}
	if e = held.Rollback(); e != nil {
		t.Fatal(e)
	}
	select {
	case <-jobs:
		t.Fatal("rollback woke worker")
	case <-time.After(20 * time.Millisecond):
	}
	// A cascade dependency is included even when the direct mutation names only
	// the referenced parent. Subscriptions remain conservative for FK updates.
	source := listen("wake_source")
	commit(`INSERT INTO wake_parent VALUES(1)`)
	select {
	case <-source:
	case <-time.After(time.Second):
		t.Fatal("foreign key dependency omitted")
	}
}

func TestExecWriteWakesMountLikeWorkerButReadDoesNot(t *testing.T) {
	db := testDB(t)
	sig := worker.NewSignal()
	stopWake := WakeOnTables(sig, "rows")
	defer stopWake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	passes := make(chan time.Time, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx, "mount-like-reconcile", sig, func(context.Context) time.Duration {
			passes <- time.Now()
			return 0
		})
	}()
	<-passes // startup reconciliation
	readRows, err := db.Query(`SELECT value FROM rows`)
	if err != nil {
		t.Fatal(err)
	}
	readRows.Close()
	select {
	case <-passes:
		t.Fatal("read woke a targeted worker")
	case <-time.After(30 * time.Millisecond):
	}
	if _, err := ExecWrite(context.Background(), db, ClassInteractive, `UPDATE rows SET value='unchanged' WHERE id=-1`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-passes:
		t.Fatal("no-op write woke a targeted worker")
	case <-time.After(30 * time.Millisecond):
	}
	start := time.Now()
	if _, err := ExecWrite(context.Background(), db, ClassInteractive, `INSERT INTO rows(value) VALUES('mount desired state')`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-passes:
		if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
			t.Fatalf("mount-like worker reconciled after %s", elapsed)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("ExecWrite did not wake the mount-like worker")
	}
	cancel()
	<-done
}

// WakeOnTableWrites wakes only for a write that names one of its tables: not
// for a write whose scope couldn't be proven, nor for an unrelated table.
func TestWakeOnTableWritesIsExact(t *testing.T) {
	signal := worker.NewSignal()
	unregister := WakeOnTableWrites(signal, "inventory_runs")
	defer unregister()
	for _, statement := range []string{"WITH x AS (SELECT 1) UPDATE jobs SET status='x'", "UPDATE jobs SET processed=1", "DELETE FROM library_sources WHERE id='x'"} {
		changes := &changeSet{}
		changes.note(statement)
		notifyCommit(changes)
		if signal.Take() {
			t.Fatalf("%q woke an exact subscriber", statement)
		}
	}
	changes := &changeSet{}
	changes.note("WITH x AS (SELECT 1) SELECT 1")
	changes.note("UPDATE inventory_runs SET discovered=discovered+1 WHERE job_id=?")
	notifyDirectWrite(changes)
	if !signal.Take() {
		t.Fatal("a write naming the table didn't wake it")
	}
}
