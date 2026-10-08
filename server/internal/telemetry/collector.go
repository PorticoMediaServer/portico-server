package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"portico.local/server/internal/dbwork"
	"sort"
	"sync"
	"time"
)

// Sampler produces one observation. The collector owns every call to it, so an
// implementation may be as slow as its own budget allows without a request ever
// waiting on it.
type Sampler interface {
	Sample(ctx context.Context, at int64) Sample
}

// Window bounds, in samples. Detail is two-second samples for ten minutes;
// rollups are one-minute averages for a day. Both are hard caps: the collector
// discards the oldest rather than growing.
const (
	DetailInterval = 2 * time.Second
	DetailSamples  = 300
	RollupInterval = time.Minute
	RollupSamples  = 24 * 60
	persistEvery   = time.Hour
	sampleBudget   = 1500 * time.Millisecond
)

// Collector samples off the request path and answers windowed reads from
// memory. A nil database is supported: history then lives only for this process.
type Collector struct {
	sampler  Sampler
	db       *sql.DB
	now      func() time.Time
	interval time.Duration

	mu          sync.Mutex
	detail      []Sample
	rollups     []Sample
	minute      []Sample
	minuteStamp int64
	lastPersist int64
	persisted   int64
}

type Options struct {
	Sampler  Sampler
	DB       *sql.DB
	Now      func() time.Time
	Interval time.Duration
}

func New(o Options) *Collector {
	c := &Collector{sampler: o.Sampler, db: o.DB, now: o.Now, interval: o.Interval}
	if c.now == nil {
		c.now = time.Now
	}
	if c.interval <= 0 {
		c.interval = DetailInterval
	}
	if c.sampler == nil {
		c.sampler = HostSampler()
	}
	return c
}

// Run samples until the context ends. It is the only caller of the sampler.
func (c *Collector) Run(ctx context.Context) {
	c.Restore(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	c.Collect(ctx)
	for {
		select {
		case <-ctx.Done():
			c.persist(context.WithoutCancel(ctx), true)
			return
		case <-ticker.C:
			c.Collect(ctx)
		}
	}
}

// Collect takes exactly one sample under a bounded budget and folds it into the
// rolling windows. Exported so a test drives the collector without a clock.
func (c *Collector) Collect(ctx context.Context) Sample {
	budget, cancel := context.WithTimeout(ctx, sampleBudget)
	defer cancel()
	at := c.now().UnixMilli()
	sample := c.sampler.Sample(budget, at)
	if sample.Metrics == nil {
		sample = newSample(at)
	}
	sample.At = at
	c.mu.Lock()
	c.detail = appendBounded(c.detail, sample, DetailSamples)
	stamp := at - at%RollupInterval.Milliseconds()
	if c.minuteStamp != 0 && stamp != c.minuteStamp {
		c.rollups = appendBounded(c.rollups, average(c.minute, c.minuteStamp), RollupSamples)
		c.minute = c.minute[:0]
	}
	c.minuteStamp = stamp
	c.minute = append(c.minute, sample)
	due := c.lastPersist == 0 || at-c.lastPersist >= persistEvery.Milliseconds()
	c.mu.Unlock()
	if due {
		c.persist(ctx, false)
	}
	return sample
}

func appendBounded(list []Sample, v Sample, limit int) []Sample {
	list = append(list, v)
	if len(list) > limit {
		list = append(list[:0], list[len(list)-limit:]...)
	}
	return list
}

// average folds a minute of detail samples into one rollup. A metric that no
// sample in the minute could measure stays unavailable rather than becoming a
// zero, and a metric measured only under restriction stays limited.
func average(samples []Sample, at int64) Sample {
	out := newSample(at)
	if len(samples) == 0 {
		return out
	}
	out.GPUDevice, out.GPUProvider = samples[len(samples)-1].GPUDevice, samples[len(samples)-1].GPUProvider
	out.MemoryUsedBytes, out.MemoryTotalBytes = samples[len(samples)-1].MemoryUsedBytes, samples[len(samples)-1].MemoryTotalBytes
	for _, name := range MetricNames {
		sum, count, status, detail := 0.0, 0, "", ""
		for _, s := range samples {
			m := s.Metrics[name]
			if !m.Known() {
				if status == "" {
					detail = m.Detail
				}
				continue
			}
			sum += m.Value
			count++
			if status != StatusAvailable {
				status, detail = m.Status, m.Detail
			}
		}
		if count == 0 {
			out.Metrics[name] = Unavailable(detail)
			continue
		}
		out.Metrics[name] = Metric{Status: status, Value: sum / float64(count), Detail: detail}
	}
	return out
}

// Latest is the most recent observation, or an all-unavailable sample before
// the first one lands.
func (c *Collector) Latest() Sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.detail) == 0 {
		return newSample(c.now().UnixMilli())
	}
	return c.detail[len(c.detail)-1]
}

// Windows are the only accepted values of the window query parameter.
var Windows = []string{"10m", "1h", "24h"}

func windowSpan(window string) (time.Duration, bool) {
	switch window {
	case "10m":
		return 10 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	}
	return 0, false
}

// Read answers one window. The ten-minute window is served from detail samples;
// longer windows from one-minute rollups, including the minute still filling so
// a fresh restart is not an empty chart.
func (c *Collector) Read(window string) (Reading, bool) {
	span, ok := windowSpan(window)
	if !ok {
		return Reading{}, false
	}
	now := c.now().UnixMilli()
	out := Reading{Window: window, Series: map[string][]Point{}, Status: map[string]Metric{}, ObservedAt: now}
	c.mu.Lock()
	source := c.detail
	if window != "10m" {
		source = append(append([]Sample{}, c.rollups...), average(c.minute, c.minuteStamp))
	}
	latest := newSample(now)
	if len(c.detail) > 0 {
		latest = c.detail[len(c.detail)-1]
	}
	c.mu.Unlock()
	from := now - span.Milliseconds()
	for _, name := range MetricNames {
		points := []Point{}
		for _, s := range source {
			if s.At < from || s.At > now {
				continue
			}
			if m := s.Metrics[name]; m.Known() {
				points = append(points, Point{T: s.At, V: m.Value})
			}
		}
		out.Series[name] = points
		out.Status[name] = latest.Metrics[name]
	}
	return out, true
}

// Restore loads persisted rollups so charts survive a restart. Rows outside the
// retained day are dropped on the way in and deleted on the next write.
func (c *Collector) Restore(ctx context.Context) {
	if c.db == nil {
		return
	}
	cutoff := c.now().Add(-24 * time.Hour).UnixMilli()
	rows, e := c.db.QueryContext(ctx, `SELECT minute_ms,body FROM telemetry_rollups WHERE minute_ms>=? ORDER BY minute_ms LIMIT ?`, cutoff, RollupSamples)
	if e != nil {
		return
	}
	defer rows.Close()
	restored := []Sample{}
	var newest int64
	for rows.Next() {
		var at int64
		var body string
		if rows.Scan(&at, &body) != nil {
			return
		}
		var s Sample
		if json.Unmarshal([]byte(body), &s) != nil || s.Metrics == nil {
			continue
		}
		s.At = at
		restored = append(restored, s)
		newest = at
	}
	if rows.Err() != nil {
		return
	}
	c.mu.Lock()
	c.rollups = append(restored, c.rollups...)
	if len(c.rollups) > RollupSamples {
		c.rollups = c.rollups[len(c.rollups)-RollupSamples:]
	}
	c.persisted = newest
	c.mu.Unlock()
}

// persist writes rollups the database has not seen yet and prunes the day. It
// runs at most hourly, so the write is one small bounded batch.
func (c *Collector) persist(ctx context.Context, final bool) {
	if c.db == nil {
		return
	}
	now := c.now().UnixMilli()
	c.mu.Lock()
	pending := []Sample{}
	for _, s := range c.rollups {
		if s.At > c.persisted {
			pending = append(pending, s)
		}
	}
	if final && len(c.minute) > 0 {
		pending = append(pending, average(c.minute, c.minuteStamp))
	}
	c.lastPersist = now
	c.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	gated, e := dbwork.Begin(ctx, c.db, dbwork.ClassMaintenance)
	if e != nil {
		return
	}
	tx := gated.Tx()
	defer gated.Rollback()
	written := int64(0)
	for _, s := range pending {
		body, e := json.Marshal(s)
		if e != nil {
			continue
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO telemetry_rollups VALUES(?,?) ON CONFLICT(minute_ms) DO UPDATE SET body=excluded.body`, s.At, string(body)); e != nil {
			return
		}
		if s.At > written {
			written = s.At
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM telemetry_rollups WHERE minute_ms<?`, now-24*time.Hour.Milliseconds()); e != nil {
		return
	}
	if gated.Commit() != nil {
		return
	}
	c.mu.Lock()
	if written > c.persisted {
		c.persisted = written
	}
	c.mu.Unlock()
}

// Rollups is the retained one-minute history, oldest first. Tests assert bounds
// against it; nothing on the wire reads it directly.
func (c *Collector) Rollups() []Sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]Sample{}, c.rollups...)
	sort.Slice(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}
