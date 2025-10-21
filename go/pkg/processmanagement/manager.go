package processmanagement

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logcollection"
	logconfig "github.com/core-tools/hsu-core/pkg/logcollection/config"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/managedprocess"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrolimpl"
	"github.com/core-tools/hsu-core/pkg/processmanagement/processstatemachine"
)

type ProcessRegistry interface {
	AddProcess(process managedprocess.ProcessOptions) error
	RemoveProcess(id string) error
}

type LogCollectorIntegration interface {
	SetLogCollectionService(service logcollection.LogCollectionService)
}

type ProcessLifecycle interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	StartProcess(ctx context.Context, id string) error
	StopProcess(ctx context.Context, id string) error
	GetManagerState() ProcessManagerState
	GetAllProcessStatesWithDiagnostics() map[string]ProcessStateWithDiagnostics
	GetProcessState(id string) (processstatemachine.ProcessState, error)
	GetProcessContext(id string) (map[string]string, error)
	GetProcessStateWithDiagnostics(id string) (ProcessStateWithDiagnostics, error)
	IsProcessOperationAllowed(id string, operation string) (bool, error)
	GetProcessProcessDiagnostics(id string) (processcontrol.ProcessDiagnostics, error)
}

type ProcessManager interface {
	ProcessRegistry
	ProcessLifecycle
	LogCollectorIntegration
}

type ProcessManagerOptions struct {
	ForceShutdownTimeout time.Duration
}

// ProcessManagerState represents the current state of the process manager
type ProcessManagerState string

const (
	// ProcessManagerStateNotStarted is the initial state before Run() is called
	ProcessManagerStateNotStarted ProcessManagerState = "not_started"

	// ProcessManagerStateRunning means process manager is running and can manage processes
	ProcessManagerStateRunning ProcessManagerState = "running"

	// ProcessManagerStateStopping means process manager is shutting down
	ProcessManagerStateStopping ProcessManagerState = "stopping"

	// ProcessManagerStateStopped means process manager has stopped
	ProcessManagerStateStopped ProcessManagerState = "stopped"
)

// processEntry combines ProcessControl and StateMachine for a process
type processEntry struct {
	ProcessControl processcontrol.ProcessControl
	StateMachine   *processstatemachine.ProcessStateMachine
}

type processManager struct {
	options              ProcessManagerOptions
	processes            map[string]*processEntry // Combined map for controls and state machines
	state                ProcessManagerState      // Track process manager state
	mutex                sync.Mutex
	logCollectionService logcollection.LogCollectionService // Log collection service
	logger               logging.Logger
}

func NewProcessManager(options ProcessManagerOptions, logger logging.Logger) ProcessManager {
	return &processManager{
		options:   options,
		logger:    logger,
		processes: make(map[string]*processEntry),
		state:     ProcessManagerStateNotStarted,
		mutex:     sync.Mutex{},
	}
}

func (pm *processManager) AddProcess(process managedprocess.ProcessOptions) error {
	// Validate input
	if process == nil {
		return errors.NewValidationError("process cannot be nil", nil)
	}

	id := process.ID()

	// Validate process ID
	if err := ValidateProcessID(id); err != nil {
		return errors.NewValidationError("invalid process ID", err).WithContext("process_id", id)
	}

	// Validate process options
	options := process.ProcessControlOptions()
	if err := managedprocess.ValidateProcessControlOptions(options); err != nil {
		return errors.NewValidationError("invalid process process control options", err).WithContext("process_id", id)
	}

	pm.logger.Infof("Adding process, id: %s, can_attach: %t, can_execute: %t, can_terminate: %t, can_restart: %t",
		id, options.CanAttach, (options.ExecuteCmd != nil), options.CanTerminate, options.CanRestart)

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check if process already exists
	if _, exists := pm.processes[id]; exists {
		return errors.NewConflictError("process already exists", nil).WithContext("process_id", id)
	}

	// Create state machine for the process
	stateMachine := processstatemachine.NewProcessStateMachine(id, pm.logger)

	// Validate that add operation is allowed
	if err := stateMachine.ValidateOperation("add"); err != nil {
		return err
	}

	// Transition to registered state
	if err := stateMachine.Transition(processstatemachine.ProcessStateRegistered, "add", nil); err != nil {
		return errors.NewInternalError("failed to transition process to registered state", err).WithContext("process_id", id)
	}

	// Create process control
	processControl := processcontrolimpl.NewProcessControl(options, id, pm.logger)

	// Store process and state machine
	pm.processes[id] = &processEntry{
		ProcessControl: processControl,
		StateMachine:   stateMachine,
	}

	pm.logger.Infof("Managed process added successfully, id: %s, state: %s", id, stateMachine.GetCurrentState())
	return nil
}

func (pm *processManager) RemoveProcess(id string) error {
	// Validate process ID
	if err := ValidateProcessID(id); err != nil {
		return errors.NewValidationError("invalid process ID", err).WithContext("process_id", id)
	}

	pm.logger.Infof("Removing process, id: %s", id)

	// Get process and check if it's safely removable
	processEntry, _, exists := pm.getProcessAndManagerState(id)
	if !exists {
		return errors.NewNotFoundError("process not found", nil).WithContext("process_id", id)
	}

	// Check if process is in a safe state for removal
	currentState := processEntry.StateMachine.GetCurrentState()
	if !isProcessSafelyRemovable(currentState) {
		return errors.NewValidationError(
			fmt.Sprintf("cannot remove process in state '%s': process must be stopped before removal", currentState),
			nil,
		).WithContext("process_id", id).
			WithContext("current_state", string(currentState)).
			WithContext("required_states", "stopped, failed").
			WithContext("suggested_action", "call StopProcess first")
	}

	// Safe to remove - acquire lock and remove
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Double-check existence under lock (could have been removed by another goroutine)
	if _, exists := pm.processes[id]; !exists {
		return errors.NewNotFoundError("process not found", nil).WithContext("process_id", id)
	}

	// Remove process and state machine
	delete(pm.processes, id)

	pm.logger.Infof("Managed process removed successfully, id: %s", id)
	return nil
}

// isProcessSafelyRemovable checks if a process is in a state safe for removal
func isProcessSafelyRemovable(state processstatemachine.ProcessState) bool {
	switch state {
	case processstatemachine.ProcessStateStopped, processstatemachine.ProcessStateFailed:
		return true // Safe to remove - process is not running
	case processstatemachine.ProcessStateUnknown, processstatemachine.ProcessStateRegistered:
		return true // Safe to remove - no process started yet
	case processstatemachine.ProcessStateStarting, processstatemachine.ProcessStateRunning, processstatemachine.ProcessStateStopping, processstatemachine.ProcessStateRestarting:
		return false // Unsafe - process may be running
	default:
		return false // Unknown state - be conservative
	}
}

func (pm *processManager) StartProcess(ctx context.Context, id string) error {
	// Validate context
	if ctx == nil {
		return errors.NewValidationError("context cannot be nil", nil)
	}

	// Validate process ID
	if err := ValidateProcessID(id); err != nil {
		return errors.NewValidationError("invalid process ID", err).WithContext("process_id", id)
	}

	// Get process and process manager state safely
	processEntry, currentState, exists := pm.getProcessAndManagerState(id)

	if !exists {
		return errors.NewNotFoundError("process not found", nil).WithContext("process_id", id)
	}

	// Validate process manager is running - after process existence check
	if currentState != ProcessManagerStateRunning {
		return errors.NewValidationError(
			fmt.Sprintf("process manager must be running to start processes, current state: %s", currentState),
			nil,
		).WithContext("process_id", id).WithContext("state", string(currentState))
	}

	pm.logger.Infof("Starting process, id: %s", id)

	// 2. Validate operation using state machine (outside lock)
	if err := processEntry.StateMachine.ValidateOperation("start"); err != nil {
		return err
	}

	// 3. Transition to starting state
	if err := processEntry.StateMachine.Transition(processstatemachine.ProcessStateStarting, "start", nil); err != nil {
		return errors.NewInternalError("failed to transition process to starting state", err).WithContext("process_id", id)
	}

	// 4. Start process control (outside of lock, can be long-running)
	err := processEntry.ProcessControl.Start(ctx)

	// 5. Update state based on result
	if err != nil {
		// Transition to failed state
		transitionErr := processEntry.StateMachine.Transition(processstatemachine.ProcessStateFailed, "start", err)
		if transitionErr != nil {
			pm.logger.Errorf("Failed to transition process to failed state, id: %s, error: %v", id, transitionErr)
		}

		pm.logger.Errorf("Failed to start process, id: %s, error: %v", id, err)

		// Check if the error is a context cancellation
		if ctx.Err() != nil {
			return errors.NewCancelledError("process start was cancelled", ctx.Err()).WithContext("process_id", id)
		}
		return errors.NewProcessError("failed to start process", err).WithContext("process_id", id)
	}

	// 6. Transition to running state on success
	if err := processEntry.StateMachine.Transition(processstatemachine.ProcessStateRunning, "start", nil); err != nil {
		pm.logger.Errorf("Failed to transition process to running state, id: %s, error: %v", id, err)
		// Note: Process is actually running, but state tracking failed
	}

	pm.logger.Infof("Managed process started successfully, id: %s, state: %s", id, processEntry.StateMachine.GetCurrentState())
	return nil
}

func (pm *processManager) StopProcess(ctx context.Context, id string) error {
	// Validate context
	if ctx == nil {
		return errors.NewValidationError("context cannot be nil", nil)
	}

	// Validate process ID
	if err := ValidateProcessID(id); err != nil {
		return errors.NewValidationError("invalid process ID", err).WithContext("process_id", id)
	}

	// Get process and process manager state safely
	processEntry, currentState, exists := pm.getProcessAndManagerState(id)

	if !exists {
		return errors.NewNotFoundError("process not found", nil).WithContext("process_id", id)
	}

	// Validate process manager is running - after process existence check
	if currentState != ProcessManagerStateRunning {
		return errors.NewValidationError(
			fmt.Sprintf("process manager must be running to stop processes, current state: %s", currentState),
			nil,
		).WithContext("process_id", id).WithContext("state", string(currentState))
	}

	pm.logger.Infof("Stopping process, id: %s", id)

	// 2. Validate operation using state machine (outside lock)
	if err := processEntry.StateMachine.ValidateOperation("stop"); err != nil {
		return err
	}

	// 3. Transition to stopping state
	if err := processEntry.StateMachine.Transition(processstatemachine.ProcessStateStopping, "stop", nil); err != nil {
		return errors.NewInternalError("failed to transition process to stopping state", err).WithContext("process_id", id)
	}

	// 4. Stop process control (outside of lock, can be long-running)
	err := processEntry.ProcessControl.Stop(ctx)

	// 5. Update state based on result
	if err != nil {
		// Transition to failed state
		transitionErr := processEntry.StateMachine.Transition(processstatemachine.ProcessStateFailed, "stop", err)
		if transitionErr != nil {
			pm.logger.Errorf("Failed to transition process to failed state, id: %s, error: %v", id, transitionErr)
		}

		pm.logger.Errorf("Failed to stop process, id: %s, error: %v", id, err)

		// Check if the error is a context cancellation
		if ctx.Err() != nil {
			return errors.NewCancelledError("process stop was cancelled", ctx.Err()).WithContext("process_id", id)
		}
		return errors.NewProcessError("failed to stop process", err).WithContext("process_id", id)
	}

	// 6. Transition to stopped state on success
	if err := processEntry.StateMachine.Transition(processstatemachine.ProcessStateStopped, "stop", nil); err != nil {
		pm.logger.Errorf("Failed to transition process to stopped state, id: %s, error: %v", id, err)
		// Note: Process is actually stopped, but state tracking failed
	}

	pm.logger.Infof("Managed process stopped successfully, id: %s, state: %s", id, processEntry.StateMachine.GetCurrentState())
	return nil
}

func (pm *processManager) Start(ctx context.Context) error {
	pm.logger.Infof("Starting process manager...")

	// Transition process manager to running state
	pm.setManagerState(ProcessManagerStateRunning)

	pm.logger.Infof("Process manager started")

	return nil
}

func (pm *processManager) Stop(ctx context.Context) error {
	pm.logger.Infof("Stopping process manager...")

	// Transition to stopping state
	pm.setManagerState(ProcessManagerStateStopping)

	if ctx == nil {
		ctx = context.Background()
	}

	forcedShutdownTimeout := pm.options.ForceShutdownTimeout
	if forcedShutdownTimeout <= 0 {
		forcedShutdownTimeout = 30 * time.Second // Timeout super-default
	}

	// Set forced shutdown timeout, it will be used for both server and processes
	ctx, _ = context.WithTimeout(ctx, forcedShutdownTimeout)

	// Stop processes
	err := pm.stopProcessControls(ctx)

	// Transition to stopped state
	pm.setManagerState(ProcessManagerStateStopped)

	pm.logger.Infof("Process manager stopped")

	return err
}

// GetAllProcessStatesWithDiagnostics returns comprehensive state and diagnostic information for all processes
func (pm *processManager) GetAllProcessStatesWithDiagnostics() map[string]ProcessStateWithDiagnostics {
	processEntriesCopy := pm.getAllProcesses()

	result := make(map[string]ProcessStateWithDiagnostics)
	for id, processEntry := range processEntriesCopy {
		processDiagnostics := processEntry.ProcessControl.GetDiagnostics()
		processStateInfo := processEntry.StateMachine.GetStateInfo()

		result[id] = ProcessStateWithDiagnostics{
			ProcessStateInfo:   processStateInfo,
			ProcessDiagnostics: processDiagnostics,
		}
	}
	return result
}

// GetProcessState returns the current state of a process
func (pm *processManager) GetProcessState(id string) (processstatemachine.ProcessState, error) {
	// Validate process ID
	if err := ValidateProcessID(id); err != nil {
		return processstatemachine.ProcessStateUnknown, errors.NewValidationError("invalid process ID", err).WithContext("process_id", id)
	}

	processEntry, _, exists := pm.getProcessAndManagerState(id)

	if !exists {
		return processstatemachine.ProcessStateUnknown, errors.NewNotFoundError("process not found", nil).WithContext("process_id", id)
	}

	return processEntry.StateMachine.GetCurrentState(), nil
}

func (pm *processManager) GetProcessContext(id string) (map[string]string, error) {
	// Validate process ID
	if err := ValidateProcessID(id); err != nil {
		return nil, errors.NewValidationError("invalid process ID", err).WithContext("process_id", id)
	}

	processEntry, _, exists := pm.getProcessAndManagerState(id)
	if !exists {
		return nil, errors.NewNotFoundError("process not found", nil).WithContext("process_id", id)
	}

	return processEntry.ProcessControl.GetContext(), nil
}

// GetProcessStateWithDiagnostics returns comprehensive state and diagnostic information for a process
func (pm *processManager) GetProcessStateWithDiagnostics(id string) (ProcessStateWithDiagnostics, error) {
	// Validate process ID
	if err := ValidateProcessID(id); err != nil {
		return ProcessStateWithDiagnostics{}, errors.NewValidationError("invalid process ID", err).WithContext("process_id", id)
	}

	processEntry, _, exists := pm.getProcessAndManagerState(id)

	if !exists {
		return ProcessStateWithDiagnostics{}, errors.NewNotFoundError("process not found", nil).WithContext("process_id", id)
	}

	processDiagnostics := processEntry.ProcessControl.GetDiagnostics()
	processStateInfo := processEntry.StateMachine.GetStateInfo()

	return ProcessStateWithDiagnostics{
		ProcessStateInfo:   processStateInfo,
		ProcessDiagnostics: processDiagnostics,
	}, nil
}

// IsProcessOperationAllowed checks if an operation is allowed for a process
func (pm *processManager) IsProcessOperationAllowed(id string, operation string) (bool, error) {
	// Validate process ID
	if err := ValidateProcessID(id); err != nil {
		return false, errors.NewValidationError("invalid process ID", err).WithContext("process_id", id)
	}

	processEntry, _, exists := pm.getProcessAndManagerState(id)

	if !exists {
		return false, errors.NewNotFoundError("process not found", nil).WithContext("process_id", id)
	}

	return processEntry.StateMachine.IsOperationAllowed(operation), nil
}

// GetManagerState returns the current state of the process manager
func (pm *processManager) GetManagerState() ProcessManagerState {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	return pm.state
}

// ProcessStateWithDiagnostics combines process state info with process diagnostics
type ProcessStateWithDiagnostics struct {
	processstatemachine.ProcessStateInfo
	ProcessDiagnostics processcontrol.ProcessDiagnostics // Detailed process diagnostics (includes State)
}

// GetProcessProcessDiagnostics returns detailed process diagnostics for a process
func (pm *processManager) GetProcessProcessDiagnostics(id string) (processcontrol.ProcessDiagnostics, error) {
	// Validate process ID
	if err := ValidateProcessID(id); err != nil {
		return processcontrol.ProcessDiagnostics{}, errors.NewValidationError("invalid process ID", err).WithContext("process_id", id)
	}

	processEntry, _, exists := pm.getProcessAndManagerState(id)

	if !exists {
		return processcontrol.ProcessDiagnostics{}, errors.NewNotFoundError("process not found", nil).WithContext("process_id", id)
	}

	return processEntry.ProcessControl.GetDiagnostics(), nil
}

func (pm *processManager) stopProcessControls(ctx context.Context) error {
	pm.logger.Infof("Stopping process controls...")

	if ctx == nil {
		ctx = context.Background()
	}

	// 1. Get all process controls under lock
	processEntriesCopy := pm.getAllProcesses()

	// 2. Stop processes outside of lock
	errorCollection := errors.NewErrorCollection()
	for id, processEntry := range processEntriesCopy {
		err := processEntry.ProcessControl.Stop(ctx)
		if err != nil {
			pm.logger.Errorf("Failed to stop process control, id: %s, error: %v", id, err)
			// Add context to the error for better debugging
			contextualErr := errors.NewProcessError("failed to stop process control", err).WithContext("process_id", id)
			errorCollection.Add(contextualErr)
		}
	}

	if errorCollection.HasErrors() {
		pm.logger.Errorf("Some process controls failed to stop: %v", errorCollection.Error())
	}

	pm.logger.Infof("Process controls stopped.")

	return errorCollection.ToError()
}

// getAllProcesses returns a copy of all process entries under lock
func (pm *processManager) getAllProcesses() map[string]*processEntry {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	processEntriesCopy := make(map[string]*processEntry)
	for id, processEntry := range pm.processes {
		processEntriesCopy[id] = processEntry
	}
	return processEntriesCopy
}

// getProcessAndManagerState returns process entry and process manager state under lock
// Returns: processEntry, state, exists
func (pm *processManager) getProcessAndManagerState(id string) (*processEntry, ProcessManagerState, bool) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	processEntry, exists := pm.processes[id]
	return processEntry, pm.state, exists
}

// setManagerState sets the process manager state and releases the lock
func (pm *processManager) setManagerState(state ProcessManagerState) {
	pm.mutex.Lock()
	pm.state = state
	pm.mutex.Unlock()
}

// SetLogCollectionService sets the log collection service for the process manager
func (pm *processManager) SetLogCollectionService(service logcollection.LogCollectionService) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.logCollectionService = service
	pm.logger.Infof("Log collection service configured for process manager")
}

// getLogCollectionConfig creates a default log collection config for processes
func (pm *processManager) getLogCollectionConfig() *logconfig.ProcessLogConfig {
	if pm.logCollectionService == nil {
		return nil
	}

	// Create default process log configuration
	defaultConfig := logconfig.DefaultProcessLogConfig()
	return &defaultConfig
}
