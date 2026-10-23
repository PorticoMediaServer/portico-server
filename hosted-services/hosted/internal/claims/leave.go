package claims

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
)

// LeaveWire is a durable, deny-only notice. It has no expiry because the server
// may switch to local while Services are unavailable. Only its original key and
// exact installed generations can withdraw registration; it grants no access.
// Field order is shared with the server's durable notice producer.
type LeaveWire struct {
	Kind                 string `json:"kind"`
	Version              string `json:"version"`
	Audience             string `json:"audience"`
	OperationID          string `json:"operationId"`
	ServerID             string `json:"serverId"`
	PublicKey            string `json:"publicKey"`
	ClaimGeneration      string `json:"claimGeneration"`
	CredentialGeneration string `json:"credentialGeneration"`
	NoticeID             string `json:"noticeId"`
	Reason               string `json:"reason"`
}

type LeaveRepository interface {
	LeaveRegistration(context.Context, LeaveWire, [32]byte) error
}

func VerifyLeave(payload, signature []byte, audience string) (LeaveWire, error) {
	var w LeaveWire
	if !validAudience(audience) || !canonical(payload, &w) || w.Kind != "portico.server.leave" || w.Version != "1" || w.Audience != audience || !validID(w.OperationID) || !validID(w.ServerID) || !validID(w.NoticeID) || !validGeneration(w.ClaimGeneration) || !validGeneration(w.CredentialGeneration) || (w.Reason != "moving_to_local" && w.Reason != "start_over_local") {
		return LeaveWire{}, ErrInvalid
	}
	key, ok := decode(w.PublicKey, ed25519.PublicKeySize)
	id, _ := ServerID(key)
	if !ok || id != w.ServerID || len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, payload, signature) {
		return LeaveWire{}, ErrInvalid
	}
	return w, nil
}

func (s *Service) Leave(ctx context.Context, payload, signature []byte) error {
	w, e := VerifyLeave(payload, signature, s.audience)
	if e != nil {
		return e
	}
	repo, ok := s.repo.(LeaveRepository)
	if !ok {
		return ErrInvalid
	}
	return repo.LeaveRegistration(ctx, w, sha256.Sum256(payload))
}
