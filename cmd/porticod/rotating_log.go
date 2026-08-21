package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// rotatingLogWriter keeps the process log useful without allowing a single
// append-only file to consume the server's storage.
type rotatingLogWriter struct {
	mu      sync.Mutex
	path    string
	maximum int64
	backups int
	file    *os.File
	size    int64
	protect func(string, int) error
}

func newRotatingLogWriter(path string, maximum int64, backups int, protectors ...func(string, int) error) (*rotatingLogWriter, error) {
	if maximum <= 0 {
		return nil, errors.New("rotating log maximum must be positive")
	}
	w := &rotatingLogWriter{path: path, maximum: maximum, backups: max(1, backups)}
	if len(protectors) > 0 {
		w.protect = protectors[0]
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingLogWriter) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file = file
	w.size = info.Size()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		w.file = nil
		return err
	}
	if w.protect != nil {
		if err := w.protect(w.path, w.backups); err != nil {
			_ = file.Close()
			w.file = nil
			return err
		}
	}
	if err := syncLogDirectory(filepath.Dir(w.path)); err != nil {
		_ = file.Close()
		w.file = nil
		return err
	}
	return nil
}

func (w *rotatingLogWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	written := 0
	for len(data) > 0 {
		if w.size >= w.maximum {
			if err := w.rotate(); err != nil {
				return written, err
			}
		}
		remaining := w.maximum - w.size
		chunk := data
		if int64(len(chunk)) > remaining {
			chunk = chunk[:remaining]
		}
		n, err := w.file.Write(chunk)
		w.size += int64(n)
		written += n
		data = data[n:]
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func (w *rotatingLogWriter) rotate() error {
	if err := w.file.Sync(); err != nil {
		return err
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	if err := os.Remove(w.path + "." + strconv.Itoa(w.backups)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for index := w.backups - 1; index >= 1; index-- {
		if err := os.Rename(w.path+"."+strconv.Itoa(index), w.path+"."+strconv.Itoa(index+1)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Rename(w.path, w.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := syncLogDirectory(filepath.Dir(w.path)); err != nil {
		return err
	}
	return w.open()
}

func (w *rotatingLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Sync()
	if closeErr := w.file.Close(); err == nil {
		err = closeErr
	}
	w.file = nil
	return err
}

func syncLogDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
