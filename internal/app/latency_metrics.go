package app

import (
	"sync/atomic"
	"time"
)

// Fixed boundaries keep latency telemetry bounded regardless of traffic,
// route cardinality, account count, or uptime. No URL, media ID, profile ID,
// query text, or other user-controlled label is retained.
var latencyBucketUpperMillis = [...]int64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

const (
	latencyRouteClassCount = 12
	latencySQLiteLaneCount = 5
)

type LatencyDiagnostic struct {
	Count         uint64 `json:"count"`
	P50Millis     int64  `json:"p50Millis"`
	P95Millis     int64  `json:"p95Millis"`
	P99Millis     int64  `json:"p99Millis"`
	MaximumMillis int64  `json:"maximumMillis"`
}

type NamedLatencyDiagnostic struct {
	Lane string `json:"lane"`
	LatencyDiagnostic
}

type boundedLatencyHistogram struct {
	buckets [len(latencyBucketUpperMillis) + 1]atomic.Uint64
	maximum atomic.Int64
}

func (histogram *boundedLatencyHistogram) observe(duration time.Duration) {
	millis := duration.Milliseconds()
	if millis < 0 {
		millis = 0
	}
	bucket := len(latencyBucketUpperMillis)
	for index, upper := range latencyBucketUpperMillis {
		if millis <= upper {
			bucket = index
			break
		}
	}
	histogram.buckets[bucket].Add(1)
	for {
		current := histogram.maximum.Load()
		if millis <= current || histogram.maximum.CompareAndSwap(current, millis) {
			break
		}
	}
}

func (histogram *boundedLatencyHistogram) snapshot() LatencyDiagnostic {
	counts := make([]uint64, len(histogram.buckets))
	var total uint64
	for index := range histogram.buckets {
		counts[index] = histogram.buckets[index].Load()
		total += counts[index]
	}
	return LatencyDiagnostic{
		Count:         total,
		P50Millis:     latencyPercentileMillis(counts, total, 50),
		P95Millis:     latencyPercentileMillis(counts, total, 95),
		P99Millis:     latencyPercentileMillis(counts, total, 99),
		MaximumMillis: histogram.maximum.Load(),
	}
}

func latencyPercentileMillis(counts []uint64, total uint64, percentile uint64) int64 {
	if total == 0 {
		return 0
	}
	target := (total*percentile + 99) / 100
	var cumulative uint64
	for index, count := range counts {
		cumulative += count
		if cumulative < target {
			continue
		}
		if index < len(latencyBucketUpperMillis) {
			return latencyBucketUpperMillis[index]
		}
		return latencyBucketUpperMillis[len(latencyBucketUpperMillis)-1] + 1
	}
	return latencyBucketUpperMillis[len(latencyBucketUpperMillis)-1] + 1
}

type latencyMetricsRegistry struct {
	routeQueue   [latencyRouteClassCount]boundedLatencyHistogram
	routeService [latencyRouteClassCount]boundedLatencyHistogram
	sqliteRead   [latencySQLiteLaneCount]boundedLatencyHistogram
	sqliteWrite  [latencySQLiteLaneCount]boundedLatencyHistogram
}

func routeLatencyIndex(lane string) int {
	switch lane {
	case workloadLaneAuth:
		return 0
	case workloadLaneBrowsing:
		return 1
	case workloadLaneExpensive:
		return 2
	case workloadLanePlayback:
		return 3
	case workloadLaneMedia:
		return 4
	case workloadLaneMediaBody:
		return 5
	case workloadLaneBulkTransfer:
		return 6
	case workloadLaneRealtime:
		return 7
	case workloadLaneDLNA:
		return 8
	case workloadLaneAdmin:
		return 9
	case workloadLaneAdminHeavy:
		return 10
	default:
		return 11
	}
}

func sqliteLatencyIndex(lane string) int {
	switch lane {
	case "user":
		return 0
	case "background":
		return 1
	case "user_tx":
		return 2
	case "background_tx":
		return 3
	default:
		return 4
	}
}

var sqliteLatencyLaneNames = [...]string{"user", "background", "user_tx", "background_tx", "other"}

func (registry *latencyMetricsRegistry) observeRouteQueue(lane string, duration time.Duration) {
	registry.routeQueue[routeLatencyIndex(lane)].observe(duration)
}

func (registry *latencyMetricsRegistry) observeRouteService(lane string, duration time.Duration) {
	registry.routeService[routeLatencyIndex(lane)].observe(duration)
}

func (registry *latencyMetricsRegistry) observeSQLiteRead(lane string, duration time.Duration) {
	registry.sqliteRead[sqliteLatencyIndex(lane)].observe(duration)
}

func (registry *latencyMetricsRegistry) observeSQLiteWrite(lane string, duration time.Duration) {
	registry.sqliteWrite[sqliteLatencyIndex(lane)].observe(duration)
}

func (registry *latencyMetricsRegistry) routeSnapshot(lane string) (LatencyDiagnostic, LatencyDiagnostic) {
	index := routeLatencyIndex(lane)
	return registry.routeQueue[index].snapshot(), registry.routeService[index].snapshot()
}

func (registry *latencyMetricsRegistry) sqliteReadSnapshot() []NamedLatencyDiagnostic {
	result := make([]NamedLatencyDiagnostic, 0, latencySQLiteLaneCount)
	for index, lane := range sqliteLatencyLaneNames {
		result = append(result, NamedLatencyDiagnostic{Lane: lane, LatencyDiagnostic: registry.sqliteRead[index].snapshot()})
	}
	return result
}

func (registry *latencyMetricsRegistry) sqliteWriteSnapshot() []NamedLatencyDiagnostic {
	result := make([]NamedLatencyDiagnostic, 0, latencySQLiteLaneCount)
	for index, lane := range sqliteLatencyLaneNames {
		result = append(result, NamedLatencyDiagnostic{Lane: lane, LatencyDiagnostic: registry.sqliteWrite[index].snapshot()})
	}
	return result
}
