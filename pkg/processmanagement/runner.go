package processmanagement

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/errors"
	"github.com/core-tools/hsu-procman-go/pkg/logging"
	"github.com/core-tools/hsu-procman-go/pkg/managedprocess"
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
		logger.Infof("Log collection is ENABLED - process logs will be collected!")
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

				processLogDir := pathManager.GenerateProcessLogDirectoryPath()
				logger.Infof("Managed process logs directory: %s", processLogDir)
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

	var processes []managedprocess.ProcessOptions
	if logIntegration != nil {
		// Set log collection service on process manager
		if logIntegration.IsEnabled() {
			processManager.SetLogCollectionService(logIntegration.GetLogCollectionService())
		}

		// Create processes from configuration with log collection support
		processes, err = CreateProcessesFromConfigWithLogCollection(config, logger, logIntegration)
		if err != nil {
			return errors.NewValidationError("failed to create processes from configuration with log collection support", err)
		}
	} else {
		// Create processes from configuration
		processes, err = CreateProcessesFromConfig(config, logger)
		if err != nil {
			return errors.NewValidationError("failed to create processes from configuration", err)
		}
	}

	logger.Infof("Created %d processes", len(processes))

	// Add all processes to process manager (registration phase)
	for _, process := range processes {
		err := processManager.AddProcess(process)
		if err != nil {
			return errors.NewValidationError(
				fmt.Sprintf("failed to add process: %s", process.ID()),
				err,
			).WithContext("process_id", process.ID())
		}
		logger.Infof("Added process: %s", process.ID())
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

	logger.Infof("Process manager is ready, starting processes...")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Start all processes (lifecycle phase)
		for _, process := range processes {
			err := processManager.StartProcess(componentCtx, process.ID())
			if err != nil {
				logger.Errorf("Failed to start process %s: %v", process.ID(), err)
				// Continue with other processes rather than failing completely
				continue
			}
			logger.Infof("Started process: %s", process.ID())
		}

		logger.Infof("All processes started, process manager is fully operational")
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

	logger.Infof("Waiting for processes start to finish...")

	// Wait for starting processes to finish
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
		ManagedProcesses:   make([]ProcessSummary, 0, len(config.ManagedProcesses)),
	}

	for _, process := range config.ManagedProcesses {
		enabled := false
		if process.Enabled != nil {
			enabled = *process.Enabled
		}

		processSummary := ProcessSummary{
			ID:      process.ID,
			Type:    string(process.Type),
			Enabled: enabled,
		}

		// Add type-specific information
		switch process.Type {
		case ProcessManagementTypeStandard:
			if process.Management.StandardManaged != nil {
				processSummary.ExecutablePath = process.Management.StandardManaged.Control.Execution.ExecutablePath
				processSummary.HealthCheckType = string(process.Management.StandardManaged.HealthCheck.Type)
			}
		case ProcessManagementTypeIntegrated:
			if process.Management.IntegratedManaged != nil {
				processSummary.ExecutablePath = process.Management.IntegratedManaged.Control.Execution.ExecutablePath
			}
		case ProcessManagementTypeUnmanaged:
			if process.Management.Unmanaged != nil {
				processSummary.DiscoveryMethod = string(process.Management.Unmanaged.Discovery.Method)
				processSummary.HealthCheckType = string(process.Management.Unmanaged.HealthCheck.Type)
			}
		}

		summary.ManagedProcesses = append(summary.ManagedProcesses, processSummary)
	}

	summary.TotalProcesses = len(summary.ManagedProcesses)
	summary.EnabledProcesses = 0
	for _, process := range summary.ManagedProcesses {
		if process.Enabled {
			summary.EnabledProcesses++
		}
	}

	return summary
}

// ConfigSummary provides a high-level overview of configuration
type ConfigSummary struct {
	ProcessManagerPort int              `json:"process_manager_port"`
	LogLevel           string           `json:"log_level"`
	TotalProcesses     int              `json:"total_processes"`
	EnabledProcesses   int              `json:"enabled_processes"`
	ManagedProcesses   []ProcessSummary `json:"managed_processes"`
	Error              string           `json:"error,omitempty"`
}

// ProcessSummary provides a summary of process configuration
type ProcessSummary struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	Enabled         bool   `json:"enabled"`
	ExecutablePath  string `json:"executable_path,omitempty"`
	DiscoveryMethod string `json:"discovery_method,omitempty"`
	HealthCheckType string `json:"health_check_type,omitempty"`
}
