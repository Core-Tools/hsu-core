package managedprocess

import (
	"context"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/errors"
	"github.com/core-tools/hsu-procman-go/pkg/logging"
	"github.com/core-tools/hsu-procman-go/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-procman-go/pkg/monitoring"
	"github.com/core-tools/hsu-procman-go/pkg/process"
	"github.com/core-tools/hsu-procman-go/pkg/processfile"
)

type StandardManagedProcessConfig struct {
	// Metadata
	Metadata ProcessMetadata `yaml:"metadata"`

	// Discovery
	// Always use process PID file discovery

	// Process control
	Control processcontrol.ManagedProcessControlConfig `yaml:"control"`

	// Health monitoring
	HealthCheck monitoring.HealthCheckConfig `yaml:"health_check,omitempty"`
}

type standardManagedProcessOptions struct {
	id                   string
	metadata             ProcessMetadata
	processControlConfig processcontrol.ManagedProcessControlConfig
	healthCheckConfig    monitoring.HealthCheckConfig
	logger               logging.Logger
	pidManager           *processfile.ProcessFileManager
}

func NewStandardManagedProcessOptions(id string, processConfig *StandardManagedProcessConfig, logger logging.Logger) ProcessOptions {
	return &standardManagedProcessOptions{
		id:                   id,
		metadata:             processConfig.Metadata,
		processControlConfig: processConfig.Control,
		healthCheckConfig:    processConfig.HealthCheck,
		logger:               logger,
		pidManager:           processfile.NewProcessFileManager(processConfig.Control.ProcessFile, logger),
	}
}

func (pd *standardManagedProcessOptions) ID() string {
	return pd.id
}

func (pd *standardManagedProcessOptions) Metadata() ProcessMetadata {
	return pd.metadata
}

func (pd *standardManagedProcessOptions) ProcessControlOptions() processcontrol.ProcessControlOptions {
	return processcontrol.ProcessControlOptions{
		CanAttach:           true, // Can attach to existing processes as fallback
		CanTerminate:        true, // Can terminate processes
		CanRestart:          true, // Can restart processes
		ExecuteCmd:          pd.ExecuteCmd,
		AttachCmd:           pd.AttachCmd,
		ContextAwareRestart: &pd.processControlConfig.ContextAwareRestart, // ✅ NEW: Full context-aware restart config
		RestartPolicy:       pd.processControlConfig.RestartPolicy,        // ✅ NEW: Policy for health monitor
		Limits:              &pd.processControlConfig.Limits,
		GracefulTimeout:     pd.processControlConfig.GracefulTimeout,
		HealthCheck:         nil, // Provided by ExecuteCmd or AttachCmd
	}
}

func (pd *standardManagedProcessOptions) AttachCmd(ctx context.Context) (*processcontrol.CommandResult, error) {
	pd.logger.Infof("Attaching to standard managed process, id: %s", pd.id)

	pidFile := pd.pidManager.GeneratePIDFilePath(pd.id)

	discovery := process.DiscoveryConfig{
		Method:        process.DiscoveryMethodPIDFile,
		PIDFile:       pidFile,
		CheckInterval: 30 * time.Second,
	}
	stdAttachCmd := process.NewStdAttachCmd(discovery, pd.id, pd.logger)
	process, stdout, err := stdAttachCmd(ctx)
	if err != nil {
		return nil, err
	}

	pd.logger.Infof("Standard managed process attached successfully, id: %s, PID: %d", pd.id, process.Pid)

	return &processcontrol.CommandResult{
		Process:           process,
		Stdout:            stdout,
		HealthCheckConfig: &pd.healthCheckConfig,
	}, nil
}

func (pd *standardManagedProcessOptions) ExecuteCmd(ctx context.Context) (*processcontrol.CommandResult, error) {
	pd.logger.Infof("Executing standard managed process command, id: %s", pd.id)

	// Create the standard execute command
	stdExecuteCmd := process.NewStdExecuteCmd(pd.processControlConfig.Execution, pd.id, pd.logger)
	process, stdout, err := stdExecuteCmd(ctx)
	if err != nil {
		return nil, errors.NewProcessError("failed to execute standard managed process command", err).WithContext("process_id", pd.id)
	}

	// Write PID file
	if err := pd.pidManager.WritePIDFile(pd.id, process.Pid); err != nil {
		// Log error but don't fail - the process is already running
		pd.logger.Errorf("Failed to write PID file for standard managed process %s: %v", pd.id, err)
	} else {
		pidFile := pd.pidManager.GeneratePIDFilePath(pd.id)
		pd.logger.Infof("PID file written for standard managed process %s: %s (PID: %d)", pd.id, pidFile, process.Pid)
	}

	pd.logger.Infof("Standard managed process command executed successfully, id: %s, PID: %d", pd.id, process.Pid)

	return &processcontrol.CommandResult{
		Process:           process,
		Stdout:            stdout,
		HealthCheckConfig: &pd.healthCheckConfig,
	}, nil
}
