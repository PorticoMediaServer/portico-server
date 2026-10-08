package recordingmedia

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"portico.local/server/internal/livechannels/dvr"
	"portico.local/server/internal/mediaartifact"
	"time"
)

func (d *Driver) SetPolicySource(source func() dvr.StoragePolicy) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	d.policySource = source
}
func (d *Driver) policy() dvr.StoragePolicy {
	if d.policySource != nil {
		return d.policySource()
	}
	return dvr.StoragePolicy{}
}
func (d *Driver) checkRoot() error {
	info, e := os.Lstat(d.root)
	if e != nil || !info.IsDir() || !os.SameFile(info, d.rootIdentity) {
		return dvr.ErrStorageUnavailable
	}
	return nil
}
func (d *Driver) Measure(ctx context.Context) (dvr.StorageMeasurement, error) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	var out dvr.StorageMeasurement
	if e := d.checkRoot(); e != nil {
		d.writeHealthy = false
		return out, e
	}
	free, e := freeBytes(d.root)
	if e != nil {
		d.writeHealthy = false
		return out, e
	}
	var used int64
	e = filepath.WalkDir(d.root, func(path string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return mediaartifact.ErrPrivate
		}
		if !entry.IsDir() {
			info, e := entry.Info()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return mediaartifact.ErrPrivate
			}
			used += info.Size()
		}
		return nil
	})
	if e != nil {
		d.writeHealthy = false
		return out, e
	}
	// An actual create/sync/remove probe checks permissions and a read-only/full
	// filesystem. It contains no media and stays inside the private managed root.
	probe, e := os.CreateTemp(d.root, ".write-health-")
	if e == nil {
		_, e = probe.Write([]byte{0})
		if e == nil {
			e = probe.Sync()
		}
		e2 := probe.Close()
		if e == nil {
			e = e2
		}
		e2 = os.Remove(probe.Name())
		if e == nil {
			e = e2
		}
	}
	d.used = used
	d.writeHealthy = e == nil
	out = dvr.StorageMeasurement{FreeBytes: free, UsedBytes: used, WriteHealthy: d.writeHealthy, MeasuredAt: time.Now().UTC().Format(time.RFC3339Nano)}
	for _, v := range d.reserved {
		out.ReservedBytes += v
	}
	return out, nil
}
func (d *Driver) reserve(in dvr.CaptureRequest) error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	if e := d.checkRoot(); e != nil {
		return e
	}
	free, e := freeBytes(d.root)
	if e != nil {
		return dvr.ErrStorageUnavailable
	}
	var reserved int64
	for _, v := range d.reserved {
		reserved += v
	}
	estimate := max(int64(0), in.EstimatedBytes)
	p := d.policy()
	if free-reserved-estimate < p.FloorBytes {
		return dvr.ErrStorageFloor
	}
	if p.CapBytes > 0 && d.used+reserved+estimate > p.CapBytes {
		return dvr.ErrStorageCap
	}
	d.reserved[in.Recording.ID] = estimate
	return nil
}
func (d *Driver) releaseReservation(id string) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	delete(d.reserved, id)
}
func (d *Driver) writeCapture(id string, w *mediaartifact.Writer, b []byte) (int, error) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	if e := d.checkRoot(); e != nil {
		d.writeHealthy = false
		return 0, e
	}
	free, e := freeBytes(d.root)
	if e != nil {
		d.writeHealthy = false
		return 0, dvr.ErrStorageUnavailable
	}
	p := d.policy()
	var other int64
	for key, v := range d.reserved {
		if key != id {
			other += v
		}
	}
	if free-int64(len(b))-other < p.FloorBytes {
		return 0, dvr.ErrStorageFloor
	}
	if p.CapBytes > 0 && d.used+int64(len(b))+other > p.CapBytes {
		return 0, dvr.ErrStorageCap
	}
	n, e := w.Write(b)
	d.used += int64(n)
	d.reserved[id] = max(int64(0), d.reserved[id]-int64(n))
	d.writeHealthy = e == nil
	if e != nil {
		return n, dvr.ErrStorageUnavailable
	}
	return n, nil
}

// CheckFloor performs the real filesystem check before the scheduler's write
// transaction. Claim revalidates logical facts; reservation and every write
// recheck the volume, closing the unavoidable filesystem/SQLite timing gap.
func (d *Driver) CheckFloor(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	if e := d.checkRoot(); e != nil {
		return e
	}
	free, e := freeBytes(d.root)
	if e != nil {
		return dvr.ErrStorageUnavailable
	}
	var reserved int64
	for _, v := range d.reserved {
		reserved += v
	}
	p := d.policy()
	if free-reserved <= p.FloorBytes {
		return dvr.ErrStorageFloor
	}
	if p.CapBytes > 0 && d.used+reserved >= p.CapBytes {
		return dvr.ErrStorageCap
	}
	return nil
}
