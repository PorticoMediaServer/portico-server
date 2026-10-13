package identity

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
)

// A Portico Account member is an ordinary account on this server whose
// identity is proven by a Hosted assertion instead of a password
// (account_portico_links, migration 0120). Membership, role, libraries,
// profiles, PINs, restrictions, sessions and devices are the direct-member
// ones; this server decides who may use it, and Hosted only says who someone is.

// ErrAccessRefused is a proven identity that is not a member here, or whose
// membership is disabled. Clients drop the server or go to sign-in.
var ErrAccessRefused = errors.New("This Portico Account does not have access to this server.")

// ErrSessionMigrated answers a refresh of a family that migration 0120 ended.
// The device still has a valid Portico Account and re-admits with an identity
// assertion; nothing about its access changed.
var ErrSessionMigrated = errors.New("This sign-in moved to the server's own membership. Sign in again with the Portico Account.")

// PorticoIdentity is what a verified Hosted assertion establishes.
type PorticoIdentity struct {
	AccountID   string
	Username    string
	DisplayName string
	// Consent is the verified assertion itself (JSON): the account's own
	// statement that it signed in here, which Hosted requires before listing
	// this server for it (INT M6).
	Consent string
	// SignerKeyID is the Hosted signing key that signed the assertion. It is
	// recorded with every session the assertion admits, so that if the key is
	// later revoked those sessions end.
	SignerKeyID string
}

func validHostedAccountID(v string) bool {
	return v != "" && len(v) <= 128 && !strings.ContainsAny(v, " \t\r\n\x00")
}

// PorticoSignIn admits a Portico Account member exactly as a direct password
// sign-in does, minus the password: same account session, profile list, device
// approval and PIN rules. The recovery-owner network rule does not apply —
// the proof came through Hosted, not the recovery password — and Hosted
// already enforced the account's second factor.
func (s *Service) PorticoSignIn(ctx context.Context, who PorticoIdentity) (DirectSignIn, error) {
	if !validHostedAccountID(who.AccountID) {
		return DirectSignIn{}, ErrUnauthorized
	}
	if RemoteSignInClosed(ctx) {
		return DirectSignIn{}, ErrRemoteSignIn
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return DirectSignIn{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	out, e := s.PorticoSignInTx(ctx, tx, who)
	if e != nil {
		return out, e
	}
	return out, gated.Commit()
}

// PorticoSignInTx is PorticoSignIn inside the caller's transaction, so an
// invitation acceptance creates the account and signs it in atomically.
func (s *Service) PorticoSignInTx(ctx context.Context, tx *sql.Tx, who PorticoIdentity) (DirectSignIn, error) {
	var account string
	e := tx.QueryRowContext(ctx, `SELECT l.account_id FROM account_portico_links l JOIN direct_memberships m ON m.account_id=l.account_id WHERE l.hosted_account_id=? AND m.disabled=0`, who.AccountID).Scan(&account)
	if errors.Is(e, sql.ErrNoRows) {
		return DirectSignIn{}, ErrAccessRefused
	}
	if e != nil {
		return DirectSignIn{}, e
	}
	c, e := s.directAccountTx(ctx, tx, account)
	if e != nil {
		return DirectSignIn{}, ErrAccessRefused
	}
	if who.Consent != "" {
		if _, e = tx.ExecContext(ctx, `UPDATE account_portico_links SET consent=?,unindexed=0 WHERE account_id=?`, who.Consent, account); e != nil {
			return DirectSignIn{}, e
		}
	}
	// An account that confirmed a second factor here (an owner who set one up
	// before linking its Portico Account) still needs it: Hosted's own second
	// factor does not replace this server's (INT M9).
	required, e := s.accountFactorRequiredTx(ctx, tx, account)
	if e != nil {
		return DirectSignIn{}, e
	}
	if required {
		challenge, err := s.issueSignInChallengeTx(ctx, tx, account)
		if err != nil {
			return DirectSignIn{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO portico_factor_challenges(token_hash,signer_key_id) VALUES(?,?)`, Digest(challenge.Token), who.SignerKeyID); err != nil {
			return DirectSignIn{}, err
		}
		return DirectSignIn{Challenge: &challenge}, nil
	}
	out, e := s.directSignInTx(ctx, tx, c, true)
	if e != nil {
		return out, e
	}
	return out, markPorticoAdmissionTx(ctx, tx, out, who.SignerKeyID)
}

// markPorticoAdmissionTx records that a sign-in's session family was admitted
// through a Portico identity assertion, and which Hosted signing key signed
// that assertion (portico_admitted_families).
func markPorticoAdmissionTx(ctx context.Context, tx *sql.Tx, out DirectSignIn, signer string) error {
	if out.Session == nil || out.Session.SessionFamilyID == "" {
		return nil
	}
	return recordPorticoAdmissionTx(ctx, tx, out.Session.SessionFamilyID, signer)
}

func recordPorticoAdmissionTx(ctx context.Context, tx *sql.Tx, family, signer string) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO portico_admitted_families(family_id,signer_key_id) VALUES(?,?) ON CONFLICT(family_id) DO UPDATE SET signer_key_id=excluded.signer_key_id WHERE excluded.signer_key_id<>''`, family, signer)
	return e
}

// inheritPorticoAdmissionTx carries a Portico admission, and the signing key
// behind it, from the family that acted (an account session choosing a
// profile, a quick-connect approver) to the family it issued. A session
// derived from one that a key admitted is admitted by that key too.
func inheritPorticoAdmissionTx(ctx context.Context, tx *sql.Tx, from, to string) error {
	if from == "" || to == "" {
		return nil
	}
	admitted, signer := porticoAdmissionTx(ctx, tx, from)
	if !admitted {
		return nil
	}
	return recordPorticoAdmissionTx(ctx, tx, to, signer)
}

// InheritPorticoAdmissionTx is inheritPorticoAdmissionTx for an issuer outside
// this package (Cast pairing), which derives a session from the one that paired it.
func InheritPorticoAdmissionTx(ctx context.Context, tx *sql.Tx, from, to string) error {
	return inheritPorticoAdmissionTx(ctx, tx, from, to)
}

func porticoFamilyTx(ctx context.Context, tx *sql.Tx, family string) bool {
	admitted, _ := porticoAdmissionTx(ctx, tx, family)
	return admitted
}

// porticoAdmissionTx reports whether a family was admitted through a Portico
// identity assertion, and the signing key that admitted it.
func porticoAdmissionTx(ctx context.Context, tx *sql.Tx, family string) (bool, string) {
	var signer string
	if family == "" || tx.QueryRowContext(ctx, `SELECT signer_key_id FROM portico_admitted_families WHERE family_id=?`, family).Scan(&signer) != nil {
		return false, ""
	}
	return true, signer
}

// RevokeSessionsAdmittedByRevokedKeysTx ends every live session that was
// admitted, directly or through a session it was derived from, by a Hosted
// signing key this server now knows to be revoked. The caller has just
// recorded the revocation in this transaction. Sessions that never depended on
// Hosted (a password sign-in, the recovery owner) are not touched, and neither
// is anything that needs Hosted to be reachable: this is a local decision
// about a key this server once believed.
func (s *Service) RevokeSessionsAdmittedByRevokedKeysTx(ctx context.Context, tx *sql.Tx) (int, error) {
	return s.revokeWhereTx(ctx, tx, RevokedSigningKeyRevoked, `id IN (SELECT a.family_id FROM portico_admitted_families a JOIN hosted_signing_revocations r ON r.key_id=a.signer_key_id WHERE a.signer_key_id<>'')`)
}

// revokeWhereTx ends every live family a trusted, static predicate selects,
// through the family hook, so what those sessions had open (playback, queues,
// readers) is retired with them. Values are always passed as arguments.
func (s *Service) revokeWhereTx(ctx context.Context, tx *sql.Tx, reason RevocationReason, predicate string, args ...any) (int, error) {
	rows, e := tx.QueryContext(ctx, `SELECT id,current_generation,authorization_horizon FROM authorization_session_families WHERE revoked=0 AND (`+predicate+`)`, args...)
	if e != nil {
		return 0, e
	}
	var families []FamilyState
	for rows.Next() {
		var f FamilyState
		var horizon string
		if e = rows.Scan(&f.ID, &f.TokenGeneration, &horizon); e != nil {
			rows.Close()
			return 0, e
		}
		f.AuthorizationHorizon, _ = time.Parse(time.RFC3339, horizon)
		families = append(families, f)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	for _, f := range families {
		if s.OnFamilyRevokedTx != nil {
			if e = s.OnFamilyRevokedTx(ctx, tx, f); e != nil {
				return 0, e
			}
		}
		if e = RevokeFamilyTx(ctx, tx, f.ID, reason); e != nil {
			return 0, e
		}
	}
	return len(families), nil
}

func familyForTokenTx(ctx context.Context, tx *sql.Tx, hash string) string {
	var family string
	if hash == "" || tx.QueryRowContext(ctx, `SELECT family_id FROM authorization_family_tokens WHERE token_hash=?`, hash).Scan(&family) != nil {
		return ""
	}
	return family
}

// porticoLinkedTx reports whether an account's identity is a Portico Account.
func porticoLinkedTx(ctx context.Context, tx *sql.Tx, account string) (bool, error) {
	var linked bool
	e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_portico_links WHERE account_id=?)`, account).Scan(&linked)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	return linked, e
}

// migratedRefreshTx reports whether a refresh credential belonged to a family
// that migration 0120 ended (the Hosted-policy sessions).
func migratedRefreshTx(ctx context.Context, tx *sql.Tx, hash string) bool {
	var found bool
	return tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM hosted_migrated_families WHERE refresh_hash=?)`, hash).Scan(&found) == nil && found
}

// AcceptPorticoCustody records a Portico Account owner accepting Hosted's
// custody of this server's claim (retiring it, its certificates): who carries
// its own fresh custody assertion, which the next membership push forwards
// with its standing. Hosted moves custody only on that (INT M6). The caller
// must be the owner here and linked to that same Portico Account.
func (s *Service) AcceptPorticoCustody(ctx context.Context, bearer string, who PorticoIdentity) error {
	if !validHostedAccountID(who.AccountID) || who.Consent == "" {
		return ErrDirectInput
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, false)
	if e != nil {
		return e
	}
	var linked string
	if e = tx.QueryRowContext(ctx, `SELECT hosted_account_id FROM account_portico_links WHERE account_id=?`, c.account.ID).Scan(&linked); e != nil || linked != who.AccountID || c.account.Role != TierOwner {
		return ErrUnauthorized
	}
	if _, e = tx.ExecContext(ctx, `UPDATE account_portico_links SET consent=? WHERE account_id=?`, who.Consent, c.account.ID); e != nil {
		return e
	}
	// The standing did not change, so no trigger journals it: say it here.
	if _, e = tx.ExecContext(ctx, `INSERT INTO hosted_membership_journal(hosted_account_id) VALUES(?)`, who.AccountID); e != nil {
		return e
	}
	return gated.Commit()
}
