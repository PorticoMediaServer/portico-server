package dvr

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/livechannels"
	"time"
)

var ErrStorageFloor = errors.New("recording storage free-space floor reached")
var ErrStorageCap = errors.New("recording storage cap reached")
var ErrStorageUnavailable = errors.New("recording storage is not writable")

// StorageDriver measures the private recording root and serializes reservations
// and writes. Values are local facts, never a replacement for runtime write checks.
type StorageDriver interface {
	Measure(context.Context) (StorageMeasurement, error)
	CheckFloor(context.Context) error
	SetPolicySource(func() StoragePolicy)
}
type StorageMeasurement struct {
	FreeBytes     int64  `json:"freeBytes"`
	UsedBytes     int64  `json:"usedBytes"`
	ReservedBytes int64  `json:"reservedBytes"`
	WriteHealthy  bool   `json:"writeHealthy"`
	MeasuredAt    string `json:"measuredAt"`
}
type StorageStatus struct {
	Policy              StoragePolicy      `json:"policy"`
	Measurement         StorageMeasurement `json:"measurement"`
	PendingDeleteBytes  int64              `json:"pendingDeleteBytes"`
	ForecastBytes       int64              `json:"forecastBytes"`
	ForecastHours       int                `json:"forecastHours"`
	ForecastDescription string             `json:"forecastDescription"`
	CaptureAvailable    bool               `json:"captureAvailable"`
	Warning             string             `json:"warning"`
}
type Usage struct {
	Bytes              int64 `json:"bytes"`
	PendingDeleteBytes int64 `json:"pendingDeleteBytes"`
	Recordings         int   `json:"recordings"`
}

func (s *Store) StorageLimits() StoragePolicy {
	if p, ok := s.policy.Load().(StoragePolicy); ok {
		return p
	}
	return StoragePolicy{}
}

// cacheStoragePolicy publishes only newer committed revisions. A receipt replay
// returns its original response, but must never roll back the live write fence.
// A slow maintenance read must not overwrite a concurrently committed policy.
func (s *Store) cacheStoragePolicy(p StoragePolicy) {
	for {
		old := s.policy.Load()
		if current, ok := old.(StoragePolicy); ok && current.Revision >= p.Revision {
			return
		}
		if s.policy.CompareAndSwap(old, p) {
			return
		}
	}
}
func (s *Store) refreshStoragePolicy(ctx context.Context) error {
	var p StoragePolicy
	e := s.db.QueryRowContext(ctx, `SELECT revision,retention_days,episode_limit,floor_bytes,cap_bytes FROM dvr_storage_policy WHERE singleton=1`).Scan(&p.Revision, &p.RetentionDays, &p.EpisodeLimit, &p.FloorBytes, &p.CapBytes)
	if e == nil {
		s.cacheStoragePolicy(p)
	}
	return e
}

// Estimate uses the source's recent measured byte rate with 25% headroom. With
// no evidence it uses 20 Mb/s, and labels it an estimate, not a storage guarantee.
func estimateBytesTx(ctx context.Context, tx *sql.Tx, source string, seconds int64) (int64, error) {
	var rate float64
	e := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(bytes*1000.0/(captured_end_ms-captured_start_ms)),0) FROM (SELECT bytes,captured_start_ms,captured_end_ms FROM dvr_recordings WHERE source_id=? AND state IN('completed','incomplete-playable') AND captured_end_ms>captured_start_ms AND bytes>0 ORDER BY finished_ms DESC LIMIT 16)`, source).Scan(&rate)
	if e != nil {
		return 0, e
	}
	rate = max(float64(2500000), rate*1.25)
	rate = min(rate, float64(1<<30))
	return int64(rate * float64(max(int64(0), min(seconds, int64((1<<53-1)/(1<<30)))))), nil
}
func (s *Store) StorageStatus(ctx context.Context, a livechannels.Authority) (StorageStatus, error) {
	out := StorageStatus{ForecastHours: 24, ForecastDescription: "Next 24 hours. Recent measured source rate plus 25% headroom, or 20 Mb/s without evidence. Forecasts are estimates; free-space floor and cap are checked on every write.", CaptureAvailable: s.captureAvailable}
	p, e := s.GetStoragePolicy(ctx, a)
	if e != nil {
		return out, e
	}
	out.Policy = p
	if s.storage == nil {
		out.Warning = "storage-unavailable"
		return out, nil
	}
	measurement, e := s.storage.Measure(ctx)
	if e != nil {
		out.Warning = "storage-unavailable"
	} else {
		out.Measurement = measurement
	}
	gated, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if a == nil {
		return out, ErrDenied
	}
	if f, _, e := a(ctx, tx, true); e != nil || f == "" {
		return out, ErrDenied
	}
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(bytes),0) FROM dvr_recordings WHERE state='pending-delete'`).Scan(&out.PendingDeleteBytes); e != nil {
		return out, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT source_id,SUM(max(0,(min(end_ms,?)-max(start_ms,?))/1000)) FROM dvr_recordings WHERE state IN('scheduled','conflicted','waiting-source','waiting-guide','preparing','recording') AND start_ms<? AND end_ms>? GROUP BY source_id`, s.now().Add(24*time.Hour).UnixMilli(), s.now().UnixMilli(), s.now().Add(24*time.Hour).UnixMilli(), s.now().UnixMilli())
	if e != nil {
		return out, e
	}
	type forecast struct {
		source  string
		seconds int64
	}
	sources := []forecast{}
	for rows.Next() {
		var f forecast
		if e = rows.Scan(&f.source, &f.seconds); e != nil {
			rows.Close()
			return out, e
		}
		sources = append(sources, f)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, f := range sources {
		b, e := estimateBytesTx(ctx, tx, f.source, f.seconds)
		if e != nil {
			return out, e
		}
		out.ForecastBytes = min(int64(1<<53-1), out.ForecastBytes+b)
	}
	if out.Warning == "" && (!measurement.WriteHealthy || measurement.FreeBytes < p.FloorBytes+out.ForecastBytes || (p.CapBytes > 0 && measurement.UsedBytes+out.ForecastBytes > p.CapBytes)) {
		out.Warning = "storage-pressure"
	}
	return out, nil
}
func usageTx(ctx context.Context, tx *sql.Tx, o livechannels.Owner) (Usage, error) {
	var u Usage
	e := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(bytes),0),COALESCE(SUM(CASE WHEN state='pending-delete' THEN bytes ELSE 0 END),0),COUNT(*) FROM dvr_recordings WHERE owner_key=? AND state!='deleted'`, o.Key()).Scan(&u.Bytes, &u.PendingDeleteBytes, &u.Recordings)
	return u, e
}
