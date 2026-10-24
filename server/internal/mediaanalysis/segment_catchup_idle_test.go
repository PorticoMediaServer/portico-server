package mediaanalysis

import (
	"context"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/dbwork"
)

func TestSettledSegmentCatchupDoesNotPublishItsOwnWake(t *testing.T) {
	for _, library := range []bool{false, true} {
		name := "no-libraries"
		if library {
			name = "empty-enabled-library"
		}
		t.Run(name, func(t *testing.T) {
			c := catalogtest.Open(t)
			c.Exec(`INSERT INTO server_settings(singleton,"library.segmentDetection") VALUES(1,'standard') ON CONFLICT(singleton) DO UPDATE SET "library.segmentDetection"=excluded."library.segmentDetection"`)
			if library {
				c.Library("tv", "TV", "tv", "/tv")
			}
			c.Drain()
			s := &Service{db: c.DB}
			ctx := context.Background()
			if more, err := s.segmentCatchup(ctx); err != nil || more {
				t.Fatal(more, err)
			}
			before := dbwork.ChangingCommits()
			for range 3 {
				if more, err := s.segmentCatchup(ctx); err != nil || more {
					t.Fatal(more, err)
				}
			}
			if after := dbwork.ChangingCommits(); after != before {
				t.Fatal("settled catchup published changing commits", after-before)
			}
			if library {
				var previous string
				if err := c.DB.QueryRow(`SELECT modes FROM segment_catchup`).Scan(&previous); err != nil {
					t.Fatal(err)
				}
				c.Exec(`UPDATE server_settings SET "library.segmentDetection"='off' WHERE singleton=1`)
				c.Drain()
				before = dbwork.ChangingCommits()
				if more, err := s.segmentCatchup(ctx); err != nil || more {
					t.Fatal(more, err)
				}
				var current string
				if err := c.DB.QueryRow(`SELECT modes FROM segment_catchup`).Scan(&current); err != nil || current == previous || dbwork.ChangingCommits() == before {
					t.Fatal("actual mode change did not persist", err)
				}
				before = dbwork.ChangingCommits()
				if _, err := s.segmentCatchup(ctx); err != nil {
					t.Fatal(err)
				}
				if dbwork.ChangingCommits() != before {
					t.Fatal("disabled settled catchup wrote again")
				}
			}
		})
	}
}
