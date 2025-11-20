package managedprocess

import "github.com/core-tools/hsu-procman-go/pkg/managedprocess/processcontrol"

type ProcessMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

type ProcessOptions interface {
	ID() string
	Metadata() ProcessMetadata
	ProcessControlOptions() processcontrol.ProcessControlOptions
}
