package httpapi

import (
	"encoding/json"
	"time"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/directroute"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/trust"
)

// Named wire shapes for handlers that previously decoded or wrote anonymous
// structs and maps. internal/contract declares these exact types, so the Hosted
// API contract and the handlers share one Go type per request and response.
// Response fields are declared in JSON key order, which is the order the maps
// they replace were encoded in.

// CodeResponse acknowledges an operation with a stable result or error code.
type CodeResponse struct {
	Code string `json:"code"`
}

// StatusResponse reports the state an operation reached.
type StatusResponse struct {
	Status string `json:"status"`
}

// StatusCodeResponse reports the state an operation reached and its result code.
type StatusCodeResponse struct {
	Code   string `json:"code"`
	Status string `json:"status"`
}

// StatusMessageResponse reports a state with a human-readable explanation.
type StatusMessageResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

// AcceptedResponse acknowledges a provider notification.
type AcceptedResponse struct {
	Accepted bool `json:"accepted"`
}

// AppStoreReceipt answers an App Store Server Notification. Reason is present
// only when the notification was refused.
type AppStoreReceipt struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

// ServerList is the account's server directory.
type ServerList struct {
	MembershipEnding *directory.MembershipEndingNotice `json:"membershipEnding,omitempty"`
	Items            []directory.Server                `json:"items"`
	// Version is the account's server-list version, also sent as the ETag. A
	// device keeps it and asks GET /v1/server-list/{watch} whether it moved.
	Version string `json:"version"`
	// Watch is the capability for that check; it needs no account session.
	Watch string `json:"watch"`
}

// KeySet is Hosted's current directory signing key and its trust chain.
type KeySet struct {
	Certificates []*trust.Certificate `json:"certificates"`
	KeyID        string               `json:"keyId"`
	PublicKey    string               `json:"publicKey"`
	Root         string               `json:"root"`
}

// SignInRequest is a password sign-in.
type SignInRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	SessionMode string `json:"sessionMode"`
	RequestID   string `json:"requestId"`
}

// IdentityOptions lists the sign-in methods this deployment offers.
type IdentityOptions struct {
	AccountSecurity bool `json:"accountSecurity"`
	// Exact supported credential continuation; older runtimes omit this fact.
	CredentialProviderReauthentication string `json:"credentialProviderReauthentication,omitempty"`
	// NativeCompletion is "code": native apps may start provider sign-in with
	// completion "code" to use the completion-code return.
	NativeCompletion        string             `json:"nativeCompletion"`
	Password                bool               `json:"password"`
	Providers               []IdentityProvider `json:"providers"`
	Recovery                bool               `json:"recovery"`
	Registration            bool               `json:"registration"`
	VerifiedContactRequired bool               `json:"verifiedContactRequired"`
}

// IdentityProvider describes one external sign-in provider.
type IdentityProvider struct {
	Enabled         bool     `json:"enabled"`
	ID              string   `json:"id"`
	Native          bool     `json:"native"`
	NativeClientIDs []string `json:"nativeClientIds"`
}

// IdentityStart is a started provider sign-in transaction.
type IdentityStart struct {
	AuthorizationURL string    `json:"authorizationUrl"`
	ExpiresAt        time.Time `json:"expiresAt"`
	Mode             string    `json:"mode"`
	NativeNonce      string    `json:"nativeNonce"`
	TransactionID    string    `json:"transactionId"`
}

// NativeAppleRequest carries a native Sign in with Apple assertion.
type NativeAppleRequest struct {
	IDToken     string `json:"idToken"`
	DisplayName string `json:"displayName"`
}

// ContactRequest asks for a verification code at a contact address.
type ContactRequest struct {
	ContactAddress string `json:"contactAddress"`
	RequestID      string `json:"requestId"`
}

// ChallengeCodeRequest answers a verification challenge.
type ChallengeCodeRequest struct {
	ChallengeID string `json:"challengeId"`
	Code        string `json:"code"`
}

// AppleNotificationRequest is Apple's server-to-server account notification.
type AppleNotificationRequest struct {
	Payload string `json:"payload"`
}

// TokenRequest carries a mailed single-use token.
type TokenRequest struct {
	Token string `json:"token"`
}

// RegistrationVerifyRequest completes an email registration.
type RegistrationVerifyRequest struct {
	Code        string `json:"code"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

// RequestIDRequest makes a resend idempotent.
type RequestIDRequest struct {
	RequestID string `json:"requestId"`
}

// RecoveryCompleteRequest sets a new password from a recovery token.
type RecoveryCompleteRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
	Factor      string `json:"factor"`
}

// MFACompleteRequest answers a sign-in MFA challenge.
type MFACompleteRequest struct {
	ChallengeID    string `json:"challengeId"`
	ChallengeToken string `json:"challengeToken"`
	Factor         string `json:"factor"`
}

// SecurityProofRequest proves the password for a purpose-bound security action.
type SecurityProofRequest struct {
	Intent   identity.SecurityIntent `json:"intent"`
	Password string                  `json:"password"`
	Factor   string                  `json:"factor"`
}

// SecurityConfirmRequest confirms an MFA enrollment or email change.
type SecurityConfirmRequest struct {
	ChallengeID string `json:"challengeId"`
	Code        string `json:"code"`
	OperationID string `json:"operationId"`
}

// SecurityEmailResendRequest rotates only the initiating family's pending code.
type SecurityEmailResendRequest struct {
	ChallengeID string `json:"challengeId"`
	OperationID string `json:"operationId"`
}

// RefreshRequest rotates a refresh credential. Browsers omit RefreshToken.
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
	FamilyID     string `json:"familyId"`
	RequestID    string `json:"requestId"`
}

// RefreshRevokeRequest ends a refresh family. Browsers omit RefreshToken.
type RefreshRevokeRequest struct {
	RefreshToken string `json:"refreshToken"`
	FamilyID     string `json:"familyId"`
}

// UserCodeRequest names a device authorization by its user code.
type UserCodeRequest struct {
	UserCode string `json:"userCode"`
}

// DeviceDecisionRequest approves or denies a device authorization.
type DeviceDecisionRequest struct {
	RequestID string `json:"requestId"`
	UserCode  string `json:"userCode"`
	Decision  string `json:"decision"`
}

// DeviceTokenRequest polls a device authorization for its session.
type DeviceTokenRequest struct {
	DeviceCode  string `json:"deviceCode"`
	SessionMode string `json:"sessionMode"`
}

// DeviceCodeRequest names a device authorization by its device code.
type DeviceCodeRequest struct {
	DeviceCode string `json:"deviceCode"`
}

// NotificationReadRequest marks feed items, or every item, read.
type NotificationReadRequest struct {
	IDs []string `json:"ids"`
	All bool     `json:"all"`
}

// DisplayNameRequest renames the account.
type DisplayNameRequest struct {
	DisplayName string `json:"displayName"`
}

// AccountDeletionRequest accepts a deletion preview's disposition.
type AccountDeletionRequest struct {
	Token       string `json:"deletionToken"`
	Disposition string `json:"disposition"`
	// Confirmation is the account's email typed by the user (its username for
	// an account without an email); no password (A80).
	Confirmation string `json:"confirmation"`
}

// DeletionTokenRequest reads a deletion receipt by its token.
type DeletionTokenRequest struct {
	Token string `json:"deletionToken"`
}

// AppStoreNotificationRequest is Apple's App Store Server Notification envelope.
type AppStoreNotificationRequest struct {
	SignedPayload string `json:"signedPayload"`
}

// CertificateLookupRequest finds a certificate order by its request ID.
type CertificateLookupRequest struct {
	RequestID string `json:"requestId"`
}

// CertificateOrderRequest submits a CSR. CSR is standard base64 (DER), the
// encoding/json form of the []byte the handler decoded before.
type CertificateOrderRequest struct {
	RequestID string `json:"requestId"`
	CSR       string `json:"csr"`
}

// CertificateChain is an issued chain in standard base64, the encoding/json
// form of the []byte the handler wrote before.
type CertificateChain struct {
	Chain string `json:"chain"`
}

// CancelRequestedResponse acknowledges a certificate order cancellation.
type CancelRequestedResponse struct {
	CancelRequested bool `json:"cancelRequested"`
}

// AddressChangeList is a page of the DNS address feed.
type AddressChangeList struct {
	Items []directroute.AddressChange `json:"items"`
}

// DirectRouteRequest publishes a server's direct address. Port 0 means 32500.
type DirectRouteRequest struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// DirectRouteWithdrawRequest withdraws one address family.
type DirectRouteWithdrawRequest struct {
	Family string `json:"family"`
}

// DirectRouteWithdrawal reports a withdrawn address family.
type DirectRouteWithdrawal struct {
	Family    string `json:"family"`
	Namespace string `json:"namespace"`
	Withdrawn bool   `json:"withdrawn"`
}

// RouteProofRequest registers an endpoint for an observed route generation.
type RouteProofRequest struct {
	Generation int64                     `json:"generation,string"`
	Endpoint   directory.EndpointRequest `json:"endpoint"`
}

// ClaimSignedRequest is a server-signed claim protocol message (base64url, unpadded).
type ClaimSignedRequest struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// ClaimCodeRequest looks up or decides a claim by its approval code.
type ClaimCodeRequest struct {
	Code             string `json:"code"`
	ReplacesServerID string `json:"replacesServerId,omitempty"`
}

// ClaimEventResult ends a claim decision stream. ApprovalEnvelope is the sealed
// approval (null when the claim closed without one).
type ClaimEventResult struct {
	ApprovalEnvelope json.RawMessage `json:"approvalEnvelope"`
	Status           string          `json:"status"`
}
