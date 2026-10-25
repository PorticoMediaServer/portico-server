package postgres

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/security"
	"strconv"
	"strings"
	"time"
)

type RegistrationInput struct {
	Username       string `json:"username"`
	DisplayName    string `json:"displayName"`
	Password       string `json:"password"`
	ContactAddress string `json:"contactAddress"`
	SessionMode    string `json:"sessionMode"`
	RequestID      string `json:"requestId"`
}
type registrationRow struct {
	ID, Username, Name, Address, Mode string
	Password, CodeHash                []byte
	Revision                          int64
	Attempts, Resends                 int
	Expires, CodeExpires              time.Time
	Verified, Completed               sql.NullTime
	Account, Family                   sql.NullString
	Epoch, Generation                 sql.NullInt64
	Receipt                           []byte
	ReceiptExpires                    sql.NullTime
	LastResend                        sql.NullString
}

func registrationView(r registrationRow) identity.Registration {
	status := "contact_pending"
	if r.Completed.Valid {
		status = "completed"
	}
	return identity.Registration{Kind: "registration", ID: r.ID, Status: status, Revision: strconv.FormatInt(r.Revision, 10), MaskedAddress: maskedContact(r.Address), ExpiresAt: r.Expires.UTC(), ContactVerified: r.Verified.Valid}
}
func (s *AccountSecurity) registration(ctx context.Context, tx *sql.Tx, id, binding string) (registrationRow, error) {
	var r registrationRow
	e := tx.QueryRowContext(ctx, `SELECT id,username,display_name,address,session_mode,password_hash,code_hash,revision,attempts,resend_count,expires_at,code_expires_at,verified_at,completed_at,account_id,family_id,account_epoch,family_generation,receipt,receipt_expires_at,last_resend_request FROM account_registrations WHERE id=$1 AND binding_hash=$2 AND cancelled_at IS NULL AND expires_at>clock_timestamp() FOR UPDATE`, id, security.Hash(binding)).Scan(&r.ID, &r.Username, &r.Name, &r.Address, &r.Mode, &r.Password, &r.CodeHash, &r.Revision, &r.Attempts, &r.Resends, &r.Expires, &r.CodeExpires, &r.Verified, &r.Completed, &r.Account, &r.Family, &r.Epoch, &r.Generation, &r.Receipt, &r.ReceiptExpires, &r.LastResend)
	if errors.Is(e, sql.ErrNoRows) {
		e = identity.ErrChallenge
	}
	if e == nil {
		var now time.Time
		if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e == nil && !r.Expires.After(now) {
			e = identity.ErrChallenge
		}
	}
	return r, e
}
func (s *AccountSecurity) CreateRegistration(ctx context.Context, binding string, q RegistrationInput) (identity.Registration, error) {
	var out identity.Registration
	if !s.MailEnabled {
		return out, identity.ErrMailUnavailable
	}
	if len(binding) != 43 || !identity.ValidMode(q.SessionMode) || len(q.RequestID) < 16 || len(q.RequestID) > 128 {
		return out, identity.ErrInvalid
	}
	q.Username = strings.ToLower(strings.TrimSpace(q.Username))
	q.DisplayName = strings.TrimSpace(q.DisplayName)
	if q.DisplayName == "" {
		q.DisplayName = q.Username
	}
	var e error
	q.ContactAddress, e = NormalizeContact(q.ContactAddress)
	if e != nil || !identity.ValidUsername(q.Username) || len(q.DisplayName) > 120 || !identity.ValidPassword(q.Password) {
		return out, identity.ErrInvalid
	}
	digest := s.requestHash(q)
	var existing string
	var stored []byte
	e = s.Store.DB.QueryRowContext(ctx, `SELECT id,request_hash FROM account_registrations WHERE binding_hash=$1 AND request_id=$2`, security.Hash(binding), q.RequestID).Scan(&existing, &stored)
	if e == nil {
		if !equalSecret(stored, digest) {
			return out, identity.ErrConflict
		}
		return s.RegistrationView(ctx, existing, binding)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	if e = s.Limit(ctx, "registration-email", strings.ToLower(q.ContactAddress), 5, time.Hour); e != nil {
		return out, e
	}
	if e = s.Limit(ctx, "registration-name", q.Username, 10, time.Hour); e != nil {
		return out, e
	}
	// Public handle only, after the existing source/address/name budgets. This
	// is advisory: the proof-backed unique INSERT below remains authoritative.
	var unavailable bool
	if e = s.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE username=$1)`, q.Username).Scan(&unavailable); e != nil {
		return out, e
	}
	if unavailable {
		// A matching request may have completed since the first replay read.
		// Preserve that receipt before reporting a fresh-name collision.
		e = s.Store.DB.QueryRowContext(ctx, `SELECT id,request_hash FROM account_registrations WHERE binding_hash=$1 AND request_id=$2`, security.Hash(binding), q.RequestID).Scan(&existing, &stored)
		if e == nil {
			if !equalSecret(stored, digest) {
				return out, identity.ErrConflict
			}
			return s.RegistrationView(ctx, existing, binding)
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return out, e
		}
		return out, identity.ErrUsernameUnavailable
	}
	hash, e := s.Identity.HashPassword(ctx, q.Password)
	if e != nil {
		return out, e
	}
	code, e := identity.NewEmailCode()
	if e != nil {
		return out, e
	}
	id := security.RandomID("reg_")
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	var expires, codeExpires time.Time
	// Unique binding/request IDs serialize concurrent first submissions. A loser
	// re-reads the winner, never changes its password or emits a second message.
	e = tx.QueryRowContext(ctx, `INSERT INTO account_registrations(id,binding_hash,request_id,request_hash,session_mode,username,display_name,address,password_hash,code_hash,expires_at,code_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,statement_timestamp()+interval '30 minutes',statement_timestamp()+interval '10 minutes') ON CONFLICT(binding_hash,request_id) DO NOTHING RETURNING expires_at,code_expires_at`, id, security.Hash(binding), q.RequestID, digest, q.SessionMode, q.Username, q.DisplayName, q.ContactAddress, hash, s.digest("registration-code", id+":1", code)).Scan(&expires, &codeExpires)
	if errors.Is(e, sql.ErrNoRows) {
		tx.Rollback()
		return s.CreateRegistration(ctx, binding, q)
	}
	if e != nil {
		return out, e
	}
	if e = EnqueueAccountMailTx(ctx, tx, s.Keys, mailoutbox.Intent{ID: security.RandomID("mail_"), Kind: mailoutbox.Registration, ResourceID: id, Revision: 1, ExpiresAt: codeExpires, Payload: mailoutbox.Payload{To: q.ContactAddress, Code: code}}); e != nil {
		return out, e
	}
	out = registrationView(registrationRow{ID: id, Address: q.ContactAddress, Revision: 1, Expires: expires})
	return out, tx.Commit()
}
func (s *AccountSecurity) RegistrationView(ctx context.Context, id, binding string) (identity.Registration, error) {
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return identity.Registration{}, e
	}
	defer tx.Rollback()
	r, e := s.registration(ctx, tx, id, binding)
	if e != nil {
		return identity.Registration{}, e
	}
	return registrationView(r), tx.Commit()
}
func (s *AccountSecurity) CurrentRegistration(ctx context.Context, binding string) (*identity.Registration, error) {
	var id string
	e := s.Store.DB.QueryRowContext(ctx, `SELECT id FROM account_registrations WHERE binding_hash=$1 AND cancelled_at IS NULL AND expires_at>clock_timestamp() AND (completed_at IS NULL OR receipt_expires_at>clock_timestamp()) ORDER BY created_at DESC LIMIT 1`, security.Hash(binding)).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	v, e := s.RegistrationView(ctx, id, binding)
	return &v, e
}
func (s *AccountSecurity) VerifyRegistration(ctx context.Context, id, binding, code, username, name string) (identity.Session, error) {
	var out identity.Session
	if e := s.Limit(ctx, "registration-code", id, 12, 15*time.Minute); e != nil {
		return out, e
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	r, e := s.registration(ctx, tx, id, binding)
	if e != nil {
		return out, e
	}
	if r.Completed.Valid {
		var now time.Time
		if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
			return out, e
		}
		if !r.ReceiptExpires.Valid || !r.ReceiptExpires.Time.After(now) {
			return out, identity.ErrChallenge
		}
		out, e = s.sessionReceipt(ctx, tx, r.Receipt, "registration:"+id, r.Account.String, r.Family.String, r.Epoch.Int64, r.Generation.Int64, r.Mode)
		if e != nil {
			return out, e
		}
		return out, tx.Commit()
	}
	if !r.Verified.Valid {
		var now time.Time
		if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
			return out, e
		}
		if r.Attempts >= 5 || !r.CodeExpires.After(now) {
			return out, identity.ErrChallenge
		}
		if !equalSecret(r.CodeHash, s.digest("registration-code", id+":"+strconv.FormatInt(r.Revision, 10), strings.TrimSpace(code))) {
			if _, e = tx.ExecContext(ctx, `UPDATE account_registrations SET attempts=attempts+1 WHERE id=$1`, id); e != nil {
				return out, e
			}
			if e = tx.Commit(); e != nil {
				return out, e
			}
			return out, identity.ErrChallenge
		}
		if _, e = tx.ExecContext(ctx, `UPDATE account_registrations SET verified_at=clock_timestamp(),code_hash=NULL WHERE id=$1`, id); e != nil {
			return out, e
		}
	}
	if username != "" {
		r.Username = strings.ToLower(strings.TrimSpace(username))
	}
	if name != "" {
		r.Name = strings.TrimSpace(name)
	}
	if !identity.ValidUsername(r.Username) || len(r.Name) == 0 || len(r.Name) > 120 {
		return out, identity.ErrInvalid
	}
	a := identity.Account{ID: security.RandomID("acct_"), Username: r.Username, DisplayName: r.Name}
	var inserted string
	e = tx.QueryRowContext(ctx, `INSERT INTO accounts(id,username,display_name,password_hash) VALUES($1,$2,$3,$4) ON CONFLICT(username) DO NOTHING RETURNING id`, a.ID, a.Username, a.DisplayName, r.Password).Scan(&inserted)
	if errors.Is(e, sql.ErrNoRows) {
		if e = tx.Commit(); e != nil {
			return out, e
		}
		return out, identity.ErrUsernameUnavailable
	}
	if e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO account_contact_emails(id,account_id,address,verified_at,source,is_primary) VALUES($1,$2,$3,clock_timestamp(),'challenge',true)`, security.RandomID("contact_"), a.ID, r.Address); e != nil {
		return out, e
	}
	out, e = s.sessionTx(ctx, tx, a, r.Mode)
	if e != nil {
		return out, e
	}
	cipher, e := s.sealResult(out, "registration:"+id)
	if e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_registrations SET completed_at=clock_timestamp(),username=$2,display_name=$3,password_hash=NULL,code_hash=NULL,account_id=$4,family_id=$5,account_epoch=0,family_generation=0,receipt=$6,receipt_expires_at=LEAST(expires_at,statement_timestamp()+interval '5 minutes') WHERE id=$1`, id, a.Username, a.DisplayName, a.ID, out.FamilyID, cipher); e != nil {
		return out, e
	}
	if e = audit(ctx, tx, a.ID, "account.verified_registration", a.ID, 0); e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func (s *AccountSecurity) ResendRegistration(ctx context.Context, id, binding, requestID string) (identity.Registration, error) {
	var out identity.Registration
	if !s.MailEnabled {
		return out, identity.ErrMailUnavailable
	}
	if len(requestID) < 16 || len(requestID) > 128 {
		return out, identity.ErrInvalid
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	r, e := s.registration(ctx, tx, id, binding)
	if e != nil {
		return out, e
	}
	if r.LastResend.String == requestID || r.Verified.Valid || r.Completed.Valid {
		return registrationView(r), tx.Commit()
	}
	if r.Resends >= 5 {
		return out, identity.ErrRateLimited
	}
	code, e := identity.NewEmailCode()
	if e != nil {
		return out, e
	}
	r.Revision++
	e = tx.QueryRowContext(ctx, `UPDATE account_registrations SET code_hash=$2,revision=$3,attempts=0,resend_count=resend_count+1,last_resend_request=$4,code_expires_at=LEAST(expires_at,statement_timestamp()+interval '10 minutes') WHERE id=$1 RETURNING code_expires_at`, id, s.digest("registration-code", id+":"+strconv.FormatInt(r.Revision, 10), code), r.Revision, requestID).Scan(&r.CodeExpires)
	if e != nil {
		return out, e
	}
	if e = EnqueueAccountMailTx(ctx, tx, s.Keys, mailoutbox.Intent{ID: security.RandomID("mail_"), Kind: mailoutbox.Registration, ResourceID: id, Revision: r.Revision, ExpiresAt: r.CodeExpires, Payload: mailoutbox.Payload{To: r.Address, Code: code}}); e != nil {
		return out, e
	}
	return registrationView(r), tx.Commit()
}
func (s *AccountSecurity) CancelRegistration(ctx context.Context, binding string) error {
	_, e := s.Store.DB.ExecContext(ctx, `UPDATE account_registrations SET cancelled_at=clock_timestamp(),password_hash=NULL,code_hash=NULL,receipt=NULL WHERE binding_hash=$1 AND cancelled_at IS NULL`, security.Hash(binding))
	return e
}
