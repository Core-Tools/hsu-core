package resourcelimits

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockLogger for testing
type MockResourceLimitLogger struct {
	mock.Mock
}

func (m *MockResourceLimitLogger) LogLevelf(level int, format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockResourceLimitLogger) Debugf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockResourceLimitLogger) Infof(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockResourceLimitLogger) Warnf(format string, args ...interface{}) {
	m.Called(format, args)
}

func (m *MockResourceLimitLogger) Errorf(format string, args ...interface{}) {
	m.Called(format, args)
}

func TestResourceLimitManagerViolationDispatch(t *testing.T) {
	logger := &MockResourceLimitLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Warnf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	// Test External mode with callback
	// Create limits with external violation handling
	limits := &ResourceLimits{
		Memory: &MemoryLimits{
			MaxRSS: 100 * 1024 * 1024, // 100MB
			Policy: ResourcePolicyRestart,
		},
		Monitoring: &ResourceMonitoringConfig{
			Enabled:  true,
			Interval: time.Second,
		},
	}

	manager := NewResourceLimitManager(12345, limits, logger)

	// Set up callback
	var callbackInvoked bool
	var receivedPolicy ResourcePolicy
	var receivedViolation *ResourceViolation

	var wg sync.WaitGroup

	manager.SetViolationCallback(func(policy ResourcePolicy, violation *ResourceViolation) {
		defer wg.Done()

		callbackInvoked = true
		receivedPolicy = policy
		receivedViolation = violation
	})

	// Test that callback gets called
	testViolation := &ResourceViolation{
		LimitType:    ResourceLimitTypeMemory,
		CurrentValue: 200 * 1024 * 1024,
		LimitValue:   100 * 1024 * 1024,
		Severity:     ViolationSeverityCritical,
		Timestamp:    time.Now(),
		Message:      "Test violation",
	}

	// Simulate violation dispatch
	wg.Add(1)
	manager.(*resourceLimitManager).dispatchViolation(testViolation)

	// Wait a bit for async processing
	wg.Wait()

	require.True(t, callbackInvoked, "External callback was not invoked")
	assert.Equal(t, ResourcePolicyRestart, receivedPolicy)
	require.NotNil(t, receivedViolation)
	assert.Equal(t, "Test violation", receivedViolation.Message)
}

func TestProcessControlIntegrationPattern(t *testing.T) {
	logger := &MockResourceLimitLogger{}
	logger.On("Infof", mock.Anything, mock.Anything).Maybe()
	logger.On("Debugf", mock.Anything, mock.Anything).Maybe()
	logger.On("Warnf", mock.Anything, mock.Anything).Maybe()
	logger.On("Errorf", mock.Anything, mock.Anything).Maybe()

	// Simulate ProcessControl configuration
	limits := &ResourceLimits{
		Memory: &MemoryLimits{
			MaxRSS: 256 * 1024 * 1024, // 256MB
			Policy: ResourcePolicyRestart,
		},
		CPU: &CPULimits{
			MaxPercent: 75.0,
			Policy:     ResourcePolicyGracefulShutdown,
		},
		Process: &ProcessLimits{
			MaxFileDescriptors: 1024,
			Policy:             ResourcePolicyAlert,
		},
	}

	manager := NewResourceLimitManager(12345, limits, logger)

	// Simulate ProcessControl violation handling
	var receivedPolicies []ResourcePolicy
	var receivedViolations []string

	var wg sync.WaitGroup

	processControlViolationHandler := func(policy ResourcePolicy, violation *ResourceViolation) {
		defer wg.Done()

		receivedPolicies = append(receivedPolicies, policy)
		receivedViolations = append(receivedViolations, violation.Message)

		// Simulate ProcessControl policy handling
		switch violation.LimitType {
		case ResourceLimitTypeMemory:
			if limits.Memory.Policy == ResourcePolicyRestart {
				logger.Infof("ProcessControl: Triggering restart for memory violation")
				// Would call pc.restartInternal(ctx) here
			}
		case ResourceLimitTypeCPU:
			if limits.CPU.Policy == ResourcePolicyGracefulShutdown {
				logger.Infof("ProcessControl: Triggering graceful shutdown for CPU violation")
				// Would call pc.stopInternal(ctx, false) here
			}
		case ResourceLimitTypeProcess:
			if limits.Process.Policy == ResourcePolicyAlert {
				logger.Infof("ProcessControl: Alerting for process violation")
				// Would log alert here
			}
		}
	}

	manager.SetViolationCallback(processControlViolationHandler)

	// Test different violation types
	testViolations := []*ResourceViolation{
		{
			LimitType: ResourceLimitTypeMemory,
			Severity:  ViolationSeverityCritical,
			Message:   "Memory limit exceeded",
		},
		{
			LimitType: ResourceLimitTypeCPU,
			Severity:  ViolationSeverityCritical,
			Message:   "CPU limit exceeded",
		},
		{
			LimitType: ResourceLimitTypeProcess,
			Severity:  ViolationSeverityWarning,
			Message:   "File descriptor limit exceeded",
		},
	}

	// Dispatch violations
	wg.Add(2) // callbacks will be called only for critical violations
	for _, violation := range testViolations {
		manager.(*resourceLimitManager).dispatchViolation(violation)
	}

	// Wait for processing
	wg.Wait()

	require.Equal(t, 2, len(receivedPolicies), "Expected 2 policies handled, got %d", len(receivedPolicies))
	assert.Contains(t, receivedPolicies, ResourcePolicyRestart)
	assert.Contains(t, receivedPolicies, ResourcePolicyGracefulShutdown)

	require.Equal(t, 2, len(receivedViolations), "Expected 2 violations handled, got %d", len(receivedViolations))
	assert.Contains(t, receivedViolations, "Memory limit exceeded")
	assert.Contains(t, receivedViolations, "CPU limit exceeded")
}
