package operations

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"portico.local/server/internal/configfile"
	"portico.local/server/internal/dbwork"
)

// ConfigurationBinding connects an approved row to its one runtime authority.
// It contains no UI copy and does not register a setting without a consumer.
type ConfigurationBinding struct {
	ID, Key, Field string
	Scale          int
	ConfigOnly     bool
}

func ConfigurationBindings() []ConfigurationBinding {
	return []ConfigurationBinding{
		{"general.name", "general.name", "name", 1, false},
		{"general.updateFeed", "general.update_feed_url", "updates.feedUrl", 1, false},
		{"updates.channel", "updates.channel", "updates.releaseChannel", 1, true},
		{"streaming.uploadSpeed", "streaming.upload_speed", "uploadCapacityKbps", 1000, false},
		{"streaming.awayExact", "streaming.away_limit_mbps", "remoteBitrateLimitKbps", 1000, false},
		{"streaming.hardware", "streaming.hardware", "hardwareBackend", 1, false},
		{"streaming.hardwareDevice", "streaming.hardware_device", "hardwareDevice", 1, false},
		{"streaming.toneMapping", "streaming.tone_mapping", "hdrToneMapping", 1, false},
		{"streaming.toneMappingMethod", "streaming.tone_mapping_method", "hdrToneMappingAlgorithm", 1, false},
		{"streaming.encoderSpeed", "streaming.encoder_speed", "x264Preset", 1, false},
		{"streaming.repackage", "streaming.direct_stream_remux", "directStreamRemux", 1, false},
		{"streaming.ffmpegPath", "streaming.ffmpeg_path", "ffmpegPath", 1, false},
		{"streaming.convertAhead", "streaming.convert_ahead_seconds", "throttleBufferMs", 1000, false},
		{"streaming.rewindKeep", "streaming.rewind_keep_seconds", "playedRetentionMs", 1000, false},
		{"streaming.sessionSpace", "streaming.session_space_mb", "sessionStorageMiB", 1, false},
		{"streaming.maxConversions", "streaming.max_conversions", "maxConcurrentSessions", 1, false},
		{"streaming.maxHardwareConversions", "streaming.max_hardware_conversions", "maxHardwareSessions", 1, false},
		{"streaming.maxSoftwareConversions", "streaming.max_software_conversions", "maxSoftwareSessions", 1, false},
		{"streaming.maxBackgroundConversions", "streaming.max_background_conversions", "maxBackgroundSessions", 1, false},
		{"streaming.maxStreams", "streaming.max_streams", "serverCap", 1, false},
		{"streaming.pausedTimeout", "streaming.paused_timeout_minutes", "pausedSessionTimeoutMinutes", 1, false},
		{"storage.analysisSpace", "storage.analysis_space_gb", "analysisStorageGiB", 1, false},
		{"storage.analysisStage", "storage.analysis_stage_limit_mb", "analysisStageStorageMiB", 1, false},
		{"storage.subtitleTracks", "storage.subtitle_tracks_per_file", "subtitleTrackLimit", 1, false},
		{"storage.subtitleFile", "storage.subtitle_file_limit_mb", "subtitleFileLimitMiB", 1, false},
		{"storage.keepNotifications", "storage.keep_notifications_days", "notificationDays", 1, false},
		{"history.keepHistory", "history.keep_days", "playHistoryDays", 1, false},
		{"remote.ownCertificate", "remote.own_certificate", "ownAddressEnabled", 1, false},
		{"remote.externalAddress", "remote.external_address", "customCertificateDomain", 1, false},
		{"remote.certFile", "remote.certificate_path", "customCertificatePath", 1, false},
		{"remote.certKey", "remote.certificate_key_path", "customCertificateKeyPath", 1, false},
		{"remote.homeNets", "remote.home_networks", "lanNetworks", 1, false},
		{"remote.discovery", "remote.discovery", "lanDiscoveryEnabled", 1, false},
		{"remote.dlna", "remote.dlna", "dlnaEnabled", 1, false},
		{"remote.interface", "network.advertised_interface", "advertisedInterface", 1, false},
		{"remote.connectionLimit", "network.connection_limit", "connectionLimit", 1, false},
		{"remote.extraAddresses", "remote.extra_addresses", "accessUrls", 1, false},
		{"remote.trustedProxies", "remote.trusted_proxies", "trustedProxies", 1, false},
		{"troubleshooting.logLevel", "logging.level", "logLevel", 1, false},
		{"troubleshooting.keepLogs", "logging.keep_days", "diagnosticDays", 1, false},
		{"troubleshooting.keepLogsByKind", "logging.keep_days_by_kind", "logRetention", 1, false},
	}
}

type ConfigurationValue struct {
	ID              string `json:"id"`
	Key             string `json:"key"`
	Current         any    `json:"current"`
	Effective       any    `json:"effective"`
	Default         any    `json:"default"`
	Source          string `json:"source"`
	ConfigOnly      bool   `json:"configOnly"`
	ApplicationMode string `json:"applicationMode"`
}
type SettingsConfiguration struct {
	Revision         int64                `json:"revision"`
	RegistryRevision string               `json:"registryRevision"`
	Path             string               `json:"path"`
	Fields           []ConfigurationValue `json:"fields"`
}

func configurationValue(b ConfigurationBinding, v any) any {
	if b.Scale > 1 {
		if n, ok := v.(int); ok {
			if b.Field == "uploadCapacityKbps" && n == 0 {
				return nil
			}
			return float64(n) / float64(b.Scale)
		}
	}
	if n, ok := v.(*int); ok {
		if n == nil {
			return nil
		}
		return *n
	}
	if rows, ok := v.([]LogRetention); ok {
		out := []map[string]any{}
		for _, entry := range rows {
			out = append(out, map[string]any{"category": entry.Category, "days": entry.Days})
		}
		return out
	}
	return v
}
func (s *Store) Configuration(ctx context.Context, auth Authorize) (out SettingsConfiguration, err error) {
	out = SettingsConfiguration{RegistryRevision: RegistryRevision, Fields: []ConfigurationValue{}}
	if s.ConfigFile != nil {
		out.Path = s.ConfigPath
	}
	err = s.snapshot(ctx, auth, "", func(tx *sql.Tx) error {
		document, e := readSettings(tx)
		if e != nil {
			return e
		}
		s.projectAppliedSettings(&document)
		out.Revision = document.Revision
		current, effective, defaults := SettingsValues(document.Requested), SettingsValues(document.Effective), SettingsValues(DefaultSettings())
		definitions := map[string]Field{}
		for _, f := range Registry() {
			definitions[f.ID] = f
		}
		sources := map[string]string{}
		rows, e := tx.QueryContext(ctx, `SELECT field,source FROM server_setting_sources`)
		if e != nil {
			return e
		}
		for rows.Next() {
			var k, v string
			if e = rows.Scan(&k, &v); e != nil {
				rows.Close()
				return e
			}
			sources[k] = v
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, b := range ConfigurationBindings() {
			source := sources[b.Field]
			if source == "" {
				source = "default"
			}
			out.Fields = append(out.Fields, ConfigurationValue{b.ID, b.Key, configurationValue(b, current[b.Field]), configurationValue(b, effective[b.Field]), configurationValue(b, defaults[b.Field]), source, b.ConfigOnly, definitions[b.Field].Apply})
		}
		return nil
	})
	return
}

// InitializeConfiguration drains an interrupted DB-to-file projection before
// accepting external file edits. The database never stores a serialized graph.
func (s *Store) InitializeConfiguration(ctx context.Context, path string) error {
	s.ConfigPath = path
	s.ConfigFile = configfile.New(path)
	if _, err := s.ConfigFile.Reload(); err != nil {
		return err
	}
	var pending int64
	if err := s.DB.QueryRowContext(ctx, `SELECT pending_revision FROM server_configuration_state WHERE singleton=1`).Scan(&pending); err != nil {
		return err
	}
	if pending > 0 {
		return s.FlushConfiguration(ctx)
	}
	return s.ImportConfiguration(ctx)
}

func configurationField(v *Settings, key string) reflect.Value {
	value := reflect.ValueOf(v).Elem()
	typ := value.Type()
	for i := 0; i < typ.NumField(); i++ {
		if strings.Split(typ.Field(i).Tag.Get("json"), ",")[0] == key {
			return value.Field(i)
		}
	}
	return reflect.Value{}
}
func assignConfiguration(v *Settings, b ConfigurationBinding, raw any) error {
	target := configurationField(v, b.Field)
	if !target.IsValid() {
		return ErrInvalid
	}
	if raw == nil {
		if target.Kind() == reflect.Pointer {
			target.SetZero()
			return nil
		}
		if b.Field == "uploadCapacityKbps" {
			target.SetInt(0)
			return nil
		}
		return ErrInvalid
	}
	switch target.Kind() {
	case reflect.Int, reflect.Pointer:
		var number float64
		switch n := raw.(type) {
		case int64:
			number = float64(n)
		case float64:
			number = n
		case int:
			number = float64(n)
		default:
			return ErrInvalid
		}
		number *= float64(b.Scale)
		if number < 0 || number > 1e12 || float64(int64(number)) != number {
			return ErrInvalid
		}
		if target.Kind() == reflect.Pointer && number == 0 {
			target.SetZero()
			return nil
		}
		if target.Kind() == reflect.Pointer {
			n := int(number)
			target.Set(reflect.ValueOf(&n))
		} else {
			target.SetInt(int64(number))
		}
	case reflect.Bool:
		x, ok := raw.(bool)
		if !ok {
			return ErrInvalid
		}
		target.SetBool(x)
	case reflect.String:
		x, ok := raw.(string)
		if !ok {
			return ErrInvalid
		}
		target.SetString(x)
	case reflect.Slice:
		if b.Field == "logRetention" {
			var values []LogRetention
			items, ok := raw.([]any)
			if !ok {
				return ErrInvalid
			}
			for _, item := range items {
				m, ok := item.(map[string]any)
				if !ok {
					return ErrInvalid
				}
				category, ok := m["category"].(string)
				if !ok {
					return ErrInvalid
				}
				days, ok := m["days"].(int64)
				if !ok {
					return ErrInvalid
				}
				values = append(values, LogRetention{category, int(days)})
			}
			target.Set(reflect.ValueOf(values))
			break
		}
		items, ok := raw.([]any)
		if !ok {
			if strings, ok := raw.([]string); ok {
				target.Set(reflect.ValueOf(strings))
				return nil
			}
			return ErrInvalid
		}
		values := []string{}
		for _, item := range items {
			x, ok := item.(string)
			if !ok {
				return ErrInvalid
			}
			values = append(values, x)
		}
		target.Set(reflect.ValueOf(values))
	default:
		return ErrInvalid
	}
	return nil
}

// ImportConfiguration validates the entire registered document before applying
// any field. A partial editor write keeps the last committed runtime policy.
func (s *Store) ImportConfiguration(ctx context.Context) error {
	if s.ConfigFile == nil {
		return nil
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	bindings := ConfigurationBindings()
	raw := map[string]any{}
	for _, b := range bindings {
		value, err := s.ConfigFile.Resolve(configfile.Binding{Key: b.Key})
		if err != nil {
			return err
		}
		if value.Source == configfile.File {
			raw[b.Key] = value.Value
		}
	}
	if len(raw) == 0 {
		return nil
	}
	return dbwork.WithWriteTx(ctx, s.DB, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		document, err := readSettings(tx)
		if err != nil {
			return err
		}
		next := document.Requested
		changed := false
		for _, b := range bindings {
			value, ok := raw[b.Key]
			if !ok {
				continue
			}
			before := configurationValue(b, SettingsValues(next)[b.Field])
			if err = assignConfiguration(&next, b, value); err != nil {
				return fmt.Errorf("invalid config key %s", b.Key)
			}
			if !reflect.DeepEqual(before, configurationValue(b, SettingsValues(next)[b.Field])) {
				changed = true
			}
		}
		if err = validateSettings(&next); err != nil {
			return err
		}
		if !changed {
			return nil
		}
		if err = writeStoredSettings(tx, next, s.now()); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO configuration VALUES('name',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, next.Name); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE playback_owner_policy SET revision=revision+1,transcoding_enabled=?,per_account_cap=?,server_cap=? WHERE singleton=1`, next.TranscodingEnabled, next.PerAccountCap, next.ServerCap); err != nil {
			return err
		}
		for _, b := range bindings {
			if _, ok := raw[b.Key]; ok {
				if _, err = tx.ExecContext(ctx, `INSERT INTO server_setting_sources VALUES(?,'file') ON CONFLICT(field) DO UPDATE SET source=excluded.source`, b.Field); err != nil {
					return err
				}
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE console_settings_revision SET revision=revision+1 WHERE singleton=1`)
		return err
	})
}

func (s *Store) FlushConfiguration(ctx context.Context) error {
	if s.ConfigFile == nil {
		return nil
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	snapshot, err := dbwork.BeginSnapshot(ctx, s.DB)
	if err != nil {
		return err
	}
	var pending int64
	err = snapshot.Tx().QueryRowContext(ctx, `SELECT pending_revision FROM server_configuration_state WHERE singleton=1`).Scan(&pending)
	if err != nil || pending == 0 {
		snapshot.Rollback()
		return err
	}
	document, err := readSettings(snapshot.Tx())
	snapshot.Rollback()
	if err != nil {
		return err
	}
	patch := map[string]any{}
	values := SettingsValues(document.Requested)
	for _, b := range ConfigurationBindings() {
		v := configurationValue(b, values[b.Field])
		if v != nil {
			patch[b.Key] = v
		} else if b.Field == "uploadCapacityKbps" {
			patch[b.Key] = int64(0)
		} else {
			patch[b.Key] = int64(0)
		}
	}
	if err = s.ConfigFile.Patch(patch); err != nil {
		return err
	}
	return dbwork.WithWriteTx(ctx, s.DB, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE server_configuration_state SET pending_revision=0 WHERE singleton=1 AND pending_revision=?`, pending)
		return e
	})
}
