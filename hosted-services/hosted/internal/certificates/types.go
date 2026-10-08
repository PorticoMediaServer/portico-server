// Package certificates owns Hosted's DNS-01 ACME coordinator. It never receives
// server TLS private keys. Every positive operation uses current claim authority;
// deny-only cleanup/revocation keeps its own durable identity after unclaim.
package certificates

import (
	"context"
	"crypto/x509"
	"errors"
	"time"

	"portico.local/hosted/internal/directory"
)

var (
	ErrNotFound      = errors.New("certificate request not found")
	ErrInvalid       = errors.New("invalid certificate request")
	ErrConflict      = errors.New("certificate request conflicts with durable intent")
	ErrRateLimited   = errors.New("certificate issuance is not yet due")
	ErrConfiguration = errors.New("certificate coordinator requires configuration")
	ErrLease         = errors.New("certificate work lease is no longer current")
	ErrUncertain     = errors.New("provider acceptance requires reconciliation")
)

type Order struct {
	ID, RequestID, Namespace, Issuer, State, ProviderURL, ErrorCode string
	Authority                                                       directory.ServerAuthority
	CSR, Chain                                                      []byte
	NotBefore, NotAfter, CreatedAt, NextAttempt                     time.Time
	Attempts                                                        int
	Lease                                                           string
	CancelRequested, RevocationDone, CreateAttempted                bool
}

type Challenge struct {
	AuthorizationURL, ChallengeURL, Namespace, Value string
	CleanupDone                                      bool
	CleanupAfter                                     time.Time
}

type Account struct {
	Cipher []byte
	URL    string
}

type Repository interface {
	WithDirectNamespace(context.Context, directory.ServerAuthority, func(string) error) error
	SubmitCertificate(context.Context, directory.ServerAuthority, string, string, string, []byte) (Order, error)
	LookupCertificate(context.Context, directory.ServerAuthority, string) (Order, error)
	ReadCertificate(context.Context, directory.ServerAuthority, string) (Order, error)
	CancelCertificate(context.Context, directory.ServerAuthority, string) error
	ACMEAccount(context.Context, string, string, []byte) (Account, error)
	SaveACMEAccountURL(context.Context, string, string) error
	LeaseCertificate(context.Context, string, string, bool) (Order, error)
	CheckCertificateLease(context.Context, Order, bool) error
	SetCertificateState(context.Context, Order, string, string, time.Time) error
	// AdmitCertificateCreation takes one slot of the issuer's per-minute order budget, shared by
	// every instance. A positive wait means the budget is spent and nothing was taken.
	AdmitCertificateCreation(ctx context.Context, o Order, perMinute int) (wait time.Duration, err error)
	RecordProviderOrder(context.Context, Order, string) error
	SaveCertificateChain(context.Context, Order, []byte, *x509.Certificate) error
	CertificateChallenges(context.Context, Order) ([]Challenge, error)
	OwnCertificateChallenge(context.Context, Order, Challenge) error
	FinishCertificateCleanup(context.Context, Order, Challenge) error
	FinishCertificateRevocation(context.Context, Order) error
	ReleaseCertificate(context.Context, Order, time.Time, string) error
	ReconcileCertificateURL(context.Context, string, string, string) error
}

type Namespace struct {
	Namespace            string `json:"namespace"`
	DNSName              string `json:"dnsName"`
	DNS01Target          string `json:"dns01Target"`
	OperationID          string `json:"operationId"`
	ClaimGeneration      string `json:"claimGeneration"`
	CredentialGeneration string `json:"credentialGeneration"`
	Issuer               string `json:"issuer"`
	Environment          string `json:"environment"`
	Configured           bool   `json:"configured"`
}

type Status struct {
	ID                   string     `json:"id"`
	RequestID            string     `json:"requestId"`
	Namespace            string     `json:"namespace"`
	State                string     `json:"state"`
	Issuer               string     `json:"issuer"`
	Environment          string     `json:"environment"`
	OperationID          string     `json:"operationId"`
	ClaimGeneration      string     `json:"claimGeneration"`
	CredentialGeneration string     `json:"credentialGeneration"`
	NotAfter             *time.Time `json:"notAfter,omitempty"`
	NextAttempt          time.Time  `json:"nextAttemptAt"`
	ErrorCode            string     `json:"errorCode,omitempty"`
}
