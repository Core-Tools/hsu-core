package managedprocess

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockLogger for testing
type MockUnmanagedLogger struct {
	mock.Mock
}

func (m *MockUnmanagedLogger) LogLevelf(level int, format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockUnmanagedLogger) Debugf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockUnmanagedLogger) Infof(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockUnmanagedLogger) Warnf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockUnmanagedLogger) Errorf(format string, args ...interface{}) {
	m.Called(format, args)
}

func createTestUnmanagedProcessConfig() *UnmanagedProcessConfig {
	// Use OS-dependent path for PID file
	var pidFile string
	if runtime.GOOS == "windows" {
		pidFile = "C:\\Temp\\test-process.pid"
	} else {
		pidFile = "/tmp/test-process.pid"
	}

	return &UnmanagedProcessConfig{
		Metadata: ProcessMetadata{
			Name:        "test-unmanaged-process",
			Description: "Test unmanaged process",
		},
		Discovery: process.DiscoveryConfig{
			Method:        process.DiscoveryMethodPIDFile,
			PIDFile:       pidFile,
			CheckInterval: 15 * time.Second,
		},
		Control: processcontrol.SystemProcessControlConfig{
			CanTerminate:    true,
			CanRestart:      false,
			ServiceManager:  "systemd",
			ServiceName:     "test-service",
			AllowedSignals:  []os.Signal{os.Interrupt, os.Kill},
			GracefulTimeout: 10 * time.Second,
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

func createTestUnmanagedProcessConfigWithPortDiscovery() *UnmanagedProcessConfig {
	return &UnmanagedProcessConfig{
		Metadata: ProcessMetadata{
			Name:        "test-unmanaged-port-process",
			Description: "Test unmanaged process with port discovery",
		},
		Discovery: process.DiscoveryConfig{
			Method:        process.DiscoveryMethodPort,
			Port:          8080,
			Protocol:      "tcp",
			CheckInterval: 20 * time.Second,
		},
		Control: processcontrol.SystemProcessControlConfig{
			CanTerminate:    false,
			CanRestart:      false,
			ServiceManager:  "",
			ServiceName:     "",
			AllowedSignals:  []os.Signal{},
			GracefulTimeout: 5 * time.Second,
		},
		HealthCheck: monitoring.HealthCheckConfig{
			Type: monitoring.HealthCheckTypeGRPC,
			GRPC: monitoring.GRPCHealthCheckConfig{
				Address: "localhost:50051",
				Service: "CoreService",
				Method:  "Ping",
			},
			RunOptions: monitoring.HealthCheckRunOptions{
				Enabled:      true,
				Interval:     15 * time.Second,
				Timeout:      3 * time.Second,
				InitialDelay: 5 * time.Second,
				Retries:      2,
			},
		},
	}
}

func TestNewUnmanagedProcess(t *testing.T) {
	logger := &MockUnmanagedLogger{}
	processConfig := createTestUnmanagedProcessConfig()

	process := NewUnmanagedProcessOptions("test-unmanaged-1", processConfig, logger)

	assert.NotNil(t, process)
	assert.Equal(t, "test-unmanaged-1", process.ID())
}

func TestUnmanagedProcess_ID(t *testing.T) {
	logger := &MockUnmanagedLogger{}
	processConfig := createTestUnmanagedProcessConfig()

	process := NewUnmanagedProcessOptions("test-unmanaged-2", processConfig, logger)

	assert.Equal(t, "test-unmanaged-2", process.ID())
}

func TestUnmanagedProcess_Metadata(t *testing.T) {
	logger := &MockUnmanagedLogger{}
	processConfig := createTestUnmanagedProcessConfig()

	process := NewUnmanagedProcessOptions("test-unmanaged-3", processConfig, logger)

	metadata := process.Metadata()
	assert.Equal(t, "test-unmanaged-process", metadata.Name)
	assert.Equal(t, "Test unmanaged process", metadata.Description)
}

func TestUnmanagedProcess_ProcessControlOptions_PIDFile(t *testing.T) {
	logger := &MockUnmanagedLogger{}
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	processConfig := createTestUnmanagedProcessConfig()

	process := NewUnmanagedProcessOptions("test-unmanaged-4", processConfig, logger)

	options := process.ProcessControlOptions()

	// Test basic capabilities
	assert.True(t, options.CanAttach, "UnmanagedProcess must support attachment")
	assert.True(t, options.CanTerminate, "UnmanagedProcess should support termination based on config")
	assert.False(t, options.CanRestart, "UnmanagedProcess should not support restart based on config")

	// Test ExecuteCmd is not present
	assert.Nil(t, options.ExecuteCmd, "UnmanagedProcess should not provide ExecuteCmd")

	// Test restart configuration is not present
	assert.Nil(t, options.ContextAwareRestart, "UnmanagedProcess should not provide restart configuration")

	// Test resource limits are not present
	assert.Nil(t, options.Limits, "UnmanagedProcess should not provide resource limits")

	// Test graceful timeout from system config
	assert.Equal(t, 10*time.Second, options.GracefulTimeout)

	// Test health check is provided by AttachCmd
	assert.Nil(t, options.HealthCheck, "UnmanagedProcess should provide health check via AttachCmd")
	assert.NotNil(t, options.AttachCmd, "UnmanagedProcess should provide AttachCmd")

	// Test that AttachCmd would return the correct health check configuration
	// Note: This is a test, so we can't actually test attachment without a real process
	// In a real scenario, AttachCmd would be called by ProcessControl

	// Test allowed signals
	require.NotNil(t, options.AllowedSignals)
	assert.Len(t, options.AllowedSignals, 2)
	assert.Contains(t, options.AllowedSignals, os.Interrupt)
	assert.Contains(t, options.AllowedSignals, os.Kill)
}

func TestUnmanagedProcess_ProcessControlOptions_Port(t *testing.T) {
	logger := &MockUnmanagedLogger{}
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	processConfig := createTestUnmanagedProcessConfigWithPortDiscovery()

	process := NewUnmanagedProcessOptions("test-unmanaged-5", processConfig, logger)

	options := process.ProcessControlOptions()

	// Test basic capabilities
	assert.True(t, options.CanAttach, "UnmanagedProcess must support attachment")
	assert.False(t, options.CanTerminate, "UnmanagedProcess should not support termination based on config")
	assert.False(t, options.CanRestart, "UnmanagedProcess should not support restart based on config")

	// Test ExecuteCmd is not present
	assert.Nil(t, options.ExecuteCmd, "UnmanagedProcess should not provide ExecuteCmd")

	// Test graceful timeout from system config
	assert.Equal(t, 5*time.Second, options.GracefulTimeout)

	// Test health check is provided by AttachCmd
	assert.Nil(t, options.HealthCheck, "UnmanagedProcess should provide health check via AttachCmd")
	assert.NotNil(t, options.AttachCmd, "UnmanagedProcess should provide AttachCmd")

	// Test that AttachCmd would return the correct health check configuration
	// Note: This is a test, so we can't actually test attachment without a real process
	// In a real scenario, AttachCmd would be called by ProcessControl

	// Test allowed signals (empty in this case)
	assert.Len(t, options.AllowedSignals, 0)
}

func TestUnmanagedProcess_IntegrationWithProcessControlOptions(t *testing.T) {
	logger := &MockUnmanagedLogger{}
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	processConfig := createTestUnmanagedProcessConfig()

	process := NewUnmanagedProcessOptions("test-unmanaged-6", processConfig, logger)

	options := process.ProcessControlOptions()

	// Test that options pass validation
	err := ValidateProcessControlOptions(options)
	assert.NoError(t, err, "UnmanagedProcess options should pass validation")
}

func TestUnmanagedProcess_IntegrationWithProcessControlOptions_Port(t *testing.T) {
	logger := &MockUnmanagedLogger{}
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	processConfig := createTestUnmanagedProcessConfigWithPortDiscovery()

	process := NewUnmanagedProcessOptions("test-unmanaged-7", processConfig, logger)

	options := process.ProcessControlOptions()

	// Test that options pass validation
	err := ValidateProcessControlOptions(options)
	assert.NoError(t, err, "UnmanagedProcess options with port discovery should pass validation")
}

func TestUnmanagedProcess_MultipleInstances(t *testing.T) {
	logger1 := &MockUnmanagedLogger{}
	logger1.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger2 := &MockUnmanagedLogger{}
	logger2.On("Debugf", mock.Anything, mock.Anything).Maybe()

	processConfig1 := createTestUnmanagedProcessConfig()
	processConfig2 := createTestUnmanagedProcessConfigWithPortDiscovery()

	process1 := NewUnmanagedProcessOptions("test-unmanaged-7", processConfig1, logger1)
	process2 := NewUnmanagedProcessOptions("test-unmanaged-8", processConfig2, logger2)

	// Test independence
	assert.NotEqual(t, process1.ID(), process2.ID())
	assert.NotEqual(t, process1.Metadata(), process2.Metadata())
}

func TestUnmanagedProcess_DifferentCapabilities(t *testing.T) {
	logger := &MockUnmanagedLogger{}
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()

	// Create process configs with different capabilities
	processConfig1 := createTestUnmanagedProcessConfig()
	processConfig1.Control.CanTerminate = true
	processConfig1.Control.CanRestart = true

	processConfig2 := createTestUnmanagedProcessConfig()
	processConfig2.Control.CanTerminate = false
	processConfig2.Control.CanRestart = false

	process1 := NewUnmanagedProcessOptions("process-1", processConfig1, logger)
	process2 := NewUnmanagedProcessOptions("process-2", processConfig2, logger)

	options1 := process1.ProcessControlOptions()
	options2 := process2.ProcessControlOptions()

	// Test different capabilities based on system config
	assert.True(t, options1.CanTerminate)
	assert.True(t, options1.CanRestart)

	assert.False(t, options2.CanTerminate)
	assert.False(t, options2.CanRestart)
}
