package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
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

func signedRestoreAuthorizationForTest(t *testing.T, document hostedRestoreAuthorization) json.RawMessage {
	t.Helper()
	document.Signature = ""
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := canonicalHostedDocument(hostedRestoreAuthorizationKind, raw)
	if err != nil {
		t.Fatal(err)
	}
	document.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(testHostedDocumentPrivateKey(), payload))
	raw, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func restoreOwnerUser() User {
	return User{ID: "owner", AccountID: "owner", ProfileID: "owner", ProfileIsPrimary: true, Role: "owner", AuthOrigin: "local", AuthProvider: "local", HasLocalPassword: true, Permissions: map[string]bool{"manageServer": true}}
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
		{name: "hosted owner requires signed proof", user: User{ID: "owner", AccountID: "owner", ProfileID: "profile", ProfileIsPrimary: true, Role: "owner", AuthOrigin: "portico", AuthProvider: "portico", Permissions: map[string]bool{"manageServer": true}}, secret: "", status: http.StatusUnauthorized, code: "restore_hosted_reauthentication_required"},
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
			operation, _, release, err := server.reserveUploadedRestoreAuthorized(context.Background(), User{}, restoreAuthorizationSnapshot{SessionID: "session"})
			results <- result{operation: operation, release: release, err: err}
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	for result := range results {
		if result.err == nil {
			successes++
			if !result.operation.AuthorizationCommitted {
				t.Fatal("successful restore reservation did not publish committed authorization")
			}
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

func TestHostedRestoreAuthorizationBindsExactServerOwnerEpochAndSignature(t *testing.T) {
	_, _, server := newAuthTestServerWithInstance(t)
	server.cfg.HostedDocumentPublicKeys = testHostedDocumentPublicKeys()
	if err := server.saveRemoteAccessSettings(RemoteAccessSettings{
		Enabled: true, HostedBaseURL: "https://hosted.invalid", ClaimStatus: "claimed", ServerID: "srv-restore",
		PublicPortMode: "disabled", PreferredRemoteAuthMode: "portico",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	document := hostedRestoreAuthorization{
		Kind: hostedRestoreAuthorizationKind, Version: 1, Audience: hostedRestoreAuthorizationAudience,
		AuthorizationID: "sra_exact", Purpose: hostedRestoreAuthorizationPurpose, ServerID: "srv-restore", AccountID: "hosted-owner",
		RestoreSecurityEpoch: 7, IssuedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), ExpiresAt: now.Add(4 * time.Minute).Format(time.RFC3339Nano),
		SignatureAlgorithm: hostedSignatureAlgorithm, SignatureKeyID: testHostedDocumentKeyID,
	}
	raw := signedRestoreAuthorizationForTest(t, document)
	var signedDocument hostedRestoreAuthorization
	if err := json.Unmarshal(raw, &signedDocument); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // configured bootstrap trust keeps this focused test offline
	user := User{ID: "local-owner", PorticoUserID: "hosted-owner", AuthOrigin: "portico", AuthProvider: "portico"}
	if _, err := server.verifyHostedRestoreAuthorization(ctx, raw, user, 7, now); err != nil {
		t.Fatalf("valid exact restore authorization rejected: %v", err)
	}
	for name, mutate := range map[string]func(*hostedRestoreAuthorization){
		"server":    func(value *hostedRestoreAuthorization) { value.ServerID = "srv-other" },
		"owner":     func(value *hostedRestoreAuthorization) { value.AccountID = "other-owner" },
		"epoch":     func(value *hostedRestoreAuthorization) { value.RestoreSecurityEpoch++ },
		"signature": func(value *hostedRestoreAuthorization) { value.AuthorizationID = "sra_tampered" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := document
			mutate(&candidate)
			candidateRaw := raw
			if name != "signature" {
				candidateRaw = signedRestoreAuthorizationForTest(t, candidate)
			} else {
				candidate.Signature = signedDocument.Signature
				candidateRaw, _ = json.Marshal(candidate)
			}
			candidateContext, stop := context.WithCancel(context.Background())
			stop()
			if _, err := server.verifyHostedRestoreAuthorization(candidateContext, candidateRaw, user, 7, now); err == nil {
				t.Fatalf("%s mismatch was accepted", name)
			}
		})
	}
}

func TestRestoreSecurityFenceDistrustsCandidateIdempotenceClaimAndQuarantinesHostedProjection(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	var accountID string
	if err := db.QueryRow(`SELECT id FROM users WHERE role = 'owner' LIMIT 1`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO hosted_profile_snapshot_state (account_id, snapshot_id, revision, payload_digest, issued_at, expires_at, applied_at, checked_at) VALUES (?, 'restored', 99, 'restored-digest', ?, ?, ?, ?)`, accountID, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	operation := database.RestoreOperation{OperationID: "restore-epoch-once", PreRestoreSecurityEpoch: 8, AuthorizationCommitted: true}
	// The installed database is hostile restore input. Preloading the current
	// operation ID must not let it claim that the out-of-database fence ran.
	if _, err := db.Exec(`UPDATE server_security_state SET restore_security_epoch = 4, last_restore_operation_id = ? WHERE id = 1`, operation.OperationID); err != nil {
		t.Fatal(err)
	}
	if err := server.fenceRestoredSecurityState(context.Background(), &operation); err != nil {
		t.Fatal(err)
	}
	if err := server.fenceRestoredSecurityState(context.Background(), &operation); err != nil {
		t.Fatalf("idempotent fence retry failed: %v", err)
	}
	var epoch int64
	var appliedOperationID, quarantinedAt string
	if err := db.QueryRow(`SELECT restore_security_epoch, last_restore_operation_id FROM server_security_state WHERE id = 1`).Scan(&epoch, &appliedOperationID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT quarantined_at FROM hosted_profile_snapshot_state WHERE account_id = ?`, accountID).Scan(&quarantinedAt); err != nil {
		t.Fatal(err)
	}
	if epoch != 9 || appliedOperationID != operation.OperationID || quarantinedAt == "" {
		t.Fatalf("fence state epoch=%d operation=%q quarantined=%q", epoch, appliedOperationID, quarantinedAt)
	}
	if !operation.SecurityFenceApplied || operation.AppliedSecurityEpoch != 9 {
		t.Fatalf("out-of-database fence evidence=%+v", operation)
	}
	journaled, err := database.ReadRestoreOperation(server.cfg.AppDataDir)
	if err != nil || !journaled.SecurityFenceApplied || journaled.AppliedSecurityEpoch != 9 {
		t.Fatalf("durable out-of-database fence evidence=%+v err=%v", journaled, err)
	}
	if _, err := server.hostedProfileStateContext(context.Background(), accountID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("quarantined Hosted projection remained authoritative: %v", err)
	}
	var sessions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("restored sessions remained after fence: count=%d err=%v", sessions, err)
	}
}

func TestRestoreAuthorizationReceiptConsumesOnceWithExactServerBinding(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	client := http.DefaultClient
	loginUser(t, client, serverURL)
	var user User
	if err := server.queryUserRow(context.Background(), `SELECT id, id, id, role FROM users WHERE role = 'owner' LIMIT 1`).Scan(&user.ID, &user.AccountID, &user.ProfileID, &user.Role); err != nil {
		t.Fatal(err)
	}
	var sessionID string
	if err := db.QueryRow(`SELECT id FROM sessions WHERE user_id = ? LIMIT 1`, user.ID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	snapshot := restoreAuthorizationSnapshot{
		SessionID: sessionID, HostedAuthorizationID: "sra_single_use", HostedAuthorizationServerID: "srv-exact",
		HostedAuthorizationExpiresAt: time.Now().UTC().Add(time.Minute), PreRestoreSecurityEpoch: 0,
	}
	consume := func() error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := validateRestoreReservationTx(tx, user, snapshot, true, time.Now().UTC()); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err := consume(); err != nil {
		t.Fatalf("first receipt consumption failed: %v", err)
	}
	if err := consume(); !errors.Is(err, errPrivilegedSessionChanged) {
		t.Fatalf("replayed receipt error=%v, want privileged-session fence", err)
	}
	expired := snapshot
	expired.HostedAuthorizationID = "sra_expired_before_reservation"
	expired.HostedAuthorizationExpiresAt = time.Now().UTC().Add(-time.Second)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRestoreReservationTx(tx, user, expired, true, time.Now().UTC()); !errors.Is(err, errPrivilegedSessionChanged) {
		_ = tx.Rollback()
		t.Fatalf("expired authorization reservation error=%v, want privileged-session fence", err)
	}
	_ = tx.Rollback()
	var serverID string
	if err := db.QueryRow(`SELECT server_id FROM hosted_restore_authorization_receipts WHERE authorization_id = ?`, snapshot.HostedAuthorizationID).Scan(&serverID); err != nil || serverID != "srv-exact" {
		t.Fatalf("receipt server binding=%q err=%v", serverID, err)
	}
}

func TestRestoreAuthorizationCommitGapFailsClosedForLocalAndHostedProofs(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	client := http.DefaultClient
	loginUser(t, client, serverURL)
	var user User
	var sessionID, passwordHash string
	if err := db.QueryRow(`
		SELECT u.id, u.id, COALESCE(NULLIF(session.profile_id, ''), u.id), u.role,
		       session.id, COALESCE(u.password_hash, '')
		FROM users u
		JOIN sessions session ON session.user_id = u.id
		WHERE u.role = 'owner'
		LIMIT 1`).Scan(&user.ID, &user.AccountID, &user.ProfileID, &user.Role, &sessionID, &passwordHash); err != nil {
		t.Fatal(err)
	}
	var epoch int64
	if err := db.QueryRow(`SELECT restore_security_epoch FROM server_security_state WHERE id = 1`).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("simulated process stop after SQLite authorization commit")
	restoreHook := setRestoreAuthorizationAfterDatabaseCommitForTest(func() error { return interrupted })
	defer restoreHook()

	for _, test := range []struct {
		name   string
		hosted bool
	}{
		{name: "local password"},
		{name: "Hosted receipt", hosted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			operationID := "restore-commit-gap-" + strings.ReplaceAll(test.name, " ", "-")
			now := time.Now().UTC()
			authorization := restoreAuthorizationSnapshot{
				SessionID: sessionID, ExpectedPasswordHash: passwordHash, PreRestoreSecurityEpoch: epoch,
			}
			if test.hosted {
				authorization.ExpectedPasswordHash = ""
				authorization.HostedAuthorizationID = "sra_commit_gap"
				authorization.HostedAuthorizationServerID = "srv-exact"
				authorization.HostedAuthorizationExpiresAt = now.Add(time.Minute)
			}
			operation := database.RestoreOperation{
				Version: database.RestoreOperationVersion, OperationID: operationID, BackupName: "backup.db",
				StagedPath: database.CanonicalRestoreStagedPath(server.cfg, operationID, false), ActivePath: server.cfg.DatabasePath,
				SafetyCopyPath: database.CanonicalRestoreSafetyCopyPath(server.cfg, operationID),
				OldActivePath:  database.CanonicalRestoreOldActivePath(server.cfg, operationID),
				InstallPath:    database.CanonicalRestoreInstallPath(server.cfg, operationID),
				AccountID:      user.AccountID, SessionID: sessionID, PreRestoreSecurityEpoch: epoch,
				HostedAuthorizationID: authorization.HostedAuthorizationID,
				Phase:                 database.RestorePhaseValidating, State: database.RestorePhaseValidating, Progress: 10,
				CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
			}
			if err := os.WriteFile(operation.StagedPath, []byte("untrusted staged candidate"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := server.commitRestoreReservationAuthorization(context.Background(), user, authorization, &operation, true, now)
			if !errors.Is(err, interrupted) {
				t.Fatalf("commit-gap error=%v, want simulated interruption", err)
			}
			journaled, err := database.ReadRestoreOperation(server.cfg.AppDataDir)
			if err != nil || journaled.AuthorizationCommitted {
				t.Fatalf("unconfirmed journal=%+v err=%v", journaled, err)
			}
			if _, claimed, err := database.ClaimRestoreOperation(server.cfg, operationID, "commit-gap-runner"); err != nil || claimed {
				t.Fatalf("unconfirmed restore executor claim claimed=%v err=%v", claimed, err)
			}
			if test.hosted {
				var receipts int
				if err := db.QueryRow(`SELECT COUNT(*) FROM hosted_restore_authorization_receipts WHERE authorization_id = ?`, authorization.HostedAuthorizationID).Scan(&receipts); err != nil || receipts != 1 {
					t.Fatalf("Hosted proof was not durably burned before interruption: count=%d err=%v", receipts, err)
				}
			}
			if err := database.RecoverInterruptedRestoreBeforeOpen(server.cfg); err != nil {
				t.Fatalf("recover uncommitted authorization: %v", err)
			}
			journaled, err = database.ReadRestoreOperation(server.cfg.AppDataDir)
			if err != nil || journaled.Phase != database.RestorePhaseFailed || journaled.ErrorCode != "restore_authorization_not_committed" {
				t.Fatalf("uncommitted restore recovery=%+v err=%v", journaled, err)
			}
			if _, err := os.Lstat(operation.StagedPath); !os.IsNotExist(err) {
				t.Fatalf("uncommitted staged candidate remained executable: %v", err)
			}
		})
	}
}

func TestRestoreResponseMarksRecoveryRequiredAsNonSuccess(t *testing.T) {
	operation := database.RestoreOperation{OperationID: "restore-response-recovery", BackupName: "backup.db", AuthorizationCommitted: true, Phase: database.RestorePhaseRollingBack, State: database.RestoreStateRecoveryNeeded, Progress: 100}
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
		AuthorizationCommitted: true,
		RawImport:              true, ImportedSizeBytes: 4096, ImportedChecksumSHA256: "checksum", ImportedIdentity: identity,
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

func TestRestoreRunnerKeepsExecutorLockThroughHostController(t *testing.T) {
	_, _, server := newAuthTestServerWithInstance(t)
	operation := restoreTestOperation(server, "restore-executor-host-boundary", false)
	operation.Phase = database.RestorePhaseStaged
	operation.State = database.RestorePhaseStaged
	operation.Progress = 25
	if err := database.WriteRestoreOperation(server.cfg.AppDataDir, operation); err != nil {
		t.Fatalf("write staged restore operation: %v", err)
	}

	controllerStarted := make(chan struct{})
	releaseController := make(chan struct{})
	controllerReleased := false
	defer func() {
		if !controllerReleased {
			close(releaseController)
		}
		server.restoreBarrier.unblock()
	}()
	server.SetRestoreRuntimeController(func(context.Context, string) error {
		close(controllerStarted)
		<-releaseController
		return nil
	})
	runnerDone := make(chan struct{})
	go func() {
		server.runRestoreOperation(operation.OperationID)
		close(runnerDone)
	}()
	select {
	case <-controllerStarted:
	case <-time.After(time.Second):
		t.Fatal("restore runner did not reach the host controller")
	}

	release, acquired, err := database.TryAcquireRestoreExecutorLock(server.cfg)
	if err != nil {
		t.Fatalf("probe restore executor ownership: %v", err)
	}
	if acquired {
		release()
		t.Fatal("restore executor lock was reacquired while the host controller remained active")
	}

	close(releaseController)
	controllerReleased = true
	select {
	case <-runnerDone:
	case <-time.After(time.Second):
		t.Fatal("restore runner did not return after the host controller completed")
	}
	release, acquired, err = database.TryAcquireRestoreExecutorLock(server.cfg)
	if err != nil {
		t.Fatalf("reacquire restore executor after controller: %v", err)
	}
	if !acquired {
		t.Fatal("restore executor lock remained held after the host controller returned")
	}
	release()
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
		AuthorizationCommitted: true,
		ActivePath:             server.cfg.DatabasePath, StagedPath: database.CanonicalRestoreStagedPath(server.cfg, operationID, false),
		SafetyCopyPath: database.CanonicalRestoreSafetyCopyPath(server.cfg, operationID),
		OldActivePath:  database.CanonicalRestoreOldActivePath(server.cfg, operationID),
		InstallPath:    database.CanonicalRestoreInstallPath(server.cfg, operationID),
		Phase:          database.RestorePhaseHealthChecking, State: database.RestorePhaseHealthChecking, Progress: 90,
		RollbackPendingHealth: rollback, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func TestRestoreUploadResponseDoesNotEchoFilesystemPaths(t *testing.T) {
	operation := database.RestoreOperation{OperationID: "restore-path-free", BackupName: "uploaded-database", AuthorizationCommitted: true, Phase: database.RestorePhaseStaged, State: database.RestorePhaseStaged, StagedPath: "/private/restore/path.db"}
	body, err := json.Marshal(restoreResponse(operation, "status"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "StagedPath") || strings.Contains(string(body), "private/restore") {
		t.Fatalf("restore response leaked internal path: %s", body)
	}
}
