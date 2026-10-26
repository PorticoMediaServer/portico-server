//go:build linux || darwin

package ownedcapture

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

// Regression SOURCES only. The fake producer below is test admission, never a
// production/security qualification: the service still requires it to admit the
// exact request, check the same transaction and reprove the lease.

// ownerScanProducer wraps the real artifact/SQL fixture's producer and resolves
// the private owner-scan precondition. It echoes the requested target into the
// admitted binding unless a test asks for a different one.
type ownerScanProducer struct {
	*policyFixture
	resolve    func(OwnerScanTarget) error
	echoTarget *OwnerScanTarget
	resolves   int
	admits     int
}

func (p *ownerScanProducer) ResolveOwnerScanTx(ctx context.Context, tx *sql.Tx, target OwnerScanTarget) error {
	p.resolves++
	if tx == nil || ctx.Err() != nil {
		return ErrInput
	}
	if p.resolve != nil {
		return p.resolve(target)
	}
	return nil
}

func (p *ownerScanProducer) Admit(ctx context.Context, id string, request Request, destination Destination) (Acquisition, error) {
	p.admits++
	acquisition, err := p.policyFixture.Admit(ctx, id, request, destination)
	if err != nil {
		return nil, err
	}
	echo := OwnerScanTarget{}
	if p.echoTarget != nil {
		echo = *p.echoTarget
	} else if target, ok := request.OwnerScanTarget(); ok {
		echo = target
	}
	a := acquisition.(*fakeAcquisition)
	a.b.OwnerScan = echo
	p.records[a.b.ReservationID].b = a.b
	return a, nil
}

// ownerScanFixture is the ordinary fixture with the resolver-capable wrapper.
func ownerScanFixture(t *testing.T) (*Service, *policyFixture, *ownerScanProducer) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := persistence.OpenFresh(filepath.Join(root, "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	policy := &policyFixture{data: []byte("one complete finite owned ISO byte fixture"), records: map[string]*policyRecord{}}
	producer := &ownerScanProducer{policyFixture: policy}
	service, err := New(Options{DB: db, Directory: filepath.Join(root, "captures"), Producer: producer})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	return service, policy, producer
}

func exactOwnerScanTarget(asset string) OwnerScanTarget {
	return OwnerScanTarget{AdmissionID: "admission-1", JobID: "job-1", ExecutionID: "execution-1", ExecutionRevision: 2,
		ObjectID: "physical-object", ObjectRevision: "observation-1", AssetID: asset, ScanPolicyRevision: 3}
}

func scanRows(t *testing.T, s *Service) int {
	t.Helper()
	return countRows(t, s, "owned_capture_scan_purposes")
}

// A malformed or zero target and any carried outer transaction are refused before
// the resolver, admission or a single byte.
func TestOwnerScanRefusesInvalidTargetsAndOuterTransactionsBeforeAdmission(t *testing.T) {
	s, p, producer := ownerScanFixture(t)
	target := exactOwnerScanTarget("physical-asset")
	changed := func(mutate func(*OwnerScanTarget)) OwnerScanTarget {
		v := target
		mutate(&v)
		return v
	}
	for _, c := range []struct {
		name   string
		target OwnerScanTarget
	}{
		{"zero", OwnerScanTarget{}},
		{"no admission", changed(func(v *OwnerScanTarget) { v.AdmissionID = "" })},
		{"no job", changed(func(v *OwnerScanTarget) { v.JobID = "" })},
		{"no execution", changed(func(v *OwnerScanTarget) { v.ExecutionID = "" })},
		{"no object", changed(func(v *OwnerScanTarget) { v.ObjectID = "" })},
		{"zero execution revision", changed(func(v *OwnerScanTarget) { v.ExecutionRevision = 0 })},
		{"zero policy revision", changed(func(v *OwnerScanTarget) { v.ScanPolicyRevision = 0 })},
		{"malformed asset", changed(func(v *OwnerScanTarget) { v.AssetID = "not an asset" })},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := s.CaptureOwnerScan(context.Background(), c.target); !errors.Is(err, ErrInput) {
				t.Fatal("invalid owner-scan target was not refused as input", err)
			}
		})
	}
	if producer.resolves != 0 || producer.admits != 0 || p.reads != 0 {
		t.Fatal("an invalid target reached the resolver or admission")
	}
	write, err := dbwork.Begin(context.Background(), s.db, dbwork.ClassInteractive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CaptureOwnerScan(write.Context(), target); !errors.Is(err, ErrInput) {
		write.Rollback()
		t.Fatal("owner scan joined an outer write transaction", err)
	}
	write.Rollback()
	view, err := dbwork.BeginSnapshot(context.Background(), s.db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CaptureOwnerScan(view.Context(), target); !errors.Is(err, ErrInput) {
		view.Rollback()
		t.Fatal("owner scan joined an outer read transaction", err)
	}
	view.Rollback()
	if producer.resolves != 0 || producer.admits != 0 || p.reads != 0 || len(p.records) != 0 {
		t.Fatal("an outer transaction reached the resolver or admission")
	}
}

// Without the optional producer, or with a resolver that refuses, no admission or
// read is attempted.
func TestOwnerScanWithoutResolverOrWithARefusedResolverAdmitsNothing(t *testing.T) {
	plain, plainPolicy := fixture(t)
	target := exactOwnerScanTarget("physical-asset")
	if _, err := plain.CaptureOwnerScan(context.Background(), target); !errors.Is(err, ErrCurrent) {
		t.Fatal("a store without the optional producer accepted an owner scan", err)
	}
	if len(plainPolicy.records) != 0 || plainPolicy.reads != 0 {
		t.Fatal("missing producer reached admission")
	}
	s, p, producer := ownerScanFixture(t)
	producer.resolve = func(OwnerScanTarget) error { return ErrCurrent }
	if _, err := s.CaptureOwnerScan(context.Background(), target); !errors.Is(err, ErrCurrent) {
		t.Fatal("a refused resolver still started the capture", err)
	}
	if producer.admits != 0 || p.reads != 0 || len(p.records) != 0 || scanRows(t, s) != 0 {
		t.Fatal("a refused resolver reached admission or bytes")
	}
}

// The private request is bound to the service and asset it was sealed for.
func TestOwnerScanCopiedRequestCannotChangeStoreOrAsset(t *testing.T) {
	s, p, producer := ownerScanFixture(t)
	other, _, _ := ownerScanFixture(t)
	target := exactOwnerScanTarget("physical-asset")
	for _, c := range []struct {
		name    string
		request Request
	}{
		{"another service", Request{AssetID: target.AssetID, ownerScan: &ownerScanRequest{service: other, target: target}}},
		{"another asset", Request{AssetID: "other-asset", ownerScan: &ownerScanRequest{service: s, target: target}}},
		{"no target", Request{AssetID: target.AssetID, ownerScan: &ownerScanRequest{service: s}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := s.Capture(context.Background(), c.request); !errors.Is(err, ErrInput) {
				t.Fatal("a copied sealed request was accepted", err)
			}
		})
	}
	if producer.admits != 0 || p.reads != 0 || len(p.records) != 0 {
		t.Fatal("a copied sealed request reached admission")
	}
}

// The accepted producer must echo the exact target in the binding it admits.
func TestOwnerScanAdmissionMustEchoTheExactTarget(t *testing.T) {
	target := exactOwnerScanTarget("physical-asset")
	otherExecution := target
	otherExecution.ExecutionID = "execution-2"
	otherPolicy := target
	otherPolicy.ScanPolicyRevision = 4
	for _, c := range []struct {
		name string
		echo *OwnerScanTarget
	}{
		{"zero binding", &OwnerScanTarget{}},
		{"different execution", &otherExecution},
		{"different policy", &otherPolicy},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, p, producer := ownerScanFixture(t)
			producer.echoTarget = c.echo
			if _, err := s.CaptureOwnerScan(context.Background(), target); !errors.Is(err, ErrInput) {
				t.Fatal("a mismatched admitted binding was accepted", err)
			}
			if p.reads != 0 {
				t.Fatal("bytes were read before the admitted binding was checked")
			}
			if countRows(t, s, "owned_capture_receipts") != 0 || scanRows(t, s) != 0 || countRows(t, s, "owned_capture_sources") != 0 {
				t.Fatal("a mismatched admitted binding persisted rows")
			}
		})
	}
}

// A successful admission stores the normalized purpose once, keeps it immutable,
// and the exact binding survives Open and the reader transaction checks.
func TestOwnerScanCaptureStoresTheExactPurposeAndReadsBack(t *testing.T) {
	s, p, _ := ownerScanFixture(t)
	target := exactOwnerScanTarget("physical-asset")
	receipt, err := s.CaptureOwnerScan(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Binding.OwnerScan != target {
		t.Fatal("receipt did not keep the exact owner-scan target")
	}
	var stored OwnerScanTarget
	row := s.db.QueryRow(`SELECT admission_id,job_id,execution_id,execution_revision,object_id,object_revision,asset_id,scan_policy_revision FROM owned_capture_scan_purposes WHERE capture_id=?`, receipt.ID)
	if err = row.Scan(&stored.AdmissionID, &stored.JobID, &stored.ExecutionID, &stored.ExecutionRevision, &stored.ObjectID, &stored.ObjectRevision, &stored.AssetID, &stored.ScanPolicyRevision); err != nil {
		t.Fatal(err)
	}
	if stored != target {
		t.Fatal("stored purpose was not the admitted exact target")
	}
	if _, err = dbwork.ExecWrite(context.Background(), s.db, dbwork.ClassMaintenance, `UPDATE owned_capture_scan_purposes SET execution_id='replacement' WHERE capture_id=?`, receipt.ID); err == nil {
		t.Fatal("stored owner-scan purpose was mutable")
	}
	want := framed("portico.owned-capture.owner-scan.scope.v1", struct {
		Binding   Binding
		OwnerScan OwnerScanTarget
	}{receipt.Binding, target})
	if got := receipt.Version.Evidence().Scope; got != want {
		t.Fatal("scan version scope did not bind the exact purpose", got)
	}
	reader, err := s.Open(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err = reader.Check(context.Background()); err != nil {
		t.Fatal("the exact scan receipt did not pass the reader check", err)
	}
	view, err := dbwork.BeginSnapshot(context.Background(), s.db)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Rollback()
	if err = reader.CheckTx(view.Context(), view.Tx()); err != nil {
		t.Fatal("the exact scan receipt did not pass the reader transaction check", err)
	}
	if p.reads == 0 {
		t.Fatal("no actual bytes were captured")
	}
}

// Once the original purpose stops resolving, the exact receipt is refused and no
// latest target replaces it.
func TestOwnerScanChangedPurposeAfterReadyRefusesTheOriginalReceipt(t *testing.T) {
	s, _, producer := ownerScanFixture(t)
	target := exactOwnerScanTarget("physical-asset")
	receipt, err := s.CaptureOwnerScan(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := s.Open(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	producer.resolve = func(candidate OwnerScanTarget) error {
		if candidate != target {
			return nil
		}
		return ErrCurrent
	}
	if err = reader.Check(context.Background()); err == nil {
		t.Fatal("a retired exact purpose still passed the reader check")
	}
	if reopened, err := s.Open(context.Background(), receipt); err == nil {
		reopened.Close()
		t.Fatal("a retired exact purpose reopened the receipt")
	}
	if _, err := s.CaptureOwnerScan(context.Background(), target); !errors.Is(err, ErrCurrent) {
		t.Fatal("the retired exact purpose was captured again", err)
	}
	// A different execution/policy is a distinct purpose; it must not revive the
	// original receipt or reuse its row.
	next := target
	next.ExecutionID, next.ScanPolicyRevision = "execution-2", target.ScanPolicyRevision+1
	fresh, err := s.CaptureOwnerScan(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID == receipt.ID || fresh.Version.ID() == receipt.Version.ID() {
		t.Fatal("a new purpose reused the retired capture identity")
	}
	if reopened, err := s.Open(context.Background(), receipt); err == nil {
		reopened.Close()
		t.Fatal("a new purpose revived the retired receipt")
	}
	if scanRows(t, s) != 2 {
		t.Fatal("the retired purpose and the new one did not keep separate rows")
	}
}

// Identical bytes and the same capture identity still get a distinct version scope
// once the execution or policy target changes, while a normal capture keeps the
// legacy serialization.
func TestOwnerScanVersionScopeIsDistinctFromNormalAndFromAnotherPurpose(t *testing.T) {
	s, _, _ := ownerScanFixture(t)
	target := exactOwnerScanTarget("physical-asset")
	scanned, err := s.CaptureOwnerScan(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	normal, err := s.Capture(context.Background(), Request{AssetID: target.AssetID})
	if err != nil {
		t.Fatal(err)
	}
	if normal.Version.ID() == scanned.Version.ID() {
		t.Fatal("a normal capture reused the scan version")
	}
	if got, want := normal.Version.Evidence().Scope, framed("portico.owned-capture.scope.v1", normal.Binding); got != want {
		t.Fatal("normal capture scope left its legacy serialization", got, want)
	}
	if normal.Binding.OwnerScan != (OwnerScanTarget{}) {
		t.Fatal("a normal capture carried an owner-scan purpose")
	}
	for _, change := range []func(*OwnerScanTarget){
		func(v *OwnerScanTarget) { v.ExecutionID = "execution-2" },
		func(v *OwnerScanTarget) { v.ScanPolicyRevision++ },
	} {
		next := scanned.Binding
		changed := target
		change(&changed)
		next.OwnerScan = changed
		version, err := captureVersion(scanned.ID, next, scanned.Object)
		if err != nil {
			t.Fatal(err)
		}
		if version.ID() == scanned.Version.ID() {
			t.Fatal("a changed execution/policy target reused the same scan version scope")
		}
		if version.Evidence().Scope == scanned.Version.Evidence().Scope {
			t.Fatal("a changed execution/policy target reused the same scan scope")
		}
	}
}
