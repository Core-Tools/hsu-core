package workerstatemachine

import (
	"fmt"
	"sync"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
)

// WorkerState represents the current state of a worker in its lifecycle
type WorkerState string

const (
	// WorkerStateUnknown is the initial state before worker registration
	WorkerStateUnknown WorkerState = "unknown"

	// WorkerStateRegistered means worker is added to master but not started
	WorkerStateRegistered WorkerState = "registered"

	// WorkerStateStarting means worker start operation is in progress
	WorkerStateStarting WorkerState = "starting"

	// WorkerStateRunning means worker is running normally
	WorkerStateRunning WorkerState = "running"

	// WorkerStateStopping means worker stop operation is in progress
	WorkerStateStopping WorkerState = "stopping"

	// WorkerStateStopped means worker stopped cleanly
	WorkerStateStopped WorkerState = "stopped"

	// WorkerStateFailed means worker failed to start or crashed
	WorkerStateFailed WorkerState = "failed"

	// WorkerStateRestarting means worker restart operation is in progress
	WorkerStateRestarting WorkerState = "restarting"
)

// WorkerStateTransition represents a state transition with metadata
type WorkerStateTransition struct {
	From      WorkerState
	To        WorkerState
	Operation string
	Timestamp time.Time
	Error     error
}

// WorkerStateMachine manages worker state transitions with validation
type WorkerStateMachine struct {
	workerID         string
	currentState     WorkerState
	transitions      []WorkerStateTransition
	validTransitions map[WorkerState][]WorkerState
	mutex            sync.RWMutex
	logger           logging.Logger
}

// NewWorkerStateMachine creates a new worker state machine
func NewWorkerStateMachine(workerID string, logger logging.Logger) *WorkerStateMachine {
	wsm := &WorkerStateMachine{
		workerID:     workerID,
		currentState: WorkerStateUnknown,
		transitions:  make([]WorkerStateTransition, 0),
		mutex:        sync.RWMutex{},
		logger:       logger,
	}

	// Define valid state transitions
	wsm.validTransitions = map[WorkerState][]WorkerState{
		WorkerStateUnknown: {
			WorkerStateRegistered, // AddWorker
		},
		WorkerStateRegistered: {
			WorkerStateStarting, // StartWorker
		},
		WorkerStateStarting: {
			WorkerStateRunning, // start success
			WorkerStateFailed,  // start failure
		},
		WorkerStateRunning: {
			WorkerStateStopping,   // StopWorker
			WorkerStateFailed,     // process crash
			WorkerStateRestarting, // RestartWorker
		},
		WorkerStateStopping: {
			WorkerStateStopped, // stop success
			WorkerStateFailed,  // stop failure
		},
		WorkerStateStopped: {
			WorkerStateStarting, // restart after clean stop
		},
		WorkerStateFailed: {
			WorkerStateStarting, // retry after failure
		},
		WorkerStateRestarting: {
			WorkerStateRunning, // restart success
			WorkerStateFailed,  // restart failure
		},
	}

	return wsm
}

// GetCurrentState returns the current state (thread-safe)
func (wsm *WorkerStateMachine) GetCurrentState() WorkerState {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()
	return wsm.currentState
}

// CanTransition checks if a state transition is valid (thread-safe)
func (wsm *WorkerStateMachine) CanTransition(to WorkerState) bool {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()
	return wsm.canTransitionUnsafe(to)
}

// Transition changes the worker state with validation (thread-safe)
func (wsm *WorkerStateMachine) Transition(to WorkerState, operation string, err error) error {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	// Validate transition
	if !wsm.canTransitionUnsafe(to) {
		return errors.NewValidationError(
			fmt.Sprintf("invalid state transition from '%s' to '%s'", wsm.currentState, to),
			nil,
		).WithContext("worker_id", wsm.workerID).
			WithContext("from_state", string(wsm.currentState)).
			WithContext("to_state", string(to)).
			WithContext("operation", operation)
	}

	// Record transition
	from := wsm.currentState
	transition := WorkerStateTransition{
		From:      from,
		To:        to,
		Operation: operation,
		Timestamp: time.Now(),
		Error:     err,
	}

	wsm.transitions = append(wsm.transitions, transition)
	wsm.currentState = to

	// Log state transition
	if err != nil {
		wsm.logger.Warnf("Worker state transition failed, worker: %s, %s->%s, operation: %s, error: %v",
			wsm.workerID, from, to, operation, err)
	} else {
		wsm.logger.Infof("Worker state transition, worker: %s, %s->%s, operation: %s",
			wsm.workerID, from, to, operation)
	}

	return nil
}

// canTransitionUnsafe checks transition validity without locking (internal use)
func (wsm *WorkerStateMachine) canTransitionUnsafe(to WorkerState) bool {
	validStates, exists := wsm.validTransitions[wsm.currentState]
	if !exists {
		return false
	}

	for _, validState := range validStates {
		if validState == to {
			return true
		}
	}
	return false
}

// GetTransitionHistory returns the complete transition history (thread-safe)
func (wsm *WorkerStateMachine) GetTransitionHistory() []WorkerStateTransition {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()

	// Return a copy to prevent external modification
	history := make([]WorkerStateTransition, len(wsm.transitions))
	copy(history, wsm.transitions)
	return history
}

// GetStateInfo returns comprehensive state information
func (wsm *WorkerStateMachine) GetStateInfo() WorkerStateInfo {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()

	var lastTransition *WorkerStateTransition
	if len(wsm.transitions) > 0 {
		lastTransition = &wsm.transitions[len(wsm.transitions)-1]
	}

	return WorkerStateInfo{
		WorkerID:        wsm.workerID,
		CurrentState:    wsm.currentState,
		LastTransition:  lastTransition,
		TransitionCount: len(wsm.transitions),
		ValidNextStates: wsm.getValidNextStatesUnsafe(),
	}
}

// getValidNextStatesUnsafe returns valid next states without locking (internal use)
func (wsm *WorkerStateMachine) getValidNextStatesUnsafe() []WorkerState {
	validStates, exists := wsm.validTransitions[wsm.currentState]
	if !exists {
		return []WorkerState{}
	}

	// Return a copy
	nextStates := make([]WorkerState, len(validStates))
	copy(nextStates, validStates)
	return nextStates
}

// WorkerStateInfo provides comprehensive information about worker state
type WorkerStateInfo struct {
	WorkerID        string
	CurrentState    WorkerState
	LastTransition  *WorkerStateTransition
	TransitionCount int
	ValidNextStates []WorkerState
}

// IsOperationAllowed checks if a specific operation is allowed in current state
func (wsm *WorkerStateMachine) IsOperationAllowed(operation string) bool {
	currentState := wsm.GetCurrentState()

	switch operation {
	case "start":
		return wsm.CanTransition(WorkerStateStarting)
	case "stop":
		return currentState == WorkerStateRunning && wsm.CanTransition(WorkerStateStopping)
	case "restart":
		return currentState == WorkerStateRunning && wsm.CanTransition(WorkerStateRestarting)
	case "add":
		return currentState == WorkerStateUnknown && wsm.CanTransition(WorkerStateRegistered)
	case "remove":
		return currentState == WorkerStateRegistered || currentState == WorkerStateStopped
	default:
		return false
	}
}

// ValidateOperation checks if an operation can be performed and returns descriptive error
func (wsm *WorkerStateMachine) ValidateOperation(operation string) error {
	if wsm.IsOperationAllowed(operation) {
		return nil
	}

	currentState := wsm.GetCurrentState()
	return errors.NewValidationError(
		fmt.Sprintf("operation '%s' not allowed in current state '%s'", operation, currentState),
		nil,
	).WithContext("worker_id", wsm.workerID).WithContext("current_state", string(currentState)).WithContext("operation", operation)
}
