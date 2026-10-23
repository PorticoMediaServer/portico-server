package remotesources

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/storage"
)

func captureOwnerSource(t *testing.T, f observationFixture, ref ObservationRef, identities *identity.Service, owner identity.Principal, purpose SourceReadPurpose, changed func()) *SourceReadLease {
	t.Helper()
	tx, err := dbwork.BeginSnapshot(context.Background(), f.s.db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	lease, err := f.s.CaptureReadSourceTx(context.Background(), tx.Tx(), SourceSessionAdmission{Purpose: purpose, Observation: ref, OnChanged: changed, CheckTx: func(ctx context.Context, tx *sql.Tx, binding SourceReadBinding) error {
		if binding.Path != f.file || binding.Observation != ref || binding.Purpose != purpose {
			return storage.ErrRemoteChanged
		}
		return currentLocalOwnerTx(ctx, tx, identities, owner)
	}})
	if err != nil {
		t.Fatal(err)
	}
	return lease
}
func TestSourceSessionCaptureDoesNotOpenProviderOrInferPermission(t *testing.T) {
	f := newObservationFixture(t)
	identities, owner, _ := observationOwner(t, f)
	ref, err := f.record(t, scanFacts("capture"), 0)
	if err != nil {
		t.Fatal(err)
	}
	lease := captureOwnerSource(t, f, ref, identities, owner, SourceReading, func() {})
	defer lease.Close()
	binding := lease.Binding()
	if binding.Path != f.file || binding.Size != 16 || binding.LeaseID < 1 || binding.Profile.Saved || binding.Profile.ScanDepth != "file_list_only" {
		t.Fatal(binding)
	}
	if len(f.s.serveLeases) != 0 {
		t.Fatal("DB capture launched a source process")
	}
	if err = lease.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := captureOwnerSource(t, f, ref, identities, owner, SourceReading, func() {})
	defer second.Close()
	if second.Binding().LeaseID == binding.LeaseID {
		t.Fatal("logical lease identity reused")
	}
	tx, err := dbwork.BeginSnapshot(context.Background(), f.s.db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = f.s.CaptureReadSourceTx(context.Background(), tx.Tx(), SourceSessionAdmission{Purpose: SourceReading, Observation: ref}); err == nil {
		t.Fatal("missing real authority defaulted to permission")
	}
	if _, err = f.s.CaptureReadSourceTx(context.Background(), tx.Tx(), SourceSessionAdmission{Purpose: "analysis", Observation: ref, CheckTx: func(context.Context, *sql.Tx, SourceReadBinding) error {
		t.Fatal("pool label dispatched authority")
		return nil
	}, OnChanged: func() {}}); err == nil {
		t.Fatal("pool label became operation permission")
	}
}
func TestSourceSessionPublicationCheckIsLocalAndABAIsTerminal(t *testing.T) {
	f := newObservationFixture(t)
	identities, owner, _ := observationOwner(t, f)
	ref, err := f.record(t, scanFacts("publication"), 0)
	if err != nil {
		t.Fatal(err)
	}
	var writerHeld atomic.Bool
	notices := 0
	lease := captureOwnerSource(t, f, ref, identities, owner, SourceReading, func() {
		if writerHeld.Load() {
			t.Error("publication transaction invoked external callback")
		}
		notices++
	})
	defer lease.Close()
	tx, err := dbwork.Begin(context.Background(), f.s.db, dbwork.ClassBackgroundMedia)
	if err != nil {
		t.Fatal(err)
	}
	writerHeld.Store(true)
	if _, err = tx.Tx().Exec(`UPDATE remote_scan_observations SET revision=revision+1`); err != nil {
		t.Fatal(err)
	}
	if err = lease.CheckTx(context.Background(), tx.Tx()); !errors.Is(err, storage.ErrRemoteChanged) {
		t.Fatal("publication missed current observation", err)
	}
	tx.Rollback()
	writerHeld.Store(false)
	if notices != 0 {
		t.Fatal("transaction delivered a notice", notices)
	}
	if err = lease.Check(context.Background()); !errors.Is(err, storage.ErrRemoteChanged) {
		t.Fatal("A-to-B-to-A revived old logical lease", err)
	}
	if err = lease.Check(context.Background()); !errors.Is(err, storage.ErrRemoteChanged) || notices != 1 {
		t.Fatal(err, notices)
	}
}
func TestSourceSessionCurrentIdentityRevocationClosesLease(t *testing.T) {
	f := newObservationFixture(t)
	identities, owner, family := observationOwner(t, f)
	ref, err := f.record(t, scanFacts("actor"), 0)
	if err != nil {
		t.Fatal(err)
	}
	lease := captureOwnerSource(t, f, ref, identities, owner, SourceReading, func() { t.Error("actor revocation reported a file replacement") })
	defer lease.Close()
	if _, err = f.s.db.Exec(`UPDATE authorization_session_families SET revoked=1 WHERE id=?`, family); err != nil {
		t.Fatal(err)
	}
	if err = lease.Check(context.Background()); err == nil {
		t.Fatal("revoked family retained source")
	}
	if _, err = f.s.db.Exec(`UPDATE authorization_session_families SET revoked=0 WHERE id=?`, family); err != nil {
		t.Fatal(err)
	}
	if err = lease.Check(context.Background()); err == nil {
		t.Fatal("old lease revived after authority ABA")
	}
}
func TestServeTransportHasNoProxyCommandOrNativeFallback(t *testing.T) {
	for _, backend := range []string{"s3", "b2", "drive", "onedrive", "dropbox", "webdav", "http", "sftp", "smb"} {
		raw := "[active]\ntype=" + backend + "\nhost=owner.example\nknown_hosts_file=/retained/owner-file\n"
		actual, host, port, err := serveTransport([]byte(raw), "active:path")
		if err != nil || actual != backend {
			t.Fatal(backend, actual, err)
		}
		if backend == "sftp" && (host != "owner.example" || port != "22") {
			t.Fatal(host, port)
		}
		if backend == "smb" && (host != "owner.example" || port != "445") {
			t.Fatal(host, port)
		}
	}
	for _, extra := range []string{"http_proxy=http://proxy.example", "ssh=external -oProxyCommand=bad", "unix_socket=/private/socket", "password_command=bad", "auth_redirect=true", "override.http_proxy=", "override.max_connections=0"} {
		if _, _, _, err := serveTransport([]byte("[active]\ntype=sftp\nhost=owner.example\n"+extra+"\n"), "active:"); err == nil {
			t.Fatal("escape hatch admitted", extra)
		}
	}
	for _, raw := range []string{"[active]\ntype=unknown\n", "[active]\ntype=crypt\nremote=active:\n", "[active]\ntype=http\ntype=sftp\n"} {
		if _, _, _, err := serveTransport([]byte(raw), "active:"); err == nil {
			t.Fatal("unknown transport became permission", strings.Split(raw, "\n")[1])
		}
	}
	actual, _, _, err := serveTransport([]byte("[active]\ntype=crypt\nremote=underlying:path\n[underlying]\ntype=drive\n"), "active:")
	if err != nil || actual != "drive" {
		t.Fatal("configured crypt chain", actual, err)
	}
}
