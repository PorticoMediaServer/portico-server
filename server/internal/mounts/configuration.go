package mounts

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"strings"
	"time"

	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

// S3Setup is a bounded managed form, not a generic config/command passthrough.
// OAuth-only backends remain available through imported owner-authorized config.
type S3Setup struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
}

func managedS3(v S3Setup) (string, string, error) {
	for _, s := range []string{v.Endpoint, v.Region, v.AccessKey, v.SecretKey, v.Bucket, v.Prefix} {
		if len(s) > 4096 || strings.ContainsAny(s, "\x00\r\n") {
			return "", "", ErrConfigInvalid
		}
	}
	if v.AccessKey == "" || v.SecretKey == "" || v.Bucket == "" || strings.ContainsAny(v.Bucket, "/:\\") || !nativeRelative(v.Prefix) {
		return "", "", ErrConfigInvalid
	}
	if v.Endpoint != "" && !strings.HasPrefix(v.Endpoint, "https://") {
		return "", "", ErrConfigInvalid
	}
	config := "[portico]\ntype = s3\nprovider = Other\nenv_auth = false\naccess_key_id = " + v.AccessKey + "\nsecret_access_key = " + v.SecretKey + "\nregion = " + v.Region + "\n"
	if v.Endpoint != "" {
		config += "endpoint = " + v.Endpoint + "\n"
	}
	remote := "portico:" + v.Bucket
	if v.Prefix != "" {
		remote += "/" + v.Prefix
	}
	return config, remote, nil
}

func (s *Service) configure(ctx context.Context, actor, id, hash string, c Command, authorize func(*sql.Tx) error) (Receipt, error) {
	var zero Receipt
	if id == "" || c.ExpectedRevision < 1 {
		return zero, ErrInvalid
	}
	m, err := s.get(id)
	if err != nil {
		return zero, err
	}
	if m.ControlRevision != c.ExpectedRevision || m.RemovalPending {
		return zero, ErrCommandConflict
	}
	b, err := s.Native(ctx, id)
	if err != nil {
		return zero, err
	}
	// A changed binary can be explicitly reapproved; Native only checks the
	// configuration generation, not the binary digest, until a lifecycle starts.
	if c.Name == "" {
		c.Name = m.Name
	}
	if c.Executable == "" {
		c.Executable = m.Executable
	}
	if c.Remote == "" {
		c.Remote = b.remote
	}
	if strings.TrimSpace(c.Name) == "" || len(c.Name) > 100 {
		return zero, ErrInvalid
	}
	raw := []byte(c.Config)
	if len(raw) == 0 {
		stored, e := os.ReadFile(b.config)
		if e != nil {
			return zero, ErrConfigInvalid
		}
		if strings.HasPrefix(string(stored), sealedConfigPrefix) {
			// Sealed-era file the migration could not convert: re-enter the
			// configuration instead of refusing to start.
			return zero, ErrConfigInvalid
		}
		raw = stored
	}
	if err = validateConfig(string(raw), c.Remote); err != nil {
		return zero, ErrConfigInvalid
	}
	executable, digest, version, err := s.approveExecutable(ctx, c.Executable)
	if err != nil {
		return zero, err
	}
	candidate, err := s.candidateFile(raw)
	clear(raw)
	if err != nil {
		return zero, err
	}
	defer os.Remove(candidate)
	if err = s.validateCandidate(ctx, executable, digest, c.Remote, candidate); err != nil {
		return zero, err
	}
	_, _, cache, floor, err := s.BackendInfo(ctx, id)
	if err != nil {
		return zero, err
	}
	if c.CacheBytes != nil {
		cache = *c.CacheBytes
	}
	if c.CacheFloor != nil {
		floor = *c.CacheFloor
	}
	if cache < 0 || cache > 64<<30 || cache != 0 && cache < 64<<20 || floor < 1<<30 || floor > 1<<40 {
		return zero, ErrInvalid
	}
	// Immutable installation first, active pointer/receipt atomically second.
	// An abandoned generation is recoverable private state, never an active config.
	file := id + ".g-" + identity.Token() + ".conf"
	installed := filepath.Join(s.private, file)
	if err = os.Rename(candidate, installed); err != nil {
		return zero, err
	}
	if err = syncDirectory(s.private); err != nil {
		return zero, err
	}
	committed := false
	defer func() {
		if !committed {
			os.Remove(installed)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.publish()
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if err != nil {
		return zero, err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if err = authorize(tx); err != nil {
		return zero, err
	}
	result, err := tx.Exec(`UPDATE mount_controls SET revision=revision+1,restart_generation=restart_generation+1 WHERE mount_id=? AND revision=? AND removal_pending=0`, id, c.ExpectedRevision)
	if err != nil {
		return zero, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return zero, ErrCommandConflict
	}
	if _, err = tx.Exec(`UPDATE mount_backend_configs SET generation=generation+1,config_file=?,version=?,cache_bytes=?,cache_floor=? WHERE mount_id=?`, file, version, cache, floor, id); err != nil {
		return zero, err
	}
	if _, err = tx.Exec(`UPDATE managed_mounts SET name=?,executable=?,digest=?,remote=?,error='' WHERE id=?`, c.Name, executable, digest, c.Remote, id); err != nil {
		return zero, err
	}
	m.Name = c.Name
	m.Executable = executable
	m.ExecutableDigest = digest
	m.ExecutableVersion = version
	m.ControlRevision++
	m.BackendRevision++
	m.CacheBytes, m.CacheFloor = cache, floor
	m.Error = ""
	receipt := Receipt{c.OperationID, id, m.ControlRevision, true, false, &m}
	if err = s.record(tx, actor, hash, receipt); err != nil {
		return zero, err
	}
	if err = gated.Commit(); err != nil {
		return zero, err
	}
	committed = true
	if s.NativeInvalidated != nil {
		s.NativeInvalidated(id)
	}
	s.available[id] = false
	if child := s.active[id]; child != nil && !child.stopping {
		child.invalidateRuntime()
		child.stopping = true
		child.stopAt = time.Now()
		_ = child.control.Close()
	}
	delete(s.retry, id)
	return receipt, nil
}

// Cleanup only private, unreferenced candidate/generation files. No remote data
// or mount path is traversed. Retain old generations for a day for late helpers.
func (s *Service) recoverNativeFiles() {
	entries, err := os.ReadDir(s.private)
	if err != nil {
		return
	}
	used := map[string]bool{}
	rows, err := s.db.Query(`SELECT config_file FROM mount_backend_configs`)
	if err != nil {
		return
	}
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			used[name] = true
		}
	}
	rows.Close()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || used[name] || !(strings.HasSuffix(name, ".candidate") || strings.Contains(name, ".g-") && strings.HasSuffix(name, ".conf")) {
			continue
		}
		info, err := e.Info()
		if err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			_ = os.Remove(filepath.Join(s.private, name))
		}
	}
}

var _ = errors.Is
var _ = storage.ErrRemoteOffline

func (s *Service) PrivateDigest(raw []byte) string {
	return s.fingerprint("remote-sources", "", Command{Config: string(raw)})
}

// CacheSpaceAvailable checks the real host free-space floor for private native
// acquisition caches as well as optional compatibility mounts.
func CacheSpaceAvailable(path string, floor int64) bool { return cacheSpaceAvailable(path, floor) }
