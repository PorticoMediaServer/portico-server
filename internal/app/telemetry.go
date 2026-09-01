package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	telemetryHistoryWindow           = 2 * time.Hour
	telemetryHistoryLimit            = 2048
	telemetryActiveSampleInterval    = 5 * time.Second
	telemetryIdleSampleInterval      = 30 * time.Second
	telemetryObservationGrace        = 10 * time.Second
	telemetryCollectionBudget        = 2 * time.Second
	telemetryCommandTimeout          = 500 * time.Millisecond
	telemetryCommandOutputLimit      = 64 << 10
	telemetryFileReadLimit           = 1 << 20
	telemetryGPUCacheInterval        = 15 * time.Second
	telemetryIntelProbeTimeout       = 750 * time.Millisecond
	telemetryIntelShutdownGrace      = 100 * time.Millisecond
	telemetryReasonLimit             = 256
	telemetryStatusOK                = "ok"
	telemetryStatusUnavailable       = "unavailable"
	telemetryStatusUnsupported       = "unsupported"
	telemetryStatusTimeout           = "timeout"
	telemetryStatusWarmingUp         = "warming_up"
	telemetryStatusOutputLimit       = "output_limit"
	telemetryStatusParseError        = "parse_error"
	telemetryStatusCanceled          = "canceled"
	telemetryStatusCollectionTimeout = "collection_timeout"
)

type telemetryMetric struct {
	Value  float64
	Status string
	Reason string
}

func availableTelemetryMetric(value float64) telemetryMetric {
	return telemetryMetric{Value: clampPercent(value), Status: telemetryStatusOK}
}

func unavailableTelemetryMetric(status, reason string) telemetryMetric {
	if strings.TrimSpace(status) == "" {
		status = telemetryStatusUnavailable
	}
	return telemetryMetric{Status: status, Reason: boundedTelemetryReason(reason)}
}

func boundedTelemetryReason(reason string) string {
	reason = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(reason, "\n", " "), "\r", " "))
	if len(reason) > telemetryReasonLimit {
		return reason[:telemetryReasonLimit]
	}
	return reason
}

type telemetryCPUState struct {
	Initialized  bool
	SystemTotal  uint64
	SystemBusy   uint64
	ProcessTotal uint64
}

type telemetryGPUSample struct {
	Usage   telemetryMetric
	Memory  telemetryMetric
	Encoder telemetryMetric
}

type telemetryGPUCache struct {
	At        time.Time
	Provider  string
	Device    string
	Available bool
	Sample    telemetryGPUSample
}

type telemetryIOPressureSampler func(context.Context) ioPressureSample

type telemetrySamplerState struct {
	cpu               telemetryCPUState
	gpu               telemetryGPUCache
	ioPressureSampler telemetryIOPressureSampler
}

type telemetryCollectionStats struct {
	commands      int
	timeouts      int
	outputLimited int
}

type telemetryCollectionContextKey struct{}

type systemTelemetryPoint struct {
	At                  time.Time
	ServerCPU           float64
	SystemCPU           float64
	ServerCPUState      telemetryMetric
	SystemCPUState      telemetryMetric
	ServerRAM           float64
	SystemRAM           float64
	ServerRAMState      telemetryMetric
	SystemRAMState      telemetryMetric
	SystemRAMUsedBytes  int64
	SystemRAMFreeBytes  int64
	SystemRAMTotalBytes int64
	GPUUsage            float64
	GPUMemory           float64
	GPUEncoder          float64
	GPUUsageState       telemetryMetric
	GPUMemoryState      telemetryMetric
	GPUEncoderState     telemetryMetric
	IOPressure          ioPressureSample
	CollectionStatus    string
	CollectionReason    string
	CommandCount        int
	CommandTimeouts     int
	CommandOutputLimits int
	CollectionMillis    int64
}

type memoryUsageSnapshot struct {
	UsedBytes  int64
	FreeBytes  int64
	TotalBytes int64
	Status     string
	Reason     string
}

type ioPressureLine struct {
	Avg10  float64
	Avg60  float64
	Avg300 float64
}

type diskStatsSample struct {
	Supported       bool
	ReadOperations  uint64
	WriteOperations uint64
	ReadSectors     uint64
	WriteSectors    uint64
	IOInProgress    uint64
}

type ioPressureSample struct {
	At        time.Time
	Supported bool
	Status    string
	Source    string
	Some      ioPressureLine
	Full      ioPressureLine
	Disk      diskStatsSample
	Error     string
}

func (s *Server) runTelemetrySampler(ctx context.Context) {
	state := &telemetrySamplerState{}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	s.appendTelemetrySampleContext(ctx, state)
	timer := time.NewTimer(s.telemetrySamplingInterval(time.Now()))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.appendTelemetrySampleContext(ctx, state)
			timer.Reset(s.telemetrySamplingInterval(time.Now()))
		}
	}
}

func (s *Server) appendTelemetrySample() {
	s.appendTelemetrySampleContext(context.Background(), &telemetrySamplerState{})
}

func (s *Server) appendTelemetrySampleContext(ctx context.Context, state *telemetrySamplerState) {
	if s == nil || (ctx != nil && ctx.Err() != nil) {
		return
	}
	s.telemetryMu.Lock()
	gpuInfo := s.gpuInfo
	s.telemetryMu.Unlock()
	point := sampleSystemTelemetryContext(ctx, state, gpuInfo)

	s.telemetryMu.Lock()
	defer s.telemetryMu.Unlock()
	s.telemetry = retainTelemetryPoint(s.telemetry, point)
}

func retainTelemetryPoint(points []systemTelemetryPoint, point systemTelemetryPoint) []systemTelemetryPoint {
	cutoff := point.At.Add(-telemetryHistoryWindow)
	keep := points[:0]
	for _, existing := range points {
		if existing.At.After(cutoff) {
			keep = append(keep, existing)
		}
	}
	if len(keep) >= telemetryHistoryLimit {
		keep = keep[len(keep)-telemetryHistoryLimit+1:]
	}
	return append(keep, point)
}

func (s *Server) telemetrySamplingInterval(now time.Time) time.Duration {
	if s != nil && s.telemetryDashboardObserved(now) {
		return telemetryActiveSampleInterval
	}
	return telemetryIdleSampleInterval
}

func sampleSystemTelemetry(gpuInfo DashboardGPUInfo) systemTelemetryPoint {
	return sampleSystemTelemetryContext(context.Background(), &telemetrySamplerState{}, gpuInfo)
}

func sampleSystemTelemetryContext(ctx context.Context, state *telemetrySamplerState, gpuInfo DashboardGPUInfo) systemTelemetryPoint {
	if ctx == nil {
		ctx = context.Background()
	}
	if state == nil {
		state = &telemetrySamplerState{}
	}
	collectionStart := time.Now()
	stats := &telemetryCollectionStats{}
	collectionCtx, cancel := context.WithTimeout(ctx, telemetryCollectionBudget)
	defer cancel()
	collectionCtx = context.WithValue(collectionCtx, telemetryCollectionContextKey{}, stats)

	cpuServer, cpuSystem := sampleCPUContext(collectionCtx, state)
	ramServer, ramSystem, memory := sampleRAMContext(collectionCtx)
	gpu := sampleGPUContext(collectionCtx, state, gpuInfo)
	ioSample := state.sampleIOPressure(collectionCtx)
	collectionStatus := telemetryStatusOK
	collectionReason := ""
	if err := collectionCtx.Err(); err != nil {
		collectionStatus = telemetryStatusCollectionTimeout
		collectionReason = boundedTelemetryReason(err.Error())
	}
	if collectionStatus == telemetryStatusOK {
		for _, metric := range []telemetryMetric{cpuServer, cpuSystem, ramServer, ramSystem, gpu.Usage, gpu.Memory, gpu.Encoder} {
			if metric.Status != telemetryStatusOK && metric.Status != telemetryStatusUnsupported {
				collectionStatus = "degraded"
				collectionReason = metric.Reason
				break
			}
		}
	}
	if collectionStatus == telemetryStatusOK && ioSample.Status != ioPressureStatusOK && ioSample.Status != ioPressureStatusUnsupported {
		collectionStatus = "degraded"
		collectionReason = boundedTelemetryReason(ioSample.Error)
		if collectionReason == "" {
			collectionReason = "io pressure telemetry is unavailable"
		}
	}
	return systemTelemetryPoint{
		At:                  time.Now().UTC(),
		ServerCPU:           cpuServer.Value,
		SystemCPU:           cpuSystem.Value,
		ServerCPUState:      cpuServer,
		SystemCPUState:      cpuSystem,
		ServerRAM:           ramServer.Value,
		SystemRAM:           ramSystem.Value,
		ServerRAMState:      ramServer,
		SystemRAMState:      ramSystem,
		SystemRAMUsedBytes:  memory.UsedBytes,
		SystemRAMFreeBytes:  memory.FreeBytes,
		SystemRAMTotalBytes: memory.TotalBytes,
		GPUUsage:            gpu.Usage.Value,
		GPUMemory:           gpu.Memory.Value,
		GPUEncoder:          gpu.Encoder.Value,
		GPUUsageState:       gpu.Usage,
		GPUMemoryState:      gpu.Memory,
		GPUEncoderState:     gpu.Encoder,
		IOPressure:          ioSample,
		CollectionStatus:    collectionStatus,
		CollectionReason:    boundedTelemetryReason(collectionReason),
		CommandCount:        stats.commands,
		CommandTimeouts:     stats.timeouts,
		CommandOutputLimits: stats.outputLimited,
		CollectionMillis:    maxInt64(0, time.Since(collectionStart).Milliseconds()),
	}
}

func (s *telemetrySamplerState) sampleIOPressure(ctx context.Context) ioPressureSample {
	if ctx == nil {
		ctx = context.Background()
	}
	sampler := telemetryIOPressureSampler(sampleIOPressureContext)
	if s != nil && s.ioPressureSampler != nil {
		sampler = s.ioPressureSampler
	}
	sample := sampler(ctx)
	if err := ctx.Err(); err != nil {
		return unavailableIOPressureSample(time.Now().UTC(), err)
	}
	return sample
}

func telemetryMetricStatus(metric telemetryMetric) TelemetryMetricStatus {
	status := strings.TrimSpace(metric.Status)
	if status == "" {
		status = telemetryStatusUnavailable
		if strings.TrimSpace(metric.Reason) == "" {
			metric.Reason = "telemetry metric did not report an availability state"
		}
	}
	return TelemetryMetricStatus{Status: status, Reason: boundedTelemetryReason(metric.Reason)}
}

func normalizedTelemetryMetric(metric telemetryMetric) telemetryMetric {
	status := telemetryMetricStatus(metric)
	metric.Status = status.Status
	metric.Reason = status.Reason
	return metric
}

func combinedTelemetryMetricState(metrics ...telemetryMetric) telemetryMetric {
	for index := len(metrics) - 1; index >= 0; index-- {
		if metrics[index].Status == telemetryStatusOK {
			return metrics[index]
		}
	}
	for index := len(metrics) - 1; index >= 0; index-- {
		if metrics[index].Status != "" {
			return metrics[index]
		}
	}
	return unavailableTelemetryMetric(telemetryStatusUnavailable, "telemetry metric did not report an availability state")
}

func (s *Server) telemetryDashboardObserved(now time.Time) bool {
	if s == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	s.dashboardCacheMu.Lock()
	observed := false
	for _, entry := range s.dashboardCache {
		if now.Before(entry.expiresAt) && now.Sub(entry.expiresAt.Add(-dashboardCacheTTL(dashboardFilters{}))) <= telemetryObservationGrace {
			observed = true
			break
		}
	}
	s.dashboardCacheMu.Unlock()
	if observed {
		return true
	}
	s.serverActivityMu.Lock()
	observed = now.Before(s.serverActivityCache.expiresAt) && now.Sub(s.serverActivityCache.expiresAt.Add(-serverActivityCacheTTL)) <= telemetryObservationGrace
	s.serverActivityMu.Unlock()
	return observed
}

const (
	linuxIOPressurePath         = "/proc/pressure/io"
	linuxDiskStatsPath          = "/proc/diskstats"
	ioPressureStatusOK          = "ok"
	ioPressureStatusUnsupported = "unsupported"
	ioPressureStatusUnavailable = "unavailable"
	ioPressureStatusParseError  = "parse_error"
	ioPressureSomeAvg10High     = 10.0
	ioPressureFullAvg10High     = 1.0
)

func sampleIOPressure() ioPressureSample {
	return sampleIOPressureContext(context.Background())
}

func sampleIOPressureContext(ctx context.Context) ioPressureSample {
	now := time.Now().UTC()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return unavailableIOPressureSample(now, err)
	}
	if runtime.GOOS != "linux" {
		return ioPressureSample{
			At:        now,
			Supported: false,
			Status:    ioPressureStatusUnsupported,
		}
	}
	data, err := readTelemetryFile(linuxIOPressurePath, telemetryFileReadLimit)
	if err != nil {
		return ioPressureSample{
			At:        now,
			Supported: false,
			Status:    ioPressureStatusUnavailable,
			Source:    linuxIOPressurePath,
			Error:     err.Error(),
		}
	}
	if err := ctx.Err(); err != nil {
		return unavailableIOPressureSample(now, err)
	}
	sample, err := parseLinuxIOPressure(string(data))
	sample.At = now
	sample.Source = linuxIOPressurePath
	if err != nil {
		sample.Supported = false
		sample.Status = ioPressureStatusParseError
		sample.Error = err.Error()
		return sample
	}
	sample.Supported = true
	sample.Status = ioPressureStatusOK
	if err := ctx.Err(); err != nil {
		return unavailableIOPressureSample(now, err)
	}
	if disk, err := sampleLinuxDiskStatsContext(ctx); err == nil {
		sample.Disk = disk
	} else if ctx.Err() != nil {
		return unavailableIOPressureSample(now, ctx.Err())
	}
	return sample
}

func unavailableIOPressureSample(now time.Time, err error) ioPressureSample {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	reason := "io pressure telemetry is unavailable"
	if err != nil {
		reason = err.Error()
	}
	return ioPressureSample{
		At:        now,
		Supported: false,
		Status:    ioPressureStatusUnavailable,
		Source:    linuxIOPressurePath,
		Error:     boundedTelemetryReason(reason),
	}
}

func readTelemetryFile(path string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("telemetry file limit must be positive")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("telemetry file %q exceeds %d-byte limit", path, limit)
	}
	return data, nil
}

func parseLinuxIOPressure(raw string) (ioPressureSample, error) {
	var sample ioPressureSample
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sample, errors.New("empty io pressure payload")
	}
	foundSome := false
	foundFull := false
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		values := map[string]float64{}
		for _, field := range fields[1:] {
			key, valueText, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			value, err := strconv.ParseFloat(strings.TrimSpace(valueText), 64)
			if err != nil {
				return sample, fmt.Errorf("parse io pressure %s: %w", key, err)
			}
			values[key] = value
		}
		pressure := ioPressureLine{
			Avg10:  values["avg10"],
			Avg60:  values["avg60"],
			Avg300: values["avg300"],
		}
		switch fields[0] {
		case "some":
			sample.Some = pressure
			foundSome = true
		case "full":
			sample.Full = pressure
			foundFull = true
		}
	}
	if !foundSome || !foundFull {
		return sample, errors.New("io pressure payload missing some/full lines")
	}
	return sample, nil
}

func sampleLinuxDiskStats() (diskStatsSample, error) {
	return sampleLinuxDiskStatsContext(context.Background())
}

func sampleLinuxDiskStatsContext(ctx context.Context) (diskStatsSample, error) {
	if runtime.GOOS != "linux" {
		return diskStatsSample{}, errors.New("diskstats unsupported on this platform")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return diskStatsSample{}, err
	}
	data, err := readTelemetryFile(linuxDiskStatsPath, telemetryFileReadLimit)
	if err != nil {
		return diskStatsSample{}, err
	}
	if err := ctx.Err(); err != nil {
		return diskStatsSample{}, err
	}
	return parseLinuxDiskStats(string(data))
}

func parseLinuxDiskStats(raw string) (diskStatsSample, error) {
	var stats diskStatsSample
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		if skipDiskStatsDevice(name) {
			continue
		}
		readOps, err := strconv.ParseUint(fields[3], 10, 64)
		if err != nil {
			return stats, err
		}
		readSectors, err := strconv.ParseUint(fields[5], 10, 64)
		if err != nil {
			return stats, err
		}
		writeOps, err := strconv.ParseUint(fields[7], 10, 64)
		if err != nil {
			return stats, err
		}
		writeSectors, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			return stats, err
		}
		ioInProgress, err := strconv.ParseUint(fields[11], 10, 64)
		if err != nil {
			return stats, err
		}
		stats.ReadOperations += readOps
		stats.ReadSectors += readSectors
		stats.WriteOperations += writeOps
		stats.WriteSectors += writeSectors
		stats.IOInProgress += ioInProgress
		stats.Supported = true
	}
	if !stats.Supported {
		return stats, errors.New("diskstats payload had no physical devices")
	}
	return stats, nil
}

func skipDiskStatsDevice(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	for _, prefix := range []string{"loop", "ram", "zram", "fd", "sr"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func ioPressureIsHigh(sample ioPressureSample) bool {
	return sample.Supported &&
		(sample.Some.Avg10 >= ioPressureSomeAvg10High || sample.Full.Avg10 >= ioPressureFullAvg10High)
}

func ioPressureDiagnosticsFromPoints(points []systemTelemetryPoint, fallback ioPressureSample) IOPressureDiagnostics {
	samples := []ioPressureSample{}
	for _, point := range points {
		if point.IOPressure.Status != "" {
			samples = append(samples, point.IOPressure)
		}
	}
	if len(samples) == 0 && fallback.Status != "" {
		samples = append(samples, fallback)
	}
	if len(samples) == 0 {
		fallback = ioPressureSample{
			At:        time.Now().UTC(),
			Supported: false,
			Status:    ioPressureStatusUnavailable,
			Source:    linuxIOPressurePath,
			Error:     "no io pressure samples available",
		}
		samples = append(samples, fallback)
	}
	latest := samples[len(samples)-1]
	diagnostics := IOPressureDiagnostics{
		Supported:           latest.Supported,
		Status:              latest.Status,
		Source:              latest.Source,
		CurrentSomeAvg10:    latest.Some.Avg10,
		CurrentSomeAvg60:    latest.Some.Avg60,
		CurrentSomeAvg300:   latest.Some.Avg300,
		CurrentFullAvg10:    latest.Full.Avg10,
		CurrentFullAvg60:    latest.Full.Avg60,
		CurrentFullAvg300:   latest.Full.Avg300,
		Samples:             len(samples),
		DiskStatsSupported:  latest.Disk.Supported,
		DiskReadOperations:  latest.Disk.ReadOperations,
		DiskWriteOperations: latest.Disk.WriteOperations,
		DiskReadSectors:     latest.Disk.ReadSectors,
		DiskWriteSectors:    latest.Disk.WriteSectors,
		DiskIOInProgress:    latest.Disk.IOInProgress,
		Error:               latest.Error,
	}
	for _, sample := range samples {
		diagnostics.HighestRecentSomeAvg10 = math.Max(diagnostics.HighestRecentSomeAvg10, sample.Some.Avg10)
		diagnostics.HighestRecentSomeAvg60 = math.Max(diagnostics.HighestRecentSomeAvg60, sample.Some.Avg60)
		diagnostics.HighestRecentSomeAvg300 = math.Max(diagnostics.HighestRecentSomeAvg300, sample.Some.Avg300)
		diagnostics.HighestRecentFullAvg10 = math.Max(diagnostics.HighestRecentFullAvg10, sample.Full.Avg10)
		diagnostics.HighestRecentFullAvg60 = math.Max(diagnostics.HighestRecentFullAvg60, sample.Full.Avg60)
		diagnostics.HighestRecentFullAvg300 = math.Max(diagnostics.HighestRecentFullAvg300, sample.Full.Avg300)
		if ioPressureIsHigh(sample) && (diagnostics.LastHighPressureAt == "" || sample.At.After(parseRFC3339Quiet(diagnostics.LastHighPressureAt))) {
			diagnostics.LastHighPressureAt = sample.At.Format(time.RFC3339)
		}
	}
	return diagnostics
}

func (s *Server) ioPressureDiagnostics() IOPressureDiagnostics {
	if s == nil {
		return ioPressureDiagnosticsFromPoints(nil, sampleIOPressure())
	}
	s.telemetryMu.Lock()
	points := append([]systemTelemetryPoint(nil), s.telemetry...)
	s.telemetryMu.Unlock()
	var fallback ioPressureSample
	if len(points) == 0 {
		fallback = sampleIOPressure()
	}
	return ioPressureDiagnosticsFromPoints(points, fallback)
}

func parseRFC3339Quiet(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339, strings.TrimSpace(value))
	return parsed
}

func sampleCPU() (float64, float64) {
	server, system := sampleCPUContext(context.Background(), &telemetrySamplerState{})
	return server.Value, system.Value
}

func sampleCPUContext(ctx context.Context, state *telemetrySamplerState) (telemetryMetric, telemetryMetric) {
	if ctx == nil {
		ctx = context.Background()
	}
	if state == nil {
		state = &telemetrySamplerState{}
	}
	return samplePlatformCPU(ctx, state)
}

func sampleRAM() (float64, float64, memoryUsageSnapshot) {
	server, system, memory := sampleRAMContext(context.Background())
	return server.Value, system.Value, memory
}

func sampleRAMContext(ctx context.Context) (telemetryMetric, telemetryMetric, memoryUsageSnapshot) {
	if ctx == nil {
		ctx = context.Background()
	}
	return samplePlatformMemory(ctx)
}

func totalMemoryBytes() float64 {
	_, _, memory := samplePlatformMemory(context.Background())
	return float64(memory.TotalBytes)
}

func systemMemoryUsage(totalBytes float64) memoryUsageSnapshot {
	_, _, memory := samplePlatformMemory(context.Background())
	_ = totalBytes
	return memory
}

func boundedMemoryUsage(totalBytes float64, freeBytes float64) memoryUsageSnapshot {
	if totalBytes <= 0 {
		return memoryUsageSnapshot{Status: telemetryStatusUnavailable, Reason: "total memory is unavailable"}
	}
	freeBytes = math.Max(0, math.Min(totalBytes, freeBytes))
	usedBytes := math.Max(0, totalBytes-freeBytes)
	return memoryUsageSnapshot{
		UsedBytes:  memoryByteCount(usedBytes),
		FreeBytes:  memoryByteCount(freeBytes),
		TotalBytes: memoryByteCount(totalBytes),
		Status:     telemetryStatusOK,
	}
}

func availableMemoryMetric(memory memoryUsageSnapshot) telemetryMetric {
	if memory.Status != telemetryStatusOK || memory.TotalBytes <= 0 {
		status := memory.Status
		if status == "" {
			status = telemetryStatusUnavailable
		}
		return unavailableTelemetryMetric(status, memory.Reason)
	}
	return availableTelemetryMetric(float64(memory.UsedBytes) / float64(memory.TotalBytes) * 100)
}

func serverErrString(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	return err.Error()
}

func memoryByteCount(value float64) int64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}
	if value >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(math.Round(value))
}

func detectGPUInfo() DashboardGPUInfo {
	return detectPlatformGPUInfo()
}

func sampleGPU(info DashboardGPUInfo) (float64, float64, float64) {
	sample := sampleGPUContext(context.Background(), &telemetrySamplerState{}, info)
	return sample.Usage.Value, sample.Memory.Value, sample.Encoder.Value
}

func sampleGPUContext(ctx context.Context, state *telemetrySamplerState, info DashboardGPUInfo) telemetryGPUSample {
	if ctx == nil {
		ctx = context.Background()
	}
	if state == nil {
		state = &telemetrySamplerState{}
	}
	if telemetryGPUCacheValid(state.gpu, info, time.Now()) {
		return state.gpu.Sample
	}
	cacheSample := func(sample telemetryGPUSample) telemetryGPUSample {
		state.gpu = telemetryGPUCache{
			At:        time.Now().UTC(),
			Provider:  info.Provider,
			Device:    info.Device,
			Available: info.Available,
			Sample:    sample,
		}
		return sample
	}

	unsupported := func(reason string) telemetryGPUSample {
		return telemetryGPUSample{
			Usage:   unavailableTelemetryMetric(telemetryStatusUnsupported, reason),
			Memory:  unavailableTelemetryMetric(telemetryStatusUnsupported, reason),
			Encoder: unavailableTelemetryMetric(telemetryStatusUnsupported, reason),
		}
	}
	if !info.Available {
		reason := info.Note
		if strings.TrimSpace(reason) == "" {
			reason = "GPU telemetry collector is unavailable"
		}
		return cacheSample(unsupported(reason))
	}

	var sample telemetryGPUSample
	switch info.Provider {
	case "NVIDIA":
		if _, err := exec.LookPath("nvidia-smi"); err != nil {
			return cacheSample(samplePlatformGPU(ctx, info))
		}
		output, err := runTelemetryCommand(ctx, "nvidia-smi", "--query-gpu=utilization.gpu,utilization.memory,utilization.encoder", "--format=csv,noheader,nounits")
		if err != nil {
			return cacheSample(unavailableGPUSample(telemetryCommandMetricStatus(err), err.Error()))
		}
		values := csvFloats(output)
		if len(values) < 3 {
			return cacheSample(unavailableGPUSample(telemetryStatusParseError, "nvidia-smi returned fewer than three GPU metrics"))
		}
		sample = telemetryGPUSample{
			Usage:   availableTelemetryMetric(values[0]),
			Memory:  availableTelemetryMetric(values[1]),
			Encoder: availableTelemetryMetric(values[2]),
		}
	case "Apple Silicon", "Windows":
		return cacheSample(samplePlatformGPU(ctx, info))
	case "Intel":
		sample = samplePlatformGPU(ctx, info)
		if gpuSampleHasData(sample) {
			return cacheSample(sample)
		}
		if _, err := exec.LookPath("intel_gpu_top"); err != nil {
			return cacheSample(sample)
		}
		output, err := runIntelGPUSample(ctx)
		if err != nil {
			return cacheSample(unavailableGPUSample(telemetryCommandMetricStatus(err), err.Error()))
		}
		sample, err = parseIntelGPUSample(output)
		if err != nil {
			return cacheSample(unavailableGPUSample(telemetryStatusParseError, err.Error()))
		}
	case "AMD":
		sample = samplePlatformGPU(ctx, info)
		if gpuSampleHasData(sample) {
			return cacheSample(sample)
		}
		if _, rocmErr := exec.LookPath("rocm-smi"); rocmErr != nil {
			if _, amdErr := exec.LookPath("amd-smi"); amdErr != nil {
				return cacheSample(sample)
			}
		}
		output, err := runTelemetryCommand(ctx, "rocm-smi", "--showuse", "--showmemuse")
		if err != nil || strings.TrimSpace(output) == "" {
			output, err = runTelemetryCommand(ctx, "amd-smi", "metric")
		}
		if err != nil {
			return cacheSample(unavailableGPUSample(telemetryCommandMetricStatus(err), err.Error()))
		}
		values := percentFloats(output)
		if len(values) < 2 {
			return cacheSample(unavailableGPUSample(telemetryStatusParseError, "AMD GPU collector returned fewer than two GPU metrics"))
		}
		sample = telemetryGPUSample{
			Usage:   availableTelemetryMetric(values[0]),
			Memory:  availableTelemetryMetric(values[1]),
			Encoder: unavailableTelemetryMetric(telemetryStatusUnsupported, "AMD GPU collector does not report encoder utilization"),
		}
	default:
		return cacheSample(unsupported("GPU telemetry provider is not supported"))
	}
	return cacheSample(sample)
}

func gpuSampleHasData(sample telemetryGPUSample) bool {
	return sample.Usage.Status == telemetryStatusOK || sample.Memory.Status == telemetryStatusOK || sample.Encoder.Status == telemetryStatusOK
}

func telemetryGPUCacheValid(cache telemetryGPUCache, info DashboardGPUInfo, now time.Time) bool {
	if cache.At.IsZero() || cache.Provider != info.Provider || cache.Device != info.Device || cache.Available != info.Available {
		return false
	}
	age := now.Sub(cache.At)
	return age >= 0 && age < telemetryGPUCacheInterval
}

func unavailableGPUSample(status, reason string) telemetryGPUSample {
	return telemetryGPUSample{
		Usage:   unavailableTelemetryMetric(status, reason),
		Memory:  unavailableTelemetryMetric(status, reason),
		Encoder: unavailableTelemetryMetric(status, reason),
	}
}

type telemetryCommandReadResult struct {
	output []byte
	err    error
}

func runIntelGPUSample(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if stats, ok := ctx.Value(telemetryCollectionContextKey{}).(*telemetryCollectionStats); ok && stats != nil {
		stats.commands++
	}
	probeCtx, cancel := context.WithTimeout(ctx, telemetryIntelProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, "intel_gpu_top", "-J", "-s", "100")
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	prepareManagedBackgroundCommand(cmd)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	lowerManagedBackgroundPriority(cmd)

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	readCh := make(chan telemetryCommandReadResult, 1)
	go func() {
		readCh <- readSingleTelemetryJSON(stdout, telemetryCommandOutputLimit)
	}()

	select {
	case result := <-readCh:
		if result.err != nil {
			killManagedBackgroundCommand(cmd)
			<-waitCh
			return "", result.err
		}
		// A clean 'q' gives intel_gpu_top a chance to terminate with a valid
		// JSON root. If a collector ignores stdin, the process-group kill below
		// still prevents a persistent child from surviving the sample.
		_, _ = io.WriteString(stdin, "q\n")
		_ = stdin.Close()
		select {
		case <-waitCh:
		case <-time.After(telemetryIntelShutdownGrace):
			killManagedBackgroundCommand(cmd)
			<-waitCh
		}
		return string(result.output), nil
	case <-probeCtx.Done():
		killManagedBackgroundCommand(cmd)
		<-waitCh
		if stats, ok := ctx.Value(telemetryCollectionContextKey{}).(*telemetryCollectionStats); ok && stats != nil && errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			stats.timeouts++
		}
		return "", probeCtx.Err()
	}
}

func readSingleTelemetryJSON(reader io.Reader, limit int) telemetryCommandReadResult {
	if limit <= 0 {
		return telemetryCommandReadResult{err: errors.New("telemetry JSON limit must be positive")}
	}
	buffer := make([]byte, 0, minInt(limit, 4096))
	chunk := make([]byte, 4096)
	for {
		read, err := reader.Read(chunk)
		if read > 0 {
			if len(buffer)+read > limit {
				return telemetryCommandReadResult{err: errManagedCommandOutputLimit}
			}
			buffer = append(buffer, chunk[:read]...)
			if telemetryJSONPayloadReady(buffer) {
				return telemetryCommandReadResult{output: buffer}
			}
		}
		if err != nil {
			if err == io.EOF && telemetryJSONPayloadReady(buffer) {
				return telemetryCommandReadResult{output: buffer}
			}
			if err == io.EOF {
				return telemetryCommandReadResult{err: errors.New("Intel GPU collector returned no complete JSON sample")}
			}
			return telemetryCommandReadResult{err: err}
		}
	}
}

func telemetryJSONPayloadReady(raw []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return false
	}
	switch typed := payload.(type) {
	case map[string]any:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	default:
		return false
	}
}

func parseIntelGPUSample(raw string) (telemetryGPUSample, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return telemetryGPUSample{}, err
	}
	if values, ok := payload.([]any); ok {
		if len(values) == 0 {
			return telemetryGPUSample{}, errors.New("Intel GPU collector returned an empty JSON array")
		}
		payload = values[0]
	}
	object, ok := payload.(map[string]any)
	if !ok {
		return telemetryGPUSample{}, errors.New("Intel GPU collector returned a non-object JSON sample")
	}
	usage := intelJSONMetric(object, "Render/3D/0")
	memory := intelJSONMetric(object, "Device memory")
	encoder := intelJSONMetric(object, "Video/0")
	if usage.Status != telemetryStatusOK && memory.Status != telemetryStatusOK && encoder.Status != telemetryStatusOK {
		return telemetryGPUSample{}, errors.New("Intel GPU collector returned no supported utilization metrics")
	}
	return telemetryGPUSample{Usage: usage, Memory: memory, Encoder: encoder}, nil
}

func intelJSONMetric(payload map[string]any, key string) telemetryMetric {
	value, ok := jsonMetricValue(payload, key)
	if !ok {
		return unavailableTelemetryMetric(telemetryStatusUnsupported, "Intel GPU collector did not report "+key)
	}
	return availableTelemetryMetric(value)
}

func jsonMetricValue(value any, key string) (float64, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if child, ok := typed[key]; ok {
			if number, ok := jsonMetricNumber(child); ok {
				return number, true
			}
			if nested, ok := child.(map[string]any); ok {
				for _, childValue := range nested {
					if number, ok := jsonMetricNumber(childValue); ok {
						return number, true
					}
				}
			}
		}
		for _, child := range typed {
			if number, ok := jsonMetricValue(child, key); ok {
				return number, true
			}
		}
	case []any:
		for _, child := range typed {
			if number, ok := jsonMetricValue(child, key); ok {
				return number, true
			}
		}
	}
	return 0, false
}

func jsonMetricNumber(value any) (float64, bool) {
	number, ok := value.(float64)
	return number, ok && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func telemetryCommandMetricStatus(err error) string {
	if err == nil {
		return telemetryStatusUnavailable
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return telemetryStatusTimeout
	}
	if errors.Is(err, context.Canceled) {
		return telemetryStatusCanceled
	}
	if errors.Is(err, errManagedCommandOutputLimit) {
		return telemetryStatusOutputLimit
	}
	return telemetryStatusUnavailable
}

func commandFloat(name string, args ...string) float64 {
	value, err := commandFloatContext(context.Background(), name, args...)
	if err != nil {
		return 0
	}
	return value
}

func commandFloatContext(ctx context.Context, name string, args ...string) (float64, error) {
	value, err := runTelemetryCommand(ctx, name, args...)
	if err != nil {
		return 0, err
	}
	if value == "" {
		return 0, errors.New("command returned empty output")
	}
	fields := strings.Fields(value)
	if len(fields) > 0 {
		value = fields[0]
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, errors.New("command returned a non-finite number")
	}
	return parsed, nil
}

func commandString(name string, args ...string) string {
	output, err := runTelemetryCommand(context.Background(), name, args...)
	if err != nil {
		return ""
	}
	return output
}

func runTelemetryCommand(ctx context.Context, name string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if stats, ok := ctx.Value(telemetryCollectionContextKey{}).(*telemetryCollectionStats); ok && stats != nil {
		stats.commands++
	}
	commandCtx, cancel := context.WithTimeout(ctx, telemetryCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, name, args...)
	cmd.Stderr = io.Discard
	output, err := managedCommandOutputLimit(commandCtx, cmd, telemetryCommandOutputLimit)
	if err != nil {
		if stats, ok := ctx.Value(telemetryCollectionContextKey{}).(*telemetryCollectionStats); ok && stats != nil {
			if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
				stats.timeouts++
			}
			if errors.Is(err, errManagedCommandOutputLimit) {
				stats.outputLimited++
			}
		}
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("%s telemetry command timed out: %w", name, context.DeadlineExceeded)
		}
		if errors.Is(commandCtx.Err(), context.Canceled) {
			return "", fmt.Errorf("%s telemetry command canceled: %w", name, context.Canceled)
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func firstCSVField(value string) string {
	line := strings.TrimSpace(strings.Split(value, "\n")[0])
	if line == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(line, ",")[0])
}

func csvFloats(value string) []float64 {
	line := strings.TrimSpace(strings.Split(value, "\n")[0])
	if line == "" {
		return nil
	}
	values := []float64{}
	for _, field := range strings.Split(line, ",") {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
		if err == nil {
			values = append(values, parsed)
		}
	}
	return values
}

func percentFloats(value string) []float64 {
	values := []float64{}
	for _, field := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == ',' || r == ':' || r == '%'
	}) {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
		if err == nil && parsed >= 0 && parsed <= 100 {
			values = append(values, parsed)
		}
	}
	return values
}

func jsonFloat(payload map[string]any, key string) float64 {
	if value, ok := payload[key]; ok {
		switch typed := value.(type) {
		case float64:
			return typed
		case map[string]any:
			for _, child := range typed {
				if number, ok := child.(float64); ok {
					return number
				}
			}
		}
	}
	return 0
}

func clampPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return math.Round(value*10) / 10
}
