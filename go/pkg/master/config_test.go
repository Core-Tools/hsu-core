package master

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"
	"github.com/core-tools/hsu-core/pkg/workers"
	"github.com/core-tools/hsu-core/pkg/workers/processcontrol"

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
		validate    func(*testing.T, *MasterConfig)
	}{
		{
			name: "valid comprehensive config",
			configYAML: `
master:
  port: 50055
  log_level: "info"

workers:
  - id: "test-managed"
    type: "managed"
    enabled: true
    unit:
      managed:
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
    type: "integrated"
    unit:
      integrated:
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
			validate: func(t *testing.T, config *MasterConfig) {
				assert.Equal(t, 50055, config.Master.Port)
				assert.Equal(t, "info", config.Master.LogLevel)
				assert.Len(t, config.Workers, 2)

				// Check managed worker
				managed := config.Workers[0]
				assert.Equal(t, "test-managed", managed.ID)
				assert.Equal(t, WorkerManagementTypeManaged, managed.Type)
				assert.True(t, *managed.Enabled)
				assert.NotNil(t, managed.Unit.Managed)
				assert.Equal(t, "Test Managed Service", managed.Unit.Managed.Metadata.Name)
				assert.Equal(t, executablePath, managed.Unit.Managed.Control.Execution.ExecutablePath)
				assert.Equal(t, args, managed.Unit.Managed.Control.Execution.Args)
				assert.Equal(t, processcontrol.RestartAlways, managed.Unit.Managed.Control.RestartPolicy)

				// Check integrated worker
				integrated := config.Workers[1]
				assert.Equal(t, "test-integrated", integrated.ID)
				assert.Equal(t, WorkerManagementTypeIntegrated, integrated.Type)
				assert.True(t, *integrated.Enabled) // Should default to true
				assert.NotNil(t, integrated.Unit.Integrated)
			},
		},
		{
			name: "minimal valid config",
			configYAML: `
master:
  port: 50055

workers:
  - id: "simple-worker"
    type: "managed"
    unit:
      managed:
        metadata:
          name: "Simple Worker"
        control:
          execution:
            executable_path: "` + escapeForYAML(executablePath) + `"
          restart:
            policy: "never"
`,
			expectError: false,
			validate: func(t *testing.T, config *MasterConfig) {
				assert.Equal(t, 50055, config.Master.Port)
				assert.Equal(t, "info", config.Master.LogLevel) // Should use default
				assert.Len(t, config.Workers, 1)

				worker := config.Workers[0]
				assert.Equal(t, "simple-worker", worker.ID)
				assert.True(t, *worker.Enabled) // Should default to true
			},
		},
		{
			name: "invalid YAML",
			configYAML: `
master:
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
		config      *MasterConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: &MasterConfig{
				Master: MasterConfigOptions{
					Port:     50055,
					LogLevel: "info",
				},
				Workers: []WorkerConfig{
					{
						ID:      "test-worker",
						Type:    WorkerManagementTypeManaged,
						Enabled: func() *bool { b := true; return &b }(),
						Unit: WorkerUnitConfig{
							Managed: &workers.ManagedUnit{
								Metadata: workers.UnitMetadata{
									Name: "Test Worker",
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
			config: &MasterConfig{
				Master: MasterConfigOptions{
					Port: -1, // Invalid port
				},
				Workers: []WorkerConfig{
					{
						ID:   "test-worker",
						Type: WorkerManagementTypeManaged,
						Unit: WorkerUnitConfig{
							Managed: &workers.ManagedUnit{
								Metadata: workers.UnitMetadata{Name: "Test"},
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

func TestCreateWorkersFromConfig(t *testing.T) {
	executablePath, _, _ := getTestExecutable()
	testLogger := &TestLogger{}

	config := &MasterConfig{
		Master: MasterConfigOptions{Port: 50055},
		Workers: []WorkerConfig{
			{
				ID:      "managed-worker",
				Type:    WorkerManagementTypeManaged,
				Enabled: func() *bool { b := true; return &b }(),
				Unit: WorkerUnitConfig{
					Managed: &workers.ManagedUnit{
						Metadata: workers.UnitMetadata{Name: "Managed Test"},
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
				ID:      "disabled-worker",
				Type:    WorkerManagementTypeManaged,
				Enabled: func() *bool { b := false; return &b }(), // Disabled
				Unit: WorkerUnitConfig{
					Managed: &workers.ManagedUnit{
						Metadata: workers.UnitMetadata{Name: "Disabled Test"},
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

	workers, err := CreateWorkersFromConfig(config, testLogger)

	assert.NoError(t, err)
	assert.Len(t, workers, 1) // Should skip disabled worker

	// Check worker ID
	assert.Equal(t, "managed-worker", workers[0].ID())
}

func TestConfigDefaults(t *testing.T) {
	executablePath, _, _ := getTestExecutable()

	config := &MasterConfig{
		Master: MasterConfigOptions{
			// Port not set - should get default
		},
		Workers: []WorkerConfig{
			{
				ID:   "test-worker",
				Type: WorkerManagementTypeManaged,
				// Enabled not set - should default to true
				Unit: WorkerUnitConfig{
					Managed: &workers.ManagedUnit{
						Metadata: workers.UnitMetadata{Name: "Test"},
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

	// Check master defaults
	assert.Equal(t, 50055, config.Master.Port)
	assert.Equal(t, "info", config.Master.LogLevel)

	// Check worker defaults
	worker := config.Workers[0]
	assert.True(t, *worker.Enabled) // Now checking pointer
	assert.Equal(t, 10*time.Second, worker.Unit.Managed.Control.Execution.WaitDelay)
	assert.Equal(t, processcontrol.RestartOnFailure, worker.Unit.Managed.Control.RestartPolicy)
	assert.Equal(t, 3, worker.Unit.Managed.Control.ContextAwareRestart.Default.MaxRetries)
	assert.Equal(t, 5*time.Second, worker.Unit.Managed.Control.ContextAwareRestart.Default.RetryDelay)
	assert.Equal(t, 1.5, worker.Unit.Managed.Control.ContextAwareRestart.Default.BackoffRate)
}

func TestGetConfigSummary(t *testing.T) {
	executablePath, _, _ := getTestExecutable()

	config := &MasterConfig{
		Master: MasterConfigOptions{
			Port:     50055,
			LogLevel: "debug",
		},
		Workers: []WorkerConfig{
			{
				ID:      "web-service",
				Type:    WorkerManagementTypeManaged,
				Enabled: func() *bool { b := true; return &b }(),
				Unit: WorkerUnitConfig{
					Managed: &workers.ManagedUnit{
						Metadata: workers.UnitMetadata{Name: "Web Service"},
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
				Type:    WorkerManagementTypeUnmanaged,
				Enabled: func() *bool { b := false; return &b }(),
				Unit: WorkerUnitConfig{
					Unmanaged: &workers.UnmanagedUnit{
						Metadata:    workers.UnitMetadata{Name: "DB Monitor"},
						Discovery:   process.DiscoveryConfig{Method: process.DiscoveryMethodPIDFile},
						HealthCheck: monitoring.HealthCheckConfig{Type: monitoring.HealthCheckTypeTCP},
					},
				},
			},
		},
	}

	summary := GetConfigSummary(config)

	assert.Equal(t, 50055, summary.MasterPort)
	assert.Equal(t, "debug", summary.LogLevel)
	assert.Equal(t, 2, summary.TotalWorkers)
	assert.Equal(t, 1, summary.EnabledWorkers)
	assert.Len(t, summary.Workers, 2)

	// Check first worker summary
	webWorker := summary.Workers[0]
	assert.Equal(t, "web-service", webWorker.ID)
	assert.Equal(t, "managed", webWorker.Type)
	assert.True(t, webWorker.Enabled)
	assert.Equal(t, executablePath, webWorker.ExecutablePath)
	assert.Equal(t, "http", webWorker.HealthCheckType)

	// Check second worker summary
	dbWorker := summary.Workers[1]
	assert.Equal(t, "db-monitor", dbWorker.ID)
	assert.Equal(t, "unmanaged", dbWorker.Type)
	assert.False(t, dbWorker.Enabled)
	assert.Equal(t, "pid-file", dbWorker.DiscoveryMethod)
	assert.Equal(t, "tcp", dbWorker.HealthCheckType)
}

func TestValidateConfigFile(t *testing.T) {
	executablePath, _, _ := getTestExecutable()

	// Create a valid config file
	validConfig := `
master:
  port: 50055

workers:
  - id: "test-worker"
    type: "managed"
    unit:
      managed:
        metadata:
          name: "Test Worker"
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
