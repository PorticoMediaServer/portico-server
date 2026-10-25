package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

// A source's own page reads these facts: what it is doing this minute, when
// Portico last checked it, and which libraries use its folders. Everything is
// something the server knows now; nothing is estimated or drawn ahead of time.
type SourceLibraryUse struct {
	LibraryID string `json:"libraryId"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Folders   int    `json:"folders"`
}

// SourceScanState is one source folder's active scan as the scanner records it.
// State, mode and phase are closed codes; counters and timestamps are the real
// stored values. SourceGeneration and RunPolicyRevision are the values the run
// captured, not proof they equal the current source/policy. It never carries a
// job error, path, locator or derived percent.
type SourceScanState struct {
	State             string `json:"state"`
	Mode              string `json:"mode,omitempty"`
	Phase             string `json:"phase,omitempty"`
	SourceGeneration  int64  `json:"sourceGeneration"`
	RunPolicyRevision int64  `json:"runPolicyRevision"`
	Processed         int64  `json:"processed"`
	Discovered        int64  `json:"discovered"`
	Analyzed          int64  `json:"analyzed"`
	Warnings          int64  `json:"warnings"`
	StartedAt         string `json:"startedAt,omitempty"`
	CompletedAt       string `json:"completedAt,omitempty"`
	FinishedAt        string `json:"finishedAt,omitempty"`
}

var sourceHealths = map[string]bool{"ready": true, "inventory_running": true, "cancelled": true, "offline": true, "root_changed": true, "removing": true}
var scanStates = map[string]bool{"queued": true, "running": true, "paused": true}
var scanModes = map[string]bool{"first-import": true, "incremental": true, "integrity": true, "full": true}
var scanPhases = map[string]bool{"inventory": true, "basic_pending": true, "basic_running": true, "analysis_enqueue": true, "reconciling": true, "verifying": true, "extras": true}

type SourceStatus struct {
	SourceID         string             `json:"sourceId"`
	Generation       string             `json:"generation"`
	State            string             `json:"state"`
	ErrorCode        string             `json:"errorCode,omitempty"`
	RangeSupport     string             `json:"rangeSupport"`
	LastCheckedAt    string             `json:"lastCheckedAt"`
	ConnectionsInUse int                `json:"connectionsInUse"`
	PlaybackReads    int                `json:"playbackReads"`
	AnalysisReads    int                `json:"analysisReads"`
	ScanIntervalSecs int                `json:"scanIntervalSeconds"`
	Libraries        []SourceLibraryUse `json:"libraries"`
	// LibrariesTruncated says the source is used by more libraries than the
	// bounded list returns; the page shows the first ones and says so.
	LibrariesTruncated bool `json:"librariesTruncated"`
	// A mounted source's program and remote are the mount's own facts; the
	// background speed is the server-wide Schedule value that applies to it.
	Program         string `json:"program,omitempty"`
	ProgramVersion  string `json:"programVersion,omitempty"`
	ProgramRevision int64  `json:"programRevision,omitempty"`
	ProgramIncluded bool   `json:"programIncluded,omitempty"`
	CacheUsedBytes  int64  `json:"cacheUsedBytes"`
	Remote          string `json:"remote,omitempty"`
	BackgroundSpeed string `json:"backgroundSpeed,omitempty"`
	// Health is the folder health when every enabled folder of this source
	// agrees on it; a mixed source is omitted rather than summed or guessed.
	Health string `json:"health,omitempty"`
	// LastCompleteAt is the completed-scan time only when every enabled folder
	// agrees on one stored value; a mixed source is omitted.
	LastCompleteAt string `json:"lastCompleteAt,omitempty"`
	// Scan is a real active scan when this server can prove one: the active row
	// still matches its folder's current generation/incarnation/identity, its
	// library and run. Absence can mean unsupported, invalid or unobserved, so it
	// is not proof that no scan is running; it is never a guess or a percentage.
	Scan *SourceScanState `json:"scan,omitempty"`
	// ServeOperationsInUse samples the saved source's shared operation budget
	// through its current live serve process. Despite the historical field name,
	// this includes direct WebDAV requests and saved-source listings on the same
	// budget; mounted media reads outside it are not measured. These are admitted
	// operations, never sockets or distinct viewers. Absent means no live process
	// handle could supply a current sample, not that no direct work exists; a
	// present zero is measured idleness of this budget. ConnectionsInUse retains
	// its shipped meaning (admitted media read objects, which may share a stream).
	// No complete per-source upstream connection count exists.
	ServeOperationsInUse *int `json:"serveOperationsInUse,omitempty"`
}

// ReadSourceStatus is a bounded read of one source's present state. It lists
// no provider folder and opens no connection: the live counts are the reads the
// server itself is holding for this source.
func (s *Service) ReadSourceStatus(ctx context.Context, identities *identity.Service, owner identity.Principal, source string) (SourceStatus, error) {
	if s == nil || s.db == nil || source == "" {
		return SourceStatus{}, storage.ErrRemoteConfig
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return SourceStatus{}, err
	}
	defer snapshot.Rollback()
	if err = currentLocalOwnerTx(ctx, snapshot.Tx(), identities, owner); err != nil {
		return SourceStatus{}, err
	}
	row, err := sourceRowTx(ctx, snapshot.Tx(), source)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SourceStatus{}, storage.ErrRemoteOffline
		}
		return SourceStatus{}, err
	}
	out := SourceStatus{SourceID: row.ID, Generation: strconv.FormatInt(row.Generation, 10), State: row.State, ErrorCode: row.ErrorCode, RangeSupport: row.RangeSupport, Libraries: []SourceLibraryUse{}}
	if code := s.pacerCode(row.ID, out.Generation); code != "" && out.ErrorCode == "" {
		out.State, out.ErrorCode = code, code
	}
	out.PlaybackReads, out.AnalysisReads, out.ConnectionsInUse = s.liveReads(source)
	// ConnectionsInUse keeps its shipped meaning: the media read objects this
	// server admits for the source. Reads can share a stream and an open object
	// need not hold an upstream operation, so that legacy field is deliberately
	// left alone and is not a connection count. The only complete measured
	// operation counter is the live serve process's admitted work; it is
	// published separately and omitted when no process holds this generation, so
	// a reader sees "unknown" instead of a fabricated global count.
	if active, ok := s.sourceOperationInUse(source, out.Generation); ok {
		out.ServeOperationsInUse = &active
	}
	var observed int64
	if err = snapshot.Tx().QueryRowContext(ctx, `SELECT COALESCE(MAX(observed_ms),0) FROM remote_source_webdav_tests WHERE source_id=? AND source_generation=?`, row.ID, row.Generation).Scan(&observed); err != nil {
		return SourceStatus{}, err
	}
	if observed > 0 {
		out.LastCheckedAt = time.UnixMilli(observed).UTC().Format(time.RFC3339)
	}
	// Ten drawn libraries are plenty for the page; the count stays bounded so a
	// constant-cost read never grows with the owner's library list.
	rows, err := snapshot.Tx().QueryContext(ctx, `SELECT l.id,l.name,l.kind,COUNT(*) FROM library_sources s JOIN libraries l ON l.id=s.library_id WHERE s.remote_source_id=? GROUP BY l.id,l.name,l.kind ORDER BY l.name,l.id LIMIT 11`, row.ID)
	if err != nil {
		return SourceStatus{}, err
	}
	for rows.Next() {
		var use SourceLibraryUse
		if err = rows.Scan(&use.LibraryID, &use.Name, &use.Kind, &use.Folders); err != nil {
			break
		}
		out.Libraries = append(out.Libraries, use)
	}
	readErr := rows.Err()
	rows.Close()
	if err != nil {
		return SourceStatus{}, err
	}
	if readErr != nil {
		return SourceStatus{}, readErr
	}
	if len(out.Libraries) > 10 {
		out.Libraries = out.Libraries[:10]
		out.LibrariesTruncated = true
	}
	if err = snapshot.Tx().QueryRowContext(ctx, `SELECT COALESCE(MIN(NULLIF(interval_seconds,0)),0) FROM library_sources WHERE remote_source_id=?`, row.ID).Scan(&out.ScanIntervalSecs); err != nil {
		return SourceStatus{}, err
	}
	var healthCount int
	var minHealth, maxHealth, minComplete, maxComplete string
	if err = snapshot.Tx().QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MIN(health),''),COALESCE(MAX(health),''),COALESCE(MIN(last_complete_at),''),COALESCE(MAX(last_complete_at),'') FROM library_sources WHERE remote_source_id=? AND enabled=1`, row.ID).Scan(&healthCount, &minHealth, &maxHealth, &minComplete, &maxComplete); err != nil {
		return SourceStatus{}, err
	}
	if healthCount > 0 && minHealth == maxHealth && sourceHealths[minHealth] {
		out.Health = minHealth
	}
	if healthCount > 0 && minComplete != "" && minComplete == maxComplete {
		out.LastCompleteAt = scanTime(minComplete)
	}
	var scanState, scanMode, scanPhase, scanStarted, scanCompleted, scanFinished string
	var scanGeneration, scanPolicy, scanProcessed, scanDiscovered, scanAnalyzed, scanWarnings int64
	scanErr := snapshot.Tx().QueryRowContext(ctx, `SELECT j.status,r.scan_mode,r.phase,r.source_generation,r.policy_revision,j.processed,r.discovered,r.analyzed,r.warnings,j.created_at,r.inventory_completed_at,r.finished_at FROM library_sources s JOIN inventory_source_active a ON a.source_id=s.id JOIN jobs j ON j.id=a.job_id JOIN inventory_runs r ON r.job_id=j.id WHERE s.remote_source_id=? AND s.enabled=1 AND r.source_id=s.id AND j.library_id=s.library_id AND r.source_generation=s.generation AND r.root_incarnation=s.incarnation AND r.root_identity=s.root_identity AND j.status IN('queued','running','paused') ORDER BY j.created_at DESC,j.id DESC LIMIT 1`, row.ID).Scan(&scanState, &scanMode, &scanPhase, &scanGeneration, &scanPolicy, &scanProcessed, &scanDiscovered, &scanAnalyzed, &scanWarnings, &scanStarted, &scanCompleted, &scanFinished)
	if scanErr == nil && scanStates[scanState] && scanFactsBounded(scanGeneration, scanPolicy, scanProcessed, scanDiscovered, scanAnalyzed, scanWarnings) {
		out.Scan = &SourceScanState{State: scanState, SourceGeneration: scanGeneration, RunPolicyRevision: scanPolicy, Processed: scanProcessed, Discovered: scanDiscovered, Analyzed: scanAnalyzed, Warnings: scanWarnings, StartedAt: scanTime(scanStarted), CompletedAt: scanTime(scanCompleted), FinishedAt: scanTime(scanFinished)}
		if scanModes[scanMode] {
			out.Scan.Mode = scanMode
		}
		if scanPhases[scanPhase] {
			out.Scan.Phase = scanPhase
		}
	} else if scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
		return SourceStatus{}, scanErr
	}
	if row.MountID != "" {
		var executable, version, remote string
		var control int64
		if err = snapshot.Tx().QueryRowContext(ctx, `SELECT m.executable,COALESCE(b.version,''),m.remote,m.program_included,(SELECT c.revision FROM mount_controls c WHERE c.mount_id=m.id) FROM managed_mounts m LEFT JOIN mount_backend_configs b ON b.mount_id=m.id WHERE m.id=?`, row.MountID).Scan(&executable, &version, &remote, &out.ProgramIncluded, &control); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return SourceStatus{}, err
		}
		out.Program, out.ProgramVersion, out.Remote, out.ProgramRevision = executable, version, remote, control
	}
	if err = snapshot.Tx().QueryRowContext(ctx, `SELECT COALESCE(SUM(used_bytes),0) FROM remote_source_cache_entries WHERE source_id=?`, row.ID).Scan(&out.CacheUsedBytes); err != nil {
		return SourceStatus{}, err
	}
	if err = snapshot.Tx().QueryRowContext(ctx, `SELECT background_speed FROM maintenance_settings WHERE singleton=1`).Scan(&out.BackgroundSpeed); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SourceStatus{}, err
	}
	return out, nil
}

// scanFactsBounded keeps stored counters and identifiers inside the ranges the
// scanner can actually produce; a corrupt or negative value omits the fact.
func scanFactsBounded(generation, policy, processed, discovered, analyzed, warnings int64) bool {
	const maxCount = int64(1) << 40
	return generation >= 0 && generation <= maxCount && policy >= 0 && policy <= maxCount &&
		processed >= 0 && processed <= maxCount && discovered >= 0 && discovered <= maxCount &&
		analyzed >= 0 && analyzed <= maxCount && warnings >= 0 && warnings <= maxCount
}

// scanTime keeps only a valid stored timestamp; anything else reads as absent.
func scanTime(value string) string {
	if value == "" {
		return ""
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		return ""
	}
	return value
}

// liveReads counts the provider reads this server holds for one source now.
// Reads of one file share a provider stream, so connections never exceed reads.
func (s *Service) liveReads(id string) (playback, analysis, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, read := range s.active[id] {
		total++
		switch read.lane {
		case "playback":
			playback++
		case "analysis", "inventory":
			analysis++
		}
	}
	return playback, analysis, total
}

// pacerCode is the same provider backoff the list shows, so the page and the
// list never disagree about a source that is waiting.
func (s *Service) pacerCode(id, generation string) string {
	if s.sourcePacerBackoff(id, generation) {
		return "rate_limited"
	}
	return ""
}
