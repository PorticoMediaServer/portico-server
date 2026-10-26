// Package contract declares the Hosted API: every /v1 route Hosted serves, with
// the exact Go types its handler decodes and writes. cmd/apigen generates the
// OpenAPI document and TypeScript contract from it, and this package's tests
// hold it to the routes internal/httpapi actually registers.
//
// Provider callbacks (/auth/sso/...), /health and /ready are not part of the
// API and are not declared.
package contract

import (
	"reflect"

	"portico.local/apikit"
	"portico.local/apikit/apierror"
	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/appauth"
	"portico.local/hosted/internal/appstore"
	"portico.local/hosted/internal/buildinfo"
	"portico.local/hosted/internal/certificates"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/device"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/directroute"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/serverreturn"
	"portico.local/hosted/internal/sessions"
)

// Hosted's access rules. apikit's Access is a plain string; these name who may
// call a Hosted route and with what credential.
const (
	// Public routes need no credential. Account-continuation routes are public:
	// their authority is a private continuation binding (cookie or
	// X-Portico-Continuation), a mailed token, or a device code.
	Public apikit.Access = "public"
	// AccountSession routes need a Portico Account access token (Bearer).
	AccountSession apikit.Access = "account_session"
	// ServerCredential routes need the path server's Hosted credential (Bearer).
	ServerCredential apikit.Access = "server_credential"
	// SignedRequest routes carry a server- or web-signed claim message instead of a bearer.
	SignedRequest apikit.Access = "signed_request"
	// DNSFeed is the DNS nodes' address feed credential.
	DNSFeed apikit.Access = "dns_feed"
	// ProviderNotification routes are authenticated by the provider's JWS signature.
	ProviderNotification apikit.Access = "provider_notification"
)

// Title is the generated contract's OpenAPI title.
const Title = "Portico Hosted API"

// Body limits the handlers enforce.
const (
	// JSONBody is decode's limit (internal/httpapi).
	JSONBody = 64 << 10
	// ClaimBody is claimDecode's limit for the server claim protocol.
	ClaimBody = 16384
)

// NoBody is the request type of a route that reads no request body, and the
// response type of a route that answers 204 No Content.
type NoBody struct{}

// Options is how cmd/apigen generates the Hosted contract. The catalogue names
// explicit semantic codes; it does not classify an HTTP status on its own or
// replace Hosted's {"error":{"code","message","retryable"}} envelope.
func Options() apikit.Options {
	return apikit.Options{Title: Title, Errors: []apierror.Definition{{Code: "username_unavailable", Status: 409, Message: "That username is already taken. Choose another.", Retry: apierror.Never}}}
}

type entry apikit.Definition

// Hosted keeps its own account/server/provider credential checks. Scope is
// contract metadata; it never makes a local-server API key a Hosted credential.
func route[Request, Response any](method, path, id, summary string, a apikit.Access, scope apikit.Scope, status int) entry {
	limit := int64(JSONBody)
	if reflect.TypeFor[Request]() == reflect.TypeFor[NoBody]() {
		limit = 0
	}
	return entry{Metadata: apikit.Metadata{ID: id, Method: method, Path: path, Summary: summary, Access: a, Requires: scope, Lane: apikit.Default, Cost: apikit.Constant, BodyLimit: limit, Errors: []string{}, Status: status}, Request: reflect.TypeFor[Request](), Response: reflect.TypeFor[Response]()}
}
func (e entry) lane(l apikit.Lane) entry     { e.Lane = l; return e }
func (e entry) errors(codes ...string) entry { e.Errors = append([]string{}, codes...); return e }
func (e entry) paged(query ...string) entry {
	e.Cost = apikit.PageSized
	e.Query = query
	return e
}
func (e entry) query(keys ...string) entry { e.Query = keys; return e }
func (e entry) body(limit int64) entry     { e.BodyLimit = limit; return e }

// Definitions is every /v1 route Hosted serves. Routes whose subsystem is not
// configured in a deployment (OIDC, device authorization, certificates, direct
// routes, App Store intake) are still declared: they are part of the API.
func Definitions() []apikit.Definition {
	security := apikit.Security
	entries := []entry{
		// Public service identity.
		route[NoBody, buildinfo.Release]("GET", "/v1/system", "system_get", "Read the public build identity.", Public, apikit.ScopeOpen, 200),
		route[NoBody, httpapi.KeySet]("GET", "/v1/keys", "keys_get", "Read the directory signing key and its trust chain.", Public, apikit.ScopeOpen, 200),

		// Sign-in, registration and recovery.
		route[NoBody, httpapi.IdentityOptions]("GET", "/v1/identity/options", "identity_options_get", "List the sign-in methods this deployment offers.", Public, apikit.ScopeOpen, 200).lane(security),
		// contract: an MFA-enrolled account answers 202 with identity.MFAChallenge instead.
		route[httpapi.SignInRequest, identity.Session]("POST", "/v1/sessions", "session_create", "Sign in with a username and password.", Public, apikit.ScopeSession, 200).lane(security),
		route[postgres.RegistrationInput, identity.Registration]("POST", "/v1/accounts", "account_create", "Start an email registration (alias of registration_create).", Public, apikit.ScopeSession, 202).lane(security).errors("username_unavailable"),
		route[postgres.RegistrationInput, identity.Registration]("POST", "/v1/registrations", "registration_create", "Start an email registration.", Public, apikit.ScopeSession, 202).lane(security).errors("username_unavailable"),
		// contract: answers JSON null when the caller has no continuation.
		route[NoBody, identity.Registration]("GET", "/v1/registrations/current", "registration_current", "Read the caller's pending registration.", Public, apikit.ScopeSession, 200).lane(security),
		route[NoBody, identity.Registration]("GET", "/v1/registrations/{id}", "registration_get", "Read a pending registration.", Public, apikit.ScopeSession, 200).lane(security),
		route[httpapi.RegistrationVerifyRequest, identity.Session]("POST", "/v1/registrations/{id}/verify", "registration_verify", "Verify a registration code and create the account.", Public, apikit.ScopeSession, 201).lane(security).errors("username_unavailable"),
		route[httpapi.RequestIDRequest, identity.Registration]("POST", "/v1/registrations/{id}/resend", "registration_resend", "Resend a registration code.", Public, apikit.ScopeSession, 202).lane(security),
		route[NoBody, NoBody]("POST", "/v1/registrations/cancel", "registration_cancel", "Cancel the caller's pending registration.", Public, apikit.ScopeSession, 204).lane(security),
		route[httpapi.TokenRequest, postgres.EmailLinkResult]("POST", "/v1/email-links/confirm", "email_link_confirm", "Confirm a mailed verification link.", Public, apikit.ScopeSession, 200).lane(security),
		route[postgres.RecoveryRequest, httpapi.StatusMessageResponse]("POST", "/v1/account/recovery/request", "account_recovery_request", "Request password recovery instructions.", Public, apikit.ScopeSession, 202).lane(security),
		route[httpapi.RecoveryCompleteRequest, httpapi.StatusResponse]("POST", "/v1/account/recovery/complete", "account_recovery_complete", "Set a new password from a recovery token.", Public, apikit.ScopeSession, 200).lane(security),
		route[httpapi.MFACompleteRequest, identity.Session]("POST", "/v1/identity/mfa/complete", "identity_mfa_complete", "Answer a sign-in MFA challenge.", Public, apikit.ScopeSession, 200).lane(security),

		// A closed provider Security return never creates a sign-in grant.
		route[NoBody, NoBody]("POST", "/v1/identity/transactions/{id}/cancel-security", "identity_security_cancel", "Cancel only an original browser credential reauthentication.", Public, apikit.ScopeSession, 204).lane(security),
		route[NoBody, NoBody]("POST", "/v1/account/security-return-intents/{id}/cancel", "account_security_return_cancel", "Cancel only the original Account Settings Security intent.", AccountSession, apikit.ScopeSession, 204).lane(security),
		route[serverreturn.SecurityReturnStart, postgres.SecurityReturnIntent]("POST", "/v1/account/security-return-intents", "account_security_return_prepare", "Register an original Account Settings Security intent.", AccountSession, apikit.ScopeSession, 201).lane(security),
		route[httpapi.RequestIDRequest, httpapi.SecurityReturnStartResponse]("POST", "/v1/account/security-return-intents/{id}/start", "account_security_return_start", "Reauthenticate the original provider from the fixed Hosted origin.", Public, apikit.ScopeSession, 200).lane(security),
		route[NoBody, serverreturn.Return]("POST", "/v1/identity/transactions/{id}/security-return", "identity_security_return", "Return the original verified Security transaction to its registered server.", Public, apikit.ScopeSession, 200).lane(security),
		route[postgres.SecurityReturnCompleteInput, postgres.OIDCCompletion]("POST", "/v1/account/security-returns/{id}/complete", "account_security_return_complete", "Complete only the original confirmed provider Security continuation.", AccountSession, apikit.ScopeSession, 200).lane(security),
		// Provider sign-in transactions.
		route[postgres.OIDCStartInput, httpapi.IdentityStart]("POST", "/v1/identity/{provider}/start", "identity_start", "Start a provider sign-in transaction.", Public, apikit.ScopeSession, 200).lane(security),
		// contract: answers JSON null when the caller has no continuation.
		route[NoBody, postgres.OIDCView]("GET", "/v1/identity/current", "identity_current", "Read the caller's provider sign-in transaction.", Public, apikit.ScopeSession, 200).lane(security),
		route[NoBody, postgres.OIDCView]("GET", "/v1/identity/transactions/{id}", "identity_transaction_get", "Read a provider sign-in transaction.", Public, apikit.ScopeSession, 200).lane(security),
		// contract: answers 202 with identity.MFAChallenge when MFA is required, and
		// 200 with postgres.OIDCCompletion for a link or proof transaction.
		route[postgres.OIDCCompleteInput, identity.Session]("POST", "/v1/identity/transactions/{id}/complete", "identity_transaction_complete", "Complete a provider sign-in transaction.", Public, apikit.ScopeSession, 200).lane(security).errors("username_unavailable"),
		route[httpapi.NativeAppleRequest, postgres.OIDCView]("POST", "/v1/identity/transactions/{id}/native-apple", "identity_transaction_native_apple", "Present a native Sign in with Apple assertion.", Public, apikit.ScopeSession, 200).lane(security),
		route[httpapi.ContactRequest, postgres.ContactChallenge]("POST", "/v1/identity/transactions/{id}/contact", "identity_transaction_contact", "Send a contact verification code.", Public, apikit.ScopeSession, 202).lane(security),
		route[httpapi.ChallengeCodeRequest, postgres.OIDCView]("POST", "/v1/identity/transactions/{id}/contact/verify", "identity_transaction_contact_verify", "Verify a contact verification code.", Public, apikit.ScopeSession, 200).lane(security),
		route[NoBody, NoBody]("POST", "/v1/identity/cancel", "identity_cancel", "Cancel the caller's provider sign-in transaction.", Public, apikit.ScopeSession, 204).lane(security),
		route[httpapi.AppleNotificationRequest, httpapi.AcceptedResponse]("POST", "/v1/identity/apple/notifications", "identity_apple_notification", "Receive a Sign in with Apple server notification.", ProviderNotification, apikit.ScopeSession, 200).lane(security).body(oidclogin.AppleNotificationMaxBytes),

		// Sessions.
		route[NoBody, identity.Viewer]("GET", "/v1/me", "me_get", "Read the signed-in account.", AccountSession, apikit.ScopeSession, 200),
		route[NoBody, NoBody]("DELETE", "/v1/sessions/current", "session_current_delete", "Sign out the presented access token.", AccountSession, apikit.ScopeSession, 204).lane(security),
		route[httpapi.RefreshRequest, identity.Session]("POST", "/v1/sessions/refresh", "session_refresh", "Rotate a refresh credential for a new session.", Public, apikit.ScopeSession, 200).lane(security),
		route[httpapi.RefreshRevokeRequest, NoBody]("POST", "/v1/sessions/revoke", "session_revoke", "End a refresh family.", Public, apikit.ScopeSession, 204).lane(security),
		route[NoBody, NoBody]("POST", "/v1/sessions/logout-all", "session_logout_all", "Sign out every session of the account.", AccountSession, apikit.ScopeSession, 204).lane(security),
		route[NoBody, sessions.ReviewPage]("GET", "/v1/account/sessions", "account_sessions_list", "List the account's sessions.", AccountSession, apikit.ScopeSession, 200).lane(security).paged("cursor"),
		route[NoBody, sessions.ReviewRevocation]("DELETE", "/v1/account/sessions/{family}", "account_session_delete", "End one of the account's sessions.", AccountSession, apikit.ScopeSession, 200).lane(security),

		// Public-client sign-in. The Hosted browser cookie plus CSRF authorizes
		// review/decision; no Hosted credential is returned to the app.
		route[appauth.Request, appauth.Review]("POST", "/v1/app-authorizations", "app_authorization_review", "Review sign-in to one server for an unregistered app.", Public, apikit.ScopeSession, 201).lane(security),
		route[appauth.Decision, appauth.Return]("POST", "/v1/app-authorizations/{id}/decision", "app_authorization_decide", "Approve or deny the reviewed app sign-in.", Public, apikit.ScopeSession, 200).lane(security),
		route[appauth.Exchange, appauth.Grant]("POST", "/v1/app-authorizations/token", "app_authorization_exchange", "Exchange a one-time PKCE code for one server identity assertion.", Public, apikit.ScopeOpen, 200).lane(security),

		route[serverreturn.RegistrationRequest, serverreturn.Registration]("POST", "/v1/servers/{id}/return-registration", "server_return_registration", "Resolve an installed server's exact verified return origin.", ServerCredential, apikit.ScopeAdmin, 200).lane(security),
		route[serverreturn.Start, serverreturn.Started]("POST", "/v1/server-returns/start", "server_return_start", "Begin a registered first-party server return.", AccountSession, apikit.ScopeSession, 201).lane(security),
		route[serverreturn.Complete, serverreturn.Return]("POST", "/v1/server-returns/{id}/complete", "server_return_complete", "Complete a registered first-party server return.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[serverreturn.ExchangeRequest, serverreturn.Grant]("POST", "/v1/server-returns/exchange", "server_return_exchange", "Spend a registered return code or an installed-server owner continuation.", Public, apikit.ScopeOpen, 200).lane(security),

		// Device and TV authorization.
		route[device.Start, device.Authorization]("POST", "/v1/device-authorizations", "device_authorization_create", "Start a device authorization.", Public, apikit.ScopeOpen, 201).lane(security),
		route[httpapi.UserCodeRequest, device.Preview]("POST", "/v1/device-authorizations/review", "device_authorization_review", "Review a device authorization by its user code.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[httpapi.DeviceDecisionRequest, NoBody]("POST", "/v1/device-authorizations/decision", "device_authorization_decide", "Approve or deny a device authorization.", AccountSession, apikit.ScopeSession, 204).lane(security),
		route[httpapi.DeviceTokenRequest, identity.Session]("POST", "/v1/device-authorizations/token", "device_authorization_token", "Poll a device authorization for its session.", Public, apikit.ScopeOpen, 200).lane(security),
		route[httpapi.DeviceCodeRequest, NoBody]("POST", "/v1/device-authorizations/cancel", "device_authorization_cancel", "Cancel a device authorization.", Public, apikit.ScopeOpen, 204).lane(security),
		// contract: the handler writes the stored protected grant, which is a
		// device.TVGrant encoded when the grant was sealed.
		route[httpapi.DeviceCodeRequest, device.TVGrant]("POST", "/v1/tv-authorizations/token", "tv_authorization_token", "Poll a TV authorization for its sealed grant.", Public, apikit.ScopeOpen, 200).lane(security),

		// Account.
		route[NoBody, postgres.AccountDocument]("GET", "/v1/account", "account_get", "Read the account document.", AccountSession, apikit.ScopeSession, 200),
		route[httpapi.DisplayNameRequest, NoBody]("POST", "/v1/account/name", "account_name_set", "Rename the account.", AccountSession, apikit.ScopeSession, 204),
		route[NoBody, postgres.AccountDeviceList]("GET", "/v1/account/devices", "account_devices_list", "List the account's devices.", AccountSession, apikit.ScopeSession, 200),
		route[NoBody, identity.SecurityState]("GET", "/v1/account/security", "account_security_get", "Read the account's sign-in methods.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[httpapi.SecurityProofRequest, identity.RecentProof]("POST", "/v1/account/security/proof", "account_security_proof", "Prove the password for a security action.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[postgres.SecurityAction, identity.SecurityResult]("POST", "/v1/account/security/actions", "account_security_action", "Perform a proven security action.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[postgres.SecurityAction, identity.SecurityResult]("POST", "/v1/account/password", "account_password_change", "Change the password with a proven security action.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[httpapi.SecurityConfirmRequest, identity.SecurityResult]("POST", "/v1/account/security/mfa/confirm", "account_security_mfa_confirm", "Confirm an MFA enrollment.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[httpapi.SecurityConfirmRequest, identity.SecurityResult]("POST", "/v1/account/security/email/confirm", "account_security_email_confirm", "Confirm an email address change.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[httpapi.SecurityEmailResendRequest, identity.SecurityResult]("POST", "/v1/account/security/email/resend", "account_security_email_resend", "Resend the initiating family's pending email change code without extending its expiry.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[postgres.SecurityAction, identity.DeletionPreview]("POST", "/v1/account/deletion/preview", "account_deletion_preview", "Preview account deletion with a proven security action.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[httpapi.AccountDeletionRequest, identity.DeletionReceipt]("POST", "/v1/account/deletion", "account_deletion_accept", "Delete the account with a previewed disposition.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[httpapi.DeletionTokenRequest, identity.DeletionReceipt]("POST", "/v1/account/deletion/status", "account_deletion_status", "Read an account deletion receipt.", Public, apikit.ScopeOpen, 200).lane(security),
		route[postgres.ExportRequest, postgres.ExportJob]("POST", "/v1/account/export", "account_export_request", "Request an account data export.", AccountSession, apikit.ScopeSession, 202),
		route[NoBody, postgres.ExportJob]("GET", "/v1/account/exports/{export}", "account_export_get", "Read an account data export's state.", AccountSession, apikit.ScopeSession, 200),
		// contract: the handler writes the export's stored bytes, which are an
		// encoded postgres.AccountExportDocument.
		route[NoBody, postgres.AccountExportDocument]("POST", "/v1/account/exports/{export}/download", "account_export_download", "Download a ready account data export.", AccountSession, apikit.ScopeSession, 200),

		// Account notifications.
		route[NoBody, postgres.NotificationFeed]("GET", "/v1/account/notifications", "account_notifications_list", "List the account's in-app notifications.", AccountSession, apikit.ScopeSession, 200).paged("cursor"),
		route[NoBody, postgres.Notification]("GET", "/v1/account/notifications/{id}", "account_notification_get", "Read one in-app notification.", AccountSession, apikit.ScopeSession, 200),
		route[httpapi.NotificationReadRequest, httpapi.CodeResponse]("POST", "/v1/account/notifications/read", "account_notifications_read", "Mark in-app notifications read.", AccountSession, apikit.ScopeSession, 200),
		route[NoBody, postgres.NotificationPreferences]("GET", "/v1/account/notification-preferences", "account_notification_preferences_get", "Read the account's notification preferences.", AccountSession, apikit.ScopeSession, 200),
		route[postgres.NotificationPreferences, postgres.NotificationPreferences]("PUT", "/v1/account/notification-preferences", "account_notification_preferences_set", "Change the account's notification preferences.", AccountSession, apikit.ScopeSession, 200),

		// Servers, as the signed-in account sees them.
		// contract: sends an ETag with the list version and answers 304 to a matching If-None-Match.
		route[NoBody, httpapi.ServerList]("GET", "/v1/servers", "servers_list", "List the account's servers.", AccountSession, apikit.ScopeSession, 200).query("include"),
		route[httpapi.IdentityRequest, directory.SignedPolicy]("POST", "/v1/servers/{id}/identity", "server_identity_issue", "Issue a short identity assertion for one server, answering its challenge.", AccountSession, apikit.ScopeSession, 200),
		route[NoBody, httpapi.CodeResponse]("POST", "/v1/servers/{id}/leave", "server_leave", "Remove a server from the account's list; the server ends the membership.", AccountSession, apikit.ScopeSession, 200),
		// contract: 304 when {version} is current, else 200 with the current one. Answered from memory.
		route[NoBody, httpapi.ServerListVersion]("GET", "/v1/server-list/{watch}/{version}", "server_list_version", "Check whether the account's server list changed.", Public, apikit.ScopeOpen, 200),
		route[NoBody, directory.SignedPolicy]("GET", "/v1/servers/{id}/routes", "server_routes_get", "Read a server's signed routes.", AccountSession, apikit.ScopeSession, 200),
		route[postgres.ServerRemoval, httpapi.StatusCodeResponse]("DELETE", "/v1/servers/{server}", "server_remove", "Retire an owned server.", AccountSession, apikit.ScopeSession, 200).lane(security),

		// Server-to-Hosted calls with the server's own credential.
		route[directory.CheckInReport, directory.Heartbeat]("POST", "/v1/servers/{id}/heartbeat", "server_heartbeat", "Check in: the wake answered and the member list's state.", ServerCredential, apikit.ScopeAdmin, 200),
		route[directory.MemberPush, directory.MemberPushResult]("POST", "/v1/servers/{id}/members", "server_members_push", "Report the server's Portico Account members for discovery.", ServerCredential, apikit.ScopeAdmin, 200).body(httpapi.MemberPushBody),
		route[postgres.ServerEvent, httpapi.CodeResponse]("POST", "/v1/servers/{id}/events", "server_event", "Report a server event for account notifications.", ServerCredential, apikit.ScopeAdmin, 200),
		route[directory.RouteObservation, directory.RouteObservationReceipt]("POST", "/v1/servers/{id}/routes", "server_routes_observe", "Report the server's observed routes.", ServerCredential, apikit.ScopeAdmin, 200),
		route[httpapi.RouteProofRequest, directory.EndpointReceipt]("POST", "/v1/servers/{id}/routes/prove", "server_routes_prove", "Register an endpoint for an observed route generation.", ServerCredential, apikit.ScopeAdmin, 200),
		route[directory.EndpointRequest, directory.EndpointReceipt]("POST", "/v1/servers/{id}/endpoint", "server_endpoint_register", "Register the server's endpoint.", ServerCredential, apikit.ScopeAdmin, 200),
		route[httpapi.DirectRouteRequest, directroute.Route]("POST", "/v1/servers/{id}/direct-route", "server_direct_route_issue", "Publish the server's direct address.", ServerCredential, apikit.ScopeAdmin, 200),
		route[httpapi.DirectRouteWithdrawRequest, httpapi.DirectRouteWithdrawal]("POST", "/v1/servers/{id}/direct-route/withdraw", "server_direct_route_withdraw", "Withdraw one address family.", ServerCredential, apikit.ScopeAdmin, 200),
		route[NoBody, certificates.Namespace]("POST", "/v1/servers/{id}/certificates/namespace", "server_certificate_namespace", "Read the server's certificate namespace.", ServerCredential, apikit.ScopeAdmin, 200),
		route[httpapi.CertificateLookupRequest, certificates.Status]("POST", "/v1/servers/{id}/certificates/orders/lookup", "server_certificate_order_lookup", "Find a certificate order by its request ID.", ServerCredential, apikit.ScopeAdmin, 200),
		route[httpapi.CertificateOrderRequest, certificates.Status]("POST", "/v1/servers/{id}/certificates/orders", "server_certificate_order_create", "Submit a certificate signing request.", ServerCredential, apikit.ScopeAdmin, 202),
		route[NoBody, certificates.Status]("GET", "/v1/servers/{id}/certificates/orders/{order}", "server_certificate_order_get", "Read a certificate order.", ServerCredential, apikit.ScopeAdmin, 200),
		route[NoBody, httpapi.CertificateChain]("GET", "/v1/servers/{id}/certificates/orders/{order}/chain", "server_certificate_chain_get", "Read an issued certificate chain.", ServerCredential, apikit.ScopeAdmin, 200),
		route[NoBody, httpapi.CancelRequestedResponse]("POST", "/v1/servers/{id}/certificates/orders/{order}/cancel", "server_certificate_order_cancel", "Cancel a certificate order.", ServerCredential, apikit.ScopeAdmin, 200),

		// DNS feed and provider notifications.
		route[NoBody, httpapi.AddressChangeList]("GET", "/v1/dns/addresses", "dns_addresses_list", "Read address changes after a sequence.", DNSFeed, apikit.ScopeRead, 200).paged("after", "limit"),
		// contract: the body is read raw (not decode) up to appstore.MaxPayloadBytes,
		// and refusals are also answered 200 with accepted false.
		route[httpapi.AppStoreNotificationRequest, httpapi.AppStoreReceipt]("POST", "/v1/appstore/notifications", "appstore_notification", "Receive an App Store Server Notification.", ProviderNotification, apikit.ScopeSession, 200).body(appstore.MaxPayloadBytes),

		// Server claim protocol (internal/httpapi ClaimHandler).
		route[claims.ApprovalInput, claims.Envelope]("POST", "/v1/server-claims/approve", "server_claim_approve", "Approve a server claim.", AccountSession, apikit.ScopeSession, 200).lane(security).body(ClaimBody),
		route[claims.Commit, claims.Commit]("POST", "/v1/server-claims/ack", "server_claim_ack", "Acknowledge a committed server claim.", ServerCredential, apikit.ScopeAdmin, 200).lane(security).body(ClaimBody),
		route[httpapi.ClaimSignedRequest, NoBody]("POST", "/v1/servers/leave", "server_registration_leave", "Withdraw an exact server registration after an offline local move.", SignedRequest, apikit.ScopeSession, 204).lane(security).body(ClaimBody),
		route[httpapi.ClaimSignedRequest, claims.Challenge]("POST", "/v1/server-claims/nonce", "server_claim_nonce", "Request a server claim nonce.", SignedRequest, apikit.ScopeSession, 200).lane(security).body(ClaimBody),
		route[httpapi.ClaimSignedRequest, claims.Commit]("POST", "/v1/server-claims/finalize", "server_claim_finalize", "Finalize a server claim.", SignedRequest, apikit.ScopeSession, 200).lane(security).body(ClaimBody),
		route[httpapi.ClaimSignedRequest, claims.ResultValue]("POST", "/v1/server-claims/result", "server_claim_result", "Read a server claim's result.", SignedRequest, apikit.ScopeSession, 200).lane(security).body(ClaimBody),
		route[httpapi.ClaimSignedRequest, claims.CancellationResult]("POST", "/v1/server-claims/cancel", "server_claim_cancel", "Cancel a server claim.", SignedRequest, apikit.ScopeSession, 200).lane(security).body(ClaimBody),
		route[claimweb.PendingRequest, claimweb.Pending]("POST", "/v1/server-claims/pending", "server_claim_pending", "Register a web server claim and any bound owner-start proof for approval.", SignedRequest, apikit.ScopeSession, 200).lane(security),
		// contract: a long poll that writes whitespace heartbeats before this object.
		route[claimweb.Signed, httpapi.ClaimEventResult]("POST", "/v1/server-claims/events", "server_claim_events", "Wait for a web server claim's decision.", SignedRequest, apikit.ScopeSession, 200).lane(apikit.Realtime),
		route[httpapi.ClaimCodeRequest, claimweb.LookupPending]("POST", "/v1/server-claims/lookup", "server_claim_lookup", "Look up a web server claim by its code.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[httpapi.ClaimCodeRequest, httpapi.StatusCodeResponse]("POST", "/v1/server-claims/deny", "server_claim_deny", "Deny a web server claim.", AccountSession, apikit.ScopeSession, 200).lane(security),
		route[httpapi.ClaimCodeRequest, httpapi.StatusCodeResponse]("POST", "/v1/server-claims/approve-code", "server_claim_approve_code", "Approve a web server claim by its code.", AccountSession, apikit.ScopeSession, 200).lane(security),
	}
	out := make([]apikit.Definition, len(entries))
	for i, e := range entries {
		out[i] = apikit.Definition(e)
	}
	return out
}
