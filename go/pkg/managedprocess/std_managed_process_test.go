package managedprocess

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"
	"github.com/core-tools/hsu-core/pkg/processfile"
	"github.com/core-tools/hsu-core/pkg/resourcelimits"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockLogger for testing
type MockManagedLogger struct {
	mock.Mock
}

func (m *MockManagedLogger) LogLevelf(level int, format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockManagedLogger) Debugf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockManagedLogger) Infof(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockManagedLogger) Warnf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockManagedLogger) Errorf(format string, args ...interface{}) {
	m.Called(format, args)
}

func createTestManagedUnit() *ManagedUnit {
	var executablePath string
	var args []string
	var workingDirectory string

	// Set platform-specific defaults
	if runtime.GOOS == "windows" {
		executablePath = "C:\\Windows\\System32\\cmd.exe"
		args = []string{"/c", "echo", "test"}
		workingDirectory = "C:\\Windows\\Temp"
	} else {
		executablePath = "/bin/echo"
		args = []string{"test"}
		workingDirectory = "/tmp"
	}

	return &ManagedUnit{
		Metadata: ProcessMetadata{
			Name:        "test-managed-unit",
			Description: "Test managed unit",
		},
		Control: processcontrol.ManagedProcessControlConfig{
			Execution: process.ExecutionConfig{
				ExecutablePath:   executablePath,
				Args:             args,
				WorkingDirectory: workingDirectory,
				WaitDelay:        5 * time.Second,
			},
			RestartPolicy: processcontrol.RestartOnFailure,
			ContextAwareRestart: processcontrol.ContextAwareRestartConfig{
				Default: processcontrol.RestartConfig{
					MaxRetries:  3,
					RetryDelay:  10 * time.Second,
					BackoffRate: 2.0,
				},
			},
			Limits: resourcelimits.ResourceLimits{
				Memory: &resourcelimits.MemoryLimits{
					MaxRSS: 1024 * 1024 * 1024, // 1GB
				},
				Process: &resourcelimits.ProcessLimits{
					MaxProcesses:       10,
					MaxFileDescriptors: 100,
				},
			},
			GracefulTimeout: 30 * time.Second,
		},
		HealthCheck: monitoring.HealthCheckConfig{
			Type: monitoring.HealthCheckTypeProcess,
			RunOptions: monitoring.HealthCheckRunOptions{
				Enabled:      true,
				Interval:     30 * time.Second,
				Timeout:      5 * time.Second,
				InitialDelay: 10 * time.Second,
				Retries:      3,
			},
		},
	}
}

func TestNewManagedWorker(t *testing.T) {
	logger := &MockManagedLogger{}
	unit := createTestManagedUnit()

	worker := NewStandardManagedProcessDescription("test-managed-1", unit, logger)

	assert.NotNil(t, worker)
	assert.Equal(t, "test-managed-1", worker.ID())
}

func TestManagedWorker_ID(t *testing.T) {
	logger := &MockManagedLogger{}
	unit := createTestManagedUnit()

	worker := NewStandardManagedProcessDescription("test-managed-2", unit, logger)

	assert.Equal(t, "test-managed-2", worker.ID())
}

func TestManagedWorker_Metadata(t *testing.T) {
	logger := &MockManagedLogger{}
	unit := createTestManagedUnit()

	worker := NewStandardManagedProcessDescription("test-managed-3", unit, logger)

	metadata := worker.Metadata()
	assert.Equal(t, "test-managed-unit", metadata.Name)
	assert.Equal(t, "Test managed unit", metadata.Description)
}

func TestManagedWorker_ProcessControlOptions(t *testing.T) {
	logger := &MockManagedLogger{}
	unit := createTestManagedUnit()

	worker := NewStandardManagedProcessDescription("test-managed-4", unit, logger)

	options := worker.ProcessControlOptions()

	// Test basic capabilities
	assert.True(t, options.CanAttach, "ManagedWorker should support attachment")
	assert.True(t, options.CanTerminate, "ManagedWorker should support termination")
	assert.True(t, options.CanRestart, "ManagedWorker should support restart")

	// Test ExecuteCmd is present
	assert.NotNil(t, options.ExecuteCmd, "ManagedWorker should provide ExecuteCmd")

	// Test AttachCmd is present
	assert.NotNil(t, options.AttachCmd, "ManagedWorker should provide AttachCmd")

	// Test restart configuration
	require.NotNil(t, options.ContextAwareRestart)
	assert.Equal(t, processcontrol.RestartOnFailure, options.RestartPolicy)
	assert.Equal(t, 3, options.ContextAwareRestart.Default.MaxRetries)
	assert.Equal(t, 10*time.Second, options.ContextAwareRestart.Default.RetryDelay)
	assert.Equal(t, 2.0, options.ContextAwareRestart.Default.BackoffRate)

	// Test resource limits
	require.NotNil(t, options.Limits)
	require.NotNil(t, options.Limits.Memory)
	require.NotNil(t, options.Limits.Process)
	assert.Equal(t, int64(1024*1024*1024), options.Limits.Memory.MaxRSS)
	assert.Equal(t, 10, options.Limits.Process.MaxProcesses)
	assert.Equal(t, 100, options.Limits.Process.MaxFileDescriptors)

	// Test graceful timeout
	assert.Equal(t, 30*time.Second, options.GracefulTimeout)

	// Test health check is provided by ExecuteCmd or AttachCmd
	assert.Nil(t, options.HealthCheck, "ManagedWorker should provide health check via ExecuteCmd or AttachCmd")
}

func TestManagedWorker_ExecuteCmd_NilContext(t *testing.T) {
	logger := &MockManagedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	unit := createTestManagedUnit()

	worker := NewStandardManagedProcessDescription("test-managed-6", unit, logger).(*standardManagedProcessDescription)

	cmdResult, err := worker.ExecuteCmd(nil)

	assert.Nil(t, cmdResult)
	assert.Error(t, err)
	assert.True(t, errors.IsValidationError(err.(*errors.DomainError).Unwrap()))
}

func TestManagedWorker_ExecuteCmd_ValidContext(t *testing.T) {
	logger := &MockManagedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	// Create a unit with platform-appropriate executable (handled by createTestManagedUnit)
	unit := createTestManagedUnit()

	worker := NewStandardManagedProcessDescription("test-managed-7", unit, logger).(*standardManagedProcessDescription)

	ctx := context.Background()
	cmdResult, err := worker.ExecuteCmd(ctx)

	// Note: This test might fail if the executable doesn't exist or isn't executable
	// But the structure should be correct
	if err == nil {
		process := cmdResult.Process
		stdout := cmdResult.Stdout
		healthCheck := cmdResult.HealthCheckConfig

		assert.NotNil(t, process)
		assert.NotNil(t, stdout)
		assert.NotNil(t, healthCheck)
		assert.Equal(t, monitoring.HealthCheckTypeProcess, healthCheck.Type)

		process.Kill()
		stdout.Close()
	} else {
		// If execution fails, error should be properly formatted
		assert.True(t, errors.IsProcessError(err) || errors.IsValidationError(err) || errors.IsPermissionError(err) || errors.IsIOError(err))
	}

	logger.AssertExpectations(t)
}

func TestManagedWorker_ExecuteCmd_PIDFileWriting(t *testing.T) {
	// Create a temporary directory for PID files
	tempDir := t.TempDir()

	pidConfig := processfile.ProcessFileConfig{
		BaseDirectory:   tempDir,
		ServiceContext:  processfile.UserService,
		AppName:         "test-app",
		UseSubdirectory: false,
	}

	var executablePath string
	var args []string
	var workingDirectory string

	// Set platform-specific defaults
	if runtime.GOOS == "windows" {
		executablePath = "C:\\Windows\\System32\\cmd.exe"
		args = []string{"/c", "echo", "test"}
		workingDirectory = "C:\\Windows\\Temp"
	} else {
		executablePath = "/bin/echo"
		args = []string{"test"}
		workingDirectory = "/tmp"
	}

	// Create test unit with PID file configuration
	unit := &ManagedUnit{
		Metadata: ProcessMetadata{
			Name:        "Test managed process",
			Description: "Test managed process for PID file testing",
		},
		Control: processcontrol.ManagedProcessControlConfig{
			Execution: process.ExecutionConfig{
				ExecutablePath:   executablePath,
				Args:             args,
				WorkingDirectory: workingDirectory,
				Environment:      []string{},
				WaitDelay:        5 * time.Second,
			},
			ProcessFile:     pidConfig,
			GracefulTimeout: 30 * time.Second,
			RestartPolicy:   processcontrol.RestartOnFailure,
			ContextAwareRestart: processcontrol.ContextAwareRestartConfig{
				Default: processcontrol.RestartConfig{
					MaxRetries:  3,
					RetryDelay:  5 * time.Second,
					BackoffRate: 2.0,
				},
			},
			Limits: resourcelimits.ResourceLimits{
				Memory: &resourcelimits.MemoryLimits{
					MaxRSS: 2048 * 1024 * 1024, // 2GB
				},
				Process: &resourcelimits.ProcessLimits{
					MaxProcesses:       20,
					MaxFileDescriptors: 200,
				},
			},
		},
		HealthCheck: monitoring.HealthCheckConfig{
			Type: monitoring.HealthCheckTypeProcess,
			RunOptions: monitoring.HealthCheckRunOptions{
				Enabled:      true,
				Interval:     30 * time.Second,
				Timeout:      5 * time.Second,
				InitialDelay: 5 * time.Second,
				Retries:      2,
			},
		},
	}

	// Create worker
	logger := &MockManagedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	worker := NewStandardManagedProcessDescription("test-worker", unit, logger).(*standardManagedProcessDescription)

	// Execute command
	ctx := context.Background()
	cmdResult, err := worker.ExecuteCmd(ctx)

	// Verify command execution
	assert.NoError(t, err)
	require.NotNil(t, cmdResult)

	process := cmdResult.Process
	stdout := cmdResult.Stdout
	healthCheck := cmdResult.HealthCheckConfig

	assert.NotNil(t, process)
	assert.NotNil(t, stdout)
	assert.NotNil(t, healthCheck)

	// Verify PID file was created
	pidFilePath := worker.pidManager.GeneratePIDFilePath("test-worker")
	assert.FileExists(t, pidFilePath)

	// Verify PID file content
	content, err := os.ReadFile(pidFilePath)
	assert.NoError(t, err)
	expectedContent := fmt.Sprintf("%d\n", process.Pid)
	assert.Equal(t, expectedContent, string(content))

	// Clean up
	process.Kill()
	stdout.Close()
}

func TestManagedWorker_IntegrationWithProcessControlOptions(t *testing.T) {
	logger := &MockManagedLogger{}
	unit := createTestManagedUnit()

	worker := NewStandardManagedProcessDescription("test-managed-8", unit, logger)

	options := worker.ProcessControlOptions()

	// Test that options pass validation
	err := ValidateProcessControlOptions(options)
	assert.NoError(t, err, "ManagedWorker options should pass validation")
}

func TestManagedWorker_MultipleInstances(t *testing.T) {
	logger := &MockManagedLogger{}
	unit := createTestManagedUnit()

	worker1 := NewStandardManagedProcessDescription("worker-1", unit, logger)
	worker2 := NewStandardManagedProcessDescription("worker-2", unit, logger)

	// Test independence
	assert.NotEqual(t, worker1.ID(), worker2.ID())
	assert.Equal(t, worker1.Metadata(), worker2.Metadata()) // Same unit, same metadata
}
