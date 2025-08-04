//go:build darwin
// +build darwin

package resourcelimits

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/core-tools/hsu-core/pkg/logging"
)

// darwinResourceMonitor provides macOS-specific monitoring
type darwinResourceMonitor struct {
	logger      logging.Logger
	lastCPUTime map[int]CPUTimeInfo // Track CPU usage over time
}

// CPUTimeInfo tracks CPU time for calculating percentage
type CPUTimeInfo struct {
	UserTime   float64
	SystemTime float64
	Timestamp  time.Time
}

func newDarwinResourceMonitor(logger logging.Logger) PlatformResourceMonitor {
	return &darwinResourceMonitor{
		logger:      logger,
		lastCPUTime: make(map[int]CPUTimeInfo),
	}
}

func (d *darwinResourceMonitor) GetProcessUsage(pid int) (*ResourceUsage, error) {
	d.logger.Debugf("Getting macOS resource usage for PID %d", pid)

	usage := &ResourceUsage{
		Timestamp: time.Now(),
	}

	// Get memory usage using ps command (reliable across macOS versions)
	if err := d.getMemoryUsagePS(pid, usage); err != nil {
		d.logger.Warnf("Failed to get memory usage for PID %d: %v", pid, err)
		// Try fallback method
		if err := d.getMemoryUsageProcFS(pid, usage); err != nil {
			d.logger.Warnf("Fallback memory monitoring failed for PID %d: %v", pid, err)
		}
	}

	// Get CPU usage using ps command
	if err := d.getCPUUsagePS(pid, usage); err != nil {
		d.logger.Warnf("Failed to get CPU usage for PID %d: %v", pid, err)
	}

	// Get file descriptor count using lsof command
	if err := d.getFileDescriptorCount(pid, usage); err != nil {
		d.logger.Debugf("Failed to get file descriptor count for PID %d: %v", pid, err)
		// This is optional - don't warn
	}

	// Get I/O usage using iostat equivalent approach
	if err := d.getIOUsage(pid, usage); err != nil {
		d.logger.Debugf("Failed to get I/O usage for PID %d: %v", pid, err)
		// This is optional - don't warn
	}

	d.logger.Debugf("macOS resource usage for PID %d: Memory RSS: %dMB, CPU: %.1f%%, FDs: %d",
		pid, usage.MemoryRSS/(1024*1024), usage.CPUPercent, usage.OpenFileDescriptors)

	return usage, nil
}

// getMemoryUsagePS gets memory usage using ps command (most reliable method)
func (d *darwinResourceMonitor) getMemoryUsagePS(pid int, usage *ResourceUsage) error {
	// Use ps to get RSS and VSZ in KB
	cmd := exec.Command("ps", "-o", "rss,vsz", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ps command failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return fmt.Errorf("unexpected ps output format")
	}

	// Parse the data line (skip header)
	fields := strings.Fields(lines[1])
	if len(fields) < 2 {
		return fmt.Errorf("insufficient ps output fields")
	}

	// RSS (Resident Set Size) in KB
	if rssKB, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
		usage.MemoryRSS = rssKB * 1024 // Convert to bytes
	}

	// VSZ (Virtual Size) in KB
	if vszKB, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
		usage.MemoryVirtual = vszKB * 1024 // Convert to bytes
	}

	// Calculate memory percentage (rough estimate)
	if usage.MemoryRSS > 0 {
		if totalMem := d.getTotalSystemMemory(); totalMem > 0 {
			usage.MemoryPercent = float64(usage.MemoryRSS) / float64(totalMem) * 100.0
		}
	}

	return nil
}

// getMemoryUsageProcFS tries to get memory usage from process info (fallback)
func (d *darwinResourceMonitor) getMemoryUsageProcFS(pid int, usage *ResourceUsage) error {
	// On macOS, we can try to read some process information indirectly
	// This is a fallback method and may not be as accurate
	d.logger.Debugf("Using fallback memory monitoring for PID %d", pid)
	return fmt.Errorf("procfs fallback not implemented for macOS")
}

// getCPUUsagePS gets CPU usage using ps command
func (d *darwinResourceMonitor) getCPUUsagePS(pid int, usage *ResourceUsage) error {
	// Use ps to get CPU percentage and cumulative CPU time
	cmd := exec.Command("ps", "-o", "pcpu,time", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ps command failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return fmt.Errorf("unexpected ps output format")
	}

	// Parse the data line (skip header)
	fields := strings.Fields(lines[1])
	if len(fields) < 2 {
		return fmt.Errorf("insufficient ps output fields")
	}

	// CPU percentage
	if cpuPercent, err := strconv.ParseFloat(fields[0], 64); err == nil {
		usage.CPUPercent = cpuPercent
	}

	// CPU time (format: MMM:SS.ss or HH:MM:SS)
	if cpuTime := d.parseCPUTime(fields[1]); cpuTime > 0 {
		usage.CPUTime = cpuTime
	}

	return nil
}

// parseCPUTime parses CPU time from ps output format
func (d *darwinResourceMonitor) parseCPUTime(timeStr string) float64 {
	// Handle formats like "0:01.23" or "1:23:45"
	parts := strings.Split(timeStr, ":")

	var seconds float64
	multiplier := 1.0

	// Process from right to left (seconds, minutes, hours)
	for i := len(parts) - 1; i >= 0; i-- {
		if val, err := strconv.ParseFloat(parts[i], 64); err == nil {
			seconds += val * multiplier
			multiplier *= 60 // Next part is 60x bigger (seconds->minutes->hours)
		}
	}

	return seconds
}

// getFileDescriptorCount gets file descriptor count using lsof
func (d *darwinResourceMonitor) getFileDescriptorCount(pid int, usage *ResourceUsage) error {
	// Use lsof to count open file descriptors
	cmd := exec.Command("lsof", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		// lsof might fail if no permissions or process doesn't exist
		return fmt.Errorf("lsof command failed: %v", err)
	}

	// Count lines (excluding header)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 1 {
		usage.OpenFileDescriptors = len(lines) - 1 // Subtract header line
	}

	return nil
}

// getIOUsage attempts to get I/O usage (limited on macOS without elevated privileges)
func (d *darwinResourceMonitor) getIOUsage(pid int, usage *ResourceUsage) error {
	// On macOS, getting per-process I/O statistics requires special privileges
	// We'll implement a basic approach using available tools

	// Try using fs_usage if available (requires root, so likely to fail)
	cmd := exec.Command("which", "fs_usage")
	if err := cmd.Run(); err != nil {
		// fs_usage not available or not accessible
		return fmt.Errorf("fs_usage not available for I/O monitoring")
	}

	// For now, set I/O statistics to 0 since reliable I/O monitoring
	// requires root privileges or special entitlements on macOS
	usage.IOReadBytes = 0
	usage.IOWriteBytes = 0
	usage.IOReadOps = 0
	usage.IOWriteOps = 0

	return nil
}

// getTotalSystemMemory gets total system memory for percentage calculations
func (d *darwinResourceMonitor) getTotalSystemMemory() int64 {
	// Use sysctl to get total memory
	cmd := exec.Command("sysctl", "-n", "hw.memsize")
	output, err := cmd.Output()
	if err != nil {
		d.logger.Debugf("Failed to get system memory: %v", err)
		return 0
	}

	if totalBytes, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64); err == nil {
		return totalBytes
	}

	return 0
}

// getChildProcessCount counts child processes (experimental)
func (d *darwinResourceMonitor) getChildProcessCount(pid int) int {
	// Use pgrep to find child processes
	cmd := exec.Command("pgrep", "-P", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}

	return len(lines)
}

// checkProcessExists verifies if process exists
func (d *darwinResourceMonitor) checkProcessExists(pid int) bool {
	// Check if process exists using kill -0
	cmd := exec.Command("kill", "-0", strconv.Itoa(pid))
	return cmd.Run() == nil
}

// getProcessInfo gets additional process information
func (d *darwinResourceMonitor) getProcessInfo(pid int) (map[string]string, error) {
	info := make(map[string]string)

	// Get process name and other details using ps
	cmd := exec.Command("ps", "-o", "comm,state,nice,ppid", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps command failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("unexpected ps output format")
	}

	// Parse the data line
	fields := strings.Fields(lines[1])
	if len(fields) >= 4 {
		info["command"] = fields[0]
		info["state"] = fields[1]
		info["nice"] = fields[2]
		info["ppid"] = fields[3]
	}

	return info, nil
}

func (d *darwinResourceMonitor) SupportsRealTimeMonitoring() bool {
	return true // macOS supports real-time monitoring via ps/lsof commands
}

// createPlatformSpecificMonitor creates macOS-specific monitor
func createPlatformSpecificMonitor(logger logging.Logger) PlatformResourceMonitor {
	return newDarwinResourceMonitor(logger)
}
