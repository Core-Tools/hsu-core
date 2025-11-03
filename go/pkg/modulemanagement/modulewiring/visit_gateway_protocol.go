package modulewiring

import (
	"errors"

	"github.com/core-tools/hsu-core/pkg/logging"
	"google.golang.org/grpc"
)

type GatewayFactoryFuncDirect[Contract any] func() (Contract, error)
type GatewayFactoryFuncGRPC[Contract any] func(grpcClientConnection *grpc.ClientConn, logger logging.Logger) (Contract, error)
type GatewayFactoryFuncHTTP[Contract any] func(httpClientConnection any /* TODO:*/, logger logging.Logger) (Contract, error)

func ErrorIfNil[T any](value T) (T, error) {
	if iface := any(value); iface == nil {
		var zero T
		return zero, errors.New("empty service handler value")
	}
	return value, nil
}

func NewGatewayFactoryFuncDirect[T any](value T) GatewayFactoryFuncDirect[T] {
	return func() (T, error) {
		return ErrorIfNil(value)
	}
}

type GatewayFactoryFuncs[Contract any] struct {
	Direct GatewayFactoryFuncDirect[Contract]
	GRPC   GatewayFactoryFuncGRPC[Contract]
	HTTP   GatewayFactoryFuncHTTP[Contract]
}

type gatewayProtocolVisitor[Contract any] struct {
	factoryFuncs GatewayFactoryFuncs[Contract]
	logger       logging.Logger
	gateway      Contract
}

func NewGatewayProtocolVisitor[Contract any](factoryFuncs GatewayFactoryFuncs[Contract], logger logging.Logger) *gatewayProtocolVisitor[Contract] {
	return &gatewayProtocolVisitor[Contract]{
		factoryFuncs: factoryFuncs,
		logger:       logger,
	}
}

func (v *gatewayProtocolVisitor[Contract]) SelectedProtocolIsDirect() error {
	gateway, err := v.factoryFuncs.Direct()
	if err != nil {
		return err
	}
	v.gateway = gateway
	return nil
}

func (v *gatewayProtocolVisitor[Contract]) SelectedProtocolIsGRPC(grpcClientConnection *grpc.ClientConn) error {
	gateway, err := v.factoryFuncs.GRPC(grpcClientConnection, v.logger)
	if err != nil {
		return err
	}
	v.gateway = gateway
	return nil
}

func (v *gatewayProtocolVisitor[Contract]) SelectedProtocolIsHTTP(httpClientConnection any /*TODO: ...*/) error {
	gateway, err := v.factoryFuncs.HTTP(httpClientConnection, v.logger)
	if err != nil {
		return err
	}
	v.gateway = gateway
	return nil
}

func (v *gatewayProtocolVisitor[Contract]) Gateway() Contract {
	return v.gateway
}
