package moduleapi

import (
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleproto"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type ModuleHandlersConfig struct {
	ModuleID moduletypes.ModuleID
	ServerID moduleproto.ServerID
	Protocol moduletypes.Protocol
}

type ModuleHandlersConfigList []ModuleHandlersConfig

type RemoteModuleAPI struct {
	ServiceIDs []moduletypes.ServiceID
	ServerPort int
	Protocol   moduletypes.Protocol
}

type RemoteAPI struct {
	ModuleID   moduletypes.ModuleID
	ModuleAPIs []RemoteModuleAPI
}
