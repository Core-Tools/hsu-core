package moduletypes

import "context"

// service == interface, i.e. a set of endpoints (== functions)
// ServiceHandler is an abstraction for a server-side service / interface
// ServiceGateway is an abstraction for a client-side service / interface
// They both should resolve to the same domain interface
type ServiceHandler interface{}
type ServiceGateway interface{}

type ServiceHandlersMap map[ServiceID]ServiceHandler

type ServiceGatewayFactory interface {
	NewServiceGateway(ctx context.Context, moduleID ModuleID, serviceID ServiceID, protocol Protocol) (ServiceGateway, error)
}

type Module interface {
	ID() ModuleID
	ServiceHandlersMap() ServiceHandlersMap
	SetServiceGatewayFactory(factory ServiceGatewayFactory)
	Lifecycle
}
