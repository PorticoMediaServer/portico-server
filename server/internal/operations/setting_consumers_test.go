package operations_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"portico.local/server/internal/administration"
	"portico.local/server/internal/downloads"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/metadata"
	"portico.local/server/internal/operations"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type settingConsumer struct{ file, witness, effect string }

// Every entry names a reviewed runtime boundary, not a settings form or registry.
// Semantic tests beside those consumers verify behavior; this guard catches new
// unowned fields and consumer removal. It is not a substitute for runtime tests.
var settingConsumers = map[string]settingConsumer{
	"administration.DVRDefaults.keepPolicy.keepCount":                        {"internal/administration/dvr_runtime.go", "KeepCount", "Set padding or retained recording count/days"},
	"administration.DVRDefaults.keepPolicy.keepDays":                         {"internal/administration/dvr_runtime.go", "KeepDays", "Set padding or retained recording count/days"},
	"administration.DVRDefaults.keepPolicy.mode":                             {"internal/administration/dvr_runtime.go", "Mode", "Set padding or retained recording count/days"},
	"administration.DVRDefaults.postPaddingSeconds":                          {"internal/administration/dvr_runtime.go", "PostPaddingSeconds", "Set padding or retained recording count/days"},
	"administration.DVRDefaults.prePaddingSeconds":                           {"internal/administration/dvr_runtime.go", "PrePaddingSeconds", "Set padding or retained recording count/days"},
	"administration.LibrarySettings.allowMediaDeletion":                      {"internal/administration/deletion.go", "AllowMediaDeletion", "Authorize physical media deletion"},
	"administration.LibrarySettings.analysis":                                {"internal/administration/library_runtime.go", "Analysis", "Select analysis recipes and trickplay geometry"},
	"administration.LibrarySettings.continueWatching.includeSeasonPremieres": {"internal/catalog/continue_policy.go", "IncludeSeasonPremieres", "Select continue-watching rows and completion rules"},
	"administration.LibrarySettings.continueWatching.maximumItems":           {"internal/catalog/continue_policy.go", "MaximumItems", "Select continue-watching rows and completion rules"},
	"administration.LibrarySettings.continueWatching.videoCompletion":        {"internal/catalog/continue_policy.go", "VideoCompletion", "Select continue-watching rows and completion rules"},
	"administration.LibrarySettings.continueWatching.videoPlayedThreshold":   {"internal/catalog/continue_policy.go", "VideoPlayedThreshold", "Select continue-watching rows and completion rules"},
	"administration.LibrarySettings.continueWatching.weeks":                  {"internal/catalog/continue_policy.go", "Weeks", "Select continue-watching rows and completion rules"},
	"administration.LibrarySettings.navigation.chapterThumbnailMode":         {"internal/administration/library_runtime.go", "ChapterThumbnailMode", "Select analysis recipes and trickplay geometry"},
	"administration.LibrarySettings.navigation.trickplayIntervalSeconds":     {"internal/administration/library_runtime.go", "TrickplayIntervalSeconds", "Select analysis recipes and trickplay geometry"},
	"administration.LibrarySettings.navigation.trickplayMaxTiles":            {"internal/administration/library_runtime.go", "TrickplayMaxTiles", "Select analysis recipes and trickplay geometry"},
	"administration.LibrarySettings.navigation.trickplayTileWidth":           {"internal/administration/library_runtime.go", "TrickplayTileWidth", "Select analysis recipes and trickplay geometry"},
	"administration.LibrarySettings.providers.language":                      {"internal/administration/library_runtime.go", "Language", "Apply provider and locale policy to domain tables"},
	"administration.LibrarySettings.providers.mediaKind":                     {"internal/administration/library_runtime.go", "MediaKind", "Apply provider and locale policy to domain tables"},
	"administration.LibrarySettings.providers.provider":                      {"internal/administration/library_runtime.go", "Provider", "Apply provider and locale policy to domain tables"},
	"administration.LibrarySettings.providers.region":                        {"internal/administration/library_runtime.go", "Region", "Apply provider and locale policy to domain tables"},
	"administration.MaintenanceSettingsDocument.backgroundSpeed":             {"internal/administration/background_policy.go", "BackgroundSpeed", "Set shared batch pacing and derived process priority"},
	"administration.MaintenanceSettingsDocument.backupKeepCount":             {"internal/administration/maintenance_runtime.go", "BackupKeepCount", "Keep the owner number of scheduled backups"},
	"administration.MaintenanceSettingsDocument.retention":                   {"internal/administration/maintenance.go", "Retention", "Select the age for an owner cleanup action"},
	"administration.MaintenanceSettingsDocument.windows.days":                {"internal/workpolicy/policy.go", "Days", "Select scheduled backup occurrences"},
	"administration.MaintenanceSettingsDocument.windows.durationMinutes":     {"internal/workpolicy/policy.go", "DurationMinutes", "Select scheduled backup occurrences"},
	"administration.MaintenanceSettingsDocument.windows.enabled":             {"internal/workpolicy/policy.go", "Enabled", "Select scheduled backup occurrences"},
	"administration.MaintenanceSettingsDocument.windows.id":                  {"internal/workpolicy/policy.go", "ID", "Select scheduled backup occurrences"},
	"administration.MaintenanceSettingsDocument.windows.startMinute":         {"internal/workpolicy/policy.go", "StartMinute", "Select scheduled backup occurrences"},
	"administration.MaintenanceSettingsDocument.windows.tasks":               {"internal/workpolicy/policy.go", "Tasks", "Select scheduled backup occurrences"},
	"administration.MaintenanceSettingsDocument.windows.timezone":            {"internal/workpolicy/policy.go", "Timezone", "Select scheduled backup occurrences"},
	"downloads.Settings.maxPreparedBytes":                                    {"internal/downloads/service.go", "MaxPreparedBytes", "Admit prepared bytes or expire download claims"},
	"downloads.Settings.retentionDays":                                       {"internal/downloads/service.go", "RetentionDays", "Admit prepared bytes or expire download claims"},
	"livechannels.LiveDefaults.discoveryEnabled":                             {"internal/administration/live_sources.go", "DiscoveryEnabled", "Apply shared guide and live-source defaults"},
	"livechannels.LiveDefaults.guideDays":                                    {"internal/livechannels/directory.go", "GuideDays", "Apply shared guide and live-source defaults"},
	"livechannels.LiveDefaults.importLimits.bytes":                           {"internal/livechannels/model.go", "Bytes", "Bound guide or playlist bytes and row counts"},
	"livechannels.LiveDefaults.importLimits.channels":                        {"internal/livechannels/model.go", "Channels", "Bound guide or playlist bytes and row counts"},
	"livechannels.LiveDefaults.importLimits.programmes":                      {"internal/livechannels/model.go", "Programmes", "Bound guide or playlist bytes and row counts"},
	"livechannels.LiveDefaults.logoImport":                                   {"internal/administration/guide_image_worker.go", "LogoImport", "Apply shared guide and live-source defaults"},
	"livechannels.LiveDefaults.retryWindowSeconds":                           {"internal/livechannels/configuration.go", "RetryWindowSeconds", "Apply shared guide and live-source defaults"},
	"livechannels.LiveDefaults.streamBufferSeconds":                          {"internal/livechannels/configuration.go", "StreamBufferSeconds", "Apply shared guide and live-source defaults"},
	"livechannels.LiveDefaults.userAgent":                                    {"internal/livechannels/configuration.go", "UserAgent", "Apply shared guide and live-source defaults"},
	"livechannels.LiveSourceSettings.channelNumbering.mode":                  {"internal/administration/live_sources.go", "Mode", "Set source kind, guide mapping and channel numbers"},
	"livechannels.LiveSourceSettings.channelNumbering.startAt":               {"internal/administration/live_sources.go", "StartAt", "Set source kind, guide mapping and channel numbers"},
	"livechannels.LiveSourceSettings.channelNumbering.step":                  {"internal/administration/live_sources.go", "Step", "Set source kind, guide mapping and channel numbers"},
	"livechannels.LiveSourceSettings.enabledChannels":                        {"internal/livechannels/remote.go", "EnabledChannels", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.enabledGroups":                          {"internal/livechannels/remote.go", "EnabledGroups", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.filters.categories.mode":                {"internal/livechannels/configuration.go", "Mode", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.filters.categories.values":              {"internal/livechannels/configuration.go", "Values", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.filters.countries.mode":                 {"internal/livechannels/configuration.go", "Mode", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.filters.countries.values":               {"internal/livechannels/configuration.go", "Values", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.filters.keywords.mode":                  {"internal/livechannels/configuration.go", "Mode", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.filters.keywords.values":                {"internal/livechannels/configuration.go", "Values", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.guideSourceId":                          {"internal/administration/live_sources.go", "GuideSourceID", "Set source kind, guide mapping and channel numbers"},
	"livechannels.LiveSourceSettings.kind":                                   {"internal/administration/live_sources.go", "Kind", "Set source kind, guide mapping and channel numbers"},
	"livechannels.LiveSourceSettings.logoImport.enabled":                     {"internal/livechannels/configuration.go", "Enabled", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.logoImport.overwriteExisting":           {"internal/livechannels/store.go", "OverwriteExisting", "Preserve or replace imported logos"},
	"livechannels.LiveSourceSettings.retryWindowSeconds":                     {"internal/livechannels/configuration.go", "RetryWindowSeconds", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.streamBufferSeconds":                    {"internal/livechannels/configuration.go", "StreamBufferSeconds", "Filter or override the saved live source"},
	"livechannels.LiveSourceSettings.userAgent":                              {"internal/livechannels/configuration.go", "UserAgent", "Filter or override the saved live source"},
	"metadata.MetadataSettingsUpdate.includeAdult":                           {"internal/metadata/runtime_settings.go", "IncludeAdult", "Apply artwork storage, adult matching or provider credentials"},
	"metadata.MetadataSettingsUpdate.localArtworkLimitBytes":                 {"internal/metadata/runtime_settings.go", "literal:local_artwork_limit_bytes", "Apply artwork storage, adult matching or provider credentials"},
	"metadata.MetadataSettingsUpdate.tmdbReadToken":                          {"internal/metadata/runtime_settings.go", "TMDBReadToken", "Apply artwork storage, adult matching or provider credentials"},
	"metadata.MetadataSettingsUpdate.tvdbProjectKey":                         {"internal/metadata/runtime_settings.go", "TVDBProjectKey", "Apply artwork storage, adult matching or provider credentials"},
	"operations.NotificationSettings.certificateWarningDays":                 {"internal/operations/notifications.go", "CertificateWarningDays", "Prune inbox history or emit owner storage/certificate notices"},
	"operations.NotificationSettings.retentionDays":                          {"internal/operations/notifications.go", "RetentionDays", "Prune inbox history or emit owner storage/certificate notices"},
	"operations.NotificationSettings.storageCriticalPercent":                 {"internal/operations/notifications.go", "StorageCriticalPercent", "Prune inbox history or emit owner storage/certificate notices"},
	"operations.NotificationSettings.storageWarningPercent":                  {"internal/operations/notifications.go", "StorageWarningPercent", "Prune inbox history or emit owner storage/certificate notices"},
	"operations.Settings.accessUrls":                                         {"cmd/server/networking_initializer.go", "AccessURLs", "Publish owner connection candidates"},
	"operations.Settings.advertiseUrl":                                       {"cmd/server/networking_initializer.go", "AdvertiseURL", "Publish owner connection candidates"},
	"operations.Settings.advertisedInterface":                                {"cmd/server/main.go", "AdvertisedInterface", "Configure the running listener or media process"},
	"operations.Settings.analysisStageStorageMiB":                            {"internal/mediaanalysis/storage_limits.go", "literal:analysisStageStorageMiB", "Bound analysis artifacts or one stage output"},
	"operations.Settings.analysisStorageGiB":                                 {"internal/mediaanalysis/storage_limits.go", "literal:analysisStorageGiB", "Bound analysis artifacts or one stage output"},
	"operations.Settings.apiRequestsPerMinute":                               {"internal/httpapi/connection_policy.go", "APIRequestsPerMinute", "Rate limit a key or token"},
	"operations.Settings.blockMemberAccessTokens":                            {"internal/httpapi/api_credentials_routes.go", "BlockMemberAccessTokens", "Admit or refuse member token creation"},
	"operations.Settings.connectionLimit":                                    {"cmd/server/main.go", "ConnectionLimit", "Configure the running listener or media process"},
	"operations.Settings.customCertificateDomain":                            {"cmd/server/main.go", "CustomCertificateDomain", "Configure the running listener or media process"},
	"operations.Settings.customCertificateKeyPath":                           {"cmd/server/main.go", "CustomCertificateKeyPath", "Configure the running listener or media process"},
	"operations.Settings.customCertificatePath":                              {"cmd/server/main.go", "CustomCertificatePath", "Configure the running listener or media process"},
	"operations.Settings.deviceApprovalRequired":                             {"internal/httpapi/access_admission.go", "DeviceApprovalRequired", "Apply device approval to playback admission"},
	"operations.Settings.diagnosticDays":                                     {"internal/operations/evidence.go", "DiagnosticDays", "Prune the corresponding history by its retention rule"},
	"operations.Settings.directStreamRemux":                                  {"internal/httpapi/delivery_settings_adapter.go", "DirectStreamRemux", "Project conversion policy to runtime delivery settings"},
	"operations.Settings.ffmpegPath":                                         {"cmd/server/main.go", "FFmpegPath", "Configure the running listener or media process"},
	"operations.Settings.hardwareBackend":                                    {"internal/playback/delivery_plan.go", "HardwareBackend", "Plan media delivery using owner policy"},
	"operations.Settings.hardwareDevice":                                     {"internal/playback/delivery_plan.go", "HardwareDevice", "Plan media delivery using owner policy"},
	"operations.Settings.hdrToneMapping":                                     {"internal/httpapi/delivery_settings_adapter.go", "HDRToneMapping", "Project conversion policy to runtime delivery settings"},
	"operations.Settings.hdrToneMappingAlgorithm":                            {"internal/httpapi/delivery_settings_adapter.go", "HDRToneMappingAlgorithm", "Project conversion policy to runtime delivery settings"},
	"operations.Settings.home.communityActivityEnabled":                      {"internal/httpapi/home_routes.go", "CommunityActivityEnabled", "Include community activity in home content"},
	"operations.Settings.jobDays":                                            {"internal/operations/evidence.go", "JobDays", "Prune the corresponding history by its retention rule"},
	"operations.Settings.lanDiscoveryEnabled":                                {"cmd/server/administration.go", "LANDiscoveryEnabled", "Start or stop LAN advertisement"},
	"operations.Settings.lanNetworks":                                        {"internal/httpapi/request_locality.go", "LANNetworks", "Classify home connections and forwarded client addresses"},
	"operations.Settings.library.missingTitles":                              {"internal/catalog/missing.go", "literal:library.missingTitles", "Keep or remove missing titles by the owner rule"},
	"operations.Settings.library.missingTitlesDays":                          {"internal/catalog/missing.go", "literal:library.missingTitlesDays", "Keep or remove missing titles by the owner rule"},
	"operations.Settings.logLevel":                                           {"cmd/server/administration.go", "LogLevel", "Choose recorded log detail"},
	"operations.Settings.logRetention.category":                              {"internal/httpapi/admin_diagnostics.go", "LogRetentionDays", "Prune the message log by category"},
	"operations.Settings.logRetention.days":                                  {"internal/httpapi/admin_diagnostics.go", "LogRetentionDays", "Prune the message log by category"},
	"operations.Settings.maxBackgroundSessions":                              {"internal/httpapi/delivery_settings_adapter.go", "MaxBackgroundSessions", "Project conversion policy to runtime delivery settings"},
	"operations.Settings.maxConcurrentSessions":                              {"internal/httpapi/delivery_settings_adapter.go", "MaxConcurrentSessions", "Project conversion policy to runtime delivery settings"},
	"operations.Settings.maxHardwareSessions":                                {"internal/httpapi/delivery_settings_adapter.go", "MaxHardwareSessions", "Project conversion policy to runtime delivery settings"},
	"operations.Settings.maxSoftwareSessions":                                {"internal/httpapi/delivery_settings_adapter.go", "MaxSoftwareSessions", "Project conversion policy to runtime delivery settings"},
	"operations.Settings.name":                                               {"internal/identity/service.go", "literal:name", "Name the server in identity documents"},
	"operations.Settings.notificationDays":                                   {"internal/operations/evidence.go", "NotificationDays", "Prune the corresponding history by its retention rule"},
	"operations.Settings.pausedSessionTimeoutMinutes":                        {"internal/httpapi/playback_v1_routes.go", "PausedSessionTimeoutMinutes", "Expire paused playback sessions"},
	"operations.Settings.perAccountCap":                                      {"internal/playback/v1_session_helpers.go", "literal:per_account_cap", "Admit streams against the owner cap"},
	"operations.Settings.planningPolicy":                                     {"internal/playback/delivery_plan.go", "PlanningPolicy", "Plan media delivery using owner policy"},
	"operations.Settings.playHistoryDays":                                    {"internal/operations/evidence.go", "PlayHistoryDays", "Prune the corresponding history by its retention rule"},
	"operations.Settings.playedRetentionSeconds":                             {"internal/playback/delivery_settings.go", "PlayedRetentionSeconds", "Pace conversion and retain played segments"},
	"operations.Settings.remoteBitrateLimitKbps":                             {"internal/httpapi/delivery_settings_adapter.go", "RemoteBitrateLimitKbps", "Project conversion policy to runtime delivery settings"},
	"operations.Settings.remoteSignInPolicy":                                 {"internal/httpapi/access_admission.go", "RemoteSignInPolicy", "Admit remote sign-ins under owner policy"},
	"operations.Settings.serverCap":                                          {"internal/playback/v1_session_helpers.go", "literal:server_cap", "Admit streams against the owner cap"},
	"operations.Settings.sessionStorageMiB":                                  {"internal/httpapi/delivery_settings_adapter.go", "SessionStorageMiB", "Project conversion policy to runtime delivery settings"},
	"operations.Settings.subtitleFileLimitMiB":                               {"internal/httpapi/connection_policy.go", "SubtitleFileLimitMiB", "Apply subtitle discovery and acquisition bounds"},
	"operations.Settings.subtitleTrackLimit":                                 {"internal/httpapi/connection_policy.go", "SubtitleTrackLimit", "Apply subtitle discovery and acquisition bounds"},
	"operations.Settings.temporaryDirectory":                                 {"cmd/server/main.go", "TemporaryDirectory", "Configure the running listener or media process"},
	"operations.Settings.throttleBufferSeconds":                              {"internal/playback/delivery_settings.go", "ThrottleBufferSeconds", "Pace conversion and retain played segments"},
	"operations.Settings.transcodingEnabled":                                 {"internal/playback/delivery_plan.go", "TranscodingEnabled", "Plan media delivery using owner policy"},
	"operations.Settings.trustedProxies":                                     {"internal/httpapi/request_locality.go", "TrustedProxies", "Classify home connections and forwarded client addresses"},
	"operations.Settings.updates.feedUrl":                                    {"cmd/server/main.go", "UpdateFeedURL", "Check the selected release feed and channel"},
	"operations.Settings.updates.releaseChannel":                             {"cmd/server/main.go", "UpdateChannel", "Check the selected release feed and channel"},
	"operations.Settings.uploadCapacityKbps":                                 {"internal/httpapi/playback_v1_routes.go", "UploadCapacityKbps", "Divide the remote upload ceiling among streams"},
	"operations.Settings.x264Preset":                                         {"internal/httpapi/delivery_settings_adapter.go", "X264Preset", "Project conversion policy to runtime delivery settings"},
}

// These fields are readback identity/vocabulary, never editable stored controls.
// Unsupported alternatives are rejected by the domain's save validation.
var informationalSettings = map[string]string{
	"*.revision":         "Readback revision fence",
	"*.expectedRevision": "Mutation revision fence",
	"administration.MaintenanceSettingsDocument.windows.name":    "Owner-authored label for the schedule editor",
	"administration.MaintenanceSettingsDocument.windows.cadence": "Schedule editor preset; expanded days are runtime authority",
	"administration.LibrarySettings.providers.apiKey*":           "Keys are held by metadata runtime; library key writes are rejected",
	"administration.LibrarySettings.navigation.videoPreview*":    "Fixed unsupported preview status; changing either value is rejected",
	"administration.DVRDefaults.conversion.*":                    "Only original recording is supported; conversion alternatives are rejected",
	"administration.DVRDefaults.sidecars.*":                      "Unsupported sidecar alternatives are rejected",
	"administration.DVRDefaults.keepPolicy.keepUntilWatched":     "Unsupported keep-until-watched alternative is rejected",
	"administration.DVRDefaults.*Template":                       "Unconfigured folder templates; nonempty writes are rejected",
	"administration.DVRDefaults.recordingProfile":                "Only original recording is supported",
	"administration.DVRDefaults.tunerPreference":                 "Only first-available allocation is supported",
	"operations.NotificationSettings.retentionSettingsField":     "Alias for the single notificationDays authority",
	"operations.NotificationSettings.maintenanceIntervalMinutes": "Readback cadence description, not an accepted mutation field",
}

func inventoryFields(typ reflect.Type, prefix string, visit func(string)) {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		visit(prefix)
		return
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		inventoryFields(field.Type, prefix+"."+tag, visit)
	}
}
func TestStoredSettingsHaveRuntimeConsumers(t *testing.T) {
	roots := map[string]reflect.Type{
		"operations.Settings":                        reflect.TypeFor[operations.Settings](),
		"administration.MaintenanceSettingsDocument": reflect.TypeFor[administration.MaintenanceSettingsDocument](),
		"administration.LibrarySettings":             reflect.TypeFor[administration.LibrarySettings](),
		"administration.DVRDefaults":                 reflect.TypeFor[administration.DVRDefaults](),
		"livechannels.LiveDefaults":                  reflect.TypeFor[livechannels.LiveDefaults](),
		"livechannels.LiveSourceSettings":            reflect.TypeFor[livechannels.LiveSourceSettings](),
		"metadata.MetadataSettingsUpdate":            reflect.TypeFor[metadata.MetadataSettingsUpdate](),
		"downloads.Settings":                         reflect.TypeFor[downloads.Settings](),
		"operations.NotificationSettings":            reflect.TypeFor[operations.NotificationSettings](),
	}
	seen := map[string]bool{}
	for name, typ := range roots {
		inventoryFields(typ, name, func(key string) {
			seen[key] = true
			consumer, ok := settingConsumers[key]
			if !ok {
				for pattern, reason := range informationalSettings {
					if matched, _ := filepath.Match(pattern, key); matched && reason != "" {
						return
					}
				}
				t.Errorf("stored setting %s has no reviewed runtime consumer", key)
				return
			}
			if consumer.effect == "" {
				t.Errorf("%s has no stated effect", key)
			}
			path := filepath.Join("../..", consumer.file)
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Error(key, err)
				return
			}
			found := false
			ast.Inspect(file, func(node ast.Node) bool {
				if strings.HasPrefix(consumer.witness, "literal:") {
					if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
						value, _ := strconv.Unquote(literal.Value)
						found = found || strings.Contains(value, strings.TrimPrefix(consumer.witness, "literal:"))
					}
				}
				if selector, ok := node.(*ast.SelectorExpr); ok {
					found = found || selector.Sel.Name == consumer.witness
				}
				return true
			})
			if !found {
				t.Errorf("%s lost its runtime witness %s in %s", key, consumer.witness, consumer.file)
			}
		})
	}
	for key := range settingConsumers {
		if !seen[key] {
			t.Errorf("stale consumer entry %s", key)
		}
	}
}
