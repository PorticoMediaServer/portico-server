package identity

import (
	"context"
	"portico.local/server/internal/dbwork"
	"time"

	"golang.org/x/crypto/bcrypt"
	"portico.local/server/internal/persistence"
)

// A profile PIN separates viewing profiles. Only an account-management session
// can clear it; a selected child profile's viewer session cannot.

// PINRecoveryMethods is the pre-flight descriptor a client renders before asking
// for anything. It never reveals whether a specific profile has a PIN. A PIN
// reset itself needs only the account-management session (PINReset); the
// fields report the account's sign-in factors. There is no emailed token: the
// email policy sends mail only for account setup, invitations and password
// reset (C59).
type PINRecoveryMethods struct {
	Password      bool `json:"password"`
	Authenticator bool `json:"authenticator"`
	RecoveryCode  bool `json:"recoveryCode"`
}

// PINRecovery reports which confirmations this account can satisfy.
func (s *Service) PINRecovery(ctx context.Context, bearer string) (PINRecoveryMethods, error) {
	gated, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return PINRecoveryMethods{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return PINRecoveryMethods{}, e
	}
	out := PINRecoveryMethods{Password: true}
	confirmed, e := s.accountFactorRequiredTx(ctx, tx, c.account.ID)
	if e != nil {
		return out, e
	}
	out.Authenticator = confirmed
	if confirmed {
		var remaining int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM identity_recovery_codes WHERE account_id=? AND used=0`, c.account.ID).Scan(&remaining); e != nil {
			return out, e
		}
		out.RecoveryCode = remaining > 0
	}
	return out, nil
}

// Hosted reports whether this server signs people in with Portico Accounts,
// which is what its own sign-in and invitation pages ask before offering that
// sign-in. The answer is whether the server's claim is installed: a Portico
// Account can only sign in to a server Hosted knows, and Hosted knows a
// server by its claim, however the server was first set up. A server set up
// for direct sign-in and never claimed answers false and is never shown the
// Portico Account hand-off.
func (s *Service) Hosted() bool {
	if s.SetupClaimReadyTx == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gated, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return false
	}
	defer gated.Rollback()
	attached, err := s.SetupClaimReadyTx(ctx, gated.Tx())
	return err == nil && attached
}

// PINReset clears or replaces a profile PIN for an account manager's current
// session. A selected child profile has viewing authority only.
//
// Resetting bumps pin_revision, which retires every remembered device trust and
// every viewing family for the profile through the existing family hook.
func (s *Service) PINReset(ctx context.Context, bearer, profile, pin string) (DirectSnapshot, error) {
	if !validFamilyID(profile) || pin != "" && !validPIN(pin) {
		return DirectSnapshot{}, ErrDirectInput
	}
	var hash []byte
	var e error
	if pin != "" {
		if slotErr := s.takeHashSlot(ctx); slotErr != nil {
			return DirectSnapshot{}, slotErr
		}
		defer func() { <-s.hashSlots }()
		if hash, e = bcrypt.GenerateFromPassword([]byte(pin), PINCost); e != nil {
			return DirectSnapshot{}, e
		}
	}
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return DirectSnapshot{}, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return DirectSnapshot{}, e
	}
	var deleted int
	if e = tx.QueryRowContext(ctx, `SELECT deleted FROM direct_profiles WHERE account_id=? AND id=?`, c.account.ID, profile).Scan(&deleted); e != nil || deleted != 0 {
		return DirectSnapshot{}, ErrNotVisible
	}
	var value any
	if len(hash) > 0 {
		value = hash
	}
	if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET pin_hash=?,pin_revision=pin_revision+1,pin_attempts=0,pin_locked_until=0,revision=revision+1 WHERE id=?`, value, profile); e != nil {
		return DirectSnapshot{}, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM direct_profile_pin_attempts WHERE profile_id=?`, profile); e != nil {
		return DirectSnapshot{}, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM direct_profile_trust WHERE profile_id=?`, profile); e != nil {
		return DirectSnapshot{}, e
	}
	if e = s.revokeDirectTx(ctx, tx, c.account.ID, profile, ""); e != nil {
		return DirectSnapshot{}, e
	}
	out, e := s.directSnapshotTx(ctx, tx, c)
	if e != nil {
		return out, e
	}
	return out, gated2.Commit()
}

// AuthCapabilities is the public, pre-authentication descriptor. A client reads it
// before showing any sign-in affordance so it never offers a method this server
// does not have. It answers only about the server, never about an account: asking
// for a username first would turn it into an account-existence oracle.
type AuthCapabilities struct {
	ServerID         string   `json:"serverId"`
	ServerName       string   `json:"serverName"`
	SetupRequired    bool     `json:"setupRequired"`
	Methods          []string `json:"methods"`
	SelfRegistration string   `json:"selfRegistration"`
	QuickConnect     bool     `json:"quickConnect"`
	TwoFactor        bool     `json:"twoFactor"`
	HostedAttach     bool     `json:"hostedAttach"`
}

// Capabilities describes the sign-in methods this server exposes.
func (s *Service) Capabilities(ctx context.Context) (AuthCapabilities, error) {
	out := AuthCapabilities{ServerID: s.ID(), ServerName: s.Name(), SetupRequired: s.SetupRequired(), TwoFactor: true, QuickConnect: true}
	out.HostedAttach = s.Hosted()
	out.SelfRegistration = s.SelfRegistration()
	out.Methods = []string{"password"}
	if out.QuickConnect {
		out.Methods = append(out.Methods, "quick-connect")
	}
	if out.HostedAttach {
		out.Methods = append(out.Methods, "hosted")
	}
	if out.SelfRegistration != SelfRegistrationOff {
		out.Methods = append(out.Methods, "self-registration")
	}
	return out, nil
}

// PruneIdentityProofs reclaims expired sign-in challenges and Top Shelf tokens.
// Expiry is authoritative on every read, so this
// only reclaims rows; skipping it would never admit anything.
func (s *Service) PruneIdentityProofs(ctx context.Context) error {
	return persistence.PruneIdentityProofs(ctx, s.db, time.Now().UTC().Format(time.RFC3339))
}
