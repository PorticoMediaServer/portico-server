package directdns

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
)

func TestCoordinatedRebuildReplacesLowerEqualAndEmptyHistories(t *testing.T) {
	for _, size := range []int{0, 1, 5001} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			feedRows := make([]FeedRow, size)
			for i := range feedRows {
				ns, _ := NewNamespace()
				feedRows[i] = FeedRow{Sequence: int64(i + 1), Namespace: ns, IPv4: "8.8.8.8"}
			}
			hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture" {
					w.WriteHeader(401)
					return
				}
				after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
				items := []FeedRow{}
				for _, row := range feedRows {
					if row.Sequence > after {
						items = append(items, row)
						if len(items) == feedPage {
							break
						}
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
			}))
			defer hosted.Close()
			var priorDigest any
			for _, oldSequence := range []int64{int64(size), 10000} {
				path := filepath.Join(t.TempDir(), "addresses.json")
				table, _ := OpenTable(path)
				old, _ := NewNamespace()
				_ = table.Apply([]FeedRow{{Sequence: max(1, oldSequence), Namespace: old, IPv4: "9.9.9.9"}})
				feed := &Feed{URL: hosted.URL, Secret: "fixture", Table: table}
				if e := feed.Rebuild(context.Background()); e != nil {
					t.Fatal(e)
				}
				if _, found := table.Lookup(old); found || table.Len() != size || table.Sequence() != int64(size) {
					t.Fatal("stale rows/cursor", table.Summary())
				}
				reopened, e := OpenTable(path)
				if e != nil || reopened.Len() != size || reopened.Sequence() != int64(size) {
					t.Fatal("restart", e)
				}
				if priorDigest != nil && priorDigest != table.Summary()["digest"] {
					t.Fatal("nodes did not converge")
				}
				priorDigest = table.Summary()["digest"]
				if e = feed.Rebuild(context.Background()); e != nil || priorDigest != table.Summary()["digest"] {
					t.Fatal("retry", e)
				}
			}
		})
	}
}
func TestFailedRebuildPreservesOldSnapshotAndCannotReportReadiness(t *testing.T) {
	for _, body := range []string{`{"error":"denied"}`, `not json`, `{"items":[{"sequence":"0","namespace":"invalid"}]}`} {
		t.Run(body, func(t *testing.T) {
			table, _ := OpenTable(filepath.Join(t.TempDir(), "addresses.json"))
			ns, _ := NewNamespace()
			_ = table.Apply([]FeedRow{{Sequence: 9000, Namespace: ns, IPv4: "8.8.8.8"}})
			hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer hosted.Close()
			feed := &Feed{Table: table, URL: hosted.URL}
			if e := feed.Rebuild(context.Background()); e == nil {
				t.Fatal("invalid feed accepted")
			}
			restored, e := OpenTable(table.path)
			if e != nil || restored.Len() != 1 || restored.Sequence() != 9000 {
				t.Fatal("old snapshot lost", e)
			}
		})
	}
	status := &Readiness{}
	check := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		status.ServeHTTP(w, httptest.NewRequest("GET", "/ready", nil))
		if w.Code != want {
			t.Fatal(w.Code, want)
		}
	}
	check(503)
	status.Record(true, nil)
	check(503)
	status.Record(false, nil)
	check(200)
	status.Record(false, fmt.Errorf("unauthorized"))
	check(503)
}
