//go:build !windows

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func samplePlatformCPU(ctx context.Context, state *telemetrySamplerState) (telemetryMetric, telemetryMetric) {
	if err := ctx.Err(); err != nil {
		return unavailableTelemetryMetric(telemetryStatusCanceled, err.Error()), unavailableTelemetryMetric(telemetryStatusCanceled, err.Error())
	}
	switch runtime.GOOS {
	case "linux":
		return sampleLinuxCPU(ctx, state)
	case "darwin":
		return sampleDarwinCPU(ctx)
	default:
		reason := "CPU telemetry is unavailable on " + runtime.GOOS
		return unavailableTelemetryMetric(telemetryStatusUnsupported, reason), unavailableTelemetryMetric(telemetryStatusUnsupported, reason)
	}
}

func sampleLinuxCPU(ctx context.Context, state *telemetrySamplerState) (telemetryMetric, telemetryMetric) {
	statRaw, err := readTelemetryFile("/proc/stat", telemetryFileReadLimit)
	if err != nil {
		reason := "read /proc/stat: " + err.Error()
		return unavailableTelemetryMetric(telemetryStatusUnavailable, reason), unavailableTelemetryMetric(telemetryStatusUnavailable, reason)
	}
	systemTotal, systemBusy, err := parseLinuxCPUStat(string(statRaw))
	if err != nil {
		reason := "parse /proc/stat: " + err.Error()
		return unavailableTelemetryMetric(telemetryStatusParseError, reason), unavailableTelemetryMetric(telemetryStatusParseError, reason)
	}
	processRaw, err := readTelemetryFile(fmt.Sprintf("/proc/%d/stat", os.Getpid()), telemetryFileReadLimit)
	if err != nil {
		reason := "read process CPU counters: " + err.Error()
		return unavailableTelemetryMetric(telemetryStatusUnavailable, reason), unavailableTelemetryMetric(telemetryStatusUnavailable, reason)
	}
	processTotal, err := parseLinuxProcessCPUStat(string(processRaw))
	if err != nil {
		reason := "parse process CPU counters: " + err.Error()
		return unavailableTelemetryMetric(telemetryStatusParseError, reason), unavailableTelemetryMetric(telemetryStatusParseError, reason)
	}
	if !state.cpu.Initialized {
		state.cpu = telemetryCPUState{Initialized: true, SystemTotal: systemTotal, SystemBusy: systemBusy, ProcessTotal: processTotal}
		reason := "CPU counters require a previous sample"
		return unavailableTelemetryMetric(telemetryStatusWarmingUp, reason), unavailableTelemetryMetric(telemetryStatusWarmingUp, reason)
	}
	if systemTotal <= state.cpu.SystemTotal || processTotal < state.cpu.ProcessTotal || systemBusy < state.cpu.SystemBusy {
		state.cpu = telemetryCPUState{Initialized: true, SystemTotal: systemTotal, SystemBusy: systemBusy, ProcessTotal: processTotal}
		reason := "CPU counters moved backwards or did not advance"
		return unavailableTelemetryMetric(telemetryStatusWarmingUp, reason), unavailableTelemetryMetric(telemetryStatusWarmingUp, reason)
	}
	totalDelta := systemTotal - state.cpu.SystemTotal
	busyDelta := systemBusy - state.cpu.SystemBusy
	processDelta := processTotal - state.cpu.ProcessTotal
	state.cpu = telemetryCPUState{Initialized: true, SystemTotal: systemTotal, SystemBusy: systemBusy, ProcessTotal: processTotal}
	if totalDelta == 0 {
		reason := "CPU counter interval was empty"
		return unavailableTelemetryMetric(telemetryStatusWarmingUp, reason), unavailableTelemetryMetric(telemetryStatusWarmingUp, reason)
	}
	return availableTelemetryMetric(float64(processDelta) / float64(totalDelta) * 100), availableTelemetryMetric(float64(busyDelta) / float64(totalDelta) * 100)
}

func parseLinuxCPUStat(raw string) (uint64, uint64, error) {
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		var total, idle uint64
		for index, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return 0, 0, err
			}
			if ^uint64(0)-total < value {
				return 0, 0, errors.New("CPU counter overflow")
			}
			total += value
			if index == 3 || index == 4 {
				if ^uint64(0)-idle < value {
					return 0, 0, errors.New("idle CPU counter overflow")
				}
				idle += value
			}
		}
		if idle > total {
			return 0, 0, errors.New("idle CPU counters exceed total")
		}
		return total, total - idle, nil
	}
	return 0, 0, errors.New("aggregate CPU line is missing")
}

func parseLinuxProcessCPUStat(raw string) (uint64, error) {
	closeParen := strings.LastIndex(raw, ")")
	if closeParen < 0 || closeParen+1 >= len(raw) {
		return 0, errors.New("process name terminator is missing")
	}
	fields := strings.Fields(raw[closeParen+1:])
	// After pid and comm, fields[0] is state. Linux utime/stime are fields
	// 14/15 in proc(5), therefore positions 11/12 in this suffix.
	if len(fields) <= 12 {
		return 0, errors.New("process CPU fields are missing")
	}
	user, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return 0, err
	}
	system, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return 0, err
	}
	if ^uint64(0)-user < system {
		return 0, errors.New("process CPU counter overflow")
	}
	return user + system, nil
}

func sampleDarwinCPU(ctx context.Context) (telemetryMetric, telemetryMetric) {
	cores := max(1, runtime.NumCPU())
	serverRaw, serverErr := commandFloatContext(ctx, "ps", "-p", strconv.Itoa(os.Getpid()), "-o", "%cpu=")
	if serverErr != nil {
		serverRaw = 0
	}
	systemOutput, systemErr := runTelemetryCommand(ctx, "ps", "-A", "-o", "%cpu=")
	if systemErr != nil {
		serverMetric := unavailableTelemetryMetric(telemetryCommandMetricStatus(serverErr), serverErrString(serverErr, "process CPU sample unavailable"))
		return serverMetric, unavailableTelemetryMetric(telemetryCommandMetricStatus(systemErr), systemErr.Error())
	}
	total := 0.0
	count := 0
	for _, field := range strings.Fields(systemOutput) {
		value, err := strconv.ParseFloat(field, 64)
		if err != nil || mathIsInvalidPercent(value) {
			continue
		}
		total += value
		count++
	}
	if count == 0 {
		return unavailableTelemetryMetric(telemetryCommandMetricStatus(serverErr), serverErrString(serverErr, "process CPU sample unavailable")), unavailableTelemetryMetric(telemetryStatusParseError, "ps returned no CPU samples")
	}
	serverMetric := availableTelemetryMetric(serverRaw / float64(cores))
	if serverErr != nil {
		serverMetric = unavailableTelemetryMetric(telemetryCommandMetricStatus(serverErr), serverErr.Error())
	}
	return serverMetric, availableTelemetryMetric(total / float64(cores))
}

func samplePlatformMemory(ctx context.Context) (telemetryMetric, telemetryMetric, memoryUsageSnapshot) {
	if err := ctx.Err(); err != nil {
		metric := unavailableTelemetryMetric(telemetryStatusCanceled, err.Error())
		return metric, metric, memoryUsageSnapshot{Status: telemetryStatusCanceled, Reason: err.Error()}
	}
	switch runtime.GOOS {
	case "linux":
		return sampleLinuxMemory(ctx)
	case "darwin":
		return sampleDarwinMemory(ctx)
	default:
		reason := "memory telemetry is unavailable on " + runtime.GOOS
		metric := unavailableTelemetryMetric(telemetryStatusUnsupported, reason)
		return metric, metric, memoryUsageSnapshot{Status: telemetryStatusUnsupported, Reason: reason}
	}
}

func sampleLinuxMemory(ctx context.Context) (telemetryMetric, telemetryMetric, memoryUsageSnapshot) {
	data, err := readTelemetryFile("/proc/meminfo", telemetryFileReadLimit)
	if err != nil {
		metric := unavailableTelemetryMetric(telemetryStatusUnavailable, "read /proc/meminfo: "+err.Error())
		return metric, metric, memoryUsageSnapshot{Status: telemetryStatusUnavailable, Reason: err.Error()}
	}
	total, available, err := parseLinuxMeminfo(string(data))
	if err != nil {
		metric := unavailableTelemetryMetric(telemetryStatusParseError, "parse /proc/meminfo: "+err.Error())
		return metric, metric, memoryUsageSnapshot{Status: telemetryStatusParseError, Reason: err.Error()}
	}
	memory := boundedMemoryUsage(total, available)
	processData, processErr := readTelemetryFile(fmt.Sprintf("/proc/%d/status", os.Getpid()), telemetryFileReadLimit)
	if processErr != nil {
		return unavailableTelemetryMetric(telemetryStatusUnavailable, processErr.Error()), availableMemoryMetric(memory), memory
	}
	rss, processErr := parseLinuxRSS(string(processData))
	if processErr != nil {
		return unavailableTelemetryMetric(telemetryStatusParseError, processErr.Error()), availableMemoryMetric(memory), memory
	}
	return availableTelemetryMetric(float64(rss) / total * 100), availableMemoryMetric(memory), memory
}

func parseLinuxMeminfo(raw string) (float64, float64, error) {
	values := map[string]float64{}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil || value < 0 {
			continue
		}
		values[strings.TrimSuffix(fields[0], ":")] = value * 1024
	}
	total, totalOK := values["MemTotal"]
	available, availableOK := values["MemAvailable"]
	if !totalOK || total <= 0 {
		return 0, 0, errors.New("MemTotal is missing")
	}
	if !availableOK || available < 0 {
		return 0, 0, errors.New("MemAvailable is missing")
	}
	return total, mathMin(total, available), nil
}

func parseLinuxRSS(raw string) (float64, error) {
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "VmRSS:" {
			value, err := strconv.ParseFloat(fields[1], 64)
			if err != nil || value < 0 {
				return 0, errors.New("VmRSS is invalid")
			}
			return value * 1024, nil
		}
	}
	return 0, errors.New("VmRSS is missing")
}

func sampleDarwinMemory(ctx context.Context) (telemetryMetric, telemetryMetric, memoryUsageSnapshot) {
	total, err := commandFloatContext(ctx, "sysctl", "-n", "hw.memsize")
	if err != nil || total <= 0 {
		if err == nil {
			err = errors.New("sysctl returned an invalid memory size")
		}
		metric := unavailableTelemetryMetric(telemetryCommandMetricStatus(err), err.Error())
		return metric, metric, memoryUsageSnapshot{Status: metric.Status, Reason: metric.Reason}
	}
	serverRSS, serverErr := commandFloatContext(ctx, "ps", "-p", strconv.Itoa(os.Getpid()), "-o", "rss=")
	serverMetric := unavailableTelemetryMetric(telemetryCommandMetricStatus(serverErr), serverErrString(serverErr, "process RSS is unavailable"))
	if serverErr == nil && serverRSS >= 0 {
		serverMetric = availableTelemetryMetric(serverRSS * 1024 / total * 100)
	}
	vmOutput, vmErr := runTelemetryCommand(ctx, "vm_stat")
	if vmErr != nil {
		return serverMetric, unavailableTelemetryMetric(telemetryCommandMetricStatus(vmErr), vmErr.Error()), memoryUsageSnapshot{Status: telemetryCommandMetricStatus(vmErr), Reason: vmErr.Error()}
	}
	usedBytes, parseErr := parseDarwinVMStat(vmOutput)
	if parseErr != nil {
		return serverMetric, unavailableTelemetryMetric(telemetryStatusParseError, parseErr.Error()), memoryUsageSnapshot{Status: telemetryStatusParseError, Reason: parseErr.Error()}
	}
	memory := boundedMemoryUsage(total, total-usedBytes)
	return serverMetric, availableMemoryMetric(memory), memory
}

func parseDarwinVMStat(raw string) (float64, error) {
	pageSize := 4096.0
	if marker := strings.Index(raw, "page size of"); marker >= 0 {
		fields := strings.Fields(raw[marker+len("page size of"):])
		if len(fields) > 0 {
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(fields[0]), 64); err == nil && parsed > 0 {
				pageSize = parsed
			}
		}
	}
	usedPages := 0.0
	found := false
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		valueText := strings.TrimSpace(strings.TrimSuffix(parts[1], "."))
		value, err := strconv.ParseFloat(valueText, 64)
		if err != nil || value < 0 {
			continue
		}
		switch strings.TrimSpace(parts[0]) {
		case "Pages active", "Pages inactive", "Pages wired down", "Pages occupied by compressor":
			usedPages += value
			found = true
		}
	}
	if !found {
		return 0, errors.New("vm_stat returned no recognized memory counters")
	}
	return mathMax(0, usedPages*pageSize), nil
}

func mathIsInvalidPercent(value float64) bool {
	return value < 0 || value > 100 || value != value
}

func mathMin(left, right float64) float64 {
	if right < left {
		return right
	}
	return left
}

func mathMax(left, right float64) float64 {
	if right > left {
		return right
	}
	return left
}
