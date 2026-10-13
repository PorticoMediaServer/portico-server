package servicelog

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PruneFiles scans only the bounded log set, outside the recording lock.
// Publication takes the lock briefly and is abandoned if rotation or append
// changed the source. A busy log is retried on the next maintenance tick.
func (r *Recorder) PruneFiles() {
	if !r.pruneMu.TryLock() {
		return
	}
	defer r.pruneMu.Unlock()
	r.mu.Lock()
	days := make(map[string]int, len(r.retention))
	for category, age := range r.retention {
		days[category] = age
	}
	r.mu.Unlock()
	if len(days) == 0 || r.options.Directory == "" {
		return
	}
	for index := r.options.FileCount; index >= 0; index-- {
		r.pruneFile(index, days)
	}
}

func (r *Recorder) pruneFile(index int, days map[string]int) {
	path := r.path(index)
	input, err := os.Open(path)
	if err != nil {
		return
	}
	before, err := input.Stat()
	if err != nil || before.Size() > r.options.FileBytes+(64<<10) {
		input.Close()
		return
	}
	output, err := os.CreateTemp(r.options.Directory, ".retention-*")
	if err != nil {
		input.Close()
		return
	}
	defer os.Remove(output.Name())
	scanner := bufio.NewScanner(io.LimitReader(input, before.Size()+1))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	removed := false
	now := r.options.Now()
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) == 4 {
			at, parseErr := time.Parse(time.RFC3339Nano, fields[0])
			if age := days[fields[2]]; age > 0 && parseErr == nil && now.Sub(at) > time.Duration(age)*24*time.Hour {
				removed = true
				continue
			}
		}
		if _, err = output.WriteString(line + "\n"); err != nil {
			break
		}
	}
	input.Close()
	if err == nil {
		err = scanner.Err()
	}
	if err == nil {
		err = output.Sync()
	}
	closeErr := output.Close()
	if err != nil || closeErr != nil || !removed {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current, err := os.Stat(path)
	if err != nil || !os.SameFile(before, current) || current.Size() != before.Size() || !current.ModTime().Equal(before.ModTime()) {
		return
	}
	if index == 0 && r.file != nil {
		r.file.Close()
		r.file = nil
	}
	// Windows cannot replace an open log. Close the recorder before rename,
	// and retain the original if publication fails.
	if err = os.Rename(output.Name(), path); err != nil {
		if index == 0 {
			r.file, _ = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		}
		return
	}
	if index == 0 {
		r.file, _ = os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if r.file != nil {
			r.written = fileSize(r.file)
		}
	}
}
