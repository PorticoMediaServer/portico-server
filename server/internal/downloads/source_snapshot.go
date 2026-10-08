package downloads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/diskspace"
	"portico.local/server/internal/mediasource"
)

// Original downloads pin an immutable private snapshot, rather than treating a
// mutable library path as content identity. Arbitrary mounted roots cannot make
// the stronger no-in-place-mutation promise required by versioned source leases.
// The snapshot digest identifies exactly the bytes copied, even if the source
// was replaced during the copy. Existing source confinement stays in OpenSource.
func (s *Service) snapshotOriginal(ctx context.Context, id string, p plan) (plan, error) {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	if free, err := diskspace.Free(s.snapshotRoot); err == nil && p.size > free-diskspace.ProducerFloor {
		return p, ErrStorageFull
	}
	open := s.openSource
	if open == nil {
		open = directSource
	}
	source, err := open(ctx, p.path, p.size, p.modifiedNS)
	if err != nil {
		return p, err
	}
	defer source.Close()
	tmp, err := os.CreateTemp(s.snapshotRoot, ".copy-*")
	if err != nil {
		return p, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	digest := sha256.New()
	// One opened source, bounded memory, and an exact length. No checkpoint can
	// ever concatenate bytes reopened from two revisions of the library path.
	buf := make([]byte, 256<<10)
	var copied int64
	for copied < p.size {
		if copied%(32<<20) == 0 {
			var current bool
			if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM download_preparations WHERE id=? AND revision=? AND state='running')`, id, p.revision).Scan(&current); err != nil {
				return p, err
			}
			if !current {
				p.hashBytes = false
				return p, nil
			}
			if !diskspace.Room(s.snapshotRoot, diskspace.ProducerFloor) {
				return p, ErrStorageFull
			}
		}
		if err := ctx.Err(); err != nil {
			return p, err
		}
		n, err := io.ReadFull(source, buf[:min(int64(len(buf)), p.size-copied)])
		if err != nil {
			return p, err
		}
		if _, err = tmp.Write(buf[:n]); err != nil {
			return p, err
		}
		_, _ = digest.Write(buf[:n])
		copied += int64(n)
	}
	if err = tmp.Sync(); err != nil {
		return p, err
	}
	if err = tmp.Close(); err != nil {
		return p, err
	}
	evidence := mediasource.Evidence{Kind: mediasource.ContentSHA256, Scope: "download-source-snapshot", Object: id, Revision: hex.EncodeToString(digest.Sum(nil)), Size: copied}
	version, err := mediasource.NewVersion(evidence)
	if err != nil {
		return p, err
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return p, err
	}
	gate, err := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if err != nil {
		return p, err
	}
	defer gate.Rollback()
	var revision int64
	var state string
	if err = gate.Tx().QueryRowContext(ctx, `SELECT revision,state FROM download_preparations WHERE id=?`, id).Scan(&revision, &state); err != nil {
		return p, err
	}
	if revision != p.revision || state != StateRunning {
		p.hashBytes = false
		return p, nil
	}
	path := filepath.Join(s.snapshotRoot, id+".source")
	// No live grant exists until the subsequent hash verification publishes
	// ready. A crash here leaves an unclaimed file that the next pass collects.
	if err = os.Rename(tmp.Name(), path); err != nil {
		return p, err
	}
	if _, err = gate.Tx().ExecContext(ctx, `UPDATE download_preparations SET source_version_json=?,bytes_done=0,hash_state=NULL,revision=revision+1,updated_ms=? WHERE id=?`, string(raw), s.millis(), id); err != nil {
		return p, err
	}
	if err = gate.Commit(); err != nil {
		return p, err
	}
	p.path, p.offset, p.state, p.snapshotVersion = path, 0, nil, version
	p.revision++
	return p, nil
}

func sourceSnapshotVersion(raw, id string, size int64) (mediasource.Version, error) {
	var evidence mediasource.Evidence
	if err := json.Unmarshal([]byte(raw), &evidence); err != nil {
		return mediasource.Version{}, err
	}
	if evidence.Kind != mediasource.ContentSHA256 || evidence.Scope != "download-source-snapshot" || evidence.Object != id || evidence.Size != size || !validID.MatchString(id) {
		return mediasource.Version{}, mediasource.ErrEvidence
	}
	return mediasource.NewVersion(evidence)
}

func (s *Service) openSnapshot(version mediasource.Version) (io.ReadSeekCloser, error) {
	evidence := version.Evidence()
	if version.ID() == "" || !validID.MatchString(evidence.Object) {
		return nil, mediasource.ErrEvidence
	}
	root, err := os.OpenRoot(s.snapshotRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(evidence.Object + ".source")
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != evidence.Size {
		f.Close()
		return nil, mediasource.ErrSourceChanged
	}
	return f, nil
}

// Reclaim only artifacts owned by this subsystem and lacking a live claim.
// Retention and cancellation retire that claim before files become collectible.
// The same lock protects unpublished files from an overlapping worker pass.
func (s *Service) collectSnapshots(ctx context.Context) error {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	entries, err := os.ReadDir(s.snapshotRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) > 6 && name[:6] == ".copy-" {
			_ = os.Remove(filepath.Join(s.snapshotRoot, name))
			continue
		}
		if filepath.Ext(name) != ".source" {
			continue
		}
		id := name[:len(name)-len(".source")]
		if !validID.MatchString(id) {
			continue
		}
		var retained bool
		if err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM download_preparations WHERE id=? AND source_version_json<>'' AND state IN('queued','running','paused','ready'))`, id).Scan(&retained); err != nil {
			return err
		}
		if !retained {
			if err = os.Remove(filepath.Join(s.snapshotRoot, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
