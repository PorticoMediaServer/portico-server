package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/diagnostics"
	"portico.local/server/internal/identity"
	"strconv"
	"time"
)

type Record struct {
	Sequence  int64            `json:"sequence"`
	Lane      string           `json:"lane"`
	At        int64            `json:"at"`
	Severity  string           `json:"severity"`
	Component string           `json:"component"`
	Code      string           `json:"code"`
	Fields    map[string]int64 `json:"fields"`
}
type AuditRecord struct {
	Sequence     int64  `json:"sequence"`
	ID           string `json:"id"`
	At           int64  `json:"at"`
	Actor        string `json:"actor"`
	Action       string `json:"action"`
	Target       string `json:"target"`
	Revision     int64  `json:"revision"`
	PreviousHash string `json:"previousHash"`
	Hash         string `json:"hash"`
}

func validComponent(v string) bool {
	switch v {
	case "scheduler", "storage", "playback", "client", "support":
		return true
	}
	return false
}

// Both client and server records reject arbitrary strings before persistence.
// Client uploads have a separate lane and cannot rotate server or audit rows.
func validRecord(lane, severity, component, code string, fields map[string]int64) bool {
	if lane != "runtime" && lane != "client" || !validComponent(component) || !validID(code) || len(code) > 64 || len(fields) > 12 {
		return false
	}
	switch severity {
	case "info", "warning", "error", "debug":
	default:
		return false
	}
	for key, value := range fields {
		switch key {
		case "processed", "attempt", "durationMs", "count", "bytes":
		default:
			return false
		}
		if value < 0 || value > 9007199254740991 {
			return false
		}
	}
	return true
}
func (s *Store) recordTx(tx *sql.Tx, lane, severity, component, code string, fields map[string]int64) error {
	if !validRecord(lane, severity, component, code, fields) {
		return ErrInvalid
	}
	if severity == "debug" {
		var body string
		if tx.QueryRow(`SELECT body FROM console_documents WHERE scope='capture'`).Scan(&body) != nil {
			return nil
		}
		var c Capture
		if decodeDocument(body, &c) != nil || c.ExpiresAt <= s.now() || c.Component != component {
			return nil
		}
	}
	b, _ := json.Marshal(fields)
	if _, e := tx.Exec(`INSERT INTO console_records(lane,time_ms,severity,component,code,fields) VALUES(?,?,?,?,?,?)`, lane, s.now(), severity, component, code, string(b)); e != nil {
		return e
	}
	limit := 20000
	if lane == "client" {
		limit = 5000
	}
	_, e := tx.Exec(`DELETE FROM console_records WHERE lane=? AND sequence NOT IN(SELECT sequence FROM console_records WHERE lane=? ORDER BY sequence DESC LIMIT ?)`, lane, lane, limit)
	return e
}
func (s *Store) Record(ctx context.Context, lane, severity, component, code string, fields map[string]int64) error {
	gated, e := dbwork.Begin(ctx, s.DB, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if e != nil {
		return e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if e = s.recordTx(tx, lane, severity, component, code, fields); e != nil {
		return e
	}
	return gated.Commit()
}
func readRecords(tx *sql.Tx, lane string, before, from, to int64, limit int) (Page[Record], error) {
	out := Page[Record]{Items: []Record{}}
	rows, e := tx.Query(`SELECT sequence,lane,time_ms,severity,component,code,fields FROM console_records WHERE lane=? AND sequence<? AND time_ms>=? AND time_ms<=? ORDER BY sequence DESC LIMIT ?`, lane, before, from, to, limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var r Record
		var fields string
		if e = rows.Scan(&r.Sequence, &r.Lane, &r.At, &r.Severity, &r.Component, &r.Code, &fields); e != nil {
			return out, e
		}
		if e = decodeDocument(fields, &r.Fields); e != nil {
			return out, e
		}
		// Validate again on projection; corrupt/migrated rows never become raw UI text.
		if !validRecord(r.Lane, r.Severity, r.Component, r.Code, r.Fields) {
			return out, ErrInvalid
		}
		out.Items = append(out.Items, r)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = strconv.FormatInt(out.Items[len(out.Items)-1].Sequence, 10)
	}
	return out, nil
}
func readAudit(tx *sql.Tx, before, from, to int64, limit int) (Page[AuditRecord], error) {
	out := Page[AuditRecord]{Items: []AuditRecord{}}
	rows, e := tx.Query(`SELECT sequence,id,time_ms,actor,action,target,revision,previous_hash,hash FROM console_audit WHERE sequence<? AND time_ms>=? AND time_ms<=? ORDER BY sequence DESC LIMIT ?`, before, from, to, limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var r AuditRecord
		if e = rows.Scan(&r.Sequence, &r.ID, &r.At, &r.Actor, &r.Action, &r.Target, &r.Revision, &r.PreviousHash, &r.Hash); e != nil {
			return out, e
		}
		out.Items = append(out.Items, r)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = strconv.FormatInt(out.Items[len(out.Items)-1].Sequence, 10)
	}
	return out, nil
}
func (s *Store) Records(ctx context.Context, p identity.Principal, auth Authorize, lane, cursor string) (out any, e error) {
	before, e := Cursor(cursor)
	if e != nil {
		return nil, e
	}
	e = s.snapshot(ctx, auth, "", func(tx *sql.Tx) error {
		var err error
		if lane == "audit" {
			out, err = readAudit(tx, before, 0, s.now(), 40)
		} else if lane == "runtime" || lane == "client" {
			out, err = readRecords(tx, lane, before, 0, s.now(), 40)
		} else {
			return ErrInvalid
		}
		return err
	})
	if e == nil && lane == "audit" {
		// C74: the audit entry for this read is a write, so it cannot share the
		// read snapshot (it failed there, and every audit-log read failed with it).
		// It is committed in its own short gated write before the page is returned;
		// if it cannot be recorded, the page is not returned.
		if e = s.auditAfterRead(ctx, auth, p, "audit.read", "audit"); e != nil {
			out = nil
		}
	}
	return
}

// auditAfterRead records an access audit entry in its own gated write, after a
// read that ran on a snapshot (C74). The caller withholds the read's result when
// this fails, so nothing is disclosed without its audit record.
func (s *Store) auditAfterRead(ctx context.Context, auth Authorize, p identity.Principal, action, target string) error {
	return s.transaction(ctx, auth, "", func(tx *sql.Tx) error {
		return Audit(tx, s.now(), AccountKey(p), action, target, 0)
	})
}

type Capture struct {
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	Component      string `json:"component"`
	ExpiresAt      int64  `json:"expiresAt"`
}

func (s *Store) Capture(ctx context.Context, p identity.Principal, auth Authorize, c Capture) error {
	if c.Component != "scheduler" || c.ExpiresAt < 0 || c.ExpiresAt > s.now()+int64(time.Hour/time.Millisecond) {
		return ErrInvalid
	}
	return s.transaction(ctx, auth, "", func(tx *sql.Tx) error {
		scope := "capture:" + AccountKey(p)
		raw, digest, e := Receipt(tx, scope, c.IdempotencyKey, c, s.now())
		if e != nil {
			return e
		}
		if raw != "" {
			return nil
		}
		// A replay can confirm an already-expired capture, but cannot reactivate it.
		saved := c
		saved.IdempotencyKey = ""
		b, _ := json.Marshal(saved)
		if _, e := tx.Exec(`INSERT INTO console_documents VALUES('capture',1,?,?) ON CONFLICT(scope) DO UPDATE SET revision=revision+1,body=excluded.body,updated_ms=excluded.updated_ms`, string(b), s.now()); e != nil {
			return e
		}
		if e := Audit(tx, s.now(), AccountKey(p), "diagnostics.capture", c.Component, 0); e != nil {
			return e
		}
		return SaveReceipt(tx, scope, c.IdempotencyKey, digest, map[string]bool{"accepted": true}, s.now())
	})
}

type ExportRequest struct {
	IdempotencyKey string   `json:"idempotencyKey"`
	From           int64    `json:"from"`
	To             int64    `json:"to"`
	Components     []string `json:"components"`
}
type Export struct {
	ID          string              `json:"id"`
	ExpiresAt   int64               `json:"expiresAt"`
	Manifest    ExportManifest      `json:"manifest"`
	Snapshot    *diagnostics.Report `json:"snapshot,omitempty"`
	Runtime     *Page[Record]       `json:"runtime,omitempty"`
	Client      *Page[Record]       `json:"client,omitempty"`
	Audit       *Page[AuditRecord]  `json:"audit,omitempty"`
	AuditAnchor *AuditAnchor        `json:"auditAnchor,omitempty"`
}
type ExportManifest struct {
	Version   string   `json:"version"`
	From      int64    `json:"from"`
	To        int64    `json:"to"`
	Included  []string `json:"included"`
	Excluded  []string `json:"excluded"`
	Redaction string   `json:"redaction"`
	MaxBytes  int      `json:"maxBytes"`
}

func (s *Store) CreateExport(ctx context.Context, p identity.Principal, auth Authorize, hosted bool, c ExportRequest) (out Export, e error) {
	if c.From < 0 || c.To < c.From || c.To > s.now()+60000 || c.To-c.From > days(7) || len(c.Components) == 0 || len(c.Components) > 4 {
		return out, ErrInvalid
	}
	seen := map[string]bool{}
	for _, v := range c.Components {
		if seen[v] {
			return out, ErrInvalid
		}
		seen[v] = true
		switch v {
		case "snapshot", "runtime", "client", "audit":
		default:
			return out, ErrInvalid
		}
	}
	// Preserve the existing support-report builder and its own bounded snapshot.
	var snapshot *diagnostics.Report
	if seen["snapshot"] {
		v, err := diagnostics.Read(ctx, s.DB, hosted)
		if err != nil {
			return out, err
		}
		snapshot = &v
	}
	e = s.transaction(ctx, auth, "", func(tx *sql.Tx) error {
		raw, digest, err := Receipt(tx, "export:"+p.Hash, c.IdempotencyKey, c, s.now())
		if err != nil {
			return err
		}
		if raw != "" {
			if err = decodeDocument(raw, &out); err != nil {
				return err
			}
			if out.ExpiresAt <= s.now() {
				return ErrExpired
			}
			return nil
		}
		var n int
		if err = tx.QueryRow(`SELECT count(*) FROM console_exports WHERE actor=? AND expires_ms>?`, p.Hash, s.now()).Scan(&n); err != nil {
			return err
		}
		if n >= 4 {
			return ErrCapacity
		}
		out = Export{ID: identity.Token(), ExpiresAt: s.now() + int64(time.Hour/time.Millisecond), Manifest: ExportManifest{RegistryRevision, c.From, c.To, c.Components, []string{"secrets", "raw logs", "media", "databases", "feedback text", "routes and paths"}, "Typed allowlist before persistence and projection; no automatic upload", 1 << 20}, Snapshot: snapshot}
		if seen["runtime"] {
			v, err := readRecords(tx, "runtime", 9223372036854775807, c.From, c.To, 1000)
			if err != nil {
				return err
			}
			out.Runtime = &v
		}
		if seen["client"] {
			v, err := readRecords(tx, "client", 9223372036854775807, c.From, c.To, 1000)
			if err != nil {
				return err
			}
			out.Client = &v
		}
		if seen["audit"] {
			v, err := readAudit(tx, 9223372036854775807, c.From, c.To, 1000)
			if err != nil {
				return err
			}
			out.Audit = &v
			anchor, err := auditAnchor(tx)
			if err != nil {
				return err
			}
			out.AuditAnchor = &anchor
		}
		b, err := json.Marshal(out)
		if err != nil {
			return err
		}
		if len(b) > 960<<10 {
			return ErrCapacity
		}
		if _, err = tx.Exec(`INSERT INTO console_exports VALUES(?,?,?,?)`, out.ID, p.Hash, out.ExpiresAt, string(b)); err != nil {
			return err
		}
		if err = Audit(tx, s.now(), AccountKey(p), "support.export", out.ID, 0); err != nil {
			return err
		}
		if err = SaveReceipt(tx, "export:"+p.Hash, c.IdempotencyKey, digest, out, s.now()); err != nil {
			return err
		}
		// The receipt contains the export; its private bytes must expire too.
		_, err = tx.Exec(`UPDATE console_receipts SET expires_ms=? WHERE scope=? AND key=?`, out.ExpiresAt, "export:"+p.Hash, c.IdempotencyKey)
		return err
	})
	return
}
func (s *Store) Export(ctx context.Context, p identity.Principal, auth Authorize, id string) (out Export, e error) {
	if !validID(id) {
		return out, ErrInvalid
	}
	e = s.snapshot(ctx, auth, "", func(tx *sql.Tx) error {
		var body string
		var expiry int64
		e := tx.QueryRow(`SELECT body,expires_ms FROM console_exports WHERE id=? AND actor=?`, id, p.Hash).Scan(&body, &expiry)
		if e != nil {
			return e
		}
		if expiry <= s.now() {
			return ErrExpired
		}
		return decodeDocument(body, &out)
	})
	if e == nil {
		// C74: audited in its own gated write; no audit, no download.
		if e = s.auditAfterRead(ctx, auth, p, "support.download", id); e != nil {
			out = Export{}
		}
	}
	return
}

type Alert struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	FirstAt     int64  `json:"firstAt"`
	LastAt      int64  `json:"lastAt"`
	Occurrences int64  `json:"occurrences"`
	Revision    int64  `json:"revision"`
}

func (s *Store) Alert(ctx context.Context, code, severity string, active bool) error {
	if !validID(code) || len(code) > 64 || severity != "warning" && severity != "critical" && severity != "info" {
		return ErrInvalid
	}
	if !active {
		// "This is still fine" was a gated write transaction and a commit, from
		// several loops, every second, forever. Resolving an alert that is not
		// raised is a read that finds nothing.
		var open int
		if e := dbwork.QueryRow(ctx, s.DB, `SELECT count(*) FROM console_alerts WHERE code=? AND status!='resolved'`, code).Scan(&open); e == nil && open == 0 {
			return nil
		}
	}
	gated2, e := dbwork.Begin(ctx, s.DB, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if e != nil {
		return e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	var id, status string
	var revision int64
	e = tx.QueryRow(`SELECT id,status,revision FROM console_alerts WHERE code=?`, code).Scan(&id, &status, &revision)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if !active {
		if _, e = tx.Exec(`UPDATE console_alerts SET status='resolved',revision=revision+1,last_ms=? WHERE code=? AND status!='resolved'`, s.now(), code); e != nil {
			return e
		}
		return gated2.Commit()
	}
	opened := e == sql.ErrNoRows || status == "resolved"
	if id == "" {
		id = identity.Token()
	}
	if _, e = tx.Exec(`INSERT INTO console_alerts VALUES(?,?,?,'open',?,?,1,1) ON CONFLICT(code) DO UPDATE SET status=CASE WHEN status='resolved' THEN 'open' ELSE status END,severity=excluded.severity,last_ms=excluded.last_ms,occurrences=occurrences+1,revision=revision+CASE WHEN status='resolved' OR severity!=excluded.severity THEN 1 ELSE 0 END`, id, code, severity, s.now(), s.now()); e != nil {
		return e
	}
	if opened {
		owners, e := ownerScopes(tx)
		if e != nil {
			return e
		}
		for _, scope := range owners {
			if e = notifyTx(tx, s.now(), scope, "owner-alert", id, id+":"+strconv.FormatInt(revision+1, 10)); e != nil {
				return e
			}
		}
	}
	return gated2.Commit()
}
func (s *Store) Alerts(ctx context.Context, auth Authorize) (out []Alert, e error) {
	out = []Alert{}
	e = s.snapshot(ctx, auth, "", func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id,code,severity,status,first_ms,last_ms,occurrences,revision FROM console_alerts ORDER BY CASE WHEN status='resolved' THEN 1 ELSE 0 END,last_ms DESC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a Alert
			if err = rows.Scan(&a.ID, &a.Code, &a.Severity, &a.Status, &a.FirstAt, &a.LastAt, &a.Occurrences, &a.Revision); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return
}
func (s *Store) Acknowledge(ctx context.Context, p identity.Principal, auth Authorize, id string, c JobCommand) error {
	if !validID(id) {
		return ErrInvalid
	}
	return s.transaction(ctx, auth, "", func(tx *sql.Tx) error {
		scope := "alert-ack:" + AccountKey(p)
		raw, digest, e := Receipt(tx, scope, c.IdempotencyKey, []any{id, c}, s.now())
		if e != nil {
			return e
		}
		if raw != "" {
			return nil
		}
		var revision int64
		var state string
		if e = tx.QueryRow(`SELECT revision,status FROM console_alerts WHERE id=?`, id).Scan(&revision, &state); e != nil {
			return e
		}
		if revision != c.ExpectedRevision || state != "open" {
			return &ConflictError{revision}
		}
		if _, e = tx.Exec(`UPDATE console_alerts SET status='acknowledged',revision=revision+1 WHERE id=?`, id); e != nil {
			return e
		}
		if e = Audit(tx, s.now(), AccountKey(p), "alert.acknowledge", id, revision+1); e != nil {
			return e
		}
		return SaveReceipt(tx, scope, c.IdempotencyKey, digest, map[string]bool{"acknowledged": true}, s.now())
	})
}
func (s *Store) Prune(ctx context.Context) error {
	gated3, e := dbwork.Begin(ctx, s.DB, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if e != nil {
		return e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	v, e := readSettings(tx)
	if e != nil {
		return e
	}
	now := s.now()
	statements := []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM console_receipts WHERE expires_ms<=?`, []any{now}},
		{`DELETE FROM console_exports WHERE expires_ms<=?`, []any{now}},
		{`DELETE FROM console_alerts WHERE status='resolved' AND last_ms<?`, []any{now - days(v.Effective.JobDays)}},
		{`DELETE FROM console_records WHERE time_ms<?`, []any{now - days(v.Effective.DiagnosticDays)}},
		{`DELETE FROM console_reports WHERE expires_ms<=?`, []any{now}},
		{`DELETE FROM console_notifications WHERE expires_ms<=? OR created_ms<?`, []any{now, now - days(v.Effective.NotificationDays)}},
		{`DELETE FROM console_operations WHERE state IN ('succeeded','failed','cancelled') AND updated_ms<?`, []any{now - days(v.Effective.JobDays)}},
		{`DELETE FROM console_documents WHERE scope LIKE 'session:%' AND NOT EXISTS(SELECT 1 FROM authorization_access WHERE hash=substr(console_documents.scope,9) AND revoked=0 AND expires_at>?)`, []any{s.Now().UTC().Format(time.RFC3339)}},
	}
	if keep := v.Effective.PlayHistoryDays; keep > 0 {
		// The server's play history is kept in full unless the owner asks for less.
		// A bounded batch per tick: shortening the setting on a long history must
		// not hold the write lock while a million rows go.
		statements = append(statements, struct {
			sql  string
			args []any
		}{`DELETE FROM play_history WHERE id IN(SELECT id FROM play_history WHERE started_ms<? ORDER BY started_ms LIMIT 5000)`, []any{now - days(keep)}})
	}
	for _, q := range statements {
		if _, e = tx.ExecContext(ctx, q.sql, q.args...); e != nil {
			return e
		}
	}
	// The notification inbox prunes with its own revision bookkeeping, so a
	// client parked on a revision learns that its counts moved.
	if e = s.pruneNotifications(tx, now, v.Effective.NotificationDays); e != nil {
		return e
	}
	// Keep a trusted-host checkpoint before dropping old links from the chain.
	var sequence int64
	var hash string
	e = tx.QueryRow(`SELECT sequence,hash FROM console_audit WHERE time_ms<? AND sequence<COALESCE((SELECT min(sequence) FROM console_audit WHERE time_ms>=?),9223372036854775807) ORDER BY sequence DESC LIMIT 1`, now-days(90), now-days(90)).Scan(&sequence, &hash)
	if e == nil {
		b, _ := json.Marshal(map[string]any{"sequence": sequence, "hash": hash})
		if _, e = tx.Exec(`INSERT INTO console_documents VALUES('audit-anchor',1,?,?) ON CONFLICT(scope) DO UPDATE SET revision=revision+1,body=excluded.body,updated_ms=excluded.updated_ms`, string(b), now); e != nil {
			return e
		}
		if _, e = tx.Exec(`DELETE FROM console_audit WHERE sequence<=?`, sequence); e != nil {
			return e
		}
	} else if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	return gated3.Commit()
}
