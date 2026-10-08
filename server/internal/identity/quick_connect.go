package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/notify"
	"portico.local/server/internal/worker"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const quickAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

var setupUUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

type SetupProtocolError struct {
	Code     string
	Interval int
}

func (e *SetupProtocolError) Error() string { return e.Code }

type QuickStart struct {
	RequestID  string `json:"requestId"`
	DeviceName string `json:"deviceName"`
	Platform   string `json:"platform"`
	AppVersion string `json:"appVersion"`
}
type QuickAuthorization struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresAt               string `json:"expiresAt"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}
type QuickPreview struct {
	RequestID  string `json:"requestId"`
	UserCode   string `json:"userCode"`
	DeviceName string `json:"deviceName"`
	Platform   string `json:"platform"`
	AppVersion string `json:"appVersion"`
	ExpiresAt  string `json:"expiresAt"`
}

func (s *Service) setupHash(kind, value string) string {
	h := hmac.New(sha256.New, s.setupHashKey)
	h.Write([]byte("portico.setup.v1/" + kind + "\x00" + value))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func normalizedQuickCode(code string) (string, error) {
	code = strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' {
			return -1
		}
		return r
	}, code))
	if len(code) != 8 {
		return "", &SetupProtocolError{Code: "invalid_grant"}
	}
	for _, r := range code {
		if !strings.ContainsRune(quickAlphabet, r) {
			return "", &SetupProtocolError{Code: "invalid_grant"}
		}
	}
	return code, nil
}
func quickCode() (string, error) {
	b := make([]byte, 8)
	for i := range b {
		alphabet := quickAlphabet
		if i == 0 {
			alphabet = "23456789"
		}
		v, e := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if e != nil {
			return "", e
		}
		b[i] = alphabet[v.Int64()]
	}
	return string(b), nil
}
func setupLabel(value string, limit int) bool {
	if len(value) < 1 || len(value) > limit {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (s *Service) sealSetup(value any, _ string) ([]byte, error) {
	// Plain JSON: folder permissions are the protection, as in Plex. There is
	// no key, so there is no authentication tag either.
	return json.Marshal(value)
}
func (s *Service) openSetup(raw []byte, _ string, value any) error {
	if len(raw) == 0 || len(raw) > 64<<10 {
		return ErrUnauthorized
	}
	return json.Unmarshal(raw, value)
}
func (s *Service) SetupLimit(ctx context.Context, subject, kind string, limit int) error {
	var count int
	now := time.Now().Unix()
	// The counter is a write like any other: it goes through the gate, so it
	// never competes with a gated writer for SQLite's lock outside the ladder.
	e := dbwork.WithWriteTx(ctx, s.db, dbwork.ClassSecurityFence, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `INSERT INTO setup_rate_limits_v1(subject_hash,kind,window_start,attempts) VALUES(?,?,?,1) ON CONFLICT(subject_hash,kind) DO UPDATE SET attempts=CASE WHEN window_start<? THEN 1 ELSE attempts+1 END,window_start=CASE WHEN window_start<? THEN excluded.window_start ELSE window_start END RETURNING attempts`, s.setupHash("rate", subject), kind, now, now-600, now-600).Scan(&count)
	})
	if e != nil {
		return e
	}
	if count > limit {
		return &SetupProtocolError{Code: "rate_limited", Interval: 600}
	}
	return nil
}
func (s *Service) StartQuick(ctx context.Context, q QuickStart, private bool) (QuickAuthorization, error) {
	if !setupUUID.MatchString(q.RequestID) || !setupLabel(q.DeviceName, 100) || !setupLabel(q.Platform, 32) || !setupLabel(q.AppVersion, 40) {
		return QuickAuthorization{}, &SetupProtocolError{Code: "invalid_request"}
	}
	raw, _ := json.Marshal(q)
	requestHash := s.setupHash("quick-request", string(raw))
	createHash := s.setupHash("quick-create", q.RequestID)
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return QuickAuthorization{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	var hash, id string
	var cipher []byte
	var expiry int64
	e = tx.QueryRowContext(ctx, `SELECT id,request_hash,create_cipher,expires_at FROM quick_connect_v1 WHERE create_hash=?`, createHash).Scan(&id, &hash, &cipher, &expiry)
	if e == nil {
		if subtle.ConstantTimeCompare([]byte(hash), []byte(requestHash)) != 1 {
			return QuickAuthorization{}, ErrConflict
		}
		if expiry <= time.Now().Unix() {
			return QuickAuthorization{}, &SetupProtocolError{Code: "invalid_grant"}
		}
		var out QuickAuthorization
		if e = s.openSetup(cipher, "quick-create:"+q.RequestID, &out); e != nil {
			return out, e
		}
		out.ExpiresIn = max(0, int(expiry-time.Now().Unix()))
		return out, gated.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return QuickAuthorization{}, e
	}
	now := time.Now().UTC()
	id = Token()
	secret := Token()
	expiry = now.Add(10 * time.Minute).Unix()
	// The TV's installation is captured here, on its create request, so the
	// session issued at collect time binds the TV even when the collect
	// request carries no installation claim. Without a claim there is nothing
	// to capture; issue falls back to the collect request as before.
	var installation string
	if claim, ok := ctx.Value(issuingDeviceKey{}).(issuingDevice); ok && validInstallationID(claim.registration.InstallationID) {
		installation = claim.registration.InstallationID
	}
	// Active code reservation is unique inside this server only. Released hashes
	// are NULL, never recycled into a different credential family.
	if _, e = tx.ExecContext(ctx, `UPDATE quick_connect_v1 SET user_hash=NULL WHERE rowid IN (SELECT rowid FROM quick_connect_v1 WHERE expires_at<=? AND user_hash IS NOT NULL LIMIT 128)`, now.Unix()); e != nil {
		return QuickAuthorization{}, e
	}
	for attempt := 0; attempt < 8; attempt++ {
		code, e := quickCode()
		if e != nil {
			return QuickAuthorization{}, e
		}
		display := code[:4] + "-" + code[4:]
		out := QuickAuthorization{DeviceCode: secret, UserCode: display, VerificationURI: "https://web.getportico.tv/device", VerificationURIComplete: "https://web.getportico.tv/device#code=" + display, ExpiresAt: time.Unix(expiry, 0).UTC().Format(time.RFC3339), ExpiresIn: 600, Interval: 5}
		sealed, e := s.sealSetup(out, "quick-create:"+q.RequestID)
		if e != nil {
			return out, e
		}
		result, e := tx.ExecContext(ctx, `INSERT INTO quick_connect_v1(id,create_hash,request_hash,create_cipher,secret_hash,user_hash,device_name,platform,app_version,installation_id,private_request,expires_at,next_poll) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(user_hash) DO NOTHING`, id, createHash, requestHash, sealed, s.setupHash("quick-secret", secret), s.setupHash("quick-code", code), q.DeviceName, q.Platform, q.AppVersion, installation, private, expiry, now.Unix()+5)
		if e != nil {
			return out, e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return out, e
		}
		if n == 1 {
			return out, gated.Commit()
		}
	}
	return QuickAuthorization{}, ErrBusy
}
func (s *Service) ReviewQuick(ctx context.Context, code string) (QuickPreview, error) {
	normalized, e := normalizedQuickCode(code)
	if e != nil {
		return QuickPreview{}, e
	}
	var p QuickPreview
	var expiry int64
	e = s.db.QueryRowContext(ctx, `SELECT id,device_name,platform,app_version,expires_at FROM quick_connect_v1 WHERE user_hash=? AND status='pending' AND expires_at>?`, s.setupHash("quick-code", normalized), time.Now().Unix()).Scan(&p.RequestID, &p.DeviceName, &p.Platform, &p.AppVersion, &expiry)
	if errors.Is(e, sql.ErrNoRows) {
		return p, &SetupProtocolError{Code: "invalid_grant"}
	}
	p.UserCode = normalized[:4] + "-" + normalized[4:]
	p.ExpiresAt = time.Unix(expiry, 0).UTC().Format(time.RFC3339)
	return p, e
}
func (s *Service) DecideQuick(ctx context.Context, p Principal, id, code, decision string) error {
	normalized, e := normalizedQuickCode(code)
	if e != nil {
		return e
	}
	if p.Authority != "local" || !Grants(p.Role, TierMember) || len(id) > 128 || (decision != "approve" && decision != "deny") {
		return ErrUnauthorized
	}
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	if _, e = s.SessionFamilyTx(ctx, tx, p); e != nil {
		return e
	}
	var status string
	var owner sql.NullString
	var private bool
	var expiry int64
	e = tx.QueryRowContext(ctx, `SELECT status,approver_id,private_request,expires_at FROM quick_connect_v1 WHERE id=? AND user_hash=?`, id, s.setupHash("quick-code", normalized)).Scan(&status, &owner, &private, &expiry)
	if errors.Is(e, sql.ErrNoRows) {
		return &SetupProtocolError{Code: "invalid_grant"}
	}
	if e != nil {
		return e
	}
	if expiry <= time.Now().Unix() {
		return &SetupProtocolError{Code: "expired_token"}
	}
	next := "approved"
	if decision == "deny" {
		next = "denied"
	}
	if owner.String == p.AccountID && (status == next || decision == "approve" && status == "consumed") {
		return gated2.Commit()
	}
	if status != "pending" {
		return ErrConflict
	}
	approver := familyForTokenTx(ctx, tx, p.Hash)
	if e = s.recoveryAllowedTx(ctx, tx, p.AccountID, approver, private); e != nil {
		return e
	}
	// The approval only records who approved. The session is issued when the new
	// device collects it (PollQuick), inside that device's own request, so the
	// family is bound to the TV's installation and not to the approving phone.
	var cipher []byte
	if decision == "approve" {
		// Staging merge (lead quick-connect x be/member M9): the TV's family is
		// issued later, in PollQuick, so the approver's Portico admission travels
		// in the sealed approval and is inherited there.
		var viewerOnly bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM authorization_family_grants WHERE family_id=? AND grant_kind='viewer_only')`, approver).Scan(&viewerOnly); e != nil {
			return e
		}
		cipher, e = s.sealSetup(quickApproval{AccountID: p.AccountID, ProfileID: p.ProfileID, Epoch: p.Epoch, PorticoAdmitted: porticoFamilyTx(ctx, tx, approver), ViewerOnly: viewerOnly}, "quick-approval:"+id)
		if e != nil {
			return e
		}
	}
	if decision == "approve" {
		// Pairing a new device is a security event the account holder must see. It
		// commits with the approval, and is deduped by the pairing request, so a
		// retried approval is one notice rather than two.
		var label string
		if e = tx.QueryRowContext(ctx, `SELECT device_name||' ('||platform||')' FROM quick_connect_v1 WHERE id=?`, id).Scan(&label); e != nil {
			return e
		}
		if _, e = notify.NotifySecurityEvent(tx, time.Now().UnixMilli(), "local", p.AccountID, p.ProfileID, notify.SecurityNewDevice, id, label); e != nil {
			return e
		}
	}
	// Approval, family issuance and recoverable encrypted delivery are ONE commit.
	result, e := tx.ExecContext(ctx, `UPDATE quick_connect_v1 SET status=?,approver_id=?,grant_cipher=? WHERE id=? AND status='pending'`, next, p.AccountID, cipher, id)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return ErrConflict
	}
	return gated2.Commit()
}
func (s *Service) PollQuick(ctx context.Context, secret string) (Envelope, error) {
	if len(secret) != 43 {
		return Envelope{}, &SetupProtocolError{Code: "invalid_grant"}
	}
	gated3, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return Envelope{}, e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	var id, status, tokenHash string
	var expiry, next int64
	var recovery sql.NullInt64
	var interval int
	var cipher []byte
	e = tx.QueryRowContext(ctx, `SELECT id,status,expires_at,next_poll,interval_seconds,grant_cipher,COALESCE(token_hash,''),recovery_until FROM quick_connect_v1 WHERE secret_hash=?`, s.setupHash("quick-secret", secret)).Scan(&id, &status, &expiry, &next, &interval, &cipher, &tokenHash, &recovery)
	if errors.Is(e, sql.ErrNoRows) {
		return Envelope{}, &SetupProtocolError{Code: "invalid_grant"}
	}
	if e != nil {
		return Envelope{}, e
	}
	now := time.Now().Unix()
	if status == "denied" || status == "cancelled" {
		return Envelope{}, &SetupProtocolError{Code: "access_denied"}
	}
	if status == "expired" || status != "consumed" && expiry <= now || status == "consumed" && (!recovery.Valid || recovery.Int64 <= now) {
		return Envelope{}, &SetupProtocolError{Code: "expired_token"}
	}
	if next > now {
		interval = min(300, interval+5)
		if _, e = tx.ExecContext(ctx, `UPDATE quick_connect_v1 SET interval_seconds=?,next_poll=? WHERE id=?`, interval, now+int64(interval), id); e != nil {
			return Envelope{}, e
		}
		if e = gated3.Commit(); e != nil {
			return Envelope{}, e
		}
		return Envelope{}, &SetupProtocolError{Code: "slow_down", Interval: interval}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE quick_connect_v1 SET next_poll=? WHERE id=?`, now+int64(interval), id); e != nil {
		return Envelope{}, e
	}
	if status == "pending" {
		if e = gated3.Commit(); e != nil {
			return Envelope{}, e
		}
		return Envelope{}, &SetupProtocolError{Code: "authorization_pending", Interval: interval}
	}
	if status == "approved" && tokenHash == "" {
		out, e := s.issueQuickTx(ctx, tx, id, cipher)
		if e != nil {
			return Envelope{}, e
		}
		return out, gated3.Commit()
	}
	r, e := s.familyTokenTx(ctx, tx, tokenHash)
	if e != nil {
		return Envelope{}, &SetupProtocolError{Code: "access_denied"}
	}
	if _, e = s.SessionFamilyTx(ctx, tx, r.principal); e != nil || r.TokenGeneration != 1 {
		return Envelope{}, &SetupProtocolError{Code: "access_denied"}
	}
	var out Envelope
	if e = s.openSetup(cipher, "quick-grant:"+id, &out); e != nil {
		return out, e
	}
	if out.SessionFamilyID != r.ID || Digest(out.AccessToken) != tokenHash {
		return Envelope{}, ErrUnauthorized
	}
	if status != "consumed" {
		if _, e = tx.ExecContext(ctx, `UPDATE quick_connect_v1 SET status='consumed',recovery_until=? WHERE id=?`, now+300, id); e != nil {
			return Envelope{}, e
		}
	}
	return out, gated3.Commit()
}
func (s *Service) CancelQuick(ctx context.Context, secret string) error {
	if len(secret) != 43 {
		return &SetupProtocolError{Code: "invalid_grant"}
	}
	gated4, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated4.Tx()
	defer gated4.Rollback()
	var id string
	var hash sql.NullString
	e = tx.QueryRowContext(ctx, `SELECT id,token_hash FROM quick_connect_v1 WHERE secret_hash=?`, s.setupHash("quick-secret", secret)).Scan(&id, &hash)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if hash.Valid && hash.String != "" {
		r, e := s.familyTokenTx(ctx, tx, hash.String)
		if e != nil {
			return e
		}
		if e = s.revokeSetupFamilyTx(ctx, tx, r); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE quick_connect_v1 SET status='cancelled',grant_cipher=NULL,user_hash=NULL WHERE id=?`, id); e != nil {
		return e
	}
	return gated4.Commit()
}
func (s *Service) revokeSetupFamilyTx(ctx context.Context, tx *sql.Tx, r familyRecord) error {
	if e := RevokeFamilyTx(ctx, tx, r.ID, RevokedExplicitSignout); e != nil {
		return e
	}
	if s.OnFamilyRevokedTx != nil {
		return s.OnFamilyRevokedTx(ctx, tx, r.FamilyState)
	}
	return nil
}
func (s *Service) PruneSetup(ctx context.Context) error {
	gated5, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return e
	}
	tx := gated5.Tx()
	defer gated5.Rollback()
	now := time.Now().Unix()
	rows, e := tx.QueryContext(ctx, `SELECT token_hash FROM quick_connect_v1 WHERE status='approved' AND expires_at<=? LIMIT 64`, now)
	if e != nil {
		return e
	}
	var hashes []string
	for rows.Next() {
		var hash string
		if e = rows.Scan(&hash); e != nil {
			rows.Close()
			return e
		}
		hashes = append(hashes, hash)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, hash := range hashes {
		r, e := s.familyTokenTx(ctx, tx, hash)
		if e != nil {
			return e
		}
		if e = s.revokeSetupFamilyTx(ctx, tx, r); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE quick_connect_v1 SET status='expired',grant_cipher=NULL,user_hash=NULL WHERE token_hash=? AND status='approved'`, hash); e != nil {
			return e
		}
	}
	for _, q := range []string{
		`UPDATE quick_connect_v1 SET user_hash=NULL WHERE rowid IN (SELECT rowid FROM quick_connect_v1 WHERE expires_at<=? AND user_hash IS NOT NULL LIMIT 128)`,
		`UPDATE quick_connect_v1 SET grant_cipher=NULL WHERE rowid IN (SELECT rowid FROM quick_connect_v1 WHERE recovery_until<=? AND grant_cipher IS NOT NULL LIMIT 128)`,
		`UPDATE onboarding_state_v1 SET receipt=NULL WHERE receipt_until<=?`,
	} {
		if _, e = tx.ExecContext(ctx, q, now); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM quick_connect_v1 WHERE rowid IN (SELECT rowid FROM quick_connect_v1 WHERE expires_at<? AND status!='approved' LIMIT 128)`, now-86400); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM setup_rate_limits_v1 WHERE rowid IN (SELECT rowid FROM setup_rate_limits_v1 WHERE window_start<? LIMIT 128)`, now-86400); e != nil {
		return e
	}
	return gated5.Commit()
}

// RunSetupMaintenance prunes expired sign-in codes, renewal records and
// identity proofs. All three only ever have something to prune after a write,
// so the loop is woken by commits: an idle server prunes nothing and asks
// nothing, where before it issued three delete transactions every thirty
// seconds whether or not a single row had been created since it started.
func (s *Service) RunSetupMaintenance(ctx context.Context) {
	wake := worker.NewSignal()
	dbwork.WakeOnCommit(wake)
	lastSessionPrune := time.Time{}
	worker.Run(ctx, "identity.setup-maintenance", wake, func(ctx context.Context) time.Duration {
		step, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = s.PruneSetup(step)
		_, _ = s.PruneAuthorizationRenewals(step)
		if time.Since(lastSessionPrune) >= 15*time.Minute {
			if removed, err := s.PruneSessionHistory(step); err == nil && removed < 128 {
				lastSessionPrune = time.Now()
			}
		}
		_ = s.PruneIdentityProofs(step)
		return 0
	})
}

// quickApproval is what an approving session leaves for the device to collect.
type quickApproval struct {
	AccountID string `json:"accountId"`
	ProfileID string `json:"profileId"`
	Epoch     int    `json:"epoch"`
	// PorticoAdmitted: the approving session was admitted through Portico
	// sign-in (be/member M9), so the device's family inherits that admission.
	PorticoAdmitted bool `json:"porticoAdmitted,omitempty"`
	// ViewerOnly: the approving session was itself a viewer-only one. A device
	// signed in by it can do no more than it could.
	ViewerOnly bool `json:"viewerOnly,omitempty"`
}

// issueQuickTx signs the collecting device in as whoever approved the code, in
// the device's own request (its installation claim is in ctx), then stores the
// envelope so a lost answer can be collected again until recovery_until.
//
// The device gets the session that person would get by signing in on it: an
// owner who approves a code has an owner's Apple TV, a member a member's. The
// role is read here, at collection, and not carried in the approval, so a role
// that changed in between is the one the device gets.
func (s *Service) issueQuickTx(ctx context.Context, tx *sql.Tx, id string, cipher []byte) (Envelope, error) {
	var a quickApproval
	if e := s.openSetup(cipher, "quick-approval:"+id, &a); e != nil {
		return Envelope{}, e
	}
	var epoch int
	var disabled, primary bool
	var role string
	e := tx.QueryRowContext(ctx, `SELECT a.epoch,m.disabled,m.role,COALESCE((SELECT is_primary FROM direct_profiles WHERE account_id=a.id AND id=? AND deleted=0),0) FROM accounts a JOIN direct_memberships m ON m.account_id=a.id WHERE a.id=?`, a.ProfileID, a.AccountID).Scan(&epoch, &disabled, &role, &primary)
	if errors.Is(e, sql.ErrNoRows) || e == nil && (epoch != a.Epoch || disabled) {
		// The approver's credentials changed or access ended since approving.
		return Envelope{}, &SetupProtocolError{Code: "access_denied"}
	}
	if e != nil {
		return Envelope{}, e
	}
	// The TV's installation was captured at create time. Issuing against it —
	// rather than whatever the collect request happens to carry — is what
	// keeps the grant renewable when the collect arrives without headers. An
	// empty stored value (an older row, or a create without a claim) keeps the
	// previous behaviour: the collect request's own installation, if any.
	var installation, deviceName, platform, appVersion string
	if e = tx.QueryRowContext(ctx, `SELECT installation_id,device_name,platform,app_version FROM quick_connect_v1 WHERE id=?`, id).Scan(&installation, &deviceName, &platform, &appVersion); e != nil {
		return Envelope{}, e
	}
	if validInstallationID(installation) {
		claim, _ := ctx.Value(issuingDeviceKey{}).(issuingDevice)
		registration := DeviceRegistration{InstallationID: installation, Name: deviceName, Platform: platform, AppVersion: appVersion}
		if registration.valid() {
			ctx, _ = WithIssuingDevice(ctx, registration, claim.peer)
		}
	}
	// The account's role belongs to its primary profile; any other profile is a
	// member, exactly as when that profile is chosen at sign-in (SelectProfile).
	if !primary || a.ViewerOnly {
		role = TierMember
	}
	out, e := s.IssueTx(ctx, tx, a.AccountID, a.ProfileID, "local", role, a.Epoch, time.Time{})
	if e != nil {
		return out, e
	}
	if a.ViewerOnly {
		if _, e = tx.ExecContext(ctx, `INSERT INTO authorization_family_grants(family_id,grant_kind) VALUES(?,'viewer_only')`, out.SessionFamilyID); e != nil {
			return out, e
		}
	}
	if a.PorticoAdmitted {
		if _, e = tx.ExecContext(ctx, `INSERT OR IGNORE INTO portico_admitted_families(family_id) VALUES(?)`, out.SessionFamilyID); e != nil {
			return out, e
		}
	}
	sealed, e := s.sealSetup(out, "quick-grant:"+id)
	if e != nil {
		return out, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE quick_connect_v1 SET status='consumed',grant_cipher=?,family_id=?,token_hash=?,recovery_until=? WHERE id=? AND status='approved'`, sealed, out.SessionFamilyID, Digest(out.AccessToken), time.Now().Unix()+300, id)
	return out, e
}
