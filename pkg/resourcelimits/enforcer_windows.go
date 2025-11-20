//go:build windows
// +build windows

package resourcelimits

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/core-tools/hsu-procman-go/pkg/logging"
)

// Platform-specific limit support checks
func supportsLimitTypeImpl(limitType ResourceLimitType) bool {
	switch limitType {
	case ResourceLimitTypeMemory, ResourceLimitTypeCPU:
		return true // Windows supports these via Job Objects
	case ResourceLimitTypeProcess:
		return true // Partial support
	default:
		return false
	}
}

// Windows Job Object constants
const (
	JOB_OBJECT_LIMIT_WORKINGSET                 = 0x00000001
	JOB_OBJECT_LIMIT_PROCESS_TIME               = 0x00000002
	JOB_OBJECT_LIMIT_JOB_TIME                   = 0x00000004
	JOB_OBJECT_LIMIT_ACTIVE_PROCESS             = 0x00000008
	JOB_OBJECT_LIMIT_AFFINITY                   = 0x00000010
	JOB_OBJECT_LIMIT_PRIORITY_CLASS             = 0x00000020
	JOB_OBJECT_LIMIT_PRESERVE_JOB_TIME          = 0x00000040
	JOB_OBJECT_LIMIT_SCHEDULING_CLASS           = 0x00000080
	JOB_OBJECT_LIMIT_PROCESS_MEMORY             = 0x00000100
	JOB_OBJECT_LIMIT_JOB_MEMORY                 = 0x00000200
	JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION = 0x00000400
	JOB_OBJECT_LIMIT_BREAKAWAY_OK               = 0x00000800
	JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK        = 0x00001000
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE          = 0x00002000
	JOB_OBJECT_LIMIT_SUBSET_AFFINITY            = 0x00004000

	JobObjectBasicLimitInformation    = 2
	JobObjectExtendedLimitInformation = 9

	// Process access rights
	PROCESS_SET_QUOTA = 0x0100
)

// Windows Job Object structures
type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// windowsResourceEnforcer provides Windows-specific enforcement using Job Objects
type windowsResourceEnforcer struct {
	logger   logging.Logger
	kernel32 *syscall.LazyDLL

	// Job Object API functions
	createJobObject           *syscall.LazyProc
	assignProcessToJob        *syscall.LazyProc
	setInformationJobObject   *syscall.LazyProc
	queryInformationJobObject *syscall.LazyProc

	// Process-to-Job mapping
	processJobs map[int]syscall.Handle
}

// newWindowsResourceEnforcer creates a Windows-specific resource enforcer
func newWindowsResourceEnforcer(logger logging.Logger) *windowsResourceEnforcer {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")

	return &windowsResourceEnforcer{
		logger:                    logger,
		kernel32:                  kernel32,
		createJobObject:           kernel32.NewProc("CreateJobObjectW"),
		assignProcessToJob:        kernel32.NewProc("AssignProcessToJobObject"),
		setInformationJobObject:   kernel32.NewProc("SetInformationJobObject"),
		queryInformationJobObject: kernel32.NewProc("QueryInformationJobObject"),
		processJobs:               make(map[int]syscall.Handle),
	}
}

// applyMemoryLimitsImpl applies memory limits using Job Objects (implementation)
func applyMemoryLimitsImpl(pid int, limits *MemoryLimits, logger logging.Logger) error {
	holdsPrivileges, err := currentProcessHoldsPrivilegesForMemoryLimits()
	if err != nil {
		return fmt.Errorf("error checking privileges: %v", err)
	}
	if !holdsPrivileges {
		return fmt.Errorf("current process does not hold privileges for memory limits")
	}

	enforcer := newWindowsResourceEnforcer(logger)

	// Create or get existing job object
	jobHandle, err := enforcer.getOrCreateJobObject(pid)
	if err != nil {
		return fmt.Errorf("failed to create job object: %v", err)
	}

	// Set memory limits
	var extendedInfo jobObjectExtendedLimitInformation

	// Set working set limits (RSS)
	if limits.MaxRSS > 0 {
		extendedInfo.BasicLimitInformation.LimitFlags |= JOB_OBJECT_LIMIT_WORKINGSET
		// https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-jobobject_basic_limit_information
		// states that:
		//   > If MaximumWorkingSetSize is nonzero, MinimumWorkingSetSize cannot be zero.
		// Thus, use some reasonably small default value.
		var defaultMinWorkingSetSize int64 = 4 * 1024 * 1024 // 4MB
		if limits.MaxRSS < defaultMinWorkingSetSize {
			return fmt.Errorf("MaxRSS is too low (%d bytes), must be greater than %d bytes (hard-coded default minimum)",
				limits.MaxRSS, defaultMinWorkingSetSize)
		}
		extendedInfo.BasicLimitInformation.MinimumWorkingSetSize = uintptr(defaultMinWorkingSetSize)
		extendedInfo.BasicLimitInformation.MaximumWorkingSetSize = uintptr(limits.MaxRSS)

		logger.Infof("Setting working set limit for PID %d: %d bytes", pid, limits.MaxRSS)
	}

	// Set process memory limit (virtual memory)
	if limits.MaxVirtual > 0 {
		extendedInfo.BasicLimitInformation.LimitFlags |= JOB_OBJECT_LIMIT_PROCESS_MEMORY
		extendedInfo.ProcessMemoryLimit = uintptr(limits.MaxVirtual)

		logger.Infof("Setting process memory limit for PID %d: %d bytes", pid, limits.MaxVirtual)
	}

	// Apply the limits
	ret, _, err := enforcer.setInformationJobObject.Call(
		uintptr(jobHandle),
		JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&extendedInfo)),
		unsafe.Sizeof(extendedInfo),
	)

	if ret == 0 {
		return fmt.Errorf("SetInformationJobObject failed: %v", err)
	}

	logger.Infof("Successfully applied memory limits to PID %d", pid)
	return nil
}

// applyCPULimitsImpl applies CPU limits using Job Objects (implementation)
func applyCPULimitsImpl(pid int, limits *CPULimits, logger logging.Logger) error {
	holdsPrivileges, err := currentProcessHoldsPrivilegesForMemoryLimits()
	if err != nil {
		return fmt.Errorf("error checking privileges: %v", err)
	}
	if !holdsPrivileges {
		return fmt.Errorf("current process does not hold privileges for CPU limits")
	}

	enforcer := newWindowsResourceEnforcer(logger)

	// Create or get existing job object
	jobHandle, err := enforcer.getOrCreateJobObject(pid)
	if err != nil {
		return fmt.Errorf("failed to create job object: %v", err)
	}

	var basicInfo jobObjectBasicLimitInformation

	// Set CPU time limit
	if limits.MaxTime > 0 {
		basicInfo.LimitFlags |= JOB_OBJECT_LIMIT_PROCESS_TIME
		// Convert to 100-nanosecond intervals (Windows FILETIME units)
		basicInfo.PerProcessUserTimeLimit = int64(limits.MaxTime.Nanoseconds() / 100)

		logger.Infof("Setting CPU time limit for PID %d: %v", pid, limits.MaxTime)
	}

	// NOTE: Windows Job Objects don't directly support CPU percentage limits
	// For CPU percentage limiting, we'd need to use:
	// 1. CPU Rate Control (SetInformationJobObject with JobObjectCpuRateControlInformation)
	// 2. Or implement a separate monitoring/throttling mechanism

	if limits.MaxPercent > 0 {
		logger.Warnf("CPU percentage limits not directly supported via Job Objects for PID %d, consider using monitoring + throttling", pid)
		// Note: CPU rate control implementation planned for Phase 4
		// Strategy: Monitoring-based throttling or Windows Job Object rate limiting extensions
	}

	// Apply the limits
	ret, _, err := enforcer.setInformationJobObject.Call(
		uintptr(jobHandle),
		JobObjectBasicLimitInformation,
		uintptr(unsafe.Pointer(&basicInfo)),
		unsafe.Sizeof(basicInfo),
	)

	if ret == 0 {
		return fmt.Errorf("SetInformationJobObject failed: %v", err)
	}

	logger.Infof("Successfully applied CPU limits to PID %d", pid)
	return nil
}

// getOrCreateJobObject gets existing job or creates new one for process
func (we *windowsResourceEnforcer) getOrCreateJobObject(pid int) (syscall.Handle, error) {
	// Check if we already have a job object for this process
	if jobHandle, exists := we.processJobs[pid]; exists {
		return jobHandle, nil
	}

	// Create new job object
	ret, _, err := we.createJobObject.Call(
		0, // NULL security attributes
		0, // Anonymous job object (no name)
	)

	if ret == 0 {
		return 0, fmt.Errorf("CreateJobObject failed: %v", err)
	}

	jobHandle := syscall.Handle(ret)

	// Open process handle
	procHandle, err := syscall.OpenProcess(
		PROCESS_SET_QUOTA|syscall.PROCESS_TERMINATE,
		false,
		uint32(pid),
	)
	if err != nil {
		syscall.CloseHandle(jobHandle)
		return 0, fmt.Errorf("failed to open process %d: %v", pid, err)
	}
	defer syscall.CloseHandle(procHandle)

	// Assign process to job object
	ret, _, err = we.assignProcessToJob.Call(
		uintptr(jobHandle),
		uintptr(procHandle),
	)

	if ret == 0 {
		syscall.CloseHandle(jobHandle)
		return 0, fmt.Errorf("AssignProcessToJobObject failed: %v", err)
	}

	// Store the mapping
	we.processJobs[pid] = jobHandle

	we.logger.Infof("Created and assigned job object for PID %d", pid)
	return jobHandle, nil
}

// cleanupJobObject removes job object for process
func (we *windowsResourceEnforcer) cleanupJobObject(pid int) {
	if jobHandle, exists := we.processJobs[pid]; exists {
		syscall.CloseHandle(jobHandle)
		delete(we.processJobs, pid)
		we.logger.Debugf("Cleaned up job object for PID %d", pid)
	}
}
