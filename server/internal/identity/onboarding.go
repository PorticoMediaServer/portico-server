package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"os"
	"portico.local/server/internal/dbwork"
	"strings"
	"time"
)

type SetupOptions struct {
	RequestID     string `json:"requestId,omitempty"`
	SetupToken    string `json:"setupToken"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Name          string `json:"name"`
	AuthMode      string `json:"authMode,omitempty"`
	RecoverySaved bool   `json:"recoverySaved,omitempty"`
	Interactive   bool   `json:"interactive,omitempty"`
}
type OnboardingState struct {
	AuthMode          string `json:"authMode"`
	Phase             string `json:"phase"`
	Ready             bool   `json:"ready"`
	RecoveryOwner     bool   `json:"recoveryOwner"`
	RecoveryConfirmed bool   `json:"recoveryConfirmed"`
	RemoteRecovery    bool   `json:"remoteRecovery"`
	ClaimInstalled    bool   `json:"claimInstalled"`
	Revision          int64  `json:"revision,string"`
	SessionLabel      string `json:"sessionLabel"`
}

func setupOperationID() string {
	raw := make([]byte, 16)
	if _, e := rand.Read(raw); e != nil {
		panic(e)
	}
	raw[6] = (raw[6] & 15) | 64
	raw[8] = (raw[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

// SetupWithOptions has a stable resume key and commits the local owner, chosen
// mode, native family and encrypted receipt together. It never creates a Hosted
// account or assumes an unacknowledged claim succeeded.
func (s *Service) SetupWithOptions(ctx context.Context, q SetupOptions, private bool) (Envelope, error) {
	if q.AuthMode == "" {
		q.AuthMode = "local"
	}
	if q.RequestID == "" {
		q.RequestID = setupOperationID()
	}
	q.Username = strings.ToLower(strings.TrimSpace(q.Username))
	q.Name = strings.TrimSpace(q.Name)
	if !setupUUID.MatchString(q.RequestID) || (q.AuthMode != "hosted" && q.AuthMode != "local") || len(q.Username) < 3 || len(q.Username) > 64 || !directPassword(q.Password) || !setupLabel(q.Name, 100) {
		return Envelope{}, errors.New("Choose a server name, a username (3-64 characters) and a password of at least 8 characters")
	}
	if q.AuthMode == "hosted" && !q.RecoverySaved {
		return Envelope{}, errors.New("Confirm that you saved the recovery owner's password")
	}
	raw, _ := json.Marshal(q)
	defer clear(raw)
	requestHash := s.setupHash("initialize-request", string(raw))
	// A repeated create is recoverable even after the one-time file is removed.
	var existing string
	e := s.db.QueryRowContext(ctx, `SELECT operation_id FROM onboarding_state_v1 WHERE singleton=1`).Scan(&existing)
	if e == nil {
		if existing != q.RequestID {
			return Envelope{}, ErrConflict
		}
		return s.resumeSetup(ctx, q.RequestID, q.SetupToken, requestHash, private)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return Envelope{}, e
	}
	if !s.SetupRequired() {
		return Envelope{}, ErrConflict
	}
	token, e := os.ReadFile(s.setupPath)
	if e != nil || subtle.ConstantTimeCompare(token, []byte(q.SetupToken)) != 1 {
		return Envelope{}, ErrUnauthorized
	}
	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	default:
		return Envelope{}, ErrBusy
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(q.Password), PasswordCost)
	if e != nil {
		return Envelope{}, e
	}
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return Envelope{}, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	var count int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM accounts`).Scan(&count); e != nil {
		return Envelope{}, e
	}
	if count != 0 {
		return Envelope{}, ErrConflict
	}
	aid, pid := Token(), Token()
	if _, e = tx.ExecContext(ctx, `INSERT INTO accounts(id,username,password_hash,profile_id) VALUES(?,?,?,?)`, aid, q.Username, hash, pid); e != nil {
		return Envelope{}, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO configuration(key,value) VALUES('name',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, q.Name); e != nil {
		return Envelope{}, e
	}
	out, e := s.IssueTx(ctx, tx, aid, pid, "local", "owner", 1, time.Time{})
	if e != nil {
		return out, e
	}
	sealed, e := s.sealSetup(out, "initialize:"+q.RequestID)
	if e != nil {
		return out, e
	}
	recovery := ""
	// Clients no longer show a setup checklist: a local server is ready once its
	// owner exists. A hosted one becomes ready when its claim is installed.
	ready := q.AuthMode == "local"
	if q.AuthMode == "hosted" {
		recovery = aid
	}
	// A server set up from outside the LAN keeps its recovery owner usable from
	// there too, so the person who set it up is never locked out. The owner can
	// switch remote recovery off afterwards.
	if _, e = tx.ExecContext(ctx, `INSERT INTO onboarding_state_v1(singleton,auth_mode,operation_id,request_hash,setup_hash,receipt,receipt_until,recovery_account,recovery_confirmed,remote_recovery,ready) VALUES(1,?,?,?,?,?,?,?,?,?,?)`, q.AuthMode, q.RequestID, requestHash, s.setupHash("initialize-secret", q.SetupToken), sealed, time.Now().Unix()+setupReceiptSeconds, recovery, q.RecoverySaved, !private, ready); e != nil {
		return out, e
	}
	if e = gated.Commit(); e != nil {
		return Envelope{}, e
	}
	_ = os.Remove(s.setupPath)
	return out, nil
}

// A reload or closed tab resumes setup within this window; after it, the owner
// signs in with the password they just chose.
const setupReceiptSeconds = 900

func (s *Service) ResumeSetup(ctx context.Context, id, secret string, private bool) (Envelope, error) {
	return s.resumeSetup(ctx, id, secret, "", private)
}
func (s *Service) resumeSetup(ctx context.Context, id, secret, requestHash string, private bool) (Envelope, error) {
	if !setupUUID.MatchString(id) || len(secret) != 43 {
		return Envelope{}, ErrUnauthorized
	}
	gated2, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return Envelope{}, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	var storedSecret, storedRequest string
	var cipher []byte
	var until int64
	e = tx.QueryRowContext(ctx, `SELECT setup_hash,request_hash,receipt,receipt_until FROM onboarding_state_v1 WHERE singleton=1 AND operation_id=?`, id).Scan(&storedSecret, &storedRequest, &cipher, &until)
	if errors.Is(e, sql.ErrNoRows) {
		return Envelope{}, e
	}
	if e != nil {
		return Envelope{}, e
	}
	if subtle.ConstantTimeCompare([]byte(storedSecret), []byte(s.setupHash("initialize-secret", secret))) != 1 || requestHash != "" && subtle.ConstantTimeCompare([]byte(storedRequest), []byte(requestHash)) != 1 {
		return Envelope{}, ErrUnauthorized
	}
	if until <= time.Now().Unix() || len(cipher) == 0 {
		return Envelope{}, ErrUnauthorized
	}
	var out Envelope
	if e = s.openSetup(cipher, "initialize:"+id, &out); e != nil {
		return out, e
	}
	record, e := s.familyTokenTx(ctx, tx, Digest(out.AccessToken))
	if e != nil {
		return Envelope{}, e
	}
	if _, e = s.SessionFamilyTx(ctx, tx, record.principal); e != nil {
		return Envelope{}, e
	}
	if e = s.recoveryPasswordAllowedTx(ctx, tx, out.Viewer.AccountID, private); e != nil {
		return Envelope{}, e
	}
	return out, gated2.Commit()
}

// recoveryPasswordAllowedTx is the recovery owner's password rule: from the
// private network only, unless the owner turned remote recovery on.
func (s *Service) recoveryPasswordAllowedTx(ctx context.Context, tx *sql.Tx, account string, private bool) error {
	var recovery string
	var remote bool
	e := tx.QueryRowContext(ctx, `SELECT recovery_account,remote_recovery FROM onboarding_state_v1 WHERE singleton=1`).Scan(&recovery, &remote)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if recovery == account && !remote && !private {
		return ErrUnauthorized
	}
	return nil
}

// recoveryAllowedTx guards an established session. A recovery owner that is
// also a Portico Account may sign in remotely through Hosted, so a session
// family admitted that way (portico_admitted_families) is usable from
// anywhere; a session that came from the recovery password stays
// private-network only (INT M9).
func (s *Service) recoveryAllowedTx(ctx context.Context, tx *sql.Tx, account, family string, private bool) error {
	e := s.recoveryPasswordAllowedTx(ctx, tx, account, private)
	if !errors.Is(e, ErrUnauthorized) {
		return e
	}
	if !porticoFamilyTx(ctx, tx, family) {
		return e
	}
	return nil
}
func (s *Service) CheckRecoveryRoute(ctx context.Context, p Principal, private bool) error {
	if p.Authority != "local" || private {
		return nil
	}
	gated3, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	return s.recoveryAllowedTx(ctx, tx, p.AccountID, familyForTokenTx(ctx, tx, p.Hash), private)
}
func (s *Service) LoginFrom(ctx context.Context, user, password string, private bool) (Envelope, error) {
	return s.loginFrom(ctx, user, password, private)
}

func (s *Service) Onboarding(ctx context.Context, p Principal) (OnboardingState, error) {
	if p.Authority != "local" || p.Role != "owner" {
		return OnboardingState{}, ErrForbidden
	}
	gated4, e := dbwork.BeginSnapshot(ctx, s.db)
	if e != nil {
		return OnboardingState{}, e
	}
	tx := gated4.Tx()
	defer gated4.Rollback()
	if _, e = s.SessionFamilyTx(ctx, tx, p); e != nil {
		return OnboardingState{}, e
	}
	out, e := s.onboardingTx(ctx, tx, p)
	if e != nil {
		return out, e
	}
	return out, gated4.Commit()
}
func (s *Service) onboardingTx(ctx context.Context, tx *sql.Tx, p Principal) (OnboardingState, error) {
	out := OnboardingState{AuthMode: "local", Ready: true, Phase: "ready", SessionLabel: "This Server"}
	var recovery string
	e := tx.QueryRowContext(ctx, `SELECT auth_mode,ready,recovery_account,recovery_confirmed,remote_recovery,revision FROM onboarding_state_v1 WHERE singleton=1`).Scan(&out.AuthMode, &out.Ready, &recovery, &out.RecoveryConfirmed, &out.RemoteRecovery, &out.Revision)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	if s.SetupClaimReadyTx != nil {
		out.ClaimInstalled, e = s.SetupClaimReadyTx(ctx, tx)
		if e != nil {
			return out, e
		}
	}
	out.RecoveryOwner = recovery != "" && recovery == p.AccountID
	// Nothing asks the owner to press "Finish" any more: a hosted server whose
	// claim is installed and whose recovery owner is confirmed is ready.
	if out.AuthMode == "hosted" && !out.Ready && out.RecoveryConfirmed && out.ClaimInstalled {
		out.Ready = true
		out.Phase = "ready"
	}
	if out.RecoveryOwner {
		out.SessionLabel = "This Server \u00b7 Recovery Owner"
	}
	if out.AuthMode == "hosted" && !out.Ready {
		out.Phase = "waiting_for_web"
		if out.ClaimInstalled {
			out.Phase = "recovery_owner_credential_confirmed"
		}
	}
	return out, nil
}
func (s *Service) FinishSetup(ctx context.Context, p Principal, expected int64) (OnboardingState, error) {
	gated5, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return OnboardingState{}, e
	}
	tx := gated5.Tx()
	defer gated5.Rollback()
	if _, e = s.SessionFamilyTx(ctx, tx, p); e != nil {
		return OnboardingState{}, e
	}
	out, e := s.onboardingTx(ctx, tx, p)
	if e != nil {
		return out, e
	}
	if p.Authority != "local" || p.Role != "owner" || out.Revision != expected {
		return out, ErrConflict
	}
	if out.AuthMode == "hosted" && (!out.RecoveryOwner || !out.RecoveryConfirmed || !out.ClaimInstalled) {
		return out, errors.New("Complete the current claim and confirm the saved local recovery credential before finishing setup")
	}
	if !out.Ready {
		if _, e = tx.ExecContext(ctx, `UPDATE onboarding_state_v1 SET ready=1,revision=revision+1,receipt_until=MIN(receipt_until,?) WHERE singleton=1 AND revision=?`, time.Now().Unix()+300, expected); e != nil {
			return out, e
		}
		out.Ready = true
		out.Phase = "ready"
		out.Revision++
	}
	return out, gated5.Commit()
}

// This is a distinct, warned local-owner setting. Ordinary remote-access opt-in
// never enables recovery login and forwarded headers never count as a LAN peer.
func (s *Service) SetRemoteRecovery(ctx context.Context, p Principal, expected int64, enabled, warningConfirmed bool) (OnboardingState, error) {
	if enabled && !warningConfirmed {
		return OnboardingState{}, ErrUnauthorized
	}
	gated6, e := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if e != nil {
		return OnboardingState{}, e
	}
	tx := gated6.Tx()
	defer gated6.Rollback()
	if _, e = s.SessionFamilyTx(ctx, tx, p); e != nil {
		return OnboardingState{}, e
	}
	out, e := s.onboardingTx(ctx, tx, p)
	if e != nil {
		return out, e
	}
	if !out.RecoveryOwner || out.Revision != expected {
		return out, ErrConflict
	}
	if _, e = tx.ExecContext(ctx, `UPDATE onboarding_state_v1 SET remote_recovery=?,revision=revision+1 WHERE singleton=1 AND revision=?`, enabled, expected); e != nil {
		return out, e
	}
	out.RemoteRecovery = enabled
	out.Revision++
	return out, gated6.Commit()
}

// BrowserSetupToken is only exposed by the local, origin-checked first-run route.
// The normal setup transaction still consumes the token atomically; no login or
// initialized-server route can retrieve it.
func (s *Service) BrowserSetupToken(ctx context.Context) (string, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM accounts").Scan(&count); err != nil {
		return "", err
	}
	if count != 0 {
		return "", ErrConflict
	}
	token, err := os.ReadFile(s.setupPath)
	if err != nil {
		return "", err
	}
	return string(token), nil
}
