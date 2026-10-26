package postgres

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
)

// The public transaction identifier is not a credential. Every continuation
// also requires the original HttpOnly cookie or native private binding.
type OIDCStartInput struct {
	Mode                      string                             `json:"mode"`
	SessionMode               string                             `json:"sessionMode"`
	RequestID                 string                             `json:"requestId"`
	Intent                    *identity.SecurityIntent           `json:"intent,omitempty"`
	ExpectedIdentity          *ExpectedProviderIdentity          `json:"expectedIdentity,omitempty"`
	Proof                     string                             `json:"proof,omitempty"`
	SecurityReturn            *serverreturn.SecurityReturnStart  `json:"-"`
	SecurityReturnExpectation *identity.CurrentFamilyExpectation `json:"-"`
	SecurityReturnIntentID    string                             `json:"-"`
	NativeApple               bool                               `json:"nativeApple,omitempty"`
	// Completion "code" is required on every native sign-in that goes through the
	// browser (all providers except the native Apple assertion): the callback
	// returns /app/account-return#tx=&code=, and the app presents that code
	// (X-Portico-Completion and completionCode). Only the browser where the person
	// signed in receives it, so the party that started the transaction cannot
	// collect someone else's sign-in by polling.
	Completion string `json:"completion,omitempty"`
}
type OIDCCompleteInput struct {
	CompletionCode  string `json:"completionCode,omitempty"`
	Username        string `json:"username"`
	DisplayName     string `json:"displayName"`
	OnboardingToken string `json:"onboardingToken"`
	Factor          string `json:"factor"`
}
type OIDCCompletion struct {
	Session   *identity.Session        `json:"-"`
	Challenge *identity.MFAChallenge   `json:"-"`
	Proof     *identity.RecentProof    `json:"recentProof,omitempty"`
	Result    *identity.SecurityResult `json:"securityResult,omitempty"`
}

func cleanProviderName(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name))
	if len(name) > 120 {
		return ""
	}
	return name
}
func (o *OIDCLogin) StartFlow(ctx context.Context, provider, binding, access string, q OIDCStartInput) (OIDCView, string, string, error) {
	var v OIDCView

	var wantedIdentity string
	var wantedRevision int64
	if q.ExpectedIdentity != nil {
		captured := *q.ExpectedIdentity
		q.ExpectedIdentity = &captured
		if q.Mode != "reauthenticate" || q.SessionMode != "browser" || q.Intent == nil || !serverreturn.CredentialPurpose(q.Intent.Purpose, q.Intent.Target) || len(captured.ID) < 1 || len(captured.ID) > 128 || captured.Revision < 1 {
			return v, "", "", identity.ErrInvalid
		}
		wantedIdentity, wantedRevision = captured.ID, captured.Revision
	}
	if q.SecurityReturn != nil {
		captured := *q.SecurityReturn
		q.SecurityReturn = &captured

		var selected struct {
			ID       string `json:"providerIdentityId"`
			Revision int64  `json:"providerRevision,string"`
		}
		raw, e := base64.RawURLEncoding.Strict().DecodeString(captured.Context.Payload)
		if e != nil || len(raw) > 6144 || json.Unmarshal(raw, &selected) != nil {
			return v, "", "", identity.ErrInvalid
		}
		if wantedIdentity != "" && (wantedIdentity != selected.ID || wantedRevision != selected.Revision) {
			return v, "", "", identity.ErrInvalid
		}
		wantedIdentity, wantedRevision = selected.ID, selected.Revision
		if q.SecurityReturnExpectation == nil || q.SecurityReturnIntentID == "" || !captured.Valid() || q.Mode != "reauthenticate" || q.SessionMode != "browser" || q.NativeApple || q.Completion != "" || q.Proof != "" || q.Intent == nil || !serverreturn.CredentialPurpose(q.Intent.Purpose, q.Intent.Target) {
			return v, "", "", identity.ErrInvalid
		}
	}
	p := o.Providers[provider]
	if p == nil || len(binding) != 43 || len(q.RequestID) < 16 || len(q.RequestID) > 128 || (q.SessionMode != "native" && q.SessionMode != "browser") || (q.Mode != "login" && q.Mode != "reauthenticate" && q.Mode != "link") || (q.NativeApple && (provider != "apple" || q.SessionMode != "native" || p.NativeAudience == "")) || (q.Completion != "" && (q.Completion != NativeCompletionCode || q.SessionMode != "native" || q.NativeApple)) || (q.SessionMode == "native" && !q.NativeApple && q.Completion != NativeCompletionCode) {
		return v, "", "", identity.ErrInvalid
	}
	if q.Mode != "login" && (q.Intent == nil || !q.Intent.Valid()) {
		return v, "", "", identity.ErrInvalid
	}
	if q.Mode == "link" && (q.Intent.Purpose != "provider_link" || q.Intent.Target != provider) {
		return v, "", "", identity.ErrInvalid
	}
	if q.Mode == "login" && (q.Intent != nil || q.Proof != "") {
		return v, "", "", identity.ErrInvalid
	}
	if e := o.Security.Limit(ctx, "oidc-start", binding, 30, time.Hour); e != nil {
		return v, "", "", e
	}
	var expected identity.CurrentFamilyExpectation
	var e error
	if q.Mode != "login" {
		expected, e = o.Store.CaptureClaimAuthority(ctx, security.Hash(access))
		if e != nil {
			return v, "", "", e
		}
		if original := q.SecurityReturnExpectation; q.SecurityReturn != nil && (original.AccountID != expected.AccountID || original.FamilyID != expected.FamilyID || original.AuthEpoch != expected.AuthEpoch || original.FamilyGeneration != expected.FamilyGeneration || original.Mode != expected.Mode || !equalSecret(original.TokenHash, expected.TokenHash)) {
			return v, "", "", identity.ErrCredentials
		}
		if e = o.Security.Limit(ctx, "oidc-security-start", expected.AccountID, 10, 15*time.Minute); e != nil {
			return v, "", "", e
		}
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return v, "", "", e
	}
	defer tx.Rollback()
	bh := security.Hash(binding)
	if _, e = tx.ExecContext(ctx, `INSERT INTO identity_browser_bindings(binding_hash,expires_at) VALUES($1,statement_timestamp()+interval '24 hours') ON CONFLICT DO NOTHING`, bh); e != nil {
		return v, "", "", e
	}
	var gen int64
	if e = tx.QueryRowContext(ctx, `SELECT generation FROM identity_browser_bindings WHERE binding_hash=$1 FOR UPDATE`, bh).Scan(&gen); e != nil {
		return v, "", "", e
	}
	req := o.Security.requestHash(struct {
		Provider, Access string
		Input            OIDCStartInput
		SecurityReturn   *serverreturn.SecurityReturnStart `json:"SecurityReturn,omitempty"`
	}{provider, access, q, q.SecurityReturn})
	var existing string
	var prior, auth, nonce []byte
	e = tx.QueryRowContext(ctx, `SELECT id,start_request_hash,authorization_cipher,native_nonce_cipher FROM identity_transactions WHERE binding_hash=$1 AND operation_id=$2`, bh, q.RequestID).Scan(&existing, &prior, &auth, &nonce)
	if e == nil {
		if !equalSecret(prior, req) {
			return v, "", "", identity.ErrConflict
		}
		r, e := o.lock(ctx, tx, existing, binding)
		if e != nil {
			return v, "", "", e
		}
		u, n := "", ""
		if len(auth) > 0 {
			u, e = o.Keys.Open(auth, "authorize:"+existing)
			if e != nil {
				return v, "", "", e
			}
		}
		if len(nonce) > 0 {
			n, e = o.Keys.Open(nonce, "native-nonce:"+existing)
			if e != nil {
				return v, "", "", e
			}
		}
		return OIDCView{ID: r.ID, Provider: r.Provider, Mode: r.Mode, Status: r.Status, Expires: r.Expires}, u, n, tx.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return v, "", "", e
	}
	var purpose, digest, account, family, epoch, generation, accessHash, link, linkRevision, target any
	if q.Mode != "login" {
		if _, e = AuthorizeCurrentFamilyTx(ctx, tx, expected); e != nil {
			return v, "", "", e
		}
		if expected.Mode != q.SessionMode {
			return v, "", "", identity.ErrCredentials
		}
		purpose = q.Intent.Purpose
		digest = intentDigest(*q.Intent)
		target = q.Intent.Target
		account = expected.AccountID
		family = expected.FamilyID
		epoch = expected.AuthEpoch
		generation = expected.FamilyGeneration
		accessHash = expected.TokenHash
		if q.Mode == "reauthenticate" {
			var id string
			var rev int64
			e = tx.QueryRowContext(ctx, `SELECT id,revision FROM account_external_identities WHERE account_id=$1 AND provider=$2 AND ($3='' OR id=$3) AND revoked_at IS NULL ORDER BY id LIMIT 1 FOR SHARE`, expected.AccountID, provider, wantedIdentity).Scan(&id, &rev)
			if e != nil {
				return v, "", "", identity.ErrRecentAuth
			}
			if wantedIdentity != "" && (id != wantedIdentity || rev != wantedRevision) {
				return v, "", "", identity.ErrRecentAuth
			}
			link = id
			linkRevision = rev
		} else if q.Intent.NeedsProof() {
			if e = o.Security.proofTx(ctx, tx, expected, *q.Intent, q.Proof); e != nil {
				return v, "", "", e
			}
		}
	}
	if e = tx.QueryRowContext(ctx, `UPDATE identity_browser_bindings SET generation=generation+1,created_at=statement_timestamp(),expires_at=statement_timestamp()+interval '24 hours' WHERE binding_hash=$1 RETURNING generation`, bh).Scan(&gen); e != nil {
		return v, "", "", e
	}
	var desc, marker string
	var dr, pr int64
	if e = tx.QueryRowContext(ctx, `SELECT d.id,d.revision,p.revision,p.marker_key_id FROM identity_provider_descriptors d CROSS JOIN identity_protocol_policy p WHERE d.provider=$1 AND d.enabled AND p.enabled FOR SHARE OF d,p`, provider).Scan(&desc, &dr, &pr, &marker); e != nil {
		return v, "", "", e
	}
	id, state, rawNonce, verifier := security.RandomID("oid_"), security.RandomID(""), security.RandomID(""), security.RandomID("")
	authorization := ""
	var ac, nc []byte
	if q.NativeApple {
		nc, e = o.Keys.Seal(rawNonce, "native-nonce:"+id)
	} else {
		authorization = p.Authorization(state, rawNonce, verifier)
		ac, e = o.Keys.Seal(authorization, "authorize:"+id)
	}
	if e != nil {
		return v, "", "", e
	}
	var expiry time.Time
	e = tx.QueryRowContext(ctx, `INSERT INTO identity_transactions(id,binding_hash,binding_generation,state_hash,nonce_hash,descriptor_id,provider,descriptor_revision,policy_revision,marker_key_id,mode,session_mode,purpose,intent_digest,expected_account_id,expected_family_id,expected_account_epoch,expected_family_generation,expected_access_hash,expected_identity_id,expected_identity_revision,expires_at,operation_id,start_request_hash,authorization_cipher,native_nonce_cipher,intent_target,native_apple) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,statement_timestamp()+interval '10 minutes',$22,$23,$24,$25,$26,$27) RETURNING expires_at`, id, bh, gen, security.Hash(state), security.Hash(rawNonce), desc, provider, dr, pr, marker, q.Mode, q.SessionMode, purpose, digest, account, family, epoch, generation, accessHash, link, linkRevision, q.RequestID, req, ac, nc, target, q.NativeApple).Scan(&expiry)
	if e != nil {
		return v, "", "", e
	}
	if q.Completion == NativeCompletionCode {
		if _, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET native_completion=true WHERE id=$1`, id); e != nil {
			return v, "", "", e
		}
	}
	// Keep the action operation separate from the transport retry key.
	if q.Intent != nil {
		b, _ := json.Marshal(q.Intent)
		sealed, e := o.Keys.Seal(string(b), "oidc-intent:"+id)
		if e != nil {
			return v, "", "", e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET security_intent_cipher=$2 WHERE id=$1`, id, sealed); e != nil {
			return v, "", "", e
		}
	}
	if q.SecurityReturn != nil {
		if _, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET security_return=true WHERE id=$1`, id); e != nil {
			return v, "", "", e
		}
		if e = o.registerSecurityReturnTx(ctx, tx, id, binding, q.SecurityReturnIntentID, expected, *q.Intent, *q.SecurityReturn, p.Callback); e != nil {
			return v, "", "", e
		}
	}
	sealed, e := o.Keys.Seal(verifier, id)
	if e != nil {
		return v, "", "", e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO identity_exchange_secrets(transaction_id,transaction_expires_at,sealed_verifier,expires_at) VALUES($1,$2,$3,$2)`, id, expiry, sealed); e != nil {
		return v, "", "", e
	}
	v = OIDCView{ID: id, Provider: provider, Mode: q.Mode, Status: "started", Expires: expiry}
	nativeNonce := ""
	if q.NativeApple {
		nativeNonce = rawNonce
	}
	return v, authorization, nativeNonce, tx.Commit()
}
func (o *OIDCLogin) rotateOnboarding(ctx context.Context, tx *sql.Tx, id string) (string, error) {
	token := security.RandomID("onb_")
	cipher, e := o.Keys.Seal(token, "onboarding:"+id)
	if e != nil {
		return "", e
	}
	_, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET onboarding_token_hash=$2,onboarding_token_cipher=$3,revision=revision+1 WHERE id=$1`, id, security.Hash(token), cipher)
	return token, e
}
func (o *OIDCLogin) viewTx(ctx context.Context, tx *sql.Tx, r oidcRow, completion string) (*OIDCView, error) {
	v := &OIDCView{ID: r.ID, Provider: r.Provider, Mode: r.Mode, Expires: r.Expires.UTC(), Status: r.Status}
	v.SecurityReturn = r.SecurityReturn
	// A12: a native (non-Apple) transaction's initiator is only its binding
	// holder, who may not be the person who signed in at the provider. Until
	// the completion code delivered to the callback browser is presented, it
	// sees the status only: no suggested name, account existence, contact state
	// or onboarding token.
	if r.NativeCompletion {
		var expected []byte
		e := tx.QueryRowContext(ctx, `SELECT code_hash FROM identity_native_completions WHERE transaction_id=$1 AND expires_at>clock_timestamp()`, r.ID).Scan(&expected)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
		if len(completion) != 43 || len(expected) == 0 || !equalSecret(expected, security.Hash(completion)) {
			return v, nil
		}
	}
	if r.Status == "verified" || r.Status == "completed" || r.Status == "proof_issued" {
		var known, contact bool
		var cipher []byte
		// Show only a contact verified for this transaction. An existing linked
		// account's primary contact is not a provider assertion or a fallback.
		e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_external_identities i WHERE i.issuer=a.issuer AND i.subject=a.subject AND i.revoked_at IS NULL),a.email_verified OR EXISTS(SELECT 1 FROM identity_contact_challenges c WHERE c.transaction_id=t.id AND c.is_current AND c.verified_at IS NOT NULL) OR EXISTS(SELECT 1 FROM account_external_identities i JOIN account_contact_emails c ON c.account_id=i.account_id AND c.is_primary WHERE i.issuer=a.issuer AND i.subject=a.subject AND i.revoked_at IS NULL),COALESCE(a.suggested_name,''),t.onboarding_token_cipher,(t.status='verified' AND t.mfa_challenge_id IS NOT NULL),COALESCE((SELECT c.address FROM identity_contact_challenges c WHERE c.transaction_id=t.id AND c.is_current AND c.verified_at IS NOT NULL),CASE WHEN a.email_verified THEN a.provider_email END,'') FROM identity_verified_assertions a JOIN identity_transactions t ON t.id=a.transaction_id WHERE a.transaction_id=$1`, r.ID).Scan(&known, &contact, &v.SuggestedName, &cipher, &v.MFARequired, &v.VerifiedEmail)
		if e != nil {
			return nil, e
		}
		v.ContactVerified = contact
		v.NeedsUsername = r.Mode == "login" && !known
		if r.Mode == "login" && r.Status == "verified" {
			v.Status = "onboarding"
			if !contact {
				v.Status = "contact_pending"
			}
			if !known {
				if len(cipher) == 0 {
					v.OnboardingToken, e = o.rotateOnboarding(ctx, tx, r.ID)
				} else {
					v.OnboardingToken, e = o.Keys.Open(cipher, "onboarding:"+r.ID)
				}
				if e != nil {
					return nil, e
				}
			}
		}
	}
	return v, nil
}
func (o *OIDCLogin) View(ctx context.Context, id, binding, completion string) (*OIDCView, error) {
	if len(binding) != 43 {
		return nil, identity.ErrCredentials
	}
	if id == "" {
		e := o.Store.DB.QueryRowContext(ctx, `SELECT t.id FROM identity_transactions t JOIN identity_browser_bindings b ON b.binding_hash=t.binding_hash AND b.generation=t.binding_generation WHERE t.binding_hash=$1 AND t.expires_at>clock_timestamp() ORDER BY t.created_at DESC LIMIT 1`, security.Hash(binding)).Scan(&id)
		if errors.Is(e, sql.ErrNoRows) {
			return nil, nil
		}
		if e != nil {
			return nil, e
		}
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	r, e := o.lock(ctx, tx, id, binding)
	if e != nil {
		return nil, e
	}
	v, e := o.viewTx(ctx, tx, r, completion)
	if e != nil {
		return nil, e
	}
	return v, tx.Commit()
}
