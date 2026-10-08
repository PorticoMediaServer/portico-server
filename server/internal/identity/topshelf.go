package identity

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"portico.local/server/internal/dbwork"
	"time"
)

var ErrTopShelfToken = errors.New("This Top Shelf feed link has expired. Open Portico on this device to refresh it.")

// TopShelfGrant is what a feed token resolves to. It carries the viewer facts a
// read needs and nothing a session carries: no family token, no renewal, no way
// to widen itself.
type TopShelfGrant struct {
	Viewer    Viewer
	Hash      string
	Epoch     int
	Token     string
	ExpiresAt string
}

// IssueTopShelfToken mints a feed token for one device. The caller must hold a
// live viewing session, and the device must be registered and approved on the
// same account: the extension inherits exactly what the app already had.
// MaxTopShelfLifetime bounds a feed token. The television's home screen asks for the feed
// when nobody is using the app, so a token that lasted a day would leave the shelf empty for
// anyone who watches at weekends. The token can read one small feed and its posters, for one
// profile on one approved device, and it dies at once with the device, the profile or the
// account; thirty days costs little and keeps the shelf filled.
const MaxTopShelfLifetime = 30 * 24 * time.Hour

func (s *Service) IssueTopShelfToken(ctx context.Context, p Principal, installation string, lifetime time.Duration) (TopShelfGrant, error) {
	if !validInstallationID(installation) || p.AccountID == "" || p.ProfileID == "" {
		return TopShelfGrant{}, ErrDeviceInput
	}
	if lifetime <= 0 || lifetime > MaxTopShelfLifetime {
		lifetime = MaxTopShelfLifetime
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return TopShelfGrant{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if _, e = s.SessionFamilyTx(ctx, tx, p); e != nil {
		return TopShelfGrant{}, e
	}
	if p.Authority != "local" && p.Authority != "hosted" {
		return TopShelfGrant{}, ErrDeviceInput
	}
	device, e := s.deviceTx(ctx, tx, p.Authority, p.AccountID, installation, "")
	if e != nil {
		return TopShelfGrant{}, e
	}
	if device.ApprovalState != "approved" {
		return TopShelfGrant{}, ErrDevicePending
	}
	now := time.Now().UTC()
	out := TopShelfGrant{Viewer: p.Viewer, Hash: p.Hash, Epoch: p.Epoch, Token: Token(), ExpiresAt: now.Add(lifetime).Format(time.RFC3339)}
	if _, e = tx.ExecContext(ctx, `DELETE FROM topshelf_tokens WHERE expires_at<=? OR (device_id=? AND profile_id=?)`, now.Format(time.RFC3339), device.ID, p.ProfileID); e != nil {
		return TopShelfGrant{}, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO topshelf_tokens(token_hash,device_id,authority,account_id,profile_id,expires_at) VALUES(?,?,?,?,?,?)`, Digest(out.Token), device.ID, p.Authority, p.AccountID, p.ProfileID, out.ExpiresAt); e != nil {
		return TopShelfGrant{}, e
	}
	return out, gated.Commit()
}

// TopShelfGrantFor resolves a feed token. Expiry, account epoch and the device's
// approval state are all re-proven here, so revoking the device or bumping the
// account epoch takes the feed away without a separate sweep.
func (s *Service) TopShelfGrant(ctx context.Context, token string) (TopShelfGrant, error) {
	if len(token) != 43 {
		return TopShelfGrant{}, ErrTopShelfToken
	}
	gated2, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return TopShelfGrant{}, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	var authority, account, profile, device, expiry string
	if e = tx.QueryRowContext(ctx, `SELECT authority,account_id,profile_id,device_id,expires_at FROM topshelf_tokens WHERE token_hash=?`, Digest(token)).Scan(&authority, &account, &profile, &device, &expiry); e != nil {
		return TopShelfGrant{}, ErrTopShelfToken
	}
	at, err := time.Parse(time.RFC3339, expiry)
	if err != nil || !at.After(time.Now().UTC()) {
		return TopShelfGrant{}, ErrTopShelfToken
	}
	var state string
	if e = tx.QueryRowContext(ctx, `SELECT approval_state FROM identity_devices WHERE id=? AND authority=? AND account_id=?`, device, authority, account).Scan(&state); e != nil || state != "approved" {
		return TopShelfGrant{}, ErrTopShelfToken
	}
	role, epoch, e := s.topShelfViewerTx(ctx, tx, authority, account, profile)
	if e != nil {
		return TopShelfGrant{}, ErrTopShelfToken
	}
	return TopShelfGrant{
		Viewer:    Viewer{AccountID: account, ProfileID: profile, ServerID: s.ID(), Authority: authority, Role: role},
		Hash:      Digest(token),
		Epoch:     epoch,
		Token:     token,
		ExpiresAt: expiry,
	}, nil
}

// topShelfViewerTx re-derives the viewer a feed token stands for, so nothing about it is
// carried in the token itself.
//
// A local account is read from this server's own account and profile tables. A Portico Account
// is read from its live viewing families here on this server — no call to Hosted Services, and
// no cached copy of anything: the family is what the policy already wrote, and when the account
// service removes the member their families are revoked, which takes the feed away with them.
func (s *Service) topShelfViewerTx(ctx context.Context, tx *sql.Tx, authority, account, profile string) (string, int, error) {
	if authority == "hosted" {
		var role string
		var epoch int
		e := tx.QueryRowContext(ctx, `SELECT role,epoch FROM authorization_session_families WHERE authority='hosted' AND account_id=? AND profile_id=? AND revoked=0 ORDER BY epoch DESC LIMIT 1`, account, profile).Scan(&role, &epoch)
		if e != nil {
			return "", 0, e
		}
		return role, epoch, nil
	}
	c, e := s.directAccountTx(ctx, tx, account)
	if e != nil {
		return "", 0, e
	}
	var primary, deleted int
	if e = tx.QueryRowContext(ctx, `SELECT is_primary,deleted FROM direct_profiles WHERE account_id=? AND id=?`, account, profile).Scan(&primary, &deleted); e != nil || deleted != 0 {
		return "", 0, ErrTopShelfToken
	}
	role := c.account.Role
	if primary == 0 {
		role = "member"
	}
	return role, c.epoch, nil
}

// Artwork capabilities disclose one item's art for five minutes. The feed token
// remains encrypted inside the capability and its live device grant is checked
// again at use, so signing the device out revokes outstanding image URLs.
func (s *Service) TopShelfArtworkCapability(token, item string) (string, error) {
	if len(token) != 43 || !validFamilyID(item) {
		return "", ErrTopShelfToken
	}
	raw, err := s.sealSetup(struct {
		Token   string
		Expires int64
	}{token, time.Now().Add(5 * time.Minute).Unix()}, "topshelf-art:"+item)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func (s *Service) ResolveTopShelfArtwork(ctx context.Context, capability, item string) (TopShelfGrant, error) {
	if len(capability) > 2048 || !validFamilyID(item) {
		return TopShelfGrant{}, ErrTopShelfToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(capability)
	if err != nil {
		return TopShelfGrant{}, ErrTopShelfToken
	}
	var payload struct {
		Token   string
		Expires int64
	}
	if err = s.openSetup(raw, "topshelf-art:"+item, &payload); err != nil || payload.Expires <= time.Now().Unix() {
		return TopShelfGrant{}, ErrTopShelfToken
	}
	return s.TopShelfGrant(ctx, payload.Token)
}
