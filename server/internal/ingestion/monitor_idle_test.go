package ingestion

import (
	"context"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/testtier"
)

func TestMonitorConfigurationSleepsUntilCommittedConfigurationChanges(t *testing.T) {
	testtier.Media(t, "observes the former five-second monitor polling interval")
	c := catalogtest.Open(t)
	s := New(c.DB, catalog.New(c.DB), assets.Probe{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	dbwork.ProfileStatements(true)
	defer dbwork.ProfileStatements(false)
	go func() { defer close(done); s.runMonitor(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("monitor did not stop")
		}
	})
	reads := func() int64 {
		for _, entry := range dbwork.StatementProfile() {
			if strings.HasPrefix(entry.Shape, "SELECT s.id,s.library_id,s.root,s.root_identity,s.incarnation,s.classification") {
				return entry.Count
			}
		}
		return 0
	}
	waitFor := func(count int64) {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for reads() < count {
			if time.Now().After(deadline) {
				t.Fatalf("monitor configuration reads=%d, want at least %d", reads(), count)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitFor(1)
	time.Sleep(6 * time.Second)
	if got := reads(); got != 1 {
		t.Fatalf("unchanged empty configuration polled %d times", got)
	}
	if _, err := dbwork.ExecWrite(ctx, c.DB, dbwork.ClassInteractive, `INSERT INTO admin_documents(scope,revision,body,updated_ms) VALUES('library:unused',1,'{}',0)`); err != nil {
		t.Fatal(err)
	}
	waitFor(2)
}
