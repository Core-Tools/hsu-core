package control

import (
	"context"
	"fmt"
	"net"

	"github.com/core-tools/hsu-core/pkg/logging"

	"google.golang.org/grpc"
)

type ServerOptions struct {
	Port int
}

type Server interface {
	GRPC() grpc.ServiceRegistrar
	Start(ctx context.Context)
	Shutdown(ctx context.Context)
}

func NewServer(options ServerOptions, logger logging.Logger) (Server, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", options.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen at port %d: %v", options.Port, err)
	}

	logger.Infof("Listening at %s", listener.Addr().String())

	grpcServer := grpc.NewServer(
		grpc.WriteBufferSize(1*1024*1024),
		grpc.InitialWindowSize(1*1024*1024),
		grpc.InitialConnWindowSize(1*1024*1024),
	)

	return &server{
		grpcServer: grpcServer,
		listener:   listener,
		logger:     logger,
	}, nil
}

type server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	logger     logging.Logger
}

func (s *server) GRPC() grpc.ServiceRegistrar {
	return s.grpcServer
}

func (s *server) Start(ctx context.Context) {
	go func() {
		err := s.grpcServer.Serve(s.listener)
		if err != nil {
			s.logger.Errorf("gRPC server Serve failed: %v", err)
			return
		}
	}()
}

func (s *server) Shutdown(ctx context.Context) {
	s.logger.Infof("Stopping server...")

	if ctx == nil {
		ctx = context.Background()
	}

	stopped := make(chan struct{})

	s.logger.Infof("Stopping gRPC server...")
	// Use GracefulStop instead of Stop for graceful shutdown
	go func() {
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	// Wait for graceful shutdown or timeout
	select {
	case <-stopped:
		s.logger.Infof("gRPC server stopped gracefully")
	case <-ctx.Done():
		s.logger.Infof("Server shutdown timed out, forcing gRPC server to stop")
		s.grpcServer.Stop()
	}

	s.logger.Infof("Server stopped")
}
