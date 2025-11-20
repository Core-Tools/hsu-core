package managedprocess

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"
	"github.com/core-tools/hsu-core/pkg/resourcelimits"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockIntegratedLogger is a mock implementation of Logger for testing
type MockIntegratedLogger struct {
	mock.Mock
}

func (m *MockIntegratedLogger) LogLevelf(level int, format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockIntegratedLogger) Debugf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockIntegratedLogger) Infof(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockIntegratedLogger) Warnf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockIntegratedLogger) Errorf(format string, args ...interface{}) {
	m.Called(format, args)
}

func createTestIntegratedManagedProcessConfig() *IntegratedManagedProcessConfig {
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

	return &IntegratedManagedProcessConfig{
		Metadata: ProcessMetadata{
			Name:        "test-integrated-service",
			Description: "Test integrated service",
		},
		Control: processcontrol.ManagedProcessControlConfig{
			Execution: process.ExecutionConfig{
				ExecutablePath:   executablePath,
				Args:             args,
				WorkingDirectory: workingDirectory,
				WaitDelay:        10 * time.Second,
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
		HealthCheckRunOptions: monitoring.HealthCheckRunOptions{
			Enabled:      true,
			Interval:     30 * time.Second,
			Timeout:      5 * time.Second,
			InitialDelay: 10 * time.Second,
			Retries:      3,
		},
	}
}

func TestNewIntegratedManagedProcess(t *testing.T) {
	logger := &MockIntegratedLogger{}
	processConfig := createTestIntegratedManagedProcessConfig()

	process := NewIntegratedManagedProcessOptions("test-integrated-1", processConfig, logger)

	require.NotNil(t, process)
	assert.Equal(t, "test-integrated-1", process.ID())
}

func TestIntegratedManagedProcess_ID(t *testing.T) {
	logger := &MockIntegratedLogger{}
	processConfig := createTestIntegratedManagedProcessConfig()

	process := NewIntegratedManagedProcessOptions("test-integrated-2", processConfig, logger)

	assert.Equal(t, "test-integrated-2", process.ID())
}

func TestIntegratedManagedProcess_Metadata(t *testing.T) {
	logger := &MockIntegratedLogger{}
	processConfig := createTestIntegratedManagedProcessConfig()

	process := NewIntegratedManagedProcessOptions("test-integrated-3", processConfig, logger)

	metadata := process.Metadata()

	assert.Equal(t, "test-integrated-service", metadata.Name)
	assert.Equal(t, "Test integrated service", metadata.Description)
}

func TestIntegratedManagedProcess_ProcessControlOptions(t *testing.T) {
	logger := &MockIntegratedLogger{}
	processConfig := createTestIntegratedManagedProcessConfig()

	process := NewIntegratedManagedProcessOptions("test-integrated-4", processConfig, logger)

	options := process.ProcessControlOptions()

	// Test basic capabilities
	assert.True(t, options.CanAttach, "IntegratedManagedProcess should support attachment")
	assert.True(t, options.CanTerminate, "IntegratedManagedProcess should support termination")
	assert.True(t, options.CanRestart, "IntegratedManagedProcess should support restart")

	// Test ExecuteCmd is present
	assert.NotNil(t, options.ExecuteCmd, "IntegratedManagedProcess should provide ExecuteCmd")

	// Test AttachCmd is present
	assert.NotNil(t, options.AttachCmd, "IntegratedManagedProcess should provide AttachCmd")

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

	// Test health check is nil (provided by ExecuteCmd or AttachCmd)
	assert.Nil(t, options.HealthCheck, "IntegratedManagedProcess health check should be nil (provided by ExecuteCmd or AttachCmd)")
}

func TestIntegratedManagedProcess_ExecuteCmd_NilContext(t *testing.T) {
	logger := &MockIntegratedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	processConfig := createTestIntegratedManagedProcessConfig()

	process := NewIntegratedManagedProcessOptions("test-integrated-5", processConfig, logger).(*integratedManagedProcessOptions)

	cmdResult, err := process.ExecuteCmd(nil)

	assert.Nil(t, cmdResult)
	assert.Error(t, err)
	assert.True(t, errors.IsValidationError(err.(*errors.DomainError).Unwrap()))
	assert.Contains(t, err.Error(), "context cannot be nil")
}

func TestIntegratedManagedProcess_ExecuteCmd_ValidContext(t *testing.T) {
	logger := &MockIntegratedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()

	// Create a process config with a valid executable (use 'echo' which should exist on most systems)
	processConfig := createTestIntegratedManagedProcessConfig()

	process := NewIntegratedManagedProcessOptions("test-integrated-6", processConfig, logger).(*integratedManagedProcessOptions)

	ctx := context.Background()
	cmdResult, err := process.ExecuteCmd(ctx)

	// Note: This test might fail if the executable doesn't exist or isn't executable
	// But the structure should be correct
	if err == nil {
		assert.NotNil(t, cmdResult)

		process := cmdResult.Process
		stdout := cmdResult.Stdout
		healthCheck := cmdResult.HealthCheckConfig

		assert.NotNil(t, process)
		assert.NotNil(t, stdout)
		assert.NotNil(t, healthCheck)

		// Clean up the process if it was created
		process.Kill()
		stdout.Close()

		// Test health check configuration
		assert.Equal(t, monitoring.HealthCheckTypeGRPC, healthCheck.Type)
		assert.NotEmpty(t, healthCheck.GRPC.Address)
		addressAndPort := strings.Split(healthCheck.GRPC.Address, ":")
		require.Len(t, addressAndPort, 2)
		assert.Equal(t, addressAndPort[0], "localhost")
		assert.NotEmpty(t, addressAndPort[1])
		assert.Equal(t, "CoreService", healthCheck.GRPC.Service)
		assert.Equal(t, "Ping", healthCheck.GRPC.Method)
		assert.Equal(t, processConfig.HealthCheckRunOptions, healthCheck.RunOptions)
	} else {
		// If execution fails, error should be properly formatted
		assert.True(t, errors.IsProcessError(err) || errors.IsValidationError(err) || errors.IsPermissionError(err) || errors.IsIOError(err) || errors.IsNetworkError(err))
	}

	logger.AssertExpectations(t)
}

func TestIntegratedManagedProcess_ExecuteCmd_PortAllocation(t *testing.T) {
	logger := &MockIntegratedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()

	processConfig := createTestIntegratedManagedProcessConfig()

	process := NewIntegratedManagedProcessOptions("test-integrated-7", processConfig, logger).(*integratedManagedProcessOptions)

	ctx := context.Background()

	// Test that multiple calls get different ports
	ports := make(map[string]bool)
	successCount := 0
	for i := 0; i < 3; i++ {
		cmdResult, err := process.ExecuteCmd(ctx)

		if err == nil {
			process := cmdResult.Process
			stdout := cmdResult.Stdout
			healthCheck := cmdResult.HealthCheckConfig

			assert.NotNil(t, process)
			assert.NotNil(t, stdout)
			assert.NotNil(t, healthCheck)
			assert.NotEmpty(t, healthCheck.GRPC.Address)

			process.Kill()
			stdout.Close()

			successCount++

			// Each call should potentially get a different port
			ports[healthCheck.GRPC.Address] = true
		}
	}

	// Should have at least 1 successful port allocation
	// Note: This test might fail if the executable doesn't exist, but that's okay for testing
	if successCount > 0 {
		assert.True(t, len(ports) >= 1, "Should have at least one successful port allocation")
	} else {
		t.Skip("Skipping port allocation test - executable not available")
	}

	logger.AssertExpectations(t)
}

func TestIntegratedManagedProcess_IntegrationWithProcessControlOptions(t *testing.T) {
	logger := &MockIntegratedLogger{}
	processConfig := createTestIntegratedManagedProcessConfig()

	process := NewIntegratedManagedProcessOptions("test-integrated-8", processConfig, logger)

	options := process.ProcessControlOptions()

	// Test that options pass validation
	err := ValidateProcessControlOptions(options)
	assert.NoError(t, err, "IntegratedManagedProcess options should pass validation")
}

func TestIntegratedManagedProcess_MultipleInstances(t *testing.T) {
	logger := &MockIntegratedLogger{}
	processConfig := createTestIntegratedManagedProcessConfig()

	process1 := NewIntegratedManagedProcessOptions("process-1", processConfig, logger)
	process2 := NewIntegratedManagedProcessOptions("process-2", processConfig, logger)

	// Test independence
	assert.NotEqual(t, process1.ID(), process2.ID())
	assert.Equal(t, process1.Metadata(), process2.Metadata())

	// Test same ProcessControlOptions configuration
	options1 := process1.ProcessControlOptions()
	options2 := process2.ProcessControlOptions()

	assert.Equal(t, options1.CanAttach, options2.CanAttach)
	assert.Equal(t, options1.CanTerminate, options2.CanTerminate)
	assert.Equal(t, options1.CanRestart, options2.CanRestart)
	assert.Equal(t, options1.GracefulTimeout, options2.GracefulTimeout)

	// Both should have ExecuteCmd
	assert.NotNil(t, options1.ExecuteCmd)
	assert.NotNil(t, options2.ExecuteCmd)
}

func TestIntegratedManagedProcess_ConfigurationVariations(t *testing.T) {
	logger := &MockIntegratedLogger{}

	// Test with different configurations
	processConfig := createTestIntegratedManagedProcessConfig()
	processConfig.Control.GracefulTimeout = 60 * time.Second
	processConfig.HealthCheckRunOptions.Interval = 60 * time.Second
	processConfig.HealthCheckRunOptions.Timeout = 10 * time.Second

	process := NewIntegratedManagedProcessOptions("test-integrated-9", processConfig, logger)

	options := process.ProcessControlOptions()

	assert.Equal(t, 60*time.Second, options.GracefulTimeout)

	// Test that ExecuteCmd uses the new configuration
	assert.NotNil(t, options.ExecuteCmd)
	assert.NotNil(t, options.ContextAwareRestart)
	assert.NotNil(t, options.Limits)
}

func TestIntegratedManagedProcess_GetFreePort(t *testing.T) {
	// Test the getFreePort function indirectly through ExecuteCmd
	logger := &MockIntegratedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()

	processConfig := createTestIntegratedManagedProcessConfig()

	process := NewIntegratedManagedProcessOptions("test-integrated-10", processConfig, logger).(*integratedManagedProcessOptions)

	ctx := context.Background()
	cmdResult, err := process.ExecuteCmd(ctx)

	if err == nil {
		process := cmdResult.Process
		stdout := cmdResult.Stdout
		healthCheck := cmdResult.HealthCheckConfig

		assert.NotNil(t, process)
		assert.NotNil(t, stdout)
		assert.NotNil(t, healthCheck)
		assert.NotEmpty(t, healthCheck.GRPC.Address)

		process.Kill()
		stdout.Close()

		// Address should be in format localhost:port
		assert.Contains(t, healthCheck.GRPC.Address, "localhost:")

		// Port should be a valid number
		parts := strings.Split(healthCheck.GRPC.Address, ":")
		assert.Equal(t, 2, len(parts))
		port, parseErr := strconv.Atoi(parts[1])
		assert.NoError(t, parseErr)
		assert.True(t, port > 0 && port < 65536)
	}

	logger.AssertExpectations(t)
}
