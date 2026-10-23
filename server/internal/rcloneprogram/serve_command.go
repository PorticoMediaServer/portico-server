package rcloneprogram

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"portico.local/server/internal/childprocess"
)

var ErrServeConfiguration = errors.New("remote serve configuration is invalid")

// ServeCommandSpec is private, already validated source configuration. Enforce
// must install the source's enforcing outbound boundary, not just approve a URL
// or set proxy environment variables (SFTP/SMB connect directly).
// There is deliberately no default enforcement or inherited environment.
type ServeCommandSpec struct {
	PrivateDirectory  string
	Configuration     []byte
	Remote            string
	Port              int
	Username          string
	Password          string
	Connections       int
	RequestsPerSecond int
	BytesPerSecond    int64
	CacheDirectory    string
	CacheBytes        int64
	CacheFloor        int64
	Enforce           func(*exec.Cmd) error
}

// PreparedServe is not a running process. Its owning supervisor retains process
// admission until Wait, uses childprocess.Kill on shutdown, and calls Loaded only
// after an authenticated readiness request proves configuration has been read.
// Loaded must succeed before publishing the endpoint. Caches are never placed
// in the transient configuration directory.
type PreparedServe struct {
	Command *exec.Cmd
	private string
}

var serveRemote = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*:[^\x00\r\n\\]*$`)

func PrepareServe(ctx context.Context, program Program, spec ServeCommandSpec) (*PreparedServe, error) {
	if spec.Enforce == nil || !filepath.IsAbs(spec.PrivateDirectory) || len(spec.Configuration) == 0 || len(spec.Configuration) > 64<<10 || !serveRemote.MatchString(spec.Remote) || spec.Port < 1 || spec.Port > 65535 || spec.Username == "" || spec.Password == "" || strings.ContainsAny(spec.Username, ":\r\n\x00") || strings.ContainsAny(spec.Password, "\r\n\x00") || spec.Connections < 1 || spec.Connections > 256 || spec.RequestsPerSecond < 1 || spec.RequestsPerSecond > 1000000 || spec.BytesPerSecond < 0 {
		return nil, ErrServeConfiguration
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if spec.CacheBytes < 0 || spec.CacheFloor < 0 || spec.CacheBytes > 0 && (!filepath.IsAbs(spec.CacheDirectory) || spec.CacheFloor == 0) {
		return nil, ErrServeConfiguration
	}
	if !program.Included {
		if _, err := program.Check(ctx); err != nil {
			return nil, err
		}
	}
	canonical, err := filepath.EvalSymlinks(spec.PrivateDirectory)
	if err != nil || canonical != spec.PrivateDirectory {
		return nil, ErrServeConfiguration
	}
	private, err := os.MkdirTemp(spec.PrivateDirectory, "serve-config-")
	if err != nil {
		return nil, err
	}
	p := &PreparedServe{private: private}
	good := false
	defer func() {
		if !good {
			_ = p.Loaded()
		}
	}()
	file := filepath.Join(private, "rclone.conf")
	if err = os.WriteFile(file, spec.Configuration, 0600); err != nil {
		return nil, err
	}
	args := []string{"serve", "webdav", spec.Remote, "--read-only", "--addr", net.JoinHostPort("127.0.0.1", strconv.Itoa(spec.Port)), "--config", file, "--ask-password=false", "--dir-cache-time=0s", "--vfs-read-ahead=0", "--buffer-size=0", "--retries=1", "--low-level-retries=1", "--contimeout=10s", "--timeout=30s", "--log-level=ERROR", "--max-connections", strconv.Itoa(spec.Connections), "--tpslimit", strconv.Itoa(spec.RequestsPerSecond), "--tpslimit-burst=1"}
	if spec.CacheBytes > 0 {
		// The source owner supplies its incarnation-scoped cache directory and
		// invalidates it on changed observations; a cache is never byte identity.
		args = append(args, "--vfs-cache-mode=full", "--cache-dir", spec.CacheDirectory, "--vfs-cache-max-size", strconv.FormatInt(spec.CacheBytes, 10)+"B", "--vfs-cache-min-free-space", strconv.FormatInt(spec.CacheFloor, 10)+"B")
	} else {
		args = append(args, "--vfs-cache-mode=off")
	}
	if spec.BytesPerSecond > 0 {
		args = append(args, "--bwlimit", strconv.FormatInt(spec.BytesPerSecond, 10)+"B")
	}
	p.Command = exec.CommandContext(ctx, program.Path, args...)
	p.Command.Env = []string{"HOME=" + private, "TMPDIR=" + private, "RCLONE_USER=" + spec.Username, "RCLONE_PASS=" + spec.Password, "RCLONE_ASK_PASSWORD=false"}
	p.Command.Stdout, p.Command.Stderr = io.Discard, io.Discard
	childprocess.Configure(p.Command)
	if err = spec.Enforce(p.Command); err != nil {
		return nil, err
	}
	good = true
	return p, nil
}

// Loaded removes only this command's fresh transient configuration, both on
// failed startup and after load. The retained source/auth configuration is owned
// elsewhere and is never removed or rewritten by this helper.
func (p *PreparedServe) Loaded() error {
	if p.private == "" {
		return nil
	}
	if err := os.RemoveAll(p.private); err != nil {
		return err
	}
	p.private = ""
	return nil
}
