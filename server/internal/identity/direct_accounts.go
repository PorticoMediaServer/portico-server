package identity

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/notify"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrProfileSelection = errors.New("choose a profile to finish signing in")
var ErrProfilePIN = errors.New("That profile PIN is not correct. Try again after the delay.")
var ErrProfileLocked = errors.New("Profile PIN attempts are temporarily locked. Try again later.")
var ErrProfileCapacity = errors.New("An account can have up to eight active profiles, including its primary profile.")
var ErrProfileChanged = errors.New("This profile changed. Refresh before trying again.")
var ErrDirectInput = errors.New("invalid Direct Sign-In account or profile input")
var ErrPrimaryProfile = errors.New("The primary profile cannot be deleted.")

// An unknown username takes the same bcrypt work as a current direct account.
// The password used to make this fixed dummy hash is never an accepted login.
var unknownAccountPasswordHash = []byte("$2b$10$k3FsQOAfb3qWTvCzZUCG6OIvqW6jBwEohdbNPHuoiacwA92kZ16LG")

type DirectAccount struct {
	ID               string   `json:"id"`
	Username         string   `json:"username"`
	PrimaryProfileID string   `json:"primaryProfileId"`
	Role             string   `json:"role"`
	Revision         int64    `json:"revision"`
	Disabled         bool     `json:"disabled"`
	AllowedLibraries []string `json:"allowedLibraries"`
	// HostedAccountID is set for a Portico Account member: its password, email
	// and second factor are managed at Hosted, everything else here.
	HostedAccountID string `json:"hostedAccountId,omitempty"`
}
type DirectSnapshot struct {
	Authority string          `json:"authority"`
	ServerID  string          `json:"serverId"`
	Account   DirectAccount   `json:"account"`
	Profiles  []DirectProfile `json:"profiles"`
	CanManage bool            `json:"canManage"`
	// CustodyPending: this account owns the server and is a Portico Account,
	// but Hosted's custody of the server's claim (retiring it, its
	// certificates) is not with it. The owner accepts it with a custody
	// assertion (POST /v1/direct/ownership/custody, INT M6).
	CustodyPending bool `json:"custodyPending,omitempty"`
}
type DirectSignIn struct {
	PasswordChangeRequired bool   `json:"passwordChangeRequired,omitempty"`
	DeviceApprovalPending  bool   `json:"deviceApprovalPending,omitempty"`
	DeviceID               string `json:"deviceId,omitempty"`
	InstallationID         string `json:"installationId,omitempty"`
	DirectSnapshot
	// Internal compatibility aliases for the one device-bound Session.
	AccountToken     string    `json:"-"`
	AccountExpiresAt string    `json:"-"`
	Session          *Envelope `json:"session,omitempty"`
	// Challenge stands in place of AccountToken when the account has a confirmed
	// second factor: the password alone has proven nothing the client may keep.
	Challenge *SignInChallenge `json:"challenge,omitempty"`
}
type directCaller struct {
	account    DirectAccount
	epoch      int
	manage     bool
	viewerOnly bool
	deviceID   string
	profileID  string
	familyID   string
}

func directName(v string) bool {
	return strings.TrimSpace(v) != "" && len(v) <= 120 && utf8.ValidString(v) && !strings.ContainsAny(v, "\r\n\t\x00") && strings.IndexFunc(v, func(r rune) bool { return r < 32 || r == 127 }) < 0
}

// directPassword is the one password rule for every local account (owner,
// recovery owner, members, invitations): at least 8 characters. Clients show a
// strength hint, but a weak password is the user's choice. The 72-byte cap is
// bcrypt's input limit, not a policy.
func directPassword(v string) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(v) >= 8 && len(v) <= 72
}
func directLibraries(ids []string) bool {
	if len(ids) > 1000 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !validFamilyID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func (s *Service) passwordAccount(ctx context.Context, user, password string) (directCaller, []byte, error) {
	if len(user) > 64 || len(password) > 72 {
		return directCaller{}, nil, ErrUnauthorized
	}
	var c directCaller
	var hash []byte
	var libs string
	e := s.db.QueryRowContext(ctx, `SELECT a.id,a.username,a.profile_id,a.epoch,a.password_hash,m.role,m.revision,m.disabled,m.allowed_libraries FROM accounts a JOIN direct_memberships m ON m.account_id=a.id WHERE a.username=?`, strings.ToLower(strings.TrimSpace(user))).Scan(&c.account.ID, &c.account.Username, &c.account.PrimaryProfileID, &c.epoch, &hash, &c.account.Role, &c.account.Revision, &c.account.Disabled, &libs)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return directCaller{}, nil, e
	}
	if errors.Is(e, sql.ErrNoRows) {
		hash = unknownAccountPasswordHash
		if err := s.unknownCooloff(ctx, user); err != nil {
			return directCaller{}, nil, err
		}
	} else if err := s.credentialCooloff(ctx, c.account.ID); err != nil {
		return directCaller{}, nil, err
	}
	select {
	case s.hashSlots <- struct{}{}:
	default:
		return directCaller{}, nil, ErrBusy
	}
	// A Portico Account member has no password here. It pays the same bcrypt
	// cost as any other account, so the answer's timing does not tell a prober
	// which usernames are linked (INT M15).
	passwordless := e == nil && len(hash) == 0
	if passwordless {
		hash = unknownAccountPasswordHash
	}
	valid := bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil && !passwordless
	<-s.hashSlots
	outcome := credentialFailure
	if e == nil && valid && !c.account.Disabled {
		outcome = credentialProbe
	}
	var attemptErr error
	if errors.Is(e, sql.ErrNoRows) {
		attemptErr = s.unknownAttempt(ctx, user)
	} else {
		attemptErr = s.passwordAttempt(ctx, c.account.ID, outcome)
	}
	if attemptErr != nil {
		return directCaller{}, nil, attemptErr
	}
	if e != nil || !valid || c.account.Disabled {
		return directCaller{}, nil, ErrUnauthorized
	}
	if json.Unmarshal([]byte(libs), &c.account.AllowedLibraries) != nil {
		return directCaller{}, nil, ErrUnauthorized
	}
	hash = s.rehashOldPassword(ctx, c.account.ID, hash, password)
	c.manage = true
	return c, hash, nil
}

// Older direct accounts used cost 12. Upgrade a proven password to the current
// cost outside the writer, then swap it only if the verified hash is still
// current. A concurrent credential change remains fenced by DirectLoginFrom.
func (s *Service) rehashOldPassword(ctx context.Context, account string, oldHash []byte, password string) []byte {
	cost, err := bcrypt.Cost(oldHash)
	// Any other cost converges on PasswordCost (lead L2), lower or higher.
	if err != nil || cost == PasswordCost {
		return oldHash
	}
	select {
	case s.hashSlots <- struct{}{}:
	default:
		return oldHash
	}
	updated, err := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
	<-s.hashSlots
	if err != nil {
		return oldHash
	}
	result, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassSecurityFence, `UPDATE accounts SET password_hash=? WHERE id=? AND password_hash=?`, updated, account, oldHash)
	if err != nil {
		return oldHash
	}
	if n, err := result.RowsAffected(); err == nil && n == 1 {
		return updated
	}
	// Another sign-in may have upgraded this same password first. The caller
	// retries its admission against that live hash if it moved.
	return oldHash
}

// Account grants cannot be used for media. Their only audience is /v1/direct/*.
// A selected viewing family remains the existing authorization-session family.
func (s *Service) DirectLogin(ctx context.Context, user, password string) (DirectSignIn, error) {
	return s.DirectLoginFrom(ctx, user, password, true)
}
func (s *Service) DirectLoginFrom(ctx context.Context, user, password string, private bool) (DirectSignIn, error) {
	for attempt := 0; attempt < 2; attempt++ {
		out, stale, err := s.directLoginOnce(ctx, user, password, private)
		if !stale {
			return out, err
		}
	}
	return DirectSignIn{}, ErrUnauthorized
}

func (s *Service) directLoginOnce(ctx context.Context, user, password string, private bool) (DirectSignIn, bool, error) {
	if err := ctx.Err(); err != nil {
		return DirectSignIn{}, false, err
	}
	c, hash, e := s.passwordAccount(ctx, user, password)
	if e != nil {
		return DirectSignIn{}, false, e
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return DirectSignIn{}, false, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	var stored []byte
	var epoch, disabled int
	if e = tx.QueryRowContext(ctx, `SELECT a.password_hash,a.epoch,m.disabled FROM accounts a JOIN direct_memberships m ON m.account_id=a.id WHERE a.id=?`, c.account.ID).Scan(&stored, &epoch, &disabled); e != nil {
		return DirectSignIn{}, false, e
	}
	if epoch != c.epoch || disabled != 0 {
		return DirectSignIn{}, false, ErrUnauthorized
	}
	if !bytes.Equal(hash, stored) {
		return DirectSignIn{}, true, nil
	}
	if e = s.recoveryPasswordAllowedTx(ctx, tx, c.account.ID, private); e != nil {
		return DirectSignIn{}, false, e
	}
	// A confirmed second factor replaces the account session with a challenge. The
	// password has been proven in this transaction; nothing else has.
	required, e := s.accountFactorRequiredTx(ctx, tx, c.account.ID)
	if e != nil {
		return DirectSignIn{}, false, e
	}
	if required {
		challenge, err := s.issueSignInChallengeTx(ctx, tx, c.account.ID)
		if err != nil {
			return DirectSignIn{}, false, err
		}
		return DirectSignIn{Challenge: &challenge}, false, gated.Commit()
	}
	out, e := s.directSignInTx(ctx, tx, c, private)
	if e != nil {
		return out, false, e
	}
	if e = s.passwordAttemptTx(ctx, tx, c.account.ID, credentialSuccess); e != nil {
		return DirectSignIn{}, false, e
	}
	return out, false, gated.Commit()
}

// directAccountTx rebuilds a caller from an account id whose credentials were
// already proven in this transaction. It never reads a token.
func (s *Service) directAccountTx(ctx context.Context, tx *sql.Tx, account string) (directCaller, error) {
	c := directCaller{manage: true}
	c.account.ID = account
	var libs string
	e := tx.QueryRowContext(ctx, `SELECT a.username,a.profile_id,a.epoch,m.role,m.revision,m.disabled,m.allowed_libraries FROM accounts a JOIN direct_memberships m ON m.account_id=a.id WHERE a.id=?`, account).Scan(&c.account.Username, &c.account.PrimaryProfileID, &c.epoch, &c.account.Role, &c.account.Revision, &c.account.Disabled, &libs)
	if e != nil || c.account.Disabled {
		return directCaller{}, ErrUnauthorized
	}
	if json.Unmarshal([]byte(libs), &c.account.AllowedLibraries) != nil {
		return directCaller{}, ErrUnauthorized
	}
	return c, nil
}

// directSignInTx mints one rotating device session. Until profile selection,
// that family has account scope and cannot authorize media.
func (s *Service) directSignInTx(ctx context.Context, tx *sql.Tx, c directCaller, private bool) (DirectSignIn, error) {
	out := DirectSignIn{}
	var e error
	out.DirectSnapshot, e = s.directSnapshotTx(ctx, tx, c)
	if e != nil {
		return out, e
	}
	device, e := s.issuingDeviceTx(ctx, tx, "local", c.account.ID)
	if errors.Is(e, ErrDevicePending) {
		return DirectSignIn{DeviceApprovalPending: true, DeviceID: device.ID, InstallationID: device.InstallationID}, nil
	}
	if e != nil {
		return out, e
	}
	out.DeviceID = device.ID
	out.InstallationID = device.InstallationID
	required, err := passwordChangeRequired(ctx, tx, c.account.ID)
	if err != nil {
		return out, err
	}
	out.PasswordChangeRequired = required
	role := "account"
	if !required && len(out.Profiles) == 1 && !out.Profiles[0].PINRequired {
		role = c.account.Role
	}
	envelope, err := s.IssueTx(withExistingDevice(ctx, device.ID), tx, c.account.ID, c.account.PrimaryProfileID, "local", role, c.epoch, time.Time{})
	if err != nil {
		return out, err
	}
	out.Session = &envelope
	out.AccountToken, out.AccountExpiresAt = envelope.AccessToken, envelope.ExpiresAt
	return out, nil
}
func (s *Service) directCallerTx(ctx context.Context, tx *sql.Tx, bearer string, manage bool) (directCaller, error) {
	var c directCaller
	var grantEpoch int
	var sessionRole string
	if len(bearer) != 43 {
		return c, ErrUnauthorized
	}
	record, err := s.familyTokenTx(ctx, tx, Digest(bearer))
	if err != nil || record.principal.Authority != "local" {
		return c, ErrUnauthorized
	}
	if _, err = s.SessionFamilyTx(ctx, tx, record.principal); err != nil {
		return c, err
	}
	c.account.ID = record.principal.AccountID
	c.familyID = record.ID
	c.profileID = record.principal.ProfileID
	if err = tx.QueryRowContext(ctx, `SELECT device_id FROM identity_device_families WHERE family_id=?`, record.ID).Scan(&c.deviceID); err != nil {
		return c, ErrUnauthorized
	}
	grantEpoch = record.principal.Epoch
	var primary int
	if err = tx.QueryRowContext(ctx, `SELECT is_primary FROM direct_profiles WHERE account_id=? AND id=? AND deleted=0`, c.account.ID, record.principal.ProfileID).Scan(&primary); err != nil {
		return c, ErrUnauthorized
	}
	c.manage = primary == 1
	sessionRole = record.principal.Role
	var viewerOnly bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM authorization_family_grants WHERE family_id=? AND grant_kind='viewer_only')`, record.ID).Scan(&viewerOnly); err != nil {
		return c, err
	}
	if viewerOnly {
		c.manage = false
		c.viewerOnly = true
	}
	var libs string
	e := tx.QueryRowContext(ctx, `SELECT a.username,a.profile_id,a.epoch,m.role,m.revision,m.disabled,m.allowed_libraries FROM accounts a JOIN direct_memberships m ON m.account_id=a.id WHERE a.id=?`, c.account.ID).Scan(&c.account.Username, &c.account.PrimaryProfileID, &c.epoch, &c.account.Role, &c.account.Revision, &c.account.Disabled, &libs)
	if sessionRole != "account" && sessionRole != c.account.Role {
		c.manage = false
		c.account.Role = sessionRole
	}
	if e != nil || c.epoch != grantEpoch || c.account.Disabled {
		return c, ErrUnauthorized
	}
	if manage && !c.manage {
		// A valid profile session that cannot manage the account (CD-51).
		return c, ErrForbidden
	}
	if json.Unmarshal([]byte(libs), &c.account.AllowedLibraries) != nil {
		return c, ErrUnauthorized
	}
	if required, err := passwordChangeRequired(ctx, tx, c.account.ID); err != nil {
		return c, err
	} else if required && ctx.Value(passwordChangeContextKey{}) != true {
		return c, ErrPasswordChangeRequired
	}
	return c, nil
}
func (s *Service) DirectMe(ctx context.Context, bearer string) (DirectSnapshot, error) {
	gated2, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return DirectSnapshot{}, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, false)
	if e != nil {
		return DirectSnapshot{}, e
	}
	return s.directSnapshotTx(ctx, tx, c)
}
func (s *Service) EndDirectAccountSession(ctx context.Context, bearer string) error {
	return s.LogoutToken(ctx, bearer)
}

// Explicit revocation traverses the existing family hook, preserving queue,
// controller and physical-reader retirement owned by the playback integration.
func (s *Service) revokeDirectTx(ctx context.Context, tx *sql.Tx, account, profile, family string) error {
	return s.revokeDirectWithReasonTx(ctx, tx, account, profile, family, RevokedAdminRevoke)
}
func (s *Service) revokeDirectWithReasonTx(ctx context.Context, tx *sql.Tx, account, profile, family string, reason RevocationReason) error {
	return s.revokeFamiliesTx(ctx, tx, "local", account, profile, family, reason)
}

// RetireFamilyTx ends one session family of one viewer inside the caller's
// transaction (a Cast receiver's superseded session). An empty family is a no-op,
// never "every family".
func (s *Service) RetireFamilyTx(ctx context.Context, tx *sql.Tx, authority, account, profile, family string) error {
	if family == "" || account == "" || profile == "" {
		return nil
	}
	return s.revokeFamiliesTx(ctx, tx, authority, account, profile, family, RevokedExplicitSignout)
}

// revokeFamiliesTx retires viewing families for one account under one authority. Signing a
// device out has to work the same way for a Portico Account as for a local one, and the only
// thing that differs is which authority's families are being ended.
func (s *Service) revokeFamiliesTx(ctx context.Context, tx *sql.Tx, authority, account, profile, family string, reason RevocationReason) error {
	rows, e := tx.QueryContext(ctx, `SELECT id,current_generation,authorization_horizon FROM authorization_session_families WHERE authority=? AND account_id=? AND (?='' OR profile_id=?) AND (?='' OR id=?) AND revoked=0`, authority, account, profile, profile, family, family)
	if e != nil {
		return e
	}
	var families []FamilyState
	for rows.Next() {
		var f FamilyState
		var expiry string
		if e = rows.Scan(&f.ID, &f.TokenGeneration, &expiry); e != nil {
			rows.Close()
			return e
		}
		f.AuthorizationHorizon, _ = time.Parse(time.RFC3339, expiry)
		families = append(families, f)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, f := range families {
		if s.OnFamilyRevokedTx != nil {
			if e = s.OnFamilyRevokedTx(ctx, tx, f); e != nil {
				return e
			}
		}
		if e = RevokeFamilyTx(ctx, tx, f.ID, reason); e != nil {
			return e
		}
	}
	return nil
}

type DirectSession struct {
	ID            string `json:"id"`
	ProfileID     string `json:"profileId"`
	ProfileName   string `json:"profileName"`
	ExpiresAt     string `json:"expiresAt"`
	Current       bool   `json:"current"`
	RevokedReason string `json:"revokedReason,omitempty"`
	RevokedAt     string `json:"revokedAt,omitempty"`
	StatusText    string `json:"statusText,omitempty"`
}

func sessionRevocationText(reason string) string {
	switch RevocationReason(reason) {
	case RevokedRefreshReuse:
		return "Signed out: sign-in was used twice"
	case RevokedExplicitSignout:
		return "Signed out"
	case RevokedAdminRevoke:
		return "Signed out by an administrator"
	case RevokedMembershipRemoved:
		return "Signed out: account access was removed"
	case RevokedDeviceDisapproved:
		return "Signed out: device was not approved"
	case RevokedPasswordChange:
		return "Signed out: account security changed"
	default:
		return "Signed out"
	}
}

func (s *Service) DirectSessions(ctx context.Context, bearer string) ([]DirectSession, error) {
	gated3, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return nil, e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return nil, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT f.id,f.profile_id,p.name,f.authorization_horizon,EXISTS(SELECT 1 FROM authorization_family_tokens t WHERE t.family_id=f.id AND t.token_hash=?),f.revoked,f.revoked_reason,f.revoked_at FROM authorization_session_families f JOIN direct_profiles p ON p.account_id=f.account_id AND p.id=f.profile_id WHERE f.authority='local' AND f.account_id=? AND (f.revoked=1 OR f.authorization_horizon>?) ORDER BY f.revoked ASC,f.revoked_at DESC,f.id DESC`, Digest(bearer), c.account.ID, time.Now().UTC().Format(time.RFC3339))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []DirectSession{}
	for rows.Next() {
		var f DirectSession
		var revoked bool
		if e = rows.Scan(&f.ID, &f.ProfileID, &f.ProfileName, &f.ExpiresAt, &f.Current, &revoked, &f.RevokedReason, &f.RevokedAt); e != nil {
			return nil, e
		}
		if revoked {
			f.Current = false
			f.StatusText = sessionRevocationText(f.RevokedReason)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
func (s *Service) RevokeDirectSessions(ctx context.Context, bearer, family string) error {
	gated4, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated4.Tx()
	defer gated4.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return e
	}
	if e = s.revokeDirectWithReasonTx(ctx, tx, c.account.ID, "", family, RevokedExplicitSignout); e != nil {
		return e
	}
	if family == "" {
		if _, e = tx.ExecContext(ctx, `UPDATE accounts SET epoch=epoch+1 WHERE id=?`, c.account.ID); e != nil {
			return e
		}
	}
	return gated4.Commit()
}
func (s *Service) ChangeDirectPassword(ctx context.Context, bearer, current, next, code string) error {
	ctx = context.WithValue(ctx, passwordChangeContextKey{}, true)
	if !directPassword(next) || len(current) > 72 || current == next {
		return ErrDirectInput
	}
	proof, e := s.proveCurrentPassword(ctx, bearer, current)
	if e != nil {
		return e
	}
	var hash []byte
	select {
	case s.hashSlots <- struct{}{}:
		hash, e = bcrypt.GenerateFromPassword([]byte(next), PasswordCost)
		<-s.hashSlots
	default:
		return ErrBusy
	}
	if e != nil {
		return e
	}
	gated5, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated5.Tx()
	defer gated5.Rollback()
	c, e := s.checkCurrentPasswordProofTx(ctx, tx, bearer, proof)
	if e != nil {
		return e
	}
	if e = s.verifySecondFactorTx(ctx, tx, c.account.ID, code); e != nil {
		return e
	}
	if e = s.revokeCredentialsTx(ctx, tx, c.account.ID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE accounts SET password_hash=?,epoch=epoch+1 WHERE id=?`, hash, c.account.ID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM identity_credential_state WHERE account_id=?`, c.account.ID); e != nil {
		return e
	}
	// A credential change is a security event the account holder must see; it
	// commits with the new password or not at all.
	var primaryProfile string
	if e = tx.QueryRowContext(ctx, `SELECT profile_id FROM accounts WHERE id=?`, c.account.ID).Scan(&primaryProfile); e != nil {
		return e
	}
	if _, e = notify.NotifySecurityEvent(tx, time.Now().UnixMilli(), "local", c.account.ID, primaryProfile, notify.SecurityPasswordChange, "", ""); e != nil {
		return e
	}
	return gated5.Commit()
}

type DirectMemberInput struct {
	Username         string   `json:"username"`
	Password         string   `json:"password"`
	Name             string   `json:"name"`
	AllowedLibraries []string `json:"allowedLibraries"`
	ExpectedRevision int64    `json:"expectedRevision"`
	Disabled         bool     `json:"disabled"`
}

func (s *Service) DirectMembers(ctx context.Context, bearer string) ([]DirectAccount, error) {
	gated6, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return nil, e
	}
	tx := gated6.Tx()
	defer gated6.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return nil, e
	}
	if c.account.Role != "owner" {
		return nil, ErrForbidden
	}
	rows, e := tx.QueryContext(ctx, `SELECT a.id,a.username,a.profile_id,m.role,m.revision,m.disabled,m.allowed_libraries,COALESCE(l.hosted_account_id,'') FROM accounts a JOIN direct_memberships m ON m.account_id=a.id LEFT JOIN account_portico_links l ON l.account_id=a.id ORDER BY a.username`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []DirectAccount{}
	for rows.Next() {
		var a DirectAccount
		var raw string
		if e = rows.Scan(&a.ID, &a.Username, &a.PrimaryProfileID, &a.Role, &a.Revision, &a.Disabled, &raw, &a.HostedAccountID); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &a.AllowedLibraries); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Service) CreateDirectMember(ctx context.Context, bearer string, q DirectMemberInput) (DirectAccount, error) {
	q.Username = strings.ToLower(strings.TrimSpace(q.Username))
	q.Name = strings.TrimSpace(q.Name)
	if len(q.Username) < 3 || len(q.Username) > 64 || strings.ContainsAny(q.Username, " \t\r\n@") || !directPassword(q.Password) || !directName(q.Name) || !directLibraries(q.AllowedLibraries) {
		return DirectAccount{}, ErrDirectInput
	}
	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	default:
		return DirectAccount{}, ErrBusy
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(q.Password), PasswordCost)
	if e != nil {
		return DirectAccount{}, e
	}
	gated7, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return DirectAccount{}, e
	}
	tx := gated7.Tx()
	defer gated7.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return DirectAccount{}, e
	}
	if c.account.Role != "owner" {
		return DirectAccount{}, ErrForbidden
	}
	var count int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE username=?`, q.Username).Scan(&count); e != nil {
		return DirectAccount{}, e
	}
	if count != 0 {
		return DirectAccount{}, ErrConflict
	}
	if q.AllowedLibraries == nil {
		q.AllowedLibraries = []string{}
	}
	out := DirectAccount{ID: Token(), Username: q.Username, PrimaryProfileID: Token(), Role: "member", Revision: 1, AllowedLibraries: q.AllowedLibraries}
	if _, e = tx.ExecContext(ctx, `INSERT INTO accounts(id,username,password_hash,profile_id) VALUES(?,?,?,?)`, out.ID, out.Username, hash, out.PrimaryProfileID); e != nil {
		return out, e
	}
	raw, _ := json.Marshal(q.AllowedLibraries)
	if _, e = tx.ExecContext(ctx, `UPDATE direct_memberships SET allowed_libraries=? WHERE account_id=?`, string(raw), out.ID); e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE direct_profiles SET name=? WHERE id=?`, q.Name, out.PrimaryProfileID); e != nil {
		return out, e
	}
	return out, gated7.Commit()
}
func (s *Service) UpdateDirectMember(ctx context.Context, bearer, id string, q DirectMemberInput) error {
	if !validFamilyID(id) || q.ExpectedRevision < 1 || !directLibraries(q.AllowedLibraries) || (q.Password != "" && !directPassword(q.Password)) {
		return ErrDirectInput
	}
	var passwordHash []byte
	if q.Password != "" {
		select {
		case s.hashSlots <- struct{}{}:
			defer func() { <-s.hashSlots }()
		default:
			return ErrBusy
		}
		var err error
		passwordHash, err = bcrypt.GenerateFromPassword([]byte(q.Password), PasswordCost)
		if err != nil {
			return err
		}
	}
	gated8, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated8.Tx()
	defer gated8.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return e
	}
	if !Administrative(c.account.Role) {
		return ErrUnauthorized
	}
	var role string
	var revision int64
	if e = tx.QueryRowContext(ctx, `SELECT role,revision FROM direct_memberships WHERE account_id=?`, id).Scan(&role, &revision); e != nil {
		return e
	}
	if !ManagesTier(c.account.Role, role) {
		return ErrUnauthorized
	}
	if revision != q.ExpectedRevision {
		return ErrProfileChanged
	}
	// A Portico Account member's password is its own, at Hosted.
	if q.Password != "" {
		if linked, err := porticoLinkedTx(ctx, tx, id); err != nil || linked {
			return ErrDirectInput
		}
	}
	if q.AllowedLibraries == nil {
		q.AllowedLibraries = []string{}
	}
	raw, _ := json.Marshal(q.AllowedLibraries)
	reason := RevokedAdminRevoke
	if q.Password != "" {
		reason = RevokedPasswordChange
	}
	if q.Disabled {
		reason = RevokedMembershipRemoved
	}
	if e = s.revokeCredentialsWithReasonTx(ctx, tx, id, reason); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE direct_memberships SET disabled=?,allowed_libraries=?,revision=revision+1 WHERE account_id=?`, q.Disabled, string(raw), id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE accounts SET epoch=epoch+1 WHERE id=?`, id); e != nil {
		return e
	}
	if passwordHash != nil {
		if _, e = tx.ExecContext(ctx, `UPDATE accounts SET password_hash=? WHERE id=?`, passwordHash, id); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO identity_credential_state(account_id,must_change) VALUES(?,1) ON CONFLICT(account_id) DO UPDATE SET must_change=1`, id); e != nil {
			return e
		}
		for _, table := range []string{"identity_credential_attempts", "identity_account_attempt_budget"} {
			if _, e = tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE account_id=?`, id); e != nil {
				return e
			}
		}
		if e = s.securityNoticeTx(ctx, tx, id, "security.password_reset"); e != nil {
			return e
		}
	}
	return gated8.Commit()
}
func (s *Service) TransferDirectOwnership(ctx context.Context, bearer, target, password string, expected int64) error {
	if len(password) > 72 || target == "" {
		return ErrDirectInput
	}
	proof, e := s.proveCurrentPassword(ctx, bearer, password)
	if e != nil {
		return e
	}
	gated9, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated9.Tx()
	defer gated9.Rollback()
	c, e := s.checkCurrentPasswordProofTx(ctx, tx, bearer, proof)
	if e != nil {
		return e
	}
	if e = s.transferOwnershipTx(ctx, tx, c, target, expected); e != nil {
		return e
	}
	return gated9.Commit()
}

// TransferPorticoOwnership is TransferDirectOwnership for an owner whose
// identity is a Portico Account: it has no password here, so a fresh Hosted
// identity assertion for that same account is the confirmation. The journal
// then carries the owner change to Hosted, which moves the claim with it.
func (s *Service) TransferPorticoOwnership(ctx context.Context, bearer, target string, who PorticoIdentity, expected int64) error {
	if target == "" || !validHostedAccountID(who.AccountID) {
		return ErrDirectInput
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	c, e := s.directCallerTx(ctx, tx, bearer, true)
	if e != nil {
		return e
	}
	var linked string
	if e = tx.QueryRowContext(ctx, `SELECT hosted_account_id FROM account_portico_links WHERE account_id=?`, c.account.ID).Scan(&linked); e != nil || linked != who.AccountID {
		// A valid credential with the wrong linked account is refused (403),
		// never unauthorized (401): a 401 would sign the owner out.
		return ErrForbidden
	}
	if e = s.transferOwnershipTx(ctx, tx, c, target, expected); e != nil {
		return e
	}
	return gated.Commit()
}

func (s *Service) transferOwnershipTx(ctx context.Context, tx *sql.Tx, c directCaller, target string, expected int64) error {
	var e error
	if c.account.Role != "owner" {
		return ErrForbidden
	}
	if target == c.account.ID || expected != c.account.Revision {
		return ErrUnauthorized
	}
	var active bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM direct_memberships WHERE account_id=? AND role='member' AND disabled=0)`, target).Scan(&active); e != nil {
		return e
	}
	if !active {
		return ErrUnauthorized
	}
	for _, id := range []string{c.account.ID, target} {
		if e = s.revokeDirectTx(ctx, tx, id, "", ""); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE direct_memberships SET role='member',allowed_libraries='[]',revision=revision+1 WHERE account_id=?`, c.account.ID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE direct_memberships SET role='owner',revision=revision+1 WHERE account_id=?`, target); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE accounts SET epoch=epoch+1 WHERE id IN(?,?)`, c.account.ID, target)
	return e
}
