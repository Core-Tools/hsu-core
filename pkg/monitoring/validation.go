package monitoring

import "github.com/core-tools/hsu-procman-go/pkg/errors"

// ValidateHealthCheckConfig validates health check configuration
func ValidateHealthCheckConfig(config HealthCheckConfig) error {
	// Validate run options
	if err := ValidateHealthCheckRunOptions(config.RunOptions); err != nil {
		return errors.NewValidationError("invalid health check run options", err)
	}

	// Validate type-specific configuration
	switch config.Type {
	case HealthCheckTypeHTTP:
		if config.HTTP.URL == "" {
			return errors.NewValidationError("HTTP URL is required for HTTP health check", nil)
		}

	case HealthCheckTypeGRPC:
		if config.GRPC.Address == "" {
			return errors.NewValidationError("gRPC address is required for gRPC health check", nil)
		}
		if config.GRPC.Service == "" {
			return errors.NewValidationError("gRPC service is required for gRPC health check", nil)
		}
		if config.GRPC.Method == "" {
			return errors.NewValidationError("gRPC method is required for gRPC health check", nil)
		}

	case HealthCheckTypeTCP:
		if config.TCP.Address == "" {
			return errors.NewValidationError("TCP address is required for TCP health check", nil)
		}
		if config.TCP.Port <= 0 || config.TCP.Port > 65535 {
			return errors.NewValidationError("TCP port must be between 1 and 65535", nil)
		}

	case HealthCheckTypeExec:
		if config.Exec.Command == "" {
			return errors.NewValidationError("command is required for exec health check", nil)
		}

	case HealthCheckTypeProcess:
		// Process health check doesn't need additional validation

	default:
		return errors.NewValidationError("unsupported health check type: "+string(config.Type), nil)
	}

	return nil
}

// ValidateHealthCheckRunOptions validates health check run options
func ValidateHealthCheckRunOptions(options HealthCheckRunOptions) error {
	if options.Interval <= 0 {
		return errors.NewValidationError("health check interval must be positive", nil)
	}

	if options.Timeout <= 0 {
		return errors.NewValidationError("health check timeout must be positive", nil)
	}

	if options.Timeout >= options.Interval {
		return errors.NewValidationError("health check timeout must be less than interval", nil)
	}

	if options.Retries < 0 {
		return errors.NewValidationError("health check retries cannot be negative", nil)
	}

	return nil
}
