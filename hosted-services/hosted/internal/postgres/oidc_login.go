package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/security"
	"time"
)

type OIDCLogin struct {
	Store     *Store
	Keys      *security.Keys
	Providers map[string]*oidclogin.Provider
	Lifetime  time.Duration
	Security  *AccountSecurity
}
type OIDCView struct {
	ID              string    `json:"transactionId"`
	Status          string    `json:"status"`
	Provider        string    `json:"provider"`
	ContactVerified bool      `json:"contactVerified"`
	Expires         time.Time `json:"expiresAt"`
	NeedsUsername   bool      `json:"needsUsername"`
	Mode            string    `json:"mode"`
	OnboardingToken string    `json:"onboardingToken,omitempty"`
	SuggestedName   string    `json:"suggestedName,omitempty"`
	MFARequired     bool      `json:"mfaRequired,omitempty"`
	SecurityReturn  bool      `json:"securityReturn,omitempty"`
	VerifiedEmail   string    `json:"verifiedEmail,omitempty"`
}
type oidcRow struct {
	ID, Provider, Status, Descriptor, Marker         string
	Generation, Revision, Policy, DescriptorRevision int64
	Expires                                          time.Time
	Nonce                                            []byte
	Mode, SessionMode                                string
	NativeApple                                      bool
	// NativeCompletion: the client opted into the completion-code return.
	NativeCompletion bool
	SecurityReturn   bool
}

// NativeCompletionCode is the value GET /v1/identity/options advertises as
// nativeCompletion and a client echoes as completion to opt in.
const NativeCompletionCode = "code"

func (o *OIDCLogin) Configure(ctx context.Context) error {
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1941002)`); e != nil {
		return e
	}
	marker := hex.EncodeToString(security.Hash(string(o.Keys.SealKey)))
	if _, e = tx.ExecContext(ctx, `UPDATE identity_protocol_policy SET enabled=true,marker_key_id=$1,revision=revision+1,updated_at=clock_timestamp() WHERE NOT enabled OR marker_key_id IS DISTINCT FROM $1`, marker); e != nil {
		return e
	}
	for _, id := range []string{"google", "apple"} {
		p := o.Providers[id]
		if p == nil {
			if _, e = tx.ExecContext(ctx, `UPDATE identity_provider_descriptors SET enabled=false,revision=revision+1 WHERE provider=$1 AND enabled`, id); e != nil {
				return e
			}
			continue
		}
		descriptor := hex.EncodeToString(security.Hash(p.Issuer + "\x00" + p.Audience + "\x00" + p.Callback + "\x00" + p.NativeAudience))
		if _, e = tx.ExecContext(ctx, `UPDATE identity_provider_descriptors SET enabled=false,revision=revision+1 WHERE provider=$1 AND enabled AND id<>$2`, id, descriptor); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO identity_provider_descriptors(id,provider,revision,enabled,issuer,audience,callback_uri) VALUES($1,$2,1,true,$3,$4,$5) ON CONFLICT(id) DO UPDATE SET enabled=true,revision=identity_provider_descriptors.revision+1 WHERE NOT identity_provider_descriptors.enabled`, descriptor, id, p.Issuer, p.Audience, p.Callback); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (o *OIDCLogin) Start(ctx context.Context, provider, binding string) (OIDCView, string, error) {
	v, url, _, e := o.StartFlow(ctx, provider, binding, "", OIDCStartInput{Mode: "login", SessionMode: "browser", RequestID: security.RandomID("")})
	return v, url, e
}
func (o *OIDCLogin) lock(ctx context.Context, tx *sql.Tx, id, binding string) (oidcRow, error) {
	return o.lockHash(ctx, tx, id, security.Hash(binding))
}
func (o *OIDCLogin) lockHash(ctx context.Context, tx *sql.Tx, id string, binding []byte) (oidcRow, error) {
	return lockOIDCTransactionHash(ctx, tx, id, binding)
}

// Shared by ordinary provider completion and its MFA continuation. Callers
// acquire the binding before account locks, then recheck its full descriptor,
// policy and transaction authority while holding the completion locks.
func lockOIDCTransactionHash(ctx context.Context, tx *sql.Tx, id string, binding []byte) (oidcRow, error) {
	var r oidcRow
	// Binding lock precedes transaction/account authority locks everywhere.
	var generation int64
	e := tx.QueryRowContext(ctx, `SELECT generation FROM identity_browser_bindings WHERE binding_hash=$1 AND expires_at>clock_timestamp() FOR SHARE`, binding).Scan(&generation)
	if e != nil {
		return r, currentFamilyError(ctx, e)
	}
	e = tx.QueryRowContext(ctx, `SELECT t.id,t.provider,t.status,t.descriptor_id,t.marker_key_id,t.binding_generation,t.revision,t.policy_revision,t.descriptor_revision,t.expires_at,t.nonce_hash,t.mode,t.session_mode,t.native_apple,t.native_completion,t.security_return FROM identity_transactions t JOIN identity_protocol_policy p ON p.singleton JOIN identity_provider_descriptors d ON d.id=t.descriptor_id WHERE t.id=$1 AND t.binding_hash=$2 AND t.binding_generation=$3 AND t.expires_at>clock_timestamp() AND p.enabled AND p.revision=t.policy_revision AND p.marker_key_id=t.marker_key_id AND d.enabled AND d.revision=t.descriptor_revision FOR UPDATE OF t FOR SHARE OF p,d`, id, binding, generation).Scan(&r.ID, &r.Provider, &r.Status, &r.Descriptor, &r.Marker, &r.Generation, &r.Revision, &r.Policy, &r.DescriptorRevision, &r.Expires, &r.Nonce, &r.Mode, &r.SessionMode, &r.NativeApple, &r.NativeCompletion, &r.SecurityReturn)
	if errors.Is(e, sql.ErrNoRows) {
		e = identity.ErrCredentials
	}
	return r, e
}

// OIDCCallback is delivered only in the provider callback response. In
// particular, CompletionCode must never be returned from View or StartFlow.
type OIDCCallback struct {
	TransactionID  string
	SessionMode    string
	CompletionCode string // only for transactions that opted in (NativeCompletion)
}

func (o *OIDCLogin) Callback(ctx context.Context, provider, binding, state, code string) (OIDCCallback, error) {
	return o.CallbackWithName(ctx, provider, binding, state, code, "")
}
func (o *OIDCLogin) CallbackWithName(ctx context.Context, provider, binding, state, code, suggestedName string) (OIDCCallback, error) {
	if len(state) > 128 || len(code) > 8192 || state == "" || code == "" {
		return OIDCCallback{}, identity.ErrInvalid
	}
	var id, mode string
	var boundHash []byte
	if e := o.Store.DB.QueryRowContext(ctx, `SELECT id,session_mode,binding_hash FROM identity_transactions WHERE state_hash=$1 AND provider=$2 AND NOT native_apple`, security.Hash(state), provider).Scan(&id, &mode, &boundHash); e != nil {
		return OIDCCallback{}, identity.ErrCredentials
	}
	if mode == "browser" && !equalSecret(boundHash, security.Hash(binding)) {
		return OIDCCallback{}, identity.ErrCredentials
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return OIDCCallback{}, e
	}
	defer tx.Rollback()
	r, e := o.lockHash(ctx, tx, id, boundHash)
	if e != nil {
		return OIDCCallback{}, e
	}
	if r.Status == "verified" || r.Status == "completed" || r.Status == "proof_issued" {
		if mode == "browser" {
			return OIDCCallback{TransactionID: id, SessionMode: mode}, tx.Commit()
		}
		return OIDCCallback{}, identity.ErrCredentials
	}
	if r.Status != "started" {
		return OIDCCallback{}, identity.ErrCredentials
	}
	var sealed []byte
	if e = tx.QueryRowContext(ctx, `SELECT sealed_verifier FROM identity_exchange_secrets WHERE transaction_id=$1`, id).Scan(&sealed); e != nil {
		return OIDCCallback{}, e
	}
	verifier, e := o.Keys.Open(sealed, id)
	if e != nil {
		return OIDCCallback{}, e
	}
	lease := security.RandomID("lease_")
	if _, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET status='exchanging',revision=revision+1 WHERE id=$1`, id); e != nil {
		return OIDCCallback{}, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO identity_exchange_claims(transaction_id,transaction_expires_at,lease_id,transaction_revision,binding_generation,lease_until) VALUES($1,$2,$3,$4,$5,LEAST(statement_timestamp()+interval '30 seconds',$2))`, id, r.Expires, lease, r.Revision+1, r.Generation); e != nil {
		return OIDCCallback{}, e
	}
	if e = tx.Commit(); e != nil {
		return OIDCCallback{}, e
	}
	// No database lock or connection remains held during provider I/O.
	a, exchangeErr := o.Providers[provider].Exchange(ctx, code, verifier)
	tx, e = o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return OIDCCallback{}, e
	}
	defer tx.Rollback()
	r, e = o.lockHash(ctx, tx, id, boundHash)
	if e != nil {
		return OIDCCallback{}, e
	}
	var leaseOK bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM identity_exchange_claims WHERE transaction_id=$1 AND lease_id=$2 AND lease_until>clock_timestamp())`, id, lease).Scan(&leaseOK); e != nil {
		return OIDCCallback{}, e
	}
	if r.Status != "exchanging" || !leaseOK {
		return OIDCCallback{}, identity.ErrCredentials
	}
	if exchangeErr != nil || !bytes.Equal(security.Hash(a.Nonce), r.Nonce) {
		_, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET status='failed',terminal_at=clock_timestamp(),revision=revision+1 WHERE id=$1`, id)
		if e == nil {
			_, e = tx.ExecContext(ctx, `DELETE FROM identity_exchange_secrets WHERE transaction_id=$1`, id)
		}
		if e == nil {
			e = tx.Commit()
		}
		return OIDCCallback{}, identity.ErrCredentials
	}
	var email any
	if a.EmailVerified {
		email = a.Email
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,private_relay,transaction_revision,lease_id,suggested_name) VALUES($1,$2,$3,$4,$5,clock_timestamp(),$6,$7,$8,$9,$10,$11,$12)`, id, a.Issuer, a.Subject, a.Audience, a.IssuedAt, r.Nonce, email, a.EmailVerified, a.PrivateRelay && a.EmailVerified, r.Revision, lease, cleanProviderName(suggestedName)); e != nil {
		return OIDCCallback{}, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET status='verified',revision=revision+1 WHERE id=$1`, id); e != nil {
		return OIDCCallback{}, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM identity_exchange_secrets WHERE transaction_id=$1`, id); e != nil {
		return OIDCCallback{}, e
	}
	result := OIDCCallback{TransactionID: id, SessionMode: mode}
	if mode == "native" && r.NativeCompletion {
		result.CompletionCode = security.RandomID("")
		if _, e = tx.ExecContext(ctx, `INSERT INTO identity_native_completions(transaction_id,code_hash,expires_at) VALUES($1,$2,LEAST(clock_timestamp()+interval '2 minutes',$3))`, id, security.Hash(result.CompletionCode), r.Expires); e != nil {
			return OIDCCallback{}, e
		}
	}
	return result, tx.Commit()
}

// Complete retains the repository's password-free session interface for older
// internal callers. HTTP and all clients use CompleteFlow and handle MFA.
func (o *OIDCLogin) Complete(ctx context.Context, id, binding, username, name string) (identity.Session, error) {
	out, e := o.CompleteFlow(ctx, id, binding, "", OIDCCompleteInput{Username: username, DisplayName: name}, false)
	if e != nil {
		return identity.Session{}, e
	}
	if out.Session == nil {
		return identity.Session{}, identity.ErrChallenge
	}
	return *out.Session, nil
}
func (o *OIDCLogin) Cancel(ctx context.Context, binding string) error {
	_, e := o.Store.DB.ExecContext(ctx, `UPDATE identity_browser_bindings SET generation=generation+1 WHERE binding_hash=$1`, security.Hash(binding))
	return e
}

// Prune is bounded independently of the number of sign-in attempts.
func (o *OIDCLogin) Prune(ctx context.Context) error {
	return o.Store.pruneStatements(ctx, []string{
		`DELETE FROM identity_transactions WHERE id IN(SELECT id FROM identity_transactions WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT 100)`,
		`DELETE FROM identity_browser_bindings WHERE binding_hash IN(SELECT binding_hash FROM identity_browser_bindings WHERE expires_at<clock_timestamp() ORDER BY expires_at LIMIT 100)`,
	})
}
