package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
)

// Completion serializes immutable subject ownership, then the account and its
// current family, before locking the transaction. Callback verification itself
// holds no account lock and never creates an account or issues a session.
func (o *OIDCLogin) CompleteFlow(ctx context.Context, id, binding, access string, q OIDCCompleteInput, enforceOnboarding bool) (OIDCCompletion, error) {
	var out OIDCCompletion
	if len(binding) != 43 || len(id) > 128 || len(q.Factor) > 128 || len(q.CompletionCode) > 128 {
		return out, identity.ErrInvalid
	}
	if e := o.Security.Limit(ctx, "oidc-complete", id, 30, 15*time.Minute); e != nil {
		return out, e
	}
	tx, e := o.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	var bindingGen int64
	if e = tx.QueryRowContext(ctx, `SELECT generation FROM identity_browser_bindings WHERE binding_hash=$1 AND expires_at>clock_timestamp() FOR SHARE`, security.Hash(binding)).Scan(&bindingGen); e != nil {
		return out, identity.ErrCredentials
	}
	var issuer, subject, mode string
	var expectedAccount sql.NullString
	e = tx.QueryRowContext(ctx, `SELECT a.issuer,a.subject,t.mode,t.expected_account_id FROM identity_transactions t JOIN identity_verified_assertions a ON a.transaction_id=t.id WHERE t.id=$1 AND t.binding_hash=$2 AND t.binding_generation=$3`, id, security.Hash(binding), bindingGen).Scan(&issuer, &subject, &mode, &expectedAccount)
	if e != nil {
		return out, currentChallengeError(e)
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,193))`, hex.EncodeToString(security.Hash(issuer+"\x00"+subject))); e != nil {
		return out, e
	}
	var accountID, identityID string
	var identityRevision int64
	var revoked sql.NullTime
	e = tx.QueryRowContext(ctx, `SELECT account_id,id,revision,revoked_at FROM account_external_identities WHERE issuer=$1 AND subject=$2`, issuer, subject).Scan(&accountID, &identityID, &identityRevision, &revoked)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	lockAccount := accountID
	if mode != "login" {
		lockAccount = expectedAccount.String
	}
	var a identity.Account
	var epoch int64
	if lockAccount != "" {
		e = tx.QueryRowContext(ctx, `SELECT id,username,display_name,auth_epoch FROM accounts WHERE id=$1 FOR UPDATE`, lockAccount).Scan(&a.ID, &a.Username, &a.DisplayName, &epoch)
		if e != nil {
			return out, currentChallengeError(e)
		}
	}
	r, e := o.lock(ctx, tx, id, binding)
	if e != nil {
		return out, e
	}
	var now time.Time
	if e = tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return out, e
	}
	if !r.Expires.After(now) {
		return out, identity.ErrChallenge
	}
	if e = appleAssertionAllowedTx(ctx, tx, id, issuer, subject); e != nil {
		return out, e
	}
	if r.NativeCompletion {
		var expected []byte
		var expiry time.Time
		var consumed sql.NullTime
		if e = tx.QueryRowContext(ctx, `SELECT code_hash,expires_at,consumed_at FROM identity_native_completions WHERE transaction_id=$1 FOR UPDATE`, id).Scan(&expected, &expiry, &consumed); e != nil {
			return out, currentChallengeError(e)
		}
		// A13: a retry after a lost response presents the same, already consumed
		// code. A completed sign-in gets its stored session receipt below, and one
		// waiting on its second factor gets the same stored MFA challenge back:
		// never a new session, a new challenge or a fresh attempt budget.
		pendingMFA := false
		if consumed.Valid && r.Mode == "login" && r.Status == "verified" {
			if e = tx.QueryRowContext(ctx, `SELECT mfa_challenge_cipher IS NOT NULL FROM identity_transactions WHERE id=$1`, id).Scan(&pendingMFA); e != nil {
				return out, e
			}
		}
		replay := consumed.Valid && r.Mode == "login" && (r.Status == "completed" || pendingMFA) && len(q.CompletionCode) == 43 && equalSecret(expected, security.Hash(q.CompletionCode))
		if !replay && !validNativeCompletion(q.CompletionCode, expected, expiry, consumed.Valid, now) {
			return out, identity.ErrCredentials
		}
		// Atomic with the resulting session, MFA challenge or recent-auth proof.
		// Failed validation rolls back; a committed completion is never reused
		// except to read back its own receipt.
		if !replay {
			if _, e = tx.ExecContext(ctx, `UPDATE identity_native_completions SET consumed_at=clock_timestamp() WHERE transaction_id=$1`, id); e != nil {
				return out, e
			}
		}
	}
	if r.Mode != "login" {
		return o.completeSecurityTx(ctx, tx, r, binding, access, q, issuer, subject, accountID, identityID, identityRevision, revoked)
	}
	if r.Status == "completed" {
		var cipher []byte
		var family string
		var expectedEpoch, generation int64
		e = tx.QueryRowContext(ctx, `SELECT ciphertext,account_id,family_id,account_epoch,family_generation FROM identity_completion_receipts WHERE transaction_id=$1 AND binding_generation=$2 AND expires_at>clock_timestamp()`, id, r.Generation).Scan(&cipher, &accountID, &family, &expectedEpoch, &generation)
		if e != nil {
			return out, currentChallengeError(e)
		}
		session, e := o.Security.sessionReceipt(ctx, tx, cipher, "identity:"+id, accountID, family, expectedEpoch, generation, r.SessionMode)
		if e != nil {
			return out, e
		}
		out.Session = &session
		return out, tx.Commit()
	}
	if r.Status != "verified" {
		return out, identity.ErrChallenge
	}
	if accountID != "" && revoked.Valid {
		restored, err := o.restoreAppleConsentTx(ctx, tx, issuer, identityID, revoked)
		if err != nil {
			return out, err
		}
		if !restored {
			return out, identity.ErrProviderConflict
		}
		revoked = sql.NullTime{}
		identityRevision++
	}
	// A short-lived keyed tombstone prevents a callback begun before deletion
	// from recreating the just-deleted account. It is not an email identifier.
	var deleted bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM deleted_identity_markers WHERE marker_key_id=$1 AND subject_digest=$2 AND expires_at>clock_timestamp())`, r.Marker, o.Security.digest("deleted-identity", issuer, subject)).Scan(&deleted); e != nil {
		return out, e
	}
	if deleted {
		return out, identity.ErrProviderConflict
	}
	if accountID == "" {
		var expectedToken []byte
		if e = tx.QueryRowContext(ctx, `SELECT onboarding_token_hash FROM identity_transactions WHERE id=$1`, id).Scan(&expectedToken); e != nil {
			return out, e
		}
		if enforceOnboarding && !equalSecret(expectedToken, security.Hash(q.OnboardingToken)) {
			return out, identity.ErrChallenge
		}
		username := strings.ToLower(strings.TrimSpace(q.Username))
		name := cleanProviderName(q.DisplayName)
		if !identity.ValidUsername(username) || name == "" {
			return out, identity.ErrInvalid
		}
		var email string
		var privateRelay, providerContact bool
		e = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT c.address FROM identity_contact_challenges c WHERE c.transaction_id=a.transaction_id AND c.is_current AND c.verified_at IS NOT NULL),CASE WHEN a.email_verified THEN a.provider_email END,''),a.private_relay AND NOT EXISTS(SELECT 1 FROM identity_contact_challenges c WHERE c.transaction_id=a.transaction_id AND c.is_current AND c.verified_at IS NOT NULL),NOT EXISTS(SELECT 1 FROM identity_contact_challenges c WHERE c.transaction_id=a.transaction_id AND c.is_current AND c.verified_at IS NOT NULL) FROM identity_verified_assertions a WHERE transaction_id=$1`, id).Scan(&email, &privateRelay, &providerContact)
		if e != nil {
			return out, e
		}
		email, e = NormalizeContact(email)
		if e != nil {
			return out, identity.ErrChallenge
		}
		a = identity.Account{ID: security.RandomID("acct_"), Username: username, DisplayName: name}
		var created string
		e = tx.QueryRowContext(ctx, `INSERT INTO accounts(id,username,display_name,password_hash) VALUES($1,$2,$3,NULL) ON CONFLICT(username) DO NOTHING RETURNING id`, a.ID, a.Username, a.DisplayName).Scan(&created)
		if errors.Is(e, sql.ErrNoRows) {
			if _, e = o.rotateOnboarding(ctx, tx, id); e != nil {
				return out, e
			}
			if r.NativeCompletion {
				if _, e = tx.ExecContext(ctx, `UPDATE identity_native_completions SET consumed_at=NULL WHERE transaction_id=$1`, id); e != nil {
					return out, e
				}
			}
			if e = tx.Commit(); e != nil {
				return out, e
			}
			// This is the confirmed public-handle unique-insert refusal only.
			return out, identity.ErrUsernameUnavailable
		}
		if e != nil {
			return out, e
		}
		accountID = a.ID
		identityID = security.RandomID("identity_")
		identityRevision = 1
		if _, e = tx.ExecContext(ctx, `INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES($1,$2,$3,$4,$5,clock_timestamp())`, identityID, a.ID, r.Provider, issuer, subject); e != nil {
			return out, e
		}
		source := "challenge"
		var sourceID any
		if providerContact {
			source = "provider"
			sourceID = identityID
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO account_contact_emails(id,account_id,address,verified_at,source,source_identity_id,private_relay,is_primary) VALUES($1,$2,$3,clock_timestamp(),$4,$5,$6,true)`, security.RandomID("contact_"), a.ID, email, source, sourceID, privateRelay); e != nil {
			return out, e
		}
		if e = audit(ctx, tx, a.ID, "account.provider_registered", identityID, 1); e != nil {
			return out, e
		}
	}
	// Re-read under the account lock: a concurrent unlink may have revoked this
	// subject after the initial ownership lookup but before we acquired authority.
	var active bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_external_identities WHERE id=$1 AND account_id=$2 AND revision=$3 AND revoked_at IS NULL)`, identityID, a.ID, identityRevision).Scan(&active); e != nil {
		return out, e
	}
	if !active {
		return out, identity.ErrProviderConflict
	}
	var mfa bool
	var mfaCipher []byte
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_mfa WHERE account_id=$1 AND secret IS NOT NULL),(SELECT mfa_challenge_cipher FROM identity_transactions WHERE id=$2)`, a.ID, id).Scan(&mfa, &mfaCipher); e != nil {
		return out, e
	}
	if mfa {
		var challenge identity.MFAChallenge
		if len(mfaCipher) > 0 {
			raw, e := o.Keys.Open(mfaCipher, "oidc-mfa:"+id)
			if e != nil {
				return out, e
			}
			if e = json.Unmarshal([]byte(raw), &challenge); e != nil {
				return out, e
			}
		} else {
			challenge, e = o.Security.createMFAChallengeTx(ctx, tx, a.ID, epoch, r.SessionMode, "provider", nil, identityID, identityRevision, id)
			if e != nil {
				return out, e
			}
			cipher, e := o.Security.sealResult(challenge, "oidc-mfa:"+id)
			if e != nil {
				return out, e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET mfa_challenge_id=$2,mfa_challenge_cipher=$3 WHERE id=$1`, id, challenge.ChallengeID, cipher); e != nil {
				return out, e
			}
		}
		out.Challenge = &challenge
		return out, tx.Commit()
	}
	session, e := o.Security.sessionTx(ctx, tx, a, r.SessionMode)
	if e != nil {
		return out, e
	}
	if e = o.Security.completeIdentitySessionTx(ctx, tx, r, session, epoch); e != nil {
		return out, e
	}
	if e = audit(ctx, tx, a.ID, "session.provider_authenticated", identityID, identityRevision); e != nil {
		return out, e
	}
	out.Session = &session
	return out, tx.Commit()
}
func (o *OIDCLogin) completeSecurityTx(ctx context.Context, tx *sql.Tx, r oidcRow, binding, access string, q OIDCCompleteInput, issuer, subject, owner, link string, revision int64, revoked sql.NullTime) (OIDCCompletion, error) {
	var out OIDCCompletion
	var expected identity.CurrentFamilyExpectation
	var expectedLink sql.NullString
	var expectedRevision sql.NullInt64
	var cipher []byte
	e := tx.QueryRowContext(ctx, `SELECT expected_account_id,expected_family_id,expected_account_epoch,expected_family_generation,expected_access_hash,session_mode,expected_identity_id,expected_identity_revision,security_intent_cipher FROM identity_transactions WHERE id=$1`, r.ID).Scan(&expected.AccountID, &expected.FamilyID, &expected.AuthEpoch, &expected.FamilyGeneration, &expected.TokenHash, &expected.Mode, &expectedLink, &expectedRevision, &cipher)
	if e != nil {
		return out, e
	}
	if !equalSecret(expected.TokenHash, security.Hash(access)) {
		return out, identity.ErrCredentials
	}
	if e = o.securityReturnAdmissionTx(ctx, tx, r.ID, binding, r.SecurityReturn); e != nil {
		return out, e
	}
	raw, e := o.Keys.Open(cipher, "oidc-intent:"+r.ID)
	if e != nil {
		return out, e
	}
	var intent identity.SecurityIntent
	if e = json.Unmarshal([]byte(raw), &intent); e != nil {
		return out, e
	}
	req := o.Security.requestHash(struct {
		ID    string
		Input OIDCCompleteInput
	}{r.ID, q})
	receiptPurpose := "oidc:" + r.ID
	found, e := o.Security.readReceiptFrom(ctx, tx, expected.TokenHash, intent.OperationID, receiptPurpose, req, &out)
	if e != nil {
		return out, e
	}
	// A consumed link returns only its result; recovering a still-usable proof
	// always revalidates the initiating family, even after a lost response.
	if r.Mode == "reauthenticate" || !found {
		if _, e = AuthorizeCurrentFamilyTx(ctx, tx, expected); e != nil {
			return out, e
		}
	}
	if found {
		return out, tx.Commit()
	}
	if r.Status != "verified" {
		return out, identity.ErrChallenge
	}
	if r.Mode == "reauthenticate" {
		if owner != expected.AccountID || link != expectedLink.String || revision != expectedRevision.Int64 || revoked.Valid {
			return out, identity.ErrRecentAuth
		}
		var live bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_external_identities WHERE id=$1 AND account_id=$2 AND revision=$3 AND revoked_at IS NULL)`, link, owner, revision).Scan(&live); e != nil {
			return out, e
		}
		if !live {
			return out, identity.ErrRecentAuth
		}
		valid, e := o.Security.factorTx(ctx, tx, owner, q.Factor)
		if e != nil {
			return out, e
		}
		if !valid {
			if e = tx.Commit(); e != nil {
				return out, e
			}
			return out, identity.ErrChallenge
		}
		kind := "provider"
		if r.SecurityReturn {
			kind = "provider_security_return"
		}
		proof, e := o.Security.issueProofTx(ctx, tx, expected, intent, kind, link, revision)
		if e != nil {
			return out, e
		}
		if e = o.boundSecurityReturnProofTx(ctx, tx, r.ID, &proof, r.SecurityReturn); e != nil {
			return out, e
		}
		out.Proof = &proof
	} else {
		if owner != "" && owner != expected.AccountID {
			return out, identity.ErrProviderConflict
		}
		if owner == "" {
			link = security.RandomID("identity_")
			if _, e = tx.ExecContext(ctx, `INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES($1,$2,$3,$4,$5,clock_timestamp())`, link, expected.AccountID, r.Provider, issuer, subject); e != nil {
				return out, e
			}
		} else if revoked.Valid {
			if _, e = tx.ExecContext(ctx, `UPDATE account_external_identities SET revoked_at=NULL,verified_at=clock_timestamp(),revision=revision+1 WHERE id=$1`, link); e != nil {
				return out, e
			}
		}
		// Never replace or merge a contact by email similarity when linking.
		// Reconfirming an already-active method adds no method and no notice.
		if owner == "" || revoked.Valid {
			if e = o.Security.notifyTx(ctx, tx, expected.AccountID, "provider_linked", intent.OperationID); e != nil {
				return out, e
			}
		}
		// Adding a sign-in method invalidates no credential, so every session
		// stays signed in (only credential changes revoke families).
		out.Result = &identity.SecurityResult{AccountID: expected.AccountID, FamilyID: expected.FamilyID, Status: "complete"}
	}
	if e = recordAppleAuthorizationTx(ctx, tx, r.ID, r.Provider); e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET status='proof_issued',terminal_at=clock_timestamp(),revision=revision+1,authorization_cipher=NULL,native_nonce_cipher=NULL WHERE id=$1`, r.ID); e != nil {
		return out, e
	}
	receiptIntent := intent
	receiptIntent.Purpose = receiptPurpose
	if e = o.Security.saveReceipt(ctx, tx, expected, receiptIntent, req, out); e != nil {
		return out, e
	}
	if e = audit(ctx, tx, expected.AccountID, "account.provider_"+r.Mode, link, revision); e != nil {
		return out, e
	}
	return out, tx.Commit()
}

// markerKeyID is tied to the local stable encryption key, never a provider key.
func (s *AccountSecurity) markerKeyID() string {
	return hex.EncodeToString(security.Hash(string(s.Keys.SealKey)))
}

// Record the same terminal provider outcome whether or not the login needed
// MFA. The receipt, factor consumption, session and transaction transition are
// committed atomically; a resumed callback cannot create another session.
func (s *AccountSecurity) completeIdentitySessionTx(ctx context.Context, tx *sql.Tx, r oidcRow, session identity.Session, epoch int64) error {
	if err := recordAppleAuthorizationTx(ctx, tx, r.ID, r.Provider); err != nil {
		return err
	}
	cipher, e := s.sealResult(session, "identity:"+r.ID)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO identity_completion_receipts(transaction_id,transaction_expires_at,binding_generation,account_id,family_id,account_epoch,family_generation,ciphertext,expires_at) VALUES($1,$2,$3,$4,$5,$6,0,$7,LEAST($2,statement_timestamp()+interval '5 minutes'))`, r.ID, r.Expires, r.Generation, session.Account.ID, session.FamilyID, epoch, cipher); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE identity_transactions SET status='completed',terminal_at=clock_timestamp(),revision=revision+1,onboarding_token_hash=NULL,onboarding_token_cipher=NULL,authorization_cipher=NULL,native_nonce_cipher=NULL,mfa_challenge_cipher=NULL WHERE id=$1`, r.ID)
	return e
}

func validNativeCompletion(code string, expected []byte, expiry time.Time, consumed bool, now time.Time) bool {
	return len(code) == 43 && !consumed && expiry.After(now) && equalSecret(expected, security.Hash(code))
}
