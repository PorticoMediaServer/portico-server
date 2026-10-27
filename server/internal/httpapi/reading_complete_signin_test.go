package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/readingtest"
)

// Exercise real multi-page Reading inventory transactions, the supervised
// path worker and real HTTP identity writes. The storage guard holds the next
// outside-writer step after 129 objects, so success cannot mean an idle scan.
func TestReadingCompleteMultiPageScanAllowsRealHTTPSignIn(t *testing.T) {
	f := newV1Fixture(t, 0)
	root := t.TempDir()
	folder := filepath.Join(root, "Writer", "Series")
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 130; i++ {
		if err := os.WriteFile(filepath.Join(folder, fmt.Sprintf("Series %03d.pdf", i)), readingtest.PDF(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	f.cat.SetStorage(f.deps.Storage)
	library, err := f.cat.Create("Reading complete contention", "book", root)
	if err != nil {
		t.Fatal(err)
	}
	scan := f.deps.Ingestion
	scan.SetStorage(f.deps.Storage)
	var once, releaseOnce sync.Once
	entered := make(chan struct{})
	release := make(chan struct{})
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	f.deps.Storage.Guard = func(path string) error {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			var n int
			if err := f.db.QueryRow(`SELECT count(*) FROM inventory_objects o JOIN library_sources s ON s.id=o.source_id WHERE s.library_id=?`, library.ID).Scan(&n); err != nil {
				return err
			}
			if n >= 129 {
				once.Do(func() { close(entered); <-release })
			}
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanDone, workerDone := make(chan struct{}), make(chan struct{})
	go func() { defer close(scanDone); scan.Run(ctx) }()
	go func() { defer close(workerDone); compactcatalog.NewWorker(f.db).Run(ctx) }()
	t.Cleanup(func() {
		unblock()
		cancel()
		for _, done := range []chan struct{}{scanDone, workerDone} {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("Reading test worker did not stop")
			}
		}
	})
	job, err := scan.Queue(library.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		t.Fatal("scan did not publish multiple actual Reading pages")
	}
	var series int
	if err = f.db.QueryRow(`SELECT count(*) FROM reading_path_head WHERE series IS NOT NULL`).Scan(&series); err != nil || series != 0 {
		t.Fatalf("open scan leaked folder series: %d %v", series, err)
	}
	server := httptest.NewServer(f.handler)
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	response, err := client.Post(server.URL+"/v1/direct/sign-in", "application/json", bytes.NewBufferString(`{"username":"owner","password":"long-test-password"}`))
	if err != nil {
		t.Fatalf("real sign-in blocked behind Reading scan: %v", err)
	}
	var signed identity.DirectSignIn
	err = json.NewDecoder(response.Body).Decode(&signed)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || signed.Session == nil {
		t.Fatalf("sign-in status=%d decode=%v", response.StatusCode, err)
	}
	if _, err = f.id.Authenticate(signed.Session.AccessToken); err != nil {
		t.Fatal("sign-in did not persist its session", err)
	}
	state, err := scan.Get(job.ID)
	if err != nil || state.Status != "running" {
		t.Fatalf("scan was not live during sign-in: %+v %v", state, err)
	}
	unblock()
	deadline := time.Now().Add(15 * time.Second)
	for {
		state, err = scan.Get(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Status == "failed" {
			t.Fatalf("Reading scan failed: %+v", state)
		}
		if state.Status == "complete" || state.Status == "complete_with_warnings" {
			if err = f.db.QueryRow(`SELECT count(*) FROM reading_path_head WHERE series='Series' AND author='Writer'`).Scan(&series); err != nil {
				t.Fatal(err)
			}
			if series == 130 {
				break
			}
		}
		if time.Now().After(deadline) {
			for _, q := range []string{`SELECT 'scope',directory_key,pending,desired,completed,queued,sealed,reason,served FROM reading_path_scope`, `SELECT 'work',kind,count(*),min(served),max(served) FROM reading_path_dirty_work GROUP BY kind`, `SELECT 'pending',* FROM reading_path_availability_pending LIMIT 5`, `SELECT 'domains',* FROM catalog_derivations WHERE domain IN(20,28)`, `SELECT 'fail',* FROM catalog_derivation_failures LIMIT 5`, `SELECT 'dirty',domain,count(*) FROM catalog_dirty GROUP BY domain`, `SELECT 'build',* FROM reading_path_build`} {
				rows, e := f.db.Query(q)
				if e != nil {
					t.Log(e)
					continue
				}
				cols, _ := rows.Columns()
				for rows.Next() {
					values := make([]any, len(cols))
					args := make([]any, len(cols))
					for i := range values {
						args[i] = &values[i]
					}
					_ = rows.Scan(args...)
					t.Log(values)
				}
				rows.Close()
			}
			t.Fatalf("scan/builder did not complete: state=%+v series=%d", state, series)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
