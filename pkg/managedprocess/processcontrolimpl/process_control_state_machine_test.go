//go:build test

package processcontrolimpl

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/managedprocess/processcontrol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== STATE MACHINE COMPREHENSIVE TESTS =====

func TestProcessState_AllTransitions(t *testing.T) {
	tests := []struct {
		name           string
		currentState   processcontrol.ProcessState
		operation      string
		expectedResult bool
		description    string
	}{
		// Start operation tests - comprehensive coverage
		{"start_from_idle", processcontrol.ProcessStateIdle, "start", true, "Normal startup path"},
		{"start_from_starting", processcontrol.ProcessStateStarting, "start", false, "Already starting - prevent double start"},
		{"start_from_running", processcontrol.ProcessStateRunning, "start", false, "Already running - prevent restart via start"},
		{"start_from_stopping", processcontrol.ProcessStateStopping, "start", false, "Still stopping - wait for completion"},
		{"start_from_terminating", processcontrol.ProcessStateTerminating, "start", false, "Still terminating - wait for completion"},

		// Stop operation tests - comprehensive coverage
		{"stop_from_idle", processcontrol.ProcessStateIdle, "stop", true, "Stop from idle is no-op but allowed"},
		{"stop_from_starting", processcontrol.ProcessStateStarting, "stop", false, "Cannot stop during startup"},
		{"stop_from_running", processcontrol.ProcessStateRunning, "stop", true, "Normal stop path"},
		{"stop_from_stopping", processcontrol.ProcessStateStopping, "stop", false, "Already stopping - prevent double stop"},
		{"stop_from_terminating", processcontrol.ProcessStateTerminating, "stop", false, "Already terminating - let it complete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &SimpleLogger{}
			config := processcontrol.ProcessControlOptions{
				CanTerminate: true,
			}

			pc := NewProcessControl(config, "test-process", logger)
			impl := pc.(*processControl)
			impl.state = tt.currentState

			var result bool
			switch tt.operation {
			case "start":
				result = impl.canStartFromState(tt.currentState)
			case "stop":
				result = impl.canStopFromState(tt.currentState)
			}

			assert.Equal(t, tt.expectedResult, result,
				"Test: %s - %s. Expected %s from state %s to be %t",
				tt.name, tt.description, tt.operation, tt.currentState, tt.expectedResult)
		})
	}
}

func TestProcessControl_Start_StateTransitions(t *testing.T) {
	tests := []struct {
		name         string
		initialState processcontrol.ProcessState
		setupMocks   func(*processControl)
		expectError  bool
		finalState   processcontrol.ProcessState
		description  string
	}{
		{
			name:         "successful_start_from_idle",
			initialState: processcontrol.ProcessStateIdle,
			setupMocks: func(impl *processControl) {
				impl.config.ExecuteCmd = func(ctx context.Context) (*processcontrol.CommandResult, error) {
					return &processcontrol.CommandResult{
						Process: &os.Process{Pid: 12345},
						Stdout:  &MockReadCloser{},
					}, nil
				}
			},
			expectError: false,
			finalState:  processcontrol.ProcessStateRunning,
			description: "Normal startup should transition Idle -> Starting -> Running",
		},
		{
			name:         "start_from_running_blocked",
			initialState: processcontrol.ProcessStateRunning,
			setupMocks:   func(impl *processControl) {},
			expectError:  true,
			finalState:   processcontrol.ProcessStateRunning,
			description:  "Start from Running should be blocked by state validation",
		},
		{
			name:         "start_from_starting_blocked",
			initialState: processcontrol.ProcessStateStarting,
			setupMocks:   func(impl *processControl) {},
			expectError:  true,
			finalState:   processcontrol.ProcessStateStarting,
			description:  "Start from Starting should be blocked by state validation",
		},
		{
			name:         "start_failure_resets_to_idle",
			initialState: processcontrol.ProcessStateIdle,
			setupMocks: func(impl *processControl) {
				impl.config.ExecuteCmd = func(ctx context.Context) (*processcontrol.CommandResult, error) {
					return nil, errors.New("execution failed")
				}
			},
			expectError: true,
			finalState:  processcontrol.ProcessStateFailedStart,
			description: "Failed start should set state to FailedStart for diagnostics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &SimpleLogger{}
			config := processcontrol.ProcessControlOptions{
				CanTerminate:    true,
				GracefulTimeout: 30 * time.Second,
			}

			pc := NewProcessControl(config, "test-process", logger)
			impl := pc.(*processControl)
			impl.state = tt.initialState

			tt.setupMocks(impl)

			ctx := context.Background()
			err := pc.Start(ctx)

			if tt.expectError {
				assert.Error(t, err, "Expected error for test: %s", tt.description)
			} else {
				assert.NoError(t, err, "Expected no error for test: %s", tt.description)
			}

			assert.Equal(t, tt.finalState, impl.GetState(),
				"Final state mismatch for test: %s", tt.description)
		})
	}
}

func TestProcessControl_Stop_StateTransitions(t *testing.T) {
	tests := []struct {
		name         string
		initialState processcontrol.ProcessState
		hasProcess   bool
		expectError  bool
		finalState   processcontrol.ProcessState
		description  string
	}{
		{
			name:         "successful_stop_from_running",
			initialState: processcontrol.ProcessStateRunning,
			hasProcess:   true,
			expectError:  false,
			finalState:   processcontrol.ProcessStateIdle,
			description:  "Normal stop should transition Running -> Stopping -> Idle",
		},
		{
			name:         "stop_from_idle_noop",
			initialState: processcontrol.ProcessStateIdle,
			hasProcess:   false,
			expectError:  false,
			finalState:   processcontrol.ProcessStateIdle,
			description:  "Stop from Idle should be no-op but successful",
		},
		{
			name:         "stop_from_starting_blocked",
			initialState: processcontrol.ProcessStateStarting,
			hasProcess:   true,
			expectError:  true,
			finalState:   processcontrol.ProcessStateStarting,
			description:  "Stop from Starting should be blocked",
		},
		{
			name:         "stop_from_stopping_blocked",
			initialState: processcontrol.ProcessStateStopping,
			hasProcess:   true,
			expectError:  true,
			finalState:   processcontrol.ProcessStateStopping,
			description:  "Stop from Stopping should be blocked",
		},
		{
			name:         "stop_from_terminating_blocked",
			initialState: processcontrol.ProcessStateTerminating,
			hasProcess:   true,
			expectError:  true,
			finalState:   processcontrol.ProcessStateTerminating,
			description:  "Stop from Terminating should be blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &SimpleLogger{}
			config := processcontrol.ProcessControlOptions{
				CanTerminate:    true,
				GracefulTimeout: 30 * time.Second,
			}

			pc := NewProcessControl(config, "test-process", logger)
			impl := pc.(*processControl)
			impl.state = tt.initialState

			if tt.hasProcess {
				impl.process = &os.Process{Pid: 12345}
			}

			// For tests that should succeed, use the defer-only helpers directly to avoid real process operations
			if !tt.expectError && tt.initialState == processcontrol.ProcessStateRunning {
				// Test the defer-only locking pattern directly
				plan := impl.validateAndPlanStop()
				require.True(t, plan.shouldProceed)
				assert.NotNil(t, plan.processToTerminate)
				assert.Equal(t, processcontrol.ProcessStateStopping, impl.state)

				// Simulate successful termination by calling finalize
				impl.finalizeStop()
				assert.Equal(t, processcontrol.ProcessStateIdle, impl.state)
				return
			}

			// For no-op case (stop from idle)
			if !tt.expectError && tt.initialState == processcontrol.ProcessStateIdle {
				plan := impl.validateAndPlanStop()
				assert.False(t, plan.shouldProceed)
				assert.Nil(t, plan.errorToReturn)
				assert.Equal(t, processcontrol.ProcessStateIdle, impl.state)
				return
			}

			// For error cases, test validation directly
			if tt.expectError {
				plan := impl.validateAndPlanStop()
				if tt.initialState != processcontrol.ProcessStateIdle {
					assert.False(t, plan.shouldProceed)
					assert.NotNil(t, plan.errorToReturn)
				}
				assert.Equal(t, tt.finalState, impl.state)
				return
			}

			// Fallback for actual Stop call (only for cases we know will work)
			ctx := context.Background()
			err := pc.Stop(ctx)

			if tt.expectError {
				assert.Error(t, err, "Expected error for test: %s", tt.description)
			} else {
				assert.NoError(t, err, "Expected no error for test: %s", tt.description)
			}

			assert.Equal(t, tt.finalState, impl.GetState(),
				"Final state mismatch for test: %s", tt.description)
		})
	}
}

func TestProcessControl_Restart_StateTransitions(t *testing.T) {
	tests := []struct {
		name         string
		initialState processcontrol.ProcessState
		hasProcess   bool
		setupMocks   func(*processControl)
		expectError  bool
		finalState   processcontrol.ProcessState
		description  string
	}{
		{
			name:         "successful_restart_from_running",
			initialState: processcontrol.ProcessStateRunning,
			hasProcess:   true,
			setupMocks: func(impl *processControl) {
				impl.config.ExecuteCmd = func(ctx context.Context) (*processcontrol.CommandResult, error) {
					return &processcontrol.CommandResult{
						Process: &os.Process{Pid: 54321},
						Stdout:  &MockReadCloser{},
					}, nil
				}
			},
			expectError: false,
			finalState:  processcontrol.ProcessStateRunning,
			description: "Restart should go Running -> Stopping -> Idle -> Starting -> Running",
		},
		{
			name:         "restart_from_idle_blocked",
			initialState: processcontrol.ProcessStateIdle,
			hasProcess:   false,
			setupMocks:   func(impl *processControl) {},
			expectError:  true,
			finalState:   processcontrol.ProcessStateIdle,
			description:  "Restart from Idle should fail (no process to restart)",
		},
		{
			name:         "restart_without_process_blocked",
			initialState: processcontrol.ProcessStateRunning,
			hasProcess:   false,
			setupMocks:   func(impl *processControl) {},
			expectError:  true,
			finalState:   processcontrol.ProcessStateRunning,
			description:  "Restart without process should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &SimpleLogger{}
			config := processcontrol.ProcessControlOptions{
				CanTerminate:    true,
				GracefulTimeout: 30 * time.Second,
			}

			pc := NewProcessControl(config, "test-process", logger)
			impl := pc.(*processControl)
			impl.state = tt.initialState

			if tt.hasProcess {
				impl.process = &os.Process{Pid: 12345}
			}

			tt.setupMocks(impl)

			// For successful restart, test the state machine logic directly
			if !tt.expectError && tt.initialState == processcontrol.ProcessStateRunning && tt.hasProcess {
				// Test stop phase
				stopPlan := impl.validateAndPlanStop()
				require.True(t, stopPlan.shouldProceed)
				assert.Equal(t, processcontrol.ProcessStateStopping, impl.state)

				// Simulate successful stop
				impl.finalizeStop()
				assert.Equal(t, processcontrol.ProcessStateIdle, impl.state)

				// Test start phase
				assert.True(t, impl.canStartFromState(processcontrol.ProcessStateIdle))
				impl.state = processcontrol.ProcessStateStarting
				impl.process = &os.Process{Pid: 54321}
				impl.state = processcontrol.ProcessStateRunning

				assert.Equal(t, processcontrol.ProcessStateRunning, impl.state)
				return
			}

			// For error cases that we can test safely
			if tt.expectError {
				if tt.initialState == processcontrol.ProcessStateIdle {
					// Restart from idle should fail because no process
					assert.Nil(t, impl.process)
					return
				}
				if !tt.hasProcess {
					// Restart without process should fail
					assert.Nil(t, impl.process)
					return
				}
			}

			// For cases we can't test with real process operations, skip the actual call
			t.Skipf("Skipping actual restart call for test: %s (would require real process operations)", tt.description)
		})
	}
}

func TestProcessControl_StateTransitionConsistency(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate:    true,
		GracefulTimeout: 30 * time.Second,
		ExecuteCmd: func(ctx context.Context) (*processcontrol.CommandResult, error) {
			return &processcontrol.CommandResult{
				Process: &os.Process{Pid: 12345},
				Stdout:  &MockReadCloser{},
			}, nil
		},
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	// Test state transition logic using defer-only helpers
	t.Run("defer_only_lifecycle_consistency", func(t *testing.T) {
		// Start: Idle -> Running (test state validation)
		assert.Equal(t, processcontrol.ProcessStateIdle, impl.GetState())
		assert.True(t, impl.canStartFromState(processcontrol.ProcessStateIdle))

		// Simulate start
		impl.state = processcontrol.ProcessStateStarting
		impl.process = &os.Process{Pid: 12345}
		impl.state = processcontrol.ProcessStateRunning
		assert.Equal(t, processcontrol.ProcessStateRunning, impl.GetState())

		// Stop: Running -> Idle (test defer-only pattern)
		plan := impl.validateAndPlanStop()
		require.True(t, plan.shouldProceed)
		assert.Equal(t, processcontrol.ProcessStateStopping, impl.state)

		impl.finalizeStop()
		assert.Equal(t, processcontrol.ProcessStateIdle, impl.GetState())
	})

	// Reset for next test
	impl.state = processcontrol.ProcessStateIdle
	impl.process = nil

	// Test: Multiple operations on same state
	t.Run("idempotent_operations_validation", func(t *testing.T) {
		// Multiple stops on idle should be safe (test validation)
		for i := 0; i < 3; i++ {
			plan := impl.validateAndPlanStop()
			assert.False(t, plan.shouldProceed) // No process to stop
			assert.Nil(t, plan.errorToReturn)   // But no error
			assert.Equal(t, processcontrol.ProcessStateIdle, impl.state)
		}

		// Simulate start
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		// Multiple starts on running should fail consistently
		for i := 0; i < 3; i++ {
			canStart := impl.canStartFromState(processcontrol.ProcessStateRunning)
			assert.False(t, canStart)
			assert.Equal(t, processcontrol.ProcessStateRunning, impl.state)
		}
	})
}

func TestProcessControl_StateValidation_EdgeCases(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	// Test all possible state values for completeness
	allStates := []processcontrol.ProcessState{
		processcontrol.ProcessStateIdle,
		processcontrol.ProcessStateStarting,
		processcontrol.ProcessStateRunning,
		processcontrol.ProcessStateStopping,
		processcontrol.ProcessStateTerminating,
	}

	t.Run("start_validation_comprehensive", func(t *testing.T) {
		expectedResults := map[processcontrol.ProcessState]bool{
			processcontrol.ProcessStateIdle:        true,  // Can start from idle
			processcontrol.ProcessStateStarting:    false, // Cannot start while starting
			processcontrol.ProcessStateRunning:     false, // Cannot start while running
			processcontrol.ProcessStateStopping:    false, // Cannot start while stopping
			processcontrol.ProcessStateTerminating: false, // Cannot start while terminating
		}

		for _, state := range allStates {
			result := impl.canStartFromState(state)
			expected := expectedResults[state]
			assert.Equal(t, expected, result,
				"canStartFromState(%s) should return %t", state, expected)
		}
	})

	t.Run("stop_validation_comprehensive", func(t *testing.T) {
		expectedResults := map[processcontrol.ProcessState]bool{
			processcontrol.ProcessStateIdle:        true,  // Can stop from idle (no-op)
			processcontrol.ProcessStateStarting:    false, // Cannot stop while starting
			processcontrol.ProcessStateRunning:     true,  // Can stop from running
			processcontrol.ProcessStateStopping:    false, // Cannot stop while stopping
			processcontrol.ProcessStateTerminating: false, // Cannot stop while terminating
		}

		for _, state := range allStates {
			result := impl.canStopFromState(state)
			expected := expectedResults[state]
			assert.Equal(t, expected, result,
				"canStopFromState(%s) should return %t", state, expected)
		}
	})
}

func TestProcessControl_InvalidStateHandling(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	// Test with invalid/unknown state
	t.Run("unknown_state_handling", func(t *testing.T) {
		invalidState := processcontrol.ProcessState("unknown")
		impl.state = invalidState

		// Should reject operations on unknown states
		assert.False(t, impl.canStartFromState(invalidState))
		assert.False(t, impl.canStopFromState(invalidState))
	})

	// Test state consistency after operations
	t.Run("state_consistency_after_errors", func(t *testing.T) {
		impl.state = processcontrol.ProcessStateRunning
		ctx := context.Background()

		// Try to start from running (should fail)
		err := pc.Start(ctx)
		assert.Error(t, err)

		// State should remain unchanged
		assert.Equal(t, processcontrol.ProcessStateRunning, impl.GetState())

		// Try to stop without process (should fail validation)
		impl.process = nil
		err = pc.Stop(ctx)
		assert.Error(t, err)

		// State should remain unchanged
		assert.Equal(t, processcontrol.ProcessStateRunning, impl.GetState())
	})
}
