package claims

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/serverreturn"
	"portico.local/hosted/trust"
	"time"
)

var ErrApprovalRequired = errors.New("claim approval required")
var ErrCancelled = errors.New("claim cancelled")
var ErrStale = errors.New("claim stale")
var ErrCapacity = errors.New("claim capacity exceeded")

type ApprovalInput struct {
	ReplacesServerID      string `json:"replacesServerId,omitempty"`
	OperationID           string `json:"operationId"`
	ServerID              string `json:"serverId"`
	PublicKey             string `json:"publicKey"`
	LocalGeneration       int64  `json:"localGeneration,string"`
	Name                  string `json:"name"`
	ExpectedRevision      int64  `json:"expectedRevision,string"`
	Purpose               string `json:"purpose"`
	CurrentOwnerAccountID string `json:"currentOwnerAccountId"`
	PreviousOwnerRole     string `json:"previousOwnerRole"`
	ReturnRegistrationID  string `json:"returnRegistrationId"`
}

func (a ApprovalInput) Valid() bool {
	key, ok := decode(a.PublicKey, 32)
	derived, _ := ServerID(key)
	return serverreturn.Binding(a.Purpose, a.CurrentOwnerAccountID, a.PreviousOwnerRole) && (a.ReturnRegistrationID == "" || validID(a.ReturnRegistrationID)) && (a.ReplacesServerID == "" || validID(a.ReplacesServerID) && a.ReplacesServerID != a.ServerID) && a.ServerID == derived && validID(a.OperationID) && validID(a.ServerID) && ok && a.LocalGeneration >= 0 && a.ExpectedRevision >= 0 && len(a.Name) > 0 && len(a.Name) <= 120
}

// IntentDigest includes the displayed server name and exact operation intent.
// They cannot be silently substituted during idempotent approval/reapproval.
func (a ApprovalInput) IntentDigest(account, audience string) [32]byte {
	v := struct {
		OperationID, ServerID, AccountID, PublicKey, Audience                   string
		LocalGeneration                                                         int64
		Name, ReplacesServerID                                                  string
		Purpose, CurrentOwnerAccountID, PreviousOwnerRole, ReturnRegistrationID string
	}{a.OperationID, a.ServerID, account, a.PublicKey, audience, a.LocalGeneration, a.Name, a.ReplacesServerID, a.Purpose, a.CurrentOwnerAccountID, a.PreviousOwnerRole, a.ReturnRegistrationID}
	raw, _ := json.Marshal(v)
	return sha256.Sum256(raw)
}

type Approval struct {
	TargetUsername        string `json:"targetUsername,omitempty"`
	Kind                  string `json:"kind"`
	Version               string `json:"version"`
	Audience              string `json:"audience"`
	OperationID           string `json:"operationId"`
	ServerID              string `json:"serverId"`
	AccountID             string `json:"accountId"`
	PublicKey             string `json:"publicKey"`
	LocalGeneration       int64  `json:"localGeneration,string"`
	ApprovalRevision      int64  `json:"approvalRevision,string"`
	Purpose               string `json:"purpose"`
	CurrentOwnerAccountID string `json:"currentOwnerAccountId"`
	PreviousOwnerRole     string `json:"previousOwnerRole"`
	ReturnRegistrationID  string `json:"returnRegistrationId"`
	TransitionMode        string `json:"transitionMode,omitempty"`
	IssuedAt              string `json:"issuedAt"`
	ExpiresAt             string `json:"expiresAt"`
}
type Envelope = trust.Envelope

func SignApproval(a Approval, key ed25519.PrivateKey, keyID string, certificate *trust.Certificate) (Envelope, error) {
	if len(key) != ed25519.PrivateKeySize || keyID == "" || a.Kind != "portico.claim.approval" || (a.Version != "2" && a.Version != "3") || (a.Version == "2" && a.TransitionMode != "") || (a.Version == "3" && a.TransitionMode != "switch" && a.TransitionMode != "start-over") || (a.TransitionMode != "" && a.Purpose != "switch") || !serverreturn.Binding(a.Purpose, a.CurrentOwnerAccountID, a.PreviousOwnerRole) || !validAudience(a.Audience) || !validBinding(a.OperationID, a.ServerID, a.AccountID, a.LocalGeneration) || a.ApprovalRevision < 1 || ((a.Purpose == "change-owner" || a.Purpose == "switch") && !identity.ValidUsername(a.TargetUsername)) || (a.Purpose != "change-owner" && a.Purpose != "switch" && a.TargetUsername != "") {
		return Envelope{}, ErrInvalid
	}
	public, ok := decode(a.PublicKey, 32)
	derived, _ := ServerID(public)
	if !ok || derived != a.ServerID {
		return Envelope{}, ErrInvalid
	}
	issued, e := timestamp(a.IssuedAt)
	if e != nil {
		return Envelope{}, e
	}
	expiry, e := timestamp(a.ExpiresAt)
	if e != nil || !expiry.After(issued) || expiry.Sub(issued) > 5*time.Minute {
		return Envelope{}, ErrInvalid
	}
	raw, e := json.Marshal(a)
	if e != nil {
		return Envelope{}, e
	}
	return Envelope{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, raw)), KeyID: keyID, Certificate: certificate}, nil
}

type Challenge struct {
	RequestDigest string    `json:"requestDigest"`
	NonceID       string    `json:"nonceId"`
	Nonce         string    `json:"nonce"`
	IssuedAt      time.Time `json:"issuedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
}
type Repository interface {
	ApproveClaim(context.Context, identity.CurrentFamilyExpectation, ApprovalInput, string) (Approval, error)
	ClaimNonce(context.Context, Verified, string, []byte) (Challenge, error)
}
