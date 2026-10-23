//go:build rcloneintegration && linux

package remotesources

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"portico.local/server/internal/childprocess"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/mounts"
	"portico.local/server/internal/rcloneprogram"
)

// An actual owner command saves a real read-only NAS connection, then the
// retained shared serve producer reads fresh metadata. No content/decode grant
// or immutable-version claim follows from this connection-only acceptance.
func TestNativeSavedNASConnectionAndForbiddenEdits(t *testing.T) {
	for _, backend := range []string{"sftp", "smb"} {
		t.Run(backend, func(t *testing.T) { testNativeSavedNAS(t, backend) })
	}
}

func testNativeSavedNAS(t *testing.T, backend string) {
	program := lockedRcloneFixture(t)
	image, job := os.Getenv("PORTICO_SAMBA_TEST_IMAGE"), os.Getenv("PORTICO_SAMBA_TEST_JOB")
	if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(image) || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`).MatchString(job) {
		t.Fatal("owned disposable NAS image and job label are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	f, managed, identities, owner, _, _ := nativeSaveFixture(t)
	f.s.private = t.TempDir()
	if err := f.s.startReadRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.s.stopServeProcesses)
	authorize := func(tx *sql.Tx) error { return currentLocalOwnerTx(ctx, tx, identities, owner) }
	environment := []string{"HOME=" + t.TempDir(), "TMPDIR=" + os.Getenv("TMPDIR")}
	docker := func(ctx context.Context, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Env, cmd.Stderr = environment, io.Discard
		childprocess.Configure(cmd)
		return cmd
	}
	label, err := docker(ctx, "image", "inspect", "--format", `{{ index .Config.Labels "portico.job" }}`, image).Output()
	if err != nil || strings.TrimSpace(string(label)) != job {
		t.Fatal("NAS image belongs to another job", err)
	}
	network := job + "-nas-" + backend
	third := 1 + int(time.Now().UnixNano()%250)
	address := ""
	for attempt := 0; attempt < 8; attempt++ {
		base := fmt.Sprintf("192.168.%d.", 1+(third+attempt)%250)
		if err := docker(ctx, "network", "create", "--internal", "--label", "portico=remote-sources-r40", "--label", "portico.job="+job, "--subnet", base+"0/24", network).Run(); err == nil {
			address = base + "2"
			break
		}
	}
	if address == "" {
		t.Fatal("owned 192.168 NAS network could not be created")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := docker(cleanup, "network", "rm", network).Run(); err != nil {
			t.Error("owned NAS network did not retire", err)
		}
	})
	root := t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("controlled-nas"), 16)
	if err := os.WriteFile(filepath.Join(root, "one.mkv"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	configuration, err := os.ReadFile("../rcloneprogram/testdata/smb/smb.conf")
	if err != nil {
		t.Fatal(err)
	}
	configuration = bytes.ReplaceAll(configuration, []byte("interfaces = 127.0.0.1"), []byte("interfaces = "+address))
	configPath := filepath.Join(t.TempDir(), "smb.conf")
	if err := os.WriteFile(configPath, configuration, 0600); err != nil {
		t.Fatal(err)
	}
	arguments := []string{"run", "--detach", "--label", "portico=remote-sources-r40", "--label", "portico.job=" + job, "--read-only", "--memory=256m", "--cpus=1", "--pids-limit=128", "--network", network, "--ip", address, "--log-driver=none", "--tmpfs", "/run", "--tmpfs", "/var/lib/samba", "--tmpfs", "/var/cache/samba", "--tmpfs", "/var/log/samba", "--mount", "type=bind,source=" + root + ",target=/media,readonly", "--mount", "type=bind,source=" + configPath + ",target=/etc/samba/smb.conf,readonly"}
	known, password := "", ""
	if backend == "sftp" {
		private := t.TempDir()
		public, key, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		encoded, e := x509.MarshalPKCS8PrivateKey(key)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(private, "key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0600); e != nil {
			t.Fatal(e)
		}
		clear(encoded)
		clear(key)
		if e = os.WriteFile(filepath.Join(private, "authorized"), nil, 0600); e != nil {
			t.Fatal(e)
		}
		serverKey, e := ssh.NewPublicKey(public)
		if e != nil {
			t.Fatal(e)
		}
		known = filepath.Join(private, "known-hosts")
		if e = os.WriteFile(known, []byte(knownhosts.Line([]string{address + ":2222"}, serverKey)+"\n"), 0600); e != nil {
			t.Fatal(e)
		}
		obscure := exec.CommandContext(ctx, program, "obscure", "fixture-only")
		obscure.Env, obscure.Stderr = environment, io.Discard
		childprocess.Configure(obscure)
		value, e := obscure.Output()
		if e != nil || len(value) > 512 {
			t.Fatal("fixture password encoding", e)
		}
		password = strings.TrimSpace(string(value))
		clear(value)
		arguments = append(arguments, "--entrypoint=/fixture/rclone", "--mount", "type=bind,source="+program+",target=/fixture/rclone,readonly", "--mount", "type=bind,source="+private+",target=/fixture/private,readonly", image, "serve", "sftp", "/media", "--addr", "0.0.0.0:2222", "--key", "/fixture/private/key", "--authorized-keys", "/fixture/private/authorized", "--user=fixture", "--pass=fixture-only", "--read-only", "--dir-cache-time=0s", "--buffer-size=0", "--log-level=ERROR")
	} else {
		arguments = append(arguments, image)
	}
	raw, err := docker(ctx, arguments...).Output()
	if err != nil {
		t.Fatal("owned NAS start", err)
	}
	container := strings.TrimSpace(string(raw))
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(container) {
		t.Fatal("NAS start returned no exact owned container")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := docker(cleanup, "rm", "--force", container).Run(); err != nil {
			t.Error("owned NAS container did not retire", err)
		}
	})
	config := func(host string) string {
		if backend == "sftp" {
			return "[nas]\ntype=sftp\nhost=" + host + "\nport=2222\nuser=fixture\npass=" + password + "\nknown_hosts_file=" + known + "\ndisable_hashcheck=true\n"
		}
		return "[nas]\ntype=smb\nhost=" + host + "\nuser=fixture-guest\n"
	}
	remote := "nas:media"
	if backend == "sftp" {
		remote = "nas:/"
	}
	command := mounts.Command{OperationID: nativeSaveOperation(101), Action: "create", Name: "NAS", Executable: program, Remote: remote, Config: config(address)}
	// The real backend connection test supplies readiness; no synthetic success
	// or pre-created approval enters the source transaction.
	var receipt mounts.Receipt
	for deadline := time.Now().Add(10 * time.Second); ; {
		receipt, err = managed.Command(ctx, "actual-owner-nas", "", command, authorize)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual owner NAS save", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := f.s.refreshBindings(ctx); err != nil {
		t.Fatal(err)
	}
	view, err := dbwork.BeginSnapshot(ctx, f.s.db)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := SourceReadProfileTx(ctx, view.Tx(), receipt.MountID)
	view.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	binding := SourceReadBinding{Observation: ObservationRef{SourceID: receipt.MountID, Generation: strconv.FormatInt(receipt.Mount.BackendRevision, 10)}, Root: receipt.Mount.MountPath, Profile: profile}
	process, err := f.s.ensureSourceServe(ctx, binding, "")
	if err != nil {
		t.Fatal("saved NAS shared source start", err)
	}
	t.Cleanup(func() {
		process.Stop()
		wait, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := process.Wait(wait); !errors.Is(err, rcloneprogram.ErrServeStopped) {
			t.Error("NAS source did not physically retire", err)
		}
	})
	metadata, err := process.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := metadata.Stat(ctx, "one.mkv", process.Check)
	if err != nil || fresh.Size != int64(len(payload)) {
		t.Fatal("saved native NAS did not connect through shared rclone", fresh.Size, err)
	}
	file, err := f.s.Stat(ctx, filepath.Join(receipt.Mount.MountPath, "one.mkv"))
	if err != nil || file.Size != int64(len(payload)) || file.Directory || file.ChangeToken == "" || file.ObjectIdentity == "" {
		t.Fatal("storage Stat did not use admitted source metadata", file, err)
	}
	directory, err := f.s.InspectRoot(ctx, receipt.Mount.MountPath)
	if err != nil || !directory.Directory || directory.ObjectIdentity == "" {
		t.Fatal("storage root inspection did not use the shared source", directory, err)
	}
	if counts := process.OperationCounts(); counts.Active != 0 || counts.Peak > counts.Capacity || counts.Capacity != profile.Limits.Connections {
		t.Fatal("storage metadata escaped the source budget", counts)
	}
	for i, host := range []string{"127.0.0.1", "169.254.169.254", "169.254.1.2"} {
		bad := mounts.Command{OperationID: nativeSaveOperation(110 + i), Action: "configure", ExpectedRevision: receipt.ControlRevision, Config: config(host)}
		if _, err := managed.Command(ctx, "actual-owner-nas", receipt.MountID, bad, authorize); err == nil {
			t.Fatal("forbidden NAS edit was accepted", host)
		}
	}
}
