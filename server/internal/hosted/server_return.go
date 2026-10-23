package hosted

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"time"
)

// RegisteredReturnExchange carries no Hosted account credential. Verifier and
// code are transient and must never be included in an error/log representation.
type RegisteredReturnExchange struct {
	Code            string `json:"code"`
	TransactionID   string `json:"transactionId"`
	Verifier        string `json:"verifier"`
	RegistrationID  string `json:"registrationId"`
	ServerChallenge string `json:"serverChallenge"`
	InstallationID  string `json:"installationId"`
	Purpose         string `json:"purpose"`
}

func (RegisteredReturnExchange) String() string     { return "[registered return redacted]" }
func (q RegisteredReturnExchange) GoString() string { return q.String() }

type serverReturnAssertion struct {
	Kind                  string    `json:"kind"`
	AccountID             string    `json:"accountId"`
	Username              string    `json:"username"`
	DisplayName           string    `json:"displayName"`
	Email                 string    `json:"email"`
	ServerID              string    `json:"serverId"`
	Challenge             string    `json:"challenge"`
	Nonce                 string    `json:"nonce"`
	IssuedAt              time.Time `json:"issuedAt"`
	ExpiresAt             time.Time `json:"expiresAt"`
	Purpose               string    `json:"purpose"`
	InstallationID        string    `json:"installationId"`
	RegistrationID        string    `json:"registrationId"`
	ClaimOperationID      string    `json:"claimOperationId"`
	ClaimGeneration       int64     `json:"claimGeneration,string"`
	CredentialGeneration  int64     `json:"credentialGeneration,string"`
	CurrentOwnerAccountID string    `json:"currentOwnerAccountId"`
	PreviousOwnerRole     string    `json:"previousOwnerRole"`
}

func (s *Service) ExchangeRegisteredReturn(ctx context.Context, q RegisteredReturnExchange) (identity.PorticoIdentity, error) {
	if s == nil || !s.Configured() || s.identity == nil || len(q.Code) < 32 || len(q.Code) > 128 || len(q.Verifier) < 32 || len(q.Verifier) > 128 || len(q.TransactionID) > 128 || len(q.RegistrationID) > 128 || len(q.InstallationID) < 32 || len(q.InstallationID) > 128 {
		return identity.PorticoIdentity{}, identity.ErrUnauthorized
	}
	if kind, e := s.identity.SignInKind(ctx); e != nil {
		return identity.PorticoIdentity{}, e
	} else if kind == identity.SignInLocal {
		var confirmed bool
		if e = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM identity_moves WHERE to_kind='portico' AND stage='preview' AND owner_confirmed=1)`).Scan(&confirmed); e != nil {
			return identity.PorticoIdentity{}, e
		}
		if !confirmed || q.Purpose != "switch" {
			return identity.PorticoIdentity{}, identity.ErrSignInKindLocal
		}
	}
	raw, _ := json.Marshal(q)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "POST", s.origin+"/v1/server-returns/exchange", bytes.NewReader(raw))
	if e != nil {
		return identity.PorticoIdentity{}, identity.ErrUnauthorized
	}
	req.Header.Set("Content-Type", "application/json")
	client := *s.client
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, e := client.Do(req)
	if e != nil {
		return identity.PorticoIdentity{}, ErrServicesUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusRequestTimeout {
		return identity.PorticoIdentity{}, ErrServicesUnavailable
	}
	if response.StatusCode != 200 {
		return identity.PorticoIdentity{}, identity.ErrUnauthorized
	}
	var grant struct {
		Assertion Signed `json:"assertion"`
	}
	d := json.NewDecoder(io.LimitReader(response.Body, 16385))
	d.DisallowUnknownFields()
	if d.Decode(&grant) != nil {
		return identity.PorticoIdentity{}, identity.ErrUnauthorized
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return identity.PorticoIdentity{}, identity.ErrUnauthorized
	}
	return s.verifyRegisteredReturn(ctx, grant.Assertion, q)
}
func (s *Service) verifyRegisteredReturn(ctx context.Context, signed Signed, q RegisteredReturnExchange) (identity.PorticoIdentity, error) {
	var none identity.PorticoIdentity
	raw, e := s.verifyEnvelope(ctx, signed)
	if e != nil || len(raw) > 8192 {
		return none, identity.ErrUnauthorized
	}
	var c serverReturnAssertion
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return none, identity.ErrUnauthorized
	}
	encoded, _ := json.Marshal(c)
	if !bytes.Equal(encoded, raw) {
		return none, identity.ErrUnauthorized
	}
	now := time.Now()
	if c.Kind != "portico.server-return.v1" || c.ServerID != s.identity.ID() || c.AccountID == "" || c.Username == "" || c.Challenge != q.ServerChallenge || c.InstallationID != q.InstallationID || c.RegistrationID != q.RegistrationID || c.Purpose != q.Purpose || len(c.Nonce) < 22 || !c.ExpiresAt.After(now) || c.IssuedAt.After(now.Add(time.Minute)) || !c.ExpiresAt.After(c.IssuedAt) || c.ExpiresAt.Sub(c.IssuedAt) > time.Minute {
		return none, identity.ErrUnauthorized
	}
	consent, _ := json.Marshal(signed)
	who := identity.PorticoIdentity{AccountID: c.AccountID, Username: c.Username, DisplayName: c.DisplayName, Email: c.Email, Consent: string(consent), SignerKeyID: signed.KeyID, Return: &identity.PorticoReturnBinding{ExpiresAt: c.ExpiresAt, Purpose: c.Purpose, InstallationID: c.InstallationID, RegistrationID: c.RegistrationID, OperationID: c.ClaimOperationID, ClaimGeneration: c.ClaimGeneration, CredentialGeneration: c.CredentialGeneration, CurrentOwnerAccountID: c.CurrentOwnerAccountID, PreviousOwnerRole: c.PreviousOwnerRole}}
	e = dbwork.WithWriteTx(ctx, s.db, dbwork.ClassSecurityFence, func(tx *sql.Tx) error {
		if e := identity.CheckPorticoReturnTx(ctx, tx, who); e != nil {
			return e
		}
		result, e := tx.ExecContext(ctx, `DELETE FROM hosted_identity_challenges WHERE challenge_hash=? AND installation_id=? AND expires_ms>?`, identity.Digest(c.Challenge), q.InstallationID, now.UnixMilli())
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return identity.ErrUnauthorized
		}
		result, e = tx.ExecContext(ctx, `INSERT INTO hosted_identity_nonces(nonce,expires_ms) VALUES(?,?) ON CONFLICT(nonce) DO NOTHING`, c.Nonce, c.ExpiresAt.UnixMilli())
		if e != nil {
			return e
		}
		n, _ = result.RowsAffected()
		if n != 1 {
			return identity.ErrUnauthorized
		}
		return identity.CheckPorticoReturnTx(ctx, tx, who)
	})
	if e != nil {
		return none, e
	}
	return who, nil
}
