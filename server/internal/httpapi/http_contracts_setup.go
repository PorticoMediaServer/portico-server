package httpapi

import (
	"portico.local/apikit"
	identity "portico.local/server/internal/identity"
	networking "portico.local/server/internal/networking"
)

func registerSetupHTTPContracts(registry *apikit.Registry) error {
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct{}, struct {
		Alive bool `json:"alive"`
	}](registry, apikit.Metadata{ID: "get_lth_live", Method: "GET", Path: "/health/live", Summary: "Get live", Access: apikit.Public, Requires: apikit.ScopeOpen, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:32
	if err := apikit.RegisterHTTP[struct{}, struct {
		Ready bool `json:"ready"`
	}](registry, apikit.Metadata{ID: "get_lth_ready", Method: "GET", Path: "/health/ready", Summary: "Get ready", Access: apikit.Public, Requires: apikit.ScopeOpen, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/onboarding.go:47
	if err := apikit.RegisterHTTP[struct {
		RequestID  string "json:\"requestId\""
		SetupToken string "json:\"setupToken\""
	}, identity.Envelope](registry, apikit.Metadata{ID: "post_setup_resume", Method: "POST", Path: "/v1/setup/resume", Summary: "Post resume", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/onboarding.go:68
	if err := apikit.RegisterHTTP[struct{}, identity.OnboardingState](registry, apikit.Metadata{ID: "get_setup_state", Method: "GET", Path: "/v1/setup/state", Summary: "Get state", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}

	// internal/httpapi/onboarding.go:84
	if err := apikit.RegisterHTTP[struct {
		Revision         int64 "json:\"revision,string\""
		Enabled          bool  "json:\"enabled\""
		WarningConfirmed bool  "json:\"warningConfirmed\""
	}, identity.OnboardingState](registry, apikit.Metadata{ID: "post_quick_connect", Method: "POST", Path: "/v1/quick-connect", Summary: "Post quick connect", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/onboarding.go:84
	if err := apikit.RegisterHTTP[struct {
		Revision         int64 "json:\"revision,string\""
		Enabled          bool  "json:\"enabled\""
		WarningConfirmed bool  "json:\"warningConfirmed\""
	}, identity.OnboardingState](registry, apikit.Metadata{ID: "post_quick_connect_review", Method: "POST", Path: "/v1/quick-connect/review", Summary: "Post review", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/onboarding.go:84
	if err := apikit.RegisterHTTP[struct {
		Revision         int64 "json:\"revision,string\""
		Enabled          bool  "json:\"enabled\""
		WarningConfirmed bool  "json:\"warningConfirmed\""
	}, identity.OnboardingState](registry, apikit.Metadata{ID: "post_quick_connect_decision", Method: "POST", Path: "/v1/quick-connect/decision", Summary: "Post decision", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}

	// internal/httpapi/topshelf.go:69
	if err := apikit.RegisterHTTP[struct {
		InstallationID string "json:\"installationId\""
	}, TopShelfToken](registry, apikit.Metadata{ID: "post_topshelf_token", Method: "POST", Path: "/v1/topshelf/token", Summary: "Post token", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.Constant, BodyLimit: 65536, Status: 201, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/topshelf.go:90
	if err := apikit.RegisterHTTP[struct{}, TopShelfFeed](registry, apikit.Metadata{ID: "get_topshelf", Method: "GET", Path: "/v1/topshelf", Summary: "Get topshelf", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Browsing, Admission: laneBrowsing, Cost: apikit.PageSized, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/topshelf.go:136
	if err := apikit.RegisterHTTP[struct{}, string](registry, apikit.Metadata{ID: "get_topshelf_art_id", Method: "GET", Path: "/v1/topshelf/art/{id}", Summary: "Get id", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Media, Admission: laneMedia, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"grant"}, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		Revision         int64 `json:"revision,string"`
		Enabled          bool  `json:"enabled"`
		WarningConfirmed bool  `json:"warningConfirmed"`
	}, identity.OnboardingState](registry, apikit.Metadata{ID: "post_setup_finish", Method: "POST", Path: "/v1/setup/finish", Summary: "Post finish", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		Revision         int64 `json:"revision,string"`
		Enabled          bool  `json:"enabled"`
		WarningConfirmed bool  `json:"warningConfirmed"`
	}, identity.OnboardingState](registry, apikit.Metadata{ID: "post_setup_remote_recovery", Method: "POST", Path: "/v1/setup/remote-recovery", Summary: "Post remote recovery", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		DeviceCode string `json:"deviceCode"`
	}, identity.Envelope](registry, apikit.Metadata{ID: "post_quick_connect_token", Method: "POST", Path: "/v1/quick-connect/token", Summary: "Post token", Access: apikit.Public, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		DeviceCode string `json:"deviceCode"`
	}, struct{}](registry, apikit.Metadata{ID: "post_quick_connect_cancel", Method: "POST", Path: "/v1/quick-connect/cancel", Summary: "Post cancel", Access: apikit.Public, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct{}, networking.CertificateStatus](registry, apikit.Metadata{ID: "get_networking_certificate", Method: "GET", Path: "/v1/networking/certificate", Summary: "Get certificate", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		AuthorityID string                       `json:"authorityId"`
		Config      networking.CertificateConfig `json:"config"`
	}, networking.CertificateStatus](registry, apikit.Metadata{ID: "post_networking_certificate_config", Method: "POST", Path: "/v1/networking/certificate/config", Summary: "Post config", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct{}, networking.CertificateStatus](registry, apikit.Metadata{ID: "post_networking_certificate_retry", Method: "POST", Path: "/v1/networking/certificate/retry", Summary: "Post retry", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct{}, networking.ClaimStatus](registry, apikit.Metadata{ID: "get_networking_claim", Method: "GET", Path: "/v1/networking/claim", Summary: "Get claim", Access: apikit.Owner, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		Expected networking.ClaimExpected `json:"expected"`
	}, networking.ClaimStatus](registry, apikit.Metadata{ID: "post_networking_claim_await_approval", Method: "POST", Path: "/v1/networking/claim/await-approval", Summary: "Post await approval", Access: apikit.Owner, Requires: apikit.ScopeSession, Lane: apikit.Realtime, Admission: laneRealtime, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		Expected networking.ClaimExpected `json:"expected"`
		BaseURL  string                   `json:"baseUrl"`
	}, networking.EndpointReceipt](registry, apikit.Metadata{ID: "post_networking_claim_endpoint", Method: "POST", Path: "/v1/networking/claim/endpoint", Summary: "Post endpoint", Access: apikit.Owner, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		Expected         networking.ClaimExpected `json:"expected"`
		ApprovalEnvelope string                   `json:"approvalEnvelope,omitempty"`
	}, networking.ClaimStatus](registry, apikit.Metadata{ID: "post_networking_claim_web_approval", Method: "POST", Path: "/v1/networking/claim/web-approval", Summary: "Post web approval", Access: apikit.Owner, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		Kind                 string `json:"kind"`
		ServerID             string `json:"serverId"`
		AccountID            string `json:"accountId"`
		OperationID          string `json:"operationId"`
		ClaimGeneration      string `json:"claimGeneration"`
		CredentialGeneration string `json:"credentialGeneration"`
		BaseURL              string `json:"baseUrl"`
		RegistrationID       string `json:"registrationId"`
		Nonce                string `json:"nonce"`
		ExpiresAt            string `json:"expiresAt"`
	}, struct {
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}](registry, apikit.Metadata{ID: "post_networking_endpoint_proof", Method: "POST", Path: "/v1/networking/endpoint-proof", Summary: "Post endpoint proof", Access: apikit.Public, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct{}, networking.RemoteStatus](registry, apikit.Metadata{ID: "get_networking_remote", Method: "GET", Path: "/v1/networking/remote", Summary: "Get remote", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct{}, networking.RemoteStatus](registry, apikit.Metadata{ID: "post_networking_remote_check", Method: "POST", Path: "/v1/networking/remote/check", Summary: "Post check", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		AuthorityID string                  `json:"authorityId"`
		Config      networking.RemoteConfig `json:"config"`
	}, networking.RemoteStatus](registry, apikit.Metadata{ID: "post_networking_remote_config", Method: "POST", Path: "/v1/networking/remote/config", Summary: "Post config", Access: apikit.Owner, Requires: apikit.ScopeAdmin, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		AccountID string                           `json:"accountId,omitempty"`
		Expected  networking.ClaimIdentityExpected `json:"expected"`
	}, networking.ClaimStatus](registry, apikit.Metadata{ID: "post_networking_claim_prepare", Method: "POST", Path: "/v1/networking/claim/prepare", Summary: "Post prepare", Access: apikit.Owner, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		Expected         networking.ClaimExpected `json:"expected"`
		ApprovalEnvelope string                   `json:"approvalEnvelope,omitempty"`
	}, networking.ClaimStatus](registry, apikit.Metadata{ID: "post_networking_claim_approve", Method: "POST", Path: "/v1/networking/claim/approve", Summary: "Post approve", Access: apikit.Owner, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		Expected         networking.ClaimExpected `json:"expected"`
		ApprovalEnvelope string                   `json:"approvalEnvelope,omitempty"`
	}, networking.ClaimStatus](registry, apikit.Metadata{ID: "post_networking_claim_continue", Method: "POST", Path: "/v1/networking/claim/continue", Summary: "Post continue", Access: apikit.Owner, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/installation.go:28
	if err := apikit.RegisterHTTP[struct {
		Expected         networking.ClaimExpected `json:"expected"`
		ApprovalEnvelope string                   `json:"approvalEnvelope,omitempty"`
	}, networking.ClaimStatus](registry, apikit.Metadata{ID: "post_networking_claim_cancel", Method: "POST", Path: "/v1/networking/claim/cancel", Summary: "Post cancel", Access: apikit.Owner, Requires: apikit.ScopeSession, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	return nil
}
