package postgres

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
)

// ExpectedProviderIdentity narrows the existing provider reauthentication to
// the actual link/revision selected before the original credential task leaves.
// It adds no proof requirement to any other account operation.
type ExpectedProviderIdentity struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision,string"`
}

type SecurityReturnDisplay struct {
	IntentID         string                      `json:"intentId"`
	RegisteredOrigin string                      `json:"registeredOrigin"`
	State            string                      `json:"state"`
	Context          serverreturn.SecuritySigned `json:"context"`
}

// The start response freezes these original registered facts in the Hosted
// page's protected journal before its provider navigation. No credential or
// recent proof is returned by this presentation metadata query.
func (o *OIDCLogin) SecurityReturnDisplay(ctx context.Context, intent, transaction string) (SecurityReturnDisplay, error) {
	var out SecurityReturnDisplay
	var cipher []byte
	e := o.Store.DB.QueryRowContext(ctx, `SELECT p.input_cipher FROM account_security_return_intents p JOIN identity_security_returns r ON r.intent_id=p.id WHERE p.id=$1 AND r.transaction_id=$2 AND p.cancelled_at IS NULL AND r.cancelled_at IS NULL AND p.expires_at>clock_timestamp()`, intent, transaction).Scan(&cipher)
	if e != nil {
		return out, identity.ErrCredentials
	}
	plain, e := o.Keys.Open(cipher, "security-return-intent:"+intent)
	if e != nil {
		return out, e
	}
	var q serverreturn.SecurityReturnStart
	if json.Unmarshal([]byte(plain), &q) != nil || !q.Valid() {
		return out, identity.ErrInvalid
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(q.Context.Payload)
	if e != nil {
		return out, identity.ErrInvalid
	}
	var c serverreturn.SecurityContext
	if json.Unmarshal(raw, &c) != nil || !serverreturn.Origin(c.Origin) {
		return out, identity.ErrInvalid
	}
	return SecurityReturnDisplay{IntentID: intent, RegisteredOrigin: c.Origin, State: q.State, Context: q.Context}, nil
}

// Cancellation is exact to one original browser Security transaction. It never
// rotates/cancels the shared binding or affects a successor provider operation.
func (o *OIDCLogin) CancelSecurityTransaction(ctx context.Context, id, binding string) error {
	if len(id) > 128 || len(binding) != 43 {
		return identity.ErrInvalid
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var generation int64
	if e = tx.QueryRowContext(ctx, `SELECT generation FROM identity_browser_bindings WHERE binding_hash=$1 AND expires_at>clock_timestamp() FOR SHARE`, security.Hash(binding)).Scan(&generation); e != nil {
		return identity.ErrCredentials
	}
	var a identity.CurrentFamilyExpectation
	var cipher []byte
	e = tx.QueryRowContext(ctx, `SELECT expected_account_id,expected_family_id,expected_account_epoch,expected_family_generation,expected_access_hash,session_mode,security_intent_cipher FROM identity_transactions WHERE id=$1 AND binding_hash=$2 AND binding_generation=$3 AND mode='reauthenticate' AND session_mode='browser'`, id, security.Hash(binding), generation).Scan(&a.AccountID, &a.FamilyID, &a.AuthEpoch, &a.FamilyGeneration, &a.TokenHash, &a.Mode, &cipher)
	if e != nil {
		return identity.ErrCredentials
	}
	// Match CompleteFlow's binding -> account/family -> transaction order.
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, a)
	if e != nil {
		return e
	}
	r, e := o.lock(ctx, tx, id, binding)
	if e != nil {
		return e
	}
	plain, e := o.Keys.Open(cipher, "oidc-intent:"+id)
	if e != nil {
		return e
	}
	var intent identity.SecurityIntent
	if json.Unmarshal([]byte(plain), &intent) != nil || !intent.Valid() || !serverreturn.CredentialPurpose(intent.Purpose, intent.Target) {
		return identity.ErrInvalid
	}
	if r.Status != "cancelled" {
		if _, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET status='cancelled',terminal_at=COALESCE(terminal_at,clock_timestamp()),revision=revision+1 WHERE id=$1`, id); e != nil {
			return e
		}
	}
	// A distinct transaction may intentionally carry the same Security intent.
	// Only this transaction's sealed completion receipt identifies its own proof;
	// never delete by shared intent (which also matches password proofs).
	var receipt []byte
	purpose := "oidc:" + id
	e = tx.QueryRowContext(ctx, `SELECT ciphertext FROM account_security_receipts WHERE account_id=$1 AND family_id=$2 AND access_hash=$3 AND operation_id=$4 AND purpose=$5`, a.AccountID, a.FamilyID, a.TokenHash, intent.OperationID, purpose).Scan(&receipt)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if e == nil {
		sealed, err := o.Keys.Open(receipt, "security-receipt:"+a.AccountID+":"+intent.OperationID+":"+purpose)
		if err != nil {
			return err
		}
		var issued OIDCCompletion
		if json.Unmarshal([]byte(sealed), &issued) != nil || issued.Proof == nil || issued.Proof.AccountID != a.AccountID || issued.Proof.FamilyID != a.FamilyID || issued.Proof.Proof == "" {
			return identity.ErrInvalid
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM account_recent_proofs WHERE proof_hash=$1 AND account_id=$2 AND family_id=$3 AND captured_access_hash=$4 AND credential_kind IN ('provider','provider_security_return')`, security.Hash(issued.Proof.Proof), a.AccountID, a.FamilyID, a.TokenHash); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE identity_security_returns SET cancelled_at=COALESCE(cancelled_at,clock_timestamp()) WHERE transaction_id=$1`, id); e != nil {
		return e
	}
	if e = returnFresh(ctx, tx, facts); e != nil {
		return e
	}
	return tx.Commit()
}
