package telemetry

import (
	"context"
	"database/sql"
	"math"
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
	persistEvery   = time.Minute
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
var Windows = []string{"live", "12h", "24h", "7d", "30d", "90d", "1y", "all", "10m", "1h"}

func windowSpan(window string) (time.Duration, bool) {
	switch window {
	case "live":
		return 2 * time.Minute, true
	case "12h":
		return 12 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	case "30d":
		return 30 * 24 * time.Hour, true
	case "90d":
		return 90 * 24 * time.Hour, true
	case "1y":
		return 365 * 24 * time.Hour, true
	case "all":
		return 100 * 365 * 24 * time.Hour, true
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
	source := append([]Sample{}, c.detail...)
	if window != "10m" && window != "live" {
		source = append(append([]Sample{}, c.rollups...), average(c.minute, c.minuteStamp))
	}
	latest := newSample(now)
	if len(c.detail) > 0 {
		latest = c.detail[len(c.detail)-1]
	}
	c.mu.Unlock()
	from := now - span.Milliseconds()
	if c.db != nil && window != "live" && window != "10m" && window != "1h" {
		if stored, err := c.readStored(context.Background(), window, from, now); err == nil {
			for name, points := range stored {
				out.Series[name] = points
			}
			for _, name := range MetricNames {
				out.Status[name] = latest.Metrics[name]
			}
			return out, true
		}
	}
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
		cap := 70
		if window == "24h" || window == "live" {
			cap = 60
		}
		if window == "10m" {
			cap = DetailSamples
		}
		if window == "1h" {
			cap = 60
		}
		if len(points) > cap {
			selected := make([]Point, cap)
			for n := range selected {
				selected[n] = points[n*(len(points)-1)/(cap-1)]
			}
			points = selected
		}
		out.Series[name] = points
		out.Status[name] = latest.Metrics[name]
	}
	return out, true
}

// Stored minute/hour/day/week facts have fixed retention, no serialized graph.
// Long reads group at the database into at most 70 buckets per metric.
var resolutions = []struct{ step, keep int64 }{
	{60000, 24 * 60 * 60000}, {3600000, 90 * 24 * 3600000},
	{86400000, 400 * 86400000}, {7 * 86400000, 5200 * 7 * 86400000},
}

func (c *Collector) Restore(ctx context.Context) {
	if c.db == nil {
		return
	}
	rows, e := c.db.QueryContext(ctx, `SELECT bucket_ms,metric,total/samples,status,detail FROM telemetry_rollups WHERE resolution_ms=60000 AND bucket_ms>=? ORDER BY bucket_ms,metric`, c.now().Add(-24*time.Hour).UnixMilli())
	if e != nil {
		return
	}
	defer rows.Close()
	list := []Sample{}
	for rows.Next() {
		var at int64
		var name string
		var m Metric
		if rows.Scan(&at, &name, &m.Value, &m.Status, &m.Detail) != nil {
			return
		}
		if len(list) == 0 || list[len(list)-1].At != at {
			list = append(list, newSample(at))
		}
		list[len(list)-1].Metrics[name] = m
	}
	if rows.Err() != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rollups = list
	if len(list) > 0 {
		c.persisted = list[len(list)-1].At
	}
}

// Only closed minutes are committed. An interrupted minute stays a gap after a
// restart; it is never represented as a complete minute or counted twice.
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
	c.mu.Unlock()
	gated, e := dbwork.Begin(ctx, c.db, dbwork.ClassMaintenance)
	if e != nil {
		return
	}
	defer gated.Rollback()
	tx := gated.Tx()
	for _, s := range pending {
		for _, name := range MetricNames {
			m := s.Metrics[name]
			if !m.Known() || math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
				continue
			}
			result, e := tx.ExecContext(ctx, `INSERT INTO telemetry_rollups(resolution_ms,bucket_ms,metric,samples,total,status,detail) VALUES(60000,?,?,1,?,?,?) ON CONFLICT DO NOTHING`, s.At, name, m.Value, m.Status, m.Detail)
			if e != nil {
				return
			}
			n, e := result.RowsAffected()
			if e != nil {
				return
			}
			if n == 0 {
				continue
			}
			for _, r := range resolutions[1:] {
				stamp := s.At - s.At%r.step
				_, e = tx.ExecContext(ctx, `INSERT INTO telemetry_rollups(resolution_ms,bucket_ms,metric,samples,total,status,detail) VALUES(?,?,?,1,?,?,?) ON CONFLICT(resolution_ms,bucket_ms,metric) DO UPDATE SET samples=samples+1,total=total+excluded.total,status=CASE WHEN status='limited' OR excluded.status='limited' THEN 'limited' ELSE 'available' END,detail=excluded.detail`, r.step, stamp, name, m.Value, m.Status, m.Detail)
				if e != nil {
					return
				}
			}
		}
	}
	for _, r := range resolutions {
		if _, e = tx.ExecContext(ctx, `DELETE FROM telemetry_rollups WHERE resolution_ms=? AND bucket_ms<?`, r.step, now-r.keep); e != nil {
			return
		}
	}
	if gated.Commit() != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPersist = now
	if len(pending) > 0 {
		c.persisted = pending[len(pending)-1].At
	}
}

func (c *Collector) readStored(ctx context.Context, window string, from, now int64) (map[string][]Point, error) {
	step := int64(60000)
	switch window {
	case "7d", "30d", "90d":
		step = 3600000
	case "1y":
		step = 86400000
	case "all":
		step = 7 * 86400000
	}
	if window == "all" {
		var earliest sql.NullInt64
		if e := c.db.QueryRowContext(ctx, `SELECT min(bucket_ms) FROM telemetry_rollups WHERE resolution_ms=?`, step).Scan(&earliest); e != nil {
			return nil, e
		}
		if earliest.Valid {
			from = max(from, earliest.Int64)
		}
	}
	width := max(step, (now-from+69)/70)
	// Stamp at the start of each aggregate; no interpolation or zero-filled gaps.
	rows, e := c.db.QueryContext(ctx, `SELECT metric, min(bucket_ms),sum(total)/sum(samples) FROM telemetry_rollups WHERE resolution_ms=? AND bucket_ms>=? AND bucket_ms<=? GROUP BY metric,(bucket_ms-?)/? ORDER BY metric,min(bucket_ms)`, step, from, now, from, width)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string][]Point{}
	for _, name := range MetricNames {
		out[name] = []Point{}
	}
	for rows.Next() {
		var name string
		var p Point
		if e = rows.Scan(&name, &p.T, &p.V); e != nil {
			return nil, e
		}
		if _, ok := out[name]; ok {
			out[name] = append(out[name], p)
		}
	}
	return out, rows.Err()
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
