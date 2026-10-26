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

// sourceChoiceApply is the committed half of a startChoice command. It is used
// only after the accepting transaction commits, for the transport effect that
// does not cross the held cache/reference seam. It is never stored or replayed.
type sourceChoiceApply struct {
	source           Source
	transportChanged bool
}

// applySourceAdvancedValuesTx is the sole Advanced row writer body. The caller
// owns the transaction, owner/generation/revision checks and optional-field
// merge. Zero scan interval clears the real enabled library schedules so
// "Only when I ask" stops automatic work rather than merely storing zero.
func (s *Service) applySourceAdvancedValuesTx(ctx context.Context, tx *sql.Tx, source string, current advancedRow, values SourceAdvanced) (advancedRow, error) {
	if tx == nil || source == "" || !values.valid() {
		return advancedRow{}, storage.ErrRemoteConfig
	}
	if current.revision == 1<<63-1 {
		return advancedRow{}, storage.ErrRemoteLimit
	}
	revision := current.revision
	nextRevision := revision + 1
	if revision == 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO remote_source_settings(source_id,revision,scan_interval_seconds,watch_for_changes,scan_on_start,full_recheck_days,cache_bytes,cache_floor_bytes,seek_previews,chapter_images,waveforms,loudness,checksums,intros_mode,double_check_folders,dir_cache_seconds,read_buffer_bytes,connect_timeout_seconds,idle_timeout_seconds,retries,low_level_retries) VALUES(?,1,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			source, values.ScanIntervalSeconds, values.WatchForChanges, values.ScanOnStart, values.FullRecheckDays, values.CacheBytes, values.CacheFloorBytes, values.SeekPreviews, values.ChapterImages, values.Waveforms, values.Loudness, values.Checksums, values.IntrosMode, values.DoubleCheckFolders, values.DirCacheSeconds, values.ReadBufferBytes, values.ConnectTimeoutSeconds, values.IdleTimeoutSeconds, values.Retries, values.LowLevelRetries); err != nil {
			return advancedRow{}, err
		}
	} else {
		result, err := tx.ExecContext(ctx, `UPDATE remote_source_settings SET revision=revision+1,scan_interval_seconds=?,watch_for_changes=?,scan_on_start=?,full_recheck_days=?,cache_bytes=?,cache_floor_bytes=?,seek_previews=?,chapter_images=?,waveforms=?,loudness=?,checksums=?,intros_mode=?,double_check_folders=?,dir_cache_seconds=?,read_buffer_bytes=?,connect_timeout_seconds=?,idle_timeout_seconds=?,retries=?,low_level_retries=? WHERE source_id=? AND revision=?`,
			values.ScanIntervalSeconds, values.WatchForChanges, values.ScanOnStart, values.FullRecheckDays, values.CacheBytes, values.CacheFloorBytes, values.SeekPreviews, values.ChapterImages, values.Waveforms, values.Loudness, values.Checksums, values.IntrosMode, values.DoubleCheckFolders, values.DirCacheSeconds, values.ReadBufferBytes, values.ConnectTimeoutSeconds, values.IdleTimeoutSeconds, values.Retries, values.LowLevelRetries, source, revision)
		if err != nil {
			return advancedRow{}, err
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return advancedRow{}, storage.ErrRemoteChanged
		}
	}
	if values.ScanIntervalSeconds > 0 {
		// The drawn page owns these folders' check interval; the scanner reads
		// library_sources, so the same rows are updated here, once.
		next := time.Now().UTC().Add(time.Duration(values.ScanIntervalSeconds) * time.Second).Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE library_sources SET interval_seconds=?,next_scan_at=CASE WHEN next_scan_at='' OR next_scan_at>? THEN ? ELSE next_scan_at END WHERE remote_source_id=? AND interval_seconds IS NOT ?`, values.ScanIntervalSeconds, next, next, source, values.ScanIntervalSeconds); err != nil {
			return advancedRow{}, err
		}
	} else {
		// The ingestion scheduler gates on library_sources.interval_seconds>0
		// and next_scan_at; clear both for actual enabled folders.
		if _, err := tx.ExecContext(ctx, `UPDATE library_sources SET interval_seconds=0,next_scan_at='' WHERE remote_source_id=? AND enabled=1`, source); err != nil {
			return advancedRow{}, err
		}
	}
	if err := dbwork.AfterCommit(tx, func() { s.retireCaptureSettings(source, nextRevision) }); err != nil {
		return advancedRow{}, err
	}
	return advancedRow{
		values:                 values,
		saved:                  true,
		revision:               nextRevision,
		appliedBytes:           current.appliedBytes,
		appliedFloor:           current.appliedFloor,
		observedIntervalSecond: values.ScanIntervalSeconds,
	}, nil
}

// applySourceChoiceTx writes both settings halves in the caller's one accepting
// transaction. The recorded reference and cache application are held until the
// original causal mount-command receipt seam is grounded.
func (s *Service) applySourceChoiceTx(ctx context.Context, tx *sql.Tx, identities *identity.Service, owner identity.Principal, source string, in SourceReadSettingsChange) (Source, SourceReadSettings, sourceChoiceApply, error) {
	var zero Source
	var zeroSettings SourceReadSettings
	var zeroApply sourceChoiceApply
	choice := in.StartChoice
	if choice == nil || source == "" || choice.Name == "" || choice.DefinitionRevision < 1 || choice.ExpectedAdvancedRevision < 0 ||
		!validSourceReadProfile(in.Values.profile(source)) {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteConfig
	}
	row, err := sourceRowTx(ctx, tx, source)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return zero, zeroSettings, zeroApply, storage.ErrRemoteOffline
		}
		return zero, zeroSettings, zeroApply, err
	}
	if strconv.FormatInt(row.Generation, 10) != in.ExpectedGeneration {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteChanged
	}
	if !sourceChoiceGeneration(choice.ExpectedConfigurationGeneration) {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteConfig
	}
	effective, err := observationGenerationTx(ctx, tx, source)
	if err != nil {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteChanged
	}
	if effective != choice.ExpectedConfigurationGeneration {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteChanged
	}
	definition, err := readSourceChoiceDefinitionTx(ctx, tx, choice.Name, choice.DefinitionRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteConfig
	}
	if err != nil {
		return zero, zeroSettings, zeroApply, err
	}
	if !sourceChoiceReadingMatches(definition, in.Values) {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteConfig
	}
	reading, err := SourceReadProfileTx(ctx, tx, source)
	if err != nil {
		return zero, zeroSettings, zeroApply, err
	}
	advanced, err := readAdvancedTx(ctx, tx, source)
	if err != nil {
		return zero, zeroSettings, zeroApply, err
	}
	if reading.Revision != in.ExpectedRevision || advanced.revision != choice.ExpectedAdvancedRevision {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteChanged
	}
	applicable := sourceChoiceApplicabilityForSource(row)
	values := sourceChoicePreserveInapplicable(definition.Advanced, advanced.values, applicable)
	if !values.valid() {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteConfig
	}
	readResult, err := s.ApplySourceReadProfileTx(ctx, tx, identities, owner, SourceReadProfileChange{SourceID: source, Generation: effective, ExpectedRevision: reading.Revision, Profile: definition.Reading.profile(source)})
	if err != nil {
		return zero, zeroSettings, zeroApply, err
	}
	if readResult.Profile.Revision < 1 {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteChanged
	}
	written, err := s.applySourceAdvancedValuesTx(ctx, tx, source, advanced, values)
	if err != nil {
		return zero, zeroSettings, zeroApply, err
	}
	if written.revision < 1 {
		return zero, zeroSettings, zeroApply, storage.ErrRemoteChanged
	}
	// Cache application and the recorded reference are held until the causal
	// mount-command receipt seam is grounded. Writing the reference here would
	// bind a pre-cache generation that the ordinary mount/native save changes.
	_, settings, err := s.readSourceReadSettingsProjectionTx(ctx, tx, source)
	if err != nil {
		return zero, zeroSettings, zeroApply, err
	}
	apply := sourceChoiceApply{
		source:           row,
		transportChanged: advanced.values.transport() != values.transport(),
	}
	return row, settings, apply, nil
}

// finishSourceChoice runs only the transport effect that is independent of the
// held cache/reference seam. Cache application and recorded-reference creation
// remain unwired until the original causal mount-command receipt is grounded.
func (s *Service) finishSourceChoice(apply sourceChoiceApply) {
	if apply.transportChanged {
		s.transportCommitted(apply.source.ID)
	}
}
