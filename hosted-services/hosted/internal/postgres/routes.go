package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"portico.local/dns/directdns"
	"portico.local/hosted/internal/directory"
	"strings"
)

func (s *Store) ObserveRoutes(ctx context.Context, a directory.ServerAuthority, q directory.RouteObservation, digest []byte) error {
	if len(digest) != 32 {
		return directory.ErrInvalid
	}
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var generation int64
	var operation string
	var previousDigest []byte
	e = tx.QueryRowContext(ctx, `SELECT operation_id,generation,observation_digest FROM networking_route_state WHERE server_id=$1 FOR UPDATE`, a.ServerID).Scan(&operation, &generation, &previousDigest)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if e == nil && operation == a.OperationID {
		if q.Generation < generation {
			return directory.ErrConflict
		}
		if q.Generation == generation {
			if !bytes.Equal(digest, previousDigest) {
				return directory.ErrConflict
			}
			if _, e = tx.ExecContext(ctx, `DELETE FROM current_server_endpoints WHERE server_id=$1 AND NOT EXISTS(SELECT 1 FROM servers s WHERE s.id=$1 AND s.endpoint_verified AND s.base_url=current_server_endpoints.origin)`, a.ServerID); e != nil {
				return e
			}
			return tx.Commit()
		}
	}
	if operation != "" && operation != a.OperationID {
		if _, e = tx.ExecContext(ctx, `DELETE FROM networking_route_state WHERE server_id=$1`, a.ServerID); e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO networking_route_state(server_id,operation_id,claim_generation,credential_generation,generation,observation_digest,remote_enabled) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(server_id) DO UPDATE SET operation_id=EXCLUDED.operation_id,claim_generation=EXCLUDED.claim_generation,credential_generation=EXCLUDED.credential_generation,generation=EXCLUDED.generation,observation_digest=EXCLUDED.observation_digest,remote_enabled=EXCLUDED.remote_enabled,observed_at=clock_timestamp()`, a.ServerID, a.OperationID, a.ClaimGeneration, a.CredentialGeneration, q.Generation, digest, q.RemoteEnabled)
	if e != nil {
		return e
	}
	// Keep only the immediately previous verified set, never an unbounded history.
	if _, e = tx.ExecContext(ctx, `DELETE FROM networking_route_candidates WHERE server_id=$1 AND (NOT $3 AND class<>'lan' OR generation<>COALESCE((SELECT max(generation) FROM networking_route_candidates WHERE server_id=$1 AND operation_id=$2 AND verified_at IS NOT NULL),-1))`, a.ServerID, a.OperationID, q.RemoteEnabled); e != nil {
		return e
	}
	for _, c := range q.Candidates {
		if _, e = tx.ExecContext(ctx, `INSERT INTO networking_route_candidates(server_id,operation_id,generation,origin,class,state) VALUES($1,$2,$3,$4,$5,'reported')`, a.ServerID, a.OperationID, q.Generation, c.BaseURL, c.Class); e != nil {
			return e
		}
	}
	if e = recordLANAddressesTx(ctx, tx, a, q); e != nil {
		return e
	}
	// A removed route cannot keep receiving wakes just because an older proof exists.
	if _, e = tx.ExecContext(ctx, `UPDATE servers SET endpoint_verified=false,base_url=NULL WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM networking_route_candidates c WHERE c.server_id=$1 AND c.generation=$2 AND c.origin=servers.base_url AND c.class<>'lan')`, a.ServerID, q.Generation); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM current_server_endpoints WHERE server_id=$1 AND NOT EXISTS(SELECT 1 FROM servers s WHERE s.id=$1 AND s.endpoint_verified AND s.base_url=current_server_endpoints.origin)`, a.ServerID); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	// Fleet-wide counters only: no account or server identifier is retained.
	s.cgnatReports.Add(1)
	if q.CGNAT {
		s.cgnatShared.Add(1)
	}
	return nil
}
func (s *Store) RecordRouteFailure(ctx context.Context, a directory.ServerAuthority, generation int64, origin, state string) error {
	if state != "failed" && state != "identity_mismatch" {
		return directory.ErrInvalid
	}
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `UPDATE networking_route_candidates c SET state=$5,updated_at=clock_timestamp() FROM networking_route_state s WHERE s.server_id=$1 AND s.operation_id=$2 AND s.generation=$3 AND c.server_id=s.server_id AND c.generation=s.generation AND c.origin=$4`, a.ServerID, a.OperationID, generation, origin, state)
	if e != nil {
		return e
	}
	if state == "identity_mismatch" {
		if _, e = tx.ExecContext(ctx, `UPDATE servers SET endpoint_verified=false,base_url=NULL WHERE id=$1 AND base_url=$2 AND EXISTS(SELECT 1 FROM networking_route_state WHERE server_id=$1 AND operation_id=$3 AND generation=$4)`, a.ServerID, origin, a.OperationID, generation); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM current_server_endpoints WHERE server_id=$1 AND NOT EXISTS(SELECT 1 FROM servers s WHERE s.id=$1 AND s.endpoint_verified AND s.base_url=current_server_endpoints.origin)`, a.ServerID); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) RouteSets(ctx context.Context, account string) (map[string]directory.RouteSet, error) {
	// One bounded query for the directory, rather than a probe or query per route.
	rows, e := s.DB.QueryContext(ctx, `SELECT s.id,s.claim_generation,s.credential_generation,COALESCE(n.generation,0),c.origin,c.class,c.generation,
 CASE WHEN c.state='verified' AND c.generation=n.generation THEN 'reachable' ELSE 'probe_required' END,COALESCE(ns.namespace,''),COALESCE(ns.member_label,'')
 FROM servers s JOIN claim_operations o ON o.server_id=s.id AND o.account_id=s.owner_account_id AND o.claim_generation=s.claim_generation AND o.credential_generation=s.credential_generation AND o.committed_token_hash=s.token_hash AND o.state='installed'
 LEFT JOIN certificate_namespaces ns ON ns.operation_id=o.id
 LEFT JOIN networking_route_state n ON n.server_id=s.id AND n.operation_id=o.id AND n.claim_generation=s.claim_generation AND n.credential_generation=s.credential_generation
 LEFT JOIN networking_route_candidates c ON c.server_id=n.server_id AND c.operation_id=n.operation_id AND c.state<>'identity_mismatch' AND (c.generation=n.generation OR c.verified_at IS NOT NULL AND NOT EXISTS(SELECT 1 FROM networking_route_candidates current WHERE current.server_id=n.server_id AND current.operation_id=n.operation_id AND current.generation=n.generation AND current.origin=c.origin)) AND (n.remote_enabled OR c.class='lan')
 WHERE s.revoked_at IS NULL AND ((s.owner_account_id=$1 AND NOT s.custody_released) OR EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.account_id=$1))
 ORDER BY s.id,c.generation DESC,c.origin LIMIT 24000`, account)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]directory.RouteSet{}
	for rows.Next() {
		var id, quality, namespace, label string
		var claim, credential, gen int64
		var origin, class sql.NullString
		var candidateGen sql.NullInt64
		if e = rows.Scan(&id, &claim, &credential, &gen, &origin, &class, &candidateGen, &quality, &namespace, &label); e != nil {
			return nil, e
		}
		set := out[id]
		set.Generation = gen
		set.ClaimGeneration = claim
		set.CredentialGeneration = credential
		if origin.Valid {
			// A86: a public candidate the server published under an older
			// members-only label is answered under the current one; the server's
			// wildcard certificate covers both.
			if class.String != "lan" && directdns.ValidMemberLabel(label) {
				if u, e := url.Parse(origin.String); e == nil && u.Scheme == "https" {
					if ns, ok := directdns.NamespaceFromCurrentHostname(u.Hostname(), "direct.getportico.tv"); ok && ns == strings.ToLower(namespace) && strings.HasPrefix(u.Hostname(), "c-") {
						host := directdns.CurrentHostname(namespace, "direct.getportico.tv", label)
						if u.Port() != "" {
							host = net.JoinHostPort(host, u.Port())
						}
						origin.String = "https://" + host
					}
				}
			}
			if class.String == "lan" && directdns.ValidMemberLabel(label) {
				u, e := url.Parse(origin.String)
				if e == nil && u.Scheme == "https" {
					host := directdns.LANHostname(namespace, "direct.getportico.tv", label)
					if u.Port() != "" {
						host = net.JoinHostPort(host, u.Port())
					}
					origin.String = "https://" + host
				}
			}

			candidate := directory.RouteCandidate{BaseURL: origin.String, Class: class.String, Quality: quality, Generation: candidateGen.Int64}
			duplicate := false
			for i, previous := range set.Candidates {
				if previous.BaseURL == candidate.BaseURL {
					// Several literals or rotated labels can name one origin.
					// Keep current evidence over history, then reachable quality.
					if candidate.Generation > previous.Generation || candidate.Generation == previous.Generation && candidate.Quality == "reachable" {
						set.Candidates[i] = candidate
					}
					duplicate = true
					break
				}
			}
			if !duplicate {
				set.Candidates = append(set.Candidates, candidate)
			}
		}
		out[id] = set
	}
	return out, rows.Err()
}
