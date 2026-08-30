package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const viewerPreferencesVersion = "v1"

const (
	preferenceStartedThresholdMinimum = 1
	preferenceStartedThresholdMaximum = 25
	preferencePlayedThresholdMinimum  = 75
	preferenceCardSizeMinimum         = 75
	preferenceCardSizeMaximum         = 150
	preferenceAudioBitrateMinimum     = 32
	preferenceAudioBitrateMaximum     = 4096
	preferenceSearchHistoryMaximum    = 20
	preferenceSearchQueryMaximumRunes = 160
)

var preferenceVideoHeightChoices = []int{360, 480, 720, 1080, 1440, 2160, 4320}

var (
	errPreferenceConflict   = errors.New("preference revision conflict")
	errPreferenceVersion    = errors.New("unsupported preference version")
	errPreferenceScope      = errors.New("invalid preference scope")
	errPreferenceValidation = errors.New("invalid preference document")
)

type preferenceDocument struct {
	Version  string          `json:"version"`
	Revision int64           `json:"revision"`
	Values   json.RawMessage `json:"values"`
}

type preferencePatchRequest struct {
	Version          string          `json:"version"`
	ExpectedRevision int64           `json:"expectedRevision"`
	Changes          json.RawMessage `json:"changes"`
}

type profileActivationPreferenceRequest struct {
	Version          string `json:"version"`
	ExpectedRevision int64  `json:"expectedRevision"`
}

type preferenceScopeIdentity struct {
	Authority      string `json:"authority"`
	AccountID      string `json:"accountId"`
	ServerID       string `json:"serverId"`
	ProfileID      string `json:"profileId"`
	DeviceClass    string `json:"deviceClass"`
	InstallationID string `json:"installationId"`
}

type viewerPreferencePolicy struct {
	FeedbackAllowed         bool `json:"feedbackAllowed"`
	DownloadsAllowed        bool `json:"downloadsAllowed"`
	CellularQualityAllowed  bool `json:"cellularQualityAllowed"`
	MaximumVideoBitrateMbps int  `json:"maximumVideoBitrateMbps,omitempty"`
}

type viewerPreferenceBundle struct {
	Identity                    preferenceScopeIdentity `json:"identity"`
	ProfileServer               preferenceDocument      `json:"profileServer"`
	ProfileDeviceClass          preferenceDocument      `json:"profileDeviceClass"`
	AccountServerInstallation   preferenceDocument      `json:"accountServerInstallation"`
	EffectiveProfileDeviceClass preferenceDocument      `json:"effectiveProfileDeviceClass"`
	Policy                      viewerPreferencePolicy  `json:"policy"`
	ClampedFields               []string                `json:"clampedFields"`
}

type profileServerPreferenceValues struct {
	Localization struct {
		Locale     string `json:"locale"`
		TimeZone   string `json:"timeZone"`
		DateFormat string `json:"dateFormat"`
		HourCycle  string `json:"hourCycle"`
	} `json:"localization"`
	Home struct {
		RowOrder     []string `json:"rowOrder"`
		HiddenRowIDs []string `json:"hiddenRowIds"`
	} `json:"home"`
	Playback struct {
		AutoplayNext               bool     `json:"autoplayNext"`
		UpNextCountdownSeconds     int      `json:"upNextCountdownSeconds"`
		PassoutProtection          bool     `json:"passoutProtection"`
		PassoutAfterEpisodes       int      `json:"passoutAfterEpisodes"`
		IntroSkip                  string   `json:"introSkip"`
		CreditsSkip                string   `json:"creditsSkip"`
		StartedThresholdPercent    int      `json:"startedThresholdPercent"`
		PlayedThresholdPercent     int      `json:"playedThresholdPercent"`
		SkipBackSeconds            int      `json:"skipBackSeconds"`
		SkipForwardSeconds         int      `json:"skipForwardSeconds"`
		DefaultSpeed               float64  `json:"defaultSpeed"`
		PreferredAudioLanguages    []string `json:"preferredAudioLanguages"`
		PreferredSubtitleLanguages []string `json:"preferredSubtitleLanguages"`
		SubtitlesEnabled           bool     `json:"subtitlesEnabled"`
		SubtitleSize               string   `json:"subtitleSize"`
		SubtitleBackground         string   `json:"subtitleBackground"`
		ShowSyncedLyrics           bool     `json:"showSyncedLyrics"`
	} `json:"playback"`
	Music struct {
		ShuffleDefault     bool   `json:"shuffleDefault"`
		RepeatDefault      string `json:"repeatDefault"`
		AutoplayDefault    bool   `json:"autoplayDefault"`
		AudioNormalization string `json:"audioNormalization"`
		CrossfadeSeconds   int    `json:"crossfadeSeconds"`
		Gapless            bool   `json:"gapless"`
	} `json:"music"`
	Privacy struct {
		PauseWatchHistory         bool `json:"pauseWatchHistory"`
		ShowActivityToMembers     bool `json:"showActivityToMembers"`
		IncludeInWatchWithFriends bool `json:"includeInWatchWithFriends"`
	} `json:"privacy"`
	Search struct {
		RememberHistory bool     `json:"rememberHistory"`
		RecentQueries   []string `json:"recentQueries"`
	} `json:"search"`
	Downloads struct {
		Quality struct {
			Mode                string `json:"mode"`
			MaxVideoBitrateMbps *int   `json:"maxVideoBitrateMbps,omitempty"`
			MaxAudioBitrateKbps *int   `json:"maxAudioBitrateKbps,omitempty"`
			MaxVideoHeight      *int   `json:"maxVideoHeight,omitempty"`
		} `json:"quality"`
		DeleteWatched bool `json:"deleteWatched"`
	} `json:"downloads"`
}

type qualityPreferenceValues struct {
	Mode                string `json:"mode"`
	MaxVideoBitrateMbps *int   `json:"maxVideoBitrateMbps,omitempty"`
	MaxAudioBitrateKbps *int   `json:"maxAudioBitrateKbps,omitempty"`
	MaxVideoHeight      *int   `json:"maxVideoHeight,omitempty"`
	AllowHDR            *bool  `json:"allowHDR,omitempty"`
}

type profileDeviceClassPreferenceValues struct {
	DeviceClass string `json:"deviceClass"`
	Appearance  struct {
		Density         string `json:"density"`
		CardSizePercent int    `json:"cardSizePercent"`
		ShowBackdrops   bool   `json:"showBackdrops"`
	} `json:"appearance"`
	Navigation struct {
		SidebarCollapsed bool     `json:"sidebarCollapsed"`
		PinnedLibraryIDs []string `json:"pinnedLibraryIds"`
		DefaultLanding   string   `json:"defaultLanding"`
	} `json:"navigation"`
	Playback struct {
		DeliveryRequest struct {
			DirectPlay   string `json:"directPlay"`
			DirectStream string `json:"directStream"`
			Transcode    string `json:"transcode"`
		} `json:"deliveryRequest"`
		Quality map[string]qualityPreferenceValues `json:"quality"`
	} `json:"playback"`
}

type accountInstallationPreferenceValues struct {
	RememberAccount  bool   `json:"rememberAccount"`
	ProfileSelection string `json:"profileSelection"`
	LastProfileID    string `json:"lastProfileId,omitempty"`
}

type preferenceScope struct {
	Type           string
	Authority      string
	AccountID      string
	ProfileID      string
	ServerID       string
	DeviceClass    string
	InstallationID string
}

func (s preferenceScope) identity() preferenceScopeIdentity {
	return preferenceScopeIdentity{
		Authority: s.Authority, AccountID: s.AccountID, ServerID: s.ServerID,
		ProfileID: s.ProfileID, DeviceClass: s.DeviceClass, InstallationID: s.InstallationID,
	}
}

func (s *Server) handleViewerPreferences(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodGet {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use GET for this endpoint.")
		return
	}
	deviceClass := strings.TrimSpace(r.URL.Query().Get("deviceClass"))
	installationID := strings.TrimSpace(r.URL.Query().Get("installationId"))
	if (deviceClass != "" || installationID != "") && validateViewerPreferenceScopeInput(deviceClass, installationID) != nil {
		writeProductError(w, http.StatusBadRequest, "invalid_preferences", "Choose a valid device and installation for these preferences.")
		return
	}
	appSession, err := s.currentViewerPreferenceAppSession(r.Context(), r, user)
	if err != nil {
		s.writeViewerPreferenceSessionError(w, err, "load")
		return
	}
	if deviceClass == "" && installationID == "" {
		deviceClass, installationID = appSession.DeviceClass, appSession.InstallationID
	}
	scope, err := s.viewerPreferenceScope(r.Context(), user, deviceClass, installationID, "")
	if err != nil {
		if errors.Is(err, errPreferenceScope) {
			writeProductError(w, http.StatusBadRequest, "invalid_preferences", "Choose a valid device and installation for these preferences.")
		} else {
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "preferences_failed", "Unable to load preferences.")
		}
		return
	}
	if scope.InstallationID != appSession.InstallationID || scope.DeviceClass != appSession.DeviceClass {
		s.writeViewerPreferenceSessionError(w, errViewerPreferenceInteractiveSession, "load")
		return
	}
	bundle, err := s.loadViewerPreferenceBundle(r.Context(), user, scope)
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "preferences_failed", "Unable to load preferences.")
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) handleViewerPreferenceDocument(w http.ResponseWriter, r *http.Request, user User) {
	if r.URL.Path == "/api/preferences/profile-activation" {
		s.handleViewerProfileActivation(w, r, user)
		return
	}
	if r.Method != http.MethodPatch {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use PATCH for this endpoint.")
		return
	}
	scopeType := strings.TrimPrefix(r.URL.Path, "/api/preferences/")
	if strings.Contains(scopeType, "/") || !oneOf(scopeType, "profile-server", "profile-device-class", "account-server-installation") {
		writeProductError(w, http.StatusNotFound, "not_found", "Preference scope not found.")
		return
	}
	deviceClass := strings.TrimSpace(r.URL.Query().Get("deviceClass"))
	installationID := strings.TrimSpace(r.URL.Query().Get("installationId"))
	if (deviceClass != "" || installationID != "") && validateViewerPreferenceScopeInput(deviceClass, installationID) != nil {
		writeProductError(w, http.StatusBadRequest, "invalid_preferences", "Choose a valid device and installation for these preferences.")
		return
	}
	appSession, err := s.currentViewerPreferenceAppSession(r.Context(), r, user)
	if err != nil {
		s.writeViewerPreferenceSessionError(w, err, "save")
		return
	}
	if deviceClass == "" && installationID == "" {
		deviceClass, installationID = appSession.DeviceClass, appSession.InstallationID
	}
	scope, err := s.viewerPreferenceScope(r.Context(), user, deviceClass, installationID, scopeType)
	if err != nil {
		if errors.Is(err, errPreferenceScope) {
			writeProductError(w, http.StatusBadRequest, "invalid_preferences", "Choose a valid device and installation for these preferences.")
		} else {
			writeDatabaseAccessError(w, err, http.StatusInternalServerError, "preferences_failed", "Unable to save preferences.")
		}
		return
	}
	if scope.InstallationID != appSession.InstallationID || scope.DeviceClass != appSession.DeviceClass {
		s.writeViewerPreferenceSessionError(w, errViewerPreferenceInteractiveSession, "save")
		return
	}
	var request preferencePatchRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Version != viewerPreferencesVersion {
		writeProductError(w, http.StatusBadRequest, "unsupported_preference_version", "This preference document version is not supported.")
		return
	}
	if request.ExpectedRevision < 0 || len(request.Changes) == 0 {
		writeProductError(w, http.StatusBadRequest, "invalid_preferences", "The preference patch is invalid.")
		return
	}
	doc, err := s.patchViewerPreferenceDocument(r.Context(), user, scope, request)
	if errors.Is(err, errPreferenceConflict) {
		writePreferenceConflict(w, doc.Revision)
		return
	}
	if errors.Is(err, errPreferenceVersion) || errors.Is(err, errPreferenceValidation) {
		writeProductError(w, http.StatusBadRequest, "invalid_preferences", err.Error())
		return
	}
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "preferences_failed", "Unable to save preferences.")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) handleViewerProfileActivation(w http.ResponseWriter, r *http.Request, user User) {
	if r.Method != http.MethodPost {
		writeProductError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	appSession, err := s.currentViewerPreferenceAppSession(r.Context(), r, user)
	if err != nil {
		s.writeViewerPreferenceSessionError(w, err, "save")
		return
	}
	var request profileActivationPreferenceRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Version != viewerPreferencesVersion || request.ExpectedRevision < 0 {
		writeProductError(w, http.StatusBadRequest, "invalid_preferences", "The profile activation preference update is invalid.")
		return
	}
	base, err := s.viewerPreferenceScope(r.Context(), user, appSession.DeviceClass, appSession.InstallationID, "account-server-installation")
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "preferences_failed", "Unable to save preferences.")
		return
	}
	doc, err := s.recordAuthoritativeProfileActivation(r.Context(), user, base, request.ExpectedRevision)
	if errors.Is(err, errPreferenceConflict) {
		writePreferenceConflict(w, doc.Revision)
		return
	}
	if err != nil {
		writeDatabaseAccessError(w, err, http.StatusInternalServerError, "preferences_failed", "Unable to save preferences.")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

type viewerPreferenceAppSession struct {
	InstallationID string
	DeviceClass    string
}

var errViewerPreferenceInteractiveSession = errors.New("viewer preferences require an active app session")

func (s *Server) currentViewerPreferenceAppSession(ctx context.Context, r *http.Request, user User) (viewerPreferenceAppSession, error) {
	if user.AuthProvider == "api_key" {
		return viewerPreferenceAppSession{}, errViewerPreferenceInteractiveSession
	}
	tokenHashes := make([]string, 0, 3)
	for _, cookie := range s.requestSessionCookies(r) {
		if value := strings.TrimSpace(cookie.Value); value != "" {
			tokenHashes = append(tokenHashes, hashToken(value))
		}
	}
	if token, ok := bearerTokenFromRequest(r); ok && token != "" && !strings.HasPrefix(token, "ptc_api_") && !strings.HasPrefix(token, "ptc_clt_") {
		tokenHashes = append(tokenHashes, hashToken(token))
	}
	for _, tokenHash := range tokenHashes {
		var installationID, app, platform, userAgent, browserVaultID, revokedAt string
		err := s.queryUserRow(ctx, `
			SELECT COALESCE(d.installation_id, ''), COALESCE(d.app, ''), COALESCE(d.platform, ''),
				COALESCE(d.user_agent, ''), COALESCE(s.browser_vault_id, ''), COALESCE(d.revoked_at, '')
			FROM sessions s
			JOIN devices d ON d.id = s.device_id AND d.user_id = s.user_id
			WHERE s.token_hash = ? AND s.user_id = ? AND COALESCE(NULLIF(s.profile_id, ''), s.user_id) = ? AND s.expires_at > ?`,
			tokenHash, accountIDForUser(user), viewerProfileID(user), time.Now().UTC().Format(time.RFC3339Nano)).
			Scan(&installationID, &app, &platform, &userAgent, &browserVaultID, &revokedAt)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return viewerPreferenceAppSession{}, err
		}
		installationID, ok := normalizeNativeInstallationID(installationID)
		if !ok || revokedAt != "" {
			return viewerPreferenceAppSession{}, errViewerPreferenceInteractiveSession
		}
		deviceClass := viewerPreferenceDeviceClass(app, platform, userAgent, browserVaultID)
		if deviceClass == "" {
			return viewerPreferenceAppSession{}, errViewerPreferenceInteractiveSession
		}
		return viewerPreferenceAppSession{InstallationID: installationID, DeviceClass: deviceClass}, nil
	}
	return viewerPreferenceAppSession{}, errViewerPreferenceInteractiveSession
}

func viewerPreferenceDeviceClass(app, platform, userAgent, browserVaultID string) string {
	if strings.TrimSpace(browserVaultID) != "" {
		return "web"
	}
	value := strings.ToLower(strings.Join([]string{app, platform, userAgent}, " "))
	switch {
	case strings.Contains(value, "appletv"), strings.Contains(value, "tv os"), strings.Contains(value, "tvos"),
		strings.Contains(value, "android tv"), strings.Contains(value, "google tv"), strings.Contains(value, "roku"),
		strings.Contains(value, "tizen"), strings.Contains(value, "webos"), strings.Contains(value, "fire tv"):
		return "television"
	case strings.Contains(value, "iphone"), strings.Contains(value, "ipad"), strings.Contains(value, "ios"),
		strings.Contains(value, "android"), strings.Contains(value, "mobile"):
		return "mobile"
	case strings.Contains(value, "portico-web"), strings.Contains(value, "portico web"), strings.Contains(value, "browser"),
		strings.Contains(value, "mozilla/"), strings.TrimSpace(platform) == "web":
		return "web"
	default:
		return ""
	}
}

func (s *Server) bindViewerPreferenceScopeToSession(r *http.Request, user User, scope preferenceScope) error {
	appSession, err := s.currentViewerPreferenceAppSession(r.Context(), r, user)
	if err != nil {
		return err
	}
	if scope.InstallationID != appSession.InstallationID || scope.DeviceClass != appSession.DeviceClass {
		return errViewerPreferenceInteractiveSession
	}
	return nil
}

func (s *Server) writeViewerPreferenceSessionError(w http.ResponseWriter, err error, action string) {
	if errors.Is(err, errViewerPreferenceInteractiveSession) || errors.Is(err, sql.ErrNoRows) {
		writeProductError(w, http.StatusForbidden, "interactive_session_required", "Preferences are available only to the active app installation.")
		return
	}
	writeDatabaseAccessError(w, err, http.StatusInternalServerError, "preferences_failed", "Unable to "+action+" preferences.")
}

func writePreferenceConflict(w http.ResponseWriter, revision int64) {
	writeProductErrorWithDetails(w, http.StatusConflict, "preference_revision_conflict", "Preferences changed on another device. Refresh and try again.", map[string]any{"currentRevision": revision})
}

func (s *Server) viewerPreferenceScope(ctx context.Context, user User, deviceClass, installationID, scopeType string) (preferenceScope, error) {
	if err := validateViewerPreferenceScopeInput(deviceClass, installationID); err != nil {
		return preferenceScope{}, err
	}
	serverID, err := s.profileDirectoryServerIDContext(ctx)
	if err != nil {
		return preferenceScope{}, err
	}
	authority := "local"
	if user.AuthOrigin == "portico" || user.AuthProvider == "portico" {
		authority = "hosted"
	}
	scope := preferenceScope{
		Type: scopeType, Authority: authority, AccountID: accountIDForUser(user), ProfileID: viewerProfileID(user),
		ServerID: serverID, DeviceClass: deviceClass, InstallationID: installationID,
	}
	if scope.AccountID == "" || scope.ProfileID == "" {
		return preferenceScope{}, errPreferenceScope
	}
	return scope, nil
}

func validateViewerPreferenceScopeInput(deviceClass, installationID string) error {
	if !oneOf(deviceClass, "web", "mobile", "television") {
		return fmt.Errorf("%w: deviceClass must be web, mobile, or television", errPreferenceScope)
	}
	if installationID == "" || len(installationID) > 128 || strings.ContainsAny(installationID, "\r\n\t") {
		return fmt.Errorf("%w: installationId is required and must be at most 128 characters", errPreferenceScope)
	}
	return nil
}

func (s *Server) loadViewerPreferenceBundle(ctx context.Context, user User, base preferenceScope) (viewerPreferenceBundle, error) {
	defaults, err := s.viewerPreferenceDefaults(ctx, user, base.DeviceClass)
	if err != nil {
		return viewerPreferenceBundle{}, err
	}
	var profileServer, device, accountInstall preferenceDocument
	err = s.withUserTxTagged(ctx, []string{"viewer_preference_documents"}, func(tx *sql.Tx) error {
		var loadErr error
		profileServer, loadErr = loadOrCreatePreferenceDocumentTx(tx, scopeFor(base, "profile-server"), defaults["profile-server"])
		if loadErr != nil {
			return loadErr
		}
		device, loadErr = loadOrCreatePreferenceDocumentTx(tx, scopeFor(base, "profile-device-class"), defaults["profile-device-class"])
		if loadErr != nil {
			return loadErr
		}
		accountInstall, loadErr = loadOrCreatePreferenceDocumentTx(tx, scopeFor(base, "account-server-installation"), defaults["account-server-installation"])
		return loadErr
	})
	if err != nil {
		return viewerPreferenceBundle{}, err
	}
	effectiveBitrateLimit := s.effectiveRemoteBitrateLimitMbpsContext(ctx, user)
	effective, clamped, err := effectiveDevicePreferences(device, effectiveBitrateLimit)
	if err != nil {
		return viewerPreferenceBundle{}, err
	}
	feedbackPolicy := s.viewerFeedbackPolicyContext(ctx)
	identity, err := s.publicViewerPreferenceIdentity(ctx, user, base)
	if err != nil {
		return viewerPreferenceBundle{}, err
	}
	return viewerPreferenceBundle{
		Identity: identity, ProfileServer: profileServer, ProfileDeviceClass: device,
		AccountServerInstallation: accountInstall, EffectiveProfileDeviceClass: effective,
		Policy: viewerPreferencePolicy{
			FeedbackAllowed: feedbackPolicy.Enabled && user.AllowFeedback && user.AuthProvider != "api_key", DownloadsAllowed: base.DeviceClass != "television" && user.Permissions["downloadMedia"],
			CellularQualityAllowed: base.DeviceClass == "mobile", MaximumVideoBitrateMbps: effectiveBitrateLimit,
		}, ClampedFields: clamped,
	}, nil
}

// publicViewerPreferenceIdentity keeps the private server account/profile keys
// used to store preference documents out of the client contract. Hosted
// clients bind preference responses to the same stable Hosted account, profile,
// and server identities carried by native credentials and /api/auth/me. Local
// Auth already uses the server-local identities as its public viewer scope.
func (s *Server) publicViewerPreferenceIdentity(ctx context.Context, user User, base preferenceScope) (preferenceScopeIdentity, error) {
	identity := base.identity()
	if base.Authority != "hosted" {
		return identity, nil
	}

	var err error
	identity.AccountID, identity.ProfileID, err = s.publicViewerIdentityForUserContext(ctx, user, "portico")
	if err != nil {
		return preferenceScopeIdentity{}, err
	}
	settings, err := s.loadSettingsContext(ctx)
	if err != nil {
		return preferenceScopeIdentity{}, err
	}
	identity.ServerID, err = s.publicServerIDForAuthProviderContext(ctx, settings, "portico")
	if err != nil {
		return preferenceScopeIdentity{}, err
	}
	return identity, nil
}

func scopeFor(base preferenceScope, scopeType string) preferenceScope {
	result := base
	result.Type = scopeType
	switch scopeType {
	case "profile-server":
		result.DeviceClass, result.InstallationID = "", ""
	case "profile-device-class":
		result.InstallationID = ""
	case "account-server-installation":
		result.ProfileID, result.DeviceClass = "", ""
	}
	return result
}

func (s *Server) viewerPreferenceDefaults(ctx context.Context, user User, deviceClass string) (map[string]json.RawMessage, error) {
	profile := defaultProfileServerValues(user)
	device := defaultProfileDeviceValues(deviceClass)
	if navigation, err := s.libraryNavigationPreferencesContext(ctx, user); err == nil {
		device.Navigation.PinnedLibraryIDs = append([]string(nil), navigation.PinnedLibraryIDs...)
	}
	profileSelection := "last-used"
	if deviceClass == "television" {
		profileSelection = "ask"
	}
	account := accountInstallationPreferenceValues{RememberAccount: true, ProfileSelection: profileSelection, LastProfileID: viewerProfileID(user)}
	result := map[string]json.RawMessage{}
	for key, value := range map[string]any{
		"profile-server": profile, "profile-device-class": device, "account-server-installation": account,
	} {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		result[key] = raw
	}
	return result, nil
}

func defaultProfileServerValues(_ User) profileServerPreferenceValues {
	var result profileServerPreferenceValues
	result.Localization.Locale = "en-US"
	result.Localization.TimeZone = "UTC"
	result.Localization.DateFormat = "medium"
	result.Localization.HourCycle = "auto"
	result.Home.RowOrder, result.Home.HiddenRowIDs = []string{}, []string{}
	result.Playback.AutoplayNext = true
	result.Playback.UpNextCountdownSeconds = 10
	result.Playback.PassoutProtection = true
	result.Playback.PassoutAfterEpisodes = 3
	result.Playback.IntroSkip, result.Playback.CreditsSkip = "ask", "ask"
	result.Playback.StartedThresholdPercent = 5
	result.Playback.PlayedThresholdPercent = 95
	result.Playback.SkipBackSeconds, result.Playback.SkipForwardSeconds = 10, 30
	result.Playback.DefaultSpeed = 1
	result.Playback.PreferredAudioLanguages = []string{"original"}
	result.Playback.PreferredSubtitleLanguages = []string{}
	result.Playback.SubtitleSize, result.Playback.SubtitleBackground = "medium", "subtle"
	result.Playback.ShowSyncedLyrics = true
	result.Music.RepeatDefault = "none"
	result.Music.AutoplayDefault = true
	result.Music.AudioNormalization = "off"
	result.Music.Gapless = true
	result.Privacy.ShowActivityToMembers = true
	result.Privacy.IncludeInWatchWithFriends = true
	result.Search.RememberHistory, result.Search.RecentQueries = true, []string{}
	result.Downloads.Quality.Mode = "ask"
	return result
}

func defaultProfileDeviceValues(deviceClass string) profileDeviceClassPreferenceValues {
	var result profileDeviceClassPreferenceValues
	result.DeviceClass = deviceClass
	result.Appearance.Density, result.Appearance.CardSizePercent, result.Appearance.ShowBackdrops = "comfortable", 100, true
	result.Navigation.PinnedLibraryIDs = []string{}
	result.Navigation.DefaultLanding = "home"
	result.Playback.DeliveryRequest.DirectPlay = "prefer"
	result.Playback.DeliveryRequest.DirectStream = "allow"
	result.Playback.DeliveryRequest.Transcode = "allow"
	allowHDR := true
	result.Playback.Quality = map[string]qualityPreferenceValues{
		"local":    {Mode: "original", AllowHDR: &allowHDR},
		"wifi":     {Mode: "original", AllowHDR: &allowHDR},
		"cellular": {Mode: "original", AllowHDR: &allowHDR},
		"unknown":  {Mode: "original", AllowHDR: &allowHDR},
	}
	return result
}

func loadOrCreatePreferenceDocumentTx(tx *sql.Tx, scope preferenceScope, defaults json.RawMessage) (preferenceDocument, error) {
	var doc preferenceDocument
	var documentID string
	var valuesJSON string
	doc.Version = viewerPreferencesVersion
	err := tx.QueryRow(`
		SELECT id, version, revision, values_json FROM viewer_preference_documents
		WHERE scope_type = ? AND authority = ? AND account_id = ? AND profile_id = ?
		  AND server_id = ? AND device_class = ? AND installation_id = ?`,
		scope.Type, scope.Authority, scope.AccountID, scope.ProfileID, scope.ServerID, scope.DeviceClass, scope.InstallationID).
		Scan(&documentID, &doc.Version, &doc.Revision, &valuesJSON)
	if err == nil {
		return repairPreferenceDocumentTx(tx, documentID, scope, doc, valuesJSON, defaults)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return preferenceDocument{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.Exec(`
		INSERT INTO viewer_preference_documents (
			id, scope_type, authority, account_id, profile_id, server_id, device_class, installation_id,
			version, revision, values_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'v1', 0, ?, ?, ?)
		ON CONFLICT(scope_type, authority, account_id, profile_id, server_id, device_class, installation_id) DO NOTHING`,
		randomID("pref"), scope.Type, scope.Authority, scope.AccountID, scope.ProfileID, scope.ServerID,
		scope.DeviceClass, scope.InstallationID, string(defaults), now, now)
	if err != nil {
		return preferenceDocument{}, err
	}
	err = tx.QueryRow(`
		SELECT id, version, revision, values_json FROM viewer_preference_documents
		WHERE scope_type = ? AND authority = ? AND account_id = ? AND profile_id = ?
		  AND server_id = ? AND device_class = ? AND installation_id = ?`,
		scope.Type, scope.Authority, scope.AccountID, scope.ProfileID, scope.ServerID, scope.DeviceClass, scope.InstallationID).
		Scan(&documentID, &doc.Version, &doc.Revision, &valuesJSON)
	if err == nil {
		return repairPreferenceDocumentTx(tx, documentID, scope, doc, valuesJSON, defaults)
	}
	return doc, err
}

func repairPreferenceDocumentTx(tx *sql.Tx, documentID string, scope preferenceScope, doc preferenceDocument, valuesJSON string, defaults json.RawMessage) (preferenceDocument, error) {
	raw := json.RawMessage(valuesJSON)
	if doc.Version == viewerPreferencesVersion {
		if normalized, err := validatePreferenceValues(scope, raw); err == nil {
			doc.Values = normalized
			return doc, nil
		}
	}

	repaired, err := applyPreferenceDefaults(defaults, raw)
	if err == nil {
		repaired, err = validatePreferenceValues(scope, repaired)
	}
	if err != nil || doc.Version != viewerPreferencesVersion {
		repaired, err = validatePreferenceValues(scope, defaults)
	}
	if err != nil {
		return preferenceDocument{}, fmt.Errorf("preference defaults are invalid for %s: %w", scope.Type, err)
	}

	reason := "stored preference document failed current validation"
	if doc.Version != viewerPreferencesVersion {
		reason = "unsupported stored preference document version"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`
		INSERT INTO viewer_preference_document_quarantine (
			id, document_id, scope_type, authority, account_id, profile_id, server_id,
			device_class, installation_id, version, revision, values_json, reason, quarantined_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		randomID("prefq"), documentID, scope.Type, scope.Authority, scope.AccountID, scope.ProfileID,
		scope.ServerID, scope.DeviceClass, scope.InstallationID, doc.Version, doc.Revision, valuesJSON, reason, now); err != nil {
		return preferenceDocument{}, err
	}
	result, err := tx.Exec(`
		UPDATE viewer_preference_documents
		SET version = 'v1', values_json = ?, revision = revision + 1, updated_at = ?
		WHERE id = ? AND revision = ?`, string(repaired), now, documentID, doc.Revision)
	if err != nil {
		return preferenceDocument{}, err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return preferenceDocument{}, err
	} else if rows != 1 {
		return preferenceDocument{}, errPreferenceConflict
	}
	return preferenceDocument{Version: viewerPreferencesVersion, Revision: doc.Revision + 1, Values: repaired}, nil
}

func (s *Server) patchViewerPreferenceDocument(ctx context.Context, user User, scope preferenceScope, request preferencePatchRequest) (preferenceDocument, error) {
	if scope.Type == "account-server-installation" {
		var changes map[string]json.RawMessage
		if err := json.Unmarshal(request.Changes, &changes); err != nil {
			return preferenceDocument{}, fmt.Errorf("%w: preference changes must be an object", errPreferenceValidation)
		}
		if _, attemptsProfileActivation := changes["lastProfileId"]; attemptsProfileActivation {
			return preferenceDocument{}, fmt.Errorf("%w: lastProfileId is updated only after authoritative profile activation", errPreferenceValidation)
		}
	}
	defaults, err := s.viewerPreferenceDefaults(ctx, user, scope.DeviceClass)
	if err != nil {
		return preferenceDocument{}, err
	}
	scope = scopeFor(scope, scope.Type)
	availableLibraries := map[string]bool{}
	if scope.Type == "profile-device-class" {
		libraries, listErr := s.listLibrariesForUserContext(ctx, user)
		if listErr != nil {
			return preferenceDocument{}, listErr
		}
		for _, library := range libraries {
			availableLibraries[library.ID] = true
		}
	}
	var result preferenceDocument
	err = s.withUserTxTagged(ctx, []string{"viewer_preference_documents"}, func(tx *sql.Tx) error {
		current, err := loadOrCreatePreferenceDocumentTx(tx, scope, defaults[scope.Type])
		if err != nil {
			return err
		}
		result = current
		if current.Version != viewerPreferencesVersion {
			return errPreferenceVersion
		}
		if current.Revision != request.ExpectedRevision {
			return errPreferenceConflict
		}
		merged, err := mergePreferenceJSON(current.Values, request.Changes)
		if err != nil {
			return fmt.Errorf("%w: %v", errPreferenceValidation, err)
		}
		merged, err = applyPreferenceDefaults(defaults[scope.Type], merged)
		if err != nil {
			return fmt.Errorf("%w: %v", errPreferenceValidation, err)
		}
		normalized, err := validatePreferenceValues(scope, merged)
		if err != nil {
			return fmt.Errorf("%w: %v", errPreferenceValidation, err)
		}
		if err := validatePreferenceReferencesTx(tx, scope, normalized, availableLibraries); err != nil {
			return fmt.Errorf("%w: %v", errPreferenceValidation, err)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		writeResult, err := tx.Exec(`
			UPDATE viewer_preference_documents SET values_json = ?, revision = revision + 1, updated_at = ?
			WHERE scope_type = ? AND authority = ? AND account_id = ? AND profile_id = ?
			  AND server_id = ? AND device_class = ? AND installation_id = ? AND revision = ?`,
			string(normalized), now, scope.Type, scope.Authority, scope.AccountID, scope.ProfileID,
			scope.ServerID, scope.DeviceClass, scope.InstallationID, current.Revision)
		if err != nil {
			return err
		}
		rows, err := writeResult.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return errPreferenceConflict
		}
		result = preferenceDocument{Version: viewerPreferencesVersion, Revision: current.Revision + 1, Values: normalized}
		return nil
	})
	return result, err
}

func (s *Server) recordAuthoritativeProfileActivation(ctx context.Context, user User, scope preferenceScope, expectedRevision int64) (preferenceDocument, error) {
	defaults, err := s.viewerPreferenceDefaults(ctx, user, scope.DeviceClass)
	if err != nil {
		return preferenceDocument{}, err
	}
	scope = scopeFor(scope, "account-server-installation")
	var result preferenceDocument
	err = s.withUserTxTagged(ctx, []string{"viewer_preference_documents"}, func(tx *sql.Tx) error {
		current, err := loadOrCreatePreferenceDocumentTx(tx, scope, defaults[scope.Type])
		if err != nil {
			return err
		}
		result = current
		if current.Version != viewerPreferencesVersion {
			return errPreferenceVersion
		}
		if current.Revision != expectedRevision {
			return errPreferenceConflict
		}
		var value accountInstallationPreferenceValues
		if err := decodeStrictPreference(current.Values, &value); err != nil {
			return err
		}
		value.LastProfileID = viewerProfileID(user)
		normalized, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err := validatePreferenceReferencesTx(tx, scope, normalized, nil); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		writeResult, err := tx.Exec(`
			UPDATE viewer_preference_documents SET values_json = ?, revision = revision + 1, updated_at = ?
			WHERE scope_type = ? AND authority = ? AND account_id = ? AND profile_id = ?
			  AND server_id = ? AND device_class = ? AND installation_id = ? AND revision = ?`,
			string(normalized), now, scope.Type, scope.Authority, scope.AccountID, scope.ProfileID,
			scope.ServerID, scope.DeviceClass, scope.InstallationID, current.Revision)
		if err != nil {
			return err
		}
		if rows, err := writeResult.RowsAffected(); err != nil {
			return err
		} else if rows != 1 {
			return errPreferenceConflict
		}
		result = preferenceDocument{Version: viewerPreferencesVersion, Revision: current.Revision + 1, Values: normalized}
		return nil
	})
	return result, err
}

func validatePreferenceReferencesTx(tx *sql.Tx, scope preferenceScope, raw json.RawMessage, availableLibraries map[string]bool) error {
	switch scope.Type {
	case "profile-device-class":
		var value profileDeviceClassPreferenceValues
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		for _, libraryID := range value.Navigation.PinnedLibraryIDs {
			if !availableLibraries[libraryID] {
				return errors.New("pinned library is not available to this profile")
			}
		}
	case "account-server-installation":
		var value accountInstallationPreferenceValues
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		if value.LastProfileID != "" {
			var exists int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM profiles WHERE id = ? AND account_id = ? AND disabled_at = ''`, value.LastProfileID, scope.AccountID).Scan(&exists); err != nil {
				return err
			}
			if exists != 1 {
				return errors.New("last profile does not belong to this account")
			}
		}
	}
	return nil
}

func mergePreferenceJSON(current, changes json.RawMessage) (json.RawMessage, error) {
	var base, patch map[string]any
	if err := json.Unmarshal(current, &base); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(changes, &patch); err != nil {
		return nil, err
	}
	if base == nil || patch == nil {
		return nil, errors.New("preference values and changes must be objects")
	}
	mergePreferenceMap(base, patch)
	return json.Marshal(base)
}

func mergePreferenceMap(base, changes map[string]any) {
	for key, raw := range changes {
		if change, ok := raw.(map[string]any); ok {
			if clear, exists := change["$clear"]; exists && clear == true && len(change) == 1 {
				delete(base, key)
				continue
			}
			if existing, ok := base[key].(map[string]any); ok {
				mergePreferenceMap(existing, change)
				continue
			}
		}
		base[key] = raw
	}
}

func applyPreferenceDefaults(defaults, current json.RawMessage) (json.RawMessage, error) {
	var base, requested map[string]any
	if err := json.Unmarshal(defaults, &base); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(current, &requested); err != nil {
		return nil, err
	}
	if base == nil || requested == nil {
		return nil, errors.New("preference defaults and values must be objects")
	}
	mergePreferenceMap(base, requested)
	return json.Marshal(base)
}

func validatePreferenceValues(scope preferenceScope, raw json.RawMessage) (json.RawMessage, error) {
	switch scope.Type {
	case "profile-server":
		var value profileServerPreferenceValues
		if err := decodeStrictPreference(raw, &value); err != nil {
			return nil, err
		}
		if err := validateProfileServerPreferences(&value); err != nil {
			return nil, err
		}
		if !value.Search.RememberHistory {
			value.Search.RecentQueries = []string{}
		}
		return json.Marshal(value)
	case "profile-device-class":
		var value profileDeviceClassPreferenceValues
		if err := decodeStrictPreference(raw, &value); err != nil {
			return nil, err
		}
		if value.DeviceClass != scope.DeviceClass {
			return nil, errors.New("deviceClass does not match the document scope")
		}
		if err := validateProfileDevicePreferences(value); err != nil {
			return nil, err
		}
		return json.Marshal(value)
	case "account-server-installation":
		var value accountInstallationPreferenceValues
		if err := decodeStrictPreference(raw, &value); err != nil {
			return nil, err
		}
		if !oneOf(value.ProfileSelection, "ask", "last-used") || len(value.LastProfileID) > 128 {
			return nil, errors.New("profile selection preference is invalid")
		}
		return json.Marshal(value)
	default:
		return nil, errPreferenceScope
	}
}

func decodeStrictPreference(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("preference document contains trailing content")
	}
	return nil
}

func validateProfileServerPreferences(value *profileServerPreferenceValues) error {
	if len(value.Localization.Locale) == 0 || len(value.Localization.Locale) > 35 || len(value.Localization.TimeZone) == 0 || len(value.Localization.TimeZone) > 128 {
		return errors.New("localization is invalid")
	}
	if !oneOf(value.Localization.DateFormat, "short", "medium", "long") || !oneOf(value.Localization.HourCycle, "auto", "h12", "h23") {
		return errors.New("localization format is invalid")
	}
	if !intOneOf(value.Playback.UpNextCountdownSeconds, 0, 5, 10, 15) || !intOneOf(value.Playback.PassoutAfterEpisodes, 2, 3, 4, 5) ||
		!oneOf(value.Playback.IntroSkip, "ask", "automatic", "off") || !oneOf(value.Playback.CreditsSkip, "ask", "automatic", "off") ||
		value.Playback.StartedThresholdPercent < preferenceStartedThresholdMinimum || value.Playback.StartedThresholdPercent > preferenceStartedThresholdMaximum ||
		value.Playback.PlayedThresholdPercent < preferencePlayedThresholdMinimum || value.Playback.PlayedThresholdPercent > 100 ||
		value.Playback.StartedThresholdPercent >= value.Playback.PlayedThresholdPercent ||
		!intOneOf(value.Playback.SkipBackSeconds, 5, 10, 15, 30) || !intOneOf(value.Playback.SkipForwardSeconds, 10, 15, 30, 45) ||
		!floatOneOf(value.Playback.DefaultSpeed, .5, .75, 1, 1.25, 1.5, 1.75, 2) {
		return errors.New("playback preference is invalid")
	}
	if !oneOf(value.Playback.SubtitleSize, "small", "medium", "large") || !oneOf(value.Playback.SubtitleBackground, "none", "subtle", "solid") {
		return errors.New("subtitle preference is invalid")
	}
	if len(value.Playback.PreferredAudioLanguages) > 12 || len(value.Playback.PreferredSubtitleLanguages) > 12 || len(value.Home.RowOrder) > 100 || len(value.Home.HiddenRowIDs) > 100 {
		return errors.New("preference list is too large")
	}
	for name, list := range map[string][]string{
		"home row order": value.Home.RowOrder, "hidden home rows": value.Home.HiddenRowIDs,
		"preferred audio languages":    value.Playback.PreferredAudioLanguages,
		"preferred subtitle languages": value.Playback.PreferredSubtitleLanguages,
	} {
		if err := validatePreferenceStringList(list, 128); err != nil {
			return fmt.Errorf("%s is invalid: %w", name, err)
		}
	}
	if !oneOf(value.Music.RepeatDefault, "none", "one", "all") || !oneOf(value.Music.AudioNormalization, "off", "attenuate") || value.Music.CrossfadeSeconds < 0 || value.Music.CrossfadeSeconds > 12 {
		return errors.New("music preference is invalid")
	}
	if len(value.Search.RecentQueries) > preferenceSearchHistoryMaximum {
		return errors.New("search history is too large")
	}
	for _, query := range value.Search.RecentQueries {
		if len([]rune(query)) > preferenceSearchQueryMaximumRunes {
			return errors.New("search history entry is too long")
		}
	}
	if err := validatePreferenceStringList(value.Search.RecentQueries, preferenceSearchQueryMaximumRunes); err != nil {
		return fmt.Errorf("search history is invalid: %w", err)
	}
	if !oneOf(value.Downloads.Quality.Mode, "ask", "original", "high", "standard", "data-saver", "optimized") {
		return errors.New("download quality is invalid")
	}
	if value.Downloads.Quality.MaxVideoBitrateMbps != nil && (*value.Downloads.Quality.MaxVideoBitrateMbps < 1 || *value.Downloads.Quality.MaxVideoBitrateMbps > 1000) {
		return errors.New("download video bitrate is invalid")
	}
	if value.Downloads.Quality.MaxAudioBitrateKbps != nil && (*value.Downloads.Quality.MaxAudioBitrateKbps < preferenceAudioBitrateMinimum || *value.Downloads.Quality.MaxAudioBitrateKbps > preferenceAudioBitrateMaximum) {
		return errors.New("download audio bitrate is invalid")
	}
	if value.Downloads.Quality.MaxVideoHeight != nil && !intOneOf(*value.Downloads.Quality.MaxVideoHeight, preferenceVideoHeightChoices...) {
		return errors.New("download video height is invalid")
	}
	return nil
}

func validateProfileDevicePreferences(value profileDeviceClassPreferenceValues) error {
	if !oneOf(value.DeviceClass, "web", "mobile", "television") || !oneOf(value.Appearance.Density, "comfortable", "compact") || value.Appearance.CardSizePercent < preferenceCardSizeMinimum || value.Appearance.CardSizePercent > preferenceCardSizeMaximum {
		return errors.New("appearance preference is invalid")
	}
	if len(value.Navigation.PinnedLibraryIDs) > 100 || !oneOf(value.Navigation.DefaultLanding, "home", "library", "channels", "saved", "downloads") {
		return errors.New("navigation preference is invalid")
	}
	if err := validatePreferenceStringList(value.Navigation.PinnedLibraryIDs, 128); err != nil {
		return fmt.Errorf("pinned libraries are invalid: %w", err)
	}
	if !oneOf(value.Playback.DeliveryRequest.DirectPlay, "allow", "prefer", "never") ||
		!oneOf(value.Playback.DeliveryRequest.DirectStream, "allow", "prefer", "never") ||
		!oneOf(value.Playback.DeliveryRequest.Transcode, "allow", "prefer", "require") {
		return errors.New("delivery preference is invalid")
	}
	if len(value.Playback.Quality) != 4 {
		return errors.New("all network quality preferences are required")
	}
	for _, network := range []string{"local", "wifi", "cellular", "unknown"} {
		quality, ok := value.Playback.Quality[network]
		if !ok || !oneOf(quality.Mode, "off", "automatic", "original", "high", "standard", "data-saver") {
			return errors.New("network quality preference is invalid")
		}
		if quality.MaxVideoBitrateMbps != nil && (*quality.MaxVideoBitrateMbps < 1 || *quality.MaxVideoBitrateMbps > 1000) {
			return errors.New("video bitrate is invalid")
		}
		if quality.MaxAudioBitrateKbps != nil && (*quality.MaxAudioBitrateKbps < preferenceAudioBitrateMinimum || *quality.MaxAudioBitrateKbps > preferenceAudioBitrateMaximum) {
			return errors.New("audio bitrate is invalid")
		}
		if quality.MaxVideoHeight != nil && !intOneOf(*quality.MaxVideoHeight, preferenceVideoHeightChoices...) {
			return errors.New("video height is invalid")
		}
	}
	return nil
}

func validatePreferenceStringList(values []string, maximumRunes int) error {
	seen := map[string]bool{}
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" || len([]rune(normalized)) > maximumRunes || seen[normalized] {
			return errors.New("values must be non-empty and unique")
		}
		seen[normalized] = true
	}
	return nil
}

func effectiveDevicePreferences(requested preferenceDocument, maximumVideoBitrateMbps int) (preferenceDocument, []string, error) {
	var value profileDeviceClassPreferenceValues
	if err := decodeStrictPreference(requested.Values, &value); err != nil {
		return preferenceDocument{}, nil, err
	}
	clamped := []string{}
	if maximumVideoBitrateMbps > 0 {
		for network, quality := range value.Playback.Quality {
			if quality.MaxVideoBitrateMbps == nil || *quality.MaxVideoBitrateMbps > maximumVideoBitrateMbps {
				limit := maximumVideoBitrateMbps
				quality.MaxVideoBitrateMbps = &limit
				value.Playback.Quality[network] = quality
				clamped = append(clamped, "playback.quality."+network+".maxVideoBitrateMbps")
			}
		}
	}
	if value.DeviceClass != "mobile" {
		quality := value.Playback.Quality["cellular"]
		quality.Mode = "off"
		value.Playback.Quality["cellular"] = quality
		clamped = append(clamped, "playback.quality.cellular.mode")
	}
	raw, err := json.Marshal(value)
	return preferenceDocument{Version: requested.Version, Revision: requested.Revision, Values: raw}, clamped, err
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func intOneOf(value int, choices ...int) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func floatOneOf(value float64, choices ...float64) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func firstNonBlank(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func defaultInt(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}

func parsePreferenceLimit(raw string, fallback, maximum int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < 1 {
		return fallback
	}
	if parsed > maximum {
		return maximum
	}
	return parsed
}
