package networking

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"portico.local/server/internal/dbwork"
)

// CleanupWire is a deny-only proof for the original immutable operation. It can
// never authorize playback, install credentials, or change a successor claim.
type CleanupWire struct {
	Kind                 string `json:"kind"`
	Version              string `json:"version"`
	Audience             string `json:"audience"`
	OperationID          string `json:"operationId"`
	ServerID             string `json:"serverId"`
	PublicKey            string `json:"publicKey"`
	ClaimGeneration      string `json:"claimGeneration"`
	CredentialGeneration string `json:"credentialGeneration"`
	RequestID            string `json:"requestId"`
	IssuedAt             string `json:"issuedAt"`
	ExpiresAt            string `json:"expiresAt"`
	AcknowledgeJobID     string `json:"acknowledgeJobId"`
	AcknowledgeRevision  int64  `json:"acknowledgeRevision"`
}

func cleanupTuple(a, b Intent) bool {
	return sameBinding(a.Binding, b.Binding) && a.ClaimGeneration == b.ClaimGeneration && a.CredentialGeneration == b.CredentialGeneration && validGeneration(a.ClaimGeneration) && validGeneration(a.CredentialGeneration)
}

// CleanupProof retains predecessor-key access only for this fixed protocol.
// An acknowledgement requires an erasure receipt committed by the local worker.
func (k *ProtectedKeys) CleanupProof(ctx context.Context, v Intent, audience, ack string) (CleanupWire, SignedProof, error) {
	var w CleanupWire
	if _, e := claimAuthority(ctx); e != nil {
		return w, SignedProof{}, e
	}
	if !validAudience(audience) || validateIntent(v, v.OperationID) != nil || (ack != "" && !validID(ack)) {
		return w, SignedProof{}, ErrInvalid
	}
	g, e := dbwork.BeginSnapshot(ctx, k.db)
	if e != nil {
		return w, SignedProof{}, e
	}
	defer g.Rollback()
	tx := g.Tx()
	if e = guardClaimRequestTx(ctx, tx); e != nil {
		return w, SignedProof{}, e
	}
	actual, e := loadIntentTx(ctx, tx, v.OperationID)
	if e != nil {
		return w, SignedProof{}, e
	}
	if !cleanupTuple(v, actual.Intent) {
		return w, SignedProof{}, ErrStale
	}
	if ack != "" {
		var revision int64
		e = tx.QueryRowContext(ctx, `SELECT revision FROM hosted_claim_cleanup_receipts WHERE operation_id=? AND job_id=?`, v.OperationID, ack).Scan(&revision)
		if e != nil || revision != 1 {
			return w, SignedProof{}, ErrStale
		}
	}
	var incarnation string
	if e = tx.QueryRowContext(ctx, `SELECT key_incarnation FROM networking_server_identities WHERE server_id=? AND public_key=?`, v.ServerID, v.PublicKey).Scan(&incarnation); e != nil {
		return w, SignedProof{}, e
	}
	key, e := k.read(incarnation)
	if e != nil {
		return w, SignedProof{}, e
	}
	defer clear(key)
	if !bytes.Equal(key.Public().(ed25519.PublicKey), v.PublicKey) {
		return w, SignedProof{}, ErrStale
	}
	nonce := make([]byte, 32)
	if _, e = rand.Read(nonce); e != nil {
		return w, SignedProof{}, e
	}
	now := time.Now().UTC()
	w = CleanupWire{"portico.claim.cleanup", "1", audience, v.OperationID, v.ServerID, base64.RawURLEncoding.EncodeToString(v.PublicKey), v.ClaimGeneration, v.CredentialGeneration, hex.EncodeToString(nonce), now.Format(time.RFC3339Nano), now.Add(30 * time.Second).Format(time.RFC3339Nano), ack, 0}
	if ack != "" {
		w.AcknowledgeRevision = 1
	}
	raw, e := json.Marshal(w)
	if e != nil {
		return w, SignedProof{}, e
	}
	if e = guardClaimRequestTx(ctx, tx); e != nil {
		return w, SignedProof{}, e
	}
	if _, e = claimAuthority(ctx); e != nil {
		return w, SignedProof{}, e
	}
	return w, SignedProof{Payload: raw, Signature: ed25519.Sign(key, raw)}, nil
}

func (t *HTTPTransport) ClaimCleanup(ctx context.Context, v Intent, p SignedProof) (PolicyEnvelope, error) {
	var out PolicyEnvelope
	envelope, e := proofRequest(v, p)
	if e != nil {
		return out, e
	}
	var w CleanupWire
	if decodeClaimJSON(p.Payload, &w) != nil || w.Kind != "portico.claim.cleanup" || w.Audience != t.origin || w.OperationID != v.OperationID || w.ServerID != v.ServerID || w.ClaimGeneration != v.ClaimGeneration || w.CredentialGeneration != v.CredentialGeneration {
		return out, ErrInvalid
	}
	e = t.controlRequest(ctx, http.MethodPost, "/v1/server-claims/cleanup", envelope, &out, nil, 1<<20)
	return out, e
}

// WithCleanupTransaction permits only retained-tuple cleanup after ordinary
// installed authority has been denied. The callback must authenticate Hosted's
// response before invoking this boundary. Its erasure/receipt writes commit
// atomically with retirement; a successor's pointer and credentials are fenced.
func (s *SQLiteStore) WithCleanupTransaction(ctx context.Context, v Intent, retired bool, apply func(context.Context, *sql.Tx) error) error {
	if apply == nil {
		return ErrInvalid
	}
	g, e := s.tx(ctx)
	if e != nil {
		return e
	}
	defer g.Rollback()
	tx := g.Tx()
	actual, e := loadIntentTx(ctx, tx, v.OperationID)
	if e != nil {
		return e
	}
	if !cleanupTuple(v, actual.Intent) {
		return ErrStale
	}
	if e = apply(ctx, tx); e != nil {
		return e
	}
	if retired {
		id, e := readIdentity(ctx, tx)
		if e != nil {
			return e
		}
		matches := id.server == v.ServerID && bytes.Equal(id.key, v.PublicKey) && id.active.Valid && id.active.String == v.OperationID && (!id.installed.Valid || id.installed.String == v.OperationID)
		if matches {
			if s.denied == nil {
				return ErrUnavailable
			}
			if e = s.denied(ctx, tx, actual.Intent, "account_deleted"); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE networking_claim_identity SET active_operation_id=NULL,installed_operation_id=NULL,reset_generation=reset_generation+1 WHERE singleton=1 AND active_operation_id=?`, v.OperationID); e != nil {
				return e
			}
		}
		if e = RevokeCertificatesTx(ctx, tx, v.OperationID); e != nil {
			return e
		}
		// Credential deletion is always exact-operation, even after a successor exists.
		if _, e = tx.ExecContext(ctx, `DELETE FROM networking_claim_credentials WHERE operation_id=?`, v.OperationID); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE networking_claim_intents SET stage='cancelled',revision=revision+1,updated_at=? WHERE operation_id=? AND stage<>'cancelled'`, time.Now().UTC().Format(time.RFC3339Nano), v.OperationID); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE networking_claim_cancellations SET state='acknowledged',acknowledged_at=? WHERE operation_id=?`, time.Now().UTC().Format(time.RFC3339Nano), v.OperationID); e != nil {
			return e
		}
	}
	return commitClaimTx(ctx, func() error {
		if e := guardClaimRequestTx(ctx, tx); e != nil {
			return e
		}
		return g.Commit()
	})
}
