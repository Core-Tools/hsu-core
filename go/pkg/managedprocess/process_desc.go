package managedprocess

import "github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"

type ProcessMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

type ProcessDescription interface {
	ID() string
	Metadata() ProcessMetadata
	ProcessControlOptions() processcontrol.ProcessControlOptions
}
