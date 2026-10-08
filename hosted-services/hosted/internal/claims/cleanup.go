package claims

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"portico.local/hosted/internal/directory"
	"time"
)

// CleanupWire authorizes only original-operation erasure, never server access.
// Reads are replayable for at most 30 seconds; acknowledgements are idempotent
// and scoped to the immutable job, revision and original claim tuple.
type CleanupWire struct {
	Kind                 string `json:"kind"`
	Version              string `json:"version"`
	Audience             string `json:"audience"`
	OperationID          string `json:"operationId"`
	ServerID             string `json:"serverId"`
	PublicKey            string `json:"publicKey"`
	ClaimGeneration      string `json:"claimGeneration"`
	CredentialGeneration string `json:"credentialGeneration"`
	RequestID            string `json:"requestId"`
	IssuedAt             string `json:"issuedAt"`
	ExpiresAt            string `json:"expiresAt"`
	AcknowledgeJobID     string `json:"acknowledgeJobId"`
	AcknowledgeRevision  int64  `json:"acknowledgeRevision"`
}
type CleanupResult struct {
	Kind                 string                 `json:"kind"`
	RequestID            string                 `json:"requestId"`
	OperationID          string                 `json:"operationId"`
	ServerID             string                 `json:"serverId"`
	ClaimGeneration      string                 `json:"claimGeneration"`
	CredentialGeneration string                 `json:"credentialGeneration"`
	Retired              bool                   `json:"retired"`
	Jobs                 []directory.ErasureJob `json:"jobs"`
	IssuedAt             time.Time              `json:"issuedAt"`
	ExpiresAt            time.Time              `json:"expiresAt"`
}
type CleanupRepository interface {
	ClaimCleanup(context.Context, CleanupWire) (bool, []directory.ErasureJob, error)
}

func (s *Service) Cleanup(ctx context.Context, payload, signature []byte) (Envelope, error) {
	var w CleanupWire
	if len(payload) > 8192 || json.Unmarshal(payload, &w) != nil {
		return Envelope{}, ErrInvalid
	}
	canonical, e := json.Marshal(w)
	if e != nil || !bytes.Equal(canonical, payload) || w.Kind != "portico.claim.cleanup" || w.Version != "1" || w.Audience != s.audience || !validID(w.OperationID) || !validID(w.ServerID) || !validID(w.RequestID) || !validGeneration(w.ClaimGeneration) || !validGeneration(w.CredentialGeneration) {
		return Envelope{}, ErrInvalid
	}
	if (w.AcknowledgeJobID == "" && w.AcknowledgeRevision != 0) || (w.AcknowledgeJobID != "" && (!validID(w.AcknowledgeJobID) || w.AcknowledgeRevision != 1)) {
		return Envelope{}, ErrInvalid
	}
	issued, e := time.Parse(time.RFC3339Nano, w.IssuedAt)
	if e != nil {
		return Envelope{}, ErrInvalid
	}
	expires, e := time.Parse(time.RFC3339Nano, w.ExpiresAt)
	now := s.now().UTC()
	if e != nil || !expires.After(now) || issued.After(now.Add(5*time.Second)) || !expires.After(issued) || expires.Sub(issued) > 30*time.Second {
		return Envelope{}, ErrInvalid
	}
	key, e := base64.RawURLEncoding.Strict().DecodeString(w.PublicKey)
	if e != nil || base64.RawURLEncoding.EncodeToString(key) != w.PublicKey || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, payload, signature) {
		return Envelope{}, ErrInvalid
	}
	server, e := ServerID(key)
	if e != nil || server != w.ServerID {
		return Envelope{}, ErrInvalid
	}
	repo, ok := s.repo.(CleanupRepository)
	if !ok {
		return Envelope{}, ErrInvalid
	}
	retired, jobs, e := repo.ClaimCleanup(ctx, w)
	if e != nil {
		return Envelope{}, e
	}
	result := CleanupResult{Kind: "portico.claim.cleanup.result", RequestID: w.RequestID, OperationID: w.OperationID, ServerID: w.ServerID, ClaimGeneration: w.ClaimGeneration, CredentialGeneration: w.CredentialGeneration, Retired: retired, Jobs: jobs, IssuedAt: now, ExpiresAt: expires}
	for i := range result.Jobs {
		result.Jobs[i].Kind = "portico.account-erasure.v1"
		result.Jobs[i].IssuedAt = now
		result.Jobs[i].ExpiresAt = expires
	}
	raw, e := json.Marshal(result)
	if e != nil {
		return Envelope{}, e
	}
	return Envelope{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(s.keys.PrivateKey, raw)), KeyID: s.keys.KeyID, Certificate: s.keys.Certificate}, nil
}
