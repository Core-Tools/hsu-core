package moduleproto

import (
	"context"
	"fmt"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"

	"google.golang.org/grpc"
)

type GRPCClientOptions struct {
	DialOptions []grpc.DialOption
}

func connectToGRPCServer(ctx context.Context, options GRPCClientOptions, port int, logger logging.Logger) (ClientConnection, error) {
	address := fmt.Sprintf("localhost:%d", port)

	logger.Debugf("Connecting to gRPC server at address: %s", address)

	// Setup dial options
	dialOptions := options.DialOptions
	if len(dialOptions) == 0 {
		dialOptions = []grpc.DialOption{
			grpc.WithInsecure(),
		}
		logger.Debugf("Using default insecure gRPC dial options")
	}

	// Create gRPC connection
	conn, err := grpc.DialContext(ctx, address, dialOptions...)
	if err != nil {
		return nil, errors.NewIOError("failed to establish gRPC connection", err).
			WithContext("address", address)
	}

	logger.Infof("gRPC client connection created successfully at %s", address)

	return &grpcClientConnection{
		conn:   conn,
		logger: logger,
	}, nil
}

type grpcClientConnection struct {
	conn   *grpc.ClientConn
	logger logging.Logger
}

func (c *grpcClientConnection) ApplyVisitor(visitor ClientConnectionVisitor) error {
	return visitor.ProtocolIsGRPC(c.conn)
}

func (c *grpcClientConnection) Close() error {
	return c.conn.Close()
}
