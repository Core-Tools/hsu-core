package managedprocess

import "github.com/core-tools/hsu-core/pkg/managedprocess/processcontrol"

type Worker interface {
	ID() string
	Metadata() UnitMetadata
	ProcessControlOptions() processcontrol.ProcessControlOptions
}
