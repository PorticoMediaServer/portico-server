package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/app"
	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

type coordinatorFakeGeneration struct {
	name              string
	events            *[]string
	eventMu           *sync.Mutex
	maintenanceStatus int
	detachDB          *sql.DB
	detachErr         error
	shutdownErr       error
	healthErr         error
	completeErr       error
	stopRunners       func() error
	beginStopped      bool
}

func (g *coordinatorFakeGeneration) record(event string) {
	if g.eventMu != nil {
		g.eventMu.Lock()
		defer g.eventMu.Unlock()
	}
	if g.events != nil {
		*g.events = append(*g.events, g.name+":"+event)
	}
}

func (g *coordinatorFakeGeneration) SetRestoreRuntimeController(app.RestoreRuntimeController) {
	g.record("controller")
}

func (g *coordinatorFakeGeneration) ActivateRuntimeGeneration() { g.record("runtime-activate") }

func (g *coordinatorFakeGeneration) DetachRestoreDatabaseHandle() (*sql.DB, error) {
	g.record("detach")
	if g.detachErr != nil {
		return nil, g.detachErr
	}
	if g.detachDB == nil {
		g.detachDB = &sql.DB{}
	}
	return g.detachDB, nil
}

func (g *coordinatorFakeGeneration) Handler() http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
}

func (g *coordinatorFakeGeneration) RestoreMaintenanceHandler() http.Handler {
	g.record("maintenance-handler")
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if g.maintenanceStatus != 0 {
			w.WriteHeader(g.maintenanceStatus)
		}
	})
}

func (g *coordinatorFakeGeneration) BeginShutdown() {
	g.beginStopped = true
	g.record("begin-shutdown")
}

func (g *coordinatorFakeGeneration) StopRestoreRunners(context.Context) error {
	g.record("stop-runners")
	if g.stopRunners != nil {
		return g.stopRunners()
	}
	return nil
}

func (g *coordinatorFakeGeneration) Shutdown(context.Context) error {
	g.record("shutdown")
	return g.shutdownErr
}

func (g *coordinatorFakeGeneration) PrepareRestoreSafetyCopy(context.Context, string) error {
	g.record("safety-copy")
	return nil
}

func (g *coordinatorFakeGeneration) ValidateRuntimeHealth(context.Context) error {
	g.record("health")
	return g.healthErr
}

func (g *coordinatorFakeGeneration) CompleteRestoreGeneration(context.Context, string) error {
	g.record("complete")
	return g.completeErr
}

func (g *coordinatorFakeGeneration) RestoreRuntimeFailure() { g.record("runtime-failure") }

func (g *coordinatorFakeGeneration) StartRemoteAccessManager() { g.record("remote-start") }

func (g *coordinatorFakeGeneration) RemoteAccessCertificateLoader() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return nil
}

func (g *coordinatorFakeGeneration) MarkHTTPReady(string) { g.record("http-ready") }

func (g *coordinatorFakeGeneration) StartDeferredStartupMaintenance() { g.record("deferred-start") }

func (g *coordinatorFakeGeneration) ResumeRestoreOperation() { g.record("resume") }

type coordinatorTestState struct {
	mu            sync.Mutex
	eventMu       sync.Mutex
	events        []string
	openErrors    []error
	closeError    error
	installError  error
	rollbackError error
	built         []*coordinatorFakeGeneration
	closedDB      []*sql.DB
	installed     int
	rolledBack    int
	activated     int
	setHandlers   int
	expectedClose *sql.DB
}

func (s *coordinatorTestState) record(event string) {
	s.eventMu.Lock()
	s.events = append(s.events, event)
	s.eventMu.Unlock()
}

func (s *coordinatorTestState) snapshotEvents() []string {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	return append([]string(nil), s.events...)
}

func newCoordinatorTest(t *testing.T) (*restoreCoordinator, *coordinatorTestState, config.Config, *coordinatorFakeGeneration, database.RestoreOperation) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{AppDataDir: filepath.Join(root, "app-data"), DatabasePath: filepath.Join(root, "portico.db")}
	if err := database.PreparePrivateDataPaths(cfg); err != nil {
		t.Fatalf("prepare private paths: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	operation := database.RestoreOperation{
		Version: database.RestoreOperationVersion, OperationID: "restore-coordinator-test", BackupName: "backup.db",
		AuthorizationCommitted: true,
		SourcePath:             filepath.Join(cfg.AppDataDir, "backups", "backup.db"),
		StagedPath:             database.CanonicalRestoreStagedPath(cfg, "restore-coordinator-test", false),
		ActivePath:             cfg.DatabasePath,
		SafetyCopyPath:         database.CanonicalRestoreSafetyCopyPath(cfg, "restore-coordinator-test"),
		OldActivePath:          database.CanonicalRestoreOldActivePath(cfg, "restore-coordinator-test"),
		InstallPath:            database.CanonicalRestoreInstallPath(cfg, "restore-coordinator-test"),
		Phase:                  database.RestorePhaseValidating, State: database.RestorePhaseValidating, Progress: 10,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := database.WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
		t.Fatalf("write operation: %v", err)
	}
	state := &coordinatorTestState{}
	old := &coordinatorFakeGeneration{name: "old", events: &state.events, eventMu: &state.eventMu, detachDB: &sql.DB{}}
	state.expectedClose = old.detachDB
	deps := restoreCoordinatorDependencies{
		BuildGeneration: func(*sql.DB) restoreGeneration {
			state.mu.Lock()
			index := len(state.built)
			name := fmt.Sprintf("fresh-%d", index)
			generation := &coordinatorFakeGeneration{name: name, events: &state.events, eventMu: &state.eventMu}
			state.built = append(state.built, generation)
			state.mu.Unlock()
			return generation
		},
		OpenDatabase: func(context.Context, config.Config) (*sql.DB, error) {
			state.record("open")
			state.mu.Lock()
			defer state.mu.Unlock()
			if len(state.openErrors) > 0 {
				err := state.openErrors[0]
				state.openErrors = state.openErrors[1:]
				if err != nil {
					return nil, err
				}
			}
			return &sql.DB{}, nil
		},
		CloseDatabase: func(db *sql.DB) error {
			state.mu.Lock()
			state.closedDB = append(state.closedDB, db)
			err := state.closeError
			state.mu.Unlock()
			state.record("close")
			return err
		},
		InstallRestore: func(context.Context, config.Config, *database.RestoreOperation) error {
			state.mu.Lock()
			state.installed++
			err := state.installError
			state.mu.Unlock()
			state.record("install")
			return err
		},
		RollbackRestore: func(context.Context, config.Config, *database.RestoreOperation, string, error) error {
			state.mu.Lock()
			state.rolledBack++
			err := state.rollbackError
			state.mu.Unlock()
			state.record("rollback")
			return err
		},
		ReadOperation: database.ReadRestoreOperation,
		MarkRecovery: func(cfg config.Config, operation *database.RestoreOperation, code string) error {
			state.record("recovery:" + code)
			return database.MarkRestoreRecoveryRequired(cfg, operation, code)
		},
		Activate: func(g restoreGeneration, _ *sql.DB, expose bool) {
			state.mu.Lock()
			state.activated++
			state.mu.Unlock()
			state.record(fmt.Sprintf("activate:%t", expose))
			g.ActivateRuntimeGeneration()
		},
		SetHandler: func(http.Handler) {
			state.mu.Lock()
			state.setHandlers++
			state.mu.Unlock()
			state.record("handler")
		},
	}
	coordinator := newRestoreCoordinator(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), deps)
	coordinator.timeout = restoreCoordinatorTimeouts{
		SafetyCopy: time.Second, Shutdown: time.Second, Install: time.Second,
		Open: time.Second, Health: time.Second, Rollback: time.Second,
	}
	coordinator.setCurrentLocked(old, &sql.DB{})
	return coordinator, state, cfg, old, operation
}

func TestRestoreCoordinatorRejectsUncommittedAuthorizationBeforeShutdown(t *testing.T) {
	coordinator, state, cfg, _, operation := newCoordinatorTest(t)
	operation.AuthorizationCommitted = false
	if err := database.WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.replace(context.Background(), operation.OperationID); err == nil {
		t.Fatal("coordinator accepted an operation whose authorization commit was not confirmed")
	}
	if events := state.snapshotEvents(); len(events) != 0 {
		t.Fatalf("uncommitted authorization changed the active runtime: %v", events)
	}
}

func eventIndex(events []string, prefix string) int {
	for index, event := range events {
		if event == prefix {
			return index
		}
	}
	return -1
}

func TestRestoreCoordinatorDetachesAuthoritativeCurrentHandleBeforeInstall(t *testing.T) {
	coordinator, state, _, old, operation := newCoordinatorTest(t)
	if err := coordinator.replace(context.Background(), operation.OperationID); err != nil {
		t.Fatalf("replace: %v", err)
	}
	events := state.snapshotEvents()
	for _, pair := range [][2]string{{"old:shutdown", "old:safety-copy"}, {"old:safety-copy", "old:detach"}, {"old:detach", "close"}, {"close", "install"}, {"install", "open"}} {
		if eventIndex(events, pair[0]) < 0 || eventIndex(events, pair[1]) < eventIndex(events, pair[0]) {
			t.Fatalf("event order %s before %s: %v", pair[0], pair[1], events)
		}
	}
	if len(state.closedDB) != 1 || state.closedDB[0] != old.detachDB {
		t.Fatalf("closed DB=%p, want authoritative detached DB=%p", state.closedDB[0], old.detachDB)
	}
	if state.installed != 1 || state.rolledBack != 0 || state.activated != 1 {
		t.Fatalf("replacement counts installed=%d rolledBack=%d activated=%d", state.installed, state.rolledBack, state.activated)
	}
	if old.beginStopped == false {
		t.Fatal("old generation was not irreversibly stopped")
	}
}

func TestRestoreCoordinatorCloseFailureNeverMutatesFilesystem(t *testing.T) {
	coordinator, state, cfg, _, operation := newCoordinatorTest(t)
	state.closeError = errors.New("close refused")
	if err := coordinator.replace(context.Background(), operation.OperationID); err == nil {
		t.Fatal("close failure unexpectedly succeeded")
	}
	if state.installed != 0 || state.rolledBack != 0 {
		t.Fatalf("close failure mutated restore: installed=%d rolledBack=%d", state.installed, state.rolledBack)
	}
	latest, err := database.ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatalf("read recovery marker: %v", err)
	}
	if latest.State != database.RestoreStateRecoveryNeeded {
		t.Fatalf("close failure state=%q, want recovery-required", latest.State)
	}
}

func TestRestoreCoordinatorOpenTimeoutCooperativeAndNoncooperative(t *testing.T) {
	t.Run("cooperative", func(t *testing.T) {
		coordinator, _, _, _, _ := newCoordinatorTest(t)
		coordinator.timeout.Open = 15 * time.Millisecond
		started := make(chan struct{})
		coordinator.deps.OpenDatabase = func(ctx context.Context, _ config.Config) (*sql.DB, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		opened, err := coordinator.openDatabaseWithTimeout(context.Background())
		if opened != nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cooperative timeout db=%v err=%v", opened, err)
		}
		select {
		case <-started:
		default:
			t.Fatal("cooperative open was not started")
		}
	})

	t.Run("noncooperative", func(t *testing.T) {
		coordinator, state, _, _, _ := newCoordinatorTest(t)
		coordinator.timeout.Open = 15 * time.Millisecond
		started := make(chan struct{})
		deadlineObserved := make(chan struct{})
		release := make(chan struct{})
		released := false
		defer func() {
			if !released {
				close(release)
			}
		}()
		coordinator.deps.OpenDatabase = func(ctx context.Context, _ config.Config) (*sql.DB, error) {
			close(started)
			<-ctx.Done()
			close(deadlineObserved)
			<-release
			return &sql.DB{}, nil
		}
		type openResult struct {
			db  *sql.DB
			err error
		}
		result := make(chan openResult, 1)
		go func() {
			db, err := coordinator.openDatabaseWithTimeout(context.Background())
			result <- openResult{db: db, err: err}
		}()
		<-started
		<-deadlineObserved
		select {
		case outcome := <-result:
			t.Fatalf("noncooperative open returned before its physical attempt: db=%v err=%v", outcome.db, outcome.err)
		default:
		}
		close(release)
		released = true
		select {
		case outcome := <-result:
			if outcome.db != nil || !errors.Is(outcome.err, context.DeadlineExceeded) {
				t.Fatalf("noncooperative timeout db=%v err=%v", outcome.db, outcome.err)
			}
		case <-time.After(time.Second):
			t.Fatal("noncooperative open did not return after its physical attempt")
		}
		state.mu.Lock()
		closed := len(state.closedDB)
		state.mu.Unlock()
		if closed != 1 {
			t.Fatalf("expired noncooperative open closed handles=%d, want 1", closed)
		}
	})
}

func TestRestoreCoordinatorNoncooperativeOpenKeepsMaintenanceUntilRollbackRebuild(t *testing.T) {
	coordinator, state, _, old, operation := newCoordinatorTest(t)
	coordinator.timeout.Open = 15 * time.Millisecond
	old.maintenanceStatus = http.StatusServiceUnavailable

	var handlerMu sync.Mutex
	var currentHandler http.Handler
	setHandler := func(handler http.Handler) {
		handlerMu.Lock()
		currentHandler = handler
		handlerMu.Unlock()
		state.record("handler")
	}
	coordinator.deps.SetHandler = setHandler
	originalActivate := coordinator.deps.Activate
	coordinator.deps.Activate = func(generation restoreGeneration, db *sql.DB, expose bool) {
		originalActivate(generation, db, expose)
		if expose {
			setHandler(generation.Handler())
		}
	}

	openStarted := make(chan struct{})
	openDeadlineObserved := make(chan struct{})
	releaseOpen := make(chan struct{})
	openReleased := false
	defer func() {
		if !openReleased {
			close(releaseOpen)
		}
	}()
	openCalls := 0
	coordinator.deps.OpenDatabase = func(ctx context.Context, _ config.Config) (*sql.DB, error) {
		state.mu.Lock()
		openCalls++
		call := openCalls
		state.mu.Unlock()
		state.record("open")
		if call == 1 {
			close(openStarted)
			<-ctx.Done()
			close(openDeadlineObserved)
			<-releaseOpen
		}
		return &sql.DB{}, nil
	}

	replaceDone := make(chan error, 1)
	go func() { replaceDone <- coordinator.replace(context.Background(), operation.OperationID) }()
	<-openStarted
	<-openDeadlineObserved
	select {
	case err := <-replaceDone:
		t.Fatalf("replacement returned while the physical database open remained active: %v", err)
	default:
	}

	handlerMu.Lock()
	maintenance := currentHandler
	handlerMu.Unlock()
	if maintenance == nil {
		t.Fatal("replacement did not publish the maintenance handler before opening the restored database")
	}
	recorder := httptest.NewRecorder()
	maintenance.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/backups/restore/"+operation.OperationID, nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("handler during blocked open status=%d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	state.mu.Lock()
	rolledBackWhileOpen, activatedWhileOpen := state.rolledBack, state.activated
	state.mu.Unlock()
	if rolledBackWhileOpen != 0 || activatedWhileOpen != 0 {
		t.Fatalf("blocked open published transition: rolledBack=%d activated=%d", rolledBackWhileOpen, activatedWhileOpen)
	}

	close(releaseOpen)
	openReleased = true
	select {
	case err := <-replaceDone:
		if err != nil {
			t.Fatalf("replacement after synchronous timeout rollback: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("replacement did not complete after the physical database open returned")
	}
	state.mu.Lock()
	rolledBack, activated, calls := state.rolledBack, state.activated, openCalls
	state.mu.Unlock()
	if rolledBack != 1 || activated != 1 || calls != 2 {
		t.Fatalf("replacement recovery rolledBack=%d activated=%d openCalls=%d, want 1/1/2", rolledBack, activated, calls)
	}

	handlerMu.Lock()
	active := currentHandler
	handlerMu.Unlock()
	recorder = httptest.NewRecorder()
	active.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/home", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("rebuilt generation handler status=%d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestRestoreCoordinatorClosesDatabaseReturnedWithOpenError(t *testing.T) {
	coordinator, state, _, _, _ := newCoordinatorTest(t)
	opened := &sql.DB{}
	coordinator.deps.OpenDatabase = func(context.Context, config.Config) (*sql.DB, error) {
		return opened, errors.New("migration failed after opening")
	}
	_, err := coordinator.openDatabaseWithTimeout(context.Background())
	if err == nil {
		t.Fatal("open error unexpectedly succeeded")
	}
	if len(state.closedDB) != 1 || state.closedDB[0] != opened {
		t.Fatalf("open-error handle close=%v, want %p", state.closedDB, opened)
	}
}

func TestRestoreCoordinatorExpiredOpenCloseFailureDoesNotRollback(t *testing.T) {
	coordinator, state, cfg, _, operation := newCoordinatorTest(t)
	coordinator.timeout.Open = 15 * time.Millisecond
	lateDB := &sql.DB{}
	coordinator.deps.OpenDatabase = func(ctx context.Context, _ config.Config) (*sql.DB, error) {
		<-ctx.Done()
		return lateDB, nil
	}
	originalClose := coordinator.deps.CloseDatabase
	coordinator.deps.CloseDatabase = func(db *sql.DB) error {
		if db == lateDB {
			state.record("late-close-failed")
			return errors.New("late handle close refused")
		}
		return originalClose(db)
	}

	err := coordinator.replace(context.Background(), operation.OperationID)
	if !errors.Is(err, errRestoreDatabaseHandleCloseUnproven) {
		t.Fatalf("expired open close error=%v, want unproven-handle classification", err)
	}
	state.mu.Lock()
	installed, rolledBack, activated := state.installed, state.rolledBack, state.activated
	state.mu.Unlock()
	if installed != 1 || rolledBack != 0 || activated != 0 {
		t.Fatalf("unproven late close crossed filesystem/runtime boundary: installed=%d rolledBack=%d activated=%d", installed, rolledBack, activated)
	}
	latest, readErr := database.ReadRestoreOperation(cfg.AppDataDir)
	if readErr != nil {
		t.Fatalf("read recovery marker: %v", readErr)
	}
	if latest.State != database.RestoreStateRecoveryNeeded {
		t.Fatalf("unproven late close state=%q, want recovery-required", latest.State)
	}
}

func TestRestoreCoordinatorOpenAndHealthFailureRollbackRebuildsSafetyGeneration(t *testing.T) {
	t.Run("open failure", func(t *testing.T) {
		coordinator, state, cfg, _, operation := newCoordinatorTest(t)
		state.openErrors = []error{errors.New("restored database cannot open"), nil}
		if err := coordinator.replace(context.Background(), operation.OperationID); err != nil {
			t.Fatalf("open-failure replacement: %v", err)
		}
		if state.rolledBack != 1 || state.activated != 1 {
			t.Fatalf("open-failure rollback counts rolledBack=%d activated=%d", state.rolledBack, state.activated)
		}
		latest, err := database.ReadRestoreOperation(cfg.AppDataDir)
		if err != nil {
			t.Fatal(err)
		}
		if latest.Phase != database.RestorePhaseFailed || latest.State == database.RestoreStateRecoveryNeeded {
			t.Fatalf("open-failure marker=%#v", latest)
		}
	})

	t.Run("health failure", func(t *testing.T) {
		coordinator, state, _, _, operation := newCoordinatorTest(t)
		state.built = []*coordinatorFakeGeneration{{name: "placeholder", events: &state.events, eventMu: &state.eventMu}}
		// The first generation returned by BuildGeneration is the restored one;
		// make its completion probe fail. The rollback generation is healthy.
		state.built = nil
		buildCount := 0
		coordinator.deps.BuildGeneration = func(db *sql.DB) restoreGeneration {
			buildCount++
			generation := &coordinatorFakeGeneration{name: fmt.Sprintf("health-%d", buildCount), events: &state.events, eventMu: &state.eventMu}
			if buildCount == 1 {
				generation.completeErr = errors.New("restored application health failed")
			}
			return generation
		}
		if err := coordinator.replace(context.Background(), operation.OperationID); err != nil {
			t.Fatalf("health-failure replacement: %v", err)
		}
		if state.rolledBack != 1 || state.activated != 1 {
			t.Fatalf("health-failure rollback counts rolledBack=%d activated=%d", state.rolledBack, state.activated)
		}
		events := state.snapshotEvents()
		if eventIndex(events, "health-2:health") < 0 || eventIndex(events, "health-2:complete") >= 0 {
			t.Fatalf("rollback generation was incorrectly normal-completed: %v", events)
		}
	})
}

func TestRestoreCoordinatorPostCommitWarningActivatesWithoutRollback(t *testing.T) {
	coordinator, state, _, _, operation := newCoordinatorTest(t)
	state.built = nil
	coordinator.deps.BuildGeneration = func(*sql.DB) restoreGeneration {
		return &coordinatorFakeGeneration{
			name: "post-commit", events: &state.events, eventMu: &state.eventMu,
			completeErr: &database.RestorePostCommitError{Err: errors.New("cleanup sync pending")},
		}
	}
	if err := coordinator.replace(context.Background(), operation.OperationID); err != nil {
		t.Fatalf("post-commit replacement: %v", err)
	}
	if state.rolledBack != 0 || state.activated != 1 {
		t.Fatalf("post-commit counts rolledBack=%d activated=%d", state.rolledBack, state.activated)
	}
}

func TestRestoreCoordinatorShutdownOrderingAndCurrentClear(t *testing.T) {
	coordinator, state, _, _, _ := newCoordinatorTest(t)
	if err := coordinator.ShutdownGeneration(context.Background()); err != nil {
		t.Fatalf("shutdown generation: %v", err)
	}
	events := state.snapshotEvents()
	for _, pair := range [][2]string{{"old:stop-runners", "old:begin-shutdown"}, {"old:begin-shutdown", "old:shutdown"}, {"old:shutdown", "old:detach"}, {"old:detach", "close"}} {
		if eventIndex(events, pair[0]) < 0 || eventIndex(events, pair[1]) < eventIndex(events, pair[0]) {
			t.Fatalf("shutdown event order %s before %s: %v", pair[0], pair[1], events)
		}
	}
	generation, db := coordinator.currentGeneration()
	if generation != nil || db != nil {
		t.Fatalf("shutdown retained current generation=%v db=%v", generation, db)
	}
}

func TestRestoreCoordinatorShutdownGateRejectsRunnerEnteringReplace(t *testing.T) {
	coordinator, state, _, old, operation := newCoordinatorTest(t)
	old.stopRunners = func() error {
		if err := coordinator.replace(context.Background(), operation.OperationID); err == nil {
			return errors.New("runner replacement crossed shutdown gate")
		}
		return nil
	}
	if err := coordinator.ShutdownGeneration(context.Background()); err != nil {
		t.Fatalf("shutdown with stalled runner: %v", err)
	}
	if state.installed != 0 || state.rolledBack != 0 {
		t.Fatalf("runner installed during shutdown: installed=%d rolledBack=%d", state.installed, state.rolledBack)
	}
	if _, db := coordinator.currentGeneration(); db != nil {
		t.Fatal("shutdown retained an authoritative database")
	}
}

func TestRestoreCoordinatorUsesConfiguredIOTimeoutForOpenAndRollback(t *testing.T) {
	cfg := config.Config{RestoreIOTimeout: 37 * time.Millisecond}
	coordinator := newRestoreCoordinator(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), restoreCoordinatorDependencies{})
	if coordinator.timeout.Open != cfg.RestoreIOTimeout || coordinator.timeout.Install != cfg.RestoreIOTimeout || coordinator.timeout.Rollback != cfg.RestoreIOTimeout {
		t.Fatalf("configured restore I/O timeout not applied: %#v", coordinator.timeout)
	}
}
