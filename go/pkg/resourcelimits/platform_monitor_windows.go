//go:build windows
// +build windows

package resourcelimits

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/core-tools/hsu-core/pkg/logging"
)

// Windows constants not in standard syscall package
const (
	PROCESS_VM_READ = 0x0010
)

// Windows-specific implementation using Win32 APIs
type windowsResourceMonitor struct {
	logger logging.Logger

	// Windows handles and function pointers
	kernel32              *syscall.LazyDLL
	psapi                 *syscall.LazyDLL
	getProcessMemoryInfo  *syscall.LazyProc
	getProcessTimes       *syscall.LazyProc
	getProcessHandleCount *syscall.LazyProc
}

// Windows API structures
type processMemoryCounters struct {
	Size                       uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

type fileTime struct {
	LowDateTime  uint32
	HighDateTime uint32
}

func newWindowsResourceMonitor(logger logging.Logger) PlatformResourceMonitor {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	psapi := syscall.NewLazyDLL("psapi.dll")

	return &windowsResourceMonitor{
		logger:                logger,
		kernel32:              kernel32,
		psapi:                 psapi,
		getProcessMemoryInfo:  psapi.NewProc("GetProcessMemoryInfo"),
		getProcessTimes:       kernel32.NewProc("GetProcessTimes"),
		getProcessHandleCount: kernel32.NewProc("GetProcessHandleCount"),
	}
}

func (w *windowsResourceMonitor) GetProcessUsage(pid int) (*ResourceUsage, error) {
	// Open process handle
	handle, err := syscall.OpenProcess(
		syscall.PROCESS_QUERY_INFORMATION|PROCESS_VM_READ,
		false,
		uint32(pid),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open process %d: %v", pid, err)
	}
	defer syscall.CloseHandle(handle)

	usage := &ResourceUsage{
		Timestamp: time.Now(),
	}

	// Get memory information
	if err := w.getMemoryUsage(handle, usage); err != nil {
		w.logger.Debugf("Failed to get memory usage for PID %d: %v", pid, err)
	}

	// Get CPU information
	if err := w.getCPUUsage(handle, usage); err != nil {
		w.logger.Debugf("Failed to get CPU usage for PID %d: %v", pid, err)
	}

	// Get handle count (file descriptors equivalent)
	if err := w.getHandleCount(handle, usage); err != nil {
		w.logger.Debugf("Failed to get handle count for PID %d: %v", pid, err)
	}

	// Get I/O information
	if err := w.getIOUsage(handle, usage); err != nil {
		w.logger.Debugf("Failed to get I/O usage for PID %d: %v", pid, err)
	}

	w.logger.Debugf("Windows resource usage for PID %d: Memory RSS: %dMB, CPU: %.1f%%, Handles: %d",
		pid, usage.MemoryRSS/(1024*1024), usage.CPUPercent, usage.OpenFileDescriptors)

	return usage, nil
}

func (w *windowsResourceMonitor) getMemoryUsage(handle syscall.Handle, usage *ResourceUsage) error {
	var pmc processMemoryCounters
	pmc.Size = uint32(unsafe.Sizeof(pmc))

	ret, _, err := w.getProcessMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&pmc)),
		uintptr(pmc.Size),
	)

	if ret == 0 {
		return fmt.Errorf("GetProcessMemoryInfo failed: %v", err)
	}

	usage.MemoryRSS = int64(pmc.WorkingSetSize)
	usage.MemoryVirtual = int64(pmc.PagefileUsage)

	// Calculate memory percentage (approximate)
	// This would ideally use GlobalMemoryStatusEx to get total system memory
	usage.MemoryPercent = 0.0 // Note: System memory percentage calculation planned for Phase 4

	return nil
}

func (w *windowsResourceMonitor) getCPUUsage(handle syscall.Handle, usage *ResourceUsage) error {
	var creationTime, exitTime, kernelTime, userTime fileTime

	ret, _, err := w.getProcessTimes.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&creationTime)),
		uintptr(unsafe.Pointer(&exitTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)

	if ret == 0 {
		return fmt.Errorf("GetProcessTimes failed: %v", err)
	}

	// Convert FILETIME to seconds
	kernelSeconds := fileTimeToSeconds(kernelTime)
	userSeconds := fileTimeToSeconds(userTime)
	usage.CPUTime = kernelSeconds + userSeconds

	// For CPU percentage, we'd need to track this over time
	// For now, set it to 0 - real implementation would need previous measurement
	usage.CPUPercent = 0.0 // Note: CPU percentage calculation requires process time tracking - planned for Phase 4

	return nil
}

func (w *windowsResourceMonitor) getHandleCount(handle syscall.Handle, usage *ResourceUsage) error {
	var handleCount uint32

	ret, _, err := w.getProcessHandleCount.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&handleCount)),
	)

	if ret == 0 {
		return fmt.Errorf("GetProcessHandleCount failed: %v", err)
	}

	usage.OpenFileDescriptors = int(handleCount)
	return nil
}

func (w *windowsResourceMonitor) getIOUsage(handle syscall.Handle, usage *ResourceUsage) error {
	// Note: I/O usage implementation planned for Phase 4 using GetProcessIoCounters API
	// For now, set default values
	usage.IOReadBytes = 0
	usage.IOWriteBytes = 0
	usage.IOReadOps = 0
	usage.IOWriteOps = 0
	return nil
}

func (w *windowsResourceMonitor) SupportsRealTimeMonitoring() bool {
	return true
}

// Helper function to convert Windows FILETIME to seconds
func fileTimeToSeconds(ft fileTime) float64 {
	// FILETIME is in 100-nanosecond intervals since January 1, 1601
	total := (int64(ft.HighDateTime) << 32) | int64(ft.LowDateTime)
	return float64(total) / 10000000.0 // Convert to seconds
}

// Enhanced Windows monitor with Performance Counters (for CPU percentage)
type windowsPerformanceMonitor struct {
	*windowsResourceMonitor

	// Previous measurements for calculating rates
	lastCPUTime     float64
	lastMeasurement time.Time

	// Performance counter handles (would be used for real-time CPU%)
	// Note: PDH (Performance Data Helper) integration planned for Phase 4 for advanced metrics
}

func newWindowsPerformanceMonitor(logger logging.Logger) PlatformResourceMonitor {
	base := newWindowsResourceMonitor(logger).(*windowsResourceMonitor)

	return &windowsPerformanceMonitor{
		windowsResourceMonitor: base,
		lastMeasurement:        time.Now(),
	}
}

func (w *windowsPerformanceMonitor) GetProcessUsage(pid int) (*ResourceUsage, error) {
	usage, err := w.windowsResourceMonitor.GetProcessUsage(pid)
	if err != nil {
		return nil, err
	}

	// Calculate CPU percentage based on time difference
	now := time.Now()
	if !w.lastMeasurement.IsZero() {
		timeDiff := now.Sub(w.lastMeasurement).Seconds()
		cpuTimeDiff := usage.CPUTime - w.lastCPUTime

		if timeDiff > 0 {
			// CPU percentage = (CPU time used / elapsed time) * 100
			usage.CPUPercent = (cpuTimeDiff / timeDiff) * 100

			// Cap at reasonable values
			if usage.CPUPercent > 100 {
				usage.CPUPercent = 100
			}
			if usage.CPUPercent < 0 {
				usage.CPUPercent = 0
			}
		}
	}

	w.lastCPUTime = usage.CPUTime
	w.lastMeasurement = now

	return usage, nil
}

// createPlatformSpecificMonitor creates Windows-specific monitor
func createPlatformSpecificMonitor(logger logging.Logger) PlatformResourceMonitor {
	return newWindowsPerformanceMonitor(logger)
}
