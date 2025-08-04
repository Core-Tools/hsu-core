package monitoring

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/processstate"
)

type HealthCheckType string

const (
	HealthCheckTypeHTTP    HealthCheckType = "http"
	HealthCheckTypeGRPC    HealthCheckType = "grpc"
	HealthCheckTypeTCP     HealthCheckType = "tcp"
	HealthCheckTypeExec    HealthCheckType = "exec"
	HealthCheckTypeProcess HealthCheckType = "process"
)

type HTTPHealthCheckConfig struct {
	URL     string            `yaml:"url"`
	PMethod string            `yaml:"method,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

type GRPCHealthCheckConfig struct {
	Address string            `yaml:"address"`
	Service string            `yaml:"service,omitempty"`
	Method  string            `yaml:"method,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

type TCPHealthCheckConfig struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

type ExecHealthCheckConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args,omitempty"`
}

type HealthCheckConfig struct {
	Type HealthCheckType `yaml:"type"`

	// HTTP health check
	HTTP HTTPHealthCheckConfig `yaml:"http,omitempty"`

	// GRPC health check
	GRPC GRPCHealthCheckConfig `yaml:"grpc,omitempty"`

	// TCP health check
	TCP TCPHealthCheckConfig `yaml:"tcp,omitempty"`

	// Exec health check
	Exec ExecHealthCheckConfig `yaml:"exec,omitempty"`

	// Run options
	RunOptions HealthCheckRunOptions `yaml:"run_options,omitempty"`
}

type HealthCheckRunOptions struct {
	Enabled      bool          `yaml:"enabled,omitempty"`
	Interval     time.Duration `yaml:"interval,omitempty"`
	Timeout      time.Duration `yaml:"timeout,omitempty"`
	InitialDelay time.Duration `yaml:"initial_delay,omitempty"`
	Retries      int           `yaml:"retries,omitempty"`
}

type HealthCheckStatus string

const (
	HealthCheckStatusUnknown   HealthCheckStatus = "unknown"
	HealthCheckStatusHealthy   HealthCheckStatus = "healthy"
	HealthCheckStatusDegraded  HealthCheckStatus = "degraded"
	HealthCheckStatusUnhealthy HealthCheckStatus = "unhealthy"
)

type HealthCheckState struct {
	Status               HealthCheckStatus
	LastCheck            time.Time
	Message              string
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
}

// HealthRestartCallback defines a callback function for triggering restarts on health failures
type HealthRestartCallback func(reason string) error

// HealthRecoveryCallback defines a callback function for when health recovers
type HealthRecoveryCallback func()

type HealthMonitor interface {
	Start(ctx context.Context) error
	Stop()
	State() *HealthCheckState
	SetRestartCallback(callback HealthRestartCallback)   // Add restart callback
	SetRecoveryCallback(callback HealthRecoveryCallback) // Add recovery callback
}

type healthMonitor struct {
	config           *HealthCheckConfig
	state            *HealthCheckState
	stopChan         chan struct{}
	wg               sync.WaitGroup
	mutex            sync.Mutex
	logger           logging.Logger
	id               string
	processInfo      *ProcessInfo           // Add process information for health checking
	restartCallback  HealthRestartCallback  // Callback for triggering restarts
	recoveryCallback HealthRecoveryCallback // Callback for health recovery
}

// ProcessInfo holds process information needed for health monitoring
type ProcessInfo struct {
	PID int // Only PID is needed for process health checking
}

// NewHealthMonitor creates a health monitor for comprehensive application health checking.
// Supports HTTP, gRPC, TCP, exec, and process health check types with configurable intervals,
// retries, timeouts, and failure thresholds. Integrates with restart circuit breakers and
// recovery callbacks for automated health management.
func NewHealthMonitor(config *HealthCheckConfig, id string, logger logging.Logger) HealthMonitor {
	return &healthMonitor{
		config:   config,
		state:    &HealthCheckState{Status: HealthCheckStatusUnknown},
		stopChan: make(chan struct{}),
		logger:   logger,
		id:       id,
	}
}

// NewHealthMonitorWithProcessInfo creates a health monitor with process information for process health checks
func NewHealthMonitorWithProcessInfo(config *HealthCheckConfig, id string, processInfo *ProcessInfo, logger logging.Logger) HealthMonitor {
	return &healthMonitor{
		config:      config,
		state:       &HealthCheckState{Status: HealthCheckStatusUnknown},
		stopChan:    make(chan struct{}),
		logger:      logger,
		id:          id,
		processInfo: processInfo,
	}
}

func (h *healthMonitor) Start(ctx context.Context) error {
	h.logger.Infof("Starting health monitor, id: %s, type: %s, interval: %v", h.id, h.config.Type, h.config.RunOptions.Interval)

	// Validate health check configuration
	if err := ValidateHealthCheckConfig(*h.config); err != nil {
		h.logger.Errorf("Health check configuration validation failed, id: %s, error: %v", h.id, err)
		return errors.NewValidationError("invalid health check configuration", err).WithContext("id", h.id)
	}

	h.wg.Add(1)
	go h.loop()
	return nil
}

func (h *healthMonitor) Stop() {
	h.logger.Infof("Stopping health monitor, id: %s", h.id)
	close(h.stopChan)
	h.wg.Wait()
	h.logger.Infof("Health monitor stopped, id: %s", h.id)
}

func (h *healthMonitor) State() *HealthCheckState {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	// Return a copy to avoid race conditions
	stateCopy := *h.state
	return &stateCopy
}

func (h *healthMonitor) loop() {
	defer h.wg.Done()

	if h.config.Type == "" {
		h.logger.Debugf("Health monitor loop is disabled due to empty type, id: %s", h.id)
		return
	}

	h.logger.Debugf("Health monitor loop started, id: %s", h.id)

	// Initial delay before first check
	if h.config.RunOptions.InitialDelay > 0 {
		h.logger.Debugf("Health monitor initial delay, id: %s, delay: %v", h.id, h.config.RunOptions.InitialDelay)
		select {
		case <-time.After(h.config.RunOptions.InitialDelay):
		case <-h.stopChan:
			h.logger.Debugf("Health monitor stopped during initial delay, id: %s", h.id)
			return
		}
	}

	ticker := time.NewTicker(h.config.RunOptions.Interval)
	defer ticker.Stop()

	// Perform initial check
	h.performCheck()

	for {
		select {
		case <-ticker.C:
			h.performCheck()
		case <-h.stopChan:
			h.logger.Debugf("Health monitor loop stopping, id: %s", h.id)
			return
		}
	}
}

func (h *healthMonitor) performCheck() {
	h.logger.Debugf("Performing health check, id: %s, type: %s", h.id, h.config.Type)

	h.mutex.Lock()
	h.state.LastCheck = time.Now()
	h.mutex.Unlock()

	var isHealthy bool
	var message string

	switch h.config.Type {
	case HealthCheckTypeHTTP:
		isHealthy, message = h.checkHTTP()
	case HealthCheckTypeGRPC:
		isHealthy, message = h.checkGRPC()
	case HealthCheckTypeTCP:
		isHealthy, message = h.checkTCP()
	case HealthCheckTypeExec:
		isHealthy, message = h.checkExec()
	case HealthCheckTypeProcess:
		isHealthy, message = h.checkProcess()
	default:
		isHealthy = false
		message = "Unknown health check type: " + string(h.config.Type)
		h.logger.Errorf("Unknown health check type, id: %s, type: %s", h.id, h.config.Type)
	}

	h.updateState(isHealthy, message)
}

func (h *healthMonitor) updateState(isHealthy bool, message string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	previousStatus := h.state.Status

	if isHealthy {
		h.state.ConsecutiveSuccesses++
		h.state.ConsecutiveFailures = 0
		// ✅ REMOVED: h.state.Retries = 0

		previousWasUnhealthy := previousStatus == HealthCheckStatusDegraded || previousStatus == HealthCheckStatusUnhealthy

		if h.state.Status != HealthCheckStatusHealthy {
			h.state.Status = HealthCheckStatusHealthy
			h.logger.Infof("Health check recovered, id: %s, previous: %s, consecutive_successes: %d, message: %s",
				h.id, previousStatus, h.state.ConsecutiveSuccesses, message)

			// Call recovery callback if process recovered from unhealthy state
			if previousWasUnhealthy && h.recoveryCallback != nil {
				h.logger.Infof("Triggering recovery callback, id: %s, recovered from: %s, message: %s",
					h.id, previousStatus, message)
				go h.recoveryCallback() // Call in goroutine to avoid blocking health check
			}
		} else {
			h.logger.Debugf("Health check passed, id: %s, consecutive_successes: %d, message: %s",
				h.id, h.state.ConsecutiveSuccesses, message)
		}
	} else {
		h.state.ConsecutiveFailures++
		h.state.ConsecutiveSuccesses = 0

		// Determine new status based on failure count
		var newStatus HealthCheckStatus
		if h.state.ConsecutiveFailures == 1 {
			newStatus = HealthCheckStatusDegraded
		} else {
			newStatus = HealthCheckStatusUnhealthy
		}

		if h.state.Status != newStatus {
			h.state.Status = newStatus
			h.logger.Warnf("Health check status changed, id: %s, status: %s->%s, consecutive_failures: %d, message: %s",
				h.id, previousStatus, newStatus, h.state.ConsecutiveFailures, message)
		} else {
			h.logger.Warnf("Health check failed, id: %s, status: %s, consecutive_failures: %d, message: %s",
				h.id, h.state.Status, h.state.ConsecutiveFailures, message)
		}

		// ✅ SIMPLIFIED: Check restart condition without retry counting
		h.checkRestartCondition(message)
	}

	h.state.Message = message
}

// checkRestartCondition now just calls callback, policy evaluation moved to processcontrolimpl
func (h *healthMonitor) checkRestartCondition(message string) {
	// Only trigger restart if we have restart callback
	if h.restartCallback == nil {
		return
	}

	h.logger.Warnf("Triggering restart due to health check failure, id: %s, reason: %s",
		h.id, message)

	// Always call callback, let processcontrolimpl handle policy evaluation
	go func() {
		if err := h.restartCallback(fmt.Sprintf("Health check failure: %s", message)); err != nil {
			h.logger.Errorf("Restart callback failed, id: %s, error: %v", h.id, err)
		}
	}()
}

func (h *healthMonitor) checkHTTP() (bool, string) {
	h.logger.Debugf("Performing HTTP health check, id: %s, url: %s", h.id, h.config.HTTP.URL)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: h.config.RunOptions.Timeout,
	}

	method := h.config.HTTP.PMethod
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequest(method, h.config.HTTP.URL, nil)
	if err != nil {
		return false, fmt.Sprintf("failed to create HTTP request: %v", err)
	}

	// Add custom headers
	for key, value := range h.config.HTTP.Headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	// Consider 2xx status codes as healthy
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, fmt.Sprintf("HTTP health check passed: %d %s", resp.StatusCode, resp.Status)
	}

	return false, fmt.Sprintf("HTTP health check failed: %d %s", resp.StatusCode, resp.Status)
}

func (h *healthMonitor) checkGRPC() (bool, string) {
	h.logger.Debugf("Performing gRPC health check, id: %s, address: %s, service: %s",
		h.id, h.config.GRPC.Address, h.config.GRPC.Service)

	// For now, implement as TCP connection check
	// Note: gRPC health check implementation planned for Phase 4
	// Implementation strategy: Use official gRPC health checking protocol (grpc.health.v1.Health/Check)
	address := h.config.GRPC.Address

	// Parse address to get host and port
	if !strings.Contains(address, ":") {
		return false, "Invalid gRPC address format (missing port)"
	}

	conn, err := net.DialTimeout("tcp", address, h.config.RunOptions.Timeout)
	if err != nil {
		return false, fmt.Sprintf("gRPC connection failed: %v", err)
	}
	defer conn.Close()

	return true, fmt.Sprintf("gRPC connection successful to %s", address)
}

func (h *healthMonitor) checkTCP() (bool, string) {
	h.logger.Debugf("Performing TCP health check, id: %s, address: %s, port: %d",
		h.id, h.config.TCP.Address, h.config.TCP.Port)

	address := fmt.Sprintf("%s:%d", h.config.TCP.Address, h.config.TCP.Port)

	conn, err := net.DialTimeout("tcp", address, h.config.RunOptions.Timeout)
	if err != nil {
		return false, fmt.Sprintf("TCP connection failed: %v", err)
	}
	defer conn.Close()

	return true, fmt.Sprintf("TCP connection successful to %s", address)
}

func (h *healthMonitor) checkExec() (bool, string) {
	h.logger.Debugf("Performing exec health check, id: %s, command: %s, args: %v",
		h.id, h.config.Exec.Command, h.config.Exec.Args)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), h.config.RunOptions.Timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, h.config.Exec.Command, h.config.Exec.Args...)

	// Execute command
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return false, fmt.Sprintf("exec health check timed out after %v", h.config.RunOptions.Timeout)
	}

	if err != nil {
		return false, fmt.Sprintf("exec health check failed: %v, output: %s", err, string(output))
	}

	return true, fmt.Sprintf("exec health check passed, output: %s", string(output))
}

func (h *healthMonitor) checkProcess() (bool, string) {
	h.logger.Debugf("Performing process health check, id: %s", h.id)

	// If we have process info, use it for more accurate checking
	if h.processInfo != nil {
		return h.checkProcessWithInfo()
	}

	// Fallback: basic existence check using discovery
	return h.checkProcessBasic()
}

func (h *healthMonitor) checkProcessWithInfo() (bool, string) {
	pid := h.processInfo.PID

	// Check if process running
	running, err := processstate.IsProcessRunning(pid)
	if !running {
		return false, fmt.Sprintf("process not running: PID %d, err: %v", pid, err)
	}

	return true, fmt.Sprintf("process is running: PID %d, err: %v", pid, err)
}

func (h *healthMonitor) checkProcessBasic() (bool, string) {
	h.logger.Debugf("Basic process health check, id: %s (no process info available)", h.id)

	// Without process info, we can't do much more than assume healthy
	// This should ideally not happen in production
	h.logger.Warnf("Process health check has no process information, id: %s", h.id)
	return true, "process health check: no process information available (assuming healthy)"
}

// SetProcessInfo allows updating process information after health monitor creation
func (h *healthMonitor) SetProcessInfo(processInfo *ProcessInfo) {
	h.processInfo = processInfo
	h.logger.Debugf("Process info updated for health monitor, id: %s, PID: %d", h.id, processInfo.PID)
}

func (h *healthMonitor) SetRestartCallback(callback HealthRestartCallback) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.restartCallback = callback
	h.logger.Debugf("Restart callback set for health monitor, id: %s", h.id)
}

func (h *healthMonitor) SetRecoveryCallback(callback HealthRecoveryCallback) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.recoveryCallback = callback
	h.logger.Debugf("Recovery callback set for health monitor, id: %s", h.id)
}
