package rcloneprogram

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// This child models process custody and authenticated readiness only. It is
// deliberately not rclone or proof of provider, cache or egress behavior.
func TestServeProcessChild(t *testing.T) {
	mode := os.Getenv("PORTICO_TEST_SERVE_PROCESS")
	if mode == "" {
		return
	}
	if mode == "crash" {
		os.Exit(3)
	}
	var address, config string
	for i, arg := range os.Args {
		if i+1 < len(os.Args) {
			switch arg {
			case "--addr":
				address = os.Args[i+1]
			case "--config":
				config = os.Args[i+1]
			}
		}
	}
	if _, err := os.Stat(config); err != nil {
		os.Exit(4)
	}
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		os.Exit(5)
	}
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if mode == "unauthorized" || !ok || user != os.Getenv("RCLONE_USER") || pass != os.Getenv("RCLONE_PASS") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != "PROPFIND" || r.Header.Get("Depth") != "0" || r.URL.Path != "/" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<d:multistatus xmlns:d="DAV:"><d:response><d:href>/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`)
	})}
	_ = server.Serve(listener)
	os.Exit(6)
}

type processLeaseFixture struct {
	life     context.Context
	released atomic.Int32
	refused  atomic.Bool
}

func (f *processLeaseFixture) Context() context.Context { return f.life }
func (f *processLeaseFixture) Check(ctx context.Context) error {
	if f.refused.Load() {
		return ErrServeStopped
	}
	return ctx.Err()
}
func (f *processLeaseFixture) Release() { f.released.Add(1) }

func serveProcessFixture(t *testing.T, mode string) (ServeCommandSpec, *processLeaseFixture, context.CancelFunc) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	life, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	lease := &processLeaseFixture{life: life}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spec := ServeCommandSpec{SourceID: "fixture", SourceGeneration: "1", PrivateDirectory: t.TempDir(), Configuration: []byte("[media]\ntype=local\n"), Remote: "media:", Port: port, Username: "private", Password: "test-only", Connections: 2, RequestsPerSecond: 2,
		Enforce: func(cmd *exec.Cmd) error {
			// Test-only executable substitution. Production still requires a real
			// enforcing outbound boundary; this callback provides no such proof.
			cmd.Path = executable
			cmd.Args = append([]string{executable, "-test.run=^TestServeProcessChild$", "--"}, cmd.Args[1:]...)
			cmd.Env = append(cmd.Env, "PORTICO_TEST_SERVE_PROCESS="+mode)
			return nil
		}}
	return spec, lease, cancel
}

func TestServeProcessCustodyAndRevocation(t *testing.T) {
	for _, termination := range []string{"stop", "lease", "parent"} {
		t.Run(termination, func(t *testing.T) {
			spec, lease, revoke := serveProcessFixture(t, "ready")
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			process, err := StartServe(parent, Program{Path: "/test-substituted", Included: true}, spec, lease)
			if err != nil {
				t.Fatal(err)
			}
			defer process.Stop()
			entries, err := os.ReadDir(spec.PrivateDirectory)
			if err != nil || len(entries) != 0 {
				t.Fatal("transient config still present at publication", entries, err)
			}
			if process.Generation() == "" || lease.released.Load() != 0 {
				t.Fatal("physical slot released before Wait")
			}
			if _, err = process.Endpoint(parent); err != nil {
				t.Fatal(err)
			}
			switch termination {
			case "stop":
				process.Stop()
			case "lease":
				revoke()
			case "parent":
				cancel()
			}
			wait, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer waitCancel()
			if err = process.Wait(wait); !errors.Is(err, ErrServeStopped) {
				t.Fatal("child not reaped", err)
			}
			if lease.released.Load() != 1 {
				t.Fatal("lease release count", lease.released.Load())
			}
			if _, err = process.Endpoint(context.Background()); err == nil {
				t.Fatal("retired endpoint was published again")
			}
		})
	}
}

func TestServeProcessStartupFailureRetiresLeaseAndConfiguration(t *testing.T) {
	for _, mode := range []string{"crash", "unauthorized", "revoked", "missing_executable"} {
		t.Run(mode, func(t *testing.T) {
			spec, lease, _ := serveProcessFixture(t, mode)
			if mode == "revoked" {
				lease.refused.Store(true)
			}
			if mode == "missing_executable" {
				spec.Enforce = func(cmd *exec.Cmd) error { cmd.Path = filepath.Join(t.TempDir(), "missing"); return nil }
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			process, err := StartServe(ctx, Program{Path: "/test-substituted", Included: true}, spec, lease)
			if err == nil || process != nil || lease.released.Load() != 1 {
				t.Fatal("startup failure leaked process ownership", process, err, lease.released.Load())
			}
			entries, readErr := os.ReadDir(spec.PrivateDirectory)
			if readErr != nil || len(entries) != 0 {
				t.Fatal("startup failure retained private configuration", entries, readErr)
			}
			if strings.Contains(err.Error(), "test-only") {
				t.Fatal("credential in error")
			}
		})
	}
}
