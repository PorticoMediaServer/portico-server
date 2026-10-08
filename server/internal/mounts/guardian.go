package mounts

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/supervise"
	"strconv"
	"time"
)

// Guardian owns the foreground rclone child. Parent death closes stdin, so the
// guardian stops its child without ever trusting a persisted PID after restart.
func Guardian(input io.Reader) error {
	decoder := json.NewDecoder(io.LimitReader(input, 128<<10))
	var request launch
	if err := decoder.Decode(&request); err != nil {
		return err
	}
	if actual, err := executableDigest(request.Executable); err != nil || actual != request.Digest {
		return errors.New("executable changed")
	}
	unlock, err := lockMount(filepath.Join(request.PrivateHome, filepath.Base(request.MountPath)+".mount.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	args := []string{"mount", request.Remote, request.MountPath,
		"--config", request.ConfigPath, "--ask-password=false", "--read-only",
		"--buffer-size=8M", "--dir-cache-time=1m", "--contimeout=10s", "--timeout=30s",
		"--low-level-retries=2", "--retries=1", "--log-level=ERROR", "--use-json-log"}
	if request.CacheBytes > 0 {
		if request.CacheBytes > 64<<30 || request.CacheFloor < 1<<30 || filepath.Dir(request.CachePath) != request.PrivateHome || !cacheSpaceAvailable(request.PrivateHome, request.CacheFloor) {
			return errors.New("cache allocation unavailable")
		}
		if err := os.MkdirAll(request.CachePath, 0700); err != nil {
			return err
		}
		canonical, e := filepath.EvalSymlinks(request.CachePath)
		if e != nil || canonical != request.CachePath {
			return errors.New("invalid private cache path")
		}
		if e = os.Chmod(request.CachePath, 0700); e != nil {
			return e
		}
		args = append(args, "--vfs-cache-mode=full", "--cache-dir", request.CachePath, "--vfs-cache-max-size", strconv.FormatInt(request.CacheBytes, 10), "--vfs-cache-min-free-space", strconv.FormatInt(request.CacheFloor, 10), "--vfs-cache-max-age=1h", "--vfs-cache-poll-interval=15s")
	} else {
		args = append(args, "--vfs-cache-mode=off")
	}
	cmd := exec.Command(request.Executable, args...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + request.PrivateHome, "TMPDIR=" + request.PrivateHome, "RCLONE_CONFIG_PASS=" + request.Password}
	// Provider logs may contain arbitrary URLs, keys and file names. No raw child
	// output leaves this boundary; the service records bounded typed events only.
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	ownProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	ended := make(chan error, 1)
	supervise.Go("mounts.guardian.wait", func() { ended <- cmd.Wait() })
	disconnected := make(chan struct{})
	supervise.Go("mounts.guardian.drain", func() { _, _ = io.Copy(io.Discard, input); close(disconnected) })
	select {
	case err := <-ended:
		return err
	case <-disconnected:
	}
	signalProcess(cmd, false)
	select {
	case err := <-ended:
		return err
	case <-time.After(5 * time.Second):
		signalProcess(cmd, true)
	}
	return <-ended // Quarantine persists if the kernel cannot reap the child.
}

type launch struct {
	Executable, Digest, Remote, MountPath, ConfigPath, PrivateHome, Password string
	CachePath                                                                string
	CacheBytes, CacheFloor                                                   int64
}
