package control

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/core-tools/hsu-core/pkg/logging"

	"google.golang.org/grpc"
)

type ServerOptions struct {
	Port int
}

type Server interface {
	GRPC() grpc.ServiceRegistrar
	Run(onShutdownFunc func())
	RunWithContextAndStopped(ctx context.Context, stopped chan struct{}, onShutdownFunc func())
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

func (s *server) Run(onShutdownFunc func()) {
	// Create a timeout context for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	stopped := make(chan struct{})
	s.RunWithContextAndStopped(ctx, stopped, onShutdownFunc)
}

func (s *server) RunWithContextAndStopped(ctx context.Context, stopped chan struct{}, onShutdownFunc func()) {
	go func() {
		err := s.grpcServer.Serve(s.listener)
		if err != nil {
			s.logger.Errorf("LLM gRPC server Serve failed: %v", err)
			return
		}
	}()

	sig := make(chan os.Signal, 1)
	if runtime.GOOS == "windows" {
		signal.Notify(sig) // Unix signals not implemented on Windows
	} else {
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	}

	receivedSignal := <-sig

	if runtime.GOOS == "windows" {
		if receivedSignal != os.Interrupt {
			s.logger.Errorf("Wrong signal received: got %q, want %q\n", receivedSignal, os.Interrupt)
			os.Exit(42)
		}
	}

	s.logger.Infof("Received signal: %v", receivedSignal)

	s.logger.Infof("Stopping...")

	// Stop components in reverse order of creation
	if onShutdownFunc != nil {
		onShutdownFunc()
	}

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
		s.logger.Infof("Shutdown timed out, forcing gRPC server to stop")
		s.grpcServer.Stop()
	}

	s.logger.Infof("Stopped")
}
