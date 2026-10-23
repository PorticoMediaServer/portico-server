package administration

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/configfile"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/livechannels/dvr"
	"portico.local/server/internal/recordingmedia"
)

func TestDVRStorageConfigurationSharesPolicyAndPhysicalFloor(t *testing.T) {
	_, db, state := newService(t)
	ctx := context.Background()
	live, err := livechannels.New(db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := dvr.New(db, live, func(context.Context, *sql.Tx, livechannels.Owner, string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	// Empty tool paths keep media unavailable. This exercises only the genuine
	// filesystem policy checks and launches no decoder or capture process.
	driver, err := recordingmedia.New(filepath.Join(state, "recordings"), "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	locks, err := livechannels.NewPhysicalLocks(filepath.Join(state, "locks"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ConfigureCapture(driver, locks); err != nil {
		t.Fatal(err)
	}
	store.StoragePolicyChangedTx = QueueDVRStorageConfigurationTx
	domain := DVRStorageConfiguration{}
	importValues := func(values map[string]any, sources map[string]string) error {
		return dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
			_, err := domain.Import(ctx, tx, values, sources, 0)
			return err
		})
	}
	read := func() dvr.StoragePolicy {
		t.Helper()
		p, err := dvr.ReadStoragePolicy(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err = importValues(map[string]any{"recording.retention_days": int64(30), "recording.episode_limit": int64(5), "recording.free_space_gb": int64(5000), "recording.cap_gb": int64(250)}, nil); err != nil {
		t.Fatal(err)
	}
	imported := read()
	if imported.RetentionDays != 30 || imported.EpisodeLimit != 5 || imported.FloorBytes != 5000000000000 || imported.CapBytes != 250000000000 {
		t.Fatalf("actual policy ignored file units: %#v", imported)
	}
	if err = store.RefreshStoragePolicy(ctx); err != nil || store.StorageLimits() != imported {
		t.Fatalf("bounded policy reload = %#v, %v", store.StorageLimits(), err)
	}
	if err = driver.CheckFloor(ctx); !errors.Is(err, dvr.ErrStorageFloor) {
		t.Fatalf("real volume check ignored imported floor: %v", err)
	}
	if err = importValues(map[string]any{"recording.retention_days": int64(90), "recording.cap_gb": 1.5}, nil); err == nil || read() != imported {
		t.Fatalf("partial invalid import escaped rollback: %#v, %v", read(), err)
	}
	// This fixture supplies only the pre-existing authority callback; it is a
	// configuration/receipt proof, not membership or media-admission acceptance.
	authority := func(context.Context, *sql.Tx, bool) (string, func(string, string) bool, error) {
		return "storage-policy-fixture", func(string, string) bool { return true }, nil
	}
	owner := livechannels.Owner{Authority: "local", AccountID: "fixture", ProfileID: "primary"}
	input := dvr.StoragePolicyInput{Mutation: dvr.Mutation{RequestID: strings.Repeat("a", 48), ExpectedRevision: imported.Revision}, Policy: dvr.StoragePolicy{}}
	saved, err := store.SetStoragePolicy(ctx, authority, owner, input)
	if err != nil || read() != saved || store.StorageLimits() != saved {
		t.Fatalf("owner writer did not adopt canonical policy: %#v, %v", saved, err)
	}
	if err = driver.CheckFloor(ctx); err != nil {
		t.Fatalf("actual owner relaxation was not applied: %v", err)
	}
	stale := input
	stale.RequestID = strings.Repeat("c", 48)
	if _, err = store.SetStoragePolicy(ctx, authority, owner, stale); !errors.Is(err, dvr.ErrConflict) || read() != saved {
		t.Fatalf("stale CAS replaced current policy: %#v, %v", read(), err)
	}
	if _, err = store.SetStoragePolicy(ctx, nil, owner, stale); !errors.Is(err, dvr.ErrDenied) || read() != saved {
		t.Fatalf("missing authority replaced current policy: %#v, %v", read(), err)
	}
	var source string
	if err = db.QueryRow(`SELECT source FROM server_setting_sources WHERE field='dvr-storage:floorBytes'`).Scan(&source); err != nil || source != "server" {
		t.Fatalf("owner source=%q, %v", source, err)
	}
	var consoleRevision int64
	if err = db.QueryRow(`SELECT revision FROM console_settings_revision WHERE singleton=1`).Scan(&consoleRevision); err != nil {
		t.Fatal(err)
	}
	replay, err := store.SetStoragePolicy(ctx, authority, owner, input)
	var afterReplay int64
	if e := db.QueryRow(`SELECT revision FROM console_settings_revision WHERE singleton=1`).Scan(&afterReplay); e != nil {
		t.Fatal(e)
	}
	if err != nil || replay != saved || afterReplay != consoleRevision {
		t.Fatalf("receipt replay rewrote policy/projection: %#v, %d -> %d, %v", replay, consoleRevision, afterReplay, err)
	}
	// Failure after source/outbox writes must roll back all of them along with
	// policy and receipt, without publishing an uncommitted physical policy.
	wakeFailure := errors.New("projection fixture failure")
	store.StoragePolicyChangedTx = func(ctx context.Context, tx *sql.Tx, before, after dvr.StoragePolicy) error {
		if err := QueueDVRStorageConfigurationTx(ctx, tx, before, after); err != nil {
			return err
		}
		return wakeFailure
	}
	_, err = store.SetStoragePolicy(ctx, authority, owner, dvr.StoragePolicyInput{Mutation: dvr.Mutation{RequestID: strings.Repeat("b", 48), ExpectedRevision: saved.Revision}, Policy: dvr.StoragePolicy{CapBytes: 1000000000}})
	if !errors.Is(err, wakeFailure) || read() != saved || store.StorageLimits() != saved {
		t.Fatalf("failed projection escaped transaction: %#v, %v", read(), err)
	}
	if e := db.QueryRow(`SELECT revision FROM console_settings_revision WHERE singleton=1`).Scan(&afterReplay); e != nil || afterReplay != consoleRevision {
		t.Fatalf("failed projection moved console revision: %d, %v", afterReplay, e)
	}
	// A real TOML roundtrip preserves units and the single normalized policy.
	snapshot, err := dbwork.BeginSnapshot(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, b := range domain.Bindings() {
		sources[domain.SourceField(b.Field)] = "server"
	}
	projection, err := domain.Projection(ctx, snapshot.Tx(), sources)
	snapshot.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	file := configfile.New(filepath.Join(state, "storage-policy.toml"))
	if err = file.Patch(projection); err != nil {
		t.Fatal(err)
	}
	decoded := map[string]any{}
	for _, b := range domain.Bindings() {
		value, err := file.Resolve(configfile.Binding{Key: b.Key})
		if err != nil || value.Source != configfile.File {
			t.Fatalf("TOML readback %s: %#v, %v", b.Key, value, err)
		}
		decoded[b.Key] = value.Value
	}
	if err = importValues(decoded, sources); err != nil || read() != saved {
		t.Fatalf("TOML roundtrip changed acknowledged policy: %#v, %v", read(), err)
	}
}

func TestDVRStorageFactoryMatchesActualFreshPolicy(t *testing.T) {
	_, db, _ := newService(t)
	actual, err := dvr.ReadStoragePolicy(context.Background(), db)
	if err != nil || actual != dvr.DefaultStoragePolicy() {
		t.Fatalf("actual fresh policy=%#v, drawn factory=%#v, %v", actual, dvr.DefaultStoragePolicy(), err)
	}
}
