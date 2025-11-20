package modulewiring

import (
	"context"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/modulelifecycle"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleproto"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type RuntimeOptions struct {
	Modules       []moduletypes.Module
	ServerManager moduleproto.ServerManager
	Logger        logging.Logger
}

func (o *RuntimeOptions) OptLogger() logging.Logger {
	if o.Logger == nil {
		return logging.NewNullLogger()
	}
	return o.Logger
}

type Runtime interface {
	moduletypes.Lifecycle
}

func NewRuntime(options RuntimeOptions) (Runtime, error) {
	logger := options.OptLogger()

	moduleLifecycleManager := modulelifecycle.NewLifecycleManager(logger)

	for _, module := range options.Modules {
		moduleLifecycleManager.AddLifecycle(module)
	}

	return &runtime{
		moduleLifecycleManager: moduleLifecycleManager,
		serverManager:          options.ServerManager,
		logger:                 logger,
	}, nil
}

type runtime struct {
	serverManager          moduletypes.Lifecycle
	moduleLifecycleManager moduletypes.Lifecycle
	logger                 logging.Logger
}

func (r *runtime) Start(ctx context.Context) error {
	err := r.moduleLifecycleManager.Start(ctx)
	if err != nil {
		return err
	}

	return r.serverManager.Start(ctx)
}

func (r *runtime) Stop(ctx context.Context) error {
	errCollection := errors.NewErrorCollection()

	err := r.serverManager.Stop(ctx)
	errCollection.Add(err)

	err = r.moduleLifecycleManager.Stop(ctx)
	errCollection.Add(err)

	return errCollection.ToError()
}
