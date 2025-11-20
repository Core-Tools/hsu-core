package processmanagement

import (
	"testing"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/errors"

	"github.com/stretchr/testify/assert"
)

func TestValidateProcessID(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		shouldErr bool
		errorType errors.ErrorType
	}{
		{"valid_simple", "process-1", false, ""},
		{"valid_with_underscore", "process_1", false, ""},
		{"valid_alphanumeric", "process123", false, ""},
		{"empty_id", "", true, errors.ErrorTypeValidation},
		{"too_long", "a" + string(make([]byte, 65)), true, errors.ErrorTypeValidation},
		{"invalid_chars", "process@1", true, errors.ErrorTypeValidation},
		{"invalid_space", "process 1", true, errors.ErrorTypeValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProcessID(tt.id)

			if tt.shouldErr {
				assert.Error(t, err)
				var domainErr *errors.DomainError
				assert.ErrorAs(t, err, &domainErr)
				assert.Equal(t, tt.errorType, domainErr.Type)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
func TestValidatePort(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		shouldErr bool
	}{
		{"valid_port", 8080, false},
		{"port_1", 1, false},
		{"port_65535", 65535, false},
		{"port_0", 0, true},
		{"port_negative", -1, true},
		{"port_too_high", 65536, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)

			if tt.shouldErr {
				assert.Error(t, err)
				assert.True(t, errors.IsValidationError(err))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateNetworkAddress(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		shouldErr bool
	}{
		{"valid_localhost", "localhost:8080", false},
		{"valid_ip", "127.0.0.1:8080", false},
		{"valid_hostname", "example.com:80", false},
		{"empty_address", "", true},
		{"no_port", "localhost", true},
		{"invalid_port", "localhost:abc", true},
		{"port_too_high", "localhost:65536", true},
		{"empty_host", ":8080", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNetworkAddress(tt.address)

			if tt.shouldErr {
				assert.Error(t, err)
				assert.True(t, errors.IsValidationError(err))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateTimeout(t *testing.T) {
	tests := []struct {
		name      string
		timeout   time.Duration
		shouldErr bool
	}{
		{"valid_timeout", 30 * time.Second, false},
		{"negative_timeout", -1 * time.Second, true},
		{"zero_timeout", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeout(tt.timeout, "test")

			if tt.shouldErr {
				assert.Error(t, err)
				assert.True(t, errors.IsValidationError(err))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
