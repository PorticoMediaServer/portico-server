package app

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func stoppedPlaybackRequest(playback PlaybackResponse) PlaybackSessionStopRequest {
	return PlaybackSessionStopRequest{
		Disposition: "stopped", Generation: int64(playback.Generation), EventSequence: 1_000_000,
		RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), PositionSeconds: 0,
		DurationSeconds: float64(playback.Timeline.DurationSeconds),
	}
}

func watchWithFriendsRevisionPtr(revision int64) *int64 { return &revision }

func TestDeviceRevocationDeletesSessions(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var devices ListResponse[Device]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/devices", nil, &devices)
	if status != http.StatusOK {
		t.Fatalf("devices status = %d, body: %s", status, body)
	}
	if len(devices.Items) == 0 {
		t.Fatalf("expected at least one tracked device")
	}
	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/devices/"+devices.Items[0].ID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("revoke status = %d, body: %s", status, body)
	}
	assertAuthenticated(t, client, serverURL, false)
}

func TestLibraryTypesDoNotSupportPhotos(t *testing.T) {
	if validLibraryStorageKind("photo") || validLibraryStorageKind("photos") {
		t.Fatal("photo library type should not resolve")
	}

	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var contract CanonicalProductContract
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/product-contract", nil, &contract)
	if status != http.StatusOK {
		t.Fatalf("product contract status = %d, body: %s", status, body)
	}
	for _, libraryKind := range contract.LibraryKinds {
		if libraryKind.ID == "photo" || libraryKind.ID == "photos" {
			t.Fatalf("product contract advertised photos: %#v", contract.LibraryKinds)
		}
	}

	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/libraries", CreateLibraryRequest{
		Name:  "Photos",
		Type:  "photo",
		Paths: []string{t.TempDir()},
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "Library type is not recognized") {
		t.Fatalf("photo library create status = %d, body: %s", status, body)
	}
}

func TestProductContractExposesCanonicalCrossPlatformHierarchy(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var contract CanonicalProductContract
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/product-contract", nil, &contract)
	if status != http.StatusOK {
		t.Fatalf("product contract status = %d, body: %s", status, body)
	}
	if contract.APIVersion != "v1" || contract.QueryLimits.CursorTTLSeconds < 1 || contract.QueryLimits.MaximumClauses < 1 {
		t.Fatalf("invalid product contract: %#v", contract)
	}
	wanted := map[string][]string{
		"movies":      {"discover", "movies", "collections", "categories"},
		"tv":          {"discover", "shows", "episodes", "collections", "categories"},
		"music":       {"discover", "artists", "albums", "tracks", "playlists", "genres"},
		"audiobooks":  {"discover", "authors", "books", "series", "collections"},
		"recorded-tv": {"recordings", "shows", "schedule", "categories"},
	}
	for _, libraryKind := range contract.LibraryKinds {
		expected, ok := wanted[libraryKind.ID]
		if !ok {
			continue
		}
		actual := make([]string, 0, len(libraryKind.Pivots))
		for _, pivot := range libraryKind.Pivots {
			actual = append(actual, pivot.ID)
		}
		if strings.Join(actual, ",") != strings.Join(expected, ",") {
			t.Fatalf("%s pivots = %#v, expected %#v", libraryKind.ID, actual, expected)
		}
		for _, pivot := range libraryKind.Pivots {
			if pivot.ID == "sources" {
				t.Fatalf("administrative Sources leaked into %s browse pivots", libraryKind.ID)
			}
			if pivot.BrowseSupported && (!stringSliceContains(pivot.SupportedViews, "table") || !stringSliceContains(pivot.SupportedViews, "compact-grid")) {
				t.Fatalf("%s/%s did not publish all supported catalogue presentations: %#v", libraryKind.ID, pivot.ID, pivot.SupportedViews)
			}
		}
	}
	if len(contract.BrowseFields) == 0 || len(contract.BrowseOperators) == 0 || len(contract.BrowseSorts) == 0 {
		t.Fatalf("product contract browse vocabulary is empty: %#v", contract)
	}
}

func TestDeviceAliasesAndTrustedDeviceEnforcement(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: jar}
	loginUser(t, adminClient, serverURL)

	var devices ListResponse[Device]
	status, body := doJSON(t, adminClient, http.MethodGet, serverURL+"/api/devices", nil, &devices)
	if status != http.StatusOK || len(devices.Items) == 0 {
		t.Fatalf("devices status=%d body=%s devices=%#v", status, body, devices)
	}
	deviceID := devices.Items[0].ID
	var updated Device
	status, body = doJSON(t, adminClient, http.MethodPatch, serverURL+"/api/devices/"+deviceID, DeviceUpdateRequest{Name: stringPtr("Living Room TV")}, &updated)
	if status != http.StatusOK {
		t.Fatalf("rename status=%d body=%s", status, body)
	}
	if updated.Name != "Living Room TV" || updated.AutoName == "" {
		t.Fatalf("renamed device = %#v", updated)
	}
	deviceOptions := DeviceOptions{PreferredAudioLanguage: "EN_ca", PreferredSubtitleLanguage: "fr-CA", SubtitleMode: "always"}
	status, body = doJSON(t, adminClient, http.MethodPatch, serverURL+"/api/devices/"+deviceID, DeviceUpdateRequest{Options: &deviceOptions}, &updated)
	if status != http.StatusOK {
		t.Fatalf("device options status=%d body=%s", status, body)
	}
	if updated.Options.PreferredAudioLanguage != "en-ca" || updated.Options.PreferredSubtitleLanguage != "fr-ca" || updated.Options.SubtitleMode != "always" {
		t.Fatalf("device options were not normalized: %#v", updated.Options)
	}
	status, body = doJSON(t, adminClient, http.MethodPatch, serverURL+"/api/devices/"+deviceID, DeviceUpdateRequest{Name: stringPtr("")}, &updated)
	if status != http.StatusOK || updated.Name != updated.AutoName {
		t.Fatalf("clear alias status=%d body=%s device=%#v", status, body, updated)
	}
	status, body = patchSettingsGroups(t, adminClient, serverURL, map[string]any{"devices": map[string]any{"requireTrustedDevices": true}}, nil)
	if status != http.StatusOK {
		t.Fatalf("save device settings status=%d body=%s", status, body)
	}

	otherJar, _ := cookiejar.New(nil)
	otherClient := &http.Client{Jar: otherJar}
	status, body = doJSONWithUserAgent(t, otherClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "admin",
		"password": "Password1234",
	}, nil, "Portico Test TV/1.0")
	if status != http.StatusForbidden || !strings.Contains(body, "device_not_trusted") {
		t.Fatalf("untrusted login status=%d body=%s", status, body)
	}

	status, body = doJSON(t, adminClient, http.MethodGet, serverURL+"/api/devices", nil, &devices)
	if status != http.StatusOK {
		t.Fatalf("reload devices status=%d body=%s", status, body)
	}
	var pending Device
	for _, candidate := range devices.Items {
		if candidate.ID != deviceID && !candidate.Trusted {
			pending = candidate
			break
		}
	}
	if pending.ID == "" {
		t.Fatalf("expected pending untrusted device, got %#v", devices.Items)
	}
	status, body = doJSON(t, adminClient, http.MethodPatch, serverURL+"/api/devices/"+pending.ID, DeviceUpdateRequest{Trusted: boolPtr(true)}, &updated)
	if status != http.StatusOK || !updated.Trusted {
		t.Fatalf("trust pending status=%d body=%s device=%#v", status, body, updated)
	}
	status, body = doJSONWithUserAgent(t, otherClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "admin",
		"password": "Password1234",
	}, nil, "Portico Test TV/1.0")
	if status != http.StatusOK {
		t.Fatalf("trusted login status=%d body=%s", status, body)
	}
	assertAuthenticated(t, otherClient, serverURL, true)
}

func TestDisplayPreferencesPersistPerUserClientAndView(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var preference DisplayPreference
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/display-preferences/web/library:lib_movies", nil, &preference)
	if status != http.StatusOK {
		t.Fatalf("display preference get status=%d body=%s", status, body)
	}
	if preference.Client != "web" || preference.View != "library:lib_movies" || len(preference.Preferences) != 0 {
		t.Fatalf("default display preference = %#v", preference)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/display-preferences/web/library:lib_movies", DisplayPreferenceRequest{
		Preferences: map[string]any{
			"tab":    "Library",
			"sort":   "year",
			"filter": "genre:Drama",
			"view":   "list",
		},
	}, &preference)
	if status != http.StatusOK {
		t.Fatalf("display preference patch status=%d body=%s", status, body)
	}
	if preference.Preferences["sort"] != "year" || preference.Preferences["view"] != "list" {
		t.Fatalf("saved display preference = %#v", preference)
	}

	preference = DisplayPreference{}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/display-preferences/web/library:lib_movies", nil, &preference)
	if status != http.StatusOK {
		t.Fatalf("display preference reload status=%d body=%s", status, body)
	}
	if preference.Preferences["filter"] != "genre:Drama" || preference.UpdatedAt == "" {
		t.Fatalf("reloaded display preference = %#v", preference)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/display-preferences/web/bad/path", DisplayPreferenceRequest{Preferences: map[string]any{}}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("bad display preference route status=%d body=%s", status, body)
	}
}

func TestUserMediaFavoriteAndReactionPersist(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var item MediaItem
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/favorite", map[string]bool{"favorite": true}, &item)
	if status != http.StatusOK {
		t.Fatalf("favorite status=%d body=%s", status, body)
	}
	if !item.State.Favorite {
		t.Fatalf("favorite state was not set: %#v", item.State)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/reaction", map[string]string{"reaction": "like"}, &item)
	if status != http.StatusOK {
		t.Fatalf("reaction status=%d body=%s", status, body)
	}
	if item.State.Reaction != "like" {
		t.Fatalf("reaction was not set: %#v", item.State)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/reaction", map[string]string{"reaction": "dislike"}, &item)
	if status != http.StatusOK || item.State.Reaction != "dislike" {
		t.Fatalf("dislike status=%d body=%s state=%#v", status, body, item.State)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/reaction", map[string]string{"reaction": "bad"}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid reaction status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("reload media status=%d body=%s", status, body)
	}
	if !item.State.Favorite || item.State.Reaction != "dislike" {
		t.Fatalf("reloaded user state = %#v", item.State)
	}
}

func TestQuickConnectAuthorizesDeviceSession(t *testing.T) {
	serverURL := newAuthTestServer(t)
	var started QuickConnectStartResponse
	status, body := doJSONWithUserAgent(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/start", QuickConnectStartRequest{
		InstallationID: "roku-living-room-0001",
		DeviceName:     "Living Room Roku",
		App:            "Portico TV",
		Platform:       "Roku",
	}, &started, "Portico Roku/1.0")
	if status != http.StatusCreated {
		t.Fatalf("quick connect start status=%d body=%s", status, body)
	}
	if len(started.Code) != 9 || normalizeQuickConnectCode(started.Code) == "" || started.ProtocolVersion != 1 || started.Secret == "" || started.ExpiresAt == "" || !strings.HasSuffix(started.ApprovalURL, "/#/settings/devices") || !strings.Contains(started.DeepLinkURL, "portico://quick-connect") || strings.Contains(started.ApprovalURL, started.Code) || strings.Contains(started.DeepLinkURL, started.Code) {
		t.Fatalf("quick connect start response = %#v", started)
	}

	var qcStatus QuickConnectStatusResponse
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/status", map[string]string{"secret": started.Secret}, &qcStatus)
	if status != http.StatusOK || qcStatus.Status != "pending" {
		t.Fatalf("quick connect pending status=%d body=%s response=%#v", status, body, qcStatus)
	}

	jar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: jar}
	loginUser(t, adminClient, serverURL)
	var pending ListResponse[QuickConnectRequest]
	status, body = doJSON(t, adminClient, http.MethodGet, serverURL+"/api/auth/quick-connect/pending", nil, &pending)
	if status != http.StatusOK || len(pending.Items) != 1 || pending.Items[0].Code != started.Code || pending.Items[0].ProtocolVersion != 1 {
		t.Fatalf("quick connect pending list status=%d body=%s pending=%#v", status, body, pending)
	}
	var approved QuickConnectRequest
	status, body = doJSON(t, adminClient, http.MethodPost, serverURL+"/api/auth/quick-connect/authorize", map[string]string{"code": started.Code}, &approved)
	if status != http.StatusOK || approved.Code != started.Code || approved.ProtocolVersion != 1 {
		t.Fatalf("quick connect approve status=%d body=%s approved=%#v", status, body, approved)
	}

	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/status", map[string]string{"secret": started.Secret}, &qcStatus)
	if status != http.StatusOK || qcStatus.Status != "approved" {
		t.Fatalf("quick connect approved status=%d body=%s response=%#v", status, body, qcStatus)
	}
	deviceJar, _ := cookiejar.New(nil)
	deviceClient := &http.Client{Jar: deviceJar}
	var auth NativeSessionCredentials
	status, body = doJSONWithUserAgent(t, deviceClient, http.MethodPost, serverURL+"/api/auth/quick-connect/exchange", map[string]string{"secret": started.Secret}, &auth, "Portico Roku/1.0")
	if status != http.StatusOK || auth.AccessToken == "" || auth.RefreshToken == "" || auth.User.Username != "admin" || auth.Device.InstallationID != "roku-living-room-0001" {
		t.Fatalf("quick connect exchange status=%d body=%s auth=%#v", status, body, auth)
	}
	assertBearerAuthenticated(t, deviceClient, serverURL, auth.AccessToken, true)

	var recovered NativeSessionCredentials
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/exchange", map[string]string{"secret": started.Secret}, &recovered)
	if status != http.StatusOK || recovered.AccessToken != auth.AccessToken || recovered.RefreshToken != auth.RefreshToken {
		t.Fatalf("quick connect receipt recovery status=%d body=%s credentials=%#v", status, body, recovered)
	}

	var deniedStart QuickConnectStartResponse
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/start", QuickConnectStartRequest{
		InstallationID: "apple-office-tv-0001",
		DeviceName:     "Office TV",
		App:            "Portico TV",
		Platform:       "Apple TV",
	}, &deniedStart)
	if status != http.StatusCreated {
		t.Fatalf("quick connect deny start status=%d body=%s", status, body)
	}
	var denied QuickConnectRequest
	status, body = doJSON(t, adminClient, http.MethodPost, serverURL+"/api/auth/quick-connect/deny", map[string]string{"code": deniedStart.Code}, &denied)
	if status != http.StatusOK || denied.Code != deniedStart.Code {
		t.Fatalf("quick connect deny status=%d body=%s denied=%#v", status, body, denied)
	}

	var audit ListResponse[AuditEvent]
	status, body = doJSON(t, adminClient, http.MethodGet, serverURL+"/api/audit-events?limit=50", nil, &audit)
	if status != http.StatusOK {
		t.Fatalf("quick connect audit status=%d body=%s", status, body)
	}
	for _, action := range []string{
		"quick_connect.approved",
		"quick_connect.exchanged",
		"quick_connect.denied",
	} {
		if !hasAuditAction(audit.Items, action) {
			t.Fatalf("expected quick connect audit action %s, got %#v", action, audit.Items)
		}
	}
}

func TestQuickConnectApprovalCanBeRestrictedToOwner(t *testing.T) {
	serverURL := newAuthTestServer(t)
	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}
	loginUser(t, adminClient, serverURL)
	status, body := patchSettingsGroups(t, adminClient, serverURL, map[string]any{
		"devices": map[string]any{"quickConnectApprovalMode": "ownerOnly"},
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("save quick connect approval mode status=%d body=%s", status, body)
	}

	var member User
	status, body = doJSON(t, adminClient, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username:    "member",
		Email:       "member@example.test",
		DisplayName: "Member",
		Password:    "Password1234",
		Role:        "user",
		Permissions: map[string]bool{"playMedia": true},
		LibraryIDs:  []string{"lib_movies"},
	}, &member)
	if status != http.StatusCreated {
		t.Fatalf("create member status=%d body=%s", status, body)
	}
	memberJar, _ := cookiejar.New(nil)
	memberClient := &http.Client{Jar: memberJar}
	var memberAuth AuthMeResponse
	status, body = doJSON(t, memberClient, http.MethodPost, serverURL+"/api/auth/login", map[string]any{
		"login":    "member",
		"password": "Password1234",
	}, &memberAuth)
	if status != http.StatusOK || !memberAuth.Authenticated {
		t.Fatalf("member login status=%d body=%s auth=%#v", status, body, memberAuth)
	}

	var started QuickConnectStartResponse
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/start", QuickConnectStartRequest{
		InstallationID: "roku-bedroom-tv-0001",
		DeviceName:     "Bedroom TV",
		App:            "Portico TV",
		Platform:       "Roku",
	}, &started)
	if status != http.StatusCreated {
		t.Fatalf("quick connect start status=%d body=%s", status, body)
	}
	status, body = doJSON(t, memberClient, http.MethodGet, serverURL+"/api/auth/quick-connect/pending", nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("member pending status=%d body=%s", status, body)
	}
	status, body = doJSON(t, memberClient, http.MethodPost, serverURL+"/api/auth/quick-connect/authorize", map[string]string{"code": started.Code}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("member approve status=%d body=%s", status, body)
	}

	var approved QuickConnectRequest
	status, body = doJSON(t, adminClient, http.MethodPost, serverURL+"/api/auth/quick-connect/authorize", map[string]string{"code": started.Code}, &approved)
	if status != http.StatusOK || approved.Code != started.Code {
		t.Fatalf("admin approve status=%d body=%s approved=%#v", status, body, approved)
	}
}

func TestQuickConnectExchangeLogsInApprovingUser(t *testing.T) {
	serverURL := newAuthTestServer(t)
	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}
	loginUser(t, adminClient, serverURL)

	var member User
	status, body := doJSON(t, adminClient, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username:    "approver",
		Email:       "approver@example.test",
		DisplayName: "Approving Member",
		Password:    "Password1234",
		Role:        "user",
		Permissions: map[string]bool{"playMedia": true},
		LibraryIDs:  []string{"lib_movies"},
	}, &member)
	if status != http.StatusCreated {
		t.Fatalf("create member status=%d body=%s", status, body)
	}
	memberJar, _ := cookiejar.New(nil)
	memberClient := &http.Client{Jar: memberJar}
	var memberAuth AuthMeResponse
	status, body = doJSON(t, memberClient, http.MethodPost, serverURL+"/api/auth/login", map[string]any{
		"login":    "approver",
		"password": "Password1234",
	}, &memberAuth)
	if status != http.StatusOK || !memberAuth.Authenticated {
		t.Fatalf("member login status=%d body=%s auth=%#v", status, body, memberAuth)
	}

	var started QuickConnectStartResponse
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/quick-connect/start", QuickConnectStartRequest{
		InstallationID: "android-kitchen-0001",
		DeviceName:     "Kitchen TV",
		App:            "Portico TV",
		Platform:       "Android TV",
	}, &started)
	if status != http.StatusCreated {
		t.Fatalf("quick connect start status=%d body=%s", status, body)
	}
	var approved QuickConnectRequest
	status, body = doJSON(t, memberClient, http.MethodPost, serverURL+"/api/auth/quick-connect/authorize", map[string]string{"code": started.Code}, &approved)
	if status != http.StatusOK || approved.Code != started.Code {
		t.Fatalf("member approve status=%d body=%s approved=%#v", status, body, approved)
	}

	deviceJar, _ := cookiejar.New(nil)
	deviceClient := &http.Client{Jar: deviceJar}
	var auth NativeSessionCredentials
	status, body = doJSONWithUserAgent(t, deviceClient, http.MethodPost, serverURL+"/api/auth/quick-connect/exchange", map[string]string{"secret": started.Secret}, &auth, "Portico Android TV/1.0")
	if status != http.StatusOK || auth.AccessToken == "" || auth.User.ID != member.ID || auth.User.Username != "approver" {
		t.Fatalf("quick connect exchange should log in approving user, status=%d body=%s auth=%#v member=%#v", status, body, auth, member)
	}
}

func TestCollectionLifecycleUsesFirstClassRoutes(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var collection Collection
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/collections", CollectionCreateRequest{Title: "Favorites"}, &collection)
	if status != http.StatusCreated {
		t.Fatalf("create collection status = %d, body: %s", status, body)
	}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playlists/"+collection.ID, nil, nil)
	if status != http.StatusNotFound || !strings.Contains(body, "playlist_not_found") {
		t.Fatalf("collection leaked through playlist domain: status=%d body=%s", status, body)
	}
	var added CollectionMembershipBatchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/collections/"+collection.ID+"/memberships:batch", CollectionMembershipBatchRequest{AddMediaIDs: []string{"movie_meridian", "movie_saffron"}}, &added)
	if status != http.StatusOK || added.Collection.ItemCount != 2 || added.Added != 2 {
		t.Fatalf("add collection memberships status=%d body=%s result=%#v", status, body, added)
	}
	var removed CollectionMembershipBatchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/collections/"+collection.ID+"/memberships:batch", CollectionMembershipBatchRequest{RemoveMediaIDs: []string{"movie_meridian", "movie_saffron"}}, &removed)
	if status != http.StatusOK || removed.Collection.ItemCount != 0 || removed.Removed != 2 {
		t.Fatalf("remove collection memberships status=%d body=%s result=%#v", status, body, removed)
	}
}

func TestRuleBackedCollectionItemsUseTheCollectionItemsRoute(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	value, _ := json.Marshal("movie")
	var view SavedView
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/saved-views", SavedViewCreateRequest{
		Title: "All Movies", LibraryID: "lib_movies", Pivot: "movies",
		Query: &BrowseExpression{Field: "entityKind", Operator: "equals", Value: value},
		Sort:  []BrowseSort{{Field: "title", Direction: "asc"}},
	}, &view)
	if status != http.StatusCreated {
		t.Fatalf("create saved view status=%d body=%s view=%#v", status, body, view)
	}
	var page BrowseLibraryResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/saved-views/"+view.ID+"/browse", SavedViewBrowseRequest{Limit: 1}, &page)
	if status != http.StatusOK || len(page.Items) != 1 {
		t.Fatalf("saved view browse status=%d body=%s page=%#v", status, body, page)
	}
}

func TestPlaylistBulkItemsAreBoundedServerSide(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playlist SavedPlaylist
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playlists", PlaylistCreateRequest{Title: "Bulk Seeds"}, &playlist)
	if status != http.StatusCreated {
		t.Fatalf("create playlist status = %d, body: %s", status, body)
	}

	var bulk PlaylistItemsBatchResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{
		AddMediaIDs: []string{"movie_meridian", "movie_saffron", "movie_meridian"},
	}, &bulk)
	if status != http.StatusOK {
		t.Fatalf("bulk add status = %d, body: %s", status, body)
	}
	if bulk.Added != 3 || bulk.Unchanged != 0 {
		t.Fatalf("unexpected bulk counters: %#v", bulk)
	}
	if bulk.Playlist.ItemCount != 3 {
		t.Fatalf("bulk playlist count=%d", bulk.Playlist.ItemCount)
	}

	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{
		AddMediaIDs: []string{"movie_meridian", "movie_saffron"},
	}, &bulk)
	if status != http.StatusOK {
		t.Fatalf("duplicate bulk add status = %d, body: %s", status, body)
	}
	if bulk.Added != 2 || bulk.Unchanged != 0 || bulk.Playlist.ItemCount != 5 {
		t.Fatalf("expected repeated media to create distinct playlist entries, got %#v", bulk)
	}

	tooMany := make([]string, maxBulkMediaItems+1)
	for index := range tooMany {
		tooMany[index] = "bulk_media_" + strconv.Itoa(index)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{AddMediaIDs: tooMany}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "limited") {
		t.Fatalf("oversized bulk add status = %d, body: %s", status, body)
	}
}

func TestPlaylistCreateWithInitialItemsIsBoundedServerSide(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playlist SavedPlaylist
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playlists", PlaylistCreateRequest{
		Title:    "Seeded",
		MediaIDs: []string{"movie_meridian", "movie_saffron", "movie_meridian"},
	}, &playlist)
	if status != http.StatusCreated {
		t.Fatalf("create seeded playlist status = %d, body: %s", status, body)
	}
	if playlist.ItemCount != 3 {
		t.Fatalf("seeded playlist count=%d", playlist.ItemCount)
	}

	var items PlaylistEntryPage
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playlists/"+playlist.ID+"/items", nil, &items)
	if status != http.StatusOK {
		t.Fatalf("get seeded playlist items status = %d, body: %s", status, body)
	}
	if len(items.Items) != 3 || items.Items[0].Media.ID != "movie_meridian" || items.Items[1].Media.ID != "movie_saffron" || items.Items[2].Media.ID != "movie_meridian" || items.Items[0].EntryID == items.Items[2].EntryID {
		t.Fatalf("unexpected seeded playlist items: %#v", items.Items)
	}

	tooMany := make([]string, maxBulkMediaItems+1)
	for index := range tooMany {
		tooMany[index] = "bulk_media_" + strconv.Itoa(index)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists", PlaylistCreateRequest{
		Title:    "Too Many Seeds",
		MediaIDs: tooMany,
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "limited") {
		t.Fatalf("oversized seeded create status = %d, body: %s", status, body)
	}
}

func TestPlaylistDetailItemsAreCappedServerSide(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO playlists (id, user_id, profile_id, kind, title, summary, visibility, smart_filter_json, created_at, updated_at)
		VALUES ('plist_cap_test', ?, ?, 'playlist', 'Cap Test', '', 'private', '{}', ?, ?)`,
		accountIDForUser(user), viewerProfileID(user), now, now); err != nil {
		t.Fatalf("insert capped playlist: %v", err)
	}
	playlistCapTotal := maxPlaylistItemsResponse + 25
	for index := 0; index < playlistCapTotal; index++ {
		mediaID := fmt.Sprintf("playlist_cap_media_%03d", index)
		title := fmt.Sprintf("Playlist Cap Media %03d", index)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, tags_json, labels_json, added_at)
			VALUES (?, 'lib_movies', 'movie', ?, ?, '[]', '[]', '[]', ?)`,
			mediaID, title, title, now); err != nil {
			t.Fatalf("insert capped playlist media %d: %v", index, err)
		}
		if _, err := db.Exec(`
			INSERT INTO playlist_items (playlist_id, media_id, sort_order, added_at)
			VALUES ('plist_cap_test', ?, ?, ?)`,
			mediaID, index, now); err != nil {
			t.Fatalf("insert capped playlist item %d: %v", index, err)
		}
	}
	playlist, err := server.getPlaylist(user, "plist_cap_test", true)
	if err != nil {
		t.Fatalf("load capped playlist: %v", err)
	}
	if playlist.ItemCount != playlistCapTotal {
		t.Fatalf("playlist item count = %d, want %d", playlist.ItemCount, playlistCapTotal)
	}
	if len(playlist.Items) != maxPlaylistItemsResponse {
		t.Fatalf("playlist detail returned %d items, want capped %d", len(playlist.Items), maxPlaylistItemsResponse)
	}
	if playlist.Items[0].ID != "playlist_cap_media_000" || playlist.Items[len(playlist.Items)-1].ID != "playlist_cap_media_249" {
		t.Fatalf("playlist cap should preserve ordered first page, first=%s last=%s", playlist.Items[0].ID, playlist.Items[len(playlist.Items)-1].ID)
	}
	metadata, err := server.getPlaylist(user, "plist_cap_test", false)
	if err != nil {
		t.Fatalf("load capped playlist metadata: %v", err)
	}
	if metadata.ItemCount != playlistCapTotal || len(metadata.Items) != 0 {
		t.Fatalf("metadata-only playlist should preserve count without items, count=%d len=%d", metadata.ItemCount, len(metadata.Items))
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/playlists/plist_cap_test/items?limit=999&offset=0", nil)
	server.handlePlaylists(rec, req, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("playlist items first page status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var firstPage ListResponse[MediaItem]
	if err := json.NewDecoder(rec.Body).Decode(&firstPage); err != nil {
		t.Fatalf("decode playlist first page: %v", err)
	}
	if firstPage.Total != playlistCapTotal || firstPage.Limit != maxPlaylistItemsResponse || firstPage.Offset != 0 || !firstPage.HasMore {
		t.Fatalf("unexpected playlist first page metadata: %#v", firstPage)
	}
	if len(firstPage.Items) != maxPlaylistItemsResponse {
		t.Fatalf("unexpected playlist first page length = %d, want %d", len(firstPage.Items), maxPlaylistItemsResponse)
	}
	if firstPage.Items[0].ID != "playlist_cap_media_000" || firstPage.Items[len(firstPage.Items)-1].ID != "playlist_cap_media_249" {
		t.Fatalf("unexpected playlist first page items: first=%s last=%s", firstPage.Items[0].ID, firstPage.Items[len(firstPage.Items)-1].ID)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/playlists/plist_cap_test/items?limit=50&offset=250", nil)
	server.handlePlaylists(rec, req, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("playlist items tail page status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var tailPage ListResponse[MediaItem]
	if err := json.NewDecoder(rec.Body).Decode(&tailPage); err != nil {
		t.Fatalf("decode playlist tail page: %v", err)
	}
	if tailPage.Total != playlistCapTotal || tailPage.Limit != 50 || tailPage.Offset != 250 || tailPage.HasMore {
		t.Fatalf("unexpected playlist tail page metadata: %#v", tailPage)
	}
	if len(tailPage.Items) != 25 {
		t.Fatalf("unexpected playlist tail page length = %d, want 25", len(tailPage.Items))
	}
	if tailPage.Items[0].ID != "playlist_cap_media_250" || tailPage.Items[len(tailPage.Items)-1].ID != "playlist_cap_media_274" {
		t.Fatalf("unexpected playlist tail page items: first=%s last=%s", tailPage.Items[0].ID, tailPage.Items[len(tailPage.Items)-1].ID)
	}
	_, err = server.reorderPlaylistItems(user, "plist_cap_test", []string{"playlist_cap_media_001", "playlist_cap_media_000"}, false)
	if err == nil || !strings.Contains(err.Error(), "reorder is limited") {
		t.Fatalf("large playlist reorder should be rejected before scanning all items, got %v", err)
	}
}

func TestPlaylistDetailHonorsContextCancellation(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := server.getPlaylistContext(ctx, user, "plist_curated", true); !errors.Is(err, context.Canceled) {
		t.Fatalf("getPlaylistContext error = %v, expected context.Canceled", err)
	}
}

func TestPlaylistSharingAllowsServerMembersAndHonorsEditPermission(t *testing.T) {
	serverURL := newAuthTestServer(t)
	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}
	loginUser(t, adminClient, serverURL)

	var viewer User
	status, body := doJSON(t, adminClient, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username:    "viewer",
		Email:       "viewer@example.test",
		DisplayName: "Viewer",
		Password:    "Password1234",
		Role:        "user",
		Permissions: map[string]bool{
			"playMedia": true,
		},
		LibraryIDs: []string{"lib_movies"},
	}, &viewer)
	if status != http.StatusCreated {
		t.Fatalf("create viewer status = %d, body: %s", status, body)
	}

	var playlist SavedPlaylist
	viewOnlyShare := []SavedResourceShareRequest{{UserID: viewer.ID, CanEdit: false}}
	status, body = doJSON(t, adminClient, http.MethodPost, serverURL+"/api/playlists", PlaylistCreateRequest{
		Title: "Shared Movies", Shares: viewOnlyShare, MediaIDs: []string{"movie_meridian"},
	}, &playlist)
	if status != http.StatusCreated {
		t.Fatalf("create playlist status = %d, body: %s", status, body)
	}
	if len(playlist.Shares) != 1 || playlist.Shares[0].UserID != viewer.ID || playlist.Shares[0].CanEdit {
		t.Fatalf("playlist shares = %#v", playlist.Shares)
	}
	viewerJar, _ := cookiejar.New(nil)
	viewerClient := &http.Client{Jar: viewerJar}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "viewer",
		"password": "Password1234",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("viewer login status = %d, body: %s", status, body)
	}

	var playlists PlaylistPage
	status, body = doJSON(t, viewerClient, http.MethodGet, serverURL+"/api/playlists", nil, &playlists)
	if status != http.StatusOK {
		t.Fatalf("viewer playlists status = %d, body: %s", status, body)
	}
	if len(playlists.Items) != 1 || playlists.Items[0].ID != playlist.ID || playlists.Items[0].CanEdit {
		t.Fatalf("viewer playlists = %#v", playlists.Items)
	}
	var shared SavedPlaylist
	status, body = doJSON(t, viewerClient, http.MethodGet, serverURL+"/api/playlists/"+playlist.ID, nil, &shared)
	if status != http.StatusOK {
		t.Fatalf("viewer get shared playlist status = %d, body: %s", status, body)
	}
	if shared.ItemCount != 1 {
		t.Fatalf("shared playlist item count = %d", shared.ItemCount)
	}
	if len(shared.Shares) != 1 || shared.Shares[0].Email != "" {
		t.Fatalf("shared playlist detail leaked recipient email: %#v", shared.Shares)
	}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{AddMediaIDs: []string{"movie_saffron"}}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "cannot edit") {
		t.Fatalf("viewer readonly add status = %d, body: %s", status, body)
	}

	editableShare := []SavedResourceShareRequest{{UserID: viewer.ID, CanEdit: true}}
	status, body = doJSON(t, adminClient, http.MethodPatch, serverURL+"/api/playlists/"+playlist.ID, PlaylistUpdateRequest{Shares: &editableShare}, &playlist)
	if status != http.StatusOK {
		t.Fatalf("grant edit status = %d, body: %s", status, body)
	}
	var added PlaylistItemsBatchResponse
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{AddMediaIDs: []string{"movie_saffron"}}, &added)
	if status != http.StatusOK {
		t.Fatalf("viewer editable add status = %d, body: %s", status, body)
	}
	if added.Playlist.ItemCount != 2 {
		t.Fatalf("editable shared playlist count = %d", added.Playlist.ItemCount)
	}
	status, body = doJSON(t, viewerClient, http.MethodPatch, serverURL+"/api/playlists/"+playlist.ID, PlaylistUpdateRequest{Title: stringPtr("Renamed By Viewer")}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("viewer metadata edit status = %d, body: %s", status, body)
	}
	var sharedEntries PlaylistEntryPage
	status, body = doJSON(t, viewerClient, http.MethodGet, serverURL+"/api/playlists/"+playlist.ID+"/items?limit=10", nil, &sharedEntries)
	if status != http.StatusOK || len(sharedEntries.Items) != 2 {
		t.Fatalf("load editable shared entries status = %d, body: %s entries=%#v", status, body, sharedEntries)
	}
	var saffronEntryID string
	for _, entry := range sharedEntries.Items {
		if entry.Media.ID == "movie_saffron" {
			saffronEntryID = entry.EntryID
		}
	}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{RemoveEntryIDs: []string{saffronEntryID}}, &added)
	if status != http.StatusOK {
		t.Fatalf("viewer editable remove status = %d, body: %s", status, body)
	}
	status, body = doJSON(t, adminClient, http.MethodDelete, serverURL+"/api/playlists/"+playlist.ID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("owner delete playlist status = %d, body: %s", status, body)
	}
	var audit ListResponse[AuditEvent]
	status, body = doJSON(t, adminClient, http.MethodGet, serverURL+"/api/audit-events?limit=50", nil, &audit)
	if status != http.StatusOK {
		t.Fatalf("playlist audit status = %d, body: %s", status, body)
	}
	for _, action := range []string{"playlist.created", "playlist.items_updated"} {
		if !hasAuditAction(audit.Items, action) {
			t.Fatalf("expected playlist audit action %s, got %#v", action, audit.Items)
		}
	}
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func TestPlaybackBitrateTestReturnsBoundedBinaryPayload(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/playback/bitrate-test?bytes=4096", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send bitrate test: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read bitrate test body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bitrate test status = %d", resp.StatusCode)
	}
	if len(body) != 4096 || resp.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("bitrate test len=%d content-type=%q", len(body), resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("bitrate test cache-control = %q", resp.Header.Get("Cache-Control"))
	}
}

func doJSONWithUserAgent(t *testing.T, client *http.Client, method string, endpoint string, payload any, out any, userAgent string) (int, string) {
	t.Helper()
	var bodyReader io.Reader
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		bodyReader = bytes.NewReader(payloadBytes)
	}
	req, err := http.NewRequest(method, endpoint, bodyReader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		req.Header.Set(csrfHeaderName, "1")
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if out != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, out); err != nil {
			t.Fatalf("decode response body: %v\n%s", err, responseBody)
		}
	}
	return resp.StatusCode, string(responseBody)
}

func doJSONWithForwardedFor(t *testing.T, client *http.Client, method string, endpoint string, payload any, out any, forwardedFor string) (int, string) {
	t.Helper()
	var bodyReader io.Reader
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		bodyReader = bytes.NewReader(payloadBytes)
	}
	req, err := http.NewRequest(method, endpoint, bodyReader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		req.Header.Set(csrfHeaderName, "1")
	}
	req.Header.Set("X-Forwarded-For", forwardedFor)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if out != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, out); err != nil {
			t.Fatalf("decode response body: %v\n%s", err, responseBody)
		}
	}
	return resp.StatusCode, string(responseBody)
}

func TestSmartPlaylistFiltersItems(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	if err := server.rebuildLibraryCategoryFacets("lib_music"); err != nil {
		t.Fatalf("rebuild music facets: %v", err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	genre, _ := json.Marshal("Electronic")
	var view SavedView
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/saved-views", SavedViewCreateRequest{
		Title: "Electronic Albums", LibraryID: "lib_music", Pivot: "albums",
		Query: &BrowseExpression{Field: "genre", Operator: "contains", Value: genre},
		Sort:  []BrowseSort{{Field: "title", Direction: "asc"}},
	}, &view)
	if status != http.StatusCreated {
		t.Fatalf("create saved view status = %d, body: %s", status, body)
	}
	var result BrowseLibraryResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/saved-views/"+view.ID+"/browse", SavedViewBrowseRequest{Limit: 10}, &result)
	if status != http.StatusOK || len(result.Items) != 1 || result.Items[0].ID != "album_mara" {
		t.Fatalf("expected only album_mara, status=%d body=%s items=%#v", status, body, result.Items)
	}
}

func TestSmartPlaylistGenreFilterUsesFacetReadModel(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	if _, err := db.Exec(`UPDATE media_items SET genres_json = '[]' WHERE id = 'album_mara'`); err != nil {
		t.Fatalf("clear album genre json: %v", err)
	}
	if _, err := db.Exec(`
		INSERT OR IGNORE INTO media_category_facets (media_id, library_id, facet_type, value, sort_value, updated_at)
		VALUES ('album_mara', 'lib_music', 'genre', 'Electronic', 'electronic', ?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("seed album genre facet: %v", err)
	}

	items, err := server.smartLibraryItems(user.ID, "lib_music", SmartFilter{Type: "album", Genre: "Electronic", Sort: "title"}, 10)
	if err != nil {
		t.Fatalf("smart library items: %v", err)
	}
	if len(items) != 1 || items[0].ID != "album_mara" {
		t.Fatalf("smart playlist should use category facet rows, got %#v", items)
	}
}

func TestSmartPlaylistRandomSortUsesPersistedRandomKey(t *testing.T) {
	if order := smartPlaylistSQLOrder("random"); !strings.Contains(order, "m.random_key ASC") || strings.Contains(order, "RANDOM()") {
		t.Fatalf("smart playlist random order = %q, expected persisted random key", order)
	}
}

func TestSmartPlaylistAppliesVisibilityBeforeLimit(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_smart_visibility', 'Smart Visibility', 'movie', 991, '/tmp/smart-visibility', '{}', ?)`, now); err != nil {
		t.Fatalf("insert smart visibility library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, max_content_rating, created_at, updated_at)
		VALUES ('usr_smart_visibility', 'smart-visibility', 'smart-visibility@example.test', 'Smart Visibility', 'hash', 'user', '{}', '{}', 'PG', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("insert smart visibility user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES ('usr_smart_visibility', 'lib_smart_visibility', ?)`, now); err != nil {
		t.Fatalf("grant smart visibility library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, content_rating, genres_json, tags_json, labels_json, added_at)
		VALUES
			('smart_visibility_blocked', 'lib_smart_visibility', 'movie', 'A Blocked', 'A Blocked', 'R', '[]', '[]', '[]', ?),
			('smart_visibility_allowed', 'lib_smart_visibility', 'movie', 'Z Allowed', 'Z Allowed', 'PG', '[]', '[]', '[]', ?)`,
		now, now); err != nil {
		t.Fatalf("insert smart visibility media: %v", err)
	}
	user, err := server.getUser("usr_smart_visibility")
	if err != nil {
		t.Fatalf("load smart visibility user: %v", err)
	}
	items, err := server.smartPlaylistItemsWithLimit(user, SmartFilter{
		Enabled:   true,
		LibraryID: "lib_smart_visibility",
		Type:      "movie",
		Sort:      "title",
		Limit:     1,
	}, 1)
	if err != nil {
		t.Fatalf("smart visibility items: %v", err)
	}
	if len(items) != 1 || items[0].ID != "smart_visibility_allowed" {
		t.Fatalf("smart playlist should filter restricted rows before LIMIT, got %#v", items)
	}
}

func TestSmartPlaylistCandidateBudgetIsBoundedAcrossLibraries(t *testing.T) {
	if got := smartPlaylistCandidateBudget(50, 1); got != 100 {
		t.Fatalf("single-library smart playlist budget = %d, want 100", got)
	}
	if got := smartPlaylistCandidateBudget(250, 50); got != 500 {
		t.Fatalf("multi-library smart playlist budget = %d, want 500", got)
	}
}

func TestInstantMixCandidateBudgetIsBounded(t *testing.T) {
	if got := instantMixCandidateBudget(10); got != 200 {
		t.Fatalf("small instant mix budget = %d, want 200", got)
	}
	if got := instantMixCandidateBudget(100); got != 1000 {
		t.Fatalf("large instant mix budget = %d, want 1000", got)
	}
	if got := instantMixCandidateBudget(5000); got != 1000 {
		t.Fatalf("oversized instant mix budget = %d, want 1000", got)
	}
}

func TestInstantMixReturnsPermissionFilteredMusicTracks(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var mix ListResponse[MediaItem]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/instant-mix/artist_mara?limit=10", nil, &mix)
	if status != http.StatusOK {
		t.Fatalf("instant mix status = %d, body: %s", status, body)
	}
	if mix.Total == 0 || !mediaIDsContain(mix.Items, "track_mara_01") {
		t.Fatalf("instant mix missing expected track: %#v", mix.Items)
	}

	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/instant-mix/genre:Electronic?limit=10", nil, &mix)
	if status != http.StatusOK {
		t.Fatalf("genre instant mix status = %d, body: %s", status, body)
	}
	if mix.Total == 0 || !mediaIDsContain(mix.Items, "track_mara_01") {
		t.Fatalf("genre instant mix missing expected track: %#v", mix.Items)
	}

	var playlist SavedPlaylist
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists", PlaylistCreateRequest{Title: "Mara Mix Seeds"}, &playlist)
	if status != http.StatusCreated {
		t.Fatalf("create playlist status = %d, body: %s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playlists/"+playlist.ID+"/items:batch", PlaylistItemsBatchRequest{AddMediaIDs: []string{"album_mara"}}, nil)
	if status != http.StatusOK {
		t.Fatalf("add playlist item status = %d, body: %s", status, body)
	}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/instant-mix/"+playlist.ID+"?limit=10", nil, &mix)
	if status != http.StatusOK {
		t.Fatalf("playlist instant mix status = %d, body: %s", status, body)
	}
	if mix.Total == 0 || !mediaIDsContain(mix.Items, "track_mara_01") {
		t.Fatalf("playlist instant mix missing expected track: %#v", mix.Items)
	}

	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/instant-mix/movie_meridian", nil, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "only available for music") {
		t.Fatalf("movie instant mix status = %d, body: %s", status, body)
	}
}

func TestPlaybackCommandLifecycle(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	payload := authenticatedPlaybackRuntimeRequest("movie_meridian")
	payload["clientInstanceId"] = "web-command"
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", payload, &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status = %d, body: %s", status, body)
	}
	var targets ListResponse[PlaybackTarget]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/targets?clientInstanceId=web-other", nil, &targets)
	if status != http.StatusOK || len(targets.Items) != 0 {
		t.Fatalf("other client session targets status=%d body=%s targets=%#v", status, body, targets)
	}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/targets?clientInstanceId=web-command", nil, &targets)
	if status != http.StatusOK || len(targets.Items) != 1 || targets.Items[0].Type != "session" || !stringSliceContains(targets.Items[0].SupportedCommands, "pause") || !stringSliceContains(targets.Items[0].SupportedCommands, "load") {
		t.Fatalf("session targets status=%d body=%s targets=%#v", status, body, targets)
	}

	var command PlaybackCommand
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions/"+playback.SessionID+"/command", PlaybackCommandRequest{Action: "pause"}, &command)
	if status != http.StatusOK {
		t.Fatalf("issue command status = %d, body: %s", status, body)
	}
	if command.Action != "pause" || command.ID == "" {
		t.Fatalf("unexpected command: %#v", command)
	}

	var polled PlaybackCommand
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback-sessions/"+playback.SessionID+"/command", nil, &polled)
	if status != http.StatusOK {
		t.Fatalf("poll command status = %d, body: %s", status, body)
	}
	if polled.ID != command.ID || polled.Action != "pause" {
		t.Fatalf("polled command mismatch: %#v vs %#v", polled, command)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/playback-sessions/"+playback.SessionID+"/command/events", nil)
	if err != nil {
		t.Fatalf("create command events request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open command events stream: %v", err)
	}
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read command event line: %v", err)
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") || strings.TrimSpace(line) != "event: command" {
		t.Fatalf("command events response status=%d content-type=%q first-line=%q", resp.StatusCode, resp.Header.Get("Content-Type"), line)
	}

	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions/"+playback.SessionID+"/command", PlaybackCommandRequest{Action: "load", MediaID: "movie_saffron", PositionSeconds: 12}, &command)
	if status != http.StatusOK {
		t.Fatalf("issue load command status = %d, body: %s", status, body)
	}
	if command.Action != "load" || command.MediaID != "movie_saffron" || command.PositionSeconds != 12 {
		t.Fatalf("unexpected load command: %#v", command)
	}

	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions/"+playback.SessionID+"/command", PlaybackCommandRequest{Action: "stop", Message: "Server restarting in 5 minutes."}, &command)
	if status != http.StatusOK {
		t.Fatalf("issue stop command status = %d, body: %s", status, body)
	}
	if command.Action != "stop" || command.Message != "Server restarting in 5 minutes." {
		t.Fatalf("unexpected stop command: %#v", command)
	}

	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback-sessions/"+playback.SessionID+"/command", nil, &polled)
	if status != http.StatusOK {
		t.Fatalf("poll ended stop command status = %d, body: %s", status, body)
	}
	if polled.ID != command.ID || polled.Action != "stop" || polled.Message != command.Message {
		t.Fatalf("polled ended stop command mismatch: %#v vs %#v", polled, command)
	}

	var live DashboardResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/dashboard?mode=live&period=5m&sections=sessions", nil, &live)
	if status != http.StatusOK {
		t.Fatalf("live dashboard status = %d, body: %s", status, body)
	}
	if len(live.NowPlaying) != 0 {
		t.Fatalf("stopped session remained on live dashboard: %#v", live.NowPlaying)
	}
}

func TestPlaybackStartUsesRequestedStartSeconds(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	payload := authenticatedPlaybackRuntimeRequest("movie_meridian")
	payload["startSeconds"] = 75
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", payload, &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status = %d, body: %s", status, body)
	}
	if playback.Media.State.ProgressSeconds != 75 {
		t.Fatalf("playback start seconds = %d, expected 75", playback.Media.State.ProgressSeconds)
	}
	if playback.StreamFormat != "hls" || !strings.Contains(playback.SourceURL, "/hls/") || !playback.Decision.RequiresTranscode {
		t.Fatalf("unverified 4K client capability must use the server-resolved HLS fallback, streamFormat=%q source=%q decision=%#v", playback.StreamFormat, playback.SourceURL, playback.Decision)
	}
}

func TestPlaybackActiveRestoreReturnsCurrentSession(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	payload := authenticatedPlaybackRuntimeRequest("movie_meridian")
	payload["clientInstanceId"], payload["startSeconds"] = "web-a", 90
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", payload, &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status = %d, body: %s", status, body)
	}

	var restored PlaybackRestoreResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/active", map[string]any{"clientInstanceId": "web-b", "clientProfile": map[string]any{"supportsHls": true}}, &restored)
	if status != http.StatusOK || restored.Active {
		t.Fatalf("expected inactive restore for another client instance status=%d body=%s restored=%#v", status, body, restored)
	}

	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/active", map[string]any{"clientProfile": map[string]any{"supportsHls": true}}, &restored)
	if status != http.StatusOK || restored.Active {
		t.Fatalf("expected inactive restore without a client instance status=%d body=%s restored=%#v", status, body, restored)
	}

	restored = PlaybackRestoreResponse{}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/active", map[string]any{"clientInstanceId": "web-a", "clientProfile": map[string]any{"supportsHls": true}}, &restored)
	if status != http.StatusOK {
		t.Fatalf("restore playback status = %d, body: %s", status, body)
	}
	if !restored.Active || restored.Playback == nil {
		t.Fatalf("expected active restore response: %#v", restored)
	}
	if restored.Playback.SessionID != playback.SessionID || restored.Playback.Media.ID != "movie_meridian" {
		t.Fatalf("unexpected restored playback: %#v", restored.Playback)
	}
	if restored.Playback.Media.State.ProgressSeconds <= 0 {
		t.Fatalf("restored playback lost progress: %#v", restored.Playback.Media.State)
	}
	if restored.Playback.StreamFormat != "hls" || !strings.Contains(restored.Playback.SourceURL, "/hls/") || !restored.Playback.Decision.RequiresTranscode {
		t.Fatalf("restored playback should preserve the server-resolved HLS fallback, streamFormat=%q source=%q", restored.Playback.StreamFormat, restored.Playback.SourceURL)
	}

	var targets ListResponse[PlaybackTarget]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/targets?clientInstanceId=web-b", nil, &targets)
	if status != http.StatusOK || len(targets.Items) != 0 {
		t.Fatalf("expected no active session target for another client status=%d body=%s targets=%#v", status, body, targets)
	}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/targets?clientInstanceId=web-a", nil, &targets)
	if status != http.StatusOK || len(targets.Items) != 1 || targets.Items[0].Type != "session" || targets.Items[0].ID != playback.SessionID {
		t.Fatalf("expected same-client active session target status=%d body=%s targets=%#v", status, body, targets)
	}

	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/playback-sessions/"+playback.SessionID, stoppedPlaybackRequest(playback), nil)
	if status != http.StatusOK {
		t.Fatalf("stop playback status = %d, body: %s", status, body)
	}
	restored = PlaybackRestoreResponse{}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/active", map[string]any{}, &restored)
	if status != http.StatusOK || restored.Active {
		t.Fatalf("expected inactive restore after stop status=%d body=%s restored=%#v", status, body, restored)
	}
}

func TestPlaybackStartStopsPreviousSessionForSameClientInstance(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	seedExactPlaybackFactsForFixture(t, server, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, server, "movie_neon")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var first PlaybackResponse
	firstPayload := authenticatedPlaybackRuntimeRequest("movie_meridian")
	firstPayload["clientInstanceId"] = "web-a"
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", firstPayload, &first)
	if status != http.StatusOK {
		t.Fatalf("first playback status = %d, body: %s", status, body)
	}

	var second PlaybackResponse
	secondPayload := authenticatedPlaybackRuntimeRequest("movie_neon")
	secondPayload["clientInstanceId"] = "web-a"
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", secondPayload, &second)
	if status != http.StatusOK {
		t.Fatalf("second playback status = %d, body: %s", status, body)
	}

	var restored PlaybackRestoreResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/active", map[string]any{"clientInstanceId": "web-a", "clientProfile": map[string]any{"supportsHls": true}}, &restored)
	if status != http.StatusOK {
		t.Fatalf("restore playback status = %d, body: %s", status, body)
	}
	if !restored.Active || restored.Playback == nil || restored.Playback.SessionID != second.SessionID {
		t.Fatalf("expected second playback session to be restored, first=%s second=%s restored=%#v", first.SessionID, second.SessionID, restored)
	}

	var targets ListResponse[PlaybackTarget]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/targets?clientInstanceId=web-a", nil, &targets)
	if status != http.StatusOK || len(targets.Items) != 1 || targets.Items[0].ID != second.SessionID {
		t.Fatalf("expected only second playback target status=%d body=%s targets=%#v", status, body, targets)
	}

	// Recreate the interleaving where the earlier inserted start reaches
	// replacement only after a newer session exists. Timestamps are deliberately
	// misleading: insertion order is the authoritative concurrency fence, so the
	// older request must never stop the newer row.
	if _, err := db.Exec(`
		UPDATE playback_sessions
		SET started_at = CASE id WHEN ? THEN ? ELSE ? END, ended_at = '', state = 'playing'
		WHERE id IN (?, ?)`,
		first.SessionID, "2026-08-23T10:00:00Z", "2026-08-23T10:00:01Z", first.SessionID, second.SessionID); err != nil {
		t.Fatalf("prepare concurrent replacement fixture: %v", err)
	}
	var userID string
	if err := db.QueryRow(`SELECT user_id FROM playback_sessions WHERE id = ?`, first.SessionID).Scan(&userID); err != nil {
		t.Fatalf("load playback owner: %v", err)
	}
	user, err := server.getUser(userID)
	if err != nil {
		t.Fatalf("load playback owner: %v", err)
	}
	if err := server.commitPlaybackSessionReplacement(context.Background(), user, first.SessionID, "web-a"); err != nil {
		t.Fatalf("commit delayed older replacement: %v", err)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM playback_sessions WHERE id = '`+second.SessionID+`' AND ended_at = '' AND state <> 'stopped'`, 1)
	if err := server.commitPlaybackSessionReplacement(context.Background(), user, second.SessionID, "web-a"); err != nil {
		t.Fatalf("commit newer replacement: %v", err)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM playback_sessions WHERE id = '`+first.SessionID+`' AND state = 'stopped'`, 1)
}

const playbackProgressTestDurationSeconds = 7200

func setMediaDurationForProgressTest(t *testing.T, db *sql.DB, mediaID string) {
	t.Helper()
	result, err := db.Exec(`UPDATE media_items SET duration_seconds = ? WHERE id = ?`, playbackProgressTestDurationSeconds, mediaID)
	if err != nil {
		t.Fatalf("set media duration for %s: %v", mediaID, err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		t.Fatalf("expected test media %s to exist", mediaID)
	}
}

func setPlaybackProgressPreferencesForTest(t *testing.T, db *sql.DB, userID string, preferences PlaybackProgressPreferences) {
	t.Helper()
	setProfileServerPreferenceValuesForTest(t, db, userID, func(values *profileServerPreferenceValues) {
		values.Playback.StartedThresholdPercent = preferences.StartedThresholdPercent
		values.Playback.PlayedThresholdPercent = preferences.PlayedThresholdPercent
	})
}

func writeOrderedPlaybackProgressForTest(t *testing.T, db *sql.DB, client *http.Client, serverURL, mediaID string, progressSeconds, durationSeconds int) MediaItem {
	t.Helper()
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, mediaID)
	var playback PlaybackResponse
	payload := authenticatedPlaybackRuntimeRequest(mediaID)
	payload["clientInstanceId"] = fmt.Sprintf("progress-contract-%s-%d", mediaID, progressSeconds)
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", payload, &playback)
	if status != http.StatusOK {
		t.Fatalf("start ordered playback for %s status=%d body=%s", mediaID, status, body)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence":   playback.NextEventSequence,
		"recordedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"state":           "playing",
		"progressSeconds": progressSeconds,
		"durationSeconds": durationSeconds,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("write ordered playback for %s status=%d body=%s", mediaID, status, body)
	}
	var item MediaItem
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/"+mediaID, nil, &item)
	if status != http.StatusOK {
		t.Fatalf("load %s after ordered playback status=%d body=%s", mediaID, status, body)
	}
	return item
}

func setUserPrivacyPreferencesForTest(t *testing.T, db *sql.DB, userID string, preferences UserPrivacyPreferences) {
	t.Helper()
	setProfileServerPreferenceValuesForTest(t, db, userID, func(values *profileServerPreferenceValues) {
		values.Privacy.PauseWatchHistory = preferences.PauseWatchHistory
		values.Privacy.ShowActivityToMembers = preferences.ShowActivityToMembers
		values.Privacy.IncludeInWatchWithFriends = preferences.IncludeInWatchWithFriends
	})
}

func setProfileServerPreferenceValuesForTest(t *testing.T, db *sql.DB, profileID string, mutate func(*profileServerPreferenceValues)) {
	t.Helper()
	server := &Server{db: db}
	serverID, err := server.profileDirectoryServerIDContext(context.Background())
	if err != nil {
		t.Fatalf("load server identity for preferences: %v", err)
	}
	values := defaultProfileServerValues(User{})
	var existing string
	if err := db.QueryRow(`SELECT values_json FROM viewer_preference_documents WHERE scope_type = 'profile-server' AND profile_id = ? AND server_id = ?`, profileID, serverID).Scan(&existing); err == nil {
		if err := json.Unmarshal([]byte(existing), &values); err != nil {
			t.Fatalf("decode profile-server preferences: %v", err)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("load profile-server preferences: %v", err)
	}
	mutate(&values)
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("marshal profile-server preferences: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		INSERT INTO viewer_preference_documents (
			id, scope_type, authority, account_id, profile_id, server_id, device_class, installation_id,
			version, revision, values_json, created_at, updated_at
		) VALUES (?, 'profile-server', 'local', ?, ?, ?, '', '', 'v1', 1, ?, ?, ?)
		ON CONFLICT(scope_type, authority, account_id, profile_id, server_id, device_class, installation_id)
		DO UPDATE SET values_json = excluded.values_json, revision = viewer_preference_documents.revision + 1, updated_at = excluded.updated_at`,
		randomID("pref"), userIDForProfileTest(t, db, profileID), profileID, serverID, string(raw), now, now); err != nil {
		t.Fatalf("store profile-server preferences: %v", err)
	}
}

func userIDForProfileTest(t *testing.T, db *sql.DB, profileID string) string {
	t.Helper()
	var accountID string
	if err := db.QueryRow(`SELECT account_id FROM profiles WHERE id = ?`, profileID).Scan(&accountID); err == nil {
		return accountID
	} else if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("load preference profile account: %v", err)
	}
	return profileID
}

func TestPlaybackHeartbeatPersistsMediaProgress(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status = %d, body: %s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence":   1,
		"recordedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"state":           "playing",
		"progressSeconds": 600,
		"durationSeconds": playbackProgressTestDurationSeconds,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("heartbeat status = %d, body: %s", status, body)
	}

	var item MediaItem
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("media detail status = %d, body: %s", status, body)
	}
	if item.State.ProgressSeconds != 600 {
		t.Fatalf("progress seconds = %d, expected 600", item.State.ProgressSeconds)
	}
}

func TestPausedWatchHistorySuppressesPlaybackProgressAndHistory(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	userID := adminUserID(t, db)
	setUserPrivacyPreferencesForTest(t, db, userID, UserPrivacyPreferences{
		PauseWatchHistory:         true,
		ShowActivityToMembers:     true,
		IncludeInWatchWithFriends: true,
	})
	if _, err := db.Exec(`DELETE FROM user_media_state WHERE user_id = ? AND media_id = ?`, userID, "movie_meridian"); err != nil {
		t.Fatalf("clear seeded media state: %v", err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status = %d, body: %s", status, body)
	}

	var historyPaused int
	if err := db.QueryRow(`SELECT history_paused FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&historyPaused); err != nil {
		t.Fatalf("load playback history_paused: %v", err)
	}
	if historyPaused != 1 {
		t.Fatalf("history_paused = %d, expected 1", historyPaused)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence":   1,
		"recordedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"state":           "playing",
		"progressSeconds": 600,
		"durationSeconds": playbackProgressTestDurationSeconds,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("heartbeat status = %d, body: %s", status, body)
	}

	var stateRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_media_state WHERE user_id = ? AND media_id = ? AND (watched <> 0 OR progress_seconds <> 0 OR COALESCE(last_played_at, '') <> '')`, userID, "movie_meridian").Scan(&stateRows); err != nil {
		t.Fatalf("count paused media state rows: %v", err)
	}
	if stateRows != 0 {
		t.Fatalf("paused watch history wrote %d media state row(s)", stateRows)
	}

	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/playback-sessions/"+playback.SessionID, stoppedPlaybackRequest(playback), nil)
	if status != http.StatusOK {
		t.Fatalf("stop playback status = %d, body: %s", status, body)
	}
	var sessionRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&sessionRows); err != nil {
		t.Fatalf("count paused playback session rows: %v", err)
	}
	if sessionRows != 0 {
		t.Fatalf("paused playback session should not remain in history, rows=%d", sessionRows)
	}
	var rollupRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dashboard_playback_rollups WHERE session_id = ?`, playback.SessionID).Scan(&rollupRows); err != nil {
		t.Fatalf("count paused dashboard rollups: %v", err)
	}
	if rollupRows != 0 {
		t.Fatalf("paused playback session should not create rollup, rows=%d", rollupRows)
	}
}

func TestClearWatchHistoryPreservesSavedMediaStateAndCurrentSession(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	userID := adminUserID(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	if _, err := db.Exec(`DELETE FROM user_media_state WHERE user_id = ? AND media_id = ?`, userID, "movie_meridian"); err != nil {
		t.Fatalf("clear seeded media state: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_media_state (user_id, profile_id, media_id, watchlisted, favorite, liked, watched, progress_seconds, rating, last_played_at, updated_at)
		VALUES (?, ?, 'movie_meridian', 1, 1, 1, 1, 900, 8, ?, ?)`,
		userID, userID, now, now); err != nil {
		t.Fatalf("insert media state: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, ended_at, state)
		VALUES ('play_clear_ended', ?, ?, 'movie_meridian', 'movie', 'Meridian', ?, ?, ?, 'stopped')`,
		userID, userID, now, now, now); err != nil {
		t.Fatalf("insert ended playback session: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO playback_sessions (id, user_id, profile_id, media_id, media_type, title, started_at, last_seen_at, state)
		VALUES ('play_clear_active', ?, ?, 'movie_meridian', 'movie', 'Meridian', ?, ?, 'playing')`,
		userID, userID, now, now); err != nil {
		t.Fatalf("insert active playback session: %v", err)
	}

	var response WatchHistoryClearResponse
	status, body := doJSON(t, client, http.MethodDelete, serverURL+"/api/account/watch-history", nil, &response)
	if status != http.StatusOK {
		t.Fatalf("clear watch history status = %d, body: %s", status, body)
	}
	if !response.OK || response.ClearedAt == "" {
		t.Fatalf("unexpected clear watch history response: %#v", response)
	}

	var watchlisted, favorite, liked, watched, progress, rating int
	var lastPlayed string
	if err := db.QueryRow(`
		SELECT watchlisted, favorite, liked, watched, progress_seconds, rating, COALESCE(last_played_at, '')
		FROM user_media_state
		WHERE user_id = ? AND profile_id = ? AND media_id = 'movie_meridian'`, userID, userID).Scan(&watchlisted, &favorite, &liked, &watched, &progress, &rating, &lastPlayed); err != nil {
		t.Fatalf("load cleared media state: %v", err)
	}
	if watchlisted != 1 || favorite != 1 || liked != 1 || rating != 8 {
		t.Fatalf("saved media state was not preserved: watchlisted=%d favorite=%d liked=%d rating=%d", watchlisted, favorite, liked, rating)
	}
	if watched != 0 || progress != 0 || lastPlayed != "" {
		t.Fatalf("watch history state was not cleared: watched=%d progress=%d lastPlayed=%q", watched, progress, lastPlayed)
	}

	var endedRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM playback_sessions WHERE id = 'play_clear_ended'`).Scan(&endedRows); err != nil {
		t.Fatalf("count ended session rows: %v", err)
	}
	if endedRows != 0 {
		t.Fatalf("ended session should be deleted after clear, rows=%d", endedRows)
	}
	var historyPaused int
	if err := db.QueryRow(`SELECT history_paused FROM playback_sessions WHERE id = 'play_clear_active'`).Scan(&historyPaused); err != nil {
		t.Fatalf("load active session history_paused: %v", err)
	}
	if historyPaused != 1 {
		t.Fatalf("active session history_paused=%d, expected 1", historyPaused)
	}
}

func TestPlaybackHeartbeatCoalescesDuplicateWrites(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status = %d, body: %s", status, body)
	}
	heartbeat := map[string]any{
		"eventSequence":   1,
		"recordedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"state":           "playing",
		"progressSeconds": 600,
		"durationSeconds": playbackProgressTestDurationSeconds,
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, heartbeat, nil)
	if status != http.StatusOK {
		t.Fatalf("initial heartbeat status = %d, body: %s", status, body)
	}
	stableSeenAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE playback_sessions SET last_seen_at = ? WHERE id = ?`, stableSeenAt, playback.SessionID); err != nil {
		t.Fatalf("stabilize last_seen_at: %v", err)
	}
	var stateUpdatedAt string
	if err := db.QueryRow(`SELECT updated_at FROM user_media_state WHERE media_id = 'movie_meridian'`).Scan(&stateUpdatedAt); err != nil {
		t.Fatalf("load initial media state update time: %v", err)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, heartbeat, nil)
	if status != http.StatusOK {
		t.Fatalf("duplicate heartbeat status = %d, body: %s", status, body)
	}
	var lastSeenAfter, stateUpdatedAfter string
	if err := db.QueryRow(`SELECT last_seen_at FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&lastSeenAfter); err != nil {
		t.Fatalf("load coalesced session last_seen_at: %v", err)
	}
	if err := db.QueryRow(`SELECT updated_at FROM user_media_state WHERE media_id = 'movie_meridian'`).Scan(&stateUpdatedAfter); err != nil {
		t.Fatalf("load coalesced media state update time: %v", err)
	}
	if lastSeenAfter != stableSeenAt || stateUpdatedAfter != stateUpdatedAt {
		t.Fatalf("duplicate heartbeat was not coalesced: lastSeen %q -> %q stateUpdated %q -> %q", stableSeenAt, lastSeenAfter, stateUpdatedAt, stateUpdatedAfter)
	}
}

func TestPlaybackProgressEventsRejectStaleUpdatesAndAllowIntentionalRewind(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("create playback session status = %d, body: %s", status, body)
	}

	recordedAt := time.Now().UTC()
	apply := func(sequence int64, position int) PlaybackProgressAcknowledgement {
		t.Helper()
		var acknowledgement PlaybackProgressAcknowledgement
		status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
			"eventSequence":   sequence,
			"recordedAt":      recordedAt.Add(time.Duration(sequence) * time.Second).Format(time.RFC3339Nano),
			"state":           "playing",
			"positionSeconds": position,
			"durationSeconds": playbackProgressTestDurationSeconds,
		}, &acknowledgement)
		if status != http.StatusOK {
			t.Fatalf("progress sequence %d status = %d, body: %s", sequence, status, body)
		}
		return acknowledgement
	}

	first := apply(2, 1200)
	if !first.Accepted || first.HighestEventSequence != 2 {
		t.Fatalf("first acknowledgement = %#v", first)
	}
	duplicate := apply(2, 1500)
	if duplicate.Accepted || !duplicate.Duplicate || duplicate.Stale || duplicate.HighestEventSequence != 2 {
		t.Fatalf("duplicate acknowledgement = %#v", duplicate)
	}
	stale := apply(1, 1800)
	if stale.Accepted || stale.Duplicate || !stale.Stale || stale.HighestEventSequence != 2 {
		t.Fatalf("stale acknowledgement = %#v", stale)
	}
	rewind := apply(3, 600)
	if !rewind.Accepted || rewind.HighestEventSequence != 3 {
		t.Fatalf("rewind acknowledgement = %#v", rewind)
	}

	var position int
	var highestSequence int64
	if err := db.QueryRow(`SELECT position_seconds, last_event_sequence FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&position, &highestSequence); err != nil {
		t.Fatalf("load ordered playback state: %v", err)
	}
	if position != 600 || highestSequence != 3 {
		t.Fatalf("ordered playback state position=%d sequence=%d", position, highestSequence)
	}
	var progressSessionID, progressRecordedAt string
	if err := db.QueryRow(`SELECT progress_session_id, progress_recorded_at FROM user_media_state WHERE media_id = ?`, "movie_meridian").Scan(&progressSessionID, &progressRecordedAt); err != nil {
		t.Fatalf("load playback progress provenance: %v", err)
	}
	if progressSessionID != playback.SessionID || progressRecordedAt == "" {
		t.Fatalf("playback progress provenance session=%q recordedAt=%q", progressSessionID, progressRecordedAt)
	}
}

func TestPlaybackStopPersistsItsAtomicPositionWhenMediaStateIsMissing(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	var profileID string
	if err := db.QueryRow(`SELECT id FROM profiles WHERE account_id = ? AND is_primary = 1`, userID).Scan(&profileID); err != nil {
		t.Fatalf("load admin primary profile: %v", err)
	}

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status = %d, body: %s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence":   1,
		"recordedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"state":           "playing",
		"progressSeconds": 600,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("heartbeat status = %d, body: %s", status, body)
	}

	if _, err := db.Exec(`DELETE FROM user_media_state WHERE profile_id = ? AND media_id = ?`, profileID, "movie_meridian"); err != nil {
		t.Fatalf("clear media state before stop: %v", err)
	}

	stopRequest := stoppedPlaybackRequest(playback)
	stopRequest.PositionSeconds = 600
	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/playback-sessions/"+playback.SessionID, stopRequest, nil)
	if status != http.StatusOK {
		t.Fatalf("stop playback status = %d, body: %s", status, body)
	}

	var item MediaItem
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("media detail status = %d, body: %s", status, body)
	}
	if item.State.ProgressSeconds != 600 {
		t.Fatalf("progress seconds after stop = %d, expected 600", item.State.ProgressSeconds)
	}
}

func TestPlaybackHeartbeatBelowStartThresholdKeepsTelemetryOutOfResume(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status = %d, body: %s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence":   1,
		"recordedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"state":           "playing",
		"progressSeconds": 123,
		"durationSeconds": playbackProgressTestDurationSeconds,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("heartbeat status = %d, body: %s", status, body)
	}

	var item MediaItem
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("media detail status = %d, body: %s", status, body)
	}
	if item.State.ProgressSeconds != 0 || item.State.Watched || item.State.LastPlayedAt != "" || item.State.Resume != nil {
		t.Fatalf("below-threshold media state should not look started: %#v", item.State)
	}

	var progressPercent, positionSeconds int
	if err := db.QueryRow(`SELECT progress, position_seconds FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&progressPercent, &positionSeconds); err != nil {
		t.Fatalf("load playback session telemetry: %v", err)
	}
	if progressPercent != 2 || positionSeconds != 123 {
		t.Fatalf("session telemetry progress=%d position=%d, expected 2 and 123", progressPercent, positionSeconds)
	}
}

func TestMediaProgressThresholdsAreServerAuthoritative(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	item := writeOrderedPlaybackProgressForTest(t, db, client, serverURL, "movie_meridian", 123, playbackProgressTestDurationSeconds)
	if item.State.ProgressSeconds != 0 || item.State.Watched || item.State.Resume != nil {
		t.Fatalf("below-threshold progress should be reset in UI state: %#v", item.State)
	}

	item = writeOrderedPlaybackProgressForTest(t, db, client, serverURL, "movie_meridian", 600, playbackProgressTestDurationSeconds)
	if item.State.ProgressSeconds != 600 || item.State.Watched || item.State.Resume == nil {
		t.Fatalf("started progress should create resume state: %#v", item.State)
	}

	item = writeOrderedPlaybackProgressForTest(t, db, client, serverURL, "movie_meridian", 6900, playbackProgressTestDurationSeconds)
	if item.State.ProgressSeconds != 0 || !item.State.Watched || item.State.Resume != nil {
		t.Fatalf("played-threshold progress should mark watched and reset resume: %#v", item.State)
	}
}

func TestPlaybackProgressThresholdPreferencesAffectServerPolicy(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	setPlaybackProgressPreferencesForTest(t, db, userID, PlaybackProgressPreferences{
		StartedThresholdPercent: 10,
		PlayedThresholdPercent:  90,
	})

	item := writeOrderedPlaybackProgressForTest(t, db, client, serverURL, "movie_meridian", 600, playbackProgressTestDurationSeconds)
	if item.State.ProgressSeconds != 0 || item.State.Watched {
		t.Fatalf("custom start threshold should reset 600 seconds: %#v", item.State)
	}

	item = writeOrderedPlaybackProgressForTest(t, db, client, serverURL, "movie_meridian", 800, playbackProgressTestDurationSeconds)
	if item.State.ProgressSeconds != 800 || item.State.Watched || item.State.Resume == nil {
		t.Fatalf("custom start threshold should keep 800 seconds: %#v", item.State)
	}

	item = writeOrderedPlaybackProgressForTest(t, db, client, serverURL, "movie_meridian", 6500, playbackProgressTestDurationSeconds)
	if item.State.ProgressSeconds != 0 || !item.State.Watched || item.State.Resume != nil {
		t.Fatalf("custom played threshold should mark watched and reset resume: %#v", item.State)
	}
}

func TestTrackProgressCreatesContinueListeningWithoutResumeOffset(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	if _, err := db.Exec(`UPDATE media_items SET source_url = 'https://media.example.com/track-mara-01.mp3' WHERE id = 'track_mara_01'`); err != nil {
		t.Fatalf("seed deterministic track source: %v", err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	item := writeOrderedPlaybackProgressForTest(t, db, client, serverURL, "track_mara_01", 80, 244)
	if item.State.ProgressSeconds != 0 || item.State.Resume != nil || item.State.LastPlayedAt == "" {
		t.Fatalf("track progress should record listening without a resume offset: %#v", item.State)
	}

	var home HomeResponse
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &home)
	if status != http.StatusOK {
		t.Fatalf("home status = %d, body: %s", status, body)
	}
	if row := homeRowByID(home, "continue_listening"); row == nil || !mediaIDsContain(row.Items, "track_mara_01") {
		t.Fatalf("continue listening should include the recently played track: %#v", home.Rows)
	}

	var playback PlaybackResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", map[string]any{
		"mediaId":          "track_mara_01",
		"clientInstanceId": "music-a",
		"startSeconds":     80,
		"skipPreroll":      true,
	}, &playback)
	if status != http.StatusOK {
		t.Fatalf("track playback start status = %d, body: %s", status, body)
	}
	if playback.Media.State.ProgressSeconds != 0 || playback.Media.State.Resume != nil {
		t.Fatalf("track playback should start from zero even with requested start seconds: %#v", playback.Media.State)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence":   1,
		"recordedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"state":           "playing",
		"progressSeconds": 80,
		"durationSeconds": 244,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("track heartbeat status = %d, body: %s", status, body)
	}
	var restored PlaybackRestoreResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/active", map[string]any{
		"clientInstanceId": "music-a",
		"clientProfile":    map[string]any{"supportsHls": true},
	}, &restored)
	if status != http.StatusOK {
		t.Fatalf("track restore status = %d, body: %s", status, body)
	}
	if !restored.Active || restored.Playback == nil {
		t.Fatalf("expected active track playback restore: %#v", restored)
	}
	if restored.Playback.Media.State.ProgressSeconds != 0 || restored.Playback.Media.State.Resume != nil {
		t.Fatalf("track restore should not seek into the track: %#v", restored.Playback.Media.State)
	}
}

func TestStalePlaybackSessionFinalizesProgressFromServerTelemetry(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	setMediaDurationForProgressTest(t, db, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, server, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)

	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &playback)
	if status != http.StatusOK {
		t.Fatalf("start playback status = %d, body: %s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback-sessions/"+playback.SessionID, map[string]any{
		"eventSequence":   1,
		"recordedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"state":           "playing",
		"progressSeconds": 6900,
		"durationSeconds": playbackProgressTestDurationSeconds,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("heartbeat status = %d, body: %s", status, body)
	}
	if _, err := db.Exec(`DELETE FROM user_media_state WHERE user_id = ? AND media_id = ?`, userID, "movie_meridian"); err != nil {
		t.Fatalf("clear media state before stale expiry: %v", err)
	}
	oldLastSeen := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE playback_sessions SET last_seen_at = ? WHERE id = ?`, oldLastSeen, playback.SessionID); err != nil {
		t.Fatalf("make playback session stale: %v", err)
	}

	if err := server.expireStalePlaybackSessions(time.Now().UTC()); err != nil {
		t.Fatalf("expire stale playback sessions: %v", err)
	}

	var item MediaItem
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("media detail status = %d, body: %s", status, body)
	}
	if item.State.ProgressSeconds != 0 || !item.State.Watched || item.State.Resume != nil {
		t.Fatalf("stale finalization should mark watched and reset resume: %#v", item.State)
	}

	var endedAt string
	if err := db.QueryRow(`SELECT ended_at FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&endedAt); err != nil {
		t.Fatalf("load ended playback session: %v", err)
	}
	if endedAt == "" {
		t.Fatalf("expected stale playback session to be stopped")
	}
	assertCount(t, db, `SELECT COUNT(*) FROM playback_media_grants WHERE playback_session_id = '`+playback.SessionID+`' AND revoked_at = ''`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM playback_session_continuation_credentials WHERE playback_session_id = '`+playback.SessionID+`' AND revoked_at = ''`, 0)
}

func TestUserRemoteBitrateLimitCapsPlaybackDecision(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	server.cfg.TrustedProxyCIDRs = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	if _, err := db.Exec(`UPDATE users SET remote_bitrate_limit_mbps = 5 WHERE id = ?`, userID); err != nil {
		t.Fatalf("set remote bitrate limit: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM media_streams WHERE media_id = 'movie_meridian'`); err != nil {
		t.Fatalf("clear streams: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title)
		VALUES
			('movie_meridian_policy_video', 'movie_meridian', 'video', 'h264', 0, 10000000, 1920, 1080, '1080p H.264'),
			('movie_meridian_policy_audio', 'movie_meridian', 'audio', 'aac', 2, 128000, 0, 0, 'AAC Stereo')`); err != nil {
		t.Fatalf("insert high bitrate streams: %v", err)
	}
	seedExactPlaybackFactsForFixture(t, server, "movie_meridian")

	var playback PlaybackResponse
	status, body := doJSONWithForwardedFor(t, client, http.MethodPost, serverURL+"/api/playback-sessions", map[string]any{
		"mediaId": "movie_meridian",
		"clientProfile": attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{
			SupportedContainers:  []string{"mp4"},
			SupportedVideoCodecs: []string{"h264"},
			SupportedAudioCodecs: []string{"aac"},
		}),
		"skipPreroll": true,
	}, &playback, "203.0.113.42")
	if status != http.StatusOK {
		t.Fatalf("start playback status=%d body=%s", status, body)
	}
	if !playback.Decision.RequiresTranscode || !containsString(playback.Decision.ReasonCodes, "video_constraint_exceeded") {
		t.Fatalf("playback decision = %#v", playback.Decision)
	}
}

func TestDeviceRemoteBitrateLimitCapsPlaybackDecision(t *testing.T) {
	serverURL, db, server := newAuthTestServerWithInstance(t)
	server.cfg.TrustedProxyCIDRs = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var devices ListResponse[Device]
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/devices", nil, &devices)
	if status != http.StatusOK || len(devices.Items) == 0 {
		t.Fatalf("devices status=%d body=%s devices=%#v", status, body, devices)
	}
	var updated Device
	limit := 5
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/devices/"+devices.Items[0].ID, DeviceUpdateRequest{RemoteBitrateLimitMbps: &limit}, &updated)
	if status != http.StatusOK || updated.RemoteBitrateLimitMbps != limit {
		t.Fatalf("device bitrate update status=%d body=%s updated=%#v", status, body, updated)
	}
	if _, err := db.Exec(`DELETE FROM media_streams WHERE media_id = 'movie_meridian'`); err != nil {
		t.Fatalf("clear streams: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title)
		VALUES
			('movie_meridian_device_policy_video', 'movie_meridian', 'video', 'h264', 0, 10000000, 1920, 1080, '1080p H.264'),
			('movie_meridian_device_policy_audio', 'movie_meridian', 'audio', 'aac', 2, 128000, 0, 0, 'AAC Stereo')`); err != nil {
		t.Fatalf("insert high bitrate streams: %v", err)
	}
	seedExactPlaybackFactsForFixture(t, server, "movie_meridian")

	var playback PlaybackResponse
	status, body = doJSONWithForwardedFor(t, client, http.MethodPost, serverURL+"/api/playback-sessions", map[string]any{
		"mediaId": "movie_meridian",
		"clientProfile": attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{
			SupportedContainers:  []string{"mp4"},
			SupportedVideoCodecs: []string{"h264"},
			SupportedAudioCodecs: []string{"aac"},
		}),
		"skipPreroll": true,
	}, &playback, "203.0.113.42")
	if status != http.StatusOK {
		t.Fatalf("start playback status=%d body=%s", status, body)
	}
	if !playback.Decision.RequiresTranscode || !containsString(playback.Decision.ReasonCodes, "video_constraint_exceeded") {
		t.Fatalf("playback decision = %#v", playback.Decision)
	}
}

func TestGlobalRemoteBitrateLimitCapsRemoteRoutePlaybackDecision(t *testing.T) {
	_, db, server := newAuthTestServerWithInstance(t)
	user, err := server.getUser(adminUserID(t, db))
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}

	upsertJSONSetting(t, db, remoteAccessSettingsKey, map[string]any{
		"enabled":                 true,
		"hostedBaseUrl":           "https://api.getportico.tv",
		"claimStatus":             "claimed",
		"serverId":                "srv_remote_cap",
		"assignedHostname":        "media.direct.getportico.tv",
		"publicPortMode":          "manual",
		"manualPublicPort":        443,
		"preferredRemoteAuthMode": "portico",
		"remoteBitrateLimitMbps":  5,
	})
	if _, err := db.Exec(`DELETE FROM media_streams WHERE media_id = 'movie_meridian'`); err != nil {
		t.Fatalf("clear streams: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title)
		VALUES
			('movie_meridian_global_remote_video', 'movie_meridian', 'video', 'h264', 0, 10000000, 1920, 1080, '1080p H.264'),
			('movie_meridian_global_remote_audio', 'movie_meridian', 'audio', 'aac', 2, 128000, 0, 0, 'AAC Stereo')`); err != nil {
		t.Fatalf("insert high bitrate streams: %v", err)
	}

	localPlayback := startPlaybackWithHostForTest(t, server, user, "", "movie_meridian")
	if localPlayback.Decision.RequiresTranscode {
		t.Fatalf("local playback should not use the remote route cap: %#v", localPlayback.Decision)
	}

	remotePlayback := startPlaybackWithHostForTest(t, server, user, "media.direct.getportico.tv", "movie_meridian")
	if !remotePlayback.Decision.RequiresTranscode || !containsString(remotePlayback.Decision.ReasonCodes, "video_constraint_exceeded") {
		t.Fatalf("remote playback did not use the global remote cap: %#v", remotePlayback.Decision)
	}
}

func startPlaybackWithHostForTest(t *testing.T, server *Server, user User, host string, mediaID string) PlaybackResponse {
	t.Helper()
	seedExactPlaybackFactsForFixture(t, server, mediaID)
	payload, err := json.Marshal(map[string]any{
		"mediaId": mediaID,
		"clientProfile": attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{
			SupportedContainers:  []string{"mp4"},
			SupportedVideoCodecs: []string{"h264"},
			SupportedAudioCodecs: []string{"aac"},
		}),
		"skipPreroll": true,
	})
	if err != nil {
		t.Fatalf("marshal playback request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/playback-sessions", bytes.NewReader(payload))
	if host != "" {
		req.Host = host
	} else {
		req.Host = "127.0.0.1"
		req.RemoteAddr = "127.0.0.1:50000"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeaderName, "1")
	rec := httptest.NewRecorder()
	server.handlePlaybackSessionCreate(rec, req, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("start playback status=%d body=%s", rec.Code, rec.Body.String())
	}
	var playback PlaybackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &playback); err != nil {
		t.Fatalf("decode playback response: %v\n%s", err, rec.Body.String())
	}
	return playback
}

func TestUserPlaybackStreamLimitRejectsAdditionalPlayback(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_saffron")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	userID := adminUserID(t, db)
	if _, err := db.Exec(`UPDATE users SET max_active_streams = 1 WHERE id = ?`, userID); err != nil {
		t.Fatalf("set playback stream limit: %v", err)
	}

	var first PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_meridian"), &first)
	if status != http.StatusOK {
		t.Fatalf("first playback status=%d body=%s", status, body)
	}

	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_saffron"), nil)
	if status != http.StatusTooManyRequests || !strings.Contains(body, "playback_session_limit") {
		t.Fatalf("second playback status=%d body=%s", status, body)
	}

	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/playback-sessions/"+first.SessionID, stoppedPlaybackRequest(first), nil)
	if status != http.StatusOK {
		t.Fatalf("end first playback status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", authenticatedPlaybackRuntimeRequest("movie_saffron"), nil)
	if status != http.StatusOK {
		t.Fatalf("playback after end status=%d body=%s", status, body)
	}
}

func TestUserActiveSessionLimitRejectsAdditionalLogin(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: jar}
	loginUser(t, adminClient, serverURL)

	var created User
	status, body := doJSON(t, adminClient, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username:               "limited",
		Email:                  "limited@example.test",
		DisplayName:            "Limited User",
		Password:               "Password1234",
		Role:                   "user",
		Permissions:            permissionsForRole("user"),
		LibraryIDs:             []string{},
		MaxActiveSessions:      1,
		RemoteBitrateLimitMbps: 8,
	}, &created)
	if status != http.StatusCreated {
		t.Fatalf("create limited user status=%d body=%s", status, body)
	}
	if created.MaxActiveSessions != 1 || created.RemoteBitrateLimitMbps != 8 {
		t.Fatalf("created policy fields = %#v", created)
	}

	firstJar, _ := cookiejar.New(nil)
	firstClient := &http.Client{Jar: firstJar}
	status, body = doJSON(t, firstClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "limited",
		"password": "Password1234",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("first limited login status=%d body=%s", status, body)
	}

	secondJar, _ := cookiejar.New(nil)
	secondClient := &http.Client{Jar: secondJar}
	status, body = doJSON(t, secondClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "limited",
		"password": "Password1234",
	}, nil)
	if status != http.StatusForbidden || !strings.Contains(body, "active_session_limit") {
		t.Fatalf("second limited login status=%d body=%s", status, body)
	}

	status, body = doJSON(t, firstClient, http.MethodPost, serverURL+"/api/auth/logout", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("logout limited user status=%d body=%s", status, body)
	}
	status, body = doJSON(t, secondClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "limited",
		"password": "Password1234",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("limited login after logout status=%d body=%s", status, body)
	}
}

func TestUserAccessScheduleBlocksAndAllowsLogin(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: jar}
	loginUser(t, adminClient, serverURL)

	now := time.Now().UTC()
	currentMinute := now.Hour()*60 + now.Minute()
	blockedStart := (currentMinute + 1438) % 1440
	blockedEnd := (currentMinute + 1439) % 1440
	allowedEnd := (currentMinute + 1) % 1440

	var created User
	status, body := doJSON(t, adminClient, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username:    "scheduled",
		Email:       "scheduled@example.test",
		DisplayName: "Scheduled User",
		Password:    "Password1234",
		Role:        "user",
		Permissions: permissionsForRole("user"),
		LibraryIDs:  []string{},
		AccessSchedule: UserAccessSchedule{
			Enabled:     true,
			StartMinute: blockedStart,
			EndMinute:   blockedEnd,
		},
	}, &created)
	if status != http.StatusCreated {
		t.Fatalf("create scheduled user status=%d body=%s", status, body)
	}
	if !created.AccessSchedule.Enabled || created.AccessSchedule.StartMinute != blockedStart || created.AccessSchedule.EndMinute != blockedEnd {
		t.Fatalf("created access schedule = %#v", created.AccessSchedule)
	}

	userJar, _ := cookiejar.New(nil)
	userClient := &http.Client{Jar: userJar}
	status, body = doJSON(t, userClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "scheduled",
		"password": "Password1234",
	}, nil)
	if status != http.StatusForbidden || !strings.Contains(body, "access_schedule_blocked") {
		t.Fatalf("blocked scheduled login status=%d body=%s", status, body)
	}

	var updated User
	status, body = doJSON(t, adminClient, http.MethodPatch, serverURL+"/api/users/"+created.ID, UserRequest{
		Username:    created.Username,
		Email:       created.Email,
		DisplayName: created.DisplayName,
		Role:        created.Role,
		Permissions: created.Permissions,
		LibraryIDs:  created.LibraryIDs,
		AccessSchedule: UserAccessSchedule{
			Enabled:     true,
			StartMinute: currentMinute,
			EndMinute:   allowedEnd,
		},
	}, &updated)
	if status != http.StatusOK {
		t.Fatalf("update scheduled user status=%d body=%s", status, body)
	}
	if updated.AccessSchedule.StartMinute != currentMinute || updated.AccessSchedule.EndMinute != allowedEnd {
		t.Fatalf("updated access schedule = %#v", updated.AccessSchedule)
	}

	status, body = doJSON(t, userClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "scheduled",
		"password": "Password1234",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("allowed scheduled login status=%d body=%s", status, body)
	}
}

func TestUserDevicePolicyAllowlistBlocksAndAllowsLogin(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: jar}
	loginUser(t, adminClient, serverURL)

	var created User
	status, body := doJSON(t, adminClient, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username:     "devicepolicy",
		Email:        "devicepolicy@example.test",
		DisplayName:  "Device Policy User",
		Password:     "Password1234",
		Role:         "user",
		Permissions:  permissionsForRole("user"),
		LibraryIDs:   []string{},
		DevicePolicy: UserDevicePolicy{Mode: "allowlist"},
	}, &created)
	if status != http.StatusCreated {
		t.Fatalf("create device policy user status=%d body=%s", status, body)
	}

	userJar, _ := cookiejar.New(nil)
	userClient := &http.Client{Jar: userJar}
	status, body = doJSONWithUserAgent(t, userClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "devicepolicy",
		"password": "Password1234",
	}, nil, "Portico Device Policy Test/1.0")
	if status != http.StatusForbidden || !strings.Contains(body, "device_not_allowed") {
		t.Fatalf("blocked device policy login status=%d body=%s", status, body)
	}

	var devices ListResponse[Device]
	status, body = doJSON(t, adminClient, http.MethodGet, serverURL+"/api/devices", nil, &devices)
	if status != http.StatusOK {
		t.Fatalf("devices status=%d body=%s", status, body)
	}
	deviceID := ""
	for _, device := range devices.Items {
		if device.UserID == created.ID {
			deviceID = device.ID
			break
		}
	}
	if deviceID == "" {
		t.Fatalf("blocked login did not record a device: %#v", devices.Items)
	}

	status, body = doJSON(t, adminClient, http.MethodPatch, serverURL+"/api/users/"+created.ID, UserRequest{
		Username:     created.Username,
		Email:        created.Email,
		DisplayName:  created.DisplayName,
		Role:         created.Role,
		Permissions:  created.Permissions,
		LibraryIDs:   created.LibraryIDs,
		DevicePolicy: UserDevicePolicy{Mode: "allowlist", AllowedDeviceIDs: []string{deviceID}},
	}, &created)
	if status != http.StatusOK {
		t.Fatalf("update device policy user status=%d body=%s", status, body)
	}

	status, body = doJSONWithUserAgent(t, userClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{
		"login":    "devicepolicy",
		"password": "Password1234",
	}, nil, "Portico Device Policy Test/1.0")
	if status != http.StatusOK {
		t.Fatalf("allowed device policy login status=%d body=%s", status, body)
	}
}

func TestAudiobookResumeIncludesCurrentChapter(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	now := time.Now().UTC().Format(time.RFC3339)
	userID := adminUserID(t, db)

	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_resume_books', 'Resume Books', 'audiobook', 91, '/tmp/resume-books', '{}', ?)`, now); err != nil {
		t.Fatalf("insert audiobook library: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES (?, 'lib_resume_books', ?)`, userID, now); err != nil {
		t.Fatalf("grant audiobook access: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, duration_seconds, genres_json, added_at, typed_metadata_json)
		VALUES ('book_resume', 'lib_resume_books', 'audiobook', 'Chaptered Book', 'Chaptered Book', 7200, '[]', ?, '{"author":"Author One","narrator":"Narrator One"}')`, now); err != nil {
		t.Fatalf("insert audiobook item: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_chapters (id, media_id, title, start_seconds, end_seconds, sort_order)
		VALUES
			('book_resume_chapter_1', 'book_resume', 'Opening', 0, 1800, 0),
			('book_resume_chapter_2', 'book_resume', 'The Middle', 1800, 3600, 1)`,
	); err != nil {
		t.Fatalf("insert audiobook chapters: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO user_media_state (profile_id, user_id, media_id, watched, progress_seconds, last_played_at, updated_at)
		VALUES (?, ?, 'book_resume', 0, 1900, ?, ?)`, userID, userID, now, now); err != nil {
		t.Fatalf("insert audiobook progress: %v", err)
	}

	var item MediaItem
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/media/book_resume", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("media detail status = %d, body: %s", status, body)
	}
	if item.State.Resume == nil || item.State.Resume.ChapterTitle != "The Middle" || item.State.Resume.ChapterIndex != 2 {
		t.Fatalf("resume info = %#v, expected current chapter", item.State.Resume)
	}
	if item.State.Resume.RemainingSeconds != 5300 {
		t.Fatalf("remaining seconds = %d, expected 5300", item.State.Resume.RemainingSeconds)
	}
}

func TestPlaybackReceiverLifecycle(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var receiver PlaybackReceiver
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers", PlaybackReceiverRequest{
		Name:              "Living Room Web",
		App:               "Portico Web",
		Platform:          "Browser",
		SupportedCommands: []string{"load", "pause", "load"},
	}, &receiver)
	if status != http.StatusCreated {
		t.Fatalf("create receiver status = %d, body: %s", status, body)
	}
	if receiver.ID == "" || receiver.Code == "" || receiver.Name != "Living Room Web" {
		t.Fatalf("unexpected receiver: %#v", receiver)
	}
	if receiver.App != "Portico Web" || receiver.Platform != "Browser" || len(receiver.SupportedCommands) != 1 || receiver.SupportedCommands[0] != "load" {
		t.Fatalf("unexpected receiver capabilities: %#v", receiver)
	}

	var receivers ListResponse[PlaybackReceiver]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/receivers", nil, &receivers)
	if status != http.StatusOK || len(receivers.Items) != 1 {
		t.Fatalf("list receivers status=%d body=%s receivers=%#v", status, body, receivers)
	}

	var viewer User
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username:    "receiver-viewer",
		Email:       "receiver-viewer@example.test",
		DisplayName: "Receiver Viewer",
		Password:    "Password1234",
		Role:        "user",
		Permissions: permissionsForRole("user"),
		LibraryIDs:  []string{"lib_movies"},
	}, &viewer)
	if status != http.StatusCreated {
		t.Fatalf("create receiver viewer status=%d body=%s", status, body)
	}
	viewerJar, _ := cookiejar.New(nil)
	viewerClient := &http.Client{Jar: viewerJar}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": "receiver-viewer", "password": "Password1234"}, nil)
	if status != http.StatusOK {
		t.Fatalf("receiver viewer login status=%d body=%s", status, body)
	}
	var viewerReceivers ListResponse[PlaybackReceiver]
	status, body = doJSON(t, viewerClient, http.MethodGet, serverURL+"/api/playback/receivers", nil, &viewerReceivers)
	if status != http.StatusOK || len(viewerReceivers.Items) != 0 {
		t.Fatalf("viewer receivers status=%d body=%s receivers=%#v", status, body, viewerReceivers)
	}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/command", PlaybackCommandRequest{Action: "load", MediaID: "movie_meridian"}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("viewer receiver command status=%d body=%s", status, body)
	}
	status, body = doJSON(t, viewerClient, http.MethodPatch, serverURL+"/api/playback/receivers/"+receiver.ID, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("viewer receiver heartbeat status=%d body=%s", status, body)
	}

	var targets ListResponse[PlaybackTarget]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/playback/targets", nil, &targets)
	if status != http.StatusOK || len(targets.Items) != 1 || targets.Items[0].Type != "receiver" {
		t.Fatalf("targets status=%d body=%s targets=%#v", status, body, targets)
	}
	if len(targets.Items[0].SupportedCommands) != 1 || targets.Items[0].SupportedCommands[0] != "load" || !strings.Contains(targets.Items[0].Detail, "Portico Web") {
		t.Fatalf("receiver target did not expose normalized capabilities: %#v", targets.Items[0])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/playback/receivers/"+receiver.ID+"/events", nil)
	if err != nil {
		t.Fatalf("receiver events request: %v", err)
	}
	eventsResp, err := client.Do(eventsReq)
	if err != nil {
		t.Fatalf("receiver events response: %v", err)
	}
	defer eventsResp.Body.Close()
	if eventsResp.StatusCode != http.StatusOK || !strings.Contains(eventsResp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("receiver events status=%d content-type=%s", eventsResp.StatusCode, eventsResp.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(eventsResp.Body)
	initialEvent := readSSEDataLine(t, reader)
	if !strings.Contains(initialEvent, receiver.ID) || !strings.Contains(initialEvent, "Portico Web") {
		t.Fatalf("receiver initial event data = %s", initialEvent)
	}

	var command PlaybackCommand
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback/receivers/"+receiver.ID+"/command", PlaybackCommandRequest{Action: "load", MediaID: "movie_meridian", PositionSeconds: 33}, &command)
	if status != http.StatusOK {
		t.Fatalf("receiver command status = %d, body: %s", status, body)
	}
	if command.Action != "load" || command.MediaID != "movie_meridian" || command.PositionSeconds != 33 {
		t.Fatalf("unexpected receiver command: %#v", command)
	}
	commandEvent := readSSEDataLine(t, reader)
	if !strings.Contains(commandEvent, command.ID) || !strings.Contains(commandEvent, "movie_meridian") {
		t.Fatalf("receiver command event data = %s", commandEvent)
	}

	var heartbeat PlaybackReceiver
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/playback/receivers/"+receiver.ID, nil, &heartbeat)
	if status != http.StatusOK {
		t.Fatalf("receiver heartbeat status = %d, body: %s", status, body)
	}
	if heartbeat.Command.ID != command.ID || heartbeat.Command.MediaID != "movie_meridian" {
		t.Fatalf("heartbeat did not return command: %#v", heartbeat)
	}
}

func TestWatchWithFriendsGroupLifecycleRequiresPermissionAndMembership(t *testing.T) {
	serverURL := newAuthTestServer(t)
	ownerJar, _ := cookiejar.New(nil)
	ownerClient := &http.Client{Jar: ownerJar}
	loginUser(t, ownerClient, serverURL)

	var created User
	status, body := doJSON(t, ownerClient, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username:    "syncviewer",
		Email:       "syncviewer@example.test",
		DisplayName: "Sync Viewer",
		Password:    "Password1234",
		Role:        "user",
		Permissions: permissionsForRole("user"),
		LibraryIDs:  []string{"lib_movies"},
	}, &created)
	if status != http.StatusCreated {
		t.Fatalf("create sync viewer status=%d body=%s", status, body)
	}

	viewerJar, _ := cookiejar.New(nil)
	viewerClient := &http.Client{Jar: viewerJar}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": "syncviewer", "password": "Password1234"}, nil)
	if status != http.StatusOK {
		t.Fatalf("viewer login status=%d body=%s", status, body)
	}
	status, body = doJSON(t, viewerClient, http.MethodGet, serverURL+"/api/watch-with-friends/groups", nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("viewer without watch_with_friends status=%d body=%s", status, body)
	}

	permissions := permissionsForRole("user")
	permissions["watchWithFriends"] = true
	status, body = doJSON(t, ownerClient, http.MethodPatch, serverURL+"/api/users/"+created.ID, UserRequest{
		Username:    created.Username,
		Email:       created.Email,
		DisplayName: created.DisplayName,
		Role:        created.Role,
		Permissions: permissions,
		LibraryIDs:  created.LibraryIDs,
	}, &created)
	if status != http.StatusOK {
		t.Fatalf("grant watch_with_friends status=%d body=%s", status, body)
	}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": "syncviewer", "password": "Password1234"}, nil)
	if status != http.StatusOK {
		t.Fatalf("viewer reauthentication after authority change status=%d body=%s", status, body)
	}

	var group WatchWithFriendsGroup
	status, body = doJSON(t, ownerClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups", WatchWithFriendsCreateRequest{MediaID: "movie_meridian", Name: "Movie Night"}, &group)
	if status != http.StatusCreated {
		t.Fatalf("create watch_with_friends group status=%d body=%s", status, body)
	}
	if group.ID == "" || group.MediaID != "movie_meridian" || len(group.Members) != 1 {
		t.Fatalf("unexpected watch_with_friends group: %#v", group)
	}

	var groups ListResponse[WatchWithFriendsGroup]
	status, body = doJSON(t, viewerClient, http.MethodGet, serverURL+"/api/watch-with-friends/groups", nil, &groups)
	if status != http.StatusOK || groups.Total != 1 {
		t.Fatalf("viewer list watch_with_friends status=%d body=%s groups=%#v", status, body, groups)
	}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/join", nil, &group)
	if status != http.StatusOK || len(group.Members) != 2 {
		t.Fatalf("viewer join watch_with_friends status=%d body=%s group=%#v", status, body, group)
	}
	ctx, cancel := context.WithCancel(context.Background())
	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/events", nil)
	if err != nil {
		t.Fatalf("watch_with_friends events request: %v", err)
	}
	eventsResp, err := viewerClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("watch_with_friends events response: %v", err)
	}
	if eventsResp.StatusCode != http.StatusOK || !strings.Contains(eventsResp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("watch_with_friends events status=%d content-type=%s", eventsResp.StatusCode, eventsResp.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(eventsResp.Body)
	var eventData string
	for line := ""; !strings.HasPrefix(line, "data: "); {
		line, err = reader.ReadString('\n')
		if err != nil {
			t.Fatalf("watch_with_friends event read: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			eventData = strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			break
		}
	}
	cancel()
	_ = eventsResp.Body.Close()
	if !strings.Contains(eventData, group.ID) || !strings.Contains(eventData, "movie_meridian") {
		t.Fatalf("watch_with_friends event data = %s", eventData)
	}
	status, body = doJSON(t, viewerClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/member/state", WatchWithFriendsMemberStateRequest{State: "ready", PositionSeconds: 41}, &group)
	if status != http.StatusOK {
		t.Fatalf("viewer member state status=%d body=%s", status, body)
	}
	viewerReady := false
	for _, member := range group.Members {
		if member.ProfileID == viewerProfileID(created) && member.State == "ready" && member.PositionSeconds == 41 {
			viewerReady = true
		}
	}
	if !viewerReady {
		t.Fatalf("viewer ready member state missing: %#v", group.Members)
	}
	status, body = doJSON(t, ownerClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", WatchWithFriendsQueueRequest{MediaID: "movie_saffron", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "flow-add-saffron"}, &group)
	if status != http.StatusOK || len(group.Queue) != 2 || group.Queue[1].MediaID != "movie_saffron" {
		t.Fatalf("owner add queue status=%d body=%s queue=%#v", status, body, group.Queue)
	}
	status, body = doJSON(t, ownerClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", WatchWithFriendsQueueRequest{MediaID: "movie_neon", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "flow-add-neon"}, &group)
	if status != http.StatusOK || len(group.Queue) != 3 || group.Queue[2].MediaID != "movie_neon" {
		t.Fatalf("owner add second queue status=%d body=%s queue=%#v", status, body, group.Queue)
	}
	status, body = doJSON(t, ownerClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", WatchWithFriendsQueueOrderRequest{MediaIDs: []string{"movie_meridian", "movie_neon", "movie_saffron"}, ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "flow-reorder"}, &group)
	if status != http.StatusOK || len(group.Queue) != 3 || group.Queue[1].MediaID != "movie_neon" || group.Queue[2].MediaID != "movie_saffron" {
		t.Fatalf("owner reorder queue status=%d body=%s queue=%#v", status, body, group.Queue)
	}
	status, body = doJSON(t, ownerClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", WatchWithFriendsStateRequest{Action: "next", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "flow-next"}, &group)
	if status != http.StatusOK || group.MediaID != "movie_neon" || group.Command.Action != "load" || group.Command.MediaID != "movie_neon" {
		t.Fatalf("owner next watch_with_friends status=%d body=%s group=%#v", status, body, group)
	}
	status, body = doJSON(t, ownerClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", WatchWithFriendsStateRequest{Action: "previous", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "flow-previous"}, &group)
	if status != http.StatusOK || group.MediaID != "movie_meridian" || group.Command.Action != "load" || group.Command.MediaID != "movie_meridian" {
		t.Fatalf("owner previous watch_with_friends status=%d body=%s group=%#v", status, body, group)
	}
	shuffleEnabled := true
	status, body = doJSON(t, ownerClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/settings", WatchWithFriendsSettingsRequest{ShuffleEnabled: &shuffleEnabled, RepeatMode: "all", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "flow-settings-all"}, &group)
	if status != http.StatusOK || !group.ShuffleEnabled || group.RepeatMode != "all" {
		t.Fatalf("owner watch_with_friends settings status=%d body=%s group=%#v", status, body, group)
	}
	shuffleEnabled = false
	status, body = doJSON(t, ownerClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/settings", WatchWithFriendsSettingsRequest{ShuffleEnabled: &shuffleEnabled, ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "flow-settings-shuffle"}, &group)
	if status != http.StatusOK || group.ShuffleEnabled || group.RepeatMode != "all" {
		t.Fatalf("owner watch_with_friends shuffle off status=%d body=%s group=%#v", status, body, group)
	}
	status, body = doJSON(t, ownerClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", WatchWithFriendsStateRequest{Action: "previous", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "flow-repeat-previous"}, &group)
	if status != http.StatusOK || group.MediaID != "movie_saffron" || group.Command.Action != "load" || group.Command.MediaID != "movie_saffron" {
		t.Fatalf("owner repeat previous watch_with_friends status=%d body=%s group=%#v", status, body, group)
	}
	status, body = doJSON(t, ownerClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", WatchWithFriendsStateRequest{Action: "next", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "flow-repeat-next"}, &group)
	if status != http.StatusOK || group.MediaID != "movie_meridian" || group.Command.Action != "load" || group.Command.MediaID != "movie_meridian" {
		t.Fatalf("owner repeat next watch_with_friends status=%d body=%s group=%#v", status, body, group)
	}
	status, body = doJSON(t, ownerClient, http.MethodDelete, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue/movie_saffron?expectedRevision="+strconv.FormatInt(group.Revision, 10)+"&idempotencyKey=flow-remove-saffron", nil, &group)
	if status != http.StatusOK || len(group.Queue) != 2 || group.Queue[0].MediaID != "movie_meridian" {
		t.Fatalf("owner remove queue status=%d body=%s queue=%#v", status, body, group.Queue)
	}
	status, body = doJSON(t, ownerClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", WatchWithFriendsStateRequest{Action: "play", PositionSeconds: 42, ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "flow-play"}, &group)
	if status != http.StatusOK || group.State != "playing" || group.PositionSeconds != 42 || group.Command.Action != "play" {
		t.Fatalf("owner watch_with_friends state status=%d body=%s group=%#v", status, body, group)
	}
	status, body = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/leave", nil, &group)
	if status != http.StatusOK {
		t.Fatalf("viewer leave watch_with_friends status=%d body=%s", status, body)
	}
	var notFoundProblem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	status, body = doJSON(t, viewerClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/state", WatchWithFriendsStateRequest{Action: "pause", PositionSeconds: 43, ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "viewer-pause"}, &notFoundProblem)
	if status != http.StatusNotFound || notFoundProblem.Code != "watch_with_friends_not_found" || notFoundProblem.Detail != "Watch With Friends group was not found." {
		t.Fatalf("viewer state after leave status=%d body=%s problem=%#v", status, body, notFoundProblem)
	}
	status, body = doJSON(t, ownerClient, http.MethodDelete, serverURL+"/api/watch-with-friends/groups/"+group.ID+"?expectedRevision="+strconv.FormatInt(group.Revision, 10)+"&idempotencyKey=flow-end", nil, &group)
	if status != http.StatusOK || group.State != "stopped" {
		t.Fatalf("owner delete watch_with_friends status=%d body=%s group=%#v", status, body, group)
	}
}

func TestWatchWithFriendsHostMutationsRequireOwnership(t *testing.T) {
	serverURL := newAuthTestServer(t)
	ownerJar, _ := cookiejar.New(nil)
	ownerClient := &http.Client{Jar: ownerJar}
	loginUser(t, ownerClient, serverURL)

	createUser := func(username, displayName, role string, permissions map[string]bool) User {
		t.Helper()
		var created User
		status, body := doJSON(t, ownerClient, http.MethodPost, serverURL+"/api/users", UserRequest{
			Username:    username,
			Email:       username + "@example.test",
			DisplayName: displayName,
			Password:    "Password1234",
			Role:        role,
			Permissions: permissions,
			LibraryIDs:  []string{"lib_movies"},
		}, &created)
		if status != http.StatusCreated {
			t.Fatalf("create %s status=%d body=%s", username, status, body)
		}
		return created
	}
	login := func(username string) *http.Client {
		t.Helper()
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar}
		status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": username, "password": "Password1234"}, nil)
		if status != http.StatusOK {
			t.Fatalf("login %s status=%d body=%s", username, status, body)
		}
		return client
	}

	memberPermissions := permissionsForRole("user")
	memberPermissions["watchWithFriends"] = true
	member := createUser("sync-member", "Sync Member", "user", memberPermissions)
	observerPermissions := permissionsForRole("user")
	observerPermissions["watchWithFriends"] = true
	manager := createUser("sync-observer", "Sync Observer", "user", observerPermissions)
	memberClient := login(member.Username)
	managerClient := login(manager.Username)

	var group WatchWithFriendsGroup
	status, body := doJSON(t, ownerClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups", WatchWithFriendsCreateRequest{MediaID: "movie_meridian", Name: "Owner Controls"}, &group)
	if status != http.StatusCreated {
		t.Fatalf("create Watch With Friends group status=%d body=%s", status, body)
	}
	status, body = doJSON(t, memberClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/join", nil, &group)
	if status != http.StatusOK {
		t.Fatalf("member join status=%d body=%s", status, body)
	}
	status, body = doJSON(t, memberClient, http.MethodPatch, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/member/state", WatchWithFriendsMemberStateRequest{State: "ready", PositionSeconds: 12}, &group)
	if status != http.StatusOK {
		t.Fatalf("member readiness status=%d body=%s", status, body)
	}
	status, body = doJSON(t, ownerClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", WatchWithFriendsQueueRequest{MediaID: "movie_saffron", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "ownership-add-saffron"}, &group)
	if status != http.StatusOK {
		t.Fatalf("owner add first queue item status=%d body=%s", status, body)
	}
	status, body = doJSON(t, ownerClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue", WatchWithFriendsQueueRequest{MediaID: "movie_neon", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "ownership-add-neon"}, &group)
	if status != http.StatusOK {
		t.Fatalf("owner add second queue item status=%d body=%s", status, body)
	}

	shuffleEnabled := true
	mutations := []struct {
		name    string
		method  string
		path    string
		payload any
	}{
		{name: "playback state", method: http.MethodPatch, path: "/state", payload: WatchWithFriendsStateRequest{Action: "play", PositionSeconds: 32, ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "forbidden-state"}},
		{name: "playback settings", method: http.MethodPatch, path: "/settings", payload: WatchWithFriendsSettingsRequest{ShuffleEnabled: &shuffleEnabled, RepeatMode: "all", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "forbidden-settings"}},
		{name: "queue add", method: http.MethodPost, path: "/queue", payload: WatchWithFriendsQueueRequest{MediaID: "movie_saffron", ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "forbidden-add"}},
		{name: "queue reorder", method: http.MethodPatch, path: "/queue", payload: WatchWithFriendsQueueOrderRequest{MediaIDs: []string{"movie_meridian", "movie_neon", "movie_saffron"}, ExpectedRevision: watchWithFriendsRevisionPtr(group.Revision), IdempotencyKey: "forbidden-reorder"}},
		{name: "queue remove", method: http.MethodDelete, path: "/queue/movie_saffron?expectedRevision=" + strconv.FormatInt(group.Revision, 10) + "&idempotencyKey=forbidden-remove"},
	}
	for _, mutation := range mutations {
		t.Run("member cannot mutate "+mutation.name, func(t *testing.T) {
			var problem struct {
				Type      string `json:"type"`
				Status    int    `json:"status"`
				Code      string `json:"code"`
				Detail    string `json:"detail"`
				RequestID string `json:"requestId"`
			}
			status, body := doJSON(t, memberClient, mutation.method, serverURL+"/api/watch-with-friends/groups/"+group.ID+mutation.path, mutation.payload, &problem)
			if status != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", status, body)
			}
			if problem.Status != http.StatusForbidden || problem.Code != "watch_with_friends_host_required" || problem.Detail != errWatchWithFriendsHostRequired.Error() || problem.RequestID == "" || problem.Type != "https://portico.media/problems/watch-with-friends-host-required" {
				t.Fatalf("unexpected problem details: %#v body=%s", problem, body)
			}
		})
	}

	status, body = doJSON(t, ownerClient, http.MethodGet, serverURL+"/api/watch-with-friends/groups/"+group.ID, nil, &group)
	if status != http.StatusOK || group.State != "paused" || group.ShuffleEnabled || group.RepeatMode != "none" || len(group.Queue) != 3 || group.Queue[1].MediaID != "movie_saffron" {
		t.Fatalf("forbidden mutations changed group status=%d body=%s group=%#v", status, body, group)
	}

	status, body = doJSON(t, managerClient, http.MethodDelete, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/queue/movie_saffron?expectedRevision="+strconv.FormatInt(group.Revision, 10)+"&idempotencyKey=manager-remove", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("server manager viewer route must not expose or control a private group status=%d body=%s", status, body)
	}
}

func TestWatchWithFriendsPrivacyPreferencesGateDiscoveryAndMemberVisibility(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}
	loginUser(t, adminClient, serverURL)

	createSyncUser := func(username, displayName string) User {
		t.Helper()
		permissions := permissionsForRole("user")
		permissions["watchWithFriends"] = true
		var created User
		status, body := doJSON(t, adminClient, http.MethodPost, serverURL+"/api/users", UserRequest{
			Username:    username,
			Email:       username + "@example.test",
			DisplayName: displayName,
			Password:    "Password1234",
			Role:        "user",
			Permissions: permissions,
			LibraryIDs:  []string{"lib_movies"},
		}, &created)
		if status != http.StatusCreated {
			t.Fatalf("create %s status=%d body=%s", username, status, body)
		}
		return created
	}
	loginSyncUser := func(username string) *http.Client {
		t.Helper()
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar}
		status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": username, "password": "Password1234"}, nil)
		if status != http.StatusOK {
			t.Fatalf("login %s status=%d body=%s", username, status, body)
		}
		return client
	}

	host := createSyncUser("sync-host", "Sync Host")
	guest := createSyncUser("sync-guest", "Sync Guest")
	hostClient := loginSyncUser(host.Username)
	guestClient := loginSyncUser(guest.Username)

	var group WatchWithFriendsGroup
	status, body := doJSON(t, hostClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups", WatchWithFriendsCreateRequest{MediaID: "movie_meridian", Name: "Private Movie Night"}, &group)
	if status != http.StatusCreated {
		t.Fatalf("host create watch_with_friends group status=%d body=%s", status, body)
	}

	setUserPrivacyPreferencesForTest(t, db, guest.ID, UserPrivacyPreferences{
		PauseWatchHistory:         false,
		ShowActivityToMembers:     true,
		IncludeInWatchWithFriends: false,
	})
	var groups ListResponse[WatchWithFriendsGroup]
	status, body = doJSON(t, guestClient, http.MethodGet, serverURL+"/api/watch-with-friends/groups", nil, &groups)
	if status != http.StatusOK || groups.Total != 0 {
		t.Fatalf("guest with Watch With Friends disabled should not discover groups status=%d body=%s groups=%#v", status, body, groups)
	}
	status, body = doJSON(t, guestClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/join", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("guest with Watch With Friends disabled should not join status=%d body=%s", status, body)
	}

	setUserPrivacyPreferencesForTest(t, db, guest.ID, UserPrivacyPreferences{
		PauseWatchHistory:         false,
		ShowActivityToMembers:     false,
		IncludeInWatchWithFriends: true,
	})
	status, body = doJSON(t, guestClient, http.MethodPost, serverURL+"/api/watch-with-friends/groups/"+group.ID+"/join", nil, &group)
	if status != http.StatusOK || len(group.Members) != 2 {
		t.Fatalf("guest should join after re-enabling Watch With Friends status=%d body=%s group=%#v", status, body, group)
	}

	var hostView WatchWithFriendsGroup
	status, body = doJSON(t, hostClient, http.MethodGet, serverURL+"/api/watch-with-friends/groups/"+group.ID, nil, &hostView)
	if status != http.StatusOK {
		t.Fatalf("host load group status=%d body=%s", status, body)
	}
	if len(hostView.Members) != 1 || hostView.Members[0].ProfileID != viewerProfileID(host) {
		t.Fatalf("host should not see hidden guest activity: %#v", hostView.Members)
	}

	var managerView WatchWithFriendsGroup
	status, body = doJSON(t, adminClient, http.MethodGet, serverURL+"/api/watch-with-friends/groups/"+group.ID, nil, &managerView)
	if status != http.StatusOK || managerView.Permissions.CanControl || managerView.Permissions.CanManageQueue || managerView.Permissions.IsHost {
		t.Fatalf("server manager must be treated as an ordinary discoverable viewer without host permissions status=%d body=%s view=%#v", status, body, managerView)
	}
}

func TestWatchWithFriendsQueueReorderRejectsOversizedRequests(t *testing.T) {
	ids := make([]string, maxPlaybackQueueItems+1)
	for index := range ids {
		ids[index] = fmt.Sprintf("wwf_queue_%03d", index)
	}
	_, err := (&Server{}).reorderWatchWithFriendsQueue(User{}, "missing", WatchWithFriendsQueueOrderRequest{MediaIDs: ids})
	if err == nil || !strings.Contains(err.Error(), "limited") {
		t.Fatalf("oversized Watch With Friends queue reorder should fail before loading group, got %v", err)
	}
}

func TestFilesystemBrowseResolvesSymlinkDirectoryEntries(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/filesystem/browse?path="+url.QueryEscape(root), nil)
	recorder := httptest.NewRecorder()
	server.handleFilesystemBrowse(recorder, req, User{ID: "usr_owner", AccountID: "usr_owner", ProfileID: "usr_owner", ProfileIsPrimary: true, Role: "owner", AuthProvider: "local", Permissions: ownerPermissions()})
	if recorder.Code != http.StatusOK {
		t.Fatalf("browse status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response FilesystemBrowseResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("parse browse response: %v", err)
	}
	for _, entry := range response.Entries {
		if entry.Name == "linked" {
			if entry.Path != resolvedTarget {
				t.Fatalf("symlink entry path = %s, expected resolved target %s", entry.Path, resolvedTarget)
			}
			return
		}
	}
	t.Fatalf("symlink directory entry missing: %#v", response.Entries)
}

func TestFilesystemBrowseRequiresServerManagementPermission(t *testing.T) {
	server := newScannerTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/filesystem/browse?path="+url.QueryEscape(t.TempDir()), nil)
	recorder := httptest.NewRecorder()
	server.handleFilesystemBrowse(recorder, req, User{ID: "usr_library_manager", Role: "user", AuthProvider: "local", Permissions: map[string]bool{"manageLibraries": true}})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("library manager browse status = %d, expected 403", recorder.Code)
	}
}

func TestFilesystemBrowseRequiresOwnerRole(t *testing.T) {
	server := newScannerTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/filesystem/browse?path="+url.QueryEscape(t.TempDir()), nil)
	recorder := httptest.NewRecorder()
	server.handleFilesystemBrowse(recorder, req, User{ID: "usr_user", Role: "user", AuthProvider: "local", Permissions: map[string]bool{"manageServer": true}})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("admin browse status = %d, expected 403", recorder.Code)
	}
}

func TestFilesystemCreateDirectoryCreatesOnlyFinalFolder(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	target := filepath.Join(root, "New Folder")
	req := newFilesystemCreateDirectoryRequest(t, target)
	recorder := httptest.NewRecorder()
	server.handleFilesystemDirectories(recorder, req, User{ID: "usr_owner", AccountID: "usr_owner", ProfileID: "usr_owner", ProfileIsPrimary: true, Role: "owner", AuthProvider: "local", Permissions: ownerPermissions()})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create directory status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("created directory missing: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("created path is not a directory")
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve created directory: %v", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve parent directory: %v", err)
	}
	var response FilesystemBrowseResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("parse create directory response: %v", err)
	}
	if response.Path != resolvedTarget {
		t.Fatalf("created response path = %s, expected %s", response.Path, resolvedTarget)
	}
	if response.Parent != resolvedRoot {
		t.Fatalf("created response parent = %s, expected %s", response.Parent, resolvedRoot)
	}
}

func TestFilesystemCreateDirectoryRejectsUnsafePaths(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	tests := []struct {
		name     string
		path     string
		wantCode int
		wantErr  string
	}{
		{name: "empty path", path: "", wantCode: http.StatusBadRequest, wantErr: "invalid_path"},
		{name: "root path", path: string(filepath.Separator), wantCode: http.StatusBadRequest, wantErr: "invalid_path"},
		{name: "relative path", path: "media/new-folder", wantCode: http.StatusBadRequest, wantErr: "invalid_path"},
		{name: "missing parent", path: filepath.Join(root, "missing", "child"), wantCode: http.StatusBadRequest, wantErr: "parent_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newFilesystemCreateDirectoryRequest(t, tt.path)
			recorder := httptest.NewRecorder()
			server.handleFilesystemDirectories(recorder, req, User{ID: "usr_owner", AccountID: "usr_owner", ProfileID: "usr_owner", ProfileIsPrimary: true, Role: "owner", AuthProvider: "local", Permissions: ownerPermissions()})
			if recorder.Code != tt.wantCode {
				t.Fatalf("create directory status = %d, expected %d, body=%s", recorder.Code, tt.wantCode, recorder.Body.String())
			}
			if code := responseErrorCode(t, recorder.Body.Bytes()); code != tt.wantErr {
				t.Fatalf("create directory error code = %q, expected %q", code, tt.wantErr)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing parent was created unexpectedly: %v", err)
	}
}

func TestFilesystemCreateDirectoryRequiresOwnerRole(t *testing.T) {
	server := newScannerTestServer(t)
	req := newFilesystemCreateDirectoryRequest(t, filepath.Join(t.TempDir(), "new-folder"))
	recorder := httptest.NewRecorder()
	server.handleFilesystemDirectories(recorder, req, User{ID: "usr_user", Role: "user", AuthProvider: "local", Permissions: map[string]bool{"manageServer": true}})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("admin create directory status = %d, expected 403", recorder.Code)
	}
}

func newFilesystemCreateDirectoryRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	body, err := json.Marshal(FilesystemCreateDirectoryRequest{Path: path})
	if err != nil {
		t.Fatalf("marshal create directory request: %v", err)
	}
	return httptest.NewRequest(http.MethodPost, "/api/filesystem/directories", bytes.NewReader(body))
}

func responseErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("parse error response: %v", err)
	}
	return payload.Code
}

func TestBackupCatalogAndAudit(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var backup BackupInfo
	var status int
	var body string
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/backups", nil, &backup)
		if status == http.StatusCreated || time.Now().After(deadline) {
			break
		}
		if status != http.StatusInternalServerError || !strings.Contains(body, `"code":"backup_failed"`) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status != http.StatusCreated {
		t.Fatalf("create backup status = %d, body: %s", status, body)
	}
	if !backup.RestoreReady || backup.Integrity != "ok" {
		t.Fatalf("backup was not restore ready: %#v", backup)
	}
	if !backup.ManifestPresent || backup.ChecksumSHA256 == "" || backup.DatabaseFormatVersion == 0 || backup.MigrationHead == "" {
		t.Fatalf("backup manifest evidence missing: %#v", backup)
	}
	var audit ListResponse[AuditEvent]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/audit-events", nil, &audit)
	if status != http.StatusOK {
		t.Fatalf("audit status = %d, body: %s", status, body)
	}
	found := false
	for _, event := range audit.Items {
		if event.Action == "backup.created" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected backup.created audit event, got %#v", audit.Items)
	}
}

func TestSystemDiagnosticsReportsRuntimeReadiness(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var diagnostics SystemDiagnostics
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/system/diagnostics", nil, &diagnostics)
	if status != http.StatusOK {
		t.Fatalf("diagnostics status = %d, body: %s", status, body)
	}
	if diagnostics.Version == "" || diagnostics.GOOS == "" || diagnostics.GOARCH == "" {
		t.Fatalf("diagnostics missing runtime identity: %#v", diagnostics)
	}
	if !diagnostics.AppDataReady || !diagnostics.DatabaseReady {
		t.Fatalf("diagnostics missing app data/database readiness: %#v", diagnostics)
	}
	if diagnostics.Startup.StartedAt == "" || diagnostics.Startup.Status == "" || len(diagnostics.Startup.Phases) == 0 {
		t.Fatalf("diagnostics missing startup state: %#v", diagnostics.Startup)
	}
	if diagnostics.Runtime.StartedAt == "" || diagnostics.Runtime.Goroutines <= 0 || diagnostics.Runtime.HeapSysBytes <= 0 {
		t.Fatalf("diagnostics missing runtime pressure metrics: %#v", diagnostics.Runtime)
	}
	if diagnostics.Runtime.IOPressure.Status == "" || diagnostics.Runtime.IOPressure.Samples <= 0 {
		t.Fatalf("diagnostics missing io pressure metrics: %#v", diagnostics.Runtime.IOPressure)
	}
	if diagnostics.Resources.Status == "" || diagnostics.Resources.MaxTranscodeSessions <= 0 || diagnostics.Resources.SQLiteMaxOpenConnections <= 0 {
		t.Fatalf("diagnostics missing resource pressure metrics: %#v", diagnostics.Resources)
	}
	if diagnostics.Admission.SearchCapacityPerUser != maxConcurrentSearchesPerUser ||
		diagnostics.Admission.SearchCapacityGlobal != maxConcurrentSearchesGlobal ||
		diagnostics.Admission.DownloadCapacityPerUser != maxConcurrentDownloadsPerUser ||
		diagnostics.Admission.StreamCapacityPerUser != maxConcurrentStreamsPerUser ||
		diagnostics.Admission.TranscodeCapacity <= 0 ||
		diagnostics.Admission.TranscodeCapacityPerUser <= 0 {
		t.Fatalf("diagnostics missing admission metrics: %#v", diagnostics.Admission)
	}
	if diagnostics.SQLite.MaxOpenConnections != 8 {
		t.Fatalf("diagnostics sqlite pool max = %d, expected bounded preset 8", diagnostics.SQLite.MaxOpenConnections)
	}
	if diagnostics.SQLite.OpenConnections <= 0 {
		t.Fatalf("diagnostics missing sqlite connection stats: %#v", diagnostics.SQLite)
	}
	if diagnostics.SQLite.DatabaseBytes <= 0 {
		t.Fatalf("diagnostics missing sqlite file size: %#v", diagnostics.SQLite)
	}
	if strings.ToLower(diagnostics.SQLite.JournalMode) != "wal" {
		t.Fatalf("diagnostics sqlite journal mode = %q, expected wal", diagnostics.SQLite.JournalMode)
	}
	if diagnostics.SQLite.WALAutoCheckpointPages <= 0 {
		t.Fatalf("diagnostics missing sqlite wal autocheckpoint setting: %#v", diagnostics.SQLite)
	}
	if len(diagnostics.JobLanes) != len(jobLaneDefinitions()) {
		t.Fatalf("diagnostics job lanes = %d, expected %d: %#v", len(diagnostics.JobLanes), len(jobLaneDefinitions()), diagnostics.JobLanes)
	}
	foundMaintenance := false
	foundWriteHeavy := false
	foundMetadata := false
	foundAnalysis := false
	for _, lane := range diagnostics.JobLanes {
		if lane.ID == "maintenance" {
			foundMaintenance = true
			if lane.Capacity != 1 {
				t.Fatalf("maintenance lane capacity = %d, expected 1", lane.Capacity)
			}
			if lane.Queued < 0 || lane.Running < 0 {
				t.Fatalf("maintenance lane has invalid backlog counts: %#v", lane)
			}
		}
		if lane.ID == jobLaneWriteHeavy {
			foundWriteHeavy = true
		}
		if lane.ID == jobLaneMetadata {
			foundMetadata = true
		}
		if lane.ID == jobLaneAnalysis {
			foundAnalysis = true
		}
	}
	if !foundMaintenance {
		t.Fatalf("diagnostics missing maintenance lane: %#v", diagnostics.JobLanes)
	}
	if !foundWriteHeavy || !foundMetadata || !foundAnalysis {
		t.Fatalf("diagnostics missing split job lanes: %#v", diagnostics.JobLanes)
	}
	if len(diagnostics.WorkloadLanes) != 12 {
		t.Fatalf("diagnostics workload lanes = %d, expected 12: %#v", len(diagnostics.WorkloadLanes), diagnostics.WorkloadLanes)
	}
	foundBrowsing := false
	foundExpensive := false
	foundAdminHeavy := false
	foundDLNA := false
	for _, lane := range diagnostics.WorkloadLanes {
		if lane.ID == workloadLaneBrowsing {
			foundBrowsing = true
			if lane.Capacity <= 0 || lane.Active < 0 {
				t.Fatalf("browsing workload lane has invalid capacity/active: %#v", lane)
			}
		}
		if lane.ID == workloadLaneExpensive {
			foundExpensive = true
			if lane.Capacity <= 0 || lane.Active < 0 {
				t.Fatalf("expensive workload lane has invalid capacity/active: %#v", lane)
			}
		}
		if lane.ID == workloadLaneAdminHeavy {
			foundAdminHeavy = true
			if lane.Capacity <= 0 || lane.Active < 0 {
				t.Fatalf("admin-expensive workload lane has invalid capacity/active: %#v", lane)
			}
		}
		if lane.ID == workloadLaneDLNA {
			foundDLNA = true
			if lane.Capacity <= 0 || lane.Active < 0 {
				t.Fatalf("dlna workload lane has invalid capacity/active: %#v", lane)
			}
		}
	}
	if !foundBrowsing {
		t.Fatalf("diagnostics missing browsing workload lane: %#v", diagnostics.WorkloadLanes)
	}
	if !foundExpensive {
		t.Fatalf("diagnostics missing expensive workload lane: %#v", diagnostics.WorkloadLanes)
	}
	if !foundAdminHeavy {
		t.Fatalf("diagnostics missing admin-expensive workload lane: %#v", diagnostics.WorkloadLanes)
	}
	if !foundDLNA {
		t.Fatalf("diagnostics missing dlna workload lane: %#v", diagnostics.WorkloadLanes)
	}
	if len(diagnostics.Dependencies) != 3 {
		t.Fatalf("dependencies = %d, expected 3", len(diagnostics.Dependencies))
	}
}

func TestSystemDiagnosticsCachesRuntimeDependencyProbes(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	tempDir := t.TempDir()
	counterPath := filepath.Join(tempDir, "dependency-probes.log")
	writeDependencyStub := func(name string) string {
		t.Helper()
		path := filepath.Join(tempDir, name)
		script := "#!/bin/sh\nprintf '" + name + "\\n' >> " + strconv.Quote(counterPath) + "\nprintf '" + name + " version test\\n'\n"
		if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
			t.Fatalf("write %s stub: %v", name, err)
		}
		return path
	}
	server.cfg.FFmpegPath = writeDependencyStub("ffmpeg")
	server.cfg.FFprobePath = writeDependencyStub("ffprobe")
	server.cfg.FPcalcPath = writeDependencyStub("fpcalc")

	server.refreshRuntimeDependencyDiagnostics(context.Background())
	first := server.runtimeDependencyDiagnostics()
	second := server.runtimeDependencyDiagnostics()
	if len(first) != 3 || len(second) != 3 {
		t.Fatalf("dependency probe lengths = %d/%d, want 3/3", len(first), len(second))
	}
	counter, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("read dependency probe counter: %v", err)
	}
	if got := len(strings.Fields(string(counter))); got != 3 {
		t.Fatalf("dependency probes executed %d times, want 3 with cached second call; log=%q", got, counter)
	}
}

func TestRuntimeDependencyDiagnosticsReturnPendingOnColdCache(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	tempDir := t.TempDir()
	server.cfg.FFmpegPath = filepath.Join(tempDir, "missing-ffmpeg")
	server.cfg.FFprobePath = filepath.Join(tempDir, "missing-ffprobe")
	server.cfg.FPcalcPath = filepath.Join(tempDir, "missing-fpcalc")

	dependencies := server.runtimeDependencyDiagnostics()
	if len(dependencies) != 3 {
		t.Fatalf("dependency diagnostics length = %d, want 3", len(dependencies))
	}
	for _, dependency := range dependencies {
		if dependency.Error != "probe pending" {
			t.Fatalf("cold dependency should be pending before background refresh, got %#v", dependency)
		}
	}
}

func TestSystemStartupReportsHTTPReadiness(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	server.MarkHTTPReady(serverURL)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var startup StartupDiagnostics
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/system/startup", nil, &startup)
	if status != http.StatusOK {
		t.Fatalf("startup status = %d, body: %s", status, body)
	}
	if !startup.HTTPReady || startup.HTTPReadyAt == "" || startup.HTTPAddr != serverURL {
		t.Fatalf("startup missing HTTP readiness: %#v", startup)
	}
	foundHTTPReady := false
	for _, phase := range startup.Phases {
		if phase.ID == "http_ready" && phase.Status == "complete" {
			foundHTTPReady = true
			break
		}
	}
	if !foundHTTPReady {
		t.Fatalf("startup missing http_ready phase: %#v", startup.Phases)
	}
}

func TestReadinessEndpointDoesNotRequireAuthentication(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	server.MarkHTTPReady(serverURL)

	var readiness ReadinessResponse
	status, body := doJSON(t, http.DefaultClient, http.MethodGet, serverURL+"/api/readiness", nil, &readiness)
	if status != http.StatusOK {
		t.Fatalf("readiness status = %d, body: %s", status, body)
	}
	if !readiness.Ready || readiness.Status != "ready" {
		t.Fatalf("readiness not ready: %#v", readiness)
	}
	if readiness.HTTPReady || readiness.DatabaseReady || readiness.Startup.HTTPAddr != "" {
		t.Fatalf("public readiness leaked operational diagnostics: %#v", readiness)
	}
}

func TestReadinessEndpointFailsWhenDatabaseIsUnavailable(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	server.MarkHTTPReady(serverURL)
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	var readiness ReadinessResponse
	status, body := doJSON(t, http.DefaultClient, http.MethodGet, serverURL+"/api/readiness", nil, &readiness)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, body: %s", status, body)
	}
	if readiness.Ready || readiness.Status != "unavailable" {
		t.Fatalf("readiness unexpectedly ready: %#v", readiness)
	}
	if readiness.DatabaseReady || readiness.DatabaseProbe.Status != "" || readiness.AuthBootstrapProbe.Status != "" {
		t.Fatalf("public readiness leaked database probes: %#v", readiness)
	}
}

func TestSQLiteHealthWatchdogDefersBackgroundUntilRecovered(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	server.MarkHTTPReady(serverURL)
	failing := true
	server.sqliteHealthProbe = func(context.Context) error {
		if failing {
			return errors.New("database is locked")
		}
		return nil
	}

	health := server.runSQLiteHealthProbeOnce(context.Background())
	if health.Status != sqliteHealthStatusHealthy || health.ConsecutiveFailures != 1 {
		t.Fatalf("first lock failure should record evidence without degrading yet: %#v", health)
	}
	if server.shouldDeferBackgroundJobsForPressure() {
		t.Fatalf("single lock failure should not defer background work")
	}

	health = server.runSQLiteHealthProbeOnce(context.Background())
	if health.Status != sqliteHealthStatusDegraded || health.ConsecutiveFailures != 2 {
		t.Fatalf("repeated lock failure should degrade sqlite health: %#v", health)
	}
	if health.LastFailureKind != "busy" || health.EvidenceCaptures != 1 {
		t.Fatalf("degraded health missing lock evidence: %#v", health)
	}
	if !server.shouldDeferBackgroundJobsForPressure() {
		t.Fatalf("degraded sqlite health should defer background work")
	}

	readiness := server.readinessReport(context.Background())
	if readiness.Ready || readiness.Status != "degraded" || !readiness.BackgroundJobsDeferred {
		t.Fatalf("readiness should reflect degraded sqlite health: %#v", readiness)
	}
	if readiness.SQLiteHealth.Status != sqliteHealthStatusDegraded {
		t.Fatalf("readiness missing sqlite health state: %#v", readiness.SQLiteHealth)
	}

	failing = false
	health = server.runSQLiteHealthProbeOnce(context.Background())
	if health.Status != sqliteHealthStatusRecovering || !server.shouldDeferBackgroundJobsForPressure() {
		t.Fatalf("first clean probe should enter recovering and keep background work deferred: %#v", health)
	}
	health = server.runSQLiteHealthProbeOnce(context.Background())
	if health.Status != sqliteHealthStatusHealthy || health.ConsecutiveSuccesses != 2 {
		t.Fatalf("second clean probe should restore healthy sqlite state: %#v", health)
	}
	if server.shouldDeferBackgroundJobsForPressure() {
		t.Fatalf("recovered sqlite health should release background deferral")
	}

	readiness = server.readinessReport(context.Background())
	if !readiness.Ready || readiness.Status != "ready" || readiness.SQLiteHealth.Status != sqliteHealthStatusHealthy {
		t.Fatalf("readiness should recover after consecutive clean probes for %s: %#v", serverURL, readiness)
	}
}

func TestSQLiteHealthWatchdogMarksCorruptImmediately(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	server.MarkHTTPReady(serverURL)
	server.sqliteHealthProbe = func(context.Context) error {
		return errors.New("database disk image is malformed")
	}

	health := server.runSQLiteHealthProbeOnce(context.Background())
	if health.Status != sqliteHealthStatusCorrupt || health.ConsecutiveFailures != 1 {
		t.Fatalf("corruption signal should mark sqlite health corrupt immediately: %#v", health)
	}
	if health.LastFailureKind != "corrupt" || health.EvidenceCaptures != 1 {
		t.Fatalf("corrupt health missing evidence: %#v", health)
	}
	if !server.shouldDeferBackgroundJobsForPressure() {
		t.Fatalf("corrupt sqlite health should defer background work")
	}

	readiness := server.readinessReport(context.Background())
	if readiness.Ready || readiness.SQLite.Status != sqliteHealthStatusCorrupt || readiness.SQLiteHealth.Status != sqliteHealthStatusCorrupt {
		t.Fatalf("readiness should expose corrupt sqlite health for %s: %#v", serverURL, readiness)
	}
}

func TestSQLiteHandleRecycleSwapsNonCorruptHandle(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	server.MarkHTTPReady(serverURL)
	oldDB := server.dbHandle()
	openerCalls := 0
	server.sqliteDBOpener = func(_ context.Context, cfg config.Config) (*sql.DB, error) {
		openerCalls++
		return database.OpenRuntimeHandle(cfg)
	}
	server.sqliteHealthMu.Lock()
	server.sqliteHealth = SQLiteHealthDiagnostic{
		Status:              sqliteHealthStatusDegraded,
		LastFailureKind:     string(database.ErrorKindBusy),
		LastFailureMessage:  "database is locked",
		ConsecutiveFailures: 2,
	}
	server.sqliteHealthMu.Unlock()

	server.attemptSQLiteHandleRecycle(context.Background(), server.sqliteHealthSnapshot())
	newDB := server.dbHandle()
	if newDB == nil || newDB == oldDB {
		t.Fatalf("sqlite recycle did not swap to a fresh handle")
	}
	t.Cleanup(func() {
		_ = server.dbHandle().Close()
	})
	if openerCalls != 1 {
		t.Fatalf("sqlite opener calls = %d, expected 1", openerCalls)
	}
	health := server.sqliteHealthSnapshot()
	if health.Status != sqliteHealthStatusRecovering || health.RecycleAttempts != 1 || health.RecycleSuccesses != 1 {
		t.Fatalf("sqlite recycle did not enter recovering state: %#v", health)
	}
	if health.LastRecoveryAction != "db_handle_recycled" || health.LastRecycleError != "" {
		t.Fatalf("sqlite recycle diagnostics not recorded: %#v", health)
	}

	readiness := server.readinessReport(context.Background())
	if readiness.Ready || readiness.SQLiteHealth.Status != sqliteHealthStatusRecovering {
		t.Fatalf("readiness should remain degraded until follow-up clean probe for %s: %#v", serverURL, readiness)
	}
	health = server.runSQLiteHealthProbeOnce(context.Background())
	if health.Status != sqliteHealthStatusHealthy || health.ConsecutiveSuccesses < 2 {
		t.Fatalf("clean probe after recycle should restore healthy state: %#v", health)
	}
	readiness = server.readinessReport(context.Background())
	if !readiness.Ready || readiness.SQLiteHealth.Status != sqliteHealthStatusHealthy {
		t.Fatalf("readiness should recover after clean recycle probe for %s: %#v", serverURL, readiness)
	}
}

func TestSQLiteHandleRecycleSkipsCorruption(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	oldDB := server.dbHandle()
	openerCalls := 0
	server.sqliteDBOpener = func(_ context.Context, cfg config.Config) (*sql.DB, error) {
		openerCalls++
		return database.OpenRuntimeHandle(cfg)
	}
	trigger := SQLiteHealthDiagnostic{
		Status:              sqliteHealthStatusCorrupt,
		LastFailureKind:     string(database.ErrorKindCorrupt),
		LastFailureMessage:  "database disk image is malformed",
		ConsecutiveFailures: 1,
	}

	server.attemptSQLiteHandleRecycle(context.Background(), trigger)
	if openerCalls != 0 {
		t.Fatalf("sqlite recycle attempted opener for corruption")
	}
	if server.dbHandle() != oldDB {
		t.Fatalf("sqlite recycle swapped handle for corruption")
	}
}

func TestSQLiteHandleRecycleFailureRecordsSafeState(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	server.sqliteDBOpener = func(context.Context, config.Config) (*sql.DB, error) {
		return nil, errors.New("open runtime handle failed")
	}
	server.sqliteHealthMu.Lock()
	server.sqliteHealth = SQLiteHealthDiagnostic{
		Status:              sqliteHealthStatusDegraded,
		LastFailureKind:     "timeout",
		LastFailureMessage:  "context deadline exceeded",
		ConsecutiveFailures: 2,
	}
	server.sqliteHealthMu.Unlock()

	server.attemptSQLiteHandleRecycle(context.Background(), server.sqliteHealthSnapshot())
	health := server.sqliteHealthSnapshot()
	if health.Status != sqliteHealthStatusFailed || health.RecycleAttempts != 1 || health.RecycleSuccesses != 0 {
		t.Fatalf("sqlite recycle failure did not enter failed safe state: %#v", health)
	}
	if health.LastRecoveryAction != "db_handle_recycle_failed" || !strings.Contains(health.LastRecycleError, "open runtime handle failed") {
		t.Fatalf("sqlite recycle failure missing diagnostics: %#v", health)
	}
	if !server.shouldDeferBackgroundJobsForPressure() {
		t.Fatalf("failed sqlite recovery should keep background work deferred")
	}
}

func TestSQLiteRuntimeHandleTimeoutCancelsTheOpenerWithoutLeavingAWorker(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	started := make(chan struct{})
	finished := make(chan struct{})
	server.sqliteDBOpener = func(ctx context.Context, _ config.Config) (*sql.DB, error) {
		close(started)
		defer close(finished)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	var result *sql.DB
	var openErr error
	go func() {
		result, openErr = server.openSQLiteRuntimeHandleWithTimeout(ctx)
		close(done)
	}()
	<-started
	<-ctx.Done()
	<-done
	if result != nil || !errors.Is(openErr, context.DeadlineExceeded) {
		t.Fatalf("timed-out runtime open db=%v err=%v", result, openErr)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("context-aware sqlite opener did not stop after cancellation")
	}
}

func TestDeferredStartupSQLiteMaintenanceReportsPhase(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	server.runStartupSQLiteMaintenance(context.Background())

	startup := server.startupDiagnostics()
	foundMaintenance := false
	for _, phase := range startup.Phases {
		if phase.ID == "startup_sqlite_maintenance" {
			foundMaintenance = true
			if phase.Status != "complete" || phase.DurationMillis < 0 || phase.Error != "" {
				t.Fatalf("unexpected sqlite maintenance phase: %#v", phase)
			}
			break
		}
	}
	if !foundMaintenance {
		t.Fatalf("startup missing sqlite maintenance phase: %#v", startup.Phases)
	}
}

func TestStartupOperationalRetentionPrunesOldTerminalRows(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	old := time.Now().UTC().AddDate(-2, 0, 0).Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('retention', '{"auditHistoryDays":365,"diagnosticHistoryDays":365}', ?)`, now); err != nil {
		t.Fatalf("configure audit retention: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO audit_events (id, actor_user_id, actor_email, action, resource_type, resource_id, severity, metadata_json, client_ip, user_agent, created_at)
		VALUES ('aud_retention_old', '', '', 'retention.old', 'test', 'old', 'info', '{}', '', '', ?),
			('aud_retention_recent', '', '', 'retention.recent', 'test', 'recent', 'info', '{}', '', '', ?)`, old, now); err != nil {
		t.Fatalf("insert retention audit rows: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, created_at, updated_at)
		VALUES ('job_retention_old_done', 'retention_test', 'complete', 100, 'Old done', 'test', 'old', ?, ?),
			('job_retention_old_active', 'retention_test', 'queued', 0, 'Old active', 'test', 'active', ?, ?),
			('job_retention_recent_done', 'retention_test', 'complete', 100, 'Recent done', 'test', 'recent', ?, ?)`,
		old, old, old, old, now, now); err != nil {
		t.Fatalf("insert retention jobs: %v", err)
	}

	server.runStartupOperationalRetention(context.Background())

	for id, want := range map[string]int{
		"aud_retention_old":         0,
		"aud_retention_recent":      1,
		"job_retention_old_done":    0,
		"job_retention_old_active":  1,
		"job_retention_recent_done": 1,
	} {
		table := "jobs"
		if strings.HasPrefix(id, "aud_") {
			table = "audit_events"
		}
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE id = ?`, id).Scan(&count); err != nil {
			t.Fatalf("count retained row %s: %v", id, err)
		}
		if count != want {
			t.Fatalf("retention row %s count = %d, expected %d", id, count, want)
		}
	}
}

func TestStartupMaintenanceDefersUnderForegroundPressure(t *testing.T) {
	tests := []struct {
		name    string
		phaseID string
		run     func(*Server, context.Context)
	}{
		{
			name:    "sqlite maintenance",
			phaseID: "startup_sqlite_maintenance",
			run:     (*Server).runStartupSQLiteMaintenance,
		},
		{
			name:    "operational retention",
			phaseID: "startup_operational_retention",
			run:     (*Server).runStartupOperationalRetention,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, server := newDiscoveryTestServer(t, config.Config{})

			server.workloadMu.Lock()
			server.workloadLanes[workloadLaneExpensive] = newWorkloadLane(workloadLaneExpensive, "Expensive API", 1)
			lane := server.workloadLanes[workloadLaneExpensive]
			if !lane.tryAcquire() {
				server.workloadMu.Unlock()
				t.Fatalf("failed to occupy expensive workload lane")
			}
			server.workloadMu.Unlock()
			defer lane.release()

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				tt.run(server, ctx)
				close(done)
			}()

			deadline := time.Now().Add(500 * time.Millisecond)
			for {
				startup := server.startupDiagnostics()
				for _, phase := range startup.Phases {
					if phase.ID == tt.phaseID && phase.Status == "running" {
						select {
						case <-done:
							t.Fatalf("%s completed while foreground lane was saturated", tt.phaseID)
						default:
						}
						cancel()
						select {
						case <-done:
							return
						case <-time.After(time.Second):
							t.Fatalf("%s did not stop after cancellation", tt.phaseID)
						}
					}
				}
				if time.Now().After(deadline) {
					cancel()
					<-done
					t.Fatalf("%s did not enter running state while waiting for pressure to ease", tt.phaseID)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestWorkloadAdmissionRejectsOverloadedBrowsingLane(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	server.workloadMu.Lock()
	server.workloadLanes[workloadLaneBrowsing] = newWorkloadLane(workloadLaneBrowsing, "Interactive browsing", 1)
	lane := server.workloadLanes[workloadLaneBrowsing]
	if !lane.tryAcquire() {
		server.workloadMu.Unlock()
		t.Fatalf("failed to occupy browsing workload lane")
	}
	server.workloadMu.Unlock()

	resp, err := client.Get(serverURL + "/api/home")
	if err != nil {
		t.Fatalf("send overloaded home request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "server_busy") {
		t.Fatalf("overloaded home status=%d body=%s", resp.StatusCode, body)
	}
	if resp.Header.Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, expected 1", resp.Header.Get("Retry-After"))
	}

	lane.release()

	var home HomeResponse
	status, responseBody := doJSON(t, client, http.MethodGet, serverURL+"/api/home", nil, &home)
	if status != http.StatusOK {
		t.Fatalf("home after workload release status=%d body=%s", status, responseBody)
	}
	diagnostics := server.workloadLaneDiagnostics()
	var rejected uint64
	for _, candidate := range diagnostics {
		if candidate.ID == workloadLaneBrowsing {
			rejected = candidate.Rejected
		}
	}
	if rejected != 1 {
		t.Fatalf("browsing rejected count = %d, expected 1: %#v", rejected, diagnostics)
	}
}

func TestDefaultWorkloadLaneCapacitiesStayConservative(t *testing.T) {
	lanes := newWorkloadLanes()
	want := map[string]int{
		workloadLaneAuth:       workloadLaneAuthCap,
		workloadLaneBrowsing:   workloadLaneBrowsingCap,
		workloadLaneExpensive:  workloadLaneExpensiveCap,
		workloadLanePlayback:   workloadLanePlaybackCap,
		workloadLaneMedia:      workloadLaneMediaCap,
		workloadLaneDLNA:       workloadLaneDLNACap,
		workloadLaneAdmin:      workloadLaneAdminCap,
		workloadLaneAdminHeavy: workloadLaneAdminHeavyCap,
		workloadLaneDefault:    workloadLaneDefaultCap,
	}
	for laneID, capacity := range want {
		lane := lanes[laneID]
		if lane == nil {
			t.Fatalf("missing workload lane %s", laneID)
		}
		if lane.capacity != capacity {
			t.Fatalf("%s capacity = %d, expected %d", laneID, lane.capacity, capacity)
		}
	}
	if lanes[workloadLaneBrowsing].capacity > 100 {
		t.Fatalf("browsing capacity drifted above active-viewer budget: %d", lanes[workloadLaneBrowsing].capacity)
	}
	if lanes[workloadLaneMedia].capacity > 100 {
		t.Fatalf("media capacity drifted above active-viewer budget: %d", lanes[workloadLaneMedia].capacity)
	}
	if lanes[workloadLaneAdmin].capacity > 100 {
		t.Fatalf("admin capacity drifted above active-viewer budget: %d", lanes[workloadLaneAdmin].capacity)
	}
}

func TestWorkloadAdmissionRejectsOverloadedExpensiveLane(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	server.workloadMu.Lock()
	server.workloadLanes[workloadLaneExpensive] = newWorkloadLane(workloadLaneExpensive, "Expensive API", 1)
	lane := server.workloadLanes[workloadLaneExpensive]
	if !lane.tryAcquire() {
		server.workloadMu.Unlock()
		t.Fatalf("failed to occupy expensive workload lane")
	}
	server.workloadMu.Unlock()

	resp, err := client.Get(serverURL + "/api/suggestions?limit=6")
	if err != nil {
		t.Fatalf("send overloaded suggestions request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "server_busy") {
		t.Fatalf("overloaded suggestions status=%d body=%s", resp.StatusCode, body)
	}
	if resp.Header.Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q, expected 1", resp.Header.Get("Retry-After"))
	}

	searchBody, err := json.Marshal(SearchRequest{Query: "Meridian"})
	if err != nil {
		t.Fatalf("marshal overloaded search request: %v", err)
	}
	searchRequest, err := http.NewRequest(http.MethodPost, serverURL+"/api/search", bytes.NewReader(searchBody))
	if err != nil {
		t.Fatalf("create overloaded search request: %v", err)
	}
	searchRequest.Header.Set("Content-Type", "application/json")
	searchRequest.Header.Set(csrfHeaderName, "1")
	resp, err = client.Do(searchRequest)
	if err != nil {
		t.Fatalf("send overloaded search request: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "server_busy") {
		t.Fatalf("overloaded search status=%d body=%s", resp.StatusCode, body)
	}
	if resp.Header.Get("Retry-After") != "1" {
		t.Fatalf("search Retry-After = %q, expected 1", resp.Header.Get("Retry-After"))
	}

	resp, err = client.Get(serverURL + "/api/libraries/lib_movies/discover?limit=12")
	if err != nil {
		t.Fatalf("send overloaded library discover request: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "server_busy") {
		t.Fatalf("overloaded library discover status=%d body=%s", resp.StatusCode, body)
	}
	if resp.Header.Get("Retry-After") != "1" {
		t.Fatalf("library discover Retry-After = %q, expected 1", resp.Header.Get("Retry-After"))
	}
	lane.release()

	var suggestions SuggestionsResponse
	status, responseBody := doJSON(t, client, http.MethodGet, serverURL+"/api/suggestions?limit=6", nil, &suggestions)
	if status != http.StatusOK {
		t.Fatalf("suggestions after workload release status=%d body=%s", status, responseBody)
	}
	diagnostics := server.workloadLaneDiagnostics()
	var rejected uint64
	for _, candidate := range diagnostics {
		if candidate.ID == workloadLaneExpensive {
			rejected = candidate.Rejected
		}
	}
	if rejected != 3 {
		t.Fatalf("expensive rejected count = %d, expected 3: %#v", rejected, diagnostics)
	}
}

func TestPlaybackMediaRoutesUsePlaybackWorkloadLane(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{name: "stream", path: "/api/media/movie_1/stream", want: workloadLaneMediaBody},
		{name: "hls", path: "/api/media/movie_1/hls/segment?name=segment_00001.ts", want: workloadLaneMediaBody},
		{name: "subtitles", path: "/api/media/movie_1/subtitles/sub_1", want: workloadLaneMediaBody},
		{name: "download", path: "/api/media/movie_1/download", want: workloadLaneBulkTransfer},
		{name: "trickplay", path: "/api/media/movie_1/trickplay", want: workloadLaneMedia},
		{name: "recommendations", path: "/api/media/movie_1/recommendations", want: workloadLaneExpensive},
		{name: "detail", path: "/api/media/movie_1", want: workloadLaneBrowsing},
		{name: "search", path: "/api/search?q=Meridian", want: workloadLaneExpensive},
		{name: "suggestions", path: "/api/suggestions?limit=6", want: workloadLaneExpensive},
		{name: "instant mix", path: "/api/instant-mix/track_1", want: workloadLaneExpensive},
		{name: "library discover", path: "/api/libraries/lib_movies/discover?limit=48", want: workloadLaneExpensive},
		{name: "product contract", path: "/api/product-contract", want: workloadLaneBrowsing},
		{name: "library browse capabilities", path: "/api/libraries/lib_movies/browse-capabilities", want: workloadLaneBrowsing},
		{name: "library browse", method: http.MethodPost, path: "/api/libraries/lib_movies/browse", want: workloadLaneBrowsing},
		{name: "playlist list", path: "/api/playlists", want: workloadLaneBrowsing},
		{name: "playlist detail", path: "/api/playlists/plist_1", want: workloadLaneExpensive},
		{name: "dashboard history", path: "/api/dashboard?mode=history&period=30d", want: workloadLaneAdminHeavy},
		{name: "dashboard history section", path: "/api/dashboard?mode=live&period=5m&sections=topPlayed", want: workloadLaneAdminHeavy},
		{name: "dashboard live", path: "/api/dashboard?mode=live&period=5m", want: workloadLaneAdmin},
		{name: "metadata health", path: "/api/metadata/health?limit=200", want: workloadLaneAdminHeavy},
		{name: "playback history", path: "/api/playback/history?count=none", want: workloadLaneAdminHeavy},
		{name: "playback history export", path: "/api/playback/history/export.csv?period=all", want: workloadLaneAdminHeavy},
		{name: "audit events", path: "/api/audit-events?limit=50", want: workloadLaneAdminHeavy},
		{name: "logs history", path: "/api/logs?limit=50", want: workloadLaneAdminHeavy},
		{name: "logs stream", path: "/api/logs/stream", want: workloadLaneRealtime},
		{name: "dlna content", path: "/dlna/content-directory", want: workloadLaneDLNA},
		{name: "dlna media", path: "/dlna/media/movie_1", want: workloadLaneDLNA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, tt.path, nil)
			if got := workloadLaneIDForRequest(req); got != tt.want {
				t.Fatalf("workload lane for %s = %q, expected %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestResourcePressureDefersBackgroundJobsWhenCriticalLaneSaturated(t *testing.T) {
	for _, laneID := range []string{workloadLaneBrowsing, workloadLaneExpensive} {
		t.Run(laneID, func(t *testing.T) {
			_, _, server := newDiscoveryTestServer(t, config.Config{})

			server.workloadMu.Lock()
			server.workloadLanes[laneID] = newWorkloadLane(laneID, laneID, 1)
			lane := server.workloadLanes[laneID]
			if !lane.tryAcquire() {
				server.workloadMu.Unlock()
				t.Fatalf("failed to occupy %s workload lane", laneID)
			}
			server.workloadMu.Unlock()
			defer lane.release()

			pressure := server.resourceDiagnostics(server.sqliteDiagnostics(), server.jobLaneDiagnostics(), server.workloadLaneDiagnostics())
			if pressure.Status != "overloaded" || !pressure.BackgroundJobsDeferred {
				t.Fatalf("resource pressure did not protect background jobs: %#v", pressure)
			}
			if !stringSliceContains(pressure.SaturatedWorkloadLanes, laneID) {
				t.Fatalf("resource pressure missing saturated %s lane: %#v", laneID, pressure)
			}
			if !server.shouldDeferBackgroundJobsForPressure() {
				t.Fatalf("job scheduler should defer background work while %s lane is saturated", laneID)
			}
		})
	}
}

func TestSystemDiagnosticsReportsJobLaneBacklog(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO jobs (id, type, status, progress, message, resource_type, resource_id, created_at, updated_at)
		VALUES
			('job_diag_scan', 'library_scan', 'queued', 0, 'Scan queued.', 'library', 'lib_movies', ?, ?),
			('job_diag_metadata', 'metadata_refresh', 'queued', 0, 'Metadata queued.', 'media', 'movie_meridian', ?, ?),
			('job_diag_live_tv', 'live_tv_refresh', 'running', 10, 'Guide refresh running.', 'live_tv', 'guide', ?, ?)`,
		now, now, now, now, now, now,
	); err != nil {
		t.Fatalf("insert diagnostic jobs: %v", err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var diagnostics SystemDiagnostics
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/system/diagnostics", nil, &diagnostics)
	if status != http.StatusOK {
		t.Fatalf("diagnostics status = %d, body: %s", status, body)
	}
	lanes := map[string]JobLaneDiagnostic{}
	for _, lane := range diagnostics.JobLanes {
		lanes[lane.ID] = lane
	}
	if lanes[jobLaneWriteHeavy].Capacity != 1 {
		t.Fatalf("write-heavy lane capacity = %d, expected 1: %#v", lanes[jobLaneWriteHeavy].Capacity, diagnostics.JobLanes)
	}
	if lanes[jobLaneWriteHeavy].Queued != 1 || lanes[jobLaneWriteHeavy].Running != 1 {
		t.Fatalf("write-heavy backlog = queued %d running %d, expected 1/1: %#v", lanes[jobLaneWriteHeavy].Queued, lanes[jobLaneWriteHeavy].Running, diagnostics.JobLanes)
	}
	if lanes[jobLaneMetadata].Queued != 1 || lanes[jobLaneMetadata].Running != 0 {
		t.Fatalf("metadata backlog = queued %d running %d, expected 1/0: %#v", lanes[jobLaneMetadata].Queued, lanes[jobLaneMetadata].Running, diagnostics.JobLanes)
	}
}

func TestSystemTimeSyncRequiresAuthAndReturnsBoundedTimestamps(t *testing.T) {
	serverURL := newAuthTestServer(t)
	status, body := doJSON(t, http.DefaultClient, http.MethodGet, serverURL+"/api/system/time", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated time status = %d, body: %s", status, body)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	before := time.Now().UTC()
	var sync SystemTimeSync
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/system/time", nil, &sync)
	after := time.Now().UTC()
	if status != http.StatusOK {
		t.Fatalf("time status = %d, body: %s", status, body)
	}
	receivedAt, err := time.Parse(time.RFC3339Nano, sync.RequestReceivedAt)
	if err != nil {
		t.Fatalf("parse request timestamp: %v", err)
	}
	responseSentAt, err := time.Parse(time.RFC3339Nano, sync.ResponseSentAt)
	if err != nil {
		t.Fatalf("parse response timestamp: %v", err)
	}
	if receivedAt.Before(before.Add(-time.Second)) || responseSentAt.After(after.Add(time.Second)) || responseSentAt.Before(receivedAt) {
		t.Fatalf("unexpected sync timestamps: %#v before=%s after=%s", sync, before, after)
	}
	if sync.ServerUnixMillis <= 0 {
		t.Fatalf("expected unix millis to be populated: %#v", sync)
	}
}

func TestClientLogUploadIsBoundedAndSettingGated(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	payload := ClientLogUploadRequest{
		Device: "Chrome",
		App:    "Portico Web",
		Entries: []ClientLogEntry{{
			Level:   "warn",
			Message: "Player stalled with Authorization Bearer should-not-store and access_token=abc123456789", // gitleaks:allow -- synthetic redaction fixture
			Context: map[string]string{
				"route":         "/watch/movie",
				"authorization": "Bearer should-not-store",
				"url":           "https://example.test/watch?api_key=abc123456789&ok=1",
			},
			Timestamp: "2026-05-04T12:00:00Z",
		}},
	}
	var response ClientLogUploadResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/client-logs", payload, &response)
	if status != http.StatusForbidden {
		t.Fatalf("disabled client log status = %d, body: %s", status, body)
	}
	if _, err := db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'troubleshooting'`, `{"clientLogUploads":true,"logLevel":"debug"}`); err != nil {
		t.Fatalf("enable client logs: %v", err)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/client-logs", payload, &response)
	if status != http.StatusCreated {
		t.Fatalf("client log status = %d, body: %s", status, body)
	}
	if !response.OK || response.Accepted != 1 {
		t.Fatalf("client log response = %#v", response)
	}
	var message, fieldsJSON string
	if err := db.QueryRow(`SELECT message, fields_json FROM client_diagnostic_events ORDER BY created_at DESC LIMIT 1`).Scan(&message, &fieldsJSON); err != nil {
		t.Fatalf("load persisted client diagnostic: %v", err)
	}
	if !strings.Contains(message, "Player stalled") {
		t.Fatalf("client diagnostic message = %q", message)
	}
	if strings.Contains(message, "should-not-store") || strings.Contains(message, "abc123456789") {
		t.Fatalf("client diagnostic message was not redacted: %s", message)
	}
	var fields map[string]string
	if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
		t.Fatalf("decode persisted client diagnostic fields: %v", err)
	}
	if fields["client.authorization"] != "[redacted]" || fields["client.route"] != "/watch/movie" || strings.Contains(fields["client.url"], "abc123456789") {
		t.Fatalf("client diagnostic fields were not sanitized: %#v", fields)
	}
	for _, event := range server.listLogEvents(50) {
		if strings.Contains(event.Message, "Player stalled") {
			t.Fatalf("client diagnostic leaked into server lane: %#v", event)
		}
	}
}

func TestSystemReleaseReportsUpdateReadiness(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	if _, err := db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('updates', '{"channel":"beta","maintenanceWindowUTC":"03:00-04:00"}', ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert legacy updater settings: %v", err)
	}
	status, body := patchSettingsGroups(t, client, serverURL, map[string]any{"updates": map[string]any{"channel": "beta"}}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("fictional updater patch status = %d, body: %s", status, body)
	}
	var release SystemReleaseInfo
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/system/release", nil, &release)
	if status != http.StatusOK {
		t.Fatalf("release status = %d, body: %s", status, body)
	}
	if release.Version == "" || release.APIVersion != systemAPIVersion || release.GOOS == "" || release.GOARCH == "" {
		t.Fatalf("release missing runtime identity: %#v", release)
	}
	if release.UpdateStatus != "unavailable" || release.InstallMethod != "manual" {
		t.Fatalf("release updater status was not truthful: %#v", release)
	}
	if !release.DatabaseReady || !release.AppDataReady || release.MigrationStatus != "ready" {
		t.Fatalf("release missing readiness fields: %#v", release)
	}
}

func TestTranscodeCapacityReportsRuntimeSettings(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := patchSettingsGroups(t, client, serverURL, map[string]any{
		"transcoder": map[string]any{
			"enabled":                 true,
			"maxConcurrentSessions":   3,
			"throttleBufferSeconds":   90,
			"x264Preset":              "faster",
			"hardwareAcceleration":    true,
			"hardwareDevice":          "videotoolbox",
			"hardwareEncoding":        true,
			"maxHardwareSessions":     4,
			"maxSoftwareSessions":     0,
			"maxBackgroundSessions":   1,
			"hdrToneMapping":          true,
			"hdrToneMappingAlgorithm": "mobius",
			"directStreamRemux":       true,
			"temporaryDirectory":      "",
		},
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("settings status = %d, body: %s", status, body)
	}

	var capacity TranscodeCapacityReport
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/transcode/capacity", nil, &capacity)
	if status != http.StatusOK {
		t.Fatalf("capacity status = %d, body: %s", status, body)
	}
	if !capacity.Enabled || capacity.MaxConcurrentSessions != 3 || capacity.AvailableSlots != 3 {
		t.Fatalf("capacity did not reflect transcode settings: %#v", capacity)
	}
	if capacity.HardwareDecodeValue != "videotoolbox" || capacity.HardwareEncoder != "h264_videotoolbox" {
		t.Fatalf("capacity did not report hardware mapping: %#v", capacity)
	}
	if capacity.MaxHardwareSessions != 4 || capacity.MaxSoftwareSessions != 0 || capacity.MaxBackgroundSessions != 1 || capacity.HDRToneMappingAlgorithm != "mobius" {
		t.Fatalf("capacity did not report transcode limit or tone-mapping settings: %#v", capacity)
	}
	if capacity.TemporaryDirectory == "" || !capacity.TemporaryDirectoryReady {
		t.Fatalf("temporary directory readiness missing: %#v", capacity)
	}
	if len(capacity.Presets) == 0 || capacity.FFmpeg.ConfiguredPath == "" || capacity.FFprobe.ConfiguredPath == "" {
		t.Fatalf("capacity missing dependency or preset details: %#v", capacity)
	}
}

func TestTranscodeCapacityReportsFullHardwareAndToneMappingSupport(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	ffmpegPath := writeCapacityFFmpegStub(t, "videotoolbox", "h264_videotoolbox", "zscale", "tonemap")
	server.cfg.FFmpegPath = ffmpegPath
	if _, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES ('transcoder', ?, ?)`, `{
		"enabled": true,
		"hardwareAcceleration": true,
		"hardwareEncoding": true,
		"hardwareDevice": "videotoolbox",
		"hdrToneMapping": true
	}`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("save transcode settings: %v", err)
	}

	server.refreshRuntimeDependencyDiagnostics(context.Background())
	server.refreshFFmpegCapabilityCache(context.Background(), ffmpegPath)
	capacity := server.transcodeCapacityReport()
	if capacity.HardwareSupportLevel != "available" || !capacity.HardwareEncoderAvailable {
		t.Fatalf("expected available hardware support, got %#v", capacity)
	}
	if !capacity.HDRToneMappingAvailable || capacity.HDRToneMappingStatus != "available" {
		t.Fatalf("expected available tone mapping support, got %#v", capacity)
	}
	if len(capacity.HardwareProbes) != 2 || capacity.HardwareProbes[0].Status != "available" || capacity.HardwareProbes[1].Status != "available" {
		t.Fatalf("expected decode and encode probes to be available, got %#v", capacity.HardwareProbes)
	}
}

func TestTranscodeCapacityReportsPartialHardwareAndToneMappingLimitations(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	ffmpegPath := writeCapacityFFmpegStub(t, "qsv", "", "scale")
	server.cfg.FFmpegPath = ffmpegPath
	if _, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES ('transcoder', ?, ?)`, `{
		"enabled": true,
		"hardwareAcceleration": true,
		"hardwareEncoding": true,
		"hardwareDevice": "qsv",
		"hdrToneMapping": true
	}`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("save transcode settings: %v", err)
	}

	server.refreshRuntimeDependencyDiagnostics(context.Background())
	server.refreshFFmpegCapabilityCache(context.Background(), ffmpegPath)
	capacity := server.transcodeCapacityReport()
	if capacity.HardwareSupportLevel != "partial" {
		t.Fatalf("expected partial hardware support, got %#v", capacity)
	}
	if capacity.HardwareProbes[0].Status != "available" || capacity.HardwareProbes[1].Status != "unavailable" {
		t.Fatalf("expected decode available and encode unavailable, got %#v", capacity.HardwareProbes)
	}
	if capacity.HDRToneMappingAvailable || capacity.HDRToneMappingStatus != "unavailable" {
		t.Fatalf("expected unavailable tone mapping support, got %#v", capacity)
	}
	if !containsText(capacity.Warnings, "partially available") || !containsText(capacity.Warnings, "zscale and tonemap") {
		t.Fatalf("expected partial hardware and tone-map warnings, got %#v", capacity.Warnings)
	}
}

func TestTranscodeSettingsCachesHDRFilterProbe(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	tempDir := t.TempDir()
	counterPath := filepath.Join(tempDir, "filter-probes.log")
	ffmpegPath := filepath.Join(tempDir, "ffmpeg")
	script := "#!/bin/sh\nfor arg in \"$@\"; do\ncase \"$arg\" in\n-filters) printf 'ffmpeg -filters\\n' >> " + strconv.Quote(counterPath) + "; echo ' ... zscale test filter'; echo ' ... tonemap test filter'; exit 0;;\nesac\ndone\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	server.cfg.FFmpegPath = ffmpegPath
	if _, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES ('transcoder', ?, ?)`, `{"hdrToneMapping": true}`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("save transcode settings: %v", err)
	}

	server.refreshFFmpegCapabilityCache(context.Background(), ffmpegPath)
	first := server.transcodeSettings()
	second := server.transcodeSettings()
	if !first.HDRToneMappingFilters || !second.HDRToneMappingFilters {
		t.Fatalf("expected cached HDR filter support, first=%#v second=%#v", first, second)
	}
	counter, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("read filter probe counter: %v", err)
	}
	if got := strings.Count(string(counter), "-filters"); got != 1 {
		t.Fatalf("filter probes executed %d times, want 1 with cached second call; log=%q", got, counter)
	}
}

func TestTranscodeSettingsDoesNotProbeHDRFiltersOnColdCache(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	tempDir := t.TempDir()
	ffmpegPath := filepath.Join(tempDir, "ffmpeg")
	script := "#!/bin/sh\nfor arg in \"$@\"; do\ncase \"$arg\" in\n-filters) sleep 1; echo ' ... zscale test filter'; echo ' ... tonemap test filter'; exit 0;;\nesac\ndone\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	server.cfg.FFmpegPath = ffmpegPath
	if _, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES ('transcoder', ?, ?)`, `{"hdrToneMapping": true}`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("save transcode settings: %v", err)
	}

	started := time.Now()
	settings := server.transcodeSettings()
	if elapsed := time.Since(started); elapsed > 300*time.Millisecond {
		t.Fatalf("cold transcodeSettings blocked on FFmpeg probe for %s", elapsed)
	}
	if settings.HDRToneMappingFilters {
		t.Fatalf("cold transcodeSettings should not claim HDR filter support before prewarm: %#v", settings)
	}
}

func TestTranscodeCapacityCachesRuntimeAndCapabilityProbes(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	tempDir := t.TempDir()
	counterPath := filepath.Join(tempDir, "capacity-probes.log")
	writeProbeStub := func(name string) string {
		t.Helper()
		path := filepath.Join(tempDir, name)
		script := "#!/bin/sh\nfor arg in \"$@\"; do\ncase \"$arg\" in\n-version) printf '" + name + " -version\\n' >> " + strconv.Quote(counterPath) + "; echo '" + name + " version test'; exit 0;;\n-filters) printf '" + name + " -filters\\n' >> " + strconv.Quote(counterPath) + "; echo ' ... zscale test filter'; echo ' ... tonemap test filter'; exit 0;;\n-hwaccels) printf '" + name + " -hwaccels\\n' >> " + strconv.Quote(counterPath) + "; echo 'Hardware acceleration methods:'; echo 'videotoolbox'; exit 0;;\n-encoders) printf '" + name + " -encoders\\n' >> " + strconv.Quote(counterPath) + "; echo ' V....D h264_videotoolbox test encoder'; exit 0;;\nesac\ndone\nexit 0\n"
		if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
			t.Fatalf("write %s stub: %v", name, err)
		}
		return path
	}
	server.cfg.FFmpegPath = writeProbeStub("ffmpeg")
	server.cfg.FFprobePath = writeProbeStub("ffprobe")
	server.cfg.FPcalcPath = writeProbeStub("fpcalc")
	if _, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES ('transcoder', ?, ?)`, `{
		"enabled": true,
		"hardwareAcceleration": true,
		"hardwareEncoding": true,
		"hardwareDevice": "videotoolbox"
	}`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("save transcode settings: %v", err)
	}

	server.refreshRuntimeDependencyDiagnostics(context.Background())
	server.refreshFFmpegCapabilityCache(context.Background(), server.cfg.FFmpegPath)
	first := server.transcodeCapacityReport()
	second := server.transcodeCapacityReport()
	if !first.HardwareEncoderAvailable || !second.HardwareEncoderAvailable {
		t.Fatalf("expected cached hardware encoder availability, first=%#v second=%#v", first, second)
	}
	counter, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatalf("read capacity probe counter: %v", err)
	}
	if got := len(strings.Fields(string(counter))) / 2; got != 6 {
		t.Fatalf("capacity probes executed %d times, want 6 with cached second call; log=%q", got, counter)
	}
}

func TestTranscodeCapacityAllowsUnlimitedSessions(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := patchSettingsGroups(t, client, serverURL, map[string]any{
		"transcoder": map[string]any{
			"enabled":               true,
			"maxConcurrentSessions": 0,
		},
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("settings status = %d, body: %s", status, body)
	}

	var capacity TranscodeCapacityReport
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/transcode/capacity", nil, &capacity)
	if status != http.StatusOK {
		t.Fatalf("capacity status = %d, body: %s", status, body)
	}
	if capacity.MaxConcurrentSessions != 0 || capacity.AvailableSlots != 0 {
		t.Fatalf("unlimited capacity was not preserved: %#v", capacity)
	}
}

func writeCapacityFFmpegStub(t *testing.T, hwaccel string, encoder string, filters ...string) string {
	t.Helper()
	ffmpegPath := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\nfor arg in \"$@\"; do\ncase \"$arg\" in\n-version) echo 'ffmpeg test build'; exit 0;;\n-hwaccels) echo 'Hardware acceleration methods:'; echo '" + hwaccel + "'; exit 0;;\n-encoders) echo ' V....D " + encoder + " test encoder'; exit 0;;\n-filters) "
	for _, filter := range filters {
		script += "echo ' ... " + filter + " test filter'; "
	}
	script += "exit 0;;\nesac\ndone\nexit 0\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return ffmpegPath
}

func containsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func TestSystemStorageReportAndCleanup(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	oldAt := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	freshAt := time.Now().UTC().Format(time.RFC3339)
	staleDir := filepath.Join(server.cfg.AppDataDir, "trickplay", safePathComponent("movie_meridian"), safePathComponent("stale_set"))
	freshDir := filepath.Join(server.cfg.AppDataDir, "trickplay", safePathComponent("movie_meridian"), safePathComponent("fresh_set"))
	if err := os.MkdirAll(staleDir, 0o700); err != nil {
		t.Fatalf("create stale trickplay dir: %v", err)
	}
	if err := os.MkdirAll(freshDir, 0o700); err != nil {
		t.Fatalf("create fresh trickplay dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "tile_00000.jpg"), []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale trickplay tile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(freshDir, "tile_00000.jpg"), []byte("fresh"), 0o600); err != nil {
		t.Fatalf("write fresh trickplay tile: %v", err)
	}
	imageCacheDir := filepath.Join(server.cfg.AppDataDir, "image-cache", "artwork")
	if err := os.MkdirAll(imageCacheDir, 0o700); err != nil {
		t.Fatalf("create image cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(imageCacheDir, "cached.jpg"), []byte("cached"), 0o600); err != nil {
		t.Fatalf("write image cache file: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_trickplay_sets (
			id, media_id, media_file_id, width, height, tile_width, tile_height,
			interval_seconds, duration_seconds, tile_count, path, stale, created_at
		) VALUES
			('stale_set', 'movie_meridian', 'stale_file', 160, 90, 160, 90, 10, 120, 1, ?, 1, ?),
			('fresh_set', 'movie_meridian', 'fresh_file', 160, 90, 160, 90, 10, 120, 1, ?, 1, ?)`,
		staleDir, oldAt, freshDir, freshAt); err != nil {
		t.Fatalf("insert trickplay sets: %v", err)
	}
	if _, err := db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'scheduledTasks'`, `{"enabled":true,"trickplayRetentionDays":1}`); err != nil {
		t.Fatalf("save scheduled settings: %v", err)
	}
	retentionNow := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	server.retentionClock = func() time.Time { return retentionNow }
	if _, err := db.Exec(`
		INSERT INTO audit_events (id, action, resource_type, resource_id, severity, metadata_json, created_at)
		VALUES ('manual-cleanup-old-audit', 'test', 'test', 'old', 'info', '{}', ?),
		       ('manual-cleanup-recent-audit', 'test', 'test', 'recent', 'info', '{}', ?)`,
		retentionNow.AddDate(0, 0, -91).Format(time.RFC3339Nano), retentionNow.AddDate(0, 0, -1).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert manual cleanup retention fixtures: %v", err)
	}

	var report SystemStorageReport
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/system/storage", nil, &report)
	if status != http.StatusOK {
		t.Fatalf("storage report status = %d, body: %s", status, body)
	}
	if len(report.Categories) == 0 {
		t.Fatalf("expected storage categories")
	}
	if !storageReportHasCategory(report, "optimized") || !storageReportHasCategory(report, "backups") || !storageReportHasCategory(report, "mediaTrash") || !storageReportHasCategory(report, "trickplay") || !storageReportHasCategory(report, "imageCache") {
		t.Fatalf("storage report missing cleanup-backed categories: %#v", report.Categories)
	}

	var cleanup SystemStorageCleanupResponse
	releaseStorageCleanup := server.acquireJobLane("system_storage_cleanup")
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/system/storage/cleanup", nil, &cleanup)
	if status != http.StatusAccepted {
		t.Fatalf("storage cleanup status = %d, body: %s", status, body)
	}
	if !cleanup.OK || !cleanup.Queued || cleanup.Job.ID == "" || cleanup.Job.Type != "system_storage_cleanup" {
		t.Fatalf("unexpected storage cleanup response: %#v", cleanup)
	}
	releaseStorageCleanup()
	server.runJob(cleanup.Job)
	completed, err := server.getJob(cleanup.Job.ID)
	if err != nil {
		t.Fatalf("get cleanup job: %v", err)
	}
	if completed.Status != "complete" {
		t.Fatalf("cleanup job status = %s, message = %q", completed.Status, completed.Message)
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatalf("expected stale trickplay dir to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(freshDir); err != nil {
		t.Fatalf("expected fresh trickplay dir to remain: %v", err)
	}
	if _, err := os.Stat(imageCacheDir); !os.IsNotExist(err) {
		t.Fatalf("expected image cache dir to be removed, stat err=%v", err)
	}
	var remainingStale int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_trickplay_sets WHERE id = 'stale_set'`).Scan(&remainingStale); err != nil {
		t.Fatalf("count stale trickplay sets: %v", err)
	}
	if remainingStale != 0 {
		t.Fatalf("stale trickplay set remained in database")
	}
	assertCount(t, db, `SELECT COUNT(*) FROM audit_events WHERE id = 'manual-cleanup-old-audit'`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM audit_events WHERE id = 'manual-cleanup-recent-audit'`, 1)
}

func TestBrandingEndpointIgnoresCustomSettings(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO settings (key, value_json, updated_at)
		VALUES ('branding', '{"applicationName":" My Studio ","tagline":"Private media","loginDisclaimer":"Authorized use only","logoUrl":"http://127.0.0.1/logo.png","accentColor":"#ABCDEF"}', ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, now); err != nil {
		t.Fatalf("save branding settings: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO settings (key, value_json, updated_at)
		VALUES ('server', '{"friendlyName":"Studio Server"}', ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, now); err != nil {
		t.Fatalf("save server settings: %v", err)
	}
	var branding BrandingInfo
	status, body := doJSON(t, http.DefaultClient, http.MethodGet, serverURL+"/api/branding", nil, &branding)
	if status != http.StatusOK {
		t.Fatalf("branding status = %d, body: %s", status, body)
	}
	if branding.ApplicationName != "Portico" || branding.Tagline != "Open Source Media Server" || branding.LoginDisclaimer != "" || branding.LogoURL != "" || branding.AccentColor != "" {
		t.Fatalf("branding endpoint should return fixed Portico identity: %#v", branding)
	}
}

func TestMediaDownloadServesLocalSourceForPermittedUser(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	mediaRoot := t.TempDir()
	sourcePath := filepath.Join(mediaRoot, "Download Me.mp4")
	if err := os.WriteFile(sourcePath, []byte("download-body"), 0o600); err != nil {
		t.Fatalf("write source media: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_download', 'Downloads', 'movie', 90, ?, '{}', ?)`, mediaRoot, now); err != nil {
		t.Fatalf("insert download library: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO library_paths (id, library_id, path, sort_order, created_at) VALUES ('lp_download', 'lib_download', ?, 0, ?)`, mediaRoot, now); err != nil {
		t.Fatalf("insert download library path: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES (?, 'lib_download', ?)`, adminUserID(t, db), now); err != nil {
		t.Fatalf("grant download library access: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, source_url, duration_seconds)
		VALUES ('download_media', 'lib_download', 'movie', 'Download Me', 'Download Me', ?, ?, 60)`, now, sourcePath); err != nil {
		t.Fatalf("insert downloadable media: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_streams (id, media_id, kind, codec, channels, bitrate, width, height, display_title)
		VALUES ('download_media_video', 'download_media', 'video', 'h264', 0, 2000000, 1280, 720, 'H.264 720p'),
		       ('download_media_audio', 'download_media', 'audio', 'aac', 2, 128000, 0, 0, 'AAC Stereo')`); err != nil {
		t.Fatalf("insert downloadable media streams: %v", err)
	}
	seedExactPlaybackFactsForFixture(t, server, "download_media")
	var playback PlaybackResponse
	status, playbackBody := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", PlaybackSessionCreateRequest{
		MediaID: "download_media",
		ClientProfile: attachAuthenticatedPlaybackRuntime(PlaybackClientProfile{
			SupportedContainers:  []string{"mp4"},
			SupportedVideoCodecs: []string{"h264"},
			SupportedAudioCodecs: []string{"aac"},
		}),
	}, &playback)
	if status != http.StatusOK || playback.SourceURL == "" {
		t.Fatalf("authorize source stream status=%d body=%s playback=%#v", status, playbackBody, playback)
	}

	accountOnlyResponse, err := client.Get(serverURL + "/api/media/download_media/download")
	if err != nil {
		t.Fatalf("account-only download request: %v", err)
	}
	_, _ = io.Copy(io.Discard, accountOnlyResponse.Body)
	_ = accountOnlyResponse.Body.Close()
	if accountOnlyResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("account credential bypassed download grant: status=%d", accountOnlyResponse.StatusCode)
	}
	sourceGrant := createDownloadGrantForTest(t, client, serverURL, "download_media", "source")
	response, err := client.Get(serverURL + sourceGrant.DownloadURL)
	if err != nil {
		t.Fatalf("download request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("download status = %d, body: %s", response.StatusCode, body)
	}
	if disposition := response.Header.Get("Content-Disposition"); !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, "DownloadMe.mp4") {
		t.Fatalf("unexpected content disposition: %q", disposition)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read download body: %v", err)
	}
	if string(body) != "download-body" {
		t.Fatalf("download body = %q", body)
	}
	streamResp, err := client.Get(serverURL + playback.SourceURL)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	streamBytes, _ := io.ReadAll(streamResp.Body)
	_ = streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK || string(streamBytes) != "download-body" {
		t.Fatalf("stream status=%d body=%s", streamResp.StatusCode, streamBytes)
	}
	server.streamMu.Lock()
	server.streamActive = map[string]int{adminUserID(t, db): maxConcurrentStreamsPerUser}
	server.streamMu.Unlock()
	limitedStreamResp, err := client.Get(serverURL + playback.SourceURL)
	if err != nil {
		t.Fatalf("limited stream request: %v", err)
	}
	limitedStreamBody, _ := io.ReadAll(limitedStreamResp.Body)
	_ = limitedStreamResp.Body.Close()
	if limitedStreamResp.StatusCode != http.StatusTooManyRequests || !strings.Contains(string(limitedStreamBody), "stream_busy") {
		t.Fatalf("limited stream status=%d body=%s", limitedStreamResp.StatusCode, limitedStreamBody)
	}
	if limitedStreamResp.Header.Get("Retry-After") != "5" {
		t.Fatalf("limited stream Retry-After = %q, expected 5", limitedStreamResp.Header.Get("Retry-After"))
	}
	if rejected := server.admissionDiagnostics().StreamRejected; rejected != 1 {
		t.Fatalf("stream rejected diagnostics = %d, expected 1", rejected)
	}
	streamHeadReq, err := http.NewRequest(http.MethodHead, serverURL+playback.SourceURL, nil)
	if err != nil {
		t.Fatalf("create stream HEAD request: %v", err)
	}
	streamHeadResp, err := client.Do(streamHeadReq)
	if err != nil {
		t.Fatalf("stream HEAD request: %v", err)
	}
	_ = streamHeadResp.Body.Close()
	if streamHeadResp.StatusCode != http.StatusOK {
		t.Fatalf("stream HEAD status=%d", streamHeadResp.StatusCode)
	}
	server.streamMu.Lock()
	server.streamActive = map[string]int{}
	server.streamMu.Unlock()

	var options DownloadOptionsResponse
	status, optionsBody := doJSON(t, client, http.MethodGet, serverURL+"/api/media/download_media/download-options", nil, &options)
	if status != http.StatusOK {
		t.Fatalf("download options status=%d body=%s", status, optionsBody)
	}
	var sourceOption *DownloadOption
	for i := range options.Options {
		if options.Options[i].ID == "source" {
			sourceOption = &options.Options[i]
		}
	}
	if sourceOption == nil || !sourceOption.Available || sourceOption.SourceKind != "local" {
		t.Fatalf("source download option = %#v", sourceOption)
	}

	userID := adminUserID(t, db)
	server.downloadMu.Lock()
	server.downloadActive = map[string]int{userID: maxConcurrentDownloadsPerUser}
	server.downloadMu.Unlock()
	limitedGrant := createDownloadGrantForTest(t, client, serverURL, "download_media", "source")
	limitedResp, err := client.Get(serverURL + limitedGrant.DownloadURL)
	if err != nil {
		t.Fatalf("limited download request: %v", err)
	}
	limitedBody, _ := io.ReadAll(limitedResp.Body)
	_ = limitedResp.Body.Close()
	if limitedResp.StatusCode != http.StatusTooManyRequests || !strings.Contains(string(limitedBody), "download_busy") {
		t.Fatalf("limited download status=%d body=%s", limitedResp.StatusCode, limitedBody)
	}
	if limitedResp.Header.Get("Retry-After") != "10" {
		t.Fatalf("limited download Retry-After = %q, expected 10", limitedResp.Header.Get("Retry-After"))
	}
	if rejected := server.admissionDiagnostics().DownloadRejected; rejected != 1 {
		t.Fatalf("download rejected diagnostics = %d, expected 1", rejected)
	}
	headGrant := createDownloadGrantForTest(t, client, serverURL, "download_media", "source")
	headReq, err := http.NewRequest(http.MethodHead, serverURL+headGrant.DownloadURL, nil)
	if err != nil {
		t.Fatalf("create download HEAD request: %v", err)
	}
	headResp, err := client.Do(headReq)
	if err != nil {
		t.Fatalf("download HEAD request: %v", err)
	}
	_ = headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK {
		t.Fatalf("download HEAD status=%d", headResp.StatusCode)
	}
}

func TestMediaSubtitleUploadStreamAndDelete(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("language", "en"); err != nil {
		t.Fatalf("write language: %v", err)
	}
	if err := writer.WriteField("label", "English Uploaded"); err != nil {
		t.Fatalf("write label: %v", err)
	}
	part, err := writer.CreateFormFile("file", "english.vtt")
	if err != nil {
		t.Fatalf("create subtitle part: %v", err)
	}
	if _, err := part.Write([]byte("WEBVTT\n\n00:00:00.000 --> 00:00:02.000\nHello\n")); err != nil {
		t.Fatalf("write subtitle part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/media/movie_meridian/subtitles", &body)
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfHeaderName, "1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload subtitle: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d, body: %s", resp.StatusCode, body)
	}
	var item MediaItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if len(item.Streams) != 0 {
		t.Fatalf("subtitle upload response should stay lightweight, got streams: %#v", item.Streams)
	}
	status, bodyText := doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", status, bodyText)
	}
	var uploaded Stream
	for _, stream := range item.Streams {
		if stream.Kind == "subtitle" && stream.Codec == "webvtt" && stream.DisplayTitle == "English Uploaded" {
			uploaded = stream
			break
		}
	}
	if uploaded.ID == "" {
		t.Fatalf("uploaded subtitle stream missing: %#v", item.Streams)
	}
	uploaded.SourceURL = "/api/media/movie_meridian/subtitles/" + uploaded.ID
	streamRequest, err := http.NewRequest(http.MethodGet, serverURL+uploaded.SourceURL, nil)
	if err != nil {
		t.Fatalf("create subtitle stream request: %v", err)
	}
	streamRequest.Header.Set("Authorization", "PorticoMedia "+playbackMediaGrantForTest(t, client, serverURL, "movie_meridian"))
	streamResp, err := client.Do(streamRequest)
	if err != nil {
		t.Fatalf("stream subtitle: %v", err)
	}
	streamBytes, _ := io.ReadAll(streamResp.Body)
	_ = streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK || !strings.Contains(string(streamBytes), "WEBVTT") {
		t.Fatalf("subtitle stream status=%d body=%s", streamResp.StatusCode, streamBytes)
	}
	var adjusted MediaItem
	status, bodyText = doJSON(t, client, http.MethodPatch, serverURL+"/api/media/movie_meridian/subtitles/"+uploaded.ID, SubtitleUpdateRequest{OffsetMs: 1500}, &adjusted)
	if status != http.StatusOK {
		t.Fatalf("subtitle timing status=%d body=%s", status, bodyText)
	}
	if len(adjusted.Streams) != 0 {
		t.Fatalf("subtitle timing response should stay lightweight, got streams: %#v", adjusted.Streams)
	}
	status, bodyText = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &adjusted)
	if status != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", status, bodyText)
	}
	var adjustedStream Stream
	for _, stream := range adjusted.Streams {
		if stream.ID == uploaded.ID {
			adjustedStream = stream
			break
		}
	}
	if adjustedStream.SubtitleOffsetMs != 1500 {
		t.Fatalf("subtitle offset was not persisted: %#v", adjustedStream)
	}
	streamResp, err = client.Get(serverURL + uploaded.SourceURL)
	if err != nil {
		t.Fatalf("stream adjusted subtitle: %v", err)
	}
	streamBytes, _ = io.ReadAll(streamResp.Body)
	_ = streamResp.Body.Close()
	if streamResp.StatusCode != http.StatusOK || !strings.Contains(string(streamBytes), "00:00:01.500 --> 00:00:03.500") {
		t.Fatalf("adjusted subtitle stream status=%d body=%s", streamResp.StatusCode, streamBytes)
	}
	status, deleteBody := doJSON(t, client, http.MethodDelete, serverURL+"/api/media/movie_meridian/subtitles/"+uploaded.ID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("delete subtitle status = %d, body: %s", status, deleteBody)
	}
}

func TestMediaAttachmentDownloadUsesManagedPath(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	attachmentDir := filepath.Join(server.cfg.AppDataDir, "attachments", safePathComponent("movie_meridian"))
	if err := os.MkdirAll(attachmentDir, 0o700); err != nil {
		t.Fatalf("create attachment dir: %v", err)
	}
	attachmentPath := filepath.Join(attachmentDir, "Fancy.ttf")
	if err := os.WriteFile(attachmentPath, []byte("font-bytes"), 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_attachments (id, media_id, stream_id, filename, mime_type, codec, path, size_bytes, created_at)
		VALUES ('att_font_1', 'movie_meridian', 'movie_meridian_probe_4', 'Fancy.ttf', 'font/ttf', 'ttf', ?, ?, ?)`,
		attachmentPath, int64(len("font-bytes")), now); err != nil {
		t.Fatalf("insert attachment: %v", err)
	}
	detailStatus, detailBody := doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, nil)
	if detailStatus != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailStatus, detailBody)
	}
	var item MediaItem
	if err := json.Unmarshal([]byte(detailBody), &item); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(item.Attachments) != 1 || item.Attachments[0].URL == "" {
		t.Fatalf("attachments = %#v", item.Attachments)
	}
	resp, err := client.Get(serverURL + item.Attachments[0].URL)
	if err != nil {
		t.Fatalf("download attachment: %v", err)
	}
	bytes, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(bytes) != "font-bytes" || resp.Header.Get("Content-Type") != "font/ttf" {
		t.Fatalf("attachment status=%d content-type=%q body=%q", resp.StatusCode, resp.Header.Get("Content-Type"), bytes)
	}
}

func TestMediaDetailIncludesSegments(t *testing.T) {
	serverURL, db, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO media_segments (id, media_id, segment_type, start_seconds, end_seconds, source, provider, confidence, created_at)
		VALUES ('seg_intro_1', 'movie_meridian', 'intro', 12, 83, 'manual', 'portico', 0.92, ?)`, now); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", status, body)
	}
	var item MediaItem
	if err := json.Unmarshal([]byte(body), &item); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(item.Segments) != 1 {
		t.Fatalf("segments = %#v", item.Segments)
	}
	segment := item.Segments[0]
	if segment.Type != "intro" || segment.StartSeconds != 12 || segment.EndSeconds != 83 || segment.Confidence != 0.92 {
		t.Fatalf("segment = %#v", segment)
	}
}

func TestMediaSegmentsCanBeCreatedAndDeleted(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/segments", MediaSegmentRequest{
		Type:         "credits",
		StartSeconds: 6120,
		EndSeconds:   6180,
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("create segment status=%d body=%s", status, body)
	}
	var created MediaItem
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode created segment response: %v", err)
	}
	if len(created.Segments) != 1 || created.Segments[0].Type != "credits" || created.Segments[0].Source != "manual" || created.Segments[0].Provider != "editor" {
		t.Fatalf("created segments = %#v", created.Segments)
	}

	deleteStatus, deleteBody := doJSON(t, client, http.MethodDelete, serverURL+"/api/media/movie_meridian/segments/"+created.Segments[0].ID, nil, nil)
	if deleteStatus != http.StatusOK {
		t.Fatalf("delete segment status=%d body=%s", deleteStatus, deleteBody)
	}
	var after MediaItem
	if err := json.Unmarshal([]byte(deleteBody), &after); err != nil {
		t.Fatalf("decode deleted segment response: %v", err)
	}
	if len(after.Segments) != 0 {
		t.Fatalf("segments after delete = %#v", after.Segments)
	}
}

func TestMediaTrickplaySetsListDoesNotExposePaths(t *testing.T) {
	serverURL, db, server := newDiscoveryTestServer(t, config.Config{})
	seedExactPlaybackFactsForFixture(t, server, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	now := time.Now().UTC().Format(time.RFC3339)
	tileDir := filepath.Join(server.cfg.AppDataDir, "trickplay", safePathComponent("movie_meridian"), safePathComponent("trick_1"))
	if err := os.MkdirAll(tileDir, 0o700); err != nil {
		t.Fatalf("create trickplay dir: %v", err)
	}
	tilePath := filepath.Join(tileDir, "tile_00001.jpg")
	if err := os.WriteFile(tilePath, []byte("jpeg"), 0o600); err != nil {
		t.Fatalf("write trickplay tile: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_trickplay_sets (
			id, media_id, media_file_id, width, height, tile_width, tile_height,
			interval_seconds, duration_seconds, tile_count, path, stale, created_at
		) VALUES (
			'trick_1', 'movie_meridian', 'file_primary', 320, 180, 160, 90,
			10, 7200, 1, ?, 0, ?
		)`, tileDir, now); err != nil {
		t.Fatalf("insert trickplay set: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_trickplay_tiles (id, set_id, tile_index, start_seconds, end_seconds, row, col, path, created_at)
		VALUES ('trick_tile_1', 'trick_1', 0, 0, 10, 0, 0, ?, ?)`, tilePath, now); err != nil {
		t.Fatalf("insert trickplay tile: %v", err)
	}
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian/trickplay", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("trickplay status=%d body=%s", status, body)
	}
	var response ListResponse[MediaTrickplaySet]
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode trickplay response: %v", err)
	}
	if response.Total != 1 || len(response.Items) != 1 {
		t.Fatalf("trickplay response = %#v", response)
	}
	set := response.Items[0]
	if set.ID != "trick_1" || set.Width != 320 || set.Height != 180 || set.TileCount != 1 || set.Stale {
		t.Fatalf("trickplay set = %#v", set)
	}
	if strings.Contains(body, "/private/app-data") || strings.Contains(body, "path") {
		t.Fatalf("trickplay response exposed internal path: %s", body)
	}
	var playback PlaybackResponse
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", PlaybackSessionCreateRequest{MediaID: "movie_meridian", SkipPreroll: true, ClientProfile: authenticatedPlaybackRuntimeProfile()}, &playback)
	if status != http.StatusOK || playback.MediaGrant.Token == "" {
		t.Fatalf("create trickplay playback grant status=%d body=%s", status, body)
	}
	tileRequest, err := http.NewRequest(http.MethodGet, serverURL+"/api/media/movie_meridian/trickplay/trick_1/tiles/0.jpg", nil)
	if err != nil {
		t.Fatalf("create trickplay tile request: %v", err)
	}
	tileRequest.Header.Set("Authorization", "PorticoMedia "+playback.MediaGrant.Token)
	resp, err := client.Do(tileRequest)
	if err != nil {
		t.Fatalf("get trickplay tile: %v", err)
	}
	tileBytes, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(tileBytes) != "jpeg" || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("tile status=%d content-type=%q body=%q", resp.StatusCode, resp.Header.Get("Content-Type"), tileBytes)
	}
	playlistRequest, err := http.NewRequest(http.MethodGet, serverURL+"/api/media/movie_meridian/trickplay/trick_1/tiles.m3u8", nil)
	if err != nil {
		t.Fatalf("create trickplay playlist request: %v", err)
	}
	playlistRequest.Header.Set("Authorization", "PorticoMedia "+playback.MediaGrant.Token)
	resp, err = client.Do(playlistRequest)
	if err != nil {
		t.Fatalf("get trickplay playlist: %v", err)
	}
	playlistBytes, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	playlist := string(playlistBytes)
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "application/vnd.apple.mpegurl") {
		t.Fatalf("playlist status=%d content-type=%q body=%q", resp.StatusCode, resp.Header.Get("Content-Type"), playlist)
	}
	for _, want := range []string{"#EXTM3U", "#EXT-X-IMAGES-ONLY", "#EXTINF:10.000,", "tiles/0.jpg", "#EXT-X-ENDLIST"} {
		if !strings.Contains(playlist, want) {
			t.Fatalf("playlist missing %q: %s", want, playlist)
		}
	}
	if strings.Contains(playlist, tileDir) || strings.Contains(playlist, tilePath) {
		t.Fatalf("playlist exposed managed path: %s", playlist)
	}
}

func playbackMediaGrantForTest(t *testing.T, client *http.Client, serverURL, mediaID string) string {
	t.Helper()
	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, serverURL+"/api/playback-sessions", PlaybackSessionCreateRequest{MediaID: mediaID, SkipPreroll: true, ClientProfile: authenticatedPlaybackRuntimeProfile()}, &playback)
	if status != http.StatusOK || playback.MediaGrant.Token == "" {
		t.Fatalf("create playback media grant status=%d body=%s", status, body)
	}
	return playback.MediaGrant.Token
}

func TestMediaSubtitleUploadConvertsSRT(t *testing.T) {
	serverURL, db := newAuthTestServerWithDB(t)
	seedExactPlaybackFactsForFixture(t, &Server{db: db}, "movie_meridian")
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("language", "en"); err != nil {
		t.Fatalf("write language: %v", err)
	}
	part, err := writer.CreateFormFile("file", "english.srt")
	if err != nil {
		t.Fatalf("create subtitle part: %v", err)
	}
	if _, err := part.Write([]byte("1\r\n00:00:01,250 --> 00:00:03,500\r\nHello from SRT\r\n\r\n")); err != nil {
		t.Fatalf("write subtitle part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/media/movie_meridian/subtitles", &body)
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfHeaderName, "1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload subtitle: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d, body: %s", resp.StatusCode, body)
	}
	var item MediaItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if len(item.Streams) != 0 {
		t.Fatalf("subtitle upload response should stay lightweight, got streams: %#v", item.Streams)
	}
	status, bodyText := doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", status, bodyText)
	}
	var uploaded Stream
	for _, stream := range item.Streams {
		if stream.Kind == "subtitle" && stream.Codec == "webvtt" && stream.DisplayTitle == "Uploaded EN" {
			uploaded = stream
			break
		}
	}
	if uploaded.ID == "" {
		t.Fatalf("uploaded subtitle stream missing: %#v", item.Streams)
	}
	uploaded.SourceURL = "/api/media/movie_meridian/subtitles/" + uploaded.ID
	streamRequest, err := http.NewRequest(http.MethodGet, serverURL+uploaded.SourceURL, nil)
	if err != nil {
		t.Fatalf("create subtitle stream request: %v", err)
	}
	streamRequest.Header.Set("Authorization", "PorticoMedia "+playbackMediaGrantForTest(t, client, serverURL, "movie_meridian"))
	streamResp, err := client.Do(streamRequest)
	if err != nil {
		t.Fatalf("stream subtitle: %v", err)
	}
	streamBytes, _ := io.ReadAll(streamResp.Body)
	_ = streamResp.Body.Close()
	text := string(streamBytes)
	if streamResp.StatusCode != http.StatusOK || !strings.Contains(text, "WEBVTT") || !strings.Contains(text, "00:00:01.250 --> 00:00:03.500") {
		t.Fatalf("subtitle stream status=%d body=%s", streamResp.StatusCode, streamBytes)
	}
}

func TestSubtitleNormalizationSupportsBroadTextFormats(t *testing.T) {
	cases := map[string][]byte{
		"dialogue.ass": []byte("[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:03.00,Default,Hello {\\i1}there\n"),
		"captions.sbv": []byte("0:00:01.000,0:00:03.000\nHello SBV\n"),
		"legacy.sub":   []byte("00:00:01.000,00:00:03.000\nHello SUB\n"),
		"timed.ttml":   []byte(`<?xml version="1.0"?><tt><body><div><p begin="00:00:01.000" end="00:00:03.000">Hello TTML</p></div></body></tt>`),
	}
	for filename, input := range cases {
		output, err := normalizeUploadedSubtitle(filename, input)
		if err != nil {
			t.Fatalf("normalize %s: %v", filename, err)
		}
		text := string(output)
		if !strings.HasPrefix(text, "WEBVTT") || !strings.Contains(text, "00:00:01.000 --> 00:00:03.000") {
			t.Fatalf("unexpected normalized subtitle for %s:\n%s", filename, text)
		}
	}
}

func TestMediaLyricsUploadAndDelete(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	// Public lyric IDs are intentionally opaque. Capture the scanned lyric from
	// the API before uploading another source rather than coupling this test to
	// its private database identity.
	var baseline MediaItem
	status, bodyText := doJSON(t, client, http.MethodGet, serverURL+"/api/media/track_mara_01", nil, &baseline)
	if status != http.StatusOK {
		t.Fatalf("baseline detail status=%d body=%s", status, bodyText)
	}
	var scanned MediaLyric
	for _, lyric := range baseline.Lyrics {
		if strings.Contains(lyric.Text, "Platform lights are blinking in time") {
			scanned = lyric
			break
		}
	}
	if scanned.ID == "" {
		t.Fatalf("baseline scanned lyric missing: %#v", baseline.Lyrics)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("language", "en-US"); err != nil {
		t.Fatalf("write language: %v", err)
	}
	part, err := writer.CreateFormFile("file", "platform-lights.lrc")
	if err != nil {
		t.Fatalf("create lyric part: %v", err)
	}
	if _, err := part.Write([]byte("[00:01.00]Manual synced line\n[00:02.50]Second line\n")); err != nil {
		t.Fatalf("write lyric part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/media/track_mara_01/lyrics", &body)
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfHeaderName, "1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload lyrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d, body: %s", resp.StatusCode, body)
	}
	var item MediaItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if len(item.Lyrics) != 0 {
		t.Fatalf("lyrics upload response should stay lightweight, got lyrics: %#v", item.Lyrics)
	}
	status, bodyText = doJSON(t, client, http.MethodGet, serverURL+"/api/media/track_mara_01", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", status, bodyText)
	}
	var uploaded MediaLyric
	for _, lyric := range item.Lyrics {
		if lyric.Language == "en-us" && strings.Contains(lyric.Text, "Manual synced line") {
			uploaded = lyric
			break
		}
	}
	if uploaded.ID == "" || uploaded.Format != "lrc" || uploaded.Language != "en-us" || !uploaded.Synced || !strings.Contains(uploaded.Text, "Manual synced line") {
		t.Fatalf("uploaded lyric missing or malformed: %#v", item.Lyrics)
	}

	status, deleteBody := doJSON(t, client, http.MethodDelete, serverURL+"/api/media/track_mara_01/lyrics/"+uploaded.ID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("delete lyrics status = %d, body: %s", status, deleteBody)
	}
	var after MediaItem
	status, bodyText = doJSON(t, client, http.MethodGet, serverURL+"/api/media/track_mara_01", nil, &after)
	if status != http.StatusOK {
		t.Fatalf("detail status = %d, body: %s", status, bodyText)
	}
	for _, lyric := range after.Lyrics {
		if lyric.ID == uploaded.ID {
			t.Fatalf("uploaded lyric still present after delete: %#v", after.Lyrics)
		}
	}
	if !mediaLyricsContainID(after.Lyrics, scanned.ID) {
		t.Fatalf("deleting uploaded lyric removed local scanned lyrics: %#v", after.Lyrics)
	}

	// The same mutation endpoint must reject a scanned/local lyric even when a
	// caller supplies its valid public ID.
	status, deleteBody = doJSON(t, client, http.MethodDelete, serverURL+"/api/media/track_mara_01/lyrics/"+scanned.ID, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("delete scanned lyrics status = %d, want 404, body: %s", status, deleteBody)
	}
	var final MediaItem
	status, bodyText = doJSON(t, client, http.MethodGet, serverURL+"/api/media/track_mara_01", nil, &final)
	if status != http.StatusOK {
		t.Fatalf("final detail status = %d, body: %s", status, bodyText)
	}
	if !mediaLyricsContainID(final.Lyrics, scanned.ID) {
		t.Fatalf("rejected scanned lyric deletion still removed it: %#v", final.Lyrics)
	}
}

func TestFeatureSpecificLyricsAndSubtitlePermissions(t *testing.T) {
	serverURL := newAuthTestServer(t)
	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}
	loginUser(t, adminClient, serverURL)

	var lyricsUser User
	status, body := doJSON(t, adminClient, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username:    "lyrics",
		Email:       "lyrics@example.test",
		DisplayName: "Lyrics",
		Password:    "Password1234",
		Role:        "user",
		Permissions: map[string]bool{"playMedia": true, "manageLyrics": true, "manageSubtitles": false, "editMetadata": false},
		LibraryIDs:  []string{"lib_music", "lib_movies"},
	}, &lyricsUser)
	if status != http.StatusCreated {
		t.Fatalf("create lyrics user status = %d, body: %s", status, body)
	}
	lyricsJar, _ := cookiejar.New(nil)
	lyricsClient := &http.Client{Jar: lyricsJar}
	status, body = doJSON(t, lyricsClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": "lyrics", "password": "Password1234"}, nil)
	if status != http.StatusOK {
		t.Fatalf("lyrics login status = %d, body: %s", status, body)
	}
	status, body = uploadTestMultipart(t, lyricsClient, serverURL+"/api/media/track_mara_01/lyrics", "file", "line.lrc", "language", "en", []byte("[00:01.00]Line\n"))
	if status != http.StatusCreated {
		t.Fatalf("lyrics-specific upload status = %d, body: %s", status, body)
	}
	status, body = uploadTestMultipart(t, lyricsClient, serverURL+"/api/media/movie_meridian/subtitles", "file", "english.srt", "language", "en", []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"))
	if status != http.StatusForbidden {
		t.Fatalf("subtitle denied status = %d, body: %s", status, body)
	}

	var subtitlesUser User
	status, body = doJSON(t, adminClient, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username:    "subtitles",
		Email:       "subtitles@example.test",
		DisplayName: "Subtitles",
		Password:    "Password1234",
		Role:        "user",
		Permissions: map[string]bool{"playMedia": true, "manageLyrics": false, "manageSubtitles": true, "editMetadata": false},
		LibraryIDs:  []string{"lib_music", "lib_movies"},
	}, &subtitlesUser)
	if status != http.StatusCreated {
		t.Fatalf("create subtitles user status = %d, body: %s", status, body)
	}
	subtitleJar, _ := cookiejar.New(nil)
	subtitleClient := &http.Client{Jar: subtitleJar}
	status, body = doJSON(t, subtitleClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": "subtitles", "password": "Password1234"}, nil)
	if status != http.StatusOK {
		t.Fatalf("subtitles login status = %d, body: %s", status, body)
	}
	status, body = uploadTestMultipart(t, subtitleClient, serverURL+"/api/media/movie_meridian/subtitles", "file", "english.srt", "language", "en", []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"))
	if status != http.StatusCreated {
		t.Fatalf("subtitle-specific upload status = %d, body: %s", status, body)
	}
	status, body = uploadTestMultipart(t, subtitleClient, serverURL+"/api/media/track_mara_01/lyrics", "file", "line.lrc", "language", "en", []byte("[00:01.00]Line\n"))
	if status != http.StatusForbidden {
		t.Fatalf("lyrics denied status = %d, body: %s", status, body)
	}
}

func uploadTestMultipart(t *testing.T, client *http.Client, endpoint string, fileField string, filename string, fieldName string, fieldValue string, data []byte) (int, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if fieldName != "" {
		if err := writer.WriteField(fieldName, fieldValue); err != nil {
			t.Fatalf("write multipart field: %v", err)
		}
	}
	part, err := writer.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		t.Fatalf("create multipart request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfHeaderName, "1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send multipart request: %v", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read multipart response: %v", err)
	}
	return resp.StatusCode, string(responseBody)
}

func mediaLyricsContainID(lyrics []MediaLyric, id string) bool {
	for _, lyric := range lyrics {
		if lyric.ID == id {
			return true
		}
	}
	return false
}

func TestMissingArtworkDoesNotSynthesizeAPlaceholder(t *testing.T) {
	serverURL, _, _ := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	resp, err := client.Get(serverURL + "/api/artwork/movie_saffron/poster.svg")
	if err != nil {
		t.Fatalf("load generated artwork: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || len(body) != 0 || resp.Header.Get("X-Portico-Optional-Resource") != "artwork-unavailable" {
		t.Fatalf("missing artwork status=%d body=%s", resp.StatusCode, string(body))
	}
}

func TestPosterArtworkFallsBackToGeneratedFrameWithoutClaimingPoster(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	if _, err := server.db.Exec(`DELETE FROM media_images WHERE media_id = ? AND image_type = 'poster'`, "movie_meridian"); err != nil {
		t.Fatalf("clear poster images: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET artwork_json = '{}' WHERE id = ?`, "movie_meridian"); err != nil {
		t.Fatalf("clear artwork map: %v", err)
	}
	thumbDir := filepath.Join(server.cfg.AppDataDir, "artwork", safePathComponent("movie_meridian"))
	if err := os.MkdirAll(thumbDir, 0o700); err != nil {
		t.Fatalf("create generated thumb dir: %v", err)
	}
	frameBytes := []byte("temporary-frame")
	if err := os.WriteFile(filepath.Join(thumbDir, "thumb.jpg"), frameBytes, 0o600); err != nil {
		t.Fatalf("write generated thumb: %v", err)
	}
	resp, err := client.Get(serverURL + "/api/artwork/movie_meridian/poster.svg")
	if err != nil {
		t.Fatalf("load poster frame fallback: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, frameBytes) {
		t.Fatalf("poster fallback status=%d body=%q", resp.StatusCode, string(body))
	}
	var posterImages int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_images WHERE media_id = ? AND image_type = 'poster'`, "movie_meridian").Scan(&posterImages); err != nil {
		t.Fatalf("count poster images: %v", err)
	}
	if posterImages != 0 {
		t.Fatalf("generated frame fallback should not be persisted as poster image, count=%d", posterImages)
	}
}

func TestMediaImageUploadServesAndDeletesManualArtwork(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEElEQVR4nGL6z8AACAAA//8DCQECWLbVUAAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", "poster"); err != nil {
		t.Fatalf("write type: %v", err)
	}
	current, err := server.getMedia("", "movie_meridian")
	if err != nil {
		t.Fatalf("load current metadata revision: %v", err)
	}
	if err := writer.WriteField("expectedRevision", strconv.Itoa(current.MetadataRevision)); err != nil {
		t.Fatalf("write expected revision: %v", err)
	}
	part, err := writer.CreateFormFile("file", "poster.png")
	if err != nil {
		t.Fatalf("create image part: %v", err)
	}
	if _, err := part.Write(pngBytes); err != nil {
		t.Fatalf("write image part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/media/movie_meridian/images", &body)
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfHeaderName, "1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload image: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d, body: %s", resp.StatusCode, body)
	}
	var item MediaItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if len(item.MediaImages) != 0 {
		t.Fatalf("image upload response should stay lightweight, got images: %#v", item.MediaImages)
	}
	status, infoBody := doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("detail status = %d, body: %s", status, infoBody)
	}
	var uploaded MediaImage
	for _, image := range item.MediaImages {
		if image.Type == "poster" && image.Preferred && image.Width == 1 && image.Height == 1 {
			uploaded = image
			break
		}
	}
	if uploaded.ID == "" || !uploaded.Preferred || uploaded.Width != 1 || uploaded.Height != 1 {
		t.Fatalf("uploaded image missing or malformed: %#v", item.MediaImages)
	}
	var imageInfo MediaImage
	status, infoBody = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian/images/"+uploaded.ID, nil, &imageInfo)
	if status != http.StatusOK {
		t.Fatalf("image info status = %d, body: %s", status, infoBody)
	}
	if imageInfo.ID != uploaded.ID || imageInfo.Type != "poster" || imageInfo.Width != 1 || imageInfo.Height != 1 {
		t.Fatalf("image info mismatch: %#v", imageInfo)
	}
	if imageInfo.Path == "" {
		t.Fatal("server manager image information omitted its filesystem path")
	}
	var viewer User
	status, infoBody = doJSON(t, client, http.MethodPost, serverURL+"/api/users", UserRequest{
		Username: "artwork-viewer", Email: "artwork-viewer@example.test", DisplayName: "Artwork Viewer",
		Password: "Password1234", Role: "user", Permissions: map[string]bool{"playMedia": true, "editMetadata": true}, LibraryIDs: []string{"lib_movies"},
	}, &viewer)
	if status != http.StatusCreated {
		t.Fatalf("create artwork viewer status = %d, body: %s", status, infoBody)
	}
	viewerJar, _ := cookiejar.New(nil)
	viewerClient := &http.Client{Jar: viewerJar}
	status, infoBody = doJSON(t, viewerClient, http.MethodPost, serverURL+"/api/auth/login", map[string]string{"login": viewer.Username, "password": "Password1234"}, nil)
	if status != http.StatusOK {
		t.Fatalf("artwork viewer login status = %d, body: %s", status, infoBody)
	}
	var viewerImageInfo MediaImage
	status, infoBody = doJSON(t, viewerClient, http.MethodGet, serverURL+"/api/media/movie_meridian/images/"+uploaded.ID, nil, &viewerImageInfo)
	if status != http.StatusOK {
		t.Fatalf("artwork viewer image info status = %d, body: %s", status, infoBody)
	}
	if viewerImageInfo.Path != "" {
		t.Fatalf("ordinary viewer received server filesystem path %q", viewerImageInfo.Path)
	}
	_ = uploadTestArtwork(t, client, serverURL, "movie_meridian", pngBytes)
	var second MediaItem
	status, infoBody = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &second)
	if status != http.StatusOK {
		t.Fatalf("detail status = %d, body: %s", status, infoBody)
	}
	if mediaImagePreferred(second.MediaImages, uploaded.ID) {
		t.Fatalf("first uploaded image should not remain preferred after second upload: %#v", second.MediaImages)
	}
	manualIDs := manualMediaImageIDs(second.MediaImages, "poster")
	if len(manualIDs) < 2 {
		t.Fatalf("expected two manual poster images: %#v", second.MediaImages)
	}
	var ordered MediaItem
	status, orderBody := doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/images/order", map[string]any{
		"imageIds": []string{manualIDs[1], manualIDs[0]}, "expectedRevision": second.MetadataRevision,
	}, &ordered)
	if status != http.StatusOK {
		t.Fatalf("order image status = %d, body: %s", status, orderBody)
	}
	if len(ordered.MediaImages) != 0 {
		t.Fatalf("image order response should stay lightweight, got images: %#v", ordered.MediaImages)
	}
	status, orderBody = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &ordered)
	if status != http.StatusOK {
		t.Fatalf("detail status = %d, body: %s", status, orderBody)
	}
	if mediaImageSortOrder(ordered.MediaImages, manualIDs[1]) != 0 || mediaImageSortOrder(ordered.MediaImages, manualIDs[0]) != 1 {
		t.Fatalf("image order was not updated: %#v", ordered.MediaImages)
	}
	var preferredResponse MediaItem
	status, preferBody := doJSON(t, client, http.MethodPost, serverURL+"/api/media/movie_meridian/images/"+uploaded.ID+"/preferred", map[string]any{"expectedRevision": ordered.MetadataRevision}, &preferredResponse)
	if status != http.StatusOK {
		t.Fatalf("prefer image status = %d, body: %s", status, preferBody)
	}
	if len(preferredResponse.MediaImages) != 0 {
		t.Fatalf("image preferred response should stay lightweight, got images: %#v", preferredResponse.MediaImages)
	}
	status, preferBody = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("detail status = %d, body: %s", status, preferBody)
	}
	if !mediaImagePreferred(item.MediaImages, uploaded.ID) {
		t.Fatalf("preferred image was not updated: %#v", item.MediaImages)
	}
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json = ? WHERE id = ?`, `{"imagePolicy":"provider_first"}`, "lib_movies"); err != nil {
		t.Fatalf("set provider-first artwork policy: %v", err)
	}
	if err := server.replaceProviderMediaImages("movie_meridian", map[string]string{"source": "tmdb", "posterPath": "/provider-poster.jpg"}); err != nil {
		t.Fatalf("replace provider images: %v", err)
	}
	status, preferBody = doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &item)
	if status != http.StatusOK {
		t.Fatalf("detail status = %d, body: %s", status, preferBody)
	}
	if !mediaImagePreferred(item.MediaImages, uploaded.ID) {
		t.Fatalf("provider refresh should not displace manual preferred artwork: %#v", item.MediaImages)
	}
	artResp, err := client.Get(serverURL + "/api/artwork/movie_meridian/poster.svg")
	if err != nil {
		t.Fatalf("load artwork: %v", err)
	}
	artBytes, _ := io.ReadAll(artResp.Body)
	_ = artResp.Body.Close()
	if artResp.StatusCode != http.StatusOK || !bytes.HasPrefix(artBytes, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("artwork status=%d prefix=%x", artResp.StatusCode, artBytes[:min(len(artBytes), 8)])
	}
	resizedResp, err := client.Get(serverURL + "/api/artwork/movie_meridian/poster.svg?width=64&height=64")
	if err != nil {
		t.Fatalf("load resized artwork: %v", err)
	}
	config, format, err := image.DecodeConfig(resizedResp.Body)
	_ = resizedResp.Body.Close()
	if resizedResp.StatusCode != http.StatusOK || err != nil || format != "jpeg" || config.Width != 64 || config.Height != 64 {
		t.Fatalf("resized artwork status=%d format=%s config=%#v err=%v", resizedResp.StatusCode, format, config, err)
	}
	cacheDir := filepath.Join(server.cfg.AppDataDir, "image-cache", "artwork")
	cacheEntries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("read artwork image cache: %v", err)
	}
	if len(cacheEntries) != 1 || cacheEntries[0].IsDir() || filepath.Ext(cacheEntries[0].Name()) != ".jpg" {
		t.Fatalf("unexpected artwork image cache entries: %#v", cacheEntries)
	}
	status, deleteBody := doJSON(t, client, http.MethodDelete, serverURL+"/api/media/movie_meridian/images/"+uploaded.ID+"?expectedRevision="+strconv.Itoa(item.MetadataRevision), nil, nil)
	if status != http.StatusOK {
		t.Fatalf("delete image status = %d, body: %s", status, deleteBody)
	}
	var after MediaItem
	status, bodyText := doJSON(t, client, http.MethodGet, serverURL+"/api/media/movie_meridian", nil, &after)
	if status != http.StatusOK {
		t.Fatalf("detail status = %d, body: %s", status, bodyText)
	}
	for _, image := range after.MediaImages {
		if image.ID == uploaded.ID {
			t.Fatalf("uploaded image still present after delete: %#v", after.MediaImages)
		}
	}
}

func TestValidateUploadedArtworkAcceptsWebP(t *testing.T) {
	webpBytes, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatalf("decode webp: %v", err)
	}
	extension, width, height, err := validateUploadedArtwork(webpBytes, "cover.webp")
	if err != nil {
		t.Fatalf("validate webp: %v", err)
	}
	if extension != "webp" || width != 1 || height != 1 {
		t.Fatalf("webp validation = ext %q %dx%d", extension, width, height)
	}
}

func uploadTestArtwork(t *testing.T, client *http.Client, serverURL, mediaID string, imageBytes []byte) MediaItem {
	t.Helper()
	var current MediaItem
	status, responseBody := doJSON(t, client, http.MethodGet, serverURL+"/api/media/"+mediaID, nil, &current)
	if status != http.StatusOK {
		t.Fatalf("load media revision status=%d body=%s", status, responseBody)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", "poster"); err != nil {
		t.Fatalf("write type: %v", err)
	}
	if err := writer.WriteField("expectedRevision", strconv.Itoa(current.MetadataRevision)); err != nil {
		t.Fatalf("write expected revision: %v", err)
	}
	part, err := writer.CreateFormFile("file", "poster.png")
	if err != nil {
		t.Fatalf("create image part: %v", err)
	}
	if _, err := part.Write(imageBytes); err != nil {
		t.Fatalf("write image part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/media/"+mediaID+"/images", &body)
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfHeaderName, "1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload image: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d, body: %s", resp.StatusCode, body)
	}
	var item MediaItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	return item
}

func mediaImagePreferred(images []MediaImage, id string) bool {
	for _, image := range images {
		if image.ID == id {
			return image.Preferred
		}
	}
	return false
}

func manualMediaImageIDs(images []MediaImage, imageType string) []string {
	var ids []string
	for _, image := range images {
		if image.Type == imageType {
			ids = append(ids, image.ID)
		}
	}
	return ids
}

func mediaImageSortOrder(images []MediaImage, id string) int {
	for _, image := range images {
		if image.ID == id {
			return image.SortOrder
		}
	}
	return -1
}

func TestAccountProfileImageUploadStreamAndDelete(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEElEQVR4nGL6z8AACAAA//8DCQECWLbVUAAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "profile.png")
	if err != nil {
		t.Fatalf("create profile part: %v", err)
	}
	if _, err := part.Write(pngBytes); err != nil {
		t.Fatalf("write profile part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/account/image", &body)
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set(csrfHeaderName, "1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload profile image: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d, body: %s", resp.StatusCode, body)
	}
	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if user.ProfileImageURL == "" {
		t.Fatalf("profile image url was not returned: %#v", user)
	}
	imageResp, err := client.Get(serverURL + user.ProfileImageURL)
	if err != nil {
		t.Fatalf("stream profile image: %v", err)
	}
	imageBytes, _ := io.ReadAll(imageResp.Body)
	_ = imageResp.Body.Close()
	if imageResp.StatusCode != http.StatusOK || !bytes.HasPrefix(imageBytes, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("profile image status=%d prefix=%x", imageResp.StatusCode, imageBytes[:min(len(imageBytes), 8)])
	}
	resizedResp, err := client.Get(serverURL + user.ProfileImageURL + "&width=96&height=96")
	if err != nil {
		t.Fatalf("stream resized profile image: %v", err)
	}
	config, format, err := image.DecodeConfig(resizedResp.Body)
	_ = resizedResp.Body.Close()
	if resizedResp.StatusCode != http.StatusOK || err != nil || format != "jpeg" || config.Width != 96 || config.Height != 96 {
		t.Fatalf("resized profile image status=%d format=%s config=%#v err=%v", resizedResp.StatusCode, format, config, err)
	}
	cacheDir := filepath.Join(server.cfg.AppDataDir, "image-cache", "profile")
	cacheEntries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("read profile image cache: %v", err)
	}
	if len(cacheEntries) != 1 || cacheEntries[0].IsDir() || filepath.Ext(cacheEntries[0].Name()) != ".jpg" {
		t.Fatalf("unexpected profile image cache entries: %#v", cacheEntries)
	}
	user = User{}
	status, deleteBody := doJSON(t, client, http.MethodDelete, serverURL+"/api/account/image", nil, &user)
	if status != http.StatusOK {
		t.Fatalf("delete profile image status = %d, body: %s", status, deleteBody)
	}
	if user.ProfileImageURL != "" {
		t.Fatalf("profile image url was not cleared: %#v", user)
	}
}

func TestUserMaxContentRatingFiltersMediaQueries(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, max_content_rating, created_at, updated_at)
		VALUES ('usr_restricted', 'restricted', 'restricted@example.test', 'Restricted', 'hash', 'user', '{}', '{}', 'PG', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("insert restricted user: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES ('usr_restricted', 'lib_movies', ?)`, now); err != nil {
		t.Fatalf("grant restricted user library access: %v", err)
	}
	items, err := server.queryMedia("usr_restricted", `WHERE m.id IN ('movie_saffron', 'movie_meridian', 'movie_neon') ORDER BY m.sort_title ASC`, nil)
	if err != nil {
		t.Fatalf("query restricted media: %v", err)
	}
	if len(items) != 1 || items[0].ID != "movie_saffron" {
		t.Fatalf("restricted items = %#v, expected only PG movie_saffron", items)
	}
	if !contentRatingAllowed("14A", "PG-13") || contentRatingAllowed("18", "PG-13") || contentRatingAllowed("Not Rated", "PG-13") {
		t.Fatalf("localized rating ranks should allow 14A, block 18, and fail closed for unknown ratings")
	}
	if normalizeMaxContentRating("18A") != "18A" || normalizeMaxContentRating("12A") != "12A" || normalizeMaxContentRating("C8") != "C8" {
		t.Fatalf("localized max content ratings should normalize as policy values")
	}
}

func TestMediaTreeDeletionAuthorizationIncludesRestrictedDescendants(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, max_content_rating, created_at, updated_at)
		VALUES ('usr_tree_delete', 'tree-delete', 'tree-delete@example.test', 'Tree Delete', 'hash', 'user', '{"deleteMedia":true}', '{}', 'PG', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES ('usr_tree_delete', 'lib_movies', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET content_rating='PG' WHERE id='movie_saffron'`); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET parent_id='movie_saffron',content_rating='R' WHERE id='movie_meridian'`); err != nil {
		t.Fatal(err)
	}
	if err := server.authorizeMediaTreeDeletionContext(context.Background(), "usr_tree_delete", "movie_saffron"); !errors.Is(err, errMediaTreeNotFullyVisible) {
		t.Fatalf("tree authorization error = %v, want restricted-descendant rejection", err)
	}
}

func TestManualContentRatingUpdateStoresRatingEvidence(t *testing.T) {
	server := newScannerTestServer(t)
	rating := "14A"
	country := "CA"
	if _, err := server.updateMedia("", "movie_meridian", UpdateMediaRequest{ContentRating: &rating, Country: &country}); err != nil {
		t.Fatalf("update media rating: %v", err)
	}
	var rawRating, evidenceCountry, ratingSystem string
	var normalizedRank int
	if err := server.db.QueryRow(`
		SELECT raw_rating, country, rating_system, normalized_rank
		FROM media_rating_evidence
		WHERE media_id = 'movie_meridian' AND provider = 'manual' AND source = 'editor'`).Scan(&rawRating, &evidenceCountry, &ratingSystem, &normalizedRank); err != nil {
		t.Fatalf("load manual rating evidence: %v", err)
	}
	if rawRating != "14A" || evidenceCountry != "CA" || ratingSystem != "CHVRS" || normalizedRank != 4 {
		t.Fatalf("manual rating evidence = %q %q %q %d", rawRating, evidenceCountry, ratingSystem, normalizedRank)
	}
}

func TestUserTagPolicyFiltersMediaQueriesAndPlaybackLookups(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	preferencesJSON, _ := marshalUserPreferencesWithPolicies(defaultUserPreferences(), UserAccessSchedule{}, UserTagPolicy{
		AllowedTags:   []string{"kids"},
		BlockedTags:   []string{"blocked"},
		BlockedLabels: []string{"private"},
	}, UserDevicePolicy{Mode: "any"}, UserChannelPolicy{})
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_tagged', 'tagged', 'tagged@example.test', 'Tagged', 'hash', 'user', '{}', ?, ?, ?)`,
		string(preferencesJSON), now, now); err != nil {
		t.Fatalf("insert tagged user: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES ('usr_tagged', 'lib_movies', ?)`, now); err != nil {
		t.Fatalf("grant tagged user library access: %v", err)
	}
	updates := map[string]string{
		"movie_saffron":  `["kids"]`,
		"movie_meridian": `["kids","blocked"]`,
		"movie_neon":     `["adult"]`,
	}
	for mediaID, tagsJSON := range updates {
		if _, err := server.db.Exec(`UPDATE media_items SET tags_json = ? WHERE id = ?`, tagsJSON, mediaID); err != nil {
			t.Fatalf("seed %s tags: %v", mediaID, err)
		}
	}
	if _, err := server.db.Exec(`UPDATE media_items SET labels_json = ? WHERE id = ?`, `["private"]`, "movie_saffron"); err != nil {
		t.Fatalf("seed movie_saffron private label: %v", err)
	}
	for _, mediaID := range []string{"movie_saffron", "movie_meridian", "movie_neon"} {
		if err := server.replaceMediaCategoryFacets(mediaID); err != nil {
			t.Fatalf("refresh %s tag policy facets: %v", mediaID, err)
		}
	}

	items, err := server.queryMedia("usr_tagged", `WHERE m.id IN ('movie_saffron', 'movie_meridian', 'movie_neon') ORDER BY m.sort_title ASC`, nil)
	if err != nil {
		t.Fatalf("query tag-restricted media: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("tag/label-restricted items = %#v, expected none", items)
	}
	if _, err := server.getMediaDetail("usr_tagged", "movie_meridian"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("blocked tag media detail err = %v, expected sql.ErrNoRows", err)
	}
	if _, err := server.getMediaDetail("usr_tagged", "movie_neon"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing allowed tag media detail err = %v, expected sql.ErrNoRows", err)
	}
	if _, err := server.getMediaDetail("usr_tagged", "movie_saffron"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("blocked label media detail err = %v, expected sql.ErrNoRows", err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET labels_json = '[]' WHERE id = ?`, "movie_saffron"); err != nil {
		t.Fatalf("clear movie_saffron private label: %v", err)
	}
	if err := server.replaceMediaCategoryFacets("movie_saffron"); err != nil {
		t.Fatalf("refresh movie_saffron cleared label facets: %v", err)
	}
	if _, err := server.getMediaDetail("usr_tagged", "movie_saffron"); err != nil {
		t.Fatalf("allowed tag and unblocked label media detail should be accessible: %v", err)
	}
}

func TestEncodeLibrarySettingsNormalizesCurationLists(t *testing.T) {
	raw, err := encodeLibrarySettings(map[string]any{
		"allowedTags":     "Family, Documentary\nFamily",
		"blockedTags":     []any{"Horror", "", "Horror"},
		"blockedKeywords": []string{"Blackwater", " climate "},
		"unknownKey":      "discarded",
	})
	if err != nil {
		t.Fatalf("encode library settings: %v", err)
	}
	var settings map[string][]string
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		t.Fatalf("decode library settings: %v", err)
	}
	if got := strings.Join(settings["allowedTags"], ","); got != "documentary,family" {
		t.Fatalf("allowedTags = %q, expected documentary,family", got)
	}
	if got := strings.Join(settings["blockedTags"], ","); got != "horror" {
		t.Fatalf("blockedTags = %q, expected horror", got)
	}
	if got := strings.Join(settings["blockedKeywords"], ","); got != "blackwater,climate" {
		t.Fatalf("blockedKeywords = %q, expected blackwater,climate", got)
	}
	if _, ok := settings["unknownKey"]; ok {
		t.Fatalf("unknown setting was persisted: %s", raw)
	}

	raw, err = encodeLibrarySettings(map[string]any{
		"allowedTags":     "",
		"blockedTags":     []string{},
		"blockedKeywords": []any{""},
	})
	if err != nil {
		t.Fatalf("encode blank library settings: %v", err)
	}
	if raw != "{}" {
		t.Fatalf("blank curation lists should be omitted, got %s", raw)
	}
}

func TestLibraryCurationFiltersUserFacingMediaQueries(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	settings, err := encodeLibrarySettings(map[string]any{
		"allowedTags":     []string{"kids"},
		"blockedTags":     []string{"blocked"},
		"blockedKeywords": []string{"pacific"},
	})
	if err != nil {
		t.Fatalf("encode library curation settings: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json = ? WHERE id = 'lib_movies'`, settings); err != nil {
		t.Fatalf("save library curation settings: %v", err)
	}
	tagUpdates := map[string]string{
		"movie_saffron":  `["kids"]`,
		"movie_meridian": `["kids","blocked"]`,
		"movie_neon":     `["adult"]`,
		"movie_pacific":  `["kids"]`,
	}
	for mediaID, tagsJSON := range tagUpdates {
		if _, err := server.db.Exec(`UPDATE media_items SET tags_json = ? WHERE id = ?`, tagsJSON, mediaID); err != nil {
			t.Fatalf("seed %s tags: %v", mediaID, err)
		}
		if err := server.replaceMediaCategoryFacets(mediaID); err != nil {
			t.Fatalf("refresh %s curation facets: %v", mediaID, err)
		}
	}

	items, total, _, _, _, _, err := server.listLibraryItemsPageContext(context.Background(), user.ID, "lib_movies", "title", "all", "asc", "exact", "", 50, 0)
	if err != nil {
		t.Fatalf("list curation-filtered library items: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != "movie_saffron" {
		t.Fatalf("curation-filtered library items total=%d items=%#v, expected only movie_saffron", total, items)
	}

	aggregates, err := server.queryLibraryCategoryFacetAggregatesContext(context.Background(), "lib_movies", user.ID)
	if err != nil {
		t.Fatalf("load curation-filtered category aggregates: %v", err)
	}
	if aggregate, ok := aggregates["genre:drama"]; !ok || aggregate.count != 1 {
		t.Fatalf("genre:drama aggregate = %#v, present=%v; expected one visible item", aggregate, ok)
	}
	if aggregate, ok := aggregates["genre:science fiction"]; ok {
		t.Fatalf("hidden science fiction aggregate leaked through curation: %#v", aggregate)
	}

	if _, err := server.getMediaDetail(user.ID, "movie_meridian"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("blocked tag media detail err = %v, expected sql.ErrNoRows", err)
	}
	if _, err := server.getMediaDetail(user.ID, "movie_pacific"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("blocked keyword media detail err = %v, expected sql.ErrNoRows", err)
	}
	if _, err := server.getMediaDetail(user.ID, "movie_saffron"); err != nil {
		t.Fatalf("allowed curation media detail should be accessible: %v", err)
	}
	for name, lookup := range map[string]func(string) error{
		"artwork seed": func(id string) error {
			_, err := server.getMediaArtworkSeedContext(context.Background(), user.ID, id)
			return err
		},
		"subtitle and lyric mutation seed": func(id string) error {
			_, err := server.getMediaLyricLookupSeedContext(context.Background(), user.ID, id)
			return err
		},
		"playback detail": func(id string) error {
			_, err := server.getMediaPlaybackDetailForUser(context.Background(), user, id)
			return err
		},
		"stream seed": func(id string) error {
			_, err := server.getMediaStreamSeedForUser(context.Background(), user, id)
			return err
		},
		"download seed": func(id string) error {
			_, err := server.getMediaDownloadSeedForUser(context.Background(), user, id)
			return err
		},
	} {
		if err := lookup("movie_meridian"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("blocked tag %s err = %v, expected sql.ErrNoRows", name, err)
		}
		if err := lookup("movie_saffron"); err != nil {
			t.Fatalf("allowed curation %s should be accessible: %v", name, err)
		}
	}

	searchItems, err := server.searchMedia(user.ID, "Pacific", 20)
	if err != nil {
		t.Fatalf("search blocked keyword media: %v", err)
	}
	if mediaIDsContain(searchItems, "movie_pacific") {
		t.Fatalf("blocked keyword media appeared in search: %#v", searchItems)
	}
	searchItems, err = server.searchMedia(user.ID, "Saffron", 20)
	if err != nil {
		t.Fatalf("search allowed media: %v", err)
	}
	if !mediaIDsContain(searchItems, "movie_saffron") {
		t.Fatalf("allowed media missing from search: %#v", searchItems)
	}
}

func TestUserLibraryAccessFiltersMediaQueriesAndPlaybackLookups(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	permissionsJSON, _ := json.Marshal(map[string]bool{"playMedia": true})
	if _, err := server.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_movies_only', 'moviesonly', 'moviesonly@example.test', 'Movies Only', 'hash', 'user', ?, '{}', ?, ?)`,
		string(permissionsJSON), now, now); err != nil {
		t.Fatalf("insert movies-only user: %v", err)
	}
	if _, err := server.db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES ('usr_movies_only', 'lib_movies', ?)`, now); err != nil {
		t.Fatalf("grant movies library access: %v", err)
	}

	items, err := server.queryMedia("usr_movies_only", `WHERE m.id IN ('movie_saffron', 'album_mara') ORDER BY m.sort_title ASC`, nil)
	if err != nil {
		t.Fatalf("query movies-only media: %v", err)
	}
	if len(items) != 1 || items[0].ID != "movie_saffron" {
		t.Fatalf("movies-only items = %#v, expected only shared movie", items)
	}
	if _, err := server.getMediaDetail("usr_movies_only", "album_mara"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unshared media detail err = %v, expected sql.ErrNoRows", err)
	}
	if _, err := server.getMediaDetail("usr_movies_only", "movie_saffron"); err != nil {
		t.Fatalf("shared media detail should be accessible: %v", err)
	}
}

func TestSearchMediaMatchesExpandedFields(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE media_items SET typed_metadata_json = ? WHERE id = ?`, `{"albumArtist":"Mara Vale","label":"Night Harbor Records"}`, "album_mara"); err != nil {
		t.Fatalf("seed typed metadata: %v", err)
	}
	album, err := server.getMediaDetail("", "album_mara")
	if err != nil {
		t.Fatalf("load album search fixture: %v", err)
	}
	if err := server.refreshMediaSearch(album); err != nil {
		t.Fatalf("refresh album search fixture: %v", err)
	}
	items, err := server.searchMedia("", "Mara Vale", 20)
	if err != nil {
		t.Fatalf("search studio: %v", err)
	}
	if !mediaIDsContain(items, "album_mara") {
		t.Fatalf("studio search did not find album_mara: %#v", items)
	}
	for _, item := range items {
		if item.ID == "album_mara" && (item.Summary != "" || item.Tagline != "" || item.SourceURL != "") {
			t.Fatalf("search result included detail-only fields: %#v", item)
		}
	}
	items, err = server.searchMedia("", "Night Harbor", 20)
	if err != nil {
		t.Fatalf("search typed metadata: %v", err)
	}
	if !mediaIDsContain(items, "album_mara") {
		t.Fatalf("typed metadata search did not find album_mara: %#v", items)
	}
	items, err = server.searchMedia("", "2023", 20)
	if err != nil {
		t.Fatalf("search year: %v", err)
	}
	if !mediaIDsContain(items, "movie_neon") {
		t.Fatalf("year search did not find movie_neon: %#v", items)
	}
	items, err = server.searchMedia("", "R", 20)
	if err != nil {
		t.Fatalf("search content rating: %v", err)
	}
	if !mediaIDsContain(items, "movie_neon") {
		t.Fatalf("content-rating search did not find movie_neon: %#v", items)
	}
}

func TestLibraryVisibilityHidesItemsFromSearch(t *testing.T) {
	server := newScannerTestServer(t)
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json = ? WHERE id = 'lib_movies'`, `{"hideFromSearch":true}`); err != nil {
		t.Fatalf("hide library from search: %v", err)
	}
	items, err := server.searchMedia("", "Meridian", 20)
	if err != nil {
		t.Fatalf("search hidden library: %v", err)
	}
	if mediaIDsContain(items, "movie_meridian") {
		t.Fatalf("hidden library item appeared in search: %#v", items)
	}
}

func TestSetWatchedOnShowUpdatesEpisodes(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	if err := server.setWatched(user.ID, "show_northbridge", true); err != nil {
		t.Fatalf("set show watched: %v", err)
	}
	var watchedCount int
	if err := server.db.QueryRow(`
		SELECT COUNT(*)
		FROM user_media_state
		WHERE user_id = ? AND media_id IN ('episode_northbridge_101', 'episode_northbridge_102', 'episode_northbridge_103') AND watched = 1`,
		user.ID).Scan(&watchedCount); err != nil {
		t.Fatalf("count watched episodes: %v", err)
	}
	if watchedCount != 3 {
		t.Fatalf("watched episodes = %d, expected 3", watchedCount)
	}
}

func mediaIDsContain(items []MediaItem, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func stringSliceContains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func readSSEDataLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("sse event read: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		}
	}
}

func clientLogEvents(events []LogEvent) []LogEvent {
	matches := []LogEvent{}
	for _, event := range events {
		if strings.HasPrefix(event.Message, "Client log:") {
			matches = append(matches, event)
		}
	}
	return matches
}

func storageReportHasCategory(report SystemStorageReport, key string) bool {
	for _, category := range report.Categories {
		if category.Key == key {
			return true
		}
	}
	return false
}
