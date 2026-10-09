package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"log"
	"os"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
	"time"
)

var ErrUnauthorized = errors.New("Authentication is required.")

// ErrForbidden and ErrNotVisible refuse a caller whose credential is valid
// (CD-51). HTTP answers them 403 and 404, never 401: clients treat 401 as "your
// sign-in ended" and send the person back to the sign-in screen, so a 401 for
// a library a member is simply not given, or an owner-only page, signed people
// out. ErrNotVisible is for things whose existence stays hidden (a library the
// profile is not given, another profile's recordings, per SEC-02).
//
// Both still satisfy errors.Is(err, ErrUnauthorized), so every internal check
// that treats "unauthorized" as a refusal keeps doing so unchanged; only the
// HTTP answer differs. Use ErrUnauthorized itself only for a missing, invalid,
// expired or revoked credential.
var ErrForbidden error = &refusal{"This action is not permitted for this profile."}
var ErrNotVisible error = &refusal{"Not found."}

type refusal struct{ message string }

func (e *refusal) Error() string        { return e.message }
func (e *refusal) Is(target error) bool { return target == ErrUnauthorized }

// Refusal classifies an authorization-family error for an HTTP answer: 401
// only for an authentication failure, 403 for a refusal, 404 for a hidden
// thing. ok is false for any other error.
func Refusal(err error) (status int, code string, ok bool) {
	switch {
	case errors.Is(err, ErrNotVisible):
		return 404, "not_found", true
	case errors.Is(err, ErrForbidden):
		return 403, "forbidden", true
	case errors.Is(err, ErrUnauthorized):
		return 401, "authentication_required", true
	}
	return 0, "", false
}

var ErrBusy = errors.New("Sign-in is busy. Wait a moment and try again.")
var ErrConflict = errors.New("already configured")

type Viewer struct {
	AccountID string `json:"accountId"`
	ProfileID string `json:"profileId"`
	ServerID  string `json:"serverId"`
	Authority string `json:"authority"`
	Role      string `json:"role"`
}
type Principal struct {
	Viewer
	Hash  string
	Epoch int
	// DeviceID is verified accounting metadata, never an authorization claim.
	DeviceID string `json:"-"`
}

// NativeServerIdentity is a pin delivered only with a successful native session.
// It is read in the issuing transaction; it is not a claim or a remote grant.
type NativeServerIdentity struct {
	PublicKey   string `json:"publicKey"`
	Fingerprint string `json:"fingerprint"`
}
type Envelope struct {
	ServerIdentity       *NativeServerIdentity `json:"serverIdentity,omitempty"`
	AccessToken          string                `json:"accessToken"`
	RefreshToken         string                `json:"refreshToken,omitempty"`
	DeviceID             string                `json:"deviceId"`
	InstallationID       string                `json:"installationId"`
	ExpiresAt            string                `json:"expiresAt"`
	Viewer               Viewer                `json:"viewer"`
	SessionFamilyID      string                `json:"sessionFamilyId"`
	TokenGeneration      int64                 `json:"tokenGeneration,string"`
	AuthorizationHorizon string                `json:"authorizationHorizon"`
}
type Service struct {
	db                  *sql.DB
	serverID, setupPath string
	hashSlots           chan struct{}
	setupHashKey        []byte
	SetupClaimReadyTx   func(context.Context, *sql.Tx) (bool, error)
	// Integration must wire this before exposing descendant v2 authority.
	OnFamilyRevokedTx func(context.Context, *sql.Tx, FamilyState) error
	// Root installs this before exposing authentication. Tests without networking
	// may omit it, but a production session must not synthesize a key from an id.
	NativeIdentityTx func(context.Context, *sql.Tx) (NativeServerIdentity, error)
	// RedeemInvitation is installed by whoever owns invitations. Invite-only
	// self-registration refuses every attempt while it is absent.
	RedeemInvitation InvitationRedeemer
}

func Token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func Digest(v string) string {
	h := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
func New(db *sql.DB, state string) (*Service, error) {
	id := persistence.Get(db, "id")
	if id == "" {
		id = Token()
		if e := persistence.Set(db, "id", id); e != nil {
			return nil, e
		}
	}
	s := &Service{db: db, serverID: id, setupPath: filepath.Join(state, "setup-token"), hashSlots: make(chan struct{}, 2)}
	if err := s.initSetupHashKey(state); err != nil {
		return nil, err
	}
	if s.SetupRequired() {
		// The setup secret only lets the browser that started setup resume it; it
		// never expires and survives restarts (no setup code is shown to anyone).
		_, e := os.Stat(s.setupPath)
		if e != nil && !os.IsNotExist(e) {
			return nil, e
		}
		if os.IsNotExist(e) {
			if e = os.WriteFile(s.setupPath, []byte(Token()), 0600); e != nil {
				return nil, e
			}
		}
		log.Printf("Portico is ready to set up.")
	}
	return s, nil
}
func (s *Service) ID() string { return s.serverID }
func (s *Service) Name() string {
	n := persistence.Get(s.db, "name")
	if n == "" {
		return "Portico"
	}
	return n
}
func (s *Service) SetupRequired() bool {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&n)
	return n == 0
}
func (s *Service) Setup(token, user, password, name string) (Envelope, error) {
	return s.SetupWithOptions(context.Background(), SetupOptions{SetupToken: token, Username: user, Password: password, Name: name}, true)
}
func (s *Service) Login(user, password string) (Envelope, error) {
	return s.loginFrom(context.Background(), user, password, true)
}
func (s *Service) loginFrom(ctx context.Context, user, password string, private bool) (Envelope, error) {
	out, err := s.DirectLoginFrom(ctx, user, password, private)
	if err != nil {
		return Envelope{}, err
	}
	if out.Challenge != nil {
		return Envelope{}, ErrFactorRequired
	}
	if out.Session == nil || out.Session.Viewer.Role == "account" {
		if err := s.retireUnreturnedLogin(ctx, out); err != nil {
			return Envelope{}, err
		}
		if out.PasswordChangeRequired {
			return Envelope{}, ErrPasswordChangeRequired
		}
		return Envelope{}, ErrProfileSelection
	}
	return *out.Session, nil
}

// Legacy one-envelope sign-in cannot return an account-scoped profile chooser.
// Retire the family before reporting that limitation, and remove the anonymous
// device if this path generated one solely for the unusable credential.
func (s *Service) retireUnreturnedLogin(ctx context.Context, out DirectSignIn) error {
	if out.Session == nil {
		return nil
	}
	if err := s.LogoutToken(ctx, out.Session.AccessToken); err != nil {
		return err
	}
	claim, _ := ctx.Value(issuingDeviceKey{}).(issuingDevice)
	if claim.deviceID == "" && claim.registration.InstallationID == "" && out.DeviceID != "" {
		_, err := dbwork.ExecWrite(ctx, s.db, dbwork.ClassSecurityFence, `DELETE FROM identity_devices WHERE id=? AND NOT EXISTS(SELECT 1 FROM identity_device_families b JOIN authorization_session_families f ON f.id=b.family_id WHERE b.device_id=? AND f.revoked=0)`, out.DeviceID, out.DeviceID)
		return err
	}
	return nil
}
func (s *Service) Issue(aid, pid, authority, role string, epoch int) (Envelope, error) {
	return s.IssueWithDevice(context.Background(), aid, pid, authority, role, epoch)
}

// IssueWithDevice issues exactly like Issue, but binds the family to the
// installation carried by ctx (WithIssuingDevice) instead of an anonymous
// device. Non-HTTP issuers that know their device (notably Cast pairing) use
// this so the bearer is an ordinary renewable device-bound session.
func (s *Service) IssueWithDevice(ctx context.Context, aid, pid, authority, role string, epoch int) (Envelope, error) {
	// Hosted issuance needs the caller's verified cached policy horizon in IssueTx.
	if authority != "local" {
		return Envelope{}, ErrUnauthorized
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassSecurityFence)
	if err != nil {
		return Envelope{}, err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	envelope, err := s.IssueTx(ctx, tx, aid, pid, authority, role, epoch, time.Time{})
	if err != nil {
		return Envelope{}, err
	}
	if err = gated.Commit(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}
func (s *Service) Authenticate(token string) (Principal, error) {
	return s.AuthenticateContext(context.Background(), token)
}

// AuthenticateContext preserves the authentication rules while allowing a bounded
// caller to cancel database connection waits and reads.
func (s *Service) AuthenticateContext(ctx context.Context, token string) (Principal, error) {
	if len(token) < 43 || len(token) > 2048 {
		return Principal{}, ErrUnauthorized
	}
	gated2, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return Principal{}, err
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	record, err := s.familyTokenTx(ctx, tx, Digest(token))
	if err != nil {
		return Principal{}, err
	}
	family, err := s.sessionFamilyRecordTx(ctx, tx, record.principal, record)
	if err != nil {
		return Principal{}, err
	}
	if record.principal.Role == "account" {
		return Principal{}, ErrUnauthorized
	}
	record.principal.DeviceID = family.DeviceID
	return record.principal, nil
}
func (s *Service) Logout(p Principal) error {
	return s.logoutFamilyHash(context.Background(), p.Hash)
}
