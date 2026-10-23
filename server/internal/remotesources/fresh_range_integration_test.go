//go:build rcloneintegration

package remotesources

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/rcloneprogram"
	"portico.local/server/internal/remotemedia"
)

type freshProcessLeaseFixture struct {
	ctx      context.Context
	released atomic.Int32
}

func (f *freshProcessLeaseFixture) Context() context.Context        { return f.ctx }
func (f *freshProcessLeaseFixture) Check(ctx context.Context) error { return ctx.Err() }
func (f *freshProcessLeaseFixture) Release()                        { f.released.Add(1) }

func TestFreshRangeRealMetadataAndRetainedRevocations(t *testing.T) {
	program := os.Getenv("PORTICO_RCLONE_TEST_PROGRAM")
	if program == "" {
		t.Fatal("PORTICO_RCLONE_TEST_PROGRAM must name the locked rclone executable")
	}
	lockRaw, err := os.ReadFile("../../../third_party/rclone/bundle.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Targets map[string]struct{ Files map[string]string }
	}
	if err = json.Unmarshal(lockRaw, &lock); err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(program)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(binary)
	if hex.EncodeToString(digest[:]) != lock.Targets["linux-amd64"].Files["rclone"] {
		t.Fatal("unqualified integration executable")
	}
	clear(binary)
	for _, changed := range []string{"mtime", "observation", "root", "actor", "process"} {
		t.Run(changed, func(t *testing.T) {
			f := newObservationFixture(t)
			identities, owner, family := observationOwner(t, f)
			if err := os.WriteFile(f.file, bytes.Repeat([]byte("A"), 16), 0600); err != nil {
				t.Fatal(err)
			}
			mtime := time.Unix(1700000000, 23)
			if err := os.Chtimes(f.file, mtime, mtime); err != nil {
				t.Fatal(err)
			}
			port := func() int {
				listener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				p := listener.Addr().(*net.TCPAddr).Port
				_ = listener.Close()
				return p
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			lease := &freshProcessLeaseFixture{ctx: ctx}
			spec := rcloneprogram.ServeCommandSpec{SourceID: "remote", SourceGeneration: "1", PrivateDirectory: t.TempDir(), Configuration: []byte("[fixture]\ntype=local\n"), Remote: "fixture:" + filepath.Dir(f.file), Port: port(), MetadataPort: port(), Username: "private", Password: "fixture-only", Connections: 1, RequestsPerSecond: 2,
				Enforce: func(*exec.Cmd) error { return nil }} // Local fixture only; no egress/production-authority claim.
			process, err := rcloneprogram.StartServe(ctx, rcloneprogram.Program{Path: program, Included: true}, spec, lease)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				process.Stop()
				wait, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := process.Wait(wait); !errors.Is(err, rcloneprogram.ErrServeStopped) || lease.released.Load() != 1 {
					t.Error("process custody", err, lease.released.Load())
				}
			})
			check := func(ctx context.Context) error {
				snapshot, err := dbwork.BeginSnapshot(ctx, f.s.db)
				if err != nil {
					return err
				}
				defer snapshot.Rollback()
				return currentLocalOwnerTx(ctx, snapshot.Tx(), identities, owner)
			}
			metadata, err := process.Metadata(ctx)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := metadata.Stat(ctx, "film.mkv", check)
			if err != nil {
				t.Fatal(err)
			}
			endpoint, err := process.Endpoint(ctx)
			if err != nil {
				t.Fatal(err)
			}
			served, err := endpoint.Stat(ctx, "film.mkv", check)
			if err != nil {
				t.Fatal(err)
			}
			facts := remotemedia.FileObservation{SourceID: "remote", Incarnation: "1", Object: "film.mkv", ObjectID: fresh.ID, Size: fresh.Size, ModifiedNS: fresh.Modified.UnixNano(), ModifiedKnown: true, ProviderHashes: fresh.Hashes, ServeETag: served.ETag}
			ref, err := f.record(t, ScanObservation{Facts: facts, ScanRevision: "1:actual-local-fixture"}, 0)
			if err != nil {
				t.Fatal(err)
			}
			notices := 0
			session, err := f.s.OpenFreshRange(ctx, process, FreshRangeAdmission{Observation: ref, ProcessGeneration: process.Generation(), Check: check, OnChanged: func() { notices++ }})
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			// Exercise an actual transport reconnect at the minimum connection
			// profile. Its fresh observation cannot wait on the range dial slot.
			endpoint.Close()
			p := make([]byte, 4)
			if n, err := session.ReadAt(ctx, p, 8); err != nil || n != 4 || string(p) != "AAAA" {
				t.Fatal("real admitted range", n, string(p), err)
			}
			switch changed {
			case "mtime":
				if err = os.Chtimes(f.file, mtime.Add(time.Second), mtime.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			case "observation":
				_, err = f.s.db.Exec(`UPDATE remote_scan_observations SET revision=revision+1`)
			case "root":
				_, err = f.s.db.Exec(`UPDATE remote_sources SET root=?`, t.TempDir())
			case "actor":
				_, err = f.s.db.Exec(`UPDATE authorization_session_families SET revoked=1 WHERE id=?`, family)
			case "process":
				process.Stop()
				_ = process.Wait(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			p = []byte("----")
			for range 2 {
				if n, err := session.ReadAt(ctx, p, 8); n != 0 || err == nil || string(p) != "----" {
					t.Fatal("revoked range resumed", changed, n, string(p), err)
				}
			}
			if changed == "mtime" || changed == "observation" || changed == "root" {
				if notices != 1 {
					t.Fatal("rescan notification count", notices)
				}
			}
		})
	}
}
