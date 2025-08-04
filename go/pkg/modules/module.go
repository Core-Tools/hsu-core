package modules

import "context"

type Module interface {
	ID() string
	Initialize(directClosureProvider DirectClosureProvider) error
	Start(ctx context.Context, gatewayFactory GatewayFactory) error
	Stop(ctx context.Context) error
}
