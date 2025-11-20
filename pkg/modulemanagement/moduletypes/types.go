package moduletypes

type ModuleID string
type ServiceID string

type Protocol string

const (
	ProtocolDirect Protocol = ""
	ProtocolAuto   Protocol = "auto"
	ProtocolGRPC   Protocol = "grpc"
	ProtocolHTTP   Protocol = "http"
)
