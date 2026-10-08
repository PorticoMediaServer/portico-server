package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

// A supervisor's health check, a browser that was already open, and a phone
// reconnecting on a LAN all reach the server before it has finished opening its
// database. Answering them with a connection refusal tells them nothing;
// answering with an honest 503 and a phase report tells them exactly what to
// wait for. That is why the listener is bound before the database is opened and
// the real handler is swapped in only when the server can actually serve.

// startupPhase is one named step of bringing the server up.
type startupPhase struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Started  int64  `json:"startedUnixMillis"`
	Finished int64  `json:"finishedUnixMillis,omitempty"`
	Failed   string `json:"error,omitempty"`
}

// startupReporter answers every request while the server is coming up.
type startupReporter struct {
	mu     sync.Mutex
	phases []startupPhase
}

func newStartupReporter() *startupReporter { return &startupReporter{} }

func (s *startupReporter) start(name, label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phases = append(s.phases, startupPhase{Name: name, Label: label, Started: time.Now().UnixMilli()})
}

func (s *startupReporter) finish(name string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.phases {
		if s.phases[index].Name == name && s.phases[index].Finished == 0 {
			s.phases[index].Finished = time.Now().UnixMilli()
			if err != nil {
				s.phases[index].Failed = err.Error()
			}
			return
		}
	}
}

// step opens a phase and returns the function that closes it, so a linear
// startup can be reported and timed without turning every call into a block:
//
//	done := reporter.step("identity", "Load the server identity")
//	ident, e := identity.New(db, state)
//	done(e)
//
// With PORTICO_STARTUP_TRACE=1 each phase logs what it cost, which is how the
// library-sized work between exec and the first served request was found.
func (s *startupReporter) step(name, label string) func(error) {
	s.start(name, label)
	started := time.Now()
	return func(err error) {
		s.finish(name, err)
		if os.Getenv("PORTICO_STARTUP_TRACE") == "1" {
			log.Printf("startup phase %-24s %s", name, time.Since(started).Round(time.Millisecond))
		}
	}
}

func (s *startupReporter) snapshot() []startupPhase {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]startupPhase, len(s.phases))
	copy(out, s.phases)
	return out
}

func (s *startupReporter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "2")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/v1/readiness" || r.URL.Path == "/health/ready" {
		w.WriteHeader(503)
		_ = json.NewEncoder(w).Encode(map[string]any{"ready": false, "status": "starting", "phases": s.snapshot()})
		return
	}
	w.WriteHeader(503)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "starting", "message": "The server is still starting. Retry shortly.", "retryable": true}, "phases": s.snapshot()})
}

// switchableHandler lets the listener serve the startup reporter first and the
// real handler afterwards, without ever closing and reopening the socket.
type switchableHandler struct {
	current atomic.Pointer[http.Handler]
}

// serverReadyHook is replaced only by the isolated idle-budget test process.
// It marks the moment the fully constructed router has replaced the startup
// reporter, before any post-ready maintenance can be mistaken for startup.
var serverReadyHook = func() {}
var deferredMaintenanceDoneHook = func() {}

func newSwitchableHandler(initial http.Handler) *switchableHandler {
	s := &switchableHandler{}
	s.set(initial)
	return s
}

func (s *switchableHandler) set(h http.Handler) { s.current.Store(&h) }

func (s *switchableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	(*s.current.Load()).ServeHTTP(w, r)
}

// runDeferredMaintenance does the database housekeeping that used to be
// tempting to run at startup: planner statistics and a WAL checkpoint. It runs after the listener is live and only
// once foreground work has gone quiet, so a viewer who opened the app the moment
// the server came up is never behind it.
//
// The write gate is never held across work the size of the library. Planner
// statistics used to be `PRAGMA optimize` under the gate: on a table with no
// sqlite_stat1 rows that counts the whole b-tree inside one write transaction,
// and on a million-song library every sign-in queued behind it for eight
// minutes. persistence.RefreshPlannerStatistics does the counting and sampling
// on a background read connection and holds the gate only to write each
// table's few statistics rows.
func runDeferredMaintenance(ctx context.Context, db *sql.DB) {
	defer deferredMaintenanceDoneHook()
	if db == nil || !dbwork.Yield(ctx) {
		return
	}
	if err := persistence.RefreshPlannerStatistics(ctx, db); err != nil && ctx.Err() == nil {
		log.Printf("Planner statistics refresh stopped early and will resume on the next start: %v", err)
	}
	if !dbwork.Yield(ctx) {
		return
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`)
}
