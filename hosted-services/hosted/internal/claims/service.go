package claims

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"time"
)

type Commit struct {
	OperationID          string `json:"operationId"`
	ServerID             string `json:"serverId"`
	ClaimGeneration      string `json:"claimGeneration"`
	CredentialGeneration string `json:"credentialGeneration"`
}
type ResultValue struct {
	Commit
	ServerCredential string `json:"serverCredential"`
}

func (ResultValue) String() string     { return "[claim result redacted]" }
func (r ResultValue) GoString() string { return r.String() }

type Material struct{ Hash, Ciphertext []byte }

func (Material) String() string     { return "[claim credential material redacted]" }
func (m Material) GoString() string { return m.String() }

type CancellationResult struct {
	OperationID string `json:"operationId"`
	RequestID   string `json:"requestId"`
}
type ConsumerRepository interface {
	Repository
	FinalizeClaim(context.Context, Verified, Material) (Commit, error)
	ResultClaim(context.Context, Verified) (Commit, []byte, error)
	CancelClaim(context.Context, CancelWire, [32]byte) (CancellationResult, error)
	AcknowledgeClaim(context.Context, []byte, Commit) (Commit, error)
}
type Service struct {
	repo     ConsumerRepository
	keys     *security.Keys
	audience string
	now      func() time.Time
}

func New(repo ConsumerRepository, keys *security.Keys, audience string) (*Service, error) {
	if repo == nil || keys == nil || len(keys.PrivateKey) != ed25519.PrivateKeySize || len(keys.SealKey) != 32 || keys.KeyID == "" || !validAudience(audience) {
		return nil, ErrInvalid
	}
	// Snapshot configured key material; subsequent caller mutation cannot change
	// a request's signer/seal identity during a multi-step claim operation.
	copyKeys := *keys
	copyKeys.PrivateKey = append(ed25519.PrivateKey(nil), keys.PrivateKey...)
	copyKeys.SealKey = append([]byte(nil), keys.SealKey...)
	return &Service{repo: repo, keys: &copyKeys, audience: audience, now: time.Now}, nil
}
func (s *Service) Approve(ctx context.Context, authority identity.CurrentFamilyExpectation, in ApprovalInput) (Envelope, error) {
	a, e := s.repo.ApproveClaim(ctx, authority, in, s.audience)
	if e != nil {
		return Envelope{}, e
	}
	return SignApproval(a, s.keys.PrivateKey, s.keys.KeyID, s.keys.Certificate)
}
func (s *Service) Nonce(ctx context.Context, payload, signature []byte) (Challenge, error) {
	v, e := Verify(payload, signature, s.audience, NonceRequest, Finalize, s.now())
	if e != nil {
		v, e = Verify(payload, signature, s.audience, NonceRequest, Result, s.now())
	}
	if e != nil {
		return Challenge{}, e
	}
	nonce := make([]byte, 32)
	if _, e = rand.Read(nonce); e != nil {
		return Challenge{}, e
	}
	id := make([]byte, 32)
	if _, e = rand.Read(id); e != nil {
		return Challenge{}, e
	}
	return s.repo.ClaimNonce(ctx, v, base64.RawURLEncoding.EncodeToString(id), nonce)
}
func (s *Service) Finalize(ctx context.Context, payload, signature []byte) (Commit, error) {
	v, e := Verify(payload, signature, s.audience, ClaimProof, Finalize, s.now())
	if e != nil {
		return Commit{}, e
	}
	random := make([]byte, 32)
	if _, e = rand.Read(random); e != nil {
		return Commit{}, e
	}
	token := "srv_" + base64.RawURLEncoding.EncodeToString(random)
	sealed, e := s.keys.Seal(token, v.Wire.ServerID)
	if e != nil {
		return Commit{}, e
	}
	return s.repo.FinalizeClaim(ctx, v, Material{security.Hash(token), sealed})
}
func (s *Service) Result(ctx context.Context, payload, signature []byte) (ResultValue, error) {
	v, e := Verify(payload, signature, s.audience, ClaimProof, Result, s.now())
	if e != nil {
		return ResultValue{}, e
	}
	commit, sealed, e := s.repo.ResultClaim(ctx, v)
	if e != nil {
		return ResultValue{}, e
	}
	credential, e := s.keys.Open(sealed, v.Wire.ServerID)
	if e != nil {
		return ResultValue{}, e
	}
	return ResultValue{commit, credential}, nil
}
func (s *Service) Acknowledge(ctx context.Context, credential string, commit Commit) (Commit, error) {
	if len(credential) < 20 || len(credential) > 128 || !validID(commit.OperationID) || !validID(commit.ServerID) || !validGeneration(commit.ClaimGeneration) || !validGeneration(commit.CredentialGeneration) {
		return Commit{}, ErrInvalid
	}
	return s.repo.AcknowledgeClaim(ctx, security.Hash(credential), commit)
}

func (s *Service) Cancel(ctx context.Context, payload, signature []byte) (CancellationResult, error) {
	v, e := VerifyCancellation(payload, signature, s.audience)
	if e != nil {
		return CancellationResult{}, e
	}
	return s.repo.CancelClaim(ctx, v, sha256.Sum256(payload))
}

func (s *Service) SignWebApproval(a Approval) (Envelope, error) {
	return SignApproval(a, s.keys.PrivateKey, s.keys.KeyID, s.keys.Certificate)
}
