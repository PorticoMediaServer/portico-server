package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/server/internal/identity"
)

// Root §23 (25 Oct): an explicit Send app log may upload a closed, code-only
// journal. This is its own producer; the feedback report path stays a
// server-reference mechanism and never carries these bytes.

// appReportCodes is the closed reviewed vocabulary. Neither a regex nor a
// free-form string is accepted; adding a code is a reviewed source change.
var appReportCodes = map[AppReportCode]struct{}{
	"audio_transition_unavailable": {},
	"audio_edge_late":              {},
	"audio_processing_unavailable": {},
	"equalizer_filter_unavailable": {},
	"audio_autoplay_blocked":       {},
}

const (
	appReportMaxEntries   = 200
	appReportMaxBodyBytes = 16384
	appReportRetentionMs  = 7 * 24 * 60 * 60 * 1000
	appReportPastMs       = 24 * 60 * 60 * 1000
	appReportSkewMs       = 5 * 60 * 1000
	appReportPerHour      = 10
	appReportStoredMax    = 50
)

type AppReportLevel string
type AppReportCode string

func (AppReportLevel) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []any{"info", "warn", "error"}}
}

func (AppReportCode) JSONSchema() map[string]any {
	codes := make([]string, 0, len(appReportCodes))
	for code := range appReportCodes {
		codes = append(codes, string(code))
	}
	sort.Strings(codes)
	values := make([]any, 0, len(codes))
	for _, code := range codes {
		values = append(values, code)
	}
	return map[string]any{"type": "string", "enum": values}
}

func (AppReportEntry) JSONSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": []any{"atMs", "level", "code"},
		"properties": map[string]any{"atMs": map[string]any{"type": "integer"}, "level": AppReportLevel("").JSONSchema(), "code": AppReportCode("").JSONSchema()}}
}

func (AppReportRequest) JSONSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": []any{"entries"},
		"properties": map[string]any{"entries": map[string]any{"type": "array", "minItems": 1, "maxItems": 200, "items": AppReportEntry{}.JSONSchema()}}}
}

type AppReportEntry struct {
	AtMs  int64          `json:"atMs"`
	Level AppReportLevel `json:"level"`
	Code  AppReportCode  `json:"code"`
}

type AppReportRequest struct {
	Entries []AppReportEntry `json:"entries"`
}

type AppReportReceipt struct {
	ID         string           `json:"id"`
	CreatedMs  int64            `json:"createdMs"`
	ExpiresMs  int64            `json:"expiresMs"`
	EntryCount int              `json:"entryCount"`
	Entries    []AppReportEntry `json:"entries,omitempty"`
}

func appReportIntent(in AppReportRequest) string {
	encoded, _ := json.Marshal(in)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func validAppOperationKey(value string) bool {
	if len(value) < 16 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}

// PurgeExpiredAppReports deletes expired payloads and receipts independent of any
// request, so an idle server never keeps them past their expiry.
func PurgeExpiredAppReports(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM app_reports WHERE expires_ms<=?`, time.Now().UnixMilli())
	return err
}

func registerAppReports(registry *apikit.Registry, d Dependencies) {
	failures := []string{"invalid_request", "unauthorized", "not_permitted", "revision_mismatch", "internal"}
	v1Route(registry, apikit.Metadata{ID: "post_app_reports", Method: "POST", Path: "/v1/app-reports", Summary: "Submit the current context’s explicit code-only app log", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: appReportMaxBodyBytes, Status: 201, Errors: failures}, func(ctx context.Context, r *http.Request, in AppReportRequest) (AppReportReceipt, error) {
		p, err := d.principal(r)
		if err != nil {
			return AppReportReceipt{}, err
		}
		operation := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if !validAppOperationKey(operation) {
			return AppReportReceipt{}, &apierror.Error{Code: "invalid_request"}
		}
		if len(in.Entries) == 0 || len(in.Entries) > appReportMaxEntries {
			return AppReportReceipt{}, &apierror.Error{Code: "invalid_request"}
		}
		now := time.Now().UnixMilli()
		intent := appReportIntent(in)
		tx, err := d.DB.BeginTx(ctx, nil)
		if err != nil {
			return AppReportReceipt{}, err
		}
		defer tx.Rollback()
		// Fresh exact session/actor/device authority inside the accepting transaction.
		actor, err := d.tierAuthorityTx(ctx, tx, p, identity.TierMember)
		if err != nil {
			return AppReportReceipt{}, err
		}
		if actor.DeviceID == "" {
			return AppReportReceipt{}, &apierror.Error{Code: "not_permitted"}
		}
		// The original intent is served before any freshness or rate rule, so its own
		// receipt replays for the full seven days even after its entries are older.
		var existingID, existingIntent string
		err = tx.QueryRowContext(ctx, `SELECT id,intent_sha FROM app_reports WHERE authority=? AND account_id=? AND profile_id=? AND device_id=? AND operation_id=?`,
			actor.Authority, actor.AccountID, actor.ProfileID, actor.DeviceID, operation).Scan(&existingID, &existingIntent)
		if err == nil {
			if existingIntent != intent {
				return AppReportReceipt{}, &apierror.Error{Code: "revision_mismatch"}
			}
			// An expired receipt is purged and refused, never replayed.
			if _, e := tx.ExecContext(ctx, `DELETE FROM app_reports WHERE id=? AND expires_ms<=?`, existingID, now); e != nil {
				return AppReportReceipt{}, e
			}
			receipt, e := loadAppReport(ctx, tx, existingID)
			if e == sql.ErrNoRows {
				return AppReportReceipt{}, &apierror.Error{Code: "not_permitted"}
			}
			return receipt, e
		}
		if err != sql.ErrNoRows {
			return AppReportReceipt{}, err
		}
		for _, entry := range in.Entries {
			if entry.Level != "info" && entry.Level != "warn" && entry.Level != "error" {
				return AppReportReceipt{}, &apierror.Error{Code: "invalid_request"}
			}
			if _, ok := appReportCodes[entry.Code]; !ok {
				return AppReportReceipt{}, &apierror.Error{Code: "invalid_request"}
			}
			// A client timestamp is untrusted: prior 24 hours and at most 5 minutes ahead.
			if entry.AtMs < now-appReportPastMs || entry.AtMs > now+appReportSkewMs {
				return AppReportReceipt{}, &apierror.Error{Code: "invalid_request"}
			}
		}
		var recent, stored int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app_reports WHERE authority=? AND account_id=? AND device_id=? AND created_ms>?`,
			actor.Authority, actor.AccountID, actor.DeviceID, now-60*60*1000).Scan(&recent); err != nil {
			return AppReportReceipt{}, err
		}
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM app_reports WHERE authority=? AND account_id=? AND expires_ms>?`,
			actor.Authority, actor.AccountID, now).Scan(&stored); err != nil {
			return AppReportReceipt{}, err
		}
		if recent >= appReportPerHour || stored >= appReportStoredMax {
			return AppReportReceipt{}, &apierror.Error{Code: "not_permitted"}
		}
		receipt := AppReportReceipt{ID: identity.Token(), CreatedMs: now, ExpiresMs: now + appReportRetentionMs, EntryCount: len(in.Entries), Entries: in.Entries}
		if _, err = tx.ExecContext(ctx, `INSERT INTO app_reports(id,authority,account_id,profile_id,device_id,created_ms,expires_ms,entry_count,operation_id,intent_sha) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			receipt.ID, actor.Authority, actor.AccountID, actor.ProfileID, actor.DeviceID, receipt.CreatedMs, receipt.ExpiresMs, receipt.EntryCount, operation, intent); err != nil {
			return AppReportReceipt{}, err
		}
		for position, entry := range in.Entries {
			if _, err = tx.ExecContext(ctx, `INSERT INTO app_report_entries(report_id,position,at_ms,level,code) VALUES(?,?,?,?,?)`,
				receipt.ID, position, entry.AtMs, entry.Level, entry.Code); err != nil {
				return AppReportReceipt{}, err
			}
		}
		// Expired payloads are deleted with their receipts on every accepted write too.
		if _, err = tx.ExecContext(ctx, `DELETE FROM app_reports WHERE expires_ms<=?`, now); err != nil {
			return AppReportReceipt{}, err
		}
		if err = tx.Commit(); err != nil {
			return AppReportReceipt{}, err
		}
		return receipt, nil
	})
	v1Route(registry, apikit.Metadata{ID: "get_app_reports", Method: "GET", Path: "/v1/app-reports", Summary: "Read the owner’s app-log receipts", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.PageSized, Status: 200, Errors: failures}, func(ctx context.Context, r *http.Request, _ noBody) ([]AppReportReceipt, error) {
		p, err := d.principal(r)
		if err != nil {
			return nil, err
		}
		tx, err := d.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		// Canonical local-owner admission, checked freshly in this read transaction.
		if _, err = d.tierAuthorityTx(ctx, tx, p, identity.TierOwner); err != nil {
			return nil, err
		}
		now := time.Now().UnixMilli()
		rows, err := tx.QueryContext(ctx, `SELECT id FROM app_reports WHERE expires_ms>? ORDER BY created_ms DESC LIMIT ?`, now, appReportStoredMax)
		if err != nil {
			return nil, err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, err
		}
		out := make([]AppReportReceipt, 0, len(ids))
		for _, id := range ids {
			receipt, e := loadAppReport(ctx, tx, id)
			if e != nil {
				return nil, e
			}
			out = append(out, receipt)
		}
		// A fresh owner read is still a committed authority: refuse after it, never
		// deliver rows to a caller whose session changed under the read.
		if _, e := d.tierAuthorityTx(ctx, tx, p, identity.TierOwner); e != nil {
			return nil, e
		}
		return out, nil
	})
}

func loadAppReport(ctx context.Context, tx *sql.Tx, id string) (AppReportReceipt, error) {
	var receipt AppReportReceipt
	if err := tx.QueryRowContext(ctx, `SELECT id,created_ms,expires_ms,entry_count FROM app_reports WHERE id=?`, id).
		Scan(&receipt.ID, &receipt.CreatedMs, &receipt.ExpiresMs, &receipt.EntryCount); err != nil {
		return AppReportReceipt{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT at_ms,level,code FROM app_report_entries WHERE report_id=? ORDER BY position LIMIT ?`, id, appReportMaxEntries)
	if err != nil {
		return AppReportReceipt{}, err
	}
	defer rows.Close()
	receipt.Entries = []AppReportEntry{}
	for rows.Next() {
		var entry AppReportEntry
		if err = rows.Scan(&entry.AtMs, &entry.Level, &entry.Code); err != nil {
			return AppReportReceipt{}, err
		}
		receipt.Entries = append(receipt.Entries, entry)
	}
	return receipt, rows.Err()
}
