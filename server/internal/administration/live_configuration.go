package administration

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"

	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/operations"
)

// LiveConfiguration joins the global live policy to the same transactional
// configuration projection. Per-source locators/credentials never enter it.
type LiveConfiguration struct{}

func (LiveConfiguration) Bindings() []operations.ConfigurationBinding {
	return []operations.ConfigurationBinding{
		{"live.buffer", "live.stream_buffer_seconds", "streamBufferMs", 1000, false},
		{"live.retry", "live.retry_seconds", "retryWindowMs", 1000, false},
		{"live.userAgent", "live.user_agent", "userAgent", 1, false},
		{"live.guideDays", "live.guide_days", "guideDays", 1, false},
		{"live.rewind", "live.timeshift_minutes", "timeshiftMinutes", 1, false},
		{"live.logos", "live.logo_import", "logoImport", 1, false},
		{"live.discovery", "live.discover_tuners", "discoveryEnabled", 1, false},
		{"live.limit.file", "live.import_limits.file_mb", "importBytes", 1 << 20, false},
		{"live.limit.channels", "live.import_limits.channels", "importChannels", 1, false},
		{"live.limit.programs", "live.import_limits.programs", "importProgrammes", 1, false},
	}
}
func (LiveConfiguration) SourceField(field string) string { return "live:" + field }

func liveConfigurationValues(v LiveDefaults) map[string]any {
	return map[string]any{
		"streamBufferMs": float64(v.StreamBufferMs) / 1000, "retryWindowMs": float64(v.RetryWindowMs) / 1000,
		"userAgent": v.UserAgent, "guideDays": v.GuideDays, "timeshiftMinutes": v.TimeShiftMinutes,
		"logoImport": v.LogoImport, "discoveryEnabled": v.DiscoveryEnabled,
		"importBytes": float64(v.ImportLimits.Bytes) / (1 << 20), "importChannels": v.ImportLimits.Channels, "importProgrammes": v.ImportLimits.Programmes,
	}
}

func assignLiveConfiguration(v *LiveDefaults, b operations.ConfigurationBinding, raw any) error {
	switch b.Field {
	case "userAgent":
		s, ok := raw.(string)
		if !ok {
			return operations.ErrInvalid
		}
		v.UserAgent = s
		return nil
	case "logoImport", "discoveryEnabled":
		flag, ok := raw.(bool)
		if !ok {
			return operations.ErrInvalid
		}
		if b.Field == "logoImport" {
			v.LogoImport = flag
		} else {
			v.DiscoveryEnabled = flag
		}
		return nil
	}
	var number float64
	switch n := raw.(type) {
	case int:
		number = float64(n)
	case int64:
		number = float64(n)
	case float64:
		number = n
	default:
		return operations.ErrInvalid
	}
	number *= float64(b.Scale)
	if number < 0 || number > 1e12 || float64(int64(number)) != number {
		return operations.ErrInvalid
	}
	n := int(number)
	switch b.Field {
	case "streamBufferMs":
		v.StreamBufferMs = n
	case "retryWindowMs":
		v.RetryWindowMs = n
	case "guideDays":
		v.GuideDays = n
	case "timeshiftMinutes":
		v.TimeShiftMinutes = n
	case "importBytes":
		v.ImportLimits.Bytes = n
	case "importChannels":
		v.ImportLimits.Channels = n
	case "importProgrammes":
		v.ImportLimits.Programmes = n
	default:
		return operations.ErrInvalid
	}
	return nil
}

func (LiveConfiguration) Read(ctx context.Context, tx *sql.Tx, sources map[string]string) ([]operations.ConfigurationValue, error) {
	v, _, err := livechannels.ReadLiveDefaultsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	current, defaults := liveConfigurationValues(v), liveConfigurationValues(DefaultLiveDefaults())
	out := []operations.ConfigurationValue{}
	for _, b := range (LiveConfiguration{}).Bindings() {
		source := sources["live:"+b.Field]
		if source == "" {
			source = "default"
		}
		out = append(out, operations.ConfigurationValue{b.ID, b.Key, maintenanceAtom(current[b.Field]), maintenanceAtom(current[b.Field]), maintenanceAtom(defaults[b.Field]), source, b.ConfigOnly, "hot"})
	}
	return out, nil
}

func (LiveConfiguration) Projection(ctx context.Context, tx *sql.Tx, sources map[string]string) (map[string]any, error) {
	v, _, err := livechannels.ReadLiveDefaultsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	values := liveConfigurationValues(v)
	out := map[string]any{}
	for _, b := range (LiveConfiguration{}).Bindings() {
		if sources["live:"+b.Field] != "" {
			out[b.Key] = values[b.Field]
		}
	}
	return out, nil
}

func (LiveConfiguration) Import(ctx context.Context, tx *sql.Tx, raw map[string]any, sources map[string]string, _ int64) (bool, error) {
	v, revision, err := livechannels.ReadLiveDefaultsTx(ctx, tx)
	if err != nil {
		return false, err
	}
	defaults := liveConfigurationValues(DefaultLiveDefaults())
	changed := false
	for _, b := range (LiveConfiguration{}).Bindings() {
		key := "live:" + b.Field
		value, ok := raw[b.Key]
		removed := !ok && sources[key] == "file"
		if removed {
			value, ok = defaults[b.Field], true
		}
		if !ok {
			continue
		}
		before := liveConfigurationValues(v)[b.Field]
		if err = assignLiveConfiguration(&v, b, value); err != nil {
			return false, fmt.Errorf("invalid config key %s", b.Key)
		}
		if !reflect.DeepEqual(before, liveConfigurationValues(v)[b.Field]) || sources[key] == "" || removed {
			changed = true
			if removed {
				_, err = tx.ExecContext(ctx, `DELETE FROM server_setting_sources WHERE field=?`, key)
			} else {
				_, err = tx.ExecContext(ctx, `INSERT INTO server_setting_sources VALUES(?,'file') ON CONFLICT(field) DO UPDATE SET source=excluded.source`, key)
			}
			if err != nil {
				return false, err
			}
		}
	}
	if !changed {
		return false, nil
	}
	if err = validateLiveDefaults(&v); err != nil {
		return false, err
	}
	_, err = livechannels.ReplaceLiveDefaultsTx(ctx, tx, revision, v)
	return err == nil, err
}

func queueLiveConfiguration(ctx context.Context, tx *sql.Tx, before, after LiveDefaults) error {
	a, b := liveConfigurationValues(before), liveConfigurationValues(after)
	changed := []string{}
	for _, binding := range (LiveConfiguration{}).Bindings() {
		if !reflect.DeepEqual(a[binding.Field], b[binding.Field]) {
			changed = append(changed, "live:"+binding.Field)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE console_settings_revision SET revision=revision+1 WHERE singleton=1`); err != nil {
		return err
	}
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM console_settings_revision WHERE singleton=1`).Scan(&revision); err != nil {
		return err
	}
	for _, field := range changed {
		if _, err := tx.ExecContext(ctx, `INSERT INTO server_setting_sources VALUES(?,'server') ON CONFLICT(field) DO UPDATE SET source=excluded.source`, field); err != nil {
			return err
		}
		if err := operations.QueueConfigurationProjectionTx(ctx, tx, field, revision); err != nil {
			return err
		}
	}
	return nil
}
