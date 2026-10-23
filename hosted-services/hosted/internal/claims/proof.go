// Package claims implements the Hosted side of the current outbound claim wire.
// Verified signatures are not database authorization: every consumer must check
// persisted operation, approval, credential and nonce state in its transaction.
package claims

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid claim proof")

const Finalize = "claim-finalize"
const Result = "claim-result"
const NonceRequest = "portico.claim.nonce-request"
const ClaimProof = "portico.claim.proof"

// Wire order is shared with networking.claimWire, not a generic JSON signing API.
type Wire struct {
	Kind                   string `json:"kind"`
	Version                string `json:"version"`
	Audience               string `json:"audience"`
	Purpose                string `json:"purpose"`
	OperationID            string `json:"operationId"`
	ServerID               string `json:"serverId"`
	AccountID              string `json:"accountId"`
	PublicKey              string `json:"publicKey"`
	LocalGeneration        int64  `json:"localGeneration,string"`
	ApprovalRevision       int64  `json:"approvalRevision,string"`
	RequestID              string `json:"requestId"`
	NonceID                string `json:"nonceId,omitempty"`
	Nonce                  string `json:"nonce,omitempty"`
	IssuedAt               string `json:"issuedAt"`
	ExpiresAt              string `json:"expiresAt"`
	ReservationAccountID   string `json:"reservationAccountId,omitempty"`
	ReservationUsername    string `json:"reservationUsername,omitempty"`
	ReservationGeneration  string `json:"reservationGeneration,omitempty"`
	ReservationOperationID string `json:"reservationOperationId,omitempty"`
}
type CancelWire struct {
	Kind            string `json:"kind"`
	Version         string `json:"version"`
	Audience        string `json:"audience"`
	OperationID     string `json:"operationId"`
	ServerID        string `json:"serverId"`
	AccountID       string `json:"accountId"`
	PublicKey       string `json:"publicKey"`
	LocalGeneration int64  `json:"localGeneration,string"`
	RequestID       string `json:"requestId"`
	Mode            string `json:"mode"`
	ClaimGeneration string `json:"claimGeneration,omitempty"`
}

type Verified struct {
	Wire                Wire
	IssuedAt, ExpiresAt time.Time
	Digest              [32]byte
}

func (Verified) String() string     { return "[verified claim facts]" }
func (v Verified) GoString() string { return v.String() }

// Verify authenticates only the expected domain. The trusted audience and now
// come from server configuration/clock, never from the submitted envelope.
func Verify(payload, signature []byte, audience, kind, purpose string, now time.Time) (Verified, error) {
	var w Wire
	if !validAudience(audience) || (kind != NonceRequest && kind != ClaimProof) || (purpose != Finalize && purpose != Result) || now.IsZero() || !canonical(payload, &w) {
		return Verified{}, ErrInvalid
	}
	if w.Kind != kind || w.Version != "1" || w.Audience != audience || w.Purpose != purpose || !validBinding(w.OperationID, w.ServerID, w.AccountID, w.LocalGeneration) || w.ApprovalRevision < 1 || !validID(w.RequestID) {
		return Verified{}, ErrInvalid
	}
	key, ok := decode(w.PublicKey, ed25519.PublicKeySize)
	derived, _ := ServerID(key)
	if !ok || w.ServerID != derived || len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, payload, signature) {
		return Verified{}, ErrInvalid
	}
	if kind == NonceRequest {
		if w.NonceID != "" || w.Nonce != "" {
			return Verified{}, ErrInvalid
		}
	} else {
		if !validID(w.NonceID) {
			return Verified{}, ErrInvalid
		}
		if _, ok = decode(w.Nonce, 32); !ok {
			return Verified{}, ErrInvalid
		}
	}
	issued, e := timestamp(w.IssuedAt)
	if e != nil {
		return Verified{}, ErrInvalid
	}
	expiry, e := timestamp(w.ExpiresAt)
	if e != nil || !expiry.After(issued) || expiry.Sub(issued) > 30*time.Second || issued.After(now.Add(5*time.Second)) || !expiry.After(now) {
		return Verified{}, ErrInvalid
	}
	return Verified{Wire: w, IssuedAt: issued, ExpiresAt: expiry, Digest: sha256.Sum256(payload)}, nil
}

// Cancellation deliberately has no expiry: its exact operation tombstone and
// original-key binding make a retained signed outbox assertion deny-only.
func VerifyCancellation(payload, signature []byte, audience string) (CancelWire, error) {
	var w CancelWire
	if !validAudience(audience) || !canonical(payload, &w) || w.Kind != "portico.claim.cancel" || w.Version != "1" || w.Audience != audience || !validBinding(w.OperationID, w.ServerID, w.AccountID, w.LocalGeneration) || !validID(w.RequestID) {
		return CancelWire{}, ErrInvalid
	}
	if !((w.Mode == "intent" && w.ClaimGeneration == "") || (w.Mode == "committed" && validGeneration(w.ClaimGeneration))) {
		return CancelWire{}, ErrInvalid
	}
	key, ok := decode(w.PublicKey, ed25519.PublicKeySize)
	derived, _ := ServerID(key)
	if !ok || w.ServerID != derived || len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, payload, signature) {
		return CancelWire{}, ErrInvalid
	}
	return w, nil
}
func canonical(payload []byte, out any) bool {
	if len(payload) == 0 || len(payload) > 16384 || json.Unmarshal(payload, out) != nil {
		return false
	}
	raw, e := json.Marshal(out)
	return e == nil && bytes.Equal(raw, payload)
}
func decode(v string, n int) ([]byte, bool) {
	b, e := base64.RawURLEncoding.Strict().DecodeString(v)
	return b, e == nil && len(b) == n && base64.RawURLEncoding.EncodeToString(b) == v
}
func timestamp(v string) (time.Time, error) {
	t, e := time.Parse(time.RFC3339Nano, v)
	if e != nil || t.UTC().Format(time.RFC3339Nano) != v {
		return time.Time{}, ErrInvalid
	}
	return t, nil
}
func validBinding(op, server, account string, generation int64) bool {
	return validID(op) && validID(server) && validID(account) && generation >= 0
}
func validID(v string) bool {
	if len(v) < 1 || len(v) > 128 {
		return false
	}
	for _, b := range []byte(v) {
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-') {
			return false
		}
	}
	return true
}
func validGeneration(v string) bool {
	n, e := strconv.ParseInt(v, 10, 64)
	return e == nil && n > 0 && strconv.FormatInt(n, 10) == v
}
func validAudience(v string) bool {
	if len(v) > 2048 || strings.ContainsAny(v, "\r\n\t ") {
		return false
	}
	u, e := url.Parse(v)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && u.RawFragment == "" && u.Opaque == "" && strings.ToLower(u.Host) == u.Host && u.String() == v
}
