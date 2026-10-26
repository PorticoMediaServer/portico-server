//go:build linux || darwin

package ownedcapture

import (
	"context"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
)

// Regression SOURCES for the Root66 presentation composition: an owned capture
// stays physically retained while a sealed, live presentation snapshot or a live
// private reader pin still needs its bytes, and collection resumes only when the
// last own pin/reference is gone. These tests seed normalized rows with real
// session/catalogue identities and drive the actual collector; they grant no
// publication authority and are not run during campaign work.

type discSeed struct {
	snapshot string
	state    string
	sealed   int
	expires  time.Time
	refUntil time.Time
	pinUntil time.Time
	ref      bool
}

func seedDiscIdentities(t *testing.T, s *Service) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO libraries(id,name,kind,root) VALUES('lib','Library','movie','/library')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO catalog_libraries(library_id,root,name,kind) VALUES('lib','/library','Library',(SELECT id FROM catalog_kinds WHERE name='movie'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO catalog_entities(id,public_id,library_id,kind,title) VALUES(1,randomblob(16),(SELECT id FROM catalog_libraries WHERE library_id='lib'),(SELECT id FROM catalog_kinds WHERE name='movie'),'Held disc')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO catalog_assets(token,path,size,modified_ns,container,video_codec,audio_codec,width,height,duration,available) VALUES('asset','/library/held.iso',1,1,'iso','','',0,0,0,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO playback_sessions(id,session_hash,account_id,profile_id,item_id,asset_id,generation,state,grant_hash,grant_token,expires_at,request_id,duration) VALUES('session','hash','account','profile',1,'asset',1,'playing','grant','token','2099-01-01T00:00:00Z','request',60)`); err != nil {
		t.Fatal(err)
	}
}

func seedDisc(t *testing.T, s *Service, captureID string, d discSeed) {
	t.Helper()
	now := time.Now().UnixMilli()
	if _, err := s.db.Exec(`INSERT INTO playback_disc_snapshots(id,session_id,generation,input_kind) VALUES(?, 'session', (SELECT COALESCE(max(generation),0)+1 FROM playback_disc_snapshots WHERE session_id='session'), 'captured')`, d.snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO playback_disc_lifecycle(snapshot_id,state,sealed,created_ms,expires_ms) VALUES(?,?,?,?,?)`, d.snapshot, d.state, d.sealed, now-time.Minute.Milliseconds(), d.expires.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if d.ref {
		if _, err := s.db.Exec(`INSERT INTO owned_capture_presentation_refs(snapshot_id,capture_id,expires_ms) VALUES(?,?,?)`, d.snapshot, captureID, d.refUntil.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	if !d.pinUntil.IsZero() {
		if _, err := s.db.Exec(`INSERT INTO playback_disc_reader_pins(id,snapshot_id,expires_ms) VALUES(?,?,?)`, d.snapshot+"-pin", d.snapshot, d.pinUntil.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
}

func retainedBytes(t *testing.T, s *Service, captureID string) bool {
	t.Helper()
	var state string
	if err := s.db.QueryRow(`SELECT state FROM owned_capture_lifecycle WHERE capture_id=?`, captureID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state != "deleted"
}

// A settled snapshot that is still sealed and live keeps the original retired
// capture's bytes and its full source charge, for both pending and active.
func TestSealedPresentationRetainsOriginalCaptureBytesAndCharge(t *testing.T) {
	for _, state := range []string{"pending", "active"} {
		t.Run(state, func(t *testing.T) {
			s, p := fixture(t)
			seedDiscIdentities(t, s)
			r := capture(t, s)
			seedDisc(t, s, r.ID, discSeed{snapshot: "snap", state: state, sealed: 1, expires: time.Now().Add(time.Hour), ref: true, refUntil: time.Now().Add(time.Hour)})
			if err := s.Retire(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			if !retainedBytes(t, s, r.ID) {
				t.Fatal("a live sealed presentation lost its original bytes")
			}
			if p.records[r.Binding.ReservationID].released {
				t.Fatal("source quota was released while the presentation still needs the capture")
			}
		})
	}
}

// Retiring one reference does not clear another snapshot that still pins the
// same capture through its own live private reader.
func TestOneRetiredReferenceStillRetainsThroughAnotherSnapshotPin(t *testing.T) {
	s, p := fixture(t)
	seedDiscIdentities(t, s)
	r := capture(t, s)
	seedDisc(t, s, r.ID, discSeed{snapshot: "gone", state: "retired", sealed: 1, expires: time.Now().Add(-time.Minute), ref: true, refUntil: time.Now().Add(time.Hour)})
	seedDisc(t, s, r.ID, discSeed{snapshot: "held", state: "active", sealed: 1, expires: time.Now().Add(time.Hour), ref: true, refUntil: time.Now().Add(time.Hour), pinUntil: time.Now().Add(time.Hour)})
	if err := s.Retire(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM owned_capture_presentation_refs WHERE snapshot_id='gone'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !retainedBytes(t, s, r.ID) {
		t.Fatal("cleanup ran while another snapshot's reader still needs the bytes")
	}
	if p.records[r.Binding.ReservationID].released {
		t.Fatal("source quota was released while another writer pin is live")
	}
	// Releasing only that snapshot's reader and reference lets collection finish.
	now := time.Now().Add(-time.Second).UnixMilli()
	if _, err := s.db.Exec(`UPDATE playback_disc_reader_pins SET expires_ms=? WHERE snapshot_id='held'`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM owned_capture_presentation_refs WHERE snapshot_id='held'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if retainedBytes(t, s, r.ID) {
		t.Fatal("an unreferenced, unpinned retired capture was not collected")
	}
}

// A different capture, and a snapshot that never referenced this one, are never
// touched by this capture's collection.
func TestCollectionLeavesOtherCapturesAndSnapshotsUntouched(t *testing.T) {
	s, _ := fixture(t)
	seedDiscIdentities(t, s)
	first := capture(t, s)
	other := capture(t, s)
	seedDisc(t, s, other.ID, discSeed{snapshot: "other", state: "active", sealed: 1, expires: time.Now().Add(time.Hour), ref: true, refUntil: time.Now().Add(time.Hour)})
	if err := s.Retire(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := s.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if retainedBytes(t, s, first.ID) {
		t.Fatal("an unreferenced retired capture was retained")
	}
	if !retainedBytes(t, s, other.ID) {
		t.Fatal("another capture's retained bytes were collected")
	}
	var snapshots int
	if err := s.db.QueryRow(`SELECT count(*) FROM playback_disc_lifecycle WHERE snapshot_id='other' AND state='active'`).Scan(&snapshots); err != nil || snapshots != 1 {
		t.Fatalf("another snapshot's lifecycle changed: %d %v", snapshots, err)
	}
}

// An expired or retired reference is no access at all: it cannot keep the bytes,
// and metadata state alone never releases the source charge.
func TestExpiredOrRetiredReferenceGrantsNoRetention(t *testing.T) {
	for _, d := range []discSeed{
		{snapshot: "expired", state: "active", sealed: 1, expires: time.Now().Add(time.Hour), ref: true, refUntil: time.Now().Add(-time.Second)},
		{snapshot: "retired", state: "retired", sealed: 1, expires: time.Now().Add(time.Hour), ref: true, refUntil: time.Now().Add(time.Hour)},
		{snapshot: "unsealed", state: "active", sealed: 0, expires: time.Now().Add(time.Hour), ref: true, refUntil: time.Now().Add(time.Hour)},
	} {
		t.Run(d.snapshot, func(t *testing.T) {
			s, p := fixture(t)
			seedDiscIdentities(t, s)
			r := capture(t, s)
			seedDisc(t, s, r.ID, d)
			if err := s.Retire(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			if retainedBytes(t, s, r.ID) {
				t.Fatal("a reference with no live sealed presentation kept the bytes")
			}
			if !p.records[r.Binding.ReservationID].released {
				t.Fatal("the full source charge was not released after collection")
			}
		})
	}
}

// The producer's own retirement still refuses resolution no matter what
// presentation rows still reference the capture.
func TestRetiredProducerRefusesResolutionDespiteReferences(t *testing.T) {
	s, p := fixture(t)
	seedDiscIdentities(t, s)
	r := capture(t, s)
	seedDisc(t, s, r.ID, discSeed{snapshot: "live", state: "active", sealed: 1, expires: time.Now().Add(time.Hour), ref: true, refUntil: time.Now().Add(time.Hour)})
	p.records[r.Binding.ReservationID].live = false
	if err := s.Retire(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	view, err := dbwork.BeginSnapshot(context.Background(), s.db)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Rollback()
	if _, err = s.resolveTx(context.Background(), view.Tx(), r.ID, r.Binding); err == nil {
		t.Fatal("a retired producer still resolved through a presentation reference")
	}
}
