// Package remotesources binds native adapters to the baseline storage interface.
// It owns adapter configuration, private listing continuations and acquisitions;
// library identity, scan scheduling, reconciliation and playback authority remain
// with their existing owners.
package remotesources

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/worker"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/mediasource"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

type Source struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	RootPath          string `json:"rootPath"`
	Generation        int64  `json:"generation"`
	State             string `json:"state"`
	ErrorCode         string `json:"errorCode,omitempty"`
	RangeSupport      string `json:"rangeSupport"`
	Origin            string `json:"origin,omitempty"`
	InsecureLocal     bool   `json:"insecureLocal"`
	CredentialPresent bool   `json:"credentialPresent"`
	MountID           string `json:"mountId,omitempty"`
}
type Service struct {
	db                  *sql.DB
	mounts              *mounts.Service
	private             string
	commands            sync.Mutex
	mu                  sync.Mutex
	active              map[string]map[string]activeRead
	bindings            atomic.Value
	listings            sync.Mutex
	listingClaims       map[string]bool
	nativeListings      map[string]*nativeListing
	nativeClosed        bool
	inventory, playback chan struct{}
	resolver            remotemedia.Resolver
}
type activeRead struct {
	cancel context.CancelFunc
	lane   string
}
type binding struct {
	ID, Kind, Root string
	Removed        bool
}

func New(db *sql.DB, state string, managed *mounts.Service) (*Service, error) {
	if db == nil || managed == nil {
		return nil, storage.ErrRemoteConfig
	}
	private := filepath.Join(state, "remote-sources")
	if err := os.MkdirAll(private, 0700); err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(private)
	if err != nil || canonical != private {
		return nil, storage.ErrRemoteConfig
	}
	if err = os.Chmod(private, 0700); err != nil {
		return nil, err
	}

	s := &Service{db: db, mounts: managed, private: private, active: map[string]map[string]activeRead{}, inventory: make(chan struct{}, 2), playback: make(chan struct{}, 8)}
	if err = s.RefreshMounts(context.Background()); err != nil {
		return nil, err
	}
	managed.NativeBusy = s.PlaybackActive
	managed.NativeInvalidated = s.invalidate
	// No acquisition survives a server process. Remove only our private spool names.
	if files, e := os.ReadDir(private); e == nil {
		for _, file := range files {
			if !file.IsDir() && strings.HasPrefix(file.Name(), "acquisition-") {
				_ = os.Remove(filepath.Join(private, file.Name()))
			}
		}
	}
	return s, nil
}
func (s *Service) RefreshMounts(ctx context.Context) error {
	rows, err := s.mounts.List()
	if err != nil {
		return err
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	for _, m := range rows {
		_, err = tx.Exec(`INSERT INTO remote_sources(id,kind,name,root,generation,range_support,credential_present,mount_id) VALUES(?,'rclone',?,?,?,'sequential',1,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,generation=excluded.generation,removed=?,state=CASE WHEN ? THEN 'removing' WHEN remote_sources.generation!=excluded.generation THEN 'ready' ELSE remote_sources.state END,error_code=CASE WHEN remote_sources.generation!=excluded.generation THEN '' ELSE remote_sources.error_code END`, m.ID, m.Name, m.MountPath, m.BackendRevision, m.ID, m.RemovalPending, m.RemovalPending)
		if err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`UPDATE remote_sources SET removed=1,state='removed' WHERE kind='rclone' AND NOT EXISTS(SELECT 1 FROM managed_mounts m WHERE m.id=remote_sources.mount_id)`); err != nil {
		return err
	}
	if err = gated.Commit(); err != nil {
		return err
	}
	return s.refreshBindings(ctx)
}
func (s *Service) refreshBindings(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,root,removed FROM remote_sources`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []binding{}
	for rows.Next() {
		var b binding
		if err = rows.Scan(&b.ID, &b.Kind, &b.Root, &b.Removed); err != nil {
			return err
		}
		out = append(out, b)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	s.bindings.Store(out)
	return nil
}
func (s *Service) find(path string) (binding, string, bool) {
	if !filepath.IsAbs(path) {
		return binding{}, "", false
	}
	rows, _ := s.bindings.Load().([]binding)
	for _, b := range rows {
		relative, err := filepath.Rel(b.Root, path)
		if err == nil && filepath.IsLocal(relative) {
			if relative == "." {
				relative = ""
			}
			return b, filepath.ToSlash(relative), true
		}
	}
	return binding{}, "", false
}
func (s *Service) Handles(path string) bool { _, _, ok := s.find(path); return ok }
func (s *Service) List(ctx context.Context) ([]Source, error) {
	if err := s.RefreshMounts(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,name,root,generation,state,error_code,range_support,origin,insecure,credential_present,mount_id FROM remote_sources WHERE removed=0 ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var x Source
		if err = rows.Scan(&x.ID, &x.Kind, &x.Name, &x.RootPath, &x.Generation, &x.State, &x.ErrorCode, &x.RangeSupport, &x.Origin, &x.InsecureLocal, &x.CredentialPresent, &x.MountID); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Service) get(ctx context.Context, id string) (Source, error) {
	var x Source
	err := s.db.QueryRowContext(ctx, `SELECT id,kind,name,root,generation,state,error_code,range_support,origin,insecure,credential_present,mount_id FROM remote_sources WHERE id=? AND removed=0`, id).Scan(&x.ID, &x.Kind, &x.Name, &x.RootPath, &x.Generation, &x.State, &x.ErrorCode, &x.RangeSupport, &x.Origin, &x.InsecureLocal, &x.CredentialPresent, &x.MountID)
	if errors.Is(err, sql.ErrNoRows) {
		return x, storage.ErrRemoteOffline
	}
	return x, err
}
func (s *Service) dav(ctx context.Context, id string) (*remotemedia.DAV, string, error) {
	var sealed []byte
	var generation int64
	err := s.db.QueryRowContext(ctx, `SELECT config,generation FROM remote_sources WHERE id=? AND kind='webdav' AND removed=0`, id).Scan(&sealed, &generation)
	if err != nil {
		return nil, "", storage.ErrRemoteOffline
	}
	raw, err := s.mounts.Open(sealed)
	if err != nil {
		return nil, "", storage.ErrRemoteCredentials
	}
	defer clear(raw)
	var config remotemedia.DAVConfig
	if json.Unmarshal(raw, &config) != nil {
		return nil, "", storage.ErrRemoteConfig
	}
	d, err := remotemedia.NewDAV(config, s.resolver)
	return d, strconv.FormatInt(generation, 10), mapError(err)
}
func (s *Service) generation(ctx context.Context, b binding) (string, error) {
	if b.Kind == "rclone" {
		n, err := s.mounts.Native(ctx, b.ID)
		if err != nil {
			return "", err
		}
		return n.Generation(), nil
	}
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT generation FROM remote_sources WHERE id=? AND removed=0`, b.ID).Scan(&n)
	if err != nil {
		return "", storage.ErrRemoteOffline
	}
	return strconv.FormatInt(n, 10), nil
}
func (s *Service) check(ctx context.Context, b binding, generation string) error {
	got, err := s.generation(ctx, b)
	if err != nil {
		return err
	}
	if got != generation {
		return storage.ErrRemoteChanged
	}
	return nil
}
func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, mediasource.ErrSourceChanged):
		return storage.ErrRemoteChanged
	case errors.Is(err, remotemedia.ErrDenied):
		return storage.ErrRemoteCredentials
	case errors.Is(err, remotemedia.ErrPolicy), errors.Is(err, remotemedia.ErrDAVResponse):
		return storage.ErrRemoteConfig
	case errors.Is(err, remotemedia.ErrDAVToken):
		return storage.ErrRemoteCursor
	case errors.Is(err, remotemedia.ErrRepresentationRange):
		return storage.ErrRemoteRange
	case errors.Is(err, remotemedia.ErrBusy):
		return storage.ErrBusy
	default:
		return err
	}
}
func ErrorCode(err error) string {
	switch {
	case errors.Is(err, storage.ErrRemoteChanged), errors.Is(err, mediasource.ErrSourceChanged):
		return "source_changed"
	case errors.Is(err, storage.ErrRemoteCredentials), errors.Is(err, remotemedia.ErrDenied):
		return "credentials_required"
	case errors.Is(err, storage.ErrRemoteBinary):
		return "binary_changed"
	case errors.Is(err, storage.ErrRemoteRange):
		return "range_unsupported"
	case errors.Is(err, storage.ErrRemoteConfig):
		return "invalid_configuration"
	case errors.Is(err, storage.ErrRemoteCursor):
		return "cursor_expired"
	case errors.Is(err, storage.ErrRemoteLimit):
		return "budget_exceeded"
	case errors.Is(err, storage.ErrBusy), errors.Is(err, remotemedia.ErrBusy):
		return "rate_limited"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return "offline"
	}
}
func (s *Service) health(b binding, generation string, err error) {
	code, state := "", "healthy"
	if err != nil {
		code = ErrorCode(err)
		state = "degraded"
		if code == "credentials_required" || code == "binary_changed" || code == "offline" || code == "rate_limited" {
			state = code
		}
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	_, _ = dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassBackgroundMedia, `UPDATE remote_sources SET state=?,error_code=? WHERE id=? AND CASE WHEN kind='rclone' THEN (SELECT CAST(m.generation AS TEXT) FROM mount_backend_configs m WHERE m.mount_id=remote_sources.mount_id) ELSE CAST(generation AS TEXT) END=? AND removed=0`, state, code, b.ID, generation)
}
func davSnapshot(e remotemedia.DAVEntry, root string) storage.Snapshot {
	modified := int64(0)
	if t, err := httpTime(e.Modified); err == nil {
		modified = t.UnixNano()
	}
	raw, _ := json.Marshal([]any{e.Relative, e.ETag, e.Size, e.Modified, e.ContentType, e.Directory})
	return storage.Snapshot{Path: filepath.Join(root, filepath.FromSlash(e.Relative)), Name: filepath.Base(e.Relative), Size: e.Size, ModifiedNS: modified, Directory: e.Directory, ObjectID: identity.Digest(e.Relative), Revision: identity.Digest(string(raw))}
}
func httpTime(v string) (time.Time, error) {
	for _, layout := range []string{time.RFC1123, time.RFC1123Z, time.RFC850, time.ANSIC} {
		if t, e := time.Parse(layout, v); e == nil {
			return t, nil
		}
	}
	return time.Time{}, storage.ErrRemoteConfig
}
func (s *Service) Stat(ctx context.Context, path string) (storage.Snapshot, error) {
	b, relative, ok := s.find(path)
	if !ok || b.Removed {
		return storage.Snapshot{}, storage.ErrRemoteOffline
	}
	var value storage.Snapshot
	var generation string
	var err error
	if b.Kind == "rclone" {
		var n *mounts.NativeBackend
		n, err = s.mounts.Native(ctx, b.ID)
		if err == nil {
			generation = n.Generation()
			var e mounts.NativeEntry
			e, err = n.Stat(ctx, relative)
			value = e.Snapshot(b.Root, relative)
		}
	} else {
		var d *remotemedia.DAV
		d, generation, err = s.dav(ctx, b.ID)
		if err == nil {
			var e remotemedia.DAVEntry
			e, err = d.Stat(ctx, relative)
			value = davSnapshot(e, b.Root)
		}
	}
	err = mapError(err)
	if err == nil {
		err = s.check(ctx, b, generation)
	}
	value = inventorySnapshot(b, generation, value)
	s.health(b, generation, err)
	return value, err
}
func (s *Service) InspectRoot(ctx context.Context, path string) (storage.Snapshot, error) {
	v, err := s.Stat(ctx, path)
	if err == nil && !v.Directory {
		err = storage.ErrRemoteConfig
	}
	if err == nil {
		v = inventoryRootSnapshot(v)
	}
	return v, err
}
func (s *Service) PlaybackActive(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.active[id] {
		if r.lane == "playback" {
			return true
		}
	}
	return false
}
func (s *Service) invalidate(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.active[id] {
		r.cancel()
	}
}

// Run reconciles mounts and sweeps the listing cache. Both are consequences of
// writes — a source added, a scan finished, a job cancelled — so the loop is
// woken by commits rather than by a thirty-second clock that issued three delete
// transactions on an idle server forever. The safety tick is the backstop.
func (s *Service) Run(ctx context.Context) {
	defer s.stopNativeListings()
	defer func() {
		s.mu.Lock()
		for _, reads := range s.active {
			for _, r := range reads {
				r.cancel()
			}
		}
		s.mu.Unlock()
	}()
	wake := worker.NewSignal()
	dbwork.WakeOnCommit(wake)
	worker.Run(ctx, "remotesources", wake, func(ctx context.Context) time.Duration {
		_ = s.RefreshMounts(ctx)
		// Completed/failed old scan listings are disposable adapter cache; current
		// pending cursors keep their snapshots regardless of wall-clock age.
		_, _ = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM remote_listing_sessions WHERE created_at<? AND NOT EXISTS(SELECT 1 FROM jobs j WHERE (j.id=remote_listing_sessions.job_id OR EXISTS(SELECT 1 FROM remote_inventory_checks c WHERE c.check_job=remote_listing_sessions.job_id AND c.job_id=j.id)) AND j.status IN ('queued','running','paused'))`, time.Now().Add(-24*time.Hour).Unix())
		_, _ = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM remote_inventory_checks WHERE NOT EXISTS(SELECT 1 FROM remote_listing_sessions l WHERE l.id=remote_inventory_checks.listing_id)`)
		_, _ = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `DELETE FROM remote_scan_cursors WHERE NOT EXISTS(SELECT 1 FROM remote_listing_sessions s WHERE s.id=remote_scan_cursors.listing_id)`)
		return 0
	})
}
func safeOrigin(root string) string {
	u, e := url.Parse(root)
	if e != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
