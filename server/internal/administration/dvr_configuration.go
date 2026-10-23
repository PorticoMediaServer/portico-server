package administration

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"

	"portico.local/server/internal/operations"
)

// DVRConfiguration projects the existing normalized defaults. Folder layouts
// and measured storage policy stay with their own production authorities.
type DVRConfiguration struct{}

func (DVRConfiguration) Bindings() []operations.ConfigurationBinding {
	return []operations.ConfigurationBinding{
		{ID: "live.rec.early", Key: "recording.start_early_minutes", Field: "prePaddingMs", Scale: 60000, ConfigOnly: false},
		{ID: "live.rec.late", Key: "recording.finish_late_minutes", Field: "postPaddingMs", Scale: 60000, ConfigOnly: false},
		{ID: "live.rec.ruleKeep", Key: "recording.default_keep", Field: "keepPolicy", Scale: 1, ConfigOnly: false},
	}
}

func (DVRConfiguration) SourceField(field string) string { return "dvr:" + field }

func dvrConfigurationValues(v DVRDefaults) map[string]any {
	return map[string]any{
		"prePaddingMs":  float64(v.PrePaddingMs) / 60000,
		"postPaddingMs": float64(v.PostPaddingMs) / 60000,
		"keepPolicy":    map[string]any{"mode": v.Keep.Mode, "keepCount": v.Keep.KeepCount, "keepDays": v.Keep.KeepDays},
	}
}

func dvrInteger(value any, maximum int) (int, error) {
	var n float64
	switch v := value.(type) {
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case float64:
		n = v
	default:
		return 0, operations.ErrInvalid
	}
	if !(n >= 0 && n <= float64(maximum)) || n != float64(int(n)) {
		return 0, operations.ErrInvalid
	}
	return int(n), nil
}

func assignDVRConfiguration(v *DVRDefaults, field string, raw any) error {
	switch field {
	case "prePaddingMs", "postPaddingMs":
		maximum := 30
		if field == "postPaddingMs" {
			maximum = 120
		}
		n, err := dvrInteger(raw, maximum)
		if err != nil {
			return err
		}
		if field == "prePaddingMs" {
			v.PrePaddingMs = n * 60000
		} else {
			v.PostPaddingMs = n * 60000
		}
	case "keepPolicy":
		m, ok := raw.(map[string]any)
		if !ok || len(m) != 3 {
			return operations.ErrInvalid
		}
		mode, ok := m["mode"].(string)
		if !ok {
			return operations.ErrInvalid
		}
		count, err := dvrInteger(m["keepCount"], 1000)
		if err != nil {
			return err
		}
		days, err := dvrInteger(m["keepDays"], 3650)
		if err != nil {
			return err
		}
		if mode == "keep-all" && (count != 0 || days != 0) || mode == "keep-count" && (count < 1 || days != 0) || mode == "keep-days" && (days < 1 || count != 0) || !oneOf(mode, KeepModes...) {
			return operations.ErrInvalid
		}
		v.Keep = KeepPolicy{Mode: mode, KeepCount: count, KeepDays: days}
	default:
		return operations.ErrInvalid
	}
	return nil
}

func (DVRConfiguration) Read(ctx context.Context, tx *sql.Tx, sources map[string]string) ([]operations.ConfigurationValue, error) {
	document, err := runtimeDefaults(ctx, tx)
	if err != nil {
		return nil, err
	}
	values, defaults := dvrConfigurationValues(document.Settings), dvrConfigurationValues(DefaultDVRDefaults())
	out := []operations.ConfigurationValue{}
	for _, b := range (DVRConfiguration{}).Bindings() {
		source := sources["dvr:"+b.Field]
		if source == "" {
			source = "default"
		}
		out = append(out, operations.ConfigurationValue{ID: b.ID, Key: b.Key, Current: maintenanceAtom(values[b.Field]), Effective: maintenanceAtom(values[b.Field]), Default: maintenanceAtom(defaults[b.Field]), Source: source, ConfigOnly: b.ConfigOnly, ApplicationMode: "next-operation"})
	}
	return out, nil
}

func (DVRConfiguration) Projection(ctx context.Context, tx *sql.Tx, sources map[string]string) (map[string]any, error) {
	document, err := runtimeDefaults(ctx, tx)
	if err != nil {
		return nil, err
	}
	values := dvrConfigurationValues(document.Settings)
	out := map[string]any{}
	for _, b := range (DVRConfiguration{}).Bindings() {
		if sources["dvr:"+b.Field] != "" {
			out[b.Key] = values[b.Field]
		}
	}
	return out, nil
}

func (DVRConfiguration) Import(ctx context.Context, tx *sql.Tx, raw map[string]any, sources map[string]string, _ int64) (bool, error) {
	document, err := runtimeDefaults(ctx, tx)
	if err != nil {
		return false, err
	}
	v := document.Settings
	defaults := dvrConfigurationValues(DefaultDVRDefaults())
	changed := false
	for _, b := range (DVRConfiguration{}).Bindings() {
		field := "dvr:" + b.Field
		value, ok := raw[b.Key]
		removed := !ok && sources[field] == "file"
		if removed {
			value, ok = defaults[b.Field], true
		}
		if !ok {
			continue
		}
		before := dvrConfigurationValues(v)[b.Field]
		if err = assignDVRConfiguration(&v, b.Field, value); err != nil {
			return false, fmt.Errorf("invalid config key %s", b.Key)
		}
		if !reflect.DeepEqual(before, dvrConfigurationValues(v)[b.Field]) || sources[field] == "" || removed {
			changed = true
			if removed {
				_, err = tx.ExecContext(ctx, `DELETE FROM server_setting_sources WHERE field=?`, field)
			} else {
				_, err = tx.ExecContext(ctx, `INSERT INTO server_setting_sources VALUES(?,'file') ON CONFLICT(field) DO UPDATE SET source=excluded.source`, field)
			}
			if err != nil {
				return false, err
			}
		}
	}
	if !changed {
		return false, nil
	}
	_, err = replaceRuntimeDVRDefaultsTx(ctx, tx, document.Revision, v)
	return err == nil, err
}

func queueDVRConfiguration(ctx context.Context, tx *sql.Tx, before, after DVRDefaults) error {
	a, b := dvrConfigurationValues(before), dvrConfigurationValues(after)
	changed := []string{}
	for _, binding := range (DVRConfiguration{}).Bindings() {
		if !reflect.DeepEqual(a[binding.Field], b[binding.Field]) {
			changed = append(changed, "dvr:"+binding.Field)
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
