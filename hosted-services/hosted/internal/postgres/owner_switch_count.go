package postgres

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/security"
)

// The caller must first perform the ordinary live owner/family/tuple checks
// in its lookup transaction. Only a fresh verified captured fact is returned.
type ownerSwitchCountProjection struct {
	people  int
	expires time.Time
}

func readOwnerSwitchCount(ctx context.Context, tx *sql.Tx, owner ownerStartRow, audience string) (*ownerSwitchCountProjection, error) {
	if !owner.liveAt(time.Now()) {
		return nil, nil
	}
	var originalPayload, originalSignature, startPayload, startSignature, countPayload, countSignature []byte
	e := tx.QueryRowContext(ctx, `SELECT original_claim_payload,original_claim_signature,start_payload,start_signature,switch_count_payload,switch_count_signature FROM owner_claim_starts WHERE transaction_id=$1`, owner.id).Scan(
		&originalPayload, &originalSignature, &startPayload, &startSignature, &countPayload, &countSignature)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if len(countPayload) == 0 || len(countSignature) == 0 ||
		!equalSecret(owner.claimHash, security.Hash(string(originalPayload))) || !equalSecret(owner.startHash, security.Hash(string(startPayload))) {
		return nil, nil
	}
	signed := func(payload, signature []byte) claimweb.Signed {
		return claimweb.Signed{Payload: base64.RawURLEncoding.EncodeToString(payload), Signature: base64.RawURLEncoding.EncodeToString(signature)}
	}
	// A database/lock wait must not preserve the caller's earlier clock sample.
	now := time.Now()
	if !owner.liveAt(now) {
		return nil, nil
	}
	count, e := claimweb.VerifyOwnerSwitchCount(signed(originalPayload, originalSignature), signed(startPayload, startSignature), signed(countPayload, countSignature), audience, now)
	if e != nil || count.OperationID != owner.operation || count.ServerID != owner.server || count.LocalGeneration != owner.local ||
		count.PublicKey != base64.RawURLEncoding.EncodeToString(owner.key) || !count.ExpiresAt.After(time.Now()) {
		return nil, nil
	}
	return &ownerSwitchCountProjection{people: count.OtherPeopleCount, expires: count.ExpiresAt}, nil
}
