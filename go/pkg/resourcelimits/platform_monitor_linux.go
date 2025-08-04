//go:build linux
// +build linux

package resourcelimits

import (
	"github.com/core-tools/hsu-core/pkg/logging"
)

// linuxResourceMonitor provides Linux-specific monitoring
type linuxResourceMonitor struct {
	logger logging.Logger
}

func newLinuxResourceMonitor(logger logging.Logger) PlatformResourceMonitor {
	return &linuxResourceMonitor{
		logger: logger,
	}
}

func (l *linuxResourceMonitor) GetProcessUsage(pid int) (*ResourceUsage, error) {
	// Note: Linux-specific resource monitoring planned for Phase 4
	// Implementation strategy: /proc/{pid}/stat, /proc/{pid}/status, /proc/{pid}/io for comprehensive metrics
	l.logger.Debugf("Linux resource monitoring for PID %d - using generic implementation for now", pid)
	return newGenericResourceMonitor(l.logger).GetProcessUsage(pid)
}

func (l *linuxResourceMonitor) SupportsRealTimeMonitoring() bool {
	return true // Linux supports real-time monitoring via /proc filesystem
}

// createPlatformSpecificMonitor creates Linux-specific monitor
func createPlatformSpecificMonitor(logger logging.Logger) PlatformResourceMonitor {
	return newLinuxResourceMonitor(logger)
}
