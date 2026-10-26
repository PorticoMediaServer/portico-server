package postgres

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
)

// This purpose has no connection to server_returns' grant-producing exchange.
// The OIDC transaction remains the authority for provider identity, intent and
// original Hosted account/family; the signed context binds the server opening.
func (o *OIDCLogin) registerSecurityReturnTx(ctx context.Context, tx *sql.Tx, id, binding, intentID string, a identity.CurrentFamilyExpectation, intent identity.SecurityIntent, q serverreturn.SecurityReturnStart, callback string) error {
	var original bool
	if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_security_return_intents WHERE id=$1 AND account_id=$2 AND family_id=$3 AND account_epoch=$4 AND family_generation=$5 AND access_hash=$6 AND operation_id=$7 AND start_binding_hash=$8 AND cancelled_at IS NULL AND expires_at>clock_timestamp())`, intentID, a.AccountID, a.FamilyID, a.AuthEpoch, a.FamilyGeneration, a.TokenHash, intent.OperationID, security.Hash(binding)).Scan(&original); e != nil {
		return e
	}
	if !original {
		return identity.ErrCredentials
	}
	var public string
	// Parsing this locator grants nothing. All fields are used only after their
	// exact canonical bytes verify under the current installed server key.
	var locator struct {
		ServerID string `json:"serverId"`
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(q.Context.Payload)
	if e != nil || len(raw) > 6144 || json.Unmarshal(raw, &locator) != nil || len(locator.ServerID) > 128 {
		return serverreturn.ErrInvalid
	}
	if e = tx.QueryRowContext(ctx, `SELECT public_key FROM servers WHERE id=$1 AND revoked_at IS NULL AND NOT custody_released`, locator.ServerID).Scan(&public); e != nil {
		return serverreturn.ErrInvalid
	}
	key, e := base64.RawURLEncoding.Strict().DecodeString(public)
	if e != nil {
		return serverreturn.ErrInvalid
	}
	c, raw, e := serverreturn.VerifySecurityContext(q.Context, key, time.Now())
	u, ue := url.Parse(callback)
	if e != nil || ue != nil || c.HostedOrigin != u.Scheme+"://"+u.Host || c.AccountID != a.AccountID || c.OperationID != intent.OperationID {
		return serverreturn.ErrInvalid
	}
	t, e := returnTupleTx(ctx, tx, a.AccountID, c.ServerID, c.ClaimOperationID, "sign-in", c.RegistrationID)
	if e != nil || t.origin != c.Origin {
		return serverreturn.ErrInvalid
	}
	if e = tx.QueryRowContext(ctx, `SELECT public_key FROM servers WHERE id=$1`, c.ServerID).Scan(&public); e != nil || public != c.PublicKey {
		return serverreturn.ErrInvalid
	}
	var providerID string
	var revision int64
	var hasPassword bool
	if e = tx.QueryRowContext(ctx, `SELECT expected_identity_id,expected_identity_revision FROM identity_transactions WHERE id=$1 AND mode='reauthenticate' AND session_mode='browser' AND purpose='password_set_or_change' AND intent_target=''`, id).Scan(&providerID, &revision); e != nil {
		return serverreturn.ErrInvalid
	}
	if e = tx.QueryRowContext(ctx, `SELECT password_hash IS NOT NULL FROM accounts WHERE id=$1`, a.AccountID).Scan(&hasPassword); e != nil {
		return e
	}
	if hasPassword || providerID != c.ProviderIdentityID || revision != c.ProviderRevision {
		return identity.ErrRecentAuth
	}
	bindingCipher, e := o.Keys.Seal(binding, "security-return-binding:"+id)
	if e != nil {
		return e
	}
	sealed, e := o.Keys.Seal(string(raw), "security-return-context:"+id)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO identity_security_returns(transaction_id,context_cipher,context_digest,server_id,registration_id,origin,claim_generation,credential_generation,route_generation,state,code_challenge,expires_at,binding_cipher,intent_id,security_operation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, id, sealed, serverreturn.SecurityDigest(raw), c.ServerID, c.RegistrationID, c.Origin, t.claim, t.credential, t.route, q.State, q.CodeChallenge, c.ExpiresAt, bindingCipher, intentID, intent.OperationID)
	if e != nil {
		return e
	}
	return o.securityReturnFreshTx(ctx, tx, securityReturnRow{a: a, expiry: c.ExpiresAt})
}

type securityReturnRow struct {
	a                                        identity.CurrentFamilyExpectation
	c                                        serverreturn.SecurityContext
	t                                        returnTuple
	digest, state, challenge                 string
	code, cipher, consumedHash               []byte
	expiry                                   time.Time
	codeExpiry, consumed, confirmationExpiry sql.NullTime
}

// Acquire binding, original account/family, then transaction and server locks,
// matching the existing sensitive OIDC completion order. No token is issued.
func (o *OIDCLogin) securityReturnTx(ctx context.Context, tx *sql.Tx, id, binding string) (securityReturnRow, error) {
	var out securityReturnRow
	if len(binding) != 43 || len(id) > 128 {
		return out, identity.ErrCredentials
	}
	var generation int64
	if e := tx.QueryRowContext(ctx, `SELECT generation FROM identity_browser_bindings WHERE binding_hash=$1 AND expires_at>clock_timestamp() FOR SHARE`, security.Hash(binding)).Scan(&generation); e != nil {
		return out, identity.ErrCredentials
	}
	var link string
	var revision int64
	e := tx.QueryRowContext(ctx, `SELECT expected_account_id,expected_family_id,expected_account_epoch,expected_family_generation,expected_access_hash,session_mode,expected_identity_id,expected_identity_revision FROM identity_transactions WHERE id=$1 AND binding_hash=$2 AND binding_generation=$3 AND mode='reauthenticate' AND session_mode='browser' AND purpose='password_set_or_change' AND intent_target=''`, id, security.Hash(binding), generation).Scan(&out.a.AccountID, &out.a.FamilyID, &out.a.AuthEpoch, &out.a.FamilyGeneration, &out.a.TokenHash, &out.a.Mode, &link, &revision)
	if e != nil {
		return out, identity.ErrCredentials
	}
	if _, e = AuthorizeCurrentFamilyTx(ctx, tx, out.a); e != nil {
		return out, e
	}
	r, e := o.lock(ctx, tx, id, binding)
	if e != nil || (r.Status != "verified" && r.Status != "proof_issued") {
		return out, identity.ErrChallenge
	}
	var encrypted []byte
	var server, registration, origin string
	e = tx.QueryRowContext(ctx, `SELECT context_cipher,context_digest,server_id,registration_id,origin,claim_generation,credential_generation,route_generation,state,code_challenge,expires_at,code_hash,code_cipher,code_expires_at,consumed_at,consume_hash,confirmation_expires_at FROM identity_security_returns WHERE transaction_id=$1 AND cancelled_at IS NULL FOR UPDATE`, id).Scan(&encrypted, &out.digest, &server, &registration, &origin, &out.t.claim, &out.t.credential, &out.t.route, &out.state, &out.challenge, &out.expiry, &out.code, &out.cipher, &out.codeExpiry, &out.consumed, &out.consumedHash, &out.confirmationExpiry)
	if e != nil {
		return out, serverreturn.ErrInvalid
	}
	plain, e := o.Keys.Open(encrypted, "security-return-context:"+id)
	if e != nil || json.Unmarshal([]byte(plain), &out.c) != nil || serverreturn.SecurityDigest([]byte(plain)) != out.digest {
		return out, serverreturn.ErrInvalid
	}
	if out.c.AccountID != out.a.AccountID || out.c.ServerID != server || out.c.RegistrationID != registration || out.c.Origin != origin || out.c.ProviderIdentityID != link || out.c.ProviderRevision != revision || !out.c.ExpiresAt.Equal(out.expiry) {
		return out, serverreturn.ErrInvalid
	}
	// The exact provider issuer/subject verified by the real callback must still
	// own the original linked identity and revision. Email is never compared.
	var linked bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM identity_verified_assertions v JOIN account_external_identities i ON i.issuer=v.issuer AND i.subject=v.subject WHERE v.transaction_id=$1 AND i.id=$2 AND i.account_id=$3 AND i.revision=$4 AND i.revoked_at IS NULL AND i.provider=$5)`, id, link, out.a.AccountID, revision, r.Provider).Scan(&linked)
	if e != nil {
		return out, e
	}
	if !linked {
		return out, identity.ErrRecentAuth
	}
	current, e := returnTupleTx(ctx, tx, out.a.AccountID, server, out.c.ClaimOperationID, "sign-in", registration)
	out.t.origin = origin
	if e != nil || !sameReturnTuple(current, out.t) {
		return out, serverreturn.ErrInvalid
	}
	var currentKey string
	if e = tx.QueryRowContext(ctx, `SELECT public_key FROM servers WHERE id=$1`, server).Scan(&currentKey); e != nil || currentKey != out.c.PublicKey {
		return out, serverreturn.ErrInvalid
	}
	if e = o.securityReturnFreshTx(ctx, tx, out); e != nil {
		return out, e
	}
	return out, nil
}

func (o *OIDCLogin) securityReturnFreshTx(ctx context.Context, tx *sql.Tx, r securityReturnRow) error {
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, r.a)
	if e != nil {
		return e
	}
	if !r.expiry.After(facts.CheckedAt) {
		return serverreturn.ErrInvalid
	}
	return returnFresh(ctx, tx, facts)
}

// FinishSecurityReturn runs at the fixed Hosted callback surface with its real
// original browser binding. It creates only a short-lived opaque return code.
func (o *OIDCLogin) FinishSecurityReturn(ctx context.Context, id, binding string) (serverreturn.Return, error) {
	var out serverreturn.Return
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	r, e := o.securityReturnTx(ctx, tx, id, binding)
	if e != nil {
		return out, e
	}
	if r.consumed.Valid {
		return out, serverreturn.ErrInvalid
	}
	code := ""
	if len(r.code) > 0 {
		if !r.codeExpiry.Valid || !r.codeExpiry.Time.After(time.Now()) {
			return out, serverreturn.ErrInvalid
		}
		code, e = o.Keys.Open(r.cipher, "security-return-code:"+id)
		if e != nil {
			return out, e
		}
	} else {
		code = security.RandomID("scr_")
		cipher, e := o.Keys.Seal(code, "security-return-code:"+id)
		if e != nil {
			return out, e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE identity_security_returns SET code_hash=$2,code_cipher=$3,code_expires_at=LEAST(expires_at,clock_timestamp()+interval '60 seconds') WHERE transaction_id=$1`, id, security.Hash(code), cipher); e != nil {
			return out, e
		}
	}
	if e = o.securityReturnFreshTx(ctx, tx, r); e != nil {
		return out, e
	}
	out.Location = r.c.Origin + serverreturn.ReturnPath + "#" + url.Values{"security": {code}, "state": {r.state}, "transaction": {id}}.Encode()
	if e = tx.Commit(); e != nil {
		return serverreturn.Return{}, e
	}
	return out, nil
}

// ConsumeSecurityReturn requires PKCE and a fresh signed confirmation made by
// the original admitted server account/profile/family. No LocalSession, Hosted
// family or recentProof is returned; the real completeSecurity path follows.
func (o *OIDCLogin) ConsumeSecurityReturn(ctx context.Context, id, binding, access, origin string, q serverreturn.SecurityReturnConsume) (serverreturn.SecurityReturnReceipt, error) {
	var out serverreturn.SecurityReturnReceipt
	if len(q.Code) != 47 || q.Code[:4] != "scr_" || len(q.Verifier) < 32 || len(q.Verifier) > 128 {
		return out, serverreturn.ErrInvalid
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	r, e := o.securityReturnTx(ctx, tx, id, binding)
	if e != nil {
		return out, e
	}
	if r.c.Origin != origin || !equalSecret(r.a.TokenHash, security.Hash(access)) || !equalSecret(r.code, security.Hash(q.Code)) || !r.codeExpiry.Valid || !r.codeExpiry.Time.After(time.Now()) || serverreturn.Challenge(q.Verifier) != r.challenge {
		return out, serverreturn.ErrInvalid
	}
	key, e := base64.RawURLEncoding.Strict().DecodeString(r.c.PublicKey)
	if e != nil {
		return out, serverreturn.ErrInvalid
	}
	confirmation, e := serverreturn.VerifySecurityConfirmation(q.Confirmation, key, r.digest, id, q.Code, time.Now())
	if e != nil {
		return out, e
	}
	request := o.Security.requestHash(q)
	if r.consumed.Valid {
		if !equalSecret(r.consumedHash, request) || !r.confirmationExpiry.Valid || !r.confirmationExpiry.Time.After(time.Now()) {
			return out, serverreturn.ErrInvalid
		}
	} else if _, e = tx.ExecContext(ctx, `UPDATE identity_security_returns SET consumed_at=clock_timestamp(),consume_hash=$2,confirmation_expires_at=LEAST(expires_at,code_expires_at,$3) WHERE transaction_id=$1`, id, request, confirmation.ExpiresAt); e != nil {
		return out, e
	}
	if e = o.securityReturnFreshTx(ctx, tx, r); e != nil {
		return out, e
	}
	out = serverreturn.SecurityReturnReceipt{TransactionID: id, OperationID: r.c.OperationID, AccountID: r.a.AccountID, FamilyID: r.a.FamilyID}
	if e = tx.Commit(); e != nil {
		return serverreturn.SecurityReturnReceipt{}, e
	}
	return out, nil
}

// Existing provider completions are unchanged. A transaction carrying the new
// Security purpose must first spend its exact original return continuation.
func (o *OIDCLogin) securityReturnAdmissionTx(ctx context.Context, tx *sql.Tx, id, binding string, required bool) error {
	if !required {
		return nil
	}
	var consumed sql.NullTime
	var expiry, confirmed sql.NullTime
	e := tx.QueryRowContext(ctx, `SELECT consumed_at,expires_at,confirmation_expires_at FROM identity_security_returns WHERE transaction_id=$1`, id).Scan(&consumed, &expiry, &confirmed)
	if errors.Is(e, sql.ErrNoRows) {
		return identity.ErrRecentAuth
	}
	if e != nil {
		return e
	}
	if !consumed.Valid || !expiry.Valid || !confirmed.Valid || !expiry.Time.After(time.Now()) || !confirmed.Time.After(time.Now()) {
		return identity.ErrRecentAuth
	}
	_, e = o.securityReturnTx(ctx, tx, id, binding)
	return e
}

type SecurityReturnCompleteInput struct {
	serverreturn.SecurityReturnConsume
	Factor string `json:"factor"`
}

// The return never renews the original Settings operation's horizon, including
// when the proof is issued near the end of the provider transaction.
func (o *OIDCLogin) boundSecurityReturnProofTx(ctx context.Context, tx *sql.Tx, id string, proof *identity.RecentProof, required bool) error {
	if !required {
		return nil
	}
	var expiry time.Time
	e := tx.QueryRowContext(ctx, `SELECT expires_at FROM identity_security_returns WHERE transaction_id=$1`, id).Scan(&expiry)
	if errors.Is(e, sql.ErrNoRows) {
		return identity.ErrRecentAuth
	}
	if e != nil {
		return e
	}
	e = tx.QueryRowContext(ctx, `UPDATE account_recent_proofs SET expires_at=LEAST(expires_at,$2) WHERE proof_hash=$1 AND $2>clock_timestamp() RETURNING expires_at`, security.Hash(proof.Proof), expiry).Scan(&proof.ExpiresAt)
	if errors.Is(e, sql.ErrNoRows) {
		return identity.ErrRecentAuth
	}
	return e
}

// A separate, purpose-closed account command bridges only this original OIDC
// transaction. It never exposes its private cookie binding or accepts login,
// linking, arbitrary intents or account/family selection from the browser.
func (o *OIDCLogin) CompleteSecurityReturnFlow(ctx context.Context, id, access, origin string, q SecurityReturnCompleteInput) (OIDCCompletion, error) {
	var empty OIDCCompletion
	if !serverreturn.Origin(origin) || len(id) > 128 || len(access) < 20 || len(access) > 128 || len(q.Factor) > 128 {
		return empty, identity.ErrInvalid
	}
	if e := o.Security.Limit(ctx, "security-return-complete", id, 30, 15*time.Minute); e != nil {
		return empty, e
	}
	var encrypted []byte
	e := o.Store.DB.QueryRowContext(ctx, `SELECT r.binding_cipher FROM identity_security_returns r JOIN identity_transactions t ON t.id=r.transaction_id WHERE r.transaction_id=$1 AND t.expected_access_hash=$2 AND t.mode='reauthenticate' AND t.session_mode='browser' AND t.purpose='password_set_or_change' AND t.intent_target=''`, id, security.Hash(access)).Scan(&encrypted)
	if e != nil {
		return empty, identity.ErrCredentials
	}
	binding, e := o.Keys.Open(encrypted, "security-return-binding:"+id)
	if e != nil {
		return empty, e
	}
	if _, e = o.ConsumeSecurityReturn(ctx, id, binding, access, origin, q.SecurityReturnConsume); e != nil {
		return empty, e
	}
	out, e := o.CompleteFlow(ctx, id, binding, access, OIDCCompleteInput{Factor: q.Factor}, false)
	if e != nil {
		return empty, e
	}
	if out.Session != nil || out.Challenge != nil || out.Result != nil || out.Proof == nil {
		return empty, identity.ErrRecentAuth
	}
	return out, nil
}
