package app

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/app/productlanguage"
)

func TestProductLanguageIsPublicVersionedAndConsistentWithProductContract(t *testing.T) {
	serverURL, _ := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	var catalog struct {
		Revision   string                     `json:"revision"`
		Locale     string                     `json:"locale"`
		IconFamily string                     `json:"iconFamily"`
		Icons      map[string]json.RawMessage `json:"icons"`
		Messages   map[string]json.RawMessage `json:"messages"`
	}
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/product-language/en-US", nil, &catalog)
	if status != http.StatusOK {
		t.Fatalf("product language status=%d body=%s", status, body)
	}
	if catalog.Revision == "" || catalog.Locale != "en-US" || catalog.IconFamily != "lucide" {
		t.Fatalf("unexpected catalog identity: %#v", catalog)
	}
	if catalog.Icons["status.server"] == nil || catalog.Messages["problem.server-unavailable"] == nil {
		t.Fatal("catalog does not contain canonical server failure presentation")
	}

	loginUser(t, client, serverURL)
	var contract CanonicalProductContract
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/product-contract", nil, &contract)
	if status != http.StatusOK {
		t.Fatalf("product contract status=%d body=%s", status, body)
	}
	if contract.Language.Revision != catalog.Revision || contract.Language.DefaultLocale != catalog.Locale || contract.Language.IconFamily != catalog.IconFamily {
		t.Fatalf("product language reference drifted: reference=%#v catalog=%#v", contract.Language, catalog)
	}
}

func TestProductLanguageRejectsUnsupportedLocales(t *testing.T) {
	serverURL, _ := newAuthTestServerWithDB(t)
	status, body := doJSON(t, &http.Client{}, http.MethodGet, serverURL+"/api/product-language/fr-CA", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unsupported locale status=%d body=%s", status, body)
	}
}

func TestMediaActionPresentationUsesCanonicalLanguageAndPlatformSurfaces(t *testing.T) {
	payload, ok := productlanguage.Catalog("en-US")
	if !ok {
		t.Fatal("expected canonical English product language catalog")
	}
	var catalog struct {
		Icons    map[string]json.RawMessage `json:"icons"`
		Messages map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatalf("decode product language catalog: %v", err)
	}

	for _, action := range canonicalMediaActionCapabilities() {
		presentation := action.Presentation
		if catalog.Messages[presentation.LabelMessageID] == nil {
			t.Errorf("action %q references unknown label message %q", action.ID, presentation.LabelMessageID)
		}
		if catalog.Icons[presentation.IconID] == nil {
			t.Errorf("action %q references unknown icon %q", action.ID, presentation.IconID)
		}
		surfaces := stringSet(presentation.Surfaces)
		if presentation.Group == "administration" {
			if len(surfaces) != 1 || !surfaces["web-admin"] {
				t.Errorf("administrative action %q leaked onto a consumer surface: %#v", action.ID, presentation.Surfaces)
			}
		} else if surfaces["web-admin"] {
			t.Errorf("consumer action %q was assigned to the web administration surface", action.ID)
		}
		if action.ID == mediaActionDownload && (len(surfaces) != 1 || !surfaces["mobile"]) {
			t.Errorf("download action must be mobile-only: %#v", presentation.Surfaces)
		}
	}
}

func TestLiveTVDVRAndLibraryChannelProblemCodesUseCanonicalLanguage(t *testing.T) {
	expected := map[string]string{
		"dvr_schedule_conflict":                      "dvr.conflict",
		"dvr_running_playback_session_required":      "live-tv.channel-busy",
		"live_tv_tuner_capacity":                     "live-tv.channel-busy",
		"live_tv_tuner_allocation_failed":            "live-tv.offline",
		"library_channel_capacity_unavailable":       "library-channel.playback-capacity",
		"library_channel_segment_starting":           "library-channel.playback-capacity",
		"library_channel_playback_unavailable":       "library-channel.program-unavailable",
		"library_channel_logo_bug_overhead_required": "library-channel.logo-processing-overhead",
		"library_channel_playback_policy_stale":      "library-channel.revision-conflict",
	}
	for code, want := range expected {
		got, ok := productlanguage.ProblemMessageID(code, http.StatusBadRequest)
		if !ok || got != want {
			t.Errorf("problem %s messageId=%q ok=%v, want %q", code, got, ok, want)
		}
	}
	recorder := httptest.NewRecorder()
	writeError(recorder, http.StatusBadRequest, "unmapped_release_candidate_problem", "raw diagnostic detail")
	var problem struct {
		MessageID string `json:"messageId"`
		Detail    string `json:"detail"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.MessageID != "problem.invalid-request" {
		t.Fatalf("fallback messageId=%q detail=%q", problem.MessageID, problem.Detail)
	}
}
