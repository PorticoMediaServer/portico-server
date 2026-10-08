package postgres

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const observabilitySchema = `CREATE TABLE hosted_readiness(singleton boolean PRIMARY KEY CHECK(singleton),checked_at timestamptz NOT NULL);CREATE TABLE ct_observations(fingerprint text PRIMARY KEY,namespace text NOT NULL,known boolean NOT NULL,observed_at timestamptz NOT NULL DEFAULT clock_timestamp());CREATE INDEX ct_unknown ON ct_observations(observed_at) WHERE NOT known;`

func (s *Store) Ready(ctx context.Context) error {
	if !s.Notifications().Connected() {
		return errors.New("notification listener unavailable")
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO hosted_readiness VALUES(true,clock_timestamp()) ON CONFLICT(singleton) DO UPDATE SET checked_at=excluded.checked_at`)
	return e
}

// The recovery-point marker (A17, A95). A restore drill reads it from the
// restored copy; its age when the restore began bounds the data lost. It is
// written every RPOMarkerInterval while the database has other writes (the
// WAL position moved since the last marker; WAL is being archived anyway), and
// not at all while it is idle, so an idle primary forces no WAL archive.
const (
	RPOMarkerCheck    = 30 * time.Second
	RPOMarkerInterval = 30 * time.Second
)

// RPOMarker remembers the last marker; Tick is called every RPOMarkerCheck.
type RPOMarker struct {
	lastLSN string
	last    time.Time
}

// Tick writes a marker if there were writes since the last one and the last
// one is at least RPOMarkerInterval old. It reports whether it wrote.
func (m *RPOMarker) Tick(ctx context.Context, s *Store, now time.Time) (bool, error) {
	var lsn string
	if e := s.DB.QueryRowContext(ctx, `SELECT pg_current_wal_lsn()::text`).Scan(&lsn); e != nil {
		return false, e
	}
	if lsn == m.lastLSN || !m.last.IsZero() && now.Sub(m.last) < RPOMarkerInterval {
		return false, nil
	}
	if e := s.MarkRPO(ctx); e != nil {
		return false, e
	}
	if e := s.DB.QueryRowContext(ctx, `SELECT pg_current_wal_lsn()::text`).Scan(&m.lastLSN); e != nil {
		return true, e
	}
	m.last = now
	return true, nil
}

// MarkRPO commits the current time as the recovery-point marker.
func (s *Store) MarkRPO(ctx context.Context) error {
	_, e := s.DB.ExecContext(ctx, `INSERT INTO hosted_readiness VALUES(true,clock_timestamp()) ON CONFLICT(singleton) DO UPDATE SET checked_at=excluded.checked_at`)
	return e
}
func (s *Store) Metrics(ctx context.Context, w io.Writer) error {
	stats := s.DB.Stats()
	fmt.Fprintf(w, "portico_postgres_pool_wait_total %d\nportico_postgres_pool_wait_seconds_total %g\nportico_postgres_pool_in_use %d\n", stats.WaitCount, stats.WaitDuration.Seconds(), stats.InUse)
	queries := []struct{ name, q string }{
		{"wake", `SELECT count(*),COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(first_attempt_at)),0) FROM wake_state WHERE desired_revision>acked_revision AND NOT wake_unreachable`},
		{"certificate", `SELECT count(*),COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(created_at)),0) FROM certificate_orders WHERE state IN('queued','creating','ordering','finalizing','reconciliation_required')`},
		{"mail", `SELECT count(*),COALESCE(EXTRACT(EPOCH FROM clock_timestamp()-min(created_at)),0) FROM account_mail_outbox WHERE status IN('pending','sending')`}}
	for _, q := range queries {
		var depth int64
		var age float64
		if e := s.DB.QueryRowContext(ctx, q.q).Scan(&depth, &age); e != nil {
			return e
		}
		fmt.Fprintf(w, "portico_queue_depth{queue=%q} %d\nportico_queue_oldest_seconds{queue=%q} %g\n", q.name, depth, q.name, max(age, 0))
	}
	for _, q := range []struct{ name, q string }{{"dns_feed_head", `SELECT COALESCE(max(sequence),0) FROM direct_addresses`}, {"ct_unknown_total", `SELECT count(*) FROM ct_observations WHERE NOT known`}, {"certificate_admissions_last_minute", `SELECT count(*) FROM certificate_order_admissions WHERE admitted_at>clock_timestamp()-interval '1 minute'`}} {
		var n int64
		if e := s.DB.QueryRowContext(ctx, q.q).Scan(&n); e != nil {
			return e
		}
		fmt.Fprintf(w, "portico_%s %d\n", q.name, n)
	}
	fmt.Fprintf(w, "portico_cgnat_reports_total %d\nportico_cgnat_shared_reports_total %d\n", s.cgnatReports.Load(), s.cgnatShared.Load())
	var last time.Time
	e := s.DB.QueryRowContext(ctx, `SELECT COALESCE(max(observed_at),'epoch') FROM ct_observations`).Scan(&last)
	if e != nil {
		return e
	}
	fmt.Fprintf(w, "portico_ct_last_observation_timestamp_seconds %d\n", last.Unix())
	return nil
}

// A local CT collector supplies the logged leaf certificate, not an asserted
// fingerprint. Reconciliation compares exact DER against retained issued orders.
func (s *Store) ObserveCT(ctx context.Context, der []byte) (bool, error) {
	cert, e := x509.ParseCertificate(der)
	if e != nil {
		return false, e
	}
	var namespace string
	for _, n := range cert.DNSNames {
		parts := strings.Split(n, ".")
		if len(parts) == 5 && strings.Join(parts[2:], ".") == "direct.getportico.tv" && strings.HasPrefix(parts[1], "ptc-") {
			namespace = parts[1]
			break
		}
	}
	if namespace == "" {
		return false, errors.New("certificate outside managed zone")
	}
	sum := sha256.Sum256(der)
	hash := hex.EncodeToString(sum[:])
	rows, e := s.DB.QueryContext(ctx, `SELECT chain FROM certificate_orders WHERE namespace=$1 AND chain IS NOT NULL`, namespace)
	if e != nil {
		return false, e
	}
	known := false
	for rows.Next() {
		var chain []byte
		if e = rows.Scan(&chain); e != nil {
			rows.Close()
			return false, e
		}
		block, _ := pem.Decode(chain)
		if block != nil {
			candidate := sha256.Sum256(block.Bytes)
			known = known || candidate == sum
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return false, e
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO ct_observations(fingerprint,namespace,known) VALUES($1,$2,$3) ON CONFLICT(fingerprint) DO UPDATE SET known=ct_observations.known OR excluded.known,observed_at=clock_timestamp()`, hash, namespace, known)
	return known, e
}
