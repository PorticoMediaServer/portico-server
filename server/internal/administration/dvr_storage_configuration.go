package administration

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"

	"portico.local/server/internal/livechannels/dvr"
	"portico.local/server/internal/operations"
)

// DVRStorageConfiguration projects the existing physical storage policy; it
// owns no recording root, layout, retirement receipt or alternative policy.
type DVRStorageConfiguration struct{}

func (DVRStorageConfiguration) Bindings() []operations.ConfigurationBinding {
	return []operations.ConfigurationBinding{
		{ID: "live.rec.days", Key: "recording.retention_days", Field: "retentionDays", Scale: 1, ConfigOnly: false},
		{ID: "live.rec.episodes", Key: "recording.episode_limit", Field: "episodeLimit", Scale: 1, ConfigOnly: false},
		{ID: "live.rec.floor", Key: "recording.free_space_gb", Field: "floorBytes", Scale: 1000000000, ConfigOnly: false},
		{ID: "live.rec.cap", Key: "recording.cap_gb", Field: "capBytes", Scale: 1000000000, ConfigOnly: false},
	}
}

func (DVRStorageConfiguration) SourceField(field string) string { return "dvr-storage:" + field }

func dvrStorageConfigurationValues(p dvr.StoragePolicy) map[string]any {
	return map[string]any{
		"retentionDays": p.RetentionDays, "episodeLimit": p.EpisodeLimit,
		"floorBytes": float64(p.FloorBytes) / 1000000000, "capBytes": float64(p.CapBytes) / 1000000000,
	}
}

func assignDVRStorageConfiguration(p *dvr.StoragePolicy, field string, raw any) error {
	maximum := 3650
	switch field {
	case "episodeLimit":
		maximum = 500
	case "floorBytes":
		maximum = 5000
	case "capBytes":
		maximum = 100000
	case "retentionDays":
	default:
		return operations.ErrInvalid
	}
	n, err := dvrInteger(raw, maximum)
	if err != nil {
		return err
	}
	switch field {
	case "retentionDays":
		p.RetentionDays = n
	case "episodeLimit":
		p.EpisodeLimit = n
	case "floorBytes":
		p.FloorBytes = int64(n) * 1000000000
	case "capBytes":
		p.CapBytes = int64(n) * 1000000000
	}
	return nil
}

func (DVRStorageConfiguration) Read(ctx context.Context, tx *sql.Tx, sources map[string]string) ([]operations.ConfigurationValue, error) {
	p, err := dvr.ReadStoragePolicyTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	values, defaults := dvrStorageConfigurationValues(p), dvrStorageConfigurationValues(dvr.DefaultStoragePolicy())
	out := []operations.ConfigurationValue{}
	for _, b := range (DVRStorageConfiguration{}).Bindings() {
		source := sources["dvr-storage:"+b.Field]
		if source == "" {
			source = "default"
		}
		mode := "next-operation"
		if b.Field == "floorBytes" || b.Field == "capBytes" {
			mode = "hot"
		}
		out = append(out, operations.ConfigurationValue{ID: b.ID, Key: b.Key, Current: maintenanceAtom(values[b.Field]), Effective: maintenanceAtom(values[b.Field]), Default: maintenanceAtom(defaults[b.Field]), Source: source, ConfigOnly: b.ConfigOnly, ApplicationMode: mode})
	}
	return out, nil
}

func (DVRStorageConfiguration) Projection(ctx context.Context, tx *sql.Tx, sources map[string]string) (map[string]any, error) {
	p, err := dvr.ReadStoragePolicyTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	values, out := dvrStorageConfigurationValues(p), map[string]any{}
	for _, b := range (DVRStorageConfiguration{}).Bindings() {
		if sources["dvr-storage:"+b.Field] != "" {
			out[b.Key] = values[b.Field]
		}
	}
	return out, nil
}

func (DVRStorageConfiguration) Import(ctx context.Context, tx *sql.Tx, raw map[string]any, sources map[string]string, _ int64) (bool, error) {
	p, err := dvr.ReadStoragePolicyTx(ctx, tx)
	if err != nil {
		return false, err
	}
	defaults := dvrStorageConfigurationValues(dvr.DefaultStoragePolicy())
	changed := false
	for _, b := range (DVRStorageConfiguration{}).Bindings() {
		field := "dvr-storage:" + b.Field
		value, ok := raw[b.Key]
		removed := !ok && sources[field] == "file"
		if removed {
			value, ok = defaults[b.Field], true
		}
		if !ok {
			continue
		}
		before := dvrStorageConfigurationValues(p)[b.Field]
		if err = assignDVRStorageConfiguration(&p, b.Field, value); err != nil {
			return false, fmt.Errorf("invalid config key %s", b.Key)
		}
		if !reflect.DeepEqual(before, dvrStorageConfigurationValues(p)[b.Field]) || sources[field] == "" || removed {
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
	_, err = dvr.ReplaceStoragePolicyTx(ctx, tx, p.Revision, p)
	return err == nil, err
}

// QueueDVRStorageConfigurationTx is installed on the actual owner writer. Its
// source annotation, durable file projection and owner receipt commit together.
func QueueDVRStorageConfigurationTx(ctx context.Context, tx *sql.Tx, before, after dvr.StoragePolicy) error {
	a, b := dvrStorageConfigurationValues(before), dvrStorageConfigurationValues(after)
	changed := []string{}
	for _, binding := range (DVRStorageConfiguration{}).Bindings() {
		if !reflect.DeepEqual(a[binding.Field], b[binding.Field]) {
			changed = append(changed, "dvr-storage:"+binding.Field)
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
