package compactcatalog_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

func TestDerivedWorkerCompletesWithForegroundPressure(t *testing.T) {
	c := catalogtest.Open(t)
	movies := c.Library("m", "Movies", "movie", "/m")
	const count = 130 // Several real derived-data quanta, rather than one batch.
	for i := 0; i < count; i++ {
		c.Movie(movies, fmt.Sprintf("/m/%03d.mkv", i), fmt.Sprintf("Film %03d", i), 2000)
	}
	dbwork.RegisterForegroundProbe(t.Name(), func() bool { return true })
	defer dbwork.RegisterForegroundProbe(t.Name(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		compactcatalog.NewWorker(c.DB).Run(ctx)
	}()
	defer func() { cancel(); <-done }()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("foreground pressure prevented derived jobs completing")
		case <-tick.C:
			var dirty, derived int
			if err := c.DB.QueryRow(`SELECT count(*) FROM catalog_dirty`).Scan(&dirty); err != nil {
				t.Fatal(err)
			}
			if err := c.DB.QueryRow(`SELECT count(*) FROM catalog_browse_rows WHERE kind=?`, compactcatalog.Movie).Scan(&derived); err != nil {
				t.Fatal(err)
			}
			if dirty == 0 && derived == count {
				if err := compactcatalog.CheckBrowseBlocks(context.Background(), c.DB); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
}
