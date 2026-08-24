package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestW1R3ConcurrentDurableActiveKeyClaim(t *testing.T) {
	server := newScannerTestServer(t)
	// Independent Server values model separate processes: each has its own
	// admission mutex and worker identity while sharing the durable SQLite
	// handle protected by the database's unique partial index.
	peers := []*Server{server}
	for index := 0; index < 3; index++ {
		peers = append(peers, &Server{
			cfg:            server.cfg,
			db:             server.db,
			log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
			logSubscribers: map[chan LogEvent]bool{},
			scannerWatch:   map[string]string{},
			transcodes:     map[string]*transcodeSession{},
		})
	}
	const callers = 256

	jobs := make(chan Job, callers)
	errorsCh := make(chan error, callers)
	var group sync.WaitGroup
	for index := 0; index < callers; index++ {
		group.Add(1)
		peer := peers[index%len(peers)]
		go func(peer *Server) {
			defer group.Done()
			job, err := peer.createJobFor("library_scan", "Concurrent scan.", "library", "lib_movies")
			if err != nil {
				errorsCh <- err
				return
			}
			jobs <- job
		}(peer)
	}
	group.Wait()
	close(jobs)
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("concurrent durable enqueue failed: %v", err)
	}

	jobIDs := map[string]bool{}
	for job := range jobs {
		if job.ActiveKey != "library_scan|library|lib_movies" {
			t.Fatalf("durable enqueue returned unexpected active key: %#v", job)
		}
		jobIDs[job.ID] = true
	}
	if len(jobIDs) != 1 {
		t.Fatalf("concurrent singleton enqueue returned %d job IDs: %#v", len(jobIDs), jobIDs)
	}
	var activeCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE active_key = ? AND status IN ('queued', 'running')`, "library_scan|library|lib_movies").Scan(&activeCount); err != nil {
		t.Fatalf("count active singleton jobs: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active singleton rows = %d, want 1", activeCount)
	}

	other, err := server.createJobFor("library_scan", "Distinct scan.", "library", "lib_tv")
	if err != nil {
		t.Fatalf("enqueue distinct active key: %v", err)
	}
	if other.ID == "" || jobIDs[other.ID] {
		t.Fatalf("distinct active-key input was over-deduplicated: %#v", other)
	}
}

func TestW1R3ActiveKeyVariantsDoNotOverDeduplicate(t *testing.T) {
	server := newScannerTestServer(t)
	probe, err := server.createJobForWithMetadata("media_analyze", "Probe analysis.", "media", "movie_variants", map[string]string{
		"analysisMode": "probe",
	})
	if err != nil {
		t.Fatalf("create probe analysis: %v", err)
	}
	full, err := server.createJobForWithMetadata("media_analyze", "Full analysis.", "media", "movie_variants", map[string]string{
		"analysisMode": "full",
	})
	if err != nil {
		t.Fatalf("create full analysis: %v", err)
	}
	if probe.ID == full.ID || probe.ActiveKey == full.ActiveKey {
		t.Fatalf("analysis mode variants were over-deduplicated: probe=%#v full=%#v", probe, full)
	}
	probeAgain, err := server.createJobForWithMetadata("media_analyze", "Probe again.", "media", "movie_variants", map[string]string{
		"analysisMode": "probe",
	})
	if err != nil {
		t.Fatalf("create duplicate probe analysis: %v", err)
	}
	if probeAgain.ID != probe.ID {
		t.Fatalf("same analysis variant did not deduplicate: first=%#v again=%#v", probe, probeAgain)
	}
	newRevision, err := server.createJobForWithMetadata("media_analyze", "Probe changed source.", "media", "movie_variants", map[string]string{
		"analysisMode": "probe", "sourceRevision": "revision-b",
	})
	if err != nil {
		t.Fatalf("create changed-source analysis: %v", err)
	}
	if newRevision.ID == probe.ID || newRevision.ActiveKey == probe.ActiveKey {
		t.Fatalf("changed source revision was over-deduplicated: old=%#v new=%#v", probe, newRevision)
	}

	standard, err := server.createJobForWithMetadata("optimize_version", "720p optimized version.", "media", "movie_variants", map[string]string{"profile": "720p-medium"})
	if err != nil {
		t.Fatalf("create standard optimized version: %v", err)
	}
	high, err := server.createJobForWithMetadata("optimize_version", "1080p optimized version.", "media", "movie_variants", map[string]string{"profile": "1080p-high"})
	if err != nil {
		t.Fatalf("create high optimized version: %v", err)
	}
	if standard.ID == high.ID || standard.ActiveKey == high.ActiveKey {
		t.Fatalf("optimized profile variants were over-deduplicated: standard=%#v high=%#v", standard, high)
	}
}

func TestW1R3MetadataContinuationTransfersActiveClaimAtomically(t *testing.T) {
	server := newScannerTestServer(t)
	parent, err := server.createJobForWithMetadata("metadata_refresh_library", "Refresh library.", "library", "lib_movies", map[string]string{"limit": "1"})
	if err != nil || !server.claimJobForRun(parent.ID) {
		t.Fatalf("create/claim parent: job=%#v err=%v", parent, err)
	}
	child, err := server.queueLibraryMetadataRefreshContinuation(parent, Library{ID: "lib_movies", Name: "Movies"}, MediaItem{ID: "movie_meridian", AddedAt: "2026-08-05T00:00:00Z"})
	if err != nil {
		t.Fatalf("queue continuation: %v", err)
	}
	var parentKey, childKey, childParent, childMetadata string
	if err := server.db.QueryRow(`SELECT active_key FROM jobs WHERE id = ?`, parent.ID).Scan(&parentKey); err != nil {
		t.Fatalf("read parent active key: %v", err)
	}
	if err := server.db.QueryRow(`SELECT active_key, parent_operation_id, metadata_json FROM jobs WHERE id = ?`, child.ID).Scan(&childKey, &childParent, &childMetadata); err != nil {
		t.Fatalf("read child continuation: %v", err)
	}
	if parentKey != "" || childKey != "metadata_refresh_library|library|lib_movies" || childParent != parent.ID || !strings.Contains(childMetadata, `"cursorId":"movie_meridian"`) {
		t.Fatalf("continuation claim transfer is incomplete: parent=%q child=%q parentID=%q metadata=%s", parentKey, childKey, childParent, childMetadata)
	}
}

func TestW1R3ExpiredWorkerCannotOverwriteReplacementLease(t *testing.T) {
	first := newScannerTestServer(t)
	second := &Server{cfg: first.cfg, db: first.db, log: first.log, jobWorkerID: randomID("replacement-worker")}
	job, err := first.createJobFor("metadata_refresh", "Refresh.", "media", "movie_meridian")
	if err != nil || !first.claimJobForRun(job.ID) {
		t.Fatalf("create/claim first attempt: job=%#v err=%v", job, err)
	}
	if _, err := first.db.Exec(`UPDATE jobs SET status = 'queued', leased_by = '', lease_expires_at = '' WHERE id = ?`, job.ID); err != nil {
		t.Fatalf("expire first attempt: %v", err)
	}
	if !second.claimJobForRun(job.ID) {
		t.Fatal("replacement worker did not claim job")
	}
	if err := first.setJobMessage(job.ID, "complete", 100, "Late completion."); err != nil {
		t.Fatalf("late worker completion returned database error: %v", err)
	}
	var status, owner string
	if err := first.db.QueryRow(`SELECT status, leased_by FROM jobs WHERE id = ?`, job.ID).Scan(&status, &owner); err != nil {
		t.Fatalf("read replacement claim: %v", err)
	}
	if status != "running" || owner != second.jobLeaseOwner(job.ID) {
		t.Fatalf("late worker overwrote replacement: status=%q owner=%q", status, owner)
	}
}

func TestW1R3TerminalJobTransitionCommitsCompleteEnvelope(t *testing.T) {
	server := newScannerTestServer(t)
	job, err := server.createJobFor("metadata_refresh", "Refresh.", "media", "movie_terminal")
	if err != nil || !server.claimJobForRun(job.ID) {
		t.Fatalf("create/claim job: job=%#v err=%v", job, err)
	}
	if err := server.setJobMessage(job.ID, "complete", 100, "Complete."); err != nil {
		t.Fatalf("complete job: %v", err)
	}
	var status, phase, owner, lease, retention string
	var progressCurrent int
	if err := server.db.QueryRow(`SELECT status, phase, progress_current, leased_by, lease_expires_at, retention_until FROM jobs WHERE id = ?`, job.ID).Scan(&status, &phase, &progressCurrent, &owner, &lease, &retention); err != nil {
		t.Fatalf("read terminal envelope: %v", err)
	}
	if status != "complete" || phase != "complete" || progressCurrent != 100 || owner != "" || lease != "" || retention == "" {
		t.Fatalf("terminal envelope status=%q phase=%q progress=%d owner=%q lease=%q retention=%q", status, phase, progressCurrent, owner, lease, retention)
	}
}

func TestW1R3ParentCancellationOnlyTargetsDurableChildren(t *testing.T) {
	server := newScannerTestServer(t)
	parent, err := server.createJobFor("metadata_refresh_library", "Refresh library.", "library", "lib_movies")
	if err != nil || !server.claimJobForRun(parent.ID) {
		t.Fatalf("create/claim parent: job=%#v err=%v", parent, err)
	}
	child, err := server.queueLibraryMetadataRefreshContinuation(parent, Library{ID: "lib_movies", Name: "Movies"}, MediaItem{ID: "movie_meridian", AddedAt: "2026-08-05T00:00:00Z"})
	if err != nil {
		t.Fatalf("queue child: %v", err)
	}
	independent, err := server.createJobFor("metadata_refresh", "Independent refresh.", "media", "movie_meridian")
	if err != nil {
		t.Fatalf("queue independent refresh: %v", err)
	}
	if _, err := server.cancelJob(parent.ID); err != nil {
		t.Fatalf("cancel parent: %v", err)
	}
	cancelledChild, err := server.getJob(child.ID)
	if err != nil {
		t.Fatalf("load child: %v", err)
	}
	unrelated, err := server.getJob(independent.ID)
	if err != nil {
		t.Fatalf("load independent job: %v", err)
	}
	if cancelledChild.Status != "cancelled" || unrelated.Status != "queued" {
		t.Fatalf("parent cancellation scope child=%q independent=%q", cancelledChild.Status, unrelated.Status)
	}
}

func TestW1R3JobAdmissionWakeAndReservationBounds(t *testing.T) {
	server := newScannerTestServer(t)
	server.jobWake = make(chan struct{}, 1)
	server.setJobRuntimeActive(true)
	for index := 0; index < 100; index++ {
		server.signalJobWake()
	}
	if got := len(server.jobWake); got != 1 {
		t.Fatalf("coalesced job wake depth = %d, want 1", got)
	}
	server.BeginShutdown()
	if _, err := server.createJobFor("library_scan", "Rejected during shutdown.", "library", "lib_shutdown"); err == nil {
		t.Fatal("job enqueue crossed shutdown admission boundary")
	}
	server.runDueJobsOnce()
	var queued int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE resource_id = 'lib_shutdown'`).Scan(&queued); err != nil {
		t.Fatalf("check rejected shutdown enqueue: %v", err)
	}
	if queued != 0 {
		t.Fatalf("shutdown enqueue left %d durable rows", queued)
	}

	for index := 0; index < 1200; index++ {
		if !server.reserveScheduledJob("library_scan", "library", fmt.Sprintf("library-%d", index), "2026-08-05-12") {
			t.Fatalf("reservation %d was unexpectedly rejected", index)
		}
	}
	server.scheduledJobMu.Lock()
	reservationCount := len(server.scheduledJobReservationAt)
	jobReservationCount := len(server.scheduledJobs)
	server.scheduledJobMu.Unlock()
	if reservationCount > 1024 || jobReservationCount > 1024 {
		t.Fatalf("scheduled reservation maps exceeded bound: timestamps=%d jobs=%d", reservationCount, jobReservationCount)
	}
}

func TestW1R3ShutdownAllowsJoinedWorkerTerminalWrite(t *testing.T) {
	server := newScannerTestServer(t)
	job, err := server.createJobFor("library_scan", "Completes during drain.", "library", "lib-drain-complete")
	if err != nil || !server.claimJobForRun(job.ID) {
		t.Fatalf("create/claim drain job: job=%#v err=%v", job, err)
	}
	server.BeginShutdown()
	if err := server.setJobMessage(job.ID, "complete", 100, "Completed while shutdown joined the worker."); err != nil {
		t.Fatalf("terminal write during drain: %v", err)
	}
	completed, err := server.getJob(job.ID)
	if err != nil || completed.Status != "complete" {
		t.Fatalf("drain terminal state=%#v err=%v", completed, err)
	}
}

func TestW1R3CancellationAcknowledgementIsLeaseFenced(t *testing.T) {
	server := newScannerTestServer(t)
	job, err := server.createJobFor("library_scan", "Lease-fenced cancellation.", "library", "lib-cancel-fence")
	if err != nil || !server.claimJobForRun(job.ID) {
		t.Fatalf("create/claim cancellation job: job=%#v err=%v", job, err)
	}
	if _, err := server.db.Exec(`UPDATE jobs SET cancellation_requested_at = ?, leased_by = 'replacement-worker' WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), job.ID); err != nil {
		t.Fatalf("replace cancellation lease: %v", err)
	}
	server.finalizeRequestedJobCancellation(job.ID)
	retained, err := server.getJob(job.ID)
	if err != nil || retained.Status != "running" {
		t.Fatalf("stale worker acknowledged replacement lease: job=%#v err=%v", retained, err)
	}
}

func TestW1R3ShutdownCancelsAndJoinsClaimedJobBeforeDatabaseClose(t *testing.T) {
	server := newScannerTestServer(t)
	job, err := server.createJobFor("metadata_refresh", "Blocked metadata refresh.", "media", "movie_meridian")
	if err != nil {
		t.Fatalf("create blocked job: %v", err)
	}
	started := make(chan struct{})
	returned := make(chan struct{})
	server.jobExecutionHook = func(ctx context.Context, _ Job) {
		close(started)
		<-ctx.Done()
		close(returned)
	}
	server.runDueJobsOnce()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("claimed job did not enter supervised worker")
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- server.Shutdown(shutdownContext) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel blocked job worker")
	}
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown returned before supervised job joined")
	}

	retained, err := server.getJob(job.ID)
	if err != nil {
		t.Fatalf("load interrupted job: %v", err)
	}
	if retained.Status != "interrupted" || retained.ErrorCode != "shutdown_interrupted" || !retained.RetryEligible || retained.InterruptedAt == "" {
		t.Fatalf("shutdown did not retain a retryable interruption envelope: %#v", retained)
	}
	server.testAfterDatabaseClose = func() {
		select {
		case <-returned:
		default:
			t.Error("database close boundary crossed before job worker returned")
		}
	}
	if err := server.db.Close(); err != nil {
		t.Fatalf("close database after joined shutdown: %v", err)
	}
	server.testAfterDatabaseClose()
}

func TestW1R3RestoreQuiescenceRequeuesJoinedJobBeforeDetach(t *testing.T) {
	server := newScannerTestServer(t)
	job, err := server.createJobFor("metadata_refresh", "Restore-blocked metadata refresh.", "media", "movie_meridian")
	if err != nil {
		t.Fatalf("create restore-blocked job: %v", err)
	}
	started := make(chan struct{})
	returned := make(chan struct{})
	server.jobExecutionHook = func(ctx context.Context, _ Job) {
		close(started)
		<-ctx.Done()
		close(returned)
	}
	server.runDueJobsOnce()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("restore test job did not start")
	}

	quiesceContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.quiesceForRestore(quiesceContext); err != nil {
		t.Fatalf("restore quiescence: %v", err)
	}
	select {
	case <-returned:
	default:
		t.Fatal("restore quiescence returned before job worker joined")
	}
	retained, err := server.getJob(job.ID)
	if err != nil {
		t.Fatalf("load restore-requeued job: %v", err)
	}
	var leasedBy string
	if err := server.db.QueryRow(`SELECT leased_by FROM jobs WHERE id = ?`, job.ID).Scan(&leasedBy); err != nil {
		t.Fatalf("load restore-requeued lease: %v", err)
	}
	if retained.Status != "queued" || retained.ErrorCode != "restore_interrupted" || leasedBy != "" {
		t.Fatalf("restore did not requeue joined job: job=%#v lease=%q", retained, leasedBy)
	}
	server.restoreBarrier.unblock()
}

func TestW1R3StartupRecoveryRespectsUnexpiredLease(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC()
	insert := func(id, lease string, leaseExpires time.Time) {
		t.Helper()
		stamp := now.Format(time.RFC3339Nano)
		if _, err := server.db.Exec(`
			INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, leased_by, lease_expires_at, created_at, updated_at)
			VALUES (?, 'metadata_refresh', 'running', 40, 'Running.', 'media', ?, ?, ?, ?, ?)`, id, id, lease, leaseExpires.Format(time.RFC3339Nano), stamp, stamp); err != nil {
			t.Fatalf("insert running lease %s: %v", id, err)
		}
	}
	insert("job-live-lease", "other-process-job-live", now.Add(time.Hour))
	insert("job-expired-lease", "other-process-job-expired", now.Add(-time.Minute))
	if err := server.requeueRunningJobsOnStartup(); err != nil {
		t.Fatalf("startup recovery: %v", err)
	}
	var liveStatus, liveOwner, expiredStatus, expiredOwner, expiredCode string
	if err := server.db.QueryRow(`SELECT status, leased_by FROM jobs WHERE id = 'job-live-lease'`).Scan(&liveStatus, &liveOwner); err != nil {
		t.Fatalf("load live lease: %v", err)
	}
	if err := server.db.QueryRow(`SELECT status, leased_by, error_code FROM jobs WHERE id = 'job-expired-lease'`).Scan(&expiredStatus, &expiredOwner, &expiredCode); err != nil {
		t.Fatalf("load expired lease: %v", err)
	}
	if liveStatus != "running" || liveOwner != "other-process-job-live" {
		t.Fatalf("startup recovery stole unexpired lease: status=%q owner=%q", liveStatus, liveOwner)
	}
	if expiredStatus != "queued" || expiredOwner != "" || expiredCode != "restart_interrupted" {
		t.Fatalf("startup recovery did not requeue expired interruption: status=%q owner=%q code=%q", expiredStatus, expiredOwner, expiredCode)
	}
}

func TestW1R3StartupRecoveryRequeuesOneRetryableClaimPerActiveKey(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, active_key, retry_eligible, error_code, created_at, updated_at)
		VALUES
			('retry-first', 'library_scan', 'interrupted', 40, 'Interrupted.', 'library', 'lib-retry', 'library_scan|library|lib-retry', 1, 'shutdown_interrupted', ?, ?),
			('retry-second', 'library_scan', 'interrupted', 50, 'Interrupted.', 'library', 'lib-retry', 'library_scan|library|lib-retry', 1, 'restart_interrupted', ?, ?),
			('retry-unkeyed', 'database_backup', 'interrupted', 20, 'Interrupted.', 'database', 'manual', '', 1, 'shutdown_interrupted', ?, ?)`,
		now, now, now, now, now, now); err != nil {
		t.Fatalf("insert interrupted jobs: %v", err)
	}
	if err := server.requeueRunningJobsOnStartup(); err != nil {
		t.Fatalf("startup recovery: %v", err)
	}
	var queued, interrupted int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE active_key = 'library_scan|library|lib-retry' AND status = 'queued'`).Scan(&queued); err != nil {
		t.Fatalf("count requeued claim: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE active_key = 'library_scan|library|lib-retry' AND status = 'interrupted'`).Scan(&interrupted); err != nil {
		t.Fatalf("count retained interruption: %v", err)
	}
	if queued != 1 || interrupted != 1 {
		t.Fatalf("active retry recovery queued=%d interrupted=%d, want 1/1", queued, interrupted)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE id = 'retry-unkeyed' AND status = 'queued'`).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("unkeyed retry recovery queued=%d err=%v", queued, err)
	}
}

func TestW1R3DiagnosticsUseIsolatedQueuesAndCanonicalRedaction(t *testing.T) {
	server := newScannerTestServer(t)
	server.serverDiagnosticQueue = make(chan serverDiagnosticRecord, 1)
	server.clientDiagnosticQueue = make(chan clientDiagnosticRecord, 1)
	server.clientDiagnosticWindows = map[string]clientDiagnosticWindow{}
	server.recordLog("warn", "Server secret=server-secret at "+server.cfg.AppDataDir+"?token=do-not-store", map[string]string{"authorization": "Bearer server-secret", "path": server.cfg.AppDataDir + "/private"})
	accepted := server.recordClientLogUpload(User{ID: "client-account"}, ClientLogUploadRequest{
		Device: "device-a", App: "client-a", Entries: []ClientLogEntry{{
			Level: "error", Message: "client flood token=client-secret", Context: map[string]string{"authorization": "Bearer client-secret"},
		}},
	})
	for index := 0; index < 250; index++ {
		server.recordClientLogUpload(User{ID: "client-account"}, ClientLogUploadRequest{
			Device: "device-a", App: "client-a", Entries: []ClientLogEntry{{Message: fmt.Sprintf("flood-%d", index)}},
		})
	}
	if accepted != 1 {
		t.Fatalf("initial client diagnostic was not accepted: %d", accepted)
	}
	if len(server.serverDiagnosticQueue) != 1 {
		t.Fatalf("client flood displaced server diagnostic evidence: queue depth=%d", len(server.serverDiagnosticQueue))
	}
	serverRecord := <-server.serverDiagnosticQueue
	if strings.Contains(serverRecord.event.Message, "server-secret") || strings.Contains(serverRecord.event.Message, server.cfg.AppDataDir) {
		t.Fatalf("server diagnostic redaction leaked secret/path: %#v", serverRecord.event)
	}
	if got := server.diagnosticClientDropped.Load(); got == 0 {
		t.Fatal("client flood was not bounded")
	}
	for index := 0; index < 250; index++ {
		server.recordClientLogUploadForOrigin(User{ID: "rotating-client"}, ClientLogUploadRequest{
			Device: fmt.Sprintf("rotating-device-%d", index), App: "client-a",
			Entries: []ClientLogEntry{{Message: "bounded rotating labels"}},
		}, fmt.Sprintf("https://origin-%d.example.test", index))
	}
	server.diagnosticMu.Lock()
	_, accountWindowExists := server.clientDiagnosticWindows["rotating-client"]
	server.diagnosticMu.Unlock()
	if !accountWindowExists {
		t.Fatal("client diagnostic limiter trusted attacker-controlled device/origin partition keys")
	}
	for index := 0; index < 1200; index++ {
		server.recordClientLogUpload(User{ID: fmt.Sprintf("client-account-%d", index)}, ClientLogUploadRequest{
			Device: fmt.Sprintf("device-%d", index), App: "client-a",
			Entries: []ClientLogEntry{{Message: "bounded principal window"}},
		})
	}
	server.diagnosticMu.Lock()
	windowCount := len(server.clientDiagnosticWindows)
	server.diagnosticMu.Unlock()
	if windowCount > 1024 {
		t.Fatalf("client diagnostic principal windows exceeded bound: %d", windowCount)
	}
	rawRecord := clientDiagnosticRecord{
		id: "client-columns", accountID: "client-account",
		device: "Bearer device-column-secret", app: server.cfg.AppDataDir + "/app-secret",
		origin: "https://user:password@example.test/?token=origin-secret",
		level:  "error", message: "safe", fields: map[string]string{},
		clientAt: "token=timestamp-secret", size: 256,
	}
	if err := server.persistClientDiagnostic(context.Background(), rawRecord); err != nil {
		t.Fatalf("persist adversarial diagnostic columns: %v", err)
	}
	var storedDevice, storedApp, storedOrigin, storedClientAt string
	if err := server.db.QueryRow(`SELECT device, app, origin, client_time FROM client_diagnostic_events WHERE id = 'client-columns'`).Scan(&storedDevice, &storedApp, &storedOrigin, &storedClientAt); err != nil {
		t.Fatalf("read sanitized diagnostic columns: %v", err)
	}
	storedColumns := strings.Join([]string{storedDevice, storedApp, storedOrigin, storedClientAt}, "|")
	for _, forbidden := range []string{"device-column-secret", "app-secret", "password", "origin-secret", "timestamp-secret", server.cfg.AppDataDir} {
		if strings.Contains(storedColumns, forbidden) {
			t.Fatalf("client diagnostic column leaked %q: %s", forbidden, storedColumns)
		}
	}

	server.recordAudit(httptest.NewRequest(http.MethodGet, "/api/audit-events", nil), User{ID: "audit-user", Email: "audit@example.test"}, "diagnostics.secret_test", "test", "resource", "warn", map[string]string{
		"url":   "https://example.test/?api_key=audit-secret",
		"path":  server.cfg.AppDataDir + "/audit-private",
		"token": "audit-secret",
	})
	var metadata string
	if err := server.db.QueryRow(`SELECT metadata_json FROM security_audit_events ORDER BY sequence DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatalf("load security audit evidence: %v", err)
	}
	if strings.Contains(metadata, "audit-secret") || strings.Contains(metadata, server.cfg.AppDataDir) {
		t.Fatalf("security audit redaction leaked secret/path: %s", metadata)
	}
	if err := server.verifySecurityAuditChain(context.Background()); err != nil {
		t.Fatalf("verify security audit hash chain: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE security_audit_events SET id = 'tampered-audit-id' WHERE sequence = (SELECT MAX(sequence) FROM security_audit_events)`); err != nil {
		t.Fatalf("tamper audit id: %v", err)
	}
	if err := server.verifySecurityAuditChain(context.Background()); err == nil {
		t.Fatal("security audit chain accepted a tampered event ID")
	}
}

func TestW1R3DebugLoggingExpiresAutomatically(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	server.retentionClock = func() time.Time { return now }
	if _, err := server.db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('troubleshooting', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		`{"logLevel":"debug","debugDurationMinutes":60,"clientLogUploads":true}`, now.Add(-2*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("configure expired debug logging: %v", err)
	}
	if got := server.troubleshootingLogLevel(); got != "info" {
		t.Fatalf("expired debug log level=%q want info", got)
	}
	if _, err := server.db.Exec(`UPDATE settings SET updated_at = ? WHERE key = 'troubleshooting'`, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("renew debug logging: %v", err)
	}
	if got := server.troubleshootingLogLevel(); got != "debug" {
		t.Fatalf("active debug log level=%q want debug", got)
	}
}

func TestW1R3AuditChainDetectsTailDeletionAndBackfillsLegacyProjectionOnce(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`INSERT INTO audit_events (id, action, resource_type, resource_id, severity, metadata_json, created_at)
		VALUES ('legacy-audit', 'legacy.action', 'test', 'resource', 'info', '{}', '2026-08-05T00:00:00Z')`); err != nil {
		t.Fatalf("insert legacy audit projection: %v", err)
	}
	events, err := server.listAuditEvents(10)
	if err != nil || len(events) != 1 || events[0].ID != "legacy-audit" {
		t.Fatalf("legacy audit backfill events=%#v err=%v", events, err)
	}
	server.recordAudit(nil, User{ID: "owner"}, "new.action", "test", "resource", "info", nil)
	if err := server.verifySecurityAuditChain(context.Background()); err != nil {
		t.Fatalf("verify complete chain: %v", err)
	}
	if _, err := server.db.Exec(`DELETE FROM security_audit_events WHERE sequence = (SELECT MAX(sequence) FROM security_audit_events)`); err != nil {
		t.Fatalf("delete audit tail: %v", err)
	}
	if err := server.verifySecurityAuditChain(context.Background()); err == nil {
		t.Fatal("security audit chain accepted tail truncation")
	}
	if _, err := server.listAuditEvents(10); err == nil {
		t.Fatal("audit API silently repaired or accepted tail truncation")
	}
}

func TestW1R3StructuredAuditMetadataIsPreservedAndRedactedInEvidence(t *testing.T) {
	server := newScannerTestServer(t)
	raw := `{"nested":{"label":"kept","token":"secret"},"items":["visible",{"password":"hidden"}]}`
	clean := server.sanitizeDiagnosticJSON(raw)
	if !strings.Contains(clean, `"label":"kept"`) || !strings.Contains(clean, `"items"`) {
		t.Fatalf("structured metadata was discarded: %s", clean)
	}
	if strings.Contains(clean, "secret") || strings.Contains(clean, "hidden") {
		t.Fatalf("structured metadata leaked secrets: %s", clean)
	}
}

func TestW1R3LogSubscriberDisconnectsOnGapAndShutdown(t *testing.T) {
	server := newScannerTestServer(t)
	subscriber := server.subscribeLogs()
	for index := 0; index < 40; index++ {
		server.recordLog("info", fmt.Sprintf("gap-%d", index), nil)
	}
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-subscriber:
			if !ok {
				server.logMu.Lock()
				remaining := len(server.logSubscribers)
				server.logMu.Unlock()
				if remaining != 0 {
					t.Fatalf("closed log stream remained registered: %d", remaining)
				}
				goto shutdown
			}
		case <-deadline:
			t.Fatal("full log subscriber did not receive a gap/disconnect signal")
		}
	}

shutdown:
	background, cancel := context.WithCancel(context.Background())
	server.backgroundCtx = background
	server.backgroundCancel = cancel
	defer cancel()
	writer := &r3StreamWriter{started: make(chan struct{})}
	request := httptest.NewRequest(http.MethodGet, "/api/logs/stream", nil)
	done := make(chan struct{})
	go func() {
		server.handleLogStream(writer, request, User{ID: "owner", Email: "owner@example.test", Role: "owner", AuthProvider: "local", Permissions: ownerPermissions()})
		close(done)
	}()
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("log stream did not start")
	}
	server.BeginShutdown()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("log stream ignored shutdown")
	}
}

func TestW1R3LogStreamRequiresResetForUnsupportedResume(t *testing.T) {
	server := newScannerTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/logs/stream", nil)
	request.Header.Set("Last-Event-ID", "log_previous_generation")
	writer := httptest.NewRecorder()
	server.handleLogStream(writer, request, User{ID: "owner", Email: "owner@example.test", Role: "owner", AuthProvider: "local", Permissions: ownerPermissions()})
	if !strings.Contains(writer.Body.String(), "event: stream-reset") || !strings.Contains(writer.Body.String(), "resume_not_supported") {
		t.Fatalf("unsupported stream resume did not receive an explicit reset: %q", writer.Body.String())
	}
	server.logMu.Lock()
	defer server.logMu.Unlock()
	if len(server.logSubscribers) != 0 {
		t.Fatalf("unsupported stream resume left %d log subscribers", len(server.logSubscribers))
	}
}

func TestW1R3LongPollShutdownHasDeliberateRetryResponse(t *testing.T) {
	server := newScannerTestServer(t)
	background, cancel := context.WithCancel(context.Background())
	server.backgroundCtx = background
	server.backgroundCancel = cancel
	server.BeginShutdown()
	_, err := server.waitLongPoll(context.Background(), make(chan struct{}), time.Second, func() bool { return true }, func() (bool, error) { return false, nil })
	if !errors.Is(err, errLongPollShutdown) {
		t.Fatalf("shutdown wait error=%v", err)
	}
	recorder := httptest.NewRecorder()
	if !writeLongPollShutdown(recorder, err) {
		t.Fatal("shutdown error was not handled")
	}
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Retry-After") != "1" || !strings.Contains(recorder.Body.String(), "server_shutting_down") {
		t.Fatalf("shutdown response code=%d headers=%v body=%q", recorder.Code, recorder.Header(), recorder.Body.String())
	}
}

type r3StreamWriter struct {
	header  http.Header
	mu      sync.Mutex
	body    bytes.Buffer
	started chan struct{}
	once    sync.Once
}

func (w *r3StreamWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *r3StreamWriter) WriteHeader(status int) {}

func (w *r3StreamWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if bytes.Contains(data, []byte(": connected")) {
		w.once.Do(func() { close(w.started) })
	}
	return w.body.Write(data)
}

func (w *r3StreamWriter) Flush() {}

var _ http.Flusher = (*r3StreamWriter)(nil)
var _ io.Writer = (*r3StreamWriter)(nil)
