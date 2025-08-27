package managedprocess

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/process"
	"github.com/core-tools/hsu-core/pkg/processfile"
)

type IntegratedManagedProcessConfig struct {
	// Metadata
	Metadata ProcessMetadata `yaml:"metadata"`

	// Discovery
	// Always use process PID file discovery

	// Process control
	Control processcontrol.ManagedProcessControlConfig `yaml:"control"`

	// Health monitoring
	HealthCheckRunOptions monitoring.HealthCheckRunOptions `yaml:"health_check_run_options,omitempty"`
}

type integratedManagedProcessDescription struct {
	id                    string
	metadata              ProcessMetadata
	processControlConfig  processcontrol.ManagedProcessControlConfig
	healthCheckRunOptions monitoring.HealthCheckRunOptions
	logger                logging.Logger
	pidManager            *processfile.ProcessFileManager
}

func NewIntegratedManagedProcessDescription(id string, config *IntegratedManagedProcessConfig, logger logging.Logger) ProcessDescription {
	return &integratedManagedProcessDescription{
		id:                    id,
		metadata:              config.Metadata,
		processControlConfig:  config.Control,
		healthCheckRunOptions: config.HealthCheckRunOptions,
		logger:                logger,
		pidManager:            processfile.NewProcessFileManager(config.Control.ProcessFile, logger),
	}
}

func (w *integratedManagedProcessDescription) ID() string {
	return w.id
}

func (w *integratedManagedProcessDescription) Metadata() ProcessMetadata {
	return w.metadata
}

func (w *integratedManagedProcessDescription) ProcessControlOptions() processcontrol.ProcessControlOptions {
	return processcontrol.ProcessControlOptions{
		CanAttach:           true,
		CanTerminate:        true,
		CanRestart:          true,
		ExecuteCmd:          w.ExecuteCmd,
		AttachCmd:           w.AttachCmd,                                 // Use custom AttachCmd that creates dynamic gRPC health check
		ContextAwareRestart: &w.processControlConfig.ContextAwareRestart, // ✅ NEW: Full context-aware restart config
		RestartPolicy:       w.processControlConfig.RestartPolicy,        // ✅ NEW: Policy for health monitor
		Limits:              &w.processControlConfig.Limits,
		GracefulTimeout:     w.processControlConfig.GracefulTimeout,
		HealthCheck:         nil, // Provided by ExecuteCmd or AttachCmd
	}
}

// AttachCmd creates a dynamic gRPC health check configuration by reading the port file
func (w *integratedManagedProcessDescription) AttachCmd(ctx context.Context) (*processcontrol.CommandResult, error) {
	w.logger.Infof("Attaching to integrated worker, id: %s", w.id)

	pidFile := w.pidManager.GeneratePIDFilePath(w.id)

	// Use standard attachment to discover the process
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

	// Read the port from the port file
	port, err := w.pidManager.ReadPortFile(w.id)
	if err != nil {
		w.logger.Warnf("Failed to read port file for worker %s: %v, using default port 50051", w.id, err)
		port = 50051 // Default fallback port
	}

	portStr := fmt.Sprintf("%d", port)
	address := "localhost:" + portStr

	healthCheckConfig := w.newDynamicHealthCheckConfig(address, process.Pid)

	processContext := map[string]string{
		"server_address": address,
	}

	w.logger.Infof("Integrated worker attached successfully, id: %s, PID: %d", w.id, process.Pid)

	return &processcontrol.CommandResult{
		Process:           process,
		ProcessContext:    processContext,
		Stdout:            stdout,
		HealthCheckConfig: healthCheckConfig,
	}, nil
}

func (w *integratedManagedProcessDescription) ExecuteCmd(ctx context.Context) (*processcontrol.CommandResult, error) {
	w.logger.Infof("Executing integrated worker command, id: %s", w.id)

	// Get a free port for the gRPC server
	port, err := getFreePort()
	if err != nil {
		return nil, errors.NewNetworkError("failed to get free port", err)
	}
	portStr := fmt.Sprintf("%d", port)

	w.logger.Infof("Got free port, port: %d", port)

	executionConfig := w.processControlConfig.Execution
	executionConfig.Args = append(executionConfig.Args, "--port", portStr)

	stdExecuteCmd := process.NewStdExecuteCmd(executionConfig, w.id, w.logger)
	process, stdout, err := stdExecuteCmd(ctx)
	if err != nil {
		return nil, errors.NewProcessError("failed to execute command", err).WithContext("port", port)
	}

	// Write PID file
	if err := w.pidManager.WritePIDFile(w.id, process.Pid); err != nil {
		// Log error but don't fail - the process is already running
		w.logger.Errorf("Failed to write PID file for worker %s: %v", w.id, err)
	} else {
		pidFile := w.pidManager.GeneratePIDFilePath(w.id)
		w.logger.Infof("PID file written for worker %s: %s (PID: %d)", w.id, pidFile, process.Pid)
	}

	// Write port file
	if err := w.pidManager.WritePortFile(w.id, port); err != nil {
		// Log error but don't fail - the process is already running
		w.logger.Errorf("Failed to write port file for worker %s: %v", w.id, err)
	} else {
		portFile := w.pidManager.GeneratePortFilePath(w.id)
		w.logger.Infof("Port file written for worker %s: %s (port: %d)", w.id, portFile, port)
	}

	address := "localhost:" + portStr

	healthCheckConfig := w.newDynamicHealthCheckConfig(address, process.Pid)

	processContext := map[string]string{
		"server_address": address,
	}

	w.logger.Infof("Integrated worker executed successfully, id: %s, PID: %d", w.id, process.Pid)

	return &processcontrol.CommandResult{
		Process:           process,
		ProcessContext:    processContext,
		Stdout:            stdout,
		HealthCheckConfig: healthCheckConfig,
	}, nil
}

func (w *integratedManagedProcessDescription) newDynamicHealthCheckConfig(address string, pid int) *monitoring.HealthCheckConfig {
	healthCheckConfig := &monitoring.HealthCheckConfig{
		Type: monitoring.HealthCheckTypeGRPC,
		GRPC: monitoring.GRPCHealthCheckConfig{
			Address: address,
			Service: "CoreService",
			Method:  "Ping",
		},
		RunOptions: w.healthCheckRunOptions,
	}

	w.logger.Infof("Created dynamic gRPC health check for executed process %d: %v", pid, healthCheckConfig)

	return healthCheckConfig
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, errors.NewNetworkError("failed to resolve TCP address", err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, errors.NewNetworkError("failed to listen on TCP address", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
