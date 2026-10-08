package compactcatalog

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"portico.local/server/internal/persistence"
)

// Writing and deriving must cost the same per entity in a library of any size.
// Every SQL statement in this package's production code runs per entity, per
// file or per batch; a statement that scans a table makes the whole catalogue
// quadratic. This reads every string literal that is a statement, asks SQLite
// for its plan, and fails on any full scan other than a batch's own json_each
// list, a constant VALUES list or a scan the allow-list below explains.
var drivesFromBatch = regexp.MustCompile(`(?is)\bFROM\s+json_each\(\?\d*\)\s+(\w+)\s+(?:CROSS\s+)?JOIN\b`)

func TestPerItemDerivationStatementsSeek(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "catalog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Scans that are bounded by something other than an index, with why.
	allowed := map[string]string{
		// catalog_derivations has one row per registered domain.
		"FROM catalog_derivations st":                 "the scheduler reads a table of one row per derived domain",
		"FROM catalog_derivations WHERE rebuilding=1": "one row per derived domain",
		"FROM audio_metadata_policies p":              "one row per audio library",
		// The rating classifier's queue: spellings not yet classified, drained
		// in batches (LIMIT) and probed for emptiness (EXISTS stops at a row).
		"FROM content_rating_pending LIMIT 5000":                    "a bounded batch of a draining queue",
		"entity_id,total FROM catalog_browse_blocks":                "CheckBrowseBlocks: a whole-table diagnostic for tests",
		"FROM catalog_browse_rows ORDER BY library_id,kind":         "CheckBrowseBlocks: a whole-table diagnostic for tests",
		"EXCEPT SELECT library_id,field,value,rating_key,label_key": "CheckFacetCounts: a whole-table diagnostic for tests",
		"FROM catalog_rec_facets f JOIN catalog_entities e":         "CheckRecPostings: a whole-table diagnostic for tests",
		"FROM catalog_rec_postings EXCEPT SELECT":                   "CheckRecPostings: a whole-table diagnostic for tests",
		"EXCEPT SELECT entity_id,facet FROM catalog_rec_postings":   "CheckRecPostings: a whole-table diagnostic for tests",
		"FROM catalog_rec_df EXCEPT SELECT":                         "CheckRecPostings: a whole-table diagnostic for tests",
		"FROM catalog_rec_postings GROUP BY facet EXCEPT":           "CheckRecPostings: a whole-table diagnostic for tests",
		"FROM rec_profile_signals s JOIN catalog_rec_facets f":      "CheckRecTaste: a whole-table diagnostic for tests",
		"SELECT profile_id,facet,long,short FROM rec_profile_taste": "CheckRecTaste: a whole-table diagnostic for tests",
		"SELECT work,facet FROM (":                                  "a work's member facets: the batch's works (json_each), each work's members by index, grouped",
		"SELECT EXISTS(SELECT 1 FROM content_rating_pending)":       "stops at the first queued row",
		// Inserting a parent row plans a scan of its children, which SQLite runs
		// only while a deferred foreign-key violation is outstanding (FkIfZero);
		// both are also first-use fallbacks, not per-item writes.
		"INSERT INTO catalog_libraries(":     "parent insert; child scan gated by FkIfZero",
		"INSERT INTO catalog_credit_labels(": "parent insert; child scan gated by FkIfZero",
	}
	// A job queue's head read (ORDER BY its key LIMIT n, in index order, no
	// sort) stops after n rows: SQLite calls it a scan, but it is bounded.
	headRead := regexp.MustCompile(`(?is)ORDER BY [\w,\s]+ LIMIT (1|\?)\s*$`)
	statement := regexp.MustCompile(`(?is)^\s*(SELECT|INSERT|UPDATE|DELETE|WITH)\b`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			query, err := strconv.Unquote(lit.Value)
			if err != nil || !statement.MatchString(query) || strings.Contains(query, "%s") {
				return true
			}
			for marker := range allowed {
				if strings.Contains(query, marker) {
					return true
				}
			}
			rows, err := db.Query(`EXPLAIN QUERY PLAN `+query, placeholders(query)...)
			if err != nil {
				// A fragment concatenated with other literals at run time.
				if testing.Verbose() {
					t.Logf("%s: not checked (%v): %.80s", file, err, strings.TrimSpace(query))
				}
				return true
			}
			plan := ""
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				plan += detail + "\n"
			}
			rows.Close()
			checked++
			// A statement that joins from its batch (FROM json_each(?) j JOIN …)
			// must start from the batch. Every line can be an index search and
			// the statement still grow with the library when the planner starts
			// from a low-selectivity key (every term of a vocabulary) and walks
			// out to the batch; CROSS JOIN fixes the order.
			if m := drivesFromBatch.FindStringSubmatch(query); m != nil {
				lines := strings.Split(strings.TrimSpace(plan), "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "SCAN "+m[1]+" ") || line == "SCAN "+m[1] {
						break
					}
					if strings.HasPrefix(line, "SEARCH ") {
						t.Errorf("%s: statement joins from its batch but the plan starts elsewhere (use CROSS JOIN):\n%s\nplan:\n%s", file, strings.TrimSpace(query), plan)
						break
					}
				}
			}
			bounded := headRead.MatchString(query) && !strings.Contains(plan, "TEMP B-TREE")
			for _, line := range strings.Split(strings.TrimSpace(plan), "\n") {
				if !strings.HasPrefix(line, "SCAN ") {
					continue
				}
				scanned := strings.Fields(line)[1]
				// A CTE the statement defines is a bounded stream built by its
				// own (checked) lines.
				cte := regexp.MustCompile(`(?i)(WITH|,)\s+` + regexp.QuoteMeta(scanned) + `\s+AS\s*\(`).MatchString(query)
				switch {
				case cte:
				case scanned == "j", scanned == "json_each", strings.HasPrefix(scanned, "CONSTANT"), strings.HasPrefix(line, "SCAN d ") && strings.Contains(query, "(VALUES"):
				case strings.Contains(line, "VIRTUAL TABLE INDEX"), bounded:
				default:
					t.Errorf("%s: statement scans %s:\n%s\nplan:\n%s", file, scanned, strings.TrimSpace(query), plan)
				}
			}
			return true
		})
	}
	if checked < 50 {
		t.Fatalf("only %d statements were checked; the literal scan is broken", checked)
	}
}

// placeholders is one NULL per parameter of query (plans don't depend on the
// values): the count of bare ? marks, or the highest ?N.
func placeholders(query string) []any {
	bare, numbered := 0, 0
	for i := 0; i < len(query); i++ {
		if query[i] != '?' {
			continue
		}
		j := i + 1
		for j < len(query) && query[j] >= '0' && query[j] <= '9' {
			j++
		}
		if j == i+1 {
			bare++
			continue
		}
		if n, err := strconv.Atoi(query[i+1 : j]); err == nil && n > numbered {
			numbered = n
		}
	}
	return make([]any, max(bare, numbered))
}
