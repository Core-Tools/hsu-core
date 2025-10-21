package moduleproto

import (
	"fmt"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
)

// ProtocolServerRegistrar is an abstraction for a protocol server registration mechanism
// E.g. for gRPC this is a grpc.ServiceRegistrar
type ProtocolServerRegistrar interface{}

type ProtocolServer interface {
	Protocol() moduletypes.Protocol
	Port() int
	HandlersRegistrar() ProtocolServerRegistrar
	moduletypes.Lifecycle
}

// ProtocolServerOptions is an abstraction for a protocol server configuration
// E.g. for gRPC this is a GRPCServerOptions
type ProtocolServerOptions interface{}

func NewProtocolServer(protocol moduletypes.Protocol, options ProtocolServerOptions, logger logging.Logger) (ProtocolServer, error) {
	logger.Infof("Creating new protocol server for protocol: %s", protocol)

	switch protocol {
	case moduletypes.ProtocolGRPC:
		grpcServerOptions, ok := options.(GRPCServerOptions)
		if !ok {
			return nil, errors.NewDomainError(errors.ErrorTypeValidation, "invalid protocol server options", nil).
				WithContext("protocol", protocol).
				WithContext("options", options)
		}
		grpcServer, err := NewGRPCServer(grpcServerOptions, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC server: %w", err)
		}
		return grpcServer, nil
	}

	return nil, errors.NewDomainError(errors.ErrorTypeValidation, "invalid protocol", nil).WithContext("protocol", protocol)
}
