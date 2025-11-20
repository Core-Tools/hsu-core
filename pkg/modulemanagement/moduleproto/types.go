package moduleproto

import (
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type ServerID string

type ClientOptionsMap map[moduletypes.Protocol]ProtocolClientOptions

type ServerOptions struct {
	ServerID        ServerID
	Protocol        moduletypes.Protocol
	ProtocolOptions ProtocolServerOptions
}

type ServerOptionsList []ServerOptions
