package access

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"portico.local/server/internal/identity"
)

// MERGE NOTE (identity workstream). This area needs a device inventory in order
// to publish trust and an approval policy, and the identity workstream owns the
// canonical device record. Until that record lands, access_devices is the
// minimal inventory: an identifier the client sends, the account and profile it
// was last seen under, a name, a platform and a trust state.
//
// When the identity workstream's devices table arrives, merge by keeping
// access_devices' trust, revision and first/last-seen columns and reading the
// name, platform and ownership columns from that table instead — replace the
// SELECT in listDevices and the upsert in Observe with a join, and drop the
// name/platform/account columns here. No wire shape below has to change.

type Device struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	ProfileID string `json:"profileId,omitempty"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	Trust     string `json:"trust"`
	FirstSeen string `json:"firstSeenAt"`
	LastSeen  string `json:"lastSeenAt"`
	Revision  int64  `json:"revision"`
}

type DevicePage struct {
	Page
	// ApprovalRequired mirrors the server policy so a console renders the list
	// and the policy that governs it from one read.
	ApprovalRequired bool     `json:"approvalRequired"`
	Items            []Device `json:"items"`
}

func deviceRow(row interface{ Scan(...any) error }) (Device, error) {
	var d Device
	var first, last int64
	if e := row.Scan(&d.ID, &d.AccountID, &d.ProfileID, &d.Name, &d.Platform, &d.Trust, &first, &last, &d.Revision); e != nil {
		return d, e
	}
	d.FirstSeen = time.UnixMilli(first).UTC().Format(time.RFC3339)
	d.LastSeen = time.UnixMilli(last).UTC().Format(time.RFC3339)
	return d, nil
}

const deviceColumns = `id,account_id,profile_id,name,platform,trust,first_seen_ms,last_seen_ms,revision`

type DeviceQuery struct {
	Cursor string
	Limit  string
	Trust  string
	// Account narrows the page to one account's devices (an account's page lists its own).
	Account string
}

func (s *Store) Devices(ctx context.Context, auth Authorize, q DeviceQuery, approvalRequired bool) (out DevicePage, err error) {
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return out, err
	}
	seen, after, err := decodeCursor(q.Cursor)
	if err != nil {
		return out, err
	}
	switch q.Trust {
	case "", "pending", "approved", "blocked":
	default:
		return out, invalid("trust")
	}
	if len(q.Account) > 128 {
		return out, invalid("account")
	}
	out.Limit, out.Items, out.ApprovalRequired = limit, []Device{}, approvalRequired
	err = s.snapshot(ctx, auth, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT `+deviceColumns+` FROM access_devices WHERE (?='' OR trust=?) AND (?='' OR account_id=?) AND (?=0 OR (last_seen_ms,id)<(?,?)) ORDER BY last_seen_ms DESC,id DESC LIMIT ?`, q.Trust, q.Trust, q.Account, q.Account, seen, seen, after, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			d, e := deviceRow(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, d)
		}
		return rows.Err()
	})
	if err == nil && len(out.Items) > limit {
		last := out.Items[limit-1]
		out.Items = out.Items[:limit]
		t, _ := time.Parse(time.RFC3339, last.LastSeen)
		out.NextCursor = encodeCursor(t.UnixMilli(), last.ID)
	}
	return
}

type TrustChange struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	OperationID      string `json:"operationId"`
	Trust            string `json:"trust"`
}

func (s *Store) SetDeviceTrust(ctx context.Context, auth Authorize, id string, c TrustChange) (out Device, err error) {
	if !validID(id) {
		return out, invalid("id")
	}
	if !validOperationID(c.OperationID) {
		return out, invalid("operationId")
	}
	if c.Trust != "pending" && c.Trust != "approved" && c.Trust != "blocked" {
		return out, invalid("trust")
	}
	err = s.transaction(ctx, auth, func(tx *sql.Tx) error {
		scope := "device:" + id
		body, want, e := receipt(tx, scope, c.OperationID, c)
		if e != nil {
			return e
		}
		if body != "" {
			return json.Unmarshal([]byte(body), &out)
		}
		var revision int64
		var account string
		if e = tx.QueryRowContext(ctx, `SELECT revision,account_id FROM access_devices WHERE id=?`, id).Scan(&revision, &account); e != nil {
			return e
		}
		if revision != c.ExpectedRevision {
			return &ConflictError{revision}
		}
		if _, e = tx.ExecContext(ctx, `UPDATE access_devices SET trust=?,revision=revision+1 WHERE id=?`, c.Trust, id); e != nil {
			return e
		}
		if c.Trust == "blocked" {
			// Blocking a device must stop what it is already doing, not only what
			// it does next: its account's live leases end with its trust.
			if _, e = tx.ExecContext(ctx, `UPDATE playback_sessions SET state='stopped' WHERE account_id=? AND state NOT IN('stopped','ended','failed')`, account); e != nil {
				return e
			}
		}
		if out, e = deviceRow(tx.QueryRowContext(ctx, `SELECT `+deviceColumns+` FROM access_devices WHERE id=?`, id)); e != nil {
			return e
		}
		return saveReceipt(tx, scope, c.OperationID, want, out, s.now())
	})
	return
}

// Observe records a device sighting. It is called from the admission path with
// whatever identifier the client presented; an unknown identifier is recorded as
// pending, so an owner who turns approval on sees real devices to approve rather
// than an empty list.
func (s *Store) Observe(ctx context.Context, tx *sql.Tx, p identity.Principal, id, name, platform string) error {
	if s == nil || tx == nil || !validID(id) {
		return nil
	}
	if !safeText(name, 120) {
		name = ""
	}
	if !safeText(platform, 60) {
		platform = ""
	}
	now := s.now()
	_, err := tx.ExecContext(ctx, `INSERT INTO access_devices(id,account_id,profile_id,name,platform,first_seen_ms,last_seen_ms) VALUES(?,?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET account_id=excluded.account_id,profile_id=excluded.profile_id,last_seen_ms=excluded.last_seen_ms,
 name=CASE WHEN excluded.name='' THEN access_devices.name ELSE excluded.name END,
 platform=CASE WHEN excluded.platform='' THEN access_devices.platform ELSE excluded.platform END`,
		id, p.AccountID, p.ProfileID, strings.TrimSpace(name), strings.TrimSpace(platform), now, now)
	return err
}

// DeviceTrust reads a device's trust state inside an existing transaction.
// An identifier with no row is "pending".
func DeviceTrust(ctx context.Context, tx *sql.Tx, id string) (string, error) {
	var trust string
	e := tx.QueryRowContext(ctx, `SELECT trust FROM access_devices WHERE id=?`, id).Scan(&trust)
	if e == sql.ErrNoRows {
		return "pending", nil
	}
	return trust, e
}
