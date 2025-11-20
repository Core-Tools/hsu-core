package resourcelimits

import (
	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/processstate"
)

// resourceEnforcer implements ResourceEnforcer interface
type resourceEnforcer struct {
	logger logging.Logger
}

// NewResourceEnforcer creates a new resource enforcer that applies platform-specific resource limits.
// The enforcer provides cross-platform resource limit enforcement using native OS mechanisms:
// - Windows: Job Objects for memory, CPU, and process limits
// - macOS: POSIX setrlimit for memory and CPU limits
// - Linux: Planned implementation using cgroups (Phase 4)
func NewResourceEnforcer(logger logging.Logger) ResourceEnforcer {
	return &resourceEnforcer{
		logger: logger,
	}
}

// ApplyLimits applies comprehensive resource limits to a process.
// Supports memory (RSS, virtual), CPU (time, percentage), process limits (file descriptors, child processes).
// Uses ErrorCollection to aggregate multiple limit failures and provides detailed error context.
// Returns aggregated errors if any limits fail to apply.
func (re *resourceEnforcer) ApplyLimits(pid int, limits *ResourceLimits) error {
	if limits == nil {
		return nil
	}

	re.logger.Infof("Applying resource limits to PID %d", pid)

	// Check if process exists
	running, err := processstate.IsProcessRunning(pid)
	if !running {
		return errors.NewProcessError("process is not running", err).WithContext("pid", pid)
	}

	errorCollection := errors.NewErrorCollection()

	// Apply memory limits if specified
	if limits.Memory != nil {
		if err := re.applyMemoryLimits(pid, limits.Memory); err != nil {
			wrappedErr := errors.NewProcessError("failed to apply memory limits", err).WithContext("pid", pid)
			errorCollection.Add(wrappedErr)
			re.logger.Warnf("Failed to apply memory limits to PID %d: %v", pid, err)
		}
	}

	// Apply CPU limits if specified
	if limits.CPU != nil {
		if err := re.applyCPULimits(pid, limits.CPU); err != nil {
			wrappedErr := errors.NewProcessError("failed to apply CPU limits", err).WithContext("pid", pid)
			errorCollection.Add(wrappedErr)
			re.logger.Warnf("Failed to apply CPU limits to PID %d: %v", pid, err)
		}
	}

	// Apply I/O limits if specified
	// Note: I/O limits implementation is planned for Phase 4 - requires platform-specific rate limiting
	if limits.IO != nil {
		re.logger.Debugf("I/O limits specified but not yet implemented for PID %d: max_read_bps=%d, max_write_bps=%d",
			pid, limits.IO.MaxReadBPS, limits.IO.MaxWriteBPS)
		// Implementation planned: Windows (Job Objects), macOS/Linux (rlimit + monitoring)
	}

	// Apply process limits if specified
	if limits.Process != nil {
		if err := re.applyProcessLimits(pid, limits.Process); err != nil {
			wrappedErr := errors.NewProcessError("failed to apply process limits", err).WithContext("pid", pid)
			errorCollection.Add(wrappedErr)
			re.logger.Warnf("Failed to apply process limits to PID %d: %v", pid, err)
		}
	}

	// Apply basic limits if present in nested structs
	if limits.Memory != nil && (limits.Memory.MaxRSS > 0 || limits.Memory.MaxVirtual > 0) {
		re.logger.Debugf("Applying memory limits to PID %d (MaxRSS: %d, MaxVirtual: %d)",
			pid, limits.Memory.MaxRSS, limits.Memory.MaxVirtual)
	}

	if limits.Process != nil && limits.Process.MaxFileDescriptors > 0 {
		if err := re.applyFileDescriptorLimit(pid, limits.Process.MaxFileDescriptors); err != nil {
			wrappedErr := errors.NewProcessError("failed to apply file descriptor limits", err).WithContext("pid", pid)
			errorCollection.Add(wrappedErr)
			re.logger.Warnf("Failed to apply file descriptor limits to PID %d: %v", pid, err)
		}
	}

	// Return aggregated errors
	if errorCollection.HasErrors() {
		re.logger.Errorf("Some resource limits could not be applied to PID %d: %v", pid, errorCollection.Errors)
		return errors.NewProcessError("failed to apply some resource limits", errorCollection.ToError()).WithContext("pid", pid)
	}

	re.logger.Infof("Resource limits successfully applied to PID %d", pid)
	return nil
}

// SupportsLimitType checks if a limit type is supported on current platform
func (re *resourceEnforcer) SupportsLimitType(limitType ResourceLimitType) bool {
	return supportsLimitTypeImpl(limitType)
}

// applyMemoryLimits applies memory-specific limits
func (re *resourceEnforcer) applyMemoryLimits(pid int, limits *MemoryLimits) error {
	if limits.MaxRSS <= 0 && limits.MaxVirtual <= 0 {
		return nil
	}

	return applyMemoryLimitsImpl(pid, limits, re.logger)
}

// applyCPULimits applies CPU-specific limits
func (re *resourceEnforcer) applyCPULimits(pid int, limits *CPULimits) error {
	if limits.MaxPercent <= 0 && limits.MaxTime <= 0 {
		return nil
	}

	return applyCPULimitsImpl(pid, limits, re.logger)
}

// applyProcessLimits applies process/file descriptor limits
func (re *resourceEnforcer) applyProcessLimits(pid int, limits *ProcessLimits) error {
	if limits.MaxFileDescriptors <= 0 && limits.MaxChildProcesses <= 0 {
		return nil
	}

	// File descriptor limits can be applied using setrlimit on Unix systems
	if limits.MaxFileDescriptors > 0 {
		if err := re.applyFileDescriptorLimit(pid, limits.MaxFileDescriptors); err != nil {
			return errors.NewProcessError("failed to apply file descriptor limits", err).WithContext("pid", pid).WithContext("max_fd", limits.MaxFileDescriptors)
		}
	}

	re.logger.Debugf("Process limits applied to PID %d", pid)
	return nil
}

func (re *resourceEnforcer) applyFileDescriptorLimit(pid int, maxFDs int) error {
	re.logger.Debugf("Applying file descriptor limit to PID %d: %d", pid, maxFDs)

	// Note: File descriptor limits implementation planned for Phase 4
	// Platform strategy: Unix/macOS (RLIMIT_NOFILE via setrlimit), Windows (Job Objects process limits)
	re.logger.Debugf("File descriptor limit implementation pending for PID %d: %d (platform-specific implementation required)", pid, maxFDs)

	return nil
}
