package networking

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"time"

	"portico.local/server/internal/claimweb"
	"portico.local/server/internal/dbwork"
)

// SignSetupStart is deliberately separate from Sign's fixed-kind allowlist.
// It signs only the complete canonical start beside its original signed claim,
// for this current prepared, initial setup. No caller gets a generic
// identity-key signing capability or a Local identity grant.
func (k *ProtectedKeys) SignSetupStart(ctx context.Context, binding Binding, original claimweb.Signed, start claimweb.SetupStart) (claimweb.Signed, error) {
	var none claimweb.Signed
	if k == nil || k.db == nil || k.root == nil || !validBinding(binding) || binding.AccountID != UnboundAccount {
		return none, ErrInvalid
	}
	if _, err := claimAuthority(ctx); err != nil {
		return none, err
	}
	g, err := dbwork.BeginSnapshot(ctx, k.db)
	if err != nil {
		return none, err
	}
	defer g.Rollback()
	tx := g.Tx()
	if err = guardClaimRequestTx(ctx, tx); err != nil {
		return none, err
	}
	current, active, installed, err := readCurrentIdentityTx(ctx, tx)
	if err != nil {
		return none, err
	}
	var pending string
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(pending_operation_id,'') FROM networking_claim_identity WHERE singleton=1`).Scan(&pending); err != nil {
		return none, err
	}
	intent, err := loadIntentTx(ctx, tx, binding.OperationID)
	if err != nil {
		return none, err
	}
	if current.ServerID != binding.ServerID || current.ResetGeneration != binding.LocalGeneration || !bytes.Equal(current.PublicKey, binding.PublicKey) || !active.Valid || active.String != binding.OperationID || installed.Valid || pending != "" || !sameBinding(intent.Binding, binding) || intent.Stage != Prepared || intent.Purpose != "setup" || intent.CurrentOwnerAccountID != "" || intent.PreviousOwnerRole != "" || intent.ReturnRegistrationID != "" || intent.owner.SetupOperationID != start.SetupRequestID || intent.owner.Epoch != start.SetupRevision {
		return none, ErrStale
	}
	key, err := k.read(current.KeyIncarnation)
	if err != nil {
		return none, err
	}
	defer clear(key)
	if !bytes.Equal(key.Public().(ed25519.PublicKey), current.PublicKey) {
		return none, ErrStale
	}
	raw, err := json.Marshal(start)
	if err != nil || len(raw) > 8192 {
		return none, ErrInvalid
	}
	signed := claimweb.Signed{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, raw))}
	claim, _, err := claimweb.VerifySetupStart(original, signed, start.Audience, time.Now())
	if err != nil || claim.AccountID != "" || claim.CurrentOwnerAccountID != "" || claim.PreviousOwnerRole != "" || claim.ReturnRegistrationID != "" || claim.OperationID != binding.OperationID || claim.ServerID != binding.ServerID || claim.LocalGeneration != binding.LocalGeneration || claim.PublicKey != base64.RawURLEncoding.EncodeToString(binding.PublicKey) {
		return none, ErrInvalid
	}
	if err = guardClaimRequestTx(ctx, tx); err != nil {
		return none, err
	}
	if _, err = claimAuthority(ctx); err != nil {
		return none, err
	}
	return signed, nil
}
