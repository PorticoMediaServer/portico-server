package subtitles

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMuxClockBFrameOrderAndWrap(t *testing.T) {
	pre := []int64{0, 12000, 6000, 3000, 9000, 24000, 18000, 15000, 21000, 36000, 30000, 27000}
	post := make([]int64, 12)
	for i, v := range pre {
		post[i] = v + 132000
	}
	v, e := MuxOffset(pre, post)
	if e != nil || v != 132000 {
		t.Fatal(v, e)
	}
	post[2], post[3] = post[3], post[2]
	if _, e = MuxOffset(pre, post); !errors.Is(e, ErrTiming) {
		t.Fatal("reordered correspondence admitted")
	}
	for i, v := range pre {
		post[i] = (v + (1 << 33) - 1000) % (1 << 33)
	}
	if v, e = MuxOffset(pre, post); e != nil || v != (1<<33)-1000 {
		t.Fatal(v, e)
	}
	post[4] = (post[4] + 2) % (1 << 33)
	if _, e = MuxOffset(pre, post); e == nil {
		t.Fatal("drift accepted")
	}
}
func TestClockRationalRoundingAndBounds(t *testing.T) {
	for line, want := range map[string]int64{"1 1/24": 3750, "1 1/1000000": 0, "6 1/1000000": 1, "-6 1/1000000": -1} {
		v, e := parseClockLine(line)
		if e != nil || v != want {
			t.Fatal(line, v, e)
		}
	}
	for _, line := range []string{"NaN 1/90000", "1 1/0", "9223372036854775807 999999999/1", "1 1/90000 extra", "1 -1/90000"} {
		if _, e := parseClockLine(line); e == nil {
			t.Fatal("invalid clock admitted")
		}
	}
}
func TestCollectorSealsPrefixAndContinuesDrain(t *testing.T) {
	r, w := io.Pipe()
	var c ClockCollector
	done := make(chan error, 1)
	go func() { done <- c.Drain(r) }()
	data := strings.Repeat("0 1/90000\n", 12)
	writer := make(chan error, 1)
	go func() {
		_, e := io.WriteString(w, data+strings.Repeat("1 1/90000\n", 10000))
		if e == nil {
			e = w.Close()
		}
		writer <- e
	}()
	select {
	case e := <-writer:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("statistics blocked encoder")
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	p, ready, e := c.Snapshot()
	if e != nil || !ready || len(p) != 12 {
		t.Fatal(p, ready, e)
	}
	p[0] = 99
	q, _, _ := c.Snapshot()
	if q[0] != 0 {
		t.Fatal("snapshot mutates collector")
	}
}
func TestCollectorFailureAndCancellation(t *testing.T) {
	for _, s := range []string{"0 1/90000\n", strings.Repeat("x", 257)} {
		var c ClockCollector
		if e := c.Drain(strings.NewReader(s)); e == nil {
			t.Fatal("short/malformed admitted")
		}
		_, ready, e := c.Snapshot()
		if !ready || e == nil {
			t.Fatal("failure not published")
		}
	}
	r, w := io.Pipe()
	var c ClockCollector
	done := make(chan error, 1)
	go func() { done <- c.Drain(r) }()
	_ = w.CloseWithError(io.ErrClosedPipe)
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancellation accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("drain retained cancelled pipe")
	}
}
