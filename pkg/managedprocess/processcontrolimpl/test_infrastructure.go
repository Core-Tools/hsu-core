//go:build test

package processcontrolimpl

import (
	"context"
	"io"
	"sync"

	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/resourcelimits"

	"github.com/stretchr/testify/mock"
)

// ===== SHARED TEST INFRASTRUCTURE =====

// SimpleLogger implements a basic logger for testing
type SimpleLogger struct{}

func (l *SimpleLogger) Debugf(format string, args ...interface{})               {}
func (l *SimpleLogger) Infof(format string, args ...interface{})                {}
func (l *SimpleLogger) Warnf(format string, args ...interface{})                {}
func (l *SimpleLogger) Errorf(format string, args ...interface{})               {}
func (l *SimpleLogger) LogLevelf(level int, format string, args ...interface{}) {}

// MockReadCloser for stdout simulation
type MockReadCloser struct{}

func (m *MockReadCloser) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

func (m *MockReadCloser) Close() error {
	return nil
}

// MockLogger implements a logger for testing with full mock capabilities
type MockLogger struct {
	mock.Mock
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

func (m *MockLogger) LogLevelf(level int, format string, args ...interface{}) {
	m.Called(level, format, args)
}

// MockHealthMonitor implements monitoring.HealthMonitor for testing
type MockHealthMonitor struct {
	mock.Mock
	stopChan chan struct{}
	wg       sync.WaitGroup
}

func (m *MockHealthMonitor) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockHealthMonitor) Stop() {
	m.Called()
}

func (m *MockHealthMonitor) State() *monitoring.HealthCheckState {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*monitoring.HealthCheckState)
}

func (m *MockHealthMonitor) SetRestartCallback(callback monitoring.HealthRestartCallback) {
	m.Called(callback)
}

func (m *MockHealthMonitor) SetRecoveryCallback(callback monitoring.HealthRecoveryCallback) {
	m.Called(callback)
}

// MockResourceLimitManager implements resourcelimits.ResourceLimitManager for testing
type MockResourceLimitManager struct {
	mock.Mock
}

func (m *MockResourceLimitManager) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockResourceLimitManager) Stop() {
	m.Called()
}

func (m *MockResourceLimitManager) GetViolations() []*resourcelimits.ResourceViolation {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]*resourcelimits.ResourceViolation)
}

func (m *MockResourceLimitManager) SetViolationCallback(callback resourcelimits.ResourceViolationCallback) {
	m.Called(callback)
}

// MockRestartCircuitBreaker for restart logic testing
type MockRestartCircuitBreaker struct {
	mock.Mock
}

func (m *MockRestartCircuitBreaker) ExecuteRestart(restartFunc RestartFunc, context processcontrol.RestartContext) error {
	args := m.Called(restartFunc, context)
	return args.Error(0)
}

func (m *MockRestartCircuitBreaker) Reset() {
	m.Called()
}

func (m *MockRestartCircuitBreaker) GetState() CircuitBreakerState {
	args := m.Called()
	return args.Get(0).(CircuitBreakerState)
}
