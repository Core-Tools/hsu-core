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

type managedWorker struct {
	id                   string
	metadata             UnitMetadata
	processControlConfig processcontrol.ManagedProcessControlConfig
	healthCheckConfig    monitoring.HealthCheckConfig
	logger               logging.Logger
	pidManager           *processfile.ProcessFileManager
}

func NewManagedWorker(id string, unit *ManagedUnit, logger logging.Logger) Worker {
	return &managedWorker{
		id:                   id,
		metadata:             unit.Metadata,
		processControlConfig: unit.Control,
		healthCheckConfig:    unit.HealthCheck,
		logger:               logger,
		pidManager:           processfile.NewProcessFileManager(unit.Control.ProcessFile, logger),
	}
}

func (w *managedWorker) ID() string {
	return w.id
}

func (w *managedWorker) Metadata() UnitMetadata {
	return w.metadata
}

func (w *managedWorker) ProcessControlOptions() processcontrol.ProcessControlOptions {
	return processcontrol.ProcessControlOptions{
		CanAttach:           true, // Can attach to existing processes as fallback
		CanTerminate:        true, // Can terminate processes
		CanRestart:          true, // Can restart processes
		ExecuteCmd:          w.ExecuteCmd,
		AttachCmd:           w.AttachCmd,
		ContextAwareRestart: &w.processControlConfig.ContextAwareRestart, // ✅ NEW: Full context-aware restart config
		RestartPolicy:       w.processControlConfig.RestartPolicy,        // ✅ NEW: Policy for health monitor
		Limits:              &w.processControlConfig.Limits,
		GracefulTimeout:     w.processControlConfig.GracefulTimeout,
		HealthCheck:         nil, // Provided by ExecuteCmd or AttachCmd
	}
}

func (w *managedWorker) AttachCmd(ctx context.Context) (*processcontrol.CommandResult, error) {
	w.logger.Infof("Attaching to managed worker, id: %s", w.id)

	pidFile := w.pidManager.GeneratePIDFilePath(w.id)

	discovery := process.DiscoveryConfig{
		Method:        process.DiscoveryMethodPIDFile,
		PIDFile:       pidFile,
		CheckInterval: 30 * time.Second,
	}
	stdAttachCmd := process.NewStdAttachCmd(discovery, w.id, w.logger)
	process, stdout, err := stdAttachCmd(ctx)
	if err != nil {
		return nil, err
	}

	w.logger.Infof("Managed worker attached successfully, id: %s, PID: %d", w.id, process.Pid)

	return &processcontrol.CommandResult{
		Process:           process,
		ProcessContext:    nil,
		Stdout:            stdout,
		HealthCheckConfig: &w.healthCheckConfig,
	}, nil
}

func (w *managedWorker) ExecuteCmd(ctx context.Context) (*processcontrol.CommandResult, error) {
	w.logger.Infof("Executing managed worker command, id: %s", w.id)

	// Create the standard execute command
	stdExecuteCmd := process.NewStdExecuteCmd(w.processControlConfig.Execution, w.id, w.logger)
	process, stdout, err := stdExecuteCmd(ctx)
	if err != nil {
		return nil, errors.NewProcessError("failed to execute managed worker command", err).WithContext("worker_id", w.id)
	}

	// Write PID file
	if err := w.pidManager.WritePIDFile(w.id, process.Pid); err != nil {
		// Log error but don't fail - the process is already running
		w.logger.Errorf("Failed to write PID file for worker %s: %v", w.id, err)
	} else {
		pidFile := w.pidManager.GeneratePIDFilePath(w.id)
		w.logger.Infof("PID file written for worker %s: %s (PID: %d)", w.id, pidFile, process.Pid)
	}

	w.logger.Infof("Managed worker command executed successfully, id: %s, PID: %d", w.id, process.Pid)

	return &processcontrol.CommandResult{
		Process:           process,
		ProcessContext:    nil,
		Stdout:            stdout,
		HealthCheckConfig: &w.healthCheckConfig,
	}, nil
}
