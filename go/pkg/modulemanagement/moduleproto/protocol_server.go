package moduleproto

import (
	"fmt"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
	"google.golang.org/grpc"
)

type ProtocolServerHandlersVisitor interface {
	RegisterHandlersGRPC(grpcServerRegistrar grpc.ServiceRegistrar) error
	RegisterHandlersHTTP(httpServerRegistrar any /*TODO: ...*/) error
}

type ProtocolServer interface {
	Protocol() moduletypes.Protocol
	Port() int
	RegisterHandlers(visitor ProtocolServerHandlersVisitor) error
	moduletypes.Lifecycle
}

func ApplyToProtocolServers[Visitor ProtocolServerHandlersVisitor](protocolServers []ProtocolServer, visitor Visitor) error {
	for _, protocolServer := range protocolServers {
		err := protocolServer.RegisterHandlers(visitor)
		if err != nil {
			return err
		}
	}
	return nil
}

// ProtocolServerOptions is a union type for server configuration
type ProtocolServerOptions struct {
	GRPC GRPCServerOptions
	// HTTP HttpServerOptions // TODO: implement
}

func NewProtocolServer(protocol moduletypes.Protocol, options ProtocolServerOptions, logger logging.Logger) (ProtocolServer, error) {
	logger.Infof("Creating new protocol server for protocol: %s", protocol)

	switch protocol {
	case moduletypes.ProtocolGRPC:
		grpcServer, err := NewGRPCServer(options.GRPC, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC server: %w", err)
		}
		return grpcServer, nil
	}

	return nil, errors.NewDomainError(errors.ErrorTypeValidation, "invalid protocol", nil).WithContext("protocol", protocol)
}
