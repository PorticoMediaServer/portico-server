package subtitles

import (
	"bufio"
	"io"
	"math/big"
	"strconv"
	"strings"
	"sync"
)

const ClockPacketCount = 12

// ClockCollector receives only the selected video's pre-mux statistics. Once its
// bounded prefix is complete it continues draining, so a long encoder neither
// blocks on an unread pipe nor receives SIGPIPE. The process owner closes the
// pipe on cancellation; this object does not own a process or background worker.
type ClockCollector struct {
	mu     sync.Mutex
	prefix []int64
	err    error
	done   bool
}

func (c *ClockCollector) Drain(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 256), 256)
	var samples []int64
	for len(samples) < ClockPacketCount && scanner.Scan() {
		v, e := parseClockLine(scanner.Text())
		if e != nil {
			c.store(nil, e)
			_, _ = io.Copy(io.Discard, r)
			return e
		}
		samples = append(samples, v)
	}
	if e := scanner.Err(); e != nil {
		c.store(nil, e)
		_, _ = io.Copy(io.Discard, r)
		return e
	}
	if len(samples) != ClockPacketCount {
		c.store(nil, ErrTiming)
		return ErrTiming
	}
	c.store(samples, nil)
	// Scanner may have read ahead; discard its buffered bytes too. No trailing
	// statistics are retained or used to change the already captured mapping.
	for scanner.Scan() {
	} // bounded line buffer remains in force
	if e := scanner.Err(); e != nil {
		_, _ = io.Copy(io.Discard, r)
		return e
	}
	return nil
}
func (c *ClockCollector) store(p []int64, e error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prefix = append([]int64(nil), p...)
	c.err = e
	c.done = true
}
func (c *ClockCollector) Snapshot() ([]int64, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.prefix...), c.done, c.err
}
func parseClockLine(line string) (int64, error) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return 0, ErrTiming
	}
	pts, e := strconv.ParseInt(fields[0], 10, 64)
	if e != nil {
		return 0, ErrTiming
	}
	tb := strings.Split(fields[1], "/")
	if len(tb) != 2 {
		return 0, ErrTiming
	}
	n, e := strconv.ParseInt(tb[0], 10, 64)
	if e != nil || n <= 0 || n > 1e9 {
		return 0, ErrTiming
	}
	d, e := strconv.ParseInt(tb[1], 10, 64)
	if e != nil || d <= 0 || d > 1e9 {
		return 0, ErrTiming
	}
	rat := new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(pts), big.NewInt(n*90000)), big.NewInt(d))
	// Nearest 90 kHz tick; both operands are exact integers, never float PTS.
	sign := rat.Sign()
	num := new(big.Int).Abs(rat.Num())
	num.Mul(num, big.NewInt(2))
	num.Add(num, rat.Denom())
	den := new(big.Int).Mul(rat.Denom(), big.NewInt(2))
	rounded := new(big.Int).Quo(num, den)
	if sign < 0 {
		rounded.Neg(rounded)
	}
	if !rounded.IsInt64() {
		return 0, ErrTiming
	}
	v := rounded.Int64()
	if v < -MaxDurationMS*90 || v > MaxDurationMS*90 {
		return 0, ErrTiming
	}
	return v, nil
}

// MuxOffset correlates exactly the same first video packet sequence, including
// B-frame PTS order. The caller must select the pinned video stream and verify
// packet provenance; sorted timestamps would destroy this correlation.
func MuxOffset(pre, post []int64) (int64, error) {
	if len(pre) != ClockPacketCount || len(post) != ClockPacketCount {
		return 0, ErrTiming
	}
	const wrap int64 = 1 << 33
	var shift int64
	for i := range pre {
		if pre[i] < -MaxDurationMS*90 || pre[i] > MaxDurationMS*90 || post[i] < 0 || post[i] >= wrap {
			return 0, ErrTiming
		}
		delta := ((post[i]-pre[i])%wrap + wrap) % wrap
		if i == 0 {
			shift = delta
		} else {
			diff := delta - shift
			if diff > wrap/2 {
				diff -= wrap
			}
			if diff < -wrap/2 {
				diff += wrap
			}
			if diff < -1 || diff > 1 {
				return 0, ErrTiming
			}
		}
	}
	return shift, nil
}
