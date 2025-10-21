package moduleapi

import (
	"context"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleproto"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type ServiceGatewayFactoryOptions struct {
	LocalModuleHandlers   map[moduletypes.ModuleID]moduletypes.ServiceHandlersMap // local service handlers
	LocalModuleGateways   ModuleGatewaysConfigMap                                 // local service gateways
	ServiceRegistryClient ServiceRegistryClient                                   // provider of remote API
	ClientOptionsMap      moduleproto.ClientOptionsMap
	Logger                logging.Logger
}

func (o *ServiceGatewayFactoryOptions) OptLogger() logging.Logger {
	if o.Logger == nil {
		return logging.NewNullLogger()
	}
	return o.Logger
}

func NewServiceGatewayFactory(options ServiceGatewayFactoryOptions) moduletypes.ServiceGatewayFactory {
	logger := options.OptLogger()

	gf := &gatewayFactory{
		localModuleHandlers:   options.LocalModuleHandlers,
		localModuleGateways:   options.LocalModuleGateways,
		serviceRegistryClient: options.ServiceRegistryClient,
		clientOptionsMap:      options.ClientOptionsMap,
		logger:                options.Logger,
	}

	logger.Infof("Service gateway factory created")

	return gf
}

type gatewayFactory struct {
	localModuleHandlers   map[moduletypes.ModuleID]moduletypes.ServiceHandlersMap // local module handlers
	localModuleGateways   ModuleGatewaysConfigMap                                 // local module gateways
	serviceRegistryClient ServiceRegistryClient                                   // provider of remote API
	clientOptionsMap      moduleproto.ClientOptionsMap
	logger                logging.Logger
}

func (gf *gatewayFactory) NewServiceGateway(ctx context.Context, moduleID moduletypes.ModuleID, serviceID moduletypes.ServiceID, protocol moduletypes.Protocol) (moduletypes.ServiceGateway, error) {
	gf.logger.Debugf("Creating gateway for module: %s, service: %s, protocol: %s", moduleID, serviceID, protocol)

	localServiceHandlersMap := gf.localModuleHandlers[moduleID]
	if localServiceHandlersMap == nil {
		localServiceHandlersMap = make(moduletypes.ServiceHandlersMap)
	}
	localServiceHandler := localServiceHandlersMap[serviceID]

	switch protocol {
	case moduletypes.ProtocolDirect:
		if localServiceHandler != nil {
			// have local service handler for the requested module
			return localServiceHandler, nil
		}
		return nil, errors.NewNotFoundError("service handler not found", nil).
			WithContext("module_id", moduleID).
			WithContext("service_id", serviceID)

	case moduletypes.ProtocolAuto:
		if localServiceHandler != nil {
			// have local service handler for the requested module
			return localServiceHandler, nil
		}

		// try to find the service in the remote module services registry
		return gf.newRemoteServiceGateway(ctx, remoteServiceGatewayRequest{
			ModuleID:            moduleID,
			ServiceID:           serviceID,
			Protocol:            protocol,
			LocalGatewayConfigs: gf.localModuleGateways[moduleID],
		})

	case moduletypes.ProtocolGRPC:
		// find the service in the remote module services registry
		return gf.newRemoteServiceGateway(ctx, remoteServiceGatewayRequest{
			ModuleID:            moduleID,
			ServiceID:           serviceID,
			Protocol:            protocol,
			LocalGatewayConfigs: gf.localModuleGateways[moduleID],
		})
	}

	return nil, errors.NewValidationError("no valid gateway configuration found", nil).
		WithContext("module_id", moduleID).
		WithContext("service_id", serviceID).
		WithContext("protocol", protocol)
}

type remoteServiceGatewayRequest struct {
	ModuleID            moduletypes.ModuleID
	ServiceID           moduletypes.ServiceID
	Protocol            moduletypes.Protocol
	LocalGatewayConfigs []ServiceGatewayConfig
}

func (gf *gatewayFactory) newRemoteServiceGateway(ctx context.Context, request remoteServiceGatewayRequest) (moduletypes.ServiceGateway, error) {
	// check if there are service gateway config for the requested service and protocol
	haveGatewayConfig := false
	for _, gatewayConfig := range request.LocalGatewayConfigs {
		if request.Protocol != moduletypes.ProtocolAuto && gatewayConfig.Protocol != request.Protocol {
			continue
		}
		if gatewayConfig.ServiceID == request.ServiceID {
			haveGatewayConfig = true
			break
		}
	}
	if !haveGatewayConfig {
		return nil, errors.NewNotFoundError("service gateway config not found", nil).
			WithContext("module_id", request.ModuleID).
			WithContext("service_id", request.ServiceID).
			WithContext("protocol", request.Protocol)
	}

	// find the module services registration in the remote module services registry
	moduleAPIs, err := gf.serviceRegistryClient.FindModuleAPIs(request.ModuleID)
	if err != nil {
		return nil, errors.NewProcessError("failed to find module APIs", err).
			WithContext("module_id", request.ModuleID).
			WithContext("service_id", request.ServiceID).
			WithContext("protocol", request.Protocol)
	}

	// TODO: cache the module services registration, with a TTL

	var targetProtocol moduletypes.Protocol
	var targetServerPort int
	var targetGatewayFactoryFunc ProtocolServiceGatewayFactoryFunc
	foundMathingModuleAPI := true
	for _, moduleAPI := range moduleAPIs {
		// find by matching protocol and service ID from request
		if request.Protocol != moduletypes.ProtocolAuto && moduleAPI.Protocol != request.Protocol {
			continue
		}
		for _, serviceID := range moduleAPI.ServiceIDs {
			if serviceID == request.ServiceID {
				targetProtocol = moduleAPI.Protocol
				targetServerPort = moduleAPI.ServerPort
				foundMathingModuleAPI = true
				break
			}
		}
		if foundMathingModuleAPI {
			// check if the matching gateway config is in the local gateway configs
			foundMatchingGatewayConfig := false
			for _, gatewayConfig := range request.LocalGatewayConfigs {
				if gatewayConfig.Protocol == moduleAPI.Protocol {
					targetGatewayFactoryFunc = gatewayConfig.GatewayFactoryFunc
					foundMatchingGatewayConfig = true
					break
				}
			}
			if !foundMatchingGatewayConfig {
				foundMathingModuleAPI = false
			}
		}
		// the first found matching module API is the one to use
		if foundMathingModuleAPI {
			break
		}
	}
	if !foundMathingModuleAPI {
		return nil, errors.NewNotFoundError("matching module API not found", nil).
			WithContext("module_id", request.ModuleID).
			WithContext("service_id", request.ServiceID).
			WithContext("protocol", request.Protocol)
	}

	targetProtocolClientOptions := gf.clientOptionsMap[targetProtocol]

	connection, err := moduleproto.ConnectToProtocolServer(ctx, targetProtocol,
		targetProtocolClientOptions, targetServerPort, gf.logger)
	if err != nil {
		return nil, errors.NewProcessError("failed to connect to protocol server", err).
			WithContext("module_id", request.ModuleID).
			WithContext("service_id", request.ServiceID).
			WithContext("protocol", request.Protocol)
	}

	// TODO: cache connection, close it if when appropriate

	// Create and return the gateway
	serviceGateway := targetGatewayFactoryFunc(connection.GatewaysClientConnection(), gf.logger)
	if serviceGateway == nil {
		connection.Close() // Clean up connection if factory returns nil
		return nil, errors.NewInternalError("gateway factory function returned nil", nil).
			WithContext("module_id", request.ModuleID).
			WithContext("service_id", request.ServiceID).
			WithContext("protocol", request.Protocol)
	}

	return serviceGateway, nil
}
