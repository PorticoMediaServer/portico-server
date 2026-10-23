package remotesources

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strconv"
	"sync/atomic"

	"portico.local/server/internal/mounts"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

// nativeSourceCandidate participates in the existing owner save. Address facts
// are prepared before its connection test; nothing is stored until the real
// test, fresh DNS/actor rechecks and the existing config/receipt transaction.
func (s *Service) nativeSourceCandidate(life context.Context, raw []byte, remote string, authorize func(*sql.Tx) error) (*mounts.SourceCandidatePublication, error) {
	backend, host, port, err := serveTransport(raw, remote)
	if err != nil {
		return nil, err
	}
	if backend != "sftp" && backend != "smb" {
		return nil, nil // No native authority is written for an HTTP source.
	}
	if s == nil || s.mounts == nil || life == nil {
		return nil, storage.ErrRemoteConfig
	}
	if err = s.authorize(life, authorize); err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return nil, storage.ErrRemoteConfig
	}
	endpoint := remotemedia.NativeEndpoint{Scheme: backend, Host: host, Port: n}
	policy := remotemedia.Policy{Resolver: s.resolver}
	approval, err := policy.PrepareNativeApproval(life, endpoint)
	if err != nil {
		return nil, err
	}
	configuration := nativeSourceConfiguration{Host: host, Port: n, ConfigurationDigest: hex.EncodeToString(sha256Sum(raw)), Approval: approval}
	var checked, used atomic.Bool
	return &mounts.SourceCandidatePublication{
		CheckAfterTest: func(ctx context.Context) error {
			if err := life.Err(); err != nil {
				return err
			}
			if err := s.authorize(ctx, authorize); err != nil {
				return err
			}
			if _, err := policy.CheckNativeEndpoint(ctx, endpoint, approval); err != nil {
				return err
			}
			if err := s.authorize(ctx, authorize); err != nil {
				return err
			}
			checked.Store(true)
			return nil
		},
		CommitTx: func(ctx context.Context, tx *sql.Tx, mount mounts.Mount) error {
			if tx == nil || !checked.Load() || !used.CompareAndSwap(false, true) || mount.ID == "" || mount.BackendRevision < 1 || !filepath.IsAbs(mount.MountPath) || filepath.Clean(mount.MountPath) != mount.MountPath {
				return storage.ErrRemoteConfig
			}
			if err := life.Err(); err != nil {
				return err
			}
			if err := authorize(tx); err != nil {
				return err
			}
			var generation int64
			var root, selectedRemote string
			if err := tx.QueryRowContext(ctx, `SELECT b.generation,m.mount_path,m.remote FROM mount_backend_configs b JOIN managed_mounts m ON m.id=b.mount_id WHERE m.id=?`, mount.ID).Scan(&generation, &root, &selectedRemote); err != nil {
				return err
			}
			if generation != mount.BackendRevision || root != mount.MountPath || selectedRemote != remote {
				return storage.ErrRemoteChanged
			}
			encoded, err := json.Marshal(configuration)
			if err != nil {
				return err
			}
			sealed, err := s.mounts.Seal(encoded)
			clear(encoded)
			if err != nil {
				return err
			}
			result, err := tx.ExecContext(ctx, `INSERT INTO remote_sources(id,kind,name,root,generation,config,state,error_code,range_support,origin,credential_present,mount_id)
 VALUES(?,?,?,?,?,?,'ready','','unknown',?,1,?) ON CONFLICT(id) DO UPDATE SET
 kind=excluded.kind,name=excluded.name,generation=excluded.generation,config=excluded.config,state='ready',error_code='',range_support='unknown',origin=excluded.origin,credential_present=1
 WHERE remote_sources.root=excluded.root AND remote_sources.mount_id=excluded.mount_id AND remote_sources.removed=0 AND remote_sources.generation<=excluded.generation AND remote_sources.kind IN('rclone','sftp','smb')`, mount.ID, backend, mount.Name, root, generation, sealed, approval.Root, mount.ID)
			if err != nil {
				return err
			}
			count, err := result.RowsAffected()
			if err == nil && count != 1 {
				return storage.ErrRemoteChanged
			}
			return err
		},
	}, nil
}
