// Package device implements explicit Portico Account approval for input-limited devices.
package device

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"net/url"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const Alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

var ErrInvalid = errors.New("invalid device authorization input")
var ErrConflict = errors.New("device authorization conflict")

// ErrCodeCollision is internal: retry a fresh human code without changing the create intent.
var ErrCodeCollision = errors.New("device authorization code collision")

type ProtocolError struct {
	Code     string
	Interval int
}

func (e *ProtocolError) Error() string { return e.Code }

type Start struct {
	SessionMode string `json:"sessionMode,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	PublicKey   string `json:"publicKey,omitempty"`
	RequestID   string `json:"requestId"`
	DeviceName  string `json:"deviceName"`
	Platform    string `json:"platform"`
	AppVersion  string `json:"appVersion"`
}
type Authorization struct {
	DeviceCode              string    `json:"deviceCode"`
	UserCode                string    `json:"userCode"`
	VerificationURI         string    `json:"verificationUri"`
	VerificationURIComplete string    `json:"verificationUriComplete"`
	ExpiresAt               time.Time `json:"expiresAt"`
	ExpiresIn               int       `json:"expiresIn"`
	Interval                int       `json:"interval"`
}
type Preview struct {
	Protocol        string    `json:"protocol,omitempty"`
	PublicKey       string    `json:"publicKey,omitempty"`
	ClientRequestID string    `json:"clientRequestId,omitempty"`
	RequestID       string    `json:"requestId"`
	UserCode        string    `json:"userCode"`
	DeviceName      string    `json:"deviceName"`
	Platform        string    `json:"platform"`
	AppVersion      string    `json:"appVersion"`
	ExpiresAt       time.Time `json:"expiresAt"`
}
type Creation struct {
	ID                                                      string
	CreationHash, RequestHash, DeviceHash, UserHash, Cipher []byte
	DeviceName, Platform, AppVersion                        string
	SessionMode                                             string
	Protocol, PublicKey, RequestID                          string
	ExpiresAt                                               time.Time
}
type Created struct {
	ID        string
	Cipher    []byte
	ExpiresAt time.Time
	Interval  int
}
type Issue struct {
	Protocol     string
	SealGrant    func(requestID, publicKey string, viewer identity.Viewer) ([]byte, error)
	Family       identity.FamilySeed
	Hash, Cipher []byte
	ExpiresAt    time.Time
}
type Grant struct {
	Protected json.RawMessage
	Cipher    []byte
	ExpiresAt time.Time
	Viewer    identity.Viewer
}
type Repository interface {
	DeviceLimit(context.Context, []byte, string, int, time.Duration) error
	StartDevice(context.Context, Creation) (Created, error)
	ReviewDevice(context.Context, []byte) (Preview, error)
	CaptureClaimAuthority(context.Context, []byte) (identity.CurrentFamilyExpectation, error)
	DecideDevice(context.Context, identity.CurrentFamilyExpectation, string, []byte, string) error
	PollDevice(context.Context, []byte, Issue) (Grant, error)
	CancelDevice(context.Context, []byte) error
}
type Service struct {
	repo            Repository
	keys            *security.Keys
	verificationURI string
	accessLifetime  time.Duration
	tvIssuer        string
}

func New(repo Repository, keys *security.Keys, verificationURI string, allowLocal bool) (*Service, error) {
	u, e := url.Parse(verificationURI)
	if e != nil || u.User != nil || u.Host == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || u.Path != "/device" {
		return nil, ErrInvalid
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if !allowLocal || u.Scheme != "http" || ip == nil || !ip.IsLoopback() {
			return nil, ErrInvalid
		}
	}
	return &Service{repo: repo, keys: keys, verificationURI: u.String(), accessLifetime: 12 * time.Hour}, nil
}
func (s *Service) hash(kind, value string) []byte {
	h := hmac.New(sha256.New, s.keys.SealKey)
	h.Write([]byte("portico.device.v1/" + kind + "\x00" + value))
	return h.Sum(nil)
}

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
var label = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 ._+-]*$`)

func normalizeCode(code string) (string, error) {
	code = strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' {
			return -1
		}
		return r
	}, code))
	if len(code) != 8 {
		return "", ErrInvalid
	}
	for _, r := range code {
		if !strings.ContainsRune(Alphabet, r) {
			return "", ErrInvalid
		}
	}
	return code, nil
}
func humanCode() (string, error) {
	b := make([]byte, 8)
	for i := range b {
		v, e := rand.Int(rand.Reader, big.NewInt(int64(len(Alphabet))))
		if e != nil {
			return "", e
		}
		b[i] = Alphabet[v.Int64()]
	}
	return string(b), nil
}
func (s *Service) Start(ctx context.Context, ip string, q Start) (Authorization, error) {
	if e := s.repo.DeviceLimit(ctx, s.hash("ip", ip), "create", 10, 10*time.Minute); e != nil {
		return Authorization{}, e
	}
	if q.SessionMode == "" {
		q.SessionMode = "native"
	}
	if !identity.ValidMode(q.SessionMode) {
		return Authorization{}, ErrInvalid
	}
	if q.Protocol != "" && q.Protocol != TVProtocol {
		return Authorization{}, ErrInvalid
	}
	if q.Protocol == TVProtocol {
		if q.SessionMode != "native" || s.tvIssuer == "" || !validTVKey(q.PublicKey) {
			return Authorization{}, ErrInvalid
		}
	} else if q.PublicKey != "" {
		return Authorization{}, ErrInvalid
	}
	q.RequestID = strings.ToLower(q.RequestID)
	q.DeviceName = strings.TrimSpace(q.DeviceName)
	if !uuid.MatchString(q.RequestID) || len(q.DeviceName) < 1 || len(q.DeviceName) > 100 || len(q.Platform) < 1 || len(q.Platform) > 32 || !label.MatchString(q.Platform) || len(q.AppVersion) < 1 || len(q.AppVersion) > 40 || !label.MatchString(q.AppVersion) {
		return Authorization{}, ErrInvalid
	}
	for _, r := range q.DeviceName {
		if unicode.IsControl(r) {
			return Authorization{}, ErrInvalid
		}
	}
	raw, _ := json.Marshal(q)
	id := security.RandomID("dreq_")
	secret := security.RandomID("device_")
	var c Created
	var e error
	// Code reservation is atomic in the repository. Collisions never consume a new
	// logical create intent or expose a partial QR/code pair to the client.
	for attempt := 0; attempt < 8; attempt++ {
		code, err := humanCode()
		if err != nil {
			return Authorization{}, err
		}
		plain, _ := json.Marshal(struct {
			DeviceCode string `json:"deviceCode"`
			UserCode   string `json:"userCode"`
		}{secret, code})
		cipher, err := s.keys.Seal(string(plain), "device-create:"+q.RequestID)
		if err != nil {
			return Authorization{}, err
		}
		c, e = s.repo.StartDevice(ctx, Creation{ID: id, SessionMode: q.SessionMode, Protocol: q.Protocol, PublicKey: q.PublicKey, RequestID: q.RequestID, CreationHash: s.hash("create", q.RequestID), RequestHash: s.hash("request", string(raw)), DeviceHash: s.hash("secret", secret), UserHash: s.hash("user", code), Cipher: cipher, DeviceName: q.DeviceName, Platform: q.Platform, AppVersion: q.AppVersion, ExpiresAt: time.Now().UTC().Add(10 * time.Minute)})
		if !errors.Is(e, ErrCodeCollision) {
			break
		}
	}
	if errors.Is(e, ErrCodeCollision) {
		return Authorization{}, &ProtocolError{Code: "temporarily_unavailable", Interval: 5}
	}
	if e != nil {
		return Authorization{}, e
	}
	value, e := s.keys.Open(c.Cipher, "device-create:"+q.RequestID)
	if e != nil {
		return Authorization{}, e
	}
	var credentials struct {
		DeviceCode string `json:"deviceCode"`
		UserCode   string `json:"userCode"`
	}
	if e = json.Unmarshal([]byte(value), &credentials); e != nil {
		return Authorization{}, e
	}
	display := credentials.UserCode[:4] + "-" + credentials.UserCode[4:]
	return Authorization{DeviceCode: credentials.DeviceCode, UserCode: display, VerificationURI: s.verificationURI, VerificationURIComplete: s.verificationURI + "#code=" + display, ExpiresAt: c.ExpiresAt, ExpiresIn: max(0, int(time.Until(c.ExpiresAt).Seconds())), Interval: c.Interval}, nil
}
func (s *Service) Review(ctx context.Context, account, ip, code string) (Preview, error) {
	if e := s.repo.DeviceLimit(ctx, s.hash("ip", ip), "review", 20, 10*time.Minute); e != nil {
		return Preview{}, e
	}
	if e := s.repo.DeviceLimit(ctx, s.hash("account", account), "review", 10, 10*time.Minute); e != nil {
		return Preview{}, e
	}
	normalized, e := normalizeCode(code)
	if e != nil {
		return Preview{}, &ProtocolError{Code: "invalid_grant"}
	}
	p, e := s.repo.ReviewDevice(ctx, s.hash("user", normalized))
	p.UserCode = normalized[:4] + "-" + normalized[4:]
	return p, e
}
func (s *Service) Decide(ctx context.Context, access, ip, id, code, decision string) error {
	if len(access) < 20 || len(access) > 128 {
		return identity.ErrCredentials
	}
	if e := s.repo.DeviceLimit(ctx, s.hash("ip", ip), "decision", 20, 10*time.Minute); e != nil {
		return e
	}
	expected, e := s.repo.CaptureClaimAuthority(ctx, security.Hash(access))
	if e != nil {
		return e
	}
	if e := s.repo.DeviceLimit(ctx, s.hash("account", expected.AccountID), "decision", 10, 10*time.Minute); e != nil {
		return e
	}
	normalized, e := normalizeCode(code)
	if e != nil || len(id) > 128 || (decision != "approve" && decision != "deny") {
		return ErrInvalid
	}
	return s.repo.DecideDevice(ctx, expected, id, s.hash("user", normalized), decision)
}
func (s *Service) Poll(ctx context.Context, secret string) (identity.Session, error) {
	return s.PollMode(ctx, secret, "native")
}
func (s *Service) PollMode(ctx context.Context, secret, mode string) (identity.Session, error) {
	if !identity.ValidMode(mode) {
		return identity.Session{}, ErrInvalid
	}
	if !strings.HasPrefix(secret, "device_") || len(secret) != 50 {
		return identity.Session{}, &ProtocolError{Code: "invalid_grant"}
	}
	token := security.RandomID("hat_")
	family, refresh := identity.NewFamily(mode)
	pair := identity.Session{AccessToken: token, RefreshToken: refresh, FamilyID: family.ID, RefreshExpiresAt: &family.RefreshExpiresAt}
	raw, _ := json.Marshal(pair)
	cipher, e := s.keys.Seal(string(raw), "device-grant:"+secret)
	if e != nil {
		return identity.Session{}, e
	}
	grant, e := s.repo.PollDevice(ctx, s.hash("secret", secret), Issue{Family: family, Hash: security.Hash(token), Cipher: cipher, ExpiresAt: time.Now().UTC().Add(s.accessLifetime)})
	if e != nil {
		return identity.Session{}, e
	}
	value, e := s.keys.Open(grant.Cipher, "device-grant:"+secret)
	if e != nil {
		return identity.Session{}, e
	}
	if e = json.Unmarshal([]byte(value), &pair); e != nil {
		return identity.Session{}, e
	}
	pair.ExpiresAt = grant.ExpiresAt
	pair.Account = grant.Viewer.Account
	pair.Profiles = grant.Viewer.Profiles
	pair.Mode = mode
	return pair, nil
}
func (s *Service) Cancel(ctx context.Context, secret string) error {
	if !strings.HasPrefix(secret, "device_") || len(secret) != 50 {
		return &ProtocolError{Code: "invalid_grant"}
	}
	return s.repo.CancelDevice(ctx, s.hash("secret", secret))
}

func (s *Service) SetAccessLifetime(ttl time.Duration) { s.accessLifetime = ttl }
