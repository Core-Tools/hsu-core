package processstatemachine

import (
	"fmt"
	"sync"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/errors"
	"github.com/core-tools/hsu-procman-go/pkg/logging"
)

// ProcessState represents the current state of a process in its lifecycle
type ProcessState string

const (
	// ProcessStateUnknown is the initial state before process registration
	ProcessStateUnknown ProcessState = "unknown"

	// ProcessStateRegistered means process is added to process manager but not started
	ProcessStateRegistered ProcessState = "registered"

	// ProcessStateStarting means process start operation is in progress
	ProcessStateStarting ProcessState = "starting"

	// ProcessStateRunning means process is running normally
	ProcessStateRunning ProcessState = "running"

	// ProcessStateStopping means process stop operation is in progress
	ProcessStateStopping ProcessState = "stopping"

	// ProcessStateStopped means process stopped cleanly
	ProcessStateStopped ProcessState = "stopped"

	// ProcessStateFailed means process failed to start or crashed
	ProcessStateFailed ProcessState = "failed"

	// ProcessStateRestarting means process restart operation is in progress
	ProcessStateRestarting ProcessState = "restarting"
)

// ProcessStateTransition represents a state transition with metadata
type ProcessStateTransition struct {
	From      ProcessState
	To        ProcessState
	Operation string
	Timestamp time.Time
	Error     error
}

// ProcessStateMachine manages process state transitions with validation
type ProcessStateMachine struct {
	processID        string
	currentState     ProcessState
	transitions      []ProcessStateTransition
	validTransitions map[ProcessState][]ProcessState
	mutex            sync.RWMutex
	logger           logging.Logger
}

// NewProcessStateMachine creates a new process state machine
func NewProcessStateMachine(processID string, logger logging.Logger) *ProcessStateMachine {
	wsm := &ProcessStateMachine{
		processID:    processID,
		currentState: ProcessStateUnknown,
		transitions:  make([]ProcessStateTransition, 0),
		mutex:        sync.RWMutex{},
		logger:       logger,
	}

	// Define valid state transitions
	wsm.validTransitions = map[ProcessState][]ProcessState{
		ProcessStateUnknown: {
			ProcessStateRegistered, // AddProcess
		},
		ProcessStateRegistered: {
			ProcessStateStarting, // StartProcess
		},
		ProcessStateStarting: {
			ProcessStateRunning, // start success
			ProcessStateFailed,  // start failure
		},
		ProcessStateRunning: {
			ProcessStateStopping,   // StopProcess
			ProcessStateFailed,     // process crash
			ProcessStateRestarting, // RestartProcess
		},
		ProcessStateStopping: {
			ProcessStateStopped, // stop success
			ProcessStateFailed,  // stop failure
		},
		ProcessStateStopped: {
			ProcessStateStarting, // restart after clean stop
		},
		ProcessStateFailed: {
			ProcessStateStarting, // retry after failure
		},
		ProcessStateRestarting: {
			ProcessStateRunning, // restart success
			ProcessStateFailed,  // restart failure
		},
	}

	return wsm
}

// GetCurrentState returns the current state (thread-safe)
func (wsm *ProcessStateMachine) GetCurrentState() ProcessState {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()
	return wsm.currentState
}

// CanTransition checks if a state transition is valid (thread-safe)
func (wsm *ProcessStateMachine) CanTransition(to ProcessState) bool {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()
	return wsm.canTransitionUnsafe(to)
}

// Transition changes the process state with validation (thread-safe)
func (wsm *ProcessStateMachine) Transition(to ProcessState, operation string, err error) error {
	wsm.mutex.Lock()
	defer wsm.mutex.Unlock()

	// Validate transition
	if !wsm.canTransitionUnsafe(to) {
		return errors.NewValidationError(
			fmt.Sprintf("invalid state transition from '%s' to '%s'", wsm.currentState, to),
			nil,
		).WithContext("process_id", wsm.processID).
			WithContext("from_state", string(wsm.currentState)).
			WithContext("to_state", string(to)).
			WithContext("operation", operation)
	}

	// Record transition
	from := wsm.currentState
	transition := ProcessStateTransition{
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
		wsm.logger.Warnf("Managed process state transition failed, managed process: %s, %s->%s, operation: %s, error: %v",
			wsm.processID, from, to, operation, err)
	} else {
		wsm.logger.Infof("Managed process state transition, managed process: %s, %s->%s, operation: %s",
			wsm.processID, from, to, operation)
	}

	return nil
}

// canTransitionUnsafe checks transition validity without locking (internal use)
func (wsm *ProcessStateMachine) canTransitionUnsafe(to ProcessState) bool {
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
func (wsm *ProcessStateMachine) GetTransitionHistory() []ProcessStateTransition {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()

	// Return a copy to prevent external modification
	history := make([]ProcessStateTransition, len(wsm.transitions))
	copy(history, wsm.transitions)
	return history
}

// GetStateInfo returns comprehensive state information
func (wsm *ProcessStateMachine) GetStateInfo() ProcessStateInfo {
	wsm.mutex.RLock()
	defer wsm.mutex.RUnlock()

	var lastTransition *ProcessStateTransition
	if len(wsm.transitions) > 0 {
		lastTransition = &wsm.transitions[len(wsm.transitions)-1]
	}

	return ProcessStateInfo{
		ProcessID:       wsm.processID,
		CurrentState:    wsm.currentState,
		LastTransition:  lastTransition,
		TransitionCount: len(wsm.transitions),
		ValidNextStates: wsm.getValidNextStatesUnsafe(),
	}
}

// getValidNextStatesUnsafe returns valid next states without locking (internal use)
func (wsm *ProcessStateMachine) getValidNextStatesUnsafe() []ProcessState {
	validStates, exists := wsm.validTransitions[wsm.currentState]
	if !exists {
		return []ProcessState{}
	}

	// Return a copy
	nextStates := make([]ProcessState, len(validStates))
	copy(nextStates, validStates)
	return nextStates
}

// ProcessStateInfo provides comprehensive information about process state
type ProcessStateInfo struct {
	ProcessID       string
	CurrentState    ProcessState
	LastTransition  *ProcessStateTransition
	TransitionCount int
	ValidNextStates []ProcessState
}

// IsOperationAllowed checks if a specific operation is allowed in current state
func (wsm *ProcessStateMachine) IsOperationAllowed(operation string) bool {
	currentState := wsm.GetCurrentState()

	switch operation {
	case "start":
		return wsm.CanTransition(ProcessStateStarting)
	case "stop":
		return currentState == ProcessStateRunning && wsm.CanTransition(ProcessStateStopping)
	case "restart":
		return currentState == ProcessStateRunning && wsm.CanTransition(ProcessStateRestarting)
	case "add":
		return currentState == ProcessStateUnknown && wsm.CanTransition(ProcessStateRegistered)
	case "remove":
		return currentState == ProcessStateRegistered || currentState == ProcessStateStopped
	default:
		return false
	}
}

// ValidateOperation checks if an operation can be performed and returns descriptive error
func (wsm *ProcessStateMachine) ValidateOperation(operation string) error {
	if wsm.IsOperationAllowed(operation) {
		return nil
	}

	currentState := wsm.GetCurrentState()
	return errors.NewValidationError(
		fmt.Sprintf("operation '%s' not allowed in current state '%s'", operation, currentState),
		nil,
	).WithContext("process_id", wsm.processID).WithContext("current_state", string(currentState)).WithContext("operation", operation)
}
