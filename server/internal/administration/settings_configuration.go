package administration

import (
	"context"
	"database/sql"
	"fmt"
	"portico.local/server/internal/operations"
	"reflect"
	"slices"
)

func (MaintenanceConfiguration) Bindings() []operations.ConfigurationBinding {
	return []operations.ConfigurationBinding{
		{"general.heavyWork", "general.heavy_work", "heavy.mode", 1, false},
		{"general.heavyStart", "general.heavy_work_start", "heavy.start", 1, false},
		{"general.heavyLength", "general.heavy_work_hours", "heavy.hours", 1, false},
		{"general.heavyDays", "general.heavy_work_days", "heavy.days", 1, false},
		{"general.backgroundSpeed", "general.background_speed", "backgroundSpeed", 1, false},
		{"storage.backupKeepExact", "storage.backups_to_keep_exact", "backupKeepCount", 1, false},
		{"storage.backupSchedule", "backup.schedule", "backup.mode", 1, false},
		{"storage.backupTime", "backup.start_time", "backup.start", 1, false},
		{"storage.backupWindowLength", "backup.window_hours", "backup.hours", 1, false},
		{"storage.backupDays", "backup.chosen_days", "backup.days", 1, false},
		{"storage.keepConversions", "storage.keep_conversions_days", "retention.conversions", 1, false},
		{"storage.keepPrepared", "storage.keep_prepared_days", "retention.prepared", 1, false},
		{"storage.keepDownloads", "storage.keep_downloads_days", "retention.downloads", 1, false},
		{"storage.keepSubtitles", "storage.keep_subtitles_days", "retention.subtitles", 1, false},
		{"storage.keepTrickplay", "storage.keep_trickplay_days", "retention.trickplay", 1, false},
	}
}
func maintenanceConfigurationValues(v MaintenanceSettingsDocument) map[string]any {
	out := map[string]any{"backgroundSpeed": v.BackgroundSpeed, "backupKeepCount": v.BackupKeepCount}
	for k, n := range v.Retention {
		out["retention."+k] = n
	}
	for _, w := range v.Windows {
		days := map[string]any{}
		for _, day := range Days {
			if slices.Contains(w.Days, day) {
				days[day] = "runs"
			} else {
				days[day] = "skips"
			}
		}
		mode := "custom"
		if w.ID == "heavy" {
			if !w.Enabled {
				mode = "any"
			} else if w.StartMinute == 60 && w.DurationMinutes == 300 && len(w.Days) == 7 && w.Timezone == "" {
				mode = "overnight"
			}
		} else {
			if !w.Enabled {
				mode = "never"
			} else {
				for _, cadence := range MaintenanceCadences() {
					if cadence.ID == "custom" || len(cadence.Days) != len(w.Days) {
						continue
					}
					exact := true
					for _, day := range cadence.Days {
						exact = exact && slices.Contains(w.Days, day)
					}
					if exact {
						mode = cadence.ID
						break
					}
				}
			}
		}
		out[w.ID+".mode"] = mode
		out[w.ID+".start"] = w.StartMinute
		out[w.ID+".hours"] = float64(w.DurationMinutes) / 60
		out[w.ID+".days"] = days
	}
	return out
}
func assignMaintenanceConfiguration(v *MaintenanceSettingsDocument, b operations.ConfigurationBinding, raw any) error {
	if b.Field == "backgroundSpeed" {
		s, ok := raw.(string)
		if !ok {
			return operations.ErrInvalid
		}
		v.BackgroundSpeed = s
		return nil
	}
	number := func() (int, error) {
		var n float64
		switch t := raw.(type) {
		case int:
			n = float64(t)
		case int64:
			n = float64(t)
		case float64:
			n = t
		default:
			return 0, operations.ErrInvalid
		}
		if n < 0 || n > 1e6 || n != float64(int(n)) {
			return 0, operations.ErrInvalid
		}
		return int(n), nil
	}
	if b.Field == "backupKeepCount" {
		n, e := number()
		v.BackupKeepCount = n
		return e
	}
	if len(b.Field) > 10 && b.Field[:10] == "retention." {
		n, e := number()
		v.Retention[b.Field[10:]] = n
		return e
	}
	for i := range v.Windows {
		w := &v.Windows[i]
		prefix := w.ID + "."
		if len(b.Field) < len(prefix) || b.Field[:len(prefix)] != prefix {
			continue
		}
		switch b.Field[len(prefix):] {
		case "mode":
			s, ok := raw.(string)
			if !ok {
				return operations.ErrInvalid
			}
			if w.ID == "heavy" {
				switch s {
				case "any":
					w.Enabled = false
				case "overnight":
					w.Enabled = true
					w.StartMinute = 60
					w.DurationMinutes = 300
					w.Days = append([]string{}, Days...)
					w.Timezone = ""
					w.Cadence = "nightly"
				case "custom":
					w.Enabled = true
					w.Cadence = "custom"
				default:
					return operations.ErrInvalid
				}
			} else {
				if s == "never" {
					w.Enabled = false
					return nil
				}
				w.Enabled = true
				for _, c := range MaintenanceCadences() {
					if c.ID == s {
						w.Cadence = s
						if s != "custom" {
							w.Days = append([]string{}, c.Days...)
						}
						return nil
					}
				}
				return operations.ErrInvalid
			}
		case "start":
			n, e := number()
			if e != nil {
				return e
			}
			w.StartMinute = n
		case "hours":
			n, e := number()
			if e != nil {
				return e
			}
			w.DurationMinutes = n * 60
		case "days":
			m, ok := raw.(map[string]any)
			if !ok || len(m) != 7 {
				return operations.ErrInvalid
			}
			w.Days = []string{}
			for _, day := range Days {
				switch m[day] {
				case "runs":
					w.Days = append(w.Days, day)
				case "skips":
				default:
					return operations.ErrInvalid
				}
			}
			w.Cadence = "custom"
		default:
			return operations.ErrInvalid
		}
		return nil
	}
	return operations.ErrInvalid
}
func (MaintenanceConfiguration) Read(ctx context.Context, tx *sql.Tx, sources map[string]string) ([]operations.ConfigurationValue, error) {
	v, _, err := ReadMaintenanceConfiguration(ctx, tx)
	if err != nil {
		return nil, err
	}
	current := maintenanceConfigurationValues(v)
	defaults := maintenanceConfigurationValues(DefaultMaintenanceSettings())
	out := []operations.ConfigurationValue{}
	for _, b := range (MaintenanceConfiguration{}).Bindings() {
		source := sources["maintenance:"+b.Field]
		if source == "" {
			source = "default"
		}
		out = append(out, operations.ConfigurationValue{b.ID, b.Key, maintenanceAtom(current[b.Field]), maintenanceAtom(current[b.Field]), maintenanceAtom(defaults[b.Field]), source, b.ConfigOnly, "hot"})
	}
	return out, nil
}
func (MaintenanceConfiguration) Import(ctx context.Context, tx *sql.Tx, raw map[string]any, sources map[string]string, now int64) (bool, error) {
	v, _, err := ReadMaintenanceConfiguration(ctx, tx)
	if err != nil {
		return false, err
	}
	defaults := maintenanceConfigurationValues(DefaultMaintenanceSettings())
	changed := false
	for _, b := range (MaintenanceConfiguration{}).Bindings() {
		key := "maintenance:" + b.Field
		value, ok := raw[b.Key]
		removed := !ok && sources[key] == "file"
		if removed {
			value = defaults[b.Field]
			ok = true
		}
		if !ok {
			continue
		}
		before := maintenanceConfigurationValues(v)[b.Field]
		if err = assignMaintenanceConfiguration(&v, b, value); err != nil {
			return false, fmt.Errorf("invalid config key %s", b.Key)
		}
		different := !reflect.DeepEqual(before, maintenanceConfigurationValues(v)[b.Field])
		if different || sources[key] == "" || removed {
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
	if changed {
		return true, WriteMaintenanceConfiguration(ctx, tx, v, now)
	}
	return false, nil
}

// MaintenanceConfiguration connects the existing normalized scheduler authority
// to the shared file inside the same transaction as runtime settings.
type MaintenanceConfiguration struct{}

func (MaintenanceConfiguration) SourceField(field string) string { return "maintenance:" + field }

func maintenanceAtom(v any) *any {
	if v == nil {
		return nil
	}
	return &v
}
func (MaintenanceConfiguration) Projection(ctx context.Context, tx *sql.Tx, sources map[string]string) (map[string]any, error) {
	v, _, err := ReadMaintenanceConfiguration(ctx, tx)
	if err != nil {
		return nil, err
	}
	values := maintenanceConfigurationValues(v)
	out := map[string]any{}
	for _, b := range (MaintenanceConfiguration{}).Bindings() {
		if sources["maintenance:"+b.Field] != "" {
			out[b.Key] = values[b.Field]
		}
	}
	return out, nil
}
