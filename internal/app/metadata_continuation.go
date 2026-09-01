package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MetadataContinuationStore persists a metadata cascade independently of the
// worker executing it. Callers may discard and recreate the store between any
// two calls without losing traversal position or item retry state.
type MetadataContinuationStore struct {
	db      *sql.DB
	now     func() time.Time
	writeTx func(context.Context, func(*sql.Tx) error) error
}

type MetadataContinuationStart struct {
	ID, RootKind, RootID, Provider, PolicyRevision, ProviderRevision string
	InitialPhase, InitialCursor                                      string
}

type MetadataContinuationOperation struct {
	ID, RootKind, RootID, Provider, PolicyRevision, ProviderRevision string
	Phase, Cursor, Status, LastError                                 string
	Processed, Remaining, Failed, RetryCount                         int
	NextRetryAt, CreatedAt, UpdatedAt, CompletedAt                   time.Time
}

type MetadataContinuationItemInput struct {
	Key, ParentKey, Kind, ProviderID, PayloadJSON string
}

type MetadataContinuationItem struct {
	OperationID, Key, ParentKey, Kind, ProviderID, PayloadJSON string
	State, LastError                                           string
	Attempts                                                   int
	NextRetryAt, LeaseUntil, CreatedAt, UpdatedAt              time.Time
}

type MetadataContinuationFailure struct {
	Key, ParentKey, Kind, ProviderID, Error string
	Attempts                                int
	UpdatedAt                               time.Time
}

var ErrMetadataContinuationNotFound = errors.New("metadata continuation operation not found")

func NewMetadataContinuationStore(db *sql.DB) *MetadataContinuationStore {
	return &MetadataContinuationStore{db: db, now: time.Now}
}

// newPrioritizedMetadataContinuationStore routes every continuation mutation
// through the Server's one-writer scheduler. The public constructor remains a
// small standalone store for migrations and focused tests; runtime background
// work must never compete with foreground playback/navigation through a raw
// SQLite transaction.
func (s *Server) newPrioritizedMetadataContinuationStore() *MetadataContinuationStore {
	store := NewMetadataContinuationStore(s.dbHandle())
	store.writeTx = func(ctx context.Context, fn func(*sql.Tx) error) error {
		return s.withBackgroundTxTagged(ctx, nil, fn)
	}
	return store
}

func (s *MetadataContinuationStore) withWriteTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s.writeTx != nil {
		return s.writeTx(ctx, fn)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Start returns the existing durable operation when ID is replayed. Immutable
// identity/revision fields are checked so an idempotency key cannot be reused
// for a different cascade.
func (s *MetadataContinuationStore) Start(ctx context.Context, in MetadataContinuationStart) (MetadataContinuationOperation, error) {
	if in.ID == "" || in.RootKind == "" || in.RootID == "" || in.Provider == "" || in.PolicyRevision == "" || in.ProviderRevision == "" || in.InitialPhase == "" {
		return MetadataContinuationOperation{}, errors.New("metadata continuation start fields must not be empty")
	}
	now := formatMetadataContinuationTime(s.now())
	var op MetadataContinuationOperation
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `INSERT INTO metadata_continuation_operations
		(id,root_kind,root_id,provider,policy_revision,provider_revision,traversal_phase,traversal_cursor,status,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?, 'running',?,?) ON CONFLICT(id) DO NOTHING`,
			in.ID, in.RootKind, in.RootID, in.Provider, in.PolicyRevision, in.ProviderRevision, in.InitialPhase, in.InitialCursor, now, now)
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if inserted == 1 {
			if _, err = tx.ExecContext(ctx, `INSERT INTO metadata_continuation_cursors(operation_id,phase,parent_key,cursor,exhausted,page_count,updated_at) VALUES(?,?, '',?,0,0,?)`, in.ID, in.InitialPhase, in.InitialCursor, now); err != nil {
				return err
			}
		}
		row := tx.QueryRowContext(ctx, `SELECT id,root_kind,root_id,provider,policy_revision,provider_revision,traversal_phase,traversal_cursor,status,processed_count,remaining_count,failed_count,retry_count,next_retry_at,last_error,created_at,updated_at,completed_at FROM metadata_continuation_operations WHERE id=?`, in.ID)
		op, err = scanMetadataContinuationOperation(row)
		if err != nil {
			return err
		}
		if op.RootKind != in.RootKind || op.RootID != in.RootID || op.Provider != in.Provider || op.PolicyRevision != in.PolicyRevision || op.ProviderRevision != in.ProviderRevision {
			return fmt.Errorf("metadata continuation id %q already has different identity or revisions", in.ID)
		}
		return nil
	})
	return op, err
}

func (s *MetadataContinuationStore) Get(ctx context.Context, id string) (MetadataContinuationOperation, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,root_kind,root_id,provider,policy_revision,provider_revision,
		traversal_phase,traversal_cursor,status,processed_count,remaining_count,failed_count,retry_count,
		next_retry_at,last_error,created_at,updated_at,completed_at FROM metadata_continuation_operations WHERE id=?`, id)
	return scanMetadataContinuationOperation(row)
}

// RecordPage atomically records a provider page, its next cursor and all
// discovered descendants. Replaying the same (phase,parent,cursor) is a no-op.
// New parent cursors are created as non-exhausted, enabling arbitrary-depth
// show/anime and music cascades without an in-memory or fixed-size frontier.
func (s *MetadataContinuationStore) RecordPage(ctx context.Context, operationID, phase, parentKey, cursor, nextCursor string, exhausted bool, items []MetadataContinuationItemInput) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		now := formatMetadataContinuationTime(s.now())
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM metadata_continuation_operations WHERE id=?`, operationID).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrMetadataContinuationNotFound
			}
			return err
		}
		if status == "cancelled" || status == "completed" || status == "completed_with_failures" || status == "failed" {
			return fmt.Errorf("metadata continuation operation is terminal: %s", status)
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO metadata_continuation_pages(operation_id,phase,parent_key,cursor,next_cursor,exhausted,created_at)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT(operation_id,phase,parent_key,cursor) DO NOTHING`, operationID, phase, parentKey, cursor, nextCursor, metadataContinuationBoolInt(exhausted), now)
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil || inserted == 0 {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO metadata_continuation_cursors(operation_id,phase,parent_key,cursor,exhausted,page_count,updated_at)
		VALUES(?,?,?,?,?,1,?) ON CONFLICT(operation_id,phase,parent_key) DO UPDATE SET cursor=excluded.cursor,exhausted=excluded.exhausted,page_count=metadata_continuation_cursors.page_count+1,updated_at=excluded.updated_at`,
			operationID, phase, parentKey, nextCursor, metadataContinuationBoolInt(exhausted), now); err != nil {
			return err
		}
		added := int64(0)
		for _, item := range items {
			if item.Key == "" || item.Kind == "" {
				return errors.New("metadata continuation item key and kind must not be empty")
			}
			res, insertErr := tx.ExecContext(ctx, `INSERT INTO metadata_continuation_items(operation_id,item_key,parent_key,item_kind,provider_id,payload_json,state,created_at,updated_at)
			VALUES(?,?,?,?,?,?,'pending',?,?) ON CONFLICT(operation_id,item_key) DO NOTHING`, operationID, item.Key, item.ParentKey, item.Kind, item.ProviderID, defaultJSON(item.PayloadJSON), now, now)
			if insertErr != nil {
				return insertErr
			}
			n, insertErr := res.RowsAffected()
			if insertErr != nil {
				return insertErr
			}
			added += n
			if phase == "descendants" && metadataContinuationKindCanHaveChildren(item.Kind) {
				if _, insertErr = tx.ExecContext(ctx, `
				INSERT INTO metadata_continuation_cursors (
					operation_id, phase, parent_key, cursor, exhausted, page_count, updated_at
				) VALUES (?, ?, ?, '', 0, 0, ?)
				ON CONFLICT(operation_id, phase, parent_key) DO NOTHING`,
					operationID, phase, item.Key, now); insertErr != nil {
					return insertErr
				}
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE metadata_continuation_operations SET traversal_phase=?,traversal_cursor=?,remaining_count=remaining_count+?,status='running',next_retry_at='',last_error='',updated_at=? WHERE id=?`, phase, nextCursor, added, now, operationID)
		return err
	})
}

// ClaimReadyItems leases pending or due retry items. Expired leases are
// reclaimable after process death. A transaction makes concurrent claims safe.
func (s *MetadataContinuationStore) ClaimReadyItems(ctx context.Context, operationID string, limit int, lease time.Duration) ([]MetadataContinuationItem, error) {
	if limit <= 0 {
		return nil, errors.New("claim limit must be positive")
	}
	if lease <= 0 {
		return nil, errors.New("claim lease must be positive")
	}
	var items []MetadataContinuationItem
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		nowTime := s.now().UTC()
		now, until := formatMetadataContinuationTime(nowTime), formatMetadataContinuationTime(nowTime.Add(lease))
		rows, err := tx.QueryContext(ctx, `SELECT item_key FROM metadata_continuation_items WHERE operation_id=? AND
		((state='pending') OR (state='retry_wait' AND next_retry_at<=?) OR (state='processing' AND lease_until<=?))
		ORDER BY created_at,item_key LIMIT ?`, operationID, now, now, limit)
		if err != nil {
			return err
		}
		var keys []string
		for rows.Next() {
			var key string
			if err = rows.Scan(&key); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, key)
		}
		if err = rows.Close(); err != nil {
			return err
		}
		for _, key := range keys {
			if _, err = tx.ExecContext(ctx, `UPDATE metadata_continuation_items SET state='processing',attempts=attempts+1,lease_until=?,updated_at=? WHERE operation_id=? AND item_key=?`, until, now, operationID, key); err != nil {
				return err
			}
		}
		items = make([]MetadataContinuationItem, 0, len(keys))
		for _, key := range keys {
			row := tx.QueryRowContext(ctx, `SELECT operation_id,item_key,parent_key,item_kind,provider_id,payload_json,state,attempts,next_retry_at,lease_until,last_error,created_at,updated_at FROM metadata_continuation_items WHERE operation_id=? AND item_key=?`, operationID, key)
			item, scanErr := scanMetadataContinuationItem(row)
			if scanErr != nil {
				return scanErr
			}
			items = append(items, item)
		}
		return nil
	})
	return items, err
}

func (s *MetadataContinuationStore) SucceedItem(ctx context.Context, operationID, key string) error {
	return s.finishItem(ctx, operationID, key, "succeeded", "", time.Time{})
}

func (s *MetadataContinuationStore) FailItem(ctx context.Context, operationID, key, message string) error {
	if message == "" {
		message = "metadata item failed"
	}
	return s.finishItem(ctx, operationID, key, "failed", message, time.Time{})
}

func (s *MetadataContinuationStore) RetryItem(ctx context.Context, operationID, key, message string, next time.Time) error {
	if next.IsZero() {
		return errors.New("retry time must not be zero")
	}
	return s.finishItem(ctx, operationID, key, "retry_wait", message, next)
}

func (s *MetadataContinuationStore) finishItem(ctx context.Context, operationID, key, state, message string, next time.Time) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		now := formatMetadataContinuationTime(s.now())
		var prior string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM metadata_continuation_items WHERE operation_id=? AND item_key=?`, operationID, key).Scan(&prior); err != nil {
			return err
		}
		if prior == "succeeded" || prior == "failed" { // terminal result replay
			if prior == state {
				return nil
			}
			return fmt.Errorf("metadata continuation item is already terminal: %s", prior)
		}
		nextText := ""
		if !next.IsZero() {
			nextText = formatMetadataContinuationTime(next)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE metadata_continuation_items SET state=?,next_retry_at=?,lease_until='',last_error=?,updated_at=? WHERE operation_id=? AND item_key=?`, state, nextText, message, now, operationID, key); err != nil {
			return err
		}
		processedDelta, failedDelta, remainingDelta := 0, 0, 0
		if state == "succeeded" {
			processedDelta, remainingDelta = 1, -1
		}
		if state == "failed" {
			failedDelta, remainingDelta = 1, -1
		}
		_, err := tx.ExecContext(ctx, `UPDATE metadata_continuation_operations SET processed_count=processed_count+?,failed_count=failed_count+?,remaining_count=remaining_count+?,updated_at=? WHERE id=?`, processedDelta, failedDelta, remainingDelta, now, operationID)
		return err
	})
}

func (s *MetadataContinuationStore) ScheduleRetry(ctx context.Context, operationID, phase, cursor, message string, next time.Time) error {
	if next.IsZero() {
		return errors.New("retry time must not be zero")
	}
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE metadata_continuation_operations SET status='retry_wait',traversal_phase=?,traversal_cursor=?,retry_count=retry_count+1,next_retry_at=?,last_error=?,updated_at=? WHERE id=? AND status NOT IN ('completed','completed_with_failures','failed','cancelled')`, phase, cursor, formatMetadataContinuationTime(next), message, formatMetadataContinuationTime(s.now()), operationID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrMetadataContinuationNotFound
		}
		return nil
	})
}

func (s *MetadataContinuationStore) ResumeDue(ctx context.Context, operationID string) (bool, error) {
	now := formatMetadataContinuationTime(s.now())
	resumed := false
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE metadata_continuation_operations SET status='running',next_retry_at='',updated_at=? WHERE id=? AND status='retry_wait' AND next_retry_at<=?`, now, operationID, now)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		resumed = n > 0
		return err
	})
	return resumed, err
}

func (s *MetadataContinuationStore) Cancel(ctx context.Context, operationID string) error {
	now := formatMetadataContinuationTime(s.now())
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE metadata_continuation_operations SET status='cancelled',completed_at=?,updated_at=? WHERE id=? AND status NOT IN ('completed','completed_with_failures','failed','cancelled')`, now, now, operationID)
		return err
	})
}

// TryComplete is deliberately conservative: missing cursors, unconsumed
// cursor pages, pending retries, and active/expired leases all prevent success.
func (s *MetadataContinuationStore) TryComplete(ctx context.Context, operationID string) (bool, error) {
	complete := false
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM metadata_continuation_operations WHERE id=?`, operationID).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrMetadataContinuationNotFound
			}
			return err
		}
		if status == "completed" || status == "completed_with_failures" {
			complete = true
			return nil
		}
		if status == "cancelled" || status == "failed" || status == "retry_wait" {
			return nil
		}
		var openCursors, openItems, failed int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM metadata_continuation_cursors WHERE operation_id=? AND exhausted=0`, operationID).Scan(&openCursors); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM metadata_continuation_items WHERE operation_id=? AND state NOT IN ('succeeded','failed')`, operationID).Scan(&openItems); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT failed_count FROM metadata_continuation_operations WHERE id=?`, operationID).Scan(&failed); err != nil {
			return err
		}
		if openCursors != 0 || openItems != 0 {
			return nil
		}
		terminal := "completed"
		if failed > 0 {
			terminal = "completed_with_failures"
		}
		now := formatMetadataContinuationTime(s.now())
		if _, err := tx.ExecContext(ctx, `UPDATE metadata_continuation_operations SET status=?,completed_at=?,updated_at=? WHERE id=?`, terminal, now, now, operationID); err != nil {
			return err
		}
		complete = true
		return nil
	})
	return complete, err
}

func (s *MetadataContinuationStore) Failures(ctx context.Context, operationID string) ([]MetadataContinuationFailure, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT item_key,parent_key,item_kind,provider_id,last_error,attempts,updated_at FROM metadata_continuation_items WHERE operation_id=? AND state='failed' ORDER BY updated_at,item_key`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var failures []MetadataContinuationFailure
	for rows.Next() {
		var f MetadataContinuationFailure
		var updated string
		if err = rows.Scan(&f.Key, &f.ParentKey, &f.Kind, &f.ProviderID, &f.Error, &f.Attempts, &updated); err != nil {
			return nil, err
		}
		f.UpdatedAt = parseMetadataContinuationTime(updated)
		failures = append(failures, f)
	}
	return failures, rows.Err()
}

type metadataContinuationScanner interface{ Scan(...any) error }

func scanMetadataContinuationOperation(row metadataContinuationScanner) (MetadataContinuationOperation, error) {
	var op MetadataContinuationOperation
	var next, created, updated, completed string
	err := row.Scan(&op.ID, &op.RootKind, &op.RootID, &op.Provider, &op.PolicyRevision, &op.ProviderRevision, &op.Phase, &op.Cursor, &op.Status, &op.Processed, &op.Remaining, &op.Failed, &op.RetryCount, &next, &op.LastError, &created, &updated, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return op, ErrMetadataContinuationNotFound
	}
	if err != nil {
		return op, err
	}
	op.NextRetryAt = parseMetadataContinuationTime(next)
	op.CreatedAt = parseMetadataContinuationTime(created)
	op.UpdatedAt = parseMetadataContinuationTime(updated)
	op.CompletedAt = parseMetadataContinuationTime(completed)
	return op, nil
}

func scanMetadataContinuationItem(row metadataContinuationScanner) (MetadataContinuationItem, error) {
	var item MetadataContinuationItem
	var next, lease, created, updated string
	err := row.Scan(&item.OperationID, &item.Key, &item.ParentKey, &item.Kind, &item.ProviderID, &item.PayloadJSON, &item.State, &item.Attempts, &next, &lease, &item.LastError, &created, &updated)
	item.NextRetryAt = parseMetadataContinuationTime(next)
	item.LeaseUntil = parseMetadataContinuationTime(lease)
	item.CreatedAt = parseMetadataContinuationTime(created)
	item.UpdatedAt = parseMetadataContinuationTime(updated)
	return item, err
}

func formatMetadataContinuationTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
func parseMetadataContinuationTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}
func metadataContinuationBoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func metadataContinuationKindCanHaveChildren(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "show", "anime", "season", "artist", "album":
		return true
	default:
		return false
	}
}
func defaultJSON(value string) string {
	if value == "" {
		return "{}"
	}
	return value
}
