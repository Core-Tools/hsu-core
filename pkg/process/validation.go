package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/core-tools/hsu-procman-go/pkg/errors"
)

// ValidateDiscoveryConfig validates discovery configuration
func ValidateDiscoveryConfig(config DiscoveryConfig) error {
	switch config.Method {
	case DiscoveryMethodPIDFile:
		if config.PIDFile == "" {
			return errors.NewValidationError("PID file path is required for PID file discovery", nil)
		}
		if !filepath.IsAbs(config.PIDFile) {
			return errors.NewValidationError("PID file path must be absolute", nil)
		}

	case DiscoveryMethodProcessName:
		if config.ProcessName == "" {
			return errors.NewValidationError("process name is required for process name discovery", nil)
		}

	case DiscoveryMethodPort:
		if config.Port <= 0 || config.Port > 65535 {
			return errors.NewValidationError("port must be between 1 and 65535", nil)
		}
		if config.Protocol != "tcp" && config.Protocol != "udp" {
			return errors.NewValidationError("protocol must be 'tcp' or 'udp'", nil)
		}

	case DiscoveryMethodServiceName:
		if config.ServiceName == "" {
			return errors.NewValidationError("service name is required for service name discovery", nil)
		}

	default:
		return errors.NewValidationError("unsupported discovery method: "+string(config.Method), nil)
	}

	// Validate check interval
	if config.CheckInterval < 0 {
		return errors.NewValidationError("check interval cannot be negative", nil)
	}

	return nil
}

// ValidatePIDFile validates PID file format and accessibility
func ValidatePIDFile(pidFile string) error {
	if pidFile == "" {
		return errors.NewValidationError("PID file path cannot be empty", nil)
	}

	if !filepath.IsAbs(pidFile) {
		return errors.NewValidationError("PID file path must be absolute", nil)
	}

	// Check if parent directory exists
	dir := filepath.Dir(pidFile)
	if info, err := os.Stat(dir); err != nil {
		return errors.NewIOError("PID file directory not accessible: "+dir, err)
	} else if !info.IsDir() {
		return errors.NewValidationError("PID file parent is not a directory: "+dir, nil)
	}

	return nil
}

// ValidatePID validates PID value
func ValidatePID(pidStr string) (int, error) {
	if pidStr == "" {
		return 0, errors.NewValidationError("PID cannot be empty", nil)
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, errors.NewValidationError("invalid PID format: "+pidStr, err)
	}

	if pid <= 0 {
		return 0, errors.NewValidationError("PID must be positive: "+pidStr, nil)
	}

	return pid, nil
}

// ValidateExecutionConfig validates execution configuration
func ValidateExecutionConfig(config ExecutionConfig) error {
	// Validate executable path
	if config.ExecutablePath == "" {
		return errors.NewValidationError("executable path is required", nil)
	}

	// Check if executable exists
	if _, err := os.Stat(config.ExecutablePath); os.IsNotExist(err) {
		return errors.NewValidationError("executable not found: "+config.ExecutablePath, err)
	}

	// Validate working directory if provided
	if config.WorkingDirectory != "" {
		if !filepath.IsAbs(config.WorkingDirectory) {
			return errors.NewValidationError("working directory must be absolute path", nil)
		}

		if info, err := os.Stat(config.WorkingDirectory); err != nil {
			return errors.NewValidationError("working directory not accessible: "+config.WorkingDirectory, err)
		} else if !info.IsDir() {
			return errors.NewValidationError("working directory is not a directory: "+config.WorkingDirectory, nil)
		}
	}

	// Validate environment variables
	for _, env := range config.Environment {
		if !strings.Contains(env, "=") {
			return errors.NewValidationError("invalid environment variable format: "+env, nil)
		}
	}

	// Validate wait delay
	if config.WaitDelay < 0 {
		return errors.NewValidationError("wait delay cannot be negative", nil)
	}

	return nil
}
