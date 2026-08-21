package app

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/database"
)

const restoreUploadLease = 30 * time.Minute

const (
	restoreMultipartOverhead = 256 << 10
	restoreDrainTimeout      = 15 * time.Second
	restoreStatusHeader      = "X-Portico-Restore-Status"
)

var restoreUploadReservationTestState struct {
	sync.RWMutex
	afterOwnerLock func()
}

func setRestoreUploadReservationAfterOwnerLockForTest(hook func()) func() {
	restoreUploadReservationTestState.Lock()
	previous := restoreUploadReservationTestState.afterOwnerLock
	restoreUploadReservationTestState.afterOwnerLock = hook
	restoreUploadReservationTestState.Unlock()
	return func() {
		restoreUploadReservationTestState.Lock()
		restoreUploadReservationTestState.afterOwnerLock = previous
		restoreUploadReservationTestState.Unlock()
	}
}

func runRestoreUploadReservationAfterOwnerLockHook() {
	restoreUploadReservationTestState.RLock()
	hook := restoreUploadReservationTestState.afterOwnerLock
	restoreUploadReservationTestState.RUnlock()
	if hook != nil {
		hook()
	}
}

func (s *Server) restoreDatabaseLimit() int64 {
	if s != nil && s.cfg.RestoreMaxDatabaseBytes > 0 {
		return s.cfg.RestoreMaxDatabaseBytes
	}
	return database.RestoreMaxDatabaseBytes
}

func (s *Server) restoreIOTimeout() time.Duration {
	if s != nil && s.cfg.RestoreIOTimeout > 0 {
		return s.cfg.RestoreIOTimeout
	}
	return 10 * time.Minute
}

// RestoreRuntimeController is owned by the process host. The app generation
// never swaps its own *sql.DB; it asks the host to replace the full runtime
// generation after the initiating request has returned.
type RestoreRuntimeController func(context.Context, string) error

type restoreAdmissionBarrier struct {
	mu      sync.Mutex
	blocked bool
	nextID  uint64
	active  map[uint64]context.CancelFunc
	notify  chan struct{}
}

func (b *restoreAdmissionBarrier) initializeLocked() {
	if b.active == nil {
		b.active = map[uint64]context.CancelFunc{}
	}
	if b.notify == nil {
		b.notify = make(chan struct{})
	}
}

func (b *restoreAdmissionBarrier) signalLocked() {
	if b.notify != nil {
		close(b.notify)
	}
	b.notify = make(chan struct{})
}

func (b *restoreAdmissionBarrier) acquire(parent context.Context) (context.Context, func(), error) {
	if parent == nil {
		parent = context.Background()
	}
	b.mu.Lock()
	b.initializeLocked()
	if b.blocked {
		b.mu.Unlock()
		return nil, nil, errors.New("restore admission is quiescing")
	}
	b.nextID++
	id := b.nextID
	ctx, cancel := context.WithCancel(parent)
	b.active[id] = cancel
	b.mu.Unlock()
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.active, id)
			b.signalLocked()
			b.mu.Unlock()
			cancel()
		})
	}, nil
}

func (b *restoreAdmissionBarrier) isBlocked() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.blocked
}

func (b *restoreAdmissionBarrier) quiesce(ctx context.Context) error {
	b.seal()
	return b.wait(ctx)
}

// seal closes the admission gate and cancels every request/producer that was
// admitted before the gate closed. It deliberately does not wait: restore
// quiescence must first cancel the jobs, transcodes, and streams those
// requests may be waiting on, otherwise a request lease can hold the barrier
// while its dependent transcode waits for restore quiescence to cancel it.
func (b *restoreAdmissionBarrier) seal() {
	b.mu.Lock()
	b.initializeLocked()
	b.blocked = true
	cancels := make([]context.CancelFunc, 0, len(b.active))
	for _, cancel := range b.active {
		cancels = append(cancels, cancel)
	}
	b.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (b *restoreAdmissionBarrier) wait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	b.mu.Lock()
	b.initializeLocked()
	for len(b.active) > 0 {
		notify := b.notify
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notify:
		}
		b.mu.Lock()
	}
	b.mu.Unlock()
	return nil
}

func (b *restoreAdmissionBarrier) unblock() {
	b.mu.Lock()
	b.initializeLocked()
	b.blocked = false
	b.signalLocked()
	b.mu.Unlock()
}

// cancelForTest deterministically exercises the narrow dispatch/register
// window without waiting on the lease it is canceling.
func (b *restoreAdmissionBarrier) cancelForTest() {
	b.mu.Lock()
	b.initializeLocked()
	b.blocked = true
	for _, cancel := range b.active {
		cancel()
	}
	b.signalLocked()
	b.mu.Unlock()
}

func (s *Server) SetRestoreRuntimeController(controller RestoreRuntimeController) {
	s.restoreRuntimeMu.Lock()
	s.restoreRuntimeController = controller
	s.restoreRuntimeMu.Unlock()
}

// RestoreRuntimeFailure releases the admission fence only when the host has
// proven that no filesystem mutation occurred and the old generation remains
// the owner. Post-close failures must rebuild a fresh generation instead.
func (s *Server) RestoreRuntimeFailure() {
	s.restoreBarrier.unblock()
}

func (s *Server) restoreRuntime() RestoreRuntimeController {
	s.restoreRuntimeMu.RLock()
	defer s.restoreRuntimeMu.RUnlock()
	return s.restoreRuntimeController
}

func (s *Server) restoreAdmission(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The host maintenance responder owns this route while the old runtime
		// is closed. The normal handler may also serve it before/after a swap.
		if isRestoreStatusPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		ctx, release, err := s.restoreBarrier.acquire(r.Context())
		if err != nil {
			w.Header().Set("Retry-After", "2")
			writeProductError(w, http.StatusServiceUnavailable, "restore_in_progress", "Portico is applying a supervised database restore.")
			return
		}
		defer release()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isRestoreStatusPath(path string) bool {
	_, ok := restoreStatusOperationID(path)
	return ok
}

func restoreStatusOperationID(path string) (string, bool) {
	const prefix = "/api/backups/restore/"
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	value := strings.TrimPrefix(path, prefix)
	value = strings.TrimSuffix(value, "/")
	if value == "" || strings.Contains(value, "/") || !validRestoreOperationIDForHTTP(value) {
		return "", false
	}
	return value, true
}

func (s *Server) checkRestorePrincipal(w http.ResponseWriter, user User) bool {
	if !canInteractivelyManageServer(user) {
		writeProductError(w, http.StatusForbidden, "restore_owner_session_required", "Database restore requires an interactive owner session with server-management authority.")
		return false
	}
	if strings.EqualFold(strings.TrimSpace(user.AuthProvider), "api_key") || strings.TrimSpace(user.APIKeyID) != "" {
		writeProductError(w, http.StatusForbidden, "restore_owner_session_required", "API keys cannot start a database restore.")
		return false
	}
	return true
}

// verifyRestoreReauthentication is intentionally distinct from profile PIN
// administration proofs. A local restore always verifies the account password
// even when the selected primary profile has a PIN. Hosted-origin owners do not
// have a local secret; W2 must provide a signed recent Hosted step-up proof.
func (s *Server) verifyRestoreReauthentication(w http.ResponseWriter, r *http.Request, user User, password string) (string, bool) {
	if user.AuthOrigin == "portico" || user.AuthProvider == "portico" {
		writeProductError(w, http.StatusConflict, "restore_hosted_reauthentication_required", "Hosted-origin restore requires the signed recent reauthentication boundary owned by W2.")
		return "", false
	}
	if !user.HasLocalPassword || strings.TrimSpace(password) == "" {
		writeProductError(w, http.StatusUnauthorized, "restore_reauthentication_required", "Enter the current account password to authorize this restore.")
		return "", false
	}
	accountID := accountIDForUser(user)
	var passwordHash string
	if err := s.queryUserRow(r.Context(), `SELECT COALESCE(password_hash, '') FROM users WHERE id = ?`, accountID).Scan(&passwordHash); err != nil {
		writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "restore_reauthentication_unavailable", "Portico could not verify the restore authorization.")
		return "", false
	}
	valid, err := s.verifyAndUpgradeLocalPassword(r.Context(), accountID, passwordHash, password)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusServiceUnavailable, "restore_reauthentication_unavailable", "Portico could not verify the restore authorization.")
		return "", false
	}
	if !valid {
		writeProductError(w, http.StatusUnauthorized, "restore_reauthentication_required", "The current account password is incorrect.")
		return "", false
	}
	sessionID, err := s.currentSessionIDContext(r.Context(), r, user)
	if err != nil {
		writeProductError(w, http.StatusUnauthorized, "restore_reauthentication_required", "Restore requires a current interactive owner session.")
		return "", false
	}
	return sessionID, true
}

type restoreStartRequest struct {
	Password     string `json:"password"`
	Confirmation string `json:"confirmation"`
}

func restoreConfirmationFor(name string) string {
	return "restore:" + strings.TrimSpace(name)
}

func (s *Server) enqueueExistingRestore(w http.ResponseWriter, r *http.Request, user User, backupName string) (RestoreBackupResponse, bool) {
	if !s.checkRestorePrincipal(w, user) {
		return RestoreBackupResponse{}, false
	}
	var request restoreStartRequest
	if !decodeJSON(w, r, &request) {
		return RestoreBackupResponse{}, false
	}
	sessionID, ok := s.verifyRestoreReauthentication(w, r, user, request.Password)
	if !ok {
		return RestoreBackupResponse{}, false
	}
	if strings.TrimSpace(request.Confirmation) != restoreConfirmationFor(backupName) {
		writeProductError(w, http.StatusBadRequest, "restore_confirmation_required", "Confirm the selected backup before starting restore.")
		return RestoreBackupResponse{}, false
	}
	backupPath := filepath.Join(s.backupDir(), backupName)
	if err := database.ValidateRegularNonSymlinkFile(backupPath); err != nil {
		writeProductError(w, http.StatusBadRequest, "restore_unidentified_database", "The selected backup is not a trusted regular file.")
		return RestoreBackupResponse{}, false
	}
	manifest, err := database.ReadBackupManifest(backupPath)
	if err != nil {
		writeRestoreValidationError(w, err)
		return RestoreBackupResponse{}, false
	}
	if stat, statErr := os.Stat(backupPath); statErr != nil {
		writeProductError(w, http.StatusServiceUnavailable, "restore_storage_unavailable", "Portico could not inspect the selected backup.")
		return RestoreBackupResponse{}, false
	} else if spaceErr := database.RestoreSpaceAdmissionForPath(s.cfg, backupPath, stat.Size()); spaceErr != nil {
		writeProductError(w, http.StatusInsufficientStorage, "restore_insufficient_space", "There is not enough space to stage the backup and retain rollback headroom.")
		return RestoreBackupResponse{}, false
	}
	if _, err := database.ValidateRestoreCandidateWithLimit(r.Context(), backupPath, &manifest, s.restoreDatabaseLimit()); err != nil {
		writeRestoreValidationError(w, err)
		return RestoreBackupResponse{}, false
	}
	return s.createRestoreOperation(w, r, user, sessionID, backupName, backupPath, manifest)
}

func (s *Server) enqueueUploadedRestore(w http.ResponseWriter, r *http.Request, user User) (RestoreBackupResponse, bool) {
	if !s.checkRestorePrincipal(w, user) {
		return RestoreBackupResponse{}, false
	}
	if err := database.PreparePrivateDataPaths(s.cfg); err != nil {
		writeProductError(w, http.StatusServiceUnavailable, "restore_storage_unavailable", "Portico could not prepare private restore staging.")
		return RestoreBackupResponse{}, false
	}
	return s.enqueueUploadedRestoreStream(w, r, user)
}

func (s *Server) enqueueUploadedRestoreStream(w http.ResponseWriter, r *http.Request, user User) (RestoreBackupResponse, bool) {
	uploadContext, cancel := context.WithCancel(r.Context())
	defer cancel()
	r.Body = http.MaxBytesReader(w, r.Body, s.restoreDatabaseLimit()+restoreMultipartOverhead)
	reader, err := r.MultipartReader()
	if err != nil {
		writeProductError(w, http.StatusBadRequest, "restore_upload_invalid", "Use a bounded multipart upload with database, manifest, and password parts.")
		return RestoreBackupResponse{}, false
	}
	var operation database.RestoreOperation
	var statusToken string
	var uploadOwnerRelease func()
	var password string
	var sessionID string
	var reauthenticated, confirmed, reserved, manifestSeen, databaseSeen, passwordSeen, confirmationSeen, declaredBytesSeen bool
	var declaredBytes int64
	var manifest database.BackupManifest
	failUpload := func(code string, status int, message string) (RestoreBackupResponse, bool) {
		if reserved {
			s.failReservedUpload(&operation, code)
		}
		writeProductError(w, status, code, message)
		return RestoreBackupResponse{}, false
	}
	defer func() {
		if uploadOwnerRelease != nil {
			uploadOwnerRelease()
			if ownerLock := database.RestoreUploadOwnerLockPath(operation); ownerLock != "" {
				_ = os.Remove(ownerLock)
			}
			uploadOwnerRelease = nil
		}
	}()
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return failUpload("restore_upload_invalid", http.StatusBadRequest, "The multipart restore upload could not be read.")
		}
		field := strings.TrimSpace(part.FormName())
		switch field {
		case "password":
			if passwordSeen {
				_ = part.Close()
				return failUpload("restore_upload_invalid", http.StatusBadRequest, "Only one password part is accepted.")
			}
			passwordSeen = true
			body, readErr := readRestoreMultipartText(part, 256)
			if readErr != nil {
				_ = part.Close()
				return failUpload("restore_reauthentication_required", http.StatusBadRequest, "A current account password is required.")
			}
			password = string(body)
			var ok bool
			sessionID, ok = s.verifyRestoreReauthentication(w, r, user, password)
			if !ok {
				_ = part.Close()
				return RestoreBackupResponse{}, false
			}
			reauthenticated = true
		case "confirmation":
			if confirmationSeen {
				_ = part.Close()
				return failUpload("restore_upload_invalid", http.StatusBadRequest, "Only one confirmation part is accepted.")
			}
			confirmationSeen = true
			body, readErr := readRestoreMultipartText(part, 128)
			if readErr != nil || strings.TrimSpace(string(body)) != restoreConfirmationFor("uploaded-database") {
				_ = part.Close()
				return failUpload("restore_confirmation_required", http.StatusBadRequest, "Confirm this database import before uploading its contents.")
			}
			confirmed = true
		case "databaseBytes":
			if declaredBytesSeen {
				_ = part.Close()
				return failUpload("restore_upload_invalid", http.StatusBadRequest, "Only one declared database size is accepted.")
			}
			if databaseSeen {
				_ = part.Close()
				return failUpload("restore_upload_invalid", http.StatusBadRequest, "The declared database size must precede the database part.")
			}
			declaredBytesSeen = true
			body, readErr := io.ReadAll(io.LimitReader(part, 64))
			if readErr != nil || len(body) > 63 {
				_ = part.Close()
				return failUpload("restore_upload_invalid", http.StatusBadRequest, "The declared database size is invalid.")
			}
			declaredBytes, err = parseDeclaredRestoreBytes(string(body), s.restoreDatabaseLimit())
			if err != nil {
				_ = part.Close()
				return failUpload("restore_upload_invalid", http.StatusBadRequest, "The declared database size is invalid.")
			}
		case "manifest":
			if manifestSeen || databaseSeen {
				_ = part.Close()
				return failUpload("restore_upload_invalid", http.StatusBadRequest, "The manifest must precede the database part and may appear only once.")
			}
			manifestSeen = true
			body, readErr := io.ReadAll(io.LimitReader(part, 128<<10+1))
			if readErr != nil || len(body) > 128<<10 {
				_ = part.Close()
				return failUpload("restore_manifest_invalid", http.StatusBadRequest, "The uploaded backup manifest is invalid or too large.")
			}
			manifest, err = parseUploadedRestoreManifest(body)
			if err != nil {
				_ = part.Close()
				return failUpload("restore_manifest_invalid", http.StatusBadRequest, "The uploaded backup manifest is invalid.")
			}
		case "database":
			if !reauthenticated || !confirmed || !declaredBytesSeen {
				_ = part.Close()
				code := "restore_confirmation_required"
				status := http.StatusBadRequest
				message := "The explicit restore confirmation must precede the database part."
				if !reauthenticated {
					code = "restore_reauthentication_required"
					status = http.StatusUnauthorized
					message = "The bounded password part must precede the database part."
				} else if !declaredBytesSeen {
					code = "restore_upload_size_required"
					message = "The declared database size must precede the database part."
				}
				return failUpload(code, status, message)
			}
			if databaseSeen {
				_ = part.Close()
				return failUpload("restore_upload_invalid", http.StatusBadRequest, "Only one database part is accepted.")
			}
			if !reserved {
				if spaceErr := database.RestoreSpaceAdmissionForPath(s.cfg, "", declaredBytes); spaceErr != nil {
					_ = part.Close()
					return failUpload("restore_insufficient_space", http.StatusInsufficientStorage, "There is not enough space to stage the database and retain rollback headroom.")
				}
				operation, statusToken, uploadOwnerRelease, err = s.reserveUploadedRestore(user, sessionID)
				if err != nil {
					_ = part.Close()
					if errors.Is(err, errRestoreBusy) {
						return failUpload("restore_busy", http.StatusConflict, "Another supervised restore is already in progress.")
					}
					return failUpload("restore_storage_unavailable", http.StatusServiceUnavailable, "Portico could not reserve the supervised restore operation.")
				}
				reserved = true
			}
			databaseSeen = true
			count, writeErr := database.WritePrivateStreamContext(uploadContext, operation.StagedPath, part, s.restoreDatabaseLimit())
			if writeErr != nil {
				_ = part.Close()
				return failUpload("restore_upload_too_large", http.StatusRequestEntityTooLarge, "The uploaded database exceeds the restore size bound or could not be staged.")
			}
			operation.UploadBytes = count
			if declaredBytesSeen && count != declaredBytes {
				_ = part.Close()
				return failUpload("restore_upload_size_mismatch", http.StatusBadRequest, "The uploaded database did not match its declared size.")
			}
			operation.UploadComplete = true
			if err := s.updateReservedUpload(operation); err != nil {
				_ = part.Close()
				return failUpload("restore_storage_unavailable", http.StatusServiceUnavailable, "The restore upload could not be durably recorded.")
			}
		default:
			_ = part.Close()
			return failUpload("restore_upload_invalid", http.StatusBadRequest, "The multipart restore upload contains an unsupported part.")
		}
		_ = part.Close()
	}
	if !reserved {
		if !databaseSeen {
			writeProductError(w, http.StatusBadRequest, "restore_database_required", "A database part is required.")
		}
		return RestoreBackupResponse{}, false
	}
	if !databaseSeen || !operation.UploadComplete {
		return failUpload("restore_database_required", http.StatusBadRequest, "A database part is required.")
	}
	rawImport := !manifestSeen
	var importedIdentity database.MigrationIdentity
	if rawImport {
		validation, validationErr := database.InspectRestoreDatabaseWithLimit(uploadContext, operation.StagedPath, s.restoreDatabaseLimit())
		if validationErr != nil {
			writeRestoreValidationError(w, validationErr)
			s.failReservedUpload(&operation, validationErrorCode(validationErr))
			return RestoreBackupResponse{}, false
		}
		importedIdentity = validation.Migration
		operation.RawImport = true
		operation.ImportedIdentity = importedIdentity
		operation.ImportedSizeBytes = validation.SizeBytes
		operation.ImportedChecksumSHA256 = validation.ChecksumSHA256
	} else {
		manifestForStage := manifest
		if len(manifestForStage.ArtifactSet) == 1 {
			manifestForStage.ArtifactSet[0].Name = filepath.Base(operation.StagedPath)
		}
		if _, validationErr := database.ValidateRestoreCandidateForBackup(uploadContext, operation.StagedPath, &manifestForStage, manifest.BackupName, s.restoreDatabaseLimit()); validationErr != nil {
			writeRestoreValidationError(w, validationErr)
			s.failReservedUpload(&operation, validationErrorCode(validationErr))
			return RestoreBackupResponse{}, false
		}
		operation.Manifest = manifestForStage
	}
	operation.Phase, operation.State, operation.Progress = database.RestorePhaseStaged, database.RestorePhaseStaged, 25
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.updateReservedUpload(operation); err != nil {
		return failUpload("restore_storage_unavailable", http.StatusServiceUnavailable, "The restore operation could not be durably journaled.")
	}
	if s.restoreRuntime() != nil {
		s.startRestoreRunner(operation.OperationID)
	}
	return restoreResponse(operation, statusToken), true
}

var errRestoreMultipartPartTooLarge = errors.New("restore multipart text part exceeds its bound")

// readRestoreMultipartText reads one credential/confirmation field with one
// byte of sentinel headroom. Limiting to max itself makes len(body)>max
// unreachable and would accept an oversized password or confirmation.
func readRestoreMultipartText(part io.Reader, maximum int64) ([]byte, error) {
	if maximum < 0 {
		return nil, errRestoreMultipartPartTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(part, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maximum {
		return nil, errRestoreMultipartPartTooLarge
	}
	return body, nil
}

func parseDeclaredRestoreBytes(value string, maximum int64) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return 0, errors.New("declared restore size is not positive")
	}
	var parsed int64
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, errors.New("declared restore size is not numeric")
		}
		if parsed > (int64(^uint64(0)>>1)-int64(char-'0'))/10 {
			return 0, errors.New("declared restore size overflows")
		}
		parsed = parsed*10 + int64(char-'0')
	}
	if parsed <= 0 || (maximum > 0 && parsed > maximum) {
		return 0, errors.New("declared restore size exceeds the server bound")
	}
	return parsed, nil
}

var errRestoreBusy = errors.New("restore operation is busy")

func (s *Server) reserveUploadedRestore(user User, sessionID string) (database.RestoreOperation, string, func(), error) {
	var operation database.RestoreOperation
	statusToken := randomToken()
	var ownerRelease func()
	err := database.WithRestoreOperationLock(s.cfg, func() error {
		s.restoreOperationMu.Lock()
		defer s.restoreOperationMu.Unlock()
		if err := s.ensureRestoreSlotLocked(); err != nil {
			return errRestoreBusy
		}
		now := time.Now().UTC()
		operationID := randomID("restore")
		operation = database.RestoreOperation{
			Version: database.RestoreOperationVersion, OperationID: operationID, BackupName: "uploaded-database",
			StagedPath: database.CanonicalRestoreStagedPath(s.cfg, operationID, true), ActivePath: s.cfg.DatabasePath,
			SafetyCopyPath: database.CanonicalRestoreSafetyCopyPath(s.cfg, operationID), OldActivePath: database.CanonicalRestoreOldActivePath(s.cfg, operationID), InstallPath: database.CanonicalRestoreInstallPath(s.cfg, operationID),
			AccountID: accountIDForUser(user), SessionID: sessionID, StatusTokenHash: hashToken(statusToken),
			Phase: database.RestorePhaseValidating, State: database.RestorePhaseValidating, Progress: 5,
			CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), RestoreMaxDatabaseBytes: s.restoreDatabaseLimit(),
			UploadReserved: true, UploadOwnerLockPath: filepath.Join(s.cfg.AppDataDir, "restore", operationID+"-upload.db.owner.lock"), UploadOwnerPID: os.Getpid(), UploadLeaseUntil: now.Add(restoreUploadLease).Format(time.RFC3339Nano),
		}
		// Acquire the kernel-owned artifact lock before publishing the marker.
		// Publishing first creates a recovery race: startup could observe an
		// incomplete reservation, acquire the not-yet-held lock, and delete it
		// while this request is about to stream bytes. A crash before the marker
		// is published leaves only a bounded, harmless lock file for debris
		// cleanup; a marker is never visible without its owner already held.
		var lockErr error
		ownerRelease, lockErr = database.AcquireRestoreArtifactLock(operation.UploadOwnerLockPath)
		if lockErr != nil {
			return lockErr
		}
		runRestoreUploadReservationAfterOwnerLockHook()
		if err := database.WriteRestoreOperation(s.cfg.AppDataDir, operation); err != nil {
			ownerRelease()
			ownerRelease = nil
			_ = os.Remove(operation.UploadOwnerLockPath)
			return err
		}
		return nil
	})
	return operation, statusToken, ownerRelease, err
}

func (s *Server) updateReservedUpload(operation database.RestoreOperation) error {
	return database.WithRestoreOperationLock(s.cfg, func() error {
		s.restoreOperationMu.Lock()
		defer s.restoreOperationMu.Unlock()
		latest, err := database.ReadRestoreOperation(s.cfg.AppDataDir)
		if err != nil {
			return err
		}
		if latest.OperationID != operation.OperationID {
			return errors.New("restore upload reservation was replaced")
		}
		operation.UploadReserved = latest.UploadReserved && !operation.UploadComplete
		operation.UploadOwnerLockPath = latest.UploadOwnerLockPath
		operation.UploadOwnerPID = latest.UploadOwnerPID
		operation.UploadLeaseUntil = latest.UploadLeaseUntil
		return database.WriteRestoreOperation(s.cfg.AppDataDir, operation)
	})
}

func (s *Server) failReservedUpload(operation *database.RestoreOperation, code string) {
	if operation == nil || operation.OperationID == "" {
		return
	}
	_ = database.WithRestoreOperationLock(s.cfg, func() error {
		s.restoreOperationMu.Lock()
		defer s.restoreOperationMu.Unlock()
		latest, err := database.ReadRestoreOperation(s.cfg.AppDataDir)
		if err != nil {
			return err
		}
		if latest.OperationID != operation.OperationID {
			return errors.New("restore upload reservation was replaced")
		}
		latest.Phase, latest.State, latest.Progress = database.RestorePhaseFailed, database.RestorePhaseFailed, 100
		latest.ErrorCode = code
		latest.ErrorMessage = "The supervised restore upload was rejected before the active database was changed."
		latest.UploadReserved = false
		latest.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		*operation = latest
		return database.WriteRestoreOperation(s.cfg.AppDataDir, latest)
	})
	if operation.StagedPath != "" && database.IsAppOwnedPath(filepath.Join(s.cfg.AppDataDir, "restore"), operation.StagedPath) {
		if info, err := os.Lstat(operation.StagedPath); err == nil && info.Mode().IsRegular() {
			_ = os.Remove(operation.StagedPath)
		}
	}
}

func parseUploadedRestoreManifest(body []byte) (database.BackupManifest, error) {
	return database.ParseBackupManifestBytes(body)
}

func (s *Server) ensureRestoreSlot() error {
	return database.WithRestoreOperationLock(s.cfg, func() error {
		s.restoreOperationMu.Lock()
		defer s.restoreOperationMu.Unlock()
		return s.ensureRestoreSlotLocked()
	})
}

func (s *Server) ensureRestoreSlotLocked() error {
	operation, err := database.ReadRestoreOperation(s.cfg.AppDataDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if operation.Phase != database.RestorePhaseComplete && operation.Phase != database.RestorePhaseFailed {
		return errors.New("restore already in progress")
	}
	if operation.CleanupPending {
		return errors.New("restore cleanup is pending")
	}
	if err := database.ArchiveRestoreOperation(s.cfg, operation); err != nil {
		return fmt.Errorf("archive prior restore operation: %w", err)
	}
	return nil
}

func (s *Server) createRestoreOperation(w http.ResponseWriter, r *http.Request, user User, sessionID, name, source string, manifest database.BackupManifest, optional ...any) (RestoreBackupResponse, bool) {
	var response RestoreBackupResponse
	var ok bool
	if err := database.WithRestoreOperationLock(s.cfg, func() error {
		s.restoreOperationMu.Lock()
		defer s.restoreOperationMu.Unlock()
		if err := s.ensureRestoreSlotLocked(); err != nil {
			writeProductError(w, http.StatusConflict, "restore_busy", "Another supervised restore is already in progress.")
			return nil
		}
		response, ok = s.createRestoreOperationLocked(w, r, user, sessionID, name, source, manifest, false, database.MigrationIdentity{}, optional...)
		return nil
	}); err != nil {
		writeProductError(w, http.StatusServiceUnavailable, "restore_storage_unavailable", "Portico could not reserve the supervised restore operation.")
		return RestoreBackupResponse{}, false
	}
	return response, ok
}

func (s *Server) createRestoreOperationLocked(w http.ResponseWriter, r *http.Request, user User, sessionID, name, source string, manifest database.BackupManifest, rawImport bool, importedIdentity database.MigrationIdentity, optional ...any) (RestoreBackupResponse, bool) {
	operationID := ""
	statusToken := ""
	stagedPath := ""
	if len(optional) == 0 {
		operationID = randomID("restore")
		statusToken = randomToken()
		stagedPath = database.CanonicalRestoreStagedPath(s.cfg, operationID, false)
	} else {
		if len(optional) != 3 {
			writeProductError(w, http.StatusInternalServerError, "restore_failed", "Restore operation parameters were invalid.")
			return RestoreBackupResponse{}, false
		}
		stagedPath, _ = optional[0].(string)
		statusToken, _ = optional[1].(string)
		operationID, _ = optional[2].(string)
	}
	if operationID == "" || stagedPath == "" {
		writeProductError(w, http.StatusInternalServerError, "restore_failed", "Restore operation parameters were invalid.")
		return RestoreBackupResponse{}, false
	}
	if source != "" {
		if err := database.CopyRestrictedFileSyncContext(r.Context(), source, stagedPath, s.restoreDatabaseLimit()); err != nil {
			writeProductError(w, http.StatusServiceUnavailable, "restore_storage_unavailable", "The backup could not be staged privately.")
			return RestoreBackupResponse{}, false
		}
		manifestForStage := manifest
		if len(manifestForStage.ArtifactSet) == 1 {
			manifestForStage.ArtifactSet[0].Name = filepath.Base(stagedPath)
		}
		if _, err := database.ValidateRestoreCandidateForBackup(r.Context(), stagedPath, &manifestForStage, manifest.BackupName, s.restoreDatabaseLimit()); err != nil {
			_ = os.Remove(stagedPath)
			writeRestoreValidationError(w, err)
			return RestoreBackupResponse{}, false
		}
		manifest.ArtifactSet = manifestForStage.ArtifactSet
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	operation := database.RestoreOperation{
		Version: database.RestoreOperationVersion, OperationID: operationID, BackupName: name,
		SourcePath: source, StagedPath: stagedPath, ActivePath: s.cfg.DatabasePath,
		SafetyCopyPath: database.CanonicalRestoreSafetyCopyPath(s.cfg, operationID),
		OldActivePath:  database.CanonicalRestoreOldActivePath(s.cfg, operationID),
		InstallPath:    database.CanonicalRestoreInstallPath(s.cfg, operationID),
		AccountID:      accountIDForUser(user), SessionID: sessionID, StatusTokenHash: hashToken(statusToken),
		Phase: database.RestorePhaseValidating, State: database.RestorePhaseValidating, Progress: 10,
		CreatedAt: now, UpdatedAt: now, Manifest: manifest, RawImport: rawImport,
		ImportedIdentity:        importedIdentity,
		RestoreMaxDatabaseBytes: s.restoreDatabaseLimit(),
	}
	if rawImport {
		if validation, validationErr := database.InspectRestoreDatabaseWithLimit(r.Context(), stagedPath, s.restoreDatabaseLimit()); validationErr != nil {
			_ = os.Remove(stagedPath)
			writeRestoreValidationError(w, validationErr)
			return RestoreBackupResponse{}, false
		} else {
			operation.ImportedSizeBytes = validation.SizeBytes
			operation.ImportedChecksumSHA256 = validation.ChecksumSHA256
		}
	}
	if err := database.WriteRestoreOperation(s.cfg.AppDataDir, operation); err != nil {
		_ = os.Remove(stagedPath)
		writeProductError(w, http.StatusServiceUnavailable, "restore_storage_unavailable", "The restore operation could not be durably journaled.")
		return RestoreBackupResponse{}, false
	}
	response := restoreResponse(operation, statusToken)
	if s.restoreRuntime() != nil {
		s.startRestoreRunner(operation.OperationID)
	}
	return response, true
}

func (s *Server) runRestoreOperation(operationID string) {
	releaseExecutor, err := database.AcquireRestoreExecutorLock(s.cfg)
	if err != nil {
		return
	}
	defer releaseExecutor()
	claimant := randomID("executor")
	operation, claimed, err := database.ClaimRestoreOperationWithExecutorLock(s.cfg, operationID, claimant)
	if err != nil || !claimed || operation.OperationID != operationID {
		return
	}
	if err := database.RenewRestoreOperationLease(s.cfg, operationID, claimant); err != nil {
		return
	}
	if operation.Phase == database.RestorePhaseValidating {
		validationCtx, validationCancel := context.WithTimeout(context.Background(), s.restoreIOTimeout())
		if operation.RawImport {
			validation, validationErr := database.InspectRestoreDatabaseWithLimit(validationCtx, operation.StagedPath, s.restoreDatabaseLimit())
			if validationErr != nil {
				validationCancel()
				s.failRestoreOperation(&operation, validationErrorCode(validationErr), validationErr, claimant)
				return
			}
			if !rawImportIdentityMatches(operation, validation) {
				validationCancel()
				s.failRestoreOperation(&operation, "restore_foreign_database", errors.New("raw import identity changed after staging"), claimant)
				return
			}
		} else {
			manifest := operation.Manifest
			if len(manifest.ArtifactSet) == 1 {
				manifest.ArtifactSet[0].Name = filepath.Base(operation.StagedPath)
			}
			if _, validationErr := database.ValidateRestoreCandidateForBackup(validationCtx, operation.StagedPath, &manifest, manifest.BackupName, s.restoreDatabaseLimit()); validationErr != nil {
				validationCancel()
				s.failRestoreOperation(&operation, validationErrorCode(validationErr), validationErr, claimant)
				return
			}
		}
		validationCancel()
		operation.Phase, operation.State, operation.Progress = database.RestorePhaseStaged, database.RestorePhaseStaged, 25
		operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		operation.ExecutorID = claimant
		operation.ExecutorPID = os.Getpid()
		if err := database.WriteRestoreOperationOwned(s.cfg, operation, claimant); err != nil {
			return
		}
	}
	operation.Phase, operation.State, operation.Progress = database.RestorePhaseQuiescing, database.RestorePhaseQuiescing, 35
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	operation.ExecutorID = claimant
	operation.ExecutorPID = os.Getpid()
	if err := database.WriteRestoreOperationOwned(s.cfg, operation, claimant); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), restoreDrainTimeout)
	defer cancel()
	if err := s.quiesceForRestore(ctx); err != nil {
		s.failRestoreOperation(&operation, "restore_quiescence_failed", err, claimant)
		s.restoreBarrier.unblock()
		return
	}
	controller := s.restoreRuntime()
	if controller == nil {
		s.failRestoreOperation(&operation, "restore_runtime_unavailable", errors.New("the host has not attached a runtime generation controller"), claimant)
		s.restoreBarrier.unblock()
		return
	}
	if err := controller(ctx, operation.OperationID); err != nil {
		latest, readErr := database.ReadRestoreOperation(s.cfg.AppDataDir)
		if readErr == nil {
			// Once the host owns the post-quiescence boundary, it is the only
			// authority allowed to classify shutdown/close/install/open/rollback
			// outcomes. In particular, never downgrade recovery-required (or a
			// durable post-quiescence phase awaiting host reconciliation) to a
			// terminal failed marker: restart would then treat an unknown active
			// filesystem as safe to open.
			postQuiescence := latest.State == database.RestoreStateRecoveryNeeded ||
				latest.Phase == database.RestorePhaseQuiescing ||
				latest.Phase == database.RestorePhaseSafetyCopy ||
				latest.Phase == database.RestorePhaseInstalling ||
				latest.Phase == database.RestorePhaseReopening ||
				latest.Phase == database.RestorePhaseHealthChecking ||
				latest.Phase == database.RestorePhaseRollingBack
			if !postQuiescence && latest.Phase != database.RestorePhaseFailed && latest.Phase != database.RestorePhaseComplete {
				s.failRestoreOperation(&latest, "restore_runtime_replacement_failed", err, claimant)
			}
		}
		// A failed host replacement leaves the old generation serving only when
		// the host explicitly returned control; keep the fence fail-closed until
		// that contract is satisfied.
	}
}

// startRestoreRunner owns the restore coordinator goroutine separately from
// generation-owned background work. The runner may be the caller that asks
// the host to shut down its generation, so placing it in the generation's
// WaitGroup would deadlock replacement. Host shutdown closes this admission
// and joins the runner before it detaches or closes the live DB.
func (s *Server) startRestoreRunner(operationID string) bool {
	if s == nil || strings.TrimSpace(operationID) == "" {
		return false
	}
	return s.startRestoreRunnerFunc(func() { s.runRestoreOperation(operationID) })
}

func (s *Server) startRestoreRunnerFunc(run func()) bool {
	if s == nil || run == nil {
		return false
	}
	s.restoreRunnerMu.Lock()
	if s.restoreRunnerClosing {
		s.restoreRunnerMu.Unlock()
		return false
	}
	s.restoreRunnerWG.Add(1)
	s.restoreRunnerMu.Unlock()
	go func() {
		defer s.restoreRunnerWG.Done()
		run()
	}()
	return true
}

// StopRestoreRunners is called by the host before lifecycle replacement or
// process shutdown. It is intentionally not called from runRestoreOperation's
// own controller callback, avoiding self-join while still proving that a
// runner stalled before controller entry cannot outlive DB detachment.
func (s *Server) StopRestoreRunners(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.restoreRunnerMu.Lock()
	s.restoreRunnerClosing = true
	s.restoreRunnerMu.Unlock()
	done := make(chan struct{})
	go func() {
		s.restoreRunnerWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func rawImportIdentityMatches(operation database.RestoreOperation, validation database.RestoreValidation) bool {
	identity := operation.ImportedIdentity
	return operation.RawImport && operation.ImportedSizeBytes == validation.SizeBytes &&
		strings.EqualFold(operation.ImportedChecksumSHA256, validation.ChecksumSHA256) &&
		identity.FormatVersion == validation.Migration.FormatVersion &&
		identity.MigrationHead == validation.Migration.MigrationHead &&
		strings.EqualFold(identity.LedgerSHA256, validation.Migration.LedgerSHA256) &&
		identity.MinimumReader == validation.Migration.MinimumReader
}

func (s *Server) quiesceForRestore(ctx context.Context) error {
	// Seal and cancel admission first, but defer waiting for HTTP leases until
	// their dependent jobs/transcodes/streams have also been canceled. An HLS
	// request may otherwise hold its lease while waiting for a transcode that
	// only restore quiescence can stop.
	s.restoreBarrier.seal()
	s.jobCancelMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.jobCancels))
	for _, cancel := range s.jobCancels {
		cancels = append(cancels, cancel)
	}
	s.jobCancelMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}

	// Snapshot and cancel the transcodes visible at the first seal. A request
	// already inside the admission/start path can publish a session after this
	// snapshot, so the final drain below rescans after all admission leases have
	// returned.
	s.transcodeMu.Lock()
	transcodes := make([]*transcodeSession, 0, len(s.transcodes))
	for _, session := range s.transcodes {
		transcodes = append(transcodes, session)
	}
	s.transcodeMu.Unlock()
	for _, session := range transcodes {
		if session.cancel != nil {
			session.cancel()
		}
	}
	if hook := s.restoreQuiesceAfterInitialCancelHook; hook != nil {
		hook()
	}

	// No admission/start operation is still in flight once this returns. The
	// first cancellation above is what lets an HTTP request release a lease
	// when it was waiting on one of those transcodes.
	if err := s.restoreBarrier.wait(ctx); err != nil {
		return err
	}

	// Re-snapshot every generation-owned registry after the barrier drain. This
	// closes the seal/snapshot race without making the general job-kernel
	// redesign part of R2.
	s.jobCancelMu.Lock()
	finalCancels := make([]context.CancelFunc, 0, len(s.jobCancels))
	for _, cancel := range s.jobCancels {
		finalCancels = append(finalCancels, cancel)
	}
	s.jobCancelMu.Unlock()
	for _, cancel := range finalCancels {
		cancel()
	}
	s.transcodeMu.Lock()
	transcodes = make([]*transcodeSession, 0, len(s.transcodes))
	for _, session := range s.transcodes {
		transcodes = append(transcodes, session)
	}
	s.transcodeMu.Unlock()
	for _, session := range transcodes {
		if session.cancel != nil {
			session.cancel()
		}
	}
	if err := waitForRestoreJobs(ctx, &s.jobCancelMu, s.jobCancels); err != nil {
		return err
	}
	if err := s.waitForTrackedJobRuns(ctx); err != nil {
		return err
	}
	if err := s.requeueOwnedJobsForRestore(ctx); err != nil {
		return err
	}
	for _, session := range transcodes {
		select {
		case <-session.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := waitForRestoreStreams(ctx, s); err != nil {
		return err
	}
	return s.restoreBarrier.wait(ctx)
}

// PrepareRestoreSafetyCopy runs after admission and owned background work have
// drained but before the host closes the old generation's SQLite handle. The
// snapshot is logical (VACUUM INTO), so committed WAL-only data is included;
// the recorded checksum and identity make it the sole rollback authority.
func (s *Server) PrepareRestoreSafetyCopy(ctx context.Context, operationID string) error {
	operation, err := database.ReadRestoreOperation(s.cfg.AppDataDir)
	if err != nil {
		return err
	}
	if operation.OperationID != operationID {
		return errors.New("restore operation changed while preparing safety copy")
	}
	operation.Phase, operation.State, operation.Progress = database.RestorePhaseSafetyCopy, database.RestorePhaseSafetyCopy, 45
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := database.WriteRestoreOperation(s.cfg.AppDataDir, operation); err != nil {
		return err
	}
	validation, err := database.CreateVerifiedDatabaseSnapshot(ctx, s.dbHandle(), operation.SafetyCopyPath)
	if err != nil {
		return err
	}
	operation.SafetyCopySizeBytes = validation.SizeBytes
	operation.SafetyCopyChecksumSHA256 = validation.ChecksumSHA256
	operation.SafetyCopyIdentity = validation.Migration
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return database.WriteRestoreOperation(s.cfg.AppDataDir, operation)
}

// ValidateRuntimeHealth is intentionally separate from activation. A host can
// construct an inert generation, prove its database and application probe, and
// only then start managers or expose its handler.
func (s *Server) ValidateRuntimeHealth(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := database.ValidateOpenDatabaseHealth(ctx, s.dbHandle()); err != nil {
		return err
	}
	return s.probeSQLiteHandle(ctx, s.dbHandle())
}

// DetachRestoreDatabaseHandle transfers ownership of the authoritative live
// SQLite handle to the host coordinator. The watchdog may have swapped s.db
// since construction, so the coordinator must never close a stale pointer.
// Call this only after the generation has joined all owned workers.
func (s *Server) DetachRestoreDatabaseHandle() (*sql.DB, error) {
	if s == nil {
		return nil, errors.New("server generation is unavailable")
	}
	s.dbHandleMu.Lock()
	db := s.db
	s.db = nil
	s.dbHandleMu.Unlock()
	if db == nil {
		return nil, errors.New("server generation database handle is unavailable")
	}
	return db, nil
}

// CompleteRestoreGeneration is the completion owner after a fresh runtime
// generation has opened and migrated the installed database. The host calls
// this before exposing the new handler; no restore response claims completion
// until these checks and the durable complete marker succeed.
func (s *Server) CompleteRestoreGeneration(ctx context.Context, operationID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	operation, err := database.ReadRestoreOperation(s.cfg.AppDataDir)
	if err != nil {
		return err
	}
	if operation.OperationID != operationID {
		return errors.New("restore operation changed before health check")
	}
	if operation.Phase == database.RestorePhaseComplete {
		return nil
	}
	operation.Phase, operation.State, operation.Progress = database.RestorePhaseHealthChecking, database.RestorePhaseHealthChecking, 90
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := database.WriteRestoreOperation(s.cfg.AppDataDir, operation); err != nil {
		return err
	}
	if !operation.RollbackPendingHealth {
		// A requested restore may resurrect credentials that were revoked after
		// the backup was created. Revoke the restored authentication families
		// before the final health gate, complete marker, or handler is published.
		// Rollback skips this step so the unchanged safety database retains its
		// prior session truth.
		if err := s.invalidateRestoredAuthentication(ctx); err != nil {
			return err
		}
	}
	if err := s.ValidateRuntimeHealth(ctx); err != nil {
		return err
	}
	var completionErr error
	if operation.RollbackPendingHealth {
		completionErr = database.CompleteRestoreRollbackOperation(s.cfg, &operation)
	} else {
		completionErr = database.CompleteRestoreOperation(s.cfg, &operation)
	}
	var postCommit *database.RestorePostCommitError
	if errors.As(completionErr, &postCommit) {
		// The complete/failed marker is already durable. Cleanup is retried by
		// startup/retention maintenance and must never cause a healthy runtime
		// to roll back.
		s.log.Warn("restore committed with cleanup pending", "operation", operation.OperationID)
		return nil
	}
	return completionErr
}

// invalidateRestoredAuthentication is deliberately a narrow restore boundary,
// not a new account/device policy. Every credential family that can authorize
// an interactive request or mint a native/ephemeral grant is invalidated in
// the restored database; W2 remains the owner of finer-grained revocation
// policy and cross-origin account coordination.
func (s *Server) invalidateRestoredAuthentication(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := s.withUserTxTagged(ctx, []string{"sessions", "devices", "native_refresh_tokens", "account", "profiles"}, func(tx *sql.Tx) error {
		for _, statement := range []string{
			`DELETE FROM sessions`,
			`UPDATE native_refresh_tokens SET revoked_at = ? WHERE revoked_at = ''`,
			`UPDATE api_keys SET revoked_at = ? WHERE revoked_at = ''`,
			`UPDATE devices SET revoked_at = ? WHERE revoked_at = ''`,
			`UPDATE browser_account_entries SET revoked_at = ? WHERE revoked_at = ''`,
			`UPDATE browser_account_vaults SET revoked_at = ? WHERE revoked_at = ''`,
			`UPDATE automatic_profile_selection_trusts SET revoked_at = ?, updated_at = ? WHERE revoked_at = ''`,
			`DELETE FROM profile_selection_grants`,
			`DELETE FROM playback_media_grants`,
			`DELETE FROM media_download_grants`,
			`DELETE FROM local_profile_admin_proofs`,
			`UPDATE quick_connect_requests SET status = 'expired', updated_at = ? WHERE status IN ('pending', 'approved')`,
			`UPDATE tv_setup_sessions SET status = 'expired', updated_at = ? WHERE status IN ('pending', 'grant_ready')`,
			`UPDATE portico_login_requests SET status = 'expired', updated_at = ? WHERE status = 'pending'`,
		} {
			var args []any
			if strings.Contains(statement, "?") {
				if strings.Contains(statement, "automatic_profile_selection_trusts") {
					args = []any{now, now}
				} else {
					args = []any{now}
				}
			}
			if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		s.invalidateSessionCookieCache()
	}
	return err
}

// ResumeRestoreOperation is used once at host startup for a durable operation
// that had not yet mutated the active database when the prior process died.
func (s *Server) ResumeRestoreOperation() {
	operation, err := database.ReadRestoreOperation(s.cfg.AppDataDir)
	if err != nil || operation.OperationID == "" {
		return
	}
	if operation.UploadReserved && !operation.UploadComplete {
		// The multipart owner is still the only writer of the reserved private
		// artifact. Do not let startup validation race an in-flight upload.
		return
	}
	switch operation.Phase {
	case database.RestorePhaseValidating, database.RestorePhaseStaged, database.RestorePhaseQuiescing:
		s.startRestoreRunner(operation.OperationID)
	}
}

func waitForRestoreJobs(ctx context.Context, mu *sync.Mutex, jobs map[string]context.CancelFunc) error {
	for {
		mu.Lock()
		count := len(jobs)
		mu.Unlock()
		if count == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func waitForRestoreStreams(ctx context.Context, s *Server) error {
	for {
		active := 0
		s.streamMu.Lock()
		for _, count := range s.streamActive {
			active += count
		}
		s.streamMu.Unlock()
		if active == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (s *Server) failRestoreOperation(operation *database.RestoreOperation, code string, err error, claimant ...string) {
	if operation == nil {
		return
	}
	operation.Phase, operation.State, operation.Progress = database.RestorePhaseFailed, database.RestorePhaseFailed, 100
	operation.ErrorCode = code
	if err != nil {
		operation.ErrorMessage = "The supervised restore did not complete."
	}
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if len(claimant) > 0 && strings.TrimSpace(claimant[0]) != "" {
		_ = database.WriteRestoreOperationOwned(s.cfg, *operation, claimant[0])
	} else {
		_ = database.WriteRestoreOperation(s.cfg.AppDataDir, *operation)
	}
	if !operation.ActiveMutationStarted && !operation.ActiveMutationCompleted && operation.State == database.RestorePhaseFailed {
		if cleanupErr := database.DiscardRestoreStagedArtifact(s.cfg, operation); cleanupErr != nil {
			operation.CleanupPending = true
			operation.WarningCode = "restore_cleanup_pending"
			operation.WarningMessage = "The rejected restore left a private cleanup task pending."
			operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if len(claimant) > 0 && strings.TrimSpace(claimant[0]) != "" {
				_ = database.WriteRestoreOperationOwned(s.cfg, *operation, claimant[0])
			} else {
				_ = database.WriteRestoreOperation(s.cfg.AppDataDir, *operation)
			}
		}
	}
}

func validationErrorCode(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "restore_io_timeout"
	}
	var typed *database.RestoreValidationError
	if errors.As(err, &typed) && typed.Code != "" {
		return typed.Code
	}
	return "restore_corrupt_database"
}

func writeRestoreValidationError(w http.ResponseWriter, err error) {
	code := validationErrorCode(err)
	status := http.StatusBadRequest
	if code == "restore_storage_unavailable" {
		status = http.StatusServiceUnavailable
	}
	writeProductError(w, status, code, "The selected backup was rejected before the active database was changed.")
}

func restoreResponse(operation database.RestoreOperation, statusToken string) RestoreBackupResponse {
	instruction := "Restore operation is in progress."
	switch operation.Phase {
	case database.RestorePhaseValidating:
		instruction = "Validating the backup before any active-database mutation."
	case database.RestorePhaseStaged:
		instruction = "Backup is staged for supervised restore; it is not restored yet."
	case database.RestorePhaseQuiescing:
		instruction = "Quiescing requests, streams, and jobs before replacement."
	case database.RestorePhaseSafetyCopy:
		instruction = "Creating the durable pre-restore safety copy."
	case database.RestorePhaseInstalling:
		instruction = "Installing the validated database atomically."
	case database.RestorePhaseReopening:
		instruction = "Reopening and reconciling the restored database."
	case database.RestorePhaseHealthChecking:
		instruction = "Running restored-database integrity and application health checks."
	case database.RestorePhaseComplete:
		instruction = "Restore completed and health checks passed."
	case database.RestorePhaseRollingBack:
		instruction = "Restore failed; rolling back to the safety copy."
	case database.RestorePhaseFailed:
		instruction = "Restore failed; the previous database was retained when rollback succeeded."
	}
	return RestoreBackupResponse{
		OK:               operation.Phase != database.RestorePhaseFailed && operation.State != database.RestoreStateRecoveryNeeded,
		Name:             operation.BackupName,
		OperationID:      operation.OperationID,
		SourceKind:       map[bool]string{true: "raw-import", false: "catalog-backup"}[operation.RawImport],
		ManifestVerified: !operation.RawImport,
		MaxDatabaseBytes: operation.RestoreMaxDatabaseBytes,
		RecoveryRequired: operation.State == database.RestoreStateRecoveryNeeded,
		State:            operation.State,
		Phase:            operation.Phase,
		Progress:         operation.Progress,
		ValidationCode:   operation.ErrorCode,
		ErrorCode:        operation.ErrorCode,
		ErrorMessage:     operation.ErrorMessage,
		WarningCode:      operation.WarningCode,
		WarningMessage:   operation.WarningMessage,
		Instruction:      instruction,
		StatusToken:      statusToken,
	}
}

func (s *Server) restoreStatusResponse(r *http.Request, operationID string, requireOwner bool, user *User) (RestoreBackupResponse, int, bool) {
	operation, err := database.ReadRestoreOperation(s.cfg.AppDataDir)
	if err != nil || operation.OperationID != operationID {
		return RestoreBackupResponse{}, http.StatusNotFound, false
	}
	if subtle.ConstantTimeCompare([]byte(operation.StatusTokenHash), []byte(hashToken(strings.TrimSpace(r.Header.Get(restoreStatusHeader))))) != 1 {
		return RestoreBackupResponse{}, http.StatusUnauthorized, false
	}
	if requireOwner {
		if user == nil || !canInteractivelyManageServer(*user) || accountIDForUser(*user) != operation.AccountID {
			return RestoreBackupResponse{}, http.StatusForbidden, false
		}
	}
	return restoreResponse(operation, ""), http.StatusOK, true
}

// handleRestoreStatusCapability is deliberately registered outside the
// authenticated /api/backups handler. The opaque status capability is the
// only restore route that must survive a generation swap: the restored
// database can remove the initiating account or invalidate its session. No
// other route bypasses normal authentication.
func (s *Server) handleRestoreStatusCapability(w http.ResponseWriter, r *http.Request) {
	// The public subtree also contains the authenticated multipart upload. Keep
	// that one exact path on the normal auth boundary; no other method or path
	// is delegated from the capability responder.
	if r.Method == http.MethodPost && strings.TrimSuffix(r.URL.Path, "/") == "/api/backups/restore/upload" {
		s.withAuth(s.handleBackups)(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for restore status.")
		return
	}
	operationID, ok := restoreStatusOperationID(r.URL.Path)
	if !ok {
		writeProductError(w, http.StatusNotFound, "restore_not_found", "The restore operation was not found.")
		return
	}
	response, status, ok := s.restoreStatusResponse(r, operationID, false, nil)
	if !ok {
		code := "restore_status_unauthorized"
		if status == http.StatusNotFound {
			code = "restore_not_found"
		}
		writeProductError(w, status, code, "The restore operation could not be loaded.")
		return
	}
	writeJSON(w, status, response)
}

// RestoreMaintenanceHandler is used by the host while no Server generation
// owns the database. The API remains fail-closed, but the packaged web shell
// and its assets stay available so a browser can reload and resume polling the
// opaque restore-status capability from sessionStorage.
func (s *Server) RestoreMaintenanceHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applyRestoreMaintenanceHeaders(s, w, r)
		if r.Method == http.MethodOptions && isRestoreStatusPath(r.URL.Path) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" && !s.isAllowedOriginForRequest(origin, r) {
				writeProductError(w, http.StatusForbidden, "origin_not_allowed", "Request origin is not allowed.")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && !isMaintenanceAPIPath(r.URL.Path) {
			if r.URL.Path == "/portico-config.js" || s.maintenanceWebTargetAvailable(r.URL.Path) {
				s.handleStatic(w, r)
				return
			}
			if !isWebAssetRequest(r.URL.Path) {
				writeRestoreMaintenanceShell(w, r)
				return
			}
		}
		if r.Method != http.MethodGet || !isRestoreStatusPath(r.URL.Path) {
			writeProductError(w, http.StatusServiceUnavailable, "restore_in_progress", "Portico is applying a supervised database restore.")
			return
		}
		operationID, valid := restoreStatusOperationID(r.URL.Path)
		if !valid {
			writeProductError(w, http.StatusNotFound, "restore_not_found", "The restore operation was not found.")
			return
		}
		response, status, ok := s.restoreStatusResponse(r, operationID, false, nil)
		if !ok {
			writeProductError(w, status, map[int]string{http.StatusUnauthorized: "restore_status_unauthorized", http.StatusNotFound: "restore_not_found"}[status], "The restore operation could not be loaded.")
			return
		}
		writeJSON(w, status, response)
	})
}

// applyRestoreMaintenanceHeaders is the DB-independent subset of the normal
// transport/security chain. The host swaps the entire handler during a
// generation replacement, so maintenance must not lose browser isolation,
// request correlation, cache truth, or the CORS headers needed by the same
// trusted web origin. It deliberately does not invoke auth, CSRF, workload,
// or database-backed middleware.
func applyRestoreMaintenanceHeaders(s *Server, w http.ResponseWriter, r *http.Request) {
	requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
	if !requestIDPattern.MatchString(requestID) {
		requestID = randomID("req")
	}
	w.Header().Set(requestIDHeader, requestID)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", porticoContentSecurityPolicy)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && s.isAllowedOriginForRequest(origin, r) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Encoding, Range, "+csrfHeaderName+", "+requestIDHeader+", "+profileAdministrationHeader+", "+restoreStatusHeader)
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", requestIDHeader+", Content-Length, Content-Range, Accept-Ranges, Content-Type")
	}
}

func writeRestoreMaintenanceShell(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Portico restore in progress</title></head><body><main><h1>Portico is restoring its database</h1><p>This page will resume status checks when the web application is available.</p></main></body></html>`)
}

func isMaintenanceAPIPath(requestPath string) bool {
	clean := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	return clean == "/api" || strings.HasPrefix(clean, "/api/")
}

func (s *Server) maintenanceWebTargetAvailable(requestPath string) bool {
	if s == nil || strings.TrimSpace(s.cfg.WebDistDir) == "" {
		return false
	}
	if target, ok := webDistTarget(s.cfg.WebDistDir, requestPath); ok {
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			return true
		}
	}
	indexPath := filepath.Join(s.cfg.WebDistDir, "index.html")
	_, err := os.Stat(indexPath)
	return err == nil
}
