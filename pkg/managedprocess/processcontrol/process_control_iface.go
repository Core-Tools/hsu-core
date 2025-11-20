package processcontrol

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/core-tools/hsu-core/pkg/logcollection"
	logconfig "github.com/core-tools/hsu-core/pkg/logcollection/config"
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/resourcelimits"
)

// ProcessControl defines the interface for controlling a process lifecycle
type ProcessControl interface {
	// Start starts the process
	Start(ctx context.Context) error

	// Stop stops the process gracefully
	Stop(ctx context.Context) error

	// Restart restarts the process (stop then start)
	// force: if true, bypasses circuit breaker for immediate restart
	// force: if false, uses circuit breaker safety mechanisms (default/recommended)
	Restart(ctx context.Context, force bool) error

	// GetState returns the current process state
	GetState() ProcessState

	// GetDiagnostics returns detailed process diagnostics including error information
	GetDiagnostics() ProcessDiagnostics
}

type CommandResult struct {
	Process           *os.Process
	Stdout            io.ReadCloser
	HealthCheckConfig *monitoring.HealthCheckConfig
}

type AttachCmd func(ctx context.Context) (*CommandResult, error)

type ExecuteCmd func(ctx context.Context) (*CommandResult, error)

// ProcessControlOptions provides configuration for ProcessControl instances
type ProcessControlOptions struct {
	// Basic control
	CanAttach    bool // Can attach to existing process
	CanTerminate bool // Can send SIGTERM/SIGKILL
	CanRestart   bool // Can restart

	// Process signals
	AllowedSignals []os.Signal // Allowed signals to send

	// Graceful shutdown
	GracefulTimeout time.Duration // Time to wait for graceful shutdown

	// Process start
	ExecuteCmd ExecuteCmd // Execute command, nil if not executable
	AttachCmd  AttachCmd  // Attach command, nil if not attachable

	// Resource management
	Limits *resourcelimits.ResourceLimits // nil if not limitable

	// Process restart configuration (Context-aware restart)
	ContextAwareRestart *ContextAwareRestartConfig // nil if not restartable
	RestartPolicy       RestartPolicy              // Policy for health monitor

	// Managed process profile type for context-aware restart decisions
	ProcessProfileType string // "batch", "web", "database", etc. - managed process's load/resource profile for restart policies

	// Log collection
	LogCollectionService logcollection.LogCollectionService // Log collection service
	LogConfig            *logconfig.ProcessLogConfig        // Log collection configuration for this specific process

	// Health check override
	HealthCheck *monitoring.HealthCheckConfig // nil if not health checkable or if ExecuteCmd/AttachCmd are provided
}
