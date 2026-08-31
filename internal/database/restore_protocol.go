package database

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

const (
	RestoreManifestFormatVersion = 2
	RestoreOperationVersion      = 1
	RestoreMaxDatabaseBytes      = 16 << 30
	RestoreOperationMarker       = "operation.json"

	RestorePhaseValidating     = "validating"
	RestorePhaseStaged         = "staged"
	RestorePhaseQuiescing      = "quiescing"
	RestorePhaseSafetyCopy     = "safety-copy"
	RestorePhaseInstalling     = "installing"
	RestorePhaseReopening      = "reopening/migrating"
	RestorePhaseHealthChecking = "health-checking"
	RestorePhaseComplete       = "complete"
	RestorePhaseRollingBack    = "rolling-back"
	RestorePhaseFailed         = "failed"
	RestoreStateRecoveryNeeded = "recovery-required"
)

const restoreOperationMaximumBytes = 512 << 10

var restoreRandomCounter uint64

// restoreRandomID is database-owned because restore protocol code cannot use
// the application package's identifier helper. Cryptographic randomness is
// preferred for collision-proof diagnostic/quarantine names; the monotonic
// fallback keeps the operation safe even if the system random source is
// temporarily unavailable.
func restoreRandomID(prefix string) string {
	var bytes [16]byte
	if _, err := cryptorand.Read(bytes[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(bytes[:])
	}
	sequence := atomic.AddUint64(&restoreRandomCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), sequence)
}

// Restore artifact metadata is deliberately explicit. R2 currently installs
// one self-contained SQLite database artifact; WAL/SHM/journal siblings are
// rejected unless a future reviewed artifact contract adds them.
type BackupArtifact struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	SizeBytes      int64  `json:"sizeBytes"`
	ChecksumSHA256 string `json:"checksumSha256"`
}

type BackupManifest struct {
	FormatVersion         int              `json:"formatVersion"`
	Release               string           `json:"release,omitempty"`
	DatabaseFormatVersion int              `json:"databaseFormatVersion,omitempty"`
	MigrationHead         string           `json:"migrationHead,omitempty"`
	MigrationLedgerSHA256 string           `json:"migrationLedgerSha256,omitempty"`
	MigrationLedgerRows   int              `json:"migrationLedgerRows,omitempty"`
	MinimumReader         string           `json:"minimumReader,omitempty"`
	ArtifactSet           []BackupArtifact `json:"artifactSet,omitempty"`
	CreatedAt             string           `json:"createdAt"`
	IntegrityResult       string           `json:"integrityResult,omitempty"`
	ForeignKeyErrors      int              `json:"foreignKeyErrors,omitempty"`
	BackupName            string           `json:"backupName,omitempty"`
	DatabaseBytes         int64            `json:"databaseBytes,omitempty"`
	ChecksumSHA256        string           `json:"checksumSha256,omitempty"`
}

type RestoreValidation struct {
	ManifestPresent  bool
	Manifestless     bool
	Manifest         BackupManifest
	SizeBytes        int64
	ChecksumSHA256   string
	IntegrityResult  string
	ForeignKeyErrors int
	Migration        MigrationIdentity
	MigrationRows    int
}

// RestoreValidationError is the stable product-facing classification. Callers
// must not expose its wrapped filesystem or SQLite detail to clients.
type RestoreValidationError struct {
	Code   string
	Detail string
	Err    error
}

func (e *RestoreValidationError) Error() string {
	if e == nil {
		return "restore validation failed"
	}
	if e.Detail != "" {
		return e.Code + ": " + e.Detail
	}
	return e.Code
}

func (e *RestoreValidationError) Unwrap() error { return e.Err }

func restoreValidationError(code, detail string, err error) error {
	return &RestoreValidationError{Code: code, Detail: detail, Err: err}
}

func RestoreOperationPath(appData string) string {
	return filepath.Join(appData, "restore", RestoreOperationMarker)
}

// CanonicalRestoreStagedPath is the only location used for a normal staged
// database artifact. Upload reservations/raw imports use the separate upload
// name so a marker cannot be rebound from one operation kind to another.
func CanonicalRestoreStagedPath(cfg config.Config, operationID string, upload bool) string {
	if upload {
		return filepath.Join(cfg.AppDataDir, "restore", operationID+"-upload.db")
	}
	return filepath.Join(cfg.AppDataDir, "restore", operationID+".db")
}

// CanonicalRestoreSafetyCopyPath deliberately lives in the already-private
// app-data restore root. DatabasePath may be on an operator-managed/NAS root;
// VACUUM INTO must never create the rollback authority there and expose it
// under inherited external permissions while the snapshot is in progress.
func CanonicalRestoreSafetyCopyPath(cfg config.Config, operationID string) string {
	return filepath.Join(cfg.AppDataDir, "restore", operationID+".pre-restore.db")
}

func CanonicalRestoreOldActivePath(cfg config.Config, operationID string) string {
	return filepath.Clean(cfg.DatabasePath) + ".restore-old-" + operationID
}

func CanonicalRestoreInstallPath(cfg config.Config, operationID string) string {
	return filepath.Clean(cfg.DatabasePath) + ".restore-install-" + operationID
}

// WithRestoreOperationLock is the cross-process admission authority for the
// single restore marker. The in-memory app mutex remains useful for local
// ordering, but cannot protect two Server generations or two Portico
// processes. The lock spans slot inspection, staging reservation, and the
// first durable operation marker publication.
func WithRestoreOperationLock(cfg config.Config, fn func() error) error {
	if fn == nil {
		return errors.New("restore operation lock callback is required")
	}
	if err := preparePrivateDataPaths(cfg); err != nil {
		return err
	}
	release, err := acquirePrivateFileLock(filepath.Join(cfg.AppDataDir, "restore", "admission.lock"))
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

// AcquireRestoreArtifactLock is a small cross-platform advisory lock for
// private artifact ownership (backup reservations and similar durable
// debris). The lock is kernel-owned and therefore released on process death;
// callers must retain the release function for the artifact's lifetime.
func AcquireRestoreArtifactLock(path string) (func(), error) {
	return acquirePrivateFileLock(path)
}

func TryAcquireRestoreArtifactLock(path string) (func(), bool, error) {
	return tryAcquirePrivateFileLock(path)
}

// RestoreUploadOwnerLockPath returns the durable per-upload lock name. The
// fallback keeps markers written before this field existed recoverable while
// making the lock path deterministic from the private staged artifact.
func RestoreUploadOwnerLockPath(operation RestoreOperation) string {
	if value := strings.TrimSpace(operation.UploadOwnerLockPath); value != "" {
		return value
	}
	if staged := strings.TrimSpace(operation.StagedPath); staged != "" {
		return staged + ".owner.lock"
	}
	return ""
}

// AcquireRestoreExecutorLock holds the OS advisory restore-executor lock for
// the complete operation. Its lifetime is the mutual-exclusion authority:
// process death releases the lock in the kernel. Durable executor fields are
// only recovery/status evidence and are never used as a substitute for this
// lock (PID namespaces and PID reuse make liveness probes insufficient).
func AcquireRestoreExecutorLock(cfg config.Config) (func(), error) {
	if err := preparePrivateDataPaths(cfg); err != nil {
		return nil, err
	}
	return acquirePrivateFileLock(filepath.Join(cfg.AppDataDir, "restore", "executor.lock"))
}

// TryAcquireRestoreExecutorLock is used only during host startup. A process
// starting while another host owns recovery must expose the capability-bound
// maintenance responder immediately instead of blocking before its listener
// exists. acquired=false is not an error: the other process owns the durable
// recovery transition.
func TryAcquireRestoreExecutorLock(cfg config.Config) (release func(), acquired bool, err error) {
	if err := preparePrivateDataPaths(cfg); err != nil {
		return nil, false, err
	}
	return tryAcquirePrivateFileLock(filepath.Join(cfg.AppDataDir, "restore", "executor.lock"))
}

// RestoreOperation is private durable state. Filesystem paths are persisted
// here for crash recovery but are never copied into the HTTP response types.
type RestoreOperation struct {
	Version                  int               `json:"version"`
	OperationID              string            `json:"operationId"`
	BackupName               string            `json:"backupName"`
	SourcePath               string            `json:"sourcePath"`
	StagedPath               string            `json:"stagedPath"`
	ActivePath               string            `json:"activePath"`
	SafetyCopyPath           string            `json:"safetyCopyPath"`
	SafetyCopySizeBytes      int64             `json:"safetyCopySizeBytes,omitempty"`
	SafetyCopyChecksumSHA256 string            `json:"safetyCopyChecksumSha256,omitempty"`
	SafetyCopyIdentity       MigrationIdentity `json:"safetyCopyIdentity,omitempty"`
	ActiveMutationStarted    bool              `json:"activeMutationStarted,omitempty"`
	ActiveMutationCompleted  bool              `json:"activeMutationCompleted,omitempty"`
	OldActivePath            string            `json:"oldActivePath"`
	InstallPath              string            `json:"installPath"`
	AccountID                string            `json:"accountId"`
	SessionID                string            `json:"sessionId"`
	StatusTokenHash          string            `json:"statusTokenHash"`
	PreRestoreSecurityEpoch  int64             `json:"preRestoreSecurityEpoch,omitempty"`
	HostedAuthorizationID    string            `json:"hostedAuthorizationId,omitempty"`
	AuthorizationCommitted   bool              `json:"authorizationCommitted"`
	SecurityFenceApplied     bool              `json:"securityFenceApplied,omitempty"`
	AppliedSecurityEpoch     int64             `json:"appliedSecurityEpoch,omitempty"`
	RawImport                bool              `json:"rawImport,omitempty"`
	UploadReserved           bool              `json:"uploadReserved,omitempty"`
	UploadOwnerLockPath      string            `json:"uploadOwnerLockPath,omitempty"`
	UploadOwnerPID           int               `json:"uploadOwnerPid,omitempty"`
	UploadLeaseUntil         string            `json:"uploadLeaseUntil,omitempty"`
	UploadBytes              int64             `json:"uploadBytes,omitempty"`
	UploadComplete           bool              `json:"uploadComplete,omitempty"`
	ExecutorID               string            `json:"executorId,omitempty"`
	ExecutorPID              int               `json:"executorPid,omitempty"`
	ExecutorLeaseUntil       string            `json:"executorLeaseUntil,omitempty"`
	ImportedIdentity         MigrationIdentity `json:"importedIdentity,omitempty"`
	ImportedSizeBytes        int64             `json:"importedSizeBytes,omitempty"`
	ImportedChecksumSHA256   string            `json:"importedChecksumSha256,omitempty"`
	RestoreMaxDatabaseBytes  int64             `json:"restoreMaxDatabaseBytes,omitempty"`
	Phase                    string            `json:"phase"`
	State                    string            `json:"state"`
	Progress                 int               `json:"progress"`
	ErrorCode                string            `json:"errorCode,omitempty"`
	ErrorMessage             string            `json:"errorMessage,omitempty"`
	WarningCode              string            `json:"warningCode,omitempty"`
	WarningMessage           string            `json:"warningMessage,omitempty"`
	RollbackPendingHealth    bool              `json:"rollbackPendingHealth,omitempty"`
	CleanupPending           bool              `json:"cleanupPending,omitempty"`
	CreatedAt                string            `json:"createdAt"`
	UpdatedAt                string            `json:"updatedAt"`
	CompletedAt              string            `json:"completedAt,omitempty"`
	Manifest                 BackupManifest    `json:"manifest,omitempty"`
}

const restoreExecutorLease = 2 * time.Minute

// ClaimRestoreOperation is the cross-process executor lease. The durable
// marker reservation prevents two generations from advancing one operation;
// takeover requires both an expired lease and a dead prior owner, so a slow
// live executor is never silently superseded by a second process.
func ClaimRestoreOperation(cfg config.Config, operationID, claimant string) (RestoreOperation, bool, error) {
	return claimRestoreOperation(cfg, operationID, claimant, false)
}

// ClaimRestoreOperationWithExecutorLock is called only after the caller has
// acquired AcquireRestoreExecutorLock. The lock proves that any prior
// claimant is gone, so a crash can be taken over immediately without trusting
// a PID or namespace-local process table.
func ClaimRestoreOperationWithExecutorLock(cfg config.Config, operationID, claimant string) (RestoreOperation, bool, error) {
	return claimRestoreOperation(cfg, operationID, claimant, true)
}

func claimRestoreOperation(cfg config.Config, operationID, claimant string, executorLockHeld bool) (RestoreOperation, bool, error) {
	var operation RestoreOperation
	claimant = strings.TrimSpace(claimant)
	if !validRestoreOperationID(operationID) || !validRestoreOperationID(claimant) {
		return operation, false, errors.New("restore executor claim is invalid")
	}
	err := WithRestoreOperationLock(cfg, func() error {
		current, err := ReadRestoreOperation(cfg.AppDataDir)
		if err != nil {
			return err
		}
		if current.OperationID != operationID {
			return errors.New("restore operation changed before executor claim")
		}
		if current.Phase == RestorePhaseComplete || current.Phase == RestorePhaseFailed || current.State == RestoreStateRecoveryNeeded {
			operation = current
			return nil
		}
		if !current.AuthorizationCommitted {
			// The filesystem marker is deliberately published before the SQLite
			// credential/receipt transaction. A false value is an incomplete
			// cross-medium commit, never authority to execute a restore.
			operation = current
			return nil
		}
		if !restoreExecutorResumable(current) {
			// Host-owned replacement phases are intentionally not transferable to
			// the app goroutine. Filesystem reconciliation or the host coordinator
			// must decide whether they resume or roll back.
			operation = current
			return nil
		}
		now := time.Now().UTC()
		if current.ExecutorID != "" && current.ExecutorID != claimant && !executorLockHeld {
			leaseUntil, parseErr := time.Parse(time.RFC3339Nano, current.ExecutorLeaseUntil)
			if parseErr != nil || now.Before(leaseUntil) || restoreProcessAlive(current.ExecutorPID) {
				operation = current
				return nil
			}
		}
		current.ExecutorID = claimant
		current.ExecutorPID = os.Getpid()
		current.ExecutorLeaseUntil = now.Add(restoreExecutorLease).Format(time.RFC3339Nano)
		current.UpdatedAt = now.Format(time.RFC3339Nano)
		if err := WriteRestoreOperation(cfg.AppDataDir, current); err != nil {
			return err
		}
		operation = current
		return nil
	})
	if err != nil {
		return RestoreOperation{}, false, err
	}
	return operation, operation.ExecutorID == claimant, nil
}

func restoreExecutorResumable(operation RestoreOperation) bool {
	if !operation.AuthorizationCommitted || operation.State != operation.Phase {
		return false
	}
	switch operation.Phase {
	case RestorePhaseValidating, RestorePhaseStaged, RestorePhaseQuiescing:
		return true
	default:
		return false
	}
}

func RenewRestoreOperationLease(cfg config.Config, operationID, claimant string) error {
	return WithRestoreOperationLock(cfg, func() error {
		operation, err := ReadRestoreOperation(cfg.AppDataDir)
		if err != nil {
			return err
		}
		if operation.OperationID != operationID || operation.ExecutorID != claimant {
			return errors.New("restore executor lease is not owned")
		}
		operation.ExecutorLeaseUntil = time.Now().UTC().Add(restoreExecutorLease).Format(time.RFC3339Nano)
		operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return WriteRestoreOperation(cfg.AppDataDir, operation)
	})
}

// RestorePostCommitError communicates that the complete marker is already
// durable. Callers must not roll back after receiving this error; cleanup is a
// retryable maintenance task and the restored database is authoritative.
type RestorePostCommitError struct {
	Err error
}

func (e *RestorePostCommitError) Error() string {
	if e == nil || e.Err == nil {
		return "restore committed; transient cleanup is pending"
	}
	return "restore committed; transient cleanup is pending: " + e.Err.Error()
}

func (e *RestorePostCommitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func restorePhaseAllowed(phase string) bool {
	switch phase {
	case "", RestorePhaseValidating, RestorePhaseStaged, RestorePhaseQuiescing,
		RestorePhaseSafetyCopy, RestorePhaseInstalling, RestorePhaseReopening,
		RestorePhaseHealthChecking, RestorePhaseComplete, RestorePhaseRollingBack,
		RestorePhaseFailed:
		return true
	default:
		return false
	}
}

func restoreSafeMessage(code string) string {
	switch strings.TrimSpace(code) {
	case "restore_recovery_required":
		return "The supervised restore requires recovery before Portico can resume service."
	case "restore_cleanup_pending":
		return "The restore completed; private transient cleanup will be retried."
	case "restore_quiescence_interrupted":
		return "The restore stopped before its safety copy was durable."
	case "restore_install_failed", "restore_restart_recovery_failed", "restore_runtime_replacement_failed":
		return "The supervised restore failed and Portico is recovering the previous database."
	default:
		return "The supervised restore did not complete."
	}
}

// ReadBackupManifest reads the mandatory sidecar without following a selected
// backup's final-component symlink.
func ReadBackupManifest(databasePath string) (BackupManifest, error) {
	manifestPath := databasePath + ".manifest.json"
	if err := requireRegularNonSymlinkFile(manifestPath); err != nil {
		return BackupManifest{}, err
	}
	if info, err := os.Stat(manifestPath); err != nil {
		return BackupManifest{}, err
	} else if info.Size() > 128<<10 {
		return BackupManifest{}, errors.New("backup manifest exceeds the bounded parser size")
	}
	file, err := openRegularFileForRead(manifestPath)
	if err != nil {
		return BackupManifest{}, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 128<<10+1))
	if err != nil {
		return BackupManifest{}, err
	}
	if len(body) > 128<<10 {
		return BackupManifest{}, errors.New("backup manifest exceeds the bounded parser size")
	}
	return ParseBackupManifestBytes(body)
}

// ParseBackupManifestBytes applies the same bounded strict parser to a
// multipart manifest without creating a temporary file outside private
// restore staging.
func ParseBackupManifestBytes(body []byte) (BackupManifest, error) {
	if len(body) == 0 || len(body) > 128<<10 {
		return BackupManifest{}, errors.New("backup manifest exceeds the bounded parser size")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest BackupManifest
	if err := decoder.Decode(&manifest); err != nil {
		return BackupManifest{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return BackupManifest{}, errors.New("backup manifest contains trailing data")
	}
	return manifest, nil
}

func FileSHA256(path string) (string, error) {
	return FileSHA256Context(context.Background(), path)
}

// FileSHA256Context is the cooperative, bounded-lifecycle checksum primitive
// used by restore admission and rollback validation. A filesystem read that
// is already inside an uninterruptible kernel call cannot be preempted by Go;
// callers must therefore keep the host in maintenance until this function
// returns rather than starting a compensating mutation concurrently.
func FileSHA256Context(ctx context.Context, path string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := openRegularFileForRead(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, &restoreContextReader{ctx: ctx, reader: file}); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type restoreContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *restoreContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func readOnlySQLiteDSN(path string) string {
	values := url.Values{}
	values.Set("mode", "ro")
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "foreign_keys(ON)")
	values.Add("_pragma", "query_only(ON)")
	return path + "?" + values.Encode()
}

// ValidateRestoreCandidate performs every read-only check before the active
// database is quiesced or mutated. It intentionally refuses sidecars: a
// database copied without its WAL is not a truthful backup source.
func ValidateRestoreCandidate(ctx context.Context, path string, manifest *BackupManifest) (RestoreValidation, error) {
	return ValidateRestoreCandidateWithLimit(ctx, path, manifest, RestoreMaxDatabaseBytes)
}

func ValidateRestoreCandidateWithLimit(ctx context.Context, path string, manifest *BackupManifest, maximum int64) (RestoreValidation, error) {
	return validateRestoreCandidate(ctx, path, manifest, true, maximum, "")
}

// ValidateRestoreCandidateForBackup validates a staged copy whose basename is
// intentionally different from the catalogued backup. The original
// BackupName remains in the manifest and is checked against expectedName; the
// artifact name is checked against the immutable staged file separately.
func ValidateRestoreCandidateForBackup(ctx context.Context, path string, manifest *BackupManifest, expectedName string, maximum int64) (RestoreValidation, error) {
	return validateRestoreCandidate(ctx, path, manifest, true, maximum, expectedName)
}

// InspectRestoreDatabase is used only while publishing a new backup: the
// manifest does not exist until the database has been copied and inspected.
// Restore admission itself must call ValidateRestoreCandidate and therefore
// requires the canonical manifest.
func InspectRestoreDatabase(ctx context.Context, path string) (RestoreValidation, error) {
	return InspectRestoreDatabaseWithLimit(ctx, path, RestoreMaxDatabaseBytes)
}

func InspectRestoreDatabaseWithLimit(ctx context.Context, path string, maximum int64) (RestoreValidation, error) {
	return validateRestoreCandidate(ctx, path, nil, false, maximum, "")
}

func validateRestoreCandidate(ctx context.Context, path string, manifest *BackupManifest, requireManifest bool, maximum int64, expectedBackupName string) (RestoreValidation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return RestoreValidation{}, err
	}
	if err := requireRegularNonSymlinkFile(path); err != nil {
		return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "the selected database is not a regular file", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return RestoreValidation{}, contextErr
		}
		return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "the selected database could not be inspected", err)
	}
	if maximum <= 0 {
		maximum = RestoreMaxDatabaseBytes
	}
	if info.Size() <= 0 || info.Size() > maximum {
		return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "the selected database size is invalid", nil)
	}
	if err := validateSQLiteHeaderContext(ctx, path); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return RestoreValidation{}, err
		}
		return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "the selected file is not a SQLite database", err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(path + suffix); err == nil {
			return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "the selected database has an untracked SQLite sidecar", nil)
		} else if !os.IsNotExist(err) {
			return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "the selected database sidecar could not be inspected", err)
		}
	}
	checksum, err := FileSHA256Context(ctx, path)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return RestoreValidation{}, err
		}
		return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "the selected database checksum could not be calculated", err)
	}

	db, err := sql.Open("sqlite", readOnlySQLiteDSN(path))
	if err != nil {
		return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "the selected database could not be opened read-only", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer db.Close()
	var quickCheck string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&quickCheck); err != nil || !strings.EqualFold(strings.TrimSpace(quickCheck), "ok") {
		if contextErr := ctx.Err(); contextErr != nil {
			return RestoreValidation{}, contextErr
		}
		return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "SQLite quick-check validation failed", err)
	}
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || !strings.EqualFold(strings.TrimSpace(integrity), "ok") {
		if contextErr := ctx.Err(); contextErr != nil {
			return RestoreValidation{}, contextErr
		}
		return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "SQLite integrity validation failed", err)
	}
	foreignKeyErrors, err := countForeignKeyErrors(ctx, db)
	if err != nil || foreignKeyErrors != 0 {
		if contextErr := ctx.Err(); contextErr != nil {
			return RestoreValidation{}, contextErr
		}
		return RestoreValidation{}, restoreValidationError("restore_corrupt_database", "SQLite foreign-key validation failed", err)
	}
	identity, rows, err := ReadMigrationIdentity(ctx, db)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return RestoreValidation{}, contextErr
		}
		return RestoreValidation{}, classifyMigrationValidationError(err)
	}
	result := RestoreValidation{
		SizeBytes: info.Size(), ChecksumSHA256: checksum, IntegrityResult: "ok",
		ForeignKeyErrors: foreignKeyErrors, Migration: identity, MigrationRows: rows,
	}
	if manifest != nil {
		result.ManifestPresent = true
		result.Manifest = *manifest
		if err := validateRestoreManifest(*manifest, path, result, expectedBackupName); err != nil {
			return RestoreValidation{}, err
		}
	} else if requireManifest {
		return RestoreValidation{}, restoreValidationError("restore_unidentified_database", "a complete backup manifest is required", nil)
	} else {
		result.Manifestless = true
	}
	return result, nil
}

func countForeignKeyErrors(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	return count, rows.Err()
}

func validateSQLiteHeader(path string) error {
	return validateSQLiteHeaderContext(context.Background(), path)
}

func validateSQLiteHeaderContext(ctx context.Context, path string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := openRegularFileForRead(path)
	if err != nil {
		return err
	}
	defer file.Close()
	header := make([]byte, 16)
	if _, err := io.ReadFull(&restoreContextReader{ctx: ctx, reader: file}, header); err != nil {
		return err
	}
	if string(header) != "SQLite format 3\x00" {
		return errors.New("SQLite header is not the reviewed format")
	}
	return nil
}

func classifyMigrationValidationError(err error) error {
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "newer"), strings.Contains(lower, "format") && strings.Contains(lower, "binary"), strings.Contains(lower, "embedded head"):
		return restoreValidationError("restore_forward_incompatible_database", "the selected database is newer than this release", err)
	case strings.Contains(lower, "missing"), strings.Contains(lower, "no such table"), strings.Contains(lower, "reconciliation") && strings.Contains(lower, "incomplete"):
		return restoreValidationError("restore_unidentified_database", "the selected database has no complete Portico release identity", err)
	default:
		return restoreValidationError("restore_foreign_database", "the selected database is not the reviewed Portico migration lineage", err)
	}
}

func validateRestoreManifest(manifest BackupManifest, databasePath string, validation RestoreValidation, expectedBackupName string) error {
	if manifest.FormatVersion > RestoreManifestFormatVersion {
		return restoreValidationError("restore_forward_incompatible_database", "the backup manifest is newer than this release", nil)
	}
	if manifest.FormatVersion != RestoreManifestFormatVersion {
		return restoreValidationError("restore_corrupt_database", "the backup manifest format is invalid", nil)
	}
	if strings.TrimSpace(manifest.BackupName) == "" {
		return restoreValidationError("restore_unidentified_database", "the backup manifest has no exact backup name", nil)
	}
	if !safeBackupName(manifest.BackupName) {
		return restoreValidationError("restore_foreign_database", "the backup manifest name is not trusted", nil)
	}
	if expected := strings.TrimSpace(expectedBackupName); expected != "" {
		if manifest.BackupName != expected {
			return restoreValidationError("restore_foreign_database", "the backup manifest name does not identify the selected backup", nil)
		}
	} else if manifest.BackupName != filepath.Base(databasePath) {
		return restoreValidationError("restore_foreign_database", "the backup manifest name does not identify the selected database", nil)
	}
	created, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(manifest.CreatedAt))
	if err != nil || created.IsZero() || !strings.HasSuffix(manifest.CreatedAt, "Z") || created.After(time.Now().UTC().Add(24*time.Hour)) || created.Year() < 2000 || created.UTC().Format(time.RFC3339Nano) != manifest.CreatedAt {
		return restoreValidationError("restore_corrupt_database", "the backup manifest creation time is not a sane RFC3339Nano UTC value", err)
	}
	if manifest.DatabaseBytes <= 0 || manifest.DatabaseBytes != validation.SizeBytes {
		return restoreValidationError("restore_corrupt_database", "the backup manifest size does not match the database", nil)
	}
	if len(strings.TrimSpace(manifest.ChecksumSHA256)) != sha256.Size*2 || !strings.EqualFold(manifest.ChecksumSHA256, validation.ChecksumSHA256) {
		return restoreValidationError("restore_corrupt_database", "the backup manifest checksum does not match the database", nil)
	}
	if !strings.EqualFold(manifest.IntegrityResult, "ok") {
		return restoreValidationError("restore_corrupt_database", "the backup manifest does not report verified integrity", nil)
	}
	if manifest.ForeignKeyErrors != 0 {
		return restoreValidationError("restore_corrupt_database", "the backup manifest reports foreign-key errors", nil)
	}
	if strings.TrimSpace(manifest.Release) == "" {
		return restoreValidationError("restore_unidentified_database", "the backup manifest has no creating-release provenance", nil)
	}
	if manifest.DatabaseFormatVersion <= 0 {
		return restoreValidationError("restore_unidentified_database", "the backup manifest has no database format identity", nil)
	}
	if manifest.DatabaseFormatVersion > validation.Migration.FormatVersion {
		return restoreValidationError("restore_forward_incompatible_database", "the backup database format is newer than this release", nil)
	}
	if manifest.DatabaseFormatVersion != validation.Migration.FormatVersion {
		return restoreValidationError("restore_foreign_database", "the backup database format is not the reviewed format", nil)
	}
	if strings.TrimSpace(manifest.MinimumReader) == "" {
		return restoreValidationError("restore_unidentified_database", "the backup manifest has no minimum-reader identity", nil)
	}
	if !minimumReaderCompatible(manifest.MinimumReader, validation.Migration.MinimumReader) {
		if minimumReaderIsNewer(manifest.MinimumReader, validation.Migration.MinimumReader) {
			return restoreValidationError("restore_forward_incompatible_database", "the backup requires a newer database reader", nil)
		}
		return restoreValidationError("restore_foreign_database", "the backup minimum-reader identity is invalid", nil)
	}
	if strings.TrimSpace(manifest.MigrationHead) == "" || manifest.MigrationHead != validation.Migration.MigrationHead {
		return restoreValidationError("restore_foreign_database", "the backup migration head does not match its identity", nil)
	}
	if strings.TrimSpace(manifest.MigrationLedgerSHA256) == "" || !strings.EqualFold(manifest.MigrationLedgerSHA256, validation.Migration.LedgerSHA256) {
		return restoreValidationError("restore_foreign_database", "the backup migration ledger does not match its identity", nil)
	}
	if manifest.MigrationLedgerRows <= 0 || manifest.MigrationLedgerRows != validation.MigrationRows {
		return restoreValidationError("restore_foreign_database", "the backup migration row count does not match its identity", nil)
	}
	if len(manifest.ArtifactSet) != 1 {
		return restoreValidationError("restore_foreign_database", "the backup artifact set is not the reviewed self-contained database set", nil)
	}
	artifact := manifest.ArtifactSet[0]
	if artifact.Kind != "database" || filepath.Base(databasePath) != artifact.Name || artifact.SizeBytes != validation.SizeBytes || !strings.EqualFold(artifact.ChecksumSHA256, validation.ChecksumSHA256) {
		return restoreValidationError("restore_corrupt_database", "the backup artifact set does not match the selected database", nil)
	}
	return nil
}

func minimumReaderIsNewer(candidate, supported string) bool {
	candidateNumber, candidateErr := strconv.Atoi(strings.TrimSpace(candidate))
	supportedNumber, supportedErr := strconv.Atoi(strings.TrimSpace(supported))
	return candidateErr == nil && supportedErr == nil && candidateNumber > supportedNumber
}

func minimumReaderCompatible(candidate, supported string) bool {
	candidateNumber, candidateErr := strconv.Atoi(strings.TrimSpace(candidate))
	supportedNumber, supportedErr := strconv.Atoi(strings.TrimSpace(supported))
	return candidateErr == nil && supportedErr == nil && candidateNumber > 0 && supportedNumber > 0 && candidateNumber <= supportedNumber
}

// WritePrivateStream enforces the upload bound while streaming directly into
// a private no-follow file. It never trusts multipart filenames and fsyncs both
// the file and its containing directory before returning success.
func WritePrivateStream(path string, source io.Reader, maximum int64) (int64, error) {
	return WritePrivateStreamContext(context.Background(), path, source, maximum)
}

// WritePrivateStreamContext is the request-cancellable upload primitive. It
// checks cancellation between every bounded read/write and removes/syncs a
// partial private artifact before returning an error.
func WritePrivateStreamContext(ctx context.Context, path string, source io.Reader, maximum int64) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if maximum <= 0 {
		return 0, errors.New("maximum stream size must be positive")
	}
	if source == nil {
		return 0, errors.New("stream source is required")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := preparePrivateFileForCreate(path); err != nil {
		return 0, err
	}
	file, err := openPrivateFileForWrite(path)
	if err != nil {
		return 0, err
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = cleanupPartialRestoreFile(path)
		}
	}()
	count, err := io.Copy(file, io.LimitReader(&restoreContextReader{ctx: ctx, reader: source}, maximum+1))
	if err != nil {
		return count, err
	}
	if count > maximum {
		return count, fmt.Errorf("stream exceeds maximum size of %d bytes", maximum)
	}
	if err := ctx.Err(); err != nil {
		return count, err
	}
	if err := file.Sync(); err != nil {
		return count, err
	}
	if err := file.Close(); err != nil {
		return count, err
	}
	if err := enforcePrivateExistingFileNoParent(path); err != nil {
		return count, err
	}
	if err := ctx.Err(); err != nil {
		return count, err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return count, err
	}
	remove = false
	return count, nil
}

// SyncFile and WriteAtomicPrivateFile are the public lifecycle primitives used
// by backup publication and the supervised restore journal.
func SyncFile(path string) error {
	return SyncFileContext(context.Background(), path)
}

func SyncFileContext(ctx context.Context, path string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := openRegularFileForRead(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return err
	}
	return ctx.Err()
}

func SyncDirectory(path string) error { return syncDirectory(path) }

func syncDirectoryContext(ctx context.Context, path string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := syncDirectory(path); err != nil {
		return err
	}
	return ctx.Err()
}

func WriteAtomicPrivateFile(path string, body []byte) error {
	return writeAtomicPrivateFile(path, body)
}

// CreateVerifiedDatabaseSnapshot uses SQLite's logical VACUUM INTO snapshot
// while the source handle is still open. A raw main-file copy is not
// sufficient when committed data still resides in the source WAL.
func CreateVerifiedDatabaseSnapshot(ctx context.Context, db *sql.DB, target string) (RestoreValidation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if db == nil {
		return RestoreValidation{}, errors.New("database handle is required for a logical snapshot")
	}
	if err := preparePrivateFileForCreate(target); err != nil {
		return RestoreValidation{}, err
	}
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, target); err != nil {
		_ = cleanupPartialRestoreFile(target)
		return RestoreValidation{}, err
	}
	if err := enforcePrivateExistingFileNoParent(target); err != nil {
		_ = cleanupPartialRestoreFile(target)
		return RestoreValidation{}, err
	}
	if err := SyncFileContext(ctx, target); err != nil {
		_ = cleanupPartialRestoreFile(target)
		return RestoreValidation{}, err
	}
	if err := syncDirectoryContext(ctx, filepath.Dir(target)); err != nil {
		_ = cleanupPartialRestoreFile(target)
		return RestoreValidation{}, err
	}
	inspection, err := InspectRestoreDatabase(ctx, target)
	if err != nil {
		_ = cleanupPartialRestoreFile(target)
		return RestoreValidation{}, err
	}
	return inspection, nil
}

func cleanupPartialRestoreFile(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("partial restore artifact is not a regular file")
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

// ValidateOpenDatabaseHealth is the handle-level counterpart to candidate
// validation. It permits SQLite's own WAL recovery, then proves the reopened
// handle has the reviewed identity, integrity, and empty foreign-key check.
func ValidateOpenDatabaseHealth(ctx context.Context, db *sql.DB) (RestoreValidation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if db == nil {
		return RestoreValidation{}, errors.New("database handle is required")
	}
	var quickCheck, integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&quickCheck); err != nil || !strings.EqualFold(strings.TrimSpace(quickCheck), "ok") {
		return RestoreValidation{}, fmt.Errorf("SQLite quick-check failed: %w", err)
	}
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || !strings.EqualFold(strings.TrimSpace(integrity), "ok") {
		return RestoreValidation{}, fmt.Errorf("SQLite integrity check failed: %w", err)
	}
	foreignKeyErrors, err := countForeignKeyErrors(ctx, db)
	if err != nil {
		return RestoreValidation{}, err
	}
	if foreignKeyErrors != 0 {
		return RestoreValidation{}, fmt.Errorf("SQLite foreign-key check returned %d errors", foreignKeyErrors)
	}
	identity, rows, err := ReadMigrationIdentity(ctx, db)
	if err != nil {
		return RestoreValidation{}, err
	}
	return RestoreValidation{IntegrityResult: "ok", ForeignKeyErrors: 0, Migration: identity, MigrationRows: rows}, nil
}

// CopyRestrictedFileSync is the bounded, durable copy primitive used for
// staged uploads, same-volume safety copies, and replacement temp files.
func CopyRestrictedFileSync(sourcePath, targetPath string, maximum int64) error {
	return CopyRestrictedFileSyncContext(context.Background(), sourcePath, targetPath, maximum)
}

// CopyRestrictedFileSyncContext copies in bounded chunks and checks the
// lifecycle context between I/O operations. It never reports a partial file
// as durable and removes/syncs the private target on cancellation or ENOSPC.
func CopyRestrictedFileSyncContext(ctx context.Context, sourcePath, targetPath string, maximum int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if maximum <= 0 {
		maximum = RestoreMaxDatabaseBytes
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := requireRegularNonSymlinkFile(sourcePath); err != nil {
		return err
	}
	if err := preparePrivateFileForCreate(targetPath); err != nil {
		return err
	}
	if _, err := os.Lstat(targetPath); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	source, err := openRegularFileForRead(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := openPrivateFileForWrite(targetPath)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		_ = target.Close()
		if remove {
			_ = cleanupPartialRestoreFile(targetPath)
		}
	}()
	count, err := io.Copy(target, io.LimitReader(&restoreContextReader{ctx: ctx, reader: source}, maximum+1))
	if err != nil {
		return err
	}
	if count > maximum {
		return fmt.Errorf("file exceeds maximum size of %d bytes", maximum)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := target.Sync(); err != nil {
		return err
	}
	if err := target.Close(); err != nil {
		return err
	}
	if err := enforcePrivateExistingFileNoParent(targetPath); err != nil {
		return err
	}
	if err := syncDirectoryContext(ctx, filepath.Dir(targetPath)); err != nil {
		return err
	}
	remove = false
	return nil
}

func writeAtomicPrivateFile(path string, body []byte) error {
	if err := validatePrivateFileParent(path); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("atomic private file target is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	var tmp string
	var file *os.File
	var err error
	for attempt := 0; attempt < 32; attempt++ {
		tmp = fmt.Sprintf("%s.tmp-%d-%d", path, time.Now().UnixNano(), attempt)
		file, err = openPrivateFileForWrite(tmp)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			return err
		}
	}
	if file == nil {
		return errors.New("unable to reserve an atomic private file name")
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := enforcePrivateExistingFileNoParent(tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := replaceFileAtomically(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := enforcePrivateExistingFileNoParent(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func restoreOperationMarkerPath(appData string) string {
	return filepath.Join(appData, "restore", RestoreOperationMarker)
}

func validRestoreOperationID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || filepath.Base(value) != value || strings.ContainsAny(value, `/\\`) {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// ReadRestoreOperation reads the single active operation marker. A completed
// or failed marker remains as evidence until a subsequent restore explicitly
// replaces it; concurrent starts therefore fail closed.
func ReadRestoreOperation(appData string) (RestoreOperation, error) {
	return readRestoreOperationFile(restoreOperationMarkerPath(appData))
}

func readRestoreOperationFile(path string) (RestoreOperation, error) {
	if err := requireRegularNonSymlinkFile(path); err != nil {
		return RestoreOperation{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return RestoreOperation{}, err
	}
	if info.Size() <= 0 || info.Size() > restoreOperationMaximumBytes {
		return RestoreOperation{}, errors.New("restore operation marker exceeds the bounded parser size")
	}
	file, err := openRegularFileForRead(path)
	if err != nil {
		return RestoreOperation{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, restoreOperationMaximumBytes))
	decoder.DisallowUnknownFields()
	var operation RestoreOperation
	if err := decoder.Decode(&operation); err != nil {
		return RestoreOperation{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return RestoreOperation{}, errors.New("restore operation marker contains trailing data")
	}
	if operation.Version != RestoreOperationVersion || !validRestoreOperationID(operation.OperationID) || !restorePhaseAllowed(operation.Phase) {
		return RestoreOperation{}, errors.New("restore operation marker is invalid")
	}
	return operation, nil
}

func WriteRestoreOperation(appData string, operation RestoreOperation) error {
	if operation.Version == 0 {
		operation.Version = RestoreOperationVersion
	}
	if !validRestoreOperationID(operation.OperationID) {
		return errors.New("restore operation id is invalid")
	}
	if operation.UpdatedAt == "" {
		operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	body, err := json.Marshal(operation)
	if err != nil {
		return err
	}
	return writeAtomicPrivateFile(restoreOperationMarkerPath(appData), body)
}

// ArchiveRestoreOperation preserves terminal operation evidence before the
// single active marker is replaced by a later restore. It is also the durable
// authority for retaining the verified logical safety snapshot referenced by
// that operation.
func ArchiveRestoreOperation(cfg config.Config, operation RestoreOperation) error {
	if !validRestoreOperationID(operation.OperationID) {
		return errors.New("restore operation id is invalid")
	}
	if err := restoreOperationPaths(cfg, &operation); err != nil {
		return fmt.Errorf("validate restore history paths: %w", err)
	}
	historyDir := filepath.Join(cfg.AppDataDir, "restore", "history")
	if err := enforcePrivateDirectory(historyDir); err != nil {
		return err
	}
	body, err := json.Marshal(operation)
	if err != nil {
		return err
	}
	path := filepath.Join(historyDir, operation.OperationID+".json")
	if err := requireRegularNonSymlinkFile(path); err == nil {
		existing, readErr := readRestoreOperationFile(path)
		if readErr != nil {
			return fmt.Errorf("read existing restore history record: %w", readErr)
		}
		existingBody, marshalErr := json.Marshal(existing)
		if marshalErr != nil {
			return marshalErr
		}
		if !bytes.Equal(existingBody, body) {
			return errors.New("restore history operation-id collision contains different evidence")
		}
		// Replaying the archive of the exact same terminal marker is safe.
		_, pruneErr := pruneRestoreHistory(cfg, 32, 90*24*time.Hour)
		return pruneErr
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := writeAtomicPrivateFile(path, body); err != nil {
		return err
	}
	_, pruneErr := pruneRestoreHistory(cfg, 32, 90*24*time.Hour)
	return pruneErr
}

// PruneRestoreHistory removes bounded, history-linked restore artifacts only
// after the history record and its safety authority are no longer needed.
// Callers that may race restore admission should use this public wrapper; the
// archive path uses the private helper because it already holds the lock.
func PruneRestoreHistory(cfg config.Config, maxEntries int, maxAge time.Duration) (int, error) {
	var removed int
	err := WithRestoreOperationLock(cfg, func() error {
		var err error
		removed, err = pruneRestoreHistory(cfg, maxEntries, maxAge)
		return err
	})
	return removed, err
}

type restoreHistoryEntry struct {
	path      string
	operation RestoreOperation
	created   time.Time
	verified  bool
}

func pruneRestoreHistory(cfg config.Config, maxEntries int, maxAge time.Duration) (int, error) {
	historyDir := filepath.Join(cfg.AppDataDir, "restore", "history")
	entries, err := os.ReadDir(historyDir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if maxEntries <= 0 {
		maxEntries = 32
	}
	if maxAge <= 0 {
		maxAge = 90 * 24 * time.Hour
	}
	active, activeErr := ReadRestoreOperation(cfg.AppDataDir)
	activeID := ""
	protectedPaths := map[string]bool{}
	if activeErr == nil {
		activeID = active.OperationID
		if restoreOperationPaths(cfg, &active) == nil {
			for _, path := range []string{active.SafetyCopyPath, active.OldActivePath, active.InstallPath, active.StagedPath} {
				if strings.TrimSpace(path) != "" {
					protectedPaths[filepath.Clean(path)] = true
				}
			}
		}
	}
	history := make([]restoreHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(historyDir, entry.Name())
		operation, readErr := readRestoreOperationFile(path)
		if readErr != nil || operation.OperationID+".json" != entry.Name() || restoreOperationPaths(cfg, &operation) != nil {
			// Invalid history is not silently treated as a recovery point. Keep it
			// visible for diagnostics; a bounded debris sweep may quarantine it
			// later once its grace period has elapsed.
			continue
		}
		created := time.Time{}
		for _, value := range []string{operation.CompletedAt, operation.UpdatedAt, operation.CreatedAt} {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, value); parseErr == nil {
				created = parsed
				break
			}
		}
		if created.IsZero() {
			if stat, statErr := entry.Info(); statErr == nil {
				created = stat.ModTime().UTC()
			}
		}
		history = append(history, restoreHistoryEntry{
			path: path, operation: operation, created: created,
			verified: restoreSafetyEvidenceRecorded(&operation),
		})
	}
	sort.SliceStable(history, func(i, j int) bool {
		if history[i].created.Equal(history[j].created) {
			return history[i].path > history[j].path
		}
		return history[i].created.After(history[j].created)
	})
	verifiedKept := false
	for _, item := range history {
		if item.verified && !verifiedKept {
			verifiedKept = true
		}
	}
	now := time.Now().UTC()
	removed := 0
	var firstErr error
	keptVerified := false
	for index, item := range history {
		if item.operation.OperationID == activeID || protectedPaths[filepath.Clean(item.operation.SafetyCopyPath)] {
			if item.verified {
				keptVerified = true
			}
			continue
		}
		tooMany := index >= maxEntries
		tooOld := !item.created.IsZero() && now.Sub(item.created) > maxAge
		if !tooMany && !tooOld {
			if item.verified {
				keptVerified = true
			}
			continue
		}
		if item.verified && !keptVerified && verifiedKept {
			keptVerified = true
			continue
		}
		if cleanupErr := cleanupHistoricalRestoreArtifacts(cfg, item.operation); cleanupErr != nil {
			if firstErr == nil {
				firstErr = cleanupErr
			}
			continue
		}
		if removeErr := removeRestoreArtifact(item.path); removeErr != nil {
			if firstErr == nil {
				firstErr = removeErr
			}
			continue
		}
		removed++
	}
	return removed, firstErr
}

func cleanupHistoricalRestoreArtifacts(cfg config.Config, operation RestoreOperation) error {
	if err := restoreOperationPaths(cfg, &operation); err != nil {
		return fmt.Errorf("validate restore history artifacts: %w", err)
	}
	active := filepath.Clean(cfg.DatabasePath)
	activeDir := filepath.Dir(active)
	restoreDir := filepath.Clean(filepath.Join(cfg.AppDataDir, "restore"))
	bases := []string{operation.SafetyCopyPath, operation.OldActivePath, operation.InstallPath,
		active + ".restore-failed-" + operation.OperationID,
		operation.InstallPath + ".restore-failed-" + operation.OperationID,
		operation.SafetyCopyPath + ".restore-interrupted-" + operation.OperationID,
		operation.InstallPath + ".restore-interrupted-" + operation.OperationID}
	// optionalRenameWithStage and interrupted quarantine intentionally choose a
	// collision suffix when a crash leaves both names. Those actual names are
	// not knowable from the fixed operation fields, so enumerate only the
	// operation-specific prefixes in the trusted active directory. Never sweep
	// arbitrary database-looking files or follow a directory/symlink.
	trustedDirs := map[string]bool{activeDir: true, restoreDir: true}
	readDirs := map[string]bool{}
	for _, base := range bases {
		dir := filepath.Dir(filepath.Clean(base))
		if trustedDirs[dir] {
			readDirs[dir] = true
		}
	}
	for dir := range readDirs {
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return readErr
		}
		prefixes := make([]string, 0, len(bases)*2)
		for _, base := range bases {
			if filepath.Dir(filepath.Clean(base)) != dir {
				continue
			}
			baseName := filepath.Base(filepath.Clean(base))
			if baseName == "." || baseName == string(filepath.Separator) || baseName == "" {
				continue
			}
			prefixes = append(prefixes, baseName+".retry-", baseName+"-retry-")
		}
		for _, entry := range entries {
			name := entry.Name()
			for _, prefix := range prefixes {
				if strings.HasPrefix(name, prefix) {
					bases = append(bases, filepath.Join(dir, name))
					break
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, base := range bases {
		base = filepath.Clean(base)
		if seen[base] || strings.TrimSpace(base) == "" || !trustedDirs[filepath.Dir(base)] {
			continue
		}
		seen[base] = true
		if err := cleanupRestoreArtifactPair(base); err != nil {
			return err
		}
	}
	if err := syncDirectory(activeDir); err != nil {
		return err
	}
	if restoreDir != activeDir {
		return syncDirectory(restoreDir)
	}
	return nil
}

// WriteRestoreOperationOwned performs a short cross-process lease check and
// marker publication. A stale executor cannot overwrite a takeover or a
// host-owned recovery-required transition after its lease has expired.
func WriteRestoreOperationOwned(cfg config.Config, operation RestoreOperation, claimant string) error {
	claimant = strings.TrimSpace(claimant)
	if !validRestoreOperationID(claimant) {
		return errors.New("restore executor claimant is invalid")
	}
	return WithRestoreOperationLock(cfg, func() error {
		current, err := ReadRestoreOperation(cfg.AppDataDir)
		if err != nil {
			return err
		}
		if current.OperationID != operation.OperationID || current.ExecutorID != claimant {
			return errors.New("restore executor lease is not owned")
		}
		operation.ExecutorID = current.ExecutorID
		operation.ExecutorPID = current.ExecutorPID
		operation.ExecutorLeaseUntil = current.ExecutorLeaseUntil
		return WriteRestoreOperation(cfg.AppDataDir, operation)
	})
}

func restoreOperationPaths(cfg config.Config, operation *RestoreOperation) error {
	if operation == nil || !validRestoreOperationID(operation.OperationID) {
		return errors.New("restore operation is invalid")
	}
	active := filepath.Clean(cfg.DatabasePath)
	if strings.TrimSpace(cfg.DatabasePath) == "" || active == "." {
		return errors.New("configured database path is required")
	}
	if filepath.Clean(operation.ActivePath) != active {
		return errors.New("restore operation active path does not match configured database")
	}
	restoreDir := filepath.Clean(filepath.Join(cfg.AppDataDir, "restore"))
	if strings.TrimSpace(cfg.AppDataDir) == "" || restoreDir == "." {
		return errors.New("private restore root is required")
	}
	regularStaged := CanonicalRestoreStagedPath(cfg, operation.OperationID, false)
	uploadStaged := CanonicalRestoreStagedPath(cfg, operation.OperationID, true)
	staged := filepath.Clean(operation.StagedPath)
	if staged != filepath.Clean(regularStaged) && staged != filepath.Clean(uploadStaged) {
		return errors.New("restore operation staged path is not the canonical operation artifact")
	}
	if operation.UploadReserved || operation.RawImport {
		if staged != filepath.Clean(uploadStaged) {
			return errors.New("restore upload operation staged path is not canonical")
		}
	}
	ownerLock := strings.TrimSpace(operation.UploadOwnerLockPath)
	if ownerLock != "" {
		if staged != filepath.Clean(uploadStaged) || filepath.Clean(ownerLock) != filepath.Clean(staged+".owner.lock") {
			return errors.New("restore operation upload owner lock is not canonical")
		}
	}
	expectedSafety := CanonicalRestoreSafetyCopyPath(cfg, operation.OperationID)
	expectedOld := CanonicalRestoreOldActivePath(cfg, operation.OperationID)
	expectedInstall := CanonicalRestoreInstallPath(cfg, operation.OperationID)
	if filepath.Clean(operation.SafetyCopyPath) != filepath.Clean(expectedSafety) {
		return errors.New("restore operation safety-copy path is not canonical")
	}
	if filepath.Clean(operation.OldActivePath) != filepath.Clean(expectedOld) {
		return errors.New("restore operation old-active path is not canonical")
	}
	if filepath.Clean(operation.InstallPath) != filepath.Clean(expectedInstall) {
		return errors.New("restore operation install path is not canonical")
	}
	return nil
}

var restoreRenameTestState struct {
	sync.RWMutex
	hook func(string) error
}

var restoreRollbackTestState struct {
	sync.RWMutex
	beforeValidation func(context.Context) error
}

// SetRestoreRenameForTest scopes the post-rename fault seam. The returned
// function restores the previous hook; the lock prevents a parallel -race
// test from observing a half-published hook. Production never installs one.
func SetRestoreRenameForTest(hook func(string) error) func() {
	restoreRenameTestState.Lock()
	previous := restoreRenameTestState.hook
	restoreRenameTestState.hook = hook
	restoreRenameTestState.Unlock()
	return func() {
		restoreRenameTestState.Lock()
		restoreRenameTestState.hook = previous
		restoreRenameTestState.Unlock()
	}
}

// SetRestoreRollbackBeforeValidationForTest pauses an entered rollback just
// before it validates/copies the verified safety authority. It is a scoped
// cancellation seam for startup recovery tests; production never installs it.
func SetRestoreRollbackBeforeValidationForTest(hook func(context.Context) error) func() {
	restoreRollbackTestState.Lock()
	previous := restoreRollbackTestState.beforeValidation
	restoreRollbackTestState.beforeValidation = hook
	restoreRollbackTestState.Unlock()
	return func() {
		restoreRollbackTestState.Lock()
		restoreRollbackTestState.beforeValidation = previous
		restoreRollbackTestState.Unlock()
	}
}

func renameRestoreArtifact(stage, source, target string) error {
	return renameRestoreArtifactContext(context.Background(), stage, source, target)
}

func renameRestoreArtifactContext(ctx context.Context, stage, source, target string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := assertSameReplacementDirectory(source, target); err != nil {
		return err
	}
	if err := replaceFileAtomicallyContext(ctx, source, target); err != nil {
		return err
	}
	restoreRenameTestState.RLock()
	hook := restoreRenameTestState.hook
	restoreRenameTestState.RUnlock()
	if hook != nil {
		return hook(stage)
	}
	return ctx.Err()
}

func optionalRenameWithStage(stage, source, target string) error {
	return optionalRenameWithStageContext(context.Background(), stage, source, target)
}

func optionalRenameWithStageContext(ctx context.Context, stage, source, target string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(source); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if targetInfo, err := os.Lstat(target); err == nil {
		if targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.Mode().IsRegular() {
			return errors.New("restore optional-rename target is not a regular file")
		}
		sourceInfo, sourceErr := os.Lstat(source)
		if sourceErr != nil {
			if os.IsNotExist(sourceErr) {
				return nil
			}
			return sourceErr
		}
		if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.Mode().IsRegular() {
			return errors.New("restore optional-rename source is not a regular file")
		}
		// A crash can leave both names after the move completed. Treat that
		// state as idempotent only after content proof; never accept an
		// existing target merely because it is present.
		sourceHash, sourceHashErr := FileSHA256Context(ctx, source)
		targetHash, targetHashErr := FileSHA256Context(ctx, target)
		if sourceHashErr == nil && targetHashErr == nil && strings.EqualFold(sourceHash, targetHash) {
			if err := os.Remove(source); err != nil && !os.IsNotExist(err) {
				return err
			}
			return syncDirectoryContext(ctx, filepath.Dir(source))
		}
		// Diagnostic old-active/failed sets are not rollback authority. Preserve
		// a differing pre-existing target under a collision-proof sibling and
		// continue the current move, so a crash/restart converges on the verified
		// logical safety snapshot instead of depending on Unix overwrite rules.
		var candidate string
		for attempt := 0; attempt < 1000; attempt++ {
			candidate = fmt.Sprintf("%s.retry-%s-%03d", target, strings.TrimPrefix(restoreRandomID("restore"), "restore_"), attempt)
			if _, candidateErr := os.Lstat(candidate); os.IsNotExist(candidateErr) {
				target = candidate
				break
			} else if candidateErr != nil {
				return candidateErr
			}
			if attempt == 999 {
				return errors.New("restore optional-rename target collision limit exceeded")
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return renameRestoreArtifactContext(ctx, stage, source, target)
}

func optionalRename(source, target string) error {
	return optionalRenameWithStage("optional-rename", source, target)
}

func moveSQLiteSidecars(sourceBase, targetBase string) error {
	return moveSQLiteSidecarsContext(context.Background(), sourceBase, targetBase)
}

func moveSQLiteSidecarsContext(ctx context.Context, sourceBase, targetBase string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := optionalRenameWithStageContext(ctx, "sidecar"+suffix, sourceBase+suffix, targetBase+suffix); err != nil {
			return err
		}
	}
	return nil
}

func validateVerifiedSafetyCopy(ctx context.Context, operation *RestoreOperation) error {
	if operation == nil || operation.SafetyCopySizeBytes <= 0 || strings.TrimSpace(operation.SafetyCopyChecksumSHA256) == "" || operation.SafetyCopyIdentity.MigrationHead == "" {
		return errors.New("verified logical pre-restore safety copy evidence is missing")
	}
	maximum := operation.RestoreMaxDatabaseBytes
	if maximum <= 0 {
		maximum = RestoreMaxDatabaseBytes
	}
	validation, err := InspectRestoreDatabaseWithLimit(ctx, operation.SafetyCopyPath, maximum)
	if err != nil {
		return err
	}
	if validation.SizeBytes != operation.SafetyCopySizeBytes || !strings.EqualFold(validation.ChecksumSHA256, operation.SafetyCopyChecksumSHA256) {
		return errors.New("pre-restore safety copy checksum or size changed")
	}
	identity := operation.SafetyCopyIdentity
	if identity.FormatVersion != validation.Migration.FormatVersion || identity.MigrationHead != validation.Migration.MigrationHead ||
		!strings.EqualFold(identity.LedgerSHA256, validation.Migration.LedgerSHA256) || identity.MinimumReader != validation.Migration.MinimumReader {
		return errors.New("pre-restore safety copy migration identity changed")
	}
	return nil
}

// InstallRestoreOperation performs the filesystem part of a supervised
// replacement after the host has stopped the old Server generation. It is
// resumable from every durable phase and never mutates the active path before
// the private safety copy and replacement temp are durable.
func InstallRestoreOperation(cfg config.Config, operation *RestoreOperation) error {
	return InstallRestoreOperationContext(context.Background(), cfg, operation)
}

func InstallRestoreOperationContext(ctx context.Context, cfg config.Config, operation *RestoreOperation) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := restoreOperationPaths(cfg, operation); err != nil {
		return err
	}
	if operation.Phase != RestorePhaseInstalling {
		operation.Phase, operation.State, operation.Progress = RestorePhaseSafetyCopy, RestorePhaseSafetyCopy, 45
		operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := WriteRestoreOperation(cfg.AppDataDir, *operation); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(cfg.DatabasePath); err == nil {
		if err := validateVerifiedSafetyCopy(ctx, operation); err != nil {
			return fmt.Errorf("validate pre-restore safety copy: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	_, activeStatErr := os.Lstat(cfg.DatabasePath)
	_, oldActiveStatErr := os.Lstat(operation.OldActivePath)
	_, installStatErr := os.Lstat(operation.InstallPath)
	if os.IsNotExist(installStatErr) && !(activeStatErr == nil && oldActiveStatErr == nil) {
		maximum := operation.RestoreMaxDatabaseBytes
		if maximum <= 0 {
			maximum = RestoreMaxDatabaseBytes
		}
		if err := CopyRestrictedFileSyncContext(ctx, operation.StagedPath, operation.InstallPath, maximum); err != nil {
			return err
		}
	} else if installStatErr != nil && !os.IsNotExist(installStatErr) {
		return installStatErr
	}
	operation.Phase, operation.State, operation.Progress = RestorePhaseInstalling, RestorePhaseInstalling, 60
	operation.ActiveMutationStarted = true
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := WriteRestoreOperation(cfg.AppDataDir, *operation); err != nil {
		return err
	}
	if _, activeErr := os.Lstat(cfg.DatabasePath); activeErr == nil {
		if _, oldErr := os.Lstat(operation.OldActivePath); os.IsNotExist(oldErr) {
			if err := moveSQLiteSidecarsContext(ctx, cfg.DatabasePath, operation.OldActivePath); err != nil {
				return err
			}
			if err := renameRestoreArtifactContext(ctx, "install-old-active", cfg.DatabasePath, operation.OldActivePath); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(activeErr) {
		return activeErr
	}
	if _, err := os.Lstat(cfg.DatabasePath); os.IsNotExist(err) {
		if err := renameRestoreArtifactContext(ctx, "install-new-active", operation.InstallPath, cfg.DatabasePath); err != nil {
			return err
		}
		operation.ActiveMutationCompleted = true
	} else if err != nil {
		return err
	} else if _, oldErr := os.Lstat(operation.OldActivePath); oldErr == nil {
		// A crash can occur after the restored install rename and before this
		// marker update. The old-active evidence proves that the current active
		// file is the installed generation, so restart reconciliation must repair
		// the durable completion bit instead of replaying or misclassifying it.
		operation.ActiveMutationCompleted = true
		if _, installErr := os.Lstat(operation.InstallPath); installErr == nil {
			// A recovery retry may have prepared a duplicate install temp before
			// recognizing that the restored active and old-active pair was already
			// published. It is transient and can be removed only after that pair is
			// proven present; leaving it would leak a database-sized artifact.
			if err := cleanupRestoreArtifactPair(operation.InstallPath); err != nil {
				return err
			}
		} else if !os.IsNotExist(installErr) {
			return installErr
		}
	}
	if err := syncDirectoryContext(ctx, filepath.Dir(cfg.DatabasePath)); err != nil {
		return err
	}
	operation.Phase, operation.State, operation.Progress = RestorePhaseReopening, RestorePhaseReopening, 72
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return WriteRestoreOperation(cfg.AppDataDir, *operation)
}

func RollbackRestoreOperation(cfg config.Config, operation *RestoreOperation, code string, cause error) error {
	return RollbackRestoreOperationContext(context.Background(), cfg, operation, code, cause)
}

func RollbackRestoreOperationContext(ctx context.Context, cfg config.Config, operation *RestoreOperation, code string, cause error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := restoreOperationPaths(cfg, operation); err != nil {
		return err
	}
	if operation.RollbackPendingHealth && operation.Phase == RestorePhaseHealthChecking {
		return nil
	}
	operation.Phase, operation.State, operation.Progress = RestorePhaseRollingBack, RestorePhaseRollingBack, 82
	operation.ErrorCode = strings.TrimSpace(code)
	operation.ErrorMessage = restoreSafeMessage(operation.ErrorCode)
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := WriteRestoreOperation(cfg.AppDataDir, *operation); err != nil {
		return err
	}
	restoreRollbackTestState.RLock()
	beforeValidation := restoreRollbackTestState.beforeValidation
	restoreRollbackTestState.RUnlock()
	if beforeValidation != nil {
		if err := beforeValidation(ctx); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateVerifiedSafetyCopy(ctx, operation); err != nil {
		return fmt.Errorf("rollback safety copy is not verified: %w", err)
	}
	failedPath := cfg.DatabasePath + ".restore-failed-" + operation.OperationID
	if _, activeErr := os.Lstat(cfg.DatabasePath); activeErr == nil {
		if err := moveSQLiteSidecarsContext(ctx, cfg.DatabasePath, failedPath); err != nil {
			return err
		}
		if err := renameRestoreArtifactContext(ctx, "rollback-failed-active", cfg.DatabasePath, failedPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(activeErr) {
		return activeErr
	}
	if _, err := os.Lstat(operation.InstallPath); err == nil {
		quarantine := operation.InstallPath + ".restore-failed-" + operation.OperationID
		if err := renameRestoreArtifactContext(ctx, "rollback-quarantine-install", operation.InstallPath, quarantine); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	maximum := operation.RestoreMaxDatabaseBytes
	if maximum <= 0 {
		maximum = RestoreMaxDatabaseBytes
	}
	if err := CopyRestrictedFileSyncContext(ctx, operation.SafetyCopyPath, operation.InstallPath, maximum); err != nil {
		return err
	}
	if err := renameRestoreArtifactContext(ctx, "rollback-install-safety", operation.InstallPath, cfg.DatabasePath); err != nil {
		return err
	}
	if err := syncDirectoryContext(ctx, filepath.Dir(cfg.DatabasePath)); err != nil {
		return err
	}
	// Filesystem rollback is not yet a truthful terminal result. The host must
	// open this verified logical snapshot and prove integrity, FK emptiness,
	// migration identity, and application health before exposing it or marking
	// the operation failed.
	operation.RollbackPendingHealth = true
	operation.Phase, operation.State, operation.Progress = RestorePhaseHealthChecking, RestorePhaseHealthChecking, 90
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return WriteRestoreOperation(cfg.AppDataDir, *operation)
}

func CompleteRestoreOperation(cfg config.Config, operation *RestoreOperation) error {
	if err := restoreOperationPaths(cfg, operation); err != nil {
		return err
	}
	operation.Phase, operation.State, operation.Progress = RestorePhaseComplete, RestorePhaseComplete, 100
	operation.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	operation.UpdatedAt = operation.CompletedAt
	if err := WriteRestoreOperation(cfg.AppDataDir, *operation); err != nil {
		return err
	}
	// Only after the verified complete marker is durable may transient staging
	// and the moved replacement be removed. Cleanup is post-commit: an error
	// records a warning and is retried, but it never invalidates the completed
	// restore or asks the host to roll it back.
	return finishRestoreCleanup(cfg, operation)
}

// CompleteRestoreRollbackOperation marks a rollback terminal only after the
// host has opened and health-checked the verified logical safety snapshot.
func CompleteRestoreRollbackOperation(cfg config.Config, operation *RestoreOperation) error {
	if err := restoreOperationPaths(cfg, operation); err != nil {
		return err
	}
	operation.RollbackPendingHealth = false
	operation.Phase, operation.State, operation.Progress = RestorePhaseFailed, RestorePhaseFailed, 100
	if strings.TrimSpace(operation.ErrorCode) == "" {
		operation.ErrorCode = "restore_runtime_replacement_failed"
	}
	operation.ErrorMessage = restoreSafeMessage(operation.ErrorCode)
	operation.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	operation.UpdatedAt = operation.CompletedAt
	if err := WriteRestoreOperation(cfg.AppDataDir, *operation); err != nil {
		return err
	}
	return finishRestoreCleanup(cfg, operation)
}

// MarkRestoreRecoveryRequired is deliberately non-terminal. It keeps the
// maintenance responder fail-closed when a rollback filesystem or health
// proof cannot be completed; a terminal failed marker would falsely imply a
// verified runtime is serving.
func MarkRestoreRecoveryRequired(cfg config.Config, operation *RestoreOperation, code string) error {
	if operation == nil {
		return errors.New("restore operation is required")
	}
	if err := restoreOperationPaths(cfg, operation); err != nil {
		return err
	}
	operation.Phase, operation.State, operation.Progress = RestorePhaseRollingBack, RestoreStateRecoveryNeeded, 100
	operation.RollbackPendingHealth = true
	operation.ErrorCode = strings.TrimSpace(code)
	if operation.ErrorCode == "" {
		operation.ErrorCode = "restore_recovery_required"
	}
	operation.ErrorMessage = restoreSafeMessage("restore_recovery_required")
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return WriteRestoreOperation(cfg.AppDataDir, *operation)
}

func removeRestoreArtifact(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("restore cleanup encountered a non-regular artifact")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func cleanupRestoreArtifactPair(base string) error {
	// Remove sidecars first. If a sidecar cannot be removed, retain the main
	// artifact so the pair is never reported as cleanly deleted.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := removeRestoreArtifact(base + suffix); err != nil {
			return err
		}
	}
	return removeRestoreArtifact(base)
}

func cleanupRestoreTransientArtifacts(cfg config.Config, operation *RestoreOperation) error {
	var first error
	for _, path := range []string{operation.StagedPath, operation.InstallPath, operation.OldActivePath} {
		if err := cleanupRestoreArtifactPair(path); err != nil && first == nil {
			first = err
		}
	}
	if err := syncDirectory(filepath.Dir(cfg.DatabasePath)); err != nil && first == nil {
		first = err
	}
	return first
}

func finishRestoreCleanup(cfg config.Config, operation *RestoreOperation) error {
	cleanupErr := cleanupRestoreTransientArtifacts(cfg, operation)
	if cleanupErr != nil {
		operation.CleanupPending = true
		operation.WarningCode = "restore_cleanup_pending"
		operation.WarningMessage = restoreSafeMessage("restore_cleanup_pending")
		operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := WriteRestoreOperation(cfg.AppDataDir, *operation); err != nil {
			return &RestorePostCommitError{Err: errors.Join(cleanupErr, err)}
		}
		return &RestorePostCommitError{Err: cleanupErr}
	}
	if operation.CleanupPending || operation.WarningCode != "" || operation.WarningMessage != "" {
		operation.CleanupPending = false
		operation.WarningCode = ""
		operation.WarningMessage = ""
		operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := WriteRestoreOperation(cfg.AppDataDir, *operation); err != nil {
			return &RestorePostCommitError{Err: err}
		}
	}
	return nil
}

// DiscardRestoreStagedArtifact is used only for a pre-mutation rejected
// operation. It never touches the active database, safety authority, or any
// path that could contain the installed generation.
func DiscardRestoreStagedArtifact(cfg config.Config, operation *RestoreOperation) error {
	if operation == nil {
		return errors.New("restore operation is required")
	}
	if operation.ActiveMutationStarted || operation.ActiveMutationCompleted {
		return errors.New("cannot discard a restore artifact after active mutation")
	}
	if err := restoreOperationPaths(cfg, operation); err != nil {
		return err
	}
	if err := cleanupRestoreArtifactPair(operation.StagedPath); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(operation.StagedPath))
}

// RetryCompletedRestoreCleanup is safe to call during startup and retention
// maintenance. The complete/failed marker remains authoritative throughout.
func RetryCompletedRestoreCleanup(cfg config.Config) error {
	operation, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		return err
	}
	if operation.Phase != RestorePhaseComplete && operation.Phase != RestorePhaseFailed {
		return nil
	}
	return finishRestoreCleanup(cfg, &operation)
}

func restoreSafetyEvidenceRecorded(operation *RestoreOperation) bool {
	return operation != nil && operation.SafetyCopySizeBytes > 0 &&
		strings.TrimSpace(operation.SafetyCopyChecksumSHA256) != "" &&
		operation.SafetyCopyIdentity.MigrationHead != "" &&
		operation.SafetyCopyIdentity.LedgerSHA256 != ""
}

func quarantineRestoreSet(base, operationID string) error {
	quarantineBase, err := chooseQuarantineBase(base, operationID)
	if err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal", ""} {
		if err := quarantineRestoreArtifact(base+suffix, quarantineBase+suffix); err != nil {
			return err
		}
	}
	return syncDirectory(filepath.Dir(base))
}

func chooseQuarantineBase(base, operationID string) (string, error) {
	baseName := base + ".restore-interrupted-" + operationID
	for attempt := 0; attempt < 1000; attempt++ {
		candidate := baseName
		if attempt > 0 {
			candidate = fmt.Sprintf("%s-retry-%03d", baseName, attempt)
		}
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			// A prior crash may have left this exact target while the source was
			// removed. Reuse it only when it is already a valid regular artifact;
			// otherwise select a fresh collision-proof name.
			return candidate, nil
		} else if err != nil {
			return "", err
		}
		if attempt == 0 {
			// Existing target is handled idempotently by the artifact mover when
			// the source has already disappeared. If both exist, choose retry.
			continue
		}
	}
	return "", errors.New("unable to reserve an interrupted-restore quarantine name")
}

func quarantineRestoreArtifact(source, target string) error {
	sourceInfo, sourceErr := os.Lstat(source)
	targetInfo, targetErr := os.Lstat(target)
	if os.IsNotExist(sourceErr) {
		if os.IsNotExist(targetErr) {
			return nil
		}
		if targetErr != nil {
			return targetErr
		}
		if targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.Mode().IsRegular() {
			return errors.New("restore quarantine target is not a regular file")
		}
		return nil
	}
	if sourceErr != nil {
		return sourceErr
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.Mode().IsRegular() {
		return errors.New("restore quarantine source is not a regular file")
	}
	if targetErr == nil {
		if targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.Mode().IsRegular() {
			return errors.New("restore quarantine target is not a regular file")
		}
		// Both names can exist only after a crash/interleaving. Never accept a
		// target blindly: matching content means the move completed and the
		// source is safe to remove; differing content is a collision and must
		// be left for the retry suffix selected by the caller.
		sourceHash, sourceHashErr := fileSHA256(source)
		targetHash, targetHashErr := fileSHA256(target)
		if sourceHashErr != nil || targetHashErr != nil || !strings.EqualFold(sourceHash, targetHash) {
			return os.ErrExist
		}
		return os.Remove(source)
	}
	if !os.IsNotExist(targetErr) {
		return targetErr
	}
	return renameRestoreArtifact("quarantine-interrupted", source, target)
}

func quarantineInterruptedRestoreArtifacts(cfg config.Config, operation *RestoreOperation) error {
	for _, path := range []string{operation.SafetyCopyPath, operation.InstallPath} {
		if err := quarantineRestoreSet(path, operation.OperationID); err != nil {
			return err
		}
	}
	return nil
}

// RecoverInterruptedRestoreBeforeOpen is called by the host before it opens
// the active SQLite handle. It handles the only crash window that can make a
// normal startup fail before the app generation exists: an install recorded
// before replacement finished, or a replacement recorded before reopening.
// Validating/staged/quiescing markers are left for the host runtime controller;
// they have not yet mutated the active database.
func RecoverInterruptedRestoreBeforeOpen(cfg config.Config) error {
	return RecoverInterruptedRestoreBeforeOpenContext(context.Background(), cfg)
}

func RecoverInterruptedRestoreBeforeOpenContext(ctx context.Context, cfg config.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	operation, err := ReadRestoreOperation(cfg.AppDataDir)
	if os.IsNotExist(err) {
		return cleanupOrphanedRestoreOwnerLocks(cfg)
	}
	if err != nil {
		return fmt.Errorf("read durable restore operation before database open: %w", err)
	}
	if !operation.AuthorizationCommitted && operation.Phase != RestorePhaseComplete && operation.Phase != RestorePhaseFailed {
		return recoverUncommittedRestoreAuthorization(cfg, &operation)
	}
	if operation.State == RestoreStateRecoveryNeeded {
		if operation.ActiveMutationStarted {
			if !restoreSafetyEvidenceRecorded(&operation) {
				return nil
			}
			if err := RollbackRestoreOperationContext(ctx, cfg, &operation, operation.ErrorCode, errors.New("restore recovery resumed after process restart")); err != nil {
				return err
			}
			return nil
		}
		// A shutdown/close failure before the first active rename leaves the
		// configured active database authoritative, but the old process could
		// not prove that at the time. On a fresh process, perform a read-only
		// identity/integrity proof and resolve the marker to a truthful failed
		// restore so normal startup can resume. If the active set cannot be
		// proven, keep the status-only maintenance responder fail-closed.
		if err := validateActiveDatabaseForRecovery(ctx, cfg); err == nil {
			operation.Phase, operation.State, operation.Progress = RestorePhaseFailed, RestorePhaseFailed, 100
			operation.ErrorCode = "restore_recovery_resolved"
			operation.ErrorMessage = "The restore did not complete; the verified active database was retained."
			operation.RollbackPendingHealth = false
			operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			return WriteRestoreOperation(cfg.AppDataDir, operation)
		}
		return nil
	}
	if operation.BackupName == "uploaded-database" && operation.Phase == RestorePhaseValidating {
		if handled, recoveryErr := recoverInterruptedUploadReservation(cfg, &operation); handled {
			return recoveryErr
		}
	}
	switch operation.Phase {
	case "", RestorePhaseValidating, RestorePhaseStaged, RestorePhaseFailed:
		return nil
	case RestorePhaseComplete:
		if err := RetryCompletedRestoreCleanup(cfg); err != nil {
			var committed *RestorePostCommitError
			if errors.As(err, &committed) {
				return nil
			}
			return err
		}
		return nil
	case RestorePhaseRollingBack:
		return RollbackRestoreOperationContext(ctx, cfg, &operation, operation.ErrorCode, errors.New("restore rollback resumed after process restart"))
	case RestorePhaseQuiescing, RestorePhaseSafetyCopy, RestorePhaseInstalling, RestorePhaseReopening, RestorePhaseHealthChecking:
		evidenceRecorded := restoreSafetyEvidenceRecorded(&operation)
		if (operation.Phase == RestorePhaseQuiescing || operation.Phase == RestorePhaseSafetyCopy || operation.Phase == RestorePhaseInstalling) && !evidenceRecorded {
			if operation.ActiveMutationStarted {
				return MarkRestoreRecoveryRequired(cfg, &operation, "restore_recovery_required")
			}
			if err := quarantineInterruptedRestoreArtifacts(cfg, &operation); err != nil {
				return err
			}
			return markInterruptedRestoreFailed(cfg, &operation, "restore_safety_copy_interrupted", errors.New("restore stopped before its logical safety copy was durable"))
		}
		if evidenceRecorded && !operation.ActiveMutationStarted {
			if err := validateVerifiedSafetyCopy(ctx, &operation); err != nil {
				if quarantineErr := quarantineInterruptedRestoreArtifacts(cfg, &operation); quarantineErr != nil {
					return quarantineErr
				}
				return markInterruptedRestoreFailed(cfg, &operation, "restore_safety_copy_interrupted", err)
			}
		}
		if operation.Phase == RestorePhaseQuiescing || operation.Phase == RestorePhaseSafetyCopy || operation.Phase == RestorePhaseInstalling {
			if err := InstallRestoreOperationContext(ctx, cfg, &operation); err != nil {
				if !evidenceRecorded {
					return MarkRestoreRecoveryRequired(cfg, &operation, "restore_recovery_required")
				}
				if rollbackErr := RollbackRestoreOperationContext(ctx, cfg, &operation, "restore_install_failed", err); rollbackErr != nil {
					return rollbackErr
				}
				return nil
			}
		}
		return reconcileInstalledRestoreFilesystem(ctx, cfg, &operation)
	default:
		return fmt.Errorf("unknown durable restore phase %q", operation.Phase)
	}
}

// recoverUncommittedRestoreAuthorization resolves a crash between publishing
// the private operation marker and confirming that the initiating credential
// or Hosted authorization receipt committed in SQLite. The staged candidate is
// untrusted and is quarantined; the active database is never touched.
func recoverUncommittedRestoreAuthorization(cfg config.Config, operation *RestoreOperation) error {
	if operation == nil || operation.AuthorizationCommitted {
		return nil
	}
	if operation.ActiveMutationStarted {
		return MarkRestoreRecoveryRequired(cfg, operation, "restore_recovery_required")
	}
	if err := restoreOperationPaths(cfg, operation); err != nil {
		return err
	}
	ownerLock := RestoreUploadOwnerLockPath(*operation)
	if ownerLock != "" {
		release, acquired, err := TryAcquireRestoreArtifactLock(ownerLock)
		if err != nil {
			return err
		}
		if !acquired {
			// A live uploader may be between the marker and SQLite commit. Leave
			// the fail-closed marker untouched until that kernel owner exits.
			return nil
		}
		release()
	}
	for _, candidate := range []string{operation.StagedPath, operation.SafetyCopyPath, operation.InstallPath} {
		if err := quarantineRestoreSet(candidate, operation.OperationID); err != nil {
			return err
		}
	}
	if ownerLock != "" {
		if err := os.Remove(ownerLock); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := syncDirectory(filepath.Dir(ownerLock)); err != nil {
			return err
		}
	}
	return markInterruptedRestoreFailed(cfg, operation, "restore_authorization_not_committed", errors.New("restore authorization commit was not durably confirmed"))
}

// cleanupOrphanedRestoreOwnerLocks removes only unlocked upload owner locks
// that were created before a reservation marker could be published. A lock
// still held by a slow/live uploader is left untouched; the bounded scan keeps
// crash debris from accumulating without treating PID or lease evidence as
// ownership.
func cleanupOrphanedRestoreOwnerLocks(cfg config.Config) error {
	restoreDir := filepath.Join(cfg.AppDataDir, "restore")
	entries, err := os.ReadDir(restoreDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var first error
	seen := 0
	for _, entry := range entries {
		if seen >= 256 || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".owner.lock") {
			continue
		}
		seen++
		path := filepath.Join(restoreDir, entry.Name())
		release, acquired, lockErr := TryAcquireRestoreArtifactLock(path)
		if lockErr != nil {
			if first == nil {
				first = lockErr
			}
			continue
		}
		if !acquired {
			continue
		}
		release()
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			if first == nil {
				first = removeErr
			}
			continue
		}
		if syncErr := syncDirectory(restoreDir); syncErr != nil && first == nil {
			first = syncErr
		}
	}
	return first
}

func validateActiveDatabaseForRecovery(ctx context.Context, cfg config.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := requireRegularNonSymlinkFile(cfg.DatabasePath); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", readOnlySQLiteDSN(cfg.DatabasePath))
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer db.Close()
	_, err = ValidateOpenDatabaseHealth(ctx, db)
	return err
}

func recoverInterruptedUploadReservation(cfg config.Config, operation *RestoreOperation) (bool, error) {
	if operation == nil {
		return false, nil
	}
	ownerLock := RestoreUploadOwnerLockPath(*operation)
	if ownerLock == "" || !pathWithin(filepath.Join(cfg.AppDataDir, "restore"), ownerLock) {
		return true, errors.New("restore upload owner lock is outside the private restore root")
	}
	ownerRelease, acquired, err := TryAcquireRestoreArtifactLock(ownerLock)
	if err != nil {
		return true, err
	}
	if !acquired {
		// The kernel-owned lock is the only live-upload authority. Do not use
		// PID namespaces or a wall-clock lease to classify a slow request as
		// abandoned while another process still owns its staged bytes.
		return true, nil
	}
	ownerRelease()
	removeOwnerLock := func() error {
		if err := os.Remove(ownerLock); err != nil && !os.IsNotExist(err) {
			return err
		}
		return syncDirectory(filepath.Dir(ownerLock))
	}
	if operation.UploadComplete && operation.StagedPath != "" {
		// A complete upload can resume ordinary validation; only an incomplete
		// reservation is classified as interrupted here.
		if err := removeOwnerLock(); err != nil {
			return true, err
		}
		return false, nil
	}
	if err := cleanupRestoreArtifactPair(operation.StagedPath); err != nil {
		return true, err
	}
	if err := removeOwnerLock(); err != nil {
		return true, err
	}
	operation.UploadReserved = false
	operation.Phase, operation.State, operation.Progress = RestorePhaseFailed, RestorePhaseFailed, 100
	operation.ErrorCode = "restore_upload_interrupted"
	operation.ErrorMessage = "The supervised database upload stopped before it was complete."
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return true, WriteRestoreOperation(cfg.AppDataDir, *operation)
}

func markInterruptedRestoreFailed(cfg config.Config, operation *RestoreOperation, code string, cause error) error {
	operation.Phase, operation.State, operation.Progress = RestorePhaseFailed, RestorePhaseFailed, 100
	operation.ErrorCode = code
	operation.ErrorMessage = restoreSafeMessage(code)
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return WriteRestoreOperation(cfg.AppDataDir, *operation)
}

func reconcileInstalledRestoreFilesystem(ctx context.Context, cfg config.Config, operation *RestoreOperation) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(cfg.DatabasePath); err != nil {
		if rollbackErr := RollbackRestoreOperationContext(ctx, cfg, operation, "restore_restart_recovery_failed", err); rollbackErr != nil {
			return rollbackErr
		}
		return nil
	}
	if err := requireRegularNonSymlinkFile(cfg.DatabasePath); err != nil {
		if rollbackErr := RollbackRestoreOperationContext(ctx, cfg, operation, "restore_restart_recovery_failed", err); rollbackErr != nil {
			return rollbackErr
		}
		return nil
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		sidecar := cfg.DatabasePath + suffix
		if _, err := os.Lstat(sidecar); err == nil {
			if err := requireRegularNonSymlinkFile(sidecar); err != nil {
				if rollbackErr := RollbackRestoreOperationContext(ctx, cfg, operation, "restore_restart_recovery_failed", err); rollbackErr != nil {
					return rollbackErr
				}
				return nil
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if operation.Phase != RestorePhaseHealthChecking {
		operation.Phase, operation.State, operation.Progress = RestorePhaseHealthChecking, RestorePhaseHealthChecking, 90
		operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := WriteRestoreOperation(cfg.AppDataDir, *operation); err != nil {
			return err
		}
	}
	return nil
}
