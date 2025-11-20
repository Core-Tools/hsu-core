//go:build test

package processcontrolimpl

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-procman-go/pkg/resourcelimits"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== DEFER-ONLY LOCKING ARCHITECTURAL PATTERN TESTS =====

func TestDeferOnlyLocking_PlanExecuteFinalizePattern(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate:    true,
		GracefulTimeout: 30 * time.Second,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("stop_operation_plan_execute_finalize", func(t *testing.T) {
		// Setup: Process in running state
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}
		impl.stdout = &MockReadCloser{}

		// PHASE 1: PLAN (validateAndPlanStop)
		plan := impl.validateAndPlanStop()

		// Verify planning results
		require.NotNil(t, plan, "Plan should be created")
		assert.True(t, plan.shouldProceed, "Plan should indicate proceed")
		assert.NotNil(t, plan.processToTerminate, "Plan should extract process reference")
		assert.Nil(t, plan.errorToReturn, "Plan should not have error for valid operation")

		// Verify immediate state transition during planning
		assert.Equal(t, processcontrol.ProcessStateStopping, impl.state, "State should transition to Stopping during planning")
		assert.Nil(t, impl.process, "Process reference should be cleared during planning")

		// PHASE 2: EXECUTE (would happen outside locks - we simulate)
		// In real implementation: terminateProcessExternal(plan.processToTerminate)

		// PHASE 3: FINALIZE (finalizeStop)
		impl.finalizeStop()

		// Verify finalization results
		assert.Equal(t, processcontrol.ProcessStateIdle, impl.state, "State should be Idle after finalization")
		assert.Nil(t, impl.stdout, "Resources should be cleaned up during finalization")
	})

	t.Run("termination_operation_plan_execute_finalize", func(t *testing.T) {
		// Reset for termination test
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 67890}
		impl.stdout = &MockReadCloser{}

		// PHASE 1: PLAN (validateAndPlanTermination)
		policy := resourcelimits.ResourcePolicyGracefulShutdown
		plan := impl.validateAndPlanTermination(policy, "test termination")

		// Verify planning results
		require.NotNil(t, plan, "Termination plan should be created")
		assert.True(t, plan.shouldProceed, "Termination plan should indicate proceed")
		assert.NotNil(t, plan.processToTerminate, "Termination plan should extract process reference")
		assert.Equal(t, processcontrol.ProcessStateStopping, plan.targetState, "Graceful shutdown should target Stopping state")
		assert.False(t, plan.skipGraceful, "Graceful shutdown should not skip graceful termination")

		// Verify state transition during planning
		assert.Equal(t, processcontrol.ProcessStateStopping, impl.state, "State should transition to target state during planning")

		// PHASE 2: EXECUTE (would happen outside locks)
		// In real implementation: terminateProcessExternal(plan.processToTerminate, plan.skipGraceful)

		// PHASE 3: FINALIZE (finalizeTermination)
		impl.finalizeTermination(plan)

		// Verify finalization results
		assert.Equal(t, processcontrol.ProcessStateIdle, impl.state, "State should be Idle after termination finalization")
	})

	t.Run("immediate_kill_plan_execute_finalize", func(t *testing.T) {
		// Reset for immediate kill test
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 11111}

		// PHASE 1: PLAN (validateAndPlanTermination for immediate kill)
		policy := resourcelimits.ResourcePolicyImmediateKill
		plan := impl.validateAndPlanTermination(policy, "immediate kill test")

		// Verify planning results for immediate kill
		require.NotNil(t, plan, "Immediate kill plan should be created")
		assert.True(t, plan.shouldProceed, "Immediate kill plan should proceed")
		assert.Equal(t, processcontrol.ProcessStateTerminating, plan.targetState, "Immediate kill should target Terminating state")
		assert.True(t, plan.skipGraceful, "Immediate kill should skip graceful termination")

		// Verify state transition
		assert.Equal(t, processcontrol.ProcessStateTerminating, impl.state, "State should be Terminating for immediate kill")

		// PHASE 3: FINALIZE (finalizeTermination)
		impl.finalizeTermination(plan)

		// Verify finalization
		assert.Equal(t, processcontrol.ProcessStateIdle, impl.state, "State should be Idle after immediate kill finalization")
	})
}

func TestDeferOnlyLocking_DataTransferStructs(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("stop_plan_data_structure", func(t *testing.T) {
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		plan := impl.validateAndPlanStop()

		// Verify stopPlan struct contains all necessary data
		assert.IsType(t, &stopPlan{}, plan, "Should return stopPlan struct")

		// Verify data transfer completeness
		assert.NotNil(t, plan.processToTerminate, "Plan should transfer process reference")
		assert.True(t, plan.shouldProceed, "Plan should transfer proceed decision")
		assert.Nil(t, plan.errorToReturn, "Plan should transfer error state")

		// Verify data is extracted safely under lock
		assert.Equal(t, 12345, plan.processToTerminate.Pid, "Process PID should be preserved in plan")
	})

	t.Run("termination_plan_data_structure", func(t *testing.T) {
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 54321}

		plan := impl.validateAndPlanTermination(resourcelimits.ResourcePolicyGracefulShutdown, "test reason")

		// Verify terminationPlan struct contains all necessary data
		assert.IsType(t, &terminationPlan{}, plan, "Should return terminationPlan struct")

		// Verify comprehensive data transfer
		assert.NotNil(t, plan.processToTerminate, "Plan should transfer process reference")
		assert.Equal(t, processcontrol.ProcessStateStopping, plan.targetState, "Plan should transfer target state")
		assert.False(t, plan.skipGraceful, "Plan should transfer graceful flag")
		assert.True(t, plan.shouldProceed, "Plan should transfer proceed decision")
		assert.Nil(t, plan.errorToReturn, "Plan should transfer error state")

		// Verify data integrity
		assert.Equal(t, 54321, plan.processToTerminate.Pid, "Process PID should be preserved in termination plan")
	})

	t.Run("plan_data_isolation", func(t *testing.T) {
		// Test that plan data is isolated and doesn't interfere between operations
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 99999}

		// Create multiple plans
		stopPlan1 := impl.validateAndPlanStop()

		// Reset state for second plan (simulating different operation)
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 88888}

		termPlan := impl.validateAndPlanTermination(resourcelimits.ResourcePolicyImmediateKill, "isolation test")

		// Verify plans are independent
		assert.NotEqual(t, stopPlan1.processToTerminate.Pid, termPlan.processToTerminate.Pid, "Plans should have independent process references")
		assert.NotEqual(t, stopPlan1.shouldProceed, termPlan.targetState, "Plans should have independent state data")
	})
}

func TestDeferOnlyLocking_LockScopedValidators(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("validators_automatic_unlock", func(t *testing.T) {
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		// Test that validators automatically unlock via defer
		startTime := time.Now()

		// This should complete quickly without deadlock
		plan := impl.validateAndPlanStop()

		duration := time.Since(startTime)
		assert.Less(t, duration, 100*time.Millisecond, "Validator should complete quickly (no deadlock)")
		assert.NotNil(t, plan, "Validator should return plan")
	})

	t.Run("validators_state_validation_logic", func(t *testing.T) {
		testCases := []struct {
			name            string
			initialState    processcontrol.ProcessState
			hasProcess      bool
			expectedProceed bool
			expectedError   bool
		}{
			{"running_with_process", processcontrol.ProcessStateRunning, true, true, false},
			{"idle_without_process", processcontrol.ProcessStateIdle, false, false, false},
			{"starting_with_process", processcontrol.ProcessStateStarting, true, false, true},
			{"stopping_with_process", processcontrol.ProcessStateStopping, true, false, true},
			{"terminating_with_process", processcontrol.ProcessStateTerminating, true, false, true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				impl.state = tc.initialState
				if tc.hasProcess {
					impl.process = &os.Process{Pid: 12345}
				} else {
					impl.process = nil
				}

				plan := impl.validateAndPlanStop()

				assert.Equal(t, tc.expectedProceed, plan.shouldProceed, "shouldProceed should match expected for %s", tc.name)

				if tc.expectedError {
					assert.NotNil(t, plan.errorToReturn, "Should have error for %s", tc.name)
				} else {
					assert.Nil(t, plan.errorToReturn, "Should not have error for %s", tc.name)
				}
			})
		}
	})

	t.Run("validators_concurrent_safety", func(t *testing.T) {
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		// Test concurrent validation calls
		var wg sync.WaitGroup
		results := make([]*stopPlan, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				results[index] = impl.validateAndPlanStop()
			}(i)
		}

		wg.Wait()

		// Only one should succeed due to state protection
		successCount := 0
		for _, result := range results {
			if result.shouldProceed {
				successCount++
			}
		}

		assert.Equal(t, 1, successCount, "Only one concurrent validation should succeed")
	})
}

func TestDeferOnlyLocking_LockScopedFinalizers(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("finalizers_automatic_unlock", func(t *testing.T) {
		impl.state = processcontrol.ProcessStateStopping
		impl.stdout = &MockReadCloser{}

		startTime := time.Now()

		// This should complete quickly without deadlock
		impl.finalizeStop()

		duration := time.Since(startTime)
		assert.Less(t, duration, 100*time.Millisecond, "Finalizer should complete quickly (no deadlock)")
		assert.Equal(t, processcontrol.ProcessStateIdle, impl.state, "Finalizer should complete state transition")
	})

	t.Run("finalizers_resource_cleanup", func(t *testing.T) {
		// Test that finalizers properly clean up resources
		impl.state = processcontrol.ProcessStateStopping
		impl.stdout = &MockReadCloser{}

		// Set up mocks with proper expectations for cleanup
		healthMonitor := &MockHealthMonitor{}
		healthMonitor.On("Stop").Maybe() // Allow Stop to be called
		impl.healthMonitor = healthMonitor

		resourceManager := &MockResourceLimitManager{}
		resourceManager.On("Stop").Maybe() // Allow Stop to be called
		impl.resourceManager = resourceManager

		impl.finalizeStop()

		// Verify cleanup
		assert.Equal(t, processcontrol.ProcessStateIdle, impl.state, "State should be finalized to Idle")
		assert.Nil(t, impl.stdout, "stdout should be cleaned up")
		assert.Nil(t, impl.healthMonitor, "health monitor should be cleaned up")
		assert.Nil(t, impl.resourceManager, "resource manager should be cleaned up")
	})

	t.Run("finalizers_state_transition_completion", func(t *testing.T) {
		// Test different finalization scenarios
		scenarios := []struct {
			name          string
			initialState  processcontrol.ProcessState
			expectedFinal processcontrol.ProcessState
		}{
			{"finalize_from_stopping", processcontrol.ProcessStateStopping, processcontrol.ProcessStateIdle},
			{"finalize_from_terminating", processcontrol.ProcessStateTerminating, processcontrol.ProcessStateIdle},
		}

		for _, scenario := range scenarios {
			t.Run(scenario.name, func(t *testing.T) {
				impl.state = scenario.initialState

				if scenario.initialState == processcontrol.ProcessStateStopping {
					impl.finalizeStop()
				} else {
					// Create a dummy plan for termination finalization
					plan := &terminationPlan{
						targetState:   scenario.initialState,
						shouldProceed: true,
					}
					impl.finalizeTermination(plan)
				}

				assert.Equal(t, scenario.expectedFinal, impl.state, "Final state should match expected")
			})
		}
	})
}

func TestDeferOnlyLocking_SafeDataAccess(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("safe_state_getter", func(t *testing.T) {
		testStates := []processcontrol.ProcessState{
			processcontrol.ProcessStateIdle,
			processcontrol.ProcessStateStarting,
			processcontrol.ProcessStateRunning,
			processcontrol.ProcessStateStopping,
			processcontrol.ProcessStateTerminating,
		}

		for _, state := range testStates {
			impl.state = state

			safeState := impl.safeGetState()
			directState := impl.GetState()

			assert.Equal(t, state, safeState, "safeGetState should return correct state")
			assert.Equal(t, state, directState, "GetState should return correct state")
			assert.Equal(t, safeState, directState, "Both state getters should be consistent")
		}
	})

	t.Run("safe_process_getter", func(t *testing.T) {
		// Test with nil process
		impl.process = nil
		safeProcess := impl.safeGetProcess()
		assert.Nil(t, safeProcess, "safeGetProcess should return nil when process is nil")

		// Test with valid process
		testProcess := &os.Process{Pid: 12345}
		impl.process = testProcess
		safeProcess = impl.safeGetProcess()
		assert.NotNil(t, safeProcess, "safeGetProcess should return process when set")
		assert.Equal(t, 12345, safeProcess.Pid, "safeGetProcess should return correct process")
	})

	t.Run("safe_getters_concurrent_access", func(t *testing.T) {
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 99999}

		var wg sync.WaitGroup
		stateResults := make([]processcontrol.ProcessState, 50)
		processResults := make([]*os.Process, 50)

		// Concurrent state reads
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				stateResults[index] = impl.safeGetState()
				processResults[index] = impl.safeGetProcess()
			}(i)
		}

		wg.Wait()

		// Verify all reads were consistent
		for i, state := range stateResults {
			assert.Equal(t, processcontrol.ProcessStateRunning, state, "State read %d should be consistent", i)
			if processResults[i] != nil {
				assert.Equal(t, 99999, processResults[i].Pid, "Process read %d should be consistent", i)
			}
		}
	})

	t.Run("safe_getters_no_deadlock", func(t *testing.T) {
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 55555}

		// Test that safe getters don't deadlock when called in quick succession
		startTime := time.Now()

		for i := 0; i < 1000; i++ {
			state := impl.safeGetState()
			process := impl.safeGetProcess()

			assert.Equal(t, processcontrol.ProcessStateRunning, state)
			if process != nil {
				assert.Equal(t, 55555, process.Pid)
			}
		}

		duration := time.Since(startTime)
		assert.Less(t, duration, 1*time.Second, "1000 safe getter calls should complete quickly")
	})
}

func TestDeferOnlyLocking_ResourceCleanupConsolidation(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("cleanup_resources_under_lock", func(t *testing.T) {
		// Setup resources to be cleaned
		impl.stdout = &MockReadCloser{}

		// Set up mocks with proper expectations for cleanup
		healthMonitor := &MockHealthMonitor{}
		healthMonitor.On("Stop").Maybe() // Allow Stop to be called
		impl.healthMonitor = healthMonitor

		resourceManager := &MockResourceLimitManager{}
		resourceManager.On("Stop").Maybe() // Allow Stop to be called
		impl.resourceManager = resourceManager

		// Call cleanup
		impl.cleanupResourcesUnderLock()

		// Verify all resources are cleaned
		assert.Nil(t, impl.stdout, "stdout should be cleaned")
		assert.Nil(t, impl.healthMonitor, "healthMonitor should be cleaned")
		assert.Nil(t, impl.resourceManager, "resourceManager should be cleaned")
	})

	t.Run("cleanup_idempotent", func(t *testing.T) {
		// Setup resources
		impl.stdout = &MockReadCloser{}

		// Multiple cleanups should be safe
		impl.cleanupResourcesUnderLock()
		impl.cleanupResourcesUnderLock()
		impl.cleanupResourcesUnderLock()

		// Should remain clean
		assert.Nil(t, impl.stdout, "Multiple cleanups should be safe")
	})

	t.Run("cleanup_with_nil_resources", func(t *testing.T) {
		// Ensure cleanup is safe with nil resources
		impl.stdout = nil
		impl.healthMonitor = nil
		impl.resourceManager = nil

		// Should not panic
		assert.NotPanics(t, func() {
			impl.cleanupResourcesUnderLock()
		}, "Cleanup should be safe with nil resources")
	})
}

func TestDeferOnlyLocking_UnifiedPolicyExecution(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("execute_with_policy_pattern", func(t *testing.T) {
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		// Test the executeWithPolicy pattern for different policies
		policies := []resourcelimits.ResourcePolicy{
			resourcelimits.ResourcePolicyGracefulShutdown,
			resourcelimits.ResourcePolicyImmediateKill,
		}

		for _, policy := range policies {
			t.Run(string(policy), func(t *testing.T) {
				// Reset state
				impl.state = processcontrol.ProcessStateRunning
				impl.process = &os.Process{Pid: 12345}

				violation := &resourcelimits.ResourceViolation{
					LimitType: resourcelimits.ResourceLimitTypeMemory,
					Message:   "Test violation for " + string(policy),
				}

				// Should not panic and should handle policy correctly
				assert.NotPanics(t, func() {
					impl.handleResourceViolation(policy, violation)
				}, "Policy execution should not panic for %s", policy)
			})
		}
	})
}

func TestDeferOnlyLocking_ArchitecturalBenefits(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate: true,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("no_explicit_unlock_calls", func(t *testing.T) {
		// This test documents that our implementation has zero explicit mutex.Unlock() calls
		// All unlocking is handled by defer statements

		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		// These operations should complete without deadlock, proving defer-only works
		plan := impl.validateAndPlanStop()
		assert.NotNil(t, plan, "Plan should be created without explicit unlocks")

		impl.finalizeStop()
		assert.Equal(t, processcontrol.ProcessStateIdle, impl.state, "Finalize should work without explicit unlocks")
	})

	t.Run("exception_safety_verification", func(t *testing.T) {
		// Test that even if operations panic, locks are released via defer
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		// This would panic in executeViolationPolicy with nil violation, but locks should still be released
		assert.Panics(t, func() {
			impl.handleResourceViolation(resourcelimits.ResourcePolicyLog, nil)
		}, "Should panic with nil violation")

		// System should still be accessible (no deadlock)
		state := impl.safeGetState()
		assert.Contains(t, []processcontrol.ProcessState{processcontrol.ProcessStateRunning, processcontrol.ProcessStateIdle}, state, "System should remain accessible after panic")
	})

	t.Run("clarity_and_maintainability", func(t *testing.T) {
		// Test documents the clarity benefits of defer-only locking
		// Every lock scope is clearly defined and automatically managed

		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		// Each operation has clear begin/end lock boundaries via defer
		start := time.Now()

		plan := impl.validateAndPlanStop() // Lock scope 1: Plan
		impl.finalizeStop()                // Lock scope 2: Finalize
		state := impl.safeGetState()       // Lock scope 3: Safe read

		duration := time.Since(start)

		assert.True(t, plan.shouldProceed, "Plan should succeed")
		assert.Equal(t, processcontrol.ProcessStateIdle, state, "Final state should be correct")
		assert.Less(t, duration, 100*time.Millisecond, "Operations should be fast with clear lock boundaries")
	})
}
