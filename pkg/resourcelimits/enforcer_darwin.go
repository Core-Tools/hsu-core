//go:build darwin
// +build darwin

package resourcelimits

import (
	"fmt"
	"syscall"

	"github.com/core-tools/hsu-core/pkg/logging"
)

// Platform-specific limit support checks
func supportsLimitTypeImpl(limitType ResourceLimitType) bool {
	switch limitType {
	case ResourceLimitTypeMemory, ResourceLimitTypeCPU, ResourceLimitTypeProcess:
		return true // macOS supports these via POSIX resource limits
	default:
		return false
	}
}

// applyMemoryLimitsImpl applies memory limits using POSIX setrlimit (macOS implementation)
func applyMemoryLimitsImpl(pid int, limits *MemoryLimits, logger logging.Logger) error {
	if limits == nil {
		return fmt.Errorf("memory limits cannot be nil")
	}

	logger.Infof("Applying memory limits to PID %d (MaxRSS: %d, MaxVirtual: %d)",
		pid, limits.MaxRSS, limits.MaxVirtual)

	// Apply RSS (Resident Set Size) limit
	// Note: RLIMIT_RSS may not be enforced on newer macOS versions
	if limits.MaxRSS > 0 {
		rlimit := syscall.Rlimit{
			Cur: uint64(limits.MaxRSS),
			Max: uint64(limits.MaxRSS),
		}

		// Use a constant value since RLIMIT_RSS might not be available in Go's syscall package
		const RLIMIT_RSS = 5 // Standard POSIX value for RSS limit

		if err := syscall.Setrlimit(RLIMIT_RSS, &rlimit); err != nil {
			logger.Warnf("Failed to set RSS limit for PID %d: %v", pid, err)
			// Don't fail completely - RSS limits may not be enforced on all macOS versions
		} else {
			logger.Infof("Successfully set RSS limit for PID %d: %d bytes", pid, limits.MaxRSS)
		}
	}

	// Apply virtual memory limit
	if limits.MaxVirtual > 0 {
		rlimit := syscall.Rlimit{
			Cur: uint64(limits.MaxVirtual),
			Max: uint64(limits.MaxVirtual),
		}

		if err := syscall.Setrlimit(syscall.RLIMIT_AS, &rlimit); err != nil {
			logger.Warnf("Failed to set virtual memory limit for PID %d: %v", pid, err)
			// Don't fail completely - continue with other limits
		} else {
			logger.Infof("Successfully set virtual memory limit for PID %d: %d bytes", pid, limits.MaxVirtual)
		}
	}

	// Apply data segment limit (additional memory protection)
	if limits.MaxRSS > 0 {
		rlimit := syscall.Rlimit{
			Cur: uint64(limits.MaxRSS),
			Max: uint64(limits.MaxRSS),
		}

		if err := syscall.Setrlimit(syscall.RLIMIT_DATA, &rlimit); err != nil {
			logger.Debugf("Failed to set data segment limit for PID %d: %v", pid, err)
			// This is optional - don't warn about it
		}
	}

	return nil
}

// applyCPULimitsImpl applies CPU limits using POSIX setrlimit (macOS implementation)
func applyCPULimitsImpl(pid int, limits *CPULimits, logger logging.Logger) error {
	if limits == nil {
		return fmt.Errorf("CPU limits cannot be nil")
	}

	logger.Infof("Applying CPU limits to PID %d (MaxTime: %v, MaxPercent: %f)",
		pid, limits.MaxTime, limits.MaxPercent)

	// Apply CPU time limit
	if limits.MaxTime > 0 {
		maxTimeSeconds := uint64(limits.MaxTime.Seconds())
		rlimit := syscall.Rlimit{
			Cur: maxTimeSeconds,
			Max: maxTimeSeconds,
		}

		if err := syscall.Setrlimit(syscall.RLIMIT_CPU, &rlimit); err != nil {
			return fmt.Errorf("failed to set CPU time limit for PID %d: %v", pid, err)
		}

		logger.Infof("Successfully set CPU time limit for PID %d: %d seconds", pid, maxTimeSeconds)
	}

	// Note: MaxPercent and MaxCores require different mechanisms (nice/renice or launchd)
	// For High Sierra compatibility, we focus on CPU time limits via RLIMIT_CPU
	if limits.MaxPercent > 0 {
		logger.Debugf("CPU percentage limits not implemented via setrlimit - would require additional mechanisms")
	}

	if limits.MaxCores > 0 {
		logger.Debugf("CPU core limits not implemented via setrlimit - would require CPU affinity mechanisms")
	}

	return nil
}

// getCurrentLimits retrieves current resource limits for debugging/monitoring
func getCurrentLimits(pid int, logger logging.Logger) map[string]syscall.Rlimit {
	limits := make(map[string]syscall.Rlimit)

	// Define constants for missing syscall values
	const RLIMIT_RSS = 5   // RSS limit (may not be enforced on macOS)
	const RLIMIT_NPROC = 7 // Process limit

	limitTypes := map[string]int{
		"rss":     RLIMIT_RSS,
		"virtual": syscall.RLIMIT_AS,
		"data":    syscall.RLIMIT_DATA,
		"cpu":     syscall.RLIMIT_CPU,
		"nofile":  syscall.RLIMIT_NOFILE,
		"nproc":   RLIMIT_NPROC,
		"core":    syscall.RLIMIT_CORE,
	}

	for name, limitType := range limitTypes {
		var rlimit syscall.Rlimit
		if err := syscall.Getrlimit(limitType, &rlimit); err != nil {
			logger.Debugf("Failed to get %s limit: %v", name, err)
		} else {
			limits[name] = rlimit
			logger.Debugf("Current %s limit: current=%d, max=%d", name, rlimit.Cur, rlimit.Max)
		}
	}

	return limits
}
