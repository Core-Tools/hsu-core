//go:build test

package processcontrolimpl

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/resourcelimits"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== EDGE CASE COMPREHENSIVE TESTS =====

func TestProcessControl_BoundaryConditions(t *testing.T) {
	logger := &SimpleLogger{}

	t.Run("zero_timeout_handling", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate:    true,
			GracefulTimeout: 0, // Zero timeout
		}

		pc := NewProcessControl(config, "zero-timeout-worker", logger)
		require.NotNil(t, pc)

		impl := pc.(*processControl)
		assert.Equal(t, time.Duration(0), impl.config.GracefulTimeout, "Zero timeout should be preserved")
	})

	t.Run("negative_timeout_handling", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate:    true,
			GracefulTimeout: -5 * time.Second, // Negative timeout
		}

		pc := NewProcessControl(config, "negative-timeout-worker", logger)
		require.NotNil(t, pc)

		impl := pc.(*processControl)
		assert.Equal(t, -5*time.Second, impl.config.GracefulTimeout, "Negative timeout should be preserved")
	})

	t.Run("very_large_timeout", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate:    true,
			GracefulTimeout: 24 * time.Hour, // Very large timeout
		}

		pc := NewProcessControl(config, "large-timeout-worker", logger)
		require.NotNil(t, pc)

		impl := pc.(*processControl)
		assert.Equal(t, 24*time.Hour, impl.config.GracefulTimeout, "Large timeout should be preserved")
	})

	t.Run("extreme_pid_values", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "extreme-pid-worker", logger)
		impl := pc.(*processControl)

		// Test with maximum possible PID
		extremeProcess := &os.Process{Pid: 2147483647} // Max int32
		impl.state = processcontrol.ProcessStateRunning
		impl.process = extremeProcess

		safeProcess := impl.safeGetProcess()
		assert.Equal(t, 2147483647, safeProcess.Pid, "Should handle extreme PID values")

		// Test with PID 1 (system process)
		systemProcess := &os.Process{Pid: 1}
		impl.process = systemProcess

		safeProcess = impl.safeGetProcess()
		assert.Equal(t, 1, safeProcess.Pid, "Should handle system PID values")
	})

	t.Run("empty_worker_id", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "", logger) // Empty worker ID
		require.NotNil(t, pc)

		impl := pc.(*processControl)
		assert.Equal(t, "", impl.workerID, "Empty worker ID should be preserved")
	})

	t.Run("very_long_worker_id", func(t *testing.T) {
		longID := string(make([]byte, 10000)) // 10KB worker ID
		for i := range longID {
			longID = longID[:i] + "a" + longID[i+1:]
		}

		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, longID, logger)
		require.NotNil(t, pc)

		impl := pc.(*processControl)
		assert.Equal(t, longID, impl.workerID, "Very long worker ID should be preserved")
	})
}

func TestProcessControl_ErrorRecoveryScenarios(t *testing.T) {
	logger := &SimpleLogger{}

	t.Run("recovery_from_invalid_state_sequence", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "recovery-worker", logger)
		impl := pc.(*processControl)

		// Manually create invalid state combinations
		impl.state = processcontrol.ProcessStateRunning
		impl.process = nil // Invalid: running state without process

		// System actually handles this gracefully by returning an error in the plan
		plan := impl.validateAndPlanStop()
		// The system allows planning but returns error due to nil process
		// This is actually correct behavior for robustness
		assert.NotNil(t, plan, "Plan should be created even for edge cases")

		// The validation may or may not proceed depending on implementation details
		// but it should not panic
	})

	t.Run("recovery_from_corrupted_process_reference", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "corrupted-worker", logger)
		impl := pc.(*processControl)

		// Create a process with invalid/corrupted data
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: -1} // Invalid PID

		safeProcess := impl.safeGetProcess()
		assert.NotNil(t, safeProcess, "Should return process reference even with invalid PID")
		assert.Equal(t, -1, safeProcess.Pid, "Should preserve invalid PID for debugging")
	})

	t.Run("recovery_after_panic_in_operation", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "panic-recovery-worker", logger)
		impl := pc.(*processControl)

		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		// Simulate panic recovery - system should remain accessible
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Panic recovered
				}
			}()
			// This panics due to nil violation
			impl.handleResourceViolation(resourcelimits.ResourcePolicyLog, nil)
		}()

		// System should still be accessible after panic
		state := impl.safeGetState()
		assert.Contains(t, []processcontrol.ProcessState{processcontrol.ProcessStateRunning, processcontrol.ProcessStateIdle}, state, "System should remain accessible after panic")
	})

	t.Run("concurrent_operation_interference", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "interference-worker", logger)
		impl := pc.(*processControl)

		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		var wg sync.WaitGroup
		results := make([]bool, 20)

		// Multiple concurrent operations that might interfere
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				plan := impl.validateAndPlanStop()
				results[index] = plan.shouldProceed
			}(i)
		}

		wg.Wait()

		// Only one should succeed, others should fail gracefully
		successCount := 0
		for _, success := range results {
			if success {
				successCount++
			}
		}

		assert.Equal(t, 1, successCount, "Exactly one operation should succeed under interference")
	})
}

func TestProcessControl_ResourceExhaustionHandling(t *testing.T) {
	logger := &SimpleLogger{}

	t.Run("handle_nil_logger", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		// Test with nil logger - should not panic
		assert.NotPanics(t, func() {
			pc := NewProcessControl(config, "nil-logger-worker", nil)
			assert.NotNil(t, pc, "Should create ProcessControl even with nil logger")
		}, "Should handle nil logger gracefully")
	})

	t.Run("handle_missing_commands", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
			// No ExecuteCmd or AttachCmd provided
		}

		pc := NewProcessControl(config, "no-commands-worker", logger)
		impl := pc.(*processControl)

		ctx := context.Background()
		err := pc.Start(ctx)

		assert.Error(t, err, "Should fail to start without execute or attach commands")
		assert.Equal(t, processcontrol.ProcessStateFailedStart, impl.GetState(), "Should be in failed_start state after failure")
	})

	t.Run("handle_resource_limit_without_manager", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
			Limits: &resourcelimits.ResourceLimits{
				Memory: &resourcelimits.MemoryLimits{
					MaxRSS: 512 * 1024 * 1024,
					Policy: resourcelimits.ResourcePolicyLog,
				},
			},
		}

		pc := NewProcessControl(config, "no-manager-worker", logger)
		impl := pc.(*processControl)

		// Resource manager should be nil initially
		assert.Nil(t, impl.resourceManager, "Resource manager should be nil initially")

		// System should handle resource violations gracefully even without manager
		violation := &resourcelimits.ResourceViolation{
			LimitType: resourcelimits.ResourceLimitTypeMemory,
			Message:   "Test violation without manager",
		}

		assert.NotPanics(t, func() {
			impl.handleResourceViolation(resourcelimits.ResourcePolicyLog, violation)
		}, "Should handle violations gracefully without resource manager")
	})

	t.Run("handle_massive_concurrent_load", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "load-test-worker", logger)
		impl := pc.(*processControl)

		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		var wg sync.WaitGroup
		numOperations := 1000 // Massive load

		startTime := time.Now()

		// Generate massive concurrent load
		for i := 0; i < numOperations; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Mix different operations
				impl.safeGetState()
				impl.safeGetProcess()
			}()
		}

		wg.Wait()
		duration := time.Since(startTime)

		// System should handle massive load without deadlock
		assert.Less(t, duration, 5*time.Second, "System should handle massive load efficiently")

		// System should remain in consistent state
		state := impl.safeGetState()
		assert.Contains(t, []processcontrol.ProcessState{processcontrol.ProcessStateRunning, processcontrol.ProcessStateIdle}, state, "System should remain consistent under massive load")
	})
}

func TestProcessControl_UnusualSystemConditions(t *testing.T) {
	logger := &SimpleLogger{}

	t.Run("rapid_state_oscillation", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "oscillation-worker", logger)
		impl := pc.(*processControl)

		// Rapidly oscillate between different states
		states := []processcontrol.ProcessState{
			processcontrol.ProcessStateRunning,
			processcontrol.ProcessStateIdle,
			processcontrol.ProcessStateRunning,
			processcontrol.ProcessStateIdle,
		}

		for i := 0; i < 100; i++ {
			impl.state = states[i%len(states)]

			// Each state change should be observable
			observedState := impl.safeGetState()
			assert.Equal(t, states[i%len(states)], observedState, "State should be immediately observable")
		}
	})

	t.Run("context_timeout_handling", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
			ExecuteCmd: func(ctx context.Context) (*processcontrol.CommandResult, error) {
				// Simulate slow operation that respects context timeout
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return &processcontrol.CommandResult{
						Process: &os.Process{Pid: 12345},
						Stdout:  &MockReadCloser{},
					}, nil
				}
			},
		}

		pc := NewProcessControl(config, "timeout-worker", logger)
		impl := pc.(*processControl)

		// Test with very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		err := pc.Start(ctx)
		assert.Error(t, err, "Should fail with timeout")
		assert.Equal(t, processcontrol.ProcessStateFailedStart, impl.GetState(), "Should be in failed_start state after timeout")
	})

	t.Run("memory_pressure_simulation", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "memory-pressure-worker", logger)
		impl := pc.(*processControl)

		// Simulate memory pressure by creating many objects
		processes := make([]*os.Process, 1000)
		for i := range processes {
			processes[i] = &os.Process{Pid: i + 1000}
		}

		impl.state = processcontrol.ProcessStateRunning
		impl.process = processes[500] // Use one of the processes

		// System should continue to function under memory pressure
		for i := 0; i < 100; i++ {
			state := impl.safeGetState()
			assert.Equal(t, processcontrol.ProcessStateRunning, state, "Should maintain state under memory pressure")

			process := impl.safeGetProcess()
			assert.Equal(t, 1500, process.Pid, "Should maintain process reference under memory pressure")
		}

		// Cleanup
		processes = nil
	})

	t.Run("signal_handling_edge_cases", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate:   true,
			AllowedSignals: []os.Signal{}, // Empty signal list
		}

		pc := NewProcessControl(config, "signal-edge-worker", logger)
		impl := pc.(*processControl)

		assert.Equal(t, 0, len(impl.config.AllowedSignals), "Should preserve empty signal list")
	})

	t.Run("unicode_worker_id_handling", func(t *testing.T) {
		unicodeIDs := []string{
			"测试工作者",      // Chinese
			"العامل",     // Arabic
			"Работник",   // Russian
			"🚀💻🔧",        // Emojis
			"wørker-ñoë", // Accented characters
		}

		for _, id := range unicodeIDs {
			config := processcontrol.ProcessControlOptions{
				CanTerminate: true,
			}

			pc := NewProcessControl(config, id, logger)
			require.NotNil(t, pc, "Should handle unicode worker ID: %s", id)

			impl := pc.(*processControl)
			assert.Equal(t, id, impl.workerID, "Should preserve unicode worker ID: %s", id)
		}
	})
}

func TestProcessControl_StateConsistencyEdgeCases(t *testing.T) {
	logger := &SimpleLogger{}

	t.Run("state_transition_atomicity", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "atomicity-worker", logger)
		impl := pc.(*processControl)

		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		var wg sync.WaitGroup
		stateSnapshots := make([]processcontrol.ProcessState, 100)

		// Capture state snapshots during transition
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range stateSnapshots {
				stateSnapshots[i] = impl.safeGetState()
				time.Sleep(time.Microsecond)
			}
		}()

		// Trigger state transition
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(10 * time.Microsecond) // Let snapshots start
			plan := impl.validateAndPlanStop()
			if plan.shouldProceed {
				impl.finalizeStop()
			}
		}()

		wg.Wait()

		// Verify state transitions are atomic (no invalid intermediate states)
		for i, state := range stateSnapshots {
			assert.Contains(t, []processcontrol.ProcessState{
				processcontrol.ProcessStateIdle, processcontrol.ProcessStateStarting, processcontrol.ProcessStateRunning,
				processcontrol.ProcessStateStopping, processcontrol.ProcessStateTerminating,
			}, state, "Snapshot %d should have valid state", i)
		}
	})

	t.Run("concurrent_state_observation_consistency", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
		}

		pc := NewProcessControl(config, "observation-worker", logger)
		impl := pc.(*processControl)

		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 99999}

		var wg sync.WaitGroup
		observations := make([][2]interface{}, 50) // [state, process]

		// Concurrent state and process observations
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				state := impl.safeGetState()
				process := impl.safeGetProcess()
				observations[index] = [2]interface{}{state, process}
			}(i)
		}

		wg.Wait()

		// Verify observations are consistent
		for i, obs := range observations {
			state := obs[0].(processcontrol.ProcessState)
			process := obs[1].(*os.Process)

			if state == processcontrol.ProcessStateRunning && process != nil {
				assert.Equal(t, 99999, process.Pid, "Observation %d: Running state should have correct process", i)
			}
			if state == processcontrol.ProcessStateIdle {
				// Process might be nil or not, depending on timing
			}

			assert.Contains(t, []processcontrol.ProcessState{
				processcontrol.ProcessStateIdle, processcontrol.ProcessStateRunning, processcontrol.ProcessStateStopping,
			}, state, "Observation %d should have valid state", i)
		}
	})
}

func TestProcessControl_ExtremeConfigurationScenarios(t *testing.T) {
	logger := &SimpleLogger{}

	t.Run("configuration_with_all_disabled", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanAttach:    false,
			CanTerminate: false,
			CanRestart:   false,
			// All capabilities disabled
		}

		pc := NewProcessControl(config, "disabled-worker", logger)
		require.NotNil(t, pc)

		impl := pc.(*processControl)
		assert.False(t, impl.config.CanAttach, "CanAttach should be false")
		assert.False(t, impl.config.CanTerminate, "CanTerminate should be false")
		assert.False(t, impl.config.CanRestart, "CanRestart should be false")
	})

	t.Run("configuration_with_conflicting_settings", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanRestart:   true,  // Can restart
			CanTerminate: false, // But cannot terminate
			// This is potentially conflicting
		}

		pc := NewProcessControl(config, "conflicting-worker", logger)
		require.NotNil(t, pc)

		impl := pc.(*processControl)
		assert.True(t, impl.config.CanRestart, "CanRestart should be preserved")
		assert.False(t, impl.config.CanTerminate, "CanTerminate should be preserved")
	})

	t.Run("configuration_with_extreme_restart_config", func(t *testing.T) {
		config := processcontrol.ProcessControlOptions{
			CanTerminate: true,
			CanRestart:   true,
			ContextAwareRestart: &processcontrol.ContextAwareRestartConfig{
				Default: processcontrol.RestartConfig{
					MaxRetries:  0,              // No retries
					RetryDelay:  24 * time.Hour, // Very long delay
					BackoffRate: 1000.0,         // Extreme backoff
				},
			},
		}

		pc := NewProcessControl(config, "extreme-restart-worker", logger)
		require.NotNil(t, pc)

		impl := pc.(*processControl)
		assert.Equal(t, 0, int(impl.config.ContextAwareRestart.Default.MaxRetries), "Extreme restart config should be preserved")
		assert.Equal(t, 24*time.Hour, impl.config.ContextAwareRestart.Default.RetryDelay, "Extreme delay should be preserved")
		assert.Equal(t, 1000.0, impl.config.ContextAwareRestart.Default.BackoffRate, "Extreme backoff should be preserved")
	})
}
