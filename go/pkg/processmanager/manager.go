package processmanager

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
	"github.com/core-tools/hsu-core/pkg/processmanager/workerstatemachine"
)

type ProcessRegistry interface {
	AddWorker(worker managedprocess.ProcessDescription) error
	RemoveWorker(id string) error
}

type LogCollectorIntegration interface {
	SetLogCollectionService(service logcollection.LogCollectionService)
}

type ProcessLifecycle interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	StartWorker(ctx context.Context, id string) error
	StopWorker(ctx context.Context, id string) error
	GetManagerState() ProcessManagerState
	GetAllWorkerStatesWithDiagnostics() map[string]WorkerStateWithDiagnostics
	GetWorkerState(id string) (workerstatemachine.WorkerState, error)
	GetWorkerContext(id string) (map[string]string, error)
	GetWorkerStateWithDiagnostics(id string) (WorkerStateWithDiagnostics, error)
	IsWorkerOperationAllowed(id string, operation string) (bool, error)
	GetWorkerProcessDiagnostics(id string) (processcontrol.ProcessDiagnostics, error)
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

	// ProcessManagerStateRunning means process manager is running and can manage workers
	ProcessManagerStateRunning ProcessManagerState = "running"

	// ProcessManagerStateStopping means process manager is shutting down
	ProcessManagerStateStopping ProcessManagerState = "stopping"

	// ProcessManagerStateStopped means process manager has stopped
	ProcessManagerStateStopped ProcessManagerState = "stopped"
)

// workerEntry combines ProcessControl and StateMachine for a worker
type workerEntry struct {
	ProcessControl processcontrol.ProcessControl
	StateMachine   *workerstatemachine.WorkerStateMachine
}

type processManager struct {
	options              ProcessManagerOptions
	workers              map[string]*workerEntry // Combined map for controls and state machines
	state                ProcessManagerState     // Track process manager state
	mutex                sync.Mutex
	logCollectionService logcollection.LogCollectionService // Log collection service
	logger               logging.Logger
}

func NewProcessManager(options ProcessManagerOptions, logger logging.Logger) ProcessManager {
	return &processManager{
		options: options,
		logger:  logger,
		workers: make(map[string]*workerEntry),
		state:   ProcessManagerStateNotStarted,
		mutex:   sync.Mutex{},
	}
}

func (pm *processManager) AddWorker(worker managedprocess.ProcessDescription) error {
	// Validate input
	if worker == nil {
		return errors.NewValidationError("worker cannot be nil", nil)
	}

	id := worker.ID()

	// Validate worker ID
	if err := ValidateWorkerID(id); err != nil {
		return errors.NewValidationError("invalid worker ID", err).WithContext("worker_id", id)
	}

	// Validate worker options
	options := worker.ProcessControlOptions()
	if err := managedprocess.ValidateProcessControlOptions(options); err != nil {
		return errors.NewValidationError("invalid worker process control options", err).WithContext("worker_id", id)
	}

	pm.logger.Infof("Adding worker, id: %s, can_attach: %t, can_execute: %t, can_terminate: %t, can_restart: %t",
		id, options.CanAttach, (options.ExecuteCmd != nil), options.CanTerminate, options.CanRestart)

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Check if worker already exists
	if _, exists := pm.workers[id]; exists {
		return errors.NewConflictError("worker already exists", nil).WithContext("worker_id", id)
	}

	// Create state machine for the worker
	stateMachine := workerstatemachine.NewWorkerStateMachine(id, pm.logger)

	// Validate that add operation is allowed
	if err := stateMachine.ValidateOperation("add"); err != nil {
		return err
	}

	// Transition to registered state
	if err := stateMachine.Transition(workerstatemachine.WorkerStateRegistered, "add", nil); err != nil {
		return errors.NewInternalError("failed to transition worker to registered state", err).WithContext("worker_id", id)
	}

	// Create process control
	processControl := processcontrolimpl.NewProcessControl(options, id, pm.logger)

	// Store worker and state machine
	pm.workers[id] = &workerEntry{
		ProcessControl: processControl,
		StateMachine:   stateMachine,
	}

	pm.logger.Infof("Managed process added successfully, id: %s, state: %s", id, stateMachine.GetCurrentState())
	return nil
}

func (pm *processManager) RemoveWorker(id string) error {
	// Validate worker ID
	if err := ValidateWorkerID(id); err != nil {
		return errors.NewValidationError("invalid worker ID", err).WithContext("worker_id", id)
	}

	pm.logger.Infof("Removing worker, id: %s", id)

	// Get worker and check if it's safely removable
	workerEntry, _, exists := pm.getWorkerAndManagerState(id)
	if !exists {
		return errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
	}

	// Check if worker is in a safe state for removal
	currentState := workerEntry.StateMachine.GetCurrentState()
	if !isWorkerSafelyRemovable(currentState) {
		return errors.NewValidationError(
			fmt.Sprintf("cannot remove worker in state '%s': worker must be stopped before removal", currentState),
			nil,
		).WithContext("worker_id", id).
			WithContext("current_state", string(currentState)).
			WithContext("required_states", "stopped, failed").
			WithContext("suggested_action", "call StopWorker first")
	}

	// Safe to remove - acquire lock and remove
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// Double-check existence under lock (could have been removed by another goroutine)
	if _, exists := pm.workers[id]; !exists {
		return errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
	}

	// Remove worker and state machine
	delete(pm.workers, id)

	pm.logger.Infof("Managed process removed successfully, id: %s", id)
	return nil
}

// isWorkerSafelyRemovable checks if a worker is in a state safe for removal
func isWorkerSafelyRemovable(state workerstatemachine.WorkerState) bool {
	switch state {
	case workerstatemachine.WorkerStateStopped, workerstatemachine.WorkerStateFailed:
		return true // Safe to remove - process is not running
	case workerstatemachine.WorkerStateUnknown, workerstatemachine.WorkerStateRegistered:
		return true // Safe to remove - no process started yet
	case workerstatemachine.WorkerStateStarting, workerstatemachine.WorkerStateRunning, workerstatemachine.WorkerStateStopping, workerstatemachine.WorkerStateRestarting:
		return false // Unsafe - process may be running
	default:
		return false // Unknown state - be conservative
	}
}

func (pm *processManager) StartWorker(ctx context.Context, id string) error {
	// Validate context
	if ctx == nil {
		return errors.NewValidationError("context cannot be nil", nil)
	}

	// Validate worker ID
	if err := ValidateWorkerID(id); err != nil {
		return errors.NewValidationError("invalid worker ID", err).WithContext("worker_id", id)
	}

	// Get worker and process manager state safely
	workerEntry, currentState, exists := pm.getWorkerAndManagerState(id)

	if !exists {
		return errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
	}

	// Validate process manager is running - after worker existence check
	if currentState != ProcessManagerStateRunning {
		return errors.NewValidationError(
			fmt.Sprintf("process manager must be running to start workers, current state: %s", currentState),
			nil,
		).WithContext("worker_id", id).WithContext("state", string(currentState))
	}

	pm.logger.Infof("Starting worker, id: %s", id)

	// 2. Validate operation using state machine (outside lock)
	if err := workerEntry.StateMachine.ValidateOperation("start"); err != nil {
		return err
	}

	// 3. Transition to starting state
	if err := workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateStarting, "start", nil); err != nil {
		return errors.NewInternalError("failed to transition worker to starting state", err).WithContext("worker_id", id)
	}

	// 4. Start process control (outside of lock, can be long-running)
	err := workerEntry.ProcessControl.Start(ctx)

	// 5. Update state based on result
	if err != nil {
		// Transition to failed state
		transitionErr := workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateFailed, "start", err)
		if transitionErr != nil {
			pm.logger.Errorf("Failed to transition worker to failed state, id: %s, error: %v", id, transitionErr)
		}

		pm.logger.Errorf("Failed to start worker, id: %s, error: %v", id, err)

		// Check if the error is a context cancellation
		if ctx.Err() != nil {
			return errors.NewCancelledError("worker start was cancelled", ctx.Err()).WithContext("worker_id", id)
		}
		return errors.NewProcessError("failed to start worker", err).WithContext("worker_id", id)
	}

	// 6. Transition to running state on success
	if err := workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateRunning, "start", nil); err != nil {
		pm.logger.Errorf("Failed to transition worker to running state, id: %s, error: %v", id, err)
		// Note: Process is actually running, but state tracking failed
	}

	pm.logger.Infof("Managed process started successfully, id: %s, state: %s", id, workerEntry.StateMachine.GetCurrentState())
	return nil
}

func (pm *processManager) StopWorker(ctx context.Context, id string) error {
	// Validate context
	if ctx == nil {
		return errors.NewValidationError("context cannot be nil", nil)
	}

	// Validate worker ID
	if err := ValidateWorkerID(id); err != nil {
		return errors.NewValidationError("invalid worker ID", err).WithContext("worker_id", id)
	}

	// Get worker and process manager state safely
	workerEntry, currentState, exists := pm.getWorkerAndManagerState(id)

	if !exists {
		return errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
	}

	// Validate process manager is running - after worker existence check
	if currentState != ProcessManagerStateRunning {
		return errors.NewValidationError(
			fmt.Sprintf("process manager must be running to stop workers, current state: %s", currentState),
			nil,
		).WithContext("worker_id", id).WithContext("state", string(currentState))
	}

	pm.logger.Infof("Stopping worker, id: %s", id)

	// 2. Validate operation using state machine (outside lock)
	if err := workerEntry.StateMachine.ValidateOperation("stop"); err != nil {
		return err
	}

	// 3. Transition to stopping state
	if err := workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateStopping, "stop", nil); err != nil {
		return errors.NewInternalError("failed to transition worker to stopping state", err).WithContext("worker_id", id)
	}

	// 4. Stop process control (outside of lock, can be long-running)
	err := workerEntry.ProcessControl.Stop(ctx)

	// 5. Update state based on result
	if err != nil {
		// Transition to failed state
		transitionErr := workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateFailed, "stop", err)
		if transitionErr != nil {
			pm.logger.Errorf("Failed to transition worker to failed state, id: %s, error: %v", id, transitionErr)
		}

		pm.logger.Errorf("Failed to stop worker, id: %s, error: %v", id, err)

		// Check if the error is a context cancellation
		if ctx.Err() != nil {
			return errors.NewCancelledError("worker stop was cancelled", ctx.Err()).WithContext("worker_id", id)
		}
		return errors.NewProcessError("failed to stop worker", err).WithContext("worker_id", id)
	}

	// 6. Transition to stopped state on success
	if err := workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateStopped, "stop", nil); err != nil {
		pm.logger.Errorf("Failed to transition worker to stopped state, id: %s, error: %v", id, err)
		// Note: Process is actually stopped, but state tracking failed
	}

	pm.logger.Infof("Managed process stopped successfully, id: %s, state: %s", id, workerEntry.StateMachine.GetCurrentState())
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

	// Set forced shutdown timeout, it will be used for both server and workers
	ctx, _ = context.WithTimeout(ctx, forcedShutdownTimeout)

	// Stop workers
	err := pm.stopWorkerProcessControls(ctx)

	// Transition to stopped state
	pm.setManagerState(ProcessManagerStateStopped)

	pm.logger.Infof("Process manager stopped")

	return err
}

// GetAllWorkerStatesWithDiagnostics returns comprehensive state and diagnostic information for all workers
func (pm *processManager) GetAllWorkerStatesWithDiagnostics() map[string]WorkerStateWithDiagnostics {
	workerEntriesCopy := pm.getAllWorkers()

	result := make(map[string]WorkerStateWithDiagnostics)
	for id, workerEntry := range workerEntriesCopy {
		processDiagnostics := workerEntry.ProcessControl.GetDiagnostics()
		workerStateInfo := workerEntry.StateMachine.GetStateInfo()

		result[id] = WorkerStateWithDiagnostics{
			WorkerStateInfo:    workerStateInfo,
			ProcessDiagnostics: processDiagnostics,
		}
	}
	return result
}

// GetWorkerState returns the current state of a worker
func (pm *processManager) GetWorkerState(id string) (workerstatemachine.WorkerState, error) {
	// Validate worker ID
	if err := ValidateWorkerID(id); err != nil {
		return workerstatemachine.WorkerStateUnknown, errors.NewValidationError("invalid worker ID", err).WithContext("worker_id", id)
	}

	workerEntry, _, exists := pm.getWorkerAndManagerState(id)

	if !exists {
		return workerstatemachine.WorkerStateUnknown, errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
	}

	return workerEntry.StateMachine.GetCurrentState(), nil
}

func (pm *processManager) GetWorkerContext(id string) (map[string]string, error) {
	// Validate worker ID
	if err := ValidateWorkerID(id); err != nil {
		return nil, errors.NewValidationError("invalid worker ID", err).WithContext("worker_id", id)
	}

	workerEntry, _, exists := pm.getWorkerAndManagerState(id)
	if !exists {
		return nil, errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
	}

	return workerEntry.ProcessControl.GetContext(), nil
}

// GetWorkerStateWithDiagnostics returns comprehensive state and diagnostic information for a worker
func (pm *processManager) GetWorkerStateWithDiagnostics(id string) (WorkerStateWithDiagnostics, error) {
	// Validate worker ID
	if err := ValidateWorkerID(id); err != nil {
		return WorkerStateWithDiagnostics{}, errors.NewValidationError("invalid worker ID", err).WithContext("worker_id", id)
	}

	workerEntry, _, exists := pm.getWorkerAndManagerState(id)

	if !exists {
		return WorkerStateWithDiagnostics{}, errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
	}

	processDiagnostics := workerEntry.ProcessControl.GetDiagnostics()
	workerStateInfo := workerEntry.StateMachine.GetStateInfo()

	return WorkerStateWithDiagnostics{
		WorkerStateInfo:    workerStateInfo,
		ProcessDiagnostics: processDiagnostics,
	}, nil
}

// IsWorkerOperationAllowed checks if an operation is allowed for a worker
func (pm *processManager) IsWorkerOperationAllowed(id string, operation string) (bool, error) {
	// Validate worker ID
	if err := ValidateWorkerID(id); err != nil {
		return false, errors.NewValidationError("invalid worker ID", err).WithContext("worker_id", id)
	}

	workerEntry, _, exists := pm.getWorkerAndManagerState(id)

	if !exists {
		return false, errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
	}

	return workerEntry.StateMachine.IsOperationAllowed(operation), nil
}

// GetManagerState returns the current state of the process manager
func (pm *processManager) GetManagerState() ProcessManagerState {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	return pm.state
}

// WorkerStateWithDiagnostics combines worker state info with process diagnostics
type WorkerStateWithDiagnostics struct {
	workerstatemachine.WorkerStateInfo
	ProcessDiagnostics processcontrol.ProcessDiagnostics // Detailed process diagnostics (includes State)
}

// GetWorkerProcessDiagnostics returns detailed process diagnostics for a worker
func (pm *processManager) GetWorkerProcessDiagnostics(id string) (processcontrol.ProcessDiagnostics, error) {
	// Validate worker ID
	if err := ValidateWorkerID(id); err != nil {
		return processcontrol.ProcessDiagnostics{}, errors.NewValidationError("invalid worker ID", err).WithContext("worker_id", id)
	}

	workerEntry, _, exists := pm.getWorkerAndManagerState(id)

	if !exists {
		return processcontrol.ProcessDiagnostics{}, errors.NewNotFoundError("worker not found", nil).WithContext("worker_id", id)
	}

	return workerEntry.ProcessControl.GetDiagnostics(), nil
}

func (pm *processManager) stopWorkerProcessControls(ctx context.Context) error {
	pm.logger.Infof("Stopping process controls...")

	if ctx == nil {
		ctx = context.Background()
	}

	// 1. Get all process controls under lock
	workerEntriesCopy := pm.getAllWorkers()

	// 2. Stop processes outside of lock
	errorCollection := errors.NewErrorCollection()
	for id, workerEntry := range workerEntriesCopy {
		err := workerEntry.ProcessControl.Stop(ctx)
		if err != nil {
			pm.logger.Errorf("Failed to stop process control, id: %s, error: %v", id, err)
			// Add context to the error for better debugging
			contextualErr := errors.NewProcessError("failed to stop process control", err).WithContext("worker_id", id)
			errorCollection.Add(contextualErr)
		}
	}

	if errorCollection.HasErrors() {
		pm.logger.Errorf("Some process controls failed to stop: %v", errorCollection.Error())
	}

	pm.logger.Infof("Process controls stopped.")

	return errorCollection.ToError()
}

// getAllWorkers returns a copy of all worker entries under lock
func (pm *processManager) getAllWorkers() map[string]*workerEntry {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	workerEntriesCopy := make(map[string]*workerEntry)
	for id, workerEntry := range pm.workers {
		workerEntriesCopy[id] = workerEntry
	}
	return workerEntriesCopy
}

// getWorkerAndManagerState returns worker entry and process manager state under lock
// Returns: workerEntry, state, exists
func (pm *processManager) getWorkerAndManagerState(id string) (*workerEntry, ProcessManagerState, bool) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	workerEntry, exists := pm.workers[id]
	return workerEntry, pm.state, exists
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

// getLogCollectionConfig creates a default log collection config for workers
func (pm *processManager) getLogCollectionConfig() *logconfig.WorkerLogConfig {
	if pm.logCollectionService == nil {
		return nil
	}

	// Create default worker log configuration
	defaultConfig := logconfig.DefaultWorkerLogConfig()
	return &defaultConfig
}
