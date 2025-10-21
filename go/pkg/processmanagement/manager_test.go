package processmanagement

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/managedprocess"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/process"
	"github.com/core-tools/hsu-core/pkg/processmanagement/processstatemachine"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockProcess is a mock implementation of ProcessOptions for testing
type mockProcess struct {
	mock.Mock
}

func (m *mockProcess) ID() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockProcess) Metadata() managedprocess.ProcessMetadata {
	args := m.Called()
	return args.Get(0).(managedprocess.ProcessMetadata)
}

func (m *mockProcess) ProcessControlOptions() processcontrol.ProcessControlOptions {
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
		logger:    logger,
		processes: make(map[string]*processEntry),
	}
}

func createTestProcess(id string) *mockProcess {
	// Use OS-dependent path for PID file
	var pidFile string
	if runtime.GOOS == "windows" {
		pidFile = fmt.Sprintf("C:\\temp\\%s.pid", id)
	} else {
		pidFile = fmt.Sprintf("/tmp/%s.pid", id)
	}

	mockProcess := &mockProcess{}
	mockProcess.On("ID").Return(id)
	mockProcess.On("Metadata").Return(managedprocess.ProcessMetadata{
		Name:        id,
		Description: fmt.Sprintf("Test process %s", id),
	}).Maybe() // Make this optional since AddProcess doesn't call Metadata()

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
			Stdout:            stdout,
			HealthCheckConfig: nil,
		}, err
	}

	mockProcess.On("ProcessControlOptions").Return(processcontrol.ProcessControlOptions{
		CanAttach:    true,
		CanTerminate: true,
		CanRestart:   true,
		AttachCmd:    attachCmd,
	})
	return mockProcess
}

func TestProcessManager_AddProcess(t *testing.T) {
	t.Run("valid_process", func(t *testing.T) {
		manager := createTestProcessManager(t)
		mockProcess := createTestProcess("test-process-1")

		err := manager.AddProcess(mockProcess)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(manager.processes))
		mockProcess.AssertExpectations(t)
	})

	t.Run("nil_process", func(t *testing.T) {
		manager := createTestProcessManager(t)
		err := manager.AddProcess(nil)

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("duplicate_process", func(t *testing.T) {
		manager := createTestProcessManager(t)
		mockProcess1 := createTestProcess("test-process-1")
		mockProcess2 := createTestProcess("test-process-1")

		err1 := manager.AddProcess(mockProcess1)
		err2 := manager.AddProcess(mockProcess2)

		assert.NoError(t, err1)
		require.Error(t, err2)
		assert.True(t, errors.IsConflictError(err2), "Expected ConflictError but got: %v", err2)
		assert.Equal(t, 1, len(manager.processes))

		// Clean up mock expectations
		mockProcess1.AssertExpectations(t)
		mockProcess2.AssertExpectations(t)
	})

	t.Run("invalid_process_id", func(t *testing.T) {
		manager := createTestProcessManager(t)
		mockProcess := &mockProcess{}
		mockProcess.On("ID").Return("") // Empty ID
		// Don't set up ProcessControlOptions expectation since validation fails before it's called

		err := manager.AddProcess(mockProcess)

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
		mockProcess.AssertExpectations(t)
	})
}

func TestProcessManager_RemoveProcess(t *testing.T) {
	t.Run("valid_removal", func(t *testing.T) {
		manager := createTestProcessManager(t)

		// Add a process
		mockProcess := createTestProcess("test-process-1")
		err := manager.AddProcess(mockProcess)
		require.NoError(t, err)

		// Managed process should be in 'registered' state, which is safe to remove
		err = manager.RemoveProcess("test-process-1")
		assert.NoError(t, err)

		// Verify process is removed
		_, err = manager.GetProcessState("test-process-1")
		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})

	t.Run("invalid_process_id", func(t *testing.T) {
		manager := createTestProcessManager(t)
		err := manager.RemoveProcess("")

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("nonexistent_process", func(t *testing.T) {
		manager := createTestProcessManager(t)
		err := manager.RemoveProcess("nonexistent-process")

		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})

	t.Run("cannot_remove_running_process", func(t *testing.T) {
		manager := createTestProcessManager(t)

		// Add a process
		mockProcess := createTestProcess("running-process")
		err := manager.AddProcess(mockProcess)
		require.NoError(t, err)

		// Manually transition to running state using proper sequence
		processEntry, _, exists := manager.getProcessAndManagerState("running-process")
		require.True(t, exists)
		// registered -> starting -> running
		err = processEntry.StateMachine.Transition(processstatemachine.ProcessStateStarting, "start", nil)
		require.NoError(t, err)
		err = processEntry.StateMachine.Transition(processstatemachine.ProcessStateRunning, "start", nil)
		require.NoError(t, err)

		// Should not be able to remove running process
		err = manager.RemoveProcess("running-process")
		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
		assert.Contains(t, err.Error(), "cannot remove process in state 'running'")
		assert.Contains(t, err.Error(), "process must be stopped before removal")

		// Managed process should still exist
		state, err := manager.GetProcessState("running-process")
		assert.NoError(t, err)
		assert.Equal(t, processstatemachine.ProcessStateRunning, state)
	})

	t.Run("can_remove_stopped_process", func(t *testing.T) {
		manager := createTestProcessManager(t)

		// Add a process
		mockProcess := createTestProcess("stopped-process")
		err := manager.AddProcess(mockProcess)
		require.NoError(t, err)

		// Manually transition to stopped state using proper sequence
		processEntry, _, exists := manager.getProcessAndManagerState("stopped-process")
		require.True(t, exists)
		// registered -> starting -> running -> stopping -> stopped
		err = processEntry.StateMachine.Transition(processstatemachine.ProcessStateStarting, "start", nil)
		require.NoError(t, err)
		err = processEntry.StateMachine.Transition(processstatemachine.ProcessStateRunning, "start", nil)
		require.NoError(t, err)
		err = processEntry.StateMachine.Transition(processstatemachine.ProcessStateStopping, "stop", nil)
		require.NoError(t, err)
		err = processEntry.StateMachine.Transition(processstatemachine.ProcessStateStopped, "stop", nil)
		require.NoError(t, err)

		// Should be able to remove stopped process
		err = manager.RemoveProcess("stopped-process")
		assert.NoError(t, err)

		// Managed process should be removed
		_, err = manager.GetProcessState("stopped-process")
		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})

	t.Run("can_remove_failed_process", func(t *testing.T) {
		manager := createTestProcessManager(t)

		// Add a process
		mockProcess := createTestProcess("failed-process")
		err := manager.AddProcess(mockProcess)
		require.NoError(t, err)

		// Manually transition to failed state using proper sequence
		processEntry, _, exists := manager.getProcessAndManagerState("failed-process")
		require.True(t, exists)
		// registered -> starting -> failed (start operation failed)
		err = processEntry.StateMachine.Transition(processstatemachine.ProcessStateStarting, "start", nil)
		require.NoError(t, err)
		err = processEntry.StateMachine.Transition(processstatemachine.ProcessStateFailed, "start", fmt.Errorf("test failure"))
		require.NoError(t, err)

		// Should be able to remove failed process
		err = manager.RemoveProcess("failed-process")
		assert.NoError(t, err)

		// Managed process should be removed
		_, err = manager.GetProcessState("failed-process")
		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})
}

func TestIsProcessSafelyRemovable(t *testing.T) {
	tests := []struct {
		state    processstatemachine.ProcessState
		expected bool
		reason   string
	}{
		{processstatemachine.ProcessStateUnknown, true, "unknown state should be safe"},
		{processstatemachine.ProcessStateRegistered, true, "registered processes have no process"},
		{processstatemachine.ProcessStateStarting, false, "starting processes may have process"},
		{processstatemachine.ProcessStateRunning, false, "running processes have active process"},
		{processstatemachine.ProcessStateStopping, false, "stopping processes still have process"},
		{processstatemachine.ProcessStateStopped, true, "stopped processes have no process"},
		{processstatemachine.ProcessStateFailed, true, "failed processes have no process"},
		{processstatemachine.ProcessStateRestarting, false, "restarting processes may have process"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := isProcessSafelyRemovable(tt.state)
			assert.Equal(t, tt.expected, result, tt.reason)
		})
	}
}

func TestProcessManager_StartProcess(t *testing.T) {
	t.Run("nil_context", func(t *testing.T) {
		manager := createTestProcessManager(t)
		err := manager.StartProcess(nil, "test-process-1")

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("invalid_process_id", func(t *testing.T) {
		manager := createTestProcessManager(t)
		ctx := context.Background()
		err := manager.StartProcess(ctx, "")

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("nonexistent_process", func(t *testing.T) {
		manager := createTestProcessManager(t)
		ctx := context.Background()
		err := manager.StartProcess(ctx, "nonexistent-process")

		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})
}

func TestProcessManager_StopProcess(t *testing.T) {
	t.Run("nil_context", func(t *testing.T) {
		manager := createTestProcessManager(t)
		err := manager.StopProcess(nil, "test-process-1")

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("invalid_process_id", func(t *testing.T) {
		manager := createTestProcessManager(t)
		ctx := context.Background()
		err := manager.StopProcess(ctx, "")

		assert.Error(t, err)
		assert.True(t, errors.IsValidationError(err))
	})

	t.Run("nonexistent_process", func(t *testing.T) {
		manager := createTestProcessManager(t)
		ctx := context.Background()
		err := manager.StopProcess(ctx, "nonexistent-process")

		assert.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
	})
}

func TestProcessManager_ConcurrentOperations(t *testing.T) {
	manager := createTestProcessManager(t)

	// Test concurrent AddProcess operations
	done := make(chan bool)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			mockProcess := createTestProcess(fmt.Sprintf("process-%d", id))
			err := manager.AddProcess(mockProcess)
			errors <- err
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check that all processes were added successfully
	errorCount := 0
	for i := 0; i < 10; i++ {
		if err := <-errors; err != nil {
			errorCount++
		}
	}

	assert.Equal(t, 0, errorCount, "No errors should occur during concurrent AddProcess operations")
	assert.Equal(t, 10, len(manager.processes))
}

func TestProcessManager_ContextCancellation(t *testing.T) {
	manager := createTestProcessManager(t)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context immediately
	cancel()

	// Operations with cancelled context should handle it gracefully
	err := manager.StartProcess(ctx, "test-process")
	assert.Error(t, err)
	// The actual error depends on whether the process exists or not
	// If process doesn't exist, we get NotFoundError before checking context
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
	err := manager.StartProcess(ctx, "test-process")
	assert.Error(t, err)
	// The actual error depends on whether the process exists or not
	assert.True(t, errors.IsNotFoundError(err) || errors.IsCancelledError(err))
}
