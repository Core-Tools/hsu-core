package processmanager

import (
	"net"
	"strconv"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
)

// ValidateProcessID validates process ID format and constraints
func ValidateProcessID(id string) error {
	if id == "" {
		return errors.NewValidationError("process ID cannot be empty", nil)
	}

	if len(id) > 64 {
		return errors.NewValidationError("process ID cannot exceed 64 characters", nil)
	}

	// Check for invalid characters
	for _, char := range id {
		if !isValidIDChar(char) {
			return errors.NewValidationError("process ID contains invalid characters: only letters, numbers, hyphens, and underscores are allowed", nil)
		}
	}

	return nil
}

// ValidatePort validates port number
func ValidatePort(port int) error {
	if port <= 0 || port > 65535 {
		return errors.NewValidationError("port must be between 1 and 65535", nil)
	}
	return nil
}

// ValidateNetworkAddress validates network address format
func ValidateNetworkAddress(address string) error {
	if address == "" {
		return errors.NewValidationError("network address cannot be empty", nil)
	}

	// Try to parse as host:port
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return errors.NewValidationError("invalid network address format: "+address, err)
	}

	// Validate host
	if host == "" {
		return errors.NewValidationError("host cannot be empty in address: "+address, nil)
	}

	// Validate port
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return errors.NewValidationError("invalid port in address: "+address, err)
	}

	if err := ValidatePort(port); err != nil {
		return errors.NewValidationError("invalid port in address: "+address, err)
	}

	return nil
}

// ValidateTimeout validates timeout duration
func ValidateTimeout(timeout time.Duration, name string) error {
	if timeout < 0 {
		return errors.NewValidationError(name+" timeout cannot be negative", nil)
	}

	if timeout == 0 {
		return errors.NewValidationError(name+" timeout cannot be zero", nil)
	}

	return nil
}

// Helper function to check if character is valid for ID
func isValidIDChar(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == '-' || char == '_'
}
