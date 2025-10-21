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

func ConnectToProtocolServer(ctx context.Context, protocol moduletypes.Protocol, serverPort int, logger logging.Logger) (ClientConnection, error) {
	switch protocol {
	case moduletypes.ProtocolGRPC:
		return connectToGRPCServer(ctx, serverPort, logger)
	}
	return nil, errors.NewValidationError("invalid protocol", nil).
		WithContext("protocol", protocol)
}
