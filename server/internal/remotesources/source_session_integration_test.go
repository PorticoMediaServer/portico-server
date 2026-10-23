//go:build rcloneintegration

package remotesources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/rcloneprogram"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

// This fixture exercises the production controller/config/metadata/transport
// path with upstream rclone. Its authority callback checks a real owner family;
// it does not stand in for the Reading owner's book/content-access resolver.
func TestSourceControllerRealSharedProcessAndScanPublication(t *testing.T) {
	program := lockedRcloneFixture(t)
	t.Setenv("PORTICO_RCLONE", program)
	for _, sequential := range []bool{false, true} {
		t.Run(fmt.Sprint("sequential=", sequential), func(t *testing.T) {
			f := newObservationFixture(t)
			state := t.TempDir()
			managed, err := mounts.New(f.s.db, state, "", program, storage.New(program))
			if err != nil {
				t.Fatal(err)
			}
			f.s.mounts = managed
			f.s.private = filepath.Join(state, "serve")
			if err = os.Mkdir(f.s.private, 0700); err != nil {
				t.Fatal(err)
			}
			identities, owner, _ := observationOwner(t, f)
			// Fixture construction is not provider I/O; keep the controller
			// operation budget separate from cold race-instrumented SQLite setup.
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			payload := bytes.Repeat([]byte("source-range-"), 6000)
			modified := time.Unix(1700000000, 0).UTC()
			var active, peak, gets atomic.Int32
			var badAuth atomic.Bool
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := active.Add(1)
				defer active.Add(-1)
				for prior := peak.Load(); n > prior && !peak.CompareAndSwap(prior, n); prior = peak.Load() {
				}
				u, p, ok := r.BasicAuth()
				if !ok || u != "fixture" || p != "private-fixture" {
					badAuth.Store(true)
					w.WriteHeader(401)
					return
				}
				if r.URL.Path != "/media/" && r.URL.Path != "/media/film.mkv" {
					http.NotFound(w, r)
					return
				}
				if r.Method == "PROPFIND" {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(207)
					fmt.Fprint(w, `<d:multistatus xmlns:d="DAV:">`)
					if r.URL.Path == "/media/" {
						fmt.Fprint(w, `<d:response><d:href>/media/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
					}
					if r.URL.Path != "/media/" || r.Header.Get("Depth") == "1" {
						fmt.Fprintf(w, `<d:response><d:href>/media/film.mkv</d:href><d:propstat><d:prop><d:resourcetype/><d:getcontentlength>%d</d:getcontentlength><d:getlastmodified>%s</d:getlastmodified><d:getetag>"origin-one"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, len(payload), modified.Format(http.TimeFormat))
					}
					fmt.Fprint(w, `</d:multistatus>`)
					return
				}
				if r.Method == http.MethodOptions {
					w.Header().Set("DAV", "1")
					return
				}
				if r.Method == http.MethodGet {
					gets.Add(1)
				}
				w.Header().Set("ETag", `"origin-one"`)
				if sequential {
					// A genuine range-unsupported origin ignores Range. No Portico
					// ReaderAt or prefix spool is introduced to disguise this200.
					w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
					w.Header().Set("Last-Modified", modified.Format(http.TimeFormat))
					if r.Method != http.MethodHead {
						_, _ = w.Write(payload)
					}
					return
				}
				http.ServeContent(w, r, "film.mkv", modified, bytes.NewReader(payload))
			}))
			defer origin.Close()
			config, err := remotemedia.ValidateDAVConfig(ctx, remotemedia.DAVConfig{Root: origin.URL + "/media/", Username: "fixture", Password: "private-fixture", InsecureLocal: true}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err = storeControllerDAV(f, managed, config, sequential); err != nil {
				t.Fatal(err)
			}
			profile, _ := SourceReadPreset("list")
			profile.Limits = ReadLimits{Connections: 1, RequestsPerSecond: 50}
			writer, err := dbwork.Begin(ctx, f.s.db, dbwork.ClassBackgroundMedia)
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.s.ApplySourceReadProfileTx(ctx, writer.Tx(), identities, owner, SourceReadProfileChange{SourceID: "remote", Generation: "1", Profile: profile})
			if err == nil {
				err = writer.Commit()
			} else {
				writer.Rollback()
			}
			if err != nil {
				t.Fatal(err)
			}
			// This running fixture job starts under the newly saved policy.
			if _, err = f.s.db.Exec(`UPDATE inventory_runs SET policy_revision=(SELECT revision FROM library_scan_policies WHERE library_id=?)`, f.library); err != nil {
				t.Fatal(err)
			}
			if err = f.s.startReadRuntime(ctx); err != nil {
				t.Fatal(err)
			}
			defer f.s.stopServeProcesses()
			observed, err := f.s.ObserveScanFile(ctx, f.job, f.file)
			if err != nil {
				t.Fatal("real factory/metadata", err)
			}
			process := observed.process
			defer func() {
				process.Stop()
				wait, stop := context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				_ = process.Wait(wait)
			}()
			writer, err = dbwork.Begin(ctx, f.s.db, dbwork.ClassBackgroundMedia)
			if err != nil {
				t.Fatal(err)
			}
			ref, err := observed.PublishTx(ctx, writer.Tx())
			if err == nil {
				err = writer.Commit()
			} else {
				writer.Rollback()
			}
			if err != nil {
				t.Fatal("scan publication", err)
			}
			if observed.Snapshot().Size != int64(len(payload)) || ref.Revision != 1 {
				t.Fatal(observed.Snapshot(), ref)
			}
			b, _, _ := f.s.find(f.file)
			listingID := "streamed-directory"
			if _, err = f.s.db.ExecContext(ctx, `INSERT INTO remote_listing_sessions(id,job_id,source_id,generation,directory,created_at) VALUES(?,?,'remote','1',?,0)`, listingID, f.job, b.Root); err != nil {
				t.Fatal(err)
			}
			if err = f.s.buildServeSnapshot(ctx, process, b, "", listingID, "1"); err != nil {
				t.Fatal("actual inventory streaming snapshot", err)
			}
			var ready bool
			var children int
			if err = f.s.db.QueryRowContext(ctx, `SELECT ready,(SELECT COUNT(*) FROM remote_listing_entries WHERE listing_id=?) FROM remote_listing_sessions WHERE id=?`, listingID, listingID).Scan(&ready, &children); err != nil || !ready || children != 1 {
				t.Fatal("incomplete streaming snapshot published", ready, children, err)
			}
			first := captureOwnerSource(t, f, ref, identities, owner, SourceReading, func() {})
			defer first.Close()
			second := captureOwnerSource(t, f, ref, identities, owner, SourceReading, func() {})
			defer second.Close()
			if !f.s.PlaybackActive("remote") {
				t.Fatal("interactive source leases disappeared from removal admission")
			}
			if first.Binding().Sequential != sequential || second.Binding().Profile.Limits.Connections != 1 {
				t.Fatal(first.Binding(), second.Binding())
			}
			if sequential {
				if _, err = first.OpenRange(ctx); !errors.Is(err, storage.ErrRemoteRange) {
					t.Fatal("sequential became ReadAt", err)
				}
				stream, err := first.OpenSequential(ctx)
				if err != nil {
					t.Fatal(err)
				}
				got, err := io.ReadAll(stream)
				stream.Close()
				if err != nil || !bytes.Equal(got, payload) {
					t.Fatal("progressive bytes", len(got), err)
				}
				if gets.Load() != 1 {
					t.Fatal("from-start body replayed", gets.Load())
				}
			} else {
				rangeReader, err := first.OpenRange(ctx)
				if err != nil {
					t.Fatal(err)
				}
				out := make([]byte, 256)
				if n, err := rangeReader.ReadAt(ctx, out, 123); err != nil || n != len(out) || !bytes.Equal(out, payload[123:123+len(out)]) {
					t.Fatal("range bytes", n, err)
				}
				rangeReader.Close()
			}
			if err = first.Close(); err != nil {
				t.Fatal(err)
			}
			if !f.s.PlaybackActive("remote") {
				t.Fatal("closing one reader hid another active session")
			}
			if err = second.Check(ctx); err != nil {
				t.Fatal("closing one logical reader killed shared process", err)
			}
			if next, err := second.process(ctx); err != nil || next != process {
				t.Fatal("shared source process changed", err)
			}
			if peak.Load() != 1 || badAuth.Load() {
				t.Fatal("upstream source cap/auth", peak.Load(), badAuth.Load())
			}
			writer, err = dbwork.Begin(ctx, f.s.db, dbwork.ClassBackgroundMedia)
			if err != nil {
				t.Fatal(err)
			}
			profile.Limits.RequestsPerSecond = 49
			updated, err := f.s.ApplySourceReadProfileTx(ctx, writer.Tx(), identities, owner, SourceReadProfileChange{SourceID: "remote", Generation: "1", ExpectedRevision: result.Profile.Revision, Profile: profile})
			if err == nil {
				err = writer.Commit()
			} else {
				writer.Rollback()
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = f.s.ReadProfileCommitted(ctx, "remote", updated.Profile.Revision); err != nil {
				t.Fatal(err)
			}
			if f.s.PlaybackActive("remote") {
				t.Fatal("revoked profile left a live interactive activity record")
			}
			if err = second.Check(ctx); !errors.Is(err, storage.ErrRemoteChanged) {
				t.Fatal("old profile lease survived", err)
			}
			if err = process.Wait(ctx); !errors.Is(err, rcloneprogram.ErrServeStopped) {
				t.Fatal("physical retirement", err)
			}
			f.s.serveMu.Lock()
			remaining := len(f.s.serveLeases)
			f.s.serveMu.Unlock()
			if remaining != 0 {
				t.Fatal("physical slot retained after Wait", remaining)
			}
			if _, err = second.OpenSequential(ctx); err == nil {
				t.Fatal("old reader reacquired a replacement process")
			}
			files, err := os.ReadDir(f.s.private)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range files {
				if strings.HasPrefix(file.Name(), "serve-config-") || strings.HasPrefix(file.Name(), "serve-cache-") {
					t.Fatal("owned transient survived Wait", file.Name())
				}
			}
		})
	}
}

func storeControllerDAV(f observationFixture, managed *mounts.Service, config remotemedia.DAVConfig, sequential bool) error {
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	stored, err := managed.Seal(raw)
	clear(raw)
	if err != nil {
		return err
	}
	defer clear(stored)
	_, err = f.s.db.Exec(`UPDATE remote_sources SET config=?,range_support=CASE WHEN ? THEN 'unsupported' ELSE 'random' END,error_code=CASE WHEN ? THEN 'range_unsupported' ELSE '' END`, stored, sequential, sequential)
	return err
}
