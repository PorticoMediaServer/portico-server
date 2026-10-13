package httpapi

import (
	"portico.local/apikit"
	hosted "portico.local/server/internal/hosted"
	identity "portico.local/server/internal/identity"
)

func registerIdentityHTTPContracts(registry *apikit.Registry) error {
	// internal/httpapi/direct_identity.go:30
	if err := apikit.RegisterHTTP[struct {
		Nonce string "json:\"nonce\""
	}, struct {
		Challenge string            `json:"challenge"`
		ExpiresAt string            `json:"expiresAt"`
		Proof     map[string]string `json:"proof"`
	}](registry, apikit.Metadata{ID: "post_direct_portico_challenge", Method: "POST", Path: "/v1/direct/portico-challenge", Summary: "Post portico challenge", Access: apikit.Public, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Query: []string{"X-Portico-Installation-Id"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:66
	if err := apikit.RegisterHTTP[struct {
		Username        string         "json:\"username\""
		Password        string         "json:\"password\""
		PorticoIdentity *hosted.Signed "json:\"porticoIdentity,omitempty\""
	}, identity.DirectSignIn](registry, apikit.Metadata{ID: "post_direct_sign_in", Method: "POST", Path: "/v1/direct/sign-in", Summary: "Post sign in", Access: apikit.Public, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Query: []string{"X-Portico-Installation-Id"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:106
	if err := apikit.RegisterHTTP[struct {
		Name string "json:\"name\""
		Art  string "json:\"art\""
	}, identity.DirectSnapshot](registry, apikit.Metadata{ID: "post_direct_profiles", Method: "POST", Path: "/v1/direct/profiles", Summary: "Post profiles", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 201, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:122
	if err := apikit.RegisterHTTP[identity.DirectProfileEdit, identity.DirectSnapshot](registry, apikit.Metadata{ID: "patch_direct_profiles_id", Method: "PATCH", Path: "/v1/direct/profiles/{id}", Summary: "Patch id", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:135
	if err := apikit.RegisterHTTP[struct {
		Profiles []identity.ProfileOrderEntry "json:\"profiles\""
	}, identity.DirectSnapshot](registry, apikit.Metadata{ID: "put_direct_profiles_order", Method: "PUT", Path: "/v1/direct/profiles/order", Summary: "Put order", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:150
	if err := apikit.RegisterHTTP[identity.DirectSelection, identity.DirectSelectionResult](registry, apikit.Metadata{ID: "post_direct_profiles_id_select", Method: "POST", Path: "/v1/direct/profiles/{id}/select", Summary: "Post select", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:163
	if err := apikit.RegisterHTTP[struct{}, identity.ProfileDeletionPreview](registry, apikit.Metadata{ID: "get_direct_profiles_id_deletion_preview", Method: "GET", Path: "/v1/direct/profiles/{id}/deletion-preview", Summary: "Get deletion preview", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:171
	if err := apikit.RegisterHTTP[struct {
		ExpectedRevision     int64 "json:\"expectedRevision\""
		DeleteSavedResources bool  "json:\"deleteSavedResources\""
	}, struct{}](registry, apikit.Metadata{ID: "delete_direct_profiles_id", Method: "DELETE", Path: "/v1/direct/profiles/{id}", Summary: "Delete id", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:186
	if err := apikit.RegisterHTTP[struct {
		ProfileID      string "json:\"profileId\""
		InstallationID string "json:\"installationId\""
	}, struct{}](registry, apikit.Metadata{ID: "delete_direct_trust", Method: "DELETE", Path: "/v1/direct/trust", Summary: "Delete trust", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneSecurityFence, Cost: apikit.Constant, BodyLimit: 65536, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:201
	if err := apikit.RegisterHTTP[struct {
		CurrentPassword string "json:\"currentPassword\""
		NewPassword     string "json:\"newPassword\""
		Code            string "json:\"code\""
	}, struct{}](registry, apikit.Metadata{ID: "post_direct_password", Method: "POST", Path: "/v1/direct/password", Summary: "Post password", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:217
	if err := apikit.RegisterHTTP[struct{}, struct {
		Items []identity.DirectAccount `json:"items"`
	}](registry, apikit.Metadata{ID: "get_direct_members", Method: "GET", Path: "/v1/direct/members", Summary: "Get members", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:225
	if err := apikit.RegisterHTTP[identity.DirectMemberInput, identity.DirectAccount](registry, apikit.Metadata{ID: "post_direct_members", Method: "POST", Path: "/v1/direct/members", Summary: "Post members", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 201, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:238
	if err := apikit.RegisterHTTP[identity.DirectMemberInput, struct{}](registry, apikit.Metadata{ID: "patch_direct_members_id", Method: "PATCH", Path: "/v1/direct/members/{id}", Summary: "Patch id", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:253
	if err := apikit.RegisterHTTP[struct {
		PorticoIdentity *hosted.Signed "json:\"porticoIdentity\""
	}, struct{}](registry, apikit.Metadata{ID: "post_direct_ownership_custody", Method: "POST", Path: "/v1/direct/ownership/custody", Summary: "Post custody", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 204, Query: []string{"X-Portico-Installation-Id"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/direct_identity.go:275
	if err := apikit.RegisterHTTP[struct {
		AccountID        string         "json:\"accountId\""
		CurrentPassword  string         "json:\"currentPassword\""
		ExpectedRevision int64          "json:\"expectedRevision\""
		PorticoIdentity  *hosted.Signed "json:\"porticoIdentity,omitempty\""
	}, struct{}](registry, apikit.Metadata{ID: "post_direct_ownership", Method: "POST", Path: "/v1/direct/ownership", Summary: "Post ownership", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 204, Query: []string{"X-Portico-Installation-Id"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:35
	if err := apikit.RegisterHTTP[struct{}, identity.ProfileAvatar](registry, apikit.Metadata{ID: "post_direct_profiles_id_avatar", Method: "POST", Path: "/v1/direct/profiles/{id}/avatar", Summary: "Post avatar", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 6291456, Status: 201, Query: []string{"Connection"}, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:72
	if err := apikit.RegisterHTTP[struct{}, struct{}](registry, apikit.Metadata{ID: "delete_direct_profiles_id_avatar", Method: "DELETE", Path: "/v1/direct/profiles/{id}/avatar", Summary: "Delete avatar", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 0, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:79
	if err := apikit.RegisterHTTP[struct{}, struct{}](registry, apikit.Metadata{ID: "get_profiles_id_avatar", Method: "GET", Path: "/v1/profiles/{id}/avatar", Summary: "Get avatar", Access: apikit.Device, Requires: apikit.ScopeRead, Lane: apikit.Default, Admission: laneDefault, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Query: []string{"v"}, ResponseContentType: "application/octet-stream", Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:155
	if err := apikit.RegisterHTTP[struct {
		InstallationID    string "json:\"installationId\""
		OutgoingProfileID string "json:\"outgoingProfileId\""
	}, struct{}](registry, apikit.Metadata{ID: "post_direct_profile_switch", Method: "POST", Path: "/v1/direct/profile-switch", Summary: "Post profile switch", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:170
	if err := apikit.RegisterHTTP[struct {
		InstallationID  string "json:\"installationId\""
		AutomaticSignIn bool   "json:\"automaticSignIn\""
	}, struct {
		Items []identity.BrowserAccount `json:"items"`
	}](registry, apikit.Metadata{ID: "post_direct_remembered_accounts", Method: "POST", Path: "/v1/direct/remembered-accounts", Summary: "Post remembered accounts", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:189
	if err := apikit.RegisterHTTP[struct {
		RefreshToken   string "json:\"refreshToken\""
		InstallationID string "json:\"installationId\""
		RequestID      string "json:\"requestId\""
	}, identity.Envelope](registry, apikit.Metadata{ID: "post_auth_refresh", Method: "POST", Path: "/v1/auth/refresh", Summary: "Post refresh", Access: apikit.Public, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:211
	if err := apikit.RegisterHTTP[identity.SelfRegistrationRequest, identity.DirectSignIn](registry, apikit.Metadata{ID: "post_auth_register", Method: "POST", Path: "/v1/auth/register", Summary: "Post register", Access: apikit.Public, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneAuth, Cost: apikit.Constant, BodyLimit: 65536, Status: 201, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:225
	if err := apikit.RegisterHTTP[struct {
		Token string "json:\"token\""
		Code  string "json:\"code\""
	}, identity.DirectSignIn](registry, apikit.Metadata{ID: "post_auth_two_factor_challenge", Method: "POST", Path: "/v1/auth/two-factor/challenge", Summary: "Post challenge", Access: apikit.Public, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneSecurityFence, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:242
	if err := apikit.RegisterHTTP[struct{}, identity.TwoFactorState](registry, apikit.Metadata{ID: "get_direct_two_factor", Method: "GET", Path: "/v1/direct/two-factor", Summary: "Get two factor", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneSecurityFence, Cost: apikit.Constant, BodyLimit: 0, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:250
	if err := apikit.RegisterHTTP[struct {
		Password string "json:\"password\""
	}, identity.TwoFactorEnrolment](registry, apikit.Metadata{ID: "post_direct_two_factor_enrol", Method: "POST", Path: "/v1/direct/two-factor/enrol", Summary: "Post enrol", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneSecurityFence, Cost: apikit.Constant, BodyLimit: 65536, Status: 201, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:266
	if err := apikit.RegisterHTTP[struct {
		Password string "json:\"password\""
		Code     string "json:\"code\""
	}, identity.TwoFactorState](registry, apikit.Metadata{ID: "post_direct_two_factor_verify", Method: "POST", Path: "/v1/direct/two-factor/verify", Summary: "Post verify", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneSecurityFence, Cost: apikit.Constant, BodyLimit: 65536, Status: 200, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	// internal/httpapi/identity_routes.go:282
	if err := apikit.RegisterHTTP[struct {
		Password string "json:\"password\""
		Code     string "json:\"code\""
	}, struct{}](registry, apikit.Metadata{ID: "delete_direct_two_factor", Method: "DELETE", Path: "/v1/direct/two-factor", Summary: "Delete two factor", Access: apikit.Device, Requires: apikit.ScopeSession, Lane: apikit.Security, Admission: laneSecurityFence, Cost: apikit.Constant, BodyLimit: 65536, Status: 204, Errors: []string{"invalid_request", "unauthorized", "not_permitted", "not_found", "internal"}}); err != nil {
		return err
	}
	return nil
}
