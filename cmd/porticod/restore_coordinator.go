package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/app"
	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
	"github.com/PorticoMediaServer/portico-server/internal/redaction"
)

// restoreGeneration is the deliberately small host/app lifecycle seam. A
// generation is constructed inert, health-checked, and only then activated.
// Keeping this interface here lets coordinator tests inject deterministic
// shutdown, timeout, and health failures without starting a Portico process.
type restoreGeneration interface {
	SetRestoreRuntimeController(app.RestoreRuntimeController)
	ActivateRuntimeGeneration()
	DetachRestoreDatabaseHandle() (*sql.DB, error)
	Handler() http.Handler
	RestoreMaintenanceHandler() http.Handler
	BeginShutdown()
	StopRestoreRunners(context.Context) error
	Shutdown(context.Context) error
	PrepareRestoreSafetyCopy(context.Context, string) error
	ValidateRuntimeHealth(context.Context) error
	CompleteRestoreGeneration(context.Context, string) error
	RestoreRuntimeFailure()
	StartRemoteAccessManager()
	RemoteAccessCertificateLoader() func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	MarkHTTPReady(string)
	StartDeferredStartupMaintenance()
	ResumeRestoreOperation()
}

type restoreCoordinatorDependencies struct {
	BuildGeneration func(*sql.DB) restoreGeneration
	OpenDatabase    func(context.Context, config.Config) (*sql.DB, error)
	CloseDatabase   func(*sql.DB) error
	InstallRestore  func(context.Context, config.Config, *database.RestoreOperation) error
	RollbackRestore func(context.Context, config.Config, *database.RestoreOperation, string, error) error
	ReadOperation   func(string) (database.RestoreOperation, error)
	MarkRecovery    func(config.Config, *database.RestoreOperation, string) error
	Activate        func(restoreGeneration, *sql.DB, bool)
	SetHandler      func(http.Handler)
}

type restoreCoordinatorTimeouts struct {
	SafetyCopy time.Duration
	Shutdown   time.Duration
	Install    time.Duration
	Open       time.Duration
	Health     time.Duration
	Rollback   time.Duration
}

func defaultRestoreCoordinatorTimeouts() restoreCoordinatorTimeouts {
	return restoreCoordinatorTimeouts{
		SafetyCopy: 30 * time.Second,
		Shutdown:   30 * time.Second,
		Install:    2 * time.Minute,
		Open:       2 * time.Minute,
		Health:     45 * time.Second,
		Rollback:   2 * time.Minute,
	}
}

type restoreCoordinator struct {
	cfg     config.Config
	logger  *slog.Logger
	deps    restoreCoordinatorDependencies
	timeout restoreCoordinatorTimeouts

	// lifecycleMu serializes replacement with process shutdown and keeps the
	// current DB/generation pair coherent across every host-owned transition.
	lifecycleMu  sync.Mutex
	stateMu      sync.Mutex
	currentDB    *sql.DB
	current      restoreGeneration
	activated    map[restoreGeneration]bool
	controller   app.RestoreRuntimeController
	shuttingDown bool
}

var errRestoreDatabaseHandleCloseUnproven = errors.New("restore database handle close could not be proven")

func newRestoreCoordinator(cfg config.Config, logger *slog.Logger, deps restoreCoordinatorDependencies) *restoreCoordinator {
	c := &restoreCoordinator{
		cfg:       cfg,
		logger:    logger,
		deps:      deps,
		timeout:   defaultRestoreCoordinatorTimeouts(),
		activated: map[restoreGeneration]bool{},
	}
	if cfg.RestoreSafetyCopyTimeout > 0 {
		c.timeout.SafetyCopy = cfg.RestoreSafetyCopyTimeout
	}
	if cfg.RestoreIOTimeout > 0 {
		c.timeout.Install = cfg.RestoreIOTimeout
		c.timeout.Open = cfg.RestoreIOTimeout
		c.timeout.Rollback = cfg.RestoreIOTimeout
	}
	c.controller = c.replace
	return c
}

func (c *restoreCoordinator) BuildGeneration(db *sql.DB) restoreGeneration {
	if c.deps.BuildGeneration == nil {
		return nil
	}
	generation := c.deps.BuildGeneration(db)
	if generation != nil {
		generation.SetRestoreRuntimeController(c.controller)
	}
	return generation
}

func (c *restoreCoordinator) setCurrentLocked(generation restoreGeneration, db *sql.DB) {
	c.stateMu.Lock()
	c.current = generation
	c.currentDB = db
	c.stateMu.Unlock()
}

func (c *restoreCoordinator) currentGeneration() (restoreGeneration, *sql.DB) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.current, c.currentDB
}

// Activate is idempotent per generation. It is intentionally called only
// after the caller has performed the explicit database and application health
// gate; the coordinator never starts an unverified runtime.
func (c *restoreCoordinator) Activate(generation restoreGeneration, db *sql.DB, expose bool) {
	if generation == nil || db == nil {
		return
	}
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	c.activateLocked(generation, db, expose)
}

func (c *restoreCoordinator) activateLocked(generation restoreGeneration, db *sql.DB, expose bool) {
	c.setCurrentLocked(generation, db)
	if !c.activated[generation] {
		c.activated[generation] = true
		if c.deps.Activate != nil {
			c.deps.Activate(generation, db, expose)
		}
	} else if expose && c.deps.SetHandler != nil {
		c.deps.SetHandler(generation.Handler())
	}
}

func (c *restoreCoordinator) markFailed(operationID, code string, cause error) error {
	operation, err := c.deps.ReadOperation(c.cfg.AppDataDir)
	if err != nil {
		return err
	}
	if operation.OperationID != operationID {
		return errors.New("restore operation changed while recording failure")
	}
	operation.Phase, operation.State, operation.Progress = database.RestorePhaseFailed, database.RestorePhaseFailed, 100
	operation.ErrorCode = code
	operation.ErrorMessage = "The supervised restore did not complete."
	operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if c.logger != nil && cause != nil {
		c.logger.Error("restore lifecycle failure", "operation", operationID, "error", redaction.Error(cause, c.cfg.DatabasePath, c.cfg.AppDataDir, c.cfg.BackupDir))
	}
	return database.WriteRestoreOperation(c.cfg.AppDataDir, operation)
}

func (c *restoreCoordinator) markRecovery(operationID, code string, cause error) error {
	operation, err := c.deps.ReadOperation(c.cfg.AppDataDir)
	if err != nil {
		return err
	}
	if operation.OperationID != operationID {
		return errors.New("restore operation changed while recording recovery requirement")
	}
	if c.logger != nil && cause != nil {
		c.logger.Error("restore recovery required", "operation", operationID, "error", redaction.Error(cause, c.cfg.DatabasePath, c.cfg.AppDataDir, c.cfg.BackupDir))
	}
	if c.deps.MarkRecovery != nil {
		return c.deps.MarkRecovery(c.cfg, &operation, code)
	}
	return database.MarkRestoreRecoveryRequired(c.cfg, &operation, code)
}

func (c *restoreCoordinator) rollbackAndRebuildLocked(operationID string, cause error) error {
	operation, err := c.deps.ReadOperation(c.cfg.AppDataDir)
	if err != nil {
		return err
	}
	if operation.OperationID != operationID {
		return errors.New("restore operation changed during rollback")
	}
	rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), c.timeout.Rollback)
	if c.deps.RollbackRestore == nil {
		rollbackCancel()
		return errors.New("restore rollback dependency is unavailable")
	}
	if err := c.deps.RollbackRestore(rollbackCtx, c.cfg, &operation, "restore_runtime_replacement_failed", cause); err != nil {
		rollbackCancel()
		_ = c.markRecovery(operationID, "restore_recovery_required", err)
		return err
	}
	rollbackCancel()
	restoredDB, err := c.openDatabaseWithTimeout(context.Background())
	if err != nil {
		_ = c.markRecovery(operationID, "restore_recovery_required", err)
		return err
	}
	generation := c.BuildGeneration(restoredDB)
	if generation == nil {
		_ = c.deps.CloseDatabase(restoredDB)
		_ = c.markRecovery(operationID, "restore_recovery_required", errors.New("restore generation construction failed"))
		return errors.New("restore generation construction failed")
	}
	healthCtx, healthCancel := context.WithTimeout(context.Background(), c.timeout.Health)
	healthErr := generation.ValidateRuntimeHealth(healthCtx)
	healthCancel()
	if err := healthErr; err != nil {
		_ = c.deps.CloseDatabase(restoredDB)
		_ = c.markRecovery(operationID, "restore_recovery_required", err)
		return err
	}
	// Rollback is a successful recovery of the runtime, not a successful
	// restore of the requested backup. Validate the rebuilt safety-copy
	// generation, then record a terminal failed/rolled-back result; never use
	// the normal restore completion transition here.
	operation, err = c.deps.ReadOperation(c.cfg.AppDataDir)
	if err != nil {
		_ = c.deps.CloseDatabase(restoredDB)
		_ = c.markRecovery(operationID, "restore_recovery_required", err)
		return err
	}
	completionErr := database.CompleteRestoreRollbackOperation(c.cfg, &operation)
	var postCommit *database.RestorePostCommitError
	if completionErr != nil && !errors.As(completionErr, &postCommit) {
		_ = c.deps.CloseDatabase(restoredDB)
		_ = c.markRecovery(operationID, "restore_recovery_required", completionErr)
		return completionErr
	}
	c.activateLocked(generation, restoredDB, true)
	return nil
}

func (c *restoreCoordinator) openDatabaseWithTimeout(parent context.Context) (*sql.DB, error) {
	if c.deps.OpenDatabase == nil {
		return nil, errors.New("restore database-open dependency is unavailable")
	}
	if parent == nil {
		parent = context.Background()
	}
	// Derive the actual phase deadline before starting the open attempt. Keep the
	// call synchronous: the lifecycle and restore-executor locks must remain held
	// until every physical SQLite open/migration attempt has returned. A platform
	// call which cannot be interrupted therefore leaves the status-only
	// maintenance handler published instead of becoming an orphan that can race
	// startup reconciliation in another process.
	ctx, cancel := context.WithTimeout(parent, c.timeout.Open)
	defer cancel()
	db, err := c.deps.OpenDatabase(ctx, c.cfg)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		if db != nil {
			if c.deps.CloseDatabase == nil {
				return nil, fmt.Errorf("%w: close dependency is unavailable", errRestoreDatabaseHandleCloseUnproven)
			}
			if closeErr := c.deps.CloseDatabase(db); closeErr != nil {
				return nil, fmt.Errorf("%w: %v", errRestoreDatabaseHandleCloseUnproven, closeErr)
			}
		}
		return nil, err
	}
	return db, nil
}

func restoreCommitSucceeded(err error) bool {
	var postCommit *database.RestorePostCommitError
	return errors.As(err, &postCommit)
}

func (c *restoreCoordinator) rebuildUnchangedActiveLocked(operationID string, cause error) error {
	db, err := c.openDatabaseWithTimeout(context.Background())
	if err != nil {
		_ = c.markRecovery(operationID, "restore_recovery_required", err)
		return err
	}
	generation := c.BuildGeneration(db)
	if generation == nil {
		_ = c.deps.CloseDatabase(db)
		_ = c.markRecovery(operationID, "restore_recovery_required", errors.New("restore generation construction failed"))
		return errors.New("restore generation construction failed")
	}
	healthCtx, cancel := context.WithTimeout(context.Background(), c.timeout.Health)
	healthErr := generation.ValidateRuntimeHealth(healthCtx)
	cancel()
	if healthErr != nil {
		_ = c.deps.CloseDatabase(db)
		_ = c.markRecovery(operationID, "restore_recovery_required", healthErr)
		return healthErr
	}
	if err := c.markFailed(operationID, "restore_safety_copy_failed", cause); err != nil {
		_ = c.deps.CloseDatabase(db)
		_ = c.markRecovery(operationID, "restore_recovery_required", err)
		return err
	}
	c.activateLocked(generation, db, true)
	return nil
}

// replace is the only host-owned runtime generation replacement authority.
// The POST has already returned before this method is reached. The reversible
// failure boundary ends before BeginShutdown; once shutdown starts, the old
// generation is never re-exposed.
func (c *restoreCoordinator) replace(_ context.Context, operationID string) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.shuttingDown {
		return errors.New("restore runtime replacement rejected during host shutdown")
	}
	operation, err := c.deps.ReadOperation(c.cfg.AppDataDir)
	if err != nil {
		return err
	}
	if operation.OperationID != operationID || !operation.AuthorizationCommitted {
		return errors.New("restore runtime replacement rejected without committed authorization")
	}

	oldGeneration, oldDB := c.currentGeneration()
	if oldGeneration == nil || oldDB == nil {
		return errors.New("active runtime generation is unavailable")
	}
	if c.deps.SetHandler != nil {
		c.deps.SetHandler(oldGeneration.RestoreMaintenanceHandler())
	}

	// The app-level admission fence stops new HTTP/job/transcode work, but it
	// cannot prove that every watcher/writer has joined. BeginShutdown cancels
	// those background loops; only after Shutdown returns is the old DB quiet
	// enough for the logical safety snapshot.
	oldGeneration.BeginShutdown()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), c.timeout.Shutdown)
	shutdownErr := oldGeneration.Shutdown(shutdownCtx)
	shutdownCancel()
	if shutdownErr != nil {
		// The generation is irreversibly canceled. Keep maintenance/status only
		// and persist recovery-required rather than presenting a dead runtime.
		_ = c.markRecovery(operationID, "restore_recovery_required", shutdownErr)
		return fmt.Errorf("drain old runtime generation: %w", shutdownErr)
	}
	safetyTimeout := c.timeout.SafetyCopy
	if info, statErr := os.Stat(c.cfg.DatabasePath); statErr == nil && info.Size() > 0 {
		// VACUUM INTO has no portable byte-progress callback. Derive a minimum
		// deadline from the active size at a conservative 8 MiB/s floor, while
		// retaining the configured timeout as the operator's lower bound. A
		// true stalled syscall still keeps the host fail-closed until it returns.
		estimate := 5*time.Minute + time.Duration(info.Size()/(8<<20))*time.Second
		if estimate > safetyTimeout {
			safetyTimeout = estimate
		}
	}
	safetyCtx, safetyCancel := context.WithTimeout(context.Background(), safetyTimeout)
	safetyErr := oldGeneration.PrepareRestoreSafetyCopy(safetyCtx, operationID)
	safetyCancel()
	if safetyErr != nil {
		c.setCurrentLocked(nil, nil)
		authoritativeDB, detachErr := oldGeneration.DetachRestoreDatabaseHandle()
		if detachErr != nil {
			_ = c.markRecovery(operationID, "restore_recovery_required", detachErr)
			return detachErr
		}
		if err := c.deps.CloseDatabase(authoritativeDB); err != nil {
			_ = c.markRecovery(operationID, "restore_recovery_required", err)
			return err
		}
		// No install occurred. Reopen the unchanged active database in a fresh,
		// verified generation; the canceled old object is never re-exposed.
		return c.rebuildUnchangedActiveLocked(operationID, safetyErr)
	}

	c.setCurrentLocked(nil, nil)
	authoritativeDB, detachErr := oldGeneration.DetachRestoreDatabaseHandle()
	if detachErr != nil {
		_ = c.markRecovery(operationID, "restore_recovery_required", detachErr)
		return fmt.Errorf("detach authoritative old database generation: %w", detachErr)
	}
	if err := c.deps.CloseDatabase(authoritativeDB); err != nil {
		// A close error means release of the old handle is unproven. Do not
		// rename, install, or rollback while SQLite may still hold the active
		// file (especially important for Windows sharing semantics).
		_ = c.markRecovery(operationID, "restore_recovery_required", err)
		return fmt.Errorf("close old database generation: %w", err)
	}
	operation, err = c.deps.ReadOperation(c.cfg.AppDataDir)
	if err != nil {
		_ = c.markRecovery(operationID, "restore_recovery_required", err)
		return err
	}
	installCtx, installCancel := context.WithTimeout(context.Background(), c.timeout.Install)
	installErr := c.deps.InstallRestore(installCtx, c.cfg, &operation)
	installCancel()
	if err := installErr; err != nil {
		if rollbackErr := c.rollbackAndRebuildLocked(operationID, fmt.Errorf("install restored database: %w", err)); rollbackErr != nil {
			return rollbackErr
		}
		return nil
	}
	newDB, err := c.openDatabaseWithTimeout(context.Background())
	if err != nil {
		if errors.Is(err, errRestoreDatabaseHandleCloseUnproven) {
			_ = c.markRecovery(operationID, "restore_recovery_required", err)
			return err
		}
		if rollbackErr := c.rollbackAndRebuildLocked(operationID, fmt.Errorf("open restored database generation: %w", err)); rollbackErr != nil {
			return rollbackErr
		}
		return nil
	}
	newGeneration := c.BuildGeneration(newDB)
	if newGeneration == nil {
		_ = c.deps.CloseDatabase(newDB)
		if rollbackErr := c.rollbackAndRebuildLocked(operationID, errors.New("restore generation construction failed")); rollbackErr != nil {
			return rollbackErr
		}
		return nil
	}
	healthCtx, healthCancel := context.WithTimeout(context.Background(), c.timeout.Health)
	healthErr := newGeneration.CompleteRestoreGeneration(healthCtx, operationID)
	healthCancel()
	if healthErr != nil && !restoreCommitSucceeded(healthErr) {
		_ = c.deps.CloseDatabase(newDB)
		if rollbackErr := c.rollbackAndRebuildLocked(operationID, fmt.Errorf("health-check restored database: %w", healthErr)); rollbackErr != nil {
			return rollbackErr
		}
		return nil
	}
	c.activateLocked(newGeneration, newDB, true)
	return nil
}

// ShutdownGeneration serializes process shutdown against replacement. It
// clears the current pair only after the generation has fully joined and the
// database close has completed.
func (c *restoreCoordinator) ShutdownGeneration(ctx context.Context) error {
	c.lifecycleMu.Lock()
	generation, db := c.currentGeneration()
	if generation == nil && db == nil {
		c.shuttingDown = true
		c.lifecycleMu.Unlock()
		return nil
	}
	// Publish the shutdown gate while the lifecycle lock is held, then release
	// it before waiting on runners. A runner stalled before controller entry
	// may proceed and call replace; the gate makes that call return instead of
	// waiting on this shutdown's later lifecycle reacquisition.
	c.shuttingDown = true
	c.lifecycleMu.Unlock()
	if generation != nil {
		if err := generation.StopRestoreRunners(ctx); err != nil {
			return err
		}
	}
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	// No replacement can begin after the gate. Re-read the authoritative pair
	// after runner join so a previously in-flight replacement is handled by the
	// same serialized close path.
	generation, db = c.currentGeneration()
	if generation == nil && db == nil {
		return nil
	}
	if generation != nil {
		generation.BeginShutdown()
		if err := generation.Shutdown(ctx); err != nil {
			return err
		}
	}
	if db != nil {
		if generation != nil {
			authoritative, detachErr := generation.DetachRestoreDatabaseHandle()
			if detachErr != nil {
				return detachErr
			}
			db = authoritative
		}
		if err := c.deps.CloseDatabase(db); err != nil {
			return err
		}
	}
	c.setCurrentLocked(nil, nil)
	return nil
}
