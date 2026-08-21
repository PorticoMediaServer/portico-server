package database

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

// atomicReplaceForTest is an intentionally narrow fault-injection seam for
// Windows sharing-violation and replacement-order tests. Production always
// uses replaceFileAtomicallyOnce below. The hook replaces one platform
// attempt, but it still runs through the same bounded retry driver as the
// native implementation.
type atomicReplaceTestHook struct {
	replaceOnce func(string, string) error
	retryable   func(error) bool
}

var atomicReplaceTestState struct {
	sync.RWMutex
	hook *atomicReplaceTestHook
}

// SetAtomicReplaceForTest scopes the replacement fault seam for a package
// test. Production callers never set it; the mutex keeps -race tests from
// observing a partially updated hook.
func SetAtomicReplaceForTest(hook func(string, string) error) func() {
	return setAtomicReplaceTestHook(&atomicReplaceTestHook{replaceOnce: hook})
}

// SetAtomicReplaceRetryForTest injects both sides of the platform-neutral
// retry policy. It is used to simulate Windows sharing/lock violations on
// non-Windows hosts without weakening or bypassing the production retry loop.
func SetAtomicReplaceRetryForTest(replaceOnce func(string, string) error, retryable func(error) bool) func() {
	return setAtomicReplaceTestHook(&atomicReplaceTestHook{replaceOnce: replaceOnce, retryable: retryable})
}

func setAtomicReplaceTestHook(hook *atomicReplaceTestHook) func() {
	atomicReplaceTestState.Lock()
	previous := atomicReplaceTestState.hook
	atomicReplaceTestState.hook = hook
	atomicReplaceTestState.Unlock()
	return func() {
		atomicReplaceTestState.Lock()
		atomicReplaceTestState.hook = previous
		atomicReplaceTestState.Unlock()
	}
}

func assertSameReplacementDirectory(source, target string) error {
	if filepath.Clean(filepath.Dir(source)) != filepath.Clean(filepath.Dir(target)) {
		return errors.New("atomic replacement requires source and target in the same directory")
	}
	return nil
}

func replaceFileAtomically(source, target string) error {
	return replaceFileAtomicallyContext(context.Background(), source, target)
}

// ReplaceFileAtomically publishes one same-directory file using the reviewed
// platform replacement primitive. Callers must fsync the source first and
// sync the parent directory after success.
func ReplaceFileAtomically(source, target string) error {
	return replaceFileAtomically(source, target)
}

func ReplaceFileAtomicallyContext(ctx context.Context, source, target string) error {
	return replaceFileAtomicallyContext(ctx, source, target)
}

func replaceFileAtomicallyContext(ctx context.Context, source, target string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := assertSameReplacementDirectory(source, target); err != nil {
		return err
	}
	replaceOnce := replaceFileAtomicallyOnce
	retryable := atomicReplaceRetryable
	atomicReplaceTestState.RLock()
	testHook := atomicReplaceTestState.hook
	atomicReplaceTestState.RUnlock()
	if testHook != nil {
		if testHook.replaceOnce != nil {
			replaceOnce = testHook.replaceOnce
		}
		if testHook.retryable != nil {
			retryable = testHook.retryable
		}
	}
	return retryAtomicReplacement(ctx, source, target, replaceOnce, retryable)
}

func retryAtomicReplacement(ctx context.Context, source, target string, replaceOnce func(string, string) error, retryable func(error) bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if replaceOnce == nil || retryable == nil {
		return errors.New("atomic replacement retry functions are required")
	}
	// Windows can transiently reject replacement while a reader still holds
	// the old target. Keep the retry bounded and fail closed; the platform
	// implementation is MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH), while Unix
	// performs one atomic same-directory rename.
	const attempts = 6
	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := replaceOnce(source, target)
		if err == nil {
			return nil
		}
		if !retryable(err) || attempt == attempts-1 {
			return fmt.Errorf("atomic replacement failed after %d attempt(s): %w", attempt+1, err)
		}
		backoff := time.Duration(10*(1<<attempt)) * time.Millisecond
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return errors.New("atomic replacement failed")
}
