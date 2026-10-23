package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/netip"
	"net/url"
	"portico.local/dns/directdns"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/directroute"
)

// WithCurrentDirectRouteAuthority commits a bounded route calculation under the
// same full-tuple authority locks used by current endpoint registration. The
// callback must never perform I/O or retain transaction authority for later use.
func (s *Store) WithCurrentDirectRouteAuthority(ctx context.Context, a directory.ServerAuthority, issue func() error) error {
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if issue == nil {
		return directory.ErrDenied
	}
	if e = issue(); e != nil {
		return e
	}
	// Recheck within this transaction immediately before returning the route.
	if e = checkServerAuthorityTx(ctx, tx, a); e != nil {
		return e
	}
	return tx.Commit()
}

// WithDirectNamespace retains account -> claim operation -> server lock order.
// Entropy is independent of the persistent server identity and never reallocated
// for a new WAN address. Reclaim under a different operation gets a fresh token.
func (s *Store) WithDirectNamespace(ctx context.Context, a directory.ServerAuthority, issue func(string) error) error {
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	ns, e := directNamespaceTx(ctx, tx, a.OperationID)
	if e != nil {
		return e
	}
	if issue == nil {
		return directory.ErrDenied
	}
	if e = issue(ns); e != nil {
		return e
	}
	if e = checkServerAuthorityTx(ctx, tx, a); e != nil {
		return e
	}
	return tx.Commit()
}

func directNamespaceTx(ctx context.Context, tx *sql.Tx, operation string) (string, error) {
	var ns string
	e := tx.QueryRowContext(ctx, `SELECT namespace FROM certificate_namespaces WHERE operation_id=$1`, operation).Scan(&ns)
	if e == nil {
		if !directdns.ValidNamespace(ns) {
			return "", directory.ErrDenied
		}
		return ns, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return "", e
	}
	var server string
	if e = tx.QueryRowContext(ctx, `SELECT server_id FROM claim_operations WHERE id=$1`, operation).Scan(&server); e != nil {
		return "", e
	}
	// The same installation key keeps its namespace across an explicit re-claim.
	e = tx.QueryRowContext(ctx, `UPDATE certificate_namespaces SET operation_id=$1,member_label=$3 WHERE server_identity=$2 RETURNING namespace`, operation, server, newMemberLabel()).Scan(&ns)
	if e == nil {
		return ns, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return "", e
	}
	ns, e = directdns.NewNamespace()
	if e != nil {
		return "", e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO certificate_namespaces(operation_id,namespace,member_label,server_identity) VALUES($1,$2,$3,$4)`, operation, ns, newMemberLabel(), server)
	return ns, e
}

// directAddressLock serialises address writes so feed sequences commit in order: a reader that
// has seen sequence N can never later find an N-1 it missed. Writes are rare (a server's WAN
// address changes about monthly), so one lock costs nothing.
const directAddressLock = 0x706f727469636f01

// RecordDirectAddress stores a server's current public address for its namespace. An
// unchanged address writes nothing; a change takes the next feed sequence and is logged.
func (s *Store) RecordDirectAddress(ctx context.Context, a directory.ServerAuthority, address netip.Addr) (string, bool, error) {
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return "", false, e
	}
	defer tx.Rollback()
	ns, e := directNamespaceTx(ctx, tx, a.OperationID)
	if e != nil {
		return "", false, e
	}
	changed, e := recordDirectAddressTx(ctx, tx, a.ServerID, ns, address)
	if e != nil {
		return "", false, e
	}
	if e = checkServerAuthorityTx(ctx, tx, a); e != nil {
		return "", false, e
	}
	return ns, changed, tx.Commit()
}

// WithdrawDirectAddress clears one address family for a server's namespace. Until this existed
// a family a server stopped having (an ISP withdrawing IPv6) stayed in direct_addresses for
// ever, and the DNS nodes kept answering AAAA with an address nothing listens on, which
// dual-stack clients try first and time out on.
//
// A withdrawal is a change like any other: it takes the next feed sequence under the same
// ordering lock and writes a tombstone event (an empty address with a non-empty previous), so
// the feed carries the removal to the DNS nodes with no new periodic contact anywhere.
func (s *Store) WithdrawDirectAddress(ctx context.Context, a directory.ServerAuthority, family string) (string, bool, error) {
	if !directroute.ValidFamily(family) {
		return "", false, directory.ErrDenied
	}
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return "", false, e
	}
	defer tx.Rollback()
	ns, e := directNamespaceTx(ctx, tx, a.OperationID)
	if e != nil {
		return "", false, e
	}
	changed, e := withdrawDirectAddressTx(ctx, tx, a.ServerID, ns, family)
	if e != nil {
		return "", false, e
	}
	if e = checkServerAuthorityTx(ctx, tx, a); e != nil {
		return "", false, e
	}
	return ns, changed, tx.Commit()
}

func withdrawDirectAddressTx(ctx context.Context, tx *sql.Tx, serverID, ns, family string) (bool, error) {
	// family reaches SQL as a column name, so it is never anything but one of the two
	// constants, checked again here rather than trusted from the caller.
	if !directroute.ValidFamily(family) {
		return false, directory.ErrDenied
	}
	var current string
	e := tx.QueryRowContext(ctx, `SELECT `+family+` FROM direct_addresses WHERE namespace=$1`, ns).Scan(&current)
	if errors.Is(e, sql.ErrNoRows) || current == "" {
		// Nothing was published for this family. A repeated withdrawal is a success that
		// spends no sequence, so a lost response costs one harmless repeat.
		return false, ignoreNoRows(e)
	}
	if e != nil {
		return false, e
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(directAddressLock)); e != nil {
		return false, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE direct_addresses SET `+family+`='',sequence=nextval('direct_address_sequence'),updated_at=clock_timestamp() WHERE namespace=$1`, ns); e != nil {
		return false, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO direct_address_events(server_id,namespace,address,previous) VALUES($1,$2,'',$3)`, serverID, ns, current)
	return e == nil, e
}

func ignoreNoRows(e error) error {
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	return e
}

func recordDirectAddressTx(ctx context.Context, tx *sql.Tx, serverID, ns string, address netip.Addr) (bool, error) {
	column := "ipv4"
	if address.Is6() {
		column = "ipv6"
	}
	var current string
	e := tx.QueryRowContext(ctx, `SELECT `+column+` FROM direct_addresses WHERE namespace=$1`, ns).Scan(&current)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return false, e
	}
	changed := current != address.String()
	if changed {
		if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(directAddressLock)); e != nil {
			return false, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO direct_addresses(namespace,server_id,`+column+`,sequence) VALUES($1,$2,$3,nextval('direct_address_sequence'))
   ON CONFLICT(namespace) DO UPDATE SET server_id=EXCLUDED.server_id,`+column+`=EXCLUDED.`+column+`,sequence=EXCLUDED.sequence,updated_at=clock_timestamp()`, ns, serverID, address.String()); e != nil {
			return false, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO direct_address_events(server_id,namespace,address,previous) VALUES($1,$2,$3,$4)`, serverID, ns, address.String(), current); e != nil {
			return false, e
		}
	}
	return changed, nil
}

func (s *Store) DirectAddressChanges(ctx context.Context, after int64, limit int) ([]directroute.AddressChange, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT d.sequence,d.namespace,d.ipv4,d.ipv6,n.member_label,d.lan_ipv4,d.lan_ipv6,CASE WHEN n.previous_label_until>clock_timestamp() THEN n.previous_member_label ELSE '' END,COALESCE(to_char(n.previous_label_until AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),'') FROM direct_addresses d JOIN certificate_namespaces n ON n.namespace=d.namespace WHERE d.sequence>$1 UNION ALL SELECT sequence,namespace,'','',member_label,'','',previous_member_label,'' FROM direct_address_withdrawals WHERE sequence>$1 ORDER BY 1 LIMIT $2`, after, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []directroute.AddressChange{}
	for rows.Next() {
		var c directroute.AddressChange
		if e = rows.Scan(&c.Sequence, &c.Namespace, &c.IPv4, &c.IPv6, &c.Label, &c.LANIPv4, &c.LANIPv6, &c.PreviousLabel, &c.PreviousLabelUntil); e != nil {
			return nil, e
		}
		if c.PreviousLabel == "" {
			c.PreviousLabelUntil = ""
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// withdrawClaimAddressesTx keeps a consumable tombstone for the exact retired
// operation. Callers must hold its server authority lock before withdrawing.
func withdrawClaimAddressesTx(ctx context.Context, tx *sql.Tx, server, operation string) error {
	var ns string
	err := tx.QueryRowContext(ctx, `SELECT namespace FROM certificate_namespaces WHERE operation_id=$1`, operation).Scan(&ns)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, family := range []string{"ipv4", "ipv6"} {
		if _, err = withdrawDirectAddressTx(ctx, tx, server, ns, family); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(directAddressLock)); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE direct_addresses SET lan_ipv4='',lan_ipv6='',sequence=nextval('direct_address_sequence'),updated_at=clock_timestamp() WHERE namespace=$1 AND (lan_ipv4<>'' OR lan_ipv6<>'')`, ns)
	if err != nil {
		return err
	}
	return nil
}

func newMemberLabel() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])
}
func (s *Store) DirectRouteLabel(ctx context.Context, namespace string) (string, error) {
	var label string
	err := s.DB.QueryRowContext(ctx, `SELECT member_label FROM certificate_namespaces WHERE namespace=$1`, namespace).Scan(&label)
	if err != nil || label != "" {
		return label, err
	}
	err = s.DB.QueryRowContext(ctx, `UPDATE certificate_namespaces SET member_label=CASE WHEN member_label='' THEN $2 ELSE member_label END WHERE namespace=$1 RETURNING member_label`, namespace, newMemberLabel()).Scan(&label)
	return label, err
}

// A non-source address requires a current-generation, bounded-lifetime dial-back proof.
func (s *Store) DirectAddressVerified(ctx context.Context, a directory.ServerAuthority, ip netip.Addr, port int) (bool, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT c.origin FROM networking_route_candidates c JOIN networking_route_state n ON n.server_id=c.server_id AND n.generation=c.generation AND n.operation_id=c.operation_id WHERE c.server_id=$1 AND c.operation_id=$2 AND n.claim_generation=$3 AND n.credential_generation=$4 AND c.state='verified' AND c.verified_at>clock_timestamp()-interval '8 days' LIMIT 32`, a.ServerID, a.OperationID, a.ClaimGeneration, a.CredentialGeneration)
	if e != nil {
		return false, e
	}
	defer rows.Close()
	for rows.Next() {
		var origin string
		if e = rows.Scan(&origin); e != nil {
			return false, e
		}
		u, e := url.Parse(origin)
		if e != nil {
			continue
		}
		addr, e := netip.ParseAddr(u.Hostname())
		if e == nil && addr == ip {
			return true, nil
		}
	}
	return false, rows.Err()
}

func recordLANAddressesTx(ctx context.Context, tx *sql.Tx, a directory.ServerAuthority, q directory.RouteObservation) error {
	var v4, v6 string
	for _, c := range q.Candidates {
		if c.Class != "lan" {
			continue
		}
		u, e := url.Parse(c.BaseURL)
		if e != nil {
			continue
		}
		ip, e := netip.ParseAddr(u.Hostname())
		if e != nil || !ip.IsPrivate() {
			continue
		}
		if ip.Is4() && v4 == "" {
			v4 = ip.String()
		}
		if ip.Is6() && !ip.Is4In6() && v6 == "" {
			v6 = ip.String()
		}
	}
	ns, e := directNamespaceTx(ctx, tx, a.OperationID)
	if e != nil {
		return e
	}
	var old4, old6 string
	e = tx.QueryRowContext(ctx, `SELECT lan_ipv4,lan_ipv6 FROM direct_addresses WHERE namespace=$1`, ns).Scan(&old4, &old6)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if old4 == v4 && old6 == v6 {
		return nil
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(directAddressLock)); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO direct_addresses(namespace,server_id,lan_ipv4,lan_ipv6,sequence) VALUES($1,$2,$3,$4,nextval('direct_address_sequence')) ON CONFLICT(namespace) DO UPDATE SET lan_ipv4=EXCLUDED.lan_ipv4,lan_ipv6=EXCLUDED.lan_ipv6,sequence=EXCLUDED.sequence,updated_at=clock_timestamp()`, ns, a.ServerID, v4, v6)
	return e
}
