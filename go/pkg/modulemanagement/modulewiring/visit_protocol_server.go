package modulewiring

import (
	"github.com/core-tools/hsu-core/pkg/logging"
	"google.golang.org/grpc"
)

type RegisterHandlerFuncGRPC[Contract any] func(grpcServerRegistrar grpc.ServiceRegistrar, handler Contract, logger logging.Logger) error
type RegisterHandlerFuncHTTP[Contract any] func(httpServerRegistrar any /*TODO: ...*/, handler Contract, logger logging.Logger) error

type RegisterHandlerFuncs[Contract any] struct {
	GRPC RegisterHandlerFuncGRPC[Contract]
	HTTP RegisterHandlerFuncHTTP[Contract]
}

func NewProtocolServerHandlersVisitor[Contract any](handler Contract, registrarFuncs RegisterHandlerFuncs[Contract], logger logging.Logger) *protocolServerHandlersVisitor[Contract] {
	return &protocolServerHandlersVisitor[Contract]{
		handler:        handler,
		registrarFuncs: registrarFuncs,
		logger:         logger,
	}
}

type protocolServerHandlersVisitor[Contract any] struct {
	handler        Contract
	registrarFuncs RegisterHandlerFuncs[Contract]
	logger         logging.Logger
}

func (v *protocolServerHandlersVisitor[Contract]) RegisterHandlersGRPC(grpcServerRegistrar grpc.ServiceRegistrar) error {
	if v.registrarFuncs.GRPC == nil {
		return nil
	}
	return v.registrarFuncs.GRPC(grpcServerRegistrar, v.handler, v.logger)
}

func (v *protocolServerHandlersVisitor[Contract]) RegisterHandlersHTTP(httpServerRegistrar any /*TODO: ...*/) error {
	if v.registrarFuncs.HTTP == nil {
		return nil
	}
	return v.registrarFuncs.HTTP(httpServerRegistrar, v.handler, v.logger)
}
