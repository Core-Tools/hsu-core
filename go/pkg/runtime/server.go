package runtime

import (
	"context"
	"fmt"
	"net"

	"github.com/core-tools/hsu-core/pkg/errors"
	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/modules"

	"google.golang.org/grpc"
)

type grpcServerOptions struct {
	Port    int
	Options []grpc.ServerOption
}

func newGRPCServer(options grpcServerOptions, logger logging.Logger) (*grpcServer, error) {
	if options.Port <= 0 {
		return nil, errors.NewValidationError("invalid port number", nil).
			WithContext("port", options.Port)
	}

	address := fmt.Sprintf("127.0.0.1:%d", options.Port)
	logger.Infof("Creating gRPC server on address: %s", address)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, errors.NewIOError("failed to create network listener", err).
			WithContext("address", address).
			WithContext("port", options.Port)
	}

	logger.Infof("gRPC server listening at %s", listener.Addr().String())

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
	s.logger.Infof("Starting gRPC server at %s", s.listener.Addr().String())
	go func() {
		err := s.serverImpl.Serve(s.listener)
		if err != nil {
			s.logger.Errorf("gRPC server serve failed at %s: %v", s.listener.Addr().String(), err)
			return
		}
		s.logger.Infof("gRPC server stopped serving at %s", s.listener.Addr().String())
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
	if handlerRegistrarInfoReader == nil {
		logger.Errorf("Handler registrar info reader is nil")
		return nil
	}

	logger.Infof("Creating new server with gRPC port: %d", config.GRPC.Port)

	grpcServerOptions := grpcServerOptions{
		Port: config.GRPC.Port,
		// TODO: Add options
	}
	grpcServer, err := newGRPCServer(grpcServerOptions, logger)
	if err != nil {
		logger.Errorf("Failed to create gRPC server: %v", err)
		return nil
	}

	// Register all handlers
	handlerRegistrarInfos := handlerRegistrarInfoReader.GetAllHandlerRegistrarInfos()
	logger.Infof("Registering %d handlers", len(handlerRegistrarInfos))

	registeredCount := 0
	for key, handlerRegistrarInfo := range handlerRegistrarInfos {
		logger.Debugf("Processing handler for key: %s", key)
		
		directClosure := handlerRegistrarInfo.DirectClosure
		if directClosure == nil {
			logger.Warnf("Direct closure is nil for handler key: %s, skipping", key)
			continue
		}
		
		registrar := handlerRegistrarInfo.Registrar
		if registrar.GRPC != nil {
			if registrar.GRPC.RegistrarFunc == nil {
				logger.Warnf("gRPC registrar function is nil for key: %s, skipping", key)
				continue
			}
			
			logger.Debugf("Registering gRPC handler for key: %s", key)
			registrar.GRPC.RegistrarFunc(grpcServer.serverImpl, directClosure, logger)
			registeredCount++
			logger.Infof("gRPC handler registered successfully for key: %s", key)
		} else {
			logger.Debugf("No gRPC registrar for key: %s, skipping", key)
		}
	}

	logger.Infof("Server created successfully with %d handlers registered", registeredCount)

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
