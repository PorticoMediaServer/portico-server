package optimizedartifact

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeReservation struct{ released *int }

func (r fakeReservation) Release() { *r.released++ }

type fakeFile struct {
	bytes.Buffer
	fs   *fakeFS
	path string
}

func (f *fakeFile) Sync() error { return f.fs.fail("sync") }
func (f *fakeFile) Close() error {
	if err := f.fs.fail("close"); err != nil {
		return err
	}
	f.fs.files[f.path] = append([]byte(nil), f.Bytes()...)
	return nil
}

type fakeFS struct {
	files    map[string][]byte
	markers  map[string]Marker
	failAt   string
	releases int
}

func newFakeFS() *fakeFS { return &fakeFS{files: map[string][]byte{}, markers: map[string]Marker{}} }
func (f *fakeFS) fail(at string) error {
	if f.failAt == at {
		return errors.New("/private/secret/path: injected")
	}
	return nil
}
func (f *fakeFS) Reserve(context.Context, string, int64) (Reservation, error) {
	if e := f.fail("reserve"); e != nil {
		return nil, e
	}
	return fakeReservation{&f.releases}, nil
}
func (f *fakeFS) CreatePrivate(p string) (PrivateFile, error) {
	if e := f.fail("create"); e != nil {
		return nil, e
	}
	return &fakeFile{fs: f, path: p}, nil
}
func (f *fakeFS) Rename(a, b string) error {
	if e := f.fail("rename"); e != nil {
		return e
	}
	f.files[b] = f.files[a]
	delete(f.files, a)
	return nil
}
func (f *fakeFS) SyncDirectory(string) error    { return f.fail("dirsync") }
func (f *fakeFS) Remove(p string) error         { delete(f.files, p); return nil }
func (f *fakeFS) Exists(p string) (bool, error) { _, ok := f.files[p]; return ok, f.fail("exists") }
func (f *fakeFS) PutMarker(_ context.Context, m Marker) error {
	if e := f.fail("marker_" + string(m.Stage)); e != nil {
		return e
	}
	f.markers[m.ID] = m
	return nil
}
func (f *fakeFS) DeleteMarker(_ context.Context, id string) error { delete(f.markers, id); return nil }

type fakeStore struct {
	current *Metadata
	fail    bool
}

func (s *fakeStore) Publish(_ context.Context, m Metadata) (*Metadata, error) {
	if s.fail {
		return nil, errors.New("secret database detail")
	}
	old := s.current
	s.current = &m
	return old, nil
}
func (s *fakeStore) Current(context.Context, string, string) (*Metadata, error) {
	return s.current, nil
}

func request() Request {
	return Request{Identity: IdentityInput{Root: "artifact-root", MediaID: "movie/private", PresetVersion: "universal-v2", SourceFingerprint: "source-secret", PlanDigest: "plan-secret", Extension: "mp4"}, PredictedBytes: 100, Produce: func(_ context.Context, w io.Writer) error { _, e := w.Write([]byte("media")); return e }, Validate: func(context.Context, string) (int64, error) { return 5, nil }, Now: func() time.Time { return time.Unix(100, 0) }}
}

func TestIdentityIsDeterministicAndOpaque(t *testing.T) {
	a, e := DeriveIdentity(request().Identity)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := DeriveIdentity(request().Identity)
	if a != b {
		t.Fatal("identity changed")
	}
	for _, secret := range []string{"movie", "private", "source-secret", "plan-secret"} {
		if strings.Contains(a.FinalPath, secret) {
			t.Fatalf("path leaked %q", secret)
		}
	}
}

func TestPublishOrdersDurabilityAndRetainsPreviousUntilCommit(t *testing.T) {
	fs := newFakeFS()
	old := &Metadata{GenerationID: "old", Path: "old-path"}
	store := &fakeStore{current: old}
	got, err := (Publisher{fs, store}).Publish(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if got.Previous != old || store.current.GenerationID != got.Metadata.GenerationID {
		t.Fatal("metadata transaction result incorrect")
	}
	if _, ok := fs.files["old-path"]; ok {
		t.Fatal("publisher must not delete prior generation")
	}
	if fs.releases != 1 {
		t.Fatal("reservation not released")
	}
	if len(fs.markers) != 0 {
		t.Fatal("marker retained")
	}
}

func TestEveryPublicationFailureIsSanitizedAndReleasesReservation(t *testing.T) {
	for _, point := range []string{"create", "marker_temp_created", "sync", "close", "marker_temp_synced", "marker_validated", "rename", "marker_renamed", "dirsync", "marker_directory_synced"} {
		t.Run(point, func(t *testing.T) {
			fs := newFakeFS()
			fs.failAt = point
			_, err := (Publisher{fs, &fakeStore{}}).Publish(context.Background(), request())
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "/") {
				t.Fatalf("private detail leaked: %v", err)
			}
			if fs.releases != 1 {
				t.Fatal("reservation leak")
			}
		})
	}
}

func TestProducerValidationAndMetadataFailuresRemainRecoverable(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(*Request, *fakeStore)
		wantFinal bool
	}{
		{"producer", func(r *Request, _ *fakeStore) {
			r.Produce = func(context.Context, io.Writer) error { return errors.New("/secret/source") }
		}, false},
		{"validation", func(r *Request, _ *fakeStore) {
			r.Validate = func(context.Context, string) (int64, error) { return 0, errors.New("/secret/probe") }
		}, false},
		{"metadata", func(_ *Request, s *fakeStore) { s.fail = true }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, store, req := newFakeFS(), &fakeStore{}, request()
			tc.edit(&req, store)
			_, err := (Publisher{fs, store}).Publish(context.Background(), req)
			if err == nil || strings.Contains(err.Error(), "/") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsanitized outcome: %v", err)
			}
			id, _ := DeriveIdentity(req.Identity)
			_, final := fs.files[id.FinalPath]
			if final != tc.wantFinal {
				t.Fatalf("final presence=%v want %v", final, tc.wantFinal)
			}
			if fs.releases != 1 {
				t.Fatal("reservation leak")
			}
		})
	}
}

func TestENOSPCUsesStablePublicOutcome(t *testing.T) {
	fs := newFakeFS()
	fs.failAt = "reserve" // convert adapter failure to sentinel explicitly
	fs2 := &noSpaceFS{fakeFS: fs}
	_, err := (Publisher{fs2, &fakeStore{}}).Publish(context.Background(), request())
	if err.Error() != "insufficient_space" {
		t.Fatalf("got %v", err)
	}
}

type noSpaceFS struct{ *fakeFS }

func (f *noSpaceFS) Reserve(context.Context, string, int64) (Reservation, error) {
	return nil, ErrNoSpace
}

func TestReconcileCompletesDurableRenameOrCleansEarlierCrash(t *testing.T) {
	r := request()
	id, _ := DeriveIdentity(r.Identity)
	meta := Metadata{GenerationID: id.GenerationID, MediaID: r.Identity.MediaID, PresetVersion: r.Identity.PresetVersion, Path: id.FinalPath}
	fs := newFakeFS()
	fs.files[id.FinalPath] = []byte("media")
	fs.markers[id.MarkerID] = Marker{ID: id.MarkerID, Stage: StageDirSynced, Metadata: meta, TempPath: id.TempPath, FinalPath: id.FinalPath}
	store := &fakeStore{}
	out, err := (Publisher{fs, store}).Reconcile(context.Background(), fs.markers[id.MarkerID])
	if err != nil || out != ReconcileCommitted || store.current == nil {
		t.Fatalf("completion: %v %v", out, err)
	}
	fs.files[id.FinalPath] = []byte("uncommitted")
	m := Marker{ID: id.MarkerID, Stage: StageRenamed, Metadata: meta, TempPath: id.TempPath, FinalPath: id.FinalPath}
	out, err = (Publisher{fs, &fakeStore{}}).Reconcile(context.Background(), m)
	if err != nil || out != ReconcileCleaned {
		t.Fatalf("cleanup: %v %v", out, err)
	}
	if _, ok := fs.files[id.FinalPath]; ok {
		t.Fatal("uncommitted artifact remains")
	}
}

func TestRetentionIsBoundedAndLeaseSafe(t *testing.T) {
	now := time.Unix(1000, 0)
	old := Superseded{PublishedAt: now.Add(-time.Hour)}
	if RetainSuperseded(old, now, time.Minute) {
		t.Fatal("expired generation retained")
	}
	old.ActiveLeases = 1
	if !RetainSuperseded(old, now, time.Minute) {
		t.Fatal("leased generation removed")
	}
	if got := DecideSupersededRetention(Superseded{PublishedAt: now.Add(-24 * time.Hour)}, now, RetentionPolicy{MinimumAge: 48 * time.Hour, MaximumAge: time.Hour}); got != RetentionDelete {
		t.Fatalf("maximum bound ignored: %s", got)
	}
	if got := DecideSupersededRetention(old, now, RetentionPolicy{MaximumAge: time.Minute}); got != RetentionDeferLeased {
		t.Fatalf("active lease was not deferred: %s", got)
	}
}
