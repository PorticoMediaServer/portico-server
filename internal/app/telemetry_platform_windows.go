//go:build windows

package app

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	telemetryKernel32             = windows.NewLazySystemDLL("kernel32.dll")
	telemetryGetSystemTimes       = telemetryKernel32.NewProc("GetSystemTimes")
	telemetryGlobalMemoryStatusEx = telemetryKernel32.NewProc("GlobalMemoryStatusEx")
	telemetryPSAPI                = windows.NewLazySystemDLL("psapi.dll")
	telemetryGetProcessMemoryInfo = telemetryPSAPI.NewProc("GetProcessMemoryInfo")
)

type windowsMemoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type windowsProcessMemoryCountersEx struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

func samplePlatformCPU(ctx context.Context, state *telemetrySamplerState) (telemetryMetric, telemetryMetric) {
	if err := ctx.Err(); err != nil {
		metric := unavailableTelemetryMetric(telemetryStatusCanceled, err.Error())
		return metric, metric
	}
	if state == nil {
		state = &telemetrySamplerState{}
	}
	var idle, kernel, user windows.Filetime
	if ok, _, callErr := telemetryGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	); ok == 0 {
		err := windowsCallError("GetSystemTimes", callErr)
		metric := unavailableTelemetryMetric(telemetryStatusUnavailable, err.Error())
		return metric, metric
	}
	var creation, exit, processKernel, processUser windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &creation, &exit, &processKernel, &processUser); err != nil {
		metric := unavailableTelemetryMetric(telemetryStatusUnavailable, "GetProcessTimes: "+err.Error())
		return metric, metric
	}
	systemKernel := windowsFiletimeTicks(kernel)
	systemUser := windowsFiletimeTicks(user)
	systemIdle := windowsFiletimeTicks(idle)
	if ^uint64(0)-systemKernel < systemUser {
		metric := unavailableTelemetryMetric(telemetryStatusUnavailable, "Windows CPU counters overflowed")
		return metric, metric
	}
	systemTotal := systemKernel + systemUser
	if systemIdle > systemTotal {
		metric := unavailableTelemetryMetric(telemetryStatusUnavailable, "Windows idle CPU counter exceeds total")
		return metric, metric
	}
	systemBusy := systemTotal - systemIdle
	processKernelTicks := windowsFiletimeTicks(processKernel)
	processUserTicks := windowsFiletimeTicks(processUser)
	if ^uint64(0)-processKernelTicks < processUserTicks {
		metric := unavailableTelemetryMetric(telemetryStatusUnavailable, "Windows process CPU counters overflowed")
		return metric, metric
	}
	processTotal := processKernelTicks + processUserTicks
	if !state.cpu.Initialized {
		state.cpu = telemetryCPUState{Initialized: true, SystemTotal: systemTotal, SystemBusy: systemBusy, ProcessTotal: processTotal}
		metric := unavailableTelemetryMetric(telemetryStatusWarmingUp, "Windows CPU counters require a previous sample")
		return metric, metric
	}
	if systemTotal <= state.cpu.SystemTotal || systemBusy < state.cpu.SystemBusy || processTotal < state.cpu.ProcessTotal {
		state.cpu = telemetryCPUState{Initialized: true, SystemTotal: systemTotal, SystemBusy: systemBusy, ProcessTotal: processTotal}
		metric := unavailableTelemetryMetric(telemetryStatusWarmingUp, "Windows CPU counters moved backwards or did not advance")
		return metric, metric
	}
	totalDelta := systemTotal - state.cpu.SystemTotal
	busyDelta := systemBusy - state.cpu.SystemBusy
	processDelta := processTotal - state.cpu.ProcessTotal
	state.cpu = telemetryCPUState{Initialized: true, SystemTotal: systemTotal, SystemBusy: systemBusy, ProcessTotal: processTotal}
	if totalDelta == 0 {
		metric := unavailableTelemetryMetric(telemetryStatusWarmingUp, "Windows CPU counter interval was empty")
		return metric, metric
	}
	return availableTelemetryMetric(float64(processDelta) / float64(totalDelta) * 100), availableTelemetryMetric(float64(busyDelta) / float64(totalDelta) * 100)
}

func samplePlatformMemory(ctx context.Context) (telemetryMetric, telemetryMetric, memoryUsageSnapshot) {
	if err := ctx.Err(); err != nil {
		metric := unavailableTelemetryMetric(telemetryStatusCanceled, err.Error())
		return metric, metric, memoryUsageSnapshot{Status: telemetryStatusCanceled, Reason: err.Error()}
	}
	memory, err := windowsMemoryUsage()
	if err != nil {
		metric := unavailableTelemetryMetric(telemetryStatusUnavailable, err.Error())
		return metric, metric, memoryUsageSnapshot{Status: telemetryStatusUnavailable, Reason: err.Error()}
	}
	serverBytes, serverErr := windowsProcessWorkingSet()
	serverMetric := unavailableTelemetryMetric(telemetryStatusUnavailable, serverErrString(serverErr, "Windows process working set is unavailable"))
	if serverErr == nil {
		serverMetric = availableTelemetryMetric(float64(serverBytes) / float64(memory.TotalBytes) * 100)
	}
	return serverMetric, availableMemoryMetric(memory), memory
}

func windowsMemoryUsage() (memoryUsageSnapshot, error) {
	status := windowsMemoryStatusEx{Length: uint32(unsafe.Sizeof(windowsMemoryStatusEx{}))}
	if ok, _, callErr := telemetryGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status))); ok == 0 {
		return memoryUsageSnapshot{}, windowsCallError("GlobalMemoryStatusEx", callErr)
	}
	if status.TotalPhys == 0 || status.AvailPhys > status.TotalPhys {
		return memoryUsageSnapshot{}, errors.New("GlobalMemoryStatusEx returned invalid physical memory values")
	}
	return boundedMemoryUsage(float64(status.TotalPhys), float64(status.AvailPhys)), nil
}

func windowsProcessWorkingSet() (uint64, error) {
	counters := windowsProcessMemoryCountersEx{CB: uint32(unsafe.Sizeof(windowsProcessMemoryCountersEx{}))}
	if ok, _, callErr := telemetryGetProcessMemoryInfo.Call(
		uintptr(windows.CurrentProcess()),
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.CB),
	); ok == 0 {
		return 0, windowsCallError("GetProcessMemoryInfo", callErr)
	}
	if counters.WorkingSetSize == 0 {
		return 0, errors.New("GetProcessMemoryInfo returned an empty working set")
	}
	return uint64(counters.WorkingSetSize), nil
}

func windowsFiletimeTicks(value windows.Filetime) uint64 {
	return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime)
}

func windowsCallError(name string, callErr error) error {
	if callErr == nil || callErr == windows.ERROR_SUCCESS {
		callErr = windows.GetLastError()
	}
	if callErr == nil || callErr == windows.ERROR_SUCCESS {
		return fmt.Errorf("%s failed", name)
	}
	return fmt.Errorf("%s: %w", name, callErr)
}
