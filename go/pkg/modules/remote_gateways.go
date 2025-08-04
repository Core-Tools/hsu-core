package modules

import (
	"github.com/core-tools/hsu-core/pkg/logging"

	"google.golang.org/grpc"
)

type GRPCGatewayFactoryFunc func(grpcClientConnection grpc.ClientConnInterface, logger logging.Logger) interface{}

type GRPCGatewayFactory struct {
	FactoryFunc GRPCGatewayFactoryFunc
	DialOptions []grpc.DialOption
}

type GatewayFactoryUnion struct {
	GRPC *GRPCGatewayFactory
	// TODO: HTTP, etc.
	EnableDirect bool
}

type GatewayFactoryProvider interface {
	ProvideGatewayFactory(moduleID, endpointID string, factory GatewayFactoryUnion) error
}

type GatewayFactoryInfo struct {
	Factory       GatewayFactoryUnion
	DirectClosure interface{}
}

type GatewayFactoryInfoReader interface {
	GetGatewayFactoryInfo(moduleID, endpointID string) (*GatewayFactoryInfo, error)
}
