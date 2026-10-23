package administration

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"portico.local/server/internal/configfile"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/livechannels/dvr"
)

func TestDVRConfigurationSharesOwnerDefaultsAndNextRecordingOptions(t *testing.T) {
	s, db, _ := newService(t)
	ctx := context.Background()
	domain := DVRConfiguration{}
	importValues := func(values map[string]any, sources map[string]string) error {
		return dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
			_, err := domain.Import(ctx, tx, values, sources, 0)
			return err
		})
	}
	raw := map[string]any{
		"recording.start_early_minutes": int64(2), "recording.finish_late_minutes": int64(10),
		"recording.default_keep": map[string]any{"mode": "keep-count", "keepCount": int64(5), "keepDays": int64(0)},
	}
	if err := importValues(raw, nil); err != nil {
		t.Fatal(err)
	}
	readOptions := func() dvr.Options {
		t.Helper()
		snapshot, err := dbwork.BeginSnapshot(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		defer snapshot.Rollback()
		options, err := dvr.DefaultOptionsTx(ctx, snapshot.Tx())
		if err != nil {
			t.Fatal(err)
		}
		return options
	}
	options := readOptions()
	if options.BeforeMs != 120000 || options.AfterMs != 600000 || options.EpisodeLimit != 5 || options.RetentionDays != 0 {
		t.Fatalf("next recording ignored file policy: %+v", options)
	}
	doc, err := s.DVRSettings(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A later bad field rolls back the earlier source annotation and value.
	if err = importValues(map[string]any{"recording.start_early_minutes": int64(4), "recording.finish_late_minutes": int64(121)}, map[string]string{}); err == nil {
		t.Fatal("invalid partial file import committed")
	}
	after, err := s.DVRSettings(ctx, nil)
	if err != nil || after.Revision != doc.Revision || readOptions().BeforeMs != 120000 {
		t.Fatalf("failed import changed defaults: %+v %v", after, err)
	}
	doc.Settings.PrePaddingMs = 300000
	doc.Settings.Keep = KeepPolicy{Mode: "keep-days", KeepDays: 30}
	change := Change[DVRDefaults]{ExpectedRevision: doc.Revision, OperationID: "recording-defaults-app-write", Settings: doc.Settings}
	saved, err := s.SaveDVRSettings(ctx, nil, change)
	if err != nil {
		t.Fatal(err)
	}
	options = readOptions()
	if options.BeforeMs != 300000 || options.AfterMs != 600000 || options.RetentionDays != 30 || options.EpisodeLimit != 0 {
		t.Fatalf("next recording ignored owner edit: %+v", options)
	}
	replay, err := s.SaveDVRSettings(ctx, nil, change)
	if err != nil || replay.Revision != saved.Revision {
		t.Fatalf("owner replay moved revision: %+v %v", replay, err)
	}
	var source string
	if err = db.QueryRow(`SELECT source FROM server_setting_sources WHERE field='dvr:prePaddingMs'`).Scan(&source); err != nil || source != "server" {
		t.Fatalf("owner source=%q %v", source, err)
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{"dvr:prePaddingMs": "server", "dvr:postPaddingMs": "file", "dvr:keepPolicy": "server"}
	projection, err := domain.Projection(ctx, snapshot.Tx(), sources)
	snapshot.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	// Prove the actual TOML codec preserves the typed keep policy and units.
	file := configfile.New(filepath.Join(t.TempDir(), "portico.toml"))
	if err = file.Patch(projection); err != nil {
		t.Fatal(err)
	}
	decoded := map[string]any{}
	for _, binding := range domain.Bindings() {
		value, err := file.Resolve(configfile.Binding{Key: binding.Key})
		if err != nil || value.Source != configfile.File {
			t.Fatalf("file readback %s: %+v %v", binding.Key, value, err)
		}
		decoded[binding.Key] = value.Value
	}
	if err = importValues(decoded, sources); err != nil {
		t.Fatal(err)
	}
	if got := readOptions(); got != options {
		t.Fatalf("TOML roundtrip changed actual options: %+v -> %+v", options, got)
	}
}
