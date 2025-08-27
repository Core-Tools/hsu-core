package processmanager

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/managedprocess"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Simple test logger that implements logging.Logger interface
type TestLogger struct{}

func (l *TestLogger) LogLevelf(level int, format string, args ...interface{}) {}
func (l *TestLogger) Debugf(format string, args ...interface{})               {}
func (l *TestLogger) Infof(format string, args ...interface{})                {}
func (l *TestLogger) Warnf(format string, args ...interface{})                {}
func (l *TestLogger) Errorf(format string, args ...interface{})               {}

// getTestExecutable returns a platform-specific executable path that exists
func getTestExecutable() (string, []string, string) {
	if runtime.GOOS == "windows" {
		return "C:\\Windows\\System32\\cmd.exe", []string{"/c", "echo", "test"}, "C:\\Windows\\Temp"
	} else {
		return "/bin/echo", []string{"test"}, "/tmp"
	}
}

// escapeForYAML properly escapes a path for YAML
func escapeForYAML(path string) string {
	if runtime.GOOS == "windows" {
		// Replace backslashes with forward slashes for YAML compatibility
		// Or escape backslashes properly
		result := ""
		for _, char := range path {
			if char == '\\' {
				result += "\\\\"
			} else {
				result += string(char)
			}
		}
		return result
	}
	return path
}

func TestLoadConfigFromFile(t *testing.T) {
	executablePath, args, workingDir := getTestExecutable()

	tests := []struct {
		name        string
		configYAML  string
		expectError bool
		validate    func(*testing.T, *ProcessManagerConfig)
	}{
		{
			name: "valid comprehensive config",
			configYAML: `
process_manager:
  port: 50055
  log_level: "info"

managed_processes:
  - id: "test-managed"
    type: "standard_managed"
    enabled: true
    unit:
      standard_managed:
        metadata:
          name: "Test Managed Service"
          description: "A test managed service"
        control:
          execution:
            executable_path: "` + escapeForYAML(executablePath) + `"
            args: ` + formatArgsForYAML(args) + `
            environment: ["LOG_LEVEL=debug"]
            working_directory: "` + escapeForYAML(workingDir) + `"
            wait_delay: "10s"
          restart_policy: "always"
          context_aware_restart:
            max_retries: 3
            retry_delay: "5s"
            backoff_rate: 1.5
        health_check:
          type: "http"
          http:
            url: "http://localhost:8080/health"
          run_options:
            enabled: true
            interval: "30s"
            timeout: "5s"

  - id: "test-integrated"
    type: "integrated_managed"
    unit:
      integrated_managed:
        metadata:
          name: "Test Integrated Service"
        control:
          execution:
            executable_path: "` + escapeForYAML(executablePath) + `"
            args: ` + formatArgsForYAML(args) + `
          restart_policy: "on-failure"
          context_aware_restart:
            max_retries: 3
            retry_delay: "5s"
            backoff_rate: 1.5
        health_check_run_options:
          enabled: true
          interval: "30s"
`,
			expectError: false,
			validate: func(t *testing.T, config *ProcessManagerConfig) {
				assert.Equal(t, 50055, config.ProcessManager.Port)
				assert.Equal(t, "info", config.ProcessManager.LogLevel)
				assert.Len(t, config.ManagedProcesses, 2)

				// Check standard managed process
				managed := config.ManagedProcesses[0]
				assert.Equal(t, "test-managed", managed.ID)
				assert.Equal(t, ProcessManagementTypeStandard, managed.Type)
				assert.True(t, *managed.Enabled)
				assert.NotNil(t, managed.Unit.StandardManaged)
				assert.Equal(t, "Test Managed Service", managed.Unit.StandardManaged.Metadata.Name)
				assert.Equal(t, executablePath, managed.Unit.StandardManaged.Control.Execution.ExecutablePath)
				assert.Equal(t, args, managed.Unit.StandardManaged.Control.Execution.Args)
				assert.Equal(t, processcontrol.RestartAlways, managed.Unit.StandardManaged.Control.RestartPolicy)

				// Check integrated managed process
				integrated := config.ManagedProcesses[1]
				assert.Equal(t, "test-integrated", integrated.ID)
				assert.Equal(t, ProcessManagementTypeIntegrated, integrated.Type)
				assert.True(t, *integrated.Enabled) // Should default to true
				assert.NotNil(t, integrated.Unit.IntegratedManaged)
			},
		},
		{
			name: "minimal valid config",
			configYAML: `
process_manager:
  port: 50055

managed_processes:
  - id: "simple-process"
    type: "standard_managed"
    unit:
      standard_managed:
        metadata:
          name: "Simple Managed Process"
        control:
          execution:
            executable_path: "` + escapeForYAML(executablePath) + `"
          restart:
            policy: "never"
`,
			expectError: false,
			validate: func(t *testing.T, config *ProcessManagerConfig) {
				assert.Equal(t, 50055, config.ProcessManager.Port)
				assert.Equal(t, "info", config.ProcessManager.LogLevel) // Should use default
				assert.Len(t, config.ManagedProcesses, 1)

				process := config.ManagedProcesses[0]
				assert.Equal(t, "simple-process", process.ID)
				assert.True(t, *process.Enabled) // Should default to true
			},
		},
		{
			name: "invalid YAML",
			configYAML: `
process_manager:
  port: 50055
  invalid_yaml: [unclosed
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpFile, err := os.CreateTemp("", "config-test-*.yaml")
			require.NoError(t, err)
			defer os.Remove(tmpFile.Name())

			// Write config to file
			_, err = tmpFile.WriteString(tt.configYAML)
			require.NoError(t, err)
			tmpFile.Close()

			// Load configuration
			config, err := LoadConfigFromFile(tmpFile.Name())

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, config)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
				if tt.validate != nil {
					tt.validate(t, config)
				}
			}
		})
	}
}

// formatArgsForYAML formats args slice for YAML
func formatArgsForYAML(args []string) string {
	result := "["
	for i, arg := range args {
		if i > 0 {
			result += ", "
		}
		result += `"` + arg + `"`
	}
	result += "]"
	return result
}

func TestValidateConfig(t *testing.T) {
	executablePath, _, _ := getTestExecutable()

	tests := []struct {
		name        string
		config      *ProcessManagerConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: &ProcessManagerConfig{
				ProcessManager: ProcessManagerConfigOptions{
					Port:     50055,
					LogLevel: "info",
				},
				ManagedProcesses: []ProcessConfig{
					{
						ID:      "test-process",
						Type:    ProcessManagementTypeStandard,
						Enabled: func() *bool { b := true; return &b }(),
						Unit: ProcessUnitConfig{
							StandardManaged: &managedprocess.StandardManagedProcessConfig{
								Metadata: managedprocess.ProcessMetadata{
									Name: "Test Managed Process",
								},
								Control: processcontrol.ManagedProcessControlConfig{
									Execution: process.ExecutionConfig{
										ExecutablePath: executablePath,
									},
									ContextAwareRestart: processcontrol.ContextAwareRestartConfig{
										Default: processcontrol.RestartConfig{
											MaxRetries:  3,
											RetryDelay:  5 * time.Second,
											BackoffRate: 1.5,
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "invalid port",
			config: &ProcessManagerConfig{
				ProcessManager: ProcessManagerConfigOptions{
					Port: -1, // Invalid port
				},
				ManagedProcesses: []ProcessConfig{
					{
						ID:   "test-process",
						Type: ProcessManagementTypeStandard,
						Unit: ProcessUnitConfig{
							StandardManaged: &managedprocess.StandardManagedProcessConfig{
								Metadata: managedprocess.ProcessMetadata{Name: "Test"},
								Control: processcontrol.ManagedProcessControlConfig{
									Execution: process.ExecutionConfig{ExecutablePath: executablePath},
									ContextAwareRestart: processcontrol.ContextAwareRestartConfig{
										Default: processcontrol.RestartConfig{
											MaxRetries:  3,
											RetryDelay:  5 * time.Second,
											BackoffRate: 1.5,
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateProcessessFromConfig(t *testing.T) {
	executablePath, _, _ := getTestExecutable()
	testLogger := &TestLogger{}

	config := &ProcessManagerConfig{
		ProcessManager: ProcessManagerConfigOptions{Port: 50055},
		ManagedProcesses: []ProcessConfig{
			{
				ID:      "managed-process",
				Type:    ProcessManagementTypeStandard,
				Enabled: func() *bool { b := true; return &b }(),
				Unit: ProcessUnitConfig{
					StandardManaged: &managedprocess.StandardManagedProcessConfig{
						Metadata: managedprocess.ProcessMetadata{Name: "Managed Test"},
						Control: processcontrol.ManagedProcessControlConfig{
							Execution: process.ExecutionConfig{ExecutablePath: executablePath},
							ContextAwareRestart: processcontrol.ContextAwareRestartConfig{
								Default: processcontrol.RestartConfig{
									MaxRetries:  3,
									RetryDelay:  5 * time.Second,
									BackoffRate: 1.5,
								},
							},
						},
					},
				},
			},
			{
				ID:      "disabled-process",
				Type:    ProcessManagementTypeStandard,
				Enabled: func() *bool { b := false; return &b }(), // Disabled
				Unit: ProcessUnitConfig{
					StandardManaged: &managedprocess.StandardManagedProcessConfig{
						Metadata: managedprocess.ProcessMetadata{Name: "Disabled Test"},
						Control: processcontrol.ManagedProcessControlConfig{
							Execution: process.ExecutionConfig{ExecutablePath: executablePath},
							ContextAwareRestart: processcontrol.ContextAwareRestartConfig{
								Default: processcontrol.RestartConfig{
									MaxRetries:  0,
									RetryDelay:  0,
									BackoffRate: 1.0,
								},
							},
						},
					},
				},
			},
		},
	}

	processes, err := CreateProcessesFromConfig(config, testLogger)

	assert.NoError(t, err)
	assert.Len(t, processes, 1) // Should skip disabled process

	// Check process ID
	assert.Equal(t, "managed-process", processes[0].ID())
}

func TestConfigDefaults(t *testing.T) {
	executablePath, _, _ := getTestExecutable()

	config := &ProcessManagerConfig{
		ProcessManager: ProcessManagerConfigOptions{
			// Port not set - should get default
		},
		ManagedProcesses: []ProcessConfig{
			{
				ID:   "test-process",
				Type: ProcessManagementTypeStandard,
				// Enabled not set - should default to true
				Unit: ProcessUnitConfig{
					StandardManaged: &managedprocess.StandardManagedProcessConfig{
						Metadata: managedprocess.ProcessMetadata{Name: "Test"},
						Control: processcontrol.ManagedProcessControlConfig{
							Execution: process.ExecutionConfig{
								ExecutablePath: executablePath,
								// WaitDelay not set - should get default
							},
							// RestartPolicy not set - should get default
							ContextAwareRestart: processcontrol.ContextAwareRestartConfig{
								Default: processcontrol.RestartConfig{
									// MaxRetries not set - should get default
								},
							},
						},
					},
				},
			},
		},
	}

	err := setConfigDefaults(config)
	assert.NoError(t, err)

	// Check process manager defaults
	assert.Equal(t, 50055, config.ProcessManager.Port)
	assert.Equal(t, "info", config.ProcessManager.LogLevel)

	// Check process defaults
	process := config.ManagedProcesses[0]
	assert.True(t, *process.Enabled) // Now checking pointer
	assert.Equal(t, 10*time.Second, process.Unit.StandardManaged.Control.Execution.WaitDelay)
	assert.Equal(t, processcontrol.RestartOnFailure, process.Unit.StandardManaged.Control.RestartPolicy)
	assert.Equal(t, 3, process.Unit.StandardManaged.Control.ContextAwareRestart.Default.MaxRetries)
	assert.Equal(t, 5*time.Second, process.Unit.StandardManaged.Control.ContextAwareRestart.Default.RetryDelay)
	assert.Equal(t, 1.5, process.Unit.StandardManaged.Control.ContextAwareRestart.Default.BackoffRate)
}

func TestGetConfigSummary(t *testing.T) {
	executablePath, _, _ := getTestExecutable()

	config := &ProcessManagerConfig{
		ProcessManager: ProcessManagerConfigOptions{
			Port:     50055,
			LogLevel: "debug",
		},
		ManagedProcesses: []ProcessConfig{
			{
				ID:      "web-service",
				Type:    ProcessManagementTypeStandard,
				Enabled: func() *bool { b := true; return &b }(),
				Unit: ProcessUnitConfig{
					StandardManaged: &managedprocess.StandardManagedProcessConfig{
						Metadata: managedprocess.ProcessMetadata{Name: "Web Service"},
						Control: processcontrol.ManagedProcessControlConfig{
							Execution:     process.ExecutionConfig{ExecutablePath: executablePath},
							RestartPolicy: processcontrol.RestartAlways,
						},
						HealthCheck: monitoring.HealthCheckConfig{
							Type: monitoring.HealthCheckTypeHTTP,
						},
					},
				},
			},
			{
				ID:      "db-monitor",
				Type:    ProcessManagementTypeUnmanaged,
				Enabled: func() *bool { b := false; return &b }(),
				Unit: ProcessUnitConfig{
					Unmanaged: &managedprocess.UnmanagedProcessConfig{
						Metadata:    managedprocess.ProcessMetadata{Name: "DB Monitor"},
						Discovery:   process.DiscoveryConfig{Method: process.DiscoveryMethodPIDFile},
						HealthCheck: monitoring.HealthCheckConfig{Type: monitoring.HealthCheckTypeTCP},
					},
				},
			},
		},
	}

	summary := GetConfigSummary(config)

	assert.Equal(t, 50055, summary.ProcessManagerPort)
	assert.Equal(t, "debug", summary.LogLevel)
	assert.Equal(t, 2, summary.TotalProcesses)
	assert.Equal(t, 1, summary.EnabledProcesses)
	assert.Len(t, summary.ManagedProcesses, 2)

	// Check first process summary
	webProcess := summary.ManagedProcesses[0]
	assert.Equal(t, "web-service", webProcess.ID)
	assert.Equal(t, "standard_managed", webProcess.Type)
	assert.True(t, webProcess.Enabled)
	assert.Equal(t, executablePath, webProcess.ExecutablePath)
	assert.Equal(t, "http", webProcess.HealthCheckType)

	// Check second process summary
	dbProcess := summary.ManagedProcesses[1]
	assert.Equal(t, "db-monitor", dbProcess.ID)
	assert.Equal(t, "unmanaged", dbProcess.Type)
	assert.False(t, dbProcess.Enabled)
	assert.Equal(t, "pid-file", dbProcess.DiscoveryMethod)
	assert.Equal(t, "tcp", dbProcess.HealthCheckType)
}

func TestValidateConfigFile(t *testing.T) {
	executablePath, _, _ := getTestExecutable()

	// Create a valid config file
	validConfig := `
process_manager:
  port: 50055

managed_processes:
  - id: "test-process"
    type: "standard_managed"
    unit:
      standard_managed:
        metadata:
          name: "Test Managed Process"
        control:
          execution:
            executable_path: "` + escapeForYAML(executablePath) + `"
          restart:
            policy: "on-failure"
            max_retries: 3
            retry_delay: "5s"
            backoff_rate: 1.5
`

	tmpFile, err := os.CreateTemp("", "valid-config-*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(validConfig)
	require.NoError(t, err)
	tmpFile.Close()

	// Test validation
	err = ValidateConfigFile(tmpFile.Name())
	assert.NoError(t, err)

	// Test with non-existent file
	err = ValidateConfigFile("/non/existent/file.yaml")
	assert.Error(t, err)
	assert.True(t, errors.IsIOError(err))
}
