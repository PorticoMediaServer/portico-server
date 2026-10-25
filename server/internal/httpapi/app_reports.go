package httpapi

import (
	"context"
	"net/http"
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
var appReportCodes = map[string]struct{}{
	"audio_transition_unavailable":  {},
	"audio_edge_late":               {},
	"audio_processing_unavailable":  {},
	"equalizer_filter_unavailable":  {},
	"audio_autoplay_blocked":        {},
}

const (
	appReportMaxEntries   = 200
	appReportMaxBodyBytes = 16384
	appReportRetentionMs  = 7 * 24 * 60 * 60 * 1000
	appReportPastMs       = 24 * 60 * 60 * 1000
	appReportSkewMs       = 5 * 60 * 1000
)

type AppReportEntry struct {
	AtMs  int64  `json:"atMs"`
	Level string `json:"level"`
	Code  string `json:"code"`
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

func registerAppReports(registry *apikit.Registry, d Dependencies) {
	failures := []string{"invalid_request", "unauthorized", "not_permitted", "internal"}
	v1Route(registry, apikit.Metadata{ID: "post_app_reports", Method: "POST", Path: "/v1/app-reports", Summary: "Submit the current context’s explicit code-only app log", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: appReportMaxBodyBytes, Status: 201, Errors: failures}, func(ctx context.Context, r *http.Request, in AppReportRequest) (AppReportReceipt, error) {
		p, err := d.principal(r)
		if err != nil {
			return AppReportReceipt{}, err
		}
		if len(in.Entries) == 0 || len(in.Entries) > appReportMaxEntries {
			return AppReportReceipt{}, &apierror.Error{Code: "invalid_request"}
		}
		now := time.Now().UnixMilli()
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
		receipt := AppReportReceipt{ID: identity.Token(), CreatedMs: now, ExpiresMs: now + appReportRetentionMs, EntryCount: len(in.Entries)}
		tx, err := d.DB.BeginTx(ctx, nil)
		if err != nil {
			return AppReportReceipt{}, err
		}
		defer tx.Rollback()
		// Actor, server, account, profile and device come from authenticated authority.
		if _, err = tx.ExecContext(ctx, `INSERT INTO app_reports(id,authority,account_id,profile_id,device_id,created_ms,expires_ms,entry_count) VALUES(?,?,?,?,?,?,?,?)`,
			receipt.ID, p.Authority, p.AccountID, p.ProfileID, p.DeviceID, receipt.CreatedMs, receipt.ExpiresMs, receipt.EntryCount); err != nil {
			return AppReportReceipt{}, err
		}
		for position, entry := range in.Entries {
			if _, err = tx.ExecContext(ctx, `INSERT INTO app_report_entries(report_id,position,at_ms,level,code) VALUES(?,?,?,?,?)`,
				receipt.ID, position, entry.AtMs, entry.Level, entry.Code); err != nil {
				return AppReportReceipt{}, err
			}
		}
		// The payload is kept for at most seven days, and an expired receipt is deleted
		// with its entries by the same read.
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
		if p.Authority != "local" || !identity.Grants(p.Role, identity.TierOwner) {
			return nil, &apierror.Error{Code: "not_permitted"}
		}
		now := time.Now().UnixMilli()
		if _, err = d.DB.ExecContext(ctx, `DELETE FROM app_reports WHERE expires_ms<=?`, now); err != nil {
			return nil, err
		}
		rows, err := d.DB.QueryContext(ctx, `SELECT id,created_ms,expires_ms,entry_count FROM app_reports WHERE authority='local' AND expires_ms>? ORDER BY created_ms DESC LIMIT 50`, now)
		if err != nil {
			return nil, err
		}
		out := []AppReportReceipt{}
		for rows.Next() {
			var receipt AppReportReceipt
			if err = rows.Scan(&receipt.ID, &receipt.CreatedMs, &receipt.ExpiresMs, &receipt.EntryCount); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, receipt)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, err
		}
		for index := range out {
			entries, e := d.appReportEntries(ctx, out[index].ID)
			if e != nil {
				return nil, e
			}
			out[index].Entries = entries
		}
		return out, nil
	})
}

func (d Dependencies) appReportEntries(ctx context.Context, id string) ([]AppReportEntry, error) {
	rows, err := d.DB.QueryContext(ctx, `SELECT at_ms,level,code FROM app_report_entries WHERE report_id=? ORDER BY position LIMIT ?`, id, appReportMaxEntries)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AppReportEntry{}
	for rows.Next() {
		var entry AppReportEntry
		if err = rows.Scan(&entry.AtMs, &entry.Level, &entry.Code); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}
