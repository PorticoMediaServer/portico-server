package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"time"

	"portico.local/hosted/internal/certificates"
)

// ACMECustody is secret operator backup data, never an HTTP result or a log.
type ACMECustody struct {
	Issuer    string
	Directory string
	Cipher    []byte
	URL       string
	CreatedAt time.Time
}

func (ACMECustody) String() string     { return "[ACME custody redacted]" }
func (a ACMECustody) GoString() string { return a.String() }

func readACMECustody(ctx context.Context, tx *sql.Tx, issuer, directory string, lock bool) (ACMECustody, error) {
	a := ACMECustody{Issuer: issuer}
	query := `SELECT directory_url,key_cipher,account_url,created_at FROM certificate_acme_accounts WHERE issuer=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	err := tx.QueryRowContext(ctx, query, issuer).Scan(&a.Directory, &a.Cipher, &a.URL, &a.CreatedAt)
	if err != nil {
		return ACMECustody{}, err
	}
	if a.Directory != directory {
		clear(a.Cipher)
		return ACMECustody{}, certificates.ErrConfiguration
	}
	return a, nil
}
func sameACMECustody(a, b ACMECustody) bool {
	return a.Issuer == b.Issuer && a.Directory == b.Directory && a.URL == b.URL && a.CreatedAt.Equal(b.CreatedAt) && bytes.Equal(a.Cipher, b.Cipher)
}

// ReadACMECustody is SELECT-only even when inspecting a preserved older database.
func (s *Store) ReadACMECustody(ctx context.Context, issuer, directory string) (ACMECustody, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ACMECustody{}, err
	}
	defer tx.Rollback()
	a, err := readACMECustody(ctx, tx, issuer, directory, false)
	if err != nil {
		return a, err
	}
	if err = tx.Commit(); err != nil {
		clear(a.Cipher)
		return ACMECustody{}, err
	}
	return a, nil
}

func checkACMECandidateOrders(ctx context.Context, tx *sql.Tx, issuer string) error {
	var crossed bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM certificate_orders WHERE issuer=$1 AND (creation_attempted OR provider_url<>'' OR chain IS NOT NULL OR lease_until>clock_timestamp()))`, issuer).Scan(&crossed)
	if err != nil {
		return err
	}
	if crossed {
		return certificates.ErrConflict
	}
	return nil
}

// CheckACMECandidate refuses any used/registered candidate, current work lease or
// provider-order boundary. The final writer repeats all of these predicates.
func (s *Store) CheckACMECandidate(ctx context.Context, want ACMECustody) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := readACMECustody(ctx, tx, want.Issuer, want.Directory, false)
	if err != nil {
		return err
	}
	defer clear(current.Cipher)
	if want.URL != "" || !sameACMECustody(current, want) {
		return certificates.ErrConflict
	}
	if err = checkACMECandidateOrders(ctx, tx, want.Issuer); err != nil {
		return err
	}
	return tx.Commit()
}

// RecoverACMECustody is the narrow operator CAS after workers are stopped,
// encrypted backup and verified existing-account lookups. It changes only the
// candidate's key/URL; all orders, claims, proofs and generations are untouched.
func (s *Store) RecoverACMECustody(ctx context.Context, want, original ACMECustody) error {
	if want.URL != "" || original.URL == "" || want.Issuer != original.Issuer || want.Directory != original.Directory || len(want.Cipher) == 0 || len(original.Cipher) == 0 || bytes.Equal(want.Cipher, original.Cipher) {
		return certificates.ErrConflict
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `LOCK TABLE certificate_orders IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	current, err := readACMECustody(ctx, tx, want.Issuer, want.Directory, true)
	if err != nil {
		return err
	}
	defer clear(current.Cipher)
	if !sameACMECustody(current, want) {
		return certificates.ErrConflict
	}
	if err = checkACMECandidateOrders(ctx, tx, want.Issuer); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE certificate_acme_accounts SET key_cipher=$2,account_url=$3 WHERE issuer=$1 AND directory_url=$4 AND key_cipher=$5 AND account_url='' AND created_at=$6`, want.Issuer, original.Cipher, original.URL, want.Directory, want.Cipher, want.CreatedAt)
	if err = certificateAffected(result, err); err != nil {
		return err
	}
	return tx.Commit()
}
