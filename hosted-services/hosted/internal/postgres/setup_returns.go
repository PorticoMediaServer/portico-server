package postgres

import (
	"context"
	"database/sql"
	"net/url"
	"time"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
	"portico.local/hosted/trust"
)

func completeSetupReturnTx(ctx context.Context, tx *sql.Tx, a identity.CurrentFamilyExpectation, pre setupStartRow, facts identity.CurrentFamilyFacts) (serverreturn.Return, error) {
	var out serverreturn.Return
	if !sameSetupFamily(pre.a, a) || (pre.phase != "installed" && pre.phase != "completed") {
		return out, serverreturn.ErrInvalid
	}
	t, e := setupTupleTx(ctx, tx, pre)
	if e != nil || !sameReturnTuple(t, pre.t) {
		return out, serverreturn.ErrInvalid
	}
	r, e := readSetupStart(ctx, tx, pre.id, false, true)
	if e != nil || !sameSetupFamily(r.a, a) || (r.phase != "installed" && r.phase != "completed") {
		return out, serverreturn.ErrInvalid
	}
	result, e := tx.ExecContext(ctx, `UPDATE setup_claim_starts SET phase='completed',completed_at=COALESCE(completed_at,clock_timestamp()) WHERE transaction_id=$1 AND phase IN('installed','completed') AND expires_at>clock_timestamp()`, r.id)
	if e != nil {
		return out, e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return out, serverreturn.ErrInvalid
	}
	if e = returnFresh(ctx, tx, facts); e != nil {
		return out, e
	}
	// Only this opaque transaction is carried by the callback. State and all
	// proof/PKCE material remain in the original secure protected pending store.
	out.Location = r.address + serverreturn.ReturnPath + "#" + url.Values{"transaction": {r.id}}.Encode()
	if e = tx.Commit(); e != nil {
		return serverreturn.Return{}, e
	}
	return out, nil
}

// ExchangeSetupReturn requires an installed server credential captured by the
// HTTP boundary. The approving Hosted family is independently reauthorized;
// neither the transaction nor the server credential alone is account authority.
func (s *Store) ExchangeSetupReturn(ctx context.Context, a directory.ServerAuthority, q serverreturn.SetupExchange, sign func(serverreturn.Assertion) (trust.Envelope, error)) (serverreturn.Grant, error) {
	var out serverreturn.Grant
	if !q.Valid() || !a.Valid() || sign == nil {
		return out, serverreturn.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	pre, e := readSetupStart(ctx, tx, q.TransactionID, false, false)
	if e != nil || pre.a.AccountID != a.AccountID || pre.server != a.ServerID || pre.operation != a.OperationID || pre.phase != "completed" {
		return out, serverreturn.ErrInvalid
	}
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, pre.a)
	if e != nil {
		return out, serverreturn.ErrInvalid
	}
	if e = lockServerAuthorityOwnerTx(ctx, tx, a); e != nil {
		return out, e
	}
	if e = checkServerAuthorityTx(ctx, tx, a); e != nil {
		return out, e
	}
	t, e := setupTupleTx(ctx, tx, pre)
	if e != nil || !sameReturnTuple(t, pre.t) || t.claim != a.ClaimGeneration || t.credential != a.CredentialGeneration {
		return out, serverreturn.ErrInvalid
	}
	r, e := readSetupStart(ctx, tx, q.TransactionID, false, true)
	if e != nil || r.phase != "completed" || r.consumed.Valid || !sameSetupFamily(r.a, pre.a) || !equalSecret(r.stateHash, security.Hash(q.State)) || !equalSecret([]byte(r.codeChallenge), []byte(serverreturn.Challenge(q.Verifier))) || q.RegistrationID != r.registration || q.InstallationID != r.installation || !equalSecret(r.challengeHash, security.Hash(q.ServerChallenge)) {
		return out, serverreturn.ErrInvalid
	}
	var username, displayName, email string
	if e = tx.QueryRowContext(ctx, `SELECT username,display_name,COALESCE((SELECT address FROM account_contact_emails WHERE account_id=$1 AND is_primary AND verified_at IS NOT NULL),'') FROM accounts WHERE id=$1`, r.a.AccountID).Scan(&username, &displayName, &email); e != nil {
		return out, e
	}
	if username != r.username {
		return out, serverreturn.ErrInvalid
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	if !r.expiry.After(now) || !facts.UnexpiredAt(now) {
		return out, serverreturn.ErrInvalid
	}
	expires := now.Add(serverreturn.AssertionLifetime)
	if r.expiry.Before(expires) {
		expires = r.expiry
	}
	out.Assertion, e = sign(serverreturn.Assertion{Kind: serverreturn.AssertionKind, AccountID: r.a.AccountID, Username: username, DisplayName: displayName, Email: email, ServerID: r.server, Challenge: q.ServerChallenge, Nonce: security.RandomID(""), IssuedAt: now.UTC(), ExpiresAt: expires.UTC(), Purpose: "setup", InstallationID: r.installation, RegistrationID: r.registration, ClaimOperationID: r.operation, ClaimGeneration: t.claim, CredentialGeneration: t.credential, CurrentOwnerAccountID: t.current, PreviousOwnerRole: t.previous})
	if e != nil {
		return serverreturn.Grant{}, e
	}
	result, e := tx.ExecContext(ctx, `UPDATE setup_claim_starts SET phase='consumed',consumed_at=clock_timestamp() WHERE transaction_id=$1 AND phase='completed' AND consumed_at IS NULL AND expires_at>clock_timestamp() AND $2>clock_timestamp() AND $3>clock_timestamp() AND $4>clock_timestamp()`, r.id, facts.AccessExpiresAt, facts.AbsoluteExpiresAt, facts.IdleExpiresAt)
	if e != nil {
		return serverreturn.Grant{}, e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return serverreturn.Grant{}, serverreturn.ErrInvalid
	}
	if e = ctx.Err(); e != nil {
		return serverreturn.Grant{}, e
	}
	if e = tx.Commit(); e != nil {
		return serverreturn.Grant{}, e
	}
	return out, nil
}
