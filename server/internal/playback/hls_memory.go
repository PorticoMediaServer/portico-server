package playback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/diskspace"
	"sort"
	"strings"
	"time"
)

const memoryHeadroom int64 = 256 << 20
const memoryUsageInterval = 5 * time.Second

type memoryFacts struct{ available, total, free, filesystemLimit int64 }

// ConfigureMemory is startup-only, before serving or starting maintenance. A
// tmpfs size limit is an additional admission bound: a periodic userspace sweep
// alone cannot bound a converter writing directly to the filesystem.
func (h *HLS) ConfigureMemory(root string) error {
	if !filepath.IsAbs(root) || root == filepath.VolumeName(root)+string(os.PathSeparator) {
		return errors.New("invalid memory output directory")
	}
	root = filepath.Clean(root)
	disk, err := filepath.Abs(h.root)
	if err != nil {
		return err
	}
	if root == disk || strings.HasPrefix(root, disk+string(os.PathSeparator)) || strings.HasPrefix(disk, root+string(os.PathSeparator)) {
		return errors.New("memory and disk output must be separate directories")
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	check := h.memoryCheck
	if check == nil {
		check = checkMemoryOutput
	}
	if err = check(root); err != nil {
		return err
	}
	pinned, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	cursor, err := os.Open(root)
	if err != nil {
		pinned.Close()
		return err
	}
	defer cursor.Close()
	adopted := map[string]bool{}
	installed := false
	defer func() {
		if !installed {
			pinned.Close()
		}
	}()
	for {
		entries, readErr := cursor.ReadDir(128)
		for _, entry := range entries {
			if !entry.IsDir() || !generatedDirectoryID.MatchString(entry.Name()) {
				continue
			}
			id := entry.Name()
			if _, err = os.Lstat(filepath.Join(h.root, id)); err == nil {
				if err = os.RemoveAll(filepath.Join(root, id)); err != nil {
					return err
				}
				continue
			} else if !os.IsNotExist(err) {
				return err
			}
			adopted[id] = true
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	h.memoryRoot, h.memoryOutputRoot = root, pinned
	h.inMemory = adopted
	h.moving = map[string]chan struct{}{}
	installed = true
	return nil
}

func (h *HLS) outputDirLocked(id string) string {
	if h.inMemory[id] {
		return filepath.Join(h.memoryRoot, id)
	}
	return filepath.Join(h.root, id)
}
func (h *HLS) outputDir(id string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.outputDirLocked(id)
}
func (h *HLS) outputRootLocked(id string) *os.Root {
	if h.inMemory[id] {
		return h.memoryOutputRoot
	}
	return h.outputRoot
}
func (h *HLS) hasMemoryOutput() bool { h.mu.Lock(); defer h.mu.Unlock(); return len(h.inMemory) > 0 }

func (h *HLS) readMemory() (memoryFacts, error) {
	if h.memoryFacts != nil {
		return h.memoryFacts(h.memoryRoot)
	}
	return readMemoryFacts(h.memoryRoot)
}
func memoryBudget(setting string, facts memoryFacts, ours int64) (cap, floor int64) {
	floor = max(1<<30, facts.total/10)
	base := facts.available + ours
	switch setting {
	case "half":
		cap = base / 2
	case "2gb":
		cap = 2 << 30
	case "4gb":
		cap = 4 << 30
	case "8gb":
		cap = 8 << 30
	case "16gb":
		cap = 16 << 30
	default:
		cap = base / 4
	}
	cap = min(cap, max(0, ours+facts.available-floor), ours+facts.free)
	return max(0, cap), floor
}
func (h *HLS) memoryBytesLocked(reserve bool) int64 {
	var ours int64
	for id := range h.inMemory {
		if usage := h.usage[id]; usage != nil && !usage.measured.IsZero() {
			ours += usage.bytes
		} else if reserve {
			ours += memoryHeadroom
		}
	}
	return ours
}

// createOutput decides once, under the same lock as producer/cleanup admission.
// Streaming callers classify disc/finite sources before creating their folder.
func (h *HLS) createOutput(id string, streaming bool) (string, error) {
	if !generatedDirectoryID.MatchString(id) {
		return "", os.ErrInvalid
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.moving[id] != nil || h.memoryFull[id] || h.cleaning[id] {
		return "", ErrSegmentPreparing
	}
	if h.inMemory[id] {
		if !streaming {
			return "", errors.New("disk-only output has a memory placement")
		}
		// A process restart may adopt output from a larger/older tmpfs. Do not
		// resume growth unless its kernel ceiling still fits today's allowance.
		facts, err := h.readMemory()
		cap, floor := memoryBudget(h.configuration().MemoryCap, facts, h.memoryBytesLocked(false))
		if err != nil || facts.filesystemLimit <= 0 || facts.filesystemLimit > cap || facts.available < floor {
			h.memoryRefused.Add(1)
			return "", ErrSegmentPreparing
		}
		return h.outputDirLocked(id), nil
	}
	dir := filepath.Join(h.root, id)
	if _, err := os.Lstat(dir); err == nil {
		return dir, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if streaming && h.memoryRoot != "" {
		facts, err := h.readMemory()
		ours := h.memoryBytesLocked(true)
		cap, floor := memoryBudget(h.configuration().MemoryCap, facts, h.memoryBytesLocked(false))
		// Require a kernel-enforced ceiling which fits the current allowance. Even
		// an arbitrarily fast/malformed FFmpeg output cannot consume beyond it.
		if err == nil && facts.total > 0 && facts.filesystemLimit > 0 && facts.filesystemLimit <= cap && ours+memoryHeadroom <= cap && facts.available-memoryHeadroom >= floor && facts.free >= memoryHeadroom {
			dir = filepath.Join(h.memoryRoot, id)
			if err = os.MkdirAll(dir, 0700); err != nil {
				return "", err
			}
			h.inMemory[id] = true
			return dir, nil
		}
		h.memoryRefused.Add(1)
	}
	return dir, os.MkdirAll(dir, 0700)
}

// waitOutputMove fences readers before resolving a path. Starts return the
// existing retryable preparing result while moving; no converter can sneak in.
func (h *HLS) waitOutputMove(ctx context.Context, id string) error {
	h.mu.Lock()
	done := h.moving[id]
	h.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sweepMemory measures idle as well as live folders, then spills at most one.
func (h *HLS) sweepMemory(ctx context.Context) {
	if h.memoryRoot == "" {
		return
	}
	h.memorySweepMu.Lock()
	defer h.memorySweepMu.Unlock()
	h.mu.Lock()
	ids := make([]string, 0, len(h.inMemory))
	for id := range h.inMemory {
		ids = append(ids, id)
	}
	h.mu.Unlock()
	h.enforceSessionCeilings(ids)
	facts, err := h.readMemory()
	// An explicit ENOSPC is independently known pressure, even if host memory
	// facts became unreadable. Unknown facts alone never trigger a spill.
	h.mu.Lock()
	forced := false
	for id := range h.memoryFull {
		if h.inMemory[id] {
			forced = true
			break
		}
	}
	h.mu.Unlock()
	if err != nil && !forced {
		return
	}
	type candidate struct {
		id     string
		live   bool
		bytes  int64
		served time.Time
	}
	h.mu.Lock()
	ours := h.memoryBytesLocked(false)
	cap, floor := memoryBudget(h.configuration().MemoryCap, facts, ours)
	if !forced && ours <= cap && facts.available >= floor && facts.free >= memoryHeadroom && facts.filesystemLimit <= cap {
		h.mu.Unlock()
		return
	}
	candidates := []candidate{}
	for id := range h.inMemory {
		if h.moving[id] != nil || h.cleaning[id] || forced && !h.memoryFull[id] {
			continue
		}
		c := candidate{id: id, live: h.sessionActiveLocked(id)}
		if u := h.usage[id]; u != nil {
			c.bytes, c.served = u.bytes, u.served
		}
		candidates = append(candidates, c)
	}
	h.mu.Unlock()
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.live != b.live {
			return !a.live
		}
		if a.live {
			return a.bytes > b.bytes
		}
		return a.served.Before(b.served)
	})
	if len(candidates) > 0 {
		h.spillMemory(ctx, candidates[0].id)
	}
}

func (h *HLS) spillMemory(ctx context.Context, id string) {
	h.mu.Lock()
	if !h.inMemory[id] || h.moving[id] != nil || h.cleaning[id] {
		h.mu.Unlock()
		return
	}
	done := make(chan struct{})
	if h.moving == nil {
		h.moving = map[string]chan struct{}{}
	}
	h.moving[id] = done
	source := h.outputDirLocked(id)
	h.mu.Unlock()
	finishMove := func() {
		h.mu.Lock()
		if h.moving[id] == done {
			delete(h.moving, id)
			close(done)
		}
		h.mu.Unlock()
	}
	defer finishMove()
	h.cancelSession(id)
	wait, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		h.mu.Lock()
		active := h.sessionActiveLocked(id) || h.outputBusy[id] > 0
		h.mu.Unlock()
		if !active {
			break
		}
		select {
		case <-wait.Done():
			return
		case <-tick.C:
		}
	}
	if ctx.Err() != nil {
		return
	}
	temp, err := os.MkdirTemp(h.root, ".spill-"+id+"-")
	if err == nil {
		defer os.RemoveAll(temp)
		err = copySpill(ctx, source, temp)
		if err == nil {
			// A producer can fill tmpfs during initial manifest publication,
			// before it installs its generation stamp. Such an incomplete tree
			// cannot resume: FileContext only regenerates wholly absent output.
			info, stampErr := os.Stat(filepath.Join(temp, "generation"))
			if stampErr != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 32 {
				err = errors.New("incomplete generation in memory output")
			}
		}
	}
	if err != nil && temp != "" {
		_ = os.RemoveAll(temp)
	}
	h.mu.Lock()
	if err == nil {
		err = os.Rename(temp, filepath.Join(h.root, id))
	}
	// Retire memory while starts and path resolution remain fenced. If copying
	// failed, dropping regenerable output is safer than retaining RAM pressure.
	retired := filepath.Join(h.memoryRoot, fmt.Sprintf(".evicted-%s-%d", id, time.Now().UnixNano()))
	retireErr := os.Rename(source, retired)
	if retireErr != nil && !os.IsNotExist(retireErr) {
		h.mu.Unlock()
		log.Printf("Memory conversion spill %s: cannot retire output: %v", id, retireErr)
		return
	}
	delete(h.inMemory, id)
	delete(h.memoryFull, id)
	delete(h.usage, id)
	h.mu.Unlock()
	finishMove()
	if retireErr == nil {
		_ = os.RemoveAll(retired)
	}
	h.memorySpills.Add(1)
	if err == nil {
		log.Printf("Conversion %s spilled from memory to disk", id)
	} else {
		log.Printf("Conversion %s moved to disk after dropping memory output: %v", id, err)
	}
	// The directory column is diagnostic only; serving never follows it.
	if h.db != nil {
		write, cancel := persist(ctx)
		defer cancel()
		_, _ = dbwork.ExecWrite(write, h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_artifacts SET directory=? WHERE session_id=?`, filepath.Join(h.root, id), id)
	}
}

// copySpill never follows links or copies device/FIFO/socket entries. Traversal
// and copy buffers are bounded, and cancellation is checked between chunks.
func copySpill(ctx context.Context, source, dest string) error {
	root, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer root.Close()
	var bytes int64
	buf := make([]byte, 64<<10)
	return walkOutput(ctx, root, func(rel string, entry os.DirEntry) error {
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink in conversion output")
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(dest, rel), 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("nonregular conversion output")
		}
		in, err := root.OpenFile(rel, hlsReadFlags, 0)
		if err != nil {
			return err
		}

		opened, statErr := in.Stat()
		if statErr != nil || !opened.Mode().IsRegular() {
			in.Close()
			return errors.New("nonregular spill source")
		}
		out, err := os.OpenFile(filepath.Join(dest, rel), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			in.Close()
			return err
		}
		for {
			if err = ctx.Err(); err != nil {
				break
			}
			var n int
			n, err = in.Read(buf)
			if n > 0 {
				bytes += int64(n)
				if bytes > hlsGlobalByteBudget {
					err = errors.New("spill copy exceeds byte bound")
					break
				}
				if _, e := out.Write(buf[:n]); e != nil {
					err = e
					break
				}
			}
			if err == io.EOF {
				err = nil
				break
			}
			if err != nil {
				break
			}
		}
		closeErr := out.Close()
		in.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}

// MemoryOutputDirectory reports the startup-only configured memory root.
func (s *Service) MemoryOutputDirectory() string {
	if s == nil || s.hls == nil {
		return ""
	}
	return s.hls.memoryRoot
}

// Disc and finite producers always create disk output, including private discs
// that do not create a chapter sidecar while their source is being opened.
func (h *HLS) createStreamingOutput(ctx context.Context, id string, plan *DeliveryPlan) (string, error) {
	streaming := plan == nil || plan.Profile != finiteDeliveryProfile
	if h.memoryRoot != "" && streaming {
		var disc bool
		err := dbwork.QueryRow(ctx, h.db, `SELECT EXISTS(SELECT 1 FROM playback_disc_pins WHERE session_id=?) OR EXISTS(SELECT 1 FROM playback_disc_snapshots WHERE session_id=?)`, id, id).Scan(&disc)
		streaming = err == nil && !disc
	}
	return h.createOutput(id, streaming)
}

// Retention deletes played output outside h.mu. A move waits for these bounded
// mutations, just as it waits for producers, before copying a stable tree.
func (h *HLS) retainOutput(id string) (string, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.moving[id] != nil || h.cleaning[id] {
		return "", nil
	}
	if h.outputBusy == nil {
		h.outputBusy = map[string]int{}
	}
	h.outputBusy[id]++
	return h.outputDirLocked(id), func() {
		h.mu.Lock()
		h.outputBusy[id]--
		if h.outputBusy[id] == 0 {
			delete(h.outputBusy, id)
		}
		h.mu.Unlock()
	}
}

// The kernel may refuse a tmpfs write before a sweep samples its size. Preserve
// the presentation, fence new producers and let maintenance move it to disk.
func (h *HLS) retryMemoryOutput(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.inMemory[id] {
		return false
	}
	if h.memoryFull == nil {
		h.memoryFull = map[string]bool{}
	}
	h.memoryFull[id] = true
	return true
}

// Early manifest/setup failures do not carry a converter failure code. If the
// memory volume is already full, defer that failure to disk regeneration too.
func (h *HLS) memoryOutputPressure(id string) bool {
	h.mu.Lock()
	memory := h.inMemory[id]
	h.mu.Unlock()
	if !memory {
		return false
	}
	free, err := diskspace.Free(h.memoryRoot)
	if h.memoryFacts != nil {
		facts, e := h.readMemory()
		free, err = facts.free, e
	}
	return err == nil && free < memoryHeadroom && h.retryMemoryOutput(id)
}
