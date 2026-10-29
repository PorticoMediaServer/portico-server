package networking

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"portico.local/server/internal/directname"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
)

// CurrentManagedHTTPS is a local, read-only address observation. It grants no
// identity or registration. A client must independently verify the HTTPS peer;
// normal registered preparation still resolves the real live registration.
func (h *ClaimHandler) CurrentManagedHTTPS(ctx context.Context) (origin string, err error) {
	if h == nil || h.store == nil || h.runner == nil || h.certificates == nil {
		return "", nil
	}
	err = h.runner.Do(ctx, func(ctx context.Context) error {
		tx, release, e := dbwork.BeginRead(ctx, h.store.db)
		if e != nil {
			return e
		}
		defer release()
		origin, e = h.currentManagedHTTPSTx(ctx, tx)
		return e
	})
	if managedHTTPSAbsent(err) {
		return "", nil
	}
	return origin, err
}
func managedHTTPSAbsent(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrStale) || errors.Is(err, ErrUnavailable) || errors.Is(err, ErrCancelled)
}
func (h *ClaimHandler) currentManagedHTTPSTx(ctx context.Context, tx *sql.Tx) (string, error) {
	var kind string
	var enabled bool
	if e := tx.QueryRowContext(ctx, "SELECT s.sign_in_kind,m.enabled FROM identity_state s JOIN identity_managed_route m ON m.singleton=s.singleton WHERE s.singleton=1").Scan(&kind, &enabled); e != nil {
		return "", e
	}
	if kind != "portico" || !enabled {
		return "", nil
	}
	m := h.certificates
	if m.environment != "production" || m.bootError != "" || !m.listening.Load() || m.listenPort.Load() < 1 {
		return "", nil
	}
	q, e := m.servingSnapshotTx(ctx, tx)
	if e != nil {
		return "", e
	}
	var publicAddress string
	var publicPort int
	// Do not call snapshotTx/readCertificateConfigTx here: those initialize or
	// repair configuration. The public observation never enters a writer.
	if e = tx.QueryRowContext(ctx, "SELECT public_address,public_port FROM networking_certificate_settings WHERE singleton=1").Scan(&publicAddress, &publicPort); e != nil {
		return "", e
	}
	if publicAddress == "" || q.RouteAddress != publicAddress || q.RoutePort != publicPort || q.RouteErrorCode != "" || !strings.HasPrefix(q.RouteURL, "https://") || !RouteOrigin(q.RouteURL) {
		return "", nil
	}
	u, e := url.Parse(q.RouteURL)
	if e != nil || u.Hostname() != q.RouteHostname || !validManagedRouteName(q.RouteHostname, q.Namespace, m.directDomain) {
		return "", nil
	}
	port := 443
	if u.Port() != "" {
		port, e = strconv.Atoi(u.Port())
		if e != nil {
			return "", nil
		}
	}
	if port != publicPort {
		return "", nil
	}
	var authority, raw, digest string
	var revision, generation int64
	var remoteEnabled bool
	var remotePort int
	if e = tx.QueryRowContext(ctx, "SELECT r.authority,r.revision,r.enabled,r.public_port,s.generation,s.route_digest,s.status FROM networking_remote_settings r JOIN networking_remote_state s ON s.singleton=r.singleton WHERE r.singleton=1").Scan(&authority, &revision, &remoteEnabled, &remotePort, &generation, &digest, &raw); e != nil {
		return "", e
	}
	if !remoteEnabled || authority != q.Scope.ID || remotePort != publicPort || generation < 1 || digest == "" || len(raw) > 1<<20 {
		return "", nil
	}
	var observation RemoteStatus
	if json.Unmarshal([]byte(raw), &observation) != nil || observation.AuthorityID != authority || observation.Generation != generation || observation.Config.Revision != revision || !observation.Config.Enabled || observation.Config.PublicPort != remotePort || observation.State != "reachable" {
		return "", nil
	}
	observed := false
	for _, candidate := range observation.Candidates {
		if candidate.BaseURL == q.RouteURL && candidate.Class == "public" && candidate.State == "reachable" && candidate.VerifiedAt != nil && !candidate.VerifiedAt.IsZero() && !candidate.VerifiedAt.After(time.Now()) {
			observed = true
			break
		}
	}
	if !observed {
		return "", nil
	}
	valid := false
	for _, id := range []string{q.ActiveID, q.PreviousID} {
		if id == "" {
			continue
		}
		material, e := readCertificateMaterialTx(ctx, tx, q.Scope.ID, id)
		if errors.Is(e, sql.ErrNoRows) {
			continue
		}
		if e != nil {
			return "", e
		}
		now := time.Now()
		if material.State != "issued" || now.Before(material.NotBefore) || !now.Before(material.NotAfter) {
			continue
		}
		cert, e := m.validateMaterial(q.Scope, q.Namespace, material, material.Chain, now)
		if e == nil && cert.Leaf.VerifyHostname(q.RouteHostname) == nil {
			valid = true
			break
		}
	}
	if !valid || !m.listening.Load() || m.listenPort.Load() < 1 {
		return "", nil
	}
	// Use the same installed authority as the TLS serving path, including the
	// current key/reset/credential tuple and sticky terminal denials. A historical
	// claim-observation row is not produced by normal installation and cannot be
	// a prerequisite for observing this genuinely serving address. Neither that
	// row nor the remote display observation grants membership or a registration.
	if e = GuardInstalledClaim(ctx, tx, q.Scope.Intent); e != nil {
		return "", e
	}
	return q.RouteURL, nil
}

func validManagedRouteName(host, namespace, domain string) bool {
	_, ns, err := directname.ParseIPHostname(host, domain)
	return err == nil && ns == namespace
}
