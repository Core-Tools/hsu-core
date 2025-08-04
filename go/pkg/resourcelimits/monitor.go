package resourcelimits

import (
	"context"
	"sync"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/processstate"
)

// resourceMonitor implements ResourceMonitor interface
type resourceMonitor struct {
	pid    int
	config *ResourceMonitoringConfig
	logger logging.Logger

	// Callbacks
	usageCallback     ResourceUsageCallback
	violationCallback ResourceViolationCallback

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mutex  sync.RWMutex

	// State
	isRunning    bool
	usageHistory []*ResourceUsage

	// Platform-specific monitor
	platformMonitor PlatformResourceMonitor
}

// PlatformResourceMonitor defines platform-specific monitoring interface
type PlatformResourceMonitor interface {
	GetProcessUsage(pid int) (*ResourceUsage, error)
	SupportsRealTimeMonitoring() bool
}

// NewResourceMonitor creates a new resource monitor for a process
func NewResourceMonitor(pid int, config *ResourceMonitoringConfig, logger logging.Logger) ResourceMonitor {
	if config == nil {
		config = &ResourceMonitoringConfig{
			Enabled:          true,
			Interval:         30 * time.Second,
			HistoryRetention: 24 * time.Hour,
			AlertingEnabled:  true,
		}
	}

	// Set defaults
	if config.Interval == 0 {
		config.Interval = 30 * time.Second
	}
	if config.HistoryRetention == 0 {
		config.HistoryRetention = 24 * time.Hour
	}

	return &resourceMonitor{
		pid:             pid,
		config:          config,
		logger:          logger,
		usageHistory:    make([]*ResourceUsage, 0),
		platformMonitor: newPlatformResourceMonitor(logger),
	}
}

// Start begins resource monitoring
func (rm *resourceMonitor) Start(ctx context.Context) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if rm.isRunning {
		return errors.NewValidationError("resource monitor is already running", nil).WithContext("pid", rm.pid)
	}

	if !rm.config.Enabled {
		rm.logger.Infof("Resource monitoring disabled for PID %d", rm.pid)
		return nil
	}

	// Check if process exists
	running, err := processstate.IsProcessRunning(rm.pid)
	if !running {
		rm.logger.Infof("Not running process PID %d, err: %v for resource monitoring", rm.pid, err)
		return errors.NewProcessError("process is not running", err).WithContext("pid", rm.pid)
	}

	rm.ctx, rm.cancel = context.WithCancel(ctx)
	rm.isRunning = true

	rm.logger.Infof("Starting resource monitoring for PID %d, interval: %v", rm.pid, rm.config.Interval)

	// Start monitoring goroutine
	rm.wg.Add(1)
	go rm.monitorLoop()

	return nil
}

// Stop stops resource monitoring
func (rm *resourceMonitor) Stop() {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	rm.logger.Infof("Stopping resource monitoring for PID %d", rm.pid)

	if !rm.isRunning {
		rm.logger.Infof("Resource monitoring not running for PID %d", rm.pid)
		return
	}

	rm.cancel()
	rm.isRunning = false

	// Wait for monitoring goroutine to finish
	rm.wg.Wait()

	rm.logger.Infof("Resource monitoring stopped for PID %d", rm.pid)
}

// GetCurrentUsage returns current resource usage
func (rm *resourceMonitor) GetCurrentUsage() (*ResourceUsage, error) {
	// Check if process exists
	running, err := processstate.IsProcessRunning(rm.pid)
	if !running {
		return nil, errors.NewProcessError("process is not running", err).WithContext("pid", rm.pid)
	}

	usage, err := rm.platformMonitor.GetProcessUsage(rm.pid)
	if err != nil {
		return nil, errors.NewInternalError("failed to get resource usage", err).WithContext("pid", rm.pid)
	}

	return usage, nil
}

// SetUsageCallback sets callback for resource usage updates
func (rm *resourceMonitor) SetUsageCallback(callback ResourceUsageCallback) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()
	rm.usageCallback = callback
}

// monitorLoop is the main monitoring goroutine
func (rm *resourceMonitor) monitorLoop() {
	defer rm.wg.Done()

	ticker := time.NewTicker(rm.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.ctx.Done():
			rm.logger.Debugf("Resource monitoring loop stopped for PID %d", rm.pid)
			return

		case <-ticker.C:
			rm.collectUsage()
		}
	}
}

// collectUsage collects current resource usage
func (rm *resourceMonitor) collectUsage() {
	// Check if process is still running
	running, err := processstate.IsProcessRunning(rm.pid)
	if !running {
		rm.logger.Warnf("Failed to collect resource usage: process %d is not running, err: %v", rm.pid, err)
		return
	}

	usage, err := rm.platformMonitor.GetProcessUsage(rm.pid)
	if err != nil {
		rm.logger.Errorf("Failed to collect resource usage for PID %d: %v", rm.pid, err)
		return
	}

	rm.logger.Debugf("Resource usage for PID %d: Memory RSS: %dMB, CPU: %.1f%%, FDs: %d",
		rm.pid, usage.MemoryRSS/(1024*1024), usage.CPUPercent, usage.OpenFileDescriptors)

	var callback ResourceUsageCallback

	// Store usage
	rm.mutex.Lock()
	rm.addToHistory(usage)
	callback = rm.usageCallback
	rm.mutex.Unlock()

	// Call usage callback
	if callback != nil {
		callback(usage)
	}
}

// addToHistory adds usage to history and manages retention
func (rm *resourceMonitor) addToHistory(usage *ResourceUsage) {
	rm.usageHistory = append(rm.usageHistory, usage)

	// Clean old entries based on retention policy
	cutoff := time.Now().Add(-rm.config.HistoryRetention)
	for len(rm.usageHistory) > 0 && rm.usageHistory[0].Timestamp.Before(cutoff) {
		rm.usageHistory = rm.usageHistory[1:]
	}
}

// GetUsageHistory returns historical usage data
func (rm *resourceMonitor) GetUsageHistory(since time.Time) []*ResourceUsage {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	var result []*ResourceUsage
	for _, usage := range rm.usageHistory {
		if usage.Timestamp.After(since) {
			result = append(result, usage)
		}
	}

	return result
}
