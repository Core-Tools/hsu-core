package modulemanagement

import (
	"context"
	"fmt"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleapi"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/modulelifecycle"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleproto"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type RuntimeOptions struct {
	Modules               []moduletypes.Module
	ServerOptions         moduleproto.ServerOptionsList
	ClientOptions         moduleproto.ClientOptionsMap
	ModuleHandlersConfigs moduleapi.ModuleHandlersConfigList
	ModuleGatewaysConfigs moduleapi.ModuleGatewaysConfigMap
	ServerRegistryURL     string
	Logger                logging.Logger
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

	// 1. Gather module API handlers to local API map

	modules := make([]moduletypes.Module, 0)

	localModuleHandlers := make(map[moduletypes.ModuleID]moduletypes.ServiceHandlersMap)

	for _, module := range options.Modules {
		if module == nil {
			return nil, fmt.Errorf("module is nil")
		}

		moduleID := module.ID()

		_, ok := localModuleHandlers[moduleID]
		if ok {
			return nil, fmt.Errorf("duplicate module ID: %v", moduleID)
		}

		localModuleHandlers[moduleID] = module.ServiceHandlersMap()

		modules = append(modules, module)
	}

	// 2. Create service manager and protocol servers

	serverManager := moduleproto.NewServerManager(logger)

	for _, serverOptions := range options.ServerOptions {
		serverID := serverOptions.ServerID
		protocol := serverOptions.Protocol
		protocolServerOptions := serverOptions.ProtocolOptions

		protocolServer, err := moduleproto.NewProtocolServer(protocol, protocolServerOptions, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create protocol server: %w", err)
		}

		err = serverManager.RegisterProtocolServer(serverID, protocolServer)
		if err != nil {
			return nil, fmt.Errorf("failed to register protocol server: %w", err)
		}
	}

	// 3. Register module handlers and gather remote module APIs

	remoteModuleAPIsMap := make(map[moduletypes.ModuleID][]moduleapi.RemoteModuleAPI)
	for _, handlersConfig := range options.ModuleHandlersConfigs {
		protocolServer := serverManager.FindProtocolServer(handlersConfig.ServerID)
		if protocolServer == nil {
			return nil, fmt.Errorf("protocol server not found: %s", handlersConfig.ServerID)
		}
		if handlersConfig.Protocol != protocolServer.Protocol() {
			return nil, fmt.Errorf("protocol mismatch: %s != %s", handlersConfig.Protocol, protocolServer.Protocol())
		}
		localServiceHandlersMap, ok := localModuleHandlers[handlersConfig.ModuleID]
		if !ok {
			return nil, fmt.Errorf("module router not found: %s", handlersConfig.ModuleID)
		}

		serviceIDs := make([]moduletypes.ServiceID, 0, len(localServiceHandlersMap))
		for serviceID := range localServiceHandlersMap {
			serviceIDs = append(serviceIDs, serviceID)
		}
		handlersConfig.HandlersRegistrarFunc(localServiceHandlersMap, protocolServer.HandlersRegistrar(), logger)

		remoteModuleAPI := moduleapi.RemoteModuleAPI{
			ServiceIDs: serviceIDs,
			ServerPort: protocolServer.Port(), // use actual port, the protocol server listens on
			Protocol:   protocolServer.Protocol(),
		}
		remoteModuleAPIs, ok := remoteModuleAPIsMap[handlersConfig.ModuleID]
		if !ok {
			remoteModuleAPIs = make([]moduleapi.RemoteModuleAPI, 0)
		}
		remoteModuleAPIs = append(remoteModuleAPIs, remoteModuleAPI)
		remoteModuleAPIsMap[handlersConfig.ModuleID] = remoteModuleAPIs
	}

	// 4. Create service registry client and service gateway factory

	serviceRegistryClient, err := moduleapi.NewServiceRegistryClient(options.ServerRegistryURL, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create service registry client: %v", err)
	}

	serviceGatewayFactory := moduleapi.NewServiceGatewayFactory(moduleapi.ServiceGatewayFactoryOptions{
		LocalModuleHandlers:   localModuleHandlers,
		LocalModuleGateways:   options.ModuleGatewaysConfigs,
		ServiceRegistryClient: serviceRegistryClient,
		ClientOptionsMap:      options.ClientOptions,
		Logger:                logger,
	})

	// 5. Publish remote module APIs to service registry

	remoteAPIs := make([]moduleapi.RemoteAPI, 0)
	for moduleID, remoteModuleAPIs := range remoteModuleAPIsMap {
		remoteAPIs = append(remoteAPIs, moduleapi.RemoteAPI{
			ModuleID:   moduleID,
			ModuleAPIs: remoteModuleAPIs,
		})
	}

	err = serviceRegistryClient.PublishAPIs(remoteAPIs)
	if err != nil {
		return nil, fmt.Errorf("failed to publish remote APIs: %v", err)
	}

	// 6. Create module lifecycle manager, add modules to it and set service gateway factory

	moduleLifecycleManager := modulelifecycle.NewLifecycleManager(logger)

	for _, module := range modules {
		module.SetServiceGatewayFactory(serviceGatewayFactory)
		moduleLifecycleManager.AddLifecycle(module)
	}

	return &runtime{
		moduleLifecycleManager: moduleLifecycleManager,
		serverManager:          serverManager,
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
