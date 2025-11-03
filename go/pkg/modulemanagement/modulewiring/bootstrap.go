package modulewiring

import (
	"context"
	"fmt"

	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleapi"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleproto"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

func RunWithConfigFile(configFile string, logger logging.Logger) error {
	logger.Infof("Loading config from %s", configFile)

	// 1. Load config
	cfg, err := LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	return RunWithConfig(cfg, logger)
}

func RunWithConfig(cfg *Config, logger logging.Logger) error {
	logger.Infof("Config: %+v", cfg)

	// 1. Create service registry client

	serviceRegistryOptions := moduleapi.ServiceRegistryClientOptions{
		URL:     cfg.Runtime.ServiceRegistry.URL,
		Timeout: cfg.Runtime.ServiceRegistry.Timeout,
		Logger:  logger,
	}
	serviceRegistryClient, err := moduleapi.NewServiceRegistryClient(serviceRegistryOptions)
	if err != nil {
		return fmt.Errorf("failed to create service registry client: %v", err)
	}

	// 2. Create service connector

	serviceConnectorOptions := moduleapi.ServiceConnectorOptions{
		ServiceRegistryClient: serviceRegistryClient,
		ClientOptionsMap:      moduleproto.ClientOptionsMap{}, // TODO: fill with client options
		Logger:                logger,
	}
	serviceConnector := moduleapi.NewServiceConnector(serviceConnectorOptions)

	// 3. Create service providers and build their index maps

	type ServiceProviderMap map[moduletypes.ModuleID]moduletypes.ServiceProvider
	type ServiceGatewaysMap map[moduletypes.ModuleID][]moduletypes.ServiceGateways

	serviceProviderMap := make(ServiceProviderMap)
	serviceGatewaysArrayMap := make(ServiceGatewaysMap)

	for _, moduleCfg := range cfg.Modules {
		if !moduleCfg.Enabled {
			continue
		}

		moduleID := moduleCfg.ID

		serviceProviderOptions := CreateServiceProviderOptions{
			ServiceConnector: serviceConnector,
			Logger:           logger,
		}
		serviceProviderHandle, err := CreateServiceProvider(moduleID, serviceProviderOptions)
		if err != nil {
			return fmt.Errorf("failed to create service provider: %w", err)
		}

		// Build forward map from module ID to service provider
		serviceProviderMap[moduleID] = serviceProviderHandle.ServiceProvider

		// Build reverse map from service gateways module ID to service gateways array
		for serviceGatewaysModuleID, serviceGateways := range serviceProviderHandle.ServiceGatewaysMap {
			serviceGatewaysArray, ok := serviceGatewaysArrayMap[serviceGatewaysModuleID]
			if !ok {
				serviceGatewaysArray = []moduletypes.ServiceGateways{}
			}
			serviceGatewaysArrayMap[serviceGatewaysModuleID] = append(serviceGatewaysArray, serviceGateways)
		}
	}

	// 4. Create server manager and register protocol servers

	serverManager := moduleproto.NewServerManager(logger)

	for _, serverCfg := range cfg.Runtime.Servers {
		if !serverCfg.Enabled {
			continue
		}

		serverID := serverCfg.ID
		protocol := serverCfg.Protocol
		protocolServerOptions := serverCfg.Options

		protocolServer, err := moduleproto.NewProtocolServer(protocol, protocolServerOptions, logger)
		if err != nil {
			return fmt.Errorf("failed to create protocol server: %w", err)
		}

		err = serverManager.RegisterProtocolServer(serverID, protocolServer)
		if err != nil {
			return fmt.Errorf("failed to register protocol server: %w", err)
		}
	}

	// 5. Create modules (with registering their handlers)

	modules := make([]moduletypes.Module, 0)
	remoteAPIs := make([]moduleapi.RemoteAPI, 0)
	for _, moduleCfg := range cfg.Modules {
		if !moduleCfg.Enabled {
			continue
		}

		moduleID := moduleCfg.ID
		moduleServers := moduleCfg.Servers

		// Prepare protocol servers list

		protocolServers := make([]moduleproto.ProtocolServer, 0, len(moduleServers))
		for _, serverID := range moduleServers {
			protocolServer := serverManager.FindProtocolServer(serverID)
			if protocolServer == nil {
				return fmt.Errorf("protocol server not found: %s", serverID)
			}

			protocolServers = append(protocolServers, protocolServer)
		}

		// Create module and register handlers

		moduleOptions := CreateModuleOptions{
			ServiceProvider: serviceProviderMap[moduleID],
			ServiceGateways: serviceGatewaysArrayMap[moduleID],
			ProtocolServers: protocolServers,
			Logger:          logger,
		}

		module, protocolToServicesMap, err := CreateModule(moduleID, moduleOptions)
		if err != nil {
			return fmt.Errorf("failed to create module %s: %w", moduleID, err)
		}

		// Evaluate remote module APIs

		remoteModuleAPIs := make([]moduleapi.RemoteModuleAPI, 0, len(moduleServers))
		for _, protocolServer := range protocolServers {
			remoteModuleAPI := moduleapi.RemoteModuleAPI{
				ServiceIDs: protocolToServicesMap[protocolServer.Protocol()],
				ServerPort: protocolServer.Port(), // use actual port, the protocol server listens on
				Protocol:   protocolServer.Protocol(),
			}
			remoteModuleAPIs = append(remoteModuleAPIs, remoteModuleAPI)
		}
		remoteAPIs = append(remoteAPIs, moduleapi.RemoteAPI{
			ModuleID:   moduleID,
			ModuleAPIs: remoteModuleAPIs,
		})

		modules = append(modules, module)
	}

	// 6. Publish remote APIs to service registry

	err = serviceRegistryClient.PublishAPIs(remoteAPIs)
	if err != nil {
		return fmt.Errorf("failed to publish remote APIs: %v", err)
	}

	// 7. Create runtime

	runtimeOptions := RuntimeOptions{
		Modules:       modules,
		ServerManager: serverManager,
		Logger:        logger,
	}
	runtime, err := NewRuntime(runtimeOptions)
	if err != nil {
		return fmt.Errorf("failed to create runtime: %w", err)
	}

	// 8. Start runtime

	componentCtx := context.Background()
	operationCtx := componentCtx

	err = runtime.Start(operationCtx)
	if err != nil {
		return fmt.Errorf("failed to start runtime: %w", err)
	}

	logger.Infof("Runtime is ready")

	// 9. Wait for signals

	WaitSignals(operationCtx, logger)

	logger.Infof("About to stop runtime...")

	// 10. Stop runtime

	ctx := context.Background()
	err = runtime.Stop(ctx)
	if err != nil {
		return fmt.Errorf("failed to stop runtime: %w", err)
	}

	logger.Infof("Done")
	return nil
}
