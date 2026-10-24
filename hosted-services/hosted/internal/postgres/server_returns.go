package postgres

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/url"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
	"portico.local/hosted/trust"
	"time"
)

type returnTuple struct {
	operation, origin, purpose, current, previous string
	claim, credential, route                      int64
}

// ServerReturnRegistration resolves existing verified location evidence. It
// cannot register an origin, admit an account or authorize an ownership claim.
func (s *Store) ServerReturnRegistration(ctx context.Context, a directory.ServerAuthority, q serverreturn.RegistrationRequest) (serverreturn.Registration, error) {
	var out serverreturn.Registration
	if !serverreturn.Origin(q.Origin) {
		return out, serverreturn.ErrInvalid
	}
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	var generation int64
	if e = tx.QueryRowContext(ctx, `SELECT generation FROM networking_route_state WHERE server_id=$1 AND operation_id=$2 AND claim_generation=$3 AND credential_generation=$4 FOR SHARE`, a.ServerID, a.OperationID, a.ClaimGeneration, a.CredentialGeneration).Scan(&generation); e != nil {
		return out, serverreturn.ErrInvalid
	}
	id := serverreturn.RegistrationID(a.ServerID, a.OperationID, a.ClaimGeneration, a.CredentialGeneration, generation, q.Origin)
	t, e := returnTupleTx(ctx, tx, a.AccountID, a.ServerID, a.OperationID, "sign-in", id)
	if e != nil || t.origin != q.Origin {
		return out, serverreturn.ErrInvalid
	}
	if e = checkServerAuthorityTx(ctx, tx, a); e != nil {
		return out, e
	}
	out = serverreturn.Registration{ID: id, Origin: t.origin}
	if e = bindSetupRegistrationTx(ctx, tx, a, out, t); e != nil {
		return out, e
	}
	if e = bindOwnerRegistrationTx(ctx, tx, a, out, t); e != nil {
		return serverreturn.Registration{}, e
	}
	return out, tx.Commit()
}

// Caller holds the account/family locks first. The same server lock used by
// withdrawal, check-in and retirement serializes the entire positive consume.
func returnTupleTx(ctx context.Context, tx *sql.Tx, account, server, operation, purpose, registration string) (returnTuple, error) {
	var t returnTuple
	var owner string
	e := tx.QueryRowContext(ctx, `SELECT owner_account_id,claim_generation,credential_generation FROM servers WHERE id=$1 AND revoked_at IS NULL AND NOT custody_released FOR UPDATE`, server).Scan(&owner, &t.claim, &t.credential)
	if e != nil {
		return t, serverreturn.ErrInvalid
	}
	e = tx.QueryRowContext(ctx, `SELECT o.id,o.purpose,o.current_owner_account_id,o.previous_owner_role FROM claim_operations o JOIN servers s ON s.id=o.server_id WHERE ($1='' OR o.id=$1) AND o.server_id=$2 AND o.account_id=$3 AND o.claim_generation=$4 AND o.credential_generation=$5 AND o.state='installed' AND o.committed_token_hash=s.token_hash AND s.public_key=translate(rtrim(encode(o.public_key,'base64'),'='),'+/','-_') FOR SHARE OF o`, operation, server, owner, t.claim, t.credential).Scan(&t.operation, &t.purpose, &t.current, &t.previous)
	if e != nil {
		return t, serverreturn.ErrInvalid
	}
	if purpose == "sign-in" {
		// Identity is not admission. Waiting accounts have no discovery row yet;
		// the server alone admits them or redeems its separate local move intent.
		t.purpose, t.current, t.previous = purpose, "", ""
	} else {
		if t.purpose != purpose || account != owner {
			return t, serverreturn.ErrInvalid
		}
		var role string
		if e = tx.QueryRowContext(ctx, `SELECT role FROM server_members WHERE server_id=$1 AND account_id=$2 FOR SHARE`, server, account).Scan(&role); e != nil || role != "owner" {
			return t, serverreturn.ErrInvalid
		}
	}

	e = tx.QueryRowContext(ctx, `SELECT generation FROM networking_route_state WHERE server_id=$1 AND operation_id=$2 AND claim_generation=$3 AND credential_generation=$4 FOR SHARE`, server, t.operation, t.claim, t.credential).Scan(&t.route)
	if e != nil {
		return t, serverreturn.ErrInvalid
	}
	rows, e := tx.QueryContext(ctx, `SELECT c.origin FROM networking_route_candidates c JOIN networking_route_state n USING(server_id) WHERE c.server_id=$1 AND c.operation_id=$2 AND c.generation=$3 AND c.state='verified' AND (n.remote_enabled OR c.class='lan') ORDER BY c.origin LIMIT 65 FOR SHARE OF c`, server, t.operation, t.route)
	if e != nil {
		return t, e
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var origin string
		if e = rows.Scan(&origin); e != nil {
			return t, e
		}
		count++
		if count > 64 {
			return t, serverreturn.ErrInvalid
		}
		if serverreturn.Origin(origin) && serverreturn.RegistrationID(server, t.operation, t.claim, t.credential, t.route, origin) == registration {
			t.origin = origin
		}
	}
	if e = rows.Err(); e != nil {
		return t, e
	}
	if t.origin == "" {
		return t, serverreturn.ErrInvalid
	}
	return t, nil
}
func (s *Store) StartServerReturn(ctx context.Context, a identity.CurrentFamilyExpectation, q serverreturn.Start) (serverreturn.Started, error) {
	var out serverreturn.Started
	if !q.Valid() {
		return out, serverreturn.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, a)
	if e != nil {
		return out, e
	}
	t, e := returnTupleTx(ctx, tx, a.AccountID, q.ServerID, q.ClaimOperationID, q.Purpose, q.RegistrationID)
	if e != nil {
		return out, e
	}
	var count int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM server_returns WHERE account_id=$1 AND created_at>clock_timestamp()-interval '1 minute'`, a.AccountID).Scan(&count); e != nil {
		return out, e
	}
	if count >= 10 {
		return out, identity.ErrRateLimited
	}
	out.TransactionID = security.RandomID("srt_")
	_, e = tx.ExecContext(ctx, `INSERT INTO server_returns(id,account_id,family_id,access_hash,account_epoch,family_generation,session_mode,server_id,operation_id,claim_generation,credential_generation,route_generation,registration_id,origin,purpose,current_owner_account_id,previous_owner_role,state,code_challenge,server_challenge,installation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`, out.TransactionID, a.AccountID, a.FamilyID, a.TokenHash, a.AuthEpoch, a.FamilyGeneration, a.Mode, q.ServerID, t.operation, t.claim, t.credential, t.route, q.RegistrationID, t.origin, q.Purpose, t.current, t.previous, q.State, q.CodeChallenge, q.ServerChallenge, q.InstallationID)
	if e != nil {
		return serverreturn.Started{}, e
	}
	if e = returnFresh(ctx, tx, facts); e != nil {
		return serverreturn.Started{}, e
	}
	return out, tx.Commit()
}
func returnFresh(ctx context.Context, tx *sql.Tx, f identity.CurrentFamilyFacts) error {
	var now time.Time
	if e := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return e
	}
	if !f.UnexpiredAt(now) {
		return serverreturn.ErrInvalid
	}
	return ctx.Err()
}

type returnRow struct {
	a        identity.CurrentFamilyExpectation
	q        serverreturn.Start
	t        returnTuple
	code     []byte
	expiry   time.Time
	consumed sql.NullTime
}

func readReturn(ctx context.Context, tx *sql.Tx, id string, lock bool) (returnRow, error) {
	var r returnRow
	query := `SELECT account_id,family_id,access_hash,account_epoch,family_generation,session_mode,server_id,operation_id,purpose,registration_id,state,code_challenge,server_challenge,installation_id,claim_generation,credential_generation,route_generation,origin,current_owner_account_id,previous_owner_role,code_hash,LEAST(expires_at,COALESCE(code_expires_at,expires_at)),consumed_at FROM server_returns WHERE id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	e := tx.QueryRowContext(ctx, query, id).Scan(&r.a.AccountID, &r.a.FamilyID, &r.a.TokenHash, &r.a.AuthEpoch, &r.a.FamilyGeneration, &r.a.Mode, &r.q.ServerID, &r.q.ClaimOperationID, &r.q.Purpose, &r.q.RegistrationID, &r.q.State, &r.q.CodeChallenge, &r.q.ServerChallenge, &r.q.InstallationID, &r.t.claim, &r.t.credential, &r.t.route, &r.t.origin, &r.t.current, &r.t.previous, &r.code, &r.expiry, &r.consumed)
	if e != nil {
		return r, serverreturn.ErrInvalid
	}
	return r, nil
}
func sameReturnTuple(a, b returnTuple) bool {
	return a.claim == b.claim && a.credential == b.credential && a.route == b.route && a.origin == b.origin && a.current == b.current && a.previous == b.previous
}
func (s *Store) CompleteServerReturn(ctx context.Context, a identity.CurrentFamilyExpectation, id string) (serverreturn.Return, error) {
	var out serverreturn.Return
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, a)
	if e != nil {
		return out, e
	}
	setup, e := readSetupStart(ctx, tx, id, false, false)
	if e == nil {
		return completeSetupReturnTx(ctx, tx, a, setup, facts)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	owner, e := readOwnerStart(ctx, tx, id, false, false)
	if e == nil {
		return completeOwnerReturnTx(ctx, tx, a, owner, facts)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	pre, e := readReturn(ctx, tx, id, false)
	if e != nil || pre.a.AccountID != a.AccountID || pre.a.FamilyID != a.FamilyID || pre.a.AuthEpoch != a.AuthEpoch || pre.a.FamilyGeneration != a.FamilyGeneration || subtle.ConstantTimeCompare(pre.a.TokenHash, a.TokenHash) != 1 {
		return out, serverreturn.ErrInvalid
	}
	t, e := returnTupleTx(ctx, tx, a.AccountID, pre.q.ServerID, pre.q.ClaimOperationID, pre.q.Purpose, pre.q.RegistrationID)
	if e != nil || !sameReturnTuple(t, pre.t) {
		return out, serverreturn.ErrInvalid
	}
	r, e := readReturn(ctx, tx, id, true)
	if e != nil || len(r.code) != 0 || r.consumed.Valid {
		return out, serverreturn.ErrInvalid
	}
	code := security.RandomID("src_")
	result, e := tx.ExecContext(ctx, `UPDATE server_returns SET code_hash=$2,code_expires_at=LEAST(expires_at,clock_timestamp()+interval '60 seconds') WHERE id=$1 AND code_hash IS NULL AND expires_at>clock_timestamp()`, id, security.Hash(code))
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
	out.Location = t.origin + serverreturn.ReturnPath + "#" + url.Values{"code": {code}, "state": {r.q.State}, "transaction": {id}}.Encode()
	if e = tx.Commit(); e != nil {
		return serverreturn.Return{}, e
	}
	return out, nil
}
func (s *Store) ExchangeServerReturn(ctx context.Context, q serverreturn.Exchange, sign func(serverreturn.Assertion) (trust.Envelope, error)) (serverreturn.Grant, error) {
	var out serverreturn.Grant
	if !q.Valid() || sign == nil {
		return out, serverreturn.ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	pre, e := readReturn(ctx, tx, q.TransactionID, false)
	if e != nil {
		return out, e
	}
	facts, e := AuthorizeCurrentFamilyTx(ctx, tx, pre.a)
	if e != nil {
		return out, serverreturn.ErrInvalid
	}
	t, e := returnTupleTx(ctx, tx, pre.a.AccountID, pre.q.ServerID, pre.q.ClaimOperationID, pre.q.Purpose, pre.q.RegistrationID)
	if e != nil || !sameReturnTuple(t, pre.t) {
		return out, serverreturn.ErrInvalid
	}
	r, e := readReturn(ctx, tx, q.TransactionID, true)
	if e != nil || r.consumed.Valid || subtle.ConstantTimeCompare(r.code, security.Hash(q.Code)) != 1 || subtle.ConstantTimeCompare([]byte(r.q.CodeChallenge), []byte(serverreturn.Challenge(q.Verifier))) != 1 || q.RegistrationID != r.q.RegistrationID || q.ServerChallenge != r.q.ServerChallenge || q.InstallationID != r.q.InstallationID || q.Purpose != r.q.Purpose {
		return out, serverreturn.ErrInvalid
	}
	var username, displayName, email string
	if e = tx.QueryRowContext(ctx, `SELECT username,display_name,COALESCE((SELECT address FROM account_contact_emails WHERE account_id=$1 AND is_primary AND verified_at IS NOT NULL),'') FROM accounts WHERE id=$1`, r.a.AccountID).Scan(&username, &displayName, &email); e != nil {
		return out, e
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	if !r.expiry.After(now) || !facts.UnexpiredAt(now) {
		return out, serverreturn.ErrInvalid
	}
	out.Assertion, e = sign(serverreturn.Assertion{Kind: serverreturn.AssertionKind, AccountID: r.a.AccountID, Username: username, DisplayName: displayName, Email: email, ServerID: r.q.ServerID, Challenge: r.q.ServerChallenge, Nonce: security.RandomID(""), IssuedAt: now.UTC(), ExpiresAt: now.UTC().Add(serverreturn.AssertionLifetime), Purpose: r.q.Purpose, InstallationID: r.q.InstallationID, RegistrationID: r.q.RegistrationID, ClaimOperationID: r.q.ClaimOperationID, ClaimGeneration: t.claim, CredentialGeneration: t.credential, CurrentOwnerAccountID: t.current, PreviousOwnerRole: t.previous})
	if e != nil {
		return serverreturn.Grant{}, e
	}
	result, e := tx.ExecContext(ctx, `UPDATE server_returns SET consumed_at=clock_timestamp() WHERE id=$1 AND consumed_at IS NULL AND code_expires_at>clock_timestamp() AND expires_at>clock_timestamp() AND $2>clock_timestamp() AND $3>clock_timestamp() AND $4>clock_timestamp()`, q.TransactionID, facts.AccessExpiresAt, facts.AbsoluteExpiresAt, facts.IdleExpiresAt)
	if e != nil {
		return serverreturn.Grant{}, e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return serverreturn.Grant{}, serverreturn.ErrInvalid
	}
	if e = tx.Commit(); e != nil {
		return serverreturn.Grant{}, e
	}
	return out, nil
}
