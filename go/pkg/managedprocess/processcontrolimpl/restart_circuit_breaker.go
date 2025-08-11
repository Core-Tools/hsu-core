package processcontrolimpl

import (
	"sync"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
)

type RestartFunc func() error

// CircuitBreakerState provides insight into circuit breaker status
type CircuitBreakerState struct {
	IsOpen          bool                              `json:"is_open"`
	RestartAttempts int                               `json:"restart_attempts"`
	LastRestartTime time.Time                         `json:"last_restart_time"`
	CreationTime    time.Time                         `json:"creation_time"`
	LastTriggerType processcontrol.RestartTriggerType `json:"last_trigger_type,omitempty"`
	LastContext     *processcontrol.RestartContext    `json:"last_context,omitempty"`
}

// RestartCircuitBreaker interface with context awareness
type RestartCircuitBreaker interface {
	GetState() CircuitBreakerState
	ExecuteRestart(restartFunc RestartFunc, context processcontrol.RestartContext) error
	Reset()
}

// Default multipliers for severity and worker profile types
var (
	DefaultSeverityMultipliers = map[string]float64{
		"warning":   0.5, // Half normal limits for warnings
		"critical":  1.0, // Normal limits for critical
		"emergency": 2.0, // Double limits for emergencies
	}

	// Default multipliers for worker profile types
	DefaultWorkerProfileMultipliers = map[string]float64{
		"batch":     3.0, // Very lenient for batch processors
		"web":       1.0, // Standard for web services
		"database":  5.0, // Extremely lenient for databases
		"worker":    2.0, // Lenient for background workers
		"scheduler": 2.5, // Moderately lenient for schedulers
		"default":   1.0, // Standard for unknown types
	}
)

// Single constructor function for context-aware circuit breaker
func NewRestartCircuitBreaker(config *processcontrol.ContextAwareRestartConfig, id string, workerProfileType string, logger logging.Logger) RestartCircuitBreaker {
	severityMultipliers := config.SeverityMultipliers
	if severityMultipliers == nil {
		severityMultipliers = DefaultSeverityMultipliers
	}

	workerProfileMultipliers := config.WorkerProfileMultipliers
	if workerProfileMultipliers == nil {
		workerProfileMultipliers = DefaultWorkerProfileMultipliers
	}

	return &enhancedRestartCircuitBreaker{
		basicConfig:              &config.Default,
		enhancedConfig:           config,
		id:                       id,
		workerProfileType:        workerProfileType,
		logger:                   logger,
		restartAttempts:          0,
		lastRestartTime:          time.Now(),
		creationTime:             time.Now(),
		circuitBreakerOpen:       false,
		severityMultipliers:      severityMultipliers,
		workerProfileMultipliers: workerProfileMultipliers,
	}
}

type enhancedRestartCircuitBreaker struct {
	// Configuration (using local RestartConfig without Policy)
	basicConfig       *processcontrol.RestartConfig
	enhancedConfig    *processcontrol.ContextAwareRestartConfig
	id                string
	workerProfileType string
	logger            logging.Logger

	// Multipliers
	severityMultipliers      map[string]float64
	workerProfileMultipliers map[string]float64

	// State
	restartAttempts    int                               // Track attempts across all restart sources
	lastRestartTime    time.Time                         // Track timing for retry delay and backoff
	creationTime       time.Time                         // Track startup grace period
	circuitBreakerOpen bool                              // Circuit breaker to stop excessive restarts
	lastTriggerType    processcontrol.RestartTriggerType // Last trigger type for diagnostics
	lastContext        *processcontrol.RestartContext    // Last context for diagnostics
	mutex              sync.Mutex                        // Protect restart state
}

// ExecuteRestart now takes context parameter (renamed from ExecuteRestartWithContext)
func (rcb *enhancedRestartCircuitBreaker) ExecuteRestart(restartFunc RestartFunc, context processcontrol.RestartContext) error {
	rcb.mutex.Lock()
	defer rcb.mutex.Unlock()

	// Store context for diagnostics
	rcb.lastContext = &context
	rcb.lastTriggerType = context.TriggerType

	rcb.logger.Debugf("Restart request, id: %s, trigger: %s, severity: %s, worker_profile: %s, message: %s",
		rcb.id, context.TriggerType, context.Severity, context.WorkerProfileType, context.Message)

	// Check circuit breaker
	if rcb.circuitBreakerOpen {
		rcb.logger.Errorf("Circuit breaker is open, ignoring restart request, id: %s, attempts: %d, trigger: %s",
			rcb.id, rcb.restartAttempts, context.TriggerType)
		return errors.NewInternalError("restart circuit breaker is open", nil).WithContext("id", rcb.id).WithContext("trigger", string(context.TriggerType))
	}

	// Check startup grace period
	if rcb.enhancedConfig != nil && rcb.enhancedConfig.StartupGracePeriod > 0 {
		if time.Since(rcb.creationTime) < rcb.enhancedConfig.StartupGracePeriod {
			rcb.logger.Infof("Restart blocked: within startup grace period, id: %s, trigger: %s, remaining: %v",
				rcb.id, context.TriggerType, rcb.enhancedConfig.StartupGracePeriod-time.Since(rcb.creationTime))
			return errors.NewValidationError("restart blocked: within startup grace period", nil).WithContext("id", rcb.id)
		}
	}

	// Get context-appropriate configuration
	config := rcb.getConfigForContext(context)

	// Apply multipliers for effective limits
	effectiveMaxRetries := rcb.applyMultipliers(config.MaxRetries, context)
	effectiveRetryDelay := rcb.applyDurationMultipliers(config.RetryDelay, context)

	// Check effective max retries
	if effectiveMaxRetries > 0 && rcb.restartAttempts >= effectiveMaxRetries {
		rcb.logger.Errorf("Effective max restart retries exceeded, opening circuit breaker, id: %s, attempts: %d, effective_max: %d, trigger: %s",
			rcb.id, rcb.restartAttempts, effectiveMaxRetries, context.TriggerType)
		rcb.circuitBreakerOpen = true
		return errors.NewInternalError("max restart retries exceeded", nil).WithContext("id", rcb.id).WithContext("trigger", string(context.TriggerType))
	}

	// Calculate retry delay with exponential backoff
	now := time.Now()
	timeSinceLastRestart := now.Sub(rcb.lastRestartTime)

	retryDelay := effectiveRetryDelay
	if rcb.restartAttempts > 0 {
		// Apply exponential backoff
		backoffMultiplier := 1.0
		for i := 0; i < rcb.restartAttempts; i++ {
			backoffMultiplier *= config.BackoffRate
		}
		retryDelay = time.Duration(float64(retryDelay) * backoffMultiplier)
	}

	// Enforce retry delay
	if timeSinceLastRestart < retryDelay {
		waitTime := retryDelay - timeSinceLastRestart
		rcb.logger.Infof("Enforcing retry delay, id: %s, trigger: %s, attempt: %d, waiting: %v",
			rcb.id, context.TriggerType, rcb.restartAttempts+1, waitTime)

		// Release lock during sleep to prevent deadlock
		rcb.mutex.Unlock()
		time.Sleep(waitTime)
		rcb.mutex.Lock()

		// Re-check circuit breaker after sleep
		if rcb.circuitBreakerOpen {
			return errors.NewInternalError("restart circuit breaker opened during delay", nil).WithContext("id", rcb.id)
		}
	}

	// Increment attempt counter and update timestamp
	rcb.restartAttempts++
	rcb.lastRestartTime = time.Now()

	rcb.logger.Warnf("Proceeding with restart, id: %s, trigger: %s, attempt: %d/%d, delay: %v, message: %s",
		rcb.id, context.TriggerType, rcb.restartAttempts, effectiveMaxRetries, retryDelay, context.Message)

	// Release lock before calling restart to prevent deadlock
	rcb.mutex.Unlock()
	defer func() { rcb.mutex.Lock() }()

	if err := restartFunc(); err != nil {
		rcb.logger.Errorf("Failed to restart, id: %s, trigger: %s, error: %v", rcb.id, context.TriggerType, err)
		return err
	}

	rcb.logger.Infof("Restart completed, id: %s, trigger: %s, attempt: %d", rcb.id, context.TriggerType, rcb.restartAttempts)
	return nil
}

func (rcb *enhancedRestartCircuitBreaker) Reset() {
	rcb.mutex.Lock()
	defer rcb.mutex.Unlock()

	if rcb.restartAttempts > 0 || rcb.circuitBreakerOpen {
		rcb.logger.Infof("Resetting circuit breaker, id: %s, previous attempts: %d",
			rcb.id, rcb.restartAttempts)
		rcb.restartAttempts = 0
		rcb.circuitBreakerOpen = false
		rcb.lastRestartTime = time.Time{} // Reset timestamp
		rcb.lastTriggerType = ""
		rcb.lastContext = nil
	}
}

func (rcb *enhancedRestartCircuitBreaker) GetState() CircuitBreakerState {
	rcb.mutex.Lock()
	defer rcb.mutex.Unlock()
	return rcb.getStateUnderLock()
}

func (rcb *enhancedRestartCircuitBreaker) getStateUnderLock() CircuitBreakerState {
	return CircuitBreakerState{
		IsOpen:          rcb.circuitBreakerOpen,
		RestartAttempts: rcb.restartAttempts,
		LastRestartTime: rcb.lastRestartTime,
		CreationTime:    rcb.creationTime,
		LastTriggerType: rcb.lastTriggerType,
		LastContext:     rcb.lastContext,
	}
}

// getConfigForContext returns appropriate configuration based on restart context
func (rcb *enhancedRestartCircuitBreaker) getConfigForContext(context processcontrol.RestartContext) *processcontrol.RestartConfig {
	if rcb.enhancedConfig == nil {
		return rcb.basicConfig
	}

	switch context.TriggerType {
	case processcontrol.RestartTriggerHealthFailure:
		if rcb.enhancedConfig.HealthFailures != nil {
			return rcb.enhancedConfig.HealthFailures
		}
	case processcontrol.RestartTriggerResourceViolation:
		if rcb.enhancedConfig.ResourceViolations != nil {
			return rcb.enhancedConfig.ResourceViolations
		}
	}
	return &rcb.enhancedConfig.Default
}

// applyMultipliers applies severity and worker profile type multipliers to integer values
func (rcb *enhancedRestartCircuitBreaker) applyMultipliers(baseValue int, context processcontrol.RestartContext) int {
	if baseValue <= 0 {
		return baseValue
	}

	multiplier := 1.0

	// Apply severity multiplier
	if severityMult, exists := rcb.severityMultipliers[context.Severity]; exists {
		multiplier *= severityMult
	}

	// Apply worker profile type multiplier
	workerProfileType := context.WorkerProfileType
	if workerProfileType == "" {
		workerProfileType = "default"
	}
	if workerProfileMult, exists := rcb.workerProfileMultipliers[workerProfileType]; exists {
		multiplier *= workerProfileMult
	}

	result := int(float64(baseValue) * multiplier)
	if result < 1 {
		result = 1 // Minimum of 1
	}

	return result
}

// applyDurationMultipliers applies severity and worker profile type multipliers to duration values
func (rcb *enhancedRestartCircuitBreaker) applyDurationMultipliers(baseDuration time.Duration, context processcontrol.RestartContext) time.Duration {
	if baseDuration <= 0 {
		return baseDuration
	}

	multiplier := 1.0

	// Apply severity multiplier
	if severityMult, exists := rcb.severityMultipliers[context.Severity]; exists {
		multiplier *= severityMult
	}

	// Apply worker profile type multiplier
	workerProfileType := context.WorkerProfileType
	if workerProfileType == "" {
		workerProfileType = "default"
	}
	if workerProfileMult, exists := rcb.workerProfileMultipliers[workerProfileType]; exists {
		multiplier *= workerProfileMult
	}

	result := time.Duration(float64(baseDuration) * multiplier)
	if result < time.Second {
		result = time.Second // Minimum of 1 second
	}

	return result
}
