package processmanager

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/managedprocess"
)

func Run(runDuration int, configFile string, enableLogCollection bool, logger logging.Logger) error {
	logger.Infof("Process manager runner starting...")

	// Log platform information
	logger.Infof("Platform: OS=%s, Arch=%s, CPUs=%d, Go=%s",
		runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())

	// Create separate contexts: one for timeout, one for components
	componentCtx := context.Background()
	operationCtx := componentCtx

	if runDuration > 0 {
		logger.Infof("Using RUN DURATION of %d seconds", runDuration)
		operationCtx, _ = context.WithTimeout(componentCtx, time.Duration(runDuration)*time.Second)
	}

	// Log configuration file
	logger.Infof("Using CONFIGURATION FILE: %s", configFile)

	// Log log collection status
	if enableLogCollection {
		logger.Infof("Log collection is ENABLED - worker logs will be collected!")
	} else {
		logger.Infof("Log collection is DISABLED")
	}

	// Load configuration
	config, err := LoadConfigFromFile(configFile)
	if err != nil {
		return errors.NewIOError("failed to load configuration", err).WithContext("config_file", configFile)
	}

	// Validate configuration
	if err := ValidateConfig(config); err != nil {
		return errors.NewValidationError("configuration validation failed", err).WithContext("config_file", configFile)
	}

	logger.Infof("Configuration loaded successfully from %s", configFile)
	logger.Infof("Process manager port: %d, managed processes: %d", config.ProcessManager.Port, len(config.ManagedProcesses))

	var logIntegration *LogCollectionIntegration
	if enableLogCollection {
		// Create log collection integration
		logIntegration, err = NewLogCollectionIntegration(config.LogCollection, logger)
		if err != nil {
			return errors.NewInternalError("failed to create log collection integration", err)
		}

		// Start log collection service
		if err := logIntegration.Start(componentCtx); err != nil {
			return errors.NewInternalError("failed to start log collection", err)
		}
		defer logIntegration.Stop()

		if logIntegration.IsEnabled() {
			logger.Infof("Log collection service started successfully")

			// Log directory information for debugging
			if pathManager := logIntegration.GetPathManager(); pathManager != nil {
				logDir := pathManager.GenerateLogDirectoryPath()
				logger.Infof("Log collection directory: %s", logDir)

				workerLogDir := pathManager.GenerateWorkerLogDirectoryPath()
				logger.Infof("Worker logs directory: %s", workerLogDir)
			}
		} else {
			logger.Infof("Log collection is disabled")
		}
	}

	// Create process manager options from config
	processManagerOptions := ProcessManagerOptions{
		ForceShutdownTimeout: config.ProcessManager.ForceShutdownTimeout,
	}

	// Create process manager instance
	processManager := NewProcessManager(processManagerOptions, logger)

	var workers []managedprocess.Worker
	if logIntegration != nil {
		// Set log collection service on process manager
		if logIntegration.IsEnabled() {
			processManager.SetLogCollectionService(logIntegration.GetLogCollectionService())
		}

		// Create workers from configuration with log collection support
		workers, err = CreateWorkersFromConfigWithLogCollection(config, logger, logIntegration)
		if err != nil {
			return errors.NewValidationError("failed to create workers from configuration with log collection support", err)
		}
	} else {
		// Create workers from configuration
		workers, err = CreateWorkersFromConfig(config, logger)
		if err != nil {
			return errors.NewValidationError("failed to create workers from configuration", err)
		}
	}

	logger.Infof("Created %d workers", len(workers))

	// Add all workers to process manager (registration phase)
	for _, worker := range workers {
		err := processManager.AddWorker(worker)
		if err != nil {
			return errors.NewValidationError(
				fmt.Sprintf("failed to add worker: %s", worker.ID()),
				err,
			).WithContext("worker_id", worker.ID())
		}
		logger.Infof("Added worker: %s", worker.ID())
	}

	// Start process manager
	processManager.Start(operationCtx)

	logger.Infof("Enabling signal handling...")

	// Enable signal handling
	sig := make(chan os.Signal, 1)
	if runtime.GOOS == "windows" {
		signal.Notify(sig) // Unix signals not implemented on Windows
	} else {
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	}

	logger.Infof("Process manager is ready, starting workers...")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Start all workers (lifecycle phase)
		for _, worker := range workers {
			err := processManager.StartWorker(componentCtx, worker.ID())
			if err != nil {
				logger.Errorf("Failed to start worker %s: %v", worker.ID(), err)
				// Continue with other workers rather than failing completely
				continue
			}
			logger.Infof("Started worker: %s", worker.ID())
		}

		logger.Infof("All workers started, process manager is fully operational")
	}()

	// Wait for graceful shutdown or timeout
	select {
	case receivedSignal := <-sig:
		logger.Infof("Process manager runner received signal: %v", receivedSignal)
		if runtime.GOOS == "windows" {
			if receivedSignal != os.Interrupt {
				logger.Errorf("Wrong signal received: got %q, want %q\n", receivedSignal, os.Interrupt)
				os.Exit(42)
			}
		}
	case <-operationCtx.Done():
		logger.Infof("Process manager runner timed out")
	}

	logger.Infof("Waiting for workers start to finish...")

	// Wait for starting workers to finish
	wg.Wait()

	logger.Infof("Ready to stop process manager...")

	// Stop process manager
	processManager.Stop(context.Background()) // Reset context to background to enable graceful shutdown

	logger.Infof("Process manager runner stopped")

	return nil
}

// ValidateConfigFile validates a configuration file without loading/running
// This is useful for configuration testing and CI/CD validation
func ValidateConfigFile(configFile string) error {
	// Load configuration
	config, err := LoadConfigFromFile(configFile)
	if err != nil {
		return errors.NewIOError("failed to load configuration", err).WithContext("config_file", configFile)
	}

	// Validate configuration
	if err := ValidateConfig(config); err != nil {
		return errors.NewValidationError("configuration validation failed", err).WithContext("config_file", configFile)
	}

	return nil
}

// GetConfigSummary returns a human-readable summary of the configuration
// This is useful for debugging and operational visibility
func GetConfigSummary(config *ProcessManagerConfig) ConfigSummary {
	if config == nil {
		return ConfigSummary{Error: "configuration is nil"}
	}

	summary := ConfigSummary{
		ProcessManagerPort: config.ProcessManager.Port,
		LogLevel:           config.ProcessManager.LogLevel,
		ManagedProcesses:   make([]WorkerSummary, 0, len(config.ManagedProcesses)),
	}

	for _, worker := range config.ManagedProcesses {
		enabled := false
		if worker.Enabled != nil {
			enabled = *worker.Enabled
		}

		workerSummary := WorkerSummary{
			ID:      worker.ID,
			Type:    string(worker.Type),
			Enabled: enabled,
		}

		// Add type-specific information
		switch worker.Type {
		case WorkerManagementTypeManaged:
			if worker.Unit.Managed != nil {
				workerSummary.ExecutablePath = worker.Unit.Managed.Control.Execution.ExecutablePath
				workerSummary.HealthCheckType = string(worker.Unit.Managed.HealthCheck.Type)
			}
		case WorkerManagementTypeUnmanaged:
			if worker.Unit.Unmanaged != nil {
				workerSummary.DiscoveryMethod = string(worker.Unit.Unmanaged.Discovery.Method)
				workerSummary.HealthCheckType = string(worker.Unit.Unmanaged.HealthCheck.Type)
			}
		case WorkerManagementTypeIntegrated:
			if worker.Unit.Integrated != nil {
				workerSummary.ExecutablePath = worker.Unit.Integrated.Control.Execution.ExecutablePath
			}
		}

		summary.ManagedProcesses = append(summary.ManagedProcesses, workerSummary)
	}

	summary.TotalWorkers = len(summary.ManagedProcesses)
	summary.EnabledWorkers = 0
	for _, worker := range summary.ManagedProcesses {
		if worker.Enabled {
			summary.EnabledWorkers++
		}
	}

	return summary
}

// ConfigSummary provides a high-level overview of configuration
type ConfigSummary struct {
	ProcessManagerPort int             `json:"process_manager_port"`
	LogLevel           string          `json:"log_level"`
	TotalWorkers       int             `json:"total_workers"`
	EnabledWorkers     int             `json:"enabled_workers"`
	ManagedProcesses   []WorkerSummary `json:"managed_processes"`
	Error              string          `json:"error,omitempty"`
}

// WorkerSummary provides a summary of worker configuration
type WorkerSummary struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	Enabled         bool   `json:"enabled"`
	ExecutablePath  string `json:"executable_path,omitempty"`
	DiscoveryMethod string `json:"discovery_method,omitempty"`
	HealthCheckType string `json:"health_check_type,omitempty"`
}
