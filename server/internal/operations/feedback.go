package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/identity"
	"strconv"
)

// Diagnostics are an allowlisted snapshot, not arbitrary client log text.
// No headers, URLs, device identifiers, titles or paths can enter this type.
type ClientDiagnostic struct {
	Platform string `json:"platform"`
	State    string `json:"state"`
	Online   bool   `json:"online"`
}

func validDiagnostic(d *ClientDiagnostic) bool {
	if d == nil {
		return true
	}
	if d.Platform != "web" && d.Platform != "ios" && d.Platform != "tvos" {
		return false
	}
	switch d.State {
	case "idle", "starting", "playing", "paused", "buffering", "failed", "unknown":
		return true
	}
	return false
}

type SubmitReport struct {
	IdempotencyKey string            `json:"idempotencyKey"`
	Category       string            `json:"category"`
	Message        string            `json:"message"`
	ItemID         string            `json:"itemId"`
	Diagnostic     *ClientDiagnostic `json:"diagnostic,omitempty"`
}
type Report struct {
	ID               string            `json:"id"`
	Sequence         int64             `json:"-"`
	CreatedAt        int64             `json:"createdAt"`
	UpdatedAt        int64             `json:"updatedAt"`
	Revision         int64             `json:"revision"`
	Status           string            `json:"status"`
	Category         string            `json:"category"`
	Message          string            `json:"message"`
	ItemID           string            `json:"itemId,omitempty"`
	Diagnostic       *ClientDiagnostic `json:"diagnostic,omitempty"`
	ContextAvailable bool              `json:"contextAvailable"`
	Events           []ReportEvent     `json:"events,omitempty"`
}
type ReportEvent struct {
	Sequence   int64  `json:"sequence"`
	At         int64  `json:"at"`
	Revision   int64  `json:"revision"`
	Status     string `json:"status"`
	Reply      string `json:"reply"`
	ActorClass string `json:"actorClass"`
}
type Triage struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	IdempotencyKey   string `json:"idempotencyKey"`
	Status           string `json:"status"`
	Reply            string `json:"reply"`
}

const reportColumns = `id,sequence,created_ms,updated_ms,revision,status,category,message,COALESCE((SELECT pid(public_id) FROM catalog_entities WHERE id=console_reports.item_id),''),attachment`

// resolveReportItem maps a report's public item id to the integer
// console_reports stores (0 when there is none). An unknown id stores 0: the
// readers blank unreachable media context through auth, so a dangling
// reference was never observable and 0 reads the same way.
func resolveReportItem(ctx context.Context, tx *sql.Tx, public string) (int64, error) {
	if public == "" {
		return 0, nil
	}
	entity, err := entityid.Resolve(ctx, tx, public)
	if errors.Is(err, entityid.ErrNotFound) {
		return 0, nil
	}
	return entity, err
}

func scanReport(row interface{ Scan(...any) error }) (Report, error) {
	var out Report
	var raw string
	e := row.Scan(&out.ID, &out.Sequence, &out.CreatedAt, &out.UpdatedAt, &out.Revision, &out.Status, &out.Category, &out.Message, &out.ItemID, &raw)
	if e == nil && raw != "" {
		e = decodeDocument(raw, &out.Diagnostic)
	}
	out.ContextAvailable = true
	return out, e
}
func reportVisible(ctx context.Context, tx *sql.Tx, auth Authorize, r *Report) {
	if r.ItemID != "" && auth(ctx, tx, r.ItemID) != nil {
		r.ContextAvailable = false
		r.ItemID = ""
		r.Message = "Media context is no longer accessible."
		r.Diagnostic = nil
		r.Events = nil
	}
}
func (s *Store) Submit(ctx context.Context, p identity.Principal, auth Authorize, c SubmitReport) (out Report, e error) {
	if !SafeText(c.Message, 4000) || !validDiagnostic(c.Diagnostic) || c.ItemID != "" && !validID(c.ItemID) {
		return out, ErrInvalid
	}
	switch c.Category {
	case "playback", "metadata", "subtitles", "other":
	default:
		return out, ErrInvalid
	}
	scope := ViewerKey(p)
	e = s.transaction(ctx, auth, c.ItemID, func(tx *sql.Tx) error {
		raw, digest, err := Receipt(tx, "report:"+scope, c.IdempotencyKey, c, s.now())
		if err != nil {
			return err
		}
		if raw != "" {
			return decodeDocument(raw, &out)
		}
		var n, recent, total int
		if err = tx.QueryRow(`SELECT count(*),COALESCE(sum(created_ms>?),0) FROM console_reports WHERE scope=?`, s.now()-60000, scope).Scan(&n, &recent); err != nil {
			return err
		}
		if err = tx.QueryRow(`SELECT count(*) FROM console_reports`).Scan(&total); err != nil {
			return err
		}
		if n >= 500 || recent >= 5 || total >= 10000 {
			return ErrCapacity
		}
		attachment := ""
		if c.Diagnostic != nil {
			b, _ := json.Marshal(c.Diagnostic)
			attachment = string(b)
		}
		id := identity.Token()
		now := s.now()
		entity, err := resolveReportItem(ctx, tx, c.ItemID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO console_reports(id,scope,created_ms,updated_ms,revision,status,category,message,item_id,attachment,expires_ms) VALUES(?,?,?,?,1,'open',?,?,?,?,?)`, id, scope, now, now, c.Category, c.Message, entity, attachment, now+days(180)); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO console_report_events(report_id,time_ms,revision,status,reply,actor_class) VALUES(?,?,1,'open','','viewer')`, id, now); err != nil {
			return err
		}
		// Owners receive an opaque local notice; report contents never enter the inbox.
		owners, err := ownerScopes(tx)
		if err != nil {
			return err
		}
		if c.Diagnostic != nil {
			// These categorical fields are already allowlisted. Report text and
			// item identity are excluded from the separate diagnostic lane.
			code := c.Diagnostic.Platform + "-" + c.Diagnostic.State
			if err = s.recordTx(tx, "client", "info", "client", code, map[string]int64{"count": 1}); err != nil {
				return err
			}
		}
		for _, owner := range owners {
			if err = notifyTx(tx, now, owner, "feedback-received", id, id+":submitted"); err != nil {
				return err
			}
		}
		out, err = scanReport(tx.QueryRow(`SELECT `+reportColumns+` FROM console_reports WHERE id=?`, id))
		if err != nil {
			return err
		}
		return SaveReceipt(tx, "report:"+scope, c.IdempotencyKey, digest, out, now)
	})
	return
}
func (s *Store) Reports(ctx context.Context, p identity.Principal, auth Authorize, owner bool, cursor string, limit int) (out Page[Report], e error) {
	out.Items = []Report{}
	before, e := Cursor(cursor)
	if e != nil || limit < 1 || limit > 40 {
		return out, ErrInvalid
	}
	e = s.snapshot(ctx, auth, "", func(tx *sql.Tx) error {
		q := `SELECT ` + reportColumns + ` FROM console_reports WHERE sequence<? AND expires_ms>?`
		args := []any{before, s.now()}
		if !owner {
			q += ` AND scope=?`
			args = append(args, ViewerKey(p))
		}
		q += ` ORDER BY sequence DESC LIMIT ?`
		args = append(args, limit+1)
		rows, err := tx.Query(q, args...)
		if err != nil {
			return err
		}
		items := []Report{}
		for rows.Next() {
			r, e := scanReport(rows)
			if e != nil {
				rows.Close()
				return e
			}
			items = append(items, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(items) > limit {
			items = items[:limit]
			out.NextCursor = strconv.FormatInt(items[len(items)-1].Sequence, 10)
		}
		for i := range items {
			reportVisible(ctx, tx, auth, &items[i])
		}
		out.Items = items
		return nil
	})
	return
}
func (s *Store) Report(ctx context.Context, p identity.Principal, auth Authorize, owner bool, id string) (out Report, e error) {
	if !validID(id) {
		return out, ErrInvalid
	}
	e = s.snapshot(ctx, auth, "", func(tx *sql.Tx) error {
		q := `SELECT ` + reportColumns + ` FROM console_reports WHERE id=? AND expires_ms>?`
		args := []any{id, s.now()}
		if !owner {
			q += ` AND scope=?`
			args = append(args, ViewerKey(p))
		}
		var err error
		out, err = scanReport(tx.QueryRow(q, args...))
		if err != nil {
			return err
		}
		reportVisible(ctx, tx, auth, &out)
		if !out.ContextAvailable {
			return nil
		}
		// Replies are capped per report, so this is a bounded complete history.
		rows, err := tx.Query(`SELECT sequence,time_ms,revision,status,reply,actor_class FROM console_report_events WHERE report_id=? ORDER BY sequence LIMIT 200`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		out.Events = []ReportEvent{}
		for rows.Next() {
			var v ReportEvent
			if err = rows.Scan(&v.Sequence, &v.At, &v.Revision, &v.Status, &v.Reply, &v.ActorClass); err != nil {
				return err
			}
			out.Events = append(out.Events, v)
		}
		return rows.Err()
	})
	return
}
func (s *Store) Triage(ctx context.Context, p identity.Principal, auth Authorize, id string, c Triage) (out Report, e error) {
	if !validID(id) || c.Reply != "" && !SafeText(c.Reply, 4000) {
		return out, ErrInvalid
	}
	switch c.Status {
	case "open", "in-progress", "resolved", "closed":
	default:
		return out, ErrInvalid
	}
	e = s.transaction(ctx, auth, "", func(tx *sql.Tx) error {
		scope := "triage:" + AccountKey(p)
		raw, digest, err := Receipt(tx, scope, c.IdempotencyKey, []any{id, c}, s.now())
		if err != nil {
			return err
		}
		if raw != "" {
			return decodeDocument(raw, &out)
		}
		var owner string
		var revision int64
		err = tx.QueryRow(`SELECT scope,revision FROM console_reports WHERE id=? AND expires_ms>?`, id, s.now()).Scan(&owner, &revision)
		if err != nil {
			return err
		}
		if revision != c.ExpectedRevision {
			return &ConflictError{revision}
		}
		var n int
		if err = tx.QueryRow(`SELECT count(*) FROM console_report_events WHERE report_id=?`, id).Scan(&n); err != nil {
			return err
		}
		if n >= 200 {
			return ErrCapacity
		}
		revision++
		now := s.now()
		if _, err = tx.Exec(`UPDATE console_reports SET status=?,revision=?,updated_ms=? WHERE id=?`, c.Status, revision, now, id); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO console_report_events(report_id,time_ms,revision,status,reply,actor_class) VALUES(?,?,?,?,?,'owner')`, id, now, revision, c.Status, c.Reply); err != nil {
			return err
		}
		if err = Audit(tx, now, AccountKey(p), "feedback.triage", id, revision); err != nil {
			return err
		}
		if err = notifyTx(tx, now, owner, "feedback-updated", id, id+":"+strconv.FormatInt(revision, 10)); err != nil {
			return err
		}
		out, err = scanReport(tx.QueryRow(`SELECT `+reportColumns+` FROM console_reports WHERE id=?`, id))
		if err != nil {
			return err
		}
		return SaveReceipt(tx, scope, c.IdempotencyKey, digest, out, now)
	})
	return
}

type Notification struct {
	ID        string `json:"id"`
	Sequence  int64  `json:"-"`
	CreatedAt int64  `json:"createdAt"`
	ReadAt    *int64 `json:"readAt"`
	Code      string `json:"code"`
	TargetID  string `json:"targetId,omitempty"`
	Message   string `json:"message"`
}
type Inbox struct {
	Page[Notification]
	Unread int `json:"unread"`
}

func notifyTx(tx *sql.Tx, now int64, scope, code, target, dedupe string) error {
	settings, e := readSettings(tx)
	if e != nil {
		return e
	}
	_, e = tx.Exec(`INSERT OR IGNORE INTO console_notifications(id,scope,created_ms,code,target_id,expires_ms,dedupe) VALUES(?,?,?,?,?,?,?)`, identity.Token(), scope, now, code, target, now+days(settings.Effective.NotificationDays), dedupe)
	if e != nil {
		return e
	}
	// Explicit per-inbox rotation; a single noisy profile cannot evict another.
	_, e = tx.Exec(`DELETE FROM console_notifications WHERE scope=? AND sequence NOT IN(SELECT sequence FROM console_notifications WHERE scope=? ORDER BY sequence DESC LIMIT 1000)`, scope, scope)
	return e
}
func (s *Store) Inbox(ctx context.Context, p identity.Principal, auth Authorize, cursor string, limit int) (out Inbox, e error) {
	out.Items = []Notification{}
	before, e := Cursor(cursor)
	if e != nil || limit < 1 || limit > 40 {
		return out, ErrInvalid
	}
	scope := ViewerKey(p)
	e = s.snapshot(ctx, auth, "", func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id,sequence,created_ms,read_ms,code,target_id FROM console_notifications WHERE scope=? AND sequence<? AND expires_ms>? ORDER BY sequence DESC LIMIT ?`, scope, before, s.now(), limit+1)
		if err != nil {
			return err
		}
		for rows.Next() {
			var n Notification
			if err = rows.Scan(&n.ID, &n.Sequence, &n.CreatedAt, &n.ReadAt, &n.Code, &n.TargetID); err != nil {
				rows.Close()
				return err
			}
			n.Message = "Your feedback has an update."
			if n.Code == "feedback-received" {
				n.Message = "New viewer feedback is ready for review."
			}
			out.Items = append(out.Items, n)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			out.NextCursor = strconv.FormatInt(out.Items[len(out.Items)-1].Sequence, 10)
		}
		for i := range out.Items {
			n := &out.Items[i]
			if n.Code == "owner-alert" {
				n.Message = "A server operation needs owner attention."
				var found int
				if p.Authority != "local" || p.Role != "owner" || tx.QueryRow(`SELECT 1 FROM console_alerts WHERE id=?`, n.TargetID).Scan(&found) != nil {
					n.TargetID = ""
				}
				continue
			}
			var item, reportScope string
			err = tx.QueryRow(`SELECT COALESCE((SELECT pid(public_id) FROM catalog_entities WHERE id=console_reports.item_id),''),scope FROM console_reports WHERE id=? AND expires_ms>?`, n.TargetID, s.now()).Scan(&item, &reportScope)
			if err != nil || reportScope != scope && (p.Authority != "local" || p.Role != "owner") || item != "" && auth(ctx, tx, item) != nil {
				n.TargetID = ""
				n.Message = "A previous report has a status update."
			}
		}
		return tx.QueryRow(`SELECT count(*) FROM console_notifications WHERE scope=? AND read_ms IS NULL AND expires_ms>?`, scope, s.now()).Scan(&out.Unread)
	})
	return
}
func (s *Store) ReadNotice(ctx context.Context, p identity.Principal, auth Authorize, id string) error {
	if !validID(id) {
		return ErrInvalid
	}
	return s.transaction(ctx, auth, "", func(tx *sql.Tx) error {
		r, e := tx.Exec(`UPDATE console_notifications SET read_ms=COALESCE(read_ms,?) WHERE id=? AND scope=? AND expires_ms>?`, s.now(), id, ViewerKey(p), s.now())
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
}

// Notify is the local registration seam for other packages. The producer must
// authorize and supply the exact viewer owner in its transaction. No free-form
// media payload or delivery-to-Hosted option exists.
func Notify(tx *sql.Tx, now int64, p identity.Principal, code, target, dedupe string) error {
	if code != "feedback-updated" && code != "feedback-received" {
		return ErrInvalid
	}
	if !validID(target) || len(dedupe) > 200 {
		return ErrInvalid
	}
	return notifyTx(tx, now, ViewerKey(p), code, target, dedupe)
}

// Owner notifications go only to the active Direct Sign-In recovery owner.
func ownerScopes(tx *sql.Tx) ([]string, error) {
	rows, e := tx.Query(`SELECT a.id,a.profile_id FROM accounts a JOIN direct_memberships m ON m.account_id=a.id WHERE m.role='owner' AND m.disabled=0`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	owners := []string{}
	for rows.Next() {
		var account, profile string
		if e = rows.Scan(&account, &profile); e != nil {
			return nil, e
		}
		owners = append(owners, ViewerKey(identity.Principal{Viewer: identity.Viewer{Authority: "local", AccountID: account, ProfileID: profile}}))
	}
	return owners, rows.Err()
}
