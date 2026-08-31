package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type settingGroupDefinition struct {
	Defaults        map[string]any
	Scope           string
	RuntimeConsumer string
	Revisioned      bool
	Fields          map[string]settingFieldDefinition
}

type settingFieldDefinition struct {
	Default           any
	Type              settingFieldType
	Validation        string
	Dependencies      []string
	Secret            bool
	EffectiveValue    string
	RuntimeConsumer   string
	OperationalStatus string
	Revision          int
	OutcomeTest       string
	SaveMode          string
	ApplicationMode   string
	Permission        string
	Capability        string
	AuditClass        string
	RetentionClass    string
	ContractRevision  string
}

// canonicalSettingRegistry owns the public settings defaults, revision
// participation, authority scope, and runtime accountability. Database rows
// are storage input, not the product contract.
var canonicalSettingRegistry = map[string]settingGroupDefinition{
	"server": {
		Defaults:        map[string]any{"friendlyName": "Portico", "operatorNote": ""},
		Scope:           "server",
		RuntimeConsumer: "server identity and presentation", Revisioned: true,
	},
	"devices": {
		Defaults:        map[string]any{"requireTrustedDevices": false, "quickConnectApprovalMode": "allUsers"},
		Scope:           "server",
		RuntimeConsumer: "session admission and Quick Connect approval", Revisioned: true,
	},
	"dlna": {
		Defaults:        map[string]any{"enabled": false, "friendlyName": "", "advertiseUrl": "", "exposedLibraries": []any{}, "reportTimeline": true},
		Scope:           "server",
		RuntimeConsumer: "DLNA discovery and media exposure", Revisioned: true,
	},
	"dvr": {
		Scope: "server",
		Defaults: map[string]any{
			"defaultStartPaddingMinutes": 0, "defaultEndPaddingMinutes": 0, "defaultRetentionDays": 30,
			"defaultFolder": "", "defaultMaxRecordingsPerSeries": 0, "defaultGuideRefreshIntervalHours": 12,
			"defaultGuideRequireEpg": false, "guideChannelAutoMatch": true,
			"defaultRuleRequiredKeywords": []any{}, "defaultRuleBlockedKeywords": []any{},
			"defaultRuleAllowedChannels": []any{}, "defaultRuleBlockedChannels": []any{},
			"recordingPathTemplate": "{folder}/{year}/{month}/{title}-{start}", "saveNFO": false,
			"saveImageSidecars": false, "convertRecordings": false, "recordingProfile": "copy", "preserveAllStreams": true,
		},
		RuntimeConsumer: "DVR scheduling, recording, guide, and retention", Revisioned: true,
	},
	"library": {
		Scope: "server",
		Defaults: map[string]any{
			"scanAutomatically": true, "scanOnFilesystemChanges": true, "emptyTrashAfterScan": false,
			"allowMediaDeletion": false, "trashRetentionDays": 30, "generateVideoPreview": "scheduled",
			"chapterThumbnailMode": "scheduled", "analysisTier": "basic", "trickplayOnScan": false,
			"readLocalMetadata": false, "readExternalSubtitlesAndLyrics": false, "discoverLocalArtwork": false,
			"fetchDescriptiveMetadata": false, "probeStreams": false, "readEmbeddedTags": false,
			"readEmbeddedIndexes": false, "generateRepresentativeThumbnail": false, "generateTrickplay": false,
			"extractSelectedEmbeddedAssets": false,
			"validateSeekBehavior":          false,
			"fullFileChecksum":              false, "generateChapterThumbnails": false, "generateWaveforms": false,
			"analyzeLoudness": false, "sonicFingerprinting": false,
			"detectSegments":                false,
			"extractAllEmbeddedAttachments": false,
			"analyzeSTRMTarget":             false,
			"trickplayIntervalSeconds":      0, "trickplayTileWidth": 160, "trickplayMaxTiles": 240,
		},
		RuntimeConsumer: "library scanner, analysis, trickplay, and trash policy", Revisioned: true,
	},
	"languages": {
		Defaults:        map[string]any{"audio": "English", "subtitle": "English", "subtitleMode": "manual", "preferForcedSubs": true},
		Scope:           "server",
		RuntimeConsumer: "server playback language selection", Revisioned: true,
	},
	"metadataAgents": {
		Scope: "server",
		Defaults: map[string]any{
			"movies": "TMDB", "moviesFallback": "TVDB", "tv": "TMDB", "tvFallback": "TVDB", "anime": "AniList", "music": "MusicBrainz",
			"localNFO": true, "embeddedTags": true, "cacheOriginalArtwork": false,
			"refreshDays": 7, "metadataLanguage": "en-US",
		},
		RuntimeConsumer: "metadata matching, enrichment, artwork, and refresh", Revisioned: true,
	},
	"network": {
		Defaults:        map[string]any{"secureConnections": "preferred", "lanNetworks": "", "customAccessUrls": ""},
		Scope:           "server",
		RuntimeConsumer: "listener connection policy and advertised access routes", Revisioned: true,
	},
	"notifications": {
		Defaults:        map[string]any{"enabled": true, "minAlertLevel": "warn"},
		Scope:           "server",
		RuntimeConsumer: "operational notification policy", Revisioned: true,
	},
	"viewerFeedback": {
		Defaults:        map[string]any{"enabled": true, "ownerResponsesEnabled": true, "feedbackRetentionDays": 365, "notificationRetentionDays": 180},
		Scope:           "server",
		RuntimeConsumer: "viewer feedback and private notification retention", Revisioned: true,
	},
	"optimizedVersions": {
		Scope: "server",
		Defaults: map[string]any{
			"defaultProfile": "universal-720p",
			"templates": []any{
				map[string]any{"id": "preset-universal-1080p", "name": "Universal 1080p", "profile": "universal-1080p", "enabled": true},
				map[string]any{"id": "preset-universal-720p", "name": "Universal 720p", "profile": "universal-720p", "enabled": true},
				map[string]any{"id": "preset-universal-480p", "name": "Universal 480p", "profile": "universal-480p", "enabled": true},
				map[string]any{"id": "preset-efficient-4k", "name": "Efficient 4K", "profile": "efficient-4k", "enabled": true},
				map[string]any{"id": "preset-efficient-1080p", "name": "Efficient 1080p", "profile": "efficient-1080p", "enabled": true},
				map[string]any{"id": "preset-efficient-720p", "name": "Efficient 720p", "profile": "efficient-720p", "enabled": true},
				map[string]any{"id": "preset-maximum-compression-source", "name": "Maximum Compression Source Size", "profile": "maximum-compression-source", "enabled": true},
				map[string]any{"id": "preset-maximum-compression-1080p", "name": "Maximum Compression 1080p", "profile": "maximum-compression-1080p", "enabled": true},
			},
			"preferOptimizedPlayback": false, "storageDirectory": "", "maxConcurrentJobs": 1,
			"autoDelete": false, "retentionDays": 0, "maxPerItem": 3, "maxStorageMB": 0,
		},
		RuntimeConsumer: "optimized-version generation, selection, storage, and cleanup", Revisioned: true,
	},
	"retention": {
		Scope: "server",
		Defaults: map[string]any{
			"playbackDetailDays": 0, "playbackHistoryDays": 0, "auditHistoryDays": 90,
			"diagnosticHistoryDays": 30, "clientDiagnosticHistoryDays": 30, "jobHistoryDays": 30,
			"authRequestDays": 14, "deviceIPDays": 30,
		},
		RuntimeConsumer: "privacy and operational history pruning", Revisioned: true,
	},
	"scheduledTasks": {
		Scope: "server",
		Defaults: map[string]any{
			"enabled": true, "maintenanceWindow": "overnight", "maintenanceDays": "every-day", "maintenanceTimezone": "UTC", "startHour": 2, "endHour": 5,
			"backupDatabase": true, "backupCadence": "daily", "backupRetentionDays": 14,
			"scanLibraries": true, "libraryScanCadence": "daily", "libraryScanIntervalHours": 24,
			"refreshMetadata": false, "metadataRefreshCadence": "daily", "metadataRefreshDays": 14,
			"analyzeMedia": true, "analysisCadence": "daily", "emptyTrash": false, "trashRetentionDays": 30,
			"trickplayRetentionDays": 14, "trickplayMaxStorageMB": 0, "trickplayIntervalSeconds": 0,
			"trickplayTileWidth": 160, "trickplayMaxTiles": 240,
			"taskTriggers": map[string]any{
				"database-backup":  map[string]any{"enabled": true, "intervalHours": 24},
				"library-scan":     map[string]any{"enabled": true, "intervalHours": 24},
				"metadata-refresh": map[string]any{"enabled": false, "intervalHours": 24},
				"media-analysis":   map[string]any{"enabled": true, "intervalHours": 24},
			},
		},
		RuntimeConsumer: "durable maintenance scheduler", Revisioned: true,
	},
	"transcoder": {
		Scope: "server",
		Defaults: map[string]any{
			"enabled": true, "planningPolicy": "maximum_fidelity", "temporaryDirectory": "", "maxConcurrentSessions": 2,
			"x264Preset": "veryfast", "throttleBufferSeconds": 60, "playedRetentionSeconds": 300,
			"hardwareAcceleration": false, "hardwareEncoding": false, "hardwareDecodeHEVC": true, "hardwareDevice": "auto",
			"maxHardwareSessions": 2, "maxSoftwareSessions": 0, "maxBackgroundSessions": 1,
			"hdrToneMapping": true, "hdrToneMappingAlgorithm": "hable", "directStreamRemux": true,
		},
		RuntimeConsumer: "playback decision and FFmpeg execution", Revisioned: true,
	},
	"troubleshooting": {
		Defaults:        map[string]any{"clientLogUploads": false, "logLevel": "info", "debugDurationMinutes": 60},
		Scope:           "server",
		RuntimeConsumer: "diagnostic logging and client log admission", Revisioned: true,
	},
}

func init() {
	// Materialize field accountability once all package variables have been
	// initialized. writableSettingSchemas remains the wire decoder
	// projection; every supported field must also exist here with its product
	// semantics and is enforced by registry completeness tests.
	for group, definition := range canonicalSettingRegistry {
		definition.Fields = map[string]settingFieldDefinition{}
		for field, kind := range writableSettingSchemas[group] {
			outcome, saveMode, applicationMode, auditClass, retentionClass := settingFieldContractMetadata(group, field)
			definition.Fields[field] = settingFieldDefinition{
				Default: definition.Defaults[field], Type: kind,
				Validation:        settingFieldValidationContract(group, field, kind),
				Dependencies:      settingFieldDependencies(group, field),
				EffectiveValue:    "stored-value-or-canonical-default",
				RuntimeConsumer:   definition.RuntimeConsumer,
				OperationalStatus: "supported", Revision: 1, OutcomeTest: outcome,
				SaveMode: saveMode, ApplicationMode: applicationMode, Permission: "manageServer", Capability: "server-settings-write",
				AuditClass: auditClass, RetentionClass: retentionClass, ContractRevision: "settings-operations.v1",
			}
		}
		if group == "metadataAgents" {
			for _, field := range []string{"tmdbReadAccessToken", "tmdbAPIKey", "tvdbAPIKey"} {
				definition.Fields[field] = settingFieldDefinition{
					Type: settingFieldObject, Validation: "secret replacement/clear envelope",
					Secret: true, EffectiveValue: "encrypted secret setting or absent",
					RuntimeConsumer: definition.RuntimeConsumer, OperationalStatus: "supported", Revision: 1,
					OutcomeTest: settingGroupOutcomeTest(group), SaveMode: "explicit-apply", ApplicationMode: "next-operation",
					Permission: "manageServer", Capability: "server-settings-write", AuditClass: "secret-setting", RetentionClass: "secret-setting", ContractRevision: "settings-operations.v1",
				}
			}
		}
		canonicalSettingRegistry[group] = definition
	}
}

func settingFieldValidationContract(group, field string, kind settingFieldType) string {
	prefix := "type:" + string(kind)
	switch kind {
	case settingFieldNumber:
		prefix = "type:integer"
	case settingFieldString:
		prefix += ";length:0..8192"
	case settingFieldStringList:
		prefix += ";items:string;count:0..512;item-length:1..256;unique:true"
	case settingFieldObject:
		prefix += ";additional-properties:field-policy"
	case settingFieldOptimizedTemplates:
		prefix += ";items:optimized-version-template"
	}
	switch group + "." + field {
	case "server.friendlyName":
		return prefix + ";length:1..120;trim:required"
	case "server.operatorNote":
		return prefix + ";length:0..500"
	case "metadataAgents.movies", "metadataAgents.moviesFallback", "metadataAgents.tv", "metadataAgents.tvFallback":
		return prefix + ";enum:TMDB|TVDB|None"
	case "metadataAgents.anime":
		return prefix + ";enum:AniList|TMDB|None"
	case "metadataAgents.music":
		return prefix + ";enum:MusicBrainz|None"
	case "network.secureConnections":
		return prefix + ";enum:preferred|required|disabled"
	case "languages.subtitleMode":
		return prefix + ";enum:manual|always|foreignAudio"
	case "devices.quickConnectApprovalMode":
		return prefix + ";enum:allUsers|ownerOnly"
	case "retention.playbackDetailDays", "retention.playbackHistoryDays", "retention.auditHistoryDays", "retention.diagnosticHistoryDays", "retention.clientDiagnosticHistoryDays", "retention.jobHistoryDays", "retention.authRequestDays", "retention.deviceIPDays":
		return prefix + ";range:0..36500"
	case "metadataAgents.refreshDays", "scheduledTasks.backupRetentionDays", "scheduledTasks.libraryScanIntervalHours", "scheduledTasks.metadataRefreshDays", "dvr.defaultRetentionDays":
		return prefix + ";range:1..36500"
	case "optimizedVersions.maxConcurrentJobs":
		return prefix + ";range:1..4"
	case "optimizedVersions.maxPerItem":
		return prefix + ";range:1..36500"
	case "dvr.defaultGuideRefreshIntervalHours":
		return prefix + ";range:1..168"
	case "dvr.defaultMaxRecordingsPerSeries":
		return prefix + ";range:0..10000"
	case "dvr.defaultStartPaddingMinutes", "dvr.defaultEndPaddingMinutes":
		return prefix + ";range:0..1440"
	case "viewerFeedback.feedbackRetentionDays", "viewerFeedback.notificationRetentionDays":
		return prefix + ";range:7..730"
	case "library.trashRetentionDays", "scheduledTasks.trashRetentionDays", "scheduledTasks.trickplayRetentionDays", "optimizedVersions.retentionDays":
		return prefix + ";range:0..36500"
	case "library.trickplayIntervalSeconds", "scheduledTasks.trickplayIntervalSeconds":
		return prefix + ";range:0..3600"
	case "library.trickplayTileWidth", "scheduledTasks.trickplayTileWidth":
		return prefix + ";range:96..640"
	case "library.trickplayMaxTiles", "scheduledTasks.trickplayMaxTiles":
		return prefix + ";range:24..2000"
	case "optimizedVersions.maxStorageMB", "scheduledTasks.trickplayMaxStorageMB":
		return prefix + ";range:0..1000000000"
	case "troubleshooting.debugDurationMinutes":
		return prefix + ";range:5..1440"
	case "scheduledTasks.startHour", "scheduledTasks.endHour":
		return prefix + ";range:0..23;depends:maintenanceWindow"
	case "scheduledTasks.maintenanceWindow":
		return prefix + ";enum:overnight|late-night|always|custom"
	case "scheduledTasks.maintenanceDays":
		return prefix + ";enum:every-day|weekdays|weekends"
	case "scheduledTasks.maintenanceTimezone":
		return prefix + ";format:iana-timezone"
	case "scheduledTasks.backupCadence", "scheduledTasks.libraryScanCadence", "scheduledTasks.metadataRefreshCadence", "scheduledTasks.analysisCadence":
		return prefix + ";enum:hourly|daily|weekly|monthly|custom"
	case "scheduledTasks.taskTriggers":
		return prefix + ";keys:known-task-id;enabled:boolean;intervalHours:1..8760"
	case "dvr.recordingProfile":
		return prefix + ";enum:copy|h264-1080p-8m|h264-720p-4m"
	case "dvr.defaultFolder":
		return prefix + ";format:safe-folder;length:0..80"
	case "dvr.recordingPathTemplate":
		return prefix + ";format:dvr-recording-path-template"
	case "library.generateVideoPreview", "library.chapterThumbnailMode":
		return prefix + ";enum:never|scheduled|on-scan"
	case "library.analysisTier":
		return prefix + ";enum:file_list_only|basic|complete|custom"
	case "metadataAgents.metadataLanguage":
		return prefix + ";format:bcp-47"
	case "network.lanNetworks":
		return prefix + ";format:cidr-list"
	case "network.customAccessUrls", "dlna.advertiseUrl":
		return prefix + ";format:http-url"
	case "optimizedVersions.defaultProfile":
		return prefix + ";enum:installed-optimized-profile;depends:templates"
	case "optimizedVersions.storageDirectory", "transcoder.temporaryDirectory":
		return prefix + ";format:absolute-path-or-blank"
	case "notifications.minAlertLevel":
		return prefix + ";enum:info|warn|error"
	case "troubleshooting.logLevel":
		return prefix + ";enum:debug|info|warn"
	case "transcoder.maxConcurrentSessions", "transcoder.maxHardwareSessions", "transcoder.maxSoftwareSessions", "transcoder.maxBackgroundSessions":
		return prefix + ";range:0..64"
	case "transcoder.throttleBufferSeconds":
		return prefix + ";range:10..3600"
	case "transcoder.playedRetentionSeconds":
		return prefix + ";range:0..86400"
	case "transcoder.x264Preset":
		return prefix + ";enum:ultrafast|superfast|veryfast|faster|fast|medium|slow|slower"
	case "transcoder.planningPolicy":
		return prefix + ";enum:maximum_fidelity|maximum_compatibility|minimize_server_work"
	case "transcoder.hardwareDevice":
		return prefix + ";enum:auto|videotoolbox|vaapi|qsv|cuda|nvenc|nvidia|amf"
	case "transcoder.hdrToneMappingAlgorithm":
		return prefix + ";enum:clip|linear|gamma|hable|reinhard|mobius"
	default:
		return prefix
	}
}

func settingGroupHealthSource(group string) string {
	switch group {
	case "server":
		return "/api/dashboard/activity"
	case "devices":
		return "/api/devices"
	case "dlna":
		return "/api/dlna/status"
	case "dvr":
		return "/api/dvr/status"
	case "library":
		return "/api/libraries/{id}/scan-operations"
	case "languages", "transcoder":
		return "/api/system/capabilities"
	case "metadataAgents":
		return "/api/metadata/health"
	case "network":
		return "/api/remote-access/health"
	case "notifications":
		return "/api/dashboard"
	case "viewerFeedback":
		return "/api/feedback/capabilities"
	case "optimizedVersions":
		return "/api/activity"
	case "retention", "troubleshooting":
		return "/api/system/diagnostics"
	case "scheduledTasks":
		return "/api/tasks"
	default:
		return "not-reported"
	}
}

func settingFieldContractMetadata(group, field string) (string, string, string, string, string) {
	// Every published field points at a concrete runtime test that exercises
	// the owning consumer. A field without reviewed evidence is not published.
	outcome := settingGroupOutcomeTest(group)
	auditClass := "settings"
	retentionClass := "settings"
	if group == "troubleshooting" || group == "retention" {
		retentionClass = "diagnostic-policy"
	}
	applicationMode := "next-operation"
	if group == "scheduledTasks" || group == "retention" || group == "optimizedVersions" {
		applicationMode = "scheduled"
	}
	return outcome, "explicit-apply", applicationMode, auditClass, retentionClass
}

func settingGroupOutcomeTest(group string) string {
	switch group {
	case "server":
		return "TestServerIdentitySettingsStoreOperatorNoteAndResetIdentity"
	case "devices":
		return "TestQuickConnectExchangeReceiptRecoveryRejectsAtTTLBoundary"
	case "dlna":
		return "TestDLNASettingsUseFixedDiscoveryCompatibilityPolicy"
	case "dvr":
		return "TestDVRTimerDefaultsApplyToRulesAndOneOffRecordings"
	case "library":
		return "TestLibraryPreviewGenerationSettings"
	case "languages":
		return "TestApplyLanguagePreferencesPrioritizesUserAudioAndServerSubtitles"
	case "metadataAgents":
		return "TestMetadataSettingsStoreAndRedactTMDBCredentials"
	case "network":
		return "TestNetworkSettingsParsesTrustedRangesAndAccessURLs"
	case "notifications", "viewerFeedback":
		return "TestViewerNotificationStreamInterleavingNeverExposesPrivatePayload"
	case "optimizedVersions":
		return "TestOptimizedVersionSettingsUseCustomStorageDirectory"
	case "retention":
		return "TestRetentionSettingsExposeBoundedOperationalDefaultsAndAcceptIndependentPeriods"
	case "scheduledTasks":
		return "TestScheduledTasksQueueCleanupThroughMaintenanceJobs"
	case "transcoder":
		return "TestPlannedTranscodeAdmissionEnforcesClassAndBackgroundLimits"
	case "troubleshooting":
		return "TestClientLogUploadIsBoundedAndSettingGated"
	default:
		return ""
	}
}

func settingFieldDependencies(group, field string) []string {
	switch group + "." + field {
	case "optimizedVersions.defaultProfile":
		return []string{"optimizedVersions.templates"}
	case "optimizedVersions.retentionDays":
		return []string{"optimizedVersions.autoDelete"}
	case "transcoder.hardwareEncoding", "transcoder.hardwareDecodeHEVC", "transcoder.hardwareDevice", "transcoder.maxHardwareSessions":
		return []string{"transcoder.hardwareAcceleration"}
	case "scheduledTasks.startHour", "scheduledTasks.endHour":
		return []string{"scheduledTasks.maintenanceWindow"}
	case "dvr.recordingProfile", "dvr.preserveAllStreams":
		return []string{"dvr.convertRecordings"}
	default:
		return []string{}
	}
}

type canonicalSettingRepair struct {
	Key      string
	Raw      string
	Reason   string
	Repaired string
}

// repairCanonicalSettingsContext isolates both syntactically malformed JSON
// and semantically invalid public groups. Exact rejected bytes are retained in
// the private quarantine ledger, while the active row is removed or replaced
// with reviewed defaults plus every independently valid supported field.
func (s *Server) repairCanonicalSettingsContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.queryUserRead(ctx, `SELECT key, value_json FROM settings ORDER BY key`)
	if err != nil {
		return err
	}
	repairs := []canonicalSettingRepair{}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			rows.Close()
			return err
		}
		definition, public := canonicalSettingRegistry[key]
		var decoded any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			repair := canonicalSettingRepair{Key: key, Raw: raw, Reason: "invalid_json"}
			if public {
				bytes, _ := json.Marshal(definition.Defaults)
				repair.Repaired = string(bytes)
			}
			repairs = append(repairs, repair)
			continue
		}
		if !public {
			continue
		}
		group, ok := decoded.(map[string]any)
		if !ok {
			bytes, _ := json.Marshal(definition.Defaults)
			repairs = append(repairs, canonicalSettingRepair{Key: key, Raw: raw, Reason: "group_not_object", Repaired: string(bytes)})
			continue
		}
		repaired := map[string]any{}
		invalid := false
		for field, value := range group {
			kind, supported := writableSettingSchemas[key][field]
			if !supported || validateSettingFieldValue(key, field, kind, value) != nil {
				invalid = true
				continue
			}
			repaired[field] = value
		}
		if !invalid {
			candidate := cloneSettingMap(repaired)
			for field, value := range definition.Defaults {
				if _, present := candidate[field]; !present {
					candidate[field] = value
				}
			}
			if validateSettingGroupPolicy(key, candidate) != nil {
				invalid = true
				repaired = cloneSettingMap(definition.Defaults)
			}
		}
		if !invalid {
			continue
		}
		for field, value := range definition.Defaults {
			if _, present := repaired[field]; !present {
				repaired[field] = value
			}
		}
		if validateSettingGroupPolicy(key, repaired) != nil {
			repaired = cloneSettingMap(definition.Defaults)
		}
		bytes, marshalErr := json.Marshal(repaired)
		if marshalErr != nil {
			rows.Close()
			return marshalErr
		}
		repairs = append(repairs, canonicalSettingRepair{Key: key, Raw: raw, Reason: "invalid_public_group", Repaired: string(bytes)})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(repairs) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withUserTxTagged(ctx, []string{"settings"}, func(tx *sql.Tx) error {
		for _, repair := range repairs {
			var result sql.Result
			var err error
			if repair.Repaired == "" {
				result, err = tx.ExecContext(ctx, `DELETE FROM settings WHERE key = ? AND value_json = ?`, repair.Key, repair.Raw)
			} else {
				result, err = tx.ExecContext(ctx, `UPDATE settings SET value_json = ?, updated_at = ? WHERE key = ? AND value_json = ?`, repair.Repaired, now, repair.Key, repair.Raw)
			}
			if err != nil {
				return fmt.Errorf("repair setting %s: %w", repair.Key, err)
			}
			changed, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if changed == 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO settings_quarantine (id, key, value_json, reason, quarantined_at) VALUES (?, ?, ?, ?, ?)`, randomID("setting_quarantine"), repair.Key, repair.Raw, repair.Reason, now); err != nil {
				return fmt.Errorf("quarantine setting %s: %w", repair.Key, err)
			}
		}
		return nil
	})
}

func applyCanonicalSettingDefaults(settings map[string]any) map[string]any {
	result := make(map[string]any, len(canonicalSettingRegistry))
	for key, definition := range canonicalSettingRegistry {
		group := cloneSettingMap(definition.Defaults)
		if stored, ok := settings[key].(map[string]any); ok {
			for field, value := range stored {
				if _, supported := writableSettingSchemas[key][field]; supported {
					group[field] = value
				}
			}
		}
		result[key] = group
	}
	return result
}

func cloneSettingMap(source map[string]any) map[string]any {
	bytes, _ := json.Marshal(source)
	var clone map[string]any
	_ = json.Unmarshal(bytes, &clone)
	return clone
}
