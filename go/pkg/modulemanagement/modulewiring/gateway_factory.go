package modulewiring

import (
	"context"

	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleapi"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

type ServiceGatewayFactory[Contract any] struct {
	ModuleID            moduletypes.ModuleID
	ServiceID           moduletypes.ServiceID
	ServiceConnector    moduleapi.ServiceConnector
	GatewayFactoryFuncs GatewayFactoryFuncs[Contract]
	Logger              logging.Logger
}

func (f *ServiceGatewayFactory[Contract]) NewServiceGateway(ctx context.Context, protocol moduletypes.Protocol) (Contract, error) {
	gatewayProtocolVisitor := NewGatewayProtocolVisitor(f.GatewayFactoryFuncs, f.Logger)
	err := f.ServiceConnector.Connect(ctx, f.ModuleID, f.ServiceID, protocol, gatewayProtocolVisitor)
	if err != nil {
		return *new(Contract), err
	}
	return gatewayProtocolVisitor.Gateway(), nil
}
