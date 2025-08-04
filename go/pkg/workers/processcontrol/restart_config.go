package processcontrol

import (
	"fmt"
	"time"
)

// RestartPolicy defines when a process should be restarted (moved from monitoring package)
type RestartPolicy string

const (
	RestartNever         RestartPolicy = "never"
	RestartOnFailure     RestartPolicy = "on-failure"
	RestartAlways        RestartPolicy = "always"
	RestartUnlessStopped RestartPolicy = "unless-stopped"
)

// RestartTriggerType defines what triggered the restart request (moved from processcontrolimpl)
type RestartTriggerType string

const (
	RestartTriggerHealthFailure     RestartTriggerType = "health_failure"
	RestartTriggerResourceViolation RestartTriggerType = "resource_violation"
	RestartTriggerManual            RestartTriggerType = "manual"
)

// RestartContext provides context about restart trigger for intelligent decision making (moved from processcontrolimpl)
type RestartContext struct {
	TriggerType       RestartTriggerType `json:"trigger_type"`
	Severity          string             `json:"severity"`            // warning, critical, emergency
	WorkerProfileType string             `json:"worker_profile_type"` // batch, web, database, etc. - worker's load profile
	ViolationType     string             `json:"violation_type"`      // memory, cpu, health, etc.
	Message           string             `json:"message"`             // Human-readable reason
}

// RestartConfig defines retry mechanics (moved from processcontrolimpl, originally from monitoring)
type RestartConfig struct {
	MaxRetries  int           `yaml:"max_retries"`
	RetryDelay  time.Duration `yaml:"retry_delay"`
	BackoffRate float64       `yaml:"backoff_rate"` // Exponential backoff multiplier
}

// ValidateRestartConfig validates restart configuration values
func ValidateRestartConfig(config RestartConfig) error {
	if config.MaxRetries < 0 {
		return fmt.Errorf("max_retries cannot be negative: %d", config.MaxRetries)
	}
	if config.RetryDelay < 0 {
		return fmt.Errorf("retry_delay cannot be negative: %v", config.RetryDelay)
	}
	if config.BackoffRate <= 0 {
		return fmt.Errorf("backoff_rate must be positive: %f", config.BackoffRate)
	}
	return nil
}

// ContextAwareRestartConfig provides context-aware restart configuration (moved from processcontrolimpl)
type ContextAwareRestartConfig struct {
	// Default configuration for all restart scenarios
	Default RestartConfig `yaml:"default"`

	// Context-specific overrides
	HealthFailures     *RestartConfig `yaml:"health_failures,omitempty"`
	ResourceViolations *RestartConfig `yaml:"resource_violations,omitempty"`

	// Multipliers for different severities (applied to max_retries and retry_delay)
	SeverityMultipliers map[string]float64 `yaml:"severity_multipliers,omitempty"`

	// Multipliers for different worker profile types (applied to max_retries and retry_delay)
	WorkerProfileMultipliers map[string]float64 `yaml:"worker_profile_multipliers,omitempty"`

	// Time-based context awareness
	StartupGracePeriod     time.Duration `yaml:"startup_grace_period,omitempty"`     // No restarts during startup
	SustainedViolationTime time.Duration `yaml:"sustained_violation_time,omitempty"` // Resource violations must be sustained
	SpikeToleranceTime     time.Duration `yaml:"spike_tolerance_time,omitempty"`     // Allow brief resource spikes
}

// ValidateContextAwareRestartConfig validates context-aware restart configuration
func ValidateContextAwareRestartConfig(config ContextAwareRestartConfig) error {
	// Validate default configuration
	if err := ValidateRestartConfig(config.Default); err != nil {
		return fmt.Errorf("invalid default restart config: %w", err)
	}

	// Validate health failures override if provided
	if config.HealthFailures != nil {
		if err := ValidateRestartConfig(*config.HealthFailures); err != nil {
			return fmt.Errorf("invalid health_failures restart config: %w", err)
		}
	}

	// Validate resource violations override if provided
	if config.ResourceViolations != nil {
		if err := ValidateRestartConfig(*config.ResourceViolations); err != nil {
			return fmt.Errorf("invalid resource_violations restart config: %w", err)
		}
	}

	// Validate time durations
	if config.StartupGracePeriod < 0 {
		return fmt.Errorf("startup_grace_period cannot be negative: %v", config.StartupGracePeriod)
	}
	if config.SustainedViolationTime < 0 {
		return fmt.Errorf("sustained_violation_time cannot be negative: %v", config.SustainedViolationTime)
	}
	if config.SpikeToleranceTime < 0 {
		return fmt.Errorf("spike_tolerance_time cannot be negative: %v", config.SpikeToleranceTime)
	}

	// Validate severity multipliers
	for severity, multiplier := range config.SeverityMultipliers {
		if multiplier <= 0 {
			return fmt.Errorf("severity multiplier for '%s' must be positive: %f", severity, multiplier)
		}
	}

	// Validate worker profile multipliers
	for profile, multiplier := range config.WorkerProfileMultipliers {
		if multiplier <= 0 {
			return fmt.Errorf("worker profile multiplier for '%s' must be positive: %f", profile, multiplier)
		}
	}

	return nil
}
