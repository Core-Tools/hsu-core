package processcontrolimpl

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logcollection"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"
	"github.com/core-tools/hsu-core/pkg/resourcelimits"
	"github.com/core-tools/hsu-core/pkg/workers/processcontrol"
)

// shouldRestartBasedOnPolicy evaluates restart policy (moved from health monitor for better encapsulation)
func shouldRestartBasedOnPolicy(policy processcontrol.RestartPolicy, status monitoring.HealthCheckStatus) bool {
	switch policy {
	case processcontrol.RestartAlways:
		return true
	case processcontrol.RestartOnFailure:
		// Restart on health check failures if we've reached unhealthy status
		return status == monitoring.HealthCheckStatusUnhealthy
	case processcontrol.RestartUnlessStopped:
		// Similar to always, but should check if process was intentionally stopped
		// For now, treat same as RestartOnFailure
		return status == monitoring.HealthCheckStatusUnhealthy
	case processcontrol.RestartNever:
		return false
	default:
		// Unknown policy, default to no restart
		return false
	}
}

type processControl struct {
	config   processcontrol.ProcessControlOptions
	stdout   io.ReadCloser
	logger   logging.Logger
	workerID string

	// Running process tracking
	process           *os.Process
	processContext    map[string]string
	processDoneSignal chan error

	// Health monitor
	healthMonitor monitoring.HealthMonitor

	// Resource limit management
	resourceManager resourcelimits.ResourceLimitManager

	// Context-aware restart circuit breaker
	restartCircuitBreaker RestartCircuitBreaker

	// Log collection
	logCollectionActive bool // Track if log collection is active for this worker

	// Process lifecycle state management
	state processcontrol.ProcessState

	// Worker profile type for context-aware restart decisions
	workerProfileType string

	// Error tracking for diagnostics
	lastError       *processcontrol.ProcessError
	failureCount    int
	lastAttemptTime time.Time
	startTime       *time.Time

	// Mutex to protect concurrent access to fields
	mutex sync.RWMutex
}

func NewProcessControl(config processcontrol.ProcessControlOptions, workerID string, logger logging.Logger) processcontrol.ProcessControl {
	// Create context-aware circuit breaker if restart is enabled
	var restartCircuitBreaker RestartCircuitBreaker
	if config.ContextAwareRestart != nil && config.CanRestart {
		// ✅ NEW: Use provided ContextAwareRestartConfig directly (no more hard-coded construction!)
		logger.Infof("Using provided context-aware restart configuration for worker %s", workerID)

		// Determine worker profile type from configuration or use default
		workerProfileType := "default"
		if config.WorkerProfileType != "" {
			workerProfileType = config.WorkerProfileType
		}

		// Use the provided configuration directly
		restartCircuitBreaker = NewRestartCircuitBreaker(
			config.ContextAwareRestart, workerID, workerProfileType, logger)
	}

	return &processControl{
		config:                config,
		logger:                logger,
		workerID:              workerID,
		restartCircuitBreaker: restartCircuitBreaker,
		state:                 processcontrol.ProcessStateIdle, // ✅ Initialize to idle state
		workerProfileType:     config.WorkerProfileType,        // ✅ RENAMED field
	}
}

func (pc *processControl) Start(ctx context.Context) error {
	// Validate context
	if ctx == nil {
		// Set state to failed_start for consistency with enhanced error reporting
		pc.mutex.Lock()
		pc.state = processcontrol.ProcessStateFailedStart
		pc.lastError = categorizeProcessError(errors.NewValidationError("context cannot be nil", nil))
		pc.failureCount++
		pc.lastAttemptTime = time.Now()
		pc.mutex.Unlock()

		return errors.NewValidationError("context cannot be nil", nil)
	}

	return pc.startInternal(ctx)
}

func (pc *processControl) Stop(ctx context.Context) error {
	// Validate context
	if ctx == nil {
		return errors.NewValidationError("context cannot be nil", nil)
	}

	if pc.process == nil {
		return errors.NewProcessError("process not attached", nil)
	}

	return pc.stopInternal(ctx, false)
}

func (pc *processControl) Restart(ctx context.Context, force bool) error {
	// Validate context
	if ctx == nil {
		return errors.NewValidationError("context cannot be nil", nil)
	}

	if pc.process == nil {
		return errors.NewProcessError("process not attached", nil)
	}

	pc.logger.Infof("Restart requested, worker: %s, force: %t", pc.workerID, force)

	// If force=true, bypass circuit breaker for immediate restart
	if force {
		pc.logger.Infof("Force restart: bypassing circuit breaker, worker: %s", pc.workerID)
		return pc.restartInternal(ctx, false)
	}

	// If force=false, use circuit breaker safety mechanisms (default/recommended)
	restartContext := processcontrol.RestartContext{
		TriggerType:       processcontrol.RestartTriggerManual,
		Severity:          "critical",
		WorkerProfileType: pc.workerProfileType,
		Message:           "Manual restart request",
	}

	// Circuit breaker handles all context-aware logic and logging
	if pc.restartCircuitBreaker != nil {
		pc.logger.Infof("Safe restart: using circuit breaker, worker: %s", pc.workerID)
		wrappedRestart := func() error {
			return pc.restartInternal(ctx, false) // normally, running process -> false
		}
		return pc.restartCircuitBreaker.ExecuteRestart(wrappedRestart, restartContext)
	}

	// Fallback if no circuit breaker configured
	pc.logger.Warnf("No circuit breaker configured, proceeding with direct restart, worker: %s", pc.workerID)
	return pc.restartInternal(ctx, false)
}

// GetState returns the current process state (for monitoring/debugging)
// ✅ DEFER-ONLY: Uses automatic unlock
func (pc *processControl) GetState() processcontrol.ProcessState {
	return pc.safeGetState()
}

// GetContext returns the current process context
func (pc *processControl) GetContext() map[string]string {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()

	return pc.processContext
}

// GetDiagnostics returns detailed process diagnostics including error information
func (pc *processControl) GetDiagnostics() processcontrol.ProcessDiagnostics {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()

	var processID int
	if pc.process != nil {
		processID = pc.process.Pid
	}

	// Determine process type and executable information
	executablePath := ""
	executableExists := true

	if pc.config.ExecuteCmd != nil {
		executablePath = "managed_process" // We don't extract path from ExecuteCmd
		// For managed processes, use the last error to determine if executable exists
		// This leverages the existing ensureExecutable logic instead of duplicating it
		if pc.lastError != nil && pc.lastError.Category == processcontrol.ErrorCategoryExecutableNotFound {
			executableExists = false
		}
	} else if pc.config.AttachCmd != nil {
		executablePath = "unmanaged_process" // No executable path for unmanaged
		// Unmanaged processes attach to existing processes, so "executable" always "exists"
		executableExists = true
	} else {
		executablePath = "no_execution_method"
		executableExists = false
	}

	return processcontrol.ProcessDiagnostics{
		State:            pc.state,
		Context:          pc.processContext,
		LastError:        pc.lastError,
		ProcessID:        processID,
		StartTime:        pc.startTime,
		ExecutablePath:   executablePath,
		ExecutableExists: executableExists,
		FailureCount:     pc.failureCount,
		LastAttemptTime:  pc.lastAttemptTime,
	}
}

func (pc *processControl) startInternal(ctx context.Context) error {
	pc.logger.Infof("Starting process control for worker %s", pc.workerID)

	// Acquire exclusive lock for field modifications
	pc.mutex.Lock()
	defer pc.mutex.Unlock()

	// ✅ CRITICAL: Validate state transition before proceeding
	if !pc.canStartFromState(pc.state) {
		return errors.NewValidationError(
			fmt.Sprintf("cannot start process in state '%s': operation not allowed", pc.state),
			nil).WithContext("worker", pc.workerID).WithContext("current_state", string(pc.state))
	}

	// Set state to starting to prevent concurrent operations
	pc.state = processcontrol.ProcessStateStarting

	// Track attempt time for diagnostics
	pc.lastAttemptTime = time.Now()

	cmdResult, err := pc.startProcess(ctx)
	if err != nil {
		pc.state = processcontrol.ProcessStateFailedStart // ✅ Use new failed start state

		// Categorize and store the error for diagnostics
		pc.lastError = categorizeProcessError(err)
		pc.failureCount++

		return errors.NewInternalError("failed to start process", err)
	}

	process := cmdResult.Process
	processContext := cmdResult.ProcessContext
	stdout := cmdResult.Stdout
	healthCheckConfig := cmdResult.HealthCheckConfig

	pc.process = process
	pc.processContext = processContext
	pc.stdout = stdout
	pc.startTime = &pc.lastAttemptTime // Record successful start time
	pc.lastError = nil                 // Clear any previous error
	pc.failureCount = 0                // Reset failure count on success

	processDoneSignal := make(chan error, 1)

	// Start a goroutine to wait for process exit
	go func() {
		state, err := process.Wait()
		if err != nil {
			pc.logger.Infof("Process PID %d wait failed: %v", process.Pid, err)
			processDoneSignal <- errors.NewProcessError("process wait failed", err).WithContext("pid", process.Pid)
		} else {
			pc.logger.Infof("Process PID %d exited with status: %v", process.Pid, state)
			processDoneSignal <- nil
		}
	}()

	pc.processDoneSignal = processDoneSignal

	// Start log collection if service is available (NEW)
	if err := pc.startLogCollection(ctx, process, stdout); err != nil {
		pc.logger.Warnf("Failed to start log collection for worker %s: %v", pc.workerID, err)
		// Don't fail process start due to log collection issues
	}

	healthMonitor, err := pc.startHealthCheck(ctx, process.Pid, healthCheckConfig)
	if err != nil {
		pc.logger.Warnf("Failed to start health monitor, worker: %s, error: %v", pc.workerID, err)
		// ignore health monitor error
	}

	pc.healthMonitor = healthMonitor

	// Initialize resource monitoring if limits are specified
	resourceManager, err := pc.startResourceMonitoring(ctx, process.Pid)
	if err != nil {
		pc.logger.Warnf("Failed to initialize resource monitoring for worker %s: %v", pc.workerID, err)
		// Don't fail process start due to monitoring issues
	}

	pc.resourceManager = resourceManager

	// ✅ Process successfully started and running
	pc.state = processcontrol.ProcessStateRunning

	pc.logger.Infof("Process control started, worker: %s", pc.workerID)

	return nil
}

// canStartFromState validates if starting is allowed from the current state
func (pc *processControl) canStartFromState(currentState processcontrol.ProcessState) bool {
	switch currentState {
	case processcontrol.ProcessStateIdle:
		return true // ✅ Can start from idle
	case processcontrol.ProcessStateStarting:
		return false // ❌ Already starting
	case processcontrol.ProcessStateRunning:
		return false // ❌ Already running
	case processcontrol.ProcessStateStopping:
		return false // ❌ Still stopping, wait for completion
	case processcontrol.ProcessStateTerminating:
		return false // ❌ Still terminating, wait for completion
	default:
		return false // ❌ Unknown state
	}
}

func (pc *processControl) startProcess(ctx context.Context) (*processcontrol.CommandResult, error) {
	pc.logger.Infof("Starting process for worker %s, can_attach: %t, can_execute: %t, can_terminate: %t",
		pc.workerID, pc.config.CanAttach, (pc.config.ExecuteCmd != nil), pc.config.CanTerminate)

	var process *os.Process
	var processContext map[string]string
	var stdout io.ReadCloser

	healthCheckConfig := pc.config.HealthCheck

	executeCmd := pc.config.ExecuteCmd
	attachCmd := pc.config.AttachCmd

	if pc.config.CanAttach && attachCmd != nil {
		pc.logger.Infof("Attempting to attach to existing process, worker: %s", pc.workerID)

		cmdResult, err := attachCmd(ctx)
		if err != nil {
			pc.logger.Warnf("Failed to attach to existing process, worker: %s, error: %v", pc.workerID, err)
		} else {
			process = cmdResult.Process
			processContext = cmdResult.ProcessContext
			stdout = cmdResult.Stdout
			healthCheckConfig = cmdResult.HealthCheckConfig // Overrides healthCheckConfig

			pc.logger.Infof("Successfully attached to existing process, worker: %s, PID: %d", pc.workerID, process.Pid)
			executeCmd = nil // attached successfully, no need to execute cmd
		}
	}

	if executeCmd != nil { // can't attach or not attached
		pc.logger.Infof("Executing new process, worker: %s", pc.workerID)

		cmdResult, err := executeCmd(ctx)
		if err != nil {
			return nil, errors.NewProcessError("failed to start process", err)
		}

		process = cmdResult.Process
		processContext = cmdResult.ProcessContext
		stdout = cmdResult.Stdout
		healthCheckConfig = cmdResult.HealthCheckConfig // Overrides healthCheckConfig

		pc.logger.Infof("New process started successfully, worker: %s, PID: %d", pc.workerID, process.Pid)

	}

	// Check if context was cancelled during startup
	if ctx.Err() != nil {
		pc.logger.Infof("Context cancelled during startup, cleaning up...")
		if process != nil {
			process.Kill()
		}
		return nil, errors.NewCancelledError("startup cancelled", ctx.Err())
	}

	// Validate that we have a process
	if process == nil {
		return nil, errors.NewInternalError("no process available after startup", nil)
	}

	pc.logger.Infof("Process started, process: %+v, process context: %+v", process, processContext)

	return &processcontrol.CommandResult{
		Process:           process,
		ProcessContext:    processContext,
		Stdout:            stdout,
		HealthCheckConfig: healthCheckConfig,
	}, nil
}

func (pc *processControl) startHealthCheck(ctx context.Context, pid int, healthCheckConfig *monitoring.HealthCheckConfig) (monitoring.HealthMonitor, error) {
	pc.logger.Infof("Starting health monitor for worker %s, config: %+v", pc.workerID, healthCheckConfig)

	if healthCheckConfig == nil {
		pc.logger.Errorf("Health check configuration is nil, worker: %s", pc.workerID)
		return nil, errors.NewValidationError("health check configuration is nil", nil).WithContext("id", pc.workerID)
	}

	var healthMonitor monitoring.HealthMonitor

	// Create process info for health monitoring - simplified
	processInfo := &monitoring.ProcessInfo{
		PID: pid, // Only PID needed
	}

	// Create health monitor with process info for all restart scenarios
	if healthCheckConfig.Type == monitoring.HealthCheckTypeProcess {
		// Create health monitor with process info but no restart
		healthMonitor = monitoring.NewHealthMonitorWithProcessInfo(
			healthCheckConfig, pc.workerID, processInfo, pc.logger)
	} else {
		// For other health check types, create standard monitor
		healthMonitor = monitoring.NewHealthMonitor(
			healthCheckConfig, pc.workerID, pc.logger)
	}

	// Set up restart callback with context awareness
	if pc.restartCircuitBreaker != nil {
		healthMonitor.SetRestartCallback(func(reason string) error {
			// ✅ NEW: Evaluate restart policy before proceeding (moved from health monitor)
			healthState := healthMonitor.State()
			shouldRestart := shouldRestartBasedOnPolicy(pc.config.RestartPolicy, healthState.Status)
			if !shouldRestart {
				pc.logger.Debugf("Health restart skipped due to policy, worker: %s, policy: %s, status: %s, reason: %s",
					pc.workerID, pc.config.RestartPolicy, healthState.Status, reason)
				return nil
			}

			pc.logger.Warnf("Health restart requested, worker: %s, policy: %s, status: %s, reason: %s",
				pc.workerID, pc.config.RestartPolicy, healthState.Status, reason)

			// Create context for health failure restart
			restartContext := processcontrol.RestartContext{
				TriggerType:       processcontrol.RestartTriggerHealthFailure,
				Severity:          "critical",           // Health failures are always critical
				WorkerProfileType: pc.workerProfileType, // ✅ RENAMED field
				ViolationType:     "health",
				Message:           reason,
			}

			wrappedRestart := func() error {
				// Use a background context for restart since this is triggered by health failure
				ctx := context.Background()
				return pc.restartInternal(ctx, true) // health check failed -> true
			}
			// Use the unified ExecuteRestart method
			return pc.restartCircuitBreaker.ExecuteRestart(wrappedRestart, restartContext)
		})

		// Set up recovery callback to reset circuit breaker when process becomes healthy
		healthMonitor.SetRecoveryCallback(func() {
			pc.logger.Infof("Health recovered, resetting circuit breaker, worker: %s", pc.workerID)
			pc.restartCircuitBreaker.Reset()
		})
	}

	err := healthMonitor.Start(ctx)
	if err != nil {
		return nil, errors.NewInternalError("failed to start health monitoring", err).WithContext("id", pc.workerID)
	}

	pc.logger.Infof("Health monitor started, worker: %s", pc.workerID)

	return healthMonitor, nil
}

// Initialize resource monitoring (internal - no validation)
func (pc *processControl) startResourceMonitoring(ctx context.Context, pid int) (resourcelimits.ResourceLimitManager, error) {
	pc.logger.Debugf("Initializing resource monitoring for worker %s, PID: %d", pc.workerID, pid)

	// Create resource limit manager
	resourceManager := resourcelimits.NewResourceLimitManager(
		pc.process.Pid,
		pc.config.Limits,
		pc.logger,
	)

	// Set violation callback to integrate with ProcessControl restart logic
	resourceManager.SetViolationCallback(pc.handleResourceViolation)

	// Start monitoring
	err := resourceManager.Start(ctx)
	if err != nil {
		return nil, errors.NewInternalError("failed to start resource monitoring", err).WithContext("id", pc.workerID)
	}

	pc.logger.Infof("Resource monitoring started for worker %s, PID: %d", pc.workerID, pc.process.Pid)
	return resourceManager, nil
}

// Handle resource violations with context awareness
func (pc *processControl) handleResourceViolation(policy resourcelimits.ResourcePolicy, violation *resourcelimits.ResourceViolation) {
	pc.logger.Warnf("Resource violation detected for worker %s: %s", pc.workerID, violation.Message)

	switch policy {
	case resourcelimits.ResourcePolicyLog:
		pc.logger.Warnf("LOG: Resource limit exceeded (policy: log): %s", violation.Message)

	case resourcelimits.ResourcePolicyAlert:
		pc.logger.Errorf("ALERT: Resource limit exceeded: %s", violation.Message)
		// Note: Alerting system integration planned for Phase 5 (Enterprise Features)

	case resourcelimits.ResourcePolicyRestart:
		pc.logger.Errorf("RESTART: Resource limit exceeded, restarting process (policy: restart): %s", violation.Message)
		// Use context-aware circuit breaker for resource violations
		if pc.restartCircuitBreaker != nil {
			// Create context for resource violation restart
			restartContext := processcontrol.RestartContext{
				TriggerType:       processcontrol.RestartTriggerResourceViolation,
				Severity:          string(violation.Severity),  // warning/critical/emergency
				WorkerProfileType: pc.workerProfileType,        // ✅ RENAMED field
				ViolationType:     string(violation.LimitType), // memory/cpu/process
				Message:           violation.Message,
			}

			wrappedRestart := func() error {
				ctx := context.Background()
				return pc.restartInternal(ctx, false) // normally, running process -> false
			}
			// Use the unified ExecuteRestart method
			if err := pc.restartCircuitBreaker.ExecuteRestart(wrappedRestart, restartContext); err != nil {
				pc.logger.Errorf("Failed to restart process after resource violation (circuit breaker): %v", err)
			}
		} else {
			// Fallback if no circuit breaker (shouldn't happen in normal operation)
			ctx := context.Background()
			err := pc.restartInternal(ctx, false) // normally, running process -> false
			if err != nil {
				pc.logger.Errorf("Failed to restart process after resource violation: %v", err)
			}
		}

	case resourcelimits.ResourcePolicyGracefulShutdown:
		pc.logger.Errorf("GRACEFUL SHUTDOWN: Resource limit exceeded, performing graceful shutdown (policy: graceful_shutdown): %s", violation.Message)
		// handleResourceViolation callback always runs in goroutine, use unified termination method with graceful policy
		ctx := context.Background()
		if err := pc.terminateProcessWithPolicy(ctx, resourcelimits.ResourcePolicyGracefulShutdown, violation.Message); err != nil {
			pc.logger.Errorf("Failed to gracefully shutdown process after resource violation: %v", err)
		}

	case resourcelimits.ResourcePolicyImmediateKill:
		pc.logger.Errorf("IMMEDIATE KILL: Resource limit exceeded, killing process immediately (policy: immediate_kill): %s", violation.Message)
		// handleResourceViolation callback always runs in goroutine, use unified termination method with kill policy
		ctx := context.Background()
		if err := pc.terminateProcessWithPolicy(ctx, resourcelimits.ResourcePolicyImmediateKill, violation.Message); err != nil {
			pc.logger.Errorf("Failed to kill process after resource violation: %v", err)
		}

	default:
		pc.logger.Warnf("Unknown resource policy: %s", policy)
	}
}

// ✅ DEFER-ONLY: stopInternal now uses automatic unlock only!
func (pc *processControl) stopInternal(ctx context.Context, idDeadPID bool) error {
	pc.logger.Infof("Stopping process control...")

	// Phase 1: State validation and planning (defer-only lock)
	plan := pc.validateAndPlanStop()
	if !plan.shouldProceed {
		return plan.errorToReturn // Could be nil for fast-path
	}

	// Phase 2: Termination outside lock (reuse the existing logic)
	var terminationError error
	if plan.processToTerminate != nil {
		if err := pc.terminateProcessExternal(ctx, plan.processToTerminate, plan.processDoneSignal, idDeadPID); err != nil {
			pc.logger.Errorf("Failed to terminate process: %v", err)
			terminationError = errors.NewProcessError("failed to terminate process", err)
		}
	}

	// Phase 3: Final cleanup and state transition (defer-only lock)
	pc.finalizeStop()

	if terminationError != nil {
		return terminationError
	}

	pc.logger.Infof("Process control stopped successfully")
	return nil
}

// canStopFromState validates if stopping is allowed from the current state
func (pc *processControl) canStopFromState(currentState processcontrol.ProcessState) bool {
	switch currentState {
	case processcontrol.ProcessStateIdle:
		return true // ✅ Can stop from idle (no-op)
	case processcontrol.ProcessStateStarting:
		return false // ❌ Wait for startup to complete
	case processcontrol.ProcessStateRunning:
		return true // ✅ Can stop from running
	case processcontrol.ProcessStateStopping:
		return false // ❌ Already stopping
	case processcontrol.ProcessStateTerminating:
		return false // ❌ Already terminating
	default:
		return false // ❌ Unknown state
	}
}

// restartInternal performs the actual restart (simplified)
func (pc *processControl) restartInternal(ctx context.Context, idDeadPID bool) error {
	pc.logger.Infof("Restarting process control, worker: %s", pc.workerID)

	// 1. Stop the current process (with state validation)
	if err := pc.stopInternal(ctx, idDeadPID); err != nil {
		pc.logger.Errorf("Failed to stop process during restart: %v", err)
		return fmt.Errorf("failed to stop process during restart: %v", err)
	}

	// 2. Start the process again (with state validation)
	// Note: stopInternal() sets state to processcontrol.ProcessStateIdle, so startInternal() will succeed
	if err := pc.startInternal(ctx); err != nil {
		pc.logger.Errorf("Failed to start process during restart: %v", err)
		return fmt.Errorf("failed to start process during restart: %v", err)
	}

	pc.logger.Infof("Process control restarted successfully, worker: %s", pc.workerID)
	return nil
}

// terminateProcessExternal handles graceful process termination with timeout and context cancellation
// This version works with an external process reference to avoid holding locks during long operations
func (pc *processControl) terminateProcessExternal(ctx context.Context, proc *os.Process, done chan error, idDeadPID bool) error {
	if proc == nil {
		return errors.NewProcessError("no process to terminate", nil)
	}

	pid := proc.Pid
	pc.logger.Infof("Terminating process PID %d", pid)

	// Determine graceful timeout
	gracefulTimeout := pc.config.GracefulTimeout
	if gracefulTimeout <= 0 {
		gracefulTimeout = 20 * time.Second // Timeout super-default
	}

	// Try graceful termination first
	pc.logger.Infof("Sending termination signal to PID %d, idDead: %t, timeout: %v", pid, idDeadPID, gracefulTimeout)
	// Use the platform-specific termination signal
	// On Unix: SIGTERM to process group
	// On Windows: Ctrl-Break event
	if err := process.SendTerminationSignal(pid, idDeadPID, gracefulTimeout); err != nil {
		pc.logger.Warnf("Failed to send termination signal for PID %d: %v", pid, err)
	}

	pc.logger.Infof("Waiting for process PID %d to terminate gracefully", pid)

	// Wait for graceful shutdown, timeout, or context cancellation
	select {
	case err := <-done:
		if err != nil {
			return errors.NewProcessError("process termination failed", err).WithContext("pid", pid)
		}
		pc.logger.Infof("Process PID %d terminated gracefully", pid)
		return nil
	case <-time.After(gracefulTimeout):
		pc.logger.Warnf("Process PID %d did not terminate within %v, forcing termination", pid, gracefulTimeout)
	case <-ctx.Done():
		pc.logger.Warnf("Context cancelled during graceful termination of PID %d, forcing termination", pid)
	}

	// Force termination if graceful didn't work
	pc.logger.Warnf("Force killing process PID %d", pid)

	// Use Kill() which sends SIGKILL on Unix and TerminateProcess on Windows
	if err := proc.Kill(); err != nil {
		return errors.NewProcessError("failed to kill process", err).WithContext("pid", pid)
	}

	// Wait for forced termination (with shorter timeout) or context cancellation
	select {
	case err := <-done:
		if err != nil {
			return errors.NewProcessError("forced termination failed", err).WithContext("pid", pid)
		}
		pc.logger.Infof("Process PID %d force terminated", pid)
		return nil
	case <-time.After(5 * time.Second):
		return errors.NewTimeoutError("process did not terminate even after force termination", nil).WithContext("pid", pid)
	case <-ctx.Done():
		pc.logger.Warnf("Context cancelled during force termination of PID %d", pid)
		return errors.NewCancelledError("termination cancelled", ctx.Err()).WithContext("pid", pid)
	}
}

// terminateProcessWithPolicy handles termination with state management and flexible policies
// This method consolidates termination logic for both normal stops and resource violations
// ✅ DEFER-ONLY: No explicit unlock calls - all automatic via defer!
func (pc *processControl) terminateProcessWithPolicy(ctx context.Context, policy resourcelimits.ResourcePolicy, reason string) error {
	pc.logger.Infof("Terminating process with policy %s, reason: %s, worker: %s", policy, reason, pc.workerID)

	// Phase 1: State validation and transition (defer-only lock)
	plan := pc.validateAndPlanTermination(policy, reason)
	if !plan.shouldProceed {
		return plan.errorToReturn // Could be nil for fast-path
	}

	// Phase 2: Termination outside lock
	var terminationError error
	if plan.processToTerminate != nil {
		if plan.skipGraceful {
			// Immediate kill
			if err := plan.processToTerminate.Kill(); err != nil {
				pc.logger.Warnf("Failed to kill process: %v", err)
				terminationError = err
			} else {
				pc.logger.Infof("Process killed immediately, worker: %s", pc.workerID)
			}
		} else {
			// Graceful termination with timeout (reuse existing logic)
			if err := pc.terminateProcessExternal(ctx, plan.processToTerminate, plan.processDoneSignal, false); err != nil {
				pc.logger.Errorf("Failed to terminate process gracefully: %v", err)
				terminationError = errors.NewProcessError("failed to terminate process", err)
			}
		}
	}

	// Phase 3: Final state transition and cleanup (defer-only lock)
	pc.finalizeTermination(plan)

	return terminationError
}

// cleanupResourcesUnderLock performs resource cleanup while holding the mutex
func (pc *processControl) cleanupResourcesUnderLock() {
	// Stop log collection
	if err := pc.stopLogCollection(); err != nil {
		pc.logger.Warnf("Error stopping log collection for worker %s: %v", pc.workerID, err)
	}

	// Stop resource monitoring
	if pc.resourceManager != nil {
		pc.resourceManager.Stop()
		pc.resourceManager = nil
		pc.logger.Debugf("Resource monitoring stopped for worker %s", pc.workerID)
	}

	// Stop health monitor
	if pc.healthMonitor != nil {
		pc.healthMonitor.Stop()
		pc.healthMonitor = nil
		pc.logger.Debugf("Health monitor stopped for worker %s", pc.workerID)
	}

	// Close stdout reader
	if pc.stdout != nil {
		if err := pc.stdout.Close(); err != nil {
			pc.logger.Warnf("Failed to close stdout: %v", err)
		}
		pc.stdout = nil
		pc.logger.Debugf("Stdout closed for worker %s", pc.workerID)
	}
}

// ===== DEFER-ONLY LOCKING HELPERS =====
// These functions encapsulate lock scopes with automatic unlock via defer

// terminationPlan holds data extracted under lock for termination operations
type terminationPlan struct {
	processToTerminate *os.Process
	processDoneSignal  chan error
	targetState        processcontrol.ProcessState
	skipGraceful       bool
	shouldProceed      bool
	errorToReturn      error
}

// validateAndPlanTermination validates state and creates termination plan (defer-only lock)
func (pc *processControl) validateAndPlanTermination(policy resourcelimits.ResourcePolicy, reason string) *terminationPlan {
	pc.mutex.Lock()
	defer pc.mutex.Unlock() // ✅ AUTOMATIC unlock - no fragility!

	plan := &terminationPlan{}

	// Fast-path: already stopped
	if pc.state == processcontrol.ProcessStateIdle {
		pc.logger.Infof("Process already stopped, worker: %s", pc.workerID)
		plan.shouldProceed = false
		return plan
	}

	// Determine target state and behavior based on policy
	switch policy {
	case resourcelimits.ResourcePolicyGracefulShutdown:
		plan.targetState = processcontrol.ProcessStateStopping
		plan.skipGraceful = false
	case resourcelimits.ResourcePolicyImmediateKill:
		plan.targetState = processcontrol.ProcessStateTerminating
		plan.skipGraceful = true
	default:
		// For unknown policies, default to graceful
		plan.targetState = processcontrol.ProcessStateStopping
		plan.skipGraceful = false
	}

	// Set appropriate state to block other operations
	pc.state = plan.targetState
	pc.logger.Debugf("State transition: -> %s (%s), worker: %s", plan.targetState, policy, pc.workerID)

	// Get process reference for termination
	plan.processToTerminate = pc.process
	plan.processDoneSignal = pc.processDoneSignal

	// For immediate kill, do full cleanup under same lock
	if plan.skipGraceful {
		pc.cleanupResourcesUnderLock()
	}

	// Clear process reference to prevent further operations
	pc.process = nil
	pc.processDoneSignal = nil

	plan.shouldProceed = true
	return plan
}

// finalizeTermination completes state transition after termination (defer-only lock)
func (pc *processControl) finalizeTermination(plan *terminationPlan) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock() // ✅ AUTOMATIC unlock - no fragility!

	// For graceful shutdown, do remaining cleanup under lock
	if !plan.skipGraceful {
		pc.cleanupResourcesUnderLock()
	}

	pc.state = processcontrol.ProcessStateIdle
	pc.logger.Debugf("State transition: %s -> idle, worker: %s", plan.targetState, pc.workerID)
}

// stopPlan holds data extracted under lock for stop operations
type stopPlan struct {
	processToTerminate *os.Process
	processDoneSignal  chan error
	shouldProceed      bool
	errorToReturn      error
}

// validateAndPlanStop validates state and creates stop plan (defer-only lock)
func (pc *processControl) validateAndPlanStop() *stopPlan {
	pc.mutex.Lock()
	defer pc.mutex.Unlock() // ✅ AUTOMATIC unlock - no fragility!

	plan := &stopPlan{}

	// Validate state transition before proceeding
	if !pc.canStopFromState(pc.state) {
		plan.shouldProceed = false
		plan.errorToReturn = errors.NewValidationError(
			fmt.Sprintf("cannot stop process in state '%s': operation not allowed", pc.state),
			nil).WithContext("worker", pc.workerID).WithContext("current_state", string(pc.state))
		return plan
	}

	// Fast-path: already stopped
	if pc.state == processcontrol.ProcessStateIdle {
		pc.logger.Infof("Process already stopped, worker: %s", pc.workerID)
		plan.shouldProceed = false
		return plan
	}

	// Set stopping state immediately to block new operations
	pc.state = processcontrol.ProcessStateStopping
	pc.logger.Debugf("State transition: -> stopping, worker: %s", pc.workerID)

	// Get process reference for termination
	plan.processToTerminate = pc.process
	plan.processDoneSignal = pc.processDoneSignal

	// Clear process reference to prevent further operations
	pc.process = nil
	pc.processDoneSignal = nil

	plan.shouldProceed = true
	return plan
}

// finalizeStop completes stop operation with cleanup (defer-only lock)
func (pc *processControl) finalizeStop() {
	pc.mutex.Lock()
	defer pc.mutex.Unlock() // ✅ AUTOMATIC unlock - no fragility!

	// Use shared cleanup logic
	pc.cleanupResourcesUnderLock()

	pc.state = processcontrol.ProcessStateIdle
	pc.logger.Debugf("State transition: stopping -> idle, worker: %s", pc.workerID)
}

// safeGetState gets current state with defer-only read lock (replaces the existing GetState)
func (pc *processControl) safeGetState() processcontrol.ProcessState {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock() // ✅ AUTOMATIC unlock - no fragility!
	return pc.state
}

// safeGetProcess safely gets process reference with defer-only read lock
func (pc *processControl) safeGetProcess() *os.Process {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock() // ✅ AUTOMATIC unlock - no fragility!
	return pc.process
}

// ===== LOG COLLECTION INTEGRATION =====

// startLogCollection starts log collection for the process if service is available
func (pc *processControl) startLogCollection(ctx context.Context, process *os.Process, stdout io.ReadCloser) error {
	// Check if log collection service is available
	if pc.config.LogCollectionService == nil {
		pc.logger.Debugf("No log collection service configured for worker %s", pc.workerID)
		return nil
	}

	// Check if log collection config is available
	if pc.config.LogConfig == nil {
		pc.logger.Debugf("No log collection config for worker %s", pc.workerID)
		return nil
	}

	// Register worker with log collection service
	if err := pc.config.LogCollectionService.RegisterWorker(pc.workerID, *pc.config.LogConfig); err != nil {
		return fmt.Errorf("failed to register worker for log collection: %w", err)
	}

	// For managed processes, we have direct access to stdout and can create stderr access
	// Collect from the stdout stream we have
	if pc.config.LogConfig.CaptureStdout && stdout != nil {
		if err := pc.config.LogCollectionService.CollectFromStream(pc.workerID, stdout, logcollection.StdoutStream); err != nil {
			return fmt.Errorf("failed to start stdout collection: %w", err)
		}
	}

	// Note: For stderr collection, we'd need to modify the startProcess method
	// to return both stdout and stderr streams. For now, we collect stdout only.

	pc.logCollectionActive = true
	pc.logger.Infof("Log collection started for worker %s", pc.workerID)

	return nil
}

// stopLogCollection stops log collection for the worker
func (pc *processControl) stopLogCollection() error {
	if !pc.logCollectionActive || pc.config.LogCollectionService == nil {
		return nil
	}

	if err := pc.config.LogCollectionService.UnregisterWorker(pc.workerID); err != nil {
		return fmt.Errorf("failed to unregister worker from log collection: %w", err)
	}

	pc.logCollectionActive = false
	pc.logger.Infof("Log collection stopped for worker %s", pc.workerID)

	return nil
}

// categorizeProcessError attempts to categorize an error based on its content and type
func categorizeProcessError(err error) *processcontrol.ProcessError {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())
	var category string
	var recoverable bool

	// Check for common error patterns from multiple sources:
	// - OS errors: "no such file or directory" (Unix), "cannot find the file" (Windows)
	// - Go stdlib: "executable file not found in $PATH"
	// Note: Our ensureExecutable wraps OS errors in DomainError, but the original OS error
	//       is preserved in DomainError.Cause and included in the final error message,
	//       so checking for OS patterns like "no such file" still works correctly.
	switch {
	case strings.Contains(errStr, "no such file") ||
		strings.Contains(errStr, "executable file not found"):
		category = processcontrol.ErrorCategoryExecutableNotFound
		recoverable = true
	case strings.Contains(errStr, "permission denied") || strings.Contains(errStr, "access denied"):
		category = processcontrol.ErrorCategoryPermissionDenied
		recoverable = true
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
		category = processcontrol.ErrorCategoryTimeout
		recoverable = true
	case strings.Contains(errStr, "out of memory") || strings.Contains(errStr, "resource limit"):
		category = processcontrol.ErrorCategoryResourceLimit
		recoverable = true
	case strings.Contains(errStr, "network") || strings.Contains(errStr, "connection"):
		category = processcontrol.ErrorCategoryNetworkIssue
		recoverable = true
	case strings.Contains(errStr, "signal:") || strings.Contains(errStr, "exit status"):
		category = processcontrol.ErrorCategoryProcessCrash
		recoverable = false
	default:
		category = processcontrol.ErrorCategoryUnknown
		recoverable = false
	}

	return &processcontrol.ProcessError{
		Category:    category,
		Details:     err.Error(),
		Underlying:  err,
		Timestamp:   time.Now(),
		Recoverable: recoverable,
	}
}
