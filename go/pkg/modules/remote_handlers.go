package modules

import (
	"github.com/core-tools/hsu-core/pkg/logging"

	"google.golang.org/grpc"
)

type GRPCHandlerRegistratFunc func(grpcServiceRegistrar grpc.ServiceRegistrar, handler interface{}, logger logging.Logger)

type GRPCHandlerRegistrar struct {
	RegistrarFunc GRPCHandlerRegistratFunc
}

type HandlerConfig struct {
	GRPC *GRPCHandlerRegistrar
	// TODO: HTTP, etc.
}

type HandlerRegistrarProvider interface {
	ProvideHandlerRegistrar(moduleID, endpointID string, registrar HandlerConfig) error
}

type HandlerRegistrarInfo struct {
	Registrar     HandlerConfig
	DirectClosure interface{}
}

type HandlerRegistrarInfoReader interface {
	GetAllHandlerRegistrarInfos() map[string]HandlerRegistrarInfo
}
