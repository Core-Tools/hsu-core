package modulelifecycle

import (
	"context"
	"sync"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type Lifecycle moduletypes.Lifecycle

type LifecycleManager interface {
	AddLifecycle(lifecycle Lifecycle)
	Lifecycle
}

type lifecycleManager struct {
	lifecycles []Lifecycle
	logger     logging.Logger
	mx         sync.Mutex
}

func NewLifecycleManager(logger logging.Logger) LifecycleManager {
	return &lifecycleManager{
		lifecycles: make([]Lifecycle, 0),
		logger:     logger,
	}
}

func (m *lifecycleManager) AddLifecycle(lifecycle Lifecycle) {
	if lifecycle == nil {
		return
	}

	m.mx.Lock()
	defer m.mx.Unlock()

	m.lifecycles = append(m.lifecycles, lifecycle)
}

func (m *lifecycleManager) getLifecycles() []Lifecycle {
	m.mx.Lock()
	defer m.mx.Unlock()

	lifecycles := make([]Lifecycle, 0)
	for _, lifecycle := range m.lifecycles {
		lifecycles = append(lifecycles, lifecycle)
	}

	return lifecycles
}

func (m *lifecycleManager) Start(ctx context.Context) error {
	lifecycles := m.getLifecycles()

	errorCollection := errors.NewErrorCollection()

	for _, lifecycle := range lifecycles {
		err := lifecycle.Start(ctx)
		errorCollection.Add(err)
	}

	return errorCollection.ToError()
}

func (m *lifecycleManager) Stop(ctx context.Context) error {
	lifecycles := m.getLifecycles()

	errorCollection := errors.NewErrorCollection()

	for _, lifecycle := range lifecycles {
		err := lifecycle.Stop(ctx)
		errorCollection.Add(err)
	}

	return errorCollection.ToError()
}
