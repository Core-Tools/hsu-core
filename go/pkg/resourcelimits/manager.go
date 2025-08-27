package resourcelimits

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
)

// ResourceLimitManager coordinates resource monitoring and enforcement
type resourceLimitManager struct {
	pid              int
	limits           *ResourceLimits
	monitor          ResourceMonitor
	enforcer         ResourceEnforcer
	violationChecker ResourceViolationChecker
	logger           logging.Logger

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mutex  sync.RWMutex

	// State
	isRunning  bool
	violations []*ResourceViolation

	// Configuration
	checkInterval time.Duration

	// External violation handling
	violationCallback ResourceViolationCallback
}

// NewResourceLimitManager creates a comprehensive resource limit manager with monitoring and violation detection.
// Provides real-time resource usage tracking, configurable violation checking, and policy-based responses
// (log, alert, restart, graceful shutdown, immediate kill). Integrates with circuit breakers for
// production-grade resource management and process protection.
func NewResourceLimitManager(pid int, limits *ResourceLimits, logger logging.Logger) ResourceLimitManager {
	// Create monitoring config if not provided
	var monitoringConfig *ResourceMonitoringConfig
	if limits != nil {
		monitoringConfig = limits.Monitoring
	}
	if monitoringConfig == nil {
		monitoringConfig = &ResourceMonitoringConfig{
			Enabled:          true,
			Interval:         30 * time.Second,
			HistoryRetention: 24 * time.Hour,
			AlertingEnabled:  true,
		}
	}

	// Set default check interval
	checkInterval := monitoringConfig.Interval
	if checkInterval == 0 {
		checkInterval = 30 * time.Second
	}

	monitor := NewResourceMonitor(pid, monitoringConfig, logger)
	enforcer := NewResourceEnforcer(logger)
	violationChecker := NewResourceViolationChecker(logger)

	return &resourceLimitManager{
		pid:              pid,
		limits:           limits,
		monitor:          monitor,
		enforcer:         enforcer,
		violationChecker: violationChecker,
		logger:           logger,
		checkInterval:    checkInterval,
		violations:       make([]*ResourceViolation, 0),
	}
}

// Start begins resource monitoring and enforcement
func (rlm *resourceLimitManager) Start(ctx context.Context) error {
	rlm.mutex.Lock()
	defer rlm.mutex.Unlock()

	if rlm.isRunning {
		return errors.NewValidationError("resource limit manager is already running", nil).WithContext("pid", rlm.pid)
	}

	if rlm.limits == nil {
		rlm.logger.Infof("No resource limits configured for PID %d", rlm.pid)
		return nil
	}

	// rlm.isRunning is always false if no limits are configured

	rlm.logger.Infof("Starting resource limit management for PID %d", rlm.pid)

	// Apply initial limits
	if err := rlm.enforcer.ApplyLimits(rlm.pid, rlm.limits); err != nil {
		rlm.logger.Warnf("Failed to apply some resource limits to PID %d: %v", rlm.pid, err)
		// Continue anyway - monitoring can still work
	}

	rlm.ctx, rlm.cancel = context.WithCancel(ctx)
	rlm.isRunning = true

	// Start monitoring (it will handle the enabled flag itself)
	if err := rlm.monitor.Start(rlm.ctx); err != nil {
		rlm.logger.Errorf("Failed to start resource monitoring for PID %d: %v", rlm.pid, err)
		return fmt.Errorf("failed to start resource monitoring: %v", err)
	}

	// Set up monitoring callbacks
	rlm.monitor.SetUsageCallback(rlm.onUsageUpdate)

	// Start violation checking loop
	rlm.wg.Add(1)
	go rlm.violationCheckLoop()

	rlm.logger.Infof("Resource limit management started for PID %d", rlm.pid)
	return nil
}

// Stop stops resource monitoring and enforcement
func (rlm *resourceLimitManager) Stop() {
	rlm.mutex.Lock()
	defer rlm.mutex.Unlock()

	rlm.logger.Infof("Stopping resource limit management for PID %d", rlm.pid)

	if !rlm.isRunning {
		rlm.logger.Infof("Resource limit management not running for PID %d", rlm.pid)
		return
	}

	// rlm.limits != nil here, see Start() logic

	rlm.cancel()
	rlm.isRunning = false

	// Stop monitoring
	rlm.monitor.Stop()

	// Wait for background goroutines
	rlm.wg.Wait()

	rlm.logger.Infof("Resource limit management stopped for PID %d", rlm.pid)
}

// GetViolations returns recent resource violations
func (rlm *resourceLimitManager) GetViolations() []*ResourceViolation {
	rlm.mutex.RLock()
	defer rlm.mutex.RUnlock()

	// Return a copy to prevent external modification
	violations := make([]*ResourceViolation, len(rlm.violations))
	copy(violations, rlm.violations)
	return violations
}

// SetViolationCallback sets a callback for handling resource violations
func (rlm *resourceLimitManager) SetViolationCallback(callback ResourceViolationCallback) {
	rlm.mutex.Lock()
	defer rlm.mutex.Unlock()
	rlm.violationCallback = callback
}

// GetLimits returns the current resource limits
func (rlm *resourceLimitManager) setViolations(violations []*ResourceViolation) {
	rlm.mutex.Lock()
	defer rlm.mutex.Unlock()
	rlm.violations = make([]*ResourceViolation, len(violations))
	copy(rlm.violations, violations)
}

// GetLimits returns the current resource limits
func (rlm *resourceLimitManager) getViolationCallback() ResourceViolationCallback {
	rlm.mutex.RLock()
	defer rlm.mutex.RUnlock()
	return rlm.violationCallback
}

// onUsageUpdate handles resource usage updates
func (rlm *resourceLimitManager) onUsageUpdate(usage *ResourceUsage) {
	rlm.logger.Debugf("Resource usage update for PID %d: Memory RSS: %dMB, CPU: %.1f%%",
		rlm.pid, usage.MemoryRSS/(1024*1024), usage.CPUPercent)

	// Could add custom usage processing here
	// For example, trend analysis, alerting thresholds, etc.
}

// violationCheckLoop periodically checks for resource violations
func (rlm *resourceLimitManager) violationCheckLoop() {
	defer rlm.wg.Done()

	// Use a shorter interval for violation checking than general monitoring
	checkInterval := rlm.checkInterval
	if checkInterval > 10*time.Second {
		checkInterval = 10 * time.Second
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rlm.ctx.Done():
			rlm.logger.Debugf("Resource violation check loop stopped for PID %d", rlm.pid)
			return

		case <-ticker.C:
			rlm.checkViolations()
		}
	}
}

// checkViolations checks for resource limit violations
func (rlm *resourceLimitManager) checkViolations() {
	if rlm.limits == nil {
		return
	}

	// Get current usage
	usage, err := rlm.monitor.GetCurrentUsage()
	if err != nil {
		rlm.logger.Debugf("Failed to get current usage for violation check on PID %d: %v", rlm.pid, err)
		return
	}

	// Check violations using the violations checker
	violations := rlm.violationChecker.CheckViolations(usage, rlm.limits)

	rlm.setViolations(violations)

	// Process any new violations
	for _, violation := range violations {
		rlm.dispatchViolation(violation)
	}
}

// onViolation handles resource limit violations (internal mode)
func (rlm *resourceLimitManager) dispatchViolation(violation *ResourceViolation) {
	callback := rlm.getViolationCallback()

	rlm.logger.Warnf("Resource violation detected for process with PID %d: %s, severity: %s", rlm.pid, violation.Message, violation.Severity)

	// Trigger enforcement
	if violation.Severity != ViolationSeverityCritical {
		return
	}

	policy := rlm.getPolicyByLimitType(violation.LimitType)
	if policy == ResourcePolicy("") {
		rlm.logger.Warnf("Invalid policy for resource limit type %s on PID %d", violation.LimitType, rlm.pid)
		return
	}

	go callback(policy, violation)
}

func (rlm *resourceLimitManager) getPolicyByLimitType(limitType ResourceLimitType) ResourcePolicy {
	// Handle different violation policies
	switch limitType {
	case ResourceLimitTypeMemory:
		if rlm.limits.Memory != nil {
			return rlm.limits.Memory.Policy
		}
	case ResourceLimitTypeCPU:
		if rlm.limits.CPU != nil {
			return rlm.limits.CPU.Policy
		}
	case ResourceLimitTypeProcess:
		if rlm.limits.Process != nil {
			return rlm.limits.Process.Policy
		}
	default:
		rlm.logger.Warnf("Unknown resource limit type: %s", limitType)
		return ResourcePolicy("")
	}

	return ResourcePolicy("")
}
