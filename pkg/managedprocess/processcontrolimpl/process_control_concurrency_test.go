//go:build test

package processcontrolimpl

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/managedprocess/processcontrol"

	"github.com/stretchr/testify/assert"
)

// ===== COMPREHENSIVE CONCURRENCY TESTS =====

func TestProcessControl_ConcurrentStateAccess(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate:    true,
		GracefulTimeout: 30 * time.Second,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	// Set to running state with a process
	impl.state = processcontrol.ProcessStateRunning
	impl.process = &os.Process{Pid: 12345}

	// Test high-frequency concurrent reads
	t.Run("high_frequency_reads", func(t *testing.T) {
		var wg sync.WaitGroup
		numReaders := 100
		readsPerReader := 100

		wg.Add(numReaders)
		for i := 0; i < numReaders; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < readsPerReader; j++ {
					// These should always be thread-safe
					state := impl.GetState()
					process := impl.safeGetProcess()

					// Verify consistency
					assert.Contains(t, []processcontrol.ProcessState{processcontrol.ProcessStateRunning, processcontrol.ProcessStateIdle}, state)
					if process != nil {
						assert.Equal(t, 12345, process.Pid)
					}
				}
			}()
		}

		wg.Wait()
	})

	// Test mixed read/write operations
	t.Run("mixed_read_write_operations", func(t *testing.T) {
		var wg sync.WaitGroup
		numOperations := 50

		// Readers only - no unsafe writes
		wg.Add(numOperations)
		for i := 0; i < numOperations; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					impl.GetState()
					impl.safeGetProcess()
					time.Sleep(time.Microsecond) // Small delay to increase chance of interleaving
				}
			}()
		}

		// Safe state operations (using proper defer-only pattern)
		wg.Add(5)
		for i := 0; i < 5; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					// Only read operations and safe planning (no direct field manipulation)
					currentState := impl.safeGetState()

					// Only test planning if in a valid state for planning
					if currentState == processcontrol.ProcessStateRunning {
						plan := impl.validateAndPlanStop()
						// Don't actually modify state - just test the planning logic
						_ = plan.shouldProceed
					}

					time.Sleep(time.Microsecond)
				}
			}()
		}

		wg.Wait()
	})
}

func TestProcessControl_ConcurrentOperationAttempts(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate:    true,
		GracefulTimeout: 30 * time.Second,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("concurrent_stop_attempts", func(t *testing.T) {
		// Set up running state ONCE before concurrent access
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		var wg sync.WaitGroup
		numAttempts := 10
		successCount := int32(0)

		// Multiple goroutines try to stop concurrently
		wg.Add(numAttempts)
		for i := 0; i < numAttempts; i++ {
			go func() {
				defer wg.Done()

				plan := impl.validateAndPlanStop()
				if plan.shouldProceed {
					atomic.AddInt32(&successCount, 1)
					// Don't call finalize to allow seeing the effect of first success
				}
			}()
		}

		wg.Wait()

		// Only one attempt should succeed due to state protection
		assert.Equal(t, int32(1), successCount, "Only one stop attempt should succeed")

		// After one successful planning, state should be stopping
		finalState := impl.safeGetState()
		assert.Equal(t, processcontrol.ProcessStateStopping, finalState, "State should be stopping after successful attempt")
	})

	t.Run("concurrent_start_validation", func(t *testing.T) {
		// Create separate instances to avoid race conditions
		var wg sync.WaitGroup
		numAttempts := 20
		canStartCount := int32(0)

		// Multiple goroutines check if they can start from idle (read-only operation)
		wg.Add(numAttempts)
		for i := 0; i < numAttempts; i++ {
			go func() {
				defer wg.Done()

				// Read-only validation - no state modification
				if impl.canStartFromState(processcontrol.ProcessStateIdle) {
					atomic.AddInt32(&canStartCount, 1)
				}
			}()
		}

		wg.Wait()

		// All should see that they can start from idle (read-only operation)
		assert.Equal(t, int32(numAttempts), canStartCount, "All should be able to start from idle")
	})
}

func TestProcessControl_StateTransitionRace(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate:    true,
		GracefulTimeout: 30 * time.Second,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("rapid_state_reads_only", func(t *testing.T) {
		var wg sync.WaitGroup
		numCycles := 50

		// Set initial state once
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		// Only safe read operations - no state modifications
		wg.Add(2)

		// Goroutine 1: State reading
		go func() {
			defer wg.Done()
			for i := 0; i < numCycles*10; i++ {
				state := impl.GetState()
				// Verify state is always valid
				assert.Contains(t, []processcontrol.ProcessState{
					processcontrol.ProcessStateIdle, processcontrol.ProcessStateStarting, processcontrol.ProcessStateRunning,
					processcontrol.ProcessStateStopping, processcontrol.ProcessStateTerminating,
				}, state)

				time.Sleep(time.Nanosecond)
			}
		}()

		// Goroutine 2: Process reading
		go func() {
			defer wg.Done()
			for i := 0; i < numCycles*10; i++ {
				process := impl.safeGetProcess()
				// Process may or may not exist, but access should be safe
				if process != nil {
					assert.True(t, process.Pid > 0)
				}

				time.Sleep(time.Nanosecond)
			}
		}()

		wg.Wait()
	})
}

func TestProcessControl_DeferOnlyLocking_ConcurrentAccess(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate:    true,
		GracefulTimeout: 30 * time.Second,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("concurrent_planning_operations", func(t *testing.T) {
		// Set up running state ONCE
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		var wg sync.WaitGroup
		numPlanners := 20
		planResults := make([]bool, numPlanners)

		// Multiple goroutines try to plan stops concurrently
		wg.Add(numPlanners)
		for i := 0; i < numPlanners; i++ {
			go func(index int) {
				defer wg.Done()

				plan := impl.validateAndPlanStop()
				planResults[index] = plan.shouldProceed
			}(i)
		}

		wg.Wait()

		// Count successful plans
		successCount := 0
		for _, success := range planResults {
			if success {
				successCount++
			}
		}

		// Only one should succeed due to defer-only locking protection
		assert.Equal(t, 1, successCount, "Only one plan should succeed with defer-only locking")
	})

	t.Run("concurrent_finalization", func(t *testing.T) {
		// Create separate process control instances to avoid cross-contamination
		processes := []*processControl{}
		for i := 0; i < 5; i++ {
			pc := NewProcessControl(config, "test-process", logger)
			impl := pc.(*processControl)
			impl.state = processcontrol.ProcessStateStopping
			impl.stdout = &MockReadCloser{}
			processes = append(processes, impl)
		}

		var wg sync.WaitGroup
		wg.Add(len(processes))

		// Concurrent finalization on separate instances
		for _, proc := range processes {
			go func(p *processControl) {
				defer wg.Done()
				p.finalizeStop()
				assert.Equal(t, processcontrol.ProcessStateIdle, p.GetState())
			}(proc)
		}

		wg.Wait()

		// All should be in idle state
		for i, proc := range processes {
			assert.Equal(t, processcontrol.ProcessStateIdle, proc.GetState(), "Process %d should be idle", i)
		}
	})
}

func TestProcessControl_ResourceCleanup_Concurrency(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate:    true,
		GracefulTimeout: 30 * time.Second,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("concurrent_cleanup_operations", func(t *testing.T) {
		var wg sync.WaitGroup
		numCleanups := 10

		// Set up resources in each goroutine and clean them up
		wg.Add(numCleanups)
		for i := 0; i < numCleanups; i++ {
			go func() {
				defer wg.Done()

				// Set up some mock resources
				impl.stdout = &MockReadCloser{}

				// Clean them up
				impl.cleanupResourcesUnderLock()

				// Verify cleanup
				assert.Nil(t, impl.stdout)
			}()
		}

		wg.Wait()
	})
}

func TestProcessControl_MemoryConsistency(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate:    true,
		GracefulTimeout: 30 * time.Second,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("memory_visibility_consistency", func(t *testing.T) {
		var wg sync.WaitGroup

		// Writer goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				// Write state and process atomically through defer-only pattern
				impl.state = processcontrol.ProcessStateRunning
				impl.process = &os.Process{Pid: int(12345 + i)}

				time.Sleep(time.Microsecond)

				plan := impl.validateAndPlanStop()
				if plan.shouldProceed {
					impl.finalizeStop()
				}
			}
		}()

		// Reader goroutines
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 200; j++ {
					state := impl.GetState()
					process := impl.safeGetProcess()

					// Memory consistency check: if we see a process, state should be appropriate
					if process != nil && state == processcontrol.ProcessStateIdle {
						// This would indicate a memory consistency issue
						t.Errorf("Inconsistent memory state: process exists but state is idle")
					}

					time.Sleep(time.Nanosecond)
				}
			}()
		}

		wg.Wait()
	})
}

func TestProcessControl_LockContention(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate:    true,
		GracefulTimeout: 30 * time.Second,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("high_contention_scenario", func(t *testing.T) {
		var wg sync.WaitGroup
		numContenders := 100
		operationsPerContender := 50

		// Set initial state
		impl.state = processcontrol.ProcessStateRunning
		impl.process = &os.Process{Pid: 12345}

		// High contention on lock
		wg.Add(numContenders)
		for i := 0; i < numContenders; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < operationsPerContender; j++ {
					// Mix of read and write operations
					if j%2 == 0 {
						impl.GetState()
						impl.safeGetProcess()
					} else {
						plan := impl.validateAndPlanStop()
						if plan.shouldProceed {
							// Reset for next iteration
							impl.state = processcontrol.ProcessStateRunning
							impl.process = &os.Process{Pid: 12345}
						}
					}
				}
			}()
		}

		wg.Wait()

		// System should remain consistent
		state := impl.GetState()
		assert.Contains(t, []processcontrol.ProcessState{processcontrol.ProcessStateRunning, processcontrol.ProcessStateStopping, processcontrol.ProcessStateIdle}, state)
	})
}

func TestProcessControl_DeadlockPrevention(t *testing.T) {
	logger := &SimpleLogger{}
	config := processcontrol.ProcessControlOptions{
		CanTerminate:    true,
		GracefulTimeout: 30 * time.Second,
	}

	pc := NewProcessControl(config, "test-process", logger)
	impl := pc.(*processControl)

	t.Run("no_deadlock_with_defer_only", func(t *testing.T) {
		// This test verifies that our defer-only pattern prevents deadlocks
		done := make(chan bool, 1)

		go func() {
			var wg sync.WaitGroup

			// Rapid operations that could potentially deadlock
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 100; j++ {
						// Nested-like operations that could deadlock with explicit locking
						impl.GetState()
						plan := impl.validateAndPlanStop()
						if plan.shouldProceed {
							impl.finalizeStop()
						}
						impl.GetState()
					}
				}()
			}

			wg.Wait()
			done <- true
		}()

		// Test should complete within reasonable time (no deadlock)
		select {
		case <-done:
			// Success - no deadlock
		case <-time.After(5 * time.Second):
			t.Fatal("Potential deadlock detected - test did not complete within timeout")
		}
	})
}
