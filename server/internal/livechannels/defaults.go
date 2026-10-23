package livechannels

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"portico.local/server/internal/dbwork"
)

func DefaultLiveDefaults() LiveDefaults {
	return LiveDefaults{ImportLimits: ImportLimits{MaxUploadBytes, MaxChannels, MaxProgrammes}, StreamBufferMs: 8000, RetryWindowMs: 60000, UserAgent: "Portico/1.0", GuideDays: 7, LogoImport: true, DiscoveryEnabled: true, TimeShiftMinutes: 30}
}

// ReadLiveDefaultsTx is the single normalized global source used by imports,
// owner settings and new live buffers. Source overrides are separate.
func ReadLiveDefaultsTx(ctx context.Context, tx *sql.Tx) (LiveDefaults, int64, error) {
	if tx == nil {
		return LiveDefaults{}, 0, ErrInvalid
	}
	v := DefaultLiveDefaults()
	revision := int64(1)
	err := tx.QueryRowContext(ctx, `SELECT revision,import_bytes,import_channels,import_programmes,stream_buffer_ms,retry_window_ms,user_agent,guide_days,logo_import,discovery_enabled,timeshift_minutes FROM live_defaults WHERE singleton=1`).Scan(&revision, &v.ImportLimits.Bytes, &v.ImportLimits.Channels, &v.ImportLimits.Programmes, &v.StreamBufferMs, &v.RetryWindowMs, &v.UserAgent, &v.GuideDays, &v.LogoImport, &v.DiscoveryEnabled, &v.TimeShiftMinutes)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return LiveDefaults{}, 0, err
	}
	if _, _, err = v.TimeShiftLimits(); err != nil {
		return LiveDefaults{}, 0, err
	}
	return v, revision, nil
}

// ReplaceLiveDefaultsTx must be called by the authorized administration writer
// after validating the complete value. CAS and its receipt share that transaction.
func ReplaceLiveDefaultsTx(ctx context.Context, tx *sql.Tx, expected int64, v LiveDefaults) (int64, error) {
	if tx == nil || expected < 1 {
		return 0, ErrInvalid
	}
	if _, _, err := v.TimeShiftLimits(); err != nil {
		return 0, err
	}
	_, revision, err := ReadLiveDefaultsTx(ctx, tx)
	if err != nil {
		return 0, err
	}
	if revision != expected {
		return 0, ErrConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO live_defaults(singleton,revision,import_bytes,import_channels,import_programmes,stream_buffer_ms,retry_window_ms,user_agent,guide_days,logo_import,discovery_enabled,timeshift_minutes) VALUES(1,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(singleton) DO UPDATE SET revision=excluded.revision,import_bytes=excluded.import_bytes,import_channels=excluded.import_channels,import_programmes=excluded.import_programmes,stream_buffer_ms=excluded.stream_buffer_ms,retry_window_ms=excluded.retry_window_ms,user_agent=excluded.user_agent,guide_days=excluded.guide_days,logo_import=excluded.logo_import,discovery_enabled=excluded.discovery_enabled,timeshift_minutes=excluded.timeshift_minutes`, revision+1, v.ImportLimits.Bytes, v.ImportLimits.Channels, v.ImportLimits.Programmes, v.StreamBufferMs, v.RetryWindowMs, v.UserAgent, v.GuideDays, v.LogoImport, v.DiscoveryEnabled, v.TimeShiftMinutes)
	return revision + 1, err
}

// TimeShiftLimits retains the drawn duration and its matching byte budget.
// It is a cap, never a promise that a provider delivered that much history.
func (v LiveDefaults) TimeShiftLimits() (time.Duration, int64, error) {
	switch v.TimeShiftMinutes {
	case 10, 30, 60, 120:
	default:
		return 0, 0, ErrInvalid
	}
	return time.Duration(v.TimeShiftMinutes) * time.Minute, min(int64(v.TimeShiftMinutes)*150_000_000, int64(16<<30)), nil
}

func ReadTimeShiftLimits(ctx context.Context, db *sql.DB) (time.Duration, int64, error) {
	snapshot, err := dbwork.BeginSnapshot(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	defer snapshot.Rollback()
	v, _, err := ReadLiveDefaultsTx(ctx, snapshot.Tx())
	if err != nil {
		return 0, 0, err
	}
	return v.TimeShiftLimits()
}
