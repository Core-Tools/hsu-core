package managedprocess

import (
	"fmt"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"
)

func ValidateManagedUnit(config ManagedUnit) error {
	// Validate metadata
	if config.Metadata.Name == "" {
		return errors.NewValidationError("unit name is required", nil)
	}

	// Validate execution config
	if config.Control.Execution.ExecutablePath == "" {
		return errors.NewValidationError("executable path is required for managed worker", nil)
	}

	// Additional validation using existing functions
	if err := process.ValidateExecutionConfig(config.Control.Execution); err != nil {
		return err
	}

	// Validate context-aware restart configuration
	if err := processcontrol.ValidateContextAwareRestartConfig(config.Control.ContextAwareRestart); err != nil {
		return fmt.Errorf("invalid context-aware restart configuration: %w", err)
	}

	if config.HealthCheck.Type != "" {
		if err := monitoring.ValidateHealthCheckConfig(config.HealthCheck); err != nil {
			return err
		}
	}

	return nil
}

func ValidateUnmanagedUnit(config UnmanagedUnit) error {
	// Validate metadata
	if config.Metadata.Name == "" {
		return errors.NewValidationError("unit name is required", nil)
	}

	// Validate discovery config
	if err := process.ValidateDiscoveryConfig(config.Discovery); err != nil {
		return err
	}

	// Validate health check if specified
	if config.HealthCheck.Type != "" {
		if err := monitoring.ValidateHealthCheckConfig(config.HealthCheck); err != nil {
			return err
		}
	}

	return nil
}

func ValidateIntegratedUnit(config IntegratedUnit) error {
	// Validate metadata
	if config.Metadata.Name == "" {
		return errors.NewValidationError("unit name is required", nil)
	}

	// Validate execution config
	if config.Control.Execution.ExecutablePath == "" {
		return errors.NewValidationError("executable path is required for integrated worker", nil)
	}

	// Additional validation
	if err := process.ValidateExecutionConfig(config.Control.Execution); err != nil {
		return err
	}

	// Validate context-aware restart configuration
	if err := processcontrol.ValidateContextAwareRestartConfig(config.Control.ContextAwareRestart); err != nil {
		return fmt.Errorf("invalid context-aware restart configuration: %w", err)
	}

	return nil
}

// ValidateProcessControlOptions validates process control options
func ValidateProcessControlOptions(options processcontrol.ProcessControlOptions) error {
	// Validate graceful timeout
	if options.GracefulTimeout < 0 {
		return errors.NewValidationError("graceful timeout cannot be negative", nil)
	}

	// ✅ REMOVED: Restart validation - field replaced with ContextAwareRestart and RestartPolicy

	// Resource limits validation is now handled in the resourcelimits package

	// Validate health check config if provided
	if options.HealthCheck != nil {
		if err := monitoring.ValidateHealthCheckConfig(*options.HealthCheck); err != nil {
			return errors.NewValidationError("invalid health check configuration", err)
		}
	}

	// Validate consistency
	if !options.CanAttach && options.ExecuteCmd == nil {
		return errors.NewValidationError("either CanAttach must be true or ExecuteCmd must be provided", nil)
	}

	if options.CanAttach && options.AttachCmd == nil {
		return errors.NewValidationError("AttachCmd must be provided if CanAttach is true", nil)
	}

	return nil
}
