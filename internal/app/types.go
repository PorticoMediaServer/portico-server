package app

import (
	"bytes"
	"encoding/json"
	"errors"
)

type User struct {
	AuthSessionID          string             `json:"-"`
	ID                     string             `json:"id"`
	AccountID              string             `json:"-"`
	ProfileID              string             `json:"profileId,omitempty"`
	ProfileIsPrimary       bool               `json:"-"`
	ProfileIdentityID      string             `json:"profileIdentityId,omitempty"`
	AuthProvider           string             `json:"authProvider,omitempty"`
	Username               string             `json:"username"`
	Email                  string             `json:"email"`
	DisplayName            string             `json:"displayName"`
	ProfileImageURL        string             `json:"profileImageUrl,omitempty"`
	Role                   string             `json:"role"`
	AuthOrigin             string             `json:"authOrigin"`
	PorticoUserID          string             `json:"porticoUserId,omitempty"`
	PorticoMembershipID    string             `json:"porticoMembershipId,omitempty"`
	HasLocalPassword       bool               `json:"hasLocalPassword"`
	Permissions            map[string]bool    `json:"permissions"`
	APIKeyID               string             `json:"-"`
	APIKeyScopes           []string           `json:"-"`
	DeviceID               string             `json:"-"`
	LibraryIDs             []string           `json:"libraryIds"`
	Preferences            UserPreferences    `json:"preferences"`
	AccessSchedule         UserAccessSchedule `json:"accessSchedule"`
	TagPolicy              UserTagPolicy      `json:"tagPolicy"`
	DevicePolicy           UserDevicePolicy   `json:"devicePolicy"`
	ChannelPolicy          UserChannelPolicy  `json:"channelPolicy"`
	MaxContentRating       string             `json:"maxContentRating,omitempty"`
	MaximumAgeRating       *int               `json:"maximumAgeRating,omitempty"`
	AllowUnrated           bool               `json:"allowUnrated"`
	BlockedProfileLabels   []string           `json:"blockedProfileLabels,omitempty"`
	AllowFeedback          bool               `json:"allowFeedback"`
	MaxActiveSessions      int                `json:"maxActiveSessions,omitempty"`
	MaxActiveStreams       int                `json:"maxActiveStreams,omitempty"`
	RemoteBitrateLimitMbps int                `json:"remoteBitrateLimitMbps,omitempty"`
	AccountProfilesAllowed bool               `json:"-"`
	SignInMethods          []SignInMethod     `json:"signInMethods,omitempty"`
}

type SignInMethod struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
}

type UserPreferences struct {
	Locale           string                      `json:"locale"`
	TimeZone         string                      `json:"timeZone"`
	DateFormat       string                      `json:"dateFormat"`
	HourCycle        string                      `json:"hourCycle"`
	AudioLanguage    string                      `json:"audioLanguage"`
	SubtitleLanguage string                      `json:"subtitleLanguage"`
	SidebarOrder     []string                    `json:"sidebarOrder"`
	MusicPlayback    MusicPlaybackPreferences    `json:"musicPlayback"`
	PlaybackProgress PlaybackProgressPreferences `json:"playbackProgress"`
	Privacy          UserPrivacyPreferences      `json:"privacy"`
}

type MusicPlaybackPreferences struct {
	ShuffleDefault    bool   `json:"shuffleDefault"`
	RepeatDefault     string `json:"repeatDefault"`
	AutoplayDefault   bool   `json:"autoplayDefault"`
	NormalizationMode string `json:"normalizationMode"`
	CrossfadeSeconds  int    `json:"crossfadeSeconds"`
	Gapless           bool   `json:"gapless"`
}

type PlaybackProgressPreferences struct {
	StartedThresholdPercent int `json:"startedThresholdPercent"`
	PlayedThresholdPercent  int `json:"playedThresholdPercent"`
}

type UserPrivacyPreferences struct {
	PauseWatchHistory         bool `json:"pauseWatchHistory"`
	ShowActivityToMembers     bool `json:"showActivityToMembers"`
	IncludeInWatchWithFriends bool `json:"includeInWatchWithFriends"`
}

type UserAccessSchedule struct {
	Enabled     bool  `json:"enabled"`
	Days        []int `json:"days,omitempty"`
	StartMinute int   `json:"startMinute,omitempty"`
	EndMinute   int   `json:"endMinute,omitempty"`
}

type UserTagPolicy struct {
	AllowedTags   []string `json:"allowedTags,omitempty"`
	BlockedTags   []string `json:"blockedTags,omitempty"`
	AllowedLabels []string `json:"allowedLabels,omitempty"`
	BlockedLabels []string `json:"blockedLabels,omitempty"`
}

type UserDevicePolicy struct {
	Mode             string   `json:"mode"`
	AllowedDeviceIDs []string `json:"allowedDeviceIds,omitempty"`
}

type UserChannelPolicy struct {
	AllowedChannelIDs []string `json:"allowedChannelIds,omitempty"`
	BlockedChannelIDs []string `json:"blockedChannelIds,omitempty"`
}

type AccountPreferencesRequest struct {
	Locale           string                      `json:"locale"`
	TimeZone         string                      `json:"timeZone"`
	DateFormat       string                      `json:"dateFormat"`
	HourCycle        string                      `json:"hourCycle"`
	AudioLanguage    string                      `json:"audioLanguage"`
	SubtitleLanguage string                      `json:"subtitleLanguage"`
	SidebarOrder     []string                    `json:"sidebarOrder"`
	MusicPlayback    MusicPlaybackPreferences    `json:"musicPlayback"`
	PlaybackProgress PlaybackProgressPreferences `json:"playbackProgress"`
	Privacy          *UserPrivacyPreferences     `json:"privacy,omitempty"`
}

type AccountProfileRequest struct {
	DisplayName     string `json:"displayName"`
	Email           string `json:"email"`
	CurrentPassword string `json:"currentPassword"`
}

type LibraryNavigationPreferences struct {
	PinnedLibraryIDs []string `json:"pinnedLibraryIds"`
}

type LibraryNavigationPreferencesRequest struct {
	PinnedLibraryIDs []string `json:"pinnedLibraryIds"`
}

type WatchHistoryClearResponse struct {
	OK        bool   `json:"ok"`
	ClearedAt string `json:"clearedAt"`
}

type DisplayPreference struct {
	Client      string         `json:"client"`
	View        string         `json:"view"`
	Preferences map[string]any `json:"preferences"`
	UpdatedAt   string         `json:"updatedAt"`
}

type DisplayPreferenceRequest struct {
	Preferences map[string]any `json:"preferences"`
}

type UserRequest struct {
	Username               string             `json:"username"`
	Email                  string             `json:"email"`
	DisplayName            string             `json:"displayName"`
	Password               string             `json:"password,omitempty"`
	Role                   string             `json:"-"`
	Permissions            map[string]bool    `json:"permissions"`
	LibraryIDs             []string           `json:"libraryIds"`
	AccessSchedule         UserAccessSchedule `json:"accessSchedule"`
	TagPolicy              UserTagPolicy      `json:"tagPolicy"`
	DevicePolicy           UserDevicePolicy   `json:"devicePolicy"`
	ChannelPolicy          UserChannelPolicy  `json:"channelPolicy"`
	MaxContentRating       string             `json:"maxContentRating,omitempty"`
	MaxActiveSessions      int                `json:"maxActiveSessions,omitempty"`
	MaxActiveStreams       int                `json:"maxActiveStreams,omitempty"`
	RemoteBitrateLimitMbps int                `json:"remoteBitrateLimitMbps,omitempty"`
}

// UserCreateRequest is the public owner-only contract for creating a server
// user. Server ownership is never expressible through this DTO.
type UserCreateRequest struct {
	Username               string             `json:"username"`
	Email                  string             `json:"email"`
	DisplayName            string             `json:"displayName"`
	Password               string             `json:"password"`
	Permissions            map[string]bool    `json:"permissions"`
	LibraryIDs             []string           `json:"libraryIds"`
	AccessSchedule         UserAccessSchedule `json:"accessSchedule"`
	TagPolicy              UserTagPolicy      `json:"tagPolicy"`
	DevicePolicy           UserDevicePolicy   `json:"devicePolicy"`
	ChannelPolicy          UserChannelPolicy  `json:"channelPolicy"`
	MaxContentRating       string             `json:"maxContentRating,omitempty"`
	MaxActiveSessions      int                `json:"maxActiveSessions,omitempty"`
	MaxActiveStreams       int                `json:"maxActiveStreams,omitempty"`
	RemoteBitrateLimitMbps int                `json:"remoteBitrateLimitMbps,omitempty"`
}

func (request UserCreateRequest) internalRequest() UserRequest {
	return UserRequest{
		Username: request.Username, Email: request.Email, DisplayName: request.DisplayName,
		Password: request.Password, Role: "user", Permissions: request.Permissions, LibraryIDs: request.LibraryIDs,
		AccessSchedule: request.AccessSchedule, TagPolicy: request.TagPolicy, DevicePolicy: request.DevicePolicy,
		ChannelPolicy: request.ChannelPolicy, MaxContentRating: request.MaxContentRating,
		MaxActiveSessions: request.MaxActiveSessions, MaxActiveStreams: request.MaxActiveStreams,
		RemoteBitrateLimitMbps: request.RemoteBitrateLimitMbps,
	}
}

// UserPatchRequest preserves every omitted field. Explicit zero values and
// empty collections are mutations; JSON null is rejected by the decoder.
type UserPatchRequest struct {
	Username               *string             `json:"username,omitempty"`
	Email                  *string             `json:"email,omitempty"`
	DisplayName            *string             `json:"displayName,omitempty"`
	Password               *string             `json:"password,omitempty"`
	Permissions            *map[string]bool    `json:"permissions,omitempty"`
	LibraryIDs             *[]string           `json:"libraryIds,omitempty"`
	AccessSchedule         *UserAccessSchedule `json:"accessSchedule,omitempty"`
	TagPolicy              *UserTagPolicy      `json:"tagPolicy,omitempty"`
	DevicePolicy           *UserDevicePolicy   `json:"devicePolicy,omitempty"`
	ChannelPolicy          *UserChannelPolicy  `json:"channelPolicy,omitempty"`
	MaxContentRating       *string             `json:"maxContentRating,omitempty"`
	MaxActiveSessions      *int                `json:"maxActiveSessions,omitempty"`
	MaxActiveStreams       *int                `json:"maxActiveStreams,omitempty"`
	RemoteBitrateLimitMbps *int                `json:"remoteBitrateLimitMbps,omitempty"`
}

type requestFieldValidationError struct {
	FieldPath string
	Detail    string
}

func (err *requestFieldValidationError) Error() string { return err.Detail }

func invalidRequestField(fieldPath, detail string) error {
	return &requestFieldValidationError{FieldPath: fieldPath, Detail: detail}
}

func (request *UserPatchRequest) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) == 0 {
		return errors.New("at least one user field is required")
	}
	allowed := map[string]bool{
		"username": true, "email": true, "displayName": true, "password": true,
		"permissions": true, "libraryIds": true, "accessSchedule": true, "tagPolicy": true,
		"devicePolicy": true, "channelPolicy": true, "maxContentRating": true,
		"maxActiveSessions": true, "maxActiveStreams": true, "remoteBitrateLimitMbps": true,
	}
	for name, value := range fields {
		if !allowed[name] {
			return invalidRequestField(name, name+" is not a supported user field")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return invalidRequestField(name, name+" must not be null")
		}
	}
	type userPatchRequest UserPatchRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded userPatchRequest
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*request = UserPatchRequest(decoded)
	return nil
}

type AuthMeResponse struct {
	Authenticated         bool           `json:"authenticated"`
	SetupRequired         bool           `json:"setupRequired"`
	User                  *User          `json:"user,omitempty"`
	ServerID              string         `json:"serverId,omitempty"`
	ServerFriendlyName    string         `json:"serverFriendlyName,omitempty"`
	AccountMode           string         `json:"accountMode,omitempty"`
	AuthProvider          string         `json:"authProvider,omitempty"`
	Authority             string         `json:"authority,omitempty"`
	AccountID             string         `json:"accountId,omitempty"`
	ProfileID             string         `json:"profileId,omitempty"`
	ProfileIdentityID     string         `json:"profileIdentityId,omitempty"`
	AuthorizationRevision string         `json:"authorizationRevision,omitempty"`
	SignInMethods         []SignInMethod `json:"signInMethods,omitempty"`
}

type BrowserLoginRequest struct {
	Login             string `json:"login"`
	Password          string `json:"password"`
	RememberOnBrowser *bool  `json:"rememberOnBrowser,omitempty"`
}

type NativeSessionCreateRequest struct {
	Login          string `json:"login"`
	Password       string `json:"password"`
	InstallationID string `json:"installationId,omitempty"`
	DeviceName     string `json:"deviceName"`
	App            string `json:"app"`
	Platform       string `json:"platform"`
}

// PorticoSessionAttachRequest exchanges an already-authorized, server-scoped
// Portico Cloud credential for a session owned and refreshable by this server.
// The Cloud credential is used only for this attachment request and is never
// accepted as an ordinary Portico Server API credential.
type PorticoSessionAttachRequest struct {
	AccessToken       string                         `json:"accessToken"`
	SelectionEnvelope HostedProfileSelectionEnvelope `json:"selectionEnvelope"`
	InstallationID    string                         `json:"installationId,omitempty"`
	DeviceName        string                         `json:"deviceName"`
	App               string                         `json:"app"`
	Platform          string                         `json:"platform"`
}

type NativeSessionRefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
	RotationKey  string `json:"rotationKey,omitempty"`
}

type NativeSessionCredentials struct {
	TokenType             string `json:"tokenType"`
	AccessToken           string `json:"accessToken"`
	AccessExpiresAt       string `json:"accessExpiresAt"`
	RefreshToken          string `json:"refreshToken"`
	RefreshExpiresAt      string `json:"refreshExpiresAt"`
	User                  User   `json:"user"`
	Device                Device `json:"device"`
	Authority             string `json:"authority"`
	AccountID             string `json:"accountId"`
	ProfileID             string `json:"profileId"`
	AuthorizationRevision string `json:"authorizationRevision"`
	ServerID              string `json:"serverId,omitempty"`
	ServerFriendlyName    string `json:"serverFriendlyName,omitempty"`
}

type AuthCapabilitiesResponse struct {
	SetupRequired             bool              `json:"setupRequired"`
	LocalCredentialsEnabled   bool              `json:"localCredentialsEnabled"`
	PorticoAccountAuthEnabled bool              `json:"porticoAccountAuthEnabled"`
	ServerFriendlyName        string            `json:"serverFriendlyName,omitempty"`
	PublicUserPickerEnabled   bool              `json:"publicUserPickerEnabled"`
	VisibleUsers              []PublicLoginUser `json:"visibleUsers"`
	GeneratedAt               string            `json:"generatedAt"`
}

type ServerCapabilitiesResponse struct {
	Version                 string                                 `json:"version"`
	APIVersion              string                                 `json:"apiVersion"`
	Features                map[string]bool                        `json:"features"`
	OperationalCapabilities map[string]OperationalCapabilityStatus `json:"operationalCapabilities"`
	Permissions             map[string]bool                        `json:"permissions"`
	PermissionCatalog       []string                               `json:"permissionCatalog"`
	MarkerTypes             []string                               `json:"markerTypes"`
	ExtraTypes              []string                               `json:"extraTypes"`
	GeneratedAt             string                                 `json:"generatedAt"`
}

type OperationalCapabilityStatus struct {
	Supported    bool   `json:"supported"`
	State        string `json:"state"`
	Revision     int    `json:"revision"`
	ReasonCode   string `json:"reasonCode,omitempty"`
	Remediation  string `json:"remediation,omitempty"`
	CacheSeconds int    `json:"cacheSeconds"`
}

type PublicLoginUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type QuickConnectStartRequest struct {
	InstallationID string `json:"installationId"`
	DeviceName     string `json:"deviceName,omitempty"`
	App            string `json:"app,omitempty"`
	Platform       string `json:"platform,omitempty"`
}

type QuickConnectStartResponse struct {
	Code            string `json:"code"`
	ProtocolVersion int    `json:"protocolVersion"`
	Secret          string `json:"secret"`
	ExpiresAt       string `json:"expiresAt"`
	ApprovalURL     string `json:"approvalUrl,omitempty"`
	DeepLinkURL     string `json:"deepLinkUrl,omitempty"`
}

type QuickConnectStatusResponse struct {
	Status    string `json:"status"`
	Device    string `json:"device,omitempty"`
	App       string `json:"app,omitempty"`
	Platform  string `json:"platform,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

type QuickConnectRequest struct {
	ID              string `json:"id"`
	Code            string `json:"code"`
	ProtocolVersion int    `json:"protocolVersion"`
	Device          string `json:"device"`
	App             string `json:"app"`
	Platform        string `json:"platform"`
	ClientIP        string `json:"clientIp,omitempty"`
	ExpiresAt       string `json:"expiresAt"`
	CreatedAt       string `json:"createdAt"`
}

type TVSetupSessionRequest struct {
	InstallationID  string `json:"installationId,omitempty"`
	DevicePublicKey string `json:"devicePublicKey"`
	DeviceName      string `json:"deviceName,omitempty"`
	Platform        string `json:"platform,omitempty"`
	AppVersion      string `json:"appVersion,omitempty"`
	ServerHint      string `json:"serverHint,omitempty"`
	AuthModeHint    string `json:"authModeHint,omitempty"`
	EndpointURL     string `json:"endpointUrl,omitempty"`
}

type TVSetupSessionResponse struct {
	SetupSessionID      string                 `json:"setupSessionId"`
	Code                string                 `json:"code"`
	Status              string                 `json:"status"`
	ProtocolVersion     int                    `json:"protocolVersion"`
	Service             string                 `json:"service"`
	DevicePublicKey     string                 `json:"devicePublicKey"`
	DeviceName          string                 `json:"deviceName"`
	Platform            string                 `json:"platform"`
	AppVersion          string                 `json:"appVersion,omitempty"`
	ServerHint          string                 `json:"serverHint,omitempty"`
	AuthModeHint        string                 `json:"authModeHint"`
	EndpointURL         string                 `json:"endpointUrl,omitempty"`
	ExpiresAt           string                 `json:"expiresAt"`
	PollIntervalSeconds int                    `json:"pollIntervalSeconds"`
	EncryptedGrant      *TVSetupEncryptedGrant `json:"encryptedGrant,omitempty"`
}

type TVSetupGrantRequest struct {
	SetupSessionID  string `json:"setupSessionId"`
	Code            string `json:"code"`
	DevicePublicKey string `json:"devicePublicKey,omitempty"`
}

type TVSetupGrantResponse struct {
	SetupSessionID string                `json:"setupSessionId"`
	Status         string                `json:"status"`
	EncryptedGrant TVSetupEncryptedGrant `json:"encryptedGrant"`
	ExpiresAt      string                `json:"expiresAt"`
}

type TVSetupRedeemRequest struct {
	SetupSessionID string `json:"setupSessionId"`
	GrantSecret    string `json:"grantSecret"`
	DeviceName     string `json:"deviceName,omitempty"`
	Platform       string `json:"platform,omitempty"`
	AppVersion     string `json:"appVersion,omitempty"`
}

type TVSetupEncryptedGrant struct {
	Version         int    `json:"version"`
	Algorithm       string `json:"algorithm"`
	ServerPublicKey string `json:"serverPublicKey"`
	Nonce           string `json:"nonce"`
	Ciphertext      string `json:"ciphertext"`
}

type SystemIdentityResponse struct {
	ServerID             string   `json:"serverId"`
	FriendlyName         string   `json:"friendlyName"`
	Claimed              bool     `json:"claimed"`
	PublicKeyFingerprint string   `json:"publicKeyFingerprint"`
	TrustModel           []string `json:"trustModel"`
	Placeholder          bool     `json:"placeholder"`
	Note                 string   `json:"note"`
}

type BrandingInfo struct {
	ApplicationName string `json:"applicationName"`
	Tagline         string `json:"tagline,omitempty"`
	LoginDisclaimer string `json:"loginDisclaimer,omitempty"`
	LogoURL         string `json:"logoUrl,omitempty"`
	AccentColor     string `json:"accentColor,omitempty"`
}

type ConnectionURL struct {
	Type        string `json:"type"`
	URL         string `json:"url"`
	Reachable   bool   `json:"reachable"`
	Placeholder bool   `json:"placeholder"`
	Note        string `json:"note,omitempty"`
}

type RemoteAccessInfo struct {
	Status      string `json:"status"`
	Discovery   string `json:"discovery"`
	DynamicDNS  string `json:"dynamicDns"`
	TLS         string `json:"tls"`
	MediaPath   string `json:"mediaPath"`
	Placeholder bool   `json:"placeholder"`
}

type NetworkConnectionInfo struct {
	ServerID     string           `json:"serverId"`
	LocalURLs    []ConnectionURL  `json:"localUrls"`
	RemoteAccess RemoteAccessInfo `json:"remoteAccess"`
	SecurePolicy string           `json:"securePolicy"`
	LANNetworks  []string         `json:"lanNetworks"`
	Placeholder  bool             `json:"placeholder"`
	Note         string           `json:"note"`
}

type DLNALibraryExposure struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Count   int    `json:"count"`
	Exposed bool   `json:"exposed"`
}

type DLNAStatus struct {
	Enabled                     bool                  `json:"enabled"`
	FriendlyName                string                `json:"friendlyName"`
	AdvertiseURL                string                `json:"advertiseUrl"`
	DeviceDescriptionURL        string                `json:"deviceDescriptionUrl"`
	ContentDirectoryURL         string                `json:"contentDirectoryUrl"`
	MediaServerURN              string                `json:"mediaServerUrn"`
	SSDPDiscovery               string                `json:"ssdpDiscovery"`
	ExposedLibraries            []DLNALibraryExposure `json:"exposedLibraries"`
	UnauthenticatedLANAccess    bool                  `json:"unauthenticatedLanAccess"`
	ByteRangeStreamingSupported bool                  `json:"byteRangeStreamingSupported"`
	DiscoveryIntervalSeconds    int                   `json:"discoveryIntervalSeconds"`
	AnnouncementLeaseSeconds    int                   `json:"announcementLeaseSeconds"`
	ProtocolInfo                string                `json:"protocolInfo"`
	RendererProfileVersion      string                `json:"rendererProfileVersion"`
	ReachableProtocols          []string              `json:"reachableProtocols"`
	Note                        string                `json:"note"`
}

type LibraryScanSummary struct {
	JobID     string `json:"jobId,omitempty"`
	Status    string `json:"status"`
	Progress  int    `json:"progress,omitempty"`
	Message   string `json:"message,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type Library struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Type        string              `json:"type"`
	SortOrder   int                 `json:"sortOrder"`
	Path        string              `json:"path,omitempty"`
	Paths       []string            `json:"paths"`
	Count       int                 `json:"count"`
	Settings    map[string]any      `json:"settings"`
	ScanSummary *LibraryScanSummary `json:"scanSummary,omitempty"`
}

type LibraryCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	Description string `json:"description,omitempty"`
	Filter      string `json:"filter"`
	Count       int    `json:"count"`
	Image       string `json:"image,omitempty"`
	Source      string `json:"source,omitempty"`
}

type LibraryFacetValue struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	EntityKind string `json:"entityKind"`
	Filter     string `json:"filter"`
	Count      int    `json:"count"`
	Image      string `json:"image,omitempty"`
}

type LibrarySourceGroup struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Label            string `json:"label"`
	Path             string `json:"path,omitempty"`
	SourceType       string `json:"sourceType"`
	Filter           string `json:"filter"`
	ItemCount        int    `json:"itemCount"`
	FileCount        int    `json:"fileCount"`
	MissingFileCount int    `json:"missingFileCount"`
	SizeBytes        int64  `json:"sizeBytes"`
}

type FilesystemBrowseResponse struct {
	Path    string                  `json:"path"`
	Parent  string                  `json:"parent,omitempty"`
	Roots   []FilesystemRoot        `json:"roots,omitempty"`
	Entries []FilesystemBrowseEntry `json:"entries"`
}

type FilesystemCreateDirectoryRequest struct {
	Path string `json:"path"`
}

type FilesystemRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type FilesystemBrowseEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Readable bool   `json:"readable"`
}

type StoragePathsResponse struct {
	AppDataDir              string `json:"appDataDir"`
	ConfigPath              string `json:"configPath"`
	ActiveDatabasePath      string `json:"activeDatabasePath"`
	ConfiguredDatabasePath  string `json:"configuredDatabasePath"`
	BackupDirectory         string `json:"backupDirectory"`
	DatabaseRestartRequired bool   `json:"databaseRestartRequired"`
}

type StoragePathsRequest struct {
	DatabasePath    string `json:"databasePath"`
	BackupDirectory string `json:"backupDirectory"`
	CopyDatabase    bool   `json:"copyDatabase"`
}

type LocalizationInfo struct {
	Locales       []LocalizationOption    `json:"locales"`
	Languages     []LocalizationOption    `json:"languages"`
	Countries     []LocalizationOption    `json:"countries"`
	TimeZones     []string                `json:"timeZones"`
	RatingSystems []LocalizationRatingSet `json:"ratingSystems"`
	GeneratedAt   string                  `json:"generatedAt"`
}

type LocalizationOption struct {
	ID     string            `json:"id"`
	Label  string            `json:"label"`
	Labels map[string]string `json:"labels,omitempty"`
}

type LocalizationRatingSet struct {
	Country string               `json:"country"`
	System  string               `json:"system"`
	Label   string               `json:"label,omitempty"`
	Labels  map[string]string    `json:"labels,omitempty"`
	Ratings []LocalizationRating `json:"ratings"`
}

type LocalizationRating struct {
	ID         string            `json:"id"`
	Label      string            `json:"label"`
	Labels     map[string]string `json:"labels,omitempty"`
	Rank       int               `json:"rank"`
	MinimumAge int               `json:"minimumAge,omitempty"`
}

type CreateLibraryRequest struct {
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Path     string         `json:"path"`
	Paths    []string       `json:"paths"`
	Settings map[string]any `json:"settings"`
}

type UpdateLibraryRequest struct {
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Path     string         `json:"path"`
	Paths    []string       `json:"paths"`
	Settings map[string]any `json:"settings"`
}

type ImageSet struct {
	Poster   string `json:"poster"`
	Backdrop string `json:"backdrop"`
	Thumb    string `json:"thumb"`
}

type DisplayImageSet struct {
	Poster   string `json:"poster,omitempty"`
	Backdrop string `json:"backdrop,omitempty"`
	Thumb    string `json:"thumb,omitempty"`
}

type MediaState struct {
	Watchlisted     bool        `json:"watchlisted"`
	Favorite        bool        `json:"favorite"`
	Reaction        string      `json:"reaction,omitempty"`
	Watched         bool        `json:"watched"`
	ProgressSeconds int         `json:"progressSeconds"`
	Rating          int         `json:"rating"`
	LastPlayedAt    string      `json:"lastPlayedAt,omitempty"`
	Resume          *ResumeInfo `json:"resume,omitempty"`
}

type ResumeInfo struct {
	PositionSeconds     int    `json:"positionSeconds"`
	RemainingSeconds    int    `json:"remainingSeconds,omitempty"`
	ChapterID           string `json:"chapterId,omitempty"`
	ChapterTitle        string `json:"chapterTitle,omitempty"`
	ChapterIndex        int    `json:"chapterIndex,omitempty"`
	ChapterStartSeconds int    `json:"chapterStartSeconds,omitempty"`
	ChapterEndSeconds   int    `json:"chapterEndSeconds,omitempty"`
}

type Stream struct {
	ID                 string  `json:"id"`
	FileID             string  `json:"fileId,omitempty"`
	SourceKind         string  `json:"-"`
	StorageKey         string  `json:"-"`
	Index              int     `json:"index"`
	Kind               string  `json:"kind"`
	Codec              string  `json:"codec"`
	Language           string  `json:"language,omitempty"`
	Channels           int     `json:"channels,omitempty"`
	Bitrate            int     `json:"bitrate,omitempty"`
	Width              int     `json:"width,omitempty"`
	Height             int     `json:"height,omitempty"`
	FrameRate          float64 `json:"frameRate,omitempty"`
	AspectRatio        string  `json:"aspectRatio,omitempty"`
	SampleRate         int     `json:"sampleRate,omitempty"`
	ChannelLayout      string  `json:"channelLayout,omitempty"`
	Default            bool    `json:"default,omitempty"`
	Forced             bool    `json:"forced,omitempty"`
	HearingImpaired    bool    `json:"hearingImpaired,omitempty"`
	DisplayTitle       string  `json:"displayTitle"`
	SourceURL          string  `json:"sourceUrl,omitempty"`
	SubtitleOffsetMs   int     `json:"subtitleOffsetMs,omitempty"`
	Profile            string  `json:"profile,omitempty"`
	Level              int     `json:"level,omitempty"`
	PixelFormat        string  `json:"pixelFormat,omitempty"`
	BitDepth           int     `json:"bitDepth,omitempty"`
	ColorTransfer      string  `json:"colorTransfer,omitempty"`
	ColorPrimaries     string  `json:"colorPrimaries,omitempty"`
	ColorSpace         string  `json:"colorSpace,omitempty"`
	ChromaLocation     string  `json:"chromaLocation,omitempty"`
	FieldOrder         string  `json:"fieldOrder,omitempty"`
	DynamicRange       string  `json:"dynamicRange,omitempty"`
	DolbyVisionProfile string  `json:"dolbyVisionProfile,omitempty"`
	ExactSeekSafe      bool    `json:"exactSeekSafe,omitempty"`
	KeyframeEvidenceAt string  `json:"keyframeEvidenceAt,omitempty"`
}

type SubtitleUpdateRequest struct {
	OffsetMs int `json:"offsetMs"`
}

type MediaAttachment struct {
	ID        string `json:"id"`
	StreamID  string `json:"streamId,omitempty"`
	Filename  string `json:"filename"`
	MimeType  string `json:"mimeType,omitempty"`
	Codec     string `json:"codec,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	URL       string `json:"url,omitempty"`
}

type MediaSegment struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	StartSeconds  int     `json:"startSeconds"`
	EndSeconds    int     `json:"endSeconds"`
	AutomaticSafe bool    `json:"automaticSafe"`
	Source        string  `json:"source,omitempty"`
	Provider      string  `json:"provider,omitempty"`
	Confidence    float64 `json:"confidence,omitempty"`
	CreatedAt     string  `json:"createdAt,omitempty"`
}

type MediaExtraRelationship struct {
	Type  string      `json:"type"`
	Label string      `json:"label"`
	Items []MediaItem `json:"items"`
}

type AudioNormalization struct {
	TrackGainDB    float64 `json:"trackGainDb"`
	TrackPeak      float64 `json:"trackPeak"`
	AlbumGainDB    float64 `json:"albumGainDb"`
	AlbumPeak      float64 `json:"albumPeak"`
	IntegratedLUFS float64 `json:"integratedLufs"`
	Source         string  `json:"source,omitempty"`
	UpdatedAt      string  `json:"updatedAt,omitempty"`
}

type MediaFileVersion struct {
	ID                 string   `json:"id"`
	Path               string   `json:"path,omitempty"`
	OriginalFilename   string   `json:"originalFilename,omitempty"`
	Quality            string   `json:"quality,omitempty"`
	Container          string   `json:"container,omitempty"`
	SourceType         string   `json:"sourceType,omitempty"`
	Analysis           string   `json:"streamAnalysisStatus,omitempty"`
	VersionLabel       string   `json:"versionLabel,omitempty"`
	Resolution         string   `json:"resolution,omitempty"`
	Source             string   `json:"source,omitempty"`
	VideoCodec         string   `json:"videoCodec,omitempty"`
	AudioCodec         string   `json:"audioCodec,omitempty"`
	DynamicRange       string   `json:"dynamicRange,omitempty"`
	ReleaseGroup       string   `json:"releaseGroup,omitempty"`
	ThreeD             bool     `json:"threeD,omitempty"`
	VersionGroup       string   `json:"versionGroup,omitempty"`
	QualityRank        int      `json:"qualityRank,omitempty"`
	SizeBytes          int64    `json:"sizeBytes,omitempty"`
	ModTime            string   `json:"-"`
	Available          bool     `json:"available"`
	MissingSince       string   `json:"missingSince,omitempty"`
	Selected           bool     `json:"selected,omitempty"`
	DurationSeconds    int      `json:"durationSeconds,omitempty"`
	Bitrate            int      `json:"bitrate,omitempty"`
	Width              int      `json:"width,omitempty"`
	Height             int      `json:"height,omitempty"`
	FrameRate          float64  `json:"frameRate,omitempty"`
	AspectRatio        string   `json:"aspectRatio,omitempty"`
	VideoProfile       string   `json:"videoProfile,omitempty"`
	VideoLevel         int      `json:"videoLevel,omitempty"`
	BitDepth           int      `json:"bitDepth,omitempty"`
	PixelFormat        string   `json:"pixelFormat,omitempty"`
	ColorTransfer      string   `json:"colorTransfer,omitempty"`
	ColorPrimaries     string   `json:"colorPrimaries,omitempty"`
	ColorSpace         string   `json:"colorSpace,omitempty"`
	ChromaLocation     string   `json:"chromaLocation,omitempty"`
	AudioChannels      int      `json:"audioChannels,omitempty"`
	AudioChannelLayout string   `json:"audioChannelLayout,omitempty"`
	AudioSampleRate    int      `json:"audioSampleRate,omitempty"`
	AudioBitrate       int      `json:"audioBitrate,omitempty"`
	Streams            []Stream `json:"streams,omitempty"`
}

type LiveTVSource struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Type                 string   `json:"type"`
	Enabled              bool     `json:"enabled"`
	M3UURL               string   `json:"m3uUrl,omitempty"`
	HasM3UText           bool     `json:"hasM3uText"`
	EPGURL               string   `json:"epgUrl,omitempty"`
	HasEPGText           bool     `json:"hasEpgText"`
	XtreamBaseURL        string   `json:"xtreamBaseUrl,omitempty"`
	XtreamUsername       string   `json:"xtreamUsername,omitempty"`
	HasXtreamPassword    bool     `json:"hasXtreamPassword"`
	HDHomeRunBaseURL     string   `json:"hdhomerunBaseUrl,omitempty"`
	UserAgent            string   `json:"userAgent,omitempty"`
	StreamBufferSeconds  int      `json:"streamBufferSeconds"`
	MaxRetrySeconds      int      `json:"maxRetrySeconds"`
	RefreshIntervalHours int      `json:"refreshIntervalHours"`
	TunerCount           int      `json:"tunerCount"`
	DiscoveredTunerCount int      `json:"discoveredTunerCount,omitempty"`
	TunerCountMode       string   `json:"tunerCountMode"`
	FilterCategories     []string `json:"filterCategories,omitempty"`
	FilterCountries      []string `json:"filterCountries,omitempty"`
	FilterRequireEPG     bool     `json:"filterRequireEpg"`
	KeywordAllow         []string `json:"keywordAllow,omitempty"`
	KeywordDeny          []string `json:"keywordDeny,omitempty"`
	SortOrder            int      `json:"sortOrder"`
	ChannelCount         int      `json:"channelCount"`
	ProgramCount         int      `json:"programCount"`
	LogoCount            int      `json:"logoCount"`
	HiddenChannelCount   int      `json:"hiddenChannelCount"`
	FavoriteChannelCount int      `json:"favoriteChannelCount"`
	LastRefreshedAt      string   `json:"lastRefreshedAt,omitempty"`
	LastError            string   `json:"lastError,omitempty"`
	CreatedAt            string   `json:"createdAt"`
	UpdatedAt            string   `json:"updatedAt"`
	Actions              []string `json:"actions"`
}

// LiveTVSourceSummary is the consumer-safe projection. Provider addresses,
// credentials, import filters, user agents, and operational error text stay on
// the owner administration contract and are never serialized into guide or
// ordinary Live TV responses.
type LiveTVSourceSummary struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Type                 string   `json:"type"`
	Enabled              bool     `json:"enabled"`
	SortOrder            int      `json:"sortOrder"`
	ChannelCount         int      `json:"channelCount"`
	ProgramCount         int      `json:"programCount"`
	LogoCount            int      `json:"logoCount"`
	HiddenChannelCount   int      `json:"hiddenChannelCount"`
	FavoriteChannelCount int      `json:"favoriteChannelCount"`
	LastRefreshedAt      string   `json:"lastRefreshedAt,omitempty"`
	Actions              []string `json:"actions"`
}

type LiveTVSourceRequest struct {
	Name                   string   `json:"name"`
	Type                   string   `json:"type"`
	Enabled                bool     `json:"enabled"`
	M3UURL                 string   `json:"m3uUrl"`
	M3UText                string   `json:"m3uText"`
	PreserveM3UText        bool     `json:"preserveM3uText"`
	EPGURL                 string   `json:"epgUrl"`
	EPGText                string   `json:"epgText"`
	PreserveEPGText        bool     `json:"preserveEpgText"`
	XtreamBaseURL          string   `json:"xtreamBaseUrl"`
	XtreamUsername         string   `json:"xtreamUsername"`
	XtreamPassword         string   `json:"xtreamPassword"`
	PreserveXtreamPassword bool     `json:"preserveXtreamPassword"`
	HDHomeRunBaseURL       string   `json:"hdhomerunBaseUrl"`
	UserAgent              string   `json:"userAgent"`
	StreamBufferSeconds    int      `json:"streamBufferSeconds"`
	MaxRetrySeconds        int      `json:"maxRetrySeconds"`
	RefreshIntervalHours   int      `json:"refreshIntervalHours"`
	TunerCount             *int     `json:"tunerCount,omitempty"`
	FilterCategories       []string `json:"filterCategories"`
	FilterCountries        []string `json:"filterCountries"`
	FilterRequireEPG       *bool    `json:"filterRequireEpg"`
	KeywordAllow           []string `json:"keywordAllow"`
	KeywordDeny            []string `json:"keywordDeny"`
}

type HDHomeRunDiscoveryCandidate struct {
	Name            string `json:"name"`
	BaseURL         string `json:"baseUrl"`
	DeviceID        string `json:"deviceId,omitempty"`
	ModelNumber     string `json:"modelNumber,omitempty"`
	FirmwareName    string `json:"firmwareName,omitempty"`
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	LineupURL       string `json:"lineupUrl,omitempty"`
	TunerCount      int    `json:"tunerCount,omitempty"`
}

type LiveTVChannel struct {
	ID              string   `json:"id"`
	SourceID        string   `json:"sourceId"`
	Number          string   `json:"number,omitempty"`
	Name            string   `json:"name"`
	LogoURL         string   `json:"logoUrl,omitempty"`
	TVGID           string   `json:"tvgId,omitempty"`
	GuideChannelRef string   `json:"guideChannelRef,omitempty"`
	GroupTitle      string   `json:"groupTitle,omitempty"`
	Country         string   `json:"country,omitempty"`
	Enabled         bool     `json:"enabled"`
	Favorite        bool     `json:"favorite"`
	Hidden          bool     `json:"hidden"`
	ProgramCount    int      `json:"programCount"`
	SortOrder       int      `json:"sortOrder"`
	Actions         []string `json:"actions"`
}

type LiveTVChannelStateRequest struct {
	Favorite        *bool   `json:"favorite"`
	Hidden          *bool   `json:"hidden"`
	GuideChannelRef *string `json:"guideChannelRef,omitempty"`
}

type LiveTVProgram struct {
	ID          string   `json:"id"`
	SourceID    string   `json:"sourceId"`
	ChannelID   string   `json:"channelId,omitempty"`
	ChannelRef  string   `json:"channelRef,omitempty"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle,omitempty"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	StartAt     string   `json:"startAt"`
	EndAt       string   `json:"endAt"`
	EpisodeNum  string   `json:"episodeNum,omitempty"`
	IsNew       bool     `json:"isNew"`
	Actions     []string `json:"actions"`
}

type LiveTVGuideCapabilities struct {
	CanPlay                 bool `json:"canPlay"`
	CanFavoriteChannels     bool `json:"canFavoriteChannels"`
	CanScheduleRecordings   bool `json:"canScheduleRecordings"`
	CanManageRecordingRules bool `json:"canManageRecordingRules"`
	CanManageSources        bool `json:"canManageSources"`
}

type LiveTVGuideResponse struct {
	Source            LiveTVSource            `json:"source"`
	Channels          []LiveTVChannel         `json:"channels"`
	Programs          []LiveTVProgram         `json:"programs"`
	ChannelGroups     []string                `json:"channelGroups"`
	From              string                  `json:"from"`
	To                string                  `json:"to"`
	ServerTime        string                  `json:"serverTime"`
	PageInfo          CursorPageInfo          `json:"pageInfo"`
	TotalChannels     int                     `json:"-"`
	Limit             int                     `json:"-"`
	Offset            int                     `json:"-"`
	HasMore           bool                    `json:"-"`
	ProgramsTruncated bool                    `json:"programsTruncated,omitempty"`
	Capabilities      LiveTVGuideCapabilities `json:"capabilities"`
}

type LiveTVChannelPageResponse struct {
	Items    []LiveTVChannel `json:"items"`
	PageInfo CursorPageInfo  `json:"pageInfo"`
	Groups   []string        `json:"groups"`
}

type MediaItem struct {
	ID                 string                   `json:"id"`
	MetadataRevision   int                      `json:"metadataRevision"`
	MetadataETag       string                   `json:"metadataEtag"`
	LibraryID          string                   `json:"libraryId,omitempty"`
	LibraryName        string                   `json:"libraryName,omitempty"`
	Counts             *MediaHierarchyCounts    `json:"counts,omitempty"`
	ParentID           string                   `json:"parentId,omitempty"`
	Type               string                   `json:"type"`
	Title              string                   `json:"title"`
	SortTitle          string                   `json:"sortTitle"`
	OriginalTitle      string                   `json:"originalTitle,omitempty"`
	Edition            string                   `json:"edition,omitempty"`
	Year               int                      `json:"year,omitempty"`
	DurationSeconds    int                      `json:"durationSeconds,omitempty"`
	Summary            string                   `json:"summary,omitempty"`
	Tagline            string                   `json:"tagline,omitempty"`
	ContentRating      string                   `json:"contentRating,omitempty"`
	CommunityRating    float64                  `json:"communityRating,omitempty"`
	CriticRating       int                      `json:"criticRating,omitempty"`
	Studio             string                   `json:"studio,omitempty"`
	Network            string                   `json:"network,omitempty"`
	Country            string                   `json:"country,omitempty"`
	Genres             []string                 `json:"genres"`
	Tags               []string                 `json:"tags"`
	Labels             []string                 `json:"labels"`
	AddedAt            string                   `json:"addedAt"`
	ReleaseDateKey     string                   `json:"-"`
	SeasonNumber       int                      `json:"seasonNumber,omitempty"`
	EpisodeNumber      int                      `json:"episodeNumber,omitempty"`
	IndexNumber        int                      `json:"indexNumber,omitempty"`
	ArtSeed            string                   `json:"artSeed,omitempty"`
	Artwork            map[string]string        `json:"artwork,omitempty"`
	TypedMetadata      map[string]string        `json:"typedMetadata,omitempty"`
	Images             ImageSet                 `json:"images"`
	DisplayImages      *DisplayImageSet         `json:"displayImages,omitempty"`
	State              MediaState               `json:"state"`
	Actions            []string                 `json:"actions"`
	Streams            []Stream                 `json:"streams,omitempty"`
	Attachments        []MediaAttachment        `json:"attachments,omitempty"`
	Segments           []MediaSegment           `json:"segments,omitempty"`
	AudioNormalization *AudioNormalization      `json:"audioNormalization,omitempty"`
	MediaFiles         []MediaFileVersion       `json:"mediaFiles,omitempty"`
	OptimizedVersions  []OptimizedVersion       `json:"optimizedVersions,omitempty"`
	Children           []MediaItem              `json:"children,omitempty"`
	ChildrenTruncated  bool                     `json:"childrenTruncated,omitempty"`
	PlaybackTarget     *MediaItem               `json:"playbackTarget,omitempty"`
	Extras             []MediaExtraRelationship `json:"extras,omitempty"`
	Chapters           []Chapter                `json:"chapters,omitempty"`
	ParentTitle        string                   `json:"parentTitle,omitempty"`
	GrandparentID      string                   `json:"grandparentId,omitempty"`
	GrandparentTitle   string                   `json:"grandparentTitle,omitempty"`
	SourceURL          string                   `json:"sourceUrl,omitempty"`
	// SourceUserAgent is an internal playback transport hint. It is never
	// serialized to clients and is only populated from an administrator-owned
	// Live TV source configuration after the source URL passes SSRF validation.
	SourceUserAgent    string                 `json:"-"`
	Missing            bool                   `json:"missing,omitempty"`
	FileCount          int                    `json:"fileCount,omitempty"`
	MissingFileCount   int                    `json:"missingFileCount,omitempty"`
	ProviderIDs        []MediaProviderID      `json:"providerIds,omitempty"`
	MatchCandidates    []MatchCandidate       `json:"-"`
	IdentityEvidence   []IdentityEvidence     `json:"-"`
	MediaImages        []MediaImage           `json:"mediaImages,omitempty"`
	Lyrics             []MediaLyric           `json:"lyrics,omitempty"`
	People             []MediaPerson          `json:"people,omitempty"`
	RecommendationRows []HomeRow              `json:"recommendationRows,omitempty"`
	LockedFields       []string               `json:"lockedFields,omitempty"`
	MetadataEvidence   *MediaMetadataEvidence `json:"metadataEvidence,omitempty"`
	SortArtistKey      string                 `json:"-"`
	SortAlbumArtistKey string                 `json:"-"`
	SortTrackArtistKey string                 `json:"-"`
	SortAuthorKey      string                 `json:"-"`
	SortNarratorKey    string                 `json:"-"`
	RandomKey          string                 `json:"-"`
	SearchRank         float64                `json:"-"`
	SearchBucket       int                    `json:"-"`
}

// MediaHierarchyCounts contains exact catalog totals for the hierarchy rooted
// at a media entity. Pointer fields distinguish an applicable zero count from
// a count that does not apply to that entity kind.
type MediaHierarchyCounts struct {
	SeasonCount  *int `json:"seasonCount,omitempty"`
	EpisodeCount *int `json:"episodeCount,omitempty"`
	ReleaseCount *int `json:"releaseCount,omitempty"`
	TrackCount   *int `json:"trackCount,omitempty"`
	BookCount    *int `json:"bookCount,omitempty"`
	ChapterCount *int `json:"chapterCount,omitempty"`
	ItemCount    *int `json:"itemCount,omitempty"`
}

// MediaMetadataEvidence is the bounded, viewer-safe projection of accepted or
// locked rich metadata at the MediaItem's exact metadata revision. Provider
// snapshots and internal acquisition details are intentionally not exposed.
type MediaMetadataEvidence struct {
	Revision      int                         `json:"revision"`
	Values        []MediaMetadataValue        `json:"values"`
	Relationships []MediaMetadataRelationship `json:"relationships"`
}

type MediaMetadataValue struct {
	Field      string          `json:"field"`
	Order      int             `json:"order"`
	Locale     string          `json:"locale,omitempty"`
	Value      json.RawMessage `json:"value"`
	SourceKind string          `json:"sourceKind"`
	Provider   string          `json:"provider,omitempty"`
	Confidence float64         `json:"confidence"`
	Decision   string          `json:"decision"`
}

type MediaMetadataRelationship struct {
	Type             string  `json:"type"`
	Name             string  `json:"name"`
	TargetKind       string  `json:"targetKind,omitempty"`
	Provider         string  `json:"provider,omitempty"`
	ExternalProvider string  `json:"externalProvider,omitempty"`
	ExternalID       string  `json:"externalId,omitempty"`
	Locale           string  `json:"locale,omitempty"`
	Country          string  `json:"country,omitempty"`
	Role             string  `json:"role,omitempty"`
	Order            int     `json:"order"`
	SourceKind       string  `json:"sourceKind"`
	Confidence       float64 `json:"confidence"`
	Decision         string  `json:"decision"`
}

type MediaLyric struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Provider  string `json:"provider,omitempty"`
	Format    string `json:"format"`
	Language  string `json:"language,omitempty"`
	Path      string `json:"path,omitempty"`
	Text      string `json:"text,omitempty"`
	Synced    bool   `json:"synced"`
	CreatedAt string `json:"createdAt"`
}

type LyricSearchCandidate struct {
	Provider        string `json:"provider"`
	ExternalID      string `json:"externalId"`
	TrackName       string `json:"trackName"`
	ArtistName      string `json:"artistName,omitempty"`
	AlbumName       string `json:"albumName,omitempty"`
	DurationSeconds int    `json:"durationSeconds,omitempty"`
	Format          string `json:"format"`
	Synced          bool   `json:"synced"`
	Instrumental    bool   `json:"instrumental,omitempty"`
}

type MediaPerson struct {
	ID                 string            `json:"id"`
	CanonicalPersonKey string            `json:"-"`
	Name               string            `json:"name"`
	Role               string            `json:"role"`
	Character          string            `json:"character,omitempty"`
	Source             string            `json:"source,omitempty"`
	SortOrder          int               `json:"sortOrder,omitempty"`
	ImageURL           string            `json:"imageUrl,omitempty"`
	ProviderIDs        map[string]string `json:"providerIds,omitempty"`
}

type MediaImage struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	Source    string  `json:"source"`
	Provider  string  `json:"provider,omitempty"`
	Path      string  `json:"path,omitempty"`
	RemoteURL string  `json:"remoteUrl,omitempty"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	Language  string  `json:"language,omitempty"`
	Rating    float64 `json:"rating,omitempty"`
	SortOrder int     `json:"sortOrder"`
	Preferred bool    `json:"preferred"`
	CreatedAt string  `json:"createdAt"`
}

type MediaProviderID struct {
	Provider     string  `json:"provider"`
	ExternalID   string  `json:"externalId"`
	ExternalType string  `json:"externalType"`
	Confidence   float64 `json:"confidence"`
	Source       string  `json:"source"`
	UpdatedAt    string  `json:"updatedAt"`
}

type MatchCandidate struct {
	Provider     string        `json:"provider"`
	ExternalID   string        `json:"externalId"`
	ExternalType string        `json:"externalType"`
	Title        string        `json:"title,omitempty"`
	Year         int           `json:"year,omitempty"`
	PosterURL    string        `json:"posterUrl,omitempty"`
	Overview     string        `json:"overview,omitempty"`
	Source       string        `json:"source"`
	Score        float64       `json:"score"`
	Accepted     bool          `json:"accepted"`
	Reasons      []scoreReason `json:"reasons"`
	CreatedAt    string        `json:"createdAt"`
}

type IdentityEvidence struct {
	Source     string  `json:"source"`
	Field      string  `json:"field"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	Path       string  `json:"path,omitempty"`
	UpdatedAt  string  `json:"updatedAt"`
}

type MetadataRepairItem struct {
	MediaID                 string             `json:"mediaId"`
	Title                   string             `json:"title"`
	Type                    string             `json:"type"`
	Provider                string             `json:"provider"`
	ExternalID              string             `json:"externalId"`
	ExternalType            string             `json:"externalType"`
	Confidence              float64            `json:"confidence"`
	Source                  string             `json:"source"`
	UpdatedAt               string             `json:"updatedAt"`
	Reason                  string             `json:"reason"`
	LatestCandidateScore    float64            `json:"latestCandidateScore,omitempty"`
	LatestCandidateSource   string             `json:"latestCandidateSource,omitempty"`
	LatestCandidateAt       string             `json:"latestCandidateAt,omitempty"`
	LatestCandidateAccepted bool               `json:"latestCandidateAccepted,omitempty"`
	LatestCandidateReasons  []scoreReason      `json:"latestCandidateReasons,omitempty"`
	Evidence                []IdentityEvidence `json:"evidence,omitempty"`
}

type MetadataRepairResponse struct {
	Items []MetadataRepairItem `json:"items"`
	Total int                  `json:"total"`
}

type MetadataHealthIssue struct {
	ID          string `json:"id"`
	MediaID     string `json:"mediaId"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	LibraryID   string `json:"libraryId,omitempty"`
	LibraryName string `json:"libraryName,omitempty"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Reason      string `json:"reason"`
	Detail      string `json:"detail,omitempty"`
	Action      string `json:"action,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

type MetadataHealthSummary struct {
	Category string `json:"category"`
	Label    string `json:"label"`
	Count    int    `json:"count"`
	Severity string `json:"severity"`
}

type MetadataHealthResponse struct {
	Items       []MetadataHealthIssue   `json:"items"`
	Summary     []MetadataHealthSummary `json:"summary"`
	Total       int                     `json:"total"`
	GeneratedAt string                  `json:"generatedAt"`
}

type MetadataHealthActionRequest struct {
	Category  string `json:"category"`
	LibraryID string `json:"libraryId,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type MetadataHealthActionResponse struct {
	Category      string `json:"category"`
	Queued        int    `json:"queued"`
	AlreadyQueued int    `json:"alreadyQueued"`
	Skipped       int    `json:"skipped"`
	Total         int    `json:"total"`
	GeneratedAt   string `json:"generatedAt"`
	Jobs          []Job  `json:"jobs"`
}

type MetadataRepairRequest struct {
	MediaID          string `json:"mediaId"`
	ClearProviderIDs bool   `json:"clearProviderIds"`
}

type MetadataRepairActionResponse struct {
	Job                Job `json:"job"`
	ClearedProviderIDs int `json:"clearedProviderIds"`
	ClearedCandidates  int `json:"clearedCandidates"`
}

type UpdateMediaRequest struct {
	ExpectedRevision   *int               `json:"expectedRevision"`
	Title              *string            `json:"title"`
	SortTitle          *string            `json:"sortTitle"`
	OriginalTitle      *string            `json:"originalTitle"`
	Edition            *string            `json:"edition"`
	Year               *int               `json:"year"`
	DurationSeconds    *int               `json:"durationSeconds"`
	Summary            *string            `json:"summary"`
	Tagline            *string            `json:"tagline"`
	ContentRating      *string            `json:"contentRating"`
	CommunityRating    *float64           `json:"communityRating"`
	CriticRating       *int               `json:"criticRating"`
	Studio             *string            `json:"studio"`
	Network            *string            `json:"network"`
	Country            *string            `json:"country"`
	Genres             *[]string          `json:"genres"`
	Tags               *[]string          `json:"tags"`
	Labels             *[]string          `json:"labels"`
	SeasonNumber       *int               `json:"seasonNumber"`
	EpisodeNumber      *int               `json:"episodeNumber"`
	IndexNumber        *int               `json:"indexNumber"`
	ArtSeed            *string            `json:"artSeed"`
	Artwork            *map[string]string `json:"artwork"`
	TypedMetadata      *map[string]string `json:"typedMetadata"`
	People             *[]MediaPerson     `json:"people"`
	LockedFields       *[]string          `json:"lockedFields"`
	SourceURL          *string            `json:"sourceUrl"`
	metadataOrigin     metadataSourceKind
	metadataSource     string
	metadataProvider   string
	metadataOperation  string
	metadataActor      string
	metadataRefreshed  bool
	metadataIdentities []metadataProviderIdentityProposal
	metadataRich       *metadataProviderRichProposal
}

type MediaMatchSearchResponse struct {
	Items []MatchCandidate `json:"items"`
}

type ManualMediaMatchRequest struct {
	Provider     string `json:"provider"`
	ExternalID   string `json:"externalId"`
	ExternalType string `json:"externalType"`
}

type MediaSegmentRequest struct {
	Type          string  `json:"type"`
	StartSeconds  int     `json:"startSeconds"`
	EndSeconds    int     `json:"endSeconds"`
	AutomaticSafe bool    `json:"automaticSafe,omitempty"`
	Provider      string  `json:"provider,omitempty"`
	Confidence    float64 `json:"confidence,omitempty"`
}

type DeleteMediaRequest struct {
	DeleteFiles  bool   `json:"deleteFiles"`
	Confirmation string `json:"confirmation,omitempty"`
}

type DeleteMediaResponse struct {
	OK           bool `json:"ok"`
	DeletedItems int  `json:"deletedItems"`
	TrashedFiles int  `json:"trashedFiles"`
}

type MediaTrickplaySet struct {
	ID              string `json:"id"`
	MediaID         string `json:"mediaId"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	TileWidth       int    `json:"tileWidth"`
	TileHeight      int    `json:"tileHeight"`
	IntervalSeconds int    `json:"intervalSeconds"`
	DurationSeconds int    `json:"durationSeconds"`
	TileCount       int    `json:"tileCount"`
	Stale           bool   `json:"stale"`
	CreatedAt       string `json:"createdAt"`
}

type OptimizedVersion struct {
	ID              string `json:"id"`
	MediaID         string `json:"mediaId"`
	Profile         string `json:"profile"`
	ProfileName     string `json:"profileName,omitempty"`
	Path            string `json:"path,omitempty"`
	SizeBytes       int64  `json:"sizeBytes"`
	DownloadURL     string `json:"downloadUrl,omitempty"`
	StreamURL       string `json:"streamUrl,omitempty"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	Container       string `json:"container,omitempty"`
	VideoCodec      string `json:"videoCodec,omitempty"`
	AudioCodec      string `json:"audioCodec,omitempty"`
	Width           int    `json:"width,omitempty"`
	Height          int    `json:"height,omitempty"`
	Bitrate         int    `json:"bitrate,omitempty"`
	DurationSeconds int    `json:"durationSeconds,omitempty"`
	Available       bool   `json:"available"`
}

type OptimizedVersionProfile struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Height    int    `json:"height"`
	VideoKbps int    `json:"videoKbps"`
	AudioKbps int    `json:"audioKbps"`
	Default   bool   `json:"default,omitempty"`
}

type OptimizedVersionRequest struct {
	Profile string `json:"profile"`
}

type OptimizedVersionListResponse struct {
	Items          []OptimizedVersion        `json:"items"`
	Profiles       []OptimizedVersionProfile `json:"profiles"`
	DefaultProfile string                    `json:"defaultProfile"`
	Total          int                       `json:"total"`
}

type DownloadOption struct {
	ID                       string `json:"id"`
	Kind                     string `json:"kind"`
	Profile                  string `json:"profile,omitempty"`
	Label                    string `json:"label"`
	Description              string `json:"description,omitempty"`
	Available                bool   `json:"available"`
	RequiresOptimizedVersion bool   `json:"requiresOptimizedVersion,omitempty"`
	URL                      string `json:"url,omitempty"`
	SizeBytes                int64  `json:"sizeBytes,omitempty"`
	Container                string `json:"container,omitempty"`
	VideoCodec               string `json:"videoCodec,omitempty"`
	AudioCodec               string `json:"audioCodec,omitempty"`
	SourceKind               string `json:"sourceKind,omitempty"`
	Job                      *Job   `json:"job,omitempty"`
}

type DownloadOptionsResponse struct {
	Media             MediaItem                 `json:"media"`
	Options           []DownloadOption          `json:"options"`
	OptimizedVersions []OptimizedVersion        `json:"optimizedVersions"`
	Profiles          []OptimizedVersionProfile `json:"profiles"`
	DefaultProfile    string                    `json:"defaultProfile"`
	CanDownload       bool                      `json:"canDownload"`
}

type DownloadPreparation struct {
	ID               string `json:"id"`
	MediaID          string `json:"mediaId"`
	MediaTitle       string `json:"mediaTitle"`
	QualityProfile   string `json:"qualityProfile"`
	State            string `json:"state"`
	Progress         int    `json:"progress"`
	SizeBytes        int64  `json:"sizeBytes,omitempty"`
	ErrorCode        string `json:"-"`
	FailureMessageID string `json:"failureMessageId,omitempty"`
	JobID            string `json:"jobId,omitempty"`
	CanPause         bool   `json:"canPause"`
	CanCancel        bool   `json:"canCancel"`
	CanRetry         bool   `json:"canRetry"`
	CanRemove        bool   `json:"canRemove"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}

type DownloadPreparationCreateRequest struct {
	MediaID        string `json:"mediaId"`
	QualityProfile string `json:"qualityProfile"`
}

type DownloadPreparationUpdateRequest struct {
	Action string `json:"action"`
}

type HomeRow struct {
	ID                 string      `json:"id"`
	Kind               string      `json:"kind,omitempty"`
	Title              string      `json:"title"`
	Explanation        string      `json:"explanation,omitempty"`
	Type               string      `json:"type"`
	ArtworkShape       string      `json:"artworkShape,omitempty"`
	LibraryID          string      `json:"libraryId,omitempty"`
	Endpoint           string      `json:"endpoint,omitempty"`
	Priority           int         `json:"priority,omitempty"`
	DefaultVisible     bool        `json:"defaultVisible"`
	Required           bool        `json:"required"`
	Hideable           bool        `json:"hideable"`
	Reorderable        bool        `json:"reorderable"`
	Critical           bool        `json:"critical,omitempty"`
	CursorCapable      bool        `json:"cursorCapable,omitempty"`
	PrivacySensitivity string      `json:"privacySensitivity,omitempty"`
	PolicyState        string      `json:"policyState,omitempty"`
	Controls           []string    `json:"controls,omitempty"`
	CacheTTLSeconds    int         `json:"cacheTtlSeconds,omitempty"`
	Items              []MediaItem `json:"items"`
	Total              int         `json:"total,omitempty"`
	Limit              int         `json:"limit,omitempty"`
	Offset             int         `json:"offset,omitempty"`
	HasMore            bool        `json:"hasMore,omitempty"`
	NextCursor         string      `json:"nextCursor,omitempty"`
}

type HomeResponse struct {
	Pivots []string  `json:"pivots"`
	Rows   []HomeRow `json:"rows"`
}

type SearchRequest struct {
	Query         string   `json:"query"`
	EntityKinds   []string `json:"entityKinds,omitempty"`
	LibraryIDs    []string `json:"libraryIds,omitempty"`
	Group         string   `json:"group,omitempty"`
	Sort          string   `json:"sort,omitempty"`
	Direction     string   `json:"direction,omitempty"`
	Cursor        string   `json:"cursor,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	RecordHistory bool     `json:"recordHistory,omitempty"`
}

type SearchHistoryItem struct {
	Query      string `json:"query"`
	LastUsedAt string `json:"lastUsedAt"`
	UseCount   int    `json:"useCount"`
}

type SearchHistoryResponse struct {
	Items []SearchHistoryItem `json:"items"`
}

type PersonSummary struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	ImageURL string   `json:"imageUrl,omitempty"`
	Roles    []string `json:"roles"`
}

type PersonDetailResponse struct {
	Person   PersonSummary  `json:"person"`
	Credits  []MediaCard    `json:"credits"`
	PageInfo CursorPageInfo `json:"pageInfo"`
}

type MediaCardPageResponse struct {
	Items    []MediaCard    `json:"items"`
	PageInfo CursorPageInfo `json:"pageInfo"`
}

type SearchGroup struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	EntityKind string      `json:"entityKind"`
	Status     string      `json:"status"`
	ErrorCode  string      `json:"errorCode,omitempty"`
	MessageID  string      `json:"messageId,omitempty"`
	Items      []MediaItem `json:"items"`
	HasMore    bool        `json:"hasMore"`
	NextCursor string      `json:"nextCursor,omitempty"`
}

type SearchResponse struct {
	Query     string        `json:"query"`
	Sort      string        `json:"sort"`
	Direction string        `json:"direction"`
	Groups    []SearchGroup `json:"groups"`
}

type MediaSuggestion struct {
	Item   MediaItem `json:"item"`
	Reason string    `json:"reason"`
	Source string    `json:"source"`
	Score  float64   `json:"score"`
}

type SuggestionsResponse struct {
	Items       []MediaSuggestion `json:"items"`
	Rows        []HomeRow         `json:"rows,omitempty"`
	Total       int               `json:"total"`
	GeneratedAt string            `json:"generatedAt"`
}

type ListResponse[T any] struct {
	Items      []T    `json:"items"`
	Total      int    `json:"total"`
	Sort       string `json:"sort,omitempty"`
	Filter     string `json:"filter,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	HasMore    bool   `json:"hasMore,omitempty"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// MarshalJSON keeps the public collection contract stable for a legitimate
// zero-result response. Go's nil slice normally serializes as null, but every
// Portico client treats items as an array and an empty collection is not an
// exceptional state.
func (response ListResponse[T]) MarshalJSON() ([]byte, error) {
	type wireListResponse ListResponse[T]
	if response.Items == nil {
		response.Items = []T{}
	}
	return json.Marshal(wireListResponse(response))
}

// CursorListResponse is the public shape for collections that can grow without
// a practical bound. Clients must treat cursors as opaque and restart from the
// first page after changing any filter or sort input.
type CursorListResponse[T any] struct {
	Items    []T            `json:"items"`
	PageInfo CursorPageInfo `json:"pageInfo"`
}

func (response CursorListResponse[T]) MarshalJSON() ([]byte, error) {
	type wireCursorListResponse CursorListResponse[T]
	if response.Items == nil {
		response.Items = []T{}
	}
	return json.Marshal(wireCursorListResponse(response))
}

type Device struct {
	ID                     string        `json:"id"`
	InstallationID         string        `json:"installationId,omitempty"`
	UserID                 string        `json:"userId"`
	User                   string        `json:"user"`
	Name                   string        `json:"name"`
	AutoName               string        `json:"autoName"`
	App                    string        `json:"app"`
	Platform               string        `json:"platform"`
	UserAgent              string        `json:"userAgent,omitempty"`
	ClientIP               string        `json:"clientIp,omitempty"`
	Trusted                bool          `json:"trusted"`
	RemoteBitrateLimitMbps int           `json:"remoteBitrateLimitMbps,omitempty"`
	Options                DeviceOptions `json:"options"`
	RevokedAt              string        `json:"revokedAt,omitempty"`
	SessionCount           int           `json:"sessionCount"`
	CreatedAt              string        `json:"createdAt"`
	LastSeenAt             string        `json:"lastSeenAt"`
}

type DeviceUpdateRequest struct {
	Name                   *string        `json:"name,omitempty"`
	Trusted                *bool          `json:"trusted,omitempty"`
	RemoteBitrateLimitMbps *int           `json:"remoteBitrateLimitMbps,omitempty"`
	Options                *DeviceOptions `json:"options,omitempty"`
}

type DeviceOptions struct {
	PreferredAudioLanguage    string `json:"preferredAudioLanguage,omitempty"`
	PreferredSubtitleLanguage string `json:"preferredSubtitleLanguage,omitempty"`
	SubtitleMode              string `json:"subtitleMode,omitempty"`
}

type AccountSession struct {
	ID           string `json:"id"`
	DeviceID     string `json:"deviceId,omitempty"`
	DeviceName   string `json:"deviceName"`
	App          string `json:"app"`
	Platform     string `json:"platform"`
	ClientIP     string `json:"clientIp,omitempty"`
	AuthProvider string `json:"authProvider"`
	Trusted      bool   `json:"trusted"`
	Current      bool   `json:"current"`
	CanRevoke    bool   `json:"canRevoke"`
	CreatedAt    string `json:"createdAt"`
	LastSeenAt   string `json:"lastSeenAt"`
	ExpiresAt    string `json:"expiresAt"`
}

type APIKey struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	UserID     string   `json:"userId"`
	Username   string   `json:"username,omitempty"`
	LastFour   string   `json:"lastFour"`
	Scopes     []string `json:"scopes"`
	CreatedAt  string   `json:"createdAt"`
	LastUsedAt string   `json:"lastUsedAt,omitempty"`
	RevokedAt  string   `json:"revokedAt,omitempty"`
}

type APIKeyCreateRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

type APIKeyCreateResponse struct {
	Key   APIKey `json:"key"`
	Token string `json:"token"`
}

type AuditEvent struct {
	ID           string            `json:"id"`
	ActorUserID  string            `json:"actorUserId,omitempty"`
	ActorEmail   string            `json:"actorEmail,omitempty"`
	Action       string            `json:"action"`
	ResourceType string            `json:"resourceType,omitempty"`
	ResourceID   string            `json:"resourceId,omitempty"`
	Severity     string            `json:"severity"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	ClientIP     string            `json:"clientIp,omitempty"`
	UserAgent    string            `json:"userAgent,omitempty"`
	CreatedAt    string            `json:"createdAt"`
}

type BackupInfo struct {
	Name                  string `json:"name"`
	SizeBytes             int64  `json:"sizeBytes"`
	CreatedAt             string `json:"createdAt"`
	Integrity             string `json:"integrity"`
	RestoreReady          bool   `json:"restoreReady"`
	ManifestPresent       bool   `json:"manifestPresent"`
	ChecksumSHA256        string `json:"checksumSha256,omitempty"`
	Release               string `json:"release,omitempty"`
	DatabaseFormatVersion int    `json:"databaseFormatVersion,omitempty"`
	MigrationHead         string `json:"migrationHead,omitempty"`
	MigrationLedgerSHA256 string `json:"migrationLedgerSha256,omitempty"`
	MigrationLedgerRows   int    `json:"migrationLedgerRows,omitempty"`
	MinimumReader         string `json:"minimumReader,omitempty"`
	ValidationCode        string `json:"validationCode,omitempty"`
	PublicationState      string `json:"publicationState,omitempty"`
	WarningCode           string `json:"warningCode,omitempty"`
	WarningMessage        string `json:"warningMessage,omitempty"`
}

type SystemDiagnostics struct {
	Version        string                        `json:"version"`
	GOOS           string                        `json:"goos"`
	GOARCH         string                        `json:"goarch"`
	Addr           string                        `json:"addr"`
	WebDistReady   bool                          `json:"webDistReady"`
	AppDataReady   bool                          `json:"appDataReady"`
	DatabaseReady  bool                          `json:"databaseReady"`
	Startup        StartupDiagnostics            `json:"startup"`
	Runtime        RuntimeDiagnostics            `json:"runtime"`
	SQLite         SQLiteDiagnostics             `json:"sqlite"`
	SQLiteHealth   SQLiteHealthDiagnostic        `json:"sqliteHealth"`
	AuthCaches     AuthorizationCacheDiagnostics `json:"authorizationCaches"`
	Resources      ResourceDiagnostics           `json:"resources"`
	Admission      AdmissionDiagnostics          `json:"admission"`
	JobLanes       []JobLaneDiagnostic           `json:"jobLanes"`
	WorkloadLanes  []WorkloadLaneDiagnostic      `json:"workloadLanes"`
	Dependencies   []RuntimeDependency           `json:"dependencies"`
	MediaToolchain MediaToolchainDiagnostic      `json:"mediaToolchain"`
	Playback       PlaybackRuntimeDiagnostics    `json:"playback"`
	GeneratedAt    string                        `json:"generatedAt"`
}

// PlaybackRuntimeDiagnostics is an owner-only, aggregate projection of the
// immutable plans currently executing. It deliberately excludes session,
// user, media, source, executable, device, and capability-evidence identity.
type PlaybackRuntimeDiagnostics struct {
	ActiveSessions      int                           `json:"activeSessions"`
	InvalidPlanBindings int                           `json:"invalidPlanBindings"`
	Executions          []PlaybackExecutionDiagnostic `json:"executions"`
	SourceHealth        map[string]int                `json:"sourceHealth"`
	Resources           PlaybackResourceDiagnostic    `json:"resources"`
}

type PlaybackExecutionDiagnostic struct {
	Count          int                        `json:"count"`
	Mode           string                     `json:"mode"`
	Protocol       string                     `json:"protocol"`
	Container      string                     `json:"container"`
	Streams        []PlaybackStreamDiagnostic `json:"streams"`
	Hardware       PlaybackHardwareDiagnostic `json:"hardware"`
	PlannerReasons []string                   `json:"plannerReasons"`
}

type PlaybackStreamDiagnostic struct {
	Kind        string `json:"kind"`
	Action      string `json:"action"`
	InputCodec  string `json:"inputCodec,omitempty"`
	OutputCodec string `json:"outputCodec,omitempty"`
}

type PlaybackHardwareDiagnostic struct {
	Backend string                    `json:"backend,omitempty"`
	Stages  []PlaybackStageDiagnostic `json:"stages"`
}

type PlaybackStageDiagnostic struct {
	Operation string `json:"operation"`
	Execution string `json:"execution"`
}

type PlaybackResourceDiagnostic struct {
	CPUUsed             int   `json:"cpuUsed"`
	CPUCapacity         int   `json:"cpuCapacity"`
	DiskUsed            int   `json:"diskUsed"`
	DiskCapacity        int   `json:"diskCapacity"`
	NetworkUsed         int   `json:"networkUsed"`
	NetworkCapacity     int   `json:"networkCapacity"`
	BackgroundCPUUsed   int   `json:"backgroundCpuUsed"`
	ReservedDiskBytes   int64 `json:"reservedDiskBytes"`
	ReservedFilesystems int   `json:"reservedFilesystems"`
}

type MediaToolchainDiagnostic struct {
	Source          string                  `json:"source"`
	Status          string                  `json:"status"`
	ReasonCode      string                  `json:"reasonCode"`
	Target          string                  `json:"target"`
	BuildID         string                  `json:"buildId,omitempty"`
	FFmpegVersion   string                  `json:"ffmpegVersion,omitempty"`
	LicenseMode     string                  `json:"licenseMode,omitempty"`
	ManifestPresent bool                    `json:"manifestPresent"`
	Verified        bool                    `json:"verified"`
	Features        []MediaToolchainFeature `json:"features"`
}

type MediaToolchainFeature struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type StartupDiagnostics struct {
	Status               string                   `json:"status"`
	StartedAt            string                   `json:"startedAt"`
	HTTPReady            bool                     `json:"httpReady"`
	HTTPReadyAt          string                   `json:"httpReadyAt,omitempty"`
	HTTPAddr             string                   `json:"httpAddr,omitempty"`
	NonCriticalWorkReady bool                     `json:"nonCriticalWorkReady"`
	Phases               []StartupPhaseDiagnostic `json:"phases"`
}

type StartupPhaseDiagnostic struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Status         string `json:"status"`
	StartedAt      string `json:"startedAt,omitempty"`
	CompletedAt    string `json:"completedAt,omitempty"`
	DurationMillis int64  `json:"durationMillis,omitempty"`
	Error          string `json:"error,omitempty"`
}

type ReadinessResponse struct {
	Status                 string                   `json:"status"`
	Ready                  bool                     `json:"ready"`
	HTTPReady              bool                     `json:"httpReady"`
	WebDistReady           bool                     `json:"webDistReady"`
	AppDataReady           bool                     `json:"appDataReady"`
	DatabaseFileReady      bool                     `json:"databaseFileReady"`
	DatabaseReady          bool                     `json:"databaseReady"`
	BackgroundJobsDeferred bool                     `json:"backgroundJobsDeferred"`
	Startup                StartupDiagnostics       `json:"startup"`
	DatabaseProbe          ReadinessProbe           `json:"databaseProbe"`
	AuthBootstrapProbe     ReadinessProbe           `json:"authBootstrapProbe"`
	SQLite                 ReadinessSQLiteSnapshot  `json:"sqlite"`
	SQLiteHealth           SQLiteHealthDiagnostic   `json:"sqliteHealth"`
	WorkloadLanes          []WorkloadLaneDiagnostic `json:"workloadLanes"`
	Signals                []string                 `json:"signals"`
	GeneratedAt            string                   `json:"generatedAt"`
}

type ReadinessProbe struct {
	Ready          bool   `json:"ready"`
	Status         string `json:"status"`
	DurationMillis int64  `json:"durationMillis"`
	Error          string `json:"error,omitempty"`
	ErrorKind      string `json:"errorKind,omitempty"`
}

type ReadinessSQLiteSnapshot struct {
	Status              string `json:"status"`
	MaxOpenConnections  int    `json:"maxOpenConnections"`
	OpenConnections     int    `json:"openConnections"`
	InUseConnections    int    `json:"inUseConnections"`
	IdleConnections     int    `json:"idleConnections"`
	WaitCount           int64  `json:"waitCount"`
	WaitDurationMillis  int64  `json:"waitDurationMillis"`
	ReadOperations      int64  `json:"readOperations"`
	ReadErrors          int64  `json:"readErrors"`
	WriteOperations     int64  `json:"writeOperations"`
	WriteAttempts       int64  `json:"writeAttempts"`
	LockRetries         int64  `json:"lockRetries"`
	LockRetryWaitMillis int64  `json:"lockRetryWaitMillis"`
	LastRetryAt         string `json:"lastRetryAt,omitempty"`
	LastRetryLane       string `json:"lastRetryLane,omitempty"`
	LastErrorKind       string `json:"lastErrorKind,omitempty"`
	LastErrorAt         string `json:"lastErrorAt,omitempty"`
	SlowestReadMillis   int64  `json:"slowestReadMillis"`
	SlowestReadLane     string `json:"slowestReadLane,omitempty"`
	SlowestWriteMillis  int64  `json:"slowestWriteMillis"`
	SlowestWriteLane    string `json:"slowestWriteLane,omitempty"`
}

type RuntimeDiagnostics struct {
	StartedAt          string                `json:"startedAt"`
	UptimeSeconds      int64                 `json:"uptimeSeconds"`
	Goroutines         int                   `json:"goroutines"`
	HeapAllocBytes     uint64                `json:"heapAllocBytes"`
	HeapSysBytes       uint64                `json:"heapSysBytes"`
	HeapIdleBytes      uint64                `json:"heapIdleBytes"`
	HeapReleasedBytes  uint64                `json:"heapReleasedBytes"`
	StackInUseBytes    uint64                `json:"stackInUseBytes"`
	NextGCBytes        uint64                `json:"nextGcBytes"`
	NumGC              uint32                `json:"numGc"`
	LastGCPauseMillis  uint64                `json:"lastGcPauseMillis"`
	TotalGCPauseMillis uint64                `json:"totalGcPauseMillis"`
	IOPressure         IOPressureDiagnostics `json:"ioPressure"`
}

type IOPressureDiagnostics struct {
	Supported               bool    `json:"supported"`
	Status                  string  `json:"status"`
	Source                  string  `json:"source,omitempty"`
	CurrentSomeAvg10        float64 `json:"currentSomeAvg10"`
	CurrentSomeAvg60        float64 `json:"currentSomeAvg60"`
	CurrentSomeAvg300       float64 `json:"currentSomeAvg300"`
	CurrentFullAvg10        float64 `json:"currentFullAvg10"`
	CurrentFullAvg60        float64 `json:"currentFullAvg60"`
	CurrentFullAvg300       float64 `json:"currentFullAvg300"`
	HighestRecentSomeAvg10  float64 `json:"highestRecentSomeAvg10"`
	HighestRecentSomeAvg60  float64 `json:"highestRecentSomeAvg60"`
	HighestRecentSomeAvg300 float64 `json:"highestRecentSomeAvg300"`
	HighestRecentFullAvg10  float64 `json:"highestRecentFullAvg10"`
	HighestRecentFullAvg60  float64 `json:"highestRecentFullAvg60"`
	HighestRecentFullAvg300 float64 `json:"highestRecentFullAvg300"`
	LastHighPressureAt      string  `json:"lastHighPressureAt,omitempty"`
	Samples                 int     `json:"samples"`
	DiskStatsSupported      bool    `json:"diskStatsSupported"`
	DiskReadOperations      uint64  `json:"diskReadOperations"`
	DiskWriteOperations     uint64  `json:"diskWriteOperations"`
	DiskReadSectors         uint64  `json:"diskReadSectors"`
	DiskWriteSectors        uint64  `json:"diskWriteSectors"`
	DiskIOInProgress        uint64  `json:"diskIoInProgress"`
	Error                   string  `json:"error,omitempty"`
}

type SQLiteDiagnostics struct {
	MaxOpenConnections     int                      `json:"maxOpenConnections"`
	OpenConnections        int                      `json:"openConnections"`
	InUseConnections       int                      `json:"inUseConnections"`
	IdleConnections        int                      `json:"idleConnections"`
	WaitCount              int64                    `json:"waitCount"`
	WaitDurationMillis     int64                    `json:"waitDurationMillis"`
	ReadOperations         int64                    `json:"readOperations"`
	ReadErrors             int64                    `json:"readErrors"`
	WriteOperations        int64                    `json:"writeOperations"`
	WriteAttempts          int64                    `json:"writeAttempts"`
	LockRetries            int64                    `json:"lockRetries"`
	LockRetryWaitMillis    int64                    `json:"lockRetryWaitMillis"`
	DatabaseBytes          int64                    `json:"databaseBytes"`
	WALBytes               int64                    `json:"walBytes"`
	SHMBytes               int64                    `json:"shmBytes"`
	JournalMode            string                   `json:"journalMode"`
	WALAutoCheckpointPages int                      `json:"walAutoCheckpointPages"`
	PageSizeBytes          int64                    `json:"pageSizeBytes"`
	PageCount              int64                    `json:"pageCount"`
	FreePageCount          int64                    `json:"freePageCount"`
	FreePageRatio          float64                  `json:"freePageRatio"`
	ConnectionCachePages   int64                    `json:"connectionCachePages"`
	ConnectionMmapBytes    int64                    `json:"connectionMmapBytes"`
	LastRetryAt            string                   `json:"lastRetryAt,omitempty"`
	LastRetryLane          string                   `json:"lastRetryLane,omitempty"`
	LastRetryWaitMillis    int64                    `json:"lastRetryWaitMillis,omitempty"`
	LastError              string                   `json:"lastError,omitempty"`
	LastErrorKind          string                   `json:"lastErrorKind,omitempty"`
	LastErrorAt            string                   `json:"lastErrorAt,omitempty"`
	SlowestReadMillis      int64                    `json:"slowestReadMillis"`
	SlowestReadLane        string                   `json:"slowestReadLane,omitempty"`
	SlowestWriteMillis     int64                    `json:"slowestWriteMillis"`
	SlowestWriteLane       string                   `json:"slowestWriteLane,omitempty"`
	ReadLatency            []NamedLatencyDiagnostic `json:"readLatency"`
	WriteLatency           []NamedLatencyDiagnostic `json:"writeLatency"`
}

type SQLiteHealthDiagnostic struct {
	Status                  string `json:"status"`
	LastSuccessfulProbeAt   string `json:"lastSuccessfulProbeAt,omitempty"`
	LastFailureAt           string `json:"lastFailureAt,omitempty"`
	LastFailureKind         string `json:"lastFailureKind,omitempty"`
	LastFailureMessage      string `json:"lastFailureMessage,omitempty"`
	ConsecutiveFailures     int    `json:"consecutiveFailures"`
	ConsecutiveSuccesses    int    `json:"consecutiveSuccesses"`
	LastTransitionAt        string `json:"lastTransitionAt,omitempty"`
	LastRecoveryAction      string `json:"lastRecoveryAction,omitempty"`
	LastProbeDurationMillis int64  `json:"lastProbeDurationMillis"`
	EvidenceCaptures        int64  `json:"evidenceCaptures"`
	RecycleAttempts         int64  `json:"recycleAttempts"`
	RecycleSuccesses        int64  `json:"recycleSuccesses"`
	LastRecycleAt           string `json:"lastRecycleAt,omitempty"`
	LastRecycleError        string `json:"lastRecycleError,omitempty"`
	LastRecycleMillis       int64  `json:"lastRecycleMillis,omitempty"`
}

type ResourceDiagnostics struct {
	Status                   string   `json:"status"`
	BackgroundJobsDeferred   bool     `json:"backgroundJobsDeferred"`
	ActivePlaybackSessions   int      `json:"activePlaybackSessions"`
	ActiveTranscodeSessions  int      `json:"activeTranscodeSessions"`
	MaxTranscodeSessions     int      `json:"maxTranscodeSessions"`
	AvailableTranscodeSlots  int      `json:"availableTranscodeSlots"`
	SQLiteInUseConnections   int      `json:"sqliteInUseConnections"`
	SQLiteMaxOpenConnections int      `json:"sqliteMaxOpenConnections"`
	SaturatedWorkloadLanes   []string `json:"saturatedWorkloadLanes"`
	SaturatedJobLanes        []string `json:"saturatedJobLanes"`
	QueuedBackgroundJobs     int      `json:"queuedBackgroundJobs"`
	RunningBackgroundJobs    int      `json:"runningBackgroundJobs"`
	DeferredMaintenanceJobs  int      `json:"deferredMaintenanceJobs"`
	FailedMaintenanceJobs    int      `json:"failedMaintenanceJobs"`
	Signals                  []string `json:"signals"`
	DegradationActions       []string `json:"degradationActions"`
}

type AdmissionDiagnostics struct {
	SearchActive             int    `json:"searchActive"`
	SearchCapacityPerUser    int    `json:"searchCapacityPerUser"`
	SearchCapacityGlobal     int    `json:"searchCapacityGlobal"`
	SearchRejected           uint64 `json:"searchRejected"`
	DownloadActive           int    `json:"downloadActive"`
	DownloadCapacityPerUser  int    `json:"downloadCapacityPerUser"`
	DownloadRejected         uint64 `json:"downloadRejected"`
	StreamActive             int    `json:"streamActive"`
	StreamCapacityPerUser    int    `json:"streamCapacityPerUser"`
	StreamRejected           uint64 `json:"streamRejected"`
	TranscodeActive          int    `json:"transcodeActive"`
	TranscodeCapacity        int    `json:"transcodeCapacity"`
	TranscodeCapacityPerUser int    `json:"transcodeCapacityPerUser"`
	TranscodeRejected        uint64 `json:"transcodeRejected"`
	TranscodeUserRejected    uint64 `json:"transcodeUserRejected"`
	GeneratedAt              string `json:"generatedAt"`
}

type JobLaneDiagnostic struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Active   int    `json:"active"`
	Capacity int    `json:"capacity"`
	Queued   int    `json:"queued"`
	Running  int    `json:"running"`
}

type WorkloadLaneDiagnostic struct {
	ID                     string            `json:"id"`
	Label                  string            `json:"label"`
	Active                 int               `json:"active"`
	Capacity               int               `json:"capacity"`
	Rejected               uint64            `json:"rejected"`
	Queued                 uint64            `json:"queued"`
	AverageQueueWaitMillis int64             `json:"averageQueueWaitMillis"`
	QueueWaitLatency       LatencyDiagnostic `json:"queueWaitLatency"`
	ServiceLatency         LatencyDiagnostic `json:"serviceLatency"`
}

type SystemTimeSync struct {
	RequestReceivedAt   string `json:"requestReceivedAt"`
	ResponseSentAt      string `json:"responseSentAt"`
	ServerUnixMillis    int64  `json:"serverUnixMillis"`
	ServerMonotonicHint string `json:"serverMonotonicHint,omitempty"`
}

type SystemReleaseInfo struct {
	Version         string `json:"version"`
	APIVersion      string `json:"apiVersion"`
	GOOS            string `json:"goos"`
	GOARCH          string `json:"goarch"`
	DatabaseReady   bool   `json:"databaseReady"`
	WebDistReady    bool   `json:"webDistReady"`
	AppDataReady    bool   `json:"appDataReady"`
	MigrationStatus string `json:"migrationStatus"`
	InstallMethod   string `json:"installMethod"`
	UpdateStatus    string `json:"updateStatus"`
	GeneratedAt     string `json:"generatedAt"`
}

type SystemStorageReport struct {
	TotalBytes  int64                   `json:"totalBytes"`
	Categories  []SystemStorageCategory `json:"categories"`
	GeneratedAt string                  `json:"generatedAt"`
}

type SystemStorageCategory struct {
	Key              string `json:"key"`
	Label            string `json:"label"`
	SizeBytes        int64  `json:"sizeBytes"`
	FileCount        int    `json:"fileCount"`
	Available        bool   `json:"available"`
	Writable         bool   `json:"writable"`
	CleanupSupported bool   `json:"cleanupSupported"`
	Error            string `json:"error,omitempty"`
}

type SystemStorageCleanupResponse struct {
	OK       bool `json:"ok"`
	Queued   bool `json:"queued"`
	Job      Job  `json:"job,omitempty"`
	Existing bool `json:"existing,omitempty"`
}

type RuntimeDependency struct {
	Name           string   `json:"name"`
	ConfiguredPath string   `json:"configuredPath"`
	ResolvedPath   string   `json:"resolvedPath,omitempty"`
	Available      bool     `json:"available"`
	VersionLine    string   `json:"versionLine,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type TranscodeCapacityReport struct {
	Enabled                  bool                  `json:"enabled"`
	MaxConcurrentSessions    int                   `json:"maxConcurrentSessions"`
	ActiveSessions           int                   `json:"activeSessions"`
	AvailableSlots           int                   `json:"availableSlots"`
	TemporaryDirectory       string                `json:"temporaryDirectory"`
	TemporaryDirectoryReady  bool                  `json:"temporaryDirectoryReady"`
	X264Preset               string                `json:"x264Preset"`
	ThrottleBufferSeconds    int                   `json:"throttleBufferSeconds"`
	PlayedRetentionSeconds   int                   `json:"playedRetentionSeconds"`
	HardwareAcceleration     bool                  `json:"hardwareAcceleration"`
	HardwareEncoding         bool                  `json:"hardwareEncoding"`
	HardwareDevice           string                `json:"hardwareDevice"`
	HardwareDecodeValue      string                `json:"hardwareDecodeValue"`
	HardwareEncoder          string                `json:"hardwareEncoder,omitempty"`
	HardwareEncoderAvailable bool                  `json:"hardwareEncoderAvailable"`
	HardwareSupportLevel     string                `json:"hardwareSupportLevel"`
	HardwareProbes           []TranscodeProbe      `json:"hardwareProbes"`
	MaxHardwareSessions      int                   `json:"maxHardwareSessions"`
	MaxSoftwareSessions      int                   `json:"maxSoftwareSessions"`
	MaxBackgroundSessions    int                   `json:"maxBackgroundSessions"`
	HDRToneMapping           bool                  `json:"hdrToneMapping"`
	HDRToneMappingAlgorithm  string                `json:"hdrToneMappingAlgorithm"`
	HDRToneMappingAvailable  bool                  `json:"hdrToneMappingAvailable"`
	HDRToneMappingStatus     string                `json:"hdrToneMappingStatus"`
	HDRToneMappingDetail     string                `json:"hdrToneMappingDetail"`
	DirectStreamRemux        bool                  `json:"directStreamRemux"`
	FFmpeg                   RuntimeDependency     `json:"ffmpeg"`
	FFprobe                  RuntimeDependency     `json:"ffprobe"`
	Presets                  []TranscodePresetInfo `json:"presets"`
	Warnings                 []string              `json:"warnings"`
	GeneratedAt              string                `json:"generatedAt"`
}

type TranscodeProbe struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type TranscodePresetInfo struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Height         int    `json:"height"`
	VideoKbps      int    `json:"videoKbps"`
	AudioKbps      int    `json:"audioKbps"`
	CRF            int    `json:"crf"`
	MaxWidth       int    `json:"maxWidth"`
	RequiresFFmpeg bool   `json:"requiresFfmpeg"`
}

type RestoreBackupResponse struct {
	OK               bool   `json:"ok"`
	Name             string `json:"name"`
	OperationID      string `json:"operationId"`
	SourceKind       string `json:"sourceKind,omitempty"`
	ManifestVerified bool   `json:"manifestVerified"`
	MaxDatabaseBytes int64  `json:"maxDatabaseBytes,omitempty"`
	RecoveryRequired bool   `json:"recoveryRequired"`
	State            string `json:"state"`
	Phase            string `json:"phase,omitempty"`
	Progress         int    `json:"progress,omitempty"`
	ValidationCode   string `json:"validationCode,omitempty"`
	ErrorCode        string `json:"errorCode,omitempty"`
	ErrorMessage     string `json:"errorMessage,omitempty"`
	WarningCode      string `json:"warningCode,omitempty"`
	WarningMessage   string `json:"warningMessage,omitempty"`
	Instruction      string `json:"instruction"`
	StatusToken      string `json:"statusToken,omitempty"`
}

type Playlist struct {
	ID          string          `json:"id"`
	UserID      string          `json:"userId"`
	ProfileID   string          `json:"-"`
	Kind        string          `json:"kind"`
	Title       string          `json:"title"`
	Summary     string          `json:"summary,omitempty"`
	Visibility  string          `json:"visibility"`
	CanEdit     bool            `json:"canEdit"`
	Smart       bool            `json:"smart"`
	SmartFilter SmartFilter     `json:"smartFilter"`
	ItemCount   int             `json:"itemCount"`
	Shares      []PlaylistShare `json:"shares,omitempty"`
	Items       []MediaItem     `json:"items,omitempty"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
}

type PlaylistShare struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	CanEdit     bool   `json:"canEdit"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

type PlaylistShareRequest struct {
	UserID  string `json:"userId"`
	CanEdit bool   `json:"canEdit"`
}

type PlaylistRequest struct {
	Kind        string                  `json:"kind"`
	Title       string                  `json:"title"`
	Summary     string                  `json:"summary"`
	Visibility  string                  `json:"visibility"`
	SmartFilter SmartFilter             `json:"smartFilter"`
	Shares      *[]PlaylistShareRequest `json:"shares,omitempty"`
	MediaIDs    []string                `json:"mediaIds,omitempty"`
}

type PlaylistItemRequest struct {
	MediaID string `json:"mediaId"`
}

type PlaylistItemOrderRequest struct {
	MediaIDs []string `json:"mediaIds"`
}

type PlaylistBulkItemsResponse struct {
	Playlist Playlist `json:"playlist"`
	Added    int      `json:"added"`
	Skipped  int      `json:"skipped"`
	Total    int      `json:"total"`
}

type SmartFilter struct {
	Enabled       bool   `json:"enabled"`
	LibraryID     string `json:"libraryId,omitempty"`
	Type          string `json:"type,omitempty"`
	Genre         string `json:"genre,omitempty"`
	Studio        string `json:"studio,omitempty"`
	YearMin       int    `json:"yearMin,omitempty"`
	YearMax       int    `json:"yearMax,omitempty"`
	UnwatchedOnly bool   `json:"unwatchedOnly,omitempty"`
	Sort          string `json:"sort,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type DVRRecordingRule struct {
	ID                     string   `json:"id"`
	UserID                 string   `json:"-"`
	ProfileID              string   `json:"profileId"`
	SourceID               string   `json:"sourceId"`
	ChannelID              string   `json:"channelId,omitempty"`
	ProgramID              string   `json:"programId,omitempty"`
	Title                  string   `json:"title"`
	MatchType              string   `json:"matchType"`
	Folder                 string   `json:"folder,omitempty"`
	StartPaddingMinutes    int      `json:"startPaddingMinutes"`
	EndPaddingMinutes      int      `json:"endPaddingMinutes"`
	RetentionDays          int      `json:"retentionDays"`
	MaxRecordingsPerSeries int      `json:"maxRecordingsPerSeries"`
	RequiredKeywords       []string `json:"requiredKeywords,omitempty"`
	BlockedKeywords        []string `json:"blockedKeywords,omitempty"`
	AllowedChannels        []string `json:"allowedChannels,omitempty"`
	BlockedChannels        []string `json:"blockedChannels,omitempty"`
	Enabled                bool     `json:"enabled"`
	Priority               int      `json:"priority"`
	Revision               int64    `json:"revision"`
	CreatedAt              string   `json:"createdAt"`
	UpdatedAt              string   `json:"updatedAt"`
	Actions                []string `json:"actions"`
}

type DVRRecording struct {
	ID                 string   `json:"id"`
	RuleID             string   `json:"ruleId,omitempty"`
	UserID             string   `json:"-"`
	ProfileID          string   `json:"-"`
	SourceID           string   `json:"sourceId"`
	ChannelID          string   `json:"channelId,omitempty"`
	ProgramID          string   `json:"programId,omitempty"`
	Title              string   `json:"title"`
	Folder             string   `json:"-"`
	Status             string   `json:"status"`
	StartsAt           string   `json:"startsAt"`
	EndsAt             string   `json:"endsAt"`
	Path               string   `json:"-"`
	SizeBytes          int64    `json:"sizeBytes"`
	Error              string   `json:"-"`
	FailureCode        string   `json:"failureCode,omitempty"`
	FailureMessageID   string   `json:"failureMessageId,omitempty"`
	Priority           int      `json:"priority"`
	Revision           int64    `json:"revision"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
	Actions            []string `json:"actions"`
	AllocationCapacity int      `json:"-"`
	AllocationDemand   int      `json:"-"`
}

type DVRPlaybackSessionCreateRequest struct {
	ClientInstanceID string                `json:"clientInstanceId,omitempty"`
	ClientProfile    PlaybackClientProfile `json:"clientProfile,omitempty"`
	Intent           PlaybackIntent        `json:"intent,omitempty"`
	VersionID        string                `json:"versionId,omitempty"`
	AudioStreamID    string                `json:"audioStreamId,omitempty"`
	SubtitleStreamID string                `json:"subtitleStreamId,omitempty"`
	BurnInSubtitleID string                `json:"burnInSubtitleId,omitempty"`
	StartSeconds     int                   `json:"startSeconds,omitempty"`
}

type DVRRecordingGroup struct {
	ID                string         `json:"id"`
	Title             string         `json:"title"`
	Folder            string         `json:"-"`
	CursorFolder      string         `json:"-"`
	Count             int            `json:"count"`
	SizeBytes         int64          `json:"sizeBytes"`
	LatestRecordingAt string         `json:"latestRecordingAt"`
	Recordings        []DVRRecording `json:"recordings"`
}

type DVRRecordingRuleRequest struct {
	SourceID               string   `json:"sourceId"`
	ChannelID              string   `json:"channelId"`
	ProgramID              string   `json:"programId"`
	Title                  string   `json:"title"`
	MatchType              string   `json:"matchType"`
	Folder                 string   `json:"folder"`
	StartPaddingMinutes    int      `json:"startPaddingMinutes"`
	EndPaddingMinutes      int      `json:"endPaddingMinutes"`
	RetentionDays          int      `json:"retentionDays"`
	MaxRecordingsPerSeries *int     `json:"maxRecordingsPerSeries,omitempty"`
	RequiredKeywords       []string `json:"requiredKeywords,omitempty"`
	BlockedKeywords        []string `json:"blockedKeywords,omitempty"`
	AllowedChannels        []string `json:"allowedChannels,omitempty"`
	BlockedChannels        []string `json:"blockedChannels,omitempty"`
	Enabled                bool     `json:"enabled"`
	Priority               *int     `json:"priority,omitempty"`
	ExpectedRevision       *int64   `json:"expectedRevision,omitempty"`
}

type DVRRecordingRulePatchRequest struct {
	SourceID               *string   `json:"sourceId,omitempty"`
	ChannelID              *string   `json:"channelId,omitempty"`
	ProgramID              *string   `json:"programId,omitempty"`
	Title                  *string   `json:"title,omitempty"`
	MatchType              *string   `json:"matchType,omitempty"`
	Folder                 *string   `json:"folder,omitempty"`
	StartPaddingMinutes    *int      `json:"startPaddingMinutes,omitempty"`
	EndPaddingMinutes      *int      `json:"endPaddingMinutes,omitempty"`
	RetentionDays          *int      `json:"retentionDays,omitempty"`
	MaxRecordingsPerSeries *int      `json:"maxRecordingsPerSeries,omitempty"`
	RequiredKeywords       *[]string `json:"requiredKeywords,omitempty"`
	BlockedKeywords        *[]string `json:"blockedKeywords,omitempty"`
	AllowedChannels        *[]string `json:"allowedChannels,omitempty"`
	BlockedChannels        *[]string `json:"blockedChannels,omitempty"`
	Enabled                *bool     `json:"enabled,omitempty"`
	Priority               *int      `json:"priority,omitempty"`
	ExpectedRevision       *int64    `json:"expectedRevision,omitempty"`
}

type DVRRecordingRequest struct {
	RuleID           string `json:"ruleId"`
	SourceID         string `json:"sourceId"`
	ChannelID        string `json:"channelId"`
	ProgramID        string `json:"programId"`
	Title            string `json:"title"`
	Folder           string `json:"folder"`
	StartsAt         string `json:"startsAt"`
	EndsAt           string `json:"endsAt"`
	Priority         *int   `json:"priority,omitempty"`
	ExpectedRevision *int64 `json:"expectedRevision,omitempty"`
}

type DVRConflict struct {
	ID           string   `json:"id"`
	RecordingIDs []string `json:"recordingIds"`
	StartsAt     string   `json:"startsAt"`
	EndsAt       string   `json:"endsAt"`
	Reason       string   `json:"reason"`
	Capacity     int      `json:"capacity"`
	Demand       int      `json:"demand"`
	MessageID    string   `json:"messageId"`
	Actions      []string `json:"actions"`
}

type DVRTunerAllocation struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	ChannelID   string `json:"channelId,omitempty"`
	RecordingID string `json:"recordingId,omitempty"`
}

type DVROperationalCapabilities struct {
	CanScheduleRecordings   bool     `json:"canScheduleRecordings"`
	CanManageRecordingRules bool     `json:"canManageRecordingRules"`
	CanCreateOwnRules       bool     `json:"canCreateOwnRules"`
	CanEditOwnRules         bool     `json:"canEditOwnRules"`
	CanDeleteOwnRules       bool     `json:"canDeleteOwnRules"`
	CanManageAllRules       bool     `json:"canManageAllRules"`
	Actions                 []string `json:"actions"`
}

type DVRGuideOperationalStatus struct {
	State           string `json:"state"`
	LastRefreshedAt string `json:"lastRefreshedAt,omitempty"`
	MessageID       string `json:"messageId,omitempty"`
}

type DVRStorageOperationalStatus struct {
	UsedBytes      int64  `json:"usedBytes"`
	AvailableBytes int64  `json:"availableBytes"`
	ForecastDays   int    `json:"forecastDays,omitempty"`
	State          string `json:"state"`
}

type DVROperationalStatus struct {
	Configured   bool                        `json:"configured"`
	Available    bool                        `json:"available"`
	Capabilities DVROperationalCapabilities  `json:"capabilities"`
	Guide        DVRGuideOperationalStatus   `json:"guide"`
	Conflicts    []DVRConflict               `json:"conflicts"`
	Tuners       []DVRTunerAllocation        `json:"tuners"`
	Storage      DVRStorageOperationalStatus `json:"storage"`
	GeneratedAt  string                      `json:"generatedAt"`
}

type DVRConsumerStatus struct {
	Capabilities DVROperationalCapabilities  `json:"capabilities"`
	Conflicts    []DVRConflict               `json:"conflicts"`
	Storage      DVRStorageOperationalStatus `json:"storage"`
	GeneratedAt  string                      `json:"generatedAt"`
}

type Job struct {
	ID                      string            `json:"id"`
	Type                    string            `json:"type"`
	Status                  string            `json:"status"`
	Progress                int               `json:"progress"`
	Message                 string            `json:"message"`
	ResourceType            string            `json:"resourceType,omitempty"`
	ResourceID              string            `json:"resourceId,omitempty"`
	Metadata                map[string]string `json:"metadata,omitempty"`
	ParentOperationID       string            `json:"parentOperationId,omitempty"`
	IdempotencyKey          string            `json:"idempotencyKey,omitempty"`
	Priority                string            `json:"priority,omitempty"`
	Phase                   string            `json:"phase,omitempty"`
	ProgressCurrent         int               `json:"progressCurrent,omitempty"`
	ProgressTotal           int               `json:"progressTotal,omitempty"`
	ResultReference         string            `json:"resultReference,omitempty"`
	AttemptCount            int               `json:"attemptCount,omitempty"`
	NextRunAt               string            `json:"nextRunAt,omitempty"`
	LastError               string            `json:"lastError,omitempty"`
	ErrorCode               string            `json:"errorCode,omitempty"`
	FailureKind             string            `json:"failureKind,omitempty"`
	RetryEligible           bool              `json:"retryEligible,omitempty"`
	CancellationRequestedAt string            `json:"cancellationRequestedAt,omitempty"`
	WorkerAcknowledgedAt    string            `json:"workerAcknowledgedAt,omitempty"`
	InterruptedAt           string            `json:"interruptedAt,omitempty"`
	RetentionUntil          string            `json:"retentionUntil,omitempty"`
	ActiveKey               string            `json:"-"`
	CreatedAt               string            `json:"createdAt"`
	UpdatedAt               string            `json:"updatedAt"`
}

type JobCancelResponse struct {
	Items     []Job  `json:"items"`
	Total     int    `json:"total"`
	Cancelled int    `json:"cancelled"`
	Type      string `json:"type,omitempty"`
}

type ScheduledTask struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Category    string               `json:"category"`
	Description string               `json:"description"`
	Enabled     bool                 `json:"enabled"`
	JobType     string               `json:"jobType"`
	Schedule    string               `json:"schedule"`
	Timezone    string               `json:"timezone,omitempty"`
	Trigger     ScheduledTaskTrigger `json:"trigger"`
	LastJob     *Job                 `json:"lastJob,omitempty"`
	Running     bool                 `json:"running"`
}

type ScheduledTaskTrigger struct {
	Enabled       bool `json:"enabled"`
	IntervalHours int  `json:"intervalHours"`
}

type ScheduledTaskUpdateRequest struct {
	Enabled       *bool `json:"enabled,omitempty"`
	IntervalHours *int  `json:"intervalHours,omitempty"`
}

type ScheduledTaskRunResponse struct {
	TaskID string `json:"taskId"`
	Jobs   []Job  `json:"jobs"`
}

type LogEvent struct {
	ID      string            `json:"id"`
	Time    string            `json:"time"`
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type ClientLogEntry struct {
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Context   map[string]string `json:"context,omitempty"`
	Timestamp string            `json:"timestamp,omitempty"`
}

type ClientLogUploadRequest struct {
	Device  string           `json:"device,omitempty"`
	App     string           `json:"app,omitempty"`
	Entries []ClientLogEntry `json:"entries"`
}

type ClientLogUploadResponse struct {
	OK       bool `json:"ok"`
	Accepted int  `json:"accepted"`
}

type DashboardResponse struct {
	NowPlaying   []PlaybackSession     `json:"nowPlaying"`
	Metrics      []DashboardMetric     `json:"metrics"`
	Bandwidth    []DashboardSample     `json:"bandwidth"`
	System       DashboardSystem       `json:"system"`
	WatchedToday DashboardWatchedToday `json:"watchedToday"`
	TopUsers     []DashboardTopUser    `json:"topUsers"`
	PlayHistory  []PlayHistoryPoint    `json:"playHistory"`
	TopPlayed    []TopPlayedGroup      `json:"topPlayed"`
	Transcodes   []TranscodeSession    `json:"transcodes"`
	Alerts       []DashboardNotice     `json:"alerts"`
	Conversions  []ConversionJob       `json:"conversions"`
	Libraries    []LibraryStat         `json:"libraries"`
	Jobs         []Job                 `json:"jobs"`
	Mode         string                `json:"mode"`
	Period       string                `json:"period"`
	UserID       string                `json:"userId,omitempty"`
	GeneratedAt  string                `json:"generatedAt"`
}

type DashboardWatchedToday struct {
	DurationSeconds  int `json:"durationSeconds"`
	Sessions         int `json:"sessions"`
	Users            int `json:"users"`
	MoviesSeconds    int `json:"moviesSeconds"`
	TVSeconds        int `json:"tvSeconds"`
	MusicSeconds     int `json:"musicSeconds"`
	AudiobookSeconds int `json:"audiobookSeconds"`
	LiveTVSeconds    int `json:"liveTvSeconds"`
}

type ServerActivityResponse struct {
	ServerName       string                `json:"serverName"`
	ActiveStreams    int                   `json:"activeStreams"`
	ActiveTranscodes int                   `json:"activeTranscodes"`
	CPUPercent       float64               `json:"cpuPercent"`
	CPUStatus        TelemetryMetricStatus `json:"cpuStatus"`
	MemoryUsedBytes  int64                 `json:"memoryUsedBytes"`
	MemoryFreeBytes  int64                 `json:"memoryFreeBytes"`
	MemoryTotalBytes int64                 `json:"memoryTotalBytes"`
	MemoryStatus     TelemetryMetricStatus `json:"memoryStatus"`
	BandwidthMbps    float64               `json:"bandwidthMbps"`
	GeneratedAt      string                `json:"generatedAt"`
	RefreshAfterMs   int                   `json:"refreshAfterMs"`
}

// TelemetryMetricStatus accompanies numeric telemetry values. A value is only
// authoritative when Status is "ok"; unavailable values remain zero in the
// backwards-compatible numeric fields and carry a reason here instead.
type TelemetryMetricStatus struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type DashboardOverviewUsageResponse struct {
	TopUsers    []DashboardTopUser `json:"topUsers"`
	PlayHistory []PlayHistoryPoint `json:"playHistory"`
	TopPlayed   []TopPlayedGroup   `json:"topPlayed"`
	GeneratedAt string             `json:"generatedAt"`
}

type PlaybackSession struct {
	ID               string              `json:"id"`
	User             string              `json:"user"`
	UserID           string              `json:"userId"`
	ClientInstanceID string              `json:"clientInstanceId,omitempty"`
	Device           string              `json:"device"`
	App              string              `json:"app"`
	Location         string              `json:"location"`
	ClientIP         string              `json:"clientIp"`
	State            string              `json:"state"`
	Progress         int                 `json:"progress"`
	PositionSeconds  int                 `json:"positionSeconds"`
	BandwidthMbps    float64             `json:"bandwidthMbps"`
	Decision         string              `json:"decision"`
	VideoDecision    string              `json:"videoDecision"`
	VideoSource      string              `json:"videoSource"`
	VideoTarget      string              `json:"videoTarget"`
	AudioDecision    string              `json:"audioDecision"`
	AudioSource      string              `json:"audioSource"`
	AudioTarget      string              `json:"audioTarget"`
	SubtitleDecision string              `json:"subtitleDecision"`
	Media            MediaItem           `json:"media"`
	StartedAt        string              `json:"startedAt"`
	LastSeenAt       string              `json:"lastSeenAt"`
	EndedAt          string              `json:"endedAt,omitempty"`
	IsLive           bool                `json:"isLive"`
	Command          PlaybackCommand     `json:"command"`
	Diagnostics      PlaybackDiagnostics `json:"diagnostics,omitempty"`
	Transcode        *TranscodeSession   `json:"transcode,omitempty"`
}

type PlaybackDiagnostics struct {
	ClientProfile        string             `json:"clientProfile,omitempty"`
	MaxBitrateMbps       int                `json:"maxBitrateMbps,omitempty"`
	DecisionReason       string             `json:"decisionReason,omitempty"`
	Protocol             string             `json:"protocol,omitempty"`
	Container            string             `json:"container,omitempty"`
	VideoCodec           string             `json:"videoCodec,omitempty"`
	AudioCodec           string             `json:"audioCodec,omitempty"`
	SubtitleBurnIn       bool               `json:"subtitleBurnIn,omitempty"`
	SubtitleBurnInReason string             `json:"subtitleBurnInReason,omitempty"`
	TranscodeQuality     string             `json:"transcodeQuality,omitempty"`
	TranscodeMethod      string             `json:"transcodeMethod,omitempty"`
	TranscodeFilter      string             `json:"transcodeFilter,omitempty"`
	FFmpegContext        string             `json:"ffmpegContext,omitempty"`
	FFmpeg               *FFmpegDiagnostics `json:"ffmpeg,omitempty"`
	ClientFallbackReason string             `json:"clientFallbackReason,omitempty"`
}

// FFmpegDiagnostics is bounded, redacted process evidence intended for
// owner-facing diagnostics. Stderr contains only a head/tail excerpt; the
// byte and line totals describe the complete stream without retaining it.
type FFmpegDiagnostics struct {
	CommandIdentity string `json:"commandIdentity,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	StderrBytes     int64  `json:"stderrBytes,omitempty"`
	StderrLines     int64  `json:"stderrLines,omitempty"`
	StderrTruncated bool   `json:"stderrTruncated,omitempty"`
	ErrorLines      int64  `json:"errorLines,omitempty"`
	ProgressLines   int64  `json:"progressLines,omitempty"`
	ExitCode        int    `json:"exitCode,omitempty"`
	Signal          string `json:"signal,omitempty"`
	DurationMillis  int64  `json:"durationMillis,omitempty"`
}

type DashboardMetric struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail"`
	Trend  string `json:"trend"`
}

type DashboardSample struct {
	Label     string `json:"label"`
	Local     int    `json:"local"`
	Remote    int    `json:"remote"`
	Transcode int    `json:"transcode"`
}

type DashboardSystem struct {
	CPU     []DashboardSystemSample `json:"cpu"`
	RAM     []DashboardSystemSample `json:"ram"`
	GPU     []DashboardGPUSample    `json:"gpu"`
	DiskIO  []DashboardDiskIOSample `json:"diskIo"`
	GPUInfo DashboardGPUInfo        `json:"gpuInfo"`
}

type DashboardSystemSample struct {
	Label        string                `json:"label"`
	Server       float64               `json:"server"`
	ServerStatus TelemetryMetricStatus `json:"serverStatus"`
	System       float64               `json:"system"`
	SystemStatus TelemetryMetricStatus `json:"systemStatus"`
}

type DashboardGPUSample struct {
	Label          string                `json:"label"`
	Usage          float64               `json:"usage"`
	UsageStatus    TelemetryMetricStatus `json:"usageStatus"`
	Memory         float64               `json:"memory"`
	MemoryStatus   TelemetryMetricStatus `json:"memoryStatus"`
	Encoder        float64               `json:"encoder"`
	EncoderStatus  TelemetryMetricStatus `json:"encoderStatus"`
	Headroom       float64               `json:"headroom"`
	HeadroomStatus TelemetryMetricStatus `json:"headroomStatus"`
}

type DashboardDiskIOSample struct {
	Label                   string  `json:"label"`
	ReadMegabytesPerSecond  float64 `json:"readMegabytesPerSecond"`
	WriteMegabytesPerSecond float64 `json:"writeMegabytesPerSecond"`
	OperationsPerSecond     float64 `json:"operationsPerSecond"`
	IOInProgress            uint64  `json:"ioInProgress"`
	Supported               bool    `json:"supported"`
}

type DashboardGPUInfo struct {
	Provider  string `json:"provider"`
	Device    string `json:"device"`
	Available bool   `json:"available"`
	Note      string `json:"note,omitempty"`
}

type DashboardTopUser struct {
	UserID          string                    `json:"userId"`
	Name            string                    `json:"name"`
	Role            string                    `json:"role"`
	Plays           int                       `json:"plays"`
	DurationSeconds int                       `json:"durationSeconds"`
	Libraries       []DashboardTopUserLibrary `json:"libraries"`
}

type DashboardTopUserLibrary struct {
	Name            string `json:"name"`
	DurationSeconds int    `json:"durationSeconds"`
	Plays           int    `json:"plays"`
}

type PlayHistoryPoint struct {
	Label            string `json:"label"`
	MoviesSeconds    int    `json:"moviesSeconds"`
	TVSeconds        int    `json:"tvSeconds"`
	MusicSeconds     int    `json:"musicSeconds"`
	AudiobookSeconds int    `json:"audiobookSeconds"`
	LiveTVSeconds    int    `json:"liveTvSeconds"`
}

type TopPlayedGroup struct {
	Type  string          `json:"type"`
	Items []TopPlayedItem `json:"items"`
}

type TopPlayedItem struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Type    string   `json:"type"`
	Plays   int      `json:"plays"`
	Users   int      `json:"users"`
	Seconds int      `json:"seconds"`
	Images  ImageSet `json:"images"`
}

type TranscodeSession struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Source          string  `json:"source"`
	Target          string  `json:"target"`
	Speed           string  `json:"speed"`
	SpeedMultiplier float64 `json:"speedMultiplier,omitempty"`
	Progress        int     `json:"progress"`
	Device          string  `json:"device"`
	Quality         string  `json:"quality,omitempty"`
	Method          string  `json:"method,omitempty"`
	Filter          string  `json:"filter,omitempty"`
	StartedAt       string  `json:"startedAt,omitempty"`
	BufferSeconds   int     `json:"bufferSeconds,omitempty"`
	SegmentCount    int     `json:"segmentCount,omitempty"`
}

type DashboardNotice struct {
	ID      string `json:"id"`
	Level   string `json:"level"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

type SettingsSummaryResponse struct {
	Groups      []SettingsGroupSummary `json:"groups"`
	StatusCards []DashboardNotice      `json:"statusCards"`
	GeneratedAt string                 `json:"generatedAt"`
}

type SettingsDocument struct {
	Revision              string              `json:"revision"`
	UpdatedAt             string              `json:"updatedAt"`
	Groups                map[string]any      `json:"groups"`
	RestartRequired       bool                `json:"restartRequired"`
	RestartRequiredFields []string            `json:"restartRequiredFields"`
	ApplyImpact           SettingsApplyImpact `json:"applyImpact"`
}

type SettingsApplyImpact struct {
	ChangedFields         []string `json:"changedFields"`
	RestartRequired       bool     `json:"restartRequired"`
	RestartRequiredFields []string `json:"restartRequiredFields"`
}

type SettingsGroupSummary struct {
	ID                        string `json:"id"`
	Label                     string `json:"label"`
	Category                  string `json:"category"`
	Summary                   string `json:"summary"`
	Implemented               bool   `json:"implemented"`
	ReadOnly                  bool   `json:"readOnly"`
	RequiresAdmin             bool   `json:"requiresAdmin"`
	RequiresPorticoClaim      bool   `json:"requiresPorticoClaim"`
	RequiresRuntimeDependency bool   `json:"requiresRuntimeDependency"`
	Dangerous                 bool   `json:"dangerous"`
	Configured                bool   `json:"configured"`
	Status                    string `json:"status"`
}

type ConversionJob struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Profile  string `json:"profile"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
}

type LibraryStat struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Count     int    `json:"count"`
	ScannedAt string `json:"scannedAt"`
}

type PlaybackSessionCreateRequest struct {
	MediaID          string                `json:"mediaId"`
	VersionID        string                `json:"versionId,omitempty"`
	ClientInstanceID string                `json:"clientInstanceId,omitempty"`
	ClientProfile    PlaybackClientProfile `json:"clientProfile,omitempty"`
	Intent           PlaybackIntent        `json:"intent,omitempty"`
	SkipPreroll      bool                  `json:"skipPreroll,omitempty"`
	BurnInSubtitleID string                `json:"burnInSubtitleId,omitempty"`
	SubtitleStreamID string                `json:"subtitleStreamId,omitempty"`
	AudioStreamID    string                `json:"audioStreamId,omitempty"`
	StartSeconds     int                   `json:"startSeconds,omitempty"`
	QueueMediaIDs    []string              `json:"queueMediaIds,omitempty"`
	RepeatMode       string                `json:"repeatMode,omitempty"`
	SourceContext    PlaybackSourceContext `json:"sourceContext,omitempty"`
}

type PlaybackRestoreRequest struct {
	ClientInstanceID string                `json:"clientInstanceId,omitempty"`
	ClientProfile    PlaybackClientProfile `json:"clientProfile,omitempty"`
	Intent           PlaybackIntent        `json:"intent,omitempty"`
}

type PlaybackRestoreResponse struct {
	Active   bool              `json:"active"`
	Playback *PlaybackResponse `json:"playback,omitempty"`
}

type PlaybackNextRequest struct {
	MediaID       string   `json:"mediaId"`
	QueueMediaIDs []string `json:"queueMediaIds,omitempty"`
}

type PlaybackNextResponse struct {
	Item   *MediaItem  `json:"item,omitempty"`
	Queue  []MediaItem `json:"queue"`
	Reason string      `json:"reason"`
}

type PlaybackQueueResponse struct {
	Items []MediaItem `json:"items"`
	Total int         `json:"total"`
}

type PlaybackSessionQueueReplaceRequest struct {
	ExpectedRevision *int64   `json:"expectedRevision"`
	MediaIDs         []string `json:"mediaIds"`
	RepeatMode       string   `json:"repeatMode"`
}

type PlaybackSessionQueueRequest struct {
	ExpectedRevision *int64   `json:"expectedRevision"`
	Action           string   `json:"action"`
	MediaID          string   `json:"mediaId,omitempty"`
	MediaIDs         []string `json:"mediaIds,omitempty"`
	Index            *int     `json:"index,omitempty"`
	FromIndex        *int     `json:"fromIndex,omitempty"`
	ToIndex          *int     `json:"toIndex,omitempty"`
	RepeatMode       string   `json:"repeatMode,omitempty"`
}

type PlaybackSessionQueueResponse struct {
	SessionID     string                `json:"sessionId"`
	Current       MediaItem             `json:"current"`
	Items         []MediaItem           `json:"items"`
	History       []MediaItem           `json:"history"`
	Total         int                   `json:"total"`
	CanMutate     bool                  `json:"canMutate"`
	RepeatMode    string                `json:"repeatMode"`
	Revision      int64                 `json:"revision"`
	SourceContext PlaybackSourceContext `json:"sourceContext,omitempty"`
}

type PlaybackPrepareNextRequest struct {
	MediaID           string                `json:"mediaId,omitempty"`
	QueueMediaIDs     []string              `json:"queueMediaIds,omitempty"`
	ClientProfile     PlaybackClientProfile `json:"clientProfile,omitempty"`
	Intent            PlaybackIntent        `json:"intent,omitempty"`
	CrossfadeSeconds  int                   `json:"crossfadeSeconds,omitempty"`
	PreferredHandoff  string                `json:"preferredHandoff,omitempty"`
	CommitPreviousEnd bool                  `json:"commitPreviousEnd,omitempty"`
	SourceContext     PlaybackSourceContext `json:"sourceContext,omitempty"`
}

type PlaybackPreparedResponse struct {
	PreparedSessionID string           `json:"preparedSessionId"`
	Playback          PlaybackResponse `json:"playback"`
	ExpiresAt         string           `json:"expiresAt"`
	PreloadPolicy     string           `json:"preloadPolicy"`
	HandoffMode       string           `json:"handoffMode"`
	Queue             []MediaItem      `json:"queue"`
	QueueRevision     int64            `json:"queueRevision"`
	PlaybackRevision  int64            `json:"playbackRevision"`
}

type PlaybackHandoffRequest struct {
	PreparedSessionID        string                `json:"preparedSessionId,omitempty"`
	RequestID                string                `json:"requestId,omitempty"`
	MediaID                  string                `json:"mediaId,omitempty"`
	ClientProfile            PlaybackClientProfile `json:"clientProfile,omitempty"`
	Intent                   PlaybackIntent        `json:"intent,omitempty"`
	QueueMediaIDs            []string              `json:"queueMediaIds,omitempty"`
	ProgressSeconds          int                   `json:"progressSeconds,omitempty"`
	SourceContext            PlaybackSourceContext `json:"sourceContext,omitempty"`
	ExpectedQueueRevision    *int64                `json:"expectedQueueRevision,omitempty"`
	ExpectedPlaybackRevision *int64                `json:"expectedPlaybackRevision,omitempty"`
}

type PlaybackSourceContext struct {
	Type     string   `json:"type,omitempty"`
	ID       string   `json:"id,omitempty"`
	Title    string   `json:"title,omitempty"`
	MediaIDs []string `json:"mediaIds,omitempty"`
}

type LiveTVPlaybackSessionCreateRequest struct {
	ChannelID        string                `json:"channelId"`
	ClientInstanceID string                `json:"clientInstanceId,omitempty"`
	ClientProfile    PlaybackClientProfile `json:"clientProfile,omitempty"`
	Intent           PlaybackIntent        `json:"intent,omitempty"`
}

type LiveTVPlaybackCloseRequest struct {
	SessionID string `json:"sessionId"`
}

type PlaybackProgressEvent struct {
	EventSequence        int64    `json:"eventSequence"`
	Generation           int64    `json:"generation,omitempty"`
	RecordedAt           string   `json:"recordedAt"`
	State                string   `json:"state,omitempty"`
	ProgressSeconds      *int     `json:"progressSeconds,omitempty"`
	PositionSeconds      *float64 `json:"positionSeconds,omitempty"`
	DurationSeconds      int      `json:"durationSeconds,omitempty"`
	BandwidthMbps        float64  `json:"bandwidthMbps,omitempty"`
	SubtitleDecision     string   `json:"subtitleDecision,omitempty"`
	QualityID            string   `json:"qualityId,omitempty"`
	AudioStreamID        string   `json:"audioStreamId,omitempty"`
	SubtitleStreamID     string   `json:"subtitleStreamId,omitempty"`
	SubtitleMode         string   `json:"subtitleMode,omitempty"`
	VersionID            string   `json:"versionId,omitempty"`
	SelectionChanged     bool     `json:"selectionChanged,omitempty"`
	ClientFallbackReason string   `json:"clientFallbackReason,omitempty"`
	Completed            *bool    `json:"completed,omitempty"`
	IsPlaying            *bool    `json:"isPlaying,omitempty"`
	Authority            string   `json:"-"`
}

type PlaybackProgressAcknowledgement struct {
	Accepted             bool   `json:"accepted"`
	Duplicate            bool   `json:"duplicate"`
	Stale                bool   `json:"stale"`
	HighestEventSequence int64  `json:"highestEventSequence"`
	SessionState         string `json:"sessionState"`
	AcceptedAt           string `json:"acceptedAt,omitempty"`
	MediaGrantExpiresAt  string `json:"mediaGrantExpiresAt,omitempty"`
	GrantExtended        bool   `json:"grantExtended,omitempty"`
	GrantSemantics       string `json:"grantSemantics,omitempty"`
	Generation           int64  `json:"generation"`
}

type PlaybackRenegotiationRequest struct {
	RequestID        string                `json:"requestId"`
	ExpectedRevision int64                 `json:"expectedRevision"`
	ClientProfile    PlaybackClientProfile `json:"clientProfile,omitempty"`
	Intent           PlaybackIntent        `json:"intent,omitempty"`
	VersionID        *string               `json:"versionId,omitempty"`
	QualityID        *string               `json:"qualityId,omitempty"`
	AudioStreamID    *string               `json:"audioStreamId,omitempty"`
	SubtitleStreamID *string               `json:"subtitleStreamId,omitempty"`
	SubtitleMode     *string               `json:"subtitleMode,omitempty"`
}

type PlaybackClientProfile struct {
	CapabilitySchemaVersion      string                       `json:"capabilitySchemaVersion,omitempty"`
	ClientFamily                 string                       `json:"clientFamily,omitempty"`
	ClientVersion                string                       `json:"clientVersion,omitempty"`
	CapabilityEvidence           []PlaybackCapabilityEvidence `json:"capabilityEvidence,omitempty"`
	Device                       string                       `json:"device,omitempty"`
	Platform                     string                       `json:"platform,omitempty"`
	SupportsHLS                  bool                         `json:"supportsHls,omitempty"`
	SupportsMSE                  bool                         `json:"supportsMse,omitempty"`
	SupportsMPEGTS               bool                         `json:"supportsMpegTs,omitempty"`
	SupportedContainers          []string                     `json:"supportedContainers,omitempty"`
	SupportedVideoCodecs         []string                     `json:"supportedVideoCodecs,omitempty"`
	SupportedAudioCodecs         []string                     `json:"supportedAudioCodecs,omitempty"`
	MaxWidth                     int                          `json:"maxWidth,omitempty"`
	MaxHeight                    int                          `json:"maxHeight,omitempty"`
	MaxAudioChannels             int                          `json:"maxAudioChannels,omitempty"`
	MaxAudioBitrateKbps          int                          `json:"maxAudioBitrateKbps,omitempty"`
	MaxBitrate                   int                          `json:"maxBitrate,omitempty"`
	MaxVideoBitDepth             int                          `json:"maxVideoBitDepth,omitempty"`
	SupportsHEVC                 bool                         `json:"supportsHevc,omitempty"`
	SupportsHDR                  bool                         `json:"supportsHdr,omitempty"`
	SupportsEAC3                 bool                         `json:"supportsEac3,omitempty"`
	SupportsAC3                  bool                         `json:"supportsAc3,omitempty"`
	SupportedVideoProfiles       []string                     `json:"supportedVideoProfiles,omitempty"`
	SupportedPixelFormats        []string                     `json:"supportedPixelFormats,omitempty"`
	SupportedHDRFormats          []string                     `json:"supportedHdrFormats,omitempty"`
	SupportedDolbyVisionProfiles []string                     `json:"supportedDolbyVisionProfiles,omitempty"`
	PrefersServerProxy           bool                         `json:"prefersServerProxy,omitempty"`
	RequiresServerProxy          bool                         `json:"requiresServerProxy,omitempty"`
	capabilityAuthority          playbackCapabilityAuthority
}

// PlaybackCapabilityEvidence is a complete set of compatible delivery tuples
// reported by one named runtime probe. The planner selects one evidence set;
// it never forms a cross-product from independent codec/container arrays.
type PlaybackCapabilityEvidence struct {
	ID              string                    `json:"id"`
	Source          string                    `json:"source"`
	Confidence      string                    `json:"confidence"`
	Producer        string                    `json:"producer"`
	ProducerVersion string                    `json:"producerVersion,omitempty"`
	ReviewedAt      string                    `json:"reviewedAt"`
	MinVersion      string                    `json:"minVersion,omitempty"`
	MaxVersion      string                    `json:"maxVersion,omitempty"`
	Tuples          []PlaybackCapabilityTuple `json:"tuples"`
}

type PlaybackCapabilityTuple struct {
	MediaKind string                     `json:"mediaKind"`
	Protocol  string                     `json:"protocol"`
	Container string                     `json:"container"`
	Video     PlaybackCapabilityVideo    `json:"video,omitempty"`
	Audio     PlaybackCapabilityAudio    `json:"audio"`
	Subtitle  PlaybackCapabilitySubtitle `json:"subtitle"`
}

type PlaybackCapabilityVideo struct {
	Codec              string  `json:"codec"`
	Profile            string  `json:"profile,omitempty"`
	Level              string  `json:"level,omitempty"`
	Tag                string  `json:"tag,omitempty"`
	PixelFormat        string  `json:"pixelFormat,omitempty"`
	Chroma             string  `json:"chroma,omitempty"`
	DynamicRange       string  `json:"dynamicRange,omitempty"`
	BitDepth           int     `json:"bitDepth"`
	DolbyVisionProfile int     `json:"dolbyVisionProfile,omitempty"`
	MaxWidth           int     `json:"maxWidth"`
	MaxHeight          int     `json:"maxHeight"`
	MaxFrameRate       float64 `json:"maxFrameRate"`
}

type PlaybackCapabilityAudio struct {
	Codec             string `json:"codec"`
	Profile           string `json:"profile,omitempty"`
	Layout            string `json:"layout,omitempty"`
	Route             string `json:"route,omitempty"`
	MaxChannels       int    `json:"maxChannels"`
	ObjectPassthrough bool   `json:"objectPassthrough,omitempty"`
}

type PlaybackCapabilitySubtitle struct {
	Codec string `json:"codec,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Mode  string `json:"mode"`
}

// PlaybackIntent is a portable request, never authority. The server resolves
// it against the media type, route, decoder facts, membership and server
// clamps before choosing a delivery mode.
type PlaybackIntent struct {
	NetworkClass               string   `json:"networkClass,omitempty"`
	TransportClass             string   `json:"transportClass,omitempty"`
	QualityProfile             string   `json:"qualityProfile,omitempty"`
	DirectPlayPolicy           string   `json:"directPlayPolicy,omitempty"`
	DirectStreamPolicy         string   `json:"directStreamPolicy,omitempty"`
	TranscodePolicy            string   `json:"transcodePolicy,omitempty"`
	MaxVideoBitrateMbps        int      `json:"maxVideoBitrateMbps,omitempty"`
	MaxAudioBitrateKbps        int      `json:"maxAudioBitrateKbps,omitempty"`
	MaxVideoHeight             int      `json:"maxVideoHeight,omitempty"`
	AllowHDR                   *bool    `json:"allowHdr,omitempty"`
	PreferredAudioLanguages    []string `json:"preferredAudioLanguages,omitempty"`
	PreferredSubtitleLanguages []string `json:"preferredSubtitleLanguages,omitempty"`
	PreferredAudioLanguage     string   `json:"preferredAudioLanguage,omitempty"`
	PreferredSubtitleLanguage  string   `json:"preferredSubtitleLanguage,omitempty"`
	PreferredSubtitleMode      string   `json:"preferredSubtitleMode,omitempty"`
	SubtitlesEnabled           *bool    `json:"subtitlesEnabled,omitempty"`
	BurnInSubtitles            bool     `json:"burnInSubtitles,omitempty"`
}

type ResolvedPlaybackPolicy struct {
	NetworkClass        string   `json:"networkClass"`
	TransportClass      string   `json:"transportClass,omitempty"`
	ServerLocality      string   `json:"serverLocality,omitempty"`
	QualityProfile      string   `json:"qualityProfile"`
	DirectPlayPolicy    string   `json:"directPlayPolicy"`
	DirectStreamPolicy  string   `json:"directStreamPolicy"`
	TranscodePolicy     string   `json:"transcodePolicy"`
	MaxVideoBitrateMbps int      `json:"maxVideoBitrateMbps,omitempty"`
	MaxAudioBitrateKbps int      `json:"maxAudioBitrateKbps,omitempty"`
	MaxVideoHeight      int      `json:"maxVideoHeight,omitempty"`
	AllowHDR            bool     `json:"allowHdr"`
	DeliveryProfile     string   `json:"deliveryProfile"`
	ServerClamps        []string `json:"serverClamps"`
	// LiveHLS is additive and never replaces the canonical delivery fields.
	LiveHLS      *LiveHLSPlaybackPolicy  `json:"liveHls,omitempty"`
	LiveDelivery *PlaybackDeliveryPolicy `json:"liveDelivery,omitempty"`
}

type LiveHLSPlaybackPolicy struct {
	AuthorizationTransport string `json:"authorizationTransport"`
	PlaylistScope          string `json:"playlistScope"`
	SegmentScope           string `json:"segmentScope"`
	CredentialQueryAllowed bool   `json:"credentialQueryAllowed"`
}

// PlaybackDeliveryPolicy describes the server-owned authorization and
// delivery boundary for continuous Live TV and Library Channel HLS media.
// It is additive to the general resolved playback policy.
type PlaybackDeliveryPolicy struct {
	DeliveryMode                string   `json:"deliveryMode"`
	GrantRequired               bool     `json:"grantRequired"`
	AllowedOperationClasses     []string `json:"allowedOperationClasses"`
	AuthorizationRecheckSeconds int      `json:"authorizationRecheckSeconds"`
	QualityProfile              string   `json:"qualityProfile,omitempty"`
	OverlayTranscode            bool     `json:"overlayTranscode,omitempty"`
	ResourceRevision            int64    `json:"resourceRevision,omitempty"`
}

type PlaybackDecision struct {
	Mode                 string   `json:"mode"`
	Reason               string   `json:"reason"`
	ReasonCodes          []string `json:"reasonCodes"`
	SourceKind           string   `json:"sourceKind,omitempty"`
	Protocol             string   `json:"protocol,omitempty"`
	Container            string   `json:"container,omitempty"`
	VideoCodec           string   `json:"videoCodec,omitempty"`
	AudioCodec           string   `json:"audioCodec,omitempty"`
	DeliveryProfile      string   `json:"deliveryProfile,omitempty"`
	RequiresTranscode    bool     `json:"requiresTranscode"`
	RequiresRemux        bool     `json:"requiresRemux,omitempty"`
	VideoTranscode       bool     `json:"videoTranscode,omitempty"`
	AudioTranscode       bool     `json:"audioTranscode,omitempty"`
	IsProxied            bool     `json:"isProxied"`
	IsServerCached       bool     `json:"isServerCached"`
	BufferSeconds        int      `json:"bufferSeconds,omitempty"`
	PlanSchemaVersion    int      `json:"planSchemaVersion,omitempty"`
	PlanDigest           string   `json:"-"`
	SourceRevision       string   `json:"-"`
	CapabilityEvidenceID string   `json:"-"`
	Generation           int      `json:"generation,omitempty"`
	VideoAction          string   `json:"videoAction,omitempty"`
	AudioAction          string   `json:"audioAction,omitempty"`
	SubtitleAction       string   `json:"subtitleAction,omitempty"`
	HardwareBackend      string   `json:"hardwareBackend,omitempty"`
	execution            *playbackExecutionBinding
}

type PlaybackResource struct {
	ID               string `json:"id"`
	SourceURL        string `json:"sourceUrl"`
	StreamFormat     string `json:"streamFormat"`
	QualityID        string `json:"qualityId,omitempty"`
	AudioStreamID    string `json:"audioStreamId,omitempty"`
	SubtitleStreamID string `json:"subtitleStreamId,omitempty"`
	SubtitleMode     string `json:"subtitleMode,omitempty"`
	Default          bool   `json:"default,omitempty"`
}

type PlaybackResponse struct {
	SessionID              string                          `json:"sessionId"`
	NextEventSequence      int64                           `json:"nextEventSequence"`
	MediaGrant             MediaGrant                      `json:"mediaGrant"`
	ContinuationCredential *PlaybackContinuationCredential `json:"continuationCredential"`
	Media                  MediaItem                       `json:"media"`
	SourceURL              string                          `json:"sourceUrl"`
	DirectPlay             bool                            `json:"directPlay"`
	IsLive                 bool                            `json:"isLive,omitempty"`
	StreamFormat           string                          `json:"streamFormat,omitempty"`
	Resources              []PlaybackResource              `json:"resources"`
	Decision               PlaybackDecision                `json:"decision"`
	Policy                 ResolvedPlaybackPolicy          `json:"policy"`
	Qualities              []Quality                       `json:"qualities"`
	AudioStreams           []Stream                        `json:"audioStreams"`
	SelectedAudioStreamID  string                          `json:"selectedAudioStreamId,omitempty"`
	SelectedQualityID      string                          `json:"selectedQualityId,omitempty"`
	SelectedSubtitleID     string                          `json:"selectedSubtitleStreamId,omitempty"`
	SelectedSubtitleMode   string                          `json:"selectedSubtitleMode,omitempty"`
	SelectedVersionID      string                          `json:"selectedVersionId,omitempty"`
	SubtitleStreams        []Stream                        `json:"subtitleStreams"`
	Chapters               []Chapter                       `json:"chapters"`
	Queue                  []MediaItem                     `json:"queue"`
	RepeatMode             string                          `json:"repeatMode"`
	QueueRevision          int64                           `json:"queueRevision"`
	SourceContext          PlaybackSourceContext           `json:"sourceContext,omitempty"`
	Timeline               PlaybackTimeline                `json:"timeline"`
	ResumePositionSeconds  int                             `json:"resumePositionSeconds,omitempty"`
	Generation             int                             `json:"generation"`
	PlaybackRevision       int64                           `json:"playbackRevision"`
}

// PlaybackContinuationCredential is a short-lived, session-only credential
// for native background/PiP progress and state continuity. It is never a
// media grant or an account credential and must be sent only as
// Authorization: PorticoPlayback <token>.
type PlaybackContinuationCredential struct {
	Token      string `json:"token"`
	ExpiresAt  string `json:"expiresAt"`
	Origin     string `json:"origin"`
	Generation int64  `json:"generation"`
}

type PlaybackContinuationState struct {
	SessionID            string `json:"sessionId"`
	State                string `json:"state"`
	PositionSeconds      int    `json:"positionSeconds"`
	HighestEventSequence int64  `json:"highestEventSequence"`
	MediaGrantExpiresAt  string `json:"mediaGrantExpiresAt,omitempty"`
	QueueRevision        int64  `json:"queueRevision"`
	PlaybackRevision     int64  `json:"playbackRevision"`
	Generation           int64  `json:"generation"`
}

type PlaybackContinuationRotateRequest struct {
	RequestID string `json:"requestId"`
}

type PlaybackTimeline struct {
	Type                 string   `json:"type"`
	DurationSeconds      int      `json:"durationSeconds,omitempty"`
	SegmentSeconds       int      `json:"segmentSeconds,omitempty"`
	SeekableStartSeconds *float64 `json:"seekableStartSeconds,omitempty"`
	SeekableEndSeconds   *float64 `json:"seekableEndSeconds,omitempty"`
	LiveEdgeSeconds      *float64 `json:"liveEdgeSeconds,omitempty"`
	CanPause             bool     `json:"canPause"`
	CanSeek              bool     `json:"canSeek"`
}

type PlaybackCommand struct {
	ID                string `json:"id"`
	Action            string `json:"action"`
	MediaID           string `json:"mediaId,omitempty"`
	PositionSeconds   int    `json:"positionSeconds,omitempty"`
	Message           string `json:"message,omitempty"`
	IssuedByUserID    string `json:"-"`
	IssuedByProfileID string `json:"issuedByProfileId,omitempty"`
	IssuedAt          string `json:"issuedAt,omitempty"`
}

type PlaybackCommandRequest struct {
	Action          string `json:"action"`
	MediaID         string `json:"mediaId,omitempty"`
	PositionSeconds int    `json:"positionSeconds"`
	Message         string `json:"message,omitempty"`
}

type PlaybackReceiver struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Code              string          `json:"code"`
	App               string          `json:"app,omitempty"`
	Platform          string          `json:"platform,omitempty"`
	SupportedCommands []string        `json:"supportedCommands"`
	Command           PlaybackCommand `json:"command"`
	CreatedAt         string          `json:"createdAt"`
	LastSeenAt        string          `json:"lastSeenAt"`
}

type PlaybackReceiverRequest struct {
	Name              string   `json:"name"`
	App               string   `json:"app,omitempty"`
	Platform          string   `json:"platform,omitempty"`
	SupportedCommands []string `json:"supportedCommands,omitempty"`
}

type WatchWithFriendsGroup struct {
	ID                  string                      `json:"id"`
	Name                string                      `json:"name"`
	OwnerUserID         string                      `json:"-"`
	OwnerProfileID      string                      `json:"ownerProfileId"`
	OwnerName           string                      `json:"ownerName"`
	MediaID             string                      `json:"mediaId"`
	MediaTitle          string                      `json:"mediaTitle"`
	State               string                      `json:"state"`
	PositionSeconds     int                         `json:"positionSeconds"`
	PositionUpdatedAt   string                      `json:"positionUpdatedAt"`
	ServerTime          string                      `json:"serverTime"`
	PlaybackRate        float64                     `json:"playbackRate"`
	Revision            int64                       `json:"revision"`
	PlaybackRevision    int64                       `json:"playbackRevision"`
	ReconnectGeneration int64                       `json:"reconnectGeneration"`
	LastIdempotencyKey  string                      `json:"-"`
	Permissions         WatchWithFriendsPermissions `json:"permissions"`
	ShuffleEnabled      bool                        `json:"shuffleEnabled"`
	RepeatMode          string                      `json:"repeatMode"`
	Command             PlaybackCommand             `json:"command"`
	Members             []WatchWithFriendsMember    `json:"members"`
	Queue               []WatchWithFriendsQueueItem `json:"queue"`
	CreatedAt           string                      `json:"createdAt"`
	UpdatedAt           string                      `json:"updatedAt"`
}

type WatchWithFriendsPermissions struct {
	IsHost         bool `json:"isHost"`
	CanControl     bool `json:"canControl"`
	CanManageQueue bool `json:"canManageQueue"`
}

type WatchWithFriendsMember struct {
	UserID          string `json:"-"`
	ProfileID       string `json:"profileId"`
	DisplayName     string `json:"displayName"`
	State           string `json:"state"`
	PositionSeconds int    `json:"positionSeconds"`
	JoinedAt        string `json:"joinedAt"`
	LastSeenAt      string `json:"lastSeenAt"`
}

type WatchWithFriendsCreateRequest struct {
	Name    string `json:"name,omitempty"`
	MediaID string `json:"mediaId"`
}

type WatchWithFriendsStateRequest struct {
	Action           string  `json:"action"`
	MediaID          string  `json:"mediaId,omitempty"`
	PositionSeconds  int     `json:"positionSeconds"`
	PlaybackRate     float64 `json:"playbackRate,omitempty"`
	ExpectedRevision *int64  `json:"expectedRevision,omitempty"`
	IdempotencyKey   string  `json:"idempotencyKey"`
}

type WatchWithFriendsMemberStateRequest struct {
	State           string `json:"state"`
	PositionSeconds int    `json:"positionSeconds"`
}

type WatchWithFriendsSettingsRequest struct {
	ShuffleEnabled   *bool  `json:"shuffleEnabled,omitempty"`
	RepeatMode       string `json:"repeatMode,omitempty"`
	ExpectedRevision *int64 `json:"expectedRevision,omitempty"`
	IdempotencyKey   string `json:"idempotencyKey"`
}

type WatchWithFriendsQueueItem struct {
	MediaID          string `json:"mediaId"`
	MediaTitle       string `json:"mediaTitle"`
	SortOrder        int    `json:"sortOrder"`
	AddedByUserID    string `json:"-"`
	AddedByProfileID string `json:"addedByProfileId"`
	AddedAt          string `json:"addedAt"`
}

type WatchWithFriendsQueueRequest struct {
	MediaID          string `json:"mediaId"`
	ExpectedRevision *int64 `json:"expectedRevision,omitempty"`
	IdempotencyKey   string `json:"idempotencyKey"`
}

type WatchWithFriendsQueueOrderRequest struct {
	MediaIDs         []string `json:"mediaIds"`
	ExpectedRevision *int64   `json:"expectedRevision,omitempty"`
	IdempotencyKey   string   `json:"idempotencyKey"`
}

type PlaybackTarget struct {
	ID                string   `json:"id"`
	Type              string   `json:"type"`
	Name              string   `json:"name"`
	Detail            string   `json:"detail"`
	Code              string   `json:"code,omitempty"`
	SupportedCommands []string `json:"supportedCommands"`
	LastSeenAt        string   `json:"lastSeenAt"`
}

type Quality struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	Description       string `json:"description"`
	Available         bool   `json:"available"`
	RequiresTranscode bool   `json:"requiresTranscode,omitempty"`
}

type Chapter struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	StartSeconds int    `json:"startSeconds"`
	EndSeconds   int    `json:"endSeconds,omitempty"`
	ThumbURL     string `json:"thumbUrl,omitempty"`
}
