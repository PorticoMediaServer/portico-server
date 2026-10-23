package operations

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
)

// Server settings have one typed row, ordered list members and log retention
// rows. Domain-owned identity/playback fields stay with their own tables.
func readStoredSettings(tx *sql.Tx, v *Settings) error {
	var rawAnalysisStorageGiB, rawAnalysisStageStorageMiB, rawSubtitleTrackLimit, rawSubtitleFileLimitMiB, rawHardwareBackend, rawHardwareDevice, rawHDRToneMapping, rawHDRToneMappingAlgorithm, rawX264Preset, rawDirectStreamRemux, rawPlanningPolicy, rawThrottleBufferMs, rawPlayedRetentionMs, rawTemporaryDirectory, rawFFmpegPath, rawSessionStorageMiB, rawMaxConcurrentSessions, rawMaxHardwareSessions, rawMaxSoftwareSessions, rawMaxBackgroundSessions, rawDiagnosticDays, rawNotificationDays, rawJobDays, rawPlayHistoryDays, rawAPIRequestsPerMinute, rawCommunityActivityEnabled, rawUpdateChannel, rawUpdateFeedURL, rawMissingTitles, rawMissingTitlesDays, rawSegmentDetection, rawRemoteSignInPolicy, rawRemoteBitrateLimitKbps, rawConnectionLimit, rawAdvertiseURL, rawLANDiscoveryEnabled, rawDLNAEnabled, rawUploadCapacityKbps, rawPausedSessionTimeoutMinutes, rawAdvertisedInterface, rawOwnAddressEnabled, rawCustomCertificatePath, rawCustomCertificateKeyPath, rawCustomCertificateDomain, rawLogLevel any
	err := tx.QueryRow(`SELECT "analysisStorageGiB", "analysisStageStorageMiB", "subtitleTrackLimit", "subtitleFileLimitMiB", "hardwareBackend", "hardwareDevice", "hdrToneMapping", "hdrToneMappingAlgorithm", "x264Preset", "directStreamRemux", "planningPolicy", "throttleBufferMs", "playedRetentionMs", "temporaryDirectory", "ffmpegPath", "sessionStorageMiB", "maxConcurrentSessions", "maxHardwareSessions", "maxSoftwareSessions", "maxBackgroundSessions", "diagnosticDays", "notificationDays", "jobDays", "playHistoryDays", "apiRequestsPerMinute", "home.communityActivityEnabled", "updates.releaseChannel", "updates.feedUrl", "library.missingTitles", "library.missingTitlesDays", "library.segmentDetection", "remoteSignInPolicy", "remoteBitrateLimitKbps", "connectionLimit", "advertiseUrl", "lanDiscoveryEnabled", "dlnaEnabled", "uploadCapacityKbps", "pausedSessionTimeoutMinutes", "advertisedInterface", "ownAddressEnabled", "customCertificatePath", "customCertificateKeyPath", "customCertificateDomain", "logLevel" FROM server_settings WHERE singleton=1`).Scan(&rawAnalysisStorageGiB, &rawAnalysisStageStorageMiB, &rawSubtitleTrackLimit, &rawSubtitleFileLimitMiB, &rawHardwareBackend, &rawHardwareDevice, &rawHDRToneMapping, &rawHDRToneMappingAlgorithm, &rawX264Preset, &rawDirectStreamRemux, &rawPlanningPolicy, &rawThrottleBufferMs, &rawPlayedRetentionMs, &rawTemporaryDirectory, &rawFFmpegPath, &rawSessionStorageMiB, &rawMaxConcurrentSessions, &rawMaxHardwareSessions, &rawMaxSoftwareSessions, &rawMaxBackgroundSessions, &rawDiagnosticDays, &rawNotificationDays, &rawJobDays, &rawPlayHistoryDays, &rawAPIRequestsPerMinute, &rawCommunityActivityEnabled, &rawUpdateChannel, &rawUpdateFeedURL, &rawMissingTitles, &rawMissingTitlesDays, &rawSegmentDetection, &rawRemoteSignInPolicy, &rawRemoteBitrateLimitKbps, &rawConnectionLimit, &rawAdvertiseURL, &rawLANDiscoveryEnabled, &rawDLNAEnabled, &rawUploadCapacityKbps, &rawPausedSessionTimeoutMinutes, &rawAdvertisedInterface, &rawOwnAddressEnabled, &rawCustomCertificatePath, &rawCustomCertificateKeyPath, &rawCustomCertificateDomain, &rawLogLevel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if rawAnalysisStorageGiB != nil {
		v.AnalysisStorageGiB = int(rawAnalysisStorageGiB.(int64))
	}
	if rawAnalysisStageStorageMiB != nil {
		v.AnalysisStageStorageMiB = int(rawAnalysisStageStorageMiB.(int64))
	}
	if rawSubtitleTrackLimit != nil {
		v.SubtitleTrackLimit = int(rawSubtitleTrackLimit.(int64))
	}
	if rawSubtitleFileLimitMiB != nil {
		v.SubtitleFileLimitMiB = int(rawSubtitleFileLimitMiB.(int64))
	}
	if rawHardwareBackend != nil {
		v.HardwareBackend = rawHardwareBackend.(string)
	}
	if rawHardwareDevice != nil {
		v.HardwareDevice = rawHardwareDevice.(string)
	}
	if rawHDRToneMapping != nil {
		v.HDRToneMapping = rawHDRToneMapping.(int64) != 0
	}
	if rawHDRToneMappingAlgorithm != nil {
		v.HDRToneMappingAlgorithm = rawHDRToneMappingAlgorithm.(string)
	}
	if rawX264Preset != nil {
		v.X264Preset = rawX264Preset.(string)
	}
	if rawDirectStreamRemux != nil {
		v.DirectStreamRemux = rawDirectStreamRemux.(int64) != 0
	}
	if rawPlanningPolicy != nil {
		v.PlanningPolicy = rawPlanningPolicy.(string)
	}
	if rawThrottleBufferMs != nil {
		v.ThrottleBufferMs = int(rawThrottleBufferMs.(int64))
	}
	if rawPlayedRetentionMs != nil {
		v.PlayedRetentionMs = int(rawPlayedRetentionMs.(int64))
	}
	if rawTemporaryDirectory != nil {
		v.TemporaryDirectory = rawTemporaryDirectory.(string)
	}
	if rawFFmpegPath != nil {
		v.FFmpegPath = rawFFmpegPath.(string)
	}
	if rawSessionStorageMiB != nil {
		v.SessionStorageMiB = int(rawSessionStorageMiB.(int64))
	}
	if rawMaxConcurrentSessions != nil {
		v.MaxConcurrentSessions = int(rawMaxConcurrentSessions.(int64))
	}
	if rawMaxHardwareSessions != nil {
		v.MaxHardwareSessions = int(rawMaxHardwareSessions.(int64))
	}
	if rawMaxSoftwareSessions != nil {
		v.MaxSoftwareSessions = int(rawMaxSoftwareSessions.(int64))
	}
	if rawMaxBackgroundSessions != nil {
		v.MaxBackgroundSessions = int(rawMaxBackgroundSessions.(int64))
	}
	if rawDiagnosticDays != nil {
		v.DiagnosticDays = int(rawDiagnosticDays.(int64))
	}
	if rawNotificationDays != nil {
		v.NotificationDays = int(rawNotificationDays.(int64))
	}
	if rawJobDays != nil {
		v.JobDays = int(rawJobDays.(int64))
	}
	if rawPlayHistoryDays != nil {
		v.PlayHistoryDays = int(rawPlayHistoryDays.(int64))
	}
	if rawAPIRequestsPerMinute != nil {
		v.APIRequestsPerMinute = int(rawAPIRequestsPerMinute.(int64))
	}
	if rawCommunityActivityEnabled != nil {
		v.CommunityActivityEnabled = rawCommunityActivityEnabled.(int64) != 0
	}
	if rawUpdateChannel != nil {
		v.UpdateChannel = rawUpdateChannel.(string)
	}
	if rawUpdateFeedURL != nil {
		v.UpdateFeedURL = rawUpdateFeedURL.(string)
	}
	if rawMissingTitles != nil {
		v.MissingTitles = rawMissingTitles.(string)
	}
	if rawMissingTitlesDays != nil {
		v.MissingTitlesDays = int(rawMissingTitlesDays.(int64))
	}
	if rawSegmentDetection != nil {
		v.SegmentDetection = rawSegmentDetection.(string)
	}
	if rawRemoteSignInPolicy != nil {
		v.RemoteSignInPolicy = rawRemoteSignInPolicy.(string)
	}
	if rawRemoteBitrateLimitKbps != nil {
		v.RemoteBitrateLimitKbps = int(rawRemoteBitrateLimitKbps.(int64))
	}
	if rawConnectionLimit != nil {
		v.ConnectionLimit = int(rawConnectionLimit.(int64))
	}
	if rawAdvertiseURL != nil {
		v.AdvertiseURL = rawAdvertiseURL.(string)
	}
	if rawLANDiscoveryEnabled != nil {
		v.LANDiscoveryEnabled = rawLANDiscoveryEnabled.(int64) != 0
	}
	if rawDLNAEnabled != nil {
		v.DLNAEnabled = rawDLNAEnabled.(int64) != 0
	}
	if rawUploadCapacityKbps != nil {
		v.UploadCapacityKbps = int(rawUploadCapacityKbps.(int64))
	}
	if rawPausedSessionTimeoutMinutes != nil {
		v.PausedSessionTimeoutMinutes = int(rawPausedSessionTimeoutMinutes.(int64))
	}
	if rawAdvertisedInterface != nil {
		v.AdvertisedInterface = rawAdvertisedInterface.(string)
	}
	if rawOwnAddressEnabled != nil {
		v.OwnAddressEnabled = rawOwnAddressEnabled.(int64) != 0
	}
	if rawCustomCertificatePath != nil {
		v.CustomCertificatePath = rawCustomCertificatePath.(string)
	}
	if rawCustomCertificateKeyPath != nil {
		v.CustomCertificateKeyPath = rawCustomCertificateKeyPath.(string)
	}
	if rawCustomCertificateDomain != nil {
		v.CustomCertificateDomain = rawCustomCertificateDomain.(string)
	}
	if rawLogLevel != nil {
		v.LogLevel = rawLogLevel.(string)
	}
	rows, err := tx.Query(`SELECT setting,value FROM server_setting_lists ORDER BY setting,position`)
	if err != nil {
		return err
	}
	for _, key := range []string{"lanNetworks", "accessUrls", "trustedProxies"} {
		var explicit bool
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM server_setting_sources WHERE field=?) OR EXISTS(SELECT 1 FROM server_setting_lists WHERE setting=?)`, key, key).Scan(&explicit); err != nil {
			rows.Close()
			return err
		}
		if explicit {
			switch key {
			case "lanNetworks":
				v.LANNetworks = []string{}
			case "accessUrls":
				v.AccessURLs = []string{}
			case "trustedProxies":
				v.TrustedProxies = []string{}
			}
		}
	}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			rows.Close()
			return err
		}
		switch key {
		case "lanNetworks":
			v.LANNetworks = append(v.LANNetworks, value)
		case "accessUrls":
			v.AccessURLs = append(v.AccessURLs, value)
		case "trustedProxies":
			v.TrustedProxies = append(v.TrustedProxies, value)
		default:
			rows.Close()
			return ErrInvalid
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = tx.Query(`SELECT category,days FROM server_log_retention ORDER BY category`)
	if err != nil {
		return err
	}
	defer rows.Close()
	v.LogRetention = []LogRetention{}
	for rows.Next() {
		var entry LogRetention
		if err = rows.Scan(&entry.Category, &entry.Days); err != nil {
			return err
		}
		v.LogRetention = append(v.LogRetention, entry)
	}
	return rows.Err()
}

func writeStoredSettings(tx *sql.Tx, v Settings, now int64) error {
	_, err := tx.Exec(`INSERT INTO server_settings(singleton,updated_ms,"analysisStorageGiB", "analysisStageStorageMiB", "subtitleTrackLimit", "subtitleFileLimitMiB", "hardwareBackend", "hardwareDevice", "hdrToneMapping", "hdrToneMappingAlgorithm", "x264Preset", "directStreamRemux", "planningPolicy", "throttleBufferMs", "playedRetentionMs", "temporaryDirectory", "ffmpegPath", "sessionStorageMiB", "maxConcurrentSessions", "maxHardwareSessions", "maxSoftwareSessions", "maxBackgroundSessions", "diagnosticDays", "notificationDays", "jobDays", "playHistoryDays", "apiRequestsPerMinute", "home.communityActivityEnabled", "updates.releaseChannel", "updates.feedUrl", "library.missingTitles", "library.missingTitlesDays", "library.segmentDetection", "remoteSignInPolicy", "remoteBitrateLimitKbps", "connectionLimit", "advertiseUrl", "lanDiscoveryEnabled", "dlnaEnabled", "uploadCapacityKbps", "pausedSessionTimeoutMinutes", "advertisedInterface", "ownAddressEnabled", "customCertificatePath", "customCertificateKeyPath", "customCertificateDomain", "logLevel") VALUES(1,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(singleton) DO UPDATE SET updated_ms=excluded.updated_ms,"analysisStorageGiB"=excluded."analysisStorageGiB","analysisStageStorageMiB"=excluded."analysisStageStorageMiB","subtitleTrackLimit"=excluded."subtitleTrackLimit","subtitleFileLimitMiB"=excluded."subtitleFileLimitMiB","hardwareBackend"=excluded."hardwareBackend","hardwareDevice"=excluded."hardwareDevice","hdrToneMapping"=excluded."hdrToneMapping","hdrToneMappingAlgorithm"=excluded."hdrToneMappingAlgorithm","x264Preset"=excluded."x264Preset","directStreamRemux"=excluded."directStreamRemux","planningPolicy"=excluded."planningPolicy","throttleBufferMs"=excluded."throttleBufferMs","playedRetentionMs"=excluded."playedRetentionMs","temporaryDirectory"=excluded."temporaryDirectory","ffmpegPath"=excluded."ffmpegPath","sessionStorageMiB"=excluded."sessionStorageMiB","maxConcurrentSessions"=excluded."maxConcurrentSessions","maxHardwareSessions"=excluded."maxHardwareSessions","maxSoftwareSessions"=excluded."maxSoftwareSessions","maxBackgroundSessions"=excluded."maxBackgroundSessions","diagnosticDays"=excluded."diagnosticDays","notificationDays"=excluded."notificationDays","jobDays"=excluded."jobDays","playHistoryDays"=excluded."playHistoryDays","apiRequestsPerMinute"=excluded."apiRequestsPerMinute","home.communityActivityEnabled"=excluded."home.communityActivityEnabled","updates.releaseChannel"=excluded."updates.releaseChannel","updates.feedUrl"=excluded."updates.feedUrl","library.missingTitles"=excluded."library.missingTitles","library.missingTitlesDays"=excluded."library.missingTitlesDays","library.segmentDetection"=excluded."library.segmentDetection","remoteSignInPolicy"=excluded."remoteSignInPolicy","remoteBitrateLimitKbps"=excluded."remoteBitrateLimitKbps","connectionLimit"=excluded."connectionLimit","advertiseUrl"=excluded."advertiseUrl","lanDiscoveryEnabled"=excluded."lanDiscoveryEnabled","dlnaEnabled"=excluded."dlnaEnabled","uploadCapacityKbps"=excluded."uploadCapacityKbps","pausedSessionTimeoutMinutes"=excluded."pausedSessionTimeoutMinutes","advertisedInterface"=excluded."advertisedInterface","ownAddressEnabled"=excluded."ownAddressEnabled","customCertificatePath"=excluded."customCertificatePath","customCertificateKeyPath"=excluded."customCertificateKeyPath","customCertificateDomain"=excluded."customCertificateDomain","logLevel"=excluded."logLevel"`, now, v.AnalysisStorageGiB, v.AnalysisStageStorageMiB, v.SubtitleTrackLimit, v.SubtitleFileLimitMiB, v.HardwareBackend, v.HardwareDevice, v.HDRToneMapping, v.HDRToneMappingAlgorithm, v.X264Preset, v.DirectStreamRemux, v.PlanningPolicy, v.ThrottleBufferMs, v.PlayedRetentionMs, v.TemporaryDirectory, v.FFmpegPath, v.SessionStorageMiB, v.MaxConcurrentSessions, v.MaxHardwareSessions, v.MaxSoftwareSessions, v.MaxBackgroundSessions, v.DiagnosticDays, v.NotificationDays, v.JobDays, v.PlayHistoryDays, v.APIRequestsPerMinute, v.CommunityActivityEnabled, v.UpdateChannel, v.UpdateFeedURL, v.MissingTitles, v.MissingTitlesDays, v.SegmentDetection, v.RemoteSignInPolicy, v.RemoteBitrateLimitKbps, v.ConnectionLimit, v.AdvertiseURL, v.LANDiscoveryEnabled, v.DLNAEnabled, v.UploadCapacityKbps, v.PausedSessionTimeoutMinutes, v.AdvertisedInterface, v.OwnAddressEnabled, v.CustomCertificatePath, v.CustomCertificateKeyPath, v.CustomCertificateDomain, v.LogLevel)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM server_setting_lists`); err != nil {
		return err
	}
	for position, value := range v.LANNetworks {
		if _, err = tx.Exec(`INSERT INTO server_setting_lists VALUES(?,?,?)`, "lanNetworks", position, value); err != nil {
			return err
		}
	}
	for position, value := range v.AccessURLs {
		if _, err = tx.Exec(`INSERT INTO server_setting_lists VALUES(?,?,?)`, "accessUrls", position, value); err != nil {
			return err
		}
	}
	for position, value := range v.TrustedProxies {
		if _, err = tx.Exec(`INSERT INTO server_setting_lists VALUES(?,?,?)`, "trustedProxies", position, value); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`DELETE FROM server_log_retention`); err != nil {
		return err
	}
	for _, entry := range v.LogRetention {
		if _, err = tx.Exec(`INSERT INTO server_log_retention VALUES(?,?)`, entry.Category, entry.Days); err != nil {
			return err
		}
	}
	return nil
}

// SettingsValues projects the typed wire field names without serializing a
// second persistence document or silently coercing values.
func SettingsValues(v Settings) map[string]any {
	out := map[string]any{}
	value := reflect.ValueOf(v)
	typ := value.Type()
	for i := 0; i < typ.NumField(); i++ {
		out[strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]] = value.Field(i).Interface()
	}
	return out
}
