//go:build linux || darwin

package storage

import (
	"errors"
	"runtime"
	"syscall"

	"portico.local/server/internal/mediaexec"
)

// descriptorInput is how a media tool names inherited descriptor 3.
var descriptorInput = func() string {
	if runtime.GOOS == "darwin" {
		return "/dev/fd/3"
	}
	return "/proc/self/fd/3"
}()

// scanCPUSeconds bounds one scan-time probe or artwork extraction: generous for
// a slow disk and a large file, fatal only to a pathological input.
const scanCPUSeconds = 300

func inventoryCommandHelper(r request) error {
	if r.Inventory == nil || len(r.Argv) < 2 || len(r.Argv) > 1024 {
		return errors.New("invalid inventory command")
	}
	for _, arg := range r.Argv {
		if len(arg) > 16384 {
			return errors.New("invalid scan argument")
		}
	}
	file, err := openInventoryContent(*r.Inventory)
	if err != nil {
		return err
	}
	defer file.Close()
	if int(file.Fd()) != 3 {
		if err = dupTo(int(file.Fd()), 3); err != nil {
			return err
		}
		defer syscall.Close(3)
	}
	// Dup2 clears CLOEXEC except when source and destination are identical.
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, 3, syscall.F_SETFD, 0); errno != 0 {
		return errno
	}
	// This helper is already its own supervised process group; the tool replaces
	// it, so no unmanaged grandchild can outlive the supervisor's Wait.
	return mediaexec.ExecArgv(r.Argv)
}
