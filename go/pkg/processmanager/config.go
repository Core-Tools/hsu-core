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
	ManagedProcesses []WorkerConfig                 `yaml:"managed_processes"`
	LogCollection    *logconfig.LogCollectionConfig `yaml:"log_collection,omitempty"` // Optional log collection configuration
}

// ProcessManagerConfigOptions represents process manager-level configuration
type ProcessManagerConfigOptions struct {
	Port                 int           `yaml:"port"`
	LogLevel             string        `yaml:"log_level,omitempty"`
	ForceShutdownTimeout time.Duration `yaml:"force_shutdown_timeout,omitempty"`
}

// WorkerConfig represents a single worker configuration
type WorkerConfig struct {
	ID          string               `yaml:"id"`
	Type        WorkerManagementType `yaml:"type"`              // How the worker is managed
	ProfileType string               `yaml:"profile_type"`      // Worker load/resource profile for restart policies
	Enabled     *bool                `yaml:"enabled,omitempty"` // Pointer to distinguish unset from false
	Unit        WorkerUnitConfig     `yaml:"unit"`
}

// WorkerManagementType represents how the worker is managed
type WorkerManagementType string

const (
	WorkerManagementTypeManaged    WorkerManagementType = "managed"
	WorkerManagementTypeUnmanaged  WorkerManagementType = "unmanaged"
	WorkerManagementTypeIntegrated WorkerManagementType = "integrated"
)

// WorkerProfileType represents the worker's load/resource profile for restart policy decisions
type WorkerProfileType string

const (
	WorkerProfileTypeBatch     WorkerProfileType = "batch"     // Batch processing, ETL jobs, ML training
	WorkerProfileTypeWeb       WorkerProfileType = "web"       // HTTP servers, API gateways, frontend services
	WorkerProfileTypeDatabase  WorkerProfileType = "database"  // Database servers, caches, persistent storage
	WorkerProfileTypeWorker    WorkerProfileType = "worker"    // Background job processors, queue workers
	WorkerProfileTypeScheduler WorkerProfileType = "scheduler" // Cron-like schedulers, orchestrators
	WorkerProfileTypeDefault   WorkerProfileType = "default"   // Unknown or generic services
)

// WorkerUnitConfig is a union type that holds configuration for different worker types
type WorkerUnitConfig struct {
	// Only one of these should be populated based on WorkerConfig.Type
	Managed    *managedprocess.ManagedUnit    `yaml:"managed,omitempty"`
	Unmanaged  *managedprocess.UnmanagedUnit  `yaml:"unmanaged,omitempty"`
	Integrated *managedprocess.IntegratedUnit `yaml:"integrated,omitempty"`
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

	// Validate workers
	if err := validateWorkersConfig(config.ManagedProcesses); err != nil {
		return errors.NewValidationError("invalid managed processes configuration", err)
	}

	return nil
}

// CreateWorkersFromConfig creates worker instances from configuration
func CreateWorkersFromConfig(config *ProcessManagerConfig, logger logging.Logger) ([]managedprocess.Worker, error) {
	if config == nil {
		return nil, errors.NewValidationError("configuration cannot be nil", nil)
	}

	var workers []managedprocess.Worker

	for i, workerConfig := range config.ManagedProcesses {
		// Skip disabled workers (only skip if explicitly set to false)
		if workerConfig.Enabled != nil && !*workerConfig.Enabled {
			logger.Infof("Skipping disabled worker, id: %s", workerConfig.ID)
			continue
		}

		worker, err := createWorkerFromConfig(workerConfig, logger)
		if err != nil {
			return nil, errors.NewValidationError(
				fmt.Sprintf("failed to create worker at index %d", i),
				err,
			).WithContext("worker_id", workerConfig.ID).WithContext("worker_index", fmt.Sprintf("%d", i))
		}

		workers = append(workers, worker)
	}

	return workers, nil
}

// createWorkerFromConfig creates a single worker from its configuration
func createWorkerFromConfig(config WorkerConfig, logger logging.Logger) (managedprocess.Worker, error) {
	switch config.Type {
	case WorkerManagementTypeManaged:
		if config.Unit.Managed == nil {
			return nil, errors.NewValidationError("managed unit configuration is required for managed worker", nil)
		}
		return managedprocess.NewManagedWorker(config.ID, config.Unit.Managed, logger), nil

	case WorkerManagementTypeUnmanaged:
		if config.Unit.Unmanaged == nil {
			return nil, errors.NewValidationError("unmanaged unit configuration is required for unmanaged worker", nil)
		}
		return managedprocess.NewUnmanagedWorker(config.ID, config.Unit.Unmanaged, logger), nil

	case WorkerManagementTypeIntegrated:
		if config.Unit.Integrated == nil {
			return nil, errors.NewValidationError("integrated unit configuration is required for integrated worker", nil)
		}
		return managedprocess.NewIntegratedWorker(config.ID, config.Unit.Integrated, logger), nil

	default:
		return nil, errors.NewValidationError(
			fmt.Sprintf("unsupported worker management type: %s", config.Type),
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

	// Set worker defaults
	for i := range config.ManagedProcesses {
		worker := &config.ManagedProcesses[i]

		// Default enabled to true if not specified
		if worker.Enabled == nil {
			enabled := true
			worker.Enabled = &enabled
		}

		// ✅ NEW: Default profile type if not specified
		if worker.ProfileType == "" {
			worker.ProfileType = string(WorkerProfileTypeDefault)
		}

		// Apply type-specific defaults
		switch worker.Type {
		case WorkerManagementTypeManaged:
			if worker.Unit.Managed != nil {
				if err := setManagedUnitDefaults(worker.Unit.Managed); err != nil {
					return err
				}
			}
		case WorkerManagementTypeUnmanaged:
			if worker.Unit.Unmanaged != nil {
				if err := setUnmanagedUnitDefaults(worker.Unit.Unmanaged); err != nil {
					return err
				}
			}
		case WorkerManagementTypeIntegrated:
			if worker.Unit.Integrated != nil {
				if err := setIntegratedUnitDefaults(worker.Unit.Integrated); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func setManagedUnitDefaults(config *managedprocess.ManagedUnit) error {
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

func setUnmanagedUnitDefaults(config *managedprocess.UnmanagedUnit) error {
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

func setIntegratedUnitDefaults(config *managedprocess.IntegratedUnit) error {
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

func validateWorkersConfig(workers []WorkerConfig) error {
	if len(workers) == 0 {
		return nil // Allow empty workers list
	}

	// Check for duplicate worker IDs
	seenIDs := make(map[string]int)
	for i, worker := range workers {
		if err := ValidateWorkerID(worker.ID); err != nil {
			return errors.NewValidationError(
				fmt.Sprintf("invalid worker ID at index %d", i),
				err,
			).WithContext("worker_id", worker.ID)
		}

		if prevIndex, exists := seenIDs[worker.ID]; exists {
			return errors.NewValidationError(
				fmt.Sprintf("duplicate worker ID '%s' found at indices %d and %d", worker.ID, prevIndex, i),
				nil,
			)
		}
		seenIDs[worker.ID] = i

		// Validate worker management type
		if err := validateWorkerManagementType(worker.Type); err != nil {
			return errors.NewValidationError(
				fmt.Sprintf("invalid worker management type at index %d", i),
				err,
			).WithContext("worker_id", worker.ID)
		}

		// ✅ NEW: Validate worker profile type
		if err := validateWorkerProfileType(worker.ProfileType); err != nil {
			return errors.NewValidationError(
				fmt.Sprintf("invalid worker profile type at index %d", i),
				err,
			).WithContext("worker_id", worker.ID)
		}

		// Validate unit configuration matches type
		if err := validateWorkerUnitConfig(worker.Type, worker.Unit); err != nil {
			return errors.NewValidationError(
				fmt.Sprintf("invalid unit configuration for worker at index %d", i),
				err,
			).WithContext("worker_id", worker.ID).WithContext("worker_type", string(worker.Type))
		}
	}

	return nil
}

func validateWorkerManagementType(workerType WorkerManagementType) error {
	validTypes := []WorkerManagementType{WorkerManagementTypeManaged, WorkerManagementTypeUnmanaged, WorkerManagementTypeIntegrated}
	for _, validType := range validTypes {
		if workerType == validType {
			return nil
		}
	}

	return errors.NewValidationError(
		fmt.Sprintf("unsupported worker management type: %s", workerType),
		nil,
	).WithContext("supported_types", "managed, unmanaged, integrated")
}

// ✅ NEW: Validate worker profile type
func validateWorkerProfileType(profileType string) error {
	if profileType == "" {
		return nil // Will be defaulted
	}

	validTypes := []WorkerProfileType{
		WorkerProfileTypeBatch, WorkerProfileTypeWeb, WorkerProfileTypeDatabase,
		WorkerProfileTypeWorker, WorkerProfileTypeScheduler, WorkerProfileTypeDefault,
	}

	for _, validType := range validTypes {
		if profileType == string(validType) {
			return nil
		}
	}

	return errors.NewValidationError(
		fmt.Sprintf("unsupported worker profile type: %s", profileType),
		nil,
	).WithContext("supported_types", "batch, web, database, worker, scheduler, default")
}

func validateWorkerUnitConfig(workerType WorkerManagementType, unitConfig WorkerUnitConfig) error {
	switch workerType {
	case WorkerManagementTypeManaged:
		if unitConfig.Managed == nil {
			return errors.NewValidationError("managed unit configuration is required for managed worker", nil)
		}
		if unitConfig.Unmanaged != nil || unitConfig.Integrated != nil {
			return errors.NewValidationError("only managed unit configuration should be specified for managed worker", nil)
		}
		return managedprocess.ValidateManagedUnit(*unitConfig.Managed)

	case WorkerManagementTypeUnmanaged:
		if unitConfig.Unmanaged == nil {
			return errors.NewValidationError("unmanaged unit configuration is required for unmanaged worker", nil)
		}
		if unitConfig.Managed != nil || unitConfig.Integrated != nil {
			return errors.NewValidationError("only unmanaged unit configuration should be specified for unmanaged worker", nil)
		}
		return managedprocess.ValidateUnmanagedUnit(*unitConfig.Unmanaged)

	case WorkerManagementTypeIntegrated:
		if unitConfig.Integrated == nil {
			return errors.NewValidationError("integrated unit configuration is required for integrated worker", nil)
		}
		if unitConfig.Managed != nil || unitConfig.Unmanaged != nil {
			return errors.NewValidationError("only integrated unit configuration should be specified for integrated worker", nil)
		}
		return managedprocess.ValidateIntegratedUnit(*unitConfig.Integrated)

	default:
		return errors.NewValidationError(fmt.Sprintf("unsupported worker management type: %s", workerType), nil)
	}
}

// CreateWorkersFromConfigWithLogCollection creates workers with log collection support
func CreateWorkersFromConfigWithLogCollection(
	config *ProcessManagerConfig,
	logger logging.Logger,
	logIntegration *LogCollectionIntegration,
) ([]managedprocess.Worker, error) {
	if config == nil {
		return nil, errors.NewValidationError("configuration cannot be nil", nil)
	}

	var workersResult []managedprocess.Worker

	for i, workerConfig := range config.ManagedProcesses {
		// Skip disabled workers
		if workerConfig.Enabled != nil && !*workerConfig.Enabled {
			logger.Infof("Skipping disabled worker, id: %s", workerConfig.ID)
			continue
		}

		// Create worker with log collection support
		worker, err := createWorkerFromConfigWithLogCollection(workerConfig, logger, logIntegration)
		if err != nil {
			return nil, errors.NewValidationError(
				fmt.Sprintf("failed to create worker at index %d", i),
				err,
			).WithContext("worker_id", workerConfig.ID).WithContext("worker_index", fmt.Sprintf("%d", i))
		}

		workersResult = append(workersResult, worker)
	}

	return workersResult, nil
}

// createWorkerFromConfigWithLogCollection creates a single worker with log collection support
func createWorkerFromConfigWithLogCollection(
	config WorkerConfig,
	logger logging.Logger,
	logIntegration *LogCollectionIntegration,
) (managedprocess.Worker, error) {

	// Create the base worker first
	baseWorker, err := createWorkerFromConfig(config, logger)
	if err != nil {
		return nil, err
	}

	// If log collection is not enabled, return the base worker
	if !logIntegration.IsEnabled() {
		return baseWorker, nil
	}

	// Enhance worker with log collection
	enhancedWorker := &logCollectionEnabledWorker{
		Worker:         baseWorker,
		logIntegration: logIntegration,
		workerConfig:   config,
	}

	logger.Infof("Worker %s created with log collection support", config.ID)
	return enhancedWorker, nil
}

// logCollectionEnabledWorker wraps a worker to add log collection capabilities
type logCollectionEnabledWorker struct {
	managedprocess.Worker
	logIntegration *LogCollectionIntegration
	workerConfig   WorkerConfig
}

// ProcessControlOptions enhances the base worker's process control options with log collection
func (w *logCollectionEnabledWorker) ProcessControlOptions() processcontrol.ProcessControlOptions {
	// Get base options from the wrapped worker
	baseOptions := w.Worker.ProcessControlOptions()

	// Add log collection service and config
	baseOptions.LogCollectionService = w.logIntegration.GetLogCollectionService()
	baseOptions.LogConfig = w.logIntegration.GetWorkerLogConfig(w.Worker.ID(), w.workerConfig)

	// ✅ NEW: Pass worker profile type from configuration
	baseOptions.WorkerProfileType = w.workerConfig.ProfileType

	return baseOptions
}
