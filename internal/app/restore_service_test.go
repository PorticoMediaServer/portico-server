package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func restoreOwnerUser() User {
	return User{ID: "owner", AccountID: "owner", Role: "owner", AuthOrigin: "local", AuthProvider: "local", HasLocalPassword: true, Permissions: map[string]bool{"manageServer": true}}
}

func TestRestorePrincipalRequiresInteractiveOwnerAndExcludesAPIKeys(t *testing.T) {
	cases := []struct {
		name string
		user User
		want bool
	}{
		{name: "interactive owner", user: restoreOwnerUser(), want: true},
		{name: "ordinary user with manageServer", user: User{Role: "user", AuthProvider: "local", Permissions: map[string]bool{"manageServer": true}}, want: false},
		{name: "api key owner manageServer", user: User{Role: "owner", AuthProvider: "api_key", APIKeyID: "key-1", Permissions: map[string]bool{"manageServer": true}}, want: false},
		{name: "api key owner all scope", user: User{Role: "owner", AuthProvider: "api_key", APIKeyID: "key-2", APIKeyScopes: []string{"all"}, Permissions: map[string]bool{"manageServer": true}}, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{}
			recorder := httptest.NewRecorder()
			got := server.checkRestorePrincipal(recorder, test.user)
			if got != test.want {
				t.Fatalf("principal allowed=%v, want %v; body=%s", got, test.want, recorder.Body.String())
			}
			if !test.want && recorder.Code != http.StatusForbidden {
				t.Fatalf("rejected principal status=%d, want 403", recorder.Code)
			}
		})
	}
}

func TestRestoreReauthenticationRejectsHostedSessionPossessionLocalAbsenceAndProfilePIN(t *testing.T) {
	cases := []struct {
		name   string
		user   User
		secret string
		status int
		code   string
	}{
		{name: "hosted owner requires W2 proof", user: User{Role: "owner", AuthOrigin: "portico", AuthProvider: "portico", Permissions: map[string]bool{"manageServer": true}}, secret: "", status: http.StatusConflict, code: "restore_hosted_reauthentication_required"},
		{name: "local session possession without password", user: restoreOwnerUser(), secret: "", status: http.StatusUnauthorized, code: "restore_reauthentication_required"},
		{name: "primary profile PIN is not account password", user: restoreOwnerUser(), secret: "", status: http.StatusUnauthorized, code: "restore_reauthentication_required"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/backups/example.db/restore", nil)
			_, ok := server.verifyRestoreReauthentication(recorder, request, test.user, test.secret)
			if ok || recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.code) {
				t.Fatalf("reauth ok=%v status=%d body=%s, want status=%d code=%s", ok, recorder.Code, recorder.Body.String(), test.status, test.code)
			}
		})
	}
}

func TestRestoreConfirmationIsServerEnforcedBeforeStaging(t *testing.T) {
	serverURL, _, _ := newAuthTestServerWithInstance(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	// Login establishes the valid interactive owner session and supplies the
	// current account password; confirmation is intentionally omitted.
	loginUser(t, client, serverURL)
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/backups/portico-missing.db/restore", map[string]string{
		"password": "Password1234",
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "restore_confirmation_required") {
		t.Fatalf("missing confirmation status=%d body=%s", status, body)
	}
}

func TestRestoreUploadRejectsDatabaseBeforeBoundedAuthorizationAndLeavesNoStage(t *testing.T) {
	root := t.TempDir()
	server := &Server{cfg: config.Config{AppDataDir: root, DatabasePath: filepath.Join(root, "portico.db")}}
	if err := database.PreparePrivateDataPaths(server.cfg); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("database", "restore.db")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("not accepted before password"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/backups/restore/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	_, ok := server.enqueueUploadedRestore(recorder, request, restoreOwnerUser())
	if ok || recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "restore_reauthentication_required") {
		t.Fatalf("database-first upload ok=%v status=%d body=%s", ok, recorder.Code, recorder.Body.String())
	}
	entries, err := os.ReadDir(filepath.Join(root, "restore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".db") || strings.Contains(entry.Name(), "operation") {
			t.Fatalf("unauthorized upload left restore artifact %q", entry.Name())
		}
	}
}

func TestRestoreDeclaredBytesIsStrictlyBoundedAndExact(t *testing.T) {
	for _, test := range []struct {
		value string
		max   int64
		want  int64
		ok    bool
	}{
		{value: "7", max: 10, want: 7, ok: true},
		{value: "", max: 10, ok: false},
		{value: "-1", max: 10, ok: false},
		{value: "11", max: 10, ok: false},
		{value: "999999999999999999999999", max: 10, ok: false},
	} {
		got, err := parseDeclaredRestoreBytes(test.value, test.max)
		if test.ok && (err != nil || got != test.want) {
			t.Fatalf("parse %q = %d, %v; want %d", test.value, got, err, test.want)
		}
		if !test.ok && err == nil {
			t.Fatalf("parse %q unexpectedly succeeded with %d", test.value, got)
		}
	}
}

func TestRestoreMultipartTextUsesOneByteOverflowSentinel(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{name: "exact password limit", body: strings.Repeat("p", 256), want: true},
		{name: "password limit plus one", body: strings.Repeat("p", 257), want: false},
		{name: "exact confirmation limit", body: strings.Repeat("c", 128), want: true},
		{name: "confirmation limit plus one", body: strings.Repeat("c", 129), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			maximum := int64(256)
			if strings.Contains(test.name, "confirmation") {
				maximum = 128
			}
			body, err := readRestoreMultipartText(strings.NewReader(test.body), maximum)
			if (err == nil) != test.want || test.want && string(body) != test.body {
				t.Fatalf("bounded read len=%d err=%v wantSuccess=%v", len(body), err, test.want)
			}
		})
	}
}

func TestRestoreAdmissionRejectsConcurrentDuplicateReservations(t *testing.T) {
	root := t.TempDir()
	server := &Server{cfg: config.Config{AppDataDir: root, DatabasePath: filepath.Join(root, "portico.db")}}
	if err := database.PreparePrivateDataPaths(server.cfg); err != nil {
		t.Fatal(err)
	}
	type result struct {
		operation database.RestoreOperation
		release   func()
		err       error
	}
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			operation, _, release, err := server.reserveUploadedRestore(restoreOwnerUser(), "session")
			results <- result{operation: operation, release: release, err: err}
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	for result := range results {
		if result.err == nil {
			successes++
			if result.release != nil {
				result.release()
			}
			_ = os.Remove(database.RestoreUploadOwnerLockPath(result.operation))
		} else if !errors.Is(result.err, errRestoreBusy) {
			t.Fatalf("duplicate reservation error=%v, want restore busy", result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent reservation successes=%d, want one", successes)
	}
}

func TestRestoreResponseAndCapabilityRemainTruthfulAfterSessionRevocation(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	client := http.DefaultClient
	loginUser(t, client, serverURL)
	// The database-level assertion below uses the current session row directly;
	// the status capability is intentionally independent of that session.
	var sessionCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessionCount); err != nil || sessionCount == 0 {
		t.Fatalf("expected an initiating session, count=%d err=%v", sessionCount, err)
	}
	operation := restoreTestOperation(server, "restore-session-revocation", false)
	statusToken := "status-session-revocation"
	operation.StatusTokenHash = hashToken(statusToken)
	if err := database.WriteRestoreOperation(server.cfg.AppDataDir, operation); err != nil {
		t.Fatal(err)
	}
	if err := server.invalidateRestoredAuthentication(context.Background()); err != nil {
		t.Fatalf("invalidate restored authentication: %v", err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessionCount); err != nil || sessionCount != 0 {
		t.Fatalf("sessions after successful restore invalidation=%d err=%v", sessionCount, err)
	}
	statusRequest := httptest.NewRequest(http.MethodGet, "/api/backups/restore/restore-session-revocation", nil)
	statusRequest.Header.Set(restoreStatusHeader, statusToken)
	response, status, ok := server.restoreStatusResponse(statusRequest, operation.OperationID, false, nil)
	if !ok || status != http.StatusOK || !response.OK {
		t.Fatalf("status capability after revocation response=%#v status=%d ok=%v", response, status, ok)
	}
}

func TestRestoreRollbackDoesNotRevokePriorSession(t *testing.T) {
	serverURL, _, server := newAuthTestServerWithInstance(t)
	client := http.DefaultClient
	loginUser(t, client, serverURL)
	operation := restoreTestOperation(server, "restore-rollback-session", true)
	if err := database.WriteRestoreOperation(server.cfg.AppDataDir, operation); err != nil {
		t.Fatal(err)
	}
	if err := server.CompleteRestoreGeneration(context.Background(), operation.OperationID); err != nil {
		t.Fatalf("complete rollback generation: %v", err)
	}
	var sessionCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessionCount); err != nil || sessionCount == 0 {
		t.Fatalf("rollback removed prior session count=%d err=%v", sessionCount, err)
	}
}

func TestRestoreResponseMarksRecoveryRequiredAsNonSuccess(t *testing.T) {
	operation := database.RestoreOperation{OperationID: "restore-response-recovery", BackupName: "backup.db", Phase: database.RestorePhaseRollingBack, State: database.RestoreStateRecoveryNeeded, Progress: 100}
	response := restoreResponse(operation, "status-token")
	if response.OK || !response.RecoveryRequired || response.StatusToken != "status-token" {
		t.Fatalf("recovery response=%#v", response)
	}
}

func TestRawImportIdentityMismatchIsRejectedBeforeRestoreController(t *testing.T) {
	identity := database.MigrationIdentity{
		FormatVersion: 2, MigrationHead: "001_initial", LedgerSHA256: "ledger",
		MinimumReader: "1",
	}
	operation := database.RestoreOperation{
		RawImport: true, ImportedSizeBytes: 4096, ImportedChecksumSHA256: "checksum", ImportedIdentity: identity,
	}
	base := database.RestoreValidation{SizeBytes: 4096, ChecksumSHA256: "checksum", Migration: identity}
	for _, mutate := range []func(*database.RestoreOperation, *database.RestoreValidation){
		func(op *database.RestoreOperation, _ *database.RestoreValidation) { op.ImportedSizeBytes++ },
		func(op *database.RestoreOperation, _ *database.RestoreValidation) {
			op.ImportedChecksumSHA256 = "different"
		},
		func(op *database.RestoreOperation, _ *database.RestoreValidation) {
			op.ImportedIdentity.MigrationHead = "foreign-head"
		},
		func(op *database.RestoreOperation, validation *database.RestoreValidation) {
			validation.Migration.MinimumReader = "2"
		},
	} {
		candidate := operation
		validation := base
		mutate(&candidate, &validation)
		if rawImportIdentityMatches(candidate, validation) {
			t.Fatalf("tampered raw-import identity was accepted: operation=%#v validation=%#v", candidate, validation)
		}
	}
	if !rawImportIdentityMatches(operation, base) {
		t.Fatal("untampered raw-import identity was rejected")
	}
}

func TestBeginShutdownWaitsForOwnedDatabaseWriter(t *testing.T) {
	background, cancel := context.WithCancel(context.Background())
	server := &Server{backgroundCtx: background, backgroundCancel: cancel}
	started := make(chan struct{})
	release := make(chan struct{})
	if !server.startOwnedAsync("restore-writer-test", func(context.Context) {
		close(started)
		<-release
	}) {
		t.Fatal("owned writer was not started")
	}
	<-started
	server.BeginShutdown()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- server.Shutdown(context.Background()) }()
	select {
	case <-shutdownDone:
		t.Fatal("shutdown crossed an in-flight owned writer")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("shutdown after writer joined: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join owned writer")
	}
}

func TestRestoreRunnerJoinBlocksShutdownBeforeControllerEntry(t *testing.T) {
	server := &Server{}
	started := make(chan struct{})
	release := make(chan struct{})
	if !server.startRestoreRunnerFunc(func() {
		close(started)
		<-release
	}) {
		t.Fatal("restore runner was not admitted")
	}
	<-started
	stopDone := make(chan error, 1)
	go func() { stopDone <- server.StopRestoreRunners(context.Background()) }()
	select {
	case <-stopDone:
		t.Fatal("host shutdown joined before the stalled restore runner released")
	case <-time.After(20 * time.Millisecond):
	}
	if server.startRestoreRunnerFunc(func() {}) {
		t.Fatal("new restore runner crossed the shutdown admission boundary")
	}
	close(release)
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("stop restore runners: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("host shutdown did not join the restore runner")
	}
}

func TestRestoreQuiescenceCancelsRegisteredTranscodeAfterAdmissionSeals(t *testing.T) {
	server := &Server{transcodes: map[string]*transcodeSession{}}
	done := make(chan struct{})
	var cancelOnce sync.Once
	session := &transcodeSession{
		key:             "active-restore-quiescence-transcode",
		mediaID:         "restore-quiescence-media",
		done:            done,
		updateCh:        make(chan struct{}),
		admissionActive: true,
		cancel: func() {
			cancelOnce.Do(func() { close(done) })
		},
	}
	server.transcodes[session.key] = session

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.quiesceForRestore(ctx); err != nil {
		t.Fatalf("quiesce active transcode: %v", err)
	}
	select {
	case <-done:
	default:
		t.Fatal("quiescence returned before the active transcode was canceled and drained")
	}
	if !server.restoreBarrier.isBlocked() {
		t.Fatal("restore admission was not sealed after quiescence")
	}
	server.restoreBarrier.unblock()
}

func TestRestoreQuiescenceCancelsTranscodeBeforeWaitingHTTPAdmissionLease(t *testing.T) {
	server := &Server{transcodes: map[string]*transcodeSession{}}
	requestContext, releaseRequest, err := server.restoreBarrier.acquire(context.Background())
	if err != nil || requestContext == nil {
		t.Fatalf("acquire simulated HTTP request lease: %v", err)
	}
	done := make(chan struct{})
	var once sync.Once
	session := &transcodeSession{
		key:      "request-dependent-transcode",
		done:     done,
		updateCh: make(chan struct{}),
		cancel:   func() { once.Do(func() { close(done) }) },
	}
	server.transcodes[session.key] = session
	// Model an admitted HLS request: it only returns its HTTP lease after the
	// transcode it was waiting on has been canceled and drained.
	go func() {
		<-done
		releaseRequest()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.quiesceForRestore(ctx); err != nil {
		t.Fatalf("quiesce request-dependent transcode: %v", err)
	}
	server.restoreBarrier.unblock()
}

func TestRestoreQuiescenceRescansTranscodeRegisteredDuringAdmissionDrain(t *testing.T) {
	server := &Server{transcodes: map[string]*transcodeSession{}}
	_, releaseAdmission, err := server.restoreBarrier.acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire in-flight admission lease: %v", err)
	}
	done := make(chan struct{})
	var once sync.Once
	session := &transcodeSession{
		key:      "late-restore-quiescence-transcode",
		done:     done,
		updateCh: make(chan struct{}),
		cancel:   func() { once.Do(func() { close(done) }) },
	}
	server.restoreQuiesceAfterInitialCancelHook = func() {
		// This models an admission operation that crossed the first transcode
		// snapshot and published its session just before its lease returned.
		server.transcodeMu.Lock()
		server.transcodes[session.key] = session
		server.transcodeMu.Unlock()
		releaseAdmission()
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.quiesceForRestore(ctx); err != nil {
		t.Fatalf("quiesce late transcode registration: %v", err)
	}
	select {
	case <-done:
	default:
		t.Fatal("final quiescence drain missed a session registered during admission drain")
	}
	server.restoreBarrier.unblock()
}

func TestRestoreQuiescenceWaitsForRunningJobHeartbeatToExit(t *testing.T) {
	server := newScannerTestServer(t)
	heartbeatRelease := make(chan struct{})
	heartbeatStarted := make(chan struct{})
	server.jobHeartbeatForTest = func(context.Context, string) {
		close(heartbeatStarted)
		<-heartbeatRelease
	}
	jobContext, done := server.registerRunningJob("restore-heartbeat-join")
	if jobContext == nil {
		t.Fatal("running job context was nil")
	}
	<-heartbeatStarted
	jobDone := make(chan struct{})
	go func() {
		done()
		close(jobDone)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := server.quiesceForRestore(ctx); err == nil {
		t.Fatal("quiescence crossed a blocked running-job heartbeat")
	}
	select {
	case <-jobDone:
		t.Fatal("running job was reported joined before its heartbeat exited")
	default:
	}
	close(heartbeatRelease)
	select {
	case <-jobDone:
	case <-time.After(time.Second):
		t.Fatal("running job did not join after heartbeat release")
	}
	server.restoreBarrier.unblock()
}

func restoreTestOperation(server *Server, operationID string, rollback bool) database.RestoreOperation {
	return database.RestoreOperation{
		Version: database.RestoreOperationVersion, OperationID: operationID, BackupName: "backup.db",
		ActivePath: server.cfg.DatabasePath, StagedPath: database.CanonicalRestoreStagedPath(server.cfg, operationID, false),
		SafetyCopyPath: database.CanonicalRestoreSafetyCopyPath(server.cfg, operationID),
		OldActivePath:  database.CanonicalRestoreOldActivePath(server.cfg, operationID),
		InstallPath:    database.CanonicalRestoreInstallPath(server.cfg, operationID),
		Phase:          database.RestorePhaseHealthChecking, State: database.RestorePhaseHealthChecking, Progress: 90,
		RollbackPendingHealth: rollback, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func TestRestoreUploadResponseDoesNotEchoFilesystemPaths(t *testing.T) {
	operation := database.RestoreOperation{OperationID: "restore-path-free", BackupName: "uploaded-database", Phase: database.RestorePhaseStaged, State: database.RestorePhaseStaged, StagedPath: "/private/restore/path.db"}
	body, err := json.Marshal(restoreResponse(operation, "status"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "StagedPath") || strings.Contains(string(body), "private/restore") {
		t.Fatalf("restore response leaked internal path: %s", body)
	}
}
