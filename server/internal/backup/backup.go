// Package backup implements the Plex-model server backup: plain files under
// <state>/backups/<UTC timestamp>/ holding a consistent copy of the database
// made with SQLite's online backup API while the server runs, the few other
// state files needed to come back as the same server, and a manifest carrying
// integrity hashes. No encryption, no keys, no archive format, no chunking.
//
// A backup is written into backups/.partial-<timestamp>/ and renamed into
// place, so List never sees a half-written backup. A timestamp clash on the
// same second takes a -2 suffix. Pre-restore copies live beside them as
// <timestamp>-pre-restore. Startup deletes stale .partial-* folders.
package backup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/supervise"
)

// Layout names.
const (
	// BackupsDir holds one folder per backup under the state directory.
	BackupsDir = "backups"
	// DatabaseName is the database copy inside every backup folder.
	DatabaseName = "portico.db"
	// ManifestName describes a backup folder. Names inside use "/".
	ManifestName = "manifest.json"
	// RestoreStagedDir holds a validated restore waiting for a restart.
	RestoreStagedDir = "restore-staged"
	// RestoreMarkerFile is the staged-restore marker inside RestoreStagedDir.
	RestoreMarkerFile = "restore.json"
	// RestoreResultFile records the last restore's outcome.
	RestoreResultFile = "restore-result.json"
	// Format and Version identify the manifest shape.
	Format  = "portico-backup"
	Version = 1
	// DefaultKeepCount is the shipped scheduled-backup retention, matching the
	// maintenance settings default.
	DefaultKeepCount = 7
)

// timestampLayout names a backup folder, in UTC.
const timestampLayout = "2006-01-02T150405Z"

// Kinds of backup. The keep count applies to scheduled and manual backups;
// pre-restore copies are the automatic copy taken before a restore applies.
type Kind string

const (
	KindScheduled  Kind = "scheduled"
	KindManual     Kind = "manual"
	KindPreRestore Kind = "pre-restore"
)

// FileEntry is one file in a backup folder, named with "/" separators.
type FileEntry struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Manifest is manifest.json: identity plus integrity hashes, not security.
type Manifest struct {
	Format        string      `json:"format"`
	Version       int         `json:"version"`
	ServerID      string      `json:"serverId"`
	ServerVersion string      `json:"serverVersion"`
	SchemaVersion int         `json:"schemaVersion"`
	CreatedAt     string      `json:"createdAt"`
	Kind          Kind        `json:"kind"`
	Files         []FileEntry `json:"files"`
}

// Info is one listed backup.
type Info struct {
	ID            string `json:"id"`
	CreatedAt     string `json:"createdAt"`
	Kind          Kind   `json:"kind"`
	SchemaVersion int    `json:"schemaVersion"`
	ServerVersion string `json:"serverVersion"`
	Bytes         int64  `json:"bytes"`
	Path          string `json:"path"`
}

// RunningJob is the in-progress backup the status endpoint reports.
type RunningJob struct {
	JobID      string `json:"jobId"`
	Phase      string `json:"phase"`
	BytesDone  int64  `json:"bytesDone"`
	BytesTotal int64  `json:"bytesTotal"`
}

// RestoreResult is the last restore's recorded outcome.
type RestoreResult struct {
	At      string `json:"at"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

var (
	// ErrInsufficientDisk refuses a backup (or a staged restore) when free
	// space is below the estimate. The API reports insufficient_disk.
	ErrInsufficientDisk = errors.New("backup needs more free disk space than is available")
	// ErrNotFound is an unknown backup id. The API reports not_found.
	ErrNotFound = errors.New("no such backup")
	// ErrInvalid is a malformed id, operation id or source. The API reports
	// invalid_request or restore_source_invalid.
	ErrInvalid = errors.New("invalid backup request")
	// ErrSourceInvalid is a restore source that names nothing usable. The API
	// reports restore_source_invalid.
	ErrSourceInvalid = errors.New("restore source is not a usable backup")
	// ErrIntegrity is a backup whose files fail their manifest hashes. The API
	// reports restore_integrity_failed.
	ErrIntegrity = errors.New("backup files do not match their manifest")
	// ErrRestorePending: a restore stopped part-way and must finish (the next
	// start resumes it) before another can be staged.
	ErrRestorePending = errors.New("a restore is still finishing; restart the server to complete it first")
)

// validID guards Delete and restore-by-id against traversal: a timestamp the
// service minted, with an optional clash suffix or pre-restore marker.
var validID = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{6}Z(-\d+)?(-pre-restore)?$`)

func checkID(id string) error {
	if !validID.MatchString(id) || len(id) > 64 {
		return ErrInvalid
	}
	return nil
}

// validOperationID mirrors the administration idempotency rule: the same
// operation id twice is one backup.
func validOperationID(v string) bool {
	if len(v) < 8 || len(v) > 128 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func newToken() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))[:32]
	}
	return hex.EncodeToString(raw[:])
}

// Job is one backup run. Progress is safe for concurrent readers.
type Job struct {
	id          string
	kind        Kind
	operationID string
	done        chan struct{}
	mu          sync.Mutex
	phase       string
	bytesDone   int64
	bytesTotal  int64
	err         error
}

// ID identifies the job for polling.
func (j *Job) ID() string { return j.id }

// Done closes when the run finishes, successfully or not.
func (j *Job) Done() <-chan struct{} { return j.done }

// Result reports the run's outcome after Done closes.
func (j *Job) Result() error {
	<-j.done
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.err
}

// Progress snapshots the run for the status endpoint.
func (j *Job) Progress() RunningJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	return RunningJob{JobID: j.id, Phase: j.phase, BytesDone: j.bytesDone, BytesTotal: j.bytesTotal}
}

func (j *Job) setTotal(total int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.bytesTotal = total
}

func (j *Job) report(phase string, done, total int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.phase = phase
	j.bytesDone = done
	if total > j.bytesTotal {
		j.bytesTotal = total
	}
}

func (j *Job) finish(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.err = err
	close(j.done)
}

// Service creates, lists and deletes Plex-model backups in one state folder.
// The zero value is unusable; construct with New.
type Service struct {
	state         string
	db            *sql.DB
	serverID      string
	serverVersion string
	schemaVersion int
	now           func() time.Time
	// onStaged runs after a restore is staged and answered, to restart the
	// server. It is injected by the composition root; nil skips the restart
	// (tests).
	onStaged func()

	// stageMu makes restore staging one at a time: every stage writes the same
	// restore-staged folder, so overlapping requests (a retry, a second tab)
	// wait and then supersede in order instead of deleting each other's copy.
	stageMu sync.Mutex

	mu         sync.Mutex
	jobs       map[string]*Job
	operations map[string]string
	running    *Job
}

// New opens the service on state. db is the live database handle the online
// backup reads from; it never takes the write gate.
func New(state string, db *sql.DB, serverID, serverVersion string, schemaVersion int) *Service {
	return &Service{
		state:         state,
		db:            db,
		serverID:      serverID,
		serverVersion: serverVersion,
		schemaVersion: schemaVersion,
		now:           time.Now,
		jobs:          map[string]*Job{},
		operations:    map[string]string{},
	}
}

// SetRestartFunc injects the restart that follows a staged restore.
func (s *Service) SetRestartFunc(fn func()) { s.onStaged = fn }

// Start begins an asynchronous backup. It is idempotent per operationID: the
// same id twice returns the same job. Only one backup runs at a time; a new
// operation while one is in progress joins the running job. The job's context
// is detached from the request, so the lane budget cannot cancel it.
func (s *Service) Start(ctx context.Context, kind Kind, operationID string) (string, error) {
	if kind != KindScheduled && kind != KindManual {
		return "", ErrInvalid
	}
	if !validOperationID(operationID) {
		return "", ErrInvalid
	}
	s.mu.Lock()
	if id, ok := s.operations[operationID]; ok {
		if job, ok := s.jobs[id]; ok {
			select {
			case <-job.done:
				// A finished job replays its id, unless it failed: a failed
				// backup is not a backup, so the operation retries.
				job.mu.Lock()
				failed := job.err != nil
				job.mu.Unlock()
				if !failed {
					s.mu.Unlock()
					return job.id, nil
				}
				delete(s.jobs, id)
				delete(s.operations, operationID)
			default:
				s.mu.Unlock()
				return job.id, nil
			}
		}
	}
	if s.running != nil {
		id := s.running.id
		s.operations[operationID] = id
		s.mu.Unlock()
		return id, nil
	}
	job := &Job{id: newToken(), kind: kind, operationID: operationID, done: make(chan struct{}), phase: "queued"}
	s.jobs[job.id] = job
	s.operations[operationID] = job.id
	s.running = job
	s.mu.Unlock()
	supervise.Go("backup.job", func() {
		_, err := s.create(dbwork.WithClass(context.WithoutCancel(ctx), dbwork.ClassMaintenance), job, job.kind)
		job.finish(err)
		s.mu.Lock()
		if s.running == job {
			s.running = nil
		}
		s.mu.Unlock()
	})
	return job.id, nil
}

// Running reports the in-progress backup, if any.
func (s *Service) Running() (RunningJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running == nil {
		return RunningJob{}, false
	}
	return s.running.Progress(), true
}

// Job finds a past or running job by id.
func (s *Service) Job(id string) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	return job, ok
}

// databasePath is the live database file Create copies from.
func (s *Service) databasePath() string { return filepath.Join(s.state, "server.sqlite") }

// backupsDir creates the backups folder if needed and returns it.
func (s *Service) backupsDir() (string, error) {
	dir := filepath.Join(s.state, BackupsDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// Create writes one backup synchronously and returns its id.
func (s *Service) Create(ctx context.Context, kind Kind) (string, error) {
	return s.create(ctx, nil, kind)
}

func (s *Service) create(ctx context.Context, job *Job, kind Kind) (string, error) {
	if s.db == nil || s.state == "" {
		return "", ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir, err := s.backupsDir()
	if err != nil {
		return "", err
	}
	estimate, err := s.estimate()
	if err != nil {
		return "", err
	}
	if job != nil {
		job.setTotal(estimate)
		job.report("snapshot", 0, estimate)
	}
	if err = checkFreeSpace(s.state, estimate); err != nil {
		return "", err
	}
	stamp := s.now().UTC().Format(timestampLayout)
	id, partial := stamp, filepath.Join(dir, ".partial-"+stamp)
	for n := 2; ; n++ {
		if _, err = os.Lstat(partial); os.IsNotExist(err) {
			if _, err = os.Lstat(filepath.Join(dir, id)); os.IsNotExist(err) {
				break
			}
		} else if err != nil {
			return "", err
		}
		id, partial = stamp+"-"+itoa(n), filepath.Join(dir, ".partial-"+stamp+"-"+itoa(n))
	}
	if err = os.Mkdir(partial, 0700); err != nil {
		return "", err
	}
	created := s.now().UTC()
	report := func(done, total int64) {
		if job != nil {
			job.report("snapshot", done, max64(total, estimate))
		}
	}
	if err = copyDatabase(ctx, s.db, filepath.Join(partial, DatabaseName), report); err != nil {
		_ = os.RemoveAll(partial)
		return "", err
	}
	if job != nil {
		job.report("files", estimate, estimate)
	}
	entries, err := copyStateFiles(s.state, partial)
	if err != nil {
		_ = os.RemoveAll(partial)
		return "", err
	}
	manifest := Manifest{
		Format: Format, Version: Version,
		ServerID: s.serverID, ServerVersion: s.serverVersion,
		SchemaVersion: s.schemaVersion, CreatedAt: created.Format(time.RFC3339),
		Kind: kind, Files: entries,
	}
	if err = writeManifest(partial, manifest); err != nil {
		_ = os.RemoveAll(partial)
		return "", err
	}
	if err = os.Rename(partial, filepath.Join(dir, id)); err != nil {
		_ = os.RemoveAll(partial)
		return "", err
	}
	return id, nil
}

// CreateKind writes one backup of the given kind synchronously. The schedule
// uses KindScheduled; the API uses KindManual through Start.
func (s *Service) CreateKind(ctx context.Context, kind Kind) (string, error) {
	return s.create(ctx, nil, kind)
}

// List returns every complete backup, newest first. Dot-names (partial or
// transient folders) are ignored.
func (s *Service) List() ([]Info, error) {
	dir := filepath.Join(s.state, BackupsDir)
	names, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Info{}, nil
		}
		return nil, err
	}
	out := []Info{}
	for _, name := range names {
		id := name.Name()
		if !name.IsDir() || len(id) == 0 || id[0] == '.' || checkID(id) != nil {
			continue
		}
		manifest, err := readManifest(filepath.Join(dir, id))
		if err != nil {
			continue
		}
		created := manifest.CreatedAt
		if _, err = time.Parse(time.RFC3339, created); err != nil {
			created = ""
		}
		out = append(out, Info{
			ID: id, CreatedAt: created, Kind: manifest.Kind,
			SchemaVersion: manifest.SchemaVersion, ServerVersion: manifest.ServerVersion,
			Bytes: dirBytes(filepath.Join(dir, id)), Path: id,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// Delete removes one backup.
func (s *Service) Delete(id string) error {
	if err := checkID(id); err != nil {
		return err
	}
	path := filepath.Join(s.state, BackupsDir, id)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	if !info.IsDir() {
		return ErrNotFound
	}
	return os.RemoveAll(path)
}

// Prune keeps the newest keep scheduled and manual backups, and the newest
// three pre-restore folders.
func (s *Service) Prune(keep int) error {
	if keep < 1 {
		keep = 1
	}
	listed, err := s.List()
	if err != nil {
		return err
	}
	kept, keptPre := 0, 0
	for _, info := range listed {
		if info.Kind == KindPreRestore {
			keptPre++
			if keptPre > 3 {
				if err = s.Delete(info.ID); err != nil {
					return err
				}
			}
			continue
		}
		kept++
		if kept > keep {
			if err = s.Delete(info.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// LastRestore reads the recorded outcome of the last restore, if any.
func (s *Service) LastRestore() (*RestoreResult, error) {
	raw, err := os.ReadFile(filepath.Join(s.state, RestoreResultFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out RestoreResult
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Outcome == "" {
		return nil, nil
	}
	return &out, nil
}

// CleanupPartials deletes stale .partial-* folders. It runs at startup,
// before any staged restore is applied.
func CleanupPartials(state string) error {
	dir := filepath.Join(state, BackupsDir)
	names, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, name := range names {
		id := name.Name()
		if len(id) == 0 || id[0] != '.' {
			continue
		}
		if err = os.RemoveAll(filepath.Join(dir, id)); err != nil {
			return err
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
