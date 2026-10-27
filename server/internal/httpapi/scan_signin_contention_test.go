package httpapi

// This regression pins the sign-in path against a live library scan.
//
// Root cause: a scan holds the one SQLite writer only for its short batches;
// between batches it is outside the writer while it probes or reads sidecars.
// A sign-in must never wait for that scan's progress or completion. The test
// keeps the real ingestion service inside its analysis seam (the existing
// AnalyzeSTRM hook), so the scan is alive but holds no database writer, and
// then sends the real POST /v1/direct/sign-in over HTTP. A fix that serialises
// auth behind the scan, or that only releases the gate after the scan ends,
// makes this request time out or return after the release.
//
// The scan is released afterwards and must still finish, so the test also
// proves the held analysis does not leak a writer or leave the job wedged.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/identity"
)

func TestScanOutsideWriterDoesNotBlockDirectSignIn(t *testing.T) {
	f := newV1Fixture(t, 0)
	scan := f.deps.Ingestion
	scan.SetStorage(f.deps.Storage)

	// A STRM descriptor is admitted through the normal inventory pass and its
	// analysis runs through Service.AnalyzeSTRM, an existing seam that never
	// touches a database writer itself. That is exactly the window a scan spends
	// outside the gate between its inventory and publication batches.
	strm := filepath.Join(f.root, "Contended Feature (2024).strm")
	if err := os.WriteFile(strm, []byte("http://127.0.0.1:9/never.mp4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO source_strm_analysis(library_id,enabled,revision) VALUES(?,1,1)
		ON CONFLICT(library_id) DO UPDATE SET enabled=1,revision=revision+1`, f.library); err != nil {
		t.Fatal(err)
	}
	// The catalogue fixture's library source is created outside an installation
	// handshake; confirm its retained root like a real configured server so the
	// scan reports its analysis result instead of a root-confirmation warning.
	if _, err := f.db.Exec(`UPDATE library_sources SET identity_confirmed=1 WHERE library_id=?`, f.library); err != nil {
		t.Fatal(err)
	}

	var entered, releaseOnce sync.Once
	started := make(chan struct{})
	release := make(chan struct{})
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	scan.AnalyzeSTRM = func(ctx context.Context, library, path string) (assets.Facts, error) {
		if library != f.library {
			t.Errorf("analysis library = %q, want %q", library, f.library)
		}
		entered.Do(func() { close(started) })
		select {
		case <-release:
		case <-ctx.Done():
			return assets.Facts{}, ctx.Err()
		}
		return assets.Facts{
			Container: "strm", VideoCodec: "h264", AudioCodec: "aac",
			Width: 1920, Height: 1080, Duration: 600,
			AnalysisSourceEvidence: "fixture:" + path,
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		scan.Run(ctx)
	}()
	job, err := scan.Queue(f.library)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not reach its analysis window")
	}
	if state, err := scan.Get(job.ID); err != nil {
		t.Fatal(err)
	} else if state.Status == "complete" || state.Status == "complete_with_warnings" {
		t.Fatalf("scan finished before the contention window: %+v", state)
	}

	server := httptest.NewServer(f.handler)
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second

	// Real HTTP, real router, real identity writes: no stubbed sign-in and no
	// direct call into the login service.
	start := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		server.URL+"/v1/direct/sign-in",
		bytes.NewBufferString(`{"username":"owner","password":"long-test-password"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("sign-in waited on the held scan for %s: %v", elapsed, err)
	}
	raw := new(bytes.Buffer)
	if _, err = raw.ReadFrom(response.Body); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	var signed identity.DirectSignIn
	decodeErr := json.Unmarshal(raw.Bytes(), &signed)
	if response.StatusCode != http.StatusOK || decodeErr != nil || signed.Session == nil ||
		signed.Session.AccessToken == "" {
		t.Fatalf("sign-in while scan held: status=%d decode=%v body=%s",
			response.StatusCode, decodeErr, raw.String())
	}
	if elapsed > 2*time.Second {
		t.Fatalf("sign-in took %s while the scan held no writer", elapsed)
	}

	// The scan must still be inside the same held window after sign-in returns;
	// otherwise this measured an idle server rather than contention.
	if state, err := scan.Get(job.ID); err != nil {
		t.Fatal(err)
	} else if state.Status == "complete" || state.Status == "complete_with_warnings" {
		t.Fatalf("scan completed while sign-in ran; contention was not held: %+v", state)
	}

	// Authenticate reads the session that sign-in just committed. A handler that
	// returned 200 without its real auth writes would fail here.
	principal, err := f.id.Authenticate(signed.Session.AccessToken)
	if err != nil || principal.AccountID == "" {
		t.Fatalf("sign-in token was not persisted by the real auth path: principal=%+v err=%v",
			principal, err)
	}

	// Release the analysis window and require the same scan to finish normally.
	unblock()
	deadline := time.Now().Add(5 * time.Second)
	for {
		state, err := scan.Get(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Status == "complete" || state.Status == "complete_with_warnings" {
			if state.Warnings != 0 {
				var issues []string
				rows, _ := f.db.Query(`SELECT relative_path||': '||reason FROM inventory_scan_issues WHERE job_id=? ORDER BY relative_path`, job.ID)
				if rows != nil {
					for rows.Next() {
						var issue string
						if rows.Scan(&issue) == nil {
							issues = append(issues, issue)
						}
					}
					rows.Close()
				}
				t.Fatalf("released scan reported warnings: %+v issues=%v", state, issues)
			}
			break
		}
		if state.Status == "failed" || time.Now().After(deadline) {
			t.Fatalf("released scan did not finish: %+v", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-scanDone:
	case <-time.After(5 * time.Second):
		t.Fatal("scan loop did not stop")
	}
}
