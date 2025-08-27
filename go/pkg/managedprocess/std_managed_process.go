package managedprocess

import (
	"context"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"
	"github.com/core-tools/hsu-core/pkg/processfile"
)

type ManagedUnit struct {
	// Metadata
	Metadata ProcessMetadata `yaml:"metadata"`

	// Discovery
	// Always use process PID file discovery

	// Process control
	Control processcontrol.ManagedProcessControlConfig `yaml:"control"`

	// Health monitoring
	HealthCheck monitoring.HealthCheckConfig `yaml:"health_check,omitempty"`
}

type standardManagedProcessDescription struct {
	id                   string
	metadata             ProcessMetadata
	processControlConfig processcontrol.ManagedProcessControlConfig
	healthCheckConfig    monitoring.HealthCheckConfig
	logger               logging.Logger
	pidManager           *processfile.ProcessFileManager
}

func NewStandardManagedProcessDescription(id string, unit *ManagedUnit, logger logging.Logger) ProcessDescription {
	return &standardManagedProcessDescription{
		id:                   id,
		metadata:             unit.Metadata,
		processControlConfig: unit.Control,
		healthCheckConfig:    unit.HealthCheck,
		logger:               logger,
		pidManager:           processfile.NewProcessFileManager(unit.Control.ProcessFile, logger),
	}
}

func (pd *standardManagedProcessDescription) ID() string {
	return pd.id
}

func (pd *standardManagedProcessDescription) Metadata() ProcessMetadata {
	return pd.metadata
}

func (pd *standardManagedProcessDescription) ProcessControlOptions() processcontrol.ProcessControlOptions {
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

func (pd *standardManagedProcessDescription) AttachCmd(ctx context.Context) (*processcontrol.CommandResult, error) {
	pd.logger.Infof("Attaching to managed worker, id: %s", pd.id)

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

	pd.logger.Infof("Managed worker attached successfully, id: %s, PID: %d", pd.id, process.Pid)

	return &processcontrol.CommandResult{
		Process:           process,
		ProcessContext:    nil,
		Stdout:            stdout,
		HealthCheckConfig: &pd.healthCheckConfig,
	}, nil
}

func (pd *standardManagedProcessDescription) ExecuteCmd(ctx context.Context) (*processcontrol.CommandResult, error) {
	pd.logger.Infof("Executing managed worker command, id: %s", pd.id)

	// Create the standard execute command
	stdExecuteCmd := process.NewStdExecuteCmd(pd.processControlConfig.Execution, pd.id, pd.logger)
	process, stdout, err := stdExecuteCmd(ctx)
	if err != nil {
		return nil, errors.NewProcessError("failed to execute managed worker command", err).WithContext("worker_id", pd.id)
	}

	// Write PID file
	if err := pd.pidManager.WritePIDFile(pd.id, process.Pid); err != nil {
		// Log error but don't fail - the process is already running
		pd.logger.Errorf("Failed to write PID file for worker %s: %v", pd.id, err)
	} else {
		pidFile := pd.pidManager.GeneratePIDFilePath(pd.id)
		pd.logger.Infof("PID file written for worker %s: %s (PID: %d)", pd.id, pidFile, process.Pid)
	}

	pd.logger.Infof("Managed worker command executed successfully, id: %s, PID: %d", pd.id, process.Pid)

	return &processcontrol.CommandResult{
		Process:           process,
		ProcessContext:    nil,
		Stdout:            stdout,
		HealthCheckConfig: &pd.healthCheckConfig,
	}, nil
}
