// Command scalebench builds an isolated catalog and measures real catalog reads.
// Run from server: go run ./internal/catalog/scalebench -dir ../../.scratch/scale-1m -items 1000000
package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

type report struct {
	Items               int           `json:"items"`
	SeedResumedFrom     int           `json:"seedResumedFrom,omitempty"`
	SeedSeconds         float64       `json:"seedSeconds"`
	BytesPerItem        float64       `json:"bytesPerItem"`
	FirstBatchMSPerItem float64       `json:"firstBatchMsPerItem"`
	LastBatchMSPerItem  float64       `json:"lastBatchMsPerItem"`
	VisibilityBuildSec  float64       `json:"visibilityBuildSeconds"`
	Routes              []routeResult `json:"routes"`
	// This process starts catalog services only. Full-server idle must be measured
	// by the server integration harness; do not confuse an unopened worker with idle.
	IdleScope               string `json:"idleScope"`
	IdleStatementsPerMinute uint64 `json:"idleStatementsPerMinute"`
}
type routeResult struct {
	Route         string  `json:"route"`
	Samples       int     `json:"samples"`
	P50MS         float64 `json:"p50Ms"`
	P95MS         float64 `json:"p95Ms"`
	MaxStatements uint64  `json:"maxStatements"`
	Error         string  `json:"error,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() (runErr error) {
	dir := flag.String("dir", "", "new isolated directory under .scratch")
	n := flag.Int("items", 1000000, "playable item count (1000000 or 5000000 for acceptance)")
	samples := flag.Int("samples", 20, "requests per route")
	reuse := flag.Bool("reuse", false, "measure an existing fixture without reseeding")
	resumeSeed := flag.Bool("resume-seed", false, "continue a previously interrupted seed from its last committed item")
	idle := flag.Duration("idle", time.Minute, "catalog-only idle observation interval")
	seedOnly := flag.Bool("seed-only", false, "seed and analyze, leaving reads to the interleaved comparison harness")
	profile := flag.Bool("profile", false, "print each route's three costliest statement shapes")
	explain := flag.String("explain", "", "print the scans and sorts in the plans of the routes whose name contains this")
	flag.Parse()
	abs, err := filepath.Abs(*dir)
	if err != nil || *dir == "" || !strings.Contains(abs+string(os.PathSeparator), string(os.PathSeparator)+".scratch"+string(os.PathSeparator)) || *n < 100 || *samples < 1 || *idle <= 0 {
		return errors.New("invalid arguments: use an explicit .scratch directory, at least 100 items and positive samples/idle")
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return err
	}
	path := filepath.Join(abs, "catalog.sqlite")
	_, statErr := os.Stat(path)
	if *reuse && *resumeSeed {
		return errors.New("reuse and resume-seed are mutually exclusive")
	}
	if !*reuse && !*resumeSeed && !errors.Is(statErr, os.ErrNotExist) {
		return errors.New("fixture already exists; use -reuse to measure it")
	}
	if (*reuse || *resumeSeed) && statErr != nil {
		return statErr
	}
	db, err := persistence.Open(path)
	if err != nil {
		return err
	}
	defer func() { db.Close() }()
	r := report{Items: *n, IdleScope: "catalog service only; full-server workers not started"}
	if !*reuse {
		startItem := 0
		if *resumeSeed {
			if err = db.QueryRow(`SELECT count(*) FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind AND k.playable=1`).Scan(&startItem); err != nil {
				return err
			}
			if startItem < 0 || startItem > *n {
				return errors.New("resume fixture has no committed item batch or exceeds requested size")
			}
			fmt.Fprintf(os.Stderr, "resume seed %d/%d\n", startItem, *n)
			r.SeedResumedFrom = startItem
		}
		start := time.Now()
		r.FirstBatchMSPerItem, r.LastBatchMSPerItem, err = seed(db, *n, startItem, *n)
		if err != nil {
			return err
		}
		r.SeedSeconds = time.Since(start).Seconds()
		if _, err = db.Exec("ANALYZE; PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			return err
		}
	}
	var pages, size int64
	if err = db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
		return err
	}
	if err = db.QueryRow("PRAGMA page_size").Scan(&size); err != nil {
		return err
	}
	r.BytesPerItem = float64(pages*size) / float64(*n)
	if *seedOnly {
		raw, e := json.MarshalIndent(r, "", "  ")
		if e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(abs, "seed-report.json"), append(raw, '\n'), 0600)
	}
	s := catalog.New(db)
	if _, err = s.ClassifyPendingRatings(context.Background()); err != nil {
		return err
	}
	var rated int
	if err = db.QueryRow(`SELECT count(*) FROM catalog_item_attribute_edges x JOIN catalog_attribute_terms t ON t.id=x.term_id JOIN catalog_attribute_fields f ON f.id=t.field_id AND f.field='contentRating'`).Scan(&rated); err != nil {
		return err
	}
	if rated == 0 {
		return errors.New("scale fixture has no content ratings; reseed before measuring a restricted viewer")
	}
	movieLibrary, musicLibrary := "movies", "music"
	libraries := []string{"movies", "tv", "music"}
	pg13 := 13
	viewers := []struct {
		name string
		v    catalog.Viewer
	}{
		{"open", catalog.Viewer{Profile: "viewer", Fence: "scale-open", Libraries: libraries}},
		{"pg13", catalog.Viewer{Profile: "viewer", Fence: "scale-pg13", Libraries: libraries, Restrictions: identity.ContentRestrictions{MaximumAge: &pg13, BlockUnrated: true}}},
	}
	buildStart := time.Now()
	for _, library := range libraries {
		if err = s.RebuildVisibilityClass(context.Background(), library, viewers[1].v.EffectiveRestrictions()); err != nil {
			return fmt.Errorf("build %s visibility class: %w", library, err)
		}
	}
	r.VisibilityBuildSec = time.Since(buildStart).Seconds()
	cases := []struct {
		name string
		read func(*catalog.Service) error
	}{}
	for _, entry := range viewers {
		viewer := entry.v
		prefix := entry.name + "/"
		cases = append(cases,
			struct {
				name string
				read func(*catalog.Service) error
			}{prefix + "home", func(s *catalog.Service) error {
				page, e := s.HomeRows(catalog.HomeRequest{Viewer: viewer, Limit: 12})
				if e == nil && len(page.Rows) == 0 {
					return errors.New("empty home projection")
				}
				return e
			}},
			struct {
				name string
				read func(*catalog.Service) error
			}{prefix + "movies", func(s *catalog.Service) error {
				page, e := s.Content(catalog.ContentRequest{Viewer: viewer, Library: movieLibrary, View: "browse", Sort: "title", Limit: 40})
				if e == nil && (len(page.Sections) == 0 || len(page.Sections[0].Entries) == 0) {
					return errors.New("empty movie projection")
				}
				return e
			}},
			struct {
				name string
				read func(*catalog.Service) error
			}{prefix + "songs", func(s *catalog.Service) error {
				page, e := s.Content(catalog.ContentRequest{Viewer: viewer, Library: musicLibrary, View: "browse", Sort: "title", Limit: 40})
				if e == nil && (len(page.Sections) == 0 || len(page.Sections[0].Entries) == 0) {
					return errors.New("empty song projection")
				}
				return e
			}},
		)
		// Each Home row on its own, so a slow Home names its row.
		catalogue, e := s.HomeRowCatalogue(catalog.HomeRequest{Viewer: viewer, Limit: 12})
		if e != nil {
			return fmt.Errorf("home row catalogue: %w", e)
		}
		for _, row := range catalogue {
			id := row.ID
			cases = append(cases, struct {
				name string
				read func(*catalog.Service) error
			}{prefix + "home-row/" + id, func(s *catalog.Service) error {
				_, e := s.HomeSingleRow(catalog.HomeRequest{Viewer: viewer, Limit: 12}, id, catalog.HomeRowPage{Limit: 12})
				if errors.Is(e, catalog.ErrHomeRowUnknown) {
					return nil // listed for layout, not served to this viewer
				}
				return e
			}})
		}
		for _, mode := range []string{"ordered", "mix"} {
			mode := mode
			cases = append(cases, struct {
				name string
				read func(*catalog.Service) error
			}{prefix + "listening-" + mode, func(s *catalog.Service) error {
				selection, e := s.ListeningSelection(catalog.ContentRequest{Viewer: viewer, Library: musicLibrary, Limit: 40}, catalog.ListeningTarget{Kind: "library", ID: musicLibrary, LibraryID: musicLibrary}, mode, "fixture", false, "")
				if e == nil && len(selection.Entries) == 0 {
					return errors.New("empty listening projection")
				}
				return e
			}})
		}
	}
	for _, c := range cases {
		if *explain != "" && strings.Contains(c.name, *explain) {
			explainRoute(db, c.name, c.read, s)
		}
		rr := routeResult{Route: c.name}
		dbwork.ProfileStatements(*profile)
		var durations []float64
		for i := 0; i < *samples; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			before := dbwork.Reads().Statements
			start := time.Now()
			e := dbwork.WithReadSnapshot(ctx, db, func(ctx context.Context) error { return c.read(s.WithContext(ctx)) })
			elapsed := float64(time.Since(start)) / float64(time.Millisecond)
			cancel()
			rr.MaxStatements = max(rr.MaxStatements, dbwork.Reads().Statements-before)
			if e != nil {
				rr.Error = e.Error()
				break
			}
			durations = append(durations, elapsed)
		}
		sort.Float64s(durations)
		rr.Samples = len(durations)
		if len(durations) > 0 {
			rr.P50MS = durations[(len(durations)-1)/2]
			rr.P95MS = durations[int(math.Ceil(.95*float64(len(durations))))-1]
		}
		r.Routes = append(r.Routes, rr)
		if rr.Error != "" {
			return fmt.Errorf("%s: %s", rr.Route, rr.Error)
		}
		b, _ := json.Marshal(rr)
		fmt.Fprintln(os.Stderr, string(b))
		if *profile {
			for i, entry := range dbwork.StatementProfile() {
				if i == 3 {
					break
				}
				fmt.Fprintf(os.Stderr, "  %8.1f ms %5d× %s\n", entry.Millis(), entry.Count, entry.Shape)
			}
		}
	}
	before := dbwork.Reads().Statements
	time.Sleep(*idle)
	r.IdleStatementsPerMinute = uint64(float64(dbwork.Reads().Statements-before) * float64(time.Minute) / float64(*idle))
	raw, _ := json.MarshalIndent(r, "", "  ")
	fmt.Println(string(raw))
	return os.WriteFile(filepath.Join(abs, "report.json"), append(raw, '\n'), 0600)
}

func seed(db *sql.DB, n, startItem, stopItem int) (first, last float64, err error) {
	return seedReference(db, n, startItem, stopItem, 32)
}

// explainRoute runs one read of a route under a statement trace and prints
// the plan lines of each statement that scan a table or sort, with every
// parameter NULL (a plan, not a measurement).
func explainRoute(db *sql.DB, name string, read func(*catalog.Service) error, s *catalog.Service) {
	ctx, statements := dbwork.TraceStatements(context.Background())
	_ = dbwork.WithReadSnapshot(ctx, db, func(ctx context.Context) error { return read(s.WithContext(ctx)) })
	conn, err := db.Conn(context.Background())
	if err != nil {
		return
	}
	defer conn.Close()
	for _, text := range statements() {
		var lines []string
		_ = conn.Raw(func(raw any) error {
			prepared, err := raw.(driver.Conn).Prepare("EXPLAIN QUERY PLAN " + text)
			if err != nil {
				return err
			}
			defer prepared.Close()
			args := make([]driver.NamedValue, 64)
			for i := range args {
				args[i] = driver.NamedValue{Ordinal: i + 1}
			}
			rows, err := prepared.(driver.StmtQueryContext).QueryContext(context.Background(), args)
			if err != nil {
				return err
			}
			defer rows.Close()
			values := make([]driver.Value, len(rows.Columns()))
			for rows.Next(values) == nil {
				if detail, ok := values[len(values)-1].(string); ok && (strings.HasPrefix(detail, "SCAN ") || strings.Contains(detail, "TEMP B-TREE")) {
					lines = append(lines, detail)
				}
			}
			return nil
		})
		if len(lines) > 0 {
			fmt.Fprintf(os.Stderr, "EXPLAIN %s: %.300s\n  %s\n", name, strings.Join(strings.Fields(text), " "), strings.Join(lines, " | "))
		}
	}
}
