package modules

import "context"

type GatewayFactory interface {
	NewGateway(ctx context.Context, moduleID, endpointID string) (interface{}, error)
}
