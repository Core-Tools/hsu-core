package moduletypes

type EmptyServiceHandlers struct{}
type EmptyServiceGateways struct{}

type Module interface {
	Lifecycle
}
