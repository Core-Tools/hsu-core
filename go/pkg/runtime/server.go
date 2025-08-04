package runtime

import (
	"context"
	"fmt"
	"net"

	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modules"

	"google.golang.org/grpc"
)

type grpcServerOptions struct {
	Port    int
	Options []grpc.ServerOption
}

func newGRPCServer(options grpcServerOptions, logger logging.Logger) (*grpcServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", options.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen at port %d: %v", options.Port, err)
	}

	logger.Infof("Listening at %s", listener.Addr().String())

	serverImpl := grpc.NewServer(options.Options...)

	return &grpcServer{
		serverImpl: serverImpl,
		listener:   listener,
		logger:     logger,
	}, nil
}

type grpcServer struct {
	serverImpl *grpc.Server
	listener   net.Listener
	logger     logging.Logger
}

func (s *grpcServer) Start(ctx context.Context) {
	go func() {
		err := s.serverImpl.Serve(s.listener)
		if err != nil {
			s.logger.Errorf("gRPC server Serve failed: %v", err)
			return
		}
	}()
}

func (s *grpcServer) Shutdown(ctx context.Context) {
	s.logger.Infof("Stopping server...")

	if ctx == nil {
		ctx = context.Background()
	}

	stopped := make(chan struct{})

	s.logger.Infof("Stopping gRPC server...")
	// Use GracefulStop instead of Stop for graceful shutdown
	go func() {
		s.serverImpl.GracefulStop()
		close(stopped)
	}()

	// Wait for graceful shutdown or timeout
	select {
	case <-stopped:
		s.logger.Infof("gRPC server stopped gracefully")
	case <-ctx.Done():
		s.logger.Infof("Server shutdown timed out, forcing gRPC server to stop")
		s.serverImpl.Stop()
	}

	s.logger.Infof("Server stopped")
}

type GRPCServerConfig struct {
	Port int
}

type ServerConfig struct {
	GRPC GRPCServerConfig
}

type Server interface {
	Start(ctx context.Context)
	Shutdown(ctx context.Context)
}

func NewServer(config ServerConfig, handlerRegistrarInfoReader modules.HandlerRegistrarInfoReader, logger logging.Logger) Server {
	grpcServerOptions := grpcServerOptions{
		Port: config.GRPC.Port,
		// TODO: Add options
	}
	grpcServer, err := newGRPCServer(grpcServerOptions, logger)
	if err != nil {
		logger.Errorf("Failed to create gRPC server: %v", err)
		return nil
	}

	handlerRegistrarInfos := handlerRegistrarInfoReader.GetAllHandlerRegistrarInfos()

	for key, handlerRegistrarInfo := range handlerRegistrarInfos {
		directClosure := handlerRegistrarInfo.DirectClosure
		if directClosure == nil {
			logger.Warnf("Direct closure is nil for key %s", key)
			continue
		}
		registrar := handlerRegistrarInfo.Registrar
		if registrar.GRPC != nil {
			registrar.GRPC.RegistrarFunc(grpcServer.serverImpl, directClosure, logger)
		}
	}

	return &server{
		grpcServer: grpcServer,
		// TODO: Add other servers
	}
}

type server struct {
	grpcServer *grpcServer
	// TODO: Add other servers
}

func (s *server) Start(ctx context.Context) {
	s.grpcServer.Start(ctx)
	// TODO: Start other servers
}

func (s *server) Shutdown(ctx context.Context) {
	s.grpcServer.Shutdown(ctx)
	// TODO: Shutdown other servers
}
