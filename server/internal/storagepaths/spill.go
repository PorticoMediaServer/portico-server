package storagepaths

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"portico.local/server/internal/diskspace"
	"sync"
)

// TranscodePool is the RAM backing primitive for closed generated output. It
// accepts only a measured RAM filesystem, limits aggregate bytes to a quarter
// of available memory and half the available tmpfs, and spills to disk. The
// future settings specification chooses whether to use this primitive; no key
// or settings field is defined here.
type TranscodePool struct {
	mu           sync.Mutex
	memory, disk string
	cap, used    int64
}
type SpillFile struct {
	pool   *TranscodePool
	file   *os.File
	memory bool
	bytes  int64
	closed bool
}

func NewTranscodePool(memory, disk string, availableRAM int64, mounts []Mount) (*TranscodePool, error) {
	if !filepath.IsAbs(disk) || disk == string(os.PathSeparator) {
		return nil, errors.New("disk spill needs a dedicated absolute directory")
	}
	if err := canonicalParents(disk); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(disk, 0700); err != nil {
		return nil, err
	}
	p := &TranscodePool{disk: disk}
	if memory == "" {
		return p, nil
	}
	if Filesystem(memory, mounts) != "tmpfs" {
		return nil, errors.New("memory transcodes require a verified tmpfs or /dev/shm filesystem")
	}
	if err := canonicalParents(memory); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(memory, 0700); err != nil {
		return nil, err
	}
	free, err := diskspace.Free(memory)
	if err != nil {
		return nil, err
	}
	p.memory, p.cap = memory, min(max(availableRAM, 0)/4, free/2)
	return p, nil
}
func (p *TranscodePool) Create() (*SpillFile, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	root, inMemory := p.disk, false
	if p.memory != "" && p.used < p.cap {
		root, inMemory = p.memory, true
	}
	f, err := os.CreateTemp(root, "transcode-")
	if err != nil && inMemory {
		f, err = os.CreateTemp(p.disk, "transcode-")
		inMemory = false
	}
	if err != nil {
		return nil, err
	}
	return &SpillFile{pool: p, file: f, memory: inMemory}, nil
}
func (f *SpillFile) spill() error {
	target, err := os.CreateTemp(f.pool.disk, "transcode-")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			target.Close()
			os.Remove(target.Name())
		}
	}()
	if _, err = f.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err = io.Copy(target, f.file); err != nil {
		f.file.Seek(0, io.SeekEnd)
		return err
	}
	old := f.file.Name()
	if err = f.file.Close(); err != nil {
		return err
	}
	f.file = target
	f.memory = false
	f.pool.used -= f.bytes
	os.Remove(old)
	keep = true
	return nil
}
func (f *SpillFile) Write(data []byte) (int, error) {
	f.pool.mu.Lock()
	defer f.pool.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	if f.memory && int64(len(data)) > f.pool.cap-f.pool.used {
		if err := f.spill(); err != nil {
			return 0, err
		}
	}
	n, err := f.file.Write(data)
	f.bytes += int64(n)
	if f.memory {
		f.pool.used += int64(n)
	}
	if err != nil && f.memory {
		if spillErr := f.spill(); spillErr != nil {
			return n, spillErr
		}
		more, e := f.file.Write(data[n:])
		f.bytes += int64(more)
		return n + more, e
	}
	return n, err
}

// Path is read after Close, since a write may relocate output to disk.
func (f *SpillFile) Path() string { return f.file.Name() }
func (f *SpillFile) Close() error {
	f.pool.mu.Lock()
	defer f.pool.mu.Unlock()
	if f.closed {
		return os.ErrClosed
	}
	f.closed = true
	return f.file.Close()
}
func (f *SpillFile) Remove() error {
	f.pool.mu.Lock()
	defer f.pool.mu.Unlock()
	if !f.closed {
		f.file.Close()
		f.closed = true
	}
	err := os.Remove(f.file.Name())
	if f.memory {
		f.pool.used -= f.bytes
		f.memory = false
	}
	return err
}
