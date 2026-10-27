package catalog

import (
	"context"
	"database/sql"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"strings"
)

// LibrarySummary is a viewer-safe bounded catalogue projection. It contains no
// paths, account details, scan percentages or current-file information.
type LibrarySummary struct {
	// PublicationCounts counts only explicitly classified, visible Reading
	// titles. It is separate from Counts so ebook totals are never doubled.
	PublicationCounts []LibraryKindCount `json:"publicationCounts,omitempty"`
	Counts            []LibraryKindCount `json:"counts"`
	CoverURLs         []string           `json:"coverUrls"`
	LatestArtwork     string             `json:"latestArtwork"`
	FirstScan         bool               `json:"firstScan"`
	EmptyKnown        bool               `json:"emptyKnown"`
	Empty             bool               `json:"empty"`
}
type LibraryKindCount struct {
	Kind    string `json:"kind"`
	Count   int    `json:"count"`
	AtLeast bool   `json:"atLeast"`
}
type LibraryAudience struct {
	Everyone bool `json:"everyone"`
	Accounts int  `json:"accounts"`
}

const librarySummaryCap = 20000

func summaryKinds(kind string) []string {
	switch kind {
	case "tv", "anime":
		return []string{"show", "episode"}
	case "music":
		return []string{"artist", "album", "song"}
	case "audiobook":
		return []string{"book", "author"}
	case "book":
		return []string{"ebook"}
	case "comic":
		return []string{"comic"}
	default:
		return []string{"movie"}
	}
}

// SummarizeLibraries uses one bounded catalogue read per allowed library, rather
// than issuing browse calls for each kind and artwork sample. Every title and
// cover is checked against the viewer's live content restrictions.
func (s *Service) SummarizeLibraries(ctx context.Context, viewer Viewer, libraries []Library) (map[string]LibrarySummary, error) {
	out := map[string]LibrarySummary{}
	err := dbwork.WithReadSnapshot(ctx, s.db, func(ctx context.Context) error {
		reader := s.WithContext(ctx)
		for _, library := range libraries {
			if !viewer.AllowsLibrary(library.ID) {
				continue
			}
			summary, err := reader.librarySummary(ctx, viewer, library)
			if err != nil {
				return err
			}
			out[library.ID] = summary
		}
		return nil
	})
	return out, err
}

// librarySummary retains the original current-snapshot response composition.
func (s *Service) librarySummary(ctx context.Context, viewer Viewer, library Library) (LibrarySummary, error) {
	core, err := s.reusableLibrarySummaryCore(ctx, viewer, library)
	if err != nil {
		return LibrarySummary{}, err
	}
	return s.decorateLibrarySummary(ctx, library, core)
}

func (s *Service) decorateLibrarySummary(ctx context.Context, library Library, core librarySummaryCore) (LibrarySummary, error) {
	out := LibrarySummary{Counts: append([]LibraryKindCount{}, core.Counts...), PublicationCounts: cloneSummaryCounts(core.PublicationCounts), CoverURLs: []string{}, Empty: core.Empty, EmptyKnown: core.EmptyKnown}
	seen := map[string]bool{}
	// Fallback flags are live bounded decoration too: details INSERT/DELETE do
	// not have a complete synchronous library-revision fence. Never cache raw
	// provider URLs or assume UPDATE-only invalidation covers those operations.
	fallback := map[artworkTarget]resolvedArtwork{}
	if library.Kind == "movie" && len(core.Targets) > 0 {
		ids := make([]any, len(core.Targets))
		for i, target := range core.Targets {
			ids[i] = target.ID
		}
		rows, e := s.read().QueryContext(ctx, `SELECT pid(e.public_id),d.poster_url<>'',d.backdrop_url<>'' FROM catalog_entities e JOIN catalog_item_details d ON d.entity_id=e.id WHERE e.public_id IN (`+strings.TrimSuffix(strings.Repeat("pid_blob(?),", len(ids)), ",")+`)`, ids...)
		if e != nil {
			return out, e
		}
		for rows.Next() {
			var id string
			var poster, backdrop bool
			if e = rows.Scan(&id, &poster, &backdrop); e != nil {
				rows.Close()
				return out, e
			}
			art := resolvedArtwork{}
			if poster {
				art.PosterURL = "/v1/items/" + id + "/art/poster"
			}
			if backdrop {
				art.BackdropURL = "/v1/items/" + id + "/art/backdrop"
			}
			fallback[artworkTarget{"item", id}] = art
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return out, e
		}
	}
	art, err := s.resolveArtworkContext(ctx, core.Targets)
	if err != nil {
		return out, err
	}
	for _, target := range core.Targets {
		a := art[target]
		if a.PosterURL == "" {
			a.PosterURL = fallback[target].PosterURL
		}
		if a.BackdropURL == "" {
			a.BackdropURL = fallback[target].BackdropURL
		}
		if len(out.CoverURLs) < 3 && a.PosterURL != "" && !seen[a.PosterURL] {
			out.CoverURLs = append(out.CoverURLs, a.PosterURL)
			seen[a.PosterURL] = true
		}
		if out.LatestArtwork == "" {
			out.LatestArtwork = a.BackdropURL
		}
	}
	err = s.read().QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM inventory_runs r JOIN jobs j ON j.id=r.job_id WHERE j.library_id=? AND j.status IN('complete','complete_with_warnings')) AND EXISTS(SELECT 1 FROM jobs WHERE library_id=? AND status IN('queued','running','paused'))`, library.ID, library.ID).Scan(&out.FirstScan)
	return out, err
}

func (s *Service) computeLibrarySummaryCore(ctx context.Context, viewer Viewer, library Library) (librarySummaryCore, error) {
	return s.computeLibrarySummaryCoreWithReading(ctx, viewer, library, nil)
}

func (s *Service) computeLibrarySummaryCoreWithReading(ctx context.Context, viewer Viewer, library Library, reading *readingBrowseScope) (librarySummaryCore, error) {
	out := librarySummaryCore{Counts: []LibraryKindCount{}, Empty: true}
	kinds := summaryKinds(library.Kind)
	clause, args := EntityRestrictionSQL("e.id", viewer.Restrictions)
	publicationTypeSQL := "NULL"
	if isReadingLibrary(library.Kind) {
		if reading == nil {
			var err error
			reading, err = s.readingBrowseScope(library.ID)
			if err != nil {
				return out, err
			}
		}
		// The bounded candidate window stays unchanged. Its visible counts and
		// artwork must additionally belong to the same ready Reading membership
		// and canonical publication/link/format choice as Browse. Availability
		// does not exclude an otherwise eligible Reading title.
		clause = "(" + clause + ") AND " + reading.eligibleSQL("e.id") +
			" AND EXISTS(SELECT 1 FROM reading_attribution_members m WHERE " + reading.memberScope() + " AND m.item_id=e.id)"
		if library.Kind == "book" {
			publicationTypeSQL = `(SELECT m.publication_type FROM reading_attribution_members m WHERE ` + reading.memberScope() + ` AND m.item_id=e.id)`
		}
	}
	selections := []string{}
	values := []any{}
	for _, kind := range kinds {
		code, err := compactcatalog.ParseKind(kind)
		if err != nil {
			return out, err
		}
		selections = append(selections, `SELECT * FROM (SELECT e.id,e.public_id,e.kind,e.added_ms FROM catalog_entities e INDEXED BY catalog_entities_summary_added WHERE e.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND e.kind=? AND e.retired=0 ORDER BY e.added_ms DESC,e.id DESC LIMIT 20001)`)
		values = append(values, library.ID, code)
	}
	values = append(values, args...)
	// Each kind contributes at most 20,001 candidates through its ordered partial
	// index. Sorting the union and applying live visibility is bounded even for
	// a cold restriction class and a catalogue with millions of titles.
	rows, err := s.read().QueryContext(ctx, `WITH candidates AS (`+strings.Join(selections, " UNION ALL ")+`)
 SELECT pid(e.public_id),k.name,(`+clause+`),`+publicationTypeSQL+` FROM candidates e JOIN catalog_kinds k ON k.id=e.kind ORDER BY e.added_ms DESC,e.id DESC`, values...)
	if err != nil {
		return out, err
	}
	counts := map[string]int{}
	publicationCounts := map[string]int{}
	visited := map[string]int{}
	coverKind := kinds[0]
	if library.Kind == "music" {
		coverKind = "album"
	}
	targets := []artworkTarget{}
	for rows.Next() {
		var id, kind string
		var visible bool
		var publicationType sql.NullString
		if err = rows.Scan(&id, &kind, &visible, &publicationType); err != nil {
			rows.Close()
			return out, err
		}
		visited[kind]++
		if visited[kind] > librarySummaryCap {
			continue
		}
		if !visible {
			continue
		}
		counts[kind]++
		if publicationType.Valid {
			switch publicationType.String {
			case "book", "magazine", "newspaper":
				publicationCounts[publicationType.String]++
			default:
				rows.Close()
				return out, ErrReadingProjectionChanged
			}
		}
		for _, countKind := range kinds {
			if kind == countKind {
				out.Empty = false
			}
		}
		if kind == coverKind {
			if len(targets) < 64 {
				targetKind := kind
				if kind == "movie" || kind == "ebook" || kind == "comic" {
					targetKind = "item"
				}
				target := artworkTarget{targetKind, id}
				targets = append(targets, target)
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.EmptyKnown = true
	for _, n := range visited {
		if n > librarySummaryCap {
			out.EmptyKnown = false
		}
	}
	for _, kind := range kinds {
		out.Counts = append(out.Counts, LibraryKindCount{Kind: kind, Count: counts[kind], AtLeast: visited[kind] > librarySummaryCap})
	}
	if library.Kind == "book" {
		for _, kind := range []string{"book", "magazine", "newspaper"} {
			out.PublicationCounts = append(out.PublicationCounts, LibraryKindCount{Kind: kind, Count: publicationCounts[kind], AtLeast: visited["ebook"] > librarySummaryCap})
		}
	}
	out.Targets = targets
	return out, nil
}

func libraryAudience(ctx context.Context, tx *sql.Tx, library string) (LibraryAudience, error) {
	var out LibraryAudience
	err := tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM direct_memberships m WHERE m.disabled=0 AND m.role<>'owner' AND json_array_length(m.allowed_libraries)>0 AND NOT EXISTS(SELECT 1 FROM json_each(m.allowed_libraries) WHERE value=?)),COUNT(*) FROM direct_memberships m WHERE m.disabled=0 AND (m.role='owner' OR json_array_length(m.allowed_libraries)=0 OR EXISTS(SELECT 1 FROM json_each(m.allowed_libraries) WHERE value=?))`, library, library).Scan(&out.Everyone, &out.Accounts)
	return out, err
}
