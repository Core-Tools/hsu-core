package moduleproto

import (
	"context"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

// ProtocolClientConnection is an abstraction for a protocol client connection interface
// E.g. grpcClientConnection grpc.ClientConnInterface
type ProtocolClientConnection interface{}

type ClientConnection interface {
	GatewaysClientConnection() ProtocolClientConnection
	Close() error
}

// ProtocolClientOptions is an abstraction for a protocol client configuration
// E.g. for gRPC this is a GRPCClientOptions
type ProtocolClientOptions interface{}

func ConnectToProtocolServer(ctx context.Context, protocol moduletypes.Protocol, options ProtocolClientOptions, serverPort int, logger logging.Logger) (ClientConnection, error) {
	switch protocol {
	case moduletypes.ProtocolGRPC:
		if options == nil {
			options = GRPCClientOptions{}
		}
		grpcClientOptions, ok := options.(GRPCClientOptions)
		if !ok {
			return nil, errors.NewDomainError(errors.ErrorTypeValidation, "invalid protocol client options", nil).
				WithContext("protocol", protocol).
				WithContext("options", options)
		}
		return connectToGRPCServer(ctx, grpcClientOptions, serverPort, logger)
	}
	return nil, errors.NewValidationError("invalid protocol", nil).
		WithContext("protocol", protocol)
}
