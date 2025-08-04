package modules

import "context"

type ModulesLifecycle interface {
	Initialize() error
	Start(ctx context.Context, gatewayFactory GatewayFactory) error
	Stop(ctx context.Context) error
}
