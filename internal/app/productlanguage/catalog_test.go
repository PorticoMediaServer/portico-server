package productlanguage

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCatalogAndReferenceShareOneRevision(t *testing.T) {
	payload, ok := Catalog("en-US")
	if !ok {
		t.Fatal("expected en-US catalog")
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	reference := CanonicalReference()
	if decoded["revision"] != reference.Revision {
		t.Fatalf("revision mismatch: catalog=%v reference=%q", decoded["revision"], reference.Revision)
	}
	if reference.DefaultLocale != "en-US" || reference.IconFamily != "lucide" {
		t.Fatalf("unexpected reference: %+v", reference)
	}
	if !bytes.Contains(payload, []byte(`"problem.server-unavailable"`)) {
		t.Fatal("catalog is missing normalized server failure copy")
	}
}

func TestCatalogRejectsUnsupportedLocale(t *testing.T) {
	if payload, ok := Catalog("fr-CA"); ok || payload != nil {
		t.Fatal("unsupported locales must not silently return English under another locale")
	}
}

func TestBrowseAndDetailVocabularyIsPublished(t *testing.T) {
	required := []string{
		"action.load-more", "action.load-more-group",
		"library.control-sort", "library.select-value", "library.filter-group-logic",
		"library.sort-order-title", "library.refine-title", "library.save-title",
		"library.technical-table-label", "library.select-item", "library.deselect-item",
		"media.hierarchy-label", "media.episode-code", "media.track-list-label",
		"media.seasons-title", "media.episodes-title", "media.season-selector-label",
	}
	messages := knownCatalogMessages()
	for _, id := range required {
		if messages[id] == nil {
			t.Errorf("generated Product Language catalog is missing %q", id)
		}
	}
}

func TestGenericAndGroupSpecificLoadMoreCopyRemainDistinct(t *testing.T) {
	var messages map[string]struct {
		Text string `json:"text"`
	}
	payload, ok := Catalog("en-US")
	if !ok {
		t.Fatal("expected en-US catalog")
	}
	var catalog struct {
		Messages map[string]struct {
			Text string `json:"text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	messages = catalog.Messages
	if got := messages["action.load-more"].Text; got != "Load more" {
		t.Fatalf("generic load-more copy = %q, want %q", got, "Load more")
	}
	if got := messages["action.load-more-group"].Text; got != "More {group}" {
		t.Fatalf("group load-more copy = %q, want %q", got, "More {group}")
	}
}

func TestEveryLibraryChannelProblemResolvesToCatalogMessage(t *testing.T) {
	codes := []string{
		"library_channel_generation_in_progress", "library_channel_generation_timeout",
		"library_channel_invalid_request", "library_channel_logo_delete_failed",
		"library_channel_logo_in_use", "library_channel_logo_invalid",
		"library_channel_logo_not_found", "library_channel_logo_store_failed",
		"library_channel_no_applicable_defaults", "library_channel_no_playable_schedule",
		"library_channel_not_found", "library_channel_program_restricted",
		"library_channel_program_unavailable", "library_channel_request_failed",
		"library_channel_revision_conflict",
		"library_channel_template_exists",
	}
	for _, code := range codes {
		if messageID, ok := ProblemMessageID(code, 500); !ok || messageID == "" {
			t.Errorf("%s did not resolve: %q, %v", code, messageID, ok)
		}
	}
}

func TestReleaseCandidateAccountAndInboxProblemsResolveToCatalogMessages(t *testing.T) {
	expected := map[string]string{
		"account_delete_failed":               "account.delete-failed",
		"invalid_password":                    "account.delete-password-invalid",
		"interactive_session_required":        "auth.device-session-required",
		"invalid_feedback":                    "feedback.invalid",
		"notification_load_failed":            "notification.load-failed",
		"notification_create_failed":          "notification.send-failed",
		"notification_recipients_failed":      "notification.recipients-failed",
		"notification_update_failed":          "notification.receipt-failed",
		"owned_servers_require_action":        "account.delete-owned-servers",
		"primary_profile_pin_in_use":          "auth.primary-profile-pin-in-use",
		"profiles_managed_by_portico_account": "auth.profiles-managed-by-portico-account",
		"unsupported_hosted_api_version":      "auth.client-update-required",
		"unsupported_server_api_version":      "auth.server-update-required",
		"bad_credentials":                     "auth.invalid-credentials",
		"credentials_required":                "auth.invalid-credentials",
		"device_session_required":             "auth.device-session-required",
		"hosted_unavailable":                  "problem.cloud-unavailable",
		"rate_limited":                        "problem.rate-limited",
		"server_session_revoked":              "auth.session-expired",
	}
	for code, want := range expected {
		if got, ok := ProblemMessageID(code, 500); !ok || got != want {
			t.Errorf("%s resolved to %q, %v; want %q", code, got, ok, want)
		}
	}
}
