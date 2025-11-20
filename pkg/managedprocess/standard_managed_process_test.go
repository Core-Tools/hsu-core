package managedprocess

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/errors"
	"github.com/core-tools/hsu-procman-go/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-procman-go/pkg/monitoring"
	"github.com/core-tools/hsu-procman-go/pkg/process"
	"github.com/core-tools/hsu-procman-go/pkg/processfile"
	"github.com/core-tools/hsu-procman-go/pkg/resourcelimits"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockLogger for testing
type mockLogger struct {
	mock.Mock
}

func (m *mockLogger) LogLevelf(level int, format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *mockLogger) Debugf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *mockLogger) Infof(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *mockLogger) Warnf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *mockLogger) Errorf(format string, args ...interface{}) {
	m.Called(format, args)
}

func createTestStandardManagedProcessConfig() *StandardManagedProcessConfig {
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

	return &StandardManagedProcessConfig{
		Metadata: ProcessMetadata{
			Name:        "test-managed-process",
			Description: "Test managed process",
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

func TestNewStandardManagedProcess(t *testing.T) {
	logger := &mockLogger{}
	processConfig := createTestStandardManagedProcessConfig()

	process := NewStandardManagedProcessOptions("test-managed-1", processConfig, logger)

	assert.NotNil(t, process)
	assert.Equal(t, "test-managed-1", process.ID())
}

func TestStandardManagedProcess_ID(t *testing.T) {
	logger := &mockLogger{}
	processConfig := createTestStandardManagedProcessConfig()

	process := NewStandardManagedProcessOptions("test-managed-2", processConfig, logger)

	assert.Equal(t, "test-managed-2", process.ID())
}

func TestStandardManagedProcess_Metadata(t *testing.T) {
	logger := &mockLogger{}
	processConfig := createTestStandardManagedProcessConfig()

	process := NewStandardManagedProcessOptions("test-managed-3", processConfig, logger)

	metadata := process.Metadata()
	assert.Equal(t, "test-managed-process", metadata.Name)
	assert.Equal(t, "Test managed process", metadata.Description)
}

func TestStandardManagedProcess_ProcessControlOptions(t *testing.T) {
	logger := &mockLogger{}
	processConfig := createTestStandardManagedProcessConfig()

	process := NewStandardManagedProcessOptions("test-managed-4", processConfig, logger)

	options := process.ProcessControlOptions()

	// Test basic capabilities
	assert.True(t, options.CanAttach, "StandardManagedProcess should support attachment")
	assert.True(t, options.CanTerminate, "StandardManagedProcess should support termination")
	assert.True(t, options.CanRestart, "StandardManagedProcess should support restart")

	// Test ExecuteCmd is present
	assert.NotNil(t, options.ExecuteCmd, "StandardManagedProcess should provide ExecuteCmd")

	// Test AttachCmd is present
	assert.NotNil(t, options.AttachCmd, "StandardManagedProcess should provide AttachCmd")

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
	assert.Nil(t, options.HealthCheck, "StandardManagedProcess should provide health check via ExecuteCmd or AttachCmd")
}

func TestStandardManagedProcess_ExecuteCmd_NilContext(t *testing.T) {
	logger := &mockLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	processConfig := createTestStandardManagedProcessConfig()

	process := NewStandardManagedProcessOptions("test-managed-6", processConfig, logger).(*standardManagedProcessOptions)

	cmdResult, err := process.ExecuteCmd(nil)

	assert.Nil(t, cmdResult)
	assert.Error(t, err)
	assert.True(t, errors.IsValidationError(err.(*errors.DomainError).Unwrap()))
}

func TestStandardManagedProcess_ExecuteCmd_ValidContext(t *testing.T) {
	logger := &mockLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	// Create a process with platform-appropriate executable (handled by createTestStandardManagedProcessConfig)
	processConfig := createTestStandardManagedProcessConfig()

	process := NewStandardManagedProcessOptions("test-managed-7", processConfig, logger).(*standardManagedProcessOptions)

	ctx := context.Background()
	cmdResult, err := process.ExecuteCmd(ctx)

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

func TestStandardManagedProcess_ExecuteCmd_PIDFileWriting(t *testing.T) {
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

	// Create test process with PID file configuration
	processConfig := &StandardManagedProcessConfig{
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

	// Create process
	logger := &mockLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	process := NewStandardManagedProcessOptions("test-process", processConfig, logger).(*standardManagedProcessOptions)

	// Execute command
	ctx := context.Background()
	cmdResult, err := process.ExecuteCmd(ctx)

	// Verify command execution
	assert.NoError(t, err)
	require.NotNil(t, cmdResult)

	assert.NotNil(t, cmdResult.Process)
	assert.NotNil(t, cmdResult.Stdout)
	assert.NotNil(t, cmdResult.HealthCheckConfig)

	// Verify PID file was created
	pidFilePath := process.pidManager.GeneratePIDFilePath("test-process")
	assert.FileExists(t, pidFilePath)

	// Verify PID file content
	content, err := os.ReadFile(pidFilePath)
	assert.NoError(t, err)
	expectedContent := fmt.Sprintf("%d\n", cmdResult.Process.Pid)
	assert.Equal(t, expectedContent, string(content))

	// Clean up
	cmdResult.Process.Kill()
	cmdResult.Stdout.Close()
}

func TestStandardManagedProcess_IntegrationWithProcessControlOptions(t *testing.T) {
	logger := &mockLogger{}
	processConfig := createTestStandardManagedProcessConfig()

	process := NewStandardManagedProcessOptions("test-managed-8", processConfig, logger)

	options := process.ProcessControlOptions()

	// Test that options pass validation
	err := ValidateProcessControlOptions(options)
	assert.NoError(t, err, "StandardManagedProcess options should pass validation")
}

func TestStandardManagedProcess_MultipleInstances(t *testing.T) {
	logger := &mockLogger{}
	processConfig := createTestStandardManagedProcessConfig()

	process1 := NewStandardManagedProcessOptions("process-1", processConfig, logger)
	process2 := NewStandardManagedProcessOptions("process-2", processConfig, logger)

	// Test independence
	assert.NotEqual(t, process1.ID(), process2.ID())
	assert.Equal(t, process1.Metadata(), process2.Metadata())
}
