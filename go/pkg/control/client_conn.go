package control

import (
	"fmt"
	"time"

	"github.com/core-tools/hsu-core/pkg/logging"
	"github.com/core-tools/hsu-core/pkg/process"

	"github.com/phayes/freeport"
	"google.golang.org/grpc"
)

type ConnectionOptions struct {
	ServerPath string
	AttachPort int
}

type Connection interface {
	GRPC() grpc.ClientConnInterface
	Shutdown()
}

func NewConnection(options ConnectionOptions, logger logging.Logger) (Connection, error) {
	var serverProcessController process.Controller

	port := options.AttachPort
	if port == 0 {
		logger.Debugf("Allocating free port")

		var err error
		port, err = freeport.GetFreePort()
		if err != nil {
			return nil, fmt.Errorf("failed allocating free ports: %v", err)
		}

		logger.Debugf("Allocated free port: %d", port)

		serverProcessController, err = newServerProcess(port, options.ServerPath, 0, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create server process: %v", err)
		}
	}

	address := fmt.Sprintf("127.0.0.1:%d", port)

	logger.Debugf("Dialing server process at %s", address)

	dialOpts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithReadBufferSize(1 * 1024 * 1024),
		grpc.WithInitialWindowSize(1 * 1024 * 1024),
		grpc.WithInitialConnWindowSize(1 * 1024 * 1024),
	}

	grpcClientConnection, err := grpc.NewClient(address, dialOpts...)
	if err != nil {
		if serverProcessController != nil {
			serverProcessController.Stop()
		}
		return nil, err
	}

	logger.Debugf("Connected to server process at %s", address)

	connection := &connection{
		grpcClientConnection:    grpcClientConnection,
		serverProcessController: serverProcessController,
		logger:                  logger,
	}

	return connection, nil
}

func newServerProcess(port int, serverPath string, retryPeriod time.Duration, logger logging.Logger) (process.Controller, error) {
	args := []string{"--port", fmt.Sprintf("%d", port)}

	logger.Debugf("Running server process '%s' with args: %v", serverPath, args)

	logConfig := process.ControllerLogConfig{
		Module: "process",
		Funcs: logging.LogFuncs{
			LogLevelf: logger.LogLevelf,
			Debugf:    logger.Debugf,
			Infof:     logger.Infof,
			Warnf:     logger.Warnf,
			Errorf:    logger.Errorf,
		},
	}
	controller, err := process.NewController(serverPath, args, retryPeriod, logConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create server process: %v", err)
	}

	return controller, nil
}

type connection struct {
	grpcClientConnection    *grpc.ClientConn
	serverProcessController process.Controller
	logger                  logging.Logger
}

func (c *connection) GRPC() grpc.ClientConnInterface {
	return c.grpcClientConnection
}

func (c *connection) Shutdown() {
	c.logger.Debugf("Stopping gRPC client connection...")
	c.grpcClientConnection.Close()
	c.logger.Debugf("gRPC client connection stopped")

	c.logger.Debugf("Stopping server process...")
	if c.serverProcessController != nil {
		c.serverProcessController.Stop()
	}
	c.logger.Debugf("Server process stopped")
}
