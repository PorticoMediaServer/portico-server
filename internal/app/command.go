package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
)

var errManagedCommandOutputLimit = errors.New("managed command output exceeded its limit")

type limitedCommandBuffer struct {
	bytes.Buffer
	limit        int
	overflow     bool
	onOverflow   func()
	overflowOnce sync.Once
}

func (buffer *limitedCommandBuffer) signalOverflow() {
	buffer.overflow = true
	buffer.overflowOnce.Do(func() {
		if buffer.onOverflow != nil {
			buffer.onOverflow()
		}
	})
}

func (buffer *limitedCommandBuffer) Write(value []byte) (int, error) {
	if buffer.limit <= 0 {
		buffer.signalOverflow()
		return 0, errManagedCommandOutputLimit
	}
	remaining := buffer.limit - buffer.Len()
	if remaining <= 0 {
		buffer.signalOverflow()
		return 0, errManagedCommandOutputLimit
	}
	if len(value) > remaining {
		written, _ := buffer.Buffer.Write(value[:remaining])
		buffer.signalOverflow()
		return written, errManagedCommandOutputLimit
	}
	return buffer.Buffer.Write(value)
}

func (buffer *limitedCommandBuffer) ReadFrom(reader io.Reader) (int64, error) {
	chunk := make([]byte, 32<<10)
	var total int64
	for {
		read, readErr := reader.Read(chunk)
		if read > 0 {
			written, writeErr := buffer.Write(chunk[:read])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return total, nil
			}
			return total, readErr
		}
	}
}

func managedCommandRun(ctx context.Context, cmd *exec.Cmd) error {
	if ctx == nil {
		ctx = context.Background()
	}
	prepareManagedBackgroundCommand(cmd)
	if err := cmd.Start(); err != nil {
		releaseManagedBackgroundCommand(cmd)
		return err
	}
	defer releaseManagedBackgroundCommand(cmd)
	lowerManagedBackgroundPriority(cmd)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		killManagedBackgroundCommand(cmd)
		<-done
		return ctx.Err()
	}
}

func managedCommandOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := managedCommandRun(ctx, cmd)
	return stdout.Bytes(), err
}

func managedCommandOutputLimit(ctx context.Context, cmd *exec.Cmd, maxBytes int) ([]byte, error) {
	var stdout limitedCommandBuffer
	stdout.limit = maxBytes
	stdout.onOverflow = func() {
		killManagedBackgroundCommand(cmd)
	}
	cmd.Stdout = &stdout
	err := managedCommandRun(ctx, cmd)
	if errors.Is(err, errManagedCommandOutputLimit) || stdout.overflow {
		return nil, errManagedCommandOutputLimit
	}
	return stdout.Bytes(), err
}

func managedCommandCombinedOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := managedCommandRun(ctx, cmd)
	return output.Bytes(), err
}
