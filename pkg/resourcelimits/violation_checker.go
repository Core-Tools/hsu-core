package resourcelimits

import (
	"fmt"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/logging"
)

type resourceViolationChecker struct {
	logger logging.Logger
}

func NewResourceViolationChecker(logger logging.Logger) ResourceViolationChecker {
	return &resourceViolationChecker{
		logger: logger,
	}
}

// CheckViolations checks for resource limit violations
func (rv *resourceViolationChecker) CheckViolations(usage *ResourceUsage, limits *ResourceLimits) []*ResourceViolation {
	if usage == nil || limits == nil {
		return nil
	}

	urv := &usageViolationsChecker{
		timestamp: time.Now(),
		usage:     usage,
	}

	var violations []*ResourceViolation

	// Check memory violations
	if limits.Memory != nil {
		violations = append(violations, urv.checkMemoryViolations(limits.Memory)...)
	}

	// Check CPU violations
	if limits.CPU != nil {
		violations = append(violations, urv.checkCPUViolations(limits.CPU)...)
	}

	// Check I/O violations
	if limits.IO != nil {
		violations = append(violations, urv.checkIOViolations(limits.IO)...)
	}

	// Check process violations
	if limits.Process != nil {
		violations = append(violations, urv.checkProcessViolations(limits.Process)...)
	}

	return violations
}

type usageViolationsChecker struct {
	timestamp time.Time
	usage     *ResourceUsage
}

// checkMemoryViolations checks for memory limit violations
func (urv *usageViolationsChecker) checkMemoryViolations(limits *MemoryLimits) []*ResourceViolation {
	var violations []*ResourceViolation

	// Check RSS limit
	if limits.MaxRSS > 0 && urv.usage.MemoryRSS > limits.MaxRSS {
		violations = append(violations, &ResourceViolation{
			LimitType:    ResourceLimitTypeMemory,
			CurrentValue: urv.usage.MemoryRSS,
			LimitValue:   limits.MaxRSS,
			Severity:     ViolationSeverityCritical,
			Timestamp:    urv.timestamp,
			Message:      fmt.Sprintf("Memory RSS (%d bytes) exceeds limit (%d bytes)", urv.usage.MemoryRSS, limits.MaxRSS),
		})
	}

	// Check virtual memory limit
	if limits.MaxVirtual > 0 && urv.usage.MemoryVirtual > limits.MaxVirtual {
		violations = append(violations, &ResourceViolation{
			LimitType:    ResourceLimitTypeMemory,
			CurrentValue: urv.usage.MemoryVirtual,
			LimitValue:   limits.MaxVirtual,
			Severity:     ViolationSeverityCritical,
			Timestamp:    urv.timestamp,
			Message:      fmt.Sprintf("Virtual memory (%d bytes) exceeds limit (%d bytes)", urv.usage.MemoryVirtual, limits.MaxVirtual),
		})
	}

	// Check warning threshold for RSS
	if limits.WarningThreshold > 0 && limits.MaxRSS > 0 {
		warningLimit := float64(limits.MaxRSS) * (limits.WarningThreshold / 100.0)
		if float64(urv.usage.MemoryRSS) > warningLimit {
			violations = append(violations, &ResourceViolation{
				LimitType:    ResourceLimitTypeMemory,
				CurrentValue: urv.usage.MemoryRSS,
				LimitValue:   int64(warningLimit),
				Severity:     ViolationSeverityWarning,
				Timestamp:    urv.timestamp,
				Message:      fmt.Sprintf("Memory RSS (%d bytes) exceeds warning threshold (%.0f bytes)", urv.usage.MemoryRSS, warningLimit),
			})
		}
	}

	return violations
}

// checkCPUViolations checks for CPU limit violations
func (urv *usageViolationsChecker) checkCPUViolations(limits *CPULimits) []*ResourceViolation {
	var violations []*ResourceViolation

	// Check CPU percentage limit
	if limits.MaxPercent > 0 && urv.usage.CPUPercent > limits.MaxPercent {
		violations = append(violations, &ResourceViolation{
			LimitType:    ResourceLimitTypeCPU,
			CurrentValue: urv.usage.CPUPercent,
			LimitValue:   limits.MaxPercent,
			Severity:     ViolationSeverityCritical,
			Timestamp:    urv.timestamp,
			Message:      fmt.Sprintf("CPU usage (%.1f%%) exceeds limit (%.1f%%)", urv.usage.CPUPercent, limits.MaxPercent),
		})
	}

	// Check CPU time limit
	if limits.MaxTime > 0 && time.Duration(urv.usage.CPUTime)*time.Second > limits.MaxTime {
		violations = append(violations, &ResourceViolation{
			LimitType:    ResourceLimitTypeCPU,
			CurrentValue: urv.usage.CPUTime,
			LimitValue:   limits.MaxTime.Seconds(),
			Severity:     ViolationSeverityCritical,
			Timestamp:    urv.timestamp,
			Message:      fmt.Sprintf("CPU time (%.1fs) exceeds limit (%v)", urv.usage.CPUTime, limits.MaxTime),
		})
	}

	// Check warning threshold
	if limits.WarningThreshold > 0 && limits.MaxPercent > 0 {
		warningLimit := limits.MaxPercent * (limits.WarningThreshold / 100.0)
		if urv.usage.CPUPercent > warningLimit {
			violations = append(violations, &ResourceViolation{
				LimitType:    ResourceLimitTypeCPU,
				CurrentValue: urv.usage.CPUPercent,
				LimitValue:   warningLimit,
				Severity:     ViolationSeverityWarning,
				Timestamp:    urv.timestamp,
				Message:      fmt.Sprintf("CPU usage (%.1f%%) exceeds warning threshold (%.1f%%)", urv.usage.CPUPercent, warningLimit),
			})
		}
	}

	return violations
}

// checkIOViolations checks for I/O limit violations
func (urv *usageViolationsChecker) checkIOViolations(limits *IOLimits) []*ResourceViolation {
	var violations []*ResourceViolation
	// TODO: Implement I/O rate checking (requires tracking rates over time)
	return violations
}

// checkProcessViolations checks for process/file descriptor violations
func (urv *usageViolationsChecker) checkProcessViolations(limits *ProcessLimits) []*ResourceViolation {
	var violations []*ResourceViolation

	// Check file descriptor limit
	if limits.MaxFileDescriptors > 0 && urv.usage.OpenFileDescriptors > limits.MaxFileDescriptors {
		violations = append(violations, &ResourceViolation{
			LimitType:    ResourceLimitTypeProcess,
			CurrentValue: urv.usage.OpenFileDescriptors,
			LimitValue:   limits.MaxFileDescriptors,
			Severity:     ViolationSeverityCritical,
			Timestamp:    urv.timestamp,
			Message:      fmt.Sprintf("Open file descriptors (%d) exceeds limit (%d)", urv.usage.OpenFileDescriptors, limits.MaxFileDescriptors),
		})
	}

	// Check child process limit
	if limits.MaxChildProcesses > 0 && urv.usage.ChildProcesses > limits.MaxChildProcesses {
		violations = append(violations, &ResourceViolation{
			LimitType:    ResourceLimitTypeProcess,
			CurrentValue: urv.usage.ChildProcesses,
			LimitValue:   limits.MaxChildProcesses,
			Severity:     ViolationSeverityCritical,
			Timestamp:    urv.timestamp,
			Message:      fmt.Sprintf("Child processes (%d) exceeds limit (%d)", urv.usage.ChildProcesses, limits.MaxChildProcesses),
		})
	}

	return violations
}
