package recordingmedia

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"portico.local/server/internal/livechannels/dvr"
)

// Share installs another profile's immutable path using a hard link. Each
// profile retains independent catalogue authority, retirement and retention.
// The filesystem releases physical bytes only when the final path is removed.
func (d *Driver) Share(ctx context.Context, target, source dvr.CaptureRequest, cp dvr.CaptureCheckpoint) (dvr.CaptureResult, error) {
	original, _, e := d.store(source.Owner, source.ArtifactKey)
	if e != nil {
		return dvr.CaptureResult{}, e
	}
	reader, e := original.Open(ctx, cp.Object)
	if e != nil {
		return dvr.CaptureResult{}, e
	}
	defer reader.Close()
	origin, e := original.ObjectPath(cp.Object)
	if e != nil {
		return dvr.CaptureResult{}, e
	}
	destination, root, e := d.store(target.Owner, target.ArtifactKey)
	if e != nil {
		return dvr.CaptureResult{}, e
	}
	path := filepath.Join(destination.Root(), "objects", cp.Object.Digest)
	if e = os.Link(origin, path); e != nil && !errors.Is(e, os.ErrExist) {
		return dvr.CaptureResult{}, e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e != nil {
		return dvr.CaptureResult{}, e
	}
	e = errors.Join(dir.Sync(), dir.Close())
	if e != nil {
		return dvr.CaptureResult{}, e
	}
	linked, e := destination.Open(ctx, cp.Object)
	if e != nil {
		return dvr.CaptureResult{}, e
	}
	defer linked.Close()
	facts, e := d.probe(ctx, target.Recording.ID, linked, cp.Object.Size)
	if e != nil {
		return dvr.CaptureResult{}, e
	}
	return d.result(target, cp, root, path, facts)
}
