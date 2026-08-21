package librarychannels

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) configured() error {
	if s == nil || s.db == nil {
		return errors.New("library channel store is not configured")
	}
	return nil
}

func (s *Store) SaveAggregate(ctx context.Context, aggregate Aggregate, expectedRevision int64) (Aggregate, error) {
	if err := s.configured(); err != nil {
		return Aggregate{}, err
	}
	if expectedRevision < 0 {
		return Aggregate{}, invalid("expectedRevision", "must not be negative")
	}
	if expectedRevision == 0 {
		aggregate.Channel.ConfigRevision = 1
	} else {
		aggregate.Channel.ConfigRevision = expectedRevision + 1
	}
	if aggregate.Channel.HealthState == "" {
		if aggregate.Channel.Enabled {
			aggregate.Channel.HealthState = "pending"
		} else {
			aggregate.Channel.HealthState = "disabled"
		}
	}
	if err := ValidateAggregate(aggregate); err != nil {
		return Aggregate{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Aggregate{}, fmt.Errorf("begin library channel configuration: %w", err)
	}
	defer tx.Rollback()
	now, err := dbNow(ctx, tx)
	if err != nil {
		return Aggregate{}, err
	}
	if expectedRevision == 0 {
		aggregate.Channel.CreatedAt, aggregate.Channel.UpdatedAt = now, now
		if err := insertChannel(ctx, tx, aggregate.Channel); err != nil {
			return Aggregate{}, err
		}
	} else {
		aggregate.Channel.UpdatedAt = now
		result, err := updateChannel(ctx, tx, aggregate.Channel, expectedRevision, now)
		if err != nil {
			if isTemplateUniquenessError(err) {
				return Aggregate{}, ErrTemplateExists
			}
			return Aggregate{}, err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			var exists int
			err := tx.QueryRowContext(ctx, `SELECT 1 FROM library_channels WHERE id=?`, aggregate.Channel.ID).Scan(&exists)
			if errors.Is(err, sql.ErrNoRows) {
				return Aggregate{}, ErrNotFound
			}
			if err != nil {
				return Aggregate{}, err
			}
			return Aggregate{}, ErrRevisionConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM library_channel_blocks WHERE channel_id=?`, aggregate.Channel.ID); err != nil {
			return Aggregate{}, fmt.Errorf("replace library channel blocks: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM library_channel_rules WHERE channel_id=?`, aggregate.Channel.ID); err != nil {
			return Aggregate{}, fmt.Errorf("replace library channel rules: %w", err)
		}
	}
	for i := range aggregate.Rules {
		aggregate.Rules[i].CreatedAt, aggregate.Rules[i].UpdatedAt = now, now
		if err := insertRule(ctx, tx, aggregate.Rules[i]); err != nil {
			return Aggregate{}, err
		}
	}
	for i := range aggregate.Blocks {
		aggregate.Blocks[i].CreatedAt, aggregate.Blocks[i].UpdatedAt = now, now
		if err := insertBlock(ctx, tx, aggregate.Blocks[i]); err != nil {
			return Aggregate{}, err
		}
	}
	if aggregate.Channel.Enabled {
		if err := enqueueGenerationTx(ctx, tx, aggregate.Channel.ID, aggregate.Channel.ConfigRevision, now); err != nil {
			return Aggregate{}, err
		}
	} else if _, err := tx.ExecContext(ctx, `DELETE FROM library_channel_generation_queue WHERE channel_id=?`, aggregate.Channel.ID); err != nil {
		return Aggregate{}, fmt.Errorf("remove disabled Library Channel generation request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Aggregate{}, fmt.Errorf("commit library channel configuration: %w", err)
	}
	return aggregate, nil
}

func (s *Store) GetAggregate(ctx context.Context, channelID string) (Aggregate, error) {
	if err := s.configured(); err != nil {
		return Aggregate{}, err
	}
	channel, err := scanChannel(s.db.QueryRowContext(ctx, channelSelect+` WHERE id=?`, channelID))
	if errors.Is(err, sql.ErrNoRows) {
		return Aggregate{}, ErrNotFound
	}
	if err != nil {
		return Aggregate{}, fmt.Errorf("load library channel: %w", err)
	}
	rules, err := s.loadRules(ctx, channelID)
	if err != nil {
		return Aggregate{}, err
	}
	blocks, err := s.loadBlocks(ctx, channelID)
	if err != nil {
		return Aggregate{}, err
	}
	return Aggregate{Channel: channel, Rules: rules, Blocks: blocks}, nil
}

func (s *Store) GetAggregateByTemplateKey(ctx context.Context, templateKey string) (Aggregate, error) {
	if err := s.configured(); err != nil {
		return Aggregate{}, err
	}
	var channelID string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM library_channels WHERE template_key=?`, strings.TrimSpace(templateKey)).Scan(&channelID); errors.Is(err, sql.ErrNoRows) {
		return Aggregate{}, ErrNotFound
	} else if err != nil {
		return Aggregate{}, err
	}
	return s.GetAggregate(ctx, channelID)
}

func (s *Store) ListChannels(ctx context.Context, includeDisabled bool) ([]Channel, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	query := channelSelect
	if !includeDisabled {
		query += ` WHERE enabled=1`
	}
	query += ` ORDER BY sort_order, lower(name), id`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list library channels: %w", err)
	}
	defer rows.Close()
	result := []Channel{}
	for rows.Next() {
		channel, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, channel)
	}
	return result, rows.Err()
}

func (s *Store) DeleteChannel(ctx context.Context, channelID string, expectedRevision int64) error {
	if err := s.configured(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM library_channels WHERE id=? AND config_revision=?`, channelID, expectedRevision)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 1 {
		return nil
	}
	var exists int
	err = s.db.QueryRowContext(ctx, `SELECT 1 FROM library_channels WHERE id=?`, channelID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return ErrRevisionConflict
}

// ChannelOrder is the complete optimistic-lock record used to reorder the
// administration list. Reordering is atomic: either every supplied revision
// still matches and all positions advance together, or nothing changes.
type ChannelOrder struct {
	ID               string
	ExpectedRevision int64
	SortOrder        int
}

func (s *Store) ReorderChannels(ctx context.Context, order []ChannelOrder) ([]Channel, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	if len(order) == 0 {
		return nil, invalid("items", "must contain the complete channel order")
	}
	seen := make(map[string]struct{}, len(order))
	for index, item := range order {
		if !validOpaqueID(item.ID) || item.ExpectedRevision < 1 || item.SortOrder != index {
			return nil, invalid(fmt.Sprintf("items[%d]", index), "must contain a unique channel, current revision, and contiguous position")
		}
		if _, exists := seen[item.ID]; exists {
			return nil, invalid(fmt.Sprintf("items[%d].id", index), "must be unique")
		}
		seen[item.ID] = struct{}{}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_channels`).Scan(&count); err != nil {
		return nil, err
	}
	if count != len(order) {
		return nil, invalid("items", "must contain every configured Library Channel")
	}
	now, err := dbNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, item := range order {
		result, err := tx.ExecContext(ctx, `UPDATE library_channels SET sort_order=?,config_revision=config_revision+1,updated_at=? WHERE id=? AND config_revision=?`, item.SortOrder, epoch(now), item.ID, item.ExpectedRevision)
		if err != nil {
			return nil, err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return nil, ErrRevisionConflict
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListChannels(ctx, true)
}

// AcquireGeneration uses the database clock and returns a high-entropy opaque
// lease token. Only its digest is persisted.
func (s *Store) AcquireGeneration(ctx context.Context, generation Generation, leaseDuration time.Duration) (GenerationLease, error) {
	if err := s.configured(); err != nil {
		return GenerationLease{}, err
	}
	if err := validateGenerationDefinition(generation); err != nil {
		return GenerationLease{}, err
	}
	seconds, err := leaseSeconds(leaseDuration)
	if err != nil {
		return GenerationLease{}, err
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return GenerationLease{}, fmt.Errorf("create generation lease token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	digest := tokenDigest(token)
	cursors, _ := json.Marshal(generation.Cursors)
	warningValues := generation.Warnings
	if warningValues == nil {
		warningValues = []string{}
	}
	warnings, _ := json.Marshal(warningValues)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationLease{}, err
	}
	defer tx.Rollback()
	now, err := dbNow(ctx, tx)
	if err != nil {
		return GenerationLease{}, err
	}
	expiry := now.Add(time.Duration(seconds) * time.Second)
	if err := recoverExpiredForChannel(ctx, tx, generation.ChannelID, now); err != nil {
		return GenerationLease{}, err
	}
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT config_revision FROM library_channels WHERE id=?`, generation.ChannelID).Scan(&revision); errors.Is(err, sql.ErrNoRows) {
		return GenerationLease{}, ErrNotFound
	} else if err != nil {
		return GenerationLease{}, err
	}
	if revision != generation.ConfigRevision {
		return GenerationLease{}, ErrGenerationStale
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO library_channel_schedule_generations
		(id,channel_id,config_revision,status,horizon_start,horizon_end,deterministic_seed,candidate_hash,initial_cursor_hash,cursor_json,warnings_json,error_message,lease_token_hash,lease_expires_at,created_at,completed_at)
		VALUES(?,?,?,'building',?,?,?,?,?,?,?,'',?,?,?,NULL)`, generation.ID, generation.ChannelID, generation.ConfigRevision,
		epoch(generation.HorizonStart), epoch(generation.HorizonEnd), generation.DeterministicSeed, generation.CandidateHash, generation.InitialCursorHash, string(cursors), string(warnings), digest, epoch(expiry), epoch(now))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return GenerationLease{}, ErrGenerationInProgress
		}
		return GenerationLease{}, fmt.Errorf("acquire generation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return GenerationLease{}, err
	}
	return GenerationLease{GenerationID: generation.ID, Token: token, ExpiresAt: expiry}, nil
}

func (s *Store) RenewGenerationLease(ctx context.Context, generationID, token string, leaseDuration time.Duration) (GenerationLease, error) {
	if err := s.configured(); err != nil {
		return GenerationLease{}, err
	}
	seconds, err := leaseSeconds(leaseDuration)
	if err != nil {
		return GenerationLease{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationLease{}, err
	}
	defer tx.Rollback()
	now, err := dbNow(ctx, tx)
	if err != nil {
		return GenerationLease{}, err
	}
	expiry := now.Add(time.Duration(seconds) * time.Second)
	result, err := tx.ExecContext(ctx, `UPDATE library_channel_schedule_generations SET lease_expires_at=? WHERE id=? AND status='building' AND lease_token_hash=? AND lease_expires_at>=?`, epoch(expiry), generationID, tokenDigest(token), epoch(now))
	if err != nil {
		return GenerationLease{}, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return GenerationLease{}, ErrGenerationStale
	}
	if err := tx.Commit(); err != nil {
		return GenerationLease{}, err
	}
	return GenerationLease{GenerationID: generationID, Token: token, ExpiresAt: expiry}, nil
}

// CommitGeneration atomically installs a complete schedule only when its
// immutable definition and unexpired opaque lease match the acquired work.
func (s *Store) CommitGeneration(ctx context.Context, generation Generation, entries []ScheduleEntry, token string) error {
	if err := s.configured(); err != nil {
		return err
	}
	if err := validateGenerationDefinition(generation); err != nil {
		return err
	}
	if err := ValidateScheduleEntries(generation.ChannelID, generation.ID, generation.HorizonStart, generation.HorizonEnd, entries); err != nil {
		return err
	}
	if hashCursors(entries[len(entries)-1].CursorAfter) != hashCursors(generation.Cursors) {
		return invalid("generation.cursors", "must match the final schedule checkpoint")
	}
	mediaCount := 0
	for _, entry := range entries {
		if entry.Kind == EntryMedia {
			mediaCount++
		}
	}
	if mediaCount == 0 {
		if err := s.FailGeneration(ctx, generation.ID, token, MessageNoPlayableSchedule); err != nil {
			return err
		}
		return ErrNoPlayableSchedule
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now, err := dbNow(ctx, tx)
	if err != nil {
		return err
	}
	var pChannel, pStatus, pSeed, pCandidates, pInitial, pCursors, pWarnings, pLease string
	var pRevision, pStart, pEnd, pExpiry int64
	err = tx.QueryRowContext(ctx, `SELECT channel_id,config_revision,status,horizon_start,horizon_end,deterministic_seed,candidate_hash,initial_cursor_hash,cursor_json,warnings_json,lease_token_hash,lease_expires_at FROM library_channel_schedule_generations WHERE id=?`, generation.ID).
		Scan(&pChannel, &pRevision, &pStatus, &pStart, &pEnd, &pSeed, &pCandidates, &pInitial, &pCursors, &pWarnings, &pLease, &pExpiry)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	expectedCursors, _ := json.Marshal(generation.Cursors)
	warningValues := generation.Warnings
	if warningValues == nil {
		warningValues = []string{}
	}
	expectedWarnings, _ := json.Marshal(warningValues)
	if pStatus != string(GenerationBuilding) || pChannel != generation.ChannelID || pRevision != generation.ConfigRevision || pStart != epoch(generation.HorizonStart) || pEnd != epoch(generation.HorizonEnd) || pSeed != generation.DeterministicSeed || pCandidates != generation.CandidateHash || pInitial != generation.InitialCursorHash || pCursors != string(expectedCursors) || pWarnings != string(expectedWarnings) || pExpiry < epoch(now) || !equalDigest(pLease, tokenDigest(token)) {
		return ErrGenerationStale
	}
	var currentRevision int64
	var activeID sql.NullString
	var enabled int
	if err := tx.QueryRowContext(ctx, `SELECT config_revision,active_generation_id,enabled FROM library_channels WHERE id=?`, generation.ChannelID).Scan(&currentRevision, &activeID, &enabled); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if currentRevision != generation.ConfigRevision {
		return ErrGenerationStale
	}
	if activeID.Valid && activeID.String != "" {
		if err := verifyStablePastAndCurrent(ctx, tx, activeID.String, now, entries); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if err := insertScheduleEntry(ctx, tx, entry); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_channel_schedule_generations SET status='superseded',completed_at=? WHERE channel_id=? AND status='active' AND id<>?`, epoch(now), generation.ChannelID, generation.ID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE library_channel_schedule_generations SET status='active',cursor_json=?,warnings_json=?,error_message='',lease_token_hash='',lease_expires_at=NULL,completed_at=unixepoch() WHERE id=? AND status='building' AND lease_token_hash=? AND lease_expires_at>=unixepoch()`, string(expectedCursors), string(expectedWarnings), generation.ID, tokenDigest(token))
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrGenerationStale
	}
	health, message := "healthy", ""
	if enabled == 0 {
		health = "disabled"
	} else if len(generation.Warnings) > 0 || hasDegradedEntries(entries) {
		health, message = "warning", MessageScheduleWarnings
	}
	result, err = tx.ExecContext(ctx, `UPDATE library_channels SET active_generation_id=?,generated_through=?,health_state=?,health_message=?,updated_at=? WHERE id=? AND config_revision=?`, generation.ID, epoch(generation.HorizonEnd), health, message, epoch(now), generation.ChannelID, generation.ConfigRevision)
	if err != nil {
		return err
	}
	changed, _ = result.RowsAffected()
	if changed != 1 {
		return ErrGenerationStale
	}
	// Clear only work satisfied by this exact-or-older configuration. If an
	// administrator saved a newer revision while this worker was building, its
	// queued request remains intact.
	if _, err := tx.ExecContext(ctx, `DELETE FROM library_channel_generation_queue WHERE channel_id=? AND requested_revision<=?`, generation.ChannelID, generation.ConfigRevision); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FailGeneration(ctx context.Context, generationID, token, errorCode string) error {
	if err := s.configured(); err != nil {
		return err
	}
	if errorCode != MessageNoPlayableSchedule && errorCode != MessageGenerationFailed {
		return invalid("errorCode", "must be a known Library Channel generation failure key")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now, err := dbNow(ctx, tx)
	if err != nil {
		return err
	}
	var channelID string
	var revision int64
	var leaseHash string
	var expiry int64
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT channel_id,config_revision,lease_token_hash,lease_expires_at,status FROM library_channel_schedule_generations WHERE id=?`, generationID).Scan(&channelID, &revision, &leaseHash, &expiry, &status); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if status != string(GenerationBuilding) || expiry < epoch(now) || !equalDigest(leaseHash, tokenDigest(token)) {
		return ErrGenerationStale
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_channel_schedule_generations SET status='failed',error_message=?,lease_token_hash='',lease_expires_at=NULL,completed_at=? WHERE id=?`, errorCode, epoch(now), generationID); err != nil {
		return err
	}
	if err := updateFailureHealth(ctx, tx, channelID, revision, errorCode, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecoverExpiredGenerations(ctx context.Context) (int64, error) {
	if err := s.configured(); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now, err := dbNow(ctx, tx)
	if err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,channel_id,config_revision FROM library_channel_schedule_generations WHERE status='building' AND lease_expires_at<?`, epoch(now))
	if err != nil {
		return 0, err
	}
	type expired struct {
		id, channel string
		revision    int64
	}
	list := []expired{}
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.id, &item.channel, &item.revision); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	recovered := int64(0)
	for _, item := range list {
		result, err := tx.ExecContext(ctx, `UPDATE library_channel_schedule_generations SET status='failed',error_message=?,lease_token_hash='',lease_expires_at=NULL,completed_at=? WHERE id=? AND status='building' AND lease_expires_at<?`, MessageGenerationLeaseExpired, epoch(now), item.id, epoch(now))
		if err != nil {
			return 0, err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			continue
		}
		recovered++
		if err := updateFailureHealth(ctx, tx, item.channel, item.revision, MessageGenerationLeaseExpired, now); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return recovered, nil
}

// LoadActiveSchedule is a server-internal scheduling view. API handlers must
// use LoadActiveScheduleForProfile so profile policy is applied before output.
func (s *Store) LoadActiveSchedule(ctx context.Context, channelID string, from, to time.Time) ([]ScheduleEntry, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	return s.loadActiveSchedule(ctx, channelID, from, to)
}
func (s *Store) LoadActiveScheduleForProfile(ctx context.Context, channelID string, from, to time.Time, decide func(string) AccessDecision) ([]ScheduleEntry, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	if decide == nil {
		return nil, invalid("profileAccess", "decision function is required")
	}
	entries, err := s.loadActiveSchedule(ctx, channelID, from, to)
	if err != nil {
		return nil, err
	}
	return ProjectSchedule(entries, decide), nil
}

func (s *Store) loadActiveSchedule(ctx context.Context, channelID string, from, to time.Time) ([]ScheduleEntry, error) {
	if !to.After(from) {
		return nil, invalid("scheduleRange", "end must follow start")
	}
	rows, err := s.db.QueryContext(ctx, scheduleEntrySelect+` JOIN library_channels c ON c.id=e.channel_id AND c.active_generation_id=e.generation_id WHERE c.enabled=1 AND e.channel_id=? AND e.ends_at>? AND e.starts_at<? ORDER BY e.starts_at,e.id`, channelID, epoch(from), epoch(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ScheduleEntry{}
	for rows.Next() {
		entry, err := scanScheduleEntry(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

type ExtensionBase struct {
	HorizonStart, TailStart time.Time
	Entries                 []ScheduleEntry
	Cursors                 map[string]RuleCursor
}

// LoadExtensionBase returns a complete active prefix and the exact cursor
// checkpoint at an entry boundary. Callers cannot cut through current media.
func (s *Store) LoadExtensionBase(ctx context.Context, channelID string, horizonStart, tailStart time.Time) (ExtensionBase, error) {
	generation, err := s.GetActiveGeneration(ctx, channelID)
	if err != nil {
		return ExtensionBase{}, err
	}
	if horizonStart.Before(generation.HorizonStart) || !tailStart.After(horizonStart) || tailStart.After(generation.HorizonEnd) {
		return ExtensionBase{}, invalid("extensionRange", "must fall inside the active schedule")
	}
	entries, err := s.loadActiveSchedule(ctx, channelID, horizonStart, tailStart)
	if err != nil {
		return ExtensionBase{}, err
	}
	if len(entries) == 0 || !entries[0].StartsAt.Equal(horizonStart) || !entries[len(entries)-1].EndsAt.Equal(tailStart) {
		return ExtensionBase{}, invalid("tailStart", "must be an active schedule entry boundary")
	}
	return ExtensionBase{HorizonStart: horizonStart, TailStart: tailStart, Entries: entries, Cursors: cloneCursors(entries[len(entries)-1].CursorAfter)}, nil
}

func (s *Store) GetActiveGeneration(ctx context.Context, channelID string) (Generation, error) {
	if err := s.configured(); err != nil {
		return Generation{}, err
	}
	generation, err := scanGeneration(s.db.QueryRowContext(ctx, generationSelect+` JOIN library_channels c ON c.active_generation_id=g.id AND g.channel_id=c.id WHERE c.id=? AND c.enabled=1 AND g.status='active'`, channelID))
	if errors.Is(err, sql.ErrNoRows) {
		return Generation{}, ErrNotFound
	}
	return generation, err
}

func (s *Store) PruneCompletedGenerations(ctx context.Context, completedBefore time.Time) (int64, error) {
	if err := s.configured(); err != nil {
		return 0, err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM library_channel_schedule_generations WHERE status IN ('superseded','failed') AND completed_at IS NOT NULL AND completed_at<?`, epoch(completedBefore))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

type scanner interface{ Scan(...any) error }

const channelSelect = `SELECT id,name,description,enabled,sort_order,timezone,seed,COALESCE(default_rule_id,''),quality_profile,logo_source,logo_ref,logo_mime_type,bug_enabled,bug_overhead_accepted,bug_corner,bug_width_percent,bug_inset_percent,bug_treatment,template_key,template_version,config_revision,COALESCE(active_generation_id,''),generated_through,health_state,health_message,created_at,updated_at FROM library_channels`

func scanChannel(row scanner) (Channel, error) {
	var c Channel
	var enabled, bug, bugAccepted int
	var generated sql.NullInt64
	var created, updated int64
	err := row.Scan(&c.ID, &c.Name, &c.Description, &enabled, &c.SortOrder, &c.Timezone, &c.Seed, &c.DefaultRuleID, &c.QualityProfile, &c.Logo.Source, &c.Logo.Ref, &c.Logo.MIMEType, &bug, &bugAccepted, &c.Logo.BugCorner, &c.Logo.BugWidthPct, &c.Logo.BugInsetPct, &c.Logo.BugTreatment, &c.TemplateKey, &c.TemplateVersion, &c.ConfigRevision, &c.ActiveGenerationID, &generated, &c.HealthState, &c.HealthMessage, &created, &updated)
	if err != nil {
		return c, err
	}
	c.Enabled = enabled != 0
	c.Logo.BugEnabled = bug != 0
	c.Logo.BugOverheadAccepted = bugAccepted != 0
	c.CreatedAt = fromEpoch(created)
	c.UpdatedAt = fromEpoch(updated)
	if generated.Valid {
		c.GeneratedThrough = fromEpoch(generated.Int64)
	}
	return c, nil
}

func (s *Store) loadRules(ctx context.Context, channelID string) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,channel_id,name,enabled,sort_order,query_json,selection_mode,episode_mode,exhaustion_mode,dedupe_window,max_consecutive,config_json,template_key,template_version,created_at,updated_at FROM library_channel_rules WHERE channel_id=? ORDER BY sort_order,id`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Rule{}
	for rows.Next() {
		var r Rule
		var enabled int
		var query, config string
		var created, updated int64
		if err := rows.Scan(&r.ID, &r.ChannelID, &r.Name, &enabled, &r.SortOrder, &query, &r.SelectionMode, &r.EpisodeMode, &r.ExhaustionMode, &r.DedupeWindow, &r.MaxConsecutive, &config, &r.TemplateKey, &r.TemplateVersion, &created, &updated); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		r.Query = json.RawMessage(query)
		r.Config = json.RawMessage(config)
		r.CreatedAt = fromEpoch(created)
		r.UpdatedAt = fromEpoch(updated)
		result = append(result, r)
	}
	return result, rows.Err()
}
func (s *Store) loadBlocks(ctx context.Context, channelID string) ([]WeeklyBlock, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,channel_id,rule_id,COALESCE(fallback_rule_id,''),name,enabled,weekday_mask,start_minute,end_minute,priority,anchored,allow_overrun,sort_order,template_key,template_version,created_at,updated_at FROM library_channel_blocks WHERE channel_id=? ORDER BY sort_order,id`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []WeeklyBlock{}
	for rows.Next() {
		var b WeeklyBlock
		var enabled, days, anchored, overrun int
		var created, updated int64
		if err := rows.Scan(&b.ID, &b.ChannelID, &b.RuleID, &b.FallbackRuleID, &b.Name, &enabled, &days, &b.StartMinute, &b.EndMinute, &b.Priority, &anchored, &overrun, &b.SortOrder, &b.TemplateKey, &b.TemplateVersion, &created, &updated); err != nil {
			return nil, err
		}
		b.Enabled = enabled != 0
		b.Weekdays = WeekdayMask(days)
		b.Anchored = anchored != 0
		b.AllowOverrun = overrun != 0
		b.CreatedAt = fromEpoch(created)
		b.UpdatedAt = fromEpoch(updated)
		result = append(result, b)
	}
	return result, rows.Err()
}

func insertChannel(ctx context.Context, tx *sql.Tx, c Channel) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO library_channels(id,name,description,enabled,sort_order,timezone,seed,default_rule_id,quality_profile,logo_source,logo_ref,logo_mime_type,bug_enabled,bug_overhead_accepted,bug_corner,bug_width_percent,bug_inset_percent,bug_treatment,template_key,template_version,config_revision,active_generation_id,generated_through,health_state,health_message,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, c.ID, c.Name, c.Description, boolInt(c.Enabled), c.SortOrder, c.Timezone, c.Seed, nullString(c.DefaultRuleID), c.QualityProfile, c.Logo.Source, c.Logo.Ref, c.Logo.MIMEType, boolInt(c.Logo.BugEnabled), boolInt(c.Logo.BugOverheadAccepted), c.Logo.BugCorner, c.Logo.BugWidthPct, c.Logo.BugInsetPct, c.Logo.BugTreatment, c.TemplateKey, c.TemplateVersion, c.ConfigRevision, nullString(c.ActiveGenerationID), nullTime(c.GeneratedThrough), c.HealthState, c.HealthMessage, epoch(c.CreatedAt), epoch(c.UpdatedAt))
	if err != nil {
		if isTemplateUniquenessError(err) {
			return ErrTemplateExists
		}
		return fmt.Errorf("create library channel: %w", err)
	}
	return nil
}

// GenerationRequest is durable, coalesced background work. RequestedRevision
// identifies the newest configuration that must have an active schedule.
type GenerationRequest struct {
	ChannelID         string
	RequestedRevision int64
	Attempts          int
}

func (s *Store) EnqueueExpiringChannels(ctx context.Context, renewBefore time.Time) (int64, error) {
	if err := s.configured(); err != nil {
		return 0, err
	}
	if renewBefore.IsZero() {
		return 0, invalid("renewBefore", "is required")
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO library_channel_generation_queue(channel_id,requested_revision,requested_at,not_before,attempts,last_error)
		SELECT id,config_revision,unixepoch(),unixepoch(),0,''
		FROM library_channels
		WHERE enabled=1 AND (generated_through IS NULL OR generated_through<=?)
		ON CONFLICT(channel_id) DO UPDATE SET
			requested_revision=excluded.requested_revision,
			requested_at=excluded.requested_at,
			not_before=excluded.not_before,
			attempts=0,
			last_error=''
		WHERE excluded.requested_revision>library_channel_generation_queue.requested_revision
	`, epoch(renewBefore))
	if err != nil {
		return 0, fmt.Errorf("enqueue expiring Library Channels: %w", err)
	}
	return result.RowsAffected()
}

func (s *Store) ListDueGenerationRequests(ctx context.Context, limit int) ([]GenerationRequest, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 64 {
		return nil, invalid("limit", "must be between 1 and 64")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT q.channel_id,q.requested_revision,q.attempts
		FROM library_channel_generation_queue q
		JOIN library_channels c ON c.id=q.channel_id
		WHERE c.enabled=1 AND q.not_before<=unixepoch()
		ORDER BY q.not_before,q.requested_at,q.channel_id
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list due Library Channel generations: %w", err)
	}
	defer rows.Close()
	requests := make([]GenerationRequest, 0, limit)
	for rows.Next() {
		var request GenerationRequest
		if err := rows.Scan(&request.ChannelID, &request.RequestedRevision, &request.Attempts); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

func (s *Store) DeferGenerationRequest(ctx context.Context, channelID string, revision int64, retryAfter time.Duration, errorCode string) error {
	if err := s.configured(); err != nil {
		return err
	}
	if !validOpaqueID(channelID) || revision < 1 || retryAfter < time.Second || retryAfter > 24*time.Hour {
		return invalid("generationRequest", "channel, revision, and retry delay must be valid")
	}
	if !isKnownLibraryChannelMessage(errorCode) {
		return invalid("generationRequest.errorCode", "must be a Product Language message identifier")
	}
	if len(errorCode) > 2000 {
		errorCode = errorCode[:2000]
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now, err := dbNow(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_channel_generation_queue
		SET attempts=MIN(attempts+1,1000),not_before=?,last_error=?
		WHERE channel_id=? AND requested_revision=?`, epoch(now.Add(retryAfter)), errorCode, channelID, revision); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE library_channels
		SET health_state=CASE WHEN active_generation_id IS NULL THEN 'error' ELSE 'warning' END,
			health_message=?,updated_at=?
		WHERE id=? AND config_revision=? AND enabled=1`, errorCode, epoch(now), channelID, revision); err != nil {
		return err
	}
	return tx.Commit()
}

func enqueueGenerationTx(ctx context.Context, tx *sql.Tx, channelID string, revision int64, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO library_channel_generation_queue(channel_id,requested_revision,requested_at,not_before,attempts,last_error)
		VALUES(?,?,?,?,0,'')
		ON CONFLICT(channel_id) DO UPDATE SET
			requested_revision=excluded.requested_revision,
			requested_at=excluded.requested_at,
			not_before=excluded.not_before,
			attempts=0,
			last_error=''`, channelID, revision, epoch(now), epoch(now))
	if err != nil {
		return fmt.Errorf("enqueue Library Channel generation: %w", err)
	}
	return nil
}

func isTemplateUniquenessError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed: library_channels.template_key") ||
		strings.Contains(message, "idx_library_channels_template")
}
func updateChannel(ctx context.Context, tx *sql.Tx, c Channel, expected int64, now time.Time) (sql.Result, error) {
	health, message := "pending", MessageRegenerationRequired
	if !c.Enabled {
		health, message = "disabled", ""
	}
	result, err := tx.ExecContext(ctx, `UPDATE library_channels SET name=?,description=?,enabled=?,sort_order=?,timezone=?,seed=?,default_rule_id=?,quality_profile=?,logo_source=?,logo_ref=?,logo_mime_type=?,bug_enabled=?,bug_overhead_accepted=?,bug_corner=?,bug_width_percent=?,bug_inset_percent=?,bug_treatment=?,template_key=?,template_version=?,config_revision=?,health_state=?,health_message=?,updated_at=? WHERE id=? AND config_revision=?`, c.Name, c.Description, boolInt(c.Enabled), c.SortOrder, c.Timezone, c.Seed, nullString(c.DefaultRuleID), c.QualityProfile, c.Logo.Source, c.Logo.Ref, c.Logo.MIMEType, boolInt(c.Logo.BugEnabled), boolInt(c.Logo.BugOverheadAccepted), c.Logo.BugCorner, c.Logo.BugWidthPct, c.Logo.BugInsetPct, c.Logo.BugTreatment, c.TemplateKey, c.TemplateVersion, c.ConfigRevision, health, message, epoch(now), c.ID, expected)
	return result, err
}
func insertRule(ctx context.Context, tx *sql.Tx, r Rule) error {
	query, config := string(r.Query), string(r.Config)
	if query == "" {
		query = "{}"
	}
	if config == "" {
		config = "{}"
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO library_channel_rules(id,channel_id,name,enabled,sort_order,query_json,selection_mode,episode_mode,exhaustion_mode,dedupe_window,max_consecutive,config_json,template_key,template_version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.ChannelID, r.Name, boolInt(r.Enabled), r.SortOrder, query, r.SelectionMode, r.EpisodeMode, r.ExhaustionMode, r.DedupeWindow, r.MaxConsecutive, config, r.TemplateKey, r.TemplateVersion, epoch(r.CreatedAt), epoch(r.UpdatedAt))
	return err
}
func insertBlock(ctx context.Context, tx *sql.Tx, b WeeklyBlock) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO library_channel_blocks(id,channel_id,rule_id,fallback_rule_id,name,enabled,weekday_mask,start_minute,end_minute,priority,anchored,allow_overrun,sort_order,template_key,template_version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, b.ID, b.ChannelID, b.RuleID, nullString(b.FallbackRuleID), b.Name, boolInt(b.Enabled), int(b.Weekdays), b.StartMinute, b.EndMinute, b.Priority, boolInt(b.Anchored), boolInt(b.AllowOverrun), b.SortOrder, b.TemplateKey, b.TemplateVersion, epoch(b.CreatedAt), epoch(b.UpdatedAt))
	return err
}

const scheduleEntrySelect = `SELECT e.id,e.generation_id,e.channel_id,e.rule_id,e.block_id,COALESCE(e.media_id,''),e.entry_kind,e.starts_at,e.ends_at,e.media_offset_seconds,e.source_duration_seconds,e.cycle_number,e.selection_index,e.title,e.subtitle,e.summary,e.content_rating,e.artwork_json,e.availability,e.selection_metadata_json,e.reason_code,e.playout_source_json,e.cursor_after_json,e.created_at FROM library_channel_schedule_entries e`

func insertScheduleEntry(ctx context.Context, tx *sql.Tx, e ScheduleEntry) error {
	art := string(e.Artwork)
	if art == "" {
		art = "{}"
	}
	meta := string(e.SelectionMetadata)
	if meta == "" {
		meta = "{}"
	}
	playout, _ := json.Marshal(e.PlayoutSource)
	cursor := []byte(`{}`)
	if e.CursorAfter != nil {
		cursor, _ = json.Marshal(e.CursorAfter)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO library_channel_schedule_entries(id,generation_id,channel_id,rule_id,block_id,media_id,entry_kind,starts_at,ends_at,media_offset_seconds,source_duration_seconds,cycle_number,selection_index,title,subtitle,summary,content_rating,artwork_json,availability,selection_metadata_json,reason_code,playout_source_json,cursor_after_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,unixepoch())`, e.ID, e.GenerationID, e.ChannelID, e.RuleID, e.BlockID, nullString(e.MediaID), e.Kind, epoch(e.StartsAt), epoch(e.EndsAt), e.MediaOffsetSeconds, e.SourceDurationSeconds, e.CycleNumber, e.SelectionIndex, e.Title, e.Subtitle, e.Summary, e.ContentRating, art, e.Availability, meta, e.ReasonCode, string(playout), string(cursor))
	if err != nil {
		return fmt.Errorf("insert schedule entry: %w", err)
	}
	return nil
}
func scanScheduleEntry(row scanner) (ScheduleEntry, error) {
	var e ScheduleEntry
	var start, end, created int64
	var artwork, metadata, playout, cursor string
	err := row.Scan(&e.ID, &e.GenerationID, &e.ChannelID, &e.RuleID, &e.BlockID, &e.MediaID, &e.Kind, &start, &end, &e.MediaOffsetSeconds, &e.SourceDurationSeconds, &e.CycleNumber, &e.SelectionIndex, &e.Title, &e.Subtitle, &e.Summary, &e.ContentRating, &artwork, &e.Availability, &metadata, &e.ReasonCode, &playout, &cursor, &created)
	if err != nil {
		return e, err
	}
	e.StartsAt = fromEpoch(start)
	e.EndsAt = fromEpoch(end)
	e.CreatedAt = fromEpoch(created)
	e.Artwork = json.RawMessage(artwork)
	e.SelectionMetadata = json.RawMessage(metadata)
	if err := json.Unmarshal([]byte(playout), &e.PlayoutSource); err != nil {
		return e, err
	}
	if err := json.Unmarshal([]byte(cursor), &e.CursorAfter); err != nil {
		return e, err
	}
	return e, nil
}

const generationSelect = `SELECT g.id,g.channel_id,g.config_revision,g.status,g.horizon_start,g.horizon_end,g.deterministic_seed,g.candidate_hash,g.initial_cursor_hash,g.cursor_json,g.warnings_json,g.error_message,g.lease_expires_at,g.created_at,g.completed_at FROM library_channel_schedule_generations g`

func scanGeneration(row scanner) (Generation, error) {
	var g Generation
	var start, end, created int64
	var expiry, completed sql.NullInt64
	var cursors, warnings string
	err := row.Scan(&g.ID, &g.ChannelID, &g.ConfigRevision, &g.Status, &start, &end, &g.DeterministicSeed, &g.CandidateHash, &g.InitialCursorHash, &cursors, &warnings, &g.ErrorMessage, &expiry, &created, &completed)
	if err != nil {
		return g, err
	}
	g.HorizonStart = fromEpoch(start)
	g.HorizonEnd = fromEpoch(end)
	g.CreatedAt = fromEpoch(created)
	if expiry.Valid {
		g.LeaseExpiresAt = fromEpoch(expiry.Int64)
	}
	if completed.Valid {
		g.CompletedAt = fromEpoch(completed.Int64)
	}
	if err := json.Unmarshal([]byte(cursors), &g.Cursors); err != nil {
		return g, err
	}
	if err := json.Unmarshal([]byte(warnings), &g.Warnings); err != nil {
		return g, err
	}
	return g, nil
}

func validateGenerationDefinition(g Generation) error {
	if !validOpaqueID(g.ID) || !validOpaqueID(g.ChannelID) {
		return invalid("generation", "id and channel id are required")
	}
	if g.ConfigRevision < 1 || !g.HorizonEnd.After(g.HorizonStart) || g.HorizonStart.Unix() < 0 || g.HorizonStart.Nanosecond() != 0 || g.HorizonEnd.Nanosecond() != 0 || g.HorizonEnd.Sub(g.HorizonStart) > 8*24*time.Hour {
		return invalid("generation", "revision and horizon must be valid")
	}
	if g.Status != "" && g.Status != GenerationBuilding {
		return invalid("generation.status", "must be building")
	}
	if len(g.DeterministicSeed) != 64 || len(g.CandidateHash) != 64 || len(g.InitialCursorHash) != 64 {
		return invalid("generation", "seed, candidate hash, and initial cursor hash must be SHA-256 values")
	}
	if err := ValidateCursorMap(g.Cursors); err != nil {
		return err
	}
	if g.Cursors == nil {
		return invalid("generation.cursors", "must be an object")
	}
	for _, warning := range g.Warnings {
		if warning != MessageNoPlayableCandidates {
			return invalid("generation.warnings", "must contain known Library Channel warning keys")
		}
	}
	return nil
}
func leaseSeconds(duration time.Duration) (int64, error) {
	if duration < time.Second || duration > time.Hour {
		return 0, invalid("leaseDuration", "must be between one second and one hour")
	}
	return int64((duration + time.Second - 1) / time.Second), nil
}
func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func equalDigest(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func dbNow(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	var seconds int64
	if err := tx.QueryRowContext(ctx, `SELECT unixepoch()`).Scan(&seconds); err != nil {
		return time.Time{}, fmt.Errorf("read database clock: %w", err)
	}
	return fromEpoch(seconds), nil
}
func epoch(value time.Time) int64     { return value.UTC().Unix() }
func fromEpoch(value int64) time.Time { return time.Unix(value, 0).UTC() }
func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return epoch(value)
}
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func recoverExpiredForChannel(ctx context.Context, tx *sql.Tx, channelID string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,config_revision FROM library_channel_schedule_generations WHERE channel_id=? AND status='building' AND lease_expires_at<?`, channelID, epoch(now))
	if err != nil {
		return err
	}
	type item struct {
		id       string
		revision int64
	}
	items := []item{}
	for rows.Next() {
		var i item
		if err := rows.Scan(&i.id, &i.revision); err != nil {
			rows.Close()
			return err
		}
		items = append(items, i)
	}
	rows.Close()
	for _, i := range items {
		result, err := tx.ExecContext(ctx, `UPDATE library_channel_schedule_generations SET status='failed',error_message=?,lease_token_hash='',lease_expires_at=NULL,completed_at=? WHERE id=? AND status='building' AND lease_expires_at<?`, MessageGenerationLeaseExpired, epoch(now), i.id, epoch(now))
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			continue
		}
		if err := updateFailureHealth(ctx, tx, channelID, i.revision, MessageGenerationLeaseExpired, now); err != nil {
			return err
		}
	}
	return nil
}
func updateFailureHealth(ctx context.Context, tx *sql.Tx, channelID string, revision int64, code string, now time.Time) error {
	var active sql.NullString
	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT active_generation_id,config_revision FROM library_channels WHERE id=?`, channelID).Scan(&active, &current); err != nil {
		return err
	}
	if current != revision {
		return nil
	}
	state := "error"
	if active.Valid && active.String != "" {
		state = "warning"
	}
	_, err := tx.ExecContext(ctx, `UPDATE library_channels SET health_state=?,health_message=?,updated_at=? WHERE id=? AND config_revision=?`, state, code, epoch(now), channelID, revision)
	return err
}
func hasDegradedEntries(entries []ScheduleEntry) bool {
	for _, e := range entries {
		if e.Kind != EntryMedia {
			return true
		}
	}
	return false
}
func verifyStablePastAndCurrent(ctx context.Context, tx *sql.Tx, activeID string, now time.Time, candidate []ScheduleEntry) error {
	if len(candidate) == 0 {
		return invalid("schedule.entries", "must preserve the active schedule")
	}
	if candidate[0].StartsAt.After(now) || !candidate[len(candidate)-1].EndsAt.After(now) {
		return invalid("schedule.entries", "replacement schedule must include the current instant")
	}
	type identity struct {
		start, end int64
		kind       EntryKind
		media      string
	}
	rows, err := tx.QueryContext(ctx, `SELECT starts_at,ends_at,entry_kind,COALESCE(media_id,'') FROM library_channel_schedule_entries WHERE generation_id=? AND ends_at>? AND starts_at<=? ORDER BY starts_at,id`, activeID, epoch(candidate[0].StartsAt), epoch(now))
	if err != nil {
		return err
	}
	defer rows.Close()
	expected := []identity{}
	for rows.Next() {
		var item identity
		if err := rows.Scan(&item.start, &item.end, &item.kind, &item.media); err != nil {
			return err
		}
		expected = append(expected, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	actual := make(map[int64]identity, len(candidate))
	for _, entry := range candidate {
		actual[epoch(entry.StartsAt)] = identity{start: epoch(entry.StartsAt), end: epoch(entry.EndsAt), kind: entry.Kind, media: entry.MediaID}
	}
	for _, item := range expected {
		if replacement, ok := actual[item.start]; !ok || replacement != item {
			return invalid("schedule.entries", "must preserve every past program and the program currently in progress")
		}
	}
	return nil
}
