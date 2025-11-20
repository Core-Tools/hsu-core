package process

import (
	"runtime"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"

	"github.com/stretchr/testify/assert"
)

func TestValidateDiscoveryConfig(t *testing.T) {
	// Use OS-dependent path for PID file
	var pidFile string
	if runtime.GOOS == "windows" {
		pidFile = "C:\\Temp\\test.pid"
	} else {
		pidFile = "/tmp/test.pid"
	}

	tests := []struct {
		name      string
		config    DiscoveryConfig
		shouldErr bool
	}{
		{
			name: "valid_pid_file",
			config: DiscoveryConfig{
				Method:  DiscoveryMethodPIDFile,
				PIDFile: pidFile,
			},
			shouldErr: false,
		},
		{
			name: "valid_process_name",
			config: DiscoveryConfig{
				Method:      DiscoveryMethodProcessName,
				ProcessName: "nginx",
			},
			shouldErr: false,
		},
		{
			name: "valid_port",
			config: DiscoveryConfig{
				Method:   DiscoveryMethodPort,
				Port:     8080,
				Protocol: "tcp",
			},
			shouldErr: false,
		},
		{
			name: "valid_service",
			config: DiscoveryConfig{
				Method:      DiscoveryMethodServiceName,
				ServiceName: "nginx",
			},
			shouldErr: false,
		},
		{
			name: "invalid_pid_file_empty",
			config: DiscoveryConfig{
				Method:  DiscoveryMethodPIDFile,
				PIDFile: "",
			},
			shouldErr: true,
		},
		{
			name: "invalid_pid_file_relative",
			config: DiscoveryConfig{
				Method:  DiscoveryMethodPIDFile,
				PIDFile: "relative/path.pid",
			},
			shouldErr: true,
		},
		{
			name: "invalid_port_zero",
			config: DiscoveryConfig{
				Method:   DiscoveryMethodPort,
				Port:     0,
				Protocol: "tcp",
			},
			shouldErr: true,
		},
		{
			name: "invalid_port_too_high",
			config: DiscoveryConfig{
				Method:   DiscoveryMethodPort,
				Port:     65536,
				Protocol: "tcp",
			},
			shouldErr: true,
		},
		{
			name: "invalid_protocol",
			config: DiscoveryConfig{
				Method:   DiscoveryMethodPort,
				Port:     8080,
				Protocol: "http",
			},
			shouldErr: true,
		},
		{
			name: "negative_check_interval",
			config: DiscoveryConfig{
				Method:        DiscoveryMethodPIDFile,
				PIDFile:       pidFile,
				CheckInterval: -1 * time.Second,
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDiscoveryConfig(tt.config)

			if tt.shouldErr {
				assert.Error(t, err)
				assert.True(t, errors.IsValidationError(err))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePID(t *testing.T) {
	tests := []struct {
		name        string
		pidStr      string
		expectedPID int
		shouldErr   bool
	}{
		{"valid_pid", "1234", 1234, false},
		{"empty_pid", "", 0, true},
		{"invalid_format", "abc", 0, true},
		{"zero_pid", "0", 0, true},
		{"negative_pid", "-1", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid, err := ValidatePID(tt.pidStr)

			if tt.shouldErr {
				assert.Error(t, err)
				assert.True(t, errors.IsValidationError(err))
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPID, pid)
			}
		})
	}
}
