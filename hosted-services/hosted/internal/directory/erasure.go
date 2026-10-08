package directory

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"time"
)

type ErasureJob struct {
	Kind        string    `json:"kind"`
	ID          string    `json:"id"`
	DeletionID  string    `json:"deletionId"`
	ServerID    string    `json:"serverId"`
	AccountID   string    `json:"accountId"`
	ProfileIDs  []string  `json:"profileIds"`
	Revision    int64     `json:"revision"`
	Disposition string    `json:"disposition"`
	IssuedAt    time.Time `json:"issuedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}
type ErasureRepository interface {
	Erasures(context.Context, string) ([]ErasureJob, error)
	AcknowledgeErasure(context.Context, string, string, int64) error
}

func (s *Service) Erasures(ctx context.Context, server string) ([]SignedPolicy, error) {
	jobs, e := s.repo.(ErasureRepository).Erasures(ctx, server)
	if e != nil {
		return nil, e
	}
	out := []SignedPolicy{}
	for _, j := range jobs {
		j.Kind = "portico.account-erasure.v1"
		j.IssuedAt = time.Now().UTC()
		j.ExpiresAt = j.IssuedAt.Add(24 * time.Hour)
		raw, e := json.Marshal(j)
		if e != nil {
			return nil, e
		}
		out = append(out, SignedPolicy{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(s.keys.PrivateKey, raw)), KeyID: s.keys.KeyID, Certificate: s.keys.Certificate})
	}
	return out, nil
}
func (s *Service) AcknowledgeErasure(ctx context.Context, server, id string, revision int64) error {
	if len(id) < 1 || len(id) > 128 || revision != 1 {
		return ErrInvalid
	}
	return s.repo.(ErasureRepository).AcknowledgeErasure(ctx, server, id, revision)
}
