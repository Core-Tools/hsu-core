package modules

type DirectClosureProvider interface {
	ProvideDirectClosure(moduleID, endpointID string, closure interface{}) error
}
