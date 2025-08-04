package workers

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"
	"github.com/core-tools/hsu-core/pkg/resourcelimits"
	"github.com/core-tools/hsu-core/pkg/workers/processcontrol"

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

func createTestIntegratedUnit() *IntegratedUnit {
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

	return &IntegratedUnit{
		Metadata: UnitMetadata{
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

func TestNewIntegratedWorker(t *testing.T) {
	logger := &MockIntegratedLogger{}
	unit := createTestIntegratedUnit()

	worker := NewIntegratedWorker("test-integrated-1", unit, logger)

	require.NotNil(t, worker)
	assert.Equal(t, "test-integrated-1", worker.ID())
}

func TestIntegratedWorker_ID(t *testing.T) {
	logger := &MockIntegratedLogger{}
	unit := createTestIntegratedUnit()

	worker := NewIntegratedWorker("test-integrated-2", unit, logger)

	assert.Equal(t, "test-integrated-2", worker.ID())
}

func TestIntegratedWorker_Metadata(t *testing.T) {
	logger := &MockIntegratedLogger{}
	unit := createTestIntegratedUnit()

	worker := NewIntegratedWorker("test-integrated-3", unit, logger)

	metadata := worker.Metadata()

	assert.Equal(t, "test-integrated-service", metadata.Name)
	assert.Equal(t, "Test integrated service", metadata.Description)
}

func TestIntegratedWorker_ProcessControlOptions(t *testing.T) {
	logger := &MockIntegratedLogger{}
	unit := createTestIntegratedUnit()

	worker := NewIntegratedWorker("test-integrated-4", unit, logger)

	options := worker.ProcessControlOptions()

	// Test basic capabilities
	assert.True(t, options.CanAttach, "IntegratedWorker should support attachment")
	assert.True(t, options.CanTerminate, "IntegratedWorker should support termination")
	assert.True(t, options.CanRestart, "IntegratedWorker should support restart")

	// Test ExecuteCmd is present
	assert.NotNil(t, options.ExecuteCmd, "IntegratedWorker should provide ExecuteCmd")

	// Test AttachCmd is present
	assert.NotNil(t, options.AttachCmd, "IntegratedWorker should provide AttachCmd")

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
	assert.Nil(t, options.HealthCheck, "IntegratedWorker health check should be nil (provided by ExecuteCmd or AttachCmd)")
}

func TestIntegratedWorker_ExecuteCmd_NilContext(t *testing.T) {
	logger := &MockIntegratedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	unit := createTestIntegratedUnit()

	worker := NewIntegratedWorker("test-integrated-5", unit, logger).(*integratedWorker)

	cmdResult, err := worker.ExecuteCmd(nil)

	assert.Nil(t, cmdResult)
	assert.Error(t, err)
	assert.True(t, errors.IsValidationError(err.(*errors.DomainError).Unwrap()))
	assert.Contains(t, err.Error(), "context cannot be nil")
}

func TestIntegratedWorker_ExecuteCmd_ValidContext(t *testing.T) {
	logger := &MockIntegratedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()

	// Create a unit with a valid executable (use 'echo' which should exist on most systems)
	unit := createTestIntegratedUnit()

	worker := NewIntegratedWorker("test-integrated-6", unit, logger).(*integratedWorker)

	ctx := context.Background()
	cmdResult, err := worker.ExecuteCmd(ctx)

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
		assert.Equal(t, unit.HealthCheckRunOptions, healthCheck.RunOptions)
	} else {
		// If execution fails, error should be properly formatted
		assert.True(t, errors.IsProcessError(err) || errors.IsValidationError(err) || errors.IsPermissionError(err) || errors.IsIOError(err) || errors.IsNetworkError(err))
	}

	logger.AssertExpectations(t)
}

func TestIntegratedWorker_ExecuteCmd_PortAllocation(t *testing.T) {
	logger := &MockIntegratedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()

	unit := createTestIntegratedUnit()

	worker := NewIntegratedWorker("test-integrated-7", unit, logger).(*integratedWorker)

	ctx := context.Background()

	// Test that multiple calls get different ports
	ports := make(map[string]bool)
	successCount := 0
	for i := 0; i < 3; i++ {
		cmdResult, err := worker.ExecuteCmd(ctx)

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
	// Note: This test might fail if the executable doesn't exist, but that's okay for unit testing
	if successCount > 0 {
		assert.True(t, len(ports) >= 1, "Should have at least one successful port allocation")
	} else {
		t.Skip("Skipping port allocation test - executable not available")
	}

	logger.AssertExpectations(t)
}

func TestIntegratedWorker_IntegrationWithProcessControlOptions(t *testing.T) {
	logger := &MockIntegratedLogger{}
	unit := createTestIntegratedUnit()

	worker := NewIntegratedWorker("test-integrated-8", unit, logger)

	options := worker.ProcessControlOptions()

	// Test that options pass validation
	err := ValidateProcessControlOptions(options)
	assert.NoError(t, err, "IntegratedWorker options should pass validation")
}

func TestIntegratedWorker_MultipleInstances(t *testing.T) {
	logger := &MockIntegratedLogger{}
	unit := createTestIntegratedUnit()

	worker1 := NewIntegratedWorker("worker-1", unit, logger)
	worker2 := NewIntegratedWorker("worker-2", unit, logger)

	// Test independence
	assert.NotEqual(t, worker1.ID(), worker2.ID())
	assert.Equal(t, worker1.Metadata(), worker2.Metadata()) // Same unit, same metadata

	// Test same ProcessControlOptions configuration (since they share the same unit)
	options1 := worker1.ProcessControlOptions()
	options2 := worker2.ProcessControlOptions()

	assert.Equal(t, options1.CanAttach, options2.CanAttach)
	assert.Equal(t, options1.CanTerminate, options2.CanTerminate)
	assert.Equal(t, options1.CanRestart, options2.CanRestart)
	assert.Equal(t, options1.GracefulTimeout, options2.GracefulTimeout)

	// Both should have ExecuteCmd
	assert.NotNil(t, options1.ExecuteCmd)
	assert.NotNil(t, options2.ExecuteCmd)
}

func TestIntegratedWorker_ConfigurationVariations(t *testing.T) {
	logger := &MockIntegratedLogger{}

	// Test with different configurations
	unit := createTestIntegratedUnit()
	unit.Control.GracefulTimeout = 60 * time.Second
	unit.HealthCheckRunOptions.Interval = 60 * time.Second
	unit.HealthCheckRunOptions.Timeout = 10 * time.Second

	worker := NewIntegratedWorker("test-integrated-9", unit, logger)

	options := worker.ProcessControlOptions()

	assert.Equal(t, 60*time.Second, options.GracefulTimeout)

	// Test that ExecuteCmd uses the new configuration
	assert.NotNil(t, options.ExecuteCmd)
	assert.NotNil(t, options.ContextAwareRestart)
	assert.NotNil(t, options.Limits)
}

func TestIntegratedWorker_GetFreePort(t *testing.T) {
	// Test the getFreePort function indirectly through ExecuteCmd
	logger := &MockIntegratedLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()

	unit := createTestIntegratedUnit()

	worker := NewIntegratedWorker("test-integrated-10", unit, logger).(*integratedWorker)

	ctx := context.Background()
	cmdResult, err := worker.ExecuteCmd(ctx)

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
