package moduleproto

import (
	"context"
	"fmt"
	"net"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"

	"google.golang.org/grpc"
)

type GRPCServerOptions struct {
	Port          int // this is only a config hint, could be 0 for dynamic port allocation
	ServerOptions []grpc.ServerOption
}

func (o *GRPCServerOptions) OptPort() int {
	if o.Port <= 0 {
		return 0
	}
	return o.Port
}

func NewGRPCServer(options GRPCServerOptions, logger logging.Logger) (*grpcServer, error) {
	port := options.OptPort()

	address := fmt.Sprintf("127.0.0.1:%d", port)
	logger.Infof("Creating gRPC server on address: %s", address)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, errors.NewIOError("failed to create network listener", err).
			WithContext("address", address).
			WithContext("port", port)
	}

	logger.Infof("gRPC server listening at %s", listener.Addr().String())

	serverOptions := options.ServerOptions
	if len(serverOptions) == 0 {
		serverOptions = []grpc.ServerOption{}
		logger.Debugf("Using default gRPC server options")
	}
	impl := grpc.NewServer(serverOptions...)

	// update the port to the actual port used
	port = listener.Addr().(*net.TCPAddr).Port

	return &grpcServer{
		port:     port,
		impl:     impl,
		listener: listener,
		logger:   logger,
	}, nil
}

type grpcServer struct {
	port     int
	impl     *grpc.Server
	listener net.Listener
	logger   logging.Logger
}

func (s *grpcServer) Port() int {
	return s.port
}

func (s *grpcServer) Protocol() moduletypes.Protocol {
	return moduletypes.ProtocolGRPC
}

func (s *grpcServer) RegisterHandlers(visitor ProtocolServerHandlersVisitor) error {
	return visitor.RegisterHandlersGRPC(s.impl)
}

func (s *grpcServer) Start(ctx context.Context) error {
	s.logger.Infof("Starting gRPC server at %s", s.listener.Addr().String())

	go func() {
		err := s.impl.Serve(s.listener)
		if err != nil {
			s.logger.Errorf("gRPC server serve failed at %s: %v", s.listener.Addr().String(), err)
			return
		}
		s.logger.Infof("gRPC server stopped serving at %s", s.listener.Addr().String())
	}()

	return nil
}

func (s *grpcServer) Stop(ctx context.Context) error {
	s.logger.Infof("Stopping server...")

	if ctx == nil {
		ctx = context.Background()
	}

	stopped := make(chan struct{})

	s.logger.Infof("Stopping gRPC server...")
	// Use GracefulStop instead of Stop for graceful shutdown
	go func() {
		s.impl.GracefulStop()
		close(stopped)
	}()

	// Wait for graceful shutdown or timeout
	select {
	case <-stopped:
		s.logger.Infof("gRPC server stopped gracefully")
	case <-ctx.Done():
		s.logger.Infof("Server shutdown timed out, forcing gRPC server to stop")
		s.impl.Stop()
	}

	s.logger.Infof("Server stopped")

	return nil
}
