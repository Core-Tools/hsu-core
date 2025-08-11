package processmanager

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/process"
	"github.com/core-tools/hsu-core/pkg/processmanager/workerstatemachine"
	"github.com/core-tools/hsu-core/pkg/workers"
	"github.com/core-tools/hsu-core/pkg/workers/processcontrol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockWorker is a mock implementation of Worker for testing
type MockWorker struct {
	mock.Mock
}

func (m *MockWorker) ID() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockWorker) Metadata() workers.UnitMetadata {
	args := m.Called()
	return args.Get(0).(workers.UnitMetadata)
}

func (m *MockWorker) ProcessControlOptions() processcontrol.ProcessControlOptions {
	args := m.Called()
	return args.Get(0).(processcontrol.ProcessControlOptions)
}

// MockLogger is a mock implementation of Logger for testing
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) LogLevelf(level int, format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockLogger) Debugf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockLogger) Infof(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockLogger) Warnf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockLogger) Errorf(format string, args ...interface{}) {
	m.Called(format, args)
}

func createTestProcessManager(t *testing.T) *processManager {
	logger := &MockLogger{}
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Warnf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	return &processManager{
		logger:  logger,
		workers: make(map[string]*workerEntry), // Updated to use new combined map
	}
}

func createTestWorker(id string) *MockWorker {
	// Use OS-dependent path for PID file
	var pidFile string
	if runtime.GOOS == "windows" {
		pidFile = fmt.Sprintf("C:\\temp\\%s.pid", id)
	} else {
		pidFile = fmt.Sprintf("/tmp/%s.pid", id)
	}

	worker := &MockWorker{}
	worker.On("ID").Return(id)
	worker.On("Metadata").Return(workers.UnitMetadata{
		Name:        id,
		Description: fmt.Sprintf("Test worker %s", id),
	}).Maybe() // Make this optional since AddWorker doesn't call Metadata()

	// Create a mock logger for the test
	mockLogger := &MockLogger{}
	mockLogger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Infof", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Warnf", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	discovery := process.DiscoveryConfig{
		Method:  process.DiscoveryMethodPIDFile,
		PIDFile: pidFile,
	}
	attachCmd := func(ctx context.Context) (*processcontrol.CommandResult, error) {
		stdCmd := process.NewStdAttachCmd(discovery, id, mockLogger)
		process, stdout, err := stdCmd(ctx)
		return &processcontrol.CommandResult{
			Process:           process,
			ProcessContext:    nil,
			Stdout:            stdout,
			HealthCheckConfig: nil,
		}, err
	}

	worker.On("ProcessControlOptions").Return(processcontrol.ProcessControlOptions{
		CanAttach:    true,
		CanTerminate: true,
		CanRestart:   true,
		AttachCmd:    attachCmd,
	})
	return worker
}

func TestProcessManager_AddWorker(t *testing.T) {
	t.Run("valid_worker", func(t *testing.T) {
		manager := createTestProcessManager(t)
		worker := createTestWorker("test-worker-1")

		err := manager.AddWorker(worker)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(manager.workers))
		worker.AssertExpectations(t)
	})

	t.Run("nil_worker", func(t *testing.T) {
		manager := createTestProcessManager(t)
		err := manager.AddWorker(nil)

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("duplicate_worker", func(t *testing.T) {
		manager := createTestProcessManager(t)
		worker1 := createTestWorker("test-worker-1")
		worker2 := createTestWorker("test-worker-1")

		err1 := manager.AddWorker(worker1)
		err2 := manager.AddWorker(worker2)

		assert.NoError(t, err1)
		require.Error(t, err2)
		assert.True(t, errors.IsConflictError(err2), "Expected ConflictError but got: %v", err2)
		assert.Equal(t, 1, len(manager.workers))

		// Clean up mock expectations
		worker1.AssertExpectations(t)
		worker2.AssertExpectations(t)
	})

	t.Run("invalid_worker_id", func(t *testing.T) {
		manager := createTestProcessManager(t)
		worker := &MockWorker{}
		worker.On("ID").Return("") // Empty ID
		// Don't set up ProcessControlOptions expectation since validation fails before it's called

		err := manager.AddWorker(worker)

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
		worker.AssertExpectations(t)
	})
}

func TestProcessManager_RemoveWorker(t *testing.T) {
	t.Run("valid_removal", func(t *testing.T) {
		manager := createTestProcessManager(t)

		// Add a worker
		worker := createTestWorker("test-worker-1")
		err := manager.AddWorker(worker)
		require.NoError(t, err)

		// Worker should be in 'registered' state, which is safe to remove
		err = manager.RemoveWorker("test-worker-1")
		assert.NoError(t, err)

		// Verify worker is removed
		_, err = manager.GetWorkerState("test-worker-1")
		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})

	t.Run("invalid_worker_id", func(t *testing.T) {
		manager := createTestProcessManager(t)
		err := manager.RemoveWorker("")

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("nonexistent_worker", func(t *testing.T) {
		manager := createTestProcessManager(t)
		err := manager.RemoveWorker("nonexistent-worker")

		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})

	t.Run("cannot_remove_running_worker", func(t *testing.T) {
		manager := createTestProcessManager(t)

		// Add a worker
		worker := createTestWorker("running-worker")
		err := manager.AddWorker(worker)
		require.NoError(t, err)

		// Manually transition to running state using proper sequence
		workerEntry, _, exists := manager.getWorkerAndManagerState("running-worker")
		require.True(t, exists)
		// registered -> starting -> running
		err = workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateStarting, "start", nil)
		require.NoError(t, err)
		err = workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateRunning, "start", nil)
		require.NoError(t, err)

		// Should not be able to remove running worker
		err = manager.RemoveWorker("running-worker")
		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
		assert.Contains(t, err.Error(), "cannot remove worker in state 'running'")
		assert.Contains(t, err.Error(), "worker must be stopped before removal")

		// Worker should still exist
		state, err := manager.GetWorkerState("running-worker")
		assert.NoError(t, err)
		assert.Equal(t, workerstatemachine.WorkerStateRunning, state)
	})

	t.Run("can_remove_stopped_worker", func(t *testing.T) {
		manager := createTestProcessManager(t)

		// Add a worker
		worker := createTestWorker("stopped-worker")
		err := manager.AddWorker(worker)
		require.NoError(t, err)

		// Manually transition to stopped state using proper sequence
		workerEntry, _, exists := manager.getWorkerAndManagerState("stopped-worker")
		require.True(t, exists)
		// registered -> starting -> running -> stopping -> stopped
		err = workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateStarting, "start", nil)
		require.NoError(t, err)
		err = workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateRunning, "start", nil)
		require.NoError(t, err)
		err = workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateStopping, "stop", nil)
		require.NoError(t, err)
		err = workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateStopped, "stop", nil)
		require.NoError(t, err)

		// Should be able to remove stopped worker
		err = manager.RemoveWorker("stopped-worker")
		assert.NoError(t, err)

		// Worker should be removed
		_, err = manager.GetWorkerState("stopped-worker")
		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})

	t.Run("can_remove_failed_worker", func(t *testing.T) {
		manager := createTestProcessManager(t)

		// Add a worker
		worker := createTestWorker("failed-worker")
		err := manager.AddWorker(worker)
		require.NoError(t, err)

		// Manually transition to failed state using proper sequence
		workerEntry, _, exists := manager.getWorkerAndManagerState("failed-worker")
		require.True(t, exists)
		// registered -> starting -> failed (start operation failed)
		err = workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateStarting, "start", nil)
		require.NoError(t, err)
		err = workerEntry.StateMachine.Transition(workerstatemachine.WorkerStateFailed, "start", fmt.Errorf("test failure"))
		require.NoError(t, err)

		// Should be able to remove failed worker
		err = manager.RemoveWorker("failed-worker")
		assert.NoError(t, err)

		// Worker should be removed
		_, err = manager.GetWorkerState("failed-worker")
		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})
}

func TestIsWorkerSafelyRemovable(t *testing.T) {
	tests := []struct {
		state    workerstatemachine.WorkerState
		expected bool
		reason   string
	}{
		{workerstatemachine.WorkerStateUnknown, true, "unknown state should be safe"},
		{workerstatemachine.WorkerStateRegistered, true, "registered workers have no process"},
		{workerstatemachine.WorkerStateStarting, false, "starting workers may have process"},
		{workerstatemachine.WorkerStateRunning, false, "running workers have active process"},
		{workerstatemachine.WorkerStateStopping, false, "stopping workers still have process"},
		{workerstatemachine.WorkerStateStopped, true, "stopped workers have no process"},
		{workerstatemachine.WorkerStateFailed, true, "failed workers have no process"},
		{workerstatemachine.WorkerStateRestarting, false, "restarting workers may have process"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := isWorkerSafelyRemovable(tt.state)
			assert.Equal(t, tt.expected, result, tt.reason)
		})
	}
}

func TestProcessManager_StartWorker(t *testing.T) {
	t.Run("nil_context", func(t *testing.T) {
		manager := createTestProcessManager(t)
		err := manager.StartWorker(nil, "test-worker-1")

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("invalid_worker_id", func(t *testing.T) {
		manager := createTestProcessManager(t)
		ctx := context.Background()
		err := manager.StartWorker(ctx, "")

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("nonexistent_worker", func(t *testing.T) {
		manager := createTestProcessManager(t)
		ctx := context.Background()
		err := manager.StartWorker(ctx, "nonexistent-worker")

		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})
}

func TestProcessManager_StopWorker(t *testing.T) {
	t.Run("nil_context", func(t *testing.T) {
		manager := createTestProcessManager(t)
		err := manager.StopWorker(nil, "test-worker-1")

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("invalid_worker_id", func(t *testing.T) {
		manager := createTestProcessManager(t)
		ctx := context.Background()
		err := manager.StopWorker(ctx, "")

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("nonexistent_worker", func(t *testing.T) {
		manager := createTestProcessManager(t)
		ctx := context.Background()
		err := manager.StopWorker(ctx, "nonexistent-worker")

		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})
}

func TestProcessManager_ConcurrentOperations(t *testing.T) {
	manager := createTestProcessManager(t)

	// Test concurrent AddWorker operations
	done := make(chan bool)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			worker := createTestWorker(fmt.Sprintf("worker-%d", id))
			err := manager.AddWorker(worker)
			errors <- err
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check that all workers were added successfully
	errorCount := 0
	for i := 0; i < 10; i++ {
		if err := <-errors; err != nil {
			errorCount++
		}
	}

	assert.Equal(t, 0, errorCount, "No errors should occur during concurrent AddWorker operations")
	assert.Equal(t, 10, len(manager.workers))
}

func TestProcessManager_ContextCancellation(t *testing.T) {
	manager := createTestProcessManager(t)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context immediately
	cancel()

	// Operations with cancelled context should handle it gracefully
	err := manager.StartWorker(ctx, "test-worker")
	assert.Error(t, err)
	// The actual error depends on whether the worker exists or not
	// If worker doesn't exist, we get NotFoundError before checking context
	assert.True(t, errors.IsNotFoundError(err) || errors.IsCancelledError(err))
}

func TestProcessManager_ContextTimeout(t *testing.T) {
	manager := createTestProcessManager(t)

	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Wait for context to timeout
	time.Sleep(2 * time.Millisecond)

	// Operations with timed out context should handle it gracefully
	err := manager.StartWorker(ctx, "test-worker")
	assert.Error(t, err)
	// The actual error depends on whether the worker exists or not
	assert.True(t, errors.IsNotFoundError(err) || errors.IsCancelledError(err))
}
