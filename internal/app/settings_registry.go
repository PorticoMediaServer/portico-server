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
}

// canonicalSettingRegistry owns the public settings defaults, revision
// participation, authority scope, and runtime accountability. Database seed
// rows remain compatibility input and are not the product contract.
var canonicalSettingRegistry = map[string]settingGroupDefinition{
	"server": {
		Defaults:        map[string]any{"friendlyName": "Portico", "operatorNote": ""},
		Scope:           "server-owner",
		RuntimeConsumer: "server identity and presentation", Revisioned: true,
	},
	"devices": {
		Defaults:        map[string]any{"requireTrustedDevices": false, "quickConnectApprovalMode": "allUsers"},
		Scope:           "server-owner",
		RuntimeConsumer: "session admission and Quick Connect approval", Revisioned: true,
	},
	"dlna": {
		Defaults:        map[string]any{"enabled": false, "friendlyName": "", "advertiseUrl": "", "exposedLibraries": []any{}, "reportTimeline": true},
		Scope:           "server-owner",
		RuntimeConsumer: "DLNA discovery and media exposure", Revisioned: true,
	},
	"dvr": {
		Scope: "server-owner",
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
		Scope: "server-owner",
		Defaults: map[string]any{
			"scanAutomatically": true, "scanOnFilesystemChanges": true, "emptyTrashAfterScan": false,
			"allowMediaDeletion": false, "trashRetentionDays": 30, "generateVideoPreview": "scheduled",
			"chapterThumbnailMode": "scheduled", "analyzeOnScan": true, "trickplayOnScan": false,
			"trickplayIntervalSeconds": 0, "trickplayTileWidth": 160, "trickplayMaxTiles": 240,
		},
		RuntimeConsumer: "library scanner, analysis, trickplay, and trash policy", Revisioned: true,
	},
	"languages": {
		Defaults:        map[string]any{"audio": "English", "subtitle": "English", "subtitleMode": "manual", "preferForcedSubs": true},
		Scope:           "server-owner",
		RuntimeConsumer: "server playback language selection", Revisioned: true,
	},
	"metadataAgents": {
		Scope: "server-owner",
		Defaults: map[string]any{
			"movies": "TMDB", "tv": "TMDB", "anime": "AniList", "music": "MusicBrainz",
			"localNFO": true, "embeddedTags": true, "cacheOriginalArtwork": false,
			"refreshDays": 7, "metadataLanguage": "en-US",
		},
		RuntimeConsumer: "metadata matching, enrichment, artwork, and refresh", Revisioned: true,
	},
	"network": {
		Defaults:        map[string]any{"secureConnections": "preferred", "lanNetworks": "", "customAccessUrls": ""},
		Scope:           "server-owner",
		RuntimeConsumer: "listener connection policy and advertised access routes", Revisioned: true,
	},
	"notifications": {
		Defaults:        map[string]any{"enabled": true, "minAlertLevel": "warn"},
		Scope:           "server-owner",
		RuntimeConsumer: "operational notification policy", Revisioned: true,
	},
	"viewerFeedback": {
		Defaults:        map[string]any{"enabled": true, "ownerResponsesEnabled": true, "feedbackRetentionDays": 365, "notificationRetentionDays": 180},
		Scope:           "server-owner",
		RuntimeConsumer: "viewer feedback and private notification retention", Revisioned: true,
	},
	"optimizedVersions": {
		Scope: "server-owner",
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
		Scope: "server-owner",
		Defaults: map[string]any{
			"playbackDetailDays": 0, "playbackHistoryDays": 0, "auditHistoryDays": 90,
			"diagnosticHistoryDays": 30, "clientDiagnosticHistoryDays": 30, "jobHistoryDays": 30,
			"authRequestDays": 14, "deviceIPDays": 30,
		},
		RuntimeConsumer: "privacy and operational history pruning", Revisioned: true,
	},
	"scheduledTasks": {
		Scope: "server-owner",
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
		Scope: "server-owner",
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
		Scope:           "server-owner",
		RuntimeConsumer: "diagnostic logging and client log admission", Revisioned: true,
	},
}

func init() {
	// Materialize field accountability once all package variables have been
	// initialized. writableSettingSchemas remains the decoder compatibility
	// projection; every supported field must also exist here with its product
	// semantics and is enforced by registry completeness tests.
	for group, definition := range canonicalSettingRegistry {
		definition.Fields = map[string]settingFieldDefinition{}
		for field, kind := range writableSettingSchemas[group] {
			definition.Fields[field] = settingFieldDefinition{
				Default: definition.Defaults[field], Type: kind,
				Validation:        "validateSettingFieldValue+validateSettingGroupPolicy",
				Dependencies:      settingFieldDependencies(group, field),
				EffectiveValue:    "stored-value-or-canonical-default",
				RuntimeConsumer:   definition.RuntimeConsumer,
				OperationalStatus: "active", Revision: 1,
			}
		}
		if group == "metadataAgents" {
			for _, field := range []string{"tmdbReadAccessToken", "tmdbAPIKey"} {
				definition.Fields[field] = settingFieldDefinition{
					Type: settingFieldObject, Validation: "secret replacement/clear envelope",
					Secret: true, EffectiveValue: "encrypted secret setting or absent",
					RuntimeConsumer: definition.RuntimeConsumer, OperationalStatus: "active", Revision: 1,
				}
			}
		}
		canonicalSettingRegistry[group] = definition
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
