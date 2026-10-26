package postgres

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
)

type SecurityReturnIntent struct {
	IntentID  string    `json:"intentId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// The locator is not authority. This reads only the already verified, sealed
// registered origin so the HTTP edge can select its exact independent cookie.
func (o *OIDCLogin) SecurityReturnIntentOrigin(ctx context.Context, id string) (string, error) {
	if len(id) > 128 {
		return "", identity.ErrInvalid
	}
	var cipher []byte
	if e := o.Store.DB.QueryRowContext(ctx, `SELECT input_cipher FROM account_security_return_intents WHERE id=$1 AND cancelled_at IS NULL AND expires_at>clock_timestamp()`, id).Scan(&cipher); e != nil {
		return "", identity.ErrCredentials
	}
	plain, e := o.Keys.Open(cipher, "security-return-intent:"+id)
	if e != nil {
		return "", e
	}
	var q serverreturn.SecurityReturnStart
	if json.Unmarshal([]byte(plain), &q) != nil {
		return "", identity.ErrInvalid
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(q.Context.Payload)
	if e != nil {
		return "", identity.ErrInvalid
	}
	var c serverreturn.SecurityContext
	if json.Unmarshal(raw, &c) != nil || !serverreturn.Origin(c.Origin) {
		return "", identity.ErrInvalid
	}
	return c.Origin, nil
}

// Cancellation names the captured intent, never the current browser binding
// or another/latest transaction. It cannot cancel unrelated provider work.
func (o *OIDCLogin) CancelSecurityReturnIntent(ctx context.Context, id, access, origin string) error {
	if len(id) > 128 || !serverreturn.Origin(origin) {
		return identity.ErrInvalid
	}
	a, e := o.Store.CaptureClaimAuthority(ctx, security.Hash(access))
	if e != nil {
		return e
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, a)
	if e != nil {
		return e
	}
	var cipher []byte
	var expected identity.CurrentFamilyExpectation
	expected.Mode = "browser"
	if e = tx.QueryRowContext(ctx, `SELECT input_cipher,account_id,family_id,account_epoch,family_generation,access_hash FROM account_security_return_intents WHERE id=$1 FOR UPDATE`, id).Scan(&cipher, &expected.AccountID, &expected.FamilyID, &expected.AuthEpoch, &expected.FamilyGeneration, &expected.TokenHash); e != nil {
		return identity.ErrCredentials
	}
	if expected.AccountID != a.AccountID || expected.FamilyID != a.FamilyID || expected.AuthEpoch != a.AuthEpoch || expected.FamilyGeneration != a.FamilyGeneration || !equalSecret(expected.TokenHash, a.TokenHash) {
		return identity.ErrCredentials
	}
	plain, e := o.Keys.Open(cipher, "security-return-intent:"+id)
	if e != nil {
		return e
	}
	var q serverreturn.SecurityReturnStart
	if json.Unmarshal([]byte(plain), &q) != nil {
		return identity.ErrInvalid
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(q.Context.Payload)
	if e != nil {
		return identity.ErrInvalid
	}
	var c serverreturn.SecurityContext
	if json.Unmarshal(raw, &c) != nil || c.Origin != origin {
		return identity.ErrCredentials
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_security_return_intents SET cancelled_at=COALESCE(cancelled_at,clock_timestamp()) WHERE id=$1`, id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE identity_security_returns SET cancelled_at=COALESCE(cancelled_at,clock_timestamp()) WHERE intent_id=$1`, id); e != nil {
		return e
	}
	if e = returnFresh(ctx, tx, facts); e != nil {
		return e
	}
	return tx.Commit()
}

// PrepareSecurityReturnIntent is an authenticated account command, not an OIDC
// endpoint. It is allowed only from the exact signed registered server origin.
// Its opaque locator carries no credential and cannot by itself start OIDC.
func (o *OIDCLogin) PrepareSecurityReturnIntent(ctx context.Context, access, origin, audience string, q serverreturn.SecurityReturnStart) (SecurityReturnIntent, error) {
	var out SecurityReturnIntent
	if !q.Valid() || !serverreturn.Origin(origin) {
		return out, identity.ErrInvalid
	}
	a, e := o.Store.CaptureClaimAuthority(ctx, security.Hash(access))
	if e != nil {
		return out, e
	}
	if a.Mode != "browser" {
		return out, identity.ErrCredentials
	}
	if e = o.Security.Limit(ctx, "security-return-prepare", a.AccountID, 10, 15*time.Minute); e != nil {
		return out, e
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, a)
	if e != nil {
		return out, e
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(q.Context.Payload)
	var locator struct {
		ServerID string `json:"serverId"`
	}
	if e != nil || len(raw) > 6144 || json.Unmarshal(raw, &locator) != nil || len(locator.ServerID) > 128 {
		return out, identity.ErrInvalid
	}
	var encoded string
	if e = tx.QueryRowContext(ctx, `SELECT public_key FROM servers WHERE id=$1 AND revoked_at IS NULL AND NOT custody_released`, locator.ServerID).Scan(&encoded); e != nil {
		return out, serverreturn.ErrInvalid
	}
	key, e := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if e != nil {
		return out, serverreturn.ErrInvalid
	}
	c, _, e := serverreturn.VerifySecurityContext(q.Context, key, facts.CheckedAt)
	if e != nil || c.Origin != origin || c.HostedOrigin != audience || c.AccountID != a.AccountID {
		return out, serverreturn.ErrInvalid
	}
	if c.Purpose == "email_change" {
		normalized, e := NormalizeContact(c.Target)
		if e != nil || normalized != c.Target {
			return out, identity.ErrInvalid
		}
	}
	var live bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_external_identities i JOIN accounts a ON a.id=i.account_id WHERE i.id=$1 AND i.account_id=$2 AND i.revision=$3 AND i.revoked_at IS NULL AND a.password_hash IS NULL)`, c.ProviderIdentityID, a.AccountID, c.ProviderRevision).Scan(&live); e != nil {
		return out, e
	}
	if !live {
		return out, identity.ErrRecentAuth
	}
	registered, e := returnTupleTx(ctx, tx, a.AccountID, c.ServerID, c.ClaimOperationID, "sign-in", c.RegistrationID)
	if e != nil || registered.origin != origin {
		return out, serverreturn.ErrInvalid
	}
	if e = tx.QueryRowContext(ctx, `SELECT public_key FROM servers WHERE id=$1`, c.ServerID).Scan(&encoded); e != nil || encoded != c.PublicKey {
		return out, serverreturn.ErrInvalid
	}
	request := o.Security.requestHash(q)
	var prior, hash []byte
	var account, family string
	var epoch, generation int64
	e = tx.QueryRowContext(ctx, `SELECT id,expires_at,access_hash,request_hash,account_id,family_id,account_epoch,family_generation FROM account_security_return_intents WHERE account_id=$1 AND operation_id=$2 AND cancelled_at IS NULL FOR UPDATE`, a.AccountID, c.OperationID).Scan(&out.IntentID, &out.ExpiresAt, &prior, &hash, &account, &family, &epoch, &generation)
	if e == nil {
		if !equalSecret(prior, a.TokenHash) || !equalSecret(hash, request) || account != a.AccountID || family != a.FamilyID || epoch != a.AuthEpoch || generation != a.FamilyGeneration || !out.ExpiresAt.After(facts.CheckedAt) {
			return SecurityReturnIntent{}, identity.ErrConflict
		}
		if e = returnFresh(ctx, tx, facts); e != nil {
			return SecurityReturnIntent{}, e
		}
		return out, tx.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	out.IntentID = security.RandomID("sri_")
	out.ExpiresAt = c.ExpiresAt
	accessCipher, e := o.Keys.Seal(access, "security-return-access:"+out.IntentID)
	if e != nil {
		return out, e
	}
	input, _ := json.Marshal(q)
	inputCipher, e := o.Keys.Seal(string(input), "security-return-intent:"+out.IntentID)
	if e != nil {
		return out, e
	}
	result, e := tx.ExecContext(ctx, `INSERT INTO account_security_return_intents(id,account_id,family_id,account_epoch,family_generation,access_hash,operation_id,request_hash,access_cipher,input_cipher,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(account_id,operation_id) DO NOTHING`, out.IntentID, a.AccountID, a.FamilyID, a.AuthEpoch, a.FamilyGeneration, a.TokenHash, c.OperationID, request, accessCipher, inputCipher, out.ExpiresAt)
	if e != nil {
		return out, e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return out, e
	}
	if n != 1 {
		return SecurityReturnIntent{}, identity.ErrConflict
	}
	if e = returnFresh(ctx, tx, facts); e != nil {
		return out, e
	}
	return out, tx.Commit()
}

// StartSecurityReturnFlow checks the real existing HttpOnly refresh cookie,
// without rotating it or issuing any credential. Only then may the original
// finite access credential be unsealed for the existing OIDC reauthenticate.
func (o *OIDCLogin) StartSecurityReturnFlow(ctx context.Context, id, binding, refresh, requestID string) (OIDCView, string, error) {
	var empty OIDCView
	if len(id) > 128 || len(binding) != 43 || len(refresh) < 20 || len(refresh) > 128 {
		return empty, "", identity.ErrCredentials
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return empty, "", e
	}
	defer tx.Rollback()
	var a identity.CurrentFamilyExpectation
	var encrypted, input []byte
	var expiry time.Time
	a.Mode = "browser"
	e = tx.QueryRowContext(ctx, `SELECT account_id,family_id,account_epoch,family_generation,access_hash,access_cipher,input_cipher,expires_at FROM account_security_return_intents WHERE id=$1 AND cancelled_at IS NULL`, id).Scan(&a.AccountID, &a.FamilyID, &a.AuthEpoch, &a.FamilyGeneration, &a.TokenHash, &encrypted, &input, &expiry)
	if e != nil {
		return empty, "", identity.ErrCredentials
	}
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, a)
	if e != nil {
		return empty, "", e
	}
	var current bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM session_families f JOIN refresh_tokens t ON t.family_id=f.id AND t.generation=f.generation AND t.token_hash=f.current_refresh_hash WHERE f.id=$1 AND f.account_id=$2 AND f.generation=$3 AND t.token_hash=$4 AND t.consumed_at IS NULL AND f.revoked_at IS NULL)`, a.FamilyID, a.AccountID, a.FamilyGeneration, security.Hash(refresh)).Scan(&current)
	if e != nil {
		return empty, "", e
	}
	if !current || !expiry.After(facts.CheckedAt) {
		return empty, "", identity.ErrCredentials
	}

	var originalRequest sql.NullString
	var originalBinding []byte
	if e = tx.QueryRowContext(ctx, `SELECT start_request_id,start_binding_hash FROM account_security_return_intents WHERE id=$1 FOR UPDATE`, id).Scan(&originalRequest, &originalBinding); e != nil {
		return empty, "", e
	}
	if originalRequest.Valid {
		if originalRequest.String != requestID || !equalSecret(originalBinding, security.Hash(binding)) {
			return empty, "", identity.ErrConflict
		}
	} else {
		if len(requestID) < 16 || len(requestID) > 128 {
			return empty, "", identity.ErrInvalid
		}
		if _, e = tx.ExecContext(ctx, `UPDATE account_security_return_intents SET start_request_id=$2,start_binding_hash=$3 WHERE id=$1`, id, requestID, security.Hash(binding)); e != nil {
			return empty, "", e
		}
	}
	plain, e := o.Keys.Open(input, "security-return-intent:"+id)
	if e != nil {
		return empty, "", e
	}
	var q serverreturn.SecurityReturnStart
	if json.Unmarshal([]byte(plain), &q) != nil || !q.Valid() {
		return empty, "", identity.ErrInvalid
	}
	access, e := o.Keys.Open(encrypted, "security-return-access:"+id)
	if e != nil || !equalSecret(a.TokenHash, security.Hash(access)) {
		return empty, "", identity.ErrCredentials
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(q.Context.Payload)
	if e != nil {
		return empty, "", identity.ErrInvalid
	}
	var c serverreturn.SecurityContext
	if json.Unmarshal(raw, &c) != nil || c.AccountID != a.AccountID {
		return empty, "", identity.ErrInvalid
	}
	var provider string
	if e = tx.QueryRowContext(ctx, `SELECT provider FROM account_external_identities WHERE id=$1 AND account_id=$2 AND revision=$3 AND revoked_at IS NULL`, c.ProviderIdentityID, a.AccountID, c.ProviderRevision).Scan(&provider); e != nil {
		return empty, "", identity.ErrRecentAuth
	}
	if e = returnFresh(ctx, tx, facts); e != nil {
		return empty, "", e
	}
	if e = tx.Commit(); e != nil {
		return empty, "", e
	}
	v, location, _, e := o.StartFlow(ctx, provider, binding, access, OIDCStartInput{Mode: "reauthenticate", SessionMode: "browser", RequestID: requestID, Intent: &identity.SecurityIntent{Purpose: c.Purpose, Target: c.Target, OperationID: c.OperationID}, SecurityReturn: &q, SecurityReturnExpectation: &a, SecurityReturnIntentID: id})
	return v, location, e
}
