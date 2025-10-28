package moduleapi

import (
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleproto"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type ModuleHandlersConfig struct {
	ModuleID              moduletypes.ModuleID
	ServerID              moduleproto.ServerID
	Protocol              moduletypes.Protocol
	HandlerRegistrarFuncs map[moduletypes.ServiceID]ProtocolHandlersRegistrarFunc
}

type ModuleHandlersConfigList []ModuleHandlersConfig

type ModuleGatewaysConfigMap map[moduletypes.ModuleID][]ServiceGatewayConfig

type ProtocolHandlersRegistrarFunc func(
	protocolServerRegistrar moduleproto.ProtocolServerRegistrar,
	handler moduletypes.ServiceHandler,
	logger logging.Logger,
)

type ServiceGatewayConfig struct {
	ServiceID          moduletypes.ServiceID
	Protocol           moduletypes.Protocol
	GatewayFactoryFunc ProtocolServiceGatewayFactoryFunc
}

type ProtocolServiceGatewayFactoryFunc func(
	protocolClientConnection moduleproto.ProtocolClientConnection,
	logger logging.Logger,
) moduletypes.ServiceGateway

type RemoteModuleAPI struct {
	ServiceIDs []moduletypes.ServiceID
	ServerPort int
	Protocol   moduletypes.Protocol
}

type RemoteAPI struct {
	ModuleID   moduletypes.ModuleID
	ModuleAPIs []RemoteModuleAPI
}
