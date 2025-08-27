package processmanager

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	logconfig "github.com/core-tools/hsu-core/pkg/logcollection/config"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/managedprocess"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"

	"gopkg.in/yaml.v3"
)

// ProcessManagerConfig represents the top-level configuration file structure
type ProcessManagerConfig struct {
	ProcessManager   ProcessManagerConfigOptions    `yaml:"process_manager"`
	ManagedProcesses []ProcessConfig                `yaml:"managed_processes"`
	LogCollection    *logconfig.LogCollectionConfig `yaml:"log_collection,omitempty"` // Optional log collection configuration
}

// ProcessManagerConfigOptions represents process manager-level configuration
type ProcessManagerConfigOptions struct {
	Port                 int           `yaml:"port"`
	LogLevel             string        `yaml:"log_level,omitempty"`
	ForceShutdownTimeout time.Duration `yaml:"force_shutdown_timeout,omitempty"`
}

// ProcessConfig represents a single process configuration
type ProcessConfig struct {
	ID          string                `yaml:"id"`
	Type        ProcessManagementType `yaml:"type"`              // How the process is managed
	ProfileType string                `yaml:"profile_type"`      // Managed process load/resource profile for restart policies
	Enabled     *bool                 `yaml:"enabled,omitempty"` // Pointer to distinguish unset from false
	Unit        ProcessUnitConfig     `yaml:"unit"`
}

// ProcessManagementType represents how the process is managed
type ProcessManagementType string

const (
	ProcessManagementTypeStandard   ProcessManagementType = "standard_managed"
	ProcessManagementTypeIntegrated ProcessManagementType = "integrated_managed"
	ProcessManagementTypeUnmanaged  ProcessManagementType = "unmanaged"
)

// ProcessProfileType represents the process's load/resource profile for restart policy decisions
type ProcessProfileType string

const (
	ProcessProfileTypeBatch     ProcessProfileType = "batch"     // Batch processing, ETL jobs, ML training
	ProcessProfileTypeWeb       ProcessProfileType = "web"       // HTTP servers, API gateways, frontend services
	ProcessProfileTypeDatabase  ProcessProfileType = "database"  // Database servers, caches, persistent storage
	ProcessProfileTypeWorker    ProcessProfileType = "worker"    // Background job processors, queue workers
	ProcessProfileTypeScheduler ProcessProfileType = "scheduler" // Cron-like schedulers, orchestrators
	ProcessProfileTypeDefault   ProcessProfileType = "default"   // Unknown or generic services
)

// ProcessUnitConfig is a union type that holds configuration for different process types
type ProcessUnitConfig struct {
	// Only one of these should be populated based on ProcessConfig.Type
	StandardManaged   *managedprocess.StandardManagedProcessConfig   `yaml:"standard_managed,omitempty"`
	IntegratedManaged *managedprocess.IntegratedManagedProcessConfig `yaml:"integrated_managed,omitempty"`
	Unmanaged         *managedprocess.UnmanagedProcessConfig         `yaml:"unmanaged,omitempty"`
}

// LoadConfigFromFile loads process manager configuration from a YAML file
func LoadConfigFromFile(filename string) (*ProcessManagerConfig, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.NewIOError("failed to read configuration file", err).WithContext("filename", filename)
	}

	var config ProcessManagerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errors.NewValidationError("failed to parse YAML configuration", err).WithContext("filename", filename)
	}

	// Set defaults
	if err := setConfigDefaults(&config); err != nil {
		return nil, errors.NewValidationError("failed to apply configuration defaults", err)
	}

	return &config, nil
}

// ValidateConfig validates the entire configuration structure
func ValidateConfig(config *ProcessManagerConfig) error {
	if config == nil {
		return errors.NewValidationError("configuration cannot be nil", nil)
	}

	// Validate process manager configuration
	if err := validateProcessManagerConfig(&config.ProcessManager); err != nil {
		return errors.NewValidationError("invalid process manager configuration", err)
	}

	// Validate processes
	if err := validateProcessesConfig(config.ManagedProcesses); err != nil {
		return errors.NewValidationError("invalid managed processes configuration", err)
	}

	return nil
}

// CreateProcessesFromConfig creates process instances from configuration
func CreateProcessesFromConfig(config *ProcessManagerConfig, logger logging.Logger) ([]managedprocess.ProcessDescription, error) {
	if config == nil {
		return nil, errors.NewValidationError("configuration cannot be nil", nil)
	}

	var processes []managedprocess.ProcessDescription

	for i, processConfig := range config.ManagedProcesses {
		// Skip disabled processes (only skip if explicitly set to false)
		if processConfig.Enabled != nil && !*processConfig.Enabled {
			logger.Infof("Skipping disabled process, id: %s", processConfig.ID)
			continue
		}

		process, err := createProcessFromConfig(processConfig, logger)
		if err != nil {
			return nil, errors.NewValidationError(
				fmt.Sprintf("failed to create process at index %d", i),
				err,
			).WithContext("process_id", processConfig.ID).WithContext("process_index", fmt.Sprintf("%d", i))
		}

		processes = append(processes, process)
	}

	return processes, nil
}

// createProcessFromConfig creates a single process from its configuration
func createProcessFromConfig(config ProcessConfig, logger logging.Logger) (managedprocess.ProcessDescription, error) {
	switch config.Type {
	case ProcessManagementTypeStandard:
		if config.Unit.StandardManaged == nil {
			return nil, errors.NewValidationError("standard managed unit configuration is required for standard managed process", nil)
		}
		return managedprocess.NewStandardManagedProcessDescription(config.ID, config.Unit.StandardManaged, logger), nil

	case ProcessManagementTypeUnmanaged:
		if config.Unit.Unmanaged == nil {
			return nil, errors.NewValidationError("unmanaged unit configuration is required for unmanaged process", nil)
		}
		return managedprocess.NewUnmanagedProcessDescription(config.ID, config.Unit.Unmanaged, logger), nil

	case ProcessManagementTypeIntegrated:
		if config.Unit.IntegratedManaged == nil {
			return nil, errors.NewValidationError("integrated managed unit configuration is required for integrated managed process", nil)
		}
		return managedprocess.NewIntegratedManagedProcessDescription(config.ID, config.Unit.IntegratedManaged, logger), nil

	default:
		return nil, errors.NewValidationError(
			fmt.Sprintf("unsupported process management type: %s", config.Type),
			nil,
		).WithContext("supported_types", "managed, unmanaged, integrated")
	}
}

// setConfigDefaults applies default values to configuration
func setConfigDefaults(config *ProcessManagerConfig) error {
	// Set process manager defaults
	if config.ProcessManager.Port == 0 {
		config.ProcessManager.Port = 50055 // Default port
	}
	if config.ProcessManager.LogLevel == "" {
		config.ProcessManager.LogLevel = "info"
	}

	// Set process defaults
	for i := range config.ManagedProcesses {
		process := &config.ManagedProcesses[i]

		// Default enabled to true if not specified
		if process.Enabled == nil {
			enabled := true
			process.Enabled = &enabled
		}

		// Default profile type if not specified
		if process.ProfileType == "" {
			process.ProfileType = string(ProcessProfileTypeDefault)
		}

		// Apply type-specific defaults
		switch process.Type {
		case ProcessManagementTypeStandard:
			if process.Unit.StandardManaged != nil {
				if err := setStandardManagedProcessConfigDefaults(process.Unit.StandardManaged); err != nil {
					return err
				}
			}
		case ProcessManagementTypeUnmanaged:
			if process.Unit.Unmanaged != nil {
				if err := setUnmanagedUnitDefaults(process.Unit.Unmanaged); err != nil {
					return err
				}
			}
		case ProcessManagementTypeIntegrated:
			if process.Unit.IntegratedManaged != nil {
				if err := setIntegratedManagedProcessConfigDefaults(process.Unit.IntegratedManaged); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func setStandardManagedProcessConfigDefaults(config *managedprocess.StandardManagedProcessConfig) error {
	// Set execution defaults
	if config.Control.Execution.WaitDelay == 0 {
		config.Control.Execution.WaitDelay = 10 * time.Second
	}

	// Set context-aware restart defaults
	if config.Control.RestartPolicy == "" {
		config.Control.RestartPolicy = processcontrol.RestartOnFailure
	}

	// Set defaults for context-aware restart configuration
	if config.Control.ContextAwareRestart.Default.MaxRetries == 0 {
		config.Control.ContextAwareRestart.Default.MaxRetries = 3
	}
	if config.Control.ContextAwareRestart.Default.RetryDelay == 0 {
		config.Control.ContextAwareRestart.Default.RetryDelay = 5 * time.Second
	}
	if config.Control.ContextAwareRestart.Default.BackoffRate == 0 {
		config.Control.ContextAwareRestart.Default.BackoffRate = 1.5
	}

	// Set context-specific defaults if not provided
	if config.Control.ContextAwareRestart.HealthFailures == nil {
		config.Control.ContextAwareRestart.HealthFailures = &processcontrol.RestartConfig{
			MaxRetries:  config.Control.ContextAwareRestart.Default.MaxRetries,  // Same as default for health failures
			RetryDelay:  config.Control.ContextAwareRestart.Default.RetryDelay,  // Same as default for health failures
			BackoffRate: config.Control.ContextAwareRestart.Default.BackoffRate, // Same as default for health failures
		}
	}

	if config.Control.ContextAwareRestart.ResourceViolations == nil {
		config.Control.ContextAwareRestart.ResourceViolations = &processcontrol.RestartConfig{
			MaxRetries:  config.Control.ContextAwareRestart.Default.MaxRetries + 2, // More lenient for resource violations
			RetryDelay:  config.Control.ContextAwareRestart.Default.RetryDelay * 2, // Longer delays for resource violations
			BackoffRate: 1.5,                                                       // Gentler backoff for resource violations
		}
	}

	// Set time-based defaults
	if config.Control.ContextAwareRestart.StartupGracePeriod == 0 {
		config.Control.ContextAwareRestart.StartupGracePeriod = 2 * time.Minute
	}
	if config.Control.ContextAwareRestart.SustainedViolationTime == 0 {
		config.Control.ContextAwareRestart.SustainedViolationTime = 5 * time.Minute
	}

	return nil
}

func setUnmanagedUnitDefaults(config *managedprocess.UnmanagedProcessConfig) error {
	// Set discovery defaults
	if config.Discovery.CheckInterval == 0 {
		config.Discovery.CheckInterval = 30 * time.Second
	}

	// Set control defaults
	if config.Control.GracefulTimeout == 0 {
		config.Control.GracefulTimeout = 30 * time.Second
	}

	return nil
}

func setIntegratedManagedProcessConfigDefaults(config *managedprocess.IntegratedManagedProcessConfig) error {
	// Set execution defaults
	if config.Control.Execution.WaitDelay == 0 {
		config.Control.Execution.WaitDelay = 10 * time.Second
	}

	// Set health check defaults
	if config.HealthCheckRunOptions.Interval == 0 {
		config.HealthCheckRunOptions.Interval = 30 * time.Second
	}
	if config.HealthCheckRunOptions.Timeout == 0 {
		config.HealthCheckRunOptions.Timeout = 5 * time.Second
	}

	return nil
}

// Validation functions

func validateProcessManagerConfig(config *ProcessManagerConfigOptions) error {
	if config.Port <= 0 || config.Port > 65535 {
		return errors.NewValidationError(
			fmt.Sprintf("invalid port number: %d", config.Port),
			nil,
		).WithContext("valid_range", "1-65535")
	}

	validLogLevels := []string{"debug", "info", "warn", "error"}
	if config.LogLevel != "" {
		valid := false
		for _, level := range validLogLevels {
			if config.LogLevel == level {
				valid = true
				break
			}
		}
		if !valid {
			return errors.NewValidationError(
				fmt.Sprintf("invalid log level: %s", config.LogLevel),
				nil,
			).WithContext("valid_levels", "debug, info, warn, error")
		}
	}

	return nil
}

func validateProcessesConfig(processes []ProcessConfig) error {
	if len(processes) == 0 {
		return nil // Allow empty processes list
	}

	// Check for duplicate process IDs
	seenIDs := make(map[string]int)
	for i, process := range processes {
		if err := ValidateProcessID(process.ID); err != nil {
			return errors.NewValidationError(
				fmt.Sprintf("invalid process ID at index %d", i),
				err,
			).WithContext("process_id", process.ID)
		}

		if prevIndex, exists := seenIDs[process.ID]; exists {
			return errors.NewValidationError(
				fmt.Sprintf("duplicate process ID '%s' found at indices %d and %d", process.ID, prevIndex, i),
				nil,
			)
		}
		seenIDs[process.ID] = i

		// Validate process management type
		if err := validateProcessManagementType(process.Type); err != nil {
			return errors.NewValidationError(
				fmt.Sprintf("invalid process management type at index %d", i),
				err,
			).WithContext("process_id", process.ID)
		}

		// Validate process profile type
		if err := validateProcessProfileType(process.ProfileType); err != nil {
			return errors.NewValidationError(
				fmt.Sprintf("invalid process profile type at index %d", i),
				err,
			).WithContext("process_id", process.ID)
		}

		// Validate unit configuration matches type
		if err := validateProcessUnitConfig(process.Type, process.Unit); err != nil {
			return errors.NewValidationError(
				fmt.Sprintf("invalid unit configuration for process at index %d", i),
				err,
			).WithContext("process_id", process.ID).WithContext("process_type", string(process.Type))
		}
	}

	return nil
}

func validateProcessManagementType(processType ProcessManagementType) error {
	validTypes := []ProcessManagementType{ProcessManagementTypeStandard, ProcessManagementTypeUnmanaged, ProcessManagementTypeIntegrated}
	for _, validType := range validTypes {
		if processType == validType {
			return nil
		}
	}

	return errors.NewValidationError(
		fmt.Sprintf("unsupported process management type: %s", processType),
		nil,
	).WithContext("supported_types", "managed, unmanaged, integrated")
}

// Validate process profile type
func validateProcessProfileType(profileType string) error {
	if profileType == "" {
		return nil // Will be defaulted
	}

	validTypes := []ProcessProfileType{
		ProcessProfileTypeBatch, ProcessProfileTypeWeb, ProcessProfileTypeDatabase,
		ProcessProfileTypeWorker, ProcessProfileTypeScheduler, ProcessProfileTypeDefault,
	}

	for _, validType := range validTypes {
		if profileType == string(validType) {
			return nil
		}
	}

	return errors.NewValidationError(
		fmt.Sprintf("unsupported process profile type: %s", profileType),
		nil,
	).WithContext("supported_types", "batch, web, database, worker, scheduler, default")
}

func validateProcessUnitConfig(processType ProcessManagementType, unitConfig ProcessUnitConfig) error {
	switch processType {
	case ProcessManagementTypeStandard:
		if unitConfig.StandardManaged == nil {
			return errors.NewValidationError("standard managed unit configuration is required for standard managed process", nil)
		}
		if unitConfig.Unmanaged != nil || unitConfig.IntegratedManaged != nil {
			return errors.NewValidationError("only standard managed unit configuration should be specified for standard managed process", nil)
		}
		return managedprocess.ValidateStandardManagedProcessConfig(*unitConfig.StandardManaged)

	case ProcessManagementTypeUnmanaged:
		if unitConfig.Unmanaged == nil {
			return errors.NewValidationError("unmanaged unit configuration is required for unmanaged process", nil)
		}
		if unitConfig.StandardManaged != nil || unitConfig.IntegratedManaged != nil {
			return errors.NewValidationError("only unmanaged unit configuration should be specified for unmanaged process", nil)
		}
		return managedprocess.ValidateUnmanagedProcessConfig(*unitConfig.Unmanaged)

	case ProcessManagementTypeIntegrated:
		if unitConfig.IntegratedManaged == nil {
			return errors.NewValidationError("integrated managed unit configuration is required for integrated managed process", nil)
		}
		if unitConfig.StandardManaged != nil || unitConfig.Unmanaged != nil {
			return errors.NewValidationError("only integrated managed unit configuration should be specified for integrated managed process", nil)
		}
		return managedprocess.ValidateIntegratedManagedProcessConfig(*unitConfig.IntegratedManaged)

	default:
		return errors.NewValidationError(fmt.Sprintf("unsupported process management type: %s", processType), nil)
	}
}

// CreateProcessesFromConfigWithLogCollection creates processes with log collection support
func CreateProcessesFromConfigWithLogCollection(
	config *ProcessManagerConfig,
	logger logging.Logger,
	logIntegration *LogCollectionIntegration,
) ([]managedprocess.ProcessDescription, error) {
	if config == nil {
		return nil, errors.NewValidationError("configuration cannot be nil", nil)
	}

	var processesResult []managedprocess.ProcessDescription

	for i, processConfig := range config.ManagedProcesses {
		// Skip disabled processes
		if processConfig.Enabled != nil && !*processConfig.Enabled {
			logger.Infof("Skipping disabled process, id: %s", processConfig.ID)
			continue
		}

		// Create process with log collection support
		process, err := createProcessFromConfigWithLogCollection(processConfig, logger, logIntegration)
		if err != nil {
			return nil, errors.NewValidationError(
				fmt.Sprintf("failed to create process at index %d", i),
				err,
			).WithContext("process_id", processConfig.ID).WithContext("process_index", fmt.Sprintf("%d", i))
		}

		processesResult = append(processesResult, process)
	}

	return processesResult, nil
}

// createProcessFromConfigWithLogCollection creates a single process with log collection support
func createProcessFromConfigWithLogCollection(
	config ProcessConfig,
	logger logging.Logger,
	logIntegration *LogCollectionIntegration,
) (managedprocess.ProcessDescription, error) {

	// Create the base process first
	baseProcess, err := createProcessFromConfig(config, logger)
	if err != nil {
		return nil, err
	}

	// If log collection is not enabled, return the base process
	if !logIntegration.IsEnabled() {
		return baseProcess, nil
	}

	// Enhance process with log collection
	enhancedProcess := &logCollectionEnabledProcess{
		ProcessDescription: baseProcess,
		logIntegration:     logIntegration,
		processConfig:      config,
	}

	logger.Infof("Managed process %s created with log collection support", config.ID)
	return enhancedProcess, nil
}

// logCollectionEnabledProcess wraps a process to add log collection capabilities
type logCollectionEnabledProcess struct {
	managedprocess.ProcessDescription
	logIntegration *LogCollectionIntegration
	processConfig  ProcessConfig
}

// ProcessControlOptions enhances the base process's process control options with log collection
func (p *logCollectionEnabledProcess) ProcessControlOptions() processcontrol.ProcessControlOptions {
	// Get base options from the wrapped process
	baseOptions := p.ProcessDescription.ProcessControlOptions()

	// Add log collection service and config
	baseOptions.LogCollectionService = p.logIntegration.GetLogCollectionService()
	baseOptions.LogConfig = p.logIntegration.GetProcessLogConfig(p.ProcessDescription.ID(), p.processConfig)

	// Pass process profile type from configuration
	baseOptions.ProcessProfileType = p.processConfig.ProfileType

	return baseOptions
}
