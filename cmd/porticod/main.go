package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/app"
	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
	"github.com/PorticoMediaServer/portico-server/internal/redaction"
)

var (
	version            = "dev"
	buildNumber        = "0"
	channel            = "development"
	commit             = "unknown"
	builtAt            = "unknown"
	releaseSafetyClass = "development"
)

type runOptions struct {
	service bool
}

func main() {
	if err := validateBuildIdentity(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "build-identity" {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"version": version, "buildNumber": buildNumber, "channel": channel, "commit": commit, "timestamp": builtAt})
		return
	}
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "portico configuration rejected: %v\n", err)
		os.Exit(2)
	}
	// The linker-injected release is the authoritative provenance recorded in
	// newly-created backup manifests. It is not used as a restore compatibility
	// decision; migration identity owns that decision.
	cfg.Release = version
	cfg.BuildNumber = buildNumber
	cfg.BuildChannel = channel
	cfg.BuildCommit = commit
	cfg.BuildTimestamp = builtAt
	logger, closeLog := newLogger(cfg)
	defer closeLog()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("server panicked", "panic", recovered, "stack", string(debug.Stack()))
			os.Exit(2)
		}
	}()

	options, handled := parseArgs(cfg, logger)
	if handled {
		return
	}

	if handled, err := runService(cfg, logger, options); handled {
		if err != nil {
			logger.Error("service stopped", "error", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel, signalName := signalContext(logger)
	defer cancel()
	if err := run(ctx, cfg, logger, options); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped", "reason", firstNonEmpty(signalName(), "normal"))
}

func validateBuildIdentity() error {
	if releaseSafetyClass == "development" {
		return nil
	}
	if releaseSafetyClass != "protected" {
		return errors.New("invalid compiled release safety class")
	}
	parsedBuild, buildErr := strconv.ParseUint(buildNumber, 10, 64)
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`).MatchString(version) || buildErr != nil || parsedBuild == 0 || (channel != "production" && channel != "stable" && channel != "beta" && channel != "staging") || !isFullGitCommit(commit) {
		return errors.New("protected Portico build identity is unstamped")
	}
	if _, err := time.Parse(time.RFC3339, builtAt); err != nil {
		return errors.New("protected Portico build timestamp must be RFC3339")
	}
	return nil
}

func isFullGitCommit(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func newLogger(cfg config.Config) (*slog.Logger, func()) {
	output := io.Writer(os.Stdout)
	closeLog := func() {}
	if path := strings.TrimSpace(cfg.LogFilePath); path != "" {
		if err := database.PreparePrivateLogArtifacts(path, 3); err == nil {
			file, err := newRotatingLogWriter(path, 20*1024*1024, 3, database.EnforcePrivateLogArtifacts)
			if err == nil {
				output = io.MultiWriter(os.Stdout, file)
				closeLog = func() {
					_ = file.Close()
				}
			} else {
				fmt.Fprintf(os.Stderr, "portico: unable to open configured log file: %v\n", redaction.Error(err, path))
			}
		} else {
			fmt.Fprintf(os.Stderr, "portico: unable to create configured log directory: %v\n", redaction.Error(err, path))
		}
	}
	return slog.New(redaction.NewHandler(
		slog.NewTextHandler(output, &slog.HandlerOptions{Level: slog.LevelInfo}),
		redaction.Policy{
			SensitivePaths: []string{
				cfg.AppDataDir,
				cfg.ConfigPath,
				cfg.DatabasePath,
				cfg.BackupDir,
				cfg.TranscodeDir,
				cfg.WebDistDir,
				cfg.LogFilePath,
				cfg.FFmpegPath,
				cfg.FFprobePath,
				cfg.FPcalcPath,
			},
			SensitiveValues: []string{cfg.TMDBReadAccessToken, cfg.TMDBAPIKey, cfg.TVDBAPIKey, cfg.AcoustIDAPIKey},
		},
	)), closeLog
}

func parseArgs(cfg config.Config, logger *slog.Logger) (runOptions, bool) {
	var options runOptions
	args := os.Args[1:]
	if len(args) == 0 {
		return options, false
	}
	if args[0] == "restore-backup" {
		logger.Error("restore-backup is disabled; use the authenticated supervised Web restore workflow")
		os.Exit(2)
	}
	for _, arg := range args {
		switch arg {
		case "--service":
			options.service = true
		default:
			logger.Error("unknown command", "command", arg)
			os.Exit(2)
		}
	}
	return options, false
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger, options runOptions) error {
	started := time.Now()
	executable, _ := os.Executable()
	logger.Info(
		"portico server starting",
		"version", version,
		"buildNumber", buildNumber,
		"channel", channel,
		"commit", commit,
		"builtAt", builtAt,
		"go", runtime.Version(),
		"os", runtime.GOOS,
		"arch", runtime.GOARCH,
		"pid", os.Getpid(),
		"ppid", os.Getppid(),
		"executable", redaction.Basename(executable),
		"cwd", "foreign-cwd-independent",
		"addr", cfg.Addr,
		"transport", "shared-http-tls",
		"appData", "private-data",
		"database", "configured-private-database",
		"webDist", "packaged-web-assets",
		"logFile", map[bool]string{true: "configured-log", false: "stdout"}[strings.TrimSpace(cfg.LogFilePath) != ""],
		"service", options.service,
	)
	// Restore filesystem reconciliation must run before any SQLite open. The
	// database package only provides the primitive; the process host owns this
	// lifecycle boundary so tests/embedded callers cannot accidentally perform
	// a process-level generation replacement.
	releaseRestoreExecutor, acquiredRestoreExecutor, executorErr := database.TryAcquireRestoreExecutorLock(cfg)
	if executorErr != nil {
		logger.Error("restore executor lock unavailable", "error", redaction.Error(executorErr, cfg.DatabasePath, cfg.AppDataDir))
		return runRecoveryMaintenanceMode(ctx, cfg, logger)
	}
	if !acquiredRestoreExecutor {
		logger.Warn("another host owns restore recovery; serving status-only maintenance")
		return runRecoveryMaintenanceMode(ctx, cfg, logger)
	}
	restoreExecutorHeld := true
	releaseStartupRestoreExecutor := func() {
		if restoreExecutorHeld {
			releaseRestoreExecutor()
			restoreExecutorHeld = false
		}
	}
	defer releaseStartupRestoreExecutor()
	recoveryCtx, recoveryCancel := context.WithTimeout(ctx, restoreHostIOTimeout(cfg))
	recoveryErr := database.RecoverInterruptedRestoreBeforeOpenContext(recoveryCtx, cfg)
	recoveryCancel()
	if err := recoveryErr; err != nil {
		if operation, readErr := database.ReadRestoreOperation(cfg.AppDataDir); readErr == nil && operation.State != database.RestoreStateRecoveryNeeded {
			_ = database.MarkRestoreRecoveryRequired(cfg, &operation, "restore_recovery_required")
		}
		logger.Error("restore filesystem recovery requires maintenance", "error", redaction.Error(err, cfg.DatabasePath, cfg.AppDataDir))
		releaseStartupRestoreExecutor()
		return runRecoveryMaintenanceMode(ctx, cfg, logger)
	}
	if operation, operationErr := database.ReadRestoreOperation(cfg.AppDataDir); operationErr == nil && operation.State == database.RestoreStateRecoveryNeeded {
		logger.Error("restore recovery is required before database service can resume", "operation", operation.OperationID)
		releaseStartupRestoreExecutor()
		return runRecoveryMaintenanceMode(ctx, cfg, logger)
	}
	logger.Info("database opening", "database", "configured-private-database")
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}
	serverURL := localHTTPURL(listener.Addr().String())
	protocols := newProtocolMuxWithObserver(listener, func(diagnostics protocolMuxDiagnostics) {
		logger.Warn("shared service-port admission saturated", "rejected", diagnostics.AdmissionRejected, "active", diagnostics.Active, "capacity", protocolConnectionLimit)
	})
	certificates := &certificateLoader{}
	startupHandler := newEarlyStartupHandler(started, serverURL)
	startupHandler.completePhase("http_ready", "HTTP listener ready", nil)
	handlerSwitch := newSwitchableHandler(startupHandler)
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           localPlaintextOnly(handlerSwitch),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
		ConnState:         protocolAdmissionConnState,
	}
	tlsServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           tlsAuthorityOnly(handlerSwitch),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
		ConnState:         protocolAdmissionConnState,
		TLSConfig: &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: certificates.GetCertificate,
		},
	}
	serverErr := make(chan error, 2)
	go func() {
		var serveErr error
		if err := httpServer.Serve(protocols.http); err != nil && err != http.ErrServerClosed && !errors.Is(err, net.ErrClosed) {
			serveErr = err
		}
		if serveErr != nil {
			logger.Error("plaintext service stopped unexpectedly", "addr", cfg.Addr, "error", serveErr)
		} else {
			logger.Info("plaintext service stopped", "addr", cfg.Addr)
		}
		serverErr <- serveErr
	}()
	go func() {
		var serveErr error
		if err := tlsServer.ServeTLS(protocols.tls, "", ""); err != nil && err != http.ErrServerClosed && !errors.Is(err, net.ErrClosed) {
			serveErr = err
		}
		if serveErr != nil {
			logger.Error("TLS service stopped unexpectedly", "addr", cfg.Addr, "error", serveErr)
		} else {
			logger.Info("TLS service stopped", "addr", cfg.Addr)
		}
		serverErr <- serveErr
	}()
	logger.Info("portico server listening", "addr", serverURL, "listenAddr", listener.Addr().String(), "transport", "HTTP+HTTPS", "status", "initializing")

	startupHandler.startPhase("database_opening", "Open database")
	openCtx, openCancel := context.WithTimeout(ctx, restoreHostIOTimeout(cfg))
	db, err := database.OpenWithReporterContext(openCtx, cfg, func(phase database.StartupPhase) {
		startupHandler.recordDatabasePhase(phase)
		attrs := []any{
			"phase", phase.ID,
			"label", phase.Label,
			"duration", phase.Duration.String(),
			"durationMs", phase.Duration.Milliseconds(),
		}
		if phase.Error != "" {
			attrs = append(attrs, "error", phase.Error)
			logger.Warn("database startup phase failed", attrs...)
			return
		}
		logger.Info("database startup phase completed", attrs...)
	})
	openCancel()
	if err != nil {
		startupHandler.completePhase("database_opening", "Open database", err)
		if operation, operationErr := database.ReadRestoreOperation(cfg.AppDataDir); operationErr == nil {
			// A crash can leave the installed database at reopening or
			// health-checking before the next normal host open. If the verified
			// logical safety snapshot is still authoritative, roll it back before
			// attempting another open; otherwise keep the listener in truthful
			// recovery-required maintenance rather than boot-looping.
			if restoreOpenFailureCanRollback(operation) {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), restoreHostIOTimeout(cfg))
				rollbackErr := database.RollbackRestoreOperationContext(rollbackCtx, cfg, &operation, "restore_open_failed", err)
				rollbackCancel()
				if rollbackErr == nil {
					reopenCtx, reopenCancel := context.WithTimeout(ctx, restoreHostIOTimeout(cfg))
					db, err = database.OpenWithReporterContext(reopenCtx, cfg, nil)
					reopenCancel()
					if err != nil {
						_ = database.MarkRestoreRecoveryRequired(cfg, &operation, "restore_recovery_required")
						operation.State = database.RestoreStateRecoveryNeeded
					}
				}
				if rollbackErr != nil {
					_ = database.MarkRestoreRecoveryRequired(cfg, &operation, "restore_recovery_required")
				}
			}
			if err == nil {
				startupHandler.completePhase("database_opening", "Open database", nil)
				logger.Warn("restored database open failed; verified safety database reopened", "operation", operation.OperationID)
			} else if operation.State == database.RestoreStateRecoveryNeeded {
				handlerSwitch.Set(app.NewInertServer(cfg, nil, logger).RestoreMaintenanceHandler())
				releaseStartupRestoreExecutor()
				return serveExistingMaintenance(ctx, logger, protocols, httpServer, tlsServer, serverErr, options)
			}
		}
		if err != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = shutdownServers(shutdownCtx, protocols, httpServer, tlsServer)
			cancel()
			<-serverErr
			return fmt.Errorf("database startup failed: %w", err)
		}
	}
	startupHandler.completePhase("database_opening", "Open database", nil)
	logger.Info("database opened", "database", "configured-private-database")

	coordinator := newRestoreCoordinator(cfg, logger, restoreCoordinatorDependencies{
		BuildGeneration: func(nextDB *sql.DB) restoreGeneration {
			return app.NewInertServer(cfg, nextDB, logger)
		},
		OpenDatabase:    database.OpenContext,
		CloseDatabase:   func(nextDB *sql.DB) error { return nextDB.Close() },
		InstallRestore:  database.InstallRestoreOperationContext,
		RollbackRestore: database.RollbackRestoreOperationContext,
		ReadOperation:   database.ReadRestoreOperation,
		MarkRecovery:    database.MarkRestoreRecoveryRequired,
		SetHandler:      handlerSwitch.Set,
		Activate: func(generation restoreGeneration, nextDB *sql.DB, expose bool) {
			certificates.Set(generation.RemoteAccessCertificateLoader())
			generation.StartRemoteAccessManager()
			generation.ActivateRuntimeGeneration()
			generation.MarkHTTPReady(serverURL)
			generation.StartDeferredStartupMaintenance()
			if expose {
				handlerSwitch.Set(generation.Handler())
			}
		},
	})
	portico := coordinator.BuildGeneration(db)
	if portico == nil {
		_ = db.Close()
		return errors.New("database runtime generation construction failed")
	}
	// Establish the authoritative host pair before any startup health or
	// recovered-marker decision can request a coordinator rollback. Activation
	// (workers, remote access, and exposure) remains deferred until every health
	// gate has passed.
	coordinator.setCurrentLocked(portico, db)
	activeGeneration := false
	startupHealthCtx, startupHealthCancel := context.WithTimeout(context.Background(), restoreHostHealthTimeout())
	startupHealthErr := portico.ValidateRuntimeHealth(startupHealthCtx)
	startupHealthCancel()
	if startupHealthErr != nil {
		if operation, operationErr := database.ReadRestoreOperation(cfg.AppDataDir); operationErr == nil && operation.OperationID != "" {
			_ = db.Close()
			coordinator.lifecycleMu.Lock()
			rollbackErr := coordinator.rollbackAndRebuildLocked(operation.OperationID, startupHealthErr)
			coordinator.lifecycleMu.Unlock()
			if rollbackErr == nil {
				portico, db = coordinator.currentGeneration()
				activeGeneration = true
			} else {
				handlerSwitch.Set(portico.RestoreMaintenanceHandler())
				logger.Error("restore startup health and rollback failed; keeping maintenance responder", "error", redaction.Error(rollbackErr, cfg.DatabasePath, cfg.AppDataDir))
				return serveExistingMaintenance(ctx, logger, protocols, httpServer, tlsServer, serverErr, options)
			}
		} else {
			_ = db.Close()
			return fmt.Errorf("database health check failed: %w", startupHealthErr)
		}
	}
	if operation, operationErr := database.ReadRestoreOperation(cfg.AppDataDir); operationErr == nil {
		if operation.State == database.RestoreStateRecoveryNeeded {
			_ = db.Close()
			handlerSwitch.Set(portico.RestoreMaintenanceHandler())
			logger.Error("restore recovery is required; keeping maintenance responder", "operation", operation.OperationID)
			releaseStartupRestoreExecutor()
			return serveExistingMaintenance(ctx, logger, protocols, httpServer, tlsServer, serverErr, options)
		}
		if operation.Phase == database.RestorePhaseHealthChecking || operation.Phase == database.RestorePhaseReopening {
			healthCtx, healthCancel := context.WithTimeout(context.Background(), restoreHostHealthTimeout())
			healthErr := portico.CompleteRestoreGeneration(healthCtx, operation.OperationID)
			healthCancel()
			if healthErr != nil {
				_ = db.Close()
				coordinator.lifecycleMu.Lock()
				rollbackErr := coordinator.rollbackAndRebuildLocked(operation.OperationID, healthErr)
				coordinator.lifecycleMu.Unlock()
				if rollbackErr != nil {
					handlerSwitch.Set(portico.RestoreMaintenanceHandler())
					logger.Error("restore handoff health and rollback failed; keeping maintenance responder", "operation", operation.OperationID, "error", redaction.Error(rollbackErr, cfg.DatabasePath, cfg.AppDataDir))
					releaseStartupRestoreExecutor()
					return serveExistingMaintenance(ctx, logger, protocols, httpServer, tlsServer, serverErr, options)
				}
				portico, db = coordinator.currentGeneration()
				activeGeneration = true
			}
		}
	}
	if !activeGeneration {
		coordinator.Activate(portico, db, false)
	}
	// The pre-open transition is now complete: recovery reconciliation, the
	// single host open/migration, application health, and any durable marker
	// completion all occurred while this process held executor.lock. A second
	// host may now perform ordinary startup.
	releaseStartupRestoreExecutor()
	handlerSwitch.Set(portico.Handler())
	portico.ResumeRestoreOperation()
	logger.Info("portico application ready", "addr", serverURL, "listenAddr", listener.Addr().String())
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := coordinator.ShutdownGeneration(closeCtx); err != nil {
			logger.Error("runtime generation shutdown failed", "error", redaction.Error(err, cfg.DatabasePath, cfg.AppDataDir))
		} else {
			logger.Info("database closed")
		}
		closeCancel()
	}()

	var shutdownOnce sync.Once
	shutdownStarted := make(chan struct{})
	shutdown := func() {
		shutdownOnce.Do(func() {
			close(shutdownStarted)
			shutdownStart := time.Now()
			logger.Info("server shutdown starting", "uptime", time.Since(started).String())
			// Listener/SSE shutdown has its own budget. A slow client must not
			// consume the generation's job cancellation/join deadline before the
			// database-close safety boundary is reached.
			httpShutdownCtx, httpCancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := shutdownServers(httpShutdownCtx, protocols, httpServer, tlsServer); err != nil {
				logger.Error("graceful shutdown failed", "error", err)
				_ = httpServer.Close()
				_ = tlsServer.Close()
			} else {
				logger.Info("shared service-port shutdown completed")
			}
			listenerDiagnostics := protocols.diagnostics()
			logger.Info("shared service-port admission summary",
				"accepted", listenerDiagnostics.Accepted,
				"active", listenerDiagnostics.Active,
				"peakActive", listenerDiagnostics.PeakActive,
				"http", listenerDiagnostics.ClassifiedHTTP,
				"tls", listenerDiagnostics.ClassifiedTLS,
				"classificationFailed", listenerDiagnostics.ClassificationFailed,
				"classificationTimedOut", listenerDiagnostics.ClassificationTimedOut,
				"admissionRejected", listenerDiagnostics.AdmissionRejected,
			)
			httpCancel()
			generationShutdownCtx, generationCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := coordinator.ShutdownGeneration(generationShutdownCtx); err != nil {
				logger.Error("app background shutdown failed", "error", redaction.Error(err, cfg.DatabasePath, cfg.AppDataDir))
			} else {
				logger.Info("app background shutdown completed")
			}
			generationCancel()
			logger.Info("server shutdown finished", "duration", time.Since(shutdownStart).String(), "uptime", time.Since(started).String())
		})
	}

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown requested", "reason", contextCause(ctx))
		shutdown()
		select {
		case err := <-serverErr:
			return err
		case <-shutdownStarted:
			err := <-serverErr
			return err
		}
	}
}

// runRecoveryMaintenanceMode deliberately has no SQLite dependency. It keeps
// the listener and the opaque restore-status capability available after a
// restart where the active database is missing/corrupt but the durable marker
// says recovery is required. No normal application handler is installed.
func runRecoveryMaintenanceMode(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen for restore recovery maintenance: %w", err)
	}
	serverURL := localHTTPURL(listener.Addr().String())
	protocols := newProtocolMuxWithObserver(listener, func(diagnostics protocolMuxDiagnostics) {
		logger.Warn("restore maintenance listener admission saturated", "rejected", diagnostics.AdmissionRejected, "active", diagnostics.Active, "capacity", protocolConnectionLimit)
	})
	certificates := &certificateLoader{}
	maintenance := app.NewInertServer(cfg, nil, logger).RestoreMaintenanceHandler()
	handlerSwitch := newSwitchableHandler(maintenance)
	httpServer := &http.Server{Addr: cfg.Addr, Handler: localPlaintextOnly(handlerSwitch), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20, ConnState: protocolAdmissionConnState}
	tlsServer := &http.Server{Addr: cfg.Addr, Handler: tlsAuthorityOnly(handlerSwitch), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20, ConnState: protocolAdmissionConnState, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: certificates.GetCertificate}}
	serverErr := make(chan error, 2)
	go func() {
		err := httpServer.Serve(protocols.http)
		if err != nil && err != http.ErrServerClosed && !errors.Is(err, net.ErrClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()
	go func() {
		err := tlsServer.ServeTLS(protocols.tls, "", "")
		if err != nil && err != http.ErrServerClosed && !errors.Is(err, net.ErrClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()
	logger.Warn("restore recovery maintenance responder listening", "addr", serverURL)
	select {
	case err := <-serverErr:
		_ = httpServer.Close()
		_ = tlsServer.Close()
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = shutdownServers(shutdownCtx, protocols, httpServer, tlsServer)
		cancel()
		return nil
	}
}

// serveExistingMaintenance keeps an already-created listener alive while the
// host has no verified runtime generation. It is used after startup/open
// failures so a status capability remains observable until a signal or a
// listener failure, rather than letting run return and taking the process
// down immediately.
func serveExistingMaintenance(ctx context.Context, logger *slog.Logger, protocols *protocolMux, httpServer, tlsServer *http.Server, serverErr <-chan error, options runOptions) error {
	select {
	case err := <-serverErr:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = shutdownServers(shutdownCtx, protocols, httpServer, tlsServer)
		cancel()
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := shutdownServers(shutdownCtx, protocols, httpServer, tlsServer); err != nil && logger != nil {
			logger.Error("maintenance responder shutdown failed", "error", redaction.Error(err))
		}
		cancel()
		return nil
	}
}

func signalContext(logger *slog.Logger) (context.Context, context.CancelFunc, func() string) {
	ctx, cancel := context.WithCancel(context.Background())
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	var mu sync.Mutex
	name := ""
	go func() {
		sig := <-signalCh
		mu.Lock()
		name = sig.String()
		mu.Unlock()
		logger.Warn("shutdown signal received", "signal", sig.String())
		cancel()
	}()
	return ctx, func() {
			signal.Stop(signalCh)
			cancel()
		}, func() string {
			mu.Lock()
			defer mu.Unlock()
			return name
		}
}

func contextCause(ctx context.Context) string {
	if err := ctx.Err(); err != nil {
		return err.Error()
	}
	return "unknown"
}

func restoreOpenFailureCanRollback(operation database.RestoreOperation) bool {
	if operation.State == database.RestoreStateRecoveryNeeded || operation.RollbackPendingHealth {
		return operation.SafetyCopySizeBytes > 0 && strings.TrimSpace(operation.SafetyCopyChecksumSHA256) != ""
	}
	switch operation.Phase {
	case database.RestorePhaseInstalling, database.RestorePhaseReopening, database.RestorePhaseHealthChecking, database.RestorePhaseRollingBack:
		return operation.ActiveMutationStarted || operation.ActiveMutationCompleted
	default:
		return false
	}
}

func restoreHostIOTimeout(cfg config.Config) time.Duration {
	if cfg.RestoreIOTimeout > 0 {
		return cfg.RestoreIOTimeout
	}
	return 10 * time.Minute
}

func restoreHostHealthTimeout() time.Duration {
	// Health probes are short bounded application checks, separate from the
	// size/progress-aware filesystem I/O deadline used for open, recovery, and
	// rollback. Native slow-I/O evidence remains a platform gate.
	return 45 * time.Second
}

type switchableHandler struct {
	mu      sync.RWMutex
	handler http.Handler
}

func newSwitchableHandler(initial http.Handler) *switchableHandler {
	return &switchableHandler{handler: initial}
}

func (h *switchableHandler) Set(next http.Handler) {
	if next == nil {
		return
	}
	h.mu.Lock()
	h.handler = next
	h.mu.Unlock()
}

func (h *switchableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	handler := h.handler
	h.mu.RUnlock()
	if handler == nil {
		http.Error(w, "Portico is starting.", http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}

type earlyStartupHandler struct {
	mu        sync.Mutex
	startedAt time.Time
	httpAddr  string
	phases    map[string]earlyStartupPhase
}

type earlyStartupPhase struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Status      string `json:"status"`
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	DurationMS  int64  `json:"durationMillis,omitempty"`
	Error       string `json:"error,omitempty"`
}

func newEarlyStartupHandler(startedAt time.Time, httpAddr string) *earlyStartupHandler {
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	return &earlyStartupHandler{
		startedAt: startedAt.UTC(),
		httpAddr:  strings.TrimSpace(httpAddr),
		phases:    map[string]earlyStartupPhase{},
	}
}

func (h *earlyStartupHandler) startPhase(id, label string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.phases[id] = earlyStartupPhase{
		ID:        id,
		Label:     firstNonEmpty(label, id),
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (h *earlyStartupHandler) completePhase(id, label string, phaseErr error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now().UTC()
	phase := h.phases[id]
	if phase.ID == "" {
		phase.ID = id
		phase.StartedAt = now.Format(time.RFC3339Nano)
	}
	phase.Label = firstNonEmpty(label, phase.Label, id)
	phase.Status = "complete"
	if phaseErr != nil {
		phase.Status = "failed"
		phase.Error = phaseErr.Error()
	}
	phase.CompletedAt = now.Format(time.RFC3339Nano)
	if startedAt, err := time.Parse(time.RFC3339Nano, phase.StartedAt); err == nil {
		phase.DurationMS = now.Sub(startedAt).Milliseconds()
	}
	h.phases[id] = phase
}

func (h *earlyStartupHandler) recordDatabasePhase(phase database.StartupPhase) {
	if phase.ID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	status := "complete"
	if phase.Error != "" {
		status = "failed"
	}
	h.phases["database_"+phase.ID] = earlyStartupPhase{
		ID:          "database_" + phase.ID,
		Label:       "Database: " + firstNonEmpty(phase.Label, phase.ID),
		Status:      status,
		StartedAt:   phase.StartedAt.UTC().Format(time.RFC3339Nano),
		CompletedAt: phase.CompletedAt.UTC().Format(time.RFC3339Nano),
		DurationMS:  phase.Duration.Milliseconds(),
		Error:       phase.Error,
	}
}

func (h *earlyStartupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/readiness":
		w.Header().Set("Retry-After", "2")
		writeEarlyStartupJSON(w, http.StatusServiceUnavailable, map[string]any{"ready": false, "status": "starting"})
	default:
		w.Header().Set("Retry-After", "2")
		writeEarlyStartupJSON(w, http.StatusServiceUnavailable, map[string]any{
			"type":   "https://portico.media/problems/starting",
			"title":  "Service Unavailable",
			"status": http.StatusServiceUnavailable,
			"code":   "starting",
			"detail": "Portico is starting. Try again shortly.",
		})
	}
}

func (h *earlyStartupHandler) snapshot() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	phases := make([]earlyStartupPhase, 0, len(h.phases))
	for _, phase := range h.phases {
		phases = append(phases, phase)
	}
	sort.SliceStable(phases, func(i, j int) bool {
		if phases[i].StartedAt == phases[j].StartedAt {
			return phases[i].ID < phases[j].ID
		}
		return phases[i].StartedAt < phases[j].StartedAt
	})
	failed := false
	for _, phase := range phases {
		if phase.Status == "failed" {
			failed = true
			break
		}
	}
	status := "serving_degraded"
	if failed {
		status = "degraded"
	}
	return map[string]any{
		"status":               status,
		"startedAt":            h.startedAt.Format(time.RFC3339Nano),
		"httpReady":            true,
		"httpReadyAt":          h.startedAt.Format(time.RFC3339Nano),
		"httpAddr":             h.httpAddr,
		"nonCriticalWorkReady": false,
		"phases":               phases,
	}
}

func writeEarlyStartupJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func emptyLabel(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func localHTTPURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + strings.TrimPrefix(addr, "http://")
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") {
		return "http://" + net.JoinHostPort(host, port)
	}
	return "http://" + host + ":" + port
}
