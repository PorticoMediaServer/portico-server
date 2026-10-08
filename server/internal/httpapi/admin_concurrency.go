package httpapi

import (
	"context"
	"net/http"
	"runtime"
	"runtime/debug"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/hostlimits"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/supervise"
)

// ConcurrencyDiagnostics is the whole admission and database picture in one
// document. Every number here is one a capacity decision was made from, so the
// same numbers have to be visible when that decision is revisited: a lane
// capacity is only defensible next to its rejection count and queue wait, and a
// pool size is only defensible next to WaitCount and WaitDuration.
type ConcurrencyDiagnostics struct {
	Pool dbwork.PoolStats `json:"pool"`
	// Snapshots is read-snapshot admission: how many composite reads hold a
	// pooled connection right now, how many may, and how long callers waited for
	// a permit. It is the number that says whether composite reads are starving
	// the writer.
	Snapshots dbwork.SnapshotStats   `json:"readSnapshots"`
	WriteGate dbwork.GateStats       `json:"writeGate"`
	Pressure  dbwork.PressureStats   `json:"pressure"`
	Health    dbwork.HealthReport    `json:"health"`
	Lanes     []LaneDiagnostics      `json:"httpLanes"`
	JobLanes  []operations.LaneState `json:"jobLanes"`
	Search    SearchDiagnostics      `json:"search"`
	// Reads is the process-wide statement picture: how much work callers asked
	// for, next to the pool and gate numbers that say how long they waited.
	Reads dbwork.ReadStats `json:"reads"`
	// Publications is the number of gated write transactions committed since the
	// process started: one per batch, which is the granularity a read-model cache
	// or a reconciler wants when it asks "has anything been published since?"
	Publications uint64 `json:"publications"`
	// Authorization is the bounded authority caches. `authorityEpoch` is the
	// fence they hang on: it moves whenever a security-fence transaction commits
	// or any statement writes a table that decides authority, and every entry
	// made before that stops matching.
	Authorization AuthorizationCacheDiagnostics `json:"authorizationCaches"`
	// RouteCosts is statements, transactions and pooled-connection acquisitions
	// per request, per route. A route whose statement count is linear in library
	// size shows here long before it shows in a percentile.
	RouteCosts []RouteCost `json:"routeCosts"`
	// Process, WAL, Helpers and Integrity are the triage half: what the runtime,
	// the write-ahead log, the media helper pool and the database's own
	// structure look like when something is wrong that the percentiles do not
	// explain.
	// Conversions is the host-capacity bound on simultaneous media conversions:
	// how many are running, how many this machine will run at once, and how many
	// starts have been turned away. The third number is how an owner learns
	// their hardware is the limit rather than their network.
	Conversions playback.ConversionCapacity `json:"conversions"`
	Process     ProcessDiagnostics          `json:"process"`
	WAL         dbwork.WALStats             `json:"wal"`
	Helpers     storage.SupervisorStats     `json:"storageHelpers"`
	Integrity   IntegrityDiagnostics        `json:"integrity"`
}

// AuthorizationCacheDiagnostics reports the authority caches and their fence.
type AuthorizationCacheDiagnostics struct {
	Principals          PrincipalCacheStats `json:"principals"`
	LibraryGrants       int                 `json:"libraryGrants"`
	GrantCapacity       int                 `json:"libraryGrantCapacity"`
	AuthorityGeneration uint64              `json:"authorityGeneration"`
}

// IntegrityDiagnostics is what the server knows about its database's structural
// health. ForeignSchemaObjects counts objects this build does not define — a
// state directory written by a different or newer Portico — and is the first
// thing to look at when the server behaves in a way the code cannot explain.
type IntegrityDiagnostics struct {
	dbwork.IntegrityReport
	ForeignSchemaObjects int `json:"foreignSchemaObjects"`
}

// ProcessDiagnostics is the triage view a field incident needs: whether
// goroutines are leaking, where the heap is against the limit the garbage
// collector is working to, and whether anything has panicked. Without these
// three a hung server and a leaking one look identical from outside.
type ProcessDiagnostics struct {
	Goroutines        int             `json:"goroutines"`
	HeapAllocBytes    uint64          `json:"heapAllocBytes"`
	HeapInUseBytes    uint64          `json:"heapInUseBytes"`
	HeapSysBytes      uint64          `json:"heapSysBytes"`
	NextGCBytes       uint64          `json:"nextGCBytes"`
	GCCycles          uint32          `json:"gcCycles"`
	MemoryLimitBytes  int64           `json:"memoryLimitBytes"`
	ContainedPanics   uint64          `json:"containedPanics"`
	LoopRestarts      uint64          `json:"supervisedLoopRestarts"`
	MaxProcs          int             `json:"maxProcs"`
	DatabaseCalls     uint64          `json:"databaseCalls"`
	OpenFileSoftLimit uint64          `json:"openFileSoftLimit"`
	OpenFileHardLimit uint64          `json:"openFileHardLimit"`
	ProcessCPU        operations.Fact `json:"processCpu"`
	CapacityPolicy    string          `json:"capacityPolicy"`
}

// processDiagnostics samples the runtime. ReadMemStats stops the world briefly,
// which is acceptable on an owner-only diagnostic and is why this is not on any
// other path.
func processDiagnostics() ProcessDiagnostics {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	soft, hard := hostlimits.OpenFiles()
	return ProcessDiagnostics{
		Goroutines:        runtime.NumGoroutine(),
		HeapAllocBytes:    mem.HeapAlloc,
		HeapInUseBytes:    mem.HeapInuse,
		HeapSysBytes:      mem.HeapSys,
		NextGCBytes:       mem.NextGC,
		GCCycles:          mem.NumGC,
		MemoryLimitBytes:  debug.SetMemoryLimit(-1),
		ContainedPanics:   supervise.Panics(),
		LoopRestarts:      supervise.Restarts(),
		MaxProcs:          runtime.GOMAXPROCS(0),
		DatabaseCalls:     dbwork.DatabaseCalls(),
		OpenFileSoftLimit: soft,
		OpenFileHardLimit: hard,
		ProcessCPU:        operations.ProcessCPU(),
		CapacityPolicy:    "Resource pressure is advisory; explicit owner caps govern admission.",
	}
}

// SearchDiagnostics reports the per-viewer search governor.
type SearchDiagnostics struct {
	Active         int    `json:"active"`
	PerViewerLimit int    `json:"perViewerLimit"`
	GlobalLimit    int    `json:"globalLimit"`
	Rejected       uint64 `json:"rejected"`
}

func (d Dependencies) concurrencyDiagnostics(ctx context.Context) ConcurrencyDiagnostics {
	out := ConcurrencyDiagnostics{
		Process:      processDiagnostics(),
		WAL:          dbwork.WAL(ctx, d.DB),
		Pool:         dbwork.Pool(d.DB),
		Snapshots:    dbwork.Snapshots(),
		WriteGate:    dbwork.WriteGate().Stats(),
		Pressure:     dbwork.Pressure(),
		Lanes:        []LaneDiagnostics{},
		JobLanes:     []operations.LaneState{},
		Search:       SearchDiagnostics{PerViewerLimit: maxSearchesPerViewer, GlobalLimit: maxSearchesGlobal},
		Reads:        dbwork.Reads(),
		Publications: dbwork.Publications(),
		Authorization: AuthorizationCacheDiagnostics{
			Principals:          d.principals.stats(),
			LibraryGrants:       d.access.size(),
			GrantCapacity:       accessCacheEntries,
			AuthorityGeneration: dbwork.AuthorityGeneration(),
		},
		RouteCosts: RouteCosts(),
	}
	if d.admission != nil {
		out.Lanes = d.admission.diagnostics()
		d.admission.searchMu.Lock()
		out.Search.Active = d.admission.searchTotal
		d.admission.searchMu.Unlock()
		out.Search.Rejected = d.admission.searchDenied.Load()
	}
	if d.Watchdog != nil {
		out.Health = d.Watchdog.Report()
	}
	if d.Storage != nil {
		out.Helpers = d.Storage.Supervisor.Stats()
	}
	if d.Playback != nil {
		out.Conversions = d.Playback.Conversions()
	}
	out.Integrity = IntegrityDiagnostics{IntegrityReport: dbwork.LastIntegrity(), ForeignSchemaObjects: dbwork.ForeignSchemaObjects()}
	return out
}

func (d Dependencies) concurrencyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/admin/diagnostics/concurrency", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.owner(r); e != nil {
			failure(w, e)
			return
		}
		out := d.concurrencyDiagnostics(r.Context())
		if d.Scheduler != nil {
			out.JobLanes = d.Scheduler.LaneStates(r.Context())
		}
		write(w, 200, out)
	})
	// The database's structural health, checked now rather than whenever the
	// scheduled pass last ran. It is the heavy admin lane because quick_check
	// and foreign_key_check both read the whole database.
	mux.HandleFunc("POST /v1/admin/diagnostics/integrity", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.owner(r); e != nil {
			failure(w, e)
			return
		}
		write(w, 200, IntegrityDiagnostics{IntegrityReport: dbwork.Integrity(r.Context(), d.DB), ForeignSchemaObjects: dbwork.ForeignSchemaObjects()})
	})
	// Repair is a separate route from the check, and a POST, because it deletes
	// rows. It removes orphans only from children the schema declares ON DELETE
	// CASCADE — where the schema has already said what removing the parent
	// means — and reports the rest for a person to decide about. Nothing here
	// runs on a timer: a repair nobody asked for is a repair nobody reviews.
	mux.HandleFunc("POST /v1/admin/diagnostics/integrity/repair", func(w http.ResponseWriter, r *http.Request) {
		if _, e := d.owner(r); e != nil {
			failure(w, e)
			return
		}
		report, err := persistence.RepairCascadeOrphans(r.Context(), d.DB)
		if err != nil {
			// A partial repair is still a result worth returning: it says what was
			// removed before it stopped, which is what an owner needs to decide
			// whether to run it again.
			write(w, 200, map[string]any{"repair": report, "incomplete": true, "error": "The repair could not be completed."})
			return
		}
		write(w, 200, map[string]any{"repair": report, "incomplete": false})
	})
	// Readiness is deliberately unauthenticated and free of database work: a
	// supervisor has to be able to ask "are you up?" of a server that is still
	// opening its database, and the answer must not itself need the database.
	mux.HandleFunc("GET /v1/readiness", func(w http.ResponseWriter, r *http.Request) {
		ready := d.DB != nil
		status := "ready"
		if !ready {
			status = "starting"
		}
		if d.Watchdog != nil {
			report := d.Watchdog.Report()
			if report.State == dbwork.CorruptState {
				ready, status = false, "corrupt"
			} else if report.State == dbwork.DegradedState {
				status = "degraded"
			}
		}
		if !ready {
			w.Header().Set("Retry-After", "2")
			write(w, 503, map[string]any{"ready": false, "status": status})
			return
		}
		write(w, 200, map[string]any{"ready": true, "status": status})
	})
}
