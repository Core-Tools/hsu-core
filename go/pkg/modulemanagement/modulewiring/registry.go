package modulewiring

import (
	"fmt"
	"sync"

	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleapi"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleproto"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type CreateModuleOptions struct {
	ServiceConnector moduleapi.ServiceConnector
	ServiceProvider  moduletypes.ServiceProvider
	ServiceGateways  []moduletypes.ServiceGateways
	ProtocolServers  []moduleproto.ProtocolServer
	Logger           logging.Logger
}

type ProtocolToServicesMap map[moduletypes.Protocol][]moduletypes.ServiceID

type HandlersRegistrar[SH any] interface {
	RegisterHandlers(handlers SH) (ProtocolToServicesMap, error)
}

type ModuleFactoryFunc func(options CreateModuleOptions) (moduletypes.Module, ProtocolToServicesMap, error)

type CreateServiceProviderOptions struct {
	ServiceConnector moduleapi.ServiceConnector
	Logger           logging.Logger
}

type ServiceProviderFactoryFunc func(options CreateServiceProviderOptions) (moduletypes.ServiceProviderHandle, error)

type DirectClosureEnableOptions[SG any, SH any] struct {
	ServiceConnector moduleapi.ServiceConnector
	ServiceGateways  SG
	ServiceHandlers  SH
}

type TypedServiceProviderFactoryFunc[SP any] func(serviceConnector moduleapi.ServiceConnector, logger logging.Logger) (moduletypes.ServiceProviderHandle, error)
type TypedModuleFactoryFunc[SP any, SH any] func(serviceProvider SP, logger logging.Logger) (moduletypes.Module, SH, error)
type TypedHandlersRegistrarFactoryFunc[SH any] func(protocolServers []moduleproto.ProtocolServer, logger logging.Logger) (HandlersRegistrar[SH], error)
type TypedDirectClosureEnableFunc[SG any, SH any] func(options DirectClosureEnableOptions[SG, SH])

type ModuleDescriptor[SP any, SG any, SH any] struct {
	ServiceProviderFactoryFunc   TypedServiceProviderFactoryFunc[SP]
	ModuleFactoryFunc            TypedModuleFactoryFunc[SP, SH]
	HandlersRegistrarFactoryFunc TypedHandlersRegistrarFactoryFunc[SH]
	DirectClosureEnableFunc      TypedDirectClosureEnableFunc[SG, SH]
}

var (
	globalModuleFactoryRegistry          = make(map[moduletypes.ModuleID]ModuleFactoryFunc)
	globalServiceProviderFactoryRegistry = make(map[moduletypes.ModuleID]ServiceProviderFactoryFunc)
	globalRegistryLock                   sync.RWMutex
)

func RegisterModule[SP any, SG any, SH any](moduleID moduletypes.ModuleID, moduleDesc ModuleDescriptor[SP, SG, SH]) {
	createServiceProvider := func(options CreateServiceProviderOptions) (moduletypes.ServiceProviderHandle, error) {
		return moduleDesc.ServiceProviderFactoryFunc(options.ServiceConnector, options.Logger)
	}

	createModule := func(options CreateModuleOptions) (moduletypes.Module, ProtocolToServicesMap, error) {
		serviceConnector := options.ServiceConnector
		serviceProvider := options.ServiceProvider
		serviceGatewaysArray := options.ServiceGateways
		protocolServers := options.ProtocolServers
		logger := options.Logger

		typedServiceProvider, ok := serviceProvider.(SP)
		if !ok {
			return nil, nil, fmt.Errorf(
				"type mismatch for module '%s': expected service provider implementing %T, got %T",
				moduleID,
				(*SP)(nil),      // Show expected type
				serviceProvider, // Show actual type
			)
		}

		module, typedServiceHandlers, err := moduleDesc.ModuleFactoryFunc(typedServiceProvider, logger)
		if err != nil {
			return nil, nil, err
		}

		var protocolToServicesMap ProtocolToServicesMap
		if moduleDesc.HandlersRegistrarFactoryFunc != nil {
			handlersRegistrar, err := moduleDesc.HandlersRegistrarFactoryFunc(protocolServers, logger)
			if err != nil {
				return nil, nil, err
			}

			protocolToServicesMap, err = handlersRegistrar.RegisterHandlers(typedServiceHandlers)
			if err != nil {
				return nil, nil, err
			}
		}

		if moduleDesc.DirectClosureEnableFunc != nil {
			for _, serviceGateway := range serviceGatewaysArray {
				typedServiceGateway, ok := serviceGateway.(SG)
				if !ok {
					return nil, nil, fmt.Errorf(
						"type mismatch for module '%s': expected service gateways implementing %T, got %T",
						moduleID,
						(*SG)(nil),     // Show expected type
						serviceGateway, // Show actual type
					)
				}

				directClosureEnableOptions := DirectClosureEnableOptions[SG, SH]{
					ServiceConnector: serviceConnector,
					ServiceGateways:  typedServiceGateway,
					ServiceHandlers:  typedServiceHandlers,
				}
				moduleDesc.DirectClosureEnableFunc(directClosureEnableOptions)
			}
		}

		return module, protocolToServicesMap, nil
	}

	globalRegistryLock.Lock()
	defer globalRegistryLock.Unlock()
	globalModuleFactoryRegistry[moduleID] = createModule
	globalServiceProviderFactoryRegistry[moduleID] = createServiceProvider
}

func GetModuleFactory(moduleID moduletypes.ModuleID) (ModuleFactoryFunc, error) {
	globalRegistryLock.RLock()
	defer globalRegistryLock.RUnlock()

	factory, ok := globalModuleFactoryRegistry[moduleID]
	if !ok {
		return nil, fmt.Errorf("unknown module ID: %s", moduleID)
	}

	return factory, nil
}

func CreateModule(moduleID moduletypes.ModuleID, options CreateModuleOptions) (moduletypes.Module, ProtocolToServicesMap, error) {
	factory, err := GetModuleFactory(moduleID)
	if err != nil {
		return nil, nil, err
	}

	return factory(options)
}

func GetServiceProviderFactory(moduleID moduletypes.ModuleID) (ServiceProviderFactoryFunc, error) {
	globalRegistryLock.RLock()
	defer globalRegistryLock.RUnlock()

	factory, ok := globalServiceProviderFactoryRegistry[moduleID]
	if !ok {
		return nil, fmt.Errorf("unknown service provider factory: %s", moduleID)
	}

	return factory, nil
}

func CreateServiceProvider(moduleID moduletypes.ModuleID, options CreateServiceProviderOptions) (moduletypes.ServiceProviderHandle, error) {
	factory, err := GetServiceProviderFactory(moduleID)
	if err != nil {
		return moduletypes.ServiceProviderHandle{}, err
	}

	return factory(options)
}
