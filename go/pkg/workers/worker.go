package workers

import "github.com/core-tools/hsu-core/pkg/workers/processcontrol"

type Worker interface {
	ID() string
	Metadata() UnitMetadata
	ProcessControlOptions() processcontrol.ProcessControlOptions
}
