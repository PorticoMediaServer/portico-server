package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/security"
	"time"
)

// AccountExportDocument is the downloadable account export. Each section is a
// JSON array of that table's explicit row projection (to_jsonb of the row), so
// its element shape follows the section's SELECT list. Fields are in JSON key
// order.
type AccountExportDocument struct {
	Account         json.RawMessage `json:"account"`
	Contacts        json.RawMessage `json:"contacts"`
	CreatedAt       time.Time       `json:"createdAt"`
	Devices         json.RawMessage `json:"devices"`
	Format          string          `json:"format"`
	Invitations     json.RawMessage `json:"invitations"`
	LinkedProviders json.RawMessage `json:"linkedProviders"`
	MediaData       string          `json:"mediaData"`
	Memberships     json.RawMessage `json:"memberships"`
	Profiles        json.RawMessage `json:"profiles"`
	SecurityAudit   json.RawMessage `json:"securityAudit"`
	Sessions        json.RawMessage `json:"sessions"`
}

// Explicit projections exclude credentials, provider subjects, relay addresses
// and other accounts' contacts. Claim/finalize writes are short; the potentially
// large repeatable-read snapshot uses a dedicated background connection.
func (s *AccountSecurity) ProcessExport(ctx context.Context) error {
	claim, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer claim.Rollback()
	var id, account string
	var expiry time.Time
	err = claim.QueryRowContext(ctx, `SELECT id,account_id,expires_at FROM account_exports WHERE expires_at>clock_timestamp() AND (state='pending' OR state='processing' AND lease_until<clock_timestamp()) ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id, &account, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	lease := security.RandomID("export_lease_")
	if _, err = claim.ExecContext(ctx, `UPDATE account_exports SET state='processing',lease_id=$2,lease_until=clock_timestamp()+interval '2 minutes' WHERE id=$1`, id, lease); err != nil {
		return err
	}
	if err = claim.Commit(); err != nil {
		return err
	}

	background, err := s.Store.backgroundReadDB()
	if err != nil {
		return err
	}
	defer background.Close()
	read, err := background.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return err
	}
	defer read.Rollback()
	doc := AccountExportDocument{Format: "portico.account-export.v1", CreatedAt: time.Now().UTC(), MediaData: "Hosted stores no media or playback history. Request personal media data from each server listed in memberships."}
	sections := map[*json.RawMessage]string{
		&doc.Account:         `SELECT id,username,display_name,created_at FROM accounts WHERE id=$1`,
		&doc.Contacts:        `SELECT CASE WHEN private_relay THEN '[private relay]' ELSE address END AS address,is_primary,verified_at FROM account_contact_emails WHERE account_id=$1`,
		&doc.LinkedProviders: `SELECT issuer,created_at,revoked_at FROM account_external_identities WHERE account_id=$1`,
		&doc.Profiles:        `SELECT id,name,created_at,deleted_at FROM profiles WHERE account_id=$1`,
		&doc.Memberships:     `SELECT server_id,role,updated_at FROM server_members WHERE account_id=$1`,
		&doc.Invitations:     `SELECT id,server_id,status,expires_at FROM invitations WHERE owner_id=$1 OR recipient_id=$1`,
		&doc.Devices:         `SELECT id,name,platform,app,app_version,created_at,last_seen_at,revoked_at FROM hosted_devices WHERE account_id=$1`,
		&doc.Sessions:        `SELECT created_at,absolute_expires_at,idle_expires_at,revoked_at FROM session_families WHERE account_id=$1`,
		&doc.SecurityAudit:   `SELECT action,target_id,created_at FROM security_audit WHERE actor_id=$1 OR target_id=$1`,
	}
	for field, q := range sections {
		var raw []byte
		if err = read.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(r)),'[]'::jsonb) FROM (`+q+`) r`, account).Scan(&raw); err != nil {
			return err
		}
		*field = json.RawMessage(raw)
	}
	if err = read.Commit(); err != nil {
		return err
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	cipher, err := s.Keys.Seal(string(raw), "account-export:"+id)
	if err != nil {
		return err
	}
	finish, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer finish.Rollback()
	result, err := finish.ExecContext(ctx, `UPDATE account_exports SET state='ready',ciphertext=$3,lease_id=NULL,lease_until=NULL WHERE id=$1 AND lease_id=$2 AND state='processing' AND expires_at>clock_timestamp()`, id, lease, cipher)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return nil
	}
	// The export is announced in the account's in-app feed, not by email.
	if err = notifyAccountTx(ctx, finish, "export_"+id, account, "", "account_export_ready", map[string]string{"exportId": id}); err != nil {
		return err
	}
	return finish.Commit()
}

// ExportPending reports whether an export is waiting for the worker.
func (s *AccountSecurity) ExportPending(ctx context.Context) (bool, error) {
	var pending bool
	err := s.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_exports WHERE expires_at>clock_timestamp() AND (state='pending' OR state='processing' AND lease_until<clock_timestamp()))`).Scan(&pending)
	return pending, err
}
