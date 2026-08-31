// Package productlanguage exposes the generated, platform-neutral Portico
// product wording and semantic icon catalog. The source of truth lives under
// api/product-language and is copied here by Client Core's generator so the Go
// server can publish the exact same bytes used by first-party clients.
package productlanguage

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed catalog.en-US.json
var englishCatalog []byte

type Reference struct {
	Revision         string   `json:"revision"`
	DefaultLocale    string   `json:"defaultLocale"`
	SupportedLocales []string `json:"supportedLocales"`
	EndpointTemplate string   `json:"endpointTemplate"`
	IconFamily       string   `json:"iconFamily"`
}

type catalogHeader struct {
	Revision   string                     `json:"revision"`
	Locale     string                     `json:"locale"`
	IconFamily string                     `json:"iconFamily"`
	Messages   map[string]json.RawMessage `json:"messages"`
}

var problemCodeMessages = map[string]string{
	"account_admin_required":                     "problem.forbidden",
	"account_delete_failed":                      "account.delete-failed",
	"account_update_failed":                      "account.save-failed",
	"automatic_profile_trust_expired":            "auth.automatic-profile-trust-required",
	"automatic_profile_trust_required":           "auth.automatic-profile-trust-required",
	"bad_credentials":                            "auth.invalid-credentials",
	"credentials_required":                       "auth.invalid-credentials",
	"device_session_required":                    "auth.device-session-required",
	"feedback_conflict":                          "feedback.conflict",
	"feedback_disabled":                          "feedback.disabled",
	"feedback_duplicate":                         "feedback.duplicate",
	"invalid_feedback":                           "feedback.invalid",
	"feedback_not_found":                         "problem.not-found",
	"feedback_rate_limited":                      "feedback.rate-limited",
	"feedback_response_failed":                   "feedback.response-failed",
	"invalid_credentials":                        "auth.invalid-credentials",
	"invalid_display_name":                       "account.invalid-display-name",
	"invalid_email_or_password":                  "auth.invalid-credentials",
	"interactive_session_required":               "auth.device-session-required",
	"invalid_password":                           "account.delete-password-invalid",
	"invalid_preferences":                        "preferences.invalid",
	"invalid_profile":                            "auth.invalid-profile",
	"invalid_profile_policy":                     "auth.invalid-profile-restrictions",
	"invalid_profile_restrictions":               "auth.invalid-profile-restrictions",
	"invalid_user_field":                         "problem.invalid-request",
	"mfa_invalid":                                "account.mfa-invalid",
	"mfa_required":                               "account.mfa-required",
	"local_profile_pin_invalid":                  "auth.local-profile-pin-invalid",
	"notification_not_found":                     "problem.not-found",
	"notification_create_failed":                 "notification.send-failed",
	"notification_load_failed":                   "notification.load-failed",
	"notification_recipients_failed":             "notification.recipients-failed",
	"notification_receipt_conflict":              "notification.receipt-conflict",
	"notification_update_failed":                 "notification.receipt-failed",
	"navigation_unavailable":                     "navigation.unavailable",
	"search_history_unavailable":                 "search.offline",
	"media_children_unavailable":                 "media.detail-unavailable",
	"owner_messaging_disabled":                   "feedback.disabled",
	"owned_servers_require_action":               "account.delete-owned-servers",
	"primary_profile_pin_required":               "auth.primary-profile-pin-required",
	"primary_profile_pin_in_use":                 "auth.primary-profile-pin-in-use",
	"primary_profile_required":                   "auth.primary-profile-required",
	"playback_failed":                            "playback.failed",
	"preference_conflict":                        "preferences.conflict",
	"preference_revision_conflict":               "preferences.conflict",
	"preferences_failed":                         "preferences.request-failed",
	"profile_admin_proof_expired":                "auth.profile-admin-step-up-expired",
	"profile_admin_proof_required":               "auth.profile-admin-step-up-required",
	"profile_admin_recovery_required":            "auth.profile-admin-recovery-required",
	"profile_admin_step_up_expired":              "auth.profile-admin-step-up-expired",
	"profile_admin_step_up_required":             "auth.profile-admin-step-up-required",
	"profile_conflict":                           "auth.profile-conflict",
	"profile_directory_failed":                   "problem.profile-request-failed",
	"profile_limit_reached":                      "auth.profile-limit-reached",
	"profile_not_available_on_server":            "auth.profile-not-available",
	"profile_not_found":                          "auth.profile-not-found",
	"profile_pin_invalid":                        "auth.profile-pin-invalid",
	"profile_pin_locked":                         "auth.profile-temporarily-locked",
	"profile_pin_required":                       "auth.profile-pin-required",
	"profile_pin_retry_later":                    "auth.profile-pin-retry-later",
	"profile_proof_failed":                       "problem.profile-request-failed",
	"profile_request_failed":                     "problem.profile-request-failed",
	"profile_selection_failed":                   "auth.profile-selection-failed",
	"profile_selection_required":                 "auth.profile-selection-required",
	"profile_temporarily_locked":                 "auth.profile-temporarily-locked",
	"profile_trust_failed":                       "auth.automatic-profile-trust-failed",
	"profiles_managed_by_portico_account":        "auth.profiles-managed-by-portico-account",
	"rate_limited":                               "problem.rate-limited",
	"recent_reauthentication_required":           "auth.session-expired",
	"hosted_unavailable":                         "problem.cloud-unavailable",
	"hosted_authority_stale":                     "problem.cloud-unavailable",
	"hosted_authority_clock_invalid":             "problem.server-unavailable",
	"csrf_failed":                                "problem.request-verification-failed",
	"server_unavailable":                         "problem.server-unavailable",
	"session_expired":                            "auth.session-expired",
	"server_session_revoked":                     "auth.session-expired",
	"unsupported_hosted_api_version":             "auth.client-update-required",
	"unsupported_server_api_version":             "auth.server-update-required",
	"invalid_product_contract_api_version":       "auth.server-update-required",
	"incompatible_hosted_api_version":            "auth.client-update-required",
	"incompatible_server_api_version":            "auth.server-update-required",
	"unsupported_preference_version":             "preferences.unsupported-version",
	"unsupported_preferences_version":            "preferences.unsupported-version",
	"library_channel_generation_in_progress":     "library-channel.generation-in-progress",
	"library_channel_generation_timeout":         "library-channel.generation-timeout",
	"library_channel_capacity_unavailable":       "library-channel.playback-capacity",
	"library_channel_segment_starting":           "library-channel.playback-capacity",
	"library_channel_playback_unavailable":       "library-channel.program-unavailable",
	"library_channel_logo_bug_overhead_required": "library-channel.logo-processing-overhead",
	"library_channel_playback_policy_stale":      "library-channel.revision-conflict",
	"library_channel_invalid_request":            "problem.invalid-request",
	"library_channel_logo_delete_failed":         "problem.request-failed",
	"library_channel_logo_in_use":                "problem.invalid-request",
	"library_channel_logo_invalid":               "problem.invalid-request",
	"library_channel_logo_not_found":             "problem.not-found",
	"library_channel_logo_store_failed":          "problem.request-failed",
	"library_channel_no_applicable_defaults":     "library-channel.no-applicable-defaults",
	"library_channel_no_playable_schedule":       "library-channel.generation-no-playable-schedule",
	"library_channel_not_found":                  "problem.not-found",
	"library_channel_program_restricted":         "library-channel.program-restricted",
	"library_channel_program_unavailable":        "library-channel.program-unavailable",
	"library_channel_request_failed":             "problem.request-failed",
	"library_channel_revision_conflict":          "library-channel.revision-conflict",
	"library_channel_template_exists":            "problem.invalid-request",
	"live_tv_tuner_capacity":                     "live-tv.channel-busy",
	"live_tv_tuner_allocation_failed":            "live-tv.offline",
	"dvr_schedule_conflict":                      "dvr.conflict",
	"dvr_running_playback_session_required":      "live-tv.channel-busy",
}

var (
	catalogMessagesOnce sync.Once
	catalogMessages     map[string]json.RawMessage
)

func knownCatalogMessages() map[string]json.RawMessage {
	catalogMessagesOnce.Do(func() {
		var header catalogHeader
		if err := json.Unmarshal(englishCatalog, &header); err != nil {
			panic(fmt.Errorf("decode generated Portico product language catalog: %w", err))
		}
		catalogMessages = header.Messages
	})
	return catalogMessages
}

// ProblemMessageID is the server-side half of the shared Product Language
// resolver. APIs emit this stable identifier and every client renders the same
// wording and semantic icon from the published catalog.
func ProblemMessageID(code string, status int) (string, bool) {
	messageID := problemCodeMessages[code]
	if messageID == "" {
		switch status {
		case 400, 405, 409, 410, 422:
			messageID = "problem.invalid-request"
		case 401:
			messageID = "auth.session-expired"
		case 403:
			messageID = "problem.forbidden"
		case 404:
			messageID = "problem.not-found"
		case 408, 504:
			messageID = "problem.timeout"
		case 429:
			messageID = "problem.rate-limited"
		case 502, 503:
			messageID = "problem.server-unavailable"
		default:
			messageID = "problem.request-failed"
		}
	}
	_, exists := knownCatalogMessages()[messageID]
	return messageID, exists
}

func Catalog(locale string) ([]byte, bool) {
	if locale != "en-US" {
		return nil, false
	}
	return append([]byte(nil), englishCatalog...), true
}

func CanonicalReference() Reference {
	var header catalogHeader
	if err := json.Unmarshal(englishCatalog, &header); err != nil {
		panic(fmt.Errorf("decode generated Portico product language catalog: %w", err))
	}
	return Reference{
		Revision:         header.Revision,
		DefaultLocale:    header.Locale,
		SupportedLocales: []string{header.Locale},
		EndpointTemplate: "/api/product-language/{locale}",
		IconFamily:       header.IconFamily,
	}
}
