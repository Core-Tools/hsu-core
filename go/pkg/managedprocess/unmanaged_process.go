package managedprocess

import (
	"context"

	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"
)

type UnmanagedProcessConfig struct {
	// Metadata
	Metadata ProcessMetadata `yaml:"metadata"`

	// Discovery
	Discovery process.DiscoveryConfig `yaml:"discovery"`

	// Process control
	Control processcontrol.SystemProcessControlConfig `yaml:"control"`

	// Health monitoring
	HealthCheck monitoring.HealthCheckConfig `yaml:"health_check,omitempty"`
}

type unmanagedProcessDescription struct {
	id                   string
	metadata             ProcessMetadata
	discoveryConfig      process.DiscoveryConfig
	processControlConfig processcontrol.SystemProcessControlConfig
	healthCheckConfig    monitoring.HealthCheckConfig
	logger               logging.Logger
}

func NewUnmanagedProcessDescription(id string, unit *UnmanagedProcessConfig, logger logging.Logger) ProcessDescription {
	return &unmanagedProcessDescription{
		id:                   id,
		metadata:             unit.Metadata,
		discoveryConfig:      unit.Discovery,
		processControlConfig: unit.Control,
		healthCheckConfig:    unit.HealthCheck,
		logger:               logger,
	}
}

func (pd *unmanagedProcessDescription) ID() string {
	return pd.id
}

func (pd *unmanagedProcessDescription) Metadata() ProcessMetadata {
	return pd.metadata
}

func (pd *unmanagedProcessDescription) ProcessControlOptions() processcontrol.ProcessControlOptions {
	pd.logger.Debugf("Preparing process control options for unmanaged process, id: %s, discovery: %s, can_terminate: %t, can_restart: %t",
		pd.id, pd.discoveryConfig.Method, pd.processControlConfig.CanTerminate, pd.processControlConfig.CanRestart)

	return processcontrol.ProcessControlOptions{
		CanAttach:           true,                                    // Must attach to existing processes
		CanTerminate:        pd.processControlConfig.CanTerminate,    // Based on system control config
		CanRestart:          pd.processControlConfig.CanRestart,      // Based on system control config
		ExecuteCmd:          nil,                                     // Cannot execute new processes
		AttachCmd:           pd.AttachCmd,                            // Use unit's health check config with logging
		ContextAwareRestart: nil,                                     // No context-aware restart configuration for unmanaged
		RestartPolicy:       processcontrol.RestartNever,             // Unmanaged processes should not auto-restart
		Limits:              nil,                                     // No resource limits for unmanaged
		GracefulTimeout:     pd.processControlConfig.GracefulTimeout, // Use configured graceful timeout
		HealthCheck:         nil,                                     // Provided by AttachCmd
		AllowedSignals:      pd.processControlConfig.AllowedSignals,  // Use configured signal permissions
	}
}

func (pd *unmanagedProcessDescription) AttachCmd(ctx context.Context) (*processcontrol.CommandResult, error) {
	pd.logger.Infof("Attaching to unmanaged process, id: %s", pd.id)

	stdAttachCmd := process.NewStdAttachCmd(pd.discoveryConfig, pd.id, pd.logger)
	process, stdout, err := stdAttachCmd(ctx)
	if err != nil {
		return nil, err
	}

	pd.logger.Infof("Unmanaged process attached successfully, id: %s, PID: %d", pd.id, process.Pid)

	return &processcontrol.CommandResult{
		Process:           process,
		ProcessContext:    nil,
		Stdout:            stdout,
		HealthCheckConfig: &pd.healthCheckConfig,
	}, nil
}
