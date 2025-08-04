package workers

import (
	"github.com/core-tools/hsu-core/pkg/monitoring"
	"github.com/core-tools/hsu-core/pkg/workers/processcontrol"
)

type ManagedUnit struct {
	// Metadata
	Metadata UnitMetadata `yaml:"metadata"`

	// Discovery
	// Always use process PID file discovery

	// Process control
	Control processcontrol.ManagedProcessControlConfig `yaml:"control"`

	// Health monitoring
	HealthCheck monitoring.HealthCheckConfig `yaml:"health_check,omitempty"`
}
