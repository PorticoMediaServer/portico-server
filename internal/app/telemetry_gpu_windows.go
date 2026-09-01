//go:build windows

package app

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	pdhFormatDouble = 0x00000200
	pdhMoreData     = 0x800007D2
)

var (
	telemetryPDH                  = windows.NewLazySystemDLL("pdh.dll")
	telemetryPdhOpenQuery         = telemetryPDH.NewProc("PdhOpenQueryW")
	telemetryPdhCloseQuery        = telemetryPDH.NewProc("PdhCloseQuery")
	telemetryPdhAddEnglishCounter = telemetryPDH.NewProc("PdhAddEnglishCounterW")
	telemetryPdhCollectQueryData  = telemetryPDH.NewProc("PdhCollectQueryData")
	telemetryPdhGetFormattedArray = telemetryPDH.NewProc("PdhGetFormattedCounterArrayW")
)

func detectPlatformGPUInfo() DashboardGPUInfo {
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		name := firstCSVField(commandString("nvidia-smi", "--query-gpu=name", "--format=csv,noheader,nounits"))
		if name != "" {
			return DashboardGPUInfo{Provider: "NVIDIA", Device: name, Available: true, Note: "Collected through the NVIDIA display driver management interface."}
		}
	}
	if !windowsGPUCounterAvailable() {
		return DashboardGPUInfo{
			Provider: "Windows", Device: windowsGPUDeviceName(),
			Note: "Windows did not expose native GPU performance counters. This normally means no supported display driver or GPU device is available to the Portico service.",
		}
	}
	return DashboardGPUInfo{
		Provider: "Windows", Device: windowsGPUDeviceName(), Available: true,
		Note: "Collected from native Windows GPU performance counters. Adapter memory percentage and encoder-specific utilization are not exposed consistently by this interface.",
	}
}

func samplePlatformGPU(ctx context.Context, _ DashboardGPUInfo) telemetryGPUSample {
	values, err := collectWindowsGPUCounter(ctx, `\GPU Engine(*)\Utilization Percentage`)
	if err != nil {
		return unavailableGPUSample(telemetryCommandMetricStatus(err), err.Error())
	}
	usage := 0.0
	for _, value := range values {
		usage = math.Max(usage, value)
	}
	return telemetryGPUSample{
		Usage:   availableTelemetryMetric(usage),
		Memory:  unavailableTelemetryMetric(telemetryStatusUnsupported, "Windows GPU counters do not provide a reliable adapter-memory percentage"),
		Encoder: unavailableTelemetryMetric(telemetryStatusUnsupported, "Windows GPU counters do not provide portable encoder utilization"),
	}
}

func windowsGPUCounterAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	values, err := collectWindowsGPUCounter(ctx, `\GPU Engine(*)\Utilization Percentage`)
	return err == nil && len(values) > 0
}

func openWindowsGPUCounter(path string) (uintptr, uintptr, error) {
	var query uintptr
	if status, _, _ := telemetryPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&query))); status != 0 {
		return 0, 0, fmt.Errorf("PdhOpenQueryW failed with status 0x%x", status)
	}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		telemetryPdhCloseQuery.Call(query)
		return 0, 0, err
	}
	var counter uintptr
	if status, _, _ := telemetryPdhAddEnglishCounter.Call(query, uintptr(unsafe.Pointer(pathPtr)), 0, uintptr(unsafe.Pointer(&counter))); status != 0 {
		telemetryPdhCloseQuery.Call(query)
		return 0, 0, fmt.Errorf("PdhAddEnglishCounterW failed with status 0x%x", status)
	}
	return query, counter, nil
}

func collectWindowsGPUCounter(ctx context.Context, path string) ([]float64, error) {
	query, counter, err := openWindowsGPUCounter(path)
	if err != nil {
		return nil, err
	}
	defer telemetryPdhCloseQuery.Call(query)
	if status, _, _ := telemetryPdhCollectQueryData.Call(query); status != 0 {
		return nil, fmt.Errorf("PdhCollectQueryData failed with status 0x%x", status)
	}
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	if status, _, _ := telemetryPdhCollectQueryData.Call(query); status != 0 {
		return nil, fmt.Errorf("PdhCollectQueryData failed with status 0x%x", status)
	}
	var size, count uint32
	status, _, _ := telemetryPdhGetFormattedArray.Call(counter, pdhFormatDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if status != pdhMoreData || size == 0 || size > telemetryCommandOutputLimit {
		if status == 0 && count == 0 {
			return nil, errors.New("Windows GPU counters returned no adapter instances")
		}
		return nil, fmt.Errorf("PdhGetFormattedCounterArrayW sizing failed with status 0x%x", status)
	}
	buffer := make([]byte, size)
	if status, _, _ = telemetryPdhGetFormattedArray.Call(counter, pdhFormatDouble, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buffer[0]))); status != 0 {
		return nil, fmt.Errorf("PdhGetFormattedCounterArrayW failed with status 0x%x", status)
	}
	// PDH_FMT_COUNTERVALUE_ITEM places the counter value at offset 8 on all
	// supported Windows ABIs; the union's double is eight bytes after CStatus.
	const itemSize, statusOffset, valueOffset = 24, 8, 16
	if uint64(count)*itemSize > uint64(len(buffer)) {
		return nil, errors.New("Windows GPU counter array exceeded its buffer")
	}
	values := make([]float64, 0, count)
	for index := uint32(0); index < count; index++ {
		offset := int(index) * itemSize
		counterStatus := binary.LittleEndian.Uint32(buffer[offset+statusOffset:])
		value := math.Float64frombits(binary.LittleEndian.Uint64(buffer[offset+valueOffset:]))
		if (counterStatus == 0 || counterStatus == 1) && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil, errors.New("Windows GPU counters returned no valid samples")
	}
	return values, nil
}

func windowsGPUDeviceName() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return "Windows graphics adapter"
	}
	defer key.Close()
	names, _ := key.ReadSubKeyNames(32)
	for _, name := range names {
		child, err := registry.OpenKey(key, name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		device, _, valueErr := child.GetStringValue("DriverDesc")
		child.Close()
		if valueErr == nil && strings.TrimSpace(device) != "" {
			return strings.TrimSpace(device)
		}
	}
	return "Windows graphics adapter"
}
