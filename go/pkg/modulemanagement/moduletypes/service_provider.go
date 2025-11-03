package moduletypes

type ServiceProvider interface{}
type ServiceGateways interface{}
type ServiceGatewaysMap map[ModuleID]ServiceGateways

type ServiceProviderHandle struct {
	ServiceProvider    ServiceProvider
	ServiceGatewaysMap ServiceGatewaysMap // Client module gateways for the service provider
}
