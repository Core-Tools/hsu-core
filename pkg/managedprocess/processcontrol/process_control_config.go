package processcontrol

import (
	"os"
	"time"

	"github.com/core-tools/hsu-procman-go/pkg/process"
	"github.com/core-tools/hsu-procman-go/pkg/processfile"
	"github.com/core-tools/hsu-procman-go/pkg/resourcelimits"
)

// SystemProcessControlConfig defines configuration for system-managed processes
type SystemProcessControlConfig struct {
	// Basic control
	CanTerminate bool `yaml:"can_terminate,omitempty"` // Can send SIGTERM/SIGKILL
	CanRestart   bool `yaml:"can_restart,omitempty"`   // Can restart (via service manager)

	// Service manager integration
	ServiceManager string `yaml:"service_manager,omitempty"` // "systemd", "windows", "launchd"
	ServiceName    string `yaml:"service_name,omitempty"`    // Service name for restart

	// Process signals
	AllowedSignals []os.Signal `yaml:"allowed_signals,omitempty"` // Allowed signals to send

	// Graceful shutdown
	GracefulTimeout time.Duration `yaml:"graceful_timeout,omitempty"` // Time to wait for graceful shutdown
}

// ManagedProcessControlConfig defines configuration for managed processes
type ManagedProcessControlConfig struct {
	// Process execution
	Execution process.ExecutionConfig `yaml:"execution"`

	// PID file configuration (optional)
	ProcessFile processfile.ProcessFileConfig `yaml:"process_file,omitempty"`

	// Process restart configuration (Context-aware restart replaces simple Restart)
	ContextAwareRestart ContextAwareRestartConfig `yaml:"context_aware_restart"`
	RestartPolicy       RestartPolicy             `yaml:"restart_policy"` // Policy for health monitor

	// Resource management
	Limits resourcelimits.ResourceLimits `yaml:"limits,omitempty"`

	// Graceful shutdown
	GracefulTimeout time.Duration `yaml:"graceful_timeout,omitempty"` // Time to wait for graceful shutdown
}
