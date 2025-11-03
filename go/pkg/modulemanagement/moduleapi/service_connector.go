package moduleapi

import (
	"context"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleproto"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"

	"google.golang.org/grpc"
)

type GatewayProtocolVisitor interface {
	SelectedProtocolIsDirect() error
	SelectedProtocolIsGRPC(grpcClientConnection *grpc.ClientConn) error
	SelectedProtocolIsHTTP(any /*TODO: ...*/) error
}

type ServiceConnector interface {
	EnableDirectClosure(moduleID moduletypes.ModuleID, serviceIDs []moduletypes.ServiceID)
	Connect(ctx context.Context, moduleID moduletypes.ModuleID, serviceID moduletypes.ServiceID, protocol moduletypes.Protocol, visitor GatewayProtocolVisitor) error
}

type ServiceConnectorOptions struct {
	//LocalModuleHandlers   map[moduletypes.ModuleID]moduletypes.ServiceHandlersMap // local service handlers
	ServiceRegistryClient ServiceRegistryClient // provider of remote API
	ClientOptionsMap      moduleproto.ClientOptionsMap
	Logger                logging.Logger
}

func (o *ServiceConnectorOptions) OptLogger() logging.Logger {
	if o.Logger == nil {
		return logging.NewNullLogger()
	}
	return o.Logger
}

func NewServiceConnector(options ServiceConnectorOptions) ServiceConnector {
	logger := options.OptLogger()

	sc := &serviceConnector{
		//localModuleHandlers:   options.LocalModuleHandlers,
		serviceRegistryClient: options.ServiceRegistryClient,
		clientOptionsMap:      options.ClientOptionsMap,
		logger:                options.Logger,
	}

	logger.Infof("Service gateway factory created")

	return sc
}

type serviceConnector struct {
	directClosureMap      map[moduletypes.ModuleID][]moduletypes.ServiceID // local module handlers
	serviceRegistryClient ServiceRegistryClient                            // provider of remote API
	clientOptionsMap      moduleproto.ClientOptionsMap
	logger                logging.Logger
}

func (sc *serviceConnector) EnableDirectClosure(moduleID moduletypes.ModuleID, serviceIDs []moduletypes.ServiceID) {
	sc.logger.Debugf("Enabling direct closure for module: %s, service IDs: %v", moduleID, serviceIDs)
	sc.directClosureMap[moduleID] = serviceIDs
}

func (sc *serviceConnector) Connect(ctx context.Context, moduleID moduletypes.ModuleID, serviceID moduletypes.ServiceID, protocol moduletypes.Protocol, visitor GatewayProtocolVisitor) error {
	sc.logger.Debugf("Connecting to module: %s, service: %s, protocol: %s", moduleID, serviceID, protocol)

	directClosureMap := sc.directClosureMap[moduleID]
	if directClosureMap == nil {
		directClosureMap = make([]moduletypes.ServiceID, 0)
	}
	hasDirectClosure := false
	for _, directClosureServiceID := range directClosureMap {
		if directClosureServiceID == serviceID {
			hasDirectClosure = true
			break
		}
	}

	switch protocol {
	case moduletypes.ProtocolDirect:
		if !hasDirectClosure {
			return errors.NewNotFoundError("service handler not found", nil).
				WithContext("module_id", moduleID).
				WithContext("service_id", serviceID)
		}
		// have local service handler for the requested module
		return visitor.SelectedProtocolIsDirect()

	case moduletypes.ProtocolAuto:
		var err error
		if hasDirectClosure {
			// have local service handler for the requested module
			err = visitor.SelectedProtocolIsDirect()
			if err == nil {
				return nil
			}
		}

		// try to find the service in the remote module services registry
		return sc.newRemoteServiceConnection(ctx, remoteServiceConnectionRequest{
			ModuleID:  moduleID,
			ServiceID: serviceID,
			Protocol:  protocol,
			Visitor:   visitor,
		})

	case moduletypes.ProtocolGRPC:
		// find the service in the remote module services registry
		return sc.newRemoteServiceConnection(ctx, remoteServiceConnectionRequest{
			ModuleID:  moduleID,
			ServiceID: serviceID,
			Protocol:  protocol,
			Visitor:   visitor,
		})
	}

	return errors.NewValidationError("no valid service connection configuration found", nil).
		WithContext("module_id", moduleID).
		WithContext("service_id", serviceID).
		WithContext("protocol", protocol)
}

type remoteServiceConnectionRequest struct {
	ModuleID  moduletypes.ModuleID
	ServiceID moduletypes.ServiceID
	Protocol  moduletypes.Protocol
	Visitor   GatewayProtocolVisitor
}

func (sc *serviceConnector) newRemoteServiceConnection(ctx context.Context, request remoteServiceConnectionRequest) error {
	// find the module services registration in the remote module services registry
	moduleAPIs, err := sc.serviceRegistryClient.FindModuleAPIs(request.ModuleID)
	if err != nil {
		return errors.NewProcessError("failed to find module APIs", err).
			WithContext("module_id", request.ModuleID).
			WithContext("service_id", request.ServiceID).
			WithContext("protocol", request.Protocol)
	}

	// TODO: cache the module services registration, with a TTL

	var targetProtocol moduletypes.Protocol
	var targetServerPort int
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
			break
		}
	}
	if !foundMathingModuleAPI {
		return errors.NewNotFoundError("matching module API not found", nil).
			WithContext("module_id", request.ModuleID).
			WithContext("service_id", request.ServiceID).
			WithContext("protocol", request.Protocol)
	}

	targetProtocolClientOptions := sc.clientOptionsMap[targetProtocol]

	connection, err := moduleproto.ConnectToProtocolServer(ctx, targetProtocol,
		targetProtocolClientOptions, targetServerPort, sc.logger)
	if err != nil {
		return errors.NewProcessError("failed to connect to protocol server", err).
			WithContext("module_id", request.ModuleID).
			WithContext("service_id", request.ServiceID).
			WithContext("protocol", request.Protocol)
	}

	// TODO: cache connection, close it when appropriate

	switch request.Protocol {
	case moduletypes.ProtocolGRPC:
		return request.Visitor.SelectedProtocolIsGRPC(connection.GatewaysClientConnection().(*grpc.ClientConn))
	case moduletypes.ProtocolHTTP:
		return request.Visitor.SelectedProtocolIsHTTP(connection.GatewaysClientConnection().(any /*TODO: ...*/))
	default:
		return errors.NewValidationError("invalid protocol", nil).
			WithContext("protocol", request.Protocol)
	}
}
