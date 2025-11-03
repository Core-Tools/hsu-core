package moduleproto

import (
	"context"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
	"google.golang.org/grpc"
)

type ClientConnectionVisitor interface {
	ProtocolIsDirect() error
	ProtocolIsGRPC(grpcClientConnection *grpc.ClientConn) error
	ProtocolIsHTTP(any /*TODO: ...*/) error
}

type ClientConnection interface {
	ApplyVisitor(visitor ClientConnectionVisitor) error
	Close() error
}

// ProtocolClientOptions is a union type for client configuration
type ProtocolClientOptions struct {
	GRPC GRPCClientOptions
	// HTTP HttpClientOptions // TODO: implement
}

func ConnectToProtocolServer(ctx context.Context, protocol moduletypes.Protocol, options ProtocolClientOptions, serverPort int, logger logging.Logger) (ClientConnection, error) {
	switch protocol {
	case moduletypes.ProtocolGRPC:
		return connectToGRPCServer(ctx, options.GRPC, serverPort, logger)
	}
	return nil, errors.NewValidationError("invalid protocol", nil).
		WithContext("protocol", protocol)
}
