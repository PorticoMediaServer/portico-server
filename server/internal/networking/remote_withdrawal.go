package networking

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"time"
)

// A server tells Hosted its public address when it changes. Until this file existed it never
// told Hosted when one went away: if an ISP stopped handing out IPv6, `direct_addresses` kept
// the old AAAA for ever and the DNS nodes went on answering it. A dual-stack client prefers
// IPv6, so every one of them spent a connection timeout on a dead address before falling back.
//
// The hard part is not sending the withdrawal; it is being sure. A STUN service blocked for a
// minute, an HTTPS whoami that times out, a router that stops answering — none of those mean
// an address is gone, and acting on one would take a working route out of DNS. So a family is
// withdrawn only when:
//
//   - the pass could actually see that family if it existed (a complete interface scan
//     for IPv6; for IPv4 a complete scan with no usable local IPv4 listener address), and
//   - the other family is still published — a server that can see a global address of the
//     other kind is online, so "no IPv4" is a statement about IPv4 and not about the internet,
//     and a server with nothing left keeps its last known addresses because removing them
//     helps nobody, and
//   - that has held for three consecutive such observations spanning at least a quarter of an
//     hour.
//
// Nothing here is periodic contact: the counters are local, and Hosted hears one request, once,
// when the conclusion is reached.

const (
	familyIPv4 = "ipv4"
	familyIPv6 = "ipv6"
)

// familyObservation is everything one pass can say about one address family.
type familyObservation int

const (
	familyUnknown familyObservation = iota // this pass could not see it either way
	familyPresent
	familyAbsent
)

const (
	// withdrawObservations and withdrawWindow are the "be sure" rule. The active cadence is
	// thirty seconds; three conclusive observations across fifteen minutes means the
	// family was independently
	// looked for several times over a period no blip survives.
	withdrawObservations = 3
	withdrawWindow       = 15 * time.Minute
)

func addressFamily(ip netip.Addr) string {
	if ip.Is4() || ip.Is4In6() {
		return familyIPv4
	}
	return familyIPv6
}

func otherFamily(family string) string {
	if family == familyIPv4 {
		return familyIPv6
	}
	return familyIPv4
}

// observeFamily turns one pass's evidence into a statement about one family.
func observeFamily(family string, present map[string]bool, conclusive bool) familyObservation {
	switch {
	case present[family]:
		return familyPresent
	case conclusive && present[otherFamily(family)]:
		return familyAbsent
	default:
		return familyUnknown
	}
}

// reportedFamilies is what Hosted was last told, by family. The route-name cache already keeps
// exactly one row per family, so this is that row's address.
func (m *RemoteManager) reportedFamilies(ctx context.Context, authority string) (map[string]string, error) {
	rows, e := m.h.store.db.QueryContext(ctx, `SELECT address FROM networking_route_names WHERE authority=? LIMIT 8`, authority)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if ip, err := netip.ParseAddr(raw); err == nil {
			out[addressFamily(ip)] = raw
		}
	}
	return out, rows.Err()
}

type familyAbsence struct {
	Since        time.Time
	Observations int
}

func (m *RemoteManager) familyAbsence(ctx context.Context, authority, family string) (familyAbsence, bool, error) {
	var since int64
	var out familyAbsence
	e := m.h.store.db.QueryRowContext(ctx, `SELECT since,observations FROM networking_address_absence WHERE authority=? AND family=?`, authority, family).Scan(&since, &out.Observations)
	if errors.Is(e, sql.ErrNoRows) {
		return out, false, nil
	}
	if e != nil {
		return out, false, e
	}
	out.Since = time.UnixMilli(since)
	return out, true, nil
}

func (m *RemoteManager) saveFamilyAbsence(ctx context.Context, authority, family string, v familyAbsence) error {
	gated, e := m.h.store.tx(ctx)
	if e != nil {
		return e
	}
	defer gated.Rollback()
	if _, e = gated.Tx().ExecContext(ctx, `INSERT INTO networking_address_absence(authority,family,since,observations) VALUES(?,?,?,?)
  ON CONFLICT(authority,family) DO UPDATE SET since=excluded.since,observations=excluded.observations`, authority, family, v.Since.UnixMilli(), v.Observations); e != nil {
		return e
	}
	return commitClaimDatabaseTx(ctx, gated)
}

func (m *RemoteManager) clearFamilyAbsence(ctx context.Context, authority, family string) error {
	gated, e := m.h.store.tx(ctx)
	if e != nil {
		return e
	}
	defer gated.Rollback()
	if _, e = gated.Tx().ExecContext(ctx, `DELETE FROM networking_address_absence WHERE authority=? AND family=?`, authority, family); e != nil {
		return e
	}
	return commitClaimDatabaseTx(ctx, gated)
}

// trackFamily records one pass's observation and reports whether the family is now due to be
// withdrawn. It writes nothing while a family is present and unrecorded, and stops writing once
// the threshold is reached, so a server in any steady state — both families, or one family
// already withdrawn — does no work here at all.
func (m *RemoteManager) trackFamily(ctx context.Context, authority, family string, observation familyObservation, now time.Time) (bool, error) {
	current, found, e := m.familyAbsence(ctx, authority, family)
	if e != nil {
		return false, e
	}
	switch observation {
	case familyPresent:
		if !found {
			return false, nil
		}
		// It came back. A family that is present is not being withdrawn, however long it
		// was away, and the count starts again if it goes a second time.
		return false, m.clearFamilyAbsence(ctx, authority, family)
	case familyAbsent:
		if !found {
			return false, m.saveFamilyAbsence(ctx, authority, family, familyAbsence{Since: now, Observations: 1})
		}
		if current.Observations >= withdrawObservations {
			// Enough observations already; only the window is still running. Nothing is
			// written on these passes, so a family that is gone for a week costs one row.
			return !now.Before(current.Since.Add(withdrawWindow)), nil
		}
		current.Observations++
		if e = m.saveFamilyAbsence(ctx, authority, family, current); e != nil {
			return false, e
		}
		return current.Observations >= withdrawObservations && !now.Before(current.Since.Add(withdrawWindow)), nil
	default:
		// Unknown: the pass could not see this family either way. Neither confirm nor deny —
		// a blind spot must not age into a withdrawal, and must not reset a real one.
		return false, nil
	}
}

// The production caller holds the shared route mutation lane across this function.
// withdrawFamily tells Hosted the family is gone and forgets the name that was reported for
// it. Hosted takes a feed sequence and the DNS nodes drop the record; no certificate, name or
// namespace changes, because the server's one permanent name is not per address.
func (m *RemoteManager) withdrawFamily(ctx context.Context, q certificateState, authority, family string) error {
	var receipt struct {
		Namespace string `json:"namespace"`
		Family    string `json:"family"`
		Withdrawn bool   `json:"withdrawn"`
	}
	e := m.h.certificates.transport.certificateCall(ctx, q.Scope.Intent, "route_withdraw", "", struct {
		Family string `json:"family"`
	}{family}, &receipt)
	if e != nil {
		return e
	}
	if receipt.Family != family || receipt.Namespace != q.Namespace {
		return ErrInvalid
	}
	return m.h.store.WithInstalledTransaction(ctx, q.Scope.Intent, func(ctx context.Context, tx *sql.Tx) error {
		// Clear the old observation atomically with the acknowledged withdrawal.
		// A cached WAN answer may not restore it; only a fresh observation can.
		if _, err := tx.ExecContext(ctx, `UPDATE networking_certificate_settings SET public_address='',revision=revision+1 WHERE singleton=1 AND public_address!='' AND (instr(public_address,':')>0)=?`, family == familyIPv6); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE networking_certificate_state SET route_address='',route_port=0,route_hostname='',route_url='',route_next_attempt=0,route_error_code='' WHERE scope_id=? AND route_address!='' AND (instr(route_address,':')>0)=?`, q.Scope.ID, family == familyIPv6); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM networking_route_names WHERE authority=? AND (instr(address,':')>0)=?`, authority, family == familyIPv6); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM networking_address_absence WHERE authority=? AND family=?`, authority, family)
		return err
	})
}

// withdrawLostFamilies is the whole of the per-pass work: look at what is published, decide
// what each family's state is, and make at most one Hosted call when a family has been gone
// long enough to be sure. It returns the first error so the caller can back off with the same
// rule as everything else.
func (m *RemoteManager) withdrawLostFamilies(ctx context.Context, q certificateState, authority string, present map[string]bool, conclusive map[string]bool, now time.Time) error {
	reported, e := m.reportedFamilies(ctx, authority)
	if e != nil {
		return e
	}
	for _, family := range []string{familyIPv4, familyIPv6} {
		if reported[family] == "" {
			// Nothing was ever published for this family, so there is nothing to withdraw
			// and no reason to keep a counter for it.
			continue
		}
		due, err := m.trackFamily(ctx, authority, family, observeFamily(family, present, conclusive[family]), now)
		if err != nil {
			return err
		}
		if !due {
			continue
		}
		if err = m.withdrawFamily(ctx, q, authority, family); err != nil {
			return err
		}
	}
	return nil
}

// unnamedAddress reports whether any address in the candidate set has no route name cached.
// That is the exact test of "this differs from what Hosted was last told", and it is what
// lets a genuinely new address be named between two weekly attempts. The topology digest used
// to stand in for it, which meant an unrelated interface change bought a Hosted request.
func (m *RemoteManager) unnamedAddress(ctx context.Context, authority string, addresses map[string]int) (bool, error) {
	for address, port := range addresses {
		var one int
		e := m.h.store.db.QueryRowContext(ctx, `SELECT 1 FROM networking_route_names WHERE authority=? AND address=? AND port=?`, authority, address, port).Scan(&one)
		if errors.Is(e, sql.ErrNoRows) {
			return true, nil
		}
		if e != nil {
			return false, e
		}
	}
	return false, nil
}

// publishedFamilies is which families the candidate set actually contains.
func publishedFamilies(addresses map[string]int) map[string]bool {
	out := map[string]bool{}
	for raw := range addresses {
		if ip, e := netip.ParseAddr(raw); e == nil {
			out[addressFamily(ip)] = true
		}
	}
	return out
}

// hasPublicIPv4 says whether an interface already carries a routable IPv4 address, which is the
// one case where asking an outside service for it would be pointless.
func hasPublicIPv4(topo Topology) bool {
	for _, raw := range topo.Public {
		if ip, e := netip.ParseAddr(raw); e == nil && ip.Unmap().Is4() {
			return true
		}
	}
	return false
}

// A failed external observer cannot prove IPv4 loss while the listener still
// has a usable IPv4 interface. Missing interface data likewise remains unknown.
func hasLocalIPv4(topo Topology) bool {
	for _, raw := range append(append([]string{}, topo.LAN...), topo.Public...) {
		if ip, err := netip.ParseAddr(raw); err == nil && ip.Unmap().Is4() {
			return true
		}
	}
	return false
}
