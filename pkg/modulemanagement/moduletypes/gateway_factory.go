package moduletypes

import (
	"context"
)

type ServiceGatewayFactory[Contract any] interface {
	NewServiceGateway(ctx context.Context, protocol Protocol) (Contract, error)
}
