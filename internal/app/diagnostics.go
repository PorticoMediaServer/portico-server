package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/PorticoMediaServer/portico-server/internal/redaction"
)

const (
	serverDiagnosticQueueByteLimit = int64(4 << 20)
	clientDiagnosticQueueByteLimit = int64(2 << 20)
	serverDiagnosticRecordLimit    = int64(16 << 20)
	clientDiagnosticRecordLimit    = int64(8 << 20)
	serverDiagnosticMaxRows        = 5000
	clientDiagnosticMaxRows        = 2000
	clientDiagnosticWindowDuration = time.Minute
	clientDiagnosticEntriesPerMin  = 200
	clientDiagnosticBytesPerMin    = 256 << 10
)

type serverDiagnosticRecord struct {
	event LogEvent
	size  int64
}

type clientDiagnosticRecord struct {
	id        string
	accountID string
	device    string
	app       string
	origin    string
	level     string
	message   string
	fields    map[string]string
	clientAt  string
	size      int64
}

type clientDiagnosticWindow struct {
	startedAt time.Time
	entries   int
	bytes     int
}

type securityAuditInput struct {
	ID           string
	ActorUserID  string
	ActorEmail   string
	Action       string
	ResourceType string
	ResourceID   string
	Severity     string
	MetadataJSON string
	ClientIP     string
	UserAgent    string
	CreatedAt    string
}

func (s *Server) diagnosticRedactionPolicy() redaction.Policy {
	if s == nil {
		return redaction.Policy{}
	}
	return redaction.Policy{
		SensitivePaths: []string{
			s.cfg.AppDataDir, s.cfg.ConfigPath, s.cfg.DatabasePath, s.cfg.BackupDir,
			s.cfg.WebDistDir, s.cfg.LogFilePath, s.cfg.TranscodeDir,
			s.cfg.FFmpegPath, s.cfg.FFprobePath, s.cfg.FPcalcPath,
		},
		SensitiveValues: []string{s.cfg.TMDBReadAccessToken, s.cfg.TMDBAPIKey, s.cfg.AcoustIDAPIKey},
	}
}

func (s *Server) sanitizeDiagnosticText(value string, limit int) string {
	value = redactClientLogText(value)
	value = s.diagnosticRedactionPolicy().RedactString(value)
	value = redactLogValue(value)
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return ' '
	}, value)
	value = strings.TrimSpace(value)
	if limit > 0 && len(value) > limit {
		value = value[:limit]
	}
	return value
}

func (s *Server) sanitizeDiagnosticFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	redacted := make(map[string]string, min(len(keys), 32))
	for _, rawKey := range keys {
		if len(redacted) >= 32 {
			break
		}
		key := s.sanitizeDiagnosticText(rawKey, 80)
		if key == "" {
			continue
		}
		if sensitiveLogKeyPattern.MatchString(key) {
			redacted[key] = "[redacted]"
			continue
		}
		if isLogicalDiagnosticRouteKey(key, fields[rawKey]) {
			redacted[key] = sanitizeLogicalDiagnosticRoute(fields[rawKey])
			continue
		}
		redacted[key] = s.sanitizeDiagnosticText(fields[rawKey], 500)
	}
	return redacted
}

func (s *Server) sanitizeDiagnosticJSON(raw string) string {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return "{}"
	}
	var sanitize func(any, int) any
	sanitize = func(current any, depth int) any {
		if depth > 8 {
			return "[redacted-depth]"
		}
		switch typed := current.(type) {
		case map[string]any:
			clean := make(map[string]any, len(typed))
			for key, child := range typed {
				cleanKey := s.sanitizeDiagnosticText(key, 80)
				if cleanKey == "" {
					continue
				}
				if sensitiveLogKeyPattern.MatchString(cleanKey) || redaction.IsSensitiveKey(cleanKey) {
					clean[cleanKey] = "[redacted]"
					continue
				}
				clean[cleanKey] = sanitize(child, depth+1)
			}
			return clean
		case []any:
			clean := make([]any, len(typed))
			for index, child := range typed {
				clean[index] = sanitize(child, depth+1)
			}
			return clean
		case string:
			return s.sanitizeDiagnosticText(typed, 2000)
		default:
			return typed
		}
	}
	encoded, err := json.Marshal(sanitize(value, 0))
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func isLogicalDiagnosticRouteKey(key, value string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(key, "route") && !strings.HasSuffix(key, ".route") {
		return false
	}
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") {
		return false
	}
	first := strings.TrimPrefix(strings.SplitN(value, "?", 2)[0], "/")
	first = strings.SplitN(first, "/", 2)[0]
	switch strings.ToLower(first) {
	case "api", "watch", "home", "library", "settings", "player", "login", "search", "media", "live", "downloads", "cast", "notifications", "maintenance", "diagnostics":
		return true
	default:
		return false
	}
}

func sanitizeLogicalDiagnosticRoute(value string) string {
	value = redactLogValue(value)
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return ' '
	}, value)
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}

func (s *Server) enqueueServerDiagnostic(event LogEvent) {
	if s == nil {
		return
	}
	fieldsJSON, err := json.Marshal(event.Fields)
	if err != nil {
		return
	}
	size := int64(len(event.ID) + len(event.Time) + len(event.Level) + len(event.Message) + len(fieldsJSON))
	if size <= 0 || size > 64<<10 {
		s.diagnosticServerDropped.Add(1)
		return
	}
	record := serverDiagnosticRecord{event: event, size: size}
	s.diagnosticMu.Lock()
	if s.diagnosticClosing || s.serverDiagnosticQueue == nil || s.serverDiagnosticQueueBytes+size > serverDiagnosticQueueByteLimit {
		s.diagnosticMu.Unlock()
		s.diagnosticServerDropped.Add(1)
		return
	}
	select {
	case s.serverDiagnosticQueue <- record:
		s.serverDiagnosticQueueBytes += size
		s.diagnosticMu.Unlock()
	default:
		s.diagnosticMu.Unlock()
		s.diagnosticServerDropped.Add(1)
	}
}

func (s *Server) nextServerDiagnostic(ctx context.Context) (serverDiagnosticRecord, bool) {
	if s == nil || s.serverDiagnosticQueue == nil {
		return serverDiagnosticRecord{}, false
	}
	select {
	case record := <-s.serverDiagnosticQueue:
		s.diagnosticMu.Lock()
		s.serverDiagnosticQueueBytes = maxInt64(0, s.serverDiagnosticQueueBytes-record.size)
		s.diagnosticMu.Unlock()
		return record, true
	case <-ctx.Done():
		return serverDiagnosticRecord{}, false
	}
}

func (s *Server) persistServerDiagnostic(ctx context.Context, record serverDiagnosticRecord) error {
	fieldsJSON, err := json.Marshal(record.event.Fields)
	if err != nil {
		return err
	}
	_, err = s.execBackgroundWrite(ctx, `
		INSERT INTO server_diagnostic_events (id, level, message, fields_json, source, byte_size, created_at)
		VALUES (?, ?, ?, ?, 'server', ?, ?)`,
		record.event.ID, record.event.Level, s.sanitizeDiagnosticText(record.event.Message, 4000),
		string(fieldsJSON), record.size, record.event.Time)
	return err
}

func (s *Server) runServerDiagnosticWriter(ctx context.Context) {
	for {
		record, ok := s.nextServerDiagnostic(ctx)
		if !ok {
			if ctx.Err() != nil {
				s.drainServerDiagnostics(250 * time.Millisecond)
			}
			return
		}
		if err := s.persistServerDiagnostic(ctx, record); err != nil && ctx.Err() == nil {
			s.diagnosticServerDropped.Add(1)
		}
	}
}

func (s *Server) drainServerDiagnostics(budget time.Duration) {
	deadline, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	for {
		record, ok := s.nextServerDiagnostic(deadline)
		if !ok {
			return
		}
		if err := s.persistServerDiagnostic(deadline, record); err != nil {
			return
		}
	}
}

func (s *Server) allowClientDiagnostic(key string, entries, bytes int) bool {
	if s == nil || entries <= 0 || bytes <= 0 {
		return false
	}
	now := time.Now().UTC()
	s.diagnosticMu.Lock()
	defer s.diagnosticMu.Unlock()
	if s.clientDiagnosticWindows == nil {
		s.clientDiagnosticWindows = map[string]clientDiagnosticWindow{}
	}
	window := s.clientDiagnosticWindows[key]
	if window.startedAt.IsZero() || now.Sub(window.startedAt) >= clientDiagnosticWindowDuration {
		window = clientDiagnosticWindow{startedAt: now}
	}
	if window.entries+entries > clientDiagnosticEntriesPerMin || window.bytes+bytes > clientDiagnosticBytesPerMin {
		return false
	}
	window.entries += entries
	window.bytes += bytes
	if _, exists := s.clientDiagnosticWindows[key]; !exists && len(s.clientDiagnosticWindows) >= 1024 {
		oldestKey := ""
		oldestAt := now
		for candidate, item := range s.clientDiagnosticWindows {
			if oldestKey == "" || item.startedAt.Before(oldestAt) {
				oldestKey = candidate
				oldestAt = item.startedAt
			}
		}
		if oldestKey != "" {
			delete(s.clientDiagnosticWindows, oldestKey)
		}
	}
	s.clientDiagnosticWindows[key] = window
	return true
}

func (s *Server) enqueueClientDiagnostic(record clientDiagnosticRecord) bool {
	if s == nil || record.size <= 0 || record.size > 64<<10 {
		return false
	}
	s.diagnosticMu.Lock()
	if s.diagnosticClosing {
		s.diagnosticMu.Unlock()
		s.diagnosticClientDropped.Add(1)
		return false
	}
	// Inert/test-constructed servers do not have a writer goroutine. Persist
	// their bounded record synchronously rather than claiming acceptance for a
	// queue that nobody will drain. Normal generations initialize this queue
	// before serving and always take the bounded asynchronous path below.
	if s.clientDiagnosticQueue == nil {
		s.diagnosticMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		err := s.persistClientDiagnostic(ctx, record)
		cancel()
		if err != nil {
			s.diagnosticClientDropped.Add(1)
			return false
		}
		return true
	}
	if s.clientDiagnosticQueueBytes+record.size > clientDiagnosticQueueByteLimit {
		s.diagnosticMu.Unlock()
		s.diagnosticClientDropped.Add(1)
		return false
	}
	select {
	case s.clientDiagnosticQueue <- record:
		s.clientDiagnosticQueueBytes += record.size
		s.diagnosticMu.Unlock()
		return true
	default:
		s.diagnosticMu.Unlock()
		s.diagnosticClientDropped.Add(1)
		return false
	}
}

func (s *Server) nextClientDiagnostic(ctx context.Context) (clientDiagnosticRecord, bool) {
	if s == nil || s.clientDiagnosticQueue == nil {
		return clientDiagnosticRecord{}, false
	}
	select {
	case record := <-s.clientDiagnosticQueue:
		s.diagnosticMu.Lock()
		s.clientDiagnosticQueueBytes = maxInt64(0, s.clientDiagnosticQueueBytes-record.size)
		s.diagnosticMu.Unlock()
		return record, true
	case <-ctx.Done():
		return clientDiagnosticRecord{}, false
	}
}

func (s *Server) persistClientDiagnostic(ctx context.Context, record clientDiagnosticRecord) error {
	// Sanitize every independently queryable column at the persistence boundary;
	// callers are not trusted to have passed through the HTTP normalization path.
	record.device = s.sanitizeDiagnosticText(record.device, 80)
	record.app = s.sanitizeDiagnosticText(record.app, 80)
	record.origin = s.sanitizeDiagnosticText(record.origin, 200)
	record.clientAt = s.sanitizeDiagnosticText(record.clientAt, 64)
	record.fields = s.sanitizeDiagnosticFields(record.fields)
	fieldsJSON, err := json.Marshal(record.fields)
	if err != nil {
		return err
	}
	_, err = s.execBackgroundWrite(ctx, `
		INSERT INTO client_diagnostic_events (id, account_id, device, app, origin, level, message, fields_json, client_time, byte_size, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.id, record.accountID, record.device, record.app, record.origin, record.level,
		s.sanitizeDiagnosticText(record.message, 4000), string(fieldsJSON), record.clientAt,
		record.size, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Server) runClientDiagnosticWriter(ctx context.Context) {
	for {
		record, ok := s.nextClientDiagnostic(ctx)
		if !ok {
			if ctx.Err() != nil {
				s.drainClientDiagnostics(250 * time.Millisecond)
			}
			return
		}
		if err := s.persistClientDiagnostic(ctx, record); err != nil && ctx.Err() == nil {
			s.diagnosticClientDropped.Add(1)
		}
	}
}

func (s *Server) drainClientDiagnostics(budget time.Duration) {
	deadline, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	for {
		record, ok := s.nextClientDiagnostic(deadline)
		if !ok {
			return
		}
		if err := s.persistClientDiagnostic(deadline, record); err != nil {
			return
		}
	}
}

func (s *Server) listPersistedServerDiagnosticEvents(limit int, cursorTime, cursorID string) []LogEvent {
	limit = normalizedLogLimit(limit)
	args := []any{}
	where := ""
	if cursorTime != "" && cursorID != "" {
		where = "WHERE (created_at < ? OR (created_at = ? AND id < ?))"
		args = append(args, cursorTime, cursorTime, cursorID)
	}
	args = append(args, limit+1)
	rows, err := s.queryBackgroundRead(context.Background(), `
		SELECT id, created_at, level, message, fields_json
		FROM server_diagnostic_events `+where+`
		ORDER BY created_at DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	events := make([]LogEvent, 0, limit+1)
	for rows.Next() {
		var event LogEvent
		var fieldsJSON string
		if err := rows.Scan(&event.ID, &event.Time, &event.Level, &event.Message, &fieldsJSON); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(fieldsJSON), &event.Fields)
		events = append(events, event)
	}
	return events
}

func (s *Server) pruneDiagnosticTables(ctx context.Context) error {
	retention := s.ownerRetentionSettings()
	now := s.operationalRetentionNow()
	if retention.DiagnosticHistoryDays > 0 {
		cutoff := now.AddDate(0, 0, -retention.DiagnosticHistoryDays).Format(time.RFC3339Nano)
		if _, err := s.execBackgroundWrite(ctx, `DELETE FROM server_diagnostic_events WHERE created_at < ?`, cutoff); err != nil {
			return err
		}
	}
	if retention.ClientDiagnosticDays > 0 {
		cutoff := now.AddDate(0, 0, -retention.ClientDiagnosticDays).Format(time.RFC3339Nano)
		if _, err := s.execBackgroundWrite(ctx, `DELETE FROM client_diagnostic_events WHERE created_at < ?`, cutoff); err != nil {
			return err
		}
	}
	if retention.DiagnosticHistoryDays > 0 {
		if err := s.pruneDiagnosticLanePastLimit(ctx, "server_diagnostic_events", serverDiagnosticMaxRows, serverDiagnosticRecordLimit); err != nil {
			return err
		}
	}
	if retention.ClientDiagnosticDays > 0 {
		return s.pruneDiagnosticLanePastLimit(ctx, "client_diagnostic_events", clientDiagnosticMaxRows, clientDiagnosticRecordLimit)
	}
	return nil
}

func (s *Server) pruneDiagnosticLanePastLimit(ctx context.Context, table string, maxRows int, maxBytes int64) error {
	if maxRows > 0 {
		if _, err := s.execBackgroundWrite(ctx, `DELETE FROM `+table+` WHERE id IN (SELECT id FROM `+table+` ORDER BY created_at DESC, id DESC LIMIT -1 OFFSET ?)`, maxRows); err != nil {
			return err
		}
	}
	if maxBytes <= 0 {
		return nil
	}
	for {
		var total int64
		if err := s.queryBackgroundRow(ctx, `SELECT COALESCE(SUM(byte_size), 0) FROM `+table).Scan(&total); err != nil || total <= maxBytes {
			return err
		}
		var id string
		if err := s.queryBackgroundRow(ctx, `SELECT id FROM `+table+` ORDER BY created_at ASC, id ASC LIMIT 1`).Scan(&id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		if _, err := s.execBackgroundWrite(ctx, `DELETE FROM `+table+` WHERE id = ?`, id); err != nil {
			return err
		}
	}
}

func (s *Server) pruneSecurityAuditEventsPastLimit(ctx context.Context, maxRows int) error {
	retention := s.ownerRetentionSettings()
	if retention.AuditHistoryDays <= 0 {
		return nil
	}
	cutoff := s.operationalRetentionNow().AddDate(0, 0, -retention.AuditHistoryDays).Format(time.RFC3339Nano)
	return s.withBackgroundTxTagged(ctx, []string{"audit"}, func(tx *sql.Tx) error {
		var deleteThrough sql.NullInt64
		if err := tx.QueryRowContext(ctx, `
			SELECT MAX(sequence) FROM security_audit_events
			WHERE created_at < ? OR sequence IN (
				SELECT sequence FROM security_audit_events
				ORDER BY sequence DESC LIMIT -1 OFFSET ?
			)`, cutoff, max(1, maxRows)).Scan(&deleteThrough); err != nil {
			return err
		}
		if !deleteThrough.Valid {
			return nil
		}
		var anchor string
		if err := tx.QueryRowContext(ctx, `SELECT event_hash FROM security_audit_events WHERE sequence = ?`, deleteThrough.Int64).Scan(&anchor); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO security_audit_chain_state (id, anchor_previous_hash, head_sequence, head_hash, updated_at)
			VALUES (1, ?, 0, ?, ?)
			ON CONFLICT(id) DO UPDATE SET anchor_previous_hash = excluded.anchor_previous_hash,
				head_sequence = CASE WHEN head_sequence <= ? THEN 0 ELSE head_sequence END,
				head_hash = CASE WHEN head_sequence <= ? THEN excluded.head_hash ELSE head_hash END,
				updated_at = excluded.updated_at`, anchor, anchor, s.operationalRetentionNow().Format(time.RFC3339Nano), deleteThrough.Int64, deleteThrough.Int64); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM audit_events WHERE id IN (SELECT id FROM security_audit_events WHERE sequence <= ?)`, deleteThrough.Int64); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM security_audit_events WHERE sequence <= ?`, deleteThrough.Int64)
		return err
	})
}

func securityAuditHashPayload(input securityAuditInput) []byte {
	payload := struct {
		ID           string          `json:"id"`
		ActorUserID  string          `json:"actorUserId"`
		ActorEmail   string          `json:"actorEmail"`
		Action       string          `json:"action"`
		ResourceType string          `json:"resourceType"`
		ResourceID   string          `json:"resourceId"`
		Severity     string          `json:"severity"`
		Metadata     json.RawMessage `json:"metadata"`
		ClientIP     string          `json:"clientIp"`
		UserAgent    string          `json:"userAgent"`
		CreatedAt    string          `json:"createdAt"`
	}{
		ID: input.ID, ActorUserID: input.ActorUserID, ActorEmail: input.ActorEmail, Action: input.Action,
		ResourceType: input.ResourceType, ResourceID: input.ResourceID, Severity: input.Severity,
		Metadata: json.RawMessage(firstNonEmpty(input.MetadataJSON, "{}")),
		ClientIP: input.ClientIP, UserAgent: input.UserAgent, CreatedAt: input.CreatedAt,
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func recordSecurityAuditEventTx(ctx context.Context, tx *sql.Tx, input securityAuditInput) error {
	var previousHash string
	if err := tx.QueryRowContext(ctx, `SELECT event_hash FROM security_audit_events ORDER BY sequence DESC LIMIT 1`).Scan(&previousHash); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT anchor_previous_hash FROM security_audit_chain_state WHERE id = 1`).Scan(&previousHash); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	payload := securityAuditHashPayload(input)
	hashInput := make([]byte, 0, len(previousHash)+1+len(payload))
	hashInput = append(hashInput, previousHash...)
	hashInput = append(hashInput, 0)
	hashInput = append(hashInput, payload...)
	sum := sha256.Sum256(hashInput)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO security_audit_events (id, previous_hash, event_hash, actor_user_id, actor_email, action, resource_type, resource_id, severity, metadata_json, client_ip, user_agent, byte_size, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.ID, previousHash, hex.EncodeToString(sum[:]), input.ActorUserID, input.ActorEmail,
		input.Action, input.ResourceType, input.ResourceID, input.Severity, input.MetadataJSON,
		input.ClientIP, input.UserAgent, len(payload), input.CreatedAt)
	if err != nil {
		return err
	}
	sequence, err := result.LastInsertId()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO security_audit_chain_state (id, anchor_previous_hash, head_sequence, head_hash, updated_at)
		VALUES (1, '', ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET head_sequence = excluded.head_sequence,
			head_hash = excluded.head_hash, updated_at = excluded.updated_at`,
		sequence, hex.EncodeToString(sum[:]), input.CreatedAt)
	return err
}

func (s *Server) backfillSecurityAuditEventsTx(ctx context.Context, tx *sql.Tx) error {
	var stateExists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM security_audit_chain_state WHERE id = 1`).Scan(&stateExists); err != nil {
		return err
	}
	if stateExists > 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT a.id, a.actor_user_id, a.actor_email, a.action, a.resource_type,
			a.resource_id, a.severity, a.metadata_json, a.client_ip, a.user_agent, a.created_at
		FROM audit_events a
		LEFT JOIN security_audit_events s ON s.id = a.id
		WHERE s.id IS NULL
		ORDER BY a.created_at ASC, a.id ASC`)
	if err != nil {
		return err
	}
	inputs := []securityAuditInput{}
	for rows.Next() {
		var input securityAuditInput
		if err := rows.Scan(&input.ID, &input.ActorUserID, &input.ActorEmail, &input.Action,
			&input.ResourceType, &input.ResourceID, &input.Severity, &input.MetadataJSON,
			&input.ClientIP, &input.UserAgent, &input.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		inputs = append(inputs, input)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, input := range inputs {
		input.ActorUserID = s.sanitizeDiagnosticText(input.ActorUserID, 240)
		input.ActorEmail = s.sanitizeDiagnosticText(input.ActorEmail, 240)
		input.Action = s.sanitizeDiagnosticText(input.Action, 160)
		input.ResourceType = s.sanitizeDiagnosticText(input.ResourceType, 120)
		input.ResourceID = s.sanitizeDiagnosticText(input.ResourceID, 240)
		input.Severity = s.sanitizeDiagnosticText(input.Severity, 32)
		input.ClientIP = s.sanitizeDiagnosticText(input.ClientIP, 80)
		input.UserAgent = s.sanitizeDiagnosticText(input.UserAgent, 240)
		input.MetadataJSON = s.sanitizeDiagnosticJSON(input.MetadataJSON)
		if err := recordSecurityAuditEventTx(ctx, tx, input); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) ensureSecurityAuditChainCoverage(ctx context.Context) error {
	return s.withBackgroundTxTagged(ctx, []string{"audit"}, func(tx *sql.Tx) error {
		return s.backfillSecurityAuditEventsTx(ctx, tx)
	})
}

func (s *Server) verifySecurityAuditChain(ctx context.Context) error {
	var anchor, expectedHeadHash string
	var expectedHeadSequence int64
	if err := s.queryBackgroundRow(ctx, `
		SELECT anchor_previous_hash, head_sequence, head_hash
		FROM security_audit_chain_state WHERE id = 1`).Scan(&anchor, &expectedHeadSequence, &expectedHeadHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	rows, err := s.queryBackgroundRead(ctx, `
		SELECT sequence, id, previous_hash, event_hash, actor_user_id, actor_email, action, resource_type,
			resource_id, severity, metadata_json, client_ip, user_agent, created_at
		FROM security_audit_events ORDER BY sequence ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	previous := anchor
	first := true
	var lastSequence int64
	for rows.Next() {
		var sequence int64
		var storedPrevious, storedHash string
		var input securityAuditInput
		if err := rows.Scan(&sequence, &input.ID, &storedPrevious, &storedHash, &input.ActorUserID, &input.ActorEmail, &input.Action, &input.ResourceType, &input.ResourceID, &input.Severity, &input.MetadataJSON, &input.ClientIP, &input.UserAgent, &input.CreatedAt); err != nil {
			return err
		}
		if storedPrevious != previous {
			return fmt.Errorf("security audit chain previous hash mismatch")
		}
		first = false
		payload := securityAuditHashPayload(input)
		hashInput := append(append([]byte(previous), 0), payload...)
		sum := sha256.Sum256(hashInput)
		if storedHash != hex.EncodeToString(sum[:]) {
			return fmt.Errorf("security audit chain hash mismatch")
		}
		previous = storedHash
		lastSequence = sequence
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if first {
		if expectedHeadSequence != 0 || expectedHeadHash != anchor {
			return fmt.Errorf("security audit chain head is missing")
		}
		return nil
	}
	if lastSequence != expectedHeadSequence || previous != expectedHeadHash {
		return fmt.Errorf("security audit chain head mismatch")
	}
	return nil
}
