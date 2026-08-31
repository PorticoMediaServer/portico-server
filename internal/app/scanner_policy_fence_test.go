package app

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestUpdateLibraryScanProfileAtomicallyFencesQueuedAndRunningContentWork(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	library, err := server.getLibrary("lib_movies")
	if err != nil {
		t.Fatal(err)
	}
	running, err := server.createJobFor("media_analyze", "Analyze media.", "media", "movie_meridian")
	if err != nil || !server.claimJobForRun(running.ID) {
		t.Fatalf("create/claim running analysis: job=%#v err=%v", running, err)
	}
	runningCtx, done := server.registerRunningJob(running.ID)
	defer done()
	queued, err := server.createJobFor("library_scan", "Scan library.", "library", library.ID)
	if err != nil {
		t.Fatal(err)
	}
	deferred, err := server.createJobFor("media_analyze", "Deferred analysis.", "media", "movie_neon")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE jobs SET status='deferred', phase='deferred', deferred_until=? WHERE id=?`, time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), deferred.ID); err != nil {
		t.Fatal(err)
	}

	library.Settings = map[string]any{"analysisTier": analysisTierFileListOnly}
	if _, err := server.updateLibrary(library.ID, UpdateLibraryRequest{
		Name: library.Name, Type: library.Type, Paths: library.Paths, Settings: library.Settings,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runningCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("running media analysis context was not cancelled after the atomic policy fence")
	}
	runningAfter, err := server.getJob(running.ID)
	if err != nil {
		t.Fatal(err)
	}
	if runningAfter.Status != "running" || runningAfter.Phase != "cancelling" || runningAfter.CancellationRequestedAt == "" {
		t.Fatalf("running analysis was not durably fenced: %#v", runningAfter)
	}
	queuedAfter, err := server.getJob(queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queuedAfter.Status != "cancelled" || queuedAfter.WorkerAcknowledgedAt == "" {
		t.Fatalf("queued scan remained runnable: %#v", queuedAfter)
	}
	deferredAfter, err := server.getJob(deferred.ID)
	if err != nil {
		t.Fatal(err)
	}
	var deferredUntil string
	if err := server.db.QueryRow(`SELECT deferred_until FROM jobs WHERE id=?`, deferred.ID).Scan(&deferredUntil); err != nil {
		t.Fatal(err)
	}
	if deferredAfter.Status != "cancelled" || deferredUntil != "" {
		t.Fatalf("deferred analysis remained eligible: job=%#v deferredUntil=%q", deferredAfter, deferredUntil)
	}
}

func TestScanProfileUpgradeDoesNotCancelAllowedWorkAndQueuesInventoryDerivedStages(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	library, err := server.getLibrary("lib_movies")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json='{"analysisTier":"basic"}' WHERE id=?`, library.ID); err != nil {
		t.Fatal(err)
	}
	library, _ = server.getLibrary(library.ID)
	running, err := server.createJobFor("media_analyze", "Allowed probe.", "media", "movie_meridian")
	if err != nil || !server.claimJobForRun(running.ID) {
		t.Fatalf("create/claim running analysis: job=%#v err=%v", running, err)
	}
	runningCtx, done := server.registerRunningJob(running.ID)
	defer done()

	library.Settings = map[string]any{"analysisTier": analysisTierComplete}
	if _, err := server.updateLibrary(library.ID, UpdateLibraryRequest{Name: library.Name, Type: library.Type, Paths: library.Paths, Settings: library.Settings}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runningCtx.Done():
		t.Fatal("Basic-to-Complete upgrade cancelled already-allowed analysis")
	case <-time.After(50 * time.Millisecond):
	}
	after, err := server.getJob(running.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.CancellationRequestedAt != "" || after.Phase == "cancelling" {
		t.Fatalf("upgrade fenced allowed work: %#v", after)
	}
	var profileJobs int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type='library_scan' AND resource_id=? AND status='queued' AND json_extract(metadata_json,'$.trigger')='profile-change' AND json_extract(metadata_json,'$.profileAnalysis')='true'`, library.ID).Scan(&profileJobs); err != nil {
		t.Fatal(err)
	}
	if profileJobs != 1 {
		t.Fatalf("upgrade profile reconciliation jobs=%d, want 1", profileJobs)
	}
	var forceFull int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type='library_scan' AND resource_id=? AND json_extract(metadata_json,'$.trigger')='profile-change' AND json_extract(metadata_json,'$.mode')='force_full'`, library.ID).Scan(&forceFull); err != nil {
		t.Fatal(err)
	}
	if forceFull != 0 {
		t.Fatalf("upgrade repeated full inventory in %d jobs", forceFull)
	}
}

func TestCustomOptionDisableCancelsAndRequeuesRetainedAnalysisWhileEnableDoesNotCancel(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	library, err := server.getLibrary("lib_movies")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json='{"analysisTier":"custom","probeStreams":true,"sonicFingerprinting":true}' WHERE id=?`, library.ID); err != nil {
		t.Fatal(err)
	}
	library, _ = server.getLibrary(library.ID)
	full, err := server.createJobForWithMetadata("media_analyze", "Custom full analysis.", "media", "movie_meridian", mediaAnalysisMetadata(mediaAnalysisModeFull))
	if err != nil || !server.claimJobForRun(full.ID) {
		t.Fatalf("create/claim full analysis: job=%#v err=%v", full, err)
	}
	fullCtx, fullDone := server.registerRunningJob(full.ID)
	defer fullDone()
	library.Settings = map[string]any{"analysisTier": analysisTierCustom, "probeStreams": true, "sonicFingerprinting": false}
	if _, err := server.updateLibrary(library.ID, UpdateLibraryRequest{Name: library.Name, Type: library.Type, Paths: library.Paths, Settings: library.Settings}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fullCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("disabling a Custom analysis capability did not cancel the broader running job")
	}
	var replacement int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type='library_scan' AND resource_id=? AND status='queued' AND json_extract(metadata_json,'$.profileAnalysis')='true'`, library.ID).Scan(&replacement); err != nil {
		t.Fatal(err)
	}
	if replacement != 1 {
		t.Fatalf("retained probe work was not requeued: %d", replacement)
	}

	probe, err := server.createJobForWithMetadata("media_analyze", "Allowed custom probe.", "media", "movie_cosmos", mediaAnalysisMetadata(mediaAnalysisModeProbe))
	if err != nil || !server.claimJobForRun(probe.ID) {
		t.Fatalf("create/claim probe analysis: job=%#v err=%v", probe, err)
	}
	probeCtx, probeDone := server.registerRunningJob(probe.ID)
	defer probeDone()
	library.Settings["sonicFingerprinting"] = true
	if _, err := server.updateLibrary(library.ID, UpdateLibraryRequest{Name: library.Name, Type: library.Type, Paths: library.Paths, Settings: library.Settings}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-probeCtx.Done():
		t.Fatal("enabling a Custom capability cancelled allowed probe work")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestFetchDisableFencesOnlyScanOriginMetadataWork(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	library, err := server.getLibrary("lib_movies")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json='{"analysisTier":"custom","fetchDescriptiveMetadata":true}' WHERE id=?`, library.ID); err != nil {
		t.Fatal(err)
	}
	library, _ = server.getLibrary(library.ID)
	scanMetadata, err := server.createJobForWithMetadata("metadata_refresh_library", "Scan metadata.", "library", library.ID, map[string]string{"subtaskScope": "scan_discoveries"})
	if err != nil || !server.claimJobForRun(scanMetadata.ID) {
		t.Fatalf("create/claim scan metadata: job=%#v err=%v", scanMetadata, err)
	}
	scanCtx, scanDone := server.registerRunningJob(scanMetadata.ID)
	defer scanDone()
	manual, err := server.createJobForWithMetadata("metadata_refresh", "Manual metadata.", "media", "movie_meridian", map[string]string{"subtaskScope": "manual"})
	if err != nil {
		t.Fatal(err)
	}
	libraryScan, err := server.createJobFor("library_scan", "Read scan content.", "library", library.ID)
	if err != nil {
		t.Fatal(err)
	}

	library.Settings["fetchDescriptiveMetadata"] = false
	if _, err := server.updateLibrary(library.ID, UpdateLibraryRequest{Name: library.Name, Type: library.Type, Paths: library.Paths, Settings: library.Settings}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-scanCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("scan-origin metadata refresh was not cancelled")
	}
	scanAfter, _ := server.getJob(scanMetadata.ID)
	manualAfter, _ := server.getJob(manual.ID)
	libraryScanAfter, _ := server.getJob(libraryScan.ID)
	if scanAfter.Phase != "cancelling" || scanAfter.CancellationRequestedAt == "" {
		t.Fatalf("scan metadata job was not durably fenced: %#v", scanAfter)
	}
	if manualAfter.Status != "queued" || manualAfter.CancellationRequestedAt != "" {
		t.Fatalf("independent manual metadata work was cancelled: %#v", manualAfter)
	}
	if libraryScanAfter.Status != "cancelled" {
		t.Fatalf("source-reading scan was not fenced: %#v", libraryScanAfter)
	}
}

func TestGlobalDefaultDowngradeAtomicallyFencesDeferredWork(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json='{}' WHERE id='lib_movies'`); err != nil {
		t.Fatal(err)
	}
	deferred, err := server.createJobFor("media_analyze", "Deferred global analysis.", "media", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE jobs SET status='deferred', phase='deferred', deferred_until=? WHERE id=?`, time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), deferred.ID); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	status, body := patchSettingsGroups(t, client, serverURL, map[string]any{"library": map[string]any{"analysisTier": analysisTierFileListOnly}}, nil)
	if status != http.StatusOK {
		t.Fatalf("settings downgrade status=%d body=%s", status, body)
	}
	after, err := server.getJob(deferred.ID)
	if err != nil {
		t.Fatal(err)
	}
	var deferredUntil string
	if err := server.db.QueryRow(`SELECT deferred_until FROM jobs WHERE id=?`, deferred.ID).Scan(&deferredUntil); err != nil {
		t.Fatal(err)
	}
	if after.Status != "cancelled" || deferredUntil != "" {
		t.Fatalf("global downgrade left deferred work eligible: job=%#v deferredUntil=%q", after, deferredUntil)
	}
}

func TestUpdateLibraryScanProfileFenceFailureRollsBackPolicy(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	library, err := server.getLibrary("lib_movies")
	if err != nil {
		t.Fatal(err)
	}
	queued, err := server.createJobFor("media_analyze", "Analyze media.", "media", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		CREATE TRIGGER fail_scan_policy_fence
		BEFORE UPDATE ON jobs
		WHEN OLD.id = '` + queued.ID + `' AND NEW.status = 'cancelled'
		BEGIN
			SELECT RAISE(ABORT, 'injected scan policy fence failure');
		END`); err != nil {
		t.Fatal(err)
	}
	defer server.db.Exec(`DROP TRIGGER IF EXISTS fail_scan_policy_fence`)

	beforeTier := settingString(library.Settings, "analysisTier", "")
	library.Settings = map[string]any{"analysisTier": analysisTierFileListOnly}
	if _, err := server.updateLibrary(library.ID, UpdateLibraryRequest{
		Name: library.Name, Type: library.Type, Paths: library.Paths, Settings: library.Settings,
	}); err == nil {
		t.Fatal("profile update succeeded despite an injected fence failure")
	}
	after, err := server.getLibrary(library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := settingString(after.Settings, "analysisTier", ""); got != beforeTier {
		t.Fatalf("profile committed without its fence: before=%q after=%q", beforeTier, got)
	}
	job, err := server.getJob(queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "queued" {
		t.Fatalf("failed transaction partially fenced job: %#v", job)
	}
}
