// Package appstore verifies and decodes App Store Server Notifications v2.
//
// Apple POSTs one JSON object, {"signedPayload":"<JWS>"}, to an endpoint the
// operator configured in App Store Connect. The JWS is the only authentication
// the request carries: there is no shared secret, no bearer token, and no
// meaningful source address. Verification is therefore the whole security
// boundary, and it has four parts, all of which must hold:
//
//  1. Compact JWS framing, ES256 only. Apple signs with ES256; accepting any
//     other algorithm — above all "none" — would make the signature optional.
//  2. An x5c certificate chain in the protected header, leaf first, which chains
//     to a root this deployment pinned. Apple does not publish a JWKS for these
//     notifications; the trust anchor is the Apple Root CA - G3 certificate the
//     operator installs, and an empty key set means the feature is off rather
//     than open.
//  3. The signature verifies against the leaf certificate's public key over the
//     exact signing input.
//  4. The decoded payload's bundle id and environment match what this deployment
//     expects, so a notification for someone else's app, or a sandbox
//     notification arriving at production, is refused rather than stored.
//
// Nothing here logs the request body, the JWS, or any transaction identifier:
// ACCOUNT_SECURITY.md classes an Apple POST body as data that must not be
// logged, and a decoded payload names a paying customer.
package appstore

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"time"
)

var (
	// ErrNotConfigured means no trust anchor was installed, so intake is off.
	ErrNotConfigured = errors.New("App Store notification intake is not configured")
	// ErrMalformed covers framing: not three parts, not base64url, not JSON.
	ErrMalformed = errors.New("malformed App Store notification")
	// ErrAlgorithm is an unexpected or absent signing algorithm.
	ErrAlgorithm = errors.New("unsupported App Store notification algorithm")
	// ErrChain is a missing, malformed or untrusted certificate chain.
	ErrChain = errors.New("untrusted App Store notification certificate chain")
	// ErrSignature is a chain that verified but a signature that did not.
	ErrSignature = errors.New("invalid App Store notification signature")
	// ErrAudience is a notification for another app or another environment.
	ErrAudience = errors.New("App Store notification is not for this application")
)

// MaxPayloadBytes bounds the signed payload. A v2 notification with a full
// certificate chain is a few kilobytes; a quarter of a megabyte is generous and
// keeps a hostile POST from becoming a parsing budget.
const MaxPayloadBytes = 256 << 10

// KeySet is the pinned trust anchor and the audience this deployment accepts.
// It is built from operator-owned configuration; an empty Roots disables intake.
type KeySet struct {
	// Roots are the PEM-encoded root certificates the chain must reach. In
	// production this is Apple Root CA - G3. It is deliberately explicit: a
	// system trust store would let any public CA mint a notification signer.
	Roots *x509.CertPool
	// BundleID is this application's bundle identifier.
	BundleID string
	// Environment is "Production" or "Sandbox". A notification from the other
	// environment is refused, so sandbox traffic can never move real entitlements.
	Environment string
	// Now is a clock seam for tests. Nil means time.Now.
	Now func() time.Time
}

// LoadKeySet builds a key set from PEM bytes. It refuses a set that parses to no
// certificate: a configuration file the operator meant to install but that
// contained nothing must be a startup error, not a silently open endpoint.
func LoadKeySet(rootPEM []byte, bundleID, environment string) (KeySet, error) {
	if environment != "Production" && environment != "Sandbox" {
		return KeySet{}, errors.New("App Store environment must be Production or Sandbox")
	}
	if strings.TrimSpace(bundleID) == "" {
		return KeySet{}, errors.New("App Store bundle identifier is required")
	}
	pool := x509.NewCertPool()
	rest := rootPEM
	count := 0
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return KeySet{}, err
		}
		pool.AddCert(certificate)
		count++
	}
	if count == 0 {
		return KeySet{}, errors.New("no App Store root certificate in the configured key set")
	}
	return KeySet{Roots: pool, BundleID: bundleID, Environment: environment}, nil
}

func (k KeySet) now() time.Time {
	if k.Now != nil {
		return k.Now()
	}
	return time.Now()
}

// Notification is the decoded payload, reduced to the facts this service stores.
type Notification struct {
	UUID                  string           `json:"notificationUUID"`
	NotificationType      string           `json:"notificationType"`
	Subtype               string           `json:"subtype"`
	Version               string           `json:"version"`
	SignedDate            int64            `json:"signedDate"`
	Data                  NotificationData `json:"data"`
	Summary               json.RawMessage  `json:"summary,omitempty"`
	ExternalPurchaseToken json.RawMessage  `json:"externalPurchaseToken,omitempty"`
}

// NotificationData is the transaction envelope. The signed transaction and
// renewal blobs inside it are themselves JWS values; this package records that
// they were present without decoding them, because entitlement decisions are not
// made here.
type NotificationData struct {
	BundleID              string `json:"bundleId"`
	BundleVersion         string `json:"bundleVersion"`
	Environment           string `json:"environment"`
	AppAppleID            int64  `json:"appAppleId"`
	SignedTransactionInfo string `json:"signedTransactionInfo"`
	SignedRenewalInfo     string `json:"signedRenewalInfo"`
	Status                int    `json:"status"`
}

// SignedAt is the notification's own signing time.
func (n Notification) SignedAt() time.Time {
	if n.SignedDate <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(n.SignedDate).UTC()
}

// TransactionClaims are the few fields of the inner signed transaction this
// service needs to attach a notification to an account. They are read from the
// inner JWS payload only after the outer JWS verified, and the inner JWS is
// verified against the same pinned chain.
type TransactionClaims struct {
	OriginalTransactionID string `json:"originalTransactionId"`
	AppAccountToken       string `json:"appAccountToken"`
	BundleID              string `json:"bundleId"`
}

type joseHeader struct {
	Alg string   `json:"alg"`
	X5C []string `json:"x5c"`
}

// Verify checks one signed payload and returns the decoded notification.
func (k KeySet) Verify(signedPayload string) (Notification, error) {
	var out Notification
	raw, err := k.verifyJWS(signedPayload)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, ErrMalformed
	}
	if out.UUID == "" || out.NotificationType == "" {
		return out, ErrMalformed
	}
	// The audience check is not a formality: a verified signature only proves
	// Apple sent it, not that it concerns this application or this environment.
	if out.Data.BundleID != "" && out.Data.BundleID != k.BundleID {
		return out, ErrAudience
	}
	if out.Data.Environment != "" && out.Data.Environment != k.Environment {
		return out, ErrAudience
	}
	return out, nil
}

// Transaction verifies and decodes the inner signed transaction of a notification.
// An absent one is not an error: several notification types carry none.
func (k KeySet) Transaction(n Notification) (TransactionClaims, bool, error) {
	if n.Data.SignedTransactionInfo == "" {
		return TransactionClaims{}, false, nil
	}
	raw, err := k.verifyJWS(n.Data.SignedTransactionInfo)
	if err != nil {
		return TransactionClaims{}, false, err
	}
	var claims TransactionClaims
	if err = json.Unmarshal(raw, &claims); err != nil {
		return TransactionClaims{}, false, ErrMalformed
	}
	if claims.BundleID != "" && claims.BundleID != k.BundleID {
		return TransactionClaims{}, false, ErrAudience
	}
	return claims, true, nil
}

// verifyJWS performs the four-part check described in the package comment and
// returns the payload bytes. It is the only place a signature is trusted.
func (k KeySet) verifyJWS(compact string) ([]byte, error) {
	if k.Roots == nil {
		return nil, ErrNotConfigured
	}
	if len(compact) == 0 || len(compact) > MaxPayloadBytes {
		return nil, ErrMalformed
	}
	parts := strings.Split(compact, ".")
	if len(parts) != 3 {
		return nil, ErrMalformed
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrMalformed
	}
	var header joseHeader
	if err = json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrMalformed
	}
	// ES256 only. Reading the algorithm from the token and then honouring it is
	// how JWS implementations get broken; here it is compared, never dispatched on.
	if header.Alg != "ES256" {
		return nil, ErrAlgorithm
	}
	if len(header.X5C) < 1 || len(header.X5C) > 8 {
		return nil, ErrChain
	}
	chain := make([]*x509.Certificate, 0, len(header.X5C))
	for _, encoded := range header.X5C {
		der, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil {
			return nil, ErrChain
		}
		certificate, parseErr := x509.ParseCertificate(der)
		if parseErr != nil {
			return nil, ErrChain
		}
		chain = append(chain, certificate)
	}
	intermediates := x509.NewCertPool()
	for _, certificate := range chain[1:] {
		intermediates.AddCert(certificate)
	}
	leaf := chain[0]
	if _, err = leaf.Verify(x509.VerifyOptions{
		Roots:         k.Roots,
		Intermediates: intermediates,
		CurrentTime:   k.now(),
		// Apple's notification signers carry no server-auth EKU, so any usage
		// is accepted. The pinned root, not the EKU, is what bounds the signer.
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return nil, ErrChain
	}
	key, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || key.Curve.Params().BitSize != 256 {
		return nil, ErrAlgorithm
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return nil, ErrSignature
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(key, digest[:], r, s) {
		return nil, ErrSignature
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrMalformed
	}
	return payload, nil
}

// FailureReason maps a verification error to the short, non-revealing string
// recorded for operators. It never includes any part of the refused payload.
func FailureReason(err error) string {
	switch {
	case errors.Is(err, ErrNotConfigured):
		return "not_configured"
	case errors.Is(err, ErrMalformed):
		return "malformed"
	case errors.Is(err, ErrAlgorithm):
		return "algorithm"
	case errors.Is(err, ErrChain):
		return "chain"
	case errors.Is(err, ErrSignature):
		return "signature"
	case errors.Is(err, ErrAudience):
		return "audience"
	}
	return "unknown"
}
